//go:build windows

package mcp

import "syscall"

// Windows does not have SIGTERM; use SIGKILL as the best available alternative.
func sigTerm() syscall.Signal { return syscall.SIGKILL }
