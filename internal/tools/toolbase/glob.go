// Package toolbase provides domain-neutral helpers shared by tool packages.
package toolbase

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

type compiledGlob struct {
	pattern string
}

func compileGlob(pattern string) (*compiledGlob, error) {
	trimmed := strings.TrimSpace(pattern)
	if trimmed == "" {
		return nil, fmt.Errorf("glob pattern is empty")
	}
	normalised := filepath.ToSlash(trimmed)
	if !doublestar.ValidatePattern(normalised) {
		return nil, fmt.Errorf("invalid glob pattern: %s", pattern)
	}
	return &compiledGlob{pattern: normalised}, nil
}

func (c *compiledGlob) match(path string) bool {
	if c == nil {
		return false
	}
	candidate := filepath.ToSlash(path)
	matched, err := doublestar.Match(c.pattern, candidate)
	if err != nil {
		return false
	}
	return matched
}

// MatchGlob matches one path with the current doublestar glob contract.
func MatchGlob(pattern string, path string) (bool, error) {
	compiled, err := compileGlob(pattern)
	if err != nil {
		return false, err
	}
	return compiled.match(path), nil
}

// MatchGlobRelativeTo behaves like MatchGlob but resolves path against root
// when path is absolute, so patterns like "src/**/*.go" continue to match
// "<root>/src/foo/bar.go". Patterns without a path separator match against
// the basename only.
func MatchGlobRelativeTo(pattern string, path string, root string) (bool, error) {
	compiled, err := compileGlob(pattern)
	if err != nil {
		return false, err
	}
	candidate := candidateForPattern(compiled.pattern, path, root)
	matched, err := doublestar.Match(compiled.pattern, candidate)
	if err != nil {
		return false, err
	}
	return matched, nil
}

func candidateForPattern(pattern string, path string, root string) string {
	candidate := filepath.ToSlash(path)
	if root != "" {
		normalisedRoot := strings.TrimRight(filepath.ToSlash(root), "/")
		if normalisedRoot != "" {
			if candidate == normalisedRoot {
				candidate = "."
			} else if strings.HasPrefix(candidate, normalisedRoot+"/") {
				candidate = strings.TrimPrefix(candidate, normalisedRoot+"/")
			} else if filepath.IsAbs(path) {
				if rel, err := filepath.Rel(root, path); err == nil {
					candidate = filepath.ToSlash(rel)
				}
			}
		}
	}
	if !strings.Contains(pattern, "/") {
		return filepath.Base(candidate)
	}
	return candidate
}
