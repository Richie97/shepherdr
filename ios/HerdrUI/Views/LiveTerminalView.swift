import SwiftTerm
import SwiftUI
import UIKit

/// A real terminal attached to an agent through the bridge's /v1/attach:
/// colors, full-screen UIs and typing, like `hc attach`.
struct LiveTerminalView: View {
    @Environment(HerdrStore.self) private var store
    let address: String

    @AppStorage("terminalFontSize") private var fontSize = 11.0
    @AppStorage("terminalWatchOnly") private var watchOnly = false
    @State private var session = LiveTerminalSession()

    var body: some View {
        TerminalHost(session: session, fontSize: fontSize)
            .ignoresSafeArea(.keyboard, edges: [])
            .background(Color.black)
            .overlay(alignment: .top) { banner }
            .navigationTitle(store.terminals[address]?.displayName ?? address)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItemGroup(placement: .topBarTrailing) {
                    Button { fontSize = max(7, fontSize - 1) } label: { Image(systemName: "textformat.size.smaller") }
                        .accessibilityLabel("Smaller text")
                    Button { fontSize = min(20, fontSize + 1) } label: { Image(systemName: "textformat.size.larger") }
                        .accessibilityLabel("Larger text")
                    Menu {
                        Toggle("Watch only", isOn: $watchOnly)
                        Button("Reconnect", action: connect)
                    } label: {
                        Image(systemName: "ellipsis.circle")
                    }
                }
            }
            .onAppear(perform: connect)
            .onDisappear { session.disconnect() }
            .onChange(of: watchOnly) { connect() }
    }

    @ViewBuilder private var banner: some View {
        if let t = store.terminals[address], !store.isReachable(t), let m = t.machine {
            VStack(spacing: 8) {
                Label("\(m) is unavailable", systemImage: "bolt.horizontal.circle")
                Text(MachineDownRow.explain(store.machineState(m) ?? "")).font(.caption2).multilineTextAlignment(.center)
                Button("Reconnect") {
                    Task { try? await store.reconnect(m); connect() }
                }
                .buttonStyle(.borderedProminent)
            }
            .font(.callout)
            .padding()
            .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 14))
            .padding()
        } else {
            sessionBanner
        }
    }

    @ViewBuilder private var sessionBanner: some View {
        switch session.state {
        case .live where watchOnly:
            Label("Watching", systemImage: "eye").bannerStyle()
        case .connecting:
            Label("Attaching…", systemImage: "antenna.radiowaves.left.and.right").bannerStyle()
        case .ended(let reason):
            Label(reason, systemImage: "bolt.horizontal.circle").bannerStyle()
        default:
            EmptyView()
        }
    }

    private func connect() {
        guard let client = store.client else { return }
        session.connect(client: client, address: address, observe: watchOnly)
    }
}

private extension View {
    func bannerStyle() -> some View {
        font(.caption)
            .padding(.horizontal, 10)
            .padding(.vertical, 5)
            .background(.regularMaterial, in: Capsule())
            .padding(.top, 6)
    }
}

/// Hosts SwiftTerm's UIKit view.
private struct TerminalHost: UIViewRepresentable {
    let session: LiveTerminalSession
    let fontSize: Double

    func makeUIView(context: Context) -> TerminalView {
        let view = session.terminalView
        view.font = UIFont.monospacedSystemFont(ofSize: fontSize, weight: .regular)
        return view
    }

    func updateUIView(_ view: TerminalView, context: Context) {
        if view.font.pointSize != fontSize {
            view.font = UIFont.monospacedSystemFont(ofSize: fontSize, weight: .regular)
        }
    }
}

/// Owns the terminal view and the WebSocket that feeds it.
@MainActor
@Observable
final class LiveTerminalSession: NSObject {
    enum State: Equatable {
        case idle, connecting, live
        case ended(String)
    }

    private(set) var state: State = .idle
    @ObservationIgnored let terminalView: TerminalView
    @ObservationIgnored private var socket: URLSessionWebSocketTask?
    @ObservationIgnored private var receiveTask: Task<Void, Never>?
    @ObservationIgnored private var observe = false
    /// The view's size in cells, from SwiftTerm's sizeChanged.
    @ObservationIgnored private var cols = 80
    @ObservationIgnored private var rows = 24

    override init() {
        terminalView = TerminalView(frame: .zero)
        super.init()
        terminalView.terminalDelegate = self
        terminalView.nativeBackgroundColor = .black
        terminalView.nativeForegroundColor = UIColor(white: 0.9, alpha: 1)
        terminalView.optionAsMetaKey = true
    }

    func connect(client: BridgeClient, address: String, observe: Bool) {
        disconnect()
        self.observe = observe
        state = .connecting
        terminalView.resetToInitialState()

        var c = URLComponents(url: client.baseURL.appending(path: "v1/attach"), resolvingAgainstBaseURL: false)!
        c.scheme = c.scheme == "http" ? "ws" : "wss"
        var items: [URLQueryItem] = [
            URLQueryItem(name: "terminal", value: address),
            URLQueryItem(name: "cols", value: String(max(cols, 20))),
            URLQueryItem(name: "rows", value: String(max(rows, 8))),
            URLQueryItem(name: "history", value: "2000"),
        ]
        if observe { items.append(URLQueryItem(name: "observe", value: "1")) }
        c.queryItems = items
        let ws = client.session.webSocketTask(with: c.url!)
        ws.maximumMessageSize = 16 << 20
        socket = ws
        ws.resume()
        receiveTask = Task { [weak self] in await self?.receive(ws) }
        if !observe { terminalView.becomeFirstResponder() }
    }

    func disconnect() {
        receiveTask?.cancel()
        receiveTask = nil
        socket?.cancel(with: .normalClosure, reason: nil)
        socket = nil
        if state != .idle { state = .idle }
    }

    private func receive(_ ws: URLSessionWebSocketTask) async {
        do {
            while !Task.isCancelled {
                switch try await ws.receive() {
                case .data(let d):
                    terminalView.feed(byteArray: ArraySlice(d))
                case .string(let s):
                    if s.contains(#""attached""#) {
                        state = .live
                        // The view's real size is known only after layout,
                        // which may come after the attach URL was built.
                        if !observe { sendControl(#"{"type":"resize","cols":\#(cols),"rows":\#(rows)}"#) }
                    }
                @unknown default:
                    break
                }
            }
        } catch {
            guard !Task.isCancelled, socket === ws else { return }
            let reason = ws.closeReason.flatMap { String(data: $0, encoding: .utf8) }.flatMap { $0.isEmpty ? nil : $0 }
            state = .ended(reason ?? "Disconnected")
        }
    }

    private func sendControl(_ json: String) {
        socket?.send(.string(json)) { _ in }
    }
}

extension LiveTerminalSession: TerminalViewDelegate {
    func send(source: TerminalView, data: ArraySlice<UInt8>) {
        guard !observe, state == .live else { return }
        socket?.send(.data(Data(data))) { _ in }
    }

    func sizeChanged(source: TerminalView, newCols: Int, newRows: Int) {
        guard newCols > 0, newRows > 0 else { return }
        cols = newCols
        rows = newRows
        guard !observe, state == .live else { return }
        sendControl(#"{"type":"resize","cols":\#(newCols),"rows":\#(newRows)}"#)
    }

    func setTerminalTitle(source: TerminalView, title: String) {}
    func hostCurrentDirectoryUpdate(source: TerminalView, directory: String?) {}
    func scrolled(source: TerminalView, position: Double) {}
    func requestOpenLink(source: TerminalView, link: String, params: [String: String]) {
        if let url = URL(string: link) { UIApplication.shared.open(url) }
    }
    func rangeChanged(source: TerminalView, startY: Int, endY: Int) {}
}
