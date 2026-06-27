package host

import (
	"encoding/json"
	"fmt"
	"sync"
)

// TabInfo describes a connected browser tab.
type TabInfo struct {
	ID     int    `json:"id"`
	URL    string `json:"url"`
	Title  string `json:"title"`
	Active bool   `json:"active"`
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

// Daemon holds shared state: tab registry and pending command map.
type Daemon struct {
	mu      sync.Mutex
	tabs    map[int]TabInfo
	pending map[string]*pendingCommand
	// relay is the write function for the active relay connection.
	// Set by the Unix socket server when a relay connects; nil when none.
	relay func(msg []byte) error
}

// NewDaemon creates an empty Daemon.
func NewDaemon() *Daemon {
	return &Daemon{
		tabs:    make(map[int]TabInfo),
		pending: make(map[string]*pendingCommand),
	}
}

// RegisterTab records a connected tab.
func (d *Daemon) RegisterTab(tabID int, info TabInfo) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.tabs[tabID] = info
}

// RemoveTab removes a tab from the registry.
func (d *Daemon) RemoveTab(tabID int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.tabs, tabID)
}

// Tabs returns a snapshot of connected tabs.
func (d *Daemon) Tabs() []TabInfo {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]TabInfo, 0, len(d.tabs))
	for _, t := range d.tabs {
		out = append(out, t)
	}
	return out
}

// FirstTabID returns the first available tab ID, or false if none.
func (d *Daemon) FirstTabID() (int, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for id := range d.tabs {
		return id, true
	}
	return 0, false
}

// SetRelay sets the write function for the current relay connection.
func (d *Daemon) SetRelay(fn func([]byte) error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.relay = fn
}

// EnqueueCommand sends a command to the relay for a specific tab.
// Returns a channel that receives the result when the extension responds.
func (d *Daemon) EnqueueCommand(tabID int, cmd Command) (<-chan CommandResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, ok := d.tabs[tabID]; !ok {
		return nil, fmt.Errorf("tab %d not connected", tabID)
	}

	ch := make(chan CommandResult, 1)
	d.pending[cmd.ID] = &pendingCommand{tabID: tabID, ch: ch}

	if d.relay != nil {
		msg := NativeMessage{
			Type:    "command",
			TabID:   tabID,
			Payload: mustMarshal(cmd),
		}
		data, _ := json.Marshal(msg)
		if err := d.relay(data); err != nil {
			delete(d.pending, cmd.ID)
			return nil, fmt.Errorf("relay write: %w", err)
		}
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
	d.mu.Unlock()

	if ok {
		p.ch <- result
	}
}

func mustMarshal(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
