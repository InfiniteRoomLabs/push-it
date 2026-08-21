package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/InfiniteRoomLabs/push-it/internal/glow"
)

func TestHookPrePushHonoursKillSwitch(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("PUSH_IT_CONFIG_DIR", cfgDir)
	t.Setenv("NO_PUSH_IT", "1")
	var out, errOut bytes.Buffer
	args := []string{"hook", "pre-push", "origin", "https://example.invalid/r.git"}
	stdin := strings.NewReader("refs/heads/main abc refs/heads/main def\n")
	if code := run(args, stdin, &out, &errOut); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errOut.String())
	}
	if _, err := os.Stat(filepath.Join(cfgDir, "push-it.log")); err == nil {
		t.Fatal("log file should not exist when NO_PUSH_IT is set")
	}
}

// TestHookPrePushSpawnsWithGitsRealArgs exercises the exact invocation the
// installed hook uses: `push-it hook pre-push "$@"`, where git supplies the
// remote name and URL as extra positional args. cmdHook must still recognise
// "pre-push" and spawn the detached child.
func TestHookPrePushSpawnsWithGitsRealArgs(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("PUSH_IT_CONFIG_DIR", cfgDir)
	t.Setenv("PUSH_IT_TEST_CHILD", "1")
	var out, errOut bytes.Buffer
	args := []string{"hook", "pre-push", "origin", "https://example.invalid/r.git"}
	stdin := strings.NewReader("refs/heads/main abc refs/heads/main def\n")
	if code := run(args, stdin, &out, &errOut); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errOut.String())
	}
	if _, err := os.Stat(filepath.Join(cfgDir, "push-it.log")); err != nil {
		t.Fatalf("log file should exist after spawning the detached child: %v", err)
	}
}

// TestPlayMissingClipsDirFails covers `play` with no clips directory at all
// (a fresh config, nothing installed yet): clips.List treats a missing
// directory as empty, so Pick must fail with ErrNoClips and cmdPlay must
// surface that message and exit 1, never panic on an empty slice.
func TestPlayMissingClipsDirFails(t *testing.T) {
	t.Setenv("PUSH_IT_CONFIG_DIR", t.TempDir())
	var out, errOut bytes.Buffer
	if code := run([]string{"play"}, strings.NewReader(""), &out, &errOut); code != 1 {
		t.Fatalf("code=%d, want 1; stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
	if !strings.Contains(errOut.String(), "no clips found") {
		t.Fatalf("stderr = %q, want it to mention \"no clips found\"", errOut.String())
	}
}

// TestPlayFileNotFoundFails covers `play --file` naming a clip that does not
// exist: decode must fail and cmdPlay must exit 1 rather than try to play it.
func TestPlayFileNotFoundFails(t *testing.T) {
	t.Setenv("PUSH_IT_CONFIG_DIR", t.TempDir())
	var out, errOut bytes.Buffer
	missing := filepath.Join(t.TempDir(), "nonexistent.wav")
	if code := run([]string{"play", "--file", missing}, strings.NewReader(""), &out, &errOut); code != 1 {
		t.Fatalf("code=%d, want 1; stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
}

// TestHueUnconfiguredFails covers `hue` run against a fresh config with no
// bridge/key set: it must refuse before ever dialing out, and say so.
func TestHueUnconfiguredFails(t *testing.T) {
	t.Setenv("PUSH_IT_CONFIG_DIR", t.TempDir())
	// Blank out any PUSH_IT_HUE_* the developer's own shell may have set -
	// config.Load() only applies an override when the env var is non-empty,
	// so this keeps the test hermetic without needing os.Unsetenv.
	t.Setenv("PUSH_IT_HUE_BRIDGE", "")
	t.Setenv("PUSH_IT_HUE_KEY", "")
	var out, errOut bytes.Buffer
	if code := run([]string{"hue"}, strings.NewReader(""), &out, &errOut); code != 1 {
		t.Fatalf("code=%d, want 1; stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
	if !strings.Contains(errOut.String(), "not configured") {
		t.Fatalf("stderr = %q, want it to mention \"not configured\"", errOut.String())
	}
}

// TestGlowDurationIsPassedThrough stubs glow.Run (a package var seam, same
// pattern as TestInstallPrintsGlowNote's glow.Install stub) to record the
// requested duration without touching any real backend, then asserts
// cmdGlow both succeeds and forwards --duration verbatim.
func TestGlowDurationIsPassedThrough(t *testing.T) {
	orig := glow.Run
	var got time.Duration
	glow.Run = func(_ context.Context, d time.Duration) error {
		got = d
		return nil
	}
	t.Cleanup(func() { glow.Run = orig })
	var out, errOut bytes.Buffer
	if code := run([]string{"glow", "--duration", "10ms"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errOut.String())
	}
	if got != 10*time.Millisecond {
		t.Fatalf("recorded duration = %v, want 10ms", got)
	}
}

// TestGlowDurationInvalidFails covers a malformed --duration value: flag
// parsing must reject it before glow.Run is ever called.
func TestGlowDurationInvalidFails(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"glow", "--duration", "banana"}, strings.NewReader(""), &out, &errOut); code != 2 {
		t.Fatalf("code=%d, want 2; stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
}
