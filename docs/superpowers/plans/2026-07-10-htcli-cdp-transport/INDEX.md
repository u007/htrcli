# htcli CDP Transport — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Spec:** `docs/superpowers/specs/2026-07-10-htcli-cdp-transport-design.md`

**Goal:** Add a direct Chrome DevTools Protocol transport to htcli (`--cdp`) so it can drive browser-restricted pages (Chrome Web Store dev console) and run background/headless — extension transport unchanged as default.

**Architecture:** New Go package `htcli/internal/cdp` speaks CDP directly (HTTP `/json` discovery + WebSocket sessions) to a Chrome that htcli launches with `--remote-debugging-port` and a dedicated profile (`htcli browser start`). DOM verbs run a JS bundle (built from the extension's `src/contentScript/elementFinder.ts`, embedded via `go:embed`) through `Runtime.evaluate`; trusted input uses `Input.dispatchMouseEvent/KeyEvent`.

**Tech Stack:** Go 1.22, cobra/viper (existing), `github.com/gorilla/websocket` v1.5 (new dep), Vite/Bun for the JS bundle, Bun test + Go `testing`.

## Global Constraints

- Extension transport remains the default; CDP only on `--transport cdp` / `--cdp` / config `transport: "cdp"`. Per-command flag overrides config in both directions.
- All CDP HTTP/WS connections go to `127.0.0.1` literal; the WS dialer must send **no `Origin` header**. Never pass `--remote-debugging-address` to Chrome.
- Chrome launch flags (exact): `--remote-debugging-port=<port> --user-data-dir=$HOME/.htcli/chrome-profile --no-first-run --disable-backgrounding-occluded-windows --disable-renderer-backgrounding` (+ `--headless` when requested; never `--headless=new`).
- Config keys are kebab-case struct fields in `configData` (`transport`, `cdp-port`, `chrome-path`); default `cdp-port` 9222. Env: `HTCLI_TRANSPORT`, `HTCLI_CDP_PORT`, `HTCLI_CHROME_PATH` via existing viper chain.
- Port answering = source of truth for "browser running"; PID file is advisory. Verify process cmdline contains `.htcli/chrome-profile` before killing.
- Biome (tabs, double quotes) for TS; `bun run check:fix` before committing TS changes. Go: `gofmt` (tabs).
- `bun` only, never npm/yarn. Go tests: `go test ./...` from `htcli/`.
- Every caught error logged or explicitly commented `// intentionally not logged: <reason>`.
- Commits end with `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`.

## Execution Order

Parts must execute in order; tasks within a part are sequential.

| Part | File | Tasks | Delivers |
|---|---|---|---|
| 1 | `01-flag-and-config.md` | 1–2 | `--tab` int→string; config fields + subcommands + transport resolution |
| 2 | `02-cdp-client.md` | 3–4 | `internal/cdp`: target discovery, WS session with call/event correlation |
| 3 | `03-browser-lifecycle.md` | 5–7 | `htcli browser start/stop/status`, spike, `hide`/`show` |
| 4 | `04-dom-bundle-and-verbs.md` | 8–9 | JS DOM bundle + `go:embed`; DOM verbs over CDP |
| 5 | `05-input-nav-docs.md` | 10–12 | click/press/open/screenshot/tabs over CDP; command routing; smoke test + docs |

## Task Table

1. Change global `--tab` flag from int to string
2. Config: `transport`, `cdp-port`, `chrome-path` fields + `set-*` subcommands + `UseCDP()` resolution
3. `internal/cdp`: HTTP discovery (`/json`, `/json/version`)
4. `internal/cdp`: WebSocket session (Call, WaitEvent, no-Origin dial)
5. `htcli browser start|stop|status` (launch, browser.json, port probe, PID guard)
6. Spike: minimized-window input/screenshot/activateTarget on macOS (records results, gates Task 7 docs)
7. `htcli browser hide|show` via `Browser.getWindowForTarget`/`setWindowBounds`
8. DOM JS bundle from `src/contentScript/` (Vite lib build → `go:embed`)
9. DOM verbs over CDP: `eval`, `find`, `fill`, `select`, `check`, `uncheck`, `value`, `page`
10. Trusted input + nav over CDP: `click`, `press`, `open`, `screenshot`, `tabs list`
11. Wire `--cdp` routing into existing commands
12. Integration smoke test (build tag) + README/GUIDE docs
