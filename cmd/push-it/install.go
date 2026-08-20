package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/InfiniteRoomLabs/push-it/internal/clips"
	"github.com/InfiniteRoomLabs/push-it/internal/config"
	"github.com/InfiniteRoomLabs/push-it/internal/glow"
	"github.com/InfiniteRoomLabs/push-it/internal/hue"
	"github.com/InfiniteRoomLabs/push-it/internal/installer"
)

func init() {
	commands["install"] = cmdInstall
	commands["uninstall"] = cmdUninstall
	commands["doctor"] = cmdDoctor
}

type prompter struct {
	in  *bufio.Scanner
	out io.Writer
}

func (p prompter) yesNo(q string, def bool) bool {
	hint := "[Y/n]"
	if !def {
		hint = "[y/N]"
	}
	fmt.Fprintf(p.out, "%s %s ", q, hint)
	if !p.in.Scan() {
		return def
	}
	switch strings.ToLower(strings.TrimSpace(p.in.Text())) {
	case "y", "yes":
		return true
	case "n", "no":
		return false
	default:
		return def
	}
}

func (p prompter) text(q, def string) string {
	if def != "" {
		fmt.Fprintf(p.out, "%s [%s]: ", q, def)
	} else {
		fmt.Fprintf(p.out, "%s: ", q)
	}
	if !p.in.Scan() {
		return def
	}
	if s := strings.TrimSpace(p.in.Text()); s != "" {
		return s
	}
	return def
}

// secret behaves like text but never echoes cur (a stored/env secret): it
// shows "[keep current]" instead of the value, and empty input keeps cur.
func (p prompter) secret(q, cur string) string {
	label := q
	if cur != "" {
		label = q + " [keep current]"
	}
	fmt.Fprintf(p.out, "%s: ", label)
	if !p.in.Scan() {
		return cur
	}
	if s := strings.TrimSpace(p.in.Text()); s != "" {
		return s
	}
	return cur
}

func glowDefault() bool {
	switch runtime.GOOS {
	case "linux":
		return strings.Contains(strings.ToLower(os.Getenv("XDG_CURRENT_DESKTOP")), "gnome")
	default:
		return true
	}
}

// short returns the first 12 bytes of s for display, or s itself if it is
// already shorter (e.g. a hand-edited pin) so we never panic slicing it.
func short(s string) string {
	if len(s) < 12 {
		return s
	}
	return s[:12]
}

func cmdInstall(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(stderr)
	sound := fs.Bool("sound", false, "install the sound component")
	hueOn := fs.Bool("hue", false, "install the Philips Hue component")
	glowOn := fs.Bool("glow", false, "install the screen glow component")
	all := fs.Bool("all", false, "install everything")
	yes := fs.Bool("yes", false, "accept defaults without prompting")
	if fs.Parse(args) != nil {
		return 2
	}
	fresh := true
	if p, err := config.Path(); err == nil {
		if _, err := os.Stat(p); err == nil {
			fresh = false
		}
	}
	cfg, ok := loadConfig(stderr)
	if !ok {
		return 1
	}
	p := prompter{in: bufio.NewScanner(stdin), out: stdout}
	explicit := *sound || *hueOn || *glowOn
	// wantSound/wantHue drive this-run behaviour below (the clips-empty hint
	// and whether to prompt/check Hue). Glow has no such this-run behaviour
	// left - the installer is gated on cfg.Glow.Enabled directly below - so
	// there is no wantGlow.
	var wantSound, wantHue bool
	switch {
	case *all:
		wantSound, wantHue = true, true
		cfg.Sound.Enabled, cfg.Hue.Enabled, cfg.Glow.Enabled = true, true, true
	case explicit:
		// Flags are additive: touch only the components explicitly named on
		// the command line and leave the rest as loaded from disk. On a
		// fresh config (no file on disk yet) there is nothing to leave as
		// loaded, so start all three off before enabling the named ones -
		// otherwise config.Default()'s Glow.Enabled=true would leak through
		// on an unrelated `install --sound`.
		wantSound, wantHue = *sound, *hueOn
		if fresh {
			cfg.Sound.Enabled, cfg.Hue.Enabled, cfg.Glow.Enabled = false, false, false
		}
		if *sound {
			cfg.Sound.Enabled = true
		}
		if *hueOn {
			cfg.Hue.Enabled = true
		}
		if *glowOn {
			cfg.Glow.Enabled = true
		}
	case *yes:
		wantSound, wantHue = true, false
		cfg.Sound.Enabled, cfg.Hue.Enabled, cfg.Glow.Enabled = wantSound, wantHue, glowDefault()
	default:
		wantSound = p.yesNo("Play a clip on push?", true)
		wantHue = p.yesNo("Rainbow a Philips Hue light on push?", false)
		glowAns := p.yesNo("Glow the screen edges on push?", glowDefault())
		cfg.Sound.Enabled, cfg.Hue.Enabled, cfg.Glow.Enabled = wantSound, wantHue, glowAns
	}

	if wantHue {
		switch {
		case *yes && (cfg.Hue.Bridge == "" || cfg.Hue.Key == ""):
			// Non-interactive and unconfigured: skip rather than prompt (we
			// can't) or save a broken bridge/key.
			fmt.Fprintln(stdout, "hue: skipped (set PUSH_IT_HUE_BRIDGE and PUSH_IT_HUE_KEY or run install --hue interactively)")
			cfg.Hue.Enabled = false
		default:
			if !*yes || cfg.Hue.Bridge == "" || cfg.Hue.Key == "" {
				cfg.Hue.Bridge = p.text("Hue bridge host or IP", cfg.Hue.Bridge)
				cfg.Hue.Key = p.secret("Hue API key", cfg.Hue.Key)
				if n, err := strconv.Atoi(p.text("Light ID", strconv.Itoa(cfg.Hue.Light))); err == nil {
					cfg.Hue.Light = n
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			fp, err := hue.Fingerprint(ctx, cfg.Hue.Bridge)
			if err == nil {
				if cfg.Hue.CertSHA256 != "" && cfg.Hue.CertSHA256 != fp {
					if *yes {
						// TOFU is strict here on purpose: a scripted `install
						// --yes` must never auto-trust a changed bridge cert.
						cancel()
						fmt.Fprintf(stderr, "push-it: hue bridge certificate changed (was %s, now %s); refusing to re-pin non-interactively - run install --hue without --yes to review it\n", short(cfg.Hue.CertSHA256), short(fp))
						return 1
					}
					fmt.Fprintf(stdout, "hue: bridge certificate changed (was %s, now %s)\n", short(cfg.Hue.CertSHA256), short(fp))
					if !p.yesNo("Trust the new certificate?", false) {
						cancel()
						fmt.Fprintln(stdout, "no changes were made to the Hue configuration")
						return 1
					}
				}
				cfg.Hue.CertSHA256 = fp
				fmt.Fprintf(stdout, "hue: pinned bridge certificate %s...\n", short(fp))
				err = hue.New(cfg.Hue.Bridge, cfg.Hue.Key, cfg.Hue.Light, fp).Ping(ctx)
			}
			cancel()
			if err != nil {
				fmt.Fprintf(stderr, "push-it: hue check failed: %v (saved anyway; fix with `push-it install --hue`)\n", err)
			} else {
				fmt.Fprintln(stdout, "hue: bridge reachable")
			}
		}
	}

	if err := os.MkdirAll(cfg.Sound.ClipsDir, 0o755); err != nil {
		fmt.Fprintf(stderr, "push-it: %v\n", err)
		return 1
	}
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(stderr, "push-it: %v\n", err)
		return 1
	}
	if err := installer.WireHook(installer.CLIGit{}, cfg.Dir(), exe, &cfg.InstallState); err != nil {
		fmt.Fprintf(stderr, "push-it: hook: %v\n", err)
		return 1
	}
	if cfg.Glow.Enabled {
		if err := glow.Install(&cfg.InstallState); err != nil {
			fmt.Fprintf(stderr, "push-it: glow: %v\n", err)
		}
	}
	if err := cfg.Save(); err != nil {
		_ = installer.UnwireHook(installer.CLIGit{}, &cfg.InstallState)
		_ = glow.Uninstall(&cfg.InstallState)
		fmt.Fprintf(stderr, "push-it: could not save config (%v); install rolled back\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "installed. config: %s\n", cfg.Dir())
	fmt.Fprintf(stdout, "enabled: sound=%v hue=%v glow=%v\n", cfg.Sound.Enabled, cfg.Hue.Enabled, cfg.Glow.Enabled)
	if files, _ := clips.List(cfg.Sound.ClipsDir); wantSound && len(files) == 0 {
		fmt.Fprintf(stdout, "no clips yet  -  drop .mp3/.wav files into %s\nsee docs/make-your-own-clips.md to cut your own.\n", cfg.Sound.ClipsDir)
	}
	return 0
}

func cmdUninstall(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	fs.SetOutput(stderr)
	yes := fs.Bool("yes", false, "do not prompt; keep clips and config")
	if fs.Parse(args) != nil {
		return 2
	}
	cfg, ok := loadConfig(stderr)
	if !ok {
		return 1
	}
	if err := installer.UnwireHook(installer.CLIGit{}, &cfg.InstallState); err != nil {
		fmt.Fprintf(stderr, "push-it: hook: %v\n", err)
		return 1
	}
	if err := glow.Uninstall(&cfg.InstallState); err != nil {
		fmt.Fprintf(stderr, "push-it: glow: %v\n", err)
	}
	cfg.Sound.Enabled, cfg.Hue.Enabled, cfg.Glow.Enabled = false, false, false
	if err := cfg.Save(); err != nil {
		fmt.Fprintf(stderr, "push-it: %v\n", err)
		return 1
	}
	p := prompter{in: bufio.NewScanner(stdin), out: stdout}
	if !*yes && p.yesNo(fmt.Sprintf("Delete config and clips in %s too?", cfg.Dir()), false) {
		if err := os.RemoveAll(cfg.Dir()); err != nil {
			fmt.Fprintf(stderr, "push-it: %v\n", err)
			return 1
		}
	}
	fmt.Fprintln(stdout, "uninstalled. the push-it binary itself was left in place.")
	return 0
}

func cmdDoctor(_ []string, _ io.Reader, stdout, stderr io.Writer) int {
	cfg, ok := loadConfig(stderr)
	if !ok {
		return 1
	}
	path, _ := config.Path()
	fmt.Fprintf(stdout, "config:  %s\n", path)
	fmt.Fprintf(stdout, "sound:   enabled=%v volume=%.2f\n", cfg.Sound.Enabled, cfg.Sound.Volume)
	files, err := clips.List(cfg.Sound.ClipsDir)
	fmt.Fprintf(stdout, "clips:   %d in %s", len(files), cfg.Sound.ClipsDir)
	if err != nil {
		fmt.Fprintf(stdout, " (error: %v)", err)
	}
	fmt.Fprintln(stdout)
	fmt.Fprintf(stdout, "hue:     enabled=%v", cfg.Hue.Enabled)
	if cfg.Hue.Enabled {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err := hue.New(cfg.Hue.Bridge, cfg.Hue.Key, cfg.Hue.Light, cfg.Hue.CertSHA256).Ping(ctx)
		cancel()
		if err != nil {
			fmt.Fprintf(stdout, " bridge=%s UNREACHABLE (%v)", cfg.Hue.Bridge, err)
		} else {
			fmt.Fprintf(stdout, " bridge=%s ok", cfg.Hue.Bridge)
		}
	}
	fmt.Fprintln(stdout)
	fmt.Fprintf(stdout, "glow:    enabled=%v backend=%s\n", cfg.Glow.Enabled, glow.Backend)
	fmt.Fprintf(stdout, "hook:    hooksPath=%s setByUs=%v appendedTo=%s\n", cfg.InstallState.HooksPath, cfg.InstallState.HooksPathSetByUs, cfg.InstallState.PrePushAppendedTo)
	return 0
}
