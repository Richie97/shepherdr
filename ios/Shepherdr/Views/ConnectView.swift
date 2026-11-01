import SwiftUI

/// First run: point the app at the hub's bridge.
struct ConnectView: View {
    var onConnect: (URL) -> Void

    @State private var address = ""
    @State private var checking = false
    @State private var error: String?
    @State private var scanning = false

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    TextField("my-hub.your-tailnet.ts.net", text: $address)
                        .keyboardType(.URL)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .onSubmit(check)
                } header: {
                    Text("Hub")
                } footer: {
                    Text("The machine running herdr-bridge, reached over Tailscale.")
                }

                if let error {
                    Section { Label(error, systemImage: "exclamationmark.triangle").foregroundStyle(.red) }
                }

                Section {
                    Button { scanning = true } label: {
                        Label("Scan QR code", systemImage: "qrcode.viewfinder")
                    }
                } footer: {
                    Text("On the hub, run `herdr-bridge pair`, or open https://<hub>:8443/pair in a browser on your tailnet.")
                }

                Section {
                    Button(action: check) {
                        if checking { ProgressView() } else { Text("Connect") }
                    }
                    .disabled(address.isEmpty || checking)
                }
            }
            .navigationTitle("Shepherdr")
            .sheet(isPresented: $scanning) {
                PairScanner { hub in
                    address = hub.host().map { h in hub.port.map { "\(h):\($0)" } ?? h } ?? hub.absoluteString
                    check()
                }
            }
        }
    }

    /// Accepts "host", "host:port" or a full URL; defaults to https and 8443.
    static func normalize(_ input: String) -> URL? {
        var s = input.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !s.isEmpty else { return nil }
        if !s.contains("://") { s = "https://" + s }
        guard var c = URLComponents(string: s), c.host != nil else { return nil }
        if c.port == nil, c.scheme == "https" { c.port = 8443 }
        c.path = ""
        return c.url
    }

    private func check() {
        guard let url = Self.normalize(address) else {
            error = "That doesn't look like an address."
            return
        }
        checking = true
        error = nil
        Task {
            defer { checking = false }
            do {
                let health = try await BridgeClient(baseURL: url).health()
                if let coreError = health.coreError {
                    error = "Bridge is up, but Herdr isn't: \(coreError)"
                } else {
                    onConnect(url)
                }
            } catch {
                self.error = error.localizedDescription
            }
        }
    }
}

#Preview { ConnectView { _ in } }
