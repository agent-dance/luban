package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

const (
	defaultGrepHeadLimit = 250
	defaultGlobLimit     = 100

	ripgrepTimeoutPartialGrace = 100 * time.Millisecond
)

var (
	vcsDirectoriesToExclude = []string{
		".git",
		".svn",
		".hg",
		".bzr",
		".jj",
		".sl",
	}
)

// GlobTool finds files matching glob patterns.
type GlobTool struct {
	runtime     types.ToolRuntimeContextProvider
	pluginCache orphanedPluginCache
}

func NewGlobTool(runtime types.ToolRuntimeContextProvider) *GlobTool {
	return &GlobTool{runtime: runtime}
}

func (t *GlobTool) Name() string { return "Glob" }

func (t *GlobTool) Description() string {
	return "Fast file pattern matching. Supports glob patterns like \"**/*.go\" or \"src/**/*.ts\"."
}

func (t *GlobTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(
		map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "The glob pattern to match files against",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "The directory to search in (defaults to current directory)",
			},
		},
		"pattern",
	)
}

type GlobOutput struct {
	DurationMs int64    `json:"durationMs"`
	NumFiles   int      `json:"numFiles"`
	Filenames  []string `json:"filenames"`
	Truncated  bool     `json:"truncated"`
}

func (t *GlobTool) ToolContract() types.ToolContract {
	return types.ToolContract{
		OutputSchema: &types.JSONSchema{
			Type: "object",
			Properties: map[string]any{
				"durationMs": map[string]any{"type": "number", "description": "Time taken to execute the search in milliseconds"},
				"numFiles":   map[string]any{"type": "number", "description": "Total number of files found"},
				"filenames":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Array of file paths that match the pattern"},
				"truncated":  map[string]any{"type": "boolean", "description": "Whether results were truncated (limited to 100 files)"},
			},
			Required: []string{"durationMs", "numFiles", "filenames", "truncated"},
		},
		Strict:             true,
		ReadOnly:           true,
		ConcurrencySafe:    true,
		MaxResultSizeChars: 100_000,
	}
}

func (t *GlobTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	output, ok := data.(GlobOutput)
	if !ok {
		return types.ToolResultBlock{
			ToolUseID: toolUseID,
			Content:   toolRuntimeText(i18n.KeyToolLegacyCGlobInvalidResult),
			IsError:   true,
		}
	}
	return types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: toolUseID, Content: globModelContent(output)}
}

func (t *GlobTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	if err := types.ValidateToolInput(t, input); err != nil {
		return ErrorResponse(err), nil
	}
	in, err := types.DecodeStrictToolInput[GlobInput](input)
	if err != nil {
		return ErrorResponse(err), nil
	}
	if strings.TrimSpace(in.Pattern) == "" {
		return ErrorResponsef("%s", toolRuntimeText(i18n.KeyToolLegacyCPatternRequired)), nil
	}

	runtime, err := searchRuntimeSnapshotFor(t.runtime, &t.pluginCache)
	if err != nil {
		return ErrorResponse(err), nil
	}
	searchRoot, err := resolveGlobSearchRootInScope(in.Path, runtime)
	if err != nil {
		return ErrorResponse(err), nil
	}

	// glob-grep-timeout-env: cap the search at CLAUDE_CODE_GLOB_TIMEOUT_SECONDS
	// (default 20s, 60s on WSL). Without this very large repos exhaust the
	// inherited request budget and the search terminates with no results.
	timeoutCtx, cancel := context.WithTimeout(ctx, GlobSearchTimeout())
	defer cancel()
	ctx = timeoutCtx

	startedAt := time.Now()
	rawPattern := strings.TrimSpace(in.Pattern)
	pattern := translateMinimatchPattern(rawPattern)

	// Bracket character classes (e.g. `[^abc]`) cannot be passed reliably
	// through ripgrep's argv on every platform — the Windows command-runtime
	// applies syscall-level quoting that mangles the brackets before they
	// reach ripgrep. Route those patterns through a Go-native walker built
	// on doublestar/v4, which already handles POSIX character classes the
	// same way TS minimatch does.
	var searchResult globSearchResult
	if globPatternNeedsNativeMatch(pattern) {
		searchResult, err = runGlobWithDoublestar(ctx, pattern, searchRoot, defaultGlobLimit, runtime)
	} else {
		searchResult, err = runGlobWithRipgrep(ctx, pattern, searchRoot, defaultGlobLimit, runtime)
	}
	if err != nil {
		res := ErrorResponse(err)
		var timeoutErr *RipgrepTimeoutError
		if errors.As(err, &timeoutErr) || errors.Is(err, context.DeadlineExceeded) {
			res.Outcome = types.ToolOutcomeTimedOut
		}
		return res, nil
	}
	files := searchResult.Files
	truncated := searchResult.Truncated

	durationMs := time.Since(startedAt).Milliseconds()
	outputData := GlobOutput{
		DurationMs: durationMs,
		NumFiles:   len(files),
		Filenames:  append([]string(nil), files...),
		Truncated:  truncated,
	}
	metadata := map[string]string{
		"matched_count": strconv.Itoa(len(files)),
		"numFiles":      strconv.Itoa(len(files)),
		"truncated":     strconv.FormatBool(truncated),
		"max_results":   strconv.Itoa(defaultGlobLimit),
		"duration_ms":   strconv.FormatInt(durationMs, 10),
		"durationMs":    strconv.FormatInt(durationMs, 10),
	}
	if searchResult.PartialReason != "" {
		metadata["partial"] = "true"
		metadata["partial_reason"] = searchResult.PartialReason
		if searchResult.PartialReason == globPartialTimeout {
			metadata["timed_out"] = "true"
		}
	}
	outcome := searchOutcomeForPartialReason(searchResult.PartialReason)

	content := globModelContent(outputData)
	if len(files) == 0 {
		res, _ := StringResponse(content)
		res.Metadata = metadata
		res.Outcome = outcome
		res.Data = outputData
		res.ContentBlocks = []types.ContentBlock{newTextBlock(content)}
		return res, nil
	}

	res, _ := StringResponse(content)
	res.Metadata = metadata
	res.Outcome = outcome
	res.Data = outputData
	res.ContentBlocks = []types.ContentBlock{newTextBlock(content)}
	return res, nil
}

const (
	globPartialTimeout   = "timeout"
	globPartialStdoutCap = "stdout_cap"
)

func searchOutcomeForPartialReason(reason string) types.ToolOutcome {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ""
	}
	if reason == globPartialTimeout || reason == grepPartialTimeout {
		return types.ToolOutcomeTimedOut
	}
	return types.ToolOutcomePartial
}

type globSearchResult struct {
	Files         []string
	Truncated     bool
	PartialReason string
}

func globModelContent(output GlobOutput) string {
	if len(output.Filenames) == 0 {
		return toolRuntimeText(i18n.KeyToolLegacyCNoFiles)
	}
	lines := append([]string(nil), output.Filenames...)
	if output.Truncated {
		lines = append(lines, toolRuntimeText(i18n.KeyToolLegacyCResultsTruncated))
	}
	return strings.Join(lines, "\n")
}

// GrepTool searches file contents using regex patterns.
type GrepTool struct {
	runtime     types.ToolRuntimeContextProvider
	pluginCache orphanedPluginCache
}

func NewGrepTool(runtime types.ToolRuntimeContextProvider) *GrepTool {
	return &GrepTool{runtime: runtime}
}

func (t *GrepTool) Name() string { return "Grep" }

func (t *GrepTool) Description() string {
	return `A powerful search tool built on ripgrep

Usage:
- ALWAYS use Grep for search tasks. NEVER invoke ` + "`grep`" + ` or ` + "`rg`" + ` as a Bash command. Grep is optimized for correct permissions and access.
- Supports full regex syntax (for example, "log.*Error" and "function\\s+\\w+").
- Filter files with the glob parameter (for example, "*.js" or "**/*.tsx") or type parameter (for example, "js", "py", or "go").
- Output modes: "content" shows matching lines, "files_with_matches" shows only file paths (default), and "count" shows match counts.
- Use the Agent tool for open-ended searches requiring multiple rounds.
- Pattern syntax uses ripgrep, not grep; literal braces need escaping (use ` + "`interface\\{\\}`" + ` to find ` + "`interface{}`" + ` in Go code).
- Multiline matching is disabled by default. Set multiline: true for patterns that span lines.`
}

func (t *GrepTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(
		map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "The regex pattern to search for",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "File or directory to search in (defaults to current directory)",
			},
			"glob": map[string]any{
				"type":        "string",
				"description": "Glob pattern to filter files (e.g. \"*.go\")",
			},
			"output_mode": map[string]any{
				"type":        "string",
				"description": "Output mode: content, files_with_matches, or count",
				"enum":        []string{"content", "files_with_matches", "count"},
			},
			"-B":      grepSemanticNumber("Number of lines to show before each match (rg -B)."),
			"-A":      grepSemanticNumber("Number of lines to show after each match (rg -A)."),
			"-i":      semanticBoolean("Case insensitive search", false),
			"-C":      grepSemanticNumber("Context lines before and after match"),
			"context": grepSemanticNumber("Number of lines to show before and after each match (rg -C)."),
			"-n":      semanticBoolean("Show line numbers in output (rg -n).", true),
			"type": map[string]any{
				"type":        "string",
				"description": "File type to search (rg --type).",
			},
			"head_limit": grepSemanticNumber("Limit output to first N entries"),
			"offset":     grepSemanticNumber("Skip first N lines/entries before applying head_limit."),
			"multiline":  semanticBoolean("Enable multiline mode where patterns can span lines.", false),
		},
		"pattern",
	)
}

func grepSemanticNumber(description string) map[string]any {
	schema := semanticNumber(description, 0, false)
	// TS semanticNumber wraps an unconstrained z.number(). Ripgrep owns
	// validation for negative and fractional context values.
	delete(schema, "minimum")
	return schema
}

type GrepOutput struct {
	Mode          string                       `json:"mode,omitempty"`
	NumFiles      int                          `json:"numFiles"`
	Filenames     []string                     `json:"filenames"`
	Content       string                       `json:"content,omitempty"`
	NumLines      int                          `json:"numLines,omitempty"`
	NumMatches    int                          `json:"numMatches,omitempty"`
	AppliedLimit  int                          `json:"appliedLimit,omitempty"`
	AppliedOffset int                          `json:"appliedOffset,omitempty"`
	Completeness  types.ToolResultCompleteness `json:"completeness"`
}

func (t *GrepTool) ToolContract() types.ToolContract {
	return types.ToolContract{
		OutputSchema: &types.JSONSchema{
			Type: "object",
			Properties: map[string]any{
				"mode":          map[string]any{"type": "string", "enum": []string{"content", "files_with_matches", "count"}},
				"numFiles":      map[string]any{"type": "number"},
				"filenames":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"content":       map[string]any{"type": "string"},
				"numLines":      map[string]any{"type": "number"},
				"numMatches":    map[string]any{"type": "number"},
				"appliedLimit":  map[string]any{"type": "number"},
				"appliedOffset": map[string]any{"type": "number"},
				"completeness": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"source": map[string]any{"type": "string", "enum": []string{"complete", "source_truncated", "capture_dropped"}},
						"view":   map[string]any{"type": "string", "enum": []string{"pagination", "display_preview"}},
						"pagination": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"offset":      map[string]any{"type": "number"},
								"limit":       map[string]any{"type": "number"},
								"next_offset": map[string]any{"type": "number"},
								"has_more":    map[string]any{"type": "boolean"},
							},
							"required":             []string{"offset", "limit", "next_offset", "has_more"},
							"additionalProperties": false,
						},
					},
					"required":             []string{"source"},
					"additionalProperties": false,
				},
			},
			Required: []string{"numFiles", "filenames", "completeness"},
		},
		Strict:             true,
		ReadOnly:           true,
		ConcurrencySafe:    true,
		MaxResultSizeChars: 20_000,
	}
}

func (t *GrepTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	output, ok := data.(GrepOutput)
	if !ok {
		return types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: toolUseID, Content: toolRuntimeText(i18n.KeyToolLegacyCGrepInvalidResult), IsError: true}
	}
	return types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: toolUseID, Content: grepModelContent(output), Completeness: output.Completeness}
}

func (t *GrepTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	if err := types.ValidateToolInput(t, input); err != nil {
		return ErrorResponse(err), nil
	}
	coerced, err := coerceGrepSemanticInput(input)
	if err != nil {
		return ErrorResponse(err), nil
	}
	in, err := types.DecodeStrictToolInput[GrepInput](coerced)
	if err != nil {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyCInvalidInput, err)), nil
	}

	if _, present := input["pattern"]; !present {
		return ErrorResponsef("%s", toolRuntimeText(i18n.KeyToolLegacyCPatternRequired)), nil
	}

	runtime, err := searchRuntimeSnapshotFor(t.runtime, &t.pluginCache)
	if err != nil {
		return ErrorResponse(err), nil
	}
	searchPath, pathInfo, err := resolveGrepSearchPathInScope(in.Path, runtime)
	if err != nil {
		return ErrorResponse(err), nil
	}

	outputMode := in.OutputMode
	if outputMode == "" {
		outputMode = "files_with_matches"
	}
	if outputMode != "content" && outputMode != "files_with_matches" && outputMode != "count" {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyCInvalidOutputMode, outputMode)), nil
	}

	rawGlob := in.Glob

	showLineNumbers := true
	if in.ShowLineNumbers != nil {
		showLineNumbers = *in.ShowLineNumbers
	}

	contextBefore, contextBeforeSet := floatFromPtr(in.ContextBefore)
	contextAfter, contextAfterSet := floatFromPtr(in.ContextAfter)
	contextAll, contextAllSet := floatFromPtr(in.Context)
	if !contextAllSet {
		contextAll, contextAllSet = floatFromPtr(in.ContextC)
	}

	offset := int(derefFloat(in.Offset))
	headLimit := defaultGrepHeadLimit
	headLimitSet := false
	unlimited := false
	if in.HeadLimit != nil {
		headLimit = int(*in.HeadLimit)
		headLimitSet = true
		if headLimit == 0 {
			unlimited = true
		}
	}

	// glob-grep-timeout-env: same timeout policy as Glob — cap at
	// CLAUDE_CODE_GLOB_TIMEOUT_SECONDS (default 20s; 60s on WSL).
	grepTimeoutCtx, grepCancel := context.WithTimeout(ctx, GlobSearchTimeout())
	defer grepCancel()
	ctx = grepTimeoutCtx

	startedAt := time.Now()
	grepOpts := grepRipgrepOptions{
		Pattern:          in.Pattern,
		SearchPath:       searchPath,
		SearchPathInfo:   pathInfo,
		OutputMode:       outputMode,
		CaseInsensitive:  in.CaseInsensitive,
		ShowLineNumbers:  showLineNumbers,
		ContextBefore:    contextBefore,
		ContextAfter:     contextAfter,
		ContextBeforeSet: contextBeforeSet,
		ContextAfterSet:  contextAfterSet,
		Context:          contextAll,
		ContextSet:       contextAllSet,
		Type:             in.Type,
		Glob:             rawGlob,
		Offset:           offset,
		// Pass an effectively-unlimited cap so we can detect truncation
		// against the user-facing head_limit at the Execute layer. The
		// runGrepWithRipgrep helper still paginates with offset.
		HeadLimit:   0,
		Unlimited:   true,
		Multiline:   in.Multiline,
		DisplayRoot: runtime.cwd,
		Ignores:     runtime.ignores,
	}
	searchResult, err := runGrepWithRipgrep(ctx, grepOpts)
	allResults := searchResult.Lines
	partialReason := searchResult.PartialReason
	if err != nil {
		// grep-timeout-vs-no-matches-distinct-error +
		// grep-partial-results-on-timeout: surface ripgrep timeouts as a
		// distinct error so the model never confuses them with "no
		// matches". When partial output was captured we render it inline.
		var timeoutErr *RipgrepTimeoutError
		if errors.As(err, &timeoutErr) {
			if len(timeoutErr.Partial) == 0 {
				body := timeoutErr.Error()
				res := types.ToolResult{Content: body, IsError: true}
				res.Metadata = map[string]string{
					"timed_out":     "true",
					"partial_count": "0",
					"output_mode":   outputMode,
					"duration_ms":   strconv.FormatInt(time.Since(startedAt).Milliseconds(), 10),
				}
				res.Outcome = types.ToolOutcomeTimedOut
				return res, nil
			}
			allResults = prepareGrepPartialResults(timeoutErr.Partial, grepOpts)
			partialReason = grepPartialTimeout
		}
		if allResults == nil {
			return ErrorResponse(err), nil
		}
	}

	// Apply head_limit at the Execute layer so we can both surface a
	// machine-readable truncated flag in metadata AND keep runGrepWithRipgrep's
	// existing pagination semantics for legacy callers.
	results := allResults
	truncated := false
	if !unlimited && headLimitSet && headLimit >= 0 && len(allResults) > headLimit {
		results = allResults[:headLimit]
		truncated = true
	} else if !unlimited && !headLimitSet && len(allResults) > defaultGrepHeadLimit {
		results = allResults[:defaultGrepHeadLimit]
		truncated = true
	}

	matchCount, numMatches := computeGrepMatchCount(outputMode, results)

	metadata := map[string]string{
		"output_mode":        outputMode,
		"truncated":          strconv.FormatBool(truncated),
		"match_count":        strconv.Itoa(matchCount),
		"num_matches":        strconv.Itoa(numMatches),
		"multiline":          strconv.FormatBool(in.Multiline),
		"duration_ms":        strconv.FormatInt(time.Since(startedAt).Milliseconds(), 10),
		"maxResultSizeChars": "20000",
	}
	if partialReason != "" {
		metadata["partial"] = "true"
		metadata["partial_reason"] = partialReason
		metadata["truncated"] = "true"
		if partialReason == grepPartialTimeout {
			metadata["timed_out"] = "true"
		}
	}
	if t := in.Type; t != "" {
		metadata["type"] = t
	}
	if truncated {
		metadata["appliedLimit"] = strconv.Itoa(headLimit)
	}
	if offset > 0 {
		metadata["appliedOffset"] = strconv.Itoa(offset)
	}

	paginated := truncated || offset > 0
	completeness := grepResultCompleteness(partialReason, paginated, truncated, headLimit, offset, len(results))
	outputData := buildGrepOutput(outputMode, results, truncated, headLimit, offset, matchCount, numMatches, completeness)
	content := grepModelContent(outputData)

	res, _ := StringResponse(content)
	res.Metadata = metadata
	res.Outcome = searchOutcomeForPartialReason(partialReason)
	if res.Outcome == "" {
		if paginated {
			res.Outcome = types.ToolOutcomePartial
		} else {
			res.Outcome = types.ToolOutcomeSucceeded
		}
	}
	res.Data = outputData
	res.Completeness = completeness.Clone()
	res.ContentBlocks = []types.ContentBlock{newTextBlock(content)}
	return res, nil
}

func buildGrepOutput(mode string, results []string, truncated bool, limit int, offset int, matchCount int, numMatches int, completeness types.ToolResultCompleteness) GrepOutput {
	out := GrepOutput{Mode: mode, Filenames: []string{}, Completeness: completeness}
	if truncated {
		out.AppliedLimit = limit
	}
	if offset > 0 {
		out.AppliedOffset = offset
	}
	switch mode {
	case "content":
		out.Content = strings.Join(results, "\n")
		out.NumLines = len(results)
	case "count":
		out.Content = strings.Join(results, "\n")
		out.NumFiles = matchCount
		out.NumMatches = numMatches
	default:
		out.Filenames = append([]string(nil), results...)
		out.NumFiles = len(results)
	}
	return out
}

func grepResultCompleteness(partialReason string, paginated, hasMore bool, limit, offset, pageCount int) types.ToolResultCompleteness {
	completeness := types.ToolResultCompleteness{Source: types.ToolResultCompletenessComplete}
	switch partialReason {
	case grepPartialTimeout:
		completeness.Source = types.ToolResultCompletenessSourceTruncated
	case grepPartialStdoutCap:
		completeness.Source = types.ToolResultCompletenessCaptureDropped
	}
	if paginated {
		if limit <= 0 {
			limit = pageCount
		}
		completeness.View = types.ToolResultCompletenessPagination
		completeness.Pagination = &types.ToolResultPagination{
			Offset: offset, Limit: limit, NextOffset: offset + pageCount, HasMore: hasMore,
		}
	}
	return completeness
}

func grepModelContent(output GrepOutput) string {
	limitInfo := grepLimitInfo(output.AppliedLimit, output.AppliedOffset)
	switch output.Mode {
	case "content":
		content := output.Content
		if content == "" {
			content = toolRuntimeText(i18n.KeyToolLegacyCNoMatches)
		}
		if limitInfo != "" {
			content += "\n\n" + toolRuntimeFormat(i18n.KeyToolLegacyCShowingPagination, limitInfo)
		}
		return content
	case "count":
		content := output.Content
		if content == "" {
			content = toolRuntimeText(i18n.KeyToolLegacyCNoMatches)
		}
		matches := output.NumMatches
		files := output.NumFiles
		occurrence := toolRuntimeText(i18n.KeyToolLegacyCOccurrences)
		if matches == 1 {
			occurrence = toolRuntimeText(i18n.KeyToolLegacyCOccurrence)
		}
		fileWord := toolRuntimeText(i18n.KeyToolLegacyCFiles)
		if files == 1 {
			fileWord = toolRuntimeText(i18n.KeyToolLegacyCFile)
		}
		summary := "\n\n" + toolRuntimeFormat(i18n.KeyToolLegacyCFoundTotalAcross, matches, occurrence, files, fileWord)
		if limitInfo != "" {
			summary += toolRuntimeFormat(i18n.KeyToolLegacyCWithPagination, limitInfo)
		}
		return content + summary
	default:
		if output.NumFiles == 0 {
			return toolRuntimeText(i18n.KeyToolLegacyCNoFiles)
		}
		fileWord := toolRuntimeText(i18n.KeyToolLegacyCFiles)
		if output.NumFiles == 1 {
			fileWord = toolRuntimeText(i18n.KeyToolLegacyCFile)
		}
		header := toolRuntimeFormat(i18n.KeyToolLegacyCFoundFiles, output.NumFiles, fileWord)
		if limitInfo != "" {
			header += " " + limitInfo
		}
		return header + "\n" + strings.Join(output.Filenames, "\n")
	}
}

func grepLimitInfo(limit int, offset int) string {
	parts := make([]string, 0, 2)
	if limit > 0 {
		parts = append(parts, toolRuntimeFormat(i18n.KeyToolLegacyCLimit, limit))
	}
	if offset > 0 {
		parts = append(parts, toolRuntimeFormat(i18n.KeyToolLegacyCOffset, offset))
	}
	return strings.Join(parts, ", ")
}

// computeGrepMatchCount derives both the user-facing "match_count" and the
// detailed "num_matches" tally from the rendered ripgrep output. For
// content-mode the count is the number of match lines (excluding context-only
// `-`-separated rows); for count-mode we sum the per-file totals; for
// files_with_matches we treat each file path as one match.
func computeGrepMatchCount(outputMode string, results []string) (matchCount int, numMatches int) {
	switch outputMode {
	case "content":
		for _, line := range results {
			// Match lines from ripgrep use ":" between the path/line and
			// the body; context-only rows use "-". Skip the latter so the
			// count reflects actual hits.
			if line == "--" {
				continue
			}
			if isGrepContextLine(line) {
				continue
			}
			matchCount++
		}
		numMatches = matchCount
	case "count":
		for _, line := range results {
			// Format: "path:N" (directory mode) or "N" (file mode).
			n := strings.TrimSpace(line)
			if idx := strings.LastIndex(line, ":"); idx >= 0 {
				n = strings.TrimSpace(line[idx+1:])
			}
			if v, err := strconv.Atoi(n); err == nil {
				numMatches += v
			}
		}
		matchCount = len(results)
	default: // files_with_matches
		matchCount = len(results)
		numMatches = matchCount
	}
	return matchCount, numMatches
}

// isGrepContextLine reports whether a ripgrep content-mode line is a context
// (-A/-B/-C) row rather than an actual match. ripgrep separates the path/line
// from the body with ':' for matches and '-' for context rows.
func isGrepContextLine(line string) bool {
	// Walk past an optional path prefix (a leading run of non-':' / non-'-'
	// then a separator). When the first separator we see is '-', this is a
	// context line; ':' indicates a match.
	for i := 0; i < len(line); i++ {
		c := line[i]
		if c == ':' {
			return false
		}
		if c == '-' {
			// ripgrep emits "<path>-<line>-<body>" or "<line>-<body>"
			// for context rows and "<path>:<line>:<body>" for matches.
			// A '-' before ':' marks context.
			return true
		}
	}
	return false
}

type grepRipgrepOptions struct {
	Pattern          string
	SearchPath       string
	SearchPathInfo   os.FileInfo
	OutputMode       string
	CaseInsensitive  bool
	ShowLineNumbers  bool
	ContextBefore    float64
	ContextAfter     float64
	ContextBeforeSet bool
	ContextAfterSet  bool
	Context          float64
	ContextSet       bool
	Type             string
	Glob             string
	Offset           int
	HeadLimit        int
	Unlimited        bool
	Multiline        bool
	DisplayRoot      string
	Ignores          fileReadIgnoreConfig
}

const (
	grepPartialTimeout   = "timeout"
	grepPartialStdoutCap = "stdout_cap"
)

type grepSearchResult struct {
	Lines         []string
	PartialReason string
}

func coerceGrepSemanticInput(input map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(input))
	for key, value := range input {
		switch key {
		case "-i", "-n", "multiline":
			coerced, err := coerceGrepSemanticBool(value)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", key, err)
			}
			out[key] = coerced
		case "-B", "-A", "-C", "context", "head_limit", "offset":
			coerced, err := coerceGrepSemanticNumber(value)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", key, err)
			}
			out[key] = coerced
		default:
			out[key] = value
		}
	}
	return out, nil
}

func coerceGrepSemanticBool(value any) (any, error) {
	switch v := value.(type) {
	case bool:
		return v, nil
	case string:
		switch v {
		case "true":
			return true, nil
		case "false":
			return false, nil
		}
	}
	return value, nil
}

func coerceGrepSemanticNumber(value any) (any, error) {
	switch v := value.(type) {
	case string:
		if !grepSemanticNumberLiteral.MatchString(v) {
			return value, nil
		}
		n, err := strconv.ParseFloat(v, 64)
		if err != nil || math.IsInf(n, 0) || math.IsNaN(n) {
			return value, nil
		}
		return n, nil
	default:
		return value, nil
	}
}

var grepSemanticNumberLiteral = regexp.MustCompile(`^-?\d+(\.\d+)?$`)

func floatFromPtr(value *float64) (float64, bool) {
	if value == nil {
		return 0, false
	}
	return *value, true
}

func derefFloat(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func runGlobWithRipgrep(ctx context.Context, pattern string, cwd string, limit int, runtime searchRuntimeSnapshot) (globSearchResult, error) {
	searchDir := cwd
	searchPattern := pattern
	if filepath.IsAbs(pattern) {
		baseDir, relativePattern := extractGlobBaseDirectory(pattern)
		if baseDir != "" {
			allowedBase, err := ensureSearchRootAllowed(baseDir, runtime.cwd, runtime.allowedDirs)
			if err != nil {
				return globSearchResult{}, err
			}
			baseDir = allowedBase
			searchDir = baseDir
			searchPattern = relativePattern
		}
	}

	args := []string{
		"--files",
		"--sort=modified",
		"--glob", searchPattern,
	}
	// glob-env-no-ignore-hidden: env-var toggles allow ops to disable
	// the hidden / no-ignore behaviour without recompiling.
	if globEnvHiddenEnabled() {
		args = append(args, "--hidden")
	}
	if globEnvNoIgnoreEnabled() {
		args = append(args, "--no-ignore")
	}
	for _, ignore := range runtime.ignores.ripgrepGlobs(searchDir) {
		args = append(args, "--glob", ignore)
	}
	// Re-order: pattern stays at the end (--glob ARG) untouched. Above
	// flags are inserted between sort and the glob arg.
	run, err := runRipgrepDetailed(ctx, args, searchDir)
	results := run.Lines
	partialReason := ""
	if run.Truncated {
		partialReason = globPartialStdoutCap
	}
	if err != nil {
		if isRipgrepUnavailable(err) {
			return runGlobWithFallback(ctx, searchPattern, searchDir, limit, runtime)
		}
		var timeoutErr *RipgrepTimeoutError
		if errors.As(err, &timeoutErr) && len(timeoutErr.Partial) > 0 {
			results = timeoutErr.Partial
			partialReason = globPartialTimeout
		} else {
			return globSearchResult{}, err
		}
	}

	absolutePaths := make([]string, 0, len(results))
	for _, path := range results {
		if filepath.IsAbs(path) {
			absolutePaths = append(absolutePaths, path)
			continue
		}
		absolutePaths = append(absolutePaths, filepath.Join(searchDir, path))
	}

	// glob-permission-ignore-patterns: respect host-configured file-read
	// deny rules so paths the agent isn't allowed to READ aren't silently
	// enumerable via Glob. Mirrors src/utils/glob.ts:86-89,110-112.
	absolutePaths = runtime.ignores.filter(absolutePaths)

	sortGlobAbsolutePathsByMtime(absolutePaths, true)
	truncated := partialReason != "" || len(absolutePaths) > limit
	if len(absolutePaths) > limit {
		absolutePaths = absolutePaths[:limit]
	}

	formatted := make([]string, 0, len(absolutePaths))
	for _, path := range absolutePaths {
		formatted = append(formatted, formatSearchDisplayPathFrom(path, runtime.cwd))
	}
	return globSearchResult{Files: formatted, Truncated: truncated, PartialReason: partialReason}, nil
}

func globPartialDisplayPaths(results []string, searchRoot string, limit int, runtime searchRuntimeSnapshot) globSearchResult {
	absolutePaths := make([]string, 0, len(results))
	for _, path := range results {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if filepath.IsAbs(path) {
			absolutePaths = append(absolutePaths, path)
			continue
		}
		absolutePaths = append(absolutePaths, filepath.Join(searchRoot, path))
	}
	absolutePaths = runtime.ignores.filter(absolutePaths)
	sortGlobAbsolutePathsByMtime(absolutePaths, true)
	truncated := len(absolutePaths) > limit
	if truncated {
		absolutePaths = absolutePaths[:limit]
	}
	files := make([]string, 0, len(absolutePaths))
	for _, path := range absolutePaths {
		files = append(files, formatSearchDisplayPathFrom(path, runtime.cwd))
	}
	return globSearchResult{Files: files, Truncated: true, PartialReason: globPartialTimeout}
}

func sortGlobAbsolutePathsByMtime(paths []string, oldestFirst bool) {
	if len(paths) <= 1 {
		return
	}
	type rec struct {
		path  string
		mtime int64
		ok    bool
		idx   int
	}
	records := make([]rec, len(paths))
	for i, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			records[i] = rec{path: path, ok: false, idx: i}
			continue
		}
		records[i] = rec{path: path, mtime: info.ModTime().UnixNano(), ok: true, idx: i}
	}
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].ok != records[j].ok {
			return records[i].ok
		}
		if !records[i].ok {
			return records[i].idx < records[j].idx
		}
		if records[i].mtime == records[j].mtime {
			return records[i].path < records[j].path
		}
		if oldestFirst {
			return records[i].mtime < records[j].mtime
		}
		return records[i].mtime > records[j].mtime
	})
	for i, r := range records {
		paths[i] = r.path
	}
}

func runGrepWithRipgrep(ctx context.Context, opts grepRipgrepOptions) (grepSearchResult, error) {
	const ripgrepStdoutCap = 20_000_000
	return runGrepWithRipgrepCap(ctx, opts, ripgrepStdoutCap)
}

func runGrepWithRipgrepCap(ctx context.Context, opts grepRipgrepOptions, stdoutCap int) (grepSearchResult, error) {
	args := []string{"--hidden", "--max-columns", "500"}

	for _, dir := range vcsDirectoriesToExclude {
		args = append(args, "--glob", "!"+dir)
	}

	if opts.Multiline {
		args = append(args, "-U", "--multiline-dotall")
	}
	if opts.CaseInsensitive {
		args = append(args, "-i")
	}

	switch opts.OutputMode {
	case "files_with_matches":
		args = append(args, "-l")
		if opts.SearchPathInfo.IsDir() {
			args = append(args, "--sort=modified")
		}
	case "count":
		args = append(args, "-c")
	case "content":
		if opts.ShowLineNumbers {
			args = append(args, "-n")
		}
		if opts.ContextSet {
			args = append(args, "-C", formatGrepNumber(opts.Context))
		} else {
			if opts.ContextBeforeSet {
				args = append(args, "-B", formatGrepNumber(opts.ContextBefore))
			}
			if opts.ContextAfterSet {
				args = append(args, "-A", formatGrepNumber(opts.ContextAfter))
			}
		}
	}

	if strings.HasPrefix(opts.Pattern, "-") {
		args = append(args, "-e", opts.Pattern)
	} else {
		args = append(args, opts.Pattern)
	}

	if opts.Type != "" {
		args = append(args, "--type", opts.Type)
	}

	for _, globPattern := range splitSearchGlobPatterns(opts.Glob) {
		args = append(args, "--glob", globPattern)
	}
	for _, ignore := range opts.Ignores.ripgrepGlobs(opts.SearchPath) {
		args = append(args, "--glob", ignore)
	}

	run, err := runRipgrepDetailedWithCap(ctx, args, opts.SearchPath, stdoutCap)
	if err != nil {
		if isRipgrepUnavailable(err) {
			if SlowFallbackEnabled() {
				results, fallbackErr := runGrepWithFallback(ctx, opts)
				return grepSearchResult{Lines: results}, fallbackErr
			}
			return grepSearchResult{}, err
		}
		return grepSearchResult{}, err
	}
	results := prepareGrepResults(run.Lines, opts)
	partialReason := ""
	if run.Truncated {
		partialReason = grepPartialStdoutCap
	}
	return grepSearchResult{Lines: results, PartialReason: partialReason}, nil
}

func formatGrepNumber(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func prepareGrepResults(results []string, opts grepRipgrepOptions) []string {
	results = filterGrepRawResults(results, opts)
	switch opts.OutputMode {
	case "content":
		for i, line := range results {
			results[i] = relativizeRipgrepContentLine(line, opts.SearchPath, opts.SearchPathInfo, opts.DisplayRoot)
		}
	case "count":
		for i, line := range results {
			results[i] = relativizeRipgrepCountLine(line, opts.SearchPath, opts.SearchPathInfo, opts.DisplayRoot)
		}
	case "files_with_matches":
		if opts.SearchPathInfo.IsDir() {
			sort.SliceStable(results, func(i, j int) bool {
				if os.Getenv("NODE_ENV") == "test" {
					return results[i] < results[j]
				}
				left := searchResultModTime(opts.SearchPath, results[i])
				right := searchResultModTime(opts.SearchPath, results[j])
				if left == right {
					return results[i] < results[j]
				}
				return left > right
			})
		}
		for i, line := range results {
			results[i] = relativizeRipgrepFileLine(line, opts.SearchPath, opts.SearchPathInfo, opts.DisplayRoot)
		}
	}
	return paginateSearchResults(results, opts.Offset, opts.HeadLimit, opts.Unlimited)
}

func filterGrepRawResults(results []string, opts grepRipgrepOptions) []string {
	if len(results) == 0 || (len(opts.Ignores.rules) == 0 && len(opts.Ignores.pluginDirs) == 0) {
		return results
	}
	filtered := make([]string, 0, len(results))
	for _, line := range results {
		path := grepRawResultPath(line, opts)
		if path != "" && opts.Ignores.ignored(path) {
			continue
		}
		filtered = append(filtered, line)
	}
	return filtered
}

func grepRawResultPath(line string, opts grepRipgrepOptions) string {
	if !opts.SearchPathInfo.IsDir() {
		return opts.SearchPath
	}
	var pathPart string
	switch opts.OutputMode {
	case "files_with_matches":
		pathPart = line
	case "count":
		if colon := strings.LastIndex(line, ":"); colon > 0 {
			pathPart = line[:colon]
		}
	case "content":
		if line == "--" {
			return ""
		}
		pathPart = grepContentLinePath(line)
	}
	if pathPart == "" {
		return ""
	}
	if filepath.IsAbs(pathPart) {
		return filepath.Clean(pathPart)
	}
	return filepath.Join(opts.SearchPath, pathPart)
}

func grepContentLinePath(line string) string {
	for i := 1; i < len(line); i++ {
		separator := line[i]
		if separator != ':' && separator != '-' {
			continue
		}
		j := i + 1
		for j < len(line) && line[j] >= '0' && line[j] <= '9' {
			j++
		}
		if j > i+1 && j < len(line) && line[j] == separator {
			return line[:i]
		}
	}
	return ""
}

func paginateSearchResults(results []string, offset int, limit int, unlimited bool) []string {
	if offset > 0 {
		if offset >= len(results) {
			return nil
		}
		results = results[offset:]
	}
	if unlimited {
		return results
	}
	if limit < len(results) {
		return results[:limit]
	}
	return results
}

func resolveGlobSearchRoot(rawPath string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return resolveGlobSearchRootInScope(rawPath, searchRuntimeSnapshot{
		cwd:         cwd,
		allowedDirs: AllowedSearchDirs(),
		ignores:     legacyFileReadIgnoreConfig(),
	})
}

func resolveGlobSearchRootInScope(rawPath string, runtime searchRuntimeSnapshot) (string, error) {
	cwd := runtime.cwd
	candidate := strings.TrimSpace(rawPath)
	if candidate == "" {
		candidate = cwd
	}
	// glob-grep-tilde-expansion: TS expands ~ / ~user; Go needs the same so
	// the model can pass home-relative paths.
	candidate = expandTildePath(candidate)
	// glob-grep-unc-path-skip: refuse UNC paths (\\host\share or
	// //host/share). Stat-ing them risks NTLM credential leak.
	if isUNCPath(candidate) {
		return "", fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolLegacyCUNCNotAllowed, rawPath))
	}

	absPath, err := absolutePathFromBase(candidate, cwd)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", directoryNotFoundErrorAt(rawPath, cwd)
		}
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolLegacyCPathNotDirectory, rawPath))
	}
	if !isPathWithinAllowedDirs(absPath, runtime.allowedDirs) {
		return "", fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolLegacyCPathOutsideAllowed, rawPath))
	}
	if real, err := filepath.EvalSymlinks(absPath); err == nil {
		if !isPathWithinAllowedDirs(real, runtime.allowedDirs) {
			return "", fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolLegacyCPathResolvesOutsideAllowed, rawPath))
		}
	}
	return absPath, nil
}

func resolveGrepSearchPath(rawPath string) (string, os.FileInfo, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", nil, err
	}
	return resolveGrepSearchPathInScope(rawPath, searchRuntimeSnapshot{
		cwd:         cwd,
		allowedDirs: AllowedSearchDirs(),
		ignores:     legacyFileReadIgnoreConfig(),
	})
}

func resolveGrepSearchPathInScope(rawPath string, runtime searchRuntimeSnapshot) (string, os.FileInfo, error) {
	cwd := runtime.cwd
	candidate := strings.TrimSpace(rawPath)
	if candidate == "" {
		candidate = cwd
	}
	candidate = expandTildePath(candidate)
	if isUNCPath(candidate) {
		return "", nil, fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolLegacyCUNCNotAllowed, rawPath))
	}

	absPath, err := absolutePathFromBase(candidate, cwd)
	if err != nil {
		return "", nil, err
	}
	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil, grepPathNotFoundErrorAt(rawPath, cwd)
		}
		return "", nil, err
	}
	if !isPathWithinAllowedDirs(absPath, runtime.allowedDirs) {
		return "", nil, fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolLegacyCPathOutsideAllowed, rawPath))
	}
	if real, err := filepath.EvalSymlinks(absPath); err == nil {
		if !isPathWithinAllowedDirs(real, runtime.allowedDirs) {
			return "", nil, fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolLegacyCPathResolvesOutsideAllowed, rawPath))
		}
	}
	return absPath, info, nil
}

func grepPathNotFoundError(path string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolLegacyCPathMissing, path))
	}
	return grepPathNotFoundErrorAt(path, cwd)
}

func grepPathNotFoundErrorAt(path string, cwd string) error {
	message := toolRuntimeFormat(i18n.KeyToolLegacyCPathMissingAtCWD, path, cwd)
	if suggestion := suggestNearbyPath(path, cwd); suggestion != "" {
		message += toolRuntimeFormat(i18n.KeyToolLegacyCDidYouMean, suggestion)
	}
	return fmt.Errorf("%s", message)
}

func directoryNotFoundError(path string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolLegacyCDirectoryMissing, path))
	}
	return directoryNotFoundErrorAt(path, cwd)
}

func directoryNotFoundErrorAt(path string, cwd string) error {
	message := toolRuntimeFormat(i18n.KeyToolLegacyCDirectoryMissingAtCWD, path, cwd)
	if suggestion := suggestNearbyPath(path, cwd); suggestion != "" {
		message += toolRuntimeFormat(i18n.KeyToolLegacyCDidYouMean, suggestion)
	}
	return fmt.Errorf("%s", message)
}

// RipgrepTimeoutError signals that a ripgrep invocation hit its context
// deadline before completing. Mirrors src/utils/ripgrep.ts:97-106. Partial
// holds whatever stdout lines we collected before the SIGKILL/cancel.
// Callers SHOULD use errors.As to render partial results plus a
// "search timed out — partial results" notice.
type RipgrepTimeoutError struct {
	Partial []string
	Cause   error
}

func (e *RipgrepTimeoutError) Error() string {
	return toolRuntimeText(i18n.KeyToolLegacyCRipgrepTimedOut)
}

func (e *RipgrepTimeoutError) Unwrap() error { return e.Cause }

// isEAGAINText reports whether stderr/error text indicates ripgrep
// failed to spawn worker threads (EAGAIN). Mirrors src/utils/ripgrep.ts:85-92.
func isEAGAINText(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "os error 11") ||
		strings.Contains(lower, "resource temporarily unavailable") ||
		strings.Contains(lower, "eagain")
}

// drainStdoutPartial cleans the captured stdout buffer and returns the
// already-flushed lines (dropping a possibly-truncated final line). Used by
// the timeout / SIGKILL path so the agent gets thousands of partial hits
// instead of a hard error.
func drainStdoutPartial(out string) []string {
	out = strings.ReplaceAll(out, "\r", "")
	out = strings.TrimRight(out, "\n")
	if out == "" {
		return nil
	}
	lines := strings.Split(out, "\n")
	// Drop the final line — without a trailing newline we can't be sure it
	// wasn't truncated mid-write by the cap or the SIGKILL.
	if len(lines) > 0 {
		lines = lines[:len(lines)-1]
	}
	return lines
}

type ripgrepRunResult struct {
	Lines     []string
	Truncated bool
}

func runRipgrep(ctx context.Context, args []string, target string) ([]string, error) {
	result, err := runRipgrepDetailed(ctx, args, target)
	return result.Lines, err
}

func runRipgrepDetailed(ctx context.Context, args []string, target string) (ripgrepRunResult, error) {
	const ripgrepStdoutCap = 50 << 20
	return runRipgrepDetailedWithCap(ctx, args, target, ripgrepStdoutCap)
}

func configureRipgrepTimeoutCommand(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		time.Sleep(ripgrepTimeoutPartialGrace)
		if cmd.Process == nil {
			return nil
		}
		if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return err
		}
		return nil
	}
	cmd.WaitDelay = ripgrepTimeoutPartialGrace
}

func runRipgrepDetailedWithCap(ctx context.Context, args []string, target string, stdoutCap int) (ripgrepRunResult, error) {
	if searchEnvTruthy(os.Getenv("CLAUDE_CODE_FORCE_SEARCH_FALLBACK")) {
		return ripgrepRunResult{}, ripgrepUnavailableError("native fallback forced by CLAUDE_CODE_FORCE_SEARCH_FALLBACK")
	}
	rgPath, err := LocateRipgrep()
	if err != nil {
		return ripgrepRunResult{}, err
	}

	cmd := exec.CommandContext(ctx, rgPath, append(args, target)...)
	configureRipgrepTimeoutCommand(cmd)
	// grep-stdout-buffer-cap: cap captured stdout at 50 MB so an
	// exceptionally large match set cannot OOM the harness. Anything past
	// the cap is dropped — runRipgrep callers already truncate to limit
	// further upstream, so this is purely a safety floor.
	var stdout cappedBuffer
	stdout.cap = stdoutCap
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		// grep-timeout-vs-no-matches-distinct-error +
		// grep-partial-results-on-timeout: a context-deadline / cancellation
		// surfaces as ctx.Err() != nil. Return a typed RipgrepTimeoutError
		// carrying any partial stdout we captured before SIGKILL so the
		// caller can show "search timed out — N partial matches" instead
		// of conflating with the empty-result path.
		if ctx.Err() != nil {
			return ripgrepRunResult{}, &RipgrepTimeoutError{
				Partial: drainStdoutPartial(stdout.String()),
				Cause:   ctx.Err(),
			}
		}
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return ripgrepRunResult{}, nil
		}
		errText := strings.TrimSpace(stderr.String())
		if errText == "" {
			errText = err.Error()
		}
		// grep-eagain-retry: ripgrep can fail to spawn worker threads in
		// resource-constrained environments (Docker, WSL, CI). Retry once
		// with -j 1 before reporting the error.
		if isEAGAINText(errText) {
			retryArgs := append([]string{"-j", "1"}, args...)
			retryCmd := exec.CommandContext(ctx, rgPath, append(retryArgs, target)...)
			configureRipgrepTimeoutCommand(retryCmd)
			var retryStdout cappedBuffer
			retryStdout.cap = stdoutCap
			var retryStderr bytes.Buffer
			retryCmd.Stdout = &retryStdout
			retryCmd.Stderr = &retryStderr
			if rerr := retryCmd.Run(); rerr != nil {
				if ctx.Err() != nil {
					return ripgrepRunResult{}, &RipgrepTimeoutError{
						Partial: drainStdoutPartial(retryStdout.String()),
						Cause:   ctx.Err(),
					}
				}
				if rExit, ok := rerr.(*exec.ExitError); ok && rExit.ExitCode() == 1 {
					return ripgrepRunResult{}, nil
				}
				rText := strings.TrimSpace(retryStderr.String())
				if rText == "" {
					rText = rerr.Error()
				}
				return ripgrepRunResult{}, fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolLegacyCRipgrepRetryFailed, rText))
			}
			return ripgrepRunResult{
				Lines:     ripgrepOutputLines(retryStdout.String(), retryStdout.dropped),
				Truncated: retryStdout.dropped,
			}, nil
		}
		if ripgrepUnavailableText(errText) || ripgrepUnavailableText(err.Error()) {
			return ripgrepRunResult{}, ripgrepUnavailableError("ripgrep failed to execute: %s", errText)
		}
		// grep-critical-error-surface: distinguish hard failures (ENOENT,
		// EACCES, EPERM) from "no matches" (exit 1, already handled above)
		// so the user sees the real error instead of an empty result.
		lower := strings.ToLower(errText)
		if strings.Contains(lower, "no such file or directory") ||
			strings.Contains(lower, "permission denied") ||
			strings.Contains(lower, "operation not permitted") {
			return ripgrepRunResult{}, fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolLegacyCRipgrepCriticalError, errText))
		}
		return ripgrepRunResult{}, fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolLegacyCRipgrepFailed, errText))
	}

	return ripgrepRunResult{
		Lines:     ripgrepOutputLines(stdout.String(), stdout.dropped),
		Truncated: stdout.dropped,
	}, nil
}

func ripgrepOutputLines(output string, truncated bool) []string {
	if truncated {
		return drainStdoutPartial(output)
	}
	// ripgrep on Windows can emit CRLF; normalize to the TS line shape.
	output = strings.ReplaceAll(strings.TrimRight(output, "\n"), "\r", "")
	if output == "" {
		return nil
	}
	return strings.Split(output, "\n")
}

func prepareGrepPartialResults(partial []string, opts grepRipgrepOptions) []string {
	return prepareGrepResults(append([]string(nil), partial...), opts)
}

func splitSearchGlobPatterns(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	parts := strings.Fields(raw)
	patterns := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.Contains(part, "{") && strings.Contains(part, "}") {
			patterns = append(patterns, part)
			continue
		}
		for _, item := range strings.Split(part, ",") {
			item = strings.TrimSpace(item)
			if item != "" {
				patterns = append(patterns, item)
			}
		}
	}
	return patterns
}

func extractGlobBaseDirectory(pattern string) (baseDir string, relativePattern string) {
	idx := strings.IndexAny(pattern, "*?[{")
	if idx == -1 {
		return filepath.Dir(pattern), filepath.Base(pattern)
	}

	staticPrefix := pattern[:idx]
	lastSep := strings.LastIndexAny(staticPrefix, `/\`)
	if lastSep == -1 {
		return "", pattern
	}

	baseDir = staticPrefix[:lastSep]
	relativePattern = pattern[lastSep+1:]
	if baseDir == "" && lastSep == 0 {
		baseDir = string(filepath.Separator)
	}
	if len(baseDir) == 2 && baseDir[1] == ':' {
		baseDir += string(filepath.Separator)
	}
	return baseDir, relativePattern
}

func formatSearchDisplayPath(path string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return displayPathForUser(path)
	}
	return formatSearchDisplayPathFrom(path, cwd)
}

func formatSearchDisplayPathFrom(path string, cwd string) string {
	path = displayPathForUser(path)
	if strings.TrimSpace(cwd) == "" {
		resolved, err := os.Getwd()
		if err != nil {
			return path
		}
		cwd = resolved
	}
	cwd = displayPathForUser(cwd)
	rel, err := filepath.Rel(cwd, path)
	if err != nil {
		return path
	}
	if rel == "." {
		return rel
	}
	if strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return path
	}
	return rel
}

func relativizeRipgrepFileLine(line string, searchPath string, searchInfo os.FileInfo, displayRoot string) string {
	if !searchInfo.IsDir() {
		return formatSearchDisplayPathFrom(line, displayRoot)
	}
	if filepath.IsAbs(line) {
		return formatSearchDisplayPathFrom(line, displayRoot)
	}
	return formatSearchDisplayPathFrom(filepath.Join(searchPath, line), displayRoot)
}

func relativizeRipgrepCountLine(line string, searchPath string, searchInfo os.FileInfo, displayRoot string) string {
	if !searchInfo.IsDir() {
		return line
	}
	colon := strings.LastIndex(line, ":")
	if colon <= 0 {
		return line
	}
	pathPart := line[:colon]
	if filepath.IsAbs(pathPart) {
		return formatSearchDisplayPathFrom(pathPart, displayRoot) + line[colon:]
	}
	return formatSearchDisplayPathFrom(filepath.Join(searchPath, pathPart), displayRoot) + line[colon:]
}

func relativizeRipgrepContentLine(line string, searchPath string, searchInfo os.FileInfo, displayRoot string) string {
	if !searchInfo.IsDir() || line == "--" {
		return line
	}
	if !strings.HasPrefix(line, searchPath) {
		return line
	}
	searchPrefixLen := len(searchPath)
	if searchPrefixLen >= len(line) || line[searchPrefixLen] != filepath.Separator {
		return line
	}
	rest := line[searchPrefixLen:]
	sepIdx := strings.IndexAny(rest, ":-")
	if sepIdx <= 0 {
		return line
	}
	absolutePath := line[:searchPrefixLen+sepIdx]
	return formatSearchDisplayPathFrom(absolutePath, displayRoot) + line[searchPrefixLen+sepIdx:]
}

func searchResultModTime(searchPath string, line string) int64 {
	path := line
	if !filepath.IsAbs(path) {
		path = filepath.Join(searchPath, path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.ModTime().UnixNano()
}

// isBinaryContent checks if the data likely represents a binary file by looking
// for null bytes in the first chunk (same heuristic used by git and grep).
func isBinaryContent(data []byte) bool {
	for _, b := range data {
		if b == 0 {
			return true
		}
	}
	return false
}
