package commands

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/u007/htrcli/internal/api"
	"github.com/u007/htrcli/internal/output"
)

// traceBundle is the aggregated data written into a trace zip. Network is empty
// until the network-capture sibling plan lands; ScreenshotPNG is empty when the
// snapshot could not be captured.
type traceBundle struct {
	Page          *api.PageInfo
	Console       []api.EventEntry
	Network       []api.EventEntry
	ScreenshotPNG []byte
	ExportedAt    time.Time
}

// buildTraceZip renders the bundle into a zip mirroring the extension-side
// exportToZip layout: a combined trace.json, raw console/network arrays, an
// optional screenshot, and a README.
func buildTraceZip(b traceBundle) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	writeFile := func(name string, data []byte) error {
		w, err := zw.Create(name)
		if err != nil {
			return fmt.Errorf("creating zip entry %s: %w", name, err)
		}
		if _, err := w.Write(data); err != nil {
			return fmt.Errorf("writing zip entry %s: %w", name, err)
		}
		return nil
	}

	trace := map[string]any{
		"exportedAt": b.ExportedAt.UTC().Format(time.RFC3339),
		"page":       b.Page,
		"console":    b.Console,
		"network":    b.Network,
	}
	traceJSON, err := json.MarshalIndent(trace, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling trace.json: %w", err)
	}
	if err := writeFile("trace.json", traceJSON); err != nil {
		return nil, err
	}

	consoleJSON, err := json.MarshalIndent(b.Console, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling console.json: %w", err)
	}
	if err := writeFile("console.json", consoleJSON); err != nil {
		return nil, err
	}

	networkJSON, err := json.MarshalIndent(b.Network, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling network.json: %w", err)
	}
	if err := writeFile("network.json", networkJSON); err != nil {
		return nil, err
	}

	if len(b.ScreenshotPNG) > 0 {
		if err := writeFile("screenshots/snapshot.png", b.ScreenshotPNG); err != nil {
			return nil, err
		}
	}

	if err := writeFile("README.md", []byte(buildTraceReadme(b))); err != nil {
		return nil, err
	}

	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("finalizing zip: %w", err)
	}
	return buf.Bytes(), nil
}

// buildTraceReadme renders a short human summary of the bundle.
func buildTraceReadme(b traceBundle) string {
	var sb strings.Builder
	sb.WriteString("# htrcli trace export\n\n")
	fmt.Fprintf(&sb, "Exported: %s\n\n", b.ExportedAt.UTC().Format(time.RFC3339))
	if b.Page != nil {
		fmt.Fprintf(&sb, "- URL: %s\n- Title: %s\n", b.Page.URL, b.Page.Title)
	}
	fmt.Fprintf(&sb, "- Console entries: %d\n", len(b.Console))
	fmt.Fprintf(&sb, "- Network entries: %d\n", len(b.Network))
	if len(b.ScreenshotPNG) > 0 {
		sb.WriteString("- Screenshot: screenshots/snapshot.png\n")
	}
	sb.WriteString("\nContents:\n")
	sb.WriteString("- trace.json — combined page info + console + network\n")
	sb.WriteString("- console.json / network.json — raw event arrays\n")
	return sb.String()
}

// collectTrace aggregates the current page, buffered console + network events,
// and a snapshot screenshot into a traceBundle. Page info and console are
// required; network and the screenshot are best-effort (network capture is a
// sibling feature; a snapshot needs a connected tab) — their absence is logged,
// not fatal, so a partial-but-useful trace still exports.
func collectTrace(c *api.Client, tabID *int) (traceBundle, error) {
	b := traceBundle{ExportedAt: time.Now()}

	page, err := c.GetPageInfo(tabID)
	if err != nil {
		return b, fmt.Errorf("reading page info: %w", err)
	}
	b.Page = page

	consoleResp, err := c.GetEvents(tabID, "console", 0)
	if err != nil {
		return b, fmt.Errorf("reading console events: %w", err)
	}
	b.Console = consoleResp.Entries

	networkResp, err := c.GetEvents(tabID, "network", 0)
	if err != nil {
		// Non-fatal: network capture is a sibling plan; export console-only.
		fmt.Fprintf(os.Stderr, "[htrcli] network events unavailable (network capture not yet enabled): %v\n", err)
	} else {
		b.Network = networkResp.Entries
	}

	screenshotOpts := api.ScreenshotOptions{}
	if tabID != nil {
		screenshotOpts.TabID = tabID
	}
	shot, err := c.GetScreenshotOpts(screenshotOpts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[htrcli] screenshot unavailable for trace: %v\n", err)
	} else {
		png, derr := base64.StdEncoding.DecodeString(shot)
		if derr != nil {
			fmt.Fprintf(os.Stderr, "[htrcli] decoding trace screenshot: %v\n", derr)
		} else {
			b.ScreenshotPNG = png
		}
	}

	return b, nil
}

var traceCmd = &cobra.Command{
	Use:   "trace",
	Short: "Export a debug trace bundle",
}

var traceExportCmd = &cobra.Command{
	Use:   "export <path.zip>",
	Short: "Export console + network + screenshot + page info as a zip",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if UseCDP() {
			return errUnsupportedCDP("trace export")
		}
		tabID, err := GetTabID()
		if err != nil {
			return err
		}
		bundle, err := collectTrace(GetClient(), tabID)
		if err != nil {
			return err
		}
		data, err := buildTraceZip(bundle)
		if err != nil {
			return err
		}
		out := args[0]
		if err := os.WriteFile(out, data, 0644); err != nil {
			return fmt.Errorf("writing %s: %w", out, err)
		}
		if output.JSONOutput {
			output.PrintJSON(map[string]any{
				"trace":   out,
				"console": len(bundle.Console),
				"network": len(bundle.Network),
			})
			return nil
		}
		fmt.Printf("Trace exported to %s (%d console, %d network entries)\n", out, len(bundle.Console), len(bundle.Network))
		return nil
	},
}

func init() {
	traceCmd.AddCommand(traceExportCmd)
	rootCmd.AddCommand(traceCmd)
}
