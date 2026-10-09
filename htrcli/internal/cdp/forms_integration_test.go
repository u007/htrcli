//go:build integration

package cdp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/u007/htrcli/internal/api"
)

// comboScript is a minimal react-select-like combobox: a 2px input with
// role=combobox and aria-controls, a listbox filled on ArrowDown / typing,
// and the selection shown in a .select__single-value div inside
// .select__control. Every handler ignores untrusted events, so the tests
// prove pick uses trusted CDP input (a synthetic dispatchEvent does nothing).
const comboScript = `<script>
function makeCombo(inputId, lbId, source, renderDelay) {
  const input = document.getElementById(inputId);
  const lb = document.getElementById(lbId);
  const vc = input.closest(".select__value-container");
  const render = (items) => {
    lb.innerHTML = "";
    for (const it of items) {
      const o = document.createElement("div");
      o.setAttribute("role", "option");
      o.className = "select__option";
      o.textContent = it;
      o.style.padding = "6px";
      o.addEventListener("click", (e) => {
        if (!e.isTrusted) return;
        const show = () => {
          const ph = vc.querySelector(".select__placeholder");
          if (ph) ph.remove();
          let sv = vc.querySelector(".select__single-value");
          if (!sv) { sv = document.createElement("div"); sv.className = "select__single-value"; vc.insertBefore(sv, vc.firstChild); }
          sv.textContent = it;
        };
        input.value = "";
        lb.innerHTML = "";
        // renderDelay mimics React committing the selected value after the click.
        if (renderDelay) setTimeout(show, renderDelay); else show();
      });
      lb.appendChild(o);
    }
  };
  const open = () => source(input.value, render, lb);
  input.addEventListener("keydown", (e) => { if (e.isTrusted && e.key === "ArrowDown") open(); });
  input.addEventListener("input", (e) => { if (e.isTrusted) open(); });
}
const colors = ["Red", "Green", "Blue", "Light Blue", "Blue Grey"];
function syncSource(q, render) {
  render(colors.filter((c) => c.toLowerCase().includes(q.toLowerCase())));
}
const cities = ["South San Francisco, California, United States",
  "San Francisco, California, United States",
  "San Francisco de Macoris, Dominican Republic"];
function asyncSource(q, render, lb) {
  lb.innerHTML = '<div class="loading">Loading...</div>';
  setTimeout(() => {
    if (q.length < 3) { render([]); return; }
    const p = q.slice(0, 3).toLowerCase();
    render(cities.filter((c) => c.toLowerCase().includes(p)));
  }, 500);
}
</script>`

func comboMarkup(id string) string {
	return fmt.Sprintf(`<div class="select__container">
  <div class="select__control" style="border:1px solid #888;min-height:30px;width:400px">
    <div class="select__value-container">
      <input id="%[1]s" role="combobox" aria-controls="%[1]s-lb" autocomplete="off"
        style="width:2px;border:0;padding:0;outline:0">
    </div>
  </div>
  <div id="%[1]s-lb" role="listbox" style="width:400px"></div>
</div>`, id)
}

// greenhouseCombo mirrors Greenhouse's react-select DOM: the input sits in
// .select__input-container inside .select__value-container inside
// .select__control, and the chosen value renders as .select__single-value in
// that same value container.
func greenhouseCombo(id, shown string) string {
	value := `<div class="select__placeholder">Select...</div>`
	if shown != "" {
		value = `<div class="select__single-value">` + shown + `</div>`
	}
	return fmt.Sprintf(`<div class="select-shell"><div class="select__control" style="border:1px solid #888;min-height:30px;width:400px">
  <div class="select__value-container">%[2]s
    <div class="select__input-container"><input class="select__input" id="%[1]s" role="combobox" aria-controls="%[1]s-lb"
      autocomplete="off" style="width:2px;border:0;padding:0;outline:0"></div>
  </div></div>
  <div id="%[1]s-lb" role="listbox" style="width:400px"></div>
</div>`, id, value)
}

// smoothPage reproduces the live Greenhouse failure: html{scroll-behavior:
// smooth} makes scrollIntoView animate, so coordinates read right after it are
// stale and the trusted click misses. The target sits far below the fold,
// renders its value 300ms after the click, and another select on the page
// already shows a value (a page-wide .select__single-value read would see it).
const smoothPage = `<!doctype html><html style="scroll-behavior:smooth"><body>
%s
<div style="height:3000px">spacer</div>
%s
<div style="height:1500px"></div>
%s
<script>
makeCombo("pre", "pre-lb", syncSource);
makeCombo("late", "late-lb", (q, render) => render(["Yes", "No"].filter((c) => c.toLowerCase().includes(q.toLowerCase()))), 300);
</script>
</body></html>`

const pickPage = `<!doctype html><html><body>
<div style="height:1400px">spacer (the comboboxes start below the fold)</div>
%s
<div style="height:40px"></div>
%s
<select id="size"><option value="">--</option><option value="s">Small</option><option value="m">Medium</option><option value="l">Large</option></select>
<div style="height:600px"></div>
%s
<script>makeCombo("color", "color-lb", syncSource); makeCombo("city", "city-lb", asyncSource);</script>
</body></html>`

const formPage = `<!doctype html><html><body>
<input id="name" type="text">
<input id="upper" type="text">
<select id="country"><option value="">--</option><option value="my">Malaysia</option><option value="sg">Singapore</option></select>
<label><input id="news" type="checkbox"> news</label>
<label><input id="terms" type="checkbox" checked> terms</label>
<label><input id="plan-free" type="radio" name="plan" value="free" checked> free</label>
<label><input id="plan-pro" type="radio" name="plan" value="pro"> pro</label>
<textarea id="bio"></textarea>
<input id="at" type="time">
<input id="cv" type="file">
<div class="extra"><span>nick</span><input name="nick"></div>
%s
%s
<script>
document.getElementById("upper").addEventListener("input", (e) => { e.target.value = e.target.value.toUpperCase(); });
makeCombo("color", "color-lb", syncSource);
</script>
</body></html>`

const outerPage = `<!doctype html><html><body style="margin:0">
<button id="decoy" style="position:absolute;left:20px;top:20px;width:100px;height:40px">decoy</button>
<iframe id="inner" src="/inner" style="position:absolute;left:200px;top:120px;width:600px;height:400px"></iframe>
<iframe id="xo" src="%s/remote" style="position:absolute;left:200px;top:560px;width:300px;height:100px"></iframe>
<script>window.decoyClicks = 0; document.getElementById("decoy").addEventListener("click", () => { window.decoyClicks++; });</script>
</body></html>`

const innerPage = `<!doctype html><html><body style="margin:0">
<button id="go" style="position:absolute;left:20px;top:20px;width:100px;height:40px">go</button>
<div style="position:absolute;left:20px;top:80px">
<input id="q" type="text">
<p id="msg">inside the frame</p>
%s
</div>
%s
<script>
window.goClicks = 0;
document.getElementById("go").addEventListener("click", (e) => { if (e.isTrusted) window.goClicks++; });
makeCombo("fcolor", "fcolor-lb", syncSource);
</script>
</body></html>`

func serveHTML(t *testing.T, pages map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := pages[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// No 'unsafe-eval': the DOM bundle and form kit must reach the top
		// document AND the --frame through Runtime.evaluate (CSP-exempt),
		// never through contentWindow.eval, which this policy blocks.
		w.Header().Set("Content-Security-Policy", "script-src 'self' 'unsafe-inline'")
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func evalString(t *testing.T, s *Session, expr string) string {
	t.Helper()
	raw, err := Evaluate(s, expr)
	if err != nil {
		t.Fatalf("eval %s: %v", expr, err)
	}
	var out string
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("eval %s: not a string: %s", expr, raw)
	}
	return out
}

func TestCDPPick(t *testing.T) {
	s, targetID := startIntegrationBrowser(t, 9224)
	srv := serveHTML(t, map[string]string{
		"/": fmt.Sprintf(pickPage, comboMarkup("color"), comboMarkup("city"), comboScript),
	})
	if err := Navigate(s, srv.URL+"/", 10000); err != nil {
		t.Fatalf("navigate: %v", err)
	}
	shown := func(id string) string {
		return evalString(t, s, fmt.Sprintf(`(document.querySelector("#%s").closest(".select__control").querySelector(".select__single-value") || {textContent: ""}).textContent`, id))
	}

	// Exact match beats the prefix ("Blue Grey") and contains ("Light Blue") matches.
	res, err := Pick(s, targetID, &api.TargetSelector{Selector: "#color"}, "blue")
	if err != nil {
		t.Fatalf("pick blue: %v", err)
	}
	if res.Mode != "combobox" || res.Selected != "Blue" || shown("color") != "Blue" {
		t.Fatalf("pick blue: got %+v, widget shows %q", res, shown("color"))
	}

	// Async: the first suggestion is a contains-match ("South San Francisco…").
	want := "San Francisco, California, United States"
	if res, err = Pick(s, targetID, &api.TargetSelector{Selector: "#city"}, want); err != nil {
		t.Fatalf("pick city: %v", err)
	}
	if res.Selected != want || shown("city") != want {
		t.Fatalf("pick city: got %+v, widget shows %q", res, shown("city"))
	}

	// Two prefix matches at the same rank: refuse to guess.
	_, err = Pick(s, targetID, &api.TargetSelector{Selector: "#city"}, "San Francisco")
	if err == nil || !strings.Contains(err.Error(), "ambiguous") || !strings.Contains(err.Error(), "San Francisco de Macoris") {
		t.Fatalf("want ambiguity error naming both options, got %v", err)
	}

	// No match: fail loudly with the options the widget offers.
	_, err = Pick(s, targetID, &api.TargetSelector{Selector: "#color"}, "Purple")
	if err == nil || !strings.Contains(err.Error(), `"Red"`) || !strings.Contains(err.Error(), `"Blue Grey"`) {
		t.Fatalf("want no-match error listing options, got %v", err)
	}
	if shown("color") != "Blue" {
		t.Fatalf("failed pick must not change the selection, widget shows %q", shown("color"))
	}

	// Plain <select>: by visible text, then by value.
	if res, err = Pick(s, targetID, &api.TargetSelector{Selector: "#size"}, "Medium"); err != nil || res.Mode != "select" || res.Selected != "Medium" {
		t.Fatalf("pick select by text: %+v %v", res, err)
	}
	if got := evalString(t, s, `document.querySelector("#size").value`); got != "m" {
		t.Fatalf("select value after pick = %q, want m", got)
	}
	if res, err = Pick(s, targetID, &api.TargetSelector{Selector: "#size"}, "l"); err != nil || res.Selected != "Large" {
		t.Fatalf("pick select by value: %+v %v", res, err)
	}
	if _, err = Pick(s, targetID, &api.TargetSelector{Selector: "#size"}, "Huge"); err == nil || !strings.Contains(err.Error(), `"Small"`) {
		t.Fatalf("want select no-match error listing options, got %v", err)
	}

	// An id-less container target: bridged to a structural CSS path, then
	// pick descends from the matched .select__control to its input.
	if res, err = Pick(s, targetID, &api.TargetSelector{Selector: ".select__container .select__control"}, "Green"); err != nil || res.Selected != "Green" {
		t.Fatalf("pick via id-less container: %+v %v", res, err)
	}
	if shown("color") != "Green" {
		t.Fatalf("id-less pick hit the wrong widget: color shows %q", shown("color"))
	}
}

// Regression: live Greenhouse pick failed with `the combobox shows ""` on a
// cold page because the smooth scroll had not finished when the option's
// coordinates were read.
func TestCDPPickSmoothScrollDelayedRender(t *testing.T) {
	s, targetID := startIntegrationBrowser(t, 9227)
	srv := serveHTML(t, map[string]string{
		"/": fmt.Sprintf(smoothPage, greenhouseCombo("pre", "Preset"), greenhouseCombo("late", ""), comboScript),
	})
	if err := Navigate(s, srv.URL+"/", 10000); err != nil {
		t.Fatalf("navigate: %v", err)
	}
	res, err := Pick(s, targetID, &api.TargetSelector{Selector: "#late"}, "Yes")
	if err != nil {
		t.Fatalf("pick: %v", err)
	}
	if res.Selected != "Yes" {
		t.Fatalf("pick returned %+v, want Yes", res)
	}
	got := evalString(t, s, `document.querySelector("#late").closest(".select__value-container").innerText.trim() + "/" + document.querySelector("#pre").closest(".select__value-container").innerText.trim()`)
	if got != "Yes/Preset" {
		t.Fatalf("value containers = %q, want Yes/Preset", got)
	}
}

func TestCDPFrame(t *testing.T) {
	s, targetID := startIntegrationBrowser(t, 9225)
	remote := serveHTML(t, map[string]string{"/remote": `<!doctype html><input id="r">`})
	srv := serveHTML(t, map[string]string{
		"/":      fmt.Sprintf(outerPage, remote.URL),
		"/inner": fmt.Sprintf(innerPage, comboMarkup("fcolor"), comboScript),
	})
	if err := Navigate(s, srv.URL+"/", 10000); err != nil {
		t.Fatalf("navigate: %v", err)
	}
	s.SetFrame("#inner")

	// DOM verbs run inside the frame.
	if r, err := ExecDOM(s, api.Command{ID: "1", Action: "fill", Target: &api.TargetSelector{Selector: "#q"}, Value: "framed"}); err != nil || !r.Success {
		t.Fatalf("fill in frame: %v %+v", err, r)
	}
	r, err := ExecDOM(s, api.Command{ID: "2", Action: "getValue", Target: &api.TargetSelector{Selector: "#q"}})
	if err != nil || !r.Success || r.Data != "framed" {
		t.Fatalf("getValue in frame: %v %+v", err, r)
	}
	r, err = ExecDOM(s, api.Command{ID: "3", Action: "getText", Target: &api.TargetSelector{Selector: "#msg"}})
	if err != nil || !r.Success || r.Data != "inside the frame" {
		t.Fatalf("getText in frame: %v %+v", err, r)
	}

	// Trusted click lands on the in-frame button, not the decoy at the
	// un-offset coordinates in the top document.
	if err := Click(s, targetID, &api.TargetSelector{Selector: "#go"}, "click"); err != nil {
		t.Fatalf("click in frame: %v", err)
	}
	if err := Click(s, targetID, &api.TargetSelector{Selector: "#go"}, "dblclick"); err != nil {
		t.Fatalf("dblclick in frame: %v", err)
	}
	s.SetFrame("")
	counts := evalString(t, s, `String(document.querySelector("#inner").contentWindow.goClicks) + "/" + String(window.decoyClicks)`)
	if counts != "2/0" { // click = 1; dblclick = one press/release pair with clickCount 2 = 1 click
		t.Fatalf("goClicks/decoyClicks = %s, want 2/0", counts)
	}

	// pick inside the frame: keys must route into the frame, clicks offset.
	s.SetFrame("#inner")
	res, err := Pick(s, targetID, &api.TargetSelector{Selector: "#fcolor"}, "Green")
	if err != nil || res.Selected != "Green" {
		t.Fatalf("pick in frame: %+v %v", res, err)
	}

	// Cross-origin frame: clear error suggesting open of its src.
	s.SetFrame("#xo")
	_, err = ExecDOM(s, api.Command{ID: "4", Action: "getValue", Target: &api.TargetSelector{Selector: "#r"}})
	if err == nil || !strings.Contains(err.Error(), "cross-origin") || !strings.Contains(err.Error(), "htrcli --cdp open "+remote.URL+"/remote") {
		t.Fatalf("want cross-origin error with open hint, got %v", err)
	}

	// Missing iframe selector.
	s.SetFrame("#nope")
	_, err = ExecDOM(s, api.Command{ID: "5", Action: "getValue", Target: &api.TargetSelector{Selector: "#q"}})
	if err == nil || !strings.Contains(err.Error(), "no element matches") {
		t.Fatalf("want missing-frame error, got %v", err)
	}

	// Selector that is not an iframe.
	s.SetFrame("#decoy")
	_, err = ExecDOM(s, api.Command{ID: "6", Action: "getValue", Target: &api.TargetSelector{Selector: "#q"}})
	if err == nil || !strings.Contains(err.Error(), "not an <iframe>") {
		t.Fatalf("want not-an-iframe error, got %v", err)
	}
}

func TestCDPFillForm(t *testing.T) {
	s, targetID := startIntegrationBrowser(t, 9226)
	srv := serveHTML(t, map[string]string{
		"/":      fmt.Sprintf(formPage, comboMarkup("color"), comboScript),
		"/outer": `<!doctype html><iframe id="f" src="/" style="width:900px;height:700px"></iframe>`,
	})
	upload := filepath.Join(t.TempDir(), "cv.txt")
	if err := os.WriteFile(upload, []byte("cv"), 0o644); err != nil {
		t.Fatal(err)
	}

	fields := func(pairs ...string) []FormField {
		var out []FormField
		for i := 0; i < len(pairs); i += 2 {
			out = append(out, FormField{Selector: pairs[i], Target: &api.TargetSelector{Selector: pairs[i]}, Value: json.RawMessage(pairs[i+1])})
		}
		return out
	}
	uploadJSON, _ := json.Marshal(upload)
	form := fields(
		"#missing", `"x"`, // fails, but the batch must continue
		"#name", `"Ada Lovelace"`,
		"#upper", `"abc"`, // the page upper-cases it: MISMATCH
		"#country", `"Singapore"`,
		"#news", `true`,
		"#terms", `false`,
		"#plan-pro", `true`,
		"#bio", `"line one\nline two"`,
		"#at", `"09:30"`,
		"#cv", string(uploadJSON),
		"#color", `"Light Blue"`,
		".extra input", `"Ziggy"`, // id-less element: bridged to a structural CSS path
		"#name", `null`,
	)

	for _, framed := range []bool{false, true} {
		path := "/"
		s.SetFrame("")
		if framed {
			path = "/outer"
		}
		if err := Navigate(s, srv.URL+path, 10000); err != nil {
			t.Fatalf("navigate: %v", err)
		}
		if framed {
			s.SetFrame("#f")
		}
		reports := FillForm(s, targetID, form)
		if len(reports) != len(form) {
			t.Fatalf("framed=%v: %d reports for %d fields", framed, len(reports), len(form))
		}
		want := []string{FieldFailed, FieldOK, FieldMismatch, FieldOK, FieldOK, FieldOK, FieldOK, FieldOK, FieldOK, FieldOK, FieldOK, FieldOK, FieldSkipped}
		for i, r := range reports {
			if r.Status != want[i] {
				t.Errorf("framed=%v %s: status %s (actual %q, err %q), want %s", framed, r.Selector, r.Status, r.Actual, r.Error, want[i])
			}
		}
		if reports[2].Actual != "ABC" {
			t.Errorf("framed=%v: #upper actual %q, want ABC", framed, reports[2].Actual)
		}
		if !strings.Contains(reports[0].Error, "no element matches") {
			t.Errorf("framed=%v: #missing error %q", framed, reports[0].Error)
		}
		if reports[3].Actual != `Singapore (value "sg")` {
			t.Errorf("framed=%v: #country actual %q", framed, reports[3].Actual)
		}
		if reports[9].Actual != "cv.txt" {
			t.Errorf("framed=%v: #cv actual %q", framed, reports[9].Actual)
		}
		// The radio group really moved: the previously checked radio is off.
		check := `document.querySelector("#plan-free").checked`
		if framed {
			check = `document.querySelector("#f").contentDocument.querySelector("#plan-free").checked`
		}
		s2 := s.Frame()
		s.SetFrame("")
		if raw, err := Evaluate(s, check); err != nil || string(raw) != "false" {
			t.Errorf("framed=%v: #plan-free still checked (%s, %v)", framed, raw, err)
		}
		s.SetFrame(s2)

		// Idempotent: a second run over the same values changes nothing.
		again := FillForm(s, targetID, fields("#news", `true`, "#terms", `false`, "#plan-pro", `true`))
		for _, r := range again {
			if r.Status != FieldOK {
				t.Errorf("framed=%v rerun %s: %s %q", framed, r.Selector, r.Status, r.Error)
			}
		}
	}
}

const dragPage = `<!doctype html><html><body style="margin:0;height:2000px">
<div id="src" style="position:absolute;left:100px;top:100px;width:60px;height:20px;background:#9cf">src</div>
<div id="dst" style="position:absolute;left:400px;top:500px;width:60px;height:20px;background:#fc9">dst</div>
<script>
window.dragLog = [];
for (const type of ["mousedown", "mousemove", "mouseup"]) {
	document.addEventListener(type, (e) => {
		if (!e.isTrusted) return;
		const hit = document.elementFromPoint(e.clientX, e.clientY);
		window.dragLog.push(type + "@" + (e.target.id || e.target.tagName.toLowerCase()) +
			"(" + e.clientX + "," + e.clientY + " scroll=" + window.scrollY + " vp=" + innerWidth + "x" + innerHeight +
			" hit=" + (hit ? (hit.id || hit.tagName.toLowerCase()) : "none") + ")");
	});
}
window.scrollTo(0, 1400); // both endpoints start far out of view
</script>
</body></html>`

// TestCDPDrag drives the real page-side drag resolver: selector endpoints
// (the bundle's prepareClick/getBoundingBox, lowercase command keys), an @eN
// style endpoint passed as an element handle, and the fit-scroll re-check —
// the destination's centering scroll pushes the source out of view, so the
// resolver must scroll once more and re-read BOTH points before pressing.
func TestCDPDrag(t *testing.T) {
	s, targetID := startIntegrationBrowser(t, 9228)
	srv := serveHTML(t, map[string]string{"/": dragPage})
	if err := Navigate(s, srv.URL+"/", 10000); err != nil {
		t.Fatalf("navigate: %v", err)
	}
	logOf := func() string {
		return evalString(t, s, `(() => { const l = window.dragLog.join(","); window.dragLog = []; return l; })()`)
	}

	src := DragEndpoint{Sel: &api.TargetSelector{Selector: "#src"}}
	dst := DragEndpoint{Sel: &api.TargetSelector{Selector: "#dst"}}
	if err := Drag(s, targetID, src, dst, 3, 0); err != nil {
		t.Fatalf("selector drag: %v", err)
	}
	got := logOf()
	if !strings.HasPrefix(got, "mousedown@src(") || !strings.Contains(got, ",mouseup@dst(") {
		t.Fatalf("selector drag events = %s, want press on #src … release on #dst", got)
	}

	// Element-handle endpoint (what an @eN ref becomes), page scrolled away again.
	evalString(t, s, `(window.scrollTo(0, 1400), "")`)
	backendID, err := ResolveBackendNodeID(s, "#src")
	if err != nil {
		t.Fatalf("resolve #src: %v", err)
	}
	if err := Drag(s, targetID, DragEndpoint{BackendNodeID: backendID}, dst, 3, 0); err != nil {
		t.Fatalf("ref drag: %v", err)
	}
	got = logOf()
	if !strings.HasPrefix(got, "mousedown@src(") || !strings.Contains(got, ",mouseup@dst(") {
		t.Fatalf("ref drag events = %s, want press on #src … release on #dst", got)
	}
}
