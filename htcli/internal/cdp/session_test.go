package cdp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type fakeMsg struct {
	ID     int64           `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

// fakeCDP answers every call with handler(method) and pushes pre/post events.
func fakeCDP(t *testing.T, handler func(m fakeMsg, conn *websocket.Conn)) string {
	t.Helper()
	up := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") != "" {
			t.Errorf("client sent Origin header %q — must send none", r.Header.Get("Origin"))
		}
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()
		for {
			var m fakeMsg
			if err := conn.ReadJSON(&m); err != nil {
				return // intentionally not logged: client closing the socket ends the fake server loop
			}
			handler(m, conn)
		}
	}))
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

func TestCallRoundTrip(t *testing.T) {
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{"value": 42}})
	})
	s, err := Dial(url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer s.Close()

	var res struct {
		Value int `json:"value"`
	}
	if err := s.Call("Runtime.evaluate", map[string]any{"expression": "6*7"}, &res); err != nil {
		t.Fatalf("call: %v", err)
	}
	if res.Value != 42 {
		t.Fatalf("want 42, got %d", res.Value)
	}
}

func TestCallSkipsAndBuffersEvents(t *testing.T) {
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		// Event arrives BEFORE the call's response.
		conn.WriteJSON(map[string]any{"method": "Page.loadEventFired", "params": map[string]any{"timestamp": 1}})
		conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
	})
	s, err := Dial(url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer s.Close()

	if err := s.Call("Page.navigate", map[string]any{"url": "https://x/"}, nil); err != nil {
		t.Fatalf("call: %v", err)
	}

	// The event buffered during Call must be returned without reading the socket.
	params, err := s.WaitEvent("Page.loadEventFired", time.Second)
	if err != nil {
		t.Fatalf("waitevent: %v", err)
	}
	if !strings.Contains(string(params), "timestamp") {
		t.Fatalf("got params %s", params)
	}
}

func TestCallCDPError(t *testing.T) {
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		conn.WriteJSON(map[string]any{"id": m.ID, "error": map[string]any{"code": -32000, "message": "Cannot find context"}})
	})
	s, err := Dial(url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer s.Close()

	err = s.Call("Runtime.evaluate", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "Cannot find context") {
		t.Fatalf("want CDP error surfaced, got %v", err)
	}
}

func TestWaitEventTimeout(t *testing.T) {
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {})
	s, err := Dial(url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer s.Close()

	if _, err := s.WaitEvent("Page.loadEventFired", 50*time.Millisecond); err == nil {
		t.Fatal("want timeout error")
	}
}
