package host

import (
	"testing"
	"time"
)

func TestSendCommandTimeoutClearsPending(t *testing.T) {
	d := NewDaemon()
	rc := d.AddConn(func(_ []byte) error { return nil })
	d.RegisterTab(rc, 1, TabInfo{ID: 1, URL: "https://example.com", Title: "Example", Active: true})

	_, err := sendCommand(d, 1, Command{ID: "cmd-timeout", Action: "navigate", Value: "https://example.com/next"}, 10)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}

	if len(d.pending) != 0 {
		t.Fatalf("expected pending map to be cleared after timeout, got %d entries", len(d.pending))
	}

	// Give the timer branch a moment to settle so the test fails noisily if the
	// cleanup regresses and a late result sneaks in.
	time.Sleep(5 * time.Millisecond)
	if len(d.pending) != 0 {
		t.Fatalf("pending map was repopulated unexpectedly, got %d entries", len(d.pending))
	}
}
