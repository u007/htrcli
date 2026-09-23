package cdp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// BrowserState is persisted at ~/.htrcli/browser.json. Advisory only: the
// debugging port answering is the source of truth for "running".
type BrowserState struct {
	PID       int       `json:"pid"`
	Port      int       `json:"port"`
	StartedAt time.Time `json:"started_at"`
	Headless  bool      `json:"headless"`
}

const macChromePath = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"

var (
	// launchTimeout is how long launchChrome waits for the debugging port to
	// answer before giving up. Overridable in tests.
	launchTimeout = 15 * time.Second
	// listeningPIDFn resolves the pid owning a listening TCP port. Overridable
	// in tests so they do not depend on lsof/netstat.
	listeningPIDFn = processListeningPID
)

// StateFilePath returns ~/.htrcli/browser.json.
func StateFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	return filepath.Join(home, ".htrcli", "browser.json"), nil
}

// ProfileDir returns ~/.htrcli/chrome-profile.
func ProfileDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	return filepath.Join(home, ".htrcli", "chrome-profile"), nil
}

// ReadState returns nil, nil when no state file exists.
func ReadState() (*BrowserState, error) {
	path, err := StateFilePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil // intentionally not logged: absent state file means "not started", an expected case
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var st BrowserState
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &st, nil
}

func writeState(st *BrowserState) error {
	path, err := StateFilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling browser state: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// LaunchArgs builds the Chrome argument list. Security: never add
// --remote-debugging-address — the port must stay bound to localhost.
func LaunchArgs(port int, profileDir string, headless bool) []string {
	args := []string{
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--user-data-dir=" + profileDir,
		"--no-first-run",
		"--disable-backgrounding-occluded-windows",
		"--disable-renderer-backgrounding",
	}
	if headless {
		args = append(args, "--headless")
	}
	return args
}

// FindChrome returns the configured binary or the standard macOS path.
func FindChrome(configured string) (string, error) {
	candidates := []string{configured, macChromePath}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf(
		"Chrome binary not found (tried %q, %q) — set it with: htrcli config set-chrome-path <path>",
		configured, macChromePath)
}

// PortAlive reports whether /json/version answers on the port.
func PortAlive(port int) bool {
	_, err := BrowserWSURL(port)
	return err == nil
}

// adoptListenerPID resolves the pid of the process already listening on port.
// launchChrome returns pid 0 in that case (an owner htrcli did not spawn), and
// persisting 0 would leave the recorded browser or context impossible to stop.
// It errors rather than returning 0 so callers never store a meaningless pid.
func adoptListenerPID(port int) (int, error) {
	pid, err := listeningPIDFn(port)
	if err != nil {
		return 0, fmt.Errorf("port %d is held by a process htrcli cannot identify: %w", port, err)
	}
	return pid, nil
}

// launchChrome starts Chrome detached on port with the given profile dir and
// waits for the debugging port to answer. It does NOT persist any state file —
// callers record the result where appropriate (browser.json vs contexts.json).
// If the port already answers it returns pid 0 (an already-running owner, e.g.
// Chrome's singleton-lock handoff).
func launchChrome(chromePath string, port int, profileDir string, headless bool) (int, error) {
	if PortAlive(port) {
		return 0, nil
	}
	if err := os.MkdirAll(profileDir, 0700); err != nil {
		return 0, fmt.Errorf("creating profile dir: %w", err)
	}
	if removed, err := clearStaleSingletonLock(profileDir); err != nil {
		return 0, err
	} else if removed {
		fmt.Fprintf(os.Stderr, "[htrcli] removed stale Chrome singleton lock in %s (owner process is gone)\n", profileDir)
	}
	cmd := exec.Command(chromePath, LaunchArgs(port, profileDir, headless)...)
	configureDetachedProcess(cmd) // detach where the platform supports it
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("launching Chrome %s: %w", chromePath, err)
	}
	// Reap exactly once so a detached Chrome that later exits never zombies
	// against a still-running htrcli process. reaped closes when Wait returns,
	// letting the timeout path confirm the killed child is actually gone.
	reaped := make(chan struct{})
	go func() {
		defer close(reaped)
		if err := cmd.Wait(); err != nil {
			fmt.Fprintf(os.Stderr, "[htrcli] Chrome exited: %v\n", err)
		}
	}()

	deadline := time.Now().Add(launchTimeout)
	for time.Now().Before(deadline) {
		if PortAlive(port) {
			return cmd.Process.Pid, nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	// Do not leave an unreachable Chrome orphaned holding the profile lock.
	pid := cmd.Process.Pid
	if err := terminateProcess(pid); err != nil {
		return 0, fmt.Errorf("Chrome (pid %d) did not answer on port %d within %s; killing it also failed: %w", pid, port, launchTimeout, err)
	}
	// terminateProcess polls processAlive, which on Unix still reports an
	// exited-but-unreaped child as alive. Wait for the reaper so the child is
	// really gone before returning instead of lingering as a zombie.
	select {
	case <-reaped:
	case <-time.After(5 * time.Second):
		fmt.Fprintf(os.Stderr, "[htrcli] Chrome (pid %d) did not reap within 5s after kill\n", pid)
	}
	return 0, fmt.Errorf("Chrome (pid %d) did not answer on port %d within %s (killed)", pid, port, launchTimeout)
}

// singletonLockFiles are the per-profile lock artefacts Chrome leaves behind
// when it dies without cleaning up (crash, reboot, SIGKILL).
var singletonLockFiles = []string{"SingletonLock", "SingletonSocket", "SingletonCookie"}

// clearStaleSingletonLock removes Chrome's profile singleton lock when the
// process it names is no longer alive on this host. Chrome itself recovers from
// this only after its own ~20s handoff timeout, which is longer than our launch
// wait, so a crashed previous Chrome otherwise turns every later start into a
// hang. The lock is a symlink whose target is "<hostname>-<pid>". A live owner,
// a lock held by another host (shared/synced profile dir), or an absent lock
// leaves everything untouched; a malformed target is an error.
func clearStaleSingletonLock(profileDir string) (bool, error) {
	lock := filepath.Join(profileDir, "SingletonLock")
	target, err := os.Readlink(lock)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil // intentionally not logged: no lock is the normal state after a clean stop
	}
	if err != nil {
		return false, fmt.Errorf("reading %s: %w", lock, err)
	}
	i := strings.LastIndex(target, "-")
	if i < 0 {
		return false, fmt.Errorf("unexpected singleton lock target %q in %s", target, lock)
	}
	host := target[:i]
	pid, err := strconv.Atoi(target[i+1:])
	if err != nil {
		return false, fmt.Errorf("unexpected singleton lock target %q in %s: %w", target, lock, err)
	}
	// A pid only means something within its own host's pid namespace. When the
	// profile directory is shared or synced, another machine's lock must be
	// left alone even if that pid is not alive locally.
	localHost, err := os.Hostname()
	if err != nil {
		return false, fmt.Errorf("resolving hostname for %s: %w", lock, err)
	}
	if !strings.EqualFold(host, localHost) {
		return false, nil
	}
	if processAlive(pid) {
		return false, nil
	}
	for _, name := range singletonLockFiles {
		path := filepath.Join(profileDir, name)
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return false, fmt.Errorf("removing stale %s: %w", path, err)
		}
	}
	return true, nil
}

// terminateProcess sends SIGTERM and, if needed, SIGKILL to a process that was
// just started by htrcli. Best-effort only; dead PIDs are treated as success.
func terminateProcess(pid int) error {
	if pid <= 0 {
		return nil
	}
	if err := terminatePID(pid, false); err != nil {
		return fmt.Errorf("signalling pid %d: %w", pid, err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := terminatePID(pid, true); err != nil {
		return fmt.Errorf("force-killing pid %d: %w", pid, err)
	}
	return nil
}

// StartBrowser launches the default-profile Chrome, waits for the port, and
// persists advisory state to browser.json.
func StartBrowser(chromePath string, port int, headless bool) (*BrowserState, error) {
	profile, err := ProfileDir()
	if err != nil {
		return nil, err
	}
	pid, err := launchChrome(chromePath, port, profile, headless)
	if err != nil {
		return nil, err
	}
	if pid == 0 {
		// Port already answered by an existing process. Only trust the recorded
		// state if its PID is still alive; otherwise it is a leftover from a
		// Chrome that died and must not be reported as the running instance.
		st, err := ReadState()
		if err != nil {
			return nil, err
		}
		if st != nil && processAlive(st.PID) {
			return st, nil
		}
		// The recorded PID is dead (or absent) yet the port still answers: an
		// untracked Chrome holds it. Resolve the real listener so StopBrowser
		// can act on it — persisting PID 0 would fail the profile check and
		// leave that Chrome unstoppable.
		livePID, err := adoptListenerPID(port)
		if err != nil {
			return nil, err
		}
		st = &BrowserState{PID: livePID, Port: port, StartedAt: time.Now(), Headless: headless}
		if err := writeState(st); err != nil {
			return nil, err
		}
		return st, nil
	}
	st := &BrowserState{PID: pid, Port: port, StartedAt: time.Now(), Headless: headless}
	if err := writeState(st); err != nil {
		return nil, err
	}
	return st, nil
}

// StopBrowser terminates the recorded PID after verifying its command line
// references the htrcli profile (PID-reuse guard), then removes the state file.
func StopBrowser() error {
	st, err := ReadState()
	if err != nil {
		return err
	}
	if st == nil {
		return errors.New("no browser state file — nothing to stop")
	}
	out, err := processCommandLine(st.PID)
	if err == nil && strings.Contains(string(out), ".htrcli/chrome-profile") {
		if err := terminateProcess(st.PID); err != nil {
			return fmt.Errorf("killing pid %d: %w", st.PID, err)
		}
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "[htrcli] pid %d not found (%v) — cleaning up state file\n", st.PID, err)
	} else {
		fmt.Fprintf(os.Stderr, "[htrcli] pid %d is not the htrcli Chrome (%s) — refusing to kill, cleaning up state file\n", st.PID, strings.TrimSpace(string(out)))
	}
	path, err := StateFilePath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing %s: %w", path, err)
	}
	return nil
}
