package commands

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"strings"
	"time"
)

// benchJudge evaluates a scenario's evidence against its rubric and returns a
// report. It is a function so the verdict rules can be tested with a fake and
// the contract with a stub command.
type benchJudge func(s BenchScenario, steps []BenchStepResult) (BenchJudgeReport, error)

// BenchJudgeScore is one rubric item's verdict from the judge.
type BenchJudgeScore struct {
	// ID is the rubric id this score answers.
	ID string `json:"id"`
	// Score is the criterion's credit, in 0..1.
	Score float64 `json:"score"`
	// Reason is the judge's human-readable justification.
	Reason string `json:"reason,omitempty"`
}

// BenchJudgeReport records the judge invocation and its result.
//
// A surprising score must be auditable: the report keeps the argv that ran and
// the judge's raw stdout alongside the parsed scores.
type BenchJudgeReport struct {
	// Cmd is the judge argv, exactly as the runner spawned it.
	Cmd []string `json:"cmd"`
	// PassScore is the weighted threshold (0..100) a PASS required.
	PassScore float64 `json:"pass_score"`
	// Score is the weighted total in 0..100. It is always present when a judge
	// ran: a legitimate zero score must not vanish into the same omission as a
	// judge that never produced a total.
	Score float64 `json:"score"`
	// Scores are the judge's per-criterion verdicts. Absent on a judge error
	// that happened before the scores were parsed.
	Scores []BenchJudgeScore `json:"scores,omitempty"`
	// RawStdout is the judge's stdout, kept even when parsing failed. It is
	// always present when a judge ran, so an empty judge is distinguishable
	// from no judge at all.
	RawStdout string `json:"raw_stdout"`
	// Error is the judge error, when one occurred. A judge error FAILS the
	// scenario; it is never recorded as a zero score.
	Error string `json:"error,omitempty"`
}

// benchJudgeScenario is the scenario half of the judge's stdin document.
type benchJudgeScenario struct {
	ID   string `json:"id"`
	Goal string `json:"goal"`
	App  string `json:"app"`
}

// benchJudgeEvidence is the evidence half of the judge's stdin document: the
// run's actual step results.
type benchJudgeEvidence struct {
	Steps   []BenchStepResult         `json:"steps"`
	Summary benchJudgeEvidenceSummary `json:"summary"`
}

// benchJudgeEvidenceSummary is the aggregate a judge can read without summing
// the steps itself.
type benchJudgeEvidenceSummary struct {
	Commands int   `json:"commands"`
	Retries  int   `json:"retries"`
	WallMS   int64 `json:"wall_ms"`
	Steps    int   `json:"steps"`
	Passed   int   `json:"passed"`
	Failed   int   `json:"failed"`
}

// benchJudgeInput is the exact JSON document sent to the judge on stdin.
type benchJudgeInput struct {
	Scenario benchJudgeScenario `json:"scenario"`
	Rubric   []BenchRubric      `json:"rubric"`
	Evidence benchJudgeEvidence `json:"evidence"`
}

// benchJudgeOutput is the exact JSON document the judge must print.
type benchJudgeOutput struct {
	Scores []BenchJudgeScore `json:"scores"`
}

// buildBenchJudgeInput renders the stdin document for one scenario.
func buildBenchJudgeInput(s BenchScenario, steps []BenchStepResult) ([]byte, error) {
	evidence := benchJudgeEvidence{
		Steps:   steps,
		Summary: benchJudgeEvidenceSummary{Steps: len(steps)},
	}
	for _, step := range steps {
		evidence.Summary.Commands += step.Attempts
		if step.Attempts > 0 {
			evidence.Summary.Retries += step.Attempts - 1
		}
		evidence.Summary.WallMS += step.WallMS
		if step.OK {
			evidence.Summary.Passed++
		} else {
			evidence.Summary.Failed++
		}
	}
	doc := benchJudgeInput{
		Scenario: benchJudgeScenario{ID: s.ID, Goal: s.Goal, App: s.App},
		Rubric:   s.Rubric,
		Evidence: evidence,
	}
	return json.MarshalIndent(doc, "", "  ")
}

// validateBenchJudgeOptions rejects a judge configuration that could never
// grade anything. A non-positive timeout would silently run forever, so it is
// a hard error rather than a fallback. A pass score of zero would make the
// judge decorative, and NaN compares false against every bound, so both are
// rejected explicitly instead of slipping through as "in range".
func validateBenchJudgeOptions(passScore float64, timeoutSeconds int) error {
	if math.IsNaN(passScore) || passScore <= 0 || passScore > 100 {
		return fmt.Errorf("--judge-pass-score must be greater than 0 and at most 100, got %g", passScore)
	}
	if timeoutSeconds <= 0 {
		return fmt.Errorf("--judge-timeout must be positive, got %d", timeoutSeconds)
	}
	return nil
}

// validateBenchJudgeAssignment rejects a rubric nobody evaluates.
//
// A rubric without --judge-cmd is a lie: the report would carry criteria that
// no run ever graded. Checked before any step executes.
func validateBenchJudgeAssignment(scenarios []BenchScenario, judgeCmd []string) error {
	if len(judgeCmd) > 0 {
		return nil
	}
	for _, s := range scenarios {
		if len(s.Rubric) > 0 {
			return fmt.Errorf("scenario %s declares a rubric but no --judge-cmd was given", s.ID)
		}
	}
	return nil
}

// defaultBenchJudgeTimeout is the deadline the --judge-timeout flag defaults
// to. runBenchJudge takes the deadline explicitly, so the flag is the only
// place the default is applied.
const defaultBenchJudgeTimeout = 120 * time.Second

// runBenchJudge spawns the judge, sends it the evidence document, and validates
// its scores against the rubric.
//
// The judge is an external program, so it cannot go through selfCommand; it
// still runs through the shared runCmdWithInputAndDeadline lifecycle (start,
// capture, wait, kill) rather than a private exec path.
//
// timeout is required: the only caller always knows its deadline, and a silent
// default would hide a caller that forgot to pass one. An explicit non-positive
// timeout is a judge error, never a silent run-to-completion.
func runBenchJudge(cmd []string, s BenchScenario, steps []BenchStepResult, passScore float64, timeout time.Duration) (BenchJudgeReport, error) {
	report := BenchJudgeReport{Cmd: cmd, PassScore: passScore}
	if len(cmd) == 0 {
		err := fmt.Errorf("judge command is empty")
		report.Error = err.Error()
		return report, err
	}
	if timeout <= 0 {
		err := fmt.Errorf("judge timeout must be positive, got %s", benchJudgeTimeoutLabel(timeout))
		report.Error = err.Error()
		return report, err
	}
	input, err := buildBenchJudgeInput(s, steps)
	if err != nil {
		report.Error = err.Error()
		return report, err
	}
	run, timedOut := runCmdWithInputAndDeadline(exec.Command(cmd[0], cmd[1:]...), timeout, bytes.NewReader(input))
	report.RawStdout = run.Stdout
	if timedOut {
		err := fmt.Errorf("judge timed out after %s", benchJudgeTimeoutLabel(timeout))
		report.Error = err.Error()
		return report, err
	}
	if run.Err != nil {
		// A start failure has no exit status; a normal non-zero exit is an
		// ExitError and is reported below with the code the judge chose.
		var exitErr *exec.ExitError
		if !errors.As(run.Err, &exitErr) {
			err := fmt.Errorf("start judge: %w", run.Err)
			report.Error = err.Error()
			return report, err
		}
	}
	if run.ExitCode != 0 {
		err := fmt.Errorf("judge exited %d: %s", run.ExitCode, benchJudgeDiagnostic(run.Stderr))
		report.Error = err.Error()
		return report, err
	}
	scores, err := parseBenchJudgeScores(run.Stdout, s.Rubric)
	if err != nil {
		report.Error = err.Error()
		return report, err
	}
	report.Scores = scores
	total, err := weightedBenchScore(s.Rubric, scores)
	if err != nil {
		report.Error = err.Error()
		return report, err
	}
	report.Score = roundBenchScore(total * 100)
	return report, nil
}

// benchJudgeTimeoutLabel renders a judge deadline with the flag's seconds unit,
// so 120*time.Second reads "120s" rather than Go's "2m0s".
func benchJudgeTimeoutLabel(d time.Duration) string {
	return fmt.Sprintf("%gs", d.Seconds())
}

// benchJudgeDiagnostic picks the stderr line the report should carry.
//
// A transport such as curl prints its own error first, while the adapter prints
// the HTTP status and response body on a "judge:" line. Prefer the adapter's
// line so a 401 body reaches the report instead of curl's generic message; fall
// back to the first non-empty line for a judge that says nothing else.
func benchJudgeDiagnostic(stderr string) string {
	for _, line := range strings.Split(stderr, "\n") {
		if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "judge:") {
			return trimmed
		}
	}
	return firstLine(stderr)
}

// parseBenchJudgeScores decodes the judge's stdout and enforces the contract:
// every rubric id appears exactly once, no unknown id appears, and every score
// is inside 0..1.
func parseBenchJudgeScores(stdout string, rubric []BenchRubric) ([]BenchJudgeScore, error) {
	var doc benchJudgeOutput
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		return nil, fmt.Errorf("judge stdout is not valid JSON: %w", err)
	}
	want := make(map[string]struct{}, len(rubric))
	for _, item := range rubric {
		want[item.ID] = struct{}{}
	}
	seen := make(map[string]struct{}, len(doc.Scores))
	for _, score := range doc.Scores {
		if _, ok := want[score.ID]; !ok {
			return nil, fmt.Errorf("judge returned unknown rubric id %q", score.ID)
		}
		if _, dup := seen[score.ID]; dup {
			return nil, fmt.Errorf("judge returned duplicate rubric id %q", score.ID)
		}
		seen[score.ID] = struct{}{}
		if math.IsNaN(score.Score) || score.Score < 0 || score.Score > 1 {
			return nil, fmt.Errorf("judge score for %q is %g, must be between 0 and 1", score.ID, score.Score)
		}
	}
	for _, item := range rubric {
		if _, ok := seen[item.ID]; !ok {
			return nil, fmt.Errorf("judge did not score rubric id %q", item.ID)
		}
	}
	return doc.Scores, nil
}

// weightedBenchScore computes sum(weight_i * score_i) / sum(weight_i) in 0..1.
func weightedBenchScore(rubric []BenchRubric, scores []BenchJudgeScore) (float64, error) {
	byID := make(map[string]float64, len(scores))
	for _, score := range scores {
		byID[score.ID] = score.Score
	}
	var total, weights float64
	for _, item := range rubric {
		total += item.Weight * byID[item.ID]
		weights += item.Weight
	}
	if weights <= 0 {
		return 0, fmt.Errorf("rubric weights total %g, must be positive", weights)
	}
	return total / weights, nil
}

// roundBenchScore rounds a 0..100 score to two decimals for a readable report
// and a deterministic threshold comparison.
func roundBenchScore(v float64) float64 {
	return math.Round(v*100) / 100
}

// applyBenchJudge grades a completed scenario and sets the judged verdict.
//
// Only a scenario whose steps ran to completion (PASS or PARTIAL) is worth
// grading: a required failure, a timeout or a skip already decides the verdict,
// and a judge call would only mask that. A judge error FAILS the scenario with
// a message naming the error; a weighted score below the threshold FAILS it
// with the score. Reaching the threshold is a PASS.
func applyBenchJudge(judge benchJudge, s BenchScenario, res BenchScenarioResult) BenchScenarioResult {
	if res.Verdict != VerdictPass && res.Verdict != VerdictPartial {
		return res
	}
	report, err := judge(s, res.Steps)
	res.Judge = &report
	if err != nil {
		res.Verdict = VerdictFail
		res.FailureMode = "judge error: " + err.Error()
		return res
	}
	if report.Score >= report.PassScore {
		res.Verdict = VerdictPass
		res.FailureMode = ""
		return res
	}
	res.Verdict = VerdictFail
	res.FailureMode = fmt.Sprintf("judge score %.2f is below the pass score %.2f", report.Score, report.PassScore)
	return res
}
