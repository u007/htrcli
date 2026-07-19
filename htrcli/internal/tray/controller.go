package tray

// Status is the read-only snapshot the tray displays every refresh tick.
type Status struct {
	Port             int
	RelaysConnected  int
	LastError        string
	TokenFingerprint string // e.g. "a1b2…f3e4"; "—" when unset
}

// Controller is the surface the tray needs from the daemon.
//
// Start/Stop are intentionally absent: htrcli serve is a single-process
// daemon that owns its HTTP port; suspending and resuming the daemon
// without exiting the process is not supported. The tray exposes only
// Restart (re-execs the process) and Quit (exits the process).
type Controller interface {
	// Lifecycle
	IsRunning() bool  // always true while the tray is attached
	Restart() error   // close listener + exec self + os.Exit(0)
	Quit() error      // trigger the daemon shutdown path
	SetQuitFn(func()) // register the shutdown callback (wired by serve.go)

	// Status (read-only; called by the refresh goroutine every 5s)
	Status() Status
	RecentLog(n int) []string // reserved for future inline log display; not yet wired to a menu item

	// Maintenance
	ReinstallHost(browser string) error
	OpenConfigFolder() error
	OpenLog() error // open ~/.htrcli/serve.log in OS default app
	CopyTokenToClipboard() (string, error)
}

// fakeController is a test double that satisfies Controller and returns
// canned values. It records which actions were invoked.
type fakeController struct {
	status     Status
	log        []string
	invoked    []string
	quitFn     func()
	copyResult string
	copyErr    error
}

func (f *fakeController) IsRunning() bool { return true }

func (f *fakeController) Restart() error {
	f.invoked = append(f.invoked, "Restart")
	return nil
}

func (f *fakeController) Quit() error {
	f.invoked = append(f.invoked, "Quit")
	if f.quitFn != nil {
		f.quitFn()
	}
	return nil
}

func (f *fakeController) SetQuitFn(fn func()) {
	f.quitFn = fn
}

func (f *fakeController) Status() Status { return f.status }

func (f *fakeController) RecentLog(n int) []string {
	if n <= 0 || n >= len(f.log) {
		return f.log
	}
	return f.log[len(f.log)-n:]
}

func (f *fakeController) ReinstallHost(browser string) error {
	f.invoked = append(f.invoked, "ReinstallHost:"+browser)
	return nil
}

func (f *fakeController) OpenConfigFolder() error {
	f.invoked = append(f.invoked, "OpenConfigFolder")
	return nil
}

func (f *fakeController) OpenLog() error {
	f.invoked = append(f.invoked, "OpenLog")
	return nil
}

func (f *fakeController) CopyTokenToClipboard() (string, error) {
	f.invoked = append(f.invoked, "CopyTokenToClipboard")
	return f.copyResult, f.copyErr
}
