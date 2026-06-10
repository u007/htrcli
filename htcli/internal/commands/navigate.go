package commands

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/u007/htcli/internal/api"
	"github.com/u007/htcli/internal/output"
)

var openCmd = &cobra.Command{
	Use:   "open <url>",
	Short: "Navigate to URL",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := GetClient()
		result, err := c.ExecuteCommand(GetTabID(), api.Command{
			ID:     "1",
			Action: "navigate",
			Value:  args[0],
		})
		if err != nil {
			return err
		}

		if output.JSONOutput {
			output.PrintJSON(result)
			return nil
		}

		fmt.Printf("Navigated to %s (%dms)\n", args[0], result.Duration)
		return nil
	},
}

var backCmd = &cobra.Command{
	Use:   "back",
	Short: "Go back",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := GetClient()
		result, err := c.ExecuteCommand(GetTabID(), api.Command{
			ID:     "1",
			Action: "goBack",
		})
		if err != nil {
			return err
		}

		if output.JSONOutput {
			output.PrintJSON(result)
			return nil
		}

		fmt.Printf("Went back (%dms)\n", result.Duration)
		return nil
	},
}

var forwardCmd = &cobra.Command{
	Use:   "forward",
	Short: "Go forward",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := GetClient()
		result, err := c.ExecuteCommand(GetTabID(), api.Command{
			ID:     "1",
			Action: "goForward",
		})
		if err != nil {
			return err
		}

		if output.JSONOutput {
			output.PrintJSON(result)
			return nil
		}

		fmt.Printf("Went forward (%dms)\n", result.Duration)
		return nil
	},
}

var reloadCmd = &cobra.Command{
	Use:   "reload",
	Short: "Reload page",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := GetClient()
		result, err := c.ExecuteCommand(GetTabID(), api.Command{
			ID:     "1",
			Action: "reload",
		})
		if err != nil {
			return err
		}

		if output.JSONOutput {
			output.PrintJSON(result)
			return nil
		}

		fmt.Printf("Reloaded (%dms)\n", result.Duration)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(openCmd)
	rootCmd.AddCommand(backCmd)
	rootCmd.AddCommand(forwardCmd)
	rootCmd.AddCommand(reloadCmd)
}
