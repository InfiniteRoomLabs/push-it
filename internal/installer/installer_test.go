package installer

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

func TestBlockEscapesSingleQuotes(t *testing.T) {
	b := Block("/x/it's/push-it")
	if !strings.Contains(b, `'/x/it'\''s/push-it' hook pre-push`) {
		t.Fatalf("block not escaped:\n%s", b)
	}
}

func TestWireRejectsRelativeHooksPath(t *testing.T) {
	cfgDir, g := setup(t)
	t.Chdir(t.TempDir())
	_ = g.Set("core.hooksPath", ".githooks")
	var st config.InstallState
	err := WireHook(g, cfgDir, "/opt/push-it", &st)
	if err == nil || !strings.Contains(err.Error(), "relative") {
		t.Fatalf("err = %v, want a \"relative\" error", err)
	}
	if _, statErr := os.Stat(filepath.Join(cfgDir, "hooks")); statErr == nil {
		t.Fatal("no files should have been created")
	}
	if _, statErr := os.Stat(".githooks"); statErr == nil {
		t.Fatal("no files should have been created")
	}
}

func TestWireRewiresAfterUserUnsetsHooksPath(t *testing.T) {
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

	// user changes their mind and unsets core.hooksPath out from under us
	if err := g.Unset("core.hooksPath"); err != nil {
		t.Fatal(err)
	}

	if err := WireHook(g, cfgDir, "/opt/push-it", &st); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(hooks, "pre-push"))
	if strings.Contains(string(b), MarkerStart) {
		t.Fatalf("user's hook still carries our block:\n%s", b)
	}
	want := filepath.Join(cfgDir, "hooks")
	if got, _ := g.Get("core.hooksPath"); got != want {
		t.Fatalf("core.hooksPath = %q, want %q", got, want)
	}

	if err := UnwireHook(g, &st); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(filepath.Join(hooks, "pre-push"))
	if string(b) != orig {
		t.Fatalf("user's original hook not preserved byte-for-byte:\n%s", b)
	}
	if got, _ := g.Get("core.hooksPath"); got != "" {
		t.Fatalf("core.hooksPath still %q after final unwire", got)
	}
}

func TestUnwireLeavesUserHooksPathAlone(t *testing.T) {
	cfgDir, g := setup(t)
	var st config.InstallState
	if err := WireHook(g, cfgDir, "/opt/push-it", &st); err != nil {
		t.Fatal(err)
	}
	ourHooks := filepath.Join(cfgDir, "hooks")

	// user takes over core.hooksPath themselves after our install
	userHooks := filepath.Join(t.TempDir(), "userhooks")
	if err := g.Set("core.hooksPath", userHooks); err != nil {
		t.Fatal(err)
	}

	if err := UnwireHook(g, &st); err != nil {
		t.Fatal(err)
	}
	if got, _ := g.Get("core.hooksPath"); got != userHooks {
		t.Fatalf("core.hooksPath = %q, want untouched user value %q", got, userHooks)
	}
	if _, err := os.Stat(filepath.Join(ourHooks, "pre-push")); err == nil {
		t.Fatal("our pre-push should be removed")
	}
}

func TestWireExpandsTildeHooksPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("~ expansion assumes HOME is the tilde target, not true on Windows")
	}
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
