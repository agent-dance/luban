package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FileWritePreparationTracker mirrors diagnosticTracker.beforeFileEdited. It
// runs before any directory or file mutation and is intentionally best-effort.
type FileWritePreparationTracker interface {
	BeforeFileEdited(ctx context.Context, absPath string) error
}

// FileWriteVSCodeNotifier receives the complete diff-view payload used by the
// TS notifyVscodeFileUpdated call.
type FileWriteVSCodeNotifier interface {
	NotifyFileUpdated(ctx context.Context, absPath, oldContent, newContent string) error
}

type FileWriteChangeEvent struct {
	FilePath        string
	Before          string
	After           string
	StructuredPatch []DiffHunk
	Diagnostics     []LSPDiagnostic
	GitDiff         *EditGitDiff
}

type FileWriteChangeListener func(FileWriteChangeEvent)

func fileWriteEnvTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (t *FileWriteTool) activateSkillsForWrittenPath(ctx context.Context, absPath string) {
	if t == nil || t.SkillManager == nil || editSimpleMode() {
		return
	}
	dirs := DiscoverSkillDirsForPaths([]string{absPath})
	if len(dirs) > 0 {
		addSkillDirectoriesForExecution(ctx, t.SkillManager, dirs)
	}
	activateConditionalPathForExecution(ctx, t.SkillManager, absPath)
	for _, name := range LookupDynamicSkillsForPath(absPath) {
		activateConditionalNameForExecution(ctx, t.SkillManager, name)
	}
}

func (t *FileWriteTool) beforeFileWrite(ctx context.Context, absPath string) {
	// TS performs both skill operations and diagnostic preparation before the
	// filesystem mutation. Neither is allowed to turn a valid Write into an
	// error.
	func() {
		defer func() { _ = recover() }()
		t.activateSkillsForWrittenPath(ctx, absPath)
	}()
	if t != nil && t.PreparationTracker != nil {
		_ = t.PreparationTracker.BeforeFileEdited(ctx, absPath)
	}
}

func (t *FileWriteTool) historyEnabled() bool {
	if t == nil || t.HistoryStore == nil {
		return false
	}
	if t.HistoryEnabled != nil {
		return t.HistoryEnabled()
	}
	if fileWriteEnvTruthy(os.Getenv("CLAUDE_CODE_DISABLE_FILE_CHECKPOINTING")) {
		return false
	}
	if t.Runtime != nil && !t.runtimeSnapshot().Interactive {
		return fileWriteEnvTruthy(os.Getenv("CLAUDE_CODE_ENABLE_SDK_FILE_CHECKPOINTING"))
	}
	return true
}

func (t *FileWriteTool) trackPreWriteHistory(ctx context.Context, absPath, before, after string) {
	if !t.historyEnabled() {
		return
	}
	correlationID := ""
	if t.HistoryCorrelationID != nil {
		correlationID = t.HistoryCorrelationID(ctx)
	}
	_ = t.HistoryStore.TrackEdit(FileHistoryEntry{
		Path: absPath, Before: before, After: after, Tool: "Write",
		Ts: time.Now().UnixMilli(), EditID: correlationID,
	})
}

func (t *FileWriteTool) remoteDiffEnabled() bool {
	if t == nil {
		return false
	}
	if t.RemoteGitDiffEnabled != nil {
		return t.RemoteGitDiffEnabled()
	}
	return fileWriteEnvTruthy(os.Getenv("CLAUDE_CODE_REMOTE")) && IsRemoteGitDiffEnabled()
}

func (t *FileWriteTool) computeWriteGitDiff(ctx context.Context, absPath string) *EditGitDiff {
	if !t.remoteDiffEnabled() {
		return nil
	}
	provider := t.GitDiffProvider
	if provider == nil {
		provider = defaultEditGitDiffProvider
	}
	started := time.Now()
	diff, err := provider(ctx, absPath)
	t.emitAnalytics("tengu_tool_use_diff_computed", map[string]any{
		"isWriteTool": true,
		"durationMs":  time.Since(started).Milliseconds(),
		"hasDiff":     err == nil && diff != nil,
	})
	if err != nil {
		return nil
	}
	return diff
}

func (t *FileWriteTool) completeWriteLifecycle(ctx context.Context, absPath, before, after string, patch []DiffHunk) ([]LSPDiagnostic, *EditGitDiff) {
	if t.DiagnosticsTracker != nil {
		t.DiagnosticsTracker.ClearForFile(absPath)
	}
	if syncer, ok := t.LSP.(LSPDocumentSync); ok && syncer != nil {
		_ = syncer.DidChange(ctx, absPath, after)
		_ = syncer.DidSave(ctx, absPath, after)
	}
	if t.VSCodeNotifier != nil {
		_ = t.VSCodeNotifier.NotifyFileUpdated(ctx, absPath, before, after)
	}

	diagnostics := []LSPDiagnostic{}
	if t.LSP != nil {
		if values, err := t.LSP.Diagnose(ctx, absPath, after); err == nil {
			if len(values) > 20 {
				values = values[:20]
			}
			if values != nil {
				diagnostics = values
			}
		}
	}
	if t.DiagnosticsTracker != nil {
		diagnostics = t.DiagnosticsTracker.FilterUndelivered(absPath, diagnostics)
		t.DiagnosticsTracker.MarkDelivered(absPath, diagnostics)
	}

	if filepath.Base(absPath) == "CLAUDE.md" {
		t.emitAnalytics("tengu_write_claudemd", map[string]any{})
		InvalidateMemoryFileMtime(absPath)
	}
	gitDiff := t.computeWriteGitDiff(ctx, absPath)
	if t.ChangeListener != nil {
		func() {
			defer func() { _ = recover() }()
			t.ChangeListener(FileWriteChangeEvent{
				FilePath: absPath, Before: before, After: after,
				StructuredPatch: append([]DiffHunk(nil), patch...),
				Diagnostics:     append([]LSPDiagnostic(nil), diagnostics...), GitDiff: gitDiff,
			})
		}()
	}
	return diagnostics, gitDiff
}
