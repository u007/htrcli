# htrcli Element Refs, findAll & File Upload Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Ship the `findAll` CLI subcommand, persistent element refs (`@e1`, `@e2`, …) that later commands accept transparently, and a `upload` command that sets file inputs without an OS file-picker — with the ref plumbing landing before upload depends on it.

**Architecture:** Refs get two transport-specific stores because the two transports have opposite lifetimes. The **extension** transport keeps an in-page `Map<string, Element>` (a new content-script module) that persists as long as the page does — the CLI is stateless and just passes `@e7` back as a selector; the page resolves it and errors loudly if the element is detached (SPA navigation) or the page reloaded (full navigation). The **CDP** transport (Go `--cdp`) has the opposite shape: each `htrcli` invocation opens a fresh short-lived `Session`, so refs are keyed to CDP's own durable `backendNodeId` (via real `DOM.*` calls — the first on the Go side) and the `@eN → backendNodeId` map is persisted to `~/.htrcli/refs.json`. Upload reuses that ref plumbing: `--cdp` sets files by `backendNodeId` (fresh selector resolve or `@eN`), the extension transport sets them by a debugger-resolved node id for a fresh CSS selector, and Firefox fails with an explicit unsupported error rather than a silent no-op.

**Tech Stack:** Go (cobra CLI, stdlib `net/http`, `github.com/gorilla/websocket` for CDP), TypeScript (Chrome/Firefox WebExtension APIs, `chrome.debugger` CDP), Bun test runner (`@happy-dom/global-registrator` for real DOM in tests), Go's `testing` package with `httptest` + the `fakeCDP` WebSocket harness.

## Global Constraints

- Package manager: `bun` only for the extension — never npm/yarn.
- Biome lint/format (tabs, double quotes) — run `bun run check:fix` before committing TS changes.
- Go tests: `go test ./...` from `htrcli/`. Module path is `github.com/u007/htrcli`.
- Async `chrome.runtime.onMessage` listeners must `return true` when responding asynchronously.
- Extension console/error logging prefix: `console.error/warn('[HTR NControl] ...')`.
- Ref id format: `@e<N>` where N is a positive integer, monotonically increasing per store (per-page for the extension registry, per `~/.htrcli/refs.json` document generation for CDP). A ref that cannot be resolved MUST produce an explicit `stale ref: ...` error — never a silent re-resolve.
- `@eN` refs are resolvable **only on the transport that minted them**. A CDP-minted ref used on the extension transport (or vice-versa) must fail with a clear cross-transport error, never a wrong-element match.
- CDP verbs not yet ported use the existing `errUnsupportedCDP(name)` helper (`internal/commands/cdp_exec.go`) — a sticky `transport=cdp` config must never silently misroute.
- No new external Go or npm dependencies for this plan.

---

### Task 1: `findAll` CLI subcommand

The `findAll` `CommandAction` and `handleFindAll` already exist on both the extension (`src/contentScript/commandExecutor.ts:188`) and CDP bundle (`htrcli/internal/cdp/bundle/htrcli-dom.js:643`) sides. This task only exposes the missing CLI subcommand, mirroring `find`.

**Files:**
- Modify: `htrcli/internal/commands/inspect.go`
- Test: `htrcli/internal/commands/findall_test.go` (create)

**Interfaces:**
- Consumes: `GetClient()`, `GetTabID()`, `UseCDP()`, `runInspectCDP(action, selector, attr string) error`, `parseSelector(arg string) *api.TargetSelector` (all existing).
- Produces: `findAllCmd` cobra command registered on `rootCmd`.

- [x] **Step 1: Write the failing test**

Create `htrcli/internal/commands/findall_test.go`:

```go
package commands

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/u007/htrcli/internal/api"
)

// TestFindAllSendsFindAllAction drives findAllCmd against a fake daemon and
// asserts it posts a command with action "findAll" and the parsed selector.
func TestFindAllSendsFindAllAction(t *testing.T) {
	var gotAction, gotSelector string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req api.CommandRequest
		json.NewDecoder(r.Body).Decode(&req)
		gotAction = req.Command.Action
		if req.Command.Target != nil {
			gotSelector = req.Command.Target.Selector
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(api.ApiResponse{
			OK:   true,
			Data: api.CommandResult{ID: "1", Success: true, Data: []any{}},
		})
	}))
	defer server.Close()

	// Point the package-global client at the fake daemon; ext transport.
	client = api.NewClient(server.URL, "")
	tabTarget = ""
	transportFlag = "ext"
	defer func() { client = nil; transportFlag = "" }()

	if err := findAllCmd.RunE(findAllCmd, []string{"button.primary"}); err != nil {
		t.Fatalf("findAll RunE: %v", err)
	}
	if gotAction != "findAll" {
		t.Fatalf("want action findAll, got %q", gotAction)
	}
	if gotSelector != "button.primary" {
		t.Fatalf("want selector button.primary, got %q", gotSelector)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `cd htrcli && go test ./internal/commands/... -run TestFindAllSendsFindAllAction -v`
Expected: FAIL — `findAllCmd` undefined.

- [x] **Step 3: Add `findAllCmd`**

In `htrcli/internal/commands/inspect.go`, add after `findCmd` (near line 50):

```go
var findAllCmd = &cobra.Command{
	Use:   "findAll <selector>",
	Short: "Find all matching elements and return their info",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if UseCDP() {
			return runInspectCDP("findAll", args[0], "")
		}
		c := GetClient()
		tabID, err := GetTabID()
		if err != nil {
			return err
		}
		result, err := c.ExecuteCommand(tabID, api.Command{
			ID:     "1",
			Action: "findAll",
			Target: parseSelector(args[0]),
		})
		if err != nil {
			return err
		}
		if err := commandError(result); err != nil {
			return err
		}
		if output.JSONOutput {
			output.PrintJSON(result)
			return nil
		}
		output.PrintJSON(result.Data)
		return nil
	},
}
```

Register it in the existing `init()` (near line 714), right after `rootCmd.AddCommand(findCmd)`:

```go
	rootCmd.AddCommand(findAllCmd)
```

- [x] **Step 4: Run test to verify it passes**

Run: `cd htrcli && go test ./internal/commands/... -run TestFindAllSendsFindAllAction -v`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add htrcli/internal/commands/inspect.go htrcli/internal/commands/findall_test.go
git commit -m "feat(htrcli): add findAll CLI subcommand"
```

---

### Task 2: `Ref` field on the selector types + `@`-prefix parsing

Adds the shared data field both transports key refs on, and teaches `parseSelector` that a `@e…` argument is a ref, not a CSS selector.

**Files:**
- Modify: `htrcli/internal/api/types.go`
- Modify: `htrcli/internal/commands/interact.go`
- Modify: `src/types/commands.ts`
- Test: `htrcli/internal/commands/commands_test.go`

**Interfaces:**
- Produces: `api.TargetSelector.Ref string` (JSON `ref`), `parseSelector("@e7") → &TargetSelector{Ref: "@e7"}`, TS `TargetSelector.ref?: string`, TS `RemoteElementInfo.ref?: string`.

- [x] **Step 1: Write the failing test**

Append to `htrcli/internal/commands/commands_test.go`:

```go
func TestParseSelector_Ref(t *testing.T) {
	s := parseSelector("@e7")
	if s.Ref != "@e7" {
		t.Errorf("expected Ref '@e7', got %q (selector=%q)", s.Ref, s.Selector)
	}
	if s.Selector != "" {
		t.Errorf("ref arg must not populate Selector, got %q", s.Selector)
	}
}

func TestParseSelector_RefLeavesRealSelectorsAlone(t *testing.T) {
	// An email like "@" mid-string is not a ref; only a leading @ is.
	s := parseSelector("input[name=email]")
	if s.Ref != "" {
		t.Errorf("expected no Ref for a CSS selector, got %q", s.Ref)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `cd htrcli && go test ./internal/commands/... -run TestParseSelector_Ref -v`
Expected: FAIL — `s.Ref` undefined (field does not exist).

- [x] **Step 3: Add the `Ref` field**

In `htrcli/internal/api/types.go`, add to the `TargetSelector` struct (after the `XPath` field near line 9):

```go
	// Ref is a persistent element handle ("@e7") minted by `find --ref` /
	// `findAll --ref`. When set, all other match strategies are ignored and
	// the element is looked up from the transport's ref store.
	Ref string `json:"ref,omitempty"`
```

- [x] **Step 4: Add the `@`-prefix branch to `parseSelector`**

In `htrcli/internal/commands/interact.go`, add at the very top of `parseSelector` (before the `name=` check, near line 15):

```go
	// A leading "@" marks a persistent element ref ("@e7"); it is resolved
	// from the transport's ref store, never treated as a CSS selector.
	if strings.HasPrefix(arg, "@") {
		return &api.TargetSelector{Ref: arg}
	}
```

- [x] **Step 5: Add the TS fields**

In `src/types/commands.ts`, add to `interface TargetSelector` (find the interface; add near its other optional string fields):

```typescript
	/** Persistent element ref ("@e7") minted by find/findAll --ref. */
	ref?: string;
```

And to `interface RemoteElementInfo` (after `attributes?` near line 84):

```typescript
	/** Ref id assigned when the command was run with --ref (assignRef). */
	ref?: string;
```

- [x] **Step 6: Run the Go test + TS typecheck**

Run: `cd htrcli && go test ./internal/commands/... -run TestParseSelector -v`
Expected: PASS
Run: `bun run typecheck`
Expected: no type errors

- [x] **Step 7: Commit**

```bash
git add htrcli/internal/api/types.go htrcli/internal/commands/interact.go src/types/commands.ts
git commit -m "feat(htrcli): add ref field to selector types and parse @e refs"
```

---

### Task 3: Extension in-page ref registry module

A content-script-world `Map<string, Element>`. Lives in the isolated content-script world (not MAIN) so it shares the same `Element` instances `elementFinder` returns. Persists across CLI calls because the page persists; resolution checks `isConnected` so a detached (SPA-navigated) element errors instead of silently matching.

**Files:**
- Create: `src/contentScript/refRegistry.ts`
- Test: `src/contentScript/refRegistry.test.ts`

**Interfaces:**
- Produces: `assignRef(el: Element): string`, `resolveRef(refId: string): Element` (throws on stale/unknown), `clearRefs(): void`, `refCount(): number`.

- [x] **Step 1: Write the failing test**

Create `src/contentScript/refRegistry.test.ts`:

```typescript
import { beforeEach, describe, expect, it } from "bun:test";
import "../test/domSetup";
import { assignRef, clearRefs, resolveRef } from "./refRegistry";

describe("refRegistry", () => {
	beforeEach(() => {
		clearRefs();
		document.body.innerHTML = "";
	});

	it("mints increasing @eN ids and resolves them back to the element", () => {
		const a = document.createElement("button");
		const b = document.createElement("input");
		document.body.append(a, b);

		const refA = assignRef(a);
		const refB = assignRef(b);
		expect(refA).toBe("@e1");
		expect(refB).toBe("@e2");
		expect(resolveRef(refA)).toBe(a);
		expect(resolveRef(refB)).toBe(b);
	});

	it("returns the same ref id for the same element (no duplicate handles)", () => {
		const a = document.createElement("button");
		document.body.appendChild(a);
		expect(assignRef(a)).toBe(assignRef(a));
	});

	it("throws a stale-ref error for a detached element", () => {
		const a = document.createElement("button");
		document.body.appendChild(a);
		const ref = assignRef(a);
		a.remove(); // SPA navigation / re-render detaches it
		expect(() => resolveRef(ref)).toThrow(/stale ref/);
	});

	it("throws for an unknown ref id", () => {
		expect(() => resolveRef("@e999")).toThrow(/stale ref/);
	});
});
```

Note: the `domSetup` helper lives at `src/test/domSetup.ts`, so from a test file in `src/contentScript/` the import is `../test/domSetup` (matching `commandExecutor.test.ts` and `elementFinder.test.ts`).

- [x] **Step 2: Run test to verify it fails**

Run: `bun test src/contentScript/refRegistry.test.ts`
Expected: FAIL — `./refRegistry` module does not exist.

- [x] **Step 3: Implement `refRegistry.ts`**

Create `src/contentScript/refRegistry.ts`:

```typescript
/**
 * In-page persistent element ref registry (extension transport).
 *
 * Refs ("@e7") let the CLI address an element across separate `htrcli`
 * invocations without re-describing it. The registry lives in the page's
 * content-script world, so it holds the very Element instances elementFinder
 * returns, and it persists as long as the page (and its content script) does.
 *
 * A full navigation reloads the content script and wipes this module state
 * (fresh Map). An SPA navigation keeps the module alive but detaches old
 * elements — resolveRef guards that with isConnected and errors explicitly,
 * per the spec's "never silently re-resolve a stale ref" rule.
 */

const refToEl = new Map<string, Element>();
const elToRef = new WeakMap<Element, string>();
let nextRef = 0;

/** Assign (or reuse) a ref id for an element. Idempotent per element. */
export function assignRef(el: Element): string {
	const existing = elToRef.get(el);
	if (existing) return existing;
	nextRef += 1;
	const refId = `@e${nextRef}`;
	refToEl.set(refId, el);
	elToRef.set(el, refId);
	return refId;
}

/**
 * Resolve a ref id back to its element. Throws an explicit stale-ref error if
 * the id was never minted (unknown / wrong page) or the element has since
 * been detached from the document.
 */
export function resolveRef(refId: string): Element {
	const el = refToEl.get(refId);
	if (!el) {
		throw new Error(
			`stale ref: ${refId} is not known on this page (it may have navigated or the ref was minted elsewhere)`,
		);
	}
	if (!el.isConnected) {
		refToEl.delete(refId);
		throw new Error(
			`stale ref: ${refId} points to an element that is no longer in the document (page re-rendered or navigated)`,
		);
	}
	return el;
}

/** Drop all refs. Called on full page navigation reset. */
export function clearRefs(): void {
	refToEl.clear();
	nextRef = 0;
}

/** Number of currently-held refs (diagnostics / tests). */
export function refCount(): number {
	return refToEl.size;
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `bun test src/contentScript/refRegistry.test.ts`
Expected: PASS

- [x] **Step 5: Biome check**

Run: `bun run check:fix`
Expected: no errors on the new files

- [x] **Step 6: Commit**

```bash
git add src/contentScript/refRegistry.ts src/contentScript/refRegistry.test.ts
git commit -m "feat(extension): add in-page element ref registry"
```

---

### Task 4: Resolve refs in elementFinder + assign refs in find/findAll

Wires the registry into the two ends: `findElement`/`findAllElementsRaw` resolve `target.ref` before any other strategy; `handleFind`/`handleFindAll` mint refs when the command carries `options.assignRef`.

**Files:**
- Modify: `src/contentScript/elementFinder.ts`
- Modify: `src/contentScript/commandExecutor.ts`
- Test: `src/contentScript/refResolution.test.ts` (create)

**Interfaces:**
- Consumes: `assignRef`, `resolveRef` from `refRegistry` (Task 3); `findElement`, `findElementInfo`, `getElementInfo` (existing).
- Produces: ref-aware `findAllElementsRaw`; `handleFind`/`handleFindAll` populate `RemoteElementInfo.ref` when `options.assignRef` is set.

- [x] **Step 1: Write the failing test**

Create `src/contentScript/refResolution.test.ts`:

```typescript
import { beforeEach, describe, expect, it } from "bun:test";
import "../test/domSetup";
import { executeCommand } from "./commandExecutor";
import { clearRefs } from "./refRegistry";

describe("ref resolution end-to-end", () => {
	beforeEach(() => {
		clearRefs();
		document.body.innerHTML = "";
	});

	it("find --ref mints a ref, and a later command resolves it", async () => {
		const btn = document.createElement("button");
		btn.id = "go";
		btn.textContent = "Go";
		document.body.appendChild(btn);

		const findRes = await executeCommand({
			id: "1",
			action: "find",
			target: { selector: "#go" },
			options: { assignRef: true },
		});
		expect(findRes.success).toBe(true);
		const info = findRes.data as { ref?: string };
		expect(info.ref).toBe("@e1");

		// A later getText addressed purely by the ref resolves to #go.
		const textRes = await executeCommand({
			id: "2",
			action: "getText",
			target: { ref: "@e1" },
		});
		expect(textRes.success).toBe(true);
		expect(textRes.data).toBe("Go");
	});

	it("errors loudly when a ref is stale (element removed)", async () => {
		const btn = document.createElement("button");
		btn.id = "gone";
		document.body.appendChild(btn);
		await executeCommand({
			id: "1",
			action: "find",
			target: { selector: "#gone" },
			options: { assignRef: true },
		});
		btn.remove();
		const res = await executeCommand({
			id: "2",
			action: "getText",
			target: { ref: "@e1" },
		});
		expect(res.success).toBe(false);
		expect(res.error).toMatch(/stale ref/);
	});
});
```

- [x] **Step 2: Run test to verify it fails**

Run: `bun test src/contentScript/refResolution.test.ts`
Expected: FAIL — refs are not resolved (getText by ref finds nothing) and `info.ref` is undefined.

- [x] **Step 3: Resolve refs in `elementFinder`**

In `src/contentScript/elementFinder.ts`, add the import at the top (after the existing type imports near line 12):

```typescript
import { resolveRef } from "./refRegistry";
```

Add a ref branch at the very top of `findAllElementsRaw` (before the `// 1. CSS selector` block near line 114):

```typescript
	// 0. Persistent ref — resolve from the in-page registry and short-circuit.
	// resolveRef throws an explicit stale-ref error rather than returning [].
	if (target.ref) {
		return [resolveRef(target.ref)];
	}
```

- [x] **Step 4: Assign refs in the find/findAll handlers**

In `src/contentScript/commandExecutor.ts`, add the import (after the `elementFinder` import block near line 28):

```typescript
import { assignRef } from "./refRegistry";
```

Change `executeAction`'s `find`/`findAll` cases (near line 186) to thread `options`:

```typescript
		case "find":
			return handleFind(requireTarget(target, action), options);
		case "findAll":
			return handleFindAll(requireTarget(target, action), options);
```

Replace `handleFind` and `handleFindAll` (near line 397) with the ref-aware versions:

```typescript
function handleFind(
	target: TargetSelector,
	options?: Command["options"],
): RemoteElementInfo | null {
	const element = findElement(target);
	if (!element) return null;
	const info = getElementInfo(element);
	if (options?.assignRef) info.ref = assignRef(element);
	return info;
}

function handleFindAll(
	target: TargetSelector,
	options?: Command["options"],
): RemoteElementInfo[] {
	const elements = findAllElements(target);
	return elements.map((el) => {
		const info = getElementInfo(el);
		if (options?.assignRef) info.ref = assignRef(el);
		return info;
	});
}
```

Add `findAllElements` to the `elementFinder` import list at the top of `commandExecutor.ts` (it currently imports `findElement, findElementInfo, getElementInfo, waitForActionableElement, waitForElement` — add `findAllElements`):

```typescript
import {
	findAllElements,
	findElement,
	findElementInfo,
	getElementInfo,
	waitForActionableElement,
	waitForElement,
} from "./elementFinder";
```

(`findElementInfo` may now be unused in `commandExecutor.ts`. If `bun run check` / typecheck flags it as unused, remove it from the import — it was only used by the old `handleFindAll`. Verify with the typecheck step before removing.)

- [x] **Step 5: Run test to verify it passes**

Run: `bun test src/contentScript/refResolution.test.ts`
Expected: PASS

- [x] **Step 6: Run the full content-script test suite + typecheck**

Run: `bun test src/contentScript/ && bun run typecheck`
Expected: PASS, no type errors (fix an unused `findElementInfo` import if flagged, per Step 4)

- [x] **Step 7: Biome + commit**

```bash
bun run check:fix
git add src/contentScript/elementFinder.ts src/contentScript/commandExecutor.ts src/contentScript/refResolution.test.ts
git commit -m "feat(extension): resolve and assign element refs in find/findAll"
```

---

### Task 5: CLI `--ref` flag on find/findAll (extension transport)

Adds the `--ref` flag that sets `options.assignRef` and prints the minted ref id(s). Extension transport only; the CDP path is Task 6.

**Files:**
- Modify: `htrcli/internal/commands/inspect.go`
- Test: `htrcli/internal/commands/findall_test.go`

**Interfaces:**
- Consumes: `findCmd`, `findAllCmd` (Tasks 1 + existing), `api.Command.Options`.
- Produces: `--ref` bool flag on both commands; `Options{"assignRef": true}` sent when set.

- [x] **Step 1: Write the failing test**

Append to `htrcli/internal/commands/findall_test.go`:

```go
func TestFindWithRefSetsAssignRefOption(t *testing.T) {
	var gotAssignRef bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req api.CommandRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Command.Options != nil {
			gotAssignRef, _ = req.Command.Options["assignRef"].(bool)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(api.ApiResponse{
			OK:   true,
			Data: api.CommandResult{ID: "1", Success: true, Data: map[string]any{"ref": "@e1", "tag": "button"}},
		})
	}))
	defer server.Close()

	client = api.NewClient(server.URL, "")
	tabTarget = ""
	transportFlag = "ext"
	findRef = true
	defer func() { client = nil; transportFlag = ""; findRef = false }()

	if err := findCmd.RunE(findCmd, []string{"#go"}); err != nil {
		t.Fatalf("find --ref RunE: %v", err)
	}
	if !gotAssignRef {
		t.Fatalf("expected assignRef=true option to be sent")
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `cd htrcli && go test ./internal/commands/... -run TestFindWithRefSetsAssignRefOption -v`
Expected: FAIL — `findRef` undefined.

- [x] **Step 3: Add the `--ref` flag and thread the option**

In `htrcli/internal/commands/inspect.go`, add a package-level var near the top (after the imports, before `findCmd`):

```go
// findRef, when set by --ref, makes find/findAll mint a persistent element
// ref ("@e7") the CLI can pass to later commands as the selector.
var findRef bool
```

Replace the body of `findCmd`'s `RunE` (near line 20) so it sends `assignRef` and prints the ref. On the extension transport:

```go
	RunE: func(cmd *cobra.Command, args []string) error {
		if UseCDP() {
			if findRef {
				return runFindRefCDP(args[0], false)
			}
			return runInspectCDP("find", args[0], "")
		}
		c := GetClient()
		tabID, err := GetTabID()
		if err != nil {
			return err
		}
		command := api.Command{ID: "1", Action: "find", Target: parseSelector(args[0])}
		if findRef {
			command.Options = map[string]any{"assignRef": true}
		}
		result, err := c.ExecuteCommand(tabID, command)
		if err != nil {
			return err
		}
		if output.JSONOutput {
			output.PrintJSON(result)
			return nil
		}
		if result.Data != nil {
			output.PrintJSON(result.Data)
		} else {
			fmt.Printf("Element not found: %s\n", args[0])
		}
		return nil
	},
```

Similarly, extend `findAllCmd` (from Task 1) to thread `assignRef` and, on CDP, call `runFindRefCDP(args[0], true)`:

```go
	RunE: func(cmd *cobra.Command, args []string) error {
		if UseCDP() {
			if findRef {
				return runFindRefCDP(args[0], true)
			}
			return runInspectCDP("findAll", args[0], "")
		}
		c := GetClient()
		tabID, err := GetTabID()
		if err != nil {
			return err
		}
		command := api.Command{ID: "1", Action: "findAll", Target: parseSelector(args[0])}
		if findRef {
			command.Options = map[string]any{"assignRef": true}
		}
		result, err := c.ExecuteCommand(tabID, command)
		if err != nil {
			return err
		}
		if err := commandError(result); err != nil {
			return err
		}
		if output.JSONOutput {
			output.PrintJSON(result)
			return nil
		}
		output.PrintJSON(result.Data)
		return nil
	},
```

Register the flags in `init()` (near line 714), after the `rootCmd.AddCommand(findAllCmd)` added in Task 1:

```go
	findCmd.Flags().BoolVar(&findRef, "ref", false, "mint a persistent element ref (@e7) instead of resolving inline")
	findAllCmd.Flags().BoolVar(&findRef, "ref", false, "mint persistent element refs for every match")
```

Note: `runFindRefCDP` is defined in Task 6. Until Task 6 lands, `--ref --cdp` will not compile; write Tasks 5 and 6 in order (this is why they are adjacent). To keep this task independently green, add a **temporary stub** at the bottom of `inspect.go` that Task 6 replaces:

```go
// runFindRefCDP is implemented in Task 6 (cdp_ref.go). Temporary stub so this
// task compiles on its own; DELETE this stub when Task 6 adds the real one.
func runFindRefCDP(selector string, all bool) error {
	return errUnsupportedCDP("find --ref")
}
```

- [x] **Step 4: Run test + full commands suite**

Run: `cd htrcli && go test ./internal/commands/... -run 'TestFind' -v`
Expected: PASS
Run: `cd htrcli && go test ./internal/commands/...`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add htrcli/internal/commands/inspect.go htrcli/internal/commands/findall_test.go
git commit -m "feat(htrcli): add --ref flag to find/findAll (extension transport)"
```

---

### Task 6: CDP ref allocation via backendNodeId + CLI-side ref store

The CDP transport has no in-page JS registry the CLI can address across its short-lived processes, so refs are keyed to CDP's durable `backendNodeId` and the `@eN → backendNodeId` map is persisted to `~/.htrcli/refs.json`. This task adds the first real `DOM.*` calls on the Go side and replaces Task 5's `runFindRefCDP` stub.

**Files:**
- Create: `htrcli/internal/cdp/elementref.go`
- Create: `htrcli/internal/commands/refstore.go`
- Modify: `htrcli/internal/commands/inspect.go` (delete the Task 5 stub, add the real `runFindRefCDP`)
- Test: `htrcli/internal/cdp/elementref_test.go`
- Test: `htrcli/internal/commands/refstore_test.go`

**Interfaces:**
- Consumes: `cdp.Session` (`Call`), `cdpSession()` (existing, `cdp_exec.go`).
- Produces:
  - `cdp.ResolveBackendNodeID(s *Session, cssSelector string) (int64, error)` — CSS selector → backendNodeId, error if not found.
  - `cdp.ResolveRefTargets(s *Session, cssSelector string) ([]int64, error)` — all matches (findAll --ref).
  - `commands.RefStore` with `LoadRefStore() (*RefStore, error)`, `(*RefStore) Alloc(backendNodeID int64) string`, `(*RefStore) Lookup(refID string) (int64, bool)`, `(*RefStore) Save() error`.
  - `commands.runFindRefCDP(selector string, all bool) error`.

- [x] **Step 1: Write the failing CDP test**

Create `htrcli/internal/cdp/elementref_test.go`:

```go
package cdp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

// TestResolveBackendNodeID walks DOM.getDocument -> DOM.querySelector ->
// DOM.describeNode and returns the backendNodeId.
func TestResolveBackendNodeID(t *testing.T) {
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		switch m.Method {
		case "DOM.enable":
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
		case "DOM.getDocument":
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"root": map[string]any{"nodeId": 1},
			}})
		case "DOM.querySelector":
			var p struct {
				NodeID   int64  `json:"nodeId"`
				Selector string `json:"selector"`
			}
			json.Unmarshal(m.Params, &p)
			if p.NodeID != 1 || !strings.Contains(p.Selector, "#go") {
				t.Errorf("unexpected querySelector params: %s", m.Params)
			}
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{"nodeId": 42}})
		case "DOM.describeNode":
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"node": map[string]any{"backendNodeId": 9007},
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

	backendID, err := ResolveBackendNodeID(s, "#go")
	if err != nil {
		t.Fatalf("ResolveBackendNodeID: %v", err)
	}
	if backendID != 9007 {
		t.Fatalf("want backendNodeId 9007, got %d", backendID)
	}
}

// TestResolveBackendNodeIDNotFound: querySelector returns nodeId 0 (no match).
func TestResolveBackendNodeIDNotFound(t *testing.T) {
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		switch m.Method {
		case "DOM.getDocument":
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"root": map[string]any{"nodeId": 1},
			}})
		case "DOM.querySelector":
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{"nodeId": 0}})
		default:
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
		}
	})
	s, _ := Dial(url)
	defer s.Close()
	if _, err := ResolveBackendNodeID(s, "#missing"); err == nil {
		t.Fatal("want error for no-match selector, got nil")
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `cd htrcli && go test ./internal/cdp/... -run TestResolveBackendNodeID -v`
Expected: FAIL — `ResolveBackendNodeID` undefined.

- [x] **Step 3: Implement `elementref.go`**

Create `htrcli/internal/cdp/elementref.go`:

```go
package cdp

import "fmt"

// backendNodeID is CDP's durable per-document element handle. Unlike a
// RemoteObjectId (which expires on GC), a backendNodeId stays valid for the
// life of the document, so htrcli keys persistent refs on it.

// ResolveBackendNodeID resolves a CSS selector to a single backendNodeId via
// DOM.getDocument -> DOM.querySelector -> DOM.describeNode. Only CSS selectors
// are supported on the CDP ref path (DOM.querySelector is CSS-only); callers
// pass the raw CSS string. Returns an error if the selector matches nothing.
func ResolveBackendNodeID(s *Session, cssSelector string) (int64, error) {
	if err := s.Call("DOM.enable", nil, nil); err != nil {
		return 0, fmt.Errorf("DOM.enable: %w", err)
	}
	var doc struct {
		Root struct {
			NodeID int64 `json:"nodeId"`
		} `json:"root"`
	}
	if err := s.Call("DOM.getDocument", map[string]any{"depth": 0}, &doc); err != nil {
		return 0, fmt.Errorf("DOM.getDocument: %w", err)
	}
	var qs struct {
		NodeID int64 `json:"nodeId"`
	}
	if err := s.Call("DOM.querySelector", map[string]any{
		"nodeId":   doc.Root.NodeID,
		"selector": cssSelector,
	}, &qs); err != nil {
		return 0, fmt.Errorf("DOM.querySelector %q: %w", cssSelector, err)
	}
	if qs.NodeID == 0 {
		return 0, fmt.Errorf("no element matched CSS selector %q", cssSelector)
	}
	var desc struct {
		Node struct {
			BackendNodeID int64 `json:"backendNodeId"`
		} `json:"node"`
	}
	if err := s.Call("DOM.describeNode", map[string]any{"nodeId": qs.NodeID}, &desc); err != nil {
		return 0, fmt.Errorf("DOM.describeNode: %w", err)
	}
	return desc.Node.BackendNodeID, nil
}

// ResolveRefTargets resolves a CSS selector to the backendNodeIds of every
// match (findAll --ref) via DOM.querySelectorAll -> DOM.describeNode.
func ResolveRefTargets(s *Session, cssSelector string) ([]int64, error) {
	if err := s.Call("DOM.enable", nil, nil); err != nil {
		return nil, fmt.Errorf("DOM.enable: %w", err)
	}
	var doc struct {
		Root struct {
			NodeID int64 `json:"nodeId"`
		} `json:"root"`
	}
	if err := s.Call("DOM.getDocument", map[string]any{"depth": 0}, &doc); err != nil {
		return nil, fmt.Errorf("DOM.getDocument: %w", err)
	}
	var qs struct {
		NodeIDs []int64 `json:"nodeIds"`
	}
	if err := s.Call("DOM.querySelectorAll", map[string]any{
		"nodeId":   doc.Root.NodeID,
		"selector": cssSelector,
	}, &qs); err != nil {
		return nil, fmt.Errorf("DOM.querySelectorAll %q: %w", cssSelector, err)
	}
	backendIDs := make([]int64, 0, len(qs.NodeIDs))
	for _, nodeID := range qs.NodeIDs {
		var desc struct {
			Node struct {
				BackendNodeID int64 `json:"backendNodeId"`
			} `json:"node"`
		}
		if err := s.Call("DOM.describeNode", map[string]any{"nodeId": nodeID}, &desc); err != nil {
			return nil, fmt.Errorf("DOM.describeNode: %w", err)
		}
		backendIDs = append(backendIDs, desc.Node.BackendNodeID)
	}
	return backendIDs, nil
}
```

- [x] **Step 4: Run the CDP test to verify it passes**

Run: `cd htrcli && go test ./internal/cdp/... -run TestResolveBackendNodeID -v`
Expected: PASS

- [x] **Step 5: Write the failing ref-store test**

Create `htrcli/internal/commands/refstore_test.go`:

```go
package commands

import (
	"testing"
)

func TestRefStoreAllocAndLookup(t *testing.T) {
	// Redirect the store to a temp file so the test never touches ~/.htrcli.
	dir := t.TempDir()
	refStorePathOverride = dir + "/refs.json"
	defer func() { refStorePathOverride = "" }()

	rs, err := LoadRefStore()
	if err != nil {
		t.Fatalf("LoadRefStore: %v", err)
	}
	refA := rs.Alloc(9007)
	refB := rs.Alloc(9008)
	if refA != "@e1" || refB != "@e2" {
		t.Fatalf("want @e1,@e2 got %s,%s", refA, refB)
	}
	if err := rs.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// A fresh load (new CLI process) still resolves the refs.
	rs2, err := LoadRefStore()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	id, ok := rs2.Lookup("@e1")
	if !ok || id != 9007 {
		t.Fatalf("want 9007,true got %d,%v", id, ok)
	}
	if _, ok := rs2.Lookup("@e999"); ok {
		t.Fatal("unknown ref must not resolve")
	}
	// Alloc continues the sequence across processes (no id reuse).
	if next := rs2.Alloc(9009); next != "@e3" {
		t.Fatalf("want @e3 after reload, got %s", next)
	}
}
```

- [x] **Step 6: Run test to verify it fails**

Run: `cd htrcli && go test ./internal/commands/... -run TestRefStoreAllocAndLookup -v`
Expected: FAIL — `LoadRefStore` / `refStorePathOverride` undefined.

- [x] **Step 7: Implement `refstore.go`**

Create `htrcli/internal/commands/refstore.go`:

```go
package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// refStorePathOverride redirects the ref store file in tests. "" = default
// ~/.htrcli/refs.json.
var refStorePathOverride string

// RefStore persists the CDP transport's @eN -> backendNodeId map. The CLI is
// stateless across invocations, so this file is the only place a CDP ref
// survives between `find --ref` and the command that uses it.
type RefStore struct {
	// NextRef continues across processes so ids are never reused.
	NextRef int              `json:"nextRef"`
	Refs    map[string]int64 `json:"refs"` // "@e7" -> backendNodeId
	path    string
}

func refStorePath() (string, error) {
	if refStorePathOverride != "" {
		return refStorePathOverride, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".htrcli", "refs.json"), nil
}

// LoadRefStore reads the ref store, returning an empty one if the file does
// not exist yet (first `find --ref`).
func LoadRefStore() (*RefStore, error) {
	path, err := refStorePath()
	if err != nil {
		return nil, err
	}
	rs := &RefStore{NextRef: 0, Refs: map[string]int64{}, path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return rs, nil // fresh store
		}
		return nil, fmt.Errorf("reading ref store %s: %w", path, err)
	}
	if err := json.Unmarshal(data, rs); err != nil {
		return nil, fmt.Errorf("parsing ref store %s: %w", path, err)
	}
	if rs.Refs == nil {
		rs.Refs = map[string]int64{}
	}
	rs.path = path
	return rs, nil
}

// Alloc mints the next @eN id for a backendNodeId.
func (rs *RefStore) Alloc(backendNodeID int64) string {
	rs.NextRef++
	refID := fmt.Sprintf("@e%d", rs.NextRef)
	rs.Refs[refID] = backendNodeID
	return refID
}

// Lookup resolves a ref id to its backendNodeId.
func (rs *RefStore) Lookup(refID string) (int64, bool) {
	id, ok := rs.Refs[refID]
	return id, ok
}

// Save writes the store back to disk, creating ~/.htrcli if needed.
func (rs *RefStore) Save() error {
	if err := os.MkdirAll(filepath.Dir(rs.path), 0o755); err != nil {
		return fmt.Errorf("creating ref store dir: %w", err)
	}
	data, err := json.MarshalIndent(rs, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(rs.path, data, 0o600); err != nil {
		return fmt.Errorf("writing ref store %s: %w", rs.path, err)
	}
	return nil
}
```

- [x] **Step 8: Run the ref-store test to verify it passes**

Run: `cd htrcli && go test ./internal/commands/... -run TestRefStoreAllocAndLookup -v`
Expected: PASS

- [x] **Step 9: Replace the Task 5 stub with the real `runFindRefCDP`**

In `htrcli/internal/commands/inspect.go`, DELETE the temporary `runFindRefCDP` stub added in Task 5, and add the real implementation (it needs the `cdp` package, already imported in `cdp_exec.go` but confirm `inspect.go`'s imports — if `inspect.go` does not import `github.com/u007/htrcli/internal/cdp`, place `runFindRefCDP` in `cdp_exec.go` instead, which already imports it). Place it in `cdp_exec.go`:

```go
// runFindRefCDP resolves a CSS selector to backendNodeId(s), mints @eN ref(s)
// in the CLI ref store, and prints them. CDP refs are CSS-only (DOM.querySelector
// is CSS-only); a name=/role=/text= arg is rejected with a clear error.
func runFindRefCDP(selector string, all bool) error {
	if parseSelector(selector).Selector == "" {
		return fmt.Errorf("--ref on --cdp supports CSS selectors only (got %q)", selector)
	}
	s, _, err := cdpSession()
	if err != nil {
		return err
	}
	defer s.Close()

	rs, err := LoadRefStore()
	if err != nil {
		return err
	}

	var refs []string
	if all {
		backendIDs, err := cdp.ResolveRefTargets(s, selector)
		if err != nil {
			return err
		}
		for _, id := range backendIDs {
			refs = append(refs, rs.Alloc(id))
		}
	} else {
		backendID, err := cdp.ResolveBackendNodeID(s, selector)
		if err != nil {
			return err
		}
		refs = append(refs, rs.Alloc(backendID))
	}
	if err := rs.Save(); err != nil {
		return err
	}

	if output.JSONOutput {
		output.PrintJSON(map[string]any{"refs": refs})
		return nil
	}
	for _, r := range refs {
		fmt.Println(r)
	}
	return nil
}
```

- [x] **Step 10: Run the full CDP + commands suites**

Run: `cd htrcli && go test ./internal/cdp/... ./internal/commands/...`
Expected: PASS (all existing + new)

- [x] **Step 11: Commit**

```bash
git add htrcli/internal/cdp/elementref.go htrcli/internal/cdp/elementref_test.go htrcli/internal/commands/refstore.go htrcli/internal/commands/refstore_test.go htrcli/internal/commands/inspect.go htrcli/internal/commands/cdp_exec.go
git commit -m "feat(htrcli): CDP element refs via backendNodeId + persistent ref store"
```

---

### Task 7: `upload` command — Go CDP transport

The spec's headline deliverable: the first real `DOM.setFileInputFiles` on the Go side. Supports a fresh CSS selector (resolve → backendNodeId) or a `@eN` ref (backendNodeId from the ref store). No OS file-picker appears.

**Files:**
- Create: `htrcli/internal/cdp/upload.go`
- Create: `htrcli/internal/commands/upload.go`
- Test: `htrcli/internal/cdp/upload_test.go`

**Interfaces:**
- Consumes: `cdp.ResolveBackendNodeID` (Task 6), `LoadRefStore`/`Lookup` (Task 6), `cdpSession()` (existing).
- Produces: `cdp.SetFileInputFiles(s *Session, backendNodeID int64, files []string) error`, `commands.uploadCmd`.

- [x] **Step 1: Write the failing CDP test**

Create `htrcli/internal/cdp/upload_test.go`:

```go
package cdp

import (
	"encoding/json"
	"testing"

	"github.com/gorilla/websocket"
)

func TestSetFileInputFiles(t *testing.T) {
	var gotBackend int64
	var gotFiles []string
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		if m.Method == "DOM.setFileInputFiles" {
			var p struct {
				BackendNodeID int64    `json:"backendNodeId"`
				Files         []string `json:"files"`
			}
			json.Unmarshal(m.Params, &p)
			gotBackend = p.BackendNodeID
			gotFiles = p.Files
		}
		conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
	})
	s, _ := Dial(url)
	defer s.Close()

	if err := SetFileInputFiles(s, 9007, []string{"/tmp/a.png", "/tmp/b.png"}); err != nil {
		t.Fatalf("SetFileInputFiles: %v", err)
	}
	if gotBackend != 9007 {
		t.Fatalf("want backendNodeId 9007, got %d", gotBackend)
	}
	if len(gotFiles) != 2 || gotFiles[0] != "/tmp/a.png" {
		t.Fatalf("unexpected files: %v", gotFiles)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `cd htrcli && go test ./internal/cdp/... -run TestSetFileInputFiles -v`
Expected: FAIL — `SetFileInputFiles` undefined.

- [x] **Step 3: Implement `upload.go` (CDP)**

Create `htrcli/internal/cdp/upload.go`:

```go
package cdp

import "fmt"

// SetFileInputFiles sets the files on a file <input> identified by
// backendNodeId, via CDP DOM.setFileInputFiles. The file paths are on the
// same host as the browser (htrcli and Chrome are both local), so no upload
// dialog appears. DOM.enable must already have been called by the resolver.
func SetFileInputFiles(s *Session, backendNodeID int64, files []string) error {
	if err := s.Call("DOM.setFileInputFiles", map[string]any{
		"backendNodeId": backendNodeID,
		"files":         files,
	}, nil); err != nil {
		return fmt.Errorf("DOM.setFileInputFiles: %w", err)
	}
	return nil
}
```

- [x] **Step 4: Run the CDP test to verify it passes**

Run: `cd htrcli && go test ./internal/cdp/... -run TestSetFileInputFiles -v`
Expected: PASS

- [x] **Step 5: Implement the `upload` command (CDP branch)**

Create `htrcli/internal/commands/upload.go`:

```go
package commands

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/u007/htrcli/internal/cdp"
	"github.com/u007/htrcli/internal/output"
)

// parseUploadFiles splits the comma-separated file arg and verifies each path
// exists locally (setFileInputFiles fails opaquely on a missing path, so we
// fail early and clearly). Returns absolute-ish paths as given.
func parseUploadFiles(arg string) ([]string, error) {
	parts := strings.Split(arg, ",")
	files := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err != nil {
			return nil, fmt.Errorf("file not found: %s", p)
		}
		files = append(files, p)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no files given")
	}
	return files, nil
}

var uploadCmd = &cobra.Command{
	Use:   "upload <selector|@ref> <file[,file...]>",
	Short: "Set files on a file input without an OS file-picker",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		files, err := parseUploadFiles(args[1])
		if err != nil {
			return err
		}
		if UseCDP() {
			return runUploadCDP(args[0], files)
		}
		return runUploadExt(args[0], files)
	},
}

// runUploadCDP resolves the target to a backendNodeId (fresh CSS selector or
// @eN ref from the store) and calls DOM.setFileInputFiles.
func runUploadCDP(target string, files []string) error {
	s, _, err := cdpSession()
	if err != nil {
		return err
	}
	defer s.Close()

	var backendID int64
	sel := parseSelector(target)
	if sel.Ref != "" {
		rs, err := LoadRefStore()
		if err != nil {
			return err
		}
		id, ok := rs.Lookup(sel.Ref)
		if !ok {
			return fmt.Errorf("stale ref: %s is not in the ref store (mint it with `htrcli find --ref --cdp`)", sel.Ref)
		}
		backendID = id
		// DOM.enable is required before setFileInputFiles addresses a
		// backendNodeId; ResolveBackendNodeID would call it, but the ref path
		// skips resolution, so enable explicitly. A backendNodeId that no
		// longer resolves (page navigated) makes setFileInputFiles fail — the
		// explicit stale signal for the CDP transport.
		if err := s.Call("DOM.enable", nil, nil); err != nil {
			return fmt.Errorf("DOM.enable: %w", err)
		}
	} else {
		if sel.Selector == "" {
			return fmt.Errorf("upload on --cdp supports CSS selectors or @refs only (got %q)", target)
		}
		backendID, err = cdp.ResolveBackendNodeID(s, sel.Selector)
		if err != nil {
			return err
		}
	}

	if err := cdp.SetFileInputFiles(s, backendID, files); err != nil {
		return err
	}
	if output.JSONOutput {
		output.PrintJSON(map[string]any{"success": true, "files": files})
		return nil
	}
	fmt.Printf("Uploaded %d file(s) to %s (cdp)\n", len(files), target)
	return nil
}

func init() {
	rootCmd.AddCommand(uploadCmd)
}
```

Note: `runUploadExt` is added in Task 8. To keep this task compiling on its own, add a **temporary stub** at the bottom of `upload.go` that Task 8 replaces:

```go
// runUploadExt is implemented in Task 8. Temporary stub; DELETE when Task 8
// adds the real extension-transport upload.
func runUploadExt(target string, files []string) error {
	return fmt.Errorf("upload on the extension transport is added in a later task; use --cdp for now")
}
```

- [x] **Step 6: Run the CDP + commands suites**

Run: `cd htrcli && go test ./internal/cdp/... ./internal/commands/...`
Expected: PASS

- [x] **Step 7: Commit**

```bash
git add htrcli/internal/cdp/upload.go htrcli/internal/cdp/upload_test.go htrcli/internal/commands/upload.go
git commit -m "feat(htrcli): add upload command (CDP transport, DOM.setFileInputFiles)"
```

---

### Task 8: `upload` — extension transport (Chrome) + Firefox unsupported

The default transport is `ext`, so upload must work there too. The extension's background attaches `chrome.debugger`, resolves the CSS selector to a debugger DOM node, and calls `DOM.setFileInputFiles` — the same primitive, host-local paths. `@eN` refs are `--cdp`-only (the in-page JS registry element is not addressable as a debugger DOM node), and Firefox (no `chrome.debugger`) fails with an explicit unsupported error.

**Files:**
- Modify: `src/types/commands.ts` (add `uploadFiles` action)
- Modify: `src/background/index.ts` (handle a new `UPLOAD_FILES` runtime message via `chrome.debugger`)
- Modify: `src/background/nativeHost.ts` (route the `uploadFiles` command to the background handler)
- Modify: `htrcli/internal/commands/upload.go` (replace the Task 7 `runUploadExt` stub)
- Test: `src/background/uploadFiles.test.ts` (create)

**Interfaces:**
- Consumes: `api.Command` with `Action: "uploadFiles"`, `Options{"files": []string}`.
- Produces: extension-side `resolveAndSetFiles(sendCommand, selector, files)` (test seam), `runUploadExt` sending the command over the daemon.

- [x] **Step 1: Write the failing extension test**

Create `src/background/uploadFiles.test.ts`. The CDP `send` is injected so no real debugger is needed:

```typescript
import { describe, expect, it } from "bun:test";
import { resolveAndSetFiles } from "./uploadFiles";

describe("resolveAndSetFiles", () => {
	it("resolves the selector via DOM.* and sets files by nodeId", async () => {
		const calls: { method: string; params: Record<string, unknown> }[] = [];
		const send = async (method: string, params: Record<string, unknown>) => {
			calls.push({ method, params });
			switch (method) {
				case "DOM.getDocument":
					return { root: { nodeId: 1 } };
				case "DOM.querySelector":
					return { nodeId: 42 };
				default:
					return {};
			}
		};

		await resolveAndSetFiles(send, "#file", ["/tmp/a.png"]);

		const setCall = calls.find((c) => c.method === "DOM.setFileInputFiles");
		expect(setCall).toBeDefined();
		expect(setCall?.params.nodeId).toBe(42);
		expect(setCall?.params.files).toEqual(["/tmp/a.png"]);
	});

	it("throws when the selector matches nothing (nodeId 0)", async () => {
		const send = async (method: string) => {
			if (method === "DOM.getDocument") return { root: { nodeId: 1 } };
			if (method === "DOM.querySelector") return { nodeId: 0 };
			return {};
		};
		await expect(resolveAndSetFiles(send, "#missing", ["/tmp/a.png"])).rejects.toThrow(
			/no element matched/,
		);
	});
});
```

- [x] **Step 2: Run test to verify it fails**

Run: `bun test src/background/uploadFiles.test.ts`
Expected: FAIL — `./uploadFiles` module does not exist.

- [x] **Step 3: Implement `uploadFiles.ts`**

Create `src/background/uploadFiles.ts`:

```typescript
/**
 * File upload for the extension transport (Chrome only).
 *
 * The background owns the `chrome.debugger` connection. It resolves a CSS
 * selector to a debugger DOM nodeId (DOM.getDocument -> DOM.querySelector) and
 * calls DOM.setFileInputFiles with host-local paths — no OS file-picker.
 * Firefox has no chrome.debugger, so callers report the unsupported case.
 */

export type CdpSend = (
	method: string,
	params: Record<string, unknown>,
) => Promise<unknown>;

/**
 * Resolve `selector` to a file input and set its files. Throws with a clear
 * message if the selector matches nothing. The `send` seam is injected so this
 * is unit-testable without a real debugger.
 */
export async function resolveAndSetFiles(
	send: CdpSend,
	selector: string,
	files: string[],
): Promise<void> {
	await send("DOM.enable", {});
	const doc = (await send("DOM.getDocument", { depth: 0 })) as {
		root: { nodeId: number };
	};
	const qs = (await send("DOM.querySelector", {
		nodeId: doc.root.nodeId,
		selector,
	})) as { nodeId: number };
	if (!qs.nodeId) {
		throw new Error(`no element matched CSS selector "${selector}"`);
	}
	await send("DOM.setFileInputFiles", { nodeId: qs.nodeId, files });
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `bun test src/background/uploadFiles.test.ts`
Expected: PASS

- [x] **Step 5: Add the `uploadFiles` action + background wiring**

In `src/types/commands.ts`, add to the `CommandAction` union (near the Script Execution group, after `"printToPDF"`):

```typescript
	| "uploadFiles"
```

In `src/background/nativeHost.ts`, add a background-handled branch at the top of `sendCommandToTab` (near the other early-return action branches around line 1012, before the `fetchInPage` check):

```typescript
	if (payload.action === "uploadFiles") {
		await handleUploadFiles(tabId, payload);
		return;
	}
```

Add the `handleUploadFiles` function in `nativeHost.ts` (near `handleDebuggerEval`), importing `resolveAndSetFiles` at the top of the file:

```typescript
async function handleUploadFiles(tabId: number, payload: Command): Promise<void> {
	if (typeof chrome.debugger === "undefined") {
		replyError(
			tabId,
			payload.id,
			"upload is not supported on Firefox (chrome.debugger API unavailable). Use the --cdp transport against Chrome instead.",
		);
		return;
	}
	const selector = payload.target?.selector;
	const files = (payload.options?.files as string[]) ?? [];
	if (payload.target?.ref) {
		replyError(
			tabId,
			payload.id,
			"upload by @ref is only supported on the --cdp transport; pass a CSS selector on the extension transport",
		);
		return;
	}
	if (!selector) {
		replyError(tabId, payload.id, "upload requires a CSS selector target");
		return;
	}
	const target = { tabId };
	try {
		await chrome.debugger.attach(target, "1.3");
		try {
			await resolveAndSetFiles(
				(method, params) => chrome.debugger.sendCommand(target, method, params),
				selector,
				files,
			);
			sendToNative({
				type: "command_result",
				tabId,
				payload: { id: payload.id, success: true, data: { files } } as CommandResult,
			});
		} finally {
			await chrome.debugger.detach(target);
		}
	} catch (error) {
		console.error("[HTR NControl] uploadFiles error:", error);
		replyError(
			tabId,
			payload.id,
			error instanceof Error ? error.message : String(error),
		);
	}
}
```

Confirm `replyError`, `sendToNative`, and the `Command`/`CommandResult` imports already exist in `nativeHost.ts` (they are used by the sibling handlers) and add the `resolveAndSetFiles` import from `./uploadFiles`.

- [x] **Step 6: Replace the Task 7 `runUploadExt` stub with the real command send**

In `htrcli/internal/commands/upload.go`, DELETE the temporary `runUploadExt` stub and replace it with:

```go
// runUploadExt sends an uploadFiles command to the extension. Chrome resolves
// the selector via chrome.debugger DOM.setFileInputFiles; Firefox replies with
// an explicit unsupported error (surfaced here as a non-zero exit).
func runUploadExt(target string, files []string) error {
	sel := parseSelector(target)
	if sel.Ref != "" {
		return fmt.Errorf("upload by @ref is only supported on --cdp; pass a CSS selector on the extension transport")
	}
	if sel.Selector == "" {
		return fmt.Errorf("upload on the extension transport supports CSS selectors only (got %q)", target)
	}
	c := GetClient()
	tabID, err := GetTabID()
	if err != nil {
		return err
	}
	// Build []any so the JSON body carries a plain string array under files.
	fileList := make([]any, len(files))
	for i, f := range files {
		fileList[i] = f
	}
	result, err := c.ExecuteCommand(tabID, api.Command{
		ID:      "1",
		Action:  "uploadFiles",
		Target:  sel,
		Options: map[string]any{"files": fileList},
	})
	if err != nil {
		return err
	}
	if err := commandError(result); err != nil {
		return err
	}
	if output.JSONOutput {
		output.PrintJSON(result)
		return nil
	}
	fmt.Printf("Uploaded %d file(s) to %s\n", len(files), target)
	return nil
}
```

Add the `api` import to `upload.go`'s import block (`github.com/u007/htrcli/internal/api`).

- [x] **Step 7: Run everything**

Run: `cd htrcli && go test ./...`
Expected: PASS
Run: `bun test src/background/uploadFiles.test.ts && bun run typecheck && bun run check:fix`
Expected: PASS, no type errors, no lint errors

- [x] **Step 8: Commit**

```bash
git add src/types/commands.ts src/background/index.ts src/background/nativeHost.ts src/background/uploadFiles.ts src/background/uploadFiles.test.ts htrcli/internal/commands/upload.go
git commit -m "feat(htrcli): upload on extension transport (Chrome), unsupported on Firefox"
```

---

### Task 9: End-to-end verification & self-review

**Files:** none (verification only)

- [ ] **Step 1: findAll end-to-end**

With `htrcli serve` running and the extension loaded in Chrome, on a page with several buttons run `htrcli findAll button --json` and confirm an array of element infos is printed.

- [ ] **Step 2: Extension refs round-trip**

Run `htrcli find "#some-button" --ref` → confirm it prints `@e1`. Then `htrcli click @e1` → confirm the button is clicked (the ref resolved). Navigate the page (full reload), then `htrcli click @e1` again → confirm an explicit `stale ref` error, not a wrong-element click or silent success.

- [ ] **Step 3: CDP refs round-trip**

With Chrome started for CDP (`--cdp`, port 9222), run `htrcli --cdp find "#some-button" --ref` → prints `@e1`, and `~/.htrcli/refs.json` now contains it. Then `htrcli --cdp upload "#file-input" ./a.png` and `htrcli --cdp upload @e2 ./a.png` (after minting `@e2` on the file input) → confirm the input's files are set (`htrcli --cdp eval "document.querySelector('#file-input').files.length"` returns 1). Navigate the page, then reuse the ref → confirm `DOM.setFileInputFiles` fails (stale), surfaced as a non-zero exit.

- [ ] **Step 4: Upload on the extension transport (Chrome) + Firefox**

On Chrome (default transport): `htrcli upload "#file-input" ./a.png,./b.png` → confirm two files set, no OS picker. On Firefox (extension loaded via `about:debugging`): `htrcli upload "#file-input" ./a.png` → confirm it exits non-zero with the explicit "not supported on Firefox" message (never a silent no-op).

- [x] **Step 5: Self-review against the spec**

Re-read spec §4 (refs + findAll) and §5 (upload). Confirm:
  - findAll subcommand mirrors find's flags/output (Task 1). ✓
  - Extension refs: in-page registry, invalidated on navigation, explicit stale error, never silent re-resolve (Tasks 3–4). ✓
  - CDP refs: backendNodeId via `DOM.enable`/`DOM.describeNode`, not a bespoke registry (Task 6). ✓
  - interact selector parsing gains an `@e`-prefix branch (Task 2). ✓
  - Upload: Chrome via `DOM.setFileInputFiles` given a backendNodeId or fresh selector resolve; Firefox explicit unsupported error, no false parity (Tasks 7–8). ✓
  - Placeholder scan: grep the new files for `TODO`/`TBD`/`FIXME`/`placeholder` — expect none except the two clearly-labelled cross-task stubs, which must be **deleted** by Tasks 6 and 8 respectively. Verify neither stub survives: `grep -rn "Temporary stub" htrcli/internal/commands/` returns nothing.
  - Type consistency: `assignRef` option key is spelled identically in `inspect.go`, `commandExecutor.ts`; ref id format `@e<N>` matches between `refRegistry.ts` (`@e${nextRef}`) and `refstore.go` (`@e%d`).

- [x] **Step 6: Final full-suite run**

Run: `cd htrcli && go test ./... && cd .. && bun test && bun run typecheck`
Expected: PASS across Go and extension suites.
