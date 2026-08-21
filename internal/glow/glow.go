// Package glow draws a rainbow glow that fades inward from every screen
// edge and travels counter-clockwise.
//
// The rendering is platform-specific and lives in build-tagged files that
// overwrite Run/Install/Uninstall in init(). The animation parameters live
// in package paint; the JS and Swift renderers mirror them verbatim.
package glow

import (
	"context"
	"time"

	"github.com/InfiniteRoomLabs/push-it/internal/config"
	"github.com/InfiniteRoomLabs/push-it/internal/glow/paint"
)

// The animation parameters are defined in package paint (the leaf package,
// so that platform backends in this package can use the renderer) and
// re-exported here under the same names.
const (
	GlowWidthAt1080 = paint.GlowWidthAt1080
	FalloffExponent = paint.FalloffExponent
	RotationPeriod  = paint.RotationPeriod
	PulsePeriod     = paint.PulsePeriod
	MinOpacity      = paint.MinOpacity
	MaxOpacity      = paint.MaxOpacity
	DefaultDuration = 3500 * time.Millisecond
)

// Backend names the compiled-in renderer: "none", "gnome", "macos", or "windows".
var Backend = "none"

// Run shows the glow for d, blocking until it ends or ctx is cancelled.
var Run = func(ctx context.Context, d time.Duration) error { return nil }

// Install puts any platform pieces in place (GNOME extension, macOS helper).
// It returns a user-facing note (for example "log out and back in") that the
// installer prints when non-empty. A nil st is an error, never a panic.
var Install = func(st *config.InstallState) (string, error) { return "", nil }

// Uninstall reverses Install.
var Uninstall = func(st *config.InstallState) error { return nil }

// Available reports whether a real backend is compiled in.
func Available() bool { return Backend != "none" }
