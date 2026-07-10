# Firefox Build

A Firefox port of the HTR NControl Chrome extension.

## Architecture

This folder contains **only** Firefox-specific glue — the entire feature
implementation (recording, screenshots, exports, side panel UI, etc.)
lives in the shared `../src/` tree, exactly the same source that powers
the Chrome build.

```
firefox/
├── README.md                    # This file
├── vite.config.ts               # Vite config (plain Vite, no @crxjs)
├── tsconfig.json                # TS config (extends the project tsconfig)
├── sidepanel.html               # Firefox sidebar HTML
├── popup.html                   # Toolbar popup HTML
├── options.html                 # Options page HTML
├── public/                      # Icons & static assets (mirrors ../public)
└── src/
    ├── browser-polyfill.ts      # Loads webextension-polyfill, aliases chrome=browser
    ├── background-entry.ts      # Firefox background service worker entry
    ├── contentScript-entry.ts   # Firefox content script entry
    ├── sidepanel-entry.ts       # Firefox sidebar (React) entry
    ├── popup-entry.ts           # Firefox popup entry
    └── options-entry.ts         # Firefox options entry
```

### Why a separate build?

`@crxjs/vite-plugin` (used by the Chrome build) is strictly
manifest-v3-shaped and only knows about Chrome's MV3 keys
(`side_panel`, `action`, etc.). Firefox MV3 has the same `manifest_version`
but uses `sidebar_action` instead of `side_panel` (Firefox 119+ has
`side_panel` too, but `sidebar_action` works in all MV3 versions of
Firefox ≥ 109 and behaves identically from a user perspective).

The Firefox build therefore uses plain Vite with a tiny custom plugin
that emits a `manifest.json` matching Firefox's schema, and re-uses
`webextension-polyfill` to normalize `chrome.*` ↔ `browser.*` so the
shared source compiles unchanged.

### Why MV3?

Firefox 109+ has full MV3 support (background as a service worker,
`browser_specific_settings.gecko.id`, etc.). Using MV3 keeps the
toolchain aligned with the Chrome build and avoids MV2's
deprecation timeline in Chrome. The sidebar UI is **identical** to the
Chrome side panel — same React code, same CSS, same components.

## Build & Install

### 1. Install dependencies (from the project root)

```bash
bun install
```

### 2. Build the Firefox extension

```bash
bun run firefox:build
```

This runs `tsc -p firefox/tsconfig.json` and then
`vite build --config firefox/vite.config.ts`, producing everything in
`firefox/build/`:

```
firefox/build/
├── manifest.json
├── background.js
├── content.js
├── firefox/
│   ├── sidepanel.html
│   ├── popup.html
│   └── options.html
├── assets/        # Hashed JS+CSS chunks
├── img/           # Icons
└── icons/
```

### 3a. Load temporarily in Firefox

1. Open `about:debugging#/runtime/this-firefox` in Firefox.
2. Click **Load Temporary Add-on…**
3. Select `firefox/build/manifest.json`

The extension is loaded for the current session. Reload by clicking
**Reload** in the same panel after each rebuild.

### 3b. Build a distributable `.xpi`

```bash
bun run firefox:zip
```

This produces `firefox/htrncontrol-firefox.xpi`, ready to upload
to [addons.mozilla.org](https://addons.mozilla.org) or distribute
directly (Firefox will install unsigned XPIs with a confirmation
prompt). To sign and submit to AMO through `htcli`, use:

```bash
htcli publish --build            # public ("listed") channel on addons.mozilla.org
htcli publish --channel unlisted # self-distributed / "own use"
```

`htcli publish` runs the Firefox build and calls `web-ext sign` for you.
See the [htcli README](../htcli/README.md#publishing-to-addonsmozillaorg-amo)
for credential setup and channel details.

## Development

For HMR-style reloads during development, run:

```bash
bun run firefox:dev
```

This starts the Vite dev server on a localhost port. To get the
extension to pick up changes, you'll need to point Firefox at the dev
URL via the `web-ext` tool, or just rebuild and reload the temporary
add-on after each change (`bun run firefox:build` is fast — sub-second).

## Differences from the Chrome build

| Area              | Chrome                              | Firefox                                            |
| ----------------- | ----------------------------------- | -------------------------------------------------- |
| Manifest version  | MV3                                 | MV3                                                |
| UI surface        | `side_panel` (right-side panel)     | `sidebar_action` (left-side sidebar)               |
| Permissions       | + `sidePanel`                       | (no `sidePanel`; `sidebar_action` is a UI key)     |
| `chrome.*` API    | Native                              | Aliased to `browser.*` via `webextension-polyfill` |
| Service worker    | Yes                                 | Yes                                                |
| `.zip` artifact   | `htrncontrol.zip`               | `htrncontrol-firefox.xpi` (Firefox convention) |

Everything else — recording state, IndexedDB sessions, exports, the
`htcli` native messaging host, screenshots, sensitive-field masking,
audio capture, the devtools panel, etc. — is shared code under `src/`
and behaves identically.

## Remote control via `htcli` (native messaging)

To drive this Firefox build from the [`htcli`](../htcli) CLI over native
messaging, register the native host for Firefox (Firefox uses
`allowed_extensions` + the add-on ID, not Chrome's `allowed_origins`):

```bash
htcli install --browser firefox --extension-id htrcontrol@mercstudio.com
```

This writes the host manifest to
`~/Library/Application Support/Mozilla/NativeMessagingHosts/` (macOS) or
`~/.mozilla/native-messaging-hosts/` (Linux). Then reload the extension
(`about:debugging` → **Reload**) and start the daemon with `htcli serve`.

Firefox and Chrome can both be registered and connected to the same daemon
at once; commands route to whichever browser owns the target tab. See the
[htcli README](../htcli/README.md#native-messaging-daemon-mode) for details.

> The add-on ID comes from `browser_specific_settings.gecko.id` in the
> built `manifest.json`. The native host requires the `nativeMessaging`
> permission, which the manifest already declares.

### Remote control without native messaging (WebSocket fallback)

Native messaging is the preferred transport, but it requires the `htcli`
host to be installed and the `htcli serve` daemon to be running. If native
messaging is unavailable on Firefox (host not installed, daemon down, or the
add-on ID not allowed), the extension **automatically falls back to a direct
WebSocket connection** to the remote-control server. You do **not** need to
register a native host to use remote control in this mode.

To use the WebSocket fallback, run the Bun API server (which speaks the
extension's WebSocket protocol) instead of `htcli serve`:

```bash
bun run server          # HTTP + WebSocket on ws://127.0.0.1:3845
```

The extension connects to the URL stored in `remoteControlServer`
(Options page). The default is `ws://127.0.0.1:3845` — note the **plain
`ws://` scheme**, not `wss://`: the server only enables TLS if `cert.pem` /
`key.pem` are present. If you have configured TLS, set the URL to
`wss://127.0.0.1:3845` in Options.

> `htcli serve` does **not** expose the WebSocket endpoint (it only offers
> the HTTP API plus the native-messaging relay), so the WebSocket fallback
> only works against `bun run server`. Use `htcli serve` for the native-
> messaging path, or `bun run server` for the WebSocket path.

The side-panel connection indicator now reflects whichever transport is
active: **Online** for native messaging or WebSocket, **Reconnecting…** while
a transient connection is retried, and **Offline** when neither is available.
