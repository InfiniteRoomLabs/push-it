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
				if err := moveFile(src, filepath.Join(keepTo, p.File)); err != nil {
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

// moveFile moves src to dst, falling back to a copy when the rename fails
// (os.Rename returns EXDEV when src and dst are on different filesystems -
// candidates/ and keepTo commonly are not).
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	return copyThenRemove(src, dst)
}

// copyThenRemove copies src to dst then removes src. It is moveFile's
// cross-filesystem fallback.
func copyThenRemove(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		_ = in.Close()
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = in.Close()
		_ = os.Remove(dst)
		return err
	}
	if err := out.Close(); err != nil {
		_ = in.Close()
		_ = os.Remove(dst)
		return err
	}
	// Close the source before removing it: Windows refuses to delete an open file.
	if err := in.Close(); err != nil {
		return err
	}
	return os.Remove(src)
}
