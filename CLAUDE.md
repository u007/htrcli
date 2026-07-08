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

# Remote control server (separate Bun project in server/)
bun run server       # Start API server (port 3845)
bun run server:dev   # Server with hot reload

# htcli (Go CLI in htcli/)
make htcli-build     # go build → htcli/bin/htcli
make htcli-install   # go install (global)
make build           # htcli-build + ext-build

# htcli native-messaging daemon (alternative to the Bun server on :3845)
htcli install --browser chrome  --extension-id <id>   # register native host (Chrome)
htcli install --browser firefox --extension-id htrcontrol@mercstudio.com
htcli serve          # run daemon: HTTP :3845 + Unix socket relay (Chrome+Firefox)

# Utility
make close           # Kill process on :3845
```

Single test file: `bun test src/path/to/file.test.ts`

## Architecture

This is a **multi-part project** with four independent runtimes:

```
┌──────────────┐   HTTP   ┌─────────────────┐  WebSocket  ┌────────────────────────┐
│  htcli (Go)  │─────────►│  server/ (Bun)  │────────────►│  Extension (Chrome/FF) │
└──────────────┘          │  port 3845      │             └────────────────────────┘
                          └─────────────────┘
```

### Extension (`src/` → `build/`)

Built with Vite + `@crxjs/vite-plugin` (Chrome only). Entry points defined in `src/manifest.ts`:

- **`src/background/index.ts`** — Service worker. Orchestrates recording sessions, captures screenshots via `chrome.tabs.captureVisibleTab`, manages state.
- **`src/contentScript/index.ts`** — Injected into every `http/https` page. Submodules handle:
  - `clickHandler.ts` / `inputHandler.ts` — Track interactions
  - `commandExecutor.ts` — Execute remote control commands (click, fill, navigate, eval…)
  - `wsClient.ts` — WebSocket connection to the server
  - `elementFinder.ts` / `selectorGenerator.ts` / `xpathGenerator.ts` — DOM query helpers
  - `highlighter.ts` — Visual overlay on elements before screenshot
  - `connectionManager.ts` — Manages WS lifecycle
- **`src/sidepanel/`** — React 18 UI shown in Chrome's side panel / Firefox sidebar.
  - `context/` — `RecordingContext.tsx` uses `useReducer` for all recording state
  - `components/` — UI components
- **`src/types/recording.ts`** — All shared TypeScript interfaces and `MessageType` union
- **`src/utils/`** — Export helpers (JSON, Markdown, ZIP via jszip), sensitive field detection
- **`src/nativeHost.ts`** — Native messaging bridge to `htcli` host

### Firefox workspace (`firefox/`)

Plain Vite build (no crxjs). `firefox/vite.config.ts` emits `manifest.json` directly. Shares 100% of `src/` source code. Firefox-specific additions:

- `firefox/src/firefox-shims.ts` — Patches `chrome.sidePanel` stub + imports `webextension-polyfill` first
- Every entry shim in `firefox/src/` re-exports from `src/` after applying polyfill
- Result: `chrome.*` calls in shared `src/` resolve to Firefox's `browser.*` at runtime

### Server (`server/`)

Independent Bun project (own `package.json`). HTTP + WebSocket API on port 3845. The extension's content script connects here via WS; external tools (including htcli) call the HTTP API.

Auth: IP whitelist (localhost only) + bearer token. Override with env vars:
```bash
HTR_BEARER_TOKEN="secret"        # Custom token
HTR_ENABLE_BEARER_TOKEN=false    # Disable token auth
HTR_ALLOWED_IPS="127.0.0.1,..."  # Expand whitelist
```

### htcli (`htcli/`)

Go CLI (Go 1.22+). Wraps the server HTTP API. Config stored at `~/.htcli/config.json`. Priority order: flags > env (`HTCLI_SERVER`, `HTCLI_TOKEN`) > config file.

## Key Conventions

- **Package manager**: `bun` only — never npm/yarn.
- **Linting/formatting**: Biome with tabs, double quotes. Run `bun run check:fix` before committing.
- **Message passing**: All cross-component messages use typed interfaces from `src/types/recording.ts`. Add new message types to the `MessageType` union + create a matching interface.
- **Async message listeners**: Always `return true` from `chrome.runtime.onMessage.addListener` callbacks that respond asynchronously.
- **Error prefix**: `console.error/warn('[HTR NControl] ...')` in extension code.
- **Tests**: Bun's built-in runner. Test files: `*.test.ts`. Currently sparse — server tests in `server/auth.test.ts`, content script tests in `src/contentScript/commandExecutor.test.ts`.
- **Build output**: Chrome → `build/`, Firefox → `firefox/build/`.
