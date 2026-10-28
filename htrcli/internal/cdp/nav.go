package cdp

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Navigate loads a URL and waits for Page.loadEventFired, bounded by
// timeoutMs (the global --timeout). SPA route changes never fire load —
// same semantics as the extension transport's navigate wait.
func Navigate(s *Session, url string, timeoutMs int) error {
	if err := s.Call("Page.enable", nil, nil); err != nil {
		return fmt.Errorf("Page.enable: %w", err)
	}
	var nav struct {
		ErrorText string `json:"errorText"`
	}
	if err := s.Call("Page.navigate", map[string]any{"url": url}, &nav); err != nil {
		return err
	}
	if nav.ErrorText != "" {
		return fmt.Errorf("navigation failed: %s", nav.ErrorText)
	}
	if _, err := s.WaitEvent("Page.loadEventFired", time.Duration(timeoutMs)*time.Millisecond); err != nil {
		return fmt.Errorf("page did not finish loading: %w", err)
	}
	return nil
}

// navigationHistory is the subset of Page.getNavigationHistory we consume.
type navigationHistory struct {
	CurrentIndex int `json:"currentIndex"`
	Entries      []struct {
		ID  int    `json:"id"`
		URL string `json:"url"`
	} `json:"entries"`
}

// historyTarget returns the index of the history entry delta steps away from
// current in a history of n entries, or an error when there is no such entry.
func historyTarget(current, n, delta int) (int, error) {
	target := current + delta
	if target >= 0 && target < n {
		return target, nil
	}
	if delta < 0 {
		return 0, errors.New("no history entry to go back to")
	}
	return 0, errors.New("no history entry to go forward to")
}

// HistoryStep moves delta entries through the session's history (-1 back,
// +1 forward), then waits for the destination document to be complete.
func HistoryStep(s *Session, delta, timeoutMs int) error {
	if err := s.Call("Page.enable", nil, nil); err != nil {
		return fmt.Errorf("Page.enable: %w", err)
	}
	var hist navigationHistory
	if err := s.Call("Page.getNavigationHistory", nil, &hist); err != nil {
		return err
	}
	idx, err := historyTarget(hist.CurrentIndex, len(hist.Entries), delta)
	if err != nil {
		return err
	}
	// The starting URL is read just before the step: a destination may rewrite its
	// own URL on load, so "moved off this URL" is the signal that it loaded.
	before, err := readDocumentState(s)
	if err != nil {
		return fmt.Errorf("before history navigation: %w", err)
	}
	if err := s.Call("Page.navigateToHistoryEntry", map[string]any{"entryId": hist.Entries[idx].ID}, nil); err != nil {
		return err
	}
	return waitForHistoryDocument(s, hist.Entries[idx].URL, before.Href, timeoutMs)
}

// documentState is the location and load state of the page's current document.
type documentState struct {
	Href  string `json:"href"`
	State string `json:"state"`
}

// readDocumentState evaluates the current document's location and readyState.
func readDocumentState(s *Session) (documentState, error) {
	raw, err := Evaluate(s, `({href: location.href, state: document.readyState})`)
	if err != nil {
		return documentState{}, fmt.Errorf("reading document state: %w", err)
	}
	var doc documentState
	if err := json.Unmarshal(raw, &doc); err != nil {
		return documentState{}, fmt.Errorf("decoding document state: %w", err)
	}
	return doc, nil
}

// waitForHistoryDocument polls until the document reports readyState "complete"
// and is either at entryURL or has moved off startHref, bounded by timeoutMs. A
// destination that rewrites its URL on load (replaceState with a nonce, a
// server redirect) counts as loaded. Page.loadEventFired is not used: a history
// step onto a page restored from the back-forward cache fires pageshow, not
// load, so an event wait times out on a navigation that succeeded.
func waitForHistoryDocument(s *Session, entryURL, startHref string, timeoutMs int) error {
	deadline := time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
	for {
		doc, err := readDocumentState(s)
		if err != nil {
			return fmt.Errorf("after history navigation: %w", err)
		}
		if doc.State == "complete" && (doc.Href == entryURL || doc.Href != startHref) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("page did not finish loading: wanted %s, at %s (readyState %s; started at %s)", entryURL, doc.Href, doc.State, startHref)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// Screenshot captures the page as PNG.
func Screenshot(s *Session) ([]byte, error) {
	var res struct {
		Data string `json:"data"`
	}
	if err := s.Call("Page.captureScreenshot", map[string]any{"format": "png"}, &res); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(res.Data)
}

// Reload reloads the current document.
//
// Deliberately Page.reload rather than Page.navigate-with-the-same-URL: a
// repeat navigate of a file:// or cached URL can be served as a
// same-document navigation, which would leave in-page JS state alive and
// silently defeat the thing a reload is used to prove (that state survives
// only because it was persisted).
func Reload(s *Session, timeoutMs int) error {
	if err := s.Call("Page.enable", nil, nil); err != nil {
		return fmt.Errorf("Page.enable: %w", err)
	}
	if err := s.Call("Page.reload", map[string]any{}, nil); err != nil {
		return err
	}
	if _, err := s.WaitEvent("Page.loadEventFired", time.Duration(timeoutMs)*time.Millisecond); err != nil {
		return fmt.Errorf("page did not finish reloading: %w", err)
	}
	return nil
}
