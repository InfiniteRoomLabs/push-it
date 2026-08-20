// Mirrors internal/glow/glow.go constants and internal/glow/paint math.
// Keep the numbers identical to the Go source.
export const FRAME_THICKNESS = 14;      // px
export const ROTATION_PERIOD_MS = 2000; // one full trip around the frame
export const PULSE_PERIOD_MS = 600;
export const MIN_OPACITY = 0.55;
export const MAX_OPACITY = 1.0;
export const DEFAULT_DURATION_S = 3.5;

// inFrame reports whether the pixel at (x, y) lies within the frame band of
// thickness FRAME_THICKNESS around a w x h screen. Mirrors paint.InFrame.
export function inFrame(x, y, w, h) {
    const t = FRAME_THICKNESS;
    return x < t || x >= w - t || y < t || y >= h - t;
}

// A pixel is assigned to the first matching band in this order: top
// (y < t), bottom (y >= h-t), right (x >= w-t), left (x < t). All four
// corner squares therefore belong to the top or bottom band. Renderers
// that mirror this function must use the same order.
export function perimeterPos(x, y, w, h) {
    const p = 2 * (w + h);
    const t = FRAME_THICKNESS;
    if (y < t) return x / p;
    if (y >= h - t) return (w + h + (w - 1 - x)) / p;
    if (x >= w - t) return (w + y) / p;
    if (x < t) return (2 * w + h + (h - 1 - y)) / p;
    // interior pixel: not part of the frame; callers check inFrame
    // first. Returning the top-band value keeps the result
    // deterministic and in [0,1).
    return x / p;
}

export function hueAt(p, elapsedMs) {
    const h = p + elapsedMs / ROTATION_PERIOD_MS;
    return h - Math.floor(h);
}

export function opacityAt(elapsedMs) {
    const phase = 2 * Math.PI * elapsedMs / PULSE_PERIOD_MS;
    return MIN_OPACITY + (MAX_OPACITY - MIN_OPACITY) * (0.5 + 0.5 * Math.sin(phase));
}

// Returns [r, g, b] in 0..1 for a fully saturated hue.
export function hsvToRgb(hue) {
    const h = (hue - Math.floor(hue)) * 6;
    const i = Math.floor(h);
    const f = h - i;
    const q = 1 - f;
    switch (i % 6) {
        case 0: return [1, f, 0];
        case 1: return [q, 1, 0];
        case 2: return [0, 1, f];
        case 3: return [0, q, 1];
        case 4: return [f, 0, 1];
        default: return [1, 0, q];
    }
}

// Gradient stops along one edge: N evenly spaced [offset, r, g, b] tuples.
// startPos/endPos are perimeter positions of the edge's two ends.
export function edgeStops(startPos, endPos, elapsedMs, n = 16) {
    const stops = [];
    for (let k = 0; k < n; k++) {
        const off = k / (n - 1);
        const pos = startPos + (endPos - startPos) * off;
        const [r, g, b] = hsvToRgb(hueAt(pos, elapsedMs));
        stops.push([off, r, g, b]);
    }
    return stops;
}

// stripGradient returns the four frame strips (top, right, bottom, left) to
// draw. Each entry is the strip's fill rectangle {x, y, sw, sh} plus its
// perimeter-clockwise gradient line: (x0, y0) is the strip's clockwise-start
// corner, (x1, y1) its clockwise-end corner (top: left->right; right:
// top->bottom; bottom: right->left; left: bottom->top), with p0 < p1 the
// perimeter positions at those two corners.
//
// The gradient line always spans a strip's full corner-to-corner run (e.g.
// (w,0) to (w,h) for the right strip), even though the fill rectangle is
// inset by the frame thickness on its short ends - Cairo samples the line at
// the fraction corresponding to the filled region, so adjacent strips stay
// color-continuous at the corners.
//
// p0/p1 are computed with the OWNING band's own formula (mirroring
// perimeterPos's four branches) rather than by calling perimeterPos at the
// corner: a rectangle corner sits exactly on a precedence boundary (e.g.
// (w, h) is claimed by the bottom band per perimeterPos's top/bottom/right/
// left precedence), so routing through perimeterPos there would silently
// substitute the neighbor band's value. Evaluating each strip's own formula
// keeps every corner-to-corner gap to at most 1/(2*(w+h)) - the same
// pixel-level rounding already present in the band formulas themselves.
export function stripGradient(w, h) {
    const p = 2 * (w + h);
    const t = FRAME_THICKNESS;
    const top = x => x / p;
    const right = y => (w + y) / p;
    const bottom = x => (w + h + (w - 1 - x)) / p;
    const left = y => (2 * w + h + (h - 1 - y)) / p;
    return [
        { x: 0, y: 0, sw: w, sh: t, x0: 0, y0: 0, x1: w, y1: 0, p0: top(0), p1: top(w) },
        { x: w - t, y: t, sw: t, sh: h - 2 * t, x0: w, y0: 0, x1: w, y1: h, p0: right(0), p1: right(h) },
        { x: 0, y: h - t, sw: w, sh: t, x0: w, y0: h, x1: 0, y1: h, p0: bottom(w), p1: bottom(0) },
        { x: 0, y: t, sw: t, sh: h - 2 * t, x0: 0, y0: h, x1: 0, y1: 0, p0: left(h), p1: left(0) },
    ];
}
