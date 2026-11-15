package cdp

import (
	"fmt"
	"strings"
)

// backendNodeID is CDP's durable per-document element handle. Unlike a
// RemoteObjectId (which expires on GC), a backendNodeId stays valid for the
// life of the document, so htrcli keys persistent refs on it.

// ResolveBackendNodeID resolves a CSS selector to a single backendNodeId via
// DOM.getDocument -> DOM.querySelector -> DOM.describeNode. Only CSS selectors
// are supported on the CDP ref path (DOM.querySelector is CSS-only); callers
// pass the raw CSS string. Returns an error if the selector matches nothing.
func ResolveBackendNodeID(s *Session, cssSelector string) (int64, error) {
	if err := s.Call("DOM.enable", nil, nil); err != nil {
		return 0, fmt.Errorf("DOM.enable: %w", err)
	}
	var doc struct {
		Root struct {
			NodeID int64 `json:"nodeId"`
		} `json:"root"`
	}
	if err := s.Call("DOM.getDocument", map[string]any{"depth": 0}, &doc); err != nil {
		return 0, fmt.Errorf("DOM.getDocument: %w", err)
	}
	var qs struct {
		NodeID int64 `json:"nodeId"`
	}
	if err := s.Call("DOM.querySelector", map[string]any{
		"nodeId":   doc.Root.NodeID,
		"selector": cssSelector,
	}, &qs); err != nil {
		return 0, fmt.Errorf("DOM.querySelector %q: %w", cssSelector, err)
	}
	if qs.NodeID == 0 {
		return 0, fmt.Errorf("no element matched CSS selector %q", cssSelector)
	}
	var desc struct {
		Node struct {
			BackendNodeID int64 `json:"backendNodeId"`
		} `json:"node"`
	}
	if err := s.Call("DOM.describeNode", map[string]any{"nodeId": qs.NodeID}, &desc); err != nil {
		return 0, fmt.Errorf("DOM.describeNode: %w", err)
	}
	return desc.Node.BackendNodeID, nil
}

// ResolveRefTargets resolves a CSS selector to the backendNodeIds of every
// match (findAll --ref) via DOM.querySelectorAll -> DOM.describeNode.
func ResolveRefTargets(s *Session, cssSelector string) ([]int64, error) {
	if err := s.Call("DOM.enable", nil, nil); err != nil {
		return nil, fmt.Errorf("DOM.enable: %w", err)
	}
	var doc struct {
		Root struct {
			NodeID int64 `json:"nodeId"`
		} `json:"root"`
	}
	if err := s.Call("DOM.getDocument", map[string]any{"depth": 0}, &doc); err != nil {
		return nil, fmt.Errorf("DOM.getDocument: %w", err)
	}
	var qs struct {
		NodeIDs []int64 `json:"nodeIds"`
	}
	if err := s.Call("DOM.querySelectorAll", map[string]any{
		"nodeId":   doc.Root.NodeID,
		"selector": cssSelector,
	}, &qs); err != nil {
		return nil, fmt.Errorf("DOM.querySelectorAll %q: %w", cssSelector, err)
	}
	backendIDs := make([]int64, 0, len(qs.NodeIDs))
	for _, nodeID := range qs.NodeIDs {
		var desc struct {
			Node struct {
				BackendNodeID int64 `json:"backendNodeId"`
			} `json:"node"`
		}
		if err := s.Call("DOM.describeNode", map[string]any{"nodeId": nodeID}, &desc); err != nil {
			return nil, fmt.Errorf("DOM.describeNode: %w", err)
		}
		backendIDs = append(backendIDs, desc.Node.BackendNodeID)
	}
	return backendIDs, nil
}

// BackendNodeState is what the protocol can say about a persistent
// backendNodeId without knowing which document minted it.
type BackendNodeState struct {
	// Resolvable is false when the target no longer knows the id. A removed
	// node that the DOM agent has pruned and a document that navigated (every
	// id from the old document is stale) both answer this way, so Resolvable
	// alone cannot distinguish removal from navigation. A caller that needs to
	// (wait --gone) must also prove the document did not change.
	Resolvable bool
	// Attached is true while the resolved node is connected to its document.
	Attached bool
}

// BackendNodePresence resolves backendNodeID and reports whether it is still a
// connected element. A node removed since the ref was minted usually still
// resolves, answering isConnected=false; once the DOM agent has pruned it (and
// always after a navigation) resolution fails with isMissingNodeError, which is
// reported as Resolvable=false. A null object means the same. Only a genuine
// protocol failure is an error.
func BackendNodePresence(s *Session, backendNodeID int64) (BackendNodeState, error) {
	if err := s.Call("DOM.enable", nil, nil); err != nil {
		return BackendNodeState{}, fmt.Errorf("DOM.enable: %w", err)
	}
	var res struct {
		Object struct {
			ObjectID string `json:"objectId"`
			Subtype  string `json:"subtype"`
		} `json:"object"`
	}
	err := s.Call("DOM.resolveNode", map[string]any{"backendNodeId": backendNodeID}, &res)
	if err != nil {
		if isMissingNodeError(err) {
			return BackendNodeState{}, nil
		}
		return BackendNodeState{}, fmt.Errorf("DOM.resolveNode for backendNodeId %d: %w", backendNodeID, err)
	}
	// A null object is a node the target can no longer describe.
	if res.Object.ObjectID == "" || res.Object.Subtype == "null" {
		return BackendNodeState{}, nil
	}
	var call struct {
		Result struct {
			Value bool `json:"value"`
		} `json:"result"`
		ExceptionDetails *struct {
			Text string `json:"text"`
		} `json:"exceptionDetails"`
	}
	if err := s.Call("Runtime.callFunctionOn", map[string]any{
		"objectId":            res.Object.ObjectID,
		"functionDeclaration": "function() { return this.isConnected === true; }",
		"returnByValue":       true,
	}, &call); err != nil {
		return BackendNodeState{}, fmt.Errorf("checking isConnected for backendNodeId %d: %w", backendNodeID, err)
	}
	if call.ExceptionDetails != nil {
		return BackendNodeState{}, fmt.Errorf("checking isConnected for backendNodeId %d: %s", backendNodeID, call.ExceptionDetails.Text)
	}
	return BackendNodeState{Resolvable: true, Attached: call.Result.Value}, nil
}

// IsStaleExecutionContext reports whether err is the CDP answer for an
// execution context that no longer exists (the document navigated before the
// evaluation started). The expression never ran, so a caller may read it as
// "the document changed" instead of a protocol failure.
func IsStaleExecutionContext(err error) bool {
	return isStaleContextErr(err)
}

// isMissingNodeError matches the CDP error for a backendNodeId the target no
// longer knows: the node was removed (and pruned), or the document navigated
// and every id from the old document is stale. BackendNodePresence reports both
// as Resolvable=false; only a caller that checks the document itself can tell
// them apart.
func isMissingNodeError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "No node with given id found") ||
		strings.Contains(msg, "Could not find node with given id")
}
