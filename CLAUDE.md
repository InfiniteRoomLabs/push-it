# CLAUDE.md

Guidance for AI coding agents working in this repository. Humans: the README and `docs/` are for you; this file is the agent-facing contract.

## What this is

`push-it` is a single Go binary that celebrates `git push`: a `pre-push` hook plays a random clip, runs a Philips Hue rainbow burst, and (where a backend exists) draws an animated rainbow frame around the screen for exactly as long as the clip plays. MIT, public, zero runtime dependencies beyond the desktop's audio server. It ships no audio; users cut their own clips with the bundled toolkit.

The binding design documents live in-repo:

- Spec: `docs/superpowers/specs/2026-08-20-push-it-design.md` (authority when code and plan disagree)
- Plans: `docs/superpowers/plans/` (Plan 1 core binary - shipped; Plan 2 glow backends; Plan 3 release pipeline)
- Backlog and open risks: the `PUSHIT` project in Plane (not in this repo)

## Layout

```
cmd/push-it/          CLI: stdlib flag subcommands registered into the `commands` map via init()
internal/config/      config.json (0600) at the OS config dir; PUSH_IT_HUE_* env overrides are never persisted
internal/clips/       list + random pick of *.mp3/*.wav
internal/player/      pure-Go MP3/WAV decode; playback via oto (macOS/Windows) or a PulseAudio/PipeWire client (Linux)
internal/hue/         Hue v1 API: save state -> rainbow -> restore; trust-on-first-use certificate pin
internal/lockfile/    exclusive-create lock with stale takeover and owner-checked release
internal/glow/         backend hook points (Run/Install/Uninstall set by build-tagged files); animation constants are defined in paint and re-exported here
internal/glow/paint/  reference renderer; every backend mirrors its math
internal/hook/        kill switches, sound || hue || glow orchestration, detached pre-push entry
internal/clipper/     transcript -> phrase grouping -> WAV cutting -> keep/skip review
internal/installer/   reversible core.hooksPath / pre-push marker-block wiring
tools/clipper/        transcribe.py (the only Python; faster-whisper via uv, dev-time only)
docs/                 user docs (install, make-your-own-clips, hue, glow, migrating)
```

## Commands

`mise` pins the toolchain (`mise.toml`); use its tasks:

- `mise run test` - `go test ./...` (CI also runs `-race` on Linux)
- `mise run lint` - gofmt, `go vet`, staticcheck (all must be clean before a commit)
- `mise run build` - `bin/push-it`
- `mise run glow:gnome:test` - gjs tests for the GNOME extension math
- Cross-compile check used everywhere: `CGO_ENABLED=0 GOOS=<os> GOARCH=<arch> go build ./...` for linux/darwin/windows x amd64/arm64

## Hard rules

- **Never block a push.** Every path from `push-it hook pre-push` must exit 0 in milliseconds; the celebration runs in a detached child and logs to `<config dir>/push-it.log`. Treat any change that can make the hook fail or wait as a Critical bug.
- **No cgo, no ffmpeg, no new dependencies without a spec amendment.** Current deps: oto/v3 (macOS/Windows audio), jfreymuth/pulse (Linux audio), go-mp3, golang.org/x/sys. oto's Linux driver needs cgo, which is why Linux uses pulse.
- **Secrets never touch disk or logs by accident.** The Hue key comes from the config file (0600) or `PUSH_IT_HUE_KEY`; env-sourced values are scrubbed by `config.Save()`; transport errors are redacted; prompts never echo the key. Keep it that way.
- **TLS pinning is trust-on-first-use and strict.** `InsecureSkipVerify` is allowed only paired with the fingerprint `VerifyPeerCertificate` callback (and in `Fingerprint` for first contact). A changed certificate is never re-pinned under `--yes`.
- **Installer is reversible.** Everything `install` changes is recorded in `InstallState`; `uninstall` reverses exactly that and restores user hooks byte-for-byte. Install flags are additive on an existing config and start from all-off on a fresh one.
- **Public repo hygiene on every commit.** No personal usernames, private hostnames, internal IPs, vault names, or light IDs anywhere - code, tests, docs, plans, commit messages. Tests use `httptest` servers and documentation IPs only.

## Conventions

- Commits: stage with `git add <files>` and commit in a separate command (a hook inspects the staged set; `-a`/`-am` are rejected). Every commit touches `CHANGELOG.md` under `## [Unreleased]` (Keep a Changelog sections). Commit subjects are conventional (`feat(hue): ...`, `fix(installer): ...`, `docs: ...`).
- Markdown, YAML, JS, Swift: ASCII punctuation only (no em dashes, curly quotes, arrows) and never hard-wrap prose.
- Tests must be hermetic: point `PUSH_IT_CONFIG_DIR`, `XDG_DATA_HOME`, `GIT_CONFIG_GLOBAL`, and `HOME` at temp dirs before touching config, extensions, or git. Never run `push-it install`/`uninstall`/`glow` against the developer's real environment from a test or a review.
- Tests must not need an audio device, a display, a Hue bridge, or the network. Playback and glow are verified manually and recorded in the changelog.
- Interfaces between packages are documented in the plan's `Interfaces:` blocks; do not rename exported names other tasks depend on.

## Agent workflow

- Work from the plan for the current phase with `superpowers:subagent-driven-development`: fresh implementer per task, task brief as the single source of requirements, a review after every task, one final whole-branch review before merge.
- Model policy for this repo: implementers are Sonnet at minimum (never Haiku); each review is done by a model one tier above the implementer (Sonnet -> Opus, Opus -> Fable); the final review is Fable. Judgment-heavy tasks (Win32, security paths) take an Opus implementer.
- Reviewers are read-only on the checkout and never spawn sub-reviewers.
- Before the first push of any branch: redaction scan of the working tree and history (`git log -p --all | grep -i <terms>`); scrub with `git filter-repo` while still unpublished if anything matches.
- Remotes: `origin` is a private mirror, `github` is the public repository. Push `main` to both.
