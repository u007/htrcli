# htcli — HTR Ncontrol CLI

Go CLI for controlling browser tabs via the [HTR Ncontrol](https://github.com/u007/htrncontrol) remote control API.

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

## Quick Start

```bash
# 1. Start the HTR Ncontrol server
cd /path/to/htrncontrol
bun run server

# 2. Configure htcli
htcli config set-server http://127.0.0.1:3845
htcli config set-token <bearer-token>

# 3. Check connection
htcli health

# 4. Control the browser
htcli open https://example.com
htcli snapshot -i
htcli click @e3
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
htcli back                                # Go back
htcli forward                             # Go forward
htcli reload                              # Reload page
```

All navigation commands wait for the destination page to finish loading
(`document.readyState === "complete"`, up to 25s) before returning.

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
(`find`, `get text`, `get value`, …) keep instant, probing semantics and do
not wait.

The `wait` command also waits for its target to appear (default 5s, tunable
via `--timeout`). **Breaking change:** it now fails with an error if the
element never appears, instead of returning a "not found" null that callers
treated as "keep going". Update any script that relied on the old null result.

### Inspection

```bash
htcli find <selector>                     # Find element info
htcli get text <selector>                 # Get text content
htcli get value <selector>                # Get input value
htcli get attr <selector> <attribute>     # Get attribute
htcli get html <selector>                 # Get innerHTML
htcli snapshot                            # Accessibility tree
htcli screenshot [path]                   # Take screenshot
htcli page                                # Get page info
htcli eval <javascript>                   # Execute JS
htcli command <json>                      # Raw JSON command
```

`eval` accepts both single expressions (`htcli eval "document.title"`) and
**multi-statement scripts with an explicit `return`** (e.g.
`htcli eval "const n = 2; return n * 2;"`); it also supports `await` for
promises. It runs in the extension's **isolated world**, so page-context
globals/variables are not visible — use `debuggerEval` for page-context code.

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

- [HTR Ncontrol](https://github.com/u007/htrncontrol) extension installed (Chrome or Firefox)
- A server on :3845 — either the Bun server (`bun run server`) or the native-messaging daemon (`htcli serve`)
- Go 1.22+ (for building from source)

## License

MIT
