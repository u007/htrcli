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
}

const macChromePath = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"

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
	cmd := exec.Command(chromePath, LaunchArgs(port, profileDir, headless)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // detach: survives htrcli exiting
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("launching Chrome %s: %w", chromePath, err)
	}
	// Reap when Chrome eventually exits so a stopped browser never zombies
	// against a still-running htrcli process.
	go func() {
		if err := cmd.Wait(); err != nil {
			fmt.Fprintf(os.Stderr, "[htrcli] Chrome exited: %v\n", err)
		}
	}()

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if PortAlive(port) {
			return cmd.Process.Pid, nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return 0, fmt.Errorf("Chrome (pid %d) did not answer on port %d within 15s", cmd.Process.Pid, port)
}

// terminateProcess sends SIGTERM and, if needed, SIGKILL to a process that was
// just started by htrcli. Best-effort only; dead PIDs are treated as success.
func terminateProcess(pid int) error {
	if pid <= 0 {
		return nil
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("signalling pid %d: %w", pid, err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); err != nil {
			if errors.Is(err, syscall.ESRCH) {
				return nil
			}
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
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
		// Port already answered by an existing process.
		st, err := ReadState()
		if err != nil || st == nil {
			st = &BrowserState{Port: port, StartedAt: time.Now()}
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
	out, err := exec.Command("ps", "-p", strconv.Itoa(st.PID), "-o", "command=").Output()
	if err == nil && strings.Contains(string(out), ".htrcli/chrome-profile") {
		if err := syscall.Kill(st.PID, syscall.SIGTERM); err != nil {
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
