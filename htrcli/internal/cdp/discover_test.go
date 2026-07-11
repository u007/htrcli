package cdp

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
)

func testServer(t *testing.T, mux *http.ServeMux) int {
	t.Helper()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())
	return port
}

func TestListTargetsFiltersPages(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/json", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[
			{"id":"AAA1","type":"page","title":"Privacy","url":"https://x/","webSocketDebuggerUrl":"ws://h/devtools/page/AAA1"},
			{"id":"BBB2","type":"iframe","title":"f","url":"https://y/","webSocketDebuggerUrl":"ws://h/devtools/page/BBB2"}
		]`))
	})
	port := testServer(t, mux)

	targets, err := ListTargets(port)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(targets) != 1 || targets[0].ID != "AAA1" {
		t.Fatalf("want only page target AAA1, got %+v", targets)
	}
}

func TestBrowserWSURL(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/json/version", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"Browser":"Chrome/140.0","webSocketDebuggerUrl":"ws://127.0.0.1:9222/devtools/browser/abc"}`))
	})
	port := testServer(t, mux)

	got, err := BrowserWSURL(port)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "ws://127.0.0.1:9222/devtools/browser/abc" {
		t.Fatalf("got %q", got)
	}
}

func TestListTargetsNotRunning(t *testing.T) {
	// Port 1 is never listening.
	_, err := ListTargets(1)
	if !errors.Is(err, ErrNotRunning) {
		t.Fatalf("want ErrNotRunning, got %v", err)
	}
}
