package commands

import (
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/u007/htrcli/internal/output"
)

type configData struct {
	Server       string            `json:"server"`
	Token        string            `json:"token"`
	AMOAPIKey    string            `json:"amo-api-key"`
	AMOAPISecret string            `json:"amo-api-secret"`
	Transport    string            `json:"transport,omitempty"`
	CDPPort      int               `json:"cdp-port,omitempty"`
	ChromePath   string            `json:"chrome-path,omitempty"`
	ExtensionID  map[string]string `json:"extension-id,omitempty"`
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
			Transport:    viper.GetString("transport"),
			CDPPort:      viper.GetInt("cdp-port"),
			ChromePath:   viper.GetString("chrome-path"),
			ExtensionID:  viper.GetStringMapString("extension-id"),
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
		fmt.Printf("Transport: %s\n", cmp.Or(cfg.Transport, "ext"))
		fmt.Printf("CDP port: %d\n", cmp.Or(cfg.CDPPort, 9222))
		fmt.Printf("Chrome path: %s\n", cmp.Or(cfg.ChromePath, "(autodetect)"))
		if extID := viper.GetString("extension-id"); extID != "" {
			fmt.Printf("Extension ID: %s\n", extID)
		} else if len(cfg.ExtensionID) > 0 {
			for k, v := range cfg.ExtensionID {
				fmt.Printf("Extension ID (%s): %s\n", k, v)
			}
		}
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

	configDir := filepath.Join(home, ".htrcli")
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
	case "transport":
		cfg.Transport = value
	case "cdp-port":
		p, err := strconv.Atoi(value)
		if err != nil || p < 1 || p > 65535 {
			return fmt.Errorf("cdp-port must be a port number, got %q", value)
		}
		cfg.CDPPort = p
		viper.Set(key, p)
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal config: %w", err)
		}
		if err := os.WriteFile(configFile, data, 0600); err != nil {
			return fmt.Errorf("failed to write config: %w", err)
		}
		fmt.Printf("Set %s to %d\n", key, p)
		return nil
	case "chrome-path":
		cfg.ChromePath = value
	default:
		return fmt.Errorf("unknown config key %q", key)
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

func validateTransport(v string) error {
	if v != "ext" && v != "cdp" {
		return fmt.Errorf("transport must be \"ext\" or \"cdp\", got %q", v)
	}
	return nil
}

var configSetTransportCmd = &cobra.Command{
	Use:   "set-transport <ext|cdp>",
	Short: "Set default transport (ext = extension, cdp = direct CDP)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateTransport(args[0]); err != nil {
			return err
		}
		return setConfigValue("transport", args[0])
	},
}

var configSetCDPPortCmd = &cobra.Command{
	Use:   "set-cdp-port <port>",
	Short: "Set CDP debugging port (default 9222)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return setConfigValue("cdp-port", args[0])
	},
}

var configSetChromePathCmd = &cobra.Command{
	Use:   "set-chrome-path <path>",
	Short: "Set Chrome binary path for htrcli browser start",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return setConfigValue("chrome-path", args[0])
	},
}

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSetServerCmd)
	configCmd.AddCommand(configSetTokenCmd)
	configCmd.AddCommand(configSetAMOKeyCmd)
	configCmd.AddCommand(configSetAMOSecretCmd)
	configCmd.AddCommand(configSetTransportCmd)
	configCmd.AddCommand(configSetCDPPortCmd)
	configCmd.AddCommand(configSetChromePathCmd)
	configCmd.AddCommand(setExtensionIDCmd)
	setExtensionIDCmd.Flags().StringVar(&setExtensionIDBrowser, "browser", "", "Browser this ID applies to (chrome, firefox). Empty = default for all.")
	rootCmd.AddCommand(configCmd)
}

var setExtensionIDBrowser string

var setExtensionIDCmd = &cobra.Command{
	Use:   "set-extension-id <id>",
	Short: "Set the browser extension ID used by 'htrcli install' (and the tray's 'Reinstall native host' menu)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		configDir := filepath.Join(home, ".htrcli")
		if err := os.MkdirAll(configDir, 0700); err != nil {
			return fmt.Errorf("failed to create config directory: %w", err)
		}
		configFile := filepath.Join(configDir, "config.json")

		// Read the existing config as a raw map so we never drop keys
		// owned by other subcommands (e.g. token, server).
		raw := map[string]any{}
		if data, rerr := os.ReadFile(configFile); rerr == nil {
			_ = json.Unmarshal(data, &raw)
		}

		extID := map[string]string{}
		if existing, ok := raw["extension-id"].(map[string]any); ok {
			for k, v := range existing {
				if s, ok := v.(string); ok {
					extID[k] = s
				}
			}
		}

		if setExtensionIDBrowser == "" {
			// Default ID applies to all browsers: stored as a plain string.
			raw["extension-id"] = id
			viper.Set("extension-id", id)
		} else {
			extID[setExtensionIDBrowser] = id
			raw["extension-id"] = extID
			viper.Set("extension-id", extID)
			// Also set the dotted key so `viper.GetString("extension-id.<browser>")`
			// resolves directly.
			viper.Set("extension-id."+setExtensionIDBrowser, id)
		}

		out, merr := json.MarshalIndent(raw, "", "  ")
		if merr != nil {
			return fmt.Errorf("failed to marshal config: %w", merr)
		}
		if werr := os.WriteFile(configFile, out, 0600); werr != nil {
			return fmt.Errorf("failed to write config: %w", werr)
		}

		which := setExtensionIDBrowser
		if which == "" {
			which = "all"
		}
		fmt.Printf("Set extension-id for %s to %s\n", which, id)
		return nil
	},
}
