// push-it glow helper for macOS.
// Mirrors internal/glow/paint via the same formulas as
// internal/glow/gnome/ext/glowmath.js (glowWidth/edgeAlpha/edgePos/
// stripGradient/alphaStops/edgeStops/hueAt/hsvToRgb): a feathered glow,
// 96 px wide at 1080p and scaled by the shorter screen side, that fades
// inward with quadratic falloff so corners render as two overlapping
// glows. Rotation 2 s, pulse 600 ms between 0.55 and 1.0.
import AppKit
import QuartzCore

let glowWidthAt1080: CGFloat = 96
let falloffExponent: CGFloat = 2.0
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

// One of the four screen edges the glow fades in from. Mirrors
// glowmath.js's EDGE / paint.Edge.
enum Edge {
    case top, bottom, left, right
}

// Mirrors glowmath.js glowWidth: the glow's width in px for a w x h
// screen, scaled by the shorter side.
func glowWidth(_ w: CGFloat, _ h: CGFloat) -> CGFloat {
    let m = min(w, h)
    let n = (m * glowWidthAt1080 / 1080).rounded()
    return n < 1 ? 1 : n
}

// Mirrors glowmath.js edgeAlpha: the glow's alpha contribution at
// distance d from an edge, for a glow of the given width.
func edgeAlpha(_ d: CGFloat, _ width: CGFloat) -> CGFloat {
    if d < 0 || d >= width { return 0 }
    return pow(1 - d / width, falloffExponent)
}

// Mirrors glowmath.js edgePos: the point (x, y) on the given edge mapped
// to its position in [0,1) along the screen perimeter, clockwise from the
// top-left corner. x, y, w, h are in y-down (Cairo-style) coordinates.
func edgePos(_ edge: Edge, _ x: CGFloat, _ y: CGFloat, _ w: CGFloat, _ h: CGFloat) -> CGFloat {
    let p = 2 * (w + h)
    switch edge {
    case .top: return x / p
    case .right: return (w + y) / p
    case .bottom: return (w + h + (w - 1 - x)) / p
    case .left: return (2 * w + h + (h - 1 - y)) / p
    }
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

// Mirrors glowmath.js alphaStops: 9 evenly spaced (offset, alpha) pairs
// from the edge (offset 0, alpha 1) to the glow width (offset 1, alpha 0),
// quadratic falloff. width is unused (the offsets are width-independent
// fractions) but kept for signature parity with the JS/Go references.
func alphaStops(_ width: CGFloat) -> [(CGFloat, CGFloat)] {
    var stops: [(CGFloat, CGFloat)] = []
    stops.reserveCapacity(9)
    for k in 0...8 {
        let off = CGFloat(k) / 8
        stops.append((off, pow(1 - off, falloffExponent)))
    }
    return stops
}

// One full-length edge strip: its fill rect in y-down (Cairo-style)
// coordinates, and the perimeter positions p0 < p1 at its clockwise
// hue-gradient line's two ends.
struct Strip {
    let edge: Edge
    let frame: CGRect
    let p0: CGFloat
    let p1: CGFloat
}

// Mirrors glowmath.js stripGradient: the four full-length edge strips
// (top, bottom, left, right), each spanning the full edge length rather
// than stopping short at the corners, so adjacent strips overlap there.
// p0/p1 use the OWNING edge's own edgePos formula at the first/last pixel
// of that edge in clockwise order (see glowmath.js for why: a corner sits
// on a precedence boundary between two edges).
func stripGradient(_ w: CGFloat, _ h: CGFloat) -> [Strip] {
    let W = glowWidth(w, h)
    return [
        Strip(edge: .top, frame: CGRect(x: 0, y: 0, width: w, height: W),
              p0: edgePos(.top, 0, 0, w, h), p1: edgePos(.top, w - 1, 0, w, h)),
        Strip(edge: .bottom, frame: CGRect(x: 0, y: h - W, width: w, height: W),
              p0: edgePos(.bottom, w - 1, h - 1, w, h), p1: edgePos(.bottom, 0, h - 1, w, h)),
        Strip(edge: .left, frame: CGRect(x: 0, y: 0, width: W, height: h),
              p0: edgePos(.left, 0, h - 1, w, h), p1: edgePos(.left, 0, 0, w, h)),
        Strip(edge: .right, frame: CGRect(x: w - W, y: 0, width: W, height: h),
              p0: edgePos(.right, w - 1, 0, w, h), p1: edgePos(.right, w - 1, h - 1, w, h)),
    ]
}

// appKitFrame converts a y-down (Cairo-style) rect to AppKit's y-up
// coordinate space for a screen of height h.
func appKitFrame(_ r: CGRect, _ h: CGFloat) -> CGRect {
    CGRect(x: r.minX, y: h - r.minY - r.height, width: r.width, height: r.height)
}

// hueDirection is the hue gradient's start/end unit points for an edge,
// running clockwise along the edge (AppKit y-up unit coordinates, local
// to the strip layer's own frame).
func hueDirection(_ edge: Edge) -> (CGPoint, CGPoint) {
    switch edge {
    case .top: return (CGPoint(x: 0, y: 0.5), CGPoint(x: 1, y: 0.5))
    case .right: return (CGPoint(x: 0.5, y: 1), CGPoint(x: 0.5, y: 0))
    case .bottom: return (CGPoint(x: 1, y: 0.5), CGPoint(x: 0, y: 0.5))
    case .left: return (CGPoint(x: 0.5, y: 0), CGPoint(x: 0.5, y: 1))
    }
}

// maskDirection is the alpha mask gradient's start/end unit points for an
// edge, running from the screen edge inward (AppKit y-up unit
// coordinates, local to the strip layer's own frame).
func maskDirection(_ edge: Edge) -> (CGPoint, CGPoint) {
    switch edge {
    case .top: return (CGPoint(x: 0.5, y: 1), CGPoint(x: 0.5, y: 0))
    case .bottom: return (CGPoint(x: 0.5, y: 0), CGPoint(x: 0.5, y: 1))
    case .left: return (CGPoint(x: 0, y: 0.5), CGPoint(x: 1, y: 0.5))
    case .right: return (CGPoint(x: 1, y: 0.5), CGPoint(x: 0, y: 0.5))
    }
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
    var hueLocations: [NSNumber] = []
    var timer: Timer?
    var start: CFTimeInterval = 0
    init(duration: CFTimeInterval) { self.duration = duration }

    func applicationDidFinishLaunching(_ n: Notification) {
        guard let screen = NSScreen.main ?? NSScreen.screens.first else { exit(0) }
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
        hueLocations = (0..<16).map { NSNumber(value: Double($0) / 15.0) }
        let W = glowWidth(frame.width, frame.height)
        let aStops = alphaStops(W)
        let maskLocations = aStops.map { NSNumber(value: Double($0.0)) }
        let maskColors = aStops.map { NSColor.black.withAlphaComponent($0.1).cgColor }

        stripLayers = strips.map { strip -> CAGradientLayer in
            let g = CAGradientLayer()
            g.type = .axial
            g.frame = appKitFrame(strip.frame, frame.height)
            let (start, end) = hueDirection(strip.edge)
            g.startPoint = start
            g.endPoint = end
            g.locations = hueLocations
            g.contentsScale = scale
            g.colors = edgeStops(strip.p0, strip.p1, 0)

            let mask = CAGradientLayer()
            mask.type = .axial
            mask.frame = g.bounds
            let (maskStart, maskEnd) = maskDirection(strip.edge)
            mask.startPoint = maskStart
            mask.endPoint = maskEnd
            mask.locations = maskLocations
            mask.colors = maskColors
            mask.contentsScale = scale
            g.mask = mask

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
