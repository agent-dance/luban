// Package tools — file_edit_suggest_similar.go: walks the cwd looking for
// files whose basename / full path looks like a near-miss of the path the
// caller passed. Used by FileEditTool when an edit references a path that
// does not exist (and is therefore not a creation).
//
// Mirrors src/tools/FileEditTool: findSimilarFile + suggestPathUnderCwd —
// the model error-recovery loop is significantly faster when the
// "Did you mean?" hint is present.
//
// The walker is bounded so a giant repo cannot stall a single edit:
//   - depth ≤ 4 directory levels below cwd
//   - ≤ 5,000 files inspected
//   - skips well-known noise dirs (.git, node_modules, etc.)
package tools

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/agent-dance/luban/i18n"
)

// suggestSimilarPath returns up to maxSuggestions absolute paths under cwd
// whose basename closely matches filepath.Base(missingPath). Returns nil
// when no candidates clear the similarity threshold or when cwd is empty.
//
// Matching strategy:
//  1. Exact basename match in any walked subdirectory.
//  2. Case-insensitive basename match.
//  3. Same basename stem with a different extension.
//  4. Substring containment in either direction.
//
// Results are de-duplicated and ordered by quality (1 before 2 before 3 before 4).
func suggestSimilarPath(cwd, missingPath string) []string {
	if cwd == "" || missingPath == "" {
		return nil
	}
	const (
		maxSuggestions = 3
		maxDepth       = 4
		maxFiles       = 5_000
	)
	targetBase := filepath.Base(missingPath)
	target := strings.ToLower(targetBase)
	targetStem := strings.ToLower(strings.TrimSuffix(targetBase, filepath.Ext(targetBase)))
	if target == "" || target == "." {
		return nil
	}
	var (
		exact   []string
		caseHit []string
		stemHit []string
		partial []string
		visited int
	)

	walk := func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // tolerate permission errors silently
		}
		if visited > maxFiles {
			return filepath.SkipDir
		}
		// Compute depth relative to cwd.
		rel, relErr := filepath.Rel(cwd, path)
		if relErr == nil {
			depth := strings.Count(rel, string(os.PathSeparator))
			if depth > maxDepth {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == ".venv" ||
				name == "__pycache__" || name == "dist" || name == "build" ||
				name == ".cache" || name == "vendor" || name == ".next" {
				return filepath.SkipDir
			}
			return nil
		}
		visited++
		base := d.Name()
		baseLower := strings.ToLower(base)
		baseStem := strings.ToLower(strings.TrimSuffix(base, filepath.Ext(base)))
		if baseLower == target {
			if base == filepath.Base(missingPath) {
				exact = append(exact, path)
			} else {
				caseHit = append(caseHit, path)
			}
			return nil
		}
		if targetStem != "" && baseStem == targetStem {
			stemHit = append(stemHit, path)
			return nil
		}
		// Substring containment in either direction.
		if strings.Contains(baseLower, target) || strings.Contains(target, baseLower) {
			partial = append(partial, path)
		}
		// Long paths: bail out early on a hit count.
		if len(exact)+len(caseHit) >= maxSuggestions {
			return filepath.SkipDir
		}
		return nil
	}
	_ = filepath.WalkDir(cwd, walk)

	out := make([]string, 0, maxSuggestions)
	push := func(src []string) {
		for _, p := range src {
			if len(out) >= maxSuggestions {
				return
			}
			if !suggestSliceContains(out, p) {
				out = append(out, p)
			}
		}
	}
	push(exact)
	push(caseHit)
	push(stemHit)
	push(partial)
	if len(out) == 0 {
		return nil
	}
	for i, p := range out {
		out[i] = displayPathForUser(p)
	}
	return out
}

func suggestSliceContains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// formatDidYouMean produces the user-facing "Did you mean?" suffix used in
// the ENOENT error path. Returns "" when no suggestions were produced.
func formatDidYouMean(missingPath string, suggestions []string) string {
	if len(suggestions) == 0 {
		return ""
	}
	return toolRuntimeFormat(i18n.KeyToolRuntimeEditDidYouMean, strings.Join(suggestions, ", "))
}
