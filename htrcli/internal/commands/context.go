package commands

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"github.com/u007/htrcli/internal/cdp"
	"github.com/u007/htrcli/internal/output"
)

var contextCmd = &cobra.Command{
	Use:   "context",
	Short: "Manage named browser contexts",
}

var contextListCmd = &cobra.Command{
	Use:   "list",
	Short: "List named browser contexts",
	RunE: func(cmd *cobra.Command, args []string) error {
		entries, err := cdp.ReadContexts()
		if err != nil {
			return err
		}

		if output.JSONOutput {
			output.PrintJSON(entries)
			return nil
		}

		if len(entries) == 0 {
			fmt.Println("No contexts defined")
			return nil
		}

		// Sort by name for consistent display (already sorted on disk, but
		// be defensive about in-memory re-reads).
		sorted := make([]cdp.ContextEntry, len(entries))
		copy(sorted, entries)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

		table := output.NewTable("Name", "Port", "PID", "Profile")
		for _, e := range sorted {
			alive := ""
			if cdp.PortAlive(e.Port) {
				alive = " (alive)"
			}
			table.AddRow(e.Name, fmt.Sprintf("%d%s", e.Port, alive), fmt.Sprintf("%d", e.PID), e.ProfileDir)
		}
		fmt.Print(table.String())
		return nil
	},
}

func init() {
	contextCmd.AddCommand(contextListCmd)
	rootCmd.AddCommand(contextCmd)
}
