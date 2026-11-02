// Renders the iOS 27 App Store "creative assets": the product page header and
// the search results image. Both are RGB PNGs without alpha.
//
//   swift appstore/make-creative.swift <raw-dir> <out-dir>
//
// <raw-dir> holds 1-agents.png, 2-blocked.png and 3-live.png from demo mode
// (see appstore/README.md). The sheep is the app icon's, redrawn as vectors.
import AppKit

let args = CommandLine.arguments
let raw = args[1], outDir = args[2]

func color(_ h: UInt32, _ a: CGFloat = 1) -> NSColor {
    NSColor(srgbRed: CGFloat((h >> 16) & 255) / 255, green: CGFloat((h >> 8) & 255) / 255, blue: CGFloat(h & 255) / 255, alpha: a)
}
func rounded(_ size: CGFloat, _ weight: NSFont.Weight) -> NSFont {
    let f = NSFont.systemFont(ofSize: size, weight: weight)
    return NSFont(descriptor: f.fontDescriptor.withDesign(.rounded) ?? f.fontDescriptor, size: size) ?? f
}
func draw(_ text: String, font: NSFont, color c: NSColor, in rect: NSRect, shadow: Bool = false) {
    let p = NSMutableParagraphStyle(); p.alignment = .center; p.lineHeightMultiple = 0.95
    var attrs: [NSAttributedString.Key: Any] = [.font: font, .foregroundColor: c, .paragraphStyle: p]
    if shadow {
        let s = NSShadow(); s.shadowColor = color(0x1A1240, 0.45); s.shadowBlurRadius = 30; s.shadowOffset = NSSize(width: 0, height: -6)
        attrs[.shadow] = s
    }
    (text as NSString).draw(with: rect, options: [.usesLineFragmentOrigin], attributes: attrs)
}
func circle(_ x: CGFloat, _ y: CGFloat, _ r: CGFloat) -> NSBezierPath {
    NSBezierPath(ovalIn: NSRect(x: x - r, y: y - r, width: r * 2, height: r * 2))
}

/// Renders a W x H image; drawing happens in a top-left coordinate space.
func image(_ name: String, _ W: Int, _ H: Int, body: (CGContext) -> Void) {
    let rep = NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: W, pixelsHigh: H, bitsPerSample: 8, samplesPerPixel: 4,
                               hasAlpha: true, isPlanar: false, colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0)!
    rep.size = NSSize(width: W, height: H)
    let g = NSGraphicsContext(bitmapImageRep: rep)!
    NSGraphicsContext.current = NSGraphicsContext(cgContext: g.cgContext, flipped: true)
    let ctx = g.cgContext
    ctx.translateBy(x: 0, y: CGFloat(H)); ctx.scaleBy(x: 1, y: -1)
    body(ctx)
    NSGraphicsContext.current = nil
    // App Store images must not have an alpha channel: flatten to RGB.
    let flat = CGContext(data: nil, width: W, height: H, bitsPerComponent: 8, bytesPerRow: 0,
                         space: CGColorSpace(name: CGColorSpace.sRGB)!, bitmapInfo: CGImageAlphaInfo.noneSkipLast.rawValue)!
    flat.draw(rep.cgImage!, in: CGRect(x: 0, y: 0, width: W, height: H))
    let png = NSBitmapImageRep(cgImage: flat.makeImage()!).representation(using: .png, properties: [:])!
    try! png.write(to: URL(fileURLWithPath: "\(outDir)/\(name).png"))
    print("wrote \(name).png (\(W)x\(H))")
}

// MARK: The icon's scene

/// Dusk sky like the icon: purple at the top, warm at the horizon.
func sky(_ W: CGFloat, _ H: CGFloat, horizon: CGFloat) {
    NSGradient(colors: [color(0x2A1E66), color(0x7A3FA8), color(0xF2778C), color(0xFFB27A)],
               atLocations: [0, 0.45, 0.8, 1], colorSpace: .sRGB)!
        .draw(in: NSRect(x: 0, y: 0, width: W, height: horizon), angle: 90)
}

/// Fixed, hand-placed stars, given as fractions of the sky.
func stars(_ W: CGFloat, _ skyH: CGFloat) {
    let spots: [(CGFloat, CGFloat, CGFloat)] = [
        (0.04, 0.12, 7), (0.10, 0.30, 5), (0.17, 0.07, 9), (0.23, 0.22, 5), (0.31, 0.10, 6), (0.37, 0.28, 4),
        (0.45, 0.05, 7), (0.52, 0.18, 5), (0.60, 0.08, 8), (0.66, 0.26, 5), (0.73, 0.12, 6), (0.80, 0.04, 9),
        (0.86, 0.22, 5), (0.92, 0.10, 7), (0.97, 0.30, 5), (0.27, 0.38, 4), (0.70, 0.40, 4), (0.08, 0.45, 4),
    ]
    color(0xFFFFFF, 0.8).setFill()
    for (x, y, r) in spots { circle(W * x, skyH * y, r).fill() }
}

func sun(_ x: CGFloat, _ y: CGFloat, _ r: CGFloat) {
    NSGradient(colors: [color(0xF3D3A0), color(0xFAD99C)])!.draw(in: circle(x, y, r), angle: -90)
}

/// Two rolling hills; `top` is roughly where the back hill crests.
func hills(_ W: CGFloat, _ H: CGFloat, top: CGFloat) {
    let back = NSBezierPath()
    back.move(to: NSPoint(x: 0, y: top + 120))
    back.curve(to: NSPoint(x: W, y: top + 60), controlPoint1: NSPoint(x: W * 0.35, y: top - 90), controlPoint2: NSPoint(x: W * 0.7, y: top - 60))
    back.line(to: NSPoint(x: W, y: H)); back.line(to: NSPoint(x: 0, y: H)); back.close()
    color(0x3F9C5A).setFill(); back.fill()
    let front = NSBezierPath()
    front.move(to: NSPoint(x: 0, y: top + 260))
    front.curve(to: NSPoint(x: W, y: top + 300), controlPoint1: NSPoint(x: W * 0.3, y: top + 120), controlPoint2: NSPoint(x: W * 0.65, y: top + 160))
    front.line(to: NSPoint(x: W, y: H)); front.line(to: NSPoint(x: 0, y: H)); front.close()
    color(0x5BC06E).setFill(); front.fill()
}

/// The icon's sheep, in the icon's 1024-point coordinates, standing with its
/// feet at (x, y). `size` is the drawn width of that 1024 box.
func sheep(_ ctx: CGContext, x: CGFloat, y: CGFloat, size: CGFloat, facingLeft: Bool = false) {
    let s = size / 1024
    ctx.saveGState()
    ctx.translateBy(x: x, y: y)
    ctx.scaleBy(x: facingLeft ? -s : s, y: s)
    ctx.translateBy(x: -480, y: -872) // feet center in the icon

    color(0x1E5A2E, 0.35).setFill()
    NSBezierPath(ovalIn: NSRect(x: 220, y: 846, width: 500, height: 52)).fill()
    color(0x2B2533).setFill()
    for lx in [296, 386, 516, 606] as [CGFloat] {
        NSBezierPath(roundedRect: NSRect(x: lx, y: 690, width: 48, height: 180), xRadius: 24, yRadius: 24).fill()
    }
    let wool: [(CGFloat, CGFloat, CGFloat)] = [
        (390, 500, 125), (510, 470, 120), (610, 520, 110), (300, 590, 105), (320, 665, 95), (430, 690, 105),
        (555, 690, 105), (665, 640, 100), (520, 600, 150), (400, 600, 150),
    ]
    color(0xD5D1E6).setFill()
    for (wx, wy, r) in wool { circle(wx, wy + 16, r).fill() }
    color(0xFFFFFF).setFill()
    for (wx, wy, r) in wool { circle(wx, wy, r).fill() }

    // Head: ears, face, tuft, and the prompt.
    color(0x2B2533).setFill()
    for (ex, angle) in [(612, -25), (880, 25)] as [(CGFloat, CGFloat)] {
        ctx.saveGState(); ctx.translateBy(x: ex, y: 445); ctx.rotate(by: angle * .pi / 180)
        NSBezierPath(ovalIn: NSRect(x: -48, y: -28, width: 96, height: 56)).fill()
        ctx.restoreGState()
    }
    NSBezierPath(roundedRect: NSRect(x: 620, y: 395, width: 250, height: 200), xRadius: 64, yRadius: 64).fill()
    color(0xFFFFFF).setFill()
    for (tx, ty) in [(685, 392), (745, 384), (805, 392)] as [(CGFloat, CGFloat)] { circle(tx, ty, 44).fill() }
    color(0x7CFC9A).setStroke(); color(0x7CFC9A).setFill()
    // Text can't be mirrored: flip the prompt back around the face's center.
    if facingLeft { ctx.translateBy(x: 745 * 2, y: 0); ctx.scaleBy(x: -1, y: 1) }
    let prompt = NSBezierPath()
    prompt.move(to: NSPoint(x: 682, y: 458)); prompt.line(to: NSPoint(x: 735, y: 503)); prompt.line(to: NSPoint(x: 682, y: 548))
    prompt.lineWidth = 30; prompt.lineCapStyle = .round; prompt.lineJoinStyle = .round; prompt.stroke()
    NSBezierPath(roundedRect: NSRect(x: 762, y: 540, width: 84, height: 26), xRadius: 13, yRadius: 13).fill()
    ctx.restoreGState()
}

/// A screenshot as a phone: rounded, outlined, shadowed.
func phone(_ file: String, x: CGFloat, top: CGFloat, width: CGFloat) {
    let img = NSImage(contentsOfFile: "\(raw)/\(file)")!
    let r = NSRect(x: x, y: top, width: width, height: width * 2868 / 1320)
    let radius = width * 0.087
    let path = NSBezierPath(roundedRect: r, xRadius: radius, yRadius: radius)
    NSGraphicsContext.saveGraphicsState()
    let shadow = NSShadow(); shadow.shadowColor = color(0x000000, 0.45); shadow.shadowBlurRadius = 70; shadow.shadowOffset = NSSize(width: 0, height: -24)
    shadow.set(); color(0x000000).setFill(); path.fill()
    NSGraphicsContext.restoreGraphicsState()
    NSGraphicsContext.saveGraphicsState(); path.addClip()
    img.draw(in: r, from: .zero, operation: .sourceOver, fraction: 1, respectFlipped: true, hints: nil)
    NSGraphicsContext.restoreGraphicsState()
    color(0xFFFFFF, 0.2).setStroke(); path.lineWidth = width * 0.006; path.stroke()
}

// MARK: Product page header: 3840x1646 (21:9).
// Safe area from Apple's template: x 1097-2743, y 493-1154. Only the scene
// sits outside it.

image("header", 3840, 1646) { ctx in
    let W: CGFloat = 3840, H: CGFloat = 1646
    sky(W, H, horizon: H)
    stars(W, 520)
    sun(640, 820, 250)
    hills(W, H, top: 1260)
    sheep(ctx, x: 3200, y: 1530, size: 900, facingLeft: true)
    sheep(ctx, x: 2780, y: 1560, size: 420, facingLeft: true)
    sheep(ctx, x: 520, y: 1550, size: 520)
    sheep(ctx, x: 1020, y: 1585, size: 330)
    draw("Your agents,\nin your pocket.", font: rounded(190, .heavy), color: .white,
         in: NSRect(x: 1097, y: 500, width: 1646, height: 460), shadow: true)
    draw("Approve, steer and start them from anywhere.", font: rounded(66, .semibold), color: color(0xFFFFFF, 0.92),
         in: NSRect(x: 1097, y: 1000, width: 1646, height: 110), shadow: true)
}

// MARK: Search results: 3840x2560 (3:2), shown about 365 pt wide.
// Safe area from Apple's template: x 836-3004, y 765-1795. Apple's advice is
// to show the app, so it's three phones; the scene fills the rest.

image("search", 3840, 2560) { ctx in
    let W: CGFloat = 3840, H: CGFloat = 2560
    sky(W, H, horizon: H)
    stars(W, 700)
    sun(420, 1150, 260)
    hills(W, H, top: 1950)
    sheep(ctx, x: 360, y: 2420, size: 640)
    sheep(ctx, x: 3460, y: 2440, size: 640, facingLeft: true)
    phone("2-blocked.png", x: 890, top: 860, width: 620)
    phone("3-live.png", x: 2330, top: 860, width: 620)
    phone("1-agents.png", x: 1570, top: 700, width: 700)
}
