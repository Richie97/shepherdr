// Frames raw 6.9" simulator screenshots (1320x2868) into App Store images
// with a caption, plus a lock-screen notification frame.
//
//   swift appstore/make-screenshots.swift <raw-dir> <icon.png> <out-dir>
//
// <raw-dir> holds 1-agents.png, 2-blocked.png, 3-live.png, 4-new.png, taken
// in demo mode (see appstore/README.md).
import AppKit

let args = CommandLine.arguments
let raw = args[1], iconPath = args[2], outDir = args[3]
let W = 1320, H = 2868

func color(_ h: UInt32, _ a: CGFloat = 1) -> NSColor {
    NSColor(srgbRed: CGFloat((h >> 16) & 255) / 255, green: CGFloat((h >> 8) & 255) / 255, blue: CGFloat(h & 255) / 255, alpha: a)
}
func rounded(_ size: CGFloat, _ weight: NSFont.Weight) -> NSFont {
    let f = NSFont.systemFont(ofSize: size, weight: weight)
    return NSFont(descriptor: f.fontDescriptor.withDesign(.rounded) ?? f.fontDescriptor, size: size) ?? f
}
func draw(_ text: String, font: NSFont, color c: NSColor, in rect: NSRect, align: NSTextAlignment = .center) {
    let p = NSMutableParagraphStyle(); p.alignment = align; p.lineSpacing = 4
    (text as NSString).draw(with: rect, options: [.usesLineFragmentOrigin], attributes: [.font: font, .foregroundColor: c, .paragraphStyle: p])
}

/// Renders one 1320x2868 frame; drawing happens in a top-left coordinate space.
func frame(_ name: String, gradient: [UInt32], body: (CGContext) -> Void) {
    let rep = NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: W, pixelsHigh: H, bitsPerSample: 8, samplesPerPixel: 4,
                               hasAlpha: true, isPlanar: false, colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0)!
    rep.size = NSSize(width: W, height: H)
    let g = NSGraphicsContext(bitmapImageRep: rep)!
    NSGraphicsContext.current = NSGraphicsContext(cgContext: g.cgContext, flipped: true)
    let ctx = g.cgContext
    ctx.translateBy(x: 0, y: CGFloat(H)); ctx.scaleBy(x: 1, y: -1)
    let grad = NSGradient(colors: gradient.map { color($0) })!
    grad.draw(in: NSRect(x: 0, y: 0, width: W, height: H), angle: 90) // first color at the top
    body(ctx)
    NSGraphicsContext.current = nil
    // App Store images must not have an alpha channel: flatten to RGB.
    let cg = rep.cgImage!
    let flat = CGContext(data: nil, width: W, height: H, bitsPerComponent: 8, bytesPerRow: 0,
                         space: CGColorSpace(name: CGColorSpace.sRGB)!, bitmapInfo: CGImageAlphaInfo.noneSkipLast.rawValue)!
    flat.draw(cg, in: CGRect(x: 0, y: 0, width: W, height: H))
    let png = NSBitmapImageRep(cgImage: flat.makeImage()!).representation(using: .png, properties: [:])!
    try! png.write(to: URL(fileURLWithPath: "\(outDir)/\(name).png"))
    print("wrote \(name).png")
}

func caption(_ title: String, _ sub: String) {
    draw(title, font: rounded(96, .heavy), color: .white, in: NSRect(x: 80, y: 190, width: W - 160, height: 240))
    draw(sub, font: rounded(50, .medium), color: color(0xFFFFFF, 0.82), in: NSRect(x: 100, y: 440, width: W - 200, height: 140))
}

/// The screenshot as a phone: rounded, outlined, shadowed, bleeding off the bottom.
func phone(_ file: String, top: CGFloat = 640, width: CGFloat = 1060) {
    let img = NSImage(contentsOfFile: "\(raw)/\(file)")!
    let h = width * CGFloat(H) / CGFloat(W)
    let r = NSRect(x: (CGFloat(W) - width) / 2, y: top, width: width, height: h)
    let path = NSBezierPath(roundedRect: r, xRadius: 92, yRadius: 92)
    NSGraphicsContext.saveGraphicsState()
    let shadow = NSShadow(); shadow.shadowColor = color(0x000000, 0.45); shadow.shadowBlurRadius = 60; shadow.shadowOffset = NSSize(width: 0, height: -20)
    shadow.set(); color(0x000000).setFill(); path.fill()
    NSGraphicsContext.restoreGraphicsState()
    NSGraphicsContext.saveGraphicsState(); path.addClip()
    img.draw(in: r, from: .zero, operation: .sourceOver, fraction: 1, respectFlipped: true, hints: nil)
    NSGraphicsContext.restoreGraphicsState()
    color(0xFFFFFF, 0.18).setStroke(); path.lineWidth = 6; path.stroke()
}

let sunset: [UInt32] = [0x2B1E6B, 0x7A3FA8, 0xF2778C]
let dusk: [UInt32] = [0x1E1A4F, 0x4B2E91, 0x9C4BB0]
let dawn: [UInt32] = [0x3A1F6E, 0xC4527F, 0xFFB27A]

frame("01-agents", gradient: sunset) { _ in
    caption("Every agent.\nEvery machine.", "See at a glance which agents need you.")
    phone("1-agents.png")
}

frame("02-notifications", gradient: dawn) { _ in
    caption("Never miss a\nblocked agent", "Approve or deny from the lock screen.")
    // Lock screen.
    draw("9:41", font: rounded(210, .semibold), color: .white, in: NSRect(x: 0, y: 720, width: W, height: 260))
    draw("Sunday, November 1", font: rounded(52, .medium), color: color(0xFFFFFF, 0.85), in: NSRect(x: 0, y: 660, width: W, height: 70))
    let icon = NSImage(contentsOfFile: iconPath)!
    func card(y: CGFloat, title: String, sub: String, body: String, ago: String, height: CGFloat = 250) -> NSRect {
        let r = NSRect(x: 90, y: y, width: CGFloat(W) - 180, height: height)
        let p = NSBezierPath(roundedRect: r, xRadius: 56, yRadius: 56)
        color(0xFFFFFF, 0.22).setFill(); p.fill()
        let ir = NSRect(x: r.minX + 40, y: r.minY + 44, width: 110, height: 110)
        NSGraphicsContext.saveGraphicsState(); NSBezierPath(roundedRect: ir, xRadius: 26, yRadius: 26).addClip()
        icon.draw(in: ir); NSGraphicsContext.restoreGraphicsState()
        draw(title, font: rounded(44, .bold), color: .white, in: NSRect(x: ir.maxX + 34, y: r.minY + 36, width: r.width - 380, height: 60), align: .left)
        draw(ago, font: rounded(38, .regular), color: color(0xFFFFFF, 0.7), in: NSRect(x: r.maxX - 200, y: r.minY + 40, width: 160, height: 56), align: .right)
        draw(sub, font: rounded(38, .medium), color: color(0xFFFFFF, 0.8), in: NSRect(x: ir.maxX + 34, y: r.minY + 96, width: r.width - 240, height: 54), align: .left)
        draw(body, font: rounded(42, .regular), color: .white, in: NSRect(x: ir.maxX + 34, y: r.minY + 150, width: r.width - 240, height: 60), align: .left)
        return r
    }
    let c = card(y: 1130, title: "claude-code needs you", sub: "laptop · storefront / 1", body: "Run npm test -- checkout?", ago: "now")
    // Expanded actions, as after a long press.
    let actions = NSRect(x: c.minX, y: c.maxY + 30, width: c.width, height: 260)
    let ap = NSBezierPath(roundedRect: actions, xRadius: 48, yRadius: 48)
    color(0xFFFFFF, 0.9).setFill(); ap.fill()
    color(0x000000, 0.12).setFill(); NSRect(x: actions.minX, y: actions.midY - 1.5, width: actions.width, height: 3).fill()
    draw("Approve", font: rounded(52, .semibold), color: color(0x1F7A3A), in: NSRect(x: actions.minX + 70, y: actions.minY + 30, width: 600, height: 80), align: .left)
    draw("✓", font: rounded(56, .bold), color: color(0x1F7A3A), in: NSRect(x: actions.maxX - 150, y: actions.minY + 26, width: 80, height: 80))
    draw("Deny", font: rounded(52, .semibold), color: color(0xD33A3A), in: NSRect(x: actions.minX + 70, y: actions.midY + 30, width: 600, height: 80), align: .left)
    draw("✕", font: rounded(52, .bold), color: color(0xD33A3A), in: NSRect(x: actions.maxX - 150, y: actions.midY + 28, width: 80, height: 80))
    _ = card(y: actions.maxY + 70, title: "codex finished", sub: "studio · api / 1", body: "Add rate limiting to /v2/orders", ago: "4m")
    _ = card(y: actions.maxY + 360, title: "claude-code finished", sub: "laptop · docs / 1", body: "Write the v3 migration guide", ago: "12m")
}

frame("03-blocked", gradient: sunset) { _ in
    caption("Unblock agents\nfrom anywhere", "Read the screen. Answer with one tap.")
    phone("2-blocked.png")
}

frame("04-live-terminal", gradient: dusk) { _ in
    caption("A real terminal\nin your pocket", "Full color, live, keyboard included.")
    phone("3-live.png")
}

frame("05-new-agent", gradient: dawn) { _ in
    caption("Start agents\non the go", "Pick a machine, a folder and a task.")
    phone("4-new.png")
}
