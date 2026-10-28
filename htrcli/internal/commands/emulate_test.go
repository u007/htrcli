package commands

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/u007/htrcli/internal/cdp"
)

type emuCall struct {
	Method string
	Params json.RawMessage
}

// useEmulationStore points the emulation store at a temp file for one test.
func useEmulationStore(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "emulation.json")
	emulationStorePathOverride = path
	t.Cleanup(func() { emulationStorePathOverride = "" })
	return path
}

// emulationBrowser is a fake Chrome: /json discovery plus one page WebSocket.
// It records every CDP call and answers the readback expression with a fixed
// object. The CDP port is wired into viper so GetCDPPort points at it.
func emulationBrowser(t *testing.T) func() []emuCall {
	t.Helper()
	return emulationBrowserFailing(t, "")
}

// emulationBrowserFailing is emulationBrowser whose fake Chrome answers
// failMethod with a CDP error instead of an empty result.
func emulationBrowserFailing(t *testing.T, failMethod string) func() []emuCall {
	t.Helper()
	var mu sync.Mutex
	var seen []emuCall
	upgrader := websocket.Upgrader{}
	mux := http.NewServeMux()
	mux.HandleFunc("/json", func(w http.ResponseWriter, r *http.Request) {
		targets := []map[string]any{{
			"id":                   "T1",
			"type":                 "page",
			"title":                "fixture",
			"url":                  "about:blank",
			"webSocketDebuggerUrl": "ws://" + r.Host + "/devtools/page/T1",
		}}
		if err := json.NewEncoder(w).Encode(targets); err != nil {
			t.Errorf("encode targets: %v", err)
		}
	})
	mux.HandleFunc("/devtools/page/T1", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()
		for {
			var m struct {
				ID     int64           `json:"id"`
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			}
			if err := conn.ReadJSON(&m); err != nil {
				return // intentionally not logged: the client closing the socket ends the fake browser loop
			}
			mu.Lock()
			seen = append(seen, emuCall{Method: m.Method, Params: m.Params})
			mu.Unlock()
			result := map[string]any{}
			if m.Method == "Runtime.evaluate" {
				result = map[string]any{"result": map[string]any{"type": "object", "value": map[string]any{
					"innerWidth": 390, "innerHeight": 844, "prefersColorScheme": "dark", "userAgent": "htrcli-test",
				}}}
			}
			reply := map[string]any{"id": m.ID, "result": result}
			if failMethod != "" && m.Method == failMethod {
				reply = map[string]any{"id": m.ID, "error": map[string]any{"code": -32000, "message": "fake rejection of " + failMethod}}
			}
			if err := conn.WriteJSON(reply); err != nil {
				t.Errorf("write %s response: %v", m.Method, err)
				return
			}
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse fake browser URL: %v", err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("fake browser port: %v", err)
	}
	setViperCDPPort(t, port)

	return func() []emuCall {
		mu.Lock()
		defer mu.Unlock()
		return append([]emuCall(nil), seen...)
	}
}

func emuMethods(calls []emuCall) string {
	names := make([]string, len(calls))
	for i, c := range calls {
		names[i] = c.Method
	}
	return strings.Join(names, ",")
}

func TestEmulationStoreRoundTripKeepsEachPort(t *testing.T) {
	useEmulationStore(t)
	seedEmulation(t, 9222, cdp.Emulation{Width: 390, Height: 844, Scale: 3, Mobile: true})
	seedEmulation(t, 9333, cdp.Emulation{ColorScheme: "dark", UserAgent: "htrcli-test"})
	got, err := loadEmulationStore()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 entries, got %d", len(got))
	}
	for port, want := range map[int]cdp.Emulation{
		9222: {Width: 390, Height: 844, Scale: 3, Mobile: true},
		9333: {ColorScheme: "dark", UserAgent: "htrcli-test"},
	} {
		e, err := portEmulation(got, port)
		if err != nil {
			t.Fatalf("port %d: %v", port, err)
		}
		if e != want {
			t.Fatalf("port %d: want %+v, got %+v", port, want, e)
		}
	}
}

func TestEmulationStoreMissingFileIsEmpty(t *testing.T) {
	useEmulationStore(t)
	got, err := loadEmulationStore()
	if err != nil {
		t.Fatalf("missing file must load as empty, got %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want empty store, got %+v", got)
	}
}

func TestEmulationStoreMalformedDocumentIsError(t *testing.T) {
	cases := map[string]string{
		"not json":       `{not json`,
		"null document":  `null`,
		"array document": `[]`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			path := useEmulationStore(t)
			writeEmulationFile(t, path, body)
			if _, err := loadEmulationStore(); err == nil {
				t.Fatalf("malformed store %q must be an error", body)
			}
		})
	}
}

func TestParseViewportSize(t *testing.T) {
	good := map[string][2]int{"390x844": {390, 844}, "1x1": {1, 1}}
	for raw, want := range good {
		w, h, err := parseViewportSize(raw)
		if err != nil || w != want[0] || h != want[1] {
			t.Fatalf("%q: want %v, got %d,%d err=%v", raw, want, w, h, err)
		}
	}
	for _, raw := range []string{"", "390", "390x", "x844", "0x844", "390x0", "-1x844", "390x-5", "abcx844", "390X844", "390x844x1"} {
		if _, _, err := parseViewportSize(raw); err == nil {
			t.Fatalf("%q must be rejected", raw)
		}
	}
}

func TestParseColorScheme(t *testing.T) {
	cases := map[string]string{"light": "light", "dark": "dark", "none": ""}
	for raw, want := range cases {
		got, err := parseColorScheme(raw)
		if err != nil || got != want {
			t.Fatalf("%q: want %q, got %q err=%v", raw, want, got, err)
		}
	}
	for _, raw := range []string{"", "Dark", "auto", "no-preference"} {
		if _, err := parseColorScheme(raw); err == nil {
			t.Fatalf("%q must be rejected", raw)
		}
	}
}

func TestCDPSessionWithEmptyStoreMakesNoEmulationCalls(t *testing.T) {
	useEmulationStore(t)
	calls := emulationBrowser(t)

	s, _, err := cdpSession()
	if err != nil {
		t.Fatalf("cdpSession: %v", err)
	}
	s.Close()
	if got := calls(); len(got) != 0 {
		t.Fatalf("empty store must add zero CDP calls, got %s", emuMethods(got))
	}
}

func TestCDPSessionReappliesStoredEmulation(t *testing.T) {
	useEmulationStore(t)
	calls := emulationBrowser(t)
	port := GetCDPPort()
	seedEmulation(t, port, cdp.Emulation{Width: 390, Height: 844, Scale: 2, Mobile: true, ColorScheme: "dark"})

	s, _, err := cdpSession()
	if err != nil {
		t.Fatalf("cdpSession: %v", err)
	}
	s.Close()
	want := "Emulation.setDeviceMetricsOverride,Emulation.setTouchEmulationEnabled,Emulation.setEmulatedMedia"
	if got := emuMethods(calls()); got != want {
		t.Fatalf("want %s, got %s", want, got)
	}
}

func TestCDPSessionFailsOnMalformedStoreBeforeDialing(t *testing.T) {
	path := useEmulationStore(t)
	calls := emulationBrowser(t)
	if err := os.WriteFile(path, []byte(`{broken`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	if _, _, err := cdpSession(); err == nil {
		t.Fatal("malformed emulation store must fail the command")
	}
	if got := calls(); len(got) != 0 {
		t.Fatalf("no CDP call expected after store failure, got %s", emuMethods(got))
	}
}

func TestUpdateEmulationPersistsAndReadsBack(t *testing.T) {
	useEmulationStore(t)
	calls := emulationBrowser(t)
	port := GetCDPPort()

	if err := updateEmulation(func(e *cdp.Emulation) { e.ColorScheme = "dark" }); err != nil {
		t.Fatalf("updateEmulation: %v", err)
	}
	store, err := loadEmulationStore()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	e, err := portEmulation(store, port)
	if err != nil {
		t.Fatalf("stored entry: %v", err)
	}
	if e.ColorScheme != "dark" {
		t.Fatalf("color scheme not persisted: %+v", e)
	}
	want := "Emulation.clearDeviceMetricsOverride,Emulation.setEmulatedMedia,Network.setUserAgentOverride,Emulation.setEmulatedMedia,Runtime.evaluate"
	if got := emuMethods(calls()); got != want {
		t.Fatalf("want %s (clear, apply, then readback), got %s", want, got)
	}
}

func TestUpdateEmulationInvalidValueIsNotStored(t *testing.T) {
	path := useEmulationStore(t)
	calls := emulationBrowser(t)

	err := updateEmulation(func(e *cdp.Emulation) {
		e.Width = 390
		e.Height = 844
		e.Scale = 0
	})
	if err == nil {
		t.Fatal("zero device scale must be rejected")
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("invalid emulation must not write the store, stat err=%v", statErr)
	}
	if got := calls(); len(got) != 0 {
		t.Fatalf("invalid emulation must not reach CDP, got %s", emuMethods(got))
	}
}

func TestUpdateEmulationRejectedByBrowserIsNotStored(t *testing.T) {
	path := useEmulationStore(t)
	calls := emulationBrowserFailing(t, "Emulation.setDeviceMetricsOverride")

	err := updateEmulation(func(e *cdp.Emulation) {
		e.Width = 390
		e.Height = 844
		e.Scale = 1
	})
	if err == nil {
		t.Fatal("a CDP rejection must fail the command")
	}
	if !strings.Contains(err.Error(), "fake rejection") {
		t.Fatalf("error must carry the CDP message, got %v", err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("a rejected emulation must not be persisted, stat err=%v", statErr)
	}
	want := "Emulation.clearDeviceMetricsOverride,Emulation.setEmulatedMedia,Network.setUserAgentOverride,Emulation.setDeviceMetricsOverride"
	if got := emuMethods(calls()); got != want {
		t.Fatalf("want %s, got %s", want, got)
	}
}

func TestUpdateEmulationRemovesEntryWhenEmptied(t *testing.T) {
	useEmulationStore(t)
	emulationBrowser(t)
	port := strconv.Itoa(GetCDPPort())

	if err := updateEmulation(func(e *cdp.Emulation) { e.ColorScheme = "dark" }); err != nil {
		t.Fatalf("set dark: %v", err)
	}
	if err := updateEmulation(func(e *cdp.Emulation) { e.ColorScheme = "" }); err != nil {
		t.Fatalf("clear color scheme: %v", err)
	}
	store, err := loadEmulationStore()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, ok := store[port]; ok {
		t.Fatalf("emptied entry must be removed so the port makes no CDP calls, got %+v", store)
	}
}

func TestRunEmulateResetClearsStoreAndIssuesClearCalls(t *testing.T) {
	useEmulationStore(t)
	calls := emulationBrowser(t)
	port := GetCDPPort()
	seedEmulation(t, port, cdp.Emulation{Width: 390, Height: 844, Scale: 1, UserAgent: "x"})

	if err := runEmulateReset(); err != nil {
		t.Fatalf("reset: %v", err)
	}
	store, err := loadEmulationStore()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(store) != 0 {
		t.Fatalf("reset must clear the port entry, got %+v", store)
	}
	want := "Emulation.clearDeviceMetricsOverride,Emulation.setEmulatedMedia,Network.setUserAgentOverride"
	if got := emuMethods(calls()); got != want {
		t.Fatalf("reset must clear without re-applying the old entry: want %s, got %s", want, got)
	}
}

func TestDescribeEmulation(t *testing.T) {
	if got := describeEmulation(cdp.Emulation{}); got != "none" {
		t.Fatalf("empty emulation: want none, got %q", got)
	}
	got := describeEmulation(cdp.Emulation{Width: 390, Height: 844, Scale: 2, Mobile: true, ColorScheme: "dark", UserAgent: "ua"})
	for _, part := range []string{"viewport 390x844 scale 2", "mobile", "color-scheme dark", `user-agent "ua"`} {
		if !strings.Contains(got, part) {
			t.Fatalf("describeEmulation missing %q in %q", part, got)
		}
	}
}

// seedEmulation stores e for port through the same helpers the commands use.
func seedEmulation(t *testing.T, port int, e cdp.Emulation) {
	t.Helper()
	store, err := loadEmulationStore()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := setPortEmulation(store, port, e); err != nil {
		t.Fatalf("set entry: %v", err)
	}
	if err := saveEmulationStore(store); err != nil {
		t.Fatalf("save: %v", err)
	}
}

// writeEmulationFile replaces the raw store file, as a hand edit would.
func writeEmulationFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func TestCDPSessionRejectsBadOwnEntry(t *testing.T) {
	cases := map[string]string{
		"viewport without scale": `{"width": 390, "height": 844}`,
		"unknown color scheme":   `{"colorScheme": "none"}`,
		"partial viewport":       `{"width": 390, "scale": 1}`,
		"wrong JSON type":        `"dark"`,
	}
	for name, entry := range cases {
		t.Run(name, func(t *testing.T) {
			path := useEmulationStore(t)
			calls := emulationBrowser(t)
			port := GetCDPPort()
			writeEmulationFile(t, path, fmt.Sprintf(`{"%d": %s}`, port, entry))

			s, _, err := cdpSession()
			if err == nil {
				s.Close()
				t.Fatalf("bad entry for port %d must fail the command", port)
			}
			if !strings.Contains(err.Error(), fmt.Sprintf("port %d", port)) || !strings.Contains(err.Error(), "emulate reset") {
				t.Fatalf("error must name port %d and the reset command, got %v", port, err)
			}
			if got := calls(); len(got) != 0 {
				t.Fatalf("bad entry must fail before any CDP call, got %s", emuMethods(got))
			}
		})
	}
}

func TestCDPSessionIgnoresBadOtherPortEntry(t *testing.T) {
	path := useEmulationStore(t)
	calls := emulationBrowser(t)
	writeEmulationFile(t, path, `{"1": {"width": 390}}`)

	s, _, err := cdpSession()
	if err != nil {
		t.Fatalf("a bad entry on another port must not block this port: %v", err)
	}
	s.Close()
	if got := calls(); len(got) != 0 {
		t.Fatalf("no emulation for this port, got %s", emuMethods(got))
	}
}

func TestEmulateResetRemovesOwnBadEntry(t *testing.T) {
	path := useEmulationStore(t)
	calls := emulationBrowser(t)
	port := GetCDPPort()
	writeEmulationFile(t, path, fmt.Sprintf(`{"%d": {"width": 390, "height": 844}}`, port))

	if err := runEmulateReset(); err != nil {
		t.Fatalf("reset must clear a bad entry for its own port: %v", err)
	}
	store, err := loadEmulationStore()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, ok := store[strconv.Itoa(port)]; ok {
		t.Fatalf("bad entry must be removed, got %s", store)
	}
	want := "Emulation.clearDeviceMetricsOverride,Emulation.setEmulatedMedia,Network.setUserAgentOverride"
	if got := emuMethods(calls()); got != want {
		t.Fatalf("want %s, got %s", want, got)
	}
}

func TestEmulateResetKeepsOtherPortEntries(t *testing.T) {
	path := useEmulationStore(t)
	emulationBrowser(t)
	port := GetCDPPort()
	writeEmulationFile(t, path, fmt.Sprintf(`{"%d": {"width": 390}, "1": {"width": 390}}`, port))

	if err := runEmulateReset(); err != nil {
		t.Fatalf("reset: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read store: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("store must stay valid JSON: %v", err)
	}
	if _, ok := doc[strconv.Itoa(port)]; ok {
		t.Fatalf("this port's entry must be removed, got %s", data)
	}
	other, ok := doc["1"].(map[string]any)
	if !ok || other["width"] != 390.0 {
		t.Fatalf("other port's entry must survive unchanged, got %s", data)
	}
}

func TestEmulateShowReportsOnlyItsOwnBadEntry(t *testing.T) {
	path := useEmulationStore(t)
	port := GetCDPPort()
	writeEmulationFile(t, path, fmt.Sprintf(`{"%d": {"width": 390}}`, port))

	err := runEmulateShow()
	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("port %d", port)) {
		t.Fatalf("show must report this port's bad entry as an error, got %v", err)
	}

	writeEmulationFile(t, path, `{"1": {"width": 390}}`)
	if err := runEmulateShow(); err != nil {
		t.Fatalf("show must ignore a bad entry on another port: %v", err)
	}
}

func TestUpdateEmulationClearsStoredFieldsBeforeApplying(t *testing.T) {
	useEmulationStore(t)
	calls := emulationBrowser(t)
	port := GetCDPPort()
	seedEmulation(t, port, cdp.Emulation{Width: 390, Height: 844, Scale: 1, ColorScheme: "dark"})

	if err := updateEmulation(func(e *cdp.Emulation) { e.ColorScheme = "" }); err != nil {
		t.Fatalf("updateEmulation: %v", err)
	}
	got := calls()
	want := "Emulation.setDeviceMetricsOverride,Emulation.setEmulatedMedia,Emulation.clearDeviceMetricsOverride,Emulation.setEmulatedMedia,Network.setUserAgentOverride,Emulation.setDeviceMetricsOverride,Runtime.evaluate"
	if seq := emuMethods(got); seq != want {
		t.Fatalf("want %s, got %s", want, seq)
	}
	cleared := false
	for _, c := range got {
		if c.Method == "Emulation.clearDeviceMetricsOverride" {
			cleared = true
			continue
		}
		if cleared && c.Method == "Emulation.setEmulatedMedia" && strings.Contains(string(c.Params), "dark") {
			t.Fatalf("stored dark scheme still in effect after the clear: %s", c.Params)
		}
	}
}
