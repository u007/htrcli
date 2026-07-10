# Part 3: Browser Lifecycle

Spec: `docs/superpowers/specs/2026-07-10-htcli-cdp-transport-design.md`. Depends on Parts 1–2 (`GetCDPPort()`, `GetChromePath()` from `commands`; `ListTargets`, `BrowserWSURL`, `Dial`, `Session` from `internal/cdp`).

Exact Chrome launch flags (Global Constraints): `--remote-debugging-port=<port> --user-data-dir=$HOME/.htcli/chrome-profile --no-first-run --disable-backgrounding-occluded-windows --disable-renderer-backgrounding` (+ `--headless` when requested).

---

### Task 5: `htcli browser start|stop|status`

**Files:**
- Create: `htcli/internal/cdp/launch.go` (spawn + state file; the single sanctioned Chrome-spawn site)
- Create: `htcli/internal/commands/browser.go` (cobra wiring)
- Test: `htcli/internal/cdp/launch_test.go`

**Interfaces:**
- Produces (package `cdp`):
  - `type BrowserState struct { PID int ` + "`json:\"pid\"`" + `; Port int ` + "`json:\"port\"`" + `; StartedAt time.Time ` + "`json:\"started_at\"`" + `; Headless bool ` + "`json:\"headless\"`" + ` }`
  - `StateFilePath() (string, error)` — `~/.htcli/browser.json`
  - `ReadState() (*BrowserState, error)` — nil, nil when the file doesn't exist
  - `LaunchArgs(port int, profileDir string, headless bool) []string` — pure, testable
  - `FindChrome(configured string) (string, error)` — configured path or macOS default `/Applications/Google Chrome.app/Contents/MacOS/Google Chrome`; error naming `config set-chrome-path` if neither exists
  - `PortAlive(port int) bool` — `/json/version` answers within 2s
  - `StartBrowser(chromePath string, port int, headless bool) (*BrowserState, error)` — skips launch if `PortAlive`; polls up to 15s for the port; writes state file
  - `StopBrowser() error` — reads state, verifies `/proc`-equivalent cmdline via `ps -p <pid> -o command=` contains `.htcli/chrome-profile` (PID-reuse guard), SIGTERMs, removes state file
- Consumed by: Task 7 (hide/show), Part 5 smoke test.

- [ ] **Step 1: Write the failing test (pure parts only — no Chrome in unit tests)**

Create `htcli/internal/cdp/launch_test.go`:

```go
package cdp

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestLaunchArgs(t *testing.T) {
	args := LaunchArgs(9222, "/home/u/.htcli/chrome-profile", false)
	for _, want := range []string{
		"--remote-debugging-port=9222",
		"--user-data-dir=/home/u/.htcli/chrome-profile",
		"--no-first-run",
		"--disable-backgrounding-occluded-windows",
		"--disable-renderer-backgrounding",
	} {
		if !slices.Contains(args, want) {
			t.Errorf("missing %s in %v", want, args)
		}
	}
	if slices.Contains(args, "--headless") {
		t.Error("headless flag present without headless=true")
	}
	for _, a := range args {
		if strings.Contains(a, "--remote-debugging-address") {
			t.Fatal("must never pass --remote-debugging-address")
		}
	}
}

func TestLaunchArgsHeadless(t *testing.T) {
	args := LaunchArgs(9333, "/p", true)
	if !slices.Contains(args, "--headless") {
		t.Error("want plain --headless")
	}
	if slices.Contains(args, "--headless=new") {
		t.Error("--headless=new is a deprecated alias; use --headless")
	}
}

func TestFindChromeConfigured(t *testing.T) {
	f := filepath.Join(t.TempDir(), "chrome")
	if err := os.WriteFile(f, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	got, err := FindChrome(f)
	if err != nil || got != f {
		t.Fatalf("want %s, got %s (%v)", f, got, err)
	}
}

func TestFindChromeMissing(t *testing.T) {
	_, err := FindChrome(filepath.Join(t.TempDir(), "nope"))
	if err == nil || !strings.Contains(err.Error(), "set-chrome-path") {
		t.Fatalf("error must mention config set-chrome-path, got %v", err)
	}
}

func TestReadStateMissingFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	st, err := ReadState()
	if err != nil || st != nil {
		t.Fatalf("want nil,nil for missing file, got %v,%v", st, err)
	}
}

func TestStateRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	want := &BrowserState{PID: 42, Port: 9222, Headless: true}
	if err := writeState(want); err != nil {
		t.Fatal(err)
	}
	got, err := ReadState()
	if err != nil || got == nil || got.PID != 42 || !got.Headless {
		t.Fatalf("round trip failed: %v %v", got, err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd htcli && go test ./internal/cdp/ -run 'TestLaunch|TestFindChrome|TestReadState|TestStateRoundTrip' -v`
Expected: compile error.

- [ ] **Step 3: Implement launch.go**

```go
package cdp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// BrowserState is persisted at ~/.htcli/browser.json. Advisory only: the
// debugging port answering is the source of truth for "running".
type BrowserState struct {
	PID       int       `json:"pid"`
	Port      int       `json:"port"`
	StartedAt time.Time `json:"started_at"`
	Headless  bool      `json:"headless"`
}

const macChromePath = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"

// StateFilePath returns ~/.htcli/browser.json.
func StateFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	return filepath.Join(home, ".htcli", "browser.json"), nil
}

// ProfileDir returns ~/.htcli/chrome-profile.
func ProfileDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	return filepath.Join(home, ".htcli", "chrome-profile"), nil
}

// ReadState returns nil, nil when no state file exists.
func ReadState() (*BrowserState, error) {
	path, err := StateFilePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil // intentionally not logged: absent state file means "not started", an expected case
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var st BrowserState
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &st, nil
}

func writeState(st *BrowserState) error {
	path, err := StateFilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling browser state: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// LaunchArgs builds the Chrome argument list. Security: never add
// --remote-debugging-address — the port must stay bound to localhost.
func LaunchArgs(port int, profileDir string, headless bool) []string {
	args := []string{
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--user-data-dir=" + profileDir,
		"--no-first-run",
		"--disable-backgrounding-occluded-windows",
		"--disable-renderer-backgrounding",
	}
	if headless {
		args = append(args, "--headless")
	}
	return args
}

// FindChrome returns the configured binary or the standard macOS path.
func FindChrome(configured string) (string, error) {
	candidates := []string{configured, macChromePath}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf(
		"Chrome binary not found (tried %q, %q) — set it with: htcli config set-chrome-path <path>",
		configured, macChromePath)
}

// PortAlive reports whether /json/version answers on the port.
func PortAlive(port int) bool {
	_, err := BrowserWSURL(port)
	return err == nil
}

// StartBrowser launches Chrome detached, waits for the port, persists state.
// If the port already answers, it records/refreshes state and returns without
// launching (also covers Chrome's singleton-lock handoff to an existing
// profile owner).
func StartBrowser(chromePath string, port int, headless bool) (*BrowserState, error) {
	if PortAlive(port) {
		st, err := ReadState()
		if err != nil || st == nil {
			st = &BrowserState{Port: port, StartedAt: time.Now()}
		}
		return st, nil
	}
	profile, err := ProfileDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(profile, 0700); err != nil {
		return nil, fmt.Errorf("creating profile dir: %w", err)
	}

	cmd := exec.Command(chromePath, LaunchArgs(port, profile, headless)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // detach: survives htcli exiting
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("launching Chrome %s: %w", chromePath, err)
	}
	// Reap when Chrome eventually exits so a stopped browser never zombies
	// against a still-running htcli daemon process.
	go func() {
		if err := cmd.Wait(); err != nil {
			fmt.Fprintf(os.Stderr, "[htcli] Chrome exited: %v\n", err)
		}
	}()

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if PortAlive(port) {
			st := &BrowserState{PID: cmd.Process.Pid, Port: port, StartedAt: time.Now(), Headless: headless}
			if err := writeState(st); err != nil {
				return nil, err
			}
			return st, nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return nil, fmt.Errorf("Chrome (pid %d) did not answer on port %d within 15s", cmd.Process.Pid, port)
}

// StopBrowser terminates the recorded PID after verifying its command line
// references the htcli profile (PID-reuse guard), then removes the state file.
func StopBrowser() error {
	st, err := ReadState()
	if err != nil {
		return err
	}
	if st == nil {
		return errors.New("no browser state file — nothing to stop")
	}
	out, err := exec.Command("ps", "-p", strconv.Itoa(st.PID), "-o", "command=").Output()
	if err == nil && strings.Contains(string(out), ".htcli/chrome-profile") {
		if err := syscall.Kill(st.PID, syscall.SIGTERM); err != nil {
			return fmt.Errorf("killing pid %d: %w", st.PID, err)
		}
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "[htcli] pid %d not found (%v) — cleaning up state file\n", st.PID, err)
	} else {
		fmt.Fprintf(os.Stderr, "[htcli] pid %d is not the htcli Chrome (%s) — refusing to kill, cleaning up state file\n", st.PID, strings.TrimSpace(string(out)))
	}
	path, err := StateFilePath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing %s: %w", path, err)
	}
	return nil
}
```

- [ ] **Step 4: Run unit tests**

Run: `cd htcli && go test ./internal/cdp/ -v`
Expected: all PASS.

- [ ] **Step 5: Implement commands/browser.go**

```go
package commands

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/u007/htcli/internal/cdp"
	"github.com/u007/htcli/internal/output"
)

var browserHeadless bool

var browserCmd = &cobra.Command{
	Use:   "browser",
	Short: "Manage the CDP-controlled Chrome instance",
}

var browserStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Launch Chrome with remote debugging (dedicated profile)",
	RunE: func(cmd *cobra.Command, args []string) error {
		chrome, err := cdp.FindChrome(GetChromePath())
		if err != nil {
			return err
		}
		st, err := cdp.StartBrowser(chrome, GetCDPPort(), browserHeadless)
		if err != nil {
			return err
		}
		if output.JSONOutput {
			output.PrintJSON(st)
			return nil
		}
		mode := "visible"
		if st.Headless {
			mode = "headless"
		}
		fmt.Printf("Browser running: pid %d, port %d (%s)\n", st.PID, st.Port, mode)
		return nil
	},
}

var browserStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the CDP-controlled Chrome",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := cdp.StopBrowser(); err != nil {
			return err
		}
		fmt.Println("Browser stopped")
		return nil
	},
}

var browserStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show CDP browser status (probes the port, not the PID file)",
	RunE: func(cmd *cobra.Command, args []string) error {
		port := GetCDPPort()
		alive := cdp.PortAlive(port)
		st, err := cdp.ReadState()
		if err != nil {
			return err
		}
		if output.JSONOutput {
			output.PrintJSON(map[string]any{"running": alive, "port": port, "state": st})
			return nil
		}
		if !alive {
			fmt.Printf("Browser: not running (port %d)\n", port)
			return nil
		}
		fmt.Printf("Browser: running on port %d\n", port)
		if st != nil {
			mode := "visible"
			if st.Headless {
				mode = "headless"
			}
			fmt.Printf("PID: %d (%s), started %s\n", st.PID, mode, st.StartedAt.Format("15:04:05"))
		}
		return nil
	},
}

func init() {
	browserStartCmd.Flags().BoolVar(&browserHeadless, "headless", false, "run without a window (sign in visible first — see GUIDE)")
	browserCmd.AddCommand(browserStartCmd)
	browserCmd.AddCommand(browserStopCmd)
	browserCmd.AddCommand(browserStatusCmd)
	rootCmd.AddCommand(browserCmd)
}
```

- [ ] **Step 6: Manual verification (requires Chrome on this Mac)**

```bash
cd htcli && go build -o bin/htcli ./cmd/htcli
./bin/htcli browser start        # window appears; prints pid/port
./bin/htcli browser status       # running
./bin/htcli browser start        # idempotent: prints status, no second window
curl -s http://127.0.0.1:9222/json/version | head -c 200
./bin/htcli browser stop         # window closes
./bin/htcli browser status       # not running
```

- [ ] **Step 7: Commit**

```bash
git add htcli/internal/cdp/launch.go htcli/internal/cdp/launch_test.go htcli/internal/commands/browser.go
git commit -m "feat(htcli): browser start/stop/status — managed CDP Chrome lifecycle

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 6: Spike — minimized-window viability (MANDATORY before Task 7)

Answers three questions from the spec, recording results that decide Task 7's documentation:
(a) does `Target.activateTarget` raise/focus the window at OS level on macOS?
(b) is `Input.dispatchMouseEvent` delivered to the active tab of a *minimized* window (with the anti-throttling launch flags)?
(c) does `Page.captureScreenshot` return fresh frames while minimized?

**Files:**
- Create: `htcli/internal/cdp/spike/main.go` (throwaway program, committed for reproducibility)
- Create: `docs/superpowers/spikes/2026-07-10-cdp-minimized-window.md` (results)

- [ ] **Step 1: Write the spike program**

Create `htcli/internal/cdp/spike/main.go`:

```go
// Spike: verify CDP input + screenshot behavior on a minimized Chrome window.
// Run: htcli browser start   (visible), then: go run ./internal/cdp/spike
// Watch the window and note: does it un-minimize / take focus at each step?
package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"time"

	"github.com/u007/htcli/internal/cdp"
)

func must(err error, step string) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL %s: %v\n", step, err)
		os.Exit(1)
	}
}

func main() {
	const port = 9222
	targets, err := cdp.ListTargets(port)
	must(err, "list targets")
	if len(targets) == 0 {
		fmt.Fprintln(os.Stderr, "no page targets — run: htcli browser start")
		os.Exit(1)
	}
	t := targets[0]

	page, err := cdp.Dial(t.WebSocketDebuggerURL)
	must(err, "dial page")
	defer page.Close()

	// Install a click counter on about:blank.
	must(page.Call("Runtime.evaluate", map[string]any{
		"expression": `window.__clicks=0; document.body.style.cssText='width:100vw;height:100vh';
			document.body.addEventListener('click', e => { window.__clicks++; window.__trusted = e.isTrusted; });
			'ready'`,
		"returnByValue": true,
	}, nil), "install counter")

	// Minimize via browser-level session.
	bws, err := cdp.BrowserWSURL(port)
	must(err, "browser ws url")
	browser, err := cdp.Dial(bws)
	must(err, "dial browser")
	defer browser.Close()

	var win struct {
		WindowID int `json:"windowId"`
	}
	must(browser.Call("Browser.getWindowForTarget", map[string]any{"targetId": t.ID}, &win), "getWindowForTarget")
	must(browser.Call("Browser.setWindowBounds", map[string]any{
		"windowId": win.WindowID, "bounds": map[string]any{"windowState": "minimized"},
	}, nil), "minimize")
	fmt.Println("STEP 1: window minimized — confirm visually")
	time.Sleep(2 * time.Second)

	// (a) activateTarget while minimized — does the window come back / steal focus?
	must(page.Call("Target.activateTarget", map[string]any{"targetId": t.ID}, nil), "activateTarget")
	fmt.Println("STEP 2 (a): activateTarget sent — did the window restore or take focus? RECORD THIS")
	time.Sleep(3 * time.Second)

	// Re-minimize in case it restored, then (b) dispatch a click while minimized.
	must(browser.Call("Browser.setWindowBounds", map[string]any{
		"windowId": win.WindowID, "bounds": map[string]any{"windowState": "minimized"},
	}, nil), "re-minimize")
	time.Sleep(time.Second)
	for _, evtType := range []string{"mousePressed", "mouseReleased"} {
		must(page.Call("Input.dispatchMouseEvent", map[string]any{
			"type": evtType, "x": 100, "y": 100, "button": "left", "clickCount": 1, "buttons": 1,
		}, nil), "dispatch "+evtType)
	}
	var clicks struct {
		Result struct {
			Value int `json:"value"`
		} `json:"result"`
	}
	must(page.Call("Runtime.evaluate", map[string]any{
		"expression": "window.__clicks", "returnByValue": true,
	}, &clicks), "read counter")
	fmt.Printf("STEP 3 (b): clicks registered while minimized = %d (want 1)\n", clicks.Result.Value)

	// (c) screenshot while minimized.
	var shot struct {
		Data string `json:"data"`
	}
	err = page.Call("Page.captureScreenshot", map[string]any{"format": "png"}, &shot)
	if err != nil {
		fmt.Printf("STEP 4 (c): screenshot FAILED while minimized: %v\n", err)
	} else {
		raw, _ := base64.StdEncoding.DecodeString(shot.Data)
		fmt.Printf("STEP 4 (c): screenshot returned %d bytes while minimized\n", len(raw))
	}

	// Restore.
	must(browser.Call("Browser.setWindowBounds", map[string]any{
		"windowId": win.WindowID, "bounds": map[string]any{"windowState": "normal"},
	}, nil), "restore")
	fmt.Println("done — restore confirmed; record all observations in docs/superpowers/spikes/2026-07-10-cdp-minimized-window.md")
}
```

- [ ] **Step 2: Run the spike**

```bash
cd htcli && go build ./... \
  && ./bin/htcli browser start \
  && go run ./internal/cdp/spike
```
Watch the Chrome window during steps 1–2 and record: restored? focused? clicks counted? screenshot bytes?

- [ ] **Step 3: Record results**

Write `docs/superpowers/spikes/2026-07-10-cdp-minimized-window.md` with the observed answers to (a), (b), (c) and a one-line conclusion choosing ONE of:
- **Outcome A:** minimized input works → Task 7 ships `hide`/`show` as fully supported.
- **Outcome B:** activateTarget steals focus and/or input is dropped while minimized → Task 7 ships `hide`/`show`, but GUIDE documents: input verbs (`click`, `press`) require headless for background use; `hide` remains useful for `eval`/`fill`/screenshot-only flows (or cosmetic).

- [ ] **Step 4: Commit**

```bash
git add htcli/internal/cdp/spike/ docs/superpowers/spikes/2026-07-10-cdp-minimized-window.md
git commit -m "spike: CDP minimized-window input/screenshot/activateTarget results

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 7: `htcli browser hide|show`

**Files:**
- Modify: `htcli/internal/commands/browser.go`
- Create: `htcli/internal/cdp/window.go`
- Test: `htcli/internal/cdp/window_test.go`

**Interfaces:**
- Consumes: `Dial`, `Session.Call`, `BrowserWSURL`, `ListTargets` (Part 2).
- Produces (package `cdp`): `SetWindowState(port int, targetID string, state string) error` — state is `"minimized"` or `"normal"`; empty targetID = first page target. `GetWindowState(port int, targetID string) (string, error)` — live via `Browser.getWindowBounds` (never cached).

- [ ] **Step 1: Write the failing test**

Create `htcli/internal/cdp/window_test.go`. Reuse the `fakeCDP` helper from `session_test.go` (same package) plus an httptest mux for discovery:

```go
package cdp

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

// fakeBrowserEndpoints serves /json, /json/version and a browser WS that
// records Browser.* calls.
func fakeBrowserEndpoints(t *testing.T, windowState string, calls *[]string) int {
	t.Helper()
	mux := http.NewServeMux()
	up := websocket.Upgrader{}
	mux.HandleFunc("/browser-ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()
		for {
			var m fakeMsg
			if err := conn.ReadJSON(&m); err != nil {
				return // intentionally not logged: client close ends fake loop
			}
			*calls = append(*calls, m.Method)
			switch m.Method {
			case "Browser.getWindowForTarget":
				conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{"windowId": 7}})
			case "Browser.getWindowBounds":
				conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{"bounds": map[string]any{"windowState": windowState}}})
			default:
				conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
			}
		}
	})
	var port int
	mux.HandleFunc("/json/version", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"webSocketDebuggerUrl":"ws://127.0.0.1:` + strconvItoa(port) + `/browser-ws"}`))
	})
	mux.HandleFunc("/json", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"id":"T1","type":"page","title":"t","url":"u","webSocketDebuggerUrl":"ws://127.0.0.1:` + strconvItoa(port) + `/page-ws"}]`))
	})
	port = testServer(t, mux) // helper from discover_test.go
	return port
}

func TestSetWindowStateMinimized(t *testing.T) {
	var calls []string
	port := fakeBrowserEndpoints(t, "normal", &calls)
	if err := SetWindowState(port, "", "minimized"); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	joined := strings.Join(calls, ",")
	if !strings.Contains(joined, "Browser.getWindowForTarget") || !strings.Contains(joined, "Browser.setWindowBounds") {
		t.Fatalf("calls = %v", calls)
	}
}

func TestGetWindowStateLive(t *testing.T) {
	var calls []string
	port := fakeBrowserEndpoints(t, "minimized", &calls)
	state, err := GetWindowState(port, "T1")
	if err != nil || state != "minimized" {
		t.Fatalf("want minimized, got %q (%v)", state, err)
	}
}
```

(Add `func strconvItoa(i int) string { return strconv.Itoa(i) }` or import `strconv` directly — whichever keeps the file compiling; the closure over `port` is why the URL is built at request time.)

- [ ] **Step 2: Run test to verify it fails**

Run: `cd htcli && go test ./internal/cdp/ -run TestSetWindowState -v`
Expected: compile error — `SetWindowState` undefined.

- [ ] **Step 3: Implement window.go**

```go
package cdp

import (
	"errors"
	"fmt"
)

// browserSessionFor resolves the target (empty = first page) and opens a
// browser-level session, returning the windowId for the target.
func browserSessionFor(port int, targetID string) (*Session, int, error) {
	if targetID == "" {
		targets, err := ListTargets(port)
		if err != nil {
			return nil, 0, err
		}
		if len(targets) == 0 {
			return nil, 0, errors.New("no page targets open")
		}
		targetID = targets[0].ID
	}
	wsURL, err := BrowserWSURL(port)
	if err != nil {
		return nil, 0, err
	}
	s, err := Dial(wsURL)
	if err != nil {
		return nil, 0, err
	}
	var win struct {
		WindowID int `json:"windowId"`
	}
	if err := s.Call("Browser.getWindowForTarget", map[string]any{"targetId": targetID}, &win); err != nil {
		if cerr := s.Close(); cerr != nil {
			fmt.Printf("[htcli] closing browser session after error: %v\n", cerr)
		}
		return nil, 0, err
	}
	return s, win.WindowID, nil
}

// SetWindowState minimizes or restores the window owning targetID
// (empty targetID = first page target). state: "minimized" | "normal".
func SetWindowState(port int, targetID string, state string) error {
	s, windowID, err := browserSessionFor(port, targetID)
	if err != nil {
		return err
	}
	defer s.Close()
	return s.Call("Browser.setWindowBounds", map[string]any{
		"windowId": windowID,
		"bounds":   map[string]any{"windowState": state},
	}, nil)
}

// GetWindowState reads the live window state via Browser.getWindowBounds.
func GetWindowState(port int, targetID string) (string, error) {
	s, windowID, err := browserSessionFor(port, targetID)
	if err != nil {
		return "", err
	}
	defer s.Close()
	var res struct {
		Bounds struct {
			WindowState string `json:"windowState"`
		} `json:"bounds"`
	}
	if err := s.Call("Browser.getWindowBounds", map[string]any{"windowId": windowID}, &res); err != nil {
		return "", err
	}
	return res.Bounds.WindowState, nil
}
```

- [ ] **Step 4: Run tests**

Run: `cd htcli && go test ./internal/cdp/ -v`
Expected: all PASS.

- [ ] **Step 5: Wire hide/show/status into browser.go**

Add to `htcli/internal/commands/browser.go`:

```go
var browserHideCmd = &cobra.Command{
	Use:   "hide",
	Short: "Minimize the CDP browser window (not applicable to headless)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := cdp.SetWindowState(GetCDPPort(), GetTabTarget(), "minimized"); err != nil {
			return err
		}
		fmt.Println("Browser hidden (minimized)")
		return nil
	},
}

var browserShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Restore the CDP browser window",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := cdp.SetWindowState(GetCDPPort(), GetTabTarget(), "normal"); err != nil {
			return err
		}
		fmt.Println("Browser shown")
		return nil
	},
}
```

Register both in `init()`. In `browserStatusCmd`, when the port is alive and state is non-headless, also print the live window state:

```go
		if st != nil && !st.Headless {
			if ws, err := cdp.GetWindowState(port, ""); err == nil {
				fmt.Printf("Window: %s\n", ws)
			} else {
				fmt.Printf("Window: unknown (%v)\n", err)
			}
		}
```

- [ ] **Step 6: Manual verification**

```bash
cd htcli && go build -o bin/htcli ./cmd/htcli
./bin/htcli browser start
./bin/htcli browser hide     # window minimizes
./bin/htcli browser status   # Window: minimized
./bin/htcli browser show     # window restores
./bin/htcli browser stop
```

- [ ] **Step 7: Commit**

```bash
git add htcli/internal/cdp/window.go htcli/internal/cdp/window_test.go htcli/internal/commands/browser.go
git commit -m "feat(htcli): browser hide/show via Browser.setWindowBounds

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```
