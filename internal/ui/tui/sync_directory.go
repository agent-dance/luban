//go:build !windows

package tui

import (
	"io/fs"
	"os"
)

func privateFilePermissionsValid(info fs.FileInfo) bool {
	return info != nil && info.Mode().Perm()&0o077 == 0
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	info, err := dir.Stat()
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return &os.PathError{Op: "sync", Path: path, Err: fs.ErrInvalid}
	}
	return dir.Sync()
}
