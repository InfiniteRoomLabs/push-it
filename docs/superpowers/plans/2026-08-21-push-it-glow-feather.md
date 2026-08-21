# push-it Feathered Glow Implementation Plan (Plan 2b)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the hard 14 px rainbow frame with a feathered glow: fully opaque at the screen edge, fading to transparent inward over 96 px (at 1080p, scaled by the shorter screen side) with a quadratic falloff, corners rendered as two overlapping glows - identically on GNOME, macOS, and Windows.

**Architecture:** The look is defined once in `internal/glow/paint` (the reference) as four full-length edge strips composited in the order top, bottom, left, right with source-over; each strip's colour is the rainbow along its edge and its alpha is `pulse * (1 - d/W)^2` where `d` is the distance from that edge. The GNOME (Cairo) and macOS (Core Animation) renderers draw the same four strips with an axial hue gradient masked by a perpendicular alpha gradient; Windows uses `paint` directly.

**Tech Stack:** unchanged (Go, gjs/Cairo, Swift/Core Animation).

**Spec:** `docs/superpowers/specs/2026-08-20-push-it-design.md` - the Glow section's "frame thickness 14 px" parameter is superseded by this plan (Task 1 amends the spec text). Parent plan: `docs/superpowers/plans/2026-08-20-push-it-glow.md`.

## Global Constraints

- Visual parameters (defined in `internal/glow/paint/paint.go`, re-exported by `internal/glow/glow.go`, mirrored verbatim in JS and Swift): `GlowWidthAt1080 = 96` (px), `FalloffExponent = 2`, `RotationPeriod = 2s`, `PulsePeriod = 600ms`, `MinOpacity = 0.55`, `MaxOpacity = 1.0`, `DefaultDuration = 3500ms`. `FrameThickness` is removed.
- Glow width for a screen of size `w x h` (logical pixels): `W = max(1, round(min(w, h) * GlowWidthAt1080 / 1080))`.
- Per-edge alpha at distance `d` from that edge: `edgeAlpha(d, W) = (1 - d/W)^FalloffExponent` for `0 <= d < W`, else `0`.
- Composition order is fixed: top strip, bottom strip, left strip, right strip, each source-over the previous. Corners therefore show both the horizontal and the vertical strip (`alpha = 1 - (1 - aH)(1 - aV)`, colour = vertical strip over horizontal strip). The pulse (`OpacityAt`) scales the composed result (applied after the four strips are composed), so edges are uniformly bright and corners are never brighter than edge midpoints.
- Hue along an edge is unchanged: `HueAt(p, elapsed)` with `p` the clockwise perimeter position of the point on that edge (top: `x/P`; right: `(w+y)/P`; bottom: `(w+h+(w-1-x))/P`; left: `(2w+h+(h-1-y))/P`; `P = 2(w+h)`). Left/right strips now run the full screen height; top/bottom the full width.
- Everything else from Plan 2's Global Constraints still applies (no cgo, no new deps, ASCII, CHANGELOG on every commit, hermetic tests, Sonnet implementers reviewed by Opus, Fable final review, reviewers never run `push-it install`/`glow`).

---

### Task 1: `paint` reference, Windows backend, spec/docs amendment

**Files:**
- Modify: `internal/glow/paint/paint.go`, `internal/glow/paint/paint_test.go`, `internal/glow/glow.go`, `internal/glow/glow_test.go`, `internal/glow/glow_windows.go`, `docs/superpowers/specs/2026-08-20-push-it-design.md`, `docs/glow.md`, `CHANGELOG.md`

**Interfaces (produced; JS and Swift mirror these names in camelCase):**
- `const GlowWidthAt1080 = 96`, `const FalloffExponent = 2.0` (replace `FrameThickness`).
- `func GlowWidth(w, h int) int` - `max(1, round(min(w,h) * GlowWidthAt1080 / 1080))`.
- `func EdgeAlpha(d float64, width int) float64` - `(1 - d/width)^FalloffExponent` for `0 <= d < width`, else 0.
- `func EdgePos(edge Edge, x, y, w, h int) float64` where `type Edge int` with constants `Top, Bottom, Left, Right` - the perimeter position formulas above.
- `func Render(buf []byte, w, h int, elapsed time.Duration)` - full frame; premultiplied BGRA; interior (farther than `W` from every edge) fully transparent.
- `func RenderGlow(buf []byte, w, h int, elapsed time.Duration)` - writes only pixels within `W` of an edge (caller zeroes once); byte-equal to `Render`.
- `HueAt`, `OpacityAt`, `HSVToRGB` unchanged. `PerimeterPos`, `InFrame`, `RenderBand` are removed.

- [ ] **Step 1: Tests first** - rewrite `paint_test.go`:
  - `TestGlowWidthScales`: `GlowWidth(1920,1080) == 96`, `GlowWidth(2560,1440) == 128`, `GlowWidth(3840,2160) == 192`, `GlowWidth(800,600) == 53`, `GlowWidth(10,10) == 1`.
  - `TestEdgeAlphaFalloff`: `EdgeAlpha(0, 96) == 1`, `EdgeAlpha(48, 96) == 0.25` (within 1e-9), `EdgeAlpha(96, 96) == 0`, `EdgeAlpha(200, 96) == 0`, monotone non-increasing over `d = 0..96`.
  - `TestEdgePosClockwise`: for `w=200,h=100`: `EdgePos(Top,0,0,...) == 0`; `EdgePos(Top,199,0) < EdgePos(Right,199,0)`; `EdgePos(Right,199,99) < EdgePos(Bottom,199,99)`; `EdgePos(Bottom,0,99) < EdgePos(Left,0,99)`; `EdgePos(Left,0,0) < 1`.
  - `TestRenderCompositesOverlappingStrips`: `w=200,h=100` (`W = round(100*96/1080) = 9`); alpha at `(100, 0)` (top edge, mid) equals `round(255*OpacityAt(0))`; alpha at `(0, 50)` (left edge, mid) same; alpha at the corner `(0,0)` same (both strips at full alpha compose to 1); alpha at `(4, 50)` equals `round(255 * OpacityAt(0) * EdgeAlpha(4, 9))` within 1; alpha at `(4, 4)` equals `round(255 * (1 - (1-aH)(1-aV)))` with `aH = aV = OpacityAt(0)*EdgeAlpha(4,9)`, within 1; alpha at `(100, 50)` is 0; every pixel premultiplied (no channel > alpha).
  - `TestRenderGlowEqualsRender`: zeroed buffer + `RenderGlow` equals `Render` byte-for-byte for `(200,100)` and `(101,37)` at `elapsed` 0 and `RotationPeriod/3`.
  - `TestRenderAdvancesWithTime` and `TestRenderRejectsShortBuffer` and `TestHSVToRGBPrimaries` and `TestHueAtRotatesOncePerPeriod` and `TestOpacityAtStaysInBounds` kept.
- [ ] **Step 2: Run** - `go test ./internal/glow/paint/` fails (undefined).
- [ ] **Step 3: Implement** in `paint.go`:

```go
const (
	GlowWidthAt1080 = 96
	FalloffExponent = 2.0
	RotationPeriod  = 2 * time.Second
	PulsePeriod     = 600 * time.Millisecond
	MinOpacity      = 0.55
	MaxOpacity      = 1.0
)

type Edge int

const (
	Top Edge = iota
	Bottom
	Left
	Right
)

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

func EdgeAlpha(d float64, width int) float64 {
	if d < 0 || d >= float64(width) {
		return 0
	}
	return math.Pow(1-d/float64(width), FalloffExponent)
}

func EdgePos(e Edge, x, y, w, h int) float64 {
	p := float64(2 * (w + h))
	switch e {
	case Top:
		return float64(x) / p
	case Right:
		return float64(w+y) / p
	case Bottom:
		return float64(w+h+(w-1-x)) / p
	default:
		return float64(2*w+h+(h-1-y)) / p
	}
}

// strip composites one edge's contribution source-over into buf for the
// pixel at i. a is the strip alpha (pulse already applied), r/g/b its colour.
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

// renderPixel composes top, bottom, left, right for (x, y) into a zeroed pixel.
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
```

  Keep `HueAt`, `OpacityAt`, `HSVToRGB` as they are. Rounding note for the corner test: `over` rounds per strip, so the test tolerances are "within 1".
- [ ] **Step 4: glow.go / glow_test.go** - re-export `GlowWidthAt1080` and `FalloffExponent` instead of `FrameThickness`; `TestParamsAreSane` checks `GlowWidthAt1080 > 0 && FalloffExponent >= 1`. Package doc: "a rainbow glow that fades inward from every screen edge and travels counter-clockwise".
- [ ] **Step 5: Windows** - `glow_windows.go`: `paint.RenderBand` -> `paint.RenderGlow`; no other change. `CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build ./... && GOOS=windows go vet ./internal/glow/...`.
- [ ] **Step 6: Spec + docs** - in the spec's Glow section replace "frame thickness 14 px" with "glow width 96 px at 1080p scaled by the shorter screen side, quadratic inward falloff, corners as overlapping glows"; `docs/glow.md` Parameters and the description sentences likewise (it is a glow, not a frame). Keep ASCII.
- [ ] **Step 7: Verify** - `go test -race ./... && mise run lint`, cross builds for darwin/arm64 and windows/arm64.
- [ ] **Step 8: Commit** - CHANGELOG `### Changed`: `- Glow is now a feathered 96 px (at 1080p) inward-fading rainbow with overlapping corners instead of a hard 14 px frame; reference renderer, Windows backend, spec, and docs updated.`

---

### Task 2: GNOME renderer + gjs tests

**Files:**
- Modify: `internal/glow/gnome/ext/glowmath.js`, `internal/glow/gnome/ext/extension.js`, `internal/glow/gnome/tests/testGlowmath.js`, `CHANGELOG.md`

**Interfaces (mirrors of Task 1):** `GLOW_WIDTH_AT_1080 = 96`, `FALLOFF_EXPONENT = 2`, `glowWidth(w, h)`, `edgeAlpha(d, width)`, `edgePos(edge, x, y, w, h)` with `EDGE = {TOP:0, BOTTOM:1, LEFT:2, RIGHT:3}`, `stripGradient(w, h)` now returning four FULL-LENGTH strips `{edge, x, y, sw, sh, x0, y0, x1, y1, p0, p1, nx, ny}` where `(x0,y0)->(x1,y1)` is the hue-gradient line (clockwise start to end, full edge length) and `(nx, ny)` is the inward unit direction for the alpha mask (`(0,1)` top, `(0,-1)` bottom, `(1,0)` left, `(-1,0)` right); `alphaStops(width)` returning `[[offset, alpha], ...]` for 9 stops `k/8` with alpha `(1 - k/8)^2`. `FRAME_THICKNESS`, `perimeterPos`, `inFrame` removed.

- [ ] **Step 1: gjs tests first** (`testGlowmath.js`): constants pin 96 and 2; `glowWidth(1920,1080) == 96`, `(2560,1440) == 128`, `(10,10) == 1`; `edgeAlpha(0,96) == 1`, `edgeAlpha(48,96) ~ 0.25`, `edgeAlpha(96,96) == 0`; `edgePos` clockwise order for `w=200,h=100` as in Task 1; `stripGradient(200,100)` returns 4 strips in order top, bottom, left, right; top `{x:0,y:0,sw:200,sh:9,x0:0,y0:0,x1:200,y1:0,nx:0,ny:1}` with `p0 = 0`, `p1 = 199/600`; bottom spans full width, `x0 > x1`, `ny = -1`; left spans full height (`sh == 100`), `y0 > y1` (bottom to top), `nx = 1`; right `nx = -1`; every strip `p0 < p1`; corner continuity within `1/P` between consecutive strips (top end -> right start, right end -> bottom start, bottom end -> left start); `alphaStops(9)` has 9 entries, first `[0,1]`, last `[1,0]`, offsets increasing, alphas non-increasing. Keep the hue/opacity/hsv/edgeStops suites.
- [ ] **Step 2: Run** - `gjs -m internal/glow/gnome/tests/run.js` fails.
- [ ] **Step 3: glowmath.js** - port Task 1's formulas verbatim (same constant names in SCREAMING_SNAKE, same functions in camelCase); `stripGradient` uses `const W = glowWidth(w, h)` and full-length rects: top `(0,0,w,W)`, bottom `(0,h-W,w,W)`, left `(0,0,W,h)`, right `(w-W,0,W,h)`; gradient lines run the full edge in clockwise direction: top `(0,0)->(w,0)`, right `(w,0)->(w,h)`, bottom `(w,h)->(0,h)`, left `(0,h)->(0,0)`; `p0/p1` from `edgePos` at the two ends (`p1` uses the last pixel: top `edgePos(TOP, w-1, 0)`, etc.).
- [ ] **Step 4: extension.js `_paint`** - for each strip in order: `cr.save(); cr.rectangle(x, y, sw, sh); cr.clip();` source = `Cairo.LinearGradient(x0,y0,x1,y1)` with `edgeStops(p0,p1,elapsedMs)` colour stops; mask = `Cairo.LinearGradient` from the edge point to the inward point at distance `W` (top: `(0,0)->(0,W)`; bottom: `(0,h)->(0,h-W)`; left: `(0,0)->(W,0)`; right: `(w,0)->(w-W,0)`) with `addColorStopRGBA(offset, 0, 0, 0, alpha)` for each `alphaStops` entry; `cr.setSource(hueGrad); cr.mask(alphaGrad); cr.restore();`. Default operator OVER gives the overlapping corners. `area.opacity` pulse stays in the timer callback.
- [ ] **Step 5: Verify** - `mise run glow:gnome:test`, `go build ./...` (embed), `grep -rnP '[^\x00-\x7F]' internal/glow/gnome` empty.
- [ ] **Step 6: Commit** - CHANGELOG: amend the GNOME bullet: "four feathered strips (axial hue gradient masked by an inward alpha falloff)".

---

### Task 3: macOS renderer

**Files:**
- Modify: `internal/glow/macos/glow.swift`, `CHANGELOG.md`

- [ ] **Step 1:** Port the Task 2 `glowmath.js` changes into the Swift file-scope functions (`glowWidthAt1080 = 96`, `falloffExponent = 2.0`, `glowWidth`, `edgeAlpha`, `edgePos`, `stripGradient` returning full-length strips with the inward direction, `alphaStops`).
- [ ] **Step 2:** For each strip (order top, bottom, left, right): a `CAGradientLayer` (`.axial`) with `frame` = the strip rect converted to AppKit y-up (`y_appkit = h - y_down - sh`), `startPoint`/`endPoint` along the edge in the clockwise direction in unit coordinates (top `(0,0.5)->(1,0.5)`, right `(0.5,1)->(0.5,0)`, bottom `(1,0.5)->(0,0.5)`, left `(0.5,0)->(0.5,1)`), 16 `locations` and `colors` from `edgeStops(p0, p1, elapsedMs)` updated by the existing 30 fps timer; and a `mask` = another `CAGradientLayer` (`.axial`, same bounds) whose `colors` are black with alpha from `alphaStops` and whose start/end run from the edge inward in unit coordinates (top, y-up: `(0.5,1)->(0.5,0)`; bottom: `(0.5,0)->(0.5,1)`; left: `(0,0.5)->(1,0.5)`; right: `(1,0.5)->(0,0.5)`). Sublayer order top, bottom, left, right. `contentsScale` on every layer. Opacity pulse animation unchanged.
- [ ] **Step 3:** Verify `CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build ./...` (Swift compiles only on the macOS CI job); read the Swift twice against glowmath.js.
- [ ] **Step 4: Commit** - CHANGELOG: amend the macOS bullet the same way.

---

## Self-review

Spec coverage: new parameters and composition rule stated once in Global Constraints and implemented identically in T1 (Go, also Windows via `RenderGlow`), T2 (Cairo mask), T3 (CA mask); the spec and docs are amended in T1. Type consistency: `Edge`/`EDGE` constants and the four strip rects/directions are defined the same way in all three tasks; `RenderGlow` replaces `RenderBand` at its one call site. Placeholders: none.
