//go:build darwin && !glowhelper

package glow

// helperBinary is empty in builds made without -tags glowhelper (for example
// a plain `go build` on a machine that never ran internal/glow/macos/build.sh).
var helperBinary []byte
