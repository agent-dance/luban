package tools

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/agent-dance/luban/i18n"
)

// fdRealPath returns the actual filesystem path for an open file descriptor.
// This is used to close the TOCTOU window between path resolution and file open:
// we open the file, then ask the kernel what the fd actually points to.
// Platform-specific implementations are in fd_path_linux.go and fd_path_darwin.go.

// validatePathInDirs checks if resolvedPath is within any of the allowedDirs.
// This is the pure path-check logic shared by checkAllowedPath and fd-based verification.
func validatePathInDirs(resolvedPath string, allowedDirs []string) error {
	resolvedPath = canonicalPathForComparison(resolvedPath)
	for _, dir := range allowedDirs {
		absDir, err := filepath.Abs(dir)
		if err != nil {
			continue
		}
		if rd, err := filepath.EvalSymlinks(absDir); err == nil {
			absDir = rd
		}
		absDir = canonicalPathForComparison(absDir)
		if strings.HasPrefix(resolvedPath, absDir+string(os.PathSeparator)) || resolvedPath == absDir {
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

func canonicalPathForComparison(path string) string {
	return normalizeDarwinPrivatePath(filepath.Clean(path))
}

func displayPathForUser(path string) string {
	return normalizeDarwinPrivatePath(filepath.Clean(path))
}

func normalizeDarwinPrivatePath(path string) string {
	if runtime.GOOS != "darwin" {
		return path
	}
	for _, prefix := range []string{"var", "tmp", "etc"} {
		privatePrefix := string(filepath.Separator) + "private" + string(filepath.Separator) + prefix
		publicPrefix := string(filepath.Separator) + prefix
		if path == privatePrefix {
			return publicPrefix
		}
		withSep := privatePrefix + string(filepath.Separator)
		if strings.HasPrefix(path, withSep) {
			return publicPrefix + string(filepath.Separator) + strings.TrimPrefix(path, withSep)
		}
	}
	return path
}
