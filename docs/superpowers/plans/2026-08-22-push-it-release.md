# push-it Plan 3: Release Pipeline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship `v0.1.0`: semver-tagged GitHub releases with six static binaries, checksums, the GNOME extension zip, a dependency-free `install.sh`, and release notes pulled from `CHANGELOG.md`.

**Architecture:** goreleaser (pinned in `mise.toml`) builds `linux/darwin/windows x amd64/arm64` with `CGO_ENABLED=0`; the darwin builds carry `-tags glowhelper` so the Swift helper built by the existing `macos-helper` CI job is embedded. `release.yml` runs on `v*` tags, reuses `ci.yml` via `workflow_call` as its gate, downloads the helper artifact from that same run, and hands goreleaser release notes extracted from the matching changelog section. `install.sh` downloads the `latest` asset for the host, verifies `checksums.txt`, drops the binary in `~/.local/bin`, and execs `push-it install "$@"`.

**Tech Stack:** Go 1.26.5 (stdlib only, no new deps), goreleaser 2.17.1, GitHub Actions, POSIX sh, shellcheck.

**Spec:** `docs/superpowers/specs/2026-08-20-push-it-design.md` (sections "install.sh" line 138, "CI / CD / releases" lines 166-176). Read it before starting; it is the authority when this plan and the code disagree.

## Global Constraints

- **Never block a push.** Nothing in this plan touches `hook pre-push`, but Task 2 edits the installer: an existing hook that push-it cannot extend safely must be refused, never broken.
- **No cgo, no new Go dependencies.** `CGO_ENABLED=0` everywhere. `go.mod` does not change in this plan.
- **Public repo hygiene on every commit.** No personal usernames, private hostnames, internal IPs, vault names, light IDs in code, tests, docs, scripts, workflows, or commit messages. Tests use temp dirs and `file://` URLs only.
- **Commits:** `git add <files>` then `git commit` in a separate command (`-a`/`-am` are rejected by a hook). Every commit touches `CHANGELOG.md` under `## [Unreleased]`. Conventional subjects (`feat(release): ...`, `fix(installer): ...`, `ci: ...`, `docs: ...`).
- **Prose:** ASCII punctuation only (no em dashes, curly quotes, arrows) in Markdown, YAML, shell; never hard-wrap prose.
- **Hermetic tests:** point `PUSH_IT_CONFIG_DIR`, `XDG_DATA_HOME`, `GIT_CONFIG_GLOBAL`, and `HOME` at temp dirs before touching config or git. Never run `push-it install`/`uninstall`/`glow` against the developer's real environment from a test or review. Shell tests never touch the network.
- **Toolchain:** `mise run test`, `mise run lint` (gofmt, vet, staticcheck) must be clean before every commit. `shellcheck` and `zip` are on the dev machine and on `ubuntu-latest`.
- **install.sh has no `usage` shebang** (it is a public bootstrap script; `usage` is an external dependency). Plain `#!/bin/sh`, POSIX only (`sh -n`, `shellcheck -s sh`).
- **GitHub Actions pinned by commit SHA** with a `# vN` trailing comment (supply-chain posture). Resolve SHAs with `gh api repos/<owner>/<repo>/git/ref/tags/<tag> --jq .object.sha` (if the ref is an annotated tag, dereference with `gh api repos/<owner>/<repo>/git/tags/<sha> --jq .object.sha`).
- Remotes: `origin` (private mirror) and `github` (public). Tasks in this plan commit on a feature branch; the orchestrator merges and pushes to both.

## Deviations (ruled during execution)

- Task 2 appends with `mode | 0o100` (owner exec only), not `| 0o111`, so a private hook is never widened.
- Task 3 builds the GNOME extension zip into `dist-extra/`, not `dist/`, because goreleaser's dist-empty check runs after before-hooks and `--clean` would wipe it.
- Tasks 3/4 pin the goreleaser binary `2.17.1` in CI (not `~> v2`) to match `mise.toml`.

---

### Task 1: `version` prints semver + commit; build task is static

**Files:**
- Modify: `cmd/push-it/main.go:11-23`
- Modify: `cmd/push-it/main_test.go` (the existing `version` test near line 45)
- Modify: `mise.toml` (`[tasks.build]`)
- Modify: `CHANGELOG.md`

**Interfaces:**
- Produces: package-level `var version = "dev"` and `var commit = ""` in `package main`, both set via `-ldflags "-X main.version=... -X main.commit=..."`. Task 3's goreleaser config and Task 1's mise task set them.
- Output format: `push-it v0.1.0 (abc1234)\n` when `commit` is non-empty; `push-it dev\n` when it is empty.

- [ ] **Step 1: Write the failing test**

In `cmd/push-it/main_test.go`, replace the existing version test with:

```go
func TestVersionDevWithoutCommit(t *testing.T) {
	oldV, oldC := version, commit
	t.Cleanup(func() { version, commit = oldV, oldC })
	version, commit = "dev", ""
	var out, errOut strings.Builder
	if code := run([]string{"version"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut.String())
	}
	if got := out.String(); got != "push-it dev\n" {
		t.Fatalf("got %q", got)
	}
}

func TestVersionWithCommit(t *testing.T) {
	oldV, oldC := version, commit
	t.Cleanup(func() { version, commit = oldV, oldC })
	version, commit = "v0.1.0", "abc1234"
	var out, errOut strings.Builder
	if code := run([]string{"version"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut.String())
	}
	if got := out.String(); got != "push-it v0.1.0 (abc1234)\n" {
		t.Fatalf("got %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/push-it/ -run 'TestVersion' -v`
Expected: compile error `undefined: commit`.

- [ ] **Step 3: Implement**

In `cmd/push-it/main.go` replace lines 11-23 with:

```go
// version and commit are overwritten at build time via
// -ldflags "-X main.version=v1.2.3 -X main.commit=abc1234".
var (
	version = "dev"
	commit  = ""
)

type command func(args []string, stdin io.Reader, stdout, stderr io.Writer) int

// commands is populated by init() functions in the other files of this package.
var commands = map[string]command{}

func init() {
	commands["version"] = func(_ []string, _ io.Reader, stdout, _ io.Writer) int {
		if commit == "" {
			fmt.Fprintf(stdout, "push-it %s\n", version)
		} else {
			fmt.Fprintf(stdout, "push-it %s (%s)\n", version, commit)
		}
		return 0
	}
}
```

In `mise.toml` replace the build task with:

```toml
[tasks.build]
env = { CGO_ENABLED = "0" }
run = "go build -ldflags \"-s -w -X main.version=$(git describe --tags --always --dirty) -X main.commit=$(git rev-parse --short HEAD)\" -o bin/push-it ./cmd/push-it"
```

- [ ] **Step 4: Run tests and lint**

Run: `mise run test && mise run lint && mise run build && ./bin/push-it version`
Expected: tests pass, lint clean, output like `push-it bc57e93 (bc57e93)` (no tags yet, so `git describe` falls back to the short hash; after Task 6 it prints `v0.1.0-N-g...`).

- [ ] **Step 5: Changelog + commit**

Under `## [Unreleased]` / `### Changed` add: `- `push-it version` prints the commit hash after the version when built with `-X main.commit`; `mise run build` now builds static (`CGO_ENABLED=0`) with version and commit stamped.`

```bash
git add cmd/push-it/main.go cmd/push-it/main_test.go mise.toml CHANGELOG.md
git commit -m "feat(cli): version prints semver and commit from ldflags"
```

---

### Task 2: Installer shebang allowlist (PUSHIT-7)

**Files:**
- Modify: `internal/installer/hook.go:26-45` (`isShellInterpreter`) and `hook.go:112-122` (append branch)
- Modify: `internal/installer/installer_test.go`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Consumes: `isShellInterpreter(firstLine string) bool` (unexported, same name and signature kept).
- Produces: the same function with an explicit allowlist; no exported surface changes.

**Why:** the current check accepts any interpreter basename ending in `sh` (so `pwsh`, `fish`, `csh`, `tcsh` pass) and appending our `sh` block to one of those would break every push. Also handle `#!/usr/bin/env -S bash -e` (skip `-S` and any `-` flags to find the interpreter) and preserve the user's hook mode bits on append (`mode | 0o111`, not an unconditional `0755`).

- [ ] **Step 1: Write the failing tests**

Add to `internal/installer/installer_test.go`:

```go
func TestIsShellInterpreter(t *testing.T) {
	cases := map[string]bool{
		"":                               true, // no shebang: git runs it via sh
		"echo hi":                        true,
		"#!/bin/sh":                      true,
		"#!/bin/bash -e":                 true,
		"#!/usr/bin/dash":                true,
		"#!/bin/zsh":                     true,
		"#!/bin/ksh":                     true,
		"#!/bin/ash":                     true,
		"#!/usr/bin/env bash":            true,
		"#!/usr/bin/env -S bash -eu":     true,
		"#!/usr/bin/env -S -i bash":      true,
		"#!/usr/bin/pwsh":                false,
		"#!/usr/bin/env pwsh":            false,
		"#!/usr/bin/fish":                false,
		"#!/bin/csh":                     false,
		"#!/bin/tcsh":                    false,
		"#!/usr/bin/env python3":         false,
		"#!/usr/bin/env -S":              false,
		"#!":                             false,
		"#!/usr/bin/env":                 false,
	}
	for line, want := range cases {
		if got := isShellInterpreter(line); got != want {
			t.Errorf("isShellInterpreter(%q) = %v, want %v", line, got, want)
		}
	}
}

func TestWireHookAppendPreservesMode(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(dir, "gitconfig"))
	hooks := filepath.Join(dir, "hooks")
	_ = os.MkdirAll(hooks, 0o755)
	file := filepath.Join(hooks, "pre-push")
	_ = os.WriteFile(file, []byte("#!/bin/bash\necho hi\n"), 0o700)
	g := fakeGit{values: map[string]string{"core.hooksPath": hooks}}
	var st config.InstallState
	if err := WireHook(g, filepath.Join(dir, "cfg"), "/opt/push-it", &st); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("mode = %o, want 0700 (user's mode plus exec bits, not 0755)", got)
	}
}
```

Look at the existing tests in `installer_test.go` for the name of the fake `Git` implementation and how they seed `core.hooksPath`; adapt `fakeGit{...}` above to match that existing helper exactly (do not add a second fake).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/installer/ -run 'TestIsShellInterpreter|TestWireHookAppendPreservesMode' -v`
Expected: `TestIsShellInterpreter` fails on `pwsh`/`fish`/`csh`/`tcsh`/`-S` cases; `TestWireHookAppendPreservesMode` fails with `mode = 755`.

- [ ] **Step 3: Implement**

Replace `isShellInterpreter` in `internal/installer/hook.go`:

```go
// shellInterpreters is the explicit allowlist of interpreters whose syntax
// accepts the push-it block verbatim. Anything else (pwsh, fish, csh, tcsh,
// python, ...) is refused rather than risk breaking every push.
var shellInterpreters = map[string]bool{
	"sh": true, "bash": true, "dash": true, "zsh": true, "ksh": true, "ash": true,
}

// isShellInterpreter reports whether a hook starting with firstLine can have
// the push-it block appended. A file with no shebang is treated as shell
// because git runs it via sh. "#!/usr/bin/env [-S] [-flags] <interp>" is
// unwrapped to <interp>.
func isShellInterpreter(firstLine string) bool {
	if !strings.HasPrefix(firstLine, "#!") {
		return true
	}
	fields := strings.Fields(strings.TrimPrefix(firstLine, "#!"))
	if len(fields) == 0 {
		return false
	}
	interp := fields[0]
	if filepath.Base(interp) == "env" {
		interp = ""
		for _, f := range fields[1:] {
			if strings.HasPrefix(f, "-") {
				continue // -S, -i, and other env flags
			}
			interp = f
			break
		}
		if interp == "" {
			return false
		}
	}
	return shellInterpreters[filepath.Base(interp)]
}
```

In the `default:` append branch of `WireHook` (around lines 112-122), replace the `os.WriteFile(..., 0o755)` + `os.Chmod(file, 0o755)` pair with:

```go
		info, err := os.Stat(file)
		if err != nil {
			return err
		}
		mode := info.Mode().Perm() | 0o111
		if err := os.WriteFile(file, []byte(content+Block(exe)), mode); err != nil {
			return err
		}
		if err := os.Chmod(file, mode); err != nil {
			return err
		}
```

(`os.WriteFile` does not change the mode of an existing file, which is why the explicit `Chmod` stays.) Leave the other two `0o755` writes alone: they create a brand-new hook file.

- [ ] **Step 4: Run tests and lint**

Run: `mise run test && mise run lint`
Expected: all pass, lint clean. Check that no existing test relied on a `fish`/`pwsh` hook being accepted; if one did, it was asserting the bug - update it to expect the refusal error.

- [ ] **Step 5: Changelog + commit**

Under `### Fixed` (create the section under `## [Unreleased]` if missing): `- Installer only appends to hooks whose shebang is an allowlisted POSIX-compatible shell (sh, bash, dash, zsh, ksh, ash, including the `env -S` form); pwsh/fish/csh/tcsh hooks are refused with instructions instead of being broken. Appending now preserves the hook's existing mode bits.`

```bash
git add internal/installer/hook.go internal/installer/installer_test.go CHANGELOG.md
git commit -m "fix(installer): allowlist shell interpreters and preserve hook mode"
```

---

### Task 3: goreleaser config, changelog extraction, release task

**Files:**
- Create: `.goreleaser.yaml`
- Create: `scripts/changelog-notes.sh`
- Create: `scripts/release.sh`
- Create: `scripts/test-changelog-notes.sh`
- Modify: `mise.toml` (add `[tasks.release]`, `[tasks."lint:scripts"]`, extend `[tasks.lint]`)
- Modify: `.github/workflows/ci.yml` (lint job: `goreleaser check`, shellcheck, script tests)
- Modify: `CHANGELOG.md`

**Interfaces:**
- Produces: `scripts/changelog-notes.sh vX.Y.Z` prints the body of `## [X.Y.Z]` from `CHANGELOG.md` to stdout, exit 1 with a message on stderr if absent. Task 4's `release.yml` calls it.
- Produces: release asset names `push-it_<os>_<arch>.tar.gz` (linux/darwin), `push-it_windows_<arch>.zip`, `checksums.txt`, `pushit-glow@infiniteroomlabs.com.shell-extension.zip`. Task 5's `install.sh` depends on these exact names.
- Produces: goreleaser build ids `push-it` (linux, windows) and `push-it-darwin` (darwin, `-tags glowhelper`).

- [ ] **Step 1: Write the changelog extraction test**

`scripts/test-changelog-notes.sh`:

```sh
#!/bin/sh
# Self-check for scripts/changelog-notes.sh against a fixture changelog.
set -eu
cd "$(dirname "$0")/.."
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
cat > "$tmp/CHANGELOG.md" <<'EOF'
# Changelog

## [Unreleased]

### Added

- not released yet

## [0.2.0] - 2026-09-01

### Added

- second thing

### Fixed

- a fix

## [0.1.0] - 2026-08-22

### Added

- first thing
EOF

got=$(CHANGELOG=$tmp/CHANGELOG.md sh scripts/changelog-notes.sh v0.2.0)
want='### Added

- second thing

### Fixed

- a fix'
[ "$got" = "$want" ] || { printf 'v0.2.0: got:\n%s\nwant:\n%s\n' "$got" "$want"; exit 1; }

got=$(CHANGELOG=$tmp/CHANGELOG.md sh scripts/changelog-notes.sh v0.1.0)
[ "$got" = '### Added

- first thing' ] || { printf 'v0.1.0: got:\n%s\n' "$got"; exit 1; }

if CHANGELOG=$tmp/CHANGELOG.md sh scripts/changelog-notes.sh v9.9.9 >/dev/null 2>&1; then
  echo "expected failure for missing version"; exit 1
fi
if sh scripts/changelog-notes.sh 0.1.0 >/dev/null 2>&1; then
  echo "expected failure for tag without v prefix"; exit 1
fi
echo "changelog-notes: ok"
```

- [ ] **Step 2: Run it to verify it fails**

Run: `sh scripts/test-changelog-notes.sh`
Expected: fails because `scripts/changelog-notes.sh` does not exist.

- [ ] **Step 3: Write `scripts/changelog-notes.sh`**

```sh
#!/bin/sh
# Prints the CHANGELOG.md section for a release tag (vX.Y.Z) to stdout.
# Exits 1 if the tag is malformed or the section is missing.
set -eu
tag=${1:-}
case $tag in
  v[0-9]*.[0-9]*.[0-9]*) ;;
  *) echo "usage: changelog-notes.sh vX.Y.Z" >&2; exit 1 ;;
esac
ver=${tag#v}
file=${CHANGELOG:-CHANGELOG.md}
notes=$(awk -v ver="$ver" '
  /^## \[/ { if (found) exit; found = ($0 ~ "^## \\[" ver "\\]") ; next }
  found { print }
' "$file")
# trim leading/trailing blank lines
notes=$(printf '%s\n' "$notes" | sed -e '/./,$!d' | sed -e ':a' -e '/^\n*$/{$d;N;ba' -e '}')
if [ -z "$notes" ]; then
  echo "changelog-notes.sh: no '## [$ver]' section in $file" >&2
  exit 1
fi
printf '%s\n' "$notes"
```

Note for the awk: `ver` contains dots, which are regex metacharacters; they only ever match a dot or any character, which is acceptable for semver (there is no `0x1.0` section to collide with). Do not try to escape them.

- [ ] **Step 4: Run the test until green**

Run: `sh scripts/test-changelog-notes.sh && shellcheck -s sh scripts/*.sh`
Expected: `changelog-notes: ok`, shellcheck silent.

- [ ] **Step 5: Write `.goreleaser.yaml`**

```yaml
# goreleaser 2.x - see https://goreleaser.com
version: 2

project_name: push-it

before:
  hooks:
    - sh -c "rm -f dist/pushit-glow@infiniteroomlabs.com.shell-extension.zip && mkdir -p dist && cd internal/glow/gnome/ext && zip -qr ../../../../dist/pushit-glow@infiniteroomlabs.com.shell-extension.zip ."

builds:
  - id: push-it
    main: ./cmd/push-it
    binary: push-it
    env: [CGO_ENABLED=0]
    goos: [linux, windows]
    goarch: [amd64, arm64]
    ldflags:
      - -s -w -X main.version={{ .Tag }} -X main.commit={{ .ShortCommit }}
    mod_timestamp: "{{ .CommitTimestamp }}"
  - id: push-it-darwin
    main: ./cmd/push-it
    binary: push-it
    env: [CGO_ENABLED=0]
    goos: [darwin]
    goarch: [amd64, arm64]
    tags: [glowhelper]
    ldflags:
      - -s -w -X main.version={{ .Tag }} -X main.commit={{ .ShortCommit }}
    mod_timestamp: "{{ .CommitTimestamp }}"

archives:
  - id: default
    ids: [push-it, push-it-darwin]
    name_template: "{{ .ProjectName }}_{{ .Os }}_{{ .Arch }}"
    formats: [tar.gz]
    format_overrides:
      - goos: windows
        formats: [zip]
    files:
      - LICENSE
      - README.md
      - docs/*.md

checksum:
  name_template: checksums.txt
  algorithm: sha256

changelog:
  disable: true

release:
  github:
    owner: InfiniteRoomLabs
    name: push-it
  draft: false
  prerelease: auto
  extra_files:
    - glob: dist/pushit-glow@infiniteroomlabs.com.shell-extension.zip
```

- [ ] **Step 6: Validate the config and a local snapshot build**

Run: `goreleaser check && CGO_ENABLED=0 goreleaser build --snapshot --clean --id push-it`
Expected: `config is valid`, then `dist/` contains `push-it_linux_amd64_v1/push-it`, `push-it_linux_arm64/push-it`, `push-it_windows_amd64_v1/push-it.exe`, `push-it_windows_arm64/push-it.exe`. (`--id push-it` skips the darwin build, which needs the Swift helper that only CI can produce.) Confirm `./dist/push-it_linux_amd64_v1/push-it version` prints `push-it v0.0.0-SNAPSHOT-<hash> (<hash>)` or similar with a commit in parentheses. `dist/` is already gitignored.

- [ ] **Step 7: Write `scripts/release.sh` and the mise tasks**

`scripts/release.sh`:

```sh
#!/bin/sh
# Tags a release after checking the tree is clean, on main, and the changelog
# has a section for it. Pushes the tag to both remotes; release.yml does the rest.
set -eu
cd "$(dirname "$0")/.."
tag=${1:-}
case $tag in
  v[0-9]*.[0-9]*.[0-9]*) ;;
  *) echo "usage: release.sh vX.Y.Z" >&2; exit 1 ;;
esac
[ "$(git branch --show-current)" = main ] || { echo "release.sh: not on main" >&2; exit 1; }
[ -z "$(git status --porcelain)" ] || { echo "release.sh: working tree not clean" >&2; exit 1; }
git rev-parse -q --verify "refs/tags/$tag" >/dev/null && { echo "release.sh: $tag already exists" >&2; exit 1; }
sh scripts/changelog-notes.sh "$tag" >/dev/null
git tag -a "$tag" -m "$tag"
for r in origin github; do
  git remote get-url "$r" >/dev/null 2>&1 && git push "$r" "$tag"
done
echo "tagged and pushed $tag - watch https://github.com/InfiniteRoomLabs/push-it/actions"
```

Append to `mise.toml`:

```toml
[tasks.release]
description = "Tag vX.Y.Z (needs a CHANGELOG section) and push the tag to every remote"
run = "sh scripts/release.sh {{arg(name='version')}}"

[tasks."lint:scripts"]
run = [
  "shellcheck -s sh install.sh scripts/*.sh 2>/dev/null || shellcheck -s sh scripts/*.sh",
  "sh scripts/test-changelog-notes.sh",
  "goreleaser check",
]
```

and add `"mise run lint:scripts"` as the last entry of the `[tasks.lint]` `run` list. (`install.sh` does not exist until Task 5; the fallback keeps lint green in between. Task 5 simplifies it.)

- [ ] **Step 8: Add the script checks to CI**

In `.github/workflows/ci.yml` lint job, after the staticcheck step and before cross-compile, add:

```yaml
      - name: shell scripts
        run: |
          shellcheck -s sh scripts/*.sh
          sh scripts/test-changelog-notes.sh
      - uses: goreleaser/goreleaser-action@v6
        with:
          version: "~> v2"
          args: check
```

(Task 4 pins every `uses:` by SHA, including this one; leave the tag form here so this task stays reviewable on its own.)

- [ ] **Step 9: Run lint and tests**

Run: `mise run lint && mise run test`
Expected: clean.

- [ ] **Step 10: Changelog + commit**

Under `### Added`: `- Release tooling: `.goreleaser.yaml` (linux/darwin/windows x amd64/arm64, static, darwin with the embedded glow helper, checksums, GNOME extension zip), `scripts/changelog-notes.sh`, and `mise run release -- vX.Y.Z`, which refuses to tag without a matching changelog section. CI validates the goreleaser config and shellchecks the scripts.`

```bash
git add .goreleaser.yaml scripts/changelog-notes.sh scripts/release.sh scripts/test-changelog-notes.sh mise.toml .github/workflows/ci.yml CHANGELOG.md
git commit -m "feat(release): goreleaser config, changelog notes, release task"
```

---

### Task 4: `release.yml` on semver tags, gated on CI, actions pinned by SHA

**Files:**
- Create: `.github/workflows/release.yml`
- Modify: `.github/workflows/ci.yml` (add `workflow_call` trigger; pin every `uses:` by SHA)
- Modify: `CHANGELOG.md`

**Interfaces:**
- Consumes: the `glow-macos-universal` artifact uploaded by the `macos-helper` job in `ci.yml` (path `internal/glow/macos/bin/glow-macos`); `scripts/changelog-notes.sh` from Task 3.
- Produces: a GitHub Release for every `v*` tag.

- [ ] **Step 1: Make `ci.yml` callable and pin its actions**

Change the `on:` block of `.github/workflows/ci.yml` to:

```yaml
on:
  push:
    branches: [main]
  pull_request:
  workflow_call:
```

Then resolve SHAs and pin every `uses:` line. For each of `actions/checkout@v4`, `actions/setup-go@v5`, `dominikh/staticcheck-action@v1`, `actions/upload-artifact@v4`, `goreleaser/goreleaser-action@v6`:

```sh
for r in actions/checkout:v4 actions/setup-go:v5 dominikh/staticcheck-action:v1 actions/upload-artifact:v4 actions/download-artifact:v4 goreleaser/goreleaser-action:v6; do
  repo=${r%:*}; tag=${r#*:}
  sha=$(gh api "repos/$repo/git/ref/tags/$tag" --jq .object.sha)
  type=$(gh api "repos/$repo/git/ref/tags/$tag" --jq .object.type)
  [ "$type" = tag ] && sha=$(gh api "repos/$repo/git/tags/$sha" --jq .object.sha)
  echo "$repo@$sha # $tag"
done
```

Rewrite each line as `uses: actions/checkout@<sha> # v4` (full 40-char SHA, comment with the tag you resolved). Use the exact SHAs the command prints; do not copy from memory.

- [ ] **Step 2: Write `release.yml`**

```yaml
name: Release

on:
  push:
    tags: ["v*"]

permissions:
  contents: read

jobs:
  ci:
    uses: ./.github/workflows/ci.yml

  release:
    needs: ci
    runs-on: ubuntu-latest
    permissions:
      contents: write
    steps:
      - uses: actions/checkout@<sha> # v4
        with:
          fetch-depth: 0
      - uses: actions/setup-go@<sha> # v5
        with:
          go-version-file: go.mod
      - uses: actions/download-artifact@<sha> # v4
        with:
          name: glow-macos-universal
          path: internal/glow/macos/bin/
      - name: verify the macOS helper landed
        run: |
          chmod +x internal/glow/macos/bin/glow-macos
          file internal/glow/macos/bin/glow-macos
          test -s internal/glow/macos/bin/glow-macos
      - name: release notes from CHANGELOG.md
        run: sh scripts/changelog-notes.sh "$GITHUB_REF_NAME" > /tmp/notes.md
      - uses: goreleaser/goreleaser-action@<sha> # v6
        with:
          version: "~> v2"
          args: release --clean --release-notes /tmp/notes.md
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

Replace every `<sha>` with the value resolved in Step 1.

- [ ] **Step 3: Validate locally**

Run:
```sh
grep -n 'uses:' .github/workflows/*.yml | grep -v '@[0-9a-f]\{40\} # v' && echo "UNPINNED" || echo "all pinned"
goreleaser check
mise run lint && mise run test
```
Expected: `all pinned`, config valid, lint and tests clean. There is no local runner for the workflow; the first tag push in Task 6 is the integration test.

- [ ] **Step 4: Changelog + commit**

Under `### Added`: `- `release.yml`: on every `v*` tag, reruns the full CI workflow as a gate, pulls the universal macOS glow helper from that run, and publishes the goreleaser artifacts with release notes taken from the tag's `CHANGELOG.md` section. All GitHub Actions are pinned by commit SHA.`

```bash
git add .github/workflows/release.yml .github/workflows/ci.yml CHANGELOG.md
git commit -m "ci: release workflow on semver tags, pin actions by SHA"
```

---

### Task 5: `install.sh` bootstrap with checksum verification + docs

**Files:**
- Create: `install.sh`
- Create: `scripts/test-install-sh.sh`
- Modify: `mise.toml` (`lint:scripts` first entry becomes `shellcheck -s sh install.sh scripts/*.sh`, add `sh scripts/test-install-sh.sh`)
- Modify: `.github/workflows/ci.yml` (shell scripts step: add `install.sh` to shellcheck and run the test)
- Modify: `README.md:8-14`, `docs/install.md:7-16`, `docs/glow.md` (Status table, macOS row)
- Modify: `CHANGELOG.md`

**Interfaces:**
- Consumes: asset names from Task 3: `push-it_<os>_<arch>.tar.gz` and `checksums.txt` under `https://github.com/InfiniteRoomLabs/push-it/releases/latest/download/`.
- Produces: `install.sh` honoring env overrides `PUSH_IT_BASE_URL` (default the URL above), `PUSH_IT_BIN_DIR` (default `$HOME/.local/bin`), `PUSH_IT_VERSION` (default `latest`; any other value replaces `latest/download` with `download/<version>`).

- [ ] **Step 1: Write the failing test**

`scripts/test-install-sh.sh`:

```sh
#!/bin/sh
# Exercises install.sh end to end against a fake release served over file://.
set -eu
cd "$(dirname "$0")/.."
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
os=$(uname -s | tr '[:upper:]' '[:lower:]')
case $(uname -m) in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo "unsupported test arch"; exit 1 ;;
esac

# fake release: a tarball whose push-it is a script that echoes its argv
mkdir -p "$tmp/rel" "$tmp/pkg"
printf '#!/bin/sh\necho "fake push-it: $*"\n' > "$tmp/pkg/push-it"
chmod +x "$tmp/pkg/push-it"
asset="push-it_${os}_${arch}.tar.gz"
tar -C "$tmp/pkg" -czf "$tmp/rel/$asset" push-it
(cd "$tmp/rel" && sha256sum "$asset" > checksums.txt)

# happy path: installs, then execs `push-it install "$@"`
out=$(PUSH_IT_BASE_URL="file://$tmp/rel" PUSH_IT_BIN_DIR="$tmp/bin" sh install.sh --sound --yes)
case $out in
  *"fake push-it: install --sound --yes"*) ;;
  *) printf 'unexpected output:\n%s\n' "$out"; exit 1 ;;
esac
[ -x "$tmp/bin/push-it" ] || { echo "binary not installed"; exit 1; }

# tampered tarball must be refused
printf 'garbage' >> "$tmp/rel/$asset"
if PUSH_IT_BASE_URL="file://$tmp/rel" PUSH_IT_BIN_DIR="$tmp/bin2" sh install.sh --yes >/dev/null 2>&1; then
  echo "expected checksum failure"; exit 1
fi
[ ! -e "$tmp/bin2/push-it" ] || { echo "tampered binary was installed"; exit 1; }

echo "install.sh: ok"
```

- [ ] **Step 2: Run it to verify it fails**

Run: `sh scripts/test-install-sh.sh`
Expected: fails because `install.sh` does not exist.

- [ ] **Step 3: Write `install.sh`**

```sh
#!/bin/sh
# push-it bootstrap: download the latest release for this machine, verify its
# checksum, put the binary in ~/.local/bin, then run `push-it install "$@"`.
#
#   curl -fsSL https://raw.githubusercontent.com/InfiniteRoomLabs/push-it/main/install.sh | sh -s -- --all
#
# Env overrides: PUSH_IT_VERSION (default latest), PUSH_IT_BIN_DIR
# (default ~/.local/bin), PUSH_IT_BASE_URL (release asset base URL).
set -eu

version=${PUSH_IT_VERSION:-latest}
bin_dir=${PUSH_IT_BIN_DIR:-$HOME/.local/bin}
if [ -n "${PUSH_IT_BASE_URL:-}" ]; then
  base=$PUSH_IT_BASE_URL
elif [ "$version" = latest ]; then
  base=https://github.com/InfiniteRoomLabs/push-it/releases/latest/download
else
  base=https://github.com/InfiniteRoomLabs/push-it/releases/download/$version
fi

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case $os in
  linux|darwin) ;;
  *) echo "install.sh: unsupported OS '$os' (on Windows, download the zip from the releases page)" >&2; exit 1 ;;
esac
case $(uname -m) in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo "install.sh: unsupported architecture '$(uname -m)'" >&2; exit 1 ;;
esac
asset="push-it_${os}_${arch}.tar.gz"

fetch() {
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL -o "$2" "$1"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$2" "$1"
  else
    echo "install.sh: need curl or wget" >&2; exit 1
  fi
}

sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d' ' -f1
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | cut -d' ' -f1
  else
    echo "install.sh: need sha256sum or shasum" >&2; exit 1
  fi
}

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
echo "downloading $base/$asset"
fetch "$base/$asset" "$tmp/$asset"
fetch "$base/checksums.txt" "$tmp/checksums.txt"

want=$(grep " $asset\$" "$tmp/checksums.txt" | cut -d' ' -f1)
[ -n "$want" ] || { echo "install.sh: $asset not listed in checksums.txt" >&2; exit 1; }
got=$(sha256 "$tmp/$asset")
[ "$got" = "$want" ] || { echo "install.sh: checksum mismatch for $asset (got $got, want $want)" >&2; exit 1; }

tar -C "$tmp" -xzf "$tmp/$asset" push-it
mkdir -p "$bin_dir"
mv "$tmp/push-it" "$bin_dir/push-it"
chmod +x "$bin_dir/push-it"
echo "installed $bin_dir/push-it"
case ":$PATH:" in
  *":$bin_dir:"*) ;;
  *) echo "note: $bin_dir is not on your PATH" >&2 ;;
esac

exec "$bin_dir/push-it" install "$@"
```

- [ ] **Step 4: Run the test and shellcheck until green**

Run: `sh scripts/test-install-sh.sh && shellcheck -s sh install.sh && sh -n install.sh`
Expected: `install.sh: ok`, shellcheck silent. (`curl` supports `file://` URLs, which is what the test relies on; wget on some distros does not, so the fallback order matters.)

- [ ] **Step 5: Wire the test into mise and CI**

In `mise.toml` `[tasks."lint:scripts"]` replace the first entry with `"shellcheck -s sh install.sh scripts/*.sh"` and add `"sh scripts/test-install-sh.sh"` after the changelog test. In `.github/workflows/ci.yml` the `shell scripts` step becomes:

```yaml
      - name: shell scripts
        run: |
          shellcheck -s sh install.sh scripts/*.sh
          sh scripts/test-changelog-notes.sh
          sh scripts/test-install-sh.sh
```

- [ ] **Step 6: Docs**

`README.md` Install section becomes:

~~~markdown
## Install

```sh
curl -fsSL https://raw.githubusercontent.com/InfiniteRoomLabs/push-it/main/install.sh | sh -s -- --all
```

That downloads the latest release for your OS/arch (Linux and macOS, amd64 and arm64), verifies its checksum, puts `push-it` in `~/.local/bin`, and runs `push-it install --all`. Drop `--all` to be asked per component, or pass `--sound`, `--hue`, `--glow` explicitly. Windows: grab the zip from the [releases page](https://github.com/InfiniteRoomLabs/push-it/releases) and run `push-it install`. From source: `go install github.com/InfiniteRoomLabs/push-it/cmd/push-it@latest` (note: a source build has no macOS glow; see [docs/glow.md](docs/glow.md)).

Details, kill switches, and uninstall: [docs/install.md](docs/install.md).
~~~

`docs/install.md` Quick start section: replace the "Until the first release ships" paragraph and code block with the same one-liner, the env overrides (`PUSH_IT_VERSION`, `PUSH_IT_BIN_DIR`), the Windows zip note, and `go install` as the alternative (same caveat about `-tags glowhelper`). Keep the rest of the section as is.

`docs/glow.md` macOS Status row: change "shipped in release binaries built with `-tags glowhelper`" to "shipped in release binaries and via `install.sh`; built with `-tags glowhelper`" (keep the rest of the cell).

- [ ] **Step 7: Lint, test, commit**

Run: `mise run lint && mise run test`
Expected: clean.

Under `### Added`: `- `install.sh`: dependency-free POSIX bootstrap (`curl ... | sh -s -- --all`) that downloads the latest release for the host, verifies `checksums.txt`, installs to `~/.local/bin`, and runs `push-it install`; tested end to end in CI against a local fake release.`

```bash
git add install.sh scripts/test-install-sh.sh mise.toml .github/workflows/ci.yml README.md docs/install.md docs/glow.md CHANGELOG.md
git commit -m "feat(release): install.sh bootstrap with checksum verification"
```

---

### Task 6: Cut v0.1.0 (orchestrator, on `main`)

Not a subagent task: it tags a public release. Steps:

- [ ] Merge the Plan 3 branch to `main` after the final review; push to `origin` and `github`; wait for CI green.
- [ ] `README.md`: add a short "Known limitations" section (PUSHIT-11): Linux needs a running PulseAudio/PipeWire server (bare ALSA gets no sound; `doctor` does not probe it yet); GNOME glow needs one logout after install (Wayland cannot hot-load extensions); Windows audio, detached spawn, and glow are CI-built and unit-tested, not yet hand-verified; macOS glow is hand-verified only if PUSHIT-12 was done, otherwise say so.
- [ ] `CHANGELOG.md`: rename `## [Unreleased]` to `## [0.1.0] - <today>`, put a fresh empty `## [Unreleased]` above it, and add the compare links at the bottom (`[Unreleased]: https://github.com/InfiniteRoomLabs/push-it/compare/v0.1.0...HEAD`, `[0.1.0]: https://github.com/InfiniteRoomLabs/push-it/releases/tag/v0.1.0`). `sh scripts/changelog-notes.sh v0.1.0` must print the section.
- [ ] Redaction scan of the working tree and the new commits (`agent-ops/scripts/resolve-redaction-terms.py`, then grep); commit `docs: prepare v0.1.0` and push to both remotes.
- [ ] Confirm with the user, then `mise run release -- v0.1.0`. Watch `gh run watch`; verify the release page has 6 archives, `checksums.txt`, and the extension zip; download the darwin arm64 archive and confirm the binary is larger than the linux one (the embedded helper).
- [ ] Dogfood: `curl -fsSL https://raw.githubusercontent.com/InfiniteRoomLabs/push-it/main/install.sh | sh -s -- --sound --hue --glow --yes` on this machine (it replaces `~/.local/bin/push-it`), then `push-it version` prints `push-it v0.1.0 (<sha>)`, `push-it doctor`, and a real push. Record the dogfood result in `CHANGELOG.md` under the new `[Unreleased]`.
- [ ] Close PUSHIT-4, -5, -6, -7, -11 in Plane; leave -12 open until someone runs macOS/Windows by hand.

