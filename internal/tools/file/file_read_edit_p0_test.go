package file

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/agent-dance/luban/types"
)

func p0WriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func p0ReplaceFileIdentity(t *testing.T, replacement, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
}

func p0Read(t *testing.T, tool *FileReadTool, input map[string]any) types.ToolResult {
	t.Helper()
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func p0Edit(t *testing.T, tool *FileEditTool, path, oldString, newString string) types.ToolResult {
	t.Helper()
	result, err := tool.Execute(context.Background(), map[string]any{
		"file_path": path, "old_string": oldString, "new_string": newString,
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestP0ReadLimitPastEOFIsCompleteAndEditable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "small.go")
	p0WriteFile(t, path, "one\ntwo\nthree")
	state := NewReadFileState()
	read := &FileReadTool{AllowedDirs: []string{dir}, ReadState: state}
	edit := &FileEditTool{AllowedDirs: []string{dir}, ReadState: state}

	result := p0Read(t, read, map[string]any{"file_path": path, "limit": 1000})
	if result.IsError {
		t.Fatalf("Read(limit past EOF) failed: %s", result.Content)
	}
	entry, ok := state.GetForContext(context.Background(), path)
	if !ok || entry.IsPartialView || !entry.CoverageComplete || !entry.FullSnapshot {
		t.Fatalf("entry did not record factual EOF coverage: %+v", entry)
	}
	dedup := p0Read(t, read, map[string]any{"file_path": path, "limit": 1000})
	if output, ok := asFileReadOutput(dedup.Data); !ok || output.Type != FileReadVariantFileUnchanged {
		t.Fatalf("identical ranged Read did not dedup: %#v", dedup.Data)
	}
	if result := p0Edit(t, edit, path, "two", "TWO"); result.IsError {
		t.Fatalf("Edit after EOF-covering ranged Read failed: %s", result.Content)
	}
}

func TestP0TokenLimitedLargeFileTargetedReadThenEdit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "large.go")
	var content strings.Builder
	for i := 1; i <= 8000; i++ {
		fmt.Fprintf(&content, "line-%05d alpha beta gamma\n", i)
	}
	p0WriteFile(t, path, content.String())
	state := NewReadFileState()
	read := &FileReadTool{
		AllowedDirs: []string{dir}, ReadState: state,
		PreciseTokenCounter: func(_ context.Context, value string) (int, error) {
			return len(strings.Fields(value)), nil
		},
	}
	edit := &FileEditTool{AllowedDirs: []string{dir}, ReadState: state}

	full := p0Read(t, read, map[string]any{"file_path": path})
	if !full.IsError {
		t.Fatal("full Read should exceed the token limit fixture")
	}
	if data, ok := full.Data.(types.ToolErrorData); !ok || data.Code != fileErrorReadTokenLimit {
		t.Fatalf("full Read error is not structured: %#v", full.Data)
	}
	targeted := p0Read(t, read, map[string]any{"file_path": path, "offset": 7000, "limit": 3})
	if targeted.IsError {
		t.Fatalf("targeted Read failed: %s", targeted.Content)
	}
	if result := p0Edit(t, edit, path, "line-07000 alpha beta gamma", "line-07000 changed safely"); result.IsError {
		t.Fatalf("targeted Read -> Edit failed: %s", result.Content)
	}
}

func TestP0FullReadThenRangeReadPreservesFullEvidence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "preserve.go")
	p0WriteFile(t, path, "first\nsecond\nthird\nfourth")
	state := NewReadFileState()
	read := &FileReadTool{AllowedDirs: []string{dir}, ReadState: state}
	edit := &FileEditTool{AllowedDirs: []string{dir}, ReadState: state}

	if result := p0Read(t, read, map[string]any{"file_path": path}); result.IsError {
		t.Fatal(result.Content)
	}
	if result := p0Read(t, read, map[string]any{"file_path": path, "offset": 2, "limit": 1}); result.IsError {
		t.Fatal(result.Content)
	}
	entry, _ := state.GetForContext(context.Background(), path)
	if !entry.CoverageComplete || !entry.FullSnapshot || entry.Content != "first\nsecond\nthird\nfourth" {
		t.Fatalf("focused Read downgraded full evidence: %+v", entry)
	}
	if result := p0Edit(t, edit, path, "fourth", "FOURTH"); result.IsError {
		t.Fatalf("Edit outside latest range lost prior full evidence: %s", result.Content)
	}
}

func TestP0ObservedRangesUnionAcrossReads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "union.go")
	p0WriteFile(t, path, "one\ntwo\nthree\nfour")
	state := NewReadFileState()
	read := &FileReadTool{AllowedDirs: []string{dir}, ReadState: state}
	edit := &FileEditTool{AllowedDirs: []string{dir}, ReadState: state}

	p0Read(t, read, map[string]any{"file_path": path, "offset": 2, "limit": 1})
	p0Read(t, read, map[string]any{"file_path": path, "offset": 3, "limit": 1})
	entry, _ := state.GetForContext(context.Background(), path)
	if len(entry.Coverage) != 1 || entry.Coverage[0] != (ReadLineRange{StartLine: 2, EndLine: 4}) {
		t.Fatalf("adjacent observations were not merged: %+v", entry.Coverage)
	}
	if result := p0Edit(t, edit, path, "two\nthree", "TWO\nTHREE"); result.IsError {
		t.Fatalf("multi-line anchor in unioned coverage failed: %s", result.Content)
	}
}

func TestP0FullReadMtimeTouchContentUnchangedEdit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "touch.go")
	p0WriteFile(t, path, "alpha\nbeta\n")
	state := NewReadFileState()
	read := &FileReadTool{AllowedDirs: []string{dir}, ReadState: state}
	edit := &FileEditTool{AllowedDirs: []string{dir}, ReadState: state}
	p0Read(t, read, map[string]any{"file_path": path})
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	if result := p0Edit(t, edit, path, "beta", "BETA"); result.IsError {
		t.Fatalf("mtime-only touch rejected unchanged full snapshot: %s", result.Content)
	}
}

func TestP0UTF8BoundaryReadThenEdit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "utf8.txt")
	original := strings.Repeat("a", 8191) + "中\nold\n"
	p0WriteFile(t, path, original)
	state := NewReadFileState()
	read := &FileReadTool{AllowedDirs: []string{dir}, ReadState: state}
	edit := &FileEditTool{AllowedDirs: []string{dir}, ReadState: state}
	if result := p0Read(t, read, map[string]any{"file_path": path}); result.IsError {
		t.Fatal(result.Content)
	}
	entry, _ := state.GetForContext(context.Background(), path)
	if entry.Encoding != EncodingUTF8 || strings.Contains(entry.Content, "ä¸") {
		t.Fatalf("valid UTF-8 was misclassified at the 8192-byte boundary: encoding=%q", entry.Encoding)
	}
	if result := p0Edit(t, edit, path, "old", "new"); result.IsError {
		t.Fatal(result.Content)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !utf8.Valid(raw) || !strings.Contains(string(raw), "中\nnew") {
		t.Fatalf("Read/Edit corrupted UTF-8: %q", raw[len(raw)-32:])
	}
}

func TestP0StructuredEditErrorIsModelVisible(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unread.go")
	p0WriteFile(t, path, "old")
	tool := &FileEditTool{AllowedDirs: []string{dir}, ReadState: NewReadFileState()}
	result := p0Edit(t, tool, path, "old", "new")
	if !result.IsError {
		t.Fatal("unread Edit unexpectedly succeeded")
	}
	data, ok := result.Data.(types.ToolErrorData)
	if !ok || data.Code != fileErrorReadRequired || data.Retry == nil || data.Retry.Tool != "Read" {
		t.Fatalf("unexpected structured error: %#v", result.Data)
	}
	block := types.MapToolResult(tool, result, "toolu_p0")
	if !strings.Contains(block.Content, "<tool_error>") || !strings.Contains(block.Content, fileErrorReadRequired) {
		t.Fatalf("stable error envelope is not model-visible: %q", block.Content)
	}
}

func TestP0EditOutsideObservedRangeReturnsExecutableRetry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "range.go")
	p0WriteFile(t, path, "one\ntwo\nthree\n")
	state := NewReadFileState()
	read := &FileReadTool{AllowedDirs: []string{dir}, ReadState: state}
	edit := &FileEditTool{AllowedDirs: []string{dir}, ReadState: state}
	p0Read(t, read, map[string]any{"file_path": path, "offset": 1, "limit": 1})
	result := p0Edit(t, edit, path, "three", "THREE")
	data, ok := result.Data.(types.ToolErrorData)
	if !result.IsError || !ok || data.Code != fileErrorAnchorUnobserved || data.Retry == nil || data.Retry.Offset != 3 || data.Retry.Limit != 1 {
		t.Fatalf("unexpected range recovery contract: result=%+v data=%#v", result, result.Data)
	}
	if data.Coverage == nil || len(data.Coverage.Observed) != 1 || data.Coverage.Observed[0] != (types.ToolErrorRange{StartLine: 1, EndLine: 1}) {
		t.Fatalf("observed coverage missing: %#v", data.Coverage)
	}
}

func TestP0ReplaceAllRetryAdvancesAcrossUncoveredTokenPages(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "replace-all-pages.go")
	oldBlock := "a\nb\nc\nd\ne\nf"
	newBlock := "A\nB\nC\nD\nE\nF"
	p0WriteFile(t, path, oldBlock+"\nseparator-1\nseparator-2\nseparator-3\n"+oldBlock+"\ntail")

	state := NewReadFileState()
	initialRead := &FileReadTool{AllowedDirs: []string{dir}, ReadState: state}
	limitedRead := &FileReadTool{
		AllowedDirs: []string{dir}, ReadState: state,
		PreciseTokenCounter: func(_ context.Context, value string) (int, error) {
			return len(strings.Fields(value)), nil
		},
	}
	edit := &FileEditTool{AllowedDirs: []string{dir}, ReadState: state}

	// Only the first of two replace_all matches is initially visible. The
	// retry must start at the second match rather than looping back to line 1.
	if result := p0Read(t, initialRead, map[string]any{"file_path": path, "offset": 1, "limit": 6}); result.IsError {
		t.Fatal(result.Content)
	}
	t.Setenv(fileReadMaxOutputTokensEnv, "2")
	executeEdit := func() types.ToolResult {
		t.Helper()
		result, err := edit.Execute(context.Background(), map[string]any{
			"file_path": path, "old_string": oldBlock, "new_string": newBlock, "replace_all": true,
		})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}

	// The six-line second match is too large for this reader. Each published
	// Edit retry is therefore paged to two lines by Read, unioned into evidence,
	// and the following Edit retry must advance by exactly those two lines.
	for index, wantOffset := range []int{10, 12, 14} {
		result := executeEdit()
		data, ok := result.Data.(types.ToolErrorData)
		wantRequired := types.ToolErrorRange{StartLine: wantOffset, EndLine: 15}
		if !result.IsError || !ok || data.Code != fileErrorAnchorUnobserved || data.Retry == nil ||
			data.Retry.Offset != wantOffset || data.Retry.Limit != 16-wantOffset {
			t.Fatalf("step %d did not advance retry to the uncovered suffix: result=%+v data=%#v", index, result, result.Data)
		}
		if data.Coverage == nil || len(data.Coverage.Required) != 1 || data.Coverage.Required[0] != wantRequired {
			t.Fatalf("step %d required covered lines again: coverage=%#v want=%+v", index, data.Coverage, wantRequired)
		}

		readResult := p0Read(t, limitedRead, map[string]any{
			"file_path": data.Retry.FilePath, "offset": data.Retry.Offset, "limit": data.Retry.Limit,
		})
		if wantOffset < 14 {
			readError, readOK := readResult.Data.(types.ToolErrorData)
			if !readResult.IsError || !readOK || readError.Code != fileErrorReadTokenLimit || readError.Retry == nil ||
				readError.Retry.Offset != wantOffset || readError.Retry.Limit != 2 {
				t.Fatalf("step %d token paging did not publish an executable prefix: result=%+v data=%#v", index, readResult, readResult.Data)
			}
			readResult = p0Read(t, limitedRead, map[string]any{
				"file_path": readError.Retry.FilePath, "offset": readError.Retry.Offset, "limit": readError.Retry.Limit,
			})
		}
		if readResult.IsError {
			t.Fatalf("step %d structured Read retry failed: %s", index, readResult.Content)
		}

		entry, found := state.GetForContext(context.Background(), path)
		wantEnd := wantOffset + 2
		if !found || len(entry.Coverage) != 2 || entry.Coverage[0] != (ReadLineRange{StartLine: 1, EndLine: 7}) ||
			entry.Coverage[1] != (ReadLineRange{StartLine: 10, EndLine: wantEnd}) {
			t.Fatalf("step %d did not monotonically union Read evidence: %+v", index, entry.Coverage)
		}
	}

	if result := executeEdit(); result.IsError {
		t.Fatalf("replace_all did not succeed after all structured retries: %s", result.Content)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(raw), newBlock); got != 2 || strings.Contains(string(raw), oldBlock) {
		t.Fatalf("replace_all post-image is wrong: replacements=%d content=%q", got, raw)
	}
}

func TestP0RangedReadAllowsScopedEditButNotWholeFileWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "write-scope.go")
	p0WriteFile(t, path, "one\ntwo\nthree")
	state := NewReadFileState()
	read := &FileReadTool{AllowedDirs: []string{dir}, ReadState: state}
	edit := &FileEditTool{AllowedDirs: []string{dir}, ReadState: state}
	write := &FileWriteTool{AllowedDirs: []string{dir}, ReadState: state}
	p0Read(t, read, map[string]any{"file_path": path, "offset": 2, "limit": 1})
	writeResult, err := write.Execute(context.Background(), map[string]any{"file_path": path, "content": "replacement"})
	if err != nil {
		t.Fatal(err)
	}
	data, ok := writeResult.Data.(types.ToolErrorData)
	if !writeResult.IsError || !ok || data.Code != fileErrorWriteFullRead {
		t.Fatalf("whole-file Write accepted ranged evidence: %+v %#v", writeResult, writeResult.Data)
	}
	if result := p0Edit(t, edit, path, "two", "TWO"); result.IsError {
		t.Fatalf("scoped Edit should remain allowed: %s", result.Content)
	}
}

func TestP0MixedLineEndingTiePrefersLF(t *testing.T) {
	if got := detectLineEnding("one\r\ntwo\n"); got != "\n" {
		t.Fatalf("equal CRLF/LF counts selected %q, want LF", got)
	}
}

func TestP0SameStatDifferentContentDoesNotDedupOrAuthorizeEdit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "same-stat.go")
	p0WriteFile(t, path, "alpha\nbeta")
	state := NewReadFileState()
	read := &FileReadTool{AllowedDirs: []string{dir}, ReadState: state}
	edit := &FileEditTool{AllowedDirs: []string{dir}, ReadState: state}
	first := p0Read(t, read, map[string]any{"file_path": path})
	if first.IsError {
		t.Fatal(first.Content)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	p0WriteFile(t, path, "gamma\nzeta") // same byte count and line count
	if err := os.Chtimes(path, before.ModTime(), before.ModTime()); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
		t.Skip("test filesystem cannot restore same-stat fixture")
	}

	second := p0Read(t, read, map[string]any{"file_path": path})
	output, ok := asFileReadOutput(second.Data)
	if second.IsError || !ok || output.Type == FileReadVariantFileUnchanged || output.File.Content != "gamma\nzeta" {
		t.Fatalf("digest-blind dedup hid same-stat content change: result=%+v output=%+v", second, output)
	}
	// Restore the old evidence explicitly so Edit proves its server-side digest
	// check, rather than relying on the second Read's refreshed evidence.
	oldDigest := fileContentDigest([]byte("alpha\nbeta"))
	state.SetForContext(context.Background(), path, ReadFileEntry{
		TimestampMs: before.ModTime().UnixMilli(), MtimeNs: before.ModTime().UnixNano(),
		TotalBytes: before.Size(), TotalLines: 2, ContentDigest: oldDigest, FileIdentity: before,
		Coverage:         []ReadLineRange{{StartLine: 1, EndLine: 3}},
		CoverageComplete: true, FullSnapshot: true, Content: "alpha\nbeta", LastTool: "Read", DedupEligible: true,
	})
	result := p0Edit(t, edit, path, "gamma", "GAMMA")
	data, ok := result.Data.(types.ToolErrorData)
	if !result.IsError || !ok || data.Code != fileErrorSnapshotStale {
		t.Fatalf("same-stat content forgery authorized Edit: result=%+v data=%#v", result, result.Data)
	}
}

func TestP0ReadIdentityRejectsSameContentReplacement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "identity.go")
	replacement := filepath.Join(dir, "replacement.go")
	const content = "alpha\nbeta\n"
	p0WriteFile(t, path, content)

	state := NewReadFileState()
	read := &FileReadTool{AllowedDirs: []string{dir}, ReadState: state}
	edit := &FileEditTool{AllowedDirs: []string{dir}, ReadState: state}
	if result := p0Read(t, read, map[string]any{"file_path": path}); result.IsError {
		t.Fatal(result.Content)
	}
	entry, ok := state.GetForContext(context.Background(), path)
	if !ok || entry.FileIdentity == nil || entry.ContentDigest != fileContentDigest([]byte(content)) {
		t.Fatalf("Read did not retain descriptor identity and digest: %+v", entry)
	}

	p0WriteFile(t, replacement, content)
	replacementInfo, err := os.Stat(replacement)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(entry.FileIdentity, replacementInfo) {
		t.Fatal("replacement fixture unexpectedly reused the original file identity")
	}
	p0ReplaceFileIdentity(t, replacement, path)

	result := p0Edit(t, edit, path, "beta", "BETA")
	data, dataOK := result.Data.(types.ToolErrorData)
	if !result.IsError || !dataOK || data.Code != fileErrorSnapshotStale {
		t.Fatalf("same-content replacement authorized Edit: result=%+v data=%#v", result, result.Data)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Fatalf("rejected Edit still changed replacement: %q", got)
	}
}

func TestP0ReadCoverageDoesNotMergeAcrossSameContentIdentityChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "coverage.go")
	replacement := filepath.Join(dir, "coverage-replacement.go")
	const content = "one\ntwo\nthree\n"
	p0WriteFile(t, path, content)

	state := NewReadFileState()
	read := &FileReadTool{AllowedDirs: []string{dir}, ReadState: state}
	if result := p0Read(t, read, map[string]any{"file_path": path, "offset": 1, "limit": 1}); result.IsError {
		t.Fatal(result.Content)
	}
	first, ok := state.GetForContext(context.Background(), path)
	if !ok || first.FileIdentity == nil {
		t.Fatalf("first Read did not retain descriptor identity: %+v", first)
	}

	p0WriteFile(t, replacement, content)
	p0ReplaceFileIdentity(t, replacement, path)
	if result := p0Read(t, read, map[string]any{"file_path": path, "offset": 2, "limit": 1}); result.IsError {
		t.Fatal(result.Content)
	}
	second, ok := state.GetForContext(context.Background(), path)
	if !ok || second.FileIdentity == nil || os.SameFile(first.FileIdentity, second.FileIdentity) {
		t.Fatalf("second Read did not replace stale identity evidence: first=%+v second=%+v", first, second)
	}
	want := []ReadLineRange{{StartLine: 2, EndLine: 3}}
	if len(second.Coverage) != len(want) || second.Coverage[0] != want[0] {
		t.Fatalf("coverage crossed file identities: got=%+v want=%+v", second.Coverage, want)
	}
}

func TestP0NewFilePublishNeverReplacesConcurrentWinner(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "winner.txt")
	p0WriteFile(t, path, "external winner")
	err := atomicCreateFile(path, []byte("tool candidate"), 0o644)
	if !errors.Is(err, fs.ErrExist) {
		t.Fatalf("atomic create error = %v, want fs.ErrExist", err)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "external winner" {
		t.Fatalf("atomic create replaced concurrent winner: %q", got)
	}
}

func TestP0VisibleReadAndDigestShareOpenedSnapshot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snapshot.go")
	raw := []byte("alpha\nbeta\n")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	limit := 1
	result, digest, err := readFileInRangeFromOpenFile(
		context.Background(), file, info, path, 0, &limit, nil, ReadFileRangeOptions{},
	)
	if err != nil || result.Content != "alpha" || digest != fileContentDigest(raw) {
		t.Fatalf("visible range and digest did not share the opened snapshot: result=%+v digest=%q err=%v", result, digest, err)
	}
}

func TestP0OpenedReadSnapshotRejectsPathReplacement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "swap.go")
	replacement := filepath.Join(dir, "replacement.go")
	p0WriteFile(t, path, "from-open-fd\n")
	p0WriteFile(t, replacement, "from-new-path\n")
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readFileInRangeFromOpenFile(
		context.Background(), file, info, path, 0, nil, nil, ReadFileRangeOptions{},
	); err == nil {
		t.Fatal("Read accepted visible content/digest after the authorized path identity was replaced")
	}
}

func TestP0EditPrecommitCASChecksIdentityAndDigest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cas.go")
	p0WriteFile(t, path, "alpha\nbeta")
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := readEditTarget(path, info)
	if err != nil {
		t.Fatal(err)
	}
	p0WriteFile(t, path, "gamma\nzeta")
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	tool := &FileEditTool{AllowedDirs: []string{dir}}
	if err := tool.recheckEditTarget(path, snapshot.Info, snapshot.ContentDigest); !errors.Is(err, errEditSnapshotCASMismatch) {
		t.Fatalf("precommit CAS accepted same-stat different content: %v", err)
	}
}

func TestP0AtomicEditCASRunsAfterPreparationBeforeRename(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "commit-cas.go")
	p0WriteFile(t, path, "original")
	called := false
	err := atomicWriteFileWithEditCAS(path, []byte("assistant"), 0o644, func() error {
		called = true
		p0WriteFile(t, path, "external")
		return errEditSnapshotCASMismatch
	})
	if !called || !errors.Is(err, errEditSnapshotCASMismatch) {
		t.Fatalf("commit CAS was not invoked or its conflict was lost: called=%v err=%v", called, err)
	}
	raw, readErr := os.ReadFile(path)
	if readErr != nil || string(raw) != "external" {
		t.Fatalf("CAS conflict overwrote external content: content=%q err=%v", raw, readErr)
	}
	matches, globErr := filepath.Glob(filepath.Join(dir, ".atomic-*"))
	if globErr != nil || len(matches) != 0 {
		t.Fatalf("CAS conflict leaked temporary files: matches=%v err=%v", matches, globErr)
	}
}

func TestP0TokenLimitRetryIsVerifiedAndExecutable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dense.go")
	content := strings.Repeat("dense\n", 30000)
	p0WriteFile(t, path, content)
	read := &FileReadTool{
		AllowedDirs: []string{dir}, ReadState: NewReadFileState(),
		PreciseTokenCounter: func(_ context.Context, value string) (int, error) {
			return len(strings.Fields(value)), nil
		},
	}
	failed := p0Read(t, read, map[string]any{"file_path": path})
	data, ok := failed.Data.(types.ToolErrorData)
	if !failed.IsError || !ok || data.Code != fileErrorReadTokenLimit || data.Retry == nil {
		t.Fatalf("token failure lacked verified recovery: %+v %#v", failed, failed.Data)
	}
	retry := p0Read(t, read, map[string]any{
		"file_path": data.Retry.FilePath, "offset": data.Retry.Offset, "limit": data.Retry.Limit,
	})
	if retry.IsError {
		t.Fatalf("published token retry failed again: retry=%+v result=%s", data.Retry, retry.Content)
	}
}

func TestP0OversizedSingleLineDoesNotAdvertiseImpossibleRange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "single-line.go")
	p0WriteFile(t, path, strings.Repeat("dense ", 30000))
	read := &FileReadTool{
		AllowedDirs: []string{dir}, ReadState: NewReadFileState(),
		PreciseTokenCounter: func(_ context.Context, value string) (int, error) {
			return len(strings.Fields(value)), nil
		},
	}
	failed := p0Read(t, read, map[string]any{"file_path": path})
	data, ok := failed.Data.(types.ToolErrorData)
	if !failed.IsError || !ok || data.Code != fileErrorReadTokenLimit || data.Retry != nil {
		t.Fatalf("single over-limit line advertised an impossible line retry: %+v %#v", failed, failed.Data)
	}
	if _, found := read.ReadState.GetForContext(context.Background(), path); found {
		t.Fatal("failed oversized Read seeded edit evidence")
	}
}
