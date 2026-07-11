package host_test

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/u007/htrcli/internal/host"
)

// TestRelayForwardsStdinToSocket verifies that a message written to the relay's
// stdin arrives on the daemon Unix socket.
func TestRelayForwardsStdinToSocket(t *testing.T) {
	// Start a mock Unix socket server
	ln, err := net.Listen("unix", t.TempDir()+"/test.sock")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	payload := []byte(`{"type":"heartbeat","tabId":1}`)
	var stdinBuf bytes.Buffer
	binary.Write(&stdinBuf, binary.LittleEndian, uint32(len(payload)))
	stdinBuf.Write(payload)
	// EOF triggers relay exit
	stdin := bytes.NewReader(stdinBuf.Bytes())

	received := make(chan []byte, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		msg, err := host.ReadMessage(conn)
		if err == nil {
			received <- msg
		}
	}()

	done := make(chan error, 1)
	go func() {
		done <- host.RunRelayWithIO(stdin, &bytes.Buffer{}, ln.Addr().String())
	}()

	select {
	case msg := <-received:
		var got map[string]interface{}
		json.Unmarshal(msg, &got)
		if got["type"] != "heartbeat" {
			t.Errorf("type = %v, want heartbeat", got["type"])
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout: relay did not forward message to socket")
	}
}
