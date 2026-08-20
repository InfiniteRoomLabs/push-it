# push-it — design spec

Date: 2026-08-20
Status: approved for planning

## Purpose

`push-it` is a `git pre-push` celebration. When you push, it plays a short "push it" sound clip, runs a rainbow burst on a Philips Hue light, and flashes an animated rainbow frame around the edge of your screen for exactly as long as the clip plays. Each of the three effects is an independent, optional component.

It replaces the current ad-hoc setup (a bash hook with an inline Python heredoc, a separate Hue script, and a clips directory with a JSON manifest) with one cross-platform Go binary, a proper installer/uninstaller, tests, CI, semver releases, and documentation — including how to cut your own clips from a track you own.

## Non-goals

- Shipping any audio. The repository and releases contain zero sound files. Users bring their own legally obtained track and cut clips with the bundled toolkit.
- A GUI or preferences UI of any kind.
- Screen glow on Linux desktops other than GNOME (KDE, wlroots, X11). Those are a silent no-op in v1.
- Multi-monitor glow. v1 draws on the primary display only.
- Publishing the GNOME extension to extensions.gnome.org, or packaging for Homebrew/aqua/winget. Release binaries plus `install.sh` are the distribution channel.

## Platform matrix

| | Sound | Hue | Glow |
|---|---|---|---|
| Linux amd64 / arm64 | yes | yes | GNOME only (Shell extension) |
| macOS amd64 / arm64 | yes | yes | yes (Swift helper, universal binary) |
| Windows amd64 / arm64 | yes (tested in CI, not hand-verified) | yes | yes (in-process Win32 layered window) |

## Architecture

One Go module (`github.com/InfiniteRoomLabs/push-it`), one binary `push-it`, stdlib wherever possible. Dependencies are limited to: `github.com/ebitengine/oto/v3` (audio output, purego-based, no cgo), `github.com/hajimehoshi/go-mp3` (MP3 decode), `golang.org/x/sys` (Windows syscalls).

```
cmd/push-it/                       main: subcommand dispatch via stdlib flag
internal/config/                   load/save ~/.config/push-it/config.json (0600); env overrides
internal/clips/                    list + pick a random *.mp3 | *.wav from the clips dir
internal/player/                   decode (mp3 via go-mp3, wav by hand) → oto; returns duration before playing
internal/hue/                      save light state → hue-wheel burst → restore (net/http + encoding/json)
internal/glow/                     Run(ctx, duration): build-tagged backends
internal/glow/paint/               pure-Go frame renderer (shared params, used by the Windows backend; unit-tested)
internal/hook/                     pre-push orchestration: sound ‖ hue ‖ glow, detached from git
internal/installer/                component selection, hook wiring, extension/helper install, uninstall
glow/gnome/pushit-glow@infiniteroomlabs.com/   GNOME Shell extension (GNOME 45+ ESM); embedded via go:embed
glow/gnome/tests/                  gjs unit tests for the extension's pure math module
glow/macos/                        Swift helper source; built by CI into a universal binary; embedded via go:embed (darwin only)
tools/clipper/                     transcribe.py · cut.py · review.py (uv PEP 723 scripts; dev-time only)
docs/                              install.md · make-your-own-clips.md · hue.md · glow.md · migrating.md
install.sh                         POSIX sh bootstrap: detect OS/arch → download release → run `push-it install "$@"`
.github/workflows/                 ci.yml · release.yml
.goreleaser.yaml                   6 binaries, extension zip, checksums
mise.toml                          pinned dev toolchain (go, goreleaser, staticcheck) and tasks
```

### Commands

| Command | Behaviour |
|---|---|
| `push-it hook pre-push` | What git runs. Reads and discards stdin, checks kill switches, re-executes itself detached (`push-it hook --run`), exits 0 immediately. Never fails or delays a push. |
| `push-it play` | Pick and play one random clip (blocking). |
| `push-it hue` | Run the Hue rainbow burst (blocking). |
| `push-it glow [--duration 3.5]` | Run the screen glow for the given seconds (default 3.5). |
| `push-it install [--sound] [--hue] [--glow] [--all] [--yes]` | Install selected components. With no component flags: interactive yes/no per component. |
| `push-it uninstall [--yes]` | Reverse exactly what install did. |
| `push-it doctor` | Report: config path, enabled components, clips found, audio backend OK, Hue reachable, glow backend available. |
| `push-it version` | Semver + commit. |

All subcommands are stdlib `flag` subcommand sets; no CLI framework.

### Kill switches (environment)

- `NO_PUSH_IT=1` — disable everything.
- `NO_RAINBOW=1` — skip Hue.
- `NO_GLOW=1` — skip glow.
- `NO_SOUND=1` — skip sound.

### Config

`$XDG_CONFIG_HOME/push-it/config.json` (Linux), `~/Library/Application Support/push-it/config.json` (macOS), `%APPDATA%\push-it\config.json` (Windows). Written with mode 0600 because it holds the Hue API key.

```json
{
  "sound": { "enabled": true, "clips_dir": "<data dir>/clips", "volume": 0.7 },
  "hue":   { "enabled": false, "bridge": "192.168.1.2", "key": "...", "light": 1 },
  "glow":  { "enabled": true },
  "install_state": {
    "hooks_path_set_by_us": true,
    "hooks_path": "<config dir>/hooks",
    "pre_push_line_appended_to": "",
    "gnome_extension_installed": false,
    "macos_helper_path": ""
  }
}
```

Environment overrides for the Hue fields (`PUSH_IT_HUE_BRIDGE`, `PUSH_IT_HUE_KEY`, `PUSH_IT_HUE_LIGHT`) so the key can come from a secret manager instead of the file.

## Components

### Sound

- Clips dir contains `*.mp3` and/or `*.wav`; no manifest. The clip's label for log output is its filename.
- Pick: uniform random over the listed files (`math/rand/v2`).
- Decode fully before playing so the duration is known; glow is started with that duration, then playback begins. WAV: parse the RIFF header and PCM data by hand (16-bit PCM only; anything else is rejected with a clear error). MP3: go-mp3.
- Concurrency guard: a lock file in the data dir; if another push-it playback is running, log `already playing, skipping` and exit 0 (matches current behaviour).

### Hue

Port of the existing script: save `on/bri/hue/sat`, set full saturation/brightness at hue 0, step through six hue-wheel values ~0.45 s apart with short transitions, pause, restore saved state. HTTP timeouts of 2 s per call; any failure logs and exits 0. Same lock-file guard (overlapping bursts would fight over save/restore). Bridge TLS: the Hue bridge uses a self-signed cert; the client accepts it only for the configured bridge host, and the docs say so.

### Glow

Contract: `glow.Run(ctx, duration time.Duration) error`. The backend is chosen at compile time by build tags. Shared visual parameters are constants in `internal/glow/params.go` (frame thickness 14 px, full hue rotation every 2 s, opacity pulse period 0.6 s between 0.55 and 1.0) and are mirrored verbatim in the JS and Swift renderers with a comment pointing back.

- **Linux / GNOME** — a Shell extension exposing D-Bus `com.infiniteroomlabs.PushItGlow.Start(d: double)` on the session bus at `/com/infiniteroomlabs/PushItGlow`. `Start` adds a non-reactive `St.DrawingArea` to `Main.layoutManager.uiGroup` covering the primary monitor, repaints the frame at ~30 fps via a GLib timeout — four edge strips, each a Cairo linear gradient whose hue phase advances per frame so the rainbow appears to travel around the perimeter (Cairo has no conic gradients), and removes itself after `d` seconds. The Go backend calls `gdbus call --session ...` via `os/exec`. If `gdbus` is missing or the call fails (extension not installed/enabled, not GNOME), it returns nil after logging at debug level — glow is best-effort.
- **macOS** — `glow/macos/glow.swift`: a borderless `NSWindow` at `.screenSaver` level, `ignoresMouseEvents = true`, `collectionBehavior = [.canJoinAllSpaces, .stationary, .fullScreenAuxiliary]`, clear background, one `CAGradientLayer` (`type = .conic`) masked to a frame path, with `CABasicAnimation`s for rotation and opacity pulse. Takes `--duration` and exits on its own. CI builds it with `swiftc` for `arm64-apple-macos` and `x86_64-apple-macos` and merges with `lipo`; the universal binary is passed to the release build as a CI artifact and embedded with `//go:build darwin` + `go:embed`. At runtime the Go backend extracts it to the data dir if missing or if its embedded hash changed, then `exec`s it detached.
- **Windows** — in-process: a `WS_EX_LAYERED | WS_EX_TRANSPARENT | WS_EX_TOPMOST | WS_EX_TOOLWINDOW | WS_EX_NOACTIVATE` window covering the primary monitor, frames rendered by `internal/glow/paint` (hue by position along the perimeter, phase advancing per frame) into a 32-bit premultiplied BGRA buffer and pushed with `UpdateLayeredWindow` at 30 fps from a locked OS thread running a message loop. `golang.org/x/sys/windows` plus `NewLazySystemDLL` for the few gdi32/user32 calls not wrapped there. No helper, no cgo.
- **Everything else** — `glow_other.go` returns nil.

### Hook orchestration

`push-it hook pre-push` must return in milliseconds. It re-execs itself as `push-it hook --run` fully detached (`setsid`-equivalent: `Setsid` on Unix, `DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP` on Windows) with stdio to the log file, then exits 0. The detached run: load config, apply kill switches, start Hue in a goroutine, decode the clip, start glow with the clip's duration in a goroutine, play, wait for all, exit. Every error is logged to `<data dir>/push-it.log` and never surfaces to git.

## Install / uninstall

`push-it install` never modifies a hook it did not create without leaving a reversible marker.

1. Determine components (flags or interactive prompts; `--yes` accepts defaults: sound on, glow on where supported, hue off).
2. Write config. Create the clips dir. If it is empty, print where to put clips and link to `docs/make-your-own-clips.md`.
3. Hook wiring:
   - If `git config --global core.hooksPath` is unset: create `<config dir>/hooks/pre-push` containing the push-it line, set `core.hooksPath` to that directory, record `hooks_path_set_by_us: true`.
   - If set: append to `<hooksPath>/pre-push` (creating it, executable, if absent) the block `# >>> push-it >>> ... # <<< push-it <<<` containing `push-it hook pre-push "$@" || true`, record the file path. If the marker already exists, leave it alone (idempotent).
4. Hue: prompt for bridge/key/light unless env overrides exist; validate with one GET; store.
5. Glow: GNOME → extract the extension from `embed.FS` into `~/.local/share/gnome-shell/extensions/<uuid>/`, run `gnome-extensions enable <uuid>`, print the "log out and back in — Wayland can't hot-load extensions" note. macOS → extract the helper into the data dir, `chmod +x`. Windows → nothing to install.

`push-it uninstall` reads `install_state` and does the inverse in reverse order: remove the marker block (or the whole file if we created it), unset `core.hooksPath` only if we set it, disable and remove the GNOME extension, delete the macOS helper, then ask before deleting the config and clips (default: keep clips).

`install.sh` is a dependency-free POSIX sh script for `curl -fsSL .../install.sh | sh -s -- --all`: detect `uname -s`/`uname -m`, map to a release asset, download the tarball plus `checksums.txt`, verify with `sha256sum`/`shasum`, place the binary in `~/.local/bin` (creating it, warning if not on PATH), then `exec push-it install "$@"`. `go install github.com/InfiniteRoomLabs/push-it/cmd/push-it@latest` is the documented alternative.

## Clip toolkit (`tools/clipper/`)

The existing three scripts, made reusable:

- `transcribe.py SOURCE [--model small.en] -o transcript.json` — faster-whisper word-level timestamps.
- `cut.py SOURCE transcript.json [--phrase push it] [--allow real good] [--gap 0.5] [--max 4.0] [--pad 0.3] -o candidates/` — groups words into phrases that start with the target phrase, writes `clip_NNN.mp3` via ffmpeg and a `candidates.json` manifest with start/end/label.
- `review.py candidates/ --keep-to clips/` — plays each candidate (via `push-it play --file` so it uses the same audio path as the hook, falling back to ffplay), asks keep/skip/replay/quit, and moves keepers into the target clips dir with a descriptive filename (`003-push-it-push-it-real-good.mp3`).

Each is a PEP 723 script run with `uv run`; Python is a dev-time tool only and never part of the runtime. `docs/make-your-own-clips.md` walks through the pipeline end to end, states the copyright position plainly, and explains how to adapt `--phrase` for a different track.

## Testing

- `go test ./...` on ubuntu, macos, windows runners:
  - `clips`: listing/filtering/picking over a temp dir; deterministic with a seeded RNG.
  - `player`: WAV header parsing and duration math on tiny generated fixtures; MP3 duration via a 0.1 s fixture generated at test setup (`ffmpeg` if present, otherwise skipped).
  - `hue`: full burst against `httptest.Server` asserting the request sequence and that the final PUT restores the saved state; failure paths (timeout, bad JSON) return without panicking.
  - `glow/paint`: frame geometry (pixels inside the frame are opaque, interior is transparent), hue rotation advances, alpha pulse bounds.
  - `glow` (linux/darwin): the backend calls the expected command line via an injected exec interface.
  - `installer`: temp `HOME`/`XDG_CONFIG_HOME` and a temp git repo; assert hooksPath handling in both branches, marker idempotency, and that `uninstall` returns the filesystem and git config to the starting state.
  - `hook`: `pre-push` returns 0 in under 100 ms with a stubbed runner.
- `gofmt -l`, `go vet`, `staticcheck`.
- GNOME extension: `gjs tests/run.js` exercising `glowmath.js` (hue→rgb, frame path points, duration→frame count) with the minimal harness pattern from `claude-usage-gnome`.
- macOS helper: `swiftc` compile on CI plus `glow --duration 0 --dry-run` exiting 0.
- Visual verification of all three glow backends is manual and recorded in the release checklist.

## CI / CD / releases

- `ci.yml` on push and PR: lint, `go test` matrix (3 OSes), gjs tests, swiftc build (artifact `glow-macos-universal`).
- Semver tags `vMAJOR.MINOR.PATCH`. `release.yml` on tag: requires the CI job to pass, downloads the Swift helper artifact into `glow/macos/bin/`, runs goreleaser: `linux/darwin/windows × amd64/arm64`, `CGO_ENABLED=0`, archives + `checksums.txt`, the GNOME extension zip, and release notes extracted from the matching `CHANGELOG.md` section.
- `CHANGELOG.md` follows Keep a Changelog; the release task (`mise run release -- vX.Y.Z`) fails if the version has no changelog section, then tags and pushes.

## Repository setup and hygiene

- MIT license. Public from the first commit, so every commit is public-safe: no personal paths, IPs, light IDs, or vault names; examples use placeholders. Run the agent-ops redaction-term scan before the first push and in the release checklist.
- Remotes: `origin` = private Gitea, `github` = `github.com/InfiniteRoomLabs/push-it`, matching sibling repos.
- `mise.toml` pins go, goreleaser, staticcheck and defines tasks: `test`, `lint`, `build`, `glow:gnome:test`, `release`.
- Plane project created with v1 work items mirroring the implementation plan.
- `docs/migrating.md` covers moving from the pre-repo setup: copy existing keeper clips into the new clips dir, move Hue credentials into config, remove the old hook scripts.

## Open risks

- oto on Linux dynamically loads `libasound.so.2`; systems without ALSA compat (rare) get a logged error and no sound. `doctor` reports it.
- The GNOME extension needs a logout to load after first install; documented, unavoidable on Wayland.
- Windows glow and audio are CI-compiled and unit-tested but not hand-verified before v1.0.0; the changelog will say so.
