# htrcli — HTR NControl CLI

Go CLI for controlling browser tabs via the [HTR NControl](https://github.com/u007/htrncontrol) remote control API.

`htrcli` is an HTTP client that talks to the native-messaging daemon on port 3845.
The daemon relays commands to the extension over native messaging:

```
htrcli (Go) ──HTTP──► htrcli serve (:3845) ──Unix socket──► relay ──stdio──► Extension ──DOM──► Chrome / Firefox
```

Both Chrome and Firefox are supported, and both can be connected to the daemon
at the same time (commands route to the browser that owns the target tab).

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

Run `htrcli serve` to start the native-messaging daemon, which provides the
HTTP API on :3845 and relays commands to the extension.

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

### Captured console output

The daemon keeps a cursor-based event buffer for page `console.*` output.
Use it to read what happened after a specific sequence number or block until a
new log line arrives:

```bash
htrcli console read --since 0
htrcli console watch --since 100 --timeout 10000
```

`console read` prints a warning when the buffer evicted older entries.

### Captured network requests

The daemon captures page network requests (URL, method, status, duration) into
the same cursor-based event buffer:

```bash
# Read buffered network entries after cursor 0
htrcli network read --since 0

# Arm capture and stream new entries until timeout (default 10s)
htrcli network watch --since 100 --timeout 15000

# Arm capture and block until a matching request completes
htrcli network wait --since 0 --timeout 10000 --url "*/api/users*" --status 200
```

`network read` and `network watch` print a warning when the buffer evicted older
entries. `network wait` accepts a glob `--url` pattern (path.Match semantics,
where `*` spans any character including `/`) and an optional `--status` filter.
It prints the first matching entry, or exits with a timeout error.

### Mocking and blocking network requests

Intercept requests before they reach the server. Rules are per-tab and apply to
subsequent requests in that tab:

```bash
# Mock a GET /api/user response with status 200 and a body file
htrcli network mock --url-pattern "*/api/user" --method GET --status 200 --body-file ./mock-user.json

# Block (fail) matching requests
htrcli network block --url-pattern "*/api/analytics*"

# Remove a specific rule by its url pattern
htrcli network unmock --url-pattern "*/api/user"

# Remove all rules
htrcli network unmock --all
```

`network mock` flags:
- `--url-pattern` (required) — glob to match request URLs
- `--method` — restrict to an HTTP method (GET, POST, PUT, DELETE, ...)
- `--status` — mock response status code (default 200)
- `--body-file` — path to a file whose contents become the response body

`network block` accepts `--url-pattern` and `--method` (no body or status — the
request fails immediately).

### Captured dialogs (alert / confirm / prompt)

The daemon can auto-handle JavaScript dialogs and record their results. Arm a
policy for the next dialog(s), then list what was handled:

```bash
# Accept the next dialog (default)
htrcli dialog handle --action accept

# Dismiss the next dialog
htrcli dialog handle --action dismiss

# Respond with text to a prompt dialog
htrcli dialog handle --action respond --text "my answer"

# List handled dialogs since cursor 0
htrcli dialog list --since 0
```

Supported `--action` values: `accept`, `dismiss`, `respond`. With `respond`,
pass `--text` with the prompt answer. `dialog list` prints a warning when the
buffer evicted older entries.

The daemon pings each relay every 15s (`{"type":"ping"}`); the extension
replies with `{"type":"heartbeat"}`. Any relay silent for 45s is force-closed
and its tabs dropped, so stale/duplicate relays (e.g. a browser respawned its
native host while the old process lingered) clean themselves up. Extensions
older than this protocol never reply and get reaped every 45s — keep the
extension and htrcli builds in sync.

### Tray icon

When you run `htrcli serve` on a desktop (macOS, Windows, Linux with a
display), a system-tray icon appears automatically. See
[htrcli/docs/tray.md](docs/tray.md) for what the menu does and how to
disable it.

Headless Linux servers (no display, or logged in over SSH) silently
skip the tray — no configuration needed.

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
# 1. Configure htrcli
htrcli config set-server http://127.0.0.1:3845
htrcli config set-token <bearer-token>

# 2. Check connection
htrcli health

# 3. Control the browser
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
htrcli back [steps]                        # Go back 1 step, or N steps (e.g. back 3)
htrcli forward [steps]                     # Go forward 1 step, or N steps (e.g. forward 2)
htrcli reload                              # Reload page
```

All navigation commands wait for the destination page to finish loading
(`document.readyState === "complete"`, up to 25s) before returning. `back` and
`forward` fail with an explicit "No previous/forward page in this tab's
history" error when the tab has no entry to navigate to, instead of silently
succeeding. `back 3` / `forward 2` loop single-step navigations sequentially;
if history runs out partway the command stops and reports how many steps
succeeded (e.g. `back 2/3: No previous page ... (went back 1 step(s) before error)`).

### Interaction

```bash
htrcli click <selector>                    # Click element
htrcli dblclick <selector>                 # Double-click
htrcli fill <selector> <value>             # Clear and fill
htrcli type <selector> <value>             # Append text
htrcli hover <selector>                    # Hover
htrcli press <key>                         # Press key (keyDown + keyUp)
htrcli keydown <key>                       # Key down only (hold; use keyup to release)
htrcli keyup <key>                         # Key up only
htrcli mousedown <selector>  # or xy=100,200 for viewport coords
htrcli mouseup <selector>    # or xy=100,200
htrcli mousemove <selector>  # or xy=100,200
htrcli drag <source> <target> [--steps 5] [--delay 0]  # each endpoint may be a selector or xy=100,200; e.g. htrcli drag xy=100,200 xy=300,400 or htrcli drag "#handle" xy=500,300
htrcli select <selector> <value>           # Select dropdown
htrcli check <selector>                    # Check checkbox
htrcli uncheck <selector>                  # Uncheck checkbox
htrcli scroll <direction> [pixels]         # Scroll page
htrcli clear <selector>                    # Clear input
```

All mouse coordinates are viewport CSS pixels (same as CDP Input.dispatchMouseEvent).
`xy=` bypasses selector lookup and waiting, but synthetic Firefox input still uses
`document.elementFromPoint` to route the event to the element under the point;
the command fails explicitly when no element is hit. CDP coordinates are sent
directly to the protocol.

Drag `--steps` is clamped to `1..100` (default `5`) and `--delay` to
`0..2000ms` (default `0`) on both transports.

Interaction commands (`click`, `dblclick`, `rightrclick`, `fill`, `type`,
`clear`, `select`, `check`, `uncheck`, `press`, and the visible-only `hover`,
`focus`, `blur`, `scroll`, `selectText`, `highlight`) **auto-wait** for their
target to exist, be visible, and (where it matters) be enabled before acting.
The default budget is 5s; override it with `--timeout <ms>` (capped at 20s). If
the element never becomes actionable the command fails with a descriptive error
(`not found` / `not visible` / `disabled`). Read-only inspection commands
(`find`, `text`, `value`, `attr`, `html`, `page`, …) keep instant, probing
semantics and do not wait.


On Chrome, `click`, `press`, `keydown`/`keyup`, `mousedown`/`mouseup`/`mousemove`/`drag`, and `type` are dispatched as **trusted** input
events via the Chrome DevTools Protocol. The page's default actions fire as if a
real user interacted: pressing `Enter` in a field submits the form, clicks pass
`event.isTrusted` checks, and focus/selection behave natively. On Firefox (no
`chrome.debugger` API) the same commands use synthetic events (with pointer-event
support) — they drive most automation but do not count as trusted.
`drag` dispatches pointer/mouse events only; it does **not** fire native HTML5
`dragstart`/`dragover`/`drop` with `DataTransfer`. Use `eval` with `DataTransfer` for native DnD.
`keydown`/`keyup` are stateless per-command (no daemon-side held-key state) — the caller
tracks hold by pairing `keydown Shift` ... `keyup Shift`.

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
--context <name>                          # Named browser context (isolated profile)
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
- The native-messaging daemon on :3845 (`htrcli serve`)
- Go 1.22+ (for building from source)
- **ffmpeg ≥ 6** on `PATH` for video recording (`htrcli record`; `brew install ffmpeg`). Missing ffmpeg produces an explicit error at both `record start` and `record stop` — never a hang.

## Named Browser Contexts

`--context <name>` lazily launches or reuses an isolated Chrome profile when a
CDP command needs its debugging port, tracked in `~/.htrcli/contexts.json`.
Each context has its own `--user-data-dir` for true cookie/storage isolation.

```bash
# Launch or reuse a named context:
htrcli --context work --cdp open https://example.com
htrcli --context work --cdp click "#login"

# List registered contexts:
htrcli context list
```

## Video Recording (--cdp, Chrome only)

Record a page screencast to MP4 over CDP. `record start` spawns a detached
recorder process; `record stop` signals it and encodes via ffmpeg.

```bash
htrcli --cdp record start              # Begin recording
htrcli --cdp record stop output.mp4    # Stop and encode to MP4
```

The extension/Firefox transport returns an explicit "not supported" error.

## Session Recordings (Chrome + Firefox)

A *session recording* is a step-by-step log of what happened in the browser —
clicks, inputs, navigations — with a screenshot per step, plus annotations.
It lives in the extension's IndexedDB.

This is a different feature from `htrcli record` above: that one captures page
**video** to MP4 and needs `--cdp` + ffmpeg; this one needs neither, so it works
on **both Chrome and Firefox**. Use it for "what did the user do" (reproducible
step lists, bug reports, test authoring).

```bash
htrcli recordings start --title "Checkout flow"
htrcli click @e1
htrcli recordings stop

htrcli recordings status                        # is anything recording?
htrcli recordings list                          # newest first, paginated
htrcli recordings list --limit 20 --offset 20
htrcli recordings get <id>                      # steps + annotations, no screenshots
htrcli recordings get <id> --with-screenshots   # include base64 screenshots
htrcli recordings get <id> --output out.json    # write to a file
htrcli recordings export <id> out.json          # always with screenshots
htrcli recordings delete <id>

htrcli recordings list --browser firefox        # prefer the Firefox profile
```

Notes:

- `--browser chrome|firefox` names the browser profile you want. It is a
  **hint, not a selector**: the daemon prefers a relay that announced that
  browser and otherwise falls back to the earliest-connected relay, so a hint
  naming a profile that is not running still returns an answer rather than an
  error. `recordings list` prints `Answered by: <browser>` so you can always
  see which profile actually served the request. Omit the flag for plain
  first-connected-wins.

- Handled entirely by the extension's background service worker, so there is no
  `--tab`: a session recording always spans the whole browser profile.
- `get` strips base64 screenshots by default (a session with a few dozen steps
  is tens of megabytes) and reports `mediaStripped: true` when it does.
  `export` always includes them.
- **Screenshots have a hard 64 MiB ceiling.** The hydrated session must fit in
  one native-messaging frame, and an over-cap frame is treated as a protocol
  error that tears down the connection — costing you remote control until the
  extension reconnects. `get --with-screenshots` and `export` pre-check the
  step count and refuse up front rather than risking that.
- Handled by the extension's background service worker, so there is no
  `--tab`: a session recording always spans the whole browser profile. These
  commands go to `POST /api/background/command`, a tab-less route that picks a
  browser **connection** rather than a tab, so **no open page is required** —
  recording works from a `chrome://` page, a settings page, or a browser sitting
  on the new-tab screen. The only requirement is that the extension is running
  and its relay is connected, else you get `404 no browser connected`.
- `--audio` defaults to off, so a remote caller can never silently open the
  microphone.
- `start` refuses to clobber an in-flight session; `stop` it first. The same
  refusal applies from the extension's popup and side panel, since all three
  go through one `startRecording`.
- `delete` refuses to remove the session that is currently recording — it is
  not persisted yet, so deleting it would report success and then let the
  session reappear on the next `stop`.
- `--cdp` returns an explicit error — these live in the extension.
- The same recorder can be driven by hand from the extension's toolbar popup or
  side panel; recordings started either way show up in `htrcli recordings list`.

## Debug Trace Export

Export console + network events, a screenshot, and page info as a zip:

```bash
htrcli trace export trace-dump.zip
```

Network events are best-effort (logged on error, non-fatal).

## License

MIT
