package cdp

import (
	"encoding/json"
	"fmt"
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
			case strings.Contains(p.Expression, "__htrcliDom.exec"):
				conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
					"result": map[string]any{"type": "object", "value": map[string]any{"ok": map[string]any{
						"id": "1", "success": true, "data": map[string]any{"x": 120.5, "y": 240.0}}}}}})
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
			case strings.Contains(p.Expression, "__htrcliDom.exec"):
				conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
					"result": map[string]any{"type": "object", "value": map[string]any{"ok": map[string]any{
						"id": "1", "success": true, "data": map[string]any{"x": 120.5, "y": 240.0}}}}}})
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

	if err := Drag(s, "T1", DragEndpoint{Sel: coordinateSelector(10, 20)}, DragEndpoint{Sel: coordinateSelector(30, 40)}, 2, -1); err != nil {
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

// TestDragResolvesBothEndpointsInOnePass pins the fix for resolving the two
// drag endpoints independently: the source point was read, then the
// destination's prepareClick centered the page, and the press landed where
// the source USED to be. Both endpoints must now be resolved in a single
// page-side evaluation (dragPointsFn), so exactly one Runtime.evaluate is
// issued and the dispatched coordinates come from that one result.
func TestDragResolvesBothEndpointsInOnePass(t *testing.T) {
	var evals []string
	var calls []map[string]any
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		switch m.Method {
		case "Runtime.evaluate":
			var p struct {
				Expression string `json:"expression"`
			}
			json.Unmarshal(m.Params, &p)
			evals = append(evals, p.Expression)
			// The batched resolver returns both points in one envelope.
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"result": map[string]any{"type": "object", "value": map[string]any{
					"ok": map[string]any{
						"a": map[string]any{"x": 100.0, "y": 200.0},
						"b": map[string]any{"x": 300.0, "y": 400.0},
					}}}}})
		case "Input.dispatchMouseEvent":
			var params map[string]any
			if err := json.Unmarshal(m.Params, &params); err != nil {
				t.Errorf("decode drag params: %v", err)
			}
			calls = append(calls, params)
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
		default:
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
		}
	})
	s, _ := Dial(url)
	defer s.Close()

	src := DragEndpoint{Sel: &api.TargetSelector{Selector: "#src"}}
	dst := DragEndpoint{Sel: &api.TargetSelector{Selector: "#dst"}}
	if err := Drag(s, "T1", src, dst, 2, -1); err != nil {
		t.Fatalf("drag: %v", err)
	}

	// ONE page-side evaluation resolves both endpoints (no per-endpoint
	// prepareClick round trips whose scroll invalidates the earlier point),
	// and no frame conversion runs (frame == "").
	if len(evals) != 1 {
		t.Fatalf("issued %d Runtime.evaluate calls (want 1 batched resolve): %v", len(evals), evals)
	}
	if !strings.Contains(evals[0], "drag-prep-a") || !strings.Contains(evals[0], "drag-prep-b") {
		t.Errorf("batched expression must prepare BOTH endpoints, got: %s", evals[0])
	}
	// The bundle destructures {action, target} (lowercase); capitalised keys
	// made every selector drag fail with "Unknown action: undefined".
	if !strings.Contains(evals[0], "dom.exec({id: id, action: action, target: target})") {
		t.Errorf("drag resolver must call the bundle with lowercase command keys, got: %s", evals[0])
	}

	if len(calls) != 4 {
		t.Fatalf("want press, 2 moves, release; got %d: %#v", len(calls), calls)
	}
	if calls[0]["type"] != "mousePressed" || calls[0]["x"] != float64(100) || calls[0]["y"] != float64(200) {
		t.Errorf("press = %v, want mousePressed at (100,200)", calls[0])
	}
	last := calls[len(calls)-1]
	if last["type"] != "mouseReleased" || last["x"] != float64(300) || last["y"] != float64(400) {
		t.Errorf("release = %v, want mouseReleased at (300,400)", last)
	}
}

// TestDragRefEndpointsTravelAsElementHandles pins @eN drag endpoints: a ref
// is NOT pre-resolved to pixels (those went stale as soon as the other
// endpoint's prepare scrolled the page). The backendNodeId becomes a
// RemoteObject via DOM.resolveNode and the page-side resolver receives the
// ELEMENT through Runtime.callFunctionOn, so it is scrolled and re-read in
// the same pass as a selector endpoint.
func TestDragRefEndpointsTravelAsElementHandles(t *testing.T) {
	var resolved []int64
	var call map[string]any
	var calls []map[string]any
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		switch m.Method {
		case "DOM.resolveNode":
			var p struct {
				BackendNodeID int64 `json:"backendNodeId"`
			}
			json.Unmarshal(m.Params, &p)
			resolved = append(resolved, p.BackendNodeID)
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"object": map[string]any{"objectId": fmt.Sprintf("obj-%d", p.BackendNodeID), "className": "HTMLDivElement"}}})
		case "Runtime.callFunctionOn":
			json.Unmarshal(m.Params, &call)
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"result": map[string]any{"type": "object", "value": map[string]any{
					"a": map[string]any{"x": 11.0, "y": 22.0},
					"b": map[string]any{"x": 33.0, "y": 44.0},
				}}}})
		case "Runtime.evaluate":
			t.Errorf("a ref drag must not pre-resolve coordinates by evaluate")
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
		case "Input.dispatchMouseEvent":
			var params map[string]any
			json.Unmarshal(m.Params, &params)
			calls = append(calls, params)
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
		default:
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
		}
	})
	s, _ := Dial(url)
	defer s.Close()

	src := DragEndpoint{BackendNodeID: 100}
	dst := DragEndpoint{Sel: &api.TargetSelector{Selector: "#dropzone"}}
	if err := Drag(s, "T1", src, dst, 1, 0); err != nil {
		t.Fatalf("drag: %v", err)
	}
	if !reflect.DeepEqual(resolved, []int64{100}) {
		t.Fatalf("resolved backend nodes = %v, want [100]", resolved)
	}
	if call["objectId"] != "obj-100" || !strings.Contains(call["functionDeclaration"].(string), "drag-prep-a") {
		t.Fatalf("callFunctionOn must run dragPointsFn on the ref element, got %v", call)
	}
	args, _ := call["arguments"].([]any)
	if len(args) != 2 {
		t.Fatalf("arguments = %v, want [element handle, selector]", call["arguments"])
	}
	if a0, _ := args[0].(map[string]any); a0["objectId"] != "obj-100" || a0["value"] != nil {
		t.Errorf("source argument = %v, want the element handle", args[0])
	}
	if a1, _ := args[1].(map[string]any); a1["objectId"] != nil {
		t.Errorf("destination argument = %v, want the selector by value", args[1])
	} else if sel, _ := a1["value"].(map[string]any); sel["selector"] != "#dropzone" {
		t.Errorf("destination argument = %v, want selector #dropzone", args[1])
	}
	if len(calls) != 3 || calls[0]["x"] != float64(11) || calls[0]["y"] != float64(22) || calls[2]["x"] != float64(33) || calls[2]["y"] != float64(44) {
		t.Fatalf("dispatched = %v, want press at (11,22) … release at (33,44)", calls)
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
