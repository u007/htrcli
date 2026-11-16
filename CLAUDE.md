# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Vision

Be the fastest browser harness an AI agent can drive, and prove it with numbers
rather than adjectives. The competitive set is Browser Use, Claude for Chrome and
ChatGPT's browser agent.

Four claims, in priority order:

1. **Fewest steps per task.** One command per intent — `extract list`, `collect`,
   `fill-form`, `upload`, `wait` — instead of a click-by-click loop. Steps are
   counted, never estimated: `bench run` records `commands` per scenario and
   `index`/`attempts` per step.
2. **Reusable, secure sessions.** A session authenticated once is reused across
   runs instead of re-earned per task. Reuse must not weaken isolation: bearer
   tokens appear only as fingerprints, never in argv or in a report, and a
   cached profile stays scoped to the browser that created it. Built at bench
   level today (a scenario `context` pins one named persistent profile); the
   profile is only *authenticated* after a human logs in once.
3. **Accuracy before speed.** A fast wrong answer is a loss. Extracted values
   must survive real markup — flattened superscripts, footnote anchors,
   virtualized lists, attribute-only fields — and every claim must trace back to
   a page.
4. **Comparable numbers.** Every run is reported as pass rate, steps
   (`commands`), wall-clock (`wall_ms`) and retries, so a comparison with
   Browser Use is a table, not a claim.

Measured today: `bench run` records `wall_ms` and `commands` per scenario, and
`attempts` plus `wall_ms` per step. **Not measured:** token/usage cost — usage is
spent by the calling agent loop, so a comparison must say where the number came
from instead of implying the harness measured it.

Upstream reference: `browser-use/benchmark` reports score against cost per task
(weighted rubric, partial credit). Match the metric shape; do not import their
task text — it is encrypted and the README forbids republishing it. Adoption
detail lives in `benchmark.md`.

## Commands

```bash
# Extension (Chrome)
bun install
bun run dev          # Vite dev server with HMR
bun run build        # bun check . + Vite production build → build/
bun run zip          # build + create distributable ZIP
bun run check        # Biome lint + format check
bun run check:fix    # Auto-fix Biome issues
bun run test         # Run tests with Bun
bun run test:watch   # Watch mode

# Extension (Firefox)
bun run firefox:build      # bun check . + Vite build → firefox/build/
bun run firefox:typecheck  # bun check . (lint + format)
bun run typecheck          # bun check . (lint + format)
bun run firefox:zip        # Build + package as .xpi

# htrcli (Go CLI in htrcli/) — native-messaging daemon (sole backend for remote control)
make htrcli-build     # go build → htrcli/bin/htrcli
make htrcli-install   # go install (global)
make build           # htrcli-build + ext-build

htrcli install --browser chrome  --extension-id <id>   # register native host (Chrome)
htrcli install --browser firefox --extension-id htrncontrol@mercstudio.com
htrcli serve          # run daemon: HTTP :3845 + Unix socket relay (Chrome+Firefox)

# Video recording requires ffmpeg ≥ 6 on PATH (brew install ffmpeg)
# Missing ffmpeg produces explicit errors at both record start and stop.
# (Session recordings — `htrcli recordings …` — need NO ffmpeg and work on
#  both Chrome and Firefox. They are a different feature from `htrcli record`.)

# Utility
make close           # Kill process on :3845
make bench-smoke     # Run one offline benchmark scenario (also: bench-office,
                     # bench-tasks, bench-form, bench-booking, bench-expense,
                     # bench-files, bench-mail, bench-crm, bench-messaging,
                     # bench-research, bench-tickets, bench-leave,
                     # bench-social, bench-survey, bench-finance, bench-store,
                     # bench-cart, bench-retrieval, bench-compare,
                     # bench-challenge, bench-slack;
                     # all need a real Chrome on the CDP port, and bench-slack
                     # reports NOT_RUN because it requires auth)
```

### Benchmark harness

`htrcli bench run <scenario-file>` runs JSON scenario files and writes a
report under `docs/benchmarks/results/`. A scenario is a list of steps, each a
raw htrcli argv plus expectations (`exit`, `stdoutContains`,
`stdoutNotContains`, `jsonPath`). `retry: N` reuses the `retry` subcommand so
the backoff policy lives in one place; `requires_auth: true` is reported as
NOT_RUN, never as PASS. `capture: {"name": "items.0.q4"}` carries a step's
value into a later `${name}` reference (per-scenario, only from a step that
passed) — in argv and in expectations — which is how an aggregating task
benchmarks more than a page visit and how a created record's id is reused as a
selector.

Scenarios live in `docs/benchmarks/scenarios/`, the spec in
`benchmark.md`, and the fixture page in
`docs/benchmarks/fixtures/`. Runner code: `htrcli/internal/commands/bench*.go`.

Single test file: `bun test src/path/to/file.test.ts`

## Architecture

This is a **multi-part project** with the extension using htrcli as its sole backend:

```
┌──────────────┐   HTTP   ┌──────────────────────────────┐
│  External    │─────────►│         htrcli serve          │
│  Tool / CLI  │          │   Go daemon (port 3845)      │
└──────────────┘          │  HTTP API + NM relay relay    │
                          └───────────┬──────────────────┘
                                      │  Native Messaging
                                      ▼
                            ┌────────────────────────┐
                            │  Extension (Chrome/FF)  │
                            │  Service Worker         │
                            └────────────────────────┘
```

Each relay connection is scoped: it announces itself with an `identify` message
(`browser: chrome|firefox`) once the daemon's greeting confirms the link, and it
registers only its own tabs. Two routes reach a relay:

- `POST /api/tabs/<id>/command` — routed by tab, to the content script.
- `POST /api/background/command` — **tab-less**, to the background service
  worker, optionally with an advisory `browser` hint, or with `browserStrict:
  true` to pin the hint with no fallback (`tabs new` and `tabs close` use this).
  Session recording needs
  this: those actions belong to no page, so requiring an open `http/https` tab
  would be wrong. Connections are ordered by a monotonic `seq`, so selection is
  deterministic; a hint that matches nothing falls back to the earliest
  connection, and `recordings list` reports which browser actually answered.

### Extension (`src/` → `build/`)

Built with Vite + `@crxjs/vite-plugin` (Chrome only). Entry points defined in `src/manifest.ts`:

- **`src/background/index.ts`** — Service worker. Orchestrates recording sessions, captures screenshots via `chrome.tabs.captureVisibleTab`, manages state. Registers the recorder with `setRecordingDeps` so the `recording*` native commands can reach it. Note: the shared `debuggerManager` (refcounted attach for network/dialog capture) is used by both capture features, and `cdpInput.ts` now routes through it via `attachShared`/`detachShared` (concurrent attaches are coalesced into one real attach). `CDP_EVAL` and the `chrome.debugger.attach` calls in `index.ts`/`networkMock.ts` still attach directly.
- **`src/background/recordingCommands.ts`** — Pure dispatch logic for the `recordingStart|Stop|Status|List|Get|Delete` actions (htrcli `recordings …`). Browser-agnostic: no `chrome.debugger`, so it works on Chrome and Firefox. Side effects are injected via `RecordingCommandDeps`.
- **`src/contentScript/index.ts`** — Injected into every `http/https` page. Submodules handle:
  - `clickHandler.ts` / `inputHandler.ts` — Track interactions
  - `collectList.ts` — Virtualized-list extraction (scroll + accumulate + de-dupe)
  - `extractData.ts` — Structured table/list extraction
  - `commandExecutor.ts` — Execute remote control commands (click, fill, navigate, eval…)
  - `connectionManager.ts` — Manages native messaging lifecycle
  - `elementFinder.ts` / `selectorGenerator.ts` / `xpathGenerator.ts` — DOM query helpers
  - `highlighter.ts` — Visual overlay on elements before screenshot
- **`src/sidepanel/`** — React 18 UI shown in Chrome's side panel / Firefox sidebar. There is no toolbar popup: the toolbar icon toggles the panel (Chrome `sidePanel.setPanelBehavior`, Firefox `sidebarAction.toggle()` in `firefox/src/firefox-shims.ts`). Two views, `Recording` and `Tabs`, are selected via `src/sidepanel/tabState.ts` (persisted in `localStorage`; both panels stay mounted so switching cannot drop recording state).
  - `context/` — `RecordingContext.tsx` uses `useReducer` for all recording state
  - `components/` — UI components (`ConnectedTabs` = tab registry for remote control, `RecentRecordings` = stored-session list/delete)
- **`src/types/recording.ts`** — All shared TypeScript interfaces and `MessageType` union
- **`src/utils/`** — Export helpers (JSON, Markdown, ZIP via jszip), sensitive field detection
- **`src/nativeHost.ts`** — Native messaging bridge to `htrcli` host

### Firefox workspace (`firefox/`)

Plain Vite build (no crxjs). `firefox/vite.config.ts` emits `manifest.json` directly. Shares 100% of `src/` source code. Firefox-specific additions:

- `firefox/src/firefox-shims.ts` — Patches `chrome.sidePanel` stub + imports `webextension-polyfill` first
- Every entry shim in `firefox/src/` re-exports from `src/` after applying polyfill
- Result: `chrome.*` calls in shared `src/` resolve to Firefox's `browser.*` at runtime

### htrcli (`htrcli/`)

Go CLI (Go 1.22+). Self-contained daemon that provides the remote-control backend via native messaging. Config stored at `~/.htrcli/config.json`. Priority order: flags > env (`HTRCLI_SERVER`, `HTRCLI_TOKEN`) > config file.

Auth: IP whitelist (localhost only) + bearer token. Override with env vars:
```bash
HTR_BEARER_TOKEN="secret"        # Custom token
HTR_ENABLE_BEARER_TOKEN=false    # Disable token auth
HTR_ALLOWED_IPS="127.0.0.1,..."  # Expand whitelist
```

## Key Conventions

- **Package manager**: `bun` only — never npm/yarn.
- **Linting/formatting**: Biome with tabs, double quotes. Run `bun run check:fix` before committing.
- **Message passing**: All cross-component messages use typed interfaces from `src/types/recording.ts`. Add new message types to the `MessageType` union + create a matching interface.
- **Async message listeners**: Always `return true` from `chrome.runtime.onMessage.addListener` callbacks that respond asynchronously.
- **Error prefix**: `console.error/warn('[HTR NControl] ...')` in extension code.
- **Tests**: Bun's built-in runner. Test files: `*.test.ts`. Currently sparse — content script tests in `src/contentScript/commandExecutor.test.ts`.
- **Build output**: Chrome → `build/`, Firefox → `firefox/build/`.
- **Cross-boundary contracts** live in `shared/` as data, not code. `shared/recording-export-contract.json` pins the recording ZIP bundle layout for BOTH producers — the extension (`src/utils/exportZip.ts`) and the CLI (`htrcli/internal/commands/recordings_export.go`). Nothing reads it at runtime; each producer is bound to it by a test on its own side (`*.contract.test.ts` / `recordings_export_test.go`). If you change a producer's output, its test fails until you update the contract too — that is the intended coupling, not a nuisance.
