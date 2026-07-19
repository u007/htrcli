# htrcli tray — manual smoke test results

The tray feature is cross-platform GUI code. Automated tests cover the
detection logic, menu build/dispatch, controller actions, and shutdown
sequencing headlessly (see `internal/tray/*_test.go` and
`internal/host/daemon_test.go`). The interactive behaviors below can only
be confirmed on a real desktop and should be run on each target OS before
a release.

## Verified programmatically (this implementation)

| Check | Result | How |
|---|---|---|
| `go build ./...` (darwin) | PASS | CGo link of `getlantern/systray` succeeds |
| `go test ./...` | PASS | detect, config, daemon, host, tray (real backend) suites green |
| `go test -tags=traytest ./internal/tray/` | PASS | menu build + click dispatch + refresh loop driven by `fakeBackend` |
| `ShouldStart` table (12 cases) | PASS | macOS/Windows always on; Linux display+no-SSH; opt-outs |
| `set-extension-id` (default + per-browser) | PASS | config written, viper reflects value |
| Bearer token fingerprint on startup | PASS | `Using bearer token: supe…3456` (no full token) |
| Headless skip | PASS | `htrcli serve --no-tray` + `HTRCLI_NO_TRAY=1` print "Tray disabled …" and serve normally |
| `Restart` strips secret flags | PASS | `stripSecrets` unit test |
| `ReinstallHost` builds install args | PASS | `fakeCommander` records `install --browser <b> --extension-id <id>` |

## Pending — interactive GUI smoke (run on real desktops)

These require a human at a desktop and were NOT executed in CI/headless.
Run `make htrcli-build && ./bin/htrcli serve` and verify:

### macOS (menu bar)
- [ ] Icon appears in the menu bar.
- [ ] Menu opens; "Status: 3845 · N relays · ok" shown.
- [ ] Maintenance → Open config folder opens `~/.htrcli/` in Finder.
- [ ] Show recent log opens `~/.htrcli/serve.log`.
- [ ] Copy bearer token places the token on the clipboard; 30s later it is
      `<cleared by htrcli>`.
- [ ] Quit exits cleanly (`ps aux | grep htrcli`).
- [ ] `kill -TERM <pid>` exits cleanly (no zombie/error).

### Linux X11 (e.g. KDE)
- [ ] Icon appears in the system tray.
- [ ] Reinstall native host is disabled until an ext-id is configured.
- [ ] Copy bearer token uses `xclip` (`xclip -o | head -c 8`).
- [ ] Quit exits cleanly.

### Linux Wayland (GNOME 45+)
- [ ] Icon appears **only** if `gnome-shell-extension-appindicator` is
      installed; otherwise document the failure.
- [ ] Copy bearer token uses `wl-copy` (`wl-paste | head -c 8`).

### Windows
- [ ] Icon appears in the notification area.
- [ ] Menu items work; Copy bearer token uses `clip.exe`.
- [ ] Quit exits cleanly.

### Headless Linux (VM/container)
- [ ] One info line: `Tray disabled (no display or HTRCLI_NO_TRAY set)`.
- [ ] No error; port :3845 bound (`ss -lnt | grep 3845`).

## Notes
- The tray depends on CGo (`getlantern/systray`). Cross-compiling for
  linux/windows from macOS requires the matching C toolchain; the Go source
  compiles and the unit tests pass on the host platform.
- `make serve` intentionally keeps the tray enabled (developer desktop use);
  `--no-tray` is reserved for CI/systemd/Docker.
