package cdp

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNormalizePerformanceSortsAndRounds(t *testing.T) {
	snap := perfSnapshot{
		Navigation: []perfEntry{{Name: "http://h/", EntryType: "navigation", InitiatorType: "navigation", ResponseStatus: 200, Duration: 10.6, Start: 1000.2}},
		Resource: []perfEntry{
			{Name: "http://h/b", InitiatorType: "fetch", ResponseStatus: 200, Duration: 3.4, TransferSize: 300, Start: 1020},
			{Name: "http://h/a", InitiatorType: "fetch", ResponseStatus: 404, Duration: 2, Start: 1010},
		},
	}
	got := normalizePerformance(snap)
	if len(got) != 3 || got[0].URL != "http://h/" || got[1].URL != "http://h/a" || got[2].URL != "http://h/b" {
		t.Fatalf("order = %+v", got)
	}
	if got[0].DurationMs != 11 || got[0].Timestamp != 1000 || got[0].Method != "" {
		t.Errorf("navigation entry = %+v (method must stay unknown)", got[0])
	}
	if got[1].Status != 404 || got[2].TransferSize != 300 || got[2].InitiatorType != "fetch" {
		t.Errorf("resource entries = %+v", got[1:])
	}
}

func feed(t *testing.T, w *NetworkWatcher, method, params string) *NetworkEntry {
	t.Helper()
	e, err := w.handle(method, json.RawMessage(params))
	if err != nil {
		t.Fatalf("%s: %v", method, err)
	}
	return e
}

func TestNetworkWatcherCompletesRequest(t *testing.T) {
	w := newNetworkWatcher(nil)
	if e := feed(t, w, "Network.requestWillBeSent", `{"requestId":"1","request":{"url":"http://h/api","method":"POST"},"timestamp":10,"wallTime":1700000000,"type":"Fetch"}`); e != nil {
		t.Fatalf("request start must not complete: %+v", e)
	}
	feed(t, w, "Network.responseReceived", `{"requestId":"1","timestamp":10.01,"type":"Fetch","response":{"url":"http://h/api","status":404}}`)
	e := feed(t, w, "Network.loadingFinished", `{"requestId":"1","timestamp":10.025,"encodedDataLength":120}`)
	if e == nil || e.Method != "POST" || e.Status != 404 || e.URL != "http://h/api" || e.DurationMs != 25 || e.TransferSize != 120 || e.ResourceType != "Fetch" {
		t.Fatalf("entry = %+v", e)
	}
	if e.Timestamp != 1700000000025 {
		t.Errorf("timestamp = %d", e.Timestamp)
	}
	if len(w.inflight) != 0 {
		t.Errorf("in-flight table not cleared: %v", w.inflight)
	}
}

func TestNetworkWatcherRedirectCompletesPreviousHop(t *testing.T) {
	w := newNetworkWatcher(nil)
	feed(t, w, "Network.requestWillBeSent", `{"requestId":"1","request":{"url":"http://h/old","method":"GET"},"timestamp":1,"wallTime":100}`)
	hop := feed(t, w, "Network.requestWillBeSent", `{"requestId":"1","request":{"url":"http://h/new","method":"GET"},"timestamp":1.5,"wallTime":100.5,"redirectResponse":{"url":"http://h/old","status":302}}`)
	if hop == nil || hop.URL != "http://h/old" || hop.Status != 302 || hop.DurationMs != 500 {
		t.Fatalf("redirect hop = %+v", hop)
	}
	if w.inflight["1"].url != "http://h/new" {
		t.Fatalf("new hop not tracked: %+v", w.inflight["1"])
	}
}

func TestNetworkWatcherRedirectWithoutInflightRecordIsStillRecorded(t *testing.T) {
	// The request began before Network.enable, so the redirected hop has no
	// in-flight record; its redirectResponse must still yield the hop.
	w := newNetworkWatcher(nil)
	hop := feed(t, w, "Network.requestWillBeSent", `{"requestId":"1","request":{"url":"http://h/new","method":"GET"},"timestamp":1.5,"wallTime":100.5,"type":"Document","redirectResponse":{"url":"http://h/old","status":301}}`)
	if hop == nil || hop.URL != "http://h/old" || hop.Status != 301 || hop.ResourceType != "Document" {
		t.Fatalf("pre-attach redirect hop = %+v", hop)
	}
	if w.inflight["1"].url != "http://h/new" {
		t.Fatalf("new hop not tracked: %+v", w.inflight["1"])
	}
}

func TestNetworkWatcherFailedAndPreAttachRequests(t *testing.T) {
	w := newNetworkWatcher(nil)
	fixed := time.UnixMilli(42)
	w.now = func() time.Time { return fixed }

	feed(t, w, "Network.requestWillBeSent", `{"requestId":"f","request":{"url":"http://h/x","method":"GET"},"timestamp":1,"wallTime":100}`)
	failed := feed(t, w, "Network.loadingFailed", `{"requestId":"f","timestamp":1.1,"errorText":"net::ERR_ABORTED","canceled":true}`)
	if failed == nil || failed.Error != "canceled: net::ERR_ABORTED" || failed.Method != "GET" {
		t.Fatalf("failed = %+v", failed)
	}

	// Response seen but request started before Network.enable: method unknown.
	feed(t, w, "Network.responseReceived", `{"requestId":"p","timestamp":2,"response":{"url":"http://h/late","status":200}}`)
	late := feed(t, w, "Network.loadingFinished", `{"requestId":"p","timestamp":2.1}`)
	if late == nil || late.Method != "" || late.Status != 200 || late.DurationMs != 0 || late.Timestamp != 42 {
		t.Fatalf("pre-attach entry = %+v", late)
	}

	// Nothing known but the id: not reported.
	if e := feed(t, w, "Network.loadingFinished", `{"requestId":"ghost","timestamp":3}`); e != nil {
		t.Fatalf("ghost = %+v", e)
	}

	// A failure for a request we never saw start is dropped for the same
	// reason: completing an empty record would emit an entry with no URL and
	// no method.
	if e := feed(t, w, "Network.loadingFailed", `{"requestId":"ghost2","timestamp":4,"errorText":"net::ERR_CONNECTION_REFUSED"}`); e != nil {
		t.Fatalf("failed ghost = %+v", e)
	}

	// But a failure whose URL was learned via responseReceived still reports:
	// the record exists even though requestWillBeSent was never seen.
	feed(t, w, "Network.responseReceived", `{"requestId":"late2","timestamp":5,"response":{"url":"http://h/late2","status":0}}`)
	failedLate := feed(t, w, "Network.loadingFailed", `{"requestId":"late2","timestamp":5.1,"errorText":"net::ERR_TIMED_OUT"}`)
	if failedLate == nil || failedLate.URL != "http://h/late2" || failedLate.Error != "net::ERR_TIMED_OUT" {
		t.Fatalf("failed pre-attach entry = %+v", failedLate)
	}
}

func TestNetworkWatcherRejectsMalformed(t *testing.T) {
	w := newNetworkWatcher(nil)
	if _, err := w.handle("Network.requestWillBeSent", json.RawMessage(`{"request":5}`)); err == nil {
		t.Fatal("want decode error")
	}
}
