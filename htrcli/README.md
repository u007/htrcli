# htrcli — HTR NControl CLI

Go CLI for controlling browser tabs via the [HTR NControl](https://github.com/u007/htrncontrol) remote control API.

`htrcli` is an HTTP client that talks to a server on port 3845. There are two
interchangeable transports for that server — pick one:

```
# WebSocket transport (Bun server)
htrcli (Go) ──HTTP──► Bun server (server/, :3845) ──WebSocket──► Extension ──DOM──► Chrome / Firefox

# Native messaging transport (htrcli daemon — no Bun server)
htrcli (Go) ──HTTP──► htrcli serve (:3845) ──Unix socket──► relay ──stdio──► Extension ──DOM──► Chrome / Firefox
```

The native-messaging daemon (`htrcli serve`) is a drop-in replacement for the Bun
server: same HTTP API on :3845, but the browser connects via a native-messaging
relay instead of a WebSocket. Both Chrome and Firefox are supported, and both can
be connected to the daemon at the same time (commands route to the browser that
owns the target tab). Only one of the two servers can hold :3845 at a time.

## Installation

### From source

```bash
git clone https://github.com/u007/htrncontrol.git
cd htrncontrol/htrcli
make build
./bin/htrcli --help
```

### Install globally

```bash
go install github.com/u007/htrcli/cmd/htrcli@latest
```

## Native Messaging (daemon mode)

Run the browser over a native-messaging relay instead of the Bun server. This
needs no `bun run server`; `htrcli serve` provides the HTTP API on :3845 itself.

```bash
# 1. Register htrcli as the browser's native-messaging host.
#    Chrome — use the extension ID from chrome://extensions → Details:
htrcli install --browser chrome  --extension-id <chrome-extension-id>
#    Firefox — use the add-on ID (browser_specific_settings.gecko.id):
htrcli install --browser firefox --extension-id htrncontrol@mercstudio.com

#    Remove a manifest with: htrcli install --browser <b> --uninstall

# 2. Reload the extension (chrome://extensions → reload, or
#    about:debugging → Reload) so it re-reads the host registration.

# 3. Start the daemon (binds :3845 + the Unix socket the relay connects to).
htrcli serve
#    Custom token / port: HTR_BEARER_TOKEN=secret HTR_PORT=3845 htrcli serve
```

Chrome and Firefox may both be registered and connected at once — `htrcli tabs
list` shows tabs from both, and `--tab <id>` routes to whichever browser owns
that tab. Screenshots and large command results (e.g. `fetch` bodies) travel
over HTTP, so they are not limited by the 1 MB native-messaging frame size.

The daemon pings each relay every 15s (`{"type":"ping"}`); the extension
replies with `{"type":"heartbeat"}`. Any relay silent for 45s is force-closed
and its tabs dropped, so stale/duplicate relays (e.g. a browser respawned its
native host while the old process lingered) clean themselves up. Extensions
older than this protocol never reply and get reaped every 45s — keep the
extension and htrcli builds in sync.

## CDP transport (direct Chrome DevTools Protocol)

By default `htrcli` drives the browser through the extension (the transports
above). With `--cdp` (or `htrcli config set-transport cdp`) it instead talks
**directly to Chrome over the Chrome DevTools Protocol** — no extension and no
server required. This is what you want for:

- **Browser-restricted pages** the extension can't reach (e.g. the Chrome Web
  Store developer console, `chrome://` internals).
- **Headless / background automation** — run Chrome with no window and drive it
  from a cron job or CI.

```bash
# Start a dedicated Chrome controlled by htrcli (fresh profile at ~/.htrcli/chrome-profile).
htrcli browser start                 # visible window
htrcli browser start --headless      # no window (recommended for background jobs)

htrcli browser status                # probe the debugging port
htrcli browser stop                  # kill the managed Chrome
htrcli browser hide                  # minimize the window (visible mode only)
htrcli browser show                  # restore the window

# Every command accepts --cdp (or the persisted transport=cdp config):
htrcli --cdp open https://chrome.google.com/webstore/.../console
htrcli --cdp fill "#email" "me@example.com"
htrcli --cdp click "#submit"
htrcli --cdp screenshot out.png
htrcli --cdp eval "document.title"
htrcli --cdp tabs list               # CDP page targets (no "Active" column)
```

### Tab-ID namespace

`--tab` means different things on the two transports:

| Transport | `--tab` value | Example |
|---|---|---|
| extension (`ext`, default) | numeric tab ID from `htrcli tabs list` | `--tab 43` |
| CDP (`cdp`) | 32-char hex **CDP target ID** from `htrcli --cdp tabs list` | `--tab 8E17C9D2...` |

`--cdp` selects the CDP path; the numeric form is rejected there (and a hex
target ID is rejected on the extension path).

### Sign in once, then drive headless

CDP can only control a profile that is already authenticated. **Sign in
visibly first** (`htrcli browser start`, log in, leave the session), then either
keep the window open or switch to `--headless` for subsequent runs — the
dedicated `~/.htrcli/chrome-profile` persists the session. The debugging port is
an **unauthenticated, localhost-only** control channel into that signed-in
profile: same trust model as the localhost daemon, minus the bearer token, so
only ever run it on a machine you trust.

### Configuration

```bash
htrcli config set-transport cdp        # make --cdp the default
htrcli config set-cdp-port 9222        # debugging port (default 9222)
htrcli config set-chrome-path /path/to/chrome   # if not auto-detected
```

Flags override config in both directions: `--transport ext` beats a
`transport=cdp` config, and `--cdp` beats a `transport=ext` config. If both
flags are passed, `--transport` wins (`--cdp` is only shorthand).

## Quick Start

```bash
# 1. Start the HTR NControl server
cd /path/to/htrncontrol
bun run server

# 2. Configure htrcli
htrcli config set-server http://127.0.0.1:3845
htrcli config set-token <bearer-token>

# 3. Check connection
htrcli health

# 4. Control the browser
htrcli open https://example.com
htrcli find "input[name=q]"                # find the search box
htrcli click "input[name=q]"               # act on the selector
htrcli screenshot page.png
```

## Commands

### Health & Config

```bash
htrcli health                              # Check server connection
htrcli config show                         # Show current config
htrcli config set-server http://...        # Set server URL
htrcli config set-token <token>            # Set bearer token
```

### Publishing to addons.mozilla.org (AMO)

`htrcli publish` builds (optionally) and signs the Firefox add-on, then
submits it to AMO via `web-ext sign`.

```bash
# Default channel is "listed" = public on addons.mozilla.org.
htrcli publish --build                     # build + sign + submit (public)

# Self-distributed / "own use" (was the old default before going public):
htrcli publish --channel unlisted

# Dry-run prints the exact web-ext command without submitting:
htrcli publish --dry-run --source-dir firefox/build
```

Channels:
- `listed` (default) — public listing on addons.mozilla.org; anyone can install.
- `unlisted` — self-distributed ("own use"); not shown in the gallery.

AMO API credentials (key + secret) are resolved in this order:
1. `--api-key` / `--api-secret` flags
2. Environment: `AMO_API_KEY` / `AMO_API_SECRET` (or `HTRCLI_AMO_API_KEY` / `HTRCLI_AMO_API_SECRET`)
3. htrcli config: `htrcli config set-amo-api-key <key>` / `htrcli config set-amo-api-secret <secret>`

Get credentials at <https://addons.mozilla.org/en-US/developers/addon/api/key/>.

`web-ext` is used automatically: if it is on `PATH` it is invoked directly,
otherwise `npx --yes web-ext` fetches it on demand. Override with `--web-ext <path>`.
The signed add-on is written to `web-ext-artifacts/`.

### Tab Management

```bash
htrcli tabs list                           # List connected tabs
htrcli tabs get <id>                       # Get tab info
```

### Navigation

```bash
htrcli open <url>                          # Navigate to URL
htrcli back                                # Go back (errors if no history)
htrcli forward                             # Go forward (errors if no history)
htrcli reload                              # Reload page
```

All navigation commands wait for the destination page to finish loading
(`document.readyState === "complete"`, up to 25s) before returning. `back` and
`forward` fail with an explicit "No previous/forward page in this tab's
history" error when the tab has no entry to navigate to, instead of silently
succeeding.

### Interaction

```bash
htrcli click <selector>                    # Click element
htrcli dblclick <selector>                 # Double-click
htrcli fill <selector> <value>             # Clear and fill
htrcli type <selector> <value>             # Append text
htrcli hover <selector>                    # Hover
htrcli press <key>                         # Press key
htrcli select <selector> <value>           # Select dropdown
htrcli check <selector>                    # Check checkbox
htrcli uncheck <selector>                  # Uncheck checkbox
htrcli scroll <direction> [pixels]         # Scroll page
htrcli clear <selector>                    # Clear input
```

Interaction commands (`click`, `dblclick`, `rightrclick`, `fill`, `type`,
`clear`, `select`, `check`, `uncheck`, `press`, and the visible-only `hover`,
`focus`, `blur`, `scroll`, `selectText`, `highlight`) **auto-wait** for their
target to exist, be visible, and (where it matters) be enabled before acting.
The default budget is 5s; override it with `--timeout <ms>` (capped at 20s). If
the element never becomes actionable the command fails with a descriptive error
(`not found` / `not visible` / `disabled`). Read-only inspection commands
(`find`, `text`, `value`, `attr`, `html`, `page`, …) keep instant, probing
semantics and do not wait.


On Chrome, `click`, `press`, and `type` are dispatched as **trusted** input
events via the Chrome DevTools Protocol. The page's default actions fire as if a
real user interacted: pressing `Enter` in a field submits the form, clicks pass
`event.isTrusted` checks, and focus/selection behave natively. On Firefox (no
`chrome.debugger` API) the same commands use synthetic events (with pointer-event
support) — they drive most automation but do not count as trusted.

While attached, Chrome shows the **“HTR NControl is debugging this browser”
infobar**; this is expected and also appears for `eval`/`print` on Chrome.

If DevTools is open on the target tab (or another debugger client is attached),
the trusted-input attach fails and the command returns an explicit error naming
the conflict — it does **not** silently fall back to synthetic events. Close
DevTools on that tab and retry.

If you need to block on an element appearing, use the raw `command` path
(which performs the wait and fails loudly on timeout):

```bash
htrcli command '{"action":"wait","target":{"selector":".loaded"},"options":{"timeout":10000}}'
```

### Inspection

```bash
htrcli find <selector>                     # Find element info (tag, attrs, box, text)
htrcli text  <selector>                    # Get text content
htrcli value <selector>                    # Get input value
htrcli attr  <selector> <attribute>        # Get attribute value
htrcli html  <selector>                    # Get innerHTML
htrcli command '{"action":"findAll","target":{"selector":"a"}}'  # multiple elements
htrcli page                                # Get page info
htrcli eval <javascript>                   # Execute JS in the page's main world
htrcli command <json>                      # Raw JSON command (any action)
```

`eval` accepts both single expressions (`htrcli eval "document.title"`) and
**multi-statement scripts with an explicit `return`** (e.g.
`htrcli eval "const n = 2; return n * 2;"`); it also supports `await` for
promises. It runs in the **page's main world** (via Chrome DevTools Protocol),
so page-context globals, React state, and closures are all visible. On
Firefox (`chrome.debugger` unavailable) `eval` returns an explicit error
message; on Chrome both the daemon and the Bun server use the same path.

### Selector Syntax

```bash
htrcli click "#submit"                     # CSS selector
htrcli click "name=email"                  # By name
htrcli click "role=button"                 # By ARIA role
htrcli click "text=Submit"                 # By text
htrcli click "label=Email"                 # By label
htrcli click "placeholder=Search"          # By placeholder
htrcli click "id=login"                    # By ID
htrcli click "xpath=//button[1]"           # By XPath
```

### Global Flags

```bash
--server <url>                            # Server URL
--token <token>                           # Bearer token
--json                                    # JSON output
--tab <id>                                # Target specific tab
--timeout <ms>                            # Command timeout
```

## Configuration

Config file: `~/.htrcli/config.json`

```json
{
  "server": "http://127.0.0.1:3845",
  "token": "your-bearer-token"
}
```

Priority: flags > env vars (`HTRCLI_SERVER`, `HTRCLI_TOKEN`) > config file > defaults.

## Requirements

- [HTR NControl](https://github.com/u007/htrncontrol) extension installed (Chrome or Firefox)
- A server on :3845 — either the Bun server (`bun run server`) or the native-messaging daemon (`htrcli serve`)
- Go 1.22+ (for building from source)

## License

MIT
