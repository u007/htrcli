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

// StartBrowser launches Chrome detached, waits for the port, persists state.
// If the port already answers, it records/refreshes state and returns without
// launching (also covers Chrome's singleton-lock handoff to an existing
// profile owner).
func StartBrowser(chromePath string, port int, headless bool) (*BrowserState, error) {
	if PortAlive(port) {
		st, err := ReadState()
		if err != nil || st == nil {
			st = &BrowserState{Port: port, StartedAt: time.Now()}
		}
		return st, nil
	}
	profile, err := ProfileDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(profile, 0700); err != nil {
		return nil, fmt.Errorf("creating profile dir: %w", err)
	}

	cmd := exec.Command(chromePath, LaunchArgs(port, profile, headless)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // detach: survives htrcli exiting
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("launching Chrome %s: %w", chromePath, err)
	}
	// Reap when Chrome eventually exits so a stopped browser never zombies
	// against a still-running htrcli daemon process.
	go func() {
		if err := cmd.Wait(); err != nil {
			fmt.Fprintf(os.Stderr, "[htrcli] Chrome exited: %v\n", err)
		}
	}()

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if PortAlive(port) {
			st := &BrowserState{PID: cmd.Process.Pid, Port: port, StartedAt: time.Now(), Headless: headless}
			if err := writeState(st); err != nil {
				return nil, err
			}
			return st, nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return nil, fmt.Errorf("Chrome (pid %d) did not answer on port %d within 15s", cmd.Process.Pid, port)
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
