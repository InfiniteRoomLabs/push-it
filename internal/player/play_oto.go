//go:build darwin || windows

package player

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
)

var (
	otoMu   sync.Mutex
	otoCtx  *oto.Context
	otoRate int
	otoCh   int
)

func audioContext(c *Clip) (*oto.Context, error) {
	otoMu.Lock()
	defer otoMu.Unlock()
	if otoCtx != nil {
		if otoRate != c.SampleRate || otoCh != c.Channels {
			return nil, fmt.Errorf("play: clip is %d Hz/%d ch but audio was opened at %d Hz/%d ch", c.SampleRate, c.Channels, otoRate, otoCh)
		}
		return otoCtx, nil
	}
	ctx, ready, err := oto.NewContext(&oto.NewContextOptions{
		SampleRate:   c.SampleRate,
		ChannelCount: c.Channels,
		Format:       oto.FormatSignedInt16LE,
	})
	if err != nil {
		return nil, err
	}
	<-ready
	otoCtx, otoRate, otoCh = ctx, c.SampleRate, c.Channels
	return ctx, nil
}

func play(ctx context.Context, c *Clip, volume float64) error {
	octx, err := audioContext(c)
	if err != nil {
		return err
	}
	p := octx.NewPlayer(bytes.NewReader(c.PCM))
	p.SetVolume(volume)
	p.Play()
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for p.IsPlaying() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
	return nil
}
