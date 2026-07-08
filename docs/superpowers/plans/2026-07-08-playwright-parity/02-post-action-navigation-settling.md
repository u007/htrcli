# Part 2: Post-Action Navigation Settling

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When a click-class action *triggers* a navigation (link click, form submit via Enter, modal button that routes), the command does not return until the destination page has finished loading — the same guarantee `navigate`/`reload`/`goBack`/`goForward` already provide.

**Architecture:** Both transports already contain a load-wait primitive from the navigation-wait work; this part extracts and reuses each one. In the background native-host relay (`src/background/nativeHost.ts`), the existing `navigateAndWaitForLoad` logic is split so its "wait for `tabs.onUpdated` status complete" half can run after any action: attach a listener before dispatching the action, and if a `loading` transition for that tab is observed by shortly after the result returns, keep waiting for `complete`. In the Bun server (`server/index.ts`), the existing `waitForNavigation` poller is reused: after a click-class result, a short settle window watches for the tab's WebSocket to drop or its URL to change; if either happens, hand off to `waitForNavigation`.

**Tech Stack:** TypeScript; `chrome.tabs.onUpdated` (already-granted `tabs` permission — no manifest change); Bun server WS lifecycle.

## Global Constraints (inherited, restated)

- Settle waits must fit inside the 25s internal budget / 30s transport ceilings.
- The settle window for "did this action start a navigation?" must be short (~500ms) so non-navigating actions aren't slowed noticeably.
- No silent fallbacks: if a navigation starts but never completes, the command fails with an error naming the action and the timeout.
- Firefox: `tabs.onUpdated` works via the polyfill — no Chrome-only APIs in this part.
- Verify after the part: `bunx tsc --noEmit`, `bun test`, `bun run build`, `bun run firefox:typecheck`, and `cd server && bun test`.

**Which actions get settling (both transports, single shared list):** `click`, `dblclick`, `rightclick`, `pressKey`. Define the set once per file next to the existing `NAV_ACTIONS` set with a comment explaining membership (actions whose page-side default behavior can start a navigation). `fill`/`check`/`select` are excluded — they do not navigate on their own; Enter-to-submit goes through `pressKey`.

---

### Task 2.1: Background relay — watch-and-settle helper

**Files:**
- Modify: `src/background/nativeHost.ts`
- Test: `src/background/navigationWatch.test.ts` (new; pure-logic tests of the state machine, with the chrome APIs injected as stubs)

**Interfaces:**
- Consumes: the existing `NAV_LOAD_TIMEOUT_MS` constant and the onUpdated-listener pattern from `navigateAndWaitForLoad`.
- Produces: a `watchForTriggeredNavigation(tabId)` function returning a handle with two operations: `settle(windowMs): Promise<"none" | "completed">` and `cancel()`. The handle records `tabs.onUpdated` events for the tab from the moment of creation (listener attached *before* the action is dispatched, so a fast navigation is never missed). `settle` resolves `"none"` if no `loading` transition was seen within the window, otherwise waits for `complete` (bounded by `NAV_LOAD_TIMEOUT_MS`) and resolves `"completed"`; it rejects if the load never completes. Extract the "wait for complete" logic shared with `navigateAndWaitForLoad` into one internal function so there is a single implementation.

**Steps:**

- [ ] Design the helper so the chrome event surface is injectable (pass listener add/remove functions in), making it unit-testable without a browser. Write failing tests: (a) no events during the window → `"none"` promptly; (b) `loading` then `complete` → `"completed"`; (c) `loading` seen but no `complete` before the timeout → rejection with the action-naming error; (d) events for other tab IDs are ignored; (e) `cancel` removes the listener.
- [ ] Implement; tests pass; refactor `navigateAndWaitForLoad` to share the internal wait-for-complete function (its five existing behaviors — full load, same-document URL change, timeout, initiate-failure, fast-load-no-miss — must be preserved; re-run the manual nav check afterwards).
- [ ] Full verify, `bun run check:fix`, commit: `feat: background watcher for action-triggered navigations`.

---

### Task 2.2: Background relay — wire into command dispatch

**Files:**
- Modify: `src/background/nativeHost.ts` (`sendCommandToTab`)

**Interfaces:**
- Consumes: `watchForTriggeredNavigation` from Task 2.1; the settling-action set defined in this part's preamble.
- Produces: for settling actions, `sendCommandToTab` creates the watcher before messaging the content script, and after a successful result calls `settle(500)`. On `"completed"`, the result is forwarded with its duration updated to include the load time. On settle rejection, the relayed result becomes a failure whose error explains that the click started a navigation that never finished loading. Non-settling actions are untouched. The content-script-not-ready retry path (`sendMsg` returning null → inject → retry) must create the watcher only once, before the first attempt.

**Steps:**

- [ ] Wire the watcher into `sendCommandToTab` per the interface block; ensure `cancel()` runs on every early-exit path (element error, tab unavailable) so listeners never leak.
- [ ] Manual verification (requires extension reload): via htcli against a real page — (a) click a normal button: response time unchanged (±500ms); (b) click a link: command returns only after the destination renders, and an immediately following `snapshot` reflects the new page; (c) press Enter in a search box: same.
- [ ] Full verify, `bun run check:fix`, commit: `feat: click-triggered navigations wait for page load (native host path)`.

---

### Task 2.3: Bun server — settle window + reuse of waitForNavigation

**Files:**
- Modify: `server/index.ts` (`sendCommandToTab`)
- Test: `server/navigationSettle.test.ts` (new; test the decision function in isolation)

**Interfaces:**
- Consumes: existing `waitForNavigation(tabId, command, urlBefore, startedAt, timeoutMs)`, `dispatchCommand`, `connectedTabs` (with `registeredAt`), the settling-action set.
- Produces: for settling actions, after a successful command result the server takes the pre-action URL (from the result's `pageInfo.url`, which the content script reports with every result) and enters a ~500ms settle window: it polls connection state briefly; if the tab's socket dropped, the tab re-registered, or a quick `getPageInfo` shows a changed URL, it hands off to `waitForNavigation` and returns that outcome; otherwise it returns the original result unchanged. Factor the "did a navigation start?" decision into a named function taking plain data (connection snapshot, urls, timestamps) so it is unit-testable without sockets.

**Steps:**

- [ ] Write failing tests for the decision function: socket-dropped → settle; re-registered-after-dispatch → settle; URL changed → settle; nothing changed → pass-through.
- [ ] Implement the decision function and wire it into `sendCommandToTab`; tests pass.
- [ ] Manual verification with the Bun server running (`bun run server`) and the extension connected over WS: click a link via `curl` to `/api/command` — the HTTP response arrives after the destination loads, and `pageInfo.url` in the response is the destination URL.
- [ ] Full verify (`cd server && bun test` plus root suite), `bun run check:fix`, commit: `feat: click-triggered navigations wait for page load (server path)`.

---

### Task 2.4: Documentation

**Files:**
- Modify: `skills/htrcli/SKILL.md` — the note added with the navigation-wait work ("Clicks that trigger a navigation do NOT wait…") is now wrong; replace it with the new guarantee and remove the readyState-polling workaround advice.
- Modify: `htcli/README.md` — extend the navigation-wait paragraph to cover click/pressKey-triggered navigations.

**Steps:**

- [ ] Update both docs; grep both files for `readyState` workaround mentions and stale claims.
- [ ] Commit: `docs: click-triggered navigation settling`.
