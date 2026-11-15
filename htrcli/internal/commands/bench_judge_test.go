package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestBenchJudgeHelperProcess is the judge subprocess used by the spawn tests.
//
// It re-executes the test binary with -test.run pinned to this function, so the
// harness is exercised without a shell or an external program. The mode comes
// from the environment.
func TestBenchJudgeHelperProcess(t *testing.T) {
	mode := os.Getenv("HTRCLI_BENCH_JUDGE_HELPER")
	if mode == "" {
		t.Skip("judge helper process; only runs as a spawned child")
	}
	if capture := os.Getenv("HTRCLI_BENCH_JUDGE_CAPTURE"); capture != "" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(9)
		}
		if err := os.WriteFile(capture, data, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(9)
		}
	}
	switch mode {
	case "pass":
		fmt.Fprint(os.Stdout, `{"scores":[{"id":"accuracy","score":1,"reason":"right"},{"id":"brevity","score":0,"reason":"too long"}]}`)
	case "out-of-range":
		fmt.Fprint(os.Stdout, `{"scores":[{"id":"accuracy","score":2,"reason":"impossible"}]}`)
	case "unparsable":
		fmt.Fprint(os.Stdout, `this is not json`)
	case "sleep":
		d, err := time.ParseDuration(os.Getenv("HTRCLI_BENCH_JUDGE_SLEEP"))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(9)
		}
		time.Sleep(d)
		fmt.Fprint(os.Stdout, `{"scores":[{"id":"accuracy","score":1},{"id":"brevity","score":0}]}`)
	case "exit-nonzero":
		fmt.Fprintln(os.Stderr, "judge blew up")
		os.Exit(3)
	default:
		fmt.Fprintf(os.Stderr, "unknown helper mode %q\n", mode)
		os.Exit(8)
	}
	os.Exit(0)
}

// helperJudgeCmd builds a judge argv that re-invokes this test binary as the
// helper process.
func helperJudgeCmd() []string {
	return []string{os.Args[0], "-test.run=TestBenchJudgeHelperProcess"}
}

// judgeTestScenario is a fixed scenario with two weighted criteria.
func judgeTestScenario() BenchScenario {
	return BenchScenario{
		ID:   "B01",
		Goal: "find the totals",
		App:  "fixture",
		Rubric: []BenchRubric{
			{ID: "accuracy", Weight: 3, Criterion: "the numbers are right"},
			{ID: "brevity", Weight: 1, Criterion: "the answer is short"},
		},
	}
}

// judgeTestSteps is fixed evidence so a golden document stays stable.
func judgeTestSteps() []BenchStepResult {
	return []BenchStepResult{
		{Index: 1, Argv: []string{"open", "https://x"}, OK: true, ExitCode: 0, Attempts: 1, WallMS: 12, Stdout: "ok"},
		{Index: 2, Argv: []string{"extract", "table", "table"}, OK: false, ExitCode: 1, Attempts: 2, WallMS: 34, Failure: "exit code 1", Stderr: "boom"},
	}
}

// TestLoadBenchScenariosAcceptsRubric proves the strict decoder has the rubric
// field: without it every judged scenario would fail to load as an unknown
// field.
func TestLoadBenchScenariosAcceptsRubric(t *testing.T) {
	body := `{"scenarios":[{"id":"B01","rubric":[
		{"id":"accuracy","weight":3,"criterion":"numbers are right"},
		{"id":"brevity","weight":1,"criterion":"answer is short"}
	],"steps":[{"argv":["health"]}]}]}`
	scenarios, err := loadBenchScenarios(writeBenchScenarioFile(t, body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rubric := scenarios[0].Rubric
	if len(rubric) != 2 {
		t.Fatalf("rubric = %+v, want 2 entries", rubric)
	}
	if rubric[0].ID != "accuracy" || rubric[0].Weight != 3 || rubric[0].Criterion == "" {
		t.Errorf("rubric[0] = %+v", rubric[0])
	}
}

// TestLoadBenchScenariosRejectsBadRubric covers every rubric invariant.
func TestLoadBenchScenariosRejectsBadRubric(t *testing.T) {
	cases := map[string]string{
		"empty id":        `[{"id":"","weight":1,"criterion":"x"}]`,
		"duplicate id":    `[{"id":"a","weight":1,"criterion":"x"},{"id":"a","weight":1,"criterion":"y"}]`,
		"zero weight":     `[{"id":"a","weight":0,"criterion":"x"}]`,
		"negative weight": `[{"id":"a","weight":-1,"criterion":"x"}]`,
		"empty criterion": `[{"id":"a","weight":1,"criterion":""}]`,
	}
	for name, rubric := range cases {
		t.Run(name, func(t *testing.T) {
			body := `{"scenarios":[{"id":"B01","rubric":` + rubric + `,"steps":[{"argv":["health"]}]}]}`
			if _, err := loadBenchScenarios(writeBenchScenarioFile(t, body)); err == nil {
				t.Fatalf("expected an error for rubric %s", rubric)
			}
		})
	}
}

// TestValidateBenchJudgeAssignmentRejectsRubricWithoutJudge is the "a rubric
// nobody evaluates is a lie" rule.
func TestValidateBenchJudgeAssignmentRejectsRubricWithoutJudge(t *testing.T) {
	scenarios := []BenchScenario{judgeTestScenario()}
	if err := validateBenchJudgeAssignment(scenarios, nil); err == nil {
		t.Fatal("expected an error for a rubric without a judge command")
	}
	if err := validateBenchJudgeAssignment(scenarios, []string{"/tmp/judge"}); err != nil {
		t.Fatalf("a rubric with a judge must pass: %v", err)
	}
	// No rubric and no judge is ordinary, unjudged operation.
	plain := []BenchScenario{{ID: "B01", Steps: []BenchStep{{Argv: []string{"health"}}}}}
	if err := validateBenchJudgeAssignment(plain, nil); err != nil {
		t.Fatalf("a scenario without a rubric must not need a judge: %v", err)
	}
}

// TestBuildBenchJudgeInputGolden pins the exact stdin document the runner
// sends. A judge contract change must show up as a golden diff.
func TestBuildBenchJudgeInputGolden(t *testing.T) {
	data, err := buildBenchJudgeInput(judgeTestScenario(), judgeTestSteps())
	if err != nil {
		t.Fatal(err)
	}
	checkBenchGolden(t, "judge-stdin.json", data)
}

// TestParseBenchJudgeScoresValid accepts a complete, in-range document.
func TestParseBenchJudgeScoresValid(t *testing.T) {
	stdout := `{"scores":[{"id":"accuracy","score":0.8,"reason":"mostly right"},{"id":"brevity","score":1}]}`
	scores, err := parseBenchJudgeScores(stdout, judgeTestScenario().Rubric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(scores) != 2 || scores[0].Score != 0.8 {
		t.Fatalf("scores = %+v", scores)
	}
}

// TestParseBenchJudgeScoresRejectsBrokenContract covers every judge-output
// violation the contract names. Each is a judge error, never a zero score.
func TestParseBenchJudgeScoresRejectsBrokenContract(t *testing.T) {
	rubric := judgeTestScenario().Rubric
	cases := map[string]string{
		"unparsable":        `not json`,
		"unknown id":        `{"scores":[{"id":"accuracy","score":1},{"id":"brevity","score":1},{"id":"extra","score":1}]}`,
		"missing id":        `{"scores":[{"id":"accuracy","score":1}]}`,
		"score below range": `{"scores":[{"id":"accuracy","score":-0.1},{"id":"brevity","score":1}]}`,
		"score above range": `{"scores":[{"id":"accuracy","score":1.1},{"id":"brevity","score":1}]}`,
		"duplicate id":      `{"scores":[{"id":"accuracy","score":1},{"id":"accuracy","score":1},{"id":"brevity","score":1}]}`,
	}
	for name, stdout := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := parseBenchJudgeScores(stdout, rubric); err == nil {
				t.Fatalf("expected a judge error for %s", name)
			}
		})
	}
}

// TestWeightedBenchScoreNormalises is the weight-normalisation guard: the
// total is sum(weight*score)/sum(weight), not a plain mean.
func TestWeightedBenchScoreNormalises(t *testing.T) {
	rubric := []BenchRubric{{ID: "a", Weight: 3}, {ID: "b", Weight: 1}}
	scores := []BenchJudgeScore{{ID: "a", Score: 1}, {ID: "b", Score: 0}}
	got, err := weightedBenchScore(rubric, scores)
	if err != nil {
		t.Fatal(err)
	}
	if got != 0.75 {
		t.Fatalf("weighted score = %v, want 0.75 (3*1 + 1*0) / (3+1)", got)
	}
}

// TestApplyBenchJudgeVerdicts covers the judged verdict rules.
func TestApplyBenchJudgeVerdicts(t *testing.T) {
	s := judgeTestScenario()
	pass := func(_ BenchScenario, _ []BenchStepResult) (BenchJudgeReport, error) {
		return BenchJudgeReport{PassScore: 60, Score: 75}, nil
	}
	low := func(_ BenchScenario, _ []BenchStepResult) (BenchJudgeReport, error) {
		return BenchJudgeReport{PassScore: 60, Score: 59.99}, nil
	}
	broken := func(_ BenchScenario, _ []BenchStepResult) (BenchJudgeReport, error) {
		return BenchJudgeReport{PassScore: 60, Error: "judge exited 3: boom"}, fmt.Errorf("judge exited 3: boom")
	}

	got := applyBenchJudge(pass, s, BenchScenarioResult{Verdict: VerdictPass})
	if got.Verdict != VerdictPass || got.Judge == nil || got.Judge.Score != 75 {
		t.Fatalf("passing score: verdict = %q, judge = %+v", got.Verdict, got.Judge)
	}

	got = applyBenchJudge(pass, s, BenchScenarioResult{Verdict: VerdictPartial})
	if got.Verdict != VerdictPass {
		t.Fatalf("a judged PARTIAL above the threshold is a PASS, got %q", got.Verdict)
	}

	got = applyBenchJudge(low, s, BenchScenarioResult{Verdict: VerdictPass})
	if got.Verdict != VerdictFail || !strings.Contains(got.FailureMode, "below the pass score") {
		t.Fatalf("low score: verdict = %q, failure = %q", got.Verdict, got.FailureMode)
	}

	got = applyBenchJudge(broken, s, BenchScenarioResult{Verdict: VerdictPass})
	if got.Verdict != VerdictFail || !strings.Contains(got.FailureMode, "judge error") {
		t.Fatalf("judge error: verdict = %q, failure = %q", got.Verdict, got.FailureMode)
	}
	// The raw report survives a judge error so the failure is auditable.
	if got.Judge == nil || got.Judge.Error == "" {
		t.Errorf("a judge error must be recorded on the report: %+v", got.Judge)
	}

	// A failed scenario is not judged: a judge call would only mask the failure.
	called := false
	spy := func(_ BenchScenario, _ []BenchStepResult) (BenchJudgeReport, error) {
		called = true
		return BenchJudgeReport{}, nil
	}
	got = applyBenchJudge(spy, s, BenchScenarioResult{Verdict: VerdictFail})
	if called || got.Verdict != VerdictFail || got.Judge != nil {
		t.Fatalf("a FAIL scenario must not be judged: called=%v verdict=%q", called, got.Verdict)
	}
}

// TestRunBenchJudgePass spawns the helper judge and checks the weighted score.
func TestRunBenchJudgePass(t *testing.T) {
	t.Setenv("HTRCLI_BENCH_JUDGE_HELPER", "pass")
	report, err := runBenchJudge(helperJudgeCmd(), judgeTestScenario(), judgeTestSteps(), 60, defaultBenchJudgeTimeout)
	if err != nil {
		t.Fatalf("unexpected judge error: %v", err)
	}
	if report.Score != 75 {
		t.Errorf("score = %v, want 75 ((3*1 + 1*0)/4 * 100)", report.Score)
	}
	if len(report.Cmd) != 2 || report.Cmd[0] != os.Args[0] {
		t.Errorf("cmd = %v, want the judge argv recorded", report.Cmd)
	}
	if !strings.Contains(report.RawStdout, `"accuracy"`) {
		t.Errorf("raw_stdout = %q, want the judge's stdout", report.RawStdout)
	}
	if report.PassScore != 60 {
		t.Errorf("pass_score = %v, want 60", report.PassScore)
	}
}

// TestRunBenchJudgeCapturesStdin proves the judge actually receives the
// evidence document on stdin.
func TestRunBenchJudgeCapturesStdin(t *testing.T) {
	capture := filepath.Join(t.TempDir(), "stdin.json")
	t.Setenv("HTRCLI_BENCH_JUDGE_HELPER", "pass")
	t.Setenv("HTRCLI_BENCH_JUDGE_CAPTURE", capture)
	if _, err := runBenchJudge(helperJudgeCmd(), judgeTestScenario(), judgeTestSteps(), 60, defaultBenchJudgeTimeout); err != nil {
		t.Fatalf("unexpected judge error: %v", err)
	}
	got, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	want, err := buildBenchJudgeInput(judgeTestScenario(), judgeTestSteps())
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("judge stdin differs from the built document:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestRunBenchJudgeRejectsOutOfRangeScore proves an impossible score is a
// judge error, not a clamped value.
func TestRunBenchJudgeRejectsOutOfRangeScore(t *testing.T) {
	t.Setenv("HTRCLI_BENCH_JUDGE_HELPER", "out-of-range")
	report, err := runBenchJudge(helperJudgeCmd(), judgeTestScenario(), judgeTestSteps(), 60, defaultBenchJudgeTimeout)
	if err == nil {
		t.Fatal("expected a judge error for a score outside 0..1")
	}
	if report.Score != 0 {
		t.Errorf("report score = %v, want no score on a judge error", report.Score)
	}
	if !strings.Contains(report.Error, "between 0 and 1") {
		t.Errorf("error = %q", report.Error)
	}
}

// TestRunBenchJudgeRejectsNonZeroExit proves a failing judge fails the
// scenario with its exit status, and never looks like a zero score.
func TestRunBenchJudgeRejectsNonZeroExit(t *testing.T) {
	t.Setenv("HTRCLI_BENCH_JUDGE_HELPER", "exit-nonzero")
	report, err := runBenchJudge(helperJudgeCmd(), judgeTestScenario(), judgeTestSteps(), 60, defaultBenchJudgeTimeout)
	if err == nil {
		t.Fatal("expected a judge error for a non-zero exit")
	}
	if report.Score != 0 {
		t.Errorf("report score = %v, want no score on a judge error", report.Score)
	}
	if !strings.Contains(report.Error, "judge exited 3") {
		t.Errorf("error = %q, want the exit status", report.Error)
	}
}

// TestRunBenchJudgeRejectsUnparsableOutput proves malformed stdout is a judge
// error.
func TestRunBenchJudgeRejectsUnparsableOutput(t *testing.T) {
	t.Setenv("HTRCLI_BENCH_JUDGE_HELPER", "unparsable")
	report, err := runBenchJudge(helperJudgeCmd(), judgeTestScenario(), judgeTestSteps(), 60, defaultBenchJudgeTimeout)
	if err == nil {
		t.Fatal("expected a judge error for unparsable stdout")
	}
	if report.RawStdout != "this is not json" {
		t.Errorf("raw_stdout = %q, want it kept for audit", report.RawStdout)
	}
}

// TestRunBenchJudgeTimeout proves an overrun is a judge error naming the
// deadline, never a zero score. The stub sleeps longer than the deadline; with
// the deadline accidentally left at zero it finishes and this test fails
// rather than hanging.
func TestRunBenchJudgeTimeout(t *testing.T) {
	t.Setenv("HTRCLI_BENCH_JUDGE_HELPER", "sleep")
	t.Setenv("HTRCLI_BENCH_JUDGE_SLEEP", "2s")
	report, err := runBenchJudge(helperJudgeCmd(), judgeTestScenario(), judgeTestSteps(), 60, time.Second)
	if err == nil {
		t.Fatal("expected a judge error when the judge overruns its deadline")
	}
	if !strings.Contains(err.Error(), "judge timed out after 1s") {
		t.Errorf("error = %q, want it to name the deadline", err.Error())
	}
	if !strings.Contains(report.Error, "judge timed out after 1s") {
		t.Errorf("report error = %q, want the timeout recorded", report.Error)
	}
	if report.Score != 0 {
		t.Errorf("report score = %v, want no score on a judge error", report.Score)
	}
}

// TestRunBenchJudgeRejectsNonPositiveTimeout is the no-silent-fallback rule an
// explicit zero deadline would otherwise become: run to completion forever.
func TestRunBenchJudgeRejectsNonPositiveTimeout(t *testing.T) {
	report, err := runBenchJudge(helperJudgeCmd(), judgeTestScenario(), judgeTestSteps(), 60, 0)
	if err == nil {
		t.Fatal("expected a judge error for a non-positive deadline")
	}
	if !strings.Contains(report.Error, "judge timeout must be positive") {
		t.Errorf("report error = %q", report.Error)
	}
}

// TestValidateBenchJudgeOptionsRejectsNonPositiveTimeout pins the flag-level
// hard error and the default the flag exposes.
func TestValidateBenchJudgeOptionsRejectsNonPositiveTimeout(t *testing.T) {
	for _, timeout := range []int{0, -1} {
		if err := validateBenchJudgeOptions(60, timeout); err == nil {
			t.Fatalf("expected an error for --judge-timeout %d", timeout)
		}
	}
	if err := validateBenchJudgeOptions(60, 120); err != nil {
		t.Fatalf("the default must validate: %v", err)
	}
	flag := benchRunCmd.Flags().Lookup("judge-timeout")
	if flag == nil {
		t.Fatal("--judge-timeout is not registered")
	}
	if flag.DefValue != "120" {
		t.Errorf("--judge-timeout default = %q, want 120", flag.DefValue)
	}
}

// TestValidateBenchJudgeOptionsRejectsInvalidPassScore rejects a threshold that
// could never grade anything. NaN compares false against every bound, so the
// old range check let it through and the report later failed to marshal; a zero
// threshold makes every scenario pass, which makes the judge decorative.
func TestValidateBenchJudgeOptionsRejectsInvalidPassScore(t *testing.T) {
	cases := map[string]float64{
		"zero":      0,
		"negative":  -1,
		"above 100": 101,
		"NaN":       math.NaN(),
	}
	for name, passScore := range cases {
		t.Run(name, func(t *testing.T) {
			if err := validateBenchJudgeOptions(passScore, 120); err == nil {
				t.Fatalf("expected an error for --judge-pass-score %g", passScore)
			}
		})
	}
	if err := validateBenchJudgeOptions(60, 120); err != nil {
		t.Fatalf("60 must validate: %v", err)
	}
	if err := validateBenchJudgeOptions(100, 120); err != nil {
		t.Fatalf("100 must validate: %v", err)
	}
}

// TestBenchJudgeReportKeepsZeroScore proves a legitimate weighted total of zero
// is in the JSON. With omitempty it vanished, so a real zero score was
// indistinguishable from a report that never produced one.
func TestBenchJudgeReportKeepsZeroScore(t *testing.T) {
	report := BenchJudgeReport{
		Cmd:       []string{"/tmp/judge"},
		PassScore: 60,
		Score:     0,
		RawStdout: `{"scores":[]}`,
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"score":0`) {
		t.Fatalf("marshalled report omits a zero score: %s", data)
	}
}

// newStubJudgeServer starts an OpenAI-compatible stub that answers every
// request with content and returns the URL plus an accessor for the last body
// it received.
func newStubJudgeServer(t *testing.T, content string) (string, func() []byte) {
	t.Helper()
	var mu sync.Mutex
	var received []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read body: "+err.Error(), http.StatusInternalServerError)
			return
		}
		mu.Lock()
		received = data
		mu.Unlock()
		envelope := map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": content}}},
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(envelope); err != nil {
			t.Errorf("encode stub response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL, func() []byte {
		mu.Lock()
		defer mu.Unlock()
		return received
	}
}

// TestOpenAIJudgeScriptSendsEvidence is the hermetic end-to-end proof that the
// shipped adapter sends the scenario, rubric AND evidence to the model. The
// earlier manual proofs used a judge that scored without reading its input, so
// a rubric-only message survived them.
func TestOpenAIJudgeScriptSendsEvidence(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh is unavailable")
	}
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq is required to run the judge script")
	}
	script := filepath.Join("..", "..", "..", "docs", "benchmarks", "judges", "openai-judge.sh")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("judge script is missing: %v", err)
	}

	const marker = "EVIDENCE-MARKER-5f0c1d"
	steps := []BenchStepResult{{
		Index: 1, Argv: []string{"extract", "table", "#totals"}, OK: true,
		ExitCode: 0, Attempts: 1, WallMS: 7, Stdout: `{"data":"` + marker + `"}`,
	}}

	t.Run("sends evidence and rubric", func(t *testing.T) {
		url, received := newStubJudgeServer(t, `{"scores":[{"id":"accuracy","score":1},{"id":"brevity","score":0}]}`)
		t.Setenv("JUDGE_URL", url)
		t.Setenv("JUDGE_MODEL", "stub-model")
		t.Setenv("JUDGE_AUTH", "none")

		report, err := runBenchJudge([]string{shell, script}, judgeTestScenario(), steps, 60, 30*time.Second)
		if err != nil {
			t.Fatalf("unexpected judge error: %v", err)
		}
		if report.Score != 75 {
			t.Fatalf("weighted score = %v, want 75", report.Score)
		}
		verdict := applyBenchJudge(
			func(BenchScenario, []BenchStepResult) (BenchJudgeReport, error) { return report, nil },
			judgeTestScenario(), BenchScenarioResult{Verdict: VerdictPass},
		)
		if verdict.Verdict != VerdictPass {
			t.Fatalf("verdict = %q, want PASS", verdict.Verdict)
		}

		body := received()
		if len(body) == 0 {
			t.Fatal("the stub server received no request body")
		}
		var payload struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("request body is not the OpenAI chat payload: %v\n%s", err, body)
		}
		if len(payload.Messages) < 2 {
			t.Fatalf("messages = %d, want a system and a user message", len(payload.Messages))
		}
		user := payload.Messages[len(payload.Messages)-1].Content
		if !strings.Contains(user, marker) {
			t.Fatalf("judge user message does not carry the evidence marker %q:\n%s", marker, user)
		}
		for _, id := range []string{"accuracy", "brevity"} {
			if !strings.Contains(user, id) {
				t.Errorf("judge user message does not carry rubric id %q:\n%s", id, user)
			}
		}
		if !strings.Contains(user, "find the totals") {
			t.Errorf("judge user message does not carry the scenario goal:\n%s", user)
		}
		t.Logf("received body contains %q; weighted score %.0f; verdict %s", marker, report.Score, verdict.Verdict)
	})

	t.Run("out-of-range score is a judge error", func(t *testing.T) {
		url, _ := newStubJudgeServer(t, `{"scores":[{"id":"accuracy","score":2},{"id":"brevity","score":0}]}`)
		t.Setenv("JUDGE_URL", url)
		t.Setenv("JUDGE_MODEL", "stub-model")
		t.Setenv("JUDGE_AUTH", "none")

		report, err := runBenchJudge([]string{shell, script}, judgeTestScenario(), steps, 60, 30*time.Second)
		if err == nil {
			t.Fatalf("expected a judge error for a score above 1, report = %+v", report)
		}
		verdict := applyBenchJudge(
			func(BenchScenario, []BenchStepResult) (BenchJudgeReport, error) { return report, err },
			judgeTestScenario(), BenchScenarioResult{Verdict: VerdictPass},
		)
		if verdict.Verdict != VerdictFail || !strings.Contains(verdict.FailureMode, "judge error") {
			t.Fatalf("verdict = %q failure = %q, want FAIL on a judge error", verdict.Verdict, verdict.FailureMode)
		}
		t.Logf("out-of-range -> judge error: %v; verdict %s", err, verdict.Verdict)
	})

	t.Run("http failure surfaces status and body", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"bad token"}`))
		}))
		t.Cleanup(srv.Close)
		t.Setenv("JUDGE_URL", srv.URL)
		t.Setenv("JUDGE_MODEL", "stub-model")
		t.Setenv("JUDGE_AUTH", "none")

		report, err := runBenchJudge([]string{shell, script}, judgeTestScenario(), steps, 60, 30*time.Second)
		if err == nil {
			t.Fatal("expected a judge error on an HTTP 401")
		}
		// A bare -f discarded the body, so a 401 looked like a generic parse
		// failure. The reported error must name the status the server sent.
		if !strings.Contains(report.Error, "401") {
			t.Errorf("error = %q, want the HTTP status", report.Error)
		}
		if !strings.Contains(report.Error, "bad token") {
			t.Errorf("error = %q, want the response body", report.Error)
		}
		t.Logf("HTTP failure -> %v", err)
	})
}

// TestBenchJudgeReportGolden pins the report shape: judge fields, EXCLUDED
// accounting, and the absence of the unexported full-output fields.
func TestBenchJudgeReportGolden(t *testing.T) {
	report := BenchReport{
		StartedAt: "2026-01-02T03:04:05Z",
		Transport: "ext",
		Scenarios: []BenchScenarioResult{
			{
				ID:       "B01",
				Goal:     "find the totals",
				Verdict:  VerdictPass,
				Commands: 1,
				WallMS:   12,
				Steps: []BenchStepResult{{
					Index: 1, Argv: []string{"open", "https://x"}, OK: true,
					ExitCode: 0, Attempts: 1, WallMS: 12, Stdout: "ok",
					FullStdout: "a full stream that must not appear in the report",
				}},
				Judge: &BenchJudgeReport{
					Cmd:       []string{"/tmp/judge", "--flag"},
					PassScore: 60,
					Score:     75,
					Scores:    []BenchJudgeScore{{ID: "accuracy", Score: 1, Reason: "right"}, {ID: "brevity", Score: 0, Reason: "too long"}},
					RawStdout: `{"scores":[]}`,
				},
			},
			{ID: "B02", Verdict: VerdictExcluded, FailureMode: "no browser", Steps: []BenchStepResult{}},
		},
		Summary: BenchSummary{Total: 2, Pass: 1, Excluded: 1, WallMS: 12},
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	checkBenchGolden(t, "report-shape.json", data)
	if strings.Contains(string(data), "a full stream") {
		t.Fatal("the full-output evidence field must be json:\"-\" and absent from the report")
	}
}

// checkBenchGolden compares data with a golden file under testdata/bench.
//
// Set UPDATE_BENCH_GOLDEN=1 to rewrite it intentionally. The default fails on
// any difference, so a contract change is always reviewed.
func checkBenchGolden(t *testing.T, name string, data []byte) {
	t.Helper()
	path := filepath.Join("testdata", "bench", name)
	if os.Getenv("UPDATE_BENCH_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run with UPDATE_BENCH_GOLDEN=1 to create it)", path, err)
	}
	if string(data) != string(want) {
		t.Fatalf("golden %s differs:\ngot:\n%s\nwant:\n%s", name, data, want)
	}
}
