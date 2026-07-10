// Spike: verify CDP input + screenshot behavior on a minimized Chrome window.
// Run: htcli browser start   (visible), then: go run ./internal/cdp/spike
// Watch the window and note: does it un-minimize / take focus at each step?
package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"time"

	"github.com/u007/htcli/internal/cdp"
)

func must(err error, step string) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL %s: %v\n", step, err)
		os.Exit(1)
	}
}

func main() {
	const port = 9222
	targets, err := cdp.ListTargets(port)
	must(err, "list targets")
	if len(targets) == 0 {
		fmt.Fprintln(os.Stderr, "no page targets — run: htcli browser start")
		os.Exit(1)
	}
	t := targets[0]

	page, err := cdp.Dial(t.WebSocketDebuggerURL)
	must(err, "dial page")
	defer page.Close()

	// Install a click counter on about:blank.
	must(page.Call("Runtime.evaluate", map[string]any{
		"expression": `window.__clicks=0; document.body.style.cssText='width:100vw;height:100vh';
			document.body.addEventListener('click', e => { window.__clicks++; window.__trusted = e.isTrusted; });
			'ready'`,
		"returnByValue": true,
	}, nil), "install counter")

	// Minimize via browser-level session.
	bws, err := cdp.BrowserWSURL(port)
	must(err, "browser ws url")
	browser, err := cdp.Dial(bws)
	must(err, "dial browser")
	defer browser.Close()

	var win struct {
		WindowID int `json:"windowId"`
	}
	must(browser.Call("Browser.getWindowForTarget", map[string]any{"targetId": t.ID}, &win), "getWindowForTarget")
	must(browser.Call("Browser.setWindowBounds", map[string]any{
		"windowId": win.WindowID, "bounds": map[string]any{"windowState": "minimized"},
	}, nil), "minimize")
	fmt.Println("STEP 1: window minimized — confirm visually")
	time.Sleep(2 * time.Second)

	// (a) activateTarget while minimized — does the window come back / steal focus?
	must(page.Call("Target.activateTarget", map[string]any{"targetId": t.ID}, nil), "activateTarget")
	fmt.Println("STEP 2 (a): activateTarget sent — did the window restore or take focus? RECORD THIS")
	time.Sleep(3 * time.Second)

	// Re-minimize in case it restored, then (b) dispatch a click while minimized.
	must(browser.Call("Browser.setWindowBounds", map[string]any{
		"windowId": win.WindowID, "bounds": map[string]any{"windowState": "minimized"},
	}, nil), "re-minimize")
	time.Sleep(time.Second)
	for _, evtType := range []string{"mousePressed", "mouseReleased"} {
		must(page.Call("Input.dispatchMouseEvent", map[string]any{
			"type": evtType, "x": 100, "y": 100, "button": "left", "clickCount": 1, "buttons": 1,
		}, nil), "dispatch "+evtType)
	}
	var clicks struct {
		Result struct {
			Value int `json:"value"`
		} `json:"result"`
	}
	must(page.Call("Runtime.evaluate", map[string]any{
		"expression": "window.__clicks", "returnByValue": true,
	}, &clicks), "read counter")
	fmt.Printf("STEP 3 (b): clicks registered while minimized = %d (want 1)\n", clicks.Result.Value)

	// (c) screenshot while minimized.
	var shot struct {
		Data string `json:"data"`
	}
	err = page.Call("Page.captureScreenshot", map[string]any{"format": "png"}, &shot)
	if err != nil {
		fmt.Printf("STEP 4 (c): screenshot FAILED while minimized: %v\n", err)
	} else {
		raw, _ := base64.StdEncoding.DecodeString(shot.Data)
		fmt.Printf("STEP 4 (c): screenshot returned %d bytes while minimized\n", len(raw))
	}

	// Restore.
	must(browser.Call("Browser.setWindowBounds", map[string]any{
		"windowId": win.WindowID, "bounds": map[string]any{"windowState": "normal"},
	}, nil), "restore")
	fmt.Println("done — restore confirmed; record all observations in docs/superpowers/spikes/2026-07-10-cdp-minimized-window.md")
}
