// push-it glow helper for macOS.
// Mirrors internal/glow/glow.go constants and internal/glow/paint math:
// frame 14 px, rotation 2 s, pulse 600 ms between 0.55 and 1.0.
import AppKit
import QuartzCore

let frameThickness: CGFloat = 14
let rotationPeriod: CFTimeInterval = 2.0
let pulsePeriod: CFTimeInterval = 0.6
let minOpacity: Float = 0.55
let maxOpacity: Float = 1.0
let defaultDuration: CFTimeInterval = 3.5

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

final class GlowWindow: NSWindow {
    override var canBecomeKey: Bool { false }
    override var canBecomeMain: Bool { false }
}

final class App: NSObject, NSApplicationDelegate {
    let duration: CFTimeInterval
    var window: GlowWindow?
    init(duration: CFTimeInterval) { self.duration = duration }

    func applicationDidFinishLaunching(_ n: Notification) {
        guard let screen = NSScreen.main else { NSApp.terminate(nil); return }
        let frame = screen.frame
        let w = GlowWindow(contentRect: frame, styleMask: .borderless, backing: .buffered, defer: false)
        w.level = .screenSaver
        w.isOpaque = false
        w.backgroundColor = .clear
        w.ignoresMouseEvents = true
        w.hasShadow = false
        w.collectionBehavior = [.canJoinAllSpaces, .stationary, .fullScreenAuxiliary, .ignoresCycle]
        let root = CALayer()
        root.frame = CGRect(origin: .zero, size: frame.size)
        w.contentView?.wantsLayer = true
        w.contentView?.layer = root

        // Conic rainbow, masked to a frame band, rotating via the gradient's
        // start angle; the frame is a square centred on the screen so the
        // rotation looks uniform, clipped by the mask to the visible band.
        let side = max(frame.width, frame.height) * 1.5
        let grad = CAGradientLayer()
        grad.type = .conic
        grad.frame = CGRect(x: (frame.width - side) / 2, y: (frame.height - side) / 2, width: side, height: side)
        grad.startPoint = CGPoint(x: 0.5, y: 0.5)
        grad.endPoint = CGPoint(x: 1.0, y: 0.5)
        grad.colors = stride(from: 0, through: 12, by: 1).map { i -> CGColor in
            NSColor(hue: CGFloat(i % 12) / 12, saturation: 1, brightness: 1, alpha: 1).cgColor
        }
        root.addSublayer(grad)

        let mask = CAShapeLayer()
        let path = CGMutablePath()
        path.addRect(CGRect(origin: .zero, size: frame.size))
        path.addRect(CGRect(x: frameThickness, y: frameThickness,
                            width: frame.width - 2 * frameThickness, height: frame.height - 2 * frameThickness))
        mask.path = path
        mask.fillRule = .evenOdd
        root.mask = mask

        let spin = CABasicAnimation(keyPath: "transform.rotation.z")
        spin.fromValue = 0
        spin.toValue = -2 * Double.pi
        spin.duration = rotationPeriod
        spin.repeatCount = .infinity
        grad.add(spin, forKey: "spin")

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
        DispatchQueue.main.asyncAfter(deadline: .now() + duration) { NSApp.terminate(nil) }
    }
}

let (duration, dryRun) = parseArgs()
if dryRun { exit(0) }
let app = NSApplication.shared
app.setActivationPolicy(.accessory)
let delegate = App(duration: duration)
app.delegate = delegate
app.run()
