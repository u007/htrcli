package cdp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/u007/htcli/internal/api"
)

// clickFake answers prepare-exec with coords and records Input.* dispatches.
func clickFake(t *testing.T, methods *[]string) string {
	return fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		*methods = append(*methods, m.Method)
		if m.Method == "Runtime.evaluate" {
			var p struct {
				Expression string `json:"expression"`
			}
			json.Unmarshal(m.Params, &p)
			switch {
			case strings.Contains(p.Expression, "typeof window.__htcliDom"):
				conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
					"result": map[string]any{"type": "string", "value": "object"}}})
			case strings.Contains(p.Expression, "prepareClick"):
				conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
					"result": map[string]any{"type": "object", "value": map[string]any{
						"id": "1", "success": true, "data": map[string]any{"x": 120.5, "y": 240.0}}}}})
			default:
				conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
					"result": map[string]any{"type": "undefined"}}})
			}
			return
		}
		conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
	})
}

func TestClickDispatchesTrustedInput(t *testing.T) {
	var methods []string
	url := clickFake(t, &methods)
	s, err := Dial(url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer s.Close()

	if err := Click(s, "T1", &api.TargetSelector{Selector: "#submit"}, "click"); err != nil {
		t.Fatalf("click: %v", err)
	}
	joined := strings.Join(methods, ",")
	for _, want := range []string{"Target.activateTarget", "Input.dispatchMouseEvent"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %s in %v", want, methods)
		}
	}
	// pressed + released
	if strings.Count(joined, "Input.dispatchMouseEvent") != 2 {
		t.Errorf("want exactly 2 mouse events, got %v", methods)
	}
}

func TestPressEnter(t *testing.T) {
	var methods []string
	url := clickFake(t, &methods)
	s, _ := Dial(url)
	defer s.Close()

	if err := Press(s, "T1", "Enter"); err != nil {
		t.Fatalf("press: %v", err)
	}
	joined := strings.Join(methods, ",")
	if !strings.Contains(joined, "Target.activateTarget") {
		t.Errorf("press must activate the target before dispatch, got %v", methods)
	}
	if strings.Count(joined, "Input.dispatchKeyEvent") != 2 {
		t.Errorf("want keyDown+keyUp, got %v", methods)
	}
}

func TestPressEnterKeyParams(t *testing.T) {
	var downParams map[string]any
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		if m.Method == "Input.dispatchKeyEvent" && downParams == nil {
			if err := json.Unmarshal(m.Params, &downParams); err != nil {
				t.Errorf("decoding key params: %v", err)
			}
		}
		conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
	})
	s, _ := Dial(url)
	defer s.Close()

	if err := Press(s, "T1", "Enter"); err != nil {
		t.Fatalf("press: %v", err)
	}
	// Enter must carry code, keycode, and "\r" text so keypress/submit
	// handlers fire (matches src/utils/keyMap.ts).
	if downParams["code"] != "Enter" || downParams["text"] != "\r" {
		t.Errorf("keyDown params missing code/text: %v", downParams)
	}
	if kc, ok := downParams["windowsVirtualKeyCode"].(float64); !ok || kc != 13 {
		t.Errorf("want windowsVirtualKeyCode 13, got %v", downParams["windowsVirtualKeyCode"])
	}
}

func TestNavigateWaitsForLoad(t *testing.T) {
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
		if m.Method == "Page.navigate" {
			conn.WriteJSON(map[string]any{"method": "Page.loadEventFired", "params": map[string]any{"timestamp": 1}})
		}
	})
	s, _ := Dial(url)
	defer s.Close()

	if err := Navigate(s, "https://example.com/", 5000); err != nil {
		t.Fatalf("navigate: %v", err)
	}
}
