# Setup Guide: Chrome Extension + `htcli serve`

This guide covers wiring the Chrome extension to `htcli` via the native-messaging
daemon (`htcli serve`) — no Bun `server/` process needed. `htcli serve` provides
the same HTTP API on `:3845` itself, talking to the extension over a native
messaging relay instead of WebSocket.

## 1. Build & load the extension

```bash
cd /Users/james/www/htrncontrol
bun install
bun run build        # → build/
```

- Open `chrome://extensions/`
- Enable **Developer mode**
- **Load unpacked** → select `build/`
- Copy the **Extension ID** shown on the card (needed in step 3)

## 2. Build htcli

```bash
cd htcli
make build            # → htcli/bin/htcli
# or: make htcli-install   (go install, puts htcli on PATH globally)
```

`htcli install` (step 3) requires `htcli` to already be resolvable via
`exec.LookPath`, so make sure the binary is on `PATH`:

```bash
export PATH="$PATH:/Users/james/www/htrncontrol/htcli/bin"
```

## 3. Register the native messaging host

```bash
htcli install --browser chrome --extension-id <chrome-extension-id>
```

This writes `com.howtorecorder.host.json` to Chrome's native messaging
manifest directory (macOS: `~/Library/Application Support/Google/Chrome/NativeMessagingHosts/`),
pointing at your `htcli` binary and allowing your extension ID to connect.

Then **reload the extension** at `chrome://extensions/` so it re-reads the
host registration.

To undo: `htcli install --browser chrome --uninstall`

## 4. Start the daemon

```bash
htcli serve
```

This binds:
- HTTP API on `:3845` (same surface as the Bun server)
- A Unix socket that the extension's native messaging relay connects to

Only one process can hold `:3845` — don't run `bun run server` at the same time.

If no bearer token is set, `htcli serve` starts **unauthenticated** and warns
you. Set a token first (recommended):

```bash
htcli config set-token <your-token>
htcli serve            # picks up the saved token automatically
# or override per-run: HTR_BEARER_TOKEN=<token> htcli serve
```

## 5. Configure htcli as a client and verify

```bash
htcli config set-server http://127.0.0.1:3845
htcli config set-token <same-token-as-above>
htcli health
```

`htcli health` should report a connected extension. If not:
- Confirm the extension was reloaded after `htcli install`
- Confirm `htcli serve` is running and the extension shows as connected (check the side panel / background console for `[NativeHost]` logs)

## 6. Drive the browser

```bash
htcli tabs list
htcli open https://example.com
htcli snapshot -i
htcli click @e3
htcli screenshot page.png
```

## Firefox (optional, same daemon)

Chrome and Firefox can both be registered and connected to the same `htcli serve`
instance at once; commands route to whichever browser owns the target tab.

```bash
bun run firefox:build
# Load firefox/build/manifest.json via about:debugging#/runtime/this-firefox
htcli install --browser firefox --extension-id htrcontrol@mercstudio.com
# Reload the add-on in about:debugging, then htcli serve is already covering it
```

If you don't install the native host (or the daemon isn't running), the
Firefox extension automatically falls back to a direct WebSocket connection
to the Bun server — see **Alternative: Bun server** below. The side-panel
indicator shows **Online** for either transport.

## Alternative: Bun server instead of `htcli serve`

If you'd rather use the WebSocket-based Bun server instead of the native
messaging daemon:

```bash
bun run server          # prints a bearer token on startup
htcli config set-server http://127.0.0.1:3845
htcli config set-token <printed-token>
htcli health
```

No `htcli install` / native messaging registration is needed for this path —
the extension connects to the Bun server directly via WebSocket. Only one of
the two servers (`bun run server` or `htcli serve`) can hold `:3845` at a time.
