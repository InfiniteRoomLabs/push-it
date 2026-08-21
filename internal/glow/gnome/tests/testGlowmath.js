import { assert, assertApprox, assertEqual, suite } from './testHarness.js';
import * as M from '../ext/glowmath.js';

suite('constants mirror internal/glow/paint/paint.go', () => {
    assertEqual(M.GLOW_WIDTH_AT_1080, 96, 'glow width at 1080p');
    assertEqual(M.FALLOFF_EXPONENT, 2, 'falloff exponent');
    assertEqual(M.ROTATION_PERIOD_MS, 2000, 'rotation period');
    assertEqual(M.PULSE_PERIOD_MS, 600, 'pulse period');
    assertEqual(M.MIN_OPACITY, 0.55, 'min opacity');
    assertEqual(M.MAX_OPACITY, 1.0, 'max opacity');
    assertEqual(M.DEFAULT_DURATION_S, 3.5, 'default duration');
});

suite('glowWidth scales by the shorter side', () => {
    assertEqual(M.glowWidth(1920, 1080), 96, '1080p');
    assertEqual(M.glowWidth(2560, 1440), 128, '1440p');
    assertEqual(M.glowWidth(10, 10), 1, 'clamped to 1');
});

suite('edgeAlpha falloff', () => {
    assertEqual(M.edgeAlpha(0, 96), 1, 'at the edge');
    assertApprox(M.edgeAlpha(48, 96), 0.25, 1e-9, 'quarter width in (quadratic)');
    assertEqual(M.edgeAlpha(96, 96), 0, 'at the glow width boundary');
});

suite('edgePos clockwise order', () => {
    const w = 200, h = 100;
    const top = M.edgePos(M.EDGE.TOP, 50, 0, w, h);
    const right = M.edgePos(M.EDGE.RIGHT, w - 1, 50, w, h);
    const bottom = M.edgePos(M.EDGE.BOTTOM, 150, h - 1, w, h);
    const left = M.edgePos(M.EDGE.LEFT, 0, 50, w, h);
    assert(top < right && right < bottom && bottom < left && left < 1, 'clockwise order');
    assertEqual(M.edgePos(M.EDGE.TOP, 0, 0, w, h), 0, 'top-left is 0');
});

suite('stripGradient full-length strips, order top/bottom/left/right', () => {
    const w = 200, h = 100, W = M.glowWidth(w, h);
    assertEqual(W, 9, 'glow width for 200x100');
    const strips = M.stripGradient(w, h);
    assertEqual(strips.length, 4, 'four strips');
    const [top, bottom, left, right] = strips;

    assertEqual(top.edge, M.EDGE.TOP, 'top edge tag');
    assertEqual(top.x, 0, 'top x');
    assertEqual(top.y, 0, 'top y');
    assertEqual(top.sw, w, 'top sw spans full width');
    assertEqual(top.sh, W, 'top sh is glow width');
    assertEqual(top.x0, 0, 'top x0');
    assertEqual(top.y0, 0, 'top y0');
    assertEqual(top.x1, w, 'top x1');
    assertEqual(top.y1, 0, 'top y1');
    assertEqual(top.nx, 0, 'top nx');
    assertEqual(top.ny, 1, 'top ny (inward is down)');
    assertEqual(top.p0, 0, 'top p0');
    assertApprox(top.p1, 199 / 600, 1e-9, 'top p1');

    assertEqual(bottom.edge, M.EDGE.BOTTOM, 'bottom edge tag');
    assertEqual(bottom.sw, w, 'bottom spans full width');
    assertEqual(bottom.sh, W, 'bottom sh is glow width');
    assert(bottom.x0 > bottom.x1, 'bottom gradient line runs right to left');
    assertEqual(bottom.ny, -1, 'bottom ny (inward is up)');

    assertEqual(left.edge, M.EDGE.LEFT, 'left edge tag');
    assertEqual(left.sh, h, 'left spans full height');
    assertEqual(left.sw, W, 'left sw is glow width');
    assert(left.y0 > left.y1, 'left gradient line runs bottom to top');
    assertEqual(left.nx, 1, 'left nx (inward is right)');

    assertEqual(right.edge, M.EDGE.RIGHT, 'right edge tag');
    assertEqual(right.sh, h, 'right spans full height');
    assertEqual(right.sw, W, 'right sw is glow width');
    assertEqual(right.nx, -1, 'right nx (inward is left)');

    for (const [name, s] of [['top', top], ['bottom', bottom], ['left', left], ['right', right]]) {
        assert(s.p0 < s.p1, `${name} p0 < p1`);
    }

    const p = 2 * (w + h);
    const tol = 1 / p + 1e-9;
    assertApprox(top.p1, right.p0, tol, 'top end == right start');
    assertApprox(right.p1, bottom.p0, tol, 'right end == bottom start');
    assertApprox(bottom.p1, left.p0, tol, 'bottom end == left start');
});

suite('alphaStops', () => {
    const stops = M.alphaStops(9);
    assertEqual(stops.length, 9, 'nine stops');
    assertEqual(stops[0][0], 0, 'first offset');
    assertEqual(stops[0][1], 1, 'first alpha is opaque');
    assertEqual(stops[8][0], 1, 'last offset');
    assertEqual(stops[8][1], 0, 'last alpha is transparent');
    for (let k = 1; k < stops.length; k++) {
        assert(stops[k][0] > stops[k - 1][0], `offset ${k} increasing`);
        assert(stops[k][1] <= stops[k - 1][1], `alpha ${k} non-increasing`);
    }
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
