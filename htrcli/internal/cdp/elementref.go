package cdp

import (
	"fmt"
	"math"
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

// ResolveBackendNodeCoordinates resolves a persistent backend node to the
// center of its visible content box in viewport CSS pixels. DOM.getBoxModel
// reports quads in document coordinates, so the current visual viewport page
// offset is subtracted before the coordinates are passed to Input events.
func ResolveBackendNodeCoordinates(s *Session, backendNodeID int64) (float64, float64, error) {
	if backendNodeID <= 0 {
		return 0, 0, fmt.Errorf("invalid backendNodeId %d", backendNodeID)
	}
	if err := s.Call("DOM.enable", nil, nil); err != nil {
		return 0, 0, fmt.Errorf("DOM.enable: %w", err)
	}

	var box struct {
		Model struct {
			Content []float64 `json:"content"`
		} `json:"model"`
	}
	if err := s.Call("DOM.getBoxModel", map[string]any{
		"backendNodeId": backendNodeID,
	}, &box); err != nil {
		return 0, 0, fmt.Errorf("DOM.getBoxModel for backendNodeId %d: %w", backendNodeID, err)
	}
	if len(box.Model.Content) < 8 || len(box.Model.Content)%2 != 0 {
		return 0, 0, fmt.Errorf("DOM.getBoxModel for backendNodeId %d returned an invalid content quad", backendNodeID)
	}

	var metrics struct {
		LayoutViewport struct {
			PageX *float64 `json:"pageX"`
			PageY *float64 `json:"pageY"`
		} `json:"layoutViewport"`
		VisualViewport struct {
			PageX *float64 `json:"pageX"`
			PageY *float64 `json:"pageY"`
		} `json:"visualViewport"`
	}
	if err := s.Call("Page.getLayoutMetrics", nil, &metrics); err != nil {
		return 0, 0, fmt.Errorf("Page.getLayoutMetrics: %w", err)
	}
	pageX, pageY := 0.0, 0.0
	if metrics.VisualViewport.PageX != nil {
		pageX = *metrics.VisualViewport.PageX
	} else if metrics.LayoutViewport.PageX != nil {
		pageX = *metrics.LayoutViewport.PageX
	}
	if metrics.VisualViewport.PageY != nil {
		pageY = *metrics.VisualViewport.PageY
	} else if metrics.LayoutViewport.PageY != nil {
		pageY = *metrics.LayoutViewport.PageY
	}

	var centerX, centerY float64
	for i := 0; i < len(box.Model.Content); i += 2 {
		centerX += box.Model.Content[i]
		centerY += box.Model.Content[i+1]
	}
	vertices := float64(len(box.Model.Content) / 2)
	x := centerX/vertices - pageX
	y := centerY/vertices - pageY
	if math.IsNaN(x) || math.IsNaN(y) || math.IsInf(x, 0) || math.IsInf(y, 0) {
		return 0, 0, fmt.Errorf("DOM.getBoxModel for backendNodeId %d returned non-finite coordinates", backendNodeID)
	}
	return x, y, nil
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
