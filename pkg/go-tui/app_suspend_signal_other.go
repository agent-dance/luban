//go:build !windows && !linux

package tui

import "syscall"

var sendProcessSuspend = func() error {
	return syscall.Kill(syscall.Getpid(), syscall.SIGTSTP)
}
