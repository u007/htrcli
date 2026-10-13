package cdp

import (
	"errors"
)

// windowIDFor resolves the window owning targetID (empty = first page target)
// over the browser-level session s.
func windowIDFor(s *Session, port int, targetID string) (int, error) {
	if targetID == "" {
		targets, err := ListTargets(port)
		if err != nil {
			return 0, err
		}
		if len(targets) == 0 {
			return 0, errors.New("no page targets open")
		}
		targetID = targets[0].ID
	}
	var win struct {
		WindowID int `json:"windowId"`
	}
	if err := s.Call("Browser.getWindowForTarget", map[string]any{"targetId": targetID}, &win); err != nil {
		return 0, err
	}
	return win.WindowID, nil
}

// SetWindowState minimizes or restores the window owning targetID
// (empty targetID = first page target). state: "minimized" | "normal".
func SetWindowState(port int, targetID string, state string) error {
	return withBrowserSession(port, func(s *Session) error {
		windowID, err := windowIDFor(s, port, targetID)
		if err != nil {
			return err
		}
		return s.Call("Browser.setWindowBounds", map[string]any{
			"windowId": windowID,
			"bounds":   map[string]any{"windowState": state},
		}, nil)
	})
}

// GetWindowState reads the live window state via Browser.getWindowBounds.
func GetWindowState(port int, targetID string) (string, error) {
	var state string
	err := withBrowserSession(port, func(s *Session) error {
		windowID, err := windowIDFor(s, port, targetID)
		if err != nil {
			return err
		}
		var res struct {
			Bounds struct {
				WindowState string `json:"windowState"`
			} `json:"bounds"`
		}
		if err := s.Call("Browser.getWindowBounds", map[string]any{"windowId": windowID}, &res); err != nil {
			return err
		}
		state = res.Bounds.WindowState
		return nil
	})
	if err != nil {
		return "", err
	}
	return state, nil
}
