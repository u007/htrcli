package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/u007/htrcli/internal/api"
	"github.com/u007/htrcli/internal/output"
)

const consoleEventKind = "console"

var allowedConsoleLevels = map[string]struct{}{
	"log":   {},
	"warn":  {},
	"error": {},
	"info":  {},
	"debug": {},
}

type consoleEventData struct {
	Level  string   `json:"level"`
	Args   []string `json:"args"`
	Source string   `json:"source,omitempty"`
}

// EventPoller polls cursor-based event APIs for a single tab/kind bucket.
type EventPoller struct {
	Client   *api.Client
	TabID    *int
	Kind     string
	Interval time.Duration
}

// Read fetches a single buffered snapshot starting after since.
func (p *EventPoller) Read(since int) (*api.EventsResponse, error) {
	if p == nil || p.Client == nil {
		return nil, fmt.Errorf("no API client configured")
	}
	kind := p.Kind
	if kind == "" {
		kind = consoleEventKind
	}
	return p.Client.GetEvents(p.TabID, kind, since)
}

// Watch polls until the context expires, calling handle for each matching batch.
// A batch is considered matching when it contains at least one matching entry,
// or when the daemon reports dropped entries (so the caller can surface the
// eviction warning even if the filter removes every entry in that batch).
func (p *EventPoller) Watch(
	ctx context.Context,
	timeout time.Duration,
	since int,
	match func(api.EventEntry) bool,
	handle func(api.EventsResponse) error,
) error {
	interval := p.Interval
	if interval <= 0 {
		interval = 250 * time.Millisecond
	}

	watchCtx := ctx
	var cancel context.CancelFunc
	if timeout > 0 {
		watchCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	cursor := since
	for {
		resp, err := p.Read(cursor)
		if err != nil {
			return err
		}
		if len(resp.Entries) > 0 {
			cursor = resp.Entries[len(resp.Entries)-1].Seq
		}

		filtered := filterConsoleEvents(resp.Entries, match)
		if len(filtered) > 0 || resp.Dropped > 0 {
			if err := handle(api.EventsResponse{
				Entries:            filtered,
				Dropped:            resp.Dropped,
				OldestAvailableSeq: resp.OldestAvailableSeq,
			}); err != nil {
				return err
			}
		}

		select {
		case <-watchCtx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func filterConsoleEvents(entries []api.EventEntry, match func(api.EventEntry) bool) []api.EventEntry {
	if match == nil {
		out := make([]api.EventEntry, len(entries))
		copy(out, entries)
		return out
	}
	out := make([]api.EventEntry, 0, len(entries))
	for _, entry := range entries {
		if match(entry) {
			out = append(out, entry)
		}
	}
	return out
}

func parseConsoleLevelFilter(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	level := strings.ToLower(strings.TrimSpace(raw))
	if _, ok := allowedConsoleLevels[level]; !ok {
		return "", fmt.Errorf("invalid level %q (expected log, warn, error, info, or debug)", raw)
	}
	return level, nil
}

func consoleEntryMatchesLevel(entry api.EventEntry, level string) bool {
	if level == "" {
		return true
	}
	var data consoleEventData
	if err := json.Unmarshal(entry.Data, &data); err != nil {
		return false
	}
	return strings.EqualFold(data.Level, level)
}

func formatConsoleEvent(entry api.EventEntry) string {
	var data consoleEventData
	if err := json.Unmarshal(entry.Data, &data); err == nil && data.Level != "" {
		args := strings.Join(data.Args, " ")
		if args == "" {
			args = "(no args)"
		}
		if data.Source != "" {
			return fmt.Sprintf("[seq %d] %s: %s (%s)", entry.Seq, data.Level, args, data.Source)
		}
		return fmt.Sprintf("[seq %d] %s: %s", entry.Seq, data.Level, args)
	}

	if len(entry.Data) == 0 {
		return fmt.Sprintf("[seq %d] %s", entry.Seq, entry.Kind)
	}
	return fmt.Sprintf("[seq %d] %s: %s", entry.Seq, entry.Kind, string(entry.Data))
}

// formatConsoleEntries renders the current buffer to a string for tests and
// non-interactive callers.
func formatConsoleEntries(resp *api.EventsResponse) string {
	if resp == nil {
		return ""
	}
	var b strings.Builder
	if resp.Dropped > 0 {
		fmt.Fprintf(&b, "%s %d events were evicted (buffer cap reached)\n", output.Warning("⚠"), resp.Dropped)
	}
	for _, entry := range resp.Entries {
		b.WriteString(formatConsoleEvent(entry))
		b.WriteByte('\n')
	}
	return b.String()
}

func printConsoleEvents(resp api.EventsResponse, levelFilter string) {
	if resp.Dropped > 0 {
		fmt.Printf("%s %d events were evicted (buffer cap reached)\n", output.Warning("⚠"), resp.Dropped)
	}
	for _, entry := range resp.Entries {
		if !consoleEntryMatchesLevel(entry, levelFilter) {
			continue
		}
		fmt.Println(formatConsoleEvent(entry))
	}
}

var consoleCmd = &cobra.Command{
	Use:   "console",
	Short: "Read and watch console events",
}

var consoleReadSince int
var consoleLevelFilter string

var consoleReadCmd = &cobra.Command{
	Use:   "read",
	Short: "Read buffered console events",
	RunE: func(cmd *cobra.Command, args []string) error {
		if UseCDP() {
			return errUnsupportedCDP("console read")
		}
		level, err := parseConsoleLevelFilter(consoleLevelFilter)
		if err != nil {
			return err
		}
		tabID, err := GetTabID()
		if err != nil {
			return err
		}
		poller := EventPoller{Client: GetClient(), TabID: tabID, Kind: consoleEventKind}
		resp, err := poller.Read(consoleReadSince)
		if err != nil {
			return err
		}

		filtered := api.EventsResponse{
			Dropped:            resp.Dropped,
			OldestAvailableSeq: resp.OldestAvailableSeq,
			Entries:            filterConsoleEvents(resp.Entries, func(entry api.EventEntry) bool { return consoleEntryMatchesLevel(entry, level) }),
		}
		if output.JSONOutput {
			output.PrintJSON(filtered)
			return nil
		}

		printConsoleEvents(filtered, level)
		return nil
	},
}

var consoleWatchTimeoutMS int

var consoleWatchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Watch console events until timeout",
	RunE: func(cmd *cobra.Command, args []string) error {
		if UseCDP() {
			return errUnsupportedCDP("console watch")
		}
		level, err := parseConsoleLevelFilter(consoleLevelFilter)
		if err != nil {
			return err
		}
		tabID, err := GetTabID()
		if err != nil {
			return err
		}
		poller := EventPoller{Client: GetClient(), TabID: tabID, Kind: consoleEventKind}
		match := func(entry api.EventEntry) bool { return consoleEntryMatchesLevel(entry, level) }
		handle := func(resp api.EventsResponse) error {
			if output.JSONOutput {
				output.PrintJSON(resp)
				return nil
			}
			printConsoleEvents(resp, level)
			return nil
		}
		timeout := time.Duration(consoleWatchTimeoutMS) * time.Millisecond
		return poller.Watch(cmd.Context(), timeout, consoleReadSince, match, handle)
	},
}

func init() {
	consoleReadCmd.PersistentFlags().IntVar(&consoleReadSince, "since", 0, "cursor to read after")
	consoleReadCmd.PersistentFlags().StringVar(&consoleLevelFilter, "level", "", "filter by console level")
	consoleWatchCmd.PersistentFlags().IntVar(&consoleReadSince, "since", 0, "cursor to watch after")
	consoleWatchCmd.PersistentFlags().StringVar(&consoleLevelFilter, "level", "", "filter by console level")
	consoleWatchCmd.PersistentFlags().IntVar(&consoleWatchTimeoutMS, "timeout", 30000, "watch timeout in ms")

	consoleCmd.AddCommand(consoleReadCmd)
	consoleCmd.AddCommand(consoleWatchCmd)
	rootCmd.AddCommand(consoleCmd)
}
