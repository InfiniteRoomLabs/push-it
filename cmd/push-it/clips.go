package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/InfiniteRoomLabs/push-it/internal/clipper"
	"github.com/InfiniteRoomLabs/push-it/internal/player"
)

func init() {
	commands["clips"] = cmdClips
}

func cmdClips(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: push-it clips cut|review ...")
		return 2
	}
	switch args[0] {
	case "cut":
		return cmdClipsCut(args[1:], stdout, stderr)
	case "review":
		return cmdClipsReview(args[1:], stdin, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "push-it clips: unknown subcommand %q\n", args[0])
		return 2
	}
}

func splitList(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' })
}

func cmdClipsCut(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("clips cut", flag.ContinueOnError)
	fs.SetOutput(stderr)
	def := clipper.DefaultOptions()
	phrase := fs.String("phrase", strings.Join(def.Phrase, " "), "words a clip must start with")
	allow := fs.String("allow", strings.Join(def.Allow, ","), "extra words allowed inside a clip (comma-separated)")
	gap := fs.Float64("gap", def.Gap, "max silence inside a phrase (seconds)")
	max := fs.Float64("max", def.Max, "max clip length (seconds)")
	pad := fs.Float64("pad", 0.3, "padding added before and after each clip (seconds)")
	out := fs.String("o", "candidates", "output directory")
	if fs.Parse(args) != nil || fs.NArg() != 2 {
		fmt.Fprintln(stderr, "usage: push-it clips cut [flags] SOURCE.(mp3|wav) transcript.json")
		return 2
	}
	src, err := player.Decode(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "push-it: %v\n", err)
		return 1
	}
	data, err := os.ReadFile(fs.Arg(1))
	if err != nil {
		fmt.Fprintf(stderr, "push-it: %v\n", err)
		return 1
	}
	var words []clipper.Word
	if err := json.Unmarshal(data, &words); err != nil {
		fmt.Fprintf(stderr, "push-it: transcript: %v\n", err)
		return 1
	}
	phrases := clipper.Group(words, clipper.Options{Phrase: splitList(*phrase), Allow: splitList(*allow), Gap: *gap, Max: *max})
	written, err := clipper.Cut(src, phrases, *pad, *out)
	if err != nil {
		fmt.Fprintf(stderr, "push-it: %v\n", err)
		return 1
	}
	for _, p := range written {
		fmt.Fprintf(stdout, "  [%02d] %6.2fs..%6.2fs  %s\n", p.ID, p.Start, p.End, p.File)
	}
	fmt.Fprintf(stdout, "wrote %d candidates to %s\n", len(written), *out)
	return 0
}

func cmdClipsReview(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("clips review", flag.ContinueOnError)
	fs.SetOutput(stderr)
	keepTo := fs.String("keep-to", "", "directory keepers are moved into (default: your configured clips dir)")
	if fs.Parse(args) != nil || fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: push-it clips review [--keep-to DIR] candidates/")
		return 2
	}
	cfg, ok := loadConfig(stderr)
	if !ok {
		return 1
	}
	dest := *keepTo
	if dest == "" {
		dest = cfg.Sound.ClipsDir
	}
	play := func(c *player.Clip) error { return player.Play(contextBackground(), c, cfg.Sound.Volume) }
	kept, err := clipper.Review(stdin, stdout, play, fs.Arg(0), dest)
	if err != nil {
		fmt.Fprintf(stderr, "push-it: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "\nkept %d clip(s) in %s\n", kept, dest)
	return 0
}
