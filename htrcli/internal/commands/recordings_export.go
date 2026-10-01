package commands

import (
	"archive/zip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/u007/htrcli/internal/api"
)

// Export formats for `htrcli recordings export <id> <output>`.
//
// The format is chosen from the output file's extension so the existing
// `output.json` form keeps working unchanged:
//
//	.json                the raw recordingGet payload (base64 screenshots inline)
//	.zip                 a bundle matching the extension's own "Export ZIP"
//	.md / .markdown      a human-readable timeline
//
// The ZIP layout is deliberately identical to src/utils/exportZip.ts so a
// bundle produced from the CLI and one produced from the sidepanel are
// interchangeable:
//
//	recording.json
//	README.md
//	screenshots/step_<n>.png, screenshots/annotation_<n>.png
//	audio/step_<n>.webm,      audio/annotation_<n>.webm
//
// recording.json carries *paths*, never base64, mirroring the extension's
// ExportedRecording — the bytes live in the sibling folders.

// ExportedStep mirrors ExportedStep in src/types/recording.ts.
type ExportedStep struct {
	ID             string                    `json:"id"`
	Timestamp      int64                     `json:"timestamp"`
	Type           string                    `json:"type"`
	URL            string                    `json:"url"`
	TabTitle       string                    `json:"tabTitle"`
	ScreenshotPath string                    `json:"screenshotPath,omitempty"`
	AudioPath      string                    `json:"audioPath,omitempty"`
	Element        *api.RecordingElementInfo `json:"element,omitempty"`
	InputValue     string                    `json:"inputValue,omitempty"`
	IsSensitive    bool                      `json:"isSensitive,omitempty"`
}

// ExportedAnnotation mirrors ExportedAnnotation in src/types/recording.ts.
type ExportedAnnotation struct {
	ID             string `json:"id"`
	Timestamp      int64  `json:"timestamp"`
	Text           string `json:"text"`
	ScreenshotPath string `json:"screenshotPath,omitempty"`
	AudioPath      string `json:"audioPath,omitempty"`
}

// ExportedRecording mirrors ExportedRecording in src/types/recording.ts.
type ExportedRecording struct {
	ID          string               `json:"id"`
	Title       string               `json:"title"`
	StartTime   int64                `json:"startTime"`
	EndTime     *int64               `json:"endTime,omitempty"`
	HasAudio    bool                 `json:"hasAudio"`
	Steps       []ExportedStep       `json:"steps"`
	Annotations []ExportedAnnotation `json:"annotations"`
}

// ─── Shared contract ────────────────────────────────────────────────
//
// The bundle layout is specified in shared/recording-export-contract.json at
// the repo root, and implemented twice: here and in src/utils/exportZip.ts.
// Neither implementation reads the file at runtime — two tests bind each of
// them to it (recordings_export_test.go and src/utils/exportZip.contract.test.ts),
// so changing either producer fails its test until the contract is updated.

// exportContract mirrors the shared JSON contract.
type exportContract struct {
	ManifestFile string            `json:"manifestFile"`
	ReadmeFile   string            `json:"readmeFile"`
	BundleFiles  []string          `json:"bundleFiles"`
	Directories  []string          `json:"directories"`
	Compression  string            `json:"compression"`
	Patterns     map[string]string `json:"patterns"`
	Paths        map[string]string `json:"paths"`
}

// loadExportContract reads the shared contract by walking up from the package
// directory to the repo root.
//
// `go test` always runs with the package source directory as CWD, and the walk
// additionally survives the contract being moved within the repo — a hardcoded
// `../../../shared/...` would not.
func loadExportContract() (*exportContract, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("resolving working directory: %w", err)
	}
	for i := 0; i < 12; i++ {
		candidate := filepath.Join(dir, "shared", "recording-export-contract.json")
		if raw, err := os.ReadFile(candidate); err == nil {
			var c exportContract
			if err := json.Unmarshal(raw, &c); err != nil {
				return nil, fmt.Errorf("parsing %s: %w", candidate, err)
			}
			return &c, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return nil, fmt.Errorf(
		"shared/recording-export-contract.json not found walking up from the package directory")
}

// contractPath formats one of the contract's full media paths for index n.
func (c *exportContract) contractPath(key string, n int) (string, error) {
	pattern, ok := c.Paths[key]
	if !ok {
		return "", fmt.Errorf("contract has no path for %q", key)
	}
	if strings.Count(pattern, "%d") != 1 {
		return "", fmt.Errorf("contract path %q must contain exactly one %%d", pattern)
	}
	return fmt.Sprintf(pattern, n), nil
}

// recordingExportFormat is the resolved output format.
type recordingExportFormat int

const (
	exportJSON recordingExportFormat = iota
	exportZip
	exportMarkdown
)

// recordingExportFormatFor maps an output path to a format. An unrecognised
// extension falls back to JSON, which is what this command produced before the
// zip/markdown formats existed.
func recordingExportFormatFor(path string) recordingExportFormat {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".zip":
		return exportZip
	case ".md", ".markdown":
		return exportMarkdown
	default:
		return exportJSON
	}
}

// decodeDataURL turns a base64 data URL (or bare base64) into raw bytes.
// The extension stores screenshots as `data:image/png;base64,…`, so the
// comma-prefixed form is the common case; a bare base64 string is accepted
// too so hand-built payloads still work.
func decodeDataURL(s string) ([]byte, error) {
	if i := strings.Index(s, ","); i >= 0 {
		s = s[i+1:]
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty base64 payload")
	}
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		// Data URLs can be URL-safe encoded even though the standard alphabet
		// is the common case; fall back rather than failing the whole export.
		raw, err = base64.RawStdEncoding.DecodeString(s)
		if err != nil {
			return nil, fmt.Errorf("decoding base64: %w", err)
		}
	}
	return raw, nil
}

// buildExportedRecording projects a session onto the path-based export shape.
func buildExportedRecording(session *api.RecordingSession) *ExportedRecording {
	out := &ExportedRecording{
		ID:          session.ID,
		Title:       session.Title,
		StartTime:   session.StartTime,
		EndTime:     session.EndTime,
		HasAudio:    session.HasAudio,
		Steps:       make([]ExportedStep, 0, len(session.Steps)),
		Annotations: make([]ExportedAnnotation, 0, len(session.Annotations)),
	}

	for i, step := range session.Steps {
		n := i + 1
		es := ExportedStep{
			ID:          step.ID,
			Timestamp:   step.Timestamp,
			Type:        step.Type,
			URL:         step.URL,
			TabTitle:    step.TabTitle,
			Element:     step.Element,
			InputValue:  step.InputValue,
			IsSensitive: step.IsSensitive,
		}
		if step.ScreenshotData != nil && *step.ScreenshotData != "" {
			es.ScreenshotPath = fmt.Sprintf("screenshots/step_%d.png", n)
		}
		if step.AudioData != nil && *step.AudioData != "" {
			es.AudioPath = fmt.Sprintf("audio/step_%d.webm", n)
		}
		out.Steps = append(out.Steps, es)
	}

	for i, ann := range session.Annotations {
		n := i + 1
		ea := ExportedAnnotation{ID: ann.ID, Timestamp: ann.Timestamp, Text: ann.Text}
		if ann.ScreenshotData != nil && *ann.ScreenshotData != "" {
			ea.ScreenshotPath = fmt.Sprintf("screenshots/annotation_%d.png", n)
		}
		if ann.AudioData != nil && *ann.AudioData != "" {
			ea.AudioPath = fmt.Sprintf("audio/annotation_%d.webm", n)
		}
		out.Annotations = append(out.Annotations, ea)
	}

	return out
}

// formatRecordingDate renders a millisecond epoch as a readable local date.
func formatRecordingDate(ms int64) string {
	return time.UnixMilli(ms).Format("January 2, 2006 at 3:04 PM")
}

// formatRecordingClock renders a step offset as mm:ss.mmm from recording start.
func formatRecordingClock(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	total := ms / 1000
	return fmt.Sprintf("%02d:%02d.%03d", total/60, total%60, ms%1000)
}

// elementPhrase renders an element as a trailing human phrase, or "" when the
// step has no element. The caller supplies the connective ("Click" + " on X",
// but "Input into" + " X") because the two need different wording.
func elementPhrase(el *api.RecordingElementInfo, connective string) string {
	if el == nil {
		return ""
	}
	if el.Text != "" {
		return fmt.Sprintf("%s%q", connective, truncate(el.Text, 50))
	}
	return fmt.Sprintf("%s<%s>", connective, el.Tag)
}

// stepDescription is a one-line human summary of what a step did.
func stepDescription(step api.RecordingStep) string {
	switch step.Type {
	case "click":
		// "Click on "Pay now"" / "Click on <button>" / "Click"
		return "Click" + elementPhrase(step.Element, " on ")
	case "input":
		// "Input into <input>" — deliberately NOT " on ", which read as
		// "Input into on <input>".
		into := elementPhrase(step.Element, " ")
		if step.IsSensitive {
			return fmt.Sprintf("Input into%s (sensitive)", into)
		}
		if step.InputValue != "" {
			return fmt.Sprintf("Input %q into%s", truncate(step.InputValue, 40), into)
		}
		return "Input into" + into
	case "navigation":
		if step.TabTitle != "" {
			return fmt.Sprintf("Navigate to %q", step.TabTitle)
		}
		return "Navigate"
	default:
		return step.Type
	}
}

// markdownMediaRef is the markdown link for a media file, or "" when absent.
func markdownMediaRef(rel string) string {
	if rel == "" {
		return ""
	}
	return fmt.Sprintf("[%s](%s)\n", filepath.Base(rel), rel)
}

// generateRecordingMarkdown renders the session as a readable timeline with
// steps and annotations merged in timestamp order.
func generateRecordingMarkdown(session *api.RecordingSession) string {
	var b strings.Builder

	b.WriteString("# " + session.Title + "\n\n")
	b.WriteString("_Recorded on " + formatRecordingDate(session.StartTime) + "_\n\n")
	b.WriteString("---\n\n")

	// Merge into one ordered timeline; steps keep their own numbering.
	steps := make([]*api.RecordingStep, len(session.Steps))
	for i := range session.Steps {
		steps[i] = &session.Steps[i]
	}
	anns := make([]*api.RecordingAnnotation, len(session.Annotations))
	for i := range session.Annotations {
		anns[i] = &session.Annotations[i]
	}

	stepNo := map[*api.RecordingStep]int{}
	for i, s := range steps {
		stepNo[s] = i + 1
	}
	annNo := map[*api.RecordingAnnotation]int{}
	for i, a := range anns {
		annNo[a] = i + 1
	}

	// Simple stable merge: repeatedly take the earliest remaining item.
	// The loop is driven by COUNTERS, not slice lengths — `len(usedStep)` is
	// the number of steps, so testing it would never terminate correctly.
	usedStep := make([]bool, len(steps))
	usedAnn := make([]bool, len(anns))
	stepsLeft, annsLeft := len(steps), len(anns)
	for stepsLeft > 0 || annsLeft > 0 {
		var pickStep *api.RecordingStep
		var pickAnn *api.RecordingAnnotation
		for i, s := range steps {
			if usedStep[i] {
				continue
			}
			if pickStep == nil || s.Timestamp < pickStep.Timestamp {
				pickStep = s
			}
		}
		for i, a := range anns {
			if usedAnn[i] {
				continue
			}
			if pickAnn == nil || a.Timestamp < pickAnn.Timestamp {
				pickAnn = a
			}
		}
		// Ties go to the step, matching the extension's stable sort.
		if pickStep != nil && (pickAnn == nil || pickStep.Timestamp <= pickAnn.Timestamp) {
			usedStep[stepNo[pickStep]-1] = true
			stepsLeft--
			writeStepMarkdown(&b, pickStep, stepNo[pickStep])
			continue
		}
		if pickAnn != nil {
			usedAnn[annNo[pickAnn]-1] = true
			annsLeft--
			writeAnnotationMarkdown(&b, pickAnn, annNo[pickAnn])
		}
	}

	return b.String()
}

func writeStepMarkdown(b *strings.Builder, step *api.RecordingStep, n int) {
	fmt.Fprintf(b, "## Step %d: %s (%s)\n\n", n, stepDescription(*step), formatRecordingClock(step.Timestamp))
	fmt.Fprintf(b, "**URL:** %s\n\n", step.URL)

	if step.Element != nil && step.Type != "navigation" {
		elem := fmt.Sprintf("`<%s>`", step.Element.Tag)
		if step.Element.Text != "" {
			elem += fmt.Sprintf(" - %q", truncate(step.Element.Text, 50))
		}
		fmt.Fprintf(b, "**Element:** %s\n\n", elem)

		b.WriteString("<details>\n<summary>Technical Details</summary>\n\n")
		fmt.Fprintf(b, "**Selector:** `%s`\n", step.Element.Selector)
		if step.Element.ID != "" {
			fmt.Fprintf(b, "**ID:** `%s`\n", step.Element.ID)
		}
		if step.Element.ClassName != "" {
			fmt.Fprintf(b, "**Classes:** `%s`\n", step.Element.ClassName)
		}
		if step.Element.Name != "" {
			fmt.Fprintf(b, "**Name:** `%s`\n", step.Element.Name)
		}
		b.WriteString("\n</details>\n\n")
	}

	if step.Type == "input" && step.InputValue != "" {
		if step.IsSensitive {
			// Raw literal: the backslash-asterisk pairs are Markdown escapes for
			// the literal asterisks, not Go escapes (which would not compile).
			b.WriteString(`**Value:** \*\*\*\*\*\*\*\* (sensitive)` + "\n\n")
		} else {
			fmt.Fprintf(b, "**Value:** %q\n\n", step.InputValue)
		}
	}

	if step.ScreenshotData != nil && *step.ScreenshotData != "" {
		fmt.Fprintf(b, "**Screenshot:** [step_%d.png](screenshots/step_%d.png)\n\n", n, n)
	}
	if step.AudioData != nil && *step.AudioData != "" {
		fmt.Fprintf(b, "**Audio:** [step_%d.webm](audio/step_%d.webm)\n\n", n, n)
	}
}

func writeAnnotationMarkdown(b *strings.Builder, ann *api.RecordingAnnotation, n int) {
	fmt.Fprintf(b, "## Annotation %d (%s)\n\n", n, formatRecordingClock(ann.Timestamp))
	fmt.Fprintf(b, "%s\n\n", ann.Text)
	if ann.ScreenshotData != nil && *ann.ScreenshotData != "" {
		fmt.Fprintf(b, "**Screenshot:** [annotation_%d.png](screenshots/annotation_%d.png)\n\n", n, n)
	}
	if ann.AudioData != nil && *ann.AudioData != "" {
		fmt.Fprintf(b, "**Audio:** [annotation_%d.webm](audio/annotation_%d.webm)\n\n", n, n)
	}
}

// writeRecordingZip writes the extension-compatible bundle to path.
func writeRecordingZip(path string, session *api.RecordingSession) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolving output path: %w", err)
	}
	if dir := filepath.Dir(abs); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("creating output directory: %w", err)
		}
	}

	f, err := os.Create(abs)
	if err != nil {
		return fmt.Errorf("writing %s: %w", abs, err)
	}
	zw := zip.NewWriter(f)

	// Deflate every entry, matching JSZip's `compression: "DEFLATE"` in
	// exportZip.ts. `zip.Writer.Create` would default to Store (no
	// compression) — the archive would still unzip everywhere, so only an
	// explicit assertion in the contract test catches the divergence.
	createEntry := func(name string) (io.Writer, error) {
		return zw.CreateHeader(&zip.FileHeader{
			Name:   name,
			Method: zip.Deflate,
		})
	}

	// A media blob that fails to decode is reported and skipped rather than
	// aborting: one corrupt screenshot should not cost the user the whole
	// recording. recording.json still lists its path, and the README names it
	// as a missing file so the gap is visible instead of silent.
	var missing []string
	addMedia := func(rel string, data *string) {
		if data == nil || *data == "" {
			return
		}
		raw, err := decodeDataURL(*data)
		if err != nil {
			missing = append(missing, rel)
			return
		}
		w, err := createEntry(rel)
		if err != nil {
			missing = append(missing, rel)
			return
		}
		if _, err := w.Write(raw); err != nil {
			missing = append(missing, rel)
		}
	}

	for i, step := range session.Steps {
		addMedia(fmt.Sprintf("screenshots/step_%d.png", i+1), step.ScreenshotData)
		addMedia(fmt.Sprintf("audio/step_%d.webm", i+1), step.AudioData)
	}
	for i, ann := range session.Annotations {
		addMedia(fmt.Sprintf("screenshots/annotation_%d.png", i+1), ann.ScreenshotData)
		addMedia(fmt.Sprintf("audio/annotation_%d.webm", i+1), ann.AudioData)
	}

	exported := buildExportedRecording(session)
	exportedJSON, err := json.MarshalIndent(exported, "", "  ")
	if err != nil {
		zw.Close()
		f.Close()
		return fmt.Errorf("encoding recording.json: %w", err)
	}
	if w, err := createEntry("recording.json"); err != nil {
		zw.Close()
		f.Close()
		return fmt.Errorf("adding recording.json: %w", err)
	} else if _, err := w.Write(append(exportedJSON, '\n')); err != nil {
		zw.Close()
		f.Close()
		return fmt.Errorf("writing recording.json: %w", err)
	}

	readme := generateRecordingMarkdown(session)
	if len(missing) > 0 {
		readme += "\n## Missing media\n\n" +
			"These files could not be decoded and were omitted from the bundle:\n\n"
		for _, m := range missing {
			readme += "- " + m + "\n"
		}
	}
	if w, err := createEntry("README.md"); err != nil {
		zw.Close()
		f.Close()
		return fmt.Errorf("adding README.md: %w", err)
	} else if _, err := w.Write([]byte(readme)); err != nil {
		zw.Close()
		f.Close()
		return fmt.Errorf("writing README.md: %w", err)
	}

	if err := zw.Close(); err != nil {
		f.Close()
		return fmt.Errorf("finalising %s: %w", abs, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", abs, err)
	}
	return nil
}
