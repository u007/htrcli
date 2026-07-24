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
prompt). To sign and submit to AMO through `htrcli`, use:

```bash
htrcli publish --build            # public ("listed") channel on addons.mozilla.org
htrcli publish --channel unlisted # self-distributed / "own use"
```

`htrcli publish` runs the Firefox build and calls `web-ext sign` for you.
See the [htrcli README](../htrcli/README.md#publishing-to-addonsmozillaorg-amo)
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
`htrcli` native messaging host, screenshots, sensitive-field masking,
audio capture, page console capture, the devtools panel, etc. — is shared
code under `src/` and behaves identically.

## Remote control via `htrcli` (native messaging)

To drive this Firefox build from the [`htrcli`](../htrcli) CLI over native
messaging, register the native host for Firefox (Firefox uses
`allowed_extensions` + the add-on ID, not Chrome's `allowed_origins`):

```bash
htrcli install --browser firefox --extension-id htrncontrol@mercstudio.com
```

This writes the host manifest to
`~/Library/Application Support/Mozilla/NativeMessagingHosts/` (macOS) or
`~/.mozilla/native-messaging-hosts/` (Linux). Then reload the extension
(`about:debugging` → **Reload**) and start the daemon with `htrcli serve`.

Firefox and Chrome can both be registered and connected to the same daemon
at once; commands route to whichever browser owns the target tab. See the
[htrcli README](../htrcli/README.md#native-messaging-daemon-mode) for details.

Console output is captured in the page's MAIN world and is available through
`htrcli console read` / `htrcli console watch` exactly the same as Chrome.

> The add-on ID comes from `browser_specific_settings.gecko.id` in the
> built `manifest.json`. The native host requires the `nativeMessaging`
> permission, which the manifest already declares.

### Remote control requires native messaging

Remote control requires `htrcli` to be installed and the `htrcli serve` daemon
to be running. Native messaging is the only transport.

If native messaging is unavailable, the side panel shows **Install htrcli**.
Install the Go CLI and register the native host:

```bash
htrcli install --browser firefox --extension-id htrncontrol@mercstudio.com
htrcli serve
```
