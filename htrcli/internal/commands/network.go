package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/u007/htrcli/internal/api"
	"github.com/u007/htrcli/internal/output"
)

const networkEventKind = "network"

type networkEventData struct {
	RequestID  string `json:"requestId"`
	URL        string `json:"url"`
	Method     string `json:"method"`
	Status     int    `json:"status,omitempty"`
	DurationMs int    `json:"durationMs,omitempty"`
}

// networkEntryMatches reports whether an entry's url matches the glob (path.Match
// semantics, matched against the whole URL) and, when status > 0, its status
// equals that code.
func networkEntryMatches(entry api.EventEntry, urlGlob string, status int) bool {
	var data networkEventData
	if err := json.Unmarshal(entry.Data, &data); err != nil {
		return false
	}
	if status > 0 && data.Status != status {
		return false
	}
	if urlGlob == "" {
		return true
	}
	ok, err := path.Match(urlGlob, data.URL)
	if err != nil || !ok {
		// path.Match does not span '/'; also try a contains-style match so
		// "*/api/users*" behaves intuitively against full URLs.
		return globContains(urlGlob, data.URL)
	}
	return true
}

// globContains implements a simple '*'-wildcard containment match that, unlike
// path.Match, treats '*' as spanning any character including '/'.
func globContains(pattern, s string) bool {
	parts := strings.Split(pattern, "*")
	pos := 0
	for i, part := range parts {
		if part == "" {
			continue
		}
		idx := strings.Index(s[pos:], part)
		if idx < 0 {
			return false
		}
		if i == 0 && !strings.HasPrefix(pattern, "*") && idx != 0 {
			return false
		}
		pos += idx + len(part)
	}
	if !strings.HasSuffix(pattern, "*") && len(parts) > 0 {
		last := parts[len(parts)-1]
		return strings.HasSuffix(s, last)
	}
	return true
}

func formatNetworkEvent(entry api.EventEntry) string {
	var data networkEventData
	if err := json.Unmarshal(entry.Data, &data); err != nil {
		return fmt.Sprintf("[seq %d] <unparseable network entry>", entry.Seq)
	}
	status := "—"
	if data.Status > 0 {
		status = fmt.Sprintf("%d", data.Status)
	}
	return fmt.Sprintf("[seq %d] %s %s %s (%dms)", entry.Seq, status, data.Method, data.URL, data.DurationMs)
}

func formatNetworkEntries(resp *api.EventsResponse) string {
	if resp == nil {
		return ""
	}
	var b strings.Builder
	if resp.Dropped > 0 {
		fmt.Fprintf(&b, "%s %d events were evicted (buffer cap reached)\n", output.Warning("⚠"), resp.Dropped)
	}
	for _, entry := range resp.Entries {
		b.WriteString(formatNetworkEvent(entry))
		b.WriteByte('\n')
	}
	return b.String()
}

// armNetworkCapture asks the extension to open a bounded Chrome capture window
// (a no-op ack on Firefox, where webRequest capture is always-on).
func armNetworkCapture(tabID *int, durationMs int) error {
	_, err := GetClient().ExecuteCommand(tabID, api.Command{
		ID:      "1",
		Action:  "networkCapture",
		Options: map[string]any{"durationMs": durationMs},
	})
	return err
}

var (
	networkSince      int
	networkTimeoutMS  int
	networkWaitURL    string
	networkWaitStatus int
)

// networkCmd is the canonical `network` command group. This plan owns it
// (per team arbitration: passive capture is Phase 2, ahead of the Phase 3
// mock plan). Other network subcommands (network mock/block/unmock, added by
// 2026-07-24-htrcli-network-mock.md) attach to this var with
// networkCmd.AddCommand(...) in their own init() and must NOT redefine it or
// re-register it on rootCmd.
var networkCmd = &cobra.Command{
	Use:   "network",
	Short: "Read and watch captured network requests",
}

var networkReadCmd = &cobra.Command{
	Use:   "read",
	Short: "Read buffered network entries",
	RunE: func(cmd *cobra.Command, args []string) error {
		if UseCDP() {
			return errUnsupportedCDP("network read")
		}
		tabID, err := GetTabID()
		if err != nil {
			return err
		}
		poller := &EventPoller{Client: GetClient(), TabID: tabID, Kind: networkEventKind}
		resp, err := poller.Read(networkSince)
		if err != nil {
			return err
		}
		if output.JSONOutput {
			output.PrintJSON(resp)
			return nil
		}
		fmt.Print(formatNetworkEntries(resp))
		return nil
	},
}

var networkWatchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Arm capture and stream network entries until timeout",
	RunE: func(cmd *cobra.Command, args []string) error {
		if UseCDP() {
			return errUnsupportedCDP("network watch")
		}
		tabID, err := GetTabID()
		if err != nil {
			return err
		}
		if err := armNetworkCapture(tabID, networkTimeoutMS); err != nil {
			return err
		}
		poller := &EventPoller{Client: GetClient(), TabID: tabID, Kind: networkEventKind}
		handle := func(resp api.EventsResponse) error {
			if output.JSONOutput {
				output.PrintJSON(resp)
				return nil
			}
			fmt.Print(formatNetworkEntries(&resp))
			return nil
		}
		timeout := time.Duration(networkTimeoutMS) * time.Millisecond
		return poller.Watch(cmd.Context(), timeout, networkSince, nil, handle)
	},
}

var networkWaitCmd = &cobra.Command{
	Use:   "wait",
	Short: "Arm capture and block until a matching request completes",
	RunE: func(cmd *cobra.Command, args []string) error {
		if UseCDP() {
			return errUnsupportedCDP("network wait")
		}
		if networkWaitURL == "" {
			return fmt.Errorf("--url is required (glob pattern to match against request URLs)")
		}
		tabID, err := GetTabID()
		if err != nil {
			return err
		}
		if err := armNetworkCapture(tabID, networkTimeoutMS); err != nil {
			return err
		}
		poller := &EventPoller{Client: GetClient(), TabID: tabID, Kind: networkEventKind}

		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()
		var matched *api.EventEntry
		match := func(entry api.EventEntry) bool {
			return networkEntryMatches(entry, networkWaitURL, networkWaitStatus)
		}
		handle := func(resp api.EventsResponse) error {
			if matched == nil && len(resp.Entries) > 0 {
				e := resp.Entries[0]
				matched = &e
				cancel()
			}
			return nil
		}
		timeout := time.Duration(networkTimeoutMS) * time.Millisecond
		if err := poller.Watch(ctx, timeout, networkSince, match, handle); err != nil {
			return err
		}
		if matched == nil {
			return fmt.Errorf("no request matching %q%s arrived within %dms", networkWaitURL, statusSuffix(networkWaitStatus), networkTimeoutMS)
		}
		if output.JSONOutput {
			output.PrintJSON(matched)
			return nil
		}
		fmt.Println(formatNetworkEvent(*matched))
		return nil
	},
}

func statusSuffix(status int) string {
	if status > 0 {
		return fmt.Sprintf(" (status %d)", status)
	}
	return ""
}

func init() {
	networkReadCmd.Flags().IntVar(&networkSince, "since", 0, "cursor to read after")
	networkWatchCmd.Flags().IntVar(&networkSince, "since", 0, "cursor to watch after")
	networkWatchCmd.Flags().IntVar(&networkTimeoutMS, "timeout", 10000, "capture/watch window in ms")
	networkWaitCmd.Flags().IntVar(&networkSince, "since", 0, "cursor to wait after")
	networkWaitCmd.Flags().IntVar(&networkTimeoutMS, "timeout", 10000, "how long to wait, in ms")
	networkWaitCmd.Flags().StringVar(&networkWaitURL, "url", "", "glob pattern to match against request URLs")
	networkWaitCmd.Flags().IntVar(&networkWaitStatus, "status", 0, "also require this HTTP status code")

	networkCmd.AddCommand(networkReadCmd)
	networkCmd.AddCommand(networkWatchCmd)
	networkCmd.AddCommand(networkWaitCmd)
	rootCmd.AddCommand(networkCmd)
}
