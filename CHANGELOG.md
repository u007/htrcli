# CHANGELOG

```txt
Summary
  1. document grouping follow 'SemVer2.0' protocol
  2. use 'PATCH' as a minimum granularity
  3. use concise descriptions
  4. type: feat \ fix \ update \ perf \ remove \ docs \ chore
  5. version timestamp follow the yyyy.MM.dd format
```

## 0.4.3 [2026.07.10]

- feat: add direct Chrome DevTools Protocol transport to `htcli` via `--cdp` / `transport=cdp`, plus `htcli browser start|stop|status|hide|show` to manage a dedicated Chrome profile (`htcli/internal/cdp`, `htcli/internal/commands`)
- feat: route existing browser commands over CDP when enabled, including DOM verbs, navigation, screenshots, and tab listing
- feat: add an embedded DOM-command bundle built from `src/contentScript/commandExecutor.ts` and a gated integration smoke test for the CDP path
- docs: explain the CDP transport, browser lifecycle, and tab-ID namespace in `htcli/README.md` and `GUIDE.md`

## 0.4.2 [2026.07.10]

- chore: bump version to 0.4.2
- fix: rename native-messaging host from `com.howtorecorder.host` → `com.htrcontrol.host` (matches the rebrand) in `htcli` (`main.go`, `install.go`), the extension service worker (`nativeHost.ts`), and `GUIDE.md`
- feat: `htcli publish` builds (optionally via `--build`) and signs the Firefox add-on, then submits it to addons.mozilla.org via `web-ext sign`; flags `--channel listed|unlisted`, `--api-key/--api-secret`, `--source-dir`, `--web-ext`, `--dry-run`, `--sign-timeout` (`publish.go`)
- feat: `htcli config` gains `set-amo-api-key` / `set-amo-api-secret` (masked in `config` output) so AMO credentials persist in `~/.htcli/config.json` (`config.go`)
- docs: document AMO publishing in `htcli/README.md` and `firefox/README.md`
- fix: `Makefile` `firefox-install` drops a stray `com.htrcontrol.host.json` argument the `htcli install` command does not accept

## 0.4.1 [2026.07.10]

- chore: bump version to 0.4.1
- feat: htcli daemon pings every relay every 15s and force-reaps any relay silent for 45s, dropping its stale tabs so a browser that respawned its native host (leaving the old relay process lingering) no longer pollutes `Tabs()` with duplicate/dead tabs (`daemon.go` `StartSweeper`/`SweepConns`/`Stop`; `bridge.go` greeting ping + `TouchConn` on every message)
- fix: `htcli serve` binds the HTTP port *before* the unix socket and refuses to unlink a socket already accepted by a running daemon, so a second `htcli serve` can no longer destroy the live daemon's socket on exit (`serve.go`, `bridge.go`)
- fix: extension reports "native" only after the daemon's greeting ping confirms the relay↔daemon chain end-to-end, and closes any superseded port so its traffic is ignored — eliminates the eternal reap/reconnect cycle a leaked old port used to cause (`nativeHost.ts` connection-confirmation state machine + single-port guard)
- feat: extension replies with a heartbeat to daemon pings so it isn't reaped as stale; relay "error" messages are treated as non-confirming (`nativeHost.ts`)
- feat: native `getReadyTabs` command reports tabs with a live content script for diagnostics via htcli; content-script injection failures now surface the real error (e.g. "Missing host permission", restricted domain) instead of a generic "tab not available" (`nativeHost.ts`, `commands.ts`)
- feat: side panel shows the outcome of each permission-grant attempt and adds a grant button on the empty/zero-tabs state with Firefox-specific guidance (`ConnectedTabs.tsx`)
- test: `nativeHost.test.ts` covers the connection-confirmation state machine (stays disconnected until greeting, ignores stale-port messages, relay error does not confirm); `daemon_test.go` covers sweep reaping stale relays and pinging live ones
- chore: `firefox:build` also builds the content script via the new `firefox/vite.content.config.ts`; `Makefile` `firefox-install` auto-registers the Firefox native host

## 0.4.0 [2026.07.09]

- chore: bump version to 0.4.0
- feat: `htcli inspect` / `GET /api/page` now accepts a `?tab=<id>` query param (the CLI `--tab` flag) to target a specific connected tab instead of always using the first connected tab; invalid ids return 400
- feat: `pressKey` and `prepareKeys` actions accept an optional target — a targetless press goes to the currently focused element (matching Playwright `keyboard.press` semantics), falling back to `body` when nothing has focus
- fix: background service worker activates the target tab before dispatching CDP mouse/key events — injected `Input.dispatchMouseEvent`/`dispatchKeyEvent` acks but never reaches a background tab, so the tab is focused first with a short settle delay
- test: `commandExecutor.test.ts` covers targetless `prepareKeys` (keeps current focus) and `pressKey` (dispatches to focused element)

## 0.3.0 [2026.07.09]

- feat: rebrand "How-To Recorder" → "HTR NControl" across extension, server, htcli, docs, and CI artifacts (manifests, options HTML, package names, `htrcontrol.zip` / `htrncontrol-firefox.xpi`)
- feat: WebSocket transport as a transparent fallback when native messaging is unavailable — content script auto-connects to the remote-control server if the native host is down (Firefox, host not installed, daemon down); side panel shows **Online** for either transport
- feat: per-install random bearer token (32-byte hex, `htr_` prefix) generated on first install; legacy `htr_aia_2026` default rotated on next launch with a one-shot migration marker
- feat: server (`bun run server` and `htcli serve`) read token from `HTR_BEARER_TOKEN_FILE` → `$XDG_CONFIG_HOME/htrcontrol/token` → `~/.config/htrcontrol/token` for one-step "make the server match the extension"
- feat: Options page gains **Regenerate** and **Copy** buttons for the bearer token with usage hint
- feat: `evaluate` action routed through CDP `Runtime.evaluate` on Chrome (page main world, no `new Function` CSP issue) with explicit error on Firefox; multi-statement and `await` scripts supported
- feat: `commandExecutor.test.ts` and `elementFinder.test.ts` exercise auto-wait, actionability, `wait` timeout semantics, `scrollTo` settling, and `evaluate` async/multi-statement handling (happy-dom harness)
- feat: `watchForTriggeredNavigation` state machine (`src/background/navigationWatch.test.ts`) — pure-logic unit tests for the new post-action settling helper
- feat: htcli daemon exposes `GET /api/page` (mirrors the Bun server's endpoint), returning the active tab's `PageInfo` (URL, title, viewport, readyState, history length)
- feat: `htcli eval` and other commands now exit non-zero on a `success: false` result with the extension's error message, instead of printing a success line
- feat: `getPageInfo` reports `document.readyState` and `window.history.length` so the server can detect load completion and `goBack`/`goForward` no-op cases
- fix: `goBack`/`goForward` no longer hang 25s on a no-op (no history); race against an 800ms URL-equality check returns "No previous/forward page in this tab's history" with the timer cleanly cancelled on the winner branch
- fix: post-action navigation settling ignores background page reloads (ad refresh, polling reload) on the same URL via `watcher.setBaseline(result.pageInfo?.url)` — the watcher no longer hangs for 25s on every click against such pages
- fix: extension connection mode in side panel reflects WS state (`"ws"` indicator) in addition to native messaging; the `CONNECTION_STATUS` listener and `setConnectionChangeCallback` paths are documented as complementary
- fix: `safeHistoryLength` no longer swallows all errors — `TypeError` (happy-dom null-frame) is silenced, other errors are logged
- fix: typed `EVALUATE_VIA_CDP: CommandAction = "evaluateViaCdp"` constant on both server and extension so a future rename fails the build rather than silently breaking the dispatch
- fix: `TabUpdateListener` and `waitForTabComplete` use the full `chrome.tabs.TabChangeInfo` / `chrome.tabs.Tab` types — no narrowing at the chrome API wrapper
- chore: `displayName` casing unified to `HTR NControl` (capital C) across server banner, package manifests, and CLI help
- docs: `skills/htrcli/SKILL.md` and `htcli/README.md` updated to reflect new back/forward no-op error messages, `page --json` output, eval semantics (main world, multi-statement, `await`)
- docs: `firefox/README.md` documents the WebSocket fallback path for users without the native host
- docs: `docs/superpowers/plans/2026-07-08-playwright-parity/` plan preserved; legacy `2026-06-27-native-messaging` plan + spec removed (work merged in earlier commits)

## 0.2.8 [2026.07.07]

- fix: add Firefox fallback in `CDP_NAVIGATE` background handler — use `chrome.tabs.update` when `chrome.debugger` is unavailable instead of crashing on the undefined API
- fix: add Firefox fallback in `PRINT_TO_PDF` background handler — return a graceful "unsupported on Firefox" error instead of throwing when `chrome.debugger` is undefined

## 0.2.7 [2026.07.06]

- fix: repair corrupted `.gitignore` line (`secrets.*.jshtcli/htcli` → separate entries), add `.env` and `htcli/bin/` to gitignore
- fix: remove tracked compiled Go binaries from `htcli/bin/` and `htcli/htcli`
- feat: extract hardcoded AIA API key into centralized `src/utils/aiaConfig.ts` (injected at build time via `VITE_AIA_API_KEY`)
- chore: add `.env.example`, `ImportMetaEnv` type declarations in `global.d.ts`
- docs: update htcli skill with daemon/native messaging transport and build instructions

## 0.2.6 [2026.07.02]

- feat: add `ConnectionMode` type (`"native" | "disconnected" | "unavailable"`) with reconnection backoff capped at 20 attempts
- feat: distinguish permanent native-host errors (not installed/forbidden) from transient ones (daemon down — relay exited) so the extension auto-recovers when the daemon restarts
- feat: add `retryConnect()` and `RECONNECT_NATIVE` message type for manual retry from the side panel
- feat: broadcast CONNECTION_STATUS to the side panel alongside content scripts, driving an Online/Reconnecting/Offline indicator with a "↻ Retry" button when permanently unavailable
- feat: Makefile `serve` and `close` targets now respect `HTR_PORT` env var (fallback `:3845`)
- chore: formatting-only cleanups in commandExecutor.ts, Options.tsx, connectionManager.ts, and nativeHost.ts

## 0.2.5 [2026.07.01]

- feat: fall back to `htcli config token` for bearer token when `HTR_BEARER_TOKEN` env var is unset
- update: improve `htcli serve` authentication warnings with actionable hint (`htcli config set-token <token>`)
- chore: rebuild htcli binary with viper token config support

## 0.2.4 [2026.06.30]

- fix: detect Firefox's native-messaging launch (manifest path + add-on ID as args) so `htcli` enters relay mode instead of leaking CLI text to stdout (was: `No such native application` / multi-hundred-MB frame errors)
- feat: `htcli install --browser chrome|firefox` — registers the native host with the correct manifest format (`allowed_extensions` vs `allowed_origins`) and per-browser directory (Mozilla vs Google Chrome)
- feat: route screenshots over HTTP (`GET` + `POST /api/screenshot`, correlated by command ID) instead of the relay, so PNGs that exceed the 1 MB native-messaging frame limit no longer tear down the connection
- fix: capture the focused window's active tab for screenshots rather than a registry tab ID (which may be stale or belong to another browser → "Invalid tab ID")
- feat: per-connection tab scoping in the daemon — Chrome and Firefox can connect simultaneously; commands route to the browser that owns the target tab, and one browser disconnecting no longer drops the others' tabs
- update: raise the relay/daemon framed-message read cap from 1 MB to 64 MB so large command results (fetch bodies, page HTML) survive the extension→daemon path
- docs: document native-messaging daemon mode (`htcli serve` / `htcli install`) and cross-browser setup in htcli and Firefox READMEs

## 0.2.3 [2026.06.29]

- feat: add Firefox cross-browser extension support (`firefox/` workspace, webextension-polyfill, sidebar shim)
- feat: redesign Options page with remote control settings (server URL, bearer token, enable toggle)
- feat: add ConnectedTabs component for managing connected browser tabs in side panel
- feat: add `fetch` and `printpdf` commands to htcli inspect (CSP-bypassing HTTP requests, CDP-based PDF export)
- feat: add optional TLS support to the API server (HTTPS via cert.pem / HTR_CERT env)
- feat: add `Access-Control-Allow-Private-Network` CORS header for local-network clients
- update: refactor native messaging host with reconnection logic and improved command dispatch
- update: bump package version from 0.1.0 to 0.2.3 with dependency updates (webextension-polyfill, @types/firefox-webext-browser, vite, typescript)
- docs: document Firefox architecture and build commands in AGENTS.md and README.md
- chore: add firefox/ build artifacts to .gitignore

## 1.1.0 [2026.06.28]

- feat: redesign options page with remote control settings (server URL, bearer token, enable toggle)
- feat: add auto-creation of Unix socket parent directory in htcli daemon
- feat: add unit test for ensureSocketParentDir
- docs: update native messaging plan with smoke-test status
- chore: update htcli binary build

## 1.0.1 [2026.06.11]

- feat: add tab management commands (`listTabs`, `getTabInfo`, `switchTab`)
- feat: add `GET_TAB_INFO` and `SWITCH_TAB` message handlers in background service worker
- feat: extend `CommandAction` type with new tab management actions

## 1.0.0 [2026.06.10]

- feat: add remote control system with HTTP/WebSocket API server (`server/`)
- feat: add content script command executor, element finder, XPath generator, and WebSocket client
- feat: add shared command types (`src/types/commands.ts`)
- feat: update manifest with new permissions (tabs, scripting, activeTab)
- feat: expand README with remote control architecture and API documentation
- chore: reformat codebase with Biome (tabs, quotes, import ordering)
- chore: add .bunrc.toml, .npmrc, .pnpmrc for cross-package-manager support
- chore: update TypeScript and Vite configurations

## 0.0.0 [2026.01.27]

- feat: initial
- feat: generator by ![create-chrome-ext](https://github.com/guocaoyi/create-chrome-ext)
