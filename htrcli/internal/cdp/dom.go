package cdp

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/u007/htrcli/internal/api"
)

//go:embed bundle/htrcli-dom.js
var domBundle string

// PageSession dials the target's WebSocket (empty targetID = first page).
func PageSession(port int, targetID string) (*Session, error) {
	targets, err := ListTargets(port)
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		return nil, errors.New("no page targets open")
	}
	if targetID == "" {
		return Dial(targets[0].WebSocketDebuggerURL)
	}
	for _, t := range targets {
		if t.ID == targetID {
			return Dial(t.WebSocketDebuggerURL)
		}
	}
	return nil, fmt.Errorf("no page target with id %q (list with: htrcli tabs list --cdp)", targetID)
}

type evalResult struct {
	Result struct {
		Type    string          `json:"type"`
		Value   json.RawMessage `json:"value"`
		Subtype string          `json:"subtype"`
	} `json:"result"`
	ExceptionDetails *struct {
		Text      string `json:"text"`
		Exception *struct {
			ClassName   string `json:"className"`
			Description string `json:"description"`
		} `json:"exception"`
	} `json:"exceptionDetails"`
}

// pageError is a script exception surfaced by Runtime.evaluate. ClassName
// distinguishes compile-time SyntaxError (retryable as a function body) from
// runtime failures (never retried — the script may have had side effects).
type pageError struct {
	ClassName string
	Message   string
}

func (e *pageError) Error() string {
	return fmt.Sprintf("page exception: %s", e.Message)
}

func evaluate(s *Session, expression string, awaitPromise bool) (*evalResult, error) {
	return evaluateIn(s, expression, 0, awaitPromise)
}

// evaluateIn is evaluate in the execution context ctx (0 = the target's
// default context, i.e. the top document's main world).
func evaluateIn(s *Session, expression string, ctx int64, awaitPromise bool) (*evalResult, error) {
	var res evalResult
	params := map[string]any{
		"expression":    expression,
		"returnByValue": true,
		"awaitPromise":  awaitPromise,
	}
	if ctx != 0 {
		params["contextId"] = ctx
	}
	err := s.Call("Runtime.evaluate", params, &res)
	if err != nil {
		return nil, err
	}
	if res.ExceptionDetails != nil {
		perr := &pageError{Message: res.ExceptionDetails.Text}
		if exc := res.ExceptionDetails.Exception; exc != nil {
			perr.ClassName = exc.ClassName
			perr.Message = exc.Description
		}
		return nil, perr
	}
	return &res, nil
}

// pageBundle is a script htrcli installs once per realm and then calls by
// name: the DOM bundle (window.__htrcliDom) and the form kit
// (window.__htrcliFormKit). install is the expression that registers it.
type pageBundle struct {
	name    string
	install func() string
}

var domBundleSpec = pageBundle{name: "DOM bundle", install: func() string { return domBundle }}

// bundleNeeded reports whether a bundle-exec result is the sentinel the page
// returns when the bundle is not installed yet. It is checked before
// anything else so the sentinel can never be mistaken for a result.
func bundleNeeded(raw json.RawMessage) bool {
	var probe struct {
		NeedBundle bool `json:"__htrcliNeedBundle"`
	}
	return json.Unmarshal(raw, &probe) == nil && probe.NeedBundle
}

// installBundle injects b into the session's realm (top document or the
// --frame's execution context — CSP-exempt either way, see frame.go).
// Callers only reach it after an exec attempt reported the bundle missing,
// so there is no separate probe round trip: the check rides on the exec
// expression itself (see withBundle), which halves the steady-state cost of
// every bundle-backed command.
func installBundle(s *Session, b pageBundle) error {
	if _, err := evaluateScoped(s, b.install(), false); err != nil {
		return fmt.Errorf("installing %s%s: %w", b.name, s.frameSuffix(), err)
	}
	return nil
}

// frameSuffix names the --frame in error messages ("" without one).
func (s *Session) frameSuffix() string {
	if s.frame == "" {
		return ""
	}
	return fmt.Sprintf(" in --frame %q", s.frame)
}

// withBundle runs a page-side call that depends on b being installed. The
// call itself reports a missing bundle by answering the sentinel object
// instead of a result; withBundle then installs b and retries exactly once
// (a second sentinel means the install did not take). Every bundle-backed
// path — DOM commands, drag resolution, the form kit — goes through here so
// the install/retry semantics live in one place.
func withBundle(s *Session, b pageBundle, run func() (json.RawMessage, error)) (json.RawMessage, error) {
	raw, err := run()
	if err != nil {
		return nil, err
	}
	if !bundleNeeded(raw) {
		return raw, nil
	}
	if err := installBundle(s, b); err != nil {
		return nil, err
	}
	if raw, err = run(); err != nil {
		return nil, err
	}
	if bundleNeeded(raw) {
		return nil, fmt.Errorf("installing %s%s: bundle did not register", b.name, s.frameSuffix())
	}
	return raw, nil
}

// execDOMFn calls the DOM bundle, or answers the sentinel when it is not
// installed in this realm yet.
const execDOMFn = `(c) => typeof window.__htrcliDom === "object" ? window.__htrcliDom.exec(c) : {__htrcliNeedBundle: true}`

// ExecDOM runs one command through the embedded bundle in the session's
// realm (top document or --frame). A failed command is returned as a
// CommandResult with Success=false, not a Go error — matching the extension
// transport's semantics. A warm realm costs exactly one round trip.
func ExecDOM(s *Session, cmd api.Command) (*api.CommandResult, error) {
	raw, err := withBundle(s, domBundleSpec, func() (json.RawMessage, error) {
		return runScoped(s, execDOMFn, cmd)
	})
	if err != nil {
		return nil, err
	}
	var result api.CommandResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decoding command result: %w", err)
	}
	return &result, nil
}

// Evaluate runs a user-supplied expression (htrcli eval) and returns the raw
// JSON value. Statement bodies that fail to compile as an expression are
// retried as an async function body, mirroring src/background/cdpEval.ts:
// the retry happens ONLY on SyntaxError (compile-time mode selection). A
// runtime exception is never retried — the script may already have had side
// effects, and re-running it would repeat them.
func Evaluate(s *Session, expression string) (json.RawMessage, error) {
	res, err := evaluate(s, expression, true)
	if err == nil {
		return res.Result.Value, nil
	}
	var perr *pageError
	if !errors.As(err, &perr) || perr.ClassName != "SyntaxError" {
		return nil, err
	}
	wrapped := fmt.Sprintf("(async () => { %s })()", expression)
	res, err2 := evaluate(s, wrapped, true)
	if err2 != nil {
		return nil, err // original error is the more useful one
	}
	return res.Result.Value, nil
}
