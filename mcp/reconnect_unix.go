//go:build !windows

package mcp

import "syscall"

func sigZero() syscall.Signal { return syscall.Signal(0) }
