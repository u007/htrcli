package host_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/u007/htrcli/internal/host"
)

func TestHealthEndpoint(t *testing.T) {
	d := host.NewDaemon()
	srv := host.NewHTTPServer(d, 0, "", nil)
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/health")
	if err != nil {
		t.Fatalf("GET /api/health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	if body["ok"] != true {
		t.Errorf("body.ok = %v, want true", body["ok"])
	}
}

func TestTabsEndpoint(t *testing.T) {
	d := host.NewDaemon()
	rc := d.AddConn(func(_ []byte) error { return nil })
	d.RegisterTab(rc, 1, host.TabInfo{ID: 1, URL: "https://a.com", Title: "A", Active: true})
	srv := host.NewHTTPServer(d, 0, "", nil)
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/tabs")
	if err != nil {
		t.Fatalf("GET /api/tabs: %v", err)
	}
	defer resp.Body.Close()

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	tabs := body["data"].([]interface{})
	if len(tabs) != 1 {
		t.Errorf("want 1 tab, got %d", len(tabs))
	}
}

func TestBearerTokenEnforced(t *testing.T) {
	d := host.NewDaemon()
	srv := host.NewHTTPServer(d, 0, "secret-token", nil)
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	// No token → 401
	resp, _ := http.Get(ts.URL + "/api/health")
	if resp.StatusCode != 401 {
		t.Errorf("no token: status = %d, want 401", resp.StatusCode)
	}

	// Wrong token → 401
	req, _ := http.NewRequest("GET", ts.URL+"/api/health", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != 401 {
		t.Errorf("wrong token: status = %d, want 401", resp.StatusCode)
	}

	// Correct token → 200
	req, _ = http.NewRequest("GET", ts.URL+"/api/health", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != 200 {
		t.Errorf("correct token: status = %d, want 200", resp.StatusCode)
	}
}

func TestEventsIngestAndRead(t *testing.T) {
	d := host.NewDaemon()
	// Give the daemon a tab so /api/events defaults cleanly if needed.
	rc := d.AddConn(func(_ []byte) error { return nil })
	d.RegisterTab(rc, 1, host.TabInfo{ID: 1, URL: "https://a.com", Title: "A", Active: true})
	srv := host.NewHTTPServer(d, 0, "", nil)
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	ingestBody := `{"tabId":1,"kind":"console","entries":[{"kind":"console","timestamp":1000,"data":{"level":"log","args":["hi"]}}]}`
	resp, err := http.Post(ts.URL+"/api/events/ingest", "application/json", strings.NewReader(ingestBody))
	if err != nil {
		t.Fatalf("POST /api/events/ingest: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("ingest status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	resp2, err := http.Get(ts.URL + "/api/events?kind=console&tab=1&since=0")
	if err != nil {
		t.Fatalf("GET /api/events: %v", err)
	}
	defer resp2.Body.Close()
	var body map[string]interface{}
	if err := json.NewDecoder(resp2.Body).Decode(&body); err != nil {
		t.Fatalf("decode events body: %v", err)
	}
	data := body["data"].(map[string]interface{})
	entries := data["entries"].([]interface{})
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
}

func TestEventsGenerationEndpoint(t *testing.T) {
	d := host.NewDaemon()
	srv := host.NewHTTPServer(d, 0, "", nil)
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/events/generation")
	if err != nil {
		t.Fatalf("GET /api/events/generation: %v", err)
	}
	defer resp.Body.Close()
	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode generation body: %v", err)
	}
	if body["ok"] != true {
		t.Fatalf("body.ok = %v, want true", body["ok"])
	}
	if _, ok := body["data"].(map[string]interface{})["generation"]; !ok {
		t.Fatalf("expected data.generation in response, got %+v", body)
	}
}

func TestScreenshotRoundTrip(t *testing.T) {
	d := host.NewDaemon()

	// Capture the command ID from the capture_screenshot trigger the daemon
	// sends over the relay, so we can correlate the simulated upload.
	triggered := make(chan string, 1)
	rc := d.AddConn(func(msg []byte) error {
		var m struct {
			Type      string `json:"type"`
			CommandID string `json:"commandId"`
		}
		json.Unmarshal(msg, &m)
		if m.Type == "capture_screenshot" {
			triggered <- m.CommandID
		}
		return nil
	})
	d.RegisterTab(rc, 1, host.TabInfo{ID: 1, URL: "https://a.com", Active: true})

	srv := host.NewHTTPServer(d, 0, "", nil)
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	// GET blocks until the extension uploads — run it concurrently.
	done := make(chan *http.Response, 1)
	go func() {
		resp, err := http.Get(ts.URL + "/api/screenshot")
		if err != nil {
			t.Errorf("GET /api/screenshot: %v", err)
			done <- nil
			return
		}
		done <- resp
	}()

	cmdID := <-triggered
	// Simulate the extension POSTing the PNG back (with a data-URL prefix).
	upload := `{"commandId":"` + cmdID + `","data":"data:image/png;base64,SGVsbG8="}`
	pr, err := http.Post(ts.URL+"/api/screenshot", "application/json", strings.NewReader(upload))
	if err != nil {
		t.Fatalf("POST /api/screenshot: %v", err)
	}
	pr.Body.Close()

	resp := <-done
	if resp == nil {
		t.Fatal("GET failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("GET status = %d, want 200", resp.StatusCode)
	}
	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	// Prefix must be stripped: htrcli base64-decodes this value directly.
	if body["data"] != "SGVsbG8=" {
		t.Errorf("data = %v, want stripped base64 \"SGVsbG8=\"", body["data"])
	}
}

func TestScreenshotNoTabs(t *testing.T) {
	d := host.NewDaemon()
	srv := host.NewHTTPServer(d, 0, "", nil)
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/screenshot")
	if err != nil {
		t.Fatalf("GET /api/screenshot: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("status = %d, want 404 (no tabs)", resp.StatusCode)
	}
}

func TestCommandEndpointNoTabs(t *testing.T) {
	d := host.NewDaemon()
	srv := host.NewHTTPServer(d, 0, "", nil)
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	body := `{"command":{"id":"c1","action":"navigate","value":"https://x.com"}}`
	resp, err := http.Post(ts.URL+"/api/command", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/command: %v", err)
	}
	if resp.StatusCode != 404 {
		t.Errorf("status = %d, want 404 (no tabs)", resp.StatusCode)
	}
}
