# Part 2: Detection, Controller Interface, and Test Infrastructure

Builds the foundation of the new `internal/tray` package: the pure detection function (highest-leverage test in the feature) and the Controller interface (the seam between tray and daemon).

---

### Task 4: `tray.ShouldStart` (pure function)

The detection function is the part that determines whether a server accidentally gets a tray. It must be a pure function (no I/O, no goroutines, no OS calls) so it can be exhaustively unit-tested.

**Files:**
- Create: `htrcli/internal/tray/detect.go`
- Create: `htrcli/internal/tray/detect_test.go`

**Interfaces:**
- Produces: `func ShouldStart(noTray bool) bool` — returns true if the tray should be launched, false otherwise. Reads `DISPLAY`, `WAYLAND_DISPLAY`, `SSH_CONNECTION`, `SSH_TTY`, `HTRCLI_NO_TRAY`. On macOS/Windows, returns `true` (or `!noTray`) unconditionally. On Linux, returns `true` only if (a) at least one of `DISPLAY` / `WAYLAND_DISPLAY` is set, AND (b) neither `SSH_CONNECTION` nor `SSH_TTY` is set, AND (c) `noTray` is false and `HTRCLI_NO_TRAY` is unset.

- [ ] **Step 1: Write the failing test**

In `htrcli/internal/tray/detect_test.go`:

```go
package tray

import (
    "runtime"
    "testing"
)

func TestShouldStart(t *testing.T) {
    tests := []struct {
        name      string
        goos      string
        env       map[string]string
        noTray    bool
        want      bool
    }{
        {"macos always on",                 "darwin", map[string]string{}, false, true},
        {"windows always on",               "windows", map[string]string{}, false, true},
        {"linux x11 desktop",               "linux", map[string]string{"DISPLAY": ":0"}, false, true},
        {"linux wayland desktop",           "linux", map[string]string{"WAYLAND_DISPLAY": "wayland-0"}, false, true},
        {"linux both empty (headless)",     "linux", map[string]string{}, false, false},
        {"linux ssh session + x11",         "linux", map[string]string{"DISPLAY": ":0", "SSH_CONNECTION": "1.2.3.4 5 6.7.8.9 10"}, false, false},
        {"linux ssh + tty",                 "linux", map[string]string{"DISPLAY": ":0", "SSH_TTY": "/dev/pts/0"}, false, false},
        {"linux ssh headless",              "linux", map[string]string{"SSH_CONNECTION": "1.2.3.4 5 6.7.8.9 10"}, false, false},
        {"linux desktop HTRCLI_NO_TRAY=1",  "linux", map[string]string{"DISPLAY": ":0", "HTRCLI_NO_TRAY": "1"}, false, false},
        {"linux desktop --no-tray",         "linux", map[string]string{"DISPLAY": ":0"}, true, false},
        {"macos no-tray override",          "darwin", map[string]string{}, true, false},
        {"macos HTRCLI_NO_TRAY override",   "darwin", map[string]string{"HTRCLI_NO_TRAY": "1"}, false, false},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Clear all relevant env vars first.
            for _, k := range []string{"DISPLAY", "WAYLAND_DISPLAY", "SSH_CONNECTION", "SSH_TTY", "HTRCLI_NO_TRAY"} {
                t.Setenv(k, "") // t.Setenv with "" is equivalent to unset for the test
            }
            for k, v := range tt.env {
                t.Setenv(k, v)
            }
            if got := ShouldStartFor(tt.goos, tt.noTray); got != tt.want {
                t.Errorf("ShouldStartFor(%q, %v) with env %v = %v, want %v",
                    tt.goos, tt.noTray, tt.env, got, tt.want)
            }
        })
    }
}
```

The test calls `ShouldStartFor(goos, noTray)` — a testable variant of `ShouldStart` that takes the GOOS as a parameter. The public `ShouldStart(noTray bool)` is a thin wrapper that calls `ShouldStartFor(runtime.GOOS, noTray)`.

- [ ] **Step 2: Run test to verify it fails**

```bash
cd htrcli && go test ./internal/tray/ -v
```

Expected: compile error — package doesn't exist.

- [ ] **Step 3: Implement `ShouldStart` and `ShouldStartFor`**

In `htrcli/internal/tray/detect.go`:

```go
package tray

import "os"

// ShouldStart reports whether the tray should be launched. It is a thin
// wrapper around ShouldStartFor using runtime.GOOS.
func ShouldStart(noTray bool) bool {
    return ShouldStartFor(/* runtime.GOOS */ "linux", noTray) // see below
}
```

Wait — `ShouldStart` should use `runtime.GOOS` directly; the test variant `ShouldStartFor` exists for testability. Refactor:

```go
package tray

import (
    "os"
    "runtime"
)

// ShouldStart reports whether the tray should be launched.
func ShouldStart(noTray bool) bool {
    return ShouldStartFor(runtime.GOOS, noTray)
}

// ShouldStartFor is the testable core: same logic as ShouldStart but
// takes the GOOS as a parameter so tests can pin it.
func ShouldStartFor(goos string, noTray bool) bool {
    // Opt-out wins on every platform.
    if noTray {
        return false
    }
    if os.Getenv("HTRCLI_NO_TRAY") != "" {
        return false
    }

    // macOS and Windows have native system trays; trust the platform.
    if goos == "darwin" || goos == "windows" {
        return true
    }

    // Linux: require a display, AND no active SSH session.
    if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
        return false
    }
    if os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_TTY") != "" {
        return false
    }
    return true
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd htrcli && go test ./internal/tray/ -v
```

Expected: ALL 12 cases PASS. If any case fails, the detection rule has a bug — fix before continuing.

- [ ] **Step 5: Commit**

```bash
cd htrcli && git add internal/tray/detect.go internal/tray/detect_test.go
git commit -m "feat(htrcli): tray.ShouldStart with table-driven tests

Pure-function detection. Highest-leverage test in the tray feature:
regression catcher for 'did we accidentally put a tray on a server?'

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 5: Controller interface, Status struct, fakeController

The Controller is the seam between the tray and `*host.Daemon`. Define it as an interface so the menu logic can be tested with a `fakeController` that returns canned values.

**Files:**
- Create: `htrcli/internal/tray/controller.go`
- Create: `htrcli/internal/tray/controller_test.go`

**Interfaces:**
- Produces:
  - `type Controller interface { IsRunning, Restart, Quit, Status, RecentLog, ReinstallHost, OpenConfigFolder, OpenLog, CopyTokenToClipboard }`
  - `type Status struct { Port, RelaysConnected int; LastError, TokenFingerprint string }`
  - `type Commander interface { Run(name string, args ...string) error; Output(name string, args ...string) ([]byte, error) }` — for `os/exec` mocking in tests
  - `type fakeController struct { ... }` with configurable canned return values and call recording (lives in `controller_test.go`)

- [ ] **Step 1: Define the Controller interface and Status struct**

In `htrcli/internal/tray/controller.go`:

```go
package tray

// Controller is the surface the tray needs from the daemon. Start/Stop
// are intentionally absent: htrcli serve is a single-process daemon that
// owns its HTTP port; suspending and resuming without exiting is not
// supported. The tray exposes only Restart (re-execs) and Quit.
type Controller interface {
    // Lifecycle
    IsRunning() bool
    Restart() error
    Quit() error

    // Status (read by the 5s refresh goroutine)
    Status() Status
    RecentLog(n int) []string

    // Maintenance
    ReinstallHost(browser string) error
    OpenConfigFolder() error
    OpenLog() error
    CopyTokenToClipboard() (string, error)
}

type Status struct {
    Port             int
    RelaysConnected  int
    LastError        string
    TokenFingerprint string
}

// Commander abstracts os/exec so tests can verify which binary was
// chosen and with which args, without actually spawning the process.
type Commander interface {
    Run(name string, args ...string) error
    Output(name string, args ...string) ([]byte, error)
}
```

- [ ] **Step 2: Write a fakeController test**

In `htrcli/internal/tray/controller_test.go`:

```go
package tray

import "testing"

type fakeController struct {
    running        bool
    status         Status
    recentLog      []string
    restartCalled  int
    quitCalled     int
    reinstallCalls []string
    openCfgCalled  int
    openLogCalled  int
    copyTokCalled  int
    copyTokReturn  string
    copyTokErr     error
    openCfgErr     error
    openLogErr     error
    reinstallErr   error
    restartErr     error
}

func (f *fakeController) IsRunning() bool                  { return f.running }
func (f *fakeController) Restart() error                   { f.restartCalled++; return f.restartErr }
func (f *fakeController) Quit() error                      { f.quitCalled++; return nil }
func (f *fakeController) Status() Status                   { return f.status }
func (f *fakeController) RecentLog(n int) []string          { return f.recentLog }
func (f *fakeController) ReinstallHost(b string) error      { f.reinstallCalls = append(f.reinstallCalls, b); return f.reinstallErr }
func (f *fakeController) OpenConfigFolder() error          { f.openCfgCalled++; return f.openCfgErr }
func (f *fakeController) OpenLog() error                   { f.openLogCalled++; return f.openLogErr }
func (f *fakeController) CopyTokenToClipboard() (string, error) {
    f.copyTokCalled++
    return f.copyTokReturn, f.copyTokErr
}

func TestFakeControllerRoundTrip(t *testing.T) {
    f := &fakeController{
        running: true,
        status:  Status{Port: 3845, RelaysConnected: 2, LastError: "", TokenFingerprint: "abcd…wxyz"},
    }
    var c Controller = f
    if !c.IsRunning() { t.Fatal("running") }
    if s := c.Status(); s.Port != 3845 || s.RelaysConnected != 2 { t.Fatalf("status: %+v", s) }
    if err := c.Restart(); err != nil { t.Fatal(err) }
    if f.restartCalled != 1 { t.Fatalf("restart not called") }
}
```

- [ ] **Step 3: Run test to verify it passes**

```bash
cd htrcli && go test ./internal/tray/ -v
```

Expected: PASS. (`TestShouldStart` still passes from Task 4; new `TestFakeControllerRoundTrip` also passes.)

- [ ] **Step 4: Commit**

```bash
cd htrcli && git add internal/tray/controller.go internal/tray/controller_test.go
git commit -m "feat(htrcli): tray Controller interface + Status + fakeController

The seam between the tray and *host.Daemon. fakeController lets
tests drive menu behavior without touching the daemon.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 6: Build-tag-gated `traytest` (fakeSystray + 50ms ticker)

`getlantern/systray` is hard to unit-test because it touches the OS UI. A `fakeSystray` shim behind a `traytest` build tag lets us drive the full click → Controller → refresh loop in unit tests without a real OS tray.

**Files:**
- Create: `htrcli/internal/tray/fake_systray_traytest.go` (build tag `traytest`)
- Create: `htrcli/internal/tray/fake_systray_test.go` (build tag `traytest`)

**Interfaces:**
- Produces (under `traytest` build tag): a `fakeSystray` that exposes the same package-level API as `getlantern/systray` (`Run`, `Quit`, `AddMenuItem`, `AddSubMenuItem`, `AddSeparator`, `SetTitle`, `SetTooltip`) and lets tests simulate clicks, assert menu titles, and override the 5s refresh ticker to 50ms.

- [ ] **Step 1: Add the dependency**

In `htrcli/go.mod`, the `getlantern/systray` dependency is added in Part 3 (when `tray.go` is written). For now, design the fakeSystray to match the public surface you intend to use.

In `htrcli/internal/tray/fake_systray_traytest.go`:

```go
//go:build traytest
// +build traytest

package tray

import (
    "sync"
    "testing"
    "time"
)

// fakeSystray mirrors the public API of getlantern/systray so we can
// drive the menu from tests. It is selected at build time via the
// `traytest` tag.

var (
    fsMu       sync.Mutex
    fsOnReady  func()
    fsOnExit   func()
    fsItems    = map[int]*fakeItem{} // id -> item
    fsNextID   = 1
    fsQuit     = make(chan struct{})
    fsTickRate = 50 * time.Millisecond // override the 5s refresh
)

type fakeItem struct {
    id        int
    title     string
    tooltip   string
    disabled  bool
    parent    int
    clickCh   chan struct{}
    children  []int
}

func fsNewItem(title string, disabled bool) *fakeItem {
    fsMu.Lock()
    defer fsMu.Unlock()
    it := &fakeItem{
        id:       fsNextID,
        title:    title,
        disabled: disabled,
        clickCh:  make(chan struct{}, 1),
    }
    fsNextID++
    fsItems[it.id] = it
    return it
}

func Run(onReady, onExit func()) {
    fsMu.Lock()
    fsOnReady, fsOnExit = onReady, onExit
    fsMu.Unlock()
    if onReady != nil {
        onReady()
    }
    <-fsQuit
    if onExit != nil {
        onExit()
    }
}

func Quit() {
    close(fsQuit)
}

func SetIcon(iconBytes []byte)         {}
func SetTitle(title string)            {}
func SetTooltip(tooltip string)        {}
func AddSeparator()                    { fsNewItem("---", false) }
func AddMenuItem(title string, disabled bool) *MenuItem { return &MenuItem{it: fsNewItem(title, disabled)} }

// The rest of the API follows the same pattern. See full file for details.
```

(This is a sketch; the full file is ~150 lines. The key idea: every function in `getlantern/systray` that we call gets a fakeSystray twin in this file, gated behind `traytest`. The build tag means this file is excluded from production builds.)

- [ ] **Step 2: Expose a hook for tests to simulate clicks**

In `fake_systray_traytest.go`, add:

```go
// Click simulates a click on a menu item by its title (for test readability).
func Click(title string) bool {
    fsMu.Lock()
    defer fsMu.Unlock()
    for _, it := range fsItems {
        if it.title == title {
            select {
            case it.clickCh <- struct{}{}:
                return true
            default:
            }
        }
    }
    return false
}

// Title returns the current title of a menu item (for test assertions).
func Title(title string) string {
    fsMu.Lock()
    defer fsMu.Unlock()
    for _, it := range fsItems {
        if it.title == title {
            return it.title
        }
    }
    return ""
}

// TickRate returns the override refresh interval for tests.
func TickRate() time.Duration { return fsTickRate }
```

- [ ] **Step 3: Write a smoke test for fakeSystray itself**

In `htrcli/internal/tray/fake_systray_test.go`:

```go
//go:build traytest
// +build traytest

package tray

import "testing"

func TestFakeSystrayAddAndClick(t *testing.T) {
    done := make(chan struct{})
    go func() {
        defer close(done)
        Run(func() {
            item := AddMenuItem("Hello", false)
            go func() {
                <-item.ClickedCh
                item.SetTitle("Clicked!")
            }()
        }, nil)
    }()
    if !Click("Hello") { t.Fatal("click failed") }
    // Give the title-update goroutine a moment.
    time.Sleep(10 * time.Millisecond)
    if got := Title("Hello"); got != "Clicked!" {
        t.Fatalf("got %q, want %q", got, "Clicked!")
    }
    Quit()
    <-done
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd htrcli && go test -tags=traytest ./internal/tray/ -v
```

Expected: PASS for `TestFakeSystrayAddAndClick`. (Other tests in the package — `TestShouldStart`, `TestFakeControllerRoundTrip` — should also still pass because they don't depend on the systray API.)

- [ ] **Step 5: Commit**

```bash
cd htrcli && git add internal/tray/fake_systray_traytest.go internal/tray/fake_systray_test.go
git commit -m "test(htrcli): build-tagged fakeSystray for menu-driven tests

Lets us drive the click → controller → refresh loop in unit tests
without touching the real OS UI. Excluded from production builds.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Part 2 complete when:

- `cd htrcli && go test ./...` passes.
- `cd htrcli && go test -tags=traytest ./internal/tray/ -v` passes.
- `ShouldStartFor` returns the correct value for all 12 table cases.
- The Controller interface compiles and the fakeController round-trips.
- fakeSystray can be added, clicked, and the title updated in test.

Proceed to Part 3 only when all three tasks are committed and both test invocations are green.
