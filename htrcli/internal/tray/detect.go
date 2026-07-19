package tray

import (
	"os"
	"runtime"
)

// ShouldStart reports whether the tray should be launched.
func ShouldStart(noTray bool) bool {
	return ShouldStartFor(runtime.GOOS, noTray)
}

// ShouldStartFor is the testable core: same logic as ShouldStart but
// takes the GOOS as a parameter so tests can pin it.
//
// Rules:
//   - Opt-out (noTray or HTRCLI_NO_TRAY) wins on every platform.
//   - macOS and Windows always have a native system tray.
//   - Linux requires a display (DISPLAY and/or WAYLAND_DISPLAY) AND no
//     active SSH session (SSH_CONNECTION or SSH_TTY).
//   - Any other GOOS never gets a tray.
func ShouldStartFor(goos string, noTray bool) bool {
	// Opt-out wins on every platform.
	if noTray {
		return false
	}
	if os.Getenv("HTRCLI_NO_TRAY") != "" {
		return false
	}

	// macOS and Windows have native system trays; trust the platform.
	if goos == "darwin" || goos == "windows" {
		return true
	}

	// Linux: require a display, AND no active SSH session.
	if goos == "linux" {
		if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
			return false
		}
		if os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_TTY") != "" {
			return false
		}
		return true
	}

	// Unsupported platform (freebsd, openbsd, js, …).
	return false
}
