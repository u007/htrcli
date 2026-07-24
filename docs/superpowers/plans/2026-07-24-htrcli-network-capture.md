# htrcli Passive Network Capture + `network wait` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Capture page network traffic (request/response metadata + response body) into the existing durable event buffer and expose it as `htrcli network read` / `network watch` / `network wait --url <glob> [--status N]`.

**Architecture:** Network capture reuses the console phase's event-buffer plumbing end-to-end — the same `chrome.storage.session` buffer (`src/background/eventStore.ts`), the same `POST /api/events/ingest` flush, the same daemon `EventStore`, and the same cursor-based `GET /api/events` poll — with a new event `kind: "network"`. Capture is **armed in bounded windows**, never as a permanent debugger attach: `network watch`/`network wait` send a `networkCapture` command that (on Chrome) attaches `chrome.debugger`, enables the CDP `Network` domain, streams `requestWillBeSent`/`responseReceived`/`loadingFinished` into the buffer, and **auto-detaches** after a self-held timer. `network read` is a pure buffer poll (no attach). On Firefox there is no `chrome.debugger`; capture uses always-on `browser.webRequest` observation, so `read` works standalone there.

**Tech Stack:** Go (cobra CLI, stdlib `net/http`), TypeScript (Chrome `chrome.debugger`/CDP `Network` domain, Firefox `browser.webRequest`, `chrome.storage.session`), Bun test runner, Go's `testing` package.

## Global Constraints

- Package manager: `bun` only for the extension — never npm/yarn.
- Biome lint/format (tabs, double quotes) — run `bun run check:fix` before committing TS changes.
- Go tests: `go test ./...` from `htrcli/`.
- Async `chrome.runtime.onMessage` listeners must `return true` when responding asynchronously.
- Extension console/error logging prefix: `console.error/warn('[HTR NControl] ...')`.
- Event buffer count cap: 500 entries per (tab, kind). Eviction is never silent — the `dropped` count is already reported by the shared `EventStore`/`eventStore.ts` and surfaced by the CLI formatter.
- **Debugger-attach lifecycle (verbatim project constraint):** never hold a permanent `chrome.debugger` attach. CDP network capture attaches only for the duration of an explicitly-armed, time-bounded window and auto-detaches when the window's timer fires, the tab navigates, or the tab closes. The arm command **acks immediately** (like the existing `getReadyTabs` action in `sendCommandToTab`) and the background holds its own detach timer independent of the daemon's 30s command timeout — so `network watch --timeout 60000` never trips that timeout.
- **Chrome/Firefox asymmetry (must be documented, never silent):** on Chrome, `network read` only returns traffic captured during a preceding or concurrent `watch`/`wait` window (you cannot capture traffic the debugger was not attached for — Playwright cannot either). On Firefox, `webRequest` observation is always-on and has no attach/banner cost, so `read` works standalone.
- **Shared debugger manager:** this plan introduces `src/background/debuggerManager.ts`, a refcounted attach + shared `chrome.debugger.onEvent` dispatcher. The dialog-handling plan (`2026-07-24-htrcli-dialog-handling.md`) depends on this module and on the `recordEvent`/`EventKind` generalization from Task 1 of this plan.
- **Known limitation (record in TODO.md, Task 8):** the existing trusted-input (`cdpInput.ts`) and `CDP_EVAL` paths do their own `chrome.debugger.attach` and are not yet routed through `debuggerManager`. Running a trusted-input/eval command *while a capture window is open on the same tab* will fail loudly with "Already attached". This is acceptable for this phase (capture windows are short and the CLI user waits for `watch`/`wait` to finish) and is documented, not silently worked around.
- No new external runtime dependencies. New extension permission: `webRequest` (non-blocking observation only — **not** `webRequestBlocking`, which belongs to the deferred mocking phase §1b).

---

### Task 1: `NetworkEntry` type + generalize the event buffer for multiple kinds

**Files:**
- Modify: `src/types/recording.ts`
- Modify: `src/background/eventStore.ts`
- Test: `src/background/eventStore.test.ts`

**Interfaces:**
- Consumes: the existing `eventStore.ts` internals (`BufferedEvent`, `getOrCreateBucket`, `recordConsoleEntry`, `flushPending`).
- Produces: `NetworkEntry` interface in `recording.ts`; `EventKind = "console" | "network" | "dialog"`; a generic `recordEvent(tabId: number, kind: EventKind, data: BufferedEventData): Promise<void>`; `recordNetworkEntry(tabId: number, entry: NetworkEntry): Promise<void>`. `recordConsoleEntry` keeps its exact existing signature and behavior (now delegating to `recordEvent`).

- [x] **Step 1: Write the failing test**

Append to `src/background/eventStore.test.ts` (inside the existing `describe` block, reusing its `installStorageMock`/`__resetEventStoreForTests` setup — check the top of the file for the exact `beforeEach` already present and add these `it` blocks alongside the console ones):

```typescript
	it("records network entries in a separate bucket with their own seq", async () => {
		await recordNetworkEntry(1, {
			requestId: "req-1",
			url: "https://example.com/api/users",
			method: "GET",
			status: 200,
			durationMs: 42,
		});
		await recordNetworkEntry(1, {
			requestId: "req-2",
			url: "https://example.com/api/orders",
			method: "POST",
			status: 500,
			durationMs: 7,
		});

		const posted: { kind: string; entries: { seq: number; data: { url: string } }[] }[] = [];
		await flushPending(async (_tabId, kind, entries) => {
			posted.push({ kind, entries: entries as { seq: number; data: { url: string } }[] });
			return true;
		});

		const networkBatch = posted.find((p) => p.kind === "network");
		expect(networkBatch).toBeDefined();
		expect(networkBatch?.entries.map((e) => e.seq)).toEqual([1, 2]);
		expect(networkBatch?.entries[0].data.url).toBe("https://example.com/api/users");
	});

	it("keeps console and network seq counters independent per tab", async () => {
		await recordConsoleEntry(1, { level: "log", args: ["a"] });
		await recordNetworkEntry(1, {
			requestId: "req-1",
			url: "https://example.com/x",
			method: "GET",
		});

		const byKind: Record<string, number[]> = {};
		await flushPending(async (_tabId, kind, entries) => {
			byKind[kind] = (entries as { seq: number }[]).map((e) => e.seq);
			return true;
		});
		expect(byKind.console).toEqual([1]);
		expect(byKind.network).toEqual([1]);
	});
```

Add `recordNetworkEntry` to the existing top-of-file import from `./eventStore` (it currently imports `flushPending`, `recordConsoleEntry`, and the test-reset helper).

- [x] **Step 2: Run test to verify it fails**

Run: `bun test src/background/eventStore.test.ts`
Expected: FAIL — `recordNetworkEntry` is not exported from `./eventStore`.

- [x] **Step 3: Add the `NetworkEntry` type**

In `src/types/recording.ts`, immediately after the existing `ConsoleEntry` interface (around line 76), add:

```typescript
// Structured network payload captured via CDP (Chrome) or webRequest (Firefox).
export interface NetworkEntry {
	requestId: string;
	url: string;
	method: string;
	status?: number;
	requestHeaders?: Record<string, string>;
	responseHeaders?: Record<string, string>;
	bodyTruncated?: boolean; // true if the response body was capped
	body?: string; // response body, omitted on Firefox (webRequest can't cheaply read it)
	durationMs?: number;
}
```

- [x] **Step 4: Generalize `eventStore.ts`**

In `src/background/eventStore.ts`:

Change the top import and kind/data types. Replace lines 1-12 (the current import + `ConsoleEntryData`/`EventKind`/`BufferedEvent`):

```typescript
import type { ConsoleEntry, NetworkEntry } from "../types/recording";

export type ConsoleEntryData = ConsoleEntry;
export type NetworkEntryData = NetworkEntry;
export type BufferedEventData = ConsoleEntryData | NetworkEntryData;

export type EventKind = "console" | "network";

export interface BufferedEvent {
	seq: number;
	kind: EventKind;
	timestamp: number;
	data: BufferedEventData;
}
```

(The dialog plan widens `EventKind` to include `"dialog"` and `BufferedEventData` to include its payload — that is an additive change in that plan.)

Replace the current `bucketKey` and `getOrCreateBucket` (lines 44-82) so the kind is a parameter rather than hardcoded `"console"`:

```typescript
function bucketKey(tabId: number, kind: EventKind): string {
	return `${tabId}:${kind}`;
}

function emptyState(): BufferedState {
	return { buckets: {}, generation: null };
}

async function loadState(): Promise<BufferedState> {
	if (state) return state;
	if (!stateLoadPromise) {
		stateLoadPromise = chrome.storage.session
			.get(STORAGE_KEY)
			.then((result) => {
				const stored = result[STORAGE_KEY] as BufferedState | undefined;
				state = stored ?? emptyState();
				return state;
			});
	}
	return stateLoadPromise;
}

async function saveState(): Promise<void> {
	if (!state) return;
	await chrome.storage.session.set({ [STORAGE_KEY]: state });
}

function getOrCreateBucket(
	currentState: BufferedState,
	tabId: number,
	kind: EventKind,
): BufferedBucket {
	const key = bucketKey(tabId, kind);
	let bucket = currentState.buckets[key];
	if (!bucket) {
		bucket = { nextSeq: 1, entries: [] };
		currentState.buckets[key] = bucket;
	}
	return bucket;
}
```

(The `emptyState`/`loadState`/`saveState` functions are unchanged in behavior — they are shown here only because they sit between the old `bucketKey` and `getOrCreateBucket`; keep the existing bodies if they already match.)

Replace the existing `recordConsoleEntry` (lines 117-138) with a generic `recordEvent` plus thin typed wrappers:

```typescript
// Record one captured event of any kind in durable session storage.
export async function recordEvent(
	tabId: number,
	kind: EventKind,
	data: BufferedEventData,
): Promise<void> {
	if (tabId <= 0) return;
	const currentState = await loadState();
	const bucket = getOrCreateBucket(currentState, tabId, kind);
	bucket.entries.push({
		seq: bucket.nextSeq,
		kind,
		timestamp: Date.now(),
		data,
	});
	bucket.nextSeq += 1;
	trimBucket(bucket);
	await saveState();
}

// Record a console entry in durable session storage.
export async function recordConsoleEntry(
	tabId: number,
	entry: ConsoleEntryData,
): Promise<void> {
	await recordEvent(tabId, "console", {
		level: normalizeLevel(entry.level),
		args: [...entry.args],
		source: entry.source,
	});
}

// Record a network entry in durable session storage.
export async function recordNetworkEntry(
	tabId: number,
	entry: NetworkEntryData,
): Promise<void> {
	await recordEvent(tabId, "network", entry);
}
```

The existing `flushPendingOnce` already splits `key.split(":", 2)` into `[tabID, kind]` and posts with that `kind`, so it handles `network` buckets with no change. Leave `flushPendingOnce`/`flushPending`/`trimBucket`/`normalizeLevel` as they are.

- [x] **Step 5: Run test to verify it passes**

Run: `bun test src/background/eventStore.test.ts`
Expected: PASS (existing console tests still green, plus the two new network tests)

- [x] **Step 6: Biome + typecheck**

Run: `bun run check:fix && bun run typecheck`
Expected: no errors

- [x] **Step 7: Commit**

```bash
git add src/types/recording.ts src/background/eventStore.ts src/background/eventStore.test.ts
git commit -m "feat(extension): generalize event buffer for network entries"
```

---

### Task 2: Refcounted shared debugger manager

**Files:**
- Create: `src/background/debuggerManager.ts`
- Test: `src/background/debuggerManager.test.ts`

**Interfaces:**
- Consumes: `chrome.debugger` (injectable in tests via the exported `__setDebuggerImplForTests`).
- Produces: `attachShared(tabId: number): Promise<void>`, `detachShared(tabId: number): Promise<void>`, `onDebuggerEvent(cb: (source: { tabId?: number }, method: string, params: unknown) => void): () => void`, `sendToTab(tabId: number, method: string, params?: Record<string, unknown>): Promise<unknown>`, `__setDebuggerImplForTests(impl | null): void`.

This module is the single attach/detach path for the *long-lived* capture features (network here, dialogs in the sibling plan) so two features attaching the same tab share one real `chrome.debugger.attach` via a refcount, and one shared `onEvent` listener fans events out to all subscribers. It does **not** refactor the existing one-shot `cdpInput.ts`/`CDP_EVAL` attaches (see Global Constraints known-limitation note).

- [x] **Step 1: Write the failing test**

Create `src/background/debuggerManager.test.ts`:

```typescript
import { beforeEach, describe, expect, it } from "bun:test";
import {
	__setDebuggerImplForTests,
	attachShared,
	detachShared,
	onDebuggerEvent,
} from "./debuggerManager";

interface FakeDebugger {
	attachCalls: number;
	detachCalls: number;
	emit: (source: { tabId?: number }, method: string, params: unknown) => void;
}

function installFakeDebugger(): FakeDebugger {
	let attachCalls = 0;
	let detachCalls = 0;
	let handler:
		| ((source: { tabId?: number }, method: string, params: unknown) => void)
		| null = null;
	__setDebuggerImplForTests({
		attach: async () => {
			attachCalls++;
		},
		detach: async () => {
			detachCalls++;
		},
		sendCommand: async () => ({}),
		onEvent: {
			addListener: (cb) => {
				handler = cb;
			},
			removeListener: () => {
				handler = null;
			},
		},
	});
	return {
		get attachCalls() {
			return attachCalls;
		},
		get detachCalls() {
			return detachCalls;
		},
		emit: (source, method, params) => handler?.(source, method, params),
	};
}

describe("debuggerManager", () => {
	beforeEach(() => {
		__setDebuggerImplForTests(null);
	});

	it("attaches once and detaches once across balanced refcounted calls", async () => {
		const fake = installFakeDebugger();
		await attachShared(7);
		await attachShared(7);
		expect(fake.attachCalls).toBe(1);
		await detachShared(7);
		expect(fake.detachCalls).toBe(0); // still one holder
		await detachShared(7);
		expect(fake.detachCalls).toBe(1);
	});

	it("delivers debugger events only to subscribers until they unsubscribe", async () => {
		const fake = installFakeDebugger();
		await attachShared(7);
		const received: string[] = [];
		const unsub = onDebuggerEvent((source, method) => {
			if (source.tabId === 7) received.push(method);
		});
		fake.emit({ tabId: 7 }, "Network.responseReceived", {});
		unsub();
		fake.emit({ tabId: 7 }, "Network.loadingFinished", {});
		expect(received).toEqual(["Network.responseReceived"]);
	});
});
```

- [x] **Step 2: Run test to verify it fails**

Run: `bun test src/background/debuggerManager.test.ts`
Expected: FAIL — `./debuggerManager` module does not exist.

- [x] **Step 3: Implement `debuggerManager.ts`**

Create `src/background/debuggerManager.ts`:

```typescript
/**
 * Refcounted chrome.debugger attach + shared onEvent fan-out. The single
 * attach/detach path for long-lived capture features (network + dialogs) so
 * two features can capture the same tab through one real attach. Follows the
 * project rule that a debugger attach is never held permanently: callers
 * (bounded capture windows) balance every attachShared with a detachShared.
 */

type EventCb = (
	source: { tabId?: number },
	method: string,
	params: unknown,
) => void;

interface DebuggerImpl {
	attach: (target: { tabId: number }, version: string) => Promise<void>;
	detach: (target: { tabId: number }) => Promise<void>;
	sendCommand: (
		target: { tabId: number },
		method: string,
		params?: Record<string, unknown>,
	) => Promise<unknown>;
	onEvent: {
		addListener: (cb: EventCb) => void;
		removeListener: (cb: EventCb) => void;
	};
}

let injected: DebuggerImpl | null = null;

// Test seam: swap the real chrome.debugger for a fake. Passing null restores
// the real API and resets internal state so each test starts clean.
export function __setDebuggerImplForTests(impl: DebuggerImpl | null): void {
	injected = impl;
	refcounts.clear();
	subscribers.clear();
	sharedListenerAttached = false;
}

function impl(): DebuggerImpl {
	if (injected) return injected;
	return chrome.debugger as unknown as DebuggerImpl;
}

const refcounts = new Map<number, number>();
const subscribers = new Set<EventCb>();
let sharedListenerAttached = false;

function sharedListener(
	source: { tabId?: number },
	method: string,
	params: unknown,
): void {
	for (const cb of subscribers) {
		cb(source, method, params);
	}
}

function ensureSharedListener(): void {
	if (sharedListenerAttached) return;
	impl().onEvent.addListener(sharedListener);
	sharedListenerAttached = true;
}

// Attach the debugger to a tab, refcounted: the real attach happens only on
// the first holder. Throws verbatim if the real attach fails (DevTools open /
// another client attached) rather than silently continuing.
export async function attachShared(tabId: number): Promise<void> {
	const current = refcounts.get(tabId) ?? 0;
	if (current === 0) {
		ensureSharedListener();
		await impl().attach({ tabId }, "1.3");
	}
	refcounts.set(tabId, current + 1);
}

// Release one hold on a tab's attach; the real detach happens only when the
// last holder releases.
export async function detachShared(tabId: number): Promise<void> {
	const current = refcounts.get(tabId) ?? 0;
	if (current <= 1) {
		refcounts.delete(tabId);
		if (current === 1) {
			try {
				await impl().detach({ tabId });
			} catch (err) {
				// Benign: the tab may have closed, which auto-detaches the
				// debugger. Log at warn so it is never a silent swallow.
				console.warn("[HTR NControl] debugger detach failed:", err);
			}
		}
		return;
	}
	refcounts.set(tabId, current - 1);
}

// Subscribe to all debugger events. Returns an unsubscribe function.
export function onDebuggerEvent(cb: EventCb): () => void {
	subscribers.add(cb);
	return () => subscribers.delete(cb);
}

// Send a CDP command to an already-attached tab.
export function sendToTab(
	tabId: number,
	method: string,
	params: Record<string, unknown> = {},
): Promise<unknown> {
	return impl().sendCommand({ tabId }, method, params);
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `bun test src/background/debuggerManager.test.ts`
Expected: PASS

- [x] **Step 5: Biome + typecheck**

Run: `bun run check:fix && bun run typecheck`
Expected: no errors

- [x] **Step 6: Commit**

```bash
git add src/background/debuggerManager.ts src/background/debuggerManager.test.ts
git commit -m "feat(extension): add refcounted shared debugger manager"
```

---

### Task 3: CDP → `NetworkEntry` assembly buffer

**Files:**
- Create: `src/background/networkCapture.ts`
- Test: `src/background/networkCapture.test.ts`

**Interfaces:**
- Consumes: `NetworkEntry` (Task 1).
- Produces: `class NetworkCaptureBuffer` with `onRequestWillBeSent(params)`, `onResponseReceived(params)`, `onLoadingFinished(params): NetworkEntry | null`, `onLoadingFailed(params): NetworkEntry | null`. Pure in-memory assembly of CDP `Network.*` events into a completed `NetworkEntry`; the live wiring (attach, `getResponseBody`, record) is Task 4 and consumes this.

- [x] **Step 1: Write the failing test**

Create `src/background/networkCapture.test.ts`:

```typescript
import { describe, expect, it } from "bun:test";
import { NetworkCaptureBuffer } from "./networkCapture";

describe("NetworkCaptureBuffer", () => {
	it("assembles a completed entry from request → response → finished", () => {
		const buf = new NetworkCaptureBuffer();
		buf.onRequestWillBeSent({
			requestId: "1",
			request: {
				url: "https://example.com/api",
				method: "GET",
				headers: { accept: "application/json" },
			},
			timestamp: 1000, // CDP monotonic seconds
		});
		buf.onResponseReceived({
			requestId: "1",
			response: { status: 200, headers: { "content-type": "application/json" } },
		});
		const entry = buf.onLoadingFinished({ requestId: "1", timestamp: 1000.5 });
		expect(entry).not.toBeNull();
		expect(entry?.url).toBe("https://example.com/api");
		expect(entry?.method).toBe("GET");
		expect(entry?.status).toBe(200);
		expect(entry?.durationMs).toBe(500);
		expect(entry?.requestHeaders?.accept).toBe("application/json");
		expect(entry?.responseHeaders?.["content-type"]).toBe("application/json");
	});

	it("returns null for a finished event with no matching request", () => {
		const buf = new NetworkCaptureBuffer();
		expect(buf.onLoadingFinished({ requestId: "missing", timestamp: 5 })).toBeNull();
	});

	it("assembles a failed request with no status", () => {
		const buf = new NetworkCaptureBuffer();
		buf.onRequestWillBeSent({
			requestId: "2",
			request: { url: "https://example.com/down", method: "POST" },
			timestamp: 2,
		});
		const entry = buf.onLoadingFailed({ requestId: "2", timestamp: 2.1 });
		expect(entry?.url).toBe("https://example.com/down");
		expect(entry?.status).toBeUndefined();
		expect(entry?.durationMs).toBe(100);
	});
});
```

- [x] **Step 2: Run test to verify it fails**

Run: `bun test src/background/networkCapture.test.ts`
Expected: FAIL — `./networkCapture` module does not exist.

- [x] **Step 3: Implement `networkCapture.ts`**

Create `src/background/networkCapture.ts`:

```typescript
/**
 * In-memory assembly of CDP Network.* events into completed NetworkEntry
 * records. Pure and dependency-free so it can be unit-tested without a real
 * debugger. Task 4 wires this to chrome.debugger via debuggerManager.
 */

import type { NetworkEntry } from "../types/recording";

interface Inflight {
	requestId: string;
	url: string;
	method: string;
	requestHeaders?: Record<string, string>;
	startMs: number; // CDP monotonic timestamp in ms
	status?: number;
	responseHeaders?: Record<string, string>;
}

interface RequestWillBeSentParams {
	requestId: string;
	request: { url: string; method: string; headers?: Record<string, string> };
	timestamp: number; // CDP Network.MonotonicTime (seconds)
}

interface ResponseReceivedParams {
	requestId: string;
	response: { status: number; headers?: Record<string, string> };
}

interface LoadingDoneParams {
	requestId: string;
	timestamp: number; // seconds
}

export class NetworkCaptureBuffer {
	private inflight = new Map<string, Inflight>();

	onRequestWillBeSent(params: RequestWillBeSentParams): void {
		this.inflight.set(params.requestId, {
			requestId: params.requestId,
			url: params.request.url,
			method: params.request.method,
			requestHeaders: params.request.headers,
			startMs: params.timestamp * 1000,
		});
	}

	onResponseReceived(params: ResponseReceivedParams): void {
		const r = this.inflight.get(params.requestId);
		if (!r) return;
		r.status = params.response.status;
		r.responseHeaders = params.response.headers;
	}

	onLoadingFinished(params: LoadingDoneParams): NetworkEntry | null {
		return this.complete(params);
	}

	onLoadingFailed(params: LoadingDoneParams): NetworkEntry | null {
		return this.complete(params);
	}

	private complete(params: LoadingDoneParams): NetworkEntry | null {
		const r = this.inflight.get(params.requestId);
		if (!r) return null;
		this.inflight.delete(params.requestId);
		const durationMs = Math.max(0, Math.round(params.timestamp * 1000 - r.startMs));
		return {
			requestId: r.requestId,
			url: r.url,
			method: r.method,
			status: r.status,
			requestHeaders: r.requestHeaders,
			responseHeaders: r.responseHeaders,
			durationMs,
		};
	}
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `bun test src/background/networkCapture.test.ts`
Expected: PASS

- [x] **Step 5: Biome + typecheck**

Run: `bun run check:fix && bun run typecheck`
Expected: no errors

- [x] **Step 6: Commit**

```bash
git add src/background/networkCapture.ts src/background/networkCapture.test.ts
git commit -m "feat(extension): add CDP network event assembly buffer"
```

---

### Task 4: Chrome arm handler — attach, capture window, response body, record

**Files:**
- Modify: `src/background/nativeHost.ts`
- Test: none (integration against `chrome.debugger`; verified in Task 7's manual E2E — this task adds the live wiring around the Task 2/3 units that are already unit-tested)

**Interfaces:**
- Consumes: `attachShared`, `detachShared`, `onDebuggerEvent`, `sendToTab` (Task 2); `NetworkCaptureBuffer` (Task 3); `recordNetworkEntry` (Task 1).
- Produces: a `networkCapture` command action handled in `sendCommandToTab`, arming a bounded Chrome capture window. Ack is immediate; a self-held timer detaches. The command's `options` are `{ durationMs?: number }` (default 10000, clamped to [1000, 120000]).

- [x] **Step 1: Add the imports and module state**

At the top of `src/background/nativeHost.ts`, alongside the existing `cdpInput` import (line ~11), add:

```typescript
import {
	attachShared,
	detachShared,
	onDebuggerEvent,
	sendToTab,
} from "./debuggerManager";
import { NetworkCaptureBuffer } from "./networkCapture";
import { recordNetworkEntry } from "./eventStore";
import type { NetworkEntry } from "../types/recording";
```

Near the other module-level state in `nativeHost.ts`, add the active-capture registry and constants:

```typescript
const MAX_RESPONSE_BODY_BYTES = 64 * 1024;

interface NetworkCaptureHandle {
	buffer: NetworkCaptureBuffer;
	unsubscribe: () => void;
	timer: ReturnType<typeof setTimeout>;
}

const activeNetworkCaptures = new Map<number, NetworkCaptureHandle>();

function clampDuration(ms: number | undefined): number {
	const v = typeof ms === "number" && Number.isFinite(ms) ? ms : 10000;
	return Math.min(120000, Math.max(1000, v));
}
```

- [x] **Step 2: Add the arm handler function**

Add this function in `src/background/nativeHost.ts` (near `handleDebuggerEval`, so it sits with the other background-handled actions):

```typescript
/**
 * Fetch a completed request's response body via CDP, capped. Returns the body
 * text plus whether it was truncated. Failures (body evicted, binary, etc.)
 * are non-fatal: the entry is still recorded without a body.
 */
async function fetchResponseBody(
	tabId: number,
	requestId: string,
): Promise<{ body?: string; bodyTruncated?: boolean }> {
	try {
		const res = (await sendToTab(tabId, "Network.getResponseBody", {
			requestId,
		})) as { body: string; base64Encoded: boolean };
		let body = res.base64Encoded ? "[base64 body omitted]" : res.body;
		let bodyTruncated = false;
		if (body.length > MAX_RESPONSE_BODY_BYTES) {
			body = body.slice(0, MAX_RESPONSE_BODY_BYTES);
			bodyTruncated = true;
		}
		return { body, bodyTruncated };
	} catch (err) {
		// Body may be unavailable (evicted, redirect, no content). Not fatal.
		console.warn("[HTR NControl] getResponseBody failed:", err);
		return {};
	}
}

/**
 * Arm a bounded Chrome network-capture window on a tab. Attaches the debugger
 * via the shared refcounted manager, enables the CDP Network domain, streams
 * request/response/finished events into a NetworkCaptureBuffer, records each
 * completed entry, and auto-detaches after a self-held timer. Never a
 * permanent attach. Firefox (no chrome.debugger) is handled by the caller as
 * a no-op ack, since webRequest capture there is always-on.
 */
async function handleNetworkCapture(
	tabId: number,
	payload: Command,
): Promise<void> {
	// Firefox: no debugger. webRequest capture is always-on (see index.ts), so
	// arming is a no-op — just ack.
	if (typeof chrome.debugger === "undefined") {
		sendToNative({
			type: "command_result",
			tabId,
			payload: {
				id: payload.id,
				success: true,
				data: { armed: true, transport: "webRequest" },
			} as CommandResult,
		});
		return;
	}

	const durationMs = clampDuration(
		(payload.options?.durationMs as number | undefined) ?? undefined,
	);

	// Re-arming an already-open window: extend it rather than double-attach.
	const existing = activeNetworkCaptures.get(tabId);
	if (existing) {
		clearTimeout(existing.timer);
		existing.timer = setTimeout(() => void stopNetworkCapture(tabId), durationMs);
		sendToNative({
			type: "command_result",
			tabId,
			payload: {
				id: payload.id,
				success: true,
				data: { armed: true, durationMs, transport: "cdp" },
			} as CommandResult,
		});
		return;
	}

	try {
		await attachShared(tabId);
		await sendToTab(tabId, "Network.enable", {});
	} catch (err) {
		// Detach the hold we may have taken, then fail loudly.
		await detachShared(tabId).catch(() => {});
		replyError(
			tabId,
			payload.id,
			`failed to arm network capture: ${err instanceof Error ? err.message : String(err)}`,
		);
		return;
	}

	const buffer = new NetworkCaptureBuffer();
	const unsubscribe = onDebuggerEvent((source, method, params) => {
		if (source.tabId !== tabId) return;
		switch (method) {
			case "Network.requestWillBeSent":
				buffer.onRequestWillBeSent(params as never);
				break;
			case "Network.responseReceived":
				buffer.onResponseReceived(params as never);
				break;
			case "Network.loadingFinished": {
				const entry = buffer.onLoadingFinished(params as never);
				if (entry) void recordCompletedRequest(tabId, entry);
				break;
			}
			case "Network.loadingFailed": {
				const entry = buffer.onLoadingFailed(params as never);
				if (entry) void recordNetworkEntry(tabId, entry);
				break;
			}
		}
	});

	const timer = setTimeout(() => void stopNetworkCapture(tabId), durationMs);
	activeNetworkCaptures.set(tabId, { buffer, unsubscribe, timer });

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

async function recordCompletedRequest(
	tabId: number,
	entry: NetworkEntry,
): Promise<void> {
	const { body, bodyTruncated } = await fetchResponseBody(tabId, entry.requestId);
	await recordNetworkEntry(tabId, { ...entry, body, bodyTruncated });
}

// Tear down a capture window: unsubscribe, disable Network, release the shared
// attach. Idempotent — safe to call on an already-stopped tab.
async function stopNetworkCapture(tabId: number): Promise<void> {
	const handle = activeNetworkCaptures.get(tabId);
	if (!handle) return;
	activeNetworkCaptures.delete(tabId);
	clearTimeout(handle.timer);
	handle.unsubscribe();
	try {
		await sendToTab(tabId, "Network.disable", {});
	} catch (err) {
		console.warn("[HTR NControl] Network.disable failed:", err);
	}
	await detachShared(tabId);
}
```

- [x] **Step 3: Route the action in `sendCommandToTab`**

In `sendCommandToTab` (line ~1004), add a branch alongside the existing `getReadyTabs`/`debuggerEval` early-return branches (before the navigation/CDP-input logic):

```typescript
	if (payload.action === "networkCapture") {
		await handleNetworkCapture(tabId, payload);
		return;
	}
```

- [x] **Step 4: Auto-detach on tab removal**

To honor "never a permanent attach" when a tab closes mid-window, ensure the capture is torn down. In `src/background/nativeHost.ts`, find the existing `chrome.tabs.onRemoved` listener if one exists; if not, add this near the module's other `chrome.tabs.*` listeners:

```typescript
chrome.tabs.onRemoved.addListener((tabId) => {
	if (activeNetworkCaptures.has(tabId)) {
		void stopNetworkCapture(tabId);
	}
});
```

(If a `chrome.tabs.onRemoved` listener already exists, add the `stopNetworkCapture` call inside it rather than registering a second listener.)

- [x] **Step 5: Typecheck + Biome**

Run: `bun run check:fix && bun run typecheck`
Expected: no errors. (`Command`/`CommandResult`/`replyError`/`sendToNative` are already in scope in `nativeHost.ts`; confirm the `Command` type import covers `options`.)

- [x] **Step 6: Full extension test suite (regression check)**

Run: `bun run test`
Expected: PASS — no existing tests broken by the `nativeHost.ts` additions.

- [x] **Step 7: Commit**

```bash
git add src/background/nativeHost.ts
git commit -m "feat(extension): arm bounded CDP network capture windows"
```

---

### Task 5: Firefox `webRequest` always-on capture + manifest permission

**Files:**
- Create: `src/background/networkWebRequest.ts`
- Test: `src/background/networkWebRequest.test.ts`
- Modify: `src/background/index.ts`
- Modify: `src/manifest.ts`
- Modify: `firefox/vite.config.ts`

**Interfaces:**
- Consumes: `recordNetworkEntry` (Task 1); `browser.webRequest` (via the `chrome.webRequest` polyfill on Firefox).
- Produces: `mapWebRequestEntry(details): NetworkEntry` (pure, testable) and `startWebRequestCapture(record): void` that registers `onBeforeRequest`/`onCompleted`/`onErrorOccurred` listeners. Metadata-only (no response body — `webRequest` cannot cheaply read bodies).

- [x] **Step 1: Write the failing test**

Create `src/background/networkWebRequest.test.ts`:

```typescript
import { describe, expect, it } from "bun:test";
import { mapWebRequestEntry } from "./networkWebRequest";

describe("mapWebRequestEntry", () => {
	it("maps a completed webRequest detail into a NetworkEntry", () => {
		const entry = mapWebRequestEntry({
			requestId: "42",
			url: "https://example.com/api/ping",
			method: "POST",
			statusCode: 204,
			timeStamp: 1_000,
			startedMs: 950,
		});
		expect(entry).toEqual({
			requestId: "42",
			url: "https://example.com/api/ping",
			method: "POST",
			status: 204,
			durationMs: 50,
		});
	});

	it("omits status and duration when unknown", () => {
		const entry = mapWebRequestEntry({
			requestId: "43",
			url: "https://example.com/x",
			method: "GET",
			timeStamp: 100,
		});
		expect(entry.status).toBeUndefined();
		expect(entry.durationMs).toBeUndefined();
	});
});
```

- [x] **Step 2: Run test to verify it fails**

Run: `bun test src/background/networkWebRequest.test.ts`
Expected: FAIL — `./networkWebRequest` module does not exist.

- [x] **Step 3: Implement `networkWebRequest.ts`**

Create `src/background/networkWebRequest.ts`:

```typescript
/**
 * Firefox network capture via browser.webRequest (exposed as chrome.webRequest
 * through the polyfill). Always-on, metadata-only — webRequest cannot cheaply
 * read response bodies, so NetworkEntry.body is omitted on Firefox. This is the
 * documented Chrome/Firefox asymmetry (see the plan's Global Constraints).
 */

import type { NetworkEntry } from "../types/recording";
import { recordNetworkEntry } from "./eventStore";

interface WebRequestCompleted {
	requestId: string;
	url: string;
	method: string;
	statusCode?: number;
	timeStamp: number; // ms epoch when completed
	startedMs?: number; // ms epoch when the request began (tracked locally)
}

export function mapWebRequestEntry(details: WebRequestCompleted): NetworkEntry {
	const entry: NetworkEntry = {
		requestId: details.requestId,
		url: details.url,
		method: details.method,
	};
	if (typeof details.statusCode === "number") entry.status = details.statusCode;
	if (typeof details.startedMs === "number") {
		entry.durationMs = Math.max(0, Math.round(details.timeStamp - details.startedMs));
	}
	return entry;
}

// Register always-on webRequest observers. tabId comes from webRequest details
// (details.tabId); requests with tabId < 0 (e.g. background/service requests)
// are ignored since the event buffer is per real tab.
export function startWebRequestCapture(): void {
	if (typeof chrome === "undefined" || !chrome.webRequest) return;

	const startTimes = new Map<string, number>();

	chrome.webRequest.onBeforeRequest.addListener(
		(details: { requestId: string; timeStamp: number }) => {
			startTimes.set(details.requestId, details.timeStamp);
		},
		{ urls: ["http://*/*", "https://*/*"] },
	);

	const onDone = (details: {
		requestId: string;
		url: string;
		method: string;
		statusCode?: number;
		timeStamp: number;
		tabId: number;
	}): void => {
		const startedMs = startTimes.get(details.requestId);
		startTimes.delete(details.requestId);
		if (details.tabId < 0) return;
		const entry = mapWebRequestEntry({ ...details, startedMs });
		void recordNetworkEntry(details.tabId, entry);
	};

	chrome.webRequest.onCompleted.addListener(onDone, {
		urls: ["http://*/*", "https://*/*"],
	});
	chrome.webRequest.onErrorOccurred.addListener(onDone, {
		urls: ["http://*/*", "https://*/*"],
	});
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `bun test src/background/networkWebRequest.test.ts`
Expected: PASS

- [x] **Step 5: Start webRequest capture on Firefox only**

In `src/background/index.ts`, near the bottom where the flush loop and `startNativeHost()` are wired (around line 1483), add a guarded start so it is a no-op on Chrome (which uses CDP capture instead):

```typescript
// Firefox: no chrome.debugger, so passive network capture uses always-on
// webRequest observation. Chrome uses bounded CDP capture windows instead
// (see nativeHost.ts handleNetworkCapture), so skip webRequest there.
if (typeof chrome.debugger === "undefined") {
	startWebRequestCapture();
}
```

Add the import near the other `./` imports at the top of `src/background/index.ts`:

```typescript
import { startWebRequestCapture } from "./networkWebRequest";
```

- [x] **Step 6: Add the `webRequest` permission (both manifests)**

In `src/manifest.ts`, add `"webRequest"` to the `permissions` array (after `"debugger"`):

```typescript
		"debugger", // CDP Page.printToPDF for headless PDF capture
		"webRequest", // Firefox passive network capture (observation only)
```

In `firefox/vite.config.ts`, add `"webRequest"` to the manifest `permissions` array (after `"nativeMessaging"`):

```typescript
					"nativeMessaging",
					"webRequest",
```

- [x] **Step 7: Typecheck + Biome + full test suite**

Run: `bun run check:fix && bun run typecheck && bun run test`
Expected: no errors, all tests pass

- [x] **Step 8: Commit**

```bash
git add src/background/networkWebRequest.ts src/background/networkWebRequest.test.ts src/background/index.ts src/manifest.ts firefox/vite.config.ts
git commit -m "feat(extension): Firefox webRequest network capture"
```

---

### Task 6: CLI `network read` / `network watch` / `network wait`

**Files:**
- Create: `htrcli/internal/commands/network.go`
- Test: `htrcli/internal/commands/network_test.go`

**Interfaces:**
- Consumes: `EventPoller{Client, TabID, Kind, Interval}` and its `Read`/`Watch` methods (existing, `htrcli/internal/commands/console.go`); `api.Client.ExecuteCommand`, `api.Client.GetEvents` (existing); `GetClient()`, `GetTabID()`, `UseCDP()`, `errUnsupportedCDP()`, `output.JSONOutput`, `output.PrintJSON`, `output.Warning` (existing).
- Produces: `networkEventData` struct, `network` cobra command with `read`/`watch`/`wait` subcommands. `wait` arms a bounded capture window, then blocks until a matching entry (url glob + optional status) arrives or the timeout elapses.

Note on `EventPoller.Watch`: its real signature is `Watch(ctx context.Context, timeout time.Duration, since int, match func(api.EventEntry) bool, handle func(api.EventsResponse) error) error` (see `console.go`). It loops until the context/timeout ends, calling `handle` per matching batch. To implement early-exit `wait`, pass a cancelable context and cancel it from `handle` on the first match — `Watch` returns `nil` on cancel, and the captured entry distinguishes match-found from timeout.

- [x] **Step 1: Write the failing test**

Create `htrcli/internal/commands/network_test.go`:

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

func networkEvent(seq int, url, method string, status int) api.EventEntry {
	data, _ := json.Marshal(networkEventData{
		RequestID:  "r",
		URL:        url,
		Method:     method,
		Status:     status,
		DurationMs: 12,
	})
	return api.EventEntry{Seq: seq, Kind: "network", Timestamp: 1000, Data: data}
}

func TestNetworkReadFormatsEntries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(api.ApiResponse{
			OK: true,
			Data: api.EventsResponse{
				Entries:            []api.EventEntry{networkEvent(1, "https://x.test/api", "GET", 200)},
				Dropped:            0,
				OldestAvailableSeq: 1,
			},
		})
	}))
	defer server.Close()

	c := api.NewClient(server.URL, "")
	poller := &EventPoller{Client: c, Kind: networkEventKind}
	resp, err := poller.Read(0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := formatNetworkEntries(resp)
	if !strings.Contains(out, "GET") || !strings.Contains(out, "200") || !strings.Contains(out, "https://x.test/api") {
		t.Fatalf("expected method/status/url in output, got: %s", out)
	}
}

func TestNetworkEntryMatches(t *testing.T) {
	entry := networkEvent(1, "https://x.test/api/users?page=2", "GET", 200)
	if !networkEntryMatches(entry, "*/api/users*", 0) {
		t.Fatalf("expected glob match on url")
	}
	if networkEntryMatches(entry, "*/api/orders*", 0) {
		t.Fatalf("did not expect match on non-matching url")
	}
	if !networkEntryMatches(entry, "*/api/users*", 200) {
		t.Fatalf("expected match when status also matches")
	}
	if networkEntryMatches(entry, "*/api/users*", 404) {
		t.Fatalf("did not expect match when status differs")
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `cd htrcli && go test ./internal/commands/... -run 'TestNetworkReadFormatsEntries|TestNetworkEntryMatches' -v`
Expected: FAIL — `networkEventKind`, `networkEventData`, `formatNetworkEntries`, `networkEntryMatches` undefined.

- [x] **Step 3: Implement `network.go`**

Create `htrcli/internal/commands/network.go`:

```go
package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/u007/htrcli/internal/api"
	"github.com/u007/htrcli/internal/output"
)

const networkEventKind = "network"

type networkEventData struct {
	RequestID  string `json:"requestId"`
	URL        string `json:"url"`
	Method     string `json:"method"`
	Status     int    `json:"status,omitempty"`
	DurationMs int    `json:"durationMs,omitempty"`
}

// networkEntryMatches reports whether an entry's url matches the glob (path.Match
// semantics, matched against the whole URL) and, when status > 0, its status
// equals that code.
func networkEntryMatches(entry api.EventEntry, urlGlob string, status int) bool {
	var data networkEventData
	if err := json.Unmarshal(entry.Data, &data); err != nil {
		return false
	}
	if status > 0 && data.Status != status {
		return false
	}
	if urlGlob == "" {
		return true
	}
	ok, err := path.Match(urlGlob, data.URL)
	if err != nil || !ok {
		// path.Match does not span '/'; also try a contains-style match so
		// "*/api/users*" behaves intuitively against full URLs.
		return globContains(urlGlob, data.URL)
	}
	return true
}

// globContains implements a simple '*'-wildcard containment match that, unlike
// path.Match, treats '*' as spanning any character including '/'.
func globContains(pattern, s string) bool {
	parts := strings.Split(pattern, "*")
	pos := 0
	for i, part := range parts {
		if part == "" {
			continue
		}
		idx := strings.Index(s[pos:], part)
		if idx < 0 {
			return false
		}
		if i == 0 && !strings.HasPrefix(pattern, "*") && idx != 0 {
			return false
		}
		pos += idx + len(part)
	}
	if !strings.HasSuffix(pattern, "*") && len(parts) > 0 {
		last := parts[len(parts)-1]
		return strings.HasSuffix(s, last)
	}
	return true
}

func formatNetworkEvent(entry api.EventEntry) string {
	var data networkEventData
	if err := json.Unmarshal(entry.Data, &data); err != nil {
		return fmt.Sprintf("[seq %d] <unparseable network entry>", entry.Seq)
	}
	status := "—"
	if data.Status > 0 {
		status = fmt.Sprintf("%d", data.Status)
	}
	return fmt.Sprintf("[seq %d] %s %s %s (%dms)", entry.Seq, status, data.Method, data.URL, data.DurationMs)
}

func formatNetworkEntries(resp *api.EventsResponse) string {
	if resp == nil {
		return ""
	}
	var b strings.Builder
	if resp.Dropped > 0 {
		fmt.Fprintf(&b, "%s %d events were evicted (buffer cap reached)\n", output.Warning("⚠"), resp.Dropped)
	}
	for _, entry := range resp.Entries {
		b.WriteString(formatNetworkEvent(entry))
		b.WriteByte('\n')
	}
	return b.String()
}

// armNetworkCapture asks the extension to open a bounded Chrome capture window
// (a no-op ack on Firefox, where webRequest capture is always-on).
func armNetworkCapture(tabID *int, durationMs int) error {
	_, err := GetClient().ExecuteCommand(tabID, api.Command{
		ID:      "1",
		Action:  "networkCapture",
		Options: map[string]any{"durationMs": durationMs},
	})
	return err
}

var (
	networkSince      int
	networkTimeoutMS  int
	networkWaitURL    string
	networkWaitStatus int
)

// networkCmd is the canonical `network` command group. This plan owns it
// (per team arbitration: passive capture is Phase 2, ahead of the Phase 3
// mock plan). Other network subcommands (network mock/block/unmock, added by
// 2026-07-24-htrcli-network-mock.md) attach to this var with
// networkCmd.AddCommand(...) in their own init() and must NOT redefine it or
// re-register it on rootCmd.
var networkCmd = &cobra.Command{
	Use:   "network",
	Short: "Read and watch captured network requests",
}

var networkReadCmd = &cobra.Command{
	Use:   "read",
	Short: "Read buffered network entries",
	RunE: func(cmd *cobra.Command, args []string) error {
		if UseCDP() {
			return errUnsupportedCDP("network read")
		}
		tabID, err := GetTabID()
		if err != nil {
			return err
		}
		poller := &EventPoller{Client: GetClient(), TabID: tabID, Kind: networkEventKind}
		resp, err := poller.Read(networkSince)
		if err != nil {
			return err
		}
		if output.JSONOutput {
			output.PrintJSON(resp)
			return nil
		}
		fmt.Print(formatNetworkEntries(resp))
		return nil
	},
}

var networkWatchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Arm capture and stream network entries until timeout",
	RunE: func(cmd *cobra.Command, args []string) error {
		if UseCDP() {
			return errUnsupportedCDP("network watch")
		}
		tabID, err := GetTabID()
		if err != nil {
			return err
		}
		if err := armNetworkCapture(tabID, networkTimeoutMS); err != nil {
			return err
		}
		poller := &EventPoller{Client: GetClient(), TabID: tabID, Kind: networkEventKind}
		handle := func(resp api.EventsResponse) error {
			if output.JSONOutput {
				output.PrintJSON(resp)
				return nil
			}
			fmt.Print(formatNetworkEntries(&resp))
			return nil
		}
		timeout := time.Duration(networkTimeoutMS) * time.Millisecond
		return poller.Watch(cmd.Context(), timeout, networkSince, nil, handle)
	},
}

var networkWaitCmd = &cobra.Command{
	Use:   "wait",
	Short: "Arm capture and block until a matching request completes",
	RunE: func(cmd *cobra.Command, args []string) error {
		if UseCDP() {
			return errUnsupportedCDP("network wait")
		}
		if networkWaitURL == "" {
			return fmt.Errorf("--url is required (glob pattern to match against request URLs)")
		}
		tabID, err := GetTabID()
		if err != nil {
			return err
		}
		if err := armNetworkCapture(tabID, networkTimeoutMS); err != nil {
			return err
		}
		poller := &EventPoller{Client: GetClient(), TabID: tabID, Kind: networkEventKind}

		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()
		var matched *api.EventEntry
		match := func(entry api.EventEntry) bool {
			return networkEntryMatches(entry, networkWaitURL, networkWaitStatus)
		}
		handle := func(resp api.EventsResponse) error {
			if matched == nil && len(resp.Entries) > 0 {
				e := resp.Entries[0]
				matched = &e
				cancel()
			}
			return nil
		}
		timeout := time.Duration(networkTimeoutMS) * time.Millisecond
		if err := poller.Watch(ctx, timeout, networkSince, match, handle); err != nil {
			return err
		}
		if matched == nil {
			return fmt.Errorf("no request matching %q%s arrived within %dms", networkWaitURL, statusSuffix(networkWaitStatus), networkTimeoutMS)
		}
		if output.JSONOutput {
			output.PrintJSON(matched)
			return nil
		}
		fmt.Println(formatNetworkEvent(*matched))
		return nil
	},
}

func statusSuffix(status int) string {
	if status > 0 {
		return fmt.Sprintf(" (status %d)", status)
	}
	return ""
}

func init() {
	networkReadCmd.Flags().IntVar(&networkSince, "since", 0, "cursor to read after")
	networkWatchCmd.Flags().IntVar(&networkSince, "since", 0, "cursor to watch after")
	networkWatchCmd.Flags().IntVar(&networkTimeoutMS, "timeout", 10000, "capture/watch window in ms")
	networkWaitCmd.Flags().IntVar(&networkSince, "since", 0, "cursor to wait after")
	networkWaitCmd.Flags().IntVar(&networkTimeoutMS, "timeout", 10000, "how long to wait, in ms")
	networkWaitCmd.Flags().StringVar(&networkWaitURL, "url", "", "glob pattern to match against request URLs")
	networkWaitCmd.Flags().IntVar(&networkWaitStatus, "status", 0, "also require this HTTP status code")

	networkCmd.AddCommand(networkReadCmd)
	networkCmd.AddCommand(networkWatchCmd)
	networkCmd.AddCommand(networkWaitCmd)
	rootCmd.AddCommand(networkCmd)
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `cd htrcli && go test ./internal/commands/... -run 'TestNetworkReadFormatsEntries|TestNetworkEntryMatches' -v`
Expected: PASS

- [x] **Step 5: Full commands suite (regression check)**

Run: `cd htrcli && go test ./internal/commands/... -v`
Expected: PASS

- [x] **Step 6: Build the CLI**

Run: `make htrcli-build`
Expected: builds `htrcli/bin/htrcli` with no errors; `htrcli/bin/htrcli network --help` lists `read`, `watch`, `wait`.

- [x] **Step 7: Commit**

```bash
git add htrcli/internal/commands/network.go htrcli/internal/commands/network_test.go
git commit -m "feat(htrcli): add network read/watch/wait CLI commands"
```

---

### Task 7: End-to-end verification (Chrome + Firefox)

**Files:** none (verification only)

- [x] **Step 1: Chrome capture window**

Run `htrcli serve`, load the unpacked Chrome build (`bun run build`), open a test page with a `fetch` on a button. In one terminal run `htrcli network watch --timeout 8000`; while it runs, click the button to trigger the fetch. Confirm the watch prints a line with the request's status, method, URL, and a duration. Then run `htrcli network read --json` and confirm the same entry is in the buffer with a `body` field for a text/JSON response.

- [x] **Step 2: Chrome `network wait`**

Run `htrcli network wait --url "*/api/*" --status 200 --timeout 8000`, then trigger a matching request. Confirm the command exits 0 and prints the matched entry. Repeat with a `--url` that never matches and confirm it exits non-zero with the "no request matching …" error after the timeout.

- [x] **Step 3: Chrome no-attach-leak check**

After a `watch`/`wait` window ends, confirm Chrome's "This tab is being debugged" banner disappears (the debugger detached). Run `htrcli network read` with no preceding `watch` and confirm it returns nothing new — documenting the Chrome behavior that `read` only surfaces traffic captured during an armed window.

- [x] **Step 4: Firefox always-on capture**

Load the Firefox build (`bun run firefox:build`, then `about:debugging`). Without any `watch`, trigger a fetch on the page, then run `htrcli network read`. Confirm the entry appears (Firefox capture is always-on, metadata-only — no `body` field). This is the documented Chrome/Firefox asymmetry.

- [x] **Step 5: Record the deferred/limitation items in TODO.md**

Add to `TODO.md` in the project root:

```markdown
## htrcli network capture — deferred / known limitations
- Response bodies are captured on Chrome (CDP getResponseBody, 64KB cap) but NOT on Firefox (webRequest cannot cheaply read bodies). Firefox network entries are metadata-only.
- The shared debuggerManager is used only by network + dialog capture. The existing trusted-input (cdpInput.ts) and CDP_EVAL paths still do their own chrome.debugger.attach, so running a click/eval command while a capture window is open on the same tab fails with "Already attached". Route those through debuggerManager in a later pass.
- Network mocking/interception (spec §1b) is a separate deferred phase (needs webRequestBlocking + Fetch.enable).
```

Commit:

```bash
git add TODO.md
git commit -m "docs: record htrcli network capture limitations"
```

---

## Self-Review

**1. Spec coverage (§1a "Passive capture + `waitForResponse`"):**
- `htrcli network read [--since N] [--json]` → Task 6 `networkReadCmd`. ✓
- `htrcli network watch [--timeout N] [--json]` → Task 6 `networkWatchCmd` (arms + streams). ✓
- `htrcli network wait --url <glob> [--status N] [--timeout N]` → Task 6 `networkWaitCmd`. ✓
- Chrome via CDP `Network.requestWillBeSent`/`responseReceived`/`loadingFinished` through `chrome.debugger` → Tasks 3+4. ✓ (Note: the spec text names `internal/cdp/network.go` for the CDP-transport path; this plan targets the **extension** transport as instructed by the task brief, using `chrome.debugger` in the background rather than the Go CDP session. The Go CDP-transport variant is out of scope for this phase and guarded by `errUnsupportedCDP`.)
- Firefox via `browser.webRequest.onBeforeRequest`/`onCompleted`, same `NetworkEntry` shape → Task 5. ✓
- `network wait` as a CLI-side filtered `EventPoller.Watch` call, no new server primitive → Task 6 (reuses `GET /api/events`; no `server.go` change). ✓
- Entries flow through the SAME `EventStore`/`eventStore.ts` buffer as console (`kind="network"`) → Task 1. ✓
- `NetworkEntry` shape from the spec (requestId, url, method, status, headers, bodyTruncated, body, durationMs) → Task 1 type + Task 4 body capture. ✓

**2. Placeholder scan:** No "TBD"/"add error handling"/"similar to Task N". Every code step is complete. The one "Files: none" tasks are verification-only by design. Error handling is concrete (loud `replyError`, `console.warn` with context per the no-silent-swallow rule).

**3. Type consistency:** `networkEventData` (Go) fields (`requestId/url/method/status/durationMs`) match `NetworkEntry` (TS) JSON keys. `EventKind`/`recordNetworkEntry`/`recordEvent` names are consistent across Tasks 1, 4, 5. `EventPoller.Watch`'s real 5-arg signature is used correctly in Task 6 (verified against `console.go`). `networkCapture` action name matches between the Go `armNetworkCapture` (Task 6) and the background `sendCommandToTab` branch (Task 4). `attachShared`/`detachShared`/`onDebuggerEvent`/`sendToTab` names match between Tasks 2 and 4.

**Judgment calls made:** (a) Response-body capture is Chrome-only (Firefox webRequest limitation, documented, not false parity). (b) Bounded capture windows with auto-detach reconcile passive capture against the "no permanent attach" constraint (flagged to the team lead). (c) A refcounted `debuggerManager` is introduced (owned here, reused by the dialog plan) to prevent network+dialog attach conflicts; existing one-shot attaches are intentionally left alone and the conflict documented in TODO.md.
