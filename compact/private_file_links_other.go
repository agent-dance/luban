//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package compact

import (
	"io/fs"
	"os"
)

func openResultPathNoFollow(path string, flag int, perm fs.FileMode, directory bool) (*os.File, error) {
	before, err := os.Lstat(path)
	if err != nil {
		if !os.IsNotExist(err) || flag&os.O_CREATE == 0 {
			return nil, err
		}
		return os.OpenFile(path, flag, perm)
	}
	if before.Mode()&os.ModeSymlink != 0 || directory != before.IsDir() || !directory && !before.Mode().IsRegular() {
		return nil, &os.PathError{Op: "open", Path: path, Err: fs.ErrInvalid}
	}
	f, err := os.OpenFile(path, flag, perm)
	if err != nil {
		return nil, err
	}
	after, err := f.Stat()
	if err != nil || !os.SameFile(before, after) || directory != after.IsDir() {
		_ = f.Close()
		if err != nil {
			return nil, err
		}
		return nil, &os.PathError{Op: "open", Path: path, Err: fs.ErrInvalid}
	}
	return f, nil
}

func validateResultPrivateRegularFileLinkCount(_ string, _ fs.FileInfo) error {
	return nil
}
