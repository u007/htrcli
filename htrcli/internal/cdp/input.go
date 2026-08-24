package cdp

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/u007/htrcli/internal/api"
)

const (
	defaultDragSteps = 5
	maxDragSteps     = 100
	maxDragDelayMs   = 2000
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

// resolveMouseCoords returns viewport coords: literal xy= if present, else via prepareClick.
func resolveMouseCoords(s *Session, sel *api.TargetSelector) (float64, float64, error) {
	if sel != nil && sel.X != nil && sel.Y != nil {
		return *sel.X, *sel.Y, nil
	}
	prep, err := ExecDOM(s, api.Command{ID: "prep", Action: "prepareClick", Target: sel})
	if err != nil {
		return 0, 0, err
	}
	if !prep.Success {
		return 0, 0, fmt.Errorf("prepare failed: %s", prep.Error)
	}
	var coords struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	}
	data, err := json.Marshal(prep.Data)
	if err != nil {
		return 0, 0, fmt.Errorf("re-encoding prepare data: %w", err)
	}
	if err := json.Unmarshal(data, &coords); err != nil {
		return 0, 0, fmt.Errorf("prepareClick returned no coordinates: %w", err)
	}
	return coords.X, coords.Y, nil
}

// dispatchMouseAt is a helper for MouseDown/Up/Move: prepareClick + activate + single mouse event.
func dispatchMouseAt(s *Session, targetID string, sel *api.TargetSelector, typ, button string, buttons, clickCount int) error {
	x, y, err := resolveMouseCoords(s, sel)
	if err != nil {
		return err
	}
	if err := s.Call("Target.activateTarget", map[string]any{"targetId": targetID}, nil); err != nil {
		return fmt.Errorf("activating target: %w", err)
	}
	if err := s.Call("Input.dispatchMouseEvent", map[string]any{
		"type": typ, "x": x, "y": y,
		"button": button, "clickCount": clickCount, "buttons": buttons, "modifiers": 0,
	}, nil); err != nil {
		return fmt.Errorf("dispatch %s: %w", typ, err)
	}
	return nil
}

func MouseDown(s *Session, targetID string, sel *api.TargetSelector) error {
	return dispatchMouseAt(s, targetID, sel, "mousePressed", "left", 1, 1)
}

func MouseUp(s *Session, targetID string, sel *api.TargetSelector) error {
	return dispatchMouseAt(s, targetID, sel, "mouseReleased", "left", 0, 1)
}

func MouseMove(s *Session, targetID string, sel *api.TargetSelector) error {
	return dispatchMouseAt(s, targetID, sel, "mouseMoved", "none", 0, 0)
}

func Drag(s *Session, targetID string, src, dst *api.TargetSelector, steps int, delayMs int) error {
	steps = normalizeDragSteps(steps)
	delayMs = normalizeDragDelay(delayMs)
	sx, sy, err := resolveMouseCoords(s, src)
	if err != nil {
		return fmt.Errorf("resolve source: %w", err)
	}
	dx, dy, err := resolveMouseCoords(s, dst)
	if err != nil {
		return fmt.Errorf("resolve dest: %w", err)
	}
	var sc = struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	}{X: sx, Y: sy}
	var dc = struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	}{X: dx, Y: dy}
	if err := s.Call("Target.activateTarget", map[string]any{"targetId": targetID}, nil); err != nil {
		return fmt.Errorf("activating target: %w", err)
	}
	if err := s.Call("Input.dispatchMouseEvent", map[string]any{
		"type": "mousePressed", "x": sc.X, "y": sc.Y,
		"button": "left", "clickCount": 1, "buttons": 1, "modifiers": 0,
	}, nil); err != nil {
		return fmt.Errorf("dispatch mousePressed: %w", err)
	}
	for i := 1; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := sc.X + (dc.X-sc.X)*t
		y := sc.Y + (dc.Y-sc.Y)*t
		if err := s.Call("Input.dispatchMouseEvent", map[string]any{
			"type": "mouseMoved", "x": x, "y": y,
			"button": "left", "clickCount": 0, "buttons": 1, "modifiers": 0,
		}, nil); err != nil {
			return fmt.Errorf("dispatch mouseMoved: %w", err)
		}
		if delayMs > 0 {
			time.Sleep(time.Duration(delayMs) * time.Millisecond)
		}
	}
	if err := s.Call("Input.dispatchMouseEvent", map[string]any{
		"type": "mouseReleased", "x": dc.X, "y": dc.Y,
		"button": "left", "clickCount": 1, "buttons": 0, "modifiers": 0,
	}, nil); err != nil {
		return fmt.Errorf("dispatch mouseReleased: %w", err)
	}
	return nil
}

func normalizeDragSteps(steps int) int {
	if steps < 1 {
		return defaultDragSteps
	}
	if steps > maxDragSteps {
		return maxDragSteps
	}
	return steps
}

func normalizeDragDelay(delayMs int) int {
	if delayMs < 0 {
		return 0
	}
	if delayMs > maxDragDelayMs {
		return maxDragDelayMs
	}
	return delayMs
}

// parseKey returns key string and modifiers bitmask from a spec like "Ctrl+Shift+a".
func parseKey(keySpec string) (string, int, error) {
	if strings.TrimSpace(keySpec) == "" {
		return "", 0, fmt.Errorf("key cannot be empty")
	}
	parts := strings.Split(keySpec, "+")
	key := strings.TrimSpace(parts[len(parts)-1])
	if key == "" {
		return "", 0, fmt.Errorf("key cannot be empty in %q", keySpec)
	}
	modifiers := 0
	for _, mod := range parts[:len(parts)-1] {
		switch strings.ToLower(strings.TrimSpace(mod)) {
		case "alt":
			modifiers |= 1
		case "ctrl", "control":
			modifiers |= 2
		case "meta", "cmd":
			modifiers |= 4
		case "shift":
			modifiers |= 8
		default:
			return "", 0, fmt.Errorf("unknown modifier %q in %q", mod, keySpec)
		}
	}
	return key, modifiers, nil
}

func KeyDown(s *Session, targetID string, keySpec string) error {
	if err := s.Call("Target.activateTarget", map[string]any{"targetId": targetID}, nil); err != nil {
		return fmt.Errorf("activating target: %w", err)
	}
	key, modifiers, err := parseKey(keySpec)
	if err != nil {
		return err
	}
	params := map[string]any{"type": "keyDown", "key": key, "modifiers": modifiers}
	if named, ok := namedKeys[key]; ok {
		params["windowsVirtualKeyCode"] = named.code
		params["code"] = named.domCode
		if named.text != "" {
			params["text"] = named.text
		}
	} else if len([]rune(key)) == 1 {
		params["text"] = key
	}
	if err := s.Call("Input.dispatchKeyEvent", params, nil); err != nil {
		return fmt.Errorf("dispatch keyDown: %w", err)
	}
	return nil
}

func KeyUp(s *Session, targetID string, keySpec string) error {
	if err := s.Call("Target.activateTarget", map[string]any{"targetId": targetID}, nil); err != nil {
		return fmt.Errorf("activating target: %w", err)
	}
	key, modifiers, err := parseKey(keySpec)
	if err != nil {
		return err
	}
	params := map[string]any{"type": "keyUp", "key": key, "modifiers": modifiers}
	if named, ok := namedKeys[key]; ok {
		params["windowsVirtualKeyCode"] = named.code
		params["code"] = named.domCode
		// text only on keyDown, not keyUp
	} else if len([]rune(key)) == 1 {
		// no text on keyUp
	}
	if err := s.Call("Input.dispatchKeyEvent", params, nil); err != nil {
		return fmt.Errorf("dispatch keyUp: %w", err)
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
