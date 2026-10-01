package host_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/u007/htrcli/internal/host"
)

// These tests go through host.NewHTTPServer and a real httptest listener, NOT
// through the handler function directly. A handler-only test would pass even if
// the route were never registered on the mux — which is precisely the mistake
// this file exists to prevent. TestBackgroundCommandRouteIsRegistered covers
// that directly: it asserts on the path, so renaming the route breaks it.

// bgRelay is a relay connection that records the commands it receives and
// answers them, so a request can be driven end to end.
type bgRelay struct {
	rc *host.RelayConn

	mu       sync.Mutex
	received []host.NativeMessage
}

func newBGRelay(d *host.Daemon) *bgRelay {
	r := &bgRelay{}
	r.rc = d.AddConn(func(msg []byte) error {
		var nm host.NativeMessage
		if err := json.Unmarshal(msg, &nm); err != nil {
			return err
		}
		r.mu.Lock()
		r.received = append(r.received, nm)
		r.mu.Unlock()

		// Answer so the HTTP request completes instead of timing out.
		var cmd host.Command
		if err := json.Unmarshal(nm.Payload, &cmd); err == nil {
			go d.ResolveCommand(cmd.ID, host.CommandResult{ID: cmd.ID, Success: true})
		}
		return nil
	})
	return r
}

func (r *bgRelay) commands() []host.NativeMessage {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]host.NativeMessage(nil), r.received...)
}

func (r *bgRelay) sawAction(action string) bool {
	for _, m := range r.commands() {
		if m.Type == "command" {
			var cmd host.Command
			if err := json.Unmarshal(m.Payload, &cmd); err == nil && cmd.Action == action {
				return true
			}
		}
	}
	return false
}

func postJSON(t *testing.T, url string, body any) (int, map[string]any) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	var decoded map[string]any
	json.NewDecoder(resp.Body).Decode(&decoded)
	return resp.StatusCode, decoded
}

// TestBackgroundCommandRouteIsRegistered is the route's own existence test. The
// mutation that proves it has teeth is renaming the path in server.go.
//
// A missing route and a handled request can share a status: the mux default
// branch answers 404 "not found", and the handler itself answers 404 when no
// relay is connected. So the discriminator is the error body, not the status.
func TestBackgroundCommandRouteIsRegistered(t *testing.T) {
	d := host.NewDaemon()
	ts := httptest.NewServer(host.NewHTTPServer(d, 0, "", nil).Handler)
	defer ts.Close()

	status, body := postJSON(t, ts.URL+"/api/background/command", map[string]any{
		"command": map[string]any{"action": "recordingList"},
	})

	if body["error"] == "not found" {
		t.Fatalf("route is not registered on the mux: %d %v", status, body)
	}
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for a missing relay", status)
	}
	if body["error"] != "no browser connected" {
		t.Errorf("error = %v, want %q (proves the handler ran, not the mux default)", body["error"], "no browser connected")
	}
}

// The core of the feature: a recording action with no tab still reaches a relay.
func TestBackgroundCommandReachesRelayWithoutATab(t *testing.T) {
	d := host.NewDaemon()
	relay := newBGRelay(d)
	ts := httptest.NewServer(host.NewHTTPServer(d, 0, "", nil).Handler)
	defer ts.Close()

	// Note: the relay has NO registered tabs. The point of this route is that it
	// does not need one.
	status, body := postJSON(t, ts.URL+"/api/background/command", map[string]any{
		"command": map[string]any{"action": "recordingList"},
	})

	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %v)", status, body)
	}
	if body["ok"] != true {
		t.Errorf("ok = %v, want true", body["ok"])
	}
	if !relay.sawAction("recordingList") {
		t.Error("the relay never received the recordingList command")
	}
}

// A relay with no tab must still be reachable; contrast with /api/tabs/N/command
// which requires one. Guards against someone reintroducing FirstTabID here.
func TestBackgroundCommandSucceedsWithZeroRegisteredTabs(t *testing.T) {
	d := host.NewDaemon()
	newBGRelay(d) // deliberately registers no tabs
	ts := httptest.NewServer(host.NewHTTPServer(d, 0, "", nil).Handler)
	defer ts.Close()

	if status, body := postJSON(t, ts.URL+"/api/background/command", map[string]any{
		"command": map[string]any{"action": "recordingStatus"},
	}); status != http.StatusOK {
		t.Errorf("status = %d, want 200 (body %v)", status, body)
	}
}

// The browser hint in the body must reach connection selection, so a caller can
// name the profile it wants instead of getting whatever connected first.
func TestBackgroundCommandForwardsBrowserHint(t *testing.T) {
	d := host.NewDaemon()
	chrome := newBGRelay(d)
	firefox := newBGRelay(d)
	d.SetConnBrowser(chrome.rc, "chrome")
	d.SetConnBrowser(firefox.rc, "firefox")

	ts := httptest.NewServer(host.NewHTTPServer(d, 0, "", nil).Handler)
	defer ts.Close()

	if status, body := postJSON(t, ts.URL+"/api/background/command", map[string]any{
		"command": map[string]any{"action": "recordingList"},
		"browser": "firefox",
	}); status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %v)", status, body)
	}

	if !firefox.sawAction("recordingList") {
		t.Error("the firefox relay did not receive the command")
	}
	if chrome.sawAction("recordingList") {
		t.Error("a firefox-hinted request was delivered to the chrome relay")
	}
}

func TestBackgroundCommandRejectsMissingAction(t *testing.T) {
	d := host.NewDaemon()
	newBGRelay(d)
	ts := httptest.NewServer(host.NewHTTPServer(d, 0, "", nil).Handler)
	defer ts.Close()

	for name, payload := range map[string]any{
		"no command":   map[string]any{},
		"no action":    map[string]any{"command": map[string]any{}},
		"empty action": map[string]any{"command": map[string]any{"action": ""}},
	} {
		status, body := postJSON(t, ts.URL+"/api/background/command", payload)
		if status != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400 (body %v)", name, status, body)
		}
	}
}

// The route lives under /api/, so it must be behind the same auth as every other
// API route. A new route added outside that prefix would silently skip the
// bearer-token check.
func TestBackgroundCommandRequiresAuth(t *testing.T) {
	d := host.NewDaemon()
	newBGRelay(d)
	srv := host.NewHTTPServer(d, 0, "secret-token", nil)
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	raw, err := json.Marshal(map[string]any{"command": map[string]any{"action": "recordingList"}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp, err := http.Post(ts.URL+"/api/background/command", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 without a bearer token", resp.StatusCode)
	}

	req, err := http.NewRequest("POST", ts.URL+"/api/background/command", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer secret-token")
	authed, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("authenticated POST: %v", err)
	}
	defer authed.Body.Close()
	if authed.StatusCode != http.StatusOK {
		t.Errorf("authenticated status = %d, want 200", authed.StatusCode)
	}
}

// The route must answer promptly when the extension never replies, rather than
// hanging until the client's own deadline.
func TestBackgroundCommandTimesOut(t *testing.T) {
	d := host.NewDaemon()
	// A relay that accepts the command but never answers it.
	d.AddConn(func(_ []byte) error { return nil })

	ts := httptest.NewServer(host.NewHTTPServer(d, 0, "", nil).Handler)
	defer ts.Close()

	start := time.Now()
	status, body := postJSON(t, ts.URL+"/api/background/command", map[string]any{
		"command": map[string]any{"action": "recordingList"},
		"timeout": 60, // milliseconds
	})
	elapsed := time.Since(start)

	if status == http.StatusOK {
		t.Errorf("status = 200, want a timeout error (body %v)", body)
	}
	if elapsed > 5*time.Second {
		t.Errorf("took %v, want the request to return promptly", elapsed)
	}
}
