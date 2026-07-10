package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/u007/htcli/internal/output"
)

type configData struct {
	Server       string `json:"server"`
	Token        string `json:"token"`
	AMOAPIKey    string `json:"amo-api-key"`
	AMOAPISecret string `json:"amo-api-secret"`
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage CLI configuration",
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := configData{
			Server:       viper.GetString("server"),
			Token:        viper.GetString("token"),
			AMOAPIKey:    viper.GetString("amo-api-key"),
			AMOAPISecret: viper.GetString("amo-api-secret"),
		}
		if cfg.Server == "" {
			cfg.Server = "http://127.0.0.1:3845"
		}

		if output.JSONOutput {
			output.PrintJSON(cfg)
			return nil
		}

		fmt.Printf("Server: %s\n", cfg.Server)
		if cfg.Token != "" {
			fmt.Printf("Token:  %s...%s\n", cfg.Token[:4], cfg.Token[len(cfg.Token)-4:])
		} else {
			fmt.Println("Token:  (not set)")
		}
		printMasked("AMO API Key", cfg.AMOAPIKey)
		printMasked("AMO API Secret", cfg.AMOAPISecret)
		return nil
	},
}

var configSetServerCmd = &cobra.Command{
	Use:   "set-server <url>",
	Short: "Set server URL",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return setConfigValue("server", args[0])
	},
}

var configSetTokenCmd = &cobra.Command{
	Use:   "set-token <token>",
	Short: "Set bearer token",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return setConfigValue("token", args[0])
	},
}

func setConfigValue(key, value string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	configDir := filepath.Join(home, ".htcli")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	configFile := filepath.Join(configDir, "config.json")

	// Read existing config.
	var cfg configData
	if data, err := os.ReadFile(configFile); err == nil {
		_ = json.Unmarshal(data, &cfg)
	}

	// Update value.
	switch key {
	case "server":
		cfg.Server = value
	case "token":
		cfg.Token = value
	case "amo-api-key":
		cfg.AMOAPIKey = value
	case "amo-api-secret":
		cfg.AMOAPISecret = value
	}

	// Write config.
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	// Also update viper.
	viper.Set(key, value)

	fmt.Printf("Set %s to %s\n", key, value)
	return nil
}

func printMasked(label, value string) {
	if value != "" {
		n := min(4, len(value))
		fmt.Printf("%s: %s...%s\n", label, value[:n], value[len(value)-n:])
	} else {
		fmt.Printf("%s: (not set)\n", label)
	}
}

var configSetAMOKeyCmd = &cobra.Command{
	Use:   "set-amo-api-key <key>",
	Short: "Set AMO (addons.mozilla.org) API key",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return setConfigValue("amo-api-key", args[0])
	},
}

var configSetAMOSecretCmd = &cobra.Command{
	Use:   "set-amo-api-secret <secret>",
	Short: "Set AMO (addons.mozilla.org) API secret",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return setConfigValue("amo-api-secret", args[0])
	},
}

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSetServerCmd)
	configCmd.AddCommand(configSetTokenCmd)
	configCmd.AddCommand(configSetAMOKeyCmd)
	configCmd.AddCommand(configSetAMOSecretCmd)
	rootCmd.AddCommand(configCmd)
}
