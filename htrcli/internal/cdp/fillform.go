package cdp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/u007/htrcli/internal/api"
)

// FormField is one fill-form step: a selector and the raw JSON value to put
// into whatever element it matches. A JSON null value skips the field.
type FormField struct {
	Selector string              // as written by the user; the report key
	Target   *api.TargetSelector // parsed form of Selector
	Value    json.RawMessage
}

// Field statuses in a fill-form report.
const (
	FieldOK       = "OK"
	FieldMismatch = "MISMATCH"
	FieldFailed   = "FAILED"
	FieldSkipped  = "SKIPPED"
)

// FieldReport is one row of the fill-form read-back table.
type FieldReport struct {
	Selector  string `json:"selector"`
	Kind      string `json:"kind,omitempty"`
	Requested string `json:"requested"`
	Actual    string `json:"actual"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
}

// Field kinds, detected from the matched element.
const (
	kindText     = "text"
	kindSelect   = "select"
	kindCheckbox = "checkbox"
	kindRadio    = "radio"
	kindFile     = "file"
	kindCombobox = "combobox"
)

// formValue is a decoded fill-form value.
type formValue struct {
	null  bool
	isB   bool
	b     bool
	isStr bool
	str   string
	list  []string // JSON array of strings (file uploads)
}

func (v formValue) String() string {
	switch {
	case v.null:
		return "null"
	case v.isB:
		return fmt.Sprintf("%t", v.b)
	case v.isStr:
		return v.str
	default:
		return strings.Join(v.list, ", ")
	}
}

// decodeFormValue accepts null, a bool, a string, a number (kept as its
// literal text, e.g. 1.50) or an array of strings.
func decodeFormValue(raw json.RawMessage) (formValue, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return formValue{}, fmt.Errorf("missing value")
	}
	switch trimmed[0] {
	case 'n':
		if string(trimmed) != "null" {
			return formValue{}, fmt.Errorf("invalid value %s", trimmed)
		}
		return formValue{null: true}, nil
	case 't', 'f':
		var b bool
		if err := json.Unmarshal(trimmed, &b); err != nil {
			return formValue{}, fmt.Errorf("invalid value %s: %w", trimmed, err)
		}
		return formValue{isB: true, b: b}, nil
	case '"':
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return formValue{}, fmt.Errorf("invalid string value %s: %w", trimmed, err)
		}
		return formValue{isStr: true, str: s}, nil
	case '[':
		var list []string
		if err := json.Unmarshal(trimmed, &list); err != nil {
			return formValue{}, fmt.Errorf("array values must be arrays of strings (file paths): %w", err)
		}
		if len(list) == 0 {
			return formValue{}, fmt.Errorf("empty array value")
		}
		return formValue{list: list}, nil
	case '{':
		return formValue{}, fmt.Errorf("object values are not supported")
	default:
		var n json.Number
		dec := json.NewDecoder(bytes.NewReader(trimmed))
		dec.UseNumber()
		if err := dec.Decode(&n); err != nil {
			return formValue{}, fmt.Errorf("invalid value %s: %w", trimmed, err)
		}
		return formValue{isStr: true, str: n.String()}, nil
	}
}

// elementKind maps one matched element to a field kind.
func elementKind(in ElementInfo) (string, error) {
	class := in.Attributes["class"]
	switch {
	case in.Tag == "select":
		return kindSelect, nil
	case in.Attributes["role"] == "combobox" || strings.Contains(class, "select__"):
		return kindCombobox, nil
	case in.Tag == "textarea":
		return kindText, nil
	case in.Tag == "input":
		switch t := strings.ToLower(in.Type); t {
		case "checkbox":
			return kindCheckbox, nil
		case "radio":
			return kindRadio, nil
		case "file":
			return kindFile, nil
		case "hidden", "submit", "button", "reset", "image":
			return "", fmt.Errorf("<input type=%s> cannot be filled", t)
		default:
			return kindText, nil
		}
	default:
		return "", fmt.Errorf("unsupported element <%s> (expected input, textarea, select or a combobox)", in.Tag)
	}
}

// classifyField detects the field kind for a selector's matches. Exactly one
// element must match: several checkboxes/radios take one selector each, so
// every bool in the form names exactly the box it sets.
func classifyField(infos []ElementInfo) (string, error) {
	if len(infos) == 0 {
		return "", fmt.Errorf("no element matches")
	}
	kind, err := elementKind(infos[0])
	if err != nil {
		return "", err
	}
	if len(infos) > 1 {
		return kind, fmt.Errorf("selector matches %d elements; use a selector that matches exactly one (one selector per checkbox/radio)", len(infos))
	}
	return kind, nil
}

// FillForm applies every field in order inside one CDP session, then reads
// every field back. A failing field never stops the batch: each problem is
// reported in its row so one run surfaces all of them. Frame-aware.
func FillForm(s *Session, targetID string, fields []FormField) []FieldReport {
	reports := make([]FieldReport, len(fields))
	values := make([]formValue, len(fields))
	// confirmed[i] is the state the fill verifiably produced (the <select>
	// option text or the combobox label pick verified after writing).
	// Read-back compares against it instead of re-matching the raw request:
	// the request may be an option value, a prefix, or text the widget never
	// displays, and a Contains() match against it reports OK for a field the
	// fill never actually set.
	confirmed := make([]string, len(fields))
	for i, f := range fields {
		r := &reports[i]
		r.Selector = f.Selector
		v, err := decodeFormValue(f.Value)
		if err != nil {
			r.Status, r.Error = FieldFailed, err.Error()
			continue
		}
		values[i] = v
		r.Requested = v.String()
		if v.null {
			r.Status = FieldSkipped
			continue
		}
		kind, ferr := applyField(s, targetID, f.Target, v, &confirmed[i])
		r.Kind = kind
		if ferr != nil {
			r.Status, r.Error = FieldFailed, ferr.Error()
		}
	}

	// Read back after ALL writes, so a later field clobbering an earlier
	// one (dependent selects, re-rendering widgets) is caught.
	for i, f := range fields {
		r := &reports[i]
		if r.Status == FieldSkipped || r.Kind == "" {
			continue
		}
		actual, ok, err := readBackField(s, f.Target, r.Kind, confirmed[i], values[i])
		r.Actual = actual
		switch {
		case r.Status == FieldFailed:
			if err != nil {
				r.Error += "; read-back: " + err.Error()
			}
		case err != nil:
			r.Status, r.Error = FieldFailed, "read-back: "+err.Error()
		case ok:
			r.Status = FieldOK
		default:
			r.Status = FieldMismatch
		}
	}
	return reports
}

// applyField writes one value and returns the detected kind. For <select>
// and combobox fields it also records in *confirmed what the fill verifiably
// produced (the option text / displayed label pick verified after writing).
// On failure *confirmed is left untouched; the row is FAILED and read-back
// ignores ok for it.
func applyField(s *Session, targetID string, target *api.TargetSelector, v formValue, confirmed *string) (string, error) {
	infos, err := findElements(s, target)
	if err != nil {
		return "", err
	}
	kind, err := classifyField(infos)
	if err != nil {
		return kind, err
	}
	path := infos[0].Path
	switch kind {
	case kindText:
		if !v.isStr {
			return kind, fmt.Errorf("text field needs a string value, got %s", v)
		}
		return kind, execOK(s, "fill", path, v.str)
	case kindSelect:
		if !v.isStr {
			return kind, fmt.Errorf("<select> needs a string value (option value or visible text), got %s", v)
		}
		selected, err := pickSelect(s, path, v.str)
		if err == nil {
			*confirmed = selected
		}
		return kind, err
	case kindCheckbox:
		if !v.isB {
			return kind, fmt.Errorf("checkbox needs true or false, got %q", v.String())
		}
		action := "uncheck"
		if v.b {
			action = "check"
		}
		return kind, execOK(s, action, path, "")
	case kindRadio:
		if !v.isB {
			return kind, fmt.Errorf("radio needs true, got %q", v.String())
		}
		if !v.b {
			return kind, fmt.Errorf("a radio cannot be unchecked directly; set another radio in its group to true")
		}
		return kind, execOK(s, "check", path, "")
	case kindFile:
		files, err := uploadPaths(v)
		if err != nil {
			return kind, err
		}
		objectID, err := elementObjectID(s, path)
		if err != nil {
			return kind, err
		}
		if err := s.Call("DOM.setFileInputFiles", map[string]any{"objectId": objectID, "files": files}, nil); err != nil {
			return kind, fmt.Errorf("DOM.setFileInputFiles: %w", err)
		}
		return kind, nil
	case kindCombobox:
		if !v.isStr {
			return kind, fmt.Errorf("combobox needs a string value, got %s", v)
		}
		shown, err := pickCombobox(s, targetID, path, v.str)
		if err == nil {
			*confirmed = shown
		}
		return kind, err
	default:
		return kind, fmt.Errorf("unhandled field kind %q", kind)
	}
}

// execOK runs one bundle verb on a CSS path and converts failure to an error.
func execOK(s *Session, action, path, value string) error {
	res, err := ExecDOM(s, api.Command{ID: "fill-form", Action: action,
		Target: &api.TargetSelector{Selector: path}, Value: value})
	if err != nil {
		return err
	}
	if !res.Success {
		return fmt.Errorf("%s failed: %s", action, res.Error)
	}
	return nil
}

// uploadPaths validates file-upload values: each must exist locally, and is
// made absolute because Chrome resolves relative paths against its own cwd.
func uploadPaths(v formValue) ([]string, error) {
	var in []string
	switch {
	case v.isStr:
		in = []string{v.str}
	case v.list != nil:
		in = v.list
	default:
		return nil, fmt.Errorf("file input needs a path or an array of paths, got %s", v)
	}
	out := make([]string, len(in))
	for i, p := range in {
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, fmt.Errorf("resolving %s: %w", p, err)
		}
		if _, err := os.Stat(abs); err != nil {
			return nil, fmt.Errorf("file not found: %s", p)
		}
		out[i] = abs
	}
	return out, nil
}

// readBackField reads a field's current state and compares it with what the
// fill verifiably produced (confirmed), falling back to the request when the
// fill failed (read-back then only enriches the FAILED error). tag selects
// the browser's value normalization for text fields.
func readBackField(s *Session, target *api.TargetSelector, kind, confirmed string, v formValue) (string, bool, error) {
	infos, err := findElements(s, target)
	if err != nil {
		return "", false, err
	}
	if len(infos) != 1 {
		return "", false, fmt.Errorf("selector now matches %d elements", len(infos))
	}
	raw, err := callKit(s, "readField", infos[0].Path, kind)
	if err != nil {
		return "", false, err
	}
	return compareField(kind, infos[0].Tag, confirmed, v, raw)
}

// normalizeTextForTag applies the same value sanitization the browser applies
// to a text field, on BOTH sides, so a fill the browser itself rewrites is
// not reported as a mismatch. <textarea> canonicalizes line endings to LF;
// <input> strips newlines entirely (HTML value sanitization). Other
// sanitizations (an unparseable date/number becoming "") are deliberately NOT
// normalized away: there the fill genuinely did not land and the mismatch is
// the point.
func normalizeTextForTag(tag, s string) string {
	if tag == "textarea" {
		s = strings.ReplaceAll(s, "\r\n", "\n")
		return strings.ReplaceAll(s, "\r", "\n")
	}
	s = strings.ReplaceAll(s, "\r\n", "")
	s = strings.ReplaceAll(s, "\n", "")
	return strings.ReplaceAll(s, "\r", "")
}

// compareField renders the read-back value and decides whether it matches.
// confirmed is what applyField verified the fill produced ("" when the fill
// failed or the kind has nothing to confirm); v is the raw request, used only
// as a fallback. tag is the matched element's tag name.
func compareField(kind, tag, confirmed string, v formValue, raw json.RawMessage) (string, bool, error) {
	switch kind {
	case kindText:
		var got string
		if err := json.Unmarshal(raw, &got); err != nil {
			return "", false, fmt.Errorf("decoding value: %w", err)
		}
		return got, normalizeTextForTag(tag, got) == normalizeTextForTag(tag, v.str), nil
	case kindSelect:
		var got selectOption
		if err := json.Unmarshal(raw, &got); err != nil {
			return "", false, fmt.Errorf("decoding <select> value: %w", err)
		}
		expected, fallback := confirmed, confirmed == ""
		if fallback {
			expected = v.str
		}
		// Exact (normalized) equality with what pickSelect verified — never a
		// substring match: an option whose text merely contains the request is
		// a different option, and an empty request must not pass at all.
		ok := normOption(got.Text) == normOption(expected)
		if fallback && got.Value == v.str {
			ok = true
		}
		return fmt.Sprintf("%s (value %q)", got.Text, got.Value), ok, nil
	case kindCheckbox, kindRadio:
		var got bool
		if err := json.Unmarshal(raw, &got); err != nil {
			return "", false, fmt.Errorf("decoding checked state: %w", err)
		}
		return fmt.Sprintf("%t", got), got == v.b, nil
	case kindFile:
		var got []string
		if err := json.Unmarshal(raw, &got); err != nil {
			return "", false, fmt.Errorf("decoding file list: %w", err)
		}
		want := v.list
		if v.isStr {
			want = []string{v.str}
		}
		ok := len(got) == len(want)
		for i := 0; ok && i < len(want); i++ {
			ok = got[i] == filepath.Base(want[i])
		}
		return strings.Join(got, ", "), ok, nil
	case kindCombobox:
		var got string
		if err := json.Unmarshal(raw, &got); err != nil {
			return "", false, fmt.Errorf("decoding combobox value: %w", err)
		}
		expected := confirmed
		if expected == "" {
			expected = v.str
		}
		// Exact (normalized) equality with what pickCombobox verified.
		// strings.Contains must not be used here: strings.Contains(x, "")
		// is always true, so an empty request — or any substring of the
		// displayed label — would report OK for a field that never changed.
		return got, normOption(got) == normOption(expected), nil
	default:
		return "", false, fmt.Errorf("unknown field kind %q", kind)
	}
}
