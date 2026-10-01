# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Extension (Chrome)
bun install
bun run dev          # Vite dev server with HMR
bun run build        # tsc + Vite production build → build/
bun run zip          # build + create distributable ZIP
bun run check        # Biome lint + format check
bun run check:fix    # Auto-fix Biome issues
bun run test         # Run tests with Bun
bun run test:watch   # Watch mode

# Extension (Firefox)
bun run firefox:build      # tsc -p firefox/tsconfig.json + Vite build → firefox/build/
bun run firefox:typecheck  # Type-check Firefox workspace only
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
```

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
  worker, optionally with an advisory `browser` hint. Session recording needs
  this: those actions belong to no page, so requiring an open `http/https` tab
  would be wrong. Connections are ordered by a monotonic `seq`, so selection is
  deterministic; a hint that matches nothing falls back to the earliest
  connection, and `recordings list` reports which browser actually answered.

### Extension (`src/` → `build/`)

Built with Vite + `@crxjs/vite-plugin` (Chrome only). Entry points defined in `src/manifest.ts`:

- **`src/background/index.ts`** — Service worker. Orchestrates recording sessions, captures screenshots via `chrome.tabs.captureVisibleTab`, manages state. Registers the recorder with `setRecordingDeps` so the `recording*` native commands can reach it.
- **`src/background/recordingCommands.ts`** — Pure dispatch logic for the `recordingStart|Stop|Status|List|Get|Delete` actions (htrcli `recordings …`). Browser-agnostic: no `chrome.debugger`, so it works on Chrome and Firefox. Side effects are injected via `RecordingCommandDeps`.
- **`src/popup/Popup.tsx`** — Toolbar popup: manual Start/Stop, optional title, audio toggle, recent recordings.
- **`src/contentScript/index.ts`** — Injected into every `http/https` page. Submodules handle:
  - `clickHandler.ts` / `inputHandler.ts` — Track interactions
  - `commandExecutor.ts` — Execute remote control commands (click, fill, navigate, eval…)
  - `connectionManager.ts` — Manages native messaging lifecycle
  - `elementFinder.ts` / `selectorGenerator.ts` / `xpathGenerator.ts` — DOM query helpers
  - `highlighter.ts` — Visual overlay on elements before screenshot
- **`src/sidepanel/`** — React 18 UI shown in Chrome's side panel / Firefox sidebar.
  - `context/` — `RecordingContext.tsx` uses `useReducer` for all recording state
  - `components/` — UI components
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
