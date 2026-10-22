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

var benchOut string

// benchCmd is the parent command for the benchmark harness.
var benchCmd = &cobra.Command{
	Use:   "bench",
	Short: "Run the office-task benchmark suite",
}

var benchRunCmd = &cobra.Command{
	Use:   "run <scenario-file>",
	Short: "Run a benchmark scenario file and write a report",
	Long: `Run one or more benchmark scenarios and write a JSON report.

Each scenario is a JSON document with an id, an optional requires_auth flag
and a list of steps. A step is a raw htrcli argv plus its expectations:

  {
    "scenarios": [
      {
        "id": "B01",
        "requires_auth": true,
        "steps": [
          {"argv": ["open", "https://mail.google.com/"],
           "expect": {"exit": 0},
           "retry": 2,
           "optional": false}
        ]
      }
    ]
  }

Expectations: "exit" (default 0), "stdoutContains", "stdoutNotContains" and
"jsonPath" ({"path": "rows.0.Name", "equals": "Alice"} or {"exists": true}).

A scenario marked requires_auth is reported as NOT_RUN and never as PASS.
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

		start := time.Now()
		results := make([]BenchScenarioResult, 0, len(scenarios))
		for _, s := range scenarios {
			res := runBenchScenario(s, benchStepRunnerFor(transport))
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
		line := fmt.Sprintf("%-8s %-8s %3d cmds %6dms", r.ID, r.Verdict, r.Commands, r.WallMS)
		if r.FailureMode != "" {
			line += "  — " + r.FailureMode
		}
		fmt.Println(line)
	}
	s := report.Summary
	fmt.Printf("total %d  pass %d  partial %d  fail %d  not_run %d  (%dms)\n",
		s.Total, s.Pass, s.Partial, s.Fail, s.NotRun, s.WallMS)
	fmt.Printf("report: %s\n", outPath)
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
	benchCmd.AddCommand(benchRunCmd)
	rootCmd.AddCommand(benchCmd)
}
