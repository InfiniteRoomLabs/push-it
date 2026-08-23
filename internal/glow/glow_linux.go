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
	lookPath       = exec.LookPath
	isGnomeSession = func() bool {
		return strings.Contains(strings.ToLower(os.Getenv("XDG_CURRENT_DESKTOP")), "gnome")
	}
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
	if !isGnomeSession() {
		return errors.New("glow: not a GNOME session; screen glow on Linux requires GNOME Shell")
	}
	if _, err := lookPath("gdbus"); err != nil {
		return errors.New("glow: gdbus not found (is this a GNOME session?)")
	}
	out, err := runCommand(ctx, "gdbus", "call", "--session",
		"--dest", gnome.BusName, "--object-path", gnome.ObjectPath,
		"--method", gnome.Interface+".Start", fmt.Sprintf("%.3f", d.Seconds()))
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("glow: %w", ctx.Err())
		}
		return fmt.Errorf("glow: gnome extension did not answer (%s); is it installed and enabled? run `push-it install --glow` and log out/in: %w", strings.TrimSpace(string(out)), err)
	}
	select {
	case <-ctx.Done():
		_, _ = runCommand(context.Background(), "gdbus", "call", "--session",
			"--dest", gnome.BusName, "--object-path", gnome.ObjectPath,
			"--method", gnome.Interface+".Stop")
		return fmt.Errorf("glow: %w", ctx.Err())
	case <-time.After(d):
		return nil
	}
}

// parseGVariantStrv parses gsettings' string-array text form, e.g.
// "['a', 'b']", "[]", or "@as []", into its elements. Extension UUIDs never
// contain quotes or commas, so a split parser is sufficient.
func parseGVariantStrv(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimSpace(strings.TrimPrefix(s, "@as"))
	s = strings.TrimSuffix(strings.TrimPrefix(s, "["), "]")
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.Trim(strings.TrimSpace(part), "'\""); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func formatGVariantStrv(items []string) string {
	quoted := make([]string, len(items))
	for i, it := range items {
		quoted[i] = "'" + it + "'"
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// preEnableViaGsettings adds (or, for enable=false, removes) the extension
// UUID in org.gnome.shell enabled-extensions so the Shell picks it up on the
// next login without a manual enable. Best-effort: returns the error for the
// caller's message but changes nothing else on failure.
func preEnableViaGsettings(ctx context.Context, enable bool) error {
	if _, err := lookPath("gsettings"); err != nil {
		return err
	}
	out, err := runCommand(ctx, "gsettings", "get", "org.gnome.shell", "enabled-extensions")
	if err != nil {
		return fmt.Errorf("gsettings get: %s: %w", strings.TrimSpace(string(out)), err)
	}
	// runCommand uses CombinedOutput, so a healthy value can arrive preceded
	// by stderr noise (e.g. "dconf-WARNING ...\n['a@b']"). Take the last
	// non-empty line - the value always comes last - and refuse to parse
	// anything that doesn't look like a strv, rather than writing mangled
	// junk back over the user's enabled-extensions.
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	raw := strings.TrimSpace(lines[len(lines)-1])
	if !strings.HasPrefix(raw, "[") && !strings.HasPrefix(raw, "@as") {
		return fmt.Errorf("gsettings get: unexpected output %q", raw)
	}
	list := parseGVariantStrv(raw)
	has := false
	for _, e := range list {
		if e == gnome.UUID {
			has = true
			break
		}
	}
	switch {
	case enable && has, !enable && !has:
		return nil // already in the desired state
	case enable:
		list = append(list, gnome.UUID)
	default:
		kept := list[:0]
		for _, e := range list {
			if e != gnome.UUID {
				kept = append(kept, e)
			}
		}
		list = kept
	}
	if out, err := runCommand(ctx, "gsettings", "set", "org.gnome.shell", "enabled-extensions", formatGVariantStrv(list)); err != nil {
		return fmt.Errorf("gsettings set: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func installGnome(st *config.InstallState) (string, error) {
	if st == nil {
		return "", errors.New("glow: nil install state")
	}
	if !isGnomeSession() {
		return "", fmt.Errorf("glow: not a GNOME session (XDG_CURRENT_DESKTOP=%q); screen glow on Linux requires GNOME Shell", os.Getenv("XDG_CURRENT_DESKTOP"))
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	gerr := preEnableViaGsettings(ctx, true)
	if _, err := lookPath("gnome-extensions"); err == nil {
		_, _ = runCommand(ctx, "gnome-extensions", "enable", gnome.UUID) // hot-load on X11; harmless no-op otherwise
	}
	if gerr == nil {
		return fmt.Sprintf("GNOME extension installed and enabled for your next login (extracted to %s); log out and back in once - Wayland cannot hot-load extensions", dir), nil
	}
	if _, err := lookPath("gsettings"); err == nil {
		return fmt.Sprintf("extension extracted to %s; automatic enable failed: %v; run `gnome-extensions enable %s`, then log out and back in (Wayland cannot hot-load extensions)", dir, gerr, gnome.UUID), nil
	}
	return fmt.Sprintf("extension extracted to %s; run `gnome-extensions enable %s`, then log out and back in (Wayland cannot hot-load extensions)", dir, gnome.UUID), nil
}

func uninstallGnome(st *config.InstallState) error {
	if st == nil {
		return errors.New("glow: nil install state")
	}
	if !st.GnomeExtensionInstalled {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = preEnableViaGsettings(ctx, false)
	if _, err := lookPath("gnome-extensions"); err == nil {
		_, _ = runCommand(ctx, "gnome-extensions", "disable", gnome.UUID)
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
