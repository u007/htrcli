package commands

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/u007/htrcli/internal/api"
	"github.com/u007/htrcli/internal/cdp"
)

func TestDialogListFormatsEntries(t *testing.T) {
	data, _ := json.Marshal(dialogEventData{
		DialogType:     "confirm",
		Message:        "Delete this item?",
		ResolvedAction: "accept",
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("kind") != "dialog" {
			t.Errorf("expected kind=dialog, got %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(api.ApiResponse{
			OK: true,
			Data: api.EventsResponse{
				Entries:            []api.EventEntry{{Seq: 1, Kind: "dialog", Timestamp: 1000, Data: data}},
				OldestAvailableSeq: 1,
			},
		})
	}))
	defer server.Close()

	c := api.NewClient(server.URL, "")
	poller := &EventPoller{Client: c, Kind: dialogEventKind}
	resp, err := poller.Read(0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := formatDialogEntries(resp)
	if !strings.Contains(out, "confirm") || !strings.Contains(out, "Delete this item?") || !strings.Contains(out, "accept") {
		t.Fatalf("expected dialog fields in output, got: %s", out)
	}
}

func TestParseDialogAction(t *testing.T) {
	if _, err := parseDialogAction("accept"); err != nil {
		t.Fatalf("accept should be valid: %v", err)
	}
	if _, err := parseDialogAction("respond"); err != nil {
		t.Fatalf("respond should be valid: %v", err)
	}
	if _, err := parseDialogAction("frobnicate"); err == nil {
		t.Fatalf("expected error for invalid action")
	}
}

// fakeDialogPage serves one CDP page socket; onRequest answers each request.
func fakeDialogPage(t *testing.T, onRequest func(conn *websocket.Conn, id int64, method string, params json.RawMessage)) *cdp.Session {
	t.Helper()
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()
		for {
			var req struct {
				ID     int64           `json:"id"`
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			}
			if err := conn.ReadJSON(&req); err != nil {
				return
			}
			onRequest(conn, req.ID, req.Method, req.Params)
		}
	}))
	t.Cleanup(server.Close)
	s, err := cdp.Dial("ws" + strings.TrimPrefix(server.URL, "http"))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestAnswerDialogCDPAnswersPromptAndReportsIt(t *testing.T) {
	answered := make(chan json.RawMessage, 1)
	s := fakeDialogPage(t, func(conn *websocket.Conn, id int64, method string, params json.RawMessage) {
		switch method {
		case "Page.enable":
			conn.WriteJSON(map[string]any{"id": id, "result": map[string]any{}})
			conn.WriteJSON(map[string]any{"method": "Page.javascriptDialogOpening", "params": map[string]any{
				"type": "prompt", "message": "Name?", "defaultPrompt": "guest",
			}})
		case "Page.handleJavaScriptDialog":
			answered <- params
			conn.WriteJSON(map[string]any{"id": id, "result": map[string]any{}})
		}
	})

	result, err := answerDialogCDP(s, "respond", "Bob", 2000)
	if err != nil {
		t.Fatalf("answerDialogCDP: %v", err)
	}
	want := cdpDialogResult{DialogType: "prompt", Message: "Name?", DefaultPrompt: "guest", ResolvedAction: "respond", RespondedText: "Bob"}
	if result != want {
		t.Fatalf("result = %+v, want %+v", result, want)
	}
	var sent struct {
		Accept     bool   `json:"accept"`
		PromptText string `json:"promptText"`
	}
	if err := json.Unmarshal(<-answered, &sent); err != nil {
		t.Fatalf("decoding Page.handleJavaScriptDialog params: %v", err)
	}
	if !sent.Accept || sent.PromptText != "Bob" {
		t.Fatalf("sent accept=%v promptText=%q, want accept=true promptText=\"Bob\"", sent.Accept, sent.PromptText)
	}
}

func TestAnswerDialogCDPTimesOutWithoutDialog(t *testing.T) {
	s := fakeDialogPage(t, func(conn *websocket.Conn, id int64, method string, params json.RawMessage) {
		conn.WriteJSON(map[string]any{"id": id, "result": map[string]any{}})
	})

	_, err := answerDialogCDP(s, "accept", "", 100)
	if err == nil || !strings.Contains(err.Error(), "no JavaScript dialog opened within 100ms") {
		t.Fatalf("err = %v, want a timeout naming 100ms", err)
	}
}

func TestCDPDialogResultJSONShape(t *testing.T) {
	accepted, err := json.Marshal(cdpDialogResult{DialogType: "confirm", Message: "Delete?", DefaultPrompt: "", ResolvedAction: "accept"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(accepted) != `{"dialogType":"confirm","message":"Delete?","defaultPrompt":"","resolvedAction":"accept"}` {
		t.Fatalf("accept JSON = %s", accepted)
	}
	responded, err := json.Marshal(cdpDialogResult{DialogType: "prompt", Message: "Name?", DefaultPrompt: "guest", ResolvedAction: "respond", RespondedText: "Bob"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(responded) != `{"dialogType":"prompt","message":"Name?","defaultPrompt":"guest","resolvedAction":"respond","respondedText":"Bob"}` {
		t.Fatalf("respond JSON = %s", responded)
	}
}

func TestDialogHandleAcceptsTimeoutOnExtTransport(t *testing.T) {
	resetTransportState()
	defer resetTransportState()
	var gotAction string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req api.CommandRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode command request: %v", err)
		}
		gotAction = req.Command.Action
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(api.ApiResponse{
			OK:   true,
			Data: api.CommandResult{ID: "1", Success: true},
		})
	}))
	defer server.Close()
	client = api.NewClient(server.URL, "")
	defer func() { client = nil }()

	flag := dialogHandleCmd.Flags().Lookup("timeout")
	if err := dialogHandleCmd.Flags().Set("timeout", "5000"); err != nil {
		t.Fatalf("set --timeout: %v", err)
	}
	t.Cleanup(func() { flag.Changed = false })
	dialogHandleAction = "accept"

	if err := dialogHandleCmd.RunE(dialogHandleCmd, nil); err != nil {
		t.Fatalf("--timeout must be accepted on the extension transport: %v", err)
	}
	if gotAction != "dialogPolicy" {
		t.Fatalf("want a dialogPolicy command sent to the daemon, got %q", gotAction)
	}
}
