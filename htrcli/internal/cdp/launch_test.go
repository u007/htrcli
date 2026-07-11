package cdp

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestLaunchArgs(t *testing.T) {
	args := LaunchArgs(9222, "/home/u/.htrcli/chrome-profile", false)
	for _, want := range []string{
		"--remote-debugging-port=9222",
		"--user-data-dir=/home/u/.htrcli/chrome-profile",
		"--no-first-run",
		"--disable-backgrounding-occluded-windows",
		"--disable-renderer-backgrounding",
	} {
		if !slices.Contains(args, want) {
			t.Errorf("missing %s in %v", want, args)
		}
	}
	if slices.Contains(args, "--headless") {
		t.Error("headless flag present without headless=true")
	}
	for _, a := range args {
		if strings.Contains(a, "--remote-debugging-address") {
			t.Fatal("must never pass --remote-debugging-address")
		}
	}
}

func TestLaunchArgsHeadless(t *testing.T) {
	args := LaunchArgs(9333, "/p", true)
	if !slices.Contains(args, "--headless") {
		t.Error("want plain --headless")
	}
	if slices.Contains(args, "--headless=new") {
		t.Error("--headless=new is a deprecated alias; use --headless")
	}
}

func TestFindChromeConfigured(t *testing.T) {
	f := filepath.Join(t.TempDir(), "chrome")
	if err := os.WriteFile(f, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	got, err := FindChrome(f)
	if err != nil || got != f {
		t.Fatalf("want %s, got %s (%v)", f, got, err)
	}
}

func TestFindChromeMissing(t *testing.T) {
	// Can't simulate "both missing" when the system Chrome exists at the
	// default macOS path; skip so the test still validates the error elsewhere.
	if _, err := os.Stat(macChromePath); err == nil {
		t.Skipf("system Chrome present at %s; cannot test missing-path error", macChromePath)
	}
	_, err := FindChrome(filepath.Join(t.TempDir(), "nope"))
	if err == nil || !strings.Contains(err.Error(), "set-chrome-path") {
		t.Fatalf("error must mention config set-chrome-path, got %v", err)
	}
}

func TestReadStateMissingFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	st, err := ReadState()
	if err != nil || st != nil {
		t.Fatalf("want nil,nil for missing file, got %v,%v", st, err)
	}
}

func TestStateRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	want := &BrowserState{PID: 42, Port: 9222, Headless: true}
	if err := writeState(want); err != nil {
		t.Fatal(err)
	}
	got, err := ReadState()
	if err != nil || got == nil || got.PID != 42 || !got.Headless {
		t.Fatalf("round trip failed: %v %v", got, err)
	}
}
