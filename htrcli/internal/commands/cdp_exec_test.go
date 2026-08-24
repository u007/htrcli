package commands

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestRunDragCDPResolvesBothPersistentRefsToCoordinates(t *testing.T) {
	var boxCalls int
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
			case "DOM.getBoxModel":
				boxCalls++
				var params struct {
					BackendNodeID int64 `json:"backendNodeId"`
				}
				_ = json.Unmarshal(request.Params, &params)
				left := float64(params.BackendNodeID)
				result = map[string]any{"model": map[string]any{
					"content": []float64{left, 10, left + 10, 10, left + 10, 20, left, 20},
				}}
			case "Page.getLayoutMetrics":
				result = map[string]any{"visualViewport": map[string]any{"pageX": 0, "pageY": 0}}
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
	if boxCalls != 2 {
		t.Fatalf("getBoxModel calls = %d, want 2", boxCalls)
	}
}

func TestResolveDragTargetCDPReportsStaleRefs(t *testing.T) {
	refs := &RefStore{Refs: map[string]int64{"@e1": 100}}
	for _, ref := range []string{"@e2", "@e999"} {
		_, err := resolveDragTargetCDP(nil, refs, &api.TargetSelector{Ref: ref})
		if err == nil || !strings.Contains(err.Error(), "stale ref") {
			t.Fatalf("resolve %s error = %v, want stale ref error", ref, err)
		}
	}
}
