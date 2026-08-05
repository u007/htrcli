package commands

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/u007/htrcli/internal/api"
	"github.com/u007/htrcli/internal/output"
)

func TestBuildFilesData(t *testing.T) {
	dir := t.TempDir()
	p1 := filepath.Join(dir, "addon.xpi")
	p2 := filepath.Join(dir, "src.zip")
	if err := os.WriteFile(p1, []byte("xpi-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p2, []byte("zip-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := buildFilesData([]string{p1, p2})
	if err != nil {
		t.Fatalf("buildFilesData: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got))
	}

	e0 := got[0].(map[string]any)
	if e0["name"] != "addon.xpi" {
		t.Errorf("entry 0 name = %v, want addon.xpi", e0["name"])
	}
	if got0 := e0["base64"].(string); got0 != base64.StdEncoding.EncodeToString([]byte("xpi-bytes")) {
		t.Errorf("entry 0 base64 = %q", got0)
	}
	if e0["mimeType"] == "" {
		t.Error("entry 0 mimeType should not be empty for .xpi")
	}

	e1 := got[1].(map[string]any)
	if e1["name"] != "src.zip" {
		t.Errorf("entry 1 name = %v, want src.zip", e1["name"])
	}
	if got1 := e1["base64"].(string); got1 != base64.StdEncoding.EncodeToString([]byte("zip-bytes")) {
		t.Errorf("entry 1 base64 = %q", got1)
	}
	if e1["mimeType"] != "application/zip" {
		t.Errorf("entry 1 mimeType = %v, want application/zip", e1["mimeType"])
	}
}

func TestBuildFilesDataMissingFile(t *testing.T) {
	if _, err := buildFilesData([]string{filepath.Join(t.TempDir(), "nope.xpi")}); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestRunUploadExtUsesRegisteredBrowserForPayload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upload.txt")
	if err := os.WriteFile(path, []byte("upload-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}

	previousClient := client
	previousTabTarget := tabTarget
	previousJSONOutput := output.JSONOutput
	defer func() {
		client = previousClient
		tabTarget = previousTabTarget
		output.JSONOutput = previousJSONOutput
	}()

	for _, test := range []struct {
		name          string
		browser       string
		wantFilesData bool
	}{
		{name: "chrome", browser: "chrome"},
		{name: "firefox", browser: "firefox", wantFilesData: true},
		{name: "legacy metadata", browser: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.Method {
				case http.MethodGet:
					if r.URL.Path != "/api/tabs/7" {
						t.Errorf("tab lookup path = %s, want /api/tabs/7", r.URL.Path)
					}
					_ = json.NewEncoder(w).Encode(api.ApiResponse{
						OK: true,
						Data: api.TabInfo{
							ID:      7,
							URL:     "https://example.com",
							Browser: test.browser,
						},
					})
				case http.MethodPost:
					if r.URL.Path != "/api/tabs/7/command" {
						t.Errorf("command path = %s, want /api/tabs/7/command", r.URL.Path)
					}
					var request api.CommandRequest
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Errorf("decode command request: %v", err)
					} else {
						_, hasFilesData := request.Command.Options["filesData"]
						if hasFilesData != test.wantFilesData {
							t.Errorf("filesData present = %t, want %t", hasFilesData, test.wantFilesData)
						}
						_, hasFiles := request.Command.Options["files"]
						if !hasFiles {
							t.Error("files path list is missing")
						}
					}
					_ = json.NewEncoder(w).Encode(api.ApiResponse{
						OK:   true,
						Data: api.CommandResult{ID: "1", Success: true},
					})
				default:
					t.Errorf("unexpected method %s", r.Method)
				}
			}))
			defer server.Close()

			client = api.NewClient(server.URL, "")
			tabTarget = "7"
			output.JSONOutput = false
			if err := runUploadExt("#upload", []string{path}); err != nil {
				t.Fatalf("runUploadExt: %v", err)
			}
		})
	}
}
