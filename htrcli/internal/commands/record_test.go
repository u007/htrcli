package commands

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"

	"github.com/u007/htrcli/internal/cdp"
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

func TestAcquireRecordStartLockRejectsActiveLock(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	lockPath, err := recordStartLockPath()
	if err != nil {
		t.Fatalf("recordStartLockPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(lockPath), 0700); err != nil {
		t.Fatalf("mkdir lock dir: %v", err)
	}
	if err := os.WriteFile(lockPath, []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
		t.Fatalf("write active lock: %v", err)
	}
	if _, err := acquireRecordStartLock(); err == nil {
		t.Fatal("expected active lock to be rejected")
	}
}

func TestRecordStartCleansUpWhenStateWriteFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	contextName = ""
	contextCDPPort = 0
	transportFlag = "cdp"
	t.Cleanup(func() {
		transportFlag = ""
		cdpFlag = false
		contextName = ""
		contextCDPPort = 0
	})

	origFindFFmpeg := recordFindFFmpeg
	origFFmpegVersion := recordFFmpegVersion
	origWriteRecording := recordWriteRecording
	origCleanupRecorder := recordCleanupRecorder
	origRemoveAll := recordRemoveAll
	t.Cleanup(func() {
		recordFindFFmpeg = origFindFFmpeg
		recordFFmpegVersion = origFFmpegVersion
		recordWriteRecording = origWriteRecording
		recordCleanupRecorder = origCleanupRecorder
		recordRemoveAll = origRemoveAll
	})

	recordFindFFmpeg = func() (string, error) { return "/usr/bin/ffmpeg", nil }
	recordFFmpegVersion = func(string) (string, error) { return "ffmpeg version fake", nil }
	recordWriteRecording = func(*cdp.RecordingState) error { return errors.New("disk full") }
	cleanupCalled := false
	var cleanupState *cdp.RecordingState
	recordCleanupRecorder = func(st *cdp.RecordingState) error {
		cleanupCalled = true
		cleanupState = st
		if st.PID > 0 {
			_ = syscall.Kill(st.PID, syscall.SIGKILL)
		}
		return nil
	}
	removedPath := ""
	recordRemoveAll = func(path string) error {
		removedPath = path
		return os.RemoveAll(path)
	}

	err := recordStartCmd.RunE(recordStartCmd, nil)
	if err == nil {
		t.Fatal("expected record start to fail when state write fails")
	}
	if !cleanupCalled {
		t.Fatal("expected cleanup to run after write failure")
	}
	if cleanupState == nil || cleanupState.PID <= 0 {
		t.Fatalf("expected cleanup state with pid, got %+v", cleanupState)
	}
	if removedPath == "" {
		t.Fatal("expected frames directory cleanup to run")
	}
	if st, err := cdp.ReadRecording(); err != nil {
		t.Fatalf("ReadRecording after failure: %v", err)
	} else if st != nil {
		t.Fatalf("expected no recording state after failure, got %+v", st)
	}
	if lockPath, err := recordStartLockPath(); err == nil {
		if _, statErr := os.Stat(lockPath); !os.IsNotExist(statErr) {
			t.Fatalf("expected recording lock to be removed, stat err = %v", statErr)
		}
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
