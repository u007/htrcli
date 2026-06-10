package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/u007/htcli/internal/api"
	"github.com/u007/htcli/internal/output"
)

var (
	cfgFile     string
	serverURL   string
	token       string
	jsonOutput  bool
	tabID       int
	timeout     int
	client      *api.Client
)

var rootCmd = &cobra.Command{
	Use:   "htcli",
	Short: "How-To Recorder CLI - control browser tabs remotely",
	Long: `htcli is a CLI for controlling browser tabs via the How-To Recorder
remote control API. Requires the How-To Recorder Chrome extension and
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
	rootCmd.PersistentFlags().IntVar(&tabID, "tab", 0, "target specific tab ID")
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

// GetTabID returns the target tab ID, or nil if not set.
func GetTabID() *int {
	if tabID > 0 {
		return &tabID
	}
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
