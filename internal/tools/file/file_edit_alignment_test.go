// Package file contains FileEditTool contract and behavior tests.
package file

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// alignmentEditFixture writes a small file under temp dir, returns
// (dir, absolute path).
func alignmentEditFixture(t *testing.T, body string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "subj.txt")
	if err := os.WriteFile(p, []byte(body), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return dir, p
}

// alignmentBuildEditTool wires a FileEditTool with isolated read state.
func alignmentBuildEditTool(t *testing.T, dir string) *FileEditTool {
	t.Helper()
	tool := &FileEditTool{
		AllowedDirs: []string{dir},
		ReadState:   NewReadFileState(),
	}
	return tool
}

// ─── Output: metadata placement ─────────────────────────────────────────────

// TestAlignment_FileEdit_MetadataSubObject asserts Occurrences and
// DurationMs live under a `metadata` sub-object, not at the top
// level so the structured data contract remains stable.
func TestAlignment_FileEdit_MetadataSubObject(t *testing.T) {
	dir, p := alignmentEditFixture(t, "alpha\n")
	tool := alignmentBuildEditTool(t, dir)
	recordStrongReadEvidenceForTest(t, tool.ReadState, p)
	res, _ := tool.Execute(context.Background(), map[string]any{
		"file_path": p, "old_string": "alpha", "new_string": "beta",
	})
	if res.IsError {
		t.Fatalf("edit failed: %s", res.Content)
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(res.Content), &obj); err != nil {
		t.Fatalf("result not JSON: %v", err)
	}
	if _, top := obj["occurrences"]; top {
		t.Errorf("occurrences must NOT be top-level; expected under metadata.occurrences")
	}
	if _, top := obj["durationMs"]; top {
		t.Errorf("durationMs must NOT be top-level; expected under metadata.durationMs")
	}
	meta, ok := obj["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("expected metadata sub-object; got obj=%v", obj)
	}
	if _, ok := meta["occurrences"]; !ok {
		t.Errorf("expected metadata.occurrences; got %v", meta)
	}
	if _, ok := meta["durationMs"]; !ok {
		t.Errorf("expected metadata.durationMs; got %v", meta)
	}
}

// ─── Surface: semanticBoolean for replace_all ───────────────────────────────

// TestAlignment_FileEdit_SchemaUsesSemanticBoolean asserts that
// replace_all is described via a semanticBoolean helper carrying a
// human-friendly description and default.
func TestAlignment_FileEdit_SchemaUsesSemanticBoolean(t *testing.T) {
	tool := &FileEditTool{}
	schema := tool.Schema()
	prop, ok := schema.Properties["replace_all"].(map[string]any)
	if !ok {
		t.Fatalf("replace_all property missing")
	}
	// semanticBoolean adds an `_semantic` marker to distinguish from raw boolean schemas.
	if prop["_semantic"] == nil {
		t.Fatalf("expected replace_all._semantic marker (semanticBoolean wrapper); got %#v", prop)
	}
}

// ─── Read-before-edit gate via shared state ─────────────────────────────────

// TestAlignment_FileEdit_ReadGateUsesSharedReadFileState asserts that
// FileEditTool reads from explicitly shared state so a Read populated
// by another goroutine satisfies the same scoped gate.
func TestAlignment_FileEdit_ReadGateUsesSharedReadFileState(t *testing.T) {
	dir, p := alignmentEditFixture(t, "alpha\n")
	state := NewReadFileState()
	tool := &FileEditTool{
		AllowedDirs: []string{dir},
		ReadState:   state,
	}
	// No prior read recorded.
	res, _ := tool.Execute(context.Background(), map[string]any{
		"file_path":  p,
		"old_string": "alpha",
		"new_string": "beta",
	})
	if !res.IsError {
		t.Fatalf("expected Read-before-Edit gate to reject; got success")
	}
	// Now register the read in the SHARED state and retry.
	recordStrongReadEvidenceForTest(t, state, p)
	res, _ = tool.Execute(context.Background(), map[string]any{
		"file_path":  p,
		"old_string": "alpha",
		"new_string": "beta",
	})
	if res.IsError {
		t.Fatalf("expected edit to succeed after shared state seeded; got error: %s", res.Content)
	}
	// Final assertion: the post-edit ReadState entry must include the
	// updated content snapshot AND a non-zero post-write mtime.
	entry, ok := state.GetForContext(context.Background(), p)
	if !ok {
		t.Fatalf("expected ReadState entry after edit; got none")
	}
	if entry.Content != "beta\n" {
		t.Fatalf("expected post-edit Content snapshot = %q; got %q", "beta\n", entry.Content)
	}
	if entry.TimestampMs == 0 {
		t.Fatalf("expected non-zero post-edit TimestampMs in ReadState entry")
	}
	// Tag the entry with the last writer tool so downstream consumers can
	// distinguish read and write origins.
	v := reflect.ValueOf(entry)
	if _, ok := v.Type().FieldByName("LastTool"); !ok {
		t.Fatalf("expected ReadFileEntry.LastTool field to identify the last writer; field missing")
	}
}
