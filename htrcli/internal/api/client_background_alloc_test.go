package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
)

// maxBackgroundDecodeBytes bounds the bytes allocated per call while decoding a
// large background-command result.
//
// The bound exists to pin the SINGLE-PASS decode. This method exists because
// ExecuteCommand marshalled ApiResponse.Data back to bytes and re-parsed it,
// which on a recording full of screenshots copies a multi-megabyte payload two
// extra times. No correctness test can catch that: marshalling a json.RawMessage
// and parsing it again yields the same value, so the wasteful version passes
// every behavioural test in this package. Measured with a 4 MiB payload:
//
//	round-tripping version: ~27.0 MiB allocated per call
//	single-pass version:    ~21.6 MiB allocated per call
//
// 24 MiB sits deliberately between the two. It is ~11% above the single-pass
// figure (headroom for a different Go version's allocator) and ~11% below the
// wasteful one, so the test fails if the round-trip ever comes back.
func TestExecuteBackgroundCommandIntoAllocatesOnce(t *testing.T) {
	// The race detector instruments every allocation, so an allocation BOUND is
	// meaningless under -race: the same code measures far higher purely from the
	// instrumentation. Skipping is the honest option — the bound has nothing to
	// say about a binary that is 40% slower and allocating extra by construction.
	if raceEnabled {
		t.Skip("allocation bounds are not meaningful under -race (instrumentation inflates them)")
	}

	const payloadBytes = 4 << 20
	const maxBytesPerCall = 24 << 20
	const runs = 3

	big := strings.Repeat("A", payloadBytes)
	body := ApiResponse{OK: true, Data: CommandResult{
		ID:      "1",
		Success: true,
		Data:    json.RawMessage(fmt.Sprintf(`{"title":%q,"total":3}`, big)),
	}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(body); err != nil {
			t.Errorf("encoding response: %v", err)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "")

	// Warm up so one-time allocations (connection setup, first decode) are not
	// attributed to the measured window.
	var out payload
	if err := c.ExecuteBackgroundCommandInto(Command{Action: "recordingList"}, "", &out); err != nil {
		t.Fatalf("warm-up call: %v", err)
	}
	if out.Total != 3 {
		t.Fatalf("decoded total = %d, want 3", out.Total)
	}

	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for range runs {
		if err := c.ExecuteBackgroundCommandInto(Command{Action: "recordingList"}, "", &out); err != nil {
			t.Fatalf("measured call: %v", err)
		}
	}
	runtime.ReadMemStats(&after)

	perCall := (after.TotalAlloc - before.TotalAlloc) / runs
	if perCall > maxBytesPerCall {
		t.Errorf("allocated %d bytes per call decoding a %d-byte payload, want at most %d; "+
			"the single-pass decode regressed to a marshal-then-reparse round trip",
			perCall, payloadBytes, maxBytesPerCall)
	}
}
