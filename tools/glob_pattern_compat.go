// Package tools — glob_pattern_compat.go provides minimatch-compat preprocessing
// for the doublestar/v4 glob engine.
//
// Two minimatch features have no native equivalent in doublestar:
//   - bracket negation `[!abc]` — doublestar uses `[^abc]`
//   - extglob alternation `+(foo|bar)`, `@(foo|bar)`, etc. — doublestar uses
//     `{foo,bar}` brace expansion
//
// translateMinimatchPattern rewrites the input so doublestar sees a pattern it
// can evaluate. The translation is best-effort and intentionally conservative:
// only the two forms above are touched, everything else passes through verbatim.
package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// translateMinimatchPattern rewrites minimatch-only constructs into the
// doublestar dialect. Returns the input unchanged when no translation
// applies.
func translateMinimatchPattern(pattern string) string {
	if pattern == "" {
		return pattern
	}
	pattern = translateBracketNegation(pattern)
	pattern = translateExtGlobAlternation(pattern)
	return pattern
}

// translateBracketNegation rewrites `[!abc]` → `[^abc]`. The TS minimatch
// reference treats `!` as the negation marker inside character classes;
// doublestar/v4 only understands the POSIX `^` form.
func translateBracketNegation(pattern string) string {
	var b strings.Builder
	b.Grow(len(pattern))
	i := 0
	for i < len(pattern) {
		c := pattern[i]
		if c == '\\' && i+1 < len(pattern) {
			// Preserve escaped characters verbatim — the next byte is literal.
			b.WriteByte(c)
			b.WriteByte(pattern[i+1])
			i += 2
			continue
		}
		if c == '[' && i+1 < len(pattern) && pattern[i+1] == '!' {
			b.WriteByte('[')
			b.WriteByte('^')
			i += 2
			continue
		}
		b.WriteByte(c)
		i++
	}
	return b.String()
}

// translateExtGlobAlternation rewrites the simple extglob forms
// `+(a|b)`, `@(a|b)`, `?(a|b)`, `*(a|b)`, `!(a|b)` (only the first four are
// translated; `!(...)` is intentionally left alone because it represents a
// negation that brace expansion can't model). The translation only fires
// when the alternation body contains no nested parens — anything more
// complex passes through unchanged.
func translateExtGlobAlternation(pattern string) string {
	var b strings.Builder
	b.Grow(len(pattern))
	i := 0
	for i < len(pattern) {
		c := pattern[i]
		if c == '\\' && i+1 < len(pattern) {
			b.WriteByte(c)
			b.WriteByte(pattern[i+1])
			i += 2
			continue
		}
		if (c == '+' || c == '@' || c == '?' || c == '*') && i+1 < len(pattern) && pattern[i+1] == '(' {
			closing := indexMatchingParen(pattern, i+1)
			if closing > i+1 {
				body := pattern[i+2 : closing]
				if strings.Contains(body, "|") && !strings.ContainsAny(body, "()") {
					alts := strings.Split(body, "|")
					b.WriteByte('{')
					b.WriteString(strings.Join(alts, ","))
					b.WriteByte('}')
					i = closing + 1
					continue
				}
			}
		}
		b.WriteByte(c)
		i++
	}
	return b.String()
}

// indexMatchingParen returns the index of the `)` that closes the `(` at
// position openAt. Returns -1 when no balanced match is found. Escaped parens
// (preceded by `\`) are skipped.
func indexMatchingParen(s string, openAt int) int {
	depth := 0
	for i := openAt; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			continue
		}
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// globPatternNeedsNativeMatch reports whether a glob pattern must be evaluated
// by the in-process doublestar matcher rather than ripgrep. ripgrep accepts
// most patterns directly, but bracket character classes (`[abc]` / `[^abc]`)
// can't be passed reliably across every platform — Windows in particular
// applies syscall-level argv quoting that mangles the brackets before they
// reach ripgrep. We route those patterns through the native walker, which
// uses doublestar (the same engine TS minimatch is modelled on).
func globPatternNeedsNativeMatch(pattern string) bool {
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		if c == '\\' && i+1 < len(pattern) {
			i++
			continue
		}
		if c == '[' {
			return true
		}
	}
	return false
}

// runGlobWithDoublestar walks the filesystem rooted at cwd and returns paths
// that match the supplied pattern using the doublestar engine. Behaviour
// mirrors runGlobWithRipgrep:
//   - results are returned with display paths (relativised against cwd when
//     possible) so the caller's metadata stays consistent;
//   - results are sorted by mtime before truncation, matching ripgrep
//     --sort=modified;
//   - truncation kicks in once `limit` is exceeded;
//   - VCS metadata directories (.git, .hg, …) are skipped to match the
//     `--no-ignore` + `--hidden` ripgrep configuration of the primary path.
//
// The walker excludes directories themselves (only files are emitted) so the
// output matches ripgrep `--files`.
func runGlobWithDoublestar(ctx context.Context, pattern string, cwd string, limit int, runtime searchRuntimeSnapshot) (globSearchResult, error) {
	searchDir := cwd
	searchPattern := pattern
	if filepath.IsAbs(pattern) {
		baseDir, relativePattern := extractGlobBaseDirectory(pattern)
		if baseDir != "" {
			allowedBase, err := ensureSearchRootAllowed(baseDir, runtime.cwd, runtime.allowedDirs)
			if err != nil {
				return globSearchResult{}, err
			}
			searchDir = allowedBase
			searchPattern = relativePattern
		}
	}

	matches := make([]string, 0)
	err := filepath.Walk(searchDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			if os.IsPermission(walkErr) {
				if info != nil && info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			return walkErr
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if info == nil {
			return nil
		}
		if info.IsDir() {
			if isVCSDirectory(info.Name()) && path != searchDir {
				return filepath.SkipDir
			}
			return nil
		}
		ok, err := MatchGlobRelativeTo(searchPattern, path, searchDir)
		if err != nil || !ok {
			return nil
		}
		matches = append(matches, path)
		return nil
	})
	if err != nil {
		return globSearchResult{}, err
	}

	// glob-permission-ignore-patterns: drop paths denied by file-read
	// deny rules before truncating to the limit.
	matches = runtime.ignores.filter(matches)

	sortGlobAbsolutePathsByMtime(matches, true)
	truncated := len(matches) > limit
	if truncated {
		matches = matches[:limit]
	}

	out := make([]string, 0, len(matches))
	for _, p := range matches {
		out = append(out, formatSearchDisplayPathFrom(p, runtime.cwd))
	}
	return globSearchResult{Files: out, Truncated: truncated}, nil
}
