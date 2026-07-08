# HTR NControl

A browser extension that records user interactions (clicks, inputs, navigation) with screenshots and optional audio narration, then exports them as step-by-step documentation.

The same source code ships as a **Chrome extension** (Manifest V3) and a **Firefox extension** (Manifest V3 with `sidebar_action`). See [`firefox/`](./firefox/) for the Firefox build.

## Features

- 📸 **Screenshot Capture** - Automatically captures screenshots during interactions
- 🎯 **Smart Element Detection** - Tracks clicks and inputs with intelligent selectors
- 🔒 **Sensitive Data Protection** - Automatically masks passwords and sensitive fields
- 📝 **Multiple Export Formats** - JSON, Markdown, and ZIP with images
- 🦊 **Cross-browser** - Chrome side panel and Firefox sidebar (identical UI, same React source)
- 🎨 **Visual Timeline** - Side panel interface for managing recordings
- 🎙️ **Audio Support** (planned) - Add voice narration to recordings

## Installation

### Build & Load Locally

1. **Clone the repository**
   ```bash
   git clone https://github.com/AStevensTaylor/htrncontrol.git
   cd htrncontrol
   ```

2. **Install dependencies**
   ```bash
   bun install
   ```

3. **Start the dev server** (with hot reload)
   ```bash
   bun run dev
   ```

4. **Load in Chrome**
   - Open `chrome://extensions/`
   - Enable **Developer mode** (toggle in top right)
   - Click **Load unpacked**
   - Select the `build/` directory from this project
   - The extension icon should appear in your toolbar

5. **(Optional) Start the remote control server**
   ```bash
   bun run server
   ```
   The server will display a bearer token on startup — use this for API authentication.

### Firefox (Build & Load Locally)

Firefox uses the same `bun` toolchain with a separate `firefox/` workspace that re-uses 100% of the shared source under `src/`:

1. **Build the Firefox extension** (Manifest V3 with `sidebar_action`)
   ```bash
   bun run firefox:build
   ```
2. **Load temporarily in Firefox**
   - Open `about:debugging#/runtime/this-firefox` in Firefox
   - Click **Load Temporary Add-on…**
   - Select `firefox/build/manifest.json`
3. **Open the sidebar** — click the extension's toolbar icon (or use `Ctrl+B` after it's focused) to open the sidebar UI. It's the same React UI as the Chrome side panel.
4. **(Optional) Build a distributable `.xpi`**
   ```bash
   bun run firefox:zip
   ```
   This writes `firefox/htrncontrol-firefox.xpi`, ready for [addons.mozilla.org](https://addons.mozilla.org) upload.

See [`firefox/README.md`](./firefox/README.md) for the full architecture, the polyfill strategy, and how the Chrome and Firefox builds share one source tree.

### From Release

1. Download the latest `.crx` file from [Releases](https://github.com/AStevensTaylor/htrncontrol/releases)
2. Open `chrome://extensions/`
3. Enable "Developer mode"
4. Drag and drop the `.crx` file onto the extensions page

## Usage

1. **Start Recording**
   - Click the extension icon or open the side panel
   - Click "Start Recording"

2. **Perform Actions**
   - Navigate websites, click buttons, fill forms
   - Each action is captured with a screenshot

3. **Stop Recording**
   - Click "Stop Recording" in the side panel

4. **Export**
   - Choose from JSON, Markdown, or ZIP formats
   - Review and download your documentation

## Remote Control

HTR NControl includes a built-in remote control system that allows external tools (like AI agents) to control browser tabs via an HTTP/WebSocket API.

### Quick Start

```bash
# 1. Start the API server
bun run server

# 2. The server displays an auth token on startup:
#    🔑 Auto-generated bearer token: abc123...

# 3. Connect the extension (enable remote control via message or URL parameter)

# 4. Send commands
curl -H "Authorization: Bearer abc123..." http://127.0.0.1:3845/api/tabs
```

### Architecture

```
┌─────────────────┐     HTTP      ┌─────────────────┐     WebSocket     ┌─────────────────┐
│   External Tool  │ ◄───────────► │   API Server     │ ◄───────────────► │   Extension     │
│   (AI Agent)     │               │   (port 3845)    │                   │   (Content)     │
└─────────────────┘               └─────────────────┘                   └─────────────────┘
```

### Authentication (Enabled by Default)

Both IP whitelist and bearer token are enabled by default:

- **IP Whitelist**: Only `127.0.0.1`, `localhost`, `::1` allowed
- **Bearer Token**: Auto-generated random 32-char hex token (displayed on server start)

```bash
# Override with custom token
HTR_BEARER_TOKEN="my-secret" bun run server

# Disable bearer token (IP whitelist only)
HTR_ENABLE_BEARER_TOKEN=false bun run server

# Add IPs to whitelist
HTR_ALLOWED_IPS="127.0.0.1,192.168.1.100" bun run server
```

### API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/health` | Health check |
| `GET` | `/api/tabs` | List connected browser tabs |
| `GET` | `/api/tabs/:id` | Get specific tab info |
| `POST` | `/api/tabs/:id/command` | Execute command on specific tab |
| `POST` | `/api/command` | Execute command on active tab |
| `GET` | `/api/page` | Get page info (URL, title, dimensions) |
| `GET` | `/api/screenshot` | Capture screenshot |

### Command Examples

```bash
# Click an element
curl -X POST http://127.0.0.1:3845/api/command \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"command":{"id":"1","action":"click","target":{"selector":"#submit-button"}}}'

# Fill a form field by name
curl -X POST http://127.0.0.1:3845/api/command \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"command":{"id":"2","action":"fill","target":{"name":"email"},"value":"user@example.com"}}'

# Find element by text
curl -X POST http://127.0.0.1:3845/api/command \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"command":{"id":"3","action":"find","target":{"text":"Submit","tag":"button"}}}'

# Navigate to URL
curl -X POST http://127.0.0.1:3845/api/command \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"command":{"id":"4","action":"navigate","value":"https://example.com"}}'

# Get page info
curl -H "Authorization: Bearer YOUR_TOKEN" http://127.0.0.1:3845/api/page
```

### Element Targeting

Find elements using various strategies:

```json
{
  "selector": "#submit-button",        // CSS selector
  "xpath": "//button[@type='submit']", // XPath
  "id": "submit-button",               // ID attribute
  "name": "email",                     // Name attribute
  "role": "button",                    // ARIA role
  "label": "Email Address",            // Associated label
  "placeholder": "Enter email",        // Placeholder text
  "text": "Submit",                    // Text content
  "tag": "button",                     // HTML tag
  "type": "email"                      // Input type
}
```

### Available Actions

| Category | Actions |
|----------|---------|
| **Finding** | `find`, `findAll`, `wait`, `isVisible`, `isEnabled`, `xpath` |
| **Inspection** | `getValue`, `getAttribute`, `getText`, `getHTML`, `getBoundingBox`, `getComputedStyle`, `getPageInfo` |
| **Interaction** | `click`, `dblclick`, `rightclick`, `hover`, `focus`, `blur`, `scrollTo` |
| **Form Input** | `fill`, `type`, `clear`, `select`, `check`, `uncheck`, `pressKey`, `selectText` |
| **Navigation** | `navigate`, `reload`, `goBack`, `goForward` |
| **Visual** | `screenshot`, `highlight`, `unhighlight` |
| **Script** | `evaluate` (execute JavaScript) |

### WebSocket Protocol

The extension connects to the server via WebSocket for real-time command execution:

```javascript
// Connect via WebSocket (token as query parameter)
const ws = new WebSocket('ws://127.0.0.1:3845?token=YOUR_TOKEN');

// Send command
ws.send(JSON.stringify({
  type: 'command',
  command: { id: '1', action: 'click', target: { selector: '#btn' } }
}));

// Receive result
ws.onmessage = (event) => {
  const result = JSON.parse(event.data);
  console.log(result);
};
```

### Full Documentation

See [server/README.md](./server/README.md) for complete API reference and examples in Python, JavaScript, and curl.

## Development

### Prerequisites

- [Bun](https://bun.sh/) v1.3.5 or higher
- Chrome/Chromium browser

### Commands

```bash
bun install          # Install dependencies
bun run dev          # Start dev server with HMR
bun run build        # Build for production
bun run test         # Run tests
bun run check        # Lint and format check
bun run check:fix    # Auto-fix lint/format issues
bun run zip          # Build and create distributable ZIP
bun run server       # Start remote control API server
bun run server:dev   # Start server with hot reload
```

### Project Structure

```
src/
├── background/       # Service worker (orchestrates recording)
├── contentScript/    # Injected scripts (track interactions + remote control)
├── sidepanel/        # React UI (control panel and timeline)
│   ├── components/   # UI components
│   └── context/      # React context providers
├── types/            # TypeScript definitions
├── utils/            # Export and utility functions
├── manifest.ts       # Extension manifest configuration
└── server/           # Remote control API server
```

### Tech Stack

- **Runtime**: Bun
- **Framework**: React 18 + TypeScript
- **Build Tool**: Vite + @crxjs/vite-plugin
- **Linting/Formatting**: Biome
- **Storage**: IndexedDB (via idb)
- **Bundling**: JSZip for exports

## CI/CD

This project uses GitHub Actions for automated testing and releases:

- **PR Checks**: Automatic linting, type checking, and testing on pull requests
- **Releases**: Automatic `.crx` packaging and GitHub releases on merge to `main`
- **Chrome Web Store**: Automated publishing (requires setup)

See [CI.md](./CI.md) for detailed CI/CD configuration and setup instructions.

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes
4. Run tests and linting (`bun run test && bun run check`)
5. Commit your changes (`git commit -m 'feat: add amazing feature'`)
6. Push to the branch (`git push origin feature/amazing-feature`)
7. Open a Pull Request

See [AGENTS.md](./AGENTS.md) for detailed development guidelines and code style conventions.

## License

MIT License - see [LICENSE](./LICENSE) for details.

## Authors

- **Ahren Stevens-Taylor** — Original author <github+htrcontrol@mercstudio.com>
- **James Tan** — Owner, remote control system <james.tan@aims-research.com>

## Acknowledgments

Built with:
- [Vite](https://vitejs.dev/)
- [React](https://react.dev/)
- [CRXJS](https://crxjs.dev/)
- [Biome](https://biomejs.dev/)
- [Bun](https://bun.sh/)
