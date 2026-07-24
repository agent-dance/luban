//go:build windows

package mcp

import "syscall"

// Signal(0) is not meaningful on Windows; use a no-op stand-in.
func sigZero() syscall.Signal { return syscall.Signal(0) }
