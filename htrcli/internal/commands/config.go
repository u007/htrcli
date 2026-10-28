package commands

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/u007/htrcli/internal/output"
)

type configData struct {
	Server       string       `json:"server"`
	Token        string       `json:"token"`
	AMOAPIKey    string       `json:"amo-api-key"`
	AMOAPISecret string       `json:"amo-api-secret"`
	Transport    string       `json:"transport,omitempty"`
	CDPPort      int          `json:"cdp-port,omitempty"`
	ChromePath   string       `json:"chrome-path,omitempty"`
	ChromeArgs   []string     `json:"chrome-args,omitempty"`
	ExtensionID  extensionIDs `json:"extension-id,omitempty"`
}

// defaultExtensionIDKey is the extensionIDs key that holds the ID for every
// browser. No browser is named "", so it cannot clash with a per-browser ID.
const defaultExtensionIDKey = ""

// extensionIDs maps a browser to its extension ID. The all-browsers ID is stored
// as a plain JSON string (what set-extension-id writes by default) and per-browser
// IDs as an object. Each shape marshals back to the shape it was read from.
// Anything else is an error, so an unreadable value is never rewritten away.
type extensionIDs map[string]string

func (e extensionIDs) MarshalJSON() ([]byte, error) {
	if id, ok := e[defaultExtensionIDKey]; ok && len(e) == 1 {
		return json.Marshal(id)
	}
	return json.Marshal(map[string]string(e))
}

func (e *extensionIDs) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	switch {
	case strings.HasPrefix(trimmed, `"`):
		var id string
		if err := json.Unmarshal([]byte(trimmed), &id); err != nil {
			return fmt.Errorf("extension-id string: %w", err)
		}
		*e = extensionIDs{defaultExtensionIDKey: id}
		return nil
	case strings.HasPrefix(trimmed, "{"):
		var ids map[string]string
		if err := json.Unmarshal([]byte(trimmed), &ids); err != nil {
			return fmt.Errorf("extension-id object: %w", err)
		}
		if _, ok := ids[defaultExtensionIDKey]; ok {
			return errors.New(`extension-id object must not name a browser ""; set the all-browsers ID as a plain string`)
		}
		*e = ids
		return nil
	default:
		return fmt.Errorf("extension-id must be a string or an object of browser IDs, got %s", trimmed)
	}
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
			ChromeArgs:   viper.GetStringSlice("chrome-args"),
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
		fmt.Printf("Chrome args: %s\n", cmp.Or(strings.Join(cfg.ChromeArgs, " "), "(none)"))
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
	var stored any = value
	err := updateConfigFile(func(cfg *configData) error {
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
			stored = p
		case "chrome-path":
			cfg.ChromePath = value
		default:
			return fmt.Errorf("unknown config key %q", key)
		}
		return nil
	})
	if err != nil {
		return err
	}
	viper.Set(key, stored)
	fmt.Printf("Set %s to %v\n", key, stored)
	return nil
}

// updateConfigFile loads ~/.htrcli/config.json, applies mutate, and writes the
// result back. A missing file is the first-run case and starts from an empty
// config. A file that exists but does not parse is an error, so a broken config
// is never rewritten without the keys it could not read. When mutate fails,
// nothing is written.
func updateConfigFile(mutate func(*configData) error) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	configDir := filepath.Join(home, ".htrcli")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	configFile := filepath.Join(configDir, "config.json")

	var cfg configData
	data, err := os.ReadFile(configFile)
	switch {
	case err == nil:
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("failed to parse %s: %w", configFile, err)
		}
	case errors.Is(err, os.ErrNotExist):
		// intentionally not logged: no config file yet, the write below creates it
	default:
		return fmt.Errorf("failed to read %s: %w", configFile, err)
	}
	if err := mutate(&cfg); err != nil {
		return err
	}
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	if err := os.WriteFile(configFile, out, 0600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}
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

var configSetChromeArgsCmd = &cobra.Command{
	Use:   "set-chrome-args [arg...]",
	Short: "Set extra Chrome arguments for every htrcli-launched Chrome (no args clears them)",
	Long: `Set extra Chrome command-line arguments for every Chrome htrcli launches.
Arguments that start with a dash need the -- separator:
  htrcli config set-chrome-args -- --no-sandbox --disable-dev-shm-usage`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return setConfigChromeArgs(args)
	},
}

// setConfigChromeArgs replaces the chrome-args list in config.json. A nil list
// clears the key, which omitempty then drops from the file.
func setConfigChromeArgs(args []string) error {
	if err := updateConfigFile(func(cfg *configData) error {
		cfg.ChromeArgs = args
		return nil
	}); err != nil {
		return err
	}
	viper.Set("chrome-args", args)
	if len(args) == 0 {
		fmt.Println("Cleared chrome-args")
		return nil
	}
	fmt.Printf("Set chrome-args to: %s\n", strings.Join(args, " "))
	return nil
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
	configCmd.AddCommand(configSetChromeArgsCmd)
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
		var droppedDefault *string
		err := updateConfigFile(func(cfg *configData) error {
			if setExtensionIDBrowser == "" {
				// Default ID applies to all browsers: stored as a plain string.
				cfg.ExtensionID = extensionIDs{defaultExtensionIDKey: id}
				return nil
			}
			ids := extensionIDs{}
			for browser, existing := range cfg.ExtensionID {
				if browser != defaultExtensionIDKey {
					ids[browser] = existing
				}
			}
			if existing, ok := cfg.ExtensionID[defaultExtensionIDKey]; ok {
				droppedDefault = &existing
			}
			ids[setExtensionIDBrowser] = id
			cfg.ExtensionID = ids
			return nil
		})
		if err != nil {
			return err
		}
		if droppedDefault != nil {
			fmt.Fprintf(os.Stderr, "[htrcli] the all-browsers extension-id %s was dropped: per-browser IDs are stored as an object, which cannot also hold it. Run set-extension-id <id> again to restore it for the other browsers.\n", *droppedDefault)
		}
		if setExtensionIDBrowser == "" {
			viper.Set("extension-id", id)
		} else {
			// Also set the dotted key so `viper.GetString("extension-id.<browser>")`
			// resolves directly.
			viper.Set("extension-id."+setExtensionIDBrowser, id)
		}

		which := setExtensionIDBrowser
		if which == "" {
			which = "all"
		}
		fmt.Printf("Set extension-id for %s to %s\n", which, id)
		return nil
	},
}
