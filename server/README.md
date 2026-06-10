# How-To Recorder Server

HTTP + WebSocket API server for remote controlling browser tabs via the How-To Recorder Chrome extension.

## Architecture

```
┌─────────────────┐     HTTP      ┌─────────────────┐     WebSocket     ┌─────────────────┐
│   External Tool  │ ◄───────────► │   API Server     │ ◄───────────────► │   Extension     │
│   (AI Agent)     │               │   (Bun/Node)     │                   │   (Content)     │
└─────────────────┘               └─────────────────┘                   └─────────────────┘
```

1. **External Tool** (e.g., AI agent) sends HTTP requests to the server
2. **Server** forwards commands to the extension via WebSocket
3. **Extension** executes commands on the page and returns results
4. **Server** returns results to the external tool

## Quick Start

### 1. Install Dependencies

```bash
cd server
npm install  # or bun install
```

### 2. Start the Server

```bash
npm run dev     # Development with hot reload
npm run start   # Production
```

### 3. Connect the Extension

The Chrome extension will automatically connect to the server when remote control is enabled. You can enable it via:

- **Content script message**: Send `ENABLE_REMOTE_CONTROL` message
- **URL parameter**: Add `?htr-server=ws://127.0.0.1:3845` to page URL
- **Storage**: Set `remoteControlServer` in `chrome.storage.local`

### 4. Send Commands

```bash
# List connected tabs
curl http://127.0.0.1:3845/api/tabs

# Click an element
curl -X POST http://127.0.0.1:3845/api/command \
  -H "Content-Type: application/json" \
  -d '{"command":{"id":"1","action":"click","target":{"selector":"#submit-button"}}}'

# Fill a form field
curl -X POST http://127.0.0.1:3845/api/command \
  -H "Content-Type: application/json" \
  -d '{"command":{"id":"2","action":"fill","target":{"name":"email"},"value":"user@example.com"}}'

# Get page info
curl http://127.0.0.1:3845/api/page
```

## API Endpoints

### Health Check

```
GET /api/health
```

Response:
```json
{
  "ok": true,
  "data": {
    "status": "running",
    "connectedTabs": 1,
    "uptime": 123.456
  }
}
```

### List Tabs

```
GET /api/tabs
```

Response:
```json
{
  "ok": true,
  "data": [
    {
      "id": 123456,
      "url": "https://example.com",
      "title": "Example Page",
      "active": true
    }
  ]
}
```

### Get Tab Info

```
GET /api/tabs/:id
```

### Execute Command

```
POST /api/tabs/:id/command
POST /api/command  (uses active tab)
```

Request body:
```json
{
  "command": {
    "id": "unique-command-id",
    "action": "click",
    "target": {
      "selector": "#submit-button"
    },
    "value": "optional-value",
    "options": {}
  },
  "screenshot": false
}
```

### Get Page Info

```
GET /api/page
```

### Take Screenshot

```
GET /api/screenshot
```

## Command Actions

### Finding / Inspection

| Action | Description | Target Required | Value/Options |
|--------|-------------|-----------------|---------------|
| `find` | Find element, return info | Yes | - |
| `findAll` | Find all matching elements | Yes | - |
| `wait` | Wait for element to appear | Yes | `timeout` (ms) |
| `isVisible` | Check if element is visible | Yes | - |
| `isEnabled` | Check if element is enabled | Yes | - |
| `getValue` | Get input/select value | Yes | - |
| `getAttribute` | Get attribute value | Yes | `attribute` |
| `getText` | Get text content | Yes | - |
| `getHTML` | Get inner/outer HTML | Yes | `outer` (bool) |
| `getBoundingBox` | Get element dimensions | Yes | - |
| `getComputedStyle` | Get CSS property value | Yes | `property` |
| `getPageInfo` | Get page URL, title, etc. | No | - |
| `xpath` | Generate XPath for element | Yes | - |

### Interaction

| Action | Description | Target Required | Value/Options |
|--------|-------------|-----------------|---------------|
| `click` | Click element | Yes | `button`, `count` |
| `dblclick` | Double-click element | Yes | - |
| `rightclick` | Right-click element | Yes | - |
| `hover` | Hover over element | Yes | - |
| `focus` | Focus element | Yes | - |
| `blur` | Blur element | Yes | - |
| `scrollTo` | Scroll element into view | Yes | - |
| `fill` | Fill input/select/textarea | Yes | value |
| `type` | Type text character by character | Yes | value |
| `clear` | Clear input value | Yes | - |
| `select` | Select dropdown option | Yes | value |
| `check` | Check checkbox/radio | Yes | - |
| `uncheck` | Uncheck checkbox/radio | Yes | - |
| `pressKey` | Press keyboard key | Yes | key (e.g., "Enter", "Control+A") |
| `selectText` | Select all text in element | Yes | - |

### Navigation

| Action | Description | Target Required | Value/Options |
|--------|-------------|-----------------|---------------|
| `navigate` | Navigate to URL | No | URL |
| `reload` | Reload page | No | - |
| `goBack` | Go back in history | No | - |
| `goForward` | Go forward in history | No | - |

### Screenshot

| Action | Description | Target Required | Value/Options |
|--------|-------------|-----------------|---------------|
| `screenshot` | Capture screenshot | No | - |

### Script Execution

| Action | Description | Target Required | Value/Options |
|--------|-------------|-----------------|---------------|
| `evaluate` | Execute JavaScript | No | script |

### Highlight

| Action | Description | Target Required | Value/Options |
|--------|-------------|-----------------|---------------|
| `highlight` | Highlight element | Yes | - |
| `unhighlight` | Remove highlight | No | - |

## Target Selectors

Find elements using various strategies:

```json
{
  "selector": "#submit-button"           // CSS selector
  "xpath": "//button[@type='submit']"    // XPath
  "id": "submit-button"                  // ID attribute
  "name": "email"                        // Name attribute
  "role": "button"                       // ARIA role
  "label": "Email Address"               // Associated label
  "placeholder": "Enter your email"      // Placeholder
  "text": "Submit"                       // Text content
  "tag": "button"                        // Tag name
  "type": "email"                        // Input type
  "textMatch": "contains"                // exact|contains|regex|startsWith|endsWith
  "caseSensitive": false                 // Case-sensitive matching
  "visible": true                        // Only visible elements
  "enabled": true                        // Only enabled elements
  "index": 0                             // Which match to use
  "all": false                           // Return all matches
  "waitForAppear": true                  // Wait for element
  "timeout": 5000                        // Wait timeout (ms)
}
```

## Authentication

### IP Whitelist (Default)

Only allows connections from whitelisted IPs (default: 127.0.0.1, localhost, ::1).

```bash
# Add custom IPs
HTR_ALLOWED_IPS="127.0.0.1,192.168.1.100" npm run start

# Disable IP whitelist
HTR_ENABLE_IP_WHITELIST=false npm run start
```

### Bearer Token (Enabled by Default)

Require a bearer token for all requests.

```bash
# Enable bearer token
HTR_BEARER_TOKEN="your-secret-token" npm run start

# Use token in requests
curl -H "Authorization: Bearer your-secret-token" http://127.0.0.1:3845/api/tabs
```

### Both

Both can be enabled simultaneously (both must pass).

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `HTR_PORT` | `3845` | HTTP server port |
| `HTR_HOST` | `127.0.0.1` | HTTP server host |
| `HTR_ENABLE_IP_WHITELIST` | `true` | Enable IP whitelist |
| `HTR_ALLOWED_IPS` | `127.0.0.1,localhost,::1` | Comma-separated allowed IPs |
| `HTR_ENABLE_BEARER_TOKEN` | `true` | Enable bearer token |
| `HTR_BEARER_TOKEN` | (auto-generated) | Required bearer token |

## Examples

### Python (requests)

```python
import requests

SERVER = "http://127.0.0.1:3845"

# List tabs
tabs = requests.get(f"{SERVER}/api/tabs").json()

# Click element
result = requests.post(f"{SERVER}/api/command", json={
    "command": {
        "id": "1",
        "action": "click",
        "target": {"selector": "#submit-button"}
    }
}).json()

# Fill form
result = requests.post(f"{SERVER}/api/command", json={
    "command": {
        "id": "2",
        "action": "fill",
        "target": {"name": "email"},
        "value": "user@example.com"
    }
}).json()
```

### JavaScript (fetch)

```javascript
const SERVER = "http://127.0.0.1:3845";

// List tabs
const tabs = await fetch(`${SERVER}/api/tabs`).then(r => r.json());

// Click element
const result = await fetch(`${SERVER}/api/command`, {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({
    command: {
      id: "1",
      action: "click",
      target: { selector: "#submit-button" }
    }
  })
}).then(r => r.json());
```

### curl

```bash
# List tabs
curl http://127.0.0.1:3845/api/tabs

# Click element
curl -X POST http://127.0.0.1:3845/api/command \
  -H "Content-Type: application/json" \
  -d '{"command":{"id":"1","action":"click","target":{"selector":"#submit-button"}}}'

# Fill form
curl -X POST http://127.0.0.1:3845/api/command \
  -H "Content-Type: application/json" \
  -d '{"command":{"id":"2","action":"fill","target":{"name":"email"},"value":"user@example.com"}}'

# Get page info
curl http://127.0.0.1:3845/api/page
```
