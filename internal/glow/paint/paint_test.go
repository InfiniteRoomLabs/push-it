package paint

import (
	"math"
	"testing"
	"time"

	"github.com/InfiniteRoomLabs/push-it/internal/glow"
)

func TestPerimeterPosWalksClockwise(t *testing.T) {
	w, h := 200, 100
	top := PerimeterPos(50, 0, w, h)
	right := PerimeterPos(w-1, 50, w, h)
	bottom := PerimeterPos(150, h-1, w, h)
	left := PerimeterPos(0, 50, w, h)
	if !(top < right && right < bottom && bottom < left && left < 1) {
		t.Fatalf("positions not clockwise: %v %v %v %v", top, right, bottom, left)
	}
	if PerimeterPos(0, 0, w, h) != 0 {
		t.Fatal("top-left corner must be position 0")
	}
}

func TestHueAtRotatesOncePerPeriod(t *testing.T) {
	if HueAt(0, 0) != 0 {
		t.Fatal("hue at origin, t=0 must be 0")
	}
	half := HueAt(0, glow.RotationPeriod/2)
	if math.Abs(half-0.5) > 1e-9 {
		t.Fatalf("half period should advance hue by 0.5, got %v", half)
	}
	full := HueAt(0.25, glow.RotationPeriod)
	if math.Abs(full-0.25) > 1e-9 {
		t.Fatalf("full period must wrap to the same hue, got %v", full)
	}
}

func TestOpacityAtStaysInBounds(t *testing.T) {
	if o := OpacityAt(0); math.Abs(o-(glow.MinOpacity+glow.MaxOpacity)/2) > 1e-9 {
		t.Fatalf("t=0 should be the midpoint, got %v", o)
	}
	if o := OpacityAt(glow.PulsePeriod / 4); math.Abs(o-glow.MaxOpacity) > 1e-9 {
		t.Fatalf("quarter period should be max, got %v", o)
	}
	for ms := 0; ms < 2000; ms += 7 {
		o := OpacityAt(time.Duration(ms) * time.Millisecond)
		if o < glow.MinOpacity-1e-9 || o > glow.MaxOpacity+1e-9 {
			t.Fatalf("opacity out of bounds at %dms: %v", ms, o)
		}
	}
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

func TestRenderFrameOnly(t *testing.T) {
	w, h := 64, 48
	buf := make([]byte, w*h*4)
	Render(buf, w, h, 0)
	alpha := func(x, y int) byte { return buf[(y*w+x)*4+3] }
	if alpha(0, 0) == 0 || alpha(w-1, h-1) == 0 || alpha(w/2, 0) == 0 || alpha(0, h/2) == 0 {
		t.Fatal("frame pixels must be opaque-ish")
	}
	if alpha(w/2, h/2) != 0 || alpha(glow.FrameThickness, glow.FrameThickness) != 0 {
		t.Fatal("interior must be transparent")
	}
	// premultiplied: no channel may exceed alpha
	for i := 0; i < len(buf); i += 4 {
		a := buf[i+3]
		if buf[i] > a || buf[i+1] > a || buf[i+2] > a {
			t.Fatalf("pixel %d not premultiplied: %v", i/4, buf[i:i+4])
		}
	}
}

func TestRenderAdvancesWithTime(t *testing.T) {
	w, h := 64, 48
	a := make([]byte, w*h*4)
	b := make([]byte, w*h*4)
	Render(a, w, h, 0)
	Render(b, w, h, glow.RotationPeriod/2)
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
