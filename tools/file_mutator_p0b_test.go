package tools

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/types"
)

func p0bWriteFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func p0bReadEvidence(t *testing.T, dir, path string, state *ReadFileState) {
	t.Helper()
	result, err := (&FileReadTool{AllowedDirs: []string{dir}, ReadState: state}).Execute(
		context.Background(), map[string]any{"file_path": path},
	)
	if err != nil || result.IsError {
		t.Fatalf("production Read evidence failed: result=%+v err=%v", result, err)
	}
}

func recordStrongReadEvidenceForTest(t *testing.T, state *ReadFileState, path string) {
	t.Helper()
	p0bReadEvidence(t, filepath.Dir(path), path, state)
}

func p0bWaitResult(t *testing.T, result <-chan types.ToolResult) types.ToolResult {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent file transaction did not complete")
		return types.ToolResult{}
	}
}

func p0bStartEditAtPrecommitBarrier(
	t *testing.T,
	dir, path, oldString, newString string,
	state *ReadFileState,
) (<-chan struct{}, chan<- struct{}, <-chan types.ToolResult) {
	t.Helper()
	verified := make(chan struct{})
	release := make(chan struct{})
	var signal sync.Once
	tool := &FileEditTool{
		AllowedDirs: []string{dir},
		ReadState:   state,
		afterPrecommitVerifyForTest: func() {
			signal.Do(func() { close(verified) })
			<-release
		},
	}
	result := make(chan types.ToolResult, 1)
	go func() {
		value, err := tool.Execute(context.Background(), map[string]any{
			"file_path": path, "old_string": oldString, "new_string": newString,
		})
		if err != nil {
			value = ErrorResponse(err)
		}
		result <- value
	}()
	select {
	case <-verified:
	case <-time.After(5 * time.Second):
		t.Fatal("Edit did not reach its final verify/commit barrier")
	}
	return verified, release, result
}

func TestP0BMutatorsSerializeWithEditFinalVerifyCommit(t *testing.T) {
	t.Run("append", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "a-target.txt")
		p0bWriteFixture(t, path, "alpha\n")
		state := NewReadFileState()
		p0bReadEvidence(t, dir, path, state)
		_, release, editResult := p0bStartEditAtPrecommitBarrier(t, dir, path, "alpha", "beta", state)

		registered := make(chan struct{})
		var signal sync.Once
		appendTool := &FileAppendTool{
			AllowedDirs:                   []string{dir},
			mutationLockRegisteredForTest: func() { signal.Do(func() { close(registered) }) },
		}
		mutationResult := make(chan types.ToolResult, 1)
		go func() {
			value, err := appendTool.Execute(context.Background(), map[string]any{"file_path": path, "content": "tail\n"})
			if err != nil {
				value = ErrorResponse(err)
			}
			mutationResult <- value
		}()
		<-registered
		close(release)
		if result := p0bWaitResult(t, editResult); result.IsError {
			t.Fatalf("Edit failed: %+v", result)
		}
		if result := p0bWaitResult(t, mutationResult); result.IsError {
			t.Fatalf("append failed: %+v", result)
		}
		raw, err := os.ReadFile(path)
		if err != nil || string(raw) != "beta\ntail\n" {
			t.Fatalf("append was lost across Edit commit: content=%q err=%v", raw, err)
		}
	})

	t.Run("delete", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "a-target.txt")
		p0bWriteFixture(t, path, "alpha\n")
		state := NewReadFileState()
		p0bReadEvidence(t, dir, path, state)
		_, release, editResult := p0bStartEditAtPrecommitBarrier(t, dir, path, "alpha", "beta", state)

		registered := make(chan struct{})
		var signal sync.Once
		deleteTool := &FileDeleteTool{
			AllowedDirs:                   []string{dir},
			mutationLockRegisteredForTest: func() { signal.Do(func() { close(registered) }) },
		}
		mutationResult := make(chan types.ToolResult, 1)
		go func() {
			value, err := deleteTool.Execute(context.Background(), map[string]any{"file_path": path})
			if err != nil {
				value = ErrorResponse(err)
			}
			mutationResult <- value
		}()
		<-registered
		close(release)
		if result := p0bWaitResult(t, editResult); result.IsError {
			t.Fatalf("Edit failed: %+v", result)
		}
		if result := p0bWaitResult(t, mutationResult); result.IsError {
			t.Fatalf("delete failed: %+v", result)
		}
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("Edit resurrected a serialized delete: err=%v", err)
		}
	})

	t.Run("move-source", func(t *testing.T) {
		dir := t.TempDir()
		source := filepath.Join(dir, "a-source.txt")
		dest := filepath.Join(dir, "z-destination.txt")
		p0bWriteFixture(t, source, "alpha\n")
		state := NewReadFileState()
		p0bReadEvidence(t, dir, source, state)
		_, release, editResult := p0bStartEditAtPrecommitBarrier(t, dir, source, "alpha", "beta", state)

		registered := make(chan struct{})
		var signal sync.Once
		moveTool := &FileMoveTool{
			AllowedDirs:                   []string{dir},
			mutationLockRegisteredForTest: func() { signal.Do(func() { close(registered) }) },
		}
		mutationResult := make(chan types.ToolResult, 1)
		go func() {
			value, err := moveTool.Execute(context.Background(), map[string]any{"source": source, "destination": dest})
			if err != nil {
				value = ErrorResponse(err)
			}
			mutationResult <- value
		}()
		<-registered // source is the first deterministic lock key
		close(release)
		if result := p0bWaitResult(t, editResult); result.IsError {
			t.Fatalf("Edit failed: %+v", result)
		}
		if result := p0bWaitResult(t, mutationResult); result.IsError {
			t.Fatalf("move failed: %+v", result)
		}
		raw, err := os.ReadFile(dest)
		if err != nil || string(raw) != "beta\n" {
			t.Fatalf("move lost the serialized Edit post-image: content=%q err=%v", raw, err)
		}
		if _, err := os.Lstat(source); !os.IsNotExist(err) {
			t.Fatalf("move source remained after commit: err=%v", err)
		}
	})

	t.Run("move-destination", func(t *testing.T) {
		dir := t.TempDir()
		dest := filepath.Join(dir, "a-destination.txt")
		source := filepath.Join(dir, "z-source.txt")
		p0bWriteFixture(t, dest, "alpha\n")
		p0bWriteFixture(t, source, "source\n")
		state := NewReadFileState()
		p0bReadEvidence(t, dir, dest, state)
		_, release, editResult := p0bStartEditAtPrecommitBarrier(t, dir, dest, "alpha", "beta", state)

		registered := make(chan struct{})
		var signal sync.Once
		moveTool := &FileMoveTool{
			AllowedDirs:                   []string{dir},
			mutationLockRegisteredForTest: func() { signal.Do(func() { close(registered) }) },
		}
		mutationResult := make(chan types.ToolResult, 1)
		go func() {
			value, err := moveTool.Execute(context.Background(), map[string]any{"source": source, "destination": dest})
			if err != nil {
				value = ErrorResponse(err)
			}
			mutationResult <- value
		}()
		<-registered // destination is the first deterministic lock key
		close(release)
		if result := p0bWaitResult(t, editResult); result.IsError {
			t.Fatalf("Edit failed: %+v", result)
		}
		if result := p0bWaitResult(t, mutationResult); result.IsError {
			t.Fatalf("move failed: %+v", result)
		}
		raw, err := os.ReadFile(dest)
		if err != nil || string(raw) != "source\n" {
			t.Fatalf("destination lock did not serialize later move: content=%q err=%v", raw, err)
		}
	})
}

func TestP0BInverseMovesUseDeterministicLockOrder(t *testing.T) {
	dir := t.TempDir()
	left := filepath.Join(dir, "left.txt")
	right := filepath.Join(dir, "right.txt")
	p0bWriteFixture(t, left, "left\n")
	p0bWriteFixture(t, right, "right\n")
	start := make(chan struct{})
	results := make(chan types.ToolResult, 2)
	run := func(source, destination string) {
		<-start
		value, err := (&FileMoveTool{AllowedDirs: []string{dir}}).Execute(
			context.Background(), map[string]any{"source": source, "destination": destination},
		)
		if err != nil {
			value = ErrorResponse(err)
		}
		results <- value
	}
	go run(left, right)
	go run(right, left)
	close(start)
	_ = p0bWaitResult(t, results)
	_ = p0bWaitResult(t, results)
}

func TestP0BSedSerializesWithEditAndPublishesStrongEvidence(t *testing.T) {
	requireBashAvailable(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "a-edit.txt")
	p0bWriteFixture(t, path, "alpha\n")
	state := NewReadFileState()
	p0bReadEvidence(t, dir, path, state)
	_, release, editResult := p0bStartEditAtPrecommitBarrier(t, dir, path, "alpha", "beta", state)

	registered := make(chan struct{})
	var signal sync.Once
	bash := &BashTool{
		CWD: dir, AllowedDirs: []string{dir}, ReadFileState: state, SedValidationEnabled: true,
		sedLockRegisteredForTest: func() { signal.Do(func() { close(registered) }) },
	}
	command := `sed -i 's/beta/gamma/' a-edit.txt`
	if runtime.GOOS == "darwin" {
		command = `sed -i '' 's/beta/gamma/' a-edit.txt`
	}
	bashResult := make(chan types.ToolResult, 1)
	bashCtx, cancelBash := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelBash()
	go func() {
		value, err := bash.Execute(bashCtx, map[string]any{"command": command})
		if err != nil {
			value = ErrorResponse(err)
		}
		bashResult <- value
	}()
	releasedEdit := false
	defer func() {
		if !releasedEdit {
			close(release)
		}
	}()
	select {
	case <-registered:
	case result := <-bashResult:
		t.Fatalf("Bash returned before registering the sed mutation lock: %+v", result)
	case <-bashCtx.Done():
		t.Fatalf("Bash did not reach the sed mutation lock barrier: %v", bashCtx.Err())
	}
	close(release)
	releasedEdit = true
	if result := p0bWaitResult(t, editResult); result.IsError {
		t.Fatalf("Edit failed: %+v", result)
	}
	if result := p0bWaitResult(t, bashResult); result.IsError {
		t.Fatalf("serialized sed failed: %+v", result)
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != "gamma\n" {
		t.Fatalf("sed/Edit ordering lost an update: content=%q err=%v", raw, err)
	}
	entry, ok := state.Get(path)
	if !ok || entry.LastTool != "Bash" || entry.Content != "gamma\n" || entry.ContentDigest != fileContentDigest(raw) || entry.FileIdentity == nil {
		t.Fatalf("post-sed evidence is not descriptor-bound: %+v", entry)
	}
}

func TestP0BSameStatRollbackAndWeakEvidenceCannotAuthorizeEdit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollback.txt")
	p0bWriteFixture(t, path, "alpha\n")
	state := NewReadFileState()
	p0bReadEvidence(t, dir, path, state)
	entry, ok := state.Get(path)
	if !ok || entry.ContentDigest == "" || entry.FileIdentity == nil {
		t.Fatalf("Read did not publish strong evidence: %+v", entry)
	}
	originalInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	p0bWriteFixture(t, path, "omega\n")
	if err := os.Chtimes(path, originalInfo.ModTime(), originalInfo.ModTime()); err != nil {
		t.Fatal(err)
	}
	rolledBack, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if rolledBack.Size() != originalInfo.Size() || !rolledBack.ModTime().Equal(originalInfo.ModTime()) {
		t.Skip("filesystem cannot construct same-size/same-mtime rollback fixture")
	}
	edit := &FileEditTool{AllowedDirs: []string{dir}, ReadState: state}
	result, err := edit.Execute(context.Background(), map[string]any{
		"file_path": path, "old_string": "omega", "new_string": "forged",
	})
	data, dataOK := result.Data.(types.ToolErrorData)
	if err != nil || !result.IsError || !dataOK || data.Code != fileErrorSnapshotStale {
		t.Fatalf("same-stat rollback authorized Edit: result=%+v data=%#v err=%v", result, result.Data, err)
	}

	// Even a matching timestamp/content FullSnapshot is not authorization when
	// it did not originate from a descriptor-bound digest observation.
	state.Set(path, ReadFileEntry{
		TimestampMs: rolledBack.ModTime().UnixMilli(), MtimeNs: rolledBack.ModTime().UnixNano(),
		TotalBytes: rolledBack.Size(), TotalLines: 1,
		CoverageKnown: true, Coverage: []ReadLineRange{{StartLine: 1, EndLine: 2}},
		CoverageComplete: true, FullSnapshot: true, Content: "omega\n", LastTool: "Read",
	})
	result, err = edit.Execute(context.Background(), map[string]any{
		"file_path": path, "old_string": "omega", "new_string": "forged",
	})
	data, dataOK = result.Data.(types.ToolErrorData)
	if err != nil || !result.IsError || !dataOK || data.Code != fileErrorSnapshotStale {
		t.Fatalf("digest/identity-free evidence authorized Edit: result=%+v data=%#v err=%v", result, result.Data, err)
	}
	if raw, readErr := os.ReadFile(path); readErr != nil || string(raw) != "omega\n" {
		t.Fatalf("rejected weak-evidence Edit changed file: content=%q err=%v", raw, readErr)
	}
}

func TestP0BMarkSedEvidenceFailsClosedAndUsesOneDescriptorSnapshot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mark.txt")
	p0bWriteFixture(t, path, "after\n")
	state := NewReadFileState()
	command := `sed -i 's/before/after/' mark.txt`
	MarkSedEditReadStateForContext(context.Background(), command, dir, state)
	entry, ok := state.Get(path)
	if !ok || !entry.FullSnapshot || entry.Content != "after\n" || entry.ContentDigest != fileContentDigest([]byte("after\n")) || entry.FileIdentity == nil {
		t.Fatalf("MarkSed published weak or inconsistent evidence: %+v", entry)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	MarkSedEditReadStateForContext(context.Background(), command, dir, state)
	if _, ok := state.Get(path); ok {
		t.Fatal("failed post-sed snapshot left stale authorization evidence")
	}
}

func TestP0BCompoundSedValidatesEveryMutationTarget(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.txt")
	second := filepath.Join(dir, "second.txt")
	p0bWriteFixture(t, first, "one\n")
	p0bWriteFixture(t, second, "two\n")
	state := NewReadFileState()
	p0bReadEvidence(t, dir, first, state)
	command := `sed -i 's/one/ONE/' first.txt; sed -i.bak 's/two/TWO/' second.txt`
	if plans := parseSedEditPlans(command); len(plans) != 2 {
		t.Fatalf("compound sed parser found %d mutation plans, want 2", len(plans))
	}
	if err := ValidateSedEditReadState(command, dir, state); err == nil {
		t.Fatal("compound sed validation ignored the unread second mutation target")
	}
	p0bReadEvidence(t, dir, second, state)
	if err := ValidateSedEditReadState(command, dir, state); err != nil {
		t.Fatalf("compound sed rejected after every target had strong evidence: %v", err)
	}
	targets := sedEditMutationTargets(command, dir)
	want := map[string]bool{
		canonicalFileEditLockPath(first):           false,
		canonicalFileEditLockPath(second):          false,
		canonicalFileEditLockPath(second + ".bak"): false,
	}
	for _, target := range targets {
		if _, exists := want[target]; exists {
			want[target] = true
		}
	}
	for target, found := range want {
		if !found {
			t.Fatalf("compound sed lock set omitted %s: %v", target, targets)
		}
	}
}

func TestP0BReadDedupRejectsPathSwapAfterDescriptorOpen(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not permit replacing this open-file fixture")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "dedup.txt")
	replacement := filepath.Join(dir, "replacement.txt")
	p0bWriteFixture(t, path, "old\n")
	p0bWriteFixture(t, replacement, "new\n")
	state := NewReadFileState()
	read := &FileReadTool{AllowedDirs: []string{dir}, ReadState: state}
	first, err := read.Execute(context.Background(), map[string]any{"file_path": path})
	if err != nil || first.IsError {
		t.Fatalf("initial Read failed: result=%+v err=%v", first, err)
	}
	var swapErr error
	read.digestAfterOpenForTest = func() {
		if err := os.Remove(path); err != nil {
			swapErr = err
			return
		}
		swapErr = os.Rename(replacement, path)
	}
	second, err := read.Execute(context.Background(), map[string]any{"file_path": path})
	if swapErr != nil {
		t.Fatal(swapErr)
	}
	output, outputOK := asFileReadOutput(second.Data)
	if err != nil || second.IsError || !outputOK || output.Type == FileReadVariantFileUnchanged || !strings.Contains(output.File.Content, "new") {
		t.Fatalf("path-swapped dedup returned stale unchanged result: result=%+v output=%+v err=%v", second, output, err)
	}
}

func TestP0BDigestOnlyReadObservationsNeverMergeCoverage(t *testing.T) {
	state := NewReadFileState()
	path := filepath.Join(t.TempDir(), "digest-only.txt")
	digest := fileContentDigest([]byte("one\ntwo\n"))
	state.RecordRead(path, ReadFileEntry{
		ContentDigest: digest, TotalLines: 2, CoverageKnown: true,
		Coverage: []ReadLineRange{{StartLine: 1, EndLine: 2}},
	})
	state.RecordRead(path, ReadFileEntry{
		ContentDigest: digest, TotalLines: 2, CoverageKnown: true,
		Coverage: []ReadLineRange{{StartLine: 2, EndLine: 3}},
	})
	entry, ok := state.Get(path)
	if !ok || entry.CoverageComplete || len(entry.Coverage) != 1 || entry.Coverage[0] != (ReadLineRange{StartLine: 2, EndLine: 3}) {
		t.Fatalf("digest-only observations merged into authorization: %+v", entry)
	}
}
