package file

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/store/secureio"
	"github.com/agent-dance/luban/internal/tools/toolbase"
	"github.com/agent-dance/luban/types"
)

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
	return toolbase.CanonicalPath(path)
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

func (t *FileEditTool) activateSkillsForEditedPath(ctx context.Context, absPath string) {
	if t == nil || t.SkillManager == nil || fileReadSimpleMode() {
		return
	}
	dirs := DiscoverSkillDirsForPaths([]string{absPath})
	if len(dirs) == 0 {
		return
	}
	addSkillDirectoriesForExecution(ctx, t.SkillManager, dirs)
	activateConditionalPathForExecution(ctx, t.SkillManager, absPath)
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
	AbsPath       string
	DisplayPath   string
	Before        string
	After         string
	OldString     string
	NewString     string
	Patch         []DiffHunk
	Occurrences   int
	ReplaceAll    bool
	StartedAt     time.Time
	Encoding      FileEncoding
	BOM           []byte
	ContentDigest string
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
	if err := secureio.WriteAll(tmp, data); err != nil {
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
	if err := secureio.ReplaceFileAtomically(tmpPath, path); err != nil {
		return i18n.WrapError(i18n.KeyToolSourceSinkAtomicReplaceTarget, err)
	}
	success = true
	if err := secureio.SyncRuntimeDirectory(dir); err != nil && !errors.Is(err, fs.ErrInvalid) {
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
	if err := secureio.WriteAll(tmp, data); err != nil {
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
	if err := secureio.PublishFileAtomicallyNoReplace(tmpPath, path); err != nil {
		return err
	}
	if err := secureio.SyncRuntimeDirectory(dir); err != nil && !errors.Is(err, fs.ErrInvalid) {
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
			CoverageComplete: true,
			FullSnapshot:     true,
			Content:          completion.After,
			IsPartialView:    false,
			LastTool:         "Edit",
			Encoding:         completion.Encoding,
			BOM:              append([]byte(nil), completion.BOM...),
		})
	}

	t.activateSkillsForEditedPath(ctx, completion.AbsPath)

	result := EditResult{
		FilePath:        completion.DisplayPath,
		OldString:       completion.OldString,
		NewString:       completion.NewString,
		OriginalFile:    completion.Before,
		StructuredPatch: completion.Patch,
		ReplaceAll:      completion.ReplaceAll,
		Occurrences:     completion.Occurrences,
		DurationMs:      time.Since(completion.StartedAt).Milliseconds(),
		Status:          "success",
	}
	return editSuccessResponse(result)
}
