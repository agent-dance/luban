package tools

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

// fakeLSPSync implements both LSPDiagnoser and LSPDocumentSync so we can
// observe the call sequence on a successful write.
type fakeLSPSync struct {
	didChangeCount int32
	didSaveCount   int32
	diagnoseCount  int32
	diagnostics    []LSPDiagnostic
}

func (f *fakeLSPSync) Diagnose(ctx context.Context, absPath, content string) ([]LSPDiagnostic, error) {
	atomic.AddInt32(&f.diagnoseCount, 1)
	return f.diagnostics, nil
}
func (f *fakeLSPSync) DidChange(ctx context.Context, absPath, content string) error {
	atomic.AddInt32(&f.didChangeCount, 1)
	return nil
}
func (f *fakeLSPSync) DidSave(ctx context.Context, absPath, content string) error {
	atomic.AddInt32(&f.didSaveCount, 1)
	return nil
}

type fakeDiagTracker struct {
	cleared   int32
	filtered  int32
	delivered int32
}

type fakeWriteVSCodeNotifier struct {
	count      *atomic.Int32
	path       *string
	oldContent *string
	newContent *string
}

func (f fakeWriteVSCodeNotifier) NotifyFileUpdated(_ context.Context, path, oldContent, newContent string) error {
	f.count.Add(1)
	*f.path = path
	*f.oldContent = oldContent
	*f.newContent = newContent
	return nil
}

func (f *fakeDiagTracker) ClearForFile(p string) { atomic.AddInt32(&f.cleared, 1) }
func (f *fakeDiagTracker) FilterUndelivered(p string, d []LSPDiagnostic) []LSPDiagnostic {
	atomic.AddInt32(&f.filtered, 1)
	return d
}
func (f *fakeDiagTracker) MarkDelivered(p string, d []LSPDiagnostic) {
	atomic.AddInt32(&f.delivered, 1)
}

func TestFileWrite_LSPLifecycle_OnNewFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "fresh.go")
	lsp := &fakeLSPSync{}
	tracker := &fakeDiagTracker{}

	tool := &FileWriteTool{
		AllowedDirs:        []string{dir},
		ReadState:          NewReadFileState(),
		LSP:                lsp,
		DiagnosticsTracker: tracker,
	}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"file_path": target,
		"content":   "package fresh\n",
	})
	if res.IsError {
		t.Fatalf("write failed: %v", res.Content)
	}
	if atomic.LoadInt32(&lsp.didChangeCount) != 1 {
		t.Errorf("expected DidChange called once, got %d", lsp.didChangeCount)
	}
	if atomic.LoadInt32(&lsp.didSaveCount) != 1 {
		t.Errorf("expected DidSave called once, got %d", lsp.didSaveCount)
	}
	if atomic.LoadInt32(&lsp.diagnoseCount) != 1 {
		t.Errorf("expected Diagnose called once, got %d", lsp.diagnoseCount)
	}
	if atomic.LoadInt32(&tracker.cleared) != 1 {
		t.Errorf("expected tracker.ClearForFile called once, got %d", tracker.cleared)
	}
}

func TestFileWrite_VSCodeNotifier_FiresOnSuccess(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "ide.go")
	var notified atomic.Int32
	var notifiedPath string
	var oldContent string
	var newContent string
	tool := &FileWriteTool{
		AllowedDirs: []string{dir},
		ReadState:   NewReadFileState(),
		VSCodeNotifier: fakeWriteVSCodeNotifier{
			count: &notified, path: &notifiedPath,
			oldContent: &oldContent, newContent: &newContent,
		},
	}
	res, _ := tool.Execute(context.Background(), map[string]any{
		"file_path": target,
		"content":   "package x\n",
	})
	if res.IsError {
		t.Fatalf("write failed: %v", res.Content)
	}
	if notified.Load() != 1 {
		t.Errorf("expected VSCodeNotifier called once, got %d", notified.Load())
	}
	abs, _ := filepath.Abs(target)
	if notifiedPath != filepath.Clean(abs) {
		t.Errorf("expected notifier path %q, got %q", abs, notifiedPath)
	}
	if oldContent != "" || newContent != "package x\n" {
		t.Errorf("expected old/new content payload, got %q -> %q", oldContent, newContent)
	}
	_ = os.Remove(target)
}
