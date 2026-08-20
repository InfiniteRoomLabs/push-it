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
	"github.com/InfiniteRoomLabs/push-it/internal/glow"
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

func TestRunDecodeFailureStillGlows(t *testing.T) {
	var glowFor atomic.Int64
	var playCalled atomic.Bool
	d := Deps{
		Pick:   func() (string, error) { return "clip.wav", nil },
		Decode: func(string) (*player.Clip, error) { return nil, errors.New("bad file") },
		Play:   func(context.Context, *player.Clip) error { playCalled.Store(true); return nil },
		Glow:   func(_ context.Context, dur time.Duration) error { glowFor.Store(int64(dur)); return nil },
	}
	Run(context.Background(), true, false, true, d, func(string, ...any) {})
	if playCalled.Load() {
		t.Fatal("play should not run after a decode error")
	}
	if got := time.Duration(glowFor.Load()); got != glow.DefaultDuration {
		t.Fatalf("glow duration after decode error = %v, want DefaultDuration %v", got, glow.DefaultDuration)
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

func TestPrePushBestEffortWhenLogPathUnwritable(t *testing.T) {
	t.Setenv("PUSH_IT_TEST_CHILD", "1")
	logPath := filepath.Join(t.TempDir(), "does-not-exist", "push-it.log")
	if err := PrePush(strings.NewReader(""), env(nil), os.Args[0], logPath); err != nil {
		t.Fatalf("PrePush should be best-effort about an unwritable log path, got: %v", err)
	}
}
