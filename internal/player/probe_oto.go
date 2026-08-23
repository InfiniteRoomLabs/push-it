//go:build darwin || windows

package player

func Probe() error { return ErrNotProbed }
