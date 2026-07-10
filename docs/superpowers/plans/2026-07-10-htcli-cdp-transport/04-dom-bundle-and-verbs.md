# Part 4: DOM Bundle & DOM Verbs

Spec: `docs/superpowers/specs/2026-07-10-htcli-cdp-transport-design.md`. Depends on Parts 1–3.

Strategy (spec "DOM verb engine" decision): do NOT reimplement the selector/actionability engine. `src/contentScript/commandExecutor.ts` already exports `executeCommand(command: Command): Promise<CommandResult>` and guards all `chrome.*` access behind `typeof chrome !== "undefined"` checks. Build it into a standalone IIFE bundle exposing `window.__htcliDom.exec(command)`, embed via `go:embed`, inject with `Runtime.evaluate` on first use per page.

---

### Task 8: DOM JS bundle

**Files:**
- Create: `src/cdpBundle/index.ts`
- Create: `vite.cdp.config.ts` (repo root)
- Create: `src/cdpBundle/index.test.ts`
- Modify: `package.json` (scripts `cdp-bundle:build`, hook into `build`)
- Output (committed): `htcli/internal/cdp/bundle/htcli-dom.js`

**Interfaces:**
- Consumes: `executeCommand` from `src/contentScript/commandExecutor.ts`; `Command`/`CommandResult` from `src/types/commands.ts`.
- Produces: a self-contained IIFE that sets `window.__htcliDom = { exec, version }` where `exec(command: Command) => Promise<CommandResult>`. Idempotent: re-evaluating the bundle replaces the global harmlessly. Task 9 evaluates this file's source once per page, then calls `window.__htcliDom.exec(<json>)`.

- [ ] **Step 1: Write the bundle entry**

Create `src/cdpBundle/index.ts`:

```ts
// Standalone DOM-command bundle for the htcli --cdp transport.
// Built as an IIFE (vite.cdp.config.ts) and embedded in the htcli Go binary
// via go:embed; injected into pages with Runtime.evaluate. Reuses the exact
// selector/actionability/fill engine the extension content script uses, so
// the two transports cannot drift.
import type { Command, CommandResult } from "../types/commands";
import { executeCommand } from "../contentScript/commandExecutor";

declare global {
	interface Window {
		__htcliDom?: {
			exec: (command: Command) => Promise<CommandResult>;
			version: number;
		};
	}
}

window.__htcliDom = {
	exec: (command: Command) => executeCommand(command),
	version: 1,
};
```

- [ ] **Step 2: Write vite.cdp.config.ts**

```ts
import { resolve } from "node:path";
import { defineConfig } from "vite";

// Builds the htcli CDP DOM bundle: a single self-contained IIFE with no
// chrome.* requirements (commandExecutor guards all chrome access), written
// directly into the Go embed directory.
export default defineConfig({
	build: {
		outDir: "htcli/internal/cdp/bundle",
		emptyOutDir: false,
		minify: false,
		lib: {
			entry: resolve(__dirname, "src/cdpBundle/index.ts"),
			formats: ["iife"],
			name: "__htcliDomBundle",
			fileName: () => "htcli-dom.js",
		},
	},
});
```

- [ ] **Step 3: Add build scripts**

In `package.json` scripts add:

```json
"cdp-bundle:build": "vite build --config vite.cdp.config.ts"
```

and append `&& bun run cdp-bundle:build` to the existing `"build"` script so extension builds keep the bundle current.

- [ ] **Step 4: Build and inspect**

Run: `bun run cdp-bundle:build && head -c 300 htcli/internal/cdp/bundle/htcli-dom.js && grep -c "chrome\.runtime" htcli/internal/cdp/bundle/htcli-dom.js`
Expected: file exists, IIFE header. `chrome.runtime` references WILL appear (screenshot/evaluate passthrough paths) — acceptable because those actions are never routed through the bundle (Task 9 routes only DOM verbs) and all access is `typeof`-guarded.

- [ ] **Step 5: Write the failing behavior test**

Create `src/cdpBundle/index.test.ts` (Bun test, happy-dom — same setup as `src/contentScript/commandExecutor.test.ts`; copy its DOM-registration preamble exactly):

```ts
import { beforeEach, describe, expect, test } from "bun:test";
import "../contentScript/commandExecutor.test-setup"; // if the existing test uses a setup import, mirror it; otherwise copy its happy-dom registration lines verbatim
import "./index";

describe("__htcliDom bundle global", () => {
	beforeEach(() => {
		document.body.innerHTML = `<input id="email" type="text" />`;
	});

	test("exposes exec on window", () => {
		expect(typeof window.__htcliDom?.exec).toBe("function");
	});

	test("fill via exec sets value and fires input event", async () => {
		let inputFired = false;
		const el = document.querySelector<HTMLInputElement>("#email")!;
		el.addEventListener("input", () => {
			inputFired = true;
		});
		const result = await window.__htcliDom!.exec({
			id: "1",
			action: "fill",
			target: { selector: "#email" },
			value: "james@mercstudio.com",
		});
		expect(result.success).toBe(true);
		expect(el.value).toBe("james@mercstudio.com");
		expect(inputFired).toBe(true);
	});
});
```

(If `commandExecutor.test.ts` has no shared setup file, inline whatever it does at its top — the goal is the identical environment.)

- [ ] **Step 6: Run tests**

Run: `bun test src/cdpBundle/index.test.ts`
Expected: PASS. Then `bun run check:fix` and `bun run typecheck`.

- [ ] **Step 7: Commit (include the built bundle — go:embed needs it in-tree)**

```bash
git add src/cdpBundle/ vite.cdp.config.ts package.json htcli/internal/cdp/bundle/htcli-dom.js
git commit -m "feat: standalone DOM-command bundle for htcli CDP transport

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 9: DOM verbs over CDP

**Files:**
- Create: `htcli/internal/cdp/dom.go` (embed + exec plumbing)
- Test: `htcli/internal/cdp/dom_test.go`

**Interfaces:**
- Consumes: `Session.Call` (Task 4), `ListTargets` (Task 3), `bundle/htcli-dom.js` (Task 8), the extension's `api.Command`/`api.CommandResult` structs from `htcli/internal/api` (same JSON wire shapes the bundle expects).
- Produces (package `cdp`):
  - `PageSession(port int, targetID string) (*Session, error)` — dial target (empty = first page)
  - `ExecDOM(s *Session, cmd api.Command) (*api.CommandResult, error)` — ensures the bundle is installed, runs `window.__htcliDom.exec`, decodes the result
  - `Evaluate(s *Session, expression string) (json.RawMessage, error)` — plain `Runtime.evaluate` for `htcli eval` (user script, not bundle)
- Consumed by: Part 5 routing (`runInteractCDP` etc.).

- [ ] **Step 1: Write the failing test**

Create `htcli/internal/cdp/dom_test.go`. Fake a page WS whose `Runtime.evaluate` handler simulates: first call checks `window.__htcliDom` (absent → `undefined`), second installs, third execs:

```go
package cdp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/u007/htcli/internal/api"
)

func TestExecDOMInstallsBundleOnce(t *testing.T) {
	var expressions []string
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		if m.Method != "Runtime.evaluate" {
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
			return
		}
		var p struct {
			Expression string `json:"expression"`
		}
		json.Unmarshal(m.Params, &p)
		expressions = append(expressions, p.Expression)
		switch {
		case strings.Contains(p.Expression, "typeof window.__htcliDom"):
			// Report "not installed" the first time, "installed" after.
			installed := false
			for _, e := range expressions[:len(expressions)-1] {
				if strings.Contains(e, "__htcliDom = {") || strings.Contains(e, "__htcliDomBundle") {
					installed = true
				}
			}
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"result": map[string]any{"type": "string", "value": map[bool]string{true: "object", false: "undefined"}[installed]},
			}})
		case strings.Contains(p.Expression, "__htcliDom.exec"):
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"result": map[string]any{"type": "object", "value": map[string]any{"id": "1", "success": true, "data": "filled"}},
			}})
		default: // bundle installation
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"result": map[string]any{"type": "undefined"},
			}})
		}
	})

	s, err := Dial(url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer s.Close()

	result, err := ExecDOM(s, api.Command{ID: "1", Action: "fill", Target: &api.TargetSelector{Selector: "#email"}, Value: "x"})
	if err != nil {
		t.Fatalf("ExecDOM: %v", err)
	}
	if !result.Success {
		t.Fatalf("want success, got %+v", result)
	}

	// Second exec must NOT reinstall: expression count grows by exactly 2
	// (probe + exec), not 3.
	before := len(expressions)
	if _, err := ExecDOM(s, api.Command{ID: "2", Action: "getValue", Target: &api.TargetSelector{Selector: "#email"}}); err != nil {
		t.Fatalf("second ExecDOM: %v", err)
	}
	if grew := len(expressions) - before; grew != 2 {
		t.Fatalf("second exec issued %d evaluates (want 2: probe + exec, no reinstall)", grew)
	}
}

func TestExecDOMErrorResult(t *testing.T) {
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		var p struct {
			Expression string `json:"expression"`
		}
		json.Unmarshal(m.Params, &p)
		if strings.Contains(p.Expression, "typeof window.__htcliDom") {
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"result": map[string]any{"type": "string", "value": "object"}}})
			return
		}
		conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
			"result": map[string]any{"type": "object", "value": map[string]any{"id": "1", "success": false, "error": "Element not found: #nope"}},
		}})
	})
	s, _ := Dial(url)
	defer s.Close()

	result, err := ExecDOM(s, api.Command{ID: "1", Action: "click", Target: &api.TargetSelector{Selector: "#nope"}})
	if err != nil {
		t.Fatalf("transport must not error on command failure: %v", err)
	}
	if result.Success || !strings.Contains(result.Error, "not found") {
		t.Fatalf("want failed result, got %+v", result)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd htcli && go test ./internal/cdp/ -run TestExecDOM -v`
Expected: compile error — `ExecDOM` undefined.

- [ ] **Step 3: Implement dom.go**

```go
package cdp

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/u007/htcli/internal/api"
)

//go:embed bundle/htcli-dom.js
var domBundle string

// PageSession dials the target's WebSocket (empty targetID = first page).
func PageSession(port int, targetID string) (*Session, error) {
	targets, err := ListTargets(port)
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		return nil, errors.New("no page targets open")
	}
	if targetID == "" {
		return Dial(targets[0].WebSocketDebuggerURL)
	}
	for _, t := range targets {
		if t.ID == targetID {
			return Dial(t.WebSocketDebuggerURL)
		}
	}
	return nil, fmt.Errorf("no page target with id %q (list with: htcli tabs list --cdp)", targetID)
}

type evalResult struct {
	Result struct {
		Type    string          `json:"type"`
		Value   json.RawMessage `json:"value"`
		Subtype string          `json:"subtype"`
	} `json:"result"`
	ExceptionDetails *struct {
		Text      string `json:"text"`
		Exception *struct {
			Description string `json:"description"`
		} `json:"exception"`
	} `json:"exceptionDetails"`
}

func evaluate(s *Session, expression string, awaitPromise bool) (*evalResult, error) {
	var res evalResult
	err := s.Call("Runtime.evaluate", map[string]any{
		"expression":    expression,
		"returnByValue": true,
		"awaitPromise":  awaitPromise,
	}, &res)
	if err != nil {
		return nil, err
	}
	if res.ExceptionDetails != nil {
		msg := res.ExceptionDetails.Text
		if res.ExceptionDetails.Exception != nil {
			msg = res.ExceptionDetails.Exception.Description
		}
		return nil, fmt.Errorf("page exception: %s", msg)
	}
	return &res, nil
}

// ensureBundle installs the DOM bundle unless the page already has it.
func ensureBundle(s *Session) error {
	probe, err := evaluate(s, "typeof window.__htcliDom", false)
	if err != nil {
		return err
	}
	var typ string
	if err := json.Unmarshal(probe.Result.Value, &typ); err != nil {
		return fmt.Errorf("decoding bundle probe: %w", err)
	}
	if typ == "object" {
		return nil
	}
	if _, err := evaluate(s, domBundle, false); err != nil {
		return fmt.Errorf("installing DOM bundle: %w", err)
	}
	return nil
}

// ExecDOM runs one command through the embedded bundle. A failed command is
// returned as a CommandResult with Success=false, not a Go error — matching
// the extension transport's semantics.
func ExecDOM(s *Session, cmd api.Command) (*api.CommandResult, error) {
	if err := ensureBundle(s); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(cmd)
	if err != nil {
		return nil, fmt.Errorf("encoding command: %w", err)
	}
	expr := fmt.Sprintf("window.__htcliDom.exec(%s)", payload)
	res, err := evaluate(s, expr, true)
	if err != nil {
		return nil, err
	}
	var result api.CommandResult
	if err := json.Unmarshal(res.Result.Value, &result); err != nil {
		return nil, fmt.Errorf("decoding command result: %w", err)
	}
	return &result, nil
}

// Evaluate runs a user-supplied expression (htcli eval) and returns the raw
// JSON value. Non-expression bodies (statements) are wrapped as an async
// function body, mirroring src/background/cdpEval.ts.
func Evaluate(s *Session, expression string) (json.RawMessage, error) {
	res, err := evaluate(s, expression, true)
	if err == nil {
		return res.Result.Value, nil
	}
	// Retry as async function body — `return`/`await` support.
	wrapped := fmt.Sprintf("(async () => { %s })()", expression)
	res, err2 := evaluate(s, wrapped, true)
	if err2 != nil {
		return nil, err // original error is the more useful one
	}
	return res.Result.Value, nil
}
```

- [ ] **Step 4: Run tests**

Run: `cd htcli && go test ./internal/cdp/ -v`
Expected: all PASS. (`go vet ./...` clean.)

- [ ] **Step 5: Commit**

```bash
git add htcli/internal/cdp/dom.go htcli/internal/cdp/dom_test.go
git commit -m "feat(htcli): DOM verbs over CDP via embedded bundle

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```
