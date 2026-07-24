//go:build !darwin && !linux

package tools

import (
	"io/fs"
	"os"
)

// Platforms without descriptor-relative no-follow opens and a trustworthy
// link-count contract fail closed. In particular, os.Chmod/os.OpenFile on
// Windows can follow a reparse point and POSIX-looking 0600 bits do not prove
// a private ACL, so history contents are never persisted through that fallback.
func appendPrivateFileHistory(root, _ string, _ []byte) error {
	return &os.PathError{Op: "append", Path: root, Err: fs.ErrPermission}
}

func readPrivateFileHistory(root, _ string) ([]byte, bool, error) {
	return nil, false, &os.PathError{Op: "read", Path: root, Err: fs.ErrPermission}
}
