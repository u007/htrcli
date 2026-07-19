package tray

import (
	"context"
	"fmt"
	"log"
	"time"
)

// menuItem is the tray package's view of a single menu entry. The real and
// fake backends (build-tag selected) provide concrete implementations.
type menuItem interface {
	Clicks() <-chan struct{}
	SetTitle(string)
	SetTooltip(string)
	Disable()
	Enable()
	// AddSubMenuItem creates a child menu item (submenu). On platforms where
	// the API nests (getlantern/systray), this is the only way to build a
	// submenu; the fake backend records it in the same flat list.
	AddSubMenuItem(title, tooltip string) menuItem
	// trigger simulates a user click. It is a test hook: the real backend
	// is a no-op (no programmatic clicks on a live menu), the fake backend
	// pushes to its click channel.
	trigger()
}

// trayBackend abstracts the platform system-tray implementation so the menu
// logic can be unit-tested without a display (see tray_fake.go, traytest tag).
type trayBackend interface {
	Run(onReady func(), onExit func())
	Quit()
	SetIcon([]byte)
	SetTitle(string)
	SetTooltip(string)
	AddMenuItem(title, tooltip string) menuItem
	AddSeparator()
}

// backend is chosen at init() by the build-tag-selected file
// (tray_real.go / tray_fake.go / tray_disabled.go).
var backend trayBackend

// Package-level menu item handles, set during onReady so the dispatch and
// refresh goroutines can read/update them.
var (
	mStatus    menuItem
	mLastErr   menuItem
	mFlash     menuItem
	mMainten   menuItem
	mReinstall menuItem
	mReChrome  menuItem
	mReFirefox menuItem
	mOpenCfg   menuItem
	mCopyTok   menuItem
	mShowLog   menuItem
	mRestart   menuItem
	mQuit      menuItem
)

// trayCancel cancels the dispatch/refresh goroutines when the tray exits.
var trayCancel context.CancelFunc

// Run blocks until Quit() is called. It drives backend.Run on the calling
// goroutine (which MUST be the main goroutine on macOS/Windows).
func Run(ctrl Controller, icon []byte) {
	backend.Run(func() { onReady(ctrl, icon) }, onExit)
}

func onReady(ctrl Controller, icon []byte) {
	backend.SetIcon(icon)
	backend.SetTitle("htrcli")
	backend.SetTooltip("htrcli daemon")

	mStatus = backend.AddMenuItem("Status: …", "")
	mStatus.Disable()
	mLastErr = backend.AddMenuItem("Last error: —", "")
	mLastErr.Disable()
	backend.AddSeparator()

	mMainten = backend.AddMenuItem("Maintenance", "Maintenance actions")
	mReinstall = mMainten.AddSubMenuItem("Reinstall native host", "Re-register the native host")
	mReChrome = mReinstall.AddSubMenuItem("Reinstall (Chrome)", "Re-register the native host in Chrome")
	mReFirefox = mReinstall.AddSubMenuItem("Reinstall (Firefox)", "Re-register the native host in Firefox")
	mOpenCfg = mMainten.AddSubMenuItem("Open config folder", "")
	mCopyTok = mMainten.AddSubMenuItem("Copy bearer token", "")
	mShowLog = mMainten.AddSubMenuItem("Show recent log", "")
	mRestart = mMainten.AddSubMenuItem("Restart", "Restart the daemon")
	mQuit = mMainten.AddSubMenuItem("Quit", "Quit the daemon")

	// Hidden flash line for transient error/toast messages.
	mFlash = backend.AddMenuItem("", "")
	mFlash.Disable()

	ctx, cancel := context.WithCancel(context.Background())
	trayCancel = cancel
	go dispatchLoop(ctx, ctrl)
	go refreshLoop(ctx, ctrl)
}

func onExit() {
	if trayCancel != nil {
		trayCancel()
	}
}

// Quit unblocks the backend.Run call in Run, allowing the main goroutine to
// return. Idempotent.
func Quit() {
	if backend != nil {
		backend.Quit()
	}
}

func dispatchLoop(ctx context.Context, ctrl Controller) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[htrcli tray] recovered from panic: %v", r)
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case <-mReChrome.Clicks():
			runAction("Reinstall Chrome", func() error { return ctrl.ReinstallHost("chrome") })
		case <-mReFirefox.Clicks():
			runAction("Reinstall Firefox", func() error { return ctrl.ReinstallHost("firefox") })
		case <-mOpenCfg.Clicks():
			runAction("Open config", ctrl.OpenConfigFolder)
		case <-mCopyTok.Clicks():
			runActionWithToast("Copy", ctrl.CopyTokenToClipboard)
		case <-mShowLog.Clicks():
			runAction("Show log", ctrl.OpenLog)
		case <-mRestart.Clicks():
			runAction("Restart", ctrl.Restart)
		case <-mQuit.Clicks():
			runAction("Quit", ctrl.Quit)
		}
	}
}

func runAction(name string, fn func() error) {
	if err := fn(); err != nil {
		flashError(name, err)
	}
}

func runActionWithToast(name string, fn func() (string, error)) {
	tok, err := fn()
	if err != nil {
		flashError(name, err)
		return
	}
	flashToast("Copied (" + Fingerprint(tok) + ")")
}

func refreshLoop(ctx context.Context, ctrl Controller) {
	ticker := time.NewTicker(refreshInterval())
	defer ticker.Stop()
	refresh(ctrl) // initial paint
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			refresh(ctrl)
		}
	}
}

func refresh(ctrl Controller) {
	s := ctrl.Status()
	mStatus.SetTitle(fmt.Sprintf("Status: %d · %d relays · %s", s.Port, s.RelaysConnected, okLabel(s.LastError)))
	if s.LastError == "" {
		mLastErr.SetTitle("Last error: —")
	} else {
		mLastErr.SetTitle("✗ Last error: " + truncate(s.LastError, 60))
	}
}

func okLabel(err string) string {
	if err == "" {
		return "ok"
	}
	return "error"
}

func flashError(name string, err error) {
	mFlash.SetTitle("✗ " + name + ": " + truncate(err.Error(), 60))
	go func() {
		time.Sleep(6 * time.Second)
		mFlash.SetTitle("")
	}()
}

func flashToast(msg string) {
	mFlash.SetTitle(msg)
	go func() {
		time.Sleep(2 * time.Second)
		mFlash.SetTitle("")
	}()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// Fingerprint returns a short, non-reversible fingerprint of a token,
// e.g. "a1b2...f3e4". An empty token yields "-".
func Fingerprint(tok string) string {
	if tok == "" {
		return "—"
	}
	if len(tok) < 8 {
		return "****"
	}
	return tok[:4] + "…" + tok[len(tok)-4:]
}
