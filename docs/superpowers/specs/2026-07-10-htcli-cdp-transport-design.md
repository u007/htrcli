# htcli CDP Transport — Design

**Date:** 2026-07-10
**Status:** Approved

## Problem

The extension transport (htcli → daemon → native messaging → extension → content script) cannot reach browser-restricted pages. Chrome blocks both content-script injection and `chrome.debugger.attach` on `chrome.google.com/webstore/*` (error: "The extensions gallery cannot be scripted"), `chrome://` pages, and other protected origins. It also cannot run truly hands-off: extension trusted input requires the target tab to be active, and OS-level input would steal cursor/keyboard focus.

An external Chrome DevTools Protocol (CDP) client is not an extension, so those restrictions do not apply, and CDP input is delivered per-target — the browser window can stay backgrounded, minimized, or headless.

## Goals

- Drive restricted pages (Chrome Web Store dev console, etc.) from htcli.
- True background operation: no cursor/focus theft; window may be hidden entirely.
- Zero change to default behavior: the extension transport remains the default.

## Non-goals

- OS-level input synthesis (rejected: blind coordinates, focus theft, macOS-only).
- Full command parity in v1 (remaining verbs follow later).
- Routing CDP through the daemon (`htcli serve`) — transports stay independent.

## Constraint: Chrome 136+ profile restriction

Since Chrome 136, `--remote-debugging-port` is ignored on the default user profile. The CDP browser therefore always runs with a dedicated `--user-data-dir` (`~/.htcli/chrome-profile`). The user signs in to Google once in that profile; the session persists across restarts.

## Design

### 1. Transport layer

- Every v1 command gains a `--cdp` flag. Without it, behavior is unchanged (extension/daemon path untouched).
- With `--cdp`, htcli talks directly to Chrome: `GET localhost:<port>/json` to enumerate targets, then a WebSocket per target for commands.
- New Go package `htcli/internal/cdp`: connection client, target discovery, session management.
- Port: config key `cdp_port` in `~/.htcli/config.json`, default `9222`.
- Sticky opt-in: `htcli config set transport cdp` makes CDP the default transport; `--cdp` remains the per-command override.
- Nothing listening on the port → clear error: `CDP browser not running — start it with: htcli browser start`.

### 2. Browser lifecycle

`htcli browser start|stop|status|hide|show` — the single sanctioned Chrome-spawn site in htcli.

- **start** — locates the Chrome binary (standard macOS path; overridable via config `chrome_path`), launches detached with `--remote-debugging-port=<port> --user-data-dir=~/.htcli/chrome-profile --no-first-run`, waits until the port answers, writes `~/.htcli/browser.json` (pid, port, started_at, headless). If already running, prints status and exits 0.
- **start --headless** — adds `--headless=new`: no window ever. Documented caveat: do the first run visible to complete Google sign-in (headless sign-in trips bot detection); once the profile holds the session, headless works.
- **stop** — kills the recorded PID, removes `browser.json`.
- **status** — running/not, pid, port, mode (headless / visible / minimized).
- **hide / show** — for a visible instance, minimize/restore the window at runtime via CDP `Browser.setWindowBounds` (`windowState: "minimized"` / `"normal"`). No relaunch needed: start visible, log in, `hide`, drive it behind the scenes. Not applicable to headless instances.
- Switching visible ↔ headless requires `stop` + `start --headless` (Chrome cannot toggle live); documented.

### 3. Command coverage (v1)

Form-filling set: `browser start/stop/status/hide/show`, `tabs list`, `open`, `page`, `eval`, `find`, `fill`, `click`, `press`, `select`, `check`, `uncheck`, `value`, `screenshot`.

Implementation mirrors what the extension already does, ported to Go:

- **DOM verbs** (`find`, `fill`, `select`, `check`, `uncheck`, `value`, `page`, `eval`) — `Runtime.evaluate`, running JS ported from `src/contentScript/commandExecutor.ts` (element finding including `waitForElement`, actionability checks). `fill` = set value + dispatch `input`/`change` events, same semantics as the extension path.
- **Trusted input verbs** (`click`, `press`) — `Runtime.evaluate` to scroll the element into view and return viewport-center coordinates, then `Input.dispatchMouseEvent` / `Input.dispatchKeyEvent`, mirroring `src/background/cdpInput.ts`. The target tab is activated first via `Target.activateTarget` (CDP input requires the tab active within its window — known lesson; the window itself needs no OS focus).
- **open** — `Page.navigate` + wait for the load event.
- **screenshot** — `Page.captureScreenshot`.
- **tabs list** — `/json` targets.

Remaining verbs (hover, scroll, html, attr, printpdf, …) follow later; all are `Runtime.evaluate` one-liners or existing CDP methods.

### 4. Tab targeting

- With `--cdp`, `--tab` takes a CDP target ID (shown by `htcli tabs list --cdp`).
- No `--tab` → first `page`-type target.
- Extension tab IDs and CDP target IDs are different namespaces — documented explicitly.

### 5. Testing & docs

- Go unit tests with a mocked CDP connection (interface-based sender, same dependency-injection pattern as `cdpInput.ts`).
- One integration smoke test behind a build tag requiring a real Chrome.
- README + GUIDE: when to use `--cdp` (restricted pages, background/hands-off runs), Chrome 136 profile caveat, sign-in-once note, headless first-run caveat, tab ID namespace note.

## Decisions log

| Decision | Choice | Rejected alternatives |
|---|---|---|
| Reaching restricted pages | External CDP client | OS-level input (blind coords, focus theft, macOS-only); extension CDP (blocked by browser policy) |
| Default transport | Extension (unchanged); `--cdp` opt-in | CDP as default |
| Browser acquisition | htcli-managed (`browser start`) | attach-only (manual incantation); silent autolaunch (surprising, murky lifecycle) |
| CDP client location | htcli binary directly | through the daemon (couples relay, requires daemon running) |
| v1 scope | Form-filling command set | full parity day one |
| Hide/show | `--headless` at start + `hide`/`show` via `Browser.setWindowBounds` | relaunch-only visibility |
