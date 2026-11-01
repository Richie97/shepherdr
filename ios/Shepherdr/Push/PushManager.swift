import Foundation
import Observation
import UIKit
import UserNotifications

/// Opens agents from notification taps and links.
@MainActor
@Observable
final class AppRouter {
    static let shared = AppRouter()
    /// Set to an address to open it; AgentsView consumes it.
    var pendingAddress: String?
    /// A hub from a pairing link, waiting for the user to confirm.
    var pendingHub: URL?

    /// Routes shepherdr:// links: pair?hub=… and terminal/<address>.
    func open(_ url: URL) {
        guard url.scheme == "shepherdr" else { return }
        switch url.host() {
        case "pair":
            pendingHub = PairLink.hub(from: url.absoluteString)
        case "terminal":
            pendingAddress = AgentsView.address(from: url)
        default:
            break
        }
    }
}

/// Notification permission, the APNs device token, registering it with the
/// hub, and the actions on agent notifications.
@MainActor
@Observable
final class PushManager: NSObject {
    static let shared = PushManager()

    enum Category: String { case blocked = "BLOCKED", done = "DONE", error = "ERROR" }
    enum Action: String { case approve = "APPROVE", deny = "DENY" }

    private(set) var authorization: UNAuthorizationStatus = .notDetermined
    private(set) var deviceToken: String?
    private(set) var registeredHub: URL?
    private(set) var lastError: String?

    /// Development builds get tokens for the APNs sandbox.
    static var isSandbox: Bool {
        #if DEBUG
        true
        #else
        false
        #endif
    }

    func configure() {
        let center = UNUserNotificationCenter.current()
        center.delegate = self
        let approve = UNNotificationAction(identifier: Action.approve.rawValue, title: "Approve",
                                           options: [.authenticationRequired],
                                           icon: UNNotificationActionIcon(systemImageName: "checkmark"))
        let deny = UNNotificationAction(identifier: Action.deny.rawValue, title: "Deny",
                                        options: [.authenticationRequired, .destructive],
                                        icon: UNNotificationActionIcon(systemImageName: "xmark"))
        center.setNotificationCategories([
            UNNotificationCategory(identifier: Category.blocked.rawValue, actions: [approve, deny], intentIdentifiers: []),
            UNNotificationCategory(identifier: Category.done.rawValue, actions: [], intentIdentifiers: []),
            UNNotificationCategory(identifier: Category.error.rawValue, actions: [], intentIdentifiers: []),
        ])
    }

    /// Asks for permission (once) and registers with APNs.
    func requestAndRegister() async {
        let center = UNUserNotificationCenter.current()
        do {
            _ = try await center.requestAuthorization(options: [.alert, .sound, .badge])
        } catch {
            lastError = error.localizedDescription
        }
        authorization = await center.notificationSettings().authorizationStatus
        // Tokens don't need permission; register regardless so the hub
        // knows this device once alerts are allowed.
        UIApplication.shared.registerForRemoteNotifications()
    }

    func didRegister(token: Data) {
        deviceToken = token.map { String(format: "%02x", $0) }.joined()
        if let hub = currentHub { Task { await register(with: hub) } }
    }

    func didFailToRegister(_ error: Error) {
        lastError = "APNs: \(error.localizedDescription)"
    }

    private var currentHub: URL? {
        guard let s = UserDefaults.standard.string(forKey: "hubURL"), !s.isEmpty else { return nil }
        return URL(string: s)
    }

    /// Tells the hub's bridge where to send notifications. Safe to repeat.
    func register(with hub: URL) async {
        guard let token = deviceToken else { return }
        struct Body: Encodable { var token: String; var sandbox: Bool; var name: String }
        var req = URLRequest(url: hub.appending(path: "v1/push/register"))
        req.httpMethod = "POST"
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        req.httpBody = try? JSONEncoder().encode(Body(token: token, sandbox: Self.isSandbox, name: UIDevice.current.name))
        do {
            let (_, resp) = try await URLSession.shared.data(for: req)
            if (resp as? HTTPURLResponse)?.statusCode == 200 {
                registeredHub = hub
                lastError = nil
            } else {
                lastError = "Hub refused registration (\((resp as? HTTPURLResponse)?.statusCode ?? 0))"
            }
        } catch {
            lastError = error.localizedDescription
        }
    }

    /// Asks the bridge to send a test notification to every device.
    func sendTest(via hub: URL) async -> String {
        struct Result: Decodable { var sent: Int; var errors: [String] }
        var req = URLRequest(url: hub.appending(path: "v1/push/test"))
        req.httpMethod = "POST"
        do {
            let (data, _) = try await URLSession.shared.data(for: req)
            let r = try JSONDecoder().decode(Result.self, from: data)
            return r.sent > 0 ? "Sent to \(r.sent) device\(r.sent == 1 ? "" : "s")" : (r.errors.first ?? "No devices registered")
        } catch {
            return error.localizedDescription
        }
    }

    // MARK: Actions

    /// Keys that answer "yes" in each agent's permission dialog.
    static func approveKeys(for app: String?) -> [String] {
        switch app {
        case "claude-code": ["1"]
        case "codex": ["y"]
        default: ["enter"]
        }
    }

    static let denyKeys = ["esc"]

    /// Approves or denies from a notification, only if the agent is still
    /// blocked (the dialog may have been answered elsewhere).
    func perform(_ action: Action, address: String, app: String?) async {
        guard let hub = currentHub else { return }
        let client = BridgeClient(baseURL: hub)
        do {
            let t = try await client.get(address)
            guard t.agentState == .blocked else {
                await notifyResult("\(t.displayName) is no longer waiting", body: "Nothing was sent.")
                return
            }
            let keys = action == .approve ? Self.approveKeys(for: t.status?.app ?? app) : Self.denyKeys
            _ = try await client.keys(address, keys)
        } catch {
            await notifyResult("Couldn't \(action == .approve ? "approve" : "deny")", body: error.localizedDescription)
        }
    }

    private func notifyResult(_ title: String, body: String) async {
        let content = UNMutableNotificationContent()
        content.title = title
        content.body = body
        try? await UNUserNotificationCenter.current().add(
            UNNotificationRequest(identifier: UUID().uuidString, content: content, trigger: nil))
    }
}

// Main-actor isolated, completion-handler forms: UserNotifications requires
// its completion handlers to be called on the main thread. The `async`
// forms, when nonisolated, complete on a background thread and crash
// ("Call must be made on main thread").
extension PushManager: @preconcurrency UNUserNotificationCenterDelegate {
    func userNotificationCenter(_ center: UNUserNotificationCenter, willPresent notification: UNNotification,
                                withCompletionHandler completionHandler: @escaping (UNNotificationPresentationOptions) -> Void) {
        completionHandler([.banner, .sound, .list])
    }

    func userNotificationCenter(_ center: UNUserNotificationCenter, didReceive response: UNNotificationResponse,
                                withCompletionHandler completionHandler: @escaping () -> Void) {
        let info = response.notification.request.content.userInfo
        guard let address = info["address"] as? String else {
            completionHandler()
            return
        }
        let app = info["app"] as? String
        if let action = Action(rawValue: response.actionIdentifier) {
            // Finish the action before completing: the app may be suspended
            // right after. The task inherits the main actor, so the handler
            // runs on the main thread.
            Task {
                await perform(action, address: address, app: app)
                completionHandler()
            }
        } else {
            if response.actionIdentifier == UNNotificationDefaultActionIdentifier {
                AppRouter.shared.pendingAddress = address
            }
            completionHandler()
        }
    }
}

/// Receives the APNs token callbacks, which only an app delegate gets.
final class AppDelegate: NSObject, UIApplicationDelegate {
    func application(_ application: UIApplication,
                     didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]? = nil) -> Bool {
        PushManager.shared.configure()
        return true
    }

    func application(_ application: UIApplication, didRegisterForRemoteNotificationsWithDeviceToken deviceToken: Data) {
        PushManager.shared.didRegister(token: deviceToken)
    }

    func application(_ application: UIApplication, didFailToRegisterForRemoteNotificationsWithError error: Error) {
        PushManager.shared.didFailToRegister(error)
    }
}
