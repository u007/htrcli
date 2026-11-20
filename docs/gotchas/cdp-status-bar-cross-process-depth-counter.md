---
type: Gotcha
title: A cross-process capture hold must live in the DOM, with two kinds and a TTL
description: Why the CDP control-status hold is two counters on the page element (shot 60 s, record 6 h) with a content-script refresh on release, rather than one Go variable or one depth — a screencast and a separate `htrcli screenshot` are different processes, and a killed process must not wedge the counter.
resource: htrcli/internal/cdp/statusbar.go; htrcli/internal/cdp/screencast.go; htrcli/internal/cdp/nav.go; htrcli/internal/cdp/screenshot.go; src/contentScript/index.ts; src/contentScript/controlStatusBar.ts
tags: [htrcli, cdp, screenshot, screencast, concurrency]
timestamp: 2026-11-19T20:35:00Z
status: active
---

# A cross-process capture hold must live in the DOM

`htrcli screenshot` and `htrcli record` are separate CLI processes. Both can be attached to the same page, and both hide `#htrncontrol-control-status` around their capture. A hold kept in the Go process (a mutex plus a counter, or "hide if visible, restore when done") is per-process, so it cannot see a hold taken by the other process:

- The recorder starts a screencast, hides the bar, and holds it for the whole recording.
- A `htrcli screenshot` runs concurrently: its own depth is 0, so a naive restore writes `display` back and the bar reappears in the next recorded frame — while the recorder still believes it is hidden.

A shared file or a daemon-side mutex is worse: the file survives a crash (a stale lock blocks every later capture), and the CLI deliberately talks to CDP directly for captures.

## What works

State the hold on the element itself, as `htrcli/internal/cdp/statusbar.go` does:

- **Two kinds, because a screenshot and a recording have different lifetimes.** `shot` uses `data-htrncontrol-shot-holds` / `data-htrncontrol-shot-since` with a 60 s TTL; `record` uses `data-htrncontrol-record-holds` / `data-htrncontrol-record-since` with a 6 h TTL. A hide prunes an expired hold of its own kind before incrementing, so a hold left by a killed process clears on the next hide rather than blocking every later restore.
- Hide: increment this kind's counter and set `display:none`. Restore: decrement; only when both counters reach 0 remove the inline `display`, delete the attributes, and dispatch `htrncontrol:control-status-refresh`.
- **The refresh is what stops the two mechanisms fighting.** The content script listens for that event (`src/contentScript/index.ts`) and calls `refreshControlStatus()` (`src/contentScript/controlStatusBar.ts`), which re-renders from the extension's own state. The Go side does not guess whether the extension wanted the line hidden.
- `Runtime.evaluate` executes atomically on the page's main thread, so two CDP connections doing this read-modify-write cannot interleave — no lock needed, and the counters span processes.

## Why not one depth counter

A single `data-...-depth` with a saved `display` works while every process releases, but a killed process leaves the depth `> 0` forever: later restores only decrement and never reach 0, so the attributes and the hidden display persist until the page reloads. Two kinds with TTLs bound that failure: a leaked `shot` hold expires in 60 s, a leaked `record` hold in 6 h, and the next hide of that kind drops it. The refresh means visibility still follows the extension between hides.

## The stop path needs a fresh session

A recorder stop cannot clean up on the capture session. The frame read is blocked in `WaitEvent`; a read deadline that fires poisons the websocket for later calls (gorilla caches the read error), so `Page.stopScreencast` and the restore would fail with the same timeout. `RunRecorder` therefore opens a fresh `PageSession` in the stop goroutine, stops the screencast and restores there, then closes the capture socket to unblock the read. See `docs/gotchas/timed-out-websocket-read-poisons-session.md`.

## When this applies

Only when the driven Chrome runs the extension (an adopted user Chrome, e.g. the benchmark on CDP port 9555, or a disposable Chrome for Testing launched with `--load-extension`). `htrcli browser start` profiles have no `--load-extension`, so there is no host and the whole hold is a no-op — not a reason to skip it, because the same binary drives both.
