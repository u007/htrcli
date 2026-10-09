package commands

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/u007/htrcli/internal/cdp"
	"github.com/u007/htrcli/internal/output"
)

var fillFormFile string

var fillFormCmd = &cobra.Command{
	Annotations: FrameAwareAnnotations(),
	Use:         "fill-form [json]",
	Short:       "Fill many form fields in one CDP session, then verify each (--cdp only)",
	Long: `Fill a whole form in one call. The JSON maps selector -> value and is applied
in the order written (or pass an array of {"selector": ..., "value": ...}).

Value by detected element:
  text input / textarea   string (numbers, dates and times as written, e.g. "09:30")
  <select>                option value or visible text
  checkbox                true / false (idempotent)
  radio                   true (checks it)
  combobox / react-select option text (same as pick)
  file input              path or array of paths
  any                     null skips the field

Every field is attempted even after a failure. Afterwards every field is read
back and a table of selector / requested / actual / status is printed. Exits
non-zero if any field FAILED or is a MISMATCH.

Examples:
  htrcli --cdp fill-form '{"#first": "Ada", "#country": "Malaysia", "#terms": true}'
  htrcli --cdp fill-form --file form.json
  htrcli --cdp --frame "#grnhse_iframe" fill-form --file form.json`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !UseCDP() {
			return errCDPOnly("fill-form")
		}
		data, err := fillFormInput(args, fillFormFile)
		if err != nil {
			return err
		}
		entries, err := parseFillFormSpec(data)
		if err != nil {
			return err
		}
		return runFillFormCDP(entries)
	},
}

// fillFormEntry is one parsed selector -> raw JSON value pair.
type fillFormEntry struct {
	Selector string
	Value    json.RawMessage
}

// fillFormInput returns the JSON from exactly one of the positional argument
// or --file.
func fillFormInput(args []string, file string) ([]byte, error) {
	switch {
	case len(args) == 1 && file != "":
		return nil, fmt.Errorf("fill-form: pass the JSON either as an argument or with --file, not both")
	case len(args) == 1:
		return []byte(args[0]), nil
	case file != "":
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("fill-form: reading %s: %w", file, err)
		}
		return data, nil
	default:
		return nil, fmt.Errorf("fill-form: pass the form JSON as an argument or with --file")
	}
}

// parseFillFormSpec decodes the form JSON, preserving field order. It
// accepts an object {"selector": value, ...} (decoded token by token, since
// Go maps lose order) or an array [{"selector": ..., "value": ...}, ...].
func parseFillFormSpec(data []byte) ([]fillFormEntry, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("fill-form: invalid JSON: %w", err)
	}
	var entries []fillFormEntry
	switch tok {
	case json.Delim('{'):
		entries, err = parseFillFormObject(dec)
	case json.Delim('['):
		entries, err = parseFillFormArray(dec)
	default:
		return nil, fmt.Errorf("fill-form: expected a JSON object or array, got %v", tok)
	}
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("fill-form: unexpected data after the form JSON")
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("fill-form: no fields given")
	}
	return entries, nil
}

func parseFillFormObject(dec *json.Decoder) ([]fillFormEntry, error) {
	var entries []fillFormEntry
	seen := map[string]bool{}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("fill-form: invalid JSON: %w", err)
		}
		key, ok := tok.(string)
		if !ok {
			return nil, fmt.Errorf("fill-form: expected a selector key, got %v", tok)
		}
		if key == "" {
			return nil, fmt.Errorf("fill-form: empty selector key")
		}
		if seen[key] {
			return nil, fmt.Errorf("fill-form: selector %q appears twice", key)
		}
		seen[key] = true
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, fmt.Errorf("fill-form: value for %q: %w", key, err)
		}
		entries = append(entries, fillFormEntry{Selector: key, Value: raw})
	}
	if _, err := dec.Token(); err != nil { // closing '}'
		return nil, fmt.Errorf("fill-form: invalid JSON: %w", err)
	}
	return entries, nil
}

func parseFillFormArray(dec *json.Decoder) ([]fillFormEntry, error) {
	var entries []fillFormEntry
	for i := 0; dec.More(); i++ {
		var obj map[string]json.RawMessage
		if err := dec.Decode(&obj); err != nil {
			return nil, fmt.Errorf("fill-form: item %d must be an object {\"selector\", \"value\"}: %w", i, err)
		}
		for k := range obj {
			if k != "selector" && k != "value" {
				return nil, fmt.Errorf("fill-form: item %d has unknown key %q (want selector, value)", i, k)
			}
		}
		rawSel, ok := obj["selector"]
		if !ok {
			return nil, fmt.Errorf("fill-form: item %d has no \"selector\"", i)
		}
		var sel string
		if err := json.Unmarshal(rawSel, &sel); err != nil || sel == "" {
			return nil, fmt.Errorf("fill-form: item %d: \"selector\" must be a non-empty string", i)
		}
		value, ok := obj["value"]
		if !ok {
			return nil, fmt.Errorf("fill-form: item %d (%s) has no \"value\" (use null to skip)", i, sel)
		}
		entries = append(entries, fillFormEntry{Selector: sel, Value: value})
	}
	if _, err := dec.Token(); err != nil { // closing ']'
		return nil, fmt.Errorf("fill-form: invalid JSON: %w", err)
	}
	return entries, nil
}

func runFillFormCDP(entries []fillFormEntry) error {
	fields := make([]cdp.FormField, len(entries))
	for i, e := range entries {
		sel := parseSelector(e.Selector)
		if err := checkFrameTarget(sel); err != nil {
			return fmt.Errorf("fill-form: %s: %w", e.Selector, err)
		}
		fields[i] = cdp.FormField{Selector: e.Selector, Target: sel, Value: e.Value}
	}
	s, targetID, err := cdpSession()
	if err != nil {
		return err
	}
	defer s.Close()

	reports := cdp.FillForm(s, targetID, fields)
	bad := 0
	for _, r := range reports {
		if r.Status == cdp.FieldFailed || r.Status == cdp.FieldMismatch {
			bad++
		}
	}
	if output.JSONOutput {
		output.PrintJSON(map[string]any{"success": bad == 0, "fields": reports})
	} else {
		printFillFormReport(reports)
	}
	if bad > 0 {
		msg := fmt.Sprintf("fill-form: %d of %d fields failed or mismatched", bad, len(reports))
		if output.JSONOutput {
			return &reportedJSONError{msg: msg} // the report above is the one JSON document
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

func printFillFormReport(reports []cdp.FieldReport) {
	table := output.NewTable("Selector", "Requested", "Actual", "Status")
	for _, r := range reports {
		table.AddRow(r.Selector, r.Requested, r.Actual, r.Status)
	}
	fmt.Print(table)
	for _, r := range reports {
		if r.Error != "" {
			fmt.Printf("%s %s: %s\n", r.Status, r.Selector, r.Error)
		}
	}
}

func init() {
	fillFormCmd.Flags().StringVar(&fillFormFile, "file", "", "read the form JSON from this file")
	rootCmd.AddCommand(fillFormCmd)
}
