package cdp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"
)

// ErrEventDeadline is returned by NextEvent when the deadline passes before
// another event arrives. It is the normal end of a bounded watch.
var ErrEventDeadline = errors.New("CDP event deadline reached")

// NextEvent returns the next CDP event of any method: events buffered during
// earlier Calls first (FIFO), then events read from the socket until deadline.
// Responses to calls are ignored here (callers do not Call while streaming).
//
// After ErrEventDeadline the session is unusable: gorilla/websocket treats a
// read timeout as a permanent connection error, so every later read or Call
// fails. Only use NextEvent with a deadline as the last thing a session does.
func (s *Session) NextEvent(deadline time.Time) (string, json.RawMessage, error) {
	if len(s.pending) > 0 {
		msg := s.pending[0]
		s.pending = s.pending[1:]
		return msg.Method, msg.Params, nil
	}
	if err := s.conn.SetReadDeadline(deadline); err != nil {
		return "", nil, fmt.Errorf("setting CDP read deadline: %w", err)
	}
	for {
		var msg cdpMessage
		if err := s.conn.ReadJSON(&msg); err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				return "", nil, ErrEventDeadline
			}
			return "", nil, fmt.Errorf("reading CDP event: %w", err)
		}
		if msg.Method != "" {
			return msg.Method, msg.Params, nil
		}
	}
}
