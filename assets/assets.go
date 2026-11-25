// Package assets embeds the application icons into the binary.
package assets

import _ "embed"

// AppIcon is the 512x512 application icon (window / taskbar / packaging).
//
//go:embed icon.png
var AppIcon []byte

// TrayIcon is the 128x128 base icon for the system tray.
// The unread badge is drawn on top of it at runtime (see internal/ui/badge.go).
//
//go:embed tray.png
var TrayIcon []byte
