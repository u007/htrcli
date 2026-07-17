# Setup Guide: Chrome Extension + `htrcli serve`

This guide covers wiring the Chrome extension to `htrcli` via the native-messaging
daemon (`htrcli serve`) — no Bun `server/` process needed. `htrcli serve` provides
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

## 2. Build htrcli

```bash
cd htrcli
make build            # → htrcli/bin/htrcli
# or: make htrcli-install   (go install, puts htrcli on PATH globally)
```

`htrcli install` (step 3) requires `htrcli` to already be resolvable via
`exec.LookPath`, so make sure the binary is on `PATH`:

```bash
export PATH="$PATH:/Users/james/www/htrncontrol/htrcli/bin"
```

## 3. Register the native messaging host

```bash
htrcli install --browser chrome --extension-id <chrome-extension-id>
```

This writes `com.htrcontrol.host.json` to Chrome's native messaging
manifest directory (macOS: `~/Library/Application Support/Google/Chrome/NativeMessagingHosts/`),
pointing at your `htrcli` binary and allowing your extension ID to connect.

Then **reload the extension** at `chrome://extensions/` so it re-reads the
host registration.

To undo: `htrcli install --browser chrome --uninstall`

## 4. Start the daemon

```bash
htrcli serve
```

This binds:
- HTTP API on `:3845` (same surface as the Bun server)
- A Unix socket that the extension's native messaging relay connects to

Only one process can hold `:3845`.

If no bearer token is set, `htrcli serve` starts **unauthenticated** and warns
you. Set a token first (recommended):

```bash
htrcli config set-token <your-token>
htrcli serve            # picks up the saved token automatically
# or override per-run: HTR_BEARER_TOKEN=<token> htrcli serve
```

## 5. Configure htrcli as a client and verify

```bash
htrcli config set-server http://127.0.0.1:3845
htrcli config set-token <same-token-as-above>
htrcli health
```

`htrcli health` should report a connected extension. If not:
- Confirm the extension was reloaded after `htrcli install`
- Confirm `htrcli serve` is running and the extension shows as connected (check the side panel / background console for `[NativeHost]` logs)

## 6. Drive the browser

```bash
htrcli tabs list
htrcli open https://example.com
htrcli snapshot -i
htrcli click @e3
htrcli screenshot page.png
```

## Firefox (optional, same daemon)

Chrome and Firefox can both be registered and connected to the same `htrcli serve`
instance at once; commands route to whichever browser owns the target tab.

```bash
bun run firefox:build
# Load firefox/build/manifest.json via about:debugging#/runtime/this-firefox
htrcli install --browser firefox --extension-id htrncontrol@mercstudio.com
# Reload the add-on in about:debugging, then htrcli serve is already covering it
```

If you don't install the native host (or the daemon isn't running), the
Firefox extension automatically falls back to a direct WebSocket connection
to the Bun server — see **Alternative: Bun server** below. The side-panel
indicator shows **Online** for either transport.

## CDP transport: direct Chrome control

`htrcli` can also drive Chrome directly over the Chrome DevTools Protocol with
`--cdp` (or `htrcli config set-transport cdp`). Use this for browser-restricted
pages like the Chrome Web Store dev console, or for headless/background runs
where you don't want the extension involved.

```bash
# Start a dedicated Chrome with a fresh profile at ~/.htrcli/chrome-profile.
htrcli browser start
htrcli browser start --headless

htrcli browser status
htrcli browser hide
htrcli browser show
htrcli browser stop

# Once Chrome is started, point commands at the CDP transport.
htrcli --cdp open https://chrome.google.com/webstore/.../console
htrcli --cdp fill "#email" "me@example.com"
htrcli --cdp click "#submit"
htrcli --cdp screenshot out.png
htrcli --cdp tabs list
```

Notes:

- Sign in once with `htrcli browser start` in visible mode, then re-use the same
  dedicated profile for headless/background automation.
- `--tab` means a numeric extension tab ID on the default transport, but a CDP
  target ID on `--cdp`.
- The debugging port is a localhost-only control channel into a signed-in
  profile; treat it like a trusted local admin surface.

