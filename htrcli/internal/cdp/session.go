package cdp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// Session is a synchronous CDP connection (page-level or browser-level).
// htrcli issues one call at a time, so no concurrent-writer handling is needed.
type Session struct {
	conn   *websocket.Conn
	nextID int64
	// events buffered while waiting for a Call's response, FIFO.
	pending []cdpMessage
}

type cdpMessage struct {
	ID     int64           `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  *cdpError       `json:"error"`
}

type cdpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Dial connects to a CDP WebSocket URL. The dialer sends no Origin header —
// Chrome ≥111 rejects unlisted origins (--remote-allow-origins) but accepts
// origin-less connections.
func Dial(wsURL string) (*Session, error) {
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, http.Header{})
	if err != nil {
		return nil, fmt.Errorf("CDP dial %s: %w", wsURL, err)
	}
	return &Session{conn: conn}, nil
}

// Call sends {id, method, params} and blocks until the response with the same
// id arrives. Events read in the meantime are buffered for WaitEvent.
// A CDP error response becomes a Go error. result may be nil.
func (s *Session) Call(method string, params any, result any) error {
	s.nextID++
	id := s.nextID
	req := map[string]any{"id": id, "method": method}
	if params != nil {
		req["params"] = params
	}
	if err := s.conn.WriteJSON(req); err != nil {
		return fmt.Errorf("%s: write: %w", method, err)
	}
	for {
		var msg cdpMessage
		if err := s.conn.ReadJSON(&msg); err != nil {
			return fmt.Errorf("%s: read: %w", method, err)
		}
		if msg.Method != "" { // event
			s.pending = append(s.pending, msg)
			continue
		}
		if msg.ID != id {
			// Response to a stale call (shouldn't happen with sequential use).
			continue
		}
		if msg.Error != nil {
			return fmt.Errorf("%s: CDP error %d: %s", method, msg.Error.Code, msg.Error.Message)
		}
		if result != nil && msg.Result != nil {
			if err := json.Unmarshal(msg.Result, result); err != nil {
				return fmt.Errorf("%s: decode result: %w", method, err)
			}
		}
		return nil
	}
}

// WaitEvent returns the params of the next event named name, checking events
// buffered during earlier Calls first, then reading the socket until timeout.
func (s *Session) WaitEvent(name string, timeout time.Duration) (json.RawMessage, error) {
	for i, msg := range s.pending {
		if msg.Method == name {
			s.pending = append(s.pending[:i], s.pending[i+1:]...)
			return msg.Params, nil
		}
	}
	deadline := time.Now().Add(timeout)
	if err := s.conn.SetReadDeadline(deadline); err != nil {
		return nil, err
	}
	defer func() {
		// intentionally not logged: clearing a deadline cannot meaningfully fail after reads succeeded
		_ = s.conn.SetReadDeadline(time.Time{})
	}()
	for {
		var msg cdpMessage
		if err := s.conn.ReadJSON(&msg); err != nil {
			return nil, fmt.Errorf("waiting for %s: %w", name, err)
		}
		if msg.Method == name {
			return msg.Params, nil
		}
		if msg.Method != "" {
			s.pending = append(s.pending, msg)
		}
	}
}

// Close closes the underlying WebSocket.
func (s *Session) Close() error {
	return s.conn.Close()
}
