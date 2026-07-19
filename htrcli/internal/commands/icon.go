package commands

import _ "embed"

// trayIcon is the PNG shown in the system tray (embedded so a single binary
// has everything it needs). It is only used when the tray attaches.
//
//go:embed icon.png
var trayIcon []byte
