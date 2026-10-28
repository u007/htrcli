package commands

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/u007/htrcli/internal/cdp"
	"github.com/u007/htrcli/internal/output"
)

var (
	browserHeadless   bool
	browserChromeArgs []string
)

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
		if err := ensureContextResolved(); err != nil {
			return err
		}
		port := GetCDPPort()
		st, err := cdp.StartBrowser(chrome, port, browserHeadless, slices.Concat(GetChromeArgs(), browserChromeArgs))
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
		if err := ensureContextResolved(); err != nil {
			return err
		}
		port := GetCDPPort()
		alive := cdp.PortAlive(port)
		st, err := cdp.ReadState()
		if err != nil {
			return err
		}
		logPath, err := stderrLogForStatus(st)
		if err != nil {
			return err
		}
		if output.JSONOutput {
			output.PrintJSON(map[string]any{"running": alive, "port": port, "state": st, "stderr_log": logPath})
			return nil
		}
		logText := logPath
		if logPath == "" {
			logText = "not recorded (Chrome was adopted or started before stderr_log existed)"
		}
		if !alive {
			fmt.Printf("Browser: not running (port %d)\n", port)
			fmt.Printf("Chrome stderr log: %s\n", logText)
			return nil
		}
		fmt.Printf("Browser: running on port %d\n", port)
		if st != nil {
			mode := "visible"
			if st.Headless {
				mode = "headless"
			}
			fmt.Printf("PID: %d (%s), started %s\n", st.PID, mode, st.StartedAt.Format("15:04:05"))
			if len(st.Args) > 0 {
				fmt.Printf("Args: %s\n", strings.Join(st.Args, " "))
			}
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

// stderrLogForStatus returns the Chrome stderr log status reports: the named
// context's when --context is set, else the default browser's. With no state
// file it is the log the default profile would write. It is "" when the state
// records no log.
func stderrLogForStatus(st *cdp.BrowserState) (string, error) {
	if contextName != "" {
		entry, err := cdp.FindContext(contextName)
		if err != nil {
			return "", err
		}
		if entry == nil {
			return "", fmt.Errorf("context %s is not registered", contextName)
		}
		return entry.StderrLog, nil
	}
	if st != nil {
		return st.StderrLog, nil
	}
	profile, err := cdp.ProfileDir()
	if err != nil {
		return "", err
	}
	return cdp.ChromeStderrLogPath(profile), nil
}

var browserHideCmd = &cobra.Command{
	Use:   "hide",
	Short: "Minimize the CDP browser window (not applicable to headless)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ensureContextResolved(); err != nil {
			return err
		}
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
		if err := ensureContextResolved(); err != nil {
			return err
		}
		if err := cdp.SetWindowState(GetCDPPort(), GetTabTarget(), "normal"); err != nil {
			return err
		}
		fmt.Println("Browser shown")
		return nil
	},
}

func init() {
	browserStartCmd.Flags().BoolVar(&browserHeadless, "headless", false, "run without a window (sign in visible first — see GUIDE)")
	browserStartCmd.Flags().StringArrayVar(&browserChromeArgs, "chrome-arg", nil, "extra Chrome command-line argument, repeatable (appended after config chrome-args)")
	browserCmd.AddCommand(browserStartCmd)
	browserCmd.AddCommand(browserStopCmd)
	browserCmd.AddCommand(browserStatusCmd)
	browserCmd.AddCommand(browserHideCmd)
	browserCmd.AddCommand(browserShowCmd)
	rootCmd.AddCommand(browserCmd)
}
