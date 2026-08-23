package player

import (
	"context"
	"errors"
)

// Play plays the clip at the given volume (0..1) and blocks until it ends
// or ctx is cancelled. Playback is backed by oto on macOS/Windows and by a
// pure-Go PulseAudio/PipeWire client on Linux; see play_oto.go, play_linux.go,
// and play_other.go for the platform-specific implementations.
func Play(ctx context.Context, c *Clip, volume float64) error {
	return play(ctx, c, volume)
}

// ErrNotProbed marks platforms where probing would initialize the audio
// device (oto contexts are process-global); doctor reports "not probed".
var ErrNotProbed = errors.New("player: audio not probed on this platform")
