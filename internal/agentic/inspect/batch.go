package inspect

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/agent-dance/luban/i18n"
	toolfile "github.com/agent-dance/luban/internal/tools/file"
	toolsearch "github.com/agent-dance/luban/internal/tools/search"
	"github.com/agent-dance/luban/types"
)

const batchWorkerLimit = 4

type batchResult struct {
	generation string
	requests   []completedRequest
	sources    map[string]sourceSnapshot
}

type completedRequest struct {
	result   RequestResult
	snippets []Snippet
	sources  map[string]sourceSnapshot
	pageSize int
}

// sourceSnapshot is captured from the private Read ledger used while Inspect
// acquires source. It is deliberately not the shared mutation-authority
// ledger: only snippets that survive model pagination may later publish a
// visible-evidence receipt derived from this snapshot.
type sourceSnapshot struct {
	absPath         string
	displayPath     string
	entry           toolfile.ReadFileEntry
	fullContent     string
	fullContentRead bool
	lineByteLengths map[int]int
	conflicted      bool
}

type rangeRead struct {
	path      string
	startLine int
	content   string
	snippets  []Snippet
	source    *sourceSnapshot
	err       *RequestError
}

func (t *Tool) executeBatch(ctx context.Context, workspace workspaceSnapshot, generation string, requests []normalizedRequest) batchResult {
	completed := make([]completedRequest, len(requests))
	jobs := make(chan int)
	workerCount := len(requests)
	if workerCount > batchWorkerLimit {
		workerCount = batchWorkerLimit
	}
	var wait sync.WaitGroup
	wait.Add(workerCount)
	for worker := 0; worker < workerCount; worker++ {
		go func() {
			defer wait.Done()
			for index := range jobs {
				completed[index] = t.executeRequest(ctx, workspace, requests[index])
			}
		}()
	}
	for index := range requests {
		jobs <- index
	}
	close(jobs)
	wait.Wait()
	sources, conflicts := mergeBatchSources(completed)
	if len(conflicts) > 0 {
		for index := range completed {
			for _, snippet := range completed[index].snippets {
				if _, conflict := conflicts[snippet.Path]; !conflict {
					continue
				}
				completed[index].result.SourcePartial = true
				completed[index].result.PartialReason = "workspace_changed"
				break
			}
		}
	}
	return batchResult{generation: generation, requests: completed, sources: sources}
}

func (t *Tool) executeRequest(ctx context.Context, workspace workspaceSnapshot, request normalizedRequest) completedRequest {
	result := RequestResult{ID: request.id, Kind: request.kind, Path: filepath.ToSlash(request.path)}
	absPath, err := resolveRepositoryPath(workspace.root, request.path)
	if err != nil {
		result.Errors = []RequestError{newRequestError("path_scope", err)}
		return completedRequest{result: result, pageSize: request.maxResults}
	}
	displayPath := repositoryDisplayPath(workspace.root, absPath)
	result.Path = displayPath

	switch request.kind {
	case KindRead:
		return t.executeReadRequest(ctx, workspace, request, result, absPath)
	case KindGlob:
		globRuntime := searchRuntimeSnapshot(workspace.runtime)
		globResult, globErr := toolsearch.RunInspectGlob(ctx, globRuntime, absPath, request.pattern, request.maxResults)
		if globErr != nil {
			result.Errors = []RequestError{newRequestError("glob_failed", globErr)}
			return completedRequest{result: result}
		}
		result.Files = append([]string(nil), globResult.Files...)
		if globResult.HasMore || globResult.PartialReason != "" {
			result.SourcePartial = true
			result.PartialReason = sourcePartialReason(globResult.PartialReason, globResult.HasMore)
		}
		return completedRequest{result: result, pageSize: request.maxResults}
	case KindSearch:
		return t.executeSearchRequest(ctx, workspace, request, result, absPath)
	default:
		return completedRequest{result: result}
	}
}

func (t *Tool) executeReadRequest(ctx context.Context, workspace workspaceSnapshot, request normalizedRequest, result RequestResult, absPath string) completedRequest {
	if info, err := os.Stat(absPath); err == nil && info.IsDir() {
		entries, readErr := os.ReadDir(absPath)
		if readErr != nil {
			result.Errors = []RequestError{newRequestError("read_failed", readErr)}
			result.SourcePartial = true
			result.PartialReason = "read_failed"
			return completedRequest{result: result}
		}
		limit := len(entries)
		if limit > maximumMaxFiles {
			limit = maximumMaxFiles
			result.SourcePartial = true
			result.PartialReason = "max_results"
		}
		result.Files = make([]string, 0, limit)
		for _, entry := range entries[:limit] {
			path := repositoryDisplayPath(workspace.root, filepath.Join(absPath, entry.Name()))
			if entry.IsDir() {
				path += "/"
			}
			result.Files = append(result.Files, path)
		}
		return completedRequest{result: result}
	}
	ranges := request.ranges
	if len(ranges) == 0 {
		ranges = []normalizedRange{{}}
	}
	reads := t.readRanges(ctx, workspace, absPath, repositoryDisplayPath(workspace.root, absPath), ranges)
	snippets := make([]Snippet, 0)
	sources := make(map[string]sourceSnapshot)
	for _, read := range reads {
		if read.err != nil {
			result.Errors = append(result.Errors, *read.err)
			continue
		}
		for _, snippet := range read.snippets {
			result.SnippetIDs = appendUniqueString(result.SnippetIDs, snippet.ID)
			snippets = append(snippets, snippet)
		}
		mergeRequestSource(sources, read.source)
	}
	if len(result.Errors) > 0 {
		result.SourcePartial = true
		result.PartialReason = "read_failed"
	}
	return completedRequest{result: result, snippets: deduplicateSnippets(snippets), sources: sources}
}

func (t *Tool) executeSearchRequest(ctx context.Context, workspace workspaceSnapshot, request normalizedRequest, result RequestResult, _ string) completedRequest {
	runtime := searchRuntimeSnapshot(workspace.runtime)
	searchResult, err := toolsearch.RunInspectSearch(ctx, runtime, request.path, request.pattern, request.maxResults)
	if err != nil {
		result.Errors = []RequestError{newRequestError("search_failed", err)}
		return completedRequest{result: result}
	}

	result.Matches = make([]Match, 0, len(searchResult.Matches))
	rangesByPath := make(map[string][]normalizedRange)
	matchText := make(map[string]string, len(searchResult.Matches))
	for _, match := range searchResult.Matches {
		path := filepath.ToSlash(filepath.Clean(match.Path))
		start := match.Line - request.context
		if start < 1 {
			start = 1
		}
		end := match.Line + request.context
		rangesByPath[path] = append(rangesByPath[path], normalizedRange{start: start, end: end})
		key := matchLineKey(path, match.Line)
		matchText[key] = match.Text
		result.Matches = append(result.Matches, Match{Path: path, Line: match.Line, Text: match.Text})
	}

	paths := make([]string, 0, len(rangesByPath))
	for path := range rangesByPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	type pathEvidence struct {
		path  string
		reads []rangeRead
	}
	evidence := make([]pathEvidence, len(paths))
	jobs := make(chan int)
	workerCount := len(paths)
	if workerCount > batchWorkerLimit {
		workerCount = batchWorkerLimit
	}
	var wait sync.WaitGroup
	wait.Add(workerCount)
	for worker := 0; worker < workerCount; worker++ {
		go func() {
			defer wait.Done()
			for index := range jobs {
				path := paths[index]
				absPath, pathErr := resolveRepositoryPath(workspace.root, path)
				if pathErr != nil {
					errorValue := newRequestError("evidence_path_scope", pathErr)
					evidence[index] = pathEvidence{path: path, reads: []rangeRead{{path: path, err: &errorValue}}}
					continue
				}
				evidence[index] = pathEvidence{
					path:  path,
					reads: t.readRanges(ctx, workspace, absPath, path, mergeRanges(rangesByPath[path])),
				}
			}
		}()
	}
	for index := range paths {
		jobs <- index
	}
	close(jobs)
	wait.Wait()

	snippets := make([]Snippet, 0)
	lineContents := make(map[string]string)
	sources := make(map[string]sourceSnapshot)
	for _, pathResult := range evidence {
		for _, read := range pathResult.reads {
			if read.err != nil {
				result.Errors = append(result.Errors, *read.err)
				continue
			}
			for offset, line := range strings.Split(read.content, "\n") {
				lineContents[matchLineKey(pathResult.path, read.startLine+offset)] = strings.TrimSuffix(line, "\r")
			}
			snippets = append(snippets, read.snippets...)
			mergeRequestSource(sources, read.source)
		}
	}
	snippets = deduplicateSnippets(snippets)
	stableMatches := result.Matches[:0]
	for _, match := range result.Matches {
		key := matchLineKey(match.Path, match.Line)
		actual, observed := lineContents[key]
		if !observed || actual != matchText[key] {
			result.SourcePartial = true
			if result.PartialReason == "" {
				result.PartialReason = "workspace_changed"
			}
			continue
		}
		start, end := match.Line-request.context, match.Line+request.context
		if start < 1 {
			start = 1
		}
		candidates := make([]Snippet, 0)
		for _, snippet := range snippets {
			if snippet.Path == match.Path && snippet.StartLine <= end && snippet.EndLine >= start {
				candidates = append(candidates, snippet)
			}
		}
		sort.SliceStable(candidates, func(i, j int) bool {
			leftMatch := candidates[i].StartLine <= match.Line && candidates[i].EndLine >= match.Line
			rightMatch := candidates[j].StartLine <= match.Line && candidates[j].EndLine >= match.Line
			if leftMatch != rightMatch {
				return leftMatch
			}
			leftDistance := absLineDistance(candidates[i], match.Line)
			rightDistance := absLineDistance(candidates[j], match.Line)
			if leftDistance != rightDistance {
				return leftDistance < rightDistance
			}
			if candidates[i].StartLine != candidates[j].StartLine {
				return candidates[i].StartLine < candidates[j].StartLine
			}
			return candidates[i].StartColumn < candidates[j].StartColumn
		})
		for _, snippet := range candidates {
			match.SnippetIDs = appendUniqueString(match.SnippetIDs, snippet.ID)
		}
		if len(match.SnippetIDs) > maximumAtomicSearchSnippets {
			match.SnippetIDs = append([]string(nil), match.SnippetIDs[:maximumAtomicSearchSnippets]...)
			result.SourcePartial = true
			if result.PartialReason == "" {
				result.PartialReason = "evidence_budget"
			}
		}
		if len(match.SnippetIDs) > 0 {
			match.Text = ""
		}
		for _, snippetID := range match.SnippetIDs {
			result.SnippetIDs = appendUniqueString(result.SnippetIDs, snippetID)
		}
		stableMatches = append(stableMatches, match)
	}
	result.Matches = stableMatches
	if searchResult.HasMore || searchResult.PartialReason != "" {
		result.SourcePartial = true
		if result.PartialReason == "" {
			result.PartialReason = sourcePartialReason(searchResult.PartialReason, searchResult.HasMore)
		}
	}
	if len(result.Errors) > 0 {
		result.SourcePartial = true
		if result.PartialReason == "" {
			result.PartialReason = "evidence_failed"
		}
	}
	return completedRequest{result: result, snippets: snippets, sources: sources, pageSize: request.maxResults}
}

func absLineDistance(snippet Snippet, line int) int {
	if line < snippet.StartLine {
		return snippet.StartLine - line
	}
	if line > snippet.EndLine {
		return line - snippet.EndLine
	}
	return 0
}

func (t *Tool) readRanges(ctx context.Context, workspace workspaceSnapshot, absPath, displayPath string, ranges []normalizedRange) []rangeRead {
	provider := immutableRuntimeProvider{runtime: workspace.runtime}
	// FileRead captures a complete descriptor-bound digest even for a ranged
	// request. Keep that rich acquisition state private until pagination has
	// established which exact source fragments will enter model-visible history.
	scratchState := toolfile.NewReadFileState()
	readTool := &toolfile.FileReadTool{Runtime: provider, ReadState: scratchState}
	reads := make([]rangeRead, 0, len(ranges))
	for _, lineRange := range ranges {
		input := map[string]any{"file_path": absPath}
		if lineRange.start > 0 {
			input["offset"] = float64(lineRange.start)
			input["limit"] = float64(lineRange.end - lineRange.start + 1)
		}
		toolResult, err := readTool.Execute(ctx, input)
		if err != nil {
			requestError := newRequestError("read_failed", err)
			reads = append(reads, rangeRead{path: displayPath, err: &requestError})
			continue
		}
		if toolResult.IsError {
			requestError := newRequestErrorText("read_failed", toolResult.Content)
			reads = append(reads, rangeRead{path: displayPath, err: &requestError})
			continue
		}
		output, ok := toolResult.Data.(toolfile.FileReadOutput)
		if !ok {
			requestError := newRequestErrorText("read_failed", toolResult.Content)
			reads = append(reads, rangeRead{path: displayPath, err: &requestError})
			continue
		}
		if output.Type == toolfile.FileReadVariantFileUnchanged {
			entry, found := scratchState.GetForContext(ctx, filepath.Clean(absPath))
			if !found {
				requestError := newRequestErrorText("read_failed", toolResult.Content)
				reads = append(reads, rangeRead{path: displayPath, err: &requestError})
				continue
			}
			start := entry.Offset
			content := entry.Content
			if entry.FullSnapshot {
				start = 1
				if lineRange.start > 0 {
					start = lineRange.start
					content = sliceFullSnapshotRange(entry.Content, lineRange)
				}
			} else if start <= 0 {
				start = 1
			}
			source := newSourceSnapshot(absPath, displayPath, entry, start, content)
			reads = append(reads, rangeRead{
				path: displayPath, startLine: start, content: content,
				snippets: splitSnippet(displayPath, start, content), source: &source,
			})
			continue
		}
		if output.Type != toolfile.FileReadVariantText {
			requestError := newRequestErrorText("unsupported_content", localizedTextOnly(displayPath))
			reads = append(reads, rangeRead{path: displayPath, err: &requestError})
			continue
		}
		start := output.File.StartLine
		if start <= 0 {
			start = 1
		}
		entry, found := scratchState.GetForContext(ctx, filepath.Clean(absPath))
		if !found {
			requestError := newRequestErrorText("read_failed", toolResult.Content)
			reads = append(reads, rangeRead{path: displayPath, err: &requestError})
			continue
		}
		source := newSourceSnapshot(absPath, displayPath, entry, start, output.File.Content)
		reads = append(reads, rangeRead{
			path: displayPath, startLine: start, content: output.File.Content,
			snippets: splitSnippet(displayPath, start, output.File.Content), source: &source,
		})
	}
	return reads
}

func newSourceSnapshot(absPath, displayPath string, entry toolfile.ReadFileEntry, startLine int, content string) sourceSnapshot {
	snapshot := sourceSnapshot{
		absPath: absPath, displayPath: displayPath, entry: entry,
		lineByteLengths: make(map[int]int),
	}
	for offset, line := range strings.Split(content, "\n") {
		snapshot.lineByteLengths[startLine+offset] = len(strings.TrimSuffix(line, "\r"))
	}
	if entry.FullSnapshot {
		snapshot.fullContent = entry.Content
		snapshot.fullContentRead = true
		if snapshot.fullContent == "" && entry.TotalLines == 0 {
			snapshot.fullContent = content
		}
	}
	return snapshot
}

func mergeRequestSource(target map[string]sourceSnapshot, source *sourceSnapshot) {
	if source == nil || source.displayPath == "" {
		return
	}
	current, exists := target[source.displayPath]
	if !exists {
		target[source.displayPath] = cloneSourceSnapshot(*source)
		return
	}
	if current.conflicted {
		return
	}
	if !sameSourceVersion(current, *source) {
		target[source.displayPath] = sourceSnapshot{displayPath: source.displayPath, conflicted: true}
		return
	}
	for line, length := range source.lineByteLengths {
		current.lineByteLengths[line] = length
	}
	if !current.fullContentRead && source.fullContentRead {
		current.fullContent = source.fullContent
		current.fullContentRead = true
	}
	current.entry = source.entry
	target[source.displayPath] = current
}

func mergeBatchSources(requests []completedRequest) (map[string]sourceSnapshot, map[string]struct{}) {
	merged := make(map[string]sourceSnapshot)
	conflicts := make(map[string]struct{})
	for _, request := range requests {
		for path, source := range request.sources {
			if _, conflict := conflicts[path]; conflict {
				continue
			}
			if source.conflicted {
				delete(merged, path)
				conflicts[path] = struct{}{}
				continue
			}
			current, exists := merged[path]
			if !exists {
				merged[path] = cloneSourceSnapshot(source)
				continue
			}
			if !sameSourceVersion(current, source) {
				delete(merged, path)
				conflicts[path] = struct{}{}
				continue
			}
			for line, length := range source.lineByteLengths {
				current.lineByteLengths[line] = length
			}
			if !current.fullContentRead && source.fullContentRead {
				current.fullContent = source.fullContent
				current.fullContentRead = true
			}
			current.entry = source.entry
			merged[path] = current
		}
	}
	return merged, conflicts
}

func sameSourceVersion(left, right sourceSnapshot) bool {
	if left.entry.ContentDigest == "" || left.entry.ContentDigest != right.entry.ContentDigest ||
		left.entry.FileIdentity == nil || right.entry.FileIdentity == nil {
		return false
	}
	return os.SameFile(left.entry.FileIdentity, right.entry.FileIdentity)
}

func cloneSourceSnapshot(source sourceSnapshot) sourceSnapshot {
	cloned := source
	cloned.lineByteLengths = make(map[int]int, len(source.lineByteLengths))
	for line, length := range source.lineByteLengths {
		cloned.lineByteLengths[line] = length
	}
	return cloned
}

func sliceFullSnapshotRange(content string, lineRange normalizedRange) string {
	if lineRange.start <= 0 || lineRange.end < lineRange.start {
		return content
	}
	lines := strings.Split(content, "\n")
	start := lineRange.start - 1
	if start >= len(lines) {
		return ""
	}
	end := lineRange.end
	if end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[start:end], "\n")
}

func searchRuntimeSnapshot(runtime types.ToolRuntimeContext) types.ToolRuntimeContext {
	out := cloneRuntimeContext(runtime)
	// Search cannot interactively approve paths it discovers after preflight.
	// Treat Read ask-rules as search exclusions so Inspect never turns path
	// discovery into an approval bypass.
	for _, rule := range runtime.AskRules {
		if strings.EqualFold(strings.TrimSpace(rule.ToolName), "Read") {
			out.DeniedRules = append(out.DeniedRules, rule)
		}
	}
	return out
}

func sourcePartialReason(reason string, hasMore bool) string {
	if strings.TrimSpace(reason) != "" {
		return reason
	}
	if hasMore {
		return "request_limit"
	}
	return ""
}

func repositoryDisplayPath(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(filepath.Clean(path))
	}
	return filepath.ToSlash(relative)
}

func mergeRanges(ranges []normalizedRange) []normalizedRange {
	if len(ranges) < 2 {
		return append([]normalizedRange(nil), ranges...)
	}
	out := append([]normalizedRange(nil), ranges...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].start == out[j].start {
			return out[i].end < out[j].end
		}
		return out[i].start < out[j].start
	})
	merged := out[:0]
	for _, lineRange := range out {
		if len(merged) == 0 || lineRange.start > merged[len(merged)-1].end+1 {
			merged = append(merged, lineRange)
			continue
		}
		if lineRange.end > merged[len(merged)-1].end {
			merged[len(merged)-1].end = lineRange.end
		}
	}
	return merged
}

func splitSnippet(path string, startLine int, content string) []Snippet {
	if content == "" {
		return []Snippet{newSnippet(path, startLine, startLine, 0, 0, "")}
	}
	lines := strings.Split(content, "\n")
	snippets := make([]Snippet, 0, len(lines)/8+1)
	var builder strings.Builder
	chunkStart := startLine
	hasChunk := false
	flush := func(endLine int) {
		if !hasChunk {
			return
		}
		snippets = append(snippets, newSnippet(path, chunkStart, endLine, 0, 0, builder.String()))
		builder.Reset()
		hasChunk = false
	}
	for index, line := range lines {
		lineNumber := startLine + index
		if len(line) > maximumSnippetBytes {
			flush(lineNumber - 1)
			for column := 0; column < len(line); {
				end := column + maximumSnippetBytes
				if end > len(line) {
					end = len(line)
				}
				for end > column && !utf8.ValidString(line[column:end]) {
					end--
				}
				if end == column {
					_, width := utf8.DecodeRuneInString(line[column:])
					end = column + width
				}
				snippets = append(snippets, newSnippet(path, lineNumber, lineNumber, column+1, end, line[column:end]))
				column = end
			}
			chunkStart = lineNumber + 1
			continue
		}
		additional := len(line)
		if hasChunk {
			additional++
		}
		if hasChunk && builder.Len()+additional > maximumSnippetBytes {
			flush(lineNumber - 1)
			chunkStart = lineNumber
		}
		if hasChunk {
			builder.WriteByte('\n')
		}
		builder.WriteString(line)
		hasChunk = true
	}
	flush(startLine + len(lines) - 1)
	return snippets
}

func newSnippet(path string, startLine, endLine, startColumn, endColumn int, content string) Snippet {
	digest := sha256.Sum256([]byte(path + "\x00" + integerString(startLine) + "\x00" + integerString(endLine) + "\x00" + integerString(startColumn) + "\x00" + content))
	return Snippet{
		ID: "s_" + hex.EncodeToString(digest[:8]), Path: path,
		StartLine: startLine, EndLine: endLine, StartColumn: startColumn,
		EndColumn: endColumn, Content: content,
	}
}

func deduplicateSnippets(snippets []Snippet) []Snippet {
	if len(snippets) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(snippets))
	out := make([]Snippet, 0, len(snippets))
	for _, snippet := range snippets {
		if _, exists := seen[snippet.ID]; exists {
			continue
		}
		seen[snippet.ID] = struct{}{}
		out = append(out, snippet)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		if out[i].StartLine != out[j].StartLine {
			return out[i].StartLine < out[j].StartLine
		}
		if out[i].StartColumn != out[j].StartColumn {
			return out[i].StartColumn < out[j].StartColumn
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func appendUniqueString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func matchLineKey(path string, line int) string {
	return path + "\x00" + integerString(line)
}

func integerString(value int) string {
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	var buffer [24]byte
	index := len(buffer)
	for value > 0 {
		index--
		buffer[index] = byte('0' + value%10)
		value /= 10
	}
	if negative {
		index--
		buffer[index] = '-'
	}
	return string(buffer[index:])
}

func newRequestError(code string, err error) RequestError {
	if err == nil {
		return RequestError{Code: code}
	}
	return newRequestErrorText(code, err.Error())
}

func newRequestErrorText(code, message string) RequestError {
	return RequestError{Code: code, Message: compactUTF8(message, maximumErrorBytes)}
}

func compactUTF8(value string, maximum int) string {
	if len(value) <= maximum {
		return value
	}
	end := maximum
	for end > 0 && !utf8.ValidString(value[:end]) {
		end--
	}
	return value[:end] + "…"
}

func localizedTextOnly(path string) string {
	return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolInspectTextOnly, path)
}
