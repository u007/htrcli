package cdp

import "fmt"

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
