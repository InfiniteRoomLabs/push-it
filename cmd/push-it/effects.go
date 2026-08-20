package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"os"
	"time"

	"github.com/InfiniteRoomLabs/push-it/internal/clips"
	"github.com/InfiniteRoomLabs/push-it/internal/config"
	"github.com/InfiniteRoomLabs/push-it/internal/glow"
	"github.com/InfiniteRoomLabs/push-it/internal/hook"
	"github.com/InfiniteRoomLabs/push-it/internal/hue"
	"github.com/InfiniteRoomLabs/push-it/internal/lockfile"
	"github.com/InfiniteRoomLabs/push-it/internal/player"
)

func init() {
	commands["play"] = cmdPlay
	commands["hue"] = cmdHue
	commands["glow"] = cmdGlow
	commands["hook"] = cmdHook
}

func loadConfig(stderr io.Writer) (*config.Config, bool) {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(stderr, "push-it: config: %v\n", err)
		return nil, false
	}
	return cfg, true
}

func deps(cfg *config.Config) hook.Deps {
	rng := rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0))
	return hook.Deps{
		Pick: func() (string, error) {
			files, err := clips.List(cfg.Sound.ClipsDir)
			if err != nil {
				return "", err
			}
			return clips.Pick(files, rng)
		},
		Decode: player.Decode,
		Play:   func(ctx context.Context, c *player.Clip) error { return player.Play(ctx, c, cfg.Sound.Volume) },
		Hue: func(ctx context.Context) error {
			return hue.New(cfg.Hue.Bridge, cfg.Hue.Key, cfg.Hue.Light, cfg.Hue.CertSHA256).Burst(ctx)
		},
		Glow: glow.Run,
	}
}

func cmdPlay(args []string, _ io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("play", flag.ContinueOnError)
	fs.SetOutput(stderr)
	file := fs.String("file", "", "play this file instead of a random clip")
	if fs.Parse(args) != nil {
		return 2
	}
	cfg, ok := loadConfig(stderr)
	if !ok {
		return 1
	}
	d := deps(cfg)
	path := *file
	if path == "" {
		var err error
		if path, err = d.Pick(); err != nil {
			fmt.Fprintf(stderr, "push-it: %v (clips dir: %s)\n", err, cfg.Sound.ClipsDir)
			return 1
		}
	}
	clip, err := d.Decode(path)
	if err != nil {
		fmt.Fprintf(stderr, "push-it: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "playing %s (%s)\n", path, clip.Duration().Round(10*time.Millisecond))
	if err := d.Play(context.Background(), clip); err != nil {
		fmt.Fprintf(stderr, "push-it: %v\n", err)
		return 1
	}
	return 0
}

func cmdHue(_ []string, _ io.Reader, _, stderr io.Writer) int {
	cfg, ok := loadConfig(stderr)
	if !ok {
		return 1
	}
	if cfg.Hue.Bridge == "" || cfg.Hue.Key == "" {
		fmt.Fprintln(stderr, "push-it: hue is not configured; run `push-it install --hue`")
		return 1
	}
	if err := deps(cfg).Hue(context.Background()); err != nil {
		fmt.Fprintf(stderr, "push-it: hue: %v\n", err)
		return 1
	}
	return 0
}

func cmdGlow(args []string, _ io.Reader, _, stderr io.Writer) int {
	fs := flag.NewFlagSet("glow", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dur := fs.Duration("duration", glow.DefaultDuration, "how long to glow")
	if fs.Parse(args) != nil {
		return 2
	}
	if !glow.Available() {
		fmt.Fprintln(stderr, "push-it: no glow backend on this platform/build")
		return 1
	}
	if err := glow.Run(context.Background(), *dur); err != nil {
		fmt.Fprintf(stderr, "push-it: glow: %v\n", err)
		return 1
	}
	return 0
}

// cmdHook handles `hook pre-push` (git entry point) and `hook --run` (the
// detached worker it spawns).
func cmdHook(args []string, stdin io.Reader, _, stderr io.Writer) int {
	fs := flag.NewFlagSet("hook", flag.ContinueOnError)
	fs.SetOutput(stderr)
	runNow := fs.Bool("run", false, "internal: run the celebration in this process")
	if fs.Parse(args) != nil {
		return 2
	}
	cfg, ok := loadConfig(stderr)
	if !ok {
		return 0 // never fail a push
	}
	if *runNow {
		logger := log.New(stderr, "", log.LstdFlags)
		release, got, err := lockfile.Acquire(cfg.LockPath(), 30*time.Second)
		if err != nil {
			logger.Printf("lock: %v", err)
			return 0
		}
		if !got {
			logger.Printf("already playing, skipping")
			return 0
		}
		defer release()
		s, h, g := hook.Enabled(os.Getenv, cfg)
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		hook.Run(ctx, s, h, g, deps(cfg), logger.Printf)
		return 0
	}
	if fs.NArg() < 1 || fs.Arg(0) != "pre-push" {
		fmt.Fprintln(stderr, "usage: push-it hook pre-push")
		return 0
	}
	exe, err := os.Executable()
	if err != nil {
		return 0
	}
	if err := os.MkdirAll(cfg.Dir(), 0o700); err != nil {
		return 0
	}
	if err := hook.PrePush(stdin, os.Getenv, exe, cfg.LogPath()); err != nil {
		fmt.Fprintf(stderr, "[push-it] %v\n", err)
	}
	return 0
}

func contextBackground() context.Context { return context.Background() }
