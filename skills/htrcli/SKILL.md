---
name: htrcli
description: HTR NControl CLI (htcli) usage guide. Read this before running any htcli commands. Covers connecting to the HTR NControl server, listing and switching tabs, navigating pages, interacting with elements (click, fill, type, select), extracting text and data, taking screenshots (viewport, full page, annotated), getting accessibility tree snapshots with @eN refs, executing JavaScript, managing browser sessions, and running the native messaging daemon. Use when the user asks to control a browser, interact with a website, fill a form, click something, extract data, take a screenshot, or automate any browser task via HTR NControl.
allowed-tools: Bash(htcli:*), Bash(go run ./cmd/htcli:*), Bash(make htcli-*)
---

# htcli — HTR NControl CLI

Go CLI for controlling browser tabs via the HTR NControl remote control
API. Supports Chrome and Firefox.

```
# WebSocket transport (Bun server)
htcli (Go) ──HTTP──► Bun server (server/, :3845) ──WebSocket──► Extension ──DOM──► Chrome / Firefox

# Native messaging transport (htcli daemon — no Bun server needed)
htcli (Go) ──HTTP──► htcli serve (:3845) ──Unix socket──► relay ──stdio──► Extension ──DOM──► Chrome / Firefox
```

Two interchangeable server transports — pick one:
- **Bun server** (`bun run server`) — WebSocket-based, requires Node/Bun runtime
- **htcli daemon** (`htcli serve`) — native messaging, pure Go, no extra runtime

Both expose the same HTTP API on port 3845. Only one can hold the port at a time.

## Setup

### Build

```bash
cd /path/to/htrncontrol/htcli
make build         # → bin/htcli
make install       # go install (global)
```

Or from the repo root:

```bash
make htcli-build   # builds htcli
make htcli-install # installs globally
```

### Configure connection

```bash
htcli config set-server http://127.0.0.1:3845
htcli config set-token <bearer-token>

# Or use environment variables
export HTCLI_SERVER=http://127.0.0.1:3845
export HTCLI_TOKEN=<bearer-token>

# Verify connection
htcli health
```

Config file: `~/.htcli/config.json`
Priority: flags > env vars (`HTCLI_SERVER`, `HTCLI_TOKEN`) > config file > defaults.

If no token is configured, htcli will attempt to auto-read it from the server.

## Native Messaging Daemon

The daemon (`htcli serve`) is a drop-in replacement for the Bun server — same
HTTP API on :3845, but the browser connects via native messaging instead of
WebSocket. Supports Chrome and Firefox connected simultaneously.

```bash
# 1. Register htcli as the browser's native messaging host
htcli install --browser chrome  --extension-id <chrome-extension-id>
htcli install --browser firefox --extension-id htrcontrol@mercstudio.com

# 2. Reload the extension so it re-reads the host registration

# 3. Start the daemon (binds :3845 + Unix socket)
htcli serve
#    Custom port / token:
HTR_PORT=48546 HTR_BEARER_TOKEN=secret htcli serve
```

Chrome and Firefox may both be registered and connected at once —
`htcli tabs list` shows tabs from both, and `--tab <id>` routes to whichever
browser owns that tab.

### Install flags

```bash
htcli install --browser chrome  --extension-id <id>   # register Chrome
htcli install --browser firefox --extension-id <id>   # register Firefox
htcli install --browser chrome  --uninstall           # remove manifest
```

### Why use the daemon?

- No Bun/Node.js runtime required — pure Go
- Firefox support via native messaging (Chrome also works)
- Both browsers can be connected simultaneously
- Screenshots and large results travel over HTTP (not limited by 1 MB NM frame size)

## The core loop

```bash
htcli open <url>              # 1. Navigate to a page (waits for page load)
htcli snapshot -i             # 2. See what's on it (interactive elements only)
htcli click @e3               # 3. Act on refs from the snapshot
htcli snapshot -i             # 4. Re-snapshot after any page change
```

`open`, `back`, `forward`, and `reload` block until the destination page
finishes loading (up to 25s), so the next command runs against the loaded
page. Clicks that *trigger* a navigation do NOT wait — after such a click,
poll readiness before interacting (e.g. `htcli eval 'document.readyState'`
or re-snapshot and check the content).

Refs (`@e1`, `@e2`, ...) are assigned fresh on every snapshot. They become
**stale the moment the page changes** — after clicks that navigate, form
submits, dynamic re-renders, dialog opens. Always re-snapshot before your
next ref interaction.

## Quickstart

```bash
# Take a screenshot of a page
htcli open https://example.com
htcli screenshot home.png
htcli health

# Search, click a result, and capture it
htcli open https://duckduckgo.com
htcli snapshot -i                        # find the search box ref
htcli fill @e1 "htcli browser automation"
htcli press Enter
htcli snapshot -i                        # refs now reflect results
htcli click @e5                          # click a result
htcli screenshot result.png
```

## Global flags

```bash
--server <url>      # Server URL (overrides config)
--token <token>     # Bearer token (overrides config)
--json              # Raw JSON output (for piping to jq)
--tab <id>          # Target a specific tab (applies to all commands)
--timeout <ms>      # Command timeout (default: 30000)
```

## Reading a page

### Accessibility tree (preferred for AI agents)

```bash
htcli snapshot                          # full accessibility tree
htcli snapshot -i                       # interactive elements only (preferred)
htcli snapshot -i -u                    # include href URLs on links
htcli snapshot -i -c                    # compact (no empty structural nodes)
htcli snapshot -i -d 3                  # cap depth at 3 levels
htcli snapshot -s "#main"               # scope to a CSS selector
htcli snapshot --json                   # machine-readable output
```

Snapshot output looks like:

```
Page: Example - Log in
URL: https://example.com/login

@e1 heading "Log in" [level=1]
@e2 form
  @e3 textbox "Email" [required]
    @e4 placeholder="Enter your email"
  @e5 textbox "Password" [required]
    @e6 placeholder="Enter your password"
  @e7 button "Submit" [enabled]
  @e8 link "Forgot password?"
```

### Get text and attributes

```bash
htcli get text @e1                      # visible text of an element
htcli get html @e1                      # innerHTML
htcli get attr @e1 href                 # any attribute value
htcli get value @e1                     # input value
htcli find "#login-form"                # find element and return info
```

### Page info

```bash
htcli page                              # URL, title, viewport, scroll position
htcli page --json                       # machine-readable
```

Output:
```
URL:      https://example.com/login
Title:    Example - Login
Domain:   example.com
Ready:    complete                      # document.readyState
History:  3 entries                     # window.history.length
Viewport: 1280x720
Document: 1280x2400
Scroll:   0, 350
```

## Interacting

### Using refs (fastest)

```bash
htcli click @e7                         # click element by ref
htcli dblclick @e3                      # double-click
htcli fill @e3 "user@example.com"       # clear and fill input
htcli type @e3 "more text"              # append text to input
htcli hover @e5                         # hover element
htcli select @e9 "option-value"         # select dropdown option
htcli check @e10                        # check checkbox
htcli uncheck @e10                      # uncheck checkbox
htcli clear @e3                         # clear input field
```

### Using selectors (when refs don't work)

```bash
htcli click "#submit"                   # CSS selector
htcli fill "input[name=email]" "user@test.com"
htcli click "button.primary"

# By name, role, text, label, placeholder
htcli click "role=button"               # by ARIA role
htcli click "text=Submit"               # by text content
htcli click "label=Email"               # by label
htcli click "name=email"                # by name attribute
htcli click "placeholder=Search"        # by placeholder
htcli click "xpath=//button[1]"         # by XPath
htcli click "id=login"                  # by ID
```

Rule of thumb: snapshot + `@eN` refs are fastest and most reliable. Use
selectors as a fallback when refs don't work.

### Actionable-wait behavior

Every interaction command (`click`, `dblclick`, `rightclick`, `fill`, `type`,
`clear`, `select`, `check`, `uncheck`, `pressKey`, and the visible-only
`hover`, `focus`, `blur`, `scrollTo`, `selectText`, `highlight`) now
**auto-waits** for its target to exist, be visible, and (where it matters) be
enabled before acting. The default budget is 5s; tune it per command with
`--timeout <ms>` (capped at 20s). If the element never becomes actionable the
command fails with a descriptive error naming the unmet condition
(`not found` / `not visible` / `disabled`) instead of a bare "element not found".

This means you usually do **not** need to sleep or re-snapshot before clicking
an element that is animating in or rendering lazily — the command waits for it.
Read-only inspection commands (`find`, `getText`, `getValue`, `isVisible`, …)
keep their instant, probing semantics and do **not** wait.

### Keys

```bash
htcli press Enter                       # press a key
htcli press Tab
htcli press Control+a                   # select all
htcli press Escape
```

Supported keys: Enter, Tab, Escape, Backspace, Delete, ArrowUp, ArrowDown,
ArrowLeft, ArrowRight, Home, End, PageUp, PageDown, F1-F12,
Control+a-z, Alt+a-z, Shift+a-z, Meta+a-z.

### Scrolling

```bash
htcli scroll down                       # scroll down (default 500px)
htcli scroll up 300                     # scroll up 300px
htcli scroll left
htcli scroll right
```

## Waiting

Agents fail more often from bad waits than from bad selectors.

```bash
# After navigation or clicks that load new content:
htcli snapshot -i                       # re-snapshot to check if content loaded
htcli page                              # check URL changed

# Wait for specific element (poll with snapshot)
htcli snapshot -i -s ".success-message" # check if element appeared

# Wait for URL change (check page)
htcli page                              # verify URL updated
```

Always re-snapshot after any action that changes the page. The snapshot
itself serves as a "wait" — if the element you expect isn't there, the
page hasn't finished loading yet.

## Screenshots

### Viewport (default)

```bash
htcli screenshot                        # save to temp file, print path
htcli screenshot page.png               # save to specific path
```

### Full page

```bash
htcli screenshot --full                 # entire scrollable page
htcli screenshot --full full-page.png
```

### Annotated (with numbered element labels)

```bash
htcli screenshot --annotate             # viewport with numbered overlays
htcli screenshot --annotate --full      # full page + annotated
htcli screenshot --annotate shot.png    # save annotated screenshot
```

### Format options

```bash
htcli screenshot --format jpeg --quality 80   # JPEG instead of PNG
htcli screenshot --selector "#login-form"     # capture specific element
```

### JSON output (for piping)

```bash
htcli screenshot --json                 # returns base64 image data
htcli screenshot --json | jq -r '.data.screenshot' | base64 -d > img.png
```

## Tab management

```bash
htcli tabs list                         # list all connected tabs
htcli tabs get 123                      # get info for specific tab

# Target a specific tab for commands
htcli --tab 123 snapshot -i
htcli --tab 123 click @e5
```

## Navigation

```bash
htcli open https://example.com          # navigate to URL
htcli back                              # browser back (errors if no history)
htcli forward                           # browser forward (errors if no history)
htcli reload                            # reload page
```

All navigation commands block until the new page reaches
`document.readyState === "complete"` (up to 25s). `back` and `forward` return
an explicit "No previous page in this tab's history" / "No forward page in
this tab's history" error if the tab has no entry to go to, instead of silently
succeeding.

## JavaScript execution

```bash
htcli eval "document.title"             # run JS and return result
htcli eval "document.querySelectorAll('a').length"
htcli eval "window.scrollTo(0, 0)"
```

`eval` now supports both single expressions (`htcli eval "document.title"`)
and **multi-statement scripts with an explicit `return`** (e.g.
`htcli eval "const n = 2; return n * 2;"`). Scripts may also `await`
promises — `htcli eval "return await fetch('/api').then(r => r.json());"`. A
script that throws surfaces its own error message in the result.

`eval` runs in the **page's main world** (via Chrome DevTools Protocol) on
both transports, so it can see page-context JavaScript globals, React state,
and closures that an isolated-world script cannot. Async/await, multi-statement
scripts, and any return value are supported natively. On Firefox
(`chrome.debugger` unavailable) `eval` returns an explicit error — use
`debugEval` (Chrome-only) for a fallback path that still works on Firefox.

## Fetching and downloading (no popup)

These commands fetch data or save files **without triggering browser download popups** — everything runs silently via the extension background.

### Fetch a URL (with cookies)

```bash
htcli fetch <url>                       # POST by default
htcli fetch <url> --method GET          # explicit GET
htcli fetch <url> --method POST --body '{"key":"value"}'  # POST with JSON body
htcli fetch <url> --json                # raw JSON output
```

`fetch` runs through the extension background script, so it:
- Sends session cookies (`credentials: "include"`)
- Bypasses page CSP
- Returns JSON data directly to the CLI (no download dialog)

Use this to download API responses, JSON data, or any URL that returns structured data.

### Print page to PDF (no save-as prompt)

```bash
htcli printpdf output.pdf               # save current page as PDF
```

Uses Chrome DevTools Protocol (`Page.printToPDF`) to generate a PDF of the
current page **without a save-as dialog**. The PDF is saved directly to the
specified path. Useful for capturing reports, receipts, or any page content.

### Download via JavaScript (no popup)

For arbitrary file downloads without popups, use `eval` to fetch the content
and send it to the CLI:

```bash
# Download a file as base64, decode locally
htcli eval "fetch('https://example.com/file.pdf').then(r => r.arrayBuffer()).then(b => btoa(String.fromCharCode(...new Uint8Array(b))))" --json | jq -r '.data' | base64 -d > file.pdf
```

Or use `fetch` + write to a file:

```bash
htcli fetch https://example.com/api/data --json | jq '.data' > output.json
```

## Raw commands

For advanced use, send raw JSON commands:

```bash
htcli command '{"action":"click","target":{"selector":"#btn"}}'
htcli command '{"action":"fill","target":{"name":"email"},"value":"test@example.com"}'
htcli command '{"action":"snapshot","interactive":true,"compact":true}'
```

## Common workflows

### Log in to a site

```bash
htcli open https://example.com/login
htcli snapshot -i
htcli fill @e3 "user@example.com"
htcli fill @e5 "password123"
htcli click @e7
htcli snapshot -i                        # verify login succeeded
htcli page                              # check URL changed to dashboard
```

### Fill a multi-step form

```bash
htcli open https://example.com/apply
htcli snapshot -i

# Step 1: Personal info
htcli fill @e1 "John"
htcli fill @e2 "Doe"
htcli fill @e3 "john@example.com"
htcli click @e4                          # Next button

htcli snapshot -i                        # re-snapshot for step 2

# Step 2: Address
htcli fill @e1 "123 Main St"
htcli fill @e2 "Springfield"
htcli click @e3                          # Submit
```

### Extract data from a page

```bash
htcli open https://example.com/products
htcli snapshot -i -c -d 2               # compact tree, shallow depth
htcli snapshot --json | jq '.data.tree' # machine-readable
```

Or use JS evaluation:

```bash
htcli eval "JSON.stringify(Array.from(document.querySelectorAll('.product')).map(el => ({name: el.querySelector('.name')?.textContent, price: el.querySelector('.price')?.textContent})))"
```

### Take documentation screenshots

```bash
htcli open https://example.com/dashboard
htcli screenshot --annotate --full documentation.png
```

### Debug a failing page

```bash
htcli page                              # check current URL and state
htcli snapshot -i                        # see what elements are present
htcli eval "document.querySelector('.error')?.textContent"  # check for errors
htcli screenshot debug.png               # visual state
```

## Troubleshooting

### "No tabs connected"

The HTR NControl extension must be open and connected to the server.
1. Open Chrome/Firefox with the extension installed
2. Click the extension icon or open the side panel
3. Ensure remote control is enabled
4. Check: `htcli health` should show connected tabs > 0

### "403 Forbidden"

Token mismatch. Check the token matches what the server displayed on startup:
```bash
htcli config show                        # show current config
htcli health                             # test connection
```

### "Connection refused"

Server not running. Start one of:
```bash
# Option A: Bun server (WebSocket transport)
cd /path/to/htrncontrol && bun run server

# Option B: htcli daemon (native messaging — no Bun needed)
htcli serve
```

### Element not found

1. Re-snapshot to get fresh refs: `htcli snapshot -i`
2. Try a different selector strategy: CSS > name > role > text
3. Use `htcli find <selector>` to verify element exists
4. Use `htcli screenshot` to see current page state

### Refs don't work after navigation

Refs become stale after page changes. Always re-snapshot:
```bash
htcli click @e5                          # this navigates
htcli snapshot -i                        # fresh refs
htcli click @e3                          # use new ref
```

## Full reference

### Commands

| Command | Description |
|---------|-------------|
| `htcli health` | Check server connection |
| `htcli config set-server <url>` | Set server URL |
| `htcli config set-token <token>` | Set bearer token |
| `htcli config show` | Show current config |
| `htcli install --browser <b> --extension-id <id>` | Register as native messaging host |
| `htcli install --browser <b> --uninstall` | Remove native messaging manifest |
| `htcli serve` | Start native messaging daemon (:3845) |
| `htcli tabs list` | List connected tabs |
| `htcli tabs get <id>` | Get tab info |
| `htcli open <url>` | Navigate to URL |
| `htcli back` | Browser back |
| `htcli forward` | Browser forward |
| `htcli reload` | Reload page |
| `htcli snapshot` | Accessibility tree with refs |
| `htcli screenshot [path]` | Take screenshot |
| `htcli page` | Get page info |
| `htcli click <sel>` | Click element |
| `htcli dblclick <sel>` | Double-click element |
| `htcli fill <sel> <val>` | Clear and fill input |
| `htcli type <sel> <val>` | Append text to input |
| `htcli hover <sel>` | Hover element |
| `htcli press <key>` | Press key |
| `htcli select <sel> <val>` | Select dropdown option |
| `htcli check <sel>` | Check checkbox |
| `htcli uncheck <sel>` | Uncheck checkbox |
| `htcli scroll <dir> [px]` | Scroll page |
| `htcli clear <sel>` | Clear input field |
| `htcli find <sel>` | Find element info |
| `htcli get text <sel>` | Get text content |
| `htcli get value <sel>` | Get input value |
| `htcli get attr <sel> <attr>` | Get attribute |
| `htcli get html <sel>` | Get innerHTML |
| `htcli eval <js>` | Execute JavaScript |
| `htcli command <json>` | Send raw JSON command |
| `htcli fetch <url>` | Fetch URL via background (no popup, includes cookies) |
| `htcli printpdf <path>` | Print page to PDF via CDP (no save-as prompt) |
|------|-------------|
| `-i`, `--interactive` | Only interactive elements |
| `-c`, `--compact` | Compact output |
| `-d`, `--depth <n>` | Max tree depth |
| `-s`, `--selector <sel>` | Scope to element |
| `-u`, `--urls` | Show URLs in links |
| `--json` | JSON output |

### Screenshot flags

| Flag | Description |
|------|-------------|
| `--full` | Full page capture |
| `--annotate` | Numbered element overlays |
| `--format <fmt>` | png (default) or jpeg |
| `--quality <n>` | JPEG quality 1-100 |
| `--selector <sel>` | Capture specific element |
| `--json` | Base64 JSON output |

### Global flags

| Flag | Description |
|------|-------------|
| `--server <url>` | Server URL (overrides config) |
| `--token <token>` | Bearer token (overrides config) |
| `--json` | Raw JSON output |
| `--tab <id>` | Target specific tab |
| `--timeout <ms>` | Command timeout (default: 30000) |

### Environment variables

| Variable | Description |
|----------|-------------|
| `HTCLI_SERVER` | Server URL |
| `HTCLI_TOKEN` | Bearer token |
| `HTR_PORT` | Daemon port (default: 3845) |
| `HTR_BEARER_TOKEN` | Daemon bearer token |
