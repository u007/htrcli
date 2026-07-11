package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/u007/htrcli/internal/api"
	"github.com/u007/htrcli/internal/cdp"
	"github.com/u007/htrcli/internal/output"
)

// errUnsupportedCDP guards verbs not yet ported to the CDP transport, so a
// sticky `transport=cdp` config can never silently misroute them to the
// extension daemon (pressing keys or scrolling in the wrong browser).
func errUnsupportedCDP(name string) error {
	return fmt.Errorf("%s is not supported over --cdp yet — run it with --transport ext", name)
}

// cdpSession opens the target page session for the current --tab/--cdp-port.
func cdpSession() (*cdp.Session, string, error) {
	targetID := GetTabTarget()
	s, err := cdp.PageSession(GetCDPPort(), targetID)
	if err != nil {
		return nil, "", err
	}
	if targetID == "" {
		targets, err := cdp.ListTargets(GetCDPPort())
		if err == nil && len(targets) > 0 {
			targetID = targets[0].ID
		}
	}
	return s, targetID, nil
}

// runInteractCDP mirrors runInteract over the CDP transport.
func runInteractCDP(action, selector, value string) error {
	s, targetID, err := cdpSession()
	if err != nil {
		return err
	}
	defer s.Close()

	switch action {
	case "click", "dblclick", "rightclick":
		if err := cdp.Click(s, targetID, parseSelector(selector), action); err != nil {
			return err
		}
	case "pressKey":
		if err := cdp.Press(s, targetID, value); err != nil {
			return err
		}
	default: // fill, select, check, uncheck, clear, type — DOM verbs
		result, err := cdp.ExecDOM(s, api.Command{
			ID: "1", Action: action, Target: parseSelector(selector), Value: value,
		})
		if err != nil {
			return err
		}
		if !result.Success {
			return fmt.Errorf("%s failed: %s", action, result.Error)
		}
	}
	if output.JSONOutput {
		output.PrintJSON(map[string]any{"success": true, "action": action})
		return nil
	}
	fmt.Printf("%s %s (cdp)\n", strings.Title(action), selector)
	return nil
}

// runInspectCDP routes inspect verbs (find, getValue, getText, getHTML,
// getAttribute, getPageInfo, ...) through the embedded DOM bundle.
func runInspectCDP(action, selector, attr string) error {
	s, _, err := cdpSession()
	if err != nil {
		return err
	}
	defer s.Close()

	cmd := api.Command{ID: "1", Action: action, Target: parseSelector(selector)}
	if action == "getAttribute" && attr != "" {
		cmd.Options = map[string]any{"attribute": attr}
	}
	result, err := cdp.ExecDOM(s, cmd)
	if err != nil {
		return err
	}
	if !result.Success {
		return fmt.Errorf("%s failed: %s", action, result.Error)
	}
	if output.JSONOutput {
		output.PrintJSON(result)
		return nil
	}
	fmt.Printf("%v\n", result.Data)
	return nil
}

// runEvalCDP runs a user script via CDP Runtime.evaluate.
func runEvalCDP(expression string) error {
	s, _, err := cdpSession()
	if err != nil {
		return err
	}
	defer s.Close()

	raw, err := cdp.Evaluate(s, expression)
	if err != nil {
		return err
	}
	if output.JSONOutput {
		output.PrintJSON(json.RawMessage(raw))
		return nil
	}
	fmt.Printf("%s\n", raw)
	return nil
}

// runScreenshotCDP captures the page via CDP Page.captureScreenshot.
func runScreenshotCDP(path string) error {
	s, _, err := cdpSession()
	if err != nil {
		return err
	}
	defer s.Close()

	png, err := cdp.Screenshot(s)
	if err != nil {
		return err
	}
	out := path
	if out == "" {
		out = filepath.Join(os.TempDir(), fmt.Sprintf("screenshot-%d.png", time.Now().UnixMilli()))
	}
	if err := os.WriteFile(out, png, 0644); err != nil {
		return fmt.Errorf("failed to write screenshot: %w", err)
	}
	if output.JSONOutput {
		output.PrintJSON(map[string]string{"screenshot": out})
		return nil
	}
	fmt.Printf("Screenshot saved to %s\n", out)
	return nil
}

// runOpenCDP navigates the page via CDP.
func runOpenCDP(url string) error {
	s, _, err := cdpSession()
	if err != nil {
		return err
	}
	defer s.Close()

	if err := cdp.Navigate(s, url, timeout); err != nil {
		return err
	}
	if output.JSONOutput {
		output.PrintJSON(map[string]any{"success": true, "url": url})
		return nil
	}
	fmt.Printf("Navigated to %s (cdp)\n", url)
	return nil
}

// runTabsListCDP lists CDP page targets (no Active column — CDP has no
// reliable notion of "active").
func runTabsListCDP() error {
	targets, err := cdp.ListTargets(GetCDPPort())
	if err != nil {
		return err
	}
	if output.JSONOutput {
		output.PrintJSON(targets)
		return nil
	}
	if len(targets) == 0 {
		fmt.Println("No tabs connected")
		return nil
	}
	table := output.NewTable("ID", "Title", "URL")
	for _, t := range targets {
		title := t.Title
		if len(title) > 40 {
			title = title[:37] + "..."
		}
		url := t.URL
		if len(url) > 40 {
			url = url[:37] + "..."
		}
		table.AddRow(t.ID, title, url)
	}
	fmt.Print(table)
	return nil
}
