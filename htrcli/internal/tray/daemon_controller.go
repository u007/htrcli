package tray

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/u007/htrcli/internal/host"
)

// Commander abstracts os/exec so tests can verify which binary/args would be
// used without spawning processes.
type Commander interface {
	Run(name string, args ...string) error
	Output(name string, args ...string) ([]byte, error)
}

type RealCommander struct{}

func (RealCommander) Run(name string, args ...string) error {
	return exec.Command(name, args...).Run()
}

func (RealCommander) Output(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

// daemonController is the real Controller: it wraps *host.Daemon and adds the
// platform-specific shell-outs for maintenance actions.
type daemonController struct {
	d        *host.Daemon
	port     int
	getToken func() string
	getExtID func(browser string) string
	selfPath string
	httpLn   net.Listener
	cmd      Commander
	quit     func()
}

// newDaemonController builds the real Controller.
func NewDaemonController(
	d *host.Daemon,
	port int,
	getToken func() string,
	getExtID func(browser string) string,
	selfPath string,
	httpLn net.Listener,
	cmd Commander,
) Controller {
	return &daemonController{
		d:        d,
		port:     port,
		getToken: getToken,
		getExtID: getExtID,
		selfPath: selfPath,
		httpLn:   httpLn,
		cmd:      cmd,
	}
}

// secretFlags are stripped from the re-exec argv by Restart to avoid leaking
// the bearer token to /proc/<pid>/cmdline. The restarted process re-resolves
// the token from $HTR_BEARER_TOKEN (inherited from the parent) or the config
// file. Add new secret-carrying flags here as they're introduced.
var secretFlags = map[string]bool{
	"--token":    true,
	"--bearer":   true,
	"--password": true, // hypothetical; included for forward-compat
	"--api-key":  true,
}

// stripSecrets removes any secret-carrying flag and its value (or `=value`
// form) from argv.
func stripSecrets(args []string) []string {
	out := make([]string, 0, len(args))
	skip := false
	for _, a := range args {
		if skip {
			skip = false
			continue
		}
		if secretFlags[a] {
			skip = true
			continue
		}
		if strings.HasPrefix(a, "--token=") ||
			strings.HasPrefix(a, "--bearer=") ||
			strings.HasPrefix(a, "--password=") ||
			strings.HasPrefix(a, "--api-key=") {
			continue
		}
		out = append(out, a)
	}
	return out
}

func (c *daemonController) IsRunning() bool { return true }

// Restart closes the HTTP listener, strips secret flags from argv, re-execs
// htrcli serve, then exits the parent. Closing the listener first avoids the
// port-binding race between the old and new process.
func (c *daemonController) Restart() error {
	if c.httpLn != nil {
		_ = c.httpLn.Close()
	}

	raw := os.Args[1:]
	safe := stripSecrets(raw)

	cmd := exec.Command(c.selfPath, safe...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("re-exec: %w", err)
	}
	// Exit the parent. Deferred functions in the parent are skipped — that's
	// intentional: the child now owns the port and is the new daemon.
	os.Exit(0)
	return nil // unreachable
}

// SetQuitFn registers the function called when the user picks Quit. serve.go
// wires this to its shutdown path (e.g. signalling SIGTERM to itself).
func (c *daemonController) SetQuitFn(fn func()) { c.quit = fn }

func (c *daemonController) Quit() error {
	if c.quit != nil {
		c.quit()
	}
	return nil
}

func (c *daemonController) Status() Status {
	s := Status{
		Port:             c.port,
		RelaysConnected:  c.d.RelaysConnected(),
		LastError:        c.d.LastError(),
		TokenFingerprint: Fingerprint(c.getToken()),
	}
	return s
}

func (c *daemonController) RecentLog(n int) []string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return nil
	}
	return tailLines(filepath.Join(home, ".htrcli", "serve.log"), n)
}

func (c *daemonController) ReinstallHost(browser string) error {
	id := c.getExtID(browser)
	if id == "" {
		return fmt.Errorf("no extension ID configured for %s — run `htrcli config set-extension-id <id> --browser %s` first", browser, browser)
	}
	return c.cmd.Run(c.selfPath, "install", "--browser", browser, "--extension-id", id)
}

func (c *daemonController) OpenConfigFolder() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".htrcli")
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("config folder not found at %s", dir)
	}
	return openViaOS(dir)
}

func (c *daemonController) OpenLog() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(home, ".htrcli", "serve.log")
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("no log yet at %s (run htrcli serve with a desktop attached to enable logging)", path)
	}
	return openViaOS(path)
}

func (c *daemonController) CopyTokenToClipboard() (string, error) {
	tok := c.getToken()
	if tok == "" {
		return "", fmt.Errorf("no bearer token set (run `htrcli config set-token <token>` or set HTR_BEARER_TOKEN)")
	}
	if err := copyToClipboard(tok); err != nil {
		return "", err
	}
	// Overwrite the clipboard after 30s so the token doesn't linger.
	go func() {
		time.Sleep(30 * time.Second)
		_ = copyToClipboard("<cleared by htrcli>")
	}()
	return tok, nil
}

// openViaOS opens a path in the OS default application (Finder/Explorer/file
// manager). It does not wait — the GUI app is detached.
func openViaOS(path string) error {
	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		name, args = "open", []string{path}
	case "windows":
		name, args = "explorer", []string{path}
	default: // linux, *bsd
		name, args = "xdg-open", []string{path}
	}
	return exec.Command(name, args...).Start()
}

// copyToClipboard writes text to the system clipboard using the platform
// helper (pbcopy / clip.exe / wl-copy / xclip).
func copyToClipboard(text string) error {
	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		name = "pbcopy"
	case "windows":
		name = "clip.exe"
	default:
		if os.Getenv("WAYLAND_DISPLAY") != "" {
			name = "wl-copy"
		} else {
			name = "xclip"
			args = []string{"-selection", "clipboard"}
		}
	}
	cmd := exec.Command(name, args...)
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

// tailLines returns the last n lines of a file.
//
// CAUTION: reads the entire file into memory (io.ReadAll). For a long-running
// daemon the serve log may grow large; this function assumes the log is either
// rotated externally or stays small enough for an in-memory read. If this
// becomes a problem, replace with a reverse-line reader.
//
// Returns nil if the file cannot be read.
func tailLines(path string, n int) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if n > 0 && len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}
