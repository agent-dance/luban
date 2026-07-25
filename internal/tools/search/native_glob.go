// Package search contains the repository search tools.
package search

import (
	"context"
	"os"
	"path/filepath"

	"github.com/agent-dance/luban/internal/tools/toolbase"
)

// globPatternNeedsNativeMatch reports whether a glob pattern must be evaluated
// by the in-process doublestar matcher rather than ripgrep. ripgrep accepts
// most patterns directly, but bracket character classes (`[abc]` / `[^abc]`)
// can't be passed reliably across every platform — Windows in particular
// applies syscall-level argv quoting that mangles the brackets before they
// reach ripgrep. We route those patterns through the native walker, which
// uses the same doublestar engine as the rest of the native glob contract.
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
		ok, err := toolbase.MatchGlobRelativeTo(searchPattern, path, searchDir)
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
