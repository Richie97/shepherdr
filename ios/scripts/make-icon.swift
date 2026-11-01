// Draws the app icon (a sheep with a terminal-prompt face).
// Usage: swift ios/scripts/make-icon.swift ios/HerdrUI/Assets.xcassets/AppIcon.appiconset/AppIcon.png
import AppKit
import CoreGraphics

let size: CGFloat = 1024
let cs = CGColorSpace(name: CGColorSpace.sRGB)!
let ctx = CGContext(data: nil, width: Int(size), height: Int(size), bitsPerComponent: 8, bytesPerRow: 0,
                    space: cs, bitmapInfo: CGImageAlphaInfo.noneSkipLast.rawValue)!
func rgb(_ h: UInt32, _ a: CGFloat = 1) -> CGColor {
    CGColor(srgbRed: CGFloat((h >> 16) & 255) / 255, green: CGFloat((h >> 8) & 255) / 255, blue: CGFloat(h & 255) / 255, alpha: a)
}
// Core Graphics origin is bottom-left; draw in a top-left space.
ctx.translateBy(x: 0, y: size); ctx.scaleBy(x: 1, y: -1)

// Sky: sunset gradient.
let sky = CGGradient(colorsSpace: cs, colors: [rgb(0x2B1E6B), rgb(0x7A3FA8), rgb(0xF2778C), rgb(0xFFC27A)] as CFArray,
                     locations: [0, 0.38, 0.72, 1])!
ctx.drawLinearGradient(sky, start: CGPoint(x: 0, y: 0), end: CGPoint(x: 0, y: 760), options: [.drawsAfterEndLocation])
// Sun.
ctx.setFillColor(rgb(0xFFE3A3, 0.9)); ctx.fillEllipse(in: CGRect(x: 120, y: 250, width: 230, height: 230))
// A few stars.
ctx.setFillColor(rgb(0xFFFFFF, 0.8))
for (x, y, r) in [(420.0, 120.0, 7.0), (600, 70, 5), (820, 150, 6), (690, 250, 4), (90, 140, 4), (930, 300, 5)] {
    ctx.fillEllipse(in: CGRect(x: x - r, y: y - r, width: 2 * r, height: 2 * r))
}
// Hills.
func hill(_ color: CGColor, _ y0: CGFloat, _ peak: CGFloat, _ cx: CGFloat) {
    let p = CGMutablePath()
    p.move(to: CGPoint(x: -50, y: y0))
    p.addQuadCurve(to: CGPoint(x: size + 50, y: y0 + 30), control: CGPoint(x: cx, y: peak))
    p.addLine(to: CGPoint(x: size + 50, y: size + 10)); p.addLine(to: CGPoint(x: -50, y: size + 10)); p.closeSubpath()
    ctx.addPath(p); ctx.setFillColor(color); ctx.fillPath()
}
hill(rgb(0x3E9E5A), 760, 560, 760)
hill(rgb(0x5BC26F), 840, 690, 300)

// Sheep body: overlapping wool puffs with a soft shadow.
let cx: CGFloat = 470, cy: CGFloat = 560
ctx.setFillColor(rgb(0x1B3D24, 0.35))
ctx.fillEllipse(in: CGRect(x: cx - 250, y: cy + 285, width: 500, height: 56))
// Legs.
ctx.setFillColor(rgb(0x2A2433))
for lx in [cx - 150, cx - 60, cx + 70, cx + 160] {
    let leg = CGPath(roundedRect: CGRect(x: lx - 24, y: cy + 100, width: 48, height: 210), cornerWidth: 22, cornerHeight: 22, transform: nil)
    ctx.addPath(leg); ctx.fillPath()
}
let puffs: [(CGFloat, CGFloat, CGFloat)] = [
    (-170, 20, 105), (-80, -60, 120), (40, -75, 125), (150, -30, 110), (190, 60, 100),
    (90, 90, 115), (-40, 95, 120), (-160, 90, 100), (0, 10, 150),
]
ctx.setFillColor(rgb(0xD9D3E8))
for (dx, dy, r) in puffs { ctx.fillEllipse(in: CGRect(x: cx + dx - r - 4, y: cy + dy - r + 8, width: 2 * r + 8, height: 2 * r + 8)) }
ctx.setFillColor(rgb(0xFFFFFF))
for (dx, dy, r) in puffs { ctx.fillEllipse(in: CGRect(x: cx + dx - r, y: cy + dy - r, width: 2 * r, height: 2 * r)) }

// Head: a little terminal window.
let head = CGRect(x: cx + 150, y: cy - 175, width: 250, height: 210)
// Ears.
ctx.setFillColor(rgb(0x2A2433))
ctx.saveGState(); ctx.translateBy(x: head.minX + 20, y: head.minY + 50); ctx.rotate(by: -0.6)
ctx.fillEllipse(in: CGRect(x: -70, y: -28, width: 100, height: 56)); ctx.restoreGState()
ctx.saveGState(); ctx.translateBy(x: head.maxX - 10, y: head.minY + 45); ctx.rotate(by: 0.6)
ctx.fillEllipse(in: CGRect(x: -30, y: -28, width: 100, height: 56)); ctx.restoreGState()
ctx.addPath(CGPath(roundedRect: head, cornerWidth: 62, cornerHeight: 62, transform: nil))
ctx.setFillColor(rgb(0x2A2433)); ctx.fillPath()
// Wool tuft on top.
ctx.setFillColor(rgb(0xFFFFFF))
for (dx, r) in [(-60.0, 42.0), (0, 50), (60, 42)] {
    ctx.fillEllipse(in: CGRect(x: head.midX + dx - r, y: head.minY - r + 8, width: 2 * r, height: 2 * r))
}
// Prompt face: ">_" in terminal green.
ctx.setStrokeColor(rgb(0x7CFFA0)); ctx.setLineWidth(26); ctx.setLineCap(.round); ctx.setLineJoin(.round)
let gx = head.minX + 62, gy = head.midY + 12
ctx.move(to: CGPoint(x: gx, y: gy - 46)); ctx.addLine(to: CGPoint(x: gx + 52, y: gy)); ctx.addLine(to: CGPoint(x: gx, y: gy + 46))
ctx.strokePath()
ctx.move(to: CGPoint(x: gx + 92, y: gy + 50)); ctx.addLine(to: CGPoint(x: gx + 150, y: gy + 50)); ctx.strokePath()

let img = ctx.makeImage()!
let rep = NSBitmapImageRep(cgImage: img)
try! rep.representation(using: .png, properties: [:])!.write(to: URL(fileURLWithPath: CommandLine.arguments[1]))
