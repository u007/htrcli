package cdp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/u007/htrcli/internal/api"
)

func countContains(exprs []string, sub string) int {
	n := 0
	for _, e := range exprs {
		if strings.Contains(e, sub) {
			n++
		}
	}
	return n
}

func TestExecDOMInstallsBundleOnce(t *testing.T) {
	var expressions []string
	var coldExprs []string
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
		case strings.Contains(p.Expression, "__htrcliDom.exec"):
			// The exec expression carries the bundle check itself: answer the
			// sentinel until an install expression has been seen, then the
			// CommandResult. The bundle source never contains ".exec", so the
			// two expression kinds cannot be confused.
			installed := false
			for _, e := range expressions[:len(expressions)-1] {
				if strings.Contains(e, "__htrcliDom = {") || strings.Contains(e, "__htrcliDomBundle") {
					installed = true
				}
			}
			if !installed {
				conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
					"result": map[string]any{"type": "object", "value": map[string]any{"ok": map[string]any{"__htrcliNeedBundle": true}}},
				}})
				return
			}
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"result": map[string]any{"type": "object", "value": map[string]any{"ok": map[string]any{"id": "1", "success": true, "data": "filled"}}},
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

	// Second exec must NOT reinstall, and must not re-probe: the bundle
	// check rides on the exec expression, so steady state is exactly 1
	// evaluate per DOM command (was 2: probe + exec).
	before := len(expressions)
	if _, err := ExecDOM(s, api.Command{ID: "2", Action: "getValue", Target: &api.TargetSelector{Selector: "#email"}}); err != nil {
		t.Fatalf("second ExecDOM: %v", err)
	}
	if grew := len(expressions) - before; grew != 1 {
		t.Fatalf("second exec issued %d evaluates (want 1: exec only, no probe, no reinstall)", grew)
	}

	// Cold path: sentinel → install → retry, exactly 3 evaluates, and the
	// install expression is shipped exactly once.
	coldURL := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		if m.Method != "Runtime.evaluate" {
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
			return
		}
		var p struct {
			Expression string `json:"expression"`
		}
		json.Unmarshal(m.Params, &p)
		coldExprs = append(coldExprs, p.Expression)
		installed := false
		for _, e := range coldExprs[:len(coldExprs)-1] {
			if strings.Contains(e, "__htrcliDom = {") || strings.Contains(e, "__htrcliDomBundle") {
				installed = true
			}
		}
		if strings.Contains(p.Expression, "__htrcliDom.exec") && !installed {
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"result": map[string]any{"type": "object", "value": map[string]any{"ok": map[string]any{"__htrcliNeedBundle": true}}},
			}})
			return
		}
		if strings.Contains(p.Expression, "__htrcliDom.exec") {
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"result": map[string]any{"type": "object", "value": map[string]any{"ok": map[string]any{"id": "1", "success": true, "data": "filled"}}},
			}})
			return
		}
		conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
			"result": map[string]any{"type": "undefined"},
		}})
	})
	cold, err := Dial(coldURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer cold.Close()
	if _, err := ExecDOM(cold, api.Command{ID: "1", Action: "fill", Target: &api.TargetSelector{Selector: "#email"}, Value: "x"}); err != nil {
		t.Fatalf("cold ExecDOM: %v", err)
	}
	if len(coldExprs) != 3 {
		t.Fatalf("cold exec issued %d evaluates (want 3: sentinel, install, retry), exprs: %v", len(coldExprs), coldExprs)
	}
	if installs := countContains(coldExprs, "__htrcliDom = {"); installs != 1 {
		t.Fatalf("cold exec shipped the bundle %d times (want 1)", installs)
	}
}

// frameFake answers the --frame context resolution (iframe lookup by
// handle → DOM.describeNode → Runtime.enable replaying the frame's main-world
// context as id 7) and hands every Runtime.evaluate that carries contextId 7
// to onFrameEval. Any other method gets an empty result.
func frameFake(t *testing.T, onFrameEval func(m fakeMsg, expr string, conn *websocket.Conn)) string {
	t.Helper()
	return fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		switch m.Method {
		case "Runtime.evaluate":
			var p struct {
				Expression    string `json:"expression"`
				ReturnByValue bool   `json:"returnByValue"`
				ContextID     int64  `json:"contextId"`
			}
			json.Unmarshal(m.Params, &p)
			if p.ContextID == 7 {
				onFrameEval(m, p.Expression, conn)
				return
			}
			if !p.ReturnByValue && strings.Contains(p.Expression, "contentDocument") {
				// The iframe element, by handle.
				conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
					"result": map[string]any{"type": "object", "className": "HTMLIFrameElement", "objectId": "iframe-1"}}})
				return
			}
			t.Errorf("unexpected top-level evaluate (contextId=%d): %s", p.ContextID, p.Expression)
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
		case "DOM.describeNode":
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"node": map[string]any{"nodeId": 0, "frameId": "F-INNER"}}})
		case "Runtime.enable":
			// Chrome replays every live context before answering enable.
			conn.WriteJSON(map[string]any{"method": "Runtime.executionContextCreated", "params": map[string]any{
				"context": map[string]any{"id": 1, "auxData": map[string]any{"frameId": "F-TOP", "isDefault": true}}}})
			conn.WriteJSON(map[string]any{"method": "Runtime.executionContextCreated", "params": map[string]any{
				"context": map[string]any{"id": 9, "auxData": map[string]any{"frameId": "F-INNER", "isDefault": false, "type": "isolated"}}}})
			conn.WriteJSON(map[string]any{"method": "Runtime.executionContextCreated", "params": map[string]any{
				"context": map[string]any{"id": 7, "auxData": map[string]any{"frameId": "F-INNER", "isDefault": true}}}})
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
		default:
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
		}
	})
}

// TestExecDOMInFrameColdPath covers the frame-mode sentinel flow: every
// frame-scoped evaluate carries the frame's MAIN-WORLD contextId (resolved
// once, then cached — CSP-exempt, unlike contentWindow.eval), the exec
// expression rides the bundle check, so a cold frame costs sentinel → install
// → retry (3 evaluates) and a warm frame costs 1.
func TestExecDOMInFrameColdPath(t *testing.T) {
	var exprs []string
	url := frameFake(t, func(m fakeMsg, expr string, conn *websocket.Conn) {
		exprs = append(exprs, expr)
		installed := false
		for _, e := range exprs[:len(exprs)-1] {
			if strings.Contains(e, "__htrcliDom = {") {
				installed = true
			}
		}
		// runScoped wraps every result in {ok|error|frameError}.
		ok := func(v any) {
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"result": map[string]any{"type": "object", "value": map[string]any{"ok": v}}}})
		}
		switch {
		case strings.Contains(expr, "__htrcliDom.exec") && !installed:
			ok(map[string]any{"__htrcliNeedBundle": true})
		case strings.Contains(expr, "__htrcliDom.exec"):
			ok(map[string]any{"id": "1", "success": true, "data": "framed"})
		default: // the bundle install itself
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"result": map[string]any{"type": "undefined"}}})
		}
	})

	s, err := Dial(url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer s.Close()
	s.SetFrame("#inner")

	// Cold: sentinel, install, retry = 3 frame evaluates, bundle shipped once.
	res, err := ExecDOM(s, api.Command{ID: "1", Action: "fill", Target: &api.TargetSelector{Selector: "#q"}, Value: "x"})
	if err != nil {
		t.Fatalf("cold frame ExecDOM: %v", err)
	}
	if !res.Success || res.Data != "framed" {
		t.Fatalf("cold result = %+v", res)
	}
	if len(exprs) != 3 {
		t.Fatalf("cold frame exec issued %d frame evaluates (want 3: sentinel, install, retry): %v", len(exprs), exprs)
	}
	if n := countContains(exprs, "__htrcliDom = {"); n != 1 {
		t.Fatalf("bundle shipped %d times (want 1)", n)
	}
	for _, e := range exprs {
		if strings.Contains(e, "eval(") {
			t.Fatalf("frame evaluate must not go through eval (CSP):\n%s", e)
		}
	}

	// Warm: exactly 1 evaluate, no install, context id reused.
	before := len(exprs)
	if _, err := ExecDOM(s, api.Command{ID: "2", Action: "getValue", Target: &api.TargetSelector{Selector: "#q"}}); err != nil {
		t.Fatalf("warm frame ExecDOM: %v", err)
	}
	if grew := len(exprs) - before; grew != 1 {
		t.Fatalf("warm frame exec issued %d evaluates (want 1)", grew)
	}
}

func TestExecDOMErrorResult(t *testing.T) {
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		var p struct {
			Expression string `json:"expression"`
		}
		json.Unmarshal(m.Params, &p)
		if strings.Contains(p.Expression, "typeof window.__htrcliDom") && !strings.Contains(p.Expression, "__htrcliDom.exec") {
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"result": map[string]any{"type": "string", "value": "object"}}})
			return
		}
		conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
			"result": map[string]any{"type": "object", "value": map[string]any{"ok": map[string]any{"id": "1", "success": false, "error": "Element not found: #nope"}}},
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
