package commands

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/u007/htrcli/internal/cdp"
)

// cdp.ConsoleEntry must marshal to the extension's console data shape so the
// shared --level filter and formatter work unchanged.
func TestCDPConsoleEntryUsesExtensionShape(t *testing.T) {
	entry, err := cdpEventEntry(3, consoleEventKind, 1000, cdp.ConsoleEntry{
		Level: "warn", Args: []string{"careful", "1"}, Source: "http://h/:7", Timestamp: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !consoleEntryMatchesLevel(entry, "warn") || consoleEntryMatchesLevel(entry, "error") {
		t.Fatalf("level filter broken for %s", entry.Data)
	}
	if got, want := formatConsoleEvent(entry), "[seq 3] warn: careful 1 (http://h/:7)"; got != want {
		t.Fatalf("format = %q, want %q", got, want)
	}
	if strings.Contains(string(entry.Data), "Timestamp") || strings.Contains(string(entry.Data), "timestamp") {
		t.Fatalf("timestamp belongs on the envelope, not in data: %s", entry.Data)
	}
}

// cdp.NetworkEntry must satisfy the shared --url/--status matcher and render an
// unknown method (Performance API) as "—", not as a blank column.
func TestCDPNetworkEntryUsesExtensionShape(t *testing.T) {
	e := cdp.NetworkEntry{URL: "http://h/api/missing", Status: 404, DurationMs: 5, InitiatorType: "fetch", TransferSize: 300}
	entry, err := cdpEventEntry(2, networkEventKind, 1000, e)
	if err != nil {
		t.Fatal(err)
	}
	if !networkEntryMatches(entry, "*/api/missing", 404) || networkEntryMatches(entry, "*/api/missing", 200) {
		t.Fatalf("matcher broken for %s", entry.Data)
	}
	if got, want := formatCDPNetworkEvent(entry, e), "[seq 2] 404 — http://h/api/missing (5ms) [fetch, 300B]"; got != want {
		t.Fatalf("format = %q, want %q", got, want)
	}

	failed := cdp.NetworkEntry{URL: "http://h/x", Method: "GET", ResourceType: "Fetch", Error: "net::ERR_FAILED"}
	fe, err := cdpEventEntry(1, networkEventKind, 1, failed)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := formatCDPNetworkEvent(fe, failed), "[seq 1] — GET http://h/x (0ms) [Fetch] error: net::ERR_FAILED"; got != want {
		t.Fatalf("format = %q, want %q", got, want)
	}
}

func TestRejectSinceCDP(t *testing.T) {
	newCmd := func() *cobra.Command {
		c := &cobra.Command{Use: "x", RunE: func(*cobra.Command, []string) error { return nil }}
		var since int
		c.Flags().IntVar(&since, "since", 0, "")
		return c
	}
	c := newCmd()
	if err := c.ParseFlags(nil); err != nil {
		t.Fatal(err)
	}
	if err := rejectSinceCDP(c, "console read"); err != nil {
		t.Fatalf("unset --since must be accepted: %v", err)
	}
	c = newCmd()
	if err := c.ParseFlags([]string{"--since", "0"}); err != nil {
		t.Fatal(err)
	}
	err := rejectSinceCDP(c, "console read")
	if err == nil || !strings.Contains(err.Error(), "--since is not supported over --cdp") {
		t.Fatalf("explicit --since must fail loudly, got %v", err)
	}
}
