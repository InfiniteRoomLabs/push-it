import Gio from 'gi://Gio';
import GLib from 'gi://GLib';
import St from 'gi://St';
import Cairo from 'cairo';
import * as Main from 'resource:///org/gnome/shell/ui/main.js';
import { Extension } from 'resource:///org/gnome/shell/extensions/extension.js';
import { DEFAULT_DURATION_S, opacityAt, edgeStops, stripGradient, alphaStops, glowWidth, EDGE } from './glowmath.js';

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

    // D-Bus: show the glow for `seconds`; calling again extends the deadline
    // (never shortens an already-running glow).
    Start(seconds) {
        const s = Number.isFinite(seconds) && seconds > 0 ? seconds : DEFAULT_DURATION_S;
        const now = GLib.get_monotonic_time();
        const end = now + Math.round(s * 1e6);
        this._endUs = this._area ? Math.max(this._endUs, end) : end;
        if (this._area) return;
        this._startUs = now;
        const m = Main.layoutManager.primaryMonitor;
        this._area = new St.DrawingArea({ reactive: false, x: m.x, y: m.y, width: m.width, height: m.height });
        this._area.opacity = Math.round(255 * opacityAt(0));
        this._area.connect('repaint', a => this._paint(a));
        Main.layoutManager.addTopChrome(this._area, { affectsInputRegion: false, affectsStruts: false, trackFullscreen: false });
        this._timer = GLib.timeout_add(GLib.PRIORITY_DEFAULT, FRAME_MS, () => {
            if (GLib.get_monotonic_time() >= this._endUs) { this.Stop(); return GLib.SOURCE_REMOVE; }
            const elapsedMs = (GLib.get_monotonic_time() - this._startUs) / 1000;
            this._area.opacity = Math.round(255 * opacityAt(elapsedMs));
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
        const W = glowWidth(w, h);
        const stops = alphaStops(W);
        // Four full-length strips, source-over in order top, bottom, left,
        // right: an axial hue gradient along the edge, masked by a
        // perpendicular alpha gradient that feathers inward over W px.
        // Overlapping corners are the default OVER operator composing two
        // strips; the opacity pulse is applied after, via area.opacity.
        // Point on the edge itself, per strip - the mask gradient's start.
        const edgePoint = { [EDGE.TOP]: [0, 0], [EDGE.BOTTOM]: [0, h], [EDGE.LEFT]: [0, 0], [EDGE.RIGHT]: [w, 0] };
        for (const { edge, x, y, sw, sh, x0, y0, x1, y1, p0, p1, nx, ny } of stripGradient(w, h)) {
            cr.save();
            cr.rectangle(x, y, sw, sh);
            cr.clip();
            const hueGrad = new Cairo.LinearGradient(x0, y0, x1, y1);
            for (const [off, r, g, b] of edgeStops(p0, p1, elapsedMs)) hueGrad.addColorStopRGBA(off, r, g, b, 1);
            const [ex, ey] = edgePoint[edge];
            const alphaGrad = new Cairo.LinearGradient(ex, ey, ex + nx * W, ey + ny * W);
            for (const [off, a] of stops) alphaGrad.addColorStopRGBA(off, 0, 0, 0, a);
            cr.setSource(hueGrad);
            cr.mask(alphaGrad);
            cr.restore();
        }
        cr.$dispose();
    }
}
