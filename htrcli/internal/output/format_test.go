package output

import (
	"bytes"
	"os"
	"testing"
)

func TestPrintJSON(t *testing.T) {
	// Capture stdout.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	PrintJSON(map[string]string{"key": "value"})

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !bytes.Contains([]byte(output), []byte(`"key": "value"`)) {
		t.Errorf("expected JSON output to contain key:value, got %s", output)
	}
}

func TestPrintJSONRaw(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	PrintJSONRaw([]byte(`{"ok":true}`))

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !bytes.Contains([]byte(output), []byte(`"ok": true`)) {
		t.Errorf("expected pretty JSON, got %s", output)
	}
}

func TestPrintOrJSON_JSON(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	JSONOutput = true
	PrintOrJSON(map[string]string{"data": "test"}, func() {
		print("human output")
	})

	JSONOutput = false
	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !bytes.Contains([]byte(output), []byte(`"data": "test"`)) {
		t.Errorf("expected JSON output, got %q", output)
	}
}
