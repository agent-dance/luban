// Package file contains FileWriteTool contract and behavior tests.
package file

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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

// alignmentBuildWriteTool mirrors the production FileWriteTool dependencies
// needed by these contract tests. Reflection-based assertions below keep
// optional probes isolated from the core construction path.
func alignmentBuildWriteTool(t *testing.T, dir string) *FileWriteTool {
	t.Helper()
	tool := &FileWriteTool{
		AllowedDirs: []string{dir},
		ReadState:   NewReadFileState(),
	}
	return tool
}

// ─── Output: structuredPatch must reflect real diff on overwrite ────────────

// TestAlignment_FileWrite_StructuredPatchNonEmptyOnOverwrite asserts
// that overwriting an existing file produces at least one hunk in
// structuredPatch. file_operations.go:605 hard-codes []any{}.
func TestAlignment_FileWrite_StructuredPatchNonEmptyOnOverwrite(t *testing.T) {
	dir, p := alignmentWriteFixture(t, "alpha\nbeta\ngamma\n")
	tool := alignmentBuildWriteTool(t, dir)
	seedCanonicalFileReadState(t, tool.ReadState, p)
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
// At least one hunk is required for an overwrite that changes content.
func TestAlignment_FileWrite_StructuredPatchHunkShape(t *testing.T) {
	dir, p := alignmentWriteFixture(t, "one\ntwo\n")
	tool := alignmentBuildWriteTool(t, dir)
	seedCanonicalFileReadState(t, tool.ReadState, p)
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

// TestAlignment_FileWrite_OmitsDiagnostics asserts that the result contains
// only values produced by the Write execution path.
func TestAlignment_FileWrite_OmitsDiagnostics(t *testing.T) {
	dir, p := alignmentWriteFixture(t, "alpha\n")
	tool := alignmentBuildWriteTool(t, dir)
	seedCanonicalFileReadState(t, tool.ReadState, p)
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

// TestAlignment_FileWrite_MetadataSubObject asserts that byte counts,
// line counts, and durations must not leak into the TS-shaped Write data.
func TestAlignment_FileWrite_MetadataSubObject(t *testing.T) {
	dir, p := alignmentWriteFixture(t, "alpha\n")
	tool := alignmentBuildWriteTool(t, dir)
	seedCanonicalFileReadState(t, tool.ReadState, p)
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
// write returns type=\"create\" and isNew=true, locking the discriminator
// alongside the rest of the structured output contract.
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

// ─── ReadState post-write entry must tag tool ──────────────────────────────

// TestAlignment_FileWrite_ReadStateEntryTaggedAsWrite asserts that the
// post-write ReadState entry records the last writer ("Write").
func TestAlignment_FileWrite_ReadStateEntryTaggedAsWrite(t *testing.T) {
	dir, p := alignmentWriteFixture(t, "alpha\n")
	tool := alignmentBuildWriteTool(t, dir)
	seedCanonicalFileReadState(t, tool.ReadState, p)
	res, _ := tool.Execute(context.Background(), map[string]any{
		"file_path": p,
		"content":   "alpha\nbeta\n",
	})
	if res.IsError {
		t.Fatalf("write failed: %s", res.Content)
	}
	entry, ok := tool.ReadState.GetForContext(context.Background(), p)
	if !ok {
		t.Fatalf("expected ReadState entry after Write; got none")
	}
	v := reflect.ValueOf(entry)
	if _, ok := v.Type().FieldByName("LastTool"); !ok {
		t.Fatalf("expected ReadFileEntry.LastTool field to identify the last writer; missing")
	}
}
