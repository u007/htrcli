package cdp

import (
	"encoding/json"
	"testing"

	"github.com/gorilla/websocket"
)

func TestSetFileInputFiles(t *testing.T) {
	var gotBackend int64
	var gotFiles []string
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		if m.Method == "DOM.setFileInputFiles" {
			var p struct {
				BackendNodeID int64    `json:"backendNodeId"`
				Files         []string `json:"files"`
			}
			json.Unmarshal(m.Params, &p)
			gotBackend = p.BackendNodeID
			gotFiles = p.Files
		}
		conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
	})
	s, _ := Dial(url)
	defer s.Close()

	if err := SetFileInputFiles(s, 9007, []string{"/tmp/a.png", "/tmp/b.png"}); err != nil {
		t.Fatalf("SetFileInputFiles: %v", err)
	}
	if gotBackend != 9007 {
		t.Fatalf("want backendNodeId 9007, got %d", gotBackend)
	}
	if len(gotFiles) != 2 || gotFiles[0] != "/tmp/a.png" {
		t.Fatalf("unexpected files: %v", gotFiles)
	}
}
