# Part 2: CDP Client Package

Spec: `docs/superpowers/specs/2026-07-10-htcli-cdp-transport-design.md`. New Go package `htcli/internal/cdp`. Depends on Part 1 being merged (uses nothing from it directly, but Part 3 consumes both).

New dependency: `github.com/gorilla/websocket` v1.5 — pinned major, run `cd htcli && go get github.com/gorilla/websocket@v1.5.3`. gorilla's client sends no `Origin` header unless one is set in the request header — set none (Chrome ≥111 rejects unlisted origins).

---

### Task 3: HTTP target discovery

**Files:**
- Create: `htcli/internal/cdp/discover.go`
- Test: `htcli/internal/cdp/discover_test.go`

**Interfaces:**
- Produces:
  - `type Target struct { ID, Type, Title, URL, WebSocketDebuggerURL string }` (JSON tags `id`, `type`, `title`, `url`, `webSocketDebuggerUrl`)
  - `ListTargets(port int) ([]Target, error)` — GET `http://127.0.0.1:<port>/json`, page targets only
  - `BrowserWSURL(port int) (string, error)` — GET `/json/version`, returns `webSocketDebuggerUrl`
  - `ErrNotRunning` — sentinel returned when the port refuses connection; callers print `CDP browser not running — start it with: htcli browser start`

- [ ] **Step 1: Write the failing test**

Create `htcli/internal/cdp/discover_test.go`:

```go
package cdp

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
)

func testServer(t *testing.T, mux *http.ServeMux) int {
	t.Helper()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())
	return port
}

func TestListTargetsFiltersPages(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/json", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[
			{"id":"AAA1","type":"page","title":"Privacy","url":"https://x/","webSocketDebuggerUrl":"ws://h/devtools/page/AAA1"},
			{"id":"BBB2","type":"iframe","title":"f","url":"https://y/","webSocketDebuggerUrl":"ws://h/devtools/page/BBB2"}
		]`))
	})
	port := testServer(t, mux)

	targets, err := ListTargets(port)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(targets) != 1 || targets[0].ID != "AAA1" {
		t.Fatalf("want only page target AAA1, got %+v", targets)
	}
}

func TestBrowserWSURL(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/json/version", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"Browser":"Chrome/140.0","webSocketDebuggerUrl":"ws://127.0.0.1:9222/devtools/browser/abc"}`))
	})
	port := testServer(t, mux)

	got, err := BrowserWSURL(port)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "ws://127.0.0.1:9222/devtools/browser/abc" {
		t.Fatalf("got %q", got)
	}
}

func TestListTargetsNotRunning(t *testing.T) {
	// Port 1 is never listening.
	_, err := ListTargets(1)
	if !errors.Is(err, ErrNotRunning) {
		t.Fatalf("want ErrNotRunning, got %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd htcli && go test ./internal/cdp/ -v`
Expected: compile error — package/functions don't exist.

- [ ] **Step 3: Implement discover.go**

```go
// Package cdp is a minimal Chrome DevTools Protocol client for the htcli
// --cdp transport. It talks only to 127.0.0.1: /json discovery over HTTP,
// then per-target and browser-level WebSocket sessions.
package cdp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// ErrNotRunning means nothing answers on the debugging port.
var ErrNotRunning = errors.New("CDP browser not running — start it with: htcli browser start")

// Target is one entry from GET /json.
type Target struct {
	ID                   string `json:"id"`
	Type                 string `json:"type"`
	Title                string `json:"title"`
	URL                  string `json:"url"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

var httpClient = &http.Client{Timeout: 5 * time.Second}

func getJSON(port int, path string, out any) error {
	// 127.0.0.1 literal: Chrome's DNS-rebinding guard rejects non-IP Hosts.
	resp, err := httpClient.Get(fmt.Sprintf("http://127.0.0.1:%d%s", port, path))
	if err != nil {
		var netErr *net.OpError
		if errors.As(err, &netErr) {
			return fmt.Errorf("%w (port %d): %v", ErrNotRunning, port, err)
		}
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: HTTP %d: %s", path, resp.StatusCode, body)
	}
	return json.Unmarshal(body, out)
}

// ListTargets returns page-type targets from GET /json.
func ListTargets(port int) ([]Target, error) {
	var all []Target
	if err := getJSON(port, "/json", &all); err != nil {
		return nil, err
	}
	pages := make([]Target, 0, len(all))
	for _, t := range all {
		if t.Type == "page" {
			pages = append(pages, t)
		}
	}
	return pages, nil
}

// BrowserWSURL returns the browser-level WebSocket endpoint from /json/version
// (required for Browser.* domain methods).
func BrowserWSURL(port int) (string, error) {
	var v struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := getJSON(port, "/json/version", &v); err != nil {
		return "", err
	}
	if v.WebSocketDebuggerURL == "" {
		return "", errors.New("/json/version returned no webSocketDebuggerUrl")
	}
	return v.WebSocketDebuggerURL, nil
}
```

- [ ] **Step 4: Run tests**

Run: `cd htcli && go test ./internal/cdp/ -v`
Expected: 3 PASS. (If `TestListTargetsNotRunning` fails because the refused-connection error isn't a `*net.OpError` on this platform, match on `errors.Is(err, syscall.ECONNREFUSED)` OR simply wrap ALL transport-level errors from `httpClient.Get` in `ErrNotRunning` — a CLI can't do anything different for timeout vs refusal here.)

- [ ] **Step 5: Commit**

```bash
git add htcli/internal/cdp/ htcli/go.mod htcli/go.sum
git commit -m "feat(htcli): CDP target discovery over /json endpoints

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 4: WebSocket session

**Files:**
- Create: `htcli/internal/cdp/session.go`
- Test: `htcli/internal/cdp/session_test.go`

**Interfaces:**
- Produces:
  - `Dial(wsURL string) (*Session, error)` — connects with NO Origin header
  - `(*Session) Call(method string, params any, result any) error` — sends `{id, method, params}`, reads until the matching `id`, unmarshals `result` field into `result` (nil ok); CDP error responses become Go errors
  - `(*Session) WaitEvent(name string, timeout time.Duration) (json.RawMessage, error)` — returns the params of the next matching event (checks events buffered during earlier Calls first)
  - `(*Session) Close() error`
- Consumes: Task 3 (`Target.WebSocketDebuggerURL`, `BrowserWSURL`).

- [ ] **Step 1: Write the failing test**

Create `htcli/internal/cdp/session_test.go`. The fake CDP server upgrades a WS, answers method calls, and can interleave events:

```go
package cdp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type fakeMsg struct {
	ID     int64           `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

// fakeCDP answers every call with handler(method) and pushes pre/post events.
func fakeCDP(t *testing.T, handler func(m fakeMsg, conn *websocket.Conn)) string {
	t.Helper()
	up := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") != "" {
			t.Errorf("client sent Origin header %q — must send none", r.Header.Get("Origin"))
		}
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()
		for {
			var m fakeMsg
			if err := conn.ReadJSON(&m); err != nil {
				return // intentionally not logged: client closing the socket ends the fake server loop
			}
			handler(m, conn)
		}
	}))
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

func TestCallRoundTrip(t *testing.T) {
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{"value": 42}})
	})
	s, err := Dial(url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer s.Close()

	var res struct {
		Value int `json:"value"`
	}
	if err := s.Call("Runtime.evaluate", map[string]any{"expression": "6*7"}, &res); err != nil {
		t.Fatalf("call: %v", err)
	}
	if res.Value != 42 {
		t.Fatalf("want 42, got %d", res.Value)
	}
}

func TestCallSkipsAndBuffersEvents(t *testing.T) {
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		// Event arrives BEFORE the call's response.
		conn.WriteJSON(map[string]any{"method": "Page.loadEventFired", "params": map[string]any{"timestamp": 1}})
		conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
	})
	s, err := Dial(url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer s.Close()

	if err := s.Call("Page.navigate", map[string]any{"url": "https://x/"}, nil); err != nil {
		t.Fatalf("call: %v", err)
	}
	// The event buffered during Call must be returned without reading the socket.
	params, err := s.WaitEvent("Page.loadEventFired", time.Second)
	if err != nil {
		t.Fatalf("waitevent: %v", err)
	}
	if !strings.Contains(string(params), "timestamp") {
		t.Fatalf("got params %s", params)
	}
}

func TestCallCDPError(t *testing.T) {
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		conn.WriteJSON(map[string]any{"id": m.ID, "error": map[string]any{"code": -32000, "message": "Cannot find context"}})
	})
	s, err := Dial(url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer s.Close()

	err = s.Call("Runtime.evaluate", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "Cannot find context") {
		t.Fatalf("want CDP error surfaced, got %v", err)
	}
}

func TestWaitEventTimeout(t *testing.T) {
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {})
	s, err := Dial(url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer s.Close()

	if _, err := s.WaitEvent("Page.loadEventFired", 50*time.Millisecond); err == nil {
		t.Fatal("want timeout error")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd htcli && go test ./internal/cdp/ -run 'TestCall|TestWaitEvent' -v`
Expected: compile error — `Dial`, `Session` undefined.

- [ ] **Step 3: Implement session.go**

```go
package cdp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// Session is a synchronous CDP connection (page-level or browser-level).
// htcli issues one call at a time, so no concurrent-writer handling is needed.
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
	defer s.conn.SetReadDeadline(time.Time{}) // intentionally not logged: clearing a deadline cannot meaningfully fail after reads succeeded
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
```

- [ ] **Step 4: Run tests**

Run: `cd htcli && go test ./internal/cdp/ -v`
Expected: all PASS (Tasks 3 + 4).

- [ ] **Step 5: Commit**

```bash
git add htcli/internal/cdp/session.go htcli/internal/cdp/session_test.go
git commit -m "feat(htcli): CDP WebSocket session with call/event correlation

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```
