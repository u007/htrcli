package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/u007/htrcli/internal/api"
	"github.com/u007/htrcli/internal/cdp"
	"github.com/u007/htrcli/internal/output"
)

// Over --cdp there is no daemon buffer: every htrcli invocation opens a fresh
// CDP session. console/network verbs therefore read what the page itself still
// holds (V8's console replay, the Performance API) or stream for the command's
// duration. Seq numbers are per-invocation ordinals, never a cursor.

// cdpEventsResponse is the --json shape of a CDP read: the extension's
// EventsResponse plus where the data came from and what it cannot contain.
type cdpEventsResponse struct {
	api.EventsResponse
	Transport   string   `json:"transport"`
	Source      string   `json:"source"`
	Limitations []string `json:"limitations"`
}

var consoleCDPLimitations = []string{
	"replay holds only what V8 still stores for the current document: it is cleared on navigation and capped by V8 (oldest messages evicted)",
	"seq numbers are per-invocation ordinals, not a cursor; --since is rejected",
}

var networkReadCDPLimitations = []string{
	"completed requests come from the page's Performance API: method and request/response bodies are NOT available",
	"in-flight requests are not listed; status is 0/— when the browser does not expose it (cross-origin without Timing-Allow-Origin, or Chrome < 109)",
	"only the current document's requests; Chrome's resource-timing buffer holds 250 entries by default",
	"seq numbers are per-invocation ordinals, not a cursor; --since is rejected",
}

const networkStreamCDPNote = "[htrcli] --cdp: only requests completing after this command attached are seen; start it before the action that triggers the request"

// rejectSinceCDP fails loudly when --since was passed on the CDP path, where
// no stable cursor exists across invocations.
func rejectSinceCDP(cmd *cobra.Command, verb string) error {
	if cmd.Flags().Changed("since") {
		return fmt.Errorf("%s: --since is not supported over --cdp — each --cdp invocation is a new CDP session, so seq numbers are per-invocation ordinals, not a stable cursor; drop --since, or use --transport ext for the daemon's cursor buffer", verb)
	}
	return nil
}

// cdpEventEntry wraps CDP-collected data in the extension's event envelope so
// both transports share the formatters and filters.
func cdpEventEntry(seq int, kind string, timestamp int64, data any) (api.EventEntry, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return api.EventEntry{}, fmt.Errorf("encoding %s entry: %w", kind, err)
	}
	return api.EventEntry{Seq: seq, Kind: kind, Timestamp: timestamp, Data: raw}, nil
}

func cdpEventsDeadline(timeoutMS int) time.Time {
	if timeoutMS <= 0 {
		return time.Time{} // no deadline, as the extension path's Watch treats timeout <= 0
	}
	return time.Now().Add(time.Duration(timeoutMS) * time.Millisecond)
}

func runConsoleReadCDP(cmd *cobra.Command) error {
	if err := rejectSinceCDP(cmd, "console read"); err != nil {
		return err
	}
	level, err := parseConsoleLevelFilter(consoleLevelFilter)
	if err != nil {
		return err
	}
	s, _, err := cdpSession()
	if err != nil {
		return err
	}
	defer s.Close()

	w, err := cdp.WatchConsole(s)
	if err != nil {
		return err
	}
	resp := cdpEventsResponse{Transport: "cdp", Source: "runtime-replay", Limitations: consoleCDPLimitations}
	resp.Entries = []api.EventEntry{}
	for i, e := range w.Replayed() {
		entry, err := cdpEventEntry(i+1, consoleEventKind, e.Timestamp, e)
		if err != nil {
			return err
		}
		if consoleEntryMatchesLevel(entry, level) {
			resp.Entries = append(resp.Entries, entry)
		}
	}
	if len(resp.Entries) > 0 {
		resp.OldestAvailableSeq = resp.Entries[0].Seq
	}
	if output.JSONOutput {
		output.PrintJSON(resp)
		return nil
	}
	fmt.Fprintf(os.Stderr, "[htrcli] --cdp console read: %s\n", strings.Join(consoleCDPLimitations, "; "))
	printConsoleEvents(resp.EventsResponse, level)
	return nil
}

func runConsoleWatchCDP(cmd *cobra.Command) error {
	if err := rejectSinceCDP(cmd, "console watch"); err != nil {
		return err
	}
	level, err := parseConsoleLevelFilter(consoleLevelFilter)
	if err != nil {
		return err
	}
	s, _, err := cdpSession()
	if err != nil {
		return err
	}
	defer s.Close()

	deadline := cdpEventsDeadline(consoleWatchTimeoutMS)
	w, err := cdp.WatchConsole(s)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "[htrcli] --cdp console watch: streaming new messages only (%d earlier ones are available via console read)\n", len(w.Replayed()))
	for seq := 1; ; seq++ {
		e, err := w.Next(deadline)
		if errors.Is(err, cdp.ErrEventDeadline) {
			return nil
		}
		if err != nil {
			return err
		}
		entry, err := cdpEventEntry(seq, consoleEventKind, e.Timestamp, e)
		if err != nil {
			return err
		}
		if !consoleEntryMatchesLevel(entry, level) {
			continue
		}
		batch := api.EventsResponse{Entries: []api.EventEntry{entry}, OldestAvailableSeq: entry.Seq}
		if output.JSONOutput {
			output.PrintJSON(batch)
			continue
		}
		printConsoleEvents(batch, level)
	}
}

// formatCDPNetworkEvent is formatNetworkEvent plus the CDP-only fields.
func formatCDPNetworkEvent(entry api.EventEntry, e cdp.NetworkEntry) string {
	line := formatNetworkEvent(entry)
	var extras []string
	if e.InitiatorType != "" {
		extras = append(extras, e.InitiatorType)
	}
	if e.ResourceType != "" {
		extras = append(extras, e.ResourceType)
	}
	if e.TransferSize > 0 {
		extras = append(extras, fmt.Sprintf("%dB", e.TransferSize))
	}
	if len(extras) > 0 {
		line += " [" + strings.Join(extras, ", ") + "]"
	}
	if e.Error != "" {
		line += " error: " + e.Error
	}
	return line
}

func runNetworkReadCDP(cmd *cobra.Command) error {
	if err := rejectSinceCDP(cmd, "network read"); err != nil {
		return err
	}
	s, _, err := cdpSession()
	if err != nil {
		return err
	}
	defer s.Close()

	entries, bufferMaybeFull, err := cdp.ReadPerformanceEntries(s)
	if err != nil {
		return err
	}
	limitations := networkReadCDPLimitations
	if bufferMaybeFull {
		limitations = append(append([]string{}, limitations...), fmt.Sprintf(
			"WARNING: %d+ resource entries — the resource-timing buffer is likely full and later requests are missing", cdp.DefaultResourceTimingBufferSize))
	}
	resp := cdpEventsResponse{Transport: "cdp", Source: "performance-timeline", Limitations: limitations}
	resp.Entries = make([]api.EventEntry, 0, len(entries))
	for i, e := range entries {
		entry, err := cdpEventEntry(i+1, networkEventKind, e.Timestamp, e)
		if err != nil {
			return err
		}
		resp.Entries = append(resp.Entries, entry)
	}
	if len(resp.Entries) > 0 {
		resp.OldestAvailableSeq = 1
	}
	if output.JSONOutput {
		output.PrintJSON(resp)
		return nil
	}
	fmt.Fprintf(os.Stderr, "[htrcli] --cdp network read: %s\n", strings.Join(limitations, "; "))
	for i, entry := range resp.Entries {
		fmt.Println(formatCDPNetworkEvent(entry, entries[i]))
	}
	return nil
}

// streamNetworkCDP attaches Network.enable for the command duration and calls
// handle for each completed request until handle returns stop=true or the
// deadline passes (reported as timedOut).
func streamNetworkCDP(timeoutMS int, handle func(api.EventEntry, cdp.NetworkEntry) (stop bool, err error)) (timedOut bool, err error) {
	s, _, err := cdpSession()
	if err != nil {
		return false, err
	}
	defer s.Close()

	deadline := cdpEventsDeadline(timeoutMS)
	w, err := cdp.WatchNetwork(s)
	if err != nil {
		return false, err
	}
	fmt.Fprintln(os.Stderr, networkStreamCDPNote)
	for seq := 1; ; seq++ {
		e, err := w.Next(deadline)
		if errors.Is(err, cdp.ErrEventDeadline) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		entry, err := cdpEventEntry(seq, networkEventKind, e.Timestamp, e)
		if err != nil {
			return false, err
		}
		stop, err := handle(entry, *e)
		if err != nil || stop {
			return false, err
		}
	}
}

func runNetworkWatchCDP(cmd *cobra.Command) error {
	if err := rejectSinceCDP(cmd, "network watch"); err != nil {
		return err
	}
	_, err := streamNetworkCDP(networkTimeoutMS, func(entry api.EventEntry, e cdp.NetworkEntry) (bool, error) {
		if output.JSONOutput {
			output.PrintJSON(api.EventsResponse{Entries: []api.EventEntry{entry}, OldestAvailableSeq: entry.Seq})
			return false, nil
		}
		fmt.Println(formatCDPNetworkEvent(entry, e))
		return false, nil
	})
	return err
}

func runNetworkWaitCDP(cmd *cobra.Command) error {
	if err := rejectSinceCDP(cmd, "network wait"); err != nil {
		return err
	}
	if networkWaitURL == "" {
		return fmt.Errorf("--url is required (glob pattern to match against request URLs)")
	}
	var matched *api.EventEntry
	var matchedData cdp.NetworkEntry
	timedOut, err := streamNetworkCDP(networkTimeoutMS, func(entry api.EventEntry, e cdp.NetworkEntry) (bool, error) {
		if !networkEntryMatches(entry, networkWaitURL, networkWaitStatus) {
			return false, nil
		}
		matched, matchedData = &entry, e
		return true, nil
	})
	if err != nil {
		return err
	}
	if timedOut || matched == nil {
		return fmt.Errorf("no request matching %q%s completed within %dms over --cdp (requests that completed before `network wait` attached are not visible — start it before the triggering action, or check `htrcli --cdp network read`)",
			networkWaitURL, statusSuffix(networkWaitStatus), networkTimeoutMS)
	}
	if output.JSONOutput {
		output.PrintJSON(matched)
		return nil
	}
	fmt.Println(formatCDPNetworkEvent(*matched, matchedData))
	return nil
}
