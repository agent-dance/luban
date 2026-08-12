package tui

import (
	"io/fs"
	"os"
)

// Windows' os.FileMode permission bits do not represent the file's ACL and
// report ordinary files as 0666 even after Chmod(0600). File identity and type
// checks remain meaningful; treating the synthetic group/other bits as ACLs
// would reject every checkpoint written by this process.
func privateFilePermissionsValid(info fs.FileInfo) bool {
	return info != nil
}

// Windows does not expose the portable directory fsync operation used after
// an atomic rename on Unix. The file itself is already flushed before the
// existing atomic rename. Validate the directory so real path failures remain
// visible, but do not turn a successful publication into Access is denied.
func syncDirectory(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return &os.PathError{Op: "sync", Path: path, Err: fs.ErrInvalid}
	}
	return nil
}
