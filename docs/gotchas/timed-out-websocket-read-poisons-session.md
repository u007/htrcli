---
type: Gotcha
title: A timed-out websocket read poisons the CDP session for later calls
description: Why htrcli's recorder cleans up on a fresh PageSession — after a read deadline fires, gorilla caches the read error, so Page.stopScreencast and the status-bar restore on the capture session fail with the same timeout.
resource: htrcli/internal/cdp/session.go; htrcli/internal/cdp/screencast.go; htrcli/internal/cdp/statusbar.go
tags: [htrcli, cdp, websocket, gorilla, recorder, timeout]
timestamp: 2026-11-19T20:36:00Z
status: active
---

# A timed-out websocket read poisons the CDP session

`Session.WaitEvent` bounds a read with `SetReadDeadline`. When the deadline fires, gorilla's websocket caches the read error on the connection, so later reads on the **same session** return the same timeout instead of the next server message. The session is effectively unusable, even though the socket was never closed by the peer.

This was observed while fixing the screencast status-bar hold: a poll loop using `WaitEvent(..., 250ms)` to notice a stop made the deferred `Page.stopScreencast` and `Runtime.evaluate` restore fail with `read: i/o timeout` on a live fake server. The fix was not a retry — it was to stop polling and use a blocking read, then perform cleanup on a **fresh** session.

## Rule

- Do not poll `WaitEvent` with a short timeout and then keep using the session.
- When cleanup must run after a read is already blocked, open a fresh `PageSession` for it, do the work there, and only then close the capture socket to unblock the original read. `RunRecorder` does exactly this (`htrcli/internal/cdp/screencast.go`): the stop goroutine opens a cleanup session, calls `Page.stopScreencast` and `restoreControlStatusBar(cleanup, holdRecord)`, then closes the capture session.
- `Session.CallDeadline` is safe for a bounded one-shot call when no other read is in flight (hide/restore use it with 2 s), but after a timeout treat the session as unusable rather than issuing more calls on it.
- A timeout is logged, never swallowed: `CallDeadline` callers log `[htrcli] …` and the capture still runs.

## Where it bites

Any long-lived CDP session with a blocked event read and a cleanup path: the screencast recorder, and anything added later that reads events and must restore state on stop. Unit tests that use `fakeCDP` reproduce it: a 250 ms poll loop makes every later call time out even though the fake server is still answering.
