# HTR NControl CLI (htrcli) — Implementation Spec

**Package:** `github.com/u007/htrcli`
**Language:** Go
**Status:** Partially shipped

---

## Overview

A Go CLI for controlling browser tabs via the HTR NControl's HTTP remote control API. The CLI is a thin HTTP client talking to the `htrcli serve` native-messaging daemon on :3845 (the sole backend for remote control). It drives the Chrome or Firefox extension over the daemon's Unix-socket relay transport, and the daemon supports Chrome and Firefox connected simultaneously, routing each command to the browser that owns the target tab.

```
htrcli (Go) ──HTTP──► htrcli serve (:3845) ──Unix socket──► relay ──stdio──► Extension ──DOM──► Chrome / Firefox
```

---

## Directory Structure

```
htrcli/
├── cmd/
│   └── htrcli/
│       └── main.go                    # Entrypoint, sets up cobra root command
├── internal/
│   ├── api/
│   │   ├── client.go                  # HTTP client wrapper (GET/POST helpers)
│   │   ├── types.go                   # All request/response types (mirrors server/types.ts)
│   │   └── errors.go                  # API error types (NotFoundError, AuthError, etc.)
	│   ├── commands/
	│   │   ├── root.go                    # Root cobra command, global flags (--server, --token, --json)
	│   │   ├── tabs.go                    # htr tabs list | tabs get <id>
	│   │   ├── console.go                 # htr console read | watch (event buffer)
	│   │   ├── navigate.go               # htr open <url> | back | forward | reload
	│   │   ├── interact.go               # htr click | fill | type | hover | press | scroll | check | select
	│   │   ├── inspect.go                # htr snapshot | screenshot | page | eval | find | get text|value|attr|html
	│   │   ├── config.go                 # htr config set-server | set-token | show
│   │   └── health.go                 # htr health
│   └── output/
│       ├── format.go                  # JSON / human-readable output switcher
│       ├── color.go                   # Terminal color helpers (respects NO_COLOR)
│       └── table.go                   # Simple ASCII table formatting
├── go.mod
├── go.sum
├── Makefile
├── .goreleaser.yml
└── README.md
```

---

## API Client (`internal/api/client.go`)

### Struct

```go
type Client struct {
    BaseURL    string
    Token      string
    HTTPClient *http.Client
}
```

### Methods

```go
func NewClient(baseURL, token string) *Client
func (c *Client) GetHealth() (*HealthResponse, error)
func (c *Client) ListTabs() ([]TabInfo, error)
func (c *Client) GetTab(id int) (*TabInfo, error)
func (c *Client) ExecuteCommand(tabID *int, cmd Command) (*CommandResult, error)
func (c *Client) GetPageInfo() (*PageInfo, error)
func (c *Client) GetScreenshot() (string, error)  // returns base64 PNG
```

All methods:
- Add `Authorization: Bearer <token>` header if token is set
- Parse `ApiResponse` envelope and return error if `ok: false`
- Return typed errors for 404 (not connected), 403 (auth), timeout

---

## Types (`internal/api/types.go`)

Direct Go mirror of `server/types.ts`:

```go
// Target selectors — multiple strategies tried in priority order
type TargetSelector struct {
    Selector      string `json:"selector,omitempty"`
    XPath         string `json:"xpath,omitempty"`
    ID            string `json:"id,omitempty"`
    Name          string `json:"name,omitempty"`
    Role          string `json:"role,omitempty"`
    Label         string `json:"label,omitempty"`
    Placeholder   string `json:"placeholder,omitempty"`
    Text          string `json:"text,omitempty"`
    TextMatch     string `json:"textMatch,omitempty"`     // exact|contains|regex|startsWith|endsWith
    CaseSensitive *bool  `json:"caseSensitive,omitempty"`
    Tag           string `json:"tag,omitempty"`
    Type          string `json:"type,omitempty"`
    Index         *int   `json:"index,omitempty"`
    All           *bool  `json:"all,omitempty"`
    Visible       *bool  `json:"visible,omitempty"`
    Enabled       *bool  `json:"enabled,omitempty"`
}

type Command struct {
    ID      string          `json:"id"`
    Action  string          `json:"action"`
    Target  *TargetSelector `json:"target,omitempty"`
    Value   string          `json:"value,omitempty"`
    Options map[string]any  `json:"options,omitempty"`
}

type CommandResult struct {
    ID         string   `json:"id"`
    Success    bool     `json:"success"`
    Data       any      `json:"data,omitempty"`
    Error      string   `json:"error,omitempty"`
    Screenshot string   `json:"screenshot,omitempty"`
    Duration   int      `json:"duration,omitempty"`
    PageInfo   *PageInfo `json:"pageInfo,omitempty"`
}

type TabInfo struct {
    ID         int    `json:"id"`
    URL        string `json:"url"`
    Title      string `json:"title"`
    Active     bool   `json:"active"`
    FavIconURL string `json:"favIconUrl,omitempty"`
}

type PageInfo struct {
    URL            string  `json:"url"`
    Title          string  `json:"title"`
    Domain         string  `json:"domain"`
    ScrollX        float64 `json:"scrollX"`
    ScrollY        float64 `json:"scrollY"`
    ViewportWidth  int     `json:"viewportWidth"`
    ViewportHeight int     `json:"viewportHeight"`
    DocumentHeight int     `json:"documentHeight"`
    DocumentWidth  int     `json:"documentWidth"`
}

type ApiResponse[T any] struct {
    OK    bool   `json:"ok"`
    Data  T      `json:"data,omitempty"`
    Error string `json:"error,omitempty"`
}

type HealthResponse struct {
    Status       string  `json:"status"`
    ConnectedTabs int    `json:"connectedTabs"`
    Uptime       float64 `json:"uptime"`
}
```

---

## Errors (`internal/api/errors.go`)

```go
type APIError struct {
    StatusCode int
    Message    string
}

func (e *APIError) Error() string

// Specific error types
type NotFoundError struct{ Message string }    // 404
type AuthError struct{ Message string }        // 403
type TimeoutError struct{ Message string }     // command timeout
```

---

## Config (`internal/commands/config.go`)

### Config File

Location: `~/.htrcli/config.json`

```json
{
  "server": "http://127.0.0.1:3845",
  "token": ""
}
```

### Config Precedence (highest to lowest)

1. CLI flags: `--server`, `--token`
2. Environment variables: `HTRCLI_SERVER`, `HTRCLI_TOKEN`
3. Config file: `~/.htrcli/config.json`
4. Defaults: `http://127.0.0.1:3845`, empty token

### Commands

```bash
htrcli config set-server http://127.0.0.1:3845
htrcli config set-token abc123...
htrcli config show
```

---

## CLI Commands

### Root Command

```
htrcli [flags] <command>
```

Global flags:
- `--server <url>` — Server URL (overrides config)
- `--token <token>` — Bearer token (overrides config)
- `--json` — Output raw JSON instead of formatted text
- `--tab <id>` — Target specific tab (for commands that support it)
- `--timeout <ms>` — Command timeout in milliseconds (default: 30000)

### Health Check

```bash
htrcli health
```

**Output (human):**
```
Server: running
Connected tabs: 3
Uptime: 1h 23m
```

**Endpoint:** `GET /api/health`

---

### Tab Management

```bash
htrcli tabs list
htrcli tabs get <id>
```

**`tabs list` output (human):**
```
ID      Title                          URL                          Active
123     GitHub - u007/htrcli           https://github.com/u007...  yes
124     Google                         https://google.com           no
```

**Endpoints:**
- `GET /api/tabs`
- `GET /api/tabs/:id`

---

### Navigation

```bash
htrcli open <url>                  # Navigate to URL
htrcli back                        # Go back
htrcli forward                     # Go forward
htrcli reload                      # Reload page
```

**Output (human):**
```
Navigated to https://example.com (142ms)
```

**Endpoints:** `POST /api/command` with actions: `navigate`, `goBack`, `goForward`, `reload`

---

### Interaction

```bash
htrcli click <selector>                        # Click element
htrcli dblclick <selector>                     # Double-click element
htrcli fill <selector> <value>                 # Clear and fill input
htrcli type <selector> <value>                 # Type into input (appends)
htrcli hover <selector>                        # Hover element
htrcli press <key>                             # Press key (Enter, Tab, Ctrl+a, etc.)
htrcli select <selector> <value>               # Select dropdown option
htrcli check <selector>                        # Check checkbox
htrcli uncheck <selector>                      # Uncheck checkbox
htrcli scroll <direction> [pixels]             # Scroll: up|down|left|right
htrcli clear <selector>                        # Clear input field
```

**Selector syntax:**
- CSS selector: `#submit-btn`, `.login-form input[type=email]`
- By name: `name=email`
- By role: `role=button`
- By text: `text=Submit`
- By label: `label=Email address`
- By placeholder: `placeholder=Enter your email`

**Output (human):**
```
Clicked #submit-btn (23ms)
```

**Endpoints:** `POST /api/command` with actions: `click`, `dblclick`, `fill`, `type`, `hover`, `pressKey`, `select`, `check`, `uncheck`, `scrollTo`, `clear`

---

### Inspection

```bash
htrcli find <selector>                  # Find element, return info
htrcli get text <selector>              # Get text content
htrcli get value <selector>             # Get input value
htrcli get attr <selector> <attribute>  # Get attribute value
htrcli get html <selector>              # Get innerHTML
htrcli snapshot                         # Get page accessibility tree
htrcli screenshot [path]                # Take screenshot (saves PNG)
htrcli page                             # Get page info (URL, title, dimensions)
htrcli eval <javascript>                # Execute JavaScript
```

**`find` output (human):**
```
Found: button#submit-btn [Submit Form]
  Selector: button#submit-btn
  XPath: /html/body/form/button[1]
  Visible: true
  Enabled: true
  Bounding box: 100x200 300x50
```

**`page` output (human):**
```
URL:      https://example.com/login
Title:    Example - Login
Domain:   example.com
Viewport: 1280x720
Document: 1280x2400
Scroll:   0, 350
```

**`screenshot`:** Saves base64 PNG to file, or prints path to temp file.

**Endpoints:**
- `POST /api/command` with actions: `find`, `getText`, `getValue`, `getAttribute`, `getHTML`, `getPageInfo`, `evaluate`
- `GET /api/screenshot`
- `GET /api/page`

---

### Raw Command

```bash
htrcli command <json>                   # Send raw JSON command
```

**Example:**
```bash
htrcli command '{"action":"click","target":{"selector":"#btn"}}'
htrcli command '{"action":"fill","target":{"name":"email"},"value":"test@example.com"}'
```

**Endpoint:** `POST /api/command`

---

## Output Modes

### Human (default)

Formatted, colored output with timing:

```
Clicked #submit-btn (23ms)
```

### JSON (`--json` flag)

Raw JSON response for piping:

```bash
htrcli tabs list --json | jq '.data[].title'
htrcli page --json | jq '.data.url'
```

---

## Makefile

```makefile
.PHONY: build run test clean

build:
	go build -o bin/htrcli ./cmd/htrcli

run:
	go run ./cmd/htrcli

test:
	go test ./...

clean:
	rm -rf bin/

install:
	go install ./cmd/htrcli
```

---

## `.goreleaser.yml`

```yaml
builds:
  - main: ./cmd/htrcli
    binary: htrcli
    env:
      - CGO_ENABLED=0
    goos:
      - linux
      - darwin
      - windows
    goarch:
      - amd64
      - arm64

archives:
  - format: tar.gz
    name_template: "htrcli_{{ .Version }}_{{ .Os }}_{{ .Arch }}"

checksum:
  name_template: "checksums.txt"

changelog:
  sort: asc
```

---

## Dependencies

```
github.com/spf13/cobra        v1.8+    # CLI framework
github.com/spf13/viper        v1.18+   # Config management
github.com/fatih/color         v1.16+   # Terminal colors
```

Minimal dependency footprint. No web framework, no ORM, no heavy libs.

---

## Implementation Phases

### Phase 1: Skeleton + Config

**Files:**
- `go.mod`
- `cmd/htrcli/main.go`
- `internal/commands/root.go`
- `internal/commands/config.go`
- `internal/api/client.go`
- `internal/api/types.go`
- `internal/api/errors.go`
- `internal/output/format.go`
- `internal/output/color.go`
- `Makefile`

**Working commands:** `htrcli config set-server`, `htrcli config set-token`, `htrcli config show`, `htrcli health`

### Phase 2: Tabs + Navigation

**Files:**
- `internal/commands/tabs.go`
- `internal/commands/navigate.go`

**Working commands:** `htrcli tabs list`, `htrcli tabs get`, `htrcli open`, `htrcli back`, `htrcli forward`, `htrcli reload`

### Phase 3: Interaction

**Files:**
- `internal/commands/interact.go`

**Working commands:** all interaction commands (click, fill, type, hover, press, select, check, scroll, clear)

### Phase 4: Inspection + Output Polish

**Files:**
- `internal/commands/inspect.go`
- `internal/output/table.go`

**Working commands:** all inspection commands (find, get text/value/attr/html, snapshot, screenshot, page, eval)

### Phase 5: Releases + Polish

**Files:**
- `.goreleaser.yml`
- `README.md`
- Shell completion support (`htrcli completion bash|zsh|fish`)

---

## Example Session

```bash
$ htrcli config set-server http://127.0.0.1:3845
$ htrcli config set-token abc123...

$ htrcli health
Server: running
Connected tabs: 2
Uptime: 5m 12s

$ htrcli tabs list
ID      Title                     URL                          Active
101     GitHub                    https://github.com           yes
102     Google                    https://google.com           no

$ htrcli open https://example.com/login
Navigated to https://example.com/login (142ms)

$ htrcli fill "input[name=email]" "user@example.com"
Filled input[name=email] (18ms)

$ htrcli fill "input[name=password]" "secret123"
Filled input[name=password] (15ms)

$ htrcli click "button[type=submit]"
Clicked button[type=submit] (23ms)

$ htrcli screenshot login-success.png
Screenshot saved to login-success.png

$ htrcli page
URL:      https://example.com/dashboard
Title:    Dashboard
Domain:   example.com
Viewport: 1280x720
Document: 1280x3200
Scroll:   0, 0
```

---


## Decisions (Resolved)

1. **Snapshot command:** Add accessibility tree support to the extension's command executor in Phase 4. The extension will use `axe-core` or CDP's `Accessibility.getFullAXTree` to generate a full accessibility tree with `@eN` refs.

2. **Multi-tab targeting:** `--tab <id>` is a **global flag** that applies to all commands.

3. **Auth flow:** `htrcli` will **auto-read the token** from the server if no token is configured. On first connection, it hits `/api/health` without auth; if the server returns a 403 with an auto-generated token hint, the CLI prompts the user or reads from env/config.

4. **Shell completions:** Defer to Phase 5.

---

## Snapshot, Screenshot & Annotate — Detailed Implementation

These three features require both **extension changes** (new actions in command executor) and **CLI flags** (formatting options).

### Feature 1: Snapshot (Accessibility Tree)

**What it is:** A TEXT-based representation of the page's accessibility tree with `@eN` refs for interactive elements. NOT a visual screenshot.

**Output example:**
```
@e1 heading "Login Form" [level=1]
@e2 textbox "Email" [required]
  @e3 placeholder="Enter your email"
@e4 textbox "Password" [required]
  @e5 placeholder="Enter your password"
@e6 button "Submit" [enabled]
```

**CLI usage:**
```bash
htrcli snapshot                              # Full page accessibility tree
htrcli snapshot --interactive                # Only interactive elements
htrcli snapshot --compact                    # Compact output
htrcli snapshot --depth 3                    # Limit tree depth
htrcli snapshot --selector "#login-form"     # Scope to element
htrcli snapshot --urls                       # Show URLs in links
htrcli snapshot --json                       # Raw JSON output
```

**Extension implementation (`snapshot` action in command executor):**

The extension needs a new `snapshot` action that:

1. Walks the DOM tree and builds an accessibility tree
2. Assigns `@eN` refs to interactive elements (buttons, links, inputs, selects, checkboxes, etc.)
3. Returns the tree as structured JSON

**Options for building the tree:**

| Approach | Pros | Cons |
|----------|------|------|
| **axe-core library** | Battle-tested, handles edge cases | ~300KB added to extension |
| **Custom DOM walker** | Lightweight, full control | Must handle all ARIA roles manually |
| **CDP Accessibility.getFullAXTree** | Most accurate, Chrome-native | Requires background script, more complex |

**Recommended:** Use **axe-core** for Phase 4. It's the industry standard for accessibility tree generation and handles all the edge cases (ARIA roles, states, relationships).

**Extension changes needed:**
1. Add `axe-core` as a dependency
2. Add `snapshot` action to `src/contentScript/commandExecutor.ts`
3. Return structured tree with refs, roles, names, states

**Server types to add:**
```typescript
// In src/types/commands.ts
interface SnapshotNode {
  ref: string;          // @e1, @e2, etc.
  role: string;         // heading, button, textbox, etc.
  name: string;         // Accessible name
  level?: number;       // Heading level
  checked?: string;     // Checkbox state
  disabled?: boolean;
  required?: boolean;
  value?: string;       // Current value
  children: SnapshotNode[];
}

interface SnapshotResult {
  tree: SnapshotNode[];
  refCount: number;     // Total number of refs assigned
}
```

---

### Feature 2: Screenshot (Viewport + Full Page)

**What it is:** A VISUAL screenshot (PNG/JPEG image) of the page.

**CLI usage:**
```bash
htrcli screenshot                              # Viewport screenshot (default)
htrcli screenshot --full                       # Full page screenshot
htrcli screenshot --format jpeg --quality 80   # JPEG format
htrcli screenshot --selector "#login-form"     # Capture specific element
htrcli screenshot output.png                   # Save to specific path
htrcli screenshot --json                       # Return base64 only
```

**Extension implementation:**

| Mode | How it works | Extension API |
|------|--------------|---------------|
| **Viewport** | Capture visible area only | `chrome.tabs.captureVisibleTab()` (already exists) |
| **Full page** | Scroll + capture + stitch | Need to add: scroll logic + multiple captures |
| **Element** | Capture specific element bounds | Need to add: element rect + clip capture |

**Viewport screenshot (already supported):**
```typescript
// Already in background/index.ts
chrome.tabs.captureVisibleTab(tab.windowId, { format: 'png' })
```

**Full page screenshot (new):**
```typescript
// New action in command executor
case "screenshot":
  const options = command.options || {};
  if (options.full) {
    return await captureFullPageScreenshot();
  } else if (options.selector) {
    return await captureElementScreenshot(options.selector);
  } else {
    return await captureViewportScreenshot();
  }
```

**Full page capture algorithm:**
1. Get document dimensions: `document.documentElement.scrollHeight/Width`
2. Get viewport dimensions: `window.innerHeight/Width`
3. Calculate number of captures needed
4. Scroll to each position, capture viewport
5. Stitch images together (can be done in background script or CLI)

**Extension changes needed:**
1. Add `screenshot` action to `src/contentScript/commandExecutor.ts`
2. For full page: implement scroll + capture loop in background script
3. For element: implement element rect capture

---

### Feature 3: Annotated Screenshot

**What it is:** A visual screenshot with numbered element overlays (like agent-browser's `--annotate`).

**Output:** PNG image with colored numbered circles overlaid on interactive elements.

**CLI usage:**
```bash
htrcli screenshot --annotate                    # Annotated screenshot
htrcli screenshot --annotate --full             # Full page + annotated
htrcli screenshot --annotate --selector "#form" # Annotate specific element
htrcli screenshot --annotate output.png         # Save annotated screenshot
```

**Extension implementation:**

The annotated screenshot works by:

1. **Collecting annotations** — Find all interactive elements, get their bounding boxes
2. **Injecting overlay** — Create positioned HTML/CSS elements with numbered labels
3. **Capturing screenshot** — Take screenshot with overlay visible
4. **Removing overlay** — Clean up injected elements
5. **Returning metadata** — Return both image and annotation positions

**Step-by-step:**

```typescript
// 1. Collect annotations
const annotations = collectAnnotations(); // Find all interactive elements
// Returns: [{ ref: "@e1", number: 1, role: "button", name: "Submit", box: {x, y, w, h} }]

// 2. Inject overlay
injectAnnotationOverlay(annotations); // Create positioned div elements
// Creates: <div class="ht-annotation" style="top:100px;left:200px">1</div>

// 3. Capture screenshot
const screenshot = await captureVisibleTab(); // Or full page

// 4. Remove overlay
removeAnnotationOverlay(); // Clean up

// 5. Return result
return { screenshot, annotations };
```

**Overlay CSS:**
```css
.ht-annotation {
  position: absolute;
  width: 24px;
  height: 24px;
  background: #ff4444;
  color: white;
  border-radius: 50%;
  font-size: 12px;
  font-weight: bold;
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 2147483647;
  pointer-events: none;
  box-shadow: 0 2px 4px rgba(0,0,0,0.3);
}
```

**Extension changes needed:**
1. Add `collectAnnotations()` function in content script
2. Add `injectAnnotationOverlay()` / `removeAnnotationOverlay()` functions
3. Add `screenshot` action with `annotate` option to command executor
4. Return both image and annotation metadata

---

### Go CLI Implementation

**Types to add (`internal/api/types.go`):**
```go
type SnapshotOptions struct {
    Interactive bool   `json:"interactive,omitempty"`
    Compact     bool   `json:"compact,omitempty"`
    Depth       *int   `json:"maxDepth,omitempty"`
    Selector    string `json:"selector,omitempty"`
    URLs        bool   `json:"urls,omitempty"`
}

type ScreenshotOptions struct {
    Full     bool   `json:"full,omitempty"`
    Format   string `json:"format,omitempty"`    // png, jpeg
    Quality  *int   `json:"quality,omitempty"`
    Selector string `json:"selector,omitempty"`
    Annotate bool   `json:"annotate,omitempty"`
}

type SnapshotNode struct {
    Ref      string         `json:"ref"`
    Role     string         `json:"role"`
    Name     string         `json:"name"`
    Level    *int           `json:"level,omitempty"`
    Checked  string         `json:"checked,omitempty"`
    Disabled bool           `json:"disabled,omitempty"`
    Required bool           `json:"required,omitempty"`
    Value    string         `json:"value,omitempty"`
    Children []SnapshotNode `json:"children,omitempty"`
}

type Annotation struct {
    Ref    string    `json:"ref"`
    Number int       `json:"number"`
    Role   string    `json:"role"`
    Name   string    `json:"name,omitempty"`
    Box    BoundingBox `json:"box"`
}

type BoundingBox struct {
    X      float64 `json:"x"`
    Y      float64 `json:"y"`
    Width  float64 `json:"width"`
    Height float64 `json:"height"`
}

type ScreenshotResult struct {
    Path        string       `json:"path,omitempty"`
    Base64      string       `json:"base64"`
    Annotations []Annotation `json:"annotations,omitempty"`
}
```

**CLI commands (`internal/commands/inspect.go`):**
```go
// snapshot command
var snapshotCmd = &cobra.Command{
    Use:   "snapshot",
    Short: "Get page accessibility tree with @eN refs",
    RunE: func(cmd *cobra.Command, args []string) error {
        client := getAPIClient()
        // Build command with options
        // Call POST /api/command with action: "snapshot"
        // Format output as text tree or JSON
    },
}

// screenshot command
var screenshotCmd = &cobra.Command{
    Use:   "screenshot [path]",
    Short: "Take screenshot (viewport, full page, or annotated)",
    Args:  cobra.MaximumNArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        client := getAPIClient()
        // If no flags, use GET /api/screenshot
        // If --full or --annotate, use POST /api/command with options
        // Save to file or print path
    },
}
```

---

### Implementation Timeline

| Phase | Feature | Files to Change |
|-------|---------|-----------------|
| Phase 4a | Snapshot | Extension: add axe-core, new action. CLI: snapshot command |
| Phase 4b | Screenshot (viewport) | Already works. CLI: screenshot command |
| Phase 4c | Screenshot (full page) | Extension: scroll + capture. CLI: --full flag |
| Phase 4d | Annotated screenshot | Extension: overlay inject/remove. CLI: --annotate flag |

---

## Tray icon

`htrcli serve` auto-attaches a cross-platform system-tray icon on
desktops. The menu exposes live status and a small set of maintenance
actions (reinstall native host, open config folder, copy bearer token,
show recent log, restart, quit). The tray is silently skipped on
headless Linux; opt out with `--no-tray` or `HTRCLI_NO_TRAY=1`.

- New package: `internal/tray` (`Controller` interface, `Run`, `ShouldStart`,
  `daemonController`). The `getlantern/systray` dependency is isolated here.
- `tray.ShouldStart(noTray bool) bool` — pure detection: macOS/Windows
  always; Linux only with a display and no active SSH session.
- `serve.go` restructures so the main goroutine drives `tray.Run`; the HTTP
  server, signal handler, and daemon sweeper run as background goroutines.
  Shutdown is explicit (`performShutdown`), not deferred.
- `host.StartUnixSocketServer` now returns its `net.Listener` so shutdown can
  close it.
- `host.Daemon` gains `RelaysConnected()` and `LastError()` (5-minute
  staleness) for the status line.
- `htrcli config set-extension-id <id> [--browser chrome|firefox]` stores the
  ID used by the tray's "Reinstall native host" menu.
- Bearer token is logged as a fingerprint (`a1b2…f3e4`) instead of the full
  value.

See `docs/tray.md` for the user-facing reference and
`docs/superpowers/specs/2026-07-18-htrcli-desktop-tray-design.md` for the
full design rationale.

---

## Feature Parity: Network, Console, Refs, Upload, Dialogs, Contexts & Video

**Status:** Partially shipped — console capture / durable event buffer landed in code; the remaining network/dialog/ref/upload/video phases below are still draft.

**Goal:** bring htrcli to parity with Playwright / the claude-in-chrome MCP
extension. htrcli now ships console capture, but still lacks network
inspection, persistent element refs, file upload, dialog handling, browser-
context support, and video/trace support. Every
Chrome-only mechanism below (anything needing `chrome.debugger`/CDP) also
gets a designed Firefox fallback rather than an explicit "unsupported"
error — Firefox lacks `chrome.debugger`, so its fallbacks use
`browser.webRequest`, injected page-world scripts, or separate-process
tricks instead of CDP.

### Foundational mechanism: event buffer + poll

Network capture, console capture, and dialog capture are all "arm before it
happens, read later" event streams, unlike the existing request/response
actions (`click`, `fill`, `eval`). This is built **once** and reused by all
three features below it, rather than three bespoke implementations.

**Extension side (`src/background/index.ts`):** a per-tab ring buffer:

```typescript
interface EventEntry {
  seq: number;           // monotonic, per tab
  kind: "console" | "network" | "dialog";
  timestamp: number;
  data: ConsoleEntry | NetworkEntry | DialogEntry;
}

interface ConsoleEntry {
  level: "log" | "warn" | "error" | "info" | "debug";
  args: string[];        // JSON-stringified args
  source?: string;       // file:line if available
}

interface NetworkEntry {
  requestId: string;
  url: string;
  method: string;
  status?: number;
  requestHeaders?: Record<string, string>;
  responseHeaders?: Record<string, string>;
  bodyTruncated?: boolean;   // true if body exceeded size cap
  body?: string;             // omitted/truncated for large payloads
  durationMs?: number;
}

interface DialogEntry {
  dialogType: "alert" | "confirm" | "prompt" | "beforeunload";
  message: string;
  resolvedAction: "accept" | "dismiss";
  respondedText?: string;
}
```

Producers write into this buffer:
- MAIN-world injected script wrapping `console.*` (Chrome + Firefox, no CDP).
- Chrome: `chrome.debugger` `Network.*` / `Page.javascriptDialogOpening` listeners.
- Firefox: `browser.webRequest.onBeforeRequest/onCompleted`, injected `window.alert/confirm/prompt` override.

Large payloads (response bodies, later video frames) reuse the existing
screenshot pattern — POST directly to a new HTTP endpoint on the daemon
instead of going through the native-messaging relay's 1MB frame cap.

**Daemon side (`internal/host/server.go`):** new endpoint:

```
POST /api/events/ingest          # extension → daemon, body: EventEntry[]
GET  /api/events?since=<seq>&kind=<kind>&tab=<id>   # CLI → daemon, cursor-based
```

The CLI always polls with `since=<seq>` (never "return everything") to avoid
duplicate/missed entries across polls.

**CLI side (`internal/commands/console.go`):** shared helper used by
`console`, `network`, and `dialog` subcommands:

```go
type EventPoller struct {
    Client   *api.Client
    TabID    *int
    Kind     string
    Since    int
}

func (p *EventPoller) Read() ([]api.EventEntry, error)              // one snapshot
func (p *EventPoller) Watch(ctx context.Context, timeout time.Duration, match func(api.EventEntry) bool) (*api.EventEntry, error)  // blocking long-poll
```

`watch` underlies `network wait`, and could later back a `console wait
--level error` style command; `read` underlies the plain `read` subcommands.

**CDP-transport equivalent:** `internal/cdp/session.go`'s existing
`Session.Call`/`WaitEvent` dispatcher already buffers raw CDP events per
session. Add:

```go
func (s *Session) Events(domain string, since int) []Event
```

so the same cursor-based `EventPoller` works transparently whether htrcli
is talking to the extension or directly to CDP.

---

### 1. Network inspection

Two distinct sub-designs — passive capture is a read; mocking is a
standing rule, not a live round trip (a paused request can't wait on a CLI
process fast enough, and CDP pause timeouts are short).

**1a. Passive capture + `waitForResponse`**

```bash
htrcli network read [--since N] [--json]
htrcl network watch [--timeout 10000] [--json]      # blocking tail, Ctrl-C to stop
htrcli network wait --url <glob> [--status 200] [--timeout 10000]
```

- Chrome: `Network.requestWillBeSent` / `responseReceived` / `loadingFinished`
  via `chrome.debugger`, wired into new `internal/cdp/network.go` using the
  existing `Session.Call`/`WaitEvent`.
- Firefox: `browser.webRequest.onBeforeRequest`/`onCompleted` in
  `src/background/index.ts`, same `NetworkEntry` shape written to the buffer.
- `network wait` is a CLI-side filtered `EventPoller.Watch` call — no new
  server primitive.

**1b. Interception / mocking** — declarative rules registered ahead of time:

```bash
htrcli network mock --url-pattern <glob> --status 200 --body-file resp.json [--method GET]
htrcli network block --url-pattern <glob>
htrcli network unmock [--all | --url-pattern <glob>]
```

New `CommandAction: "mockNetworkRule"` pushes a rule list to the extension.
- Chrome: `Fetch.enable` + `requestPaused` handler in background, matches
  registered rules, fulfills/fails/continues locally — no CLI round trip
  per request.
- Firefox: `webRequest.onBeforeRequest` blocking listener. Requires adding
  `webRequestBlocking` to `src/manifest.ts` permissions (currently only has
  `host_permissions: <all_urls>`, no `webRequest`/`webRequestBlocking`).

New: `internal/cdp/network.go`, `src/background/networkMock.ts`.
Modified: `src/manifest.ts`, `src/background/index.ts`, `internal/commands/network.go`.

---

### 2. Console log capture

Build this first — it needs no CDP at all, so it's the cheapest end-to-end
proof of the event-buffer mechanism, and doubles as the Firefox reference
implementation.

```bash
htrcli console read [--since N] [--level error,warn] [--json]
htrcli console watch [--timeout 10000]
```

New `src/contentScript/consoleCapture.ts`, injected as a MAIN-world script,
wraps `console.log/warn/error/info/debug`, posts each call via
`chrome.runtime.sendMessage` to background, which appends a `ConsoleEntry`
to the ring buffer. Identical on Chrome and Firefox.

New: `src/contentScript/consoleCapture.ts`, `internal/commands/console.go`.
Modified: `src/contentScript/index.ts` (inject), `src/background/index.ts`
(buffer + relay), `internal/host/server.go` (`/api/events*`),
`internal/api/client.go` (poll client).

---

### 3. Full-page + annotated screenshots

Current state: the extension only does `chrome.tabs.captureVisibleTab`
(viewport) plus an internal `captureScreenshotWithHighlight` used during
recording — no full-page stitching or CLI-exposed annotate mode exists
today, despite the docs noting them as "side-panel only." This section
supersedes that note.

```bash
htrcli screenshot --full-page [path]
htrcli screenshot --annotate "button,input,a" [path]
htrcli screenshot --full-page --annotate "role=button" [path]
```

- **Full-page:** Chrome via CDP `Page.captureScreenshot({captureBeyondViewport: true})`
  + `Page.getLayoutMetrics` for content size, in new `internal/cdp/screenshot.go`.
  Extension-transport/Firefox fallback: scroll-and-stitch — resize/scroll
  loop, multiple `captureVisibleTab` calls, composited via `OffscreenCanvas`
  in the background script.
- **Annotate:** generalize the existing `captureScreenshotWithHighlight` /
  `src/contentScript/highlighter.ts` pattern to accept an arbitrary
  selector/box list from the CLI (reusing `TargetSelector` matching) instead
  of only recording-step highlights.

```go
type ScreenshotOptions struct {
    Full     bool     `json:"full,omitempty"`
    FullPage bool     `json:"fullPage,omitempty"`
    Annotate []string `json:"annotate,omitempty"`   // selectors to number/box
    Format   string   `json:"format,omitempty"`
    Quality  *int     `json:"quality,omitempty"`
}
```

Reuses the existing screenshot POST-back path (extension → daemon HTTP,
bypassing the native-messaging relay) — no protocol change needed beyond
the options above.

New: `internal/cdp/screenshot.go`. Modified: `internal/commands/inspect.go`,
`src/background/index.ts`, `src/contentScript/highlighter.ts`.

---

### 4. Persistent element refs (`@e1`) + `findAll` subcommand

**`findAll` — ship immediately, no new plumbing needed.** The `findAll`
`CommandAction`/`handleFindAll` already exist server- and extension-side;
only the CLI subcommand is missing.

```bash
htrcli findAll <selector> [--json]
```

Mirrors `find`'s flags and output shape, in `internal/commands/inspect.go`.

**Persistent refs** — explicit storage decision per transport, since
neither transport has one today:

- **Extension-transport:** an in-page registry (new content-script module)
  holding a `Map<string, Element>`, assigning `@e1`, `@e2`, ... when
  `find`/`findAll` is called with `--ref`. Later commands resolve `@e1`
  back to the element within the same document. **Navigation invalidates
  the registry** — a stale ref must return an explicit "stale ref: page
  navigated" error, never silently re-resolve by guessing.
- **CDP path:** use CDP's own `backendNodeId` (via `DOM.enable` +
  `DOM.describeNode`) instead of a bespoke JS registry.  `RemoteObjectId`
  expires on GC; `backendNodeId` is durable for a document's lifetime. This
  is the first place the Go CDP side needs real `DOM.*` calls rather than
  only the injected JS bundle (`internal/cdp/bundle/htrcli-dom.js`).

```bash
htrcli find <selector> --ref          # returns e.g. "@e7" instead of resolving inline
htrcli click "@e7"                    # interact commands accept refs transparently
```

`internal/commands/interact.go`'s selector parsing gains an `@e`-prefix
branch that skips normal `TargetSelector` resolution and sends the ref id
instead.

New: `internal/cdp/elementref.go`, extension-side ref registry module.
Modified: `internal/commands/interact.go`, `internal/commands/inspect.go`,
`src/contentScript/elementFinder.ts`.

---

### 5. File upload

```bash
htrcli upload <selector> <file-path>[,<file-path>...]
```

- **Chrome:** CDP `DOM.setFileInputFiles` (first real `DOM.*` usage on the
  Go side) given a `backendNodeId` (reuses feature 4's ref plumbing, or a
  fresh selector resolve) and a list of local file paths — no OS
  file-picker dialog appears.
- **Firefox:** genuine limitation, not equivalent parity. Firefox has no
  remote-debugging primitive to fabricate a real `File` object with
  content. Document this explicitly as unsupported/degraded on Firefox
  (OS-level file-picker automation is out of extension scope) — do not
  claim false parity.

New: `internal/cdp/upload.go`. Modified: `internal/commands/interact.go`.

---

### 6. Dialog handling

The handler must be armed **before** the action that triggers the dialog.
CLI sets a standing policy for the session, applied to whatever fires next,
plus a readable log of what already happened.

```bash
htrcli dialog handle --action accept|dismiss|respond --text "..."
htrcli dialog list [--since N]
```

- **Chrome:** `Page.javascriptDialogOpening` event → immediately
  `Page.handleJavaScriptDialog` per the current policy, logged as a
  `DialogEntry` in the event buffer.
- **Firefox fallback:** MAIN-world override of `window.alert/confirm/prompt`
  injected at `document_start`, before page scripts run. Explicit
  limitations to document: racy against dialogs a page fires before the
  override lands, and cannot intercept true native dialogs like
  `beforeunload`.

New: `internal/cdp/dialog.go`, `src/contentScript/dialogOverride.ts`.
Modified: `src/background/index.ts`, `internal/commands/console.go`.

---

### 7. Browser contexts + video/trace recording

Highest risk section — build last, after 1–4 exist since trace export
depends on their data.

**7a. Contexts**

```bash
htrcli --context work open https://example.com    # new global flag, alongside --tab
htrcli context list
```

Extend `internal/cdp/launch.go` (already launches Chrome) to accept
`--context <name>`, backed by either a separate `--user-data-dir` profile
(true isolation) or CDP `Target.createBrowserContext` (lighter, in-process,
Chrome-only). Firefox fallback: a separate Firefox process via
`-profile <dir>` — no CDP-context equivalent exists on Firefox. New global
`--context` flag threaded through `internal/commands/root.go`.

**7b. Video recording**

No native "record to file" primitive exists anywhere in the stack today.
Playwright itself builds this from `Page.startScreencast` streaming JPEG
frames as CDP events, encoded client-side — htrcli follows the same
approach:

```bash
htrcli record start [--context <name>]
htrcli record stop <output.mp4>
```

Frames flow through the event-buffer POST-back path (same large-binary
pattern as screenshots) into a temp frame directory; an **external ffmpeg
dependency** stitches frames into video. This is the single riskiest new
piece — new external binary dependency, frame-timing/drop concerns.
**Firefox has no stable remote-screencast equivalent — document as
infeasible on Firefox, not a fallback.** `htrcli health`/`record start`
must fail with a clear "ffmpeg not found" error rather than hang if the
binary is missing.

**Trace export** is an aggregation, not new capture — it bundles network
(§1) + console (§2) + screenshots (§3) + timestamped actions into one file.
Must be built after §1–§3 exist.

```bash
htrcli trace export <path.zip>
```

Mirrors the existing `src/utils/exportZip.ts`/`exportJson.ts` export
patterns already in the codebase.

New: `internal/cdp/context.go`, `internal/cdp/screencast.go`,
`internal/commands/context.go`, `internal/commands/trace.go`. Modified:
`internal/cdp/launch.go`, `internal/commands/root.go`,
`src/utils/exportZip.ts` (trace variant).

---

### Build order

1. **Phase 0** (parallel, ship immediately): `findAll` subcommand (§4);
   viewport-annotate wiring (§3, defer full-page stitching).
2. **Phase 1** (foundational — blocks 3 downstream features): event buffer +
   poll infra, validated end-to-end via console capture (§2), which needs
   no CDP at all.
3. **Phase 2:** passive network capture + `waitForResponse` (§1a), dialog
   handling (§6) — both build on the Phase 1 buffer.
4. **Phase 3:** declarative network mock/intercept (§1b), persistent
   element refs via `backendNodeId`/in-page registry (§4), file upload (§5,
   reuses refs from this phase).
5. **Phase 4:** full-page screenshot stitching (§3) — independent, can slot
   in any time after Phase 0.
6. **Phase 5** (highest risk, last): browser contexts (§7a), then
   screencast → ffmpeg video → trace export (§7b), layered on data from
   Phases 1–4.

### Verification

- Each new CLI subcommand gets a Go test in `internal/commands/*_test.go`
  following the existing pattern (`cdp_exec_test.go`, `root_test.go`) —
  mock the daemon/CDP session, assert flag parsing and output shape.
- Manual end-to-end pass per phase: run `htrcli serve`, load the extension
  in both Chrome and Firefox, drive the new subcommands against a local
  test page with a `console.log`, a `fetch` call, a `confirm()`, and a file
  input. Confirm Chrome behavior matches Playwright's equivalent API, and
  confirm Firefox fallbacks either work as designed or fail with the
  explicitly documented limitation — never a silent no-op.
- For §7b, verify the ffmpeg prerequisite is documented in the README/docs
  and that its absence produces an explicit error, not a hang.

*Spec version: 1.3 — 2026-07-23*
