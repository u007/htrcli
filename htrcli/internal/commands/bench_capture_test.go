package commands

import (
	"strings"
	"testing"
)

// captureRunner records the argv each step received and answers from a table.
func captureRunner(results map[string]BenchStepResult, seen *[]string) benchRunner {
	return func(step BenchStep) BenchStepResult {
		*seen = append(*seen, strings.Join(step.Argv, " "))
		key := strings.Join(step.Argv, " ")
		if r, ok := results[key]; ok {
			return r
		}
		return BenchStepResult{OK: true, ExitCode: 0, Attempts: 1, WallMS: 1}
	}
}

func TestRunBenchScenarioCapturesAndSubstitutes(t *testing.T) {
	var seen []string
	runner := captureRunner(map[string]BenchStepResult{
		"extract list #metrics": {
			OK:       true,
			ExitCode: 0,
			Stdout:   `{"items":[{"metric":"Revenue","q4":"15.1M"}]}`,
		},
	}, &seen)

	s := BenchScenario{
		ID: "B14",
		Steps: []BenchStep{
			{
				Argv:    []string{"extract", "list", "#metrics"},
				Capture: map[string]string{"revenue": "items.0.q4"},
			},
			{
				Argv: []string{"fill", "#summary", "Revenue closed at ${revenue} in Q4."},
			},
		},
	}
	res := runBenchScenario(s, runner)
	if res.Verdict != VerdictPass {
		t.Fatalf("verdict = %q (%s), want PASS", res.Verdict, res.FailureMode)
	}
	if got := res.Steps[0].Captured["revenue"]; got != "15.1M" {
		t.Errorf("captured revenue = %q, want 15.1M", got)
	}
	want := "fill #summary Revenue closed at 15.1M in Q4."
	if seen[1] != want {
		t.Fatalf("second argv = %q, want %q", seen[1], want)
	}
}

func TestRunBenchScenarioCaptureMissingPathFails(t *testing.T) {
	var seen []string
	runner := captureRunner(map[string]BenchStepResult{
		"extract list #metrics": {
			OK:       true,
			ExitCode: 0,
			Stdout:   `{"items":[{"metric":"Revenue"}]}`,
		},
	}, &seen)

	s := BenchScenario{
		ID: "B14",
		Steps: []BenchStep{
			{
				Argv:    []string{"extract", "list", "#metrics"},
				Capture: map[string]string{"q4": "items.0.q4"},
			},
			{Argv: []string{"fill", "#summary", "closed at ${q4}"}},
		},
	}
	res := runBenchScenario(s, runner)
	if res.Verdict != VerdictFail {
		t.Fatalf("verdict = %q, want FAIL", res.Verdict)
	}
	if !strings.Contains(res.FailureMode, "capture \"q4\"") {
		t.Errorf("failure_mode = %q, want it to name the capture", res.FailureMode)
	}
	// The value was never captured, so the reference must stay literal rather
	// than becoming an empty string.
	if got := seen[1]; got != "fill #summary closed at ${q4}" {
		t.Errorf("second argv = %q, want the literal ${q4}", got)
	}
}

func TestRunBenchScenarioCaptureSkippedWhenStepFails(t *testing.T) {
	var seen []string
	// The failed step still emits a PARSEABLE value on the path, so a
	// regression that captures regardless of the verdict is observable.
	runner := captureRunner(map[string]BenchStepResult{
		"extract list #metrics": {
			OK:       false,
			ExitCode: 1,
			Failure:  "boom",
			Attempts: 1,
			Stdout:   `{"items":[{"q4":"15.1M"}]}`,
		},
	}, &seen)

	s := BenchScenario{
		ID: "B14",
		Steps: []BenchStep{
			{
				Argv:     []string{"extract", "list", "#metrics"},
				Capture:  map[string]string{"v": "items.0.q4"},
				Optional: true,
			},
			{Argv: []string{"fill", "#s", "${v}"}},
		},
	}
	res := runBenchScenario(s, runner)
	if res.Verdict != VerdictPartial {
		t.Fatalf("verdict = %q, want PARTIAL", res.Verdict)
	}
	if len(res.Steps[0].Captured) != 0 {
		t.Errorf("captured = %v, want none from a failed step", res.Steps[0].Captured)
	}
	if got := seen[1]; got != "fill #s ${v}" {
		t.Errorf("second argv = %q, want the literal ${v}", got)
	}
}

func TestRunBenchScenarioCapturesDoNotLeakBetweenScenarios(t *testing.T) {
	var seen []string
	runner := captureRunner(map[string]BenchStepResult{
		"extract list #m": {
			OK:       true,
			ExitCode: 0,
			Stdout:   `{"items":[{"q4":"15.1M"}]}`,
		},
	}, &seen)

	first := runBenchScenario(BenchScenario{
		ID: "S1",
		Steps: []BenchStep{{
			Argv:    []string{"extract", "list", "#m"},
			Capture: map[string]string{"q4": "items.0.q4"},
		}},
	}, runner)
	if first.Verdict != VerdictPass {
		t.Fatalf("first verdict = %q", first.Verdict)
	}

	runBenchScenario(BenchScenario{
		ID:    "S2",
		Steps: []BenchStep{{Argv: []string{"fill", "#s", "${q4}"}}},
	}, runner)
	if got := seen[len(seen)-1]; got != "fill #s ${q4}" {
		t.Errorf("second scenario argv = %q, want the literal ${q4}", got)
	}
}

func TestSubstituteCapturesLeavesUnknownNames(t *testing.T) {
	got := substituteCaptures([]string{"fill", "#s", "${known}-${unknown}"}, map[string]string{"known": "A"})
	if got[2] != "A-${unknown}" {
		t.Fatalf("argv = %q, want A-${unknown}", got[2])
	}
}

func TestSubstituteCapturesDoesNotRescanValue(t *testing.T) {
	got := substituteCaptures([]string{"fill", "#s", "${a}"}, map[string]string{"a": "${b}", "b": "LEAK"})
	if got[2] != "${b}" {
		t.Fatalf("argv = %q, want the literal ${b}", got[2])
	}
}

func TestSubstituteCapturesWinsOverEnvironment(t *testing.T) {
	t.Setenv("revenue", "from-env")
	got := substituteCaptures([]string{"fill", "#s", "${revenue}"}, map[string]string{"revenue": "from-capture"})
	if got[2] != "from-capture" {
		t.Fatalf("argv = %q, want the captured value to win", got[2])
	}
}

func TestValidateBenchScenarioRejectsBadCaptureNames(t *testing.T) {
	cases := []map[string]string{
		{"": "items.0.v"},
		{"has space": "items.0.v"},
		{"ok_name": ""},
	}
	for _, capture := range cases {
		s := BenchScenario{
			ID:    "S1",
			Steps: []BenchStep{{Argv: []string{"extract", "list", "#m"}, Capture: capture}},
		}
		if err := validateBenchScenario(&s); err == nil {
			t.Errorf("capture %v: want a validation error, got nil", capture)
		}
	}
}

func TestValidateBenchScenarioAcceptsCaptureNames(t *testing.T) {
	s := BenchScenario{
		ID: "S1",
		Steps: []BenchStep{{
			Argv:    []string{"extract", "list", "#m"},
			Capture: map[string]string{"q4_revenue": "items.0.v", "_x9": "items.0.v"},
		}},
	}
	if err := validateBenchScenario(&s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// expectRunner answers each step from a table keyed by its argv and evaluates
// the step's expectations the way runBenchStep does. captureRunner alone is not
// enough for these tests: it reports OK unconditionally, so an expectation
// would never be consulted.
func expectRunner(stdoutByArgv map[string]string) benchRunner {
	return func(step BenchStep) BenchStepResult {
		stdout := stdoutByArgv[strings.Join(step.Argv, " ")]
		run := selfRun{Stdout: stdout, ExitCode: 0}
		res := BenchStepResult{
			Argv:     step.Argv,
			Stdout:   stdout,
			ExitCode: 0,
			Attempts: 1,
		}
		if err := evaluateBenchExpect(expandBenchExpect(step.Expect), run); err != nil {
			res.Failure = err.Error()
			return res
		}
		res.OK = true
		return res
	}
}

// An expectation must be able to reference a captured value: the natural
// cross-check ("the detail view opened on the id I just captured") is
// unwritable otherwise.
func TestRunBenchScenarioExpectationUsesCapture(t *testing.T) {
	runner := expectRunner(map[string]string{
		"--json text #confirmation-id":       `{"data":"TKT-1042"}`,
		"--json attr #detail data-ticket-id": `{"data":"TKT-1042"}`,
	})

	s := BenchScenario{
		ID: "B17",
		Steps: []BenchStep{
			{
				Argv:    []string{"--json", "text", "#confirmation-id"},
				Capture: map[string]string{"ticket_id": "data"},
			},
			{
				Argv: []string{"--json", "attr", "#detail", "data-ticket-id"},
				Expect: &BenchExpect{
					JSONPath: &JSONPathExpect{Path: "data", Equals: "${ticket_id}"},
				},
			},
		},
	}
	res := runBenchScenario(s, runner)
	if res.Verdict != VerdictPass {
		t.Fatalf("verdict = %q, failure = %q, want PASS", res.Verdict, res.Steps[1].Failure)
	}
}

// A capture referenced from an expectation must win over an environment
// variable of the same name, exactly as it does in argv.
func TestRunBenchScenarioExpectationCaptureWinsOverEnvironment(t *testing.T) {
	t.Setenv("ticket_id", "FROM-ENV")
	runner := expectRunner(map[string]string{
		"--json text #confirmation-id": `{"data":"TKT-1042"}`,
		"--json text #detail-id":       `{"data":"TKT-1042"}`,
	})

	s := BenchScenario{
		ID: "B17",
		Steps: []BenchStep{
			{
				Argv:    []string{"--json", "text", "#confirmation-id"},
				Capture: map[string]string{"ticket_id": "data"},
			},
			{
				Argv:   []string{"--json", "text", "#detail-id"},
				Expect: &BenchExpect{StdoutContains: "${ticket_id}"},
			},
		},
	}
	res := runBenchScenario(s, runner)
	if res.Verdict != VerdictPass {
		t.Fatalf("verdict = %q, failure = %q, want PASS (capture must shadow the env var)", res.Verdict, res.Steps[1].Failure)
	}
}

// An expectation naming a value that was never captured keeps its literal form
// and fails loudly, rather than silently matching an empty string.
func TestRunBenchScenarioExpectationUnresolvedCaptureFails(t *testing.T) {
	runner := expectRunner(map[string]string{
		"--json text #detail-id": `{"data":"TKT-1042"}`,
	})

	s := BenchScenario{
		ID: "B17",
		Steps: []BenchStep{
			{
				Argv: []string{"--json", "text", "#detail-id"},
				Expect: &BenchExpect{
					JSONPath: &JSONPathExpect{Path: "data", Equals: "${never_captured}"},
				},
			},
		},
	}
	res := runBenchScenario(s, runner)
	if res.Verdict != VerdictFail {
		t.Fatalf("verdict = %q, want FAIL for an unresolved capture", res.Verdict)
	}
	if !strings.Contains(res.Steps[0].Failure, "never_captured") {
		t.Fatalf("failure = %q, want the unresolved name visible", res.Steps[0].Failure)
	}
}
