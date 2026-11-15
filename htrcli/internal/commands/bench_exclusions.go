package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
)

// BenchExclusion lists one scenario id to skip, with a reason.
type BenchExclusion struct {
	// ID is the scenario id to exclude. It must exist in the scenario file.
	ID string `json:"id"`
	// Reason explains the exclusion. It must be non-empty so an EXCLUDED
	// verdict is never silent.
	Reason string `json:"reason"`
}

// BenchExclusionsFile is the --exclusions document format.
type BenchExclusionsFile struct {
	Excluded []BenchExclusion `json:"excluded"`
}

// loadBenchExclusions reads and validates an exclusions file, returning a
// scenario-id -> reason map.
//
// An exclusion naming an id absent from the scenario file is a hard error: a
// typo would otherwise silently exclude nothing.
func loadBenchExclusions(path string, scenarios []BenchScenario) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read exclusions file: %w", err)
	}
	var file BenchExclusionsFile
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&file); err != nil {
		return nil, fmt.Errorf("parse exclusions file %s: %w", path, err)
	}
	known := make(map[string]struct{}, len(scenarios))
	for _, s := range scenarios {
		known[s.ID] = struct{}{}
	}
	reasons := make(map[string]string, len(file.Excluded))
	for i, ex := range file.Excluded {
		if ex.ID == "" {
			return nil, fmt.Errorf("exclusion %d has an empty id", i+1)
		}
		if ex.Reason == "" {
			return nil, fmt.Errorf("exclusion %d (%s) has an empty reason", i+1, ex.ID)
		}
		if _, ok := known[ex.ID]; !ok {
			return nil, fmt.Errorf("exclusion names unknown scenario id %q", ex.ID)
		}
		if _, dup := reasons[ex.ID]; dup {
			return nil, fmt.Errorf("exclusion repeats scenario id %q", ex.ID)
		}
		reasons[ex.ID] = ex.Reason
	}
	return reasons, nil
}

// excludedBenchResult builds the result for a scenario skipped by
// --exclusions: no steps ran, so it has no commands and is never a PASS.
func excludedBenchResult(s BenchScenario, reason string) BenchScenarioResult {
	return BenchScenarioResult{
		ID:          s.ID,
		Ref:         s.Ref,
		App:         s.App,
		Goal:        s.Goal,
		Verdict:     VerdictExcluded,
		FailureMode: reason,
		Steps:       make([]BenchStepResult, 0),
	}
}
