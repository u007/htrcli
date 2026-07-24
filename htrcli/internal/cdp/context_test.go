package cdp

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

func TestEnsureContextCleansUpWhenRegistryWriteFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	origLaunch := launchChromeFn
	origUpsert := upsertContextFn
	origTerminate := terminateProcessFn
	defer func() {
		launchChromeFn = origLaunch
		upsertContextFn = origUpsert
		terminateProcessFn = origTerminate
	}()

	launchChromeFn = func(_ string, port int, _ string, _ bool) (int, error) {
		return 4242, nil
	}
	upsertContextFn = func(ContextEntry) error {
		return fmt.Errorf("disk full")
	}
	called := false
	terminateProcessFn = func(pid int) error {
		called = true
		if pid != 4242 {
			t.Fatalf("terminateProcessFn pid = %d, want 4242", pid)
		}
		return nil
	}

	port, err := EnsureContext("work", "/tmp/chrome", false)
	if err == nil {
		t.Fatal("expected EnsureContext to fail when registry write fails")
	}
	if port != 0 {
		t.Fatalf("expected zero port on failure, got %d", port)
	}
	if !called {
		t.Fatal("expected cleanup to terminate the newly launched process")
	}
}
