package host_test

import (
	"testing"

	"github.com/u007/htrcli/internal/host"
)

// TestFirstTabIDIsDeterministicAcrossRepeatedCalls pins the selection order of
// FirstTabID.
//
// The loop count is the point of this test, not incidental: FirstTabID selects
// by ranging over a Go map, and Go deliberately randomises map iteration order
// on every range. A single call can never detect that — the non-determinism
// only shows up when the caller runs the same lookup repeatedly, which is
// exactly what the daemon does across successive HTTP requests. So this
// asserts the answer is identical across many calls, and a fixed number of
// repeats (rather than a time budget) keeps it deterministic to reason about.
//
// Each connection must come from AddConn: RegisterTab alone writes rc.tabs but
// never inserts the connection into d.conns, and d.conns is the map FirstTabID
// iterates. Omitting that step makes FirstTabID return false, and the test
// would pass for the wrong reason.
func TestFirstTabIDIsDeterministicAcrossRepeatedCalls(t *testing.T) {
	const repeats = 2000

	d := host.NewDaemon()
	rcA := noopConn(d)
	rcB := noopConn(d)

	// Three tabs each, so the assertion below distinguishes "same connection,
	// same tab" from "same connection, arbitrary tab".
	for id := range 3 {
		tabID := id + 1
		d.RegisterTab(rcA, tabID, host.TabInfo{ID: tabID, URL: "https://a.example"})
		d.RegisterTab(rcB, tabID+100, host.TabInfo{ID: tabID + 100, URL: "https://b.example"})
	}

	first, ok := d.FirstTabID()
	if !ok {
		t.Fatal("FirstTabID reported no tab; AddConn did not register the connection")
	}

	for i := range repeats {
		got, ok := d.FirstTabID()
		if !ok {
			t.Fatalf("call %d: FirstTabID reported no tab", i)
		}
		if got != first {
			t.Fatalf("call %d of %d: FirstTabID = %d, want %d (first call); "+
				"tab selection depends on Go map iteration order",
				i, repeats, got, first)
		}
	}
}
