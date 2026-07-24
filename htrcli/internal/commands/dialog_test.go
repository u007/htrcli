package commands

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/u007/htrcli/internal/api"
)

func TestDialogListFormatsEntries(t *testing.T) {
	data, _ := json.Marshal(dialogEventData{
		DialogType:     "confirm",
		Message:        "Delete this item?",
		ResolvedAction: "accept",
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("kind") != "dialog" {
			t.Errorf("expected kind=dialog, got %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(api.ApiResponse{
			OK: true,
			Data: api.EventsResponse{
				Entries:            []api.EventEntry{{Seq: 1, Kind: "dialog", Timestamp: 1000, Data: data}},
				OldestAvailableSeq: 1,
			},
		})
	}))
	defer server.Close()

	c := api.NewClient(server.URL, "")
	poller := &EventPoller{Client: c, Kind: dialogEventKind}
	resp, err := poller.Read(0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := formatDialogEntries(resp)
	if !strings.Contains(out, "confirm") || !strings.Contains(out, "Delete this item?") || !strings.Contains(out, "accept") {
		t.Fatalf("expected dialog fields in output, got: %s", out)
	}
}

func TestParseDialogAction(t *testing.T) {
	if _, err := parseDialogAction("accept"); err != nil {
		t.Fatalf("accept should be valid: %v", err)
	}
	if _, err := parseDialogAction("respond"); err != nil {
		t.Fatalf("respond should be valid: %v", err)
	}
	if _, err := parseDialogAction("frobnicate"); err == nil {
		t.Fatalf("expected error for invalid action")
	}
}
