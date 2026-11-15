package commands

import (
	"encoding/json"
	"fmt"

	"github.com/u007/htrcli/internal/cdp"
)

// waitRefProbe answers one presence tick for a persistent @eN ref. A missing
// node id is ambiguous (removal or navigation), so the caller pairs it with a
// waitDocumentGuard before reading it as "--gone".
type waitRefProbe func() (cdp.BackendNodeState, error)

// waitDocumentGuard reports why a --gone answer about a ref cannot be trusted
// yet: the page is blank, still loading, or has navigated away from the
// document the guard snapshotted. reason is "" only when the document is
// settled and unchanged.
type waitDocumentGuard func() (reason string, err error)

// newWaitRefProbe translates a persistent @eN ref through the CLI ref store
// once, then binds the per-tick probe to its backendNodeId. The page bundle
// keeps its own in-page ref registry, which only knows refs minted during that
// page's own ExecDOM calls, so a CLI-minted ref handed to it fails on every
// tick with "not known on this page". Resolving here is what makes wait --gone
// work on CDP.
//
// An unknown ref fails immediately: polling an id the store never issued can
// never succeed, and a silent retry loop would hide a bad selector.
func newWaitRefProbe(s *cdp.Session, ref string) (waitRefProbe, error) {
	rs, err := LoadRefStore()
	if err != nil {
		return nil, err
	}
	backendNodeID, ok := rs.Lookup(ref)
	if !ok {
		return nil, fmt.Errorf("wait: stale ref: %s is not known in the CDP ref store (mint one with: htrcli find <selector> --ref)", ref)
	}
	return func() (cdp.BackendNodeState, error) {
		return cdp.BackendNodePresence(s, backendNodeID)
	}, nil
}

// waitDocumentIdentity is the main frame's document state: loaderId is
// Chrome's per-document identity, so comparing a later loaderId with the
// snapshot proves the document did not navigate.
type waitDocumentIdentity struct {
	LoaderID string
	URL      string
}

func readWaitDocumentIdentity(s *cdp.Session) (waitDocumentIdentity, error) {
	var tree struct {
		FrameTree struct {
			Frame struct {
				LoaderID string `json:"loaderId"`
				URL      string `json:"url"`
			} `json:"frame"`
		} `json:"frameTree"`
	}
	if err := s.Call("Page.getFrameTree", nil, &tree); err != nil {
		return waitDocumentIdentity{}, fmt.Errorf("Page.getFrameTree: %w", err)
	}
	id := waitDocumentIdentity{LoaderID: tree.FrameTree.Frame.LoaderID, URL: tree.FrameTree.Frame.URL}
	if id.LoaderID == "" {
		return waitDocumentIdentity{}, fmt.Errorf("Page.getFrameTree: main frame has no loaderId, so a navigation cannot be ruled out")
	}
	return id, nil
}

// newWaitDocumentGuard snapshots the document before the first poll, then
// reports per tick why a missing node id cannot be read as "removed". It
// exists because "No node with given id found" is what BOTH a pruned removal
// and a navigation produce (cdp.isMissingNodeError); only a loaderId that still
// matches the snapshot turns absence into proof of removal. A blank tab and a
// document that is still loading are likewise not proof: an element that has
// not rendered yet is not an element that was removed.
//
// shortcut: the ref store does not record the document a ref was minted in, so
// an id that is already unresolvable when the wait starts cannot be proven to
// have belonged to this document. The wait then fails rather than guessing (a
// node removed and pruned by the DOM agent before the wait began therefore
// times out instead of passing). Persisting the mint-time loaderId in the ref
// store would remove that false failure.
func newWaitDocumentGuard(s *cdp.Session) (waitDocumentGuard, error) {
	start, err := readWaitDocumentIdentity(s)
	if err != nil {
		return nil, err
	}
	return func() (string, error) {
		now, err := readWaitDocumentIdentity(s)
		if err != nil {
			return "", err
		}
		if now.URL == "about:blank" {
			return "the page is about:blank (no page loaded)", nil
		}
		if now.LoaderID != start.LoaderID {
			return "the document navigated", nil
		}
		state, err := readWaitReadyState(s)
		if err != nil {
			// A context that died between the two reads means the document
			// changed under us: report the navigation, not a protocol failure.
			if cdp.IsStaleExecutionContext(err) {
				return "the document navigated", nil
			}
			return "", err
		}
		if state == "loading" {
			return "the document is still loading", nil
		}
		return "", nil
	}, nil
}

func readWaitReadyState(s *cdp.Session) (string, error) {
	raw, err := cdp.Evaluate(s, "document.readyState")
	if err != nil {
		return "", fmt.Errorf("reading document.readyState: %w", err)
	}
	var state string
	if err := json.Unmarshal(raw, &state); err != nil {
		return "", fmt.Errorf("decoding document.readyState %s: %w", raw, err)
	}
	return state, nil
}
