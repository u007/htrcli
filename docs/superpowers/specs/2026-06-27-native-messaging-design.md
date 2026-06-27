# Native Messaging Support — Design Spec

**Date:** 2026-06-27  
**Status:** Approved (revised after arch review)

## Goal

Add Chrome Native Messaging as the primary communication channel between htcli and the extension, eliminating the need to run the Bun server. The existing WebSocket server remains as a fallback alternative. CLI commands (`htcli navigate`, `htcli click`, etc.) are unchanged.

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│  Chrome Extension                                        │
│  connectionManager.ts                                    │
│   1. connectNative("com.howtorecorder.host")             │
│   2. if fails → WebSocket ws://127.0.0.1:3845            │
└───────────────┬─────────────────────────┬───────────────┘
                │ Native Messaging         │ WebSocket
                │ (stdin/stdout)           │ (fallback)
                ▼                          ▼
     ┌──────────────────┐       ┌──────────────────────┐
     │  thin relay      │       │ bun run server        │
     │  (Chrome-spawned │       │ (Bun server)          │
     │   ephemeral)     │       │  - WS server          │
     │  stdin/stdout ↔  │       │  - HTTP :3845         │
     │  Unix socket     │       └──────────────────────┘
     └────────┬─────────┘
              │ Unix socket (~/.htcli/daemon.sock)
              ▼
     ┌──────────────────┐
     │  htcli serve     │  ← user-started, persistent
     │  (Go daemon)     │
     │  - HTTP :3845    │
     │  - Unix socket   │
     └────────┬─────────┘
              │ HTTP :3845 (bearer token + IP whitelist enforced)
              ▼
     htcli navigate / click / inspect / ...  (CLI unchanged)
```

### Key constraints

- **Chrome always spawns a new native host process** — `connectNative()` launches a fresh binary, not an existing one. The relay binary is intentionally thin and ephemeral.
- **MV3 service worker is killed on idle** — the relay dies with it. The persistent daemon survives, so CLI commands always reach it via HTTP.
- **Only one daemon owns :3845** — if Bun server is running, `htcli serve` exits fast. They are mutually exclusive.
- **Tab IDs are reconciled** — native path uses real `chrome.tabs` IDs; wsClient's URL-hash pseudo-IDs are mapped to real IDs when switching paths. Tab ID model is consistent from the CLI's perspective.

## Section 1: Native Messaging Host Setup

### Host manifest (`com.howtorecorder.host.json`)

```json
{
  "name": "com.howtorecorder.host",
  "description": "How-To Recorder native messaging host",
  "path": "/usr/local/bin/htcli",
  "type": "stdio",
  "allowed_origins": ["chrome-extension://<EXTENSION_ID>/"]
}
```

The `path` points to the htcli binary. When Chrome spawns it, it passes the calling extension's origin as argv[1] (e.g. `chrome-extension://abcdef.../`). The binary detects this to enter relay mode.

### Platform registration paths

| OS | Path |
|----|------|
| macOS | `~/Library/Application Support/Google/Chrome/NativeMessagingHosts/` |
| Linux | `~/.config/google-chrome/NativeMessagingHosts/` |
| Windows | `HKCU\Software\Google\Chrome\NativeMessagingHosts\com.howtorecorder.host` |

### `htcli install` command

- Detects OS, writes manifest to correct platform path
- Sets `path` field to `which htcli` resolved at install time
- Requires `--extension-id <id>` flag — errors if omitted
- Prints confirmation: manifest path and registered extension ID
- `htcli install --uninstall` removes the manifest

### Extension ID handling

- **Dev builds**: ID changes per machine/path. `htcli install --dev` prompts user to paste ID from `chrome://extensions`
- **Published CWS builds**: fixed ID — hardcoded in `htcli install` after first publish

### Extension manifest change

Add `"nativeMessaging"` to the `permissions` array in `src/manifest.ts`.

## Section 2: htcli — two new modes

### Mode A: relay (`htcli` invoked by Chrome)

Detected by checking `os.Args[1]` for a `chrome-extension://` prefix at startup.

**Thin relay binary behavior:**
- Connects to daemon Unix socket at `~/.htcli/daemon.sock`
- If daemon not running: writes error to stdout (NM protocol), exits
- Reads 4-byte little-endian length-prefixed JSON from stdin (Chrome NM protocol), forwards to daemon socket
- Reads responses from daemon socket, writes back to stdout (NM protocol)
- Exits when stdin closes (Chrome killed the port) — daemon is unaffected

### Mode B: daemon (`htcli serve`)

User-started persistent process. Owns two interfaces:

**Unix socket (`~/.htcli/daemon.sock`)**
- Accepts relay connections (one per Chrome-spawned relay instance)
- Receives native messages forwarded by relay, routes to HTTP command bridge
- Sends command results back to relay → Chrome → extension

**HTTP server (`:3845`)**
- Same REST API as Bun server: `/api/health`, `/api/tabs`, `/api/command`, etc.
- Bearer token + IP whitelist enforced (same config as Bun server via `HTR_*` env vars)
- When CLI sends command via HTTP: daemon queues it → sends via Unix socket to relay → relay forwards to extension → result returned to relay → daemon → HTTP response
- Pending command map: `commandId → chan CommandResult`
- Exits fast if `:3845` already in use (Bun server running)

### New files in `htcli/`

| File | Purpose |
|------|---------|
| `internal/host/relay.go` | Relay mode: stdin/stdout ↔ Unix socket bridge |
| `internal/host/daemon.go` | Daemon mode: Unix socket server + state |
| `internal/host/server.go` | Embedded HTTP server on :3845 |
| `internal/host/bridge.go` | Routes HTTP commands → Unix socket → results |
| `internal/host/native.go` | Shared 4-byte NM framing read/write |
| `internal/commands/serve.go` | `htcli serve` cobra command |
| `internal/commands/install.go` | `htcli install` cobra command |

### Entry point detection (in `main.go`)

```
if len(os.Args) > 1 && strings.HasPrefix(os.Args[1], "chrome-extension://") {
    host.RunRelay()
    return
}
commands.Execute()  // normal CLI path
```

## Section 3: Extension changes

### Architecture note

Content scripts cannot call `connectNative()` — only the background service worker can. All native messaging is owned by the background.

### New files

| File | Purpose |
|------|---------|
| `src/background/nativeHost.ts` | Owns native port, handles reconnection |
| `src/contentScript/connectionManager.ts` | Auto-detects native vs WebSocket, unified interface |

### `src/background/nativeHost.ts`

- Calls `chrome.runtime.connectNative("com.howtorecorder.host")` on startup
- On `port.onDisconnect`: re-attempts `connectNative()` with exponential backoff (handles SW restart + relay dying on idle)
- Listens for messages from native host, forwards commands to content scripts via `chrome.tabs.sendMessage` using real tab IDs
- Sends command results back to native host
- Reports connection status to content scripts via `chrome.runtime.sendMessage`
- On permanent failure (host not installed): sends `{ type: "native_unavailable" }` to trigger WS fallback

### `src/contentScript/connectionManager.ts`

- On load: sends `{ type: "GET_CONNECTION_STATUS" }` to background
- On `native_unavailable` response: calls `wsClient.connectToServer()` 
- Exposes unified interface: `isConnected()`, `disconnect()`
- Tab IDs reported to server always use real `chrome.tabs` ID (fetched via background message), not URL hash

### Tab ID reconciliation

`wsClient.ts` currently uses `hashString(window.location.href)` as a pseudo tab ID. With native messaging, the background has the real tab ID. `connectionManager.ts` fetches the real tab ID from background on init (`chrome.runtime.sendMessage({ type: "GET_TAB_ID" })`) and uses it for both paths. `wsClient`'s `getTabId()` is overridden via an exported setter so both paths report consistent IDs to the server/daemon.

### Modified existing files

| File | Change |
|------|--------|
| `src/contentScript/index.ts` | Import `connectionManager` instead of `wsClient` directly |
| `src/background/index.ts` | Register `nativeHost` message handlers; handle `GET_TAB_ID` |
| `src/manifest.ts` | Add `"nativeMessaging"` to `permissions` array |

### Unchanged files

- `src/contentScript/wsClient.ts` — untouched, invoked by `connectionManager` on fallback

## Section 4: Installation & setup flow

### First-time setup

```bash
# 1. Install htcli binary
go install github.com/u007/htcli/cmd/htcli@latest

# 2. Start the persistent daemon
htcli serve &

# 3. Register native host (once per machine)
htcli install --extension-id <id>

# 4. Reload extension in Chrome
# Extension connects via native messaging — no Bun server needed
```

### Fallback UX

- Native host not installed / daemon not running → extension silently falls back to WebSocket
- Neither native nor WS available → extension logs warning, no crash, graceful no-op
- Extension options page shows current connection mode: **Native / WebSocket / Disconnected**

### Auth

- HTTP :3845 on the daemon enforces bearer token + IP whitelist via `HTR_*` env vars (same as Bun server)
- Relay ↔ daemon Unix socket is local-only (no auth needed — socket permissions provide OS-level isolation)

### Native reconnection

- Background re-calls `connectNative()` on every `port.onDisconnect` with exponential backoff
- MV3 SW restart automatically triggers re-connect on next SW wake
- Relay dying (SW idle kill) is transparent — daemon persists, next SW wake spawns a new relay
