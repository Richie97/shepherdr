import SwiftUI

/// A Herdr workspace ("space") on one machine, derived from its terminals'
/// `herdr.space` / `herdr.tab` / `where` labels.
struct HerdrSpace: Identifiable, Hashable {
    struct Tab: Identifiable, Hashable {
        var id: String
        var name: String
    }
    var id: String
    var name: String
    var tabs: [Tab]
    var cwd: String?
}

extension HerdrStore {
    /// Machine keys for `terminal.run`'s `on`: nil is the hub itself.
    var startableMachines: [(key: String?, name: String)] {
        [(nil, hubName)] + machines.filter { $0.state == "connected" }.map { ($0.name, $0.name) }
    }

    func terminals(on machine: String?) -> [Terminal] {
        terminals.values.filter { $0.isLive && $0.machine == machine }
    }

    func spaces(on machine: String?) -> [HerdrSpace] {
        var spaces: [String: HerdrSpace] = [:]
        for t in terminals(on: machine).sorted(by: { ($0.createdMs ?? 0) < ($1.createdMs ?? 0) }) {
            guard let space = t.labels?["herdr.space"] else { continue }
            let whereParts = (t.whereLabel ?? "").components(separatedBy: " / ")
            var s = spaces[space] ?? HerdrSpace(id: space, name: whereParts.first.flatMap { $0.isEmpty ? nil : $0 } ?? "Workspace",
                                                tabs: [], cwd: t.currentDirectory)
            if let tab = t.labels?["herdr.tab"], !s.tabs.contains(where: { $0.id == tab }) {
                s.tabs.append(.init(id: tab, name: t.whereLabel ?? "Tab \(s.tabs.count + 1)"))
            }
            spaces[space] = s
        }
        return spaces.values.sorted { $0.name.localizedCaseInsensitiveCompare($1.name) == .orderedAscending }
    }

    /// Working directories in use on a machine, most common first.
    func directories(on machine: String?) -> [String] {
        var counts: [String: Int] = [:]
        for t in terminals(on: machine) { if let d = t.currentDirectory { counts[d, default: 0] += 1 } }
        return counts.sorted { $0.value != $1.value ? $0.value > $1.value : $0.key < $1.key }.map(\.key)
    }
}

struct NewAgentView: View {
    enum Agent: String, CaseIterable, Identifiable {
        case claude = "Claude Code", codex = "Codex", shell = "Shell", custom = "Custom"
        var id: String { rawValue }
        var command: String? {
            switch self {
            case .claude: "claude"
            case .codex: "codex"
            case .shell: ""
            case .custom: nil
            }
        }
    }

    @Environment(HerdrStore.self) private var store
    @Environment(\.dismiss) private var dismiss

    @State private var machine: String?
    @State private var spaceID = ""   // "" = new workspace
    @State private var tabID = ""     // "" = new tab
    @State private var cwd = ""
    @State private var agent = Agent.claude
    @State private var customCommand = ""
    @State private var name = ""
    @State private var task = ""
    @State private var starting = false
    @State private var error: String?

    private var spaces: [HerdrSpace] { store.spaces(on: machine) }
    private var space: HerdrSpace? { spaces.first { $0.id == spaceID } }

    private var command: String {
        (agent.command ?? customCommand).trimmingCharacters(in: .whitespacesAndNewlines)
    }

    static func isValidName(_ s: String) -> Bool {
        s.isEmpty || s.range(of: #"^[a-z0-9_.-]{1,32}$"#, options: .regularExpression) != nil
    }

    var body: some View {
        NavigationStack {
            Form {
                Section("Where") {
                    Picker("Machine", selection: $machine) {
                        ForEach(store.startableMachines, id: \.name) { m in Text(m.name).tag(m.key) }
                    }
                    Picker("Workspace", selection: $spaceID) {
                        Text("New workspace").tag("")
                        ForEach(spaces) { s in Text(s.name).tag(s.id) }
                    }
                    if let space {
                        Picker("Tab", selection: $tabID) {
                            Text("New tab").tag("")
                            ForEach(space.tabs) { t in Text(t.name).tag(t.id) }
                        }
                    }
                    HStack {
                        TextField("Folder (default: home)", text: $cwd)
                            .textInputAutocapitalization(.never)
                            .autocorrectionDisabled()
                            .font(.callout.monospaced())
                        let dirs = store.directories(on: machine)
                        if !dirs.isEmpty {
                            Menu {
                                ForEach(dirs, id: \.self) { d in Button(d) { cwd = d } }
                            } label: {
                                Image(systemName: "folder.badge.gearshape")
                            }
                            .accessibilityLabel("Recent folders")
                        }
                    }
                }

                Section("What") {
                    Picker("Agent", selection: $agent) {
                        ForEach(Agent.allCases) { Text($0.rawValue).tag($0) }
                    }
                    .pickerStyle(.segmented)
                    if agent == .custom {
                        TextField("Command, e.g. aider --model sonnet", text: $customCommand)
                            .textInputAutocapitalization(.never)
                            .autocorrectionDisabled()
                            .font(.callout.monospaced())
                    }
                    TextField("Name (optional)", text: $name)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                    if !Self.isValidName(name) {
                        Text("Use 1–32 of a–z, 0–9, - _ .").font(.caption).foregroundStyle(.red)
                    }
                }

                if agent != .shell {
                    Section {
                        TextField("First task (optional)", text: $task, axis: .vertical)
                            .lineLimit(3...8)
                    } footer: {
                        Text("Sent once the agent is ready. If it asks something first (like trusting the folder), you'll get a notification and the task goes in after you answer.")
                    }
                }

                if let error {
                    Section { Label(error, systemImage: "exclamationmark.triangle").foregroundStyle(.red) }
                }
            }
            .navigationTitle("New agent")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("Cancel") { dismiss() } }
                ToolbarItem(placement: .confirmationAction) {
                    Button(action: start) {
                        if starting { ProgressView() } else { Text("Start") }
                    }
                    .disabled(starting || !Self.isValidName(name) || (agent == .custom && command.isEmpty))
                }
            }
            .onChange(of: machine) {
                spaceID = ""
                tabID = ""
                cwd = store.directories(on: machine).first ?? ""
            }
            .onChange(of: spaceID) {
                tabID = ""
                if let c = space?.cwd { cwd = c }
            }
            .onAppear {
                if cwd.isEmpty { cwd = store.directories(on: machine).first ?? "" }
                #if DEBUG
                if DemoData.isOn {
                    name = "orders-pagination"
                    task = "Add cursor pagination to GET /v2/orders and update the API docs."
                }
                #endif
            }
        }
    }

    private func start() {
        guard let client = store.client else { return }
        starting = true
        error = nil
        let req = StartRequest(on: machine, cwd: cwd.isEmpty ? nil : cwd, command: command,
                               task: agent == .shell ? "" : task.trimmingCharacters(in: .whitespacesAndNewlines),
                               name: name.isEmpty ? nil : name, space: spaceID.isEmpty ? nil : spaceID,
                               tab: tabID.isEmpty ? nil : tabID)
        Task {
            defer { starting = false }
            do {
                let result = try await client.start(req)
                dismiss()
                AppRouter.shared.pendingAddress = result.address
            } catch {
                self.error = error.localizedDescription
            }
        }
    }
}

#Preview { NewAgentView().environment(PreviewData.store) }
