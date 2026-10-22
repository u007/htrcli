package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/u007/htrcli/internal/api"
	"github.com/u007/htrcli/internal/cdp"
	"github.com/u007/htrcli/internal/output"
)

var (
	extractFields    string
	extractContainer bool
)

var extractCmd = &cobra.Command{
	Annotations: FrameAwareAnnotations(),
	Use:         "extract <table|list> <selector>",
	Short:       "Extract structured data (table rows or list items) from the page",
	Long: `Extract structured data from the page as JSON.

  extract table <selector>   Read an HTML <table> (or the first table inside
                             the matched container) into {columns, rows}.
  extract list <selector>    Read a list of similar elements into {items}.
                             With --fields "name:sel,age:sel" each item
                             becomes an object with one value per field;
                             otherwise each item is {text}.
                             The selector names the ITEMS; pass --container
                             when it names a wrapper whose children are the
                             items.

Field selectors:
  ".title"          a descendant's TEXT
  "@data-due"       the item's OWN attribute value
  "@.link@href"     a DESCENDANT's attribute value (selector@attribute)
  "[data-due]"      an attribute value: the first descendant match, else the
                    item itself
`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Dispatch to the right subcommand based on args[0].
		switch args[0] {
		case "table":
			return runExtractTable(args[1])
		case "list":
			return runExtractList(args[1])
		default:
			return fmt.Errorf("unknown extract mode %q (use 'table' or 'list')", args[0])
		}
	},
}

var extractTableCmd = &cobra.Command{
	Annotations: FrameAwareAnnotations(),
	Use:         "table <selector>",
	Short:       "Extract an HTML table into columns + rows",
	Args:        cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runExtractTable(args[0])
	},
}

var extractListCmd = &cobra.Command{
	Annotations: FrameAwareAnnotations(),
	Use:         "list <selector>",
	Short:       "Extract a list of similar elements into structured items",
	Args:        cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runExtractList(args[0])
	},
}

func runExtractTable(selector string) error {
	if err := checkFrameTarget(parseSelector(selector)); err != nil {
		return err
	}
	if UseCDP() {
		s, _, err := cdpSession()
		if err != nil {
			return err
		}
		defer s.Close()
		cmd := api.Command{ID: "1", Action: "extractTable", Target: parseSelector(selector)}
		result, err := cdp.ExecDOM(s, cmd)
		if err != nil {
			return err
		}
		if !result.Success {
			return fmt.Errorf("extractTable failed: %s", result.Error)
		}
		if output.JSONOutput {
			output.PrintJSON(result)
			return nil
		}
		output.PrintJSON(result.Data)
		return nil
	}
	c := GetClient()
	tabID, err := GetTabID()
	if err != nil {
		return err
	}
	payload := api.Command{ID: "1", Action: "extractTable", Target: parseSelector(selector)}
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
	output.PrintJSON(result.Data)
	return nil
}

func parseExtractFields(s string) map[string]any {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	out := map[string]any{}
	for _, tok := range strings.Split(s, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		parts := strings.SplitN(tok, ":", 2)
		if len(parts) != 2 {
			continue
		}
		out[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func runExtractList(selector string) error {
	if err := checkFrameTarget(parseSelector(selector)); err != nil {
		return err
	}
	options := map[string]any{}
	if fields := parseExtractFields(extractFields); fields != nil {
		options["fields"] = fields
	}
	if extractContainer {
		options["container"] = true
	}
	if UseCDP() {
		s, _, err := cdpSession()
		if err != nil {
			return err
		}
		defer s.Close()
		cmd := api.Command{ID: "1", Action: "extractList", Target: parseSelector(selector), Options: options}
		result, err := cdp.ExecDOM(s, cmd)
		if err != nil {
			return err
		}
		if !result.Success {
			return fmt.Errorf("extractList failed: %s", result.Error)
		}
		if output.JSONOutput {
			output.PrintJSON(result)
			return nil
		}
		output.PrintJSON(result.Data)
		return nil
	}
	c := GetClient()
	tabID, err := GetTabID()
	if err != nil {
		return err
	}
	payload := api.Command{ID: "1", Action: "extractList", Target: parseSelector(selector), Options: options}
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
	output.PrintJSON(result.Data)
	return nil
}

func init() {
	extractCmd.Flags().StringVar(&extractFields, "fields", "", "Comma-separated name:selector pairs for list extraction (e.g. \"name:.name,age:.age\")")
	extractTableCmd.Flags().StringVar(&extractFields, "fields", "", "Comma-separated name:selector pairs for list extraction")
	extractListCmd.Flags().StringVar(&extractFields, "fields", "", "Comma-separated name:selector pairs for list extraction")
	extractListCmd.Flags().BoolVar(&extractContainer, "container", false, "Treat the selector as a container and use its children as the items")
	extractCmd.AddCommand(extractTableCmd)
	extractCmd.AddCommand(extractListCmd)
	rootCmd.AddCommand(extractCmd)
}
