package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/InfiniteRoomLabs/push-it/internal/config"
	"github.com/InfiniteRoomLabs/push-it/internal/glow"
)

// hueTestServer starts an httptest TLS server that answers Hue v1 API
// requests well enough for install/doctor's Fingerprint+Ping checks to
// succeed, and returns the bridge address (host:port, no scheme) to set as
// PUSH_IT_HUE_BRIDGE - keeping these tests hermetic instead of dialing a
// real port.
func hueTestServer(t *testing.T) (bridge string, srv *httptest.Server) {
	t.Helper()
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"state":{"on":false,"bri":1,"hue":1,"sat":1}}`))
			return
		}
		_, _ = w.Write([]byte(`[{"success":{}}]`))
	}))
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "https://"), srv
}

func TestDoctorOnFreshConfig(t *testing.T) {
	isolateInstall(t)
	var out, errOut bytes.Buffer
	if code := run([]string{"doctor"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errOut.String())
	}
	for _, want := range []string{"config:", "sound:", "clips:", "hue:", "glow:"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("doctor output missing %q:\n%s", want, out.String())
		}
	}
}

func TestInstallAndUninstallSoundOnly(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	tmp := isolateInstall(t)
	var out, errOut bytes.Buffer
	if code := run([]string{"install", "--sound", "--yes"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("install code=%d stderr=%s", code, errOut.String())
	}
	if _, err := os.Stat(filepath.Join(tmp, "cfg", "hooks", "pre-push")); err != nil {
		t.Fatal("hook not written")
	}
	if _, err := os.Stat(filepath.Join(tmp, "cfg", "clips")); err != nil {
		t.Fatal("clips dir not created")
	}
	out.Reset()
	if code := run([]string{"uninstall", "--yes"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("uninstall code=%d stderr=%s", code, errOut.String())
	}
	if _, err := os.Stat(filepath.Join(tmp, "cfg", "hooks", "pre-push")); err == nil {
		t.Fatal("hook not removed")
	}
}

func TestInstallPrintsGlowNote(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	isolateInstall(t)
	glow.Install = func(*config.InstallState) (string, error) { return "log out and back in", nil }
	var out, errOut bytes.Buffer
	if code := run([]string{"install", "--glow", "--yes"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "glow: log out and back in") {
		t.Fatalf("note not printed:\n%s", out.String())
	}
}

// TestInstallGlowErrorDisablesGlow covers I2: a failed glow.Install must not
// leave glow.Enabled=true in the saved config, and must tell the user why.
func TestInstallGlowErrorDisablesGlow(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	tmp := isolateInstall(t)
	glow.Install = func(*config.InstallState) (string, error) { return "", errors.New("boom") }
	var out, errOut bytes.Buffer
	if code := run([]string{"install", "--glow", "--yes"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "glow disabled") {
		t.Fatalf("stderr = %q, want it to mention glow disabled", errOut.String())
	}
	if !strings.Contains(out.String(), "glow=false") {
		t.Fatalf("stdout = %q, want the enabled summary to show glow=false", out.String())
	}
	b, err := os.ReadFile(filepath.Join(tmp, "cfg", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		Glow struct {
			Enabled bool `json:"enabled"`
		} `json:"glow"`
	}
	if err := json.Unmarshal(b, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Glow.Enabled {
		t.Fatalf("glow.enabled should be false after a failed glow.Install:\n%s", b)
	}
}

func TestInstallInteractiveReadsAnswers(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	tmp := isolateInstall(t)
	var out, errOut bytes.Buffer
	// sound? n   hue? n   glow? n
	if code := run([]string{"install"}, strings.NewReader("n\nn\nn\n"), &out, &errOut); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errOut.String())
	}
	b, _ := os.ReadFile(filepath.Join(tmp, "cfg", "config.json"))
	if !strings.Contains(string(b), `"enabled": false`) || strings.Contains(string(b), `"enabled": true`) {
		t.Fatalf("config:\n%s", b)
	}
}

// TestInstallExplicitFlagsAreAdditive verifies that a later `install --hue`
// does not silently disable sound (or glow) that an earlier `install --all`
// turned on: single-component flags only touch the components they name.
func TestInstallExplicitFlagsAreAdditive(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	tmp := isolateInstall(t)
	var out, errOut bytes.Buffer
	if code := run([]string{"install", "--all", "--yes"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("install --all code=%d stderr=%s", code, errOut.String())
	}

	bridge, _ := hueTestServer(t)
	t.Setenv("PUSH_IT_HUE_BRIDGE", bridge)
	t.Setenv("PUSH_IT_HUE_KEY", "k")
	out.Reset()
	errOut.Reset()
	if code := run([]string{"install", "--hue", "--yes"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("install --hue code=%d stderr=%s", code, errOut.String())
	}

	b, err := os.ReadFile(filepath.Join(tmp, "cfg", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		Sound struct {
			Enabled bool `json:"enabled"`
		} `json:"sound"`
	}
	if err := json.Unmarshal(b, &saved); err != nil {
		t.Fatal(err)
	}
	if !saved.Sound.Enabled {
		t.Fatalf("sound should still be enabled after `install --hue`:\n%s", b)
	}
}

// TestInstallFlagsOnFreshConfigStartFromOff guards against
// config.Default()'s Glow.Enabled=true leaking through an unrelated
// `install --sound` on a machine with no config file yet: with nothing on
// disk to treat as "already loaded", an explicit single-component flag must
// start all three components off before enabling the one named.
func TestInstallFlagsOnFreshConfigStartFromOff(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	tmp := isolateInstall(t)
	var out, errOut bytes.Buffer
	if code := run([]string{"install", "--sound", "--yes"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("install --sound code=%d stderr=%s", code, errOut.String())
	}
	b, err := os.ReadFile(filepath.Join(tmp, "cfg", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		Sound struct {
			Enabled bool `json:"enabled"`
		} `json:"sound"`
		Hue struct {
			Enabled bool `json:"enabled"`
		} `json:"hue"`
		Glow struct {
			Enabled bool `json:"enabled"`
		} `json:"glow"`
	}
	if err := json.Unmarshal(b, &saved); err != nil {
		t.Fatal(err)
	}
	if !saved.Sound.Enabled || saved.Hue.Enabled || saved.Glow.Enabled {
		t.Fatalf("fresh `install --sound` should start sound=true, hue=false, glow=false:\n%s", b)
	}
}

// TestInstallHueYesSkipsWhenUnconfigured covers the --yes + no-Hue-env case:
// install must not prompt (it can't, non-interactively) and must not save
// hue.enabled=true with an empty bridge/key.
func TestInstallHueYesSkipsWhenUnconfigured(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	tmp := isolateInstall(t)
	var out, errOut bytes.Buffer
	if code := run([]string{"install", "--hue", "--yes"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("install --hue --yes code=%d stderr=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "hue: skipped") {
		t.Fatalf("stdout = %q, want a \"hue: skipped\" notice", out.String())
	}
	b, err := os.ReadFile(filepath.Join(tmp, "cfg", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		Hue struct {
			Enabled bool `json:"enabled"`
		} `json:"hue"`
	}
	if err := json.Unmarshal(b, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Hue.Enabled {
		t.Fatalf("hue.enabled should be false when --yes has no bridge/key:\n%s", b)
	}
}

// TestInstallHueYesRefusesChangedCert covers the security-critical TOFU
// path: `install --hue --yes` must never auto-trust a bridge certificate
// that differs from the stored pin. It must refuse non-interactively,
// leave the old pin on disk, and exit 1.
func TestDoctorReportsProblems(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PUSH_IT_CONFIG_DIR", dir)
	t.Setenv("NO_SOUND", "1")
	raw := `{"sound":{"enabled":true,"clips_dir":"` + filepath.ToSlash(dir) + `","volume":9},"hue":{"enabled":false,"light":1},"glow":{"enabled":false}}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	oldProbe := audioProbe
	t.Cleanup(func() { audioProbe = oldProbe })
	audioProbe = func() error { return errors.New("player: no server (stub)") }

	var out, errOut strings.Builder
	if code := run([]string{"doctor"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut.String())
	}
	got := out.String()
	for _, want := range []string{
		"warn:", "sound.volume", // Task 1 warning surfaced
		"audio:", "UNREACHABLE", // stubbed probe failure
		"env:", "NO_SOUND", // active kill switch
		"log:", // log writability line
	} {
		if !strings.Contains(got, want) {
			t.Errorf("doctor output missing %q:\n%s", want, got)
		}
	}
}

func TestDoctorHookStaleness(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PUSH_IT_CONFIG_DIR", dir)
	hook := filepath.Join(dir, "pre-push")
	// A hook file with our marker but pointing at a different binary path.
	if err := os.WriteFile(hook, []byte("#!/bin/sh\n# >>> push-it >>>\n'/nonexistent/push-it' hook pre-push \"$@\" || true\n# <<< push-it <<<\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := `{"install_state":{"pre_push_line_appended_to":"` + filepath.ToSlash(hook) + `"}}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errOut strings.Builder
	if code := run([]string{"doctor"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "different binary") {
		t.Errorf("doctor did not flag a stale hook block:\n%s", out.String())
	}
}

func TestInstallHueYesRefusesChangedCert(t *testing.T) {
	tmp := isolateInstall(t)
	cfgDir := filepath.Join(tmp, "cfg")

	stalePin := strings.Repeat("0", 64)
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	preExisting := `{"hue":{"cert_sha256":"` + stalePin + `"}}`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(preExisting), 0o600); err != nil {
		t.Fatal(err)
	}

	bridge, _ := hueTestServer(t)
	t.Setenv("PUSH_IT_HUE_BRIDGE", bridge)
	t.Setenv("PUSH_IT_HUE_KEY", "k")

	var out, errOut bytes.Buffer
	code := run([]string{"install", "--hue", "--yes"}, strings.NewReader(""), &out, &errOut)
	if code != 1 {
		t.Fatalf("code=%d, want 1; stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
	if !strings.Contains(errOut.String(), "refusing to re-pin non-interactively") {
		t.Fatalf("stderr = %q, want the refusal message", errOut.String())
	}

	b, err := os.ReadFile(filepath.Join(cfgDir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		Hue struct {
			CertSHA256 string `json:"cert_sha256"`
		} `json:"hue"`
	}
	if err := json.Unmarshal(b, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Hue.CertSHA256 != stalePin {
		t.Fatalf("cert_sha256 = %q, want unchanged stale pin %q", saved.Hue.CertSHA256, stalePin)
	}
}
