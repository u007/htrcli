# Part 2 — Video Recording Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `htrcli record start` / `htrcli record stop <output.mp4>` — Chrome-only video capture via CDP `Page.startScreencast`, stitched to MP4 with an external ffmpeg dependency. Firefox/extension transport fails with an explicit "not supported" error.

**Architecture:** `record start` preflight-checks ffmpeg (fail fast), then spawns a **detached recorder process** (`htrcli record _run`, hidden) that holds a CDP page session, runs `Page.startScreencast`, and writes each JPEG frame to a temp frames dir plus a per-frame `frames.jsonl` manifest — managed exactly like `StartBrowser`/`StopBrowser` manage the Chrome PID (state file at `~/.htrcli/recording.json` + SIGTERM). `record stop` signals the recorder to finish, reads the manifest, builds an ffmpeg concat-demuxer list (per-frame durations from capture timestamps), and encodes the MP4. Capturing over the CDP socket means Go already holds the frames in-process — no native-messaging relay hop, so no screenshot-style HTTP POST-back is needed on this path (the extension-transport variant is deferred, see below).

**Tech Stack:** Go (cobra, stdlib `os/exec`, `os/signal`, `syscall`, `encoding/base64`, `sync/atomic`), the existing `internal/cdp` session/screencast CDP client, external `ffmpeg` (≥ 6). Go `testing`.

## Global Constraints

- Go module root: `htrcli/`. Run Go tests with `cd htrcli && go test ./...`.
- ffmpeg is an external prerequisite. It is a system binary, not a pinnable
  package — we detect it (fail fast) at BOTH `record start` and `record stop`,
  log its reported version, and document a minimum major (≥ 6) in the README.
  A missing ffmpeg ALWAYS produces an explicit error, never a hang.
- Video is **Chrome/CDP-only**. Under the extension/Firefox transport,
  `record start` returns an explicit "not supported on Firefox / the extension
  transport" error — never a silent no-op or partial attempt.
- Detached child processes use `SysProcAttr{Setsid: true}` (matches `StartBrowser`).
- State files under `~/.htrcli/`, files `0600`, dirs `0700`,
  `json.MarshalIndent(..., "", "  ")`.
- Every caught error is logged with attempt + error, or carries an explicit
  `// intentionally not logged: <reason>` comment.

## Deferred / simplified for this first version (stated honestly)

- **Extension-transport recording is NOT built.** The spec sketches frames
  flowing through the screenshot HTTP POST-back path (extension attaches
  `chrome.debugger`, POSTs to a new `/api/record/frames`). That requires a new
  `debugger` manifest permission and a background screencast module. This plan
  implements the spec's named `internal/cdp/screencast.go` (Go/CDP) path instead,
  which needs no extension change. The extension path is a future addition.
- **Frame-drop / precise timing is best-effort.** Durations come from
  `Page.screencastFrame` metadata timestamps via a concat list; dropped or
  coalesced frames are not reconstructed. Documented, not silently smoothed.
- **Idle-page capture ends via socket close, not frame polling.** The recorder
  blocks on the frame stream and is unblocked by closing the CDP socket on
  SIGTERM (see Task 3), avoiding gorilla read-deadline corruption. On a page
  that emits no frames at all, capture yields an empty manifest and `record stop`
  errors explicitly ("no frames captured").
- **No audio.** Screencast is video frames only (Playwright's `video` is also
  silent). Not a regression — documented.

---

### Task 1: ffmpeg detection + encode wrapper (`internal/media`)

**Files:**
- Create: `htrcli/internal/media/ffmpeg.go`
- Test: `htrcli/internal/media/ffmpeg_test.go`

**Interfaces:**
- Produces: `media.FindFFmpeg() (string, error)`, `media.FFmpegVersion(path string) (string, error)`, `media.EncodeArgs(concatListPath, output string) []string`, `media.EncodeFrames(ffmpegPath, workDir, concatListPath, output string) error`.

- [ ] **Step 1: Write the failing test**

Create `htrcli/internal/media/ffmpeg_test.go`:

```go
package media

import (
	"errors"
	"strings"
	"testing"
)

func TestFindFFmpegMissing(t *testing.T) {
	orig := lookPath
	t.Cleanup(func() { lookPath = orig })
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	if _, err := FindFFmpeg(); err == nil {
		t.Fatal("expected error when ffmpeg is missing")
	}
}

func TestFindFFmpegPresent(t *testing.T) {
	orig := lookPath
	t.Cleanup(func() { lookPath = orig })
	lookPath = func(name string) (string, error) { return "/usr/local/bin/" + name, nil }
	path, err := FindFFmpeg()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "/usr/local/bin/ffmpeg" {
		t.Fatalf("expected /usr/local/bin/ffmpeg, got %q", path)
	}
}

func TestEncodeArgs(t *testing.T) {
	args := EncodeArgs("/tmp/frames.txt", "/tmp/out.mp4")
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"-f concat", "-safe 0", "-i /tmp/frames.txt",
		"-vsync vfr", "-pix_fmt yuv420p", "/tmp/out.mp4",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected args to contain %q, got %q", want, joined)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd htrcli && go test ./internal/media/ -v`
Expected: FAIL — package `media` does not exist / `lookPath`, `FindFFmpeg`, `EncodeArgs` undefined.

- [ ] **Step 3: Write the implementation**

Create `htrcli/internal/media/ffmpeg.go`:

```go
// Package media wraps the external ffmpeg binary used to stitch screencast
// frames into an MP4. ffmpeg is a prerequisite, not vendored — every entry
// point surfaces its absence as an explicit error.
package media

import (
	"fmt"
	"os/exec"
	"strings"
)

// lookPath is a package var so tests can stub PATH resolution.
var lookPath = exec.LookPath

// FindFFmpeg returns the ffmpeg binary path or an explicit error. Callers MUST
// surface this error rather than proceeding — a missing binary must never turn
// into a hang at encode time.
func FindFFmpeg() (string, error) {
	path, err := lookPath("ffmpeg")
	if err != nil {
		return "", fmt.Errorf("ffmpeg not found on PATH — install ffmpeg (>= 6) and retry: %w", err)
	}
	return path, nil
}

// FFmpegVersion returns the first line of `ffmpeg -version`, for logging so the
// build/version in use is captured in record output.
func FFmpegVersion(ffmpegPath string) (string, error) {
	out, err := exec.Command(ffmpegPath, "-version").Output()
	if err != nil {
		return "", fmt.Errorf("running %s -version: %w", ffmpegPath, err)
	}
	line := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]
	return line, nil
}

// EncodeArgs builds the ffmpeg concat-demuxer args. VFR preserves the per-frame
// timing encoded in the concat list; yuv420p keeps the MP4 broadly playable.
func EncodeArgs(concatListPath, output string) []string {
	return []string{
		"-y",
		"-f", "concat",
		"-safe", "0",
		"-i", concatListPath,
		"-vsync", "vfr",
		"-pix_fmt", "yuv420p",
		output,
	}
}

// EncodeFrames stitches the frames referenced by concatListPath into output.
// workDir is the ffmpeg working directory so relative frame basenames in the
// concat list resolve correctly. ffmpeg's stderr is captured into the error.
func EncodeFrames(ffmpegPath, workDir, concatListPath, output string) error {
	cmd := exec.Command(ffmpegPath, EncodeArgs(concatListPath, output)...)
	cmd.Dir = workDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg encode failed: %w\n%s", err, out)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd htrcli && go test ./internal/media/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add htrcli/internal/media/ffmpeg.go htrcli/internal/media/ffmpeg_test.go
git commit -m "feat(htrcli): media package wraps ffmpeg detection + encode"
```

---

### Task 2: Recording state, frame manifest, concat-list builder

**Files:**
- Create: `htrcli/internal/cdp/screencast.go`
- Test: `htrcli/internal/cdp/screencast_test.go`

**Interfaces:**
- Produces: `cdp.RecordingState{PID int, FramesDir string, Port int, StartedAt time.Time}`, `cdp.RecordingStatePath() (string, error)`, `cdp.ReadRecording() (*RecordingState, error)`, `cdp.WriteRecording(*RecordingState) error`, `cdp.RemoveRecording() error`, `cdp.FrameMeta{File string, Timestamp float64}`, `cdp.ReadFrameManifest(framesDir string) ([]FrameMeta, error)`, `cdp.BuildConcatList(frames []FrameMeta, defaultDur float64) string`, and the exported const `cdp.FrameManifestName`.

- [ ] **Step 1: Write the failing test**

Create `htrcli/internal/cdp/screencast_test.go`:

```go
package cdp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecordingStateRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if st, err := ReadRecording(); err != nil || st != nil {
		t.Fatalf("expected no recording, got %+v err %v", st, err)
	}

	want := &RecordingState{PID: 4242, FramesDir: "/tmp/frames", Port: 9333}
	if err := WriteRecording(want); err != nil {
		t.Fatalf("WriteRecording: %v", err)
	}
	got, err := ReadRecording()
	if err != nil || got == nil {
		t.Fatalf("ReadRecording: %+v err %v", got, err)
	}
	if got.PID != 4242 || got.FramesDir != "/tmp/frames" || got.Port != 9333 {
		t.Fatalf("round-trip mismatch: %+v", got)
	}

	if err := RemoveRecording(); err != nil {
		t.Fatalf("RemoveRecording: %v", err)
	}
	if st, err := ReadRecording(); err != nil || st != nil {
		t.Fatalf("expected removed, got %+v err %v", st, err)
	}
	// Removing a second time is a no-op, not an error.
	if err := RemoveRecording(); err != nil {
		t.Fatalf("second RemoveRecording should be nil, got %v", err)
	}
}

func TestReadFrameManifest(t *testing.T) {
	dir := t.TempDir()
	manifest := `{"file":"frame-000001.jpg","timestamp":100.0}
{"file":"frame-000002.jpg","timestamp":100.04}

{"file":"frame-000003.jpg","timestamp":100.10}
`
	if err := os.WriteFile(filepath.Join(dir, FrameManifestName), []byte(manifest), 0644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	frames, err := ReadFrameManifest(dir)
	if err != nil {
		t.Fatalf("ReadFrameManifest: %v", err)
	}
	if len(frames) != 3 {
		t.Fatalf("expected 3 frames (blank line skipped), got %d", len(frames))
	}
	if frames[0].File != "frame-000001.jpg" || frames[2].Timestamp != 100.10 {
		t.Fatalf("unexpected frames: %+v", frames)
	}
}

func TestBuildConcatList(t *testing.T) {
	frames := []FrameMeta{
		{File: "frame-000001.jpg", Timestamp: 100.00},
		{File: "frame-000002.jpg", Timestamp: 100.04},
		{File: "frame-000003.jpg", Timestamp: 100.10},
	}
	list := BuildConcatList(frames, 0.033)
	if !strings.HasPrefix(list, "ffconcat version 1.0\n") {
		t.Fatalf("missing header: %q", list)
	}
	// Gap 1->2 is 0.04, gap 2->3 is 0.06.
	if !strings.Contains(list, "duration 0.040000") || !strings.Contains(list, "duration 0.060000") {
		t.Fatalf("expected computed durations, got:\n%s", list)
	}
	// Final frame is repeated once so ffmpeg honors its duration.
	if strings.Count(list, "frame-000003.jpg") != 2 {
		t.Fatalf("expected final frame repeated once, got:\n%s", list)
	}
}

func TestBuildConcatListEmpty(t *testing.T) {
	if got := BuildConcatList(nil, 0.033); got != "ffconcat version 1.0\n" {
		t.Fatalf("expected header-only for empty frames, got %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd htrcli && go test ./internal/cdp/ -run 'TestRecordingStateRoundTrip|TestReadFrameManifest|TestBuildConcatList' -v`
Expected: FAIL — `RecordingState`, `ReadRecording`, `FrameManifestName`, `ReadFrameManifest`, `BuildConcatList` undefined.

- [ ] **Step 3: Write the state/manifest/concat implementation**

Create `htrcli/internal/cdp/screencast.go`:

```go
package cdp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RecordingState is advisory state for an in-progress recording, at
// ~/.htrcli/recording.json. As with browser.json, the recorder process
// answering (its PID being alive) is the real source of truth.
type RecordingState struct {
	PID       int       `json:"pid"`
	FramesDir string    `json:"frames_dir"`
	Port      int       `json:"port"`
	StartedAt time.Time `json:"started_at"`
}

// FrameMeta is one captured frame: its file basename + capture timestamp
// (seconds, from Page.screencastFrame metadata).
type FrameMeta struct {
	File      string  `json:"file"`
	Timestamp float64 `json:"timestamp"`
}

// FrameManifestName is the per-frame JSONL manifest inside a frames dir.
const FrameManifestName = "frames.jsonl"

// RecordingStatePath returns ~/.htrcli/recording.json.
func RecordingStatePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	return filepath.Join(home, ".htrcli", "recording.json"), nil
}

// ReadRecording returns nil, nil when no recording is in progress.
func ReadRecording() (*RecordingState, error) {
	path, err := RecordingStatePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil // intentionally not logged: absent file means "not recording", expected
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var st RecordingState
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &st, nil
}

// WriteRecording persists the recording state.
func WriteRecording(st *RecordingState) error {
	path, err := RecordingStatePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling recording state: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// RemoveRecording deletes the recording state file (absent = no-op).
func RemoveRecording() error {
	path, err := RecordingStatePath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing %s: %w", path, err)
	}
	return nil
}

// ReadFrameManifest loads the per-frame metadata written during capture,
// skipping blank lines.
func ReadFrameManifest(framesDir string) ([]FrameMeta, error) {
	f, err := os.Open(filepath.Join(framesDir, FrameManifestName))
	if err != nil {
		return nil, fmt.Errorf("opening frame manifest: %w", err)
	}
	defer f.Close()

	var frames []FrameMeta
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var fm FrameMeta
		if err := json.Unmarshal([]byte(line), &fm); err != nil {
			return nil, fmt.Errorf("parsing frame manifest line %q: %w", line, err)
		}
		frames = append(frames, fm)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("reading frame manifest: %w", err)
	}
	return frames, nil
}

// BuildConcatList renders an ffmpeg concat-demuxer list. Per-frame durations
// come from consecutive capture timestamps; a non-positive gap falls back to
// defaultDur (seconds). The final frame is repeated once so the concat demuxer
// honors its duration (a documented ffmpeg quirk).
func BuildConcatList(frames []FrameMeta, defaultDur float64) string {
	var b strings.Builder
	b.WriteString("ffconcat version 1.0\n")
	for i, fm := range frames {
		dur := defaultDur
		if i+1 < len(frames) {
			if gap := frames[i+1].Timestamp - fm.Timestamp; gap > 0 {
				dur = gap
			}
		}
		fmt.Fprintf(&b, "file '%s'\n", fm.File)
		fmt.Fprintf(&b, "duration %.6f\n", dur)
	}
	if len(frames) > 0 {
		fmt.Fprintf(&b, "file '%s'\n", frames[len(frames)-1].File)
	}
	return b.String()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd htrcli && go test ./internal/cdp/ -run 'TestRecordingStateRoundTrip|TestReadFrameManifest|TestBuildConcatList' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add htrcli/internal/cdp/screencast.go htrcli/internal/cdp/screencast_test.go
git commit -m "feat(htrcli): recording state, frame manifest, ffmpeg concat builder"
```

---

### Task 3: CDP screencast capture loop (`RunRecorder`)

**Files:**
- Modify: `htrcli/internal/cdp/screencast.go`

**Interfaces:**
- Consumes: `PageSession`, `Session.Call`, `Session.WaitEvent`, `Session.Close` (existing `internal/cdp`); `FrameMeta`, `FrameManifestName` (Task 2).
- Produces: `cdp.RunRecorder(port int, targetID, framesDir string, stop <-chan os.Signal) error`.

**Note on stop mechanism:** `Session.WaitEvent` uses a socket read deadline, and a
gorilla read timeout can corrupt the connection. To stop cleanly we instead block
on the frame stream with a long deadline and unblock it by **closing the CDP
socket** from a goroutine when `stop` fires — gorilla explicitly permits `Close`
concurrently with a blocked read. A recorder-initiated close is distinguished
from a real read error by the `stopped` flag.

**No unit test:** this function drives a live Chrome screencast; it cannot be
exercised without a real browser. It is covered by the Task 5 manual end-to-end
smoke test. The pure logic it relies on (manifest, concat list) is unit-tested in
Task 2.

- [ ] **Step 1: Add RunRecorder**

Append to `htrcli/internal/cdp/screencast.go`. Add `"encoding/base64"`,
`"os/signal"`, and `"sync/atomic"` to the file's import block (alongside the
existing `bufio`, `encoding/json`, `errors`, `fmt`, `os`, `path/filepath`,
`strings`, `time`):

```go
// RunRecorder opens a JPEG screencast on the target page and writes each frame
// to framesDir plus a line to the frame manifest, until a signal arrives on
// stop. Chrome-only (CDP). Stop is delivered by closing the CDP socket, which
// unblocks the frame read; the deferred stopScreencast is best-effort.
func RunRecorder(port int, targetID, framesDir string, stop <-chan os.Signal) error {
	if err := os.MkdirAll(framesDir, 0700); err != nil {
		return fmt.Errorf("creating frames dir: %w", err)
	}
	manifest, err := os.Create(filepath.Join(framesDir, FrameManifestName))
	if err != nil {
		return fmt.Errorf("creating frame manifest: %w", err)
	}
	defer manifest.Close()

	s, err := PageSession(port, targetID)
	if err != nil {
		return err
	}
	defer s.Close()

	if err := s.Call("Page.enable", nil, nil); err != nil {
		return fmt.Errorf("Page.enable: %w", err)
	}
	if err := s.Call("Page.startScreencast", map[string]any{
		"format":        "jpeg",
		"quality":       80,
		"everyNthFrame": 1,
	}, nil); err != nil {
		return fmt.Errorf("Page.startScreencast: %w", err)
	}
	defer func() {
		if err := s.Call("Page.stopScreencast", nil, nil); err != nil {
			// Expected to fail if the socket was already closed by the stop path.
			fmt.Fprintf(os.Stderr, "[htrcli] stopScreencast (recorder shutdown): %v\n", err)
		}
	}()

	var stopped atomic.Bool
	go func() {
		<-stop
		stopped.Store(true)
		// Unblocks the WaitEvent read below; gorilla permits concurrent Close.
		if err := s.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "[htrcli] closing recorder session on stop: %v\n", err)
		}
	}()

	seq := 0
	for {
		params, err := s.WaitEvent("Page.screencastFrame", 24*time.Hour)
		if err != nil {
			if stopped.Load() {
				return nil // clean stop: socket closed by the stop goroutine
			}
			return fmt.Errorf("waiting for screencast frame: %w", err)
		}
		var frame struct {
			Data      string `json:"data"`
			SessionID int    `json:"sessionId"`
			Metadata  struct {
				Timestamp float64 `json:"timestamp"`
			} `json:"metadata"`
		}
		if err := json.Unmarshal(params, &frame); err != nil {
			fmt.Fprintf(os.Stderr, "[htrcli] decode screencast frame: %v\n", err)
			continue
		}
		// Ack immediately so Chrome keeps sending frames.
		if err := s.Call("Page.screencastFrameAck", map[string]any{"sessionId": frame.SessionID}, nil); err != nil {
			if stopped.Load() {
				return nil
			}
			fmt.Fprintf(os.Stderr, "[htrcli] screencastFrameAck: %v\n", err)
		}
		img, err := base64.StdEncoding.DecodeString(frame.Data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[htrcli] decode frame base64: %v\n", err)
			continue
		}
		seq++
		name := fmt.Sprintf("frame-%06d.jpg", seq)
		if err := os.WriteFile(filepath.Join(framesDir, name), img, 0644); err != nil {
			return fmt.Errorf("writing frame %s: %w", name, err)
		}
		ts := frame.Metadata.Timestamp
		if ts == 0 {
			ts = float64(time.Now().UnixNano()) / 1e9
		}
		meta, err := json.Marshal(FrameMeta{File: name, Timestamp: ts})
		if err != nil {
			return fmt.Errorf("marshaling frame meta: %w", err)
		}
		if _, err := manifest.Write(append(meta, '\n')); err != nil {
			return fmt.Errorf("writing frame manifest: %w", err)
		}
	}
}
```

- [ ] **Step 2: Verify the package still builds and existing tests pass**

Run: `cd htrcli && go build ./... && go test ./internal/cdp/...`
Expected: clean build, PASS (Task 2 tests still green; no new unit test here).

- [ ] **Step 3: Commit**

```bash
git add htrcli/internal/cdp/screencast.go
git commit -m "feat(htrcli): CDP screencast capture loop with socket-close stop"
```

---

### Task 4: `record start` + hidden `record _run`

**Files:**
- Create: `htrcli/internal/commands/record.go`
- Test: `htrcli/internal/commands/record_test.go`

**Interfaces:**
- Consumes: `media.FindFFmpeg`, `media.FFmpegVersion` (Task 1); `cdp.RunRecorder`, `cdp.RecordingState`, `cdp.ReadRecording`, `cdp.WriteRecording` (Tasks 2–3); `UseCDP`, `GetCDPPort`, `GetTabTarget`, `output.PrintJSON`, `output.JSONOutput` (existing).
- Produces: the `record`, `record start`, and hidden `record _run` cobra commands; helpers `recordFramesDir() (string, error)`, `errRecordUnsupported() error`, `processAlive(pid int) bool`.

- [ ] **Step 1: Write the failing test**

Create `htrcli/internal/commands/record_test.go`:

```go
package commands

import (
	"testing"
)

func TestRecordStartRejectsExtTransport(t *testing.T) {
	// Default transport is ext; record is CDP-only.
	t.Cleanup(func() { transportFlag = ""; cdpFlag = false })
	transportFlag = "ext"
	err := recordStartCmd.RunE(recordStartCmd, nil)
	if err == nil {
		t.Fatal("expected record start to reject the extension transport")
	}
	if got := err.Error(); !contains(got, "not supported") {
		t.Fatalf("expected 'not supported' error, got %q", got)
	}
}

func TestErrRecordUnsupportedMentionsFirefox(t *testing.T) {
	msg := errRecordUnsupported().Error()
	if !contains(msg, "Firefox") || !contains(msg, "--cdp") {
		t.Fatalf("expected Firefox + --cdp guidance, got %q", msg)
	}
}

// contains is a tiny substring helper local to the test.
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd htrcli && go test ./internal/commands/ -run 'TestRecordStart|TestErrRecordUnsupported' -v`
Expected: FAIL — `recordStartCmd`, `errRecordUnsupported` undefined.

- [ ] **Step 3: Write the commands**

Create `htrcli/internal/commands/record.go`:

```go
package commands

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/u007/htrcli/internal/cdp"
	"github.com/u007/htrcli/internal/media"
	"github.com/u007/htrcli/internal/output"
)

var recordCmd = &cobra.Command{
	Use:   "record",
	Short: "Record the page to video (Chrome/CDP only)",
}

// errRecordUnsupported is the explicit non-CDP guard. Video capture needs CDP
// Page.startScreencast, which the extension/Firefox transport cannot provide.
func errRecordUnsupported() error {
	return fmt.Errorf("record is not supported on Firefox or the extension transport — it requires Chrome via --cdp")
}

// recordFramesDir returns a fresh per-recording temp frames dir under ~/.htrcli.
func recordFramesDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	dir := filepath.Join(home, ".htrcli", "recordings", fmt.Sprintf("rec-%d", time.Now().UnixMilli()), "frames")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("creating frames dir: %w", err)
	}
	return dir, nil
}

// processAlive reports whether pid is a live process (signal 0 probe).
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

var recordStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start recording the current page",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !UseCDP() {
			return errRecordUnsupported()
		}

		// Fail fast if ffmpeg is missing — never start a capture we can't encode.
		ffmpegPath, err := media.FindFFmpeg()
		if err != nil {
			return err
		}
		if ver, verr := media.FFmpegVersion(ffmpegPath); verr == nil {
			fmt.Fprintf(os.Stderr, "[htrcli] using %s\n", ver)
		}

		// Refuse to start a second concurrent recording.
		if st, err := cdp.ReadRecording(); err != nil {
			return err
		} else if st != nil && processAlive(st.PID) {
			return fmt.Errorf("a recording is already in progress (pid %d, frames %s) — stop it first", st.PID, st.FramesDir)
		}

		framesDir, err := recordFramesDir()
		if err != nil {
			return err
		}
		port := GetCDPPort()

		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("resolving htrcli path: %w", err)
		}
		runArgs := []string{"record", "_run", "--frames-dir", framesDir, "--rec-port", strconv.Itoa(port)}
		if target := GetTabTarget(); target != "" {
			runArgs = append(runArgs, "--rec-tab", target)
		}
		child := exec.Command(exe, runArgs...)
		child.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // detach: survives this CLI exiting
		if err := child.Start(); err != nil {
			return fmt.Errorf("spawning recorder: %w", err)
		}

		st := &cdp.RecordingState{PID: child.Process.Pid, FramesDir: framesDir, Port: port, StartedAt: time.Now()}
		if err := cdp.WriteRecording(st); err != nil {
			return err
		}

		if output.JSONOutput {
			output.PrintJSON(map[string]any{"recording": true, "pid": st.PID, "framesDir": framesDir})
			return nil
		}
		fmt.Printf("Recording started (pid %d). Stop with: htrcli record stop <output.mp4>\n", st.PID)
		return nil
	},
}

var (
	recRunFramesDir string
	recRunPort      int
	recRunTab       string
)

// recordRunCmd is the detached worker spawned by `record start`. Hidden — not a
// user-facing verb.
var recordRunCmd = &cobra.Command{
	Use:    "_run",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if recRunFramesDir == "" || recRunPort <= 0 {
			return fmt.Errorf("_run requires --frames-dir and --rec-port")
		}
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
		return cdp.RunRecorder(recRunPort, recRunTab, recRunFramesDir, stop)
	},
}

func init() {
	recordRunCmd.Flags().StringVar(&recRunFramesDir, "frames-dir", "", "frames output directory (internal)")
	recordRunCmd.Flags().IntVar(&recRunPort, "rec-port", 0, "CDP debugging port (internal)")
	recordRunCmd.Flags().StringVar(&recRunTab, "rec-tab", "", "CDP target id (internal)")

	recordCmd.AddCommand(recordStartCmd)
	recordCmd.AddCommand(recordRunCmd)
	rootCmd.AddCommand(recordCmd)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd htrcli && go test ./internal/commands/ -run 'TestRecordStart|TestErrRecordUnsupported' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add htrcli/internal/commands/record.go htrcli/internal/commands/record_test.go
git commit -m "feat(htrcli): record start + hidden detached recorder worker"
```

---

### Task 5: `record stop <output.mp4>`

**Files:**
- Modify: `htrcli/internal/commands/record.go`
- Modify: `htrcli/internal/commands/record_test.go`

**Interfaces:**
- Consumes: `cdp.ReadRecording`, `cdp.RemoveRecording`, `cdp.ReadFrameManifest`, `cdp.BuildConcatList`, `cdp.FrameManifestName` (Tasks 2); `media.FindFFmpeg`, `media.EncodeFrames` (Task 1); `processAlive` (Task 4).
- Produces: `record stop` cobra command; helpers `stopRecorder(st *cdp.RecordingState) error`, `writeConcatList(framesDir string, frames []cdp.FrameMeta) (string, error)`.

- [ ] **Step 1: Write the failing test**

Append to `htrcli/internal/commands/record_test.go`:

```go
func TestRecordStopNoRecording(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	err := recordStopCmd.RunE(recordStopCmd, []string{"out.mp4"})
	if err == nil {
		t.Fatal("expected error when no recording is in progress")
	}
	if !contains(err.Error(), "no recording") {
		t.Fatalf("expected 'no recording' error, got %q", err.Error())
	}
}

func TestRecordStopRequiresOutputArg(t *testing.T) {
	if err := recordStopCmd.Args(recordStopCmd, nil); err == nil {
		t.Fatal("expected record stop to require exactly one output arg")
	}
	if err := recordStopCmd.Args(recordStopCmd, []string{"a", "b"}); err == nil {
		t.Fatal("expected record stop to reject two args")
	}
}

func TestWriteConcatList(t *testing.T) {
	dir := t.TempDir()
	frames := []cdp.FrameMeta{
		{File: "frame-000001.jpg", Timestamp: 1.0},
		{File: "frame-000002.jpg", Timestamp: 1.05},
	}
	path, err := writeConcatList(dir, frames)
	if err != nil {
		t.Fatalf("writeConcatList: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read concat list: %v", err)
	}
	if !contains(string(data), "ffconcat version 1.0") {
		t.Fatalf("expected ffconcat header, got %q", string(data))
	}
}
```

Add `"os"` and `"github.com/u007/htrcli/internal/cdp"` to the test file's imports
(the earlier test only needed the `commands` package itself):

```go
import (
	"os"
	"testing"

	"github.com/u007/htrcli/internal/cdp"
)
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd htrcli && go test ./internal/commands/ -run 'TestRecordStop|TestWriteConcatList' -v`
Expected: FAIL — `recordStopCmd`, `writeConcatList` undefined.

- [ ] **Step 3: Add record stop + helpers**

Append to `htrcli/internal/commands/record.go`. Add `"github.com/u007/htrcli/internal/cdp"` is already imported; no new imports needed beyond what Task 4 added:

```go
// stopRecorder signals the recorder to finish and waits for it to exit. It
// verifies the PID looks like our recorder (command line references the frames
// dir) before signalling, to avoid killing an unrelated reused PID. If the
// process does not exit within the grace period it is SIGKILLed.
func stopRecorder(st *cdp.RecordingState) error {
	if !processAlive(st.PID) {
		return nil // already gone — encode whatever frames exist
	}
	out, err := exec.Command("ps", "-p", strconv.Itoa(st.PID), "-o", "command=").Output()
	if err == nil && !containsStr(string(out), st.FramesDir) {
		fmt.Fprintf(os.Stderr, "[htrcli] pid %d does not look like the htrcli recorder (%s) — not signalling; encoding captured frames\n", st.PID, string(out))
		return nil
	}
	if err := syscall.Kill(st.PID, syscall.SIGTERM); err != nil {
		return fmt.Errorf("signalling recorder pid %d: %w", st.PID, err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if !processAlive(st.PID) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	fmt.Fprintf(os.Stderr, "[htrcli] recorder pid %d did not exit in 10s — sending SIGKILL\n", st.PID)
	if err := syscall.Kill(st.PID, syscall.SIGKILL); err != nil {
		return fmt.Errorf("force-killing recorder pid %d: %w", st.PID, err)
	}
	return nil
}

func containsStr(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && stringsIndex(s, sub) >= 0)
}

func stringsIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// writeConcatList renders the ffmpeg concat list for frames into
// <framesDir>/concat.txt and returns its path.
func writeConcatList(framesDir string, frames []cdp.FrameMeta) (string, error) {
	// 30fps fallback for non-positive gaps / a lone final frame.
	list := cdp.BuildConcatList(frames, 1.0/30.0)
	path := filepath.Join(framesDir, "concat.txt")
	if err := os.WriteFile(path, []byte(list), 0644); err != nil {
		return "", fmt.Errorf("writing concat list: %w", err)
	}
	return path, nil
}

var recordStopCmd = &cobra.Command{
	Use:   "stop <output.mp4>",
	Short: "Stop recording and encode to MP4",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Preflight ffmpeg again — the environment may differ from record start.
		ffmpegPath, err := media.FindFFmpeg()
		if err != nil {
			return err
		}

		st, err := cdp.ReadRecording()
		if err != nil {
			return err
		}
		if st == nil {
			return fmt.Errorf("no recording in progress — start one with: htrcli record start")
		}

		if err := stopRecorder(st); err != nil {
			return err
		}

		frames, err := cdp.ReadFrameManifest(st.FramesDir)
		if err != nil {
			return fmt.Errorf("reading captured frames: %w", err)
		}
		if len(frames) == 0 {
			return fmt.Errorf("no frames captured — the page may have emitted no screencast frames")
		}

		concatPath, err := writeConcatList(st.FramesDir, frames)
		if err != nil {
			return err
		}

		outPath, err := filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("resolving output path: %w", err)
		}
		if err := media.EncodeFrames(ffmpegPath, st.FramesDir, concatPath, outPath); err != nil {
			return err
		}

		// Success: clear state and reclaim the frames dir.
		if err := cdp.RemoveRecording(); err != nil {
			return err
		}
		if err := os.RemoveAll(st.FramesDir); err != nil {
			fmt.Fprintf(os.Stderr, "[htrcli] cleaning up frames dir %s: %v\n", st.FramesDir, err)
		}

		fmt.Printf("Recording saved to %s (%d frames)\n", outPath, len(frames))
		return nil
	},
}
```

Wire it into the `init()` in `record.go` — add one line alongside the existing
`recordCmd.AddCommand(...)` calls:

```go
	recordCmd.AddCommand(recordStopCmd)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd htrcli && go test ./internal/commands/ -run 'TestRecordStop|TestWriteConcatList|TestRecordStart|TestErrRecordUnsupported' -v`
Expected: PASS.

- [ ] **Step 5: Full suite + build**

Run: `cd htrcli && go test ./... && go build ./...`
Expected: PASS / clean build.

- [ ] **Step 6: Commit**

```bash
git add htrcli/internal/commands/record.go htrcli/internal/commands/record_test.go
git commit -m "feat(htrcli): record stop encodes screencast frames to MP4"
```

---

### Task 6: Document the ffmpeg prerequisite + manual end-to-end

**Files:**
- Modify: `README.md` (or the repo's primary docs entry — check for a `docs/` index first)
- Modify: `CLAUDE.md` (Commands section — note the new dependency)

- [ ] **Step 1: Document ffmpeg as a prerequisite**

Add an "External dependencies" note near the htrcli build commands in `CLAUDE.md`
and the README, stating: `record stop` requires **ffmpeg ≥ 6** on `PATH`
(`brew install ffmpeg`); its absence produces an explicit error at both
`record start` and `record stop`, never a hang. Note video recording is
**Chrome/CDP-only** (`--cdp`) and unsupported on Firefox.

- [ ] **Step 2: Manual end-to-end smoke test (requires Chrome + ffmpeg)**

Run:
```bash
make htrcli-build
brew install ffmpeg              # if not already present
./htrcli/bin/htrcli --cdp browser start
./htrcli/bin/htrcli --cdp open https://example.com
./htrcli/bin/htrcli --cdp record start
# interact with / scroll the page for a few seconds
./htrcli/bin/htrcli --cdp record stop /tmp/demo.mp4
```
Expected: `record start` prints a recorder PID; after a few seconds of activity,
`record stop` reports "Recording saved to /tmp/demo.mp4 (N frames)"; `/tmp/demo.mp4`
plays back the interaction. Confirm `~/.htrcli/recording.json` is gone and the
frames dir is cleaned up.

- [ ] **Step 3: ffmpeg-missing check (the no-hang requirement)**

Run: temporarily shadow ffmpeg (`PATH=/usr/bin htrcli --cdp record start` in an
env without ffmpeg, or rename it), then `htrcli --cdp record start`.
Expected: immediate explicit error "ffmpeg not found on PATH — install ffmpeg
(>= 6) and retry", exit non-zero, no hang, no recorder process spawned.

- [ ] **Step 4: Firefox / ext-transport check**

Run: `./htrcli/bin/htrcli record start` (default ext transport).
Expected: immediate error "record is not supported on Firefox or the extension
transport — it requires Chrome via --cdp".

- [ ] **Step 5: Commit**

```bash
git add README.md CLAUDE.md
git commit -m "docs(htrcli): document ffmpeg prerequisite for record"
```

---

## Part 2 Self-Review

- **Spec coverage (§7b):** `record start`/`record stop <output.mp4>` → Tasks 4–5;
  CDP `Page.startScreencast` streaming JPEG frames → Task 3; external ffmpeg
  stitching with explicit "ffmpeg not found" error at start AND stop (never a
  hang) → Tasks 1, 4, 5; Firefox "not supported" (not a fallback) → Task 4;
  README/docs prerequisite note + absence-produces-error verification → Task 6.
- **Placeholder scan:** every step ships complete code; no TBD/TODO; the two tiny
  substring helpers (test-side `contains`/`indexOf`, prod-side
  `containsStr`/`stringsIndex`) are fully defined where introduced. (They avoid an
  extra `strings` import churn; `strings.Contains` would be equivalent if the
  implementer prefers it.)
- **Type consistency:** `RecordingState` fields and `FrameMeta{File,Timestamp}`
  match across Tasks 2–5; `RunRecorder(port, targetID, framesDir, stop)`,
  `BuildConcatList(frames, defaultDur)`, `EncodeFrames(ffmpegPath, workDir,
  concatListPath, output)`, `writeConcatList(framesDir, frames)` signatures are
  used identically at every call site; `FrameManifestName` is the single source
  for the manifest filename.
- **Honest deferrals (restated):** extension-transport POST-back path not built;
  frame-drop/precise-timing best-effort; no audio; idle-page capture may yield an
  empty manifest → explicit "no frames captured" error. All surfaced above, not
  glossed.
- **Open judgment call:** `Page.startScreencast` `quality`/`everyNthFrame` are
  fixed (80 / 1). Exposing them as flags is deferred until there's a demonstrated
  need — YAGNI.
