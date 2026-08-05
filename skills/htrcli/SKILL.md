---
name: htrcli
description: HTR NControl CLI (htrcli) usage guide. Read this before running any htrcli commands. Covers connecting to the HTR NControl server, listing and switching tabs, navigating pages, interacting with elements (click, fill, type, select, press), extracting text and data (text/html/attr/value/find), taking screenshots, executing JavaScript in the page's main world, managing browser sessions, recording video, network capture/mocking, console watching, dialog handling, and more. Use when the user asks to control a browser, interact with a website, fill a form, click something, extract data, take a screenshot, or automate any browser task via HTR NControl.
allowed-tools: Bash(htrcli:*), Bash(go run ./cmd/htrcli:*), Bash(make htrcli-*)
---

# htrcli — HTR NControl CLI

Go CLI for controlling browser tabs via the HTR NControl remote control API.
Supports **two transports** — extension (default) and direct CDP:

```
# Extension transport (default) — drives the browser through the extension
htrcli (Go) ──HTTP──► htrcli serve (:3845) ──Unix socket──► relay ──stdio──► Extension ──DOM──► Chrome / Firefox

# CDP transport (--cdp) — drives Chrome directly via DevTools Protocol, no extension needed
htrcli (Go) ──CDP──► Chrome DevTools Protocol (:9222)
```

The extension transport works with both Chrome and Firefox; CDP transport works
with Chrome only.

## Setup

### Build

```bash
cd /path/to/htrncontrol/htrcli
make build         # → bin/htrcli
make install       # go install (global)
```

Or from the repo root:

```bash
make htrcli-build   # builds htrcli
make htrcli-install # installs globally
```

### Configure connection

```bash
htrcli config set-server http://127.0.0.1:3845
htrcli config set-token <bearer-token>

# Or use environment variables
export HTRCLI_SERVER=http://127.0.0.1:3845
export HTRCLI_TOKEN=<bearer-token>

# Verify connection
htrcli health
```

Config file: `~/.htrcli/config.json`
Priority: flags > env vars (`HTRCLI_SERVER`, `HTRCLI_TOKEN`) > config file > defaults.

If no token is configured, htrcli will attempt to auto-read it from the server.

## Native Messaging Daemon

The daemon (`htrcli serve`) is the sole backend. It exposes the HTTP API on
:3845 and relays commands to the extension via native messaging. Supports
Chrome and Firefox connected simultaneously.

```bash
# 1. Register htrcli as the browser's native messaging host
htrcli install --browser chrome  --extension-id <chrome-extension-id>
htrcli install --browser firefox --extension-id htrncontrol@mercstudio.com

# 2. Reload the extension so it re-reads the host registration

# 3. Start the daemon (binds :3845 + Unix socket)
htrcli serve
#    Custom port / token:
HTR_PORT=48546 HTR_BEARER_TOKEN=secret htrcli serve
```

### Install flags

```bash
htrcli install --browser chrome  --extension-id <id>   # register Chrome
htrcli install --browser firefox --extension-id <id>   # register Firefox
htrcli install --browser chrome  --uninstall           # remove manifest
```

Chrome and Firefox may both be registered and connected at once —
`htrcli tabs list` shows tabs from both, and `--tab <id>` routes to whichever
browser owns that tab.

### Tray icon

When you run `htrcli serve` on a desktop (macOS, Windows, Linux with a
display), a system-tray icon auto-attaches. It exposes live status and
maintenance actions (reinstall native host, open config folder, copy bearer
token, show recent log, restart, quit). On headless Linux servers (no
display, or SSH session), the tray is silently skipped. See
`htrcli/docs/tray.md` for the full menu and `--no-tray` opt-out.

### CDP transport (direct Chrome DevTools Protocol)

By default `htrcli` drives the browser through the extension. With `--cdp`
(or `htrcli config set-transport cdp`) it instead talks **directly to Chrome
over CDP** — no extension and no server required. Use this for:

- **Browser-restricted pages** the extension can't reach (e.g. Chrome Web Store dev console, `chrome://` URLs).
- **Headless / background automation** — run Chrome with no window and drive it from a cron job or CI.

`--cdp` is only supported by commands that explicitly implement CDP;
unsupported commands fail with `errUnsupportedCDP(...)`. Commands that support
CDP use it directly.

```bash
# Start a dedicated Chrome controlled by htrcli
htrcli browser start                 # visible window
htrcli browser start --headless      # no window

htrcli browser status                # probe the debugging port
htrcli browser stop                  # kill the managed Chrome
htrcli browser hide                  # minimize the window
htrcli browser show                  # restore the window

# Commands that support CDP use it directly
htrcli --cdp open https://chrome.google.com
htrcli --cdp screenshot out.png
htrcli --cdp eval "document.title"
```

**Tab-ID namespaces:**

| Transport | `--tab` value | Example |
|---|---|---|
| extension (`ext`, default) | numeric tab ID from `htrcli tabs list` | `--tab 43` |
| CDP (`cdp`) | 32-char hex CDP target ID | `--tab 8E17C9D2...` |

**Configuration:**

```bash
htrcli config set-transport cdp        # make --cdp the default
htrcli config set-cdp-port 9222        # debugging port (default 9222)
htrcli config set-chrome-path /path/to/chrome   # if not auto-detected
```

## Global flags

```bash
--server <url>      # Server URL (overrides config)
--token <token>     # Bearer token (overrides config)
--json              # Raw JSON output (for piping)
--tab <id>          # Target specific tab
--timeout <ms>      # Command timeout (default: 30000)
--transport <type>  # Transport: ext (extension, default) or cdp
--cdp               # Shorthand for --transport cdp
--context <name>    # Named browser context (isolated profile)
```

## The core loop

```bash
htrcli open <url>              # 1. Navigate to a page (waits for page load)
htrcli find "input[name=q]"    # 2. Locate the element you want to act on
htrcli click "input[name=q]"   # 3. Act on it (auto-waits for actionability)
htrcli find "input[name=q]"    # 4. Re-inspect after any page change
```

`open`, `back`, `forward`, and `reload` block until the destination page
finishes loading (up to 25s). Clicks that *trigger* a navigation also block
for the destination page to finish loading.

Selectors (`"input[name=q]"`, `"#submit"`, `"role=button"`, `"text=Submit"`)
and **refs** (`@e3`, `@e7`) work directly in all interaction commands. All
interaction commands auto-wait for their target to become visible and enabled
(up to 5s by default, override with `--timeout`).

## Quickstart

```bash
# Take a screenshot of a page
htrcli open https://example.com
htrcli screenshot home.png
htrcli health

# Search, click a result, and capture it
htrcli open https://duckduckgo.com
htrcli find "input[name=q]"               # locate the search input
htrcli fill "input[name=q]" "browser automation"
htrcli press Enter
htrcli screenshot result.png

# Use a ref for repeated interaction
htrcli find "input[name=q]" --ref         # mints a ref like @e3
htrcli fill @e3 "new search term"
htrcli press Enter
```

## Page info

```bash
htrcli page                    # URL, title, readyState, dimensions, scroll position
htrcli page --json             # machine-readable output
```

Example output:
```
URL:      https://example.com/login
Title:    Example - Login
Domain:   example.com
Ready:    complete
Viewport: 1280x720
Document: 1280x2400
Scroll:   0, 350
```

## Interacting

### Selectors and refs

Every interaction command accepts CSS selectors, semantic shortcuts, or refs:

```bash
htrcli click "#submit"                   # CSS selector
htrcli click "role=button"               # by ARIA role
htrcli click "text=Submit"               # by visible text
htrcli click "label=Email"               # by associated label
htrcli click "name=email"                # by name attribute
htrcli click "placeholder=Search"        # by placeholder
htrcli click "xpath=//button[1]"         # by XPath
htrcli click "id=login"                  # by ID

# Refs — persistent handles minted by `--ref` on find/findAll
htrcli find "#my-form" --ref             # mint @e3
htrcli click @e3                         # use the ref
htrcli fill @e3 "value"                  # fills the form
```

`find --ref` saves the ref to `~/.htrcli/refs.json`. Refs survive the CLI
invocation but not page navigation (the element goes stale). Use `findAll`
with `--ref` to mint refs for every match.

### Interaction commands

```bash
htrcli click "#submit"                   # Click element
htrcli dblclick ".row:first-child"       # Double-click
htrcli fill  "input[name=email]" "user@test.com"   # Clear and fill
htrcli type  "input[name=email]" " more text"      # Append, doesn't clear
htrcli hover ".menu-trigger"
htrcli select "select#country" "us"
htrcli check   "#terms"                  # Check a checkbox
htrcli uncheck "#newsletter"             # Uncheck a checkbox
htrcli clear   "input[name=email]"       # Clear an input
htrcli press   Enter                     # Press a key (no selector)
htrcli scroll  down 300                  # Scroll direction + pixels
```

Supported key names: Enter, Tab, Escape, Backspace, Delete, ArrowUp, ArrowDown,
ArrowLeft, ArrowRight, Home, End, PageUp, PageDown, F1–F12,
Control+a–z, Alt+a–z, Shift+a–z, Meta+a–z.

### Actionable-wait behavior

Every interaction command (`click`, `fill`, `type`, `clear`, `select`, `check`,
`uncheck`, `press`, `hover`) **auto-waits** for its target to exist, be visible,
and (where relevant) be enabled before acting. Default budget: 5s; tune per
command with `--timeout <ms>` (capped at 20s). If the element never becomes
actionable the command fails with a descriptive error naming the unmet condition
(`not found` / `not visible` / `disabled`).

Read-only inspection commands (`find`, `text`, `value`, `attr`, `html`) keep
instant, probing semantics and do **not** wait.

On the CDP transport, `click`, `press`, `type`, `fill` are dispatched as
**trusted** input via the Chrome DevTools Protocol, so the page's default
actions fire as if a real user interacted: pressing `Enter` in a field submits
the form, clicks pass `event.isTrusted` checks. On the extension transport
(and Firefox), the same commands use synthetic events with pointer-event
support.

While connected via CDP, Chrome shows the **"HTR NControl is debugging this
browser" infobar**; this is expected.

## Waiting

Agents fail more often from bad waits than from bad selectors. The
auto-wait covers most cases (every interaction command waits up to 5s for
its target to become visible and enabled). For page transitions without a
clear target:

```bash
# Check current page state
htrcli page                              # URL, title, readyState
htrcli find ".success-message"           # poll for an element

# Block on an element via raw command
htrcli command '{"action":"wait","target":{"selector":".success-message"},"options":{"timeout":10000}}'

# Wait for a network request to complete
htrcli network wait --since 0 --url "*/api/users*" --status 200 --timeout 10000
```

For URL/readyState polling:

```bash
htrcli page | grep Ready                 # should show "complete"
htrcli eval 'document.readyState'        # "loading" | "interactive" | "complete"
```

## Screenshots

### Viewport (default)

```bash
htrcli screenshot                        # save to temp file, print path
htrcli screenshot page.png               # save to specific path
```

### Full page

```bash
htrcli screenshot --full-page            # entire scrollable page
htrcli screenshot --full-page full-page.png
```

### Annotated (with numbered element labels)

```bash
htrcli screenshot --annotate "#form,#submit"   # viewport with numbered overlays on selectors
htrcli screenshot --full-page --annotate "#nav,.content"   # full page + annotations
```

`--annotate` takes a comma-separated list of selectors to draw numbered
overlay boxes on before capture (extension transport only).

### Format options

```bash
htrcli screenshot --format jpeg --quality 80   # JPEG instead of PNG
htrcli screenshot --selector "#login-form"     # capture specific element
```

### JSON output (for piping)

```bash
htrcli screenshot --json                 # returns base64 image data
htrcli screenshot --json | jq -r '.data.screenshot' | base64 -d > img.png
```

## Element inspection

```bash
htrcli find <selector>              # Element info (tag, text, selector, xpath, visibility, bounding box)
htrcli findAll <selector>           # All matching elements
htrcli find <selector> --ref        # Mint a persistent ref (@eN) for later use
htrcli findAll <selector> --ref     # Mint refs for every match
htrcli text <selector>              # Text content
htrcli value <selector>             # Input value
htrcli attr <selector> <attr>       # Attribute value (e.g. href, src)
htrcli html <selector>              # innerHTML
htrcli snapshot                     # Accessibility snapshot tree with refs
```

## Tab management

```bash
htrcli tabs list                           # list all connected tabs
htrcli tabs get 123                        # get info for specific tab

# Target a specific tab for commands
htrcli --tab 123 find "input[name=q]"
htrcli --tab 123 click "input[name=q]"
```

## Navigation

```bash
htrcli open https://example.com          # navigate to URL
htrcli back                              # browser back (errors if no history)
htrcli forward                           # browser forward (errors if no history)
htrcli reload                            # reload page
```

All navigation commands block until the new page reaches
`document.readyState === "complete"` (up to 25s).

## JavaScript execution

```bash
htrcli eval "document.title"             # run JS and return result
htrcli eval "document.querySelectorAll('a').length"
htrcli eval "window.scrollTo(0, 0)"
```

`eval` supports single expressions, multi-statement scripts with an explicit
`return`, and `async/await`:

```bash
htrcli eval "const n = 2; return n * 2;"
htrcli eval "return await fetch('/api').then(r => r.json());"
```

`eval` runs in the **page's main world** on both extension and CDP transports,
so it can see page-context JavaScript globals, React state, and closures.
On Firefox (`chrome.debugger` unavailable) `eval` returns an explicit error.

## Fetching and downloading (no popup)

### Fetch a URL (with cookies)

```bash
htrcli fetch <url>                       # POST by default
htrcli fetch <url> --method GET          # explicit GET
htrcli fetch <url> --method POST --body '{"key":"value"}'  # POST with JSON body
htrcli fetch <url> --json                # raw JSON output
```

`fetch` runs through the extension background script, so it:
- Sends session cookies (`credentials: "include"`)
- Bypasses page CSP
- Returns JSON data directly to the CLI (no download dialog)

### Print page to PDF (no save-as prompt)

```bash
htrcli printpdf output.pdf               # save current page as PDF
```

Uses the extension path to generate a PDF without a save-as dialog.
Extension-only; not available in direct `--cdp` mode.

### Upload files (no file picker)

```bash
htrcli upload "input[type=file]" /path/to/file.pdf          # selector
htrcli upload @e3 /path/to/photo.jpg,/path/to/doc.pdf       # ref, multiple files
```

Sets files on a file input without triggering an OS file-picker dialog.
Works on both extension and CDP transports. On the **extension transport**
Chrome uses `chrome.debugger` (`DOM.setFileInputFiles`); **Firefox** (no
`chrome.debugger`) gets the file bytes embedded as base64 and the content
script assigns them via `File` + `DataTransfer` — no browser difference from
the caller's perspective. `@eN` refs are CDP-only.

## Console events

The daemon buffers page console output. Read or watch it with cursor-based
polling:

```bash
htrcli console read --since 0                  # read all buffered entries
htrcli console watch --since 100 --timeout 10000   # stream new entries
```

`console read` warns when the buffer evicted older entries.

## Network capture and mocking

The daemon captures page network activity into the same cursor-based buffer:

```bash
# Read buffered network entries
htrcli network read --since 0

# Stream new entries
htrcli network watch --since 100 --timeout 15000

# Block until a matching request completes
htrcli network wait --since 0 --url "*/api/users*" --status 200 --timeout 10000
```

`network wait` accepts a glob `--url` pattern (`path.Match` semantics, `*`
spans any character including `/`) and an optional `--status` filter.

### Mocking and blocking

```bash
# Mock a GET /api/user response
htrcli network mock --url-pattern "*/api/user" --method GET --status 200 --body-file ./mock.json

# Block (fail) matching requests
htrcli network block --url-pattern "*/api/analytics*"

# Remove a rule
htrcli network unmock --url-pattern "*/api/user"

# Remove all rules
htrcli network unmock --all
```

`network mock` flags: `--url-pattern` (required), `--method`, `--status`
(default 200), `--body-file` (file path for response body).

## Dialog handling

The daemon can auto-handle JavaScript dialogs (alert/confirm/prompt) and
record their results:

```bash
# Accept the next dialog (default)
htrcli dialog handle --action accept

# Dismiss the next dialog
htrcli dialog handle --action dismiss

# Respond with text to a prompt
htrcli dialog handle --action respond --text "my answer"

# List handled dialogs since cursor 0
htrcli dialog list --since 0
```

## Video recording (Chrome/CDP only)

Record the page to video via CDP screencast:

```bash
htrcli record start             # start recording
htrcli record stop              # stop and encode to MP4
```

Requires ffmpeg ≥ 6 on PATH.

## Trace export

Export a debug trace bundle (console logs + network entries + screenshot +
page info) as a zip:

```bash
htrcli trace export             # bundle everything into a timestamped zip
```

## Browser contexts

Manage isolated browser contexts (separate cookie jars, storage):

```bash
htrcli context list             # list named contexts
```

Use with the `--context` global flag:

```bash
htrcli --context work open https://example.com
htrcli --context personal open https://other.com
```

## Publishing to AMO

```bash
htrcli publish --build                     # build + sign + submit (public)
htrcli publish --channel unlisted          # self-distributed
htrcli publish --dry-run --source-dir firefox/build  # dry-run
# Submit source code as the AMO source submission (2nd upload). AMO requires
# human-readable source when the built add-on is bundled/minified:
htrcli publish --upload-source-code htrncontrol-src-0.4.6.zip
```

Channels: `listed` (default, public on addons.mozilla.org), `unlisted`
(self-distributed).

AMO API credentials (key + secret) resolved from:
1. `--api-key` / `--api-secret` flags
2. Environment: `AMO_API_KEY` / `AMO_API_SECRET` (or `HTRCLI_AMO_API_KEY` / `HTRCLI_AMO_API_SECRET`)
3. Config: `htrcli config set-amo-api-key <key>` / `htrcli config set-amo-api-secret <secret>`

## Raw commands

For advanced use, send raw JSON commands:

```bash
htrcli command '{"action":"click","target":{"selector":"#btn"}}'
htrcli command '{"action":"fill","target":{"name":"email"},"value":"test@example.com"}'
htrcli command '{"action":"findAll","target":{"selector":"a"}}'
htrcli command '{"action":"wait","target":{"selector":".loaded"},"options":{"timeout":5000}}'
```

## Common workflows

### Log in to a site

```bash
htrcli open https://example.com/login
htrcli find "input[name=email]"          # verify the form is there
htrcli fill "input[name=email]" "user@example.com"
htrcli fill "input[name=password]" "password123"
htrcli click "button[type=submit]"
htrcli page                               # verify URL changed to dashboard
```

### Fill a multi-step form

```bash
htrcli open https://example.com/apply
htrcli find "#personal-info"              # confirm step 1 is loaded

# Step 1: Personal info
htrcli fill "input[name=firstName]" "John"
htrcli fill "input[name=lastName]" "Doe"
htrcli fill "input[name=email]" "john@example.com"
htrcli click "button.next"

# Step 2: Address (page is fully loaded before the next fill runs)
htrcli find "#address"
htrcli fill "input[name=street]" "123 Main St"
htrcli fill "input[name=city]" "Springfield"
htrcli click "button.submit"
```

### Extract data from a page

```bash
htrcli open https://example.com/products
# Pull every product card's name + price
htrcli eval "JSON.stringify(Array.from(document.querySelectorAll('.product')).map(el => ({name: el.querySelector('.name')?.textContent, price: el.querySelector('.price')?.textContent})))"
```

### Take documentation screenshots

```bash
htrcli open https://example.com/dashboard
htrcli screenshot documentation.png       # viewport
htrcli screenshot --full-page full.png    # full page
htrcli screenshot --annotate "#header,#sidebar,#main"  # annotated
```

### Debug a failing page

```bash
htrcli page                              # check current URL, title, readyState
htrcli eval "document.querySelector('.error')?.textContent"  # check for errors
htrcli screenshot debug.png               # visual state
htrcli find "input[name=email]"           # verify the form is in the DOM
```

### Use refs for repeated interaction

```bash
htrcli find "#login-form" --ref          # mint @e3
htrcli find @e3                          # re-inspect
htrcli fill @e3 "admin"                  # use as a target
```

## Troubleshooting

### "No tabs connected"

The HTR NControl extension must be open and connected to the server (or CDP
transport must be active).

1. Open Chrome/Firefox with the extension installed
2. Click the extension icon or open the side panel
3. Ensure remote control is enabled
4. Check: `htrcli health` should show connected tabs > 0

### "403 Forbidden"

Token mismatch. Check the token matches what the server displayed on startup:

```bash
htrcli config show                        # show current config
htrcli health                             # test connection
```

### "Connection refused"

Server not running. Start the daemon:

```bash
htrcli serve
```

### Element not found / not actionable

An error like `Element "..." was not found (waited 5000ms for it to become
actionable)` means the selector never resolved, was hidden, or was disabled.

1. Confirm the element is in the DOM: `htrcli find <selector>`
2. Take a screenshot: `htrcli screenshot debug.png`
3. If the element appears after a delay, the auto-wait should handle it;
   if you need longer than 5s, use `--timeout`
4. For lazy-loading content, try `htrcli scroll down` first

### Stale ref after page navigation

Refs (`@eN`) are tied to the DOM element at the time they were minted.
Page transitions invalidate them. Re-mint the ref with `find <selector> --ref`
after the page loads.

## Full reference

### Commands

| Command | Description |
|---------|-------------|
| `htrcli health` | Check server connection |
| `htrcli config set-server <url>` | Set server URL |
| `htrcli config set-token <token>` | Set bearer token |
| `htrcli config set-extension-id <id>` | Set extension ID (for tray reinstall) |
| `htrcli config set-transport <type>` | Set default transport (ext/cdp) |
| `htrcli config set-cdp-port <port>` | Set CDP debugging port |
| `htrcli config set-chrome-path <path>` | Set Chrome binary path |
| `htrcli config set-amo-api-key <key>` | Set AMO API key |
| `htrcli config set-amo-api-secret <secret>` | Set AMO API secret |
| `htrcli config show` | Show current config |
| `htrcli install` | Register as native messaging host |
| `htrcli serve` | Start native messaging daemon (:3845) |
| `htrcli tabs list` | List connected tabs |
| `htrcli tabs get <id>` | Get tab info |
| `htrcli open <url>` | Navigate to URL |
| `htrcli back` | Browser back |
| `htrcli forward` | Browser forward |
| `htrcli reload` | Reload page |
| `htrcli screenshot [path]` | Take screenshot (viewport, --full-page, --annotate) |
| `htrcli page` | Get page info |
| `htrcli click <sel>` | Click element |
| `htrcli dblclick <sel>` | Double-click element |
| `htrcli fill <sel> <val>` | Clear and fill input |
| `htrcli type <sel> <val>` | Append text to input |
| `htrcli hover <sel>` | Hover element |
| `htrcli press <key>` | Press key |
| `htrcli select <sel> <val>` | Select dropdown option |
| `htrcli check <sel>` | Check checkbox |
| `htrcli uncheck <sel>` | Uncheck checkbox |
| `htrcli scroll <dir> [px]` | Scroll page |
| `htrcli clear <sel>` | Clear input field |
| `htrcli find <sel>` | Find element info |
| `htrcli findAll <sel>` | Find all elements matching selector |
| `htrcli text <sel>` | Get text content |
| `htrcli value <sel>` | Get input value |
| `htrcli attr <sel> <attr>` | Get attribute |
| `htrcli html <sel>` | Get innerHTML |
| `htrcli snapshot` | Accessibility snapshot tree with refs |
| `htrcli eval <js>` | Execute JavaScript (page main world) |
| `htrcli command <json>` | Send raw JSON command |
| `htrcli fetch <url>` | Fetch URL via background (includes cookies) |
| `htrcli printpdf <path>` | Print page to PDF via the extension path (no save-as prompt) |
| `htrcli upload <sel> <file>` | Set files on a file input (no file picker) |
| `htrcli console read` | Read buffered console events |
| `htrcli console watch` | Stream console events until timeout |
| `htrcli network read` | Read buffered network requests |
| `htrcli network watch` | Stream network entries until timeout |
| `htrcli network wait` | Block until a matching request completes |
| `htrcli network mock` | Mock responses for matching requests |
| `htrcli network block` | Block (fail) matching requests |
| `htrcli network unmock` | Remove mock/block rules |
| `htrcli dialog handle` | Arm dialog handling policy |
| `htrcli dialog list` | List handled dialogs |
| `htrcli browser start` | Launch CDP-controlled Chrome |
| `htrcli browser stop` | Kill managed Chrome |
| `htrcli browser status` | Probe CDP port |
| `htrcli browser hide` | Minimize CDP browser window |
| `htrcli browser show` | Restore CDP browser window |
| `htrcli context list` | List named browser contexts |
| `htrcli record start` | Start video recording (CDP only) |
| `htrcli record stop` | Stop recording and encode to MP4 |
| `htrcli trace export` | Export debug trace bundle (zip) |
| `htrcli publish` | Build + sign + submit to addons.mozilla.org |

### Global flags

| Flag | Description |
|------|-------------|
| `--server <url>` | Server URL (overrides config) |
| `--token <token>` | Bearer token (overrides config) |
| `--json` | Raw JSON output |
| `--tab <id>` | Target specific tab |
| `--timeout <ms>` | Command timeout (default: 30000) |
| `--transport <type>` | Transport: ext (default) or cdp |
| `--cdp` | Shorthand for --transport cdp |
| `--context <name>` | Named browser context |

### Environment variables

| Variable | Description |
|----------|-------------|
| `HTRCLI_SERVER` | Server URL |
| `HTRCLI_TOKEN` | Bearer token |
| `HTR_PORT` | Daemon port (default: 3845) |
| `HTR_BEARER_TOKEN` | Daemon bearer token |
| `AMO_API_KEY` / `HTRCLI_AMO_API_KEY` | AMO API key for publishing |
| `AMO_API_SECRET` / `HTRCLI_AMO_API_SECRET` | AMO API secret for publishing |
