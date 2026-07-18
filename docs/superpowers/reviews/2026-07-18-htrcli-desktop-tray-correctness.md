# Correctness Review: htrcli Desktop Tray Design

**Spec:** `docs/superpowers/specs/2026-07-18-htrcli-desktop-tray-design.md`
**Reviewer Dimension:** Correctness — does the design actually work?
**Reviewed Files:** `serve.go`, `daemon.go`, `install.go`, `relay.go`, `bridge.go`, `config.go`, `root.go`, `main.go`, `go.mod`, `getlantern/systray` v1.2.2 source

---

## Findings

### Critical (will not work as designed)

SEVERITY: error
FILE: docs/superpowers/specs/2026-07-18-htrcli-desktop-tray-design.md
LINE: 44 / 175-185
MESSAGE: **Goroutine inversion for systray.Run on macOS.** The `getlantern/systray` package calls `runtime.LockOSThread()` in its package `init()` (systray.go:27-29), and `systray.Run` → `nativeLoop()` enters the platform GUI event loop (NSApplication on macOS, AppIndicator/Gtk on Linux). The spec proposes calling `systray.Run` in a goroutine (line 180: `go systray.Run(onReady, icon)`) while the main goroutine runs `srv.Serve(ln)`. This is the inverse of the standard pattern — `getlantern/systray` is designed to be called from `main()`, where `init()` locks the main goroutine to the main thread. Launching it from a non-main goroutine means the AppKit run loop runs on a non-main thread on macOS, which is not the intended usage and is likely to fail (the NSApplication event loop expects to own the main thread). The library's own example (example/main.go) calls `systray.Run` from `main()`.
SUGGESTION: Restructure so the main goroutine calls `systray.Run` and `srv.Serve(ln)` runs in a goroutine. The tray's `refreshLoop` and `dispatchLoop` can run as additional goroutines spawned from `onReady`.

SEVERITY: error
FILE: docs/superpowers/specs/2026-07-18-htrcli-desktop-tray-design.md
LINE: 221-238
MESSAGE: **Signal handler cannot coexist with blocking srv.Serve.** The spec's signal-handling pattern (`<-sigCh` then defer chain runs on return) cannot be inserted into `serveCmd.RunE` as written because `srv.Serve(ln)` blocks the main goroutine. `srv.Serve` never returns until shutdown, so `<-sigCh` would never execute. The two must be in separate goroutines. Additionally, `serve.go` currently has NO signal handling at all (grep for `signal.Notify`, `SIGINT`, `SIGTERM` in serve.go returns zero results), so there is no "existing pattern" to add to. The spec's elaborate shutdown ordering (§221-231, tray first → HTTP → daemon) depends on this restructuring but does not describe it.
SUGGESTION: Explicitly spell out that `srv.Serve(ln)` must run in a goroutine, the main goroutine must block on signal, and the shutdown sequence (`srv.Shutdown(ctx)` → `tray.Stop()` → `d.Stop()`) must be explicitly sequenced (not deferred) so non-defer calls like `srv.Shutdown` can be interleaved correctly.

SEVERITY: error
FILE: docs/superpowers/specs/2026-07-18-htrcli-desktop-tray-design.md
LINE: 191
MESSAGE: **ReinstallHost references non-existent config and command.** `ReinstallHost` reads the extension ID from `~/.config/htrcontrol/config.json` (or `$XDG_CONFIG_HOME/htrcontrol/config.json`). No such config file exists. The actual htrcli config is at `~/.htrcli/config.json` (see `root.go:64`: `viper.AddConfigPath(home + "/.htrcli")`, and `config.go:92`: `configDir := filepath.Join(home, ".htrcli")`). Furthermore, the `configData` struct in `config.go:16-24` has **no ExtensionID field** — there is no `htrcli config set-extension-id` command (grep for `set-extension` across all of htrcli returns zero results). The spec invents a CLI command and a config file path that do not exist.
SUGGESTION: (a) Add `ExtensionID` to the `configData` struct. (b) Add a `htrcli config set-extension-id <id>` subcommand. (c) Have `ReinstallHost` read from `~/.htrcli/config.json` (via viper), not from `~/.config/htrcontrol/config.json`.

### High (will produce wrong behavior)

SEVERITY: error
FILE: docs/superpowers/specs/2026-07-18-htrcli-desktop-tray-design.md
LINE: 63 / 193
MESSAGE: **OpenConfigFolder opens the wrong directory.** `OpenConfigFolder` resolves the "same fallback chain as serve.go" (XDG → `~/.config/htrcontrol` → `~/.htrcontrol`). This is incorrect. The fallback chain in `serve.go:readBearerTokenFile()` was designed for the **legacy bearer token file** (Bun-server era compatibility), not for the config folder. The actual htrcli config directory is `~/.htrcli/` (root.go:64, config.go:92). Opening `~/.htrcontrol` would show the user a directory containing only an optional token file, not their actual config. Meanwhile the tray log lives at `~/.htrcli/serve.log` (spec line 211), creating a confusing split where the user sees two different directories for related data.
SUGGESTION: `OpenConfigFolder` should open `~/.htrcli/` (the actual config and runtime directory). The legacy `htrcontrol` paths are a token-file fallback only, not the config directory.

SEVERITY: error
FILE: docs/superpowers/specs/2026-07-18-htrcli-desktop-tray-design.md
LINE: 50-53, 63
MESSAGE: **Controller.Start/Stop not implementable with current Daemon API.** The `Controller` interface defines `IsRunning()`, `Start()`, `Stop()` as lifecycle methods for the daemon, but `*host.Daemon` does not support independent start/stop. `d.Stop()` (daemon.go:234-236) closes a `sync.Once` stop channel that only terminates the sweeper goroutine. There is no `d.Start()` method — `NewDaemon()` creates a fresh instance and the stop channel is one-shot. The tray's Stop/Start menu items imply the daemon can be suspended and resumed, but the architecture doesn't support this. Furthermore, if Stop stops the HTTP server, port 3845 is still bound; Start cannot re-bind it. If Stop stops the daemon but not the HTTP server, the refresh loop's HTTP requests to `Status()` will immediately fail, turning the tray into a dangling UI.
SUGGESTION: Either (a) remove Stop/Start from the tray menu and keep only Restart (which exits the process), or (b) redesign the Daemon type to support start/stop lifecycle properly, including port management.

SEVERITY: warning
FILE: docs/superpowers/specs/2026-07-18-htrcli-desktop-tray-design.md
LINE: 200-201
MESSAGE: **CopyTokenToClipboard uses xclip which fails on Wayland.** `CopyTokenToClipboard` uses `xclip -selection clipboard` on Linux (with `xsel` fallback). This works on X11 but not on Wayland without XWayland compatibility. Modern GNOME (45+) on Wayland typically does not have `xclip` working for clipboard access. The spec includes "Linux Wayland (GNOME 45+)" in manual smoke tests (line 311) but does not add a `wl-copy` fallback. This means copy-to-clipboard will silently fail or produce no output on Wayland-native desktops.
SUGGESTION: Add `wl-copy` (from `wl-clipboard`) as a Wayland-specific fallback by checking `$WAYLAND_DISPLAY` and trying `wl-copy` before `xclip`.

### Medium (structural issues)

SEVERITY: warning
FILE: docs/superpowers/specs/2026-07-18-htrcli-desktop-tray-design.md
LINE: 189
MESSAGE: **Restart has a port-binding race.** `Restart()` spawns a new process with `exec.Command(selfPath, "serve", args...)` then calls `os.Exit(0)`. The current process holds port 3845 via `net.Listen("tcp", srv.Addr)`. Between `cmd.Start()` and `os.Exit(0)`, the new process starts up and tries to bind port 3845, which is still held by the old process. This causes the new process to fail with "port already in use" — the exact condition that `serve.go:46-49` was written to detect and produce a clear error for. The race is narrow (process spawn is fast, `os.Exit(0)` is normally immediate) but real on loaded systems.
SUGGESTION: Close the listener (`ln.Close()`) before spawning the new process. Since `srv.Serve(ln)` blocks the goroutine, this requires tracking the listener separately and calling `ln.Close()` from the Restart code path.

SEVERITY: warning
FILE: docs/superpowers/specs/2026-07-18-htrcli-desktop-tray-design.md
LINE: 137 / 222-228
MESSAGE: **Unix socket server has no stop mechanism.** The Unix socket server (`StartUnixSocketServer`, bridge.go:17-44) is started in a goroutine (serve.go:52-56) with only `defer ln.Close()` when the goroutine exits. It blocks in `for { ln.Accept() }` and cannot be cleanly shut down. The spec's shutdown ordering (§222-228) claims to close the daemon (step 3) but `d.Stop()` (daemon.go:234) only stops the sweeper, not the Unix socket server. The spec's `defer d.Stop()` is already present in serve.go (line 64) but it does not stop the socket accept loop.
SUGGESTION: Make the listener accessible from outside the goroutine (e.g., store it on the Daemon struct), or pass a context/stop channel into `StartUnixSocketServer`, so the socket server can be cleanly shut down during Restart or process shutdown.

SEVERITY: info
FILE: docs/superpowers/specs/2026-07-18-htrcli-desktop-tray-design.md
LINE: 94-97
MESSAGE: **Start/Stop menu items imply an illusion of lifecycle.** Since `htrcli serve` IS the daemon process, "Stop" from the tray means stopping the daemon's services. But after Stop, the tray process continues running (it's the same process), the refresh loop tries to query the stopped daemon for `Status()` and fails, and "Start" doesn't know how to re-bind the port or restart the stopped goroutines. The menu items create an expectation that the architecture cannot fulfill.
SUGGESTION: Either remove Start/Stop and keep only Restart + Quit, or rename them to something accurate like "Suspend connections" / "Resume connections" and implement the underlying lifecycle changes in the Daemon.

### Low (minor gaps, omissions, and edge cases)

SEVERITY: info
FILE: docs/superpowers/specs/2026-07-18-htrcli-desktop-tray-design.md
LINE: 153
MESSAGE: The wiring snippet passes `ctx` to `tray.Start(ctx, ctrl, icon)`, but `serveCmd.RunE` has signature `func(cmd *cobra.Command, args []string) error` — there is no `ctx` parameter. The spec does not define where `ctx` comes from.
SUGGESTION: Add `ctx := context.Background()` or `ctx, cancel := context.WithCancel(context.Background())` near the top of the wiring block.

SEVERITY: info
FILE: docs/superpowers/specs/2026-07-18-htrcli-desktop-tray-design.md
LINE: 157
MESSAGE: The spec writes `ctrl := newDaemonController(d, port, getToken)` but `getToken` is not defined anywhere. The bearer token is resolved inline in serve.go (lines 26-33) and stored in the `bearerToken` local variable. The spec does not define what `getToken` is (a closure? a func reference?).
SUGGESTION: Define `getToken` explicitly — either `func() string { return bearerToken }` (if captured at construction) or a re-resolution function that checks env → file → viper on each call.

SEVERITY: info
FILE: docs/superpowers/specs/2026-07-18-htrcli-desktop-tray-design.md
LINE: 166-170
MESSAGE: The spec shows `serveCmd.Flags().BoolVar(&noTray, ...)` but then reads the flag with `cmd.Flags().GetBool("no-tray")`. `BoolVar` stores the value into a variable; `GetBool` reads from the parsed flags. If `noTray` is a package-level variable (not shown in the spec), `GetBool` is redundant. If it isn't, `BoolVar` won't compile. The spec is ambiguous about which pattern is intended.
SUGGESTION: Pick one pattern: either use `BoolVar` with the variable directly, or use `cmd.Flags().Bool()` (returns `*bool`) and read the return value.

SEVERITY: info
FILE: docs/superpowers/specs/2026-07-18-htrcli-desktop-tray-design.md
LINE: 77
MESSAGE: The `ShouldStart` detection always returns `true` on macOS and Windows. On CI runners (e.g., GitHub Actions macOS runner without a GUI, Windows Server Core), the tray will attempt to start and fail. The `--no-tray` / `HTRCLI_NO_TRAY=1` opt-out covers CI scripts, but the spec should document this limitation for macOS/Windows servers.
SUGGESTION: Add a note that macOS and Windows CI/headless environments must pass `--no-tray` or set `HTRCLI_NO_TRAY=1`.

SEVERITY: info
FILE: docs/superpowers/specs/2026-07-18-htrcli-desktop-tray-design.md
LINE: 310-311
MESSAGE: `getlantern/systray` on Wayland uses `libappindicator`/`ayatana-appindicator` via D-Bus, which is not natively supported by GNOME 42+. It requires the `gnome-shell-extension-appindicator` community extension (or `ayatana-indicator`). The spec's manual smoke tests mention GNOME 45+ Wayland but do not document this dependency.
SUGGESTION: Add a note that GNOME/Wayland users need `gnome-shell-extension-appindicator` or `ayatana-indicator` installed for the tray icon to appear.

SEVERITY: info
FILE: docs/superpowers/specs/2026-07-18-htrcli-desktop-tray-design.md
LINE: 0 (general)
MESSAGE: The spec introduces `getlantern/systray` as a new dependency (not currently in `go.mod`). The library v1.2.2 (latest) depends on `golang.org/x/sys v0.1.0`, which is much older than the project's existing `golang.org/x/sys v0.20.0` (from go.mod indirects via viper/cobra). This should resolve via Go's MVS (minimal version selection), but should be verified.
SUGGESTION: After `go get github.com/getlantern/systray`, run `go mod tidy` and verify no unexpected downgrades occur.

SEVERITY: info
FILE: docs/superpowers/specs/2026-07-18-htrcli-desktop-tray-design.md
LINE: 42-43 / 73
MESSAGE: The build-tag-gated stub `tray_disabled.go` (`//go:build !darwin && !linux && !windows`) would need to stub ALL exported functions from the tray package (`ShouldStart`, `Start`, `Stop`) to prevent imports of `getlantern/systray` on unsupported OSes. If `detect.go` is compiled (it has no build tag), it would pull in `runtime.GOOS` (fine) but not `systray`. But if `tray.go` is pulled in transitively via `serve.go`'s import of `internal/tray` on an unsupported OS, it would try to import `getlantern/systray` and fail on the C code. The stub must break the import chain at the package level.
SUGGESTION: Place build tags on ALL files in the tray package that import `getlantern/systray` (or that call `systray.*` functions), and have `tray_disabled.go` provide no-op stubs for every exported function. Alternatively, guard the import in `serve.go` with a build tag.

SEVERITY: info
FILE: docs/superpowers/specs/2026-07-18-htrcli-desktop-tray-design.md
LINE: 79-82
MESSAGE: `SSH_TTY` is only set for interactive SSH sessions, not for `ssh command` (non-interactive). `SSH_CONNECTION` is always set when SSH is active. The combination of both checks is fine — `SSH_CONNECTION` catches both cases. This is not a bug but `SSH_TTY` is redundant for correctness purposes.
SUGGESTION: (No action needed — redundant but not harmful.)

---

## Summary

| Severity | Count | Key theme |
|----------|-------|-----------|
| **error** | 5 | Goroutine inversion (macOS NSApplication), signal handler can't coexist with blocking Serve, non-existent config path + command in ReinstallHost, OpenConfigFolder opens wrong directory, Controller Start/Stop not implementable |
| **warning** | 3 | Restart port race, Unix socket server cannot be stopped, Start/Stop UX illusion |
| **info** | 7 | Undefined `ctx`, undefined `getToken`, redundant flag patterns, macOS/Windows CI gap, GNOME Wayland extension dependency, new dep version skew, build-tag stub completeness |

### Most impactful issues

1. **Goroutine inversion (error × 2, lines 44 + 232):** The spec has `systray.Run` in a goroutine and `srv.Serve` on main, but macOS + `getlantern/systray` require the opposite. This is a fundamental architecture problem — it affects the entire shutdown sequence and signal handling. Fixing it requires restructuring `serve.go` significantly.

2. **Config path confusion / missing command (error × 2, lines 191 + 193):** Two maintenance actions (`ReinstallHost` and `OpenConfigFolder`) reference the legacy `~/.htrcontrol/` token paths instead of the actual `~/.htrcli/` config directory. `ReinstallHost` also requires a `set-extension-id` command that does not exist. These are implementation blockers.

3. **Controller Start/Stop unworkable (error, line 63):** The tray menu promises Start/Stop lifecycle but the `Daemon` type doesn't support it. The menu must be redesigned or the Daemon must gain start/stop capability.
