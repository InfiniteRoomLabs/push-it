import { assert, assertApprox, assertEqual, suite } from './testHarness.js';
import * as M from '../ext/glowmath.js';

suite('constants mirror internal/glow/glow.go', () => {
    assertEqual(M.FRAME_THICKNESS, 14, 'frame thickness');
    assertEqual(M.ROTATION_PERIOD_MS, 2000, 'rotation period');
    assertEqual(M.PULSE_PERIOD_MS, 600, 'pulse period');
    assertEqual(M.MIN_OPACITY, 0.55, 'min opacity');
    assertEqual(M.MAX_OPACITY, 1.0, 'max opacity');
    assertEqual(M.DEFAULT_DURATION_S, 3.5, 'default duration');
});

suite('inFrame', () => {
    const w = 200, h = 100, t = M.FRAME_THICKNESS;
    assert(M.inFrame(0, 0, w, h), 'top-left corner');
    assert(M.inFrame(w - 1, 0, w, h), 'top-right corner');
    assert(M.inFrame(0, h - 1, w, h), 'bottom-left corner');
    assert(M.inFrame(w - 1, h - 1, w, h), 'bottom-right corner');
    assert(M.inFrame(w / 2, 0, w, h), 'top edge midpoint');
    assert(M.inFrame(w / 2, h - 1, w, h), 'bottom edge midpoint');
    assert(M.inFrame(0, h / 2, w, h), 'left edge midpoint');
    assert(M.inFrame(w - 1, h / 2, w, h), 'right edge midpoint');
    assert(!M.inFrame(w / 2, h / 2, w, h), 'centre is not in frame');
    assert(!M.inFrame(t, t, w, h), '(t, t) is not in frame');
});

suite('perimeterPos', () => {
    const w = 200, h = 100, t = M.FRAME_THICKNESS;
    const top = M.perimeterPos(50, 0, w, h), right = M.perimeterPos(w - 1, 50, w, h);
    const bottom = M.perimeterPos(150, h - 1, w, h), left = M.perimeterPos(0, 50, w, h);
    assert(top < right && right < bottom && bottom < left && left < 1, 'clockwise order');
    assertEqual(M.perimeterPos(0, 0, w, h), 0, 'top-left is 0');
    assertEqual(M.perimeterPos(t, t, w, h), M.perimeterPos(t, 0, w, h), 'interior pixel matches top-band value of its column');
});

suite('hueAt / opacityAt match the Go reference values', () => {
    assertEqual(M.hueAt(0, 0), 0, 'hue origin');
    assertApprox(M.hueAt(0, 1000), 0.5, 1e-9, 'half rotation');
    assertApprox(M.hueAt(0.25, 2000), 0.25, 1e-9, 'full rotation wraps');
    assertApprox(M.opacityAt(0), 0.775, 1e-9, 'midpoint at t=0');
    assertApprox(M.opacityAt(150), 1.0, 1e-9, 'max at quarter period');
    for (let ms = 0; ms < 2000; ms += 7) {
        const o = M.opacityAt(ms);
        assert(o >= 0.55 - 1e-9 && o <= 1 + 1e-9, `bounds at ${ms}ms`);
    }
});

suite('hsvToRgb primaries', () => {
    assertEqual(JSON.stringify(M.hsvToRgb(0)), '[1,0,0]', 'red');
    assertEqual(JSON.stringify(M.hsvToRgb(1 / 3)), '[0,1,0]', 'green');
    assertEqual(JSON.stringify(M.hsvToRgb(2 / 3)), '[0,0,1]', 'blue');
});

suite('edgeStops', () => {
    const stops = M.edgeStops(0, 0.25, 0, 5);
    assertEqual(stops.length, 5, 'count');
    assertEqual(stops[0][0], 0, 'first offset');
    assertEqual(stops[4][0], 1, 'last offset');
});
