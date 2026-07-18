# htrcli Desktop Tray — Design

**Date:** 2026-07-18
**Status:** Approved (pending review of written spec)

## Problem

`htrcli serve` is a long-running Go daemon (TCP :3845 + Unix socket) that controls browser tabs through native messaging. On a developer desktop, operators currently interact with it three ways: (1) restart from a terminal, (2) read log lines from stderr or a tmux pane, (3) run `htrcli install` in another terminal to re-register the native host after an extension update. None of these is "glance at the menu bar and click."

Meanwhile on headless Linux servers, the same `htrcli serve` runs unchanged — and must keep doing so, with zero behavior difference. The tray must be a true add-on, not a new dependency for server installs.

The goal is a cross-platform system-tray icon, auto-attached to `htrcli serve` on desktops, silently skipped on headless servers, that gives operators lifecycle control, live status, and a few high-value maintenance actions.

## Goals

- One process: `htrcli serve` continues to be the only thing operators run. The tray is auto-attached, not a separate command.
- Cross-platform: macOS, Windows, and Linux desktops all show a working menu.
- Lifecycle + status + actions: Start / Stop / Restart; live status (port, relay count, last error); maintenance actions (reinstall native host, open config folder, copy bearer token, show recent log).
- Headless-safe: on a Linux server with no display, the daemon prints one info line and continues as a pure daemon. No tray, no error, no exit-code change.
- Opt-out: a `--no-tray` flag and `HTRCLI_NO_TRAY=1` env var for systemd units, Docker containers, and CI.

## Non-goals

- A new `htrcli tray` subcommand. The tray is a side effect of `htrcli serve`, not a peer command.
- A separate tray binary. Two artifacts to ship is not worth the cleaner separation; the seam is the `Controller` interface.
- Platform-specific tray code (`tray_darwin.go` / `tray_linux.go` / `tray_windows.go`). `getlantern/systray` abstracts the three.
- Log rotation. `~/.htrcli/serve.log` grows unbounded in v1; can add `lumberjack` later.
- A native settings window. The menu IS the UI. (Re-)configuring extension IDs happens via `htrcli config set-extension-id`.
- Auto-start at login. v1 is run-on-demand; a future `htrcli tray --install-autostart` could add a LaunchAgent / .desktop file.
- Start/Stop lifecycle controls in the menu. The tray exposes only Restart and Quit; the underlying daemon does not support start/stop without a redesign (see §"Configuration prerequisite" and the Controller interface in §"Architecture").

## Prerequisites (upstream changes required before/during this feature)

Three small, additive changes to existing code must land alongside the tray feature:

1. **`internal/commands/config.go`** — add `ExtensionID` to `configData`, add `htrcli config set-extension-id <id> [--browser chrome|firefox]`. The tray's "Reinstall native host" submenu depends on this.
2. **`internal/host/bridge.go`** — `StartUnixSocketServer` must return its `net.Listener` so the caller can close it during shutdown. The tray's clean-shutdown sequence depends on this.
3. **`internal/commands/serve.go:72`** — change `fmt.Printf("[htrcli serve] Using bearer token: %s\n", bearerToken)` to print only the fingerprint via `log.Printf` (see §S2 in Security). This is a one-line change but it's a behavior change visible in journald/launchd.

All three are small, additive, and ship in the same PR as the tray feature.

## Design

### Architecture (new package, three files)

New package `htrcli/internal/tray`, in three small files plus a build-tag-gated stub:

| File | Purpose | Build tags |
|---|---|---|
| `tray.go` | `Run(ctrl, icon)` (blocks main goroutine on `systray.Run` until `systray.Quit`), `ShouldStart()`. Builds the menu, runs the click-dispatch goroutine, owns the 5s refresh goroutine. | always |
| `controller.go` | The `Controller` interface (7 methods — see below), the `Status` struct, the `daemonController` implementation, the `Commander` interface for `os/exec` mocking. | always |
| `detect.go` | Pure-function `ShouldStart(noTray bool) bool`. No I/O, no goroutines, no OS calls — only env-var reads. | always |
| `tray_disabled.go` | No-op stubs for `Run` / `ShouldStart` returning `false`. Prevents the `getlantern/systray` import from breaking compilation on unsupported GOOS (e.g. `freebsd`, `openbsd`, `js`). | `//go:build !darwin && !linux && !windows` |

**`systray.Run` is called from the main goroutine** (in `tray.Run`). The library's package `init()` calls `runtime.LockOSThread()` and `systray.Run` enters the platform GUI event loop; this MUST happen on the main thread. `serve.go` is restructured so the main goroutine owns the tray, and the HTTP server, signal handler, and daemon goroutines run in the background (see §"Single-process guarantee").

The `Controller` interface is the seam between the tray and the daemon. Tests drive the menu with a `fakeController` that satisfies the interface and returns canned `Status` / `RecentLog` values. `serve.go` constructs the real controller and never imports `getlantern/systray` directly.

```go
// Controller is the surface the tray needs from the daemon.
// Start/Stop are intentionally absent: htrcli serve is a single-process
// daemon that owns its HTTP port; suspending and resuming the daemon
// without exiting the process is not supported. The tray exposes only
// Restart (re-execs the process) and Quit (exits the process).
type Controller interface {
    // Lifecycle
    IsRunning() bool              // always true while the tray is attached;
                                  // used for UI feedback after a refresh tick
    Restart() error               // d.Stop() + exec self + os.Exit(0)
    Quit() error                  // os.Exit(0); for the Quit menu item

    // Status (read-only; called by the refresh goroutine every 5s)
    Status() Status
    RecentLog(n int) []string

    // Maintenance
    ReinstallHost(browser string) error
    OpenConfigFolder() error
    OpenLog() error                    // open ~/.htrcli/serve.log in OS default app
    CopyTokenToClipboard() (string, error)
}

type Status struct {
    Port             int
    RelaysConnected  int
    LastError        string
    TokenFingerprint string // e.g. "a1b2…f3e4"; "—" when unset
}
```

**Rationale:** `*host.Daemon` exposes a one-shot `Stop()` (closes the sweeper's stop channel) and no `Start()`. A `Start` menu item that re-binds port 3845 is not implementable without redesigning the daemon. The user-visible options collapse to:
- **Restart** — clean way to apply config changes or recover from a stuck state.
- **Quit** — clean way to shut the daemon down from the menu bar.

### Detection rules (`detect.go`)

`ShouldStart(noTray bool) bool` is a pure function:

| Platform | Decision |
|---|---|
| macOS, Windows | always `true` (native system tray) |
| Linux | `true` only if (a) at least one of `DISPLAY` / `WAYLAND_DISPLAY` is set, **and** (b) neither `SSH_CONNECTION` nor `SSH_TTY` is set |
| Any | `false` if `noTray` is true or `HTRCLI_NO_TRAY=1` |

The SSH rule is the safety net for the original concern: a user SSH'd into a desktop box with X forwarding should not get a tray (they're not at the keyboard, and the tray adds nothing to that session).

This function is the highest-leverage test in the feature; its table-driven test lives in `detect_test.go` and is the canonical regression catcher for "did we accidentally put a tray on a server."

### Menu tree (cross-platform)

```
htrcli (root icon)
├── Status: 3845 · 2 relays · ok              ← disabled label, refreshed 5s
├── Last error: —                              ← disabled label, refreshed 5s
├── ───────
├── Maintenance ▸
│   ├── Reinstall native host ▸
│   │   ├── Chrome                            ← disabled if no ext-id configured
│   │   └── Firefox                           ← disabled if no ext-id configured
│   ├── Open config folder
│   ├── Copy bearer token                     ← flashes "Copied" for 2s
│   ├── Show recent log                       ← opens ~/.htrcli/serve.log in OS default app
│   └── ───────
│   ├── Restart                               ← exits, re-execs htrcli serve
│   └── Quit                                  ← exits cleanly
```

Disabled items (labels + start/stop gating) are re-evaluated on the 5s tick AND immediately after any successful Start/Stop/Restart click, so the UI doesn't lag 5s after a click.

### Click dispatch and refresh

```go
// tray.go (dispatch loop and refresh loop; both run as goroutines
// spawned from systray.onReady)
func (t *Tray) dispatchLoop(ctx context.Context) {
    defer func() {
        if r := recover(); r != nil {
            log.Printf("[htrcli tray] recovered from panic: %v", r)
        }
    }()
    for {
        select {
        case <-ctx.Done():
            return
        case <-mMaintenanceReinstallChrome.ClickedCh:
            t.runAction("Reinstall Chrome", func() error { return t.ctrl.ReinstallHost("chrome") })
        case <-mMaintenanceReinstallFirefox.ClickedCh:
            t.runAction("Reinstall Firefox", func() error { return t.ctrl.ReinstallHost("firefox") })
        case <-mMaintenanceOpenConfig.ClickedCh:
            t.runAction("Open config", t.ctrl.OpenConfigFolder)
        case <-mMaintenanceCopyToken.ClickedCh:
            t.runActionWithToast("Copy", t.ctrl.CopyTokenToClipboard)
        case <-mMaintenanceShowLog.ClickedCh:
            t.runAction("Show log", t.ctrl.OpenLog)
        case <-mMaintenanceRestart.ClickedCh:
            t.runAction("Restart", t.ctrl.Restart)
        case <-mMaintenanceQuit.ClickedCh:
            t.runAction("Quit", t.ctrl.Quit)
        }
    }
}

func (t *Tray) refreshLoop(ctx context.Context) {
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()
    t.refresh()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            t.refresh()
        }
    }
}
```

Notes on the dispatch loop:
- The `Refresh` after a Restart/Quit click is intentionally **not** called. Restart/Quit tear down the process; the refresh would race with shutdown.
- `OpenLog` is a new Controller method (added during this revision) that opens the log file in the OS default app. Distinct from `RecentLog` (which returns the lines) because the menu item opens the file, not a text buffer.
- The `m*` constants are the `*systray.MenuItem` values returned by `systray.AddMenuItem` / `AddSubMenuItem` during `onReady`.

The refresh interval is 5s. The 5s choice is the UX trade-off: 2s is snappier but means 30 menu repaints/minute, 10s feels passive. 5s reads as "live" without being noisy.

### Single-process guarantee (main thread, not goroutine)

`getlantern/systray` calls `runtime.LockOSThread()` in its package `init()` and `systray.Run` enters the platform GUI event loop (NSApplication on macOS, AppIndicator/GtkStatusIcon on Linux, Win32 on Windows). The library's own example calls `systray.Run` from `main()`. **Calling `systray.Run` from a non-main goroutine is unsupported and breaks on macOS** because NSApplication expects to own the main thread.

The architecture is therefore inverted from a typical "spawn the UI" pattern:

- **Main goroutine** → `systray.Run(onReady, onExit)`. This blocks until `systray.Quit()` is called.
- **Background goroutines** → HTTP server, Unix-socket server, sweeper, refresh loop, click dispatch.
- **Signal goroutine** → blocks on `<-sigCh`, drives the explicit shutdown sequence.

`serve.go` is restructured so the main function is:

```go
func main() {
    // ...resolve bearerToken, port, d, srv, ln (unchanged from current serve.go)...

    // NOTE: the existing `defer d.Stop()` in serve.go is REMOVED.
    // d.Stop() is now called explicitly from performShutdown().

    // Spawn HTTP server in its own goroutine.
    httpErrCh := make(chan error, 1)
    go func() { httpErrCh <- srv.Serve(ln) }()

    // Spawn Unix-socket server, sweeper (unchanged).

    // Spawn signal handler goroutine.
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
    go func() {
        select {
        case <-sigCh:
        case err := <-httpErrCh: // exit if HTTP server dies unexpectedly
            log.Printf("[htrcli serve] HTTP server exited: %v", err)
        }
        performShutdown()  // explicit, NOT deferred
    }()

    // Main goroutine: drive the tray (blocks until systray.Quit).
    if tray.ShouldStart(noTray) {
        ctrl := newDaemonController(d, port, bearerToken, selfPath)
        icon, _ := iconFS.ReadFile("icon.png")
        tray.Run(ctrl, icon)  // blocks here
    } else {
        log.Printf("[htrcli serve] Tray disabled (no display or HTRCLI_NO_TRAY set)")
        <-httpErrCh  // block on HTTP server instead
    }
}
```

`tray.Run` (replaces `Start`/`Stop` because lifecycle is fused to the main goroutine) blocks until `systray.Quit`. The signal handler goroutine calls `performShutdown()` which explicitly sequences the steps below. **No `defer` is used for tray teardown** — the explicit ordering is required because `tray.Stop()` is non-deferrable when the main goroutine owns `systray.Run`.

### Maintenance action implementations (`daemonController`)

All maintenance actions take a `Controller`-level dependency on a `getToken func() string` closure (the same env → file → viper resolution that `serve.go` uses today) and a `getExtID func(browser string) string` closure that reads from the htrcli config (see "Configuration prerequisite" below).

`Restart()` closes the HTTP listener (`ln.Close()`) and Unix-socket listener (held on the daemon), then `exec.Command(selfPath, "serve", args...)` with the original args **minus any `--token` or other secret-carrying flag** (see Security §S1 below), then `os.Exit(0)` on the parent. The OS reaps the new process; the old one exits. `selfPath` is captured at controller-construction time via `os.Executable()` and stored on `daemonController`. **Closing the listener first is required** to avoid the port-binding race where the new process fails with "port already in use" because the old process still holds it.

`Quit()` calls `performShutdown()` (defined in `serve.go`; same code path as signal-driven shutdown) and then `os.Exit(0)`. The Quit menu item and SIGINT/SIGTERM share the shutdown sequence.

`ReinstallHost(browser)` reads the extension ID for the given browser from the htrcli config (`getExtID(browser)`). If missing or empty, returns a *helpful* error: "Run `htrcli config set-extension-id <id>` first." Otherwise, shells out to `selfPath install --browser <b> --extension-id <id>`.

`OpenConfigFolder()` opens `~/.htrcli/` — the actual htrcli config and runtime directory (matches `viper.AddConfigPath(home + "/.htrcli")` in `root.go:64` and `configDir := filepath.Join(home, ".htrcli")` in `config.go:92`). It does NOT use the legacy `~/.htrcontrol/` token-file fallback chain, which is a token-file concern only, not the config dir. The platform-native command is:
- macOS: `open <path>`
- Windows: `explorer <path>` (more idiomatic for folder-open than `rundll32`)
- Linux: `xdg-open <path>`

`CopyTokenToClipboard()` resolves the bearer token (via `getToken()` closure) and pipes it to:
- macOS: `pbcopy`
- Windows: `clip.exe`
- Linux: chooses by display server — if `WAYLAND_DISPLAY` is set, `wl-copy` (from `wl-clipboard`); otherwise `xclip -selection clipboard` with `xsel` as a fallback. This is required because `xclip` does not work on Wayland-native desktops.

Returns the token so the menu can show a "Copied (a1b2…f3e4)" toast.

`OpenLog()` opens `~/.htrcli/serve.log` in the OS default app (using the same `open` / `explorer` / `xdg-open` pattern as `OpenConfigFolder`). Distinct from `RecentLog` (which is for in-menu display) so the user can read the full log in their preferred editor/viewer.

### Configuration prerequisite (new in v1 of this feature)

The current `htrcli config` command has no `set-extension-id` subcommand, and the `configData` struct in `config.go:16-24` has no `ExtensionID` field. This feature requires both:

1. Add `ExtensionID` (and per-browser IDs if the user wants one for Chrome and a different one for Firefox) to `configData` in `config.go`.
2. Add `htrcli config set-extension-id <id> [--browser chrome|firefox]` to `internal/commands/config.go`. Without this, the "Reinstall native host" submenu is dead.

This is a small, additive change to existing config plumbing. It can ship in the same PR as the tray feature because the tray code depends on it.

### Log file (only when tray attaches)

`Show recent log` needs a real log file. Today the daemon uses `log.Printf` to stderr. When the tray attaches, `serve.go` adds a second writer:

```go
if trayAttached {
    logPath := filepath.Join(logDir, "serve.log")  // logDir = ~/.htrcli/
    logFile, _ := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
    log.SetOutput(io.MultiWriter(os.Stderr, logFile))
}
```

Stderr is preserved so systemd journal / launchd logs still get the lines. The MultiWriter is unconditional and zero-cost when `logFile` is nil. Server runs are completely unchanged (the MultiWriter only attaches when the tray attaches).

`RecentLog(n int)` reads the last `n` lines of `~/.htrcli/serve.log` (simple reverse-buffer read, no tail library needed for a small file).

### Shutdown ordering (explicit, not deferred)

Because the main goroutine owns `systray.Run` and we must terminate that loop first, **the shutdown sequence is an explicit function, not a defer chain**. This is required because `systray.Quit()` must be called from outside `systray.Run` to unblock the main goroutine, but `os.Exit(0)` from a signal handler would skip defers on the way out.

`performShutdown()` (in `serve.go`):

```go
func performShutdown() {
    // 1. Quit the tray (idempotent if not started). Returns immediately;
    //    causes systray.Run in the main goroutine to unblock and return.
    if trayAttached {
        systray.Quit()
    }

    // 2. Drain HTTP requests (5s timeout). Closes the listener so the
    //    port is freed before any future re-exec by Restart.
    shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    if err := srv.Shutdown(shutdownCtx); err != nil {
        log.Printf("[htrcli serve] HTTP shutdown: %v", err)
    }

    // 3. Stop the daemon (sweeper + relay connections).
    d.Stop()

    // 4. The Unix-socket server is stopped by closing the listener it
    //    holds. The goroutine spawned for it in serve.go must expose
    //    its listener; we add a small change to host.StartUnixSocketServer
    //    to return the listener so the caller can close it here.
    if unixLn != nil { unixLn.Close() }
}
```

The signal handler goroutine calls `performShutdown()` then returns. The main goroutine, which was blocked in `systray.Run`, unblocks. The headless branch (`<httpErrCh`) unblocks via the HTTP server's normal exit. Both paths converge to the bottom of `main`, which calls `os.Exit(0)`.

**Prerequisite change in `host.StartUnixSocketServer`:** the function must return its `net.Listener` so the caller can close it cleanly during shutdown. Today it discards the listener and runs the accept loop in a goroutine with no stop signal. This is a small, additive change to `internal/host/bridge.go`.

**Prerequisite change in `daemonController.Restart()`:** the HTTP listener (`ln`) must be tracked on the controller (or threaded through) so it can be closed before `exec.Command` to avoid the port-binding race.

The order matters: tray first (so the icon disappears cleanly), then HTTP (so the icon's last refresh sees a closed port and exits its 5s tick instead of flashing a connection error), then daemon (so in-flight commands can complete), then Unix socket (so the accept loop unblocks and the goroutine returns).

## Security

The tray introduces several new surfaces for secret exposure and process-state visibility. Each is addressed as follows.

### S1 — `Restart()` re-exec must not leak `--token` via argv

`htrcli --token <secret> serve` is a supported invocation (the `--token` flag is a global root flag, see `root.go:46`). If `daemonController.Restart()` re-execs the process with the **same args** literally, the full bearer token is written into the child's argv and visible to any process that can read `/proc/<pid>/cmdline` or run `ps`.

**Mitigation:** `Restart` filters the args before re-exec. A list of secret-carrying flags is maintained (`--token`, `--bearer-token`, plus anything new) and stripped. The restarted process re-resolves the token from `HTR_BEARER_TOKEN` env (inherited from the parent) or from the config file. Document this strip list in `daemonController.Restart()` so future secret flags are obvious additions.

### S2 — `serve.go` prints the full token to stdout at startup

`serve.go:72` does `fmt.Printf("[htrcli serve] Using bearer token: %s\n", bearerToken)`. This puts the full token in `journald` / `Console.app` / anywhere stdout is captured. The new `MultiWriter` log-redirection (§"Log file") only captures `log.*` output, not `fmt.Printf` — so the line would also be missing from `serve.log` and "Show recent log" would mislead the user.

**Mitigation:** change `serve.go:72` to `log.Printf("[htrcli serve] Using bearer token fingerprint: %s\n", fingerprint(bearerToken))` and define `fingerprint(token string) string` to return `first4…last4` (or `"—"` for empty). This is consistent with the tray's `Status.TokenFingerprint` and the user's menu shows the same value.

### S3 — Clipboard auto-clear after copy

`CopyTokenToClipboard` writes the bearer token to the system clipboard. On macOS, clipboard data persists indefinitely; clipboard managers (Klipper, Clipboard History Pro, macOS's built-in history) may retain the token permanently.

**Mitigation:** after a successful copy, spawn a 30s goroutine that overwrites the clipboard with a sentinel string (e.g., `<cleared by htrcli>`). This does not prevent a determined attacker who polls the clipboard in that 30s window, but raises the bar from "permanent" to "brief." Document in the tray doc that shared-workstation users should use `htrcli config` or the log file instead.

### S4 — Token fingerprint disclosure in the always-visible status

`Status.TokenFingerprint` ("a1b2…f3e4") is refreshed every 5s and always visible in the menu. On an unlocked workstation, a shoulder-surfer learns (a) that a token IS configured and (b) the first/last 4 characters.

**Mitigation:** Acceptable for v1 because the bearer token is 32+ random characters; 8 known characters do not meaningfully reduce the search space. The fingerprint is a UI affordance, not a secret. Document this trade-off in the design; revisit if the token length or generation policy changes.

### S5 — Token file permissions

`~/.htrcontrol/token` (the legacy token file) is created with default umask (typically 0644 = world-readable). The tray's "Open config folder" makes the file listing trivially visible in Finder/Explorer/Dolphin.

**Mitigation (out of scope for v1, but a future improvement):** `htrcli install` or `serve` should `os.Chmod` the token file to `0600` at creation. This is independent of the tray feature but the tray's `OpenConfigFolder` raises the practical risk. Track in a follow-up issue; do not block this feature on it.

### S6 — CGo / signal interaction on macOS

`getlantern/systray` uses CGo (Cocoa on macOS, AppIndicator on Linux, Win32 on Windows). The library calls `runtime.LockOSThread()` in its `init()`. CGo introduces the usual supply-chain and memory-safety risks; the macOS event loop also installs its own signal handling. The `signal.Notify` pattern must coexist with Cocoa's signal handling.

**Mitigation:** rely on the inverted main-goroutine architecture (main → `systray.Run`, signal handler in a separate goroutine) so the signal handler doesn't fight the main thread. Add a manual smoke test for "Cmd-Q on macOS → clean exit" before merging. Document the known risk in the package README; this is a pre-existing trait of the library, not a new one.

## Error handling

Every clickable menu item goes through one wrapper:

```go
func (t *Tray) runAction(name string, fn func() error) {
    if err := fn(); err != nil {
        t.showError(name, err)  // 6s title-flash: "✗ <name>: <truncated err>"
    }
}
```

Toasts use a 2s flash on a separate hidden status item ("Copied (a1b2…f3e4)"). Errors use 6s — they deserve a longer read window. Error messages are truncated to ~80 chars; full detail is in `~/.htrcli/serve.log`, which the user can open via "Show recent log."

### Failure modes

| Action | Likely error | UX |
|---|---|---|
| Restart | "port already in use" race (new process binds while old still holds) | Mitigated by closing the listener before `exec.Command`. If the rare race fires anyway, 6s title-flash surfaces the error. |
| Quit | `performShutdown` non-graceful exit (HTTP 5s timeout exceeded) | 6s title-flash; process still exits; user retries. |
| Reinstall native host | No extension ID in config | 6s title-flash: "Run `htrcli config set-extension-id <id>` first." |
| Open config folder | Folder doesn't exist | 6s title-flash: "Config folder not found at <path>." |
| Copy token | No token configured | 6s title-flash: "No bearer token set." |
| Show recent log | Log file doesn't exist (just-started daemon) | 6s title-flash: "No log yet." Open is skipped. |

The click-dispatch loop wraps its body in `defer recover()` so a single bad click never crashes the daemon.

## Testing strategy

### Unit tests on `ShouldStart()`

Highest-leverage test in the feature. Table-driven, no I/O. Critical cases:

| Case | Expected |
|---|---|
| macOS, no env | start |
| Windows, no env | start |
| Linux, `DISPLAY=:0` | start |
| Linux, `WAYLAND_DISPLAY=wayland-0` | start |
| Linux, both empty | skip |
| Linux, `DISPLAY=:0` + `SSH_CONNECTION=…` | **skip** (the safety-net case) |
| Linux, `SSH_TTY=/dev/pts/0` | skip |
| Linux, `DISPLAY=:0` + `HTRCLI_NO_TRAY=1` | skip |
| Linux, `DISPLAY=:0` + `--no-tray` | skip |

### Unit tests on the `Controller`

A `fakeController` records calls and returns canned values. Tests assert that clicking menu items routes to the right Controller method, and that errors flash the error label. The fake makes the 5s ticker overridable (50ms in test mode) so the refresh loop is testable in under a second.

### `os/exec` mock for platform glue

For `OpenConfigFolder` / `CopyTokenToClipboard`, the controller depends on a `Commander` interface. The real `Commander` runs `os/exec`; tests inject a `fakeCommander` that records the call. We test *which binary was chosen* and *which args* — not whether `xdg-open` actually opens a folder. The latter is a manual smoke test.

### Build-tag-gated `traytest`

```go
//go:build traytest
// +build traytest

package tray
```

A `fakeSystray` that exposes the same API as `getlantern/systray` and lets tests simulate clicks and assert menu titles. The build tag means this test never ships in production, and the production binary is unaffected.

### Manual smoke tests (documented, not automated)

| Platform | Check |
|---|---|
| macOS | `htrcli serve` → menu bar icon appears; menu opens; Cmd-Q on icon exits cleanly; SIGINT (Ctrl-C in terminal) exits cleanly. |
| Linux X11 (GNOME + `gnome-shell-extension-appindicator`) | Tray icon visible; menu opens. |
| Linux Wayland (GNOME 45+, KDE Plasma 6) | Tray icon visible; menu opens. **GNOME 42+ requires `gnome-shell-extension-appindicator` or `ayatana-indicator` to be installed** — documented in `htrcli/docs/tray.md`. |
| Headless Linux (sandbox, no `DISPLAY`) | One info log line, no tray, port still bound. |
| Windows | Tray icon in system tray; menu opens; Quit exits. |
| Wayland clipboard | `Copy bearer token` works (uses `wl-copy`, not `xclip`). |

### CI / headless safety

On macOS and Windows, `ShouldStart` returns `true` unconditionally. CI runners (GitHub Actions macOS, Windows Server Core) will attempt to start the tray and fail. The `--no-tray` flag and `HTRCLI_NO_TRAY=1` env var are the documented opt-out. **All CI invocations of `htrcli serve` MUST pass `--no-tray`.** The Makefile targets and `make htrcli-build` script (if any invokes `serve`) must be updated.

### Test commands

```bash
cd htrcli
go test ./internal/tray/...                  # always-run unit tests
go test -tags=traytest ./internal/tray/...   # full-stack menu simulation
go test ./...                                # no regressions in host, commands, etc.
```

## Documentation updates

| Doc | Change |
|---|---|
| `htrcli/docs/tray.md` | **New.** User-facing reference: what it is, headless behavior, opt-out, menu reference, troubleshooting, per-platform notes. |
| `htrcli/README.md` | Add a "Tray icon" subsection under "Daemon" with a 3-line summary + link to `tray.md`. |
| `htrcli/SPEC_HTRCLI.md` | Add a 30-line "Tray icon" section at the end, linking to this design doc. |
| `CHANGELOG.md` | Single user-visible entry under the next unreleased version. |
| `skills/htrcli/SKILL.md` | One paragraph under "Setup" noting the auto-attached tray and headless no-op. |

No update to `AGENTS.md`, `CLAUDE.md`, `firefox/README.md`, or `GUIDE.md` (unrelated).

## Open questions

- **Q (deferred):** Should `RecentLog(n)` be exposed as a submenu (last 5 / last 20 / tail -f) or just an "open file" item? Current design is "open file"; submenu is a v2 addition.
- **Q (deferred):** Auto-start at login. Out of scope for v1; could be a future `htrcli tray --install-autostart` that writes a LaunchAgent (macOS), .desktop file (Linux), or Run key (Windows).
- **Q (deferred):** Notifications on relay connect/disconnect. Would require hooking the daemon's `AddConn` / `RemoveConn`. Useful but additive; not in v1.

## Changelog entry

> **Tray icon (desktop)**: `htrcli serve` now shows a cross-platform system-tray icon on macOS, Windows, and Linux desktops. Menu provides live status (port, relay count, last error) and maintenance actions: reinstall native host (Chrome/Firefox), open config folder, copy bearer token (with 30s auto-clear), show recent log, and Restart/Quit lifecycle. The main goroutine now drives the tray; the HTTP server and signal handler run as goroutines behind it. On headless Linux servers the tray is silently skipped; opt out with `--no-tray` or `HTRCLI_NO_TRAY=1` (CI must use this). Bearer token is now logged as a fingerprint (`a1b2…f3e4`) instead of the full value. Requires `gnome-shell-extension-appindicator` or `ayatana-indicator` on GNOME/Wayland for the icon to appear.
