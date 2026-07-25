package search

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/agent-dance/luban/i18n"
)

var errRipgrepUnavailable = errors.New("ripgrep unavailable")

func ripgrepUnavailableError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errRipgrepUnavailable, fmt.Sprintf(format, args...))
}

func isRipgrepUnavailable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errRipgrepUnavailable) {
		return true
	}
	return ripgrepUnavailableText(err.Error())
}

func ripgrepUnavailableText(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "access is denied") ||
		strings.Contains(lower, "permission denied") ||
		strings.Contains(lower, "not found in path") ||
		strings.Contains(lower, "executable file not found") ||
		strings.Contains(lower, "no such file or directory")
}

func runGlobWithFallback(ctx context.Context, pattern string, cwd string, limit int, runtime searchRuntimeSnapshot) (globSearchResult, error) {
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

	matches := make([]searchFile, 0)
	err := walkSearchFiles(ctx, searchDir, false, func(path string, info os.FileInfo) error {
		if matchSearchGlob(searchPattern, path, searchDir) {
			matches = append(matches, searchFile{Path: path, ModTime: info.ModTime().UnixNano()})
		}
		return nil
	})
	if err != nil {
		return globSearchResult{}, err
	}

	// glob-permission-ignore-patterns: drop deny-listed paths.
	if len(runtime.ignores.rules) > 0 || len(runtime.ignores.pluginDirs) > 0 {
		filtered := matches[:0]
		for _, m := range matches {
			if !runtime.ignores.ignored(m.Path) {
				filtered = append(filtered, m)
			}
		}
		matches = filtered
	}

	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].ModTime == matches[j].ModTime {
			return matches[i].Path < matches[j].Path
		}
		return matches[i].ModTime < matches[j].ModTime
	})

	truncated := len(matches) > limit
	if truncated {
		matches = matches[:limit]
	}

	out := make([]string, 0, len(matches))
	for _, match := range matches {
		out = append(out, formatSearchDisplayPathFrom(match.Path, runtime.cwd))
	}
	return globSearchResult{Files: out, Truncated: truncated}, nil
}

func runGrepWithFallback(ctx context.Context, opts grepRipgrepOptions) ([]string, error) {
	re, err := compileGrepPattern(opts.Pattern, opts.CaseInsensitive)
	if err != nil {
		return nil, err
	}
	contextBefore, contextAfter, err := grepFallbackContextWindow(opts)
	if err != nil {
		return nil, err
	}

	files, err := collectGrepFallbackFiles(ctx, opts)
	if err != nil {
		return nil, err
	}

	results := make([]string, 0)
	switch opts.OutputMode {
	case "content":
		for _, file := range files {
			lines, err := grepFallbackContentLines(file.Path, opts, re, contextBefore, contextAfter)
			if err != nil {
				return nil, err
			}
			results = append(results, lines...)
		}
	case "count":
		for _, file := range files {
			count, ok, err := grepFallbackMatchCount(file.Path, re)
			if err != nil {
				return nil, err
			}
			if ok {
				results = append(results, formatGrepFallbackCountLine(file.Path, count, opts))
			}
		}
	case "files_with_matches":
		for _, file := range files {
			ok, err := grepFallbackFileMatches(file.Path, re)
			if err != nil {
				return nil, err
			}
			if ok {
				results = append(results, formatSearchDisplayPathFrom(file.Path, opts.DisplayRoot))
			}
		}
	}
	return paginateSearchResults(results, opts.Offset, opts.HeadLimit, opts.Unlimited), nil
}

type searchFile struct {
	Path    string
	ModTime int64
}

func collectGrepFallbackFiles(ctx context.Context, opts grepRipgrepOptions) ([]searchFile, error) {
	files := make([]searchFile, 0)
	if !opts.SearchPathInfo.IsDir() {
		if grepFallbackFileAllowed(opts.SearchPath, opts) {
			files = append(files, searchFile{
				Path:    opts.SearchPath,
				ModTime: opts.SearchPathInfo.ModTime().UnixNano(),
			})
		}
		return files, nil
	}

	err := walkSearchFiles(ctx, opts.SearchPath, true, func(path string, info os.FileInfo) error {
		if grepFallbackFileAllowed(path, opts) {
			files = append(files, searchFile{Path: path, ModTime: info.ModTime().UnixNano()})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(files, func(i, j int) bool {
		if files[i].ModTime == files[j].ModTime {
			return files[i].Path < files[j].Path
		}
		return files[i].ModTime > files[j].ModTime
	})
	return files, nil
}

func walkSearchFiles(ctx context.Context, root string, excludeVCS bool, visit func(path string, info os.FileInfo) error) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsPermission(err) {
				if info != nil && info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if info == nil {
			return nil
		}
		if info.IsDir() {
			if excludeVCS && isVCSDirectory(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		return visit(path, info)
	})
}

func isVCSDirectory(name string) bool {
	for _, excluded := range vcsDirectoriesToExclude {
		if name == excluded {
			return true
		}
	}
	return false
}

func compileGrepPattern(pattern string, caseInsensitive bool) (*regexp.Regexp, error) {
	if caseInsensitive {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, i18n.WrapError(i18n.KeyToolSourceSinkSearchInvalidRegex, err)
	}
	return re, nil
}

func grepFallbackFileAllowed(path string, opts grepRipgrepOptions) bool {
	if opts.Ignores.ignored(path) {
		return false
	}
	if opts.Type != "" && !matchesFallbackType(path, opts.Type) {
		return false
	}
	if !matchesGrepFallbackGlob(path, opts.SearchPath, opts.Glob) {
		return false
	}
	return true
}

func grepFallbackContextWindow(opts grepRipgrepOptions) (int, int, error) {
	if opts.OutputMode != "content" {
		return 0, 0, nil
	}
	before, after := float64(0), float64(0)
	if opts.ContextSet {
		before, after = opts.Context, opts.Context
	} else {
		if opts.ContextBeforeSet {
			before = opts.ContextBefore
		}
		if opts.ContextAfterSet {
			after = opts.ContextAfter
		}
	}
	if before < 0 || after < 0 || math.Trunc(before) != before || math.Trunc(after) != after {
		return 0, 0, i18n.NewError(i18n.KeyToolSourceSinkSearchInvalidContext)
	}
	return int(before), int(after), nil
}

func matchesFallbackType(path string, typ string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case "go":
		return ext == ".go"
	case "js", "javascript":
		return ext == ".js" || ext == ".mjs" || ext == ".cjs"
	case "jsx":
		return ext == ".jsx"
	case "ts", "typescript":
		return ext == ".ts" || ext == ".mts" || ext == ".cts"
	case "tsx":
		return ext == ".tsx"
	case "py", "python":
		return ext == ".py"
	case "json":
		return ext == ".json"
	case "md", "markdown":
		return ext == ".md" || ext == ".markdown"
	case "java":
		return ext == ".java"
	case "rs", "rust":
		return ext == ".rs"
	case "c":
		return ext == ".c" || ext == ".h"
	case "cpp", "c++":
		return ext == ".cc" || ext == ".cpp" || ext == ".cxx" || ext == ".hpp" || ext == ".hh" || ext == ".hxx"
	case "cs", "csharp":
		return ext == ".cs"
	case "sh", "shell":
		return ext == ".sh" || ext == ".bash" || ext == ".zsh"
	default:
		return ext == "."+strings.ToLower(strings.TrimSpace(typ))
	}
}

func matchesGrepFallbackGlob(path string, root string, raw string) bool {
	patterns := splitSearchGlobPatterns(raw)
	if len(patterns) == 0 {
		return true
	}
	hasPositive := false
	matchedPositive := false
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		negated := strings.HasPrefix(pattern, "!")
		if negated {
			pattern = strings.TrimPrefix(pattern, "!")
		} else {
			hasPositive = true
		}
		if matchSearchGlob(pattern, path, root) {
			if negated {
				return false
			}
			matchedPositive = true
		}
	}
	return !hasPositive || matchedPositive
}

func grepFallbackFileMatches(path string, re *regexp.Regexp) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsPermission(err) {
			return false, nil
		}
		return false, err
	}
	if isBinaryContent(firstSearchChunk(data)) {
		return false, nil
	}
	return re.Match(data), nil
}

func grepFallbackMatchCount(path string, re *regexp.Regexp) (int, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsPermission(err) {
			return 0, false, nil
		}
		return 0, false, err
	}
	if isBinaryContent(firstSearchChunk(data)) {
		return 0, false, nil
	}
	count := 0
	for _, line := range splitSearchLines(string(data)) {
		if re.MatchString(line) {
			count++
		}
	}
	return count, count > 0, nil
}

func grepFallbackContentLines(path string, opts grepRipgrepOptions, re *regexp.Regexp, contextBefore int, contextAfter int) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsPermission(err) {
			return nil, nil
		}
		return nil, err
	}
	if isBinaryContent(firstSearchChunk(data)) {
		return nil, nil
	}

	lines := splitSearchLines(string(data))
	out := make([]string, 0)
	seen := make(map[int]bool)
	for idx, line := range lines {
		if !re.MatchString(line) {
			continue
		}
		start := idx - contextBefore
		if start < 0 {
			start = 0
		}
		end := idx + contextAfter
		if end >= len(lines) {
			end = len(lines) - 1
		}
		for lineIdx := start; lineIdx <= end; lineIdx++ {
			if seen[lineIdx] {
				continue
			}
			seen[lineIdx] = true
			out = append(out, formatGrepFallbackContentLine(path, lineIdx+1, lines[lineIdx], lineIdx == idx, opts))
		}
	}
	return out, nil
}

func splitSearchLines(content string) []string {
	content = strings.TrimRight(content, "\n")
	if content == "" {
		return []string{""}
	}
	lines := strings.Split(content, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], "\r")
	}
	return lines
}

func firstSearchChunk(data []byte) []byte {
	if len(data) > 8192 {
		return data[:8192]
	}
	return data
}

func formatGrepFallbackContentLine(path string, lineNumber int, line string, isMatch bool, opts grepRipgrepOptions) string {
	if opts.SearchPathInfo.IsDir() {
		sep := ":"
		if !isMatch {
			sep = "-"
		}
		prefix := formatSearchDisplayPathFrom(path, opts.DisplayRoot)
		if opts.ShowLineNumbers {
			return fmt.Sprintf("%s%s%d%s%s", prefix, sep, lineNumber, sep, line)
		}
		return fmt.Sprintf("%s%s%s", prefix, sep, line)
	}
	if opts.ShowLineNumbers {
		return fmt.Sprintf("%d:%s", lineNumber, line)
	}
	return line
}

func formatGrepFallbackCountLine(path string, count int, opts grepRipgrepOptions) string {
	if opts.SearchPathInfo.IsDir() {
		return fmt.Sprintf("%s:%d", formatSearchDisplayPathFrom(path, opts.DisplayRoot), count)
	}
	return fmt.Sprintf("%d", count)
}

func matchSearchGlob(pattern string, path string, root string) bool {
	pattern = filepath.ToSlash(strings.TrimSpace(pattern))
	if pattern == "" {
		return false
	}
	candidate := filepath.ToSlash(path)
	if filepath.IsAbs(path) {
		if rel, err := filepath.Rel(root, path); err == nil {
			candidate = filepath.ToSlash(rel)
		}
	}
	if !strings.Contains(pattern, "/") {
		return globPatternMatches(pattern, filepath.Base(candidate))
	}
	return globPatternMatches(pattern, candidate)
}

func globPatternMatches(pattern string, candidate string) bool {
	re, err := regexp.Compile("^" + searchGlobToRegex(pattern) + "$")
	if err != nil {
		return false
	}
	return re.MatchString(candidate)
}

func searchGlobToRegex(pattern string) string {
	var out strings.Builder
	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		switch ch {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				i++
				if i+1 < len(pattern) && pattern[i+1] == '/' {
					i++
					out.WriteString("(?:.*/)?")
				} else {
					out.WriteString(".*")
				}
				continue
			}
			out.WriteString("[^/]*")
		case '?':
			out.WriteString("[^/]")
		case '{':
			end := strings.IndexByte(pattern[i+1:], '}')
			if end >= 0 {
				body := pattern[i+1 : i+1+end]
				parts := strings.Split(body, ",")
				for j, part := range parts {
					parts[j] = regexp.QuoteMeta(part)
				}
				out.WriteString("(?:")
				out.WriteString(strings.Join(parts, "|"))
				out.WriteString(")")
				i += end + 1
			} else {
				out.WriteString(regexp.QuoteMeta(string(ch)))
			}
		default:
			out.WriteString(regexp.QuoteMeta(string(ch)))
		}
	}
	return out.String()
}
