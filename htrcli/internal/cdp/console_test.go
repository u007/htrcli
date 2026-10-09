package cdp

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestParseConsoleAPICalled(t *testing.T) {
	params := json.RawMessage(`{
		"type": "warning",
		"timestamp": 1700000000123.7,
		"args": [
			{"type": "string", "value": "hello"},
			{"type": "number", "value": 42, "description": "42"},
			{"type": "object", "className": "Object", "description": "Object",
			 "preview": {"type": "object", "overflow": false, "properties": [
				{"name": "a", "type": "number", "value": "1"},
				{"name": "b", "type": "string", "value": "x"}]}}
		],
		"stackTrace": {"callFrames": [{"url": "http://127.0.0.1/page", "lineNumber": 9, "columnNumber": 4}]}
	}`)
	got, err := parseConsoleEvent("Runtime.consoleAPICalled", params)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Level != "warn" {
		t.Errorf("level = %q, want warn (CDP 'warning' must map to the extension's 'warn')", got.Level)
	}
	want := []string{"hello", "42", `{a: 1, b: "x"}`}
	if strings.Join(got.Args, "|") != strings.Join(want, "|") {
		t.Errorf("args = %q, want %q", got.Args, want)
	}
	if got.Source != "http://127.0.0.1/page:10" {
		t.Errorf("source = %q, want 1-based line", got.Source)
	}
	if got.Timestamp != 1700000000123 {
		t.Errorf("timestamp = %d", got.Timestamp)
	}
}

func TestParseExceptionThrown(t *testing.T) {
	params := json.RawMessage(`{
		"timestamp": 1700000000000,
		"exceptionDetails": {
			"text": "Uncaught", "lineNumber": 4, "url": "http://127.0.0.1/app.js",
			"exception": {"type": "object", "subtype": "error", "className": "Error", "description": "Error: boom\n    at x"}
		}
	}`)
	got, err := parseConsoleEvent("Runtime.exceptionThrown", params)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Level != "error" || len(got.Args) != 1 || !strings.HasPrefix(got.Args[0], "Uncaught Error: boom") {
		t.Errorf("got %+v", got)
	}
	if got.Source != "http://127.0.0.1/app.js:5" {
		t.Errorf("source = %q", got.Source)
	}
}

func TestParseConsoleEventIgnoresOtherMethods(t *testing.T) {
	got, err := parseConsoleEvent("Runtime.executionContextCreated", json.RawMessage(`{}`))
	if err != nil || got != nil {
		t.Fatalf("got %+v, %v; want nil, nil", got, err)
	}
}

func TestParseConsoleEventRejectsMalformed(t *testing.T) {
	if _, err := parseConsoleEvent("Runtime.consoleAPICalled", json.RawMessage(`{"args": 5}`)); err == nil {
		t.Fatal("want a decode error, not a silently empty entry")
	}
}

func TestFormatRemoteObject(t *testing.T) {
	cases := []struct {
		name string
		obj  remoteObject
		want string
	}{
		{"undefined", remoteObject{Type: "undefined"}, "undefined"},
		{"unserializable", remoteObject{Type: "number", UnserializableValue: "NaN"}, "NaN"},
		{"null", remoteObject{Type: "object", Subtype: "null", Value: json.RawMessage(`null`)}, "null"},
		{"bool", remoteObject{Type: "boolean", Value: json.RawMessage(`true`)}, "true"},
		{"array preview", remoteObject{Type: "object", Subtype: "array", Preview: &objectPreview{Subtype: "array", Overflow: true,
			Properties: []propertyPreview{{Name: "0", Type: "number", Value: "1"}, {Name: "1", Type: "string", Value: "a"}}}}, `[1, "a", …]`},
		{"description fallback", remoteObject{Type: "function", Description: "function f() {}"}, "function f() {}"},
	}
	for _, c := range cases {
		if got := formatRemoteObject(c.obj); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestConsoleLevel(t *testing.T) {
	for in, want := range map[string]string{"warning": "warn", "assert": "error", "log": "log", "table": "table"} {
		if got := consoleLevel(in); got != want {
			t.Errorf("consoleLevel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNextEventDrainsPendingFIFO(t *testing.T) {
	s := &Session{pending: []cdpMessage{
		{Method: "A", Params: json.RawMessage(`1`)},
		{Method: "B", Params: json.RawMessage(`2`)},
	}}
	for _, want := range []string{"A", "B"} {
		method, _, err := s.NextEvent(time.Now().Add(time.Second))
		if err != nil || method != want {
			t.Fatalf("got %q, %v; want %q", method, err, want)
		}
	}
}
