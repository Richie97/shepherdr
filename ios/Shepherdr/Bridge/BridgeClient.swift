import Foundation

/// An error from the bridge (`{error:{kind,message}}`) or passed through
/// from herdr-core (`{error:{code,message,data:{kind,hint}}}`).
struct BridgeError: LocalizedError, Decodable, Sendable {
    struct Data: Decodable, Sendable {
        var kind: String?
        var hint: String?
    }
    var kind: String?
    var message: String
    var code: Int?
    var data: Data?

    /// The stable error kind, from either shape.
    var errorKind: String? { data?.kind ?? kind }
    var errorDescription: String? { message }

    init(kind: String?, message: String) {
        self.kind = kind
        self.message = message
    }
}

/// Talks to herdr-bridge over HTTPS. See docs/protocol.md.
struct BridgeClient: Sendable {
    let baseURL: URL
    var session: URLSession = .shared

    private struct RPCBody<P: Encodable>: Encodable {
        var method: String
        var params: P
    }

    private struct Envelope<R: Decodable>: Decodable {
        var result: R?
        var error: BridgeError?
    }

    func health() async throws -> Health {
        var req = URLRequest(url: baseURL.appending(path: "v1/health"))
        req.timeoutInterval = 10
        let (data, _) = try await session.data(for: req)
        // 503 still carries a Health body with core_error.
        if let health = try? JSONDecoder().decode(Health.self, from: data) {
            return health
        }
        throw try decodeError(data) ?? BridgeError(kind: "bad_response", message: "Unexpected response from bridge")
    }

    func call<P: Encodable & Sendable, R: Decodable & Sendable>(
        _ method: String, _ params: P, as _: R.Type = R.self, timeout: TimeInterval = 30
    ) async throws -> R {
        var req = URLRequest(url: baseURL.appending(path: "v1/rpc"))
        req.httpMethod = "POST"
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        req.timeoutInterval = timeout
        req.httpBody = try JSONEncoder().encode(RPCBody(method: method, params: params))
        let (data, _) = try await session.data(for: req)
        let env: Envelope<R>
        do {
            env = try JSONDecoder().decode(Envelope<R>.self, from: data)
        } catch {
            throw try decodeError(data) ?? BridgeError(kind: "bad_response", message: "Could not read \(method) response: \(error)")
        }
        if let err = env.error { throw err }
        guard let result = env.result else {
            throw BridgeError(kind: "bad_response", message: "\(method) returned nothing")
        }
        return result
    }

    private func decodeError(_ data: Foundation.Data) throws -> BridgeError? {
        try? JSONDecoder().decode(Envelope<Empty>.self, from: data).error
    }

    private struct Empty: Decodable {}

    /// The event stream WebSocket URL.
    var eventsURL: URL {
        var c = URLComponents(url: baseURL.appending(path: "v1/ws"), resolvingAgainstBaseURL: false)!
        c.scheme = c.scheme == "http" ? "ws" : "wss"
        return c.url!
    }
}

// MARK: - Typed calls

extension BridgeClient {
    private struct TerminalParam: Encodable, Sendable { var terminal: String }

    func list() async throws -> TerminalList {
        try await call("terminal.list", [String: Bool]())
    }

    func get(_ address: String) async throws -> Terminal {
        try await call("terminal.get", TerminalParam(terminal: address))
    }

    func read(_ address: String, lines: Int = 300) async throws -> ReadResult {
        struct P: Encodable, Sendable { var terminal: String; var lines: Int }
        return try await call("terminal.read", P(terminal: address, lines: lines))
    }

    /// Pastes text and presses Enter. `force` types even when the agent is
    /// blocked at a dialog (it may answer it).
    func send(_ address: String, text: String, force: Bool = false) async throws -> TurnResult {
        struct P: Encodable, Sendable { var terminal: String; var text: String; var force: Bool }
        // The core waits for ~3 s of quiet before typing.
        return try await call("terminal.send", P(terminal: address, text: text, force: force), timeout: 60)
    }

    /// Presses named keys: enter, esc, tab, up, ctrl+c, a, …
    func keys(_ address: String, _ keys: [String]) async throws -> TurnResult {
        struct P: Encodable, Sendable { var terminal: String; var keys: [String] }
        return try await call("terminal.keys", P(terminal: address, keys: keys), timeout: 60)
    }

    func interrupt(_ address: String) async throws -> InterruptResult {
        try await call("terminal.interrupt", TerminalParam(terminal: address), timeout: 30)
    }
}

// MARK: - Starting agents

struct StartRequest: Encodable, Sendable {
    var on: String?
    var cwd: String?
    var command: String
    var task: String
    var name: String?
    var space: String?
    var tab: String?
}

struct StartResult: Decodable, Sendable {
    var address: String
    var space: String
    var tab: String
}

extension BridgeClient {
    /// Starts an agent the way the Herdr app does (see bridge /v1/start).
    func start(_ req: StartRequest) async throws -> StartResult {
        struct Response: Decodable {
            var address: String?
            var space: String?
            var tab: String?
            var error: BridgeError?
        }
        var r = URLRequest(url: baseURL.appending(path: "v1/start"))
        r.httpMethod = "POST"
        r.setValue("application/json", forHTTPHeaderField: "Content-Type")
        r.timeoutInterval = 60
        r.httpBody = try JSONEncoder().encode(req)
        let (data, _) = try await session.data(for: r)
        let resp = try JSONDecoder().decode(Response.self, from: data)
        if let err = resp.error { throw err }
        guard let address = resp.address else { throw BridgeError(kind: "bad_response", message: "The hub didn't return the new terminal") }
        return StartResult(address: address, space: resp.space ?? "", tab: resp.tab ?? "")
    }
}
