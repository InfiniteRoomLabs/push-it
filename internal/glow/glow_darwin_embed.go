//go:build darwin && glowhelper

package glow

import _ "embed"

//go:embed macos/bin/glow-macos
var helperBinary []byte

func init() { HelperEmbedded = true }
