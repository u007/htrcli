package cdp

import (
	"errors"
	"testing"

	"github.com/gorilla/websocket"
)

// frameContext replays Runtime.executionContextCreated on every lookup
// (Runtime.disable + enable). The replay must not stay in the event buffer,
// and the other events read during the lookup must be kept.
func TestFrameContextReplayIsNotBuffered(t *testing.T) {
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		switch m.Method {
		case "Runtime.evaluate":
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{"result": map[string]any{
				"type": "object", "className": "HTMLIFrameElement", "objectId": "o1",
			}}})
		case "DOM.describeNode":
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{"node": map[string]any{"frameId": "F1"}}})
		case "Runtime.enable":
			conn.WriteJSON(map[string]any{"method": "Page.frameNavigated", "params": map[string]any{}})
			// Enable also replays stored console messages and exceptions.
			conn.WriteJSON(map[string]any{"method": "Runtime.consoleAPICalled", "params": map[string]any{"type": "log"}})
			conn.WriteJSON(map[string]any{"method": "Runtime.exceptionThrown", "params": map[string]any{}})
			conn.WriteJSON(map[string]any{"method": "Runtime.executionContextCreated", "params": map[string]any{
				"context": map[string]any{"id": 7, "auxData": map[string]any{"frameId": "F1", "isDefault": true}},
			}})
			conn.WriteJSON(map[string]any{"method": "Runtime.executionContextCreated", "params": map[string]any{
				"context": map[string]any{"id": 8, "auxData": map[string]any{"frameId": "F2", "isDefault": true}},
			}})
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
		default:
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
		}
	})
	s, err := Dial(url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer s.Close()
	s.frame = "iframe"

	for round := 0; round < 2; round++ {
		s.frameContextID = 0
		ctx, err := s.frameContext()
		if err != nil {
			t.Fatalf("round %d: frameContext: %v", round, err)
		}
		if ctx != 7 {
			t.Fatalf("round %d: context = %d, want 7", round, ctx)
		}
	}
	replayed, other := 0, 0
	for _, msg := range s.pending {
		switch msg.Method {
		case "Runtime.executionContextCreated", "Runtime.consoleAPICalled", "Runtime.exceptionThrown":
			replayed++
		case "Page.frameNavigated":
			other++
		}
	}
	if replayed != 0 {
		t.Errorf("replayed Runtime events left in buffer: %d", replayed)
	}
	if other != 2 {
		t.Errorf("unrelated events kept = %d, want 2 (one per lookup)", other)
	}
}

func TestIsStaleContextErr(t *testing.T) {
	stale := []string{
		"Runtime.callFunctionOn: CDP error -32000: Cannot find context with specified id",
		"Runtime.evaluate: CDP error -32000: Cannot find default execution context",
	}
	for _, msg := range stale {
		if !isStaleContextErr(errors.New(msg)) {
			t.Errorf("%q must be treated as a stale context", msg)
		}
	}
	// Destroyed mid-expression: input may already have been dispatched.
	if isStaleContextErr(errors.New("Runtime.evaluate: CDP error -32000: Execution context was destroyed.")) {
		t.Error("a context destroyed mid-expression must not be retried (could repeat a click)")
	}
	if isStaleContextErr(errors.New("Runtime.evaluate: CDP error -32000: Uncaught ReferenceError: x is not defined")) {
		t.Error("a page exception must not be retried as a stale context")
	}
}

func TestDropEventsFrom(t *testing.T) {
	buf := []cdpMessage{
		{Method: "Runtime.executionContextCreated"},
		{Method: "Console.messageAdded"},
		{Method: "Runtime.executionContextCreated"},
		{Method: "Page.frameNavigated"},
	}
	got := dropEventsFrom(buf, 1, "Runtime.executionContextCreated")
	if len(got) != 3 || got[0].Method != "Runtime.executionContextCreated" || got[1].Method != "Console.messageAdded" || got[2].Method != "Page.frameNavigated" {
		t.Fatalf("dropEventsFrom = %+v", got)
	}
	if len(buf) != 4 || buf[2].Method != "Runtime.executionContextCreated" {
		t.Fatalf("input buffer was modified: %+v", buf)
	}
}
