package cdp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// BrowserState is persisted at ~/.htrcli/browser.json. Advisory only: the
// debugging port answering is the source of truth for "running".
type BrowserState struct {
	PID       int       `json:"pid"`
	Port      int       `json:"port"`
	StartedAt time.Time `json:"started_at"`
	Headless  bool      `json:"headless"`
	Args      []string  `json:"args,omitempty"`
	// StderrLog is the file Chrome's stdout and stderr go to. Empty when htrcli
	// did not launch the Chrome this state describes (an adopted listener).
	StderrLog string `json:"stderr_log"`
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

// LaunchArgs builds the Chrome argument list. extra is appended last, after the
// fixed list and --headless; callers validate it with validateExtraChromeArgs
// first. Security: never add --remote-debugging-address — the port must stay
// bound to localhost.
func LaunchArgs(port int, profileDir string, headless bool, extra []string) []string {
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
	return append(args, extra...)
}

// forbiddenChromeSwitches are the switches htrcli owns. The debugging port must
// stay on localhost (see LaunchArgs), and StopBrowser verifies the profile dir
// before it kills a Chrome.
var forbiddenChromeSwitches = []string{
	"remote-debugging-port",
	"remote-debugging-address",
	"user-data-dir",
}

// validateExtraChromeArgs rejects an extra argument that sets a switch htrcli
// owns. Chrome accepts one or two leading dashes, so every leading dash is
// stripped before the name is compared; any =value is ignored.
func validateExtraChromeArgs(extra []string) error {
	for _, arg := range extra {
		name := strings.TrimLeft(arg, "-/")
		if i := strings.IndexByte(name, '='); i >= 0 {
			name = name[:i]
		}
		if slices.Contains(forbiddenChromeSwitches, name) {
			return fmt.Errorf("chrome arg %q is managed by htrcli and cannot be set (not allowed: --remote-debugging-port, --remote-debugging-address, --user-data-dir)", arg)
		}
	}
	return nil
}

// warnExtraArgsIgnored says so when extra args cannot reach a Chrome that was
// already listening on port, so they are never dropped silently.
func warnExtraArgsIgnored(port int, extra []string) {
	if len(extra) == 0 {
		return
	}
	fmt.Fprintf(os.Stderr, "[htrcli] Chrome already answers on port %d; chrome args %q were not applied (run htrcli browser stop, then start again)\n", port, extra)
}

// chromeCandidates lists the Chrome locations to try for goos, in search order.
// A bare name is resolved through PATH by FindChrome. getenv supplies the
// Windows install roots; an unset root contributes no candidate.
func chromeCandidates(goos string, getenv func(string) string) []string {
	switch goos {
	case "darwin":
		return []string{
			macChromePath,
			"/Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		}
	case "linux":
		names := []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser"}
		out := append([]string{}, names...)
		for _, dir := range []string{"/usr/bin", "/snap/bin"} {
			for _, name := range names {
				out = append(out, dir+"/"+name)
			}
		}
		return out
	case "windows":
		var out []string
		for _, root := range []string{getenv("ProgramFiles"), getenv("ProgramFiles(x86)"), getenv("LOCALAPPDATA")} {
			if root == "" {
				continue // unset install root: nothing to search there
			}
			out = append(out, root+`\Google\Chrome\Application\chrome.exe`)
		}
		return out
	default:
		return nil
	}
}

// FindChrome returns the configured binary, else the first Chrome found in the
// candidate list for this OS. A candidate that cannot be run is skipped; the
// not-found error quotes every candidate with the reason it was rejected.
func FindChrome(configured string) (string, error) {
	candidates := chromeCandidates(runtime.GOOS, os.Getenv)
	if configured != "" {
		candidates = append([]string{configured}, candidates...)
	}
	return resolveChrome(configured, candidates)
}

// resolveChrome returns the first candidate that exec.LookPath accepts.
// configured is the user's setting ("" when unset) and, when set, is the first
// candidate. A rejected configured path is reported on stderr, so a fallback to
// a later Chrome never hides the user's own mistake.
func resolveChrome(configured string, candidates []string) (string, error) {
	var rejected []string
	var configuredReason string
	for i, c := range candidates {
		path, err := exec.LookPath(c)
		if err != nil {
			reason := lookPathReason(err)
			rejected = append(rejected, strconv.Quote(c)+": "+reason)
			if i == 0 && configured != "" {
				configuredReason = reason
			}
			continue
		}
		if configuredReason != "" {
			fmt.Fprintf(os.Stderr, "[htrcli] configured Chrome path %q was skipped (%s); using %s\n", configured, configuredReason, path)
		}
		return path, nil
	}
	return "", fmt.Errorf(
		"Chrome binary not found (tried %s) — set it with: htrcli config set-chrome-path <path>",
		strings.Join(rejected, ", "))
}

// lookPathReason says in a few words why exec.LookPath rejected a candidate.
func lookPathReason(err error) string {
	switch {
	case errors.Is(err, exec.ErrNotFound):
		return "not found in PATH"
	case errors.Is(err, fs.ErrNotExist):
		return "no such file"
	case errors.Is(err, syscall.EISDIR):
		return "is a directory"
	case errors.Is(err, fs.ErrPermission):
		return "permission denied"
	default:
		return err.Error()
	}
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
// Chrome's singleton-lock handoff). extra is validated before anything else, so
// a forbidden arg fails even when Chrome is already up.
func launchChrome(chromePath string, port int, profileDir string, headless bool, extra []string) (int, error) {
	if err := validateExtraChromeArgs(extra); err != nil {
		return 0, err
	}
	if PortAlive(port) {
		warnExtraArgsIgnored(port, extra)
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
	// Chrome outlives htrcli, so its stdout and stderr must be a real file, never
	// a pipe: a pipe closed by htrcli would SIGPIPE Chrome later. Truncated per
	// launch of this profile; the profile dir was created above.
	logPath := ChromeStderrLogPath(profileDir)
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return 0, fmt.Errorf("opening Chrome stderr log %s: %w", logPath, err)
	}
	defer func() {
		if err := logFile.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "[htrcli] closing Chrome stderr log %s: %v\n", logPath, err)
		}
	}()
	cmd := exec.Command(chromePath, LaunchArgs(port, profileDir, headless, extra)...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
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
			fmt.Fprintf(os.Stderr, "[htrcli] Chrome exited: %v%s\n", err, stderrSuffix(logPath))
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
	return 0, fmt.Errorf("Chrome (pid %d) did not answer on port %d within %s (killed)%s", pid, port, launchTimeout, stderrSuffix(logPath))
}

// stderrTailBytes caps how much of the Chrome stderr log an error quotes.
const stderrTailBytes = 2048

// ChromeStderrLogPath returns <profileDir>/chrome-stderr.log, where the Chrome
// launched on that profile writes its stdout and stderr. Each profile has its
// own log, so launching one context never truncates another's.
func ChromeStderrLogPath(profileDir string) string {
	return filepath.Join(profileDir, "chrome-stderr.log")
}

// readStderrTail returns the last whole lines of the log, within about
// stderrTailBytes. A cut that lands mid-line drops that partial line, and a
// window holding no complete line returns "".
func readStderrTail(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("opening %s: %w", path, err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "[htrcli] closing %s: %v\n", path, err)
		}
	}()
	info, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("statting %s: %w", path, err)
	}
	start := max(info.Size()-stderrTailBytes, 0)
	buf := make([]byte, info.Size()-start)
	if _, err := f.ReadAt(buf, start); err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}
	text := string(buf)
	if start > 0 {
		i := strings.IndexByte(text, '\n')
		if i < 0 {
			return "", nil
		}
		text = text[i+1:]
	}
	return strings.TrimRight(text, "\n"), nil
}

// stderrSuffix returns the text appended to a Chrome error that quotes the tail
// of the stderr log. A log that cannot be read is reported inside the message,
// never dropped.
func stderrSuffix(path string) string {
	tail, err := readStderrTail(path)
	if err != nil {
		return fmt.Sprintf("\nlast stderr: unavailable: %v", err)
	}
	if tail == "" {
		return "\nlast stderr: (no lines captured)"
	}
	return "\nlast stderr:\n" + tail
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

// StartBrowser launches the default-profile Chrome with extra arguments, waits
// for the port, and persists advisory state to browser.json. Args records the
// argv this launch used; an adopted listener's argv is unknown, so Args stays empty.
func StartBrowser(chromePath string, port int, headless bool, extra []string) (*BrowserState, error) {
	profile, err := ProfileDir()
	if err != nil {
		return nil, err
	}
	pid, err := launchChrome(chromePath, port, profile, headless, extra)
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
	st := &BrowserState{PID: pid, Port: port, StartedAt: time.Now(), Headless: headless, Args: LaunchArgs(port, profile, headless, extra), StderrLog: ChromeStderrLogPath(profile)}
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
