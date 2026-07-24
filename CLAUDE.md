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

### Extension (`src/` → `build/`)

Built with Vite + `@crxjs/vite-plugin` (Chrome only). Entry points defined in `src/manifest.ts`:

- **`src/background/index.ts`** — Service worker. Orchestrates recording sessions, captures screenshots via `chrome.tabs.captureVisibleTab`, manages state.
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
