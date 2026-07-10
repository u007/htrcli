# htcli CDP Transport — Design

**Date:** 2026-07-10
**Status:** Approved (revised after review)

## Problem

The extension transport (htcli → daemon → native messaging → extension → content script) cannot reach browser-restricted pages. Chrome blocks both content-script injection and `chrome.debugger.attach` on `chrome.google.com/webstore/*` (error: "The extensions gallery cannot be scripted"), `chrome://` pages, and other protected origins. It also cannot run truly hands-off: extension trusted input requires the target tab to be active, and OS-level input would steal cursor/keyboard focus.

An external Chrome DevTools Protocol (CDP) client is not an extension, so those restrictions do not apply (`Input.dispatch*` events are trusted, `isTrusted: true`), and CDP input is delivered per-target — with headless mode, no window exists at all.

## Goals

- Drive restricted pages (Chrome Web Store dev console, etc.) from htcli.
- Background operation without cursor/keyboard theft. **Headless is the guaranteed background mode**; hidden/minimized headful operation is best-effort (see §2a and the spike in §5).
- Zero change to default behavior: the extension transport remains the default.

## Non-goals

- OS-level input synthesis (rejected: blind coordinates, focus theft, macOS-only).
- Full command parity in v1 (remaining verbs follow later).
- Routing CDP through the daemon (`htcli serve`) — transports stay independent.
- Non-macOS Chrome binary discovery (v1 is macOS-only; `chrome-path` config overrides for other platforms).

## Constraint: Chrome 136+ profile restriction

Since Chrome 136, `--remote-debugging-port` is ignored on the default user profile. The CDP browser therefore always runs with a dedicated `--user-data-dir` (`~/.htcli/chrome-profile`). The user signs in to Google once in that profile; the session persists across restarts.

## Design

### 1. Transport layer

- Commands gain a `--transport ext|cdp` flag, with `--cdp` as sugar for `--transport cdp`. Default `ext` — behavior unchanged. Sticky default via config `transport`; the per-command flag always overrides, in both directions (config `cdp` + `--transport ext` forces the extension path).
- With CDP, htcli talks directly to Chrome:
  - `GET http://127.0.0.1:<port>/json` — enumerate page targets; per-target WebSocket for page-level domains (`Runtime`, `Input`, `Page`, `Target`).
  - `GET http://127.0.0.1:<port>/json/version` — obtain the **browser-level** `webSocketDebuggerUrl`, required for `Browser.*` domain calls (`getWindowForTarget`, `setWindowBounds`, `getWindowBounds`).
- **Handshake requirements** (implementer traps in modern Chrome): connect via `127.0.0.1` literal (the `/json` endpoint rejects non-IP `Host` headers as a DNS-rebinding guard), and the Go WebSocket client must send **no `Origin` header** (Chrome ≥111 rejects unlisted origins unless `--remote-allow-origins` is passed; sending none avoids the whole class).
- New Go package `htcli/internal/cdp`: HTTP discovery, page-session and browser-session WebSocket clients, request/response correlation.
- Config: new fields added to the `configData` struct in `htcli/internal/commands/config.go` — `transport`, `cdp-port` (default 9222), `chrome-path` — kebab-case, matching the existing `amo-api-key` style. They must be struct fields (the config file is round-tripped through that struct; keys outside it are silently dropped on the next write). New subcommands follow the existing pattern: `config set-transport`, `config set-cdp-port`, `config set-chrome-path`. Env vars follow the existing precedence chain (flags > `HTCLI_TRANSPORT`, `HTCLI_CDP_PORT`, `HTCLI_CHROME_PATH` > config file).
- Nothing listening on the port → clear error: `CDP browser not running — start it with: htcli browser start`.

**Security note:** the debugging port is an unauthenticated control channel into a profile holding a live Google session. Chrome binds it to localhost by default — htcli must never pass `--remote-debugging-address`. Residual risk (any local process can attach) is documented in the GUIDE; this matches the trust model of the existing localhost daemon, minus the bearer token.

### 2. Browser lifecycle

`htcli browser start|stop|status|hide|show` — the single sanctioned Chrome-spawn site in htcli.

- **start** — locates the Chrome binary (standard macOS app path; `chrome-path` config overrides), launches detached with:
  `--remote-debugging-port=<port> --user-data-dir=~/.htcli/chrome-profile --no-first-run --disable-backgrounding-occluded-windows --disable-renderer-backgrounding`
  then polls `/json/version` until it answers, and writes `~/.htcli/browser.json` (pid, port, started_at, headless). The port answering is the source of truth for "running" — if the port already answers, `start` prints status and exits 0 (this also handles Chrome's singleton-lock handoff, where a second launch defers to the existing profile owner and exits).
- **start --headless** — adds `--headless` (the old `=new` suffix is a deprecated alias since Chrome 132): no window ever. Documented caveat: do the first run visible to complete Google sign-in (headless sign-in trips bot detection); once the profile holds the session, headless works.
- **stop** — reads the PID from `browser.json`, verifies the process command line references `~/.htcli/chrome-profile` before killing (PID-reuse guard), removes `browser.json`.
- **status** — probes the port (not the PID file) for liveness; reports pid, port, and mode. Window state (visible/minimized) is read live via `Browser.getWindowBounds`, never cached.

#### 2a. Hide/show (best-effort)

- **hide / show** — for a visible instance: browser-level WS → `Browser.getWindowForTarget` → `Browser.setWindowBounds` with `windowState: "minimized"` / `"normal"`. No relaunch needed: start visible, log in, `hide`, drive it behind the scenes. Not applicable to headless instances.
- **Known risks, resolved by a mandatory spike before implementation (§5):**
  - Renderers of minimized/occluded windows are throttled; CDP input may be dropped and `Page.captureScreenshot` may return stale frames or hang. The `--disable-backgrounding-occluded-windows --disable-renderer-backgrounding` launch flags mitigate; the spike verifies.
  - `Target.activateTarget` may raise/focus the window at the OS level on macOS (Chromium calls window `Activate()`), which would defeat `hide` for `click`/`press`. The spike measures this; if confirmed, v1 documents headless as the only background mode for input verbs and `hide` as cosmetic (page stays drivable via `eval`/`fill`, which need no activation).
- Switching visible ↔ headless requires `stop` + `start --headless` (Chrome cannot toggle live); documented.

### 3. Command coverage (v1)

Form-filling set: `browser start/stop/status/hide/show`, `tabs list`, `open`, `page`, `eval`, `find`, `fill`, `click`, `press`, `select`, `check`, `uncheck`, `value`, `screenshot`.

- **DOM verbs** (`find`, `fill`, `select`, `check`, `uncheck`, `value`, `page`, `eval`) — run in the page via `Runtime.evaluate`. To avoid a divergent second implementation of the 1,500-line selector/actionability engine in `src/contentScript/commandExecutor.ts` (selector syntax `name=`/`role=`/`text=`/`label=`/`placeholder=`/`xpath=`, `waitForElement`, React-compatible native-setter `fill`), the element-finder/fill core is **extracted from `src/contentScript/` into a standalone JS bundle** built by the existing Vite setup and embedded in htcli via `go:embed`. Both transports then execute the same code; semantics cannot drift.
- **Trusted input verbs** (`click`, `press`) — `Runtime.evaluate` (using the embedded bundle) to scroll the element into view and return viewport-center coordinates, then `Input.dispatchMouseEvent` / `Input.dispatchKeyEvent`, mirroring `src/background/cdpInput.ts`. The target tab is activated first via `Target.activateTarget` (CDP input is dropped on unrendered tabs — known lesson). `press` without a selector dispatches to whatever holds focus, matching the extension path — no implicit focus step.
- **open** — `Page.enable` + `Page.navigate`, wait for `Page.loadEventFired` bounded by the global `--timeout` (default 30000 ms). SPA route changes don't fire load; same semantics as the extension's navigate wait.
- **screenshot** — `Page.captureScreenshot` (spike verifies behavior on hidden windows; headless is always reliable).
- **tabs list** — `/json` page targets. Output: target ID, title, URL. No `Active` column (CDP has no reliable "active" concept; the extension transport's table keeps its existing shape).

Remaining verbs (hover, scroll, html, attr, printpdf, …) follow later; all are `Runtime.evaluate` one-liners or existing CDP methods.

### 4. Tab targeting

- The global `--tab` flag changes from `int` to `string` (it is currently `IntVar` in `root.go`; CDP target IDs are 32-char hex strings). The extension path parses it to an int and errors clearly on non-numeric input; the CDP path passes it through. This touches `GetTabID()` and its callers — called out as its own implementation task.
- No `--tab` → first `page`-type target.
- Extension tab IDs and CDP target IDs are different namespaces — documented explicitly.

### 5. Testing & docs

- **Mandatory pre-implementation spike (half-day):** against a real Chrome with the §2 launch flags — (a) does `Target.activateTarget` raise/focus the window on macOS? (b) is `Input.dispatch*` delivered to the active tab of a minimized window? (c) does `Page.captureScreenshot` return fresh frames when minimized? Spike results decide whether `hide` supports input verbs or is documented as eval/fill-only.
- Go unit tests with a mocked CDP connection (interface-based sender, same dependency-injection pattern as `cdpInput.ts`).
- One integration smoke test behind a build tag requiring a real Chrome, covering the four likeliest breakages: fresh-profile `browser start`, `/json` discovery, `fill` on a React-controlled input (native-setter regression), screenshot while hidden.
- README + GUIDE: when to use CDP transport (restricted pages, background/hands-off runs), Chrome 136 profile caveat, sign-in-once note, headless first-run caveat, tab ID namespace note, unauthenticated-port security note.

## Decisions log

| Decision | Choice | Rejected alternatives |
|---|---|---|
| Reaching restricted pages | External CDP client | OS-level input (blind coords, focus theft, macOS-only); extension CDP (blocked by browser policy) |
| Default transport | Extension (unchanged); `--transport cdp` / `--cdp` opt-in, overridable both directions | CDP as default; one-way `--cdp` flag with no escape hatch |
| Browser acquisition | htcli-managed (`browser start`) | attach-only (manual incantation); silent autolaunch (surprising, murky lifecycle) |
| CDP client location | htcli binary directly | through the daemon (couples relay, requires daemon running) |
| v1 scope | Form-filling command set | full parity day one |
| DOM verb engine | Shared JS bundle from `src/contentScript/` via `go:embed` | reimplementing the selector engine in Go-emitted JS (guaranteed drift) |
| Hide/show | `--headless` at start (guaranteed) + best-effort `hide`/`show` via `Browser.setWindowBounds`, gated on spike | relaunch-only visibility; promising minimized input without verification |
| `--tab` flag | Change global flag to string | separate `--target` flag for CDP (two flags, one concept) |
