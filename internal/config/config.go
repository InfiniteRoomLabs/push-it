// Package config loads and saves push-it's JSON configuration.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strconv"
)

type Sound struct {
	Enabled  bool    `json:"enabled"`
	ClipsDir string  `json:"clips_dir"`
	Volume   float64 `json:"volume"`
}

type Hue struct {
	Enabled    bool   `json:"enabled"`
	Bridge     string `json:"bridge"`
	Key        string `json:"key"`
	Light      int    `json:"light"`
	CertSHA256 string `json:"cert_sha256"` // trust-on-first-use pin of the bridge's TLS cert
}

type Glow struct {
	Enabled bool `json:"enabled"`
}

// InstallState records exactly what `push-it install` changed so that
// `push-it uninstall` can reverse it and nothing else.
type InstallState struct {
	HooksPathSetByUs        bool   `json:"hooks_path_set_by_us"`
	HooksPath               string `json:"hooks_path"`
	PrePushAppendedTo       string `json:"pre_push_line_appended_to"`
	PrePushCreatedByUs      bool   `json:"pre_push_created_by_us"`
	GnomeExtensionInstalled bool   `json:"gnome_extension_installed"`
	MacOSHelperPath         string `json:"macos_helper_path"`
}

type Config struct {
	Sound        Sound        `json:"sound"`
	Hue          Hue          `json:"hue"`
	Glow         Glow         `json:"glow"`
	InstallState InstallState `json:"install_state"`

	// Warnings collects non-fatal problems found while loading (out-of-range
	// values that were corrected, malformed env overrides). `push-it doctor`
	// prints them. Never persisted.
	Warnings []string `json:"-"`

	dir string
	// fileHue holds the Hue values as they stood before PUSH_IT_HUE_* env
	// overrides were applied (i.e. what is actually on disk). Save uses it
	// so a transient env override never gets written into the file.
	fileHue Hue
}

// Dir returns the configuration directory: $PUSH_IT_CONFIG_DIR if set,
// otherwise the OS user config dir plus "push-it".
func Dir() (string, error) {
	if d := os.Getenv("PUSH_IT_CONFIG_DIR"); d != "" {
		return d, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "push-it"), nil
}

// Path returns the config file path.
func Path() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.json"), nil
}

// Default returns a config with sound and glow enabled and hue disabled.
func Default() (*Config, error) {
	d, err := Dir()
	if err != nil {
		return nil, err
	}
	c := &Config{
		Sound: Sound{Enabled: true, ClipsDir: filepath.Join(d, "clips"), Volume: 0.7},
		Hue:   Hue{Enabled: false, Light: 1},
		Glow:  Glow{Enabled: true},
		dir:   d,
	}
	c.fileHue = c.Hue
	return c, nil
}

// Load reads the config file, falling back to Default when it does not
// exist, then applies PUSH_IT_HUE_* environment overrides.
func Load() (*Config, error) {
	c, err := Default()
	if err != nil {
		return nil, err
	}
	p := filepath.Join(c.dir, "config.json")
	data, err := os.ReadFile(p)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return nil, err
	default:
		if err := json.Unmarshal(data, c); err != nil {
			return nil, err
		}
	}
	c.fileHue = c.Hue // on-disk values, before env overrides are applied below
	applyEnv(c)
	c.normalize()
	return c, nil
}

func applyEnv(c *Config) {
	if v := os.Getenv("PUSH_IT_HUE_BRIDGE"); v != "" {
		c.Hue.Bridge = v
	}
	if v := os.Getenv("PUSH_IT_HUE_KEY"); v != "" {
		c.Hue.Key = v
	}
	if v := os.Getenv("PUSH_IT_HUE_LIGHT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Hue.Light = n
		} else {
			c.Warnings = append(c.Warnings, fmt.Sprintf("PUSH_IT_HUE_LIGHT=%q is not a number; ignored", v))
		}
	}
}

// normalize corrects out-of-range values in place, recording a warning for
// each correction. It never fails: the hook path must always get a usable
// config, so bad numbers are fixed, not fatal.
func (c *Config) normalize() {
	v := c.Sound.Volume
	switch {
	case math.IsNaN(v):
		c.Sound.Volume = 0.7
		c.Warnings = append(c.Warnings, "sound.volume was not a number; reset to 0.7")
	case v < 0:
		c.Sound.Volume = 0
		c.Warnings = append(c.Warnings, fmt.Sprintf("sound.volume %v below 0; clamped to 0", v))
	case v > 1:
		c.Sound.Volume = 1
		c.Warnings = append(c.Warnings, fmt.Sprintf("sound.volume %v above 1; clamped to 1", v))
	}
	if c.Hue.Light < 1 {
		c.Warnings = append(c.Warnings, fmt.Sprintf("hue.light %d below 1; set to 1", c.Hue.Light))
		c.Hue.Light = 1
	}
	// fileHue must reflect what normalize() actually corrects on-disk values
	// to, so hueForSave's revert-to-file-value path never re-persists a
	// bad number it just fixed.
	if c.fileHue.Light < 1 {
		c.fileHue.Light = 1
	}
}

// hueForSave returns the Hue values to persist: any field that still equals
// a live PUSH_IT_HUE_* env override is replaced with its pre-override,
// on-disk value, so a transient env var (e.g. an ephemeral key) never gets
// written into the file. Fields the caller changed to something else are
// persisted as set.
func (c *Config) hueForSave() Hue {
	h := c.Hue
	if v := os.Getenv("PUSH_IT_HUE_BRIDGE"); v != "" && h.Bridge == v {
		h.Bridge = c.fileHue.Bridge
	}
	if v := os.Getenv("PUSH_IT_HUE_KEY"); v != "" && h.Key == v {
		h.Key = c.fileHue.Key
	}
	if v := os.Getenv("PUSH_IT_HUE_LIGHT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			// normalize() floors an out-of-range Hue.Light the same way it
			// would floor this parsed env value, so mirror that here before
			// comparing - otherwise a normalized light no longer equals the
			// override that produced it, and this looks like an explicit
			// user change instead of a transient env var.
			if n < 1 {
				n = 1
			}
			if h.Light == n {
				h.Light = c.fileHue.Light
			}
		}
	}
	return h
}

// Save writes the config with restrictive permissions (it holds the Hue
// key). Permissions are enforced even when the dir/file already existed
// with looser modes.
func (c *Config) Save() error {
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(c.dir, 0o700); err != nil {
		return err
	}
	saved := *c
	saved.Hue = c.hueForSave()
	data, err := json.MarshalIndent(&saved, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(c.dir, "config.json")
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

// Dir returns the directory this config was loaded from.
func (c *Config) Dir() string { return c.dir }

// LogPath is where the detached hook run writes its log.
func (c *Config) LogPath() string { return filepath.Join(c.dir, "push-it.log") }

// LockPath guards against overlapping playbacks.
func (c *Config) LockPath() string { return filepath.Join(c.dir, "play.lock") }
