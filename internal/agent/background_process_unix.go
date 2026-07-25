//go:build !windows

package agent

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func runtimeIsWindows() bool { return false }

func backgroundProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil || process == nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}

func prepareBackgroundCommand(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

func terminateBackgroundProcess(process *os.Process) error {
	if process == nil {
		return nil
	}
	if err := syscall.Kill(-process.Pid, syscall.SIGTERM); err == nil || errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return process.Signal(syscall.SIGTERM)
}

func forceKillBackgroundProcess(process *os.Process) error {
	if process == nil {
		return nil
	}
	if err := syscall.Kill(-process.Pid, syscall.SIGKILL); err == nil || errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return process.Kill()
}
