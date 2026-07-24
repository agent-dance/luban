//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package session

import (
	"errors"
	"io/fs"
	"os"
	"syscall"
)

func openPathNoFollow(path string, flag int, perm fs.FileMode, directory bool) (*os.File, error) {
	flag |= syscall.O_CLOEXEC | syscall.O_NOFOLLOW
	if directory {
		flag |= syscall.O_DIRECTORY
	} else {
		// A hostile FIFO must not block the process before Fstat can reject it.
		flag |= syscall.O_NONBLOCK
	}
	fd, err := syscall.Open(path, flag, uint32(perm.Perm()))
	if err != nil {
		if errors.Is(err, syscall.ELOOP) || directory && errors.Is(err, syscall.ENOTDIR) {
			err = fs.ErrInvalid
		}
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(fd), path), nil
}

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
