//go:build windows

package cdp

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func configureDetachedProcess(_ *exec.Cmd) {}

func terminatePID(pid int, _ bool) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Kill()
}

// processAlive reports whether pid is a live process. It parses tasklist's
// CSV output and compares the PID column instead of matching a localized
// "no tasks" message, so it behaves the same on non-English Windows installs.
func processAlive(pid int) bool {
	// pid 0 / negative pids never identify a recorded process.
	if pid <= 0 {
		return false
	}
	out, err := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV", "/NH").Output()
	if err != nil {
		return false
	}
	// CSV columns: "Image Name","PID","Session Name","Session#","Mem Usage"
	fields := strings.Split(strings.TrimSpace(string(out)), ",")
	if len(fields) < 2 {
		return false
	}
	return strings.Trim(strings.TrimSpace(fields[1]), `"`) == strconv.Itoa(pid)
}

// processListeningPID returns the pid of the process listening on the given
// TCP port, parsed from netstat's LISTENING rows. Used to adopt a browser whose
// recorded pid went stale so it stays stoppable.
func processListeningPID(port int) (int, error) {
	out, err := exec.Command("netstat", "-ano", "-p", "tcp").Output()
	if err != nil {
		return 0, fmt.Errorf("netstat -ano -p tcp: %w", err)
	}
	suffix := ":" + strconv.Itoa(port)
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		// Proto  Local Address  Foreign Address  State  PID
		if len(fields) < 5 || !strings.EqualFold(fields[3], "LISTENING") {
			continue
		}
		if !strings.HasSuffix(fields[1], suffix) {
			continue
		}
		pid, err := strconv.Atoi(fields[4])
		if err != nil {
			continue
		}
		return pid, nil
	}
	return 0, fmt.Errorf("no listener found on port %d", port)
}

// processCommandLine returns the command line for pid. PowerShell's CIM query
// is the primary source because wmic is deprecated and removed on Windows 11
// 24H2+; wmic is kept as a fallback for older hosts.
func processCommandLine(pid int) ([]byte, error) {
	script := fmt.Sprintf("(Get-CimInstance Win32_Process -Filter 'ProcessId=%d').CommandLine", pid)
	if out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script).Output(); err == nil && strings.TrimSpace(string(out)) != "" {
		return out, nil
	}
	return exec.Command("wmic", "process", "where", "ProcessId="+strconv.Itoa(pid), "get", "CommandLine", "/value").Output()
}
