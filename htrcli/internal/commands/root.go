package commands

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/u007/htrcli/internal/api"
	"github.com/u007/htrcli/internal/cdp"
	"github.com/u007/htrcli/internal/output"
)

var (
	cfgFile          string
	serverURL        string
	token            string
	jsonOutput       bool
	tabTarget        string
	transportFlag    string
	cdpFlag          bool
	contextName      string
	timeout          int
	client           *api.Client
	resolveContextFn = resolveContext
)

var rootCmd = &cobra.Command{
	Use:   "htrcli",
	Short: "HTR NControl CLI - control browser tabs remotely",
	Long: `htrcli is a CLI for controlling browser tabs via the HTR NControl
remote control API. Requires the HTR NControl Chrome extension and
server running.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		output.JSONOutput = jsonOutput
		initClient()
		return nil
	},
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ~/.htrcli/config.json)")
	rootCmd.PersistentFlags().StringVar(&serverURL, "server", "", "server URL (overrides config)")
	rootCmd.PersistentFlags().StringVar(&token, "token", "", "bearer token (overrides config)")
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "output raw JSON")
	rootCmd.PersistentFlags().StringVar(&tabTarget, "tab", "", "target tab: numeric ID (extension) or CDP target ID (--cdp)")
	rootCmd.PersistentFlags().StringVar(&transportFlag, "transport", "", "transport: ext (extension, default) or cdp")
	rootCmd.PersistentFlags().BoolVar(&cdpFlag, "cdp", false, "shorthand for --transport cdp")
	rootCmd.PersistentFlags().StringVar(&contextName, "context", "", "named browser context (isolated profile)")
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

		viper.AddConfigPath(home + "/.htrcli")
		viper.SetConfigName("config")
		viper.SetConfigType("json")
	}

	viper.AutomaticEnv()
	viper.SetEnvPrefix("HTRCLI")
	// Dashed config keys (cdp-port, chrome-path) map to underscored env vars
	// (HTRCLI_CDP_PORT, HTRCLI_CHROME_PATH) — dashes can't appear in shell
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

// contextCDPPort caches the resolved context port so GetCDPPort returns the
// same port for the duration of a single command invocation.
var contextCDPPort int

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

// GetCDPPort returns the CDP debugging port.
//   - When --context is set, returns the resolved context port (cached after resolveContext).
//   - When cdp-port is configured, returns that value.
//   - Default: 9222.
func GetCDPPort() int {
	if contextCDPPort > 0 {
		return contextCDPPort
	}
	if p := viper.GetInt("cdp-port"); p > 0 {
		return p
	}
	return 9222
}

// ensureContextResolved resolves a named context lazily the first time a CDP
// command actually needs the port. Non-CDP commands never call this helper, so
// `--context` stays inert for extension/HTTP-only verbs.
func ensureContextResolved() error {
	if contextName == "" || contextCDPPort > 0 {
		return nil
	}
	return resolveContextFn()
}

// GetChromePath returns the configured Chrome binary path ("" = autodetect).
func GetChromePath() string {
	return viper.GetString("chrome-path")
}

// resolveContext ensures the named context's Chrome process is running and
// caches its debugging port. Called from PersistentPreRunE when --context is set.
func resolveContext() error {
	chrome, err := cdp.FindChrome(GetChromePath())
	if err != nil {
		return fmt.Errorf("--context %s: %w", contextName, err)
	}
	port, err := cdp.EnsureContext(contextName, chrome, false)
	if err != nil {
		return fmt.Errorf("--context %s: %w", contextName, err)
	}
	contextCDPPort = port
	return nil
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
