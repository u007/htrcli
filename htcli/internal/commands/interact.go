package commands

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/u007/htcli/internal/api"
	"github.com/u007/htcli/internal/output"
)

// parseSelector converts a string argument into a TargetSelector.
func parseSelector(arg string) *api.TargetSelector {
	// Check for prefix patterns.
	if strings.HasPrefix(arg, "name=") {
		return &api.TargetSelector{Name: strings.TrimPrefix(arg, "name=")}
	}
	if strings.HasPrefix(arg, "role=") {
		return &api.TargetSelector{Role: strings.TrimPrefix(arg, "role=")}
	}
	if strings.HasPrefix(arg, "text=") {
		return &api.TargetSelector{Text: strings.TrimPrefix(arg, "text=")}
	}
	if strings.HasPrefix(arg, "label=") {
		return &api.TargetSelector{Label: strings.TrimPrefix(arg, "label=")}
	}
	if strings.HasPrefix(arg, "placeholder=") {
		return &api.TargetSelector{Placeholder: strings.TrimPrefix(arg, "placeholder=")}
	}
	if strings.HasPrefix(arg, "id=") {
		return &api.TargetSelector{ID: strings.TrimPrefix(arg, "id=")}
	}
	if strings.HasPrefix(arg, "xpath=") {
		return &api.TargetSelector{XPath: strings.TrimPrefix(arg, "xpath=")}
	}

	// Default to CSS selector.
	return &api.TargetSelector{Selector: arg}
}

var clickCmd = &cobra.Command{
	Use:   "click <selector>",
	Short: "Click element",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInteract("click", args[0], "")
	},
}

var dblclickCmd = &cobra.Command{
	Use:   "dblclick <selector>",
	Short: "Double-click element",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInteract("dblclick", args[0], "")
	},
}

var fillCmd = &cobra.Command{
	Use:   "fill <selector> <value>",
	Short: "Clear and fill input",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInteract("fill", args[0], args[1])
	},
}

var typeCmd = &cobra.Command{
	Use:   "type <selector> <value>",
	Short: "Type into input (appends)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInteract("type", args[0], args[1])
	},
}

var hoverCmd = &cobra.Command{
	Use:   "hover <selector>",
	Short: "Hover element",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInteract("hover", args[0], "")
	},
}

var pressCmd = &cobra.Command{
	Use:   "press <key>",
	Short: "Press key (Enter, Tab, Ctrl+a, etc.)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := GetClient()
		result, err := c.ExecuteCommand(GetTabID(), api.Command{
			ID:     "1",
			Action: "pressKey",
			Value:  args[0],
		})
		if err != nil {
			return err
		}

		if output.JSONOutput {
			output.PrintJSON(result)
			return nil
		}

		fmt.Printf("Pressed %s (%dms)\n", args[0], result.Duration)
		return nil
	},
}

var selectCmd = &cobra.Command{
	Use:   "select <selector> <value>",
	Short: "Select dropdown option",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInteract("select", args[0], args[1])
	},
}

var checkCmd = &cobra.Command{
	Use:   "check <selector>",
	Short: "Check checkbox",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInteract("check", args[0], "")
	},
}

var uncheckCmd = &cobra.Command{
	Use:   "uncheck <selector>",
	Short: "Uncheck checkbox",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInteract("uncheck", args[0], "")
	},
}

var scrollCmd = &cobra.Command{
	Use:   "scroll <direction> [pixels]",
	Short: "Scroll page (up, down, left, right)",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := GetClient()
		pixels := 500
		if len(args) > 1 {
			p, err := strconv.Atoi(args[1])
			if err != nil {
				return fmt.Errorf("invalid pixel value: %s", args[1])
			}
			pixels = p
		}

		result, err := c.ExecuteCommand(GetTabID(), api.Command{
			ID:     "1",
			Action: "scrollTo",
			Value:  args[0],
			Options: map[string]any{
				"pixels": pixels,
			},
		})
		if err != nil {
			return err
		}

		if output.JSONOutput {
			output.PrintJSON(result)
			return nil
		}

		fmt.Printf("Scrolled %s %dpx (%dms)\n", args[0], pixels, result.Duration)
		return nil
	},
}

var clearCmd = &cobra.Command{
	Use:   "clear <selector>",
	Short: "Clear input field",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInteract("clear", args[0], "")
	},
}

func runInteract(action, selector, value string) error {
	c := GetClient()
	result, err := c.ExecuteCommand(GetTabID(), api.Command{
		ID:     "1",
		Action: action,
		Target: parseSelector(selector),
		Value:  value,
	})
	if err != nil {
		return err
	}

	if output.JSONOutput {
		output.PrintJSON(result)
		return nil
	}

	// Format action description.
	desc := strings.Title(action)
	fmt.Printf("%s %s (%dms)\n", desc, selector, result.Duration)
	return nil
}

func init() {
	rootCmd.AddCommand(clickCmd)
	rootCmd.AddCommand(dblclickCmd)
	rootCmd.AddCommand(fillCmd)
	rootCmd.AddCommand(typeCmd)
	rootCmd.AddCommand(hoverCmd)
	rootCmd.AddCommand(pressCmd)
	rootCmd.AddCommand(selectCmd)
	rootCmd.AddCommand(checkCmd)
	rootCmd.AddCommand(uncheckCmd)
	rootCmd.AddCommand(scrollCmd)
	rootCmd.AddCommand(clearCmd)
}
