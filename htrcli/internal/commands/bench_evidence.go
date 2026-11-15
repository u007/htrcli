package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// BenchEvidenceStep is the full, untruncated record of one step, written to
// --evidence-dir as <dir>/<scenario-id>/<NNN>-step.json.
//
// Screenshots are NOT captured: that would need an extra browser call per step,
// so an artifact is command output, never a visual record. Do not read this
// format as if it contained images.
type BenchEvidenceStep struct {
	// Index is the 1-based step number, matching the file name.
	Index int `json:"index"`
	// Note is the scenario's label for the step.
	Note string `json:"note,omitempty"`
	// Argv is the argv that was executed.
	Argv []string `json:"argv"`
	// ExitCode is the child's exit code.
	ExitCode int `json:"exit_code"`
	// Attempts is how many times the step ran.
	Attempts int `json:"attempts"`
	// WallMS is the step's wall-clock time in milliseconds.
	WallMS int64 `json:"wall_ms"`
	// TimedOut reports that the step exceeded its deadline and was killed.
	TimedOut bool `json:"timed_out"`
	// OK reports whether the step met its expectations.
	OK bool `json:"ok"`
	// Failure describes why the step failed.
	Failure string `json:"failure,omitempty"`
	// Stdout is the FULL untruncated stdout (unlike the report's capped copy).
	Stdout string `json:"stdout"`
	// Stderr is the FULL untruncated stderr.
	Stderr string `json:"stderr"`
}

// writeBenchEvidence writes one artifact per step of a finished scenario.
//
// It is called whether the scenario passed or failed: evidence of a pass is as
// useful as evidence of a failure. A scenario with no steps writes nothing.
func writeBenchEvidence(dir string, res BenchScenarioResult) error {
	if len(res.Steps) == 0 {
		return nil
	}
	scenarioDir := filepath.Join(dir, res.ID)
	if err := os.MkdirAll(scenarioDir, 0o755); err != nil {
		return fmt.Errorf("create evidence directory %s: %w", scenarioDir, err)
	}
	for _, step := range res.Steps {
		artifact := BenchEvidenceStep{
			Index:    step.Index,
			Note:     step.Note,
			Argv:     step.Argv,
			ExitCode: step.ExitCode,
			Attempts: step.Attempts,
			WallMS:   step.WallMS,
			TimedOut: step.TimedOut,
			OK:       step.OK,
			Failure:  step.Failure,
			Stdout:   step.FullStdout,
			Stderr:   step.FullStderr,
		}
		data, err := json.MarshalIndent(artifact, "", "  ")
		if err != nil {
			return fmt.Errorf("encode evidence for step %d: %w", step.Index, err)
		}
		data = append(data, '\n')
		name := filepath.Join(scenarioDir, fmt.Sprintf("%03d-step.json", step.Index))
		if err := os.WriteFile(name, data, 0o644); err != nil {
			return fmt.Errorf("write evidence %s: %w", name, err)
		}
	}
	return nil
}
