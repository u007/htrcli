package commands

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/u007/htrcli/internal/api"
	"github.com/u007/htrcli/internal/cdp"
	"github.com/u007/htrcli/internal/output"
)

var (
	waitSelector string
	waitText     string
	waitURL      string
	waitTimeout  int
	waitInterval int
)

// waitConditionJS builds a JS expression for text/url conditions (selector
// conditions are handled by the native `wait` action instead).
func waitConditionJS() (string, error) {
	switch {
	case waitText != "":
		b, _ := json.Marshal(waitText)
		return fmt.Sprintf("!!document.body && document.body.innerText.indexOf(%s) !== -1", string(b)), nil
	case waitURL != "":
		b, _ := json.Marshal(waitURL)
		return fmt.Sprintf("window.location.href.indexOf(%s) !== -1", string(b)), nil
	default:
		return "", fmt.Errorf("wait: specify --selector, --text, or --url")
	}
}

func waitExtSelector(timeout int) error {
	c := GetClient()
	tabID, err := GetTabID()
	if err != nil {
		return err
	}
	target := parseSelector(waitSelector)
	result, err := c.ExecuteCommand(tabID, api.Command{
		ID:      "1",
		Action:  "wait",
		Target:  target,
		Options: map[string]any{"timeout": timeout},
	})
	if err != nil {
		return err
	}
	if err := commandError(result); err != nil {
		return err
	}
	if output.JSONOutput {
		output.PrintJSON(result.Data)
	} else {
		fmt.Println("ok")
	}
	return nil
}

func waitExtPoll(expr string, timeout, interval int) error {
	deadline := time.Now().Add(time.Duration(timeout) * time.Millisecond)
	for {
		c := GetClient()
		tabID, err := GetTabID()
		if err != nil {
			return err
		}
		result, err := c.ExecuteCommand(tabID, api.Command{
			ID:     "1",
			Action: "evaluate",
			Value:  expr,
		})
		if err != nil {
			return err
		}
		if err := commandError(result); err != nil {
			return err
		}
		ok := false
		switch v := result.Data.(type) {
		case bool:
			ok = v
		case float64:
			ok = v != 0
		case string:
			s := strings.TrimSpace(v)
			ok = s == "true" || s == "1" || (s != "" && s != "false" && s != "0" && s != "null")
		}
		if ok {
			if output.JSONOutput {
				output.PrintJSON(map[string]any{"ok": true})
			} else {
				fmt.Println("ok")
			}
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("wait: timeout after %dms", timeout)
		}
		time.Sleep(time.Duration(interval) * time.Millisecond)
	}
}

func waitCDPSelector(timeout int) error {
	target := parseSelector(waitSelector)
	if err := checkFrameTarget(target); err != nil {
		return err
	}
	s, _, err := cdpSession()
	if err != nil {
		return err
	}
	defer s.Close()
	// Poll findAll each tick rather than one long page-side wait. A navigation
	// during a single evaluate destroys its execution context, which is not
	// retried; a fresh evaluate per tick follows the new document. findAll
	// resolves the target the same way every other CDP DOM verb does (CSS,
	// name=, role=, text=, label=, xpath and @eN refs) and matches on presence,
	// as the page-side wait does.
	deadline := time.Now().Add(time.Duration(timeout) * time.Millisecond)
	for {
		result, err := cdp.ExecDOM(s, api.Command{ID: "1", Action: "findAll", Target: target})
		if err != nil {
			return err
		}
		if err := commandError(result); err != nil {
			return err
		}
		// A page with no matches may report no data at all.
		found := false
		switch matches := result.Data.(type) {
		case nil:
		case []any:
			found = len(matches) > 0
		default:
			return fmt.Errorf("wait: findAll returned %T, expected a list", result.Data)
		}
		if found {
			if output.JSONOutput {
				output.PrintJSON(map[string]any{"ok": true})
			} else {
				fmt.Println("ok")
			}
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("wait: timeout after %dms", timeout)
		}
		time.Sleep(time.Duration(waitInterval) * time.Millisecond)
	}
}

func waitCDPPoll(expr string, timeout, interval int) error {
	s, _, err := cdpSession()
	if err != nil {
		return err
	}
	defer s.Close()
	deadline := time.Now().Add(time.Duration(timeout) * time.Millisecond)
	for {
		raw, err := cdp.Evaluate(s, expr)
		if err != nil {
			return err
		}
		v := strings.TrimSpace(string(raw))
		ok := v == "true" || v == "1"
		if !ok {
			var anyv any
			if err := json.Unmarshal(raw, &anyv); err == nil {
				switch x := anyv.(type) {
				case bool:
					ok = x
				case float64:
					ok = x != 0
				}
			}
		}
		if ok {
			if output.JSONOutput {
				output.PrintJSON(map[string]any{"ok": true})
			} else {
				fmt.Println("ok")
			}
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("wait: timeout after %dms", timeout)
		}
		time.Sleep(time.Duration(interval) * time.Millisecond)
	}
}

func validateWaitFlags() error {
	if waitTimeout <= 0 {
		return fmt.Errorf("wait: --timeout must be > 0")
	}
	if waitInterval <= 0 {
		return fmt.Errorf("wait: --interval must be > 0")
	}
	count := 0
	if waitSelector != "" {
		count++
	}
	if waitText != "" {
		count++
	}
	if waitURL != "" {
		count++
	}
	if count != 1 {
		return fmt.Errorf("wait: specify exactly one of --selector, --text, --url")
	}
	return nil
}

var waitCmd = &cobra.Command{
	Use:   "wait",
	Short: "Wait for a DOM condition (--selector/--text/--url)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateWaitFlags(); err != nil {
			return err
		}

		if waitSelector != "" {
			if UseCDP() {
				return waitCDPSelector(waitTimeout)
			}
			return waitExtSelector(waitTimeout)
		}
		expr, err := waitConditionJS()
		if err != nil {
			return err
		}
		if UseCDP() {
			return waitCDPPoll(expr, waitTimeout, waitInterval)
		}
		return waitExtPoll(expr, waitTimeout, waitInterval)
	},
}

func init() {
	waitCmd.Flags().StringVar(&waitSelector, "selector", "", "CSS selector to wait for")
	waitCmd.Flags().StringVar(&waitText, "text", "", "Text to wait for in document body")
	waitCmd.Flags().StringVar(&waitURL, "url", "", "Substring to wait for in the page URL")
	waitCmd.Flags().IntVar(&waitTimeout, "timeout", 10000, "Timeout in ms")
	waitCmd.Flags().IntVar(&waitInterval, "interval", 250, "Poll interval in ms")
	rootCmd.AddCommand(waitCmd)
}
