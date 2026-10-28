package cdp

import (
	"fmt"
	"math"
)

// Emulation is the device and media emulation persisted per CDP port and
// re-applied on every session open. CDP Emulation overrides reset when the
// DevTools session detaches, so a one-off call would not survive the command.
// Zero fields mean "no override".
type Emulation struct {
	Width       int     `json:"width,omitempty"`
	Height      int     `json:"height,omitempty"`
	Scale       float64 `json:"scale,omitempty"`
	Mobile      bool    `json:"mobile,omitempty"`
	ColorScheme string  `json:"colorScheme,omitempty"`
	UserAgent   string  `json:"userAgent,omitempty"`
}

// Validate rejects an Emulation that CDP would ignore or misapply. A zero
// device scale means "no override" to CDP, so a viewport without a positive
// scale is an error, not a silent no-op.
func (e Emulation) Validate() error {
	if e.Width < 0 || e.Height < 0 || (e.Width == 0) != (e.Height == 0) {
		return fmt.Errorf("viewport needs a positive width and height, got %dx%d", e.Width, e.Height)
	}
	if e.Width > 0 && (math.IsNaN(e.Scale) || math.IsInf(e.Scale, 0) || e.Scale <= 0) {
		return fmt.Errorf("device scale must be a positive finite number, got %v", e.Scale)
	}
	switch e.ColorScheme {
	case "", "light", "dark":
	default:
		return fmt.Errorf("color scheme must be light, dark, or empty, got %q", e.ColorScheme)
	}
	return nil
}

// emulationCall is one CDP method call. Params is nil for calls without params.
type emulationCall struct {
	Method string
	Params any
}

// emulationCalls returns the CDP calls that realize e, in apply order.
func emulationCalls(e Emulation) []emulationCall {
	var calls []emulationCall
	if e.Width > 0 {
		calls = append(calls, emulationCall{"Emulation.setDeviceMetricsOverride", map[string]any{
			"width":             e.Width,
			"height":            e.Height,
			"deviceScaleFactor": e.Scale,
			"mobile":            e.Mobile,
		}})
		if e.Mobile {
			calls = append(calls, emulationCall{"Emulation.setTouchEmulationEnabled", map[string]any{"enabled": true}})
		}
	}
	if e.ColorScheme != "" {
		calls = append(calls, emulationCall{"Emulation.setEmulatedMedia", map[string]any{
			"features": []map[string]string{{"name": "prefers-color-scheme", "value": e.ColorScheme}},
		}})
	}
	if e.UserAgent != "" {
		calls = append(calls, emulationCall{"Network.setUserAgentOverride", map[string]any{"userAgent": e.UserAgent}})
	}
	return calls
}

// ApplyEmulation sends the CDP calls for e on s. An empty Emulation sends
// nothing. The first CDP error is returned unchanged and stops the sequence.
func ApplyEmulation(s *Session, e Emulation) error {
	if err := e.Validate(); err != nil {
		return err
	}
	for _, c := range emulationCalls(e) {
		if err := s.Call(c.Method, c.Params, nil); err != nil {
			return err
		}
	}
	return nil
}

// ClearEmulation removes the device metrics, media, and user-agent overrides
// on s. Used by `emulate reset` on a live session.
func ClearEmulation(s *Session) error {
	calls := []emulationCall{
		{"Emulation.clearDeviceMetricsOverride", nil},
		{"Emulation.setEmulatedMedia", nil},
		{"Network.setUserAgentOverride", map[string]any{"userAgent": ""}},
	}
	for _, c := range calls {
		if err := s.Call(c.Method, c.Params, nil); err != nil {
			return err
		}
	}
	return nil
}
