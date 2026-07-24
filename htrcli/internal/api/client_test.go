package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"testing"
)

func TestNewClient(t *testing.T) {
	c := NewClient("http://localhost:3845", "test-token")
	if c.BaseURL != "http://localhost:3845" {
		t.Errorf("expected BaseURL http://localhost:3845, got %s", c.BaseURL)
	}
	if c.Token != "test-token" {
		t.Errorf("expected Token test-token, got %s", c.Token)
	}
}

func TestGetHealth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/health" {
			t.Errorf("expected path /api/health, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("expected Authorization header, got %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ApiResponse{
			OK: true,
			Data: HealthResponse{
				Status:        "running",
				ConnectedTabs: 2,
				Uptime:        123.45,
			},
		})
	}))
	defer server.Close()

	c := NewClient(server.URL, "test-token")
	health, err := c.GetHealth()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if health.Status != "running" {
		t.Errorf("expected status running, got %s", health.Status)
	}
	if health.ConnectedTabs != 2 {
		t.Errorf("expected 2 connected tabs, got %d", health.ConnectedTabs)
	}
}

func TestListTabs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ApiResponse{
			OK: true,
			Data: []TabInfo{
				{ID: 1, URL: "https://example.com", Title: "Example", Active: true},
				{ID: 2, URL: "https://google.com", Title: "Google", Active: false},
			},
		})
	}))
	defer server.Close()

	c := NewClient(server.URL, "")
	tabs, err := c.ListTabs()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tabs) != 2 {
		t.Fatalf("expected 2 tabs, got %d", len(tabs))
	}
	if tabs[0].URL != "https://example.com" {
		t.Errorf("expected first tab URL https://example.com, got %s", tabs[0].URL)
	}
}

func TestPostEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/events/ingest" {
			t.Errorf("expected path /api/events/ingest, got %s", r.URL.Path)
		}
		var req IngestEventsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.TabID != 1 || req.Kind != "console" || len(req.Entries) != 1 {
			t.Fatalf("unexpected request body: %+v", req)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ApiResponse{OK: true, Data: map[string]any{"received": true}})
	}))
	defer server.Close()

	c := NewClient(server.URL, "")
	err := c.PostEvents(1, "console", []EventEntry{{
		Seq:       1,
		Kind:      "console",
		Timestamp: 1000,
		Data:      json.RawMessage(`{"level":"log","args":["hi"]}`),
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("since") != "40" || r.URL.Query().Get("kind") != "console" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ApiResponse{
			OK: true,
			Data: EventsResponse{
				Entries: []EventEntry{{
					Seq:       41,
					Kind:      "console",
					Timestamp: 2000,
					Data:      json.RawMessage(`{"level":"error","args":["boom"]}`),
				}},
				Dropped:            0,
				OldestAvailableSeq: 1,
			},
		})
	}))
	defer server.Close()

	c := NewClient(server.URL, "")
	resp, err := c.GetEvents(nil, "console", 40)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Entries) != 1 || resp.Entries[0].Seq != 41 {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestExecuteCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST method, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ApiResponse{
			OK: true,
			Data: CommandResult{
				ID:       "1",
				Success:  true,
				Duration: 42,
			},
		})
	}))
	defer server.Close()

	c := NewClient(server.URL, "")
	result, err := c.ExecuteCommand(nil, Command{
		ID:     "1",
		Action: "click",
		Target: &TargetSelector{Selector: "#btn"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success=true")
	}
	if result.Duration != 42 {
		t.Errorf("expected duration 42, got %d", result.Duration)
	}
}

func TestAuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		w.Write([]byte(`{"ok":false,"error":"unauthorized"}`))
	}))
	defer server.Close()

	c := NewClient(server.URL, "wrong-token")
	_, err := c.GetHealth()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if _, ok := err.(*AuthError); !ok {
		t.Errorf("expected AuthError, got %T", err)
	}
}

func TestNotFoundError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`{"ok":false,"error":"tab not found"}`))
	}))
	defer server.Close()

	c := NewClient(server.URL, "")
	_, err := c.GetTab(999)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if _, ok := err.(*NotFoundError); !ok {
		t.Errorf("expected NotFoundError, got %T", err)
	}
}

func TestGetScreenshotOptsSendsFullPageAndAnnotate(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ApiResponse{OK: true, Data: "QUJD"})
	}))
	defer server.Close()

	c := NewClient(server.URL, "")
	data, err := c.GetScreenshotOpts(ScreenshotOptions{
		FullPage: true,
		Annotate: []TargetSelector{{Selector: "button"}, {Role: "link"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data != "QUJD" {
		t.Fatalf("data = %q, want QUJD", data)
	}
	q, err := neturl.ParseQuery(gotQuery)
	if err != nil {
		t.Fatalf("parse query %q: %v", gotQuery, err)
	}
	if q.Get("fullPage") != "true" {
		t.Fatalf("fullPage = %q, want true", q.Get("fullPage"))
	}
	if q.Get("annotate") != `[{"selector":"button"},{"role":"link"}]` {
		t.Fatalf("annotate = %q, want the selector JSON array", q.Get("annotate"))
	}
}

func TestGetScreenshotViewportSendsNoOptions(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ApiResponse{OK: true, Data: "QUJD"})
	}))
	defer server.Close()

	c := NewClient(server.URL, "")
	if _, err := c.GetScreenshot(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotQuery != "" {
		t.Fatalf("viewport screenshot should send no query params, got %q", gotQuery)
	}
}

func TestGetScreenshotOptsSendsTab(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ApiResponse{OK: true, Data: "QUJD"})
	}))
	defer server.Close()

	c := NewClient(server.URL, "")
	tabID := 7
	if _, err := c.GetScreenshotOpts(ScreenshotOptions{TabID: &tabID}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	q, err := neturl.ParseQuery(gotQuery)
	if err != nil {
		t.Fatalf("parse query %q: %v", gotQuery, err)
	}
	if q.Get("tab") != "7" {
		t.Fatalf("tab = %q, want 7", q.Get("tab"))
	}
}

func TestNoAuthHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("expected no Authorization header, got %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ApiResponse{OK: true, Data: HealthResponse{Status: "ok"}})
	}))
	defer server.Close()

	c := NewClient(server.URL, "")
	_, err := c.GetHealth()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
