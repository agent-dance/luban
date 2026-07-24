//go:build !windows

package tools

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func prepareBackgroundCommand(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

func configureCommandCancellation(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	prepareBackgroundCommand(cmd)
	cmd.Cancel = func() error {
		return terminateBackgroundProcess(cmd.Process)
	}
	cmd.WaitDelay = stopGracePeriod
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
