//go:build (darwin || linux || windows) && !traytest

package tray

import (
	"github.com/getlantern/systray"
)

// realBackend binds the tray to github.com/getlantern/systray.
type realBackend struct{}

func init() {
	backend = realBackend{}
}

func (realBackend) Run(onReady func(), onExit func()) {
	systray.Run(onReady, onExit)
}

func (realBackend) Quit() {
	systray.Quit()
}

func (realBackend) SetIcon(b []byte) {
	systray.SetIcon(b)
}

func (realBackend) SetTitle(t string) {
	systray.SetTitle(t)
}

func (realBackend) SetTooltip(t string) {
	systray.SetTooltip(t)
}

func (realBackend) AddMenuItem(title, tooltip string) menuItem {
	return &realMenuItem{MenuItem: systray.AddMenuItem(title, tooltip)}
}

func (realBackend) AddSeparator() {
	systray.AddSeparator()
}

// realMenuItem wraps *systray.MenuItem.
type realMenuItem struct {
	*systray.MenuItem
}

func (m *realMenuItem) Clicks() <-chan struct{} { return m.ClickedCh }
func (m *realMenuItem) AddSubMenuItem(title, tooltip string) menuItem {
	return &realMenuItem{MenuItem: m.MenuItem.AddSubMenuItem(title, tooltip)}
}
func (m *realMenuItem) trigger() {} // no programmatic clicks on a live menu
