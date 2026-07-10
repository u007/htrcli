# Spike: CDP minimized-window input / screenshot / activateTarget

**Date:** 2026-07-10
**Environment:** macOS server, Chrome 150.0.7871.101, run **headless** only
(no GUI/display available, so a visible (non-headless) window could not be
observed). Program: `htcli/internal/cdp/spike/main.go`.

## What was tested

Against `htcli browser start --headless` (port 9222, fresh `~/.htcli/chrome-profile`),
the spike:

1. Installs a click counter on `about:blank`.
2. Minimizes the window via `Browser.setWindowBounds({windowState:"minimized"})`.
3. Sends `Target.activateTarget` while "minimized" (a).
4. Re-minimizes, dispatches `Input.dispatchMouseEvent` pressed+released at (100,100) (b).
5. Reads the click counter, then `Page.captureScreenshot` (c).

## Observed results

| Question | Result |
|---|---|
| (a) `Target.activateTarget` restores/steals focus? | N/A in headless (no window to observe). Call returned without error. |
| (b) trusted click delivered while minimized? | **YES — clicks registered = 1** (wanted 1). `Input.dispatchMouseEvent` is delivered and counted. |
| (c) screenshot while minimized returns fresh frames? | **YES — 20964 bytes** of valid PNG returned. |

## Conclusion → Outcome B guidance

In **headless** mode (the recommended background/automation path, and the only
mode runnable on this server), trusted input (`Input.dispatchMouseEvent`) and
screenshots both work reliably regardless of the no-op "minimized" state. The
spike could not observe a *visible* minimized window because no display is
available here.

Therefore Task 7 ships `hide`/`show` as functional window controls, but the
GUIDE documents:

- For background/headless automation, launch with `htcli browser start --headless`
  (or keep the window visible and just run commands) — input verbs (`click`,
  `press`) and `screenshot` are guaranteed to work (confirmed by this spike).
- `hide`/`show` manipulate the real OS window and are meaningful in **visible**
  (non-headless) mode; in headless they are no-ops. Because a visible minimized
  window's input-delivery behavior was not empirically verified here, the GUIDE
  instructs users who need input while the window is hidden to use `--headless`
  rather than relying on `hide` + visible-mode input.

This matches the spec's note that headless is "the guaranteed hidden mode".
