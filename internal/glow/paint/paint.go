// Package paint is the reference renderer for the glow: a rainbow that
// fades inward from every screen edge while it travels counter-clockwise
// and its opacity pulses.
// The Windows backend uses it directly; the GNOME (JS) and macOS (Swift)
// renderers mirror HueAt/OpacityAt/EdgePos/EdgeAlpha exactly.
package paint

import (
	"math"
	"time"
)

// Animation parameters. These are the single source of truth for every
// renderer; package glow re-exports them under the same names, and the JS
// and Swift renderers mirror them verbatim. They live here, in the leaf
// package, so the Windows backend inside package glow can call this
// renderer without an import cycle.
const (
	GlowWidthAt1080 = 96                     // px, at a 1080-logical-pixel shorter screen side
	FalloffExponent = 2.0                    // quadratic inward falloff
	RotationPeriod  = 2 * time.Second        // one full trip of the rainbow around the perimeter
	PulsePeriod     = 600 * time.Millisecond // opacity pulse
	MinOpacity      = 0.55
	MaxOpacity      = 1.0
)

// Edge names one of the four screen edges the glow fades in from.
type Edge int

const (
	Top Edge = iota
	Bottom
	Left
	Right
)

// GlowWidth is the glow's width in px for a w x h screen, scaled by the
// shorter side so the glow reads consistently across resolutions.
func GlowWidth(w, h int) int {
	m := w
	if h < m {
		m = h
	}
	n := int(math.Round(float64(m) * GlowWidthAt1080 / 1080))
	if n < 1 {
		return 1
	}
	return n
}

// EdgeAlpha is the glow's alpha contribution at distance d from an edge,
// for a glow of the given width: 1 at the edge, falling to 0 at width.
func EdgeAlpha(d float64, width int) float64 {
	if d < 0 || d >= float64(width) {
		return 0
	}
	return math.Pow(1-d/float64(width), FalloffExponent)
}

// EdgePos maps the point (x, y) on the given edge to its position in [0,1)
// along the screen perimeter, clockwise from the top-left corner. Renderers
// that mirror this function must use the same per-edge formula.
func EdgePos(e Edge, x, y, w, h int) float64 {
	p := float64(2 * (w + h))
	switch e {
	case Top:
		return float64(x) / p
	case Right:
		return float64(w+y) / p
	case Bottom:
		return float64(w+h+(w-1-x)) / p
	default: // Left
		return float64(2*w+h+(h-1-y)) / p
	}
}

// HueAt is the hue in [0,1) at perimeter position p after elapsed time.
func HueAt(p float64, elapsed time.Duration) float64 {
	h := p + elapsed.Seconds()/RotationPeriod.Seconds()
	return h - math.Floor(h)
}

// OpacityAt pulses sinusoidally between MinOpacity and MaxOpacity.
func OpacityAt(elapsed time.Duration) float64 {
	phase := 2 * math.Pi * elapsed.Seconds() / PulsePeriod.Seconds()
	return MinOpacity + (MaxOpacity-MinOpacity)*(0.5+0.5*math.Sin(phase))
}

// HSVToRGB converts a fully saturated, full-value hue to 8-bit RGB.
func HSVToRGB(h float64) (r, g, b uint8) {
	h = (h - math.Floor(h)) * 6
	i := int(h)
	f := h - float64(i)
	q := uint8(math.Round(255 * (1 - f)))
	t := uint8(math.Round(255 * f))
	switch i % 6 {
	case 0:
		return 255, t, 0
	case 1:
		return q, 255, 0
	case 2:
		return 0, 255, t
	case 3:
		return 0, q, 255
	case 4:
		return t, 0, 255
	default:
		return 255, 0, q
	}
}

// over composites one edge strip's contribution source-over into buf at
// pixel byte offset i. a is the strip alpha (pulse already applied), r/g/b
// its colour.
func over(buf []byte, i int, r, g, b uint8, a float64) {
	if a <= 0 {
		return
	}
	inv := 1 - a
	buf[i] = uint8(math.Round(float64(b)*a + float64(buf[i])*inv))
	buf[i+1] = uint8(math.Round(float64(g)*a + float64(buf[i+1])*inv))
	buf[i+2] = uint8(math.Round(float64(r)*a + float64(buf[i+2])*inv))
	buf[i+3] = uint8(math.Round(255*a + float64(buf[i+3])*inv))
}

// renderPixel composes top, bottom, left, right onto a zeroed pixel (x, y).
func renderPixel(buf []byte, x, y, w, h, width int, pulse float64, elapsed time.Duration) {
	i := (y*w + x) * 4
	buf[i], buf[i+1], buf[i+2], buf[i+3] = 0, 0, 0, 0
	type contrib struct {
		e Edge
		d float64
	}
	cs := [4]contrib{{Top, float64(y)}, {Bottom, float64(h - 1 - y)}, {Left, float64(x)}, {Right, float64(w - 1 - x)}}
	for _, c := range cs {
		a := pulse * EdgeAlpha(c.d, width)
		if a <= 0 {
			continue
		}
		r, g, b := HSVToRGB(HueAt(EdgePos(c.e, x, y, w, h), elapsed))
		over(buf, i, r, g, b, a)
	}
}

// Render fills buf (premultiplied BGRA, row-major, w*h*4 bytes) with the
// glow at the given elapsed time. Pixels farther than GlowWidth from every
// edge are fully transparent. A buffer that is too small is left untouched.
func Render(buf []byte, w, h int, elapsed time.Duration) {
	if len(buf) < w*h*4 {
		return
	}
	width := GlowWidth(w, h)
	pulse := OpacityAt(elapsed)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			renderPixel(buf, x, y, w, h, width, pulse, elapsed)
		}
	}
}

// RenderGlow writes only the pixels within GlowWidth of an edge. The caller
// zeroes buf once before the first call; interior pixels are never touched.
// For any buffer zeroed once before the first call, the result is
// byte-for-byte identical to Render, at a fraction of the per-frame cost.
// A buffer that is too small is left untouched.
func RenderGlow(buf []byte, w, h int, elapsed time.Duration) {
	if len(buf) < w*h*4 {
		return
	}
	width := GlowWidth(w, h)
	pulse := OpacityAt(elapsed)
	for y := 0; y < h; y++ {
		edgeRow := y < width || y >= h-width
		for x := 0; x < w; x++ {
			if !edgeRow && x >= width && x < w-width {
				x = w - width - 1 // skip the interior run
				continue
			}
			renderPixel(buf, x, y, w, h, width, pulse, elapsed)
		}
	}
}
