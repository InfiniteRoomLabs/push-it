//go:build darwin

package glow

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/InfiniteRoomLabs/push-it/internal/config"
)

func TestBackendIsMacOS(t *testing.T) {
	if Backend != "macos" {
		t.Fatalf("Backend = %q", Backend)
	}
}

func TestInstallExtractsHelperWhenEmbedded(t *testing.T) {
	orig := helperBinary
	helperBinary = []byte("#!/bin/sh\nexit 0\n")
	t.Cleanup(func() { helperBinary = orig })
	t.Setenv("PUSH_IT_CONFIG_DIR", t.TempDir())
	var st config.InstallState
	if _, err := Install(&st); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(st.MacOSHelperPath)
	if err != nil || fi.Mode().Perm()&0o111 == 0 {
		t.Fatalf("helper not extracted executable: %v", err)
	}
	if _, err := Install(&st); err != nil { // idempotent
		t.Fatal(err)
	}
	if err := Uninstall(&st); err != nil || st.MacOSHelperPath != "" {
		t.Fatalf("uninstall: %v %+v", err, st)
	}
}

func TestInstallWithoutEmbeddedHelperErrors(t *testing.T) {
	orig := helperBinary
	helperBinary = nil
	t.Cleanup(func() { helperBinary = orig })
	t.Setenv("PUSH_IT_CONFIG_DIR", t.TempDir())
	var st config.InstallState
	if _, err := Install(&st); err == nil || !strings.Contains(err.Error(), "glowhelper") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunPassesDuration(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PUSH_IT_CONFIG_DIR", dir)
	_ = os.WriteFile(filepath.Join(dir, "glow-macos"), []byte("stub"), 0o755)
	var got []string
	orig := runHelper
	runHelper = func(_ context.Context, p string, args ...string) error {
		got = append([]string{p}, args...)
		return nil
	}
	t.Cleanup(func() { runHelper = orig })
	if err := Run(context.Background(), 1500*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[1] != "--duration" || got[2] != "1.500" {
		t.Fatalf("args = %v", got)
	}
}
