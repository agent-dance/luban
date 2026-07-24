//go:build !windows

package tools

import (
	"os"
	"syscall"
)

func syscallZeroSignal() os.Signal { return syscall.Signal(0) }
func runtimeIsWindows() bool       { return false }
