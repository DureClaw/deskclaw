// Draws the DeskClaw app icon: a desktop screen gripped by three orange talons (the same
// orange as the "computer use running" overlay). Run: swift app/icon.swift <out-dir>
// → <out-dir>/DeskClaw.iconset/*.png, then `iconutil -c icns` (see Makefile `icon`).
import AppKit

let orange = NSColor(calibratedRed: 1.0, green: 0.45, blue: 0.0, alpha: 1)
let orangeLight = NSColor(calibratedRed: 1.0, green: 0.62, blue: 0.22, alpha: 1)
let navyTop = NSColor(calibratedRed: 0.17, green: 0.22, blue: 0.42, alpha: 1)
let navyBottom = NSColor(calibratedRed: 0.07, green: 0.09, blue: 0.20, alpha: 1)

func draw(_ s: CGFloat) {
    let u = s / 1024  // design on a 1024 grid
    func r(_ x: CGFloat, _ y: CGFloat, _ w: CGFloat, _ h: CGFloat) -> NSRect { NSRect(x: x * u, y: y * u, width: w * u, height: h * u) }
    func p(_ x: CGFloat, _ y: CGFloat) -> NSPoint { NSPoint(x: x * u, y: y * u) }

    // macOS icon grid: 824pt body inset 100pt, ~185pt corner radius
    let body = NSBezierPath(roundedRect: r(100, 100, 824, 824), xRadius: 185 * u, yRadius: 185 * u)
    NSGraphicsContext.saveGraphicsState()
    let shadow = NSShadow()
    shadow.shadowColor = NSColor.black.withAlphaComponent(0.35)
    shadow.shadowBlurRadius = 24 * u
    shadow.shadowOffset = NSSize(width: 0, height: -10 * u)
    shadow.set()
    NSGradient(starting: navyTop, ending: navyBottom)!.draw(in: body, angle: -90)
    NSGraphicsContext.restoreGraphicsState()

    // the desktop: monitor with an orange "running" banner on top
    let screen = NSBezierPath(roundedRect: r(205, 300, 614, 400), xRadius: 34 * u, yRadius: 34 * u)
    NSColor(calibratedWhite: 0.97, alpha: 1).setFill()
    screen.fill()
    let glass = NSBezierPath(roundedRect: r(229, 324, 566, 352), xRadius: 18 * u, yRadius: 18 * u)
    NSGradient(starting: NSColor(calibratedRed: 0.80, green: 0.87, blue: 1.0, alpha: 1),
               ending: NSColor(calibratedRed: 0.62, green: 0.72, blue: 0.95, alpha: 1))!.draw(in: glass, angle: -90)
    // a couple of window shapes on the screen
    NSColor.white.withAlphaComponent(0.9).setFill()
    NSBezierPath(roundedRect: r(262, 390, 230, 160), xRadius: 12 * u, yRadius: 12 * u).fill()
    NSBezierPath(roundedRect: r(520, 360, 240, 120), xRadius: 12 * u, yRadius: 12 * u).fill()
    orange.setFill()
    NSBezierPath(roundedRect: r(330, 610, 364, 40), xRadius: 20 * u, yRadius: 20 * u).fill()
    // stand
    NSColor(calibratedWhite: 0.85, alpha: 1).setFill()
    let stand = NSBezierPath()
    stand.move(to: p(450, 300)); stand.line(to: p(574, 300)); stand.line(to: p(600, 230)); stand.line(to: p(424, 230)); stand.close()
    stand.fill()
    NSBezierPath(roundedRect: r(380, 205, 264, 34), xRadius: 17 * u, yRadius: 17 * u).fill()

    // three hooked talons reaching in from above the icon edge and gripping the screen
    NSGraphicsContext.saveGraphicsState()
    body.addClip()
    for (i, x) in [CGFloat(340), 510, 680].enumerated() {
        let tipY: CGFloat = i == 1 ? 540 : 575  // middle talon reaches deepest
        let talon = NSBezierPath()
        talon.move(to: p(x - 58, 990))                                               // root, outside the body
        talon.curve(to: p(x + 62, tipY), controlPoint1: p(x - 110, 760), controlPoint2: p(x - 50, 590))  // outer curve → tip
        talon.curve(to: p(x + 30, 990), controlPoint1: p(x + 2, 660), controlPoint2: p(x + 28, 820))     // inner curve back up
        talon.close()
        NSGraphicsContext.saveGraphicsState()
        let ts = NSShadow()
        ts.shadowColor = NSColor.black.withAlphaComponent(0.4)
        ts.shadowBlurRadius = 16 * u
        ts.shadowOffset = NSSize(width: 6 * u, height: -10 * u)
        ts.set()
        NSGradient(starting: orangeLight, ending: orange)!.draw(in: talon, angle: -70)
        NSGraphicsContext.restoreGraphicsState()
        // highlight along the outer curve
        let hl = NSBezierPath()
        hl.move(to: p(x - 50, 920))
        hl.curve(to: p(x + 40, tipY + 36), controlPoint1: p(x - 82, 760), controlPoint2: p(x - 34, 620))
        hl.lineWidth = 8 * u
        hl.lineCapStyle = .round
        NSColor.white.withAlphaComponent(0.35).setStroke()
        hl.stroke()
    }
    NSGraphicsContext.restoreGraphicsState()
}

let out = URL(fileURLWithPath: CommandLine.arguments.count > 1 ? CommandLine.arguments[1] : ".")
    .appendingPathComponent("DeskClaw.iconset")
try? FileManager.default.createDirectory(at: out, withIntermediateDirectories: true)
for base in [16, 32, 128, 256, 512] {
    for scale in [1, 2] {
        let px = base * scale
        let rep = NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: px, pixelsHigh: px, bitsPerSample: 8,
                                   samplesPerPixel: 4, hasAlpha: true, isPlanar: false, colorSpaceName: .deviceRGB,
                                   bytesPerRow: 0, bitsPerPixel: 0)!
        NSGraphicsContext.saveGraphicsState()
        NSGraphicsContext.current = NSGraphicsContext(bitmapImageRep: rep)
        draw(CGFloat(px))
        NSGraphicsContext.restoreGraphicsState()
        let name = scale == 1 ? "icon_\(base)x\(base).png" : "icon_\(base)x\(base)@2x.png"
        try! rep.representation(using: .png, properties: [:])!.write(to: out.appendingPathComponent(name))
    }
}
print(out.path)
