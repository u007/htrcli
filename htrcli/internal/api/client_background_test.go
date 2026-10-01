package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// backgroundServer answers one POST with the given body, and captures the raw
// request so a test can assert on the path AND on what the body contained.
func backgroundServer(t *testing.T, body any) (*httptest.Server, *string, *map[string]any) {
	t.Helper()
	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(body); err != nil {
			t.Errorf("encoding response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &gotPath, &gotBody
}

func okResult(data any) ApiResponse {
	return ApiResponse{OK: true, Data: CommandResult{ID: "1", Success: true, Data: mustJSON(data)}}
}

func mustJSON(v any) json.RawMessage {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return raw
}

func TestExecuteBackgroundCommandIntoDecodesDataDirectly(t *testing.T) {
	want := payload{Title: "Checkout", Steps: []string{"a", "b"}, Total: 2, MediaHit: true}
	srv, gotPath, _ := backgroundServer(t, okResult(want))

	c := NewClient(srv.URL, "")
	var got payload
	if err := c.ExecuteBackgroundCommandInto(Command{Action: "recordingList"}, "firefox", &got); err != nil {
		t.Fatalf("ExecuteBackgroundCommandInto: %v", err)
	}

	if *gotPath != "/api/background/command" {
		t.Errorf("path = %q, want /api/background/command", *gotPath)
	}
	if got.Title != want.Title || got.Total != want.Total || !got.MediaHit || len(got.Steps) != 2 {
		t.Errorf("decoded %+v, want %+v", got, want)
	}
}

// The hint must reach the daemon so it can pick the right browser profile.
func TestExecuteBackgroundCommandIntoSendsBrowserHint(t *testing.T) {
	srv, _, gotBody := backgroundServer(t, okResult(payload{Title: "x"}))

	c := NewClient(srv.URL, "")
	var got payload
	if err := c.ExecuteBackgroundCommandInto(Command{Action: "recordingList"}, "firefox", &got); err != nil {
		t.Fatalf("ExecuteBackgroundCommandInto: %v", err)
	}

	if (*gotBody)["browser"] != "firefox" {
		t.Errorf("request browser = %v, want firefox", (*gotBody)["browser"])
	}
}

// An empty hint must be omitted entirely, so the request body for a caller with
// no preference is unchanged from the tab-less shape.
func TestExecuteBackgroundCommandIntoOmitsEmptyBrowserHint(t *testing.T) {
	srv, _, gotBody := backgroundServer(t, okResult(payload{Title: "x"}))

	c := NewClient(srv.URL, "")
	var got payload
	if err := c.ExecuteBackgroundCommandInto(Command{Action: "recordingList"}, "", &got); err != nil {
		t.Fatalf("ExecuteBackgroundCommandInto: %v", err)
	}

	if v, present := (*gotBody)["browser"]; present {
		t.Errorf("request body carries browser = %v, want the key omitted for an empty hint", v)
	}
}

// A daemon-level failure (the ApiResponse envelope) is an APIError, so callers
// can tell "the daemon refused" from "the command failed".
func TestExecuteBackgroundCommandIntoSurfacesDaemonError(t *testing.T) {
	srv, _, _ := backgroundServer(t, ApiResponse{OK: false, Error: "no browser connected"})

	c := NewClient(srv.URL, "")
	var got payload
	err := c.ExecuteBackgroundCommandInto(Command{Action: "recordingList"}, "", &got)

	if err == nil {
		t.Fatal("want an error for a failed envelope")
	}
	var apiErr *APIError
	if !asAPIError(err, &apiErr) {
		t.Fatalf("error is %T, want *APIError", err)
	}
	if apiErr.Message != "no browser connected" {
		t.Errorf("message = %q, want %q", apiErr.Message, "no browser connected")
	}
}

// The extension reported failure with a real message.
func TestExecuteBackgroundCommandIntoSurfacesCommandError(t *testing.T) {
	srv, _, _ := backgroundServer(t, ApiResponse{OK: true, Data: CommandResult{
		ID: "1", Success: false, Error: "recording xyz is currently being recorded",
	}})

	c := NewClient(srv.URL, "")
	var got payload
	err := c.ExecuteBackgroundCommandInto(Command{Action: "recordingGet"}, "", &got)

	if err == nil {
		t.Fatal("want an error when the command failed")
	}
	if !strings.Contains(err.Error(), "currently being recorded") {
		t.Errorf("error = %q, want the extension's message", err)
	}
}

// Failure with an EMPTY error must not read as success at the call site.
func TestExecuteBackgroundCommandIntoEmptyCommandErrorStillReports(t *testing.T) {
	srv, _, _ := backgroundServer(t, ApiResponse{OK: true, Data: CommandResult{
		ID: "1", Success: false, Error: "",
	}})

	c := NewClient(srv.URL, "")
	var got payload
	err := c.ExecuteBackgroundCommandInto(Command{Action: "recordingStatus"}, "", &got)

	if err == nil {
		t.Fatal("a failed command with an empty error must still return an error")
	}
	if err.Error() == "" {
		t.Error("error message is empty, so the failure is indistinguishable from success")
	}
	if !strings.Contains(err.Error(), "recordingStatus") {
		t.Errorf("error = %q, want it to name the action", err)
	}
}

// A success carrying no data is not a failure: out is left untouched.
func TestExecuteBackgroundCommandIntoSuccessWithoutData(t *testing.T) {
	srv, _, _ := backgroundServer(t, ApiResponse{OK: true, Data: CommandResult{ID: "1", Success: true}})

	c := NewClient(srv.URL, "")
	got := payload{Title: "untouched"}
	if err := c.ExecuteBackgroundCommandInto(Command{Action: "recordingStart"}, "", &got); err != nil {
		t.Fatalf("a success with no data must not error, got %v", err)
	}
	if got.Title != "untouched" {
		t.Errorf("out was modified to %q, want it left alone", got.Title)
	}
}

// out == nil is allowed: the caller wants the side effect, not the payload.
func TestExecuteBackgroundCommandIntoAllowsNilOut(t *testing.T) {
	srv, _, _ := backgroundServer(t, okResult(payload{Title: "x"}))

	c := NewClient(srv.URL, "")
	if err := c.ExecuteBackgroundCommandInto(Command{Action: "recordingList"}, "", nil); err != nil {
		t.Fatalf("out=nil must be accepted, got %v", err)
	}
}

// A payload that does not match the caller's type is a decode error, not a
// silent zero value.
func TestExecuteBackgroundCommandIntoDecodeMismatch(t *testing.T) {
	srv, _, _ := backgroundServer(t, okResult(map[string]any{"total": "not a number"}))

	c := NewClient(srv.URL, "")
	var got payload
	if err := c.ExecuteBackgroundCommandInto(Command{Action: "recordingList"}, "", &got); err == nil {
		t.Fatal("want a decode error for a mismatched payload")
	}
}

// asAPIError is a small errors.As shim kept local so this file does not depend on
// the error helpers the other client tests use.
func asAPIError(err error, target **APIError) bool {
	for err != nil {
		if e, ok := err.(*APIError); ok {
			*target = e
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
