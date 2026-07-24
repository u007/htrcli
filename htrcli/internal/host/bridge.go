package host

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// StartUnixSocketServer listens on socketPath for relay connections.
// Each accepted connection becomes the active relay for the daemon.
// It returns the net.Listener so the caller can close it during shutdown
// (the tray feature's clean-shutdown sequence depends on this). The accept
// loop runs in a background goroutine; StartUnixSocketServer itself returns
// as soon as the listener is bound.
func StartUnixSocketServer(d *Daemon, socketPath string, port int, bearerToken string) (net.Listener, error) {
	if err := ensureSocketParentDir(socketPath); err != nil {
		return nil, fmt.Errorf("create socket dir: %w", err)
	}
	// Only clear a STALE socket file. If another daemon is actively
	// accepting on it, bail out instead of unlinking it from under them.
	if probe, err := net.DialTimeout("unix", socketPath, time.Second); err == nil {
		probe.Close()
		return nil, fmt.Errorf("another daemon is already accepting on %s", socketPath)
	}
	os.Remove(socketPath)
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("listen unix %s: %w", socketPath, err)
	}

	go func() {
		// Remove the socket file when the listener is closed (shutdown).
		defer os.Remove(socketPath)
		for {
			conn, err := ln.Accept()
			if err != nil {
				// Listener closed during shutdown — exit the loop.
				return
			}
			go handleRelayConn(d, conn, port, bearerToken)
		}
	}()

	return ln, nil
}

func ensureSocketParentDir(socketPath string) error {
	return os.MkdirAll(filepath.Dir(socketPath), 0700)
}

func handleRelayConn(d *Daemon, conn net.Conn, port int, bearerToken string) {
	defer conn.Close()

	// Each relay connection is one browser. Scope its tabs to this connection
	// so commands route to the right browser, and drop only this connection's
	// tabs when it disconnects (leaving other browsers working).
	rc := d.AddConn(func(msg []byte) error {
		return WriteMessage(conn, msg)
	})
	d.SetConnCloser(rc, conn.Close)
	defer d.RemoveConn(rc)

	// Greet the relay with an immediate ping. The extension treats the first
	// daemon message as proof the daemon is reachable (connectNative alone
	// succeeds even when it isn't) and only then reports itself connected.
	genPayload, _ := json.Marshal(map[string]any{
		"generation":  d.Events.Generation(),
		"httpBaseUrl": fmt.Sprintf("http://127.0.0.1:%d", port),
		"token":       bearerToken,
	})
	if greeting, err := json.Marshal(NativeMessage{Type: "ping", Payload: genPayload}); err == nil {
		if err := WriteMessage(conn, greeting); err != nil {
			log.Printf("[htrcli serve] greeting ping failed, dropping relay: %v", err)
			return
		}
	}

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
		// Any traffic proves the relay+extension are alive.
		d.TouchConn(rc)
		switch msg.Type {
		case "register":
			var info TabInfo
			if err := json.Unmarshal(msg.Payload, &info); err == nil {
				d.RegisterTab(rc, msg.TabID, info)
			}
		case "command_result":
			var result CommandResult
			if err := json.Unmarshal(msg.Payload, &result); err == nil {
				d.ResolveCommand(result.ID, result)
			}
		case "heartbeat":
			// Liveness reply to the daemon's ping; TouchConn above already
			// refreshed lastSeen, so nothing else to do.
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
		d.mu.Lock()
		delete(d.pending, cmd.ID)
		d.mu.Unlock()
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
