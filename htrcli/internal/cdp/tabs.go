package cdp

import (
	"errors"
	"fmt"
)

// withBrowserSession opens a browser-level session on port, runs fn, and
// closes the session. Target.* methods are only served on the browser endpoint.
func withBrowserSession(port int, fn func(s *Session) error) error {
	wsURL, err := BrowserWSURL(port)
	if err != nil {
		return err
	}
	s, err := Dial(wsURL)
	if err != nil {
		return err
	}
	defer s.Close()
	return fn(s)
}

// CreateTarget opens url in a new page target and returns its target ID.
func CreateTarget(port int, url string) (string, error) {
	var targetID string
	err := withBrowserSession(port, func(s *Session) error {
		var res struct {
			TargetID string `json:"targetId"`
		}
		if err := s.Call("Target.createTarget", map[string]any{"url": url, "background": true}, &res); err != nil {
			return err
		}
		targetID = res.TargetID
		return nil
	})
	if err != nil {
		return "", err
	}
	if targetID == "" {
		return "", errors.New("Target.createTarget returned no targetId")
	}
	return targetID, nil
}

// CloseTarget closes the page target targetID.
func CloseTarget(port int, targetID string) error {
	return withBrowserSession(port, func(s *Session) error {
		var res struct {
			Success bool `json:"success"`
		}
		if err := s.Call("Target.closeTarget", map[string]any{"targetId": targetID}, &res); err != nil {
			return err
		}
		if !res.Success {
			return fmt.Errorf("Target.closeTarget reported failure for %s", targetID)
		}
		return nil
	})
}
