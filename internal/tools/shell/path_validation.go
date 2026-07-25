package shell

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/tools/toolbase"
	"mvdan.cc/sh/v3/syntax"
)

// ExtractPathsFromCommand returns every literal path-like argument referenced
// in `cmd`. It walks the shell AST so quoted strings, redirect targets, and
// command-substitution arguments are all surfaced. Variable expansions and
// other dynamic words are ignored (they cannot be statically validated).
//
// Returned paths are kept verbatim — relative paths are not resolved against
// CWD. Callers can resolve themselves via resolvePathsAgainstCWD.
func ExtractPathsFromCommand(cmd string) []string {
	if strings.TrimSpace(cmd) == "" {
		return nil
	}
	prog, err := syntax.NewParser(syntax.KeepComments(false)).Parse(strings.NewReader(cmd), "")
	if err != nil {
		return nil
	}
	seen := make(map[string]struct{})
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		// Heuristic: skip flag-like tokens and obviously non-path arguments.
		if !looksLikePath(s) {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}

	syntax.Walk(prog, func(node syntax.Node) bool {
		switch n := node.(type) {
		case *syntax.CallExpr:
			// The first word selects the executable. It is permission/security
			// input, not a data path constrained by allowed_dirs.
			if len(n.Args) <= 1 {
				break
			}
			for _, w := range n.Args[1:] {
				if lit := wordToString(w); lit != "" {
					add(lit)
				}
			}
		case *syntax.Stmt:
			for _, redir := range n.Redirs {
				if redir.Word != nil {
					if lit := wordToString(redir.Word); lit != "" {
						add(lit)
					}
				}
			}
		}
		return true
	})
	return out
}

// looksLikePath returns true if `s` plausibly identifies a filesystem path.
// We exclude flag tokens, URLs (handled separately), and pure identifiers.
func looksLikePath(s string) bool {
	if s == "" {
		return false
	}
	if strings.HasPrefix(s, "-") {
		return false
	}
	if strings.Contains(s, "://") {
		return false
	}
	// Treat anything containing a path separator, "." extension, "/" or "\\"
	// as a candidate path; also '~' (home expansion).
	switch {
	case strings.HasPrefix(s, "/"):
		return true
	case strings.HasPrefix(s, "./"), strings.HasPrefix(s, "../"):
		return true
	case strings.HasPrefix(s, "~"):
		return true
	case strings.HasPrefix(s, `\`):
		return true
	case strings.Contains(s, "/"):
		return true
	case strings.Contains(s, `\`) && !strings.HasPrefix(s, "--"):
		return true
	case strings.Contains(s, "."):
		// "foo.txt" is plausibly a path (relative file).
		// Skip simple identifiers like "v1.0" — we still classify these as
		// path-like; ValidatePathsAgainstAllowedDirs only fires when the
		// resolved file actually escapes the allowed dirs.
		return true
	}
	return false
}

// resolvePathsAgainstCWD resolves each relative path against `cwd` and
// expands a leading "~/" to the user's home directory.
func resolvePathsAgainstCWD(paths []string, cwd string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if strings.HasPrefix(p, "~") {
			if home, err := os.UserHomeDir(); err == nil {
				p = filepath.Join(home, strings.TrimPrefix(p, "~"))
			}
		}
		if !filepath.IsAbs(p) && cwd != "" {
			p = filepath.Join(cwd, p)
		}
		// Clean to collapse .. and . segments.
		out = append(out, filepath.Clean(p))
	}
	return out
}

// FilterBashPathScopeExemptions removes standard process devices that do not
// grant filesystem access. They are valid shell sinks/sources even when the
// workspace is restricted to explicit directories.
func FilterBashPathScopeExemptions(paths []string) []string {
	filtered := make([]string, 0, len(paths))
	for _, path := range paths {
		cleaned := filepath.Clean(strings.TrimSpace(path))
		switch cleaned {
		case filepath.Clean(os.DevNull), "/dev/stdin", "/dev/stdout", "/dev/stderr":
			continue
		}
		filtered = append(filtered, path)
	}
	return filtered
}

// ValidatePathsAgainstAllowedDirs returns an error describing the first path
// in `paths` that is outside every entry of `allowedDirs`. allowedDirs may be
// empty (no restriction). Paths and allowedDirs are compared as cleaned
// absolute paths after symlink-free Clean().
func ValidatePathsAgainstAllowedDirs(paths []string, allowedDirs []string) error {
	if len(allowedDirs) == 0 {
		return nil
	}
	cleanedAllowed := make([]string, 0, len(allowedDirs))
	for _, d := range allowedDirs {
		if d == "" {
			continue
		}
		d = expandHomePath(d)
		abs, err := filepath.Abs(d)
		if err != nil {
			abs = d
		}
		cleanedAllowed = append(cleanedAllowed, resolveExistingPathPrefix(abs))
	}
	for _, p := range paths {
		p = expandHomePath(p)
		abs, err := filepath.Abs(p)
		if err != nil {
			abs = p
		}
		abs = resolveExistingPathPrefix(abs)
		if !toolbase.PathWithinAllowedDirs(abs, cleanedAllowed) {
			return i18n.NewError(i18n.KeyToolPathOutsideAllowed, p)
		}
	}
	return nil
}

func expandHomePath(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			if path == "~" {
				return home
			}
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// resolveExistingPathPrefix resolves symlinks for the longest existing prefix
// and reattaches any not-yet-created suffix. This protects both read paths and
// output targets without requiring the final file to exist.
func resolveExistingPathPrefix(path string) string {
	path = filepath.Clean(path)
	current := path
	var suffix []string
	for {
		if resolved, err := filepath.EvalSymlinks(current); err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return filepath.Clean(resolved)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return path
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}
