package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/u007/htrcli/internal/output"
)

// fakeRunner returns a runner that answers per-argv with a canned result.
func fakeRunner(results map[string]BenchStepResult) benchRunner {
	return func(step BenchStep) BenchStepResult {
		key := strings.Join(step.Argv, " ")
		if r, ok := results[key]; ok {
			r.Argv = benchArgv(step)
			return r
		}
		return BenchStepResult{OK: true, ExitCode: 0, Attempts: 1, WallMS: 1, Argv: benchArgv(step)}
	}
}

func TestBenchArgvNoRetry(t *testing.T) {
	got := benchArgv(BenchStep{Argv: []string{"open", "https://x"}})
	want := []string{"open", "https://x"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("argv = %v, want %v", got, want)
	}
}

func TestBenchArgvAppliesRetryPrefix(t *testing.T) {
	got := benchArgv(BenchStep{Argv: []string{"click", "#x"}, Retry: 2})
	want := []string{"retry", "--times", "2", "--", "click", "#x"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("argv = %v, want %v", got, want)
	}
}

func TestRunBenchScenarioPass(t *testing.T) {
	s := BenchScenario{
		ID: "B01",
		Steps: []BenchStep{
			{Argv: []string{"open", "https://x"}},
			{Argv: []string{"extract", "table", "table"}},
		},
	}
	res := runBenchScenario(s, fakeRunner(nil))
	if res.Verdict != VerdictPass {
		t.Fatalf("verdict = %q, want %q", res.Verdict, VerdictPass)
	}
	if res.Commands != 2 {
		t.Errorf("commands = %d, want 2", res.Commands)
	}
	if res.FailureMode != "" {
		t.Errorf("failure_mode = %q, want empty", res.FailureMode)
	}
}

func TestRunBenchScenarioRequiredStepFails(t *testing.T) {
	s := BenchScenario{
		ID:    "B01",
		Steps: []BenchStep{{Argv: []string{"click", "#x"}}},
	}
	runner := func(step BenchStep) BenchStepResult {
		return BenchStepResult{OK: false, ExitCode: 1, Failure: "element not found", Attempts: 1}
	}
	res := runBenchScenario(s, runner)
	if res.Verdict != VerdictFail {
		t.Fatalf("verdict = %q, want %q", res.Verdict, VerdictFail)
	}
	if !strings.Contains(res.FailureMode, "element not found") {
		t.Errorf("failure_mode = %q, should carry the step's failure", res.FailureMode)
	}
}

func TestRunBenchScenarioOptionalStepFailsIsPartial(t *testing.T) {
	s := BenchScenario{
		ID: "B01",
		Steps: []BenchStep{
			{Argv: []string{"open", "https://x"}},
			{Argv: []string{"screenshot"}, Optional: true},
		},
	}
	runner := fakeRunner(map[string]BenchStepResult{
		"screenshot": {OK: false, ExitCode: 1, Failure: "no tab"},
	})
	res := runBenchScenario(s, runner)
	if res.Verdict != VerdictPartial {
		t.Fatalf("verdict = %q, want %q", res.Verdict, VerdictPartial)
	}
	if res.FailureMode == "" {
		t.Error("failure_mode should mention the optional failure")
	}
}

func TestRunBenchScenarioRequiresAuthIsNotRun(t *testing.T) {
	s := BenchScenario{
		ID:           "B01",
		RequiresAuth: true,
		Steps:        []BenchStep{{Argv: []string{"open", "https://x"}}},
	}
	called := false
	runner := func(step BenchStep) BenchStepResult {
		called = true
		return BenchStepResult{OK: true}
	}
	res := runBenchScenario(s, runner)
	if res.Verdict != VerdictNotRun {
		t.Fatalf("verdict = %q, want %q", res.Verdict, VerdictNotRun)
	}
	if called {
		t.Error("a requires_auth scenario must not execute any step")
	}
	if res.Commands != 0 {
		t.Errorf("commands = %d, want 0", res.Commands)
	}
}

func TestRunBenchScenarioUsesObservedAttempts(t *testing.T) {
	s := BenchScenario{
		ID:    "B01",
		Steps: []BenchStep{{Argv: []string{"click", "#x"}, Retry: 2}},
	}

	// A step that succeeded on the FIRST attempt must report the attempts it
	// actually made, not the retry budget it was granted. The spec defines
	// "commands" as htrcli invocations and "retries" as retries/recoveries.
	res := runBenchScenario(s, fakeRunner(nil))
	if res.Commands != 1 {
		t.Errorf("commands = %d, want 1 (the step passed first try)", res.Commands)
	}
	if res.Retries != 0 {
		t.Errorf("retries = %d, want 0", res.Retries)
	}

	// When the runner really did retry, the observed count is used.
	retried := func(step BenchStep) BenchStepResult {
		return BenchStepResult{OK: true, Attempts: 3, Argv: benchArgv(step)}
	}
	res2 := runBenchScenario(s, retried)
	if res2.Commands != 3 {
		t.Errorf("commands = %d, want 3", res2.Commands)
	}
	if res2.Retries != 2 {
		t.Errorf("retries = %d, want 2", res2.Retries)
	}
}

func TestParseRetryAttempts(t *testing.T) {
	cases := []struct {
		name   string
		stderr string
		want   int
		ok     bool
	}{
		{"success after retries", "retry: attempt 1/3 failed: x\nretry: attempts=2\n", 2, true},
		{"exhausted", "retry: all 3 attempts failed\nretry: attempts=3\n", 3, true},
		{"absent", "some other output", 0, false},
		{"garbage", "retry: attempts=abc", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseRetryAttempts(tc.stderr)
			if ok != tc.ok || (ok && got != tc.want) {
				t.Fatalf("parseRetryAttempts(%q) = (%d,%v), want (%d,%v)", tc.stderr, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestEvaluateBenchExpectExit(t *testing.T) {
	run := selfRun{ExitCode: 1, Stderr: "boom"}
	if err := evaluateBenchExpect(&BenchExpect{Exit: 0}, run); err == nil {
		t.Fatal("expected an error for a non-zero exit")
	}
	ok := selfRun{ExitCode: 0}
	if err := evaluateBenchExpect(&BenchExpect{Exit: 0}, ok); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEvaluateBenchExpectNilExpectDefaultsToZeroExit(t *testing.T) {
	if err := evaluateBenchExpect(nil, selfRun{ExitCode: 0}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := evaluateBenchExpect(nil, selfRun{ExitCode: 2}); err == nil {
		t.Fatal("expected an error for a non-zero exit with no expectations")
	}
}

func TestEvaluateBenchExpectStdout(t *testing.T) {
	run := selfRun{ExitCode: 0, Stdout: "Navigated to https://example.com"}
	if err := evaluateBenchExpect(&BenchExpect{StdoutContains: "Navigated"}, run); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := evaluateBenchExpect(&BenchExpect{StdoutContains: "missing"}, run); err == nil {
		t.Fatal("expected an error when the substring is absent")
	}
	if err := evaluateBenchExpect(&BenchExpect{StdoutNotContains: "missing"}, run); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := evaluateBenchExpect(&BenchExpect{StdoutNotContains: "Navigated"}, run); err == nil {
		t.Fatal("expected an error when the forbidden substring is present")
	}
}

func TestEvaluateBenchExpectJSONPath(t *testing.T) {
	run := selfRun{ExitCode: 0, Stdout: `{"columns":["Name"],"rows":[{"Name":"Alice"}],"rowCount":1}`}
	if err := evaluateBenchExpect(&BenchExpect{
		JSONPath: &JSONPathExpect{Path: "rows.0.Name", Equals: "Alice"},
	}, run); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := evaluateBenchExpect(&BenchExpect{
		JSONPath: &JSONPathExpect{Path: "rows.0.Name", Equals: "Bob"},
	}, run); err == nil {
		t.Fatal("expected an error for a mismatched value")
	}
	if err := evaluateBenchExpect(&BenchExpect{
		JSONPath: &JSONPathExpect{Path: "rows.5.Name", Exists: true},
	}, run); err == nil {
		t.Fatal("expected an error for a missing path")
	}
	if err := evaluateBenchExpect(&BenchExpect{
		JSONPath: &JSONPathExpect{Path: "rowCount", Equals: "1"},
	}, run); err != nil {
		t.Fatalf("unexpected error for a numeric value: %v", err)
	}
}

func TestEvaluateBenchExpectJSONPathNonJSONStdout(t *testing.T) {
	run := selfRun{ExitCode: 0, Stdout: "not json"}
	err := evaluateBenchExpect(&BenchExpect{
		JSONPath: &JSONPathExpect{Path: "a", Exists: true},
	}, run)
	if err == nil {
		t.Fatal("expected an error for non-JSON stdout")
	}
	if !strings.Contains(err.Error(), "not JSON") {
		t.Errorf("error = %q, should mention the JSON problem", err)
	}
}

func TestLookupJSONPath(t *testing.T) {
	doc := map[string]any{
		"rows": []any{
			map[string]any{"Name": "Alice", "Age": float64(30)},
		},
		"rowCount": float64(1),
	}
	if v, ok := lookupJSONPath(doc, "rows.0.Name"); !ok || v != "Alice" {
		t.Errorf("rows.0.Name = %v, %v", v, ok)
	}
	if _, ok := lookupJSONPath(doc, "rows.9.Name"); ok {
		t.Error("out-of-range index should not resolve")
	}
	if _, ok := lookupJSONPath(doc, "missing"); ok {
		t.Error("missing key should not resolve")
	}
}

func TestNormalizeJSONValue(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{"Alice", "Alice"},
		{float64(30), "30"},
		{float64(1.5), "1.5"},
		{true, "true"},
		{nil, ""},
	}
	for _, tc := range cases {
		if got := normalizeJSONValue(tc.in); got != tc.want {
			t.Errorf("normalizeJSONValue(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestLoadBenchScenariosStrict(t *testing.T) {
	dir := t.TempDir()

	// Unknown field must be an error.
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte(`{"scenarios":[{"id":"B01","stepz":[]}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadBenchScenarios(bad); err == nil {
		t.Fatal("expected an error for an unknown field")
	}

	// Missing steps must be an error.
	nosteps := filepath.Join(dir, "nosteps.json")
	if err := os.WriteFile(nosteps, []byte(`{"scenarios":[{"id":"B01"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadBenchScenarios(nosteps); err == nil {
		t.Fatal("expected an error for a scenario with no steps")
	}

	// Empty argv must be an error.
	emptyArgv := filepath.Join(dir, "emptyargv.json")
	if err := os.WriteFile(emptyArgv, []byte(`{"scenarios":[{"id":"B01","steps":[{"argv":[]}]}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadBenchScenarios(emptyArgv); err == nil {
		t.Fatal("expected an error for a step with no argv")
	}

	// A valid file loads.
	good := filepath.Join(dir, "good.json")
	body := `{"scenarios":[{"id":"B01","requires_auth":true,"steps":[{"argv":["health"]}]}]}`
	if err := os.WriteFile(good, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	scenarios, err := loadBenchScenarios(good)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(scenarios) != 1 || scenarios[0].ID != "B01" {
		t.Fatalf("scenarios = %+v", scenarios)
	}
}

func TestWriteBenchReport(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "report.json")
	report := BenchReport{
		StartedAt: "2026-10-20T00:00:00Z",
		Transport: "ext",
		Scenarios: []BenchScenarioResult{{ID: "B01", Verdict: VerdictPass, Commands: 2}},
		Summary:   BenchSummary{Total: 1, Pass: 1},
	}
	if err := writeBenchReport(path, report); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var back BenchReport
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("report is not valid JSON: %v", err)
	}
	if back.Scenarios[0].Verdict != VerdictPass {
		t.Errorf("verdict = %q, want %q", back.Scenarios[0].Verdict, VerdictPass)
	}
}

func TestSummarizeBenchReport(t *testing.T) {
	results := []BenchScenarioResult{
		{Verdict: VerdictPass},
		{Verdict: VerdictPass},
		{Verdict: VerdictPartial},
		{Verdict: VerdictFail},
		{Verdict: VerdictNotRun},
	}
	sum := summarizeBenchReport(results, 1234)
	if sum.Total != 5 || sum.Pass != 2 || sum.Partial != 1 || sum.Fail != 1 || sum.NotRun != 1 {
		t.Errorf("summary = %+v", sum)
	}
	if sum.WallMS != 1234 {
		t.Errorf("wall_ms = %d, want 1234", sum.WallMS)
	}
}

func TestExpandBenchVars(t *testing.T) {
	t.Setenv("BENCH_TEST_URL", "https://example.com")
	got := expandBenchVars([]string{"open", "${BENCH_TEST_URL}/x", "literal"})
	want := []string{"open", "https://example.com/x", "literal"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("argv = %v, want %v", got, want)
	}
}

func TestExpandBenchVarsLeavesUnsetVariable(t *testing.T) {
	got := expandBenchVars([]string{"${BENCH_DEFINITELY_UNSET_VAR}"})
	if got[0] != "${BENCH_DEFINITELY_UNSET_VAR}" {
		t.Errorf("argv = %v, want the placeholder left in place", got)
	}
}

func TestBenchHasTransportFlag(t *testing.T) {
	cases := []struct {
		argv []string
		want bool
	}{
		{[]string{"open", "https://x"}, false},
		{[]string{"--cdp", "open"}, true},
		{[]string{"--transport=cdp", "open"}, true},
		{[]string{"--transport", "cdp", "open"}, true},
		{[]string{"open", "--transport=ext"}, true},
	}
	for _, tc := range cases {
		if got := benchHasTransportFlag(tc.argv); got != tc.want {
			t.Errorf("benchHasTransportFlag(%v) = %v, want %v", tc.argv, got, tc.want)
		}
	}
}

func TestInjectBenchTransport(t *testing.T) {
	got := injectBenchTransport([]string{"open", "https://x"}, "cdp")
	want := "--transport=cdp open https://x"
	if strings.Join(got, " ") != want {
		t.Errorf("argv = %v, want %q", got, want)
	}
}

func TestInjectBenchTransportKeepsAnExplicitTransport(t *testing.T) {
	got := injectBenchTransport([]string{"--cdp", "open"}, "ext")
	if strings.Join(got, " ") != "--cdp open" {
		t.Errorf("argv = %v, want the step's own transport", got)
	}
}

func TestInjectBenchTransportEmptyTransportIsANoOp(t *testing.T) {
	got := injectBenchTransport([]string{"open"}, "")
	if strings.Join(got, " ") != "open" {
		t.Errorf("argv = %v, want unchanged", got)
	}
}

func TestDefaultBenchReportPath(t *testing.T) {
	ts := time.Date(2026, 10, 20, 13, 45, 6, 0, time.UTC)
	got := defaultBenchReportPath(ts)
	want := filepath.Join("docs", "benchmarks", "results", "20261020-134506.json")
	if got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
}

func TestFinishBenchRunFailsInBothOutputModes(t *testing.T) {
	// The JSON branch used to return nil before the failure check, so a CI run
	// with --json reported success for a failing suite.
	report := BenchReport{Summary: BenchSummary{Total: 1, Fail: 1}}
	old := output.JSONOutput
	defer func() { output.JSONOutput = old }()

	output.JSONOutput = true
	if err := finishBenchRun(report, filepath.Join(t.TempDir(), "r.json")); err == nil {
		t.Fatal("--json run with a failing scenario must return an error")
	}

	output.JSONOutput = false
	if err := finishBenchRun(report, filepath.Join(t.TempDir(), "r.json")); err == nil {
		t.Fatal("non-json run with a failing scenario must return an error")
	}

	ok := BenchReport{Summary: BenchSummary{Total: 1, Pass: 1}}
	if err := finishBenchRun(ok, filepath.Join(t.TempDir(), "r.json")); err != nil {
		t.Fatalf("a passing run must not error: %v", err)
	}
}

func TestRunBenchStepWithTransportArgv(t *testing.T) {
	// The transport flag must be injected BEFORE the step's own argv, and the
	// retry prefix must wrap the whole thing, so the flag reaches the child
	// that retry re-invokes.
	cases := []struct {
		name      string
		step      BenchStep
		transport string
		want      []string
	}{
		{
			name:      "plain step",
			step:      BenchStep{Argv: []string{"open", "https://x"}},
			transport: "cdp",
			want:      []string{"--transport=cdp", "open", "https://x"},
		},
		{
			name:      "retrying step",
			step:      BenchStep{Argv: []string{"open", "https://x"}, Retry: 2},
			transport: "cdp",
			want:      []string{"retry", "--times", "2", "--", "--transport=cdp", "open", "https://x"},
		},
		{
			name:      "step names its own transport",
			step:      BenchStep{Argv: []string{"--cdp", "open", "https://x"}},
			transport: "ext",
			want:      []string{"--cdp", "open", "https://x"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := benchArgv(BenchStep{
				Argv:  injectBenchTransport(tc.step.Argv, tc.transport),
				Retry: tc.step.Retry,
			})
			if strings.Join(got, " ") != strings.Join(tc.want, " ") {
				t.Fatalf("argv = %v, want %v", got, tc.want)
			}
		})
	}
}
