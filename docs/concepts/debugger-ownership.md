---
type: Concept
title: Debugger ownership — never attach chrome.debugger outside attachShared/detachShared
description: "The single-owner rule for chrome.debugger in the extension service worker: refcounted attach, coalescing of concurrent attaches, rollback on a failed attach, stale-attach recovery, networkMock re-arm bookkeeping, and the call sites (trusted input, upload, PDF, CDP navigate, CDP eval, network mock) that must not attach directly."
resource: src/background/debuggerManager.ts; src/background/networkMock.ts; src/background/nativeHost.ts; src/background/cdpInput.ts; src/background/index.ts
tags: [extension, background, debugger, cdp, concurrency, refcount]
timestamp: 2026-11-19T03:54:00Z
status: active
---

# Debugger ownership

`chrome.debugger` is a single-tenant resource per tab, and the extension has several features that want it: trusted input, uploads, print-to-PDF, CDP navigation, CDP eval, network mocking, and dialog handling. Every `chrome.debugger.attach`/`detach` in `src/background` goes through `attachShared`/`detachShared` in `src/background/debuggerManager.ts`. No other file calls `chrome.debugger.attach` or `chrome.debugger.detach` (verified by grep, 2026-11-19).

## Why the manager exists

- Two features can need the tab at once (a capture in flight while a command runs). A refcount lets them share one real attach.
- Two *concurrent* callers must not both attempt a real attach: the loser gets `Another debugger is already attached`, and its caller's compensating `detachShared` would then release the **winner's** hold and kill a live debugger.
- A command the caller abandons (client timeout, closed tab) can leave a real attach with no refcount, and every later command then fails until the tab reloads.

## attachShared contract

- The hold is reserved **synchronously** before awaiting (`refcounts.set(tabId, 1)`), so a concurrent caller sees it instead of starting a second attach.
- Concurrent callers share one in-flight promise (`attaching`); each resolved caller adds one to the refcount.
- A real attach goes through `attachWithStaleRecovery`: on `another debugger is already attached` it detaches once, retries once, and otherwise throws verbatim. Detaching only releases *our* session, so a user's DevTools keeps the error.
- A failed attach rolls the reservation back (`refcounts.delete(tabId)`) and logs `[HTR NControl] debugger attach failed:`. Rollback means a caller that detaches in its own catch block is a no-op, not a release of somebody else's hold.
- `ensureSharedListener()` runs **inside** the try, so its failure follows the same rollback instead of leaving a reservation behind.
- The shared `onEvent` listener is attached once and fans out to subscribers (`onDebuggerEvent`), so one real attach serves network, dialog and mock listeners.

## detachShared contract

- Releases exactly one hold. The real `chrome.debugger.detach` runs only when the last holder releases.
- A detach failure (the tab may have closed, which auto-detaches) is logged at warn, never swallowed silently.
- Every `attachShared` has a matching `detachShared` in a `finally`; no code path holds a debugger attach permanently.

## networkMock re-arm bookkeeping

`armFetch`/`disarmFetch` add a second layer of bookkeeping on top of the refcount:

- `armedTabs` records tabs whose mock owns a shared hold, so re-arming does not attach twice.
- `armPromises` coalesces two concurrent arms into one `Fetch.enable`; the second awaits the first.
- If `Fetch.enable` fails, the arm rolls back only when it was the one that attached (`newlyAttached`), so a failed re-arm cannot detach a hold another feature owns.

## Call sites

| Path | File |
|---|---|
| Trusted input (`attachShared` before dispatch) | `src/background/cdpInput.ts` |
| Upload files (`DOM.setFileInputFiles`) | `src/background/nativeHost.ts` |
| Native-host eval | `src/background/nativeHost.ts` |
| `PRINT_TO_PDF` | `src/background/index.ts` |
| `CDP_NAVIGATE` | `src/background/index.ts` |
| `CDP_EVAL` | `src/background/index.ts` |
| `armFetch`/`disarmFetch` | `src/background/networkMock.ts` |

`chrome.debugger.sendCommand` is still called directly by these paths; only attach/detach ownership is centralized.

## Validation

- `src/background/debuggerManager.test.ts` — balanced refcount (one attach / one detach), event fan-out until unsubscribe, dispatch during an open capture does not throw `Already attached`, two overlapping attaches coalesce into one real attach, stale-attach recovery (detach + one retry), a non-stale error does not detach, and a failed attach rolls the hold back so `detachShared` adds no extra detach.
- Batch gates (2026-11-19): `bun run check` (215 files), `bun test` (632 pass / 0 fail, 40 files), `go vet ./...`, `go test ./...`, `bun run build`, `bun run firefox:build`, `bun run cdp-bundle:check`, `git diff --check`.
