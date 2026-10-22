package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ─── Scenario file format ────────────────────────────────────────────
//
// A scenario FILE is a JSON document holding one or more scenarios:
//
//	{
//	  "scenarios": [
//	    {
//	      "id": "B01",
//	      "ref": "B01",                // link back to docs/benchmarks/office-task-benchmark.md
//	      "app": "Gmail web",
//	      "goal": "Search unread mail and return a structured list",
//	      "requires_auth": true,       // reported as NOT_RUN without an authenticated session
//	      "steps": [
//	        {
//	          "argv": ["open", "https://mail.google.com/"],
//	          "expect": {"exit": 0},
//	          "retry": 2,               // prefix "retry --times 2 --" to argv
//	          "optional": false         // a failing optional step downgrades PASS to PARTIAL
//	        }
//	      ]
//	    }
//	  ]
//	}
//
// Steps carry raw htrcli argv rather than a fixed set of step kinds: the CLI
// is the interface under test, and a DSL would duplicate it.

const (
	// VerdictPass means every step met its expectations.
	VerdictPass = "PASS"
	// VerdictPartial means only optional steps failed.
	VerdictPartial = "PARTIAL"
	// VerdictFail means a required step failed.
	VerdictFail = "FAIL"
	// VerdictNotRun means the scenario was skipped (e.g. requires_auth).
	VerdictNotRun = "NOT_RUN"
)

// BenchStep is one htrcli invocation plus its expectations.
type BenchStep struct {
	// Argv is the htrcli argument list, WITHOUT the leading "htrcli".
	Argv []string `json:"argv"`
	// Expect holds the assertions for this step.
	Expect *BenchExpect `json:"expect,omitempty"`
	// Retry is the number of retries for this step (0 = no retry).
	Retry int `json:"retry,omitempty"`
	// Optional marks a step whose failure downgrades the verdict to PARTIAL
	// instead of failing the scenario.
	Optional bool `json:"optional,omitempty"`
	// Note is a human-readable label for the report.
	Note string `json:"note,omitempty"`
}

// BenchExpect holds the assertions for a step. All present fields must hold;
// absent fields are not checked.
type BenchExpect struct {
	// Exit is the expected process exit code. Default 0.
	Exit int `json:"exit,omitempty"`
	// StdoutContains requires the step's stdout to contain this substring.
	StdoutContains string `json:"stdoutContains,omitempty"`
	// StdoutNotContains requires the step's stdout NOT to contain this substring.
	StdoutNotContains string `json:"stdoutNotContains,omitempty"`
	// JSONPath asserts on a value inside the step's stdout parsed as JSON.
	// Only dotted paths into objects and [n] indices into arrays are supported.
	JSONPath *JSONPathExpect `json:"jsonPath,omitempty"`
}

// JSONPathExpect asserts a value at a dotted path in a JSON document.
type JSONPathExpect struct {
	// Path is the dotted path, e.g. "rowCount" or "rows.0.Name".
	Path string `json:"path"`
	// Equals is the expected value, compared as a string after
	// normalisation. Use "exists" in Exists to assert presence only.
	Equals string `json:"equals,omitempty"`
	// Exists asserts the path resolves to a value (including null/empty).
	Exists bool `json:"exists,omitempty"`
}

// BenchScenario is one benchmark scenario.
type BenchScenario struct {
	ID           string      `json:"id"`
	Ref          string      `json:"ref,omitempty"`
	App          string      `json:"app,omitempty"`
	Goal         string      `json:"goal,omitempty"`
	RequiresAuth bool        `json:"requires_auth,omitempty"`
	Steps        []BenchStep `json:"steps"`
}

// BenchScenarioFile is the on-disk scenario file format. It holds one or more
// scenarios so a suite can ship as a single document.
type BenchScenarioFile struct {
	Scenarios []BenchScenario `json:"scenarios"`
}

// loadBenchScenarios reads and validates a scenario file.
func loadBenchScenarios(path string) ([]BenchScenario, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read scenario file: %w", err)
	}
	var file BenchScenarioFile
	// Unknown fields are a hard error: a typo in a scenario file must not
	// silently drop an expectation and turn a FAIL into a PASS.
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&file); err != nil {
		return nil, fmt.Errorf("parse scenario file %s: %w", path, err)
	}
	if len(file.Scenarios) == 0 {
		return nil, fmt.Errorf("scenario file %s contains no scenarios", path)
	}
	for i := range file.Scenarios {
		if err := validateBenchScenario(&file.Scenarios[i]); err != nil {
			return nil, fmt.Errorf("scenario %s: %w", scenarioLabel(&file.Scenarios[i], i), err)
		}
	}
	return file.Scenarios, nil
}

// validateBenchScenario rejects a scenario that could not run.
func validateBenchScenario(s *BenchScenario) error {
	if s.ID == "" {
		return fmt.Errorf("missing id")
	}
	if len(s.Steps) == 0 {
		return fmt.Errorf("has no steps")
	}
	for i, step := range s.Steps {
		if len(step.Argv) == 0 {
			return fmt.Errorf("step %d has no argv", i+1)
		}
		if step.Retry < 0 {
			return fmt.Errorf("step %d has a negative retry count", i+1)
		}
	}
	return nil
}

// scenarioLabel is a display name for a scenario, falling back to its index.
func scenarioLabel(s *BenchScenario, idx int) string {
	if s.ID != "" {
		return s.ID
	}
	return fmt.Sprintf("#%d", idx+1)
}

// ─── JSON path lookup ────────────────────────────────────────────────

// lookupJSONPath resolves a dotted path inside a decoded JSON value.
// It supports object keys and array indices ("rows.0.Name").
func lookupJSONPath(doc any, path string) (any, bool) {
	if path == "" {
		return doc, true
	}
	cur := doc
	for _, seg := range strings.Split(path, ".") {
		if seg == "" {
			return nil, false
		}
		switch node := cur.(type) {
		case map[string]any:
			v, ok := node[seg]
			if !ok {
				return nil, false
			}
			cur = v
		case []any:
			idx, err := strconv.Atoi(seg)
			if err != nil || idx < 0 || idx >= len(node) {
				return nil, false
			}
			cur = node[idx]
		default:
			return nil, false
		}
	}
	return cur, true
}

// normalizeJSONValue renders a JSON value as a comparable string.
func normalizeJSONValue(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case float64:
		// JSON numbers decode as float64; render whole numbers without a
		// trailing ".0" so "30" and 30 compare equal.
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}
