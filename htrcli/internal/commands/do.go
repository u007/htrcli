package commands

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/u007/htrcli/internal/api"
	"github.com/u007/htrcli/internal/output"
)

// doStep is one entry of the `do` command's JSON step list. It mirrors
// api.Command, but takes the selector as the same prefixed string the single
// verbs accept ("#id", "text=Move", "role=menuitem", "@e3", "xy=10,20").
type doStep struct {
	Action   string         `json:"action"`
	Selector string         `json:"selector"`
	Value    string         `json:"value"`
	Options  map[string]any `json:"options"`
	// WaitMs delays BEFORE the step; DelayMs delays AFTER it.
	WaitMs  int `json:"waitMs"`
	DelayMs int `json:"delayMs"`
}

// buildDoSteps validates the parsed steps and converts them to the wire form
// the content script's `sequence` handler reads: options.steps[] with a
// parsed target, and no field the caller did not set.
func buildDoSteps(parsed []doStep) ([]map[string]any, error) {
	if len(parsed) == 0 {
		return nil, fmt.Errorf(`no steps: expected a JSON array such as [{"action":"click","selector":"#open"}]`)
	}

	steps := make([]map[string]any, 0, len(parsed))
	for i, s := range parsed {
		action := strings.TrimSpace(s.Action)
		if action == "" {
			return nil, fmt.Errorf("step %d has no \"action\"", i+1)
		}
		if action == "sequence" {
			return nil, fmt.Errorf("step %d: sequence steps cannot nest", i+1)
		}

		step := map[string]any{"action": action}
		if s.Selector != "" {
			step["target"] = parseSelector(s.Selector)
		}
		if s.Value != "" {
			step["value"] = s.Value
		}
		if len(s.Options) > 0 {
			step["options"] = s.Options
		}
		if s.WaitMs > 0 {
			step["waitMs"] = s.WaitMs
		}
		if s.DelayMs > 0 {
			step["delayMs"] = s.DelayMs
		}
		steps = append(steps, step)
	}
	return steps, nil
}

var doCmd = &cobra.Command{
	Use:   "do <steps-json>",
	Short: "Run several actions in one command (a menu opened by one step survives the next)",
	Long: `Run several actions as one command, in order.

A menu (or submenu) opened by one browser command is dismissed before the next
command arrives, and a focused field loses focus. Anything that needs "open
this, then click that" only works when both interactions travel together, so
the steps run inside a single page round trip.

Steps are a JSON array. Each step takes:
  action    (required) click, dblclick, rightclick, hover, fill, type,
            select, pressKey, check, uncheck, clear, scrollTo, ...
  selector  optional, same prefix forms as the single commands
            (#id, text=, role=, label=, name=, placeholder=, xpath=, xy=, @eN)
  value     optional
  options   optional map (timeout is the useful one)
  waitMs    delay before the step, in ms
  delayMs   delay after the step, in ms

Examples:
  htrcli do '[{"action":"rightclick","selector":"tr[data-id=7]"},{"action":"click","selector":"text=Move","waitMs":300}]'
  htrcli do '[{"action":"click","selector":"#open"},{"action":"fill","selector":"#to","value":"a@b.c"},{"action":"pressKey","value":"Enter"}]'

The first failing step aborts the run and names itself ("sequence step 2
(click) failed: ..."). Nothing after it runs.

--cdp is not supported: the steps must share one page round trip, which
per-command CDP calls cannot provide.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if UseCDP() {
			return fmt.Errorf("do is not supported with --cdp: steps must run in one page round trip")
		}

		var parsed []doStep
		if err := json.Unmarshal([]byte(args[0]), &parsed); err != nil {
			return fmt.Errorf("invalid steps JSON: %w", err)
		}
		steps, err := buildDoSteps(parsed)
		if err != nil {
			return err
		}

		c := GetClient()
		tabID, err := GetTabID()
		if err != nil {
			return err
		}
		result, err := c.ExecuteCommand(tabID, api.Command{
			ID:      "1",
			Action:  "sequence",
			Options: map[string]any{"steps": steps},
		})
		if err != nil {
			return err
		}
		if err := commandError(result); err != nil {
			return err
		}

		if output.JSONOutput {
			output.PrintJSON(result)
			return nil
		}

		fmt.Printf("Ran %d steps (%dms)\n", len(steps), result.Duration)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(doCmd)
}
