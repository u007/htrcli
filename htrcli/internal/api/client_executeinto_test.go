package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// payload is a stand-in for a recording result: a nested object with a big
// string field, which is the shape ExecuteCommandInto exists to avoid copying.
type payload struct {
	Title    string   `json:"title"`
	Steps    []string `json:"steps"`
	Total    int      `json:"total"`
	MediaHit bool     `json:"mediaHit"`
}

// commandServer answers one /api/command POST with the given envelope.
func commandServer(t *testing.T, body any) (*httptest.Server, *string) {
	t.Helper()
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(body); err != nil {
			t.Errorf("encoding response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &gotPath
}

func TestExecuteCommandIntoDecodesDataDirectly(t *testing.T) {
	want := payload{Title: "Checkout", Steps: []string{"a", "b"}, Total: 2, MediaHit: true}
	srv, gotPath := commandServer(t, ApiResponse{
		OK: true,
		Data: CommandResult{
			ID:      "1",
			Success: true,
			Data:    want,
		},
	})

	var got payload
	c := NewClient(srv.URL, "")
	if err := c.ExecuteCommandInto(nil, Command{ID: "1", Action: "recordingGet"}, &got); err != nil {
		t.Fatalf("ExecuteCommandInto: %v", err)
	}
	if *gotPath != "/api/command" {
		t.Errorf("path = %q, want /api/command for a nil tab", *gotPath)
	}
	if got.Title != want.Title || got.Total != want.Total || got.MediaHit != want.MediaHit {
		t.Errorf("decoded %+v, want %+v", got, want)
	}
	if len(got.Steps) != 2 {
		t.Errorf("Steps = %v, want 2 entries", got.Steps)
	}
}

func TestExecuteCommandIntoTargetsTab(t *testing.T) {
	srv, gotPath := commandServer(t, ApiResponse{
		OK:   true,
		Data: CommandResult{ID: "1", Success: true},
	})
	tab := 42
	c := NewClient(srv.URL, "")
	if err := c.ExecuteCommandInto(&tab, Command{ID: "1", Action: "recordingGet"}, nil); err != nil {
		t.Fatalf("ExecuteCommandInto: %v", err)
	}
	if *gotPath != "/api/tabs/42/command" {
		t.Errorf("path = %q, want /api/tabs/42/command", *gotPath)
	}
}

func TestExecuteCommandIntoSurfacesCommandFailure(t *testing.T) {
	srv, _ := commandServer(t, ApiResponse{
		OK: true,
		Data: CommandResult{
			ID:      "1",
			Success: false,
			Error:   "no recording found with id abc",
		},
	})
	c := NewClient(srv.URL, "")
	err := c.ExecuteCommandInto(nil, Command{ID: "1", Action: "recordingGet"}, nil)
	if err == nil {
		t.Fatal("expected an error when the command reports success:false")
	}
	if !strings.Contains(err.Error(), "no recording found with id abc") {
		t.Errorf("error should carry the extension's message, got: %v", err)
	}
}

func TestExecuteCommandIntoSurfacesEmptyCommandError(t *testing.T) {
	// A failed command with no message used to surface as an empty error
	// string, which reads like success at the call site.
	srv, _ := commandServer(t, ApiResponse{
		OK:   true,
		Data: CommandResult{ID: "1", Success: false},
	})
	c := NewClient(srv.URL, "")
	err := c.ExecuteCommandInto(nil, Command{ID: "1", Action: "recordingStop"}, nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.TrimSpace(err.Error()) == "" {
		t.Errorf("error must not be empty, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "recordingStop") {
		t.Errorf("error should name the failing action, got: %v", err)
	}
}

func TestExecuteCommandIntoSurfacesEnvelopeFailure(t *testing.T) {
	// ok:false is a transport-level refusal, distinct from success:false.
	srv, _ := commandServer(t, ApiResponse{OK: false, Error: "no tabs connected"})
	c := NewClient(srv.URL, "")
	err := c.ExecuteCommandInto(nil, Command{ID: "1", Action: "recordingList"}, nil)
	if err == nil {
		t.Fatal("expected an error when the envelope is not ok")
	}
	if !strings.Contains(err.Error(), "no tabs connected") {
		t.Errorf("error should carry the daemon's message, got: %v", err)
	}
}

func TestExecuteCommandIntoTolerantOfMissingData(t *testing.T) {
	// A success with no data field must not fail the call.
	srv, _ := commandServer(t, ApiResponse{OK: true, Data: CommandResult{ID: "1", Success: true}})
	c := NewClient(srv.URL, "")
	if err := c.ExecuteCommandInto(nil, Command{ID: "1", Action: "recordingStop"}, nil); err != nil {
		t.Fatalf("a success with no data must not error: %v", err)
	}
	var got payload
	if err := c.ExecuteCommandInto(nil, Command{ID: "1", Action: "x"}, &got); err != nil {
		t.Fatalf("a success with no data must leave out untouched without error: %v", err)
	}
	if got.Title != "" {
		t.Errorf("out should be untouched, got %+v", got)
	}
}

func TestExecuteCommandIntoRejectsMismatchedOut(t *testing.T) {
	// A decode failure must be reported, not silently leave out zero-valued.
	srv, _ := commandServer(t, ApiResponse{
		OK:   true,
		Data: CommandResult{ID: "1", Success: true, Data: map[string]any{"title": "x"}},
	})
	c := NewClient(srv.URL, "")
	var wrong struct {
		Title int `json:"title"` // string into int — must fail
	}
	if err := c.ExecuteCommandInto(nil, Command{ID: "1", Action: "x"}, &wrong); err == nil {
		t.Error("expected a decode error for a mismatched out type")
	}
}
