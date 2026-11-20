---
type: Concept
title: Capture suppression — hide the control status line before every capture
description: The hide-before-capture lifecycle on both transports — the extension's background-to-content CONTROL_STATUS_SUPPRESS message (two-frame paint ack, refcount, 5 s watchdog, 500 ms send bound) and htrcli's Go-side DOM hold around CDP screenshot, full-page screenshot and screencast (two hold kinds with TTLs, plus a content-script refresh on release).
resource: src/background/captureSuppression.ts; src/contentScript/index.ts; src/contentScript/controlStatusBar.ts; src/background/index.ts; src/types/recording.ts; htrcli/internal/cdp/statusbar.go
tags: [extension, background, content-script, capture, screenshot, pdf, remote-control, cdp]
timestamp: 2026-11-19T05:07:38Z
status: active
---

# Capture suppression

The control status line is composited into `chrome.tabs.captureVisibleTab` pixels, `Page.printToPDF` output and CDP captures (`Page.captureScreenshot`, `Page.startScreencast`). Every capture that runs while it is visible must hide it first or the line lands in saved evidence. Visual states and triggers are in `control-status-bar.md`; this doc owns the ordering and failure contract for both transports — the extension message lifecycle and the Go-side DOM hold.

## The lifecycle

1. The background calls `withControlStatusSuppressed(tabId, capture)` (`src/background/captureSuppression.ts`).
2. It sends `{ type: "CONTROL_STATUS_SUPPRESS", suppressed: true }` to the tab (500 ms bound).
3. The content script calls `setControlStatusSuppressed(true)` and defers its ack with `afterNextPaint` — two `requestAnimationFrame` ticks (`src/contentScript/index.ts`). A style write is not painted synchronously, so an immediate ack would let the capture read the frame that still contains the line.
4. Only after the ack does the background run `capture`.
5. `finally` sends `suppressed: false`, which decrements the content-side depth and answers immediately.

`setControlStatusSuppressedInTab(tabId, suppressed, send?)` is the single-message form; `send` is injectable so tests never touch `chrome.tabs`. The in-command path (`handleCommand` around `CAPTURE_SCREENSHOT`) still uses the content-script function directly; the two paths stack because suppression is refcounted.

## Content-side semantics

- `suppressDepth` is an integer, not a boolean: overlapping captures each hold one level, and the line stays hidden until the last restore. A capture that starts during another capture cannot un-hide the line early.
- Each hide re-arms a **5 s watchdog** (`SUPPRESS_WATCHDOG_MS = 5000`) that force-clears the depth. A dropped restore (tab closed mid-capture, message lost) must not hide the line for the tab's lifetime.
- Suppression never touches the idle timer, so pausing for a screenshot cannot race the 1.5 s success / 3 s failure countdown.
- `hideControlStatus()` also zeroes the depth and clears the watchdog, so `disableRemoteControl()` tears down both the visible line and any suppression state.

## Failure policy

- A send is bounded to **500 ms**; a content script from an older version may never answer, and every capture in that tab would otherwise wait forever.
- A no-receiver error (`Receiving end does not exist`, `Could not establish connection`, `no receiver`) is expected for tabs with no content script, e.g. browser-internal pages, and is swallowed so it never blocks capture.
- Any other failure is logged with `[HTR NControl] control-status suppression failed:`. A hang or version mismatch must leave a trace, not vanish.

## Sites that must stay wrapped

In `src/background/index.ts`:

- `captureScreenshot` — recording steps, `captureScreenshotWithHighlight`, and the `CAPTURE_SCREENSHOT` message.
- `captureScreenshotForUpload` — the direct `captureVisibleTab` used before an upload.
- `captureFullPageScreenshot` — the whole tile loop is wrapped **once**, not per tile. Per-tile wrapping would let the line reappear between segments of one stitched image.
- `PRINT_TO_PDF` — wrapped inside its `attachShared`/`detachShared` window (the CDP print itself is not a `captureVisibleTab` call).

Adding a new extension capture path means wrapping it in `withControlStatusSuppressed`; a bare `captureVisibleTab` is a regression.

## Message contract

`ControlStatusSuppressMessage { type: "CONTROL_STATUS_SUPPRESS"; suppressed: boolean }` is defined once in `src/types/recording.ts`, appears in the `MessageType` union, and is included in the content-script handler union (`src/contentScript/index.ts`). The background sender, the handler, and `src/background/captureSuppression.test.ts` import that one definition — a second local shape would let the sender and handler drift.

## CDP captures — the Go-side hold (closed 2026-11-19)

The CDP gap is closed. htrcli does not send the extension message; it hides the host in the page itself, because a CDP capture can composite whatever the page currently paints.

`htrcli/internal/cdp/statusbar.go` owns the whole mechanism:

- `hideControlStatusBar(s, hold)` / `restoreControlStatusBar(s, hold)` call `Runtime.evaluate` with `returnByValue: true`, targeting `document.getElementById("htrncontrol-control-status")`. No host on the page is a no-op (`return false`), not an error.
- **Two hold kinds, because their lifetimes differ.** `holdShot` (screenshot) uses `data-htrncontrol-shot-holds` / `data-htrncontrol-shot-since` with a 60 s TTL. `holdRecord` (screencast) uses `data-htrncontrol-record-holds` / `data-htrncontrol-record-since` with a 6 h TTL. A hide prunes an expired hold of its own kind first, so a hold leaked by a killed process clears on the next hide instead of wedging the counter.
- **The hold lives on the element, not in the Go process.** A screencast and a separate `htrcli screenshot` are different processes sharing one page; a per-process counter would let the screenshot restore the bar mid-recording. `Runtime.evaluate` is atomic on the page's main thread, so the read-modify-write cannot interleave across connections.
- **The restore leaves visibility to the extension.** When both counters reach 0, restore removes the inline `display`, deletes the attributes, and dispatches `htrncontrol:control-status-refresh`. The content script listens (`src/contentScript/index.ts`) and calls `refreshControlStatus()` (`src/contentScript/controlStatusBar.ts`), which re-renders from the extension's own state. That is what stops the two mechanisms fighting over `display`.
- **A call is bounded to 2 s.** `Session.CallDeadline` with `statusBarCallTimeout` prevents a navigating or dead page from hanging a capture or a recorder stop. A failure is logged as `[htrcli] hide/restore control status bar: …` and the capture still runs.
- **The screencast stops cooperatively.** The stop goroutine opens a fresh `PageSession`, calls `Page.stopScreencast` and the record restore there, then closes the capture socket to unblock the frame read. A second session is required because a timed-out websocket read poisons the session for later calls (see `docs/gotchas/timed-out-websocket-read-poisons-session.md`).

Call sites:

- `Screenshot` in `htrcli/internal/cdp/nav.go` — `holdShot`.
- `ScreenshotFullPage` in `htrcli/internal/cdp/screenshot.go` — `holdShot`.
- `RunRecorder` in `htrcli/internal/cdp/screencast.go` — `holdRecord` across `Page.startScreencast` … `Page.stopScreencast`; error exits restore via a defer guarded by `cleanupDone`.

## Where the bar exists at all

The host only exists when the Chrome being driven runs the extension — an adopted user Chrome, for example the benchmark harness on CDP port 9555. `htrcli browser start` launches a dedicated profile (`--user-data-dir`, `htrcli/internal/cdp/launch.go`) with **no `--load-extension`**, so there is no content script and no bar to hide; the Go hold is a no-op there. That distinction is unchanged by this closure: the fix matters for `--cdp` runs against a Chrome that already has the extension installed.

## Validation

- `src/background/captureSuppression.test.ts` — 4 tests: typed message shape, hide-then-capture-then-restore order, restore when the capture throws, and no blocking for a tab without a content script.
- `src/contentScript/controlStatusBar.test.ts` — refcount keeps the line hidden until every overlapping restore, the watchdog force-clears a lost restore, a failure in an overlapping batch is still shown, a failing shadow root disables the bar without throwing, and the refresh re-renders a line hidden by an external override.
- `htrcli/internal/cdp/statusbar_test.go` — 6 tests against a fake CDP socket: `TestScreenshotSuppressesControlStatusBarAroundCapture` (exact `Runtime.evaluate` → `Page.captureScreenshot` → `Runtime.evaluate` order and the `htrncontrolShotHolds` expression), `TestScreenshotRestoresControlStatusBarWhenCaptureFails`, `TestScreenshotCapturesWhenStatusBarEvalFails`, `TestScreenshotFullPageSuppressesControlStatusBarAroundCapture`, `TestStatusBarEvalIsBounded` (a never-answering evaluate returns within the deadline), and `TestScreencastStopRestoresRecordHold` (hide before start, stop before restore, `htrncontrolRecordHolds` on both).
- Batch gates (2026-11-19): `bun run check` (215 files), `bun test` (634 pass / 0 fail, 40 files), `go vet ./...`, `go test ./...` (all packages ok), `bun run build`, `bun run firefox:build`, `bun run cdp-bundle:check`, `git diff --check`.
- **Live-verified, both transports on one tab (2026-11-19).** A disposable Chrome for Testing (Playwright's Chromium 1255) loaded the built unpacked extension; the extension id is path-derived and its service worker was driven over raw CDP (no native messaging). Results:
  - A `--cdp screenshot` during an active extension command left `display=""` and no `htrncontrolShotHolds`, so the Go hold released and the extension's visible state won.
  - A `--cdp screenshot` during a screencast left `record="1"` throughout, released the `shot` hold, and kept `display:none`; `record stop` released `record`; a second `record start` re-acquired `record="1"`, proving no stale hold blocked it.
  - The suppress ack measured **25–31 ms** (the two-frame `afterNextPaint` barrier).
  - A simulated leaked hold (`true,true,false`) re-rendered visible after **5.2 s** (the watchdog).
- **`make bench-smoke` PASS** (S01, 7 commands, 219 ms, 0 fail).
- ffmpeg 9 encoding is fixed and proven (2026-11-19): `media.EncodeArgs` uses `-fps_mode vfr`; the new argv encoded three PNG frames to H.264 with ffmpeg 9.0.2 (exit 0), while the old `-vsync` argv exited 8. The recorder was not re-run end-to-end with frames after the change.
- **Firefox + daemon path live-verified (2026-11-19):** the current `firefox/build` loaded as a temporary add-on via `web-ext run` and connected to the daemon; an extension screenshot during the 1.5 s status grace had white top rows (no amber), proving the background `withControlStatusSuppressed` path on Firefox. Chrome's daemon path could not be re-run: the installed extension is stale (hashed loader 404s) and its window returns `image readback failed`.
