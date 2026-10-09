package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestParseFillFormSpecObjectKeepsOrder(t *testing.T) {
	entries, err := parseFillFormSpec([]byte(`{"#z": "last-alpha", "#a": true, "label=Email": null, "#n": 1.50, "#f": ["a","b"]}`))
	if err != nil {
		t.Fatal(err)
	}
	wantSel := []string{"#z", "#a", "label=Email", "#n", "#f"}
	wantVal := []string{`"last-alpha"`, `true`, `null`, `1.50`, `["a","b"]`}
	if len(entries) != len(wantSel) {
		t.Fatalf("got %d entries", len(entries))
	}
	for i, e := range entries {
		if e.Selector != wantSel[i] || string(e.Value) != wantVal[i] {
			t.Errorf("entry %d = %s %s, want %s %s", i, e.Selector, e.Value, wantSel[i], wantVal[i])
		}
	}
}

func TestParseFillFormSpecArrayForm(t *testing.T) {
	entries, err := parseFillFormSpec([]byte(`[{"selector":"#b","value":"x"},{"value":false,"selector":"#a"},{"selector":"#c","value":null}]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 || entries[0].Selector != "#b" || entries[1].Selector != "#a" || string(entries[1].Value) != "false" || string(entries[2].Value) != "null" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestParseFillFormSpecErrors(t *testing.T) {
	cases := map[string]string{
		``:                                    "invalid JSON",
		`"x"`:                                 "expected a JSON object or array",
		`{}`:                                  "no fields",
		`[]`:                                  "no fields",
		`{"#a": 1, "#a": 2}`:                  "appears twice",
		`{"": 1}`:                             "empty selector",
		`{"#a": }`:                            "value for",
		`{"#a": 1} {"#b": 2}`:                 "unexpected data",
		`[{"selector":"#a"}]`:                 `no "value"`,
		`[{"value":1}]`:                       `no "selector"`,
		`[{"selector":"","value":1}]`:         "non-empty string",
		`[{"selector":"#a","value":1,"x":2}]`: `unknown key "x"`,
		`["#a"]`:                              "must be an object",
	}
	for in, want := range cases {
		_, err := parseFillFormSpec([]byte(in))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("parseFillFormSpec(%s) err = %v, want containing %q", in, err, want)
		}
	}
}

func TestFillFormInput(t *testing.T) {
	file := filepath.Join(t.TempDir(), "form.json")
	if err := os.WriteFile(file, []byte(`{"#a":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := fillFormInput(nil, file); err != nil || string(got) != `{"#a":1}` {
		t.Fatalf("--file: %s %v", got, err)
	}
	if got, err := fillFormInput([]string{`{"#b":2}`}, ""); err != nil || string(got) != `{"#b":2}` {
		t.Fatalf("arg: %s %v", got, err)
	}
	if _, err := fillFormInput([]string{"{}"}, file); err == nil || !strings.Contains(err.Error(), "not both") {
		t.Fatalf("both: %v", err)
	}
	if _, err := fillFormInput(nil, ""); err == nil {
		t.Fatal("neither must error")
	}
	if _, err := fillFormInput(nil, filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("missing file must error")
	}
}

func TestPickAndFillFormRejectExtensionTransport(t *testing.T) {
	resetTransportState()
	defer resetTransportState()
	if err := pickCmd.RunE(pickCmd, []string{"#x", "y"}); err == nil || err.Error() != "pick is only supported over --cdp" {
		t.Fatalf("pick on ext: %v", err)
	}
	if err := fillFormCmd.RunE(fillFormCmd, []string{`{"#a":1}`}); err == nil || err.Error() != "fill-form is only supported over --cdp" {
		t.Fatalf("fill-form on ext: %v", err)
	}
}

// findCmdByName locates a registered command by leaf name (searching
// subcommands too), so the --frame tests exercise the real command objects
// and their annotations rather than a name string.
func findCmdByName(t *testing.T, name string) *cobra.Command {
	t.Helper()
	var walk func(cmds []*cobra.Command) *cobra.Command
	walk = func(cmds []*cobra.Command) *cobra.Command {
		for _, c := range cmds {
			if c.Name() == name {
				return c
			}
			if sub := walk(c.Commands()); sub != nil {
				return sub
			}
		}
		return nil
	}
	if c := walk(rootCmd.Commands()); c != nil {
		return c
	}
	t.Fatalf("no registered command named %q", name)
	return nil
}

func TestValidateFrameFlag(t *testing.T) {
	eval := findCmdByName(t, "eval")
	click := findCmdByName(t, "click")
	if err := validateFrameFlag("", eval, false); err != nil {
		t.Fatalf("no --frame: %v", err)
	}
	if err := validateFrameFlag("#f", click, false); err == nil || err.Error() != "--frame is only supported over --cdp" {
		t.Fatalf("ext: %v", err)
	}
	// Drift guard: this is the documented frame-aware set (README, SKILL,
	// SPEC). Every one must carry the annotation ON the command — if a name
	// here stops resolving to an annotated command, the flag is silently
	// rejected for a verb that honours it.
	frameAwareVerbsList := []string{"find", "fill", "type", "check", "uncheck", "select", "click", "dblclick", "value", "text", "attr", "html", "pick", "fill-form"}
	for _, name := range frameAwareVerbsList {
		cmd := findCmdByName(t, name)
		if !frameAware(cmd) {
			t.Errorf("%s must carry the %s annotation", name, AnnotationFrameAware)
		}
		if err := validateFrameFlag("#f", cmd, true); err != nil {
			t.Errorf("%s must honour --frame: %v", name, err)
		}
	}
	for _, name := range []string{"eval", "screenshot", "open", "upload", "drag", "findAll", "list", "press", "hover"} {
		cmd := findCmdByName(t, name)
		if frameAware(cmd) {
			t.Errorf("%s must not carry the %s annotation", name, AnnotationFrameAware)
		}
		if err := validateFrameFlag("#f", cmd, true); err == nil || !strings.Contains(err.Error(), "not supported by") {
			t.Errorf("%s must reject --frame, got %v", name, err)
		}
	}
	// The rejection message is generated from the annotations, so it must
	// name every frame-aware verb.
	err := validateFrameFlag("#f", eval, true)
	if err == nil {
		t.Fatal("eval must reject --frame")
	}
	for _, name := range frameAwareVerbsList {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("rejection message must list %q, got: %v", name, err)
		}
	}
}

func TestCheckFrameTarget(t *testing.T) {
	defer func() { frameFlag = "" }()
	frameFlag = ""
	if err := checkFrameTarget(parseSelector("@e1")); err != nil {
		t.Fatalf("no frame: %v", err)
	}
	frameFlag = "#f"
	if err := checkFrameTarget(parseSelector("#x")); err != nil {
		t.Fatalf("css in frame: %v", err)
	}
	if err := checkFrameTarget(parseSelector("@e1")); err == nil || !strings.Contains(err.Error(), "@eN") {
		t.Fatalf("ref in frame: %v", err)
	}
	if err := checkFrameTarget(parseSelector("xy=10,20")); err == nil || !strings.Contains(err.Error(), "xy=") {
		t.Fatalf("xy in frame: %v", err)
	}
	if err := runFindRefCDP("#x", false); err == nil || !strings.Contains(err.Error(), "--frame") {
		t.Fatalf("find --ref in frame: %v", err)
	}
}

func TestReportedJSONErrorSuppressesSecondJSONObject(t *testing.T) {
	if errorObjectNeeded(&reportedJSONError{msg: "x"}) {
		t.Fatal("a reported --json failure must not print a second JSON object")
	}
	if errorObjectNeeded(fmt.Errorf("wrapped: %w", &reportedJSONError{msg: "x"})) {
		t.Fatal("wrapped reported errors must also be recognised")
	}
	if !errorObjectNeeded(fmt.Errorf("plain")) {
		t.Fatal("ordinary errors still print the error object")
	}
}
