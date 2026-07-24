//go:build !windows

package tui

import (
	"os"
	"syscall"
)

func appTerminationSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP}
}
