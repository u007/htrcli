package commands

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/u007/htrcli/internal/api"
	"github.com/u007/htrcli/internal/output"
)

// parseXY parses "x,y" into two floats. Accepts "100,200" or "100, 200".
func parseXY(s string) (float64, float64, error) {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid xy %q: expected \"x,y\" (e.g. \"100,200\")", s)
	}
	x, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid x in %q: %w", s, err)
	}
	y, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid y in %q: %w", s, err)
	}
	return x, y, nil
}

// parseSelector converts a string argument into a TargetSelector.
func parseSelector(arg string) *api.TargetSelector {
	// Element ref (@eN) — must be checked first as it's the whole selector.
	if strings.HasPrefix(arg, "@") && len(arg) > 2 && arg[1] == 'e' && arg[2] >= '0' && arg[2] <= '9' {
		return &api.TargetSelector{Ref: arg}
	}

	// Explicit viewport coordinates: xy=100,200 (viewport CSS pixels).
	if strings.HasPrefix(arg, "xy=") {
		xy := strings.TrimPrefix(arg, "xy=")
		x, y, err := parseXY(xy)
		if err == nil {
			return &api.TargetSelector{X: &x, Y: &y}
		}
		// Fall through to CSS selector on parse failure so error surfaces later.
	}

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
		if UseCDP() {
			return runInteractCDP("pressKey", "", args[0])
		}
		c := GetClient()
		tabID, err := GetTabID()
		if err != nil {
			return err
		}
		result, err := c.ExecuteCommand(tabID, api.Command{
			ID:     "1",
			Action: "pressKey",
			Value:  args[0],
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
		if UseCDP() {
			return errUnsupportedCDP("scroll")
		}
		c := GetClient()
		pixels := 500
		if len(args) > 1 {
			p, err := strconv.Atoi(args[1])
			if err != nil {
				return fmt.Errorf("invalid pixel value: %s", args[1])
			}
			pixels = p
		}

		tabID, err := GetTabID()
		if err != nil {
			return err
		}
		result, err := c.ExecuteCommand(tabID, api.Command{
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
		if err := commandError(result); err != nil {
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

var mouseDownCmd = &cobra.Command{
	Use:   "mousedown <selector>",
	Short: "Press mouse button down on element",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInteract("mouseDown", args[0], "")
	},
}

var mouseUpCmd = &cobra.Command{
	Use:   "mouseup <selector>",
	Short: "Release mouse button on element",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInteract("mouseUp", args[0], "")
	},
}

var mouseMoveCmd = &cobra.Command{
	Use:   "mousemove <selector>",
	Short: "Move mouse to element (no button)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInteract("mouseMove", args[0], "")
	},
}

var dragCmd = &cobra.Command{
	Use:   "drag <source> <target>",
	Short: "Drag from source element to target element (mouse down + move + up)",
	Long: `Drag from source selector to target selector.

Examples:
  htrcli drag "#handle" "#dropzone"
  htrcli drag "#slider" "#slider-end" --steps 10 --delay 20
  htrcli drag "@e1" "@e2"

Note: dispatches pointer/mouse events (pointerdown/mousedown, pointermove/mousemove, pointerup/mouseup).
Does NOT fire native HTML5 dragstart/dragover/drop with DataTransfer - most custom sortables/sliders use pointer events.
Use eval with DataTransfer if you need native DnD.`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		steps, _ := cmd.Flags().GetInt("steps")
		delay, _ := cmd.Flags().GetInt("delay")
		if UseCDP() {
			// CDP path: call Drag with explicit steps/delay
			s, targetID, err := cdpSession()
			if err != nil {
				return err
			}
			defer s.Close()
			// Use typed cdp.Drag via import; avoid runInteractCDP default
			// to keep steps/delay plumbing explicit.
			// Import cycle-safe: delegate to cdp directly.
			if err := runDragCDP(s, targetID, args[0], args[1], steps, delay); err != nil {
				return err
			}
			if output.JSONOutput {
				output.PrintJSON(map[string]any{"success": true, "action": "drag"})
				return nil
			}
			fmt.Printf("Drag %s -> %s (cdp, steps=%d)\n", args[0], args[1], steps)
			return nil
		}
		c := GetClient()
		tabID, err := GetTabID()
		if err != nil {
			return err
		}
		result, err := c.ExecuteCommand(tabID, api.Command{
			ID:     "1",
			Action: "drag",
			Target: parseSelector(args[0]),
			Options: map[string]any{
				"endTarget": parseSelector(args[1]),
				"steps":     steps,
				"delay":     delay,
			},
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
		fmt.Printf("Drag %s -> %s (%dms)\n", args[0], args[1], result.Duration)
		return nil
	},
}

var keyDownCmd = &cobra.Command{
	Use:   "keydown <key>",
	Short: "Press key down (hold) - use keyup to release",
	Long: `Dispatch a single keyDown. Key specs: "Enter", "Tab", "Shift", "Ctrl+a", etc.
Modifiers are per-command and stateless - caller tracks hold across keydown/keyup.

Examples:
  htrcli keydown Shift
  htrcli keydown "Ctrl+a"
  htrcli keyup Shift`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInteractWithValue("keyDown", args[0])
	},
}

var keyUpCmd = &cobra.Command{
	Use:   "keyup <key>",
	Short: "Release key (after keydown)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInteractWithValue("keyUp", args[0])
	},
}

// commandError converts a failed command result into a CLI error so the
// process exits non-zero with the extension's error message instead of
// printing a success line.
func commandError(result *api.CommandResult) error {
	if result.Success {
		return nil
	}
	return fmt.Errorf("%s", result.Error)
}

func runInteract(action, selector, value string) error {
	if UseCDP() {
		return runInteractCDP(action, selector, value)
	}
	c := GetClient()
	tabID, err := GetTabID()
	if err != nil {
		return err
	}
	result, err := c.ExecuteCommand(tabID, api.Command{
		ID:     "1",
		Action: action,
		Target: parseSelector(selector),
		Value:  value,
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

	// Format action description.
	desc := strings.Title(action)
	fmt.Printf("%s %s (%dms)\n", desc, selector, result.Duration)
	return nil
}

func runInteractWithValue(action, value string) error {
	if UseCDP() {
		return runInteractCDP(action, "", value)
	}
	c := GetClient()
	tabID, err := GetTabID()
	if err != nil {
		return err
	}
	result, err := c.ExecuteCommand(tabID, api.Command{
		ID:     "1",
		Action: action,
		Value:  value,
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
	fmt.Printf("%s %s (%dms)\n", strings.Title(action), value, result.Duration)
	return nil
}

func init() {
	dragCmd.Flags().Int("steps", 5, "Number of interpolated mouse moves (1-100)")
	dragCmd.Flags().Int("delay", 0, "Delay between moves in ms (0-2000)")
	rootCmd.AddCommand(clickCmd)
	rootCmd.AddCommand(dblclickCmd)
	rootCmd.AddCommand(fillCmd)
	rootCmd.AddCommand(typeCmd)
	rootCmd.AddCommand(hoverCmd)
	rootCmd.AddCommand(pressCmd)
	rootCmd.AddCommand(keyDownCmd)
	rootCmd.AddCommand(keyUpCmd)
	rootCmd.AddCommand(mouseDownCmd)
	rootCmd.AddCommand(mouseUpCmd)
	rootCmd.AddCommand(mouseMoveCmd)
	rootCmd.AddCommand(dragCmd)
	rootCmd.AddCommand(selectCmd)
	rootCmd.AddCommand(checkCmd)
	rootCmd.AddCommand(uncheckCmd)
	rootCmd.AddCommand(scrollCmd)
	rootCmd.AddCommand(clearCmd)
}
