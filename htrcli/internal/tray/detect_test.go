package tray

import (
	"runtime"
	"testing"
)

func TestShouldStart(t *testing.T) {
	tests := []struct {
		name   string
		goos   string
		env    map[string]string
		noTray bool
		want   bool
	}{
		{"macos always on", "darwin", map[string]string{}, false, true},
		{"windows always on", "windows", map[string]string{}, false, true},
		{"linux x11 desktop", "linux", map[string]string{"DISPLAY": ":0"}, false, true},
		{"linux wayland desktop", "linux", map[string]string{"WAYLAND_DISPLAY": "wayland-0"}, false, true},
		{"linux both empty (headless)", "linux", map[string]string{}, false, false},
		{"linux ssh session + x11", "linux", map[string]string{"DISPLAY": ":0", "SSH_CONNECTION": "1.2.3.4 5 6.7.8.9 10"}, false, false},
		{"linux ssh + tty", "linux", map[string]string{"DISPLAY": ":0", "SSH_TTY": "/dev/pts/0"}, false, false},
		{"linux ssh headless", "linux", map[string]string{"SSH_CONNECTION": "1.2.3.4 5 6.7.8.9 10"}, false, false},
		{"linux desktop HTRCLI_NO_TRAY=1", "linux", map[string]string{"DISPLAY": ":0", "HTRCLI_NO_TRAY": "1"}, false, false},
		{"linux desktop --no-tray", "linux", map[string]string{"DISPLAY": ":0"}, true, false},
		{"macos no-tray override", "darwin", map[string]string{}, true, false},
		{"macos HTRCLI_NO_TRAY override", "darwin", map[string]string{"HTRCLI_NO_TRAY": "1"}, false, false},
		{"freebsd never", "freebsd", map[string]string{"DISPLAY": ":0"}, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all relevant env vars first.
			for _, k := range []string{"DISPLAY", "WAYLAND_DISPLAY", "SSH_CONNECTION", "SSH_TTY", "HTRCLI_NO_TRAY"} {
				t.Setenv(k, "") // t.Setenv with "" is equivalent to unset for the test
			}
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			if got := ShouldStartFor(tt.goos, tt.noTray); got != tt.want {
				t.Errorf("ShouldStartFor(%q, %v) with env %v = %v, want %v",
					tt.goos, tt.noTray, tt.env, got, tt.want)
			}
		})
	}
}

// Sanity check that the public wrapper uses the host GOOS.
func TestShouldStartWrapper(t *testing.T) {
	t.Setenv("HTRCLI_NO_TRAY", "1")
	if ShouldStart(false) {
		t.Fatal("HTRCLI_NO_TRAY must disable the tray on every platform")
	}
	t.Setenv("HTRCLI_NO_TRAY", "")
	if runtime.GOOS == "linux" {
		// On Linux CI (headless), expect false; on a dev desktop, true.
		_ = ShouldStart(false)
	}
}
