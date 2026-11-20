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

// CDP modifier bits, as carried on Input.dispatchKeyEvent.
const (
	modifierAlt = 1 << iota
	modifierCtrl
	modifierMeta
	modifierShift
)

// keyDescriptor is the field set one logical key contributes to a CDP key event.
type keyDescriptor struct {
	vk        int    // windowsVirtualKeyCode
	domCode   string // KeyboardEvent.code
	text      string // character the key produces
	printable bool   // one character: Ctrl/Alt/Meta suppress its text
}

// describeKey resolves a named key ("Enter", "F5", "Shift") or a single
// printable character to its descriptor.
func describeKey(key string) (keyDescriptor, bool) {
	if d, ok := namedKeys[key]; ok {
		return d, true
	}
	runes := []rune(key)
	if len(runes) != 1 {
		return keyDescriptor{}, false
	}
	return describeChar(runes[0])
}

// describeChar maps a printable ASCII character to its physical key, so a
// single-character press carries `code` and the VK that apps read — not just
// `key` and `text`.
func describeChar(r rune) (keyDescriptor, bool) {
	switch {
	case r >= 'a' && r <= 'z':
		return keyDescriptor{vk: int(r-'a') + 65, domCode: "Key" + strings.ToUpper(string(r)), text: string(r), printable: true}, true
	case r >= 'A' && r <= 'Z':
		return keyDescriptor{vk: int(r), domCode: "Key" + string(r), text: string(r), printable: true}, true
	case r >= '0' && r <= '9':
		return keyDescriptor{vk: int(r), domCode: "Digit" + string(r), text: string(r), printable: true}, true
	case r == ' ':
		return keyDescriptor{vk: 32, domCode: "Space", text: " ", printable: true}, true
	}
	if d, ok := punctuationKeys[r]; ok {
		d.text = string(r)
		d.printable = true
		return d, true
	}
	if d, ok := shiftedPunctuationKeys[r]; ok {
		d.text = string(r)
		d.printable = true
		return d, true
	}
	return keyDescriptor{}, false
}

var punctuationKeys = map[rune]keyDescriptor{
	'-':  {vk: 189, domCode: "Minus"},
	'=':  {vk: 187, domCode: "Equal"},
	'[':  {vk: 219, domCode: "BracketLeft"},
	']':  {vk: 221, domCode: "BracketRight"},
	'\\': {vk: 220, domCode: "Backslash"},
	';':  {vk: 186, domCode: "Semicolon"},
	'\'': {vk: 222, domCode: "Quote"},
	',':  {vk: 188, domCode: "Comma"},
	'.':  {vk: 190, domCode: "Period"},
	'/':  {vk: 191, domCode: "Slash"},
	'`':  {vk: 192, domCode: "Backquote"},
}

// shiftedPunctuationKeys maps a shifted symbol to the code/VK of the physical
// key it shares with its unshifted base, mirroring the TS SHIFTED_SYMBOL_KEYS
// table so `press "!"` carries the same code/VK on both transports.
var shiftedPunctuationKeys = map[rune]keyDescriptor{
	'~': {vk: 192, domCode: "Backquote"},
	'_': {vk: 189, domCode: "Minus"},
	'+': {vk: 187, domCode: "Equal"},
	'{': {vk: 219, domCode: "BracketLeft"},
	'}': {vk: 221, domCode: "BracketRight"},
	'|': {vk: 220, domCode: "Backslash"},
	':': {vk: 186, domCode: "Semicolon"},
	'"': {vk: 222, domCode: "Quote"},
	'<': {vk: 188, domCode: "Comma"},
	'>': {vk: 190, domCode: "Period"},
	'?': {vk: 191, domCode: "Slash"},
	'!': {vk: 49, domCode: "Digit1"},
	'@': {vk: 50, domCode: "Digit2"},
	'#': {vk: 51, domCode: "Digit3"},
	'$': {vk: 52, domCode: "Digit4"},
	'%': {vk: 53, domCode: "Digit5"},
	'^': {vk: 54, domCode: "Digit6"},
	'&': {vk: 55, domCode: "Digit7"},
	'*': {vk: 56, domCode: "Digit8"},
	'(': {vk: 57, domCode: "Digit9"},
	')': {vk: 48, domCode: "Digit0"},
}

// shiftedChars maps an unshifted printable character to the character the same
// physical key produces with Shift held on the US layout the tables model.
// Letters are handled with strings.ToUpper; digits and punctuation are not.
var shiftedChars = map[rune]rune{
	'1': '!', '2': '@', '3': '#', '4': '$', '5': '%',
	'6': '^', '7': '&', '8': '*', '9': '(', '0': ')',
	'-': '_', '=': '+', '[': '{', ']': '}', '\\': '|',
	';': ':', '\'': '"', ',': '<', '.': '>', '/': '?', '`': '~',
}

// requiresShiftKey reports whether the character is only reachable with Shift
// held on the US layout: an uppercase letter or a shifted symbol.
func requiresShiftKey(key string) bool {
	runes := []rune(key)
	if len(runes) != 1 {
		return false
	}
	if runes[0] >= 'A' && runes[0] <= 'Z' {
		return true
	}
	_, shifted := shiftedPunctuationKeys[runes[0]]
	return shifted
}

// keyParams builds the shared payload for one key event from a key spec's key
// and modifiers.
//
// Shift on a letter produces the uppercase character ("Shift+a" is "A"); a
// printable character under Ctrl/Alt/Meta produces no character at all, so no
// text is sent — the shortcut must not insert the letter it was combined with.
func keyParams(key string, modifiers int) map[string]any {
	// A character only reachable with Shift must not claim shiftKey:false.
	if modifiers == 0 && requiresShiftKey(key) {
		modifiers |= modifierShift
	}
	params := map[string]any{"key": key, "modifiers": modifiers}
	d, known := describeKey(key)
	if !known {
		// Unknown single characters keep the old text-only fallback.
		if len([]rune(key)) == 1 && modifiers&(modifierCtrl|modifierAlt|modifierMeta) == 0 {
			params["text"] = key
		}
		return params
	}
	params["windowsVirtualKeyCode"] = d.vk
	params["code"] = d.domCode

	text := d.text
	if d.printable {
		if modifiers&modifierShift != 0 {
			// Shift+1 is "!", Shift+- is "_": upper-casing a digit is a no-op,
			// so the shifted table supplies the character for non-letters.
			shifted := strings.ToUpper(key)
			if r := []rune(key); len(r) == 1 {
				if s, ok := shiftedChars[r[0]]; ok {
					shifted = string(s)
				}
			}
			if len([]rune(shifted)) == 1 {
				text = shifted
				params["key"] = shifted
			}
		}
		if modifiers&(modifierCtrl|modifierAlt|modifierMeta) != 0 {
			text = ""
		}
	}
	if text != "" {
		params["text"] = text
	}
	return params
}

// validateKeyName rejects a multi-character key name that is not in the table:
// a typo must not dispatch a garbage event with exit 0. A single unknown rune
// stays a valid text-only character.
func validateKeyName(key, keySpec string) error {
	if _, known := describeKey(key); !known && len([]rune(key)) != 1 {
		return fmt.Errorf("unknown key %q in %q", key, keySpec)
	}
	return nil
}

// parseKey returns key string and modifiers bitmask from a spec like "Ctrl+Shift+a".
func parseKey(keySpec string) (string, int, error) {
	if strings.TrimSpace(keySpec) == "" {
		return "", 0, fmt.Errorf("key cannot be empty")
	}
	// "+" is both the separator and a key: a bare "+" is the plus key, and
	// "Ctrl++" is Ctrl held with it. A lone trailing "+" after a modifier
	// ("Ctrl+") still names no key and stays an error.
	if keySpec == "+" {
		return "+", 0, nil
	}
	plusKey := strings.HasSuffix(keySpec, "++")
	body := keySpec
	if plusKey {
		body = strings.TrimSuffix(keySpec, "++")
	}
	var parts []string
	if body != "" {
		parts = strings.Split(body, "+")
	}
	if !plusKey && len(parts) == 1 {
		key := strings.TrimSpace(parts[0])
		if err := validateKeyName(key, keySpec); err != nil {
			return "", 0, err
		}
		return key, 0, nil
	}
	key := "+"
	modParts := parts
	if !plusKey {
		key = strings.TrimSpace(parts[len(parts)-1])
		if key == "" {
			return "", 0, fmt.Errorf("key cannot be empty in %q", keySpec)
		}
		modParts = parts[:len(parts)-1]
	}
	// A multi-character name that is not in the table is a typo, not a
	// character: fail loudly instead of dispatching garbage with exit 0.
	if err := validateKeyName(key, keySpec); err != nil {
		return "", 0, err
	}
	modifiers := 0
	for _, mod := range modParts {
		switch strings.ToLower(strings.TrimSpace(mod)) {
		case "alt", "option":
			modifiers |= modifierAlt
		case "ctrl", "control":
			modifiers |= modifierCtrl
		case "meta", "cmd", "command":
			modifiers |= modifierMeta
		case "shift":
			modifiers |= modifierShift
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
	params := keyParams(key, modifiers)
	params["type"] = "keyDown"
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
	params := keyParams(key, modifiers)
	params["type"] = "keyUp"
	delete(params, "text") // text only on keyDown
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
	key, modifiers, err := parseKey(keySpec)
	if err != nil {
		return err
	}
	for _, typ := range []string{"keyDown", "keyUp"} {
		p := keyParams(key, modifiers)
		p["type"] = typ
		if typ == "keyUp" {
			delete(p, "text") // text only on keyDown
		}
		if err := s.Call("Input.dispatchKeyEvent", p, nil); err != nil {
			return fmt.Errorf("dispatch %s: %w", typ, err)
		}
	}
	return nil
}

// namedKeys mirrors the descriptor table in src/utils/keyMap.ts: virtual
// keycode, DOM `code`, and — for Enter/Tab — the control-char text that makes
// char events (form submit, focus move) actually fire. F1–F12 and the bare
// modifier keys are named so "F5" or "Shift" resolves on its own.
var namedKeys = map[string]keyDescriptor{
	"Enter":      {vk: 13, domCode: "Enter", text: "\r"},
	"Tab":        {vk: 9, domCode: "Tab", text: "\t"},
	"Escape":     {vk: 27, domCode: "Escape"},
	"Backspace":  {vk: 8, domCode: "Backspace"},
	"Delete":     {vk: 46, domCode: "Delete"},
	"ArrowLeft":  {vk: 37, domCode: "ArrowLeft"},
	"ArrowUp":    {vk: 38, domCode: "ArrowUp"},
	"ArrowRight": {vk: 39, domCode: "ArrowRight"},
	"ArrowDown":  {vk: 40, domCode: "ArrowDown"},
	"Home":       {vk: 36, domCode: "Home"},
	"End":        {vk: 35, domCode: "End"},
	"PageUp":     {vk: 33, domCode: "PageUp"},
	"PageDown":   {vk: 34, domCode: "PageDown"},
	"F1":         {vk: 112, domCode: "F1"},
	"F2":         {vk: 113, domCode: "F2"},
	"F3":         {vk: 114, domCode: "F3"},
	"F4":         {vk: 115, domCode: "F4"},
	"F5":         {vk: 116, domCode: "F5"},
	"F6":         {vk: 117, domCode: "F6"},
	"F7":         {vk: 118, domCode: "F7"},
	"F8":         {vk: 119, domCode: "F8"},
	"F9":         {vk: 120, domCode: "F9"},
	"F10":        {vk: 121, domCode: "F10"},
	"F11":        {vk: 122, domCode: "F11"},
	"F12":        {vk: 123, domCode: "F12"},
	"Shift":      {vk: 16, domCode: "ShiftLeft"},
	"Control":    {vk: 17, domCode: "ControlLeft"},
	"Ctrl":       {vk: 17, domCode: "ControlLeft"},
	"Alt":        {vk: 18, domCode: "AltLeft"},
	"Meta":       {vk: 91, domCode: "MetaLeft"},
	"Cmd":        {vk: 91, domCode: "MetaLeft"},
}
