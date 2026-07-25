package file

import (
	"context"
	"os"

	"github.com/agent-dance/luban/i18n"
	filemutationcontract "github.com/agent-dance/luban/internal/contracts/filemutation"
)

// fileMutationCoordinator keeps non-file executors on the same canonical
// locks, file-identity checks, and read-evidence ledger as Edit and Write.
// File-domain implementation details deliberately remain private to tools.
type fileMutationCoordinator struct {
	readState *ReadFileState
}

var _ filemutationcontract.Coordinator = (*fileMutationCoordinator)(nil)

// NewFileMutationCoordinator creates the file-domain mutation capability for
// executors such as shell tools. Callers should share the same ReadFileState
// with Read, Edit, and Write.
func NewFileMutationCoordinator(readState *ReadFileState) filemutationcontract.Coordinator {
	return &fileMutationCoordinator{readState: readState}
}

func (c *fileMutationCoordinator) Lock(targets []filemutationcontract.Target) func() {
	paths := make([]string, 0, len(targets)*2)
	for _, target := range targets {
		paths = appendMutationPath(paths, target.Path)
		paths = appendMutationPath(paths, target.BackupPath)
	}
	return lockFileEditsWithRegisteredHook(nil, paths...)
}

func (c *fileMutationCoordinator) ValidateFullRead(ctx context.Context, targets []filemutationcontract.Target) error {
	if c == nil || c.readState == nil {
		for _, path := range primaryMutationPaths(targets) {
			return i18n.NewError(i18n.KeyToolRuntimeBashSedReadRequired, path)
		}
		return nil
	}
	for _, path := range primaryMutationPaths(targets) {
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return i18n.NewError(i18n.KeyToolRuntimeBashSedReadRequired, path)
		}
		entry, found := c.readState.GetForContext(ctx, path)
		if !found || entry.IsPartialView || !readEntryHasFullSnapshot(entry) ||
			!mutationReadStateIsFresh(path, info, entry) {
			return i18n.NewError(i18n.KeyToolRuntimeBashSedReadRequired, path)
		}
	}
	return nil
}

func (c *fileMutationCoordinator) Commit(ctx context.Context, targets []filemutationcontract.Target, source string) error {
	if c == nil || c.readState == nil {
		return nil
	}

	type committedSnapshot struct {
		path     string
		snapshot editTargetSnapshot
	}
	paths := primaryMutationPaths(targets)
	snapshots := make([]committedSnapshot, 0, len(paths))
	for _, path := range paths {
		snapshot, err := readEditTarget(path, nil)
		if err != nil {
			c.Invalidate(ctx, targets)
			return err
		}
		snapshots = append(snapshots, committedSnapshot{path: path, snapshot: snapshot})
	}

	for _, committed := range snapshots {
		detected := detectFileEncoding(committed.snapshot.Raw)
		decoded := decodeFileBytes(committed.snapshot.Raw, detected)
		totalLines := readStateTotalLines(decoded)
		coverage, _ := readObservationCoverage(1, totalLines, totalLines)
		c.readState.SetForContext(ctx, committed.path, ReadFileEntry{
			TimestampMs:      committed.snapshot.Info.ModTime().UnixMilli(),
			MtimeNs:          committed.snapshot.Info.ModTime().UnixNano(),
			TotalBytes:       committed.snapshot.Info.Size(),
			ContentDigest:    committed.snapshot.ContentDigest,
			FileIdentity:     committed.snapshot.Info,
			TotalLines:       totalLines,
			Coverage:         coverage,
			CoverageComplete: true,
			FullSnapshot:     true,
			Content:          decoded,
			LastTool:         source,
			Encoding:         detected.Encoding,
			BOM:              append([]byte(nil), detected.BOM...),
		})
	}
	for _, backupPath := range backupMutationPaths(targets) {
		c.readState.ClearForContext(ctx, backupPath)
	}
	return nil
}

func (c *fileMutationCoordinator) Invalidate(ctx context.Context, targets []filemutationcontract.Target) {
	if c == nil || c.readState == nil {
		return
	}
	for _, path := range allMutationPaths(targets) {
		c.readState.ClearForContext(ctx, path)
	}
}

func appendMutationPath(paths []string, path string) []string {
	if path == "" {
		return paths
	}
	return append(paths, path)
}

func primaryMutationPaths(targets []filemutationcontract.Target) []string {
	paths := make([]string, 0, len(targets))
	for _, target := range targets {
		paths = appendMutationPath(paths, target.Path)
	}
	return uniqueCanonicalMutationPaths(paths)
}

func backupMutationPaths(targets []filemutationcontract.Target) []string {
	paths := make([]string, 0, len(targets))
	for _, target := range targets {
		paths = appendMutationPath(paths, target.BackupPath)
	}
	return uniqueCanonicalMutationPaths(paths)
}

func allMutationPaths(targets []filemutationcontract.Target) []string {
	paths := make([]string, 0, len(targets)*2)
	for _, target := range targets {
		paths = appendMutationPath(paths, target.Path)
		paths = appendMutationPath(paths, target.BackupPath)
	}
	return uniqueCanonicalMutationPaths(paths)
}

func uniqueCanonicalMutationPaths(paths []string) []string {
	unique := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		canonical := canonicalFileEditLockPath(path)
		if canonical == "" {
			continue
		}
		if _, found := seen[canonical]; found {
			continue
		}
		seen[canonical] = struct{}{}
		unique = append(unique, path)
	}
	return unique
}

func mutationReadStateIsFresh(path string, info os.FileInfo, entry ReadFileEntry) bool {
	if entry.ContentDigest == "" || entry.FileIdentity == nil || info == nil ||
		!os.SameFile(entry.FileIdentity, info) || !readEntryMatchesModTime(entry, info.ModTime()) {
		return false
	}
	snapshot, err := readEditTarget(path, entry.FileIdentity)
	return err == nil && snapshot.ContentDigest == entry.ContentDigest
}
