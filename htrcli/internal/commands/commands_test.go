package commands

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/u007/htrcli/internal/api"
)

func TestParseSelector_CSS(t *testing.T) {
	s := parseSelector("#submit-btn")
	if s.Selector != "#submit-btn" {
		t.Errorf("expected Selector '#submit-btn', got %q", s.Selector)
	}
}

func TestParseSelector_Name(t *testing.T) {
	s := parseSelector("name=email")
	if s.Name != "email" {
		t.Errorf("expected Name 'email', got %q", s.Name)
	}
}

func TestParseSelector_Role(t *testing.T) {
	s := parseSelector("role=button")
	if s.Role != "button" {
		t.Errorf("expected Role 'button', got %q", s.Role)
	}
}

func TestParseSelector_Text(t *testing.T) {
	s := parseSelector("text=Submit")
	if s.Text != "Submit" {
		t.Errorf("expected Text 'Submit', got %q", s.Text)
	}
}

func TestParseSelector_Label(t *testing.T) {
	s := parseSelector("label=Email address")
	if s.Label != "Email address" {
		t.Errorf("expected Label 'Email address', got %q", s.Label)
	}
}

func TestParseSelector_Placeholder(t *testing.T) {
	s := parseSelector("placeholder=Enter email")
	if s.Placeholder != "Enter email" {
		t.Errorf("expected Placeholder 'Enter email', got %q", s.Placeholder)
	}
}

func TestParseSelector_ID(t *testing.T) {
	s := parseSelector("id=login-form")
	if s.ID != "login-form" {
		t.Errorf("expected ID 'login-form', got %q", s.ID)
	}
}

func TestParseSelector_XPath(t *testing.T) {
	s := parseSelector("xpath=//button[1]")
	if s.XPath != "//button[1]" {
		t.Errorf("expected XPath '//button[1]', got %q", s.XPath)
	}
}

func TestParseSelector_DefaultCSS(t *testing.T) {
	s := parseSelector(".login-form input[type=email]")
	if s.Selector != ".login-form input[type=email]" {
		t.Errorf("expected Selector '.login-form input[type=email]', got %q", s.Selector)
	}
}

func TestFormatUptime(t *testing.T) {
	tests := []struct {
		seconds  float64
		expected string
	}{
		{0, "0s"},
		{30, "30s"},
		{60, "1m 0s"},
		{90, "1m 30s"},
		{3600, "1h 0m 0s"},
		{3661, "1h 1m 1s"},
		{86400, "24h 0m 0s"},
	}

	for _, tt := range tests {
		result := formatUptime(tt.seconds)
		if result != tt.expected {
			t.Errorf("formatUptime(%v) = %q, expected %q", tt.seconds, result, tt.expected)
		}
	}
}

func TestParseSelector_Ref(t *testing.T) {
	s := parseSelector("@e7")
	if s.Ref != "@e7" {
		t.Errorf("expected Ref '@e7', got %q (selector=%q)", s.Ref, s.Selector)
	}
	if s.Selector != "" {
		t.Errorf("ref arg must not populate Selector, got %q", s.Selector)
	}
}

func TestParseSelector_RefLeavesRealSelectorsAlone(t *testing.T) {
	// An email like "@" mid-string is not a ref; only a leading @ is.
	s := parseSelector("input[name=email]")
	if s.Ref != "" {
		t.Errorf("expected no Ref for a CSS selector, got %q", s.Ref)
	}
}

func TestFindWithRefSetsAssignRefOption(t *testing.T) {
	var gotAssignRef bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req api.CommandRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Command.Options != nil {
			gotAssignRef, _ = req.Command.Options["assignRef"].(bool)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(api.ApiResponse{
			OK:   true,
			Data: api.CommandResult{ID: "1", Success: true, Data: map[string]any{"ref": "@e1", "tag": "button"}},
		})
	}))
	defer server.Close()

	client = api.NewClient(server.URL, "")
	tabTarget = ""
	transportFlag = "ext"
	findRef = true
	defer func() { client = nil; transportFlag = ""; findRef = false }()

	if err := findCmd.RunE(findCmd, []string{"#go"}); err != nil {
		t.Fatalf("find --ref RunE: %v", err)
	}
	if !gotAssignRef {
		t.Fatalf("expected assignRef=true option to be sent")
	}
}

func TestParseUploadFiles_ExtRejectsRef(t *testing.T) {
	err := runUploadExt("@e1", []string{"/tmp/a.png"})
	if err == nil {
		t.Fatal("expected error for @eN ref on extension transport, got nil")
	}
	if !strings.Contains(err.Error(), "only supported on the --cdp transport") {
		t.Fatalf("want 'only supported on the --cdp transport' in error, got %q", err.Error())
	}
}

func TestParseUploadFiles_CDPRejectsUnknownRef(t *testing.T) {
	dir := t.TempDir()
	refStorePathOverride = dir + "/refs.json"
	defer func() { refStorePathOverride = "" }()

	err := runUploadCDP("@e999", []string{"/tmp/a.png"})
	if err == nil {
		t.Fatal("expected error for unknown ref, got nil")
	}
	if !strings.Contains(err.Error(), "stale ref") {
		t.Fatalf("want 'stale ref' in error, got %q", err.Error())
	}
}

func TestParseAnnotateSelectors(t *testing.T) {
	got := parseAnnotateSelectors("button, role=link ,#submit")
	if len(got) != 3 {
		t.Fatalf("want 3 selectors, got %d: %+v", len(got), got)
	}
	if got[0].Selector != "button" {
		t.Errorf("got[0].Selector = %q, want button", got[0].Selector)
	}
	if got[1].Role != "link" {
		t.Errorf("got[1].Role = %q, want link (whitespace trimmed)", got[1].Role)
	}
	if got[2].Selector != "#submit" {
		t.Errorf("got[2].Selector = %q, want #submit", got[2].Selector)
	}
}

func TestParseAnnotateSelectorsEmpty(t *testing.T) {
	if got := parseAnnotateSelectors(""); got != nil {
		t.Fatalf("empty string should yield nil, got %+v", got)
	}
}

// TestRightClickSendsRightclickAction guards the missing CLI verb: the extension
// and CDP layers both understand "rightclick", so the command must forward that
// exact action rather than degrading to a left click.
func TestRightClickSendsRightclickAction(t *testing.T) {
	var gotAction, gotSelector string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req api.CommandRequest
		json.NewDecoder(r.Body).Decode(&req)
		gotAction = req.Command.Action
		if req.Command.Target != nil {
			gotSelector = req.Command.Target.Selector
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(api.ApiResponse{
			OK:   true,
			Data: api.CommandResult{ID: "1", Success: true},
		})
	}))
	defer server.Close()

	client = api.NewClient(server.URL, "")
	tabTarget = "1"
	transportFlag = "ext"
	defer func() { client = nil; transportFlag = ""; tabTarget = "" }()

	if err := rightclickCmd.RunE(rightclickCmd, []string{".row"}); err != nil {
		t.Fatalf("rightclick RunE: %v", err)
	}
	if gotAction != "rightclick" {
		t.Fatalf("action = %q, want %q", gotAction, "rightclick")
	}
	if gotSelector != ".row" {
		t.Fatalf("selector = %q, want %q", gotSelector, ".row")
	}
}

// ─── `do` (compound sequence) ───────────────────────────────────────

func TestBuildDoStepsCarriesSelectorsDelaysAndValues(t *testing.T) {
	steps, err := buildDoSteps([]doStep{
		{Action: "rightclick", Selector: "tr[data-id=7]"},
		{Action: "click", Selector: "text=Move", WaitMs: 200},
		{Action: "fill", Selector: "#to", Value: "a@b.c", DelayMs: 50},
	})
	if err != nil {
		t.Fatalf("buildDoSteps: %v", err)
	}
	if len(steps) != 3 {
		t.Fatalf("steps = %d, want 3", len(steps))
	}
	if steps[0]["action"] != "rightclick" {
		t.Fatalf("step 1 action = %v", steps[0]["action"])
	}
	first, ok := steps[0]["target"].(*api.TargetSelector)
	if !ok || first.Selector != "tr[data-id=7]" {
		t.Fatalf("step 1 target = %#v", steps[0]["target"])
	}
	if steps[1]["waitMs"] != 200 {
		t.Fatalf("step 2 waitMs = %v, want 200", steps[1]["waitMs"])
	}
	second, ok := steps[1]["target"].(*api.TargetSelector)
	if !ok || second.Text != "Move" {
		t.Fatalf("step 2 target should carry text= as Text, got %#v", steps[1]["target"])
	}
	if steps[2]["value"] != "a@b.c" {
		t.Fatalf("step 3 value = %v", steps[2]["value"])
	}
	if steps[2]["delayMs"] != 50 {
		t.Fatalf("step 3 delayMs = %v, want 50", steps[2]["delayMs"])
	}
}

// The extension reads options.steps[]; an empty value or a nil options map
// must not appear on the wire, or the content script sees a step that clears
// a field it was never told about.
func TestBuildDoStepsMarshalsToTheShapeTheContentScriptReads(t *testing.T) {
	steps, err := buildDoSteps([]doStep{
		{Action: "click", Selector: "role=menuitem"},
	})
	if err != nil {
		t.Fatalf("buildDoSteps: %v", err)
	}
	raw, err := json.Marshal(map[string]any{"steps": steps})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(raw)
	if !strings.Contains(got, `"action":"click"`) {
		t.Fatalf("payload missing action: %s", got)
	}
	if !strings.Contains(got, `"target":{"role":"menuitem"}`) {
		t.Fatalf("payload target not the role selector: %s", got)
	}
	if strings.Contains(got, `"value"`) || strings.Contains(got, `"waitMs"`) {
		t.Fatalf("payload carries an unset field: %s", got)
	}
}

func TestBuildDoStepsRejectsBadInput(t *testing.T) {
	cases := []struct {
		name  string
		steps []doStep
		want  string
	}{
		{"empty", nil, "no steps"},
		{"empty action", []doStep{{Selector: "#a"}}, `step 1 has no "action"`},
		{"blank action", []doStep{{Action: "   "}}, `step 1 has no "action"`},
		{"nested", []doStep{{Action: "click", Selector: "#a"}, {Action: "sequence"}}, "cannot nest"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := buildDoSteps(tc.steps)
			if err == nil {
				t.Fatalf("expected an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want it to contain %q", err.Error(), tc.want)
			}
		})
	}
}

func TestDoCmdSendsOneSequenceCommand(t *testing.T) {
	var gotAction string
	var gotSteps []any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req api.CommandRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode: %v", err)
		}
		gotAction = req.Command.Action
		gotSteps, _ = req.Command.Options["steps"].([]any)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.ApiResponse{
			OK:   true,
			Data: api.CommandResult{ID: "1", Success: true},
		})
	}))
	defer server.Close()

	client = api.NewClient(server.URL, "")
	tabTarget = "1"
	transportFlag = "ext"
	defer func() { client = nil; transportFlag = ""; tabTarget = "" }()

	err := doCmd.RunE(doCmd, []string{
		`[{"action":"click","selector":"#open"},{"action":"click","selector":"text=Move","waitMs":150}]`,
	})
	if err != nil {
		t.Fatalf("do RunE: %v", err)
	}
	if gotAction != "sequence" {
		t.Fatalf("action = %q, want sequence", gotAction)
	}
	if len(gotSteps) != 2 {
		t.Fatalf("steps = %d, want 2", len(gotSteps))
	}
}

func TestDoCmdRejectsInvalidJSON(t *testing.T) {
	err := doCmd.RunE(doCmd, []string{`not json`})
	if err == nil {
		t.Fatal("expected an error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "invalid steps JSON") {
		t.Fatalf("error = %q", err.Error())
	}
}
