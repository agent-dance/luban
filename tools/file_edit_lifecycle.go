package tools

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// EditChangeEvent is emitted after a successful atomic write and contains the
// same changed-file payload used to build the structured tool result.
type EditChangeEvent struct {
	FilePath        string
	Before          string
	After           string
	StructuredPatch []DiffHunk
	Occurrences     int
	ReplaceAll      bool
	GitDiff         *EditGitDiff
}

// EditChangeListener receives successful Edit changes. Listener failures are
// isolated by the caller; the file has already been committed at this point.
type EditChangeListener func(EditChangeEvent)

type editPathLock struct {
	mu   sync.Mutex
	refs int
}

var editPathLocks = struct {
	sync.Mutex
	locks map[string]*editPathLock
}{locks: make(map[string]*editPathLock)}

// lockFileEdit serializes the complete read-check-write transaction for one
// canonical path. This recreates the no-await critical section used by TS in a
// Go runtime where multiple tool calls can execute concurrently.
func lockFileEdit(path string) func() {
	return lockFileEditWithRegisteredHook(path, nil)
}

func lockFileEditWithRegisteredHook(path string, registered func()) func() {
	key := canonicalFileEditLockPath(path)
	editPathLocks.Lock()
	lock := editPathLocks.locks[key]
	if lock == nil {
		lock = &editPathLock{}
		editPathLocks.locks[key] = lock
	}
	lock.refs++
	editPathLocks.Unlock()
	if registered != nil {
		registered()
	}

	lock.mu.Lock()
	return func() {
		lock.mu.Unlock()
		editPathLocks.Lock()
		lock.refs--
		if lock.refs == 0 {
			delete(editPathLocks.locks, key)
		}
		editPathLocks.Unlock()
	}
}

// canonicalFileEditLockPath maps every cooperating file mutator onto the same
// absolute lock key. The filesystem operation may retain the caller's display
// spelling, but relative and absolute spellings of the same target must never
// enter independent mutation transactions.
func canonicalFileEditLockPath(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return canonicalPathForComparison(path)
}

// lockFileEdits acquires a set of path locks in deterministic order. Duplicate
// spellings collapse after canonicalization, which both prevents self-deadlock
// for source==destination and prevents the A->B / B->A move cycle.
func lockFileEdits(paths ...string) func() {
	return lockFileEditsWithRegisteredHook(nil, paths...)
}

func lockFileEditsWithRegisteredHook(registered func(), paths ...string) func() {
	keys := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		key := canonicalFileEditLockPath(path)
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	unlocks := make([]func(), 0, len(keys))
	for index, key := range keys {
		var hook func()
		if index == 0 {
			hook = registered
		}
		unlocks = append(unlocks, lockFileEditWithRegisteredHook(key, hook))
	}
	return func() {
		for index := len(unlocks) - 1; index >= 0; index-- {
			unlocks[index]()
		}
	}
}

func editSimpleMode() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("CLAUDE_CODE_SIMPLE"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (t *FileEditTool) activateSkillsForEditedPath(ctx context.Context, absPath string) {
	if t == nil || t.SkillManager == nil || editSimpleMode() {
		return
	}
	dirs := DiscoverSkillDirsForPaths([]string{absPath})
	conditional := LookupDynamicSkillsForPath(absPath)
	if len(dirs) == 0 && len(conditional) == 0 {
		return
	}
	if len(dirs) > 0 {
		addSkillDirectoriesForExecution(ctx, t.SkillManager, dirs)
	}
	activateConditionalPathForExecution(ctx, t.SkillManager, absPath)
}

func (t *FileEditTool) emitEditAnalytics(event string, payload map[string]any) {
	if t != nil && t.AnalyticsHook != nil {
		t.AnalyticsHook(event, payload)
	}
}

func (t *FileEditTool) computeEditGitDiff(ctx context.Context, absPath string) *EditGitDiff {
	if t == nil {
		return nil
	}
	provider := t.GitDiffProvider
	if provider == nil {
		if !IsRemoteGitDiffEnabled() {
			return nil
		}
		provider = defaultEditGitDiffProvider
	}
	started := time.Now()
	diff, err := provider(ctx, absPath)
	t.emitEditAnalytics("tengu_tool_use_diff_computed", map[string]any{
		"isEditTool": true,
		"durationMs": time.Since(started).Milliseconds(),
		"hasDiff":    diff != nil,
		"error":      formatEditGitDiffError(err),
	})
	if err != nil {
		return nil
	}
	return diff
}

type editTargetSnapshot struct {
	Raw           []byte
	ContentDigest string
	Info          os.FileInfo
}

var errEditSnapshotCASMismatch = errors.New("edit target snapshot compare-and-swap mismatch")

func readEditTarget(absPath string, expected os.FileInfo) (editTargetSnapshot, error) {
	file, err := os.Open(absPath)
	if err != nil {
		return editTargetSnapshot{}, err
	}
	defer file.Close()

	opened, err := file.Stat()
	if err != nil {
		return editTargetSnapshot{}, err
	}
	if expected != nil && !os.SameFile(expected, opened) {
		return editTargetSnapshot{}, i18n.NewError(i18n.KeyToolFileHelperEditTargetReplacedBefore)
	}
	content, err := io.ReadAll(file)
	if err != nil {
		return editTargetSnapshot{}, err
	}
	afterRead, err := file.Stat()
	if err != nil {
		return editTargetSnapshot{}, err
	}
	if opened.Size() != afterRead.Size() || !opened.ModTime().Equal(afterRead.ModTime()) {
		return editTargetSnapshot{}, i18n.NewError(i18n.KeyToolFileHelperEditTargetChangedWhileRead)
	}
	currentPath, err := os.Lstat(absPath)
	if err != nil || currentPath.Mode()&os.ModeSymlink != 0 || !os.SameFile(opened, currentPath) {
		return editTargetSnapshot{}, i18n.NewError(i18n.KeyToolFileHelperEditTargetReplacedBefore)
	}
	return editTargetSnapshot{Raw: content, ContentDigest: fileContentDigest(content), Info: afterRead}, nil
}

type editCompletion struct {
	AbsPath            string
	DisplayPath        string
	Before             string
	After              string
	OldString          string
	AnalyticsOldString string
	NewString          string
	Patch              []DiffHunk
	Occurrences        int
	ReplaceAll         bool
	StartedAt          time.Time
	Encoding           FileEncoding
	BOM                []byte
	SettingsWarning    string
	ContentDigest      string
}

func (t *FileEditTool) recheckEditTarget(absPath string, original os.FileInfo, expectedDigest string) error {
	if err := checkAllowedPath(absPath, t.AllowedDirs); err != nil {
		return i18n.WrapError(i18n.KeyToolFileHelperEditTargetOutsideAllowed, err)
	}
	current, err := os.Lstat(absPath)
	if err != nil {
		return i18n.WrapError(i18n.KeyToolFileHelperRecheckEditTargetFailed, err)
	}
	if current.Mode()&os.ModeSymlink != 0 {
		return i18n.NewError(i18n.KeyToolFileHelperEditThroughSymlink, absPath)
	}
	if original != nil && !os.SameFile(original, current) {
		return i18n.WrapInternalError(i18n.KeyToolFileHelperEditTargetReplacedAfter, errEditSnapshotCASMismatch)
	}
	snapshot, err := readEditTarget(absPath, original)
	if err != nil {
		return i18n.WrapInternalError(i18n.KeyToolFileHelperEditTargetChangedAfter, errEditSnapshotCASMismatch)
	}
	if expectedDigest == "" || snapshot.ContentDigest != expectedDigest {
		return i18n.WrapInternalError(i18n.KeyToolFileHelperEditTargetChangedAfter, errEditSnapshotCASMismatch)
	}
	return nil
}

// atomicWriteFileWithEditCAS prepares and fsyncs the replacement first, then
// verifies identity+digest immediately before the commit rename. The per-path
// lock serializes cooperating tools and the precommit guard removes the large
// temporary-file I/O window. Generic filesystems do not provide an atomic
// "rename only if content digest still matches" primitive, so an uncooperative
// external writer retains a narrow verify-to-rename race.
func atomicWriteFileWithEditCAS(path string, data []byte, perm os.FileMode, verify func() error) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".atomic-*")
	if err != nil {
		return i18n.WrapError(i18n.KeyToolSourceSinkAtomicCreateTemporary, err)
	}
	tmpPath := tmp.Name()
	success := false
	defer func() {
		if !success {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
		}
	}()
	if err := writeAll(tmp, data); err != nil {
		return i18n.WrapError(i18n.KeyToolSourceSinkAtomicWriteTemporary, err)
	}
	if err := tmp.Chmod(perm); err != nil {
		return i18n.WrapError(i18n.KeyToolSourceSinkAtomicChmodTemporary, err)
	}
	if err := tmp.Sync(); err != nil {
		return i18n.WrapError(i18n.KeyToolSourceSinkAtomicSyncTemporary, err)
	}
	if err := tmp.Close(); err != nil {
		return i18n.WrapError(i18n.KeyToolSourceSinkAtomicCloseTemporary, err)
	}
	if verify != nil {
		if err := verify(); err != nil {
			return err
		}
	}
	if err := replaceFileAtomically(tmpPath, path); err != nil {
		return i18n.WrapError(i18n.KeyToolSourceSinkAtomicReplaceTarget, err)
	}
	success = true
	if err := syncRuntimeDirectory(dir); err != nil && !errors.Is(err, fs.ErrInvalid) {
		return i18n.WrapError(i18n.KeyToolSourceSinkAtomicSyncTemporary, err)
	}
	return nil
}

// atomicCreateFile publishes a fully written temporary inode only if path is
// still absent. The platform commit primitive either installs the new name or
// reports that another writer won, without exposing a partially written
// destination.
func atomicCreateFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".atomic-create-*")
	if err != nil {
		return i18n.WrapError(i18n.KeyToolSourceSinkAtomicCreateTemporary, err)
	}
	tmpPath := tmp.Name()
	closed := false
	defer func() {
		if !closed {
			_ = tmp.Close()
		}
		_ = os.Remove(tmpPath)
	}()
	if err := writeAll(tmp, data); err != nil {
		return i18n.WrapError(i18n.KeyToolSourceSinkAtomicWriteTemporary, err)
	}
	if err := tmp.Chmod(perm); err != nil {
		return i18n.WrapError(i18n.KeyToolSourceSinkAtomicChmodTemporary, err)
	}
	if err := tmp.Sync(); err != nil {
		return i18n.WrapError(i18n.KeyToolSourceSinkAtomicSyncTemporary, err)
	}
	if err := tmp.Close(); err != nil {
		closed = true
		return i18n.WrapError(i18n.KeyToolSourceSinkAtomicCloseTemporary, err)
	}
	closed = true
	if err := publishFileAtomicallyNoReplace(tmpPath, path); err != nil {
		return err
	}
	if err := syncRuntimeDirectory(dir); err != nil && !errors.Is(err, fs.ErrInvalid) {
		return i18n.WrapError(i18n.KeyToolSourceSinkAtomicSyncTemporary, err)
	}
	return nil
}

func (t *FileEditTool) completeSuccessfulEdit(ctx context.Context, completion editCompletion) (types.ToolResult, error) {
	mtimeMs := time.Now().UnixMilli()
	if info, err := os.Stat(completion.AbsPath); err == nil {
		mtimeMs = info.ModTime().UnixMilli()
	}
	info, _ := os.Stat(completion.AbsPath)
	mtimeNs, totalBytes := int64(0), int64(len(completion.After))
	if info != nil {
		mtimeNs, totalBytes = info.ModTime().UnixNano(), info.Size()
	}
	postDigest := ""
	if completion.ContentDigest != "" && info != nil {
		if digest, snapshotInfo, err := digestFileAtPath(completion.AbsPath, info); err == nil && digest == completion.ContentDigest {
			postDigest = digest
			info = snapshotInfo
			mtimeMs = snapshotInfo.ModTime().UnixMilli()
			mtimeNs, totalBytes = snapshotInfo.ModTime().UnixNano(), snapshotInfo.Size()
		}
	}
	totalLines := readStateTotalLines(completion.After)
	coverage, _ := readObservationCoverage(1, totalLines, totalLines)
	if postDigest == "" {
		t.readState().ClearForContext(ctx, completion.AbsPath)
	} else {
		t.readState().SetForContext(ctx, completion.AbsPath, ReadFileEntry{
			TimestampMs:      mtimeMs,
			MtimeNs:          mtimeNs,
			TotalBytes:       totalBytes,
			ContentDigest:    postDigest,
			FileIdentity:     info,
			TotalLines:       totalLines,
			Coverage:         coverage,
			CoverageKnown:    true,
			CoverageComplete: true,
			FullSnapshot:     true,
			Content:          completion.After,
			IsPartialView:    false,
			LastTool:         "Edit",
			Encoding:         completion.Encoding,
			BOM:              append([]byte(nil), completion.BOM...),
		})
	}

	if t.DiagnosticsTracker != nil {
		t.DiagnosticsTracker.ClearForFile(completion.AbsPath)
	}
	if syncer, ok := t.LSP.(LSPDocumentSync); ok && syncer != nil {
		_ = syncer.DidChange(ctx, completion.AbsPath, completion.After)
		_ = syncer.DidSave(ctx, completion.AbsPath, completion.After)
	}
	if t.VSCodeNotifier != nil {
		_ = t.VSCodeNotifier.NotifyFileUpdated(ctx, completion.AbsPath, completion.After)
	}
	if t.HistoryStore != nil {
		_ = t.HistoryStore.TrackEdit(FileHistoryEntry{
			Path:   completion.AbsPath,
			Before: completion.Before,
			After:  completion.After,
			Tool:   "Edit",
			Ts:     time.Now().UnixMilli(),
		})
	}

	diagnostics := []LSPDiagnostic{}
	if t.LSP != nil {
		if values, err := t.LSP.Diagnose(ctx, completion.AbsPath, completion.After); err == nil {
			if len(values) > 20 {
				values = values[:20]
			}
			if values != nil {
				diagnostics = values
			}
		}
	}
	if t.DiagnosticsTracker != nil {
		diagnostics = t.DiagnosticsTracker.FilterUndelivered(completion.AbsPath, diagnostics)
		t.DiagnosticsTracker.MarkDelivered(completion.AbsPath, diagnostics)
	}

	t.activateSkillsForEditedPath(ctx, completion.AbsPath)
	t.emitEditAnalytics("tengu_edit_string_lengths", map[string]any{
		"oldStringBytes": len([]byte(completion.AnalyticsOldString)),
		"newStringBytes": len([]byte(completion.NewString)),
		"replaceAll":     completion.ReplaceAll,
	})
	t.emitEditAnalytics("tengu_file_operation", map[string]any{
		"operation": "edit",
		"tool":      "FileEditTool",
		"filePath":  completion.AbsPath,
	})
	if strings.EqualFold(filepath.Base(completion.AbsPath), "CLAUDE.md") {
		t.emitEditAnalytics("tengu_write_claudemd", map[string]any{})
	}
	gitDiff := t.computeEditGitDiff(ctx, completion.AbsPath)

	event := EditChangeEvent{
		FilePath:        completion.AbsPath,
		Before:          completion.Before,
		After:           completion.After,
		StructuredPatch: append([]DiffHunk(nil), completion.Patch...),
		Occurrences:     completion.Occurrences,
		ReplaceAll:      completion.ReplaceAll,
		GitDiff:         gitDiff,
	}
	if t.ChangeListener != nil {
		t.ChangeListener(event)
	}

	result := EditResult{
		FilePath:        completion.DisplayPath,
		OldString:       completion.OldString,
		NewString:       completion.NewString,
		OriginalFile:    completion.Before,
		StructuredPatch: completion.Patch,
		UserModified:    t.UserModified,
		ReplaceAll:      completion.ReplaceAll,
		ReplaceAllUsed:  completion.ReplaceAll,
		GitDiff:         gitDiff,
		Occurrences:     completion.Occurrences,
		DurationMs:      time.Since(completion.StartedAt).Milliseconds(),
		Status:          "success",
		Diagnostics:     diagnostics,
		Warning:         completion.SettingsWarning,
	}
	return editSuccessResponse(result)
}
