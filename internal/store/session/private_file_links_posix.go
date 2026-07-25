//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package session

import (
	"io/fs"
	"os"
	"syscall"
)

// validatePrivateRegularFileLinkCount rejects aliases to private session
// files. On POSIX, a managed file is private only while this pathname is its
// sole directory entry; an unexpected stat representation also fails closed.
func validatePrivateRegularFileLinkCount(path, op string, info fs.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil || stat.Nlink != 1 {
		return &os.PathError{Op: op, Path: path, Err: fs.ErrInvalid}
	}
	return nil
}
