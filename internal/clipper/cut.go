package clipper

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/InfiniteRoomLabs/push-it/internal/player"
)

func fileName(p Phrase) string {
	return fmt.Sprintf("%03d-%s.wav", p.ID, strings.ReplaceAll(p.Label, " ", "-"))
}

func seconds(f float64) time.Duration { return time.Duration(f * float64(time.Second)) }

// Cut writes one WAV per phrase (padded by pad seconds on each side) into
// outDir plus candidates.json, and returns the phrases with File set.
func Cut(src *player.Clip, phrases []Phrase, pad float64, outDir string) ([]Phrase, error) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}
	out := make([]Phrase, 0, len(phrases))
	for _, p := range phrases {
		clip := src.Slice(seconds(p.Start-pad), seconds(p.End+pad))
		p.File = fileName(p)
		f, err := os.Create(filepath.Join(outDir, p.File))
		if err != nil {
			return nil, err
		}
		err = player.EncodeWAV(f, clip)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, err
	}
	return out, os.WriteFile(filepath.Join(outDir, "candidates.json"), append(data, '\n'), 0o644)
}
