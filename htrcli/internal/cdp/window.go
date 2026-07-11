package cdp

import (
	"errors"
	"fmt"
)

// browserSessionFor resolves the target (empty = first page) and opens a
// browser-level session, returning the windowId for the target.
func browserSessionFor(port int, targetID string) (*Session, int, error) {
	if targetID == "" {
		targets, err := ListTargets(port)
		if err != nil {
			return nil, 0, err
		}
		if len(targets) == 0 {
			return nil, 0, errors.New("no page targets open")
		}
		targetID = targets[0].ID
	}
	wsURL, err := BrowserWSURL(port)
	if err != nil {
		return nil, 0, err
	}
	s, err := Dial(wsURL)
	if err != nil {
		return nil, 0, err
	}
	var win struct {
		WindowID int `json:"windowId"`
	}
	if err := s.Call("Browser.getWindowForTarget", map[string]any{"targetId": targetID}, &win); err != nil {
		if cerr := s.Close(); cerr != nil {
			fmt.Printf("[htrcli] closing browser session after error: %v\n", cerr)
		}
		return nil, 0, err
	}
	return s, win.WindowID, nil
}

// SetWindowState minimizes or restores the window owning targetID
// (empty targetID = first page target). state: "minimized" | "normal".
func SetWindowState(port int, targetID string, state string) error {
	s, windowID, err := browserSessionFor(port, targetID)
	if err != nil {
		return err
	}
	defer s.Close()
	return s.Call("Browser.setWindowBounds", map[string]any{
		"windowId": windowID,
		"bounds":   map[string]any{"windowState": state},
	}, nil)
}

// GetWindowState reads the live window state via Browser.getWindowBounds.
func GetWindowState(port int, targetID string) (string, error) {
	s, windowID, err := browserSessionFor(port, targetID)
	if err != nil {
		return "", err
	}
	defer s.Close()
	var res struct {
		Bounds struct {
			WindowState string `json:"windowState"`
		} `json:"bounds"`
	}
	if err := s.Call("Browser.getWindowBounds", map[string]any{"windowId": windowID}, &res); err != nil {
		return "", err
	}
	return res.Bounds.WindowState, nil
}
