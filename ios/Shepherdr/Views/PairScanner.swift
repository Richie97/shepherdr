import SwiftUI
import VisionKit

/// Pairing links: `shepherdr://pair?hub=<url>` (from `herdr-bridge pair` or the
/// hub's /pair page). A bare hub URL or host also works.
enum PairLink {
    static func hub(from string: String) -> URL? {
        let s = string.trimmingCharacters(in: .whitespacesAndNewlines)
        if let u = URL(string: s), u.scheme == "shepherdr" {
            guard u.host() == "pair",
                  let hub = URLComponents(url: u, resolvingAgainstBaseURL: false)?
                      .queryItems?.first(where: { $0.name == "hub" })?.value
            else { return nil }
            return ConnectView.normalize(hub)
        }
        return ConnectView.normalize(s)
    }
}

/// Scans a pairing QR code with the camera.
struct PairScanner: View {
    var onHub: (URL) -> Void
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        NavigationStack {
            Group {
                if DataScannerViewController.isSupported && DataScannerViewController.isAvailable {
                    ScannerView { payload in
                        if let hub = PairLink.hub(from: payload) {
                            onHub(hub)
                            dismiss()
                        }
                    }
                    .ignoresSafeArea()
                } else {
                    ContentUnavailableView("Camera unavailable", systemImage: "camera",
                                           description: Text("Allow camera access in Settings, or type the hub address instead."))
                }
            }
            .navigationTitle("Scan pairing code")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar { ToolbarItem(placement: .cancellationAction) { Button("Cancel") { dismiss() } } }
        }
    }
}

private struct ScannerView: UIViewControllerRepresentable {
    var onPayload: (String) -> Void

    func makeUIViewController(context: Context) -> DataScannerViewController {
        let vc = DataScannerViewController(recognizedDataTypes: [.barcode(symbologies: [.qr])],
                                           qualityLevel: .balanced, isHighlightingEnabled: true)
        vc.delegate = context.coordinator
        try? vc.startScanning()
        return vc
    }

    func updateUIViewController(_ vc: DataScannerViewController, context: Context) {}

    func makeCoordinator() -> Coordinator { Coordinator(onPayload: onPayload) }

    final class Coordinator: NSObject, DataScannerViewControllerDelegate {
        let onPayload: (String) -> Void
        init(onPayload: @escaping (String) -> Void) { self.onPayload = onPayload }

        func dataScanner(_ scanner: DataScannerViewController, didAdd items: [RecognizedItem], allItems: [RecognizedItem]) {
            for case .barcode(let code) in items {
                if let s = code.payloadStringValue { onPayload(s) }
            }
        }
    }
}
