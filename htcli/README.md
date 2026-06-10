# htcli — How-To Recorder CLI

Go CLI for controlling browser tabs via the [How-To Recorder](https://github.com/u007/how-to-recorder) remote control API.

```
htcli (Go) ──HTTP──► Server (Bun, port 3845) ──WebSocket──► Extension ──DOM──► Chrome
```

## Installation

### From source

```bash
git clone https://github.com/u007/how-to-recorder.git
cd how-to-recorder/htcli
make build
./bin/htcli --help
```

### Install globally

```bash
go install github.com/u007/htcli/cmd/htcli@latest
```

## Quick Start

```bash
# 1. Start the How-To Recorder server
cd /path/to/how-to-recorder
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

- [How-To Recorder](https://github.com/u007/how-to-recorder) Chrome extension installed
- How-To Recorder server running (`bun run server`)
- Go 1.22+ (for building from source)

## License

MIT
