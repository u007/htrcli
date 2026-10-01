package commands

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/u007/htrcli/internal/api"
	"github.com/u007/htrcli/internal/output"
)

// --browser is an ADVISORY hint, not a selector: the daemon prefers a relay that
// announced the named browser and falls back to the earliest-connected relay
// when none matches, because `recordings list` reports which browser actually
// answered. These tests pin the CLI half of that contract — that the flag
// reaches the request, that an unusable value is rejected loudly rather than
// silently ignored, and that the CDP rejection still fires.

// browserServer stands up a fake daemon that answers a recordingList, capturing
// the path and the raw body so a test can assert on what the CLI actually sent.
type browserServer struct {
	*httptest.Server
	gotPath  string
	gotBody  map[string]any
	browser  string
	sessions []api.RecordingSessionMeta
}

func newBrowserServer(t *testing.T, browser string) *browserServer {
	t.Helper()
	bs := &browserServer{browser: browser}
	bs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bs.gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&bs.gotBody); err != nil {
			t.Errorf("decoding request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		// Pass the value, not marshalled bytes: CommandResult.Data is `any`, so a
		// []byte would encode as a base64 STRING and the client would fail to
		// decode it into a struct.
		json.NewEncoder(w).Encode(api.ApiResponse{OK: true, Data: api.CommandResult{
			ID: "1", Success: true,
			Data: api.RecordingListData{Sessions: bs.sessions, Browser: browser, Limit: 50},
		}})
	}))
	t.Cleanup(bs.Close)
	return bs
}

func useBrowserServer(t *testing.T, bs *browserServer) {
	t.Helper()
	prev := client
	client = api.NewClient(bs.URL, "")
	t.Cleanup(func() { client = prev })
}

func withBrowserFlag(t *testing.T, value string) {
	t.Helper()
	prev := browserFlag
	t.Cleanup(func() { browserFlag = prev })
	browserFlag = value
}

// The hint must actually reach the daemon, otherwise --browser is a no-op that
// looks like it worked.
func TestRecordingsListSendsBrowserHint(t *testing.T) {
	bs := newBrowserServer(t, "firefox")
	useBrowserServer(t, bs)
	withBrowserFlag(t, "firefox")

	if err := recordingsListCmd.RunE(recordingsListCmd, nil); err != nil {
		t.Fatalf("recordings list: %v", err)
	}
	if bs.gotBody["browser"] != "firefox" {
		t.Errorf("request browser = %v, want firefox", bs.gotBody["browser"])
	}
}

func TestRecordingsListOmitsEmptyBrowserHint(t *testing.T) {
	bs := newBrowserServer(t, "chrome")
	useBrowserServer(t, bs)
	withBrowserFlag(t, "")

	if err := recordingsListCmd.RunE(recordingsListCmd, nil); err != nil {
		t.Fatalf("recordings list: %v", err)
	}
	if v, present := bs.gotBody["browser"]; present {
		t.Errorf("request carries browser = %v, want the key omitted for no preference", v)
	}
}

// The request must use the tab-less route. A nil tab on the OLD route still
// resolves through FirstTabID, so without this the feature would look finished
// while silently keeping the "needs an open http/https tab" limitation.
func TestRecordingsListUsesTheBackgroundRoute(t *testing.T) {
	bs := newBrowserServer(t, "chrome")
	useBrowserServer(t, bs)
	withBrowserFlag(t, "")

	if err := recordingsListCmd.RunE(recordingsListCmd, nil); err != nil {
		t.Fatalf("recordings list: %v", err)
	}
	if bs.gotPath != "/api/background/command" {
		t.Errorf("path = %q, want /api/background/command", bs.gotPath)
	}
}

// A typo must be a loud error. Silently ignoring an unrecognised browser would
// make `htrcli recordings list --browser safri` behave exactly like a
// first-wins call, which reads to the user as success.
func TestValidateBrowserHint(t *testing.T) {
	for _, tc := range []struct {
		value   string
		wantErr string
	}{
		{"", ""},
		{"chrome", ""},
		{"firefox", ""},
		{"safari", "chrome"},
		{"Chrome", "chrome"},
		{"chrome,firefox", "chrome"},
	} {
		err := validateBrowserHint(tc.value)
		switch {
		case tc.wantErr == "" && err != nil:
			t.Errorf("validateBrowserHint(%q) = %v, want nil", tc.value, err)
		case tc.wantErr != "" && err == nil:
			t.Errorf("validateBrowserHint(%q) = nil, want an error naming %q", tc.value, tc.wantErr)
		case tc.wantErr != "" && err != nil && !strings.Contains(err.Error(), tc.wantErr):
			t.Errorf("validateBrowserHint(%q) = %q, want it to name %q", tc.value, err, tc.wantErr)
		}
	}
}

// A browser hint must not become a way around the CDP rejection.
func TestBrowserHintDoesNotBypassCDPRejection(t *testing.T) {
	prev := transportFlag
	t.Cleanup(func() { transportFlag = prev })
	transportFlag = "cdp"
	withBrowserFlag(t, "firefox")

	err := recordingData("recordingList", nil, nil)
	if err == nil {
		t.Fatal("want the CDP rejection even with a browser hint set")
	}
	if !strings.Contains(err.Error(), "not supported over --cdp") {
		t.Errorf("error = %q, want the CDP rejection", err)
	}
}

// The list output must say which browser answered — that is the whole point of
// the hint being advisory rather than a hard selector.
func TestRecordingsListSurfacesBrowserInTable(t *testing.T) {
	bs := newBrowserServer(t, "firefox")
	bs.sessions = []api.RecordingSessionMeta{{ID: "s1", Title: "Checkout", StartTime: 1000, StepCount: 3}}
	useBrowserServer(t, bs)
	withBrowserFlag(t, "")

	prevJSON := output.JSONOutput
	t.Cleanup(func() { output.JSONOutput = prevJSON })
	output.JSONOutput = false

	out := captureStdout(t, func() {
		if err := recordingsListCmd.RunE(recordingsListCmd, nil); err != nil {
			t.Fatalf("recordings list: %v", err)
		}
	})
	if !strings.Contains(out, "firefox") {
		t.Errorf("table output does not name the answering browser:\n%s", out)
	}
}

// The machine-readable path is what a caller parses, so the field has to be
// there too — a table-only column would be invisible to scripts.
func TestRecordingsListSurfacesBrowserInJSON(t *testing.T) {
	bs := newBrowserServer(t, "firefox")
	bs.sessions = []api.RecordingSessionMeta{{ID: "s1", Title: "Checkout", StartTime: 1000, StepCount: 3}}
	useBrowserServer(t, bs)
	withBrowserFlag(t, "")

	prevJSON := output.JSONOutput
	t.Cleanup(func() { output.JSONOutput = prevJSON })
	output.JSONOutput = true

	out := captureStdout(t, func() {
		if err := recordingsListCmd.RunE(recordingsListCmd, nil); err != nil {
			t.Fatalf("recordings list: %v", err)
		}
	})
	// Decode rather than string-match: PrintJSON uses MarshalIndent, so a
	// compact `"browser":"firefox"` literal would never appear.
	var decoded map[string]any
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("JSON output is not valid JSON: %v\n%s", err, out)
	}
	if decoded["browser"] != "firefox" {
		t.Errorf("JSON browser = %v, want firefox\n%s", decoded["browser"], out)
	}
}
