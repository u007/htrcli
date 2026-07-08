# Part 3: Trusted Input via CDP (Chrome)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** On Chrome, `click`, `pressKey`, and `type` produce **trusted** input events (CDP `Input.dispatchMouseEvent` / `Input.dispatchKeyEvent` / `Input.insertText`) so browser default actions fire — Enter submits forms, clicks pass `isTrusted` checks, focus/selection behave natively. Firefox keeps the synthetic path, upgraded with pointer events.

**Architecture:** Input synthesis moves to the background service worker, which already holds the `debugger` permission and the attach/detach pattern (see `handleDebuggerEval` in `src/background/nativeHost.ts`). The content script's job shrinks to *preparation*: wait for the element to be actionable, scroll it into view, focus it (for keys), and report its viewport-center coordinates. Both transports converge on the same background handlers — the native-host path calls them directly inside its dispatch; the server/WS path reaches them via `chrome.runtime.sendMessage` relays, the established pattern used by `CDP_NAVIGATE`/`PRINT_TO_PDF`. On Firefox (`typeof chrome.debugger === "undefined"`), the content script executes the synthetic path itself, exactly as today plus pointer events.

**Tech Stack:** CDP 1.3 `Input` domain via `chrome.debugger`; content-script preparation reuses `waitForActionableElement` (delivered before this part: it waits for an element to exist, be visible, and optionally be enabled, throwing a condition-naming error on timeout).

## Global Constraints (inherited, restated)

- Chrome-only APIs must gate on `typeof chrome.debugger === "undefined"` with a working Firefox path; `bun run firefox:typecheck` must pass.
- Attach/detach per command (existing convention in `handleDebuggerEval` and `CDP_NAVIGATE`); always detach in a finally-style path.
- No silent fallbacks: if debugger attach fails on Chrome (typically DevTools is open on that tab, or another client is attached), the command fails with an error that says exactly that — it does not quietly downgrade to synthetic events.
- All waits within the 25s internal budget.
- Verify after the part: `bunx tsc --noEmit`, `bun test`, `bun run build`, `bun run firefox:typecheck`.

**Known UX tradeoff (accepted, documented in Task 3.5):** while attached, Chrome shows the "HTR NControl is debugging this browser" infobar. This already occurs for `printToPDF`/`debuggerEval`/`cdpNavigate`; this part makes it appear during clicks and typing too.

---

### Task 3.1: Key mapping table

**Files:**
- Create: `src/utils/keyMap.ts`
- Test: `src/utils/keyMap.test.ts`

**Interfaces:**
- Produces: `resolveKey(key: string)` returning the CDP key descriptor — `key`, `code`, `windowsVirtualKeyCode`, `text` (for printable keys) — plus a `isPrintable` flag. Covers: all printable ASCII (letters, digits, symbols, space), and named keys `Enter`, `Tab`, `Escape`, `Backspace`, `Delete`, `ArrowUp/Down/Left/Right`, `Home`, `End`, `PageUp`, `PageDown`. Unknown keys throw with the offending name. This is the single source of truth for both the CDP path and the synthetic-event fallback (which today emits wrong `code` values like `KeyEnter`).

**Steps:**

- [ ] Write failing tests: Enter maps to code `Enter` with VK 13 and text `\r`; letters map to `KeyX` codes with correct shift-less VKs; digits map to `Digit5`-style codes; space maps to `Space`; ArrowDown maps to VK 40 and is non-printable; unknown key name throws.
- [ ] Implement the table; tests pass.
- [ ] `bun run check:fix`, commit: `feat: CDP key descriptor table`.

---

### Task 3.2: Content-script preparation actions

**Files:**
- Modify: `src/contentScript/commandExecutor.ts`
- Modify: `src/types/commands.ts` and `server/types.ts` (add the two new internal actions to the action union, keeping the files identical)
- Test: `src/contentScript/commandExecutor.test.ts`

**Interfaces:**
- Consumes: `waitForActionableElement`.
- Produces: two internal actions the background invokes on the content script:
  - `prepareClick` — waits actionable (visible + enabled), scrolls into view (instant, centered), returns the element's viewport-center `x`/`y` (post-scroll) plus the page's devicePixelRatio-independent CSS coordinates CDP expects.
  - `prepareKeys` — waits actionable, scrolls, focuses the element, returns confirmation that focus landed (activeElement check).
  Both honor `options.timeout` like other interaction actions.

**Steps:**

- [ ] Write failing DOM tests: `prepareClick` returns center coordinates for a visible button and fails with the actionability error for a hidden one; `prepareKeys` leaves the target focused.
- [ ] Implement both handlers and register them in the action router; keep the existing `click`/`pressKey`/`type` synthetic handlers untouched (they remain the Firefox executors).
- [ ] Full verify, `bun run check:fix`, commit: `feat: prepareClick/prepareKeys content-script actions`.

---

### Task 3.3: Background CDP input dispatch + native-host routing

**Files:**
- Modify: `src/background/nativeHost.ts`
- Test: `src/background/cdpInput.test.ts` (event-sequence construction tested with an injected sendCommand stub)

**Interfaces:**
- Consumes: `resolveKey` from Task 3.1; `prepareClick`/`prepareKeys` from Task 3.2; the attach/detach pattern from `handleDebuggerEval`.
- Produces: `dispatchCdpClick(tabId, command)`, `dispatchCdpKey(tabId, command)`, `dispatchCdpType(tabId, command)`. Behavior:
  - click: ask the content script to `prepareClick` (element wait + coords), then attach and send `mousePressed`+`mouseReleased` at those coords with the requested button and clickCount (double-click = clickCount 2, single press/release pair per CDP convention).
  - pressKey: `prepareKeys` first, then keyDown (with text for printables) + keyUp built from `resolveKey`.
  - type: `prepareKeys` first, then `Input.insertText` with the whole string (matches how IMEs insert text; per-char key events are not needed for value entry).
  - In `sendCommandToTab`, route `click`/`dblclick`/`rightclick`/`pressKey`/`type` to these dispatchers **only when `chrome.debugger` exists**; otherwise fall through to the existing content-script message (Firefox). Attach failure → explicit error result mentioning DevTools/another debugger.
  - Ordering with Part 2: the triggered-navigation watcher must wrap the whole prepare→dispatch sequence, and CDP must detach *before* the settle wait begins (a navigation can kill the attach).
- **Element-targetless coordinates:** if the command carries explicit coordinates in options (future-proofing), skip prepare — out of scope now; require a target.

**Steps:**

- [ ] Write failing tests for event-sequence construction with a stubbed CDP sender: click produces pressed/released pairs with correct button names and clickCounts for the three click variants; pressKey Enter produces keyDown with text `\r` then keyUp; type produces a single insertText call.
- [ ] Implement the three dispatchers and the routing gate; tests pass.
- [ ] Manual verification on Chrome (extension reloaded, daemon running): (a) `htcli fill` a search box then `htcli press Enter` — the form actually submits (this fails on the current synthetic path); (b) `htcli click` on a button whose handler checks `event.isTrusted` — handler runs; (c) with DevTools open on the tab, `htcli click` returns the explicit attach-failure error.
- [ ] Full verify, `bun run check:fix`, commit: `feat: trusted click/key/type input via CDP on Chrome`.

---

### Task 3.4: Server/WS path relay + Firefox synthetic upgrade

**Files:**
- Modify: `src/contentScript/commandExecutor.ts` (click/pressKey/type handlers)
- Modify: `src/background/index.ts` (three new runtime-message handlers mirroring the Task 3.3 dispatchers, following the `CDP_NAVIGATE` handler pattern including its Firefox guard)
- Test: `src/contentScript/commandExecutor.test.ts`

**Interfaces:**
- Consumes: Task 3.3 dispatchers (exposed to `background/index.ts` as exported functions), `resolveKey`.
- Produces: when the content script receives `click`/`pressKey`/`type` over the WS path on Chrome, it relays to the background via `chrome.runtime.sendMessage` (new message types added to the `MessageType` union in `src/types/recording.ts`, one interface each, per project convention) and returns the background's result. On Firefox — detected in the content script by asking the background once whether CDP is available, or by the relay responding "unsupported" — the content script runs the synthetic path. The synthetic click sequence gains `pointerover`/`pointerdown`/`pointerup` events interleaved in correct order before their mouse counterparts, and the synthetic pressKey/type paths take their `key`/`code` values from `resolveKey` instead of the hand-built `Key${x}` strings.
- Async listener rule: every new `onMessage` handler that responds asynchronously returns `true` (project convention).

**Steps:**

- [ ] Write failing DOM tests for the synthetic upgrade: click on an element records `pointerdown` before `mousedown` and `pointerup` before `mouseup`; synthetic Enter keydown carries code `Enter` (not `KeyEnter`).
- [ ] Implement the relay message types, background handlers, and synthetic upgrades; tests pass.
- [ ] Manual verification with the Bun server path (`bun run server`, extension connected over WS): trusted Enter-submits works via `curl` to `/api/command`.
- [ ] Full verify including `bun run firefox:build`, `bun run check:fix`, commit: `feat: trusted input over WS path; pointer-event synthetic fallback for Firefox`.

---

### Task 3.5: Documentation

**Files:**
- Modify: `skills/htrcli/SKILL.md` — note that on Chrome, click/press/type are trusted input (Enter submits forms); note the debugger infobar; note the DevTools-open failure mode and its error message.
- Modify: `htcli/README.md` — same, one paragraph.

**Steps:**

- [ ] Update docs; commit: `docs: trusted input semantics and caveats`.
