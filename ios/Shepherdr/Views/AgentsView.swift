import SwiftUI

/// Home: every agent on every machine, most urgent first.
///
/// Adapts to the width it gets: a list that pushes an agent on phones and the
/// folded iPhone Duo, and a list + agent split view when there's room (the
/// unfolded Duo, landscape Pro Max). Both share one selection, so folding or
/// unfolding keeps the open agent.
struct AgentsView: View {
    @Environment(HerdrStore.self) private var store
    @Environment(\.horizontalSizeClass) private var sizeClass
    @AppStorage("showShells") private var showShells = false
    @State private var showSettings = false
    @State private var showNewAgent = false
    /// The open agent (compact) or the selected one (split).
    @State private var selected: String?

    private var isSplit: Bool { sizeClass == .regular }

    var body: some View {
        Group {
            if isSplit { split } else { stack }
        }
        .sheet(isPresented: $showSettings) { SettingsView() }
        .sheet(isPresented: $showNewAgent) { NewAgentView() }
        #if DEBUG
        .task {
            // `simctl launch <app> -openTerminal simrig/t9`, for screenshots.
            if let a = UserDefaults.standard.string(forKey: "openTerminal"), selected == nil { selected = a }
            if UserDefaults.standard.bool(forKey: "openNewAgent") {
                try? await Task.sleep(for: .seconds(2)) // let the snapshot arrive
                showNewAgent = true
            }
        }
        #endif
        .task {
            // A link or notification that arrived before this view existed.
            if let address = AppRouter.shared.pendingAddress {
                selected = address
                AppRouter.shared.pendingAddress = nil
            }
        }
        .onChange(of: AppRouter.shared.pendingAddress) { _, address in
            // A tapped notification or shepherdr://terminal link.
            guard let address else { return }
            selected = address
            AppRouter.shared.pendingAddress = nil
        }
    }

    // MARK: Layouts

    private var stack: some View {
        NavigationStack(path: Binding(get: { selected.map { [$0] } ?? [] }, set: { selected = $0.last })) {
            agentList
                .navigationDestination(for: String.self) { AgentDetailView(address: $0) }
        }
    }

    private var split: some View {
        NavigationSplitView {
            agentList
                .navigationSplitViewColumnWidth(min: 300, ideal: 360, max: 440)
        } detail: {
            // Its own stack, so the agent can push its live terminal.
            NavigationStack {
                if let selected {
                    AgentDetailView(address: selected).id(selected)
                } else {
                    ContentUnavailableView("No agent selected", systemImage: "sidebar.left",
                                           description: Text("Pick an agent to see its screen."))
                }
            }
        }
        .onAppear { if selected == nil { selected = mostUrgent } }
        .onChange(of: store.terminals.count) { if selected == nil { selected = mostUrgent } }
    }

    /// The agent to show first when there's room for a detail column.
    private var mostUrgent: String? {
        AgentState.allCases.lazy.compactMap { store.agents(in: $0).first?.address }.first
    }

    private var agentList: some View {
        // Always bound to `selected`: a List with a selection binding routes
        // NavigationLink taps into it instead of pushing, so a constant one
        // makes rows dead. The stack's path follows `selected`, so this
        // pushes on phones and fills the detail column when split.
        List(selection: $selected) {
            ConnectionBanner()
            ForEach(store.unavailableMachines) { m in MachineDownRow(machine: m) }
            ForEach(AgentState.allCases, id: \.self) { state in
                let agents = store.agents(in: state)
                if !agents.isEmpty {
                    Section {
                        ForEach(agents) { t in
                            NavigationLink(value: t.address) { AgentRow(terminal: t) }
                        }
                    } header: {
                        Label(state.title, systemImage: state.symbol)
                            .foregroundStyle(state.color)
                    }
                }
            }
            if showShells, !store.shells.isEmpty {
                Section("Terminals") {
                    ForEach(store.shells) { t in
                        NavigationLink(value: t.address) { AgentRow(terminal: t) }
                    }
                }
            }
            if store.connection == .live, store.terminals.values.allSatisfy({ !$0.isLive || !$0.isAgent }) {
                ContentUnavailableView("No agents running", systemImage: "terminal",
                                       description: Text("Agents started in Herdr on any machine show up here."))
            }
        }
        .navigationTitle("Agents")
        .refreshable { await store.resync() }
        .toolbar {
            ToolbarItem(placement: .topBarLeading) {
                Toggle(isOn: $showShells) { Image(systemName: "apple.terminal") }
                    .toggleStyle(.button)
                    .accessibilityLabel("Show terminals without agents")
            }
            ToolbarItem(placement: .topBarTrailing) {
                Button { showSettings = true } label: { Image(systemName: "gearshape") }
            }
            ToolbarItem(placement: .topBarTrailing) {
                Button { showNewAgent = true } label: { Image(systemName: "plus") }
                    .accessibilityLabel("New agent")
                    .disabled(store.connection != .live)
            }
        }
    }

    static func address(from url: URL) -> String? {
        guard url.scheme == "shepherdr", url.host() == "terminal" else { return nil }
        let address = url.path().trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        return address.isEmpty ? nil : address
    }
}

struct AgentRow: View {
    @Environment(HerdrStore.self) private var store
    let terminal: Terminal

    var body: some View {
        HStack(alignment: .top, spacing: 12) {
            Image(systemName: terminal.agentState?.symbol ?? "terminal")
                .foregroundStyle(terminal.agentState?.color ?? .secondary)
                .symbolEffect(.pulse, isActive: terminal.agentState == .working)
                .frame(width: 22)
                .padding(.top, 2)
            VStack(alignment: .leading, spacing: 3) {
                HStack(spacing: 6) {
                    Text(terminal.displayName).font(.headline).lineLimit(1)
                    MachineBadge(name: store.machineLabel(for: terminal))
                    Spacer(minLength: 4)
                    Text(terminal.localAddress).font(.caption.monospaced()).foregroundStyle(.tertiary)
                }
                if let detail = terminal.statusLine {
                    Text(detail).font(.subheadline).foregroundStyle(.secondary).lineLimit(2)
                }
                HStack(spacing: 8) {
                    if let w = terminal.whereLabel { Label(w, systemImage: "square.split.2x1") }
                    if let d = terminal.shortDirectory { Label(d, systemImage: "folder") }
                }
                .font(.caption)
                .foregroundStyle(.secondary)
                .lineLimit(1)
                .labelStyle(.titleAndIcon)
            }
        }
        .padding(.vertical, 2)
        .opacity(store.isReachable(terminal) ? 1 : 0.45)
    }
}

/// A machine the hub can't reach right now, with a way to redial it.
struct MachineDownRow: View {
    @Environment(HerdrStore.self) private var store
    let machine: Machine
    @State private var busy = false
    @State private var error: String?

    var body: some View {
        HStack(alignment: .top, spacing: 12) {
            Image(systemName: "bolt.horizontal.circle").foregroundStyle(.orange).padding(.top, 2)
            VStack(alignment: .leading, spacing: 2) {
                Text("\(machine.name) is unavailable").font(.subheadline.weight(.semibold))
                Text(error ?? Self.explain(machine.state)).font(.caption).foregroundStyle(.secondary)
            }
            Spacer()
            Button {
                busy = true
                Task {
                    defer { busy = false }
                    do { try await store.reconnect(machine.name); error = nil } catch { self.error = error.localizedDescription }
                }
            } label: {
                if busy { ProgressView() } else { Text("Reconnect") }
            }
            .buttonStyle(.bordered)
            .disabled(busy)
        }
    }

    static func explain(_ state: String) -> String {
        switch state {
        case "needs_login": "The hub's ssh login to it needs attention (e.g. a Tailscale SSH re-check). The bridge retries on its own."
        default: "State: \(state.replacingOccurrences(of: "_", with: " ")). It may be asleep or offline."
        }
    }
}

struct MachineBadge: View {
    let name: String
    var body: some View {
        Text(name)
            .font(.caption2.weight(.medium))
            .lineLimit(1)
            .truncationMode(.middle)
            .padding(.horizontal, 6)
            .padding(.vertical, 2)
            .background(.tint.opacity(0.15), in: Capsule())
            .foregroundStyle(.tint)
    }
}

struct ConnectionBanner: View {
    @Environment(HerdrStore.self) private var store

    var body: some View {
        switch store.connection {
        case .live, .disconnected:
            EmptyView()
        case .connecting:
            Label("Connecting…", systemImage: "antenna.radiowaves.left.and.right")
                .foregroundStyle(.secondary)
        case .failed(let message):
            Label {
                VStack(alignment: .leading) {
                    Text("Can't reach the hub")
                    Text(message).font(.caption).foregroundStyle(.secondary)
                }
            } icon: {
                Image(systemName: "wifi.exclamationmark").foregroundStyle(.orange)
            }
        }
    }
}

extension AgentState {
    var color: Color {
        switch self {
        case .blocked: .red
        case .error: .orange
        case .done: .green
        case .working: .blue
        case .idle: .secondary
        }
    }
}

extension Terminal {
    /// The most useful one-liner: the status message, else the title.
    var statusLine: String? {
        if let msg = status?.msg, !msg.isEmpty { return msg }
        if let t = status?.title, !t.isEmpty { return t }
        if let t = title, !t.isEmpty { return t }
        return nil
    }
}

#Preview {
    AgentsView().environment(PreviewData.store)
}
