package cdp

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ConsoleEntry is one console message or uncaught exception collected over
// CDP. Its JSON tags match the extension transport's console event data
// (commands.consoleEventData), so both transports share one formatter and one
// --level filter. Timestamp is epoch milliseconds and travels on the event
// envelope, not in the data.
type ConsoleEntry struct {
	Level     string   `json:"level"`
	Args      []string `json:"args"`
	Source    string   `json:"source,omitempty"`
	Timestamp int64    `json:"-"`
}

// ConsoleWatcher holds a page session with the Runtime domain enabled.
//
// Each htrcli invocation is a fresh CDP session, so there is no daemon-side
// buffer. Instead, Runtime.enable makes V8 replay the console messages and
// exceptions it still stores for the page (bounded by V8, cleared on
// navigation). Those are Replayed(); anything later comes from Next.
type ConsoleWatcher struct {
	s        *Session
	replayed []ConsoleEntry
}

// WatchConsole enables the Runtime domain and captures the replayed backlog.
//
// The backlog boundary is an ordering barrier, not a timeout: CDP messages on
// one session are delivered in order, so once a Runtime.evaluate issued after
// Runtime.enable has answered, every replayed event is already buffered in
// s.pending. No read deadline is set, so the session stays usable.
func WatchConsole(s *Session) (*ConsoleWatcher, error) {
	if err := s.Call("Runtime.enable", nil, nil); err != nil {
		return nil, fmt.Errorf("Runtime.enable: %w", err)
	}
	if err := s.Call("Runtime.evaluate", map[string]any{"expression": "0"}, nil); err != nil {
		return nil, fmt.Errorf("console replay barrier: %w", err)
	}
	w := &ConsoleWatcher{s: s}
	backlog := s.pending
	s.pending = nil
	for _, msg := range backlog {
		entry, err := parseConsoleEvent(msg.Method, msg.Params)
		if err != nil {
			return nil, err
		}
		if entry != nil {
			w.replayed = append(w.replayed, *entry)
		}
	}
	return w, nil
}

// Replayed returns the console messages and exceptions that existed before
// the watcher attached, oldest first.
func (w *ConsoleWatcher) Replayed() []ConsoleEntry {
	return w.replayed
}

// Next blocks until a new console message or exception arrives, or returns
// ErrEventDeadline when deadline passes (the session is then unusable — see
// Session.NextEvent).
func (w *ConsoleWatcher) Next(deadline time.Time) (*ConsoleEntry, error) {
	for {
		method, params, err := w.s.NextEvent(deadline)
		if err != nil {
			return nil, err
		}
		entry, err := parseConsoleEvent(method, params)
		if err != nil {
			return nil, err
		}
		if entry != nil {
			return entry, nil
		}
	}
}

type remoteObject struct {
	Type                string          `json:"type"`
	Subtype             string          `json:"subtype"`
	Value               json.RawMessage `json:"value"`
	UnserializableValue string          `json:"unserializableValue"`
	Description         string          `json:"description"`
	Preview             *objectPreview  `json:"preview"`
}

type objectPreview struct {
	Subtype    string            `json:"subtype"`
	Overflow   bool              `json:"overflow"`
	Properties []propertyPreview `json:"properties"`
}

type propertyPreview struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

type callFrame struct {
	URL        string `json:"url"`
	LineNumber int    `json:"lineNumber"`
}

type stackTrace struct {
	CallFrames []callFrame `json:"callFrames"`
}

type consoleAPICalledParams struct {
	Type       string         `json:"type"`
	Args       []remoteObject `json:"args"`
	Timestamp  float64        `json:"timestamp"`
	StackTrace *stackTrace    `json:"stackTrace"`
}

type exceptionThrownParams struct {
	Timestamp        float64 `json:"timestamp"`
	ExceptionDetails struct {
		Text       string        `json:"text"`
		LineNumber int           `json:"lineNumber"`
		URL        string        `json:"url"`
		StackTrace *stackTrace   `json:"stackTrace"`
		Exception  *remoteObject `json:"exception"`
	} `json:"exceptionDetails"`
}

// parseConsoleEvent converts Runtime.consoleAPICalled / Runtime.exceptionThrown
// into a ConsoleEntry. Any other method returns (nil, nil).
func parseConsoleEvent(method string, params json.RawMessage) (*ConsoleEntry, error) {
	switch method {
	case "Runtime.consoleAPICalled":
		var p consoleAPICalledParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("decoding %s: %w", method, err)
		}
		args := make([]string, 0, len(p.Args))
		for _, a := range p.Args {
			args = append(args, formatRemoteObject(a))
		}
		return &ConsoleEntry{
			Level:     consoleLevel(p.Type),
			Args:      args,
			Source:    frameSource(p.StackTrace),
			Timestamp: int64(p.Timestamp),
		}, nil
	case "Runtime.exceptionThrown":
		var p exceptionThrownParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("decoding %s: %w", method, err)
		}
		d := p.ExceptionDetails
		text := d.Text
		if d.Exception != nil {
			text = strings.TrimSpace(d.Text + " " + formatRemoteObject(*d.Exception))
		}
		source := frameSource(d.StackTrace)
		if d.URL != "" {
			source = fmt.Sprintf("%s:%d", d.URL, d.LineNumber+1)
		}
		return &ConsoleEntry{
			Level:     "error",
			Args:      []string{text},
			Source:    source,
			Timestamp: int64(p.Timestamp),
		}, nil
	}
	return nil, nil
}

// consoleLevel maps CDP console types onto the extension's level names
// (log/warn/error/info/debug) so --level filters mean the same on both
// transports. Types with no extension equivalent (table, trace, dir, …) pass
// through unchanged.
func consoleLevel(cdpType string) string {
	switch cdpType {
	case "warning":
		return "warn"
	case "assert":
		return "error"
	}
	return cdpType
}

// frameSource renders the top stack frame as url:line (CDP lines are 0-based).
// Extension frames are skipped: the extension's console wrapper is injected
// into the page's MAIN world, so it is callFrames[0] of every console event on
// a patched page - falling back to frame 0 would attribute the page's own
// output to chrome-extension://.../chunk-*.js (Notion, 2026-10-08). When every
// frame is extension code there is no page source to report: return "" rather
// than the wrapper, matching getSourceFromStack on the extension side.
func frameSource(st *stackTrace) string {
	if st == nil || len(st.CallFrames) == 0 {
		return ""
	}
	for _, f := range st.CallFrames {
		if f.URL == "" || isExtensionScriptURL(f.URL) {
			continue
		}
		return fmt.Sprintf("%s:%d", f.URL, f.LineNumber+1)
	}
	return ""
}

// isExtensionScriptURL reports whether a CDP frame URL points at an extension
// script (any browser's scheme).
func isExtensionScriptURL(url string) bool {
	return strings.HasPrefix(url, "chrome-extension://") ||
		strings.HasPrefix(url, "moz-extension://") ||
		strings.HasPrefix(url, "safari-web-extension://")
}

// formatRemoteObject renders a console argument: strings verbatim (as the
// extension does), primitives as JSON, objects from their preview when CDP
// supplied one, else their description.
func formatRemoteObject(o remoteObject) string {
	if o.Type == "string" {
		var s string
		if err := json.Unmarshal(o.Value, &s); err == nil {
			return s
		}
	}
	if o.Type == "undefined" {
		return "undefined"
	}
	if o.UnserializableValue != "" {
		return o.UnserializableValue
	}
	if len(o.Value) > 0 {
		return string(o.Value)
	}
	if o.Preview != nil && o.Type == "object" {
		return formatPreview(*o.Preview)
	}
	return o.Description
}

func formatPreview(p objectPreview) string {
	parts := make([]string, 0, len(p.Properties)+1)
	for _, prop := range p.Properties {
		v := prop.Value
		if prop.Type == "string" {
			v = strconv.Quote(v)
		}
		if p.Subtype == "array" {
			parts = append(parts, v)
		} else {
			parts = append(parts, prop.Name+": "+v)
		}
	}
	if p.Overflow {
		parts = append(parts, "…")
	}
	if p.Subtype == "array" {
		return "[" + strings.Join(parts, ", ") + "]"
	}
	return "{" + strings.Join(parts, ", ") + "}"
}
