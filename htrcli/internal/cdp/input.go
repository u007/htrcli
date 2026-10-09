package cdp

import (
	"encoding/json"
	"errors"
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
	// prepareClick ran inside the --frame (if any): shift to top viewport.
	if coords.X, coords.Y, err = toTopViewport(s, coords.X, coords.Y); err != nil {
		return err
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
	x, y, err := resolveLocalCoords(s, sel)
	if err != nil {
		return 0, 0, err
	}
	if sel != nil && sel.X != nil && sel.Y != nil {
		return x, y, nil
	}
	return toTopViewport(s, x, y)
}

// resolveLocalCoords returns the point in the session's own (frame-local when
// --frame is set) viewport, before any top-viewport conversion.
func resolveLocalCoords(s *Session, sel *api.TargetSelector) (float64, float64, error) {
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

// dragPointsFn resolves BOTH drag endpoints in a single page-side evaluation
// and returns them as {a:{x,y}, b:{x,y}} in the session realm's coordinates.
//
// Resolving the endpoints one at a time (the old resolveMouseCoords per
// endpoint) reads the source point, then prepareClick's unconditional
// scrollIntoView(center) for the destination moves the page — the press then
// lands where the source USED to be, on any scrollable page, frame or not.
// So: run every scroll first (both prepares), then re-read BOTH centres
// without scrolling, then — if a prepared endpoint fell out of view — fit
// both into the viewport with ONE scroll and re-read again. A throw here
// surfaces as the drag error in Go.
//
// An endpoint is one of three things:
//   - a selector object: prepared by the bundle's prepareClick (waits
//     actionable + centers) and re-read via getBoundingBox;
//   - an Element (an @eN ref, passed in by handle): centered with
//     scrollIntoView and re-read via getBoundingClientRect — the same
//     scroll-then-read discipline, so a ref is never a stale pixel;
//   - literal {x, y}: raw viewport pixels, never prepared, never re-read
//     (scrolling cannot move a pixel coordinate, only the content under it)
//     and never fit-scrolled — they mean "these pixels at dispatch time".
const dragPointsFn = `async (sa, sb) => {` + "\n" + clipAncestorsJS + `
	if (typeof window.__htrcliDom !== "object") return {__htrcliNeedBundle: true};
	const dom = window.__htrcliDom;
	const lit = (t) => t && typeof t.x === "number" && typeof t.y === "number";
	const isEl = (t) => t instanceof Element;
	const exec = async (action, target, id) => {
		const r = await dom.exec({id: id, action: action, target: target});
		if (!r.success) throw new Error(r.error || (action + " failed"));
		return r.data;
	};
	const prep = async (t, id) => {
		if (lit(t)) return;
		if (isEl(t)) {
			if (!t.isConnected) throw new Error("drag endpoint " + id + ": the referenced element is no longer in the document");
			t.scrollIntoView({behavior: "instant", block: "center", inline: "center"});
			return;
		}
		await exec("prepareClick", t, id); // waits actionable + centers (scrolls)
	};
	const read = async (t, id) => {
		if (lit(t)) return {x: t.x, y: t.y};
		if (isEl(t)) {
			const r = t.getBoundingClientRect();
			return {x: r.left + r.width / 2, y: r.top + r.height / 2};
		}
		const box = await exec("getBoundingBox", t, id); // no scroll, no wait
		if (!box) throw new Error("drag endpoint is no longer in the document after it was prepared");
		return {x: box.x + box.width / 2, y: box.y + box.height / 2};
	};
	// Each selector endpoint is tracked with its element: the element is the one
	// under the point once the endpoint is prepared, and the point is moved by the
	// element's own movement, so it follows any scroll of an ancestor container.
	const track = (t, p) => {
		if (lit(t)) return null;
		const el = isEl(t) ? t : document.elementFromPoint(p.x, p.y);
		if (!el) return null;
		const r = el.getBoundingClientRect();
		return {el: el, x: p.x, y: p.y, rx: r.left, ry: r.top};
	};
	const live = (tr) => {
		const r = tr.el.getBoundingClientRect();
		return {x: tr.x + r.left - tr.rx, y: tr.y + r.top - tr.ry};
	};
	await prep(sa, "drag-prep-a");
	const trA = track(sa, await read(sa, "drag-track-a"));
	await prep(sb, "drag-prep-b");
	const trB = track(sb, await read(sb, "drag-track-b"));
	// Both prepares have scrolled; only NOW are coordinates meaningful.
	let a = await read(sa, "drag-read-a");
	let b = await read(sb, "drag-read-b");
	// Nested overflow containers first, whether or not the window is short of
	// room: a point inside the window can still be clipped by a scroll
	// container. Both points are passed, so a shared container keeps both.
	const liveAll = () => [trA, trB].filter((tr) => tr !== null).map(live);
	for (const tr of [trA, trB]) {
		if (tr !== null) scrollClippingAncestors(tr.el, liveAll);
	}
	a = await read(sa, "drag-clip-a");
	b = await read(sb, "drag-clip-b");
	// Recomputed after every re-read: the fit-scroll check below must look at
	// the CURRENT points, not the ones captured before the scroll.
	const prepared = () => [lit(sa) ? null : a, lit(sb) ? null : b].filter((p) => p !== null);
	const inVP = (p) => p.x >= 0 && p.y >= 0 && p.x < innerWidth && p.y < innerHeight;
	if (prepared().some((p) => !inVP(p))) {
		// One fit-scroll to reveal every prepared endpoint, if their span fits.
		const xs = prepared().map((p) => p.x), ys = prepared().map((p) => p.y);
		const minX = Math.min(...xs), maxX = Math.max(...xs);
		const minY = Math.min(...ys), maxY = Math.max(...ys);
		if (maxX - minX >= innerWidth || maxY - minY >= innerHeight) {
			throw new Error("drag source (" + Math.round(a.x) + "," + Math.round(a.y) +
				") and destination (" + Math.round(b.x) + "," + Math.round(b.y) +
				") cannot both be seen at once: they span more than the " + innerWidth + "x" +
				innerHeight + " viewport; bring both into view before dragging");
		}
		window.scrollBy({
			left: (minX + maxX) / 2 - innerWidth / 2,
			top: (minY + maxY) / 2 - innerHeight / 2,
			behavior: "instant",
		});
		a = await read(sa, "drag-recheck-a");
		b = await read(sb, "drag-recheck-b");
		if (prepared().some((p) => !inVP(p))) {
			throw new Error("drag endpoints (" + Math.round(a.x) + "," + Math.round(a.y) + "),(" +
				Math.round(b.x) + "," + Math.round(b.y) + ") are still outside the " +
				innerWidth + "x" + innerHeight + " viewport after scrolling; not draggable");
		}
	}
	return {a: a, b: b};
}`

// DragEndpoint is one end of a drag: a selector (CSS, name=, xy= …) or a
// persistent @eN ref already resolved by the command layer to its
// backendNodeId (BackendNodeID != 0, Sel ignored). The cdp package never
// reads the ref store; it only turns the node into a live element handle.
type DragEndpoint struct {
	Sel           *api.TargetSelector
	BackendNodeID int64
}

func (e DragEndpoint) literal() bool {
	return e.BackendNodeID == 0 && e.Sel != nil && e.Sel.X != nil && e.Sel.Y != nil
}

// callArg is one Runtime.callFunctionOn argument: a JSON value, or a handle
// to a page object (an element resolved from a backendNodeId).
type callArg struct {
	Value    any    `json:"value,omitempty"`
	ObjectID string `json:"objectId,omitempty"`
}

// callFunctionOn invokes fnSrc(args...) with `this` = objectID and returns
// its JSON value. It is how a page-side function receives an ELEMENT (an
// @eN ref) rather than a selector: Runtime.evaluate can only ship JSON.
// Exceptions become a pageError, as with evaluate.
func callFunctionOn(s *Session, objectID, fnSrc string, args []callArg) (json.RawMessage, error) {
	var res evalResult
	if err := s.Call("Runtime.callFunctionOn", map[string]any{
		"functionDeclaration": fnSrc,
		"objectId":            objectID,
		"arguments":           args,
		"returnByValue":       true,
		"awaitPromise":        true,
	}, &res); err != nil {
		return nil, err
	}
	if res.ExceptionDetails != nil {
		perr := &pageError{Message: res.ExceptionDetails.Text}
		if exc := res.ExceptionDetails.Exception; exc != nil {
			perr.ClassName = exc.ClassName
			perr.Message = exc.Description
		}
		return nil, perr
	}
	return res.Result.Value, nil
}

// resolveNodeObject turns a backendNodeId into a RemoteObjectId for the
// element, so it can be passed to a page function by handle.
func resolveNodeObject(s *Session, backendNodeID int64) (string, error) {
	if err := s.Call("DOM.enable", nil, nil); err != nil {
		return "", fmt.Errorf("DOM.enable: %w", err)
	}
	var res struct {
		Object struct {
			ObjectID string `json:"objectId"`
			Subtype  string `json:"subtype"`
		} `json:"object"`
	}
	if err := s.Call("DOM.resolveNode", map[string]any{"backendNodeId": backendNodeID}, &res); err != nil {
		return "", fmt.Errorf("DOM.resolveNode for backendNodeId %d: %w", backendNodeID, err)
	}
	if res.Object.ObjectID == "" || res.Object.Subtype == "null" {
		return "", fmt.Errorf("backendNodeId %d no longer resolves to an element (stale ref — re-run find --ref)", backendNodeID)
	}
	return res.Object.ObjectID, nil
}

// resolveDragPoints resolves BOTH drag endpoints in one page-side pass (see
// dragPointsFn) and returns top-viewport coordinates: when s.frame is set,
// the prepared (non-literal) points come back frame-local and are converted
// together in ONE toTopViewportPts call, so no scroll between two
// conversions can invalidate the first point. Literal xy= endpoints are
// already top-viewport pixels and are never converted.
func resolveDragPoints(s *Session, src, dst DragEndpoint) (a, b [2]float64, err error) {
	if src.literal() && dst.literal() {
		// Pure pixel drag: nothing to prepare, nothing to convert.
		return [2]float64{*src.Sel.X, *src.Sel.Y}, [2]float64{*dst.Sel.X, *dst.Sel.Y}, nil
	}
	hasRef := src.BackendNodeID != 0 || dst.BackendNodeID != 0
	if hasRef && s.frame != "" {
		return a, b, errors.New("@eN refs resolve in the top-level document and cannot be combined with --frame")
	}
	run := func() (json.RawMessage, error) {
		if !hasRef {
			return runScoped(s, dragPointsFn, src.Sel, dst.Sel)
		}
		// Refs travel as element handles, so the call goes through
		// Runtime.callFunctionOn with the first ref as `this`.
		args := make([]callArg, 2)
		this := ""
		for i, e := range []DragEndpoint{src, dst} {
			if e.BackendNodeID == 0 {
				args[i] = callArg{Value: e.Sel}
				continue
			}
			id, err := resolveNodeObject(s, e.BackendNodeID)
			if err != nil {
				return nil, err
			}
			args[i] = callArg{ObjectID: id}
			if this == "" {
				this = id
			}
		}
		return callFunctionOn(s, this, dragPointsFn, args)
	}
	raw, err := withBundle(s, domBundleSpec, run)
	if err != nil {
		return a, b, err
	}
	var pts struct {
		A struct {
			X float64 `json:"x"`
			Y float64 `json:"y"`
		} `json:"a"`
		B struct {
			X float64 `json:"x"`
			Y float64 `json:"y"`
		} `json:"b"`
	}
	if err := json.Unmarshal(raw, &pts); err != nil {
		return a, b, fmt.Errorf("decoding drag points: %w", err)
	}
	a, b = [2]float64{pts.A.X, pts.A.Y}, [2]float64{pts.B.X, pts.B.Y}
	// Only prepared points are frame-local; a literal is top-viewport already.
	var local [][2]float64
	if !src.literal() {
		local = append(local, a)
	}
	if !dst.literal() {
		local = append(local, b)
	}
	top, err := toTopViewportPts(s, local)
	if err != nil {
		return a, b, err
	}
	if !src.literal() {
		a, top = top[0], top[1:]
	}
	if !dst.literal() {
		b = top[0]
	}
	return a, b, nil
}

func Drag(s *Session, targetID string, src, dst DragEndpoint, steps int, delayMs int) error {
	steps = normalizeDragSteps(steps)
	delayMs = normalizeDragDelay(delayMs)
	// Both endpoints are resolved TOGETHER (see resolveDragPoints): resolving
	// them one after another let the second prepareClick centering scroll
	// invalidate the first point, so the press landed where the source USED
	// to be. The top-viewport conversion is batched for the same reason.
	sc, dc, err := resolveDragPoints(s, src, dst)
	if err != nil {
		return fmt.Errorf("resolve drag points: %w", err)
	}
	if err := s.Call("Target.activateTarget", map[string]any{"targetId": targetID}, nil); err != nil {
		return fmt.Errorf("activating target: %w", err)
	}
	if err := s.Call("Input.dispatchMouseEvent", map[string]any{
		"type": "mousePressed", "x": sc[0], "y": sc[1],
		"button": "left", "clickCount": 1, "buttons": 1, "modifiers": 0,
	}, nil); err != nil {
		return fmt.Errorf("dispatch mousePressed: %w", err)
	}
	for i := 1; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := sc[0] + (dc[0]-sc[0])*t
		y := sc[1] + (dc[1]-sc[1])*t
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
		"type": "mouseReleased", "x": dc[0], "y": dc[1],
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
