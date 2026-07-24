package commands

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/u007/htrcli/internal/api"
)

func TestNetworkMockSendsRuleWithBodyFile(t *testing.T) {
	dir := t.TempDir()
	bodyPath := filepath.Join(dir, "resp.json")
	if err := os.WriteFile(bodyPath, []byte(`{"ok":true}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var gotAction string
	var gotRule map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req api.CommandRequest
		json.NewDecoder(r.Body).Decode(&req)
		gotAction = req.Command.Action
		if rules, ok := req.Command.Options["rules"].([]any); ok && len(rules) > 0 {
			gotRule, _ = rules[0].(map[string]any)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(api.ApiResponse{OK: true, Data: api.CommandResult{ID: "1", Success: true}})
	}))
	defer server.Close()

	client = api.NewClient(server.URL, "")
	tabTarget = ""
	transportFlag = "ext"
	mockStatus = 200
	mockURLPattern = "https://api.example.com/*"
	mockBodyFile = bodyPath
	mockMethod = ""
	defer func() {
		client = nil
		transportFlag = ""
		mockStatus = 0
		mockURLPattern = ""
		mockBodyFile = ""
	}()

	if err := networkMockCmd.RunE(networkMockCmd, nil); err != nil {
		t.Fatalf("network mock RunE: %v", err)
	}
	if gotAction != "networkMock" {
		t.Fatalf("want action networkMock, got %q", gotAction)
	}
	if gotRule["urlPattern"] != "https://api.example.com/*" {
		t.Fatalf("unexpected urlPattern: %v", gotRule["urlPattern"])
	}
	if gotRule["kind"] != "fulfill" {
		t.Fatalf("want kind fulfill, got %v", gotRule["kind"])
	}
	if gotRule["body"] != `{"ok":true}` {
		t.Fatalf("body not read from file: %v", gotRule["body"])
	}
}

func TestNetworkMockUnsupportedOnCDP(t *testing.T) {
	transportFlag = "cdp"
	mockURLPattern = "https://x/*"
	defer func() { transportFlag = ""; mockURLPattern = "" }()
	if err := networkMockCmd.RunE(networkMockCmd, nil); err == nil {
		t.Fatal("want error on --cdp transport, got nil")
	}
}
