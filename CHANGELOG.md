# CHANGELOG

```txt
Summary
  1. document grouping follow 'SemVer2.0' protocol
  2. use 'PATCH' as a minimum granularity
  3. use concise descriptions
  4. type: feat \ fix \ update \ perf \ remove \ docs \ chore
  5. version timestamp follow the yyyy.MM.dd format
```

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
