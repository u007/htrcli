package commands

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/u007/htrcli/internal/output"
)

// writeBenchScenarioFile writes body to a temporary scenario file and returns
// its path.
func writeBenchScenarioFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "scenario.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// withBenchAllowNetwork sets the --allow-network policy for one test.
func withBenchAllowNetwork(t *testing.T, allow bool) {
	t.Helper()
	old := benchAllowNetwork
	benchAllowNetwork = allow
	t.Cleanup(func() { benchAllowNetwork = old })
}

// TestLoadBenchScenariosNewFields proves the loader accepts and keeps every
// field added for the schema change. Without the struct fields the strict
// decoder rejects the file as an unknown field, so this fails loudly.
func TestLoadBenchScenariosNewFields(t *testing.T) {
	body := `{"scenarios":[{
		"id":"B01",
		"requires_network":true,
		"usage":{"model":"driver-model","tokens_in":1200,"tokens_out":340},
		"deadline_seconds":30,
		"attempts":3,
		"context":"work",
		"steps":[
			{"argv":["open","https://x"],"retry":2,"deadline_seconds":10},
			{"argv":["extract","table","table"]}
		]
	}]}`
	scenarios, err := loadBenchScenarios(writeBenchScenarioFile(t, body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := scenarios[0]
	if !s.RequiresNetwork {
		t.Error("requires_network was not parsed")
	}
	if s.Usage == nil || s.Usage.Model != "driver-model" || s.Usage.TokensIn != 1200 || s.Usage.TokensOut != 340 {
		t.Errorf("usage = %+v", s.Usage)
	}
	if s.DeadlineSeconds != 30 {
		t.Errorf("deadline_seconds = %d, want 30", s.DeadlineSeconds)
	}
	if s.Attempts == nil || *s.Attempts != 3 {
		t.Errorf("attempts = %v, want 3", s.Attempts)
	}
	if s.Context != "work" {
		t.Errorf("context = %q, want work", s.Context)
	}
	if s.Steps[0].DeadlineSeconds != 10 {
		t.Errorf("step deadline_seconds = %d, want 10", s.Steps[0].DeadlineSeconds)
	}
}

// TestLoadBenchScenariosRejectsNegativeDeadlines covers both deadline levels.
func TestLoadBenchScenariosRejectsNegativeDeadlines(t *testing.T) {
	cases := map[string]string{
		"scenario": `{"scenarios":[{"id":"B01","deadline_seconds":-1,"steps":[{"argv":["health"]}]}]}`,
		"step":     `{"scenarios":[{"id":"B01","steps":[{"argv":["health"],"deadline_seconds":-1}]}]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := loadBenchScenarios(writeBenchScenarioFile(t, body)); err == nil {
				t.Fatal("expected an error for a negative deadline_seconds")
			}
		})
	}
}

// TestLoadBenchScenariosRejectsAttemptsBelowOne enforces attempts >= 1.
func TestLoadBenchScenariosRejectsAttemptsBelowOne(t *testing.T) {
	for _, body := range []string{
		`{"scenarios":[{"id":"B01","attempts":0,"steps":[{"argv":["health"]}]}]}`,
		`{"scenarios":[{"id":"B01","attempts":-2,"steps":[{"argv":["health"]}]}]}`,
	} {
		if _, err := loadBenchScenarios(writeBenchScenarioFile(t, body)); err == nil {
			t.Fatalf("expected an error for %s", body)
		}
	}
}

// TestLoadBenchScenariosRejectsRetryBeyondAttempts is the contradictions rule:
// a declared cap must cover the step's retry budget.
func TestLoadBenchScenariosRejectsRetryBeyondAttempts(t *testing.T) {
	body := `{"scenarios":[{"id":"B01","attempts":2,"steps":[{"argv":["health"],"retry":2}]}]}`
	if _, err := loadBenchScenarios(writeBenchScenarioFile(t, body)); err == nil {
		t.Fatal("expected an error for retry exceeding the attempts cap")
	}
}

// TestLoadBenchScenariosAcceptsRetryWhenAttemptsAbsent keeps the existing
// scenario files working: attempts is a cap only when declared.
func TestLoadBenchScenariosAcceptsRetryWhenAttemptsAbsent(t *testing.T) {
	body := `{"scenarios":[{"id":"B01","steps":[{"argv":["health"],"retry":5}]}]}`
	if _, err := loadBenchScenarios(writeBenchScenarioFile(t, body)); err != nil {
		t.Fatalf("a retry budget without a declared attempts cap must load: %v", err)
	}
}

// TestLoadBenchScenariosRejectsBadContextName rejects anything --context would
// misread as a flag or as a path.
func TestLoadBenchScenariosRejectsBadContextName(t *testing.T) {
	for _, name := range []string{"-work", "../work", "a/b", `a\b`} {
		body := `{"scenarios":[{"id":"B01","context":` + quoteJSON(name) + `,"steps":[{"argv":["health"]}]}]}`
		if _, err := loadBenchScenarios(writeBenchScenarioFile(t, body)); err == nil {
			t.Fatalf("expected an error for context %q", name)
		}
	}
}

// TestLoadBenchScenariosRejectsTraversalID proves a scenario id cannot escape
// --evidence-dir. writeBenchEvidence uses the id as a directory component, so
// "../x" would write outside the evidence root.
func TestLoadBenchScenariosRejectsTraversalID(t *testing.T) {
	for i, id := range []string{"../x", `..\x`, "a/b", `a\b`, "..", "-x"} {
		body := `{"scenarios":[{"id":` + quoteJSON(id) + `,"steps":[{"argv":["health"]}]}]}`
		if _, err := loadBenchScenarios(writeBenchScenarioFile(t, body)); err == nil {
			t.Fatalf("case %d: expected an error for scenario id %q", i, id)
		}
	}
	body := `{"scenarios":[{"id":"B01","steps":[{"argv":["health"]}]}]}`
	if _, err := loadBenchScenarios(writeBenchScenarioFile(t, body)); err != nil {
		t.Fatalf("an ordinary id must validate: %v", err)
	}
}

// TestLoadBenchScenariosRejectsContextStepConflict refuses a file that names
// both a scenario context and a step-level one: one of them would silently win.
func TestLoadBenchScenariosRejectsContextStepConflict(t *testing.T) {
	body := `{"scenarios":[{"id":"B01","context":"work","steps":[{"argv":["--context","other","health"]}]}]}`
	if _, err := loadBenchScenarios(writeBenchScenarioFile(t, body)); err == nil {
		t.Fatal("expected an error for a step that contradicts the scenario context")
	}
}

// TestLoadBenchScenariosRejectsBadUsage covers the usage object's invariants.
func TestLoadBenchScenariosRejectsBadUsage(t *testing.T) {
	cases := map[string]string{
		"missing model":   `{"model":"","tokens_in":1,"tokens_out":1}`,
		"negative input":  `{"model":"m","tokens_in":-1,"tokens_out":1}`,
		"negative output": `{"model":"m","tokens_in":1,"tokens_out":-1}`,
	}
	for name, usage := range cases {
		t.Run(name, func(t *testing.T) {
			body := `{"scenarios":[{"id":"B01","usage":` + usage + `,"steps":[{"argv":["health"]}]}]}`
			if _, err := loadBenchScenarios(writeBenchScenarioFile(t, body)); err == nil {
				t.Fatalf("expected an error for usage %s", usage)
			}
		})
	}
}

// TestLoadBenchScenariosRejectsUnknownNewField proves the strict decoder still
// rejects a typo next to the new fields.
func TestLoadBenchScenariosRejectsUnknownNewField(t *testing.T) {
	body := `{"scenarios":[{"id":"B01","requires_networks":true,"steps":[{"argv":["health"]}]}]}`
	if _, err := loadBenchScenarios(writeBenchScenarioFile(t, body)); err == nil {
		t.Fatal("expected an error for an unknown field")
	}
}

// TestRunBenchScenarioRequiresNetworkIsNotRun is the offline-contract guard: a
// needs-network scenario must never run, and never count as a pass, unless the
// run opted in.
func TestRunBenchScenarioRequiresNetworkIsNotRun(t *testing.T) {
	withBenchAllowNetwork(t, false)
	s := BenchScenario{
		ID:              "B01",
		RequiresNetwork: true,
		Steps:           []BenchStep{{Argv: []string{"open", "https://x"}}},
	}
	called := false
	runner := func(step BenchStep) BenchStepResult {
		called = true
		return BenchStepResult{OK: true, Attempts: 1}
	}
	res := runBenchScenario(s, runner)
	if res.Verdict != VerdictNotRun {
		t.Fatalf("verdict = %q, want %q", res.Verdict, VerdictNotRun)
	}
	if called {
		t.Error("a requires_network scenario must not execute any step")
	}
	if res.Commands != 0 {
		t.Errorf("commands = %d, want 0", res.Commands)
	}
	if !strings.Contains(res.FailureMode, "--allow-network") {
		t.Errorf("failure_mode = %q, should name the opt-in flag", res.FailureMode)
	}
	// NOT_RUN must never be counted as a pass.
	sum := summarizeBenchReport([]BenchScenarioResult{res}, 0)
	if sum.Pass != 0 || sum.NotRun != 1 {
		t.Errorf("summary = %+v, want pass 0 and not_run 1", sum)
	}
}

// TestRunBenchScenarioRequiresNetworkRunsWithOptIn proves the flag is what
// gates execution.
func TestRunBenchScenarioRequiresNetworkRunsWithOptIn(t *testing.T) {
	withBenchAllowNetwork(t, true)
	s := BenchScenario{
		ID:              "B01",
		RequiresNetwork: true,
		Steps:           []BenchStep{{Argv: []string{"open", "https://x"}}},
	}
	res := runBenchScenario(s, fakeRunner(nil))
	if res.Verdict != VerdictPass {
		t.Fatalf("verdict = %q, want %q", res.Verdict, VerdictPass)
	}
}

// TestRunBenchScenarioEchoesUsage checks the driver-supplied accounting lands
// in the result unchanged.
func TestRunBenchScenarioEchoesUsage(t *testing.T) {
	s := BenchScenario{
		ID:    "B01",
		Usage: &BenchUsage{Model: "m", TokensIn: 1200, TokensOut: 340},
		Steps: []BenchStep{{Argv: []string{"health"}}},
	}
	res := runBenchScenario(s, fakeRunner(nil))
	if res.Usage == nil || *res.Usage != *s.Usage {
		t.Fatalf("usage = %+v, want %+v", res.Usage, s.Usage)
	}
}

// TestRunBenchScenarioSkippedScenariosDoNotBillUsage proves a NOT_RUN scenario
// contributes no tokens. The driver's accounting describes an attempt; a
// skipped scenario never made one, so counting it would bill a run that never
// happened.
func TestRunBenchScenarioSkippedScenariosDoNotBillUsage(t *testing.T) {
	withBenchAllowNetwork(t, false)
	auth := BenchScenario{
		ID:           "B01",
		RequiresAuth: true,
		Usage:        &BenchUsage{Model: "m", TokensIn: 5000, TokensOut: 800},
		Steps:        []BenchStep{{Argv: []string{"open", "https://x"}}},
	}
	network := BenchScenario{
		ID:              "B02",
		RequiresNetwork: true,
		Usage:           &BenchUsage{Model: "m", TokensIn: 4000, TokensOut: 800},
		Steps:           []BenchStep{{Argv: []string{"open", "https://x"}}},
	}
	results := []BenchScenarioResult{
		runBenchScenario(auth, fakeRunner(nil)),
		runBenchScenario(network, fakeRunner(nil)),
	}
	for _, res := range results {
		if res.Verdict != VerdictNotRun {
			t.Fatalf("%s verdict = %q, want NOT_RUN", res.ID, res.Verdict)
		}
		if res.Usage != nil {
			t.Errorf("%s usage = %+v, want none for a skipped scenario", res.ID, res.Usage)
		}
	}
	sum := summarizeBenchReport(results, 0)
	if sum.Usage != nil {
		t.Fatalf("summary usage = %+v, want absent when every scenario was skipped", sum.Usage)
	}
}

// TestBenchReportOmitsAbsentUsage ensures an unmeasured scenario does not
// report a zero that would read as "free".
func TestBenchReportOmitsAbsentUsage(t *testing.T) {
	res := runBenchScenario(BenchScenario{ID: "B01", Steps: []BenchStep{{Argv: []string{"health"}}}}, fakeRunner(nil))
	sum := summarizeBenchReport([]BenchScenarioResult{res}, 10)
	data, err := json.Marshal(struct {
		Result  BenchScenarioResult `json:"result"`
		Summary BenchSummary        `json:"summary"`
	}{res, sum})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"usage"`) {
		t.Fatalf("JSON for an unmeasured scenario must omit usage: %s", data)
	}
	if got := formatBenchUsage(res.Usage); got != "—" {
		t.Errorf("human usage = %q, want an em-dash", got)
	}
	if got := formatBenchUsageTotals(sum.Usage); got != "—" {
		t.Errorf("human usage total = %q, want an em-dash", got)
	}
}

// TestSummarizeBenchReportTotalsUsageAndTimeout covers the run-level totals.
func TestSummarizeBenchReportTotalsUsageAndTimeout(t *testing.T) {
	results := []BenchScenarioResult{
		{Verdict: VerdictPass, Usage: &BenchUsage{Model: "m", TokensIn: 100, TokensOut: 10}},
		{Verdict: VerdictTimeout, Usage: &BenchUsage{Model: "m", TokensIn: 200, TokensOut: 20}},
	}
	sum := summarizeBenchReport(results, 0)
	if sum.Timeout != 1 {
		t.Errorf("timeout = %d, want 1", sum.Timeout)
	}
	if sum.Pass != 1 || sum.Fail != 0 {
		t.Errorf("a timeout must not count as a pass: %+v", sum)
	}
	if sum.Usage == nil || sum.Usage.TokensIn != 300 || sum.Usage.TokensOut != 30 {
		t.Fatalf("usage totals = %+v", sum.Usage)
	}
}

// TestRunBenchScenarioInjectsContextIntoEveryStepArgv proves every step runs
// with the named context and that the reported argv shows it.
func TestRunBenchScenarioInjectsContextIntoEveryStepArgv(t *testing.T) {
	s := BenchScenario{
		ID:      "B01",
		Context: "work",
		Steps: []BenchStep{
			{Argv: []string{"open", "https://x"}},
			{Argv: []string{"extract", "table", "table"}},
		},
	}
	var seen [][]string
	runner := func(step BenchStep) BenchStepResult {
		seen = append(seen, step.Argv)
		return BenchStepResult{OK: true, Attempts: 1, Argv: benchArgv(step)}
	}
	res := runBenchScenario(s, runner)
	if len(seen) != 2 {
		t.Fatalf("saw %d steps, want 2", len(seen))
	}
	for i, argv := range seen {
		if len(argv) < 3 || argv[0] != "--context" || argv[1] != "work" {
			t.Fatalf("step %d argv = %v, want a --context work prefix", i+1, argv)
		}
	}
	for i, sr := range res.Steps {
		if len(sr.Argv) < 3 || sr.Argv[0] != "--context" || sr.Argv[1] != "work" {
			t.Fatalf("reported step %d argv = %v, want a --context work prefix", i+1, sr.Argv)
		}
	}
}

// TestRunBenchScenarioStepDeadlineInheritance checks the inheritance rule: a
// step without its own deadline gets the scenario's, an explicit one wins, and
// the remaining scenario budget caps either.
func TestRunBenchScenarioStepDeadlineInheritance(t *testing.T) {
	s := BenchScenario{
		ID:              "B01",
		DeadlineSeconds: 30,
		Steps: []BenchStep{
			{Argv: []string{"first"}},
			{Argv: []string{"second"}, DeadlineSeconds: 5},
		},
	}
	var deadlines []time.Duration
	runner := func(step BenchStep) BenchStepResult {
		deadlines = append(deadlines, step.deadline)
		return BenchStepResult{OK: true, Attempts: 1, Argv: benchArgv(step)}
	}
	runBenchScenario(s, runner)
	if len(deadlines) != 2 {
		t.Fatalf("saw %d deadlines, want 2", len(deadlines))
	}
	// A little headroom: the inherited value is the scenario budget minus the
	// elapsed time of the (instant) first step.
	if deadlines[0] <= 20*time.Second || deadlines[0] > 30*time.Second {
		t.Errorf("inherited deadline = %s, want the 30s scenario value (less elapsed)", deadlines[0])
	}
	if deadlines[1] != 5*time.Second {
		t.Errorf("explicit step deadline = %s, want 5s", deadlines[1])
	}
}

// TestBenchEffectiveDeadline covers the cap and the exhausted case directly.
func TestBenchEffectiveDeadline(t *testing.T) {
	cases := []struct {
		name      string
		scenario  time.Duration
		step      time.Duration
		elapsed   time.Duration
		want      time.Duration
		exhausted bool
	}{
		{name: "no deadlines", want: 0},
		{name: "step only", step: 5 * time.Second, want: 5 * time.Second},
		{name: "scenario inherited", scenario: 30 * time.Second, want: 30 * time.Second},
		{name: "step shorter than budget", scenario: 30 * time.Second, step: 5 * time.Second, want: 5 * time.Second},
		{name: "budget caps a longer step", scenario: 30 * time.Second, step: 60 * time.Second, elapsed: 10 * time.Second, want: 20 * time.Second},
		{name: "budget exhausted", scenario: 30 * time.Second, elapsed: 30 * time.Second, exhausted: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, exhausted := benchEffectiveDeadline(tc.scenario, tc.step, tc.elapsed)
			if exhausted != tc.exhausted || got != tc.want {
				t.Fatalf("= (%s,%v), want (%s,%v)", got, exhausted, tc.want, tc.exhausted)
			}
		})
	}
}

// TestRunBenchScenarioRequiredStepTimeout proves a timed-out required step
// fails the scenario as TIMEOUT, not FAIL/PASS.
func TestRunBenchScenarioRequiredStepTimeout(t *testing.T) {
	s := BenchScenario{ID: "B01", Steps: []BenchStep{{Argv: []string{"wait"}}}}
	runner := func(step BenchStep) BenchStepResult {
		return BenchStepResult{OK: false, Attempts: 1, TimedOut: true, Failure: "timed out after 5s", Argv: benchArgv(step)}
	}
	res := runBenchScenario(s, runner)
	if res.Verdict != VerdictTimeout {
		t.Fatalf("verdict = %q, want %q", res.Verdict, VerdictTimeout)
	}
	if !res.Steps[0].TimedOut {
		t.Error("the step result must carry timed_out")
	}
	if !strings.Contains(res.FailureMode, "timed out") {
		t.Errorf("failure_mode = %q, should say the step timed out", res.FailureMode)
	}
}

// TestRunBenchScenarioOptionalStepTimeoutIsPartial keeps the existing optional
// rule: only a required timeout fails the scenario.
func TestRunBenchScenarioOptionalStepTimeoutIsPartial(t *testing.T) {
	s := BenchScenario{ID: "B01", Steps: []BenchStep{{Argv: []string{"wait"}, Optional: true}}}
	runner := func(step BenchStep) BenchStepResult {
		return BenchStepResult{OK: false, Attempts: 1, TimedOut: true, Failure: "timed out", Argv: benchArgv(step)}
	}
	if res := runBenchScenario(s, runner); res.Verdict != VerdictPartial {
		t.Fatalf("verdict = %q, want %q", res.Verdict, VerdictPartial)
	}
}

// TestRunBenchScenarioExhaustedBudgetIsTimeout proves the scenario budget stops
// later steps: the first step eats the whole second, so the second never runs.
func TestRunBenchScenarioExhaustedBudgetIsTimeout(t *testing.T) {
	s := BenchScenario{
		ID:              "B01",
		DeadlineSeconds: 1,
		Steps: []BenchStep{
			{Argv: []string{"first"}},
			{Argv: []string{"second"}},
		},
	}
	calls := 0
	runner := func(step BenchStep) BenchStepResult {
		calls++
		time.Sleep(1100 * time.Millisecond)
		return BenchStepResult{OK: true, Attempts: 1, WallMS: 1100, Argv: benchArgv(step)}
	}
	res := runBenchScenario(s, runner)
	if calls != 1 {
		t.Fatalf("runner called %d times, want 1 (the budget was exhausted)", calls)
	}
	if res.Verdict != VerdictTimeout {
		t.Fatalf("verdict = %q, want %q", res.Verdict, VerdictTimeout)
	}
	if !res.Steps[1].TimedOut {
		t.Fatal("the skipped step must be reported as timed out")
	}
	if res.Steps[1].Attempts != 0 {
		t.Errorf("a step that never started reports attempts = %d, want 0", res.Steps[1].Attempts)
	}
	if res.Commands != 1 {
		t.Errorf("commands = %d, want 1", res.Commands)
	}
	if res.Retries != 0 {
		t.Errorf("retries = %d, want 0 (a skipped step made no attempt)", res.Retries)
	}
}

// TestRunCmdWithDeadlineKillsSlowProcess proves the deadline really kills the
// child. The test re-executes itself as the child so no shell is required.
func TestRunCmdWithDeadlineKillsSlowProcess(t *testing.T) {
	if os.Getenv("HTRCLI_BENCH_DEADLINE_HELPER") == "1" {
		time.Sleep(30 * time.Second)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestRunCmdWithDeadlineKillsSlowProcess")
	cmd.Env = append(os.Environ(), "HTRCLI_BENCH_DEADLINE_HELPER=1")
	run, timedOut := runCmdWithDeadline(cmd, time.Second)
	if !timedOut {
		t.Fatalf("timedOut = false, stderr = %q", run.Stderr)
	}
	if run.Duration > 5*time.Second {
		t.Fatalf("child ran for %s; the deadline did not kill it", run.Duration)
	}
}

// TestFinishBenchRunFailsOnTimeout keeps a hanging suite from exiting 0.
func TestFinishBenchRunFailsOnTimeout(t *testing.T) {
	report := BenchReport{Summary: BenchSummary{Total: 1, Timeout: 1}}
	old := output.JSONOutput
	defer func() { output.JSONOutput = old }()
	output.JSONOutput = true
	if err := finishBenchRun(report, filepath.Join(t.TempDir(), "r.json")); err == nil {
		t.Fatal("a timed-out scenario must make the run exit non-zero")
	}
}

// TestRunCmdWithDeadlineWithoutDeadline runs the child to completion: a zero
// deadline must not start a timer that kills a healthy step.
func TestRunCmdWithDeadlineWithoutDeadline(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	run, timedOut := runCmdWithDeadline(cmd, 0)
	if timedOut {
		t.Fatal("a zero deadline must not report a timeout")
	}
	if run.ExitCode != 0 {
		t.Fatalf("exit code = %d, stderr = %q", run.ExitCode, run.Stderr)
	}
}

// quoteJSON renders a string as a JSON string literal for fixture bodies.
func quoteJSON(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}
