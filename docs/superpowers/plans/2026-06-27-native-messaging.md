# Native Messaging Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Chrome Native Messaging as the primary connection path between htcli and the extension, with automatic fallback to the existing WebSocket server.

**Architecture:** Chrome spawns a thin ephemeral relay binary (htcli in relay mode) that bridges its stdin/stdout to a persistent `htcli serve` daemon via a Unix socket. The daemon also serves HTTP on :3845 so existing CLI commands need zero changes. The extension tries `connectNative()` first; on failure it falls back to the existing `wsClient.ts` WebSocket path.

**Tech Stack:** Go 1.22 (htcli), TypeScript/MV3 (extension), Bun (existing server, unchanged), cobra (CLI), net/http (Go HTTP), net (Go Unix sockets)

## Global Constraints

- Go module: `github.com/u007/htcli` — all new Go files use this module path
- Relay mode detected via `os.Args[1]` having `chrome-extension://` prefix (Chrome passes origin as first arg)
- Unix socket path: `~/.htcli/daemon.sock`
- Native host name: `com.howtorecorder.host`
- HTTP port: `3845`, same `HTR_*` env vars as Bun server
- Native messaging framing: 4-byte little-endian uint32 length prefix + JSON body (Chrome protocol)
- `wsClient.ts` must NOT be modified except to export a `setTabId(id: number)` setter
- All new Go code must have unit tests in `_test.go` files in the same package
- Run tests with: `cd htcli && go test ./...`
- Build with: `cd htcli && go build ./cmd/htcli`

---

## File Map

**New Go files:**
- `htcli/internal/host/native.go` — 4-byte NM framing read/write + shared message types
- `htcli/internal/host/relay.go` — relay mode: stdin/stdout ↔ Unix socket
- `htcli/internal/host/daemon.go` — daemon: Unix socket server + tab/command state
- `htcli/internal/host/server.go` — daemon: HTTP server on :3845
- `htcli/internal/host/bridge.go` — routes HTTP commands → daemon state → relay → response
- `htcli/internal/commands/serve.go` — `htcli serve` cobra command
- `htcli/internal/commands/install.go` — `htcli install` cobra command
- `htcli/internal/host/native_test.go` — NM framing tests
- `htcli/internal/host/relay_test.go` — relay tests
- `htcli/internal/host/daemon_test.go` — daemon state tests

**Modified Go files:**
- `htcli/cmd/htcli/main.go` — detect relay mode, dispatch before cobra

**New extension files:**
- `src/background/nativeHost.ts` — owns native port, reconnection, command dispatch
- `src/contentScript/connectionManager.ts` — auto-detects native vs WS, unified interface

**Modified extension files:**
- `src/contentScript/wsClient.ts` — export `setTabId(id: number)` setter
- `src/contentScript/index.ts` — use `connectionManager` instead of `wsClient` directly
- `src/background/index.ts` — register native host handlers + `GET_TAB_ID` handler
- `src/manifest.ts` — add `"nativeMessaging"` permission

---

## Task 1: NM framing protocol + shared types

**Files:**
- Create: `htcli/internal/host/native.go`
- Create: `htcli/internal/host/native_test.go`

**Interfaces:**
- Produces:
  - `ReadMessage(r io.Reader) ([]byte, error)` — reads one NM-framed message
  - `WriteMessage(w io.Writer, data []byte) error` — writes one NM-framed message
  - `NativeMessage` struct — shared envelope for all extension↔daemon messages

- [x] **Step 1: Write the failing tests**

```go
// htcli/internal/host/native_test.go
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
```

- [x] **Step 2: Run tests to confirm failure**

```bash
cd /Users/james/www/how-to-recorder/htcli && go test ./internal/host/... 2>&1
```
Expected: `cannot find package` or `undefined: host.WriteMessage`

- [x] **Step 3: Implement native.go**

```go
// htcli/internal/host/native.go
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
```

- [x] **Step 4: Run tests to confirm pass**

```bash
cd /Users/james/www/how-to-recorder/htcli && go test ./internal/host/... -v 2>&1
```
Expected: `PASS` for all three tests

- [x] **Step 5: Commit**

```bash
cd /Users/james/www/how-to-recorder && git add htcli/internal/host/native.go htcli/internal/host/native_test.go && git commit -m "feat(htcli): add NM framing protocol (native.go)"
```

---

## Task 2: Daemon state — tab registry + pending commands

**Files:**
- Create: `htcli/internal/host/daemon.go`
- Create: `htcli/internal/host/daemon_test.go`

**Interfaces:**
- Consumes: `NativeMessage` from `host` package (Task 1)
- Produces:
  - `NewDaemon() *Daemon`
  - `(d *Daemon) RegisterTab(tabID int, info TabInfo)`
  - `(d *Daemon) RemoveTab(tabID int)`
  - `(d *Daemon) Tabs() []TabInfo`
  - `(d *Daemon) FirstTabID() (int, bool)`
  - `(d *Daemon) EnqueueCommand(tabID int, cmd Command) (<-chan CommandResult, error)`
  - `(d *Daemon) ResolveCommand(commandID string, result CommandResult)`
  - `TabInfo` struct: `{ ID int; URL string; Title string; Active bool }`
  - `Command` struct: `{ ID string; Action string; Value string; Target json.RawMessage; Options json.RawMessage }`
  - `CommandResult` struct: `{ ID string; Success bool; Data json.RawMessage; Error string; Duration int }`

- [x] **Step 1: Write the failing tests**

```go
// htcli/internal/host/daemon_test.go
package host_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/u007/htcli/internal/host"
)

func TestDaemonTabRegistry(t *testing.T) {
	d := host.NewDaemon()

	d.RegisterTab(1, host.TabInfo{ID: 1, URL: "https://a.com", Title: "A", Active: true})
	d.RegisterTab(2, host.TabInfo{ID: 2, URL: "https://b.com", Title: "B", Active: false})

	tabs := d.Tabs()
	if len(tabs) != 2 {
		t.Fatalf("want 2 tabs, got %d", len(tabs))
	}

	d.RemoveTab(1)
	if len(d.Tabs()) != 1 {
		t.Fatalf("want 1 tab after removal, got %d", len(d.Tabs()))
	}

	id, ok := d.FirstTabID()
	if !ok || id != 2 {
		t.Errorf("FirstTabID = %d, %v; want 2, true", id, ok)
	}
}

func TestDaemonEnqueueAndResolve(t *testing.T) {
	d := host.NewDaemon()
	d.RegisterTab(1, host.TabInfo{ID: 1, URL: "https://a.com", Title: "A", Active: true})

	ch, err := d.EnqueueCommand(1, host.Command{ID: "cmd-1", Action: "navigate", Value: "https://x.com"})
	if err != nil {
		t.Fatalf("EnqueueCommand: %v", err)
	}

	result := host.CommandResult{ID: "cmd-1", Success: true, Duration: 42}
	go func() {
		time.Sleep(10 * time.Millisecond)
		d.ResolveCommand("cmd-1", result)
	}()

	select {
	case got := <-ch:
		if got.ID != "cmd-1" || !got.Success {
			t.Errorf("unexpected result: %+v", got)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("timeout waiting for command result")
	}
}

func TestDaemonEnqueueTabNotFound(t *testing.T) {
	d := host.NewDaemon()
	_, err := d.EnqueueCommand(99, host.Command{ID: "cmd-x", Action: "navigate"})
	if err == nil {
		t.Error("expected error for missing tab, got nil")
	}
}

func TestCommandJSON(t *testing.T) {
	cmd := host.Command{ID: "c1", Action: "click"}
	b, _ := json.Marshal(cmd)
	var got host.Command
	json.Unmarshal(b, &got)
	if got.ID != "c1" || got.Action != "click" {
		t.Errorf("round-trip failed: %+v", got)
	}
}
```

- [x] **Step 2: Run tests to confirm failure**

```bash
cd /Users/james/www/how-to-recorder/htcli && go test ./internal/host/... 2>&1
```
Expected: `undefined: host.NewDaemon`

- [x] **Step 3: Implement daemon.go**

```go
// htcli/internal/host/daemon.go
package host

import (
	"encoding/json"
	"fmt"
	"sync"
)

// TabInfo describes a connected browser tab.
type TabInfo struct {
	ID     int    `json:"id"`
	URL    string `json:"url"`
	Title  string `json:"title"`
	Active bool   `json:"active"`
}

// Command is a browser action to execute.
type Command struct {
	ID      string          `json:"id"`
	Action  string          `json:"action"`
	Value   string          `json:"value,omitempty"`
	Target  json.RawMessage `json:"target,omitempty"`
	Options json.RawMessage `json:"options,omitempty"`
}

// CommandResult is the response from a completed command.
type CommandResult struct {
	ID       string          `json:"id"`
	Success  bool            `json:"success"`
	Data     json.RawMessage `json:"data,omitempty"`
	Error    string          `json:"error,omitempty"`
	Duration int             `json:"duration,omitempty"`
}

type pendingCommand struct {
	tabID int
	ch    chan CommandResult
}

// Daemon holds shared state: tab registry and pending command map.
type Daemon struct {
	mu       sync.Mutex
	tabs     map[int]TabInfo
	pending  map[string]*pendingCommand
	// relay is the write function for the active relay connection.
	// Set by the Unix socket server when a relay connects; nil when none.
	relay    func(msg []byte) error
}

// NewDaemon creates an empty Daemon.
func NewDaemon() *Daemon {
	return &Daemon{
		tabs:    make(map[int]TabInfo),
		pending: make(map[string]*pendingCommand),
	}
}

// RegisterTab records a connected tab.
func (d *Daemon) RegisterTab(tabID int, info TabInfo) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.tabs[tabID] = info
}

// RemoveTab removes a tab from the registry.
func (d *Daemon) RemoveTab(tabID int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.tabs, tabID)
}

// Tabs returns a snapshot of connected tabs.
func (d *Daemon) Tabs() []TabInfo {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]TabInfo, 0, len(d.tabs))
	for _, t := range d.tabs {
		out = append(out, t)
	}
	return out
}

// FirstTabID returns the first available tab ID, or false if none.
func (d *Daemon) FirstTabID() (int, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for id := range d.tabs {
		return id, true
	}
	return 0, false
}

// SetRelay sets the write function for the current relay connection.
func (d *Daemon) SetRelay(fn func([]byte) error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.relay = fn
}

// EnqueueCommand sends a command to the relay for a specific tab.
// Returns a channel that receives the result when the extension responds.
func (d *Daemon) EnqueueCommand(tabID int, cmd Command) (<-chan CommandResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, ok := d.tabs[tabID]; !ok {
		return nil, fmt.Errorf("tab %d not connected", tabID)
	}

	ch := make(chan CommandResult, 1)
	d.pending[cmd.ID] = &pendingCommand{tabID: tabID, ch: ch}

	if d.relay != nil {
		msg := NativeMessage{
			Type:    "command",
			TabID:   tabID,
			Payload: mustMarshal(cmd),
		}
		data, _ := json.Marshal(msg)
		if err := d.relay(data); err != nil {
			delete(d.pending, cmd.ID)
			return nil, fmt.Errorf("relay write: %w", err)
		}
	}

	return ch, nil
}

// ResolveCommand delivers a command result to the waiting caller.
func (d *Daemon) ResolveCommand(commandID string, result CommandResult) {
	d.mu.Lock()
	p, ok := d.pending[commandID]
	if ok {
		delete(d.pending, commandID)
	}
	d.mu.Unlock()

	if ok {
		p.ch <- result
	}
}

func mustMarshal(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
```

- [x] **Step 4: Run tests to confirm pass**

```bash
cd /Users/james/www/how-to-recorder/htcli && go test ./internal/host/... -v 2>&1
```
Expected: all tests PASS

- [x] **Step 5: Commit**

```bash
cd /Users/james/www/how-to-recorder && git add htcli/internal/host/daemon.go htcli/internal/host/daemon_test.go && git commit -m "feat(htcli): add daemon state (tab registry + pending commands)"
```

---

## Task 3: Relay mode

**Files:**
- Create: `htcli/internal/host/relay.go`
- Create: `htcli/internal/host/relay_test.go`

**Interfaces:**
- Consumes: `ReadMessage`, `WriteMessage` (Task 1)
- Produces: `RunRelay()` — blocks until stdin closes or socket disconnects

- [x] **Step 1: Write the failing test**

```go
// htcli/internal/host/relay_test.go
package host_test

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/u007/htcli/internal/host"
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
```

- [x] **Step 2: Run test to confirm failure**

```bash
cd /Users/james/www/how-to-recorder/htcli && go test ./internal/host/... -run TestRelayForwards 2>&1
```
Expected: `undefined: host.RunRelayWithIO`

- [x] **Step 3: Implement relay.go**

```go
// htcli/internal/host/relay.go
package host

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
)

const DefaultSocketPath = "/.htcli/daemon.sock"

// RunRelay is the entry point when Chrome spawns htcli as a native host.
// It connects to the daemon Unix socket and bridges stdin/stdout to it.
func RunRelay() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}
	return RunRelayWithIO(os.Stdin, os.Stdout, home+DefaultSocketPath)
}

// RunRelayWithIO is the testable core of RunRelay.
func RunRelayWithIO(stdin io.Reader, stdout io.Writer, socketPath string) error {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		writeErrorToChrome(stdout, "daemon not running: "+err.Error())
		return fmt.Errorf("dial daemon: %w", err)
	}
	defer conn.Close()

	errc := make(chan error, 2)

	// stdin → socket
	go func() {
		for {
			msg, err := ReadMessage(stdin)
			if err != nil {
				errc <- err
				return
			}
			if err := WriteMessage(conn, msg); err != nil {
				errc <- err
				return
			}
		}
	}()

	// socket → stdout
	go func() {
		for {
			msg, err := ReadMessage(conn)
			if err != nil {
				errc <- err
				return
			}
			if err := WriteMessage(stdout, msg); err != nil {
				errc <- err
				return
			}
		}
	}()

	<-errc
	return nil
}

func writeErrorToChrome(w io.Writer, msg string) {
	data, _ := json.Marshal(map[string]string{"type": "error", "error": msg})
	WriteMessage(w, data) //nolint:errcheck
}
```

- [x] **Step 4: Run test to confirm pass**

```bash
cd /Users/james/www/how-to-recorder/htcli && go test ./internal/host/... -v 2>&1
```
Expected: all tests PASS

- [x] **Step 5: Commit**

```bash
cd /Users/james/www/how-to-recorder && git add htcli/internal/host/relay.go htcli/internal/host/relay_test.go && git commit -m "feat(htcli): add relay mode (stdin/stdout ↔ Unix socket)"
```

---

## Task 4: Daemon HTTP server + bridge

**Files:**
- Create: `htcli/internal/host/server.go`
- Create: `htcli/internal/host/bridge.go`

**Interfaces:**
- Consumes: `*Daemon`, `Command`, `CommandResult`, `TabInfo` (Task 2)
- Produces:
  - `NewHTTPServer(d *Daemon, port int, bearerToken string, allowedIPs []string) *http.Server`
  - `StartUnixSocketServer(d *Daemon, socketPath string) error` — blocks, handles relay connections

- [x] **Step 1: Write failing test**

```go
// htcli/internal/host/server_test.go  (create this file)
package host_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/u007/htcli/internal/host"
)

func TestHealthEndpoint(t *testing.T) {
	d := host.NewDaemon()
	srv := host.NewHTTPServer(d, 0, "", nil)
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/health")
	if err != nil {
		t.Fatalf("GET /api/health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	if body["ok"] != true {
		t.Errorf("body.ok = %v, want true", body["ok"])
	}
}

func TestTabsEndpoint(t *testing.T) {
	d := host.NewDaemon()
	d.RegisterTab(1, host.TabInfo{ID: 1, URL: "https://a.com", Title: "A", Active: true})
	srv := host.NewHTTPServer(d, 0, "", nil)
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/tabs")
	if err != nil {
		t.Fatalf("GET /api/tabs: %v", err)
	}
	defer resp.Body.Close()

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	tabs := body["data"].([]interface{})
	if len(tabs) != 1 {
		t.Errorf("want 1 tab, got %d", len(tabs))
	}
}

func TestBearerTokenEnforced(t *testing.T) {
	d := host.NewDaemon()
	srv := host.NewHTTPServer(d, 0, "secret-token", nil)
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	// No token → 401
	resp, _ := http.Get(ts.URL + "/api/health")
	if resp.StatusCode != 401 {
		t.Errorf("no token: status = %d, want 401", resp.StatusCode)
	}

	// Wrong token → 401
	req, _ := http.NewRequest("GET", ts.URL+"/api/health", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != 401 {
		t.Errorf("wrong token: status = %d, want 401", resp.StatusCode)
	}

	// Correct token → 200
	req, _ = http.NewRequest("GET", ts.URL+"/api/health", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != 200 {
		t.Errorf("correct token: status = %d, want 200", resp.StatusCode)
	}
}

func TestCommandEndpointNoTabs(t *testing.T) {
	d := host.NewDaemon()
	srv := host.NewHTTPServer(d, 0, "", nil)
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	body := `{"command":{"id":"c1","action":"navigate","value":"https://x.com"}}`
	resp, err := http.Post(ts.URL+"/api/command", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/command: %v", err)
	}
	if resp.StatusCode != 404 {
		t.Errorf("status = %d, want 404 (no tabs)", resp.StatusCode)
	}
}
```

- [x] **Step 2: Run tests to confirm failure**

```bash
cd /Users/james/www/how-to-recorder/htcli && go test ./internal/host/... -run "TestHealth|TestTabs|TestBearer|TestCommand" 2>&1
```
Expected: `undefined: host.NewHTTPServer`

- [x] **Step 3: Implement bridge.go**

```go
// htcli/internal/host/bridge.go
package host

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"
)

// StartUnixSocketServer listens on socketPath for relay connections.
// Each accepted connection becomes the active relay for the daemon.
// Blocks until an error occurs.
func StartUnixSocketServer(d *Daemon, socketPath string) error {
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
```

- [x] **Step 4: Implement server.go**

```go
// htcli/internal/host/server.go
package host

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// NewHTTPServer builds the HTTP server with all API routes.
// bearerToken: if non-empty, all requests must supply "Authorization: Bearer <token>".
// allowedIPs: if non-nil and non-empty, requests from other IPs are rejected.
func NewHTTPServer(d *Daemon, port int, bearerToken string, allowedIPs []string) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/api/", authMiddleware(bearerToken, allowedIPs, apiHandler(d)))

	return &http.Server{
		Addr:         addrFromPort(port),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
	}
}

func addrFromPort(port int) string {
	if port == 0 {
		return ""
	}
	return fmt.Sprintf("127.0.0.1:%d", port)
}

func authMiddleware(token string, allowedIPs []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token != "" {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") || strings.TrimPrefix(auth, "Bearer ") != token {
				apiError(w, 401, "unauthorized")
				return
			}
		}
		if len(allowedIPs) > 0 {
			remote := r.RemoteAddr
			if idx := strings.LastIndex(remote, ":"); idx >= 0 {
				remote = remote[:idx]
			}
			remote = strings.Trim(remote, "[]")
			allowed := false
			for _, ip := range allowedIPs {
				if ip == remote {
					allowed = true
					break
				}
			}
			if !allowed {
				apiError(w, 403, "forbidden")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

var tabCmdRe = regexp.MustCompile(`^/api/tabs/(\d+)/command$`)
var tabGetRe = regexp.MustCompile(`^/api/tabs/(\d+)$`)

func apiHandler(d *Daemon) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		switch {
		case path == "/api/health" && r.Method == "GET":
			apiOK(w, map[string]any{
				"status":        "running",
				"connectedTabs": len(d.Tabs()),
				"uptime":        0,
			})

		case path == "/api/tabs" && r.Method == "GET":
			apiOK(w, d.Tabs())

		case tabGetRe.MatchString(path) && r.Method == "GET":
			m := tabGetRe.FindStringSubmatch(path)
			id := parseTabID(m[1])
			tabs := d.Tabs()
			for _, t := range tabs {
				if t.ID == id {
					apiOK(w, t)
					return
				}
			}
			apiError(w, 404, "tab not found")

		case path == "/api/command" && r.Method == "POST":
			handleCommand(w, r, d, 0)

		case tabCmdRe.MatchString(path) && r.Method == "POST":
			m := tabCmdRe.FindStringSubmatch(path)
			handleCommand(w, r, d, parseTabID(m[1]))

		default:
			apiError(w, 404, "not found")
		}
	})
}

type commandRequest struct {
	Command Command `json:"command"`
	Timeout int     `json:"timeout"`
}

func handleCommand(w http.ResponseWriter, r *http.Request, d *Daemon, tabID int) {
	var req commandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Command.Action == "" {
		apiError(w, 400, "invalid request body")
		return
	}
	if req.Command.ID == "" {
		req.Command.ID = generateID()
	}
	timeoutMs := req.Timeout
	if timeoutMs <= 0 {
		timeoutMs = 30000
	}

	if tabID == 0 {
		id, ok := d.FirstTabID()
		if !ok {
			apiError(w, 404, "no tabs connected")
			return
		}
		tabID = id
	}

	result, err := sendCommand(d, tabID, req.Command, timeoutMs)
	if err != nil {
		apiError(w, 404, err.Error())
		return
	}
	apiOK(w, result)
}

func parseTabID(s string) int {
	var id int
	fmt.Sscanf(s, "%d", &id)
	return id
}
```

- [x] **Step 5: Add missing fmt import to server.go**

`server.go` uses `fmt.Sprintf` and `fmt.Sscanf` — ensure `"fmt"` is in the import block:

```go
import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)
```

- [x] **Step 6: Run tests to confirm pass**

```bash
cd /Users/james/www/how-to-recorder/htcli && go test ./internal/host/... -v 2>&1
```
Expected: all tests PASS including the four new ones

- [x] **Step 7: Commit**

```bash
cd /Users/james/www/how-to-recorder && git add htcli/internal/host/server.go htcli/internal/host/bridge.go htcli/internal/host/server_test.go && git commit -m "feat(htcli): add daemon HTTP server + bridge"
```

---

## Task 5: `htcli serve` and `htcli install` commands + main.go relay detection

**Files:**
- Create: `htcli/internal/commands/serve.go`
- Create: `htcli/internal/commands/install.go`
- Modify: `htcli/cmd/htcli/main.go`

**Interfaces:**
- Consumes: `host.RunRelay()`, `host.NewDaemon()`, `host.NewHTTPServer()`, `host.StartUnixSocketServer()` (Tasks 1–4)
- Produces: `htcli serve` CLI command, `htcli install [--extension-id] [--uninstall]` CLI command

- [x] **Step 1: Implement serve.go**

```go
// htcli/internal/commands/serve.go
package commands

import (
	"fmt"
	"log"
	"net"
	"os"

	"github.com/spf13/cobra"
	"github.com/u007/htcli/internal/host"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the htcli daemon (native messaging host + HTTP :3845)",
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		socketPath := home + host.DefaultSocketPath

		bearerToken := os.Getenv("HTR_BEARER_TOKEN")
		port := 3845
		if p := os.Getenv("HTR_PORT"); p != "" {
			fmt.Sscanf(p, "%d", &port)
		}

		d := host.NewDaemon()

		// Start Unix socket server (for relay connections from Chrome)
		go func() {
			if err := host.StartUnixSocketServer(d, socketPath); err != nil {
				log.Printf("[htcli serve] Unix socket error: %v", err)
			}
		}()

		// Start HTTP server
		srv := host.NewHTTPServer(d, port, bearerToken, defaultAllowedIPs())
		ln, err := net.Listen("tcp", srv.Addr)
		if err != nil {
			return fmt.Errorf("port %d already in use (Bun server running?): %w", port, err)
		}

		fmt.Printf("[htcli serve] Listening on http://127.0.0.1:%d\n", port)
		fmt.Printf("[htcli serve] Unix socket: %s\n", socketPath)
		if bearerToken == "" {
			fmt.Println("[htcli serve] Warning: no HTR_BEARER_TOKEN set — unauthenticated")
		}

		return srv.Serve(ln)
	},
}

func defaultAllowedIPs() []string {
	if v := os.Getenv("HTR_ALLOWED_IPS"); v != "" {
		var ips []string
		for _, ip := range splitComma(v) {
			if ip != "" {
				ips = append(ips, ip)
			}
		}
		return ips
	}
	return []string{"127.0.0.1", "::1", "localhost"}
}

func splitComma(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}

func init() {
	rootCmd.AddCommand(serveCmd)
}
```

- [x] **Step 2: Implement install.go**

```go
// htcli/internal/commands/install.go
package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

const hostName = "com.howtorecorder.host"

type nativeHostManifest struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Path           string   `json:"path"`
	Type           string   `json:"type"`
	AllowedOrigins []string `json:"allowed_origins"`
}

var (
	installExtensionID string
	installUninstall   bool
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Register htcli as a Chrome Native Messaging host",
	RunE: func(cmd *cobra.Command, args []string) error {
		manifestDir, err := nativeMessagingDir()
		if err != nil {
			return err
		}
		manifestPath := filepath.Join(manifestDir, hostName+".json")

		if installUninstall {
			if err := os.Remove(manifestPath); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove manifest: %w", err)
			}
			fmt.Printf("Removed: %s\n", manifestPath)
			return nil
		}

		if installExtensionID == "" {
			return fmt.Errorf("--extension-id is required\n  Find it at chrome://extensions → Details → Extension ID")
		}

		htcliPath, err := exec.LookPath("htcli")
		if err != nil {
			return fmt.Errorf("htcli not found in PATH: %w", err)
		}
		htcliPath, _ = filepath.Abs(htcliPath)

		manifest := nativeHostManifest{
			Name:        hostName,
			Description: "How-To Recorder native messaging host",
			Path:        htcliPath,
			Type:        "stdio",
			AllowedOrigins: []string{
				"chrome-extension://" + strings.TrimPrefix(installExtensionID, "chrome-extension://") + "/",
			},
		}

		if err := os.MkdirAll(manifestDir, 0755); err != nil {
			return fmt.Errorf("create manifest dir: %w", err)
		}

		data, _ := json.MarshalIndent(manifest, "", "  ")
		if err := os.WriteFile(manifestPath, data, 0644); err != nil {
			return fmt.Errorf("write manifest: %w", err)
		}

		fmt.Printf("Manifest written: %s\n", manifestPath)
		fmt.Printf("htcli path:       %s\n", htcliPath)
		fmt.Printf("Extension ID:     %s\n", installExtensionID)
		fmt.Println("\nReload the extension in Chrome (chrome://extensions → reload button).")
		return nil
	},
}

func nativeMessagingDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Google", "Chrome", "NativeMessagingHosts"), nil
	case "linux":
		return filepath.Join(home, ".config", "google-chrome", "NativeMessagingHosts"), nil
	default:
		return "", fmt.Errorf("unsupported OS for automatic install: %s\n  See: https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging", runtime.GOOS)
	}
}

func init() {
	installCmd.Flags().StringVar(&installExtensionID, "extension-id", "", "Chrome extension ID (from chrome://extensions)")
	installCmd.Flags().BoolVar(&installUninstall, "uninstall", false, "Remove the native host manifest")
	rootCmd.AddCommand(installCmd)
}
```

- [x] **Step 3: Modify main.go for relay mode detection**

```go
// htcli/cmd/htcli/main.go
package main

import (
	"os"
	"strings"

	"github.com/u007/htcli/internal/commands"
	"github.com/u007/htcli/internal/host"
)

func main() {
	// Chrome passes the calling extension origin as the first argument
	// when spawning a native messaging host.
	if len(os.Args) > 1 && strings.HasPrefix(os.Args[1], "chrome-extension://") {
		if err := host.RunRelay(); err != nil {
			os.Exit(1)
		}
		return
	}
	commands.Execute()
}
```

- [x] **Step 4: Build to confirm it compiles**

```bash
cd /Users/james/www/how-to-recorder/htcli && go build ./cmd/htcli && echo "BUILD OK" 2>&1
```
Expected: `BUILD OK`

- [x] **Step 5: Run all tests**

```bash
cd /Users/james/www/how-to-recorder/htcli && go test ./... 2>&1
```
Expected: all PASS

- [x] **Step 6: Commit**

```bash
cd /Users/james/www/how-to-recorder && git add htcli/internal/commands/serve.go htcli/internal/commands/install.go htcli/cmd/htcli/main.go && git commit -m "feat(htcli): add serve + install commands, relay detection in main"
```

---

## Task 6: Extension background — nativeHost.ts

**Files:**
- Create: `src/background/nativeHost.ts`
- Modify: `src/background/index.ts`

**Interfaces:**
- Produces (message types consumed by Task 7):
  - Background listens for `{ type: "GET_CONNECTION_STATUS" }` → responds `{ type: "CONNECTION_STATUS", mode: "native" | "unavailable" }`
  - Background listens for `{ type: "GET_TAB_ID" }` → responds `{ tabId: number }`
  - Background receives `{ type: "COMMAND_RESULT", commandId: string, result: CommandResult }` from content scripts, forwards to native port

- [x] **Step 1: Create src/background/nativeHost.ts**

```typescript
// src/background/nativeHost.ts
import type { Command, CommandResult, TabInfo } from "../types/commands";

const HOST_NAME = "com.howtorecorder.host";
const RECONNECT_BASE_MS = 1000;
const RECONNECT_MAX_MS = 30000;

let nativePort: chrome.runtime.Port | null = null;
let reconnectDelay = RECONNECT_BASE_MS;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let connectionMode: "native" | "unavailable" = "unavailable";

// ─── Public API ────────────────────────────────────────────────────

export function startNativeHost(): void {
  connect();
}

export function getConnectionMode(): "native" | "unavailable" {
  return connectionMode;
}

export function sendToNative(msg: object): void {
  if (nativePort) {
    try {
      nativePort.postMessage(msg);
    } catch (err) {
      console.error("[NativeHost] postMessage failed:", err);
    }
  }
}

// ─── Connection ────────────────────────────────────────────────────

function connect(): void {
  if (reconnectTimer) {
    clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }

  try {
    nativePort = chrome.runtime.connectNative(HOST_NAME);
  } catch (err) {
    console.warn("[NativeHost] connectNative failed:", err);
    markUnavailable();
    return;
  }

  nativePort.onMessage.addListener(handleNativeMessage);
  nativePort.onDisconnect.addListener(() => {
    const err = chrome.runtime.lastError?.message ?? "unknown";
    console.warn(`[NativeHost] Disconnected: ${err}`);
    nativePort = null;

    if (err.includes("not found") || err.includes("not installed")) {
      markUnavailable();
      return;
    }

    // Relay died (SW was killed) — retry with backoff
    scheduleReconnect();
  });

  connectionMode = "native";
  reconnectDelay = RECONNECT_BASE_MS;
  console.log("[NativeHost] Connected");

  // Broadcast new status to all content scripts
  broadcastStatus();
}

function markUnavailable(): void {
  connectionMode = "unavailable";
  nativePort = null;
  broadcastStatus();
}

function scheduleReconnect(): void {
  reconnectTimer = setTimeout(() => {
    reconnectDelay = Math.min(reconnectDelay * 2, RECONNECT_MAX_MS);
    connect();
  }, reconnectDelay);
}

function broadcastStatus(): void {
  chrome.tabs.query({}, (tabs) => {
    for (const tab of tabs) {
      if (tab.id == null) continue;
      chrome.tabs.sendMessage(tab.id, {
        type: "CONNECTION_STATUS",
        mode: connectionMode,
      }).catch(() => {
        // Content script may not be loaded on this tab — ignore
      });
    }
  });
}

// ─── Message handling (native host → extension) ───────────────────

interface NativeCommandMessage {
  type: "command";
  tabId: number;
  payload: Command;
}

interface NativeRegisterAckMessage {
  type: "ping";
}

type NativeMessage = NativeCommandMessage | NativeRegisterAckMessage;

function handleNativeMessage(msg: NativeMessage): void {
  if (msg.type === "command") {
    const { tabId, payload } = msg;
    chrome.tabs.sendMessage(tabId, {
      type: "EXECUTE_COMMAND",
      command: payload,
    }, (result: CommandResult) => {
      if (chrome.runtime.lastError) {
        // Tab may be closed; relay error back to daemon
        sendToNative({
          type: "command_result",
          tabId,
          payload: { id: payload.id, success: false, error: "tab not available" },
        });
        return;
      }
      sendToNative({
        type: "command_result",
        tabId,
        payload: result,
      });
    });
  }
}

// ─── Tab registration ─────────────────────────────────────────────

export function registerTab(tabId: number, info: TabInfo): void {
  sendToNative({
    type: "register",
    tabId,
    payload: info,
  });
}
```

- [x] **Step 2: Add nativeHost handlers to src/background/index.ts**

Find the bottom of `src/background/index.ts` where `chrome.runtime.onMessage.addListener` is registered. Add the following after all existing handler code (before the closing of the file):

```typescript
// --- Native host integration ---
import { startNativeHost, getConnectionMode, registerTab } from "./nativeHost";

// Start native host on SW startup
startNativeHost();

// Handle GET_CONNECTION_STATUS from content scripts
// (added inside the existing onMessage listener switch/if block)
// Add these cases to the existing message handler:
//
//   case "GET_CONNECTION_STATUS":
//     sendResponse({ type: "CONNECTION_STATUS", mode: getConnectionMode() });
//     break;
//
//   case "GET_TAB_ID":
//     sendResponse({ tabId: sender.tab?.id ?? 0 });
//     break;
//
//   case "CONTENT_SCRIPT_READY":
//     if (sender.tab?.id != null) {
//       registerTab(sender.tab.id, {
//         id: sender.tab.id,
//         url: sender.tab.url ?? "",
//         title: sender.tab.title ?? "",
//         active: sender.tab.active ?? false,
//       });
//     }
//     break;
```

Open `src/background/index.ts` and locate the `chrome.runtime.onMessage.addListener` callback. In the switch or if-else chain, add:

```typescript
case "GET_CONNECTION_STATUS":
  sendResponse({ type: "CONNECTION_STATUS", mode: getConnectionMode() });
  return true;

case "GET_TAB_ID":
  sendResponse({ tabId: sender.tab?.id ?? 0 });
  return true;
```

Also, in the existing `CONTENT_SCRIPT_READY` handler, add a call to `registerTab`:

```typescript
// Find existing CONTENT_SCRIPT_READY case and add:
if (sender.tab?.id != null) {
  registerTab(sender.tab.id, {
    id: sender.tab.id,
    url: (message as { url?: string }).url ?? sender.tab.url ?? "",
    title: sender.tab.title ?? "",
    active: sender.tab.active ?? false,
  });
}
```

Add the import at the top of `src/background/index.ts`:

```typescript
import { startNativeHost, getConnectionMode, registerTab } from "./nativeHost";
```

And add the startup call at the bottom:

```typescript
// Start native host connection
startNativeHost();
```

- [x] **Step 3: Type-check**

```bash
cd /Users/james/www/how-to-recorder && bun run typecheck 2>&1
```
Expected: no errors in `src/background/`

- [x] **Step 4: Commit**

```bash
cd /Users/james/www/how-to-recorder && git add src/background/nativeHost.ts src/background/index.ts && git commit -m "feat(extension): add nativeHost background service"
```

---

## Task 7: Extension — connectionManager, wsClient tab ID setter, manifest

**Files:**
- Create: `src/contentScript/connectionManager.ts`
- Modify: `src/contentScript/wsClient.ts` — export `setTabId`
- Modify: `src/contentScript/index.ts` — use connectionManager
- Modify: `src/manifest.ts` — add nativeMessaging permission

**Interfaces:**
- Consumes: `getConnectionMode` messages from background (Task 6), `connectToServer`, `disconnectFromServer` from wsClient
- Produces: `connect()`, `disconnect()`, `isConnected(): boolean` (same shape as wsClient exports)

- [x] **Step 1: Add setTabId to wsClient.ts**

Open `src/contentScript/wsClient.ts`. Find the `getTabId` function and the module-level `let ws` declaration. Add:

```typescript
// Module-level real tab ID (set by connectionManager once known)
let realTabId: number | null = null;

/**
 * Override the pseudo tab ID with the real chrome.tabs ID.
 * Called by connectionManager after fetching the real ID from background.
 */
export function setTabId(id: number): void {
  realTabId = id;
}
```

Then modify `getTabId()` to prefer `realTabId`:

```typescript
function getTabId(): number {
  if (realTabId !== null) return realTabId;
  return hashString(window.location.href);
}
```

- [x] **Step 2: Create connectionManager.ts**

```typescript
// src/contentScript/connectionManager.ts
import {
  connectToServer,
  disconnectFromServer,
  isConnected as wsIsConnected,
  setTabId,
} from "./wsClient";

type ConnectionMode = "native" | "ws" | "disconnected";

let mode: ConnectionMode = "disconnected";

// ─── Init ───────────────────────────────────────────────────────────

export async function connect(): Promise<void> {
  // Fetch real tab ID from background
  const tabId = await getRealTabId();
  if (tabId) setTabId(tabId);

  // Ask background for native host status
  const status = await getConnectionStatus();

  if (status === "native") {
    mode = "native";
    console.log("[ConnectionManager] Using native messaging");
    return;
  }

  // Native unavailable — fall back to WebSocket
  mode = "ws";
  console.log("[ConnectionManager] Native unavailable, falling back to WebSocket");
  await checkAutoConnectWS();
}

export function disconnect(): void {
  if (mode === "ws") {
    disconnectFromServer();
  }
  mode = "disconnected";
}

export function isConnected(): boolean {
  if (mode === "native") return true;
  if (mode === "ws") return wsIsConnected();
  return false;
}

// ─── Helpers ────────────────────────────────────────────────────────

function getRealTabId(): Promise<number | null> {
  return new Promise((resolve) => {
    chrome.runtime.sendMessage({ type: "GET_TAB_ID" }, (resp) => {
      if (chrome.runtime.lastError || !resp?.tabId) {
        resolve(null);
        return;
      }
      resolve(resp.tabId as number);
    });
  });
}

function getConnectionStatus(): Promise<"native" | "unavailable"> {
  return new Promise((resolve) => {
    chrome.runtime.sendMessage({ type: "GET_CONNECTION_STATUS" }, (resp) => {
      if (chrome.runtime.lastError || resp?.mode !== "native") {
        resolve("unavailable");
        return;
      }
      resolve("native");
    });
  });
}

async function checkAutoConnectWS(): Promise<void> {
  try {
    const result = await chrome.storage.local.get([
      "remoteControlServer",
      "remoteControlToken",
    ]);
    if (result.remoteControlServer) {
      const token = result.remoteControlToken as string | undefined;
      const url = token
        ? `${result.remoteControlServer as string}?token=${encodeURIComponent(token)}`
        : (result.remoteControlServer as string);
      connectToServer(url);
    }
  } catch {
    // storage unavailable
  }
}

// ─── Listen for status changes from background ────────────────────

chrome.runtime.onMessage.addListener((message) => {
  if (message?.type === "CONNECTION_STATUS") {
    if (message.mode === "native" && mode !== "native") {
      mode = "native";
      if (wsIsConnected()) disconnectFromServer();
      console.log("[ConnectionManager] Switched to native messaging");
    } else if (message.mode === "unavailable" && mode === "native") {
      mode = "disconnected";
      checkAutoConnectWS();
    }
  }
});
```

- [x] **Step 3: Update src/contentScript/index.ts**

Find these lines in `index.ts`:

```typescript
import { connectToServer, disconnectFromServer } from "./wsClient";
```

Replace with:

```typescript
import { connect as connectRemote, disconnect as disconnectRemote, isConnected as remoteIsConnected } from "./connectionManager";
```

Find `enableRemoteControl`:

```typescript
function enableRemoteControl(serverUrl?: string): void {
  if (remoteControlEnabled) {
    console.warn("[How-To Recorder] Remote control already enabled");
    return;
  }
  remoteControlEnabled = true;
  connectToServer(serverUrl);
  console.info("[How-To Recorder] Remote control enabled");
}
```

Replace with:

```typescript
function enableRemoteControl(_serverUrl?: string): void {
  if (remoteControlEnabled) {
    console.warn("[How-To Recorder] Remote control already enabled");
    return;
  }
  remoteControlEnabled = true;
  connectRemote();
  console.info("[How-To Recorder] Remote control enabled");
}
```

Find `disableRemoteControl`:

```typescript
function disableRemoteControl(): void {
  if (!remoteControlEnabled) return;
  remoteControlEnabled = false;
  disconnectFromServer();
  console.info("[How-To Recorder] Remote control disabled");
}
```

Replace with:

```typescript
function disableRemoteControl(): void {
  if (!remoteControlEnabled) return;
  remoteControlEnabled = false;
  disconnectRemote();
  console.info("[How-To Recorder] Remote control disabled");
}
```

Also remove the old `checkAutoConnect` call at the bottom of `index.ts` if it calls `wsClient` directly — `connectionManager` now handles auto-connect.

- [x] **Step 4: Add nativeMessaging to manifest**

Open `src/manifest.ts`. Find the `permissions` array:

```typescript
permissions: [
  "activeTab",
  "tabs",
  "contextMenus",
  "downloads",
  "storage",
  "scripting",
  "sidePanel",
],
```

Replace with:

```typescript
permissions: [
  "activeTab",
  "tabs",
  "contextMenus",
  "downloads",
  "storage",
  "scripting",
  "sidePanel",
  "nativeMessaging",
],
```

- [x] **Step 5: Type-check entire extension**

```bash
cd /Users/james/www/how-to-recorder && bun run typecheck 2>&1
```
Expected: no errors

- [x] **Step 6: Build extension**

```bash
cd /Users/james/www/how-to-recorder && bun run build 2>&1
```
Expected: successful build with no errors

- [x] **Step 7: Commit**

```bash
cd /Users/james/www/how-to-recorder && git add src/contentScript/connectionManager.ts src/contentScript/wsClient.ts src/contentScript/index.ts src/manifest.ts && git commit -m "feat(extension): add connectionManager with native/WS auto-detect"
```

---

## Task 8: Manual integration test

This task verifies the full flow end-to-end before shipping.

Current shell smoke-test status:
- `htcli serve` now starts cleanly after creating the Unix socket parent directory.
- `/api/health` responds `running` on the daemon.
- Native host relay registration and a command round-trip both work in a local shell simulation.
- Steps that require a live Chrome session and extension reload still need browser validation.

- [ ] **Step 1: Build htcli**

```bash
cd /Users/james/www/how-to-recorder/htcli && go build -o bin/htcli ./cmd/htcli && echo "BUILD OK"
```

- [ ] **Step 2: Start the daemon**

```bash
/Users/james/www/how-to-recorder/htcli/bin/htcli serve &
# Expected output:
# [htcli serve] Listening on http://127.0.0.1:3845
# [htcli serve] Unix socket: ~/.htcli/daemon.sock
```

- [ ] **Step 3: Verify health endpoint**

```bash
curl -s http://127.0.0.1:3845/api/health | python3 -m json.tool
# Expected: {"ok": true, "data": {"status": "running", "connectedTabs": 0, ...}}
```

- [ ] **Step 4: Register native host**

```bash
# Find your extension ID from chrome://extensions
/Users/james/www/how-to-recorder/htcli/bin/htcli install --extension-id <YOUR_EXTENSION_ID>
# Expected: "Manifest written: ~/Library/Application Support/.../com.howtorecorder.host.json"
```

- [ ] **Step 5: Load extension in Chrome**

1. Go to `chrome://extensions`
2. Enable Developer mode
3. Load unpacked → select `/Users/james/www/how-to-recorder/build`
4. Open any webpage
5. Open DevTools → Console
6. Look for: `[NativeHost] Connected` or `[ConnectionManager] Using native messaging`

- [ ] **Step 6: Verify tab appears in daemon**

```bash
curl -s http://127.0.0.1:3845/api/tabs | python3 -m json.tool
# Expected: {"ok": true, "data": [{"id": <real_tab_id>, "url": "...", ...}]}
```

- [ ] **Step 7: Run a command via htcli**

```bash
/Users/james/www/how-to-recorder/htcli/bin/htcli health
# Expected: Server: running, Connected tabs: 1

/Users/james/www/how-to-recorder/htcli/bin/htcli open https://example.com
# Expected: "Navigated to https://example.com (Xms)"
```

- [ ] **Step 8: Test fallback — stop daemon, verify WS fallback**

```bash
kill %1   # stop htcli serve
# Reload extension page
# Console should show: [ConnectionManager] Native unavailable, falling back to WebSocket
# Start Bun server: bun run server
# Console should show wsClient reconnecting
```

- [ ] **Step 9: Final commit**

```bash
cd /Users/james/www/how-to-recorder && git add -A && git commit -m "feat: native messaging support complete (relay + daemon + extension auto-detect)"
```
