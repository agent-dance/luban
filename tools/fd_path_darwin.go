//go:build darwin

package tools

import (
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

// fdRealPath returns the actual filesystem path for an open fd using
// fcntl(F_GETPATH) on macOS. This is the macOS equivalent of
// /proc/self/fd/N — it asks the kernel for the vnode's path.
func fdRealPath(f *os.File) (string, error) {
	buf := make([]byte, 1024) // MAXPATHLEN on macOS
	_, _, errno := syscall.Syscall(
		syscall.SYS_FCNTL,
		f.Fd(),
		syscall.F_GETPATH,
		uintptr(unsafe.Pointer(&buf[0])),
	)
	if errno != 0 {
		return "", errno
	}
	// buf is null-terminated
	n := 0
	for n < len(buf) && buf[n] != 0 {
		n++
	}
	return displayPathForUser(filepath.Clean(string(buf[:n]))), nil
}
