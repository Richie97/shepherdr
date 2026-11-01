import SwiftUI

@main
struct HerdrUIApp: App {
    @UIApplicationDelegateAdaptor(AppDelegate.self) private var appDelegate
    @State private var store = HerdrStore()
    @AppStorage("hubURL") private var hubURL = ""
    @Environment(\.scenePhase) private var scenePhase

    private var pairingPrompt: Binding<Bool> {
        Binding(get: { AppRouter.shared.pendingHub != nil },
                set: { if !$0 { AppRouter.shared.pendingHub = nil } })
    }

    private func pair(with hub: URL) {
        AppRouter.shared.pendingHub = nil
        hubURL = hub.absoluteString
        store.connect(to: hub)
    }

    var body: some Scene {
        WindowGroup {
            Group {
                if let url = URL(string: hubURL), !hubURL.isEmpty {
                    AgentsView()
                        .onAppear { if store.client?.baseURL != url { store.connect(to: url) } }
                } else {
                    ConnectView { url in
                        hubURL = url.absoluteString
                        store.connect(to: url)
                    }
                }
            }
            .environment(store)
            .onOpenURL { AppRouter.shared.open($0) }
            .alert("Connect to this hub?", isPresented: pairingPrompt, presenting: AppRouter.shared.pendingHub) { hub in
                Button("Connect") { pair(with: hub) }
                Button("Cancel", role: .cancel) { AppRouter.shared.pendingHub = nil }
            } message: { hub in
                Text(hub.host() ?? hub.absoluteString)
            }
            .task(id: hubURL) {
                // Ask for notifications once a hub is set, and (re)register
                // this device with it.
                guard let url = URL(string: hubURL), !hubURL.isEmpty else { return }
                await PushManager.shared.requestAndRegister()
                await PushManager.shared.register(with: url)
            }
            .onChange(of: scenePhase) { _, phase in
                // iOS drops sockets in the background; reconnect on return.
                guard let url = URL(string: hubURL), !hubURL.isEmpty else { return }
                if phase == .active, store.connection != .live { store.connect(to: url) }
            }
        }
    }
}
