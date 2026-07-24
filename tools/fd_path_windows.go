//go:build windows

package tools

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/sys/windows"
)

// fdRealPath returns the actual filesystem path for an open file descriptor on
// Windows by asking the kernel for the final path of the underlying handle.
func fdRealPath(f *os.File) (string, error) {
	buf := make([]uint16, windows.MAX_PATH)
	for {
		n, err := windows.GetFinalPathNameByHandle(windows.Handle(f.Fd()), &buf[0], uint32(len(buf)), 0)
		if err != nil {
			return "", err
		}
		if int(n) < len(buf) {
			runtime.KeepAlive(f)
			return normalizeWindowsFinalPath(windows.UTF16ToString(buf[:n]))
		}
		buf = make([]uint16, n+1)
	}
}

func normalizeWindowsFinalPath(path string) (string, error) {
	switch {
	case strings.HasPrefix(path, `\\?\UNC\`):
		path = `\\` + strings.TrimPrefix(path, `\\?\UNC\`)
	case strings.HasPrefix(path, `\\?\`):
		path = strings.TrimPrefix(path, `\\?\`)
	}
	return filepath.EvalSymlinks(path)
}
