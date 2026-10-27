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

func TestExpandBenchRefKeepsCurrencyLiteral(t *testing.T) {
	caps := map[string]string{"balance": "4820.50"}
	lookup := func(name string) (string, bool) {
		v, ok := caps[name]
		return v, ok
	}
	cases := []struct{ in, want string }{
		// Currency is ordinary benchmark data. os.Expand read the digit after
		// "$" as a variable name and rewrote these, which silently changed the
		// expected value once ANY capture existed (S22 caught it).
		{"$4,820.50", "$4,820.50"},
		{"+$520.50", "+$520.50"},
		{"-$117.50", "-$117.50"},
		{"$12,483.07", "$12,483.07"},
		{"$-4.65", "$-4.65"},
		{"$", "$"},
		// Real references still expand, braced or bare.
		{"${balance}", "4820.50"},
		{"$balance", "4820.50"},
		{"balance=${balance}", "balance=4820.50"},
		// An unknown name keeps its original form.
		{"cost ${missing} and $9.99", "cost ${missing} and $9.99"},
		{"$missing", "$missing"},
	}
	for _, c := range cases {
		if got := expandBenchRef(c.in, lookup); got != c.want {
			t.Errorf("expandBenchRef(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSubstituteExpectCapturesLeavesCurrencyAlone(t *testing.T) {
	caps := map[string]string{"balance": "4820.50"}

	got := substituteExpectCaptures(&BenchExpect{
		JSONPath: &JSONPathExpect{Path: "data", Equals: "$4,760.50"},
	}, caps)
	if got.JSONPath.Equals != "$4,760.50" {
		t.Errorf("currency expectation rewritten to %q", got.JSONPath.Equals)
	}

	got = substituteExpectCaptures(&BenchExpect{
		JSONPath: &JSONPathExpect{Path: "data", Equals: "${balance}"},
	}, caps)
	if got.JSONPath.Equals != "4820.50" {
		t.Errorf("captured reference = %q, want 4820.50", got.JSONPath.Equals)
	}
}
