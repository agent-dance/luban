//go:build !windows

package mcp

import "syscall"

func sigTerm() syscall.Signal { return syscall.SIGTERM }
