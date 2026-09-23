//go:build windows && !traytest && !htrcli_native_tray

package tray

// disabledBackend is the default Windows tray backend: a no-op stub selected
// unless the `htrcli_native_tray` build tag is supplied. Keeping the stub as
// the default makes plain cross-compiles independent of the
// getlantern/systray backend, while still allowing it on request. Run is a
// no-op so callers can always invoke tray.Run without branching on GOOS.
//
// To build with the real systray backend:
//
//	go build -tags htrcli_native_tray ./cmd/htrcli
type disabledBackend struct{}

func init() {
	backend = disabledBackend{}
}

func (disabledBackend) Run(onReady func(), onExit func()) {
	// No tray when cross-compiled; do nothing.
}

func (disabledBackend) Quit()             {}
func (disabledBackend) SetIcon([]byte)    {}
func (disabledBackend) SetTitle(string)   {}
func (disabledBackend) SetTooltip(string) {}
func (disabledBackend) AddMenuItem(title, tooltip string) menuItem {
	return nil
}
func (disabledBackend) AddSeparator() {}
