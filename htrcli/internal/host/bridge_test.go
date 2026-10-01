package host

import (
	"encoding/json"
	"net"
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

func TestRegisterTabPreservesBrowserCapability(t *testing.T) {
	d := NewDaemon()
	rc := d.AddConn(func(_ []byte) error { return nil })
	d.RegisterTab(rc, 7, TabInfo{
		ID:      7,
		URL:     "https://example.com",
		Browser: "firefox",
	})

	tabs := d.Tabs()
	if len(tabs) != 1 {
		t.Fatalf("expected one registered tab, got %d", len(tabs))
	}
	if tabs[0].Browser != "firefox" {
		t.Fatalf("browser = %q, want firefox", tabs[0].Browser)
	}
}

func TestGreetingIncludesGenerationAndConnectionInfo(t *testing.T) {
	d := NewDaemon()
	server, client := net.Pipe()
	defer client.Close()

	go handleRelayConn(d, server, 3845, "secret-token")

	msg, err := ReadMessage(client)
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	var nm NativeMessage
	if err := json.Unmarshal(msg, &nm); err != nil {
		t.Fatalf("unmarshal greeting: %v", err)
	}
	if nm.Type != "ping" {
		t.Fatalf("want type ping, got %s", nm.Type)
	}
	var payload map[string]any
	if err := json.Unmarshal(nm.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if _, ok := payload["generation"]; !ok {
		t.Fatalf("expected generation in greeting payload, got %+v", payload)
	}
	if payload["httpBaseUrl"] != "http://127.0.0.1:3845" {
		t.Fatalf("expected httpBaseUrl in payload, got %+v", payload)
	}
	if payload["token"] != "secret-token" {
		t.Fatalf("expected token in payload, got %+v", payload)
	}
}

// relayForTest starts a relay connection on a pipe pair and returns the client
// end, so a test can speak the native-messaging wire protocol to the daemon.
func relayForTest(t *testing.T, d *Daemon) net.Conn {
	t.Helper()
	server, client := net.Pipe()
	t.Cleanup(func() { client.Close() })
	go handleRelayConn(d, server, 3845, "secret-token")

	// The daemon greets every new relay with a ping; drain it so the pipe is
	// not holding an unread frame when the test sends its own message.
	if _, err := ReadMessage(client); err != nil {
		t.Fatalf("reading the greeting: %v", err)
	}
	return client
}

func sendNative(t *testing.T, conn net.Conn, msg NativeMessage) {
	t.Helper()
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal outbound message: %v", err)
	}
	if err := WriteMessage(conn, data); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}
}

// waitFor polls until cond holds or the deadline passes. Polling avoids a sleep
// in the passing case while still bounding the failing one.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestIdentifyRecordsBrowserOnConnection(t *testing.T) {
	d := NewDaemon()
	conn := relayForTest(t, d)

	browser, err := json.Marshal(map[string]string{"browser": "firefox"})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	sendNative(t, conn, NativeMessage{Type: "identify", Payload: browser})

	var got string
	waitFor(t, "the relay connection to record its browser", func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		for rc := range d.conns {
			got = rc.browser
			return rc.browser != ""
		}
		return false
	})
	if got != "firefox" {
		t.Errorf("recorded browser = %q, want firefox", got)
	}
}

// An extension is not a trusted source: junk in an identify must not be able to
// disconnect the relay or fail the connection. An empty value, an absent value,
// and a browser this build does not know are all simply "not announced" — none of
// them may be stored, because a stored bogus value would be selectable as a
// background-command target.
func TestIdentifyIgnoresEmptyAndUnknownBrowser(t *testing.T) {
	for _, payload := range []string{`{}`, `{"browser":""}`, `{"browser":"netscape"}`, `{"browser":123}`} {
		t.Run(payload, func(t *testing.T) {
			d := NewDaemon()
			conn := relayForTest(t, d)

			raw, err := json.Marshal(NativeMessage{Type: "identify", Payload: json.RawMessage(payload)})
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if err := WriteMessage(conn, raw); err != nil {
				t.Fatalf("WriteMessage: %v", err)
			}

			// Prove the relay survived: a heartbeat after the junk must be accepted.
			hb, err := json.Marshal(NativeMessage{Type: "heartbeat"})
			if err != nil {
				t.Fatalf("marshal heartbeat: %v", err)
			}
			if err := WriteMessage(conn, hb); err != nil {
				t.Fatalf("relay did not survive the junk identify: %v", err)
			}

			waitFor(t, "the relay connection to still be connected", func() bool {
				return d.RelaysConnected() == 1
			})

			d.mu.Lock()
			defer d.mu.Unlock()
			for rc := range d.conns {
				if rc.browser != "" {
					t.Errorf("browser = %q, want it left empty for payload %s", rc.browser, payload)
				}
			}
		})
	}
}

// A completely unparseable frame is dropped before the type switch, but it must
// not tear the relay down either — the read loop continues to the next message.
func TestIdentifyGarbageFrameDoesNotDropRelay(t *testing.T) {
	d := NewDaemon()
	conn := relayForTest(t, d)

	if err := WriteMessage(conn, []byte("this is not json")); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}

	sendNative(t, conn, NativeMessage{Type: "identify",
		Payload: json.RawMessage(`{"browser":"chrome"}`)})

	waitFor(t, "the relay to still record identify after a garbage frame", func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		for rc := range d.conns {
			return rc.browser == "chrome"
		}
		return false
	})
}
