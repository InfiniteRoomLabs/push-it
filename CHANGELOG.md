# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Design spec for the `push-it` pre-push celebration: sound, Hue rainbow, and cross-platform screen glow (`docs/superpowers/specs/2026-08-20-push-it-design.md`) and the Plan 1 implementation plan for the core binary (`docs/superpowers/plans/2026-08-20-push-it-core.md`); Linux audio goes through a pure-Go PulseAudio/PipeWire client because oto's Linux driver needs cgo.
- Go module scaffold, MIT license, pinned mise toolchain, and the `push-it version` command.
- `internal/config`: JSON config at the OS config dir with 0600 permissions (enforced even on pre-existing files) and `PUSH_IT_HUE_*` env overrides that never get written back to disk.
- `internal/clips`: list and randomly pick `.mp3`/`.wav` clips from a directory, with tests covering absolute paths, subdirectory skipping, and RNG usage.
- `internal/player`: MP3 (go-mp3) and 16-bit WAV decoding, WAV encoding, slicing, and playback via oto (macOS/Windows) or a pure-Go PulseAudio/PipeWire client (Linux) - no ffmpeg, no cgo.
- `internal/hue`: save -> rainbow burst -> restore against the Hue v1 API with 2 s timeouts and a trust-on-first-use certificate pin.
- `internal/lockfile`: exclusive-create lock with stale takeover, so overlapping pushes don't stack playback or fight over the Hue state.
