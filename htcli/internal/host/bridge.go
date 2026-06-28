package host

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// StartUnixSocketServer listens on socketPath for relay connections.
// Each accepted connection becomes the active relay for the daemon.
// Blocks until an error occurs.
func StartUnixSocketServer(d *Daemon, socketPath string) error {
	if err := ensureSocketParentDir(socketPath); err != nil {
		return fmt.Errorf("create socket dir: %w", err)
	}
	os.Remove(socketPath)
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("listen unix %s: %w", socketPath, err)
	}
	defer func() {
		ln.Close()
		os.Remove(socketPath)
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go handleRelayConn(d, conn)
	}
}

func ensureSocketParentDir(socketPath string) error {
	return os.MkdirAll(filepath.Dir(socketPath), 0700)
}

func handleRelayConn(d *Daemon, conn net.Conn) {
	defer conn.Close()

	d.SetRelay(func(msg []byte) error {
		return WriteMessage(conn, msg)
	})
	defer d.SetRelay(nil)

	// Read results from relay (extension responses)
	for {
		raw, err := ReadMessage(conn)
		if err != nil {
			return
		}
		var msg NativeMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		switch msg.Type {
		case "register":
			var info TabInfo
			if err := json.Unmarshal(msg.Payload, &info); err == nil {
				d.RegisterTab(msg.TabID, info)
			}
		case "command_result":
			var result CommandResult
			if err := json.Unmarshal(msg.Payload, &result); err == nil {
				d.ResolveCommand(result.ID, result)
			}
		case "heartbeat":
			// no-op, keeps connection alive
		}
	}
}

// sendCommand sends a command to a tab and waits for the result.
// timeout is in milliseconds.
func sendCommand(d *Daemon, tabID int, cmd Command, timeoutMs int) (*CommandResult, error) {
	ch, err := d.EnqueueCommand(tabID, cmd)
	if err != nil {
		return nil, err
	}
	timer := time.NewTimer(time.Duration(timeoutMs) * time.Millisecond)
	defer timer.Stop()
	select {
	case result := <-ch:
		return &result, nil
	case <-timer.C:
		return nil, fmt.Errorf("command timed out after %dms", timeoutMs)
	}
}

func generateID() string {
	return fmt.Sprintf("cmd-%d", time.Now().UnixNano())
}

func jsonResponse(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

func apiOK(w http.ResponseWriter, data any) {
	jsonResponse(w, 200, map[string]any{"ok": true, "data": data})
}

func apiError(w http.ResponseWriter, status int, msg string) {
	jsonResponse(w, status, map[string]any{"ok": false, "error": msg})
}
