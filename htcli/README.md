# htcli — HTR NControl CLI

Go CLI for controlling browser tabs via the [HTR NControl](https://github.com/u007/htrncontrol) remote control API.

`htcli` is an HTTP client that talks to a server on port 3845. There are two
interchangeable transports for that server — pick one:

```
# WebSocket transport (Bun server)
htcli (Go) ──HTTP──► Bun server (server/, :3845) ──WebSocket──► Extension ──DOM──► Chrome / Firefox

# Native messaging transport (htcli daemon — no Bun server)
htcli (Go) ──HTTP──► htcli serve (:3845) ──Unix socket──► relay ──stdio──► Extension ──DOM──► Chrome / Firefox
```

The native-messaging daemon (`htcli serve`) is a drop-in replacement for the Bun
server: same HTTP API on :3845, but the browser connects via a native-messaging
relay instead of a WebSocket. Both Chrome and Firefox are supported, and both can
be connected to the daemon at the same time (commands route to the browser that
owns the target tab). Only one of the two servers can hold :3845 at a time.

## Installation

### From source

```bash
git clone https://github.com/u007/htrncontrol.git
cd htrncontrol/htcli
make build
./bin/htcli --help
```

### Install globally

```bash
go install github.com/u007/htcli/cmd/htcli@latest
```

## Native Messaging (daemon mode)

Run the browser over a native-messaging relay instead of the Bun server. This
needs no `bun run server`; `htcli serve` provides the HTTP API on :3845 itself.

```bash
# 1. Register htcli as the browser's native-messaging host.
#    Chrome — use the extension ID from chrome://extensions → Details:
htcli install --browser chrome  --extension-id <chrome-extension-id>
#    Firefox — use the add-on ID (browser_specific_settings.gecko.id):
htcli install --browser firefox --extension-id htrcontrol@mercstudio.com

#    Remove a manifest with: htcli install --browser <b> --uninstall

# 2. Reload the extension (chrome://extensions → reload, or
#    about:debugging → Reload) so it re-reads the host registration.

# 3. Start the daemon (binds :3845 + the Unix socket the relay connects to).
htcli serve
#    Custom token / port: HTR_BEARER_TOKEN=secret HTR_PORT=3845 htcli serve
```

Chrome and Firefox may both be registered and connected at once — `htcli tabs
list` shows tabs from both, and `--tab <id>` routes to whichever browser owns
that tab. Screenshots and large command results (e.g. `fetch` bodies) travel
over HTTP, so they are not limited by the 1 MB native-messaging frame size.

The daemon pings each relay every 15s (`{"type":"ping"}`); the extension
replies with `{"type":"heartbeat"}`. Any relay silent for 45s is force-closed
and its tabs dropped, so stale/duplicate relays (e.g. a browser respawned its
native host while the old process lingered) clean themselves up. Extensions
older than this protocol never reply and get reaped every 45s — keep the
extension and htcli builds in sync.

## Quick Start

```bash
# 1. Start the HTR NControl server
cd /path/to/htrncontrol
bun run server

# 2. Configure htcli
htcli config set-server http://127.0.0.1:3845
htcli config set-token <bearer-token>

# 3. Check connection
htcli health

# 4. Control the browser
htcli open https://example.com
htcli find "input[name=q]"                # find the search box
htcli click "input[name=q]"               # act on the selector
htcli screenshot page.png
```

## Commands

### Health & Config

```bash
htcli health                              # Check server connection
htcli config show                         # Show current config
htcli config set-server http://...        # Set server URL
htcli config set-token <token>            # Set bearer token
```

### Tab Management

```bash
htcli tabs list                           # List connected tabs
htcli tabs get <id>                       # Get tab info
```

### Navigation

```bash
htcli open <url>                          # Navigate to URL
htcli back                                # Go back (errors if no history)
htcli forward                             # Go forward (errors if no history)
htcli reload                              # Reload page
```

All navigation commands wait for the destination page to finish loading
(`document.readyState === "complete"`, up to 25s) before returning. `back` and
`forward` fail with an explicit "No previous/forward page in this tab's
history" error when the tab has no entry to navigate to, instead of silently
succeeding.

### Interaction

```bash
htcli click <selector>                    # Click element
htcli dblclick <selector>                 # Double-click
htcli fill <selector> <value>             # Clear and fill
htcli type <selector> <value>             # Append text
htcli hover <selector>                    # Hover
htcli press <key>                         # Press key
htcli select <selector> <value>           # Select dropdown
htcli check <selector>                    # Check checkbox
htcli uncheck <selector>                  # Uncheck checkbox
htcli scroll <direction> [pixels]         # Scroll page
htcli clear <selector>                    # Clear input
```

Interaction commands (`click`, `dblclick`, `rightclick`, `fill`, `type`,
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
htcli command '{"action":"wait","target":{"selector":".loaded"},"options":{"timeout":10000}}'
```

### Inspection

```bash
htcli find <selector>                     # Find element info (tag, attrs, box, text)
htcli text  <selector>                    # Get text content
htcli value <selector>                    # Get input value
htcli attr  <selector> <attribute>        # Get attribute value
htcli html  <selector>                    # Get innerHTML
htcli command '{"action":"findAll","target":{"selector":"a"}}'  # multiple elements
htcli page                                # Get page info
htcli eval <javascript>                   # Execute JS in the page's main world
htcli command <json>                      # Raw JSON command (any action)
```

`eval` accepts both single expressions (`htcli eval "document.title"`) and
**multi-statement scripts with an explicit `return`** (e.g.
`htcli eval "const n = 2; return n * 2;"`); it also supports `await` for
promises. It runs in the **page's main world** (via Chrome DevTools Protocol),
so page-context globals, React state, and closures are all visible. On
Firefox (`chrome.debugger` unavailable) `eval` returns an explicit error
message; on Chrome both the daemon and the Bun server use the same path.

### Selector Syntax

```bash
htcli click "#submit"                     # CSS selector
htcli click "name=email"                  # By name
htcli click "role=button"                 # By ARIA role
htcli click "text=Submit"                 # By text
htcli click "label=Email"                 # By label
htcli click "placeholder=Search"          # By placeholder
htcli click "id=login"                    # By ID
htcli click "xpath=//button[1]"           # By XPath
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

Config file: `~/.htcli/config.json`

```json
{
  "server": "http://127.0.0.1:3845",
  "token": "your-bearer-token"
}
```

Priority: flags > env vars (`HTCLI_SERVER`, `HTCLI_TOKEN`) > config file > defaults.

## Requirements

- [HTR NControl](https://github.com/u007/htrncontrol) extension installed (Chrome or Firefox)
- A server on :3845 — either the Bun server (`bun run server`) or the native-messaging daemon (`htcli serve`)
- Go 1.22+ (for building from source)

## License

MIT
