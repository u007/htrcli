package commands

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/u007/htrcli/internal/cdp"
)

// fakeWaitCDP serves one CDP websocket. handle returns the result payload, or
// an error message that is sent back as a CDP error response.
func fakeWaitCDP(t *testing.T, handle func(method string, params json.RawMessage) (result any, errMsg string)) *cdp.Session {
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
			result, errMsg := handle(req.Method, req.Params)
			resp := map[string]any{"id": req.ID}
			if errMsg != "" {
				resp["error"] = map[string]any{"code": -32000, "message": errMsg}
			} else {
				resp["result"] = result
			}
			if err := conn.WriteJSON(resp); err != nil {
				return
			}
		}
	}))
	s, err := cdp.Dial("ws" + strings.TrimPrefix(server.URL, "http"))
	if err != nil {
		server.Close()
		t.Fatalf("dial fake CDP: %v", err)
	}
	t.Cleanup(func() {
		_ = s.Close()
		server.Close()
	})
	return s
}

// withRefStore points LoadRefStore at a temp file holding refs.
func withRefStore(t *testing.T, refs map[string]int64) {
	t.Helper()
	withRefStoreDocuments(t, refs, nil)
}

// withRefStoreDocuments points LoadRefStore at a temp file holding refs and the
// document identity each was minted in.
func withRefStoreDocuments(t *testing.T, refs map[string]int64, loaderIDs map[string]string) {
	t.Helper()
	refStorePathOverride = t.TempDir() + "/refs.json"
	t.Cleanup(func() { refStorePathOverride = "" })
	store, err := LoadRefStore()
	if err != nil {
		t.Fatalf("load ref store: %v", err)
	}
	store.Refs = refs
	store.LoaderIDs = loaderIDs
	if err := store.Save(); err != nil {
		t.Fatalf("save ref store: %v", err)
	}
}

// frameTreeResult is the CDP answer for Page.getFrameTree, the document
// identity the --gone guard snapshots and re-reads.
func frameTreeResult(loaderID, url string) map[string]any {
	return map[string]any{"frameTree": map[string]any{"frame": map[string]any{
		"loaderId": loaderID,
		"url":      url,
	}}}
}

// readyStateResult is the CDP answer for the guard's document.readyState read.
func readyStateResult(state string) map[string]any {
	return map[string]any{"result": map[string]any{"type": "string", "value": state}}
}

// A present CLI-minted ref must succeed, and it must reach the page as the
// store's backendNodeId — never as an @eN string for the page bundle, which
// does not know it.
func TestWaitCDPRefPresentResolvesThroughStore(t *testing.T) {
	withRefStore(t, map[string]int64{"@e1": 4242})
	var resolved []int64
	bundleEvals := 0
	s := fakeWaitCDP(t, func(method string, params json.RawMessage) (any, string) {
		switch method {
		case "DOM.resolveNode":
			var p struct {
				BackendNodeID int64 `json:"backendNodeId"`
			}
			if err := json.Unmarshal(params, &p); err != nil {
				t.Errorf("decode DOM.resolveNode params: %v", err)
			}
			resolved = append(resolved, p.BackendNodeID)
			return map[string]any{"object": map[string]any{"objectId": "obj-4242"}}, ""
		case "Runtime.callFunctionOn":
			return map[string]any{"result": map[string]any{"type": "boolean", "value": true}}, ""
		case "Runtime.evaluate":
			bundleEvals++
			return map[string]any{"result": map[string]any{"value": map[string]any{"__htrcliNeedBundle": true}}}, ""
		}
		return map[string]any{}, ""
	})

	if err := waitCDPSelectorLoop(s, parseSelector("@e1"), false, 1000, 10); err != nil {
		t.Fatalf("wait @e1: %v", err)
	}
	if len(resolved) != 1 || resolved[0] != 4242 {
		t.Fatalf("DOM.resolveNode backendNodeIds = %v, want [4242] from the ref store", resolved)
	}
	if bundleEvals != 0 {
		t.Fatalf("a CLI-minted ref must not be sent to the page bundle (saw %d Runtime.evaluate calls)", bundleEvals)
	}
}

// wait --gone succeeds when the store's node was removed from a settled,
// unchanged document. A missing id is also what a navigation produces, so the
// wait must have observed the node connected and the guard must confirm the
// loaderId still matches and the document is settled before absence counts as
// removal. The other protocol shape means the same thing: the id resolves to a
// node that is no longer connected. Appear on the same ref is "absent", i.e. a
// timeout — never a repeated resolution error.
func TestWaitCDPRefGoneWhenNodeIsGone(t *testing.T) {
	t.Run("pruned after the wait observed the node", func(t *testing.T) {
		// The node was connected on the first poll and the DOM agent pruned its
		// id on the next: removal inside the same document, which is the "its
		// node is gone" half of the gone condition.
		withRefStore(t, map[string]int64{"@e1": 77})
		resolves := 0
		frameTrees := 0
		bundleEvals := 0
		s := fakeWaitCDP(t, func(method string, params json.RawMessage) (any, string) {
			switch method {
			case "Page.getFrameTree":
				frameTrees++
				return frameTreeResult("loader-1", "https://example.test/list"), ""
			case "DOM.resolveNode":
				resolves++
				if resolves == 1 {
					return map[string]any{"object": map[string]any{"objectId": "obj-77"}}, ""
				}
				return nil, "No node with given id found"
			case "Runtime.callFunctionOn":
				return map[string]any{"result": map[string]any{"type": "boolean", "value": true}}, ""
			case "Runtime.evaluate":
				var p struct {
					Expression string `json:"expression"`
				}
				if err := json.Unmarshal(params, &p); err != nil {
					t.Errorf("decode Runtime.evaluate params: %v", err)
				}
				if p.Expression != "document.readyState" {
					bundleEvals++
					return map[string]any{"result": map[string]any{"value": map[string]any{"__htrcliNeedBundle": true}}}, ""
				}
				return readyStateResult("complete"), ""
			}
			return map[string]any{}, ""
		})
		if err := waitCDPSelectorLoop(s, parseSelector("@e1"), true, 1000, 10); err != nil {
			t.Fatalf("wait @e1 --gone: %v", err)
		}
		if resolves != 2 {
			t.Fatalf("DOM.resolveNode calls = %d, want 2 (attached, then pruned)", resolves)
		}
		if frameTrees != 2 {
			t.Fatalf("Page.getFrameTree calls = %d, want 2 (snapshot + check after the missing id)", frameTrees)
		}
		if bundleEvals != 0 {
			t.Fatalf("--gone must not fall back to the page bundle (saw %d non-readiness Runtime.evaluate calls)", bundleEvals)
		}
	})

	t.Run("resolves but is detached", func(t *testing.T) {
		withRefStore(t, map[string]int64{"@e1": 88})
		s := fakeWaitCDP(t, func(method string, params json.RawMessage) (any, string) {
			switch method {
			case "Page.getFrameTree":
				return frameTreeResult("loader-1", "https://example.test/list"), ""
			case "Runtime.evaluate":
				return readyStateResult("complete"), ""
			case "DOM.resolveNode":
				return map[string]any{"object": map[string]any{"objectId": "obj-88"}}, ""
			case "Runtime.callFunctionOn":
				return map[string]any{"result": map[string]any{"type": "boolean", "value": false}}, ""
			}
			return map[string]any{}, ""
		})
		if err := waitCDPSelectorLoop(s, parseSelector("@e1"), true, 1000, 10); err != nil {
			t.Fatalf("wait @e1 --gone: %v", err)
		}
		err := waitCDPSelectorLoop(s, parseSelector("@e1"), false, 40, 5)
		if err == nil || !strings.Contains(err.Error(), "is not resolvable") {
			t.Fatalf("appear on a detached ref = %v, want a clear timeout", err)
		}
	})
}

// A blank tab has no document to be gone from: a missing id there is not
// removal, so --gone must never succeed on about:blank.
func TestWaitCDPRefGoneBlankPageDoesNotSucceed(t *testing.T) {
	withRefStore(t, map[string]int64{"@e1": 55})
	resolves := 0
	s := fakeWaitCDP(t, func(method string, params json.RawMessage) (any, string) {
		switch method {
		case "Page.getFrameTree":
			return frameTreeResult("blank-loader", "about:blank"), ""
		case "DOM.resolveNode":
			resolves++
			return nil, "No node with given id found"
		}
		return map[string]any{}, ""
	})
	err := waitCDPSelectorLoop(s, parseSelector("@e1"), true, 60, 5)
	if err == nil {
		t.Fatal("wait @e1 --gone on about:blank: want a failure, got success")
	}
	for _, want := range []string{"@e1", "not confirmed gone", "about:blank"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "still present") {
		t.Fatalf("blank page must not be reported as %q: %v", "still present", err)
	}
	if resolves == 0 {
		t.Fatal("the ref must still be probed each tick; the document check decides, not a skipped probe")
	}
}

// A navigation invalidates every id from the old document, which looks exactly
// like removal. When the ref store does NOT record the minting document (a ref
// written by an older build), --gone must stay unsettled and time out naming
// the navigation — never guess "gone".
func TestWaitCDPRefGoneAfterNavigationWithUnknownIdentityDoesNotSucceed(t *testing.T) {
	withRefStore(t, map[string]int64{"@e1": 66})
	frameTrees := 0
	resolves := 0
	s := fakeWaitCDP(t, func(method string, params json.RawMessage) (any, string) {
		switch method {
		case "Page.getFrameTree":
			frameTrees++
			if frameTrees == 1 {
				return frameTreeResult("loader-1", "https://example.test/list"), ""
			}
			return frameTreeResult("loader-2", "https://example.test/other"), ""
		case "Runtime.evaluate":
			return readyStateResult("complete"), ""
		case "DOM.resolveNode":
			resolves++
			return nil, "No node with given id found"
		}
		return map[string]any{}, ""
	})
	err := waitCDPSelectorLoop(s, parseSelector("@e1"), true, 60, 5)
	if err == nil {
		t.Fatal("wait @e1 --gone after a navigation: want a failure, got success")
	}
	for _, want := range []string{"@e1", "not confirmed gone", "navigated"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "still present") {
		t.Fatalf("a navigation must not be reported as %q: %v", "still present", err)
	}
	if resolves == 0 {
		t.Fatal("the ref must still be probed each tick; the document check decides, not a skipped probe")
	}
}

// A ref that is already unresolvable when the wait starts, whose minting
// document is unknown (a ref written by an older build), is not proof the
// element was removed: it may have been minted in a document that has since
// navigated. The wait must fail with a re-mint instruction instead of reporting
// gone.
func TestWaitCDPRefGoneUnresolvableAtStartWithUnknownIdentityDoesNotSucceed(t *testing.T) {
	withRefStore(t, map[string]int64{"@e1": 31})
	frameTrees := 0
	s := fakeWaitCDP(t, func(method string, params json.RawMessage) (any, string) {
		switch method {
		case "Page.getFrameTree":
			frameTrees++
			return frameTreeResult("loader-1", "https://example.test/other"), ""
		case "Runtime.evaluate":
			return readyStateResult("complete"), ""
		case "DOM.resolveNode":
			return nil, "No node with given id found"
		}
		return map[string]any{}, ""
	})
	err := waitCDPSelectorLoop(s, parseSelector("@e1"), true, 60, 5)
	if err == nil {
		t.Fatal("wait @e1 --gone on an id that never resolved: want a failure, got success")
	}
	for _, want := range []string{"@e1", "not confirmed gone", "navigation cannot be ruled out"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "still present") {
		t.Fatalf("an unresolvable ref must not be reported as %q: %v", "still present", err)
	}
	if frameTrees < 2 {
		t.Fatalf("Page.getFrameTree calls = %d, want the document checked while polling", frameTrees)
	}
}

// A document that has not finished loading has not rendered its elements, so an
// absent id is not proof of removal — whether the id is missing or resolves to a
// detached node. --gone must stay unsettled until the deadline.
func TestWaitCDPRefGoneLoadingDocumentDoesNotSucceed(t *testing.T) {
	for _, tc := range []struct {
		name     string
		resolves bool
	}{
		{name: "node id missing"},
		{name: "node resolves detached", resolves: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withRefStore(t, map[string]int64{"@e1": 44})
			s := fakeWaitCDP(t, func(method string, params json.RawMessage) (any, string) {
				switch method {
				case "Page.getFrameTree":
					return frameTreeResult("loader-1", "https://example.test/list"), ""
				case "Runtime.evaluate":
					return readyStateResult("loading"), ""
				case "DOM.resolveNode":
					if tc.resolves {
						return map[string]any{"object": map[string]any{"objectId": "obj-44"}}, ""
					}
					return nil, "No node with given id found"
				case "Runtime.callFunctionOn":
					return map[string]any{"result": map[string]any{"type": "boolean", "value": false}}, ""
				}
				return map[string]any{}, ""
			})
			err := waitCDPSelectorLoop(s, parseSelector("@e1"), true, 60, 5)
			if err == nil {
				t.Fatalf("wait @e1 --gone on a loading document (%s): want a failure, got success", tc.name)
			}
			for _, want := range []string{"@e1", "not confirmed gone", "still loading"} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error %q missing %q", err, want)
				}
			}
			if strings.Contains(err.Error(), "still present") {
				t.Fatalf("a loading document must not be reported as %q: %v", "still present", err)
			}
		})
	}
}

// An id the store never issued can never be gone either: fail at once, naming
// the ref, before the document guard or the page is touched.
func TestWaitCDPRefGoneUnknownFailsImmediately(t *testing.T) {
	withRefStore(t, map[string]int64{})
	calls := 0
	s := fakeWaitCDP(t, func(method string, params json.RawMessage) (any, string) {
		calls++
		return map[string]any{}, ""
	})
	err := waitCDPSelectorLoop(s, parseSelector("@e999"), true, 5000, 10)
	if err == nil {
		t.Fatal("wait @e999 --gone: want an error")
	}
	for _, want := range []string{"@e999", "ref store"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err, want)
		}
	}
	if calls != 0 {
		t.Fatalf("unknown ref must fail before touching the page, saw %d CDP calls", calls)
	}
}

// An id the store never issued can never appear: fail at once, naming the ref,
// before touching the page. A silent retry loop hides the operator's mistake.
func TestWaitCDPRefUnknownFailsImmediately(t *testing.T) {
	withRefStore(t, map[string]int64{})
	calls := 0
	s := fakeWaitCDP(t, func(method string, params json.RawMessage) (any, string) {
		calls++
		return map[string]any{}, ""
	})
	start := time.Now()
	err := waitCDPSelectorLoop(s, parseSelector("@e999"), false, 5000, 10)
	if err == nil {
		t.Fatal("wait @e999: want an error")
	}
	for _, want := range []string{"@e999", "ref store"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err, want)
		}
	}
	if calls != 0 {
		t.Fatalf("unknown ref must fail before touching the page, saw %d CDP calls", calls)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("unknown ref took %s: it must fail immediately, not poll", elapsed)
	}
}

// A ref minted, then removed and pruned by the DOM agent before the wait
// starts, is genuinely gone: the store's mint-time loaderId still matches the
// current document, so the absence proves removal instead of looking like a
// navigation. This is the case that used to false-fail.
func TestWaitCDPRefGoneSameDocumentPruneSucceeds(t *testing.T) {
	withRefStoreDocuments(t, map[string]int64{"@e1": 31}, map[string]string{"@e1": "loader-1"})
	resolves := 0
	s := fakeWaitCDP(t, func(method string, params json.RawMessage) (any, string) {
		switch method {
		case "Page.getFrameTree":
			return frameTreeResult("loader-1", "https://example.test/list"), ""
		case "Runtime.evaluate":
			return readyStateResult("complete"), ""
		case "DOM.resolveNode":
			resolves++
			return nil, "No node with given id found"
		}
		return map[string]any{}, ""
	})
	if err := waitCDPSelectorLoop(s, parseSelector("@e1"), true, 1000, 10); err != nil {
		t.Fatalf("wait @e1 --gone after a same-document prune: %v", err)
	}
	if resolves == 0 {
		t.Fatal("the ref must be probed; the mint-time loaderId decides, not a skipped probe")
	}
}

// A ref minted in a document that has since been replaced by a settled
// navigation: the node is gone because the document holding it is no longer
// displayed. --gone must succeed, and nothing may describe it as "still
// present".
func TestWaitCDPRefGoneCrossDocumentSucceeds(t *testing.T) {
	withRefStoreDocuments(t, map[string]int64{"@e1": 62}, map[string]string{"@e1": "loader-1"})
	s := fakeWaitCDP(t, func(method string, params json.RawMessage) (any, string) {
		switch method {
		case "Page.getFrameTree":
			return frameTreeResult("loader-2", "https://example.test/other"), ""
		case "Runtime.evaluate":
			return readyStateResult("complete"), ""
		case "DOM.resolveNode":
			return nil, "No node with given id found"
		}
		return map[string]any{}, ""
	})
	var err error
	out := captureStdout(t, func() {
		err = waitCDPSelectorLoop(s, parseSelector("@e1"), true, 1000, 10)
	})
	if err != nil {
		t.Fatalf("wait @e1 --gone after a cross-document navigation: %v", err)
	}
	if strings.Contains(out, "still present") {
		t.Fatalf("cross-document gone must never read as %q: %q", "still present", out)
	}
}

// Blank is blank even when the ref records the document it was minted in: there
// is no page to be gone from, so --gone must not succeed.
func TestWaitCDPRefGoneBlankPageWithKnownDocumentDoesNotSucceed(t *testing.T) {
	withRefStoreDocuments(t, map[string]int64{"@e1": 55}, map[string]string{"@e1": "loader-1"})
	resolves := 0
	s := fakeWaitCDP(t, func(method string, params json.RawMessage) (any, string) {
		switch method {
		case "Page.getFrameTree":
			return frameTreeResult("blank-loader", "about:blank"), ""
		case "DOM.resolveNode":
			resolves++
			return nil, "No node with given id found"
		}
		return map[string]any{}, ""
	})
	err := waitCDPSelectorLoop(s, parseSelector("@e1"), true, 60, 5)
	if err == nil {
		t.Fatal("wait @e1 --gone on about:blank: want a failure, got success")
	}
	for _, want := range []string{"@e1", "not confirmed gone", "about:blank"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "still present") {
		t.Fatalf("blank page must not be reported as %q: %v", "still present", err)
	}
	if resolves == 0 {
		t.Fatal("the ref must still be probed each tick")
	}
}

// A still-loading document proves nothing yet, whether it is the document the
// ref was minted in or a new one reached by navigation. A cross-document answer
// must not turn a half-rendered page into "gone".
func TestWaitCDPRefGoneLoadingDocumentWithKnownDocumentDoesNotSucceed(t *testing.T) {
	for _, tc := range []struct {
		name      string
		wantFrame string
	}{
		{name: "same document loading", wantFrame: "loader-1"},
		{name: "navigated document loading", wantFrame: "loader-2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withRefStoreDocuments(t, map[string]int64{"@e1": 44}, map[string]string{"@e1": "loader-1"})
			s := fakeWaitCDP(t, func(method string, params json.RawMessage) (any, string) {
				switch method {
				case "Page.getFrameTree":
					return frameTreeResult(tc.wantFrame, "https://example.test/list"), ""
				case "Runtime.evaluate":
					return readyStateResult("loading"), ""
				case "DOM.resolveNode":
					return nil, "No node with given id found"
				}
				return map[string]any{}, ""
			})
			err := waitCDPSelectorLoop(s, parseSelector("@e1"), true, 60, 5)
			if err == nil {
				t.Fatalf("wait @e1 --gone on a loading document (%s): want a failure, got success", tc.name)
			}
			for _, want := range []string{"@e1", "not confirmed gone", "still loading"} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error %q missing %q", err, want)
				}
			}
			if strings.Contains(err.Error(), "still present") {
				t.Fatalf("a loading document must not be reported as %q: %v", "still present", err)
			}
		})
	}
}

// A refs.json written by the previous build has no "loaderIds" key. It must
// still load, and with an unknown mint-time identity the wait keeps the old
// conservative answer: no false success on a node that is already unresolvable.
func TestWaitCDPRefGoneOldFormatStoreKeepsConservativeBehaviour(t *testing.T) {
	refStorePathOverride = t.TempDir() + "/refs.json"
	t.Cleanup(func() { refStorePathOverride = "" })
	oldFormat := `{
  "nextRef": 1,
  "refs": {
    "@e1": 31
  }
}`
	if err := os.WriteFile(refStorePathOverride, []byte(oldFormat), 0o600); err != nil {
		t.Fatalf("write old-format store: %v", err)
	}
	s := fakeWaitCDP(t, func(method string, params json.RawMessage) (any, string) {
		switch method {
		case "Page.getFrameTree":
			return frameTreeResult("loader-1", "https://example.test/list"), ""
		case "Runtime.evaluate":
			return readyStateResult("complete"), ""
		case "DOM.resolveNode":
			return nil, "No node with given id found"
		}
		return map[string]any{}, ""
	})
	err := waitCDPSelectorLoop(s, parseSelector("@e1"), true, 60, 5)
	if err == nil {
		t.Fatal("wait @e1 --gone on an old-format ref: want a failure, got success")
	}
	for _, want := range []string{"@e1", "not confirmed gone", "navigation cannot be ruled out"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "still present") {
		t.Fatalf("an old-format ref must not be reported as %q: %v", "still present", err)
	}
}
