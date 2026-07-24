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
