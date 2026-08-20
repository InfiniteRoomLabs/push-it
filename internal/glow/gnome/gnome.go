// Package gnome embeds the push-it GNOME Shell extension.
package gnome

import "embed"

const (
	UUID       = "pushit-glow@infiniteroomlabs.com"
	BusName    = "org.gnome.Shell"
	ObjectPath = "/com/infiniteroomlabs/PushItGlow"
	Interface  = "com.infiniteroomlabs.PushItGlow"
)

// FS holds the extension sources under "ext/".
//
//go:embed ext
var FS embed.FS
