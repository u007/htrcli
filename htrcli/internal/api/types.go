package api

import "encoding/json"

// TargetSelector defines how to find an element on the page.
// Multiple strategies can be combined; they are tried in priority order.
type TargetSelector struct {
	Selector      string   `json:"selector,omitempty"`
	XPath         string   `json:"xpath,omitempty"`
	ID            string   `json:"id,omitempty"`
	Name          string   `json:"name,omitempty"`
	Role          string   `json:"role,omitempty"`
	Label         string   `json:"label,omitempty"`
	Placeholder   string   `json:"placeholder,omitempty"`
	Text          string   `json:"text,omitempty"`
	TextMatch     string   `json:"textMatch,omitempty"`
	CaseSensitive *bool    `json:"caseSensitive,omitempty"`
	Tag           string   `json:"tag,omitempty"`
	Type          string   `json:"type,omitempty"`
	Index         *int     `json:"index,omitempty"`
	All           *bool    `json:"all,omitempty"`
	Visible       *bool    `json:"visible,omitempty"`
	Enabled       *bool    `json:"enabled,omitempty"`
	Ref           string   `json:"ref,omitempty"`
	X             *float64 `json:"x,omitempty"`
	Y             *float64 `json:"y,omitempty"`
}

// ScreenshotOptions controls htrcli screenshot capture. Annotate is a list of
// selectors whose matched elements get numbered overlay boxes drawn before
// capture. TabID scopes extension screenshots to a specific connected tab when
// provided. Empty options = plain viewport screenshot (unchanged behavior).
type ScreenshotOptions struct {
	FullPage bool             `json:"fullPage,omitempty"`
	Annotate []TargetSelector `json:"annotate,omitempty"`
	TabID    *int             `json:"tabId,omitempty"`
}

// Command represents a remote control command to execute on a browser tab.
type Command struct {
	ID      string          `json:"id"`
	Action  string          `json:"action"`
	Target  *TargetSelector `json:"target,omitempty"`
	Value   string          `json:"value,omitempty"`
	Options map[string]any  `json:"options,omitempty"`
}

// CommandResult is the response from executing a command.
type CommandResult struct {
	ID         string    `json:"id"`
	Success    bool      `json:"success"`
	Data       any       `json:"data,omitempty"`
	Error      string    `json:"error,omitempty"`
	Screenshot string    `json:"screenshot,omitempty"`
	Duration   int       `json:"duration,omitempty"`
	PageInfo   *PageInfo `json:"pageInfo,omitempty"`
}

// ─── Session recordings ─────────────────────────────────────────────
//
// These mirror the extension's session recorder (interaction steps +
// screenshots in IndexedDB), NOT the CDP screencast video recorder that
// `htrcli record` drives. They are background-handled, so they need no tab.

// RecordingSessionMeta is one entry in a `recordings list` result.
type RecordingSessionMeta struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	StartTime       int64  `json:"startTime"`
	EndTime         *int64 `json:"endTime,omitempty"`
	HasAudio        bool   `json:"hasAudio"`
	StepCount       int    `json:"stepCount"`
	AnnotationCount int    `json:"annotationCount"`
}

// RecordingListData is the `recordingList` result.
type RecordingListData struct {
	Sessions []RecordingSessionMeta `json:"sessions"`
	Total    int                    `json:"total"`
	Limit    int                    `json:"limit"`
	Offset   int                    `json:"offset"`
	// Browser is which browser answered: "chrome", "firefox", or "" if the
	// result did not say. The extension is the authority on its own identity.
	//
	// The "" case is not hypothetical: encoding/json silently drops a JSON key
	// with no matching struct field, so forgetting this line would not error —
	// it would make the CLI's browser column permanently blank, which reads as
	// a UI bug rather than a missing field. TestRecordingListDataDecodesBrowser
	// exists to catch exactly that.
	Browser string `json:"browser"`
}

// RecordingStateData is the `recordingStart`/`recordingStop`/
// `recordingStatus` result.
type RecordingStateData struct {
	Recording bool                  `json:"recording"`
	Session   *RecordingSessionMeta `json:"session"`
}

// RecordingElementInfo identifies the element a step acted on.
// Mirrors ElementInfo in src/types/recording.ts.
type RecordingElementInfo struct {
	Tag       string `json:"tag"`
	Text      string `json:"text"`
	Selector  string `json:"selector"`
	Type      string `json:"type,omitempty"`
	Name      string `json:"name,omitempty"`
	ID        string `json:"id,omitempty"`
	ClassName string `json:"className,omitempty"`
	AriaLabel string `json:"ariaLabel,omitempty"`
}

// RecordingStep is one captured interaction in a session.
// Mirrors RecordingStep in src/types/recording.ts.
type RecordingStep struct {
	ID        string                `json:"id"`
	Timestamp int64                 `json:"timestamp"` // ms from recording start
	Type      string                `json:"type"`      // click | input | navigation
	TabID     int                   `json:"tabId"`
	TabTitle  string                `json:"tabTitle"`
	URL       string                `json:"url"`
	Element   *RecordingElementInfo `json:"element,omitempty"`
	// InputValue is already masked by the extension when IsSensitive is set.
	InputValue     string  `json:"inputValue,omitempty"`
	IsSensitive    bool    `json:"isSensitive,omitempty"`
	ScreenshotData *string `json:"screenshotData,omitempty"` // base64
	AudioData      *string `json:"audioData,omitempty"`      // base64 webm
}

// RecordingAnnotation is a user note pinned to a session.
// Mirrors Annotation in src/types/recording.ts.
type RecordingAnnotation struct {
	ID             string  `json:"id"`
	Timestamp      int64   `json:"timestamp"`
	Text           string  `json:"text"`
	ScreenshotData *string `json:"screenshotData,omitempty"`
	AudioData      *string `json:"audioData,omitempty"`
}

// RecordingSession is a full session, as returned by `recordingGet`.
type RecordingSession struct {
	ID           string                `json:"id"`
	Title        string                `json:"title"`
	StartTime    int64                 `json:"startTime"`
	EndTime      *int64                `json:"endTime,omitempty"`
	IsRecording  bool                  `json:"isRecording"`
	HasAudio     bool                  `json:"hasAudio"`
	Steps        []RecordingStep       `json:"steps"`
	Annotations  []RecordingAnnotation `json:"annotations"`
	TrackedTabID []int                 `json:"trackedTabIds,omitempty"`
}

// RecordingGetData is the `recordingGet` result.
type RecordingGetData struct {
	Session RecordingSession `json:"session"`
	// MediaStripped reports that base64 screenshots/audio were removed to keep
	// the payload inside the native-messaging limit.
	MediaStripped bool `json:"mediaStripped"`
}

// RecordingDeleteData is the `recordingDelete` result.
type RecordingDeleteData struct {
	ID      string `json:"id"`
	Deleted bool   `json:"deleted"`
}

// TabInfo contains information about a connected browser tab.
type TabInfo struct {
	ID         int    `json:"id"`
	URL        string `json:"url"`
	Title      string `json:"title"`
	Active     bool   `json:"active"`
	FavIconURL string `json:"favIconUrl,omitempty"`
	// Browser is "chrome" or "firefox". Empty means legacy/unknown metadata;
	// callers must use the Chrome-compatible path in that case.
	Browser string `json:"browser,omitempty"`
}

// PageInfo contains information about the current page state.
// Field names mirror the extension's PageInfo; new fields added on the
// extension side must be mirrored here (the daemon's /api/page returns
// the live PageInfo and the client decodes it back into this struct).
type PageInfo struct {
	URL            string  `json:"url"`
	Title          string  `json:"title"`
	Domain         string  `json:"domain"`
	ReadyState     string  `json:"readyState,omitempty"`
	ScrollX        float64 `json:"scrollX"`
	ScrollY        float64 `json:"scrollY"`
	ViewportWidth  int     `json:"viewportWidth"`
	ViewportHeight int     `json:"viewportHeight"`
	DocumentHeight int     `json:"documentHeight"`
	DocumentWidth  int     `json:"documentWidth"`
	HistoryLength  int     `json:"historyLength,omitempty"`
}

// ApiResponse is the standard response envelope from the server.
type ApiResponse struct {
	OK    bool   `json:"ok"`
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

// EventEntry is one captured page event. The Data payload is caller-specific.
type EventEntry struct {
	Seq       int             `json:"seq"`
	Kind      string          `json:"kind"`
	Timestamp int64           `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
}

// EventsResponse is returned by GET /api/events.
type EventsResponse struct {
	Entries            []EventEntry `json:"entries"`
	Dropped            int          `json:"dropped"`
	OldestAvailableSeq int          `json:"oldestAvailableSeq"`
}

// IngestEventsRequest is the request body for POST /api/events/ingest.
type IngestEventsRequest struct {
	TabID   int          `json:"tabId"`
	Kind    string       `json:"kind"`
	Entries []EventEntry `json:"entries"`
}

// CommandRequest is the request body for POST /api/command.
type CommandRequest struct {
	Command    Command `json:"command"`
	Screenshot bool    `json:"screenshot,omitempty"`
	Timeout    int     `json:"timeout,omitempty"`
}

// BackgroundCommandRequest is the body of POST /api/background/command.
// Deliberately NOT CommandRequest: that type carries `Screenshot`, a tab-routed
// concept this route does not support, and reusing it would advertise a field
// the daemon ignores.
type BackgroundCommandRequest struct {
	Command Command `json:"command"`
	Timeout int     `json:"timeout,omitempty"`
	// Browser is an advisory hint ("chrome"/"firefox") naming the profile to
	// prefer. omitempty so a caller with no preference sends nothing, leaving the
	// body identical to one that predates the flag.
	Browser string `json:"browser,omitempty"`
}

// HealthResponse is the response from GET /api/health.
type HealthResponse struct {
	Status        string  `json:"status"`
	ConnectedTabs int     `json:"connectedTabs"`
	Uptime        float64 `json:"uptime"`
}
