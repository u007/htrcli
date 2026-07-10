package commands

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/u007/htcli/internal/output"
)

var tabsCmd = &cobra.Command{
	Use:   "tabs",
	Short: "Manage browser tabs",
}

var tabsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List connected browser tabs",
	RunE: func(cmd *cobra.Command, args []string) error {
		if UseCDP() {
			return runTabsListCDP()
		}
		c := GetClient()
		tabs, err := c.ListTabs()
		if err != nil {
			return err
		}

		if output.JSONOutput {
			output.PrintJSON(tabs)
			return nil
		}

		if len(tabs) == 0 {
			fmt.Println("No tabs connected")
			return nil
		}

		table := output.NewTable("ID", "Title", "URL", "Active")
		for _, t := range tabs {
			active := "no"
			if t.Active {
				active = "yes"
			}
			title := t.Title
			if len(title) > 40 {
				title = title[:37] + "..."
			}
			url := t.URL
			if len(url) > 40 {
				url = url[:37] + "..."
			}
			table.AddRow(strconv.Itoa(t.ID), title, url, active)
		}
		fmt.Print(table)
		return nil
	},
}

var tabsGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get information about a specific tab",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if UseCDP() {
			return errUnsupportedCDP("tabs get")
		}
		id, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid tab ID: %s", args[0])
		}

		c := GetClient()
		tab, err := c.GetTab(id)
		if err != nil {
			return err
		}

		if output.JSONOutput {
			output.PrintJSON(tab)
			return nil
		}

		fmt.Printf("ID:      %d\n", tab.ID)
		fmt.Printf("Title:   %s\n", tab.Title)
		fmt.Printf("URL:     %s\n", tab.URL)
		fmt.Printf("Active:  %v\n", tab.Active)
		if tab.FavIconURL != "" {
			fmt.Printf("Favicon: %s\n", tab.FavIconURL)
		}
		return nil
	},
}

func init() {
	tabsCmd.AddCommand(tabsListCmd)
	tabsCmd.AddCommand(tabsGetCmd)
	rootCmd.AddCommand(tabsCmd)
}
