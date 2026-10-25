package cdp

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestReloadSendsPageReload pins the verb: reload must go through Page.reload,
// never through a repeat Page.navigate, because a repeat navigate of a cached
// or file:// URL can be served as a same-document navigation that keeps the
// page's JS state alive — which would make any "this survived a reload"
// benchmark assertion vacuously true.
func TestReloadSendsPageReload(t *testing.T) {
	var mu sync.Mutex
	var methods []string

	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		mu.Lock()
		methods = append(methods, m.Method)
		mu.Unlock()

		if m.ID != 0 {
			// intentionally not logged: a fake-server write to a closing socket is expected at teardown
			_ = conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
		}
		if m.Method == "Page.reload" {
			// intentionally not logged: same teardown race as above
			_ = conn.WriteJSON(map[string]any{
				"method": "Page.loadEventFired",
				"params": map[string]any{"timestamp": 1},
			})
		}
	})

	s, err := Dial(url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer s.Close()

	if err := Reload(s, 2000); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	var sawReload, sawNavigate bool
	for _, m := range methods {
		switch m {
		case "Page.reload":
			sawReload = true
		case "Page.navigate":
			sawNavigate = true
		}
	}
	if !sawReload {
		t.Fatalf("Page.reload was never sent; calls=%v", methods)
	}
	if sawNavigate {
		t.Fatalf("reload must not navigate the page; calls=%v", methods)
	}
}

// TestReloadWaitsForLoadEvent is the half that matters: a Reload that fired
// Page.reload and returned immediately would let the next step run against the
// pre-reload document, so the caller must block until the load event arrives.
func TestReloadWaitsForLoadEvent(t *testing.T) {
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		if m.ID != 0 {
			// intentionally not logged: fake-server write during teardown
			_ = conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
		}
		// Deliberately no Page.loadEventFired.
	})

	s, err := Dial(url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer s.Close()

	start := time.Now()
	err = Reload(s, 150)
	if err == nil {
		t.Fatal("Reload returned before the load event — it does not wait")
	}
	if !strings.Contains(err.Error(), "did not finish reloading") {
		t.Fatalf("want a reload timeout error, got %v", err)
	}
	if elapsed := time.Since(start); elapsed < 100*time.Millisecond {
		t.Fatalf("gave up after %s — it never waited for the event", elapsed)
	}
}
