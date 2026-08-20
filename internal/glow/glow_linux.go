//go:build linux

package glow

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/InfiniteRoomLabs/push-it/internal/config"
	"github.com/InfiniteRoomLabs/push-it/internal/glow/gnome"
)

// Seams for tests.
var (
	runCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return exec.CommandContext(ctx, name, args...).CombinedOutput()
	}
	lookPath = exec.LookPath
)

func init() {
	Backend = "gnome"
	Run = runGnome
	Install = installGnome
	Uninstall = uninstallGnome
}

func extensionDir() (string, error) {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "gnome-shell", "extensions", gnome.UUID), nil
}

// runGnome asks the Shell extension to start the glow, then blocks for d so
// the caller's WaitGroup keeps the glow alive for the clip.
func runGnome(ctx context.Context, d time.Duration) error {
	if _, err := lookPath("gdbus"); err != nil {
		return errors.New("glow: gdbus not found (is this a GNOME session?)")
	}
	out, err := runCommand(ctx, "gdbus", "call", "--session",
		"--dest", gnome.BusName, "--object-path", gnome.ObjectPath,
		"--method", gnome.Interface+".Start", fmt.Sprintf("%.3f", d.Seconds()))
	if err != nil {
		return fmt.Errorf("glow: gnome extension did not answer (%s); is it installed and enabled? run `push-it install --glow` and log out/in: %w", strings.TrimSpace(string(out)), err)
	}
	select {
	case <-ctx.Done():
		_, _ = runCommand(context.Background(), "gdbus", "call", "--session",
			"--dest", gnome.BusName, "--object-path", gnome.ObjectPath,
			"--method", gnome.Interface+".Stop")
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

func installGnome(st *config.InstallState) (string, error) {
	if st == nil {
		return "", errors.New("glow: nil install state")
	}
	dir, err := extensionDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	err = fs.WalkDir(gnome.FS, "ext", func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return walkErr
		}
		data, err := gnome.FS.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel("ext", p)
		return os.WriteFile(filepath.Join(dir, rel), data, 0o644)
	})
	if err != nil {
		return "", fmt.Errorf("glow: extract extension: %w", err)
	}
	st.GnomeExtensionInstalled = true
	if _, err := lookPath("gnome-extensions"); err != nil {
		return fmt.Sprintf("extension extracted to %s; run `gnome-extensions enable %s`, then log out and back in (Wayland cannot hot-load extensions)", dir, gnome.UUID), nil
	}
	if out, err := runCommand(context.Background(), "gnome-extensions", "enable", gnome.UUID); err != nil {
		return fmt.Sprintf("extension extracted to %s but `gnome-extensions enable` failed (%s); log out and back in, then enable it in the Extensions app", dir, strings.TrimSpace(string(out))), nil
	}
	return "GNOME extension installed and enabled; log out and back in once so the Shell loads it (Wayland cannot hot-load extensions)", nil
}

func uninstallGnome(st *config.InstallState) error {
	if st == nil {
		return errors.New("glow: nil install state")
	}
	if !st.GnomeExtensionInstalled {
		return nil
	}
	if _, err := lookPath("gnome-extensions"); err == nil {
		_, _ = runCommand(context.Background(), "gnome-extensions", "disable", gnome.UUID)
	}
	dir, err := extensionDir()
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	st.GnomeExtensionInstalled = false
	return nil
}
