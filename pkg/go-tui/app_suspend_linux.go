//go:build linux

package tui

import (
	"runtime"
	"syscall"

	"golang.org/x/sys/unix"
)

var linuxSuspendThread = unix.Tgkill

var sendProcessSuspend = func() error {
	// A process-directed SIGTSTP can be delivered on another Go runtime thread,
	// allowing the caller to start terminal resume work before the thread group
	// has stopped. Target the locked calling thread so this syscall returns only
	// after the process is continued.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	return linuxSuspendThread(unix.Getpid(), unix.Gettid(), syscall.SIGTSTP)
}
