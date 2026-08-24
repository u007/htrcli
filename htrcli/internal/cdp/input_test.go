package cdp

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/u007/htrcli/internal/api"
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
			case strings.Contains(p.Expression, "typeof window.__htrcliDom"):
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

func TestRightClickDispatchesTrustedInput(t *testing.T) {
	var methods []string
	var buttons []string
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		methods = append(methods, m.Method)
		if m.Method == "Runtime.evaluate" {
			var p struct {
				Expression string `json:"expression"`
			}
			json.Unmarshal(m.Params, &p)
			switch {
			case strings.Contains(p.Expression, "typeof window.__htrcliDom"):
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
		if m.Method == "Input.dispatchMouseEvent" {
			var p struct {
				Button string `json:"button"`
			}
			if err := json.Unmarshal(m.Params, &p); err != nil {
				t.Errorf("decoding mouse params: %v", err)
				return
			}
			buttons = append(buttons, p.Button)
		}
		conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
	})
	s, err := Dial(url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer s.Close()

	if err := Click(s, "T1", &api.TargetSelector{Selector: "#submit"}, "rightclick"); err != nil {
		t.Fatalf("rightclick: %v", err)
	}
	joined := strings.Join(methods, ",")
	for _, want := range []string{"Target.activateTarget", "Input.dispatchMouseEvent"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %s in %v", want, methods)
		}
	}
	if strings.Count(joined, "Input.dispatchMouseEvent") != 2 {
		t.Errorf("want exactly 2 mouse events, got %v", methods)
	}
	if got := strings.Join(buttons, ","); got != "right,right" {
		t.Errorf("want right button for both events, got %q", got)
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

func coordinateSelector(x, y float64) *api.TargetSelector {
	return &api.TargetSelector{X: &x, Y: &y}
}

func TestMousePrimitivesUseExactCDPPayloads(t *testing.T) {
	type wantCall struct {
		name   string
		params map[string]any
	}
	var calls []wantCall
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		if m.Method == "Input.dispatchMouseEvent" {
			var params map[string]any
			if err := json.Unmarshal(m.Params, &params); err != nil {
				t.Errorf("decode mouse params: %v", err)
			}
			calls = append(calls, wantCall{name: m.Method, params: params})
		}
		conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
	})
	s, err := Dial(url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer s.Close()

	if err := MouseDown(s, "T1", coordinateSelector(10, 20)); err != nil {
		t.Fatalf("mousedown: %v", err)
	}
	if err := MouseUp(s, "T1", coordinateSelector(30, 40)); err != nil {
		t.Fatalf("mouseup: %v", err)
	}
	if err := MouseMove(s, "T1", coordinateSelector(50, 60)); err != nil {
		t.Fatalf("mousemove: %v", err)
	}

	want := []map[string]any{
		{"type": "mousePressed", "x": float64(10), "y": float64(20), "button": "left", "clickCount": float64(1), "buttons": float64(1), "modifiers": float64(0)},
		{"type": "mouseReleased", "x": float64(30), "y": float64(40), "button": "left", "clickCount": float64(1), "buttons": float64(0), "modifiers": float64(0)},
		{"type": "mouseMoved", "x": float64(50), "y": float64(60), "button": "none", "clickCount": float64(0), "buttons": float64(0), "modifiers": float64(0)},
	}
	if len(calls) != len(want) {
		t.Fatalf("want %d mouse calls, got %d: %+v", len(want), len(calls), calls)
	}
	for i := range want {
		if !reflect.DeepEqual(calls[i].params, want[i]) {
			t.Errorf("call %d params = %#v, want %#v", i, calls[i].params, want[i])
		}
	}
}

func TestKeyDownAndKeyUpUseExactCDPPayloads(t *testing.T) {
	var calls []map[string]any
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		if m.Method == "Input.dispatchKeyEvent" {
			var params map[string]any
			if err := json.Unmarshal(m.Params, &params); err != nil {
				t.Errorf("decode key params: %v", err)
			}
			calls = append(calls, params)
		}
		conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
	})
	s, _ := Dial(url)
	defer s.Close()

	if err := KeyDown(s, "T1", "Ctrl+Enter"); err != nil {
		t.Fatalf("keydown: %v", err)
	}
	if err := KeyUp(s, "T1", "Ctrl+Enter"); err != nil {
		t.Fatalf("keyup: %v", err)
	}
	wantDown := map[string]any{
		"type": "keyDown", "key": "Enter", "modifiers": float64(2),
		"windowsVirtualKeyCode": float64(13), "code": "Enter", "text": "\r",
	}
	wantUp := map[string]any{
		"type": "keyUp", "key": "Enter", "modifiers": float64(2),
		"windowsVirtualKeyCode": float64(13), "code": "Enter",
	}
	if !reflect.DeepEqual(calls, []map[string]any{wantDown, wantUp}) {
		t.Fatalf("key payloads = %#v, want %#v", calls, []map[string]any{wantDown, wantUp})
	}
}

func TestDragUsesCoordinatesAndBoundedSteps(t *testing.T) {
	var calls []map[string]any
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		if m.Method == "Input.dispatchMouseEvent" {
			var params map[string]any
			if err := json.Unmarshal(m.Params, &params); err != nil {
				t.Errorf("decode drag params: %v", err)
			}
			calls = append(calls, params)
		}
		conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
	})
	s, _ := Dial(url)
	defer s.Close()

	if err := Drag(s, "T1", coordinateSelector(10, 20), coordinateSelector(30, 40), 2, -1); err != nil {
		t.Fatalf("drag: %v", err)
	}
	if len(calls) != 4 {
		t.Fatalf("want press, 2 moves, release; got %d calls: %#v", len(calls), calls)
	}
	wantTypes := []string{"mousePressed", "mouseMoved", "mouseMoved", "mouseReleased"}
	for i, wantType := range wantTypes {
		if calls[i]["type"] != wantType {
			t.Errorf("call %d type = %v, want %s", i, calls[i]["type"], wantType)
		}
	}
	if calls[1]["x"] != float64(20) || calls[1]["y"] != float64(30) {
		t.Errorf("first move = (%v,%v), want (20,30)", calls[1]["x"], calls[1]["y"])
	}
	if calls[2]["x"] != float64(30) || calls[2]["y"] != float64(40) {
		t.Errorf("second move = (%v,%v), want (30,40)", calls[2]["x"], calls[2]["y"])
	}
}

func TestDragNormalizationBounds(t *testing.T) {
	if got := normalizeDragSteps(0); got != defaultDragSteps {
		t.Errorf("default steps = %d, want %d", got, defaultDragSteps)
	}
	if got := normalizeDragSteps(101); got != maxDragSteps {
		t.Errorf("clamped steps = %d, want %d", got, maxDragSteps)
	}
	if got := normalizeDragDelay(-1); got != 0 {
		t.Errorf("negative delay = %d, want 0", got)
	}
	if got := normalizeDragDelay(maxDragDelayMs + 1); got != maxDragDelayMs {
		t.Errorf("clamped delay = %d, want %d", got, maxDragDelayMs)
	}
}

func TestKeyInputRejectsInvalidSpecs(t *testing.T) {
	for _, keySpec := range []string{"", "+", "Ctrl+"} {
		if _, _, err := parseKey(keySpec); err == nil {
			t.Errorf("parseKey(%q) succeeded, want error", keySpec)
		}
	}
}
