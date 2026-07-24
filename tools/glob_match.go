// Package tools — glob_match.go wraps doublestar/v4 to give us a string-based
// glob matcher with TS-minimatch parity (brace expansion {a,b}, globstar **,
// POSIX classes, negation via leading "!"). This is what the GlobTool now uses
// for matching, replacing the lighter custom regex matcher in
// search_fallback.go for the public Compile/Match surface.
//
// The matcher is intentionally string-only so callers don't have to reason
// about Windows backslashes vs forward-slashes — both inputs are normalised
// to forward-slash form before evaluation, matching the way TS minimatch
// behaves on every platform.
package tools

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// CompiledGlob is a parsed glob pattern ready for repeated matching.
// It captures whether the pattern is negated (starts with "!") and the
// underlying doublestar pattern with normalisation already applied.
type CompiledGlob struct {
	pattern string
	negated bool
}

// CompileGlob parses a glob pattern and returns a reusable matcher. Returns an
// error if the pattern is not syntactically valid (per doublestar.ValidatePattern).
func CompileGlob(pattern string) (*CompiledGlob, error) {
	trimmed := strings.TrimSpace(pattern)
	if trimmed == "" {
		return nil, fmt.Errorf("glob pattern is empty")
	}
	negated := false
	if strings.HasPrefix(trimmed, "!") {
		negated = true
		trimmed = strings.TrimPrefix(trimmed, "!")
	}
	normalised := filepath.ToSlash(trimmed)
	if !doublestar.ValidatePattern(normalised) {
		return nil, fmt.Errorf("invalid glob pattern: %s", pattern)
	}
	return &CompiledGlob{pattern: normalised, negated: negated}, nil
}

// IsNegated reports whether the pattern was prefixed with "!".
func (c *CompiledGlob) IsNegated() bool {
	if c == nil {
		return false
	}
	return c.negated
}

// Pattern returns the normalised pattern (without the leading "!").
func (c *CompiledGlob) Pattern() string {
	if c == nil {
		return ""
	}
	return c.pattern
}

// Match reports whether the path matches the compiled pattern. Negation is
// applied — a negated pattern returns true only when the path does NOT match
// the underlying pattern. Paths are normalised to forward-slash form first.
func (c *CompiledGlob) Match(path string) bool {
	if c == nil {
		return false
	}
	candidate := filepath.ToSlash(path)
	matched, err := doublestar.Match(c.pattern, candidate)
	if err != nil {
		return false
	}
	if c.negated {
		return !matched
	}
	return matched
}

// MatchGlob is a one-shot helper: compile + match in a single call. Callers
// matching many paths should prefer CompileGlob + (*CompiledGlob).Match to
// amortise the parse cost.
func MatchGlob(pattern string, path string) (bool, error) {
	compiled, err := CompileGlob(pattern)
	if err != nil {
		return false, err
	}
	return compiled.Match(path), nil
}

// MatchGlobRelativeTo behaves like MatchGlob but resolves path against root
// when path is absolute, so patterns like "src/**/*.go" continue to match
// "<root>/src/foo/bar.go". Patterns without a path separator match against
// the basename only, mirroring TS minimatch behaviour.
func MatchGlobRelativeTo(pattern string, path string, root string) (bool, error) {
	compiled, err := CompileGlob(pattern)
	if err != nil {
		return false, err
	}
	candidate := candidateForPattern(compiled.pattern, path, root)
	matched, err := doublestar.Match(compiled.pattern, candidate)
	if err != nil {
		return false, err
	}
	if compiled.negated {
		return !matched, nil
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

// CombinedGlobMatch evaluates a list of glob patterns (split via
// splitSearchGlobPatterns) against path. Behaviour mirrors TS:
//   - Patterns starting with "!" are exclusions: a single match drops the path.
//   - At least one positive pattern must match if any positive patterns exist.
//   - When no positive patterns are supplied (only negations), every non-excluded
//     path is considered a match.
func CombinedGlobMatch(patterns []string, path string, root string) bool {
	if len(patterns) == 0 {
		return true
	}
	hasPositive := false
	matchedPositive := false
	for _, raw := range patterns {
		pat := strings.TrimSpace(raw)
		if pat == "" {
			continue
		}
		negated := strings.HasPrefix(pat, "!")
		if negated {
			pat = strings.TrimPrefix(pat, "!")
		} else {
			hasPositive = true
		}
		ok, err := MatchGlobRelativeTo(pat, path, root)
		if err != nil {
			continue
		}
		if ok {
			if negated {
				return false
			}
			matchedPositive = true
		}
	}
	return !hasPositive || matchedPositive
}
