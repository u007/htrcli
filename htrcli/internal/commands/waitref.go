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

// waitDocumentState is one tick's view of the main frame.
type waitDocumentState struct {
	// LoaderID is the current document's identity.
	LoaderID string
	// Blank is true while the tab shows about:blank.
	Blank bool
	// Loading is true while document.readyState is "loading".
	Loading bool
	// Navigated is true when LoaderID differs from the snapshot taken when the
	// wait started. It is only consulted for refs whose mint-time identity is
	// unknown (a ref store written before LoaderIDs existed).
	Navigated bool
}

// waitDocumentGuard reports the main frame's document state for one tick, so a
// --gone answer can tell a pruned node from a navigation. reason is no longer a
// string: the caller compares LoaderID with the ref's mint-time identity and
// treats a blank or still-loading page as unsettled in either case.
type waitDocumentGuard func() (waitDocumentState, error)

// newWaitRefProbe translates a persistent @eN ref through the CLI ref store
// once, then binds the per-tick probe to its backendNodeId. The page bundle
// keeps its own in-page ref registry, which only knows refs minted during that
// page's own ExecDOM calls, so a CLI-minted ref handed to it fails on every
// tick with "not known on this page". Resolving here is what makes wait --gone
// work on CDP.
//
// It also returns the loaderId of the document the ref was minted in, or "" for
// a ref written before the store recorded document identities.
//
// An unknown ref fails immediately: polling an id the store never issued can
// never succeed, and a silent retry loop would hide a bad selector.
func newWaitRefProbe(s *cdp.Session, ref string) (waitRefProbe, string, error) {
	rs, err := LoadRefStore()
	if err != nil {
		return nil, "", err
	}
	backendNodeID, ok := rs.Lookup(ref)
	if !ok {
		return nil, "", fmt.Errorf("wait: stale ref: %s is not known in the CDP ref store (mint one with: htrcli find <selector> --ref)", ref)
	}
	mintedLoaderID, _ := rs.LookupDocument(ref)
	return func() (cdp.BackendNodeState, error) {
		return cdp.BackendNodePresence(s, backendNodeID)
	}, mintedLoaderID, nil
}

// mainFrameIdentity is the main frame's document state: loaderId is Chrome's
// per-document identity, so comparing a later loaderId with a mint-time or
// wait-start snapshot proves the document did (or did not) change.
type mainFrameIdentity struct {
	LoaderID string
	URL      string
}

func readMainFrameIdentity(s *cdp.Session) (mainFrameIdentity, error) {
	var tree struct {
		FrameTree struct {
			Frame struct {
				LoaderID string `json:"loaderId"`
				URL      string `json:"url"`
			} `json:"frame"`
		} `json:"frameTree"`
	}
	if err := s.Call("Page.getFrameTree", nil, &tree); err != nil {
		return mainFrameIdentity{}, fmt.Errorf("Page.getFrameTree: %w", err)
	}
	id := mainFrameIdentity{LoaderID: tree.FrameTree.Frame.LoaderID, URL: tree.FrameTree.Frame.URL}
	if id.LoaderID == "" {
		return mainFrameIdentity{}, fmt.Errorf("Page.getFrameTree: main frame has no loaderId, so a navigation cannot be ruled out")
	}
	return id, nil
}

// newWaitDocumentGuard snapshots the document before the first poll, then
// reports per tick whether the page is blank, still loading, and (for refs with
// no recorded mint-time identity) whether it navigated. It exists because "No
// node with given id found" is what BOTH a pruned removal and a navigation
// produce (cdp.isMissingNodeError); only the document identity turns absence
// into proof of removal.
func newWaitDocumentGuard(s *cdp.Session) (waitDocumentGuard, error) {
	start, err := readMainFrameIdentity(s)
	if err != nil {
		return nil, err
	}
	return func() (waitDocumentState, error) {
		now, err := readMainFrameIdentity(s)
		if err != nil {
			return waitDocumentState{}, err
		}
		if now.URL == "about:blank" {
			return waitDocumentState{LoaderID: now.LoaderID, Blank: true}, nil
		}
		state, err := readWaitReadyState(s)
		if err != nil {
			// A context that died between the two reads means the document
			// changed under us: report it as unsettled, not a protocol failure.
			if cdp.IsStaleExecutionContext(err) {
				return waitDocumentState{
					LoaderID:  now.LoaderID,
					Loading:   true,
					Navigated: now.LoaderID != start.LoaderID,
				}, nil
			}
			return waitDocumentState{}, err
		}
		return waitDocumentState{
			LoaderID:  now.LoaderID,
			Loading:   state == "loading",
			Navigated: now.LoaderID != start.LoaderID,
		}, nil
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
