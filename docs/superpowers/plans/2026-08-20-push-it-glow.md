# push-it Glow Backends Implementation Plan (Plan 2 of 3)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `push-it glow` and the pre-push hook actually draw the animated rainbow frame on GNOME (Linux), macOS, and Windows, installed and removed by `push-it install --glow` / `uninstall`, with the same look on all three.

**Architecture:** `internal/glow` keeps its Plan 1 contract (package-level `Run`, `Install`, `Uninstall`, `Backend` overwritten in `init()` by build-tagged files) with one amendment: `Install` returns a user-facing note. A pure-Go `internal/glow/paint` package is the reference renderer (used directly by Windows, mirrored by the JS and Swift renderers, and the thing the cross-platform tests pin down). GNOME gets a Shell extension embedded in the binary and triggered over D-Bus; macOS gets a Swift helper built by CI and embedded behind a `glowhelper` build tag; Windows draws in-process through a layered window.

**Tech Stack:** Go 1.26 (`embed`, `os/exec`, `image`-free byte buffers), `golang.org/x/sys/windows` + `syscall.NewCallback` for Win32, GNOME Shell 46+ ESM extension (St, Cairo, Gio D-Bus), Swift 5 (AppKit, Core Animation), gjs for extension tests, GitHub Actions (`macos-latest` swiftc + lipo).

**Spec:** `docs/superpowers/specs/2026-08-20-push-it-design.md` (Glow section). Plan 1: `docs/superpowers/plans/2026-08-20-push-it-core.md`.

## Global Constraints

- Module `github.com/InfiniteRoomLabs/push-it`, Go `1.26.5`; `CGO_ENABLED=0` must build for linux/darwin/windows x amd64/arm64 with NO extra build tags (the macOS helper embed is opt-in via `-tags glowhelper`).
- No new Go dependencies. Shell-outs limited to `gdbus` and `gnome-extensions` (Linux) and the embedded helper (macOS).
- Visual parameters are the constants in `internal/glow/glow.go` (`FrameThickness = 14`, `RotationPeriod = 2s`, `PulsePeriod = 600ms`, `MinOpacity = 0.55`, `MaxOpacity = 1.0`, `DefaultDuration = 3500ms`); the JS and Swift renderers copy the same numbers with a comment pointing back.
- Glow never captures input and never blocks a push: every backend error is returned to the caller (the hook logs it; `push-it glow` prints it) and never panics.
- `Run(ctx, d)` blocks for `d` (or until `ctx` is done) on every backend so `hook.Run`'s WaitGroup keeps the glow alive for the clip.
- Contract amendment (this plan): `Install func(st *config.InstallState) (note string, err error)`; `note` is printed by `cmd/push-it install` when non-empty. `Uninstall` keeps `func(st *config.InstallState) error`. Both must tolerate a nil `st` only by returning an error, never by panicking.
- Spec layout amendment: extension and helper sources live under `internal/glow/gnome/ext/` and `internal/glow/macos/` (not top-level `glow/`) because `go:embed` cannot cross package directories.
- Repository is public: no personal paths, IPs, hostnames. Every commit includes a `CHANGELOG.md` entry under `## [Unreleased]`; markdown/JS/Swift/YAML use ASCII punctuation only; markdown prose is never hard-wrapped.
- Commit discipline from Plan 1: `git add <files>` then a separate `git commit`; trailer `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`.
- Review tiers: Sonnet implementers reviewed by Opus; Task 4 (Win32) is judgment-heavy and uses an Opus implementer reviewed by Fable.
- Reviewers never run `push-it install`/`uninstall`/`glow` outside a temp `PUSH_IT_CONFIG_DIR`, `XDG_DATA_HOME`, `GIT_CONFIG_GLOBAL`, and `HOME`.

---

## File structure

```
internal/glow/glow.go                     contract (Install signature amended), constants, Available()
internal/glow/glow_test.go                stub test updated for the new Install signature
internal/glow/paint/paint.go              Render(buf, w, h, elapsed), HueAt, OpacityAt, HSVToRGB, PerimeterPos
internal/glow/paint/paint_test.go
internal/glow/gnome/gnome.go              package gnome: UUID, embedded FS of ext/, Dir()
internal/glow/gnome/ext/metadata.json
internal/glow/gnome/ext/extension.js      D-Bus export + St.DrawingArea renderer
internal/glow/gnome/ext/glowmath.js       mirror of paint math (pure functions)
internal/glow/gnome/tests/testHarness.js  gjs harness (from claude-usage-gnome)
internal/glow/gnome/tests/testGlowmath.js
internal/glow/gnome/tests/run.js          imports the test files, calls summary()
internal/glow/glow_linux.go               //go:build linux - gdbus backend, extension install/uninstall
internal/glow/glow_linux_test.go
internal/glow/macos/glow.swift            helper source
internal/glow/macos/build.sh              swiftc x2 + lipo + --dry-run check (used by CI and locally)
internal/glow/macos/bin/.gitkeep          CI drops glow-macos here; the binary itself is gitignored
internal/glow/glow_darwin.go              //go:build darwin - Run/Install/Uninstall using the extracted helper
internal/glow/glow_darwin_embed.go        //go:build darwin && glowhelper - go:embed macos/bin/glow-macos
internal/glow/glow_darwin_noembed.go      //go:build darwin && !glowhelper - Install returns a "built without helper" error
internal/glow/glow_darwin_test.go
internal/glow/glow_windows.go             //go:build windows - layered window renderer
internal/glow/win32_windows.go            //go:build windows - LazyDLL procs and structs
cmd/push-it/install.go                    print the install note; doctor shows the backend's install state
cmd/push-it/main_test.go                  install note test
docs/glow.md / docs/install.md / README.md / CHANGELOG.md
.github/workflows/ci.yml                  gjs test job, macOS helper job (builds + tests darwin with -tags glowhelper)
mise.toml                                 tasks glow:gnome:test, glow:macos:build
.gitignore                                internal/glow/macos/bin/glow-macos
```

---

### Task 1: Contract amendment + `paint` reference renderer

**Files:**
- Modify: `internal/glow/glow.go`, `internal/glow/glow_test.go`, `cmd/push-it/install.go`, `cmd/push-it/main_test.go`, `CHANGELOG.md`
- Create: `internal/glow/paint/paint.go`, `internal/glow/paint/paint_test.go`

**Interfaces:**
- Produces: `glow.Install func(st *config.InstallState) (string, error)`; `paint.Render(buf []byte, w, h int, elapsed time.Duration)` (premultiplied BGRA, `len(buf) == w*h*4`, interior fully transparent); `paint.HueAt(p float64, elapsed time.Duration) float64` in `[0,1)`; `paint.OpacityAt(elapsed time.Duration) float64` in `[MinOpacity, MaxOpacity]`; `paint.HSVToRGB(h float64) (r, g, b uint8)` (s = v = 1); `paint.PerimeterPos(x, y, w, h int) float64` in `[0,1)` clockwise from the top-left corner.

- [ ] **Step 1: Amend the contract and its stub test**

`internal/glow/glow.go` - replace the `Install` var and comment:

```go
// Install puts any platform pieces in place (GNOME extension, macOS helper).
// It returns a user-facing note (for example "log out and back in") that the
// installer prints when non-empty. A nil st is an error, never a panic.
var Install = func(st *config.InstallState) (string, error) { return "", nil }
```

`internal/glow/glow_test.go` - update `TestStubBackendIsNoop`:

```go
	note, err := Install(nil)
	if err != nil || note != "" || Uninstall(nil) != nil {
		t.Fatal("stub install/uninstall must be no-ops")
	}
```

- [ ] **Step 2: Print the note in the installer**

`cmd/push-it/install.go` - the existing glow call becomes:

```go
	if cfg.Glow.Enabled {
		note, err := glow.Install(&cfg.InstallState)
		if err != nil {
			fmt.Fprintf(stderr, "push-it: glow: %v\n", err)
		} else if note != "" {
			fmt.Fprintf(stdout, "glow: %s\n", note)
		}
	}
```

(The rollback path that calls `glow.Uninstall` is unchanged.) Add to `cmd/push-it/main_test.go`:

```go
func TestInstallPrintsGlowNote(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	tmp := t.TempDir()
	t.Setenv("PUSH_IT_CONFIG_DIR", filepath.Join(tmp, "cfg"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(tmp, "gitconfig"))
	t.Setenv("HOME", tmp)
	orig := glow.Install
	glow.Install = func(*config.InstallState) (string, error) { return "log out and back in", nil }
	t.Cleanup(func() { glow.Install = orig })
	var out, errOut bytes.Buffer
	if code := run([]string{"install", "--glow", "--yes"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "glow: log out and back in") {
		t.Fatalf("note not printed:\n%s", out.String())
	}
}
```

(Import `github.com/InfiniteRoomLabs/push-it/internal/config` and `.../internal/glow` in the test file.)

- [ ] **Step 3: Write the failing paint tests**

`internal/glow/paint/paint_test.go`:

```go
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
```

- [ ] **Step 4: Run to verify failure** - `go test ./internal/glow/...` -> FAIL undefined.

- [ ] **Step 5: Implement paint.go**

```go
// Package paint is the reference renderer for the glow frame: a rainbow
// that travels clockwise around the screen edge while its opacity pulses.
// The Windows backend uses it directly; the GNOME (JS) and macOS (Swift)
// renderers mirror HueAt/OpacityAt/PerimeterPos exactly.
package paint

import (
	"math"
	"time"

	"github.com/InfiniteRoomLabs/push-it/internal/glow"
)

// PerimeterPos maps a pixel to its position in [0,1) along the screen
// perimeter, clockwise from the top-left corner. Pixels are assigned to the
// nearest edge; corners belong to the top/bottom bands.
func PerimeterPos(x, y, w, h int) float64 {
	p := float64(2 * (w + h))
	t := glow.FrameThickness
	switch {
	case y < t:
		return float64(x) / p
	case y >= h-t:
		return float64(w+h+(w-1-x)) / p
	case x >= w-t:
		return float64(w+y) / p
	default: // left band
		return float64(2*w+h+(h-1-y)) / p
	}
}

// HueAt is the hue in [0,1) at perimeter position p after elapsed time.
func HueAt(p float64, elapsed time.Duration) float64 {
	h := p + elapsed.Seconds()/glow.RotationPeriod.Seconds()
	return h - math.Floor(h)
}

// OpacityAt pulses sinusoidally between MinOpacity and MaxOpacity.
func OpacityAt(elapsed time.Duration) float64 {
	phase := 2 * math.Pi * elapsed.Seconds() / glow.PulsePeriod.Seconds()
	return glow.MinOpacity + (glow.MaxOpacity-glow.MinOpacity)*(0.5+0.5*math.Sin(phase))
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
	t := glow.FrameThickness
	alpha := OpacityAt(elapsed)
	a8 := uint8(math.Round(255 * alpha))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 4
			if x >= t && x < w-t && y >= t && y < h-t {
				buf[i], buf[i+1], buf[i+2], buf[i+3] = 0, 0, 0, 0
				continue
			}
			r, g, b := HSVToRGB(HueAt(PerimeterPos(x, y, w, h), elapsed))
			buf[i] = uint8(math.Round(float64(b) * alpha))
			buf[i+1] = uint8(math.Round(float64(g) * alpha))
			buf[i+2] = uint8(math.Round(float64(r) * alpha))
			buf[i+3] = a8
		}
	}
}
```

- [ ] **Step 6: Run everything** - `go test ./... && mise run lint` -> PASS.

- [ ] **Step 7: Commit**

CHANGELOG `### Added`: `- \`internal/glow/paint\`: reference renderer for the rainbow frame (perimeter position, hue rotation, opacity pulse, premultiplied BGRA); \`glow.Install\` now returns a user-facing note.`

```bash
git add internal/glow cmd/push-it/install.go cmd/push-it/main_test.go CHANGELOG.md
git commit -m "feat(glow): paint reference renderer; Install returns a note

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: GNOME Shell extension + gjs tests

**Files:**
- Create: `internal/glow/gnome/gnome.go`, `internal/glow/gnome/ext/metadata.json`, `internal/glow/gnome/ext/extension.js`, `internal/glow/gnome/ext/glowmath.js`, `internal/glow/gnome/tests/testHarness.js`, `internal/glow/gnome/tests/testGlowmath.js`, `internal/glow/gnome/tests/run.js`
- Modify: `mise.toml`, `CHANGELOG.md`

**Interfaces:**
- Produces: `gnome.UUID = "pushit-glow@infiniteroomlabs.com"`; `gnome.FS embed.FS` rooted at `ext`; `gnome.BusName = "org.gnome.Shell"`, `gnome.ObjectPath = "/com/infiniteroomlabs/PushItGlow"`, `gnome.Interface = "com.infiniteroomlabs.PushItGlow"`; D-Bus methods `Start(d: double)` and `Stop()`.

- [ ] **Step 1: gnome.go**

```go
// Package gnome embeds the push-it GNOME Shell extension.
package gnome

import "embed"

const (
	UUID       = "pushit-glow@infiniteroomlabs.com"
	BusName    = "org.gnome.Shell"
	ObjectPath = "/com/infiniteroomlabs/PushItGlow"
	Interface  = "com.infiniteroomlabs.PushItGlow"
)

// FS holds the extension sources under "ext/".
//
//go:embed ext
var FS embed.FS
```

- [ ] **Step 2: metadata.json**

```json
{
  "name": "push-it glow",
  "description": "Animated rainbow frame around the screen, triggered over D-Bus by push-it on git push.",
  "uuid": "pushit-glow@infiniteroomlabs.com",
  "shell-version": ["46", "47", "48"],
  "url": "https://github.com/InfiniteRoomLabs/push-it",
  "version": 1,
  "version-name": "0.1.0"
}
```

- [ ] **Step 3: glowmath.js (pure functions, mirrors internal/glow/paint)**

```js
// Mirrors internal/glow/glow.go constants and internal/glow/paint math.
// Keep the numbers identical to the Go source.
export const FRAME_THICKNESS = 14;      // px
export const ROTATION_PERIOD_MS = 2000; // one full trip around the frame
export const PULSE_PERIOD_MS = 600;
export const MIN_OPACITY = 0.55;
export const MAX_OPACITY = 1.0;
export const DEFAULT_DURATION_S = 3.5;

export function perimeterPos(x, y, w, h) {
    const p = 2 * (w + h);
    const t = FRAME_THICKNESS;
    if (y < t) return x / p;
    if (y >= h - t) return (w + h + (w - 1 - x)) / p;
    if (x >= w - t) return (w + y) / p;
    return (2 * w + h + (h - 1 - y)) / p;
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
```

- [ ] **Step 4: extension.js**

```js
import Gio from 'gi://Gio';
import GLib from 'gi://GLib';
import St from 'gi://St';
import Cairo from 'cairo';
import * as Main from 'resource:///org/gnome/shell/ui/main.js';
import { Extension } from 'resource:///org/gnome/shell/extensions/extension.js';
import { FRAME_THICKNESS, DEFAULT_DURATION_S, opacityAt, edgeStops, perimeterPos } from './glowmath.js';

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

    // D-Bus: show the glow for `seconds`; calling again extends the deadline.
    Start(seconds) {
        const s = Number.isFinite(seconds) && seconds > 0 ? seconds : DEFAULT_DURATION_S;
        const now = GLib.get_monotonic_time();
        this._endUs = now + Math.round(s * 1e6);
        if (this._area) return;
        this._startUs = now;
        const m = Main.layoutManager.primaryMonitor;
        this._area = new St.DrawingArea({ reactive: false, x: m.x, y: m.y, width: m.width, height: m.height });
        this._area.connect('repaint', a => this._paint(a));
        Main.layoutManager.addTopChrome(this._area, { affectsInputRegion: false, affectsStruts: false, trackFullscreen: false });
        this._timer = GLib.timeout_add(GLib.PRIORITY_DEFAULT, FRAME_MS, () => {
            if (GLib.get_monotonic_time() >= this._endUs) { this.Stop(); return GLib.SOURCE_REMOVE; }
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
        const t = FRAME_THICKNESS;
        area.opacity = Math.round(255 * opacityAt(elapsedMs));
        // Four strips, each a linear gradient along its length.
        const strips = [
            [0, 0, w, t, perimeterPos(0, 0, w, h), perimeterPos(w - 1, 0, w, h), true],              // top: left -> right
            [w - t, t, t, h - 2 * t, perimeterPos(w - 1, t, w, h), perimeterPos(w - 1, h - t - 1, w, h), false], // right: top -> bottom
            [0, h - t, w, t, perimeterPos(w - 1, h - 1, w, h), perimeterPos(0, h - 1, w, h), true],     // bottom: right -> left
            [0, t, t, h - 2 * t, perimeterPos(0, h - t - 1, w, h), perimeterPos(0, t, w, h), false],    // left: bottom -> top
        ];
        for (const [x, y, sw, sh, p0, p1, horizontal] of strips) {
            let grad;
            if (horizontal) grad = new Cairo.LinearGradient(p0 <= p1 ? x : x + sw, y, p0 <= p1 ? x + sw : x, y);
            else grad = new Cairo.LinearGradient(x, p0 <= p1 ? y : y + sh, x, p0 <= p1 ? y + sh : y);
            const lo = Math.min(p0, p1), hi = Math.max(p0, p1);
            for (const [off, r, g, b] of edgeStops(lo, hi, elapsedMs)) grad.addColorStopRGBA(off, r, g, b, 1);
            cr.setSource(grad);
            cr.rectangle(x, y, sw, sh);
            cr.fill();
        }
        cr.$dispose();
    }
}
```

- [ ] **Step 5: gjs tests**

`tests/testHarness.js` - copy verbatim from `claude-usage-gnome/tests/testHarness.js` (assert, assertApprox, assertEqual, suite, summary).

`tests/testGlowmath.js`:

```js
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

suite('perimeterPos', () => {
    const w = 200, h = 100;
    const top = M.perimeterPos(50, 0, w, h), right = M.perimeterPos(w - 1, 50, w, h);
    const bottom = M.perimeterPos(150, h - 1, w, h), left = M.perimeterPos(0, 50, w, h);
    assert(top < right && right < bottom && bottom < left && left < 1, 'clockwise order');
    assertEqual(M.perimeterPos(0, 0, w, h), 0, 'top-left is 0');
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
```

`tests/run.js`:

```js
import './testGlowmath.js';
import { summary } from './testHarness.js';
summary();
```

Run: `gjs -m internal/glow/gnome/tests/run.js` -> `=== N passed, 0 failed ===`. Add to `mise.toml`:

```toml
[tasks."glow:gnome:test"]
run = "gjs -m internal/glow/gnome/tests/run.js"
```

- [ ] **Step 6: Verify and commit**

`go build ./... && go test ./internal/glow/... && mise run lint && mise run glow:gnome:test` -> PASS. Confirm `grep -nP '[^\x00-\x7F]' internal/glow/gnome -r` is empty.

CHANGELOG `### Added`: `- GNOME Shell extension \`pushit-glow@infiniteroomlabs.com\` (embedded in the binary): D-Bus \`Start(seconds)\`/\`Stop\`, click-through rainbow frame on the primary monitor; gjs unit tests for the shared math.`

```bash
git add internal/glow/gnome mise.toml CHANGELOG.md
git commit -m "feat(glow): GNOME Shell extension and gjs tests

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: Linux backend - gdbus trigger, extension install/uninstall

**Files:**
- Create: `internal/glow/glow_linux.go`, `internal/glow/glow_linux_test.go`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Consumes: `gnome.FS/UUID/BusName/ObjectPath/Interface`, `config.Dir()` not needed (extension dir is under `XDG_DATA_HOME`).
- Produces (linux only, set in `init()`): `Backend = "gnome"`; `Run` calls `gdbus` then blocks for `d`; `Install` extracts the extension and enables it, returns the logout note; `Uninstall` disables and removes it. Seams for tests: `var runCommand = func(ctx context.Context, name string, args ...string) ([]byte, error)` and `var lookPath = exec.LookPath`. `func extensionDir() (string, error)` = `$XDG_DATA_HOME` or `~/.local/share` + `/gnome-shell/extensions/<UUID>`.

- [ ] **Step 1: Write the failing tests**

```go
//go:build linux

package glow

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/InfiniteRoomLabs/push-it/internal/config"
	"github.com/InfiniteRoomLabs/push-it/internal/glow/gnome"
)

type call struct {
	name string
	args []string
}

func stubExec(t *testing.T, out []byte, err error) *[]call {
	t.Helper()
	var calls []call
	origRun, origLook := runCommand, lookPath
	runCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, call{name, args})
		return out, err
	}
	lookPath = func(string) (string, error) { return "/usr/bin/stub", nil }
	t.Cleanup(func() { runCommand, lookPath = origRun, origLook })
	return &calls
}

func TestBackendIsGnome(t *testing.T) {
	if Backend != "gnome" || !Available() {
		t.Fatalf("Backend = %q", Backend)
	}
}

func TestRunCallsGdbusAndBlocksForDuration(t *testing.T) {
	calls := stubExec(t, []byte("()\n"), nil)
	start := time.Now()
	if err := Run(context.Background(), 80*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if el := time.Since(start); el < 70*time.Millisecond {
		t.Fatalf("Run returned after %v, should block for the duration", el)
	}
	c := (*calls)[0]
	want := []string{"call", "--session", "--dest", gnome.BusName, "--object-path", gnome.ObjectPath, "--method", gnome.Interface + ".Start", "0.080"}
	if c.name != "gdbus" || strings.Join(c.args, " ") != strings.Join(want, " ") {
		t.Fatalf("gdbus args = %v", c.args)
	}
}

func TestRunReturnsErrorWhenGdbusFails(t *testing.T) {
	stubExec(t, []byte("Error: GDBus.Error:org.freedesktop.DBus.Error.UnknownMethod"), errors.New("exit status 1"))
	err := Run(context.Background(), 10*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "extension") {
		t.Fatalf("err = %v, want a hint about the extension", err)
	}
}

func TestRunStopsOnContextCancel(t *testing.T) {
	stubExec(t, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(20 * time.Millisecond); cancel() }()
	start := time.Now()
	_ = Run(ctx, 5*time.Second)
	if time.Since(start) > time.Second {
		t.Fatal("Run did not stop on cancel")
	}
}

func TestInstallExtractsEnablesAndNotes(t *testing.T) {
	calls := stubExec(t, nil, nil)
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	var st config.InstallState
	note, err := Install(&st)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(data, "gnome-shell", "extensions", gnome.UUID)
	for _, f := range []string{"metadata.json", "extension.js", "glowmath.js"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatalf("missing %s: %v", f, err)
		}
	}
	if !st.GnomeExtensionInstalled {
		t.Fatal("state not recorded")
	}
	if !strings.Contains(strings.ToLower(note), "log out") {
		t.Fatalf("note = %q", note)
	}
	last := (*calls)[len(*calls)-1]
	if last.name != "gnome-extensions" || last.args[0] != "enable" || last.args[1] != gnome.UUID {
		t.Fatalf("enable call = %+v", last)
	}
}

func TestInstallIsIdempotentAndUninstallReverses(t *testing.T) {
	calls := stubExec(t, nil, nil)
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	var st config.InstallState
	if _, err := Install(&st); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(&st); err != nil {
		t.Fatal(err)
	}
	if err := Uninstall(&st); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(data, "gnome-shell", "extensions", gnome.UUID)); !os.IsNotExist(err) {
		t.Fatal("extension dir should be removed")
	}
	if st.GnomeExtensionInstalled {
		t.Fatal("state not cleared")
	}
	last := (*calls)[len(*calls)-1]
	if last.name != "gnome-extensions" || last.args[0] != "disable" {
		t.Fatalf("disable call = %+v", last)
	}
}

func TestInstallWithoutGnomeExtensionsCLIStillExtracts(t *testing.T) {
	stubExec(t, nil, nil)
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	var st config.InstallState
	note, err := Install(&st)
	if err != nil || !strings.Contains(note, "gnome-extensions enable") {
		t.Fatalf("note=%q err=%v", note, err)
	}
}

func TestNilStateIsAnError(t *testing.T) {
	if _, err := Install(nil); err == nil {
		t.Fatal("Install(nil) must error")
	}
	if err := Uninstall(nil); err == nil {
		t.Fatal("Uninstall(nil) must error")
	}
}
```

- [ ] **Step 2: Run to verify failure** - `go test ./internal/glow/` -> FAIL.

- [ ] **Step 3: Implement glow_linux.go**

```go
//go:build linux

package glow

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/InfiniteRoomLabs/push-it/internal/config"
	"github.com/InfiniteRoomLabs/push-it/internal/glow/gnome"
)

// Seams for tests.
var (
	runCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return exec.CommandContext(ctx, name, args...).CombinedOutput()
	}
	lookPath = exec.LookPath
)

func init() {
	Backend = "gnome"
	Run = runGnome
	Install = installGnome
	Uninstall = uninstallGnome
}

func extensionDir() (string, error) {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "gnome-shell", "extensions", gnome.UUID), nil
}

// runGnome asks the Shell extension to start the glow, then blocks for d so
// the caller's WaitGroup keeps the glow alive for the clip.
func runGnome(ctx context.Context, d time.Duration) error {
	if _, err := lookPath("gdbus"); err != nil {
		return errors.New("glow: gdbus not found (is this a GNOME session?)")
	}
	out, err := runCommand(ctx, "gdbus", "call", "--session",
		"--dest", gnome.BusName, "--object-path", gnome.ObjectPath,
		"--method", gnome.Interface+".Start", fmt.Sprintf("%.3f", d.Seconds()))
	if err != nil {
		return fmt.Errorf("glow: gnome extension did not answer (%s); is it installed and enabled? run `push-it install --glow` and log out/in: %w", strings.TrimSpace(string(out)), err)
	}
	select {
	case <-ctx.Done():
		_, _ = runCommand(context.Background(), "gdbus", "call", "--session",
			"--dest", gnome.BusName, "--object-path", gnome.ObjectPath,
			"--method", gnome.Interface+".Stop")
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

func installGnome(st *config.InstallState) (string, error) {
	if st == nil {
		return "", errors.New("glow: nil install state")
	}
	dir, err := extensionDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	err = fs.WalkDir(gnome.FS, "ext", func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return walkErr
		}
		data, err := gnome.FS.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel("ext", p)
		return os.WriteFile(filepath.Join(dir, rel), data, 0o644)
	})
	if err != nil {
		return "", fmt.Errorf("glow: extract extension: %w", err)
	}
	st.GnomeExtensionInstalled = true
	if _, err := lookPath("gnome-extensions"); err != nil {
		return fmt.Sprintf("extension extracted to %s; run `gnome-extensions enable %s`, then log out and back in (Wayland cannot hot-load extensions)", dir, gnome.UUID), nil
	}
	if out, err := runCommand(context.Background(), "gnome-extensions", "enable", gnome.UUID); err != nil {
		return fmt.Sprintf("extension extracted to %s but `gnome-extensions enable` failed (%s); log out and back in, then enable it in the Extensions app", dir, strings.TrimSpace(string(out))), nil
	}
	return "GNOME extension installed and enabled; log out and back in once so the Shell loads it (Wayland cannot hot-load extensions)", nil
}

func uninstallGnome(st *config.InstallState) error {
	if st == nil {
		return errors.New("glow: nil install state")
	}
	if !st.GnomeExtensionInstalled {
		return nil
	}
	if _, err := lookPath("gnome-extensions"); err == nil {
		_, _ = runCommand(context.Background(), "gnome-extensions", "disable", gnome.UUID)
	}
	dir, err := extensionDir()
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	st.GnomeExtensionInstalled = false
	return nil
}
```

Note: the test `TestInstallIsIdempotentAndUninstallReverses` expects the last call to be `disable`; the implementation issues `disable` before `RemoveAll`, which is the last command, so the assertion holds.

- [ ] **Step 4: Verify** - `go test ./internal/glow/... && mise run lint && CGO_ENABLED=0 GOOS=darwin go build ./... && CGO_ENABLED=0 GOOS=windows go build ./...` -> PASS (the other OSes still get the stub).

- [ ] **Step 5: Commit**

CHANGELOG `### Added`: `- Linux glow backend: \`push-it install --glow\` extracts and enables the GNOME extension; the hook triggers it over D-Bus with the clip's exact duration.`

```bash
git add internal/glow/glow_linux.go internal/glow/glow_linux_test.go CHANGELOG.md
git commit -m "feat(glow): Linux backend via the GNOME extension and gdbus

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 4: Windows backend - layered window (Opus implementer, Fable reviewer)

**Files:**
- Create: `internal/glow/win32_windows.go`, `internal/glow/glow_windows.go`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Consumes: `paint.Render`, `glow` constants.
- Produces (windows only): `Backend = "windows"`; `Run` creates the window on a locked OS thread, renders at ~30 fps for `d`, destroys it; `Install` returns `("", nil)`; `Uninstall` returns nil (nil `st` -> error).

- [ ] **Step 1: win32_windows.go - bindings**

```go
//go:build windows

package glow

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32 = windows.NewLazySystemDLL("user32.dll")
	gdi32  = windows.NewLazySystemDLL("gdi32.dll")

	pRegisterClassExW    = user32.NewProc("RegisterClassExW")
	pCreateWindowExW     = user32.NewProc("CreateWindowExW")
	pDefWindowProcW      = user32.NewProc("DefWindowProcW")
	pDestroyWindow       = user32.NewProc("DestroyWindow")
	pShowWindow          = user32.NewProc("ShowWindow")
	pPeekMessageW        = user32.NewProc("PeekMessageW")
	pTranslateMessage    = user32.NewProc("TranslateMessage")
	pDispatchMessageW    = user32.NewProc("DispatchMessageW")
	pUpdateLayeredWindow = user32.NewProc("UpdateLayeredWindow")
	pGetDC               = user32.NewProc("GetDC")
	pReleaseDC           = user32.NewProc("ReleaseDC")
	pUnregisterClassW    = user32.NewProc("UnregisterClassW")

	pCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	pCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	pSelectObject       = gdi32.NewProc("SelectObject")
	pDeleteObject       = gdi32.NewProc("DeleteObject")
	pDeleteDC           = gdi32.NewProc("DeleteDC")
)

const (
	wsPopup           = 0x80000000
	wsExLayered       = 0x00080000
	wsExTransparent   = 0x00000020
	wsExTopmost       = 0x00000008
	wsExToolWindow    = 0x00000080
	wsExNoActivate    = 0x08000000
	swShowNoActivate  = 4
	pmRemove          = 1
	wmDestroy         = 0x0002
	ulwAlpha          = 2
	acSrcOver         = 0
	acSrcAlpha        = 1
	biRGB             = 0
	dibRGBColors      = 0
	smCxScreen        = 0
	smCyScreen        = 1
)

type wndClassExW struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   windows.Handle
	Icon       windows.Handle
	Cursor     windows.Handle
	Background windows.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSm     windows.Handle
}

type point struct{ X, Y int32 }
type size struct{ CX, CY int32 }

type msg struct {
	HWnd    windows.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

type blendFunction struct {
	BlendOp, BlendFlags, SourceConstantAlpha, AlphaFormat byte
}

type bitmapInfoHeader struct {
	Size          uint32
	Width, Height int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type bitmapInfo struct {
	Header bitmapInfoHeader
	Colors [1]uint32
}

func wndProc(hwnd windows.Handle, m uint32, wp, lp uintptr) uintptr {
	r, _, _ := pDefWindowProcW.Call(uintptr(hwnd), uintptr(m), wp, lp)
	return r
}

var wndProcPtr = syscall.NewCallback(wndProc)

func utf16ptr(s string) *uint16 { p, _ := windows.UTF16PtrFromString(s); return p }

// sizeofInt32 avoids importing unsafe in the renderer file.
const sizeofBlend = unsafe.Sizeof(blendFunction{})
```

- [ ] **Step 2: glow_windows.go - renderer**

```go
//go:build windows

package glow

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/InfiniteRoomLabs/push-it/internal/config"
	"github.com/InfiniteRoomLabs/push-it/internal/glow/paint"
)

const frameInterval = 33 * time.Millisecond

func init() {
	Backend = "windows"
	Run = runWindows
	Install = func(st *config.InstallState) (string, error) {
		if st == nil {
			return "", errors.New("glow: nil install state")
		}
		return "", nil
	}
	Uninstall = func(st *config.InstallState) error {
		if st == nil {
			return errors.New("glow: nil install state")
		}
		return nil
	}
}

// runWindows draws the frame in a click-through layered window on the
// primary monitor for d. All Win32 calls happen on one locked OS thread.
func runWindows(ctx context.Context, d time.Duration) (err error) {
	done := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		done <- renderLoop(ctx, d)
	}()
	return <-done
}

func renderLoop(ctx context.Context, d time.Duration) error {
	w := int(windows.GetSystemMetrics(smCxScreen))
	h := int(windows.GetSystemMetrics(smCyScreen))
	if w <= 0 || h <= 0 {
		return errors.New("glow: no primary display")
	}
	inst, err := windows.GetModuleHandle(nil)
	if err != nil {
		return fmt.Errorf("glow: GetModuleHandle: %w", err)
	}
	className := utf16ptr("PushItGlowWindow")
	wc := wndClassExW{
		Size:      uint32(unsafe.Sizeof(wndClassExW{})),
		WndProc:   wndProcPtr,
		Instance:  inst,
		ClassName: className,
	}
	if atom, _, e := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); atom == 0 {
		if e != windows.ERROR_CLASS_ALREADY_EXISTS {
			return fmt.Errorf("glow: RegisterClassEx: %v", e)
		}
	}
	defer pUnregisterClassW.Call(uintptr(unsafe.Pointer(className)), uintptr(inst))

	hwnd, _, e := pCreateWindowExW.Call(
		wsExLayered|wsExTransparent|wsExTopmost|wsExToolWindow|wsExNoActivate,
		uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(utf16ptr("push-it glow"))),
		wsPopup, 0, 0, uintptr(w), uintptr(h), 0, 0, uintptr(inst), 0)
	if hwnd == 0 {
		return fmt.Errorf("glow: CreateWindowEx: %v", e)
	}
	defer pDestroyWindow.Call(hwnd)

	screenDC, _, _ := pGetDC.Call(0)
	defer pReleaseDC.Call(0, screenDC)
	memDC, _, _ := pCreateCompatibleDC.Call(screenDC)
	defer pDeleteDC.Call(memDC)

	bmi := bitmapInfo{Header: bitmapInfoHeader{
		Size: uint32(unsafe.Sizeof(bitmapInfoHeader{})), Width: int32(w), Height: -int32(h), // top-down
		Planes: 1, BitCount: 32, Compression: biRGB,
	}}
	var bits unsafe.Pointer
	bmp, _, e := pCreateDIBSection.Call(screenDC, uintptr(unsafe.Pointer(&bmi)), dibRGBColors, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bmp == 0 || bits == nil {
		return fmt.Errorf("glow: CreateDIBSection: %v", e)
	}
	defer pDeleteObject.Call(bmp)
	old, _, _ := pSelectObject.Call(memDC, bmp)
	defer pSelectObject.Call(memDC, old)

	buf := unsafe.Slice((*byte)(bits), w*h*4)
	pShowWindow.Call(hwnd, swShowNoActivate)

	start := time.Now()
	deadline := start.Add(d)
	tick := time.NewTicker(frameInterval)
	defer tick.Stop()
	blend := blendFunction{BlendOp: acSrcOver, SourceConstantAlpha: 255, AlphaFormat: acSrcAlpha}
	sz := size{int32(w), int32(h)}
	src := point{0, 0}
	var m msg
	for {
		// Pump pending messages so the window stays responsive to the system.
		for {
			r, _, _ := pPeekMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, pmRemove)
			if r == 0 {
				break
			}
			pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
		}
		paint.Render(buf, w, h, time.Since(start))
		if r, _, e := pUpdateLayeredWindow.Call(hwnd, screenDC, 0, uintptr(unsafe.Pointer(&sz)), memDC,
			uintptr(unsafe.Pointer(&src)), 0, uintptr(unsafe.Pointer(&blend)), ulwAlpha); r == 0 {
			return fmt.Errorf("glow: UpdateLayeredWindow: %v", e)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case now := <-tick.C:
			if now.After(deadline) {
				return nil
			}
		}
	}
}
```

- [ ] **Step 3: Verify what can be verified off-Windows**

`CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./... && GOOS=windows go vet ./internal/glow/ && go test ./... && mise run lint` -> PASS. `windows.ERROR_CLASS_ALREADY_EXISTS` and `windows.GetSystemMetrics` exist in `golang.org/x/sys/windows v0.47.0`; if `go vet` for windows reports either missing, report the exact error (do not invent a replacement). The CI `test (windows-latest)` job compiles this file; visual verification is manual (PUSHIT-3 in Plane).

- [ ] **Step 4: Commit**

CHANGELOG `### Added`: `- Windows glow backend: in-process click-through layered window rendered by the shared \`paint\` package (compiles and vets in CI; visual verification pending).`

```bash
git add internal/glow/win32_windows.go internal/glow/glow_windows.go CHANGELOG.md
git commit -m "feat(glow): Windows layered-window backend

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 5: macOS backend - Swift helper + embed behind `glowhelper`

**Files:**
- Create: `internal/glow/macos/glow.swift`, `internal/glow/macos/build.sh`, `internal/glow/macos/bin/.gitkeep`, `internal/glow/glow_darwin.go`, `internal/glow/glow_darwin_embed.go`, `internal/glow/glow_darwin_noembed.go`, `internal/glow/glow_darwin_test.go`
- Modify: `.gitignore`, `mise.toml`, `CHANGELOG.md`

**Interfaces:**
- Produces (darwin only): `Backend = "macos"`; `Run` execs the extracted helper with `--duration <seconds>` and waits; `Install` extracts the embedded helper to `<config dir>/glow-macos` (0755) when missing or its SHA-256 differs, records `st.MacOSHelperPath`; `Uninstall` removes it. Without `-tags glowhelper`, `Install` returns an error `glow: this build has no macOS helper (built without -tags glowhelper)` and `Run` returns the same error if the helper file is absent. Seam: `var runHelper = func(ctx context.Context, path string, args ...string) error`.

- [ ] **Step 1: glow.swift**

```swift
// push-it glow helper for macOS.
// Mirrors internal/glow/glow.go constants and internal/glow/paint math:
// frame 14 px, rotation 2 s, pulse 600 ms between 0.55 and 1.0.
import AppKit
import QuartzCore

let frameThickness: CGFloat = 14
let rotationPeriod: CFTimeInterval = 2.0
let pulsePeriod: CFTimeInterval = 0.6
let minOpacity: Float = 0.55
let maxOpacity: Float = 1.0
let defaultDuration: CFTimeInterval = 3.5

func parseArgs() -> (duration: CFTimeInterval, dryRun: Bool) {
    var duration = defaultDuration
    var dryRun = false
    var args = CommandLine.arguments.dropFirst().makeIterator()
    while let a = args.next() {
        switch a {
        case "--duration":
            if let v = args.next(), let d = Double(v), d >= 0 { duration = d }
        case "--dry-run":
            dryRun = true
        default:
            break
        }
    }
    return (duration, dryRun)
}

final class GlowWindow: NSWindow {
    override var canBecomeKey: Bool { false }
    override var canBecomeMain: Bool { false }
}

final class App: NSObject, NSApplicationDelegate {
    let duration: CFTimeInterval
    var window: GlowWindow?
    init(duration: CFTimeInterval) { self.duration = duration }

    func applicationDidFinishLaunching(_ n: Notification) {
        guard let screen = NSScreen.main else { NSApp.terminate(nil); return }
        let frame = screen.frame
        let w = GlowWindow(contentRect: frame, styleMask: .borderless, backing: .buffered, defer: false)
        w.level = .screenSaver
        w.isOpaque = false
        w.backgroundColor = .clear
        w.ignoresMouseEvents = true
        w.hasShadow = false
        w.collectionBehavior = [.canJoinAllSpaces, .stationary, .fullScreenAuxiliary, .ignoresCycle]
        let root = CALayer()
        root.frame = CGRect(origin: .zero, size: frame.size)
        w.contentView?.wantsLayer = true
        w.contentView?.layer = root

        // Conic rainbow, masked to a frame band, rotating via the gradient's
        // start angle; the frame is a square centred on the screen so the
        // rotation looks uniform, clipped by the mask to the visible band.
        let side = max(frame.width, frame.height) * 1.5
        let grad = CAGradientLayer()
        grad.type = .conic
        grad.frame = CGRect(x: (frame.width - side) / 2, y: (frame.height - side) / 2, width: side, height: side)
        grad.startPoint = CGPoint(x: 0.5, y: 0.5)
        grad.endPoint = CGPoint(x: 1.0, y: 0.5)
        grad.colors = stride(from: 0, through: 12, by: 1).map { i -> CGColor in
            NSColor(hue: CGFloat(i % 12) / 12, saturation: 1, brightness: 1, alpha: 1).cgColor
        }
        root.addSublayer(grad)

        let mask = CAShapeLayer()
        let path = CGMutablePath()
        path.addRect(CGRect(origin: .zero, size: frame.size))
        path.addRect(CGRect(x: frameThickness, y: frameThickness,
                            width: frame.width - 2 * frameThickness, height: frame.height - 2 * frameThickness))
        mask.path = path
        mask.fillRule = .evenOdd
        root.mask = mask

        let spin = CABasicAnimation(keyPath: "transform.rotation.z")
        spin.fromValue = 0
        spin.toValue = -2 * Double.pi
        spin.duration = rotationPeriod
        spin.repeatCount = .infinity
        grad.add(spin, forKey: "spin")

        let pulse = CABasicAnimation(keyPath: "opacity")
        pulse.fromValue = minOpacity
        pulse.toValue = maxOpacity
        pulse.duration = pulsePeriod / 2
        pulse.autoreverses = true
        pulse.repeatCount = .infinity
        pulse.timingFunction = CAMediaTimingFunction(name: .easeInEaseOut)
        root.add(pulse, forKey: "pulse")

        w.orderFrontRegardless()
        window = w
        DispatchQueue.main.asyncAfter(deadline: .now() + duration) { NSApp.terminate(nil) }
    }
}

let (duration, dryRun) = parseArgs()
if dryRun { exit(0) }
let app = NSApplication.shared
app.setActivationPolicy(.accessory)
let delegate = App(duration: duration)
app.delegate = delegate
app.run()
```

- [ ] **Step 2: build.sh**

```sh
#!/bin/sh
# Builds the universal macOS glow helper into internal/glow/macos/bin/glow-macos.
set -eu
cd "$(dirname "$0")"
mkdir -p bin
swiftc -O -target arm64-apple-macos12 -o bin/glow-macos-arm64 glow.swift
swiftc -O -target x86_64-apple-macos12 -o bin/glow-macos-x86_64 glow.swift
lipo -create -output bin/glow-macos bin/glow-macos-arm64 bin/glow-macos-x86_64
rm -f bin/glow-macos-arm64 bin/glow-macos-x86_64
./bin/glow-macos --dry-run
echo "built bin/glow-macos ($(lipo -archs bin/glow-macos))"
```

`chmod +x build.sh`. `.gitignore` gets `internal/glow/macos/bin/glow-macos`. `mise.toml`: `[tasks."glow:macos:build"] run = "sh internal/glow/macos/build.sh"`.

- [ ] **Step 3: Go side**

`glow_darwin_embed.go`:

```go
//go:build darwin && glowhelper

package glow

import _ "embed"

//go:embed macos/bin/glow-macos
var helperBinary []byte
```

`glow_darwin_noembed.go`:

```go
//go:build darwin && !glowhelper

package glow

// helperBinary is empty in builds made without -tags glowhelper (for example
// a plain `go build` on a machine that never ran internal/glow/macos/build.sh).
var helperBinary []byte
```

`glow_darwin.go`:

```go
//go:build darwin

package glow

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/InfiniteRoomLabs/push-it/internal/config"
)

var errNoHelper = errors.New("glow: this build has no macOS helper (built without -tags glowhelper)")

var runHelper = func(ctx context.Context, path string, args ...string) error {
	cmd := exec.CommandContext(ctx, path, args...)
	return cmd.Run()
}

func init() {
	Backend = "macos"
	Run = runMac
	Install = installMac
	Uninstall = uninstallMac
}

func helperPath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "glow-macos"), nil
}

func runMac(ctx context.Context, d time.Duration) error {
	p, err := helperPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(p); err != nil {
		if len(helperBinary) == 0 {
			return errNoHelper
		}
		return fmt.Errorf("glow: helper not installed; run `push-it install --glow`: %w", err)
	}
	return runHelper(ctx, p, "--duration", fmt.Sprintf("%.3f", d.Seconds()))
}

func installMac(st *config.InstallState) (string, error) {
	if st == nil {
		return "", errors.New("glow: nil install state")
	}
	if len(helperBinary) == 0 {
		return "", errNoHelper
	}
	p, err := helperPath()
	if err != nil {
		return "", err
	}
	want := sha256.Sum256(helperBinary)
	if cur, err := os.ReadFile(p); err == nil && bytes.Equal(want[:], hashOf(cur)) {
		st.MacOSHelperPath = p
		return "", nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return "", err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, helperBinary, 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, p); err != nil {
		return "", err
	}
	st.MacOSHelperPath = p
	return "", nil
}

func hashOf(b []byte) []byte { h := sha256.Sum256(b); return h[:] }

func uninstallMac(st *config.InstallState) error {
	if st == nil {
		return errors.New("glow: nil install state")
	}
	if st.MacOSHelperPath == "" {
		return nil
	}
	if err := os.Remove(st.MacOSHelperPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	st.MacOSHelperPath = ""
	return nil
}
```

`glow_darwin_test.go` (runs on the macOS CI leg; uses a fake helper so no real window is needed):

```go
//go:build darwin

package glow

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/InfiniteRoomLabs/push-it/internal/config"
)

func TestBackendIsMacOS(t *testing.T) {
	if Backend != "macos" {
		t.Fatalf("Backend = %q", Backend)
	}
}

func TestInstallExtractsHelperWhenEmbedded(t *testing.T) {
	orig := helperBinary
	helperBinary = []byte("#!/bin/sh\nexit 0\n")
	t.Cleanup(func() { helperBinary = orig })
	t.Setenv("PUSH_IT_CONFIG_DIR", t.TempDir())
	var st config.InstallState
	if _, err := Install(&st); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(st.MacOSHelperPath)
	if err != nil || fi.Mode().Perm()&0o111 == 0 {
		t.Fatalf("helper not extracted executable: %v", err)
	}
	if _, err := Install(&st); err != nil { // idempotent
		t.Fatal(err)
	}
	if err := Uninstall(&st); err != nil || st.MacOSHelperPath != "" {
		t.Fatalf("uninstall: %v %+v", err, st)
	}
}

func TestInstallWithoutEmbeddedHelperErrors(t *testing.T) {
	orig := helperBinary
	helperBinary = nil
	t.Cleanup(func() { helperBinary = orig })
	t.Setenv("PUSH_IT_CONFIG_DIR", t.TempDir())
	var st config.InstallState
	if _, err := Install(&st); err == nil || !strings.Contains(err.Error(), "glowhelper") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunPassesDuration(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PUSH_IT_CONFIG_DIR", dir)
	_ = os.WriteFile(filepath.Join(dir, "glow-macos"), []byte("stub"), 0o755)
	var got []string
	orig := runHelper
	runHelper = func(_ context.Context, p string, args ...string) error { got = append([]string{p}, args...); return nil }
	t.Cleanup(func() { runHelper = orig })
	if err := Run(context.Background(), 1500*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[1] != "--duration" || got[2] != "1.500" {
		t.Fatalf("args = %v", got)
	}
}
```

- [ ] **Step 4: Verify off-macOS** - `CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build ./... && GOOS=darwin go vet ./internal/glow/ && go test ./... && mise run lint`. On this Linux machine `swiftc` is absent, so `build.sh` is exercised by CI only (Task 6 adds the job).

- [ ] **Step 5: Commit**

CHANGELOG `### Added`: `- macOS glow backend: a Swift helper (universal binary, built in CI, embedded with \`-tags glowhelper\`) draws the frame with Core Animation; \`install --glow\` extracts it next to the config.`

```bash
git add internal/glow/macos internal/glow/glow_darwin.go internal/glow/glow_darwin_embed.go internal/glow/glow_darwin_noembed.go internal/glow/glow_darwin_test.go .gitignore mise.toml CHANGELOG.md
git commit -m "feat(glow): macOS Swift helper backend behind -tags glowhelper

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 6: CI jobs, docs, dogfood on this machine

**Files:**
- Modify: `.github/workflows/ci.yml`, `docs/glow.md`, `docs/install.md`, `README.md`, `CHANGELOG.md`

- [ ] **Step 1: CI**

Add two jobs to `ci.yml`:

```yaml
  gnome-extension:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: sudo apt-get update && sudo apt-get install -y gjs
      - run: gjs -m internal/glow/gnome/tests/run.js

  macos-helper:
    runs-on: macos-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - run: sh internal/glow/macos/build.sh
      - name: build and test darwin with the embedded helper
        env:
          CGO_ENABLED: "0"
        run: |
          go build -tags glowhelper ./cmd/push-it
          go test -tags glowhelper ./internal/glow/...
      - uses: actions/upload-artifact@v4
        with:
          name: glow-macos-universal
          path: internal/glow/macos/bin/glow-macos
```

Validate YAML (`uv run --with pyyaml python -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))"`).

- [ ] **Step 2: Docs**

`docs/glow.md` status table: Linux/GNOME 45+ -> "shipped (Shell extension; log out once after install)"; macOS -> "shipped in release binaries (helper embedded); `go install` builds have no helper"; Windows -> "shipped, visual verification pending". Control section: `push-it glow --duration 3.5s` now works where a backend exists; `doctor` shows `backend=gnome|macos|windows`. Add a "How it is triggered" paragraph: the hook decodes the clip first, then calls the backend with the exact duration.

`docs/install.md`: replace the future-tense glow sentence with: "Glow only: on Linux extracts and enables the GNOME Shell extension (log out and back in once - Wayland cannot hot-load extensions); on macOS extracts the helper app next to the config; on Windows nothing to install."

`README.md`: drop any "glow is planned" wording.

- [ ] **Step 3: Dogfood (this machine, GNOME 46 Wayland)**

```bash
mise run build && install -m 755 bin/push-it ~/.local/bin/push-it
push-it install --glow            # additive: sound + hue stay on; prints the logout note
push-it doctor                    # glow: enabled=true backend=gnome
ls ~/.local/share/gnome-shell/extensions/pushit-glow@infiniteroomlabs.com/
gnome-extensions info pushit-glow@infiniteroomlabs.com
```

Then the operator logs out and back in (cannot be automated), and runs `push-it glow --duration 3s` followed by a real `git push`. Record the result in the CHANGELOG entry for this task.

- [ ] **Step 4: Commit and push**

CHANGELOG `### Added`: `- CI: gjs tests for the GNOME extension and a macOS job that builds the universal helper and tests the darwin build with it embedded.` `### Changed`: `- docs: glow is shipped on GNOME, macOS (release binaries), and Windows (visual verification pending).`

```bash
git add .github/workflows/ci.yml docs/glow.md docs/install.md README.md CHANGELOG.md
git commit -m "ci,docs: glow backends in CI and docs

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

Push to `origin` and `github`, watch CI (all five jobs).

---

## Self-review

**Spec coverage:** GNOME extension + D-Bus Start with exact duration + embed.FS install + logout note (T2, T3); macOS Swift helper, universal via lipo, embedded, extracted on first run/hash change (T5); Windows layered window with the pure-Go paint renderer (T1, T4); shared params in one place mirrored in JS/Swift with tests on the shared math (T1, T2); install/uninstall per platform (T3, T5, T4 no-op); CI for gjs and swiftc (T6); docs (T6). Deviations from the spec, all stated in Global Constraints: sources under `internal/glow/` for `go:embed`; `Install` returns a note; macOS helper behind `-tags glowhelper` so plain `CGO_ENABLED=0` builds never need the binary; the extension also exposes `Stop` (used on context cancel).

**Type consistency:** `Install` signature changed in T1 and used with two return values in T1 (cmd), T3, T4, T5; `paint.Render(buf, w, h, elapsed)` used in T4; `gnome.UUID/BusName/ObjectPath/Interface/FS` defined in T2 and used in T3; `helperBinary` declared in exactly one of the two tagged files; `runCommand`/`lookPath`/`runHelper` seams declared where their tests use them.

**Placeholders:** none. The two "if the API is missing, report it" notes (T4 `windows.ERROR_CLASS_ALREADY_EXISTS`/`GetSystemMetrics`) name the exact symbols and the action.
