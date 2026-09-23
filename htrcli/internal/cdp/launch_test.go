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
	"strings"
	"testing"
	"time"
)

func TestLaunchArgs(t *testing.T) {
	args := LaunchArgs(9222, "/home/u/.htrcli/chrome-profile", false)
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
	args := LaunchArgs(9333, "/p", true)
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
	// Can't simulate "both missing" when the system Chrome exists at the
	// default macOS path; skip so the test still validates the error elsewhere.
	if _, err := os.Stat(macChromePath); err == nil {
		t.Skipf("system Chrome present at %s; cannot test missing-path error", macChromePath)
	}
	_, err := FindChrome(filepath.Join(t.TempDir(), "nope"))
	if err == nil || !strings.Contains(err.Error(), "set-chrome-path") {
		t.Fatalf("error must mention config set-chrome-path, got %v", err)
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
	want := &BrowserState{PID: 42, Port: 9222, Headless: true}
	if err := writeState(want); err != nil {
		t.Fatal(err)
	}
	got, err := ReadState()
	if err != nil || got == nil || got.PID != 42 || !got.Headless {
		t.Fatalf("round trip failed: %v %v", got, err)
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

			st, err := StartBrowser("ignored-chrome-path", port, false)
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

	if _, err := StartBrowser("ignored-chrome-path", port, false); err == nil {
		t.Fatal("want an error when the listener pid cannot be resolved")
	}
	// Must not have persisted a meaningless PID-0 state.
	if st, err := ReadState(); err != nil || st != nil {
		t.Fatalf("state must stay absent, got %+v err %v", st, err)
	}
}

func TestLaunchChromeTimeoutReapsChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell script as the fake Chrome")
	}
	dir := t.TempDir()
	fake := filepath.Join(dir, "fake-chrome")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexec sleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	port, err := freePort()
	if err != nil {
		t.Fatal(err)
	}
	origTimeout := launchTimeout
	t.Cleanup(func() { launchTimeout = origTimeout })
	launchTimeout = 400 * time.Millisecond

	start := time.Now()
	_, err = launchChrome(fake, port, filepath.Join(dir, "profile"), true)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("want a timeout error")
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
