package cdp

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"
)

// NetworkEntry is one completed request seen over CDP. Its JSON tags are a
// superset of the extension transport's network event data
// (commands.networkEventData), so the shared formatter and the --url/--status
// matcher work unchanged. An empty Method means "unknown" (the Performance API
// does not expose it). Timestamp is epoch milliseconds and travels on the
// event envelope, not in the data.
type NetworkEntry struct {
	RequestID     string `json:"requestId,omitempty"`
	URL           string `json:"url"`
	Method        string `json:"method"`
	Status        int    `json:"status,omitempty"`
	DurationMs    int    `json:"durationMs,omitempty"`
	InitiatorType string `json:"initiatorType,omitempty"` // Performance API vocabulary (fetch, script, …)
	ResourceType  string `json:"resourceType,omitempty"`  // Network-domain vocabulary (Fetch, XHR, Document, …)
	TransferSize  int64  `json:"transferSize,omitempty"`
	Error         string `json:"error,omitempty"`
	Timestamp     int64  `json:"-"`
}

// DefaultResourceTimingBufferSize is Chrome's default resource-timing buffer
// (performance.setResourceTimingBufferSize). Once full, later resource entries
// are dropped by the browser.
const DefaultResourceTimingBufferSize = 250

// performanceExpr lists the navigation + resource timing entries of the
// current document. Fields are shipped exactly as the Performance API
// reports them: responseStatus (Chrome 109+) and transferSize are 0 by
// spec for cross-origin resources without Timing-Allow-Origin, and a field
// the browser does not implement is simply absent from the JSON — either
// way the Go side sees its zero value, which NetworkEntry's omitempty tags
// render as "unknown" rather than inventing a default here.
const performanceExpr = `(() => {
  const origin = performance.timeOrigin;
  const pick = (e) => ({
    name: e.name,
    entryType: e.entryType,
    initiatorType: e.initiatorType,
    responseStatus: e.responseStatus,
    duration: e.duration,
    transferSize: e.transferSize,
    start: origin + e.startTime,
  });
  return {
    navigation: performance.getEntriesByType("navigation").map(pick),
    resource: performance.getEntriesByType("resource").map(pick),
  };
})()`

type perfEntry struct {
	Name           string  `json:"name"`
	EntryType      string  `json:"entryType"`
	InitiatorType  string  `json:"initiatorType"`
	ResponseStatus int     `json:"responseStatus"`
	Duration       float64 `json:"duration"`
	TransferSize   int64   `json:"transferSize"`
	Start          float64 `json:"start"`
}

type perfSnapshot struct {
	Navigation []perfEntry `json:"navigation"`
	Resource   []perfEntry `json:"resource"`
}

// ReadPerformanceEntries returns the requests the current document has already
// completed, from the page's Performance API (Network.enable cannot replay
// past requests). Method and request/response bodies are not available this
// way. bufferMaybeFull reports that the resource list reached Chrome's default
// buffer size, so later requests may be missing.
func ReadPerformanceEntries(s *Session) (entries []NetworkEntry, bufferMaybeFull bool, err error) {
	res, err := evaluate(s, performanceExpr, false)
	if err != nil {
		return nil, false, fmt.Errorf("reading performance entries: %w", err)
	}
	var snap perfSnapshot
	if err := json.Unmarshal(res.Result.Value, &snap); err != nil {
		return nil, false, fmt.Errorf("decoding performance entries: %w", err)
	}
	return normalizePerformance(snap), len(snap.Resource) >= DefaultResourceTimingBufferSize, nil
}

// normalizePerformance converts timing entries to NetworkEntry, sorted by
// start time (navigation first on ties).
func normalizePerformance(snap perfSnapshot) []NetworkEntry {
	all := append(append([]perfEntry{}, snap.Navigation...), snap.Resource...)
	out := make([]NetworkEntry, 0, len(all))
	for _, e := range all {
		out = append(out, NetworkEntry{
			URL:           e.Name,
			Status:        e.ResponseStatus,
			DurationMs:    int(math.Round(e.Duration)),
			InitiatorType: e.InitiatorType,
			TransferSize:  e.TransferSize,
			Timestamp:     int64(math.Round(e.Start)),
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Timestamp < out[j].Timestamp })
	return out
}

// NetworkWatcher correlates Network-domain events into completed requests for
// the lifetime of one CDP session. Requests that completed before the watcher
// attached are never seen.
type NetworkWatcher struct {
	s        *Session
	inflight map[string]*inflightRequest
	now      func() time.Time
}

type inflightRequest struct {
	url, method   string
	status        int
	resourceType  string
	start         float64 // CDP monotonic seconds; 0 = unknown
	wallTime      float64 // epoch seconds at start; 0 = unknown
	startObserved bool
}

// WatchNetwork enables the Network domain on s.
func WatchNetwork(s *Session) (*NetworkWatcher, error) {
	if err := s.Call("Network.enable", nil, nil); err != nil {
		return nil, fmt.Errorf("Network.enable: %w", err)
	}
	return newNetworkWatcher(s), nil
}

func newNetworkWatcher(s *Session) *NetworkWatcher {
	return &NetworkWatcher{s: s, inflight: map[string]*inflightRequest{}, now: time.Now}
}

// Next blocks until a request completes (finished or failed), or returns
// ErrEventDeadline when deadline passes (the session is then unusable — see
// Session.NextEvent).
func (w *NetworkWatcher) Next(deadline time.Time) (*NetworkEntry, error) {
	for {
		method, params, err := w.s.NextEvent(deadline)
		if err != nil {
			return nil, err
		}
		entry, err := w.handle(method, params)
		if err != nil {
			return nil, err
		}
		if entry != nil {
			return entry, nil
		}
	}
}

type cdpResponse struct {
	URL    string `json:"url"`
	Status int    `json:"status"`
}

type requestWillBeSentParams struct {
	RequestID string `json:"requestId"`
	Request   struct {
		URL    string `json:"url"`
		Method string `json:"method"`
	} `json:"request"`
	Timestamp        float64      `json:"timestamp"`
	WallTime         float64      `json:"wallTime"`
	Type             string       `json:"type"`
	RedirectResponse *cdpResponse `json:"redirectResponse"`
}

type responseReceivedParams struct {
	RequestID string      `json:"requestId"`
	Timestamp float64     `json:"timestamp"`
	Type      string      `json:"type"`
	Response  cdpResponse `json:"response"`
}

type loadingFinishedParams struct {
	RequestID         string  `json:"requestId"`
	Timestamp         float64 `json:"timestamp"`
	EncodedDataLength float64 `json:"encodedDataLength"`
}

type loadingFailedParams struct {
	RequestID string  `json:"requestId"`
	Timestamp float64 `json:"timestamp"`
	Type      string  `json:"type"`
	ErrorText string  `json:"errorText"`
	Canceled  bool    `json:"canceled"`
}

// handle folds one Network event into the in-flight table and returns the
// request it completed, if any. Redirect hops complete the previous hop.
func (w *NetworkWatcher) handle(method string, params json.RawMessage) (*NetworkEntry, error) {
	switch method {
	case "Network.requestWillBeSent":
		var p requestWillBeSentParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("decoding %s: %w", method, err)
		}
		var done *NetworkEntry
		if p.RedirectResponse != nil {
			if prev, ok := w.inflight[p.RequestID]; ok {
				prev.status = p.RedirectResponse.Status
				done = w.complete(p.RequestID, prev, p.Timestamp)
			} else {
				// The hop began before Network.enable, so it has no in-flight
				// record; its redirectResponse still carries URL and status.
				// CDP does not report the hop's method, and its start time was
				// never observed, so only the wall clock at receipt is known.
				done = &NetworkEntry{
					RequestID:    p.RequestID,
					URL:          p.RedirectResponse.URL,
					Status:       p.RedirectResponse.Status,
					ResourceType: p.Type,
					Timestamp:    w.now().UnixMilli(),
				}
			}
		}
		w.inflight[p.RequestID] = &inflightRequest{
			url:           p.Request.URL,
			method:        p.Request.Method,
			resourceType:  p.Type,
			start:         p.Timestamp,
			wallTime:      p.WallTime,
			startObserved: true,
		}
		return done, nil
	case "Network.responseReceived":
		var p responseReceivedParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("decoding %s: %w", method, err)
		}
		req, ok := w.inflight[p.RequestID]
		if !ok {
			// Started before Network.enable: method and start time are unknown.
			req = &inflightRequest{url: p.Response.URL, resourceType: p.Type}
			w.inflight[p.RequestID] = req
		}
		req.status = p.Response.Status
		return nil, nil
	case "Network.loadingFinished":
		var p loadingFinishedParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("decoding %s: %w", method, err)
		}
		req, ok := w.inflight[p.RequestID]
		if !ok {
			return nil, nil // started and answered before Network.enable: nothing known but its id
		}
		entry := w.complete(p.RequestID, req, p.Timestamp)
		entry.TransferSize = int64(p.EncodedDataLength)
		return entry, nil
	case "Network.loadingFailed":
		var p loadingFailedParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("decoding %s: %w", method, err)
		}
		req, ok := w.inflight[p.RequestID]
		if !ok {
			// Same rule as loadingFinished: a failure for a request we never
			// saw start carries only its id and type. Completing an empty
			// record would emit an entry with no URL and no method, which the
			// formatter cannot distinguish from a real row.
			return nil, nil
		}
		entry := w.complete(p.RequestID, req, p.Timestamp)
		entry.Error = p.ErrorText
		if p.Canceled {
			entry.Error = "canceled: " + p.ErrorText
		}
		return entry, nil
	}
	return nil, nil
}

// complete removes requestID from the in-flight table and builds its entry.
func (w *NetworkWatcher) complete(requestID string, req *inflightRequest, endTimestamp float64) *NetworkEntry {
	delete(w.inflight, requestID)
	entry := &NetworkEntry{
		RequestID:    requestID,
		URL:          req.url,
		Method:       req.method,
		Status:       req.status,
		ResourceType: req.resourceType,
	}
	if req.startObserved && endTimestamp >= req.start {
		elapsed := endTimestamp - req.start
		entry.DurationMs = int(math.Round(elapsed * 1000))
		entry.Timestamp = int64(math.Round((req.wallTime + elapsed) * 1000))
	} else {
		entry.Timestamp = w.now().UnixMilli()
	}
	return entry
}
