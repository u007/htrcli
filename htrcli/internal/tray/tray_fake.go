//go:build traytest

package tray

import (
	"sync"
	"time"
)

// fakeBackend is an in-memory tray implementation used by tests under the
// traytest build tag. It lets the menu logic run without a display or a
// CGo link of getlantern/systray.
type fakeBackend struct {
	mu      sync.Mutex
	items   []*fakeMenuItem
	quitCh  chan struct{}
	running bool
	onReady func()
	onExit  func()
}

func init() {
	backend = &fakeBackend{quitCh: make(chan struct{})}
}

func (b *fakeBackend) Run(onReady func(), onExit func()) {
	b.mu.Lock()
	b.onReady = onReady
	b.onExit = onExit
	// Fresh channel per Run so the backend can be driven repeatedly within
	// a single test process (each Run is independent).
	b.quitCh = make(chan struct{})
	b.running = true
	b.mu.Unlock()
	onReady()
	// Block until Quit is called.
	<-b.quitCh
}

func (b *fakeBackend) Quit() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.running {
		return
	}
	b.running = false
	close(b.quitCh)
	if b.onExit != nil {
		b.onExit()
	}
}

func (b *fakeBackend) SetIcon([]byte)    {}
func (b *fakeBackend) SetTitle(string)   {}
func (b *fakeBackend) SetTooltip(string) {}

func (b *fakeBackend) AddMenuItem(title, tooltip string) menuItem {
	return b.add(title, tooltip)
}

func (b *fakeBackend) AddSeparator() {}

func (b *fakeBackend) add(title, tooltip string) *fakeMenuItem {
	b.mu.Lock()
	defer b.mu.Unlock()
	mi := &fakeMenuItem{back: b, title: title, tooltip: tooltip, ch: make(chan struct{}, 1)}
	b.items = append(b.items, mi)
	return mi
}

// fakeMenuItem is an in-memory menu entry.
type fakeMenuItem struct {
	back    *fakeBackend
	title   string
	tooltip string
	ch      chan struct{}
	enabled bool
}

func (m *fakeMenuItem) Clicks() <-chan struct{} { return m.ch }
func (m *fakeMenuItem) SetTitle(t string)       { m.title = t }
func (m *fakeMenuItem) SetTooltip(t string)     { m.tooltip = t }
func (m *fakeMenuItem) Disable()                { m.enabled = false }
func (m *fakeMenuItem) Enable()                 { m.enabled = true }
func (m *fakeMenuItem) AddSubMenuItem(title, tooltip string) menuItem {
	return m.back.add(title, tooltip)
}
func (m *fakeMenuItem) trigger() { m.ch <- struct{}{} }

// ClickTitle triggers the first fake menu item whose title contains sub.
// Used by tests to simulate a user click.
func ClickTitle(sub string) bool {
	fb, ok := backend.(*fakeBackend)
	if !ok {
		return false
	}
	fb.mu.Lock()
	defer fb.mu.Unlock()
	for _, it := range fb.items {
		if it.title != "" && (it.title == sub || contains(it.title, sub)) {
			it.ch <- struct{}{}
			return true
		}
	}
	return false
}

func contains(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// refreshInterval is short under traytest so refresh-driven assertions settle fast.
func refreshInterval() time.Duration {
	return 50 * time.Millisecond
}
