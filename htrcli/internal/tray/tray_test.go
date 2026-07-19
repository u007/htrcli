//go:build traytest

package tray

import (
	"strings"
	"testing"
	"time"
)

func containsStr(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// TestTrayDispatch exercises the real menu build + dispatch loop against the
// in-memory fake backend: clicking a maintenance item invokes the controller,
// and clicking Quit cleanly exits Run.
func TestTrayDispatch(t *testing.T) {
	fc := &fakeController{
		status: Status{Port: 3845, RelaysConnected: 2},
		log:    []string{"line1", "line2", "line3"},
	}
	// Quit from the menu must unblock Run; wire the fake's quit callback to
	// the backend's Quit (mirrors serve.go's SetQuitFn → SIGTERM → tray.Quit).
	fc.quitFn = func() { Quit() }

	done := make(chan struct{})
	go func() {
		Run(fc, []byte("fake-icon-bytes"))
		close(done)
	}()

	// Let onReady build the menu.
	time.Sleep(50 * time.Millisecond)

	// Maintenance → Reinstall (Chrome)
	if !ClickTitle("Reinstall (Chrome)") {
		t.Fatal("could not click Reinstall (Chrome)")
	}
	time.Sleep(50 * time.Millisecond)
	if !containsStr(fc.invoked, "ReinstallHost:chrome") {
		t.Fatalf("ReinstallHost:chrome not invoked, got %v", fc.invoked)
	}

	// Maintenance → Copy bearer token (records an invocation).
	if !ClickTitle("Copy bearer token") {
		t.Fatal("could not click Copy bearer token")
	}
	time.Sleep(50 * time.Millisecond)
	if !containsStr(fc.invoked, "CopyTokenToClipboard") {
		t.Fatalf("CopyTokenToClipboard not invoked, got %v", fc.invoked)
	}

	// Quit → must unblock Run.
	if !ClickTitle("Quit") {
		t.Fatal("could not click Quit")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("tray did not exit after Quit")
	}
	if !containsStr(fc.invoked, "Quit") {
		t.Fatalf("Quit not invoked, got %v", fc.invoked)
	}
}

// TestTrayTitleRefresh verifies the refresh loop paints the status line.
func TestTrayTitleRefresh(t *testing.T) {
	fc := &fakeController{
		status: Status{Port: 3845, RelaysConnected: 1, LastError: "boom"},
	}
	fc.quitFn = func() { Quit() }

	done := make(chan struct{})
	go func() {
		Run(fc, nil)
		close(done)
	}()

	// Refresh runs every 50ms under traytest; give it a couple of ticks.
	time.Sleep(150 * time.Millisecond)

	fb, ok := backend.(*fakeBackend)
	if !ok {
		t.Fatal("expected fakeBackend")
	}
	fb.mu.Lock()
	var statusTitle string
	for _, it := range fb.items {
		if strings.HasPrefix(it.title, "Status:") {
			statusTitle = it.title
		}
	}
	fb.mu.Unlock()

	if !strings.Contains(statusTitle, "3845") || !strings.Contains(statusTitle, "error") {
		t.Fatalf("status line not refreshed correctly: %q", statusTitle)
	}

	Quit()
	<-done
}
