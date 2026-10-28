package cdp

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestLaunchArgs(t *testing.T) {
	args := LaunchArgs(9222, "/home/u/.htrcli/chrome-profile", false, nil)
	for _, want := range []string{
		"--remote-debugging-port=9222",
		"--user-data-dir=/home/u/.htrcli/chrome-profile",
		"--no-first-run",
		"--disable-backgrounding-occluded-windows",
		"--disable-renderer-backgrounding",
	} {
		if !slices.Contains(args, want) {
			t.Errorf("missing %s in %v", want, args)
		}
	}
	if slices.Contains(args, "--headless") {
		t.Error("headless flag present without headless=true")
	}
	for _, a := range args {
		if strings.Contains(a, "--remote-debugging-address") {
			t.Fatal("must never pass --remote-debugging-address")
		}
	}
}

func TestLaunchArgsHeadless(t *testing.T) {
	args := LaunchArgs(9333, "/p", true, nil)
	if !slices.Contains(args, "--headless") {
		t.Error("want plain --headless")
	}
	if slices.Contains(args, "--headless=new") {
		t.Error("--headless=new is a deprecated alias; use --headless")
	}
}

func TestFindChromeConfigured(t *testing.T) {
	f := filepath.Join(t.TempDir(), "chrome")
	if err := os.WriteFile(f, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	got, err := FindChrome(f)
	if err != nil || got != f {
		t.Fatalf("want %s, got %s (%v)", f, got, err)
	}
}

func TestFindChromeMissing(t *testing.T) {
	// Can't simulate "nothing installed" when the search list finds a Chrome on
	// this host; skip so the test still validates the error elsewhere.
	if _, err := FindChrome(""); err == nil {
		t.Skip("a Chrome is installed on this host; cannot test missing-path error")
	}
	_, err := FindChrome(filepath.Join(t.TempDir(), "nope"))
	if err == nil || !strings.Contains(err.Error(), "set-chrome-path") {
		t.Fatalf("error must mention config set-chrome-path, got %v", err)
	}
}

func TestResolveChromeSkipsRejectedConfiguredPath(t *testing.T) {
	dir := t.TempDir() // a directory, as when the app bundle is set instead of its binary
	exe := filepath.Join(t.TempDir(), "chrome")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	got, err := resolveChrome(dir, []string{dir, exe})
	if err != nil || got != exe {
		t.Fatalf("want the search to continue to %s, got %q (%v)", exe, got, err)
	}
}

func TestResolveChromeErrorQuotesEveryRejectedCandidate(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(t.TempDir(), "absent")
	bare := "htrcli-no-such-chrome-binary"
	_, err := resolveChrome(dir, []string{dir, missing, bare})
	if err == nil {
		t.Fatal("want a not-found error")
	}
	msg := err.Error()
	// A directory is "is a directory" on Unix toolchains and may be reported as
	// permission denied elsewhere, so either reason is accepted for it.
	dirQuoted := strconv.Quote(dir)
	if !strings.Contains(msg, dirQuoted+": is a directory") && !strings.Contains(msg, dirQuoted+": permission denied") {
		t.Fatalf("error %q does not quote the configured directory with its reason", msg)
	}
	ordered := []string{dirQuoted, strconv.Quote(missing) + ": no such file", strconv.Quote(bare) + ": not found in PATH"}
	last := -1
	for _, want := range ordered {
		i := strings.Index(msg, want)
		if i < 0 {
			t.Fatalf("error %q does not quote %s", msg, want)
		}
		if i <= last {
			t.Fatalf("candidates must be listed in search order, configured first; error = %q", msg)
		}
		last = i
	}
}

func TestReadStateMissingFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	st, err := ReadState()
	if err != nil || st != nil {
		t.Fatalf("want nil,nil for missing file, got %v,%v", st, err)
	}
}

func TestStateRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	want := &BrowserState{PID: 42, Port: 9222, Headless: true, Args: []string{"--no-sandbox", "--window-size=390,844"}, StderrLog: "/p/chrome-profile/chrome-stderr.log"}
	if err := writeState(want); err != nil {
		t.Fatal(err)
	}
	got, err := ReadState()
	if err != nil || got == nil || got.PID != 42 || !got.Headless {
		t.Fatalf("round trip failed: %v %v", got, err)
	}
	if !slices.Equal(got.Args, want.Args) {
		t.Fatalf("args did not round trip: got %v, want %v", got.Args, want.Args)
	}
	if got.StderrLog != want.StderrLog {
		t.Fatalf("stderr_log did not round trip: got %q, want %q", got.StderrLog, want.StderrLog)
	}
}

func TestClearStaleSingletonLockDeadPID(t *testing.T) {
	dir := t.TempDir()
	host, _ := os.Hostname()
	// PID 2^22-1 is above the default macOS/Linux pid_max, so it is never alive.
	for _, name := range []string{"SingletonLock", "SingletonSocket", "SingletonCookie"} {
		if err := os.Symlink(host+"-4194303", filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	removed, err := clearStaleSingletonLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("expected stale lock to be removed")
	}
	for _, name := range []string{"SingletonLock", "SingletonSocket", "SingletonCookie"} {
		if _, err := os.Lstat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s should be gone, got %v", name, err)
		}
	}
}

func TestClearStaleSingletonLockLivePID(t *testing.T) {
	dir := t.TempDir()
	host, _ := os.Hostname()
	if err := os.Symlink(fmt.Sprintf("%s-%d", host, os.Getpid()), filepath.Join(dir, "SingletonLock")); err != nil {
		t.Fatal(err)
	}
	removed, err := clearStaleSingletonLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	if removed {
		t.Fatal("must not remove a lock owned by a live process")
	}
	if _, err := os.Lstat(filepath.Join(dir, "SingletonLock")); err != nil {
		t.Fatalf("lock should still exist: %v", err)
	}
}

func TestClearStaleSingletonLockAbsent(t *testing.T) {
	removed, err := clearStaleSingletonLock(t.TempDir())
	if err != nil || removed {
		t.Fatalf("absent lock: want false,nil got %v,%v", removed, err)
	}
}

func TestClearStaleSingletonLockMalformed(t *testing.T) {
	dir := t.TempDir()
	if err := os.Symlink("garbage", filepath.Join(dir, "SingletonLock")); err != nil {
		t.Fatal(err)
	}
	if _, err := clearStaleSingletonLock(dir); err == nil {
		t.Fatal("malformed lock target must be an error, not silently removed")
	}
}

func TestProcessAliveRejectsNonPositivePID(t *testing.T) {
	for _, pid := range []int{0, -1} {
		if processAlive(pid) {
			t.Errorf("processAlive(%d) = true; pid %d never identifies a recorded process", pid, pid)
		}
	}
}

func TestProcessListeningPIDFindsListener(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("lsof-based lookup; the netstat parser runs on Windows hosts")
	}
	if _, err := exec.LookPath("lsof"); err != nil {
		t.Skipf("lsof unavailable: %v", err)
	}
	mux := http.NewServeMux()
	port := testServer(t, mux)
	pid, err := processListeningPID(port)
	if err != nil {
		t.Fatalf("processListeningPID(%d): %v", port, err)
	}
	if pid != os.Getpid() {
		t.Fatalf("want listener pid %d, got %d", os.Getpid(), pid)
	}
}

func TestStartBrowserAdoptsLiveListenerPID(t *testing.T) {
	cases := []struct {
		name  string
		state *BrowserState
	}{
		{"recorded pid is zero", &BrowserState{PID: 0}},
		{"recorded pid is dead", &BrowserState{PID: 4194303}},
		{"no state file", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			mux := http.NewServeMux()
			mux.HandleFunc("/json/version", func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(`{"Browser":"Chrome/140.0","webSocketDebuggerUrl":"ws://127.0.0.1:9222/devtools/browser/abc"}`))
			})
			port := testServer(t, mux) // PortAlive(port) is now true

			if tc.state != nil {
				tc.state.Port = port
				if err := writeState(tc.state); err != nil {
					t.Fatal(err)
				}
			}

			orig := listeningPIDFn
			t.Cleanup(func() { listeningPIDFn = orig })
			listeningPIDFn = func(p int) (int, error) {
				if p != port {
					t.Errorf("listeningPIDFn port = %d, want %d", p, port)
				}
				return 4242, nil
			}

			st, err := StartBrowser("ignored-chrome-path", port, false, nil)
			if err != nil {
				t.Fatalf("StartBrowser: %v", err)
			}
			if st.PID != 4242 {
				t.Fatalf("want adopted pid 4242, got %d", st.PID)
			}
			persisted, err := ReadState()
			if err != nil || persisted == nil {
				t.Fatalf("ReadState: %+v %v", persisted, err)
			}
			if persisted.PID != 4242 || persisted.Port != port {
				t.Fatalf("persisted state = %+v, want pid 4242 port %d", persisted, port)
			}
		})
	}
}

func TestStartBrowserRefusesUnidentifiableListener(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	mux := http.NewServeMux()
	mux.HandleFunc("/json/version", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"Browser":"Chrome/140.0","webSocketDebuggerUrl":"ws://127.0.0.1:9222/devtools/browser/abc"}`))
	})
	port := testServer(t, mux)

	orig := listeningPIDFn
	t.Cleanup(func() { listeningPIDFn = orig })
	listeningPIDFn = func(int) (int, error) {
		return 0, errors.New("listener lookup failed")
	}

	if _, err := StartBrowser("ignored-chrome-path", port, false, nil); err == nil {
		t.Fatal("want an error when the listener pid cannot be resolved")
	}
	// Must not have persisted a meaningless PID-0 state.
	if st, err := ReadState(); err != nil || st != nil {
		t.Fatalf("state must stay absent, got %+v err %v", st, err)
	}
}

// writeFakeChrome writes a Chrome stand-in that prints stderrLine and then runs
// until killed, so a launch against it always times out.
func writeFakeChrome(t *testing.T, dir, stderrLine string) string {
	t.Helper()
	fake := filepath.Join(dir, "fake-chrome")
	script := "#!/bin/sh\necho '" + stderrLine + "' >&2\nexec sleep 30\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return fake
}

func TestLaunchChromeDoesNotTruncateAnotherProfilesLog(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell script as the fake Chrome")
	}
	dir := t.TempDir()
	fake := writeFakeChrome(t, dir, "context chrome output")
	port, err := freePort()
	if err != nil {
		t.Fatal(err)
	}
	origTimeout := launchTimeout
	t.Cleanup(func() { launchTimeout = origTimeout })
	launchTimeout = 400 * time.Millisecond

	defaultProfile := filepath.Join(dir, "default")
	if err := os.MkdirAll(defaultProfile, 0o700); err != nil {
		t.Fatal(err)
	}
	defaultLog := ChromeStderrLogPath(defaultProfile)
	if err := os.WriteFile(defaultLog, []byte("default chrome output\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	contextProfile := filepath.Join(dir, "context")
	if _, err := launchChrome(fake, port, contextProfile, true, nil); err == nil {
		t.Fatal("want the fake Chrome to time out")
	}
	got, err := os.ReadFile(defaultLog)
	if err != nil || string(got) != "default chrome output\n" {
		t.Fatalf("launching another profile changed the default log: %q (%v)", got, err)
	}
	ctxLog, err := os.ReadFile(ChromeStderrLogPath(contextProfile))
	if err != nil || !strings.Contains(string(ctxLog), "context chrome output") {
		t.Fatalf("the launched profile's log must hold its own Chrome output: %q (%v)", ctxLog, err)
	}
}

func TestLaunchChromeTimeoutReapsChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell script as the fake Chrome")
	}
	dir := t.TempDir()
	fake := writeFakeChrome(t, dir, "fake chrome: sandbox refused")
	port, err := freePort()
	if err != nil {
		t.Fatal(err)
	}
	origTimeout := launchTimeout
	t.Cleanup(func() { launchTimeout = origTimeout })
	launchTimeout = 400 * time.Millisecond

	start := time.Now()
	_, err = launchChrome(fake, port, filepath.Join(dir, "profile"), true, nil)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("want a timeout error")
	}
	if !strings.Contains(err.Error(), "last stderr:") || !strings.Contains(err.Error(), "fake chrome: sandbox refused") {
		t.Fatalf("timeout error must quote the Chrome stderr tail, got %v", err)
	}
	if _, err := os.Stat(ChromeStderrLogPath(filepath.Join(dir, "profile"))); err != nil {
		t.Fatalf("Chrome stderr log must live inside the profile dir: %v", err)
	}
	// A child that was killed but never reaped stays a zombie, which keeps
	// processAlive true and burns terminateProcess's full 5s SIGTERM grace.
	if elapsed > launchTimeout+2*time.Second {
		t.Fatalf("timeout path took %s; the killed child was not reaped promptly", elapsed)
	}
}

func TestClearStaleSingletonLockForeignHost(t *testing.T) {
	dir := t.TempDir()
	// Another machine's lock with a pid that is not alive here must survive:
	// a shared/synced profile dir makes the pid meaningless on this host.
	if err := os.Symlink("some-other-host-4194303", filepath.Join(dir, "SingletonLock")); err != nil {
		t.Fatal(err)
	}
	removed, err := clearStaleSingletonLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	if removed {
		t.Fatal("must not remove a lock held by another host")
	}
	if _, err := os.Lstat(filepath.Join(dir, "SingletonLock")); err != nil {
		t.Fatalf("foreign lock should still exist: %v", err)
	}
}

func TestClearStaleSingletonLockHostMatchIsCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	// Chrome may record the hostname with different casing; a dead local owner
	// must still be recognized and cleared.
	if err := os.Symlink(strings.ToUpper(host)+"-4194303", filepath.Join(dir, "SingletonLock")); err != nil {
		t.Fatal(err)
	}
	removed, err := clearStaleSingletonLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("expected a same-host stale lock to be removed regardless of case")
	}
}

func TestLaunchArgsExtraAppended(t *testing.T) {
	extra := []string{"--no-sandbox", "--window-size=390,844"}
	args := LaunchArgs(9222, "/p", true, extra)
	tail := len(args) - len(extra)
	if !slices.Equal(args[tail:], extra) {
		t.Fatalf("extra args must be last, got %v", args)
	}
	if slices.Index(args, "--headless") >= tail {
		t.Fatalf("extra args must follow --headless, got %v", args)
	}
}

func TestValidateExtraChromeArgsRejectsOwnedSwitches(t *testing.T) {
	for _, arg := range []string{
		"--remote-debugging-port",
		"--remote-debugging-port=9999",
		"--remote-debugging-address",
		"--remote-debugging-address=0.0.0.0",
		"-remote-debugging-address=0.0.0.0",
		"---remote-debugging-port=1",
		"--user-data-dir",
		"--user-data-dir=/tmp/x",
		"-user-data-dir=/tmp/x",
	} {
		if err := validateExtraChromeArgs([]string{arg}); err == nil {
			t.Errorf("want %q rejected", arg)
		}
	}
}

func TestValidateExtraChromeArgsAcceptsOrdinaryFlags(t *testing.T) {
	ok := []string{"--no-sandbox", "--window-size=390,844", "--force-dark-mode", "--disable-dev-shm-usage", "--use-mock-keychain"}
	if err := validateExtraChromeArgs(ok); err != nil {
		t.Fatalf("want accepted, got %v", err)
	}
}

func TestChromeCandidatesPerOS(t *testing.T) {
	noEnv := func(string) string { return "" }
	winEnv := func(k string) string {
		return map[string]string{
			"ProgramFiles":      `C:\Program Files`,
			"ProgramFiles(x86)": `C:\Program Files (x86)`,
			"LOCALAPPDATA":      `C:\Users\u\AppData\Local`,
		}[k]
	}
	cases := []struct {
		name   string
		goos   string
		getenv func(string) string
		want   []string
	}{
		{"darwin", "darwin", noEnv, []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		}},
		{"linux", "linux", noEnv, []string{
			"google-chrome", "google-chrome-stable", "chromium", "chromium-browser",
			"/usr/bin/google-chrome", "/usr/bin/google-chrome-stable", "/usr/bin/chromium", "/usr/bin/chromium-browser",
			"/snap/bin/google-chrome", "/snap/bin/google-chrome-stable", "/snap/bin/chromium", "/snap/bin/chromium-browser",
		}},
		{"windows", "windows", winEnv, []string{
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
			`C:\Users\u\AppData\Local\Google\Chrome\Application\chrome.exe`,
		}},
		{"windows with no install roots", "windows", noEnv, nil},
		{"unknown OS", "plan9", noEnv, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := chromeCandidates(tc.goos, tc.getenv)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("chromeCandidates(%q) = %v, want %v", tc.goos, got, tc.want)
			}
		})
	}
}

func writeTestLog(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "chrome-stderr.log")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadStderrTailCapsAtWholeLines(t *testing.T) {
	var b strings.Builder
	for i := range 1000 {
		fmt.Fprintf(&b, "line %04d\n", i)
	}
	tail, err := readStderrTail(writeTestLog(t, b.String()))
	if err != nil {
		t.Fatalf("readStderrTail: %v", err)
	}
	if len(tail) > stderrTailBytes {
		t.Fatalf("tail is %d bytes, want at most %d", len(tail), stderrTailBytes)
	}
	lines := strings.Split(tail, "\n")
	first := 999 - len(lines) + 1
	for k, line := range lines {
		if want := fmt.Sprintf("line %04d", first+k); line != want {
			t.Fatalf("line %d = %q, want %q (partial or misplaced line)", k, line, want)
		}
	}
}

func TestReadStderrTailShortLog(t *testing.T) {
	tail, err := readStderrTail(writeTestLog(t, "first\nsecond\n"))
	if err != nil {
		t.Fatalf("readStderrTail: %v", err)
	}
	if tail != "first\nsecond" {
		t.Fatalf("tail = %q, want both lines without the trailing newline", tail)
	}
}

func TestReadStderrTailNoCompleteLine(t *testing.T) {
	tail, err := readStderrTail(writeTestLog(t, strings.Repeat("x", 3*stderrTailBytes)))
	if err != nil {
		t.Fatalf("readStderrTail: %v", err)
	}
	if tail != "" {
		t.Fatalf("a window with no complete line must return empty, got %d bytes", len(tail))
	}
}

func TestReadStderrTailMissingFileIsError(t *testing.T) {
	if _, err := readStderrTail(filepath.Join(t.TempDir(), "absent.log")); err == nil {
		t.Fatal("missing stderr log must be an error")
	}
}
