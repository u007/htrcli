package host_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/u007/htcli/internal/host"
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
