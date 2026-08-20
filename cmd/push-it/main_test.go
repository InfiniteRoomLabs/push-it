package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
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
