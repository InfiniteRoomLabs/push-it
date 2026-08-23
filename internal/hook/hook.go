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
// logf may be called from multiple goroutines (hue and glow run concurrently
// with the main flow); Run serialises those calls so callers need not.
func Run(ctx context.Context, sound, hue, glowOn bool, d Deps, logf func(string, ...any)) {
	var mu sync.Mutex
	safeLogf := func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		logf(format, args...)
	}
	var wg sync.WaitGroup
	if hue {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := d.Hue(ctx); err != nil {
				safeLogf("%v", err)
			}
		}()
	}
	startGlow := func(dur time.Duration) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := d.Glow(ctx, dur); err != nil {
				safeLogf("%v", err)
			}
		}()
	}
	switch {
	case sound:
		path, err := d.Pick()
		if err != nil {
			safeLogf("clips: %v", err)
			if glowOn {
				startGlow(glow.DefaultDuration)
			}
			break
		}
		clip, err := d.Decode(path)
		if err != nil {
			safeLogf("decode %s: %v", path, err)
			if glowOn {
				startGlow(glow.DefaultDuration)
			}
			break
		}
		if glowOn {
			startGlow(clip.Duration())
		}
		safeLogf("playing %s (%s)", path, clip.Duration().Round(10*time.Millisecond))
		if err := d.Play(ctx, clip); err != nil {
			safeLogf("play: %v", err)
		}
	case glowOn:
		startGlow(glow.DefaultDuration)
	}
	wg.Wait()
}

// startCommand is a seam over (*exec.Cmd).Start so tests can assert PrePush
// spawns the right command without paying for a real fork/exec.
var startCommand = func(cmd *exec.Cmd) error { return cmd.Start() }

// PrePush is what git invokes. It must return in milliseconds and must not
// block a push: it drains stdin, honours NO_PUSH_IT, and re-execs exe as a
// detached "hook --run". It is best-effort - if the log file can't be
// opened, the child's stdout/stderr fall back to the null device rather than
// failing the push, and the process handle is released without checking its
// error. Only a failure to start the child process is returned.
func PrePush(stdin io.Reader, getenv func(string) string, exe, logPath string) error {
	_, _ = io.Copy(io.Discard, stdin)
	if getenv("NO_PUSH_IT") == "1" {
		return nil
	}
	cmd := exec.Command(exe, "hook", "--run")
	if logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
		defer logFile.Close()
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}
	// else: leave Stdout/Stderr nil - exec redirects the child to the null
	// device rather than failing the push over a log file we can't open.
	detach(cmd)
	if err := startCommand(cmd); err != nil {
		return err
	}
	if cmd.Process != nil {
		_ = cmd.Process.Release()
	}
	return nil
}
