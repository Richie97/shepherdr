import SwiftUI

struct SettingsView: View {
    @Environment(HerdrStore.self) private var store
    @Environment(\.dismiss) private var dismiss
    @AppStorage("hubURL") private var hubURL = ""
    @State private var testResult: String?
    @State private var testing = false
    private var push: PushManager { .shared }

    var body: some View {
        NavigationStack {
            Form {
                Section("Hub") {
                    LabeledContent("Address", value: hubURL)
                    LabeledContent("Status", value: statusText)
                    if let core = store.health?.core {
                        LabeledContent("Core", value: "\(core.core.name) · hc \(core.version)")
                    }
                    if let v = store.health?.bridgeVersion { LabeledContent("Bridge", value: v) }
                }
                Section("Machines") {
                    LabeledContent(store.hubName, value: "hub")
                    ForEach(store.machines) { m in
                        LabeledContent(m.name) {
                            Text(m.state).foregroundStyle(m.state == "connected" ? .green : .orange)
                        }
                    }
                }
                Section {
                    LabeledContent("Permission", value: permissionText)
                    LabeledContent("This device", value: push.registeredHub != nil ? "Registered" : (push.deviceToken == nil ? "No token yet" : "Not registered"))
                    LabeledContent("Hub can send", value: store.health?.pushEnabled == true ? "Yes" : "No APNs key")
                    Button {
                        guard let url = URL(string: hubURL) else { return }
                        testing = true
                        Task {
                            testResult = await push.sendTest(via: url)
                            testing = false
                        }
                    } label: {
                        if testing { ProgressView() } else { Text("Send test notification") }
                    }
                    .disabled(testing)
                    if let testResult { Text(testResult).font(.caption).foregroundStyle(.secondary) }
                    if let err = push.lastError { Text(err).font(.caption).foregroundStyle(.red) }
                } header: {
                    Text("Notifications")
                } footer: {
                    Text("The hub notifies you when an agent needs you, finishes, or hits an error.")
                }
                Section {
                    Button("Disconnect and forget hub", role: .destructive) {
                        store.disconnect()
                        hubURL = ""
                        dismiss()
                    }
                }
            }
            .navigationTitle("Settings")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar { ToolbarItem(placement: .confirmationAction) { Button("Done") { dismiss() } } }
        }
    }

    private var permissionText: String {
        switch push.authorization {
        case .authorized: "Allowed"
        case .denied: "Off — enable in Settings"
        case .provisional, .ephemeral: "Quiet"
        case .notDetermined: "Not asked"
        @unknown default: "Unknown"
        }
    }

    private var statusText: String {
        switch store.connection {
        case .live: "Connected"
        case .connecting: "Connecting…"
        case .disconnected: "Disconnected"
        case .failed(let m): "Error: \(m)"
        }
    }
}
