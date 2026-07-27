package search

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// InspectGlobResult is the language-independent repository-glob projection
// consumed by the composite Inspect tool. Files are repository-display paths.
type InspectGlobResult struct {
	Files         []string
	HasMore       bool
	PartialReason string
}

// InspectSearchMatch is one matching source line returned to Inspect. Path is
// relative to the pinned project root whenever possible.
type InspectSearchMatch struct {
	Path string
	Line int
	Text string
}

// InspectSearchResult is the language-independent content-search projection
// consumed by the composite Inspect tool.
type InspectSearchResult struct {
	Matches       []InspectSearchMatch
	HasMore       bool
	PartialReason string
}

type inspectRuntimeProvider struct {
	runtime types.ToolRuntimeContext
}

func (p inspectRuntimeProvider) ToolRuntimeContext() types.ToolRuntimeContext {
	out := p.runtime
	out.AllowedDirs = append([]string(nil), p.runtime.AllowedDirs...)
	out.AllowedRules = append([]types.PermissionRuleValue(nil), p.runtime.AllowedRules...)
	out.DeniedRules = append([]types.PermissionRuleValue(nil), p.runtime.DeniedRules...)
	out.AskRules = append([]types.PermissionRuleValue(nil), p.runtime.AskRules...)
	if p.runtime.Features != nil {
		out.Features = make(map[string]bool, len(p.runtime.Features))
		for key, value := range p.runtime.Features {
			out.Features[key] = value
		}
	}
	return out
}

// RunInspectGlob reuses Glob's root resolution, ignore policy, ripgrep/native
// implementations, timeout policy, and display-path normalization while
// returning a compact typed result. runtime must already be the immutable
// repository-only snapshot selected by Inspect.
func RunInspectGlob(ctx context.Context, runtime types.ToolRuntimeContext, rawPath, pattern string, maxResults int) (InspectGlobResult, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return InspectGlobResult{}, i18n.NewError(i18n.KeyToolSearchPatternRequired)
	}
	provider := inspectRuntimeProvider{runtime: runtime}
	searchRuntime, err := searchRuntimeSnapshotFor(provider, &orphanedPluginCache{})
	if err != nil {
		return InspectGlobResult{}, err
	}
	root, err := resolveGlobSearchRootInScope(rawPath, searchRuntime)
	if err != nil {
		return InspectGlobResult{}, err
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, globSearchTimeout())
	defer cancel()
	acquisitionLimit := maxResults + 1
	var result globSearchResult
	if globPatternNeedsNativeMatch(pattern) {
		result, err = runGlobWithDoublestar(timeoutCtx, pattern, root, acquisitionLimit, searchRuntime)
	} else {
		result, err = runGlobWithRipgrep(timeoutCtx, pattern, root, acquisitionLimit, searchRuntime)
	}
	if err != nil {
		return InspectGlobResult{}, err
	}

	files := append([]string(nil), result.Files...)
	sort.Strings(files)
	hasMore := result.Truncated || len(files) > maxResults
	if len(files) > maxResults {
		files = files[:maxResults]
	}
	return InspectGlobResult{
		Files:         files,
		HasMore:       hasMore,
		PartialReason: result.PartialReason,
	}, nil
}

// RunInspectSearch reuses Grep's scope, ignore policy, ripgrep/fallback
// implementations, timeout handling, and path rendering. It intentionally
// requests match lines without context; Inspect obtains context through Read
// so the exact model-visible file version also becomes edit evidence.
func RunInspectSearch(ctx context.Context, runtime types.ToolRuntimeContext, rawPath, pattern string, maxResults int) (InspectSearchResult, error) {
	if strings.TrimSpace(pattern) == "" {
		return InspectSearchResult{}, i18n.NewError(i18n.KeyToolSearchPatternRequired)
	}
	provider := inspectRuntimeProvider{runtime: runtime}
	searchRuntime, err := searchRuntimeSnapshotFor(provider, &orphanedPluginCache{})
	if err != nil {
		return InspectSearchResult{}, err
	}
	searchPath, pathInfo, err := resolveGrepSearchPathInScope(rawPath, searchRuntime)
	if err != nil {
		return InspectSearchResult{}, err
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, globSearchTimeout())
	defer cancel()
	opts := grepRipgrepOptions{
		Pattern:         pattern,
		SearchPath:      searchPath,
		SearchPathInfo:  pathInfo,
		OutputMode:      "content",
		ShowLineNumbers: true,
		Unlimited:       true,
		DisplayRoot:     searchRuntime.cwd,
		Ignores:         searchRuntime.ignores,
	}
	grepResult, runErr := runGrepWithRipgrep(timeoutCtx, opts)
	lines := grepResult.Lines
	partialReason := grepResult.PartialReason
	if runErr != nil {
		var timeoutErr *ripgrepTimeoutError
		if !errors.As(runErr, &timeoutErr) || len(timeoutErr.Partial) == 0 {
			return InspectSearchResult{}, runErr
		}
		lines = prepareGrepPartialResults(timeoutErr.Partial, opts)
		partialReason = grepPartialTimeout
	}

	matches := make([]InspectSearchMatch, 0, len(lines))
	seen := make(map[string]struct{}, len(lines))
	for _, line := range lines {
		path, lineNumber, text, ok := splitInspectContentLine(line, searchPath, pathInfo, searchRuntime.cwd)
		if !ok {
			continue
		}
		key := path + "\x00" + strconv.Itoa(lineNumber)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		matches = append(matches, InspectSearchMatch{Path: path, Line: lineNumber, Text: text})
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Path != matches[j].Path {
			return matches[i].Path < matches[j].Path
		}
		if matches[i].Line != matches[j].Line {
			return matches[i].Line < matches[j].Line
		}
		return matches[i].Text < matches[j].Text
	})
	hasMore := len(matches) > maxResults
	if hasMore {
		matches = matches[:maxResults]
	}
	return InspectSearchResult{Matches: matches, HasMore: hasMore, PartialReason: partialReason}, nil
}

func splitInspectContentLine(line, searchPath string, pathInfo os.FileInfo, displayRoot string) (string, int, string, bool) {
	if pathInfo == nil {
		return "", 0, "", false
	}
	if !pathInfo.IsDir() {
		separator := strings.IndexByte(line, ':')
		if separator <= 0 {
			return "", 0, "", false
		}
		lineNumber, err := strconv.Atoi(line[:separator])
		if err != nil || lineNumber <= 0 {
			return "", 0, "", false
		}
		return formatSearchDisplayPathFrom(searchPath, displayRoot), lineNumber, line[separator+1:], true
	}

	for index := 1; index < len(line); index++ {
		if line[index] != ':' {
			continue
		}
		digitStart := index + 1
		digitEnd := digitStart
		for digitEnd < len(line) && line[digitEnd] >= '0' && line[digitEnd] <= '9' {
			digitEnd++
		}
		if digitEnd == digitStart || digitEnd >= len(line) || line[digitEnd] != ':' {
			continue
		}
		lineNumber, err := strconv.Atoi(line[digitStart:digitEnd])
		if err != nil || lineNumber <= 0 {
			continue
		}
		path := filepath.Clean(line[:index])
		if filepath.IsAbs(path) {
			path = formatSearchDisplayPathFrom(path, displayRoot)
		}
		return path, lineNumber, line[digitEnd+1:], true
	}
	return "", 0, "", false
}
