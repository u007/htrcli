package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteBenchEvidenceWritesFullStreams proves one artifact per step, with
// the untruncated streams, written for a passing step too.
func TestWriteBenchEvidenceWritesFullStreams(t *testing.T) {
	dir := t.TempDir()
	res := BenchScenarioResult{
		ID: "B01",
		Steps: []BenchStepResult{
			{
				Index: 1, Note: "open", Argv: []string{"open", "https://x"},
				OK: true, ExitCode: 0, Attempts: 1, WallMS: 12,
				Stdout: "capped", FullStdout: strings.Repeat("s", benchReportMaxStream+50),
				Stderr: "capped-err", FullStderr: strings.Repeat("e", benchReportMaxStream+50),
			},
			{
				Index: 2, Argv: []string{"extract", "table", "table"},
				OK: false, ExitCode: 1, Attempts: 2, WallMS: 34, TimedOut: true,
				Failure: "killed", FullStdout: "boom", FullStderr: "err",
			},
		},
	}
	if err := writeBenchEvidence(dir, res); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	first := filepath.Join(dir, "B01", "001-step.json")
	second := filepath.Join(dir, "B01", "002-step.json")
	for _, path := range []string{first, second} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("missing evidence artifact %s: %v", path, err)
		}
	}
	data, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	var artifact BenchEvidenceStep
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatalf("evidence is not valid JSON: %v", err)
	}
	if artifact.OK != true || artifact.Attempts != 1 || artifact.WallMS != 12 {
		t.Errorf("artifact = %+v", artifact)
	}
	if len(artifact.Stdout) != benchReportMaxStream+50 {
		t.Errorf("stdout length = %d, want the full untruncated stream", len(artifact.Stdout))
	}
	if len(artifact.Stderr) != benchReportMaxStream+50 {
		t.Errorf("stderr length = %d, want the full untruncated stream", len(artifact.Stderr))
	}
	if strings.Contains(artifact.Stdout, "truncated") {
		t.Error("evidence must not carry the report's truncation marker")
	}

	data, err = os.ReadFile(second)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatal(err)
	}
	if artifact.TimedOut != true || artifact.Failure == "" {
		t.Errorf("second artifact = %+v, want the timeout and failure recorded", artifact)
	}
}

// TestWriteBenchEvidenceUncreatablePathIsAnError proves a bad evidence path is
// a hard error, not a silent skip.
func TestWriteBenchEvidenceUncreatablePathIsAnError(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := BenchScenarioResult{ID: "B01", Steps: []BenchStepResult{{Index: 1, OK: true}}}
	if err := writeBenchEvidence(blocker, res); err == nil {
		t.Fatal("expected an error when the evidence directory cannot be created")
	}
}

// TestWriteBenchEvidenceSkipsScenariosWithoutSteps keeps a NOT_RUN/EXCLUDED
// scenario from creating an empty directory.
func TestWriteBenchEvidenceSkipsScenariosWithoutSteps(t *testing.T) {
	dir := t.TempDir()
	if err := writeBenchEvidence(dir, BenchScenarioResult{ID: "B01", Verdict: VerdictExcluded}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "B01")); !os.IsNotExist(err) {
		t.Fatalf("a scenario with no steps must write nothing, stat err = %v", err)
	}
}

// TestLoadBenchExclusionsValid returns the id -> reason map.
func TestLoadBenchExclusionsValid(t *testing.T) {
	path := writeBenchExclusionsFile(t, `{"excluded":[{"id":"B01","reason":"no browser"},{"id":"B02","reason":"flaky"}]}`)
	scenarios := []BenchScenario{{ID: "B01"}, {ID: "B02"}, {ID: "B03"}}
	reasons, err := loadBenchExclusions(path, scenarios)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reasons["B01"] != "no browser" || reasons["B02"] != "flaky" {
		t.Fatalf("reasons = %+v", reasons)
	}
	if _, ok := reasons["B03"]; ok {
		t.Error("B03 was not listed and must not be excluded")
	}
}

// TestLoadBenchExclusionsRejectsBadFiles covers every exclusion invariant.
func TestLoadBenchExclusionsRejectsBadFiles(t *testing.T) {
	cases := map[string]string{
		"unknown id":    `{"excluded":[{"id":"B99","reason":"typo"}]}`,
		"empty reason":  `{"excluded":[{"id":"B01","reason":""}]}`,
		"empty id":      `{"excluded":[{"id":"","reason":"x"}]}`,
		"duplicate id":  `{"excluded":[{"id":"B01","reason":"a"},{"id":"B01","reason":"b"}]}`,
		"unknown field": `{"excluded":[{"id":"B01","reason":"a","note":"typo"}]}`,
	}
	scenarios := []BenchScenario{{ID: "B01"}}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := loadBenchExclusions(writeBenchExclusionsFile(t, body), scenarios); err == nil {
				t.Fatalf("expected an error for %s", name)
			}
		})
	}
}

// TestExcludedScenarioIsCountedSeparately proves an exclusion never runs a step
// and never counts as a pass.
func TestExcludedScenarioIsCountedSeparately(t *testing.T) {
	s := BenchScenario{ID: "B01", Steps: []BenchStep{{Argv: []string{"health"}}}}
	res := excludedBenchResult(s, "no browser")
	if res.Verdict != VerdictExcluded {
		t.Fatalf("verdict = %q, want %q", res.Verdict, VerdictExcluded)
	}
	if res.Commands != 0 || len(res.Steps) != 0 {
		t.Errorf("excluded result = %+v, want no commands and no steps", res)
	}
	if res.FailureMode != "no browser" {
		t.Errorf("failure_mode = %q, want the reason", res.FailureMode)
	}
	sum := summarizeBenchReport([]BenchScenarioResult{res}, 0)
	if sum.Excluded != 1 || sum.Pass != 0 {
		t.Fatalf("summary = %+v, want excluded 1 and pass 0", sum)
	}
}

// writeBenchExclusionsFile writes body to a temporary exclusions file.
func writeBenchExclusionsFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "exclusions.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
