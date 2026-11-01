import SwiftUI

/// Home: every agent on every machine, most urgent first.
struct AgentsView: View {
    @Environment(HerdrStore.self) private var store
    @AppStorage("showShells") private var showShells = false
    @State private var showSettings = false
    @State private var showNewAgent = false
    @State private var path: [String] = []

    var body: some View {
        NavigationStack(path: $path) {
            List {
                ConnectionBanner()
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
            .navigationDestination(for: String.self) { AgentDetailView(address: $0) }
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
            .sheet(isPresented: $showSettings) { SettingsView() }
            .sheet(isPresented: $showNewAgent) { NewAgentView() }
        }
        #if DEBUG
        .task {
            // `simctl launch <app> -openTerminal simrig/t9`, for screenshots.
            if let a = UserDefaults.standard.string(forKey: "openTerminal"), path.isEmpty { path = [a] }
            if UserDefaults.standard.bool(forKey: "openNewAgent") {
                try? await Task.sleep(for: .seconds(2)) // let the snapshot arrive
                showNewAgent = true
            }
        }
        #endif
        .task {
            // A link or notification that arrived before this view existed.
            if let address = AppRouter.shared.pendingAddress {
                path = [address]
                AppRouter.shared.pendingAddress = nil
            }
        }
        .onChange(of: AppRouter.shared.pendingAddress) { _, address in
            // A tapped notification or herdr://terminal link.
            guard let address else { return }
            path = [address]
            AppRouter.shared.pendingAddress = nil
        }
    }

    static func address(from url: URL) -> String? {
        guard url.scheme == "herdr", url.host() == "terminal" else { return nil }
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
