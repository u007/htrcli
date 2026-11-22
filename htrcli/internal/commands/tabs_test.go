package commands

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/u007/htrcli/internal/api"
	"github.com/u007/htrcli/internal/output"
)

// tabsDaemon is a fake daemon with a fixed tab list. It records the last command
// (path, browser, strict flag, action, options) so tests can assert the routing.
// openTab answers with openedTabID and the browser that answered: the strictly
// named browser when there is one, otherwise answeredBy.
type tabsDaemon struct {
	*httptest.Server
	tabs        []api.TabInfo
	gotPath     string
	gotBrowser  string
	gotStrict   bool
	gotAction   string
	gotOptions  map[string]any
	openedTabID int
	answeredBy  string
}

// tabsCommandBody is the part of a command request body the fake daemon reads.
// It serves both /api/tabs/<id>/command and /api/background/command.
type tabsCommandBody struct {
	Command       api.Command `json:"command"`
	Browser       string      `json:"browser"`
	BrowserStrict bool        `json:"browserStrict"`
}

func newTabsDaemon(t *testing.T, tabs []api.TabInfo, openedTabID int) *tabsDaemon {
	t.Helper()
	td := &tabsDaemon{tabs: tabs, openedTabID: openedTabID, answeredBy: "chrome"}
	td.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" && r.URL.Path == "/api/tabs" {
			json.NewEncoder(w).Encode(api.ApiResponse{OK: true, Data: td.tabs})
			return
		}
		var req tabsCommandBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decoding request: %v", err)
		}
		td.gotPath = r.URL.Path
		td.gotBrowser = req.Browser
		td.gotStrict = req.BrowserStrict
		td.gotAction = req.Command.Action
		td.gotOptions = req.Command.Options

		var data any
		if req.Command.Action == "openTab" {
			answered := td.answeredBy
			if req.BrowserStrict {
				answered = req.Browser
			}
			data = map[string]any{"tabId": td.openedTabID, "browser": answered}
		}
		json.NewEncoder(w).Encode(api.ApiResponse{
			OK:   true,
			Data: api.CommandResult{ID: req.Command.ID, Success: true, Data: data},
		})
	}))
	t.Cleanup(td.Close)
	return td
}

// useTabsDaemon points the package client at td and forces the extension
// transport with a clean --browser filter.
func useTabsDaemon(t *testing.T, td *tabsDaemon, browser string) {
	t.Helper()
	resetTransportState()
	prevClient := client
	prevBrowser := tabsBrowser
	client = api.NewClient(td.URL, "")
	tabsBrowser = browser
	t.Cleanup(func() {
		client = prevClient
		tabsBrowser = prevBrowser
		resetTransportState()
	})
}

var sharedIDTabs = []api.TabInfo{
	{ID: 7, Browser: "firefox", Title: "ff", URL: "https://ff.example"},
	{ID: 7, Browser: "chrome", Title: "ch", URL: "https://ch.example"},
	{ID: 9, Browser: "chrome", Title: "ch2", URL: "https://ch2.example"},
}

func TestFilterTabsByBrowserSortsByBrowserThenID(t *testing.T) {
	got := filterTabs(sharedIDTabs, "")
	if len(got) != 3 || got[0].Browser != "chrome" || got[0].ID != 7 || got[1].ID != 9 || got[2].Browser != "firefox" {
		t.Fatalf("unfiltered order = %+v", got)
	}

	firefox := filterTabs(sharedIDTabs, "firefox")
	if len(firefox) != 1 || firefox[0].Browser != "firefox" {
		t.Fatalf("firefox filter = %+v", firefox)
	}
}

func TestFindTabAmbiguousWithoutBrowserFilter(t *testing.T) {
	_, err := findTab(sharedIDTabs, 7, "")
	if err == nil || !strings.Contains(err.Error(), "--browser") {
		t.Fatalf("shared tab ID must demand --browser, got %v", err)
	}

	tab, err := findTab(sharedIDTabs, 7, "firefox")
	if err != nil || tab.Title != "ff" {
		t.Fatalf("firefox-scoped lookup = %+v, %v", tab, err)
	}
}

func TestFindTabMissingInBrowserFails(t *testing.T) {
	_, err := findTab(sharedIDTabs, 9, "firefox")
	if err == nil || !strings.Contains(err.Error(), "not connected in firefox") {
		t.Fatalf("want not-connected-in-firefox error, got %v", err)
	}
}

func TestTabsGetRejectsSharedIDWithoutBrowser(t *testing.T) {
	td := newTabsDaemon(t, sharedIDTabs, 0)
	useTabsDaemon(t, td, "")

	err := tabsGetCmd.RunE(tabsGetCmd, []string{"7"})
	if err == nil || !strings.Contains(err.Error(), "--browser") {
		t.Fatalf("tabs get 7 without --browser must be ambiguous, got %v", err)
	}
}

// TestTabsNewStrictlyTargetsNamedBrowser pins that --browser on tabs new is a
// hard filter: the request goes to that browser alone, and the output names it.
func TestTabsNewStrictlyTargetsNamedBrowser(t *testing.T) {
	td := newTabsDaemon(t, sharedIDTabs, 55)
	useTabsDaemon(t, td, "firefox")

	var err error
	out := captureStdout(t, func() {
		err = tabsNewCmd.RunE(tabsNewCmd, []string{"https://example.com"})
	})
	if err != nil {
		t.Fatalf("tabs new: %v", err)
	}
	if td.gotPath != "/api/background/command" || td.gotBrowser != "firefox" || !td.gotStrict {
		t.Fatalf("routed to %s browser=%q strict=%v; want strict firefox background command", td.gotPath, td.gotBrowser, td.gotStrict)
	}
	if td.gotAction != "openTab" || td.gotOptions["url"] != "https://example.com" {
		t.Fatalf("action=%q options=%v", td.gotAction, td.gotOptions)
	}
	if out != "Opened tab 55 (firefox)\n" {
		t.Fatalf("output = %q", out)
	}
}

// TestTabsNewWithoutBrowserReportsEarliestBrowser pins the default: no hint goes
// to the background route unpinned, and the output names the browser that answered.
func TestTabsNewWithoutBrowserReportsEarliestBrowser(t *testing.T) {
	td := newTabsDaemon(t, sharedIDTabs, 55)
	useTabsDaemon(t, td, "")

	var err error
	out := captureStdout(t, func() {
		err = tabsNewCmd.RunE(tabsNewCmd, []string{"https://example.com"})
	})
	if err != nil {
		t.Fatalf("tabs new: %v", err)
	}
	if td.gotPath != "/api/background/command" || td.gotBrowser != "" || td.gotStrict {
		t.Fatalf("routed to %s browser=%q strict=%v; want unpinned background command", td.gotPath, td.gotBrowser, td.gotStrict)
	}
	if out != "Opened tab 55 (chrome)\n" {
		t.Fatalf("output = %q", out)
	}
}

// TestTabsNewJSONReportsAnsweringBrowser pins the JSON shape: tabId plus the
// browser that opened the tab, even when no --browser filter was given.
func TestTabsNewJSONReportsAnsweringBrowser(t *testing.T) {
	td := newTabsDaemon(t, nil, 55)
	td.answeredBy = "firefox"
	useTabsDaemon(t, td, "")
	output.JSONOutput = true
	t.Cleanup(func() { output.JSONOutput = false })

	var err error
	out := captureStdout(t, func() {
		err = tabsNewCmd.RunE(tabsNewCmd, []string{"https://example.com"})
	})
	if err != nil {
		t.Fatalf("tabs new: %v", err)
	}
	var got map[string]any
	if jerr := json.Unmarshal([]byte(out), &got); jerr != nil {
		t.Fatalf("output is not JSON: %v\n%s", jerr, out)
	}
	if got["tabId"] != float64(55) || got["browser"] != "firefox" {
		t.Fatalf("json output = %v; want tabId 55 browser firefox", got)
	}
}

// TestTabsNewSucceedsWithoutTabInNamedBrowser pins the fix for a browser that has
// no http(s) tab open: the background worker opens the tab, so no tab is needed.
func TestTabsNewSucceedsWithoutTabInNamedBrowser(t *testing.T) {
	td := newTabsDaemon(t, nil, 55)
	useTabsDaemon(t, td, "firefox")

	var err error
	out := captureStdout(t, func() {
		err = tabsNewCmd.RunE(tabsNewCmd, []string{"https://example.com"})
	})
	if err != nil {
		t.Fatalf("tabs new with no tabs in firefox: %v", err)
	}
	if td.gotPath != "/api/background/command" || td.gotBrowser != "firefox" || !td.gotStrict {
		t.Fatalf("routed to %s browser=%q strict=%v; want strict firefox background command", td.gotPath, td.gotBrowser, td.gotStrict)
	}
	if out != "Opened tab 55 (firefox)\n" {
		t.Fatalf("output = %q", out)
	}
}

// TestTabsCloseStrictlyTargetsOwningBrowser pins that close goes to the browser
// that owns the tab, pinned so a shared tab ID cannot close the other browser's tab.
func TestTabsCloseStrictlyTargetsOwningBrowser(t *testing.T) {
	td := newTabsDaemon(t, sharedIDTabs, 0)
	useTabsDaemon(t, td, "chrome")

	if err := tabsCloseCmd.RunE(tabsCloseCmd, []string{"7"}); err != nil {
		t.Fatalf("tabs close: %v", err)
	}
	if td.gotPath != "/api/background/command" || td.gotBrowser != "chrome" || !td.gotStrict {
		t.Fatalf("close routed to %s browser=%q strict=%v; want strict chrome background command", td.gotPath, td.gotBrowser, td.gotStrict)
	}
	if td.gotAction != "closeTab" || td.gotOptions["tabId"] != float64(7) {
		t.Fatalf("action=%q options=%v", td.gotAction, td.gotOptions)
	}
}

// TestTabsCloseLastTabInBrowserSucceeds pins that a browser's only tab can be
// closed: the close no longer depends on another tab in that browser.
func TestTabsCloseLastTabInBrowserSucceeds(t *testing.T) {
	td := newTabsDaemon(t, sharedIDTabs, 0)
	useTabsDaemon(t, td, "firefox")

	if err := tabsCloseCmd.RunE(tabsCloseCmd, []string{"7"}); err != nil {
		t.Fatalf("tabs close of the only firefox tab: %v", err)
	}
	if td.gotBrowser != "firefox" || !td.gotStrict || td.gotOptions["tabId"] != float64(7) {
		t.Fatalf("close browser=%q strict=%v options=%v; want strict firefox closeTab 7", td.gotBrowser, td.gotStrict, td.gotOptions)
	}
}

// TestTabsCloseAmbiguousIDSendsNothing pins that a shared ID without --browser is
// rejected before any command reaches the daemon.
func TestTabsCloseAmbiguousIDSendsNothing(t *testing.T) {
	td := newTabsDaemon(t, sharedIDTabs, 0)
	useTabsDaemon(t, td, "")

	err := tabsCloseCmd.RunE(tabsCloseCmd, []string{"7"})
	if err == nil || !strings.Contains(err.Error(), "--browser") {
		t.Fatalf("want ambiguity error, got %v", err)
	}
	if td.gotAction != "" {
		t.Fatalf("no command may be sent for an ambiguous close, sent %q", td.gotAction)
	}
}

func TestTabsFilterRejectsFirefoxOverCDP(t *testing.T) {
	resetTransportState()
	cdpFlag = true
	prev := tabsBrowser
	tabsBrowser = "firefox"
	defer func() {
		tabsBrowser = prev
		resetTransportState()
	}()

	_, err := tabsFilter()
	if err == nil || !strings.Contains(err.Error(), "CDP drives Chrome only") {
		t.Fatalf("want firefox-over-CDP rejection, got %v", err)
	}
}

// TestTabsListChromeAppendsFirefox pins the Chrome-filtered list: Chrome rows
// print first, then a labeled Firefox table for the tabs the filter hides.
func TestTabsListChromeAppendsFirefox(t *testing.T) {
	td := newTabsDaemon(t, sharedIDTabs, 0)
	useTabsDaemon(t, td, "chrome")

	var err error
	out := captureStdout(t, func() {
		err = tabsListCmd.RunE(tabsListCmd, nil)
	})
	if err != nil {
		t.Fatalf("tabs list --browser chrome: %v", err)
	}

	const heading = "Other tabs open in Firefox:"
	idxHeading := strings.Index(out, heading)
	if idxHeading < 0 {
		t.Fatalf("missing %q in output:\n%s", heading, out)
	}
	if strings.Count(out, heading) != 1 {
		t.Fatalf("Firefox section must appear exactly once:\n%s", out)
	}
	// Chrome row ("ch2" is unique to Chrome and not a substring of "chrome")
	// must come before the Firefox section, and the Firefox row after it.
	if i := strings.Index(out, "ch2"); i < 0 || i > idxHeading {
		t.Fatalf("chrome row must precede the Firefox section:\n%s", out)
	}
	if i := strings.Index(out, "ff"); i < idxHeading {
		t.Fatalf("firefox row must follow the Firefox section:\n%s", out)
	}
}

// TestTabsListFirefoxFilterDoesNotAppendChrome pins the asymmetry: only the
// Chrome-filtered list widens, so a Firefox filter stays Firefox-only.
func TestTabsListFirefoxFilterDoesNotAppendChrome(t *testing.T) {
	td := newTabsDaemon(t, sharedIDTabs, 0)
	useTabsDaemon(t, td, "firefox")

	var err error
	out := captureStdout(t, func() {
		err = tabsListCmd.RunE(tabsListCmd, nil)
	})
	if err != nil {
		t.Fatalf("tabs list --browser firefox: %v", err)
	}
	if strings.Contains(out, "Other tabs open in Firefox:") {
		t.Fatalf("firefox filter must not append a Firefox section:\n%s", out)
	}
	if strings.Contains(out, "ch2") {
		t.Fatalf("firefox filter must not show chrome rows:\n%s", out)
	}
	if !strings.Contains(out, "ff") {
		t.Fatalf("firefox filter must still show the firefox row:\n%s", out)
	}
}

// TestTabsListChromeJSONStaysFiltered pins that --json never widens: the
// Chrome filter returns only Chrome tabs even though Firefox tabs exist.
func TestTabsListChromeJSONStaysFiltered(t *testing.T) {
	td := newTabsDaemon(t, sharedIDTabs, 0)
	useTabsDaemon(t, td, "chrome")
	output.JSONOutput = true
	t.Cleanup(func() { output.JSONOutput = false })

	var err error
	out := captureStdout(t, func() {
		err = tabsListCmd.RunE(tabsListCmd, nil)
	})
	if err != nil {
		t.Fatalf("tabs list --browser chrome --json: %v", err)
	}
	var got []api.TabInfo
	if jerr := json.Unmarshal([]byte(out), &got); jerr != nil {
		t.Fatalf("output is not JSON: %v\n%s", jerr, out)
	}
	if len(got) != 2 {
		t.Fatalf("json must stay chrome-only (2 tabs), got %d: %+v", len(got), got)
	}
	for _, tab := range got {
		if tab.Browser != "chrome" {
			t.Fatalf("json widened to %s: %+v", tab.Browser, got)
		}
	}
}
