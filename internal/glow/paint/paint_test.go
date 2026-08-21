package paint

import (
	"bytes"
	"math"
	"testing"
	"time"
)

func TestGlowWidthScales(t *testing.T) {
	cases := []struct {
		w, h, want int
	}{
		{1920, 1080, 96},
		{2560, 1440, 128},
		{3840, 2160, 192},
		{800, 600, 53},
		{10, 10, 1},
	}
	for _, c := range cases {
		if got := GlowWidth(c.w, c.h); got != c.want {
			t.Fatalf("GlowWidth(%d, %d) = %d, want %d", c.w, c.h, got, c.want)
		}
	}
}

func TestEdgeAlphaFalloff(t *testing.T) {
	if a := EdgeAlpha(0, 96); a != 1 {
		t.Fatalf("EdgeAlpha(0, 96) = %v, want 1", a)
	}
	if a := EdgeAlpha(48, 96); math.Abs(a-0.25) > 1e-9 {
		t.Fatalf("EdgeAlpha(48, 96) = %v, want 0.25", a)
	}
	if a := EdgeAlpha(96, 96); a != 0 {
		t.Fatalf("EdgeAlpha(96, 96) = %v, want 0", a)
	}
	if a := EdgeAlpha(200, 96); a != 0 {
		t.Fatalf("EdgeAlpha(200, 96) = %v, want 0", a)
	}
	prev := EdgeAlpha(0, 96)
	for d := 1.0; d <= 96; d++ {
		cur := EdgeAlpha(d, 96)
		if cur > prev {
			t.Fatalf("EdgeAlpha not monotone non-increasing at d=%v: %v > %v", d, cur, prev)
		}
		prev = cur
	}
}

func TestEdgePosClockwise(t *testing.T) {
	w, h := 200, 100
	if p := EdgePos(Top, 0, 0, w, h); p != 0 {
		t.Fatalf("EdgePos(Top, 0, 0) = %v, want 0", p)
	}
	top := EdgePos(Top, 199, 0, w, h)
	right := EdgePos(Right, 199, 0, w, h)
	if !(top < right) {
		t.Fatalf("top(%v) must be < right(%v)", top, right)
	}
	right2 := EdgePos(Right, 199, 99, w, h)
	bottom := EdgePos(Bottom, 199, 99, w, h)
	if !(right2 < bottom) {
		t.Fatalf("right(%v) must be < bottom(%v)", right2, bottom)
	}
	bottom2 := EdgePos(Bottom, 0, 99, w, h)
	left := EdgePos(Left, 0, 99, w, h)
	if !(bottom2 < left) {
		t.Fatalf("bottom(%v) must be < left(%v)", bottom2, left)
	}
	if p := EdgePos(Left, 0, 0, w, h); !(p < 1) {
		t.Fatalf("EdgePos(Left, 0, 0) = %v, want < 1", p)
	}
}

func TestRenderCompositesOverlappingStrips(t *testing.T) {
	w, h := 200, 100
	width := 9 // round(100*96/1080)
	if GlowWidth(w, h) != width {
		t.Fatalf("test assumption broken: GlowWidth(%d,%d) = %d, want %d", w, h, GlowWidth(w, h), width)
	}
	buf := make([]byte, w*h*4)
	Render(buf, w, h, 0)
	alpha := func(x, y int) byte { return buf[(y*w+x)*4+3] }

	// The opacity pulse scales the composed result (applied after the four
	// strips are composed), so edges are uniformly bright and corners are
	// never brighter than edge midpoints: (100,0), (0,50), and the corner
	// (0,0) all equal round(255*OpacityAt(0)).
	want := uint8(math.Round(255 * OpacityAt(0)))
	if a := alpha(100, 0); a != want {
		t.Fatalf("alpha(100,0) = %d, want %d", a, want)
	}
	if a := alpha(0, 50); a != want {
		t.Fatalf("alpha(0,50) = %d, want %d", a, want)
	}
	if a := alpha(0, 0); a != want {
		t.Fatalf("alpha(0,0) (corner) = %d, want %d", a, want)
	}

	wantEdge := uint8(math.Round(255 * OpacityAt(0) * EdgeAlpha(4, width)))
	if a := alpha(4, 50); absDiff(a, wantEdge) > 1 {
		t.Fatalf("alpha(4,50) = %d, want ~%d", a, wantEdge)
	}

	e := EdgeAlpha(4, width)
	wantCorner := uint8(math.Round(255 * OpacityAt(0) * (1 - (1-e)*(1-e))))
	if a := alpha(4, 4); absDiff(a, wantCorner) > 1 {
		t.Fatalf("alpha(4,4) = %d, want ~%d", a, wantCorner)
	}

	if a := alpha(100, 50); a != 0 {
		t.Fatalf("alpha(100,50) (interior) = %d, want 0", a)
	}

	if a0, aMid := alpha(0, 0), alpha(100, 0); a0 > aMid {
		t.Fatalf("corner alpha %d must not exceed edge-midpoint alpha %d", a0, aMid)
	}

	for i := 0; i < len(buf); i += 4 {
		a := buf[i+3]
		if buf[i] > a || buf[i+1] > a || buf[i+2] > a {
			t.Fatalf("pixel %d not premultiplied: %v", i/4, buf[i:i+4])
		}
	}
}

func absDiff(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}

func TestRenderGlowEqualsRender(t *testing.T) {
	sizes := []struct{ w, h int }{{200, 100}, {101, 37}}
	elapsed := []time.Duration{0, RotationPeriod / 3}
	for _, s := range sizes {
		for _, e := range elapsed {
			want := make([]byte, s.w*s.h*4)
			Render(want, s.w, s.h, e)
			got := make([]byte, s.w*s.h*4)
			RenderGlow(got, s.w, s.h, e)
			if !bytes.Equal(got, want) {
				t.Fatalf("RenderGlow != Render for %dx%d at %v", s.w, s.h, e)
			}
		}
	}
}

func TestRenderAdvancesWithTime(t *testing.T) {
	w, h := 64, 48
	a := make([]byte, w*h*4)
	b := make([]byte, w*h*4)
	Render(a, w, h, 0)
	Render(b, w, h, RotationPeriod/2)
	same := true
	for i := range a {
		if a[i] != b[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("frames half a rotation apart must differ")
	}
}

func TestRenderRejectsShortBuffer(t *testing.T) {
	defer func() {
		if recover() != nil {
			t.Fatal("Render must not panic on a short buffer")
		}
	}()
	Render(make([]byte, 10), 64, 48, 0)
}

func TestHSVToRGBPrimaries(t *testing.T) {
	cases := []struct {
		h       float64
		r, g, b uint8
	}{{0, 255, 0, 0}, {1.0 / 3, 0, 255, 0}, {2.0 / 3, 0, 0, 255}}
	for _, c := range cases {
		r, g, b := HSVToRGB(c.h)
		if r != c.r || g != c.g || b != c.b {
			t.Fatalf("hue %v -> %d %d %d", c.h, r, g, b)
		}
	}
}

func TestHueAtRotatesOncePerPeriod(t *testing.T) {
	if HueAt(0, 0) != 0 {
		t.Fatal("hue at origin, t=0 must be 0")
	}
	half := HueAt(0, RotationPeriod/2)
	if math.Abs(half-0.5) > 1e-9 {
		t.Fatalf("half period should advance hue by 0.5, got %v", half)
	}
	full := HueAt(0.25, RotationPeriod)
	if math.Abs(full-0.25) > 1e-9 {
		t.Fatalf("full period must wrap to the same hue, got %v", full)
	}
}

func TestOpacityAtStaysInBounds(t *testing.T) {
	if o := OpacityAt(0); math.Abs(o-(MinOpacity+MaxOpacity)/2) > 1e-9 {
		t.Fatalf("t=0 should be the midpoint, got %v", o)
	}
	if o := OpacityAt(PulsePeriod / 4); math.Abs(o-MaxOpacity) > 1e-9 {
		t.Fatalf("quarter period should be max, got %v", o)
	}
	for ms := 0; ms < 2000; ms += 7 {
		o := OpacityAt(time.Duration(ms) * time.Millisecond)
		if o < MinOpacity-1e-9 || o > MaxOpacity+1e-9 {
			t.Fatalf("opacity out of bounds at %dms: %v", ms, o)
		}
	}
}
