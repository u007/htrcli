//go:build integration

package cdp

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// eventsTestPage logs to the console, throws an uncaught error, and fires a
// POST that 404s and a GET that 200s, then sets window.__done.
const eventsTestPage = `<!doctype html><html><body><script>
console.log("hello", 42, {a: 1});
console.warn("careful");
setTimeout(function () { throw new Error("boom"); }, 0);
Promise.all([
  fetch("/api/missing", {method: "POST", body: "x"}),
  fetch("/api/ok")
]).then(function () { setTimeout(function () { window.__done = true; }, 50); });
</script></body></html>`

func newEventsTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/ok", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok") })
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, eventsTestPage)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// startEventsBrowser launches headless Chrome on its own port (not
// TestCDPEndToEnd's 9223) with a throwaway HOME/profile.
func startEventsBrowser(t *testing.T, port int) string {
	t.Helper()
	chrome, err := FindChrome(os.Getenv("HTRCLI_CHROME_PATH"))
	if err != nil {
		t.Skipf("no Chrome available: %v", err)
	}
	t.Setenv("HOME", t.TempDir())
	st, err := StartBrowser(chrome, port, true)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() {
		if err := StopBrowser(); err != nil {
			t.Logf("stop: %v", err)
		}
		if p, kerr := os.FindProcess(st.PID); kerr == nil {
			_ = p.Kill()
		}
		_ = exec.Command("pkill", "-f", fmt.Sprintf("remote-debugging-port=%d", port)).Run()
		time.Sleep(500 * time.Millisecond)
	})
	return freshBlankTarget(t, port).WebSocketDebuggerURL
}

func dialT(t *testing.T, wsURL string) *Session {
	t.Helper()
	s, err := Dial(wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestCDPConsoleAndNetworkEvents(t *testing.T) {
	const port = 9224
	srv := newEventsTestServer(t)
	wsURL := startEventsBrowser(t, port)

	driver := dialT(t, wsURL)
	if err := Navigate(driver, srv.URL+"/", 10000); err != nil {
		t.Fatalf("navigate: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		done, err := Evaluate(driver, "window.__done === true")
		if err != nil {
			t.Fatalf("poll: %v", err)
		}
		if string(done) == "true" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("test page never finished its fetches")
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Run("console read replays history from a fresh session", func(t *testing.T) {
		s := dialT(t, wsURL)
		w, err := WatchConsole(s)
		if err != nil {
			t.Fatalf("watch: %v", err)
		}
		var log, warn, exc *ConsoleEntry
		for i := range w.Replayed() {
			e := &w.Replayed()[i]
			t.Logf("replayed: %+v", *e)
			switch {
			case e.Level == "log" && len(e.Args) > 0 && e.Args[0] == "hello":
				log = e
			case e.Level == "warn" && strings.Join(e.Args, " ") == "careful":
				warn = e
			case e.Level == "error" && len(e.Args) == 1 && strings.Contains(e.Args[0], "Uncaught Error: boom"):
				exc = e
			}
		}
		if log == nil || warn == nil || exc == nil {
			t.Fatalf("missing replayed entries: log=%v warn=%v exception=%v", log, warn, exc)
		}
		// V8 replays backlog messages without object previews, so objects
		// render as their description; live (watch) messages carry previews.
		if strings.Join(log.Args, "|") != `hello|42|Object` {
			t.Errorf("log args = %q", log.Args)
		}
		if !strings.HasPrefix(log.Source, srv.URL+"/:") || !strings.HasPrefix(exc.Source, srv.URL+"/:") {
			t.Errorf("sources = %q / %q, want url:line", log.Source, exc.Source)
		}
		if log.Timestamp < time.Now().Add(-time.Minute).UnixMilli() {
			t.Errorf("timestamp %d is not epoch ms", log.Timestamp)
		}
	})

	t.Run("network read lists completed requests with status", func(t *testing.T) {
		s := dialT(t, wsURL)
		entries, full, err := ReadPerformanceEntries(s)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if full {
			t.Error("buffer reported full for a 3-request page")
		}
		status := map[string]int{}
		for _, e := range entries {
			t.Logf("perf: %+v", e)
			status[e.URL] = e.Status
		}
		for url, want := range map[string]int{srv.URL + "/": 200, srv.URL + "/api/missing": 404, srv.URL + "/api/ok": 200} {
			if got, ok := status[url]; !ok || got != want {
				t.Errorf("%s: status %d (present=%v), want %d", url, got, ok, want)
			}
		}
	})

	t.Run("network watch sees new requests with method and status", func(t *testing.T) {
		s := dialT(t, wsURL)
		w, err := WatchNetwork(s)
		if err != nil {
			t.Fatalf("watch: %v", err)
		}
		// Trigger from a second client, as a separate htrcli invocation would.
		if _, err := Evaluate(driver, `Promise.all([fetch("/api/missing?w=1", {method: "POST", body: "y"}), fetch("/api/ok?w=1")]).then(() => 1)`); err != nil {
			t.Fatalf("trigger: %v", err)
		}
		var post, get *NetworkEntry
		end := time.Now().Add(5 * time.Second)
		for post == nil || get == nil {
			e, err := w.Next(end)
			if err != nil {
				t.Fatalf("next (post=%v get=%v): %v", post, get, err)
			}
			t.Logf("watched: %+v", *e)
			switch e.URL {
			case srv.URL + "/api/missing?w=1":
				post = e
			case srv.URL + "/api/ok?w=1":
				get = e
			}
		}
		if post.Method != "POST" || post.Status != 404 {
			t.Errorf("post = %+v", post)
		}
		if get.Method != "GET" || get.Status != 200 {
			t.Errorf("get = %+v", get)
		}
	})

	t.Run("console watch streams only new messages", func(t *testing.T) {
		s := dialT(t, wsURL)
		w, err := WatchConsole(s)
		if err != nil {
			t.Fatalf("watch: %v", err)
		}
		if len(w.Replayed()) == 0 {
			t.Fatal("expected a replayed backlog")
		}
		if _, err := Evaluate(driver, `console.error("after-attach", {k: "v"}); 1`); err != nil {
			t.Fatalf("trigger: %v", err)
		}
		e, err := w.Next(time.Now().Add(5 * time.Second))
		if err != nil {
			t.Fatalf("next: %v", err)
		}
		if e.Level != "error" || strings.Join(e.Args, " ") != `after-attach {k: "v"}` {
			t.Fatalf("first streamed entry = %+v, want the new message (replay must not repeat)", e)
		}
	})

	t.Run("watch ends with ErrEventDeadline when nothing matches", func(t *testing.T) {
		s := dialT(t, wsURL)
		w, err := WatchNetwork(s)
		if err != nil {
			t.Fatalf("watch: %v", err)
		}
		end := time.Now().Add(700 * time.Millisecond)
		for {
			e, err := w.Next(end)
			if errors.Is(err, ErrEventDeadline) {
				return
			}
			if err != nil {
				t.Fatalf("want ErrEventDeadline, got %v", err)
			}
			if strings.Contains(e.URL, "/never") {
				t.Fatalf("unexpected entry %+v", e)
			}
		}
	})
}
