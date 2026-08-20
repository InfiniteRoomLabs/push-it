# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- Repository published: public GitHub (`InfiniteRoomLabs/push-it`) with a private mirror; CI runs lint, a no-cgo cross-compile matrix, and tests on Linux, macOS, and Windows on every push.

### Added

- `CLAUDE.md`: agent-facing contract for the repo (layout, hard rules, commit/test conventions, review tiers).
- Design spec for the `push-it` pre-push celebration: sound, Hue rainbow, and cross-platform screen glow (`docs/superpowers/specs/2026-08-20-push-it-design.md`), the Plan 1 implementation plan for the core binary (`docs/superpowers/plans/2026-08-20-push-it-core.md`), and the Plan 2 plan for the glow backends (`docs/superpowers/plans/2026-08-20-push-it-glow.md`).
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
- `internal/glow/paint`: reference renderer for the rainbow frame (perimeter position, hue rotation, opacity pulse, premultiplied BGRA); `glow.Install` now returns a user-facing note.
- GNOME Shell extension `pushit-glow@infiniteroomlabs.com` (embedded in the binary): D-Bus `Start(seconds)`/`Stop`, click-through rainbow frame on the primary monitor; gjs unit tests for the shared math.
- Linux glow backend: `push-it install --glow` extracts and enables the GNOME extension; the hook triggers it over D-Bus with the clip's exact duration.
