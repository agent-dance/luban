package inspect

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

const cursorSizeReservation = 34

type pageItemKind uint8

const (
	pageItemEmpty pageItemKind = iota
	pageItemFile
	pageItemMatch
	pageItemSnippet
	pageItemError
)

type pageItem struct {
	requestIndex int
	kind         pageItemKind
	file         string
	match        Match
	snippetID    string
	snippetIDs   []string
	requestError RequestError
}

type paginationState struct {
	batch      batchResult
	items      []pageItem
	snippets   map[string]Snippet
	next       int
	pageLimits limits
	sourceLoss bool
}

func (s *paginationState) estimatedBytes() int {
	if s == nil {
		return 0
	}
	total := len(s.items) * 96
	for _, completed := range s.batch.requests {
		total += len(completed.result.ID) + len(completed.result.Kind) + len(completed.result.Path)
		for _, file := range completed.result.Files {
			total += len(file)
		}
		for _, match := range completed.result.Matches {
			total += len(match.Path) + len(match.Text) + len(match.SnippetIDs)*24
		}
		for _, requestError := range completed.result.Errors {
			total += len(requestError.Code) + len(requestError.Message)
		}
	}
	for _, snippet := range s.snippets {
		total += len(snippet.ID) + len(snippet.Path) + len(snippet.Content) + 64
	}
	return total
}

func newPaginationState(batch batchResult, pageLimits limits) *paginationState {
	state := &paginationState{
		batch: batch, snippets: make(map[string]Snippet), pageLimits: pageLimits,
	}
	buckets := make([][]pageItem, len(batch.requests))
	for requestIndex, completed := range batch.requests {
		for _, snippet := range completed.snippets {
			state.snippets[snippet.ID] = snippet
		}
		if completed.result.SourcePartial || len(completed.result.Errors) > 0 {
			state.sourceLoss = true
		}
		itemCount := 0
		for _, file := range completed.result.Files {
			buckets[requestIndex] = append(buckets[requestIndex], pageItem{requestIndex: requestIndex, kind: pageItemFile, file: file})
			itemCount++
		}
		for _, match := range completed.result.Matches {
			buckets[requestIndex] = append(buckets[requestIndex], pageItem{
				requestIndex: requestIndex, kind: pageItemMatch, file: match.Path,
				match: match, snippetIDs: append([]string(nil), match.SnippetIDs...),
			})
			itemCount++
		}
		// Search evidence is an indivisible part of its match item. Exact reads
		// retain snippet-sized atomic units so large files can paginate safely.
		if completed.result.Kind == KindRead {
			for _, snippetID := range completed.result.SnippetIDs {
				snippet := state.snippets[snippetID]
				buckets[requestIndex] = append(buckets[requestIndex], pageItem{requestIndex: requestIndex, kind: pageItemSnippet, file: snippet.Path, snippetID: snippetID})
				itemCount++
			}
		}
		for _, requestError := range completed.result.Errors {
			buckets[requestIndex] = append(buckets[requestIndex], pageItem{requestIndex: requestIndex, kind: pageItemError, requestError: requestError})
			itemCount++
		}
		if itemCount == 0 {
			buckets[requestIndex] = append(buckets[requestIndex], pageItem{requestIndex: requestIndex, kind: pageItemEmpty})
		}
	}
	requestOrder := make([]int, 0, len(batch.requests))
	for index, completed := range batch.requests {
		if completed.result.Kind == KindRead {
			requestOrder = append(requestOrder, index)
		}
	}
	for index, completed := range batch.requests {
		if completed.result.Kind != KindRead {
			requestOrder = append(requestOrder, index)
		}
	}
	// Stable round-robin prevents one large request from monopolizing the
	// first view while still giving exact read evidence first priority.
	for round := 0; ; round++ {
		added := false
		for _, requestIndex := range requestOrder {
			if round < len(buckets[requestIndex]) {
				state.items = append(state.items, buckets[requestIndex][round])
				added = true
			}
		}
		if !added {
			break
		}
	}
	return state
}

type pageBuilder struct {
	result           Result
	requestOffsets   map[int]int
	snippetIDs       map[string]struct{}
	files            map[string]struct{}
	items            int
	matches          int
	includedRequests map[int]struct{}
}

func newPageBuilder(generation string, state *paginationState) *pageBuilder {
	builder := &pageBuilder{
		result:         Result{Generation: generation, Requests: make([]RequestResult, 0, len(state.batch.requests)), Snippets: []Snippet{}},
		requestOffsets: make(map[int]int), snippetIDs: make(map[string]struct{}), files: make(map[string]struct{}),
		includedRequests: make(map[int]struct{}),
	}
	// Every request receives a compact routing envelope on every page. Payload
	// is populated below; omitted_request_ids distinguishes deferred payload
	// from a completed empty request.
	for index, completed := range state.batch.requests {
		base := completed.result
		base.Files, base.Matches, base.SnippetIDs, base.Errors = nil, nil, nil, nil
		builder.requestOffsets[index] = len(builder.result.Requests)
		builder.result.Requests = append(builder.result.Requests, base)
	}
	return builder
}

func (b *pageBuilder) add(state *paginationState, item pageItem) {
	requestOffset := b.requestOffsets[item.requestIndex]
	b.includedRequests[item.requestIndex] = struct{}{}
	request := &b.result.Requests[requestOffset]
	switch item.kind {
	case pageItemFile:
		request.Files = append(request.Files, item.file)
		b.addFile(item.file)
	case pageItemMatch:
		request.Matches = append(request.Matches, item.match)
		b.matches++
		b.addFile(item.file)
		for _, snippetID := range item.snippetIDs {
			b.addSnippet(state, snippetID)
		}
	case pageItemSnippet:
		request.SnippetIDs = appendUniqueString(request.SnippetIDs, item.snippetID)
		b.addFile(item.file)
		b.addSnippet(state, item.snippetID)
	case pageItemError:
		request.Errors = append(request.Errors, item.requestError)
	case pageItemEmpty:
	}
	b.items++
}

func (b *pageBuilder) addFile(path string) {
	if strings.TrimSpace(path) != "" {
		b.files[path] = struct{}{}
	}
}

func (b *pageBuilder) addSnippet(state *paginationState, id string) {
	if _, exists := b.snippetIDs[id]; exists {
		return
	}
	snippet, exists := state.snippets[id]
	if !exists {
		return
	}
	b.snippetIDs[id] = struct{}{}
	b.result.Snippets = append(b.result.Snippets, snippet)
}

func (b *pageBuilder) encodedSizeWithCursorReservation(view *evidenceView) int {
	copy := b.result
	copy.HasMoreView = true
	copy.Cursor = strings.Repeat("x", cursorSizeReservation)
	copy.Stats = ResultStats{
		Requests: len(b.result.Requests), Files: len(b.files), Matches: b.matches,
		Snippets: len(b.result.Snippets), Items: b.items,
	}
	encoded, _, err := marshalModelResult(copy, view)
	if err != nil {
		return maximumMaxChars + 1
	}
	return len(encoded)
}

func (s *paginationState) nextPage(generation string, views ...*evidenceView) (Result, int, int) {
	var view *evidenceView
	if len(views) > 0 {
		view = views[0]
	}
	start := s.next
	capEnd := s.pageCapEnd(start)
	bestEnd := start
	low, high := start+1, capEnd
	for low <= high {
		middle := low + (high-low)/2
		candidate := s.buildPage(generation, start, middle)
		if candidate.encodedSizeWithCursorReservation(view) <= s.pageLimits.maxChars {
			bestEnd = middle
			low = middle + 1
		} else {
			high = middle - 1
		}
	}
	if bestEnd == start && start < capEnd {
		// Every atomic snippet and path is bounded against the conservative
		// max_chars minimum, so this branch is only for pathological JSON
		// escaping. Advancing avoids a non-progressing cursor; renderPage still
		// verifies the final byte contract before exposing the result.
		bestEnd = start + 1
	}
	builder := s.buildPage(generation, start, bestEnd)
	s.next = bestEnd
	result := builder.result
	result.Stats = ResultStats{
		Requests: len(result.Requests), Files: len(builder.files), Matches: builder.matches,
		Snippets: len(result.Snippets), Items: builder.items,
	}
	result.HasMoreView = s.next < len(s.items)
	result.SourceTruncated = s.sourceLoss
	return result, start, s.next
}

func (s *paginationState) pageCapEnd(start int) int {
	files := make(map[string]struct{})
	requestItems := make(map[int]int)
	matches := 0
	end := start
	for end < len(s.items) {
		item := s.items[end]
		if pageSize := s.batch.requests[item.requestIndex].pageSize; pageSize > 0 && requestItems[item.requestIndex] >= pageSize {
			break
		}
		if item.kind == pageItemMatch && matches >= s.pageLimits.maxMatches {
			break
		}
		if item.file != "" {
			if _, exists := files[item.file]; !exists && len(files) >= s.pageLimits.maxFiles {
				break
			}
			files[item.file] = struct{}{}
		}
		if item.kind == pageItemMatch {
			matches++
		}
		requestItems[item.requestIndex]++
		end++
	}
	return end
}

func (s *paginationState) buildPage(generation string, start, end int) *pageBuilder {
	builder := newPageBuilder(generation, s)
	for index := start; index < end; index++ {
		builder.add(s, s.items[index])
	}
	pending := make(map[int]struct{})
	for index := end; index < len(s.items); index++ {
		pending[s.items[index].requestIndex] = struct{}{}
	}
	for requestIndex := range pending {
		if _, included := builder.includedRequests[requestIndex]; included {
			continue
		}
		builder.result.OmittedRequestIDs = append(builder.result.OmittedRequestIDs, s.batch.requests[requestIndex].result.ID)
	}
	sort.Strings(builder.result.OmittedRequestIDs)
	builder.result.HasMoreView = end < len(s.items)
	builder.result.SourceTruncated = s.sourceLoss
	return builder
}

func (t *Tool) renderPage(ctx context.Context, workspace workspaceSnapshot, generation string, state *paginationState) (types.ToolResult, error) {
	if state == nil {
		return resultError(localizedError(i18n.KeyToolInspectCursorInvalid)), nil
	}
	store := t.evidence
	if store == nil {
		store = newEvidenceStore()
		t.evidence = store
	}
	var rendered types.ToolResult
	namespace := evidenceNamespaceKey(workspace)
	if !workspace.historyBound {
		// Never let an unowned or stale execution context establish references
		// to evidence that may not exist in its visible history.
		namespace += "\x00" + generation
	}
	err := store.withNamespace(namespace, func(view *evidenceView) error {
		page, offset, nextOffset := state.nextPage(generation, view)
		hasMore := page.HasMoreView
		if hasMore {
			page.Cursor = t.cursorStore().put(workspace, generation, state)
		}
		wire, observations := projectModelResult(page, view)
		encoded, encodeErr := json.Marshal(wire)
		if encodeErr != nil || len(encoded) > state.pageLimits.maxChars {
			return localizedError(i18n.KeyToolInspectResultEncodingFailed)
		}
		page.modelContent = string(encoded)
		page.modelStats = wire.Stats
		if !workspace.deferEvidenceCommit {
			view.observe(observations)
		}
		page.visibleReceipt = t.newVisibleEvidenceReceipt(ctx, workspace, page.modelContent, page, state, observations)

		completeness := types.ToolResultCompleteness{Source: types.ToolResultCompletenessComplete}
		if page.SourceTruncated {
			completeness.Source = types.ToolResultCompletenessSourceTruncated
		}
		if hasMore {
			completeness.View = types.ToolResultCompletenessPagination
			completeness.Pagination = &types.ToolResultPagination{
				Offset: offset, Limit: nextOffset - offset, NextOffset: nextOffset, HasMore: true,
			}
		}
		outcome := types.ToolOutcomeSucceeded
		if page.HasMoreView || page.SourceTruncated {
			outcome = types.ToolOutcomePartial
		}
		metadata := map[string]string{
			"generation":       generation,
			"has_more_view":    strconv.FormatBool(page.HasMoreView),
			"source_truncated": strconv.FormatBool(page.SourceTruncated),
			"requests":         strconv.Itoa(page.Stats.Requests),
			"files":            strconv.Itoa(page.Stats.Files),
			"matches":          strconv.Itoa(page.Stats.Matches),
			"snippets":         strconv.Itoa(page.Stats.Snippets),
			"items":            strconv.Itoa(page.Stats.Items),
			"new_chars":        strconv.Itoa(page.modelStats.NewChars),
			"reused_chars":     strconv.Itoa(page.modelStats.ReusedChars),
		}
		for key, value := range inspectPartialDiagnosticMetadata(state.batch) {
			metadata[key] = value
		}
		if len(page.OmittedRequestIDs) > 0 {
			metadata["omitted_request_ids"] = strings.Join(page.OmittedRequestIDs, ",")
		}
		rendered = types.ToolResult{
			Content: string(encoded), Data: page, Metadata: metadata, Outcome: outcome,
			Completeness: completeness,
		}
		return nil
	})
	if err != nil {
		return resultError(err), nil
	}
	if !workspace.deferEvidenceCommit {
		if page, ok := rendered.Data.(Result); ok && page.visibleReceipt != nil {
			page.visibleReceipt.commit(rendered.Content)
		}
	}
	return rendered, nil
}
