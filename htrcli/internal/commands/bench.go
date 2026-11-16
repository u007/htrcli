package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// ─── Scenario file format ────────────────────────────────────────────
//
// A scenario FILE is a JSON document holding one or more scenarios:
//
//	{
//	  "scenarios": [
//	    {
//	      "id": "B01",
//	      "ref": "B01",                // link back to benchmark.md
//	      "app": "Gmail web",
//	      "goal": "Search unread mail and return a structured list",
//	      "requires_auth": true,       // reported as NOT_RUN without an authenticated session
//	      "requires_network": true,    // reported as NOT_RUN unless --allow-network
//	      "usage": {"model": "x", "tokens_in": 1200, "tokens_out": 340},
//	      "deadline_seconds": 30,      // scenario wall-clock budget
//	      "attempts": 3,               // cap on a step's total attempts
//	      "context": "work",           // injected as --context work into every step
//	      "rubric": [                  // optional weighted grading (needs --judge-cmd)
//	        {"id": "accuracy", "weight": 2, "criterion": "the list is correct"}
//	      ],
//	      "steps": [
//	        {
//	          "argv": ["open", "https://mail.google.com/"],
//	          "expect": {"exit": 0},
//	          "retry": 2,               // prefix "retry --times 2 --" to argv
//	          "deadline_seconds": 10,   // overrides the scenario deadline for this step
//	          "optional": false,        // a failing optional step downgrades PASS to PARTIAL
//	          "capture": {              // pull values out of this step's JSON stdout
//	            "revenue": "items.0.value"   // later steps may use ${revenue}
//	          }
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
	// VerdictTimeout means a required step exceeded its deadline. It is a
	// failure, but distinct from FAIL so a hanging step is visible as such.
	VerdictTimeout = "TIMEOUT"
	// VerdictExcluded means --exclusions skipped the scenario. Its steps never
	// run, it is counted separately from NOT_RUN, and it is never a PASS.
	VerdictExcluded = "EXCLUDED"
)

// BenchUsage is DRIVER-SUPPLIED token accounting for one scenario.
//
// htrcli never spends tokens; the calling agent loop does. The harness only
// echoes what the driver declares, so a report can be costed next to its
// verdicts. A scenario that declares no usage omits the field entirely: a
// reported zero would read as "free", which is a different claim from
// "not measured".
type BenchUsage struct {
	// Model is the model the driver used for this scenario.
	Model string `json:"model"`
	// TokensIn is the number of input tokens the driver spent.
	TokensIn int `json:"tokens_in"`
	// TokensOut is the number of output tokens the driver spent.
	TokensOut int `json:"tokens_out"`
}

// BenchRubric is one weighted grading criterion for an optional judge.
//
// A rubric is only meaningful with --judge-cmd: a scenario that declares one
// without a judge is rejected at load time, because a criterion nobody
// evaluates is a lie.
type BenchRubric struct {
	// ID names the criterion. It must be non-empty and unique within the
	// scenario; the judge echoes it back in its scores.
	ID string `json:"id"`
	// Weight is the criterion's share of the weighted total. It must be > 0.
	Weight float64 `json:"weight"`
	// Criterion tells the judge what to evaluate.
	Criterion string `json:"criterion"`
}

// BenchStep is one htrcli invocation plus its expectations.
type BenchStep struct {
	// Argv is the htrcli argument list, WITHOUT the leading "htrcli".
	Argv []string `json:"argv"`
	// Expect holds the assertions for this step.
	Expect *BenchExpect `json:"expect,omitempty"`
	// Retry is the number of retries for this step (0 = no retry).
	Retry int `json:"retry,omitempty"`
	// DeadlineSeconds bounds this step's wall-clock time. Zero inherits the
	// scenario's deadline_seconds.
	DeadlineSeconds int `json:"deadline_seconds,omitempty"`
	// Optional marks a step whose failure downgrades the verdict to PARTIAL
	// instead of failing the scenario.
	Optional bool `json:"optional,omitempty"`
	// Note is a human-readable label for the report.
	Note string `json:"note,omitempty"`
	// Capture pulls values out of this step's stdout into run-scoped
	// variables, keyed by variable name and valued by a dotted JSON path.
	// Later steps in the SAME scenario may then use ${name} in their argv.
	//
	// A capture path that does not resolve FAILS the step: an extraction the
	// scenario depends on must never degrade into an empty string.
	Capture map[string]string `json:"capture,omitempty"`

	// deadline is the evaluated per-step deadline (0 = none). The scenario
	// runner computes it from deadline_seconds and the remaining scenario
	// budget before handing the step to a runner; it is not a file field.
	deadline time.Duration
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
	ID   string `json:"id"`
	Ref  string `json:"ref,omitempty"`
	App  string `json:"app,omitempty"`
	Goal string `json:"goal,omitempty"`
	// RequiresAuth marks a scenario that needs an authenticated session. It is
	// reported as NOT_RUN without one.
	RequiresAuth bool `json:"requires_auth,omitempty"`
	// RequiresNetwork marks a scenario that needs the network. Without the
	// --allow-network flag it is reported as NOT_RUN: the offline contract is
	// the default, so a suite cannot silently depend on connectivity.
	RequiresNetwork bool `json:"requires_network,omitempty"`
	// Usage is the driver-supplied token accounting, echoed into the report and
	// totalled in the summary. Nil means "not measured", not "free".
	Usage *BenchUsage `json:"usage,omitempty"`
	// DeadlineSeconds bounds the scenario's total wall-clock time. A step that
	// exceeds its own deadline, or that starts after this budget is exhausted,
	// is killed and reported as TIMEOUT.
	DeadlineSeconds int `json:"deadline_seconds,omitempty"`
	// Attempts caps a step's total attempts (>= 1). When declared, a step's
	// retry budget must not exceed attempts-1; the file is rejected otherwise.
	Attempts *int `json:"attempts,omitempty"`
	// Context names a browser context injected into every step as
	// --context <name>. A named context is an isolated persistent profile, so
	// this is what makes session reuse measurable inside a bench run.
	Context string `json:"context,omitempty"`
	// Rubric is the optional weighted grading rubric. Declaring one requires
	// --judge-cmd; a rubric nobody evaluates is rejected at load time.
	Rubric []BenchRubric `json:"rubric,omitempty"`
	Steps  []BenchStep   `json:"steps"`
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
	if err := validateBenchScenarioID(s.ID); err != nil {
		return err
	}
	if len(s.Steps) == 0 {
		return fmt.Errorf("has no steps")
	}
	// An absent attempts field means "no declared cap", so a step's existing
	// retry budget stays untouched. A declared cap must be at least one
	// attempt, or the scenario could never run.
	if s.Attempts != nil && *s.Attempts < 1 {
		return fmt.Errorf("attempts must be at least 1, got %d", *s.Attempts)
	}
	if s.DeadlineSeconds < 0 {
		return fmt.Errorf("deadline_seconds must not be negative, got %d", s.DeadlineSeconds)
	}
	if err := validateBenchContextName(s.Context); err != nil {
		return err
	}
	if err := validateBenchUsage(s.Usage); err != nil {
		return err
	}
	if err := validateBenchRubric(s.Rubric); err != nil {
		return err
	}
	for i, step := range s.Steps {
		if len(step.Argv) == 0 {
			return fmt.Errorf("step %d has no argv", i+1)
		}
		if step.Retry < 0 {
			return fmt.Errorf("step %d has a negative retry count", i+1)
		}
		if step.DeadlineSeconds < 0 {
			return fmt.Errorf("step %d has a negative deadline_seconds (%d)", i+1, step.DeadlineSeconds)
		}
		// attempts is the policy half of the retry wrapper: a step may not
		// declare more retries than the scenario allows.
		if s.Attempts != nil && step.Retry > *s.Attempts-1 {
			return fmt.Errorf(
				"step %d has retry %d, which exceeds the scenario's attempts cap of %d (retries must be <= attempts-1)",
				i+1, step.Retry, *s.Attempts,
			)
		}
		// The scenario context is injected into every step. A step that sets its
		// own context contradicts it, and silently letting one win would hide the
		// contradiction.
		if s.Context != "" && benchHasContextFlag(step.Argv) {
			return fmt.Errorf(
				"step %d sets its own --context, contradicting the scenario context %q",
				i+1, s.Context,
			)
		}
		for name, path := range step.Capture {
			if !isBenchVarName(name) {
				return fmt.Errorf(
					"step %d has capture %q: a capture name must match [A-Za-z_][A-Za-z0-9_]*",
					i+1, name,
				)
			}
			if path == "" {
				return fmt.Errorf("step %d has capture %q with an empty json path", i+1, name)
			}
		}
	}
	return nil
}

// validateBenchContextName rejects a context name the root --context flag
// could not take.
//
// A leading dash would be parsed as a flag, and a path separator would let the
// name escape the ~/.htrcli/contexts/<name> profile directory. Contexts are
// created lazily, so an unregistered name is valid; a malformed one is not.
func validateBenchContextName(name string) error {
	if name == "" {
		return nil
	}
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("context %q must not start with '-'", name)
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("context %q must not contain a path separator", name)
	}
	return nil
}

// validateBenchScenarioID rejects an id that cannot safely name the evidence
// subdirectory <evidence-dir>/<id>/.
//
// The id is used as a single directory component, so a separator or ".." would
// let a scenario write outside --evidence-dir. A leading dash is rejected for
// the same reason context names reject one: callers pass these strings onward
// as flag values. ".." is checked explicitly because filepath.Join cleans it
// before any separator check could see the traversal.
func validateBenchScenarioID(id string) error {
	if strings.HasPrefix(id, "-") {
		return fmt.Errorf("id %q must not start with '-'", id)
	}
	if strings.ContainsAny(id, `/\`) {
		return fmt.Errorf("id %q must not contain a path separator", id)
	}
	if strings.Contains(id, "..") {
		return fmt.Errorf("id %q must not contain '..'", id)
	}
	return nil
}

// validateBenchUsage rejects token accounting that could not be true.
func validateBenchUsage(u *BenchUsage) error {
	if u == nil {
		return nil
	}
	if u.Model == "" {
		return fmt.Errorf("usage.model must not be empty")
	}
	if u.TokensIn < 0 {
		return fmt.Errorf("usage.tokens_in must not be negative, got %d", u.TokensIn)
	}
	if u.TokensOut < 0 {
		return fmt.Errorf("usage.tokens_out must not be negative, got %d", u.TokensOut)
	}
	return nil
}

// validateBenchRubric rejects a rubric that could not be graded.
//
// A duplicate id would make the judge's scores ambiguous, a non-positive
// weight would let one criterion dominate silently, and an empty criterion
// gives the judge nothing to evaluate.
func validateBenchRubric(rubric []BenchRubric) error {
	seen := make(map[string]struct{}, len(rubric))
	for i, item := range rubric {
		if item.ID == "" {
			return fmt.Errorf("rubric entry %d has an empty id", i+1)
		}
		if _, dup := seen[item.ID]; dup {
			return fmt.Errorf("rubric id %q is duplicated", item.ID)
		}
		seen[item.ID] = struct{}{}
		if item.Weight <= 0 {
			return fmt.Errorf("rubric id %q has weight %g, must be > 0", item.ID, item.Weight)
		}
		if item.Criterion == "" {
			return fmt.Errorf("rubric id %q has an empty criterion", item.ID)
		}
	}
	return nil
}

// isBenchVarName reports whether name can be written as ${name} in an argv.
func isBenchVarName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		switch {
		case r == '_':
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
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
