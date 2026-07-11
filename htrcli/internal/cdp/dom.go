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
	var res evalResult
	err := s.Call("Runtime.evaluate", map[string]any{
		"expression":    expression,
		"returnByValue": true,
		"awaitPromise":  awaitPromise,
	}, &res)
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

// ensureBundle installs the DOM bundle unless the page already has it.
func ensureBundle(s *Session) error {
	probe, err := evaluate(s, "typeof window.__htrcliDom", false)
	if err != nil {
		return err
	}
	var typ string
	if err := json.Unmarshal(probe.Result.Value, &typ); err != nil {
		return fmt.Errorf("decoding bundle probe: %w", err)
	}
	if typ == "object" {
		return nil
	}
	if _, err := evaluate(s, domBundle, false); err != nil {
		return fmt.Errorf("installing DOM bundle: %w", err)
	}
	return nil
}

// ExecDOM runs one command through the embedded bundle. A failed command is
// returned as a CommandResult with Success=false, not a Go error — matching
// the extension transport's semantics.
func ExecDOM(s *Session, cmd api.Command) (*api.CommandResult, error) {
	if err := ensureBundle(s); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(cmd)
	if err != nil {
		return nil, fmt.Errorf("encoding command: %w", err)
	}
	expr := fmt.Sprintf("window.__htrcliDom.exec(%s)", payload)
	res, err := evaluate(s, expr, true)
	if err != nil {
		return nil, err
	}
	var result api.CommandResult
	if err := json.Unmarshal(res.Result.Value, &result); err != nil {
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
