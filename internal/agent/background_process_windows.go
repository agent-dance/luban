//go:build windows

package agent

import (
	"os"
	"os/exec"
)

func runtimeIsWindows() bool { return true }

// Windows has no portable zero-signal probe for an arbitrary PID. Callers
// fail closed and reconcile records owned by another process.
func backgroundProcessAlive(int) bool { return false }

func prepareBackgroundCommand(*exec.Cmd) {}

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
