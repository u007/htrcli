package commands

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/spf13/viper"
	"github.com/u007/htrcli/internal/api"
	"github.com/u007/htrcli/internal/cdp"
)

func TestRunInteractCDPFailsClearlyWhenNotRunning(t *testing.T) {
	resetTransportState()
	cdpFlag = true
	defer resetTransportState()
	// Point at a dead port so PageSession fails fast.
	setViperCDPPort(t, 1)

	err := runInteractCDP("fill", "#email", "x")
	if err == nil || !strings.Contains(err.Error(), "htrcli browser start") {
		t.Fatalf("want ErrNotRunning guidance, got %v", err)
	}
}

func setViperCDPPort(t *testing.T, port int) {
	t.Helper()
	viper.Set("cdp-port", port)
	t.Cleanup(func() { viper.Set("cdp-port", 0) })
}

func TestRunDragCDPPassesPersistentRefsAsElementHandles(t *testing.T) {
	var resolved []int64
	var dispatched int
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()
		for {
			var request struct {
				ID     int64           `json:"id"`
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			}
			if err := conn.ReadJSON(&request); err != nil {
				return
			}
			result := map[string]any{}
			switch request.Method {
			case "DOM.resolveNode":
				var params struct {
					BackendNodeID int64 `json:"backendNodeId"`
				}
				_ = json.Unmarshal(request.Params, &params)
				resolved = append(resolved, params.BackendNodeID)
				result = map[string]any{"object": map[string]any{"objectId": fmt.Sprintf("obj-%d", params.BackendNodeID)}}
			case "Runtime.callFunctionOn":
				// The page-side resolver scrolls both ELEMENTS, then reads them.
				result = map[string]any{"result": map[string]any{"type": "object", "value": map[string]any{
					"a": map[string]any{"x": 105, "y": 15}, "b": map[string]any{"x": 205, "y": 15}}}}
			case "Input.dispatchMouseEvent":
				dispatched++
			case "DOM.getBoxModel", "Page.getLayoutMetrics":
				t.Errorf("%s must not be called: refs are no longer pre-resolved to stale pixels", request.Method)
			}
			if err := conn.WriteJSON(map[string]any{"id": request.ID, "result": result}); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	s, err := cdp.Dial("ws" + strings.TrimPrefix(server.URL, "http"))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer s.Close()
	refStorePathOverride = t.TempDir() + "/refs.json"
	defer func() { refStorePathOverride = "" }()
	refs, err := LoadRefStore()
	if err != nil {
		t.Fatalf("load refs: %v", err)
	}
	refs.Refs = map[string]int64{"@e1": 100, "@e2": 200}
	if err := refs.Save(); err != nil {
		t.Fatalf("save refs: %v", err)
	}

	if err := runDragCDP(s, "T1", "@e1", "@e2", 1, 0); err != nil {
		t.Fatalf("runDragCDP: %v", err)
	}
	if len(resolved) != 2 || resolved[0] != 100 || resolved[1] != 200 {
		t.Fatalf("resolved backend nodes = %v, want [100 200]", resolved)
	}
	if dispatched != 3 {
		t.Fatalf("mouse events = %d, want press, move, release", dispatched)
	}
}

func TestResolveDragTargetCDPReportsStaleRefs(t *testing.T) {
	refs := &RefStore{Refs: map[string]int64{"@e1": 100}}
	for _, ref := range []string{"@e2", "@e999"} {
		_, err := resolveDragTargetCDP(refs, &api.TargetSelector{Ref: ref})
		if err == nil || !strings.Contains(err.Error(), "stale ref") {
			t.Fatalf("resolve %s error = %v, want stale ref error", ref, err)
		}
	}
}

func TestScrollDelta(t *testing.T) {
	cases := []struct {
		direction string
		pixels    int
		dx, dy    int
		wantErr   bool
	}{
		{"down", 300, 0, 300, false},
		{"up", 300, 0, -300, false},
		{"left", 50, -50, 0, false},
		{"right", 50, 50, 0, false},
		{"sideways", 50, 0, 0, true},
	}
	for _, tc := range cases {
		dx, dy, err := scrollDelta(tc.direction, tc.pixels)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("scrollDelta(%q) error = nil, want error", tc.direction)
			}
			continue
		}
		if err != nil {
			t.Fatalf("scrollDelta(%q): %v", tc.direction, err)
		}
		if dx != tc.dx || dy != tc.dy {
			t.Fatalf("scrollDelta(%q, %d) = (%d, %d), want (%d, %d)", tc.direction, tc.pixels, dx, dy, tc.dx, tc.dy)
		}
	}
}

// runFindRefCDP must record the CDP target that served the DOM query alongside
// the document, because a backendNodeId is only meaningful inside that target.
// The mint here resolves its target from the browser's target list (no --tab),
// which is the default path an operator takes.
func TestRunFindRefCDPRecordsMintTarget(t *testing.T) {
	resetTransportState()
	cdpFlag = true
	defer resetTransportState()
	tabTarget = ""
	defer func() { tabTarget = "" }()
	useEmulationStore(t)
	refStorePathOverride = t.TempDir() + "/refs.json"
	defer func() { refStorePathOverride = "" }()

	upgrader := websocket.Upgrader{}
	mux := http.NewServeMux()
	mux.HandleFunc("/json", func(w http.ResponseWriter, r *http.Request) {
		targets := []map[string]any{{
			"id":                   "TAB-A",
			"type":                 "page",
			"title":                "fixture",
			"url":                  "https://example.test/list",
			"webSocketDebuggerUrl": "ws://" + r.Host + "/devtools/page/TAB-A",
		}}
		if err := json.NewEncoder(w).Encode(targets); err != nil {
			t.Errorf("encode targets: %v", err)
		}
	})
	mux.HandleFunc("/devtools/page/TAB-A", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()
		for {
			var req struct {
				ID     int64           `json:"id"`
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			}
			if err := conn.ReadJSON(&req); err != nil {
				return // intentionally not logged: the client closing the socket ends the fake browser loop
			}
			result := map[string]any{}
			switch req.Method {
			case "Page.getFrameTree":
				result = frameTreeResult("loader-A", "https://example.test/list")
			case "DOM.getDocument":
				result = map[string]any{"root": map[string]any{"nodeId": 1}}
			case "DOM.querySelector":
				result = map[string]any{"nodeId": 2}
			case "DOM.describeNode":
				result = map[string]any{"node": map[string]any{"backendNodeId": 4242}}
			}
			if err := conn.WriteJSON(map[string]any{"id": req.ID, "result": result}); err != nil {
				return // intentionally not logged: the client closing the socket ends the fake browser loop
			}
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse fake browser URL: %v", err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("fake browser port: %v", err)
	}
	setViperCDPPort(t, port)

	if err := runFindRefCDP("#go", false); err != nil {
		t.Fatalf("runFindRefCDP: %v", err)
	}

	rs, err := LoadRefStore()
	if err != nil {
		t.Fatalf("load ref store: %v", err)
	}
	if id, ok := rs.Lookup("@e1"); !ok || id != 4242 {
		t.Fatalf("backendNodeId for @e1 = %d,%v, want 4242,true", id, ok)
	}
	if targetID, ok := rs.LookupTarget("@e1"); !ok || targetID != "TAB-A" {
		t.Fatalf("LookupTarget(@e1) = %q,%v, want TAB-A,true", targetID, ok)
	}
	if loaderID, ok := rs.LookupDocument("@e1"); !ok || loaderID != "loader-A" {
		t.Fatalf("LookupDocument(@e1) = %q,%v, want loader-A,true", loaderID, ok)
	}
}
