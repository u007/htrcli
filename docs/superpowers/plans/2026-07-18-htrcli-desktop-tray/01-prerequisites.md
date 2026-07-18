# Part 1: Prerequisites

Three small, additive upstream changes to existing code. They unblock the tray feature and can ship in the same PR.

---

### Task 1: `htrcli config set-extension-id` subcommand

The "Reinstall native host" tray submenu reads the extension ID from viper config. Today `configData` has no `ExtensionID` field and no subcommand writes one. Add both.

**Files:**
- Modify: `htrcli/internal/commands/config.go` (`configData` struct, `setConfigValue` switch, new `setExtensionIDCmd`, register in `init()`)
- Test: `htrcli/internal/commands/config_test.go` (create if absent; add cases)

**Interfaces:**
- Produces: `htrcli config set-extension-id <id> [--browser chrome|firefox]`. Stores the ID in `~/.htrcli/config.json` under the `extension-id` key (per-browser map if `--browser` is set; default applies to all). `viper.GetString("extension-id")` returns the value, or `viper.GetStringMapString("extension-id")` for per-browser lookup.

- [ ] **Step 1: Write the failing test**

In `htrcli/internal/commands/config_test.go`:

```go
func TestSetExtensionID(t *testing.T) {
    // Use a temp HOME so we don't clobber the real config.
    tmpHome := t.TempDir()
    t.Setenv("HOME", tmpHome)
    t.Setenv("XDG_CONFIG_HOME", tmpHome)
    viper.Reset()

    // Default browser = chrome
    setExtensionIDCmd.SetArgs([]string{"abc123def456"})
    if err := setExtensionIDCmd.Execute(); err != nil {
        t.Fatalf("set-extension-id: %v", err)
    }
    if got := viper.GetString("extension-id"); got != "abc123def456" {
        t.Fatalf("got %q", got)
    }
}

func TestSetExtensionIDPerBrowser(t *testing.T) {
    tmpHome := t.TempDir()
    t.Setenv("HOME", tmpHome)
    t.Setenv("XDG_CONFIG_HOME", tmpHome)
    viper.Reset()

    setExtensionIDCmd.SetArgs([]string{"ff-id-xyz", "--browser", "firefox"})
    if err := setExtensionIDCmd.Execute(); err != nil {
        t.Fatalf("set-extension-id: %v", err)
    }
    if got := viper.GetString("extension-id.firefox"); got != "ff-id-xyz" {
        t.Fatalf("got %q", got)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd htrcli && go test ./internal/commands/ -run TestSetExtensionID -v
```

Expected: compile error — `setExtensionIDCmd` undefined.

- [ ] **Step 3: Implement `setExtensionIDCmd`**

In `htrcli/internal/commands/config.go`, after the existing `setConfigValue` switch and the other `set*Cmd` blocks:

```go
var setExtensionIDBrowser string

var setExtensionIDCmd = &cobra.Command{
    Use:   "set-extension-id <id>",
    Short: "Set the browser extension ID used by 'htrcli install' (and the tray's 'Reinstall native host' menu)",
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        id := args[0]
        if setExtensionIDBrowser == "" {
            return viper.WriteConfig() // ...actually set then save
        }
        m := viper.GetStringMapString("extension-id")
        m[setExtensionIDBrowser] = id
        viper.Set("extension-id", m)
        return viper.WriteConfig()
    },
}

func init() {
    setExtensionIDCmd.Flags().StringVar(&setExtensionIDBrowser, "browser", "", "Browser this ID applies to (chrome, firefox). Empty = default for all.")
    configCmd.AddCommand(setExtensionIDCmd)
}
```

(The `viper.WriteConfig()` pattern matches whatever the existing `set*Cmd` siblings use; copy that exactly so the file write semantics — atomic write, mkdir-p, etc. — are consistent. If the existing code uses a helper, call it.)

- [ ] **Step 4: Run test to verify it passes**

```bash
cd htrcli && go test ./internal/commands/ -run TestSetExtensionID -v
```

Expected: PASS.

- [ ] **Step 5: Add a `htrcli config show` field for the new value**

In the `configShowCmd` RunE, add a line that prints the current `extension-id`. Match the formatting of the surrounding lines.

- [ ] **Step 6: Commit**

```bash
cd htrcli && git add internal/commands/config.go internal/commands/config_test.go
git commit -m "feat(htrcli): config set-extension-id for native host install

Adds the extension-id field to configData and a set-extension-id
subcommand. The tray's 'Reinstall native host' menu reads from this
field; without it, ReinstallHost cannot function.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: `host.StartUnixSocketServer` returns its `net.Listener`

Today the Unix-socket server discards its listener and has no stop mechanism. The tray's clean-shutdown sequence needs to close that listener. Change the signature to return it.

**Files:**
- Modify: `htrcli/internal/host/bridge.go` (`StartUnixSocketServer` signature and body)
- Test: `htrcli/internal/host/bridge_test.go` (update existing callers + add a stop test)

**Interfaces:**
- Produces: `StartUnixSocketServer(d *Daemon, path string) (net.Listener, error)` — returns the bound listener so the caller can `Close()` it for shutdown. The accept loop continues to run in a goroutine; closing the listener makes the next `Accept` return an error and the goroutine exits.

- [ ] **Step 1: Find all current callers**

```bash
cd htrcli && grep -rn "StartUnixSocketServer" --include="*.go"
```

Expected hits: `internal/commands/serve.go`, possibly `internal/host/bridge_test.go`.

- [ ] **Step 2: Update the signature and body**

In `htrcli/internal/host/bridge.go`:

```go
// StartUnixSocketServer starts the Unix-socket relay server. It returns
// the bound listener so the caller can Close() it for clean shutdown.
// On error, returns the listener (so partial state can still be cleaned)
// and a non-nil error.
func StartUnixSocketServer(d *Daemon, path string) (net.Listener, error) {
    ln, err := net.Listen("unix", path)
    if err != nil {
        return nil, fmt.Errorf("listen %s: %w", path, err)
    }
    go func() {
        for {
            conn, err := ln.Accept()
            if err != nil {
                // ln.Close() unblocks Accept with an error; exit cleanly.
                return
            }
            go host.handleConn(d, conn)  // adjust to the actual function name
        }
    }()
    return ln, nil
}
```

(Use the actual handler function name from bridge.go — likely `handleRelayConn` or `handleConn`. The signature and error-handling pattern is what matters; the inner detail is mechanical.)

- [ ] **Step 3: Update callers**

In `htrcli/internal/commands/serve.go`:

```go
unixLn, err := host.StartUnixSocketServer(d, socketPath)
if err != nil {
    log.Printf("[htrcli serve] Unix socket error: %v", err)
}
// Store unixLn for use in performShutdown (added in Part 4 Task 11).
```

(For now, the existing `defer ln.Close()` and goroutine log are replaced by this; the explicit-close behavior moves to `performShutdown` in Part 4.)

- [ ] **Step 4: Add a stop test**

In `htrcli/internal/host/bridge_test.go` (create if absent):

```go
func TestStartUnixSocketServerReturnsListener(t *testing.T) {
    d := NewDaemon()
    tmpDir := t.TempDir()
    sockPath := filepath.Join(tmpDir, "test.sock")
    ln, err := StartUnixSocketServer(d, sockPath)
    if err != nil {
        t.Fatalf("StartUnixSocketServer: %v", err)
    }
    if ln == nil {
        t.Fatal("expected non-nil listener")
    }
    if err := ln.Close(); err != nil {
        t.Fatalf("close listener: %v", err)
    }
    // The accept goroutine should have exited; no way to assert directly
    // without instrumenting, but Close() returning without hanging is enough.
}
```

- [ ] **Step 5: Run tests**

```bash
cd htrcli && go test ./internal/host/ -v
```

Expected: PASS, including the new test. If other bridge tests fail due to the signature change, update their call sites in the same step.

- [ ] **Step 6: Commit**

```bash
cd htrcli && git add internal/host/
git commit -m "refactor(htrcli): StartUnixSocketServer returns its listener

Required for the tray's clean-shutdown sequence: performShutdown
needs to close the Unix-socket listener to unblock the accept loop.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: `serve.go:72` prints token fingerprint, not full token

The tray's `MultiWriter` log redirection only captures `log.*` output, not `fmt.Printf`. The current line also writes the full token to journald/launchd. Switch to `log.Printf` and print only the fingerprint.

**Files:**
- Modify: `htrcli/internal/commands/serve.go` (line 72, plus a small `fingerprint` helper)
- Test: `htrcli/internal/commands/serve_test.go` (add a fingerprint test)

**Interfaces:**
- Produces: a private helper `fingerprint(token string) string` that returns `first4…last4` (or `—` for empty or tokens shorter than 8 chars). The startup line becomes `log.Printf("[htrcli serve] Bearer token fingerprint: %s", fingerprint(bearerToken))`.

- [ ] **Step 1: Write the failing test**

In `htrcli/internal/commands/serve_test.go` (create if absent):

```go
package commands

import "testing"

func TestFingerprint(t *testing.T) {
    tests := []struct {
        in   string
        want string
    }{
        {"", "—"},
        {"short", "—"},
        {"abcd1234efgh5678", "abcd…5678"},
        {"abcdefghijklmnop", "abcd…mnop"},
    }
    for _, tt := range tests {
        if got := fingerprint(tt.in); got != tt.want {
            t.Errorf("fingerprint(%q) = %q, want %q", tt.in, got, tt.want)
        }
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd htrcli && go test ./internal/commands/ -run TestFingerprint -v
```

Expected: compile error — `fingerprint` undefined.

- [ ] **Step 3: Implement `fingerprint` and update serve.go:72**

In `htrcli/internal/commands/serve.go`, add the helper near the top of the file (or in a small new `internal/commands/print.go` if you prefer):

```go
import "github.com/u007/htrcli/internal/output"  // if such a helper package exists, prefer that; otherwise local.

func fingerprint(token string) string {
    if len(token) < 8 {
        return "—"
    }
    return token[:4] + "…" + token[len(token)-4:]
}
```

Replace the line at the current 72:

```go
// before:
fmt.Printf("[htrcli serve] Using bearer token: %s\n", bearerToken)
// after:
log.Printf("[htrcli serve] Bearer token fingerprint: %s", fingerprint(bearerToken))
```

(If `fingerprint` already exists elsewhere, reuse it instead of defining twice.)

- [ ] **Step 4: Run test to verify it passes**

```bash
cd htrcli && go test ./internal/commands/ -run TestFingerprint -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd htrcli && git add internal/commands/serve.go internal/commands/serve_test.go
git commit -m "fix(htrcli): log bearer token fingerprint, not full token

serve.go was printing the full bearer token to stdout via fmt.Printf,
exposing it to journald/launchd/Console.app. Switch to log.Printf and
print only the first4…last4 fingerprint. Required for the tray's
'Show recent log' menu (the MultiWriter only captures log.* output).

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Part 1 complete when:

- `cd htrcli && go test ./...` passes.
- `htrcli config set-extension-id` works and persists.
- `host.StartUnixSocketServer` returns its listener.
- `serve.go` startup output shows the fingerprint, not the full token.

Proceed to Part 2 only when all three tasks are committed and the full test suite is green.
