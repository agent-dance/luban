package file

import (
	"os"
	"path/filepath"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/tools/toolbase"
)

// fdRealPath returns the actual filesystem path for an open file descriptor.
// This is used to close the TOCTOU window between path resolution and file open:
// we open the file, then ask the kernel what the fd actually points to.
// Platform-specific implementations are in fd_path_linux.go and fd_path_darwin.go.

// validatePathInDirs checks if resolvedPath is within any of the allowedDirs.
// This is the pure path-check logic shared by checkAllowedPath and fd-based verification.
func validatePathInDirs(resolvedPath string, allowedDirs []string) error {
	resolvedPath = toolbase.CanonicalPath(resolvedPath)
	for _, dir := range allowedDirs {
		absDir, err := filepath.Abs(dir)
		if err != nil {
			continue
		}
		if rd, err := filepath.EvalSymlinks(absDir); err == nil {
			absDir = rd
		}
		absDir = toolbase.CanonicalPath(absDir)
		if toolbase.PathContains(absDir, resolvedPath) {
			return nil
		}
	}
	return i18n.NewError(i18n.KeyToolFileHelperPathOutsideAllowed, resolvedPath)
}

// verifyOpenFd checks that an already-opened file descriptor actually points to
// a path within allowedDirs. Returns nil if allowedDirs is empty (no sandbox).
// This eliminates the TOCTOU window: the check is on the open fd, not a path string.
func verifyOpenFd(f *os.File, allowedDirs []string) error {
	if len(allowedDirs) == 0 {
		return nil
	}
	actualPath, err := fdRealPath(f)
	if err != nil {
		return i18n.WrapError(i18n.KeyToolFileHelperVerifyFDPathFailed, err)
	}
	return validatePathInDirs(actualPath, allowedDirs)
}
