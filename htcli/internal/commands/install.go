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

type nativeHostManifest struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Path           string   `json:"path"`
	Type           string   `json:"type"`
	AllowedOrigins []string `json:"allowed_origins"`
}

var (
	installExtensionID string
	installUninstall   bool
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Register htcli as a Chrome Native Messaging host",
	RunE: func(cmd *cobra.Command, args []string) error {
		manifestDir, err := nativeMessagingDir()
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
			return fmt.Errorf("--extension-id is required\n  Find it at chrome://extensions \u2192 Details \u2192 Extension ID")
		}

		htcliPath, err := exec.LookPath("htcli")
		if err != nil {
			return fmt.Errorf("htcli not found in PATH: %w", err)
		}
		htcliPath, _ = filepath.Abs(htcliPath)

		manifest := nativeHostManifest{
			Name:        hostName,
			Description: "How-To Recorder native messaging host",
			Path:        htcliPath,
			Type:        "stdio",
			AllowedOrigins: []string{
				"chrome-extension://" + strings.TrimPrefix(installExtensionID, "chrome-extension://") + "/",
			},
		}

		if err := os.MkdirAll(manifestDir, 0755); err != nil {
			return fmt.Errorf("create manifest dir: %w", err)
		}

		data, _ := json.MarshalIndent(manifest, "", "  ")
		if err := os.WriteFile(manifestPath, data, 0644); err != nil {
			return fmt.Errorf("write manifest: %w", err)
		}

		fmt.Printf("Manifest written: %s\n", manifestPath)
		fmt.Printf("htcli path:       %s\n", htcliPath)
		fmt.Printf("Extension ID:     %s\n", installExtensionID)
		fmt.Println("\nReload the extension in Chrome (chrome://extensions \u2192 reload button).")
		return nil
	},
}

func nativeMessagingDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Google", "Chrome", "NativeMessagingHosts"), nil
	case "linux":
		return filepath.Join(home, ".config", "google-chrome", "NativeMessagingHosts"), nil
	default:
		return "", fmt.Errorf("unsupported OS for automatic install: %s\n  See: https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging", runtime.GOOS)
	}
}

func init() {
	installCmd.Flags().StringVar(&installExtensionID, "extension-id", "", "Chrome extension ID (from chrome://extensions)")
	installCmd.Flags().BoolVar(&installUninstall, "uninstall", false, "Remove the native host manifest")
	rootCmd.AddCommand(installCmd)
}
