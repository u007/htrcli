package commands

import (
	"strings"
	"testing"
)

func TestCDPRawArgsRejectMissingMethod(t *testing.T) {
	if _, _, err := parseCDPRawArgs([]string{""}); err == nil {
		t.Fatal("empty method must be rejected")
	}
	if err := cdpRawCmd.Args(cdpRawCmd, nil); err == nil {
		t.Fatal("cobra must reject zero positional arguments")
	}
}

func TestCDPRawArgsRejectBadJSON(t *testing.T) {
	if _, _, err := parseCDPRawArgs([]string{"Page.reload", "{"}); err == nil {
		t.Fatal("malformed params JSON must be rejected")
	}
	if _, _, err := parseCDPRawArgs([]string{"Page.reload", ""}); err == nil {
		t.Fatal("empty params argument must be rejected")
	}
}

func TestCDPRawArgsRejectNonObjectJSON(t *testing.T) {
	for _, raw := range []string{"null", "[1]", `"x"`, "1", "true"} {
		if _, _, err := parseCDPRawArgs([]string{"Page.reload", raw}); err == nil {
			t.Fatalf("params %s must be rejected as non-object", raw)
		}
	}
}

func TestCDPRawArgsNoParamsIsNil(t *testing.T) {
	method, params, err := parseCDPRawArgs([]string{"Browser.getVersion"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if method != "Browser.getVersion" || params != nil {
		t.Fatalf("got method %q params %v, want method only", method, params)
	}
}

func TestCDPRawArgsKeepParamsBytes(t *testing.T) {
	raw := `{"b": 1,  "a": 2.50}`
	_, params, err := parseCDPRawArgs([]string{"Page.x", raw})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(params) != raw {
		t.Fatalf("params %s, want original bytes %s", params, raw)
	}
}

func TestCDPRawRejectsExtTransport(t *testing.T) {
	resetTransportState()
	transportFlag = "ext"
	defer resetTransportState()

	err := cdpRawCmd.RunE(cdpRawCmd, []string{"Browser.getVersion"})
	if err == nil || !strings.Contains(err.Error(), "only supported over --cdp") {
		t.Fatalf("want CDP-only error on extension transport, got %v", err)
	}
}

func TestCDPRawRejectsFrameFlag(t *testing.T) {
	err := validateFrameFlag("#frame", cdpRawCmd, true)
	if err == nil || !strings.Contains(err.Error(), `not supported by "cdp"`) {
		t.Fatalf("want --frame rejection for cdp, got %v", err)
	}
}

func TestCDPRawEmulationNote(t *testing.T) {
	if note := cdpRawEmulationNote("Emulation.setEmulatedMedia"); !strings.Contains(note, "emulate viewport") {
		t.Fatalf("Emulation.* must get the persistent-verb note, got %q", note)
	}
	for _, method := range []string{"Page.getLayoutMetrics", "EmulationFoo.bar", "Emulation"} {
		if note := cdpRawEmulationNote(method); note != "" {
			t.Fatalf("%s must get no note, got %q", method, note)
		}
	}
}

func TestCDPRawSendsParamsToBrowser(t *testing.T) {
	useEmulationStore(t)
	resetTransportState()
	transportFlag = "cdp"
	defer resetTransportState()
	calls := emulationBrowser(t)

	if err := cdpRawCmd.RunE(cdpRawCmd, []string{"Page.getLayoutMetrics"}); err != nil {
		t.Fatalf("call without params: %v", err)
	}
	if err := cdpRawCmd.RunE(cdpRawCmd, []string{"Page.getLayoutMetrics", `{"a":1}`}); err != nil {
		t.Fatalf("call with params: %v", err)
	}

	got := calls()
	if len(got) != 2 {
		t.Fatalf("want 2 CDP calls, got %s", emuMethods(got))
	}
	if len(got[0].Params) != 0 {
		t.Fatalf("call without params sent %s, want no params field", got[0].Params)
	}
	if string(got[1].Params) != `{"a":1}` {
		t.Fatalf("call with params sent %s, want {\"a\":1}", got[1].Params)
	}
}
