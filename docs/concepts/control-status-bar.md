---
type: Concept
title: Control status bar — 2px page-top line during active remote commands
description: How the content-script #htrncontrol-control-status bar shows a 2px amber/red line for active commands only, isolated in a shadow root so it does not affect the page; suppression is refcounted with a 5 s watchdog and a paint-barrier ack, extension captures hide it before capturing, and htrcli's CDP screenshot/full-page/screencast holds hide it through two TTL-bounded DOM counters and a content-script refresh.
resource: src/contentScript/controlStatusBar.ts; src/contentScript/index.ts; src/background/captureSuppression.ts; src/background/index.ts; src/types/recording.ts; src/contentScript/controlStatusBar.test.ts; htrcli/internal/cdp/statusbar.go
tags: [extension, content-script, background, remote-control, ui, firefox, shadow-dom, cdp]
timestamp: 2026-11-19T05:07:38Z
status: active
---

# Control status bar

A 2px line fixed to the top of the page while a remote-control command is executing in that tab, hidden when control goes idle. It lives in shared `src/`, so it renders in both Chrome and Firefox (the Firefox entry shim reuses the same content script). Hook point is `handleCommand` in `src/contentScript/index.ts` — the single choke point for `EXECUTE_COMMAND` messages in the content script.

## Trigger: active commands only

- Amber while commands are in flight; red for 3s after the last command failed; hidden 1.5s after the last success.
- Green was deliberately dropped to reduce noise.
- The trigger is **active commands**, **not** daemon connection state and **not** a recording session. There is no explicit "control session" concept in the extension; `CONNECTION_STATUS` (mode `native|disconnected|unavailable`) is broadcast but explicitly not used as the trigger.

## Isolation ("transparent, not affecting the page")

- One lazily-created host `div#htrncontrol-control-status`, attached to `documentElement` — not `body`, which pages replace and which is null at `document_start`. Marked `data-htrncontrol-ignore="true"` and `aria-hidden="true"`.
- Host styles: `position: fixed; top: 0; left: 0; right: 0; height: 2px; z-index: 2147483647; pointer-events: none`.
- The visible line lives inside an **open shadow root** with its own `<style>`. Host-page CSS cannot restyle it and our styles cannot leak into the page — this is what satisfies "not affecting the page".
- A `prefers-reduced-motion` media query removes the 120ms background-color transition.

## State machine

- An `inFlight` counter keeps the line amber until **all** overlapping commands settle.
- A batch failure flag (`sawFailure`) is set by `markControlSettled(false)` and cleared by `markControlActive()` only when a new batch starts (`inFlight === 0`). A failure anywhere in an overlapping batch therefore still ends red even when the last command succeeded.
- One idle timer, re-armed (cleared and restarted) on every command start: success hides after 1500ms, failure holds red for 3000ms.
- `setControlStatusSuppressed(bool)` is **refcounted** (`suppressDepth`): overlapping captures each hold one level and the line stays hidden until the last restore. A **5 s watchdog** force-clears a lost restore, because a dropped `false` message must not hide the line for the tab's lifetime. Suppression deliberately does not touch the idle timer, so pausing for a screenshot cannot race the hide countdown.
- `hideControlStatus()` force-resets the timer, the watchdog, the in-flight counter, the failure flag, and the suppression depth. `disableRemoteControl()` calls it, so a torn-down tab does not keep a stale line.
- `handleCommand` calls `markControlActive()` once and `markControlSettled(...)` exactly once. The failure path settles before replying and returns, so a `sendResponse` that throws after the port closed cannot decrement another command's in-flight slot.
- Host creation is guarded: a failure sets a `disabled` flag, logs `[HTR NControl] Control status bar unavailable`, and never throws, so remote control keeps working. State updates after creation are plain DOM writes (no layout reads).

## Screenshot interaction — extension and CDP captures covered (2026-11-19)

The line is composited into captured pixels, so it must be hidden before a capture or it lands in saved evidence. Both **extension-background** and **htrcli CDP** captures are covered.

- **In-command capture (unchanged):** `handleCommand` suppresses the line around its `CAPTURE_SCREENSHOT` call (when the action is `screenshot` or `options.screenshot`) via try/finally. This remains.
- **Background-initiated captures (previously a deliberate blind spot, now FIXED for the extension):** these bypass `handleCommand`, so the line could appear in them. Suppression is now achieved via a **background→content message before each capture**. `src/background/captureSuppression.ts` exports `setControlStatusSuppressedInTab(tabId, suppressed, send?)` and `withControlStatusSuppressed(tabId, capture, send?)`: the helper sends a `CONTROL_STATUS_SUPPRESS` message to the tab, runs the capture, and restores in `finally`. Each send is bounded to **500 ms**; a "no receiver" error (a tab with no content script, e.g. a browser-internal page) is swallowed, while any other failure is logged with `[HTR NControl]` so a hang or version mismatch leaves a trace. `send` is injectable for tests. Suppression is refcounted and additional to the in-command suppression. The full lifecycle is in `capture-suppression.md`.
- **CDP-driven captures now hide the line (closed 2026-11-19):** htrcli does not send `CONTROL_STATUS_SUPPRESS`; it hides the host in the page. `htrcli/internal/cdp/statusbar.go` keeps two counters on the element — `shot` (60 s TTL) for screenshots and `record` (6 h TTL) for the screencast — prunes an expired hold of its kind on each hide, and on the last release removes the inline `display` and dispatches `htrncontrol:control-status-refresh`, which the content script answers by re-rendering. That makes the extension's state authoritative instead of the two mechanisms fighting over `display`. Each call is bounded by `Session.CallDeadline` (2 s); an eval failure is logged to stderr and the capture still runs. The screencast stops on a fresh `PageSession` because a timed-out read poisons the capture session. Mechanism in `capture-suppression.md`.
- **The bar only exists under the extension.** An adopted user Chrome (e.g. the benchmark at CDP port 9555) has the content script; `htrcli browser start` uses a dedicated `--user-data-dir` profile with no `--load-extension`, so its captures never had the bar and the hold is a no-op there.
- **Message plumbing:** `CONTROL_STATUS_SUPPRESS` added to the `MessageType` union plus `ControlStatusSuppressMessage { type, suppressed }` in `src/types/recording.ts` — one definition, imported by the background sender, the content-script handler union, and the tests. A content-script handler in `src/contentScript/index.ts` (`case "CONTROL_STATUS_SUPPRESS"`) calls `setControlStatusSuppressed(msg.suppressed)`.
- **The hide must be painted before the capture starts.** A style write is not painted synchronously, so answering a hide immediately would let the capture read the frame that still contains the line. The handler defers its ack with `afterNextPaint` — two `requestAnimationFrame` ticks — and only then calls `sendResponse`; a restore answers immediately.
- **Background wiring** in `src/background/index.ts`: every extension capture site is wrapped in `withControlStatusSuppressed` — `captureScreenshot` (used by recording steps, `captureScreenshotWithHighlight`, and `CAPTURE_SCREENSHOT`), full-page stitching in `captureFullPageScreenshot`, and direct capture in `captureScreenshotForUpload`. The full-page tile loop is wrapped **once around the whole loop**, not per tile, so the line stays hidden for a multi-tile stitch instead of flickering between segments. `PRINT_TO_PDF` is wrapped inside its `attachShared`/`detachShared` window.

## Validation (2026-11-19, 0.5.9 hardening batch)

- `bun run check` (Biome, 215 files), `bun test` (634 pass / 0 fail, 40 files), `go vet ./...`, `go test ./...` (all packages ok), `bun run build`, `bun run firefox:build`, `bun run cdp-bundle:check` (the committed `htrcli/internal/cdp/bundle/htrcli-dom.js` matches a fresh build), and `git diff --check` — all passed.
- `src/contentScript/controlStatusBar.test.ts` — 12 tests: one isolated host on `documentElement`, amber/red, idle grace, longer red hold, amber across overlapping commands, a failure in an overlapping batch still shown, suppression without resetting the timer, force-hide, suppression kept until every overlapping capture restores, the watchdog force-clearing a lost restore, a failing shadow root disabling the bar without throwing, and the refresh re-rendering a line hidden by an external override.
- `src/contentScript/index.test.ts` — entry-point wiring through the real message listener: `EXECUTE_COMMAND` creates the bar and settles on failure; `CONTROL_STATUS_SUPPRESS` hides and restores it through the handler; the refresh event re-renders it from state.
- `src/background/captureSuppression.test.ts` — 4 tests: typed message shape, hide-then-capture-then-restore order, restore on capture failure, and no blocking when the tab has no content script.
- `src/utils/keyMap.test.ts` — the shared key descriptor table and modifier combos (`+`, aliases, Shift derivation, unknown-key rejection); `htrcli/internal/cdp/input_test.go` — `TestKeyParamsDerivesShiftForShiftedCharacters`, `TestParseKeyAcceptsModifierAliases`, `TestParseKeyAcceptsPlusKey`, and `TestKeyInputRejectsInvalidSpecs` cover the Go payload/parse contract. Both transports must agree, and `bun run cdp-bundle:check` keeps the embedded `--cdp` bundle from drifting when `src/contentScript` or `src/utils` changes.
- Both extension production bundles were statically verified to contain `CONTROL_STATUS_SUPPRESS` and `htrncontrol-control-status` in the earlier follow-up; that remains their only evidence. `cdp-bundle:check` covers only the embedded `--cdp` DOM bundle.
- CDP hold (`htrcli/internal/cdp/statusbar_test.go`) — 6 tests: hide before capture and restore after (exact `Runtime.evaluate` → `Page.captureScreenshot` → `Runtime.evaluate` order and the `htrncontrolShotHolds` expression), restore when the capture fails, capture still succeeds when the status-bar eval fails, full-page ordering, a never-answering evaluate bounded by the deadline, and `TestScreencastStopRestoresRecordHold` (hide before start, stop before restore, `htrncontrolRecordHolds` on both).
- **Live-verified, both transports on one tab (2026-11-19).** A disposable Chrome for Testing (Playwright's Chromium 1255) loaded the built unpacked extension; its service worker was driven over raw CDP (no native messaging). A `--cdp screenshot` during an active command left `display=""` and no `shot` hold; a `--cdp screenshot` during a screencast left `record="1"` throughout, released the `shot` hold, and kept `display:none`; `record stop` released `record`; a second `record start` re-acquired `record="1"`. The suppress ack measured **25–31 ms** (two-frame paint barrier) and a simulated leaked hold (`true,true,false`) re-rendered visible after **5.2 s** (watchdog). `make bench-smoke` PASS (S01, 7 commands, 219 ms, 0 fail).
- **ffmpeg 9 encode fixed (2026-11-19):** `media.EncodeArgs` uses `-fps_mode vfr`; `-vsync` was removed in ffmpeg 9 (`-fps_mode` exists since 5.1, and the project requires ≥6). Proven against ffmpeg 9.0.2: the new argv encoded three PNG frames to H.264 (4 frames, 0.36 s, exit 0), while the old argv exited 8 with `Unrecognized option 'vsync'`. The recorder itself was not re-run end-to-end with frames after the change.
- **Firefox + daemon path live-verified (2026-11-19):** the current `firefox/build` loaded as a temporary add-on via `web-ext run` connected to the running daemon; an extension `attr` command read the status host, and an extension screenshot taken within the 1.5 s grace had white top rows (no amber), so the capture suppression ran on Firefox through native messaging. Chrome's daemon path could not be re-run: the installed extension is stale (its hashed content-script loader 404s) and its minimized window returns `image readback failed`.
- Verification scope: the CDP live run drove the extension service worker over raw CDP in a disposable Chrome for Testing; the Firefox/daemon run used `web-ext` with native messaging. Neither exercised the toolbar UI; the standalone helper-Chrome visual check plus static bundle verification remain the evidence for the rendered line itself.

## Files

- Added (original): `src/contentScript/controlStatusBar.ts`, `src/contentScript/controlStatusBar.test.ts`.
- Added (follow-up): `src/background/captureSuppression.ts`, `src/background/captureSuppression.test.ts`.
- Modified: `src/contentScript/index.ts` (import, `markControlActive`, `markControlSettled`, in-command suppression, and the `CONTROL_STATUS_SUPPRESS` handler); `src/background/index.ts` (four `withControlStatusSuppressed` wrappers); `src/types/recording.ts` (message type + interface).
- Added (CDP closure): `htrcli/internal/cdp/statusbar.go`, `htrcli/internal/cdp/statusbar_test.go`.
- Modified (CDP closure): `htrcli/internal/cdp/nav.go`, `htrcli/internal/cdp/screenshot.go`, `htrcli/internal/cdp/screencast.go` (cooperative stop and the `record` hold), `htrcli/internal/cdp/session.go` (`CallDeadline`), `src/contentScript/index.ts` and `src/contentScript/controlStatusBar.ts` (the refresh event).
