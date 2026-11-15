package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/u007/htrcli/internal/output"
)

var (
	benchOut string
	// benchJudgeCmd is the judge argv. Repeat --judge-cmd once per argument;
	// the runner never splits a single string or goes through sh -c.
	benchJudgeCmd []string
	// benchJudgePassScore is the weighted threshold (0..100) for a judged PASS.
	benchJudgePassScore float64
	// benchJudgeTimeout bounds a judge invocation, in seconds. A judge that
	// overruns it is killed and reported as a judge error, never a zero score.
	benchJudgeTimeout int
	// benchEvidenceDir, when set, receives one full-output artifact per step.
	benchEvidenceDir string
	// benchExclusions is the path to the exclusions JSON file.
	benchExclusions string
)

// benchAllowNetwork lets a run execute scenarios marked requires_network.
// Off by default: the offline contract is the default so a suite cannot
// silently depend on connectivity.
var benchAllowNetwork bool

// benchCmd is the parent command for the benchmark harness.
var benchCmd = &cobra.Command{
	Use:   "bench",
	Short: "Run the office-task benchmark suite",
}

var benchRunCmd = &cobra.Command{
	Use:   "run <scenario-file>",
	Short: "Run a benchmark scenario file and write a report",
	Long: `Run one or more benchmark scenarios and write a JSON report.

Each scenario is a JSON document with an id, optional requires_auth and
requires_network flags, and a list of steps. A step is a raw htrcli argv plus
its expectations:

  {
    "scenarios": [
      {
        "id": "B01",
        "requires_auth": true,
        "requires_network": false,
        "usage": {"model": "driver-model", "tokens_in": 1200, "tokens_out": 340},
        "deadline_seconds": 30,
        "attempts": 3,
        "context": "work",
        "steps": [
          {"argv": ["open", "https://mail.google.com/"],
           "expect": {"exit": 0},
           "retry": 2,
           "deadline_seconds": 10,
           "optional": false}
        ]
      }
    ]
  }

Expectations: "exit" (default 0), "stdoutContains", "stdoutNotContains" and
"jsonPath" ({"path": "rows.0.Name", "equals": "Alice"} or {"exists": true}).

A scenario marked requires_auth is reported as NOT_RUN and never as PASS.
The same applies to requires_network unless --allow-network is given.
deadline_seconds bounds a scenario and a step (a step without one inherits
the scenario value); an overrun is killed and reported as TIMEOUT. attempts
caps a step's retries (retry must be <= attempts-1). context injects
--context <name> into every step and is included in the reported argv.

--judge-cmd takes a judge ARGV: repeat the flag once per argument, e.g.
--judge-cmd /path/judge --judge-cmd --flag. A scenario with a "rubric" array
is graded by that program and passes when the weighted score reaches
--judge-pass-score (default 60). The judge reads the scenario, rubric and the
run's step results on stdin and prints {"scores":[{"id","score","reason"}]};
the runner computes sum(weight*score)/sum(weight) and reports it 0..100. A
judge error (bad exit, unparsable output, unknown or missing id, score outside
0..1) FAILS the scenario; it is never reported as a zero score.

--judge-timeout (default 120) bounds the judge process in seconds. A judge
that overruns it is killed and reported as a judge error, "judge timed out
after 120s", which FAILS the scenario; it is never reported as a zero score.
A value <= 0 is rejected.

--exclusions takes a JSON file {"excluded":[{"id","reason"}]}; each listed
scenario is reported EXCLUDED, counted separately, and runs no steps.

--evidence-dir writes one artifact per step to
<dir>/<scenario-id>/<NNN>-step.json with the full (untruncated) stdout/stderr.
Screenshots are NOT captured: that would need an extra browser call per step.

The report is written to --out (default docs/benchmarks/results/<timestamp>.json).`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		scenarios, err := loadBenchScenarios(args[0])
		if err != nil {
			return err
		}
		if transportFlag != "" && transportFlag != "ext" && transportFlag != "cdp" {
			return fmt.Errorf("--transport must be ext or cdp, got %q", transportFlag)
		}
		// Root --transport and --cdp resolve the transport, same as every other
		// command. The default is ext.
		transport := "ext"
		if UseCDP() {
			transport = "cdp"
		}

		// A rubric nobody evaluates is a lie, so it is rejected before any
		// step runs. A threshold outside 0..100 could never grade anything.
		if err := validateBenchJudgeAssignment(scenarios, benchJudgeCmd); err != nil {
			return err
		}
		if err := validateBenchJudgeOptions(benchJudgePassScore, benchJudgeTimeout); err != nil {
			return err
		}

		exclusions := map[string]string{}
		if benchExclusions != "" {
			var err error
			exclusions, err = loadBenchExclusions(benchExclusions, scenarios)
			if err != nil {
				return err
			}
		}

		// An uncreatable evidence path is a hard error, not a warning: a run
		// that silently drops evidence is worse than one that never starts.
		if benchEvidenceDir != "" {
			if err := os.MkdirAll(benchEvidenceDir, 0o755); err != nil {
				return fmt.Errorf("create evidence directory %s: %w", benchEvidenceDir, err)
			}
		}

		var judge benchJudge
		if len(benchJudgeCmd) > 0 {
			judgeCmd := benchJudgeCmd
			passScore := benchJudgePassScore
			timeout := time.Duration(benchJudgeTimeout) * time.Second
			judge = func(s BenchScenario, steps []BenchStepResult) (BenchJudgeReport, error) {
				return runBenchJudge(judgeCmd, s, steps, passScore, timeout)
			}
		}

		start := time.Now()
		results := make([]BenchScenarioResult, 0, len(scenarios))
		for _, s := range scenarios {
			if reason, ok := exclusions[s.ID]; ok {
				results = append(results, excludedBenchResult(s, reason))
				continue
			}
			res := runBenchScenario(s, benchStepRunnerFor(transport))
			if judge != nil && len(s.Rubric) > 0 {
				res = applyBenchJudge(judge, s, res)
			}
			if benchEvidenceDir != "" {
				if err := writeBenchEvidence(benchEvidenceDir, res); err != nil {
					return err
				}
			}
			results = append(results, res)
		}
		report := BenchReport{
			StartedAt: start.Format(time.RFC3339),
			Transport: transport,
			Scenarios: results,
			Summary:   summarizeBenchReport(results, time.Since(start).Milliseconds()),
		}

		outPath := benchOut
		if outPath == "" {
			outPath = defaultBenchReportPath(start)
		}
		if err := writeBenchReport(outPath, report); err != nil {
			return err
		}

		return finishBenchRun(report, outPath)
	},
}

// finishBenchRun prints a finished report in the requested mode and returns a
// non-zero-exit error when any scenario failed.
//
// The failure check must apply to BOTH output modes: the JSON branch used to
// return before it, so `bench run --json` exited 0 for a failing suite and CI
// saw success.
func finishBenchRun(report BenchReport, outPath string) error {
	if output.JSONOutput {
		printBenchReportJSON(report)
	} else {
		printBenchSummary(report, outPath)
	}
	if report.Summary.Fail > 0 {
		return fmt.Errorf("%d scenario(s) failed", report.Summary.Fail)
	}
	if report.Summary.Timeout > 0 {
		return fmt.Errorf("%d scenario(s) timed out", report.Summary.Timeout)
	}
	return nil
}

// defaultBenchReportPath is the report path when --out is not given.
func defaultBenchReportPath(start time.Time) string {
	return filepath.Join("docs", "benchmarks", "results",
		start.Format("20060102-150405")+".json")
}

// writeBenchReport writes the report as pretty JSON, creating the directory.
func writeBenchReport(path string, report BenchReport) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create report directory: %w", err)
		}
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode report: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}

// printBenchSummary prints a one-line-per-scenario summary.
func printBenchSummary(report BenchReport, outPath string) {
	for _, r := range report.Scenarios {
		line := fmt.Sprintf("%-8s %-8s %3d cmds %6dms  tokens %s",
			r.ID, r.Verdict, r.Commands, r.WallMS, formatBenchUsage(r.Usage))
		if r.FailureMode != "" {
			line += "  — " + r.FailureMode
		}
		fmt.Println(line)
	}
	s := report.Summary
	fmt.Printf("total %d  pass %d  partial %d  fail %d  not_run %d  timeout %d  excluded %d  tokens %s  (%dms)\n",
		s.Total, s.Pass, s.Partial, s.Fail, s.NotRun, s.Timeout, s.Excluded, formatBenchUsageTotals(s.Usage), s.WallMS)
	fmt.Printf("report: %s\n", outPath)
}

// formatBenchUsage renders a scenario's token accounting, or an em-dash when
// the driver declared none. A zero would read as "free", which is a different
// claim from "not measured".
func formatBenchUsage(u *BenchUsage) string {
	if u == nil {
		return "—"
	}
	return fmt.Sprintf("%d/%d", u.TokensIn, u.TokensOut)
}

// formatBenchUsageTotals renders the run's token totals, or an em-dash when no
// scenario declared usage.
func formatBenchUsageTotals(u *BenchUsageTotals) string {
	if u == nil {
		return "—"
	}
	return fmt.Sprintf("%d in / %d out", u.TokensIn, u.TokensOut)
}

// printBenchReportJSON prints the whole report as JSON.
func printBenchReportJSON(report BenchReport) {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "encode report: %v\n", err)
		return
	}
	fmt.Println(string(data))
}

func init() {
	benchRunCmd.Flags().StringVar(&benchOut, "out", "", "report path (default docs/benchmarks/results/<timestamp>.json)")
	benchRunCmd.Flags().BoolVar(&benchAllowNetwork, "allow-network", false,
		"run scenarios marked requires_network (the offline contract is the default)")
	benchRunCmd.Flags().StringArrayVar(&benchJudgeCmd, "judge-cmd", nil,
		"judge argv; repeat once per argument (no shell, no space splitting)")
	benchRunCmd.Flags().Float64Var(&benchJudgePassScore, "judge-pass-score", 60,
		"minimum weighted judge score (>0 and at most 100) for a judged PASS")
	benchRunCmd.Flags().IntVar(&benchJudgeTimeout, "judge-timeout", int(defaultBenchJudgeTimeout/time.Second),
		"judge deadline in seconds; an overrun is a judge error (must be > 0)")
	benchRunCmd.Flags().StringVar(&benchEvidenceDir, "evidence-dir", "",
		"write one full-output artifact per step under <dir>/<scenario-id>/ (no screenshots)")
	benchRunCmd.Flags().StringVar(&benchExclusions, "exclusions", "",
		"JSON file of excluded scenario ids with reasons")
	benchCmd.AddCommand(benchRunCmd)
	rootCmd.AddCommand(benchCmd)
}
