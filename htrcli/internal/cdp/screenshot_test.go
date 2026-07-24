package cdp

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/gorilla/websocket"
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
