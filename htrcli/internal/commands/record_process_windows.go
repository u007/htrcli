//go:build windows

package commands

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// recordProcessAlive reports whether pid is a live process. It parses
// tasklist's CSV output and compares the PID column instead of matching a
// localized "no tasks" message, so it works on non-English Windows installs.
func recordProcessAlive(pid int) bool {
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

func recordConfigureDetachedProcess(_ *exec.Cmd) {}

// recordProcessCommandLine returns the command line for pid. PowerShell's CIM
// query is the primary source because wmic is deprecated and removed on
// Windows 11 24H2+; wmic is kept as a fallback for older hosts.
func recordProcessCommandLine(pid int) ([]byte, error) {
	script := fmt.Sprintf("(Get-CimInstance Win32_Process -Filter 'ProcessId=%d').CommandLine", pid)
	if out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script).Output(); err == nil && strings.TrimSpace(string(out)) != "" {
		return out, nil
	}
	return exec.Command("wmic", "process", "where", "ProcessId="+strconv.Itoa(pid), "get", "CommandLine", "/value").Output()
}

func recordTerminateProcess(pid int, _ bool) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Kill()
}
