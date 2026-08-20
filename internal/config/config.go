// Package config loads and saves push-it's JSON configuration.
package config

import (
	"encoding/json"
	"errors"
	"io/fs"
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

	dir string
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
	return &Config{
		Sound: Sound{Enabled: true, ClipsDir: filepath.Join(d, "clips"), Volume: 0.7},
		Hue:   Hue{Enabled: false, Light: 1},
		Glow:  Glow{Enabled: true},
		dir:   d,
	}, nil
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
	applyEnv(c)
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
		}
	}
}

// Save writes the config with restrictive permissions (it holds the Hue key).
func (c *Config) Save() error {
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(c.dir, "config.json"), append(data, '\n'), 0o600)
}

// Dir returns the directory this config was loaded from.
func (c *Config) Dir() string { return c.dir }

// LogPath is where the detached hook run writes its log.
func (c *Config) LogPath() string { return filepath.Join(c.dir, "push-it.log") }

// LockPath guards against overlapping playbacks.
func (c *Config) LockPath() string { return filepath.Join(c.dir, "play.lock") }
