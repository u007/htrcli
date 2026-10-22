---
name: htrcli
description: HTR NControl CLI (htrcli) usage guide. Read this before running any htrcli commands. Covers connecting to the HTR NControl server, listing, filtering by browser, opening and closing tabs, navigating pages, interacting with elements (click, fill, type, select, press, keydown/keyup, mousedown/mouseup/mousemove/drag with xy= viewport coordinates and @eN refs), extracting text and data (text/html/attr/value/find), taking screenshots, executing JavaScript in the page's main world, managing browser sessions, recording video, session recordings (start/stop/list/get/export/delete — works on Chrome and Firefox), network capture/mocking, console watching, dialog handling, and more. Includes a form-filling guide (pick/fill-form/--frame, datepickers, react-select, iframes, submit-and-verify) and the list of what works over --cdp. Use when the user asks to control a browser, interact with a website, fill a form, click something, drag, press keys, extract data, take a screenshot, or automate any browser task via HTR NControl.
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

## Agent quick rules (read this first; skip the probing)

Everything you need is in this file. Do **not** run `htrcli --help`, `htrcli <cmd> --help`, `htrcli health`,
`strings`/`grep` on the binary, or read htrcli's source: the commands, flags and CDP limits are listed here and
each probe costs a round trip.

1. **Transport.** If the task says a CDP browser is running (or you started one with `htrcli browser start`),
   put `--cdp` on **every** command. `health` is an extension-daemon command and says nothing about CDP; skip it.
2. **Tab.** `--cdp` without `--tab` drives the **first page target**, which may be someone else's tab, and
   `open` replaces its page. If more than one tab exists, run `htrcli --cdp tabs list` once and pass
   `--tab <id>` on every command. Never navigate a tab you did not open or were not given.
3. **Navigate by clicking, not by URL.** After entering the site, follow its links and buttons the way a user
   would: `find` the link/button, `click` it, then verify the page. Do not `open` a URL you constructed or guessed
   from the site's structure. Use `open` only for the entry URL you were given, or when no on-page path exists
   (and say so). Clicking runs the site's own navigation (SPA routing, referrer, state set along the way); a typed
   URL can 404 or skip steps the site expects.
4. **Look once, then act.** One `htrcli --cdp snapshot` (or one `eval` that lists
   `tag/type/name/id/label/required` of every `input,select,textarea,button`) tells you the whole form. Do not
   re-inspect between fields unless the page changed.
5. **Batch.** Do not issue one tool call per field. Put all steps for a form in **one bash script** (or use
   `fill-form`, below), finish with a single read-back, and fix only what the read-back shows wrong.
6. **Scrolling is automatic.** An element that is rendered but below the fold is scrolled into view before
   `fill`/`click`/`check`/... act. "not visible" now means hidden/collapsed/zero-size (open its step, accordion or
   menu first), not "off screen". `scroll` itself is not available over `--cdp`.
7. **Pick the right verb for the widget** (table below), and **verify**: read values back, submit **once**, then
   confirm from the outcome (confirmation text/URL, or `network wait`, see below).
8. **Never** fill a field you were not given data for, solve a CAPTCHA, enter card details, or submit when told
   not to. Hidden/off-screen inputs (honeypots) stay empty.

### Over `--cdp`: what is and is not available

| Works | Not available over `--cdp` (use `eval` or the extension transport) |
|---|---|
| open, find/findAll, snapshot, text/value/attr/html, click/dblclick/rightclick, fill, type, select, check/uncheck, clear, press/keydown/keyup, mouse*, drag, upload, eval, screenshot, tabs list/get/new/close (Chrome only), **pick**, **fill-form**, **--frame**, console read/watch, network read/watch/wait | scroll, back/forward/reload, dialog handle/list, fetch, printpdf, network mock/block/unmock, trace export, screenshot --annotate |

### Filling forms: verb per widget

| Widget | Use |
|---|---|
| text, email, tel, password, textarea, `type=time` (`18:30`), `type=number` | `fill "<sel>" "<value>"` |
| native `<select>` | `pick "<sel>" "<option text or value>"` (verifies; plain `select` silently selects nothing for a label it can't match) |
| custom dropdown / react-select / autocomplete (role=combobox) | `pick "<sel>" "<full option text>"`: opens, types, waits for the options, clicks the best match, and checks the widget shows it. Never `fill`/`click` the tiny input. |
| checkbox / radio | `check "<sel>"` / `uncheck "<sel>"` (idempotent; `click` toggles). Target by `input[value=x]` or `label=...`, not position. |
| contenteditable (rich text: Gmail reply box, Docs body) | `type "<sel>" "<value>"` — **appends** at the caret end; it never wipes the quoted text or the signature. `fill` **replaces** the whole content (correct for a spreadsheet cell, wrong for a reply box). |
| JS datepicker text input | `type "<sel>" "03/14/2026"` (trusted keystrokes; `fill` leaves the widget's own state empty and it reverts), in the widget's format; then click elsewhere and read the value back |
| calendar popup (grid of day buttons) | click the trigger, click next/prev until the month label matches, click the day |
| range / colour input | `eval` set `.value` and dispatch `input`+`change` events (bubbles), then read back |
| file input | `upload "<sel>" /abs/path` (never click it; it opens a native dialog). Only upload files you were told to. |
| form inside an `<iframe>` (same origin) | add `--frame "<iframe css>"` to find/fill/type/check/select/click/pick/value/text/... or `open` the frame's `src` directly. Cross-origin frames: `open` their `src`. |

**`fill-form`: many fields, one call, with read-back.**

```bash
htrcli --cdp fill-form '{
  "#first_name": "Jordan",
  "#email": "jordan@example.com",
  "#plan": "Pro",
  "input[value=pro]": true,
  "#terms": true,
  "#resume": "/abs/path/resume.pdf"
}'            # or: --file fields.json ; add --frame "iframe#contact" for an iframe form
```

Values: string/number as written (text, date, time), option text/value for `<select>`, `true`/`false` for
checkboxes (`true` only for radios), a path (or array) for file inputs, `null` skips. Comboboxes behave like `pick`.
Each selector must match exactly one element. All fields are attempted even after a failure, then a table
`Selector / Requested / Actual / Status` (`OK|MISMATCH|FAILED|SKIPPED`) is printed; the exit code is non-zero on
any `FAILED`/`MISMATCH`. Read-back is exact: `<select>`/combobox rows compare against what the fill verified
it set (never a substring of the request), and text rows are compared after the browser's own value
sanitization, so an empty or partial request cannot pass for a field the fill never touched.

**Submit and verify (do this once).** To confirm a submission, start the wait first, then click:

```bash
htrcli --cdp network wait --url "*/submit*" --timeout 10000 &
htrcli --cdp click "button[type=submit]"
wait
htrcli --cdp text "body"     # confirmation text; do not re-click Submit "to be sure"
```

A submit that does nothing is almost always a validation error: read the page for the message (or
`eval` over `:invalid` fields), fix the data (e.g. match the required `pattern`), then submit again. Do not
remove the validation attribute.

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
browser owns that tab. Each browser numbers its own tabs, so the same ID can
exist in both: `tabs get` and `tabs close` then fail until you add
`--browser chrome|firefox`. Plain `--tab <id>` on other commands takes the
earliest-connected browser that owns the ID. With `--browser`, `--tab <id>` is
strict: the tab must belong to that browser, or the command fails.

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
| extension (`ext`, default) | numeric tab ID from `htrcli tabs list` (per browser; see below) | `--tab 43` |
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
--frame <css>       # (--cdp) run the command inside a same-origin <iframe>
```

## The core loop

```bash
htrcli open <url>              # 1. Enter the site at the URL you were given (waits for page load)
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

Every interaction command accepts CSS selectors, semantic shortcuts, refs, or viewport coordinates:

```bash
htrcli click "#submit"                   # CSS selector
htrcli click "role=button"               # by ARIA role
htrcli click "text=Submit"               # by visible text
htrcli click "label=Email"               # by associated label
htrcli click "name=email"                # by name attribute
htrcli click "placeholder=Search"        # by placeholder
htrcli click "xpath=//button[1]"         # by XPath
htrcli click "id=login"                  # by ID
htrcli click "xy=100,200"                # viewport CSS pixels (no element lookup)

# Refs — persistent handles minted by `--ref` on find/findAll
htrcli find "#my-form" --ref             # mint @e3
htrcli click @e3                         # use the ref
htrcli fill @e3 "value"                  # fills the form
htrcli click @e3                         # refs also work for mousedown/drag on CDP
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
htrcli press   Enter                     # Press a key (keyDown + keyUp)
htrcli scroll  down 300                  # Scroll direction + pixels

# Low-level keyboard primitives (stateless — caller tracks hold)
htrcli keydown Shift                     # Key down only (hold)
htrcli keyup   Shift                     # Key up to release
htrcli keydown "Ctrl+a"                  # Modifier + key

# Low-level mouse primitives (selector, @eN ref, or xy= coordinates)
htrcli mousedown "#handle"               # Press mouse button down
htrcli mousedown "xy=100,200"            # At viewport coordinates
htrcli mouseup   "#dropzone"             # Release mouse button
htrcli mousemove "xy=150,300"            # Move mouse (no button)

# Drag (pointer/mouse events only — not native HTML5 DnD)
htrcli drag "#handle" "#dropzone"                         # Selector → selector
htrcli drag "xy=100,200" "xy=300,400"                      # Coords → coords
htrcli drag "#handle" "xy=500,300" --steps 10 --delay 20  # Mixed + tuning
htrcli drag @e1 @e2                                         # Ref → ref (CDP resolves via backendNodeId)

Both endpoints are resolved in one page-side pass, so scrolling for one cannot move the
other; if source and destination cannot both be visible in the viewport at once, `drag`
errors instead of dispatching at stale coordinates.
```

Supported key names: Enter, Tab, Escape, Backspace, Delete, ArrowUp, ArrowDown,
ArrowLeft, ArrowRight, Home, End, PageUp, PageDown, F1–F12,
Control+a–z, Alt+a–z, Shift+a–z, Meta+a–z.

### Viewport coordinates (`xy=`)

Any selector argument can be `xy=X,Y` — viewport CSS pixels (same units as CDP `Input.dispatchMouseEvent`):

```bash
htrcli mousedown "xy=100,200"
htrcli mousemove "xy=300,400"
htrcli drag "xy=100,200" "xy=300,400"
```

`xy=` bypasses selector lookup and actionable-wait, but the transport still needs a target element to route the event:

- **CDP transport** — coordinates are sent directly to `Input.dispatchMouseEvent`.
- **Extension transport (Firefox)** — synthetic events are hit-tested with `document.elementFromPoint(x, y)` and dispatched on the element under the point. If no element is hit, the command fails explicitly. See `docs/gotchas/firefox-coordinate-input-requires-hit-testing.md`.

Coordinates are always **viewport-relative**, not document-relative. Scroll position matters.

### Low-level mouse primitives

`mousedown` / `mouseup` / `mousemove` are single-event primitives built on the same trusted/synthetic path as `click`/`press`:

- They accept any selector form: CSS, `role=`, `text=`, `label=`, `name=`, `placeholder=`, `xpath=`, `id=`, `@eN` ref, or `xy=X,Y`.
- Element targets auto-wait for actionability (visible + enabled, up to 5 s) and scroll into view before the event — same as `click`. `xy=` targets skip the wait.
- On Chrome/CDP they dispatch **trusted** `Input.dispatchMouseEvent` (`mousePressed` / `mouseReleased` / `mouseMoved`); on Firefox/extension they dispatch synthetic pointer + mouse events with hit-testing.

Use `mousemove` to position the cursor without pressing a button; pair `mousedown` → … → `mouseup` to hold and release manually, or use `drag` for an interpolated sequence.

### Drag

`htrcli drag <source> <target> [--steps 5] [--delay 0]` dispatches an interpolated drag: `mousePressed` at source → N `mouseMoved` steps → `mouseReleased` at target.

- Both endpoints accept any selector form (CSS, `@eN`, or `xy=X,Y`) and can be mixed (`"#handle"` → `"xy=500,300"`). On CDP, `@eN` refs are looked up in the command layer via the persistent `RefStore` (`~/.htrcli/refs.json`) → `backendNodeId`, then handed to the page-side drag resolver as a live element handle (`DOM.resolveNode` + `Runtime.callFunctionOn`), so a ref is scrolled into view and re-read in the same pass as a selector endpoint — never a pixel captured before the other endpoint's scroll. Stale refs fail explicitly. See `docs/gotchas/cdp-drag-element-ref-coordinate-resolution.md`.
- **Bounds**: `--steps` is clamped to `1..100` (default `5`; values `<1` become `5`); `--delay` is clamped to `0..2000` ms (default `0`). See `docs/gotchas/drag-input-bounds.md`.
- On the extension transport, Firefox drag dispatches pointer/mouse events with `elementFromPoint` hit-testing at source, intermediate steps, and destination — not on `document.body`.
- **Known limitation — native HTML5 DnD is NOT synthesized**: `drag` dispatches `pointerdown`/`mousedown` → `pointermove`/`mousemove` → `pointerup`/`mouseup` only. It does **not** fire `dragstart`/`dragover`/`drop` with `DataTransfer`. Most custom sortables/sliders listen to pointer/mouse events and work; native `draggable=true` drop zones and file-drop handlers require `eval` with a manual `DataTransfer`. See `TODO.md` and `src/contentScript/commandExecutor.ts:handleDrag`.

### Low-level keyboard primitives

```bash
htrcli press Enter          # composite: keyDown + keyUp
htrcli keydown Shift        # hold
htrcli keydown "Ctrl+a"
htrcli keyup Shift          # release
```

- `press` is the composite trusted key press (dispatches `keyDown` then `keyUp`).
- `keydown` / `keyup` are **stateless per-command** — there is no daemon-side held-key state. Holding a modifier across several commands is caller-managed (`keydown Shift` → … → `keyup Shift`). Modifiers are parsed per-command (`Ctrl+Shift+a` → bitmask) and empty key specs fail explicitly. See `docs/gotchas/drag-input-bounds.md` and `htrcli/README.md`.
- Key events target the focused element when no selector is supplied; with a selector they auto-wait and focus that element first. On Chrome/CDP they use `Input.dispatchKeyEvent` with virtual-key/code mapping; on Firefox/extension they dispatch synthetic `KeyboardEvent`s.

### Actionable-wait behavior

Every element-targeted interaction command (`click`, `fill`, `type`, `clear`, `select`, `check`,
`uncheck`, `press`, `hover`, `keydown`, `keyup`, `mousedown`, `mouseup`, `mousemove`, `drag`) **auto-waits** for its target to exist, be visible,
and (where relevant) be enabled before acting. Default budget: 5 s; tune per
command with `--timeout <ms>` (capped at 20 s). If the element never becomes
actionable the command fails with a descriptive error naming the unmet condition
(`not found` / `not visible` / `disabled`). `xy=` coordinate targets skip the wait (no element lookup), but on Firefox/extension the command still fails explicitly when `elementFromPoint` hits nothing.

Read-only inspection commands (`find`, `text`, `value`, `attr`, `html`) keep
instant, probing semantics and do **not** wait.

On the CDP transport, `click`, `dblclick`, `rightclick`, `press`, `keydown`/`keyup`, `mousedown`/`mouseup`/`mousemove`/`drag`, and `type`/`fill` are dispatched as
**trusted** input via the Chrome DevTools Protocol (`Input.dispatchMouseEvent` / `Input.dispatchKeyEvent`), so the page's default
actions fire as if a real user interacted: pressing `Enter` in a field submits
the form, clicks pass `event.isTrusted` checks, and focus/selection behave natively. On the extension transport
and Firefox (no `chrome.debugger`), the same commands use synthetic events with pointer-event
support — they drive most automation but do not count as trusted. `drag` pointer/mouse dispatch follows the same split (trusted on CDP, synthetic with hit-testing on Firefox). `drag` never synthesizes native HTML5 `dragstart`/`dragover`/`drop` + `DataTransfer` on either transport — use `eval` with `DataTransfer` for native DnD.

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

## Retrying flaky commands

Some pages are slow or intermittently fail (a click races a re-render, a list
is still loading). `retry` runs another htrcli subcommand and re-runs it with
exponential backoff on a non-zero exit. It is **opt-in** — normal commands
never retry — and every failed attempt is printed to stderr, so a failure is
never silent. The command exits non-zero when all attempts fail.

```bash
htrcli retry --times 3 --delay 500 -- wait --selector ".results-table" --timeout 8000
htrcli retry --times 5 --delay 250 --max-delay 5000 -- extract table "table"
htrcli retry --times 3 -- --transport cdp wait --selector ".results-table"   # transport AFTER `--`
```

- `--times N` — retries after the first attempt (default 3; total attempts = N+1)
- `--delay MS` — initial backoff (default 500ms; doubles each attempt)
- `--max-delay MS` — cap on the backoff (default 10000ms)

**Put a transport selector AFTER the `--`.** `htrcli --transport cdp retry -- …` also works (the parent forwards the flags it was given), but the after-`--` form is the one that always has.

**`retry` re-runs the WHOLE subcommand**, so it must not wrap a non-idempotent
action: a `click` on submit, `fill-form`, `tabs new`/`tabs close`, or a send can
fire twice. Retry only reads and waits (`wait`, `text`, `find`, `collect`,
`extract`, `page`). Use it for genuinely transient failures only; if a selector
is wrong, retrying just delays the error — fix the selector instead.

## Benchmarking

`htrcli bench run <scenario-file>` runs a JSON scenario file and writes a
report. It is how the office-task benchmark
(`docs/benchmarks/office-task-benchmark.md`) is executed.

```bash
htrcli bench run docs/benchmarks/scenarios/b01-gmail-find-emails.json --transport cdp
htrcli bench run scenario.json --out /tmp/report.json
```

A scenario is a list of steps, each a raw htrcli argv plus its expectations:

```json
{
  "scenarios": [
    {
      "id": "B01",
      "requires_auth": true,
      "steps": [
        {"argv": ["open", "https://mail.google.com/"],
         "expect": {"exit": 0},
         "retry": 2}
      ]
    }
  ]
}
```

- `expect` may assert `exit` (default 0), `stdoutContains`,
  `stdoutNotContains` and `jsonPath` (`{"path": "rows.0.Name",
  "equals": "Alice"}`).
- `retry: N` reuses the `retry` subcommand, so the backoff policy lives in
  one place.
- `optional: true` downgrades a failing step to PARTIAL.
- `requires_auth: true` is reported as NOT_RUN, never as PASS.
- `${VAR}` in an argv expands from the environment.

Verdicts: PASS / PARTIAL / FAIL / NOT_RUN. The report holds the per-step
exit code, attempts, wall time and output, plus the per-scenario
`commands`/`wall_ms`/`retries`/`failure_mode` metrics.

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
htrcli findAll <selector>           # All matching elements (viewport only!)
htrcli collect <selector>            # Every match in a VIRTUALIZED list (scrolls + accumulates)
htrcli collect <selector> --max-items 100        # Stop after N rows
htrcli collect <selector> --max-steps 200        # Scroll-step ceiling (default 200)
htrcli collect <selector> --step-delay 120       # ms between scroll steps (default 120)
htrcli collect <selector> --ref      # Mint refs for every collected row, at harvest time
htrcli extract table <selector>     # HTML table -> {columns, rows}
htrcli extract list <selector>       # List of items -> {items}
htrcli extract list <selector> --fields "name:.name,age:.age"
htrcli extract list <selector> --fields "title:.title,due:@data-due"   # @attr = an attribute
htrcli find <selector> --ref        # Mint a persistent ref (@eN) for later use
htrcli findAll <selector> --ref     # Mint refs for every match
htrcli text <selector>              # Text content
htrcli value <selector>             # Input value
htrcli attr <selector> <attr>       # Attribute value (e.g. href, src)
htrcli html <selector>              # innerHTML
htrcli snapshot                     # Accessibility snapshot tree with refs
```

**Virtualized lists:** `findAll` only sees the rows currently in the DOM. Long
lists (Gmail threads, Drive rows, chat messages, Sheets) recycle row elements,
so `findAll` returns a truncated slice. Use `collect`, which finds the list's
scroll container, scrolls it step by step and accumulates every row it sees:

```bash
htrcli collect "tr.zA" --json    # every Gmail row
```

It reports `steps`, `truncated`, `truncatedBy`, `scrolled` and `keyStrategy`:

| `keyStrategy` | meaning | risk |
|---|---|---|
| `key` | rows carry `data-id` / `data-item-id` / `data-msg-id` | none — reliable |
| `text` | rows have no id; de-duplicated by text | rows with identical text may be merged |
| `index` | rows have neither id nor text | node-identity de-duplication — may duplicate or drop rows |

`truncatedBy` says what stopped collection early: `maxItems` (the `--max-items`
cap) or `maxSteps` (the step limit, raise `--max-steps`). It is `null` when the
list was read to its end. The caveats are printed as warnings on stderr in the
default output, so check them before trusting a row count.

## Tab management

```bash
htrcli tabs list                           # list all connected tabs (Chrome and Firefox)
htrcli tabs list --browser firefox         # only Firefox tabs (chrome|firefox)
htrcli tabs get 123                        # get info for a tab
htrcli tabs get 123 --browser chrome       # same, when the ID also exists in another browser
htrcli tabs new https://example.com        # open a URL in a new tab (not activated); prints its ID
htrcli tabs new https://example.com --browser firefox
htrcli tabs close 123 --browser chrome     # close a tab

# Target a specific tab for commands
htrcli --tab 123 find "input[name=q]"
htrcli --tab 123 click "input[name=q]"
```

`--browser` on the `tabs` subcommands is a strict filter: it never falls back
to the other browser, and it is separate from the global `--browser` hint used
by tab-less commands. Pass it after `tabs`, e.g. `htrcli tabs list --browser
firefox`.

- `tabs new <url>` opens the tab from the extension's background worker, so no
  connected page is needed. With `--browser`, only that browser answers and the
  command fails if it is not connected; without it, the earliest-connected
  browser answers. The output names the browser, e.g. `Opened tab 7 (chrome)`,
  and `--json` carries it as `browser`. Pass the ID to `--tab`.
- `tabs close <id>` runs in the owning browser's background worker, so it no
  longer needs another tab in that browser. It still resolves the ID against the
  connected http(s) tabs, so a tab must be connected before it can be closed.
- Over `--cdp`, `tabs list`, `tabs get`, `tabs new`, and `tabs close` work on
  Chrome. `--browser firefox` is rejected because CDP only drives Chrome.

## Navigation

```bash
htrcli open https://example.com          # entry point only; within the site, click links instead (quick rule 3)
htrcli back [steps]                      # browser back — 1 step, or N steps (e.g. back 3)
htrcli forward [steps]                   # browser forward — 1 step, or N steps
htrcli reload                            # reload page
```

All navigation commands block until the new page reaches
`document.readyState === "complete"` (up to 25 s). `back` and `forward` fail with an explicit "No previous/forward page in this tab's history" error when history runs out; `back 3` / `forward 2` loop single-step navigations sequentially and report partial progress (e.g. `back 2/3: No previous page … (went back 1 step(s) before error)`). See `htrcli/README.md` for the full navigation contract.

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

### Console and network over `--cdp`

There is no buffer over `--cdp` (each invocation is a fresh CDP session), and
`--since` is rejected:

- `console read` — messages + uncaught exceptions V8 still stores for the
  current document (cleared on navigation). `console watch` — new ones only.
- `network read` — completed requests from the page's Performance API: url,
  status, initiator type, duration. **No method (`—`) and no bodies.**
- `network watch` / `network wait` — only requests that complete **after** the
  command attached (method, status, url). To confirm a submission, start the
  wait first, then click:

```bash
htrcli --cdp network wait --url "*/api/submit*" --status 200 --timeout 10000 &
htrcli --cdp click "#submit"
wait
# Missed it? `htrcli --cdp network read` still shows its status.
```

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

## Session recordings (Chrome AND Firefox)

A **session recording** is a step-by-step log of what happened in the browser
— clicks, inputs, navigations — with a screenshot per step, plus your
annotations. It lives in the extension's IndexedDB.

This is **not** the same as `htrcli record` above. Use this section for
"what did the user do" (reproducible step lists, bug reports, test authoring);
use `htrcli record` for page **video**/MP4.

Unlike video, session recordings need no CDP and no ffmpeg, so they work on
**both Chrome and Firefox**. They are handled by the extension's background
service worker, so there is no `--tab` — they always apply to the whole
browser profile.

```bash
# Record a flow from the CLI
htrcli recordings start --title "Checkout flow"
htrcli click @e1
htrcli fill @e2 "hello"
htrcli recordings stop

htrcli recordings status                  # is anything recording?
htrcli recordings list                    # newest first, paginated
htrcli recordings list --limit 20 --offset 20
htrcli recordings get <id>                # steps + annotations (NO screenshots)
htrcli recordings get <id> --with-screenshots   # include base64 screenshots
htrcli recordings delete <id>
```

`export` always includes screenshots, and picks its format from the output
file's extension:

```bash
htrcli recordings export <id> out.json    # raw payload, base64 screenshots inline
htrcli recordings export <id> out.zip     # bundle: recording.json + README.md + screenshots/ + audio/
htrcli recordings export <id> out.md      # human-readable timeline
```

The `.zip` layout is identical to the side panel's "Export ZIP", so bundles are
interchangeable:

```
recording.json      manifest — screenshots/audio referenced by PATH, never base64
README.md           the timeline
screenshots/step_1.png, screenshots/annotation_1.png
audio/step_1.webm,      audio/annotation_1.webm
```

Prefer `.zip` when you want the actual screenshot files on disk. An
unrecognised extension falls back to JSON.

### Starting a recording manually

The same recorder can be driven from the extension UI, which is the fastest
way to hand-capture a flow:

- **Toolbar popup** — click the HTR NControl icon: optional title, an
  "Record audio" checkbox, Start/Stop, and a list of recent recordings.
- **Side panel** — the full session view (live step list, annotations, export).

A recording started from the popup, the side panel, or the CLI is
indistinguishable afterwards: all three write to the same IndexedDB store and
sync the open side panel. So you can start from the UI and finish with
`htrcli recordings stop`, or vice versa.

### Notes

- **Screenshots are stripped by default** in `get`. Every step carries a
  full-page PNG data URL, so a session with a few dozen steps runs into the
  tens of megabytes. `get` reports `mediaStripped: true` when it did so. Use
  `--with-screenshots`, or `recordings export` (which always includes them),
  when you actually need images.
- **There is a hard 64 MiB ceiling on screenshots.** The whole hydrated
  session has to fit in ONE native-messaging frame, and both ends cap it at
  64 MiB (`htrcli.MaxMessageSize`). Base64 inflates PNGs by ~4/3, so ~48 MiB
  of raw screenshot data is already over the line. An over-cap frame is
  treated as a protocol error and **tears down the connection** — the
  extension then loses remote control until it reconnects, so every later
  command fails. `get --with-screenshots` and `export` therefore pre-check the
  step count from `list` and refuse up front (over ~120 steps) with an
  actionable message rather than risking the connection. If you hit it, use
  `get` without screenshots, or record the flow in shorter sessions.
- **`list` is sorted newest-first and paginated.** `total` is the count
  *before* the window, so paging is deterministic.
- **`--audio` is off by default** so a remote caller can never silently open
  the microphone.
- `recordingStart` refuses to clobber an in-flight session (it would discard
  in-memory steps) — `recordings stop` first. The same refusal applies from
  the popup and side panel, since they all go through the one `startRecording`.
- `recordingDelete` refuses to delete the session that is currently
  recording: it is not in the store yet, so deleting it would report success
  and then have the session reappear on the next `stop`.
- `--cdp` is rejected with a clear error: these live in the extension.
- `--browser chrome|firefox` names the browser profile you want. It is a
  **hint, not a selector**: the daemon prefers a relay that announced that
  browser and otherwise falls back to the earliest-connected relay, so a hint
  naming a profile that is not running still returns an answer rather than an
  error. `recordings list` prints `Answered by: <browser>` so you can always
  see which profile actually served the request. Omit the flag for plain
  first-connected-wins.
- No open page is required. These commands use the tab-less route
  `POST /api/background/command`, which selects a browser **connection** rather
  than a tab, so recording works from a `chrome://` page, a settings page, or a
  browser on the new-tab screen. The only requirement is a connected extension
  relay, else you get `404 no browser connected`.
- Sensitive input values are masked by the extension before storage; steps
  carry `isSensitive: true` so you know the value was redacted.

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

## Office apps: Gmail, Calendar, Docs, Sheets, Tasks

These recipes were verified against the local fixtures in
`docs/benchmarks/fixtures/` (same DOM shapes as the real apps, no real data).
**Run them against a test account only** — a send, an invite, a share or a
cell edit is a real write.

Run the fixtures yourself:

```bash
make bench-smoke                              # the harness smoke scenario
htrcli --cdp open "file://$PWD/docs/benchmarks/fixtures/gmail.html"
```

### Gmail — find emails (B01)

Gmail virtualizes the message list: only the visible rows exist in the DOM, so
`findAll` returns a partial slice. Use `collect` (scrolls and accumulates),
then `extract` to turn rows into records.

```bash
htrcli --cdp open "https://mail.google.com/mail/u/0/#search/is%3Aunread"
htrcli --cdp wait --selector "div[role='main']" --timeout 15000
htrcli --cdp collect "div[role='main'] tr"          # every row, scrolls the list
htrcli --cdp extract list "div[role='main'] tr" \
  --fields "sender:.yW .span,subject:.yP .span,date:.xW .span"
```

`extract list` returns `{"count": N, "items": [{sender, subject, date}]}`.
A selector that names the ROWS (`.row`, `tr`) returns one item per match; a
selector that names the CONTAINER (`ul#items`) returns the container's
children. Both are supported — do not pick the first match and assume the
rest.

`collect` prints a warning when it de-duplicates by text (`keyStrategy`), which
means identical-looking rows may have been merged. Prefer a stable field
(`data-id`, a link href) when the list has one.

### Gmail — reply (B02)

The reply box is a **contenteditable** that already holds the quoted thread
and your signature.

- `type` **appends** (it places the caret at the end). It never wipes the
  signature or the quoted text.
- `fill` **replaces** the whole content — never use it on a reply box.

```bash
htrcli --cdp click "div[role='textbox']"          # focus the reply box
htrcli --cdp type "div[role='textbox']" "Thanks — see the attached."
htrcli --cdp text "div[role='textbox']"           # READ BACK before sending
```

Always read the box back before sending. If the text does not contain your
body **and** the signature, stop and fix it — do not send.

### Calendar — create an event (B03)

Date and time fields are **JS datepickers**: `fill` sets the text but leaves
the widget's own state empty and it reverts. Use `type` and read back.

Guest autocomplete is a combobox: use `pick` with the full option text, never
`fill` on the small input.

```bash
htrcli --cdp fill "#title" "Team sync"
htrcli --cdp type "#date" "10/22/2026"              # widget's own format
htrcli --cdp type "#time" "09:30"
htrcli --cdp value "#date"                          # read back
htrcli --cdp pick "#guest-input" "Dana Example"     # combobox
htrcli --cdp value "#guest-input"                   # read back
```

The day picker is a grid of buttons: click the trigger, click next/prev until
the month label matches, then click the day. There is no `pick` for a date
grid.

### Docs — edit and share (B05)

The editing surface is a contenteditable, not an `<input>`: `fill` is not the
verb. Type into the surface, and use `press` for formatting (headings, lists).

```bash
htrcli --cdp click "#doc-surface"                   # focus the document body
htrcli --cdp type "#doc-surface" "New paragraph."
htrcli --cdp press "Enter"
htrcli --cdp text "#doc-surface"                    # read back
```

Share opens a dialog: fill the people field, then confirm the dialog reports
the new permission. `pick` is the verb for a share-to combobox.

### Sheets — data entry and formulas (B06)

Cells are contenteditable. `fill` replaces a cell's content (correct for a
cell); `type` appends (wrong for a cell).

```bash
htrcli --cdp click '[data-col="A"][data-row="1"]'
htrcli --cdp fill '[data-col="A"][data-row="1"]' "Widget"
htrcli --cdp fill '[data-col="B"][data-row="1"]' "12"
htrcli --cdp text '[data-col="A"][data-row="1"]'   # read back
```

Cell navigation is keyboard-driven: `press "Tab"` moves right, `press "Enter"`
moves down. Read the grid back as a table when you need the values:

```bash
htrcli --cdp extract table "#grid"
```

A formula cell is typed like any other cell (`=SUM(A1:A9)`); the computed
value is what the grid shows after the sheet recalculates — read it back
before trusting it.

### Tasks — todo lists and reminders (B04)

Google Tasks has **two ways** to add a task, and they need different verbs.

**Quick-add**: one field. The whole task, including its date, is typed into
it and committed with `Enter`.

```bash
htrcli --cdp type "#quick-add" "Send the report 10/22/2026 5pm"
htrcli --cdp press "Enter"          # commit — the field is not submitted by typing
```

**Full form**: a details panel, and the date is chosen from a **popover grid
of day buttons** — there is no date field to type into. Click the trigger,
then click the day.

```bash
htrcli --cdp click "#show-details"   # reveal the panel
htrcli --cdp fill "#task-title" "Book the room"
htrcli --cdp click "#date-trigger"   # open the date popover
htrcli --cdp click "#day-grid button[data-day='2026-10-23']"
htrcli --cdp attr "#date-trigger" "data-due"   # read the chosen date back
htrcli --cdp click "#save"
```

**The checkbox is an ARIA checkbox**, not an `<input type=checkbox>`: it is a
`<div role="checkbox" aria-checked="false">`. `check` / `uncheck` drive
`aria-checked` and dispatch a click so the app's own handler runs. They are
idempotent — the state ends up as asked either way.

```bash
htrcli --cdp check   "#task-list [role=checkbox]"
htrcli --cdp uncheck "#task-list [role=checkbox]"
htrcli --cdp attr    "#task-list [role=checkbox]" "aria-checked"   # read back
```

A task's state usually lives in **attributes**, not in text, so `extract list`
must be told to read attributes:

```bash
# "@data-due"      the item's OWN attribute
# "@.link@href"    a DESCENDANT's attribute (selector@attribute)
# "[data-due]"     an attribute value — a descendant match, else the item itself
# ".title"         a descendant's TEXT
htrcli --cdp extract list "#task-list .task" \
  --fields "title:.title,due:@data-due"
```

A pure attribute selector (`[data-due]`) returns the attribute's **value**,
not the matched element's text. A plain selector (`.title`) returns the
descendant's text; a descendant wins over the item itself.

### Booking (B07)

Date pickers, seat maps and payment. Fill the search form, read the available
slots back, pick one, and **stop before any card entry** — a benchmark run
must never enter payment details. Book a clearly-labelled test slot and
cancel it after capturing the confirmation number.

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

### `browser start` says "Chrome (pid N) did not answer on port"

Usually a leftover from a Chrome that died without cleaning up (crash, reboot).
htrcli removes a stale `~/.htrcli/chrome-profile/SingletonLock` automatically
when its owner pid is dead **on this host** — a lock whose `<hostname>` belongs
to another machine (shared/synced profile dir) is left untouched — and kills
the unreachable Chrome it just spawned.
If it still fails, check nothing else holds the profile or the CDP port:

```bash
htrcli browser status
readlink ~/.htrcli/chrome-profile/SingletonLock   # hostname-<pid>; is that pid alive?
lsof -nP -iTCP:9333 -sTCP:LISTEN
```

If the port answers but `browser.json`'s recorded PID is stale, `browser start`
adopts the real listener PID (so `browser stop` can still kill it). If it cannot
identify the listener, it refuses to write a bogus PID and reports the error.

### Self-signed https (`net::ERR_CERT_AUTHORITY_INVALID`)

CDP Chrome uses its own profile with no trusted dev certs. Point htrcli at a
wrapper that adds `--ignore-certificate-errors`:

```bash
printf '#!/bin/sh\nexec "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" --ignore-certificate-errors "$@"\n' > ~/.htrcli/chrome-ignore-certs.sh
chmod +x ~/.htrcli/chrome-ignore-certs.sh
htrcli config set-chrome-path ~/.htrcli/chrome-ignore-certs.sh
htrcli browser stop && htrcli browser start --headless
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
| `htrcli tabs list [--browser chrome\|firefox]` | List connected tabs, optionally one browser |
| `htrcli tabs get <id> [--browser chrome\|firefox]` | Get tab info |
| `htrcli tabs new <url> [--browser chrome\|firefox]` | Open a URL in a new tab; prints its ID and browser |
| `htrcli tabs close <id> [--browser chrome\|firefox]` | Close a tab |
| `htrcli open <url>` | Navigate to URL |
| `htrcli back [steps]` | Browser back — 1 step or N steps (`back 3` loops sequentially, reports partial progress) |
| `htrcli forward [steps]` | Browser forward — 1 step or N steps |
| `htrcli reload` | Reload page |
| `htrcli screenshot [path]` | Take screenshot (viewport, --full-page, --annotate) |
| `htrcli page` | Get page info |
| `htrcli click <sel>` | Click element |
| `htrcli dblclick <sel>` | Double-click element |
| `htrcli fill <sel> <val>` | Clear and fill input |
| `htrcli type <sel> <val>` | Append text to input |
| `htrcli hover <sel>` | Hover element |
| `htrcli press <key>` | Press key (keyDown + keyUp) |
| `htrcli keydown <key>` | Key down only (hold; `keyup` to release, stateless, `--cdp` supported) |
| `htrcli keyup <key>` | Key up only |
| `htrcli mousedown <sel>` | Press mouse button down (`<sel>` or `xy=X,Y` or `@eN`, `--cdp` supported) |
| `htrcli mouseup <sel>` | Release mouse button |
| `htrcli mousemove <sel>` | Move mouse to element/coords (no button) |
| `htrcli drag <src> <dst>` | Drag source → target (`xy=`/`@eN`/selector, `--steps 1..100` `--delay 0..2000ms`, pointer/mouse only, no native DnD) |
| `htrcli select <sel> <val>` | Select dropdown option |
| `htrcli check <sel>` | Check checkbox |
| `htrcli uncheck <sel>` | Uncheck checkbox |
| `htrcli scroll <dir> [px]` | Scroll page |
| `htrcli clear <sel>` | Clear input field |
| `htrcli find <sel>` | Find element info |
| `htrcli findAll <sel>` | Find all elements matching selector (viewport only) |
| `htrcli collect <sel>` | Collect every match in a virtualized list (scrolls) |
| `htrcli extract table\|list <sel>` | Extract structured data (table rows / list items); `--fields "name:sel,due:@attr"`; `--container` when the selector names a wrapper |
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
| `htrcli wait` | Block until a DOM condition holds (--selector/--text/--url) |
| `htrcli retry -- <cmd>` | Run a subcommand with exponential-backoff retries (opt-in, loud) |
| `htrcli bench run <file>` | Run a benchmark scenario file and write a JSON report |
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
| `htrcli recordings start` | Start a session recording (Chrome + Firefox) |
| `htrcli recordings stop` | Stop the current session recording |
| `htrcli recordings status` | Show whether a session recording is live |
| `htrcli recordings list` | List session recordings (newest first, paginated) |
| `htrcli recordings get <id>` | Print a session's steps + annotations as JSON |
| `htrcli recordings export <id> <file>` | Write a session (with screenshots) as JSON, ZIP bundle, or Markdown |
| `htrcli recordings delete <id>` | Delete a stored session recording |
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
