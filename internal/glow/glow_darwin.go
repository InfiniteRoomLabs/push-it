//go:build darwin

package glow

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/InfiniteRoomLabs/push-it/internal/config"
)

var errNoHelper = errors.New("glow: this build has no macOS helper (built without -tags glowhelper)")

var runHelper = func(ctx context.Context, path string, args ...string) error {
	cmd := exec.CommandContext(ctx, path, args...)
	return cmd.Run()
}

func init() {
	Backend = "macos"
	Run = runMac
	Install = installMac
	Uninstall = uninstallMac
}

func helperPath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "glow-macos"), nil
}

func runMac(ctx context.Context, d time.Duration) error {
	p, err := helperPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(p); err != nil {
		if len(helperBinary) == 0 {
			return errNoHelper
		}
		return fmt.Errorf("glow: helper not installed; run `push-it install --glow`: %w", err)
	}
	return runHelper(ctx, p, "--duration", fmt.Sprintf("%.3f", d.Seconds()))
}

func installMac(st *config.InstallState) (string, error) {
	if st == nil {
		return "", errors.New("glow: nil install state")
	}
	if len(helperBinary) == 0 {
		return "", errNoHelper
	}
	p, err := helperPath()
	if err != nil {
		return "", err
	}
	want := sha256.Sum256(helperBinary)
	if cur, err := os.ReadFile(p); err == nil && bytes.Equal(want[:], hashOf(cur)) {
		st.MacOSHelperPath = p
		return "", nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return "", err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, helperBinary, 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, p); err != nil {
		return "", err
	}
	st.MacOSHelperPath = p
	return "", nil
}

func hashOf(b []byte) []byte { h := sha256.Sum256(b); return h[:] }

func uninstallMac(st *config.InstallState) error {
	if st == nil {
		return errors.New("glow: nil install state")
	}
	if st.MacOSHelperPath == "" {
		return nil
	}
	if err := os.Remove(st.MacOSHelperPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	st.MacOSHelperPath = ""
	return nil
}
