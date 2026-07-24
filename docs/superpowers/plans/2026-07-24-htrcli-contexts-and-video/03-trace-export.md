# Part 3 — Trace Export Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `htrcli trace export <path.zip>` — aggregate already-captured console + network events, a snapshot screenshot, and current page info into one zip, mirroring the `src/utils/exportZip.ts` bundle layout.

**Architecture:** Read-only aggregation over the existing daemon HTTP API. `collectTrace` pulls `GET /api/page`, `GET /api/events?kind=console`, `GET /api/events?kind=network`, and a `GET /api/screenshot` snapshot via the existing `api.Client`; `buildTraceZip` writes them into a Go `archive/zip` bundle (`trace.json` + `console.json` + `network.json` + `screenshots/snapshot.png` + `README.md`), matching the extension-side `exportToZip` structure (a combined JSON + README + screenshots folder). No new capture is introduced — this is purely an aggregator.

**Tech Stack:** Go (cobra, stdlib `archive/zip`, `encoding/json`, `encoding/base64`), the existing `internal/api` client. Go `testing` + `net/http/httptest`.

## Global Constraints

- Go module root: `htrcli/`. Run Go tests with `cd htrcli && go test ./...`.
- Go tests use `httptest.NewServer` + the `api.ApiResponse{OK,Data,Error}`
  envelope (matches `internal/api/client_test.go`).
- Trace export is an **extension-transport** feature — it reads the daemon's
  `/api/events` buffer. Under `--cdp` it returns `errUnsupportedCDP("trace export")`
  (mirrors `console read`).
- Every caught error logged with attempt + error, or an explicit
  `// intentionally not logged` comment.

## Cross-plan dependencies (stated honestly)

- **Network events** come from `GET /api/events?kind=network`, populated by the
  sibling plan `2026-07-24-htrcli-network-capture.md`. Until it lands, this read
  returns an empty set (or an error the daemon maps to empty). This plan treats a
  network read error as **non-fatal + logged** so a console-only trace still
  exports — the one place trace export deliberately degrades. Revisit when network
  capture ships to make it a hard read.
- **Per-step full-page screenshots** (§3, sibling plan
  `2026-07-24-htrcli-fullpage-annotated-screenshots.md`) do not exist yet. This
  plan includes a **single** snapshot via the current `GetScreenshot()` viewport
  capture. When §3 lands, extend `collectTrace` to embed per-action screenshots.
- **Timestamped action log** — no action buffer exists in the daemon today, so the
  bundle contains events + one screenshot + page info only. When an action stream
  exists, add an `actions.json` to `buildTraceZip`; the zip structure already
  leaves room for it.

---

### Task 1: Trace bundle builder (`buildTraceZip`) + README

**Files:**
- Create: `htrcli/internal/commands/trace.go`
- Test: `htrcli/internal/commands/trace_test.go`

**Interfaces:**
- Consumes: `api.PageInfo`, `api.EventEntry` (existing `internal/api/types.go`).
- Produces: `traceBundle{Page *api.PageInfo, Console []api.EventEntry, Network []api.EventEntry, ScreenshotPNG []byte, ExportedAt time.Time}`, `buildTraceZip(b traceBundle) ([]byte, error)`, `buildTraceReadme(b traceBundle) string`.

- [ ] **Step 1: Write the failing test**

Create `htrcli/internal/commands/trace_test.go`:

```go
package commands

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/u007/htrcli/internal/api"
)

func zipEntries(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	out := map[string][]byte{}
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open entry %s: %v", f.Name, err)
		}
		var buf bytes.Buffer
		if _, err := buf.ReadFrom(rc); err != nil {
			t.Fatalf("read entry %s: %v", f.Name, err)
		}
		rc.Close()
		out[f.Name] = buf.Bytes()
	}
	return out
}

func TestBuildTraceZip(t *testing.T) {
	bundle := traceBundle{
		Page:          &api.PageInfo{URL: "https://example.com", Title: "Example"},
		Console:       []api.EventEntry{{Seq: 1, Kind: "console", Timestamp: 100, Data: json.RawMessage(`{"level":"log","args":["hi"]}`)}},
		Network:       nil,
		ScreenshotPNG: []byte("\x89PNGfake"),
		ExportedAt:    time.Unix(1700000000, 0),
	}
	data, err := buildTraceZip(bundle)
	if err != nil {
		t.Fatalf("buildTraceZip: %v", err)
	}
	entries := zipEntries(t, data)

	for _, name := range []string{"trace.json", "console.json", "network.json", "README.md", "screenshots/snapshot.png"} {
		if _, ok := entries[name]; !ok {
			t.Errorf("expected zip entry %q, missing (have %v)", name, keys(entries))
		}
	}

	// trace.json embeds the page + console.
	var trace struct {
		Page    *api.PageInfo     `json:"page"`
		Console []api.EventEntry  `json:"console"`
		Network []api.EventEntry  `json:"network"`
	}
	if err := json.Unmarshal(entries["trace.json"], &trace); err != nil {
		t.Fatalf("parse trace.json: %v", err)
	}
	if trace.Page == nil || trace.Page.URL != "https://example.com" {
		t.Errorf("trace.json page mismatch: %+v", trace.Page)
	}
	if len(trace.Console) != 1 || trace.Console[0].Seq != 1 {
		t.Errorf("trace.json console mismatch: %+v", trace.Console)
	}
	if string(entries["screenshots/snapshot.png"]) != "\x89PNGfake" {
		t.Errorf("screenshot bytes mismatch")
	}
}

func TestBuildTraceZipOmitsScreenshotWhenAbsent(t *testing.T) {
	data, err := buildTraceZip(traceBundle{ExportedAt: time.Now()})
	if err != nil {
		t.Fatalf("buildTraceZip: %v", err)
	}
	entries := zipEntries(t, data)
	if _, ok := entries["screenshots/snapshot.png"]; ok {
		t.Error("expected no screenshot entry when ScreenshotPNG is empty")
	}
	if _, ok := entries["trace.json"]; !ok {
		t.Error("expected trace.json even with no screenshot")
	}
}

func keys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd htrcli && go test ./internal/commands/ -run 'TestBuildTraceZip' -v`
Expected: FAIL — `traceBundle`, `buildTraceZip` undefined.

- [ ] **Step 3: Write the builder**

Create `htrcli/internal/commands/trace.go`:

```go
package commands

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/u007/htrcli/internal/api"
)

// traceBundle is the aggregated data written into a trace zip. Network is empty
// until the network-capture sibling plan lands; ScreenshotPNG is empty when the
// snapshot could not be captured.
type traceBundle struct {
	Page          *api.PageInfo
	Console       []api.EventEntry
	Network       []api.EventEntry
	ScreenshotPNG []byte
	ExportedAt    time.Time
}

// buildTraceZip renders the bundle into a zip mirroring the extension-side
// exportToZip layout: a combined trace.json, raw console/network arrays, an
// optional screenshot, and a README.
func buildTraceZip(b traceBundle) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	writeFile := func(name string, data []byte) error {
		w, err := zw.Create(name)
		if err != nil {
			return fmt.Errorf("creating zip entry %s: %w", name, err)
		}
		if _, err := w.Write(data); err != nil {
			return fmt.Errorf("writing zip entry %s: %w", name, err)
		}
		return nil
	}

	trace := map[string]any{
		"exportedAt": b.ExportedAt.UTC().Format(time.RFC3339),
		"page":       b.Page,
		"console":    b.Console,
		"network":    b.Network,
	}
	traceJSON, err := json.MarshalIndent(trace, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling trace.json: %w", err)
	}
	if err := writeFile("trace.json", traceJSON); err != nil {
		return nil, err
	}

	consoleJSON, err := json.MarshalIndent(b.Console, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling console.json: %w", err)
	}
	if err := writeFile("console.json", consoleJSON); err != nil {
		return nil, err
	}

	networkJSON, err := json.MarshalIndent(b.Network, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling network.json: %w", err)
	}
	if err := writeFile("network.json", networkJSON); err != nil {
		return nil, err
	}

	if len(b.ScreenshotPNG) > 0 {
		if err := writeFile("screenshots/snapshot.png", b.ScreenshotPNG); err != nil {
			return nil, err
		}
	}

	if err := writeFile("README.md", []byte(buildTraceReadme(b))); err != nil {
		return nil, err
	}

	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("finalizing zip: %w", err)
	}
	return buf.Bytes(), nil
}

// buildTraceReadme renders a short human summary of the bundle.
func buildTraceReadme(b traceBundle) string {
	var sb strings.Builder
	sb.WriteString("# htrcli trace export\n\n")
	fmt.Fprintf(&sb, "Exported: %s\n\n", b.ExportedAt.UTC().Format(time.RFC3339))
	if b.Page != nil {
		fmt.Fprintf(&sb, "- URL: %s\n- Title: %s\n", b.Page.URL, b.Page.Title)
	}
	fmt.Fprintf(&sb, "- Console entries: %d\n", len(b.Console))
	fmt.Fprintf(&sb, "- Network entries: %d\n", len(b.Network))
	if len(b.ScreenshotPNG) > 0 {
		sb.WriteString("- Screenshot: screenshots/snapshot.png\n")
	}
	sb.WriteString("\nContents:\n")
	sb.WriteString("- trace.json — combined page info + console + network\n")
	sb.WriteString("- console.json / network.json — raw event arrays\n")
	return sb.String()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd htrcli && go test ./internal/commands/ -run 'TestBuildTraceZip' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add htrcli/internal/commands/trace.go htrcli/internal/commands/trace_test.go
git commit -m "feat(htrcli): trace zip bundle builder"
```

---

### Task 2: `collectTrace` — aggregate from the daemon API

**Files:**
- Modify: `htrcli/internal/commands/trace.go`
- Modify: `htrcli/internal/commands/trace_test.go`

**Interfaces:**
- Consumes: `api.Client.GetPageInfo`, `GetEvents`, `GetScreenshot` (existing); `traceBundle` (Task 1).
- Produces: `collectTrace(c *api.Client, tabID *int) (traceBundle, error)`.

- [ ] **Step 1: Write the failing test**

Append to `htrcli/internal/commands/trace_test.go`. The httptest server answers the
four reads `collectTrace` makes, each in the `ApiResponse` envelope:

```go
func TestCollectTrace(t *testing.T) {
	pngB64 := base64.StdEncoding.EncodeToString([]byte("\x89PNGfake"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/page":
			json.NewEncoder(w).Encode(api.ApiResponse{OK: true, Data: api.PageInfo{URL: "https://ex.com", Title: "Ex"}})
		case r.URL.Path == "/api/events" && r.URL.Query().Get("kind") == "console":
			json.NewEncoder(w).Encode(api.ApiResponse{OK: true, Data: api.EventsResponse{
				Entries: []api.EventEntry{{Seq: 1, Kind: "console", Timestamp: 1, Data: json.RawMessage(`{"level":"log","args":["hi"]}`)}},
			}})
		case r.URL.Path == "/api/events" && r.URL.Query().Get("kind") == "network":
			json.NewEncoder(w).Encode(api.ApiResponse{OK: true, Data: api.EventsResponse{Entries: nil}})
		case r.URL.Path == "/api/screenshot":
			json.NewEncoder(w).Encode(api.ApiResponse{OK: true, Data: pngB64})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	c := api.NewClient(srv.URL, "")
	bundle, err := collectTrace(c, nil)
	if err != nil {
		t.Fatalf("collectTrace: %v", err)
	}
	if bundle.Page == nil || bundle.Page.URL != "https://ex.com" {
		t.Errorf("page mismatch: %+v", bundle.Page)
	}
	if len(bundle.Console) != 1 {
		t.Errorf("expected 1 console entry, got %d", len(bundle.Console))
	}
	if string(bundle.ScreenshotPNG) != "\x89PNGfake" {
		t.Errorf("screenshot decode mismatch: %q", bundle.ScreenshotPNG)
	}
}

func TestCollectTraceNetworkErrorNonFatal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/page":
			json.NewEncoder(w).Encode(api.ApiResponse{OK: true, Data: api.PageInfo{URL: "https://ex.com"}})
		case r.URL.Path == "/api/events" && r.URL.Query().Get("kind") == "console":
			json.NewEncoder(w).Encode(api.ApiResponse{OK: true, Data: api.EventsResponse{Entries: nil}})
		case r.URL.Path == "/api/events" && r.URL.Query().Get("kind") == "network":
			json.NewEncoder(w).Encode(api.ApiResponse{OK: false, Error: "network capture not enabled"})
		case r.URL.Path == "/api/screenshot":
			json.NewEncoder(w).Encode(api.ApiResponse{OK: false, Error: "no tab"})
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	c := api.NewClient(srv.URL, "")
	bundle, err := collectTrace(c, nil)
	if err != nil {
		t.Fatalf("collectTrace should not fail on network/screenshot errors: %v", err)
	}
	if len(bundle.Network) != 0 || len(bundle.ScreenshotPNG) != 0 {
		t.Errorf("expected empty network + screenshot, got %+v", bundle)
	}
}
```

Add the extra imports to the `trace_test.go` import block (alongside the Task 1
imports `archive/zip`, `bytes`, `encoding/json`, `testing`, `time`, and the api
package):

```go
	"encoding/base64"
	"net/http"
	"net/http/httptest"
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd htrcli && go test ./internal/commands/ -run 'TestCollectTrace' -v`
Expected: FAIL — `collectTrace` undefined.

- [ ] **Step 3: Write collectTrace**

Append to `htrcli/internal/commands/trace.go`. Add `"encoding/base64"`, `"os"`, and
`"github.com/u007/htrcli/internal/api"` is already imported; add `"encoding/base64"`
and `"os"` to the import block:

```go
// collectTrace aggregates the current page, buffered console + network events,
// and a snapshot screenshot into a traceBundle. Page info and console are
// required; network and the screenshot are best-effort (network capture is a
// sibling feature; a snapshot needs a connected tab) — their absence is logged,
// not fatal, so a partial-but-useful trace still exports.
func collectTrace(c *api.Client, tabID *int) (traceBundle, error) {
	b := traceBundle{ExportedAt: time.Now()}

	page, err := c.GetPageInfo(tabID)
	if err != nil {
		return b, fmt.Errorf("reading page info: %w", err)
	}
	b.Page = page

	consoleResp, err := c.GetEvents(tabID, "console", 0)
	if err != nil {
		return b, fmt.Errorf("reading console events: %w", err)
	}
	b.Console = consoleResp.Entries

	networkResp, err := c.GetEvents(tabID, "network", 0)
	if err != nil {
		// Non-fatal: network capture is a sibling plan; export console-only.
		fmt.Fprintf(os.Stderr, "[htrcli] network events unavailable (network capture not yet enabled): %v\n", err)
	} else {
		b.Network = networkResp.Entries
	}

	shot, err := c.GetScreenshot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[htrcli] screenshot unavailable for trace: %v\n", err)
	} else {
		png, derr := base64.StdEncoding.DecodeString(shot)
		if derr != nil {
			fmt.Fprintf(os.Stderr, "[htrcli] decoding trace screenshot: %v\n", derr)
		} else {
			b.ScreenshotPNG = png
		}
	}

	return b, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd htrcli && go test ./internal/commands/ -run 'TestCollectTrace|TestBuildTraceZip' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add htrcli/internal/commands/trace.go htrcli/internal/commands/trace_test.go
git commit -m "feat(htrcli): collectTrace aggregates daemon API into a bundle"
```

---

### Task 3: `htrcli trace export <path.zip>` command

**Files:**
- Modify: `htrcli/internal/commands/trace.go`
- Modify: `htrcli/internal/commands/trace_test.go`

**Interfaces:**
- Consumes: `collectTrace`, `buildTraceZip` (Tasks 1–2); `UseCDP`, `errUnsupportedCDP`, `GetTabID`, `GetClient`, `output.PrintJSON`, `output.JSONOutput` (existing).
- Produces: the `trace` and `trace export` cobra commands.

- [ ] **Step 1: Write the failing test**

Append to `htrcli/internal/commands/trace_test.go`:

```go
func TestTraceExportRejectsCDP(t *testing.T) {
	t.Cleanup(func() { transportFlag = ""; cdpFlag = false })
	transportFlag = "cdp"
	err := traceExportCmd.RunE(traceExportCmd, []string{"out.zip"})
	if err == nil {
		t.Fatal("expected trace export to reject the CDP transport")
	}
}

func TestTraceExportRequiresOneArg(t *testing.T) {
	if err := traceExportCmd.Args(traceExportCmd, nil); err == nil {
		t.Fatal("expected trace export to require an output path")
	}
	if err := traceExportCmd.Args(traceExportCmd, []string{"a", "b"}); err == nil {
		t.Fatal("expected trace export to reject two args")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd htrcli && go test ./internal/commands/ -run 'TestTraceExport' -v`
Expected: FAIL — `traceExportCmd` undefined.

- [ ] **Step 3: Write the command**

Append to `htrcli/internal/commands/trace.go`. Add `"github.com/spf13/cobra"` and
`"github.com/u007/htrcli/internal/output"` to the import block:

```go
var traceCmd = &cobra.Command{
	Use:   "trace",
	Short: "Export a debug trace bundle",
}

var traceExportCmd = &cobra.Command{
	Use:   "export <path.zip>",
	Short: "Export console + network + screenshot + page info as a zip",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if UseCDP() {
			return errUnsupportedCDP("trace export")
		}
		tabID, err := GetTabID()
		if err != nil {
			return err
		}
		bundle, err := collectTrace(GetClient(), tabID)
		if err != nil {
			return err
		}
		data, err := buildTraceZip(bundle)
		if err != nil {
			return err
		}
		out := args[0]
		if err := os.WriteFile(out, data, 0644); err != nil {
			return fmt.Errorf("writing %s: %w", out, err)
		}
		if output.JSONOutput {
			output.PrintJSON(map[string]any{
				"trace":   out,
				"console": len(bundle.Console),
				"network": len(bundle.Network),
			})
			return nil
		}
		fmt.Printf("Trace exported to %s (%d console, %d network entries)\n", out, len(bundle.Console), len(bundle.Network))
		return nil
	},
}

func init() {
	traceCmd.AddCommand(traceExportCmd)
	rootCmd.AddCommand(traceCmd)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd htrcli && go test ./internal/commands/ -run 'TestTraceExport|TestCollectTrace|TestBuildTraceZip' -v`
Expected: PASS.

- [ ] **Step 5: Full suite + build**

Run: `cd htrcli && go test ./... && go build ./...`
Expected: PASS / clean build.

- [ ] **Step 6: Manual end-to-end smoke test**

Run (with `htrcli serve` running + the extension connected to a page that has
logged a `console.log`):
```bash
make htrcli-build
./htrcli/bin/htrcli trace export /tmp/trace.zip
unzip -l /tmp/trace.zip
```
Expected: the zip contains `trace.json`, `console.json`, `network.json`,
`README.md`, and (if a tab is connected) `screenshots/snapshot.png`. `trace.json`
lists the captured console entry; `network.json` is `[]` until the network-capture
plan lands (a `[htrcli] network events unavailable` line is printed to stderr,
which is expected).

- [ ] **Step 7: Commit**

```bash
git add htrcli/internal/commands/trace.go htrcli/internal/commands/trace_test.go
git commit -m "feat(htrcli): trace export command"
```

---

## Part 3 Self-Review

- **Spec coverage (§7b trace):** `htrcli trace export <path.zip>` → Task 3;
  aggregation of network (§1) + console (§2) + screenshots (§3) + page info into
  one file mirroring `exportZip.ts` → Tasks 1–2 (`trace.json` + per-kind JSON +
  `screenshots/` + `README.md`, matching `exportToZip`'s combined-JSON + README +
  screenshots-folder shape).
- **Placeholder scan:** every step ships complete code; no TBD/TODO. Test helpers
  (`zipEntries`, `keys`) are fully defined.
- **Type consistency:** `traceBundle` fields
  (`Page/Console/Network/ScreenshotPNG/ExportedAt`) are identical across Tasks
  1–3; `collectTrace(c, tabID)` and `buildTraceZip(b)` signatures match every call
  site; reads use the confirmed client methods `GetPageInfo(tabID *int)`,
  `GetEvents(tabID *int, kind string, since int)`, `GetScreenshot() (string, error)`.
- **Honest dependencies/deferrals (restated):** network events depend on the
  network-capture sibling plan (non-fatal + logged until it lands); per-step
  full-page screenshots depend on the screenshots sibling plan (single viewport
  snapshot for now); no action log exists yet (events + snapshot + page info
  only). All three are surfaced in the header and in code comments, and the zip
  layout reserves room (`actions.json`) for the action log when it exists.
- **Open judgment call:** network read errors are non-fatal here **by design**
  because the producing feature isn't built yet. This is the single deliberate
  deviation from the repo's "fail loud / no fallback" default, justified by the
  documented cross-plan dependency and logged explicitly — flagged for revisit
  (make it a hard read) once `2026-07-24-htrcli-network-capture.md` ships.
