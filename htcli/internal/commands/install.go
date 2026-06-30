package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

const hostName = "com.howtorecorder.host"

type chromeManifest struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Path           string   `json:"path"`
	Type           string   `json:"type"`
	AllowedOrigins []string `json:"allowed_origins"`
}

type firefoxManifest struct {
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	Path               string   `json:"path"`
	Type               string   `json:"type"`
	AllowedExtensions  []string `json:"allowed_extensions"`
}

var (
	installExtensionID string
	installUninstall   bool
	installBrowser     string
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Register htcli as a Native Messaging host for Chrome or Firefox",
	RunE: func(cmd *cobra.Command, args []string) error {
		browser := strings.ToLower(installBrowser)
		if browser != "chrome" && browser != "firefox" {
			return fmt.Errorf("--browser must be 'chrome' or 'firefox'")
		}

		manifestDir, err := nativeMessagingDir(browser)
		if err != nil {
			return err
		}
		manifestPath := filepath.Join(manifestDir, hostName+".json")

		if installUninstall {
			if err := os.Remove(manifestPath); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove manifest: %w", err)
			}
			fmt.Printf("Removed: %s\n", manifestPath)
			return nil
		}

		if installExtensionID == "" {
			if browser == "chrome" {
				return fmt.Errorf("--extension-id is required\n  Find it at chrome://extensions → Details → Extension ID")
			}
			return fmt.Errorf("--extension-id is required\n  Use the extension's ID from about:debugging#/runtime/this-firefox")
		}

		htcliPath, err := exec.LookPath("htcli")
		if err != nil {
			return fmt.Errorf("htcli not found in PATH: %w", err)
		}
		htcliPath, _ = filepath.Abs(htcliPath)

		if err := os.MkdirAll(manifestDir, 0755); err != nil {
			return fmt.Errorf("create manifest dir: %w", err)
		}

		var data []byte
		if browser == "firefox" {
			id := installExtensionID
			manifest := firefoxManifest{
				Name:              hostName,
				Description:       "How-To Recorder native messaging host",
				Path:              htcliPath,
				Type:              "stdio",
				AllowedExtensions: []string{id},
			}
			data, _ = json.MarshalIndent(manifest, "", "  ")
		} else {
			id := "chrome-extension://" + strings.TrimPrefix(installExtensionID, "chrome-extension://") + "/"
			manifest := chromeManifest{
				Name:           hostName,
				Description:    "How-To Recorder native messaging host",
				Path:           htcliPath,
				Type:           "stdio",
				AllowedOrigins: []string{id},
			}
			data, _ = json.MarshalIndent(manifest, "", "  ")
		}

		if err := os.WriteFile(manifestPath, data, 0644); err != nil {
			return fmt.Errorf("write manifest: %w", err)
		}

		fmt.Printf("Manifest written: %s\n", manifestPath)
		fmt.Printf("htcli path:       %s\n", htcliPath)
		fmt.Printf("Extension ID:     %s\n", installExtensionID)
		if browser == "chrome" {
			fmt.Println("\nReload the extension in Chrome (chrome://extensions → reload button).")
		} else {
			fmt.Println("\nReload the extension in Firefox (about:debugging → Reload).")
		}
		return nil
	},
}

func nativeMessagingDir(browser string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch runtime.GOOS {
	case "darwin":
		if browser == "firefox" {
			return filepath.Join(home, "Library", "Application Support", "Mozilla", "NativeMessagingHosts"), nil
		}
		return filepath.Join(home, "Library", "Application Support", "Google", "Chrome", "NativeMessagingHosts"), nil
	case "linux":
		if browser == "firefox" {
			return filepath.Join(home, ".mozilla", "native-messaging-hosts"), nil
		}
		return filepath.Join(home, ".config", "google-chrome", "NativeMessagingHosts"), nil
	default:
		return "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func init() {
	installCmd.Flags().StringVar(&installExtensionID, "extension-id", "", "Extension ID (Chrome: from chrome://extensions; Firefox: e.g. how-to-recorder@stevenstaylor.dev)")
	installCmd.Flags().BoolVar(&installUninstall, "uninstall", false, "Remove the native host manifest")
	installCmd.Flags().StringVar(&installBrowser, "browser", "chrome", "Target browser: chrome or firefox")
	rootCmd.AddCommand(installCmd)
}
