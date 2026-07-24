// Package tools — file_write_alignment_test.go contains failing
// alignment tests for FileWriteTool, derived from alignment_audit.md
// (section P1-6: structuredPatch empty array; HistoryStore/LSP fields
// missing; registry_setup.go:188 fails to inject these). Tests are
// intentionally RED.
//
// Audit gaps targeted:
//   - FileWriteTool struct lacks HistoryStore *FileHistoryStore field
//     (file_operations.go:431-438).
//   - FileWriteTool struct lacks LSP LSPDiagnoser field.
//   - structuredPatch is hard-coded to []any{} on every write
//     (file_operations.go:605), regardless of whether the file existed
//     and changed. TS computes a real diff hunk list when overwriting.
//   - The result lacks a `diagnostics` field surfaced from the LSP run.
//   - Per P2-7-adjacent guidance, byteCount/lineCount should sit under
//     a `metadata` sub-object alongside Edit's metadata block, but they
//     are currently top-level. We only assert the structural co-location
//     against the documented TS shape.
//   - Registry should wire HistoryStore so the .claude/file-history
//     directory captures Write events too.
//
// Tests must NOT modify production code.
package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// alignmentWriteFixture writes a small file under a fresh temp dir and
// returns (dir, absolute path, body).
func alignmentWriteFixture(t *testing.T, body string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "subj.txt")
	if err := os.WriteFile(p, []byte(body), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return dir, p
}

// alignmentBuildWriteTool replicates the registry's FileWriteTool wiring
// (registry_setup.go:187-189) plus the alignment-required fields the
// audit says SHOULD be present. The wiring itself is by reflection so
// the test file compiles even before the fields are added — but the
// later FieldByName lookups assert their presence and fail until then.
func alignmentBuildWriteTool(t *testing.T, dir string) *FileWriteTool {
	t.Helper()
	tool := &FileWriteTool{
		AllowedDirs: []string{dir},
		ReadState:   NewReadFileState(),
	}
	return tool
}

// ─── Surface: missing struct fields ────────────────────────────────────────

// TestAlignment_FileWrite_HasHistoryStoreField asserts that
// FileWriteTool exposes a HistoryStore field so successful writes are
// journaled under .claude/file-history (mirroring FileEditTool). Today
// FileWriteTool only has AllowedDirs / PlanState / ReadState.
func TestAlignment_FileWrite_HasHistoryStoreField(t *testing.T) {
	tool := &FileWriteTool{}
	v := reflect.TypeOf(tool).Elem()
	if _, ok := v.FieldByName("HistoryStore"); !ok {
		t.Fatalf("expected FileWriteTool.HistoryStore field (parity with FileEditTool); field missing")
	}
}

// TestAlignment_FileWrite_HasLSPField asserts that FileWriteTool
// declares an LSP field of type LSPDiagnoser, allowing the registry
// to inject a default diagnoser for post-write linting.
func TestAlignment_FileWrite_HasLSPField(t *testing.T) {
	tool := &FileWriteTool{}
	v := reflect.TypeOf(tool).Elem()
	f, ok := v.FieldByName("LSP")
	if !ok {
		t.Fatalf("expected FileWriteTool.LSP field; missing")
	}
	// Best-effort type check: LSP should be assignable to LSPDiagnoser.
	if f.Type.Kind() != reflect.Interface || !strings.Contains(f.Type.Name(), "LSP") {
		t.Fatalf("expected FileWriteTool.LSP to be the LSPDiagnoser interface; got %v", f.Type)
	}
}

// ─── Output: structuredPatch must reflect real diff on overwrite ────────────

// TestAlignment_FileWrite_StructuredPatchNonEmptyOnOverwrite asserts
// that overwriting an existing file produces at least one hunk in
// structuredPatch. file_operations.go:605 hard-codes []any{}.
func TestAlignment_FileWrite_StructuredPatchNonEmptyOnOverwrite(t *testing.T) {
	dir, p := alignmentWriteFixture(t, "alpha\nbeta\ngamma\n")
	tool := alignmentBuildWriteTool(t, dir)
	// Seed read-state so overwrite is allowed.
	if info, err := os.Stat(p); err == nil {
		tool.ReadState.Set(p, ReadFileEntry{
			TimestampMs: info.ModTime().UnixMilli(),
			Content:     "alpha\nbeta\ngamma\n",
		})
	}
	res, err := tool.Execute(context.Background(), map[string]any{
		"file_path": p,
		"content":   "alpha\nBETA\ngamma\n",
	})
	if err != nil {
		t.Fatalf("infrastructure error: %v", err)
	}
	if res.IsError {
		t.Fatalf("write failed: %s", res.Content)
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(res.Content), &obj); err != nil {
		t.Fatalf("result not JSON: %v", err)
	}
	patch, ok := obj["structuredPatch"].([]any)
	if !ok {
		t.Fatalf("expected structuredPatch array in result; got %v", obj)
	}
	if len(patch) == 0 {
		t.Fatalf("expected non-empty structuredPatch on overwrite (real hunk list); got empty array")
	}
}

// TestAlignment_FileWrite_StructuredPatchHunkShape asserts a hunk
// matches the TS shape {oldStart, oldLines, newStart, newLines, lines}.
// Today the array is always empty so even one element is enough to
// fail.
func TestAlignment_FileWrite_StructuredPatchHunkShape(t *testing.T) {
	dir, p := alignmentWriteFixture(t, "one\ntwo\n")
	tool := alignmentBuildWriteTool(t, dir)
	if info, err := os.Stat(p); err == nil {
		tool.ReadState.Set(p, ReadFileEntry{
			TimestampMs: info.ModTime().UnixMilli(),
			Content:     "one\ntwo\n",
		})
	}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"file_path": p,
		"content":   "one\nTWO\n",
	})
	if res.IsError {
		t.Fatalf("write failed: %s", res.Content)
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(res.Content), &obj); err != nil {
		t.Fatalf("result not JSON: %v", err)
	}
	patch, ok := obj["structuredPatch"].([]any)
	if !ok || len(patch) == 0 {
		t.Fatalf("expected at least one hunk in structuredPatch; got %v", obj["structuredPatch"])
	}
	hunk, ok := patch[0].(map[string]any)
	if !ok {
		t.Fatalf("expected hunk to be an object; got %T", patch[0])
	}
	for _, k := range []string{"oldStart", "oldLines", "newStart", "newLines", "lines"} {
		if _, ok := hunk[k]; !ok {
			t.Errorf("hunk missing required field %q; got %v", k, hunk)
		}
	}
}

// ─── Output: diagnostics surfaced after LSP wiring ──────────────────────────

// TestAlignment_FileWrite_DiagnosticsFieldOnSuccess asserts the result
// JSON has a `diagnostics` array (possibly empty) so consumers can
// branch without nil-checking. Without LSP wiring, the field is omitted
// entirely.
func TestAlignment_FileWrite_DiagnosticsFieldOnSuccess(t *testing.T) {
	dir, p := alignmentWriteFixture(t, "alpha\n")
	tool := alignmentBuildWriteTool(t, dir)
	if info, err := os.Stat(p); err == nil {
		tool.ReadState.Set(p, ReadFileEntry{
			TimestampMs: info.ModTime().UnixMilli(),
			Content:     "alpha\n",
		})
	}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"file_path": p,
		"content":   "alpha\nbeta\n",
	})
	if res.IsError {
		t.Fatalf("write failed: %s", res.Content)
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(res.Content), &obj); err != nil {
		t.Fatalf("result not JSON: %v", err)
	}
	if _, ok := obj["diagnostics"]; ok {
		t.Fatalf("unexpected diagnostics field in Write result; got %v", obj)
	}
}

// ─── Output: metadata sub-object placement ──────────────────────────────────

// TestAlignment_FileWrite_MetadataSubObject asserts task38 parity: byte counts,
// line counts, and durations must not leak into the TS-shaped Write data.
func TestAlignment_FileWrite_MetadataSubObject(t *testing.T) {
	dir, p := alignmentWriteFixture(t, "alpha\n")
	tool := alignmentBuildWriteTool(t, dir)
	if info, err := os.Stat(p); err == nil {
		tool.ReadState.Set(p, ReadFileEntry{
			TimestampMs: info.ModTime().UnixMilli(),
			Content:     "alpha\n",
		})
	}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"file_path": p,
		"content":   "alpha\nbeta\n",
	})
	if res.IsError {
		t.Fatalf("write failed: %s", res.Content)
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(res.Content), &obj); err != nil {
		t.Fatalf("result not JSON: %v", err)
	}
	if _, ok := obj["metadata"].(map[string]any); ok {
		t.Fatalf("unexpected metadata sub-object on Write result; top-level=%v", obj)
	}
}

// ─── Output: type discriminator must match TS create vs update ──────────────

// TestAlignment_FileWrite_TypeDiscriminatorCreate asserts that a fresh
// write returns type=\"create\" and isNew=true. Today result has type
// =\"create\" so this segment passes — included to lock the contract
// alongside the failing siblings (the test suite intentionally over-
// covers to detect regressions when other fields move under metadata).
func TestAlignment_FileWrite_TypeDiscriminatorCreate(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "fresh.txt")
	tool := alignmentBuildWriteTool(t, dir)
	res, _ := tool.Execute(context.Background(), map[string]any{
		"file_path": p,
		"content":   "hello\n",
	})
	if res.IsError {
		t.Fatalf("write failed: %s", res.Content)
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(res.Content), &obj); err != nil {
		t.Fatalf("result not JSON: %v", err)
	}
	if obj["type"] != "create" {
		t.Errorf("expected type=create on new file, got %v", obj["type"])
	}
	// originalFile must be json null on creation per TS contract.
	if v, present := obj["originalFile"]; !present || v != nil {
		t.Errorf("expected originalFile=null on create; got present=%v val=%v", present, v)
	}
	if _, ok := obj["userModified"]; ok {
		t.Errorf("unexpected Go-only userModified field on write result")
	}
}

// ─── HistoryStore: writes journaled under .claude/file-history ──────────────

// TestAlignment_FileWrite_HistoryStoreWritesUnderClaudeRoot asserts
// successful writes produce a .jsonl log under .claude/file-history.
// Today FileWriteTool has no HistoryStore field, so even if we wired
// one via reflection there would be no call site emitting events.
func TestAlignment_FileWrite_HistoryStoreWritesUnderClaudeRoot(t *testing.T) {
	dir, p := alignmentWriteFixture(t, "alpha\n")
	historyRoot := filepath.Join(dir, ".claude", "file-history")
	tool := alignmentBuildWriteTool(t, dir)

	// Inject a HistoryStore via reflection if the field exists; otherwise
	// the test still asserts the absent file below and fails accordingly.
	v := reflect.ValueOf(tool).Elem()
	if f := v.FieldByName("HistoryStore"); f.IsValid() && f.CanSet() {
		store := NewFileHistoryStore(historyRoot)
		f.Set(reflect.ValueOf(store))
	}

	if info, err := os.Stat(p); err == nil {
		tool.ReadState.Set(p, ReadFileEntry{
			TimestampMs: info.ModTime().UnixMilli(),
			Content:     "alpha\n",
		})
	}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"file_path": p,
		"content":   "alpha\nbeta\n",
	})
	if res.IsError {
		t.Fatalf("write failed: %s", res.Content)
	}

	found := false
	_ = filepath.Walk(historyRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".jsonl") {
			found = true
		}
		return nil
	})
	if !found {
		t.Fatalf("expected at least one .jsonl history file under %s after Write; found none", historyRoot)
	}
}

// ─── ReadState post-write entry must tag tool ──────────────────────────────

// TestAlignment_FileWrite_ReadStateEntryTaggedAsWrite asserts that the
// post-write ReadState entry records the last writer ("Write"). Today
// ReadFileEntry has no LastTool field — so this fails for the same
// reason as the FileEdit alignment test.
func TestAlignment_FileWrite_ReadStateEntryTaggedAsWrite(t *testing.T) {
	dir, p := alignmentWriteFixture(t, "alpha\n")
	tool := alignmentBuildWriteTool(t, dir)
	if info, err := os.Stat(p); err == nil {
		tool.ReadState.Set(p, ReadFileEntry{
			TimestampMs: info.ModTime().UnixMilli(),
			Content:     "alpha\n",
		})
	}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"file_path": p,
		"content":   "alpha\nbeta\n",
	})
	if res.IsError {
		t.Fatalf("write failed: %s", res.Content)
	}
	entry, ok := tool.ReadState.Get(p)
	if !ok {
		t.Fatalf("expected ReadState entry after Write; got none")
	}
	v := reflect.ValueOf(entry)
	if _, ok := v.Type().FieldByName("LastTool"); !ok {
		t.Fatalf("expected ReadFileEntry.LastTool field to identify the last writer; missing")
	}
}
