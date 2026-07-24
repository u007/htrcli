package commands

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/u007/htrcli/internal/api"
)

func TestConsoleReadFormatsDroppedWarning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.ApiResponse{
			OK: true,
			Data: api.EventsResponse{
				Entries: []api.EventEntry{{
					Seq:       41,
					Kind:      "console",
					Timestamp: 1000,
					Data:      json.RawMessage(`{"level":"error","args":["boom"]}`),
				}},
				Dropped:            12,
				OldestAvailableSeq: 41,
			},
		})
	}))
	defer server.Close()

	c := api.NewClient(server.URL, "")
	poller := &EventPoller{Client: c, Kind: "console"}
	resp, err := poller.Read(40)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Dropped != 12 {
		t.Fatalf("want dropped=12, got %d", resp.Dropped)
	}

	out := formatConsoleEntries(resp)
	if !strings.Contains(out, "12 events were evicted") {
		t.Fatalf("expected drop warning in output, got: %s", out)
	}
	if !strings.Contains(out, "boom") {
		t.Fatalf("expected entry content in output, got: %s", out)
	}
}
