# Screen glow

An animated rainbow frame around the screen edge, shown for exactly as long as the clip plays (the clip is decoded before playback, so its length is known up front). It never captures input.

## Status

| Platform | Backend | Status |
|---|---|---|
| Linux / GNOME 45+ | Shell extension, triggered over D-Bus | planned (next release) |
| macOS | helper app (Cocoa / Core Animation), universal binary | planned |
| Windows | in-process layered window | planned |
| Linux / KDE, wlroots, X11 | - | not planned for v1; glow is a silent no-op |

This release ships the parameters and the hook points only; `push-it doctor` shows `glow: backend=none`.

## Parameters

Frame thickness 14 px, one full rainbow rotation every 2 s, opacity pulsing between 0.55 and 1.0 every 0.6 s. They live in `internal/glow/glow.go` and are mirrored in each renderer.

## Control

- `push-it glow --duration 3.5` shows it on demand.
- `NO_GLOW=1 git push` skips it once.
