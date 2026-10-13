package commands

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/u007/htrcli/internal/api"
	"github.com/u007/htrcli/internal/cdp"
	"github.com/u007/htrcli/internal/output"
)

// tabsBrowser is the --browser filter for the tabs subcommands. It is separate
// from the root --browser hint, which is advisory and falls back to any browser;
// a filter must not silently widen to the other browser.
var tabsBrowser string

var tabsCmd = &cobra.Command{
	Use:   "tabs",
	Short: "Manage browser tabs",
}

var tabsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List connected browser tabs",
	RunE: func(cmd *cobra.Command, args []string) error {
		browser, err := tabsFilter()
		if err != nil {
			return err
		}
		if UseCDP() {
			return runTabsListCDP()
		}
		c := GetClient()
		tabs, err := c.ListTabs()
		if err != nil {
			return err
		}
		tabs = filterTabs(tabs, browser)

		if output.JSONOutput {
			output.PrintJSON(tabs)
			return nil
		}

		if len(tabs) == 0 {
			fmt.Println("No tabs connected")
			return nil
		}

		table := output.NewTable("ID", "Browser", "Title", "URL", "Active")
		for _, t := range tabs {
			active := "no"
			if t.Active {
				active = "yes"
			}
			title := t.Title
			if len(title) > 40 {
				title = title[:37] + "..."
			}
			url := t.URL
			if len(url) > 40 {
				url = url[:37] + "..."
			}
			table.AddRow(strconv.Itoa(t.ID), browserLabel(t.Browser), title, url, active)
		}
		fmt.Print(table)
		return nil
	},
}

var tabsGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get information about a specific tab",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		browser, err := tabsFilter()
		if err != nil {
			return err
		}
		if UseCDP() {
			return runTabsGetCDP(args[0])
		}
		id, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid tab ID: %s", args[0])
		}

		c := GetClient()
		tabs, err := c.ListTabs()
		if err != nil {
			return err
		}
		tab, err := findTab(tabs, id, browser)
		if err != nil {
			return err
		}

		if output.JSONOutput {
			output.PrintJSON(tab)
			return nil
		}

		fmt.Printf("ID:      %d\n", tab.ID)
		fmt.Printf("Browser: %s\n", browserLabel(tab.Browser))
		fmt.Printf("Title:   %s\n", tab.Title)
		fmt.Printf("URL:     %s\n", tab.URL)
		fmt.Printf("Active:  %v\n", tab.Active)
		if tab.FavIconURL != "" {
			fmt.Printf("Favicon: %s\n", tab.FavIconURL)
		}
		return nil
	},
}

var tabsNewCmd = &cobra.Command{
	Use:   "new <url>",
	Short: "Open a URL in a new tab (not activated)",
	Long: `Open a URL in a new tab and print its ID and browser, usable with --tab.

The extension's background worker opens the tab, so no connected page is needed:
it works even when only chrome:// pages are open. With --browser the request goes
to that browser alone and fails if it is not connected. Without it, the
earliest-connected browser answers. The new tab is not activated, so later
commands without --tab keep targeting the current tab.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		browser, err := tabsFilter()
		if err != nil {
			return err
		}
		if UseCDP() {
			return runTabsNewCDP(args[0])
		}
		c := GetClient()
		command := api.Command{
			ID:      "1",
			Action:  "openTab",
			Options: map[string]any{"url": args[0]},
		}

		var opened openedTab
		if browser == "" {
			err = c.ExecuteBackgroundCommandInto(command, "", &opened)
		} else {
			err = c.ExecuteBrowserCommandInto(browser, command, &opened)
		}
		if err != nil {
			return err
		}
		if opened.TabID == 0 || opened.Browser == "" {
			return fmt.Errorf("openTab returned no tabId or browser: %+v", opened)
		}

		if output.JSONOutput {
			output.PrintJSON(map[string]any{"tabId": opened.TabID, "browser": opened.Browser})
			return nil
		}
		fmt.Printf("Opened tab %d (%s)\n", opened.TabID, opened.Browser)
		return nil
	},
}

var tabsCloseCmd = &cobra.Command{
	Use:   "close <id>",
	Short: "Close a tab",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		browser, err := tabsFilter()
		if err != nil {
			return err
		}
		if UseCDP() {
			return runTabsCloseCDP(args[0])
		}
		id, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid tab ID: %s", args[0])
		}

		c := GetClient()
		tabs, err := c.ListTabs()
		if err != nil {
			return err
		}
		target, err := findTab(tabs, id, browser)
		if err != nil {
			return err
		}
		if target.Browser == "" {
			return fmt.Errorf("tab %d has no browser metadata; cannot route the close", id)
		}
		// closeTab runs in the target browser's background worker, so it no longer
		// needs another tab in that browser to carry the request.
		if err := c.ExecuteBrowserCommandInto(target.Browser, api.Command{
			ID:      "1",
			Action:  "closeTab",
			Options: map[string]any{"tabId": id},
		}, nil); err != nil {
			return err
		}

		if output.JSONOutput {
			output.PrintJSON(map[string]any{"closed": id, "browser": target.Browser})
			return nil
		}
		fmt.Printf("Closed tab %d (%s)\n", id, target.Browser)
		return nil
	},
}

// openedTab is the openTab result: the new tab's ID and the browser that opened it.
type openedTab struct {
	TabID   int    `json:"tabId"`
	Browser string `json:"browser"`
}

// tabsFilter validates --browser and rejects a browser CDP cannot drive. It
// returns the filter ("" = all browsers).
func tabsFilter() (string, error) {
	if err := validateBrowserHint(tabsBrowser); err != nil {
		return "", err
	}
	if UseCDP() && tabsBrowser == "firefox" {
		return "", fmt.Errorf("--browser firefox has no tabs over --cdp (CDP drives Chrome only)")
	}
	return tabsBrowser, nil
}

// filterTabs keeps the tabs of browser ("" keeps all) and sorts them by
// browser, then ID, so output order is stable. Connected tabs are few, so the
// list is not paginated.
func filterTabs(tabs []api.TabInfo, browser string) []api.TabInfo {
	out := make([]api.TabInfo, 0, len(tabs))
	for _, t := range tabs {
		if browser == "" || t.Browser == browser {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Browser != out[j].Browser {
			return out[i].Browser < out[j].Browser
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// findTab resolves id to one connected tab, limited to browser when set. A
// native tab ID can exist in Chrome and Firefox at once, so an unscoped match
// in more than one browser is an error rather than a guess.
func findTab(tabs []api.TabInfo, id int, browser string) (api.TabInfo, error) {
	var matches []api.TabInfo
	for _, t := range filterTabs(tabs, browser) {
		if t.ID == id {
			matches = append(matches, t)
		}
	}
	switch len(matches) {
	case 0:
		if browser != "" {
			return api.TabInfo{}, fmt.Errorf("tab %d not connected in %s", id, browser)
		}
		return api.TabInfo{}, fmt.Errorf("tab %d not connected", id)
	case 1:
		return matches[0], nil
	default:
		return api.TabInfo{}, fmt.Errorf("tab %d exists in more than one browser; pass --browser chrome or --browser firefox", id)
	}
}

func browserLabel(browser string) string {
	if browser == "" {
		return "-"
	}
	return browser
}

// runTabsGetCDP prints one CDP page target. CDP target IDs are hex strings.
func runTabsGetCDP(id string) error {
	if err := ensureContextResolved(); err != nil {
		return err
	}
	targets, err := cdp.ListTargets(GetCDPPort())
	if err != nil {
		return err
	}
	for _, t := range targets {
		if t.ID != id {
			continue
		}
		if output.JSONOutput {
			output.PrintJSON(t)
			return nil
		}
		fmt.Printf("ID:    %s\n", t.ID)
		fmt.Printf("Title: %s\n", t.Title)
		fmt.Printf("URL:   %s\n", t.URL)
		return nil
	}
	return fmt.Errorf("tab %s not connected", id)
}

// runTabsNewCDP opens url in a new CDP page target.
func runTabsNewCDP(url string) error {
	if err := ensureContextResolved(); err != nil {
		return err
	}
	targetID, err := cdp.CreateTarget(GetCDPPort(), url)
	if err != nil {
		return err
	}
	if output.JSONOutput {
		output.PrintJSON(map[string]any{"tabId": targetID})
		return nil
	}
	fmt.Printf("Opened tab %s (cdp)\n", targetID)
	return nil
}

// runTabsCloseCDP closes a CDP page target by its hex target ID.
func runTabsCloseCDP(id string) error {
	if err := ensureContextResolved(); err != nil {
		return err
	}
	if err := cdp.CloseTarget(GetCDPPort(), id); err != nil {
		return err
	}
	if output.JSONOutput {
		output.PrintJSON(map[string]any{"closed": id})
		return nil
	}
	fmt.Printf("Closed tab %s (cdp)\n", id)
	return nil
}

func init() {
	tabsCmd.PersistentFlags().StringVar(&tabsBrowser, "browser", "",
		"limit tabs commands to one browser (chrome|firefox); omit for all browsers")
	tabsCmd.AddCommand(tabsListCmd)
	tabsCmd.AddCommand(tabsGetCmd)
	tabsCmd.AddCommand(tabsNewCmd)
	tabsCmd.AddCommand(tabsCloseCmd)
	rootCmd.AddCommand(tabsCmd)
}
