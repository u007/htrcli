package cdp

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
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

// pageSession is overridable in tests so RunRecorder can be driven against a
// fake CDP endpoint without a real Chrome.
var pageSession = PageSession

// RunRecorder opens a JPEG screencast on the target page and writes each frame
// to framesDir plus a line to the frame manifest, until a signal arrives on
// stop. Chrome-only (CDP). On stop it cleans up on a fresh session (stop the
// screencast, release the status-bar hold), then closes the capture socket to
// unblock the frame read.
func RunRecorder(port int, targetID, framesDir string, stop <-chan os.Signal) error {
	if err := os.MkdirAll(framesDir, 0700); err != nil {
		return fmt.Errorf("creating frames dir: %w", err)
	}
	manifest, err := os.Create(filepath.Join(framesDir, FrameManifestName))
	if err != nil {
		return fmt.Errorf("creating frame manifest: %w", err)
	}
	defer manifest.Close()

	s, err := pageSession(port, targetID)
	if err != nil {
		return err
	}
	defer s.Close()

	if err := s.Call("Page.enable", nil, nil); err != nil {
		return fmt.Errorf("Page.enable: %w", err)
	}
	var stopped atomic.Bool
	var cleanupDone atomic.Bool
	// Keep the extension's status line out of the recorded frames.
	hideControlStatusBar(s, holdRecord)
	// The stop goroutine restores on its own session; these cover error exits.
	defer func() {
		if cleanupDone.Load() {
			return
		}
		if err := s.Call("Page.stopScreencast", nil, nil); err != nil {
			fmt.Fprintf(os.Stderr, "[htrcli] stopScreencast: %v\n", err)
		}
		restoreControlStatusBar(s, holdRecord)
	}()
	if err := s.Call("Page.startScreencast", map[string]any{
		"format":        "jpeg",
		"quality":       80,
		"everyNthFrame": 1,
	}, nil); err != nil {
		return fmt.Errorf("Page.startScreencast: %w", err)
	}

	go func() {
		<-stop
		stopped.Store(true)
		// Clean up on a fresh session: the frame read below is blocked, and a
		// timed-out read poisons the websocket for further calls.
		cleanup, err := pageSession(port, targetID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[htrcli] opening recorder cleanup session: %v\n", err)
		} else {
			if err := cleanup.Call("Page.stopScreencast", nil, nil); err != nil {
				fmt.Fprintf(os.Stderr, "[htrcli] stopScreencast: %v\n", err)
			}
			restoreControlStatusBar(cleanup, holdRecord)
			_ = cleanup.Close()
		}
		cleanupDone.Store(true)
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
				return nil // clean stop: cleanup ran on the fresh session
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
