# Part 1 — Browser Contexts Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add named, isolated browser contexts to htrcli: a global `--context <name>` flag that launches/reuses a separate Chrome process with its own `--user-data-dir` profile on its own debugging port, plus `htrcli context list`.

**Architecture:** A context is a separate Chrome process with an isolated profile dir (`~/.htrcli/contexts/<name>`) on its own debugging port, tracked in a `~/.htrcli/contexts.json` registry. This reuses the exact detached-process launch pattern `internal/cdp/launch.go` already uses for the single default profile — it is NOT `Target.createBrowserContext`, whose in-process context would die with htrcli's stateless one-shot CDP session (`internal/cdp/session.go` closes the socket after every command). The `--context` flag resolves (launching on first use) to a debugging port, which `GetCDPPort()` then returns so all existing CDP verbs operate against that context.

**Tech Stack:** Go (cobra CLI, stdlib `net`/`os`/`os/exec`, `syscall`), Go `testing` + `net/http/httptest`.

## Global Constraints

- Go module root: `htrcli/`. Run Go tests with `cd htrcli && go test ./...`.
- State files under `~/.htrcli/`, files mode `0600`, dirs `0700`, written with
  `json.MarshalIndent(..., "", "  ")` — matches `writeState` in `launch.go`.
- Detached child processes use `SysProcAttr{Setsid: true}` + a `cmd.Wait()` reaper
  goroutine — matches `StartBrowser`.
- Never add `--remote-debugging-address`; the debugging port stays localhost-bound.
- Every caught error logged with attempt + error, or carries an explicit
  `// intentionally not logged: <reason>` comment.
- `context list` output is sorted by context name (the registry is persisted
  pre-sorted).
- No silent fallback: a `--context` that fails to resolve aborts the command with
  an explicit error before any CDP verb runs — it must never silently target the
  default browser.

---

### Task 1: Context registry + profile/port helpers

**Files:**
- Create: `htrcli/internal/cdp/context.go`
- Test: `htrcli/internal/cdp/context_test.go`

**Interfaces:**
- Produces: `cdp.ContextEntry{Name string, ProfileDir string, Port int, PID int, CreatedAt time.Time}`, `cdp.ContextsFilePath() (string, error)`, `cdp.ContextProfileDir(name string) (string, error)`, `cdp.ReadContexts() ([]ContextEntry, error)`, `cdp.FindContext(name string) (*ContextEntry, error)`, and unexported `upsertContext(ContextEntry) error`, `freePort() (int, error)`.

- [ ] **Step 1: Write the failing test**

Create `htrcli/internal/cdp/context_test.go`:

```go
package cdp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUpsertAndReadContexts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if entries, err := ReadContexts(); err != nil || entries != nil {
		t.Fatalf("expected empty registry, got %v err %v", entries, err)
	}

	if err := upsertContext(ContextEntry{Name: "work", Port: 9333, PID: 111}); err != nil {
		t.Fatalf("upsert work: %v", err)
	}
	if err := upsertContext(ContextEntry{Name: "alpha", Port: 9444, PID: 222}); err != nil {
		t.Fatalf("upsert alpha: %v", err)
	}
	// Upsert must replace, not duplicate.
	if err := upsertContext(ContextEntry{Name: "work", Port: 9555, PID: 333}); err != nil {
		t.Fatalf("upsert work again: %v", err)
	}

	entries, err := ReadContexts()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d: %+v", len(entries), entries)
	}
	// Persisted sorted by name: alpha before work.
	if entries[0].Name != "alpha" || entries[1].Name != "work" {
		t.Fatalf("expected sorted [alpha, work], got %+v", entries)
	}
	if entries[1].Port != 9555 || entries[1].PID != 333 {
		t.Fatalf("expected work replaced (port 9555 pid 333), got %+v", entries[1])
	}

	got, err := FindContext("work")
	if err != nil || got == nil || got.Port != 9555 {
		t.Fatalf("FindContext(work) = %+v, err %v", got, err)
	}
	missing, err := FindContext("nope")
	if err != nil || missing != nil {
		t.Fatalf("FindContext(nope) = %+v, err %v", missing, err)
	}

	if _, err := os.Stat(filepath.Join(home, ".htrcli", "contexts.json")); err != nil {
		t.Fatalf("contexts.json not written: %v", err)
	}
}

func TestContextProfileDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir, err := ContextProfileDir("work")
	if err != nil {
		t.Fatalf("ContextProfileDir: %v", err)
	}
	want := filepath.Join(home, ".htrcli", "contexts", "work")
	if dir != want {
		t.Fatalf("expected %s, got %s", want, dir)
	}
}

func TestFreePort(t *testing.T) {
	p, err := freePort()
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	if p <= 0 || p > 65535 {
		t.Fatalf("freePort returned invalid port %d", p)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd htrcli && go test ./internal/cdp/ -run 'TestUpsertAndReadContexts|TestContextProfileDir|TestFreePort' -v`
Expected: FAIL — `ContextEntry`, `ReadContexts`, `upsertContext`, `FindContext`, `ContextProfileDir`, `freePort` undefined (build error).

- [ ] **Step 3: Write the implementation**

Create `htrcli/internal/cdp/context.go`:

```go
package cdp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// ContextEntry records one named browser context: an isolated Chrome profile
// launched as its own process on its own debugging port. The default (unnamed)
// context is NOT stored here — it stays on ProfileDir()/browser.json.
type ContextEntry struct {
	Name       string    `json:"name"`
	ProfileDir string    `json:"profile_dir"`
	Port       int       `json:"port"`
	PID        int       `json:"pid"`
	CreatedAt  time.Time `json:"created_at"`
}

// ContextsFilePath returns ~/.htrcli/contexts.json.
func ContextsFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	return filepath.Join(home, ".htrcli", "contexts.json"), nil
}

// ContextProfileDir returns ~/.htrcli/contexts/<name>.
func ContextProfileDir(name string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	return filepath.Join(home, ".htrcli", "contexts", name), nil
}

// ReadContexts returns the persisted registry (nil when the file is absent —
// an expected "no contexts yet" case).
func ReadContexts() ([]ContextEntry, error) {
	path, err := ContextsFilePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil // intentionally not logged: absent registry means "no contexts", an expected case
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var entries []ContextEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return entries, nil
}

// FindContext returns the entry for name (nil, nil when absent).
func FindContext(name string) (*ContextEntry, error) {
	entries, err := ReadContexts()
	if err != nil {
		return nil, err
	}
	for i := range entries {
		if entries[i].Name == name {
			return &entries[i], nil
		}
	}
	return nil, nil
}

// writeContexts persists the registry, sorted by name for a stable file and
// sorted `context list` output.
func writeContexts(entries []ContextEntry) error {
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	path, err := ContextsFilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling contexts: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// upsertContext inserts or replaces the entry for entry.Name.
func upsertContext(entry ContextEntry) error {
	entries, err := ReadContexts()
	if err != nil {
		return err
	}
	replaced := false
	for i := range entries {
		if entries[i].Name == entry.Name {
			entries[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		entries = append(entries, entry)
	}
	return writeContexts(entries)
}

// freePort asks the OS for an unused localhost TCP port by binding :0 and
// reading back the assigned port. There is an inherent TOCTOU gap between
// releasing this port and Chrome binding it; callers launch immediately after
// to minimize it, and PortAlive verification catches a lost race.
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("allocating free port: %w", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd htrcli && go test ./internal/cdp/ -run 'TestUpsertAndReadContexts|TestContextProfileDir|TestFreePort' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add htrcli/internal/cdp/context.go htrcli/internal/cdp/context_test.go
git commit -m "feat(htrcli): context registry + profile/port helpers"
```

---

### Task 2: Generalize Chrome launch + EnsureContext

**Files:**
- Modify: `htrcli/internal/cdp/launch.go`
- Modify: `htrcli/internal/cdp/context.go`
- Test: `htrcli/internal/cdp/context_test.go`

**Interfaces:**
- Consumes: `ContextEntry`, `FindContext`, `upsertContext`, `ContextProfileDir`, `freePort` (Task 1); `PortAlive`, `LaunchArgs`, `ProfileDir`, `BrowserState`, `ReadState`, `writeState` (existing `launch.go`).
- Produces: unexported `launchChrome(chromePath string, port int, profileDir string, headless bool) (int, error)` (returns child PID, or 0 when the port already answered); `cdp.EnsureContext(name, chromePath string, headless bool) (int, error)` (returns the context's debugging port, launching on first use).

- [ ] **Step 1: Write the failing test**

Append to `htrcli/internal/cdp/context_test.go`. This test fakes a "live" context port with an `httptest` server answering `/json/version` (exactly what `PortAlive`→`BrowserWSURL` probes), so `EnsureContext` short-circuits to the recorded port without launching Chrome:

```go
func TestEnsureContextReusesLivePort(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Fake a live CDP endpoint: /json/version with a webSocketDebuggerUrl.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/json/version" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"webSocketDebuggerUrl":"ws://127.0.0.1/devtools/browser/x"}`)
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	// httptest binds 127.0.0.1:<port>; extract the port.
	_, portStr, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("atoi: %v", err)
	}

	if err := upsertContext(ContextEntry{Name: "work", Port: port, PID: 4242, ProfileDir: "/tmp/x"}); err != nil {
		t.Fatalf("seed context: %v", err)
	}

	// chromePath "" is fine: a live port means EnsureContext never launches.
	got, err := EnsureContext("work", "", false)
	if err != nil {
		t.Fatalf("EnsureContext: %v", err)
	}
	if got != port {
		t.Fatalf("expected reused port %d, got %d", port, got)
	}
}

func TestEnsureContextRejectsEmptyName(t *testing.T) {
	if _, err := EnsureContext("", "", false); err == nil {
		t.Fatal("expected error for empty context name")
	}
}
```

Add these imports to the top `import (...)` block of `context_test.go` (alongside the existing `os`, `path/filepath`, `testing`):

```go
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd htrcli && go test ./internal/cdp/ -run 'TestEnsureContext' -v`
Expected: FAIL — `EnsureContext` undefined.

- [ ] **Step 3: Refactor launch.go to a profile-parameterized launcher**

In `htrcli/internal/cdp/launch.go`, replace the body of `StartBrowser` (the function currently spanning the `if PortAlive(port) {...}` block through the final `return nil, fmt.Errorf("Chrome (pid %d)...")`) with a thin wrapper over a new shared `launchChrome`. Replace this existing function:

```go
// StartBrowser launches Chrome detached, waits for the port, persists state.
// If the port already answers, it records/refreshes state and returns without
// launching (also covers Chrome's singleton-lock handoff to an existing
// profile owner).
func StartBrowser(chromePath string, port int, headless bool) (*BrowserState, error) {
	if PortAlive(port) {
		st, err := ReadState()
		if err != nil || st == nil {
			st = &BrowserState{Port: port, StartedAt: time.Now()}
		}
		return st, nil
	}
	profile, err := ProfileDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(profile, 0700); err != nil {
		return nil, fmt.Errorf("creating profile dir: %w", err)
	}

	cmd := exec.Command(chromePath, LaunchArgs(port, profile, headless)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // detach: survives htrcli exiting
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("launching Chrome %s: %w", chromePath, err)
	}
	// Reap when Chrome eventually exits so a stopped browser never zombies
	// against a still-running htrcli daemon process.
	go func() {
		if err := cmd.Wait(); err != nil {
			fmt.Fprintf(os.Stderr, "[htrcli] Chrome exited: %v\n", err)
		}
	}()

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if PortAlive(port) {
			st := &BrowserState{PID: cmd.Process.Pid, Port: port, StartedAt: time.Now(), Headless: headless}
			if err := writeState(st); err != nil {
				return nil, err
			}
			return st, nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return nil, fmt.Errorf("Chrome (pid %d) did not answer on port %d within 15s", cmd.Process.Pid, port)
}
```

with:

```go
// launchChrome starts Chrome detached on port with the given profile dir and
// waits for the debugging port to answer. It does NOT persist any state file —
// callers record the result where appropriate (browser.json vs contexts.json).
// If the port already answers it returns pid 0 (an already-running owner, e.g.
// Chrome's singleton-lock handoff).
func launchChrome(chromePath string, port int, profileDir string, headless bool) (int, error) {
	if PortAlive(port) {
		return 0, nil
	}
	if err := os.MkdirAll(profileDir, 0700); err != nil {
		return 0, fmt.Errorf("creating profile dir: %w", err)
	}
	cmd := exec.Command(chromePath, LaunchArgs(port, profileDir, headless)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // detach: survives htrcli exiting
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("launching Chrome %s: %w", chromePath, err)
	}
	// Reap when Chrome eventually exits so a stopped browser never zombies
	// against a still-running htrcli process.
	go func() {
		if err := cmd.Wait(); err != nil {
			fmt.Fprintf(os.Stderr, "[htrcli] Chrome exited: %v\n", err)
		}
	}()

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if PortAlive(port) {
			return cmd.Process.Pid, nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return 0, fmt.Errorf("Chrome (pid %d) did not answer on port %d within 15s", cmd.Process.Pid, port)
}

// StartBrowser launches the default-profile Chrome, waits for the port, and
// persists advisory state to browser.json.
func StartBrowser(chromePath string, port int, headless bool) (*BrowserState, error) {
	profile, err := ProfileDir()
	if err != nil {
		return nil, err
	}
	pid, err := launchChrome(chromePath, port, profile, headless)
	if err != nil {
		return nil, err
	}
	if pid == 0 {
		// Port already answered — record/refresh advisory state without relaunch.
		st, err := ReadState()
		if err != nil || st == nil {
			st = &BrowserState{Port: port, StartedAt: time.Now()}
		}
		return st, nil
	}
	st := &BrowserState{PID: pid, Port: port, StartedAt: time.Now(), Headless: headless}
	if err := writeState(st); err != nil {
		return nil, err
	}
	return st, nil
}
```

This is behavior-preserving for the default browser (same launch args, same state
file, same already-running short-circuit) and extracts the reusable launch core.
The `launch.go` import list is unchanged — `os`, `os/exec`, `syscall`, `time`,
`fmt` are all already imported.

- [ ] **Step 4: Add EnsureContext to context.go**

Append to `htrcli/internal/cdp/context.go`:

```go
// EnsureContext launches (or reuses) the named context's Chrome process and
// returns its debugging port. A context is an isolated --user-data-dir profile
// on its own port, launched as a separate process so it survives across CLI
// invocations and gives true cookie/storage isolation. Chrome-only: the Firefox
// equivalent (a separate `firefox -profile <dir>` process) is out of scope for
// this task and documented as the Firefox fallback in the spec.
func EnsureContext(name, chromePath string, headless bool) (int, error) {
	if name == "" {
		return 0, errors.New("context name must not be empty")
	}
	entry, err := FindContext(name)
	if err != nil {
		return 0, err
	}
	if entry != nil && PortAlive(entry.Port) {
		return entry.Port, nil
	}

	profileDir, err := ContextProfileDir(name)
	if err != nil {
		return 0, err
	}

	// Reuse the recorded (now-dead) port to keep the profile↔port mapping
	// stable across restarts; allocate a fresh one only when never launched.
	port := 0
	if entry != nil {
		port = entry.Port
	}
	if port == 0 {
		port, err = freePort()
		if err != nil {
			return 0, err
		}
	}

	pid, err := launchChrome(chromePath, port, profileDir, headless)
	if err != nil {
		return 0, err
	}
	createdAt := time.Now()
	if entry != nil {
		createdAt = entry.CreatedAt
	}
	if err := upsertContext(ContextEntry{
		Name:       name,
		ProfileDir: profileDir,
		Port:       port,
		PID:        pid,
		CreatedAt:  createdAt,
	}); err != nil {
		return 0, err
	}
	return port, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd htrcli && go test ./internal/cdp/ -run 'TestEnsureContext|TestUpsertAndReadContexts|TestContextProfileDir|TestFreePort|TestStartBrowser|TestLaunchArgs' -v`
Expected: PASS — including the pre-existing `launch.go` tests (`LaunchArgs`, any
`StartBrowser` test), proving the refactor preserved behavior.

- [ ] **Step 6: Run the full cdp package suite (regression guard)**

Run: `cd htrcli && go test ./internal/cdp/...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add htrcli/internal/cdp/launch.go htrcli/internal/cdp/context.go htrcli/internal/cdp/context_test.go
git commit -m "feat(htrcli): EnsureContext launches isolated per-context Chrome profile"
```

---

### Task 3: `--context` global flag + context-aware CDP port

**Files:**
- Modify: `htrcli/internal/commands/root.go`
- Test: `htrcli/internal/commands/root_test.go`

**Interfaces:**
- Consumes: `cdp.EnsureContext`, `cdp.FindChrome` (existing), `GetChromePath` (existing).
- Produces: package var `contextName string`; a `--context` persistent flag;
  `GetCDPPort()` returns the resolved context port when `--context` is set;
  `resolveContext() error` (called from `PersistentPreRunE`).

- [ ] **Step 1: Write the failing test**

Append to `htrcli/internal/commands/root_test.go`:

```go
func TestGetCDPPortDefault(t *testing.T) {
	contextName = ""
	resolvedContextPort = 0
	if got := GetCDPPort(); got != 9222 {
		t.Fatalf("expected default 9222, got %d", got)
	}
}

func TestGetCDPPortUsesResolvedContext(t *testing.T) {
	t.Cleanup(func() { contextName = ""; resolvedContextPort = 0 })
	contextName = "work"
	resolvedContextPort = 9412
	if got := GetCDPPort(); got != 9412 {
		t.Fatalf("expected resolved context port 9412, got %d", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd htrcli && go test ./internal/commands/ -run 'TestGetCDPPort' -v`
Expected: FAIL — `contextName` / `resolvedContextPort` undefined, or default test
fails because `GetCDPPort` doesn't yet honor `resolvedContextPort`.

- [ ] **Step 3: Add the flag, resolution, and port wiring**

In `htrcli/internal/commands/root.go`:

Add `"github.com/u007/htrcli/internal/cdp"` to the import block.

Add two package vars to the existing `var (...)` block (after `client *api.Client`):

```go
	contextName         string
	resolvedContextPort int
```

Register the flag inside `init()` (after the `--timeout` registration):

```go
	rootCmd.PersistentFlags().StringVar(&contextName, "context", "", "named browser context (isolated profile); launches on first use")
```

Replace the root command's `PersistentPreRun` with `PersistentPreRunE` so context
resolution can fail loudly before any verb runs. Replace:

```go
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		output.JSONOutput = jsonOutput
		initClient()
	},
```

with:

```go
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		output.JSONOutput = jsonOutput
		initClient()
		return resolveContext()
	},
```

Add the resolver (place it just above `GetCDPPort`):

```go
// resolveContext launches (or reuses) the --context browser once, before any
// command runs, and caches its debugging port. Resolution failure aborts the
// command explicitly — a --context is never silently downgraded to the default
// browser. Contexts are a CDP concept, so setting --context implies CDP intent;
// verbs that ignore the CDP port simply never read resolvedContextPort.
func resolveContext() error {
	if contextName == "" {
		resolvedContextPort = 0
		return nil
	}
	chrome, err := cdp.FindChrome(GetChromePath())
	if err != nil {
		return fmt.Errorf("resolving context %q: %w", contextName, err)
	}
	port, err := cdp.EnsureContext(contextName, chrome, false)
	if err != nil {
		return fmt.Errorf("launching context %q: %w", contextName, err)
	}
	resolvedContextPort = port
	return nil
}
```

Replace the existing `GetCDPPort`:

```go
// GetCDPPort returns the CDP debugging port (flags none; env/config; default 9222).
func GetCDPPort() int {
	if p := viper.GetInt("cdp-port"); p > 0 {
		return p
	}
	return 9222
}
```

with:

```go
// GetCDPPort returns the CDP debugging port. A resolved --context wins over
// config/env; otherwise env/config `cdp-port`, else the 9222 default.
func GetCDPPort() int {
	if resolvedContextPort > 0 {
		return resolvedContextPort
	}
	if p := viper.GetInt("cdp-port"); p > 0 {
		return p
	}
	return 9222
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd htrcli && go test ./internal/commands/ -run 'TestGetCDPPort' -v`
Expected: PASS.

- [ ] **Step 5: Run the full commands suite (PreRunE regression guard)**

Run: `cd htrcli && go test ./internal/commands/...`
Expected: PASS — confirms switching `PersistentPreRun`→`PersistentPreRunE` did not
break existing command tests (e.g. `root_test.go`).

- [ ] **Step 6: Commit**

```bash
git add htrcli/internal/commands/root.go htrcli/internal/commands/root_test.go
git commit -m "feat(htrcli): --context global flag resolves to isolated CDP port"
```

---

### Task 4: `htrcli context list`

**Files:**
- Create: `htrcli/internal/commands/context.go`
- Test: `htrcli/internal/commands/context_test.go`

**Interfaces:**
- Consumes: `cdp.ReadContexts`, `cdp.PortAlive` (Task 1 / existing); `output.PrintJSON`, `output.NewTable`, `output.JSONOutput` (existing).
- Produces: `contextStatus{Name string, Port int, PID int, Profile string, Running bool}`, `collectContextStatuses() ([]contextStatus, error)`, the `context` / `context list` cobra commands.

- [ ] **Step 1: Write the failing test**

Create `htrcli/internal/commands/context_test.go`. It seeds the registry via the
`cdp` package, points one entry at a fake-live `/json/version` server and leaves
another on a dead port, then asserts `collectContextStatuses` reports the right
`Running` flags in name-sorted order:

```go
package commands

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/u007/htrcli/internal/cdp"
)

func TestCollectContextStatuses(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/json/version" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"webSocketDebuggerUrl":"ws://127.0.0.1/devtools/browser/x"}`)
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()
	_, portStr, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	livePort, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("atoi: %v", err)
	}

	// Seed two contexts: "work" live, "alpha" dead. cdp.EnsureContext isn't used
	// (it would launch Chrome); write the registry directly through a helper that
	// exercises the same persistence path. A dead port (1) yields Running=false.
	if err := cdp.WriteContextsForTest([]cdp.ContextEntry{
		{Name: "work", Port: livePort, PID: 10},
		{Name: "alpha", Port: 1, PID: 20},
	}); err != nil {
		t.Fatalf("seed contexts: %v", err)
	}

	statuses, err := collectContextStatuses()
	if err != nil {
		t.Fatalf("collectContextStatuses: %v", err)
	}
	if len(statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %d: %+v", len(statuses), statuses)
	}
	// Registry is persisted sorted by name: alpha first.
	if statuses[0].Name != "alpha" || statuses[0].Running {
		t.Fatalf("expected alpha not running, got %+v", statuses[0])
	}
	if statuses[1].Name != "work" || !statuses[1].Running {
		t.Fatalf("expected work running, got %+v", statuses[1])
	}
}
```

This test needs a tiny exported test seam so the command test can seed the
registry without launching Chrome. Add it to `htrcli/internal/cdp/context.go`:

```go
// WriteContextsForTest persists entries directly. Exported solely so
// command-package tests can seed the registry without launching Chrome.
func WriteContextsForTest(entries []ContextEntry) error {
	return writeContexts(entries)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd htrcli && go test ./internal/commands/ -run 'TestCollectContextStatuses' -v`
Expected: FAIL — `collectContextStatuses` undefined (and `cdp.WriteContextsForTest`
undefined until the seam above is added).

- [ ] **Step 3: Write the command**

Add `WriteContextsForTest` to `htrcli/internal/cdp/context.go` as shown in Step 1,
then create `htrcli/internal/commands/context.go`:

```go
package commands

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/u007/htrcli/internal/cdp"
	"github.com/u007/htrcli/internal/output"
)

var contextCmd = &cobra.Command{
	Use:   "context",
	Short: "Manage isolated browser contexts",
}

type contextStatus struct {
	Name    string `json:"name"`
	Port    int    `json:"port"`
	PID     int    `json:"pid"`
	Profile string `json:"profileDir"`
	Running bool   `json:"running"`
}

// collectContextStatuses returns each registered context with a live-probe of
// its debugging port. The registry is persisted name-sorted, so the output is
// already sorted.
func collectContextStatuses() ([]contextStatus, error) {
	entries, err := cdp.ReadContexts()
	if err != nil {
		return nil, err
	}
	out := make([]contextStatus, 0, len(entries))
	for _, e := range entries {
		out = append(out, contextStatus{
			Name:    e.Name,
			Port:    e.Port,
			PID:     e.PID,
			Profile: e.ProfileDir,
			Running: cdp.PortAlive(e.Port),
		})
	}
	return out, nil
}

var contextListCmd = &cobra.Command{
	Use:   "list",
	Short: "List browser contexts",
	RunE: func(cmd *cobra.Command, args []string) error {
		statuses, err := collectContextStatuses()
		if err != nil {
			return err
		}
		if output.JSONOutput {
			output.PrintJSON(statuses)
			return nil
		}
		if len(statuses) == 0 {
			fmt.Println("No contexts")
			return nil
		}
		table := output.NewTable("Name", "Port", "PID", "Running")
		for _, s := range statuses {
			running := "no"
			if s.Running {
				running = "yes"
			}
			table.AddRow(s.Name, fmt.Sprintf("%d", s.Port), fmt.Sprintf("%d", s.PID), running)
		}
		fmt.Print(table)
		return nil
	},
}

func init() {
	contextCmd.AddCommand(contextListCmd)
	rootCmd.AddCommand(contextCmd)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd htrcli && go test ./internal/commands/ -run 'TestCollectContextStatuses' -v && go test ./internal/cdp/...`
Expected: PASS.

- [ ] **Step 5: Full suite + build**

Run: `cd htrcli && go test ./... && go build ./...`
Expected: PASS / clean build.

- [ ] **Step 6: Manual smoke test**

Run:
```bash
make htrcli-build
./htrcli/bin/htrcli --context work browser start   # launches an isolated Chrome
./htrcli/bin/htrcli context list                   # shows "work" Running=yes
./htrcli/bin/htrcli --context work open https://example.com
./htrcli/bin/htrcli context list --json
```
Expected: `context list` shows `work` running on its own port with a PID; the
`work` Chrome window is a distinct profile (no shared cookies with the default
browser); `open` navigates the context's tab, not the default browser's.

- [ ] **Step 7: Commit**

```bash
git add htrcli/internal/commands/context.go htrcli/internal/commands/context_test.go htrcli/internal/cdp/context.go
git commit -m "feat(htrcli): context list subcommand"
```

---

## Part 1 Self-Review

- **Spec coverage (§7a):** `--context <name>` global flag → Task 3; separate
  `--user-data-dir` profile launch (chosen over `Target.createBrowserContext`,
  justified in the header) → Task 2; `context list` → Task 4; registry threaded
  through `root.go` → Task 3. Firefox `-profile` fallback is noted as out of scope
  for this Chrome-first task (documented in `EnsureContext`'s doc comment), matching
  the spec's "Firefox fallback: a separate Firefox process" as a later addition.
- **Placeholder scan:** every step ships complete code; no TBD/TODO; the one test
  seam (`WriteContextsForTest`) is fully defined where first referenced.
- **Type consistency:** `ContextEntry` fields (`Name/ProfileDir/Port/PID/CreatedAt`)
  are used identically in Tasks 1, 2, 4; `EnsureContext(name, chromePath, headless)`
  and `GetCDPPort()`/`resolveContext()` signatures match across Tasks 2–3;
  `contextStatus` shape is consistent in Task 4's code and test.
- **Deferred:** Firefox context process launch; context teardown/`context rm`
  (not in spec §7a — only `list` is specified).
