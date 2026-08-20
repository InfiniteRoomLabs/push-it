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
