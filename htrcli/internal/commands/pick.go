package commands

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/u007/htrcli/internal/cdp"
	"github.com/u007/htrcli/internal/output"
)

var pickCmd = &cobra.Command{
	Annotations: FrameAwareAnnotations(),
	Use:         "pick <selector> <text>",
	Short:       "Choose an option in a combobox (react-select, autocomplete) or <select> (--cdp only)",
	Long: `Choose the option matching <text> in a custom combobox or a native <select>.

Custom comboboxes (react-select, async autocompletes): focuses the input,
presses ArrowDown, types <text> with trusted input, waits up to 3s for
role=option entries (preferring the listbox named by aria-controls/aria-owns),
clicks the best match with a trusted mouse click (exact > prefix > contains,
case-insensitive; a tie is an error), then verifies the widget displays it.
On failure the error lists up to 15 available options.

Native <select>: resolves <text> to an option by value or visible text and
selects it with the select action, then verifies the selection.

Examples:
  htrcli --cdp pick "#country" "Malaysia"
  htrcli --cdp pick "#location" "San Francisco, California, United States"
  htrcli --cdp --frame "#grnhse_iframe" pick "#gender" "Decline to self identify"`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !UseCDP() {
			return errCDPOnly("pick")
		}
		return runPickCDP(args[0], args[1])
	},
}

func runPickCDP(selector, text string) error {
	sel := parseSelector(selector)
	if err := checkFrameTarget(sel); err != nil {
		return err
	}
	s, targetID, err := cdpSession()
	if err != nil {
		return err
	}
	defer s.Close()

	res, err := cdp.Pick(s, targetID, sel, text)
	if err != nil {
		return err
	}
	if output.JSONOutput {
		output.PrintJSON(map[string]any{
			"success": true, "action": "pick", "selector": selector,
			"text": text, "selected": res.Selected, "mode": res.Mode,
		})
		return nil
	}
	fmt.Printf("Picked %q in %s (cdp)\n", text, selector)
	return nil
}

func init() {
	rootCmd.AddCommand(pickCmd)
}
