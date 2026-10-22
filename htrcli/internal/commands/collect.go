package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/u007/htrcli/internal/api"
	"github.com/u007/htrcli/internal/cdp"
	"github.com/u007/htrcli/internal/output"
)

var (
	collectMaxSteps  int
	collectStepDelay int
	collectMaxItems  int
	collectRef       bool
)

var collectCmd = &cobra.Command{
	Annotations: FrameAwareAnnotations(),
	Use:         "collect <selector>",
	Short:       "Collect every match in a virtualized list (scrolls and accumulates)",
	Long: `Collect every element matching a selector, scrolling the list's scroll
container until the rows stop changing. Use this instead of findAll for long
or virtualized lists (Gmail threads, Drive rows, chat messages): findAll only
sees the rows currently in the DOM, collect scrolls and accumulates them.

Output: a JSON object with "elements" (array of element info), "steps" (scroll
steps performed), "truncated" (true when collection stopped before the list
ended), "truncatedBy" ("maxItems" or "maxSteps", null when not truncated) and
"scrolled" (false when no scrollable container was found).`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := checkFrameTarget(parseSelector(args[0])); err != nil {
			return err
		}
		options := map[string]any{}
		if collectMaxSteps > 0 {
			options["maxSteps"] = collectMaxSteps
		}
		if collectStepDelay >= 0 {
			options["stepDelayMs"] = collectStepDelay
		}
		if collectMaxItems > 0 {
			options["maxItems"] = collectMaxItems
		}
		if collectRef {
			options["assignRef"] = true
		}

		if UseCDP() {
			if err := runCollectCDP(args[0], options); err != nil {
				return err
			}
			return nil
		}

		c := GetClient()
		tabID, err := GetTabID()
		if err != nil {
			return err
		}
		payload := api.Command{
			ID:      "1",
			Action:  "collectAll",
			Target:  parseSelector(args[0]),
			Options: options,
		}
		result, err := c.ExecuteCommand(tabID, payload)
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
		if result.Data == nil {
			fmt.Printf("No elements found: %s\n", args[0])
			return nil
		}
		output.PrintJSON(result.Data)
		printCollectWarnings(result.Data)
		return nil
	},
}

// printCollectWarnings surfaces the collection caveats that the JSON payload
// carries, so a truncated or unreliably-de-duplicated result is visible in
// the default (non-JSON) output too.
func printCollectWarnings(data any) {
	m, ok := data.(map[string]any)
	if !ok {
		return
	}
	if cause, ok := m["truncatedBy"].(string); ok {
		switch cause {
		case "maxItems":
			fmt.Fprintln(os.Stderr, "warning: collection stopped at the --max-items cap; more rows may exist (raise --max-items)")
		case "maxSteps":
			fmt.Fprintln(os.Stderr, "warning: collection hit the step limit; the list may be incomplete (raise --max-steps)")
		}
	}
	if strategy, ok := m["keyStrategy"].(string); ok && strategy == "index" {
		fmt.Fprintln(os.Stderr, "warning: rows had no stable id or text; de-duplication tracked DOM nodes, so the result may contain duplicates or be missing rows")
	} else if strategy, ok := m["keyStrategy"].(string); ok && strategy == "text" {
		fmt.Fprintln(os.Stderr, "note: rows were de-duplicated by text; rows with identical text may have been merged")
	}
}

// runCollectCDP runs the collectAll action through the embedded DOM bundle.
func runCollectCDP(selector string, options map[string]any) error {
	s, _, err := cdpSession()
	if err != nil {
		return err
	}
	defer s.Close()

	cmd := api.Command{
		ID:      "1",
		Action:  "collectAll",
		Target:  parseSelector(selector),
		Options: options,
	}
	result, err := cdp.ExecDOM(s, cmd)
	if err != nil {
		return err
	}
	if !result.Success {
		return fmt.Errorf("collectAll failed: %s", result.Error)
	}
	if output.JSONOutput {
		output.PrintJSON(result)
		return nil
	}
	// Same wire format as the extension path and every sibling command: the
	// command's own help promises "a JSON object", and benchmark steps parse
	// stdout as JSON via `jsonPath`.
	output.PrintJSON(result.Data)
	printCollectWarnings(result.Data)
	return nil
}

func init() {
	collectCmd.Flags().IntVar(&collectMaxSteps, "max-steps", 0, "Max scroll steps (0 = library default)")
	collectCmd.Flags().IntVar(&collectStepDelay, "step-delay", 120, "Milliseconds between scroll steps")
	collectCmd.Flags().IntVar(&collectMaxItems, "max-items", 0, "Stop after collecting N elements (0 = no limit)")
	collectCmd.Flags().BoolVar(&collectRef, "ref", false, "mint persistent element refs for every collected row")
	rootCmd.AddCommand(collectCmd)
}
