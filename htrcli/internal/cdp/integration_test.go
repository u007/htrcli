//go:build integration

package cdp

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/u007/htrcli/internal/api"
)

// Requires a real Chrome. Run: make htrcli-test-integration
// Uses port 9223 + a temp profile so it never collides with a dev browser.
func TestCDPEndToEnd(t *testing.T) {
	const port = 9223
	chrome, err := FindChrome(os.Getenv("HTRCLI_CHROME_PATH"))
	if err != nil {
		t.Skipf("no Chrome available: %v", err)
	}
	t.Setenv("HOME", t.TempDir()) // fresh profile + state file

	st, err := StartBrowser(chrome, port, true /* headless */)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() {
		if err := StopBrowser(); err != nil {
			t.Logf("stop: %v", err)
		}
		// Force-kill the Chrome process group so it releases its profile
		// Cache handles before the test harness removes the temp HOME.
		if st != nil {
			if p, kerr := os.FindProcess(st.PID); kerr == nil {
				_ = p.Kill()
			}
			// Best-effort: also clear any lingering chrome on this port.
			_ = exec.Command("pkill", "-f", "remote-debugging-port=9223").Run()
		}
		time.Sleep(500 * time.Millisecond)
	})
	if st.Port != port {
		t.Fatalf("state port %d", st.Port)
	}

	// Discovery.
	targets, err := ListTargets(port)
	if err != nil || len(targets) == 0 {
		t.Fatalf("targets: %v %v", targets, err)
	}
	s, err := Dial(targets[0].WebSocketDebuggerURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer s.Close()

	// React-controlled input: value set via native setter must survive a
	// re-render (the framework must SEE the input event). Inline scripts do
	// not run on data:/about:blank origins, so inject the DOM + listener via
	// Runtime.evaluate instead.
	if err := Navigate(s, "about:blank", 10000); err != nil {
		t.Fatalf("navigate: %v", err)
	}
	inject := `document.body.innerHTML = '<input id="email"><span id="echo"></span>';` +
		`var el = document.getElementById('email');` +
		`el.addEventListener('input', function(e){ document.getElementById('echo').textContent = e.target.value; });`
	if _, err := Evaluate(s, inject); err != nil {
		t.Fatalf("inject DOM: %v", err)
	}
	result, err := ExecDOM(s, api.Command{ID: "1", Action: "fill",
		Target: &api.TargetSelector{Selector: "#email"}, Value: "james@mercstudio.com"})
	if err != nil || !result.Success {
		t.Fatalf("fill: %v %+v", err, result)
	}
	echo, err := Evaluate(s, "document.getElementById('echo').textContent")
	if err != nil || !strings.Contains(string(echo), "james@mercstudio.com") {
		t.Fatalf("controlled-input state not updated: %s (%v)", echo, err)
	}

	// Screenshot (headless = the guaranteed hidden mode).
	png, err := Screenshot(s)
	if err != nil || len(png) < 1000 {
		t.Fatalf("screenshot: %d bytes, %v", len(png), err)
	}
}
