package host

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
)

// Relay liveness. The daemon pings every relay each PingInterval; any message
// from the relay (heartbeat reply, register, command_result…) refreshes its
// lastSeen. A relay silent for longer than StaleAfter is force-closed so its
// tabs drop out of Tabs() instead of lingering as stale duplicates (e.g. a
// browser respawned its native host without the old relay process exiting).
const (
	PingInterval = 15 * time.Second
	StaleAfter   = 45 * time.Second
)

// TabInfo describes a connected browser tab.
type TabInfo struct {
	ID     int    `json:"id"`
	URL    string `json:"url"`
	Title  string `json:"title"`
	Active bool   `json:"active"`
	// Browser is "chrome" or "firefox" for current registrations. It is
	// optional on the wire so tabs registered by older extensions remain valid.
	Browser string `json:"browser,omitempty"`
}

// Command is a browser action to execute.
type Command struct {
	ID      string          `json:"id"`
	Action  string          `json:"action"`
	Value   string          `json:"value,omitempty"`
	Target  json.RawMessage `json:"target,omitempty"`
	Options json.RawMessage `json:"options,omitempty"`
}

// CommandResult is the response from a completed command.
type CommandResult struct {
	ID       string          `json:"id"`
	Success  bool            `json:"success"`
	Data     json.RawMessage `json:"data,omitempty"`
	Error    string          `json:"error,omitempty"`
	Duration int             `json:"duration,omitempty"`
}

type pendingCommand struct {
	tabID int
	ch    chan CommandResult
}

// shotResult carries a screenshot upload back to the waiting GET handler.
// Exactly one of data / err is set.
type shotResult struct {
	data string // base64 PNG (prefix stripped)
	err  string
}

// RelayConn represents a single connected browser relay (e.g. Chrome and
// Firefox each get their own). Each connection owns the set of tabs it
// registered; commands route to the connection that owns the target tab, so
// multiple browsers can be driven simultaneously without their tab IDs
// colliding or commands being misdelivered to the wrong browser.
type RelayConn struct {
	write func(msg []byte) error
	tabs  map[int]TabInfo
	// close force-closes the underlying transport (unix socket conn). Set by
	// the socket server; nil in tests that never need reaping.
	close func() error
	// lastSeen is the time of the last message received from this relay.
	// Guarded by Daemon.mu.
	lastSeen time.Time
	// browser is the browser this relay announced ("chrome" or "firefox"),
	// recorded from the extension's identify message. Empty means the
	// extension has not announced itself, which is the normal state for an
	// older extension that predates the identify message — so empty is not
	// an error. Guarded by Daemon.mu.
	browser string
	// seq is this connection's position in connect order. Assigned once in
	// AddConn and never reused. It is the tie-breaker that makes connection
	// selection deterministic: Go randomises map iteration order, so ranging
	// over d.conns cannot define "first". Guarded by Daemon.mu.
	seq uint64
}

// Daemon holds shared state: the set of relay connections (each with its own
// tabs) and the pending command / screenshot maps (keyed by globally-unique
// command ID, so they are connection-agnostic).
type Daemon struct {
	mu      sync.Mutex
	conns   map[*RelayConn]struct{}
	pending map[string]*pendingCommand
	// Events is the CLI-facing event store for console/network/dialog capture.
	Events *EventStore
	// pendingShots correlates a capture_screenshot trigger with the HTTP
	// upload the extension POSTs back, keyed by command ID.
	pendingShots map[string]chan shotResult
	// lastErr records the most recent non-fatal error (a failed command, a
	// dropped relay, or a failed screenshot) for display in the tray. It
	// carries a timestamp so the tray can age it out (see LastError).
	lastErr   string
	lastErrAt time.Time
	// stop is closed by Stop to terminate background goroutines (e.g. the
	// sweeper). Initialized in NewDaemon; closing it is guarded by stopOnce.
	stopOnce sync.Once
	stop     chan struct{}
	// nextConnSeq hands out RelayConn.seq values. Monotonically increasing
	// and never reused, so "earliest connected" stays well defined for the life
	// of the daemon even as connections come and go. Guarded by mu.
	nextConnSeq uint64
}

// RelaysConnected returns the number of live relay connections (browsers).
func (d *Daemon) RelaysConnected() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.conns)
}

// LastError returns the most recent non-fatal error, or "" if none within the
// last 5 minutes. The staleness window keeps the tray from showing a
// long-past transient error forever.
func (d *Daemon) LastError() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if time.Since(d.lastErrAt) > 5*time.Minute {
		return ""
	}
	return d.lastErr
}

// setLastErr records a non-fatal error with the current timestamp.
func (d *Daemon) setLastErr(err string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lastErr = err
	d.lastErrAt = time.Now()
}

// NewDaemon creates an empty Daemon.
func NewDaemon() *Daemon {
	return &Daemon{
		conns:        make(map[*RelayConn]struct{}),
		pending:      make(map[string]*pendingCommand),
		Events:       NewEventStore(),
		pendingShots: make(map[string]chan shotResult),
		stop:         make(chan struct{}),
	}
}

// AddConn registers a new relay connection with its write function and returns
// a handle used to scope that connection's tabs.
func (d *Daemon) AddConn(write func(msg []byte) error) *RelayConn {
	d.mu.Lock()
	defer d.mu.Unlock()
	// Assign seq here, inside the lock, so the sequence cannot be observed out
	// of order by a concurrent reader: AddConn is the only place a RelayConn is
	// constructed and registered, so this is the only place a seq is handed out.
	d.nextConnSeq++
	rc := &RelayConn{
		write:    write,
		tabs:     make(map[int]TabInfo),
		lastSeen: time.Now(),
		seq:      d.nextConnSeq,
		// browser stays empty until the extension announces itself via identify.
	}
	d.conns[rc] = struct{}{}
	return rc
}

// SetConnCloser attaches the transport close function used to reap the
// connection when it goes stale.
func (d *Daemon) SetConnCloser(rc *RelayConn, close func() error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	rc.close = close
}

// TouchConn refreshes a relay's liveness timestamp. Called for every message
// received from that relay.
func (d *Daemon) TouchConn(rc *RelayConn) {
	d.mu.Lock()
	defer d.mu.Unlock()
	rc.lastSeen = time.Now()
}

// SweepConns performs one liveness pass: relays silent for longer than
// staleAfter are force-closed and dropped; every remaining relay is sent a
// {"type":"ping"} (the extension replies with a heartbeat, refreshing
// lastSeen). A relay whose transport write fails is dropped immediately.
// Returns the number of connections reaped, for tests and logging.
//
// Closing the socket also makes the relay process exit (its socket read
// fails), so the browser reaps the orphaned native-host process too.
func (d *Daemon) SweepConns(staleAfter time.Duration) int {
	// Snapshot under the lock: which connections exist, their write closures,
	// and whether each is already stale. The actual ping writes happen OUTSIDE
	// the lock — a wedged relay (peer not reading, full buffer) can block a
	// socket write for the duration of TCP timeout detection, and holding d.mu
	// there would freeze every other daemon operation (EnqueueCommand,
	// RegisterTab, Tabs, ResolveCommand) for that window.
	ping, _ := json.Marshal(NativeMessage{Type: "ping"})
	type snap struct {
		rc    *RelayConn
		write func([]byte) error
		stale bool
	}
	d.mu.Lock()
	conns := make([]snap, 0, len(d.conns))
	for rc := range d.conns {
		conns = append(conns, snap{
			rc:    rc,
			write: rc.write,
			stale: time.Since(rc.lastSeen) > staleAfter,
		})
	}
	d.mu.Unlock()

	reaped := 0
	// Pass 1: ping the non-stale connections outside the lock. If a write
	// fails, the relay is dead — reap it (re-checking membership under the
	// lock in case it already disconnected in the meantime).
	for _, c := range conns {
		if c.stale {
			continue
		}
		if err := c.write(ping); err != nil {
			d.mu.Lock()
			if _, ok := d.conns[c.rc]; ok {
				log.Printf("[htrcli serve] reaping relay after ping write failure (%d tabs): %v",
					len(c.rc.tabs), err)
				d.dropConnLocked(c.rc)
				reaped++
			}
			d.mu.Unlock()
		}
	}

	// Pass 2: reap connections that were stale at snapshot time. Re-check
	// staleness under the lock — a relay that spoke between the snapshot and
	// here is spared rather than wrongly reaped.
	d.mu.Lock()
	for _, c := range conns {
		if !c.stale {
			continue
		}
		if _, ok := d.conns[c.rc]; !ok {
			continue // already gone (normal disconnect)
		}
		if time.Since(c.rc.lastSeen) > staleAfter {
			log.Printf("[htrcli serve] reaping stale relay (%d tabs, silent %s)",
				len(c.rc.tabs), time.Since(c.rc.lastSeen).Round(time.Second))
			d.dropConnLocked(c.rc)
			reaped++
		}
	}
	d.mu.Unlock()
	return reaped
}

// dropConnLocked removes a connection and closes its transport. Caller must
// hold d.mu.
func (d *Daemon) dropConnLocked(rc *RelayConn) {
	delete(d.conns, rc)
	if rc.close != nil {
		if err := rc.close(); err != nil {
			// Expected when the transport is already half-dead; log at
			// debug-equivalent level for traceability.
			log.Printf("[htrcli serve] relay close: %v", err)
		}
	}
}

// StartSweeper runs SweepConns every interval until the daemon is stopped via
// Stop. The sweeper owns its lifecycle through the daemon's internal stop
// channel rather than an ad-hoc throwaway channel, so it can be cleanly
// terminated (e.g. on daemon shutdown) instead of leaking until process exit.
func (d *Daemon) StartSweeper(interval, staleAfter time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			d.SweepConns(staleAfter)
		case <-d.stop:
			return
		}
	}
}

// Stop terminates background goroutines started by the daemon (currently the
// sweeper). Safe to call multiple times; the first call closes the stop
// channel and subsequent calls are no-ops.
func (d *Daemon) Stop() {
	d.stopOnce.Do(func() { close(d.stop) })
}

// RemoveConn drops a relay connection and every tab it owned. Other
// connections are left untouched, so one browser disconnecting does not stop
// commands to the others. A dropped relay is a notable event, so it is
// recorded as the daemon's last error for tray display.
func (d *Daemon) RemoveConn(rc *RelayConn) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.conns[rc]; ok {
		d.lastErr = "relay disconnected"
		d.lastErrAt = time.Now()
	}
	delete(d.conns, rc)
}

// RegisterTab records a tab as owned by the given connection.
func (d *Daemon) RegisterTab(rc *RelayConn, tabID int, info TabInfo) {
	d.mu.Lock()
	defer d.mu.Unlock()
	rc.tabs[tabID] = info
}

// RemoveTab removes a tab from the given connection.
func (d *Daemon) RemoveTab(rc *RelayConn, tabID int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(rc.tabs, tabID)
}

// Tabs returns a snapshot of all connected tabs across every connection.
func (d *Daemon) Tabs() []TabInfo {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]TabInfo, 0)
	for rc := range d.conns {
		for _, t := range rc.tabs {
			out = append(out, t)
		}
	}
	return out
}

// FirstTabID returns a connected tab ID, or false if no connection has a tab.
// Used as the default target for commands and screenshots that don't specify a
// tab.
//
// Selection is deterministic: the tab comes from the eligible connection with
// the lowest seq (earliest connected), and within that connection from the
// lowest tab ID. The previous implementation returned whichever entry Go's
// randomised map iteration happened to yield, so repeated calls could name
// different browsers — see TestFirstTabIDIsDeterministicAcrossRepeatedCalls.
// The old doc comment claimed "first match wins", which was never true.
func (d *Daemon) FirstTabID() (int, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	rc := d.earliestConnWithTabs()
	if rc == nil {
		return 0, false
	}
	return lowestTabID(rc.tabs), true
}

// earliestConnWithTabs returns the connected RelayConn with the lowest seq that
// has at least one registered tab, or nil if none does. Caller must hold d.mu.
func (d *Daemon) earliestConnWithTabs() *RelayConn {
	var best *RelayConn
	for rc := range d.conns {
		if len(rc.tabs) == 0 {
			continue
		}
		if best == nil || rc.seq < best.seq {
			best = rc
		}
	}
	return best
}

// lowestTabID returns the smallest tab ID registered on a connection. Caller
// must hold d.mu, and must only call it for a connection with at least one tab.
func lowestTabID(tabs map[int]TabInfo) int {
	best, seen := 0, false
	for id := range tabs {
		if !seen || id < best {
			best, seen = id, true
		}
	}
	return best
}

// findOwner returns the connection that owns tabID. Caller must hold d.mu.
// If the same native tab ID exists in more than one browser (possible only
// when both use low, overlapping IDs), the first match wins.
func (d *Daemon) findOwner(tabID int) (*RelayConn, bool) {
	for rc := range d.conns {
		if _, ok := rc.tabs[tabID]; ok {
			return rc, true
		}
	}
	return nil, false
}

// EnqueueCommand sends a command to the relay for a specific tab.
// Returns a channel that receives the result when the extension responds.
func (d *Daemon) EnqueueCommand(tabID int, cmd Command) (<-chan CommandResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	rc, ok := d.findOwner(tabID)
	if !ok {
		return nil, fmt.Errorf("tab %d not connected", tabID)
	}

	ch := make(chan CommandResult, 1)
	d.pending[cmd.ID] = &pendingCommand{tabID: tabID, ch: ch}

	msg := NativeMessage{
		Type:    "command",
		TabID:   tabID,
		Payload: mustMarshal(cmd),
	}
	data, _ := json.Marshal(msg)
	if err := rc.write(data); err != nil {
		delete(d.pending, cmd.ID)
		return nil, fmt.Errorf("relay write: %w", err)
	}

	return ch, nil
}

// SetConnBrowser records the browser a relay announced via its identify message.
//
// An empty browser is ignored rather than stored: "not announced yet" and
// "announced as nothing" both mean unknown, and treating an empty value as a
// real announcement would let a relay un-identify itself. Connections from
// extensions that predate the identify message simply keep the empty value and
// remain selectable via the no-hint path.
func (d *Daemon) SetConnBrowser(rc *RelayConn, browser string) {
	if browser == "" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	rc.browser = browser
}

// ConnBrowser returns the browser a relay announced, or "" if it has not
// announced one.
func (d *Daemon) ConnBrowser(rc *RelayConn) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return rc.browser
}

// selectConn picks the connection that should answer a tab-less command.
//
// The hint is advisory. A hint that matches nothing falls back to the
// earliest-connected connection instead of returning an error, on purpose: a
// caller that hard-failed on a stale hint would be strictly worse off than one
// that got an answer from the wrong profile, because `recordings list` reports
// which browser actually answered. The same fallback covers an empty hint.
//
// Ties — including two connections that both announced the same browser — go to
// the lowest seq, i.e. the earliest connected. Caller must hold d.mu.
func (d *Daemon) selectConn(browserHint string) (*RelayConn, bool) {
	var matched, earliest *RelayConn
	for rc := range d.conns {
		if earliest == nil || rc.seq < earliest.seq {
			earliest = rc
		}
		if browserHint != "" && rc.browser == browserHint {
			if matched == nil || rc.seq < matched.seq {
				matched = rc
			}
		}
	}
	if matched != nil {
		return matched, true
	}
	if earliest != nil {
		return earliest, true
	}
	return nil, false
}

// EnqueueBackgroundCommand sends a command to a browser relay without naming a
// tab, for actions the extension handles in its background service worker rather
// than in a page (session recording being the motivating case).
//
// browserHint is advisory — see selectConn. The command is written with TabID
// left at zero; result delivery keys on the command ID, exactly as
// EnqueueCommand does, so the pending bookkeeping is identical.
//
// Takes the whole Command, exactly as EnqueueCommand does, so the caller owns
// cmd.ID. That matters on the timeout path: the caller deletes d.pending[cmd.ID]
// itself, so it has to be the same ID the entry was registered under. An empty
// cmd.ID is filled in here for convenience.
//
// Returns a channel that receives the result when the extension responds, or an
// error if no browser is connected or the relay write fails.
func (d *Daemon) EnqueueBackgroundCommand(cmd Command, browserHint string) (<-chan CommandResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	rc, ok := d.selectConn(browserHint)
	if !ok {
		return nil, fmt.Errorf("no browser connected")
	}

	if cmd.ID == "" {
		cmd.ID = generateID()
	}

	ch := make(chan CommandResult, 1)
	d.pending[cmd.ID] = &pendingCommand{tabID: 0, ch: ch}

	msg := NativeMessage{
		Type:    "command",
		TabID:   0, // tab-less: the background worker handles it, no page involved
		Payload: mustMarshal(cmd),
	}
	data, _ := json.Marshal(msg)
	if err := rc.write(data); err != nil {
		// Mirror EnqueueCommand: drop the pending entry so a failed write
		// cannot leak an entry nobody will ever resolve.
		delete(d.pending, cmd.ID)
		return nil, fmt.Errorf("relay write: %w", err)
	}

	return ch, nil
}

// ResolveCommand delivers a command result to the waiting caller.
func (d *Daemon) ResolveCommand(commandID string, result CommandResult) {
	d.mu.Lock()
	p, ok := d.pending[commandID]
	if ok {
		delete(d.pending, commandID)
	}
	if !result.Success {
		d.lastErr = result.Error
		d.lastErrAt = time.Now()
	}
	d.mu.Unlock()

	if ok {
		p.ch <- result
	}
}

// screenshotTrigger is the payload of a capture_screenshot native message.
// It tells the extension where to upload the captured PNG over HTTP, since the
// extension has no other way to learn the daemon's URL + token in native mode.
// ScreenshotOpts carries optional parameters for a screenshot capture.
type ScreenshotOpts struct {
	FullPage bool
	Annotate json.RawMessage
}

type screenshotTrigger struct {
	UploadURL string          `json:"uploadUrl"`
	Token     string          `json:"token,omitempty"`
	FullPage  bool            `json:"fullPage,omitempty"`
	Annotate  json.RawMessage `json:"annotate,omitempty"`
}

// TriggerScreenshot asks the extension (via the relay) to capture tab tabID and
// POST the PNG back to uploadURL. Returns a channel that receives the upload.
// Screenshots are deliberately NOT returned over the relay: a base64 PNG
// routinely exceeds the 1 MB native-messaging frame limit, so they travel over
// HTTP instead (see POST /api/screenshot).
func (d *Daemon) TriggerScreenshot(tabID int, commandID, uploadURL, token string, opts ScreenshotOpts) (<-chan shotResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	rc, ok := d.findOwner(tabID)
	if !ok {
		return nil, fmt.Errorf("tab %d not connected", tabID)
	}

	ch := make(chan shotResult, 1)
	d.pendingShots[commandID] = ch

	msg := NativeMessage{
		Type:      "capture_screenshot",
		TabID:     tabID,
		CommandID: commandID,
		Payload: mustMarshal(screenshotTrigger{
			UploadURL: uploadURL,
			Token:     token,
			FullPage:  opts.FullPage,
			Annotate:  opts.Annotate,
		}),
	}
	data, _ := json.Marshal(msg)
	if err := rc.write(data); err != nil {
		delete(d.pendingShots, commandID)
		return nil, fmt.Errorf("relay write: %w", err)
	}
	return ch, nil
}

// ResolveScreenshot delivers an uploaded screenshot (or error) to the waiting
// GET handler. No-op if the command ID is unknown (timed out / duplicate).
func (d *Daemon) ResolveScreenshot(commandID, data, errMsg string) {
	d.mu.Lock()
	ch, ok := d.pendingShots[commandID]
	if ok {
		delete(d.pendingShots, commandID)
	}
	if errMsg != "" {
		d.lastErr = errMsg
		d.lastErrAt = time.Now()
	}
	d.mu.Unlock()

	if ok {
		ch <- shotResult{data: data, err: errMsg}
	}
}

func mustMarshal(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
