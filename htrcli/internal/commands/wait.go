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
	waitGone     bool
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

// requireGoneAck accepts only the extension's gone answer ({"gone": true}). Any
// other payload is an appear answer from a build that predates --gone, which
// would otherwise read as success while the element is still there.
func requireGoneAck(data any) error {
	info, ok := data.(map[string]any)
	if !ok || info["gone"] != true {
		return fmt.Errorf("wait --gone: extension did not confirm the element is gone (its build predates --gone; update the extension)")
	}
	return nil
}

func waitExtSelector(timeout int) error {
	c := GetClient()
	tabID, err := GetTabID()
	if err != nil {
		return err
	}
	target := parseSelector(waitSelector)
	opts := map[string]any{"timeout": timeout, "interval": waitInterval}
	if waitGone {
		opts["gone"] = true
	}
	result, err := c.ExecuteCommand(tabID, api.Command{
		ID:      "1",
		Action:  "wait",
		Target:  target,
		Options: opts,
	})
	if err != nil {
		return err
	}
	if err := commandError(result); err != nil {
		return err
	}
	// An extension build that predates --gone ignores options.gone and answers
	// the appear wait, so success here proves nothing about absence. Only the
	// gone answer carries gone:true; anything else is refused.
	if waitGone {
		if err := requireGoneAck(result.Data); err != nil {
			return err
		}
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

// evalWaitCheck interprets one findAll tick. done reports whether the wait
// condition holds: a match exists for appear, none exists for gone. A payload
// that is not a list (including nil) is an error, never an empty result:
// findAll always answers a list, so a missing one must not read as "gone".
func evalWaitCheck(data any, gone bool) (done bool, matches []map[string]any, err error) {
	list, ok := data.([]any)
	if !ok {
		return false, nil, fmt.Errorf("wait: findAll returned %T, expected a list", data)
	}
	matches = make([]map[string]any, 0, len(list))
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			return false, nil, fmt.Errorf("wait: findAll returned a %T match, expected an object", item)
		}
		matches = append(matches, m)
	}
	return (len(matches) == 0) == gone, matches, nil
}

// goneUnsettled reports whether an empty match cannot count as gone yet. A
// blank tab has no page to wait on (an error, not a pass), and a document still
// loading has not rendered its elements (keep polling).
func goneUnsettled(selector string, page *api.PageInfo) (bool, error) {
	if page == nil {
		return false, fmt.Errorf("wait --gone: %q: findAll answered without pageInfo, cannot tell if the page is settled", selector)
	}
	if page.URL == "about:blank" {
		return false, fmt.Errorf("wait --gone: %q cannot be checked on about:blank (no page loaded)", selector)
	}
	return page.ReadyState == "loading", nil
}

// isDetachedRefError matches the page's DetachedRefError (src/contentScript/
// refRegistry.ts): the @eN ref's element was removed from the document, which
// is the gone condition. The "not known" ref error (never minted, or the page
// navigated) is deliberately NOT matched and stays a failure.
func isDetachedRefError(err error) bool {
	return strings.Contains(err.Error(), "points to an element that is no longer in the document")
}

// goneTimeoutError names the selector and the element that stayed present.
func goneTimeoutError(selector string, timeout int, matches []map[string]any) error {
	tag, _ := matches[0]["tag"].(string)
	id, _ := matches[0]["id"].(string)
	desc := tag
	if id != "" {
		desc += "#" + id
	}
	return fmt.Errorf("wait: element %q still present after %dms: %s (%d match(es))", selector, timeout, desc, len(matches))
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
	// as the page-side wait does. With --gone the sense is inverted: success is
	// the absence of every match (use it to prove a row was archived/deleted).
	deadline := time.Now().Add(time.Duration(timeout) * time.Millisecond)
	for {
		result, err := cdp.ExecDOM(s, api.Command{ID: "1", Action: "findAll", Target: target})
		if err != nil {
			return err
		}
		var done, unsettled bool
		var matches []map[string]any
		if err := commandError(result); err != nil {
			if !waitGone || !isDetachedRefError(err) {
				return err
			}
			// The ref's element was removed: that is the gone condition.
			done = true
		} else {
			done, matches, err = evalWaitCheck(result.Data, waitGone)
			if err != nil {
				return err
			}
			if waitGone && done {
				unsettled, err = goneUnsettled(waitSelector, result.PageInfo)
				if err != nil {
					return err
				}
				done = !unsettled
			}
		}
		if done {
			if output.JSONOutput {
				output.PrintJSON(map[string]any{"ok": true})
			} else {
				fmt.Println("ok")
			}
			return nil
		}
		if time.Now().After(deadline) {
			if waitGone {
				if unsettled {
					return fmt.Errorf("wait: %q not confirmed gone, document still loading after %dms", waitSelector, timeout)
				}
				return goneTimeoutError(waitSelector, timeout, matches)
			}
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
	if waitGone && waitSelector == "" {
		return fmt.Errorf("wait: --gone only applies to --selector")
	}
	return nil
}

var waitCmd = &cobra.Command{
	Use:   "wait",
	Short: "Wait for a DOM condition (--selector/--text/--url; --gone inverts --selector)",
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
	waitCmd.Flags().BoolVar(&waitGone, "gone", false, "With --selector: wait until NO element matches (archive/delete)")
	waitCmd.Flags().StringVar(&waitText, "text", "", "Text to wait for in document body")
	waitCmd.Flags().StringVar(&waitURL, "url", "", "Substring to wait for in the page URL")
	waitCmd.Flags().IntVar(&waitTimeout, "timeout", 10000, "Timeout in ms")
	waitCmd.Flags().IntVar(&waitInterval, "interval", 250, "Poll interval in ms")
	rootCmd.AddCommand(waitCmd)
}
