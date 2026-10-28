package cdp

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/u007/htrcli/internal/api"
)

func TestScreenshotFullPageClipsToContentSize(t *testing.T) {
	wantPNG := []byte("full-page-png-bytes")
	var captureParams map[string]any
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		switch m.Method {
		case "Page.enable":
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
		case "Page.getLayoutMetrics":
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"cssContentSize": map[string]any{"x": 0, "y": 0, "width": 1280, "height": 3200},
			}})
		case "Page.captureScreenshot":
			json.Unmarshal(m.Params, &captureParams)
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"data": base64.StdEncoding.EncodeToString(wantPNG),
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

	got, err := ScreenshotFullPage(s)
	if err != nil {
		t.Fatalf("ScreenshotFullPage: %v", err)
	}
	if string(got) != string(wantPNG) {
		t.Fatalf("decoded bytes = %q, want %q", got, wantPNG)
	}
	if captureParams["captureBeyondViewport"] != true {
		t.Fatalf("captureBeyondViewport = %v, want true", captureParams["captureBeyondViewport"])
	}
	clip, ok := captureParams["clip"].(map[string]any)
	if !ok {
		t.Fatalf("clip missing or wrong type: %v", captureParams["clip"])
	}
	if clip["width"] != float64(1280) || clip["height"] != float64(3200) {
		t.Fatalf("clip = %v, want width 1280 height 3200", clip)
	}
}

func TestScreenshotFullPageErrorsOnZeroContentSize(t *testing.T) {
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		switch m.Method {
		case "Page.getLayoutMetrics":
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"cssContentSize": map[string]any{"width": 0, "height": 0},
			}})
		default:
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
		}
	})
	s, _ := Dial(url)
	defer s.Close()

	if _, err := ScreenshotFullPage(s); err == nil {
		t.Fatal("expected error on zero content size, got nil")
	}
}

// fakeBundleResults answers each Runtime.evaluate with the next entry of
// results as the DOM bundle's command result (the envelope runScoped unwraps).
func fakeBundleResults(t *testing.T, results []any) string {
	t.Helper()
	next := 0
	return fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		if m.Method != "Runtime.evaluate" {
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
			return
		}
		if next >= len(results) {
			t.Errorf("more bundle calls than expected (%d)", len(results))
			return
		}
		cmd := map[string]any{"id": "annotate", "success": true, "data": results[next]}
		next++
		conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
			"result": map[string]any{"type": "object", "value": map[string]any{"ok": cmd}},
		}})
	})
}

func TestAnnotationBoxesNumbersInArgumentOrder(t *testing.T) {
	url := fakeBundleResults(t, []any{
		map[string]any{"x": 10, "y": 20, "width": 100, "height": 50},
		map[string]any{"x": 0, "y": 300, "width": 8, "height": 8},
	})
	s, err := Dial(url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer s.Close()

	boxes, err := AnnotationBoxes(s, []api.TargetSelector{{Selector: "#a"}, {Selector: "#b"}})
	if err != nil {
		t.Fatalf("AnnotationBoxes: %v", err)
	}
	want := []AnnotationBox{
		{Index: 1, X: 10, Y: 20, Width: 100, Height: 50},
		{Index: 2, X: 0, Y: 300, Width: 8, Height: 8},
	}
	if len(boxes) != len(want) {
		t.Fatalf("got %d boxes, want %d", len(boxes), len(want))
	}
	for i := range want {
		if boxes[i] != want[i] {
			t.Fatalf("box %d = %+v, want %+v", i, boxes[i], want[i])
		}
	}
}

func TestAnnotationBoxesNamesSelectorThatMatchesNothing(t *testing.T) {
	url := fakeBundleResults(t, []any{nil})
	s, err := Dial(url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer s.Close()

	_, err = AnnotationBoxes(s, []api.TargetSelector{{Selector: "#missing"}})
	if err == nil || !strings.Contains(err.Error(), "#missing") {
		t.Fatalf("err = %v, want an error naming #missing", err)
	}
}

func TestAnnotateScriptCarriesIndexAndMarker(t *testing.T) {
	script, err := AnnotateScript([]AnnotationBox{
		{Index: 1, X: 10, Y: 20, Width: 100, Height: 50},
		{Index: 2, X: 0, Y: 0, Width: 8, Height: 8},
	})
	if err != nil {
		t.Fatalf("AnnotateScript: %v", err)
	}
	for _, want := range []string{`"index":1`, `"index":2`, `"x":10`, `"width":100`, AnnotationMarkerAttr} {
		if !strings.Contains(script, want) {
			t.Fatalf("script does not contain %q:\n%s", want, script)
		}
	}
}

func TestAnnotateClearScriptTargetsMarker(t *testing.T) {
	script := annotateClearScript()
	if !strings.Contains(script, "["+AnnotationMarkerAttr+"]") {
		t.Fatalf("clear script does not select by marker attribute:\n%s", script)
	}
}
