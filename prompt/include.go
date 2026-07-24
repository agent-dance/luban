package prompt

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const maxIncludeDepth = 5

var textFileExtensions = map[string]struct{}{
	".md": {}, ".txt": {}, ".text": {},
	".json": {}, ".yaml": {}, ".yml": {}, ".toml": {}, ".xml": {}, ".csv": {},
	".html": {}, ".htm": {}, ".css": {}, ".scss": {}, ".sass": {}, ".less": {},
	".js": {}, ".ts": {}, ".tsx": {}, ".jsx": {}, ".mjs": {}, ".cjs": {}, ".mts": {}, ".cts": {},
	".py": {}, ".pyi": {}, ".pyw": {},
	".rb": {}, ".erb": {}, ".rake": {},
	".go": {}, ".rs": {}, ".java": {}, ".kt": {}, ".kts": {}, ".scala": {},
	".c": {}, ".cpp": {}, ".cc": {}, ".cxx": {}, ".h": {}, ".hpp": {}, ".hxx": {},
	".cs": {}, ".swift": {},
	".sh": {}, ".bash": {}, ".zsh": {}, ".fish": {}, ".ps1": {}, ".bat": {}, ".cmd": {},
	".env": {}, ".ini": {}, ".cfg": {}, ".conf": {}, ".config": {}, ".properties": {},
	".sql": {}, ".graphql": {}, ".gql": {}, ".proto": {},
	".vue": {}, ".svelte": {}, ".astro": {},
	".ejs": {}, ".hbs": {}, ".pug": {}, ".jade": {},
	".php": {}, ".pl": {}, ".pm": {}, ".lua": {}, ".r": {}, ".R": {}, ".dart": {},
	".ex": {}, ".exs": {}, ".erl": {}, ".hrl": {}, ".clj": {}, ".cljs": {}, ".cljc": {}, ".edn": {},
	".hs": {}, ".lhs": {}, ".elm": {}, ".ml": {}, ".mli": {}, ".f": {}, ".f90": {}, ".f95": {}, ".for": {},
	".cmake": {}, ".make": {}, ".makefile": {}, ".gradle": {}, ".sbt": {},
	".rst": {}, ".adoc": {}, ".asciidoc": {}, ".org": {}, ".tex": {}, ".latex": {},
	".lock": {}, ".log": {}, ".diff": {}, ".patch": {},
}

func isSupportedIncludePath(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		return true
	}
	_, ok := textFileExtensions[ext]
	return ok
}

func extractIncludePaths(content, basePath string) []string {
	baseDir := filepath.Dir(basePath)
	seen := make(map[string]struct{})
	var paths []string

	inFence := false
	fenceMarker := ""
	for _, line := range strings.SplitAfter(content, "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		if isFenceStart(trimmed) {
			marker := fenceRun(trimmed)
			if inFence {
				if strings.HasPrefix(marker, fenceMarker) {
					inFence = false
					fenceMarker = ""
				}
			} else {
				inFence = true
				fenceMarker = marker
			}
			continue
		}
		if inFence {
			continue
		}

		for _, rawPath := range includePathCandidates(stripInlineCode(line)) {
			resolved := resolveIncludePath(rawPath, baseDir)
			if resolved == "" {
				continue
			}
			normalized := normalizeMemoryPath(resolved)
			if _, ok := seen[normalized]; ok {
				continue
			}
			seen[normalized] = struct{}{}
			paths = append(paths, resolved)
		}
	}
	return paths
}

func isFenceStart(trimmed string) bool {
	return strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")
}

func fenceRun(trimmed string) string {
	if trimmed == "" {
		return ""
	}
	ch := trimmed[0]
	i := 0
	for i < len(trimmed) && trimmed[i] == ch {
		i++
	}
	return trimmed[:i]
}

func stripInlineCode(line string) string {
	var out strings.Builder
	for i := 0; i < len(line); {
		if line[i] != '`' {
			out.WriteByte(line[i])
			i++
			continue
		}
		runStart := i
		for i < len(line) && line[i] == '`' {
			i++
		}
		runLen := i - runStart
		close := strings.Index(line[i:], strings.Repeat("`", runLen))
		if close == -1 {
			out.WriteString(line[runStart:i])
			continue
		}
		i += close + runLen
	}
	return out.String()
}

var includePathRe = regexp.MustCompile(`(?:^|\s)@((?:[^\s\\]|\\ )+)`)

func includePathCandidates(text string) []string {
	var paths []string
	for _, match := range includePathRe.FindAllStringSubmatch(text, -1) {
		if len(match) < 2 || match[1] == "" {
			continue
		}
		path := match[1]
		if hash := strings.IndexByte(path, '#'); hash >= 0 {
			path = path[:hash]
		}
		path = strings.ReplaceAll(path, `\ `, " ")
		if validIncludePath(path) {
			paths = append(paths, path)
		}
	}
	return paths
}

func validIncludePath(path string) bool {
	if path == "" || path == "/" {
		return false
	}
	if strings.HasPrefix(path, "./") || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "/") {
		return true
	}
	if strings.HasPrefix(path, "@") {
		return false
	}
	first := path[0]
	if !(first >= 'a' && first <= 'z') && !(first >= 'A' && first <= 'Z') &&
		!(first >= '0' && first <= '9') && first != '.' && first != '_' && first != '-' {
		return false
	}
	return true
}

func resolveIncludePath(path, baseDir string) string {
	switch {
	case strings.HasPrefix(path, "~/"):
		home, err := osUserHomeDir()
		if err != nil || strings.TrimSpace(home) == "" {
			return ""
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
	case filepath.IsAbs(path):
	case strings.HasPrefix(path, "./"):
		path = filepath.Join(baseDir, strings.TrimPrefix(path, "./"))
	default:
		path = filepath.Join(baseDir, path)
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return filepath.Clean(path)
}

var osUserHomeDir = func() (string, error) {
	return os.UserHomeDir()
}
