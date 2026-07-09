---
name: htrcli
description: HTR NControl CLI (htcli) usage guide. Read this before running any htcli commands. Covers connecting to the HTR NControl server, listing and switching tabs, navigating pages, interacting with elements (click, fill, type, select, press), extracting text and data (text/html/attr/value/find), taking screenshots, executing JavaScript in the page's main world, managing browser sessions, and running the native messaging daemon. Use when the user asks to control a browser, interact with a website, fill a form, click something, extract data, take a screenshot, or automate any browser task via HTR NControl.
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
htcli find "input[name=q]"    # 2. Locate the element you want to act on
htcli click "input[name=q]"   # 3. Act on it (auto-waits for actionability)
htcli find "input[name=q]"    # 4. Re-inspect after any page change
```

`open`, `back`, `forward`, and `reload` block until the destination page
finishes loading (up to 25s), so the next command runs against the loaded
page. Clicks that *trigger* a navigation also block for the destination
page to finish loading — no manual polling needed.

Selectors (`"input[name=q]"`, `"#submit"`, `"role=button"`, `"text=Submit"`)
work directly; refs like `@e3` are not supported in the Go CLI (use
`htcli command` for low-level action names if you need them). All
interaction commands auto-wait for their target to become visible and
enabled (up to 5s by default, override with `--timeout`).

## Quickstart

```bash
# Take a screenshot of a page
htcli open https://example.com
htcli screenshot home.png
htcli health

# Search, click a result, and capture it
htcli open https://duckduckgo.com
htcli find "input[name=q]"               # locate the search input
htcli fill "input[name=q]" "htcli browser automation"
htcli press Enter
htcli find "input[name=q]"               # re-inspect after the change
htcli click "a[data-testid=result]"      # click a result
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

### Find element info

`htcli find <selector>` returns the full info (tag, attributes, bounding
box, text, children) for the first element matching a CSS selector,
accessibility role, or text match.

```bash
htcli find "h1"                         # first <h1> on the page
htcli find "#submit"                    # element by id
htcli find "role=button"                # by ARIA role
htcli find "text=Submit"                # by visible text
htcli find "input[name=q]" --json       # machine-readable
```

For multiple elements, use the raw `command` path:

```bash
htcli command '{"action":"findAll","target":{"selector":"a"}}'
```

### Get text, HTML, attributes, and values

```bash
htcli text  "h1"                        # visible text of an element
htcli html  "h1"                        # innerHTML
htcli attr  "a.nav" href                # any attribute value
htcli value "input[name=q]"             # current input value
```

Add `--json` to any of these to get structured output.

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

### Using selectors (the only way in the Go CLI)

Refs like `@e1` are not supported in the Go CLI — every command takes a
selector. The interaction subcommands are:

```bash
htcli click "#submit"                   # CSS selector (any of the forms below)
htcli dblclick ".row:first-child"
htcli fill  "input[name=email]" "user@test.com"
htcli type  "input[name=email]" " more text"  # append, doesn't clear
htcli hover ".menu-trigger"
htcli select "select#country" "us"
htcli check   "#terms"
htcli uncheck "#newsletter"
htcli clear   "input[name=email]"
htcli press   Enter                    # key, no selector
htcli focus   "#search"                # focus an element
htcli blur    "#search"                # blur an element
htcli scroll  down 300                 # direction + pixels
htcli scrollTo "#footer"               # scroll an element into view
```

Supported selector forms:

```bash
htcli click "#submit"                   # CSS selector (id, class, attribute, etc.)
htcli click "button.primary"

# Semantic shortcuts
htcli click "role=button"               # by ARIA role
htcli click "text=Submit"               # by visible text
htcli click "label=Email"               # by associated label
htcli click "name=email"                # by name attribute
htcli click "placeholder=Search"        # by placeholder
htcli click "xpath=//button[1]"         # by XPath
htcli click "id=login"                  # by ID
```

Selectors auto-wait for their target to become visible and enabled before
acting (default 5s; pass `--timeout` to change). An error like
`Element "..." was not found (waited 5000ms for it to become actionable)`
means the selector never resolved, was hidden, or was disabled — re-check
the page (`htcli find <candidate>` or `htcli command '{"action":"findAll",...}'`).

### Actionable-wait behavior

Every interaction command (`click`, `dblclick`, `rightclick`, `fill`, `type`,
`clear`, `select`, `check`, `uncheck`, `pressKey`, and the visible-only
`hover`, `focus`, `blur`, `scrollTo`, `selectText`, `highlight`) now
**auto-waits** for its target to exist, be visible, and (where it matters) be
enabled before acting. The default budget is 5s; tune it per command with
`--timeout <ms>` (capped at 20s). If the element never becomes actionable the
command fails with a descriptive error naming the unmet condition
(`not found` / `not visible` / `disabled`) instead of a bare "element not found".

This means you usually do **not** need to sleep or re-inspect before clicking
an element that is animating in or rendering lazily — the command waits for it.
Read-only inspection commands (`find`, `getText`, `getValue`, `isVisible`, …)
keep their instant, probing semantics and do **not** wait.


On Chrome, `click`, `press`/`type` are dispatched as **trusted** input via the
Chrome DevTools Protocol, so the page's default actions fire as if a real user
interacted: pressing `Enter` in a field submits the form, clicks pass
`event.isTrusted` checks, and focus/selection behave natively. On Firefox (no
`chrome.debugger` API) the same commands use synthetic events with pointer-event
support — they drive most automation but are not trusted.

While attached, Chrome shows the **“HTR NControl is debugging this browser”
infobar**; this is expected (it also appears for `eval`/`print` on Chrome).

If DevTools is open on the target tab (or another debugger client is attached),
the trusted-input attach fails and the command returns an explicit error naming
the conflict — it does **not** silently fall back to synthetic events. Close
DevTools on that tab and retry.
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

Agents fail more often from bad waits than from bad selectors. The
extension's auto-wait covers most cases (every interaction command waits
up to 5s for its target to become visible and enabled before acting), so
you usually **don't need an explicit wait** for actions that target a real
element.

For cases where the page transitions without a clear target (URL change,
network settle, custom loading state), use:

```bash
# Check current page state
htcli page                              # URL, title, readyState
htcli find ".success-message"           # poll for an element to appear

# Block on an element via the raw `command` path (waits up to the
# command timeout, fails loudly on timeout instead of returning null):
htcli command '{"action":"wait","target":{"selector":".success-message"},"options":{"timeout":10000}}'
```

For URL/readyState polling:

```bash
htcli page | grep Ready                 # should show "complete"
htcli eval 'document.readyState'        # returns "loading" | "interactive" | "complete"
```

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
htcli --tab 123 find "input[name=q]"
htcli --tab 123 click "input[name=q]"
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

For advanced use, send raw JSON commands. Useful for actions that don't
have a top-level subcommand (`wait`, `findAll`, custom targets) and for
bypassing auto-wait when needed:

```bash
htcli command '{"action":"click","target":{"selector":"#btn"}}'
htcli command '{"action":"fill","target":{"name":"email"},"value":"test@example.com"}'
htcli command '{"action":"findAll","target":{"selector":"a"}}'
htcli command '{"action":"wait","target":{"selector":".loaded"},"options":{"timeout":5000}}'
```

## Common workflows

### Log in to a site

```bash
htcli open https://example.com/login
htcli find "input[name=email]"          # verify the form is there
htcli fill "input[name=email]" "user@example.com"
htcli fill "input[name=password]" "password123"
htcli click "button[type=submit]"
htcli page                               # verify URL changed to dashboard
```

### Fill a multi-step form

```bash
htcli open https://example.com/apply
htcli find "#personal-info"              # confirm step 1 is loaded

# Step 1: Personal info (auto-wait handles the form transition)
htcli fill "input[name=firstName]" "John"
htcli fill "input[name=lastName]" "Doe"
htcli fill "input[name=email]" "john@example.com"
htcli click "button.next"

# Step 2: Address (page is fully loaded before the next fill runs)
htcli find "#address"
htcli fill "input[name=street]" "123 Main St"
htcli fill "input[name=city]" "Springfield"
htcli click "button.submit"
```

### Extract data from a page

```bash
htcli open https://example.com/products
# Pull every product card's name + price from the page's main world:
htcli eval "JSON.stringify(Array.from(document.querySelectorAll('.product')).map(el => ({name: el.querySelector('.name')?.textContent, price: el.querySelector('.price')?.textContent})))"
```

### Take documentation screenshots

```bash
htcli open https://example.com/dashboard
htcli screenshot documentation.png       # viewport (the only mode today)
```

> **Note:** the Go CLI's `screenshot` command captures the viewport only.
> Full-page and annotated capture exist in the extension's side-panel UI
> but are not yet wired into a CLI subcommand — track that as a future
> enhancement, or take multiple viewport screenshots and stitch with
> external tools.

### Debug a failing page

```bash
htcli page                              # check current URL, title, readyState
htcli eval "document.querySelector('.error')?.textContent"  # check for errors
htcli screenshot debug.png               # visual state
htcli find "input[name=email]"           # verify the form is in the DOM
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

### Element not found / not actionable

An error like `Element "..." was not found (waited 5000ms for it to become
actionable)` means the selector never resolved, was hidden, or was disabled.

1. Confirm the element is in the DOM: `htcli find <selector>`
2. Confirm it's visible: `htcli find <selector> | grep -i 'display\|visibility\|hidden'`
3. Confirm it's enabled: `htcli eval '!document.querySelector("...").disabled'`
4. If the element appears after a delay, the auto-wait should handle it;
   if you need longer than 5s, use `--timeout`
5. Take a screenshot to see the current state: `htcli screenshot debug.png`

### Stale selector after a page change

Page transitions invalidate selectors that target elements that were
re-rendered. Use a selector that survives the transition (semantic
attributes like `data-testid`, `aria-label`, `name` are more durable
than positional ones like `.row:nth-child(2)`), or re-inspect with
`htcli find` after the page changes.

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
| `htcli screenshot [path]` | Take screenshot (viewport only) |
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
| `htcli text <sel>` | Get text content |
| `htcli value <sel>` | Get input value |
| `htcli attr <sel> <attr>` | Get attribute |
| `htcli html <sel>` | Get innerHTML |
| `htcli eval <js>` | Execute JavaScript (page main world) |
| `htcli command <json>` | Send raw JSON command |
| `htcli fetch <url>` | Fetch URL via background (no popup, includes cookies) |
| `htcli printpdf <path>` | Print page to PDF via CDP (no save-as prompt) |

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
