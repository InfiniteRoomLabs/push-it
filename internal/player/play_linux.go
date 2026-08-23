//go:build linux

package player

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/jfreymuth/pulse"
)

func scaleSample(v int16, volume float64) int16 {
	scaled := float64(v) * volume
	if scaled > 32767 {
		return 32767
	}
	if scaled < -32768 {
		return -32768
	}
	return int16(scaled)
}

func play(ctx context.Context, c *Clip, volume float64) error {
	if c.Channels != 1 && c.Channels != 2 {
		return fmt.Errorf("player: unsupported channel count %d (need 1 or 2)", c.Channels)
	}

	client, err := pulse.NewClient(pulse.ClientApplicationName("push-it"))
	if err != nil {
		return fmt.Errorf("player: no PulseAudio/PipeWire server: %w", err)
	}
	defer client.Close()

	total := len(c.PCM) / 2     // int16 samples, interleaved across channels
	total -= total % c.Channels // drop a trailing partial frame rather than hand pulse a split sample
	pos := 0
	reader := pulse.Int16Reader(func(buf []int16) (int, error) {
		if pos >= total {
			return 0, pulse.EndOfData
		}
		n := 0
		for n < len(buf) && pos < total {
			v := int16(binary.LittleEndian.Uint16(c.PCM[pos*2:]))
			buf[n] = scaleSample(v, volume)
			pos++
			n++
		}
		return n, nil
	})

	opts := []pulse.PlaybackOption{pulse.PlaybackSampleRate(c.SampleRate)}
	if c.Channels == 1 {
		opts = append(opts, pulse.PlaybackMono)
	} else {
		opts = append(opts, pulse.PlaybackStereo)
	}

	stream, err := client.NewPlayback(reader, opts...)
	if err != nil {
		return fmt.Errorf("player: no PulseAudio/PipeWire server: %w", err)
	}
	defer stream.Close()

	// The library's own completion signals are unusable against PipeWire's
	// pulse server once the reader hits EndOfData - which happens immediately
	// for any clip that fits the server's multi-second buffer target:
	//
	//   - Start() blocks forever on <-p.started: the stream goes idle when
	//     the reader is exhausted, and the client then drops the server's
	//     Started event on its state guard. Stop() cannot unblock it, so
	//     hook processes used to hang for days (observed symptom: garbled,
	//     truncated audio - typically a fragment of the clip's tail - and an
	//     immortal detached child per push).
	//   - Drain() is a no-op in the same situation.
	//
	// So Start() runs in a goroutine and the wall clock - clip duration plus
	// a small tail margin - is the completion signal. The goroutine may stay
	// parked in Start() until the process exits; the hook child and the play
	// CLI are short-lived, and `clips review` leaks one parked goroutine per
	// previewed candidate for the life of the session - accepted. The recover
	// guards the library's close(p.request): stream.Close() below can close
	// that channel while Start() is still trying to send on it.
	go func() {
		defer func() { _ = recover() }()
		stream.Start()
	}()
	select {
	case <-ctx.Done():
	case <-time.After(c.Duration() + 250*time.Millisecond):
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	return stream.Error()
}
