# push-it Core Implementation Plan (Plan 1 of 3)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A cross-platform `push-it` Go binary that plays a random clip and runs a Hue rainbow on `git pre-push`, with a reversible installer, the clip-cutting toolkit, tests, CI, and docs  -  glow stubbed behind a stable interface for Plan 2.

**Architecture:** One Go module, one binary, stdlib-first. Small `internal/` packages with one job each (`config`, `clips`, `player`, `hue`, `lockfile`, `glow`, `hook`, `clipper`, `installer`) wired together in `cmd/push-it`. Every external effect (git, HTTP, audio, process spawning) sits behind an injectable function or interface so tests run headless on Linux, macOS, and Windows runners.

**Tech Stack:** Go 1.26 (`flag`, `encoding/json`, `net/http`, `os/exec`, `math/rand/v2`), `github.com/ebitengine/oto/v3` (audio out, purego  -  no cgo), `github.com/hajimehoshi/go-mp3` (MP3 decode), `golang.org/x/sys/windows` (detached process flags). GitHub Actions, mise, staticcheck.

**Spec:** `docs/superpowers/specs/2026-08-20-push-it-design.md`

## Global Constraints

- Module path `github.com/InfiniteRoomLabs/push-it`; Go `1.26`; `CGO_ENABLED=0` must build on linux/darwin/windows x amd64/arm64.
- Dependencies limited to `github.com/ebitengine/oto/v3` (audio on macOS/Windows; its Linux driver needs cgo so it is build-tagged out there), `github.com/jfreymuth/pulse` (pure-Go PulseAudio/PipeWire client, Linux audio), `github.com/hajimehoshi/go-mp3`, `golang.org/x/sys`. No CLI framework, no TOML lib, no ffmpeg.
- Repository is public from the first commit: no personal paths, IPs, Hue light IDs, vault names, or internal hostnames anywhere, including tests, docs, and this plan. Examples use `192.168.1.2`, light `1`, `<your-...>` placeholders.
- No audio files committed except the 0.1 s silent MP3 test fixture.
- `pre-push` must never fail or delay a push: `push-it hook pre-push` exits 0 in under 100 ms and logs errors to `<config dir>/push-it.log`.
- Config file mode `0600`, directory `0700`. Kill switches: `NO_PUSH_IT`, `NO_SOUND`, `NO_RAINBOW`, `NO_GLOW` (value `1`).
- Every commit includes a `CHANGELOG.md` entry under `## [Unreleased]` (repo hook enforces it) and stages with `git add <files>` **then** commits in a separate command (no `-a`/`-am`).
- Markdown prose is never hard-wrapped.
- Commit messages end with `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`.

---

## File structure

```
go.mod / go.sum
LICENSE                         MIT, "Infinite Room Labs LLC"
README.md / CHANGELOG.md / .gitignore / mise.toml
cmd/push-it/main.go             run(args, stdin, stdout, stderr) int; subcommand dispatch
cmd/push-it/main_test.go
cmd/push-it/install.go          install / uninstall / doctor subcommands
cmd/push-it/clips.go            clips cut / clips review subcommands
internal/config/config.go       Config types, Dir(), Load(), Save(), env overrides
internal/config/config_test.go
internal/clips/clips.go         List(dir), Pick(files, rng)
internal/clips/clips_test.go
internal/player/clip.go         Clip, Duration(), Slice()
internal/player/wav.go          DecodeWAV, EncodeWAV
internal/player/mp3.go          DecodeMP3 (go-mp3)
internal/player/decode.go       Decode(path) by extension
internal/player/play.go         Play(ctx, clip, volume) via oto
internal/player/player_test.go
internal/player/testdata/silence.mp3
internal/hue/hue.go             Client, Ping, Burst
internal/hue/hue_test.go
internal/lockfile/lockfile.go   Acquire(path, stale)
internal/lockfile/lockfile_test.go
internal/glow/glow.go           params + Run/Install/BackendName hooks (stub backend; Plan 2 fills)
internal/glow/glow_test.go
internal/hook/hook.go           Enabled(), Run(), PrePush()
internal/hook/detach_unix.go    Setsid
internal/hook/detach_windows.go DETACHED_PROCESS
internal/hook/hook_test.go
internal/clipper/group.go       Word, Phrase, Options, Group()
internal/clipper/cut.go         Cut()
internal/clipper/review.go      Review()
internal/clipper/clipper_test.go
internal/installer/git.go       Git interface, CLIGit
internal/installer/hook.go      WireHook, UnwireHook, marker block
internal/installer/installer_test.go
tools/clipper/transcribe.py     faster-whisper word timestamps (uv PEP 723)
docs/install.md / docs/make-your-own-clips.md / docs/hue.md / docs/glow.md / docs/migrating.md
.github/workflows/ci.yml
```

---

### Task 1: Module scaffold, license, mise, CLI skeleton

**Files:**
- Create: `go.mod`, `LICENSE`, `.gitignore`, `mise.toml`, `README.md`, `cmd/push-it/main.go`, `cmd/push-it/main_test.go`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Produces: `func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int` in package `main`; `var version = "dev"` (set by `-ldflags "-X main.version=..."`); `commands map[string]func(args []string, stdin io.Reader, stdout, stderr io.Writer) int` that later tasks register into.

- [ ] **Step 1: Initialise the module and toolchain**

```bash
cd ~/projects/infinite-room-labs/push-it
go mod init github.com/InfiniteRoomLabs/push-it
mise use go@1.26.5
mise use aqua:goreleaser/goreleaser aqua:dominikh/go-tools/staticcheck
```

`mise use` pins exact versions into `mise.toml`. Then append tasks to `mise.toml`:

```toml
[tasks.test]
run = "go test ./..."

[tasks.lint]
run = [
  "test -z \"$(gofmt -l .)\" || (gofmt -l . && exit 1)",
  "go vet ./...",
  "staticcheck ./...",
]

[tasks.build]
run = "go build -o bin/push-it ./cmd/push-it"
```

- [ ] **Step 2: Write LICENSE and .gitignore**

`LICENSE`: the standard MIT text with `Copyright (c) 2026 Infinite Room Labs LLC`.

`.gitignore`:

```
bin/
dist/
*.shell-extension.zip
.claude/
.codex/
.gemini/
.DS_Store
```

- [ ] **Step 3: Write the failing CLI test**

`cmd/push-it/main_test.go`:

```go
package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionSubcommand(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"version"}, strings.NewReader(""), &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, errOut.String())
	}
	if !strings.HasPrefix(out.String(), "push-it ") {
		t.Fatalf("stdout = %q, want prefix %q", out.String(), "push-it ")
	}
}

func TestUnknownSubcommand(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"bogus"}, strings.NewReader(""), &out, &errOut)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "unknown command") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestNoArgsPrintsUsage(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run(nil, strings.NewReader(""), &out, &errOut)
	if code != 2 || !strings.Contains(errOut.String(), "usage:") {
		t.Fatalf("code = %d, stderr = %q", code, errOut.String())
	}
}
```

- [ ] **Step 4: Run it to verify it fails**

Run: `go test ./cmd/push-it/`
Expected: FAIL  -  `undefined: run`

- [ ] **Step 5: Write the CLI skeleton**

`cmd/push-it/main.go`:

```go
// Command push-it celebrates git pushes with sound, light, and screen glow.
package main

import (
	"fmt"
	"io"
	"os"
	"sort"
)

// version is overwritten at build time via -ldflags "-X main.version=v1.2.3".
var version = "dev"

type command func(args []string, stdin io.Reader, stdout, stderr io.Writer) int

// commands is populated by init() functions in the other files of this package.
var commands = map[string]command{}

func init() {
	commands["version"] = func(_ []string, _ io.Reader, stdout, _ io.Writer) int {
		fmt.Fprintf(stdout, "push-it %s\n", version)
		return 0
	}
}

func usage(w io.Writer) {
	names := make([]string, 0, len(commands))
	for n := range commands {
		names = append(names, n)
	}
	sort.Strings(names)
	fmt.Fprintln(w, "usage: push-it <command> [flags]")
	fmt.Fprintln(w, "commands:")
	for _, n := range names {
		fmt.Fprintf(w, "  %s\n", n)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return 2
	}
	cmd, ok := commands[args[0]]
	if !ok {
		fmt.Fprintf(stderr, "push-it: unknown command %q\n", args[0])
		usage(stderr)
		return 2
	}
	return cmd(args[1:], stdin, stdout, stderr)
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
```

- [ ] **Step 6: Run tests and lint**

Run: `go test ./... && mise run lint`
Expected: PASS, no lint output.

- [ ] **Step 7: Write the README stub and CHANGELOG entry**

`README.md`:

```markdown
# push-it

Celebrate every `git push`: a random "push it" clip, a rainbow burst on a Philips Hue light, and an animated rainbow frame around your screen for exactly as long as the clip plays. Each effect is optional. One binary, no runtime dependencies.

Ships no audio  -  you cut your own clips from a track you own with the bundled toolkit. See [docs/make-your-own-clips.md](docs/make-your-own-clips.md).

## Install

See [docs/install.md](docs/install.md).

## Docs

- [docs/install.md](docs/install.md)  -  install, uninstall, `doctor`
- [docs/make-your-own-clips.md](docs/make-your-own-clips.md)  -  transcribe, cut, review
- [docs/hue.md](docs/hue.md)  -  Philips Hue setup
- [docs/glow.md](docs/glow.md)  -  screen glow per platform
- [docs/migrating.md](docs/migrating.md)  -  moving from a hand-rolled hook

## License

MIT  -  see [LICENSE](LICENSE).
```

Add under `## [Unreleased]` -> `### Added` in `CHANGELOG.md`: `- Go module scaffold, MIT license, mise toolchain, and the \`push-it version\` command.`

- [ ] **Step 8: Commit**

```bash
git add go.mod LICENSE .gitignore mise.toml README.md CHANGELOG.md cmd/push-it/main.go cmd/push-it/main_test.go
git commit -m "feat: module scaffold and CLI skeleton

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: config package

**Files:**
- Create: `internal/config/config.go`, `internal/config/config_test.go`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Produces:
  - `type Config struct { Sound Sound; Hue Hue; Glow Glow; InstallState InstallState }` with JSON tags exactly as in the spec, plus `InstallState.PrePushCreatedByUs bool` (`json:"pre_push_created_by_us"`) and `Hue.CertSHA256 string` (`json:"cert_sha256"`, the TOFU-pinned bridge certificate fingerprint).
  - `func Dir() (string, error)`  -  `$PUSH_IT_CONFIG_DIR` if set, else `os.UserConfigDir()/push-it`.
  - `func Path() (string, error)`  -  `Dir()/config.json`.
  - `func Default() (*Config, error)`  -  sound enabled, volume 0.7, clips dir `Dir()/clips`, hue disabled light 1, glow enabled.
  - `func Load() (*Config, error)`  -  default when the file is missing; env overrides `PUSH_IT_HUE_BRIDGE`, `PUSH_IT_HUE_KEY`, `PUSH_IT_HUE_LIGHT` applied after.
  - `func (c *Config) Save() error`  -  dir 0700, file 0600.
  - `func (c *Config) LogPath() string`, `func (c *Config) LockPath() string`  -  `Dir()/push-it.log`, `Dir()/play.lock` (computed from the dir used at Load time; stored unexported).

- [ ] **Step 1: Write the failing tests**

`internal/config/config_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDirHonoursOverride(t *testing.T) {
	t.Setenv("PUSH_IT_CONFIG_DIR", "/tmp/x/push-it")
	d, err := Dir()
	if err != nil || d != "/tmp/x/push-it" {
		t.Fatalf("Dir() = %q, %v", d, err)
	}
}

func TestLoadReturnsDefaultWhenMissing(t *testing.T) {
	t.Setenv("PUSH_IT_CONFIG_DIR", t.TempDir())
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !c.Sound.Enabled || c.Sound.Volume != 0.7 || c.Hue.Enabled || c.Hue.Light != 1 || !c.Glow.Enabled {
		t.Fatalf("unexpected defaults: %+v", c)
	}
	if filepath.Base(c.Sound.ClipsDir) != "clips" {
		t.Fatalf("clips dir = %q", c.Sound.ClipsDir)
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PUSH_IT_CONFIG_DIR", dir)
	c, _ := Load()
	c.Hue.Enabled = true
	c.Hue.Bridge = "192.168.1.2"
	c.Hue.Key = "secret"
	c.InstallState.HooksPathSetByUs = true
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		fi, _ := os.Stat(filepath.Join(dir, "config.json"))
		if fi.Mode().Perm() != 0o600 {
			t.Fatalf("mode = %o, want 600", fi.Mode().Perm())
		}
	}
	c2, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c2.Hue.Bridge != "192.168.1.2" || c2.Hue.Key != "secret" || !c2.InstallState.HooksPathSetByUs {
		t.Fatalf("round trip lost data: %+v", c2)
	}
}

func TestEnvOverridesHue(t *testing.T) {
	t.Setenv("PUSH_IT_CONFIG_DIR", t.TempDir())
	t.Setenv("PUSH_IT_HUE_BRIDGE", "10.0.0.9")
	t.Setenv("PUSH_IT_HUE_KEY", "envkey")
	t.Setenv("PUSH_IT_HUE_LIGHT", "7")
	c, _ := Load()
	if c.Hue.Bridge != "10.0.0.9" || c.Hue.Key != "envkey" || c.Hue.Light != 7 {
		t.Fatalf("env overrides not applied: %+v", c.Hue)
	}
}

func TestLogAndLockPaths(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PUSH_IT_CONFIG_DIR", dir)
	c, _ := Load()
	if c.LogPath() != filepath.Join(dir, "push-it.log") || c.LockPath() != filepath.Join(dir, "play.lock") {
		t.Fatalf("paths: %q %q", c.LogPath(), c.LockPath())
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/config/`
Expected: FAIL  -  `undefined: Dir` etc.

- [ ] **Step 3: Implement**

`internal/config/config.go`:

```go
// Package config loads and saves push-it's JSON configuration.
package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
)

type Sound struct {
	Enabled  bool    `json:"enabled"`
	ClipsDir string  `json:"clips_dir"`
	Volume   float64 `json:"volume"`
}

type Hue struct {
	Enabled    bool   `json:"enabled"`
	Bridge     string `json:"bridge"`
	Key        string `json:"key"`
	Light      int    `json:"light"`
	CertSHA256 string `json:"cert_sha256"` // trust-on-first-use pin of the bridge's TLS cert
}

type Glow struct {
	Enabled bool `json:"enabled"`
}

// InstallState records exactly what `push-it install` changed so that
// `push-it uninstall` can reverse it and nothing else.
type InstallState struct {
	HooksPathSetByUs        bool   `json:"hooks_path_set_by_us"`
	HooksPath               string `json:"hooks_path"`
	PrePushAppendedTo       string `json:"pre_push_line_appended_to"`
	PrePushCreatedByUs      bool   `json:"pre_push_created_by_us"`
	GnomeExtensionInstalled bool   `json:"gnome_extension_installed"`
	MacOSHelperPath         string `json:"macos_helper_path"`
}

type Config struct {
	Sound        Sound        `json:"sound"`
	Hue          Hue          `json:"hue"`
	Glow         Glow         `json:"glow"`
	InstallState InstallState `json:"install_state"`

	dir string
}

// Dir returns the configuration directory: $PUSH_IT_CONFIG_DIR if set,
// otherwise the OS user config dir plus "push-it".
func Dir() (string, error) {
	if d := os.Getenv("PUSH_IT_CONFIG_DIR"); d != "" {
		return d, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "push-it"), nil
}

// Path returns the config file path.
func Path() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.json"), nil
}

// Default returns a config with sound and glow enabled and hue disabled.
func Default() (*Config, error) {
	d, err := Dir()
	if err != nil {
		return nil, err
	}
	return &Config{
		Sound: Sound{Enabled: true, ClipsDir: filepath.Join(d, "clips"), Volume: 0.7},
		Hue:   Hue{Enabled: false, Light: 1},
		Glow:  Glow{Enabled: true},
		dir:   d,
	}, nil
}

// Load reads the config file, falling back to Default when it does not
// exist, then applies PUSH_IT_HUE_* environment overrides.
func Load() (*Config, error) {
	c, err := Default()
	if err != nil {
		return nil, err
	}
	p := filepath.Join(c.dir, "config.json")
	data, err := os.ReadFile(p)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return nil, err
	default:
		if err := json.Unmarshal(data, c); err != nil {
			return nil, err
		}
	}
	applyEnv(c)
	return c, nil
}

func applyEnv(c *Config) {
	if v := os.Getenv("PUSH_IT_HUE_BRIDGE"); v != "" {
		c.Hue.Bridge = v
	}
	if v := os.Getenv("PUSH_IT_HUE_KEY"); v != "" {
		c.Hue.Key = v
	}
	if v := os.Getenv("PUSH_IT_HUE_LIGHT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Hue.Light = n
		}
	}
}

// Save writes the config with restrictive permissions (it holds the Hue key).
func (c *Config) Save() error {
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(c.dir, "config.json"), append(data, '\n'), 0o600)
}

// Dir returns the directory this config was loaded from.
func (c *Config) Dir() string { return c.dir }

// LogPath is where the detached hook run writes its log.
func (c *Config) LogPath() string { return filepath.Join(c.dir, "push-it.log") }

// LockPath guards against overlapping playbacks.
func (c *Config) LockPath() string { return filepath.Join(c.dir, "play.lock") }
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/config/`
Expected: PASS

- [ ] **Step 5: Commit**

Add to CHANGELOG `### Added`: `- \`internal/config\`: JSON config at the OS config dir with 0600 permissions and \`PUSH_IT_HUE_*\` env overrides.`

```bash
git add internal/config CHANGELOG.md
git commit -m "feat(config): load/save JSON config with env overrides

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: clips package

**Files:**
- Create: `internal/clips/clips.go`, `internal/clips/clips_test.go`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Produces: `func List(dir string) ([]string, error)` (sorted absolute paths of `*.mp3`/`*.wav`, case-insensitive extension; missing dir -> `nil, nil`); `func Pick(files []string, r *rand.Rand) (string, error)`; `var ErrNoClips = errors.New("no clips found")`.

- [ ] **Step 1: Write the failing tests**

```go
package clips

import (
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"
)

func touch(t *testing.T, p string) {
	t.Helper()
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestListFiltersAndSorts(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "b.mp3"))
	touch(t, filepath.Join(dir, "a.WAV"))
	touch(t, filepath.Join(dir, "notes.txt"))
	touch(t, filepath.Join(dir, "candidates.json"))
	got, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || filepath.Base(got[0]) != "a.WAV" || filepath.Base(got[1]) != "b.mp3" {
		t.Fatalf("List = %v", got)
	}
}

func TestListMissingDirIsEmpty(t *testing.T) {
	got, err := List(filepath.Join(t.TempDir(), "nope"))
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestPickIsDeterministicWithSeed(t *testing.T) {
	files := []string{"a", "b", "c", "d"}
	r1 := rand.New(rand.NewPCG(1, 2))
	r2 := rand.New(rand.NewPCG(1, 2))
	p1, _ := Pick(files, r1)
	p2, _ := Pick(files, r2)
	if p1 != p2 {
		t.Fatalf("same seed, different picks: %q %q", p1, p2)
	}
}

func TestPickEmpty(t *testing.T) {
	if _, err := Pick(nil, rand.New(rand.NewPCG(0, 0))); err != ErrNoClips {
		t.Fatalf("err = %v, want ErrNoClips", err)
	}
}
```

- [ ] **Step 2: Run to verify failure**  -  `go test ./internal/clips/` -> FAIL undefined.

- [ ] **Step 3: Implement**

```go
// Package clips finds and picks sound clips.
package clips

import (
	"errors"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ErrNoClips is returned by Pick when there is nothing to choose from.
var ErrNoClips = errors.New("no clips found")

// List returns the sorted absolute paths of *.mp3 and *.wav files in dir.
// A missing directory yields an empty list, not an error.
func List(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		switch strings.ToLower(filepath.Ext(e.Name())) {
		case ".mp3", ".wav":
			p, err := filepath.Abs(filepath.Join(dir, e.Name()))
			if err != nil {
				return nil, err
			}
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out, nil
}

// Pick returns one file chosen uniformly at random.
func Pick(files []string, r *rand.Rand) (string, error) {
	if len(files) == 0 {
		return "", ErrNoClips
	}
	return files[r.IntN(len(files))], nil
}
```

- [ ] **Step 4: Run tests**  -  PASS.

- [ ] **Step 5: Commit**

CHANGELOG `### Added`: `- \`internal/clips\`: list and randomly pick \`.mp3\`/\`.wav\` clips from a directory.`

```bash
git add internal/clips CHANGELOG.md
git commit -m "feat(clips): list and pick clips

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 4: player package  -  decode, slice, encode, play

**Files:**
- Create: `internal/player/clip.go`, `internal/player/wav.go`, `internal/player/mp3.go`, `internal/player/decode.go`, `internal/player/play.go`, `internal/player/player_test.go`, `internal/player/testdata/silence.mp3`
- Modify: `go.mod`, `go.sum`, `CHANGELOG.md`

**Interfaces:**
- Produces:
  - `type Clip struct { PCM []byte; SampleRate int; Channels int }`  -  interleaved signed 16-bit little-endian.
  - `func (c *Clip) Duration() time.Duration`; `func (c *Clip) Slice(from, to time.Duration) *Clip` (clamped, frame-aligned).
  - `func DecodeWAV(r io.Reader) (*Clip, error)`; `func EncodeWAV(w io.Writer, c *Clip) error`; `func DecodeMP3(r io.Reader) (*Clip, error)`; `func Decode(path string) (*Clip, error)` (by extension, case-insensitive; other -> error).
  - `func Play(ctx context.Context, c *Clip, volume float64) error`  -  blocks until done or ctx cancelled.

- [ ] **Step 1: Add dependencies and the MP3 fixture**

```bash
go get github.com/ebitengine/oto/v3@latest github.com/hajimehoshi/go-mp3@latest
mkdir -p internal/player/testdata
ffmpeg -f lavfi -i anullsrc=r=44100:cl=stereo -t 0.1 -q:a 9 internal/player/testdata/silence.mp3
ls -la internal/player/testdata/silence.mp3   # expect well under 2 KB
```

ffmpeg is used once here, on the developer machine, to produce a committed fixture; it is not a build or test dependency.

- [ ] **Step 2: Write the failing tests**

```go
package player

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// sine builds a mono 16-bit sine wave clip.
func sine(rate int, d time.Duration, hz float64) *Clip {
	n := int(float64(rate) * d.Seconds())
	pcm := make([]byte, n*2)
	for i := 0; i < n; i++ {
		v := int16(math.Sin(2*math.Pi*hz*float64(i)/float64(rate)) * 16000)
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(v))
	}
	return &Clip{PCM: pcm, SampleRate: rate, Channels: 1}
}

func TestDuration(t *testing.T) {
	c := sine(8000, 250*time.Millisecond, 440)
	if c.Duration() != 250*time.Millisecond {
		t.Fatalf("Duration = %v", c.Duration())
	}
}

func TestWAVRoundTrip(t *testing.T) {
	c := sine(8000, 100*time.Millisecond, 440)
	var buf bytes.Buffer
	if err := EncodeWAV(&buf, c); err != nil {
		t.Fatal(err)
	}
	got, err := DecodeWAV(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if got.SampleRate != 8000 || got.Channels != 1 || !bytes.Equal(got.PCM, c.PCM) {
		t.Fatalf("round trip mismatch: rate=%d ch=%d len=%d", got.SampleRate, got.Channels, len(got.PCM))
	}
}

func TestDecodeWAVRejectsNonPCM(t *testing.T) {
	c := sine(8000, 10*time.Millisecond, 440)
	var buf bytes.Buffer
	_ = EncodeWAV(&buf, c)
	b := buf.Bytes()
	binary.LittleEndian.PutUint16(b[20:], 3) // format tag 3 = IEEE float
	if _, err := DecodeWAV(bytes.NewReader(b)); err == nil {
		t.Fatal("expected error for non-PCM WAV")
	}
}

func TestDecodeWAVRejectsGarbage(t *testing.T) {
	if _, err := DecodeWAV(bytes.NewReader([]byte("hello"))); err == nil {
		t.Fatal("expected error")
	}
}

func TestSlice(t *testing.T) {
	c := sine(1000, time.Second, 10)
	s := c.Slice(250*time.Millisecond, 750*time.Millisecond)
	if s.Duration() != 500*time.Millisecond {
		t.Fatalf("slice duration = %v", s.Duration())
	}
	if !bytes.Equal(s.PCM, c.PCM[500:1500]) {
		t.Fatal("slice bytes wrong")
	}
	if c.Slice(-time.Second, 10*time.Second).Duration() != time.Second {
		t.Fatal("slice should clamp to clip bounds")
	}
}

func TestDecodeMP3Fixture(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "silence.mp3"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	c, err := DecodeMP3(f)
	if err != nil {
		t.Fatal(err)
	}
	if c.SampleRate != 44100 || c.Channels != 2 {
		t.Fatalf("rate=%d ch=%d", c.SampleRate, c.Channels)
	}
	if d := c.Duration(); d < 50*time.Millisecond || d > 200*time.Millisecond {
		t.Fatalf("duration = %v, want ~100ms", d)
	}
}

func TestDecodeByExtension(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "Clip.WAV")
	var buf bytes.Buffer
	_ = EncodeWAV(&buf, sine(8000, 10*time.Millisecond, 440))
	_ = os.WriteFile(p, buf.Bytes(), 0o644)
	if _, err := Decode(p); err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(filepath.Join(dir, "x.ogg")); err == nil {
		t.Fatal("expected unsupported extension error")
	}
}
```

- [ ] **Step 3: Run to verify failure**  -  `go test ./internal/player/` -> FAIL undefined.

- [ ] **Step 4: Implement clip.go**

```go
// Package player decodes MP3/WAV clips into 16-bit PCM and plays them.
package player

import "time"

// Clip is interleaved signed 16-bit little-endian PCM.
type Clip struct {
	PCM        []byte
	SampleRate int
	Channels   int
}

func (c *Clip) frameSize() int { return c.Channels * 2 }

// Duration is the playback length of the clip.
func (c *Clip) Duration() time.Duration {
	if c.SampleRate == 0 || c.Channels == 0 {
		return 0
	}
	frames := len(c.PCM) / c.frameSize()
	return time.Duration(frames) * time.Second / time.Duration(c.SampleRate)
}

// Slice returns the portion of the clip between from and to, clamped to the
// clip's bounds and aligned to whole frames. The PCM is copied.
func (c *Clip) Slice(from, to time.Duration) *Clip {
	fs := c.frameSize()
	total := len(c.PCM) / fs
	toFrame := func(d time.Duration) int {
		n := int(d.Seconds() * float64(c.SampleRate))
		if n < 0 {
			return 0
		}
		if n > total {
			return total
		}
		return n
	}
	a, b := toFrame(from), toFrame(to)
	if b < a {
		b = a
	}
	out := make([]byte, (b-a)*fs)
	copy(out, c.PCM[a*fs:b*fs])
	return &Clip{PCM: out, SampleRate: c.SampleRate, Channels: c.Channels}
}
```

- [ ] **Step 5: Implement wav.go**

```go
package player

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// DecodeWAV parses a RIFF/WAVE file containing 16-bit PCM.
func DecodeWAV(r io.Reader) (*Clip, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if len(data) < 12 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, errors.New("wav: not a RIFF/WAVE file")
	}
	var rate, channels int
	pos := 12
	for pos+8 <= len(data) {
		id := string(data[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		body := pos + 8
		if body+size > len(data) {
			return nil, errors.New("wav: truncated chunk")
		}
		switch id {
		case "fmt ":
			if size < 16 {
				return nil, errors.New("wav: short fmt chunk")
			}
			format := binary.LittleEndian.Uint16(data[body:])
			channels = int(binary.LittleEndian.Uint16(data[body+2:]))
			rate = int(binary.LittleEndian.Uint32(data[body+4:]))
			bits := binary.LittleEndian.Uint16(data[body+14:])
			if format != 1 || bits != 16 {
				return nil, fmt.Errorf("wav: unsupported format tag %d / %d-bit (need 16-bit PCM)", format, bits)
			}
		case "data":
			if rate == 0 {
				return nil, errors.New("wav: data chunk before fmt chunk")
			}
			pcm := make([]byte, size)
			copy(pcm, data[body:body+size])
			return &Clip{PCM: pcm, SampleRate: rate, Channels: channels}, nil
		}
		pos = body + size + size%2 // chunks are word-aligned
	}
	return nil, errors.New("wav: no data chunk")
}

// EncodeWAV writes c as a canonical 44-byte-header 16-bit PCM WAV.
func EncodeWAV(w io.Writer, c *Clip) error {
	h := make([]byte, 44)
	copy(h[0:], "RIFF")
	binary.LittleEndian.PutUint32(h[4:], uint32(36+len(c.PCM)))
	copy(h[8:], "WAVE")
	copy(h[12:], "fmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], 1)
	binary.LittleEndian.PutUint16(h[22:], uint16(c.Channels))
	binary.LittleEndian.PutUint32(h[24:], uint32(c.SampleRate))
	binary.LittleEndian.PutUint32(h[28:], uint32(c.SampleRate*c.Channels*2))
	binary.LittleEndian.PutUint16(h[32:], uint16(c.Channels*2))
	binary.LittleEndian.PutUint16(h[34:], 16)
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], uint32(len(c.PCM)))
	if _, err := w.Write(h); err != nil {
		return err
	}
	_, err := w.Write(c.PCM)
	return err
}
```

- [ ] **Step 6: Implement mp3.go and decode.go**

`mp3.go`:

```go
package player

import (
	"io"

	"github.com/hajimehoshi/go-mp3"
)

// DecodeMP3 decodes an MP3 stream. go-mp3 always yields 16-bit stereo.
func DecodeMP3(r io.Reader) (*Clip, error) {
	d, err := mp3.NewDecoder(r)
	if err != nil {
		return nil, err
	}
	pcm, err := io.ReadAll(d)
	if err != nil {
		return nil, err
	}
	return &Clip{PCM: pcm, SampleRate: d.SampleRate(), Channels: 2}, nil
}
```

`decode.go`:

```go
package player

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Decode opens path and decodes it according to its extension.
func Decode(path string) (*Clip, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	switch strings.ToLower(filepath.Ext(path)) {
	case ".wav":
		return DecodeWAV(f)
	case ".mp3":
		return DecodeMP3(f)
	default:
		return nil, fmt.Errorf("decode: unsupported file type %q (need .mp3 or .wav)", filepath.Ext(path))
	}
}
```

- [ ] **Step 7: Implement play.go**

oto allows one context per process, so the first clip fixes the format; later clips must match (true for the hook, which plays one clip, and for review, whose candidates share a source).

```go
package player

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
)

var (
	otoMu   sync.Mutex
	otoCtx  *oto.Context
	otoRate int
	otoCh   int
)

func context(c *Clip) (*oto.Context, error) {
	otoMu.Lock()
	defer otoMu.Unlock()
	if otoCtx != nil {
		if otoRate != c.SampleRate || otoCh != c.Channels {
			return nil, fmt.Errorf("play: clip is %d Hz/%d ch but audio was opened at %d Hz/%d ch", c.SampleRate, c.Channels, otoRate, otoCh)
		}
		return otoCtx, nil
	}
	ctx, ready, err := oto.NewContext(&oto.NewContextOptions{
		SampleRate:   c.SampleRate,
		ChannelCount: c.Channels,
		Format:       oto.FormatSignedInt16LE,
	})
	if err != nil {
		return nil, err
	}
	<-ready
	otoCtx, otoRate, otoCh = ctx, c.SampleRate, c.Channels
	return ctx, nil
}

// Play plays the clip at the given volume (0..1) and blocks until it ends
// or ctx is cancelled.
func Play(ctx context.Context, c *Clip, volume float64) error {
	octx, err := context(c)
	if err != nil {
		return err
	}
	p := octx.NewPlayer(bytes.NewReader(c.PCM))
	p.SetVolume(volume)
	p.Play()
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for p.IsPlaying() {
		select {
		case <-ctx.Done():
			_ = p.Close()
			return ctx.Err()
		case <-tick.C:
		}
	}
	return p.Close()
}
```

Note: the helper is named `context` which shadows the imported package inside that function only; rename it `audioContext` if staticcheck complains.

- [ ] **Step 8: Run tests and lint**

Run: `go test ./internal/player/ && mise run lint`
Expected: PASS. (Playback is not exercised in tests; it is verified manually in Task 13.)

- [ ] **Step 9: Commit**

CHANGELOG `### Added`: `- \`internal/player\`: MP3 (go-mp3) and 16-bit WAV decoding, WAV encoding, slicing, and playback via oto  -  no ffmpeg, no cgo.`

```bash
git add go.mod go.sum internal/player CHANGELOG.md
git commit -m "feat(player): decode mp3/wav, encode wav, play via oto

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 5: hue package

**Files:**
- Create: `internal/hue/hue.go`, `internal/hue/hue_test.go`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Produces: `type Client struct { BaseURL string; Key string; Light int; HTTP *http.Client; Sleep func(time.Duration) }`; `func New(bridge, key string, light int, certSHA256 string) *Client` (BaseURL `https://<bridge>`, 2 s timeout; the transport pins the bridge certificate to `certSHA256`  -  Hue bridges present certificates no public CA signs, so push-it uses trust-on-first-use: `Fingerprint` captures the cert at install time, every later connection must match it); `func Fingerprint(ctx context.Context, bridge string) (string, error)`; `func (c *Client) Ping(ctx) error`; `func (c *Client) Burst(ctx) error`; `var Steps = []int{10922, 21845, 32768, 43690, 54613, 65535}`.

- [ ] **Step 1: Write the failing test**

```go
package hue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type call struct {
	method, path string
	body         map[string]any
}

func newServer(t *testing.T) (*httptest.Server, *[]call) {
	t.Helper()
	var mu sync.Mutex
	var calls []call
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var body map[string]any
		if len(b) > 0 {
			_ = json.Unmarshal(b, &body)
		}
		mu.Lock()
		calls = append(calls, call{r.Method, r.URL.Path, body})
		mu.Unlock()
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"state":{"on":false,"bri":120,"hue":5000,"sat":200,"reachable":true}}`))
			return
		}
		_, _ = w.Write([]byte(`[{"success":{}}]`))
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func fingerprintOf(srv *httptest.Server) string {
	sum := sha256.Sum256(srv.Certificate().Raw)
	return hex.EncodeToString(sum[:])
}

// testClient uses the real pinned transport against the test server's cert.
func testClient(srv *httptest.Server) *Client {
	c := New("ignored", "KEY", 1, fingerprintOf(srv))
	c.BaseURL = srv.URL
	c.Sleep = func(time.Duration) {}
	return c
}

func TestPinnedTransportRejectsOtherCert(t *testing.T) {
	srv, _ := newServer(t)
	c := New("ignored", "KEY", 1, strings.Repeat("00", 32))
	c.BaseURL = srv.URL
	if err := c.Ping(context.Background()); err == nil {
		t.Fatal("mismatched fingerprint must fail")
	}
}

func TestFingerprintMatchesServerCert(t *testing.T) {
	srv, _ := newServer(t)
	got, err := Fingerprint(context.Background(), strings.TrimPrefix(srv.URL, "https://"))
	if err != nil {
		t.Fatal(err)
	}
	if got != fingerprintOf(srv) {
		t.Fatalf("fingerprint = %s, want %s", got, fingerprintOf(srv))
	}
}

func TestBurstSequenceAndRestore(t *testing.T) {
	srv, calls := newServer(t)
	c := testClient(srv)
	if err := c.Burst(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := *calls
	// GET state, PUT prime, 6 hue steps, PUT restore = 9 calls
	if len(got) != 9 {
		t.Fatalf("got %d calls: %+v", len(got), got)
	}
	if got[0].method != "GET" || got[0].path != "/api/KEY/lights/1" {
		t.Fatalf("first call = %+v", got[0])
	}
	if got[1].body["hue"] != float64(0) || got[1].body["bri"] != float64(254) {
		t.Fatalf("prime = %+v", got[1].body)
	}
	for i, h := range Steps {
		if got[2+i].path != "/api/KEY/lights/1/state" || got[2+i].body["hue"] != float64(h) {
			t.Fatalf("step %d = %+v", i, got[2+i])
		}
	}
	last := got[8].body
	if last["on"] != false || last["bri"] != float64(120) || last["hue"] != float64(5000) || last["sat"] != float64(200) {
		t.Fatalf("restore = %+v", last)
	}
	if _, leaked := last["reachable"]; leaked {
		t.Fatal("restore must only send on/bri/hue/sat")
	}
}

func TestBurstFailsFastWhenBridgeDown(t *testing.T) {
	srv, _ := newServer(t)
	c := testClient(srv)
	srv.Close()
	if err := c.Burst(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}

func TestPing(t *testing.T) {
	srv, calls := newServer(t)
	if err := testClient(srv).Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 {
		t.Fatalf("calls = %d", len(*calls))
	}
}
```

- [ ] **Step 2: Run to verify failure**  -  FAIL undefined.

- [ ] **Step 3: Implement**

```go
// Package hue runs a rainbow burst on one Philips Hue light and restores it.
package hue

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

// Steps are the hue-wheel values (0..65535) visited in order.
var Steps = []int{10922, 21845, 32768, 43690, 54613, 65535}

// Client talks to a Hue bridge's v1 API.
type Client struct {
	BaseURL string
	Key     string
	Light   int
	HTTP    *http.Client
	Sleep   func(time.Duration)
}

// pinned returns a TLS config that accepts exactly one certificate: the one
// whose SHA-256 fingerprint is certSHA256. Hue bridges present certificates
// no public CA signs, so chain verification is replaced by this pin
// (trust-on-first-use: captured by Fingerprint at install time).
func pinned(certSHA256 string) *tls.Config {
	return &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true, // chain verification is replaced by the fingerprint pin below
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return errors.New("hue: server sent no certificate")
			}
			sum := sha256.Sum256(rawCerts[0])
			if got := hex.EncodeToString(sum[:]); got != certSHA256 {
				return fmt.Errorf("hue: bridge certificate fingerprint %s does not match pinned %s (re-run `push-it install --hue` if the bridge was replaced)", got, certSHA256)
			}
			return nil
		},
	}
}

// New returns a client for https://<bridge> pinned to certSHA256.
func New(bridge, key string, light int, certSHA256 string) *Client {
	return &Client{
		BaseURL: "https://" + bridge,
		Key:     key,
		Light:   light,
		HTTP: &http.Client{
			Timeout:   2 * time.Second,
			Transport: &http.Transport{TLSClientConfig: pinned(certSHA256)},
		},
		Sleep: time.Sleep,
	}
}

// Fingerprint connects to the bridge once and returns the SHA-256 of its
// leaf certificate, for storing as the trust-on-first-use pin.
func Fingerprint(ctx context.Context, bridge string) (string, error) {
	d := tls.Dialer{Config: &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}} // first contact: nothing to verify against yet
	host := bridge
	if _, _, err := net.SplitHostPort(bridge); err != nil {
		host = net.JoinHostPort(bridge, "443")
	}
	conn, err := d.DialContext(ctx, "tcp", host)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	certs := conn.(*tls.Conn).ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return "", errors.New("hue: bridge sent no certificate")
	}
	sum := sha256.Sum256(certs[0].Raw)
	return hex.EncodeToString(sum[:]), nil
}

type state struct {
	On  bool `json:"on"`
	Bri int  `json:"bri"`
	Hue int  `json:"hue"`
	Sat int  `json:"sat"`
}

func (c *Client) lightURL() string {
	return fmt.Sprintf("%s/api/%s/lights/%d", c.BaseURL, c.Key, c.Light)
}

func (c *Client) get(ctx context.Context, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.lightURL(), nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("hue: GET light: %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

func (c *Client) put(ctx context.Context, body any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.lightURL()+"/state", bytes.NewReader(b))
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("hue: PUT state: %s", resp.Status)
	}
	return nil
}

// Ping fetches the light once to confirm bridge, key, and light ID work.
func (c *Client) Ping(ctx context.Context) error {
	var v struct {
		State state `json:"state"`
	}
	return c.get(ctx, &v)
}

// Burst saves the light's state, runs the hue wheel, and restores it.
func (c *Client) Burst(ctx context.Context) error {
	var saved struct {
		State state `json:"state"`
	}
	if err := c.get(ctx, &saved); err != nil {
		return err
	}
	if err := c.put(ctx, map[string]any{"on": true, "bri": 254, "sat": 254, "hue": 0, "transitiontime": 0}); err != nil {
		return err
	}
	for _, h := range Steps {
		c.Sleep(450 * time.Millisecond)
		if err := c.put(ctx, map[string]any{"hue": h, "transitiontime": 3}); err != nil {
			return err
		}
	}
	c.Sleep(600 * time.Millisecond)
	return c.put(ctx, saved.State)
}
```

- [ ] **Step 4: Run tests**  -  PASS.

- [ ] **Step 5: Commit**

CHANGELOG `### Added`: `- \`internal/hue\`: save -> rainbow burst -> restore against the Hue v1 API with 2 s timeouts and a trust-on-first-use certificate pin.`

```bash
git add internal/hue CHANGELOG.md
git commit -m "feat(hue): rainbow burst with state restore

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 6: lockfile package

**Files:**
- Create: `internal/lockfile/lockfile.go`, `internal/lockfile/lockfile_test.go`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Produces: `func Acquire(path string, stale time.Duration) (release func(), ok bool, err error)`  -  `ok=false` when another live holder exists; a lock older than `stale` is treated as abandoned and taken over.

- [ ] **Step 1: Write the failing tests**

```go
package lockfile

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAcquireAndRelease(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.lock")
	rel, ok, err := Acquire(p, time.Minute)
	if err != nil || !ok {
		t.Fatalf("first acquire: ok=%v err=%v", ok, err)
	}
	if _, _, err := Acquire(p, time.Minute); err != nil {
		t.Fatal(err)
	} else if _, ok2, _ := Acquire(p, time.Minute); ok2 {
		t.Fatal("second acquire should fail while held")
	}
	rel()
	if _, ok3, _ := Acquire(p, time.Minute); !ok3 {
		t.Fatal("acquire after release should succeed")
	}
}

func TestStaleLockIsTakenOver(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.lock")
	_ = os.WriteFile(p, nil, 0o600)
	old := time.Now().Add(-time.Hour)
	_ = os.Chtimes(p, old, old)
	_, ok, err := Acquire(p, time.Minute)
	if err != nil || !ok {
		t.Fatalf("stale takeover: ok=%v err=%v", ok, err)
	}
}
```

- [ ] **Step 2: Run to verify failure**  -  FAIL undefined.

- [ ] **Step 3: Implement**

```go
// Package lockfile provides a tiny cross-platform mutual-exclusion file.
package lockfile

import (
	"errors"
	"io/fs"
	"os"
	"time"
)

// Acquire creates path exclusively. It returns ok=false if another process
// holds a fresh lock. A lock older than stale is removed and retaken.
func Acquire(path string, stale time.Duration) (release func(), ok bool, err error) {
	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_ = f.Close()
			return func() { _ = os.Remove(path) }, true, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, false, err
		}
		fi, serr := os.Stat(path)
		if serr == nil && time.Since(fi.ModTime()) > stale {
			_ = os.Remove(path)
			continue
		}
		return nil, false, nil
	}
	return nil, false, nil
}
```

- [ ] **Step 4: Run tests**  -  PASS.

- [ ] **Step 5: Commit**

CHANGELOG `### Added`: `- \`internal/lockfile\`: exclusive-create lock with stale takeover, so overlapping pushes don't stack playback or fight over the Hue state.`

```bash
git add internal/lockfile CHANGELOG.md
git commit -m "feat(lockfile): cross-platform lock with stale takeover

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 7: glow package (interface + shared params, stub backend)

**Files:**
- Create: `internal/glow/glow.go`, `internal/glow/glow_test.go`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Produces (Plan 2 replaces the stubs via `init()` in build-tagged files):
  - Constants `FrameThickness = 14`, `RotationPeriod = 2 * time.Second`, `PulsePeriod = 600 * time.Millisecond`, `MinOpacity = 0.55`, `MaxOpacity = 1.0`, `DefaultDuration = 3500 * time.Millisecond`.
  - `var Backend = "none"`; `var Run func(ctx context.Context, d time.Duration) error`; `var Install func(st *config.InstallState) error`; `var Uninstall func(st *config.InstallState) error`  -  all default to no-ops returning nil.
  - `func Available() bool`  -  `Backend != "none"`.

- [ ] **Step 1: Write the failing test**

```go
package glow

import (
	"context"
	"testing"
	"time"
)

func TestStubBackendIsNoop(t *testing.T) {
	if Available() {
		t.Skip("a real backend is compiled in")
	}
	if err := Run(context.Background(), 10*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if err := Install(nil); err != nil || Uninstall(nil) != nil {
		t.Fatal("stub install/uninstall must be no-ops")
	}
}

func TestParamsAreSane(t *testing.T) {
	if FrameThickness <= 0 || MinOpacity >= MaxOpacity || MaxOpacity > 1 {
		t.Fatal("bad params")
	}
}
```

- [ ] **Step 2: Run to verify failure**  -  FAIL undefined.

- [ ] **Step 3: Implement**

```go
// Package glow draws an animated rainbow frame around the screen.
//
// The rendering is platform-specific and lives in build-tagged files that
// overwrite Run/Install/Uninstall in init(). The parameters below are the
// single source of truth; the JS and Swift renderers mirror them verbatim.
package glow

import (
	"context"
	"time"

	"github.com/InfiniteRoomLabs/push-it/internal/config"
)

const (
	FrameThickness  = 14                     // px
	RotationPeriod  = 2 * time.Second        // one full trip of the rainbow around the frame
	PulsePeriod     = 600 * time.Millisecond // opacity pulse
	MinOpacity      = 0.55
	MaxOpacity      = 1.0
	DefaultDuration = 3500 * time.Millisecond
)

// Backend names the compiled-in renderer: "none", "gnome", "macos", or "windows".
var Backend = "none"

// Run shows the glow for d, blocking until it ends or ctx is cancelled.
var Run = func(ctx context.Context, d time.Duration) error { return nil }

// Install puts any platform pieces in place (GNOME extension, macOS helper).
var Install = func(st *config.InstallState) error { return nil }

// Uninstall reverses Install.
var Uninstall = func(st *config.InstallState) error { return nil }

// Available reports whether a real backend is compiled in.
func Available() bool { return Backend != "none" }
```

- [ ] **Step 4: Run tests**  -  PASS.

- [ ] **Step 5: Commit**

CHANGELOG `### Added`: `- \`internal/glow\`: shared animation parameters and the backend hook points (no-op until platform renderers land).`

```bash
git add internal/glow CHANGELOG.md
git commit -m "feat(glow): shared params and backend hook points

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 8: hook package  -  switches, orchestration, detached pre-push

**Files:**
- Create: `internal/hook/hook.go`, `internal/hook/detach_unix.go`, `internal/hook/detach_windows.go`, `internal/hook/hook_test.go`
- Modify: `go.mod`, `go.sum` (x/sys), `CHANGELOG.md`

**Interfaces:**
- Produces:
  - `func Enabled(getenv func(string) string, cfg *config.Config) (sound, hue, glow bool)`  -  config flag AND the `NO_*` switch unset; `NO_PUSH_IT=1` disables all three.
  - `type Deps struct { Pick func() (string, error); Decode func(string) (*player.Clip, error); Play func(context.Context, *player.Clip) error; Hue func(context.Context) error; Glow func(context.Context, time.Duration) error }`.
  - `func Run(ctx context.Context, sound, hue, glow bool, d Deps, logf func(string, ...any))`  -  runs hue concurrently, decodes the clip, starts glow with the clip's duration, plays, waits for everything. Errors are logged, never returned.
  - `func PrePush(stdin io.Reader, getenv func(string) string, exe, logPath string) error`  -  drains stdin, returns immediately on `NO_PUSH_IT=1`, else spawns `exe hook --run` detached with stdout/stderr appended to `logPath`.

- [ ] **Step 1: Add x/sys**

```bash
go get golang.org/x/sys@latest
```

- [ ] **Step 2: Write the failing tests**

```go
package hook

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/InfiniteRoomLabs/push-it/internal/config"
	"github.com/InfiniteRoomLabs/push-it/internal/player"
)

// TestMain lets this test binary stand in for the push-it executable when
// PrePush re-execs "exe hook --run": the child exits immediately.
func TestMain(m *testing.M) {
	if os.Getenv("PUSH_IT_TEST_CHILD") == "1" {
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestEnabledRespectsConfigAndSwitches(t *testing.T) {
	cfg := &config.Config{Sound: config.Sound{Enabled: true}, Hue: config.Hue{Enabled: true}, Glow: config.Glow{Enabled: false}}
	s, h, g := Enabled(env(nil), cfg)
	if !s || !h || g {
		t.Fatalf("config only: %v %v %v", s, h, g)
	}
	s, h, g = Enabled(env(map[string]string{"NO_RAINBOW": "1"}), cfg)
	if !s || h || g {
		t.Fatalf("NO_RAINBOW: %v %v %v", s, h, g)
	}
	s, h, g = Enabled(env(map[string]string{"NO_PUSH_IT": "1"}), cfg)
	if s || h || g {
		t.Fatal("NO_PUSH_IT must disable everything")
	}
	cfg.Glow.Enabled = true
	s, h, g = Enabled(env(map[string]string{"NO_SOUND": "1", "NO_GLOW": "1"}), cfg)
	if s || !h || g {
		t.Fatalf("NO_SOUND+NO_GLOW: %v %v %v", s, h, g)
	}
}

func fakeClip(d time.Duration) *player.Clip {
	return &player.Clip{PCM: make([]byte, int(d.Seconds()*1000)*2), SampleRate: 1000, Channels: 1}
}

func TestRunWiresDurationIntoGlowAndRunsAll(t *testing.T) {
	var played, hued atomic.Bool
	var glowFor atomic.Int64
	d := Deps{
		Pick:   func() (string, error) { return "clip.wav", nil },
		Decode: func(string) (*player.Clip, error) { return fakeClip(1200 * time.Millisecond), nil },
		Play:   func(context.Context, *player.Clip) error { played.Store(true); return nil },
		Hue:    func(context.Context) error { hued.Store(true); return nil },
		Glow:   func(_ context.Context, dur time.Duration) error { glowFor.Store(int64(dur)); return nil },
	}
	Run(context.Background(), true, true, true, d, func(string, ...any) {})
	if !played.Load() || !hued.Load() || time.Duration(glowFor.Load()) != 1200*time.Millisecond {
		t.Fatalf("played=%v hued=%v glow=%v", played.Load(), hued.Load(), time.Duration(glowFor.Load()))
	}
}

func TestRunLogsErrorsAndContinues(t *testing.T) {
	var logs []string
	d := Deps{
		Pick:   func() (string, error) { return "", errors.New("no clips") },
		Decode: func(string) (*player.Clip, error) { t.Fatal("decode should not run"); return nil, nil },
		Play:   func(context.Context, *player.Clip) error { t.Fatal("play should not run"); return nil },
		Hue:    func(context.Context) error { return errors.New("bridge down") },
		Glow:   func(_ context.Context, dur time.Duration) error { return nil },
	}
	Run(context.Background(), true, true, true, d, func(f string, a ...any) { logs = append(logs, f) })
	if len(logs) < 2 {
		t.Fatalf("expected pick and hue errors logged, got %v", logs)
	}
}

func TestRunGlowOnlyUsesDefaultDuration(t *testing.T) {
	var glowFor time.Duration
	d := Deps{Glow: func(_ context.Context, dur time.Duration) error { glowFor = dur; return nil }}
	Run(context.Background(), false, false, true, d, func(string, ...any) {})
	if glowFor == 0 {
		t.Fatal("glow should run with a default duration when sound is off")
	}
}

func TestPrePushKillSwitchSkipsSpawn(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "push-it.log")
	err := PrePush(strings.NewReader("refs/heads/main abc refs/heads/main def\n"), env(map[string]string{"NO_PUSH_IT": "1"}), "/nonexistent/exe", logPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(logPath); err == nil {
		t.Fatal("log must not be created when disabled")
	}
}

func TestPrePushSpawnsDetachedAndReturnsFast(t *testing.T) {
	t.Setenv("PUSH_IT_TEST_CHILD", "1")
	logPath := filepath.Join(t.TempDir(), "push-it.log")
	start := time.Now()
	if err := PrePush(strings.NewReader(""), env(nil), os.Args[0], logPath); err != nil {
		t.Fatal(err)
	}
	if el := time.Since(start); el > 100*time.Millisecond {
		t.Fatalf("PrePush took %v, must be < 100ms", el)
	}
	if _, err := os.Stat(logPath); err != nil {
		t.Fatal("log file should exist")
	}
}
```

- [ ] **Step 3: Run to verify failure**  -  FAIL undefined.

- [ ] **Step 4: Implement hook.go**

```go
// Package hook orchestrates the pre-push celebration.
package hook

import (
	"context"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/InfiniteRoomLabs/push-it/internal/config"
	"github.com/InfiniteRoomLabs/push-it/internal/glow"
	"github.com/InfiniteRoomLabs/push-it/internal/player"
)

// Enabled combines config flags with the NO_* kill switches.
func Enabled(getenv func(string) string, cfg *config.Config) (sound, hue, glowOn bool) {
	if getenv("NO_PUSH_IT") == "1" {
		return false, false, false
	}
	sound = cfg.Sound.Enabled && getenv("NO_SOUND") != "1"
	hue = cfg.Hue.Enabled && getenv("NO_RAINBOW") != "1"
	glowOn = cfg.Glow.Enabled && getenv("NO_GLOW") != "1"
	return
}

// Deps are the effects Run drives; tests inject fakes.
type Deps struct {
	Pick   func() (string, error)
	Decode func(path string) (*player.Clip, error)
	Play   func(ctx context.Context, c *player.Clip) error
	Hue    func(ctx context.Context) error
	Glow   func(ctx context.Context, d time.Duration) error
}

// Run performs the celebration. Hue runs concurrently; glow is started with
// the decoded clip's exact duration just before playback. Errors are logged.
func Run(ctx context.Context, sound, hue, glowOn bool, d Deps, logf func(string, ...any)) {
	var wg sync.WaitGroup
	if hue {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := d.Hue(ctx); err != nil {
				logf("hue: %v", err)
			}
		}()
	}
	startGlow := func(dur time.Duration) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := d.Glow(ctx, dur); err != nil {
				logf("glow: %v", err)
			}
		}()
	}
	switch {
	case sound:
		path, err := d.Pick()
		if err != nil {
			logf("clips: %v", err)
			if glowOn {
				startGlow(glow.DefaultDuration)
			}
			break
		}
		clip, err := d.Decode(path)
		if err != nil {
			logf("decode %s: %v", path, err)
			break
		}
		if glowOn {
			startGlow(clip.Duration())
		}
		logf("playing %s (%s)", path, clip.Duration().Round(10*time.Millisecond))
		if err := d.Play(ctx, clip); err != nil {
			logf("play: %v", err)
		}
	case glowOn:
		startGlow(glow.DefaultDuration)
	}
	wg.Wait()
}

// PrePush is what git invokes. It must return in milliseconds: it drains
// stdin, honours NO_PUSH_IT, and re-execs exe as a detached "hook --run".
func PrePush(stdin io.Reader, getenv func(string) string, exe, logPath string) error {
	_, _ = io.Copy(io.Discard, stdin)
	if getenv("NO_PUSH_IT") == "1" {
		return nil
	}
	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer logFile.Close()
	cmd := exec.Command(exe, "hook", "--run")
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
```

- [ ] **Step 5: Implement the detach files**

`detach_unix.go`:

```go
//go:build !windows

package hook

import (
	"os/exec"
	"syscall"
)

// detach puts the child in its own session so it outlives git.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
```

`detach_windows.go`:

```go
//go:build windows

package hook

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// detach starts the child without a console and in its own process group.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP,
	}
}
```

- [ ] **Step 6: Run tests, vet for windows too**

Run: `go test ./internal/hook/ && GOOS=windows go vet ./internal/hook/ && GOOS=darwin go vet ./internal/hook/`
Expected: PASS, no vet output.

- [ ] **Step 7: Commit**

CHANGELOG `### Added`: `- \`internal/hook\`: kill switches, concurrent sound/hue/glow orchestration with the glow synced to the clip length, and a detached \`pre-push\` entry that returns in milliseconds.`

```bash
git add go.mod go.sum internal/hook CHANGELOG.md
git commit -m "feat(hook): orchestration and detached pre-push

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 9: clipper package  -  group, cut, review

**Files:**
- Create: `internal/clipper/group.go`, `internal/clipper/cut.go`, `internal/clipper/review.go`, `internal/clipper/clipper_test.go`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Produces:
  - `type Word struct { Word string; Start, End float64 }` (JSON `word`, `start`, `end`  -  the transcript format written by `tools/clipper/transcribe.py`).
  - `type Phrase struct { ID int; Start, End float64; Label, File string }` (JSON `id`, `start`, `end`, `label`, `file`).
  - `type Options struct { Phrase []string; Allow []string; Gap, Max float64 }`; `func DefaultOptions() Options` -> `{Phrase: ["push","it"], Allow: ["real","good"], Gap: 0.5, Max: 4.0}`.
  - `func Group(words []Word, o Options) []Phrase`.
  - `func Cut(src *player.Clip, phrases []Phrase, pad float64, outDir string) ([]Phrase, error)`  -  writes `NNN-label.wav` + `candidates.json`, returns phrases with `File` set.
  - `func Review(in io.Reader, out io.Writer, play func(*player.Clip) error, candDir, keepTo string) (kept int, err error)`.

- [ ] **Step 1: Write the failing tests**

```go
package clipper

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/InfiniteRoomLabs/push-it/internal/player"
)

func w(s string, start, end float64) Word { return Word{Word: s, Start: start, End: end} }

func TestGroupFindsPhrases(t *testing.T) {
	words := []Word{
		w("ooh", 0, 0.3), w("baby", 0.3, 0.6),
		w("push", 1.0, 1.2), w("it", 1.2, 1.4),
		w("push", 1.5, 1.7), w("it", 1.7, 1.9), w("real", 1.9, 2.1), w("good!", 2.1, 2.4),
		w("yeah", 3.5, 3.8),
		w("push", 5.0, 5.2), w("it", 5.2, 5.4),
		w("push", 6.5, 6.7), w("them", 6.7, 6.9), // not a phrase start
		w("push", 8.0, 8.2), w("it", 9.0, 9.2), // gap too big between push and it
	}
	got := Group(words, DefaultOptions())
	if len(got) != 2 {
		t.Fatalf("got %d phrases: %+v", len(got), got)
	}
	if got[0].Label != "push it push it real good" || got[0].Start != 1.0 || got[0].End != 2.4 || got[0].ID != 1 {
		t.Fatalf("phrase 1 = %+v", got[0])
	}
	if got[1].Label != "push it" || got[1].Start != 5.0 || got[1].ID != 2 {
		t.Fatalf("phrase 2 = %+v", got[1])
	}
}

func TestGroupRespectsMaxDuration(t *testing.T) {
	var words []Word
	for i := 0; i < 20; i++ {
		s := float64(i) * 0.5
		words = append(words, w("push", s, s+0.2), w("it", s+0.2, s+0.4))
	}
	got := Group(words, Options{Phrase: []string{"push", "it"}, Gap: 0.5, Max: 2.0})
	if len(got) < 2 {
		t.Fatalf("expected the run to be split by Max, got %d", len(got))
	}
	for _, p := range got {
		if p.End-p.Start > 2.0 {
			t.Fatalf("phrase exceeds max: %+v", p)
		}
	}
}

func tone(rate int, d time.Duration) *player.Clip {
	return &player.Clip{PCM: make([]byte, int(float64(rate)*d.Seconds())*2), SampleRate: rate, Channels: 1}
}

func TestCutWritesWavsAndManifest(t *testing.T) {
	out := t.TempDir()
	src := tone(8000, 10*time.Second)
	phrases := []Phrase{{ID: 1, Start: 1.0, End: 2.0, Label: "push it"}, {ID: 2, Start: 5.0, End: 5.5, Label: "push it real good"}}
	got, err := Cut(src, phrases, 0.3, out)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].File != "001-push-it.wav" || got[1].File != "002-push-it-real-good.wav" {
		t.Fatalf("files = %q %q", got[0].File, got[1].File)
	}
	c, err := player.Decode(filepath.Join(out, got[0].File))
	if err != nil {
		t.Fatal(err)
	}
	if d := c.Duration(); d < 1590*time.Millisecond || d > 1610*time.Millisecond {
		t.Fatalf("clip 1 duration = %v, want 1.6s (1s + 2x0.3 pad)", d)
	}
	var manifest []Phrase
	data, _ := os.ReadFile(filepath.Join(out, "candidates.json"))
	if err := json.Unmarshal(data, &manifest); err != nil || len(manifest) != 2 || manifest[1].File != got[1].File {
		t.Fatalf("manifest: %v %+v", err, manifest)
	}
}

func TestReviewKeepsAndSkips(t *testing.T) {
	cand, keep := t.TempDir(), t.TempDir()
	src := tone(8000, 3*time.Second)
	_, err := Cut(src, []Phrase{{ID: 1, Start: 0, End: 0.5, Label: "push it"}, {ID: 2, Start: 1, End: 1.5, Label: "push it"}, {ID: 3, Start: 2, End: 2.5, Label: "push it"}}, 0, cand)
	if err != nil {
		t.Fatal(err)
	}
	plays := 0
	in := strings.NewReader("k\nr\ns\nq\n")
	var out bytes.Buffer
	kept, err := Review(in, &out, func(*player.Clip) error { plays++; return nil }, cand, keep)
	if err != nil {
		t.Fatal(err)
	}
	if kept != 1 || plays != 3 { // clip1 play, clip2 play + replay, then skip, then quit before clip3
		t.Fatalf("kept=%d plays=%d", kept, plays)
	}
	if _, err := os.Stat(filepath.Join(keep, "001-push-it.wav")); err != nil {
		t.Fatal("keeper not moved")
	}
	if _, err := os.Stat(filepath.Join(cand, "001-push-it.wav")); err == nil {
		t.Fatal("keeper should be moved, not copied")
	}
}
```

- [ ] **Step 2: Run to verify failure**  -  FAIL undefined.

- [ ] **Step 3: Implement group.go**

```go
// Package clipper turns a word-level transcript into reviewed sound clips.
package clipper

import "strings"

// Word is one transcribed word with timestamps in seconds.
type Word struct {
	Word  string  `json:"word"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// Phrase is a candidate clip.
type Phrase struct {
	ID    int     `json:"id"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Label string  `json:"label"`
	File  string  `json:"file,omitempty"`
}

// Options control grouping.
type Options struct {
	Phrase []string // the words a phrase must start with, in order
	Allow  []string // extra words allowed inside a phrase
	Gap    float64  // max silence (s) between consecutive words
	Max    float64  // max phrase length (s)
}

// DefaultOptions matches the Salt-N-Pepa use case.
func DefaultOptions() Options {
	return Options{Phrase: []string{"push", "it"}, Allow: []string{"real", "good"}, Gap: 0.5, Max: 4.0}
}

func clean(s string) string { return strings.ToLower(strings.Trim(s, ".,!?\"' ")) }

func (o Options) allowed() map[string]bool {
	m := map[string]bool{}
	for _, w := range append(append([]string{}, o.Phrase...), o.Allow...) {
		m[clean(w)] = true
	}
	return m
}

// startsPhrase reports whether words[i:] begins with o.Phrase within Gap.
func (o Options) startsPhrase(words []Word, i int) bool {
	if i+len(o.Phrase) > len(words) {
		return false
	}
	for k, want := range o.Phrase {
		if clean(words[i+k].Word) != clean(want) {
			return false
		}
		if k > 0 && words[i+k].Start-words[i+k-1].End > o.Gap {
			return false
		}
	}
	return true
}

// Group finds phrases: runs of allowed words that begin with o.Phrase,
// separated by no more than Gap seconds, capped at Max seconds.
func Group(words []Word, o Options) []Phrase {
	allowed := o.allowed()
	var out []Phrase
	i := 0
	for i < len(words) {
		if !o.startsPhrase(words, i) {
			i++
			continue
		}
		start := words[i].Start
		j := i + len(o.Phrase)
		for j < len(words) {
			wd := words[j]
			if wd.Start-words[j-1].End > o.Gap || !allowed[clean(wd.Word)] || wd.End-start > o.Max {
				break
			}
			j++
		}
		labels := make([]string, 0, j-i)
		for k := i; k < j; k++ {
			labels = append(labels, clean(words[k].Word))
		}
		out = append(out, Phrase{ID: len(out) + 1, Start: start, End: words[j-1].End, Label: strings.Join(labels, " ")})
		i = j
	}
	return out
}
```

- [ ] **Step 4: Implement cut.go**

```go
package clipper

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/InfiniteRoomLabs/push-it/internal/player"
)

func fileName(p Phrase) string {
	return fmt.Sprintf("%03d-%s.wav", p.ID, strings.ReplaceAll(p.Label, " ", "-"))
}

func seconds(f float64) time.Duration { return time.Duration(f * float64(time.Second)) }

// Cut writes one WAV per phrase (padded by pad seconds on each side) into
// outDir plus candidates.json, and returns the phrases with File set.
func Cut(src *player.Clip, phrases []Phrase, pad float64, outDir string) ([]Phrase, error) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}
	out := make([]Phrase, 0, len(phrases))
	for _, p := range phrases {
		clip := src.Slice(seconds(p.Start-pad), seconds(p.End+pad))
		p.File = fileName(p)
		f, err := os.Create(filepath.Join(outDir, p.File))
		if err != nil {
			return nil, err
		}
		err = player.EncodeWAV(f, clip)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, err
	}
	return out, os.WriteFile(filepath.Join(outDir, "candidates.json"), append(data, '\n'), 0o644)
}
```

- [ ] **Step 5: Implement review.go**

```go
package clipper

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/InfiniteRoomLabs/push-it/internal/player"
)

// Review plays each candidate and asks keep/skip/replay/quit. Keepers are
// moved into keepTo. It returns how many were kept.
func Review(in io.Reader, out io.Writer, play func(*player.Clip) error, candDir, keepTo string) (int, error) {
	data, err := os.ReadFile(filepath.Join(candDir, "candidates.json"))
	if err != nil {
		return 0, err
	}
	var phrases []Phrase
	if err := json.Unmarshal(data, &phrases); err != nil {
		return 0, err
	}
	if err := os.MkdirAll(keepTo, 0o755); err != nil {
		return 0, err
	}
	sc := bufio.NewScanner(in)
	kept := 0
	for i, p := range phrases {
		src := filepath.Join(candDir, p.File)
		clip, err := player.Decode(src)
		if err != nil {
			return kept, err
		}
		fmt.Fprintf(out, "\n[%d/%d] %.2fs..%.2fs  %q\n", i+1, len(phrases), p.Start, p.End, p.Label)
		if err := play(clip); err != nil {
			fmt.Fprintf(out, "  (play failed: %v)\n", err)
		}
		for {
			fmt.Fprint(out, "  [k]eep / [s]kip / [r]eplay / [q]uit > ")
			if !sc.Scan() {
				return kept, sc.Err()
			}
			switch strings.ToLower(strings.TrimSpace(sc.Text())) {
			case "k":
				if err := os.Rename(src, filepath.Join(keepTo, p.File)); err != nil {
					return kept, err
				}
				kept++
			case "s":
			case "r":
				if err := play(clip); err != nil {
					fmt.Fprintf(out, "  (play failed: %v)\n", err)
				}
				continue
			case "q":
				return kept, nil
			default:
				fmt.Fprintln(out, "  ? enter k, s, r, or q")
				continue
			}
			break
		}
	}
	return kept, nil
}
```

- [ ] **Step 6: Run tests**  -  `go test ./internal/clipper/` -> PASS.

- [ ] **Step 7: Commit**

CHANGELOG `### Added`: `- \`internal/clipper\`: group transcript words into phrases, cut padded WAV candidates, and an interactive keep/skip review loop.`

```bash
git add internal/clipper CHANGELOG.md
git commit -m "feat(clipper): phrase grouping, WAV cutting, review loop

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 10: installer package  -  reversible hook wiring

**Files:**
- Create: `internal/installer/git.go`, `internal/installer/hook.go`, `internal/installer/installer_test.go`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Produces:
  - `type Git interface { Get(key string) (string, error); Set(key, value string) error; Unset(key string) error }`; `type CLIGit struct{}` implementing it with `git config --global`. `Get` returns `"", nil` when the key is unset.
  - `func WireHook(g Git, cfgDir, exe string, st *config.InstallState) error`.
  - `func UnwireHook(g Git, st *config.InstallState) error`.
  - `const MarkerStart = "# >>> push-it >>>"`, `const MarkerEnd = "# <<< push-it <<<"`; `func Block(exe string) string`.

- [ ] **Step 1: Write the failing tests**

Tests drive real `git` with `GIT_CONFIG_GLOBAL` pointed at a temp file (git >= 2.32).

```go
package installer

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/InfiniteRoomLabs/push-it/internal/config"
)

func setup(t *testing.T) (cfgDir string, g Git) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	tmp := t.TempDir()
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(tmp, "gitconfig"))
	t.Setenv("HOME", tmp)
	return filepath.Join(tmp, "push-it"), CLIGit{}
}

func TestWireWhenHooksPathUnsetCreatesDirAndSetsIt(t *testing.T) {
	cfgDir, g := setup(t)
	var st config.InstallState
	if err := WireHook(g, cfgDir, "/opt/push-it", &st); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(cfgDir, "hooks")
	if got, _ := g.Get("core.hooksPath"); got != want {
		t.Fatalf("core.hooksPath = %q, want %q", got, want)
	}
	b, err := os.ReadFile(filepath.Join(want, "pre-push"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(b), "#!/bin/sh\n") || !strings.Contains(string(b), "'/opt/push-it' hook pre-push \"$@\" || true") {
		t.Fatalf("hook content:\n%s", b)
	}
	if !st.HooksPathSetByUs || st.HooksPath != want {
		t.Fatalf("state = %+v", st)
	}
	// and uninstall reverses it
	if err := UnwireHook(g, &st); err != nil {
		t.Fatal(err)
	}
	if got, _ := g.Get("core.hooksPath"); got != "" {
		t.Fatalf("core.hooksPath still %q after unwire", got)
	}
	if _, err := os.Stat(filepath.Join(want, "pre-push")); err == nil {
		t.Fatal("pre-push should be removed")
	}
	if st.HooksPathSetByUs || st.HooksPath != "" {
		t.Fatalf("state not reset: %+v", st)
	}
}

func TestWireAppendsToExistingPrePush(t *testing.T) {
	cfgDir, g := setup(t)
	hooks := filepath.Join(t.TempDir(), "myhooks")
	_ = os.MkdirAll(hooks, 0o755)
	orig := "#!/bin/sh\necho existing\n"
	_ = os.WriteFile(filepath.Join(hooks, "pre-push"), []byte(orig), 0o755)
	_ = g.Set("core.hooksPath", hooks)

	var st config.InstallState
	if err := WireHook(g, cfgDir, "/opt/push-it", &st); err != nil {
		t.Fatal(err)
	}
	if err := WireHook(g, cfgDir, "/opt/push-it", &st); err != nil { // idempotent
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(hooks, "pre-push"))
	if strings.Count(string(b), MarkerStart) != 1 || !strings.HasPrefix(string(b), orig) {
		t.Fatalf("hook content:\n%s", b)
	}
	if st.HooksPathSetByUs || st.PrePushAppendedTo != filepath.Join(hooks, "pre-push") || st.PrePushCreatedByUs {
		t.Fatalf("state = %+v", st)
	}
	if err := UnwireHook(g, &st); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(filepath.Join(hooks, "pre-push"))
	if string(b) != orig {
		t.Fatalf("original not restored:\n%s", b)
	}
	if got, _ := g.Get("core.hooksPath"); got != hooks {
		t.Fatal("must not touch a hooksPath we did not set")
	}
}

func TestWireCreatesPrePushInExistingHooksPath(t *testing.T) {
	cfgDir, g := setup(t)
	hooks := filepath.Join(t.TempDir(), "myhooks")
	_ = os.MkdirAll(hooks, 0o755)
	_ = g.Set("core.hooksPath", hooks)
	var st config.InstallState
	if err := WireHook(g, cfgDir, "/opt/push-it", &st); err != nil {
		t.Fatal(err)
	}
	if !st.PrePushCreatedByUs {
		t.Fatalf("state = %+v", st)
	}
	if err := UnwireHook(g, &st); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(hooks, "pre-push")); err == nil {
		t.Fatal("file we created should be removed")
	}
	if _, err := os.Stat(hooks); err != nil {
		t.Fatal("user's hooks dir must remain")
	}
}

func TestWireExpandsTildeHooksPath(t *testing.T) {
	cfgDir, g := setup(t)
	_ = os.MkdirAll(filepath.Join(os.Getenv("HOME"), "h"), 0o755)
	_ = g.Set("core.hooksPath", "~/h")
	var st config.InstallState
	if err := WireHook(g, cfgDir, "/opt/push-it", &st); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), "h", "pre-push")); err != nil {
		t.Fatal("~ in hooksPath must be expanded")
	}
}
```

On Windows `HOME` is not what `~` expands to; the tilde test may be skipped there with `if runtime.GOOS == "windows" { t.Skip() }`.

- [ ] **Step 2: Run to verify failure**  -  FAIL undefined.

- [ ] **Step 3: Implement git.go**

```go
// Package installer wires push-it into git's global pre-push hook reversibly.
package installer

import (
	"errors"
	"os/exec"
	"strings"
)

// Git is the subset of `git config --global` the installer needs.
type Git interface {
	Get(key string) (string, error)
	Set(key, value string) error
	Unset(key string) error
}

// CLIGit shells out to git.
type CLIGit struct{}

func (CLIGit) Get(key string) (string, error) {
	out, err := exec.Command("git", "config", "--global", "--get", key).Output()
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 1 {
		return "", nil // unset
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (CLIGit) Set(key, value string) error {
	return exec.Command("git", "config", "--global", key, value).Run()
}

func (CLIGit) Unset(key string) error {
	err := exec.Command("git", "config", "--global", "--unset", key).Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 5 {
		return nil // already unset
	}
	return err
}
```

- [ ] **Step 4: Implement hook.go**

```go
package installer

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/InfiniteRoomLabs/push-it/internal/config"
)

const (
	MarkerStart = "# >>> push-it >>>"
	MarkerEnd   = "# <<< push-it <<<"
)

// Block is the shell snippet appended to pre-push. The absolute binary path
// is used because GUI git clients often run hooks with a minimal PATH.
func Block(exe string) string {
	return fmt.Sprintf("%s\n'%s' hook pre-push \"$@\" || true\n%s\n", MarkerStart, filepath.ToSlash(exe), MarkerEnd)
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") || p == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[1:])
		}
	}
	return p
}

// WireHook makes git run push-it on pre-push, recording what it changed in st.
func WireHook(g Git, cfgDir, exe string, st *config.InstallState) error {
	hp, err := g.Get("core.hooksPath")
	if err != nil {
		return err
	}
	if hp == "" {
		dir := filepath.Join(cfgDir, "hooks")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "pre-push"), []byte("#!/bin/sh\n"+Block(exe)), 0o755); err != nil {
			return err
		}
		if err := g.Set("core.hooksPath", dir); err != nil {
			return err
		}
		st.HooksPathSetByUs, st.HooksPath, st.PrePushAppendedTo, st.PrePushCreatedByUs = true, dir, "", false
		return nil
	}
	hp = expandHome(hp)
	file := filepath.Join(hp, "pre-push")
	existing, err := os.ReadFile(file)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if err := os.MkdirAll(hp, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(file, []byte("#!/bin/sh\n"+Block(exe)), 0o755); err != nil {
			return err
		}
		st.PrePushCreatedByUs = true
	case err != nil:
		return err
	case strings.Contains(string(existing), MarkerStart):
		// already wired; idempotent
	default:
		content := string(existing)
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		if err := os.WriteFile(file, []byte(content+Block(exe)), 0o755); err != nil {
			return err
		}
	}
	st.HooksPathSetByUs, st.HooksPath, st.PrePushAppendedTo = false, hp, file
	return nil
}

// UnwireHook reverses WireHook and resets st.
func UnwireHook(g Git, st *config.InstallState) error {
	switch {
	case st.HooksPathSetByUs:
		_ = os.Remove(filepath.Join(st.HooksPath, "pre-push"))
		_ = os.Remove(st.HooksPath) // only succeeds if empty  -  never deletes user files
		if err := g.Unset("core.hooksPath"); err != nil {
			return err
		}
	case st.PrePushAppendedTo != "" && st.PrePushCreatedByUs:
		if err := os.Remove(st.PrePushAppendedTo); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	case st.PrePushAppendedTo != "":
		b, err := os.ReadFile(st.PrePushAppendedTo)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if err == nil {
			if err := os.WriteFile(st.PrePushAppendedTo, []byte(stripBlock(string(b))), 0o755); err != nil {
				return err
			}
		}
	}
	st.HooksPathSetByUs, st.HooksPath, st.PrePushAppendedTo, st.PrePushCreatedByUs = false, "", "", false
	return nil
}

// stripBlock removes everything from MarkerStart through MarkerEnd inclusive.
func stripBlock(s string) string {
	start := strings.Index(s, MarkerStart)
	end := strings.Index(s, MarkerEnd)
	if start < 0 || end < 0 || end < start {
		return s
	}
	end += len(MarkerEnd)
	if end < len(s) && s[end] == '\n' {
		end++
	}
	if start > 0 && s[start-1] == '\n' && (start < 2 || s[start-2] == '\n') {
		// keep the file's original trailing newline; drop the one we added
	}
	return s[:start] + s[end:]
}
```

Remove the empty `if` in `stripBlock` if staticcheck flags it (SA9003); it is there only as a reminder that the test expects the original file byte-for-byte, which the `content += "\n"` guard in `WireHook` already guarantees for files that ended in a newline.

- [ ] **Step 5: Run tests**  -  `go test ./internal/installer/` -> PASS (the append test asserts byte-for-byte restoration; fix `stripBlock` until it does).

- [ ] **Step 6: Commit**

CHANGELOG `### Added`: `- \`internal/installer\`: reversible \`core.hooksPath\` / \`pre-push\` wiring with marker blocks; uninstall restores the user's hook byte-for-byte.`

```bash
git add internal/installer CHANGELOG.md
git commit -m "feat(installer): reversible pre-push wiring

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 11: CLI  -  play, hue, glow, hook, clips, install, uninstall, doctor

**Files:**
- Create: `cmd/push-it/effects.go`, `cmd/push-it/install.go`, `cmd/push-it/clips.go`
- Modify: `cmd/push-it/main_test.go`, `CHANGELOG.md`

**Interfaces:**
- Consumes everything above.
- Produces the user-facing commands from the spec table.

- [ ] **Step 1: Add failing CLI tests**

Append to `cmd/push-it/main_test.go`:

```go
func TestHookPrePushHonoursKillSwitch(t *testing.T) {
	t.Setenv("PUSH_IT_CONFIG_DIR", t.TempDir())
	t.Setenv("NO_PUSH_IT", "1")
	var out, errOut bytes.Buffer
	if code := run([]string{"hook", "pre-push"}, strings.NewReader("refs\n"), &out, &errOut); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errOut.String())
	}
}

func TestDoctorOnFreshConfig(t *testing.T) {
	t.Setenv("PUSH_IT_CONFIG_DIR", t.TempDir())
	var out, errOut bytes.Buffer
	if code := run([]string{"doctor"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errOut.String())
	}
	for _, want := range []string{"config:", "sound:", "clips:", "hue:", "glow:"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("doctor output missing %q:\n%s", want, out.String())
		}
	}
}

func TestInstallAndUninstallSoundOnly(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	tmp := t.TempDir()
	t.Setenv("PUSH_IT_CONFIG_DIR", filepath.Join(tmp, "cfg"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(tmp, "gitconfig"))
	t.Setenv("HOME", tmp)
	var out, errOut bytes.Buffer
	if code := run([]string{"install", "--sound", "--yes"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("install code=%d stderr=%s", code, errOut.String())
	}
	if _, err := os.Stat(filepath.Join(tmp, "cfg", "hooks", "pre-push")); err != nil {
		t.Fatal("hook not written")
	}
	if _, err := os.Stat(filepath.Join(tmp, "cfg", "clips")); err != nil {
		t.Fatal("clips dir not created")
	}
	out.Reset()
	if code := run([]string{"uninstall", "--yes"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("uninstall code=%d stderr=%s", code, errOut.String())
	}
	if _, err := os.Stat(filepath.Join(tmp, "cfg", "hooks", "pre-push")); err == nil {
		t.Fatal("hook not removed")
	}
}

func TestInstallInteractiveReadsAnswers(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	tmp := t.TempDir()
	t.Setenv("PUSH_IT_CONFIG_DIR", filepath.Join(tmp, "cfg"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(tmp, "gitconfig"))
	t.Setenv("HOME", tmp)
	var out, errOut bytes.Buffer
	// sound? n   hue? n   glow? n
	if code := run([]string{"install"}, strings.NewReader("n\nn\nn\n"), &out, &errOut); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errOut.String())
	}
	b, _ := os.ReadFile(filepath.Join(tmp, "cfg", "config.json"))
	if !strings.Contains(string(b), `"enabled": false`) || strings.Contains(string(b), `"enabled": true`) {
		t.Fatalf("config:\n%s", b)
	}
}
```

Add `"os"`, `"os/exec"`, `"path/filepath"` to the test imports.

- [ ] **Step 2: Run to verify failure**  -  `go test ./cmd/push-it/` -> FAIL (unknown command exit 2).

- [ ] **Step 3: Write effects.go (play, hue, glow, hook)**

```go
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"os"
	"time"

	"github.com/InfiniteRoomLabs/push-it/internal/clips"
	"github.com/InfiniteRoomLabs/push-it/internal/config"
	"github.com/InfiniteRoomLabs/push-it/internal/glow"
	"github.com/InfiniteRoomLabs/push-it/internal/hook"
	"github.com/InfiniteRoomLabs/push-it/internal/hue"
	"github.com/InfiniteRoomLabs/push-it/internal/lockfile"
	"github.com/InfiniteRoomLabs/push-it/internal/player"
)

func init() {
	commands["play"] = cmdPlay
	commands["hue"] = cmdHue
	commands["glow"] = cmdGlow
	commands["hook"] = cmdHook
}

func loadConfig(stderr io.Writer) (*config.Config, bool) {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(stderr, "push-it: config: %v\n", err)
		return nil, false
	}
	return cfg, true
}

func deps(cfg *config.Config) hook.Deps {
	rng := rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0))
	return hook.Deps{
		Pick: func() (string, error) {
			files, err := clips.List(cfg.Sound.ClipsDir)
			if err != nil {
				return "", err
			}
			return clips.Pick(files, rng)
		},
		Decode: player.Decode,
		Play:   func(ctx context.Context, c *player.Clip) error { return player.Play(ctx, c, cfg.Sound.Volume) },
		Hue:    func(ctx context.Context) error { return hue.New(cfg.Hue.Bridge, cfg.Hue.Key, cfg.Hue.Light, cfg.Hue.CertSHA256).Burst(ctx) },
		Glow:   glow.Run,
	}
}

func cmdPlay(args []string, _ io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("play", flag.ContinueOnError)
	fs.SetOutput(stderr)
	file := fs.String("file", "", "play this file instead of a random clip")
	if fs.Parse(args) != nil {
		return 2
	}
	cfg, ok := loadConfig(stderr)
	if !ok {
		return 1
	}
	d := deps(cfg)
	path := *file
	if path == "" {
		var err error
		if path, err = d.Pick(); err != nil {
			fmt.Fprintf(stderr, "push-it: %v (clips dir: %s)\n", err, cfg.Sound.ClipsDir)
			return 1
		}
	}
	clip, err := d.Decode(path)
	if err != nil {
		fmt.Fprintf(stderr, "push-it: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "playing %s (%s)\n", path, clip.Duration().Round(10*time.Millisecond))
	if err := d.Play(context.Background(), clip); err != nil {
		fmt.Fprintf(stderr, "push-it: %v\n", err)
		return 1
	}
	return 0
}

func cmdHue(_ []string, _ io.Reader, _, stderr io.Writer) int {
	cfg, ok := loadConfig(stderr)
	if !ok {
		return 1
	}
	if cfg.Hue.Bridge == "" || cfg.Hue.Key == "" {
		fmt.Fprintln(stderr, "push-it: hue is not configured; run `push-it install --hue`")
		return 1
	}
	if err := deps(cfg).Hue(context.Background()); err != nil {
		fmt.Fprintf(stderr, "push-it: hue: %v\n", err)
		return 1
	}
	return 0
}

func cmdGlow(args []string, _ io.Reader, _, stderr io.Writer) int {
	fs := flag.NewFlagSet("glow", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dur := fs.Duration("duration", glow.DefaultDuration, "how long to glow")
	if fs.Parse(args) != nil {
		return 2
	}
	if !glow.Available() {
		fmt.Fprintln(stderr, "push-it: no glow backend on this platform/build")
		return 1
	}
	if err := glow.Run(context.Background(), *dur); err != nil {
		fmt.Fprintf(stderr, "push-it: glow: %v\n", err)
		return 1
	}
	return 0
}

// cmdHook handles `hook pre-push` (git entry point) and `hook --run` (the
// detached worker it spawns).
func cmdHook(args []string, stdin io.Reader, _, stderr io.Writer) int {
	fs := flag.NewFlagSet("hook", flag.ContinueOnError)
	fs.SetOutput(stderr)
	runNow := fs.Bool("run", false, "internal: run the celebration in this process")
	if fs.Parse(args) != nil {
		return 2
	}
	cfg, ok := loadConfig(stderr)
	if !ok {
		return 0 // never fail a push
	}
	if *runNow {
		logger := log.New(stderr, "", log.LstdFlags)
		release, got, err := lockfile.Acquire(cfg.LockPath(), 30*time.Second)
		if err != nil {
			logger.Printf("lock: %v", err)
			return 0
		}
		if !got {
			logger.Printf("already playing, skipping")
			return 0
		}
		defer release()
		s, h, g := hook.Enabled(os.Getenv, cfg)
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		hook.Run(ctx, s, h, g, deps(cfg), logger.Printf)
		return 0
	}
	if fs.NArg() != 1 || fs.Arg(0) != "pre-push" {
		fmt.Fprintln(stderr, "usage: push-it hook pre-push")
		return 0
	}
	exe, err := os.Executable()
	if err != nil {
		return 0
	}
	if err := os.MkdirAll(cfg.Dir(), 0o700); err != nil {
		return 0
	}
	if err := hook.PrePush(stdin, os.Getenv, exe, cfg.LogPath()); err != nil {
		fmt.Fprintf(stderr, "[push-it] %v\n", err)
	}
	return 0
}
```

- [ ] **Step 4: Write install.go (install, uninstall, doctor)**

```go
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/InfiniteRoomLabs/push-it/internal/clips"
	"github.com/InfiniteRoomLabs/push-it/internal/config"
	"github.com/InfiniteRoomLabs/push-it/internal/glow"
	"github.com/InfiniteRoomLabs/push-it/internal/hue"
	"github.com/InfiniteRoomLabs/push-it/internal/installer"
)

func init() {
	commands["install"] = cmdInstall
	commands["uninstall"] = cmdUninstall
	commands["doctor"] = cmdDoctor
}

type prompter struct {
	in  *bufio.Scanner
	out io.Writer
}

func (p prompter) yesNo(q string, def bool) bool {
	hint := "[Y/n]"
	if !def {
		hint = "[y/N]"
	}
	fmt.Fprintf(p.out, "%s %s ", q, hint)
	if !p.in.Scan() {
		return def
	}
	switch strings.ToLower(strings.TrimSpace(p.in.Text())) {
	case "y", "yes":
		return true
	case "n", "no":
		return false
	default:
		return def
	}
}

func (p prompter) text(q, def string) string {
	if def != "" {
		fmt.Fprintf(p.out, "%s [%s]: ", q, def)
	} else {
		fmt.Fprintf(p.out, "%s: ", q)
	}
	if !p.in.Scan() {
		return def
	}
	if s := strings.TrimSpace(p.in.Text()); s != "" {
		return s
	}
	return def
}

func glowDefault() bool {
	switch runtime.GOOS {
	case "linux":
		return strings.Contains(strings.ToLower(os.Getenv("XDG_CURRENT_DESKTOP")), "gnome")
	default:
		return true
	}
}

func cmdInstall(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(stderr)
	sound := fs.Bool("sound", false, "install the sound component")
	hueOn := fs.Bool("hue", false, "install the Philips Hue component")
	glowOn := fs.Bool("glow", false, "install the screen glow component")
	all := fs.Bool("all", false, "install everything")
	yes := fs.Bool("yes", false, "accept defaults without prompting")
	if fs.Parse(args) != nil {
		return 2
	}
	cfg, ok := loadConfig(stderr)
	if !ok {
		return 1
	}
	p := prompter{in: bufio.NewScanner(stdin), out: stdout}
	wantSound, wantHue, wantGlow := *sound || *all, *hueOn || *all, *glowOn || *all
	if !*sound && !*hueOn && !*glowOn && !*all {
		if *yes {
			wantSound, wantHue, wantGlow = true, false, glowDefault()
		} else {
			wantSound = p.yesNo("Play a clip on push?", true)
			wantHue = p.yesNo("Rainbow a Philips Hue light on push?", false)
			wantGlow = p.yesNo("Glow the screen edges on push?", glowDefault())
		}
	}
	cfg.Sound.Enabled, cfg.Hue.Enabled, cfg.Glow.Enabled = wantSound, wantHue, wantGlow

	if wantHue {
		if !*yes || cfg.Hue.Bridge == "" || cfg.Hue.Key == "" {
			cfg.Hue.Bridge = p.text("Hue bridge host or IP", cfg.Hue.Bridge)
			cfg.Hue.Key = p.text("Hue API key", cfg.Hue.Key)
			if n, err := strconv.Atoi(p.text("Light ID", strconv.Itoa(cfg.Hue.Light))); err == nil {
				cfg.Hue.Light = n
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		fp, err := hue.Fingerprint(ctx, cfg.Hue.Bridge)
		if err == nil {
			if cfg.Hue.CertSHA256 != "" && cfg.Hue.CertSHA256 != fp {
				fmt.Fprintf(stdout, "hue: bridge certificate changed (was %s, now %s)\n", cfg.Hue.CertSHA256[:12], fp[:12])
				if !*yes && !p.yesNo("Trust the new certificate?", false) {
					cancel()
					fmt.Fprintln(stderr, "push-it: keeping the old pin; hue will fail until you re-run install --hue")
					return 1
				}
			}
			cfg.Hue.CertSHA256 = fp
			fmt.Fprintf(stdout, "hue: pinned bridge certificate %s...\n", fp[:12])
			err = hue.New(cfg.Hue.Bridge, cfg.Hue.Key, cfg.Hue.Light, fp).Ping(ctx)
		}
		cancel()
		if err != nil {
			fmt.Fprintf(stderr, "push-it: hue check failed: %v (saved anyway; fix with `push-it install --hue`)\n", err)
		} else {
			fmt.Fprintln(stdout, "hue: bridge reachable")
		}
	}

	if err := os.MkdirAll(cfg.Sound.ClipsDir, 0o755); err != nil {
		fmt.Fprintf(stderr, "push-it: %v\n", err)
		return 1
	}
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(stderr, "push-it: %v\n", err)
		return 1
	}
	if err := installer.WireHook(installer.CLIGit{}, cfg.Dir(), exe, &cfg.InstallState); err != nil {
		fmt.Fprintf(stderr, "push-it: hook: %v\n", err)
		return 1
	}
	if wantGlow {
		if err := glow.Install(&cfg.InstallState); err != nil {
			fmt.Fprintf(stderr, "push-it: glow: %v\n", err)
		}
	}
	if err := cfg.Save(); err != nil {
		fmt.Fprintf(stderr, "push-it: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "installed. config: %s\n", cfg.Dir())
	if files, _ := clips.List(cfg.Sound.ClipsDir); wantSound && len(files) == 0 {
		fmt.Fprintf(stdout, "no clips yet  -  drop .mp3/.wav files into %s\nsee docs/make-your-own-clips.md to cut your own.\n", cfg.Sound.ClipsDir)
	}
	return 0
}

func cmdUninstall(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	fs.SetOutput(stderr)
	yes := fs.Bool("yes", false, "do not prompt; keep clips and config")
	if fs.Parse(args) != nil {
		return 2
	}
	cfg, ok := loadConfig(stderr)
	if !ok {
		return 1
	}
	if err := installer.UnwireHook(installer.CLIGit{}, &cfg.InstallState); err != nil {
		fmt.Fprintf(stderr, "push-it: hook: %v\n", err)
		return 1
	}
	if err := glow.Uninstall(&cfg.InstallState); err != nil {
		fmt.Fprintf(stderr, "push-it: glow: %v\n", err)
	}
	cfg.Sound.Enabled, cfg.Hue.Enabled, cfg.Glow.Enabled = false, false, false
	if err := cfg.Save(); err != nil {
		fmt.Fprintf(stderr, "push-it: %v\n", err)
		return 1
	}
	p := prompter{in: bufio.NewScanner(stdin), out: stdout}
	if !*yes && p.yesNo(fmt.Sprintf("Delete config and clips in %s too?", cfg.Dir()), false) {
		if err := os.RemoveAll(cfg.Dir()); err != nil {
			fmt.Fprintf(stderr, "push-it: %v\n", err)
			return 1
		}
	}
	fmt.Fprintln(stdout, "uninstalled. the push-it binary itself was left in place.")
	return 0
}

func cmdDoctor(_ []string, _ io.Reader, stdout, stderr io.Writer) int {
	cfg, ok := loadConfig(stderr)
	if !ok {
		return 1
	}
	path, _ := config.Path()
	fmt.Fprintf(stdout, "config:  %s\n", path)
	fmt.Fprintf(stdout, "sound:   enabled=%v volume=%.2f\n", cfg.Sound.Enabled, cfg.Sound.Volume)
	files, err := clips.List(cfg.Sound.ClipsDir)
	fmt.Fprintf(stdout, "clips:   %d in %s", len(files), cfg.Sound.ClipsDir)
	if err != nil {
		fmt.Fprintf(stdout, " (error: %v)", err)
	}
	fmt.Fprintln(stdout)
	fmt.Fprintf(stdout, "hue:     enabled=%v", cfg.Hue.Enabled)
	if cfg.Hue.Enabled {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err := hue.New(cfg.Hue.Bridge, cfg.Hue.Key, cfg.Hue.Light, cfg.Hue.CertSHA256).Ping(ctx)
		cancel()
		if err != nil {
			fmt.Fprintf(stdout, " bridge=%s UNREACHABLE (%v)", cfg.Hue.Bridge, err)
		} else {
			fmt.Fprintf(stdout, " bridge=%s ok", cfg.Hue.Bridge)
		}
	}
	fmt.Fprintln(stdout)
	fmt.Fprintf(stdout, "glow:    enabled=%v backend=%s\n", cfg.Glow.Enabled, glow.Backend)
	fmt.Fprintf(stdout, "hook:    hooksPath=%s setByUs=%v appendedTo=%s\n", cfg.InstallState.HooksPath, cfg.InstallState.HooksPathSetByUs, cfg.InstallState.PrePushAppendedTo)
	return 0
}
```

- [ ] **Step 5: Write clips.go (clips cut / clips review)**

```go
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/InfiniteRoomLabs/push-it/internal/clipper"
	"github.com/InfiniteRoomLabs/push-it/internal/player"
)

func init() {
	commands["clips"] = cmdClips
}

func cmdClips(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: push-it clips cut|review ...")
		return 2
	}
	switch args[0] {
	case "cut":
		return cmdClipsCut(args[1:], stdout, stderr)
	case "review":
		return cmdClipsReview(args[1:], stdin, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "push-it clips: unknown subcommand %q\n", args[0])
		return 2
	}
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' }) {
		out = append(out, p)
	}
	return out
}

func cmdClipsCut(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("clips cut", flag.ContinueOnError)
	fs.SetOutput(stderr)
	def := clipper.DefaultOptions()
	phrase := fs.String("phrase", strings.Join(def.Phrase, " "), "words a clip must start with")
	allow := fs.String("allow", strings.Join(def.Allow, ","), "extra words allowed inside a clip (comma-separated)")
	gap := fs.Float64("gap", def.Gap, "max silence inside a phrase (seconds)")
	max := fs.Float64("max", def.Max, "max clip length (seconds)")
	pad := fs.Float64("pad", 0.3, "padding added before and after each clip (seconds)")
	out := fs.String("o", "candidates", "output directory")
	if fs.Parse(args) != nil || fs.NArg() != 2 {
		fmt.Fprintln(stderr, "usage: push-it clips cut [flags] SOURCE.(mp3|wav) transcript.json")
		return 2
	}
	src, err := player.Decode(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "push-it: %v\n", err)
		return 1
	}
	data, err := os.ReadFile(fs.Arg(1))
	if err != nil {
		fmt.Fprintf(stderr, "push-it: %v\n", err)
		return 1
	}
	var words []clipper.Word
	if err := json.Unmarshal(data, &words); err != nil {
		fmt.Fprintf(stderr, "push-it: transcript: %v\n", err)
		return 1
	}
	phrases := clipper.Group(words, clipper.Options{Phrase: splitList(*phrase), Allow: splitList(*allow), Gap: *gap, Max: *max})
	written, err := clipper.Cut(src, phrases, *pad, *out)
	if err != nil {
		fmt.Fprintf(stderr, "push-it: %v\n", err)
		return 1
	}
	for _, p := range written {
		fmt.Fprintf(stdout, "  [%02d] %6.2fs..%6.2fs  %s\n", p.ID, p.Start, p.End, p.File)
	}
	fmt.Fprintf(stdout, "wrote %d candidates to %s\n", len(written), *out)
	return 0
}

func cmdClipsReview(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("clips review", flag.ContinueOnError)
	fs.SetOutput(stderr)
	keepTo := fs.String("keep-to", "", "directory keepers are moved into (default: your configured clips dir)")
	if fs.Parse(args) != nil || fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: push-it clips review [--keep-to DIR] candidates/")
		return 2
	}
	cfg, ok := loadConfig(stderr)
	if !ok {
		return 1
	}
	dest := *keepTo
	if dest == "" {
		dest = cfg.Sound.ClipsDir
	}
	play := func(c *player.Clip) error { return player.Play(contextBackground(), c, cfg.Sound.Volume) }
	kept, err := clipper.Review(stdin, stdout, play, fs.Arg(0), dest)
	if err != nil {
		fmt.Fprintf(stderr, "push-it: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "\nkept %d clip(s) in %s\n", kept, dest)
	return 0
}
```

Add at the bottom of `effects.go` (shared helper, avoids importing `context` into `clips.go` solely for this):

```go
func contextBackground() context.Context { return context.Background() }
```

- [ ] **Step 6: Run the whole suite and lint**

Run: `go test ./... && mise run lint && go build ./cmd/push-it && GOOS=windows GOARCH=arm64 go build ./cmd/push-it && GOOS=darwin GOARCH=arm64 go build ./cmd/push-it`
Expected: PASS, all three builds succeed with `CGO_ENABLED=0` (the default when cross-compiling).

- [ ] **Step 7: Commit**

CHANGELOG `### Added`: `- CLI: \`play\`, \`hue\`, \`glow\`, \`hook pre-push\`, \`clips cut\`, \`clips review\`, \`install\` (interactive or flagged), \`uninstall\`, \`doctor\`.`

```bash
git add cmd/push-it CHANGELOG.md
git commit -m "feat(cli): wire all subcommands

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 12: transcribe.py, docs, CI workflow

**Files:**
- Create: `tools/clipper/transcribe.py`, `docs/install.md`, `docs/make-your-own-clips.md`, `docs/hue.md`, `docs/glow.md`, `docs/migrating.md`, `.github/workflows/ci.yml`
- Modify: `README.md`, `CHANGELOG.md`

- [ ] **Step 1: Write tools/clipper/transcribe.py**

```python
# /// script
# requires-python = ">=3.11"
# dependencies = [
#     "faster-whisper>=1.1,<2",
# ]
# ///
"""Transcribe a track with word-level timestamps.

Usage:
    uv run tools/clipper/transcribe.py SOURCE.mp3 -o transcript.json [--model small.en]

The output is a JSON list of {"word", "start", "end"} objects  -  the input
format for `push-it clips cut`. faster-whisper's `av` dependency bundles
ffmpeg's libraries, so no system ffmpeg is required.
"""

import argparse
import json
import sys
from pathlib import Path

from faster_whisper import WhisperModel


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("source", type=Path, help="audio file (mp3, wav, ...)")
    ap.add_argument("-o", "--output", type=Path, default=Path("transcript.json"))
    ap.add_argument("--model", default="small.en", help="faster-whisper model name (default: small.en)")
    args = ap.parse_args()

    # Word-level timestamps are the point: segment-level is too coarse to cut
    # two-second clips. VAD is off because music has no real silence.
    model = WhisperModel(args.model, device="cpu", compute_type="int8")
    print(f"transcribing {args.source} with {args.model} ...", file=sys.stderr)
    segments, _info = model.transcribe(str(args.source), word_timestamps=True, vad_filter=False)

    words = []
    for seg in segments:
        for w in seg.words or []:
            words.append({"word": w.word.strip().lower(), "start": round(w.start, 3), "end": round(w.end, 3)})
            print(f"  [{w.start:6.2f}-{w.end:6.2f}] {w.word!r}", file=sys.stderr)

    args.output.write_text(json.dumps(words, indent=2) + "\n")
    print(f"wrote {len(words)} words to {args.output}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
```

Verify it parses: `uv run --no-sync tools/clipper/transcribe.py --help` (downloads deps the first time; skip if offline and just `python3 -m py_compile tools/clipper/transcribe.py`).

- [ ] **Step 2: Write docs/install.md**

```markdown
# Install

## Quick start

Until the first release ships (`install.sh` and prebuilt binaries arrive with v0.1.0), install from source:

```sh
go install github.com/InfiniteRoomLabs/push-it/cmd/push-it@latest
push-it install
```

`push-it install` with no flags asks three yes/no questions (sound, Hue, glow). Or be explicit:

```sh
push-it install --sound --glow        # pick components
push-it install --all                 # everything
push-it install --sound --yes         # non-interactive
```

Then drop `.mp3` or `.wav` files into the clips directory it prints (or cut your own  -  see [make-your-own-clips.md](make-your-own-clips.md)) and push something.

## What install changes

- Writes `config.json` (mode 0600) to your OS config dir: `~/.config/push-it/` on Linux, `~/Library/Application Support/push-it/` on macOS, `%APPDATA%\push-it\` on Windows. Override with `PUSH_IT_CONFIG_DIR`.
- Wires the git hook. If `git config --global core.hooksPath` is unset, it is pointed at `<config dir>/hooks/`. If you already have one, a marker-delimited block is appended to its `pre-push` (created if missing):

  ```sh
  # >>> push-it >>>
  '/path/to/push-it' hook pre-push "$@" || true
  # <<< push-it <<<
  ```

- Glow only: installs the GNOME Shell extension (Linux) or extracts the helper app (macOS). See [glow.md](glow.md).

Everything it did is recorded in `config.json` under `install_state`, and `push-it uninstall` reverses exactly that  -  your own hooks are restored byte-for-byte.

## Kill switches

| Variable | Effect |
|---|---|
| `NO_PUSH_IT=1` | skip everything |
| `NO_SOUND=1` | skip the clip |
| `NO_RAINBOW=1` | skip Hue |
| `NO_GLOW=1` | skip the screen glow |

Example: `NO_PUSH_IT=1 git push` for a quiet push.

## Checking on it

`push-it doctor` prints the config path, component status, clip count, Hue reachability, and which glow backend this build has. Errors during a push never block the push; they go to `<config dir>/push-it.log`.

## Uninstall

```sh
push-it uninstall          # asks whether to delete config + clips (default: keep)
push-it uninstall --yes    # keep them, no questions
```

The binary itself is left for you to remove (`rm $(which push-it)` or your package manager).
```

- [ ] **Step 3: Write docs/make-your-own-clips.md**

```markdown
# Make your own clips

push-it ships no audio. Clips are short excerpts you cut from a track you own a copy of, for your own use on your own machine. Distributing the resulting clips is your responsibility, not something this project does or encourages.

The pipeline is three steps: transcribe -> cut -> review.

## 0. Prerequisites

- `push-it` installed ([install.md](install.md))
- [`uv`](https://docs.astral.sh/uv/) for the one-time transcription step (the only Python in the project)
- Your source track as `.mp3` or `.wav`. Other formats: convert once with whatever you have (`ffmpeg -i in.m4a out.mp3`, Audacity, ...). push-it itself never needs ffmpeg.

## 1. Transcribe with word timestamps

```sh
uv run tools/clipper/transcribe.py source.mp3 -o transcript.json
```

This runs [faster-whisper](https://github.com/SYSTRAN/faster-whisper) (`small.en` on CPU; a few minutes for a 4-minute track) and writes a list of `{"word","start","end"}` entries. Use `--model medium.en` for better accuracy at the cost of time and RAM.

## 2. Cut candidates

```sh
push-it clips cut source.mp3 transcript.json -o candidates/
```

Defaults find phrases that start with "push it" and may continue with "push", "it", "real", "good"  -  up to 4 s long, with at most 0.5 s between words  -  and write each as a padded WAV (`001-push-it.wav`, `002-push-it-push-it-real-good.wav`, ...) plus `candidates.json`.

Tune for your track:

| Flag | Default | Meaning |
|---|---|---|
| `--phrase "push it"` | `push it` | words a clip must start with, in order |
| `--allow real,good` | `real,good` | extra words allowed inside a clip |
| `--gap 0.5` | `0.5` | max silence inside a phrase (s) |
| `--max 4.0` | `4.0` | max clip length (s) |
| `--pad 0.3` | `0.3` | padding before/after (s) |

Example for a different hook line: `push-it clips cut track.mp3 transcript.json --phrase "ship it" --allow "now,yeah"`.

## 3. Review and keep

```sh
push-it clips review candidates/
```

Each candidate plays; answer `k` (keep), `s` (skip), `r` (replay), or `q` (quit). Keepers are moved into your configured clips dir (`--keep-to DIR` to choose another). Whisper timestamps are good, not perfect  -  expect to skip a few clipped or mistimed ones.

Done: `push-it play` plays a random keeper; `git push` does the rest.
```

- [ ] **Step 4: Write docs/hue.md**

```markdown
# Philips Hue

On push, push-it saves one light's state, runs it through the hue wheel for about 3.5 s, and restores it.

## Setup

You need the bridge address, an API key, and a light ID.

1. Find the bridge: your router's client list, the Hue app (Settings -> My Hue System -> bridge -> i), or `https://discovery.meethue.com/`.
2. Create an API key: press the bridge's link button, then within 30 s:

   ```sh
   curl -sk -X POST https://<bridge>/api -d '{"devicetype":"push-it#laptop"}'
   ```

   The response contains `"username": "<key>"`.
3. List lights to find the ID: `curl -sk https://<bridge>/api/<key>/lights | jq 'map_values(.name)'`.
4. `push-it install --hue` and answer the prompts, or set `PUSH_IT_HUE_BRIDGE`, `PUSH_IT_HUE_KEY`, `PUSH_IT_HUE_LIGHT` in the environment  -  env values override the config file, so the key can live in your secret manager instead of on disk.

`push-it hue` fires the burst on demand; `push-it doctor` checks reachability.

## Notes

- Hue bridges present TLS certificates no public CA signs. Instead of skipping verification, push-it pins the bridge's certificate on first contact (`push-it install --hue` prints the fingerprint and stores it in `config.json`) and refuses to talk to anything else afterwards. If you replace the bridge, re-run `push-it install --hue`; it shows the old and new fingerprints and asks before trusting the new one.
- Every call has a 2 s timeout; if the bridge is unreachable the push is unaffected and the error goes to the log.
- Overlapping pushes do not stack bursts; a lock file skips the second one so the save/restore can't fight.
- `NO_RAINBOW=1 git push` skips Hue for one push.
```

- [ ] **Step 5: Write docs/glow.md**

```markdown
# Screen glow

An animated rainbow frame around the screen edge, shown for exactly as long as the clip plays (the clip is decoded before playback, so its length is known up front). It never captures input.

## Status

| Platform | Backend | Status |
|---|---|---|
| Linux / GNOME 45+ | Shell extension, triggered over D-Bus | planned (next release) |
| macOS | helper app (Cocoa / Core Animation), universal binary | planned |
| Windows | in-process layered window | planned |
| Linux / KDE, wlroots, X11 |  -  | not planned for v1; glow is a silent no-op |

This release ships the parameters and the hook points only; `push-it doctor` shows `glow: backend=none`.

## Parameters

Frame thickness 14 px, one full rainbow rotation every 2 s, opacity pulsing between 0.55 and 1.0 every 0.6 s. They live in `internal/glow/glow.go` and are mirrored in each renderer.

## Control

- `push-it glow --duration 3.5` shows it on demand.
- `NO_GLOW=1 git push` skips it once.
```

- [ ] **Step 6: Write docs/migrating.md**

```markdown
# Migrating from a hand-rolled hook

If you previously had your own `pre-push` that played a clip (and maybe a separate Hue script), here is how to move over without losing your clips.

1. **Install push-it**  -  `push-it install --sound` (add `--hue`/`--glow` as you like). If your old hook lives in a global `core.hooksPath` directory, push-it appends itself to it; it does not remove anything.
2. **Copy your clips** into the directory `push-it doctor` prints under `clips:`. Any `.mp3`/`.wav` works; no manifest is needed  -  the filename is the label.
3. **Move Hue credentials**  -  run `push-it install --hue` and paste the bridge, key, and light ID, or export `PUSH_IT_HUE_BRIDGE` / `PUSH_IT_HUE_KEY` / `PUSH_IT_HUE_LIGHT` from your secret manager.
4. **Retire the old scripts**  -  delete the old player/Hue lines from your `pre-push` (keep the `# >>> push-it >>>` block) so the clip doesn't play twice.
5. **Test**  -  `push-it play`, `push-it hue`, then push to a scratch branch.

Kill-switch names are compatible with common conventions: `NO_PUSH_IT=1` silences everything, `NO_RAINBOW=1` skips the light.
```

- [ ] **Step 7: Write .github/workflows/ci.yml**

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:

permissions:
  contents: read

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - name: gofmt
        run: test -z "$(gofmt -l .)" || (gofmt -l . && exit 1)
      - run: go vet ./...
      - uses: dominikh/staticcheck-action@v1
        with:
          version: latest
          install-go: false
      - name: cross-compile without cgo
        env:
          CGO_ENABLED: "0"
        run: |
          for t in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do
            GOOS=${t%/*} GOARCH=${t#*/} go build -o /dev/null ./cmd/push-it
          done

  test:
    strategy:
      fail-fast: false
      matrix:
        os: [ubuntu-latest, macos-latest, windows-latest]
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - run: go test ./...
```

- [ ] **Step 8: Update README usage section**

Replace the `## Install` section of `README.md` with:

```markdown
## Install

```sh
go install github.com/InfiniteRoomLabs/push-it/cmd/push-it@latest
push-it install            # asks: sound? hue? glow?
```

Prebuilt binaries for Linux, macOS, and Windows (amd64 + arm64) and a one-line `install.sh` come with the first tagged release. Details, kill switches, and uninstall: [docs/install.md](docs/install.md).

## Usage

```sh
git push                   # that's it
push-it play               # one random clip, on demand
push-it doctor             # what's configured, what's reachable
NO_PUSH_IT=1 git push      # quiet push
```
```

- [ ] **Step 9: Verify and commit**

Run: `go test ./... && mise run lint`
Then scan for operator-specific strings before anything becomes public: `grep -rniE '100\.[0-9]+\.|<user>|\.lab\.|internal\.|/home/' --exclude-dir=.git . ; echo "exit $?"`  -  expected: no matches (exit 1).

CHANGELOG `### Added`: `- Docs: install, make-your-own-clips, hue, glow, migrating; \`tools/clipper/transcribe.py\`; GitHub Actions CI (lint, no-cgo cross-compile, tests on Linux/macOS/Windows).`

```bash
git add tools docs/install.md docs/make-your-own-clips.md docs/hue.md docs/glow.md docs/migrating.md .github README.md CHANGELOG.md
git commit -m "docs: user docs, transcribe script, CI workflow

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 13: Publish the repo and dogfood on this machine

**Files:** none in-repo beyond `CHANGELOG.md`; this task is remotes, a redaction scan, a first push, and a local install.

- [ ] **Step 1: Redaction scan of the working tree and history**

```bash
cd ~/projects/infinite-room-labs/push-it
terms=$(python3 <agent-ops>/scripts/resolve-redaction-terms.py)   # the agent-ops repo's redaction helper
git log -p --all | grep -niF -f <(printf '%s\n' "$terms") | head   # expect no output
```

If anything matches, fix it and rewrite history now (`git filter-repo`)  -  this is still a private local repo.

- [ ] **Step 2: Create remotes (private Gitea origin, public GitHub)**

```bash
tea repos create --name push-it --owner InfiniteRoomLabs --private --description "Celebrate git pushes with sound, Hue light, and screen glow"   # check `tea repos create --help` for the exact owner flag on this tea version
git remote add origin git@<private-gitea-host>:InfiniteRoomLabs/push-it.git
gh repo create InfiniteRoomLabs/push-it --public --description "Celebrate git pushes with sound, Hue light, and screen glow" --disable-wiki
git remote add github git@github.com:InfiniteRoomLabs/push-it.git
git remote -v
```

(The Gitea hostname above is the operator's private remote and appears only in this local command, never in committed files.)

- [ ] **Step 3: Push and confirm CI is green**

```bash
git push -u origin main
git push github main
gh run watch --repo InfiniteRoomLabs/push-it --exit-status
```

Expected: lint + 3 test jobs pass. If macOS or Windows fails, fix forward in a new commit; do not skip the matrix.

- [ ] **Step 4: Install locally and migrate clips**

```bash
mise run build && install -m 755 bin/push-it ~/.local/bin/push-it
push-it install --sound --hue      # answer the Hue prompts from your existing credentials file
mkdir -p "$(push-it doctor | awk '/^clips:/{print $4}')"
# copy the clips your old keepers.json points at:
grep -o 'clip_[0-9]*\.mp3' ~/.local/share/push-it-hook/keepers.json | sort -u | while read f; do cp ~/.local/share/push-it-hook/clips/"$f" "$(push-it doctor | awk '/^clips:/{print $4}')/"; done
push-it doctor
push-it play
push-it hue
```

Expected: `doctor` shows 16 clips and `hue: ... ok`; `play` plays one clip; the light runs the wheel and restores.

- [ ] **Step 5: Retire the old hook scripts and test a real push**

The old global `pre-push` lives in the existing `core.hooksPath` dir, so `push-it install` appended its block to it. Edit that file: delete the old clip-player and hue-rainbow logic, keep the `# >>> push-it >>>` block. Then:

```bash
cat "$(git config --global core.hooksPath)/pre-push"
git push github main    # or a no-op push; expect the clip + light, and no duplicate playback
tail -5 "$(push-it doctor | awk '/^config:/{print $2}' | xargs dirname)/push-it.log"
```

- [ ] **Step 6: Record it**

CHANGELOG `### Changed`: `- Repository published (GitHub public + private mirror); CI running on every push.`

```bash
git add CHANGELOG.md
git commit -m "docs(changelog): note repository publication

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
git push origin main && git push github main
```

- [ ] **Step 7: Seed the Plane project**

Create a Plane project `push-it` and add one work item per remaining plan: "Plan 2  -  glow backends (GNOME, macOS, Windows)" and "Plan 3  -  release pipeline (goreleaser, install.sh, v0.1.0)", plus one item per open risk from the spec (ALSA availability, GNOME logout requirement, Windows unverified).

---

## Self-review

**Spec coverage (Plan 1 scope):** config yes (T2), clips yes (T3), player incl. WAV-by-hand + MP3 + no ffmpeg yes (T4), hue incl. save/restore, timeouts, lock yes (T5, T6, T11), glow interface + params yes (T7; renderers -> Plan 2), hook detached + kill switches + duration sync yes (T8), clipper cut/review in binary + transcribe.py yes (T9, T12), installer reversible wiring + idempotency yes (T10), CLI table incl. doctor/uninstall/interactive install yes (T11), docs set yes (T12), CI on 3 OSes + no-cgo cross-compile incl. arm64 yes (T12), MIT + mise + remotes + redaction + Plane yes (T1, T13). Deferred by design: goreleaser/release.yml/install.sh (Plan 3), glow renderers and glow install/uninstall bodies (Plan 2).

**Type consistency:** `config.InstallState` fields used by `installer` (`HooksPathSetByUs`, `HooksPath`, `PrePushAppendedTo`, `PrePushCreatedByUs`) are defined in T2; `hook.Deps` field names match `deps()` in T11; `clipper.Phrase.File` set by `Cut` is read by `Review`; `glow.Run/Install/Uninstall/Backend/Available/DefaultDuration` are referenced identically in T8 and T11; `player.Clip.Duration/Slice`, `EncodeWAV`, `Decode` match their uses in T9.

**Placeholders:** none  -  the only "check `--help`" note is for the `tea` owner flag, which varies by version and is an operator-time command, not code.
