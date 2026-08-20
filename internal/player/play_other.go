//go:build !linux && !darwin && !windows

package player

import (
	"context"
	"errors"
)

func play(_ context.Context, _ *Clip, _ float64) error {
	return errors.New("player: no audio backend for this platform")
}
