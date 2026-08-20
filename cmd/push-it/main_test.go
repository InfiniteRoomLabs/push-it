package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestMain lets this test binary double as the detached child process that
// hook.PrePush spawns (os.Executable() is the test binary under `go test`).
// When PUSH_IT_TEST_CHILD is set, exit immediately instead of running the
// suite, so the "child" does nothing but let PrePush's already-opened log
// file prove the spawn happened.
func TestMain(m *testing.M) {
	if os.Getenv("PUSH_IT_TEST_CHILD") == "1" {
		os.Exit(0)
	}
	os.Exit(m.Run())
}

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

// TestInstallExplicitFlagsAreAdditive verifies that a later `install --hue`
// does not silently disable sound (or glow) that an earlier `install --all`
// turned on: single-component flags only touch the components they name.
func TestInstallExplicitFlagsAreAdditive(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	tmp := t.TempDir()
	t.Setenv("PUSH_IT_CONFIG_DIR", filepath.Join(tmp, "cfg"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(tmp, "gitconfig"))
	t.Setenv("HOME", tmp)
	var out, errOut bytes.Buffer
	if code := run([]string{"install", "--all", "--yes"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("install --all code=%d stderr=%s", code, errOut.String())
	}

	t.Setenv("PUSH_IT_HUE_BRIDGE", "127.0.0.1")
	t.Setenv("PUSH_IT_HUE_KEY", "k")
	out.Reset()
	errOut.Reset()
	if code := run([]string{"install", "--hue", "--yes"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("install --hue code=%d stderr=%s", code, errOut.String())
	}

	b, err := os.ReadFile(filepath.Join(tmp, "cfg", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		Sound struct {
			Enabled bool `json:"enabled"`
		} `json:"sound"`
	}
	if err := json.Unmarshal(b, &saved); err != nil {
		t.Fatal(err)
	}
	if !saved.Sound.Enabled {
		t.Fatalf("sound should still be enabled after `install --hue`:\n%s", b)
	}
}
