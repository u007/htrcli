package commands

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestRunInteractCDPFailsClearlyWhenNotRunning(t *testing.T) {
	resetTransportState()
	cdpFlag = true
	defer resetTransportState()
	// Point at a dead port so PageSession fails fast.
	setViperCDPPort(t, 1)

	err := runInteractCDP("fill", "#email", "x")
	if err == nil || !strings.Contains(err.Error(), "htcli browser start") {
		t.Fatalf("want ErrNotRunning guidance, got %v", err)
	}
}

func setViperCDPPort(t *testing.T, port int) {
	t.Helper()
	viper.Set("cdp-port", port)
	t.Cleanup(func() { viper.Set("cdp-port", 0) })
}
