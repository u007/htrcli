package commands

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/u007/htrcli/internal/api"
)

func TestCollectSendsCollectAllAction(t *testing.T) {
	var gotAction, gotSelector string
	var gotOptions map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req api.CommandRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		gotAction = req.Command.Action
		if req.Command.Target != nil {
			gotSelector = req.Command.Target.Selector
		}
		gotOptions = req.Command.Options
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(api.ApiResponse{
			OK: true,
			Data: api.CommandResult{ID: "1", Success: true, Data: map[string]any{
				"elements":  []any{},
				"steps":     0,
				"truncated": false,
				"scrolled":  false,
			}},
		})
	}))
	defer server.Close()

	client = api.NewClient(server.URL, "")
	tabTarget = ""
	transportFlag = "ext"
	defer func() { client = nil; transportFlag = "" }()

	collectMaxSteps = 0
	collectStepDelay = 120
	collectMaxItems = 0
	collectRef = false

	if err := collectCmd.RunE(collectCmd, []string{"tr.zA"}); err != nil {
		t.Fatalf("collect RunE: %v", err)
	}
	if gotAction != "collectAll" {
		t.Fatalf("want action collectAll, got %q", gotAction)
	}
	if gotSelector != "tr.zA" {
		t.Fatalf("want selector tr.zA, got %q", gotSelector)
	}
	// The default step delay is always sent; the optional limits are omitted
	// so the library defaults apply.
	if gotOptions["stepDelayMs"] != float64(120) {
		t.Fatalf("want stepDelayMs 120, got %v", gotOptions["stepDelayMs"])
	}
	if _, ok := gotOptions["maxSteps"]; ok {
		t.Fatalf("maxSteps should be omitted when 0, got %v", gotOptions["maxSteps"])
	}
	if _, ok := gotOptions["maxItems"]; ok {
		t.Fatalf("maxItems should be omitted when 0, got %v", gotOptions["maxItems"])
	}
	if _, ok := gotOptions["assignRef"]; ok {
		t.Fatalf("assignRef should be omitted when false, got %v", gotOptions["assignRef"])
	}
}

func TestCollectSendsLimitsAndRefFlag(t *testing.T) {
	var gotOptions map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req api.CommandRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		gotOptions = req.Command.Options
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

	collectMaxSteps = 25
	collectStepDelay = 50
	collectMaxItems = 100
	collectRef = true
	defer func() {
		collectMaxSteps = 0
		collectStepDelay = 120
		collectMaxItems = 0
		collectRef = false
	}()

	if err := collectCmd.RunE(collectCmd, []string{"li"}); err != nil {
		t.Fatalf("collect RunE: %v", err)
	}
	if gotOptions["maxSteps"] != float64(25) {
		t.Fatalf("want maxSteps 25, got %v", gotOptions["maxSteps"])
	}
	if gotOptions["stepDelayMs"] != float64(50) {
		t.Fatalf("want stepDelayMs 50, got %v", gotOptions["stepDelayMs"])
	}
	if gotOptions["maxItems"] != float64(100) {
		t.Fatalf("want maxItems 100, got %v", gotOptions["maxItems"])
	}
	if gotOptions["assignRef"] != true {
		t.Fatalf("want assignRef true, got %v", gotOptions["assignRef"])
	}
}

// --visible must reach the page as visibleOnly. Gmail keeps its previous view
// in the DOM, so a collection can otherwise accumulate rows the user cannot
// see — the same 87-for-37 defect extract list had.
func TestCollectPassesVisibleThrough(t *testing.T) {
	var gotOptions map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req api.CommandRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		gotOptions = req.Command.Options
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
	collectVisible = true
	defer func() { client = nil; transportFlag = ""; collectVisible = false }()

	if err := collectCmd.RunE(collectCmd, []string{"tr.zA"}); err != nil {
		t.Fatalf("collect RunE: %v", err)
	}
	if gotOptions["visibleOnly"] != true {
		t.Fatalf("want visibleOnly true, got %v", gotOptions["visibleOnly"])
	}
}

// Without the flag the page must not receive visibleOnly at all: an absent key
// keeps every row, and a false value would be a silent behaviour change for
// existing callers.
func TestCollectOmitsVisibleUnlessAsked(t *testing.T) {
	var gotOptions map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req api.CommandRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		gotOptions = req.Command.Options
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
	collectVisible = false
	defer func() { client = nil; transportFlag = "" }()

	if err := collectCmd.RunE(collectCmd, []string{"tr.zA"}); err != nil {
		t.Fatalf("collect RunE: %v", err)
	}
	if _, present := gotOptions["visibleOnly"]; present {
		t.Fatalf("visibleOnly must be absent when --visible is not passed: %v", gotOptions)
	}
}
