// Package clips finds and picks sound clips.
package clips

import (
	"errors"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ErrNoClips is returned by Pick when there is nothing to choose from.
var ErrNoClips = errors.New("no clips found")

// List returns the sorted absolute paths of *.mp3 and *.wav files in dir.
// A missing directory yields an empty list, not an error.
func List(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		switch strings.ToLower(filepath.Ext(e.Name())) {
		case ".mp3", ".wav":
			p, err := filepath.Abs(filepath.Join(dir, e.Name()))
			if err != nil {
				return nil, err
			}
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out, nil
}

// Pick returns one file chosen uniformly at random.
func Pick(files []string, r *rand.Rand) (string, error) {
	if len(files) == 0 {
		return "", ErrNoClips
	}
	return files[r.IntN(len(files))], nil
}
