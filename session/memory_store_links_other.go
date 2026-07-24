//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package session

import (
	"io/fs"
	"os"
)

// Platforms without a reliable link-count contract fail closed instead of
// persisting private memory through a path whose aliases cannot be proven.
func validateMemoryRegularFileLinkCount(_ *os.File, path string) error {
	return &os.PathError{Op: "open", Path: path, Err: fs.ErrInvalid}
}
