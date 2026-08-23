//go:build linux

package player

import "github.com/jfreymuth/pulse"

// Probe reports whether a PulseAudio/PipeWire server is reachable. It opens
// and immediately closes a client; no stream is created.
func Probe() error {
	c, err := pulse.NewClient(pulse.ClientApplicationName("push-it doctor"))
	if err != nil {
		return err
	}
	c.Close()
	return nil
}
