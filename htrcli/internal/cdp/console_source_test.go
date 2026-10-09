package cdp

import "testing"

// The extension patches console.* in the page's MAIN world, so its wrapper is
// callFrames[0] of every console event on a patched page (Notion report,
// 2026-10-08: chrome-extension://…/chunk-*.js:2 attributed as the source of
// Notion's own console output). frameSource must skip extension frames the
// same way the TS side does: prefer the first page frame, and when every
// frame is extension code report nothing rather than the wrapper.
func TestFrameSourceSkipsExtensionFrames(t *testing.T) {
	st := &stackTrace{CallFrames: []callFrame{
		{URL: "chrome-extension://abcdefghijklmnop/assets/chunk-CaEYyn41.js", LineNumber: 1},
		{URL: "https://app.notion.com/_assets/transport-support.js", LineNumber: 0},
		{URL: "https://app.notion.com/_assets/56643.js", LineNumber: 0},
	}}
	got := frameSource(st)
	if got != "https://app.notion.com/_assets/transport-support.js:1" {
		t.Errorf("frameSource = %q, want the first PAGE frame", got)
	}
}

func TestFrameSourceSkipsFirefoxExtensionFrames(t *testing.T) {
	st := &stackTrace{CallFrames: []callFrame{
		{URL: "moz-extension://uuid/contentScript/consoleCapture.js", LineNumber: 87},
		{URL: "https://app.test/app.js", LineNumber: 6},
	}}
	if got := frameSource(st); got != "https://app.test/app.js:7" {
		t.Errorf("frameSource = %q, want the page frame", got)
	}
}

func TestFrameSourceAllExtensionFramesIsEmpty(t *testing.T) {
	st := &stackTrace{CallFrames: []callFrame{
		{URL: "chrome-extension://abcdefghijklmnop/assets/chunk-CaEYyn41.js", LineNumber: 1},
		{URL: "chrome-extension://abcdefghijklmnop/assets/chunk-CaEYyn41.js", LineNumber: 2},
	}}
	if got := frameSource(st); got != "" {
		t.Errorf("frameSource = %q, want empty (never attribute to the wrapper)", got)
	}
}

func TestFrameSourceUnchangedForPlainPageFrames(t *testing.T) {
	st := &stackTrace{CallFrames: []callFrame{
		{URL: "http://127.0.0.1/page", LineNumber: 9},
	}}
	if got := frameSource(st); got != "http://127.0.0.1/page:10" {
		t.Errorf("frameSource = %q, want page:10", got)
	}
}
