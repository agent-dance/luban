//go:build windows

package shell

import (
	"os/exec"
	"time"
)

func configureCommandCancellation(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return cmd.Process.Kill()
	}
	cmd.WaitDelay = 2 * time.Second
}
