package cdp

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/gorilla/websocket"
)

// TestCreateTargetOpensInBackground pins that `tabs new` does not steal focus:
// Target.createTarget must carry background: true, matching the extension path
// (chrome.tabs.create with active: false).
func TestCreateTargetOpensInBackground(t *testing.T) {
	got := make(chan map[string]any, 1)
	mux := http.NewServeMux()
	up := websocket.Upgrader{}
	var port int
	mux.HandleFunc("/browser-ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()
		var m fakeMsg
		if err := conn.ReadJSON(&m); err != nil {
			t.Errorf("read: %v", err)
			return
		}
		var params map[string]any
		if err := json.Unmarshal(m.Params, &params); err != nil {
			t.Errorf("decode params for %s: %v", m.Method, err)
		}
		got <- map[string]any{"method": m.Method, "params": params}
		conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{"targetId": "T9"}})
	})
	mux.HandleFunc("/json/version", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"webSocketDebuggerUrl":"ws://127.0.0.1:` + strconv.Itoa(port) + `/browser-ws"}`))
	})
	port = testServer(t, mux)

	id, err := CreateTarget(port, "https://example.com")
	if err != nil {
		t.Fatalf("CreateTarget: %v", err)
	}
	if id != "T9" {
		t.Fatalf("targetId = %q, want T9", id)
	}

	call := <-got
	if call["method"] != "Target.createTarget" {
		t.Fatalf("method = %v, want Target.createTarget", call["method"])
	}
	params, _ := call["params"].(map[string]any)
	if params["url"] != "https://example.com" || params["background"] != true {
		t.Fatalf("createTarget params = %v; want url and background: true", params)
	}
}
