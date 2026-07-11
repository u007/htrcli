package commands

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/u007/htrcli/internal/cdp"
	"github.com/u007/htrcli/internal/output"
)

var browserHeadless bool

var browserCmd = &cobra.Command{
	Use:   "browser",
	Short: "Manage the CDP-controlled Chrome instance",
}

var browserStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Launch Chrome with remote debugging (dedicated profile)",
	RunE: func(cmd *cobra.Command, args []string) error {
		chrome, err := cdp.FindChrome(GetChromePath())
		if err != nil {
			return err
		}
		st, err := cdp.StartBrowser(chrome, GetCDPPort(), browserHeadless)
		if err != nil {
			return err
		}
		if output.JSONOutput {
			output.PrintJSON(st)
			return nil
		}
		mode := "visible"
		if st.Headless {
			mode = "headless"
		}
		fmt.Printf("Browser running: pid %d, port %d (%s)\n", st.PID, st.Port, mode)
		return nil
	},
}

var browserStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the CDP-controlled Chrome",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := cdp.StopBrowser(); err != nil {
			return err
		}
		fmt.Println("Browser stopped")
		return nil
	},
}

var browserStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show CDP browser status (probes the port, not the PID file)",
	RunE: func(cmd *cobra.Command, args []string) error {
		port := GetCDPPort()
		alive := cdp.PortAlive(port)
		st, err := cdp.ReadState()
		if err != nil {
			return err
		}
		if output.JSONOutput {
			output.PrintJSON(map[string]any{"running": alive, "port": port, "state": st})
			return nil
		}
		if !alive {
			fmt.Printf("Browser: not running (port %d)\n", port)
			return nil
		}
		fmt.Printf("Browser: running on port %d\n", port)
		if st != nil {
			mode := "visible"
			if st.Headless {
				mode = "headless"
			}
			fmt.Printf("PID: %d (%s), started %s\n", st.PID, mode, st.StartedAt.Format("15:04:05"))
		}
		if st != nil && !st.Headless {
			if ws, err := cdp.GetWindowState(port, ""); err == nil {
				fmt.Printf("Window: %s\n", ws)
			} else {
				fmt.Printf("Window: unknown (%v)\n", err)
			}
		}
		return nil
	},
}

var browserHideCmd = &cobra.Command{
	Use:   "hide",
	Short: "Minimize the CDP browser window (not applicable to headless)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := cdp.SetWindowState(GetCDPPort(), GetTabTarget(), "minimized"); err != nil {
			return err
		}
		fmt.Println("Browser hidden (minimized)")
		return nil
	},
}

var browserShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Restore the CDP browser window",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := cdp.SetWindowState(GetCDPPort(), GetTabTarget(), "normal"); err != nil {
			return err
		}
		fmt.Println("Browser shown")
		return nil
	},
}

func init() {
	browserStartCmd.Flags().BoolVar(&browserHeadless, "headless", false, "run without a window (sign in visible first — see GUIDE)")
	browserCmd.AddCommand(browserStartCmd)
	browserCmd.AddCommand(browserStopCmd)
	browserCmd.AddCommand(browserStatusCmd)
	browserCmd.AddCommand(browserHideCmd)
	browserCmd.AddCommand(browserShowCmd)
	rootCmd.AddCommand(browserCmd)
}
