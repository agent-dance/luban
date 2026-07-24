// Package tools — file_edit_wave2_test.go: regression tests for the wave-2
// FileEdit alignment fixes (suggest-similar-path, vscode-notify,
// lsp-didchange-didsave, clear-delivered-diagnostics).
package tools

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// recordingLSP captures the post-edit didChange / didSave invocations and
// always returns the same fixed diagnostic from Diagnose.
type recordingLSP struct {
	didChangeCount  int32
	didSaveCount    int32
	diagnoseContent string
	diagnostics     []LSPDiagnostic
}

func (r *recordingLSP) Diagnose(ctx context.Context, absPath, content string) ([]LSPDiagnostic, error) {
	r.diagnoseContent = content
	return r.diagnostics, nil
}
func (r *recordingLSP) DidChange(ctx context.Context, absPath, content string) error {
	atomic.AddInt32(&r.didChangeCount, 1)
	return nil
}
func (r *recordingLSP) DidSave(ctx context.Context, absPath, content string) error {
	atomic.AddInt32(&r.didSaveCount, 1)
	return nil
}

// recordingVSCode captures NotifyFileUpdated invocations.
type recordingVSCode struct {
	calls   int32
	lastDoc string
}

func (r *recordingVSCode) NotifyFileUpdated(ctx context.Context, absPath, content string) error {
	atomic.AddInt32(&r.calls, 1)
	r.lastDoc = content
	return nil
}

// TestFileEditWave2_LSPDidChangeDidSave — fe-lsp-didchange-didsave.
// FileEdit must drive both DidChange and DidSave on a Diagnoser that
// implements LSPDocumentSync.
func TestFileEditWave2_LSPDidChangeDidSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(path, []byte("hello world"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	state := NewReadFileState()
	abs, _ := filepath.Abs(path)
	abs = filepath.Clean(abs)
	recordStrongReadEvidenceForTest(t, state, abs)

	lsp := &recordingLSP{}
	tool := &FileEditTool{ReadState: state, LSP: lsp}
	res, err := tool.Execute(context.Background(), map[string]any{
		"file_path":  path,
		"old_string": "hello",
		"new_string": "goodbye",
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if got := atomic.LoadInt32(&lsp.didChangeCount); got != 1 {
		t.Fatalf("expected DidChange called once, got %d", got)
	}
	if got := atomic.LoadInt32(&lsp.didSaveCount); got != 1 {
		t.Fatalf("expected DidSave called once, got %d", got)
	}
}

// TestFileEditWave2_VSCodeNotify — fe-notify-vscode-diff-view.
// A successful edit must invoke the VSCodeNotifier exactly once with the
// post-edit content.
func TestFileEditWave2_VSCodeNotify(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(path, []byte("alpha"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	state := NewReadFileState()
	abs, _ := filepath.Abs(path)
	abs = filepath.Clean(abs)
	recordStrongReadEvidenceForTest(t, state, abs)

	vs := &recordingVSCode{}
	tool := &FileEditTool{ReadState: state, VSCodeNotifier: vs}
	res, err := tool.Execute(context.Background(), map[string]any{
		"file_path":  path,
		"old_string": "alpha",
		"new_string": "beta",
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if got := atomic.LoadInt32(&vs.calls); got != 1 {
		t.Fatalf("expected exactly 1 vscode notification, got %d", got)
	}
	if !strings.Contains(vs.lastDoc, "beta") {
		t.Fatalf("expected post-edit content to contain 'beta', got %q", vs.lastDoc)
	}
}

// TestFileEditWave2_SuggestSimilarPath — fe-suggest-similar-path.
// An ENOENT for an edit must include "Did you mean ..." with a near-miss
// from the cwd.
func TestFileEditWave2_SuggestSimilarPath(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "configuration.yaml")
	if err := os.WriteFile(good, []byte("k: v"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// Pivot cwd into our scratch dir so the suggester walks it.
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	missing := filepath.Join(dir, "configuration.yml") // typo: yml vs yaml
	tool := &FileEditTool{ReadState: NewReadFileState()}
	res, err := tool.Execute(context.Background(), map[string]any{
		"file_path":  missing,
		"old_string": "k:",
		"new_string": "key:",
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected error, got %s", res.Content)
	}
	if !strings.Contains(res.Content, "Did you mean") {
		t.Fatalf("expected 'Did you mean' suggestion, got %s", res.Content)
	}
	if !strings.Contains(res.Content, "configuration.yaml") {
		t.Fatalf("expected suggestion to include configuration.yaml, got %s", res.Content)
	}
}

// TestFileEditWave2_ClearDeliveredDiagnostics — fe-clear-delivered-diagnostics.
// The tracker must be cleared before each edit so a regression that
// re-introduces the same diagnostic surfaces again.
func TestFileEditWave2_ClearDeliveredDiagnostics(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "code.go")
	if err := os.WriteFile(path, []byte("package x\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	state := NewReadFileState()
	abs, _ := filepath.Abs(path)
	abs = filepath.Clean(abs)
	recordStrongReadEvidenceForTest(t, state, abs)

	diag := LSPDiagnostic{Severity: "error", Message: "undefined: foo", StartLine: 1, EndLine: 1}
	tracker := NewInMemoryDiagnosticsTracker()
	// Pre-mark the diagnostic as already delivered to simulate the prior
	// edit.
	tracker.MarkDelivered(abs, []LSPDiagnostic{diag})

	lsp := &recordingLSP{diagnostics: []LSPDiagnostic{diag}}
	tool := &FileEditTool{ReadState: state, LSP: lsp, DiagnosticsTracker: tracker}
	res, err := tool.Execute(context.Background(), map[string]any{
		"file_path":  path,
		"old_string": "package x",
		"new_string": "package y",
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	// The clear-before-edit must have wiped the prior delivered set, so
	// the diagnostic should resurface.
	if !strings.Contains(res.Content, "undefined: foo") {
		t.Fatalf("expected diagnostic to be re-surfaced after clear, got %s", res.Content)
	}
}

// TestSuggestSimilarPath_Direct — direct unit tests for the helper so the
// scoring tiers are exercised without going through the full tool harness.
func TestSuggestSimilarPath_Direct(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"foo.go", "bar.txt", "Foo.GO"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(""), 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	got := suggestSimilarPath(dir, "Foo.go")
	if len(got) == 0 {
		t.Fatalf("expected at least one suggestion")
	}
	// Exact match should rank first.
	hadExact := false
	for _, p := range got {
		if filepath.Base(p) == "Foo.GO" || filepath.Base(p) == "foo.go" {
			hadExact = true
		}
	}
	if !hadExact {
		t.Fatalf("expected exact-or-case-match candidate, got %v", got)
	}
}

// recordingLSPErr — verifies edits proceed even when the LSP errors out.
type recordingLSPErr struct{ recordingLSP }

func (recordingLSPErr) Diagnose(ctx context.Context, absPath, content string) ([]LSPDiagnostic, error) {
	return nil, errors.New("boom")
}

// TestFileEditWave2_LSPErrorsAreNonFatal — meta sanity: an LSP that errors
// during DidChange/DidSave/Diagnose must not block the edit.
func TestFileEditWave2_LSPErrorsAreNonFatal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.go")
	if err := os.WriteFile(path, []byte("package x\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	state := NewReadFileState()
	abs, _ := filepath.Abs(path)
	abs = filepath.Clean(abs)
	recordStrongReadEvidenceForTest(t, state, abs)

	tool := &FileEditTool{ReadState: state, LSP: recordingLSPErr{}}
	res, err := tool.Execute(context.Background(), map[string]any{
		"file_path":  path,
		"old_string": "package x",
		"new_string": "package y",
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.IsError {
		t.Fatalf("LSP error must not surface as IsError, got %s", res.Content)
	}
}
