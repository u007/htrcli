# Part 4: Maintenance Actions and Shutdown

`daemonController` is the real implementation of the `Controller` interface. It wraps `*host.Daemon` and adds the platform-specific shell-out actions (ReinstallHost, OpenConfigFolder, OpenLog, CopyTokenToClipboard). Restart and Quit thread through to the process lifecycle.

---

### Task 10: `daemonController` — Restart, Quit, and platform shell-outs

**Files:**
- Create: `htrcli/internal/tray/daemon_controller.go` (real implementation)
- Create: `htrcli/internal/tray/daemon_controller_test.go` (uses fakeCommander)

**Interfaces:**
- Produces: `func newDaemonController(d *host.Daemon, port int, getToken func() string, getExtID func(browser string) string, selfPath string, httpLn net.Listener, cmd Commander) Controller` — the real `Controller`. All maintenance actions use `cmd` (a `Commander` interface) instead of `os/exec` directly so tests can verify which binary was chosen.
- Produces: `func (c *daemonController) Restart() error` — closes the HTTP listener, strips secret flags, re-execs `selfPath serve <args>`, then `os.Exit(0)`.
- Produces: `func (c *daemonController) Quit() error` — calls a registered `quitFn` (set by `serve.go` to its `performShutdown`).
- Produces: `func (c *daemonController) ReinstallHost(browser string) error` — looks up ext-ID via `getExtID(browser)`, runs `cmd.Run(selfPath, "install", "--browser", browser, "--extension-id", id)`.
- Produces: `func (c *daemonController) OpenConfigFolder() error` — opens `~/.htrcli/` via `open` / `explorer` / `xdg-open`.
- Produces: `func (c *daemonController) OpenLog() error` — opens `~/.htrcli/serve.log` in the OS default app.
- Produces: `func (c *daemonController) CopyTokenToClipboard() (string, error)` — `pbcopy` / `clip.exe` / `wl-copy` or `xclip`. Spawns a 30s goroutine to clear the clipboard.

- [ ] **Step 1: Define the secret-flag strip list**

In `daemon_controller.go`:

```go
// secretFlags are stripped from the re-exec argv by Restart to avoid
// leaking the bearer token to /proc/<pid>/cmdline. The restarted
// process re-resolves the token from $HTR_BEARER_TOKEN (inherited
// from the parent) or the config file.
//
// Add new secret-carrying flags here as they're introduced.
var secretFlags = map[string]bool{
    "--token":     true,
    "--bearer":    true,
    "--password":  true, // hypothetical; included for forward-compat
    "--api-key":   true,
}

func stripSecrets(args []string) []string {
    out := args[:0]
    skip := false
    for _, a := range args {
        if skip {
            skip = false
            continue
        }
        if secretFlags[a] {
            skip = true
            continue
        }
        if strings.HasPrefix(a, "--token=") || strings.HasPrefix(a, "--bearer=") {
            continue
        }
        out = append(out, a)
    }
    return out
}
```

- [ ] **Step 2: Implement `Restart`**

```go
func (c *daemonController) Restart() error {
    // 1. Close the HTTP listener to free port 3845 BEFORE re-exec.
    //    Otherwise the new process fails with "port already in use".
    if c.httpLn != nil { c.httpLn.Close() }

    // 2. Capture and filter argv.
    raw := os.Args[1:]
    safe := stripSecrets(raw)

    // 3. Re-exec.
    cmd := exec.Command(c.selfPath, append([]string{"serve"}, safe...)...)
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    cmd.Stdin = os.Stdin
    if err := cmd.Start(); err != nil {
        return fmt.Errorf("re-exec: %w", err)
    }
    // 4. Exit the parent. Skips deferred functions in the parent;
    //    that's intentional — the child has the port, the child is
    //    the new daemon.
    os.Exit(0)
    return nil // unreachable
}
```

- [ ] **Step 3: Implement `Quit`**

```go
type quitFn func()

func (c *daemonController) SetQuitFn(fn quitFn) { c.quit = fn }

func (c *daemonController) Quit() error {
    if c.quit != nil { c.quit() }
    return nil
}
```

`serve.go` registers the quit function during the wiring (in Part 3 Task 9 step 1 — add this there if not already):

```go
ctrl := newDaemonController(...)
ctrl.SetQuitFn(func() {
    // Triggers the same performShutdown as SIGINT/SIGTERM.
    sigCh <- syscall.SIGTERM
})
```

(Sending SIGTERM to ourselves re-uses the existing shutdown path. The signal handler goroutine is already in place from Part 3.)

- [ ] **Step 4: Implement `ReinstallHost`**

```go
func (c *daemonController) ReinstallHost(browser string) error {
    id := c.getExtID(browser)
    if id == "" {
        return fmt.Errorf("no extension ID configured for %s — run `htrcli config set-extension-id <id> --browser %s` first", browser, browser)
    }
    return c.cmd.Run(c.selfPath, "install", "--browser", browser, "--extension-id", id)
}
```

- [ ] **Step 5: Implement `OpenConfigFolder`**

```go
func (c *daemonController) OpenConfigFolder() error {
    home, err := os.UserHomeDir()
    if err != nil { return err }
    dir := filepath.Join(home, ".htrcli")
    if _, err := os.Stat(dir); err != nil {
        return fmt.Errorf("config folder not found at %s", dir)
    }
    return openViaOS(dir)
}

func openViaOS(path string) error {
    var name string
    var args []string
    switch runtime.GOOS {
    case "darwin":
        name, args = "open", []string{path}
    case "windows":
        name, args = "explorer", []string{path}
    default: // linux, *bsd
        name, args = "xdg-open", []string{path}
    }
    return exec.Command(name, args...).Start()  // don't wait; the GUI app is detached
}
```

- [ ] **Step 6: Implement `OpenLog`**

```go
func (c *daemonController) OpenLog() error {
    home, err := os.UserHomeDir()
    if err != nil { return err }
    path := filepath.Join(home, ".htrcli", "serve.log")
    if _, err := os.Stat(path); err != nil {
        return fmt.Errorf("no log yet at %s (run htrcli serve with a desktop attached to enable logging)", path)
    }
    return openViaOS(path)
}
```

- [ ] **Step 7: Implement `CopyTokenToClipboard` with platform glue and 30s auto-clear**

```go
func (c *daemonController) CopyTokenToClipboard() (string, error) {
    tok := c.getToken()
    if tok == "" {
        return "", fmt.Errorf("no bearer token set (run `htrcli config set-token <token>` or set HTR_BEARER_TOKEN)")
    }
    if err := copyToClipboard(tok); err != nil {
        return "", err
    }
    // Spawn a 30s goroutine to overwrite the clipboard with a sentinel.
    go func() {
        time.Sleep(30 * time.Second)
        _ = copyToClipboard("<cleared by htrcli>")
    }()
    return tok, nil
}

func copyToClipboard(text string) error {
    var name string
    var args []string
    switch runtime.GOOS {
    case "darwin":
        name, args = "pbcopy", nil
    case "windows":
        name, args = "clip.exe", nil
    default:
        if os.Getenv("WAYLAND_DISPLAY") != "" {
            name, args = "wl-copy", nil
        } else {
            name, args = "xclip", []string{"-selection", "clipboard"}
        }
    }
    cmd := exec.Command(name, args...)
    cmd.Stdin = strings.NewReader(text)
    return cmd.Run()
}
```

- [ ] **Step 8: Implement `IsRunning`, `Status`, `RecentLog`**

```go
func (c *daemonController) IsRunning() bool { return true }

func (c *daemonController) Status() Status {
    s := Status{
        Port:             c.port,
        RelaysConnected:  c.d.RelaysConnected(),
        LastError:        c.d.LastError(),
        TokenFingerprint: fingerprint(c.getToken()),
    }
    return s
}

func (c *daemonController) RecentLog(n int) []string {
    home, _ := os.UserHomeDir()
    path := filepath.Join(home, ".htrcli", "serve.log")
    return tailLines(path, n)
}
```

(`c.d.RelaysConnected()` and `c.d.LastError()` are new methods on `*host.Daemon` — add them if they don't exist; they are 1-2 line wrappers over the existing daemon state.)

- [ ] **Step 9: Add the real `os/exec` Commander**

```go
type realCommander struct{}

func (realCommander) Run(name string, args ...string) error {
    return exec.Command(name, args...).Run()
}
func (realCommander) Output(name string, args ...string) ([]byte, error) {
    return exec.Command(name, args...).Output()
}
```

- [ ] **Step 10: Write the platform-glue tests**

In `daemon_controller_test.go`:

```go
type fakeCommander struct {
    calls []fakeCmd
}

type fakeCmd struct {
    Name string
    Args []string
}

func (f *fakeCommander) Run(name string, args ...string) error {
    f.calls = append(f.calls, fakeCmd{name, args})
    return nil
}
func (f *fakeCommander) Output(name string, args ...string) ([]byte, error) {
    return nil, nil
}

func TestReinstallHost(t *testing.T) {
    d := &daemonController{
        selfPath: "/usr/bin/htrcli",
        getExtID: func(b string) string { return "my-ext-id" },
        cmd:      &fakeCommander{},
    }
    if err := d.ReinstallHost("chrome"); err != nil { t.Fatal(err) }
    fc := d.cmd.(*fakeCommander)
    if len(fc.calls) != 1 { t.Fatalf("calls: %v", fc.calls) }
    want := []string{"install", "--browser", "chrome", "--extension-id", "my-ext-id"}
    if !reflect.DeepEqual(fc.calls[0].Args, want) {
        t.Fatalf("got %v, want %v", fc.calls[0].Args, want)
    }
}

func TestReinstallHostMissingExtID(t *testing.T) {
    d := &daemonController{getExtID: func(b string) string { return "" }, cmd: &fakeCommander{}}
    if err := d.ReinstallHost("chrome"); err == nil {
        t.Fatal("want error for missing ext ID")
    }
}

func TestCopyTokenToClipboardDarwin(t *testing.T) {
    if runtime.GOOS != "darwin" { t.Skip("darwin only") }
    // ...similar
}

func TestStripSecrets(t *testing.T) {
    in := []string{"serve", "--token", "supersecret", "--port", "3845"}
    out := stripSecrets(in)
    want := []string{"serve", "--port", "3845"}
    if !reflect.DeepEqual(out, want) { t.Fatalf("got %v, want %v", out, want) }
}
```

(Adapt the platform tests to be `t.Skip()` on non-target platforms. The point is to assert the right binary is chosen.)

- [ ] **Step 11: Run tests**

```bash
cd htrcli && go test ./internal/tray/ -v
cd htrcli && go test -tags=traytest ./internal/tray/ -v
```

Expected: all green. Platform-glue tests skip on the wrong OS.

- [ ] **Step 12: Commit**

```bash
cd htrcli && git add internal/tray/daemon_controller.go internal/tray/daemon_controller_test.go
git commit -m "feat(htrcli): daemonController implementation

Real Controller implementation: Restart (with --token stripping and
port-bug fix), Quit, ReinstallHost, OpenConfigFolder, OpenLog,
CopyTokenToClipboard (with 30s auto-clear and Wayland wl-copy support).
Tests use fakeCommander to verify which binary is chosen without
actually spawning the process.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 11: Log file redirection and `performShutdown` finalization

When the tray attaches, redirect the daemon's `log.*` output to `~/.htrcli/serve.log` in addition to stderr. The `MultiWriter` keeps both, so systemd journal and launchd logs still get the lines. When the tray is NOT attached, this code path is not entered — server runs are completely unchanged.

**Files:**
- Modify: `htrcli/internal/commands/serve.go` (add `attachServeLog()`, call it when `trayAttached`)
- Modify: `htrcli/internal/commands/serve.go` (finalize `performShutdown`)

- [ ] **Step 1: Implement `attachServeLog`**

```go
func attachServeLog() (func() error, error) {
    home, err := os.UserHomeDir()
    if err != nil { return nil, err }
    dir := filepath.Join(home, ".htrcli")
    if err := os.MkdirAll(dir, 0755); err != nil { return nil, err }
    logPath := filepath.Join(dir, "serve.log")
    f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
    if err != nil { return nil, err }
    log.SetOutput(io.MultiWriter(os.Stderr, f))
    return f.Close, nil
}
```

- [ ] **Step 2: Call it from the wiring in `serveCmd.RunE`**

After `trayAttached := true` is set (in Part 3 Task 9 step 1):

```go
if trayAttached {
    icon, _ := iconFS.ReadFile("icon.png")
    if closeLog, err := attachServeLog(); err != nil {
        log.Printf("[htrcli serve] tray log redirect: %v (continuing without)", err)
    } else {
        // No defer here — performShutdown owns teardown.
        _ = closeLog
    }
    ctrl := newDaemonController(d, port, bearerToken, getToken, getExtID, selfPath, httpLn, realCommander{})
    ctrl.SetQuitFn(func() { sigCh <- syscall.SIGTERM })
    ...
}
```

(You'll need `getToken` and `getExtID` closures built earlier in `RunE` — they encapsulate the env → file → viper resolution chain.)

- [ ] **Step 3: Finalize `performShutdown`**

The Part 3 stub becomes:

```go
func performShutdown(trayAttached bool, srv *http.Server, d *host.Daemon, httpLn, unixLn net.Listener) {
    // 1. Quit the tray (idempotent). Routed through the tray package
    //    so serve.go doesn't import getlantern/systray.
    if trayAttached { tray.Quit() }

    // 2. Drain HTTP (5s timeout).
    shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    if err := srv.Shutdown(shutdownCtx); err != nil {
        log.Printf("[htrcli serve] HTTP shutdown: %v", err)
    }

    // 3. Stop the daemon.
    d.Stop()

    // 4. Close the Unix-socket listener.
    if unixLn != nil { unixLn.Close() }
}
```

Add `tray.Quit()` as a thin wrapper in the tray package (in `tray.go`):

```go
// Quit unblocks the systray.Run call in Run, allowing main to return.
// Idempotent.
func Quit() { systray.Quit() }
```

- [ ] **Step 4: Add the `tailLines` helper (used by `RecentLog`)**

In `internal/tray/daemon_controller.go` (or a new `tail.go`):

```go
func tailLines(path string, n int) []string {
    f, err := os.Open(path)
    if err != nil { return nil }
    defer f.Close()
    // Slurp the whole file (small; no rotation in v1).
    data, err := io.ReadAll(f)
    if err != nil { return nil }
    lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
    if len(lines) > n { lines = lines[len(lines)-n:] }
    return lines
}
```

- [ ] **Step 5: Run tests**

```bash
cd htrcli && go test ./...
```

Expected: all green.

- [ ] **Step 6: Commit**

```bash
cd htrcli && git add internal/commands/serve.go internal/tray/
git commit -m "feat(htrcli): log redirect to ~/.htrcli/serve.log when tray attached

MultiWriter preserves stderr for journald/launchd. Server runs
unaffected. performShutdown finalizes the explicit sequence.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 12: New `*host.Daemon` methods (`RelaysConnected`, `LastError`)

The `Status()` method on `daemonController` reads relay count and last error from the daemon. If those getters don't exist on `*host.Daemon`, add them.

**Files:**
- Modify: `htrcli/internal/host/daemon.go` (add `RelaysConnected() int` and `LastError() string`)
- Test: `htrcli/internal/host/daemon_test.go` (add cases)

- [ ] **Step 1: Add `RelaysConnected`**

```go
func (d *Daemon) RelaysConnected() int {
    d.mu.Lock()
    defer d.mu.Unlock()
    return len(d.conns)
}
```

- [ ] **Step 2: Add `LastError`**

`LastError` is a new field; the daemon sets it when:
- A command result returns `Success: false` (from `ResolveCommand`).
- A relay connection is dropped unexpectedly (from `RemoveConn`).
- A screenshot capture fails (from `ResolveScreenshot`).

```go
type Daemon struct {
    // ...existing fields...
    lastErr   string
    lastErrAt time.Time
}

func (d *Daemon) LastError() string {
    d.mu.Lock()
    defer d.mu.Unlock()
    if time.Since(d.lastErrAt) > 5*time.Minute { return "" } // 5-min half-life
    return d.lastErr
}
```

(`5*time.Minute` is a heuristic; the spec says "last error" with a 5-min staleness so the menu doesn't show a 3-day-old transient error forever. Adjust if feedback disagrees.)

- [ ] **Step 3: Wire the writers**

In the existing `ResolveCommand`, `RemoveConn`, and `ResolveScreenshot`:

```go
func (d *Daemon) ResolveCommand(commandID string, result CommandResult) {
    d.mu.Lock()
    if !result.Success {
        d.lastErr = result.Error
        d.lastErrAt = time.Now()
    }
    d.mu.Unlock()
    // ...existing body...
}
```

(Same pattern in the other two methods.)

- [ ] **Step 4: Run tests**

```bash
cd htrcli && go test ./internal/host/ -v
```

- [ ] **Step 5: Commit**

```bash
cd htrcli && git add internal/host/daemon.go internal/host/daemon_test.go
git commit -m "feat(htrcli): Daemon.RelaysConnected and LastError for tray status

LastError has a 5-minute half-life so the menu doesn't show ancient
transient errors. Reset on the next successful operation.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Part 4 complete when:

- `cd htrcli && go test ./...` passes.
- `cd htrcli && go test -tags=traytest ./internal/tray/ -v` passes.
- `daemonController` correctly routes ReinstallHost, OpenConfigFolder, OpenLog, CopyTokenToClipboard, Restart, Quit.
- `stripSecrets` removes `--token` and similar flags.
- `~/.htrcli/serve.log` is created when the tray attaches; nothing changes on headless.
- `performShutdown` runs in the right order with no `defer` for tray teardown.

Proceed to Part 5 only when all three tasks are committed and the test suite is green.
