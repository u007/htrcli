package cdp

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestOptionRank(t *testing.T) {
	cases := []struct {
		option, want string
		rank         int
	}{
		{"Blue", "blue", 3},
		{"  Blue\n ", "BLUE", 3},
		{"Blue Grey", "blue", 2},
		{"Light Blue", "blue", 1},
		{"Red", "blue", 0},
		{"Red", "", 0},
		{"San  Francisco,\tCA", "san francisco, ca", 3},
	}
	for _, c := range cases {
		if got := optionRank(c.option, c.want); got != c.rank {
			t.Errorf("optionRank(%q, %q) = %d, want %d", c.option, c.want, got, c.rank)
		}
	}
}

func TestBestOption(t *testing.T) {
	colors := []string{"Light Blue", "Blue Grey", "Blue", "Red"}
	if idx, tied := bestOption(colors, "blue"); idx != 2 || tied != nil {
		t.Fatalf("exact must win: idx=%d tied=%v", idx, tied)
	}
	if idx, tied := bestOption([]string{"Light Blue", "Blue Grey"}, "blue"); idx != 1 || tied != nil {
		t.Fatalf("prefix must beat contains: idx=%d tied=%v", idx, tied)
	}
	cities := []string{"South San Francisco, CA", "San Francisco, CA", "San Francisco de Macoris"}
	if idx, tied := bestOption(cities, "San Francisco"); idx != -1 || len(tied) != 2 {
		t.Fatalf("two prefix matches must be ambiguous: idx=%d tied=%v", idx, tied)
	}
	if idx, tied := bestOption(cities, "San Francisco, CA"); idx != 1 || tied != nil {
		t.Fatalf("exact among contains: idx=%d tied=%v", idx, tied)
	}
	if idx, tied := bestOption([]string{"Blue", "blue"}, "blue"); idx != -1 || len(tied) != 2 {
		t.Fatalf("duplicate exact options must be ambiguous: idx=%d tied=%v", idx, tied)
	}
	if idx, tied := bestOption(colors, "purple"); idx != -1 || tied != nil {
		t.Fatalf("no match: idx=%d tied=%v", idx, tied)
	}
	if idx, tied := bestOption(nil, "x"); idx != -1 || tied != nil {
		t.Fatalf("empty list: idx=%d tied=%v", idx, tied)
	}
}

func TestListOptionsCapsAt15(t *testing.T) {
	var opts []string
	for i := 0; i < 20; i++ {
		opts = append(opts, string(rune('a'+i)))
	}
	got := listOptions(opts)
	if !strings.Contains(got, `"o"`) || strings.Contains(got, `"p"`) || !strings.Contains(got, "and 5 more") {
		t.Fatalf("listOptions = %s", got)
	}
	if listOptions(nil) != "(no options visible)" {
		t.Fatalf("empty listOptions = %s", listOptions(nil))
	}
}

func TestSameOptions(t *testing.T) {
	if sameOptions(nil, nil) {
		t.Fatal("a missing previous poll is never 'settled'")
	}
	if !sameOptions([]string{"a"}, []string{"a"}) || sameOptions([]string{"a"}, []string{"b"}) {
		t.Fatal("sameOptions compares element-wise")
	}
}

func TestResolveSelectOption(t *testing.T) {
	opts := []selectOption{{"", "--"}, {"my", "Malaysia"}, {"sg", "Singapore"}, {"mx", "Mexico"}}
	for want, value := range map[string]string{"sg": "sg", "Singapore": "sg", "malay": "my"} {
		got, err := resolveSelectOption(opts, want)
		if err != nil || got.Value != value {
			t.Errorf("resolveSelectOption(%q) = %+v, %v; want value %q", want, got, err, value)
		}
	}
	if _, err := resolveSelectOption(opts, "Japan"); err == nil || !strings.Contains(err.Error(), `"Malaysia"`) {
		t.Errorf("no match must list options, got %v", err)
	}
	if _, err := resolveSelectOption(opts, "m"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("tied prefix must be ambiguous, got %v", err)
	}
}

func TestDecodeFormValue(t *testing.T) {
	cases := []struct {
		raw     string
		display string
		err     bool
	}{
		{`null`, "null", false},
		{`true`, "true", false},
		{`false`, "false", false},
		{`"09:30"`, "09:30", false},
		{`1.50`, "1.50", false},
		{`-3`, "-3", false},
		{`["a.pdf","b.pdf"]`, "a.pdf, b.pdf", false},
		{`[]`, "", true},
		{`[1]`, "", true},
		{`{"a":1}`, "", true},
		{``, "", true},
	}
	for _, c := range cases {
		v, err := decodeFormValue(json.RawMessage(c.raw))
		if (err != nil) != c.err {
			t.Errorf("decodeFormValue(%s) err = %v, want err=%v", c.raw, err, c.err)
			continue
		}
		if err == nil && v.String() != c.display {
			t.Errorf("decodeFormValue(%s) = %q, want %q", c.raw, v.String(), c.display)
		}
	}
	if v, _ := decodeFormValue(json.RawMessage(`1.50`)); !v.isStr {
		t.Error("numbers are filled as their literal text")
	}
}

func TestClassifyField(t *testing.T) {
	in := func(tag, typ string, attrs map[string]string) ElementInfo {
		return ElementInfo{Tag: tag, Type: typ, Path: "#x", Attributes: attrs}
	}
	cases := []struct {
		infos []ElementInfo
		kind  string
		err   string
	}{
		{[]ElementInfo{in("input", "", nil)}, kindText, ""},
		{[]ElementInfo{in("input", "time", nil)}, kindText, ""},
		{[]ElementInfo{in("textarea", "", nil)}, kindText, ""},
		{[]ElementInfo{in("select", "", nil)}, kindSelect, ""},
		{[]ElementInfo{in("input", "Checkbox", nil)}, kindCheckbox, ""},
		{[]ElementInfo{in("input", "radio", nil)}, kindRadio, ""},
		{[]ElementInfo{in("input", "file", nil)}, kindFile, ""},
		{[]ElementInfo{in("input", "text", map[string]string{"role": "combobox"})}, kindCombobox, ""},
		{[]ElementInfo{in("input", "text", map[string]string{"class": "select__input"})}, kindCombobox, ""},
		{[]ElementInfo{in("div", "", map[string]string{"class": "select__control"})}, kindCombobox, ""},
		{[]ElementInfo{in("input", "hidden", nil)}, "", "cannot be filled"},
		{[]ElementInfo{in("div", "", nil)}, "", "unsupported element"},
		{nil, "", "no element matches"},
		{[]ElementInfo{in("input", "checkbox", nil), in("input", "checkbox", nil)}, kindCheckbox, "matches 2 elements"},
	}
	for i, c := range cases {
		kind, err := classifyField(c.infos)
		if c.err == "" && (err != nil || kind != c.kind) {
			t.Errorf("case %d: got %q, %v; want %q", i, kind, err, c.kind)
		}
		if c.err != "" && (err == nil || !strings.Contains(err.Error(), c.err)) {
			t.Errorf("case %d: err = %v, want containing %q", i, err, c.err)
		}
	}
}

func TestCompareField(t *testing.T) {
	str := func(s string) formValue { return formValue{isStr: true, str: s} }
	cases := []struct {
		kind, tag, raw string
		confirmed      string
		v              formValue
		actual         string
		ok             bool
	}{
		// Text: the browser's own value sanitization is applied to both sides.
		{kindText, "input", `"Ada"`, "", str("Ada"), "Ada", true},
		{kindText, "input", `"ABC"`, "", str("abc"), "ABC", false},
		{kindText, "textarea", `"line one\nline two"`, "", str("line one\r\nline two"), "line one\nline two", true},
		{kindText, "input", `"ab"`, "", str("a\nb"), "ab", true},
		// A fill the browser rejected (bad number) must stay a mismatch.
		{kindText, "input", `""`, "", str("not-a-number"), "", false},

		// Select/combobox compare against what the fill CONFIRMED, not the
		// request: the request may be an option value or a prefix.
		{kindSelect, "select", `{"value":"sg","text":"Singapore"}`, "Singapore", str("Singapore"), `Singapore (value "sg")`, true},
		{kindSelect, "select", `{"value":"sg","text":"Singapore"}`, "Singapore", str("sg"), `Singapore (value "sg")`, true},
		// Fill confirmed Singapore but a later field clobbered the select.
		{kindSelect, "select", `{"value":"my","text":"Malaysia"}`, "Singapore", str("Singapore"), `Malaysia (value "my")`, false},
		// Fallback (fill FAILED): compare against the request, exact only.
		{kindSelect, "select", `{"value":"my","text":"Malaysia"}`, "", str("sg"), `Malaysia (value "my")`, false},
		{kindSelect, "select", `{"value":"my","text":"Malaysia"}`, "", str("Malaysia"), `Malaysia (value "my")`, true},
		// An option whose text merely CONTAINS the request is a different
		// option: it must not report OK (the old Contains() check did).
		{kindSelect, "select", `{"value":"in","text":"India"}`, "", str("i"), `India (value "in")`, false},

		{kindCheckbox, "input", `true`, "", formValue{isB: true, b: true}, "true", true},
		{kindRadio, "input", `false`, "", formValue{isB: true, b: true}, "false", false},
		{kindFile, "input", `["cv.pdf"]`, "", str("/tmp/x/cv.pdf"), "cv.pdf", true},
		{kindFile, "input", `["a.pdf"]`, "", formValue{list: []string{"a.pdf", "b.pdf"}}, "a.pdf", false},

		{kindCombobox, "input", `"San Francisco, California"`, "San Francisco, California", str("san francisco"), "San Francisco, California", true},
		{kindCombobox, "input", `""`, "", str("Blue"), "", false},
		// The empty request must not pass: strings.Contains(x, "") is true
		// for every x, so the old check reported OK for any displayed label.
		{kindCombobox, "input", `"whatever"`, "", str(""), "whatever", false},
		// Clobbered: fill confirmed Blue, widget now shows Blue Grey.
		{kindCombobox, "input", `"Blue Grey"`, "Blue", str("Blue"), "Blue Grey", false},
	}
	for i, c := range cases {
		actual, ok, err := compareField(c.kind, c.tag, c.confirmed, c.v, json.RawMessage(c.raw))
		if err != nil || actual != c.actual || ok != c.ok {
			t.Errorf("case %d (%s): got %q ok=%v err=%v; want %q ok=%v", i, c.kind, actual, ok, err, c.actual, c.ok)
		}
	}
	if _, _, err := compareField("bogus", "input", "", str("x"), json.RawMessage(`"x"`)); err == nil {
		t.Error("unknown kind must error")
	}
}

// TestToTopViewportPts pins the batched frame→top conversion: one evaluate
// carries EVERY point (a scroll done for one can no longer invalidate one
// already converted), the mapped points all come back, and a --frame lookup
// failure surfaces as a plain error.
func TestToTopViewportPts(t *testing.T) {
	// Without a frame it is the identity — no round trip at all.
	noFrame, err := Dial(fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		t.Errorf("frame-less conversion must not evaluate, got %s", m.Method)
		conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer noFrame.Close()
	got, err := toTopViewportPts(noFrame, [][2]float64{{1, 2}, {3, 4}})
	if err != nil {
		t.Fatalf("no-frame conversion: %v", err)
	}
	if !reflect.DeepEqual(got, [][2]float64{{1, 2}, {3, 4}}) {
		t.Fatalf("no-frame conversion = %v", got)
	}

	var exprs []string
	framed, err := Dial(fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		if m.Method != "Runtime.evaluate" {
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
			return
		}
		var p struct {
			Expression string `json:"expression"`
		}
		json.Unmarshal(m.Params, &p)
		exprs = append(exprs, p.Expression)
		if strings.Contains(p.Expression, "BOOM") {
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"result": map[string]any{"type": "object", "value": map[string]any{
					"frameError": "--frame \"#x\": exploded"}}}})
			return
		}
		// Both points are inside the expression and come back converted.
		conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
			"result": map[string]any{"type": "object", "value": map[string]any{
				"ok": []map[string]float64{{"x": 220, "y": 140}, {"x": 260, "y": 180}}}}}})
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer framed.Close()
	framed.SetFrame("#inner")

	out, err := toTopViewportPts(framed, [][2]float64{{20, 20}, {60, 60}})
	if err != nil {
		t.Fatalf("framed conversion: %v", err)
	}
	if !reflect.DeepEqual(out, [][2]float64{{220, 140}, {260, 180}}) {
		t.Fatalf("framed conversion = %v", out)
	}
	if len(exprs) != 1 {
		t.Fatalf("want ONE evaluate carrying both points, got %d: %v", len(exprs), exprs)
	}
	for _, want := range []string{"[20,20]", "[60,60]", "elementFromPoint", "scrollBy"} {
		if !strings.Contains(exprs[0], want) {
			t.Errorf("batched expression missing %q", want)
		}
	}

	// A failed --frame lookup must surface as an error, not a zero point.
	framed.SetFrame("BOOM")
	if _, err := toTopViewportPts(framed, [][2]float64{{1, 1}}); err == nil || !strings.Contains(err.Error(), "exploded") {
		t.Fatalf("frame error = %v, want the lookup message", err)
	}
}

func TestScopedExpr(t *testing.T) {
	// One expression for every realm: the frame is chosen by the execution
	// context the expression is evaluated in (CSP-exempt), never by
	// contentWindow.eval inside the page.
	expr, err := scopedExpr("(a) => a", []any{1})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(expr, "contentDocument") || strings.Contains(expr, "eval(") || !strings.Contains(expr, "((a) => a)(...[1])") {
		t.Fatalf("scoped expr should call the function directly, without eval:\n%s", expr)
	}
}

// TestEvaluateObjectReportsExceptions pins evaluateObject surfacing a script
// exception as an error carrying its text — it used to hand the decoded
// exceptionDetails back AS the result, so elementObjectID reported every
// throw as "no element matches".
func TestEvaluateObjectReportsExceptions(t *testing.T) {
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
			"result": map[string]any{"type": "object", "subtype": "error", "className": "SyntaxError"},
			"exceptionDetails": map[string]any{
				"text":      "Uncaught",
				"exception": map[string]any{"className": "SyntaxError", "description": "SyntaxError: '#[' is not a valid selector"},
			},
		}})
	})
	s, err := Dial(url)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, err = elementObjectID(s, "#[")
	if err == nil || !strings.Contains(err.Error(), "is not a valid selector") {
		t.Fatalf("want the exception text, got %v", err)
	}
}

// TestPickSettledReportsTieOnce pins the tie short-circuit: a tie on a list
// that did not change between two polls is final, not polled to the deadline.
func TestPickSettledReportsTieOnce(t *testing.T) {
	options := []string{"Malaysia", "Malaysia (East)"}
	choice, tied, settled := pickSettled(nil, options, "Malay")
	if choice != -1 || tied == nil || settled {
		t.Fatalf("first poll: choice=%d tied=%v settled=%v, want unsettled tie", choice, tied, settled)
	}
	choice, tied, settled = pickSettled(options, options, "Malay")
	if choice != -1 || len(tied) != 2 || !settled {
		t.Fatalf("second identical poll: choice=%d tied=%v settled=%v, want settled tie", choice, tied, settled)
	}
	// An exact match settles at once; a weaker one needs an unchanged list.
	if c, _, ok := pickSettled(nil, []string{"Green", "Greenish"}, "Green"); c != 0 || !ok {
		t.Fatalf("exact match: choice=%d settled=%v", c, ok)
	}
	if c, _, ok := pickSettled(nil, []string{"Greenish"}, "Green"); c != 0 || ok {
		t.Fatalf("prefix match on first poll: choice=%d settled=%v, want unsettled", c, ok)
	}
}
