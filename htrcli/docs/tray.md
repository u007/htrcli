# htrcli tray icon

When you run `htrcli serve` on a desktop, a small icon appears in your
menu bar (macOS) or system tray (Windows, Linux). Click it for live
status, lifecycle controls, and common maintenance tasks.

## What it does

The tray menu has two read-only labels and a Maintenance submenu:

- **Status** — port, number of connected browsers, and a quick
  `ok` / `error` indicator.
- **Last error** — the most recent non-fatal error from the daemon
  (cleared after 5 minutes or on the next success).
- **Maintenance**:
  - **Reinstall native host ▸ Chrome / Firefox** — re-register the browser
    extension's native-messaging host. Use this after the extension gets a
    new ID (configure it first with `htrcli config set-extension-id <id>
    --browser <chrome|firefox>`).
  - **Open config folder** — opens `~/.htrcli/` in Finder/Explorer/your
    file manager.
  - **Copy bearer token** — copies the bearer token to the clipboard
    for 30 seconds, then overwrites it with `<cleared by htrcli>`.
  - **Show recent log** — opens `~/.htrcli/serve.log` in your
    default app. (The log is only written when the tray is attached.)
  - **Restart** — cleanly restart the daemon (re-applies config). The
    bearer token is passed to the restarted process via the environment,
    never on the command line.
  - **Quit** — cleanly shut down the daemon.

## Headless behavior

The tray is automatically skipped when:

- No `DISPLAY` is set (X11)
- No `WAYLAND_DISPLAY` is set (Wayland)
- You're logged in over SSH (`SSH_CONNECTION` or `SSH_TTY` is set)
- You pass `--no-tray` or set `HTRCLI_NO_TRAY=1`

On a headless Linux server (no display, or logged in over SSH), `htrcli
serve` prints one info line and continues as a pure daemon — no tray, no
error, no change in exit code.

## Disabling the tray

```bash
htrcli serve --no-tray
# or
HTRCLI_NO_TRAY=1 htrcli serve
```

This is required in CI, systemd units, Docker containers, and any
non-interactive context.

## Per-platform notes

- **macOS** — the icon appears in the menu bar. Uses `open` to open
  folders/files and `pbcopy` for the clipboard.
- **Windows** — the icon appears in the system tray (notification area).
  Uses `explorer` and `clip.exe`.
- **Linux (X11)** — uses `xdg-open` and `xclip -selection clipboard`.
- **Linux (Wayland)** — uses `xdg-open` and `wl-copy`. **The icon only
  appears if you have the `gnome-shell-extension-appindicator` extension
  (or the Ayatana indicator) installed.** Without it, the process runs
  fine but no icon is visible — this is a GNOME/Wayland limitation, not a
  bug in htrcli.

## Troubleshooting

- **No icon on GNOME/Wayland** — install
  `gnome-shell-extension-appindicator` (or `ayatana-indicator`), then
  restart `htrcli serve`.
- **"Reinstall native host" does nothing** — you haven't configured an
  extension ID yet. Run `htrcli config set-extension-id <id> --browser
  <chrome|firefox>` first.
- **"Show recent log" says no log yet** — the log file is only created
  when the tray is attached. Run `htrcli serve` on a desktop to enable it.

## Design notes

The tray is a true add-on: `htrcli serve` remains the only command
operators run, and headless servers are completely unaffected. The main
goroutine drives the tray UI; the HTTP server, signal handler, and daemon
sweeper run as background goroutines behind it. The `Controller` interface
is the seam between the tray and the daemon, which keeps the `systray`
dependency isolated to the `internal/tray` package.

See `docs/superpowers/specs/2026-07-18-htrcli-desktop-tray-design.md` for
the full design rationale.
