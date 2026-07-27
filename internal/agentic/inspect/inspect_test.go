package inspect

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/agent-dance/luban/internal/contracts/workspacerevision"
	toolfile "github.com/agent-dance/luban/internal/tools/file"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type testRuntimeProvider struct {
	runtime types.ToolRuntimeContext
}

type switchingRuntimeProvider struct {
	mu      sync.Mutex
	values  []types.ToolRuntimeContext
	samples int
}

func (p *switchingRuntimeProvider) ToolRuntimeContext() types.ToolRuntimeContext {
	p.mu.Lock()
	defer p.mu.Unlock()
	value := p.values[p.samples%len(p.values)]
	p.samples++
	return cloneRuntimeContext(value)
}

func (p testRuntimeProvider) ToolRuntimeContext() types.ToolRuntimeContext {
	return cloneRuntimeContext(p.runtime)
}

func TestInspectMixedBatchIsStableAndRecordsSearchEvidence(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "alpha.go")
	writeInspectFixture(t, path, "package alpha\n\nfunc first() {}\n// needle\nfunc last() {}\n")
	writeInspectFixture(t, filepath.Join(root, "notes.md"), "needle in notes\n")

	state := toolfile.NewReadFileState()
	tool := New(testRuntimeProvider{runtime: testRuntime(root)}, state)
	result, err := tool.Execute(context.Background(), map[string]any{
		"requests": []any{
			map[string]any{
				"id": "read-alpha", "kind": KindRead, "path": "alpha.go",
				"ranges": []any{map[string]any{"start": 2, "end": 4}},
			},
			map[string]any{
				"id": "find-needle", "kind": KindSearch, "path": ".", "pattern": "needle", "context": 1,
			},
			map[string]any{
				"id": "go-files", "kind": KindGlob, "path": ".", "pattern": "**/*.go",
			},
		},
	})
	if err != nil || result.IsError {
		t.Fatalf("Inspect failed: err=%v result=%+v", err, result)
	}
	output, ok := result.Data.(Result)
	if !ok {
		t.Fatalf("Inspect data type = %T", result.Data)
	}
	if len(output.Requests) != 3 {
		t.Fatalf("request results = %#v", output.Requests)
	}
	for index, want := range []string{"read-alpha", "find-needle", "go-files"} {
		if output.Requests[index].ID != want {
			t.Fatalf("request order[%d] = %q, want %q", index, output.Requests[index].ID, want)
		}
	}
	if got := output.Requests[2].Files; len(got) != 1 || got[0] != "alpha.go" {
		t.Fatalf("glob files = %#v", got)
	}
	if got := len(output.Requests[1].Matches); got != 2 {
		t.Fatalf("search matches = %#v", output.Requests[1].Matches)
	}
	seenSnippets := make(map[string]struct{}, len(output.Snippets))
	for _, snippet := range output.Snippets {
		if _, exists := seenSnippets[snippet.ID]; exists {
			t.Fatalf("duplicate snippet id %q in %#v", snippet.ID, output.Snippets)
		}
		seenSnippets[snippet.ID] = struct{}{}
	}
	if len(seenSnippets) == 0 {
		t.Fatal("mixed Inspect batch returned no source snippets")
	}

	evidence, found := state.GetForContext(context.Background(), path)
	if !found {
		t.Fatal("search/read batch did not record Read evidence")
	}
	if !rangeCovered(evidence.Coverage, 4) {
		t.Fatalf("search match line lacks edit evidence: %#v", evidence.Coverage)
	}
	if result.Content == "" || result.Content[0] != '{' {
		t.Fatalf("model content is not compact JSON: %q", result.Content)
	}
}

func TestInspectToolContractIsEagerReadOnlyAndStrict(t *testing.T) {
	tool := New(nil, nil)
	metadata := tool.ToolMetadata(nil)
	if !metadata.ReadOnly || !metadata.Search || !metadata.ConcurrencySafe || metadata.Write || metadata.MaxResultSizeChars != types.UnlimitedToolResultSize {
		t.Fatalf("Inspect metadata = %+v", metadata)
	}
	if !tool.Schema().RejectsUnknownFields() {
		t.Fatal("Inspect schema is not strict")
	}
	discovery := registry.DiscoveryMetadata(tool)
	if !discovery.AlwaysLoad || registry.IsDeferredTool(tool) {
		t.Fatalf("Inspect discovery metadata = %+v", discovery)
	}
}

func TestInspectGlobalFileLimitUsesOneShotOpaqueCursor(t *testing.T) {
	root := t.TempDir()
	for index := 0; index < 6; index++ {
		name := filepath.Join(root, "file-"+integerString(index)+".txt")
		writeInspectFixture(t, name, "value\n")
	}
	tool := New(testRuntimeProvider{runtime: testRuntime(root)}, toolfile.NewReadFileState())
	first, err := tool.Execute(context.Background(), map[string]any{
		"requests": []any{map[string]any{
			"id": "files", "kind": KindGlob, "path": ".", "pattern": "**/*.txt", "max_results": 10,
		}},
		"max_files": 2,
		"max_chars": minimumMaxChars,
	})
	if err != nil || first.IsError {
		t.Fatalf("first page failed: err=%v result=%+v", err, first)
	}
	firstPage := first.Data.(Result)
	if !firstPage.HasMoreView || firstPage.Cursor == "" || len(firstPage.Requests[0].Files) != 2 {
		t.Fatalf("first page = %#v", firstPage)
	}
	if first.Outcome != types.ToolOutcomePartial || first.Completeness.View != types.ToolResultCompletenessPagination {
		t.Fatalf("first page completeness = outcome:%s completeness:%+v", first.Outcome, first.Completeness)
	}
	if len(first.Content) > minimumMaxChars {
		t.Fatalf("first page exceeded max_chars: %d", len(first.Content))
	}

	allFiles := append([]string(nil), firstPage.Requests[0].Files...)
	oldCursor := firstPage.Cursor
	page := firstPage
	for page.Cursor != "" {
		next, nextErr := tool.Execute(context.Background(), map[string]any{"cursor": page.Cursor})
		if nextErr != nil || next.IsError {
			t.Fatalf("cursor page failed: err=%v result=%+v", nextErr, next)
		}
		page = next.Data.(Result)
		if len(page.Requests) != 1 || len(page.Requests[0].Files) == 0 || len(page.Requests[0].Files) > 2 {
			t.Fatalf("cursor page = %#v", page)
		}
		allFiles = append(allFiles, page.Requests[0].Files...)
	}
	sort.Strings(allFiles)
	if len(allFiles) != 6 {
		t.Fatalf("cursor pages returned %d files: %#v", len(allFiles), allFiles)
	}
	for index, path := range allFiles {
		want := "file-" + integerString(index) + ".txt"
		if path != want {
			t.Fatalf("allFiles[%d] = %q, want %q", index, path, want)
		}
	}

	replay, replayErr := tool.Execute(context.Background(), map[string]any{"cursor": oldCursor})
	if replayErr != nil || !replay.IsError {
		t.Fatalf("consumed cursor replay = err:%v result:%+v", replayErr, replay)
	}
}

func TestInspectRejectsRepositoryEscapeAndFiltersReadDeniedSearchPaths(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	writeInspectFixture(t, secret, "outside-secret-value\n")
	if err := os.Symlink(secret, filepath.Join(root, "escape.txt")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	writeInspectFixture(t, filepath.Join(root, "public.txt"), "needle-public\n")
	if err := os.MkdirAll(filepath.Join(root, "secret"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeInspectFixture(t, filepath.Join(root, "secret", "hidden.txt"), "needle-hidden\n")

	runtime := testRuntime(root)
	runtime.DeniedRules = []types.PermissionRuleValue{{ToolName: "Read", RuleContent: "secret/**"}}
	tool := New(testRuntimeProvider{runtime: runtime}, toolfile.NewReadFileState())
	result, err := tool.Execute(context.Background(), map[string]any{
		"requests": []any{
			map[string]any{"id": "escape", "kind": KindRead, "path": "escape.txt"},
			map[string]any{"id": "search", "kind": KindSearch, "path": ".", "pattern": "needle"},
		},
	})
	if err != nil || result.IsError {
		t.Fatalf("composite security result failed wholesale: err=%v result=%+v", err, result)
	}
	output := result.Data.(Result)
	if len(output.Requests) != 2 || len(output.Requests[0].Errors) == 0 {
		t.Fatalf("escape request did not fail in place: %#v", output.Requests)
	}
	if strings.Contains(result.Content, "outside-secret-value") || strings.Contains(result.Content, "needle-hidden") || strings.Contains(result.Content, "secret/hidden") {
		t.Fatalf("Inspect leaked denied content: %s", result.Content)
	}
	if !strings.Contains(result.Content, "public.txt") {
		t.Fatalf("allowed search result missing: %s", result.Content)
	}
}

func TestInspectRequestLimitIsSourcePartialNotToolError(t *testing.T) {
	root := t.TempDir()
	writeInspectFixture(t, filepath.Join(root, "matches.txt"), "needle one\nneedle two\n")
	tool := New(testRuntimeProvider{runtime: testRuntime(root)}, toolfile.NewReadFileState())
	result, err := tool.Execute(context.Background(), map[string]any{
		"requests": []any{map[string]any{
			"id": "limited", "kind": KindSearch, "path": ".", "pattern": "needle", "max_results": 1,
		}},
	})
	if err != nil || result.IsError {
		t.Fatalf("source truncation became an error: err=%v result=%+v", err, result)
	}
	output := result.Data.(Result)
	request := output.Requests[0]
	if !output.SourceTruncated || !request.SourcePartial || request.PartialReason != "request_limit" || len(request.Matches) != 1 {
		t.Fatalf("source partial contract = %#v", output)
	}
	if result.Completeness.Source != types.ToolResultCompletenessSourceTruncated {
		t.Fatalf("source completeness = %+v", result.Completeness)
	}
}

func TestInspectSearchReturnsContextAcrossSnippetChunks(t *testing.T) {
	root := t.TempDir()
	// Each source line remains below Grep's long-line threshold, while the
	// five-line context window still spans multiple 1 KiB Inspect snippets.
	padding := strings.Repeat("x", 430)
	path := filepath.Join(root, "context.txt")
	writeInspectFixture(t, path, strings.Join([]string{
		"before-2 " + padding,
		"before-1 " + padding,
		"needle " + padding,
		"after-1 " + padding,
		"after-2 " + padding,
	}, "\n"))

	tool := New(testRuntimeProvider{runtime: testRuntime(root)}, toolfile.NewReadFileState())
	result, err := tool.Execute(context.Background(), map[string]any{
		"requests": []any{map[string]any{
			"id": "context", "kind": KindSearch, "path": ".", "pattern": "needle", "context": 2,
		}},
	})
	if err != nil || result.IsError {
		t.Fatalf("context search failed: err=%v result=%+v", err, result)
	}
	output := result.Data.(Result)
	if len(output.Requests) != 1 || len(output.Requests[0].Matches) != 1 {
		t.Fatalf("context search result = %#v", output)
	}
	available := make(map[string]struct{}, len(output.Snippets))
	for _, snippet := range output.Snippets {
		available[snippet.ID] = struct{}{}
	}
	for _, snippetID := range output.Requests[0].Matches[0].SnippetIDs {
		if _, ok := available[snippetID]; !ok {
			t.Fatalf("match was separated from context evidence %q", snippetID)
		}
	}
	for line := 1; line <= 5; line++ {
		covered := false
		for _, snippet := range output.Snippets {
			if snippet.Path == "context.txt" && snippet.StartLine <= line && snippet.EndLine >= line {
				covered = true
				break
			}
		}
		if !covered {
			t.Fatalf("requested context line %d missing from snippets: %#v", line, output.Snippets)
		}
	}
}

func TestInspectUnchangedRangeDoesNotReuseWholeSnapshotAsRange(t *testing.T) {
	root := t.TempDir()
	writeInspectFixture(t, filepath.Join(root, "range.txt"), "one\ntwo\nthree\nfour\nfive\n")
	tool := New(testRuntimeProvider{runtime: testRuntime(root)}, toolfile.NewReadFileState())

	full, fullErr := tool.Execute(context.Background(), map[string]any{
		"requests": []any{map[string]any{"id": "full", "kind": KindRead, "path": "range.txt"}},
	})
	if fullErr != nil || full.IsError {
		t.Fatalf("full snapshot read failed: err=%v result=%+v", fullErr, full)
	}
	input := map[string]any{
		"requests": []any{map[string]any{
			"id": "range", "kind": KindRead, "path": "range.txt",
			"ranges": []any{map[string]any{"start": 3, "end": 3}},
		}},
	}
	first, firstErr := tool.Execute(context.Background(), input)
	if firstErr != nil || first.IsError {
		t.Fatalf("first range read failed: err=%v result=%+v", firstErr, first)
	}
	second, secondErr := tool.Execute(context.Background(), input)
	if secondErr != nil || second.IsError {
		t.Fatalf("deduplicated range read failed: err=%v result=%+v", secondErr, second)
	}
	output := second.Data.(Result)
	if len(output.Snippets) != 1 || output.Snippets[0].StartLine != 3 || output.Snippets[0].EndLine != 3 || output.Snippets[0].Content != "three" {
		t.Fatalf("deduplicated range reused mislabeled full snapshot: %#v", output.Snippets)
	}
}

func TestInspectLargeRequestedPageStaysValidAndPreservesCursor(t *testing.T) {
	root := t.TempDir()
	var source strings.Builder
	for line := 1; line <= 500; line++ {
		source.WriteString("stable-source-line-")
		source.WriteString(integerString(line))
		source.WriteString(strings.Repeat("x", 48))
		source.WriteByte('\n')
	}
	writeInspectFixture(t, filepath.Join(root, "large.txt"), source.String())

	tool := New(testRuntimeProvider{runtime: testRuntime(root)}, toolfile.NewReadFileState())
	result, err := tool.Execute(context.Background(), map[string]any{
		"requests": []any{map[string]any{
			"id": "large", "kind": KindRead, "path": "large.txt",
		}},
		"max_chars": 50_000,
	})
	if err != nil || result.IsError {
		t.Fatalf("large Inspect failed: err=%v result=%+v", err, result)
	}
	if len(result.Content) > maximumModelVisibleChars {
		t.Fatalf("model result bytes = %d, want <= %d", len(result.Content), maximumModelVisibleChars)
	}
	var wire modelResult
	if err := json.Unmarshal([]byte(result.Content), &wire); err != nil {
		t.Fatalf("bounded Inspect result is invalid JSON: %v", err)
	}
	page := result.Data.(Result)
	if !wire.HasMoreView || wire.Cursor == "" || wire.Cursor != page.Cursor {
		t.Fatalf("bounded Inspect lost pagination cursor: wire=%+v page=%+v", wire, page)
	}
	mapped := types.MapToolResult(tool, result, "toolu_large")
	if mapped.Content != result.Content || len(mapped.Content) > 15_000 {
		t.Fatalf("mapped result changed the bounded model view: got=%d want=%d", len(mapped.Content), len(result.Content))
	}
}

func TestInspectVisiblePageDoesNotAuthorizeUnseenApplyPatch(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "large.txt")
	var source strings.Builder
	for line := 1; line <= 500; line++ {
		source.WriteString("original-line-")
		source.WriteString(integerString(line))
		source.WriteString(strings.Repeat("x", 48))
		source.WriteByte('\n')
	}
	writeInspectFixture(t, path, source.String())
	readState := toolfile.NewReadFileState()
	inspectTool := New(testRuntimeProvider{runtime: testRuntime(root)}, readState)
	first, err := inspectTool.Execute(context.Background(), map[string]any{
		"requests":  []any{map[string]any{"id": "large", "kind": KindRead, "path": "large.txt"}},
		"max_chars": maximumModelVisibleChars,
	})
	if err != nil || first.IsError {
		t.Fatalf("first Inspect page: result=%+v err=%v", first, err)
	}
	firstPage := first.Data.(Result)
	if !firstPage.HasMoreView || firstPage.Cursor == "" {
		t.Fatalf("large read did not paginate: %#v", firstPage)
	}
	entry, found := readState.GetForContext(context.Background(), path)
	if !found || entry.FullSnapshot || entry.CoverageComplete || rangeCovered(entry.Coverage, 450) {
		t.Fatalf("first page over-authorized private source: found=%t entry=%+v", found, entry)
	}

	patchTool := &toolfile.ApplyPatchTool{
		AllowedDirs: []string{root}, Runtime: testRuntimeProvider{runtime: testRuntime(root)}, ReadState: readState,
	}
	patch := strings.Join([]string{
		"--- a/large.txt", "+++ b/large.txt", "@@ -450,1 +450,1 @@",
		"-original-line-450" + strings.Repeat("x", 48),
		"+changed-line-450" + strings.Repeat("x", 49),
	}, "\n")
	blocked, blockedErr := patchTool.Execute(context.Background(), map[string]any{"patch": patch})
	if blockedErr != nil {
		t.Fatal(blockedErr)
	}
	data, ok := blocked.Data.(types.ToolErrorData)
	if !blocked.IsError || !ok || data.Code != "file.apply_patch.read_required" {
		t.Fatalf("unseen line mutation was not rejected: %+v", blocked)
	}

	page := firstPage
	for page.Cursor != "" {
		next, nextErr := inspectTool.Execute(context.Background(), map[string]any{"cursor": page.Cursor})
		if nextErr != nil || next.IsError {
			t.Fatalf("Inspect continuation: result=%+v err=%v", next, nextErr)
		}
		page = next.Data.(Result)
	}
	entry, found = readState.GetForContext(context.Background(), path)
	if !found || !entry.FullSnapshot || !entry.CoverageComplete || !rangeCovered(entry.Coverage, 450) {
		t.Fatalf("all visible pages did not establish a full receipt: found=%t entry=%+v", found, entry)
	}
	applied, applyErr := patchTool.Execute(context.Background(), map[string]any{"patch": patch})
	if applyErr != nil || applied.IsError {
		t.Fatalf("fully visible mutation: result=%+v err=%v", applied, applyErr)
	}
}

func TestInspectCursorRejectsWorkspaceRevisionChangeAndReemits(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "revision.txt")
	writeInspectFixture(t, path, strings.Repeat("before revision\n", 900))
	ledger := workspacerevision.NewLedger()
	tool := New(testRuntimeProvider{runtime: testRuntime(root)}, toolfile.NewReadFileState())
	tool.WorkspaceRevisions = ledger
	input := map[string]any{
		"requests":  []any{map[string]any{"id": "revision", "kind": KindRead, "path": "revision.txt"}},
		"max_chars": minimumMaxChars,
	}
	first, err := tool.Execute(context.Background(), input)
	if err != nil || first.IsError {
		t.Fatalf("first page: result=%+v err=%v", first, err)
	}
	page := first.Data.(Result)
	if page.Cursor == "" {
		t.Fatal("fixture did not paginate")
	}
	writeInspectFixture(t, path, strings.Repeat("after revision\n", 900))
	if _, commitErr := ledger.Commit(root, []string{path}); commitErr != nil {
		t.Fatal(commitErr)
	}
	if epoch, ok := ledger.CurrentEpoch(root); !ok || epoch != 1 {
		t.Fatalf("workspace revision after commit = %d, ok=%t", epoch, ok)
	}
	workspace, workspaceErr := tool.workspaceSnapshot(context.Background(), types.ToolRuntimeContext{})
	if workspaceErr != nil || !workspace.workspaceRevisionBound || workspace.workspaceRevision != 1 {
		t.Fatalf("Inspect revision snapshot = %+v, err=%v", workspace, workspaceErr)
	}
	stale, staleErr := tool.Execute(context.Background(), map[string]any{"cursor": page.Cursor})
	if staleErr != nil || !stale.IsError {
		t.Fatalf("stale workspace cursor was accepted: result=%+v err=%v", stale, staleErr)
	}
	fresh, freshErr := tool.Execute(context.Background(), input)
	if freshErr != nil || fresh.IsError {
		t.Fatalf("fresh revision read: result=%+v err=%v", fresh, freshErr)
	}
	stats := fresh.Data.(Result).modelStats
	if stats.NewChars == 0 || stats.ReusedChars != 0 {
		t.Fatalf("new revision reused stale evidence: %+v", stats)
	}
}

func TestInspectFirstPageHasEveryRequestEnvelopeAndPrioritizesExactRead(t *testing.T) {
	root := t.TempDir()
	for index := 0; index < 15; index++ {
		writeInspectFixture(t, filepath.Join(root, "a-file-"+integerString(index)+".txt"), "glob\n")
	}
	writeInspectFixture(t, filepath.Join(root, "z-priority.txt"), "priority evidence\n")
	requests := make([]any, 0, maximumRequests)
	for index := 0; index < maximumRequests-1; index++ {
		requests = append(requests, map[string]any{
			"id": "glob-" + integerString(index), "kind": KindGlob, "path": ".", "pattern": "*.txt",
		})
	}
	requests = append(requests, map[string]any{
		"id": "priority-read", "kind": KindRead, "path": "z-priority.txt",
	})
	tool := New(testRuntimeProvider{runtime: testRuntime(root)}, toolfile.NewReadFileState())
	result, err := tool.Execute(context.Background(), map[string]any{
		"requests": requests, "max_files": 1, "max_chars": minimumMaxChars,
	})
	if err != nil || result.IsError {
		t.Fatalf("fair page: result=%+v err=%v", result, err)
	}
	page := result.Data.(Result)
	wire := decodeInspectModelResult(t, result.Content)
	if len(page.Requests) != maximumRequests || len(wire.Requests) != maximumRequests {
		t.Fatalf("request envelopes: typed=%d wire=%d", len(page.Requests), len(wire.Requests))
	}
	seen := make(map[string]struct{}, maximumRequests)
	for _, request := range wire.Requests {
		seen[request.ID] = struct{}{}
	}
	if _, ok := seen["priority-read"]; !ok || len(seen) != maximumRequests {
		t.Fatalf("missing first-page request envelope: %#v", seen)
	}
	if len(page.OmittedRequestIDs) != maximumRequests-1 || !page.HasMoreView {
		t.Fatalf("deferred payload routing = %#v", page.OmittedRequestIDs)
	}
	if len(wire.Evidence) == 0 || wire.Evidence[0].Path != "z-priority.txt" {
		t.Fatalf("exact read did not receive first payload priority: %#v", wire.Evidence)
	}
}

func TestInspectModelEvidenceDeduplicatesOverlappingLinesAcrossCalls(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "evidence.txt")
	writeInspectFixture(t, path, "one\ntwo\nthree\nfour\n")
	tool := New(testRuntimeProvider{runtime: testRuntime(root)}, toolfile.NewReadFileState())

	first, firstErr := tool.Execute(context.Background(), map[string]any{
		"requests": []any{map[string]any{
			"id": "first", "kind": KindRead, "path": "evidence.txt",
			"ranges": []any{map[string]any{"start": 1, "end": 4}},
		}},
	})
	if firstErr != nil || first.IsError {
		t.Fatalf("first evidence read failed: err=%v result=%+v", firstErr, first)
	}
	decodeInspectModelResult(t, first.Content)
	firstStats := first.Data.(Result).modelStats
	if firstStats.NewChars == 0 || firstStats.ReusedChars != 0 {
		t.Fatalf("cold evidence stats = %+v", firstStats)
	}

	second, secondErr := tool.Execute(context.Background(), map[string]any{
		"requests": []any{map[string]any{
			"id": "overlap", "kind": KindRead, "path": "evidence.txt",
			"ranges": []any{map[string]any{"start": 2, "end": 3}},
		}},
	})
	if secondErr != nil || second.IsError {
		t.Fatalf("overlapping evidence read failed: err=%v result=%+v", secondErr, second)
	}
	secondWire := decodeInspectModelResult(t, second.Content)
	secondStats := second.Data.(Result).modelStats
	if secondStats.NewChars != 0 || secondStats.ReusedChars != len("two")+len("three") {
		t.Fatalf("warm evidence stats = %+v", secondStats)
	}
	if len(secondWire.Evidence) != 1 || len(secondWire.Evidence[0].Chunks) != 1 ||
		secondWire.Evidence[0].Chunks[0].Seen == "" || secondWire.Evidence[0].Chunks[0].Content != nil {
		t.Fatalf("overlap was not projected as one path-scoped reference: %#v", secondWire.Evidence)
	}
	if len(second.Content) >= len(first.Content) {
		t.Fatalf("warm evidence did not shrink: first=%d second=%d", len(first.Content), len(second.Content))
	}

	writeInspectFixture(t, path, "one\ntwo changed\nthree\nfour\n")
	third, thirdErr := tool.Execute(context.Background(), map[string]any{
		"requests": []any{map[string]any{
			"id": "changed", "kind": KindRead, "path": "evidence.txt",
			"ranges": []any{map[string]any{"start": 2, "end": 3}},
		}},
	})
	if thirdErr != nil || third.IsError {
		t.Fatalf("changed evidence read failed: err=%v result=%+v", thirdErr, third)
	}
	thirdWire := decodeInspectModelResult(t, third.Content)
	thirdStats := third.Data.(Result).modelStats
	if thirdStats.NewChars != len("two changed") || thirdStats.ReusedChars != len("three") {
		t.Fatalf("changed evidence stats = %+v", thirdStats)
	}
	var hasChangedContent, hasSeen bool
	for _, file := range thirdWire.Evidence {
		for _, chunk := range file.Chunks {
			hasChangedContent = hasChangedContent || chunk.Content != nil
			hasSeen = hasSeen || chunk.Seen != ""
		}
	}
	if !hasChangedContent || !hasSeen {
		t.Fatalf("changed and unchanged lines were not separated: %#v", thirdWire.Evidence)
	}
}

func TestInspectEvidenceLedgerIsSessionScoped(t *testing.T) {
	root := t.TempDir()
	writeInspectFixture(t, filepath.Join(root, "scope.txt"), "session evidence\n")
	parentRuntime := testRuntime(root)
	parent := New(testRuntimeProvider{runtime: parentRuntime}, toolfile.NewReadFileState())
	input := map[string]any{"requests": []any{map[string]any{
		"id": "scope", "kind": KindRead, "path": "scope.txt",
	}}}
	first, firstErr := parent.Execute(context.Background(), input)
	if firstErr != nil || first.IsError {
		t.Fatalf("parent read failed: err=%v result=%+v", firstErr, first)
	}
	if got := first.Data.(Result).modelStats.NewChars; got == 0 {
		t.Fatal("parent did not expose cold evidence")
	}

	childRuntime := parentRuntime
	childRuntime.SessionID = "inspect-other-session"
	child := parent.WithRuntime(testRuntimeProvider{runtime: childRuntime}).(*Tool)
	childResult, childErr := child.Execute(context.Background(), input)
	if childErr != nil || childResult.IsError {
		t.Fatalf("child read failed: err=%v result=%+v", childErr, childResult)
	}
	decodeInspectModelResult(t, childResult.Content)
	childStats := childResult.Data.(Result).modelStats
	if childStats.NewChars == 0 || childStats.ReusedChars != 0 {
		t.Fatalf("evidence crossed session boundary: %+v", childStats)
	}
}

func TestInspectEvidenceLedgerResetsAcrossRunAndCacheLineage(t *testing.T) {
	store := newEvidenceStore()
	key := "visible-line"
	base := workspaceSnapshot{
		root: "/repo", sessionID: "session", runID: "run-a",
		cacheLineageID: "cache-a", historyEpoch: "history-a", workspaceRevision: 7,
	}
	namespace := evidenceNamespaceKey(base)
	store.observe(namespace, []evidenceObservation{{key: key}})
	assertSeen := func(workspace workspaceSnapshot, want bool) {
		t.Helper()
		var got bool
		if err := store.withNamespace(evidenceNamespaceKey(workspace), func(view *evidenceView) error {
			got = view.contains(key)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("seen=%t, want %t for %+v", got, want, workspace)
		}
	}
	assertSeen(base, true)
	changed := base
	changed.runID = "run-b"
	assertSeen(changed, false)
	changed = base
	changed.cacheLineageID = "cache-b"
	assertSeen(changed, false)
	changed = base
	changed.historyEpoch = "history-b"
	assertSeen(changed, false)
	changed = base
	changed.workspaceRevision++
	assertSeen(changed, false)
}

func TestInspectConcurrentDuplicateEvidenceEmitsOneContentCopy(t *testing.T) {
	root := t.TempDir()
	writeInspectFixture(t, filepath.Join(root, "concurrent.txt"), "shared one\nshared two\n")
	tool := New(testRuntimeProvider{runtime: testRuntime(root)}, toolfile.NewReadFileState())
	input := map[string]any{"requests": []any{map[string]any{
		"id": "same", "kind": KindRead, "path": "concurrent.txt",
	}}}

	type executionResult struct {
		result types.ToolResult
		err    error
	}
	results := make(chan executionResult, 2)
	for call := 0; call < 2; call++ {
		go func() {
			result, err := tool.Execute(context.Background(), input)
			results <- executionResult{result: result, err: err}
		}()
	}
	var newCalls, reusedCalls int
	for call := 0; call < 2; call++ {
		execution := <-results
		if execution.err != nil || execution.result.IsError {
			t.Fatalf("concurrent Inspect failed: err=%v result=%+v", execution.err, execution.result)
		}
		stats := execution.result.Data.(Result).modelStats
		if stats.NewChars > 0 {
			newCalls++
		}
		if stats.ReusedChars > 0 {
			reusedCalls++
		}
	}
	if newCalls != 1 || reusedCalls != 1 {
		t.Fatalf("concurrent evidence copies: new=%d reused=%d", newCalls, reusedCalls)
	}
}

func TestInspectPinsOneRuntimeSnapshotAndWithRuntimeCannotConsumeParentCursor(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	writeInspectFixture(t, filepath.Join(rootA, "a-1.txt"), "alpha\n")
	writeInspectFixture(t, filepath.Join(rootA, "a-2.txt"), "alpha\n")
	writeInspectFixture(t, filepath.Join(rootB, "b.txt"), "beta\n")

	provider := &switchingRuntimeProvider{values: []types.ToolRuntimeContext{testRuntime(rootA), testRuntime(rootB)}}
	snapshotTool := New(provider, toolfile.NewReadFileState())
	snapshotResult, err := snapshotTool.Execute(context.Background(), map[string]any{
		"requests": []any{map[string]any{
			"id": "parent", "kind": KindGlob, "path": ".", "pattern": "**/*.txt", "max_results": 10,
		}},
	})
	if err != nil || snapshotResult.IsError {
		t.Fatalf("snapshot Inspect failed: err=%v result=%+v", err, snapshotResult)
	}
	snapshotPage := snapshotResult.Data.(Result)
	if len(snapshotPage.Requests) != 1 || len(snapshotPage.Requests[0].Files) != 2 {
		t.Fatalf("snapshot batch mixed workspaces: %#v", snapshotPage)
	}
	for _, path := range snapshotPage.Requests[0].Files {
		if !strings.HasPrefix(path, "a-") {
			t.Fatalf("snapshot batch leaked another runtime: %#v", snapshotPage.Requests[0].Files)
		}
	}
	provider.mu.Lock()
	samples := provider.samples
	provider.mu.Unlock()
	if samples != 1 {
		t.Fatalf("one Inspect batch sampled runtime %d times, want 1", samples)
	}

	parent := New(testRuntimeProvider{runtime: testRuntime(rootA)}, toolfile.NewReadFileState())
	first, err := parent.Execute(context.Background(), map[string]any{
		"requests": []any{map[string]any{
			"id": "parent", "kind": KindGlob, "path": ".", "pattern": "**/*.txt", "max_results": 10,
		}},
		"max_files": 1,
	})
	if err != nil || first.IsError {
		t.Fatalf("parent Inspect failed: err=%v result=%+v", err, first)
	}
	page := first.Data.(Result)
	if page.Cursor == "" || len(page.Requests) != 1 || len(page.Requests[0].Files) != 1 || !strings.HasPrefix(page.Requests[0].Files[0], "a-") {
		t.Fatalf("parent cursor page = %#v", page)
	}

	childRuntime := testRuntime(rootB)
	childRuntime.SessionID = "inspect-child"
	child := parent.WithRuntime(testRuntimeProvider{runtime: childRuntime}).(*Tool)
	foreign, foreignErr := child.Execute(context.Background(), map[string]any{"cursor": page.Cursor})
	if foreignErr != nil || !foreign.IsError {
		t.Fatalf("child consumed parent cursor: err=%v result=%+v", foreignErr, foreign)
	}

	continuation, continuationErr := parent.Execute(context.Background(), map[string]any{"cursor": page.Cursor})
	if continuationErr != nil || continuation.IsError {
		t.Fatalf("foreign cursor attempt destroyed parent cursor: err=%v result=%+v", continuationErr, continuation)
	}
	continuedPage := continuation.Data.(Result)
	if len(continuedPage.Requests) != 1 || len(continuedPage.Requests[0].Files) != 1 || !strings.HasPrefix(continuedPage.Requests[0].Files[0], "a-") {
		t.Fatalf("parent continuation crossed workspace: %#v", continuedPage)
	}

	childResult, childErr := child.Execute(context.Background(), map[string]any{
		"requests": []any{map[string]any{
			"id": "child", "kind": KindGlob, "path": ".", "pattern": "**/*.txt",
		}},
	})
	if childErr != nil || childResult.IsError {
		t.Fatalf("child Inspect failed: err=%v result=%+v", childErr, childResult)
	}
	childPage := childResult.Data.(Result)
	if got := childPage.Requests[0].Files; len(got) != 1 || got[0] != "b.txt" {
		t.Fatalf("child workspace result = %#v", got)
	}
}

func TestInspectActorCloneUsesChildReadStateAndIsolatesCursor(t *testing.T) {
	root := t.TempDir()
	parentPath := filepath.Join(root, "parent.txt")
	childPath := filepath.Join(root, "child.txt")
	writeInspectFixture(t, parentPath, "parent evidence\n")
	writeInspectFixture(t, childPath, "child evidence\n")

	runtime := testRuntimeProvider{runtime: testRuntime(root)}
	parentState := toolfile.NewReadFileState()
	childState := toolfile.NewReadFileState()
	parent := New(runtime, parentState)
	first, err := parent.Execute(context.Background(), map[string]any{
		"requests": []any{map[string]any{
			"id": "files", "kind": KindGlob, "path": ".", "pattern": "**/*.txt", "max_results": 10,
		}},
		"max_files": 1,
	})
	if err != nil || first.IsError {
		t.Fatalf("parent cursor setup failed: err=%v result=%+v", err, first)
	}
	parentPage := first.Data.(Result)
	if parentPage.Cursor == "" {
		t.Fatalf("parent result did not paginate: %#v", parentPage)
	}

	child := parent.WithRuntimeAndReadState(runtime, childState).(*Tool)
	foreign, foreignErr := child.Execute(context.Background(), map[string]any{"cursor": parentPage.Cursor})
	if foreignErr != nil || !foreign.IsError {
		t.Fatalf("child consumed parent actor cursor: err=%v result=%+v", foreignErr, foreign)
	}
	continued, continueErr := parent.Execute(context.Background(), map[string]any{"cursor": parentPage.Cursor})
	if continueErr != nil || continued.IsError {
		t.Fatalf("child cursor attempt affected parent: err=%v result=%+v", continueErr, continued)
	}

	read, readErr := child.Execute(context.Background(), map[string]any{
		"requests": []any{map[string]any{
			"id": "child-read", "kind": KindRead, "path": "child.txt",
		}},
	})
	if readErr != nil || read.IsError {
		t.Fatalf("child Inspect read failed: err=%v result=%+v", readErr, read)
	}
	if _, found := childState.GetForContext(context.Background(), childPath); !found {
		t.Fatal("child Inspect did not record evidence in child ledger")
	}
	if _, found := parentState.GetForContext(context.Background(), childPath); found {
		t.Fatal("child Inspect leaked read evidence into parent ledger")
	}
	if _, found := childState.GetForContext(context.Background(), parentPath); found {
		t.Fatal("child ledger unexpectedly inherited unrelated parent evidence")
	}
}

func testRuntime(root string) types.ToolRuntimeContext {
	return types.ToolRuntimeContext{SessionID: "inspect-test", ProjectRoot: root, AllowedDirs: []string{root}}
}

func writeInspectFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func decodeInspectModelResult(t *testing.T, content string) modelResult {
	t.Helper()
	var result modelResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		t.Fatalf("decode compact Inspect result: %v", err)
	}
	return result
}

func rangeCovered(ranges []toolfile.ReadLineRange, line int) bool {
	for _, lineRange := range ranges {
		if lineRange.StartLine <= line && line < lineRange.EndLine {
			return true
		}
	}
	return false
}
