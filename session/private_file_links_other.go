//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package session

import (
	"io/fs"
	"os"
)

// Platforms without a portable O_NOFOLLOW use an identity check around the
// open. POSIX builds use the atomic implementation in
// private_file_links_posix.go; Windows additionally remains covered by the
// repository's cross-compile gate and native target tests.
func openPathNoFollow(path string, flag int, perm fs.FileMode, directory bool) (*os.File, error) {
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

// FileInfo does not expose a portable link count. Platforms without the
// POSIX Stat_t contract retain the existing regular-file and identity checks.
func validatePrivateRegularFileLinkCount(_, _ string, _ fs.FileInfo) error {
	return nil
}
