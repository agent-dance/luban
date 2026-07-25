package toolbase

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// CanonicalPath normalizes a filesystem path for containment comparisons.
// On Darwin it also removes the kernel's /private alias for public system
// directories so the same path has one comparison form.
func CanonicalPath(path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS != "darwin" {
		return path
	}
	for _, prefix := range []string{"var", "tmp", "etc"} {
		privatePrefix := string(filepath.Separator) + "private" + string(filepath.Separator) + prefix
		publicPrefix := string(filepath.Separator) + prefix
		if path == privatePrefix {
			return publicPrefix
		}
		withSeparator := privatePrefix + string(filepath.Separator)
		if strings.HasPrefix(path, withSeparator) {
			return publicPrefix + string(filepath.Separator) + strings.TrimPrefix(path, withSeparator)
		}
	}
	return path
}

// DisplayPath returns the stable user-facing representation of a path.
func DisplayPath(path string) string {
	return CanonicalPath(path)
}

// PathContains reports whether candidate is root itself or a descendant.
func PathContains(root, candidate string) bool {
	root = CanonicalPath(root)
	candidate = CanonicalPath(candidate)
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	if relative == "." {
		return true
	}
	return relative != ".." &&
		!filepath.IsAbs(relative) &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

// PathWithinAllowedDirs checks containment against canonical absolute roots.
// Symlinks in the longest existing prefix are resolved for both candidate and
// roots; a not-yet-created suffix is then reattached to the resolved prefix.
func PathWithinAllowedDirs(candidate string, allowedDirs []string) bool {
	if len(allowedDirs) == 0 {
		return true
	}
	candidate, ok := canonicalScopePath(candidate)
	if !ok {
		return false
	}
	for _, root := range allowedDirs {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		root, ok = canonicalScopePath(root)
		if !ok {
			continue
		}
		if PathContains(root, candidate) {
			return true
		}
	}
	return false
}

func canonicalScopePath(path string) (string, bool) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	abs = filepath.Clean(abs)
	current := abs
	var suffix []string
	for {
		_, err := os.Lstat(current)
		if err == nil {
			resolved, err := filepath.EvalSymlinks(current)
			if err != nil {
				return "", false
			}
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return CanonicalPath(resolved), true
		}
		if !os.IsNotExist(err) {
			return "", false
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", false
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}
