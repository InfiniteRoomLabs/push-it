package player

import (
	"io"

	"github.com/hajimehoshi/go-mp3"
)

// DecodeMP3 decodes an MP3 stream. go-mp3 always yields 16-bit stereo.
func DecodeMP3(r io.Reader) (*Clip, error) {
	d, err := mp3.NewDecoder(r)
	if err != nil {
		return nil, err
	}
	pcm, err := io.ReadAll(d)
	if err != nil {
		return nil, err
	}
	return &Clip{PCM: pcm, SampleRate: d.SampleRate(), Channels: 2}, nil
}
