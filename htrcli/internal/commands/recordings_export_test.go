package commands

import (
	"archive/zip"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/u007/htrcli/internal/api"
)

// pngBytes is a tiny but structurally real PNG (1x1, transparent). The export
// path only moves bytes around, so the content does not need to be a real
// image — but it must round-trip byte-for-byte through base64.
var pngBytes = []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x01, 0x02, 0x03}

func dataURL(b []byte) *string {
	s := "data:image/png;base64," + base64.StdEncoding.EncodeToString(b)
	return &s
}

func webmURL(b []byte) *string {
	s := "data:audio/webm;base64," + base64.StdEncoding.EncodeToString(b)
	return &s
}

func exportSession() *api.RecordingSession {
	end := int64(1_700_000_010_000)
	return &api.RecordingSession{
		ID:        "session_1",
		Title:     "Checkout flow",
		StartTime: 1_700_000_000_000,
		EndTime:   &end,
		HasAudio:  true,
		Steps: []api.RecordingStep{
			{
				ID: "s1", Timestamp: 0, Type: "navigation",
				TabTitle: "Cart", URL: "https://shop.test/cart",
				ScreenshotData: dataURL(pngBytes),
			},
			{
				ID: "s2", Timestamp: 1500, Type: "click",
				TabTitle: "Cart", URL: "https://shop.test/cart",
				Element:        &api.RecordingElementInfo{Tag: "button", Text: "Pay now", Selector: "#pay"},
				ScreenshotData: dataURL([]byte{0x0a, 0x0b}),
				AudioData:      webmURL([]byte{0x1f, 0x2e}),
			},
			{
				ID: "s3", Timestamp: 4000, Type: "input",
				TabTitle: "Cart", URL: "https://shop.test/cart",
				Element: &api.RecordingElementInfo{Tag: "input", Selector: "#cvv"},
				// Sensitive: the value is already masked by the extension.
				InputValue:  "********",
				IsSensitive: true,
			},
		},
		Annotations: []api.RecordingAnnotation{
			{ID: "a1", Timestamp: 3000, Text: "validation error appears here",
				ScreenshotData: dataURL([]byte{0xcc, 0xdd})},
			{ID: "a2", Timestamp: 9000, Text: "no audio here"},
		},
	}
}

// readZip returns name -> bytes for every entry in the archive at path.
func readZip(t *testing.T, path string) map[string][]byte {
	t.Helper()
	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("opening zip: %v", err)
	}
	t.Cleanup(func() { r.Close() })
	out := map[string][]byte{}
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("opening %s: %v", f.Name, err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("reading %s: %v", f.Name, err)
		}
		out[f.Name] = b
	}
	return out
}

func TestRecordingExportFormatForPicksByExtension(t *testing.T) {
	cases := []struct {
		path string
		want recordingExportFormat
	}{
		{"out.json", exportJSON},
		{"out.zip", exportZip},
		{"out.ZIP", exportZip},
		{"out.md", exportMarkdown},
		{"out.markdown", exportMarkdown},
		// Unknown extensions fall back to JSON, which is what this command
		// produced before zip/markdown existed — a script passing a bare
		// filename must keep working rather than start erroring.
		{"out", exportJSON},
		{"out.txt", exportJSON},
	}
	for _, tc := range cases {
		if got := recordingExportFormatFor(tc.path); got != tc.want {
			t.Errorf("recordingExportFormatFor(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestDecodeDataURL(t *testing.T) {
	t.Run("strips the data-url prefix", func(t *testing.T) {
		got, err := decodeDataURL("data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes))
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		if string(got) != string(pngBytes) {
			t.Errorf("got %x, want %x", got, pngBytes)
		}
	})

	t.Run("accepts bare base64", func(t *testing.T) {
		got, err := decodeDataURL(base64.StdEncoding.EncodeToString(pngBytes))
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		if string(got) != string(pngBytes) {
			t.Errorf("got %x, want %x", got, pngBytes)
		}
	})

	t.Run("rejects empty", func(t *testing.T) {
		if _, err := decodeDataURL("   "); err == nil {
			t.Error("expected an error for an empty payload")
		}
	})

	t.Run("rejects non-base64", func(t *testing.T) {
		if _, err := decodeDataURL("!!!not base64!!!"); err == nil {
			t.Error("expected an error for invalid base64")
		}
	})
}

func TestBuildExportedRecordingUsesPathsNotBase64(t *testing.T) {
	exported := buildExportedRecording(exportSession())

	if len(exported.Steps) != 3 {
		t.Fatalf("got %d steps, want 3", len(exported.Steps))
	}
	if got := exported.Steps[0].ScreenshotPath; got != "screenshots/step_1.png" {
		t.Errorf("step 1 screenshot path = %q", got)
	}
	if got := exported.Steps[1].AudioPath; got != "audio/step_2.webm" {
		// Media is numbered by step position, matching the extension's ZIP.
		t.Errorf("step 2 audio path = %q, want audio/step_2.webm", got)
	}
	// A step with no media must omit the field entirely, not emit "".
	if exported.Steps[2].ScreenshotPath != "" {
		t.Errorf("media-free step should have no screenshot path, got %q", exported.Steps[2].ScreenshotPath)
	}
	if got := exported.Annotations[0].ScreenshotPath; got != "screenshots/annotation_1.png" {
		t.Errorf("annotation 1 screenshot path = %q", got)
	}
	if exported.Annotations[1].ScreenshotPath != "" {
		t.Errorf("media-free annotation should have no screenshot path")
	}

	// The critical property: no base64 anywhere in the exported manifest.
	raw, err := json.Marshal(exported)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "base64") {
		t.Error("recording.json must reference media by path, never inline base64")
	}
}

func TestWriteRecordingZipMatchesExtensionLayout(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bundle.zip")
	if err := writeRecordingZip(out, exportSession()); err != nil {
		t.Fatalf("writeRecordingZip: %v", err)
	}
	entries := readZip(t, out)

	// The layout must match src/utils/exportZip.ts exactly.
	for _, want := range []string{
		"recording.json", "README.md",
		"screenshots/step_1.png", "screenshots/step_2.png",
		"screenshots/annotation_1.png",
		"audio/step_2.webm",
	} {
		if _, ok := entries[want]; !ok {
			t.Errorf("missing entry %q (have %v)", want, keysOf(entries))
		}
	}
	// Media-free steps must not create empty files.
	if _, ok := entries["screenshots/step_3.png"]; ok {
		t.Error("media-free step 3 should not produce a screenshot file")
	}
	if _, ok := entries["audio/step_2.webm"]; !ok {
		t.Error("expected the step-2 audio note")
	}

	// Screenshot bytes must survive the base64 round-trip untouched.
	if got := entries["screenshots/step_1.png"]; string(got) != string(pngBytes) {
		t.Errorf("step_1.png = %x, want %x", got, pngBytes)
	}
	if got := entries["audio/step_2.webm"]; string(got) != string([]byte{0x1f, 0x2e}) {
		t.Errorf("step_2.webm = %x", got)
	}

	// recording.json inside the zip must parse and carry paths.
	var exported ExportedRecording
	if err := json.Unmarshal(entries["recording.json"], &exported); err != nil {
		t.Fatalf("recording.json is not valid JSON: %v", err)
	}
	if exported.Title != "Checkout flow" || len(exported.Steps) != 3 {
		t.Errorf("unexpected recording.json: %+v", exported)
	}
	if exported.Steps[0].ScreenshotPath != "screenshots/step_1.png" {
		t.Errorf("recording.json step 1 path = %q", exported.Steps[0].ScreenshotPath)
	}
}

func TestWriteRecordingZipReportsUndecodableMedia(t *testing.T) {
	bad := "data:image/png;base64,!!!not-base64!!!"
	s := exportSession()
	s.Steps[0].ScreenshotData = &bad

	out := filepath.Join(t.TempDir(), "bundle.zip")
	if err := writeRecordingZip(out, s); err != nil {
		t.Fatalf("a single corrupt screenshot must not fail the whole export: %v", err)
	}
	entries := readZip(t, out)

	// The good screenshots still land...
	if _, ok := entries["screenshots/step_2.png"]; !ok {
		t.Error("expected the intact step-2 screenshot to still be written")
	}
	// ...and the gap is disclosed in the README rather than being silent.
	readme := string(entries["README.md"])
	if !strings.Contains(readme, "Missing media") || !strings.Contains(readme, "screenshots/step_1.png") {
		t.Errorf("README should list the undecodable file; got:\n%s", readme)
	}
}

func TestGenerateRecordingMarkdownOrdersTimeline(t *testing.T) {
	md := generateRecordingMarkdown(exportSession())

	if !strings.HasPrefix(md, "# Checkout flow\n") {
		t.Errorf("missing title heading, got:\n%.120s", md)
	}
	// Steps and annotations are merged in timestamp order, so the step at
	// 4000ms must come after the annotation at 3000ms.
	annIdx := strings.Index(md, "## Annotation 1")
	lateStepIdx := strings.Index(md, "## Step 3")
	if annIdx < 0 || lateStepIdx < 0 {
		t.Fatalf("expected both an annotation and step 3 heading in:\n%s", md)
	}
	if annIdx > lateStepIdx {
		t.Error("annotation at 3000ms should be ordered before the step at 4000ms")
	}
	// A sensitive value is masked, never printed raw.
	if strings.Contains(md, "**Value:** \"********\"") {
		t.Error("sensitive input value must not be printed as a plain value")
	}
	if !strings.Contains(md, "(sensitive)") {
		t.Error("expected the sensitive marker in the markdown")
	}
	// Screenshot links point at the real bundle-relative paths.
	if !strings.Contains(md, "(screenshots/step_1.png)") {
		t.Error("expected a screenshot link in the markdown")
	}
	// Regression: an input step used to render "Input into on <input>",
	// because the element connective (" on ") was shared with click steps.
	if strings.Contains(md, "into on ") {
		t.Errorf("input heading has broken grammar 'into on':\n%s", md)
	}
	if !strings.Contains(md, "Input into <input> (sensitive)") {
		t.Errorf("expected 'Input into <input> (sensitive)' heading, got:\n%s", md)
	}
}

func TestStepDescriptionPerTypeWording(t *testing.T) {
	btn := &api.RecordingElementInfo{Tag: "button", Text: "Pay now", Selector: "#pay"}
	inp := &api.RecordingElementInfo{Tag: "input", Selector: "#cvv"}

	cases := []struct {
		name string
		step api.RecordingStep
		want string
	}{
		{"click with text", api.RecordingStep{Type: "click", Element: btn}, `Click on "Pay now"`},
		{"click without text", api.RecordingStep{Type: "click", Element: &api.RecordingElementInfo{Tag: "a"}}, "Click on <a>"},
		{"click with no element", api.RecordingStep{Type: "click"}, "Click"},
		{"sensitive input", api.RecordingStep{Type: "input", Element: inp, InputValue: "****", IsSensitive: true}, "Input into <input> (sensitive)"},
		{"plain input", api.RecordingStep{Type: "input", Element: inp, InputValue: "4242"}, `Input "4242" into <input>`},
		{"navigation with title", api.RecordingStep{Type: "navigation", TabTitle: "Cart"}, `Navigate to "Cart"`},
		{"navigation without title", api.RecordingStep{Type: "navigation"}, "Navigate"},
		{"unknown type falls through", api.RecordingStep{Type: "hover"}, "hover"},
	}
	for _, tc := range cases {
		if got := stepDescription(tc.step); got != tc.want {
			t.Errorf("%s: stepDescription = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// runRecordingsExport invokes the export command's RunE directly.
//
// It deliberately does NOT go through rootCmd.Execute(): the root persistent
// pre-run rebuilds the package client from the user's config file, which would
// replace the fake server installed by useServer. Calling RunE keeps the fake
// daemon in play while still exercising the real command body.
func runRecordingsExport(t *testing.T, sessionID, out string) *recordingServer {
	t.Helper()
	prevTransport := transportFlag
	transportFlag = "ext"
	t.Cleanup(func() { transportFlag = prevTransport })

	rs := newRecordingServer(t, "recordingGet", api.RecordingGetData{Session: *exportSession()})
	useServer(t, rs)

	if err := recordingsExportCmd.RunE(recordingsExportCmd, []string{sessionID, out}); err != nil {
		t.Fatalf("export: %v", err)
	}
	return rs
}

func TestRecordingsExportZipEndToEnd(t *testing.T) {
	out := filepath.Join(t.TempDir(), "rec.zip")
	rs := runRecordingsExport(t, "session_1", out)

	// Export must always ask for media, otherwise the file silently downgrades.
	if rs.gotOptions["includeMedia"] != true {
		t.Errorf("export must request includeMedia, got %v", rs.gotOptions["includeMedia"])
	}

	entries := readZip(t, out)
	if _, ok := entries["recording.json"]; !ok {
		t.Fatalf("expected recording.json in the bundle, have %v", keysOf(entries))
	}
	if _, ok := entries["screenshots/step_1.png"]; !ok {
		t.Error("expected screenshots/step_1.png in the bundle")
	}
}

func TestRecordingsExportMarkdownEndToEnd(t *testing.T) {
	out := filepath.Join(t.TempDir(), "rec.md")
	runRecordingsExport(t, "session_1", out)

	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("reading markdown: %v", err)
	}
	if !strings.HasPrefix(string(b), "# Checkout flow") {
		t.Errorf("unexpected markdown: %.100s", b)
	}
}

func TestRecordingsExportJSONStillWorks(t *testing.T) {
	out := filepath.Join(t.TempDir(), "rec.json")
	runRecordingsExport(t, "session_1", out)

	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("reading json: %v", err)
	}
	var got api.RecordingGetData
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("exported json does not parse: %v", err)
	}
	// The JSON form keeps the base64 screenshots inline (that is its contract).
	if got.Session.Steps[0].ScreenshotData == nil {
		t.Error("json export should keep the inline base64 screenshot")
	}
}

func TestRecordingsExportZipRejectsOverCDP(t *testing.T) {
	prevTransport := transportFlag
	transportFlag = "cdp"
	t.Cleanup(func() { transportFlag = prevTransport })

	err := recordingsExportCmd.RunE(recordingsExportCmd, []string{"s1", "out.zip"})
	if err == nil {
		t.Fatal("export must refuse over --cdp")
	}
	if !strings.Contains(err.Error(), "--transport ext") {
		t.Errorf("error should point at the ext transport, got: %v", err)
	}
}

func keysOf(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
