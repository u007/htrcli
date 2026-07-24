package commands

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/u007/htrcli/internal/api"
)

func networkEvent(seq int, url, method string, status int) api.EventEntry {
	data, _ := json.Marshal(networkEventData{
		RequestID:  "r",
		URL:        url,
		Method:     method,
		Status:     status,
		DurationMs: 12,
	})
	return api.EventEntry{Seq: seq, Kind: "network", Timestamp: 1000, Data: data}
}

func TestNetworkReadFormatsEntries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(api.ApiResponse{
			OK: true,
			Data: api.EventsResponse{
				Entries:            []api.EventEntry{networkEvent(1, "https://x.test/api", "GET", 200)},
				Dropped:            0,
				OldestAvailableSeq: 1,
			},
		})
	}))
	defer server.Close()

	c := api.NewClient(server.URL, "")
	poller := &EventPoller{Client: c, Kind: networkEventKind}
	resp, err := poller.Read(0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := formatNetworkEntries(resp)
	if !strings.Contains(out, "GET") || !strings.Contains(out, "200") || !strings.Contains(out, "https://x.test/api") {
		t.Fatalf("expected method/status/url in output, got: %s", out)
	}
}

func TestNetworkEntryMatches(t *testing.T) {
	entry := networkEvent(1, "https://x.test/api/users?page=2", "GET", 200)
	if !networkEntryMatches(entry, "*/api/users*", 0) {
		t.Fatalf("expected glob match on url")
	}
	if networkEntryMatches(entry, "*/api/orders*", 0) {
		t.Fatalf("did not expect match on non-matching url")
	}
	if !networkEntryMatches(entry, "*/api/users*", 200) {
		t.Fatalf("expected match when status also matches")
	}
	if networkEntryMatches(entry, "*/api/users*", 404) {
		t.Fatalf("did not expect match when status differs")
	}
}
