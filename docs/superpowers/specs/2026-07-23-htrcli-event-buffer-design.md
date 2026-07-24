# htrcli Durable Event Buffer (Console Capture Phase) — Design

**Date:** 2026-07-23
**Status:** Approved (pending review of written spec)

## Problem

htrcli has no way to capture "arm before it happens, read later" streams from a page — console output, network requests, dialogs. Unlike the existing request/response commands (`click`, `fill`, `eval`), these are events the page fires on its own schedule; the CLI needs to read them after the fact, not synchronously request them.

This design covers the **foundational buffer mechanism plus its first consumer, console log capture** — the cheapest end-to-end slice that proves the mechanism, since it needs no CDP and works identically on Chrome and Firefox. Network and dialog capture (a later phase) reuse this buffer unchanged.

The mechanism must survive a Chrome MV3 service-worker restart. The background script is not a long-lived process — Chrome kills and restarts it on idle/memory pressure — so an in-memory-only buffer would silently lose events between a page firing them and the CLI reading them.

## Goals

- A per-tab, per-kind (console/network/dialog) event buffer that the CLI can poll with a cursor (`since=<seq>`), never "return everything."
- Durable across service-worker restarts: `chrome.storage.session` / `browser.storage.session` (Firefox 115+) backs the buffer, not a plain in-memory `Map`.
- Bounded: count-capped per tab per kind (default 500), oldest evicted first. This also bounds `storage.session` usage (~10MB default quota on Chrome, extendable via the `unlimitedStorage` permission) — console args are small, so 500 entries/tab/kind is nowhere near the quota in this phase, but the cap is what protects it once network capture (larger payloads) lands later.
- Honest about loss: eviction is never silent. Every poll response reports how many entries the *requesting client* missed since its own `since` cursor, so `read`/`watch` never look complete when they aren't (this codebase's standing rule against silent truncation — see CLAUDE.md). This is computed per-request from `since` vs. `oldestAvailableSeq`, not a replayed global counter (see Data model / Error handling).
- Durable across a **daemon** restart too, not just the service worker: on daemon startup, it resyncs from the extension rather than starting from an empty in-memory store (see Error handling).
- First consumer: console log capture (`console.log/warn/error/info/debug`), identical on Chrome and Firefox, via a MAIN-world injected script — no `chrome.debugger`/CDP dependency.

## Non-goals

- Network request/response capture and dialog capture — same buffer, different producers, built in a later phase once this foundation lands.
- Guaranteed zero event loss. A message lost in-flight between the page and the background script exactly as the service worker is killed is an accepted small gap; solving that would need a page-side ack/retry protocol, out of scope here.
- CDP-transport event buffering — `internal/cdp/session.go`'s existing per-session event dispatcher is a separate, already-durable-enough mechanism (the CDP session itself doesn't survive restarts the same way); this design covers only the extension/native-messaging transport's buffer. A later phase adds a matching `Session.Events(domain, since)` accessor so the CLI-facing shape is uniform across transports, but that accessor is out of scope for this design.
- Log rotation / archival beyond the count cap. Once evicted, an entry is gone; there is no export-to-disk step in this phase (trace export, in a much later phase, is a separate aggregation feature).

## Architecture

```
Page (MAIN world)             Background service worker                    Daemon (:3845)                CLI
consoleCapture.ts    ──msg──▶  eventStore.ts                        ──HTTP POST──▶ /api/events/ingest
(wraps console.*)              - assigns next seq (durable counter)
                                - appends to chrome.storage.session
                                  key: "events:<tabId>:<kind>"
                                - evicts oldest if count > cap (500)
                                - tracks dropped + oldestAvailableSeq
                                - retries POST w/ backoff on failure
                                                                     ◀──HTTP GET──── /api/events?since=&kind=&tab=
                                                                        returns {entries, dropped, oldestAvailableSeq}
                                                                                                          ◀── EventPoller
                                                                                                              .Read() / .Watch()
```

### Components

| File | Purpose |
|---|---|
| `src/contentScript/consoleCapture.ts` (new) | Injected as a MAIN-world script at `document_start`. Wraps `console.log/warn/error/info/debug`; forwards `{level, args, timestamp}` via `chrome.runtime.sendMessage`. Pure producer — no buffering, no retry logic here. |
| `src/background/eventStore.ts` (new) | Owns the per-tab, per-kind ring buffer. Assigns the next `seq` from a counter persisted in `storage.session` (not reset to 0 on restart). Appends entries, evicts oldest past the count cap, tracks `dropped` and `oldestAvailableSeq` per (tab, kind). POSTs new entries to the daemon; retries with backoff if the daemon is unreachable — `storage.session` already holds the durable copy, so a failed POST is retried, not lost. |
| `internal/host/server.go` (modified) | New `POST /api/events/ingest` (extension → daemon), `GET /api/events?since=&kind=&tab=` (CLI → daemon, computes `dropped = max(0, oldestAvailableSeq - since - 1)` for *this request's* cursor rather than replaying a stored total), and `GET /api/events/resync-needed` polled by the extension after each of the daemon's own (re)starts so it knows to replay its full `storage.session` buffer once. The daemon's copy is the CLI-facing source of truth, so a read doesn't depend on the extension being reachable at that moment — except immediately after a daemon restart, until resync completes. |
| `internal/commands/events.go` (new) | `EventPoller` with `Read()` (one snapshot) and `Watch(ctx, timeout, match)` (blocking long-poll, used later by `network wait`-style commands). `Read` prints `⚠ N events were evicted (buffer cap reached)` before printing entries whenever `dropped > 0`. |
| `internal/commands/console.go` (new) | `htrcli console read [--since N] [--level error,warn] [--json]` and `htrcli console watch [--timeout 10000]`, built on `EventPoller`. |

### Data model

```typescript
interface EventEntry {
  seq: number;            // monotonic per (tab, kind), durable across SW restarts
  kind: "console" | "network" | "dialog";
  timestamp: number;
  data: ConsoleEntry;     // | NetworkEntry | DialogEntry in later phases
}

interface ConsoleEntry {
  level: "log" | "warn" | "error" | "info" | "debug";
  args: string[];         // JSON-stringified args
  source?: string;        // file:line if available
}
```

```go
// internal/api/types.go
type EventEntry struct {
    Seq       int             `json:"seq"`
    Kind      string          `json:"kind"`
    Timestamp int64           `json:"timestamp"`
    Data      json.RawMessage `json:"data"`
}

type EventsResponse struct {
    Entries          []EventEntry `json:"entries"`
    Dropped          int          `json:"dropped"`
    OldestAvailableSeq int        `json:"oldestAvailableSeq"`
}
```

### Data flow (console read example)

1. Page calls `console.error("boom")`.
2. `consoleCapture.ts` forwards `{level:"error", args:["boom"], timestamp}` to the background script.
3. `eventStore.ts` assigns `seq=53`, persists it to `storage.session["events:123:console"]`, evicts seq 1 if the cap (500) was just exceeded (bumping `dropped` and `oldestAvailableSeq`), then POSTs the new entry to the daemon.
4. CLI runs `htrcli console read --since 40`.
5. Daemon's `GET /api/events?since=40&kind=console&tab=123` returns entries 41–53 plus `dropped: 12, oldestAvailableSeq: 41` (if eviction happened earlier in the session).
6. `console read` prints `⚠ 12 events were evicted (buffer cap reached)` then the 13 entries.

### Error handling

- **Service-worker restart mid-session**: `eventStore.ts` rehydrates its `seq`/`dropped` counters from `storage.session` on first access after restart rather than resetting to 0, so seq numbers stay monotonic and no silent counter reset occurs.
- **Message lost in-flight** (page → background right as the SW is killed): accepted small gap, not solved by this design.
- **Daemon unreachable when the extension tries to POST**: extension retries with backoff; `storage.session` already holds the entries, so retrying the POST doesn't risk re-dropping data.
- **Daemon process restart**: the daemon's in-memory store starts empty. On startup it marks itself as needing resync; the extension's background script polls a lightweight daemon-generation marker (bumped on every daemon start) and, on detecting a change, replays its entire `storage.session` buffer (all tabs, all kinds) via the same `/api/events/ingest` endpoint. Until that replay completes, CLI reads reflect only what's arrived since the restart — the daemon does not block reads waiting for resync, since a hung/unreachable extension shouldn't hang the CLI.
- **`dropped` accounting is per-request, not a replayed global counter**: the daemon computes `dropped = max(0, oldestAvailableSeq - since - 1)` from the *client's own* `since` cursor at query time. A client that already read past an eviction sees `dropped: 0` on its next poll even though the extension's own running eviction count (used only for `eventStore.ts`'s internal bookkeeping) is nonzero — the two numbers serve different purposes and must not be conflated.
- **Eviction**: never silent — `dropped` and `oldestAvailableSeq` are always returned, even when zero, so the CLI can distinguish "nothing was dropped" from "the daemon doesn't support this field."

## Testing

- Unit test `eventStore.ts` eviction/seq/dropped-count logic in isolation, mocking `chrome.storage.session` (Bun test, following the existing `*.test.ts` pattern e.g. `src/contentScript/commandExecutor.test.ts`).
- Go test for `/api/events` cursor pagination and `dropped` propagation in a new `internal/host/server_test.go` case.
- Go test for `EventPoller.Read`/`Watch` flag parsing and output formatting in `internal/commands/events_test.go`, following the existing pattern (`cdp_exec_test.go`, `root_test.go`).
- Manual end-to-end: open a page with a `console.log` loop exceeding 500 calls, force-restart the service worker (`chrome://serviceworker-internals` or reload the extension), then run `htrcli console read` and confirm a contiguous seq range with an accurate `dropped` count.
