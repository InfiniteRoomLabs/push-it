//go:build !linux && !darwin && !windows

package player

func Probe() error { return ErrNotProbed }
