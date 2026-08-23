# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.2.0] - 2026-08-23

### Added

- mise run glow:gnome:dev: nested GNOME Shell dev loop that runs the extension from the working tree in a window - no logout per change, and the real session's extension list, dconf, and config are never touched.
- push-it doctor now reports: config warnings, audio server reachability on Linux (not probed on macOS/Windows), whether the wired pre-push block still points at this binary, glow install state (GNOME extension present; macOS helper embedded/extracted), active NO_* kill switches, and whether the log file is writable; other Unixes report the audio probe as not probed and keep building.

### Fixed

- Linux playback was broken against PipeWire since v0.1.0: once the reader hit EndOfData (any clip that fits the server's multi-second buffer target), the pulse library's Start() blocked forever on a Started event its own state guard had dropped, leaving a hung hook process behind on every push and only a garbled fragment of the clip (typically the tail) audible. play now runs Start() in a guarded goroutine and uses the wall clock (clip duration plus a 250 ms tail margin) as the completion signal instead of the library's Start/Drain signals. Verified manually: a 0.3 s WAV returns in 0.58 s and a 4.32 s clip plays in full from the start, returning at 4.75 s.
- Config values are normalized on load with a warning instead of being used raw: volume is clamped to 0..1 (NaN resets to 0.7), the Hue light number is floored at 1, and a malformed PUSH_IT_HUE_LIGHT is reported instead of silently ignored. push-it doctor prints the warnings.
- Release notes: `.goreleaser.yaml` no longer sets `changelog.disable`, which made goreleaser skip loading `--release-notes` and published v0.1.0 with an empty body (patched in place).
- install --glow on GNOME now pre-enables the extension by writing the UUID into org.gnome.shell enabled-extensions (deduplicated, existing entries preserved), so no manual enable is needed after the logout; uninstall removes the UUID again. The gnome-extensions enable/disable attempts now run under a 10 s timeout. Reading enabled-extensions ignores dconf warnings and refuses to write back an unparseable list, instead falling back to the manual-enable note with the failure reason.
- Hook log no longer double-prefixes glow/hue errors ("glow: glow: ..."); a cancelled glow reports the cancellation instead of "extension did not answer", and cancelled glow errors stay attributed to glow (the other bare `ctx.Err()` returns in the Linux and Windows backends are now wrapped as `glow: %w` too, since the hook log no longer adds the prefix itself); the Windows glow calls SetProcessDPIAware before measuring the screen so the band is crisp and correctly sized at >100% scaling.

### Changed

- Dogfood of v0.1.0 on Linux/GNOME via `install.sh --sound --hue --glow --yes`: checksum verified, binary replaced in `~/.local/bin`, `push-it version` prints `v0.1.0 (67ddec5)`, `doctor` reports sound/Hue/glow enabled, and a real `git push` played a clip with the Hue burst; the release workflow published six archives, `checksums.txt`, and the GNOME extension zip on the first tag. macOS and Windows remain CI-verified only.

## [0.1.0] - 2026-08-22

### Changed

- `push-it version` prints the commit hash after the version when built with `-X main.commit`; `mise run build` now builds static (`CGO_ENABLED=0`) with version and commit stamped.
- Glow is now a feathered 96 px (at 1080p) inward-fading rainbow with overlapping corners instead of a hard 14 px frame; reference renderer, Windows backend, spec, and docs updated.
- Repository published: public GitHub (`InfiniteRoomLabs/push-it`) with a private mirror; CI runs lint, a no-cgo cross-compile matrix, and tests on Linux, macOS, and Windows on every push.
- The glow animation parameters (`GlowWidthAt1080`, `FalloffExponent`, `RotationPeriod`, `PulsePeriod`, `MinOpacity`, `MaxOpacity`) now live in `internal/glow/paint` and are re-exported by `internal/glow` under the same names, so an in-process backend inside package `glow` can call the renderer without an import cycle.
- docs: glow is shipped on GNOME, macOS (release binaries), and Windows (visual verification pending).

### Added

- `install.sh`: dependency-free POSIX bootstrap (`curl ... | sh -s -- --all`) that downloads the latest release for the host, verifies `checksums.txt`, installs to `~/.local/bin`, and runs `push-it install`, re-attaches the terminal when piped from curl; tested end to end in CI (Linux and macOS) against a local fake release.
- Release tooling: `.goreleaser.yaml` (linux/darwin/windows x amd64/arm64, static, darwin with the embedded glow helper, checksums, GNOME extension zip), `scripts/changelog-notes.sh`, and `mise run release -- vX.Y.Z`, which refuses to tag without a matching changelog section. CI validates the goreleaser config and shellchecks the scripts.
- `release.yml`: on every `v*` tag, reruns the full CI workflow as a gate, pulls the universal macOS glow helper from that run, and publishes the goreleaser artifacts with release notes taken from the tag's `CHANGELOG.md` section. All GitHub Actions are pinned by commit SHA.
- CI: gjs tests for the GNOME extension and a macOS job that builds the universal helper and tests the darwin build with it embedded.
- CLI tests split per source file with flag/exit-code coverage for play, hue, glow, and clips.
- `CLAUDE.md`: agent-facing contract for the repo (layout, hard rules, commit/test conventions, review tiers).
- Design spec for the `push-it` pre-push celebration: sound, Hue rainbow, and cross-platform screen glow (`docs/superpowers/specs/2026-08-20-push-it-design.md`), the Plan 1 implementation plan for the core binary (`docs/superpowers/plans/2026-08-20-push-it-core.md`), the Plan 2 plan for the glow backends (`docs/superpowers/plans/2026-08-20-push-it-glow.md`), and Plan 2b for the feathered glow (`docs/superpowers/plans/2026-08-21-push-it-glow-feather.md`), and Plan 3 for the release pipeline (`docs/superpowers/plans/2026-08-22-push-it-release.md`); Plan 4 for v0.2.0 (`docs/superpowers/plans/2026-08-23-push-it-v0.2.0.md`).
- Go module scaffold, MIT license, pinned mise toolchain, and the `push-it version` command.
- `internal/config`: JSON config at the OS config dir with 0600 permissions (enforced even on pre-existing files) and `PUSH_IT_HUE_*` env overrides that never get written back to disk.
- `internal/clips`: list and randomly pick `.mp3`/`.wav` clips from a directory, with tests covering absolute paths, subdirectory skipping, and RNG usage.
- `internal/player`: MP3 (go-mp3) and 16-bit WAV decoding, WAV encoding, slicing, and playback via oto (macOS/Windows) or a pure-Go PulseAudio/PipeWire client (Linux) - no ffmpeg, no cgo; the Linux backend rounds down to a whole frame before handing samples to pulse.
- `internal/hue`: save -> rainbow burst -> restore against the Hue v1 API with 2 s timeouts and a trust-on-first-use certificate pin; transport errors never leak the API key, and an unpinned bridge fails with a clear message instead of a raw fingerprint mismatch.
- `internal/lockfile`: exclusive-create lock with stale takeover and owner-checked release, so overlapping pushes don't stack playback or fight over the Hue state.
- `internal/glow`: shared animation parameters and the backend hook points (no-op until platform renderers land).
- `internal/hook`: kill switches, concurrent sound/hue/glow orchestration with the glow synced to the clip length, and a detached `pre-push` entry that returns in milliseconds.
- `internal/clipper`: group transcript words into phrases, cut padded WAV candidates, and an interactive keep/skip review loop; keeping a candidate falls back to copy-then-remove when a plain rename fails across filesystems; cross-filesystem moves close the source before removing it (Windows-safe).
- `internal/installer`: reversible `core.hooksPath` / `pre-push` wiring with marker blocks; uninstall restores the user's hook byte-for-byte; refuses to append to an existing `pre-push` that isn't a shell script rather than risk breaking every push, and always leaves the hook file executable.
- CLI: `play`, `hue`, `glow`, `hook pre-push`, `clips cut`, `clips review`, `install` (interactive or flagged), `uninstall`, `doctor`; `install --hue --yes` never auto-trusts a changed bridge certificate - it refuses non-interactively and keeps the old pin - and skips Hue entirely, rather than prompting or saving a broken config, when the bridge or key is still unset; the Hue API key prompt never echoes the stored/env value.
- Docs: install, make-your-own-clips, hue, glow, migrating; `tools/clipper/transcribe.py`; GitHub Actions CI (lint, no-cgo cross-compile, tests on Linux/macOS/Windows).
- `internal/glow/paint`: reference renderer for the rainbow glow (`GlowWidth`, `EdgeAlpha`, `EdgePos`, `Render`, `RenderGlow`: perimeter position, hue rotation, opacity pulse, premultiplied BGRA); `glow.Install` now returns a user-facing note.
- GNOME Shell extension `pushit-glow@infiniteroomlabs.com` (embedded in the binary): D-Bus `Start(seconds)`/`Stop`, click-through feathered glow on the primary monitor rendered as four strips (axial hue gradient masked by an inward alpha falloff), declares `shell-version` support for GNOME 46-50; gjs unit tests for the shared math.
- Linux glow backend: `push-it install --glow` extracts and enables the GNOME extension; the hook triggers it over D-Bus with the clip's exact duration; refuses clearly on non-GNOME sessions.
- Windows glow backend: in-process click-through layered window rendering the feathered glow via the shared `paint` package (compiles and vets in CI; visual verification pending).
- macOS glow backend: a Swift helper (universal binary, built in CI, embedded with `-tags glowhelper`) draws the four-strip feathered glow (axial hue gradient masked by an inward alpha falloff) mirroring the reference renderer; `install --glow` extracts it next to the config.

### Fixed

- GNOME extension: the first frame of the glow animation now starts at its pulsed opacity instead of full opacity, matching every subsequent frame.
- `install`: a failed glow install now disables glow instead of leaving it enabled, and prints why.
- Installer only appends to hooks whose shebang is an allowlisted POSIX-compatible shell (sh, bash, dash, zsh, ksh, ash, including the `env -S` form); pwsh/fish/csh/tcsh hooks are refused with instructions instead of being broken. Appending now preserves the hook's existing mode bits, only adding the owner exec bit rather than forcing group/other exec too.
- `install.sh`: the terminal probe opens `/dev/tty` read-only so it can never create a stray file; `PUSH_IT_VERSION` accepts `0.1.0` as well as `v0.1.0`; `scripts/changelog-notes.sh` stops at the link-reference footer.

[Unreleased]: https://github.com/InfiniteRoomLabs/push-it/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/InfiniteRoomLabs/push-it/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/InfiniteRoomLabs/push-it/releases/tag/v0.1.0
