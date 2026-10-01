package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/u007/htrcli/internal/api"
	"github.com/u007/htrcli/internal/output"
)

// Session recordings — the extension's OWN recorder (interaction steps +
// screenshots persisted in IndexedDB), driven remotely.
//
// This is deliberately a separate verb from `htrcli record`, which captures
// PAGE VIDEO via CDP screencast + ffmpeg and is Chrome-only. The commands here
// need no CDP and work on both Chrome and Firefox, because everything is
// handled by the extension's background service worker.

var (
	recTitle           string
	recAudio           bool
	recLimit           int
	recOffset          int
	recWithScreenshots bool
	recOutput          string
)

var recordingsCmd = &cobra.Command{
	Use:   "recordings",
	Short: "Session recordings (steps + screenshots) — works on Chrome and Firefox",
	Long: `Record and retrieve the extension's session recordings.

A "session recording" is a step-by-step log of what happened in the browser
(clicks, inputs, navigations) with a screenshot per step, plus your
annotations. It is stored in the extension's IndexedDB and works on both
Chrome and Firefox.

This is NOT the same as "htrcli record", which captures page VIDEO via the
Chrome DevTools Protocol and needs ffmpeg. Use "htrcli record" for MP4.

Typical remote-control flow:
  htrcli recordings start --title "Checkout flow"
  htrcli click @e1 && htrcli fill @e2 "hello"
  htrcli recordings stop
  htrcli recordings list
  htrcli recordings export <id> ./checkout.json`,
}

// errRecordingsCDP is the explicit guard for the CDP transport: these commands
// are answered by the extension's background worker, which the CDP path never
// reaches. Names the extension action so the message is greppable.
func errRecordingsCDP(action string) error {
	return fmt.Errorf("the %q action is not supported over --cdp — session recordings live in the extension; run with --transport ext", action)
}

// sendRecordingCommand dispatches one recording action and unwraps the
// CommandResult.
//
// These actions are answered by the extension's BACKGROUND service worker, not
// by a content script, so there is no page to target and the session spans the
// whole browser profile. That is why this goes to the TAB-LESS route
// (POST /api/background/command) rather than passing a nil tab to
// /api/command.
//
// The distinction matters. A nil tab on /api/command is resolved by the daemon
// through FirstTabID, which meant a recording could only be driven when at least
// one http/https tab with an active content script had reported in — so it
// failed on a chrome:// page, a settings page, a headless browser, or a browser
// sitting entirely on new-tab. The background route selects a relay CONNECTION
// instead, so only the extension needs to be running. The only requirement now
// is that some browser relay is connected.
func sendRecordingCommand(action string, options map[string]any) (*api.CommandResult, error) {
	if UseCDP() {
		return nil, errRecordingsCDP(action)
	}
	return GetClient().ExecuteCommand(nil, api.Command{
		ID:      "1",
		Action:  action,
		Options: options,
	})
}

// recordingData dispatches a recording action and decodes the response's
// `data` payload directly into out.
//
// It uses ExecuteCommandInto rather than sendRecordingCommand + json.Marshal on
// purpose. The old path held the payload three times over: ExecuteCommand
// materialised it as map[string]any, Marshal turned that back into bytes, and
// Unmarshal parsed those bytes a third time into out. For `export` on a long
// recording that is ~48 MB of base64 screenshots copied twice for nothing.
// ExecuteCommandInto decodes the body once into out.
func recordingData(action string, options map[string]any, out any) error {
	if UseCDP() {
		// Delegate the transport rejection so that message keeps exactly one home.
		_, err := sendRecordingCommand(action, options)
		return err
	}
	// The tab-less background route, carrying the advisory --browser hint so a
	// caller can name the profile it wants. The hint falls back rather than
	// failing when that profile is not connected, and recordingList reports
	// which browser actually answered.
	if err := GetClient().ExecuteBackgroundCommandInto(api.Command{
		ID:      "1",
		Action:  action,
		Options: options,
	}, BrowserHint(), out); err != nil {
		return fmt.Errorf("failed to parse %s response: %w", action, err)
	}
	return nil
}

var recordingsStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start recording the current session",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		options := map[string]any{"hasAudio": recAudio}
		if recTitle != "" {
			options["title"] = recTitle
		}
		var data api.RecordingStateData
		if err := recordingData("recordingStart", options, &data); err != nil {
			return err
		}
		if output.JSONOutput {
			output.PrintJSON(data)
			return nil
		}
		title := ""
		if data.Session != nil {
			title = data.Session.Title
		}
		fmt.Printf("Recording started: %s\n", title)
		fmt.Printf("Stop it with: htrcli recordings stop\n")
		return nil
	},
}

var recordingsStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the current recording",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		var data api.RecordingStateData
		if err := recordingData("recordingStop", nil, &data); err != nil {
			return err
		}
		if output.JSONOutput {
			output.PrintJSON(data)
			return nil
		}
		if data.Session == nil {
			fmt.Println("Recording stopped")
			return nil
		}
		fmt.Printf("Recording stopped: %s (%d steps)\n", data.Session.Title, data.Session.StepCount)
		fmt.Printf("Inspect it with: htrcli recordings get %s\n", data.Session.ID)
		return nil
	},
}

var recordingsStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show whether a recording is in progress",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		var data api.RecordingStateData
		if err := recordingData("recordingStatus", nil, &data); err != nil {
			return err
		}
		if output.JSONOutput {
			output.PrintJSON(data)
			return nil
		}
		if !data.Recording {
			fmt.Println("Not recording")
			return nil
		}
		if data.Session == nil {
			fmt.Println("Recording (details unavailable)")
			return nil
		}
		dur := time.Duration(time.Now().UnixMilli()-data.Session.StartTime) * time.Millisecond
		fmt.Printf("Recording: %s (running %s)\n", data.Session.Title, dur.Round(time.Second))
		return nil
	},
}

var recordingsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List stored recordings (newest first)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		var data api.RecordingListData
		if err := recordingData("recordingList", map[string]any{
			"limit":  recLimit,
			"offset": recOffset,
		}, &data); err != nil {
			return err
		}
		if output.JSONOutput {
			output.PrintJSON(data)
			return nil
		}
		if len(data.Sessions) == 0 {
			if data.Total == 0 {
				fmt.Println("No recordings yet")
			} else {
				fmt.Printf("No recordings in this page (offset %d of %d)\n", data.Offset, data.Total)
			}
			return nil
		}
		// Name the answering browser once, above the rows. --browser is an
		// advisory hint that falls back rather than failing, so this line is the
		// only way a user can tell that the profile they asked for is not the
		// profile that answered.
		if data.Browser != "" {
			fmt.Printf("Answered by: %s\n\n", data.Browser)
		}
		for _, s := range data.Sessions {
			started := time.UnixMilli(s.StartTime).Format("2006-01-02 15:04:05")
			fmt.Printf("%s  %-28s  %s  %3d steps", s.ID, truncate(s.Title, 28), started, s.StepCount)
			if s.AnnotationCount > 0 {
				fmt.Printf("  %d notes", s.AnnotationCount)
			}
			if s.HasAudio {
				fmt.Printf("  [audio]")
			}
			if s.EndTime == nil {
				fmt.Printf("  [live]")
			}
			fmt.Println()
		}
		if data.Offset+len(data.Sessions) < data.Total {
			fmt.Printf("\n%d of %d — next: htrcli recordings list --offset %d\n",
				data.Offset+len(data.Sessions), data.Total, data.Offset+len(data.Sessions))
		}
		return nil
	},
}

// maxMediaSteps bounds `--with-screenshots` / `export` requests.
//
// The extension must serialise the whole hydrated session into ONE
// native-messaging frame, and that frame is capped at 64 MiB on both sides
// (host.MaxMessageSize). Over the cap the daemon's relay treats the frame as a
// protocol error and RETIRES the connection — the extension then loses remote
// control until it reconnects, so every later command fails. That is a far
// worse failure than a clean refusal, hence this pre-check.
//
// The cap is in BYTES, and base64 inflates PNGs by ~4/3, so 48 MiB of raw
// screenshot data is already over the line. Since the extension cannot report
// a size before serialising, the check is by step count: it errs on the side of
// refusing too often rather than killing the host. Sessions that do fit can
// still be pulled without `--with-screenshots`, which is the normal path.
const maxMediaSteps = 120

// errTooManySteps explains the ceiling and the way out.
func errTooManySteps(n int) error {
	return fmt.Errorf(
		"session has %d steps — too large to fetch with screenshots in one message "+
			"(the native-messaging frame is capped at 64 MiB, and a single frame over "+
			"the cap tears down the connection, losing remote control). "+
			"Use `htrcli recordings get <id>` without --with-screenshots, "+
			"or split the flow into shorter sessions", n)
}

// preflightMediaSize refuses a media-bearing fetch that is likely to blow the
// native-messaging frame cap, BEFORE requesting it.
//
// It asks `recordingList` (cheap metadata only — no screenshots) for the
// session's step count. If the session is not in the first maxMediaSteps+1
// entries we cannot pre-check it and let the request through; that is the
// acceptable direction, since the alternative is refusing every fetch of an
// older session.
func preflightMediaSize(sessionID string) error {
	var list api.RecordingListData
	if err := recordingData("recordingList", map[string]any{"limit": maxMediaSteps + 1}, &list); err != nil {
		// Metadata lookup failed; don't block the real request over it.
		return nil
	}
	for _, s := range list.Sessions {
		if s.ID == sessionID && s.StepCount > maxMediaSteps {
			return errTooManySteps(s.StepCount)
		}
	}
	return nil
}

var recordingsGetCmd = &cobra.Command{
	Use:   "get <session-id>",
	Short: "Print a recording's steps and annotations as JSON",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if recWithScreenshots {
			if err := preflightMediaSize(args[0]); err != nil {
				return err
			}
		}
		var data api.RecordingGetData
		if err := recordingData("recordingGet", map[string]any{
			"sessionId":    args[0],
			"includeMedia": recWithScreenshots,
		}, &data); err != nil {
			return err
		}
		if recOutput != "" {
			if err := writeJSONFile(recOutput, data); err != nil {
				return err
			}
			if !output.JSONOutput {
				fmt.Printf("Wrote %s (%d steps, %d annotations)\n",
					recOutput, len(data.Session.Steps), len(data.Session.Annotations))
				if data.MediaStripped {
					fmt.Println("Screenshots were NOT included — re-run with --with-screenshots")
				}
			}
			return nil
		}
		output.PrintJSON(data)
		return nil
	},
}

var recordingsExportCmd = &cobra.Command{
	Use:   "export <session-id> <output.{json,zip,md}>",
	Short: "Write a recording (with screenshots) to JSON, a ZIP bundle, or Markdown",
	Long: `Write a recording to a file. The format comes from the output extension:

  .json            the raw recordingGet payload, base64 screenshots inline
  .zip             a bundle: recording.json + README.md + screenshots/ + audio/
                   (identical layout to the sidepanel's "Export ZIP")
  .md / .markdown  a human-readable timeline

Screenshots are always included.`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionID, out := args[0], args[1]
		// Export always asks for media: writing a screenshot-free file when the
		// user said "export" would be a silent downgrade. So it is also the
		// path most likely to exceed the native-messaging frame cap.
		if err := preflightMediaSize(sessionID); err != nil {
			return err
		}
		var data api.RecordingGetData
		if err := recordingData("recordingGet", map[string]any{
			"sessionId":    sessionID,
			"includeMedia": true,
		}, &data); err != nil {
			return err
		}

		switch recordingExportFormatFor(out) {
		case exportZip:
			if err := writeRecordingZip(out, &data.Session); err != nil {
				return err
			}
			if !output.JSONOutput {
				fmt.Printf("Wrote %s (%d steps, %d annotations, with screenshots)\n",
					out, len(data.Session.Steps), len(data.Session.Annotations))
			}
		case exportMarkdown:
			if err := writeTextFile(out, generateRecordingMarkdown(&data.Session)); err != nil {
				return err
			}
			if !output.JSONOutput {
				fmt.Printf("Wrote %s (%d steps, %d annotations)\n",
					out, len(data.Session.Steps), len(data.Session.Annotations))
			}
		default:
			if err := writeJSONFile(out, data); err != nil {
				return err
			}
			if !output.JSONOutput {
				fmt.Printf("Wrote %s (%d steps, %d annotations, with screenshots)\n",
					out, len(data.Session.Steps), len(data.Session.Annotations))
			}
		}
		return nil
	},
}

var recordingsDeleteCmd = &cobra.Command{
	Use:   "delete <session-id>",
	Short: "Delete a stored recording",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var data api.RecordingDeleteData
		if err := recordingData("recordingDelete", map[string]any{"sessionId": args[0]}, &data); err != nil {
			return err
		}
		if output.JSONOutput {
			output.PrintJSON(data)
			return nil
		}
		if !data.Deleted {
			return fmt.Errorf("no recording found with id %s", data.ID)
		}
		fmt.Printf("Deleted recording %s\n", data.ID)
		return nil
	},
}

// writeJSONFile writes v as indented JSON to path, creating parent dirs.
func writeJSONFile(path string, v any) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolving output path: %w", err)
	}
	if dir := filepath.Dir(abs); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("creating output directory: %w", err)
		}
	}
	buf, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding JSON: %w", err)
	}
	buf = append(buf, '\n')
	if err := os.WriteFile(abs, buf, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", abs, err)
	}
	return nil
}

// writeTextFile writes a UTF-8 text artifact, creating parent directories.
func writeTextFile(path, content string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolving output path: %w", err)
	}
	if dir := filepath.Dir(abs); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("creating output directory: %w", err)
		}
	}
	if err := os.WriteFile(abs, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", abs, err)
	}
	return nil
}

// truncate shortens s to at most n runes, marking the cut with an ellipsis.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	if n <= 1 {
		return string(runes[:n])
	}
	return string(runes[:n-1]) + "…"
}

func init() {
	recordingsStartCmd.Flags().StringVar(&recTitle, "title", "", "recording title")
	recordingsStartCmd.Flags().BoolVar(&recAudio, "audio", false, "also capture microphone audio (off by default)")

	recordingsListCmd.Flags().IntVar(&recLimit, "limit", 50, "max recordings to return (1-500)")
	recordingsListCmd.Flags().IntVar(&recOffset, "offset", 0, "skip this many recordings")

	recordingsGetCmd.Flags().BoolVar(&recWithScreenshots, "with-screenshots", false, "include base64 screenshots (large)")
	recordingsGetCmd.Flags().StringVar(&recOutput, "output", "", "write to this file instead of stdout")

	recordingsCmd.AddCommand(recordingsStartCmd)
	recordingsCmd.AddCommand(recordingsStopCmd)
	recordingsCmd.AddCommand(recordingsStatusCmd)
	recordingsCmd.AddCommand(recordingsListCmd)
	recordingsCmd.AddCommand(recordingsGetCmd)
	recordingsCmd.AddCommand(recordingsExportCmd)
	recordingsCmd.AddCommand(recordingsDeleteCmd)
	rootCmd.AddCommand(recordingsCmd)
}
