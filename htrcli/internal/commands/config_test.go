package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

func TestExtensionIDsRoundTripsBothShapes(t *testing.T) {
	cases := []struct{ name, in string }{
		{"default string", `"abc123"`},
		{"per-browser object", `{"chrome":"c1","firefox":"f1"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var cfg configData
			if err := json.Unmarshal([]byte(`{"extension-id":`+tc.in+`}`), &cfg); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			out, err := json.Marshal(cfg)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var raw map[string]json.RawMessage
			if err := json.Unmarshal(out, &raw); err != nil {
				t.Fatalf("decode output: %v", err)
			}
			if got := string(raw["extension-id"]); got != tc.in {
				t.Fatalf("extension-id = %s, want %s (shape must round-trip)", got, tc.in)
			}
		})
	}
}

func TestExtensionIDsRejectsOtherShapes(t *testing.T) {
	for _, in := range []string{`5`, `true`, `null`, `["abc"]`, `{"":"x"}`, `{"chrome":5}`} {
		var cfg configData
		if err := json.Unmarshal([]byte(`{"extension-id":`+in+`}`), &cfg); err == nil {
			t.Errorf("extension-id %s must be an error, not silently accepted", in)
		}
	}
}

// TestSetTokenKeepsDefaultExtensionID is the regression for a silent data loss:
// set-token used to rewrite config.json without the plain-string extension-id.
func TestSetTokenKeepsDefaultExtensionID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	viper.Reset()
	// The --browser flag value persists on the shared command across tests; an
	// earlier per-browser test would otherwise make this write an object.
	setExtensionIDBrowser = ""

	rootCmd.SetArgs([]string{"config", "set-extension-id", "abc123def456"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("set-extension-id: %v", err)
	}
	rootCmd.SetArgs([]string{"config", "set-token", "tok1234"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("set-token: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(home, ".htrcli", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["extension-id"] != "abc123def456" || raw["token"] != "tok1234" {
		t.Fatalf("set-token dropped or changed a key: %s", data)
	}
}

func TestConfigWritesRefuseUnparsableFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configFile := filepath.Join(home, ".htrcli", "config.json")
	if err := os.MkdirAll(filepath.Dir(configFile), 0700); err != nil {
		t.Fatal(err)
	}
	broken := []byte("{not json")
	if err := os.WriteFile(configFile, broken, 0600); err != nil {
		t.Fatal(err)
	}

	writes := map[string]func() error{
		"set-server":      func() error { return setConfigValue("server", "https://x") },
		"set-chrome-args": func() error { return setConfigChromeArgs([]string{"--no-sandbox"}) },
	}
	for name, write := range writes {
		err := write()
		if err == nil || !strings.Contains(err.Error(), "failed to parse") {
			t.Fatalf("%s: err = %v, want a parse error", name, err)
		}
		if got, _ := os.ReadFile(configFile); string(got) != string(broken) {
			t.Fatalf("%s overwrote an unparsable config: %q", name, got)
		}
	}
}

func TestConfigInvalidValueLeavesFileUntouched(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configFile := filepath.Join(home, ".htrcli", "config.json")
	if err := os.MkdirAll(filepath.Dir(configFile), 0700); err != nil {
		t.Fatal(err)
	}
	valid := []byte(`{"token": "keep-me"}`)
	if err := os.WriteFile(configFile, valid, 0600); err != nil {
		t.Fatal(err)
	}

	for _, bad := range []func() error{
		func() error { return setConfigValue("cdp-port", "0") },
		func() error { return setConfigValue("no-such-key", "x") },
	} {
		if err := bad(); err == nil {
			t.Fatal("want an error for an invalid config write")
		}
		if got, _ := os.ReadFile(configFile); string(got) != string(valid) {
			t.Fatalf("a failed write changed config.json: %q", got)
		}
	}
}
