package player

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Decode opens path and decodes it according to its extension.
func Decode(path string) (*Clip, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	switch strings.ToLower(filepath.Ext(path)) {
	case ".wav":
		return DecodeWAV(f)
	case ".mp3":
		return DecodeMP3(f)
	default:
		return nil, fmt.Errorf("decode: unsupported file type %q (need .mp3 or .wav)", filepath.Ext(path))
	}
}
