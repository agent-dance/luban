//go:build windows

package tools

import (
	"os"
	"os/exec"
)

func prepareBackgroundCommand(_ *exec.Cmd) {}

func configureCommandCancellation(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.Cancel = func() error {
		return terminateBackgroundProcess(cmd.Process)
	}
	cmd.WaitDelay = stopGracePeriod
}

func terminateBackgroundProcess(process *os.Process) error {
	if process == nil {
		return nil
	}
	return process.Kill()
}

func forceKillBackgroundProcess(process *os.Process) error {
	if process == nil {
		return nil
	}
	return process.Kill()
}
