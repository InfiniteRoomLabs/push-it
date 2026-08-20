package clipper

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/InfiniteRoomLabs/push-it/internal/player"
)

// Review plays each candidate and asks keep/skip/replay/quit. Keepers are
// moved into keepTo. It returns how many were kept.
func Review(in io.Reader, out io.Writer, play func(*player.Clip) error, candDir, keepTo string) (int, error) {
	data, err := os.ReadFile(filepath.Join(candDir, "candidates.json"))
	if err != nil {
		return 0, err
	}
	var phrases []Phrase
	if err := json.Unmarshal(data, &phrases); err != nil {
		return 0, err
	}
	if err := os.MkdirAll(keepTo, 0o755); err != nil {
		return 0, err
	}
	sc := bufio.NewScanner(in)
	kept := 0
	for i, p := range phrases {
		src := filepath.Join(candDir, p.File)
		if _, err := os.Stat(src); errors.Is(err, fs.ErrNotExist) {
			fmt.Fprintf(out, "  [%d/%d] %s - already reviewed, skipping\n", i+1, len(phrases), p.Label)
			continue
		}
		clip, err := player.Decode(src)
		if err != nil {
			return kept, err
		}
		fmt.Fprintf(out, "\n[%d/%d] %.2fs..%.2fs  %q\n", i+1, len(phrases), p.Start, p.End, p.Label)
		if err := play(clip); err != nil {
			fmt.Fprintf(out, "  (play failed: %v)\n", err)
		}
		for {
			fmt.Fprint(out, "  [k]eep / [s]kip / [r]eplay / [q]uit > ")
			if !sc.Scan() {
				return kept, sc.Err()
			}
			switch strings.ToLower(strings.TrimSpace(sc.Text())) {
			case "k":
				if err := os.Rename(src, filepath.Join(keepTo, p.File)); err != nil {
					return kept, err
				}
				kept++
			case "s":
			case "r":
				if err := play(clip); err != nil {
					fmt.Fprintf(out, "  (play failed: %v)\n", err)
				}
				continue
			case "q":
				return kept, nil
			default:
				fmt.Fprintln(out, "  ? enter k, s, r, or q")
				continue
			}
			break
		}
	}
	return kept, nil
}
