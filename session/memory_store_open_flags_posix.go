//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package session

import "syscall"

func memoryStoreNonblockFlag() int {
	return syscall.O_NONBLOCK
}
