package host_test

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/u007/htrcli/internal/host"
)

// probe is a relay connection whose write function records what it was asked
// to send, so a test can tell WHICH connection a background command landed on.
// Selection is therefore observed through its effect (who received the command)
// rather than by calling a selector helper directly.
type probe struct {
	rc *host.RelayConn

	mu       sync.Mutex
	commands []host.NativeMessage
	// failWrite makes every write error, standing in for a relay that died
	// between being selected and being written to.
	failWrite bool
}

func newProbe(d *host.Daemon) *probe {
	p := &probe{}
	p.rc = d.AddConn(func(msg []byte) error {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.failWrite {
			return errWriteFailed
		}
		var nm host.NativeMessage
		if err := json.Unmarshal(msg, &nm); err != nil {
			return err
		}
		p.commands = append(p.commands, nm)
		return nil
	})
	return p
}

func (p *probe) got() []host.NativeMessage {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]host.NativeMessage(nil), p.commands...)
}

// errWriteFailed is a sentinel so a test can tell a deliberate write failure
// apart from a JSON decode problem.
var errWriteFailed = &writeError{}

type writeError struct{}

func (*writeError) Error() string { return "relay write failed" }

// winner reports whether this probe received a command of the given action.
func (p *probe) winner(action string) bool {
	for _, m := range p.got() {
		if m.Type == "command" && strings.Contains(string(m.Payload), action) {
			return true
		}
	}
	return false
}

// TestEnqueueBackgroundCommandNoConnections covers the empty-set case: the
// caller must get a clear error, never a hang on a channel nobody will write to.
func TestEnqueueBackgroundCommandNoConnections(t *testing.T) {
	d := host.NewDaemon()

	_, err := d.EnqueueBackgroundCommand(host.Command{Action: "recordingList"}, "")
	if err == nil {
		t.Fatal("want an error when no browser is connected")
	}
	if !strings.Contains(err.Error(), "no browser connected") {
		t.Errorf("error = %q, want it to mention \"no browser connected\"", err)
	}
}

// TestEnqueueBackgroundCommandSingleConnection is the common case: one browser,
// no hint, and it is the one that must receive the command.
func TestEnqueueBackgroundCommandSingleConnection(t *testing.T) {
	d := host.NewDaemon()
	p := newProbe(d)

	if _, err := d.EnqueueBackgroundCommand(host.Command{Action: "recordingList"}, ""); err != nil {
		t.Fatalf("EnqueueBackgroundCommand: %v", err)
	}
	if !p.winner("recordingList") {
		t.Error("the only connected relay did not receive the command")
	}
}

// TestEnqueueBackgroundCommandNoHintPicksEarliest asserts the no-hint case is
// the earliest-connected connection, not merely *a* connection. It repeats,
// because a single call cannot distinguish a stable rule from a lucky one.
func TestEnqueueBackgroundCommandNoHintPicksEarliest(t *testing.T) {
	const repeats = 200

	d := host.NewDaemon()
	earliest := newProbe(d)
	newProbe(d) // connected second, so it must lose

	for i := range repeats {
		if _, err := d.EnqueueBackgroundCommand(host.Command{Action: "recordingList"}, ""); err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
	}
	if n := len(earliest.got()); n != repeats {
		t.Errorf("earliest connection received %d of %d commands; want all of them", n, repeats)
	}
}

// TestEnqueueBackgroundCommandHintSelectsMatchingConnection is the whole point
// of the hint: it must be able to reach a browser that is NOT the earliest.
func TestEnqueueBackgroundCommandHintSelectsMatchingConnection(t *testing.T) {
	d := host.NewDaemon()
	chrome := newProbe(d) // earliest, but the hint does not name it
	firefox := newProbe(d)
	d.SetConnBrowser(chrome.rc, "chrome")
	d.SetConnBrowser(firefox.rc, "firefox")

	if _, err := d.EnqueueBackgroundCommand(host.Command{Action: "recordingList"}, "firefox"); err != nil {
		t.Fatalf("EnqueueBackgroundCommand: %v", err)
	}
	if !firefox.winner("recordingList") {
		t.Error("the connection matching the hint did not receive the command")
	}
	if chrome.winner("recordingList") {
		t.Error("a hint naming firefox was delivered to chrome")
	}
}

// TestEnqueueBackgroundCommandUnmatchedHintFallsBack pins the advisory
// semantics: a hint naming a browser that is not connected must still answer,
// using the earliest connection, rather than failing the call. recordings list
// reports which browser answered, so a wrong-profile answer is recoverable.
func TestEnqueueBackgroundCommandUnmatchedHintFallsBack(t *testing.T) {
	d := host.NewDaemon()
	earliest := newProbe(d)
	other := newProbe(d)
	d.SetConnBrowser(other.rc, "firefox")

	if _, err := d.EnqueueBackgroundCommand(host.Command{Action: "recordingList"}, "safari"); err != nil {
		t.Fatalf("a hint matching no connection must not be an error, got %v", err)
	}
	if !earliest.winner("recordingList") {
		t.Error("the fallback did not use the earliest connection")
	}
}

// TestEnqueueBackgroundCommandDuplicateBrowserPicksEarliest covers two
// connections announcing the same browser: the hint cannot disambiguate, so the
// documented tie-breaker is the connect order.
func TestEnqueueBackgroundCommandDuplicateBrowserPicksEarliest(t *testing.T) {
	d := host.NewDaemon()
	earliest := newProbe(d)
	second := newProbe(d)
	d.SetConnBrowser(earliest.rc, "chrome")
	d.SetConnBrowser(second.rc, "chrome")

	if _, err := d.EnqueueBackgroundCommand(host.Command{Action: "recordingList"}, "chrome"); err != nil {
		t.Fatalf("EnqueueBackgroundCommand: %v", err)
	}
	if !earliest.winner("recordingList") {
		t.Error("with two chrome connections, the earliest did not win")
	}
	if second.winner("recordingList") {
		t.Error("the later chrome connection received the command")
	}
}

// TestEnqueueBackgroundCommandPromotesAfterWinnerCloses covers a relay dying:
// the connection that WON the last command is the one removed, and the very next
// command must land on the next-earliest surviving connection — with no restart
// and no state left behind. Removing a loser instead would pass trivially,
// because the winner would still be there to answer.
func TestEnqueueBackgroundCommandPromotesAfterWinnerCloses(t *testing.T) {
	d := host.NewDaemon()
	survivor := newProbe(d) // earliest
	winner := newProbe(d)   // the one the hint selects
	d.SetConnBrowser(survivor.rc, "chrome")
	d.SetConnBrowser(winner.rc, "firefox")

	if _, err := d.EnqueueBackgroundCommand(host.Command{Action: "recordingList"}, "firefox"); err != nil {
		t.Fatalf("first command: %v", err)
	}
	if !winner.winner("recordingList") {
		t.Fatal("the hint should have selected the firefox connection")
	}
	if survivor.winner("recordingList") {
		t.Fatal("the chrome connection received a firefox-hinted command")
	}

	// Close the winner, not a bystander.
	d.RemoveConn(winner.rc)

	// The firefox hint now matches nothing, so the documented fallback applies:
	// the earliest surviving connection takes the command.
	if _, err := d.EnqueueBackgroundCommand(host.Command{Action: "recordingList"}, "firefox"); err != nil {
		t.Fatalf("after the winner closed: %v", err)
	}
	if !survivor.winner("recordingList") {
		t.Error("the next-earliest connection did not take over after the winner closed")
	}
}

// TestEnqueueBackgroundCommandFailedWriteReportsError pins that a write failure
// is surfaced to the caller. (The matching cleanup of the pending entry is an
// internal detail, asserted in daemon_background_internal_test.go.)
func TestEnqueueBackgroundCommandFailedWriteReportsError(t *testing.T) {
	d := host.NewDaemon()
	p := newProbe(d)
	p.failWrite = true

	ch, err := d.EnqueueBackgroundCommand(host.Command{Action: "recordingList"}, "")
	if err == nil {
		t.Fatal("want an error when the relay write fails")
	}
	if ch != nil {
		t.Error("want no result channel when the write failed")
	}
	if !strings.Contains(err.Error(), "relay write") {
		t.Errorf("error = %q, want it to mention the relay write", err)
	}
}
