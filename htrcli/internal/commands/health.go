package commands

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/u007/htrcli/internal/output"
)

var healthCmd = &cobra.Command{
	Use:   "health",
	Short: "Check server connection",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := GetClient()
		health, err := c.GetHealth()
		if err != nil {
			return err
		}

		if output.JSONOutput {
			output.PrintJSON(health)
			return nil
		}

		fmt.Printf("Server: %s\n", output.Success("running"))
		fmt.Printf("Connected tabs: %d\n", health.ConnectedTabs)
		fmt.Printf("Uptime: %s\n", formatUptime(health.Uptime))
		return nil
	},
}

func formatUptime(seconds float64) string {
	h := int(seconds) / 3600
	m := (int(seconds) % 3600) / 60
	s := int(seconds) % 60

	if h > 0 {
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

func init() {
	rootCmd.AddCommand(healthCmd)
}
