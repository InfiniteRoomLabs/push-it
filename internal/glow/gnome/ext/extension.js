import Gio from 'gi://Gio';
import GLib from 'gi://GLib';
import St from 'gi://St';
import Cairo from 'cairo';
import * as Main from 'resource:///org/gnome/shell/ui/main.js';
import { Extension } from 'resource:///org/gnome/shell/extensions/extension.js';
import { FRAME_THICKNESS, DEFAULT_DURATION_S, opacityAt, edgeStops, perimeterPos } from './glowmath.js';

const IFACE = `
<node>
  <interface name="com.infiniteroomlabs.PushItGlow">
    <method name="Start"><arg type="d" direction="in" name="seconds"/></method>
    <method name="Stop"/>
  </interface>
</node>`;

const FRAME_MS = 33;

export default class PushItGlowExtension extends Extension {
    enable() {
        this._area = null;
        this._timer = 0;
        this._endUs = 0;
        this._startUs = 0;
        this._dbus = Gio.DBusExportedObject.wrapJSObject(IFACE, this);
        this._dbus.export(Gio.DBus.session, '/com/infiniteroomlabs/PushItGlow');
    }

    disable() {
        this.Stop();
        if (this._dbus) { this._dbus.unexport(); this._dbus = null; }
    }

    // D-Bus: show the glow for `seconds`; calling again extends the deadline.
    Start(seconds) {
        const s = Number.isFinite(seconds) && seconds > 0 ? seconds : DEFAULT_DURATION_S;
        const now = GLib.get_monotonic_time();
        this._endUs = now + Math.round(s * 1e6);
        if (this._area) return;
        this._startUs = now;
        const m = Main.layoutManager.primaryMonitor;
        this._area = new St.DrawingArea({ reactive: false, x: m.x, y: m.y, width: m.width, height: m.height });
        this._area.connect('repaint', a => this._paint(a));
        Main.layoutManager.addTopChrome(this._area, { affectsInputRegion: false, affectsStruts: false, trackFullscreen: false });
        this._timer = GLib.timeout_add(GLib.PRIORITY_DEFAULT, FRAME_MS, () => {
            if (GLib.get_monotonic_time() >= this._endUs) { this.Stop(); return GLib.SOURCE_REMOVE; }
            this._area.queue_repaint();
            return GLib.SOURCE_CONTINUE;
        });
    }

    // D-Bus: remove the glow immediately.
    Stop() {
        if (this._timer) { GLib.source_remove(this._timer); this._timer = 0; }
        if (this._area) { Main.layoutManager.removeChrome(this._area); this._area.destroy(); this._area = null; }
    }

    _paint(area) {
        const cr = area.get_context();
        const [w, h] = area.get_surface_size();
        const elapsedMs = (GLib.get_monotonic_time() - this._startUs) / 1000;
        const t = FRAME_THICKNESS;
        area.opacity = Math.round(255 * opacityAt(elapsedMs));
        // Four strips, each a linear gradient along its length.
        const strips = [
            [0, 0, w, t, perimeterPos(0, 0, w, h), perimeterPos(w - 1, 0, w, h), true],              // top: left -> right
            [w - t, t, t, h - 2 * t, perimeterPos(w - 1, t, w, h), perimeterPos(w - 1, h - t - 1, w, h), false], // right: top -> bottom
            [0, h - t, w, t, perimeterPos(w - 1, h - 1, w, h), perimeterPos(0, h - 1, w, h), true],     // bottom: right -> left
            [0, t, t, h - 2 * t, perimeterPos(0, h - t - 1, w, h), perimeterPos(0, t, w, h), false],    // left: bottom -> top
        ];
        for (const [x, y, sw, sh, p0, p1, horizontal] of strips) {
            let grad;
            if (horizontal) grad = new Cairo.LinearGradient(p0 <= p1 ? x : x + sw, y, p0 <= p1 ? x + sw : x, y);
            else grad = new Cairo.LinearGradient(x, p0 <= p1 ? y : y + sh, x, p0 <= p1 ? y + sh : y);
            const lo = Math.min(p0, p1), hi = Math.max(p0, p1);
            for (const [off, r, g, b] of edgeStops(lo, hi, elapsedMs)) grad.addColorStopRGBA(off, r, g, b, 1);
            cr.setSource(grad);
            cr.rectangle(x, y, sw, sh);
            cr.fill();
        }
        cr.$dispose();
    }
}
