package host_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/u007/htcli/internal/host"
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
	d.RegisterTab(1, host.TabInfo{ID: 1, URL: "https://a.com", Title: "A", Active: true})
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
