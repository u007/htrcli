package cdp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/u007/htrcli/internal/api"
)

func TestExecDOMInstallsBundleOnce(t *testing.T) {
	var expressions []string
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		if m.Method != "Runtime.evaluate" {
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
			return
		}
		var p struct {
			Expression string `json:"expression"`
		}
		json.Unmarshal(m.Params, &p)
		expressions = append(expressions, p.Expression)
		switch {
		case strings.Contains(p.Expression, "typeof window.__htrcliDom"):
			// Report "not installed" the first time, "installed" after.
			installed := false
			for _, e := range expressions[:len(expressions)-1] {
				if strings.Contains(e, "__htrcliDom = {") || strings.Contains(e, "__htrcliDomBundle") {
					installed = true
				}
			}
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"result": map[string]any{"type": "string", "value": map[bool]string{true: "object", false: "undefined"}[installed]},
			}})
		case strings.Contains(p.Expression, "__htrcliDom.exec"):
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"result": map[string]any{"type": "object", "value": map[string]any{"id": "1", "success": true, "data": "filled"}},
			}})
		default: // bundle installation
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"result": map[string]any{"type": "undefined"},
			}})
		}
	})

	s, err := Dial(url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer s.Close()

	result, err := ExecDOM(s, api.Command{ID: "1", Action: "fill", Target: &api.TargetSelector{Selector: "#email"}, Value: "x"})
	if err != nil {
		t.Fatalf("ExecDOM: %v", err)
	}
	if !result.Success {
		t.Fatalf("want success, got %+v", result)
	}

	// Second exec must NOT reinstall: expression count grows by exactly 2
	// (probe + exec), not 3.
	before := len(expressions)
	if _, err := ExecDOM(s, api.Command{ID: "2", Action: "getValue", Target: &api.TargetSelector{Selector: "#email"}}); err != nil {
		t.Fatalf("second ExecDOM: %v", err)
	}
	if grew := len(expressions) - before; grew != 2 {
		t.Fatalf("second exec issued %d evaluates (want 2: probe + exec, no reinstall)", grew)
	}
}

func TestExecDOMErrorResult(t *testing.T) {
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		var p struct {
			Expression string `json:"expression"`
		}
		json.Unmarshal(m.Params, &p)
		if strings.Contains(p.Expression, "typeof window.__htrcliDom") {
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"result": map[string]any{"type": "string", "value": "object"}}})
			return
		}
		conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
			"result": map[string]any{"type": "object", "value": map[string]any{"id": "1", "success": false, "error": "Element not found: #nope"}},
		}})
	})
	s, _ := Dial(url)
	defer s.Close()

	result, err := ExecDOM(s, api.Command{ID: "1", Action: "click", Target: &api.TargetSelector{Selector: "#nope"}})
	if err != nil {
		t.Fatalf("transport must not error on command failure: %v", err)
	}
	if result.Success || !strings.Contains(result.Error, "not found") {
		t.Fatalf("want failed result, got %+v", result)
	}
}

// evalFake answers Runtime.evaluate with an exception of the given class on
// the first call and success on any later call, counting calls.
func evalFake(t *testing.T, className string, calls *int) string {
	return fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		if m.Method != "Runtime.evaluate" {
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
			return
		}
		*calls++
		if *calls == 1 {
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"result": map[string]any{"type": "object", "subtype": "error"},
				"exceptionDetails": map[string]any{
					"text": "Uncaught",
					"exception": map[string]any{
						"className":   className,
						"description": className + ": boom",
					},
				},
			}})
			return
		}
		conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
			"result": map[string]any{"type": "number", "value": 7},
		}})
	})
}

func TestEvaluateRetriesOnlySyntaxError(t *testing.T) {
	var calls int
	url := evalFake(t, "SyntaxError", &calls)
	s, _ := Dial(url)
	defer s.Close()

	raw, err := Evaluate(s, "const a = 1; return a + 6")
	if err != nil {
		t.Fatalf("SyntaxError must be retried as function body: %v", err)
	}
	if calls != 2 || string(raw) != "7" {
		t.Fatalf("want 2 calls and value 7, got %d calls, %s", calls, raw)
	}
}

func TestEvaluateDoesNotRetryRuntimeException(t *testing.T) {
	var calls int
	url := evalFake(t, "TypeError", &calls)
	s, _ := Dial(url)
	defer s.Close()

	_, err := Evaluate(s, "submitOrder(); reportStatus()")
	if err == nil || !strings.Contains(err.Error(), "TypeError") {
		t.Fatalf("want runtime exception surfaced, got %v", err)
	}
	// Re-running a script that already had side effects would repeat them.
	if calls != 1 {
		t.Fatalf("runtime exception must not be retried: %d evaluate calls", calls)
	}
}
