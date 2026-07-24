package cdp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// ContextEntry records one named browser context: an isolated Chrome profile
// launched as its own process on its own debugging port. The default (unnamed)
// context is NOT stored here — it stays on ProfileDir()/browser.json.
type ContextEntry struct {
	Name       string    `json:"name"`
	ProfileDir string    `json:"profile_dir"`
	Port       int       `json:"port"`
	PID        int       `json:"pid"`
	CreatedAt  time.Time `json:"created_at"`
}

var (
	launchChromeFn     = launchChrome
	upsertContextFn    = upsertContext
	terminateProcessFn = terminateProcess
)

// ContextsFilePath returns ~/.htrcli/contexts.json.
func ContextsFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	return filepath.Join(home, ".htrcli", "contexts.json"), nil
}

// ContextProfileDir returns ~/.htrcli/contexts/<name>.
func ContextProfileDir(name string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	return filepath.Join(home, ".htrcli", "contexts", name), nil
}

// ReadContexts returns the persisted registry (nil when the file is absent —
// an expected "no contexts yet" case).
func ReadContexts() ([]ContextEntry, error) {
	path, err := ContextsFilePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil // intentionally not logged: absent registry means "no contexts", an expected case
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var entries []ContextEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return entries, nil
}

// FindContext returns the entry for name (nil, nil when absent).
func FindContext(name string) (*ContextEntry, error) {
	entries, err := ReadContexts()
	if err != nil {
		return nil, err
	}
	for i := range entries {
		if entries[i].Name == name {
			return &entries[i], nil
		}
	}
	return nil, nil
}

// writeContexts persists the registry, sorted by name for a stable file and
// sorted `context list` output.
func writeContexts(entries []ContextEntry) error {
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	path, err := ContextsFilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling contexts: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// upsertContext inserts or replaces the entry for entry.Name.
func upsertContext(entry ContextEntry) error {
	entries, err := ReadContexts()
	if err != nil {
		return err
	}
	replaced := false
	for i := range entries {
		if entries[i].Name == entry.Name {
			entries[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		entries = append(entries, entry)
	}
	return writeContexts(entries)
}

// freePort asks the OS for an unused localhost TCP port by binding :0 and
// reading back the assigned port. There is an inherent TOCTOU gap between
// releasing this port and Chrome binding it; callers launch immediately after
// to minimize it, and PortAlive verification catches a lost race.
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("allocating free port: %w", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// EnsureContext launches (or reuses) the named context's Chrome process and
// returns its debugging port. A context is an isolated --user-data-dir profile
// on its own port, launched as a separate process so it survives across CLI
// invocations and gives true cookie/storage isolation. Chrome-only: the Firefox
// equivalent (a separate `firefox -profile <dir>` process) is out of scope for
// this task and documented as the Firefox fallback in the spec.
func EnsureContext(name, chromePath string, headless bool) (int, error) {
	if name == "" {
		return 0, errors.New("context name must not be empty")
	}
	entry, err := FindContext(name)
	if err != nil {
		return 0, err
	}
	if entry != nil && PortAlive(entry.Port) {
		return entry.Port, nil
	}

	profileDir, err := ContextProfileDir(name)
	if err != nil {
		return 0, err
	}

	// Reuse the recorded (now-dead) port to keep the profile↔port mapping
	// stable across restarts; allocate a fresh one only when never launched.
	port := 0
	if entry != nil {
		port = entry.Port
	}
	if port == 0 {
		port, err = freePort()
		if err != nil {
			return 0, err
		}
	}

	pid, err := launchChromeFn(chromePath, port, profileDir, headless)
	if err != nil {
		return 0, err
	}
	createdAt := time.Now()
	if entry != nil {
		createdAt = entry.CreatedAt
	}
	if err := upsertContextFn(ContextEntry{
		Name:       name,
		ProfileDir: profileDir,
		Port:       port,
		PID:        pid,
		CreatedAt:  createdAt,
	}); err != nil {
		if pid > 0 {
			if killErr := terminateProcessFn(pid); killErr != nil {
				return 0, fmt.Errorf("upserting context %s: %w (cleanup failed: %v)", name, err, killErr)
			}
		}
		return 0, err
	}
	return port, nil
}
