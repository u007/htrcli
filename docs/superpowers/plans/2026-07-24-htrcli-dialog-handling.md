# htrcli Dialog Handling Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let htrcli auto-answer JavaScript dialogs (`alert`/`confirm`/`prompt`) according to a policy armed ahead of time, and expose a readable log of what was answered — `htrcli dialog handle --action accept|dismiss|respond --text "..."` and `htrcli dialog list [--since N]`.

**Architecture:** A dialog handler must be armed **before** the action that triggers the dialog, so `dialog handle` sets a standing (time-bounded) policy for the session; the policy applies to whatever dialog fires next. On Chrome the policy is enforced via CDP: `chrome.debugger` attaches (through the shared refcounted manager), `Page.enable` turns on `Page.javascriptDialogOpening`, and each opening is immediately answered with `Page.handleJavaScriptDialog` per the policy and logged as a `DialogEntry` in the shared event buffer (`kind: "dialog"`). On Firefox (no `chrome.debugger`) the fallback is a MAIN-world override of `window.alert/confirm/prompt` injected at `document_start`, mirroring the existing `consoleCapture.ts` relay/ready-handshake architecture; the policy is pushed down to the override and answered dialogs are relayed up into the same buffer. Both paths auto-release: the CDP attach detaches after a self-held timer, never permanently.

**Tech Stack:** Go (cobra CLI, stdlib `net/http`), TypeScript (Chrome `chrome.debugger`/CDP `Page` domain, Firefox MAIN-world content script + `window.postMessage` relay, `chrome.storage.session`), Bun test runner, Go's `testing` package.

## Global Constraints

- Package manager: `bun` only for the extension — never npm/yarn.
- Biome lint/format (tabs, double quotes) — run `bun run check:fix` before committing TS changes.
- Go tests: `go test ./...` from `htrcli/`.
- Async `chrome.runtime.onMessage` listeners must `return true` when responding asynchronously.
- Extension console/error logging prefix: `console.error/warn('[HTR NControl] ...')`.
- Event buffer count cap: 500 entries per (tab, kind) — handled by the shared `EventStore`/`eventStore.ts`; unchanged here.
- **Debugger-attach lifecycle (verbatim project constraint):** never hold a permanent `chrome.debugger` attach. The CDP dialog path attaches only for the duration of an explicitly-armed, time-bounded policy window and auto-detaches when its timer fires, the tab navigates, or the tab closes. The `dialogPolicy` arm command **acks immediately** (like the existing `getReadyTabs` action in `sendCommandToTab`) and the background holds its own detach timer independent of the daemon's 30s command timeout.
- **DEPENDENCY on the network-capture plan (`2026-07-24-htrcli-network-capture.md`):** this plan reuses two pieces that plan introduces and owns:
  1. The event-buffer generalization in its **Task 1** — `EventKind`, `recordEvent(tabId, kind, data)` in `src/background/eventStore.ts`. This plan's Task 1 widens `EventKind`/`BufferedEventData` to add `"dialog"` and calls `recordEvent`.
  2. The shared refcounted debugger manager in its **Task 2** — `src/background/debuggerManager.ts` (`attachShared`/`detachShared`/`onDebuggerEvent`/`sendToTab`). This plan's Task 3 reuses it.
  Execute the network plan's Tasks 1–2 first (both are Phase 2 of the spec build order, alongside this one). If dialog handling is executed standalone, apply the network plan's Task 1 generalization and Task 2 manager first.
- **Firefox documented limitations (must be surfaced, never silent):** the MAIN-world override is racy against dialogs a page fires before the override lands, and it cannot intercept true native dialogs like `beforeunload`. Record these in TODO.md (Task 6). (The Firefox 128+ `world: "MAIN"` requirement is already the extension's hard floor — `firefox/vite.config.ts` sets `browser_specific_settings.gecko.strict_min_version` to `128.0` for the console-capture MAIN-world script — so the dialog override injects on every supported Firefox; no per-feature version caveat is needed.)
- **Firefox build wiring (must be explicit, not assumed out of scope):** Firefox content scripts execute as classic scripts, not ES modules, so a MAIN-world script cannot ship as the main build's code-split module (top-level `import` fails to load). Each MAIN-world script needs the established three-piece pattern already used for `consoleCapture.ts`: (1) a `firefox/src/<name>-entry.ts` shim, (2) a dedicated IIFE build-pass config `firefox/vite.<name>.config.ts` that overwrites `firefox/build/<name>.js`, (3) a `firefox:build` script step, plus a rollup input in `firefox/vite.config.ts` and a `world: "MAIN"` `content_scripts` entry in that file's emitted manifest. Task 4 does all of this for `dialogOverride.ts`.
- No new external runtime dependencies.

---

### Task 1: `DialogEntry` type + `recordDialogEntry`

**Files:**
- Modify: `src/types/recording.ts`
- Modify: `src/background/eventStore.ts`
- Test: `src/background/eventStore.test.ts`

**Interfaces:**
- Consumes: `recordEvent(tabId, kind, data)` and the `EventKind`/`BufferedEventData` types from the network plan's Task 1.
- Produces: `DialogEntry` interface in `recording.ts`; `EventKind` widened to include `"dialog"`; `recordDialogEntry(tabId: number, entry: DialogEntry): Promise<void>`.

- [ ] **Step 1: Write the failing test**

Append to `src/background/eventStore.test.ts` (inside the existing `describe`, next to the console/network `it` blocks):

```typescript
	it("records dialog entries in their own bucket", async () => {
		await recordDialogEntry(1, {
			dialogType: "confirm",
			message: "Delete this item?",
			resolvedAction: "accept",
		});

		const posted: { kind: string; entries: { data: { message: string } }[] }[] = [];
		await flushPending(async (_tabId, kind, entries) => {
			posted.push({ kind, entries: entries as { data: { message: string } }[] });
			return true;
		});

		const dialogBatch = posted.find((p) => p.kind === "dialog");
		expect(dialogBatch).toBeDefined();
		expect(dialogBatch?.entries[0].data.message).toBe("Delete this item?");
	});
```

Add `recordDialogEntry` to the existing top-of-file import from `./eventStore`.

- [ ] **Step 2: Run test to verify it fails**

Run: `bun test src/background/eventStore.test.ts`
Expected: FAIL — `recordDialogEntry` is not exported from `./eventStore`.

- [ ] **Step 3: Add the `DialogEntry` type**

In `src/types/recording.ts`, after the `NetworkEntry` interface (added by the network plan's Task 1), add:

```typescript
// Structured dialog payload captured via CDP (Chrome) or window.* override (Firefox).
export interface DialogEntry {
	dialogType: "alert" | "confirm" | "prompt" | "beforeunload";
	message: string;
	resolvedAction: "accept" | "dismiss";
	respondedText?: string;
}
```

- [ ] **Step 4: Widen `eventStore.ts` for the dialog kind**

In `src/background/eventStore.ts`, update the kind/data types added by the network plan's Task 1. Change:

```typescript
import type { ConsoleEntry, NetworkEntry } from "../types/recording";

export type ConsoleEntryData = ConsoleEntry;
export type NetworkEntryData = NetworkEntry;
export type BufferedEventData = ConsoleEntryData | NetworkEntryData;

export type EventKind = "console" | "network";
```

to:

```typescript
import type { ConsoleEntry, DialogEntry, NetworkEntry } from "../types/recording";

export type ConsoleEntryData = ConsoleEntry;
export type NetworkEntryData = NetworkEntry;
export type DialogEntryData = DialogEntry;
export type BufferedEventData = ConsoleEntryData | NetworkEntryData | DialogEntryData;

export type EventKind = "console" | "network" | "dialog";
```

Then add a `recordDialogEntry` wrapper next to `recordNetworkEntry`:

```typescript
// Record a dialog entry in durable session storage.
export async function recordDialogEntry(
	tabId: number,
	entry: DialogEntryData,
): Promise<void> {
	await recordEvent(tabId, "dialog", entry);
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `bun test src/background/eventStore.test.ts`
Expected: PASS

- [ ] **Step 6: Biome + typecheck**

Run: `bun run check:fix && bun run typecheck`
Expected: no errors

- [ ] **Step 7: Commit**

```bash
git add src/types/recording.ts src/background/eventStore.ts src/background/eventStore.test.ts
git commit -m "feat(extension): add dialog entries to the event buffer"
```

---

### Task 2: Pure dialog-policy resolver

**Files:**
- Create: `src/background/dialogPolicy.ts`
- Test: `src/background/dialogPolicy.test.ts`

**Interfaces:**
- Consumes: `DialogEntry` (Task 1).
- Produces: `type DialogAction = "accept" | "dismiss" | "respond"`; `interface DialogPolicy { action: DialogAction; text?: string }`; `resolveDialog(policy: DialogPolicy, dialogType: DialogEntry["dialogType"], message: string): { accept: boolean; promptText?: string; entry: DialogEntry }`. Shared by both the Chrome CDP path (Task 3) and the Firefox override (Task 4), so the decision logic lives in exactly one tested place.

- [ ] **Step 1: Write the failing test**

Create `src/background/dialogPolicy.test.ts`:

```typescript
import { describe, expect, it } from "bun:test";
import { resolveDialog } from "./dialogPolicy";

describe("resolveDialog", () => {
	it("accepts when policy is accept", () => {
		const r = resolveDialog({ action: "accept" }, "confirm", "Sure?");
		expect(r.accept).toBe(true);
		expect(r.promptText).toBeUndefined();
		expect(r.entry).toEqual({
			dialogType: "confirm",
			message: "Sure?",
			resolvedAction: "accept",
		});
	});

	it("dismisses when policy is dismiss", () => {
		const r = resolveDialog({ action: "dismiss" }, "confirm", "Sure?");
		expect(r.accept).toBe(false);
		expect(r.entry.resolvedAction).toBe("dismiss");
	});

	it("responds with text for a prompt", () => {
		const r = resolveDialog({ action: "respond", text: "hello" }, "prompt", "Name?");
		expect(r.accept).toBe(true);
		expect(r.promptText).toBe("hello");
		expect(r.entry).toEqual({
			dialogType: "prompt",
			message: "Name?",
			resolvedAction: "accept",
			respondedText: "hello",
		});
	});

	it("treats respond with no text as an empty string response", () => {
		const r = resolveDialog({ action: "respond" }, "prompt", "Name?");
		expect(r.accept).toBe(true);
		expect(r.promptText).toBe("");
		expect(r.entry.respondedText).toBe("");
	});
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bun test src/background/dialogPolicy.test.ts`
Expected: FAIL — `./dialogPolicy` module does not exist.

- [ ] **Step 3: Implement `dialogPolicy.ts`**

Create `src/background/dialogPolicy.ts`:

```typescript
/**
 * Pure decision logic for answering a JavaScript dialog per an armed policy.
 * Shared by the Chrome CDP path (Page.handleJavaScriptDialog) and the Firefox
 * MAIN-world override, so "accept/dismiss/respond" is defined in one place.
 */

import type { DialogEntry } from "../types/recording";

export type DialogAction = "accept" | "dismiss" | "respond";

export interface DialogPolicy {
	action: DialogAction;
	text?: string;
}

export function resolveDialog(
	policy: DialogPolicy,
	dialogType: DialogEntry["dialogType"],
	message: string,
): { accept: boolean; promptText?: string; entry: DialogEntry } {
	const accept = policy.action === "accept" || policy.action === "respond";
	const entry: DialogEntry = {
		dialogType,
		message,
		resolvedAction: accept ? "accept" : "dismiss",
	};
	if (policy.action === "respond") {
		const text = policy.text ?? "";
		entry.respondedText = text;
		return { accept, promptText: text, entry };
	}
	return { accept, entry };
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `bun test src/background/dialogPolicy.test.ts`
Expected: PASS

- [ ] **Step 5: Biome + typecheck**

Run: `bun run check:fix && bun run typecheck`
Expected: no errors

- [ ] **Step 6: Commit**

```bash
git add src/background/dialogPolicy.ts src/background/dialogPolicy.test.ts
git commit -m "feat(extension): add pure dialog-policy resolver"
```

---

### Task 3: Chrome CDP dialog arm handler

**Files:**
- Modify: `src/background/nativeHost.ts`
- Test: none (integration against `chrome.debugger`; the decision logic is unit-tested in Task 2 and verified end-to-end in Task 6)

**Interfaces:**
- Consumes: `attachShared`, `detachShared`, `onDebuggerEvent`, `sendToTab` (network plan Task 2); `resolveDialog`, `DialogPolicy` (Task 2); `recordDialogEntry` (Task 1).
- Produces: a `dialogPolicy` command action handled in `sendCommandToTab` that arms a bounded Chrome dialog-handling window. `options` are `{ action: "accept"|"dismiss"|"respond", text?: string, durationMs?: number }` (default duration 30000, clamped to [1000, 300000] — dialogs are armed ahead of a user action, so the default window is longer than network capture's).

- [ ] **Step 1: Add imports and module state**

At the top of `src/background/nativeHost.ts`, extend the debugger-manager import (added by the network plan) and add the dialog imports:

```typescript
import { resolveDialog, type DialogPolicy } from "./dialogPolicy";
import { recordDialogEntry } from "./eventStore";
```

(`attachShared`/`detachShared`/`onDebuggerEvent`/`sendToTab` are already imported from `./debuggerManager` by the network plan's Task 4; if executing dialog standalone, add that import too.)

Near the network-capture module state, add the dialog-capture registry:

```typescript
interface DialogCaptureHandle {
	unsubscribe: () => void;
	timer: ReturnType<typeof setTimeout>;
}

const activeDialogCaptures = new Map<number, DialogCaptureHandle>();

function clampDialogDuration(ms: number | undefined): number {
	const v = typeof ms === "number" && Number.isFinite(ms) ? ms : 30000;
	return Math.min(300000, Math.max(1000, v));
}
```

- [ ] **Step 2: Add the arm handler**

Add this function in `src/background/nativeHost.ts` (near `handleNetworkCapture` from the network plan, or near `handleDebuggerEval` if executing standalone):

```typescript
/**
 * Arm a bounded Chrome dialog-handling window on a tab. Attaches the debugger
 * via the shared refcounted manager, enables the CDP Page domain, and answers
 * each Page.javascriptDialogOpening per the armed policy, logging a DialogEntry.
 * Auto-detaches after a self-held timer. Never a permanent attach. Firefox (no
 * chrome.debugger) is handled by the caller: the policy is forwarded to the
 * MAIN-world override in the content script instead.
 */
async function handleDialogPolicyChrome(
	tabId: number,
	payload: Command,
	policy: DialogPolicy,
): Promise<void> {
	const durationMs = clampDialogDuration(
		payload.options?.durationMs as number | undefined,
	);

	// Re-arming: replace the policy + extend the window rather than double-attach.
	const existing = activeDialogCaptures.get(tabId);
	if (existing) {
		existing.unsubscribe();
		clearTimeout(existing.timer);
		activeDialogCaptures.delete(tabId);
	}

	try {
		await attachShared(tabId);
		await sendToTab(tabId, "Page.enable", {});
	} catch (err) {
		await detachShared(tabId).catch(() => {});
		replyError(
			tabId,
			payload.id,
			`failed to arm dialog handling: ${err instanceof Error ? err.message : String(err)}`,
		);
		return;
	}

	const unsubscribe = onDebuggerEvent((source, method, params) => {
		if (source.tabId !== tabId) return;
		if (method !== "Page.javascriptDialogOpening") return;
		const p = params as { type: string; message: string };
		const dialogType = (["alert", "confirm", "prompt", "beforeunload"].includes(p.type)
			? p.type
			: "alert") as "alert" | "confirm" | "prompt" | "beforeunload";
		const { accept, promptText, entry } = resolveDialog(policy, dialogType, p.message);
		void sendToTab(tabId, "Page.handleJavaScriptDialog", {
			accept,
			...(promptText !== undefined ? { promptText } : {}),
		}).catch((err) => {
			console.warn("[HTR NControl] handleJavaScriptDialog failed:", err);
		});
		void recordDialogEntry(tabId, entry);
	});

	const timer = setTimeout(() => void stopDialogCapture(tabId), durationMs);
	activeDialogCaptures.set(tabId, { unsubscribe, timer });

	sendToNative({
		type: "command_result",
		tabId,
		payload: {
			id: payload.id,
			success: true,
			data: { armed: true, durationMs, transport: "cdp" },
		} as CommandResult,
	});
}

// Tear down a dialog-handling window. Idempotent.
async function stopDialogCapture(tabId: number): Promise<void> {
	const handle = activeDialogCaptures.get(tabId);
	if (!handle) return;
	activeDialogCaptures.delete(tabId);
	clearTimeout(handle.timer);
	handle.unsubscribe();
	try {
		await sendToTab(tabId, "Page.disable", {});
	} catch (err) {
		console.warn("[HTR NControl] Page.disable failed:", err);
	}
	await detachShared(tabId);
}

/**
 * Entry point for the dialogPolicy command. Parses the policy, then dispatches
 * to the CDP path (Chrome) or forwards the policy to the content-script MAIN-
 * world override (Firefox).
 */
async function handleDialogPolicy(tabId: number, payload: Command): Promise<void> {
	const action = (payload.options?.action as string | undefined) ?? "accept";
	if (action !== "accept" && action !== "dismiss" && action !== "respond") {
		replyError(tabId, payload.id, `invalid dialog action: ${action}`);
		return;
	}
	const policy: DialogPolicy = {
		action,
		text: payload.options?.text as string | undefined,
	};

	if (typeof chrome.debugger === "undefined") {
		// Firefox: forward the policy to the content script, which relays it to
		// the MAIN-world override. Ack after the content script acknowledges.
		chrome.tabs.sendMessage(
			tabId,
			{ type: "DIALOG_POLICY", policy },
			() => {
				if (chrome.runtime.lastError) {
					replyError(
						tabId,
						payload.id,
						`content script not ready to arm dialog policy: ${chrome.runtime.lastError.message}`,
					);
					return;
				}
				sendToNative({
					type: "command_result",
					tabId,
					payload: {
						id: payload.id,
						success: true,
						data: { armed: true, transport: "override" },
					} as CommandResult,
				});
			},
		);
		return;
	}

	await handleDialogPolicyChrome(tabId, payload, policy);
}
```

- [ ] **Step 3: Route the action in `sendCommandToTab` and tear down on tab close**

In `sendCommandToTab` (line ~1004), add a branch alongside the other background-handled early returns:

```typescript
	if (payload.action === "dialogPolicy") {
		await handleDialogPolicy(tabId, payload);
		return;
	}
```

In the `chrome.tabs.onRemoved` listener (added by the network plan's Task 4; if executing standalone, add one), also tear down dialog capture:

```typescript
	if (activeDialogCaptures.has(tabId)) {
		void stopDialogCapture(tabId);
	}
```

- [ ] **Step 4: Typecheck + Biome + full test suite**

Run: `bun run check:fix && bun run typecheck && bun run test`
Expected: no errors, all tests pass

- [ ] **Step 5: Commit**

```bash
git add src/background/nativeHost.ts
git commit -m "feat(extension): arm bounded CDP dialog handling"
```

---

### Task 4: Firefox MAIN-world dialog override + relay

**Files:**
- Create: `src/contentScript/dialogOverride.ts`
- Create: `firefox/src/dialogOverride-entry.ts`
- Create: `firefox/vite.dialogOverride.config.ts`
- Modify: `src/contentScript/index.ts`
- Modify: `src/types/recording.ts`
- Modify: `src/background/index.ts`
- Modify: `src/manifest.ts` (Chrome MAIN-world entry)
- Modify: `firefox/vite.config.ts` (Firefox rollup input + MAIN-world manifest entry)
- Modify: `package.json` (`firefox:build` build-pass step)

**Interfaces:**
- Consumes: `resolveDialog` (Task 2), `DialogEntry`/`DialogPolicy` types.
- Produces: a MAIN-world script overriding `window.alert/confirm/prompt`, a `DIALOG_POLICY` message (background → content script → MAIN world) and a `DIALOG_ENTRY` message (MAIN world → content script → background), following the exact relay + ready-handshake shape of `consoleCapture.ts`.

- [ ] **Step 1: Add the message types**

In `src/types/recording.ts`, add `"DIALOG_ENTRY"` and `"DIALOG_POLICY"` to the `MessageType` union (after `"CONSOLE_ENTRY"`):

```typescript
	| "CONSOLE_ENTRY"
	| "DIALOG_ENTRY"
	| "DIALOG_POLICY"
```

Add a `DialogEntryMessage` interface near `ConsoleEntryMessage` (around line 239):

```typescript
export interface DialogEntryMessage extends BaseMessage {
	type: "DIALOG_ENTRY";
	entry: DialogEntry;
}
```

Add `DialogEntryMessage` to the exported `RecordingMessage` union at the bottom of the file (next to `| ConsoleEntryMessage;`):

```typescript
	| ConsoleEntryMessage
	| DialogEntryMessage;
```

(Ensure `DialogEntry` is imported/defined above — it was added in Task 1 in this same file.)

- [ ] **Step 2: Create the MAIN-world override**

Create `src/contentScript/dialogOverride.ts` (mirrors `consoleCapture.ts`'s ready-handshake and buffering):

```typescript
import type { DialogEntry } from "../types/recording";
import { type DialogPolicy, resolveDialog } from "../background/dialogPolicy";

const MESSAGE_SOURCE = "htrncontrol-dialog-override";
const RELAY_READY_TYPE = "HTR_DIALOG_OVERRIDE_RELAY_READY";
const MAX_BUFFERED_BEFORE_READY = 100;

interface DialogOverrideWindow extends Window {
	__htrncontrolDialogOverrideInitialized?: boolean;
}

const overrideWindow = window as DialogOverrideWindow;
let relayReady = false;
let policy: DialogPolicy = { action: "accept" };
const pendingBeforeReady: DialogEntry[] = [];

function sendDialogEntry(entry: DialogEntry): void {
	window.postMessage(
		{ source: MESSAGE_SOURCE, type: "DIALOG_ENTRY", entry },
		"*",
	);
}

function postDialogEntry(entry: DialogEntry): void {
	if (!relayReady) {
		pendingBeforeReady.push(entry);
		if (pendingBeforeReady.length > MAX_BUFFERED_BEFORE_READY) {
			pendingBeforeReady.shift();
		}
		return;
	}
	sendDialogEntry(entry);
}

window.addEventListener("message", (event) => {
	if (event.source !== window) return;
	const data = event.data as
		| { source?: string; type?: string; policy?: DialogPolicy }
		| undefined;
	if (data?.source !== MESSAGE_SOURCE) return;
	if (data.type === RELAY_READY_TYPE) {
		if (relayReady) return;
		relayReady = true;
		for (const entry of pendingBeforeReady) sendDialogEntry(entry);
		pendingBeforeReady.length = 0;
		return;
	}
	if (data.type === "HTR_DIALOG_POLICY" && data.policy) {
		policy = data.policy;
	}
});

function overrideDialog(
	type: "alert" | "confirm" | "prompt",
	defaultReturn: unknown,
): void {
	(window as unknown as Record<string, unknown>)[type] = (message?: unknown) => {
		const text = typeof message === "string" ? message : String(message ?? "");
		const { accept, promptText, entry } = resolveDialog(policy, type, text);
		postDialogEntry(entry);
		if (type === "alert") return undefined;
		if (type === "confirm") return accept;
		// prompt
		return accept ? (promptText ?? "") : null;
	};
	void defaultReturn;
}

if (!overrideWindow.__htrncontrolDialogOverrideInitialized) {
	overrideWindow.__htrncontrolDialogOverrideInitialized = true;
	overrideDialog("alert", undefined);
	overrideDialog("confirm", false);
	overrideDialog("prompt", null);
}
```

- [ ] **Step 3: Relay in the isolated-world content script**

In `src/contentScript/index.ts`, near the existing `CONSOLE_CAPTURE_SOURCE` constant and `handleConsoleCaptureMessage` (lines 41-89), add the dialog source and relay handlers:

```typescript
const DIALOG_OVERRIDE_SOURCE = "htrncontrol-dialog-override";

function handleDialogOverrideMessage(event: MessageEvent): void {
	if (event.source !== window) return;
	const data = event.data as
		| { source?: string; type?: string; entry?: unknown }
		| undefined;
	if (data?.source !== DIALOG_OVERRIDE_SOURCE || data.type !== "DIALOG_ENTRY") {
		return;
	}
	sendToBackground({
		type: "DIALOG_ENTRY",
		entry: data.entry,
	} as RecordingMessage);
}
```

In the same file, register a runtime listener that forwards `DIALOG_POLICY` messages (from the background) down to the MAIN-world override. Find where the content script handles runtime messages (`chrome.runtime.onMessage.addListener` / `handleMessage` around line 224) and add a branch for `DIALOG_POLICY`:

```typescript
		case "DIALOG_POLICY": {
			const msg = message as { type: "DIALOG_POLICY"; policy: unknown };
			window.postMessage(
				{
					source: DIALOG_OVERRIDE_SOURCE,
					type: "HTR_DIALOG_POLICY",
					policy: msg.policy,
				},
				"*",
			);
			sendResponse({ success: true });
			return true;
		}
```

(Match the exact `sendResponse`/`return true` shape the surrounding switch uses — check the neighboring cases before pasting.)

Finally, alongside the existing `window.addEventListener("message", handleConsoleCaptureMessage)` + relay-ready post (lines 353-362), add the dialog equivalents:

```typescript
	window.addEventListener("message", handleDialogOverrideMessage);
	window.postMessage(
		{ source: DIALOG_OVERRIDE_SOURCE, type: "HTR_DIALOG_OVERRIDE_RELAY_READY" },
		"*",
	);
```

- [ ] **Step 4: Handle `DIALOG_ENTRY` in the background**

In `src/background/index.ts`, add `"DIALOG_ENTRY"` handling next to the existing `CONSOLE_ENTRY` case (line ~881). First add the message shape to the listener's parameter union near the `ConsoleEntryMessage` entry (line ~792) — add `| DialogEntryMessage`. Then add the case:

```typescript
				case "DIALOG_ENTRY": {
					const msg = message as DialogEntryMessage;
					if (sender.tab?.id) {
						await recordDialogEntry(sender.tab.id, msg.entry);
					}
					sendResponse({ success: true });
					break;
				}
```

Add the imports at the top of `src/background/index.ts`: extend the existing `./eventStore` import to include `recordDialogEntry`, and add `DialogEntryMessage` to the existing `../types/recording` import.

- [ ] **Step 5: Register the MAIN-world override in the Chrome manifest**

In `src/manifest.ts`, add a third `content_scripts` entry (after the `consoleCapture.ts` MAIN-world entry):

```typescript
		{
			matches: ["http://*/*", "https://*/*"],
			js: ["src/contentScript/dialogOverride.ts"],
			world: "MAIN",
			run_at: "document_start",
		},
```

- [ ] **Step 6: Add the Firefox entry shim**

Create `firefox/src/dialogOverride-entry.ts` (mirrors `firefox/src/consoleCapture-entry.ts`):

```typescript
// Firefox-specific MAIN-world dialog-override entry. Same pattern as the
// other Firefox entry shims: import the polyfill first, then the shared
// source. dialogOverride imports dialogPolicy (pure logic, no chrome.*/
// browser.* APIs), but every Firefox entry point imports the polyfill for
// consistency and in case that changes.
import "./browser-polyfill";
import "../src/contentScript/dialogOverride";
```

- [ ] **Step 7: Add the Firefox IIFE build pass**

Create `firefox/vite.dialogOverride.config.ts` (mirrors `firefox/vite.consoleCapture.config.ts`, overwriting `dialogOverride.js`):

```typescript
// Fourth build pass for the Firefox MAIN-world DIALOG-OVERRIDE content
// script only.
//
// Same problem as vite.content.config.ts / vite.consoleCapture.config.ts:
// content scripts execute as classic scripts, not ES modules, so the main
// build's code-split `dialogOverride.js` (with top-level `import`
// statements) fails to load in Firefox with "import declarations may only
// appear at top level of a module" and the override never runs. This pass
// rebuilds the same entry as a self-contained IIFE and overwrites
// `firefox/build/dialogOverride.js`.
//
// Run AFTER the main build and the other content-script passes (see the
// `firefox:build` script): this config sets `emptyOutDir: false` so it only
// replaces dialogOverride.js.

import { existsSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { defineConfig } from "vite";

const projectRoot = process.cwd();
const fallbackRoot = dirname(fileURLToPath(import.meta.url));
const configRoot = existsSync(resolve(projectRoot, "firefox/vite.config.ts"))
	? projectRoot
	: resolve(fallbackRoot, "..");

export default defineConfig({
	root: configRoot,
	publicDir: false,
	base: "./",
	resolve: {
		alias: {
			"../src": resolve(configRoot, "src"),
		},
	},
	build: {
		emptyOutDir: false,
		outDir: resolve(configRoot, "firefox/build"),
		rollupOptions: {
			input: resolve(configRoot, "firefox/src/dialogOverride-entry.ts"),
			output: {
				format: "iife",
				entryFileNames: "dialogOverride.js",
				inlineDynamicImports: true,
			},
		},
	},
});
```

- [ ] **Step 8: Wire the Firefox rollup input, manifest entry, and build script**

In `firefox/vite.config.ts`, add the rollup input alongside the existing `consoleCapture` one (in the `rollupOptions.input` object, ~line 218):

```typescript
					consoleCapture: "firefox/src/consoleCapture-entry.ts",
					dialogOverride: "firefox/src/dialogOverride-entry.ts",
```

In the same file's emitted manifest `content_scripts` array, add a MAIN-world entry after the `consoleCapture.js` one (~line 112):

```typescript
					{
						matches: ["http://*/*", "https://*/*"],
						js: ["dialogOverride.js"],
						world: "MAIN",
						run_at: "document_start",
						all_frames: false,
					},
```

(`strict_min_version` is already `128.0` from the console-capture MAIN-world work — no change needed.)

In `package.json`, extend the `firefox:build` script with a fourth build pass (after the `vite.consoleCapture.config.ts` pass):

```json
		"firefox:build": "tsc -p firefox/tsconfig.json && vite build --config firefox/vite.config.ts && vite build --config firefox/vite.content.config.ts && vite build --config firefox/vite.consoleCapture.config.ts && vite build --config firefox/vite.dialogOverride.config.ts",
```

- [ ] **Step 9: Verify both builds and the Firefox IIFE output**

Run: `bun run check:fix && bun run typecheck && bun run test`
Expected: no errors, all tests pass.

Run: `bun run firefox:build`
Expected: build succeeds; `firefox/build/dialogOverride.js` exists and contains **zero** top-level `import`/`export` statements (verify: `grep -nE "^(import|export) " firefox/build/dialogOverride.js` returns nothing), and `firefox/build/manifest.json` shows the `dialogOverride.js` `world: "MAIN"` content-script entry.

Run: `bun run build`
Expected: Chrome build succeeds with the `src/contentScript/dialogOverride.ts` MAIN-world entry.

- [ ] **Step 10: Commit**

```bash
git add src/contentScript/dialogOverride.ts firefox/src/dialogOverride-entry.ts firefox/vite.dialogOverride.config.ts src/contentScript/index.ts src/types/recording.ts src/background/index.ts src/manifest.ts firefox/vite.config.ts package.json
git commit -m "feat(extension): Firefox + Chrome MAIN-world dialog override"
```

---

### Task 5: CLI `dialog handle` / `dialog list`

**Files:**
- Create: `htrcli/internal/commands/dialog.go`
- Test: `htrcli/internal/commands/dialog_test.go`

**Interfaces:**
- Consumes: `EventPoller` (existing, `console.go`); `api.Client.ExecuteCommand`, `api.Client.GetEvents`; `GetClient()`, `GetTabID()`, `UseCDP()`, `errUnsupportedCDP()`, `output.JSONOutput`, `output.PrintJSON`, `output.Warning`.
- Produces: `dialogEventData` struct; `dialog` cobra command with `handle` (arm a policy) and `list` (read the dialog buffer) subcommands.

- [ ] **Step 1: Write the failing test**

Create `htrcli/internal/commands/dialog_test.go`:

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

func TestDialogListFormatsEntries(t *testing.T) {
	data, _ := json.Marshal(dialogEventData{
		DialogType:     "confirm",
		Message:        "Delete this item?",
		ResolvedAction: "accept",
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("kind") != "dialog" {
			t.Errorf("expected kind=dialog, got %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(api.ApiResponse{
			OK: true,
			Data: api.EventsResponse{
				Entries:            []api.EventEntry{{Seq: 1, Kind: "dialog", Timestamp: 1000, Data: data}},
				OldestAvailableSeq: 1,
			},
		})
	}))
	defer server.Close()

	c := api.NewClient(server.URL, "")
	poller := &EventPoller{Client: c, Kind: dialogEventKind}
	resp, err := poller.Read(0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := formatDialogEntries(resp)
	if !strings.Contains(out, "confirm") || !strings.Contains(out, "Delete this item?") || !strings.Contains(out, "accept") {
		t.Fatalf("expected dialog fields in output, got: %s", out)
	}
}

func TestParseDialogAction(t *testing.T) {
	if _, err := parseDialogAction("accept"); err != nil {
		t.Fatalf("accept should be valid: %v", err)
	}
	if _, err := parseDialogAction("respond"); err != nil {
		t.Fatalf("respond should be valid: %v", err)
	}
	if _, err := parseDialogAction("frobnicate"); err == nil {
		t.Fatalf("expected error for invalid action")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd htrcli && go test ./internal/commands/... -run 'TestDialogListFormatsEntries|TestParseDialogAction' -v`
Expected: FAIL — `dialogEventKind`, `dialogEventData`, `formatDialogEntries`, `parseDialogAction` undefined.

- [ ] **Step 3: Implement `dialog.go`**

Create `htrcli/internal/commands/dialog.go`:

```go
package commands

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/u007/htrcli/internal/api"
	"github.com/u007/htrcli/internal/output"
)

const dialogEventKind = "dialog"

type dialogEventData struct {
	DialogType     string `json:"dialogType"`
	Message        string `json:"message"`
	ResolvedAction string `json:"resolvedAction"`
	RespondedText  string `json:"respondedText,omitempty"`
}

var allowedDialogActions = map[string]struct{}{
	"accept":  {},
	"dismiss": {},
	"respond": {},
}

func parseDialogAction(raw string) (string, error) {
	action := strings.ToLower(strings.TrimSpace(raw))
	if _, ok := allowedDialogActions[action]; !ok {
		return "", fmt.Errorf("invalid action %q (expected accept, dismiss, or respond)", raw)
	}
	return action, nil
}

func formatDialogEvent(entry api.EventEntry) string {
	var data dialogEventData
	if err := json.Unmarshal(entry.Data, &data); err != nil {
		return fmt.Sprintf("[seq %d] <unparseable dialog entry>", entry.Seq)
	}
	if data.RespondedText != "" {
		return fmt.Sprintf("[seq %d] %s %q → %s (%q)", entry.Seq, data.DialogType, data.Message, data.ResolvedAction, data.RespondedText)
	}
	return fmt.Sprintf("[seq %d] %s %q → %s", entry.Seq, data.DialogType, data.Message, data.ResolvedAction)
}

func formatDialogEntries(resp *api.EventsResponse) string {
	if resp == nil {
		return ""
	}
	var b strings.Builder
	if resp.Dropped > 0 {
		fmt.Fprintf(&b, "%s %d events were evicted (buffer cap reached)\n", output.Warning("⚠"), resp.Dropped)
	}
	for _, entry := range resp.Entries {
		b.WriteString(formatDialogEvent(entry))
		b.WriteByte('\n')
	}
	return b.String()
}

var (
	dialogHandleAction string
	dialogHandleText   string
	dialogListSince    int
)

var dialogCmd = &cobra.Command{
	Use:   "dialog",
	Short: "Arm dialog handling and list handled dialogs",
}

var dialogHandleCmd = &cobra.Command{
	Use:   "handle",
	Short: "Arm a policy for the next JavaScript dialog(s)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if UseCDP() {
			return errUnsupportedCDP("dialog handle")
		}
		action, err := parseDialogAction(dialogHandleAction)
		if err != nil {
			return err
		}
		tabID, err := GetTabID()
		if err != nil {
			return err
		}
		options := map[string]any{"action": action}
		if action == "respond" {
			options["text"] = dialogHandleText
		}
		result, err := GetClient().ExecuteCommand(tabID, api.Command{
			ID:      "1",
			Action:  "dialogPolicy",
			Options: options,
		})
		if err != nil {
			return err
		}
		if !result.Success {
			return fmt.Errorf("%s", result.Error)
		}
		if output.JSONOutput {
			output.PrintJSON(result)
			return nil
		}
		fmt.Printf("Dialog policy armed: %s\n", action)
		return nil
	},
}

var dialogListCmd = &cobra.Command{
	Use:   "list",
	Short: "List handled dialogs",
	RunE: func(cmd *cobra.Command, args []string) error {
		if UseCDP() {
			return errUnsupportedCDP("dialog list")
		}
		tabID, err := GetTabID()
		if err != nil {
			return err
		}
		poller := &EventPoller{Client: GetClient(), TabID: tabID, Kind: dialogEventKind}
		resp, err := poller.Read(dialogListSince)
		if err != nil {
			return err
		}
		if output.JSONOutput {
			output.PrintJSON(resp)
			return nil
		}
		fmt.Print(formatDialogEntries(resp))
		return nil
	},
}

func init() {
	dialogHandleCmd.Flags().StringVar(&dialogHandleAction, "action", "accept", "accept, dismiss, or respond")
	dialogHandleCmd.Flags().StringVar(&dialogHandleText, "text", "", "response text (used with --action respond)")
	dialogListCmd.Flags().IntVar(&dialogListSince, "since", 0, "cursor to list after")

	dialogCmd.AddCommand(dialogHandleCmd)
	dialogCmd.AddCommand(dialogListCmd)
	rootCmd.AddCommand(dialogCmd)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd htrcli && go test ./internal/commands/... -run 'TestDialogListFormatsEntries|TestParseDialogAction' -v`
Expected: PASS

- [ ] **Step 5: Full commands suite (regression check)**

Run: `cd htrcli && go test ./internal/commands/... -v`
Expected: PASS

- [ ] **Step 6: Build the CLI**

Run: `make htrcli-build`
Expected: builds with no errors; `htrcli/bin/htrcli dialog --help` lists `handle` and `list`.

- [ ] **Step 7: Commit**

```bash
git add htrcli/internal/commands/dialog.go htrcli/internal/commands/dialog_test.go
git commit -m "feat(htrcli): add dialog handle/list CLI commands"
```

---

### Task 6: End-to-end verification + record limitations

**Files:** none (verification) + `TODO.md`

- [ ] **Step 1: Chrome accept/dismiss**

Run `htrcli serve`, load the Chrome build, open a page with a button that calls `confirm("Proceed?")` and logs the result. Run `htrcli dialog handle --action accept`, then click the button. Confirm the page sees `true` and `htrcli dialog list` shows the confirm with `resolvedAction: accept`. Repeat with `--action dismiss` and confirm the page sees `false`.

- [ ] **Step 2: Chrome respond (prompt)**

Add a button calling `prompt("Name?")`. Run `htrcli dialog handle --action respond --text "Ada"`, click the button, and confirm the page receives `"Ada"` and `dialog list` records `respondedText: "Ada"`.

- [ ] **Step 3: Chrome no-attach-leak**

After the armed window's default 30s timer elapses (or the tab closes), confirm Chrome's "This tab is being debugged" banner disappears — the debugger detached. Confirm a dialog fired *after* the window closes is no longer auto-answered (native dialog appears), proving the window is bounded.

- [ ] **Step 4: Firefox override (128+)**

Load the Firefox build (`bun run firefox:build`, Firefox 128+ via `about:debugging`). Run `htrcli dialog handle --action accept`, then trigger a `confirm()` on the page. Confirm it is auto-accepted and `dialog list` shows the entry. Note the documented limitations: a dialog fired before the override lands is missed, and `beforeunload` cannot be intercepted.

- [ ] **Step 5: Record limitations in TODO.md**

Append to `TODO.md`:

```markdown
## htrcli dialog handling — known limitations
- The Firefox MAIN-world override is racy against dialogs a page fires before it lands at document_start, and cannot intercept native beforeunload dialogs (Chrome CDP handles beforeunload; Firefox does not).
- Dialog handling shares the refcounted debuggerManager with network capture; the "Already attached" limitation from the network plan's TODO applies here too.
```

(Note: the Firefox 128+ `world:"MAIN"` requirement is not a per-feature limitation — it is already the extension's hard `strict_min_version` floor from the console-capture MAIN-world work, so the dialog override injects on every supported Firefox.)

Commit:

```bash
git add TODO.md
git commit -m "docs: record htrcli dialog handling limitations"
```

---

## Self-Review

**1. Spec coverage (§6 "Dialog handling"):**
- `htrcli dialog handle --action accept|dismiss|respond --text "..."` → Task 5 `dialogHandleCmd`. ✓
- `htrcli dialog list [--since N]` → Task 5 `dialogListCmd`. ✓
- Handler armed **before** the triggering action (standing policy) → the `dialogPolicy` command sets policy ahead of time; Tasks 3 (Chrome) & 4 (Firefox). ✓
- Chrome via `Page.javascriptDialogOpening` → immediately `Page.handleJavaScriptDialog` per policy, logged as `DialogEntry` → Task 3. ✓
- Firefox fallback: MAIN-world override of `window.alert/confirm/prompt` at `document_start`, mirroring `consoleCapture.ts` + ready-handshake → Task 4. ✓
- Documented limitations (racy pre-override dialogs; cannot intercept `beforeunload`) → Task 6 + Global Constraints. ✓
- New files `src/background/dialog*.ts` / `src/contentScript/dialogOverride.ts`; modified `src/background/index.ts`, `internal/commands/*` → matches the spec's "New/Modified" list (spec named `internal/cdp/dialog.go` for the Go CDP-transport path; per the task brief this plan targets the **extension** transport via `chrome.debugger` in the background, and guards the CDP transport with `errUnsupportedCDP`). ✓

**2. Placeholder scan:** No "TBD"/"add error handling"/"similar to Task N". Every code step is complete. Caught errors are logged with context (`console.warn('[HTR NControl] ...')`) or surfaced loudly via `replyError` — no silent swallows.

**3. Type consistency:** `dialogEventData` (Go) JSON keys (`dialogType/message/resolvedAction/respondedText`) match `DialogEntry` (TS). `resolveDialog`'s return shape (`{accept, promptText, entry}`) is consumed identically in Task 3 (CDP) and Task 4 (override). `DialogPolicy`/`DialogAction` names are consistent across Tasks 2, 3, 4. The `dialogPolicy` action name matches between the Go `dialogHandleCmd` (Task 5) and the `sendCommandToTab` branch (Task 3). Message names `DIALOG_ENTRY`/`DIALOG_POLICY` are consistent across `recording.ts`, `dialogOverride.ts`, `contentScript/index.ts`, and `background/index.ts`.

**Cross-plan dependency check:** This plan's Task 1 assumes `recordEvent`/`EventKind`/`BufferedEventData` from the network plan's Task 1, and Task 3 assumes `debuggerManager` from the network plan's Task 2. Both are stated in Global Constraints and are Phase-2 siblings in the spec build order. No file is edited by both plans in a conflicting way: the network plan owns the `eventStore.ts` generalization skeleton and the `nativeHost.ts` debugger-manager import + `onRemoved` listener; this plan makes additive widenings (`"dialog"` kind, an extra `onRemoved` branch, an extra `sendCommandToTab` branch).

**Judgment calls made:** (a) Dialog default window is 30s (vs network's 10s) because dialogs are armed ahead of an unpredictable user action. (b) Firefox override is fully wired for both browsers via the established three-piece MAIN-world pattern (`consoleCapture.ts`'s entry shim + IIFE build pass + rollup input + manifest entry — Task 4 Steps 6-8), not just registered in the Chrome manifest; the extension's existing Firefox 128+ `strict_min_version` floor already covers `world:"MAIN"`, so no per-feature version caveat remains. (c) `respond` with no `--text` is treated as an empty-string response, not an error.
