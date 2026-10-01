package commands

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/u007/htrcli/internal/api"
)

// recordingServer stands up a fake daemon that answers exactly one command
// action, and captures the CommandRequest so tests can assert what the CLI
// actually sent.
type recordingServer struct {
	*httptest.Server
	gotAction  string
	gotOptions map[string]any
	gotTabPath string
}

func newRecordingServer(t *testing.T, action string, data any) *recordingServer {
	t.Helper()
	rs := &recordingServer{}
	rs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rs.gotTabPath = r.URL.Path
		var req api.CommandRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decoding request: %v", err)
		}
		rs.gotAction = req.Command.Action
		rs.gotOptions = req.Command.Options

		if req.Command.Action != action {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(api.ApiResponse{
				OK:    false,
				Error: "unexpected action " + req.Command.Action,
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(api.ApiResponse{
			OK:   true,
			Data: api.CommandResult{ID: req.Command.ID, Success: true, Data: data},
		})
	}))
	t.Cleanup(rs.Close)
	return rs
}

// useServer points the package client at rs for the duration of the test.
func useServer(t *testing.T, rs *recordingServer) {
	t.Helper()
	prev := client
	client = api.NewClient(rs.URL, "")
	t.Cleanup(func() { client = prev })
}

func TestRecordingsListDecodesAndSorts(t *testing.T) {
	data := api.RecordingListData{
		Sessions: []api.RecordingSessionMeta{
			{ID: "s1", Title: "First", StartTime: 1000, StepCount: 3},
			{ID: "s2", Title: "Second", StartTime: 2000, StepCount: 7, AnnotationCount: 1, HasAudio: true},
		},
		Total: 2, Limit: 50, Offset: 0,
	}
	rs := newRecordingServer(t, "recordingList", data)
	useServer(t, rs)

	var got api.RecordingListData
	if err := recordingData("recordingList", map[string]any{"limit": 50}, &got); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(got.Sessions))
	}
	if got.Sessions[1].ID != "s2" {
		t.Fatalf("expected s2 second, got %s", got.Sessions[1].ID)
	}
	if got.Sessions[1].AnnotationCount != 1 || !got.Sessions[1].HasAudio {
		t.Fatalf("annotation/hasAudio lost in decode: %+v", got.Sessions[1])
	}
	// Background-handled actions must NOT be pinned to a tab, and must not be
	// routed through /api/command either.
	//
	// CHANGED from `/api/command`. That expectation was correct when a nil tab
	// was resolved by the daemon through FirstTabID, which meant recording
	// commands failed unless some http/https tab with an active content script
	// had reported in. Recordings now use the dedicated tab-less route
	// (POST /api/background/command), which selects a relay CONNECTION instead,
	// so this no longer needs an open page. The intent the old assertion encoded
	// — "not pinned to a tab" — is still enforced, and now more strictly.
	if rs.gotTabPath != "/api/background/command" {
		t.Fatalf("expected the tab-less /api/background/command path, got %s", rs.gotTabPath)
	}
}

func TestRecordingsStartPassesTitleAndAudio(t *testing.T) {
	data := api.RecordingStateData{
		Recording: true,
		Session:   &api.RecordingSessionMeta{ID: "s1", Title: "Checkout", StartTime: 1},
	}
	rs := newRecordingServer(t, "recordingStart", data)
	useServer(t, rs)

	var got api.RecordingStateData
	err := recordingData("recordingStart", map[string]any{
		"title":    "Checkout",
		"hasAudio": true,
	}, &got)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Recording || got.Session == nil || got.Session.Title != "Checkout" {
		t.Fatalf("unexpected state: %+v", got)
	}
	if rs.gotOptions["title"] != "Checkout" {
		t.Fatalf("title not forwarded, got %v", rs.gotOptions["title"])
	}
	// hasAudio must arrive as a real bool so the extension's
	// readBoolean() check (which rejects non-booleans) accepts it.
	if v, ok := rs.gotOptions["hasAudio"].(bool); !ok || !v {
		t.Fatalf("hasAudio not sent as bool true, got %#v", rs.gotOptions["hasAudio"])
	}
}

func TestRecordingsStartOmitsEmptyTitle(t *testing.T) {
	rs := newRecordingServer(t, "recordingStart", api.RecordingStateData{Recording: true})
	useServer(t, rs)

	if err := recordingData("recordingStart", map[string]any{"hasAudio": false}, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, present := rs.gotOptions["title"]; present {
		t.Fatalf("empty title should be omitted so the extension generates a dated one")
	}
}

func TestRecordingsGetSurfacesExtensionError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(api.ApiResponse{
			OK: true,
			Data: api.CommandResult{
				ID:      "1",
				Success: false,
				Error:   "no recording found with id ghost",
			},
		})
	}))
	defer server.Close()
	prev := client
	client = api.NewClient(server.URL, "")
	defer func() { client = prev }()

	var got api.RecordingGetData
	err := recordingData("recordingGet", map[string]any{"sessionId": "ghost"}, &got)
	if err == nil {
		t.Fatal("expected an error for a failed command")
	}
	if !strings.Contains(err.Error(), "no recording found with id ghost") {
		t.Fatalf("error text not propagated: %v", err)
	}
}

func TestRecordingsGetDecodesStepsAndAnnotations(t *testing.T) {
	shot := "data:image/png;base64,AAAA"
	session := api.RecordingSession{
		ID: "s1", Title: "Checkout", StartTime: 1000, HasAudio: false,
		Steps: []api.RecordingStep{
			{ID: "st1", Type: "click", TabID: 7, TabTitle: "Cart", URL: "https://shop/cart", Element: &api.RecordingElementInfo{Tag: "button", Text: "Pay", Selector: "#pay"}},
			{ID: "st2", Type: "navigation", TabID: 7, TabTitle: "Done", URL: "https://shop/done", ScreenshotData: &shot},
		},
		Annotations: []api.RecordingAnnotation{{ID: "a1", Text: "bug: total wrong", Timestamp: 500}},
	}
	rs := newRecordingServer(t, "recordingGet", api.RecordingGetData{Session: session, MediaStripped: false})
	useServer(t, rs)

	var got api.RecordingGetData
	if err := recordingData("recordingGet", map[string]any{
		"sessionId":    "s1",
		"includeMedia": true,
	}, &got); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Session.Steps) != 2 || got.Session.Steps[1].ScreenshotData == nil {
		t.Fatalf("screenshot data lost: %+v", got.Session.Steps)
	}
	if got.Session.Steps[0].Element == nil || got.Session.Steps[0].Element.Selector != "#pay" {
		t.Fatalf("element info lost: %+v", got.Session.Steps[0].Element)
	}
	if len(got.Session.Annotations) != 1 || got.Session.Annotations[0].Text != "bug: total wrong" {
		t.Fatalf("annotations lost: %+v", got.Session.Annotations)
	}
	if rs.gotOptions["includeMedia"] != true {
		t.Fatalf("includeMedia not forwarded: %v", rs.gotOptions["includeMedia"])
	}
}

func TestRecordingsCDPIsRejectedExplicitly(t *testing.T) {
	prevTransport := transportFlag
	transportFlag = "cdp"
	defer func() { transportFlag = prevTransport }()

	_, err := sendRecordingCommand("recordingList", nil)
	if err == nil {
		t.Fatal("expected an error over --cdp")
	}
	if !strings.Contains(err.Error(), "--transport ext") || !strings.Contains(err.Error(), "recordingList") {
		t.Fatalf("error should point at the ext transport, got: %v", err)
	}
}

func TestWriteJSONFileCreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deeper", "out.json")
	if err := writeJSONFile(path, map[string]string{"k": "v"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if !strings.Contains(string(raw), `"k": "v"`) {
		t.Fatalf("unexpected file contents: %s", raw)
	}
	if !strings.HasSuffix(string(raw), "\n") {
		t.Fatalf("file should end with a newline for POSIX friendliness: %q", raw)
	}
}

func TestTruncate(t *testing.T) {
	cases := []struct {
		in   string
		n    int
		want string
	}{
		{"short", 10, "short"},
		{"exactlyten", 10, "exactlyten"},
		{"truncateme", 8, "truncat…"},
		{"", 5, ""},
		{"abcdef", 1, "a"},
		// Multi-byte runes must not be split mid-character.
		{"héllo wörld", 6, "héllo…"},
	}
	for _, c := range cases {
		if got := truncate(c.in, c.n); got != c.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", c.in, c.n, got, c.want)
		}
	}
}

// oversizedListServer answers `recordingList` with a session whose stepCount is
// over the native-messaging frame cap, and fails the test if `recordingGet` is
// ever requested (the pre-check must stop it BEFORE the dangerous call).
func oversizedListServer(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req api.CommandRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decoding request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch req.Command.Action {
		case "recordingList":
			json.NewEncoder(w).Encode(api.ApiResponse{OK: true, Data: api.CommandResult{
				ID: "1", Success: true,
				Data: api.RecordingListData{
					Sessions: []api.RecordingSessionMeta{
						{ID: "huge", Title: "Huge", StartTime: 1, StepCount: maxMediaSteps + 50},
						{ID: "small", Title: "Small", StartTime: 2, StepCount: 3},
					},
					Total: 2, Limit: maxMediaSteps + 1,
				},
			}})
		case "recordingGet":
			t.Error("recordingGet was requested despite the pre-check refusing it — this would have sent the oversized frame")
			json.NewEncoder(w).Encode(api.ApiResponse{OK: true, Data: api.CommandResult{ID: "1", Success: true}})
		default:
			t.Errorf("unexpected action %q", req.Command.Action)
			json.NewEncoder(w).Encode(api.ApiResponse{OK: true, Data: api.CommandResult{ID: "1", Success: true}})
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestPreflightMediaSizeRefusesOversizedSession(t *testing.T) {
	prev := client
	client = api.NewClient(oversizedListServer(t), "")
	defer func() { client = prev }()

	err := preflightMediaSize("huge")
	if err == nil {
		t.Fatal("expected a refusal for an oversized session")
	}
	if !strings.Contains(err.Error(), "64 MiB") {
		t.Fatalf("error should name the cap, got: %v", err)
	}
	if !strings.Contains(err.Error(), "without --with-screenshots") {
		t.Fatalf("error should offer the way out, got: %v", err)
	}
}

func TestPreflightMediaSizeAllowsSmallSession(t *testing.T) {
	prev := client
	client = api.NewClient(oversizedListServer(t), "")
	defer func() { client = prev }()

	if err := preflightMediaSize("small"); err != nil {
		t.Fatalf("a small session must pass the pre-check: %v", err)
	}
}

func TestPreflightMediaSizeAllowsUnknownSession(t *testing.T) {
	prev := client
	client = api.NewClient(oversizedListServer(t), "")
	defer func() { client = prev }()

	// Not in the first page: we cannot size it, so we must not block it.
	if err := preflightMediaSize("not-in-first-page"); err != nil {
		t.Fatalf("an unlisted session must not be blocked: %v", err)
	}
}
