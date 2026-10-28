package cdp

import (
	"encoding/json"
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

func TestHistoryTarget(t *testing.T) {
	cases := []struct {
		name              string
		current, n, delta int
		want              int
		wantErr           string
	}{
		{"back from second entry", 1, 2, -1, 0, ""},
		{"forward from first entry", 0, 2, 1, 1, ""},
		{"back past the first entry", 0, 2, -1, 0, "no history entry to go back to"},
		{"forward past the last entry", 1, 2, 1, 0, "no history entry to go forward to"},
		{"back with a single entry", 0, 1, -1, 0, "no history entry to go back to"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := historyTarget(tc.current, tc.n, tc.delta)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("target = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestHistoryStepNavigatesToPreviousEntry(t *testing.T) {
	var mu sync.Mutex
	var navigatedEntry float64
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		switch m.Method {
		case "Page.getNavigationHistory":
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"currentIndex": 1,
				"entries": []map[string]any{
					{"id": 10, "url": "https://example.com/"},
					{"id": 11, "url": "https://example.org/"},
				},
			}})
		case "Page.navigateToHistoryEntry":
			var params struct {
				EntryID float64 `json:"entryId"`
			}
			json.Unmarshal(m.Params, &params)
			mu.Lock()
			navigatedEntry = params.EntryID
			mu.Unlock()
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
		case "Runtime.evaluate":
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"result": map[string]any{"type": "object", "value": map[string]any{"href": "https://example.com/", "state": "complete"}},
			}})
		default:
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
		}
	})

	s, err := Dial(url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer s.Close()

	if err := HistoryStep(s, -1, 2000); err != nil {
		t.Fatalf("HistoryStep: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if navigatedEntry != 10 {
		t.Fatalf("navigated to entry %v, want 10 (the entry before currentIndex 1)", navigatedEntry)
	}
}

func TestHistoryStepErrorsWithoutNavigatingAtStartOfHistory(t *testing.T) {
	var mu sync.Mutex
	navigated := false
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		switch m.Method {
		case "Page.getNavigationHistory":
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"currentIndex": 0,
				"entries":      []map[string]any{{"id": 10, "url": "https://example.com/"}},
			}})
		case "Page.navigateToHistoryEntry":
			mu.Lock()
			navigated = true
			mu.Unlock()
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
		default:
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
		}
	})

	s, err := Dial(url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer s.Close()

	err = HistoryStep(s, -1, 2000)
	if err == nil || !strings.Contains(err.Error(), "no history entry to go back to") {
		t.Fatalf("err = %v, want no-history-entry error", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if navigated {
		t.Fatalf("Page.navigateToHistoryEntry was sent although there is no entry to go back to")
	}
}

func TestHistoryStepAcceptsDestinationThatRewritesItsURL(t *testing.T) {
	var mu sync.Mutex
	navigated := false
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		switch m.Method {
		case "Page.getNavigationHistory":
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"currentIndex": 1,
				"entries": []map[string]any{
					{"id": 10, "url": "https://example.com/"},
					{"id": 11, "url": "https://example.org/"},
				},
			}})
		case "Page.navigateToHistoryEntry":
			mu.Lock()
			navigated = true
			mu.Unlock()
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
		case "Runtime.evaluate":
			// Before the step the page is on example.org. Once the entry is
			// navigated to, it completes on a URL the entry does not name.
			mu.Lock()
			done := navigated
			mu.Unlock()
			href := "https://example.org/"
			if done {
				href = "https://example.com/?nonce=7"
			}
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"result": map[string]any{"type": "object", "value": map[string]any{"href": href, "state": "complete"}},
			}})
		default:
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
		}
	})

	s, err := Dial(url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer s.Close()

	if err := HistoryStep(s, -1, 2000); err != nil {
		t.Fatalf("HistoryStep must accept a page that loaded at a rewritten URL: %v", err)
	}
}

func TestHistoryStepTimesOutWhenDocumentNeverCompletes(t *testing.T) {
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		switch m.Method {
		case "Page.getNavigationHistory":
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"currentIndex": 1,
				"entries": []map[string]any{
					{"id": 10, "url": "https://example.com/"},
					{"id": 11, "url": "https://example.org/"},
				},
			}})
		case "Runtime.evaluate":
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"result": map[string]any{"type": "object", "value": map[string]any{"href": "https://example.org/", "state": "complete"}},
			}})
		default:
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
		}
	})

	s, err := Dial(url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer s.Close()

	err = HistoryStep(s, -1, 150)
	if err == nil || !strings.Contains(err.Error(), "page did not finish loading") {
		t.Fatalf("err = %v, want a load timeout when the document stays on the wrong URL", err)
	}
}
