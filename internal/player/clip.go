// Package player decodes MP3/WAV clips into 16-bit PCM and plays them.
package player

import "time"

// Clip is interleaved signed 16-bit little-endian PCM.
type Clip struct {
	PCM        []byte
	SampleRate int
	Channels   int
}

func (c *Clip) frameSize() int { return c.Channels * 2 }

// Duration is the playback length of the clip.
func (c *Clip) Duration() time.Duration {
	if c.SampleRate == 0 || c.Channels == 0 {
		return 0
	}
	frames := len(c.PCM) / c.frameSize()
	return time.Duration(frames) * time.Second / time.Duration(c.SampleRate)
}

// Slice returns the portion of the clip between from and to, clamped to the
// clip's bounds and aligned to whole frames. The PCM is copied.
func (c *Clip) Slice(from, to time.Duration) *Clip {
	fs := c.frameSize()
	if fs == 0 {
		return &Clip{PCM: nil, SampleRate: c.SampleRate, Channels: c.Channels}
	}
	total := len(c.PCM) / fs
	toFrame := func(d time.Duration) int {
		n := int(d.Seconds() * float64(c.SampleRate))
		if n < 0 {
			return 0
		}
		if n > total {
			return total
		}
		return n
	}
	a, b := toFrame(from), toFrame(to)
	if b < a {
		b = a
	}
	out := make([]byte, (b-a)*fs)
	copy(out, c.PCM[a*fs:b*fs])
	return &Clip{PCM: out, SampleRate: c.SampleRate, Channels: c.Channels}
}
