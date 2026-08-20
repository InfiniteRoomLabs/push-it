//go:build linux

package player

import (
	"context"
	"encoding/binary"
	"fmt"

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

	total := len(c.PCM) / 2 // int16 samples, interleaved across channels
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

	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			stream.Stop()
		case <-done:
		}
	}()

	stream.Start()
	stream.Drain()
	close(done)

	if err := ctx.Err(); err != nil {
		return err
	}
	return stream.Error()
}
