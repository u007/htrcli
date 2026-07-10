// Package cdp is a minimal Chrome DevTools Protocol client for the htcli
// --cdp transport. It talks only to 127.0.0.1: /json discovery over HTTP,
// then per-target and browser-level WebSocket sessions.
package cdp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// ErrNotRunning means nothing answers on the debugging port.
var ErrNotRunning = errors.New("CDP browser not running — start it with: htcli browser start")

// Target is one entry from GET /json.
type Target struct {
	ID                   string `json:"id"`
	Type                 string `json:"type"`
	Title                string `json:"title"`
	URL                  string `json:"url"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

var httpClient = &http.Client{Timeout: 5 * time.Second}

func getJSON(port int, path string, out any) error {
	// 127.0.0.1 literal: Chrome's DNS-rebinding guard rejects non-IP Hosts.
	resp, err := httpClient.Get(fmt.Sprintf("http://127.0.0.1:%d%s", port, path))
	if err != nil {
		var netErr *net.OpError
		if errors.As(err, &netErr) {
			return fmt.Errorf("%w (port %d): %v", ErrNotRunning, port, err)
		}
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: HTTP %d: %s", path, resp.StatusCode, body)
	}
	return json.Unmarshal(body, out)
}

// ListTargets returns page-type targets from GET /json.
func ListTargets(port int) ([]Target, error) {
	var all []Target
	if err := getJSON(port, "/json", &all); err != nil {
		return nil, err
	}
	pages := make([]Target, 0, len(all))
	for _, t := range all {
		if t.Type == "page" {
			pages = append(pages, t)
		}
	}
	return pages, nil
}

// BrowserWSURL returns the browser-level WebSocket endpoint from /json/version
// (required for Browser.* domain methods).
func BrowserWSURL(port int) (string, error) {
	var v struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := getJSON(port, "/json/version", &v); err != nil {
		return "", err
	}
	if v.WebSocketDebuggerURL == "" {
		return "", errors.New("/json/version returned no webSocketDebuggerUrl")
	}
	return v.WebSocketDebuggerURL, nil
}
