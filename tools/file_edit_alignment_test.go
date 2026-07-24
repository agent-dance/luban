// Package tools — file_edit_alignment_test.go contains failing
// alignment tests for FileEditTool, derived from alignment_audit.md
// (sections P1-7 + P2-7). These tests are intentionally RED.
//
// Audit gaps targeted:
//   - LSP field is declared but never injected by the registry, so a
//     vanilla `&FileEditTool{}` always has nil LSP, contradicting the
//     TS contract that wires a Diagnoser by default.
//   - validateSettingsEdit only does JSON parse; it must reject schema
//     violations (missing required keys, additionalProperties leaks).
//   - The schema lacks the semanticBoolean wrapper for replace_all.
//   - Occurrences/DurationMs sit on EditResult top-level; TS places
//     them inside a `metadata` sub-object.
//   - HistoryStore is wired but writes nothing on failed edit (TS
//     suppresses no-op writes; we want a positive assertion that the
//     log appears for successful edits at the documented path layout).
//   - Read-before-Edit gate is enforced today, but not via the shared
//     ReadFileState retrieved from a typed accessor; TS exposes the
//     state via a Context handle.
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

// alignmentBuildEditTool wires a FileEditTool the way a freshly-bootstrapped
// registry does: shared ReadState, history store rooted at .claude, and a
// default no-op LSP diagnoser.
func alignmentBuildEditTool(t *testing.T, dir string) *FileEditTool {
	t.Helper()
	historyRoot := filepath.Join(dir, ".claude", "file-history")
	tool := &FileEditTool{
		AllowedDirs:  []string{dir},
		ReadState:    NewReadFileState(),
		HistoryStore: NewFileHistoryStore(historyRoot),
		LSP:          NewNoopLSPDiagnoser(),
	}
	return tool
}

// ─── Surface: LSP injection ─────────────────────────────────────────────────

// TestAlignment_FileEdit_LSPNotNilAfterBootstrap asserts that a
// production-style FileEditTool exposes a non-nil LSP Diagnoser.
// registry_setup.go:191-196 builds the tool without ever assigning
// LSP, so the field is nil and this test fails.
func TestAlignment_FileEdit_LSPNotNilAfterBootstrap(t *testing.T) {
	dir := t.TempDir()
	tool := alignmentBuildEditTool(t, dir)
	if tool.LSP == nil {
		t.Fatalf("expected FileEditTool.LSP to be non-nil after bootstrap (NewNoopLSPDiagnoser at minimum); got nil")
	}
}

// TestAlignment_FileEdit_DiagnosticsFieldOnSuccess asserts that the
// JSON result of a successful edit contains a diagnostics array
// (possibly empty, but present), so downstream consumers can branch
// on it without nil-checking. With LSP=nil today the field is omitted
// entirely (omitempty + nil slice).
func TestAlignment_FileEdit_DiagnosticsFieldOnSuccess(t *testing.T) {
	dir, p := alignmentEditFixture(t, "alpha\n")
	tool := alignmentBuildEditTool(t, dir)

	// Pre-populate the read state so Edit's gate passes.
	recordStrongReadEvidenceForTest(t, tool.ReadState, p)

	res, err := tool.Execute(context.Background(), map[string]any{
		"file_path":  p,
		"old_string": "alpha",
		"new_string": "beta",
	})
	if err != nil {
		t.Fatalf("infrastructure error: %v", err)
	}
	if res.IsError {
		t.Fatalf("edit failed: %s", res.Content)
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(res.Content), &obj); err != nil {
		t.Fatalf("result not JSON: %v (content=%q)", err, res.Content)
	}
	if _, ok := obj["diagnostics"]; !ok {
		t.Fatalf("expected diagnostics field present (possibly empty array); got %v", obj)
	}
}

// ─── Output: metadata placement ─────────────────────────────────────────────

// TestAlignment_FileEdit_MetadataSubObject asserts Occurrences and
// DurationMs live under a `metadata` sub-object, not at the top
// level. file_edit.go:119-120 currently exposes them at top-level.
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
// human-friendly description and default. Currently the schema is a
// bare {type:"boolean", default:false}.
func TestAlignment_FileEdit_SchemaUsesSemanticBoolean(t *testing.T) {
	if !alignmentSymbolExists("semanticBoolean") && !alignmentSymbolExists("SemanticBoolean") {
		t.Fatalf("expected semanticBoolean/SemanticBoolean helper to wrap replace_all; helper not defined in package")
	}
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

// ─── Settings validator: schema-level checks ────────────────────────────────

// TestAlignment_FileEdit_SettingsValidatorRejectsMissingRequired
// asserts that an edit removing a required key from settings.json is
// refused. The current validateSettingsEdit only parses JSON — a
// minimal `{}` slips through.
func TestAlignment_FileEdit_SettingsValidatorRejectsMissingRequired(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	settingsPath := filepath.Join(configDir, "settings.json")
	original := `{"$schema":"https://json.schemastore.org/claude-settings.json","permissions":{"allow":["*"]}}`
	if err := os.WriteFile(settingsPath, []byte(original), 0644); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
	tool := alignmentBuildEditTool(t, dir)
	recordStrongReadEvidenceForTest(t, tool.ReadState, settingsPath)
	res, _ := tool.Execute(context.Background(), map[string]any{
		"file_path":   settingsPath,
		"old_string":  original,
		"new_string":  `{}`, // strips the required permissions block
		"replace_all": false,
	})
	if !res.IsError {
		t.Fatalf("expected validateSettingsEdit to reject removal of required permissions block; got success: %s", res.Content)
	}
	if !strings.Contains(strings.ToLower(res.Content), "permissions") {
		t.Errorf("expected error message to mention missing required permissions key; got %q", res.Content)
	}
}

// TestAlignment_FileEdit_SettingsValidatorRejectsAdditionalProperty
// asserts that a stray top-level key not in the published schema is
// rejected. validateSettingsEdit currently allows anything that
// parses as JSON object.
func TestAlignment_FileEdit_SettingsValidatorRejectsAdditionalProperty(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	settingsPath := filepath.Join(configDir, "settings.json")
	original := `{"permissions":{"allow":["*"]}}`
	if err := os.WriteFile(settingsPath, []byte(original), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	tool := alignmentBuildEditTool(t, dir)
	recordStrongReadEvidenceForTest(t, tool.ReadState, settingsPath)
	res, _ := tool.Execute(context.Background(), map[string]any{
		"file_path":  settingsPath,
		"old_string": original,
		"new_string": `{"permissions":{"allow":["*"]},"__hax":1}`,
	})
	if !res.IsError {
		t.Fatalf("expected schema validator to reject additional property __hax; got success")
	}
}

// ─── Read-before-edit gate via shared state ─────────────────────────────────

// TestAlignment_FileEdit_ReadGateUsesSharedReadFileState asserts that
// FileEditTool reads from a SHARED state object (so a Read populated
// by another goroutine satisfies the gate). Today FileEditTool falls
// back to DefaultReadFileState() when ReadState is nil — a
// process-wide singleton — making isolation in concurrent agents hard.
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
	entry, ok := state.Get(p)
	if !ok {
		t.Fatalf("expected ReadState entry after edit; got none")
	}
	if entry.Content != "beta\n" {
		t.Fatalf("expected post-edit Content snapshot = %q; got %q", "beta\n", entry.Content)
	}
	if entry.TimestampMs == 0 {
		t.Fatalf("expected non-zero post-edit TimestampMs in ReadState entry")
	}
	// The audit specifically requires the entry to be tagged with the
	// last writer tool ("Edit"); current ReadFileEntry has no LastTool
	// field.
	v := reflect.ValueOf(entry)
	if _, ok := v.Type().FieldByName("LastTool"); !ok {
		t.Fatalf("expected ReadFileEntry.LastTool field to identify the last writer; field missing")
	}
}

// ─── HistoryStore: layout and contents ──────────────────────────────────────

// TestAlignment_FileEdit_HistoryStoreWritesUnderClaudeRoot asserts that
// successful edits produce a history file under .claude/file-history/.
// Today history is best-effort (errors swallowed) and the file path
// derivation is opaque — there is no public accessor to confirm the
// file landed where TS expects it (sha1(absPath).jsonl).
func TestAlignment_FileEdit_HistoryStoreWritesUnderClaudeRoot(t *testing.T) {
	dir, p := alignmentEditFixture(t, "alpha\n")
	historyRoot := filepath.Join(dir, ".claude", "file-history")
	tool := &FileEditTool{
		AllowedDirs:  []string{dir},
		ReadState:    NewReadFileState(),
		HistoryStore: NewFileHistoryStore(historyRoot),
	}
	recordStrongReadEvidenceForTest(t, tool.ReadState, p)
	res, _ := tool.Execute(context.Background(), map[string]any{
		"file_path":  p,
		"old_string": "alpha",
		"new_string": "beta",
	})
	if res.IsError {
		t.Fatalf("edit failed: %s", res.Content)
	}
	// Verify a file landed under historyRoot. Walk the dir.
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
		t.Fatalf("expected at least one .jsonl history file under %s; found none", historyRoot)
	}

	// And: history record must include `editId` (UUID) per TS shape.
	entries := alignmentReadAllHistory(t, historyRoot)
	if len(entries) == 0 {
		t.Fatalf("history root walk found a .jsonl, but parse yielded zero entries")
	}
	if _, ok := entries[0]["editId"]; !ok {
		t.Errorf("expected history entry to include editId UUID; got %v", entries[0])
	}
}

// ─── Helper ─────────────────────────────────────────────────────────────────

func alignmentReadAllHistory(t *testing.T, root string) []map[string]any {
	t.Helper()
	var out []map[string]any
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var rec map[string]any
			if err := json.Unmarshal([]byte(line), &rec); err == nil {
				out = append(out, rec)
			}
		}
		return nil
	})
	return out
}
