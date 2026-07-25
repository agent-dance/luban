//go:build linux

package file

import (
	"fmt"
	"os"
	"path/filepath"
)

// fdRealPath returns the actual filesystem path for an open file descriptor
// by reading /proc/self/fd/N. This is immune to TOCTOU because it queries
// the kernel's fd table, not the filesystem namespace.
func fdRealPath(f *os.File) (string, error) {
	procPath := fmt.Sprintf("/proc/self/fd/%d", f.Fd())
	target, err := os.Readlink(procPath)
	if err != nil {
		return "", fmt.Errorf("readlink %s: %w", procPath, err)
	}
	return filepath.Clean(target), nil
}
