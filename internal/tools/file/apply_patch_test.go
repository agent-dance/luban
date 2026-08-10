package file

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/contracts/workspacerevision"
	"github.com/agent-dance/luban/types"
)

func newApplyPatchTestTool(root string) *ApplyPatchTool {
	return &ApplyPatchTool{
		AllowedDirs: []string{root},
		ReadState:   NewReadFileState(),
		Runtime: testRuntimeProvider{snapshot: types.ToolRuntimeContext{
			ProjectRoot: root, AllowedDirs: []string{root}, PermissionMode: "acceptEdits",
		}},
	}
}

func writeApplyPatchFixture(t testing.TB, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func requireApplyPatchErrorCode(t testing.TB, result types.ToolResult, code string) {
	t.Helper()
	if !result.IsError {
		t.Fatalf("expected error result, got %+v", result)
	}
	data, ok := result.Data.(types.ToolErrorData)
	if !ok {
		t.Fatalf("error data = %T, want ToolErrorData", result.Data)
	}
	if data.Code != code {
		t.Fatalf("error code = %q, want %q", data.Code, code)
	}
}

func TestApplyPatchContractAdvertisesDeterministicPreflightRules(t *testing.T) {
	tool := &ApplyPatchTool{}
	lang := i18n.DetectOrLoadLanguage()
	rules := i18n.Text(lang, i18n.KeyToolApplyPatchPreflightRules)
	if !strings.Contains(tool.Description(), rules) {
		t.Fatalf("ApplyPatch description omitted semantic preflight rules: %q", tool.Description())
	}
	patchProperty, ok := tool.Schema().Properties["patch"].(map[string]any)
	if !ok || !strings.Contains(fmt.Sprint(patchProperty["description"]), rules) {
		t.Fatalf("ApplyPatch patch schema omitted semantic preflight rules: %#v", patchProperty)
	}
}

func TestApplyPatchCustomTransactionUpdatesCreatesAndDeletes(t *testing.T) {
	root := t.TempDir()
	updated := filepath.Join(root, "updated.txt")
	deleted := filepath.Join(root, "deleted.txt")
	created := filepath.Join(root, "nested", "created.txt")
	writeApplyPatchFixture(t, updated, "alpha\nbeta\ngamma\n")
	writeApplyPatchFixture(t, deleted, "remove me\n")
	tool := newApplyPatchTestTool(root)
	seedCanonicalFileReadState(t, tool.ReadState, updated)
	seedCanonicalFileReadState(t, tool.ReadState, deleted)

	result, err := tool.Execute(context.Background(), map[string]any{"patch": strings.Join([]string{
		"*** Begin Patch",
		"*** Update File: updated.txt",
		"@@",
		" alpha",
		"-beta",
		"+BETA",
		" gamma",
		"*** Add File: nested/created.txt",
		"+first",
		"+second",
		"*** Delete File: deleted.txt",
		"*** End Patch",
	}, "\n")})
	if err != nil || result.IsError {
		t.Fatalf("ApplyPatch result=%+v err=%v", result, err)
	}
	if got, _ := os.ReadFile(updated); string(got) != "alpha\nBETA\ngamma\n" {
		t.Fatalf("updated content = %q", got)
	}
	if got, _ := os.ReadFile(created); string(got) != "first\nsecond\n" {
		t.Fatalf("created content = %q", got)
	}
	if _, err := os.Lstat(deleted); !os.IsNotExist(err) {
		t.Fatalf("deleted file still exists: %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal([]byte(result.Content), &wire); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.Content, "alpha") || strings.Contains(result.Content, "remove me") {
		t.Fatalf("result leaked a source snapshot: %s", result.Content)
	}
	if _, exists := wire["originalFile"]; exists {
		t.Fatalf("result contains originalFile: %v", wire)
	}
	typed, ok := result.Data.(ApplyPatchResult)
	if !ok || typed.Summary.Files != 3 || typed.Summary.Hunks != 2 || typed.Summary.Additions != 3 || typed.Summary.Deletions != 2 {
		t.Fatalf("unexpected diffstat: %#v", result.Data)
	}
}

func TestApplyPatchUnifiedDiffSupportsMultipleHunks(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.txt")
	writeApplyPatchFixture(t, path, "one\ntwo\nthree\nfour\nfive\n")
	tool := newApplyPatchTestTool(root)
	seedCanonicalFileReadState(t, tool.ReadState, path)
	patch := strings.Join([]string{
		"--- a/sample.txt",
		"+++ b/sample.txt",
		"@@ -1,2 +1,2 @@",
		" one",
		"-two",
		"+TWO",
		"@@ -4,2 +4,2 @@",
		" four",
		"-five",
		"+FIVE",
	}, "\n")
	result, err := tool.Execute(context.Background(), map[string]any{"patch": patch})
	if err != nil || result.IsError {
		t.Fatalf("ApplyPatch result=%+v err=%v", result, err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "one\nTWO\nthree\nfour\nFIVE\n" {
		t.Fatalf("content = %q", got)
	}
	if typed := result.Data.(ApplyPatchResult); typed.Summary.Hunks != 2 || typed.Summary.Additions != 2 || typed.Summary.Deletions != 2 {
		t.Fatalf("diffstat = %+v", typed.Summary)
	}
}

func TestApplyPatchReceiptSeparatesParameterChangeAndReceiptMetrics(t *testing.T) {
	root := t.TempDir()
	tool := newApplyPatchTestTool(root)
	patch := "*** Begin Patch\n*** Add File: target.txt\n+ONE\n*** End Patch"
	result, err := tool.Execute(context.Background(), map[string]any{"patch": patch})
	if err != nil || result.IsError {
		t.Fatalf("ApplyPatch result=%+v err=%v", result, err)
	}
	mapped := types.MapToolResult(tool, result, "metrics")
	for key, want := range map[string]string{
		"apply_patch.parameter_bytes": strconv.Itoa(len(patch)),
		"apply_patch.changed_files":   "1",
		"apply_patch.additions":       "1",
		"apply_patch.deletions":       "0",
		"apply_patch.receipt_bytes":   strconv.Itoa(len(mapped.Content)),
	} {
		if mapped.Metadata[key] != want {
			t.Errorf("metadata[%q] = %q, want %q", key, mapped.Metadata[key], want)
		}
	}
	if mapped.Metadata["apply_patch.parameter_bytes"] == mapped.Metadata["apply_patch.receipt_bytes"] {
		t.Fatalf("parameter and receipt metrics were conflated: %+v", mapped.Metadata)
	}
}

func TestApplyPatchTenKiBParameterAndMultiFileCompletionMetadata(t *testing.T) {
	root := t.TempDir()
	tool := newApplyPatchTestTool(root)
	prefix := "*** Begin Patch\n*** Add File: first.txt\n+"
	middle := "\n*** Add File: second.txt\n+second\n*** End Patch"
	patch := prefix + strings.Repeat("x", 10*1024-len(prefix)-len(middle)) + middle
	if len(patch) != 10*1024 {
		t.Fatalf("fixture parameter bytes = %d", len(patch))
	}
	result, err := tool.Execute(context.Background(), map[string]any{"patch": patch})
	if err != nil || result.IsError {
		t.Fatalf("ApplyPatch result=%+v err=%v", result, err)
	}
	mapped := types.MapToolResult(tool, result, "ten-kib-multi-file")
	for key, want := range map[string]string{
		"apply_patch.parameter_bytes": "10240",
		"apply_patch.changed_files":   "2",
		"apply_patch.additions":       "2",
		"apply_patch.deletions":       "0",
	} {
		if mapped.Metadata[key] != want {
			t.Errorf("metadata[%q] = %q, want %q", key, mapped.Metadata[key], want)
		}
	}
	for _, name := range []string{"first.txt", "second.txt"} {
		if _, statErr := os.Stat(filepath.Join(root, name)); statErr != nil {
			t.Fatalf("completed multi-file patch omitted %s: %v", name, statErr)
		}
	}
}

func TestApplyPatchUnifiedDiffCreatesAndDeletesWithContext(t *testing.T) {
	root := t.TempDir()
	oldPath := filepath.Join(root, "old.txt")
	newPath := filepath.Join(root, "new.txt")
	writeApplyPatchFixture(t, oldPath, "old one\nold two\n")
	tool := newApplyPatchTestTool(root)
	seedCanonicalFileReadState(t, tool.ReadState, oldPath)
	patch := strings.Join([]string{
		"--- /dev/null",
		"+++ b/new.txt",
		"@@ -0,0 +1,2 @@",
		"+new one",
		"+new two",
		"--- a/old.txt",
		"+++ /dev/null",
		"@@ -1,2 +0,0 @@",
		"-old one",
		"-old two",
	}, "\n")
	result, err := tool.Execute(context.Background(), map[string]any{"patch": patch})
	if err != nil || result.IsError {
		t.Fatalf("ApplyPatch result=%+v err=%v", result, err)
	}
	if got, _ := os.ReadFile(newPath); string(got) != "new one\nnew two\n" {
		t.Fatalf("created content = %q", got)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old file was not deleted: %v", err)
	}
}

func TestApplyPatchConflictLeavesEveryTargetUnchanged(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first.txt")
	second := filepath.Join(root, "second.txt")
	writeApplyPatchFixture(t, first, "first old\n")
	writeApplyPatchFixture(t, second, "second old\n")
	tool := newApplyPatchTestTool(root)
	seedCanonicalFileReadState(t, tool.ReadState, first)
	seedCanonicalFileReadState(t, tool.ReadState, second)
	patch := strings.Join([]string{
		"*** Begin Patch",
		"*** Update File: first.txt",
		"@@",
		"-first old",
		"+first new",
		"*** Update File: second.txt",
		"@@",
		"-missing context",
		"+second new",
		"*** End Patch",
	}, "\n")
	result, err := tool.Execute(context.Background(), map[string]any{"patch": patch})
	if err != nil {
		t.Fatal(err)
	}
	requireApplyPatchErrorCode(t, result, fileErrorApplyPatchAnchorMissing)
	for path, want := range map[string]string{first: "first old\n", second: "second old\n"} {
		got, _ := os.ReadFile(path)
		if string(got) != want {
			t.Fatalf("%s changed to %q", path, got)
		}
	}
}

func TestApplyPatchCommitCASRejectsAnExternalSnapshotChange(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	writeApplyPatchFixture(t, path, "old\n")
	tool := newApplyPatchTestTool(root)
	seedCanonicalFileReadState(t, tool.ReadState, path)
	tool.beforeCommitForTest = func(index int, commitPath string) {
		if index == 0 {
			writeApplyPatchFixture(t, commitPath, "external\n")
		}
	}
	patch := "*** Begin Patch\n*** Update File: target.txt\n@@\n-old\n+patched\n*** End Patch"
	result, err := tool.Execute(context.Background(), map[string]any{"patch": patch})
	if err != nil {
		t.Fatal(err)
	}
	requireApplyPatchErrorCode(t, result, fileErrorApplyPatchConflict)
	got, _ := os.ReadFile(path)
	if string(got) != "external\n" {
		t.Fatalf("CAS overwrote the external winner with %q", got)
	}
}

func TestApplyPatchContextFreeDeleteRequiresFreshFullRead(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "delete.txt")
	writeApplyPatchFixture(t, path, "content\n")
	tool := newApplyPatchTestTool(root)
	patch := "*** Begin Patch\n*** Delete File: delete.txt\n*** End Patch"
	result, err := tool.Execute(context.Background(), map[string]any{"patch": patch})
	if err != nil {
		t.Fatal(err)
	}
	requireApplyPatchErrorCode(t, result, fileErrorApplyPatchReadRequired)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file changed before Read: %v", err)
	}
	seedCanonicalFileReadState(t, tool.ReadState, path)
	result, err = tool.Execute(context.Background(), map[string]any{"patch": patch})
	if err != nil || result.IsError {
		t.Fatalf("ApplyPatch result=%+v err=%v", result, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file was not deleted: %v", err)
	}
}

func TestApplyPatchContextFreeDeleteRejectsStaleReadEvidence(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "delete.txt")
	writeApplyPatchFixture(t, path, "observed\n")
	tool := newApplyPatchTestTool(root)
	seedCanonicalFileReadState(t, tool.ReadState, path)
	writeApplyPatchFixture(t, path, "changed externally\n")
	patch := "*** Begin Patch\n*** Delete File: delete.txt\n*** End Patch"
	result, err := tool.Execute(context.Background(), map[string]any{"patch": patch})
	if err != nil {
		t.Fatal(err)
	}
	requireApplyPatchErrorCode(t, result, fileErrorApplyPatchReadRequired)
	got, _ := os.ReadFile(path)
	if string(got) != "changed externally\n" {
		t.Fatalf("stale evidence delete changed the file to %q", got)
	}
}

func TestApplyPatchUnifiedHeaderOnlyDeleteRequiresFullVisibleRead(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "delete.txt")
	writeApplyPatchFixture(t, path, "one\ntwo\nthree\n")
	tool := newApplyPatchTestTool(root)
	reader := &FileReadTool{AllowedDirs: []string{root}, ReadState: tool.ReadState}
	partial, partialErr := reader.Execute(context.Background(), map[string]any{
		"file_path": path, "offset": 1, "limit": 1,
	})
	if partialErr != nil || partial.IsError {
		t.Fatalf("partial read: result=%+v err=%v", partial, partialErr)
	}
	patch := "--- a/delete.txt\n+++ /dev/null"
	result, err := tool.Execute(context.Background(), map[string]any{"patch": patch})
	if err != nil {
		t.Fatal(err)
	}
	requireApplyPatchErrorCode(t, result, fileErrorApplyPatchReadRequired)
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("partial evidence authorized header-only delete: %v", statErr)
	}

	seedCanonicalFileReadState(t, tool.ReadState, path)
	result, err = tool.Execute(context.Background(), map[string]any{"patch": patch})
	if err != nil || result.IsError {
		t.Fatalf("full-visible delete: result=%+v err=%v", result, err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("header-only delete did not remove target: %v", statErr)
	}
}

func TestApplyPatchZeroCountInsertionUsesVisibleAdjacentAnchor(t *testing.T) {
	tests := []struct {
		name       string
		oldStart   int
		readLine   int
		insertLine string
		want       string
	}{
		{name: "bof", oldStart: 0, readLine: 1, insertLine: "zero", want: "zero\none\ntwo\nthree\n"},
		{name: "middle", oldStart: 2, readLine: 2, insertLine: "between", want: "one\ntwo\nbetween\nthree\n"},
		{name: "eof", oldStart: 3, readLine: 3, insertLine: "four", want: "one\ntwo\nthree\nfour\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "target.txt")
			writeApplyPatchFixture(t, path, "one\ntwo\nthree\n")
			tool := newApplyPatchTestTool(root)
			reader := &FileReadTool{AllowedDirs: []string{root}, ReadState: tool.ReadState}
			read, readErr := reader.Execute(context.Background(), map[string]any{
				"file_path": path, "offset": test.readLine, "limit": 1,
			})
			if readErr != nil || read.IsError {
				t.Fatalf("anchor read: result=%+v err=%v", read, readErr)
			}
			patch := strings.Join([]string{
				"--- a/target.txt", "+++ b/target.txt",
				fmt.Sprintf("@@ -%d,0 +%d,1 @@", test.oldStart, test.oldStart+1),
				"+" + test.insertLine,
			}, "\n")
			result, err := tool.Execute(context.Background(), map[string]any{"patch": patch})
			if err != nil || result.IsError {
				t.Fatalf("zero-count insertion: result=%+v err=%v", result, err)
			}
			got, _ := os.ReadFile(path)
			if string(got) != test.want {
				t.Fatalf("content = %q, want %q", got, test.want)
			}
		})
	}
}

func TestApplyPatchReadRecoveryUsesTypedInspectBatch(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	writeApplyPatchFixture(t, path, "old\n")
	tool := newApplyPatchTestTool(root)
	result, err := tool.Execute(context.Background(), map[string]any{"patch": "*** Begin Patch\n*** Update File: target.txt\n@@\n-old\n+new\n*** End Patch"})
	if err != nil {
		t.Fatal(err)
	}
	requireApplyPatchErrorCode(t, result, fileErrorApplyPatchReadRequired)
	data := result.Data.(types.ToolErrorData)
	if data.Retry == nil || data.Retry.Tool != "Inspect" || data.Retry.Action != "inspect_batch" ||
		len(data.Retry.Requests) != 1 || data.Retry.Requests[0].Kind != "read" ||
		data.Retry.Requests[0].Path != "target.txt" {
		t.Fatalf("retry = %#v, want typed Inspect batch", data.Retry)
	}
}

func TestApplyPatchPartialVisibleUpdateDoesNotPromotePrivateFullSnapshot(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	writeApplyPatchFixture(t, path, "one\ntwo\nthree\n")
	tool := newApplyPatchTestTool(root)
	reader := &FileReadTool{AllowedDirs: []string{root}, ReadState: tool.ReadState}
	read, readErr := reader.Execute(context.Background(), map[string]any{
		"file_path": path, "offset": 2, "limit": 1,
	})
	if readErr != nil || read.IsError {
		t.Fatalf("partial read: result=%+v err=%v", read, readErr)
	}
	result, err := tool.Execute(context.Background(), map[string]any{"patch": "--- a/target.txt\n+++ b/target.txt\n@@ -2,1 +2,1 @@\n-two\n+TWO"})
	if err != nil || result.IsError {
		t.Fatalf("partial-visible patch: result=%+v err=%v", result, err)
	}
	if entry, found := tool.ReadState.GetForContext(context.Background(), path); found {
		t.Fatalf("patch promoted unseen private source to mutation evidence: %+v", entry)
	}
}

func TestApplyPatchPreservesTypedAnchorFailures(t *testing.T) {
	tests := []struct {
		name    string
		content string
		patch   string
		code    string
	}{
		{name: "missing", content: "one\ntwo\n", patch: "*** Begin Patch\n*** Update File: target.txt\n@@\n-missing\n+new\n*** End Patch", code: fileErrorApplyPatchAnchorMissing},
		{name: "ambiguous", content: "same\nother\nsame\n", patch: "*** Begin Patch\n*** Update File: target.txt\n@@\n-same\n+new\n*** End Patch", code: fileErrorApplyPatchAnchorAmbiguous},
		{name: "position", content: "one\ntwo\n", patch: "--- a/target.txt\n+++ b/target.txt\n@@ -9,0 +10,1 @@\n+new", code: fileErrorApplyPatchPosition},
		{name: "eof", content: "one\ntwo\n", patch: "*** Begin Patch\n*** Update File: target.txt\n@@\n-one\n+new\n*** End of File\n*** End Patch", code: fileErrorApplyPatchEOF},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "target.txt")
			writeApplyPatchFixture(t, path, test.content)
			tool := newApplyPatchTestTool(root)
			seedCanonicalFileReadState(t, tool.ReadState, path)
			result, err := tool.Execute(context.Background(), map[string]any{"patch": test.patch})
			if err != nil {
				t.Fatal(err)
			}
			requireApplyPatchErrorCode(t, result, test.code)
		})
	}
}

func TestApplyPatchRejectsTraversalAndSymlinks(t *testing.T) {
	root := t.TempDir()
	tool := newApplyPatchTestTool(root)
	traversal := "*** Begin Patch\n*** Add File: ../escape.txt\n+bad\n*** End Patch"
	result, err := tool.Execute(context.Background(), map[string]any{"patch": traversal})
	if err != nil {
		t.Fatal(err)
	}
	requireApplyPatchErrorCode(t, result, fileErrorApplyPatchParse)

	realPath := filepath.Join(root, "real.txt")
	linkPath := filepath.Join(root, "link.txt")
	writeApplyPatchFixture(t, realPath, "old\n")
	if err := os.Symlink(realPath, linkPath); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	symlinkPatch := "*** Begin Patch\n*** Update File: link.txt\n@@\n-old\n+new\n*** End Patch"
	result, err = tool.Execute(context.Background(), map[string]any{"patch": symlinkPatch})
	if err != nil {
		t.Fatal(err)
	}
	requireApplyPatchErrorCode(t, result, fileErrorApplyPatchPermission)
	got, _ := os.ReadFile(realPath)
	if string(got) != "old\n" {
		t.Fatalf("symlink target changed to %q", got)
	}
}

func TestApplyPatchCommitFailureRollsBackCommittedFiles(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "a.txt")
	second := filepath.Join(root, "b.txt")
	writeApplyPatchFixture(t, first, "a old\n")
	writeApplyPatchFixture(t, second, "b old\n")
	tool := newApplyPatchTestTool(root)
	seedCanonicalFileReadState(t, tool.ReadState, first)
	seedCanonicalFileReadState(t, tool.ReadState, second)
	tool.afterCommitForTest = func(index int, _ string) error {
		if index == 0 {
			return errors.New("injected commit failure")
		}
		return nil
	}
	patch := strings.Join([]string{
		"*** Begin Patch",
		"*** Update File: a.txt",
		"@@",
		"-a old",
		"+a new",
		"*** Update File: b.txt",
		"@@",
		"-b old",
		"+b new",
		"*** End Patch",
	}, "\n")
	result, err := tool.Execute(context.Background(), map[string]any{"patch": patch})
	if err != nil {
		t.Fatal(err)
	}
	requireApplyPatchErrorCode(t, result, fileErrorApplyPatchCommit)
	if result.Metadata["apply_patch.failure_reason"] != "commit_failed" {
		t.Fatalf("commit failure proof reason = %q", result.Metadata["apply_patch.failure_reason"])
	}
	for path, want := range map[string]string{first: "a old\n", second: "b old\n"} {
		got, _ := os.ReadFile(path)
		if string(got) != want {
			t.Fatalf("rollback left %s as %q", path, got)
		}
	}
	temps, err := filepath.Glob(filepath.Join(root, ".luban-patch-*"))
	if err != nil || len(temps) != 0 {
		t.Fatalf("staging files remain: %v, err=%v", temps, err)
	}
}

func TestApplyPatchRollbackRemovesCreatedFilesAndDirectories(t *testing.T) {
	root := t.TempDir()
	tool := newApplyPatchTestTool(root)
	tool.afterCommitForTest = func(index int, _ string) error {
		if index == 0 {
			return errors.New("injected commit failure")
		}
		return nil
	}
	patch := strings.Join([]string{
		"*** Begin Patch",
		"*** Add File: nested/a.txt",
		"+a",
		"*** Add File: nested/b.txt",
		"+b",
		"*** End Patch",
	}, "\n")
	result, err := tool.Execute(context.Background(), map[string]any{"patch": patch})
	if err != nil {
		t.Fatal(err)
	}
	requireApplyPatchErrorCode(t, result, fileErrorApplyPatchCommit)
	if _, err := os.Stat(filepath.Join(root, "nested")); !os.IsNotExist(err) {
		entries, _ := os.ReadDir(filepath.Join(root, "nested"))
		t.Fatalf("rollback left the created tree: %v, entries=%v", err, entries)
	}
}

func TestApplyPatchPlanModeAndPathPermission(t *testing.T) {
	root := t.TempDir()
	tool := newApplyPatchTestTool(root)
	tool.PlanState = testPlanMode{active: true}
	patch := "*** Begin Patch\n*** Add File: new.txt\n+new\n*** End Patch"
	result, err := tool.Execute(context.Background(), map[string]any{"patch": patch})
	if err != nil {
		t.Fatal(err)
	}
	requireApplyPatchErrorCode(t, result, fileErrorApplyPatchPermission)
	if _, err := os.Stat(filepath.Join(root, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("plan mode wrote a file: %v", err)
	}

	tool.PlanState = nil
	decision, err := tool.CheckPermissions(context.Background(), map[string]any{"patch": patch}, types.ToolPermissionRequest{Runtime: types.ToolRuntimeContext{
		ProjectRoot: root,
		AllowedDirs: []string{root},
		DeniedRules: []types.PermissionRuleValue{{ToolName: "ApplyPatch", RuleContent: filepath.Join(root, "new.txt")}},
	}})
	if err != nil || decision.Behavior != types.PermissionBehaviorDeny || decision.BlockedPath == "" {
		t.Fatalf("permission decision=%+v err=%v", decision, err)
	}
	decision, err = tool.CheckPermissions(context.Background(), map[string]any{"patch": patch}, types.ToolPermissionRequest{Runtime: types.ToolRuntimeContext{
		ProjectRoot: root, AllowedDirs: []string{root}, PermissionMode: "acceptEdits",
	}})
	if err != nil || decision.Behavior != types.PermissionBehaviorAllow {
		t.Fatalf("acceptEdits decision=%+v err=%v", decision, err)
	}
}

func TestApplyPatchSuccessCarriesPostCommitWorkspaceRevision(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "revision.txt")
	writeApplyPatchFixture(t, path, "before\n")
	tool := newApplyPatchTestTool(root)
	tool.WorkspaceRevisions = workspacerevision.NewLedger()
	seedCanonicalFileReadState(t, tool.ReadState, path)

	result, err := tool.Execute(context.Background(), map[string]any{"patch": strings.Join([]string{
		"*** Begin Patch",
		"*** Update File: revision.txt",
		"@@",
		"-before",
		"+after",
		"*** End Patch",
	}, "\n")})
	if err != nil || result.IsError {
		t.Fatalf("ApplyPatch result=%+v err=%v", result, err)
	}
	typed, ok := result.Data.(ApplyPatchResult)
	if !ok {
		t.Fatalf("ApplyPatch Data = %T", result.Data)
	}
	receipt, ok := typed.WorkspaceRevisionReceipt()
	if !ok || receipt.Epoch() != 1 {
		t.Fatalf("revision receipt = %#v, ok=%t", receipt, ok)
	}
	if err := tool.WorkspaceRevisions.Validate(receipt); err != nil {
		t.Fatalf("post-commit revision was not current: %v", err)
	}
	if err := os.WriteFile(path, []byte("intervening\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := tool.WorkspaceRevisions.Validate(receipt); err == nil {
		t.Fatal("intervening mutation did not invalidate ApplyPatch receipt")
	}
}
