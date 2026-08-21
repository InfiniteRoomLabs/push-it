// Package paint is the reference renderer for the glow frame: a rainbow
// that travels counter-clockwise around the screen edge while its opacity pulses.
// The Windows backend uses it directly; the GNOME (JS) and macOS (Swift)
// renderers mirror HueAt/OpacityAt/PerimeterPos exactly.
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
	FrameThickness = 14                     // px
	RotationPeriod = 2 * time.Second        // one full trip of the rainbow around the frame
	PulsePeriod    = 600 * time.Millisecond // opacity pulse
	MinOpacity     = 0.55
	MaxOpacity     = 1.0
)

// InFrame reports whether the pixel at (x, y) lies within the frame band of
// thickness FrameThickness around a w x h screen.
func InFrame(x, y, w, h int) bool {
	t := FrameThickness
	return x < t || x >= w-t || y < t || y >= h-t
}

// PerimeterPos maps a pixel to its position in [0,1) along the screen
// perimeter, clockwise from the top-left corner. A pixel is assigned to the
// first matching band in this order: top (y < t), bottom (y >= h-t), right
// (x >= w-t), left (x < t). All four corner squares therefore belong to the
// top or bottom band. Renderers that mirror this function must use the same
// order.
func PerimeterPos(x, y, w, h int) float64 {
	p := float64(2 * (w + h))
	t := FrameThickness
	switch {
	case y < t:
		return float64(x) / p
	case y >= h-t:
		return float64(w+h+(w-1-x)) / p
	case x >= w-t:
		return float64(w+y) / p
	case x < t:
		return float64(2*w+h+(h-1-y)) / p
	default:
		// interior pixel: not part of the frame; callers check InFrame
		// first. Returning the top-band value keeps the result
		// deterministic and in [0,1).
		return float64(x) / p
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

// Render fills buf (premultiplied BGRA, row-major, w*h*4 bytes) with the
// frame at the given elapsed time. The interior is left fully transparent.
// A buffer that is too small is left untouched.
func Render(buf []byte, w, h int, elapsed time.Duration) {
	if len(buf) < w*h*4 {
		return
	}
	alpha := OpacityAt(elapsed)
	a8 := uint8(math.Round(255 * alpha))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 4
			if !InFrame(x, y, w, h) {
				buf[i], buf[i+1], buf[i+2], buf[i+3] = 0, 0, 0, 0
				continue
			}
			setPixel(buf, i, x, y, w, h, elapsed, alpha, a8)
		}
	}
}

// RenderBand writes only the frame band - the four strips of thickness
// FrameThickness - and never touches the interior. For any buffer the
// caller zeroed once before the first call, the result is byte-for-byte
// identical to Render, at a fraction of the per-frame cost (the interior of
// the frame stays transparent for the whole animation). A buffer that is too
// small is left untouched.
func RenderBand(buf []byte, w, h int, elapsed time.Duration) {
	if len(buf) < w*h*4 {
		return
	}
	t := FrameThickness
	alpha := OpacityAt(elapsed)
	a8 := uint8(math.Round(255 * alpha))
	span := func(y, x0, x1 int) {
		for x := x0; x < x1; x++ {
			setPixel(buf, (y*w+x)*4, x, y, w, h, elapsed, alpha, a8)
		}
	}
	for y := 0; y < h; y++ {
		if y < t || y >= h-t { // full-width top and bottom strips
			span(y, 0, w)
			continue
		}
		span(y, 0, min(t, w))   // left strip
		span(y, max(t, w-t), w) // right strip
	}
}

// setPixel writes one premultiplied BGRA frame pixel at byte offset i.
func setPixel(buf []byte, i, x, y, w, h int, elapsed time.Duration, alpha float64, a8 uint8) {
	r, g, b := HSVToRGB(HueAt(PerimeterPos(x, y, w, h), elapsed))
	buf[i] = uint8(math.Round(float64(b) * alpha))
	buf[i+1] = uint8(math.Round(float64(g) * alpha))
	buf[i+2] = uint8(math.Round(float64(r) * alpha))
	buf[i+3] = a8
}
