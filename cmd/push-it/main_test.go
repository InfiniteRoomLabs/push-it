package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/InfiniteRoomLabs/push-it/internal/config"
	"github.com/InfiniteRoomLabs/push-it/internal/glow"
)

// isolateInstall points every path install/uninstall can touch at temp dirs
// and stubs the glow backend so no test can reach the developer's machine.
func isolateInstall(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("PUSH_IT_CONFIG_DIR", filepath.Join(tmp, "cfg"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(tmp, "gitconfig"))
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))
	t.Setenv("XDG_CURRENT_DESKTOP", "")
	origI, origU := glow.Install, glow.Uninstall
	glow.Install = func(*config.InstallState) (string, error) { return "", nil }
	glow.Uninstall = func(*config.InstallState) error { return nil }
	t.Cleanup(func() { glow.Install, glow.Uninstall = origI, origU })
	return tmp
}

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
