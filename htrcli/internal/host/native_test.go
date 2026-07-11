package host_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/u007/htrcli/internal/host"
)

func TestWriteMessage(t *testing.T) {
	var buf bytes.Buffer
	payload := []byte(`{"type":"ping"}`)
	if err := host.WriteMessage(&buf, payload); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}

	// first 4 bytes = little-endian length
	var length uint32
	binary.Read(bytes.NewReader(buf.Bytes()[:4]), binary.LittleEndian, &length)
	if int(length) != len(payload) {
		t.Errorf("length prefix = %d, want %d", length, len(payload))
	}
	if string(buf.Bytes()[4:]) != string(payload) {
		t.Errorf("body = %q, want %q", buf.Bytes()[4:], payload)
	}
}

func TestReadMessage(t *testing.T) {
	payload := []byte(`{"type":"pong"}`)
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, uint32(len(payload)))
	buf.Write(payload)

	got, err := host.ReadMessage(&buf)
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if string(got) != string(payload) {
		t.Errorf("got %q, want %q", got, payload)
	}
}

func TestReadMessageEOF(t *testing.T) {
	_, err := host.ReadMessage(bytes.NewReader(nil))
	if err == nil {
		t.Error("expected error on empty reader, got nil")
	}
}

// A command result carrying a large fetch body / page HTML exceeds 1 MB and
// must survive the relay (extension→daemon direction), not be rejected.
func TestReadMessageLargePayload(t *testing.T) {
	payload := bytes.Repeat([]byte("a"), 2<<20) // 2 MB
	var buf bytes.Buffer
	if err := host.WriteMessage(&buf, payload); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}
	got, err := host.ReadMessage(&buf)
	if err != nil {
		t.Fatalf("ReadMessage on 2 MB payload: %v", err)
	}
	if len(got) != len(payload) {
		t.Errorf("got %d bytes, want %d", len(got), len(payload))
	}
}

// A corrupt/oversized length prefix (e.g. the 538 MB stdout-leak bug) must
// still be rejected rather than triggering a giant allocation.
func TestReadMessageRejectsOversizedLength(t *testing.T) {
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, uint32(host.MaxMessageSize+1))
	if _, err := host.ReadMessage(&buf); err == nil {
		t.Error("expected error on oversized length prefix, got nil")
	}
}
