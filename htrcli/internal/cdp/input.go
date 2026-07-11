package cdp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/u007/htrcli/internal/api"
)

// Click prepares the element via the bundle (wait actionable + scroll +
// viewport-center coords — same prepareClick the extension CDP path uses),
// activates the target, then dispatches trusted mouse events.
func Click(s *Session, targetID string, sel *api.TargetSelector, action string) error {
	prep, err := ExecDOM(s, api.Command{ID: "prep", Action: "prepareClick", Target: sel})
	if err != nil {
		return err
	}
	if !prep.Success {
		return fmt.Errorf("prepare failed: %s", prep.Error)
	}
	var coords struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	}
	data, err := json.Marshal(prep.Data)
	if err != nil {
		return fmt.Errorf("re-encoding prepare data: %w", err)
	}
	if err := json.Unmarshal(data, &coords); err != nil {
		return fmt.Errorf("prepareClick returned no coordinates: %w", err)
	}

	// CDP input is dropped on unrendered tabs — activate first. The window
	// itself may still be minimized/backgrounded (spike-dependent; headless
	// always works).
	if err := s.Call("Target.activateTarget", map[string]any{"targetId": targetID}, nil); err != nil {
		return fmt.Errorf("activating target: %w", err)
	}

	button := "left"
	if action == "rightclick" {
		button = "right"
	}
	clickCount := 1
	if action == "dblclick" {
		clickCount = 2
	}
	buttons := 1
	if button == "right" {
		buttons = 2
	}
	for _, typ := range []string{"mousePressed", "mouseReleased"} {
		if err := s.Call("Input.dispatchMouseEvent", map[string]any{
			"type": typ, "x": coords.X, "y": coords.Y,
			"button": button, "clickCount": clickCount, "buttons": buttons, "modifiers": 0,
		}, nil); err != nil {
			return fmt.Errorf("dispatch %s: %w", typ, err)
		}
	}
	return nil
}

// Press activates the target, then dispatches a trusted key press to
// whatever holds focus. Key specs: "Enter", "Tab", "Ctrl+a", "Shift+Tab".
// Modifier bitmask matches CDP: Alt=1, Ctrl=2, Meta=4, Shift=8. Named keys
// carry windowsVirtualKeyCode plus their control-char text (Enter="\r",
// Tab="\t") so keypress/char handlers fire; single printable chars carry
// themselves as text. Mirrors src/background/cdpInput.ts dispatchCdpKey.
func Press(s *Session, targetID string, keySpec string) error {
	// CDP input is dropped on unrendered tabs — activate first, same as Click.
	if err := s.Call("Target.activateTarget", map[string]any{"targetId": targetID}, nil); err != nil {
		return fmt.Errorf("activating target: %w", err)
	}
	parts := strings.Split(keySpec, "+")
	key := parts[len(parts)-1]
	modifiers := 0
	for _, mod := range parts[:len(parts)-1] {
		switch strings.ToLower(mod) {
		case "alt":
			modifiers |= 1
		case "ctrl", "control":
			modifiers |= 2
		case "meta", "cmd":
			modifiers |= 4
		case "shift":
			modifiers |= 8
		default:
			return fmt.Errorf("unknown modifier %q in %q", mod, keySpec)
		}
	}
	params := map[string]any{"key": key, "modifiers": modifiers}
	if named, ok := namedKeys[key]; ok {
		params["windowsVirtualKeyCode"] = named.code
		params["code"] = named.domCode
		if named.text != "" {
			params["text"] = named.text
		}
	} else if len([]rune(key)) == 1 {
		params["text"] = key
	}
	for _, typ := range []string{"keyDown", "keyUp"} {
		p := map[string]any{"type": typ}
		for k, v := range params {
			if typ == "keyUp" && k == "text" {
				continue // text only on keyDown
			}
			p[k] = v
		}
		if err := s.Call("Input.dispatchKeyEvent", p, nil); err != nil {
			return fmt.Errorf("dispatch %s: %w", typ, err)
		}
	}
	return nil
}

// namedKeys mirrors the descriptor table in src/utils/keyMap.ts: virtual
// keycode, DOM `code`, and — for Enter/Tab — the control-char text that makes
// char events (form submit, focus move) actually fire.
var namedKeys = map[string]struct {
	code    int
	domCode string
	text    string
}{
	"Enter":      {13, "Enter", "\r"},
	"Tab":        {9, "Tab", "\t"},
	"Escape":     {27, "Escape", ""},
	"Backspace":  {8, "Backspace", ""},
	"Delete":     {46, "Delete", ""},
	"ArrowLeft":  {37, "ArrowLeft", ""},
	"ArrowUp":    {38, "ArrowUp", ""},
	"ArrowRight": {39, "ArrowRight", ""},
	"ArrowDown":  {40, "ArrowDown", ""},
	"Home":       {36, "Home", ""},
	"End":        {35, "End", ""},
	"PageUp":     {33, "PageUp", ""},
	"PageDown":   {34, "PageDown", ""},
}
