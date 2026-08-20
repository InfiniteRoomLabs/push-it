// Package glow draws an animated rainbow frame around the screen.
//
// The rendering is platform-specific and lives in build-tagged files that
// overwrite Run/Install/Uninstall in init(). The parameters below are the
// single source of truth; the JS and Swift renderers mirror them verbatim.
package glow

import (
	"context"
	"time"

	"github.com/InfiniteRoomLabs/push-it/internal/config"
)

const (
	FrameThickness  = 14                     // px
	RotationPeriod  = 2 * time.Second        // one full trip of the rainbow around the frame
	PulsePeriod     = 600 * time.Millisecond // opacity pulse
	MinOpacity      = 0.55
	MaxOpacity      = 1.0
	DefaultDuration = 3500 * time.Millisecond
)

// Backend names the compiled-in renderer: "none", "gnome", "macos", or "windows".
var Backend = "none"

// Run shows the glow for d, blocking until it ends or ctx is cancelled.
var Run = func(ctx context.Context, d time.Duration) error { return nil }

// Install puts any platform pieces in place (GNOME extension, macOS helper).
var Install = func(st *config.InstallState) error { return nil }

// Uninstall reverses Install.
var Uninstall = func(st *config.InstallState) error { return nil }

// Available reports whether a real backend is compiled in.
func Available() bool { return Backend != "none" }
