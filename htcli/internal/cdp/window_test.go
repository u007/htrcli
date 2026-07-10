package cdp

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

// fakeBrowserEndpoints serves /json, /json/version and a browser WS that
// records Browser.* calls.
func fakeBrowserEndpoints(t *testing.T, windowState string, calls *[]string) int {
	t.Helper()
	mux := http.NewServeMux()
	up := websocket.Upgrader{}
	mux.HandleFunc("/browser-ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()
		for {
			var m fakeMsg
			if err := conn.ReadJSON(&m); err != nil {
				return // intentionally not logged: client close ends fake loop
			}
			*calls = append(*calls, m.Method)
			switch m.Method {
			case "Browser.getWindowForTarget":
				conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{"windowId": 7}})
			case "Browser.getWindowBounds":
				conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{"bounds": map[string]any{"windowState": windowState}}})
			default:
				conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
			}
		}
	})
	var port int
	mux.HandleFunc("/json/version", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"webSocketDebuggerUrl":"ws://127.0.0.1:` + strconv.Itoa(port) + `/browser-ws"}`))
	})
	mux.HandleFunc("/json", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"id":"T1","type":"page","title":"t","url":"u","webSocketDebuggerUrl":"ws://127.0.0.1:` + strconv.Itoa(port) + `/page-ws"}]`))
	})
	port = testServer(t, mux) // helper from discover_test.go
	return port
}

func TestSetWindowStateMinimized(t *testing.T) {
	var calls []string
	port := fakeBrowserEndpoints(t, "normal", &calls)
	if err := SetWindowState(port, "", "minimized"); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	joined := strings.Join(calls, ",")
	if !strings.Contains(joined, "Browser.getWindowForTarget") || !strings.Contains(joined, "Browser.setWindowBounds") {
		t.Fatalf("calls = %v", calls)
	}
}

func TestGetWindowStateLive(t *testing.T) {
	var calls []string
	port := fakeBrowserEndpoints(t, "minimized", &calls)
	state, err := GetWindowState(port, "T1")
	if err != nil || state != "minimized" {
		t.Fatalf("want minimized, got %q (%v)", state, err)
	}
}
