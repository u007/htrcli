# How-To Recorder CLI (htcli) — Implementation Spec

**Package:** `github.com/u007/htcli`
**Language:** Go
**Status:** Draft

---

## Overview

A Go CLI for controlling browser tabs via the How-To Recorder's HTTP/WebSocket remote control API. The CLI is a thin HTTP client; the existing Bun server (`server/index.ts`) handles all browser interaction through the Chrome extension.

```
htcli (Go) ──HTTP──► Server (Bun, port 3845) ──WebSocket──► Extension ──DOM──► Chrome
```

---

## Directory Structure

```
htcli/
├── cmd/
│   └── htcli/
│       └── main.go                    # Entrypoint, sets up cobra root command
├── internal/
│   ├── api/
│   │   ├── client.go                  # HTTP client wrapper (GET/POST helpers)
│   │   ├── types.go                   # All request/response types (mirrors server/types.ts)
│   │   └── errors.go                  # API error types (NotFoundError, AuthError, etc.)
│   ├── commands/
│   │   ├── root.go                    # Root cobra command, global flags (--server, --token, --json)
│   │   ├── tabs.go                    # htr tabs list | tabs get <id>
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

Location: `~/.htcli/config.json`

```json
{
  "server": "http://127.0.0.1:3845",
  "token": ""
}
```

### Config Precedence (highest to lowest)

1. CLI flags: `--server`, `--token`
2. Environment variables: `HTCLI_SERVER`, `HTCLI_TOKEN`
3. Config file: `~/.htcli/config.json`
4. Defaults: `http://127.0.0.1:3845`, empty token

### Commands

```bash
htcli config set-server http://127.0.0.1:3845
htcli config set-token abc123...
htcli config show
```

---

## CLI Commands

### Root Command

```
htcli [flags] <command>
```

Global flags:
- `--server <url>` — Server URL (overrides config)
- `--token <token>` — Bearer token (overrides config)
- `--json` — Output raw JSON instead of formatted text
- `--tab <id>` — Target specific tab (for commands that support it)
- `--timeout <ms>` — Command timeout in milliseconds (default: 30000)

### Health Check

```bash
htcli health
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
htcli tabs list
htcli tabs get <id>
```

**`tabs list` output (human):**
```
ID      Title                          URL                          Active
123     GitHub - u007/htcli           https://github.com/u007...  yes
124     Google                         https://google.com           no
```

**Endpoints:**
- `GET /api/tabs`
- `GET /api/tabs/:id`

---

### Navigation

```bash
htcli open <url>                  # Navigate to URL
htcli back                        # Go back
htcli forward                     # Go forward
htcli reload                      # Reload page
```

**Output (human):**
```
Navigated to https://example.com (142ms)
```

**Endpoints:** `POST /api/command` with actions: `navigate`, `goBack`, `goForward`, `reload`

---

### Interaction

```bash
htcli click <selector>                        # Click element
htcli dblclick <selector>                     # Double-click element
htcli fill <selector> <value>                 # Clear and fill input
htcli type <selector> <value>                 # Type into input (appends)
htcli hover <selector>                        # Hover element
htcli press <key>                             # Press key (Enter, Tab, Ctrl+a, etc.)
htcli select <selector> <value>               # Select dropdown option
htcli check <selector>                        # Check checkbox
htcli uncheck <selector>                      # Uncheck checkbox
htcli scroll <direction> [pixels]             # Scroll: up|down|left|right
htcli clear <selector>                        # Clear input field
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
htcli find <selector>                  # Find element, return info
htcli get text <selector>              # Get text content
htcli get value <selector>             # Get input value
htcli get attr <selector> <attribute>  # Get attribute value
htcli get html <selector>              # Get innerHTML
htcli snapshot                         # Get page accessibility tree
htcli screenshot [path]                # Take screenshot (saves PNG)
htcli page                             # Get page info (URL, title, dimensions)
htcli eval <javascript>                # Execute JavaScript
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
htcli command <json>                   # Send raw JSON command
```

**Example:**
```bash
htcli command '{"action":"click","target":{"selector":"#btn"}}'
htcli command '{"action":"fill","target":{"name":"email"},"value":"test@example.com"}'
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
htcli tabs list --json | jq '.data[].title'
htcli page --json | jq '.data.url'
```

---

## Makefile

```makefile
.PHONY: build run test clean

build:
	go build -o bin/htcli ./cmd/htcli

run:
	go run ./cmd/htcli

test:
	go test ./...

clean:
	rm -rf bin/

install:
	go install ./cmd/htcli
```

---

## `.goreleaser.yml`

```yaml
builds:
  - main: ./cmd/htcli
    binary: htcli
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
    name_template: "htcli_{{ .Version }}_{{ .Os }}_{{ .Arch }}"

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
- `cmd/htcli/main.go`
- `internal/commands/root.go`
- `internal/commands/config.go`
- `internal/api/client.go`
- `internal/api/types.go`
- `internal/api/errors.go`
- `internal/output/format.go`
- `internal/output/color.go`
- `Makefile`

**Working commands:** `htcli config set-server`, `htcli config set-token`, `htcli config show`, `htcli health`

### Phase 2: Tabs + Navigation

**Files:**
- `internal/commands/tabs.go`
- `internal/commands/navigate.go`

**Working commands:** `htcli tabs list`, `htcli tabs get`, `htcli open`, `htcli back`, `htcli forward`, `htcli reload`

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
- Shell completion support (`htcli completion bash|zsh|fish`)

---

## Example Session

```bash
$ htcli config set-server http://127.0.0.1:3845
$ htcli config set-token abc123...

$ htcli health
Server: running
Connected tabs: 2
Uptime: 5m 12s

$ htcli tabs list
ID      Title                     URL                          Active
101     GitHub                    https://github.com           yes
102     Google                    https://google.com           no

$ htcli open https://example.com/login
Navigated to https://example.com/login (142ms)

$ htcli fill "input[name=email]" "user@example.com"
Filled input[name=email] (18ms)

$ htcli fill "input[name=password]" "secret123"
Filled input[name=password] (15ms)

$ htcli click "button[type=submit]"
Clicked button[type=submit] (23ms)

$ htcli screenshot login-success.png
Screenshot saved to login-success.png

$ htcli page
URL:      https://example.com/dashboard
Title:    Dashboard
Domain:   example.com
Viewport: 1280x720
Document: 1280x3200
Scroll:   0, 0
```

---

## Open Questions

1. **Snapshot command:** The current server doesn't have a `snapshot` action in the extension's command executor. Should Phase 4 add accessibility tree support to the extension, or should `htcli snapshot` just call `getPageInfo` + `eval` to build a basic tree?

2. **Multi-tab targeting:** Should `--tab <id>` be a global flag or a per-command flag? Global seems cleaner.

3. **Auth flow:** Should `htcli` auto-read the token from the server's stdout on startup (like agent-browser does)? Or require manual `config set-token`?

4. **Shell completions:** Cobra has built-in support. Include from Phase 1 or defer to Phase 5?

---

*Spec version: 1.0 — 2026-06-10*
