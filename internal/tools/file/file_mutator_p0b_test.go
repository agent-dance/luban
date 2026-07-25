package file

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

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

func TestP0BSameStatRollbackAndWeakEvidenceCannotAuthorizeEdit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollback.txt")
	p0bWriteFixture(t, path, "alpha\n")
	state := NewReadFileState()
	p0bReadEvidence(t, dir, path, state)
	entry, ok := state.GetForContext(context.Background(), path)
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
	state.SetForContext(context.Background(), path, ReadFileEntry{
		TimestampMs: rolledBack.ModTime().UnixMilli(), MtimeNs: rolledBack.ModTime().UnixNano(),
		TotalBytes: rolledBack.Size(), TotalLines: 1,
		Coverage:         []ReadLineRange{{StartLine: 1, EndLine: 2}},
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
	state.RecordReadForContext(context.Background(), path, ReadFileEntry{
		ContentDigest: digest, TotalLines: 2, Coverage: []ReadLineRange{{StartLine: 1, EndLine: 2}},
	})
	state.RecordReadForContext(context.Background(), path, ReadFileEntry{
		ContentDigest: digest, TotalLines: 2, Coverage: []ReadLineRange{{StartLine: 2, EndLine: 3}},
	})
	entry, ok := state.GetForContext(context.Background(), path)
	if !ok || entry.CoverageComplete || len(entry.Coverage) != 1 || entry.Coverage[0] != (ReadLineRange{StartLine: 2, EndLine: 3}) {
		t.Fatalf("digest-only observations merged into authorization: %+v", entry)
	}
}
