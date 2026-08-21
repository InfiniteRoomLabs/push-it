# Screen glow

An animated rainbow glow that fades inward from the screen edge, shown for exactly as long as the clip plays (the clip is decoded before playback, so its length is known up front). It never captures input.

All three renderers mirror `internal/glow/paint` (the reference renderer): the GNOME extension and the macOS helper each port `glowmath.js`'s math (perimeter position, hue rotation, opacity pulse) to their own runtime, and the Windows backend calls the `paint` package directly.

## Status

| Platform | Backend | Status |
|---|---|---|
| Linux / GNOME 46+ | Shell extension, triggered over D-Bus | shipped (extension extracted and enabled by `install --glow`; log out and back in once - Wayland cannot hot-load extensions); `push-it install --glow` refuses clearly on non-GNOME sessions |
| macOS | helper app (Cocoa / Core Animation), universal binary | shipped in release binaries built with `-tags glowhelper`; a plain `go install`/`go build` (no build tag) prints "built without -tags glowhelper" instead of drawing anything |
| Windows | in-process layered window | shipped, visual verification pending |
| Linux / KDE, wlroots, X11 | - | not supported - `install --glow` refuses with a clear message and the hook logs an error |

`push-it doctor` shows `glow: enabled=... backend=gnome|macos|windows|none`.

## Parameters

Glow width 96 px at 1080p, scaled by the shorter screen side, fading inward with a quadratic falloff; corners are rendered as two overlapping glows. One full rainbow rotation every 2 s, opacity pulsing between 0.55 and 1.0 every 0.6 s. The constants are defined once in `internal/glow/paint/paint.go` and re-exported under the same names by `internal/glow/glow.go`; every renderer (GNOME, macOS, Windows) is mirrored from that single source.

## How it is triggered

The hook decodes the clip first, so it knows the clip's exact duration before anything happens on screen. It then calls the backend with that exact duration, so the glow runs for precisely as long as the clip plays - not a fixed guess.

## Control

- `push-it glow --duration 3.5s` shows it on demand, on any platform with a backend compiled in.
- `push-it doctor` shows `backend=gnome|macos|windows` (or `none` where nothing is compiled in for this platform).
- `NO_GLOW=1 git push` skips it once.
