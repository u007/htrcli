# Part 5: Trusted Input, Navigation, Routing, Docs

Spec: `docs/superpowers/specs/2026-07-10-htcli-cdp-transport-design.md`. Depends on Parts 1–4. Delivers the user-visible feature: `htcli --cdp fill/click/... ` end to end.

---

### Task 10: Trusted input + nav over CDP

**Files:**
- Create: `htcli/internal/cdp/input.go`
- Create: `htcli/internal/cdp/nav.go`
- Test: `htcli/internal/cdp/input_test.go`

**Interfaces:**
- Consumes: `Session.Call`, `WaitEvent` (Task 4), `ExecDOM`, `evaluate` (Task 9), `api.Command`/`api.TargetSelector`.
- Produces (package `cdp`):
  - `Click(s *Session, targetID string, sel *api.TargetSelector, action string) error` — action `click|dblclick|rightclick`; runs the bundle's `prepareClick` (scroll + coords), `Target.activateTarget`, then `Input.dispatchMouseEvent` pressed/released
  - `Press(s *Session, key string) error` — key spec like `Enter`, `Ctrl+a` resolved to CDP key events (port the key mapping from `src/background/cdpInput.ts` `dispatchCdpKey` — same modifiers bitmask: Alt=1, Ctrl=2, Meta=4, Shift=8)
  - `Navigate(s *Session, url string, timeoutMs int) error` — `Page.enable` + `Page.navigate` + `WaitEvent("Page.loadEventFired", timeout)`
  - `Screenshot(s *Session) ([]byte, error)` — `Page.captureScreenshot` format png, base64-decoded

- [ ] **Step 1: Write the failing test**

Create `htcli/internal/cdp/input_test.go`:

```go
package cdp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/u007/htcli/internal/api"
)

// clickFake answers prepare-exec with coords and records Input.* dispatches.
func clickFake(t *testing.T, methods *[]string) string {
	return fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		*methods = append(*methods, m.Method)
		if m.Method == "Runtime.evaluate" {
			var p struct {
				Expression string `json:"expression"`
			}
			json.Unmarshal(m.Params, &p)
			switch {
			case strings.Contains(p.Expression, "typeof window.__htcliDom"):
				conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
					"result": map[string]any{"type": "string", "value": "object"}}})
			case strings.Contains(p.Expression, "prepareClick"):
				conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
					"result": map[string]any{"type": "object", "value": map[string]any{
						"id": "1", "success": true, "data": map[string]any{"x": 120.5, "y": 240.0}}}}})
			default:
				conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
					"result": map[string]any{"type": "undefined"}}})
			}
			return
		}
		conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
	})
}

func TestClickDispatchesTrustedInput(t *testing.T) {
	var methods []string
	url := clickFake(t, &methods)
	s, err := Dial(url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer s.Close()

	if err := Click(s, "T1", &api.TargetSelector{Selector: "#submit"}, "click"); err != nil {
		t.Fatalf("click: %v", err)
	}
	joined := strings.Join(methods, ",")
	for _, want := range []string{"Target.activateTarget", "Input.dispatchMouseEvent"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %s in %v", want, methods)
		}
	}
	// pressed + released
	if strings.Count(joined, "Input.dispatchMouseEvent") != 2 {
		t.Errorf("want exactly 2 mouse events, got %v", methods)
	}
}

func TestPressEnter(t *testing.T) {
	var methods []string
	url := clickFake(t, &methods)
	s, _ := Dial(url)
	defer s.Close()

	if err := Press(s, "Enter"); err != nil {
		t.Fatalf("press: %v", err)
	}
	if strings.Count(strings.Join(methods, ","), "Input.dispatchKeyEvent") != 2 {
		t.Errorf("want keyDown+keyUp, got %v", methods)
	}
}

func TestNavigateWaitsForLoad(t *testing.T) {
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
		if m.Method == "Page.navigate" {
			conn.WriteJSON(map[string]any{"method": "Page.loadEventFired", "params": map[string]any{"timestamp": 1}})
		}
	})
	s, _ := Dial(url)
	defer s.Close()

	if err := Navigate(s, "https://example.com/", 5000); err != nil {
		t.Fatalf("navigate: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd htcli && go test ./internal/cdp/ -run 'TestClick|TestPress|TestNavigate' -v`
Expected: compile error.

- [ ] **Step 3: Implement input.go**

```go
package cdp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/u007/htcli/internal/api"
)

// Click prepares the element via the bundle (wait actionable + scroll +
// viewport-center coords — same prepareClick the extension CDP path uses),
// activates the target, then dispatches trusted mouse events.
func Click(s *Session, targetID string, sel *api.TargetSelector, action string) error {
	prep, err := ExecDOM(s, api.Command{ID: "prep", Action: "prepareClick", Target: sel})
	if err != nil {
		return err
	}
	if !prep.Success {
		return fmt.Errorf("prepare failed: %s", prep.Error)
	}
	var coords struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	}
	data, err := json.Marshal(prep.Data)
	if err != nil {
		return fmt.Errorf("re-encoding prepare data: %w", err)
	}
	if err := json.Unmarshal(data, &coords); err != nil {
		return fmt.Errorf("prepareClick returned no coordinates: %w", err)
	}

	// CDP input is dropped on unrendered tabs — activate first. The window
	// itself may still be minimized/backgrounded (spike-dependent; headless
	// always works).
	if err := s.Call("Target.activateTarget", map[string]any{"targetId": targetID}, nil); err != nil {
		return fmt.Errorf("activating target: %w", err)
	}

	button := "left"
	if action == "rightclick" {
		button = "right"
	}
	clickCount := 1
	if action == "dblclick" {
		clickCount = 2
	}
	buttons := 1
	if button == "right" {
		buttons = 2
	}
	for _, typ := range []string{"mousePressed", "mouseReleased"} {
		if err := s.Call("Input.dispatchMouseEvent", map[string]any{
			"type": typ, "x": coords.X, "y": coords.Y,
			"button": button, "clickCount": clickCount, "buttons": buttons, "modifiers": 0,
		}, nil); err != nil {
			return fmt.Errorf("dispatch %s: %w", typ, err)
		}
	}
	return nil
}

// Press dispatches a trusted key press to whatever holds focus. Key specs:
// "Enter", "Tab", "Ctrl+a", "Shift+Tab" — port the exact key/keyCode/text
// mapping from src/background/cdpInput.ts dispatchCdpKey (modifier bitmask:
// Alt=1, Ctrl=2, Meta=4, Shift=8; named keys get windowsVirtualKeyCode;
// single chars get text so char events fire).
func Press(s *Session, keySpec string) error {
	parts := strings.Split(keySpec, "+")
	key := parts[len(parts)-1]
	modifiers := 0
	for _, mod := range parts[:len(parts)-1] {
		switch strings.ToLower(mod) {
		case "alt":
			modifiers |= 1
		case "ctrl", "control":
			modifiers |= 2
		case "meta", "cmd":
			modifiers |= 4
		case "shift":
			modifiers |= 8
		default:
			return fmt.Errorf("unknown modifier %q in %q", mod, keySpec)
		}
	}
	params := map[string]any{"key": key, "modifiers": modifiers}
	// Copy the named-key virtual keycode table from cdpInput.ts verbatim
	// (Enter=13, Tab=9, Escape=27, Backspace=8, Delete=46, arrows 37-40,
	// Home=36, End=35, PageUp=33, PageDown=34).
	if code, ok := namedKeyCodes[key]; ok {
		params["windowsVirtualKeyCode"] = code
	} else if len([]rune(key)) == 1 {
		params["text"] = key
	}
	for _, typ := range []string{"keyDown", "keyUp"} {
		p := map[string]any{"type": typ}
		for k, v := range params {
			if typ == "keyUp" && k == "text" {
				continue // text only on keyDown
			}
			p[k] = v
		}
		if err := s.Call("Input.dispatchKeyEvent", p, nil); err != nil {
			return fmt.Errorf("dispatch %s: %w", typ, err)
		}
	}
	return nil
}

var namedKeyCodes = map[string]int{
	"Enter": 13, "Tab": 9, "Escape": 27, "Backspace": 8, "Delete": 46,
	"ArrowLeft": 37, "ArrowUp": 38, "ArrowRight": 39, "ArrowDown": 40,
	"Home": 36, "End": 35, "PageUp": 33, "PageDown": 34,
}
```

- [ ] **Step 4: Implement nav.go**

```go
package cdp

import (
	"encoding/base64"
	"fmt"
	"time"
)

// Navigate loads a URL and waits for Page.loadEventFired, bounded by
// timeoutMs (the global --timeout). SPA route changes never fire load —
// same semantics as the extension transport's navigate wait.
func Navigate(s *Session, url string, timeoutMs int) error {
	if err := s.Call("Page.enable", nil, nil); err != nil {
		return fmt.Errorf("Page.enable: %w", err)
	}
	var nav struct {
		ErrorText string `json:"errorText"`
	}
	if err := s.Call("Page.navigate", map[string]any{"url": url}, &nav); err != nil {
		return err
	}
	if nav.ErrorText != "" {
		return fmt.Errorf("navigation failed: %s", nav.ErrorText)
	}
	if _, err := s.WaitEvent("Page.loadEventFired", time.Duration(timeoutMs)*time.Millisecond); err != nil {
		return fmt.Errorf("page did not finish loading: %w", err)
	}
	return nil
}

// Screenshot captures the page as PNG.
func Screenshot(s *Session) ([]byte, error) {
	var res struct {
		Data string `json:"data"`
	}
	if err := s.Call("Page.captureScreenshot", map[string]any{"format": "png"}, &res); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(res.Data)
}
```

- [ ] **Step 5: Run tests, then commit**

Run: `cd htcli && go test ./internal/cdp/ -v` — all PASS.

```bash
git add htcli/internal/cdp/input.go htcli/internal/cdp/nav.go htcli/internal/cdp/input_test.go
git commit -m "feat(htcli): trusted CDP input, navigation, screenshot

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 11: Route `--cdp` through existing commands

**Files:**
- Create: `htcli/internal/commands/cdp_exec.go`
- Modify: `htcli/internal/commands/interact.go` (`runInteract`), `htcli/internal/commands/inspect.go` (eval/value/find/screenshot runners), `htcli/internal/commands/navigate.go` (open), `htcli/internal/commands/tabs.go` (list)
- Test: `htcli/internal/commands/cdp_exec_test.go`

**Interfaces:**
- Consumes: `UseCDP()`, `GetCDPPort()`, `GetTabTarget()` (Part 1); `PageSession`, `ExecDOM`, `Evaluate`, `Click`, `Press`, `Navigate`, `Screenshot`, `ListTargets` (Parts 2–4).
- Produces: every v1 command honors the transport. Pattern: each existing `RunE` body branches FIRST on `UseCDP()`.

- [ ] **Step 1: Implement cdp_exec.go**

```go
package commands

import (
	"fmt"
	"strings"

	"github.com/u007/htcli/internal/api"
	"github.com/u007/htcli/internal/cdp"
	"github.com/u007/htcli/internal/output"
)

// cdpSession opens the target page session for the current --tab/--cdp-port.
func cdpSession() (*cdp.Session, string, error) {
	targetID := GetTabTarget()
	s, err := cdp.PageSession(GetCDPPort(), targetID)
	if err != nil {
		return nil, "", err
	}
	if targetID == "" {
		targets, err := cdp.ListTargets(GetCDPPort())
		if err == nil && len(targets) > 0 {
			targetID = targets[0].ID
		}
	}
	return s, targetID, nil
}

// runInteractCDP mirrors runInteract over the CDP transport.
func runInteractCDP(action, selector, value string) error {
	s, targetID, err := cdpSession()
	if err != nil {
		return err
	}
	defer s.Close()

	switch action {
	case "click", "dblclick", "rightclick":
		if err := cdp.Click(s, targetID, parseSelector(selector), action); err != nil {
			return err
		}
	case "pressKey":
		if err := cdp.Press(s, value); err != nil {
			return err
		}
	default: // fill, select, check, uncheck, clear, type — DOM verbs
		result, err := cdp.ExecDOM(s, api.Command{
			ID: "1", Action: action, Target: parseSelector(selector), Value: value,
		})
		if err != nil {
			return err
		}
		if !result.Success {
			return fmt.Errorf("%s failed: %s", action, result.Error)
		}
	}
	if output.JSONOutput {
		output.PrintJSON(map[string]any{"success": true, "action": action})
		return nil
	}
	fmt.Printf("%s %s (cdp)\n", strings.Title(action), selector)
	return nil
}
```

- [ ] **Step 2: Branch runInteract**

At the top of `runInteract` in `interact.go`:

```go
func runInteract(action, selector, value string) error {
	if UseCDP() {
		return runInteractCDP(action, selector, value)
	}
	// ... existing body unchanged
```

Apply the same two-line branch to the other v1 runners:
- `eval` (inspect.go): `if UseCDP() { s, _, err := cdpSession(); ...; raw, err := cdp.Evaluate(s, expression); print raw }`
- `value`, `find`, `page` (inspect.go): route through `cdp.ExecDOM` with actions `getValue`, `find`, `getPageInfo`; print `result.Data` (JSON mode: whole result), matching each command's existing plain-text shape where cheap, else fall back to JSON printing.
- `screenshot` (inspect.go): `cdp.Screenshot`, write bytes to the same output path logic the extension branch uses.
- `open` (navigate.go): `cdp.Navigate(s, url, timeout)` — `timeout` is the existing global flag variable.
- `tabs list` (tabs.go): `cdp.ListTargets(GetCDPPort())`, print `ID  Title  URL` table (no Active column — CDP has no reliable notion; extension table unchanged).

- [ ] **Step 3: Write the routing test**

Create `htcli/internal/commands/cdp_exec_test.go`:

```go
package commands

import (
	"strings"
	"testing"
)

func TestRunInteractCDPFailsClearlyWhenNotRunning(t *testing.T) {
	resetTransportState()
	cdpFlag = true
	defer resetTransportState()
	// Point at a dead port so PageSession fails fast.
	// (viper key set directly; GetCDPPort reads it.)
	setViperCDPPort(t, 1)

	err := runInteractCDP("fill", "#email", "x")
	if err == nil || !strings.Contains(err.Error(), "htcli browser start") {
		t.Fatalf("want ErrNotRunning guidance, got %v", err)
	}
}
```

with helper:

```go
func setViperCDPPort(t *testing.T, port int) {
	t.Helper()
	viper.Set("cdp-port", port)
	t.Cleanup(func() { viper.Set("cdp-port", 0) })
}
```

(add `"github.com/spf13/viper"` import).

- [ ] **Step 4: Run tests + build**

Run: `cd htcli && go build ./... && go test ./... -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add htcli/internal/commands/
git commit -m "feat(htcli): route v1 commands over CDP with --cdp/--transport

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 12: Integration smoke test + docs

**Files:**
- Create: `htcli/internal/cdp/integration_test.go` (build tag `integration`)
- Modify: `htcli/README.md`, `GUIDE.md`, `Makefile` (target `htcli-test-integration`), `CHANGELOG.md`

- [ ] **Step 1: Write the gated smoke test**

Create `htcli/internal/cdp/integration_test.go` covering the spec's four likeliest breakages: fresh-profile start, `/json` discovery, fill on a React-controlled input, screenshot while hidden.

```go
//go:build integration

package cdp

import (
	"os"
	"strings"
	"testing"

	"github.com/u007/htcli/internal/api"
)

// Requires a real Chrome. Run: make htcli-test-integration
// Uses port 9223 + a temp profile so it never collides with a dev browser.
func TestCDPEndToEnd(t *testing.T) {
	const port = 9223
	chrome, err := FindChrome(os.Getenv("HTCLI_CHROME_PATH"))
	if err != nil {
		t.Skipf("no Chrome available: %v", err)
	}
	t.Setenv("HOME", t.TempDir()) // fresh profile + state file

	st, err := StartBrowser(chrome, port, true /* headless */)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() {
		if err := StopBrowser(); err != nil {
			t.Logf("stop: %v", err)
		}
	})
	if st.Port != port {
		t.Fatalf("state port %d", st.Port)
	}

	// Discovery.
	targets, err := ListTargets(port)
	if err != nil || len(targets) == 0 {
		t.Fatalf("targets: %v %v", targets, err)
	}
	s, err := Dial(targets[0].WebSocketDebuggerURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer s.Close()

	// React-controlled input: value set via native setter must survive a
	// React re-render (the framework must SEE the input event).
	page := `data:text/html,<div id="root"></div><script>
		const root = document.getElementById('root');
		let state = '';
		function render() {
			root.innerHTML = '<input id="email" value="' + state + '"><span id="echo">' + state + '</span>';
			root.querySelector('#email').addEventListener('input', e => { state = e.target.value; render(); });
		}
		render();
	</script>`
	if err := Navigate(s, page, 10000); err != nil {
		t.Fatalf("navigate: %v", err)
	}
	result, err := ExecDOM(s, api.Command{ID: "1", Action: "fill",
		Target: &api.TargetSelector{Selector: "#email"}, Value: "james@mercstudio.com"})
	if err != nil || !result.Success {
		t.Fatalf("fill: %v %+v", err, result)
	}
	echo, err := Evaluate(s, "document.getElementById('echo').textContent")
	if err != nil || !strings.Contains(string(echo), "james@mercstudio.com") {
		t.Fatalf("controlled-input state not updated: %s (%v)", echo, err)
	}

	// Screenshot (headless = the guaranteed hidden mode).
	png, err := Screenshot(s)
	if err != nil || len(png) < 1000 {
		t.Fatalf("screenshot: %d bytes, %v", len(png), err)
	}
}
```

- [ ] **Step 2: Add Makefile target**

```makefile
htcli-test-integration:
	cd htcli && go test -tags integration ./internal/cdp/ -run TestCDPEndToEnd -v
```

- [ ] **Step 3: Run it**

Run: `make htcli-test-integration`
Expected: PASS on this Mac (Chrome installed). Fix anything it flushes out.

- [ ] **Step 4: Documentation**

- `htcli/README.md` — new "CDP transport" section: what `--cdp` is for (restricted pages like the Chrome Web Store dev console; background/headless runs), `browser start/stop/status/hide/show`, sign-in-once note, tab-ID namespace note (`--tab` numeric = extension, hex target ID = CDP).
- `GUIDE.md` — usage walkthrough: `htcli browser start` → sign in → `htcli --cdp open <devconsole url>` → `htcli --cdp fill ...`; Chrome 136 dedicated-profile explanation; headless first-run caveat; spike-determined statement on whether `hide` supports input verbs (copy the conclusion from `docs/superpowers/spikes/2026-07-10-cdp-minimized-window.md`); security note: the debugging port is an unauthenticated localhost-only control channel into a signed-in profile — same trust model as the localhost daemon, minus the bearer token.
- `CHANGELOG.md` — feature entry.

- [ ] **Step 5: Full check + commit**

```bash
cd htcli && go build ./... && go test ./... && cd .. && bun run check && bun run typecheck
git add htcli/ GUIDE.md CHANGELOG.md Makefile
git commit -m "test(htcli): CDP integration smoke test; document CDP transport

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```
