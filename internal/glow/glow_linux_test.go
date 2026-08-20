//go:build linux

package glow

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/InfiniteRoomLabs/push-it/internal/config"
	"github.com/InfiniteRoomLabs/push-it/internal/glow/gnome"
)

type call struct {
	name string
	args []string
}

func stubExec(t *testing.T, out []byte, err error) *[]call {
	t.Helper()
	var calls []call
	origRun, origLook := runCommand, lookPath
	runCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, call{name, args})
		return out, err
	}
	lookPath = func(string) (string, error) { return "/usr/bin/stub", nil }
	t.Cleanup(func() { runCommand, lookPath = origRun, origLook })
	return &calls
}

func TestBackendIsGnome(t *testing.T) {
	if Backend != "gnome" || !Available() {
		t.Fatalf("Backend = %q", Backend)
	}
}

func TestRunCallsGdbusAndBlocksForDuration(t *testing.T) {
	t.Setenv("XDG_CURRENT_DESKTOP", "ubuntu:GNOME")
	calls := stubExec(t, []byte("()\n"), nil)
	start := time.Now()
	if err := Run(context.Background(), 80*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if el := time.Since(start); el < 70*time.Millisecond {
		t.Fatalf("Run returned after %v, should block for the duration", el)
	}
	c := (*calls)[0]
	want := []string{"call", "--session", "--dest", gnome.BusName, "--object-path", gnome.ObjectPath, "--method", gnome.Interface + ".Start", "0.080"}
	if c.name != "gdbus" || strings.Join(c.args, " ") != strings.Join(want, " ") {
		t.Fatalf("gdbus args = %v", c.args)
	}
}

func TestRunReturnsErrorWhenGdbusFails(t *testing.T) {
	t.Setenv("XDG_CURRENT_DESKTOP", "ubuntu:GNOME")
	stubExec(t, []byte("Error: GDBus.Error:org.freedesktop.DBus.Error.UnknownMethod"), errors.New("exit status 1"))
	err := Run(context.Background(), 10*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "extension") {
		t.Fatalf("err = %v, want a hint about the extension", err)
	}
}

func TestRunStopsOnContextCancel(t *testing.T) {
	t.Setenv("XDG_CURRENT_DESKTOP", "ubuntu:GNOME")
	stubExec(t, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(20 * time.Millisecond); cancel() }()
	start := time.Now()
	_ = Run(ctx, 5*time.Second)
	if time.Since(start) > time.Second {
		t.Fatal("Run did not stop on cancel")
	}
}

func TestInstallExtractsEnablesAndNotes(t *testing.T) {
	t.Setenv("XDG_CURRENT_DESKTOP", "ubuntu:GNOME")
	calls := stubExec(t, nil, nil)
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	var st config.InstallState
	note, err := Install(&st)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(data, "gnome-shell", "extensions", gnome.UUID)
	for _, f := range []string{"metadata.json", "extension.js", "glowmath.js"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatalf("missing %s: %v", f, err)
		}
	}
	if !st.GnomeExtensionInstalled {
		t.Fatal("state not recorded")
	}
	if !strings.Contains(strings.ToLower(note), "log out") {
		t.Fatalf("note = %q", note)
	}
	last := (*calls)[len(*calls)-1]
	if last.name != "gnome-extensions" || last.args[0] != "enable" || last.args[1] != gnome.UUID {
		t.Fatalf("enable call = %+v", last)
	}
}

func TestInstallIsIdempotentAndUninstallReverses(t *testing.T) {
	t.Setenv("XDG_CURRENT_DESKTOP", "ubuntu:GNOME")
	calls := stubExec(t, nil, nil)
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	var st config.InstallState
	if _, err := Install(&st); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(&st); err != nil {
		t.Fatal(err)
	}
	if err := Uninstall(&st); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(data, "gnome-shell", "extensions", gnome.UUID)); !os.IsNotExist(err) {
		t.Fatal("extension dir should be removed")
	}
	if st.GnomeExtensionInstalled {
		t.Fatal("state not cleared")
	}
	last := (*calls)[len(*calls)-1]
	if last.name != "gnome-extensions" || last.args[0] != "disable" {
		t.Fatalf("disable call = %+v", last)
	}
}

func TestInstallWithoutGnomeExtensionsCLIStillExtracts(t *testing.T) {
	t.Setenv("XDG_CURRENT_DESKTOP", "ubuntu:GNOME")
	stubExec(t, nil, nil)
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	var st config.InstallState
	note, err := Install(&st)
	if err != nil || !strings.Contains(note, "gnome-extensions enable") {
		t.Fatalf("note=%q err=%v", note, err)
	}
}

func TestInstallRefusesNonGnomeSession(t *testing.T) {
	t.Setenv("XDG_CURRENT_DESKTOP", "KDE")
	calls := stubExec(t, nil, nil)
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	var st config.InstallState
	_, err := Install(&st)
	if err == nil || !strings.Contains(err.Error(), "not a GNOME session") {
		t.Fatalf("err = %v, want a not-a-GNOME-session error", err)
	}
	if _, statErr := os.Stat(filepath.Join(data, "gnome-shell", "extensions", gnome.UUID)); !os.IsNotExist(statErr) {
		t.Fatal("extension dir should not have been created")
	}
	if st.GnomeExtensionInstalled {
		t.Fatal("state should not be recorded")
	}
	if len(*calls) != 0 {
		t.Fatalf("runCommand should not have been called, got %v", *calls)
	}
}

func TestRunRefusesNonGnomeSession(t *testing.T) {
	t.Setenv("XDG_CURRENT_DESKTOP", "KDE")
	calls := stubExec(t, nil, nil)
	err := Run(context.Background(), 10*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "not a GNOME session") {
		t.Fatalf("err = %v, want a not-a-GNOME-session error", err)
	}
	if len(*calls) != 0 {
		t.Fatalf("runCommand should not have been called, got %v", *calls)
	}
}

func TestNilStateIsAnError(t *testing.T) {
	if _, err := Install(nil); err == nil {
		t.Fatal("Install(nil) must error")
	}
	if err := Uninstall(nil); err == nil {
		t.Fatal("Uninstall(nil) must error")
	}
}
