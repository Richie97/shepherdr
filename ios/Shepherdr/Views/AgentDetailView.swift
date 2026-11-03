import SwiftUI

/// One terminal: its screen, a prompt composer and quick keys.
struct AgentDetailView: View {
    @Environment(HerdrStore.self) private var store
    let address: String

    @State private var screen: ReadResult?
    @State private var draft = ""
    @State private var busy = false
    @State private var notice: Notice?
    @State private var confirmForce = false
    @State private var showLive = false
    @FocusState private var composerFocused: Bool

    private var terminal: Terminal? { store.terminals[address] }
    private var revision: Int { store.revisions[address] ?? 0 }

    var body: some View {
        VStack(spacing: 0) {
            if let t = terminal { StatusHeader(terminal: t) }
            output
            controls
        }
        .navigationTitle(terminal?.displayName ?? address)
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Button { showLive = true } label: {
                    Label("Live terminal", systemImage: "terminal")
                }
                .disabled(terminal?.isLive == false)
            }
            ToolbarItem(placement: .topBarTrailing) {
                Button(role: .destructive) { run("Stop") { try await interrupt() } } label: {
                    Label("Stop", systemImage: "stop.circle")
                }
                .disabled(busy || terminal?.isLive == false)
            }
        }
        .navigationDestination(isPresented: $showLive) { LiveTerminalView(address: address) }
        #if DEBUG
        .onAppear { if UserDefaults.standard.bool(forKey: "openLive") { showLive = true } }
        #endif
        .task(id: revision) { await reload() }
        .task(id: terminal?.agentState) {
            // Screen output isn't an event; poll while the agent works.
            while terminal?.agentState == .working, !Task.isCancelled {
                try? await Task.sleep(for: .seconds(2))
                await reload()
            }
        }
        .confirmationDialog("The agent is waiting at a dialog", isPresented: $confirmForce, titleVisibility: .visible) {
            Button("Send anyway") { send(force: true) }
        } message: {
            Text("Typing now may answer its question. Use the keys bar to answer it instead.")
        }
    }

    // MARK: Output

    private var output: some View {
        ScrollViewReader { proxy in
            ScrollView([.vertical]) {
                VStack(alignment: .leading, spacing: 0) {
                    if let screen {
                        Text(screen.text.trimmingTrailingBlankLines)
                            .font(.system(size: 11, design: .monospaced))
                            .textSelection(.enabled)
                            .frame(maxWidth: .infinity, alignment: .leading)
                    } else {
                        ProgressView().frame(maxWidth: .infinity, minHeight: 200)
                    }
                    Color.clear.frame(height: 1).id("bottom")
                }
                .padding(10)
            }
            .background(Color(.secondarySystemBackground))
            .defaultScrollAnchor(.bottom)
            .onChange(of: screen?.text) { proxy.scrollTo("bottom", anchor: .bottom) }
            .overlay(alignment: .top) {
                if let notice {
                    NoticeView(notice: notice).padding(8).transition(.move(edge: .top).combined(with: .opacity))
                }
            }
        }
    }

    // MARK: Controls

    private var controls: some View {
        VStack(spacing: 8) {
            QuickKeysBar(disabled: busy || terminal?.isLive == false) { keys, label in
                run(label) { _ = try await client().keys(address, keys) }
            }
            HStack(alignment: .bottom, spacing: 8) {
                TextField("Message \(terminal?.displayName ?? "agent")", text: $draft, axis: .vertical)
                    .lineLimit(1...6)
                    .focused($composerFocused)
                    .padding(.horizontal, 12)
                    .padding(.vertical, 8)
                    .background(Color(.secondarySystemBackground), in: RoundedRectangle(cornerRadius: 18))
                Button { send(force: false) } label: {
                    Image(systemName: busy ? "ellipsis.circle.fill" : "arrow.up.circle.fill")
                        .font(.system(size: 32))
                }
                .disabled(busy || terminal?.isLive == false)
                .accessibilityLabel(draftText.isEmpty ? "Accept suggestion" : "Send")
            }
        }
        .padding(.horizontal)
        .padding(.vertical, 8)
        // Past the home indicator and the trailing edge only: on all edges, the
        // bar would spread sideways under the sidebar and span the whole
        // unfolded screen. The leading edge borders the sidebar.
        .background(.bar, ignoresSafeAreaEdges: [.bottom, .trailing])
    }

    // MARK: Actions

    private func client() throws -> BridgeClient {
        guard let c = store.client else { throw BridgeError(kind: "offline", message: "Not connected to the hub") }
        return c
    }

    private func reload() async {
        #if DEBUG
        if DemoData.isOn { screen = DemoData.screen(for: address); return }
        #endif
        guard let c = store.client else { return }
        if let r = try? await c.read(address) { screen = r }
    }

    private var draftText: String { draft.trimmingCharacters(in: .whitespacesAndNewlines) }

    /// Sends the draft, or accepts the agent's suggestion if the draft is empty.
    private func send(force: Bool) {
        let text = draftText
        if terminal?.agentState == .blocked, !force {
            confirmForce = true
            return
        }
        if text.isEmpty {
            run("Accepted suggestion") { try await acceptSuggestion() }
            return
        }
        run("Sent") {
            _ = try await client().send(address, text: text, force: force)
            draft = ""
        }
    }

    /// Tab fills in the suggestion and Enter submits it. Sent one at a time with
    /// a pause: in one call they can arrive 1 ms apart, before the agent's UI has
    /// applied the Tab.
    private func acceptSuggestion() async throws {
        let c = try client()
        _ = try await c.keys(address, ["tab"])
        try await Task.sleep(for: .milliseconds(300))
        _ = try await c.keys(address, ["enter"])
    }

    private func interrupt() async throws {
        let r = try await client().interrupt(address)
        show(.init(text: "Stop: \(r.outcome.replacingOccurrences(of: "_", with: " "))", isError: r.outcome == "still_working"))
    }

    /// Runs an action with a busy state and reports success or failure.
    private func run(_ label: String, _ action: @escaping @MainActor () async throws -> Void) {
        busy = true
        Task {
            defer { busy = false }
            do {
                try await action()
                if label != "Stop" { show(.init(text: label, isError: false)) }
                await reload()
            } catch {
                show(.init(text: error.localizedDescription, isError: true))
            }
        }
    }

    private func show(_ n: Notice) {
        withAnimation { notice = n }
        Task {
            try? await Task.sleep(for: .seconds(n.isError ? 4 : 1.5))
            withAnimation { if notice == n { notice = nil } }
        }
    }
}

struct Notice: Equatable {
    let id = UUID()
    var text: String
    var isError: Bool
}

private struct NoticeView: View {
    let notice: Notice
    var body: some View {
        Label(notice.text, systemImage: notice.isError ? "exclamationmark.triangle.fill" : "checkmark")
            .font(.subheadline)
            .padding(.horizontal, 12)
            .padding(.vertical, 6)
            .background(.regularMaterial, in: Capsule())
            .foregroundStyle(notice.isError ? .red : .primary)
    }
}

/// State, machine and location; prominent when the agent needs you.
private struct StatusHeader: View {
    @Environment(HerdrStore.self) private var store
    let terminal: Terminal

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack(spacing: 6) {
                if let s = terminal.agentState {
                    Label(s.title, systemImage: s.symbol).foregroundStyle(s.color).font(.subheadline.weight(.semibold))
                } else if !terminal.isLive {
                    Label("Finished", systemImage: "stop.fill").foregroundStyle(.secondary).font(.subheadline)
                }
                if let kind = terminal.status?.kind { Text(kind).font(.caption).foregroundStyle(.secondary) }
                if let p = terminal.status?.progress { Text("\(p)%").font(.caption.monospacedDigit()) }
                Spacer()
                MachineBadge(name: store.machineLabel(for: terminal))
            }
            if terminal.agentState == .blocked || terminal.agentState == .error, let line = terminal.statusLine {
                Text(line).font(.callout)
            }
            if let dir = terminal.currentDirectory {
                Text(dir).font(.caption.monospaced()).foregroundStyle(.secondary).lineLimit(1).truncationMode(.head)
            }
        }
        .padding(.horizontal)
        .padding(.vertical, 8)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(terminal.agentState == .blocked ? Color.red.opacity(0.12) : Color.clear)
    }
}

/// Keys for answering dialogs and steering an agent without the keyboard.
struct QuickKeysBar: View {
    var disabled: Bool
    var press: (_ keys: [String], _ label: String) -> Void

    private typealias Key = (label: String, key: String)
    /// Moving around a dialog, then answering it.
    private static let navigation: [Key] = [("esc", "esc"), ("⏎", "enter"), ("↑", "up"), ("↓", "down"), ("tab", "tab")]
    private static let answers: [Key] = [("1", "1"), ("2", "2"), ("3", "3"), ("y", "y"), ("n", "n"), ("^C", "ctrl+c")]

    var body: some View {
        // Full size if one row fits, smaller buttons if not (narrow detail
        // columns), two rows on the narrowest screens (the folded iPhone Duo's
        // outer screen), and scrolling as a last resort.
        ViewThatFits(in: .horizontal) {
            row(Self.navigation + Self.answers, spacing: 6)
            row(Self.navigation + Self.answers, spacing: 4).controlSize(.small)
            VStack(alignment: .leading, spacing: 4) {
                row(Self.navigation, spacing: 4)
                row(Self.answers, spacing: 4)
            }
            .controlSize(.small)
            ScrollView(.horizontal, showsIndicators: false) {
                row(Self.navigation + Self.answers, spacing: 4).controlSize(.small)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .disabled(disabled)
    }

    private func row(_ keys: [Key], spacing: CGFloat) -> some View {
        HStack(spacing: spacing) {
            ForEach(keys, id: \.key) { k in
                Button(k.label) { press([k.key], "Pressed \(k.label)") }
                    .font(.system(.callout, design: .monospaced))
                    .buttonStyle(.bordered)
                    .buttonBorderShape(.roundedRectangle(radius: 8))
                    .tint(k.key == "ctrl+c" ? .red : .primary)
            }
        }
        .fixedSize(horizontal: true, vertical: false)
    }
}

extension String {
    var trimmingTrailingBlankLines: String {
        var lines = split(separator: "\n", omittingEmptySubsequences: false)
        while let last = lines.last, last.allSatisfy(\.isWhitespace) { lines.removeLast() }
        return lines.joined(separator: "\n")
    }
}

#Preview {
    NavigationStack { AgentDetailView(address: "t1") }.environment(PreviewData.store)
}
