# CHANGELOG

```txt
Summary
  1. document grouping follow 'SemVer2.0' protocol
  2. use 'PATCH' as a minimum granularity
  3. use concise descriptions
  4. type: feat \ fix \ update \ perf \ remove \ docs \ chore
  5. version timestamp follow the yyyy.MM.dd format
```

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
