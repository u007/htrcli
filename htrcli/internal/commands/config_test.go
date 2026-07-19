package commands

import (
	"testing"

	"github.com/spf13/viper"
)

func TestSetExtensionID(t *testing.T) {
	// Use a temp HOME so we don't clobber the real config.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("XDG_CONFIG_HOME", tmpHome)
	viper.Reset()

	// Default browser = chrome. Drive through rootCmd so cobra parses the
	// command exactly as the real binary does.
	rootCmd.SetArgs([]string{"config", "set-extension-id", "abc123def456"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("set-extension-id: %v", err)
	}
	if got := viper.GetString("extension-id"); got != "abc123def456" {
		t.Fatalf("got %q", got)
	}
}

func TestSetExtensionIDPerBrowser(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("XDG_CONFIG_HOME", tmpHome)
	viper.Reset()

	rootCmd.SetArgs([]string{"config", "set-extension-id", "ff-id-xyz", "--browser=firefox"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("set-extension-id: %v", err)
	}
	if got := viper.GetString("extension-id.firefox"); got != "ff-id-xyz" {
		t.Fatalf("got %q", got)
	}
}

func resetTransportState() {
	transportFlag = ""
	cdpFlag = false
	viper.Set("transport", "")
	viper.Set("cdp-port", 0)
}

func TestUseCDPDefaultFalse(t *testing.T) {
	resetTransportState()
	if UseCDP() {
		t.Fatal("default transport must be extension")
	}
}

func TestUseCDPFlag(t *testing.T) {
	resetTransportState()
	cdpFlag = true
	defer resetTransportState()
	if !UseCDP() {
		t.Fatal("--cdp must enable CDP transport")
	}
}

func TestUseCDPConfigSticky(t *testing.T) {
	resetTransportState()
	viper.Set("transport", "cdp")
	defer resetTransportState()
	if !UseCDP() {
		t.Fatal("config transport=cdp must enable CDP")
	}
}

func TestFlagOverridesConfigBothDirections(t *testing.T) {
	resetTransportState()
	viper.Set("transport", "cdp")
	transportFlag = "ext"
	defer resetTransportState()
	if UseCDP() {
		t.Fatal("--transport ext must override config cdp")
	}
}

func TestGetCDPPortDefault(t *testing.T) {
	resetTransportState()
	if got := GetCDPPort(); got != 9222 {
		t.Fatalf("want 9222, got %d", got)
	}
}

func TestSetTransportRejectsInvalid(t *testing.T) {
	if err := validateTransport("chrome"); err == nil {
		t.Fatal("want error for invalid transport value")
	}
	if err := validateTransport("cdp"); err != nil {
		t.Fatalf("cdp must be valid: %v", err)
	}
	if err := validateTransport("ext"); err != nil {
		t.Fatalf("ext must be valid: %v", err)
	}
}
