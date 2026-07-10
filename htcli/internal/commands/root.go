package commands

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/u007/htcli/internal/api"
	"github.com/u007/htcli/internal/output"
)

var (
	cfgFile       string
	serverURL     string
	token         string
	jsonOutput    bool
	tabTarget     string
	transportFlag string
	cdpFlag       bool
	timeout       int
	client        *api.Client
)

var rootCmd = &cobra.Command{
	Use:   "htcli",
	Short: "HTR NControl CLI - control browser tabs remotely",
	Long: `htcli is a CLI for controlling browser tabs via the HTR NControl
remote control API. Requires the HTR NControl Chrome extension and
server running.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		output.JSONOutput = jsonOutput
		initClient()
	},
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ~/.htcli/config.json)")
	rootCmd.PersistentFlags().StringVar(&serverURL, "server", "", "server URL (overrides config)")
	rootCmd.PersistentFlags().StringVar(&token, "token", "", "bearer token (overrides config)")
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "output raw JSON")
	rootCmd.PersistentFlags().StringVar(&tabTarget, "tab", "", "target tab: numeric ID (extension) or CDP target ID (--cdp)")
	rootCmd.PersistentFlags().StringVar(&transportFlag, "transport", "", "transport: ext (extension, default) or cdp")
	rootCmd.PersistentFlags().BoolVar(&cdpFlag, "cdp", false, "shorthand for --transport cdp")
	rootCmd.PersistentFlags().IntVar(&timeout, "timeout", 30000, "command timeout in ms")
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting home dir: %v\n", err)
			os.Exit(1)
		}

		viper.AddConfigPath(home + "/.htcli")
		viper.SetConfigName("config")
		viper.SetConfigType("json")
	}

	viper.AutomaticEnv()
	viper.SetEnvPrefix("HTCLI")
	// Dashed config keys (cdp-port, chrome-path) map to underscored env vars
	// (HTCLI_CDP_PORT, HTCLI_CHROME_PATH) — dashes can't appear in shell
	// variable names.
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	_ = viper.ReadInConfig()
}

func initClient() {
	// Priority: flags > env > config file > defaults
	srv := serverURL
	if srv == "" {
		srv = viper.GetString("server")
	}
	if srv == "" {
		srv = "http://127.0.0.1:3845"
	}

	tkn := token
	if tkn == "" {
		tkn = viper.GetString("token")
	}

	client = api.NewClient(srv, tkn)
}

// GetClient returns the initialized API client.
func GetClient() *api.Client {
	if client == nil {
		initClient()
	}
	return client
}

// GetTabID returns the numeric extension tab ID, nil if unset.
// Errors when --tab is non-numeric (that form is CDP-only).
func GetTabID() (*int, error) {
	if tabTarget == "" {
		return nil, nil
	}
	id, err := strconv.Atoi(tabTarget)
	if err != nil || id <= 0 {
		return nil, fmt.Errorf("--tab %q is not a numeric tab ID (CDP target IDs require --cdp)", tabTarget)
	}
	return &id, nil
}

// GetTabTarget returns the raw --tab value for the CDP transport ("" = unset).
func GetTabTarget() string {
	return tabTarget
}

// UseCDP resolves the transport: flags > config/env > default ext.
func UseCDP() bool {
	if transportFlag != "" {
		return transportFlag == "cdp"
	}
	if cdpFlag {
		return true
	}
	return viper.GetString("transport") == "cdp"
}

// GetCDPPort returns the CDP debugging port (flags none; env/config; default 9222).
func GetCDPPort() int {
	if p := viper.GetInt("cdp-port"); p > 0 {
		return p
	}
	return 9222
}

// GetChromePath returns the configured Chrome binary path ("" = autodetect).
func GetChromePath() string {
	return viper.GetString("chrome-path")
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		output.PrintJSON(map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		os.Exit(1)
	}
}
