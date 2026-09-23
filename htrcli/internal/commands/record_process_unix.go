//go:build !windows

package commands

import (
	"os/exec"
	"strconv"
	"syscall"
)

func recordConfigureDetachedProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

func recordProcessAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

func recordProcessCommandLine(pid int) ([]byte, error) {
	return exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "command=").Output()
}

func recordTerminateProcess(pid int, force bool) error {
	signal := syscall.SIGTERM
	if force {
		signal = syscall.SIGKILL
	}
	// A PID that exited between the liveness probe and the signal is a
	// success, not an error (mirrors cdp.terminatePID).
	if err := syscall.Kill(pid, signal); err != nil && err != syscall.ESRCH {
		return err
	}
	return nil
}
