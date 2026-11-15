package cdp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestResolveBackendNodeID(t *testing.T) {
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		switch m.Method {
		case "DOM.enable":
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
		case "DOM.getDocument":
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"root": map[string]any{"nodeId": 1},
			}})
		case "DOM.querySelector":
			var p struct {
				NodeID   int64  `json:"nodeId"`
				Selector string `json:"selector"`
			}
			json.Unmarshal(m.Params, &p)
			if p.NodeID != 1 || !strings.Contains(p.Selector, "#go") {
				t.Errorf("unexpected querySelector params: %s", m.Params)
			}
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{"nodeId": 42}})
		case "DOM.describeNode":
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"node": map[string]any{"backendNodeId": 9007},
			}})
		default:
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
		}
	})

	s, err := Dial(url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer s.Close()

	backendID, err := ResolveBackendNodeID(s, "#go")
	if err != nil {
		t.Fatalf("ResolveBackendNodeID: %v", err)
	}
	if backendID != 9007 {
		t.Fatalf("want backendNodeId 9007, got %d", backendID)
	}
}

func TestResolveBackendNodeIDNotFound(t *testing.T) {
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		switch m.Method {
		case "DOM.getDocument":
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"root": map[string]any{"nodeId": 1},
			}})
		case "DOM.querySelector":
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{"nodeId": 0}})
		default:
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
		}
	})
	s, _ := Dial(url)
	defer s.Close()
	if _, err := ResolveBackendNodeID(s, "#missing"); err == nil {
		t.Fatal("want error for no-match selector, got nil")
	}
}

func TestResolveRefTargets(t *testing.T) {
	var capturedSelector string
	url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
		switch m.Method {
		case "DOM.enable":
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
		case "DOM.getDocument":
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"root": map[string]any{"nodeId": 1},
			}})
		case "DOM.querySelectorAll":
			var p struct {
				Selector string `json:"selector"`
			}
			json.Unmarshal(m.Params, &p)
			capturedSelector = p.Selector
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"nodeIds": []int64{42, 43},
			}})
		case "DOM.describeNode":
			// Return different backendNodeIds depending on the input nodeId.
			var params struct {
				NodeID int64 `json:"nodeId"`
			}
			json.Unmarshal(m.Params, &params)
			backendID := int64(0)
			switch params.NodeID {
			case 42:
				backendID = 9007
			case 43:
				backendID = 9008
			}
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{
				"node": map[string]any{"backendNodeId": backendID},
			}})
		default:
			conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
		}
	})
	s, err := Dial(url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer s.Close()

	ids, err := ResolveRefTargets(s, "button.primary")
	if err != nil {
		t.Fatalf("ResolveRefTargets: %v", err)
	}
	if len(ids) != 2 || ids[0] != 9007 || ids[1] != 9008 {
		t.Fatalf("want [9007 9008], got %v", ids)
	}
	if capturedSelector != "button.primary" {
		t.Fatalf("want selector 'button.primary', got %q", capturedSelector)
	}
}

// BackendNodePresence must read every Chrome phrasing for an id the current
// document cannot resolve as "not resolvable" (absence), not as a protocol
// failure. A navigation answers "does not belong to the document"; a pruned
// node answers "No node with given id found". Anything else is a real error.
func TestBackendNodePresenceUnresolvableVersusProtocolFailure(t *testing.T) {
	for _, tc := range []struct {
		name    string
		message string
		wantErr bool
	}{
		{name: "pruned node", message: "No node with given id found"},
		{name: "could not find node", message: "Could not find node with given id"},
		{name: "navigated document", message: "Node with given id does not belong to the document"},
		{name: "genuine protocol failure", message: "Database is locked", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			url := fakeCDP(t, func(m fakeMsg, conn *websocket.Conn) {
				switch m.Method {
				case "DOM.resolveNode":
					conn.WriteJSON(map[string]any{"id": m.ID, "error": map[string]any{
						"code":    -32000,
						"message": tc.message,
					}})
				default:
					conn.WriteJSON(map[string]any{"id": m.ID, "result": map[string]any{}})
				}
			})
			s, err := Dial(url)
			if err != nil {
				t.Fatalf("dial: %v", err)
			}
			defer s.Close()

			state, err := BackendNodePresence(s, 4242)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("message %q must surface as an error, got %+v", tc.message, state)
				}
				if !strings.Contains(err.Error(), tc.message) {
					t.Fatalf("error %q does not mention %q", err, tc.message)
				}
				return
			}
			if err != nil {
				t.Fatalf("message %q must mean absence, got error %v", tc.message, err)
			}
			if state.Resolvable || state.Attached {
				t.Fatalf("message %q must mean absent, got %+v", tc.message, state)
			}
		})
	}
}
