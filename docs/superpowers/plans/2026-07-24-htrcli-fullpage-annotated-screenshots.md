# htrcli Full-Page + Annotated Screenshots Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `--full-page` and `--annotate <selectors>` flags to `htrcli screenshot`, working over both the CDP transport (direct Chrome debugging) and the extension/daemon transport (Chrome + Firefox), with the two flags composable.

**Architecture:** Screenshot options travel the existing spine: CLI flag → `GET /api/screenshot` query params → daemon `screenshotTrigger` payload → `capture_screenshot` native message → the extension's capturer. Full-page has two implementations — CDP `Page.captureScreenshot({captureBeyondViewport:true})` clipped to `Page.getLayoutMetrics` content size, and an extension scroll-and-stitch loop composited on an `OffscreenCanvas` in the background service worker. Annotate generalizes the existing `highlighter.ts` overlay into absolute-positioned numbered markers so they scroll with the page and land correctly in every stitched segment; it is extension-only, and CDP `--annotate` returns an explicit unsupported error rather than a silent no-op.

**Tech Stack:** Go (cobra CLI, stdlib `net/http`, gorilla/websocket for CDP), TypeScript (Chrome/Firefox WebExtension APIs, `OffscreenCanvas`, `chrome.scripting`), Bun test runner, Go's `testing` package.

## Global Constraints

- Package manager: `bun` only for the extension — never npm/yarn.
- Biome lint/format (tabs, double quotes) — run `bun run check:fix` before committing TS changes.
- Go tests: `go test ./...` from `htrcli/`.
- Async `chrome.runtime.onMessage` listeners must `return true` when responding asynchronously.
- Extension console/error logging prefix: `console.error/warn('[HTR NControl] ...')`.
- No new external dependencies (Go or npm) for this feature.
- **Annotate overlays MUST be `position: absolute`** (document-relative), never `position: fixed`. Fixed overlays stay pinned to the viewport, so during a full-page scroll-stitch they render at the wrong place in every segment. This is the correctness hinge for `--annotate --full-page` composition. Do **not** modify the existing `showHighlight`/`HIGHLIGHT_STYLES` (recording uses `position: fixed` deliberately) — add a separate annotation path.
- **`captureVisibleTab` is rate-limited** by Chrome (`MAX_CAPTURE_VISIBLE_TAB_CALLS_PER_SECOND`, ≈2/sec). The scroll-stitch loop MUST delay between captures (`CAPTURE_THROTTLE_MS = 600`) or a tall page throws "quota exceeded" partway through.
- **DevicePixelRatio scaling** (extension stitch path only): `captureVisibleTab` returns images at DPR scale (a 1280px viewport on a 2× display yields a 2560px PNG). The `OffscreenCanvas` composite MUST size itself in device pixels and place each segment at `scroll * dpr`. CDP `captureBeyondViewport` handles DPR internally — this is a stitch-path-only concern.

### Payload size & timeout budget (answers the "do size limits need adjusting?" question)

Screenshots already travel over HTTP POST-back (extension → daemon `POST /api/screenshot`), deliberately bypassing the 1 MB native-messaging frame cap. A full-page/stitched PNG can be several MB larger, but:

- **Bytes are fine.** `handleScreenshotPost` decodes the body with `json.NewDecoder` and there is **no** `http.MaxBytesReader` cap and no per-frame limit on this path. No byte-size change is required.
- **Time is the real limit.** The daemon's `GET /api/screenshot` waits up to `screenshotTimeout = 25s`, inside the CLI client's 30s HTTP timeout. A tall page's scroll → capture (throttled ≥600ms/segment) → composite → upload can exceed 25s and return a spurious 504. **Fix:** a *separate* longer budget for full-page requests only — `fullPageScreenshotTimeout = 45s` server-side and a 90s client-side timeout on full-page requests — rather than raising the shared 25s/30s constants (which would slacken normal viewport screenshots). 45s server wait + streaming a few MB over loopback stays within the server's existing 60s `WriteTimeout`, so that need not change.

---

### Task 1: CDP full-page screenshot (`internal/cdp/screenshot.go`)

**Files:**
- Create: `htrcli/internal/cdp/screenshot.go`
- Test: `htrcli/internal/cdp/screenshot_test.go`

**Interfaces:**
- Consumes: `cdp.Session` (`Call`, from `session.go`).
- Produces: `func ScreenshotFullPage(s *Session) ([]byte, error)`.

The existing viewport `Screenshot(s)` lives in `nav.go` and stays exactly where it is — this is a **sibling** function, not a replacement and not a duplicate. Do not add a bool parameter to `Screenshot` (CLAUDE.md: no behavior-changing optional params) and do not move it (keep the diff surgical).

- [ ] **Step 1: Write the failing test**

Create `htrcli/internal/cdp/screenshot_test.go`:

```go
package cdp

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/gorilla/websocket"
)

func TestScreenshotFullPageClipsToContentSize(t *testing.T) {
	wantPNG := []byte("full-page-png-bytes")
	var captureParams map[string]any
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		switch m.Method {
		case "Page.enable":
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
		case "Page.getLayoutMetrics":
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"cssContentSize": map[string]any{"x": 0, "y": 0, "width": 1280, "height": 3200},
			}})
		case "Page.captureScreenshot":
			json.Unmarshal(m.Params, &captureParams)
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"data": base64.StdEncoding.EncodeToString(wantPNG),
			}})
		default:
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
		}
	})

	s, err := Dial(url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer s.Close()

	got, err := ScreenshotFullPage(s)
	if err != nil {
		t.Fatalf("ScreenshotFullPage: %v", err)
	}
	if string(got) != string(wantPNG) {
		t.Fatalf("decoded bytes = %q, want %q", got, wantPNG)
	}
	if captureParams["captureBeyondViewport"] != true {
		t.Fatalf("captureBeyondViewport = %v, want true", captureParams["captureBeyondViewport"])
	}
	clip, ok := captureParams["clip"].(map[string]any)
	if !ok {
		t.Fatalf("clip missing or wrong type: %v", captureParams["clip"])
	}
	if clip["width"] != float64(1280) || clip["height"] != float64(3200) {
		t.Fatalf("clip = %v, want width 1280 height 3200", clip)
	}
}

func TestScreenshotFullPageErrorsOnZeroContentSize(t *testing.T) {
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		switch m.Method {
		case "Page.getLayoutMetrics":
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"cssContentSize": map[string]any{"width": 0, "height": 0},
			}})
		default:
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
		}
	})
	s, _ := Dial(url)
	defer s.Close()

	if _, err := ScreenshotFullPage(s); err == nil {
		t.Fatal("expected error on zero content size, got nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd htrcli && go test ./internal/cdp/... -run TestScreenshotFullPage -v`
Expected: FAIL — `ScreenshotFullPage` undefined.

- [ ] **Step 3: Implement `ScreenshotFullPage`**

Create `htrcli/internal/cdp/screenshot.go`:

```go
package cdp

import (
	"encoding/base64"
	"fmt"
)

// layoutMetrics is the subset of Page.getLayoutMetrics we consume. cssContentSize
// is the full scrollable document size in CSS pixels (modern Chrome); it is what
// the full-page clip must match.
type layoutMetrics struct {
	CSSContentSize struct {
		Width  float64 `json:"width"`
		Height float64 `json:"height"`
	} `json:"cssContentSize"`
}

// ScreenshotFullPage captures the entire page, including content below the fold,
// via Page.captureScreenshot{captureBeyondViewport:true} clipped to the document's
// CSS content size (from Page.getLayoutMetrics). Sibling to Screenshot (viewport-
// only) in nav.go; the plain viewport path is intentionally left unchanged.
func ScreenshotFullPage(s *Session) ([]byte, error) {
	if err := s.Call("Page.enable", nil, nil); err != nil {
		return nil, fmt.Errorf("Page.enable: %w", err)
	}
	var m layoutMetrics
	if err := s.Call("Page.getLayoutMetrics", nil, &m); err != nil {
		return nil, fmt.Errorf("Page.getLayoutMetrics: %w", err)
	}
	w, h := m.CSSContentSize.Width, m.CSSContentSize.Height
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("could not determine page content size (got %gx%g); requires Chrome with cssContentSize support", w, h)
	}

	var res struct {
		Data string `json:"data"`
	}
	params := map[string]any{
		"format":                "png",
		"captureBeyondViewport": true,
		"clip": map[string]any{
			"x":      0.0,
			"y":      0.0,
			"width":  w,
			"height": h,
			"scale":  1.0,
		},
	}
	if err := s.Call("Page.captureScreenshot", params, &res); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(res.Data)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd htrcli && go test ./internal/cdp/... -run TestScreenshotFullPage -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add htrcli/internal/cdp/screenshot.go htrcli/internal/cdp/screenshot_test.go
git commit -m "feat(htrcli): add CDP full-page screenshot via captureBeyondViewport"
```

---

### Task 2: Daemon-side screenshot options plumbing

**Files:**
- Modify: `htrcli/internal/host/daemon.go`
- Modify: `htrcli/internal/host/server.go`
- Test: `htrcli/internal/host/server_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces: exported `host.ScreenshotOpts{FullPage bool; Annotate json.RawMessage}`; changed signature `func (d *Daemon) TriggerScreenshot(tabID int, commandID, uploadURL, token string, opts ScreenshotOpts) (<-chan shotResult, error)`; `handleScreenshotGet` reads `?fullPage=&annotate=` query params and uses `fullPageScreenshotTimeout` for full-page requests. The native `capture_screenshot` payload gains `fullPage` and `annotate` fields.

`Annotate` is carried as opaque `json.RawMessage` (a JSON array of selector objects) because the `host` package does not import `api`; the extension parses it. Empty/absent means "no annotation".

- [ ] **Step 1: Write the failing test**

Append to `htrcli/internal/host/server_test.go`:

```go
func TestTriggerScreenshotIncludesOptions(t *testing.T) {
	d := host.NewDaemon()
	var written [][]byte
	rc := d.AddConn(func(msg []byte) error {
		written = append(written, msg)
		return nil
	})
	d.RegisterTab(rc, 1, host.TabInfo{ID: 1, URL: "https://a.com", Title: "A", Active: true})

	annotate := json.RawMessage(`[{"selector":"button"}]`)
	_, err := d.TriggerScreenshot(1, "cmd1", "http://127.0.0.1:3845/api/screenshot", "",
		host.ScreenshotOpts{FullPage: true, Annotate: annotate})
	if err != nil {
		t.Fatalf("TriggerScreenshot: %v", err)
	}
	if len(written) != 1 {
		t.Fatalf("want 1 native message written, got %d", len(written))
	}

	var nm struct {
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(written[0], &nm); err != nil {
		t.Fatalf("unmarshal native message: %v", err)
	}
	if nm.Type != "capture_screenshot" {
		t.Fatalf("type = %q, want capture_screenshot", nm.Type)
	}
	var payload struct {
		UploadURL string          `json:"uploadUrl"`
		FullPage  bool            `json:"fullPage"`
		Annotate  json.RawMessage `json:"annotate"`
	}
	if err := json.Unmarshal(nm.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if !payload.FullPage {
		t.Fatalf("payload.fullPage = false, want true")
	}
	if string(payload.Annotate) != `[{"selector":"button"}]` {
		t.Fatalf("payload.annotate = %s, want the selector array", payload.Annotate)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd htrcli && go test ./internal/host/... -run TestTriggerScreenshotIncludesOptions -v`
Expected: FAIL — `host.ScreenshotOpts` undefined and `TriggerScreenshot` has the old arity.

- [ ] **Step 3: Extend the trigger payload and `TriggerScreenshot`**

In `htrcli/internal/host/daemon.go`, replace the `screenshotTrigger` struct (currently just `UploadURL`/`Token`) with:

```go
// screenshotTrigger is the payload of a capture_screenshot native message. It
// tells the extension where to upload the captured PNG over HTTP, plus the
// capture options. Annotate is an opaque JSON array of selector objects the
// extension parses (host does not import api).
type screenshotTrigger struct {
	UploadURL string          `json:"uploadUrl"`
	Token     string          `json:"token,omitempty"`
	FullPage  bool            `json:"fullPage,omitempty"`
	Annotate  json.RawMessage `json:"annotate,omitempty"`
}

// ScreenshotOpts carries the full-page / annotate options from the HTTP layer
// down to TriggerScreenshot. Annotate is a JSON array of selector objects.
type ScreenshotOpts struct {
	FullPage bool
	Annotate json.RawMessage
}
```

Change `TriggerScreenshot` to accept and forward the options:

```go
// TriggerScreenshot asks the extension (via the relay) to capture tab tabID and
// POST the PNG back to uploadURL. Returns a channel that receives the upload.
// Screenshots are deliberately NOT returned over the relay: a base64 PNG
// routinely exceeds the 1 MB native-messaging frame limit, so they travel over
// HTTP instead (see POST /api/screenshot).
func (d *Daemon) TriggerScreenshot(tabID int, commandID, uploadURL, token string, opts ScreenshotOpts) (<-chan shotResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	rc, ok := d.findOwner(tabID)
	if !ok {
		return nil, fmt.Errorf("tab %d not connected", tabID)
	}

	ch := make(chan shotResult, 1)
	d.pendingShots[commandID] = ch

	msg := NativeMessage{
		Type:      "capture_screenshot",
		TabID:     tabID,
		CommandID: commandID,
		Payload: mustMarshal(screenshotTrigger{
			UploadURL: uploadURL,
			Token:     token,
			FullPage:  opts.FullPage,
			Annotate:  opts.Annotate,
		}),
	}
	data, _ := json.Marshal(msg)
	if err := rc.write(data); err != nil {
		delete(d.pendingShots, commandID)
		return nil, fmt.Errorf("relay write: %w", err)
	}
	return ch, nil
}
```

- [ ] **Step 4: Read the query params and pick the timeout in `handleScreenshotGet`**

In `htrcli/internal/host/server.go`, add the full-page timeout constant next to `screenshotTimeout`:

```go
// fullPageScreenshotTimeout gives full-page captures (scroll-stitch-composite-
// upload) a longer budget than a single viewport shot, without slackening the
// normal 25s path. Stays under the server's 60s WriteTimeout. The CLI client
// uses a matching 90s HTTP timeout for full-page requests (see api.GetScreenshotOpts).
const fullPageScreenshotTimeout = 45 * time.Second
```

Replace the body of `handleScreenshotGet` with the options-aware version:

```go
func handleScreenshotGet(w http.ResponseWriter, r *http.Request, d *Daemon, port int, bearerToken string) {
	tabID, ok := d.FirstTabID()
	if !ok {
		apiError(w, 404, "no tabs connected")
		return
	}

	fullPage := r.URL.Query().Get("fullPage") == "true"
	var annotate json.RawMessage
	if raw := r.URL.Query().Get("annotate"); raw != "" {
		annotate = json.RawMessage(raw)
	}

	commandID := generateID()
	uploadURL := fmt.Sprintf("http://127.0.0.1:%d/api/screenshot", port)

	ch, err := d.TriggerScreenshot(tabID, commandID, uploadURL, bearerToken,
		ScreenshotOpts{FullPage: fullPage, Annotate: annotate})
	if err != nil {
		apiError(w, 404, err.Error())
		return
	}

	wait := screenshotTimeout
	if fullPage {
		wait = fullPageScreenshotTimeout
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case res := <-ch:
		if res.err != "" {
			apiError(w, 502, "screenshot capture failed: "+res.err)
			return
		}
		apiOK(w, res.data)
	case <-timer.C:
		d.ResolveScreenshot(commandID, "", "") // drop the pending entry
		apiError(w, 504, "screenshot timed out")
	}
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd htrcli && go test ./internal/host/... -run TestTriggerScreenshotIncludesOptions -v`
Expected: PASS

- [ ] **Step 6: Run the full host package suite for regressions**

Run: `cd htrcli && go test ./internal/host/... -v`
Expected: PASS (existing screenshot/tabs/events tests still green; note the old `TriggerScreenshot` call site is only `handleScreenshotGet`, now updated).

- [ ] **Step 7: Commit**

```bash
git add htrcli/internal/host/daemon.go htrcli/internal/host/server.go htrcli/internal/host/server_test.go
git commit -m "feat(htrcli): thread full-page/annotate options through the daemon screenshot path"
```

---

### Task 3: API client `GetScreenshotOpts` + `ScreenshotOptions` type

**Files:**
- Modify: `htrcli/internal/api/types.go`
- Modify: `htrcli/internal/api/client.go`
- Test: `htrcli/internal/api/client_test.go`

**Interfaces:**
- Consumes: existing `Client` internals.
- Produces: `api.ScreenshotOptions{FullPage bool; Annotate []TargetSelector}`; `func (c *Client) GetScreenshotOpts(opts ScreenshotOptions) (string, error)` (returns base64 PNG); `GetScreenshot()` becomes a thin wrapper delegating to it; a new private `doRequestClient(client *http.Client, method, path string, body any) ([]byte, error)` so full-page can use a longer-timeout client without changing the shared 30s one.

- [ ] **Step 1: Write the failing test**

Append to `htrcli/internal/api/client_test.go`:

```go
func TestGetScreenshotOptsSendsFullPageAndAnnotate(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ApiResponse{OK: true, Data: "QUJD"}) // base64 "ABC"
	}))
	defer server.Close()

	c := NewClient(server.URL, "")
	data, err := c.GetScreenshotOpts(ScreenshotOptions{
		FullPage: true,
		Annotate: []TargetSelector{{Selector: "button"}, {Role: "link"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data != "QUJD" {
		t.Fatalf("data = %q, want QUJD", data)
	}
	q, err := neturl.ParseQuery(gotQuery)
	if err != nil {
		t.Fatalf("parse query %q: %v", gotQuery, err)
	}
	if q.Get("fullPage") != "true" {
		t.Fatalf("fullPage = %q, want true", q.Get("fullPage"))
	}
	if q.Get("annotate") != `[{"selector":"button"},{"role":"link"}]` {
		t.Fatalf("annotate = %q, want the selector JSON array", q.Get("annotate"))
	}
}

func TestGetScreenshotViewportSendsNoOptions(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ApiResponse{OK: true, Data: "QUJD"})
	}))
	defer server.Close()

	c := NewClient(server.URL, "")
	if _, err := c.GetScreenshot(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotQuery != "" {
		t.Fatalf("viewport screenshot should send no query params, got %q", gotQuery)
	}
}
```

Add `neturl "net/url"` to the imports of `client_test.go`.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd htrcli && go test ./internal/api/... -run 'TestGetScreenshotOpts|TestGetScreenshotViewport' -v`
Expected: FAIL — `ScreenshotOptions` and `GetScreenshotOpts` undefined.

- [ ] **Step 3: Add the `ScreenshotOptions` type**

Append to `htrcli/internal/api/types.go`:

```go
// ScreenshotOptions controls htrcli screenshot capture. Annotate is a list of
// selectors whose matched elements get numbered overlay boxes drawn before
// capture. Empty options = plain viewport screenshot (unchanged behavior).
type ScreenshotOptions struct {
	FullPage bool             `json:"fullPage,omitempty"`
	Annotate []TargetSelector `json:"annotate,omitempty"`
}
```

- [ ] **Step 4: Refactor `doRequest` and add the options-aware screenshot methods**

In `htrcli/internal/api/client.go`, add `"net/url"` to the imports. Replace the existing `doRequest` method with a delegating pair (keeps the shared 30s client for everything else, lets full-page pass a longer-timeout client):

```go
// doRequest executes a request on the shared HTTP client (30s timeout).
func (c *Client) doRequest(method, path string, body any) ([]byte, error) {
	return c.doRequestClient(c.HTTPClient, method, path, body)
}

// doRequestClient is doRequest with an explicit client, so callers that need a
// different timeout (e.g. full-page screenshots) don't mutate the shared one.
func (c *Client) doRequestClient(client *http.Client, method, path string, body any) ([]byte, error) {
	url := c.BaseURL + path

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, &ConnectionError{Message: fmt.Sprintf("failed to create request: %v", err)}
	}

	req.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, &ConnectionError{Message: fmt.Sprintf("failed to connect to server: %v", err)}
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode == 403 {
		return nil, &AuthError{Message: "authentication failed: invalid or missing token"}
	}

	if resp.StatusCode == 404 {
		var apiResp ApiResponse
		if json.Unmarshal(data, &apiResp) == nil && apiResp.Error != "" {
			return nil, &NotFoundError{Message: apiResp.Error}
		}
		return nil, &NotFoundError{Message: "resource not found"}
	}

	if resp.StatusCode >= 400 {
		return nil, &APIError{StatusCode: resp.StatusCode, Message: string(data)}
	}

	return data, nil
}
```

Replace the existing `GetScreenshot` method with the options-aware version plus a thin wrapper:

```go
// GetScreenshot captures a plain viewport screenshot and returns base64 PNG data.
func (c *Client) GetScreenshot() (string, error) {
	return c.GetScreenshotOpts(ScreenshotOptions{})
}

// GetScreenshotOpts captures a screenshot with the given options and returns the
// base64 PNG data. Full-page requests use a longer HTTP timeout because the
// extension scroll-stitch (or CDP captureBeyondViewport) can exceed the shared
// 30s client budget; it must stay above the daemon's fullPageScreenshotTimeout.
func (c *Client) GetScreenshotOpts(opts ScreenshotOptions) (string, error) {
	q := url.Values{}
	if opts.FullPage {
		q.Set("fullPage", "true")
	}
	if len(opts.Annotate) > 0 {
		raw, err := json.Marshal(opts.Annotate)
		if err != nil {
			return "", fmt.Errorf("failed to marshal annotate selectors: %w", err)
		}
		q.Set("annotate", string(raw))
	}
	path := "/api/screenshot"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}

	client := c.HTTPClient
	if opts.FullPage {
		client = &http.Client{Timeout: 90 * time.Second}
	}

	data, err := c.doRequestClient(client, "GET", path, nil)
	if err != nil {
		return "", err
	}

	var apiResp ApiResponse
	if err := json.Unmarshal(data, &apiResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}
	if !apiResp.OK {
		return "", &APIError{Message: apiResp.Error}
	}
	if apiResp.Data == nil {
		return "", fmt.Errorf("no screenshot data received")
	}

	dataBytes, err := json.Marshal(apiResp.Data)
	if err != nil {
		return "", fmt.Errorf("failed to marshal data: %w", err)
	}
	var screenshot string
	if err := json.Unmarshal(dataBytes, &screenshot); err != nil {
		return "", fmt.Errorf("failed to parse screenshot: %w", err)
	}
	return screenshot, nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd htrcli && go test ./internal/api/... -run 'TestGetScreenshotOpts|TestGetScreenshotViewport' -v`
Expected: PASS

- [ ] **Step 6: Run the full api package suite for regressions**

Run: `cd htrcli && go test ./internal/api/... -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add htrcli/internal/api/types.go htrcli/internal/api/client.go htrcli/internal/api/client_test.go
git commit -m "feat(htrcli): add ScreenshotOptions and GetScreenshotOpts client method"
```

---

### Task 4: CLI flags + transport dispatch on `screenshot`

**Files:**
- Modify: `htrcli/internal/commands/inspect.go`
- Modify: `htrcli/internal/commands/cdp_exec.go`
- Test: `htrcli/internal/commands/commands_test.go`

**Interfaces:**
- Consumes: `api.ScreenshotOptions`, `Client.GetScreenshotOpts` (Task 3), `cdp.ScreenshotFullPage` (Task 1), `parseSelector`, `errUnsupportedCDP`, `UseCDP`.
- Produces: `--full-page`, `--annotate` flags on `screenshotCmd`; pure helper `func parseAnnotateSelectors(s string) []api.TargetSelector`; `runScreenshotCDP(path string, fullPage bool, annotate []api.TargetSelector) error`.

Decision on `--annotate` parsing: the value is a single comma-separated string (`"button,input,role=button"`); split on comma, run each token through `parseSelector`. **Known edge:** a comma *inside* one CSS selector (e.g. `:is(a,b)`) is not supported — each comma starts a new selector. Documented here rather than left to the implementer.

- [ ] **Step 1: Write the failing test**

Append to `htrcli/internal/commands/commands_test.go`:

```go
import "github.com/u007/htrcli/internal/api" // add to the existing import block

func TestParseAnnotateSelectors(t *testing.T) {
	got := parseAnnotateSelectors("button, role=link ,#submit")
	if len(got) != 3 {
		t.Fatalf("want 3 selectors, got %d: %+v", len(got), got)
	}
	if got[0].Selector != "button" {
		t.Errorf("got[0].Selector = %q, want button", got[0].Selector)
	}
	if got[1].Role != "link" {
		t.Errorf("got[1].Role = %q, want link (whitespace trimmed)", got[1].Role)
	}
	if got[2].Selector != "#submit" {
		t.Errorf("got[2].Selector = %q, want #submit", got[2].Selector)
	}
}

func TestParseAnnotateSelectorsEmpty(t *testing.T) {
	if got := parseAnnotateSelectors(""); got != nil {
		t.Fatalf("empty string should yield nil, got %+v", got)
	}
}

var _ = api.TargetSelector{} // keep the api import referenced if unused elsewhere
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd htrcli && go test ./internal/commands/... -run TestParseAnnotateSelectors -v`
Expected: FAIL — `parseAnnotateSelectors` undefined.

- [ ] **Step 3: Add flags, the parse helper, and the new dispatch to `screenshotCmd`**

In `htrcli/internal/commands/inspect.go`, add package-level flag vars near the top (after the imports, before `findCmd`):

```go
var (
	screenshotFullPage bool
	screenshotAnnotate string
)

// parseAnnotateSelectors splits a comma-separated --annotate value into selectors,
// one per token, reusing parseSelector so prefix forms (role=, name=, xpath=, …)
// work. A comma inside a single CSS selector is not supported — each comma starts
// a new selector. Returns nil for an empty value.
func parseAnnotateSelectors(s string) []api.TargetSelector {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var out []api.TargetSelector
	for _, tok := range strings.Split(s, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		out = append(out, *parseSelector(tok))
	}
	return out
}
```

Add `"strings"` to the `inspect.go` import block.

Replace the `screenshotCmd` definition with the options-aware version:

```go
var screenshotCmd = &cobra.Command{
	Use:   "screenshot [path]",
	Short: "Take screenshot (viewport, --full-page, and/or --annotate)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := ""
		if len(args) > 0 {
			path = args[0]
		}
		annotate := parseAnnotateSelectors(screenshotAnnotate)

		if UseCDP() {
			return runScreenshotCDP(path, screenshotFullPage, annotate)
		}

		c := GetClient()
		data, err := c.GetScreenshotOpts(api.ScreenshotOptions{
			FullPage: screenshotFullPage,
			Annotate: annotate,
		})
		if err != nil {
			return err
		}

		if output.JSONOutput {
			output.PrintJSON(map[string]string{"screenshot": data})
			return nil
		}

		imgData, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return fmt.Errorf("failed to decode screenshot: %w", err)
		}

		out := path
		if out == "" {
			out = filepath.Join(os.TempDir(), fmt.Sprintf("screenshot-%d.png", time.Now().UnixMilli()))
		}
		if err := os.WriteFile(out, imgData, 0644); err != nil {
			return fmt.Errorf("failed to write screenshot: %w", err)
		}

		fmt.Printf("Screenshot saved to %s\n", out)
		return nil
	},
}
```

Register the flags in the existing `init()` in `inspect.go` — add these two lines (keep the existing `rootCmd.AddCommand(screenshotCmd)` line):

```go
	screenshotCmd.Flags().BoolVar(&screenshotFullPage, "full-page", false, "capture the entire scrollable page, not just the viewport")
	screenshotCmd.Flags().StringVar(&screenshotAnnotate, "annotate", "", "comma-separated selectors to draw numbered overlay boxes on before capture (extension transport only)")
```

- [ ] **Step 4: Update the CDP screenshot runner**

In `htrcli/internal/commands/cdp_exec.go`, replace `runScreenshotCDP` with the options-aware version. Annotate is extension-only, so CDP `--annotate` fails loud (never a silent no-op):

```go
// runScreenshotCDP captures via CDP: viewport (Screenshot) or full page
// (ScreenshotFullPage). --annotate is not supported on the CDP transport (the
// overlay is drawn by the extension content script), so it errors explicitly
// rather than silently ignoring the flag.
func runScreenshotCDP(path string, fullPage bool, annotate []api.TargetSelector) error {
	if len(annotate) > 0 {
		return errUnsupportedCDP("screenshot --annotate")
	}

	s, _, err := cdpSession()
	if err != nil {
		return err
	}
	defer s.Close()

	var png []byte
	if fullPage {
		png, err = cdp.ScreenshotFullPage(s)
	} else {
		png, err = cdp.Screenshot(s)
	}
	if err != nil {
		return err
	}

	out := path
	if out == "" {
		out = filepath.Join(os.TempDir(), fmt.Sprintf("screenshot-%d.png", time.Now().UnixMilli()))
	}
	if err := os.WriteFile(out, png, 0644); err != nil {
		return fmt.Errorf("failed to write screenshot: %w", err)
	}
	if output.JSONOutput {
		output.PrintJSON(map[string]string{"screenshot": out})
		return nil
	}
	fmt.Printf("Screenshot saved to %s\n", out)
	return nil
}
```

Ensure `cdp_exec.go` imports `"github.com/u007/htrcli/internal/api"` (add if the file does not already import it).

- [ ] **Step 5: Run test to verify it passes**

Run: `cd htrcli && go test ./internal/commands/... -run TestParseAnnotateSelectors -v`
Expected: PASS

- [ ] **Step 6: Build + full commands suite for regressions**

Run: `cd htrcli && go build ./... && go test ./internal/commands/... -v`
Expected: build succeeds, all tests PASS.

- [ ] **Step 7: Commit**

```bash
git add htrcli/internal/commands/inspect.go htrcli/internal/commands/cdp_exec.go htrcli/internal/commands/commands_test.go
git commit -m "feat(htrcli): add --full-page/--annotate flags and CDP dispatch to screenshot"
```

---

### Task 5: Extension annotation overlay (generalize `highlighter.ts`)

**Files:**
- Modify: `src/contentScript/highlighter.ts`
- Modify: `src/contentScript/index.ts`
- Modify: `src/types/recording.ts`
- Test: `src/contentScript/highlighter.test.ts`

**Interfaces:**
- Consumes: `findAllElements` (from `elementFinder.ts`), `TargetSelector` (from `types/recording.ts`).
- Produces: `AnnotationBox{number,x,y,width,height}`, `toAnnotationBox(rect, scrollX, scrollY, number)`, `showAnnotations(boxes)`, `removeAnnotations()`; new message types `ANNOTATE_ELEMENTS` / `CLEAR_ANNOTATIONS` handled in the content script.

The existing `showHighlight`/`HIGHLIGHT_STYLES` (used by recording, `position: fixed`) are left untouched — this adds a parallel absolute-positioned annotation path.

- [ ] **Step 1: Write the failing test**

Create `src/contentScript/highlighter.test.ts`:

```typescript
import { describe, expect, it } from "bun:test";
import { type AnnotationBox, toAnnotationBox } from "./highlighter";

describe("toAnnotationBox", () => {
	it("converts a viewport rect to document-absolute coordinates", () => {
		const rect = { left: 10, top: 20, width: 30, height: 40 } as DOMRect;
		const box: AnnotationBox = toAnnotationBox(rect, 0, 100, 1);
		expect(box).toEqual({ number: 1, x: 10, y: 120, width: 30, height: 40 });
	});

	it("applies horizontal scroll offset too", () => {
		const rect = { left: 5, top: 5, width: 8, height: 8 } as DOMRect;
		const box = toAnnotationBox(rect, 50, 0, 7);
		expect(box.x).toBe(55);
		expect(box.number).toBe(7);
	});
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bun test src/contentScript/highlighter.test.ts`
Expected: FAIL — `toAnnotationBox` / `AnnotationBox` not exported.

- [ ] **Step 3: Add the annotation overlay to `highlighter.ts`**

Append to `src/contentScript/highlighter.ts`:

```typescript
/**
 * Numbered annotation overlay — a generalization of the single-element highlight
 * above, used by `htrcli screenshot --annotate`. Unlike showHighlight (which is
 * position:fixed for a live recording overlay), these markers are position:absolute
 * (document-relative) so they scroll with the page and land in the correct place
 * in every segment of a full-page scroll-stitch capture.
 */

export interface AnnotationBox {
	number: number;
	x: number;
	y: number;
	width: number;
	height: number;
}

let annotationContainer: HTMLDivElement | null = null;

/**
 * Convert a getBoundingClientRect() (viewport-relative) into document-absolute
 * coordinates by adding the current scroll offset, and tag it with a marker number.
 */
export function toAnnotationBox(
	rect: DOMRect,
	scrollX: number,
	scrollY: number,
	number: number,
): AnnotationBox {
	return {
		number,
		x: rect.left + scrollX,
		y: rect.top + scrollY,
		width: rect.width,
		height: rect.height,
	};
}

/** Draw numbered overlay boxes. Replaces any existing annotation overlay. */
export function showAnnotations(boxes: AnnotationBox[]): void {
	removeAnnotations();

	const container = document.createElement("div");
	container.id = "htrncontrol-annotations";
	container.setAttribute("data-htrncontrol-ignore", "true");
	Object.assign(container.style, {
		position: "absolute",
		top: "0",
		left: "0",
		width: "0",
		height: "0",
		pointerEvents: "none",
		zIndex: "2147483647",
	});

	for (const b of boxes) {
		const rect = document.createElement("div");
		Object.assign(rect.style, {
			position: "absolute",
			left: `${b.x}px`,
			top: `${b.y}px`,
			width: `${b.width}px`,
			height: `${b.height}px`,
			border: "2px solid #ef4444",
			boxSizing: "border-box",
			pointerEvents: "none",
		});

		const label = document.createElement("div");
		label.textContent = String(b.number);
		Object.assign(label.style, {
			position: "absolute",
			left: `${b.x}px`,
			top: `${b.y}px`,
			transform: "translateY(-100%)",
			background: "#ef4444",
			color: "#fff",
			font: "bold 12px sans-serif",
			padding: "1px 4px",
			borderRadius: "3px",
			pointerEvents: "none",
			whiteSpace: "nowrap",
		});

		container.appendChild(rect);
		container.appendChild(label);
	}

	document.body.appendChild(container);
	annotationContainer = container;
}

/** Remove the annotation overlay from the DOM. */
export function removeAnnotations(): void {
	if (annotationContainer?.parentNode) {
		annotationContainer.parentNode.removeChild(annotationContainer);
	}
	annotationContainer = null;
}
```

- [ ] **Step 4: Add the message types**

In `src/types/recording.ts`, add to the `MessageType` union (find the existing union that lists `"HIGHLIGHT_ELEMENT"`, `"HIDE_HIGHLIGHT"`, etc.) the two new literals:

```typescript
	| "ANNOTATE_ELEMENTS"
	| "CLEAR_ANNOTATIONS"
```

And add the matching interfaces next to `HighlightElementMessage`:

```typescript
export interface AnnotateElementsMessage {
	type: "ANNOTATE_ELEMENTS";
	targets: TargetSelector[];
}

export interface ClearAnnotationsMessage {
	type: "CLEAR_ANNOTATIONS";
}
```

- [ ] **Step 5: Handle the messages in the content script**

In `src/contentScript/index.ts`, extend the highlighter import (line ~23) to include the new functions:

```typescript
import {
	hideHighlight,
	removeAnnotations,
	removeHighlight,
	showAnnotations,
	showHighlight,
	toAnnotationBox,
	type AnnotationBox,
} from "./highlighter";
```

Ensure `findAllElements` is imported from `./elementFinder` (add to the existing elementFinder import if not already present), and `TargetSelector`/`AnnotateElementsMessage` from the types module.

Add two cases to the message `switch` (next to the existing `HIGHLIGHT_ELEMENT` / `HIDE_HIGHLIGHT` cases):

```typescript
		case "ANNOTATE_ELEMENTS": {
			const annMsg = message as AnnotateElementsMessage;
			try {
				const boxes: AnnotationBox[] = [];
				let n = 1;
				for (const target of annMsg.targets) {
					for (const el of findAllElements(target)) {
						boxes.push(
							toAnnotationBox(
								el.getBoundingClientRect(),
								window.scrollX,
								window.scrollY,
								n,
							),
						);
						n += 1;
					}
				}
				showAnnotations(boxes);
				sendResponse({ success: true, count: boxes.length });
			} catch (error) {
				console.warn("[HTR NControl] Failed to annotate elements:", error);
				sendResponse({ success: false, error: String(error) });
			}
			break;
		}

		case "CLEAR_ANNOTATIONS":
			removeAnnotations();
			sendResponse({ success: true });
			break;
```

- [ ] **Step 6: Run test to verify it passes**

Run: `bun test src/contentScript/highlighter.test.ts`
Expected: PASS

- [ ] **Step 7: Biome + typecheck**

Run: `bun run check:fix && bun run typecheck`
Expected: no errors on the changed files.

- [ ] **Step 8: Commit**

```bash
git add src/contentScript/highlighter.ts src/contentScript/index.ts src/types/recording.ts src/contentScript/highlighter.test.ts
git commit -m "feat(extension): add absolute-positioned numbered annotation overlay"
```

---

### Task 6: Extension full-page scroll-stitch + options threading

**Files:**
- Modify: `src/background/index.ts`
- Modify: `src/background/nativeHost.ts`
- Test: `src/background/fullPageScreenshot.test.ts`
- Create: `src/background/stitch.ts`

**Interfaces:**
- Consumes: `showAnnotations`/`removeAnnotations` via the content-script `ANNOTATE_ELEMENTS`/`CLEAR_ANNOTATIONS` messages (Task 5); the daemon `capture_screenshot` payload's `fullPage`/`annotate` fields (Task 2).
- Produces: pure `computeStitchPlan(contentWidth, contentHeight, viewportWidth, viewportHeight, dpr)` in `stitch.ts`; `captureScreenshotForUpload(tabId, opts)` where `opts = { fullPage?: boolean; annotate?: TargetSelector[] }`; the `ScreenshotCapturer` type gains the `opts` parameter; `NativeCaptureScreenshotMessage.payload` gains `fullPage?`/`annotate?`.

Extract the stitch math into `stitch.ts` so it is unit-testable without a browser; the `OffscreenCanvas` + `captureVisibleTab` loop stays in `index.ts` and is covered by the Task 7 manual smoke test.

- [ ] **Step 1: Write the failing test**

Create `src/background/fullPageScreenshot.test.ts`:

```typescript
import { describe, expect, it } from "bun:test";
import { computeStitchPlan } from "./stitch";

describe("computeStitchPlan", () => {
	it("sizes the canvas in device pixels and clamps the last row's scroll", () => {
		const plan = computeStitchPlan(1280, 2400, 1280, 720, 2);
		expect(plan.canvasWidth).toBe(2560); // 1280 * dpr
		expect(plan.canvasHeight).toBe(4800); // 2400 * dpr
		// 4 rows: ceil(2400/720) = 4; last scrollY clamped to 2400-720 = 1680.
		expect(plan.segments.map((s) => s.scrollY)).toEqual([0, 720, 1440, 1680]);
		expect(plan.segments.every((s) => s.scrollX === 0)).toBe(true);
	});

	it("handles a page that fits in one viewport (single segment)", () => {
		const plan = computeStitchPlan(800, 600, 800, 600, 1);
		expect(plan.segments).toEqual([{ scrollX: 0, scrollY: 0 }]);
		expect(plan.canvasWidth).toBe(800);
		expect(plan.canvasHeight).toBe(600);
	});

	it("tiles both axes for a page wider and taller than the viewport", () => {
		const plan = computeStitchPlan(2000, 1500, 1000, 1000, 1);
		// cols = ceil(2000/1000)=2, rows = ceil(1500/1000)=2 → 4 segments.
		expect(plan.segments.length).toBe(4);
		expect(plan.segments).toContainEqual({ scrollX: 1000, scrollY: 500 });
	});
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bun test src/background/fullPageScreenshot.test.ts`
Expected: FAIL — `./stitch` does not exist.

- [ ] **Step 3: Implement the stitch plan**

Create `src/background/stitch.ts`:

```typescript
/**
 * Pure geometry for full-page scroll-and-stitch capture. Kept browser-free so it
 * can be unit-tested; the actual OffscreenCanvas compositing lives in index.ts.
 */

export interface StitchSegment {
	scrollX: number; // CSS px to scroll to before capturing
	scrollY: number;
}

export interface StitchPlan {
	canvasWidth: number; // device px
	canvasHeight: number; // device px
	segments: StitchSegment[];
}

/**
 * Plan the scroll positions and canvas size for stitching a full page.
 * Segments tile the content in viewport-sized steps; the final row/column is
 * clamped so it never scrolls past the content edge (the overlap simply redraws
 * correct pixels). Canvas is sized in device pixels (content * dpr) because
 * captureVisibleTab returns images at DPR scale.
 */
export function computeStitchPlan(
	contentWidth: number,
	contentHeight: number,
	viewportWidth: number,
	viewportHeight: number,
	dpr: number,
): StitchPlan {
	const cols = Math.max(1, Math.ceil(contentWidth / viewportWidth));
	const rows = Math.max(1, Math.ceil(contentHeight / viewportHeight));
	const maxScrollX = Math.max(0, contentWidth - viewportWidth);
	const maxScrollY = Math.max(0, contentHeight - viewportHeight);

	const segments: StitchSegment[] = [];
	for (let r = 0; r < rows; r++) {
		for (let c = 0; c < cols; c++) {
			segments.push({
				scrollX: Math.min(c * viewportWidth, maxScrollX),
				scrollY: Math.min(r * viewportHeight, maxScrollY),
			});
		}
	}

	return {
		canvasWidth: Math.round(contentWidth * dpr),
		canvasHeight: Math.round(contentHeight * dpr),
		segments,
	};
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `bun test src/background/fullPageScreenshot.test.ts`
Expected: PASS

- [ ] **Step 5: Implement the full-page capture and thread options in `index.ts`**

In `src/background/index.ts`, add imports near the top (alongside the existing eventStore/nativeHost imports):

```typescript
import { computeStitchPlan } from "./stitch";
import type { TargetSelector } from "../types/recording";
```

Add the throttle constant and the capture helpers near `captureScreenshotForUpload`:

```typescript
// Chrome rate-limits captureVisibleTab (~2/sec). The scroll-stitch loop waits
// this long between captures to stay under quota and let layout settle after
// each scroll.
const CAPTURE_THROTTLE_MS = 600;

export interface ScreenshotUploadOptions {
	fullPage?: boolean;
	annotate?: TargetSelector[];
}

async function blobToBase64(blob: Blob): Promise<string> {
	const bytes = new Uint8Array(await blob.arrayBuffer());
	let binary = "";
	const chunk = 0x8000; // avoid String.fromCharCode arg overflow on large PNGs
	for (let i = 0; i < bytes.length; i += chunk) {
		binary += String.fromCharCode(...bytes.subarray(i, i + chunk));
	}
	return btoa(binary);
}

async function captureFullPageScreenshot(
	tabId: number,
): Promise<{ data?: string; error?: string }> {
	const tab = await chrome.tabs.get(tabId);
	if (!tab.windowId) return { error: "tab has no windowId" };
	if (!tab.active) {
		await chrome.tabs.update(tabId, { active: true });
	}

	const [{ result: metrics }] = await chrome.scripting.executeScript({
		target: { tabId },
		func: () => ({
			contentWidth: document.documentElement.scrollWidth,
			contentHeight: document.documentElement.scrollHeight,
			viewportWidth: window.innerWidth,
			viewportHeight: window.innerHeight,
			dpr: window.devicePixelRatio || 1,
			originX: window.scrollX,
			originY: window.scrollY,
		}),
	});
	if (!metrics) return { error: "could not read page metrics" };

	const plan = computeStitchPlan(
		metrics.contentWidth,
		metrics.contentHeight,
		metrics.viewportWidth,
		metrics.viewportHeight,
		metrics.dpr,
	);

	const canvas = new OffscreenCanvas(plan.canvasWidth, plan.canvasHeight);
	const ctx = canvas.getContext("2d");
	if (!ctx) return { error: "could not get OffscreenCanvas 2D context" };

	try {
		for (const seg of plan.segments) {
			await chrome.scripting.executeScript({
				target: { tabId },
				func: (x: number, y: number) => window.scrollTo(x, y),
				args: [seg.scrollX, seg.scrollY],
			});
			await new Promise((r) => setTimeout(r, CAPTURE_THROTTLE_MS));

			const dataUrl = await chrome.tabs.captureVisibleTab(tab.windowId, {
				format: "png",
			});
			if (!dataUrl) return { error: "captureVisibleTab returned empty" };

			const bmp = await createImageBitmap(await (await fetch(dataUrl)).blob());
			ctx.drawImage(
				bmp,
				Math.round(seg.scrollX * metrics.dpr),
				Math.round(seg.scrollY * metrics.dpr),
			);
			bmp.close();
		}
	} finally {
		// Always restore the user's original scroll position.
		await chrome.scripting.executeScript({
			target: { tabId },
			func: (x: number, y: number) => window.scrollTo(x, y),
			args: [metrics.originX, metrics.originY],
		});
	}

	const blob = await canvas.convertToBlob({ type: "image/png" });
	return { data: `data:image/png;base64,${await blobToBase64(blob)}` };
}
```

Replace `captureScreenshotForUpload` with the options-aware version (annotate injection wraps whichever capture mode runs; overlays are drawn before capture and cleared after):

```typescript
/**
 * Capture a screenshot for native-host upload, surfacing the real failure reason
 * instead of swallowing it. Supports --full-page (scroll-stitch) and --annotate
 * (numbered overlays drawn by the content script before capture, then cleared).
 */
async function captureScreenshotForUpload(
	tabId: number,
	opts: ScreenshotUploadOptions = {},
): Promise<{ data?: string; error?: string }> {
	const hasAnnotations = (opts.annotate?.length ?? 0) > 0;
	try {
		if (hasAnnotations) {
			await chrome.tabs.sendMessage(tabId, {
				type: "ANNOTATE_ELEMENTS",
				targets: opts.annotate,
			});
		}

		let result: { data?: string; error?: string };
		if (opts.fullPage) {
			result = await captureFullPageScreenshot(tabId);
		} else {
			const tab = await chrome.tabs.get(tabId);
			if (!tab.windowId) {
				result = { error: "tab has no windowId" };
			} else {
				if (!tab.active) {
					await chrome.tabs.update(tabId, { active: true });
				}
				const dataUrl = await chrome.tabs.captureVisibleTab(tab.windowId, {
					format: "png",
				});
				result = dataUrl
					? { data: dataUrl }
					: { error: "captureVisibleTab returned empty" };
			}
		}
		return result;
	} catch (err) {
		return { error: err instanceof Error ? err.message : String(err) };
	} finally {
		if (hasAnnotations) {
			try {
				await chrome.tabs.sendMessage(tabId, { type: "CLEAR_ANNOTATIONS" });
			} catch (err) {
				console.warn("[HTR NControl] Failed to clear annotations:", err);
			}
		}
	}
}
```

- [ ] **Step 6: Thread options through `nativeHost.ts`**

In `src/background/nativeHost.ts`, widen the capturer type and payload type. Change the `ScreenshotCapturer`/`ScreenshotResult` block (near line 36):

```typescript
type ScreenshotResult = { data?: string; error?: string };
type ScreenshotCapturer = (
	tabId: number,
	opts?: { fullPage?: boolean; annotate?: unknown[] },
) => Promise<ScreenshotResult>;
let captureScreenshot: ScreenshotCapturer | null = null;
```

Change `NativeCaptureScreenshotMessage` (near line 314) to carry the options:

```typescript
interface NativeCaptureScreenshotMessage {
	type: "capture_screenshot";
	tabId: number;
	commandId: string;
	payload: {
		uploadUrl: string;
		token?: string;
		fullPage?: boolean;
		annotate?: unknown[];
	};
}
```

In `handleCaptureScreenshot`, pass the options through (the relay has already JSON-parsed `annotate` into an array):

```typescript
		const res: ScreenshotResult = captureScreenshot
			? await captureScreenshot(tabId, {
					fullPage: payload.fullPage,
					annotate: payload.annotate,
				})
			: { error: "no screenshot capturer registered" };
```

(The `payload` destructure already exists at the top of `handleCaptureScreenshot`; extend it to `const { tabId, commandId, payload } = msg;` if it currently pulls out `uploadUrl`/`token` individually, and read `payload.fullPage`/`payload.annotate` from there.)

The registration `setScreenshotCapturer(captureScreenshotForUpload)` at the bottom of `index.ts` needs no change — the wider signature is compatible (the extra param is optional).

- [ ] **Step 7: Run tests + typecheck + biome**

Run: `bun test src/background/fullPageScreenshot.test.ts && bun run typecheck && bun run check:fix`
Expected: PASS, no type errors. (`opts.annotate` is typed `unknown[]` at the nativeHost boundary and `TargetSelector[]` inside `captureScreenshotForUpload`; the content script receives it as `targets` and matches via `findAllElements`.)

- [ ] **Step 8: Run the full extension test suite for regressions**

Run: `bun run test`
Expected: PASS

- [ ] **Step 9: Commit**

```bash
git add src/background/index.ts src/background/nativeHost.ts src/background/stitch.ts src/background/fullPageScreenshot.test.ts
git commit -m "feat(extension): full-page scroll-stitch + annotate capture for screenshots"
```

---

### Task 7: End-to-end verification

**Files:** none (verification only)

- [ ] **Step 1: CDP full-page**

With Chrome running with `--remote-debugging-port=9222` on a tall page, run `htrcli --cdp screenshot --full-page /tmp/cdp-full.png`. Open the PNG; confirm it contains content below the fold (the whole scrollable page), not just the viewport. Then run `htrcli --cdp screenshot --annotate "button" /tmp/x.png` and confirm it errors with "screenshot --annotate is not supported over --cdp yet".

- [ ] **Step 2: Extension viewport (regression)**

With `htrcli serve` running and the extension connected in Chrome, run `htrcli screenshot /tmp/vp.png` (no flags). Confirm it still saves a normal viewport screenshot — the default path must be unchanged.

- [ ] **Step 3: Extension full-page (scroll-stitch + DPR)**

On a tall page, run `htrcli screenshot --full-page /tmp/ext-full.png`. Confirm: (a) the image spans the full page, (b) on a HiDPI display the pixel dimensions are `contentSize * devicePixelRatio` (no cropping/misalignment between stitched bands), (c) the page's scroll position is restored afterward, (d) it completes without a 504 (validates `fullPageScreenshotTimeout` + the 90s client timeout), and (e) no "quota exceeded" error in the service-worker console (validates `CAPTURE_THROTTLE_MS`).

- [ ] **Step 4: Extension annotate, and compose with full-page**

Run `htrcli screenshot --annotate "button,input,role=link" /tmp/ann.png` and confirm numbered red boxes appear over the matched elements in the viewport shot. Then run `htrcli screenshot --annotate "button,input" --full-page /tmp/ann-full.png` and confirm the numbered overlays land on the correct elements **throughout** the full-page image (top and bottom) — this is the `position: absolute` correctness check; if overlays cluster only near the top or appear at wrong positions in lower bands, the overlay is still viewport-fixed.

- [ ] **Step 5: Firefox full-page parity**

Load the extension in Firefox (`bun run firefox:build`, then `about:debugging`), connect via `htrcli serve`, and repeat Step 3. Confirm the scroll-stitch path works identically (it uses no CDP — `chrome.tabs.captureVisibleTab` + `OffscreenCanvas` are available in Firefox). Annotate (Step 4) should also work, since it is pure content-script DOM overlay.

---

## Self-Review

**1. Spec coverage (§3 "Full-page + annotated screenshots"):**
- `htrcli screenshot --full-page [path]` — CDP path (Task 1 + Task 4), extension scroll-stitch (Task 6). ✔
- `htrcli screenshot --annotate "button,input,a" [path]` — flag + parse (Task 4), overlay generalization (Task 5), capture wiring (Task 6). ✔
- `htrcli screenshot --full-page --annotate "role=button" [path]` — composition verified in Task 7 Step 4; `position: absolute` constraint (Global Constraints + Task 5) makes it correct. ✔
- CDP `Page.captureScreenshot({captureBeyondViewport:true})` + `Page.getLayoutMetrics` — Task 1. ✔
- Extension/Firefox fallback: resize/scroll loop + multiple `captureVisibleTab` composited via `OffscreenCanvas` in the **background** script — Task 6. ✔ (metrics/scroll driven via `chrome.scripting.executeScript` since `captureVisibleTab` is background-only; annotate overlay drawn in the content script.)
- Generalize `captureScreenshotWithHighlight` / `highlighter.ts` to accept a CLI selector/box list reusing `TargetSelector` matching — Task 5 (`findAllElements` per selector). ✔
- `ScreenshotOptions` with `FullPage`/`Annotate` — Task 3 (trimmed to the two fields this feature ships; see scope note). ✔
- Reuse the existing screenshot POST-back path, no protocol change beyond the options — Tasks 2/3/6; the size-limit question is answered explicitly (bytes fine, timeout is the real constraint → separate 45s/90s budget). ✔

**Scope note / judgment calls (deliberately not built):**
- The spec's §3 struct sketch also lists `Format`/`Quality`; §3's CLI examples do not use them and the task scoped to `--full-page` + `--annotate` only, so those are **not** implemented (avoid unrequested surface). If wanted later, add `--format`/`--quality` flags threaded the same way.
- CDP `--annotate` is **not** built (would require a second overlay path via `Runtime.evaluate`); it returns `errUnsupportedCDP` instead — fail-loud, matching the existing CDP-unsupported pattern.

**2. Placeholder scan:** No "TBD"/"similar to Task N"/"add error handling" — every code step shows real code; error handling is concrete (`errUnsupportedCDP`, `finally`-restore scroll, logged `catch` on annotation clear, zero-content-size guard).

**3. Type consistency:** `ScreenshotOptions{FullPage, Annotate []TargetSelector}` (api) ↔ query params `fullPage`/`annotate` (Task 3/2) ↔ `host.ScreenshotOpts{FullPage bool, Annotate json.RawMessage}` (Task 2) ↔ native payload `{fullPage, annotate}` (Task 2/6) ↔ `ScreenshotUploadOptions{fullPage?, annotate?}` (Task 6) ↔ `AnnotateElementsMessage.targets` (Task 5). `computeStitchPlan` signature identical in `stitch.ts` and its test. `ScreenshotFullPage(s)` used in Task 1 and Task 4. `parseAnnotateSelectors` / `toAnnotationBox` names consistent between definition and call sites.

## Execution Handoff

**Plan complete and saved to `docs/superpowers/plans/2026-07-24-htrcli-fullpage-annotated-screenshots.md`. Two execution options:**

**1. Subagent-Driven (recommended)** — dispatch a fresh subagent per task, review between tasks, fast iteration.

**2. Inline Execution** — execute tasks in this session using executing-plans, batch execution with checkpoints.

**Which approach?**
