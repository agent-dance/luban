//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package compact

import (
	"io/fs"
	"os"
	"syscall"
)

func validateResultPrivateRegularFileLinkCount(path string, info fs.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil || stat.Nlink != 1 {
		return &os.PathError{Op: "open", Path: path, Err: fs.ErrInvalid}
	}
	return nil
}
