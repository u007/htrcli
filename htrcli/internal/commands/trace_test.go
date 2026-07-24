package commands

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/u007/htrcli/internal/api"
)

func zipEntries(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	out := map[string][]byte{}
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open entry %s: %v", f.Name, err)
		}
		var buf bytes.Buffer
		if _, err := buf.ReadFrom(rc); err != nil {
			t.Fatalf("read entry %s: %v", f.Name, err)
		}
		rc.Close()
		out[f.Name] = buf.Bytes()
	}
	return out
}

func TestBuildTraceZip(t *testing.T) {
	bundle := traceBundle{
		Page:          &api.PageInfo{URL: "https://example.com", Title: "Example"},
		Console:       []api.EventEntry{{Seq: 1, Kind: "console", Timestamp: 100, Data: json.RawMessage(`{"level":"log","args":["hi"]}`)}},
		Network:       nil,
		ScreenshotPNG: []byte("\x89PNGfake"),
		ExportedAt:    time.Unix(1700000000, 0),
	}
	data, err := buildTraceZip(bundle)
	if err != nil {
		t.Fatalf("buildTraceZip: %v", err)
	}
	entries := zipEntries(t, data)

	for _, name := range []string{"trace.json", "console.json", "network.json", "README.md", "screenshots/snapshot.png"} {
		if _, ok := entries[name]; !ok {
			t.Errorf("expected zip entry %q, missing (have %v)", name, keys(entries))
		}
	}

	// trace.json embeds the page + console.
	var trace struct {
		Page    *api.PageInfo    `json:"page"`
		Console []api.EventEntry `json:"console"`
		Network []api.EventEntry `json:"network"`
	}
	if err := json.Unmarshal(entries["trace.json"], &trace); err != nil {
		t.Fatalf("parse trace.json: %v", err)
	}
	if trace.Page == nil || trace.Page.URL != "https://example.com" {
		t.Errorf("trace.json page mismatch: %+v", trace.Page)
	}
	if len(trace.Console) != 1 || trace.Console[0].Seq != 1 {
		t.Errorf("trace.json console mismatch: %+v", trace.Console)
	}
	if string(entries["screenshots/snapshot.png"]) != "\x89PNGfake" {
		t.Errorf("screenshot bytes mismatch")
	}
}

func TestBuildTraceZipOmitsScreenshotWhenAbsent(t *testing.T) {
	data, err := buildTraceZip(traceBundle{ExportedAt: time.Now()})
	if err != nil {
		t.Fatalf("buildTraceZip: %v", err)
	}
	entries := zipEntries(t, data)
	if _, ok := entries["screenshots/snapshot.png"]; ok {
		t.Error("expected no screenshot entry when ScreenshotPNG is empty")
	}
	if _, ok := entries["trace.json"]; !ok {
		t.Error("expected trace.json even with no screenshot")
	}
}

func TestCollectTrace(t *testing.T) {
	pngB64 := base64.StdEncoding.EncodeToString([]byte("\x89PNGfake"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/page":
			json.NewEncoder(w).Encode(api.ApiResponse{OK: true, Data: api.PageInfo{URL: "https://ex.com", Title: "Ex"}})
		case r.URL.Path == "/api/events" && r.URL.Query().Get("kind") == "console":
			json.NewEncoder(w).Encode(api.ApiResponse{OK: true, Data: api.EventsResponse{
				Entries: []api.EventEntry{{Seq: 1, Kind: "console", Timestamp: 1, Data: json.RawMessage(`{"level":"log","args":["hi"]}`)}},
			}})
		case r.URL.Path == "/api/events" && r.URL.Query().Get("kind") == "network":
			json.NewEncoder(w).Encode(api.ApiResponse{OK: true, Data: api.EventsResponse{Entries: nil}})
		case r.URL.Path == "/api/screenshot":
			json.NewEncoder(w).Encode(api.ApiResponse{OK: true, Data: pngB64})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	c := api.NewClient(srv.URL, "")
	bundle, err := collectTrace(c, nil)
	if err != nil {
		t.Fatalf("collectTrace: %v", err)
	}
	if bundle.Page == nil || bundle.Page.URL != "https://ex.com" {
		t.Errorf("page mismatch: %+v", bundle.Page)
	}
	if len(bundle.Console) != 1 {
		t.Errorf("expected 1 console entry, got %d", len(bundle.Console))
	}
	if string(bundle.ScreenshotPNG) != "\x89PNGfake" {
		t.Errorf("screenshot decode mismatch: %q", bundle.ScreenshotPNG)
	}
}

func TestCollectTraceUsesTabForScreenshot(t *testing.T) {
	var screenshotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/page":
			json.NewEncoder(w).Encode(api.ApiResponse{OK: true, Data: api.PageInfo{URL: "https://ex.com"}})
		case r.URL.Path == "/api/events" && r.URL.Query().Get("kind") == "console":
			json.NewEncoder(w).Encode(api.ApiResponse{OK: true, Data: api.EventsResponse{Entries: nil}})
		case r.URL.Path == "/api/events" && r.URL.Query().Get("kind") == "network":
			json.NewEncoder(w).Encode(api.ApiResponse{OK: true, Data: api.EventsResponse{Entries: nil}})
		case r.URL.Path == "/api/screenshot":
			screenshotQuery = r.URL.RawQuery
			json.NewEncoder(w).Encode(api.ApiResponse{OK: true, Data: base64.StdEncoding.EncodeToString([]byte("\x89PNGfake"))})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	tabID := 5
	c := api.NewClient(srv.URL, "")
	bundle, err := collectTrace(c, &tabID)
	if err != nil {
		t.Fatalf("collectTrace: %v", err)
	}
	if screenshotQuery == "" || !strings.Contains(screenshotQuery, "tab=5") {
		t.Fatalf("expected screenshot query to include tab=5, got %q", screenshotQuery)
	}
	if len(bundle.ScreenshotPNG) == 0 {
		t.Fatal("expected screenshot bytes")
	}
}

func TestCollectTraceNetworkErrorNonFatal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/page":
			json.NewEncoder(w).Encode(api.ApiResponse{OK: true, Data: api.PageInfo{URL: "https://ex.com"}})
		case r.URL.Path == "/api/events" && r.URL.Query().Get("kind") == "console":
			json.NewEncoder(w).Encode(api.ApiResponse{OK: true, Data: api.EventsResponse{Entries: nil}})
		case r.URL.Path == "/api/events" && r.URL.Query().Get("kind") == "network":
			json.NewEncoder(w).Encode(api.ApiResponse{OK: false, Error: "network capture not enabled"})
		case r.URL.Path == "/api/screenshot":
			json.NewEncoder(w).Encode(api.ApiResponse{OK: false, Error: "no tab"})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	c := api.NewClient(srv.URL, "")
	bundle, err := collectTrace(c, nil)
	if err != nil {
		t.Fatalf("collectTrace should not fail on network/screenshot errors: %v", err)
	}
	if len(bundle.Network) != 0 || len(bundle.ScreenshotPNG) != 0 {
		t.Errorf("expected empty network + screenshot, got %+v", bundle)
	}
}

func TestTraceExportRejectsCDP(t *testing.T) {
	t.Cleanup(func() { transportFlag = ""; cdpFlag = false })
	transportFlag = "cdp"
	err := traceExportCmd.RunE(traceExportCmd, []string{"out.zip"})
	if err == nil {
		t.Fatal("expected trace export to reject the CDP transport")
	}
}

func TestTraceExportRequiresOneArg(t *testing.T) {
	if err := traceExportCmd.Args(traceExportCmd, nil); err == nil {
		t.Fatal("expected trace export to require an output path")
	}
	if err := traceExportCmd.Args(traceExportCmd, []string{"a", "b"}); err == nil {
		t.Fatal("expected trace export to reject two args")
	}
}

func keys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
