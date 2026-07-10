package cdp

import (
	"encoding/base64"
	"fmt"
	"time"
)

// Navigate loads a URL and waits for Page.loadEventFired, bounded by
// timeoutMs (the global --timeout). SPA route changes never fire load —
// same semantics as the extension transport's navigate wait.
func Navigate(s *Session, url string, timeoutMs int) error {
	if err := s.Call("Page.enable", nil, nil); err != nil {
		return fmt.Errorf("Page.enable: %w", err)
	}
	var nav struct {
		ErrorText string `json:"errorText"`
	}
	if err := s.Call("Page.navigate", map[string]any{"url": url}, &nav); err != nil {
		return err
	}
	if nav.ErrorText != "" {
		return fmt.Errorf("navigation failed: %s", nav.ErrorText)
	}
	if _, err := s.WaitEvent("Page.loadEventFired", time.Duration(timeoutMs)*time.Millisecond); err != nil {
		return fmt.Errorf("page did not finish loading: %w", err)
	}
	return nil
}

// Screenshot captures the page as PNG.
func Screenshot(s *Session) ([]byte, error) {
	var res struct {
		Data string `json:"data"`
	}
	if err := s.Call("Page.captureScreenshot", map[string]any{"format": "png"}, &res); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(res.Data)
}
