package commands

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/u007/htrcli/internal/api"
)

func TestFindAllSendsFindAllAction(t *testing.T) {
	var gotAction, gotSelector string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req api.CommandRequest
		json.NewDecoder(r.Body).Decode(&req)
		gotAction = req.Command.Action
		if req.Command.Target != nil {
			gotSelector = req.Command.Target.Selector
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(api.ApiResponse{
			OK:   true,
			Data: api.CommandResult{ID: "1", Success: true, Data: []any{}},
		})
	}))
	defer server.Close()

	client = api.NewClient(server.URL, "")
	tabTarget = ""
	transportFlag = "ext"
	defer func() { client = nil; transportFlag = "" }()

	if err := findAllCmd.RunE(findAllCmd, []string{"button.primary"}); err != nil {
		t.Fatalf("findAll RunE: %v", err)
	}
	if gotAction != "findAll" {
		t.Fatalf("want action findAll, got %q", gotAction)
	}
	if gotSelector != "button.primary" {
		t.Fatalf("want selector button.primary, got %q", gotSelector)
	}
}
