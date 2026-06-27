package host

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

// NativeMessage is the envelope for all messages between the relay and daemon.
type NativeMessage struct {
	Type      string          `json:"type"`
	TabID     int             `json:"tabId,omitempty"`
	CommandID string          `json:"commandId,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

// ReadMessage reads one Chrome Native Messaging framed message from r.
// Format: 4-byte little-endian uint32 length + that many bytes of JSON.
func ReadMessage(r io.Reader) ([]byte, error) {
	var length uint32
	if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
		return nil, fmt.Errorf("reading length: %w", err)
	}
	if length == 0 || length > 1<<20 {
		return nil, fmt.Errorf("invalid message length: %d", length)
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("reading body: %w", err)
	}
	return buf, nil
}

// WriteMessage writes one Chrome Native Messaging framed message to w.
func WriteMessage(w io.Writer, data []byte) error {
	if err := binary.Write(w, binary.LittleEndian, uint32(len(data))); err != nil {
		return fmt.Errorf("writing length: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("writing body: %w", err)
	}
	return nil
}
