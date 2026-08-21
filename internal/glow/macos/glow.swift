// push-it glow helper for macOS.
// Mirrors internal/glow/paint via the same formulas as
// internal/glow/gnome/ext/glowmath.js (stripGradient/edgeStops/hueAt/
// hsvToRgb/perimeterPos): frame 14 px, rotation 2 s, pulse 600 ms between
// 0.55 and 1.0.
import AppKit
import QuartzCore

let frameThickness: CGFloat = 14
let rotationPeriodMs: CGFloat = 2000
let pulsePeriod: CFTimeInterval = 0.6
let minOpacity: Float = 0.55
let maxOpacity: Float = 1.0
let defaultDuration: CFTimeInterval = 3.5
let frameInterval: TimeInterval = 1.0 / 30

func parseArgs() -> (duration: CFTimeInterval, dryRun: Bool) {
    var duration = defaultDuration
    var dryRun = false
    var args = CommandLine.arguments.dropFirst().makeIterator()
    while let a = args.next() {
        switch a {
        case "--duration":
            if let v = args.next(), let d = Double(v), d >= 0 { duration = d }
        case "--dry-run":
            dryRun = true
        default:
            break
        }
    }
    return (duration, dryRun)
}

// Mirrors glowmath.js perimeterPos: a pixel is assigned to the first
// matching band in this order: top (y < t), bottom (y >= h-t), right
// (x >= w-t), left (x < t).
func perimeterPos(_ x: CGFloat, _ y: CGFloat, _ w: CGFloat, _ h: CGFloat) -> CGFloat {
    let p = 2 * (w + h)
    let t = frameThickness
    if y < t { return x / p }
    if y >= h - t { return (w + h + (w - 1 - x)) / p }
    if x >= w - t { return (w + y) / p }
    if x < t { return (2 * w + h + (h - 1 - y)) / p }
    return x / p
}

// Mirrors glowmath.js hueAt.
func hueAt(_ p: CGFloat, _ elapsedMs: CGFloat) -> CGFloat {
    let hh = p + elapsedMs / rotationPeriodMs
    return hh - hh.rounded(.down)
}

// Mirrors glowmath.js hsvToRgb (fully saturated, full brightness).
func hsvToRgb(_ hue: CGFloat) -> (CGFloat, CGFloat, CGFloat) {
    let hh = (hue - hue.rounded(.down)) * 6
    let i = Int(hh.rounded(.down))
    let f = hh - CGFloat(i)
    let q = 1 - f
    switch i % 6 {
    case 0: return (1, f, 0)
    case 1: return (q, 1, 0)
    case 2: return (0, 1, f)
    case 3: return (0, q, 1)
    case 4: return (f, 0, 1)
    default: return (1, 0, q)
    }
}

// Mirrors glowmath.js edgeStops: n evenly spaced colors between startPos and
// endPos along the perimeter. Callers pair these with evenly spaced
// locations (the offsets glowmath.js returns alongside each color).
func edgeStops(_ startPos: CGFloat, _ endPos: CGFloat, _ elapsedMs: CGFloat, _ n: Int = 16) -> [CGColor] {
    var stops: [CGColor] = []
    stops.reserveCapacity(n)
    for k in 0..<n {
        let off = CGFloat(k) / CGFloat(n - 1)
        let pos = startPos + (endPos - startPos) * off
        let (r, g, b) = hsvToRgb(hueAt(pos, elapsedMs))
        stops.append(NSColor(red: r, green: g, blue: b, alpha: 1).cgColor)
    }
    return stops
}

// One frame strip: its screen-space fill rect (AppKit coordinates, y axis
// up) and the perimeter-clockwise gradient line through it.
struct Strip {
    let frame: CGRect
    let start: CGPoint
    let end: CGPoint
    let p0: CGFloat
    let p1: CGFloat
}

// Mirrors glowmath.js stripGradient. glowmath's rects are in y-down (Cairo)
// coordinates; AppKit's y axis points up, so each rect's y becomes
// h - y_down - sh. p0/p1 keep glowmath's own per-band formulas (not
// perimeterPos at the corner - see glowmath.js for why) so adjacent strips
// stay color-continuous at the corners. Gradient direction per strip runs
// clockwise-start -> clockwise-end: top left->right, right top->bottom,
// bottom right->left, left bottom->top - expressed directly in each strip's
// own unit space.
func stripGradient(_ w: CGFloat, _ h: CGFloat) -> [Strip] {
    let p = 2 * (w + h)
    let t = frameThickness
    let top: (CGFloat) -> CGFloat = { x in x / p }
    let right: (CGFloat) -> CGFloat = { y in (w + y) / p }
    let bottom: (CGFloat) -> CGFloat = { x in (w + h + (w - 1 - x)) / p }
    let left: (CGFloat) -> CGFloat = { y in (2 * w + h + (h - 1 - y)) / p }
    return [
        Strip(frame: CGRect(x: 0, y: h - t, width: w, height: t),
              start: CGPoint(x: 0, y: 0.5), end: CGPoint(x: 1, y: 0.5),
              p0: top(0), p1: top(w)),
        Strip(frame: CGRect(x: w - t, y: t, width: t, height: h - 2 * t),
              start: CGPoint(x: 0.5, y: 1), end: CGPoint(x: 0.5, y: 0),
              p0: right(0), p1: right(h)),
        Strip(frame: CGRect(x: 0, y: 0, width: w, height: t),
              start: CGPoint(x: 1, y: 0.5), end: CGPoint(x: 0, y: 0.5),
              p0: bottom(w), p1: bottom(0)),
        Strip(frame: CGRect(x: 0, y: t, width: t, height: h - 2 * t),
              start: CGPoint(x: 0.5, y: 0), end: CGPoint(x: 0.5, y: 1),
              p0: left(h), p1: left(0)),
    ]
}

final class GlowWindow: NSWindow {
    override var canBecomeKey: Bool { false }
    override var canBecomeMain: Bool { false }
}

final class App: NSObject, NSApplicationDelegate {
    let duration: CFTimeInterval
    var window: GlowWindow?
    var strips: [Strip] = []
    var stripLayers: [CAGradientLayer] = []
    var locations: [NSNumber] = []
    var timer: Timer?
    var start: CFTimeInterval = 0
    init(duration: CFTimeInterval) { self.duration = duration }

    func applicationDidFinishLaunching(_ n: Notification) {
        guard let screen = NSScreen.main else { NSApp.terminate(nil); return }
        let frame = screen.frame
        let scale = screen.backingScaleFactor
        let w = GlowWindow(contentRect: frame, styleMask: .borderless, backing: .buffered, defer: false)
        w.level = .screenSaver
        w.isOpaque = false
        w.backgroundColor = .clear
        w.ignoresMouseEvents = true
        w.hasShadow = false
        w.collectionBehavior = [.canJoinAllSpaces, .stationary, .fullScreenAuxiliary, .ignoresCycle]
        let root = CALayer()
        root.frame = CGRect(origin: .zero, size: frame.size)
        root.contentsScale = scale
        w.contentView?.layer = root
        w.contentView?.wantsLayer = true

        strips = stripGradient(frame.width, frame.height)
        locations = (0..<16).map { NSNumber(value: Double($0) / 15.0) }
        stripLayers = strips.map { strip -> CAGradientLayer in
            let g = CAGradientLayer()
            g.type = .axial
            g.frame = strip.frame
            g.startPoint = strip.start
            g.endPoint = strip.end
            g.locations = locations
            g.contentsScale = scale
            g.colors = edgeStops(strip.p0, strip.p1, 0)
            root.addSublayer(g)
            return g
        }

        let pulse = CABasicAnimation(keyPath: "opacity")
        pulse.fromValue = minOpacity
        pulse.toValue = maxOpacity
        pulse.duration = pulsePeriod / 2
        pulse.autoreverses = true
        pulse.repeatCount = .infinity
        pulse.timingFunction = CAMediaTimingFunction(name: .easeInEaseOut)
        root.add(pulse, forKey: "pulse")

        w.orderFrontRegardless()
        window = w

        start = CACurrentMediaTime()
        timer = Timer.scheduledTimer(withTimeInterval: frameInterval, repeats: true) { [weak self] _ in
            self?.tick()
        }

        DispatchQueue.main.asyncAfter(deadline: .now() + duration) { [weak self] in
            self?.timer?.invalidate()
            NSApp.terminate(nil)
        }
    }

    func tick() {
        let elapsedMs = CGFloat((CACurrentMediaTime() - start) * 1000)
        CATransaction.begin()
        CATransaction.setDisableActions(true)
        for (layer, strip) in zip(stripLayers, strips) {
            layer.colors = edgeStops(strip.p0, strip.p1, elapsedMs)
        }
        CATransaction.commit()
    }
}

let (duration, dryRun) = parseArgs()
if dryRun { exit(0) }
let app = NSApplication.shared
app.setActivationPolicy(.accessory)
let delegate = App(duration: duration)
app.delegate = delegate
app.run()
