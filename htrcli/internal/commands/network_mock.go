package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/u007/htrcli/internal/api"
	"github.com/u007/htrcli/internal/output"
)

var (
	mockURLPattern string
	mockStatus     int
	mockBodyFile   string
	mockMethod     string
	unmockAll      bool
	unmockPattern  string
	ruleSeq        int
)

// NOTE: the `networkCmd` parent is defined by the passive-capture plan (see
// this plan's Coordination note + Task 4 Step 0). This file only attaches
// subcommands to it — it does NOT declare `var networkCmd` in the normal
// execution order.

// buildMockRule constructs a rule map from flags. body is read from bodyFile
// (the CLI has filesystem access; the extension does not). Returns the rule as
// a map[string]any so it serializes into Command.Options cleanly.
func buildMockRule(kind, urlPattern, method string, status int, bodyFile string) (map[string]any, error) {
	if urlPattern == "" {
		return nil, fmt.Errorf("--url-pattern is required")
	}
	ruleSeq++
	rule := map[string]any{
		"id":         fmt.Sprintf("r%d", ruleSeq),
		"urlPattern": urlPattern,
		"kind":       kind,
	}
	if method != "" {
		rule["method"] = method
	}
	if kind == "fulfill" {
		rule["status"] = status
		if bodyFile != "" {
			data, err := os.ReadFile(bodyFile)
			if err != nil {
				return nil, fmt.Errorf("reading --body-file: %w", err)
			}
			rule["body"] = string(data)
		}
	}
	return rule, nil
}

func sendMockCommand(action string, options map[string]any) error {
	if UseCDP() {
		return errUnsupportedCDP("network mock/block/unmock")
	}
	c := GetClient()
	tabID, err := GetTabID()
	if err != nil {
		return err
	}
	result, err := c.ExecuteCommand(tabID, api.Command{
		ID:      "1",
		Action:  action,
		Options: options,
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
	fmt.Println("ok")
	return nil
}

var networkMockCmd = &cobra.Command{
	Use:   "mock",
	Short: "Fulfill matching requests with a mock response",
	RunE: func(cmd *cobra.Command, args []string) error {
		rule, err := buildMockRule("fulfill", mockURLPattern, mockMethod, mockStatus, mockBodyFile)
		if err != nil {
			return err
		}
		return sendMockCommand("networkMock", map[string]any{"rules": []any{rule}})
	},
}

var networkBlockCmd = &cobra.Command{
	Use:   "block",
	Short: "Block (fail) matching requests",
	RunE: func(cmd *cobra.Command, args []string) error {
		rule, err := buildMockRule("fail", mockURLPattern, mockMethod, 0, "")
		if err != nil {
			return err
		}
		return sendMockCommand("networkMock", map[string]any{"rules": []any{rule}})
	},
}

var networkUnmockCmd = &cobra.Command{
	Use:   "unmock",
	Short: "Remove mock/block rules (--all or --url-pattern)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !unmockAll && unmockPattern == "" {
			return fmt.Errorf("pass --all or --url-pattern")
		}
		options := map[string]any{"all": unmockAll}
		if unmockPattern != "" {
			options["urlPattern"] = unmockPattern
		}
		return sendMockCommand("networkUnmock", options)
	},
}

func init() {
	networkMockCmd.Flags().StringVar(&mockURLPattern, "url-pattern", "", "glob URL pattern to match")
	networkMockCmd.Flags().IntVar(&mockStatus, "status", 200, "mock response status code")
	networkMockCmd.Flags().StringVar(&mockBodyFile, "body-file", "", "file whose contents become the mock response body")
	networkMockCmd.Flags().StringVar(&mockMethod, "method", "", "restrict to an HTTP method (GET, POST, ...)")

	networkBlockCmd.Flags().StringVar(&mockURLPattern, "url-pattern", "", "glob URL pattern to block")
	networkBlockCmd.Flags().StringVar(&mockMethod, "method", "", "restrict to an HTTP method")

	networkUnmockCmd.Flags().BoolVar(&unmockAll, "all", false, "remove every rule")
	networkUnmockCmd.Flags().StringVar(&unmockPattern, "url-pattern", "", "remove rules with this exact pattern")

	// Attach to the networkCmd parent owned by the passive-capture plan. Do
	// NOT call rootCmd.AddCommand(networkCmd) here — that plan already does,
	// and a second registration panics. (Out-of-order fallback: see Step 0.)
	networkCmd.AddCommand(networkMockCmd, networkBlockCmd, networkUnmockCmd)
}
