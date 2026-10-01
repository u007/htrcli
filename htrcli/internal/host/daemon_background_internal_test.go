package host

import (
	"encoding/json"
	"errors"
	"testing"
)

// This file is in package host (not host_test) so it can assert on the
// unexported pending-command bookkeeping. The public suite in
// daemon_background_test.go cannot observe it: a leaked entry is invisible from
// outside, because the only consequence is a slow memory leak in a long-running
// daemon. Asserting it needs access to d.pending directly.
//
// Everything else about background commands is tested from package host_test, so
// this file stays deliberately narrow.

func TestEnqueueBackgroundCommandFailedWriteClearsPending(t *testing.T) {
	d := NewDaemon()
	d.AddConn(func(_ []byte) error { return errTestWrite })

	if _, err := d.EnqueueBackgroundCommand(Command{Action: "recordingList"}, ""); err == nil {
		t.Fatal("want an error when the relay write fails")
	}

	d.mu.Lock()
	leaked := len(d.pending)
	d.mu.Unlock()

	if leaked != 0 {
		t.Errorf("pending has %d entries after a failed write, want 0; a retained "+
			"entry leaks for the life of the daemon and is never resolvable", leaked)
	}
}

// TestSetConnBrowserIgnoresEmptyValue pins the "unknown is not a value" rule: an
// empty announcement must not erase an identity a connection already reported,
// because a relayer that repeats an empty identify on reconnect would otherwise
// silently un-identify itself.
func TestSetConnBrowserIgnoresEmptyValue(t *testing.T) {
	d := NewDaemon()
	rc := d.AddConn(func(_ []byte) error { return nil })

	d.SetConnBrowser(rc, "firefox")
	d.SetConnBrowser(rc, "")

	if got := d.ConnBrowser(rc); got != "firefox" {
		t.Errorf("ConnBrowser = %q after an empty identify, want %q retained", got, "firefox")
	}
}

// TestEnqueueBackgroundCommandWritesZeroTabID pins the difference from
// EnqueueCommand: a background command carries no tab, so TabID stays zero and
// is omitted from the wire (it is `omitempty` on NativeMessage).
func TestEnqueueBackgroundCommandWritesZeroTabID(t *testing.T) {
	d := NewDaemon()
	var raw string
	d.AddConn(func(msg []byte) error {
		raw = string(msg)
		return nil
	})

	if _, err := d.EnqueueBackgroundCommand(Command{Action: "recordingList"}, ""); err != nil {
		t.Fatalf("EnqueueBackgroundCommand: %v", err)
	}

	var nm NativeMessage
	if err := json.Unmarshal([]byte(raw), &nm); err != nil {
		t.Fatalf("relay received unparseable message %q: %v", raw, err)
	}
	if nm.Type != "command" {
		t.Errorf("Type = %q, want %q", nm.Type, "command")
	}
	if nm.TabID != 0 {
		t.Errorf("TabID = %d, want 0 for a tab-less command", nm.TabID)
	}
}

// errTestWrite stands in for a relay write failure inside this package.
var errTestWrite = errors.New("relay write failed")
