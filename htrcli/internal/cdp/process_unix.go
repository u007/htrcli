//go:build !windows

package cdp

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func configureDetachedProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

func terminatePID(pid int, force bool) error {
	signal := syscall.SIGTERM
	if force {
		signal = syscall.SIGKILL
	}
	if err := syscall.Kill(pid, signal); err != nil && err != syscall.ESRCH {
		return err
	}
	return nil
}

func processAlive(pid int) bool {
	// pid 0 addresses the caller's process group and negative pids address
	// process groups too; neither identifies the process we recorded, so they
	// must never be reported as alive (kill(0, 0) succeeds).
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

// processListeningPID returns the pid of the process listening on the given
// TCP port. Used to adopt a browser whose recorded pid went stale so it stays
// stoppable. Relies on lsof, which ships with macOS and most Linux distros.
func processListeningPID(port int) (int, error) {
	out, err := exec.Command("lsof", "-nP", fmt.Sprintf("-iTCP:%d", port), "-sTCP:LISTEN", "-t").Output()
	if err != nil {
		return 0, fmt.Errorf("lsof -iTCP:%d: %w", port, err)
	}
	for _, field := range strings.Fields(string(out)) {
		if pid, err := strconv.Atoi(field); err == nil {
			return pid, nil
		}
	}
	return 0, fmt.Errorf("no listener found on port %d", port)
}

func processCommandLine(pid int) ([]byte, error) {
	return exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "command=").Output()
}
