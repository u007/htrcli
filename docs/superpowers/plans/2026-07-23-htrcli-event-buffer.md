# htrcli Durable Event Buffer + Console Capture Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give htrcli a durable, cursor-based event buffer for "arm before it happens, read later" page events, and ship its first consumer: `htrcli console read`/`console watch` for captured `console.*` output.

**Architecture:** A MAIN-world content script wraps `console.*` and forwards each call to the background service worker. The background's `eventStore` assigns a durable per-(tab,kind) sequence number, persists to `chrome.storage.session` (survives SW restarts), count-caps at 500 entries/tab/kind (oldest evicted first, drop count tracked), and POSTs new entries to the daemon. The daemon keeps its own copy as the CLI-facing source of truth, exposes a cursor-based `GET /api/events`, and bumps a generation marker on its own restart so the extension knows to replay its buffer. The CLI's `EventPoller` wraps the read/long-poll pattern; `console read`/`console watch` are its first callers.

**Tech Stack:** Go (cobra CLI, stdlib `net/http`), TypeScript (Chrome/Firefox WebExtension APIs, `chrome.storage.session` / `browser.storage.session`), Bun test runner, Go's `testing` package.

## Global Constraints

- Package manager: `bun` only for the extension — never npm/yarn.
- Biome lint/format (tabs, double quotes) — run `bun run check:fix` before committing TS changes.
- Go tests: `go test ./...` from `htrcli/`.
- Async `chrome.runtime.onMessage` listeners must `return true` when responding asynchronously.
- Extension console/error logging prefix: `console.error/warn('[HTR NControl] ...')`.
- Count cap: 500 entries per (tab, kind). Eviction is never silent — always report a `dropped` count, computed per-request from the client's own `since` cursor, never a replayed global total.
- No new external dependencies for this phase.

---

### Task 1: Go API types + client methods for events

**Files:**
- Modify: `htrcli/internal/api/types.go`
- Modify: `htrcli/internal/api/client.go`
- Test: `htrcli/internal/api/client_test.go`

**Interfaces:**
- Produces: `api.EventEntry{Seq int, Kind string, Timestamp int64, Data json.RawMessage}`, `api.EventsResponse{Entries []EventEntry, Dropped int, OldestAvailableSeq int}`, `api.IngestEventsRequest{TabID int, Kind string, Entries []EventEntry}`, `(*Client) PostEvents(tabID int, kind string, entries []EventEntry) error`, `(*Client) GetEvents(tabID *int, kind string, since int) (*EventsResponse, error)`.

- [ ] **Step 1: Write the failing test**

Append to `htrcli/internal/api/client_test.go`:

```go
func TestPostEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/events/ingest" {
			t.Errorf("expected path /api/events/ingest, got %s", r.URL.Path)
		}
		var req IngestEventsRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.TabID != 1 || req.Kind != "console" || len(req.Entries) != 1 {
			t.Errorf("unexpected request body: %+v", req)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ApiResponse{OK: true, Data: map[string]any{"received": true}})
	}))
	defer server.Close()

	c := NewClient(server.URL, "")
	err := c.PostEvents(1, "console", []EventEntry{
		{Seq: 1, Kind: "console", Timestamp: 1000, Data: json.RawMessage(`{"level":"log","args":["hi"]}`)},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("since") != "40" || r.URL.Query().Get("kind") != "console" {
			t.Errorf("unexpected query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ApiResponse{
			OK: true,
			Data: EventsResponse{
				Entries:            []EventEntry{{Seq: 41, Kind: "console", Timestamp: 2000, Data: json.RawMessage(`{"level":"error","args":["boom"]}`)}},
				Dropped:            0,
				OldestAvailableSeq: 1,
			},
		})
	}))
	defer server.Close()

	c := NewClient(server.URL, "")
	resp, err := c.GetEvents(nil, "console", 40)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Entries) != 1 || resp.Entries[0].Seq != 41 {
		t.Errorf("unexpected response: %+v", resp)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd htrcli && go test ./internal/api/... -run 'TestPostEvents|TestGetEvents' -v`
Expected: FAIL — `EventEntry`, `EventsResponse`, `IngestEventsRequest`, `PostEvents`, `GetEvents` undefined.

- [ ] **Step 3: Add the types**

Append to `htrcli/internal/api/types.go`:

```go
// EventEntry is one captured event (console line, network request, or
// dialog). Data is kind-specific and decoded by the caller.
type EventEntry struct {
	Seq       int             `json:"seq"`
	Kind      string          `json:"kind"`
	Timestamp int64           `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
}

// EventsResponse is the response from GET /api/events. Dropped is computed
// relative to the requesting client's own `since` cursor, not a replayed
// global counter — a client that already read past an eviction sees 0.
type EventsResponse struct {
	Entries            []EventEntry `json:"entries"`
	Dropped            int          `json:"dropped"`
	OldestAvailableSeq int          `json:"oldestAvailableSeq"`
}

// IngestEventsRequest is the request body for POST /api/events/ingest.
type IngestEventsRequest struct {
	TabID   int          `json:"tabId"`
	Kind    string       `json:"kind"`
	Entries []EventEntry `json:"entries"`
}
```

Add `"encoding/json"` to the imports of `types.go` (needed for `json.RawMessage`).

- [ ] **Step 4: Add the client methods**

Append to `htrcli/internal/api/client.go`:

```go
// PostEvents forwards a batch of captured events for one tab/kind to the
// daemon. Used by the extension-side ingest path in tests and by any
// future replay/resync tooling — the CLI itself only reads events.
func (c *Client) PostEvents(tabID int, kind string, entries []EventEntry) error {
	req := IngestEventsRequest{TabID: tabID, Kind: kind, Entries: entries}
	_, err := c.doRequestWithEnvelope("POST", "/api/events/ingest", req)
	return err
}

// GetEvents polls for events of a kind since a cursor (0 = from the start).
// A nil tabID targets the server's default tab, mirroring GetPageInfo.
func (c *Client) GetEvents(tabID *int, kind string, since int) (*EventsResponse, error) {
	path := fmt.Sprintf("/api/events?kind=%s&since=%d", kind, since)
	if tabID != nil {
		path += "&tab=" + strconv.Itoa(*tabID)
	}
	data, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var apiResp ApiResponse
	if err := json.Unmarshal(data, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if !apiResp.OK {
		return nil, &APIError{Message: apiResp.Error}
	}

	dataBytes, err := json.Marshal(apiResp.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data: %w", err)
	}
	var events EventsResponse
	if err := json.Unmarshal(dataBytes, &events); err != nil {
		return nil, fmt.Errorf("failed to parse events response: %w", err)
	}
	return &events, nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd htrcli && go test ./internal/api/... -run 'TestPostEvents|TestGetEvents' -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add htrcli/internal/api/types.go htrcli/internal/api/client.go htrcli/internal/api/client_test.go
git commit -m "feat(htrcli): add EventEntry API types and client methods"
```

---

### Task 2: Go daemon-side event store

**Files:**
- Create: `htrcli/internal/host/events.go`
- Test: `htrcli/internal/host/events_test.go`

**Interfaces:**
- Consumes: nothing new (stdlib only).
- Produces: `host.NewEventStore() *EventStore`, `(*EventStore) Ingest(tabID int, kind string, entries []Command_Event)` — see note below on the concrete entry type — `(*EventStore) Read(tabID int, kind string, since int) (entries []Event, dropped int, oldestAvailableSeq int)`, `(*EventStore) Generation() int64` (bumped once at daemon startup, read-only accessor for the resync marker).

Use the daemon's own `NativeMessage`-style plain struct rather than importing `api` (the `host` package does not import `api` today — see `daemon.go`'s `Command`/`CommandResult` types, which are host-local mirrors, not `api.Command`). Define a host-local `Event` type mirroring `api.EventEntry`.

- [ ] **Step 1: Write the failing test**

Create `htrcli/internal/host/events_test.go`:

```go
package host

import "testing"

func TestEventStoreIngestAndRead(t *testing.T) {
	es := NewEventStore()
	es.Ingest(1, "console", []Event{
		{Kind: "console", Timestamp: 1000, Data: []byte(`{"level":"log","args":["a"]}`)},
		{Kind: "console", Timestamp: 1001, Data: []byte(`{"level":"log","args":["b"]}`)},
	})

	entries, dropped, oldest := es.Read(1, "console", 0)
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %d", len(entries))
	}
	if entries[0].Seq != 1 || entries[1].Seq != 2 {
		t.Fatalf("want seq 1,2, got %d,%d", entries[0].Seq, entries[1].Seq)
	}
	if dropped != 0 || oldest != 1 {
		t.Fatalf("want dropped=0 oldest=1, got dropped=%d oldest=%d", dropped, oldest)
	}
}

func TestEventStoreEvictionReportsDroppedPerCursor(t *testing.T) {
	es := NewEventStore()
	es.capOverride = 3 // test-only override of the 500 default, see Step 3
	for i := 0; i < 5; i++ {
		es.Ingest(1, "console", []Event{{Kind: "console", Timestamp: int64(i), Data: []byte(`{}`)}})
	}
	// Seq 1,2 evicted; 3,4,5 remain. A client that last saw seq 0 (never
	// read) missed 2 entries.
	entries, dropped, oldest := es.Read(1, "console", 0)
	if len(entries) != 3 || entries[0].Seq != 3 {
		t.Fatalf("want 3 entries starting at seq 3, got %+v", entries)
	}
	if dropped != 2 {
		t.Fatalf("want dropped=2, got %d", dropped)
	}
	if oldest != 3 {
		t.Fatalf("want oldestAvailableSeq=3, got %d", oldest)
	}

	// A client that already read up to seq 3 should see dropped=0 — the
	// drop happened before its cursor, not after it.
	entries2, dropped2, _ := es.Read(1, "console", 3)
	if len(entries2) != 2 || dropped2 != 0 {
		t.Fatalf("want 2 entries dropped=0, got %d entries dropped=%d", len(entries2), dropped2)
	}
}

func TestEventStoreSeparatesTabsAndKinds(t *testing.T) {
	es := NewEventStore()
	es.Ingest(1, "console", []Event{{Kind: "console", Timestamp: 1, Data: []byte(`{}`)}})
	es.Ingest(2, "console", []Event{{Kind: "console", Timestamp: 1, Data: []byte(`{}`)}})
	es.Ingest(1, "network", []Event{{Kind: "network", Timestamp: 1, Data: []byte(`{}`)}})

	if entries, _, _ := es.Read(1, "console", 0); len(entries) != 1 {
		t.Fatalf("tab 1 console: want 1 entry, got %d", len(entries))
	}
	if entries, _, _ := es.Read(2, "console", 0); len(entries) != 1 {
		t.Fatalf("tab 2 console: want 1 entry, got %d", len(entries))
	}
	if entries, _, _ := es.Read(1, "network", 0); len(entries) != 1 {
		t.Fatalf("tab 1 network: want 1 entry, got %d", len(entries))
	}
}

func TestEventStoreGenerationSetOnce(t *testing.T) {
	es := NewEventStore()
	g1 := es.Generation()
	es2 := NewEventStore()
	g2 := es2.Generation()
	if g1 == g2 {
		t.Fatalf("expected distinct generations across daemon instances, got %d == %d", g1, g2)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd htrcli && go test ./internal/host/... -run TestEventStore -v`
Expected: FAIL — package `host` has no `NewEventStore`/`Event`/`EventStore`.

- [ ] **Step 3: Implement the event store**

Create `htrcli/internal/host/events.go`:

```go
package host

import (
	"sync"
	"time"
)

// Event is the daemon's internal representation of one captured event.
// Mirrors api.EventEntry but stays host-local like Command/CommandResult
// do elsewhere in this package.
type Event struct {
	Seq       int
	Kind      string
	Timestamp int64
	Data      []byte
}

const defaultEventCap = 500

type eventBucket struct {
	entries    []Event // ring buffer, oldest first
	nextSeq    int
	evictedTotal int // total ever evicted for this (tab, kind), for internal bookkeeping only
}

// EventStore holds per-(tab, kind) capped ring buffers of captured events.
// It is the daemon's CLI-facing source of truth — the extension's
// chrome.storage.session copy is durable across service-worker restarts,
// but a client asking "what happened" always reads from here.
type EventStore struct {
	mu         sync.Mutex
	buckets    map[string]*eventBucket // key: tabID:kind
	cap        int
	capOverride int // test-only; 0 means "use cap"
	generation int64
}

// NewEventStore creates an empty store and stamps it with a generation
// derived from the current time, so the extension can detect "the daemon
// restarted" by comparing generations across polls (see Generation).
func NewEventStore() *EventStore {
	return &EventStore{
		buckets:    make(map[string]*eventBucket),
		cap:        defaultEventCap,
		generation: time.Now().UnixNano(),
	}
}

func (s *EventStore) capFor() int {
	if s.capOverride > 0 {
		return s.capOverride
	}
	return s.cap
}

func bucketKey(tabID int, kind string) string {
	// int -> string without strconv to keep this file dependency-free;
	// matches the small-integer tab IDs used throughout this package.
	return kind + ":" + itoa(tabID)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// Ingest appends entries for a (tab, kind), assigning each the next
// sequence number and evicting the oldest entries past the cap.
func (s *EventStore) Ingest(tabID int, kind string, entries []Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := bucketKey(tabID, kind)
	b, ok := s.buckets[key]
	if !ok {
		b = &eventBucket{}
		s.buckets[key] = b
	}

	for _, e := range entries {
		b.nextSeq++
		e.Seq = b.nextSeq
		e.Kind = kind
		b.entries = append(b.entries, e)
	}

	if cap := s.capFor(); len(b.entries) > cap {
		overflow := len(b.entries) - cap
		b.evictedTotal += overflow
		b.entries = b.entries[overflow:]
	}
}

// Read returns entries with seq > since, plus how many entries THIS
// client missed (computed from its own cursor, not a replayed running
// total) and the oldest seq still available.
func (s *EventStore) Read(tabID int, kind string, since int) (entries []Event, dropped int, oldestAvailableSeq int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	b, ok := s.buckets[bucketKey(tabID, kind)]
	if !ok || len(b.entries) == 0 {
		return nil, 0, 0
	}

	oldestAvailableSeq = b.entries[0].Seq
	if since < oldestAvailableSeq-1 {
		dropped = oldestAvailableSeq - since - 1
	}

	for _, e := range b.entries {
		if e.Seq > since {
			entries = append(entries, e)
		}
	}
	return entries, dropped, oldestAvailableSeq
}

// Generation identifies this daemon process instance. The extension polls
// it; a change means the daemon restarted and the extension should replay
// its full chrome.storage.session buffer (see docs/superpowers/specs/
// 2026-07-23-htrcli-event-buffer-design.md, "daemon process restart").
func (s *EventStore) Generation() int64 {
	return s.generation
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd htrcli && go test ./internal/host/... -run TestEventStore -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add htrcli/internal/host/events.go htrcli/internal/host/events_test.go
git commit -m "feat(htrcli): add daemon-side capped event store"
```

---

### Task 3: Wire event store into the daemon and HTTP API

**Files:**
- Modify: `htrcli/internal/host/daemon.go`
- Modify: `htrcli/internal/host/server.go`
- Test: `htrcli/internal/host/server_test.go`

**Interfaces:**
- Consumes: `EventStore` from Task 2 (`NewEventStore`, `Ingest`, `Read`, `Generation`).
- Produces: `Daemon.Events *EventStore` field (exported, so `server.go` handlers can reach it directly like `d.Tabs()` does elsewhere), `GET /api/events?kind=&since=&tab=`, `POST /api/events/ingest`, `GET /api/events/generation`.

- [ ] **Step 1: Write the failing test**

Append to `htrcli/internal/host/server_test.go`:

```go
func TestEventsIngestAndRead(t *testing.T) {
	d := host.NewDaemon()
	srv := host.NewHTTPServer(d, 0, "", nil)
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	ingestBody := `{"tabId":1,"kind":"console","entries":[{"kind":"console","timestamp":1000,"data":{"level":"log","args":["hi"]}}]}`
	resp, err := http.Post(ts.URL+"/api/events/ingest", "application/json", strings.NewReader(ingestBody))
	if err != nil {
		t.Fatalf("POST /api/events/ingest: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("ingest status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	resp2, err := http.Get(ts.URL + "/api/events?kind=console&tab=1&since=0")
	if err != nil {
		t.Fatalf("GET /api/events: %v", err)
	}
	defer resp2.Body.Close()
	var body map[string]interface{}
	json.NewDecoder(resp2.Body).Decode(&body)
	data := body["data"].(map[string]interface{})
	entries := data["entries"].([]interface{})
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
}

func TestEventsGenerationEndpoint(t *testing.T) {
	d := host.NewDaemon()
	srv := host.NewHTTPServer(d, 0, "", nil)
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/events/generation")
	if err != nil {
		t.Fatalf("GET /api/events/generation: %v", err)
	}
	defer resp.Body.Close()
	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	if body["ok"] != true {
		t.Fatalf("body.ok = %v, want true", body["ok"])
	}
	if _, ok := body["data"].(map[string]interface{})["generation"]; !ok {
		t.Fatalf("expected data.generation in response, got %+v", body)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd htrcli && go test ./internal/host/... -run 'TestEventsIngestAndRead|TestEventsGenerationEndpoint' -v`
Expected: FAIL — 404 not found for the new routes.

- [ ] **Step 3: Add the `Events` field to `Daemon`**

In `htrcli/internal/host/daemon.go`, add to the `Daemon` struct (near the other fields, before the closing `}` of `type Daemon struct { ... }`):

```go
	// Events is the CLI-facing capped event store (console/network/dialog
	// capture). Exported like Tabs()/FirstTabID() are implicitly reachable
	// from server.go's handlers.
	Events *EventStore
```

In `NewDaemon()` (find the existing constructor that initializes `conns`/`pending`/`pendingShots`), add initialization:

```go
	d.Events = NewEventStore()
```

- [ ] **Step 4: Add the HTTP handlers**

In `htrcli/internal/host/server.go`, add three cases to the `switch` inside `apiHandler` (alongside the existing `/api/tabs`, `/api/page` cases):

```go
		case path == "/api/events" && r.Method == "GET":
			handleEventsGet(w, r, d)

		case path == "/api/events/ingest" && r.Method == "POST":
			handleEventsIngest(w, r, d)

		case path == "/api/events/generation" && r.Method == "GET":
			apiOK(w, map[string]any{"generation": d.Events.Generation()})
```

Add the handler functions and request/response types near `handlePageGet`:

```go
// eventEntryWire is the wire shape for one event in POST /api/events/ingest
// and GET /api/events. Seq is omitted on ingest (the store assigns it) and
// populated on read.
type eventEntryWire struct {
	Seq       int             `json:"seq,omitempty"`
	Kind      string          `json:"kind"`
	Timestamp int64           `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
}

type ingestEventsRequest struct {
	TabID   int              `json:"tabId"`
	Kind    string           `json:"kind"`
	Entries []eventEntryWire `json:"entries"`
}

// handleEventsIngest receives a batch of newly captured events from the
// extension and appends them to the daemon's EventStore.
func handleEventsIngest(w http.ResponseWriter, r *http.Request, d *Daemon) {
	var req ingestEventsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.TabID == 0 || req.Kind == "" {
		apiError(w, 400, "invalid request body")
		return
	}
	events := make([]Event, len(req.Entries))
	for i, e := range req.Entries {
		events[i] = Event{Kind: e.Kind, Timestamp: e.Timestamp, Data: e.Data}
	}
	d.Events.Ingest(req.TabID, req.Kind, events)
	apiOK(w, map[string]any{"received": true})
}

// handleEventsGet answers a cursor-based poll: ?kind=console&since=40&tab=1.
// tab defaults to the daemon's first connected tab, mirroring handlePageGet.
func handleEventsGet(w http.ResponseWriter, r *http.Request, d *Daemon) {
	kind := r.URL.Query().Get("kind")
	if kind == "" {
		apiError(w, 400, "kind is required")
		return
	}
	var tabID int
	if q := r.URL.Query().Get("tab"); q != "" {
		tabID = parseTabID(q)
	} else {
		id, ok := d.FirstTabID()
		if !ok {
			apiError(w, 404, "no tabs connected")
			return
		}
		tabID = id
	}
	since := parseTabID(r.URL.Query().Get("since")) // reuses the same "parse int, 0 on error" helper

	entries, dropped, oldest := d.Events.Read(tabID, kind, since)
	out := make([]eventEntryWire, len(entries))
	for i, e := range entries {
		out[i] = eventEntryWire{Seq: e.Seq, Kind: e.Kind, Timestamp: e.Timestamp, Data: e.Data}
	}
	apiOK(w, map[string]any{
		"entries":            out,
		"dropped":            dropped,
		"oldestAvailableSeq": oldest,
	})
}
```

- [ ] **Step 5: Run the tests**

Run: `cd htrcli && go test ./internal/host/... -run 'TestEventsIngestAndRead|TestEventsGenerationEndpoint' -v`
Expected: PASS

- [ ] **Step 6: Run the full host package test suite to check for regressions**

Run: `cd htrcli && go test ./internal/host/... -v`
Expected: PASS (all existing tests plus the new ones)

- [ ] **Step 7: Commit**

```bash
git add htrcli/internal/host/daemon.go htrcli/internal/host/server.go htrcli/internal/host/server_test.go
git commit -m "feat(htrcli): expose event store over /api/events"
```

---

### Task 4: CLI `EventPoller` + `console read`/`console watch`

**Files:**
- Create: `htrcli/internal/commands/events.go`
- Create: `htrcli/internal/commands/console.go`
- Test: `htrcli/internal/commands/console_test.go`

**Interfaces:**
- Consumes: `api.Client.GetEvents(tabID *int, kind string, since int) (*api.EventsResponse, error)` (Task 1), `GetClient()`, `GetTabID()` (existing, `root.go`).
- Produces: `EventPoller{Client *api.Client, TabID *int, Kind string}`, `(*EventPoller) Read(since int) (*api.EventsResponse, error)`, `(*EventPoller) Watch(timeout time.Duration, since int, match func(api.EventEntry) bool) (*api.EventEntry, error)`.

- [ ] **Step 1: Write the failing test**

Create `htrcli/internal/commands/console_test.go`:

```go
package commands

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/u007/htrcli/internal/api"
)

func TestConsoleReadFormatsDroppedWarning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(api.ApiResponse{
			OK: true,
			Data: api.EventsResponse{
				Entries: []api.EventEntry{
					{Seq: 41, Kind: "console", Timestamp: 1000, Data: json.RawMessage(`{"level":"error","args":["boom"]}`)},
				},
				Dropped:            12,
				OldestAvailableSeq: 41,
			},
		})
	}))
	defer server.Close()

	c := api.NewClient(server.URL, "")
	poller := &EventPoller{Client: c, Kind: "console"}
	resp, err := poller.Read(40)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Dropped != 12 {
		t.Fatalf("want dropped=12, got %d", resp.Dropped)
	}

	out := formatConsoleEntries(resp)
	if !strings.Contains(out, "12 events were evicted") {
		t.Fatalf("expected drop warning in output, got: %s", out)
	}
	if !strings.Contains(out, "boom") {
		t.Fatalf("expected entry content in output, got: %s", out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd htrcli && go test ./internal/commands/... -run TestConsoleReadFormatsDroppedWarning -v`
Expected: FAIL — `EventPoller`, `formatConsoleEntries` undefined.

- [ ] **Step 3: Implement `EventPoller`**

Create `htrcli/internal/commands/events.go`:

```go
package commands

import (
	"context"
	"time"

	"github.com/u007/htrcli/internal/api"
)

// EventPoller wraps the cursor-based GET /api/events pattern shared by
// console/network/dialog subcommands (see docs/superpowers/specs/
// 2026-07-23-htrcli-event-buffer-design.md).
type EventPoller struct {
	Client *api.Client
	TabID  *int
	Kind   string
}

// Read returns one snapshot of events since the given cursor.
func (p *EventPoller) Read(since int) (*api.EventsResponse, error) {
	return p.Client.GetEvents(p.TabID, p.Kind, since)
}

// Watch long-polls (bounded by timeout) until match returns true for some
// entry newer than since, or the timeout elapses. Returns the matching
// entry, or nil with no error on timeout (callers decide whether that's a
// failure).
func (p *EventPoller) Watch(ctx context.Context, timeout time.Duration, since int, match func(api.EventEntry) bool) (*api.EventEntry, error) {
	deadline := time.Now().Add(timeout)
	const pollInterval = 250 * time.Millisecond
	cursor := since

	for {
		resp, err := p.Client.GetEvents(p.TabID, p.Kind, cursor)
		if err != nil {
			return nil, err
		}
		for _, e := range resp.Entries {
			if match(e) {
				return &e, nil
			}
			cursor = e.Seq
		}
		if time.Now().After(deadline) {
			return nil, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}
```

- [ ] **Step 4: Implement `console read`/`console watch` and the formatter**

Create `htrcli/internal/commands/console.go`:

```go
package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/u007/htrcli/internal/api"
	"github.com/u007/htrcli/internal/output"
)

type consoleEntryData struct {
	Level string   `json:"level"`
	Args  []string `json:"args"`
}

var (
	consoleSince   int
	consoleTimeout int
)

var consoleCmd = &cobra.Command{
	Use:   "console",
	Short: "Read captured console.* output from the page",
}

var consoleReadCmd = &cobra.Command{
	Use:   "read",
	Short: "Read console entries since a cursor",
	RunE: func(cmd *cobra.Command, args []string) error {
		tabID, err := GetTabID()
		if err != nil {
			return err
		}
		poller := &EventPoller{Client: GetClient(), TabID: tabID, Kind: "console"}
		resp, err := poller.Read(consoleSince)
		if err != nil {
			return err
		}
		if output.JSONOutput {
			output.PrintJSON(resp)
			return nil
		}
		fmt.Print(formatConsoleEntries(resp))
		return nil
	},
}

var consoleWatchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Block until a new console entry arrives or the timeout elapses",
	RunE: func(cmd *cobra.Command, args []string) error {
		tabID, err := GetTabID()
		if err != nil {
			return err
		}
		poller := &EventPoller{Client: GetClient(), TabID: tabID, Kind: "console"}
		entry, err := poller.Watch(
			context.Background(),
			time.Duration(consoleTimeout)*time.Millisecond,
			consoleSince,
			func(api.EventEntry) bool { return true }, // any new entry matches
		)
		if err != nil {
			return err
		}
		if entry == nil {
			return fmt.Errorf("no console entry arrived within %dms", consoleTimeout)
		}
		if output.JSONOutput {
			output.PrintJSON(entry)
			return nil
		}
		fmt.Print(formatConsoleEntries(&api.EventsResponse{Entries: []api.EventEntry{*entry}}))
		return nil
	},
}

// formatConsoleEntries renders a human-readable dump: a drop warning (if
// any) followed by one line per entry.
func formatConsoleEntries(resp *api.EventsResponse) string {
	var b strings.Builder
	if resp.Dropped > 0 {
		fmt.Fprintf(&b, "⚠ %d events were evicted (buffer cap reached)\n", resp.Dropped)
	}
	for _, e := range resp.Entries {
		var data consoleEntryData
		if err := json.Unmarshal(e.Data, &data); err != nil {
			fmt.Fprintf(&b, "[seq %d] <unparseable entry>\n", e.Seq)
			continue
		}
		fmt.Fprintf(&b, "[seq %d] %s: %s\n", e.Seq, data.Level, strings.Join(data.Args, " "))
	}
	return b.String()
}

func init() {
	consoleReadCmd.Flags().IntVar(&consoleSince, "since", 0, "only show entries after this sequence number")
	consoleWatchCmd.Flags().IntVar(&consoleSince, "since", 0, "only show entries after this sequence number")
	consoleWatchCmd.Flags().IntVar(&consoleTimeout, "timeout", 10000, "how long to wait for a new entry, in ms")
	consoleCmd.AddCommand(consoleReadCmd, consoleWatchCmd)
	rootCmd.AddCommand(consoleCmd)
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd htrcli && go test ./internal/commands/... -run TestConsoleReadFormatsDroppedWarning -v`
Expected: PASS

- [ ] **Step 6: Run the full commands package test suite to check for regressions**

Run: `cd htrcli && go test ./internal/commands/... -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add htrcli/internal/commands/events.go htrcli/internal/commands/console.go htrcli/internal/commands/console_test.go
git commit -m "feat(htrcli): add console read/watch CLI commands"
```

---

### Task 5: Daemon-restart resync signal in the native-messaging greeting

**Files:**
- Modify: `htrcli/internal/host/bridge.go`
- Test: `htrcli/internal/host/bridge_test.go`

**Interfaces:**
- Consumes: `Daemon.Events.Generation()` (Task 3).
- Produces: the relay's greeting `NativeMessage` gains a `Payload` carrying `{"generation": <int64>}`, so the extension can compare it across connections and detect a daemon restart.

- [ ] **Step 1: Write the failing test**

Append to `htrcli/internal/host/bridge_test.go` (check the existing test's helper names before duplicating setup — reuse them):

```go
func TestGreetingIncludesGeneration(t *testing.T) {
	d := NewDaemon()
	server, client := net.Pipe()
	defer client.Close()
	go RunRelayOverConn(d, server) // matches whatever the existing bridge tests call to start a relay loop; align the name with the function used in TestSendCommandTimeoutClearsPending's setup

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
}
```

Before writing this test, read `htrcli/internal/host/bridge_test.go` in full and use its actual relay-startup helper name and signature (the sketch above is illustrative — match the existing harness exactly rather than inventing a new one).

- [ ] **Step 2: Run test to verify it fails**

Run: `cd htrcli && go test ./internal/host/... -run TestGreetingIncludesGeneration -v`
Expected: FAIL — greeting payload has no `generation` key.

- [ ] **Step 3: Add the generation payload to the greeting**

In `htrcli/internal/host/bridge.go`, find the existing greeting construction:

```go
	if greeting, err := json.Marshal(NativeMessage{Type: "ping"}); err == nil {
```

Change it to include the generation in the payload:

```go
	genPayload, _ := json.Marshal(map[string]any{"generation": d.Events.Generation()})
	if greeting, err := json.Marshal(NativeMessage{Type: "ping", Payload: genPayload}); err == nil {
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd htrcli && go test ./internal/host/... -run TestGreetingIncludesGeneration -v`
Expected: PASS

- [ ] **Step 5: Run the full host package test suite to check for regressions**

Run: `cd htrcli && go test ./internal/host/... -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add htrcli/internal/host/bridge.go htrcli/internal/host/bridge_test.go
git commit -m "feat(htrcli): include daemon generation in relay greeting"
```

---

### Task 6: Extension-side durable event store (`chrome.storage.session`)

**Files:**
- Create: `src/background/eventStore.ts`
- Test: `src/background/eventStore.test.ts`

**Interfaces:**
- Consumes: `chrome.storage.session.get`/`set` (or `browser.storage.session` on Firefox via the existing `webextension-polyfill` shim already used elsewhere in `firefox/src/`).
- Produces: `recordConsoleEntry(tabId: number, entry: ConsoleEntryData): Promise<void>`, `getGeneration(): Promise<number | null>`, `setLastKnownGeneration(gen: number): Promise<void>`, `resetForResync(): Promise<void>` (clears local seq bookkeeping so the next flush replays everything), `flushPending(post: (tabId: number, kind: string, entries: StoredEntry[]) => Promise<boolean>): Promise<void>`.

- [ ] **Step 1: Write the failing test**

Create `src/background/eventStore.test.ts`:

```typescript
import { beforeEach, describe, expect, it, mock } from "bun:test";
import { flushPending, recordConsoleEntry } from "./eventStore";

// Minimal chrome.storage.session mock backed by an in-memory object, so the
// tests exercise the same read/write shape the real API provides.
function installStorageMock() {
	const store: Record<string, unknown> = {};
	(globalThis as any).chrome = {
		storage: {
			session: {
				get: (keys: string[]) =>
					Promise.resolve(
						Object.fromEntries(keys.map((k) => [k, store[k]])),
					),
				set: (values: Record<string, unknown>) => {
					Object.assign(store, values);
					return Promise.resolve();
				},
			},
		},
	};
	return store;
}

describe("eventStore console capture", () => {
	beforeEach(() => {
		installStorageMock();
	});

	it("assigns increasing seq numbers per tab", async () => {
		await recordConsoleEntry(1, { level: "log", args: ["a"] });
		await recordConsoleEntry(1, { level: "log", args: ["b"] });
		const posted: unknown[] = [];
		await flushPending(async (tabId, kind, entries) => {
			posted.push({ tabId, kind, entries });
			return true; // simulate a successful POST
		});
		const call = posted[0] as { entries: { seq: number }[] };
		expect(call.entries.map((e) => e.seq)).toEqual([1, 2]);
	});

	it("caps at 500 entries per (tab, kind) and reports eviction locally", async () => {
		for (let i = 0; i < 501; i++) {
			await recordConsoleEntry(1, { level: "log", args: [String(i)] });
		}
		let capturedEntries: { seq: number }[] = [];
		await flushPending(async (_tabId, _kind, entries) => {
			capturedEntries = entries as { seq: number }[];
			return true;
		});
		expect(capturedEntries.length).toBe(500);
		expect(capturedEntries[0].seq).toBe(2); // seq 1 evicted
	});

	it("retries a failed POST instead of dropping the entries", async () => {
		await recordConsoleEntry(1, { level: "log", args: ["retry-me"] });
		let attempts = 0;
		await flushPending(async () => {
			attempts++;
			return false; // simulate a failed POST
		});
		let secondAttemptEntries = 0;
		await flushPending(async (_tabId, _kind, entries) => {
			secondAttemptEntries = entries.length;
			return true;
		});
		expect(attempts).toBe(1);
		expect(secondAttemptEntries).toBe(1); // still there on the next flush
	});
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bun test src/background/eventStore.test.ts`
Expected: FAIL — `./eventStore` module does not exist.

- [ ] **Step 3: Implement `eventStore.ts`**

Create `src/background/eventStore.ts`:

```typescript
/**
 * Durable, capped event capture buffer. Backs console (and later
 * network/dialog) capture. Survives service-worker restarts via
 * chrome.storage.session — see docs/superpowers/specs/
 * 2026-07-23-htrcli-event-buffer-design.md.
 */

export interface ConsoleEntryData {
	level: "log" | "warn" | "error" | "info" | "debug";
	args: string[];
	source?: string;
}

export interface StoredEntry {
	seq: number;
	kind: "console";
	timestamp: number;
	data: ConsoleEntryData;
}

interface BucketState {
	entries: StoredEntry[]; // pending entries not yet successfully POSTed
	nextSeq: number;
}

const CAP_PER_BUCKET = 500;

function bucketKey(tabId: number, kind: string): string {
	return `events:${tabId}:${kind}`;
}

async function loadBucket(key: string): Promise<BucketState> {
	const stored = await chrome.storage.session.get([key]);
	return (stored[key] as BucketState) ?? { entries: [], nextSeq: 0 };
}

async function saveBucket(key: string, bucket: BucketState): Promise<void> {
	await chrome.storage.session.set({ [key]: bucket });
}

/**
 * Append a console entry for a tab, assigning it the next durable seq
 * number and evicting the oldest entry past CAP_PER_BUCKET. Entries stay
 * in chrome.storage.session until flushPending successfully POSTs them.
 */
// Track which tabs currently have pending entries, so flushPending doesn't
// need to enumerate all of chrome.storage.session (no such enumeration API
// exists) — every recordConsoleEntry call registers its tab here.
const knownTabs = new Set<number>();

export async function recordConsoleEntry(
	tabId: number,
	data: ConsoleEntryData,
): Promise<void> {
	knownTabs.add(tabId);
	const key = bucketKey(tabId, "console");
	const bucket = await loadBucket(key);
	bucket.nextSeq += 1;
	bucket.entries.push({
		seq: bucket.nextSeq,
		kind: "console",
		timestamp: Date.now(),
		data,
	});
	if (bucket.entries.length > CAP_PER_BUCKET) {
		bucket.entries.splice(0, bucket.entries.length - CAP_PER_BUCKET);
	}
	await saveBucket(key, bucket);
}

/**
 * Attempt to POST every tab's pending console entries via `post`. On
 * success (post resolves true), the flushed entries are cleared from
 * storage. On failure they remain, so the next flush retries them —
 * storage.session already holds the durable copy, so a failed POST never
 * loses data (see design doc, "Daemon unreachable when the extension
 * tries to POST").
 */
export async function flushPending(
	post: (
		tabId: number,
		kind: string,
		entries: StoredEntry[],
	) => Promise<boolean>,
): Promise<void> {
	for (const tabId of knownTabs) {
		const key = bucketKey(tabId, "console");
		const bucket = await loadBucket(key);
		if (bucket.entries.length === 0) continue;
		const ok = await post(tabId, "console", bucket.entries);
		if (ok) {
			bucket.entries = [];
			await saveBucket(key, bucket);
		}
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `bun test src/background/eventStore.test.ts`
Expected: PASS

- [ ] **Step 5: Run Biome check**

Run: `bun run check:fix`
Expected: no errors on the new file

- [ ] **Step 6: Commit**

```bash
git add src/background/eventStore.ts src/background/eventStore.test.ts
git commit -m "feat(extension): add durable console event capture buffer"
```

---

### Task 7: MAIN-world console capture script + wiring to the daemon

**Files:**
- Create: `src/contentScript/consoleCapture.ts`
- Modify: `src/manifest.ts`
- Modify: `src/background/nativeHost.ts`
- Modify: `src/background/index.ts`

**Interfaces:**
- Consumes: `eventStore.recordConsoleEntry`, `eventStore.flushPending` (Task 6).
- Produces: a `CONSOLE_ENTRY` runtime message type; a periodic flush loop in the background that calls `flushPending` and POSTs to the daemon's `/api/events/ingest` using the daemon's HTTP base URL/token, which the extension learns from the greeting's `generation` payload host (see below).

This task also closes a gap the design doc's "config propagation" note leaves implicit: the extension needs to know the daemon's HTTP base URL and token to POST proactively (screenshots avoid this because the daemon supplies `uploadUrl` per-trigger; events are extension-initiated, so this can't rely on a trigger). Reuse the exact value the daemon already knows about itself — the daemon is what starts the HTTP server, so extend the existing native `ping` greeting message to also carry `httpBaseUrl`/`token`, matching the `generation` field added in Task 5.

- [ ] **Step 1: Extend the daemon's greeting payload with connection info (Go side, small addition to Task 5's work)**

In `htrcli/internal/host/bridge.go`, change the greeting payload built in Task 5 from `{"generation": ...}` to also include the port/token the relay's `RunRelayOverConn` (or equivalent, per Task 5's actual helper name) already has in scope:

```go
	genPayload, _ := json.Marshal(map[string]any{
		"generation":  d.Events.Generation(),
		"httpBaseUrl": fmt.Sprintf("http://127.0.0.1:%d", port),
		"token":       bearerToken,
	})
```

(`port` and `bearerToken` must already be parameters/closure variables available wherever this greeting is constructed — check the surrounding function signature; if they are not currently threaded through to that call site, thread them the same way `handleScreenshotGet` receives `port, bearerToken` as parameters.)

Update the corresponding test from Task 5 to also assert `httpBaseUrl` and `token` are present in the decoded payload.

Run: `cd htrcli && go test ./internal/host/... -v`
Expected: PASS

- [ ] **Step 2: Store the connection info on the extension side**

In `src/background/nativeHost.ts`, add near the other module-level state (close to `portConfirmed`):

```typescript
let daemonHttpBaseUrl: string | null = null;
let daemonToken: string | undefined;
let lastKnownGeneration: number | null = null;

export function getDaemonConnectionInfo(): {
	httpBaseUrl: string | null;
	token: string | undefined;
} {
	return { httpBaseUrl: daemonHttpBaseUrl, token: daemonToken };
}

/** True the first time a new generation is observed after the first one. */
export function checkAndUpdateGeneration(generation: number): boolean {
	const isRestart = lastKnownGeneration !== null && lastKnownGeneration !== generation;
	lastKnownGeneration = generation;
	return isRestart;
}
```

In `handleNativeMessage`, inside the `if (msg.type === "ping")` branch (the existing `sendToNative({ type: "heartbeat" })` reply), parse the new payload fields before replying:

```typescript
	if (msg.type === "ping") {
		if (msg.payload) {
			const info = msg.payload as {
				generation?: number;
				httpBaseUrl?: string;
				token?: string;
			};
			if (info.httpBaseUrl) daemonHttpBaseUrl = info.httpBaseUrl;
			daemonToken = info.token;
			if (info.generation !== undefined) {
				const restarted = checkAndUpdateGeneration(info.generation);
				if (restarted) {
					console.warn(
						"[NativeHost] Daemon restarted (generation changed) — resync needed",
					);
					onDaemonRestart?.();
				}
			}
		}
		sendToNative({ type: "heartbeat" });
	}
```

Add the `NativePingMessage` interface's `payload` field (currently `NativePingMessage` has no payload — check its definition and add `payload?: unknown` matching how `NativeCaptureScreenshotMessage` declares its `payload`).

Add a registration seam mirroring `setScreenshotCapturer`:

```typescript
let onDaemonRestart: (() => void) | null = null;
export function setOnDaemonRestart(fn: () => void): void {
	onDaemonRestart = fn;
}
```

- [ ] **Step 3: Register the MAIN-world console capture script**

Create `src/contentScript/consoleCapture.ts`:

```typescript
/**
 * Injected as a MAIN-world content script (see src/manifest.ts). Wraps
 * console.* so the background can capture output without chrome.debugger —
 * works identically on Chrome and Firefox.
 */

const LEVELS = ["log", "warn", "error", "info", "debug"] as const;

for (const level of LEVELS) {
	const original = console[level].bind(console);
	console[level] = (...args: unknown[]) => {
		original(...args);
		try {
			window.postMessage(
				{
					source: "htr-ncontrol-console-capture",
					level,
					args: args.map((a) => {
						try {
							return typeof a === "string" ? a : JSON.stringify(a);
						} catch {
							return String(a);
						}
					}),
				},
				"*",
			);
		} catch {
			// intentionally not logged: must never let capture break the page's
			// own console usage
		}
	};
}
```

This posts a `window.postMessage`, not a direct `chrome.runtime.sendMessage`, because MAIN-world scripts injected via a manifest `content_scripts` entry (as opposed to `chrome.scripting.executeScript`) do not have access to `chrome.runtime` — only the isolated-world content script does. Add a small relay in the existing isolated-world `src/contentScript/index.ts`:

```typescript
window.addEventListener("message", (event) => {
	if (event.source !== window) return;
	const data = event.data;
	if (data?.source !== "htr-ncontrol-console-capture") return;
	chrome.runtime.sendMessage({
		type: "CONSOLE_ENTRY",
		level: data.level,
		args: data.args,
	});
});
```

Add this listener registration near the top of `src/contentScript/index.ts`'s setup code (check the file's existing structure for where other `window.addEventListener`/init calls live, and place it alongside them).

- [ ] **Step 4: Register the MAIN-world script in the manifest**

In `src/manifest.ts`, add a second entry to the `content_scripts` array (after the existing isolated-world entry):

```typescript
	content_scripts: [
		{
			matches: ["http://*/*", "https://*/*"],
			js: ["src/contentScript/index.ts"],
		},
		{
			matches: ["http://*/*", "https://*/*"],
			js: ["src/contentScript/consoleCapture.ts"],
			world: "MAIN",
			run_at: "document_start",
		},
	],
```

- [ ] **Step 5: Handle `CONSOLE_ENTRY` in the background and wire the flush loop**

In `src/background/index.ts`, add `"CONSOLE_ENTRY"` to the message union near the other message types:

```typescript
			| { type: "CONSOLE_ENTRY"; level: string; args: string[] }
```

Add a case to the `switch (message.type)`:

```typescript
				case "CONSOLE_ENTRY": {
					const msg = message as {
						type: "CONSOLE_ENTRY";
						level: string;
						args: string[];
					};
					if (sender.tab?.id) {
						await recordConsoleEntry(sender.tab.id, {
							level: msg.level as ConsoleEntryData["level"],
							args: msg.args,
						});
					}
					// Fire-and-forget from the page's perspective; no response needed.
					sendResponse({ success: true });
					break;
				}
```

Import `recordConsoleEntry`, `flushPending`, and the `ConsoleEntryData` type at the top of `src/background/index.ts`:

```typescript
import {
	type ConsoleEntryData,
	flushPending,
	recordConsoleEntry,
} from "./eventStore";
import { getDaemonConnectionInfo, setOnDaemonRestart } from "./nativeHost";
```

Near the bottom of the file, alongside the existing `setScreenshotCapturer`/`setReadyTabsProvider`/`startNativeHost()` wiring, add a periodic flush and the resync hook:

```typescript
const FLUSH_INTERVAL_MS = 2000;

async function postEventsToDaemon(
	tabId: number,
	kind: string,
	entries: unknown[],
): Promise<boolean> {
	const { httpBaseUrl, token } = getDaemonConnectionInfo();
	if (!httpBaseUrl) return false; // not connected yet; retry next tick
	try {
		const res = await fetch(`${httpBaseUrl}/api/events/ingest`, {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
				...(token ? { Authorization: `Bearer ${token}` } : {}),
			},
			body: JSON.stringify({ tabId, kind, entries }),
		});
		return res.ok;
	} catch (err) {
		console.warn("[HTR NControl] events POST failed:", err);
		return false;
	}
}

setInterval(() => {
	void flushPending(postEventsToDaemon);
}, FLUSH_INTERVAL_MS);

setOnDaemonRestart(() => {
	// The daemon's copy is gone; nothing to actively "replay" beyond what's
	// still pending in storage.session (unflushed entries will be sent on
	// the next tick as usual). Per docs/superpowers/specs/
	// 2026-07-23-htrcli-event-buffer-design.md this satisfies the
	// resync requirement for already-pending data; already-flushed entries
	// from before the restart are accepted as lost, matching the "Message
	// lost in-flight" non-goal, since the daemon had them at the time.
	console.warn("[HTR NControl] daemon restarted; resuming event flush");
});
```

- [ ] **Step 6: Manual smoke test**

Run: `bun run dev`, load the unpacked extension, open a page, run `console.log("hello from htr")` in that page's own devtools console, then run `htrcli console read` from a terminal with `htrcli serve` running and the extension connected.
Expected: within ~2 seconds (the flush interval), `htrcli console read` prints a line containing `hello from htr`.

- [ ] **Step 7: Run the full extension test suite and typecheck**

Run: `bun run test && bun run typecheck`
Expected: PASS, no type errors

- [ ] **Step 8: Commit**

```bash
git add src/contentScript/consoleCapture.ts src/contentScript/index.ts src/manifest.ts src/background/nativeHost.ts src/background/index.ts
git commit -m "feat(extension): capture console.* output and forward to daemon"
```

---

### Task 8: End-to-end verification against the design doc

**Files:** none (verification only)

- [ ] **Step 1: Restart-durability check**

With `htrcli serve` and the extension running, open a page and log more than 500 `console.log` calls in a tight loop (e.g. `for (let i = 0; i < 600; i++) console.log(i)`), then run `htrcli console read --since 0 --json` and confirm the response's `dropped` field is `100` (600 logged, 500-cap, oldest 100 evicted) and `entries` starts at seq 101.

- [ ] **Step 2: Service-worker restart check**

Force the service worker to restart (`chrome://extensions` → the extension's "service worker" link → close devtools, or use `chrome://serviceworker-internals` to terminate it), then immediately run `console.log("after restart")` on the page and `htrcli console read`. Confirm the entry appears with a seq number continuing from before the restart (not reset to 1).

- [ ] **Step 3: Daemon restart check**

With entries already captured, stop and restart `htrcli serve`. Confirm `htrcli console read` initially returns nothing (fresh daemon-side store), then within ~2 seconds (next flush tick) previously-unflushed-but-still-pending entries appear — and log explicitly in the PR/commit description that already-flushed pre-restart entries are expected to be gone, per the design doc's accepted daemon-restart scope.

- [ ] **Step 4: Firefox parity check**

Repeat Step 1 in Firefox (load the extension via `about:debugging`, using `firefox:build`). Confirm `htrcli console read` behaves identically — this is the phase's cross-browser proof point since console capture uses no CDP.
