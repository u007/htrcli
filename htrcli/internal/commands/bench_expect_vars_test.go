package commands

import "testing"

func TestExpandBenchExpectExpandsVariables(t *testing.T) {
	t.Setenv("BENCH_FIXTURE_X", "/tmp/x.html")
	got := expandBenchExpect(&BenchExpect{
		Exit:              0,
		StdoutContains:    "file://${BENCH_FIXTURE_X}",
		StdoutNotContains: "nope-${BENCH_FIXTURE_X}",
		JSONPath:          &JSONPathExpect{Path: "data", Equals: "file://${BENCH_FIXTURE_X}"},
	})
	if got.StdoutContains != "file:///tmp/x.html" {
		t.Errorf("stdoutContains = %q", got.StdoutContains)
	}
	if got.StdoutNotContains != "nope-/tmp/x.html" {
		t.Errorf("stdoutNotContains = %q", got.StdoutNotContains)
	}
	if got.JSONPath == nil || got.JSONPath.Equals != "file:///tmp/x.html" {
		t.Errorf("jsonPath.equals = %v", got.JSONPath)
	}
}

func TestExpandBenchExpectLeavesUnsetAndNil(t *testing.T) {
	if got := expandBenchExpect(nil); got != nil {
		t.Errorf("nil expect = %v, want nil", got)
	}
	got := expandBenchExpect(&BenchExpect{StdoutContains: "keep ${NOT_SET_ANYWHERE}"})
	if got.StdoutContains != "keep ${NOT_SET_ANYWHERE}" {
		t.Errorf("unset variable = %q, want it left literal", got.StdoutContains)
	}
}

func TestExpandBenchExpectDoesNotMutateInput(t *testing.T) {
	t.Setenv("BENCH_FIXTURE_Y", "/tmp/y.html")
	orig := &BenchExpect{StdoutContains: "${BENCH_FIXTURE_Y}", JSONPath: &JSONPathExpect{Path: "data", Equals: "${BENCH_FIXTURE_Y}"}}
	_ = expandBenchExpect(orig)
	if orig.StdoutContains != "${BENCH_FIXTURE_Y}" {
		t.Errorf("original stdoutContains mutated to %q", orig.StdoutContains)
	}
	if orig.JSONPath.Equals != "${BENCH_FIXTURE_Y}" {
		t.Errorf("original jsonPath.equals mutated to %q", orig.JSONPath.Equals)
	}
}
