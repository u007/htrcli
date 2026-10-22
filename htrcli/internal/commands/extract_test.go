package commands

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/u007/htrcli/internal/api"
)

func TestExtractTableSendsExtractTableAction(t *testing.T) {
	var gotAction, gotSelector string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req api.CommandRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode: %v", err)
		}
		gotAction = req.Command.Action
		if req.Command.Target != nil {
			gotSelector = req.Command.Target.Selector
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(api.ApiResponse{
			OK: true,
			Data: api.CommandResult{ID: "1", Success: true, Data: map[string]any{
				"columns":  []any{"A", "B"},
				"rows":     []any{map[string]any{"A": "1", "B": "2"}},
				"rowCount": 1,
			}},
		})
	}))
	defer server.Close()

	client = api.NewClient(server.URL, "")
	tabTarget = ""
	transportFlag = "ext"
	defer func() { client = nil; transportFlag = "" }()

	if err := extractTableCmd.RunE(extractTableCmd, []string{"table"}); err != nil {
		t.Fatalf("RunE: %v", err)
	}
	if gotAction != "extractTable" {
		t.Fatalf("want extractTable, got %q", gotAction)
	}
	if gotSelector != "table" {
		t.Fatalf("want selector table, got %q", gotSelector)
	}
}

func TestExtractListSendsFields(t *testing.T) {
	var gotAction string
	var gotOptions map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req api.CommandRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode: %v", err)
		}
		gotAction = req.Command.Action
		gotOptions = req.Command.Options
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(api.ApiResponse{
			OK: true,
			Data: api.CommandResult{ID: "1", Success: true, Data: map[string]any{
				"items": []any{map[string]any{"name": "Alice"}},
				"count": 1,
			}},
		})
	}))
	defer server.Close()

	client = api.NewClient(server.URL, "")
	tabTarget = ""
	transportFlag = "ext"
	defer func() { client = nil; transportFlag = "" }()

	extractFields = "name:.name,age:.age"
	defer func() { extractFields = "" }()

	if err := extractListCmd.RunE(extractListCmd, []string{"div"}); err != nil {
		t.Fatalf("RunE: %v", err)
	}
	if gotAction != "extractList" {
		t.Fatalf("want extractList, got %q", gotAction)
	}
	fields, ok := gotOptions["fields"].(map[string]any)
	if !ok {
		t.Fatalf("want fields option, got %v", gotOptions)
	}
	if fields["name"] != ".name" || fields["age"] != ".age" {
		t.Fatalf("bad fields: %v", fields)
	}
}

// An attribute field ("@data-due", "@.link@href") must survive the
// name:selector parsing intact: the "@" is part of the selector, not a
// separator, and a CSS selector may itself contain a colon.
func TestExtractListPassesAttributeFieldsThrough(t *testing.T) {
	var gotOptions map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req api.CommandRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode: %v", err)
		}
		gotOptions = req.Command.Options
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(api.ApiResponse{
			OK: true,
			Data: api.CommandResult{ID: "1", Success: true, Data: map[string]any{
				"items": []any{},
				"count": 0,
			}},
		})
	}))
	defer server.Close()

	client = api.NewClient(server.URL, "")
	tabTarget = ""
	transportFlag = "ext"
	defer func() { client = nil; transportFlag = "" }()

	extractFields = "title:.title,due:@data-due,href:@.link@href"
	defer func() { extractFields = "" }()

	if err := extractListCmd.RunE(extractListCmd, []string{"div"}); err != nil {
		t.Fatalf("RunE: %v", err)
	}
	fields, ok := gotOptions["fields"].(map[string]any)
	if !ok {
		t.Fatalf("want fields option, got %v", gotOptions)
	}
	if fields["title"] != ".title" {
		t.Errorf("title field = %v, want .title", fields["title"])
	}
	if fields["due"] != "@data-due" {
		t.Errorf("due field = %v, want @data-due", fields["due"])
	}
	if fields["href"] != "@.link@href" {
		t.Errorf("href field = %v, want @.link@href", fields["href"])
	}
}

func TestExtractDispatchesByMode(t *testing.T) {
	var gotAction string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req api.CommandRequest
		json.NewDecoder(r.Body).Decode(&req)
		gotAction = req.Command.Action
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(api.ApiResponse{
			OK:   true,
			Data: api.CommandResult{ID: "1", Success: true, Data: map[string]any{}},
		})
	}))
	defer server.Close()

	client = api.NewClient(server.URL, "")
	tabTarget = ""
	transportFlag = "ext"
	defer func() { client = nil; transportFlag = "" }()

	if err := extractCmd.RunE(extractCmd, []string{"table", "table"}); err != nil {
		t.Fatalf("RunE: %v", err)
	}
	if gotAction != "extractTable" {
		t.Fatalf("want extractTable, got %q", gotAction)
	}
}
