package file

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	filemutationcontract "github.com/agent-dance/luban/internal/contracts/filemutation"
)

func TestFileMutationCoordinatorValidatesOnlyPrimaryFullReads(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	backup := filepath.Join(dir, "target.txt.bak")
	if err := os.WriteFile(target, []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backup, []byte("old backup\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	state := NewReadFileState()
	recordFullMutationRead(t, state, target)
	coordinator := NewFileMutationCoordinator(state)
	targets := []filemutationcontract.Target{{Path: target, BackupPath: backup}}
	if err := coordinator.ValidateFullRead(context.Background(), targets); err != nil {
		t.Fatalf("valid primary evidence rejected because backup was unread: %v", err)
	}

	if err := os.WriteFile(target, []byte("changed behind evidence\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := coordinator.ValidateFullRead(context.Background(), targets); err == nil {
		t.Fatal("stale primary evidence was accepted")
	}
}

func TestFileMutationCoordinatorWithoutLedgerFailsClosed(t *testing.T) {
	target := filepath.Join(t.TempDir(), "target.txt")
	coordinator := NewFileMutationCoordinator(nil)
	if err := coordinator.ValidateFullRead(context.Background(), []filemutationcontract.Target{{Path: target}}); err == nil {
		t.Fatal("mutation validation succeeded without a read-evidence ledger")
	}
}

func TestFileMutationCoordinatorCommitAndInvalidate(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	backup := filepath.Join(dir, "target.txt.bak")
	if err := os.WriteFile(target, []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backup, []byte("backup\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	state := NewReadFileState()
	recordFullMutationRead(t, state, target)
	recordFullMutationRead(t, state, backup)
	coordinator := NewFileMutationCoordinator(state)
	targets := []filemutationcontract.Target{{Path: target, BackupPath: backup}}

	utf16LE := append([]byte(nil), bomUTF16LE...)
	utf16LE = append(utf16LE, 'a', 0, 'f', 0, 't', 0, 'e', 0, 'r', 0, '\n', 0)
	if err := os.WriteFile(target, utf16LE, 0o600); err != nil {
		t.Fatal(err)
	}
	release := coordinator.Lock(targets)
	if err := coordinator.Commit(context.Background(), targets, "Bash"); err != nil {
		release()
		t.Fatal(err)
	}
	release()

	entry, found := state.GetForContext(context.Background(), target)
	if !found {
		t.Fatal("primary post-image evidence was not committed")
	}
	if entry.LastTool != "Bash" || entry.Content != "after\n" ||
		entry.Encoding != EncodingUTF16LE || !bytes.Equal(entry.BOM, bomUTF16LE) ||
		!entry.CoverageComplete || !entry.FullSnapshot || entry.FileIdentity == nil {
		t.Fatalf("incomplete committed evidence: %+v", entry)
	}
	if _, found := state.GetForContext(context.Background(), backup); found {
		t.Fatal("backup evidence survived a successful commit")
	}

	coordinator.Invalidate(context.Background(), targets)
	if _, found := state.GetForContext(context.Background(), target); found {
		t.Fatal("primary evidence survived invalidation")
	}
}

func TestFileMutationCoordinatorFailedCommitInvalidatesEveryTarget(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.txt")
	missing := filepath.Join(dir, "missing.txt")
	backup := filepath.Join(dir, "missing.txt.bak")
	for _, path := range []string{first, missing, backup} {
		if err := os.WriteFile(path, []byte("observed\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	state := NewReadFileState()
	for _, path := range []string{first, missing, backup} {
		recordFullMutationRead(t, state, path)
	}
	coordinator := NewFileMutationCoordinator(state)
	targets := []filemutationcontract.Target{
		{Path: first},
		{Path: missing, BackupPath: backup},
	}
	if err := os.Remove(missing); err != nil {
		t.Fatal(err)
	}
	if err := coordinator.Commit(context.Background(), targets, "Bash"); err == nil {
		t.Fatal("commit unexpectedly succeeded after a primary target disappeared")
	}
	for _, path := range []string{first, missing, backup} {
		if _, found := state.GetForContext(context.Background(), path); found {
			t.Fatalf("evidence for %q survived a failed multi-target commit", path)
		}
	}
}

func TestFileMutationCoordinatorLocksPrimaryAndBackupCanonicalPaths(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	backup := filepath.Join(dir, "target.txt.bak")
	coordinator := NewFileMutationCoordinator(NewReadFileState())
	release := coordinator.Lock([]filemutationcontract.Target{
		{Path: target, BackupPath: backup},
		{Path: filepath.Join(dir, ".", "target.txt")},
	})

	keys := []string{canonicalFileEditLockPath(target), canonicalFileEditLockPath(backup)}
	editPathLocks.Lock()
	for _, key := range keys {
		lock, found := editPathLocks.locks[key]
		if !found || lock.refs != 1 {
			editPathLocks.Unlock()
			release()
			t.Fatalf("canonical mutation lock %q = %#v, found=%t", key, lock, found)
		}
	}
	editPathLocks.Unlock()

	release()
	editPathLocks.Lock()
	defer editPathLocks.Unlock()
	for _, key := range keys {
		if _, found := editPathLocks.locks[key]; found {
			t.Fatalf("canonical mutation lock %q leaked after release", key)
		}
	}
}

func recordFullMutationRead(t *testing.T, state *ReadFileState, path string) {
	t.Helper()
	snapshot, err := readEditTarget(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	detected := detectFileEncoding(snapshot.Raw)
	decoded := decodeFileBytes(snapshot.Raw, detected)
	totalLines := readStateTotalLines(decoded)
	coverage, complete := readObservationCoverage(1, totalLines, totalLines)
	state.RecordReadForContext(context.Background(), path, ReadFileEntry{
		TimestampMs:      snapshot.Info.ModTime().UnixMilli(),
		MtimeNs:          snapshot.Info.ModTime().UnixNano(),
		TotalBytes:       snapshot.Info.Size(),
		ContentDigest:    snapshot.ContentDigest,
		FileIdentity:     snapshot.Info,
		TotalLines:       totalLines,
		Coverage:         coverage,
		CoverageComplete: complete,
		FullSnapshot:     true,
		Content:          decoded,
		LastTool:         "Read",
		Encoding:         detected.Encoding,
		BOM:              append([]byte(nil), detected.BOM...),
	})
}
