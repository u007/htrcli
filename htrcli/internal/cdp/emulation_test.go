package cdp

import (
	"encoding/json"
	"math"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/websocket"
)

// recordingCDP answers every call with an empty result and returns a reader
// for the calls received so far, in order.
func recordingCDP(t *testing.T) (string, func() []fakeMsg) {
	t.Helper()
	var mu sync.Mutex
	var seen []fakeMsg
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		mu.Lock()
		seen = append(seen, m)
		mu.Unlock()
		conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
	})
	return url, func() []fakeMsg {
		mu.Lock()
		defer mu.Unlock()
		return append([]fakeMsg(nil), seen...)
	}
}

func methodsOf(msgs []fakeMsg) []string {
	out := make([]string, len(msgs))
	for i, m := range msgs {
		out[i] = m.Method
	}
	return out
}

func TestApplyEmulationSendsFullSequenceInOrder(t *testing.T) {
	url, calls := recordingCDP(t)
	s, err := Dial(url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer s.Close()

	e := Emulation{Width: 390, Height: 844, Scale: 3, Mobile: true, ColorScheme: "dark", UserAgent: "htrcli-test"}
	if err := ApplyEmulation(s, e); err != nil {
		t.Fatalf("apply: %v", err)
	}

	got := calls()
	want := []string{
		"Emulation.setDeviceMetricsOverride",
		"Emulation.setTouchEmulationEnabled",
		"Emulation.setEmulatedMedia",
		"Network.setUserAgentOverride",
	}
	if strings.Join(methodsOf(got), ",") != strings.Join(want, ",") {
		t.Fatalf("want %v, got %v", want, methodsOf(got))
	}

	var metrics struct {
		Width             int     `json:"width"`
		Height            int     `json:"height"`
		DeviceScaleFactor float64 `json:"deviceScaleFactor"`
		Mobile            bool    `json:"mobile"`
	}
	if err := json.Unmarshal(got[0].Params, &metrics); err != nil {
		t.Fatalf("decode metrics params: %v", err)
	}
	if metrics.Width != 390 || metrics.Height != 844 || metrics.DeviceScaleFactor != 3 || !metrics.Mobile {
		t.Fatalf("bad metrics params: %+v", metrics)
	}
	if !strings.Contains(string(got[2].Params), `"name":"prefers-color-scheme","value":"dark"`) {
		t.Fatalf("bad media params: %s", got[2].Params)
	}
	if !strings.Contains(string(got[3].Params), `"userAgent":"htrcli-test"`) {
		t.Fatalf("bad user-agent params: %s", got[3].Params)
	}
}

func TestApplyEmulationDesktopSkipsTouch(t *testing.T) {
	url, calls := recordingCDP(t)
	s, err := Dial(url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer s.Close()

	if err := ApplyEmulation(s, Emulation{Width: 1280, Height: 800, Scale: 1}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if got := methodsOf(calls()); len(got) != 1 || got[0] != "Emulation.setDeviceMetricsOverride" {
		t.Fatalf("want only metrics call, got %v", got)
	}
}

func TestApplyEmptyEmulationSendsNothing(t *testing.T) {
	url, calls := recordingCDP(t)
	s, err := Dial(url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer s.Close()

	if err := ApplyEmulation(s, Emulation{}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if got := calls(); len(got) != 0 {
		t.Fatalf("empty emulation must make zero CDP calls, got %v", methodsOf(got))
	}
}

func TestApplyEmulationRejectsInvalidBeforeAnyCall(t *testing.T) {
	url, calls := recordingCDP(t)
	s, err := Dial(url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer s.Close()

	if err := ApplyEmulation(s, Emulation{Width: 390, Height: 844}); err == nil {
		t.Fatal("viewport without a device scale must be rejected")
	}
	if got := calls(); len(got) != 0 {
		t.Fatalf("invalid emulation must not reach CDP, got %v", methodsOf(got))
	}
}

func TestApplyEmulationStopsAtFirstCDPError(t *testing.T) {
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		if m.Method == "Emulation.setEmulatedMedia" {
			conn.WriteJSON(map[string]any{"id": m.ID, "error": map[string]any{"code": -32602, "message": "bad media"}})
			return
		}
		conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
	})
	s, err := Dial(url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer s.Close()

	err = ApplyEmulation(s, Emulation{ColorScheme: "dark", UserAgent: "x"})
	if err == nil || !strings.Contains(err.Error(), "bad media") {
		t.Fatalf("want CDP error surfaced, got %v", err)
	}
}

func TestClearEmulationIssuesResetCalls(t *testing.T) {
	url, calls := recordingCDP(t)
	s, err := Dial(url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer s.Close()

	if err := ClearEmulation(s); err != nil {
		t.Fatalf("clear: %v", err)
	}
	got := calls()
	want := []string{
		"Emulation.clearDeviceMetricsOverride",
		"Emulation.setEmulatedMedia",
		"Network.setUserAgentOverride",
	}
	if strings.Join(methodsOf(got), ",") != strings.Join(want, ",") {
		t.Fatalf("want %v, got %v", want, methodsOf(got))
	}
	if len(got[1].Params) != 0 {
		t.Fatalf("setEmulatedMedia reset must carry no params, got %s", got[1].Params)
	}
	if !strings.Contains(string(got[2].Params), `"userAgent":""`) {
		t.Fatalf("user-agent reset must send an empty string, got %s", got[2].Params)
	}
}

func TestEmulationValidate(t *testing.T) {
	cases := []struct {
		name    string
		e       Emulation
		wantErr bool
	}{
		{"empty", Emulation{}, false},
		{"viewport with scale", Emulation{Width: 390, Height: 844, Scale: 2}, false},
		{"mobile alone is allowed", Emulation{Mobile: true}, false},
		{"width without height", Emulation{Width: 390, Scale: 1}, true},
		{"negative width", Emulation{Width: -1, Height: 10, Scale: 1}, true},
		{"zero scale with viewport", Emulation{Width: 390, Height: 844}, true},
		{"negative scale", Emulation{Width: 390, Height: 844, Scale: -1}, true},
		{"NaN scale", Emulation{Width: 390, Height: 844, Scale: math.NaN()}, true},
		{"unknown color scheme", Emulation{ColorScheme: "none"}, true},
		{"dark color scheme", Emulation{ColorScheme: "dark"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.e.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate(%+v) error = %v, wantErr %v", tc.e, err, tc.wantErr)
			}
		})
	}
}
