package cdp

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Same-origin iframe targeting (global --frame).
//
// A scoped call runs a JS function inside the target realm: the top-level
// document, or — when the session has a frame — the main-world execution
// context of the <iframe> matched by Session.frame. The expression is sent
// with Runtime.evaluate's contextId, so it is compiled by the frame's own
// realm (`window`, `document`, `HTMLInputElement`, `getComputedStyle` … all
// bind to the frame; cross-realm instanceof checks would otherwise silently
// fail) and — unlike `contentWindow.eval` — it is exempt from the frame
// document's Content-Security-Policy, so a frame served without
// 'unsafe-eval' still accepts the DOM bundle.
//
// Cross-origin frames are rejected: their contentDocument is null to the
// parent page, and an out-of-process frame would need its own CDP target.

// scopedEnvelope is the uniform result of a scoped call: exactly one of Ok,
// Error (the function threw) or FrameError (the --frame lookup failed) is set.
type scopedEnvelope struct {
	Ok         json.RawMessage `json:"ok"`
	Error      *string         `json:"error"`
	FrameError *string         `json:"frameError"`
}

// frameLookupJS resolves the --frame iframe in the top document into `f`
// (element), `w` (its window) and `d` (its document), or returns a
// {frameError} envelope. %[1]s is the JSON-encoded frame selector.
const frameLookupJS = `const sel = %[1]s;
	let f;
	try { f = document.querySelector(sel); } catch (e) {
		return {frameError: "--frame " + JSON.stringify(sel) + " is not a valid CSS selector: " + e.message};
	}
	if (!f) return {frameError: "--frame " + JSON.stringify(sel) + ": no element matches it in the top-level document"};
	if (f.tagName !== "IFRAME" && f.tagName !== "FRAME") {
		return {frameError: "--frame " + JSON.stringify(sel) + " matches a <" + f.tagName.toLowerCase() + ">, not an <iframe>"};
	}
	const d = f.contentDocument;
	if (!d) {
		return {frameError: "--frame " + JSON.stringify(sel) + " is a cross-origin iframe (src=" + JSON.stringify(f.src) +
			"); its document is not reachable from the parent page. Open the frame directly instead: htrcli --cdp open " + f.src};
	}
	const w = f.contentWindow;`

// scopedExpr builds the expression that calls fnSrc with args and wraps the
// outcome in a scopedEnvelope. The realm is chosen by WHERE it is evaluated
// (see runScoped), not by the expression, so the same text serves the top
// document and a --frame context.
func scopedExpr(fnSrc string, args []any) (string, error) {
	if args == nil {
		args = []any{}
	}
	argsJSON, err := json.Marshal(args)
	if err != nil {
		return "", fmt.Errorf("encoding scoped call args: %w", err)
	}
	return fmt.Sprintf(`(async () => {
	try { return {ok: await (%s)(...%s)}; }
	catch (e) { return {error: String(e && e.message || e)}; }
})()`, fnSrc, argsJSON), nil
}

// runScoped calls fnSrc(args...) in the session's realm and returns its
// JSON value. A thrown error or a failed --frame lookup becomes a Go error.
func runScoped(s *Session, fnSrc string, args ...any) (json.RawMessage, error) {
	expr, err := scopedExpr(fnSrc, args)
	if err != nil {
		return nil, err
	}
	res, err := evaluateScoped(s, expr, true)
	if err != nil {
		return nil, err
	}
	var env scopedEnvelope
	if err := json.Unmarshal(res.Result.Value, &env); err != nil {
		return nil, fmt.Errorf("decoding scoped result: %w", err)
	}
	if env.FrameError != nil {
		return nil, fmt.Errorf("%s", *env.FrameError)
	}
	if env.Error != nil {
		return nil, fmt.Errorf("%s", *env.Error)
	}
	if env.Ok == nil {
		return json.RawMessage("null"), nil
	}
	return env.Ok, nil
}

// staleContextErrors are the CDP messages for an execution context that no
// longer exists when an evaluation is submitted: the frame navigated or was
// removed after its context id was cached, so the id must be re-resolved.
// The expression never started, so retrying cannot repeat a side effect.
// "Execution context was destroyed" is deliberately absent: it is reported
// when the context dies while the expression runs, after input may already
// have been dispatched (a click that navigated), and a retry would repeat it.
var staleContextErrors = []string{
	"Cannot find context with specified id",
	"Cannot find default execution context",
}

func isStaleContextErr(err error) bool {
	for _, m := range staleContextErrors {
		if strings.Contains(err.Error(), m) {
			return true
		}
	}
	return false
}

// runtimeReplayMethods are the events Runtime.enable re-sends for state that
// already exists: live execution contexts, stored console messages and
// uncaught exceptions.
var runtimeReplayMethods = []string{
	"Runtime.executionContextCreated",
	"Runtime.consoleAPICalled",
	"Runtime.exceptionThrown",
}

// dropEventsFrom returns buf without the events whose method is in methods, at
// or after index from; the events before from are kept as they are.
func dropEventsFrom(buf []cdpMessage, from int, methods ...string) []cdpMessage {
	out := append([]cdpMessage(nil), buf[:from]...)
	for _, msg := range buf[from:] {
		if !slices.Contains(methods, msg.Method) {
			out = append(out, msg)
		}
	}
	return out
}

// evaluateScoped is evaluate in the session's realm: the top document, or
// the --frame's main-world execution context. A context id goes stale when
// the frame navigates (see staleContextErrors); that clears the cached id and
// the evaluation is retried once in the frame's new context.
func evaluateScoped(s *Session, expression string, awaitPromise bool) (*evalResult, error) {
	if s.frame == "" {
		return evaluate(s, expression, awaitPromise)
	}
	ctx, err := s.frameContext()
	if err != nil {
		return nil, err
	}
	res, err := evaluateIn(s, expression, ctx, awaitPromise)
	if err != nil && isStaleContextErr(err) {
		s.frameContextID = 0
		if ctx, err = s.frameContext(); err != nil {
			return nil, err
		}
		return evaluateIn(s, expression, ctx, awaitPromise)
	}
	return res, err
}

// frameContext returns the main-world execution context id of the --frame
// iframe, resolving and caching it on first use:
//
//  1. frameLookupJS locates the <iframe> in the top document (the same
//     validation and messages every frame path uses: bad selector, no
//     match, not an iframe, cross-origin) and hands back the element;
//  2. DOM.describeNode on that element yields its frameId;
//  3. Runtime.enable makes Chrome announce every live execution context
//     (Runtime.executionContextCreated, buffered by Call), and the default
//     (main-world) context whose auxData.frameId matches is the one.
func (s *Session) frameContext() (int64, error) {
	if s.frameContextID != 0 {
		return s.frameContextID, nil
	}
	frameJSON, err := json.Marshal(s.frame)
	if err != nil {
		return 0, fmt.Errorf("encoding --frame selector: %w", err)
	}
	expr := fmt.Sprintf("(() => {\n"+frameLookupJS+"\n\treturn f;\n})()", frameJSON)
	res, err := s.evaluateObject(expr)
	if err != nil {
		return 0, fmt.Errorf("resolving --frame %q: %w", s.frame, err)
	}
	if res.ClassName == "Object" {
		// The frame lookup failed and returned the {frameError} envelope.
		// Re-run by value (error path only) to read the message out of it.
		frameErr, ferr := s.frameErrorOf(expr)
		if ferr != nil {
			return 0, fmt.Errorf("resolving --frame %q: %w", s.frame, ferr)
		}
		return 0, errors.New(frameErr)
	}
	if res.ObjectID == "" {
		return 0, fmt.Errorf("resolving --frame %q: the lookup returned no element", s.frame)
	}
	var desc struct {
		Node struct {
			FrameID string `json:"frameId"`
		} `json:"node"`
	}
	if err := s.Call("DOM.describeNode", map[string]any{"objectId": res.ObjectID}, &desc); err != nil {
		return 0, fmt.Errorf("resolving --frame %q: %w", s.frame, err)
	}
	if desc.Node.FrameID == "" {
		return 0, fmt.Errorf("resolving --frame %q: DOM.describeNode reported no frameId for the <iframe>", s.frame)
	}
	// Runtime.enable replays executionContextCreated for every live context
	// each time it is (re-)enabled; disable first so a session that already
	// enabled it (console watch) still receives the replay.
	if err := s.Call("Runtime.disable", nil, nil); err != nil {
		return 0, fmt.Errorf("Runtime.disable: %w", err)
	}
	mark := len(s.pending)
	if err := s.Call("Runtime.enable", nil, nil); err != nil {
		return 0, fmt.Errorf("Runtime.enable: %w", err)
	}
	// The replay is exactly the events buffered since mark. Enable also replays
	// stored console messages and exceptions; this lookup reads none of them, so
	// they are taken out of the buffer rather than left to pile up on every
	// re-resolve. Other buffered events stay untouched.
	replay := append([]cdpMessage(nil), s.pending[mark:]...)
	s.pending = dropEventsFrom(s.pending, mark, runtimeReplayMethods...)
	// Newest first: an earlier enable may have left a now-stale context for
	// the same frameId in the replay, and the one received last is current.
	for i := len(replay) - 1; i >= 0; i-- {
		msg := replay[i]
		if msg.Method != "Runtime.executionContextCreated" {
			continue
		}
		var ev struct {
			Context struct {
				ID      int64 `json:"id"`
				AuxData struct {
					FrameID   string `json:"frameId"`
					IsDefault bool   `json:"isDefault"`
				} `json:"auxData"`
			} `json:"context"`
		}
		if err := json.Unmarshal(msg.Params, &ev); err != nil {
			return 0, fmt.Errorf("decoding Runtime.executionContextCreated: %w", err)
		}
		if ev.Context.AuxData.FrameID == desc.Node.FrameID && ev.Context.AuxData.IsDefault {
			s.frameContextID = ev.Context.ID
			return s.frameContextID, nil
		}
	}
	return 0, fmt.Errorf("resolving --frame %q: no main-world execution context for frame %s (the frame may still be loading)", s.frame, desc.Node.FrameID)
}

// toTopViewport converts one viewport point inside the session's frame to
// top-level viewport coordinates. A no-op without a frame.
func toTopViewport(s *Session, x, y float64) (float64, float64, error) {
	pts, err := toTopViewportPts(s, [][2]float64{{x, y}})
	if err != nil {
		return 0, 0, err
	}
	return pts[0][0], pts[0][1], nil
}

// clipAncestorsJS defines scrollClippingAncestors(anchor, pointsFn), shared by
// the frame path (toTopViewportPts) and the drag path (dragPointsFn). Each
// scrollable ancestor of anchor that clips one of pointsFn() is scrolled just
// far enough to bring that point inside it. pointsFn is re-read for every
// container because an earlier scroll moves the points. It is a JS fragment
// spliced into both scripts, so it must not contain '%' (fmt verbs).
const clipAncestorsJS = `const scrollClippingAncestors = (anchor, pointsFn) => {
	for (let el = anchor.parentElement; el; el = el.parentElement) {
		const oe = getComputedStyle(el);
		const scrollY = oe.overflowY !== "visible" && el.scrollHeight > el.clientHeight;
		const scrollX = oe.overflowX !== "visible" && el.scrollWidth > el.clientWidth;
		if (!scrollY && !scrollX) continue;
		const r = el.getBoundingClientRect();
		let dx = 0, dy = 0;
		for (const p of pointsFn()) {
			if (scrollY) {
				if (p.y < r.top) dy = Math.min(dy, p.y - r.top - 4);
				else if (p.y >= r.bottom) dy = Math.max(dy, p.y - r.bottom + 4);
			}
			if (scrollX) {
				if (p.x < r.left) dx = Math.min(dx, p.x - r.left - 4);
				else if (p.x >= r.right) dx = Math.max(dx, p.x - r.right + 4);
			}
		}
		if (dx !== 0 || dy !== 0) el.scrollBy({left: dx, top: dy, behavior: "instant"});
	}
};
`

// toTopViewportPts converts EVERY point in one pass so a scroll performed to
// reveal one point cannot invalidate another that was already converted —
// resolving points one call at a time is how Drag ended up pressing at
// coordinates the second scroll had moved. For each point it:
//
//  1. maps frame-local coordinates onto the top viewport using the iframe's
//     bounding rect, border and padding, scaled by rect/offsetWidth so a
//     transform:scale'd (or otherwise visually resized) iframe still maps
//     correctly — offsetWidth is layout px and is never affected by
//     transforms. Rotation/skew are not modelled (see the hit test below);
//  2. scrolls every scrollable ANCESTOR that clips a point (nested overflow
//     containers, which window.scrollBy cannot reach) and then the window,
//     once, to bring all points into the viewport;
//  3. verifies each point with document.elementFromPoint: it must land on
//     the iframe itself. A point that lands elsewhere would dispatch the
//     trusted event at the wrong element (covered by an overlay, or a
//     mapping the hit test cannot vouch for), so it is an error naming what
//     is actually there instead of a silently wrong click.
func toTopViewportPts(s *Session, pts [][2]float64) ([][2]float64, error) {
	if s.frame == "" {
		return pts, nil
	}
	frameJSON, err := json.Marshal(s.frame)
	if err != nil {
		return nil, fmt.Errorf("encoding --frame selector: %w", err)
	}
	var b strings.Builder
	for _, p := range pts {
		fmt.Fprintf(&b, `[%v,%v],`, p[0], p[1])
	}
	expr := fmt.Sprintf(`(() => {
	`+frameLookupJS+`
	`+clipAncestorsJS+`
	const pts = [%[2]s];
	const map = () => {
		const r = f.getBoundingClientRect();
		// offsetWidth is layout px (transforms do not change it); the rect is
		// the rendered box, so their ratio is the visual scale applied to the
		// iframe and everything inside it.
		const sx = f.offsetWidth ? r.width / f.offsetWidth : 1;
		const sy = f.offsetHeight ? r.height / f.offsetHeight : 1;
		const cs = getComputedStyle(f);
		const ox = f.clientLeft + parseFloat(cs.paddingLeft);
		const oy = f.clientTop + parseFloat(cs.paddingTop);
		return pts.map(([fx, fy]) => ({
			x: r.left + (ox + fx) * sx,
			y: r.top + (oy + fy) * sy,
		}));
	};
	const inVP = (p) => p.x >= 0 && p.y >= 0 && p.x < innerWidth && p.y < innerHeight;
	// 1. Scroll every scrollable ancestor that clips a point: window.scrollBy
	// alone cannot reveal anything inside a nested overflow container.
	scrollClippingAncestors(f, map);
	// 2. Then the window: centre the bounding box of all points at once.
	let ps = map();
	if (ps.some((p) => !inVP(p))) {
		const xs = ps.map((p) => p.x), ys = ps.map((p) => p.y);
		window.scrollBy({
			left: (Math.min(...xs) + Math.max(...xs)) / 2 - innerWidth / 2,
			top: (Math.min(...ys) + Math.max(...ys)) / 2 - innerHeight / 2,
			behavior: "instant",
		});
		ps = map();
	}
	// 3. Every point must be in the viewport AND land on the iframe, or the
	// dispatch would hit whatever is actually there.
	for (const p of ps) {
		if (!inVP(p)) {
			return {frameError: "--frame: point (" + Math.round(p.x) + "," + Math.round(p.y) +
				") is still outside the " + innerWidth + "x" + innerHeight + " viewport after scrolling"};
		}
		const cx = Math.min(Math.max(p.x, 0), innerWidth - 1);
		const cy = Math.min(Math.max(p.y, 0), innerHeight - 1);
		const hit = document.elementFromPoint(cx, cy);
		if (hit !== f && !(hit && f.contains(hit))) {
			const what = hit ? "<" + hit.tagName.toLowerCase() +
				(hit.id ? "#" + hit.id : "") + ">" : "nothing";
			return {frameError: "--frame: the click point (" + Math.round(p.x) + "," + Math.round(p.y) +
				") lands on " + what + ", not on the <iframe> — it is covered, or the frame is rotated/skewed (unsupported transform)"};
		}
	}
	return {ok: ps};
})()`, frameJSON, b.String())
	res, err := evaluate(s, expr, false)
	if err != nil {
		return nil, err
	}
	var env scopedEnvelope
	if err := json.Unmarshal(res.Result.Value, &env); err != nil {
		return nil, fmt.Errorf("decoding frame offset: %w", err)
	}
	if env.FrameError != nil {
		return nil, fmt.Errorf("%s", *env.FrameError)
	}
	// The JS maps to [{x, y}, …] objects — decode those, not JSON arrays.
	var out []struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	}
	if err := json.Unmarshal(env.Ok, &out); err != nil {
		return nil, fmt.Errorf("decoding frame offset points: %w", err)
	}
	if len(out) != len(pts) {
		return nil, fmt.Errorf("frame offset returned %d points, want %d", len(out), len(pts))
	}
	pts2 := make([][2]float64, len(out))
	for i, p := range out {
		pts2[i] = [2]float64{p.X, p.Y}
	}
	return pts2, nil
}

// elementObjectID returns a RemoteObjectId for the element matched by the
// CSS selector cssPath in the session's realm (frame-aware). Used where a
// CDP DOM method needs a node handle (DOM.setFileInputFiles), since
// DOM.querySelector on the top document cannot see inside a frame.
func elementObjectID(s *Session, cssPath string) (string, error) {
	pathJSON, err := json.Marshal(cssPath)
	if err != nil {
		return "", fmt.Errorf("encoding element path: %w", err)
	}
	expr := fmt.Sprintf(`document.querySelector(%s)`, pathJSON)
	ctx := int64(0)
	if s.frame != "" {
		if ctx, err = s.frameContext(); err != nil {
			return "", err
		}
	}
	res, err := s.evaluateObjectIn(expr, ctx)
	if err != nil {
		return "", fmt.Errorf("resolving %s: %w", cssPath, err)
	}
	if res.ObjectID == "" || res.Subtype == "null" {
		return "", fmt.Errorf("no element matches %s", cssPath)
	}
	return res.ObjectID, nil
}

// objectResult is the subset of Runtime.evaluate's reply the object-handle
// paths need: an objectId for the element, or className "Object" for the
// {frameError} envelope returned by a failed frame lookup.
type objectResult struct {
	ObjectID  string `json:"objectId"`
	Subtype   string `json:"subtype"`
	ClassName string `json:"className"`
}

// evaluateObject runs expr in the top document with returnByValue:false and
// decodes the object handle it produced. A script exception is a Go error
// carrying the exception text, never a result.
func (s *Session) evaluateObject(expr string) (*objectResult, error) {
	return s.evaluateObjectIn(expr, 0)
}

// evaluateObjectIn is evaluateObject in the execution context ctx (0 = the
// target's default context).
func (s *Session) evaluateObjectIn(expr string, ctx int64) (*objectResult, error) {
	var res struct {
		Result    objectResult `json:"result"`
		Exception *struct {
			Text      string `json:"text"`
			Exception *struct {
				Description string `json:"description"`
			} `json:"exception"`
		} `json:"exceptionDetails"`
	}
	params := map[string]any{"expression": expr, "returnByValue": false}
	if ctx != 0 {
		params["contextId"] = ctx
	}
	if err := s.Call("Runtime.evaluate", params, &res); err != nil {
		return nil, err
	}
	if res.Exception != nil {
		msg := res.Exception.Text
		if res.Exception.Exception != nil && res.Exception.Exception.Description != "" {
			msg = res.Exception.Exception.Description
		}
		return nil, &pageError{Message: msg}
	}
	return &res.Result, nil
}

// frameErrorOf re-evaluates expr with returnByValue:true and returns the
// {frameError} message it produced. Only called after a frame lookup has
// already failed, so the extra round trip is error-path only.
func (s *Session) frameErrorOf(expr string) (string, error) {
	var res struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
	}
	if err := s.Call("Runtime.evaluate", map[string]any{
		"expression":    expr,
		"returnByValue": true,
	}, &res); err != nil {
		return "", err
	}
	var env scopedEnvelope
	if err := json.Unmarshal(res.Result.Value, &env); err != nil {
		return "", err
	}
	if env.FrameError != nil {
		return *env.FrameError, nil
	}
	return "", nil
}
