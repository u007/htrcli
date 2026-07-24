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
