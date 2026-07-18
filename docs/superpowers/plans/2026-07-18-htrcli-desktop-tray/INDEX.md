# htrcli Desktop Tray — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Spec:** `docs/superpowers/specs/2026-07-18-htrcli-desktop-tray-design.md`

**Goal:** Add a cross-platform system-tray icon to `htrcli serve` that auto-attaches on macOS, Windows, and Linux desktops, and is silently skipped on headless Linux servers. Exposes live status and maintenance actions (reinstall native host, open config folder, copy bearer token, show recent log, restart, quit).

**Architecture:** New Go package `htrcli/internal/tray` owns the menu and click dispatch. The main goroutine drives `systray.Run` (per `getlantern/systray` requirement); the HTTP server, signal handler, sweeper, refresh loop, and click dispatch run as background goroutines. The `Controller` interface is the seam between the tray and `*host.Daemon`; tests drive the menu via `fakeController` and (under `traytest` build tag) a `fakeSystray`.

**Tech Stack:** Go 1.22, cobra + viper (existing), `github.com/getlantern/systray` v1.2.2 (new dep, CGo), Go `testing`, no JS/TS changes.

## Global Constraints

- **Main goroutine owns the tray.** `systray.Run` MUST run on the main thread (its package `init()` calls `runtime.LockOSThread()`). Calling it from a non-main goroutine breaks on macOS.
- **Shutdown is explicit, not deferred.** A signal-handler goroutine calls `performShutdown()` which sequences `systray.Quit() → srv.Shutdown(5s) → d.Stop() → unixLn.Close()`. No `defer` for tray teardown.
- **Headless safety:** `ShouldStart()` returns false on Linux when `DISPLAY` and `WAYLAND_DISPLAY` are both empty OR `SSH_CONNECTION`/`SSH_TTY` is set. Always silent-skip with one info log line; never exit non-zero.
- **CI safety:** all CI invocations of `htrcli serve` MUST pass `--no-tray`. macOS/Windows `ShouldStart` always returns `true`.
- **Token handling:** never log the full bearer token to stdout or to a file. Print only the fingerprint (`a1b2…f3e4` or `—`).
- **Config path:** `~/.htrcli/` is the htrcli config and runtime dir (per `root.go:64` `viper.AddConfigPath(home + "/.htrcli")` and `config.go:92`). NOT `~/.htrcontrol/` (that is a legacy token-file fallback only).
- **Restart re-exec:** strip `--token` and any other secret-carrying flag from the re-exec argv to avoid leaking the token to `ps` / `/proc/*/cmdline`. Restart closes the HTTP listener before `exec.Command` to avoid the port-binding race.
- **Code style:** `gofmt` (tabs). The tray package lives in its own files; no `getlantern/systray` import in `serve.go` or any other package.
- **Commits** end with `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`.
- **Bun-only, never npm/yarn.** Go tests: `cd htrcli && go test ./...`.

## Execution Order

Parts must execute in order; tasks within a part are sequential.

| Part | File | Tasks | Delivers |
|---|---|---|---|
| 1 | `01-prerequisites.md` | 1–3 | `set-extension-id` config command, `StartUnixSocketServer` returns listener, fingerprint-only token logging |
| 2 | `02-detection-and-controller.md` | 4–6 | `tray.ShouldStart` (pure function), `tray.Controller` interface, `tray_test` build tag |
| 3 | `03-menu-and-dispatch.md` | 7–9 | `systray.Run` on main goroutine, menu tree, click dispatch, 5s refresh |
| 4 | `04-maintenance-and-shutdown.md` | 10–12 | `daemonController` implementation, `performShutdown`, log file redirection, restart-port-bugfix |
| 5 | `05-docs-and-ci.md` | 13–15 | `tray.md` user doc, README/SPEC/CHANGELOG/skill updates, CI opt-out enforcement |

## Task Table

1. Add `ExtensionID` to `configData` + `htrcli config set-extension-id` subcommand
2. `host.StartUnixSocketServer` returns its `net.Listener`
3. `serve.go:72` prints bearer token fingerprint, not full token
4. `internal/tray/detect.go` + `detect_test.go` (table-driven `ShouldStart`)
5. `internal/tray/controller.go` (Controller interface, Status, fakeController for tests)
6. Build-tag-gated `traytest` (fakeSystray shim, 50ms ticker override)
7. `internal/tray/tray.go` + `tray_disabled.go` (systray.Run on main goroutine, menu tree, click dispatch)
8. `internal/tray/tray.go` (refresh loop, status label updates, error flash logic)
9. Wire `tray.Run` into `serve.go` (inverted main, performShutdown, signal handler goroutine)
10. `internal/tray/controller.go` (daemonController implementation: Restart, Quit, ReinstallHost, OpenConfigFolder, OpenLog, CopyTokenToClipboard with platform glue)
11. `serve.go` (MultiWriter to `~/.htrcli/serve.log` when tray attached; performShutdown unixLn.Close)
12. Update `Makefile` / CI invocations to pass `--no-tray`; smoke test the headless case
13. New `htrcli/docs/tray.md` (user-facing reference)
14. Update `htrcli/README.md`, `htrcli/SPEC_HTRCLI.md`, `CHANGELOG.md`, `skills/htrcli/SKILL.md`
15. Manual smoke tests on macOS / Linux X11 / Linux Wayland / Windows / headless Linux
