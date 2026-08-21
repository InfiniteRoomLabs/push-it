// Mirrors internal/glow/paint/paint.go. Keep the numbers and formulas
// identical to the Go reference.
export const GLOW_WIDTH_AT_1080 = 96;   // px, at a 1080-logical-pixel shorter screen side
export const FALLOFF_EXPONENT = 2;      // quadratic inward falloff
export const ROTATION_PERIOD_MS = 2000; // one full trip around the perimeter
export const PULSE_PERIOD_MS = 600;
export const MIN_OPACITY = 0.55;
export const MAX_OPACITY = 1.0;
export const DEFAULT_DURATION_S = 3.5;

export const EDGE = { TOP: 0, BOTTOM: 1, LEFT: 2, RIGHT: 3 };

// glowWidth is the glow's width in px for a w x h screen, scaled by the
// shorter side so the glow reads consistently across resolutions.
// Mirrors paint.GlowWidth.
export function glowWidth(w, h) {
    const m = Math.min(w, h);
    const n = Math.round(m * GLOW_WIDTH_AT_1080 / 1080);
    return n < 1 ? 1 : n;
}

// edgeAlpha is the glow's alpha contribution at distance d from an edge,
// for a glow of the given width: 1 at the edge, falling to 0 at width.
// Mirrors paint.EdgeAlpha.
export function edgeAlpha(d, width) {
    if (d < 0 || d >= width) return 0;
    return Math.pow(1 - d / width, FALLOFF_EXPONENT);
}

// edgePos maps the point (x, y) on the given edge to its position in [0,1)
// along the screen perimeter, clockwise from the top-left corner. Mirrors
// paint.EdgePos.
export function edgePos(edge, x, y, w, h) {
    const p = 2 * (w + h);
    switch (edge) {
        case EDGE.TOP: return x / p;
        case EDGE.RIGHT: return (w + y) / p;
        case EDGE.BOTTOM: return (w + h + (w - 1 - x)) / p;
        default: return (2 * w + h + (h - 1 - y)) / p; // LEFT
    }
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

// stripGradient returns the four full-length edge strips (top, bottom, left,
// right) to draw. Each entry is the strip's fill rectangle {x, y, sw, sh},
// its clockwise hue-gradient line (x0, y0) -> (x1, y1) spanning the full
// edge, the perimeter positions p0 < p1 at that line's two ends, and the
// inward unit direction (nx, ny) used for the perpendicular alpha mask.
//
// p0/p1 are computed with the OWNING edge's own edgePos formula at the
// first/last pixel of that edge in clockwise order, not by evaluating
// edgePos at the shared corner: a corner sits exactly on a precedence
// boundary between two edges, so this keeps each strip's own values and
// leaves at most a 1/(2*(w+h)) gap between adjacent strips at the corner -
// the same pixel-level rounding already present in edgePos itself.
export function stripGradient(w, h) {
    const W = glowWidth(w, h);
    return [
        {
            edge: EDGE.TOP, x: 0, y: 0, sw: w, sh: W,
            x0: 0, y0: 0, x1: w, y1: 0,
            p0: edgePos(EDGE.TOP, 0, 0, w, h), p1: edgePos(EDGE.TOP, w - 1, 0, w, h),
            nx: 0, ny: 1,
        },
        {
            edge: EDGE.BOTTOM, x: 0, y: h - W, sw: w, sh: W,
            x0: w, y0: h, x1: 0, y1: h,
            p0: edgePos(EDGE.BOTTOM, w - 1, h - 1, w, h), p1: edgePos(EDGE.BOTTOM, 0, h - 1, w, h),
            nx: 0, ny: -1,
        },
        {
            edge: EDGE.LEFT, x: 0, y: 0, sw: W, sh: h,
            x0: 0, y0: h, x1: 0, y1: 0,
            p0: edgePos(EDGE.LEFT, 0, h - 1, w, h), p1: edgePos(EDGE.LEFT, 0, 0, w, h),
            nx: 1, ny: 0,
        },
        {
            edge: EDGE.RIGHT, x: w - W, y: 0, sw: W, sh: h,
            x0: w, y0: 0, x1: w, y1: h,
            p0: edgePos(EDGE.RIGHT, w - 1, 0, w, h), p1: edgePos(EDGE.RIGHT, w - 1, h - 1, w, h),
            nx: -1, ny: 0,
        },
    ];
}

// alphaStops returns 9 evenly spaced [offset, alpha] pairs from the edge
// (offset 0, alpha 1) to the glow width (offset 1, alpha 0), quadratic
// falloff. Used as the perpendicular mask gradient's color stops; the
// gradient's own start/end points (edge to edge + width) supply the
// physical distance, so the stops themselves are width-independent
// fractions - mirrors paint.EdgeAlpha evaluated at width*offset.
export function alphaStops(width) {
    const stops = [];
    for (let k = 0; k <= 8; k++) {
        const off = k / 8;
        stops.push([off, Math.pow(1 - off, FALLOFF_EXPONENT)]);
    }
    return stops;
}
