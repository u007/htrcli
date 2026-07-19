//go:build !traytest && !darwin && !linux && !windows

package tray

// disabledBackend is used on platforms without a system tray (e.g. freebsd,
// openbsd, js). Run is a no-op so callers can always invoke tray.Run without
// branching on GOOS.
type disabledBackend struct{}

func init() {
	backend = disabledBackend{}
}

func (disabledBackend) Run(onReady func(), onExit func()) {
	// No tray on this platform; do nothing.
}

func (disabledBackend) Quit()             {}
func (disabledBackend) SetIcon([]byte)    {}
func (disabledBackend) SetTitle(string)   {}
func (disabledBackend) SetTooltip(string) {}
func (disabledBackend) AddMenuItem(string, string) menuItem {
	return &noopMenuItem{}
}
func (disabledBackend) AddSeparator() {}

// noopMenuItem satisfies menuItem but does nothing.
type noopMenuItem struct{}

func (noopMenuItem) Clicks() <-chan struct{} {
	return make(chan struct{})
}
func (noopMenuItem) SetTitle(string)   {}
func (noopMenuItem) SetTooltip(string) {}
func (noopMenuItem) Disable()          {}
func (noopMenuItem) Enable()           {}
func (noopMenuItem) AddSubMenuItem(string, string) menuItem {
	return &noopMenuItem{}
}
func (noopMenuItem) trigger() {}
