package cdp

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/u007/htrcli/internal/api"
)

//go:embed formkit.js
var formKit string

const (
	// pickOptionsWait bounds how long pick polls for a matching option after
	// typing (async autocompletes answer after a network round trip).
	pickOptionsWait = 3 * time.Second
	// pickVerifyWait bounds how long pick waits for the widget to display
	// the clicked option (react-select re-renders after the click).
	pickVerifyWait = 2 * time.Second
	pickPollEvery  = 100 * time.Millisecond
	// pickListMax caps the options listed in a failure message.
	pickListMax = 15
)

// formKitSpec installs formkit.js once per realm as window.__htrcliFormKit;
// callKit then invokes methods by name instead of re-shipping the source on
// every call (pick polls every 100 ms, fill-form calls it per field).
//
// The source is wrapped in parentheses, so its trailing semicolon has to go:
// "(...)" followed by ";" is a syntax error, and the realm would reject the
// whole install with "Unexpected token ';'".
var formKitSpec = pageBundle{name: "form kit", install: func() string {
	return "window.__htrcliFormKit = (\n" + trimTrailingSemicolon(formKit) + "\n)"
}}

// trimTrailingSemicolon removes the trailing semicolon (and the whitespace
// after it) from a JavaScript expression.
func trimTrailingSemicolon(src string) string {
	return strings.TrimRight(strings.TrimSpace(src), ";")
}

const callKitFn = `(m, a) => typeof window.__htrcliFormKit === "object" ? window.__htrcliFormKit[m](...a) : {__htrcliNeedBundle: true}`

// callKit runs one formkit.js method in the session's realm (frame-aware).
func callKit(s *Session, method string, args ...any) (json.RawMessage, error) {
	if args == nil {
		args = []any{}
	}
	return withBundle(s, formKitSpec, func() (json.RawMessage, error) {
		return runScoped(s, callKitFn, method, args)
	})
}

// ElementInfo is the subset of the DOM bundle's element info that pick and
// fill-form need: Path is a CSS path that querySelector resolves back to the
// element, so htrcli selector syntax (name=, label=, …) works everywhere.
type ElementInfo struct {
	Tag        string            `json:"tag"`
	Type       string            `json:"type"`
	Path       string            `json:"selector"`
	Attributes map[string]string `json:"attributes"`
}

// findElements returns every element matched by sel in the session's realm.
func findElements(s *Session, sel *api.TargetSelector) ([]ElementInfo, error) {
	res, err := ExecDOM(s, api.Command{ID: "find", Action: "findAll", Target: sel})
	if err != nil {
		return nil, err
	}
	if !res.Success {
		return nil, fmt.Errorf("findAll failed: %s", res.Error)
	}
	data, err := json.Marshal(res.Data)
	if err != nil {
		return nil, fmt.Errorf("re-encoding findAll data: %w", err)
	}
	var infos []ElementInfo
	if err := json.Unmarshal(data, &infos); err != nil {
		return nil, fmt.Errorf("decoding findAll data: %w", err)
	}
	for _, in := range infos {
		if in.Path == "" {
			return nil, fmt.Errorf("findAll returned an element without a CSS path (tag %q)", in.Tag)
		}
	}
	return infos, nil
}

// normOption lower-cases, trims and collapses whitespace for comparison.
func normOption(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// optionRank scores how well an option text matches the wanted text:
// 3 exact, 2 prefix, 1 contains, 0 no match (all case-insensitive, after
// whitespace normalisation).
func optionRank(option, want string) int {
	o, w := normOption(option), normOption(want)
	switch {
	case w == "":
		return 0
	case o == w:
		return 3
	case strings.HasPrefix(o, w):
		return 2
	case strings.Contains(o, w):
		return 1
	default:
		return 0
	}
}

// bestOption picks the option that best matches want. It returns the index
// of the unique best match; -1 with a nil slice when nothing matches; or -1
// with the tied option texts when several options share the best rank
// (ambiguous — the caller must not guess).
func bestOption(options []string, want string) (int, []string) {
	bestRank, best := 0, []int{}
	for i, o := range options {
		r := optionRank(o, want)
		switch {
		case r == 0:
		case r > bestRank:
			bestRank, best = r, []int{i}
		case r == bestRank:
			best = append(best, i)
		}
	}
	if len(best) == 0 {
		return -1, nil
	}
	if len(best) > 1 {
		tied := make([]string, len(best))
		for i, idx := range best {
			tied[i] = options[idx]
		}
		return -1, tied
	}
	return best[0], nil
}

// sameOptions reports whether two polled option lists are identical.
func sameOptions(a, b []string) bool {
	if a == nil || len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// listOptions renders up to pickListMax options for an error message.
func listOptions(options []string) string {
	if len(options) == 0 {
		return "(no options visible)"
	}
	shown := options
	more := ""
	if len(shown) > pickListMax {
		shown = shown[:pickListMax]
		more = fmt.Sprintf(" … and %d more", len(options)-pickListMax)
	}
	quoted := make([]string, len(shown))
	for i, o := range shown {
		quoted[i] = fmt.Sprintf("%q", o)
	}
	return strings.Join(quoted, ", ") + more
}

// PickResult reports what pick selected and how.
type PickResult struct {
	Mode     string `json:"mode"`     // "select" or "combobox"
	Selected string `json:"selected"` // the displayed selected value after the pick
}

// Pick chooses the option matching text in the element matched by sel: a
// native <select> (resolved to an option, then the bundle's select action)
// or a custom combobox such as react-select / an async autocomplete
// (trusted ArrowDown + Input.insertText, poll for options, trusted click on
// the best match, then verify the widget displays it). Frame-aware.
func Pick(s *Session, targetID string, sel *api.TargetSelector, text string) (*PickResult, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("pick: option text cannot be empty")
	}
	infos, err := findElements(s, sel)
	if err != nil {
		return nil, err
	}
	if len(infos) == 0 {
		return nil, fmt.Errorf("pick: no element matches the selector")
	}
	el := infos[0]
	if el.Tag == "select" {
		selected, err := pickSelect(s, el.Path, text)
		if err != nil {
			return nil, err
		}
		return &PickResult{Mode: "select", Selected: selected}, nil
	}
	selected, err := pickCombobox(s, targetID, el.Path, text)
	if err != nil {
		return nil, err
	}
	return &PickResult{Mode: "combobox", Selected: selected}, nil
}

type selectOption struct {
	Value string `json:"value"`
	Text  string `json:"text"`
}

// resolveSelectOption maps want onto an option value: an exact value match
// first, else the best-ranked visible text (exact > prefix > contains).
func resolveSelectOption(options []selectOption, want string) (selectOption, error) {
	var byValue []selectOption
	for _, o := range options {
		if o.Value == want {
			byValue = append(byValue, o)
		}
	}
	if len(byValue) == 1 {
		return byValue[0], nil
	}
	texts := make([]string, len(options))
	for i, o := range options {
		texts[i] = o.Text
	}
	idx, tied := bestOption(texts, want)
	if tied != nil {
		return selectOption{}, fmt.Errorf("%q is ambiguous: it matches %s equally well", want, listOptions(tied))
	}
	if idx < 0 {
		return selectOption{}, fmt.Errorf("no option matches %q; available: %s", want, listOptions(texts))
	}
	return options[idx], nil
}

// pickSelect selects want in the native <select> at path and returns the
// selected option's text after verifying the value took. The bundle's select
// action assigns .value directly, so a visible label passed straight through
// would silently select nothing — resolve it to the option value first.
func pickSelect(s *Session, path, want string) (string, error) {
	raw, err := callKit(s, "selectOptions", path)
	if err != nil {
		return "", err
	}
	var options []selectOption
	if err := json.Unmarshal(raw, &options); err != nil {
		return "", fmt.Errorf("decoding <select> options: %w", err)
	}
	opt, err := resolveSelectOption(options, want)
	if err != nil {
		return "", fmt.Errorf("pick: %w", err)
	}
	res, err := ExecDOM(s, api.Command{ID: "pick", Action: "select",
		Target: &api.TargetSelector{Selector: path}, Value: opt.Value})
	if err != nil {
		return "", err
	}
	if !res.Success {
		return "", fmt.Errorf("select failed: %s", res.Error)
	}
	raw, err = callKit(s, "readSelect", path)
	if err != nil {
		return "", err
	}
	var got selectOption
	if err := json.Unmarshal(raw, &got); err != nil {
		return "", fmt.Errorf("decoding <select> read-back: %w", err)
	}
	if got.Value != opt.Value {
		return "", fmt.Errorf("pick: selected %q (value %q) but the <select> now holds value %q (%q)", opt.Text, opt.Value, got.Value, got.Text)
	}
	return got.Text, nil
}

// comboUnfilteredOptions clears the query, reopens the menu with ArrowDown,
// reads the options it shows, then closes it again with Escape. Used only to
// enrich a no-match error.
func comboUnfilteredOptions(s *Session, targetID, path string) ([]string, error) {
	if _, err := callKit(s, "comboPrepare", path); err != nil {
		return nil, err
	}
	if err := Press(s, targetID, "ArrowDown"); err != nil {
		return nil, err
	}
	var options []string
	for deadline := time.Now().Add(pickVerifyWait); ; {
		raw, err := callKit(s, "comboOptions", path)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &options); err != nil {
			return nil, fmt.Errorf("decoding combobox options: %w", err)
		}
		if len(options) > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(pickPollEvery)
	}
	if err := Press(s, targetID, "Escape"); err != nil {
		return nil, err
	}
	return options, nil
}

// pickSettled decides whether one poll of the option list is conclusive. An
// exact match is taken at once. A weaker (prefix/contains) match — or a tie
// between several options — is only final once the list is unchanged across
// two polls, so a stale or half-loaded async list cannot win over (or hide)
// the exact option still loading; a tie on a settled list is reported
// immediately instead of being polled until the deadline.
func pickSettled(prev, options []string, want string) (choice int, tied []string, settled bool) {
	choice, tied = bestOption(options, want)
	switch {
	case choice >= 0:
		settled = optionRank(options[choice], want) == 3 || sameOptions(prev, options)
	case tied != nil:
		settled = sameOptions(prev, options)
	}
	return choice, tied, settled
}

// pickCombobox drives a custom combobox with trusted input.
func pickCombobox(s *Session, targetID, path, want string) (string, error) {
	raw, err := callKit(s, "comboPrepare", path)
	if err != nil {
		return "", err
	}
	var prep struct {
		Focused bool `json:"focused"`
	}
	if err := json.Unmarshal(raw, &prep); err != nil {
		return "", fmt.Errorf("decoding combobox prepare: %w", err)
	}
	if !prep.Focused {
		return "", fmt.Errorf("pick: could not focus the combobox input at %s", path)
	}
	// Press activates the target first (CDP input is dropped on unrendered tabs).
	if err := Press(s, targetID, "ArrowDown"); err != nil {
		return "", fmt.Errorf("pick: opening the menu: %w", err)
	}
	if err := s.Call("Input.insertText", map[string]any{"text": want}, nil); err != nil {
		return "", fmt.Errorf("pick: typing %q: %w", want, err)
	}
	// From here on the menu is open with the query typed: close it on every
	// failure so it cannot overlay the next widget (fill-form keeps going).
	fail := func(err error) (string, error) {
		if perr := Press(s, targetID, "Escape"); perr != nil {
			err = fmt.Errorf("%w (closing the menu with Escape also failed: %v)", err, perr)
		}
		return "", err
	}

	var options, prev, tied []string
	choice := -1
	for deadline := time.Now().Add(pickOptionsWait); ; {
		raw, err := callKit(s, "comboOptions", path)
		if err != nil {
			return fail(err)
		}
		prev, options = options, nil
		if err := json.Unmarshal(raw, &options); err != nil {
			return fail(fmt.Errorf("decoding combobox options: %w", err))
		}
		var settled bool
		choice, tied, settled = pickSettled(prev, options, want)
		if settled || time.Now().After(deadline) {
			break
		}
		time.Sleep(pickPollEvery)
	}
	if tied != nil {
		return fail(fmt.Errorf("pick: %q is ambiguous: it matches %s equally well; pass the full option text", want, listOptions(tied)))
	}
	if choice < 0 {
		if len(options) == 0 {
			// The typed text filtered everything out: show the unfiltered menu
			// so the caller can see what the widget actually offers.
			available, err := comboUnfilteredOptions(s, targetID, path)
			if err != nil {
				return fail(fmt.Errorf("pick: no option matching %q appeared within %s (listing the unfiltered menu also failed: %w)", want, pickOptionsWait, err))
			}
			return "", fmt.Errorf("pick: no option matching %q appeared within %s; the menu without a query offers: %s", want, pickOptionsWait, listOptions(available))
		}
		return fail(fmt.Errorf("pick: no option matching %q appeared within %s; available: %s", want, pickOptionsWait, listOptions(options)))
	}
	chosen := options[choice]

	raw, err = callKit(s, "optionPoint", path, chosen)
	if err != nil {
		return fail(fmt.Errorf("pick: locating option %q: %w", chosen, err))
	}
	var pt struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	}
	if err := json.Unmarshal(raw, &pt); err != nil {
		return fail(fmt.Errorf("decoding option point: %w", err))
	}
	x, y, err := toTopViewport(s, pt.X, pt.Y)
	if err != nil {
		return fail(err)
	}
	for _, ev := range []struct {
		typ     string
		buttons int
	}{{"mouseMoved", 0}, {"mousePressed", 1}, {"mouseReleased", 0}} {
		params := map[string]any{"type": ev.typ, "x": x, "y": y, "buttons": ev.buttons, "modifiers": 0}
		if ev.typ == "mouseMoved" {
			params["button"] = "none"
		} else {
			params["button"] = "left"
			params["clickCount"] = 1
		}
		if err := s.Call("Input.dispatchMouseEvent", params, nil); err != nil {
			return fail(fmt.Errorf("pick: dispatch %s on option %q: %w", ev.typ, chosen, err))
		}
	}

	var shown struct {
		Text string `json:"text"`
		From string `json:"from"`
	}
	for deadline := time.Now().Add(pickVerifyWait); ; {
		raw, err := callKit(s, "comboValue", path)
		if err != nil {
			return fail(err)
		}
		if err := json.Unmarshal(raw, &shown); err != nil {
			return fail(fmt.Errorf("decoding combobox value: %w", err))
		}
		// Verify against the option actually clicked, not the (fuzzy) request:
		// an empty or partial request would match any stale label or chip.
		if c := normOption(chosen); c != "" && strings.Contains(normOption(shown.Text), c) {
			return shown.Text, nil
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(pickPollEvery)
	}
	return fail(fmt.Errorf("pick: clicked option %q at (%.0f, %.0f) but after %s the combobox shows %q (read from %s), which does not contain %q; options were: %s",
		chosen, x, y, pickVerifyWait, shown.Text, shown.From, want, listOptions(options)))
}
