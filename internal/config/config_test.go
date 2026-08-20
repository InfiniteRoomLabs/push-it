package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDirHonoursOverride(t *testing.T) {
	t.Setenv("PUSH_IT_CONFIG_DIR", "/tmp/x/push-it")
	d, err := Dir()
	if err != nil || d != "/tmp/x/push-it" {
		t.Fatalf("Dir() = %q, %v", d, err)
	}
}

func TestLoadReturnsDefaultWhenMissing(t *testing.T) {
	t.Setenv("PUSH_IT_CONFIG_DIR", t.TempDir())
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !c.Sound.Enabled || c.Sound.Volume != 0.7 || c.Hue.Enabled || c.Hue.Light != 1 || !c.Glow.Enabled {
		t.Fatalf("unexpected defaults: %+v", c)
	}
	if filepath.Base(c.Sound.ClipsDir) != "clips" {
		t.Fatalf("clips dir = %q", c.Sound.ClipsDir)
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PUSH_IT_CONFIG_DIR", dir)
	c, _ := Load()
	c.Hue.Enabled = true
	c.Hue.Bridge = "192.168.1.2"
	c.Hue.Key = "secret"
	c.InstallState.HooksPathSetByUs = true
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		fi, _ := os.Stat(filepath.Join(dir, "config.json"))
		if fi.Mode().Perm() != 0o600 {
			t.Fatalf("mode = %o, want 600", fi.Mode().Perm())
		}
	}
	c2, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c2.Hue.Bridge != "192.168.1.2" || c2.Hue.Key != "secret" || !c2.InstallState.HooksPathSetByUs {
		t.Fatalf("round trip lost data: %+v", c2)
	}
}

func TestEnvOverridesHue(t *testing.T) {
	t.Setenv("PUSH_IT_CONFIG_DIR", t.TempDir())
	t.Setenv("PUSH_IT_HUE_BRIDGE", "10.0.0.9")
	t.Setenv("PUSH_IT_HUE_KEY", "envkey")
	t.Setenv("PUSH_IT_HUE_LIGHT", "7")
	c, _ := Load()
	if c.Hue.Bridge != "10.0.0.9" || c.Hue.Key != "envkey" || c.Hue.Light != 7 {
		t.Fatalf("env overrides not applied: %+v", c.Hue)
	}
}

func TestSaveTightensExistingPermissions(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PUSH_IT_CONFIG_DIR", dir)
	if runtime.GOOS != "windows" {
		if err := os.Chmod(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		di, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if di.Mode().Perm() != 0o700 {
			t.Fatalf("dir mode = %o, want 700", di.Mode().Perm())
		}
		fi, err := os.Stat(filepath.Join(dir, "config.json"))
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o600 {
			t.Fatalf("file mode = %o, want 600", fi.Mode().Perm())
		}
	}
}

func TestSaveDoesNotPersistTransientEnvOverride(t *testing.T) {
	t.Setenv("PUSH_IT_CONFIG_DIR", t.TempDir())
	t.Setenv("PUSH_IT_HUE_KEY", "envkey")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Hue.Key != "envkey" {
		t.Fatalf("Hue.Key = %q, want envkey", c.Hue.Key)
	}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	os.Unsetenv("PUSH_IT_HUE_KEY")
	c2, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c2.Hue.Key != "" {
		t.Fatalf("Hue.Key persisted the env override: %q, want empty", c2.Hue.Key)
	}
}

func TestSavePersistsExplicitChangeOverEnvOverride(t *testing.T) {
	t.Setenv("PUSH_IT_CONFIG_DIR", t.TempDir())
	t.Setenv("PUSH_IT_HUE_KEY", "envkey")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	c.Hue.Key = "explicit"
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	os.Unsetenv("PUSH_IT_HUE_KEY")
	c2, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c2.Hue.Key != "explicit" {
		t.Fatalf("Hue.Key = %q, want explicit", c2.Hue.Key)
	}
}

func TestLogAndLockPaths(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PUSH_IT_CONFIG_DIR", dir)
	c, _ := Load()
	if c.LogPath() != filepath.Join(dir, "push-it.log") || c.LockPath() != filepath.Join(dir, "play.lock") {
		t.Fatalf("paths: %q %q", c.LogPath(), c.LockPath())
	}
}
