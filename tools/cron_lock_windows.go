//go:build windows

package tools

import (
	"os"
)

// On Windows, syscall.Signal(0) is unsupported via os/exec process handles
// so we use os.Interrupt as a noop signal that triggers the FindProcess
// liveness check without actually delivering a signal.
func syscallZeroSignal() os.Signal { return os.Interrupt }
func runtimeIsWindows() bool       { return true }
