package cdp

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func pngResult() map[string]any {
	return map[string]any{
		"data": base64.StdEncoding.EncodeToString([]byte("png")),
	}
}

func countMethod(methods []string, name string) int {
	n := 0
	for _, method := range methods {
		if method == name {
			n++
		}
	}
	return n
}

func indexOf(methods []string, name string) int {
	for i, method := range methods {
		if method == name {
			return i
		}
	}
	return -1
}

func lastIndexOf(methods []string, name string) int {
	for i := len(methods) - 1; i >= 0; i-- {
		if methods[i] == name {
			return i
		}
	}
	return -1
}

func TestScreenshotSuppressesControlStatusBarAroundCapture(t *testing.T) {
	var methods, expressions []string
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		methods = append(methods, m.Method)
		switch m.Method {
		case "Runtime.evaluate":
			var params struct {
				Expression string `json:"expression"`
			}
			json.Unmarshal(m.Params, &params)
			expressions = append(expressions, params.Expression)
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
		case "Page.captureScreenshot":
			conn.WriteJSON(map[string]any{"id": m.ID, "result": pngResult()})
		default:
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
		}
	})
	s, err := Dial(url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer s.Close()

	if _, err := Screenshot(s); err != nil {
		t.Fatalf("Screenshot: %v", err)
	}

	if len(methods) != 3 || methods[0] != "Runtime.evaluate" || methods[1] != "Page.captureScreenshot" || methods[2] != "Runtime.evaluate" {
		t.Fatalf("method order = %v, want [Runtime.evaluate Page.captureScreenshot Runtime.evaluate]", methods)
	}
	if len(expressions) != 2 {
		t.Fatalf("Runtime.evaluate count = %d, want 2", len(expressions))
	}
	if !strings.Contains(expressions[0], statusBarHostID) || !strings.Contains(expressions[0], "htrncontrolShotHolds") {
		t.Fatalf("hide expression = %q, want host id and shot counter", expressions[0])
	}
	if !strings.Contains(expressions[1], "htrncontrolShotHolds") {
		t.Fatalf("restore expression = %q, want shot counter", expressions[1])
	}
}

func TestScreenshotRestoresControlStatusBarWhenCaptureFails(t *testing.T) {
	var methods []string
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		methods = append(methods, m.Method)
		if m.Method == "Page.captureScreenshot" {
			conn.WriteJSON(map[string]any{"id": m.ID, "error": map[string]any{"code": -32000, "message": "capture failed"}})
			return
		}
		conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
	})
	s, _ := Dial(url)
	defer s.Close()

	if _, err := Screenshot(s); err == nil {
		t.Fatal("Screenshot succeeded, want capture error")
	}
	if got := countMethod(methods, "Runtime.evaluate"); got != 2 {
		t.Fatalf("Runtime.evaluate count = %d, want hide + restore", got)
	}
	if methods[len(methods)-1] != "Runtime.evaluate" {
		t.Fatalf("last method = %s, want the restore evaluate", methods[len(methods)-1])
	}
}

func TestScreenshotCapturesWhenStatusBarEvalFails(t *testing.T) {
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		if m.Method == "Runtime.evaluate" {
			// Browser-internal target or no execution context.
			conn.WriteJSON(map[string]any{"id": m.ID, "error": map[string]any{"code": -32000, "message": "no context"}})
			return
		}
		conn.WriteJSON(map[string]any{"id": m.ID, "result": pngResult()})
	})
	s, _ := Dial(url)
	defer s.Close()

	got, err := Screenshot(s)
	if err != nil {
		t.Fatalf("Screenshot must not fail when the optional suppression fails: %v", err)
	}
	if string(got) != "png" {
		t.Fatalf("bytes = %q, want png", got)
	}
}

func TestScreenshotFullPageSuppressesControlStatusBarAroundCapture(t *testing.T) {
	var methods []string
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		methods = append(methods, m.Method)
		switch m.Method {
		case "Page.getLayoutMetrics":
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"cssContentSize": map[string]any{"width": 800, "height": 600},
			}})
		case "Page.captureScreenshot":
			conn.WriteJSON(map[string]any{"id": m.ID, "result": pngResult()})
		default:
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
		}
	})
	s, _ := Dial(url)
	defer s.Close()

	if _, err := ScreenshotFullPage(s); err != nil {
		t.Fatalf("ScreenshotFullPage: %v", err)
	}
	if got := countMethod(methods, "Runtime.evaluate"); got != 2 {
		t.Fatalf("Runtime.evaluate count = %d, want hide + restore", got)
	}
	if countMethod(methods, "Page.captureScreenshot") != 1 {
		t.Fatalf("capture count = %d, want 1", countMethod(methods, "Page.captureScreenshot"))
	}
	hideAt := indexOf(methods, "Runtime.evaluate")
	captureAt := indexOf(methods, "Page.captureScreenshot")
	if hideAt < 0 || captureAt < 0 || hideAt > captureAt {
		t.Fatalf("hide at %d, capture at %d: want hide before capture", hideAt, captureAt)
	}
	if methods[len(methods)-1] != "Runtime.evaluate" {
		t.Fatalf("last method = %s, want the restore evaluate", methods[len(methods)-1])
	}
}

func TestStatusBarEvalIsBounded(t *testing.T) {
	old := statusBarCallTimeout
	statusBarCallTimeout = 20 * time.Millisecond
	defer func() { statusBarCallTimeout = old }()

	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		// Never answer Runtime.evaluate: the deadline must release the caller.
		if m.Method == "Runtime.evaluate" {
			return
		}
		conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
	})
	s, _ := Dial(url)
	defer s.Close()

	start := time.Now()
	hideControlStatusBar(s, holdShot)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("hide blocked for %s, want it bounded by %s", elapsed, statusBarCallTimeout)
	}
}

func TestScreencastStopRestoresRecordHold(t *testing.T) {
	var mu sync.Mutex
	var methods []string
	var hideExpr, restoreExpr string
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		mu.Lock()
		methods = append(methods, m.Method)
		if m.Method == "Runtime.evaluate" {
			var params struct {
				Expression string `json:"expression"`
			}
			json.Unmarshal(m.Params, &params)
			if strings.Contains(params.Expression, `display = "none"`) {
				hideExpr = params.Expression
			} else {
				restoreExpr = params.Expression
			}
		}
		mu.Unlock()
		conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
	})

	oldSession := pageSession
	pageSession = func(_ int, _ string) (*Session, error) { return Dial(url) }
	defer func() { pageSession = oldSession }()

	stop := make(chan os.Signal, 1)
	done := make(chan error, 1)
	go func() { done <- RunRecorder(1, "target", t.TempDir(), stop) }()

	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		started := countMethod(methods, "Page.startScreencast") > 0
		mu.Unlock()
		if started {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("screencast did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	close(stop)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunRecorder: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("RunRecorder did not stop")
	}

	mu.Lock()
	got := append([]string(nil), methods...)
	hide := hideExpr
	restore := restoreExpr
	mu.Unlock()

	hideAt := indexOf(got, "Runtime.evaluate")
	startAt := indexOf(got, "Page.startScreencast")
	stopAt := indexOf(got, "Page.stopScreencast")
	restoreAt := lastIndexOf(got, "Runtime.evaluate")
	if hideAt < 0 || startAt < 0 || stopAt < 0 || restoreAt < 0 {
		t.Fatalf("methods = %v, want hide, startScreencast, stopScreencast and restore", got)
	}
	if !(hideAt < startAt && startAt < stopAt && stopAt < restoreAt) {
		t.Fatalf("order = %v, want hide < startScreencast < stopScreencast < restore", got)
	}
	if !strings.Contains(hide, "htrncontrolRecordHolds") || !strings.Contains(restore, "htrncontrolRecordHolds") {
		t.Fatalf("hide = %q restore = %q, want the record hold attributes", hide, restore)
	}
}
