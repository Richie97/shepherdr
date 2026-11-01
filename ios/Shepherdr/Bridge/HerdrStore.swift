import Foundation
import Observation

/// Live state of every terminal on the hub and its machines, kept current
/// from the bridge's event WebSocket.
@MainActor
@Observable
final class HerdrStore {
    enum Connection: Equatable {
        case disconnected
        case connecting
        case live
        case failed(String)
    }

    private(set) var terminals: [String: Terminal] = [:]
    private(set) var connection: Connection = .disconnected
    private(set) var health: Health?
    /// Bumped whenever a terminal changes, so open views can refresh.
    private(set) var revisions: [String: Int] = [:]

    private(set) var client: BridgeClient?
    private var streamTask: Task<Void, Never>?
    private var healthTask: Task<Void, Never>?
    private var refreshing: Set<String> = []
    private var backoff: Duration = .seconds(1)

    var hubName: String { health?.core?.core.name ?? "hub" }

    var machines: [Machine] { health?.machines?.machines ?? [] }

    /// Live agents grouped by state, most urgent first.
    func agents(in state: AgentState) -> [Terminal] {
        terminals.values
            .filter { $0.isLive && $0.agentState == state }
            .sorted { ($0.statusMs ?? 0) > ($1.statusMs ?? 0) }
    }

    /// Live terminals that don't report status (shells, servers, …).
    var shells: [Terminal] {
        terminals.values.filter { $0.isLive && !$0.isAgent }.sorted { $0.address < $1.address }
    }

    var blockedCount: Int { terminals.values.count { $0.isLive && $0.agentState == .blocked } }

    func machineLabel(for t: Terminal) -> String { t.machine ?? hubName }

    // MARK: Connection

    func connect(to url: URL) {
        disconnect()
        let client = BridgeClient(baseURL: url)
        self.client = client
        streamTask = Task { [weak self] in await self?.run(client) }
        // Machine link states aren't in the event stream; poll them.
        healthTask = Task { [weak self] in
            while !Task.isCancelled {
                try? await Task.sleep(for: .seconds(30))
                await self?.refreshHealth()
            }
        }
    }

    func refreshHealth() async {
        guard let client, let h = try? await client.health() else { return }
        health = h
    }

    /// A machine's link state as seen by the hub (nil for the hub itself).
    func machineState(_ name: String?) -> String? {
        guard let name else { return nil }
        return machines.first { $0.name == name }?.state
    }

    /// Machines whose link to the hub is down.
    var unavailableMachines: [Machine] { machines.filter { $0.state != "connected" && $0.state != "connecting" } }

    func isReachable(_ t: Terminal) -> Bool {
        guard let state = machineState(t.machine) else { return true }
        return state == "connected"
    }

    /// Asks the hub to redial a machine (after an ssh re-login, a sleep, …).
    func reconnect(_ machine: String) async throws {
        guard let client else { return }
        struct P: Encodable, Sendable { var machine: String }
        struct R: Decodable, Sendable { var state: String }
        let r = try await client.call("machine.connect", P(machine: machine), as: R.self, timeout: 60)
        await refreshHealth()
        if r.state != "connected" {
            throw BridgeError(kind: r.state, message: "\(machine) is still \(r.state.replacingOccurrences(of: "_", with: " "))")
        }
        await resync()
    }

    func disconnect() {
        streamTask?.cancel()
        streamTask = nil
        healthTask?.cancel()
        healthTask = nil
        client = nil
        connection = .disconnected
    }

    /// Keeps the event stream open, reconnecting with backoff.
    private func run(_ client: BridgeClient) async {
        backoff = .seconds(1)
        while !Task.isCancelled {
            connection = .connecting
            do {
                health = try await client.health()
                try await stream(client) { [weak self] in
                    self?.connection = .live
                    self?.backoff = .seconds(1)
                }
            } catch is CancellationError {
                return
            } catch {
                if Task.isCancelled { return }
                connection = .failed(error.localizedDescription)
            }
            try? await Task.sleep(for: backoff)
            backoff = min(backoff * 2, .seconds(30))
        }
    }

    private func stream(_ client: BridgeClient, onOpen: @escaping @MainActor () -> Void) async throws {
        let ws = client.session.webSocketTask(with: client.eventsURL)
        ws.maximumMessageSize = 32 << 20
        ws.resume()
        defer { ws.cancel(with: .goingAway, reason: nil) }
        try await withTaskCancellationHandler {
            var opened = false
            while true {
                let message = try await ws.receive()
                let data: Data = switch message {
                case .data(let d): d
                case .string(let s): Data(s.utf8)
                @unknown default: Data()
                }
                if !opened {
                    opened = true
                    onOpen()
                }
                apply(data)
            }
        } onCancel: {
            ws.cancel(with: .goingAway, reason: nil)
        }
    }

    // MARK: Events

    /// Applies one stream message. Internal for tests.
    func apply(_ data: Data) {
        guard let msg = try? JSONDecoder().decode(StreamMessage.self, from: data) else { return }
        if let snapshot = msg.result {
            terminals = Dictionary(snapshot.terminals.map { ($0.address, $0) }, uniquingKeysWith: { $1 })
            for address in terminals.keys { bump(address) }
            return
        }
        switch msg.method {
        case "event":
            guard let type = msg.params?.type, let address = msg.params?.terminal else { return }
            switch type {
            case "created":
                if let created = try? JSONDecoder().decode(CreatedEvent.self, from: data) {
                    terminals[address] = created.params.data
                }
            case "removed":
                terminals[address] = nil
            case "size", "doc":
                return // nothing the app shows
            default:
                // status, turn, title, labels, foreground, finished, lost,
                // restarted, keep: refetch rather than patch, so the app
                // never depends on each event's data shape.
                refresh(address)
            }
            bump(address)
        case "missed":
            Task { await resync() }
        default:
            break // heartbeat
        }
    }

    private func bump(_ address: String) {
        revisions[address, default: 0] += 1
    }

    /// Refetches one terminal, coalescing bursts of events.
    func refresh(_ address: String) {
        guard let client, !refreshing.contains(address) else { return }
        refreshing.insert(address)
        Task {
            defer { refreshing.remove(address) }
            if let t = try? await client.get(address) {
                terminals[address] = t
                bump(address)
            }
        }
    }

    /// Replaces all state from terminal.list (after the stream fell behind).
    func resync() async {
        guard let client, let list = try? await client.list() else { return }
        terminals = Dictionary(list.terminals.map { ($0.address, $0) }, uniquingKeysWith: { $1 })
        for address in terminals.keys { bump(address) }
    }

    /// For previews and tests.
    func load(_ terminals: [Terminal], health: Health? = nil) {
        self.terminals = Dictionary(terminals.map { ($0.address, $0) }, uniquingKeysWith: { $1 })
        self.health = health
        connection = .live
    }
}
