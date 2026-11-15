package commands

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// BenchStepResult is the outcome of one step.
type BenchStepResult struct {
	// Index is the 1-based step number.
	Index int `json:"index"`
	// Argv is the argv that was executed (with the retry prefix applied).
	Argv []string `json:"argv"`
	// Note is the scenario's label for the step.
	Note string `json:"note,omitempty"`
	// OK reports whether the step met its expectations.
	OK bool `json:"ok"`
	// Optional marks a step whose failure only downgrades the verdict.
	Optional bool `json:"optional,omitempty"`
	// ExitCode is the child's exit code.
	ExitCode int `json:"exit_code"`
	// Attempts is how many times the step ran (1 when no retry).
	Attempts int `json:"attempts"`
	// WallMS is the step's total wall-clock time in milliseconds.
	WallMS int64 `json:"wall_ms"`
	// TimedOut reports that the step exceeded its deadline and was killed.
	// A timed-out step always fails, even when its exit code looks normal.
	TimedOut bool `json:"timed_out,omitempty"`
	// Failure describes why the step failed.
	Failure string `json:"failure,omitempty"`
	// Captured holds the values this step pulled out for later steps.
	Captured map[string]string `json:"captured,omitempty"`
	// Stdout is the child's stdout (truncated to keep the report readable).
	Stdout string `json:"stdout,omitempty"`
	// Stderr is the child's stderr (truncated).
	Stderr string `json:"stderr,omitempty"`
	// FullStdout is the child's untruncated stdout. It is kept OUT of the
	// report (json:"-") and used only by --evidence-dir; the report carries
	// the truncated Stdout so one chatty step cannot bloat it.
	FullStdout string `json:"-"`
	// FullStderr is the child's untruncated stderr, used only by
	// --evidence-dir.
	FullStderr string `json:"-"`
}

// BenchScenarioResult is the outcome of one scenario.
type BenchScenarioResult struct {
	ID      string `json:"id"`
	Ref     string `json:"ref,omitempty"`
	App     string `json:"app,omitempty"`
	Goal    string `json:"goal,omitempty"`
	Verdict string `json:"verdict"`
	// Commands is the number of htrcli invocations the scenario made.
	Commands int `json:"commands"`
	// WallMS is the scenario's total wall-clock time in milliseconds.
	WallMS int64 `json:"wall_ms"`
	// Retries is the number of retried attempts across all steps.
	Retries int `json:"retries"`
	// Usage is the scenario's driver-supplied token accounting, echoed from
	// the scenario file. Absent when the driver declared none.
	Usage *BenchUsage `json:"usage,omitempty"`
	// FailureMode is a short description of the first failure.
	FailureMode string            `json:"failure_mode,omitempty"`
	Steps       []BenchStepResult `json:"steps"`
	// Judge records the rubric evaluation when the scenario was judged. Absent
	// when no judge ran.
	Judge *BenchJudgeReport `json:"judge,omitempty"`
}

// BenchReport is the whole run.
type BenchReport struct {
	// StartedAt is the RFC3339 timestamp of the run's start.
	StartedAt string `json:"started_at"`
	// Transport is the transport the steps used ("ext" or "cdp").
	Transport string `json:"transport"`
	// Scenarios are the per-scenario results, in scenario-file order.
	Scenarios []BenchScenarioResult `json:"scenarios"`
	// Summary counts verdicts.
	Summary BenchSummary `json:"summary"`
}

// BenchSummary counts the verdicts of a run.
type BenchSummary struct {
	Total   int `json:"total"`
	Pass    int `json:"pass"`
	Partial int `json:"partial"`
	Fail    int `json:"fail"`
	NotRun  int `json:"not_run"`
	// Timeout counts scenarios whose required step exceeded its deadline.
	Timeout int `json:"timeout"`
	// Excluded counts scenarios skipped by --exclusions. They are separate
	// from NOT_RUN and are never counted as a pass.
	Excluded int `json:"excluded"`
	// Usage totals the driver-supplied token accounting. Absent when no
	// scenario declared usage: a zero total would read as "free".
	Usage *BenchUsageTotals `json:"usage,omitempty"`
	// WallMS is the whole run's wall-clock time.
	WallMS int64 `json:"wall_ms"`
}

// BenchUsageTotals sums driver-supplied token accounting across a run.
type BenchUsageTotals struct {
	// TokensIn is the run's total input tokens.
	TokensIn int `json:"tokens_in"`
	// TokensOut is the run's total output tokens.
	TokensOut int `json:"tokens_out"`
}

// benchRunner executes one scenario. It is a function so the tests can
// substitute a fake without a browser.
type benchRunner func(step BenchStep) BenchStepResult

// runBenchScenario executes a scenario's steps and scores it.
//
// A scenario marked requires_auth is reported as NOT_RUN without executing
// anything: an unauthenticated run must never look like a pass. Likewise a
// requires_network scenario is NOT_RUN unless --allow-network opted in — the
// offline contract is the default.
func runBenchScenario(s BenchScenario, run benchRunner) BenchScenarioResult {
	res := BenchScenarioResult{
		ID:    s.ID,
		Ref:   s.Ref,
		App:   s.App,
		Goal:  s.Goal,
		Steps: make([]BenchStepResult, 0, len(s.Steps)),
	}
	if s.RequiresAuth {
		res.Verdict = VerdictNotRun
		res.FailureMode = "requires an authenticated session"
		return res
	}
	if s.RequiresNetwork && !benchAllowNetwork {
		res.Verdict = VerdictNotRun
		res.FailureMode = "requires network access (re-run with --allow-network)"
		return res
	}
	// A skipped scenario spent no tokens: the driver's accounting belongs to
	// the attempt, so attributing it to a NOT_RUN result would bill a run that
	// never happened. This mirrors excludedBenchResult, which carries no usage.
	res.Usage = s.Usage

	start := time.Now()
	scenarioDeadline := time.Duration(s.DeadlineSeconds) * time.Second
	anyOptionalFailed := false
	// caps lives for ONE scenario: a capture is a value read from the page in
	// this run, so it must not leak into another scenario.
	caps := make(map[string]string)
	for i, step := range s.Steps {
		prepared := step
		prepared.Argv = injectBenchContext(substituteCaptures(step.Argv, caps), s.Context)
		prepared.Expect = substituteExpectCaptures(step.Expect, caps)
		deadline, exhausted := benchEffectiveDeadline(
			scenarioDeadline,
			time.Duration(step.DeadlineSeconds)*time.Second,
			time.Since(start),
		)
		var sr BenchStepResult
		if exhausted {
			sr = BenchStepResult{
				Argv:     benchArgv(prepared),
				TimedOut: true,
				Failure:  fmt.Sprintf("scenario deadline of %ds was exhausted before this step", s.DeadlineSeconds),
			}
		} else {
			prepared.deadline = deadline
			sr = run(prepared)
		}
		sr.Index = i + 1
		sr.Note = step.Note
		sr.Optional = step.Optional
		// A step that never started (exhausted budget) made zero invocations;
		// claiming one would inflate Commands.
		if sr.Attempts < 1 && !sr.TimedOut {
			sr.Attempts = 1
		}
		// Capture only from a step whose expectations held: pulling a value
		// out of a step that failed would seed later steps with garbage.
		if sr.OK && len(step.Capture) > 0 {
			captured, err := applyCaptureSpec(step.Capture, sr.Stdout)
			if err != nil {
				sr.OK = false
				sr.Failure = err.Error()
			} else {
				sr.Captured = captured
				for name, value := range captured {
					caps[name] = value
				}
			}
		}
		// Do NOT overwrite sr.Attempts with the retry BUDGET: the runner
		// reports the attempts it actually observed, and a step that passed on
		// the first try made exactly one invocation.
		res.Steps = append(res.Steps, sr)
		res.Commands += sr.Attempts
		// A step that never started made no invocation and therefore no retry.
		if sr.Attempts > 0 {
			res.Retries += sr.Attempts - 1
		}
		res.WallMS += sr.WallMS
		if !sr.OK {
			switch {
			case step.Optional:
				anyOptionalFailed = true
			case sr.TimedOut:
				// The FIRST required failure decides the verdict, so a timeout
				// stays visible as TIMEOUT instead of being overwritten by a
				// later step's ordinary failure.
				if res.Verdict == "" {
					res.Verdict = VerdictTimeout
				}
				if res.FailureMode == "" {
					res.FailureMode = fmt.Sprintf("step %d timed out: %s", i+1, sr.Failure)
				}
			default:
				if res.Verdict == "" {
					res.Verdict = VerdictFail
				}
				if res.FailureMode == "" {
					res.FailureMode = fmt.Sprintf("step %d failed: %s", i+1, sr.Failure)
				}
			}
		}
	}
	if res.WallMS == 0 {
		res.WallMS = time.Since(start).Milliseconds()
	}
	if res.Verdict == "" {
		if anyOptionalFailed {
			res.Verdict = VerdictPartial
			res.FailureMode = "an optional step failed"
		} else {
			res.Verdict = VerdictPass
		}
	}
	return res
}

// substituteCaptures replaces ${name} in each argv element with a value
// captured earlier in the same scenario.
//
// Names with no captured value are left as the literal ${name}, so the
// existing environment expansion still runs and an unresolved reference stays
// visible in the failing argv instead of silently becoming an empty string.
// A captured value is never re-scanned, so a value that itself contains
// "${...}" is passed through verbatim.
func substituteCaptures(argv []string, caps map[string]string) []string {
	if len(caps) == 0 {
		return argv
	}
	out := make([]string, len(argv))
	for i, arg := range argv {
		out[i] = expandBenchRef(arg, func(name string) (string, bool) {
			v, ok := caps[name]
			return v, ok
		})
	}
	return out
}

// substituteExpectCaptures replaces ${name} with a captured value inside the
// expectation strings, mirroring substituteCaptures for argv.
//
// Without this a scenario could use a captured value in a selector but not in
// the assertion that reads the result back — the natural cross-check
// ("the detail view opened on the id I just captured") would be unwritable.
// Environment expansion still runs later, so an unresolved name keeps its
// literal form and fails the expectation loudly.
func substituteExpectCaptures(expect *BenchExpect, caps map[string]string) *BenchExpect {
	if expect == nil || len(caps) == 0 {
		return expect
	}
	out := *expect
	out.StdoutContains = substituteCaptures([]string{expect.StdoutContains}, caps)[0]
	out.StdoutNotContains = substituteCaptures([]string{expect.StdoutNotContains}, caps)[0]
	if expect.JSONPath != nil {
		path := *expect.JSONPath
		path.Equals = substituteCaptures([]string{expect.JSONPath.Equals}, caps)[0]
		out.JSONPath = &path
	}
	return &out
}

// applyCaptureSpec pulls values out of a step's stdout.
//
// Every path must resolve: the scenario declared the value as an input to a
// later step, so a missing path is a broken scenario rather than a soft miss.
func applyCaptureSpec(spec map[string]string, stdout string) (map[string]string, error) {
	var doc any
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		return nil, fmt.Errorf("capture: stdout is not JSON: %w", err)
	}
	captured := make(map[string]string, len(spec))
	for name, path := range spec {
		val, ok := lookupJSONPath(doc, path)
		if !ok {
			return nil, fmt.Errorf("capture %q: json path %q not found", name, path)
		}
		captured[name] = normalizeJSONValue(val)
	}
	return captured, nil
}

// benchArgv builds the argv for a step, applying the retry prefix.
//
// Retry reuses the `retry` subcommand (it owns the backoff policy); the
// harness never implements a second loop. `retry --times N` means N retries
// AFTER the first attempt, so the total attempts are N+1.
func benchArgv(step BenchStep) []string {
	argv := expandBenchVars(step.Argv)
	if step.Retry <= 0 {
		return argv
	}
	prefixed := []string{"retry", "--times", fmt.Sprintf("%d", step.Retry), "--"}
	return append(prefixed, argv...)
}

// expandBenchVars replaces ${VAR} in each argument with the environment
// variable of that name.
//
// A scenario can therefore stay machine-independent: the fixture path, the
// target URL or a test account can come from the environment. An unset
// variable is left as-is (the literal ${VAR}), so a missing value shows up as
// an obvious step failure rather than a silent empty string.
func expandBenchVars(argv []string) []string {
	out := make([]string, len(argv))
	for i, a := range argv {
		out[i] = expandBenchRef(a, os.LookupEnv)
	}
	return out
}

// benchRefPattern matches a ${name} or $name reference whose name is a
// shell-style identifier: a letter or underscore, then letters, digits or
// underscores.
//
// The name rule is load-bearing, not cosmetic. os.Expand reads ANY run of
// alphanumerics after a "$" as a variable, so a currency or template string
// such as "$4,760.50" became "${4},760.50" and "$520.50" became "${5}20.50" as
// soon as a capture map was non-empty — the expected value silently changed and
// the step failed on data that was never wrong. Money, prices and percent
// strings are ordinary benchmark data, so a "$" that cannot start an identifier
// must stay literal.
var benchRefPattern = regexp.MustCompile(`\$\{[A-Za-z_][A-Za-z0-9_]*\}|\$[A-Za-z_][A-Za-z0-9_]*`)

// expandBenchRef replaces ${name} and $name using lookup, leaving every other
// "$" untouched. An unknown name keeps its ORIGINAL form (braced or bare) so a
// typo fails as an obvious unmatched value instead of being rewritten.
func expandBenchRef(input string, lookup func(string) (string, bool)) string {
	if !strings.ContainsRune(input, '$') {
		return input
	}
	return benchRefPattern.ReplaceAllStringFunc(input, func(match string) string {
		var name string
		if strings.HasPrefix(match, "${") {
			name = match[2 : len(match)-1]
		} else {
			name = match[1:]
		}
		if v, ok := lookup(name); ok {
			return v
		}
		return match
	})
}

// benchStepRunnerFor returns the runner that executes a step over the given
// transport.
//
// The transport flag is injected into each step's argv, so a scenario file
// does not have to repeat it and a run can be replayed over the other
// transport by flipping one flag. A step that already names a transport flag
// keeps its own value.
func benchStepRunnerFor(transport string) benchRunner {
	return func(step BenchStep) BenchStepResult {
		return runBenchStepWithTransport(step, transport)
	}
}

// benchHasTransportFlag reports whether an argv already selects a transport.
func benchHasTransportFlag(argv []string) bool {
	for _, a := range argv {
		switch a {
		case "--cdp", "--transport", "--transport=cdp", "--transport=ext":
			return true
		}
	}
	return false
}

// injectBenchTransport prefixes a step's argv with the transport flag when the
// step does not already select one.
func injectBenchTransport(argv []string, transport string) []string {
	if transport == "" || benchHasTransportFlag(argv) {
		return argv
	}
	out := make([]string, 0, len(argv)+1)
	out = append(out, "--transport="+transport)
	return append(out, argv...)
}

// benchHasContextFlag reports whether an argv already selects a context.
func benchHasContextFlag(argv []string) bool {
	for _, a := range argv {
		if a == "--context" || strings.HasPrefix(a, "--context=") {
			return true
		}
	}
	return false
}

// injectBenchContext prefixes a step's argv with "--context <name>".
//
// The scenario runner injects it before the step runs and the reported argv is
// the one actually executed, so a reader can see that the isolated profile was
// in force. Validation rejects a step that names its own --context alongside a
// scenario context, so this never has to pick a winner.
func injectBenchContext(argv []string, name string) []string {
	if name == "" {
		return argv
	}
	out := make([]string, 0, len(argv)+2)
	out = append(out, "--context", name)
	return append(out, argv...)
}

// runBenchStepWithTransport executes one step over the given transport and
// evaluates its expectations.
func runBenchStepWithTransport(step BenchStep, transport string) BenchStepResult {
	step.Argv = injectBenchTransport(step.Argv, transport)
	return runBenchStep(step)
}

// runBenchStep executes one step for real and evaluates its expectations.
func runBenchStep(step BenchStep) BenchStepResult {
	argv := benchArgv(step)
	run, timedOut := runSelfWithDeadline(argv, step.deadline)
	res := BenchStepResult{
		Argv:       argv,
		ExitCode:   run.ExitCode,
		Attempts:   1,
		WallMS:     run.Duration.Milliseconds(),
		Stdout:     truncateForReport(run.Stdout),
		Stderr:     truncateForReport(run.Stderr),
		FullStdout: run.Stdout,
		FullStderr: run.Stderr,
	}
	if step.Retry > 0 {
		// The retry wrapper performs the attempts and reports how many it
		// actually made on stderr. Prefer that observed count; fall back to
		// the budget only if the line is missing (e.g. the child failed to
		// start).
		res.Attempts = step.Retry + 1
		if observed, ok := parseRetryAttempts(run.Stderr); ok {
			res.Attempts = observed
		}
	}
	// A killed step always fails: its exit code is an artefact of the signal,
	// and its output is whatever it managed to emit before the deadline.
	if timedOut {
		res.TimedOut = true
		res.Failure = fmt.Sprintf("timed out after %s", step.deadline)
		return res
	}
	if run.Err != nil {
		// Only a start failure is an error here: a normal non-zero exit is an
		// *exec.ExitError and is judged by the step's expectations below, so a
		// step that deliberately fails must still be able to pass.
		var exitErr *exec.ExitError
		if !errors.As(run.Err, &exitErr) {
			res.Failure = fmt.Sprintf("could not start step: %v", run.Err)
			return res
		}
	}
	if err := evaluateBenchExpect(expandBenchExpect(step.Expect), run); err != nil {
		res.Failure = err.Error()
		return res
	}
	res.OK = true
	return res
}

// benchEffectiveDeadline bounds a step by its own deadline and by what remains
// of the scenario budget.
//
// A step without its own deadline inherits the scenario value. The scenario
// budget always caps the result, so a step cannot outlive the run. exhausted
// reports that the scenario budget is already gone, in which case the step must
// not start at all.
func benchEffectiveDeadline(scenario, step, elapsed time.Duration) (deadline time.Duration, exhausted bool) {
	d := step
	if d == 0 {
		d = scenario
	}
	if scenario > 0 {
		remaining := scenario - elapsed
		if remaining <= 0 {
			return 0, true
		}
		if d == 0 || remaining < d {
			d = remaining
		}
	}
	return d, false
}

// runSelfWithDeadline re-invokes htrcli and kills it when the deadline elapses.
//
// A zero deadline runs the child to completion, exactly like runSelf.
func runSelfWithDeadline(args []string, deadline time.Duration) (selfRun, bool) {
	// With no deadline the shared runner is authoritative, so there is only one
	// no-deadline path.
	if deadline <= 0 {
		return runSelf(args), false
	}
	cmd, err := selfCommand(args)
	if err != nil {
		return selfRun{ExitCode: 1, Err: err}, false
	}
	return runCmdWithDeadline(cmd, deadline)
}

// runCmdWithDeadline starts cmd, captures its output, and kills it when the
// deadline elapses. timedOut reports that the kill was actually attempted, so a
// child that finished just as the timer fired is not misreported as a timeout.
//
// shortcut: it kills only the direct child, so a step that re-execs further
// children can leak them; upgrade to a process group if a timed-out step is
// observed leaking grandchildren.
func runCmdWithDeadline(cmd *exec.Cmd, deadline time.Duration) (selfRun, bool) {
	return runCmdWithInputAndDeadline(cmd, deadline, nil)
}

// runCmdWithInputAndDeadline is the one child lifecycle the harness uses: it
// starts the command, captures stdout/stderr, and kills it when the deadline
// elapses. A nil stdin leaves the child with no input, as before.
func runCmdWithInputAndDeadline(cmd *exec.Cmd, deadline time.Duration, stdin io.Reader) (selfRun, bool) {
	start := time.Now()
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	if stdin != nil {
		cmd.Stdin = stdin
	}
	if err := cmd.Start(); err != nil {
		return selfRun{ExitCode: 1, Duration: time.Since(start), Err: err}, false
	}
	var killed atomic.Bool
	var timer *time.Timer
	if deadline > 0 {
		timer = time.AfterFunc(deadline, func() {
			killed.Store(true)
			_ = cmd.Process.Kill()
		})
	}
	err := cmd.Wait()
	if timer != nil {
		timer.Stop()
	}
	run := selfRun{
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
		Duration: time.Since(start),
	}
	if err != nil {
		run.Err = err
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			run.ExitCode = exitErr.ExitCode()
		} else {
			run.ExitCode = 1
		}
	}
	return run, killed.Load()
}

// expandBenchExpect expands ${VAR} inside the expectation strings.
//
// Argv has always expanded environment references; expectations did not, so a
// scenario could not assert on an environment-dependent value (a fixture's
// absolute file:// URL) without hard-coding the machine's path.
func expandBenchExpect(expect *BenchExpect) *BenchExpect {
	if expect == nil {
		return nil
	}
	out := *expect
	out.StdoutContains = expandBenchValue(expect.StdoutContains)
	out.StdoutNotContains = expandBenchValue(expect.StdoutNotContains)
	if expect.JSONPath != nil {
		path := *expect.JSONPath
		path.Equals = expandBenchValue(expect.JSONPath.Equals)
		out.JSONPath = &path
	}
	return &out
}

// expandBenchValue expands one string with the same unset-variable rule as
// benchArgv: an unset ${VAR} stays literal so the mismatch is visible.
func expandBenchValue(value string) string {
	if value == "" || !strings.Contains(value, "${") {
		return value
	}
	return expandBenchVars([]string{value})[0]
}

// evaluateBenchExpect checks a step's expectations against its output.
//
// It returns nil when every present expectation holds.
func evaluateBenchExpect(expect *BenchExpect, run selfRun) error {
	if expect == nil {
		if run.ExitCode != 0 {
			return fmt.Errorf("exit code %d (expected 0): %s", run.ExitCode, firstLine(run.Stderr))
		}
		return nil
	}
	if expect.Exit != run.ExitCode {
		return fmt.Errorf("exit code %d, want %d: %s", run.ExitCode, expect.Exit, firstLine(run.Stderr))
	}
	// Anything still shaped like ${name} after capture and environment
	// expansion names a capture that never resolved. Comparing it as a literal
	// makes a negative assertion pass vacuously ("${name}" is never in the
	// output either), which can hide a missing value inside a PASS.
	for _, value := range []string{expect.StdoutContains, expect.StdoutNotContains, jsonPathEquals(expect.JSONPath)} {
		if ref, ok := unresolvedBenchReference(value); ok {
			return fmt.Errorf("expectation has an unresolved capture %s", ref)
		}
	}
	if expect.StdoutContains != "" && !strings.Contains(run.Stdout, expect.StdoutContains) {
		return fmt.Errorf("stdout does not contain %q", expect.StdoutContains)
	}
	if expect.StdoutNotContains != "" && strings.Contains(run.Stdout, expect.StdoutNotContains) {
		return fmt.Errorf("stdout contains %q", expect.StdoutNotContains)
	}
	if expect.JSONPath != nil {
		if err := evaluateJSONPath(expect.JSONPath, run.Stdout); err != nil {
			return err
		}
	}
	return nil
}

// jsonPathEquals returns the expectation's literal comparison value, or "" when
// there is no JSON path. evaluateBenchExpect uses it to scan every string that
// substitution could have left unresolved.
func jsonPathEquals(expect *JSONPathExpect) string {
	if expect == nil {
		return ""
	}
	return expect.Equals
}

// unresolvedBenchReference returns the first ${...} placeholder left in value.
//
// It runs after capture and environment expansion, so a surviving placeholder
// belongs to a capture that never resolved; callers turn it into a hard step
// failure instead of comparing the literal text.
func unresolvedBenchReference(value string) (string, bool) {
	start := strings.Index(value, "${")
	if start < 0 {
		return "", false
	}
	end := strings.Index(value[start:], "}")
	if end < 0 {
		return "", false
	}
	return value[start : start+end+1], true
}

// evaluateJSONPath asserts on a value inside the step's JSON stdout.
func evaluateJSONPath(expect *JSONPathExpect, stdout string) error {
	var doc any
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		return fmt.Errorf("stdout is not JSON: %w", err)
	}
	val, ok := lookupJSONPath(doc, expect.Path)
	if !ok {
		return fmt.Errorf("json path %q not found", expect.Path)
	}
	if expect.Exists {
		return nil
	}
	got := normalizeJSONValue(val)
	if got != expect.Equals {
		return fmt.Errorf("json path %q = %q, want %q", expect.Path, got, expect.Equals)
	}
	return nil
}

// firstLine returns the first non-empty line of s (for error messages).
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			return t
		}
	}
	return ""
}

// benchReportMaxStream bounds how much of a step's output lands in a report.
const benchReportMaxStream = 4000

// truncateForReport bounds a captured stream so one chatty step cannot blow
// up the report file.
func truncateForReport(s string) string {
	if len(s) <= benchReportMaxStream {
		return s
	}
	return s[:benchReportMaxStream] + fmt.Sprintf("\n... (truncated, %d bytes total)", len(s))
}

// summarizeBenchReport counts the verdicts of a report.
func summarizeBenchReport(results []BenchScenarioResult, wallMS int64) BenchSummary {
	sum := BenchSummary{Total: len(results), WallMS: wallMS}
	tokensIn, tokensOut := 0, 0
	anyUsage := false
	for _, r := range results {
		switch r.Verdict {
		case VerdictPass:
			sum.Pass++
		case VerdictPartial:
			sum.Partial++
		case VerdictFail:
			sum.Fail++
		case VerdictNotRun:
			sum.NotRun++
		case VerdictTimeout:
			sum.Timeout++
		case VerdictExcluded:
			sum.Excluded++
		}
		if r.Usage != nil {
			anyUsage = true
			tokensIn += r.Usage.TokensIn
			tokensOut += r.Usage.TokensOut
		}
	}
	// Only report totals when at least one scenario declared usage. A run with
	// no accounting must not look like it cost nothing.
	if anyUsage {
		sum.Usage = &BenchUsageTotals{TokensIn: tokensIn, TokensOut: tokensOut}
	}
	return sum
}

// parseRetryAttempts reads the machine-readable attempt count the `retry`
// wrapper prints ("retry: attempts=N"). The LAST occurrence wins, so a
// multi-line stderr still resolves to the final count.
func parseRetryAttempts(stderr string) (int, bool) {
	const marker = "retry: attempts="
	idx := strings.LastIndex(stderr, marker)
	if idx < 0 {
		return 0, false
	}
	rest := stderr[idx+len(marker):]
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0, false
	}
	n, err := strconv.Atoi(rest[:end])
	if err != nil || n < 1 {
		return 0, false
	}
	return n, true
}
