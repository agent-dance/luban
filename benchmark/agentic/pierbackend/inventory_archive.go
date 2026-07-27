package pierbackend

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

func (backend *Backend) BindInventoryLockArchive(ctx context.Context, archivePath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !filepath.IsAbs(archivePath) || filepath.Base(archivePath) != harness.InventoryLockArchiveRelativePath {
		return errors.New("inventory-lock archive path must be absolute and canonical")
	}
	raw, err := readRegularInventoryArchive(archivePath)
	if errors.Is(err, os.ErrNotExist) {
		raw, err = os.ReadFile(backend.config.InventoryLockPath)
		if err != nil {
			return err
		}
		if _, err := decodeInventoryLock(raw); err != nil {
			return err
		}
		if err := writeExclusiveInventoryArchive(archivePath, raw); err != nil {
			if !errors.Is(err, os.ErrExist) {
				return err
			}
			raw, err = readRegularInventoryArchive(archivePath)
			if err != nil {
				return err
			}
		}
	} else if err != nil {
		return err
	}
	lock, err := decodeInventoryLock(raw)
	if err != nil {
		return err
	}
	inventory := make([]harness.Task, 0, len(lock.Tasks))
	for _, task := range lock.Tasks {
		inventory = append(inventory, task.HarnessTask())
	}
	inventorySHA, err := harness.HashTaskInventory(inventory)
	if err != nil {
		return err
	}
	fileSum := sha256.Sum256(raw)
	snapshot := harness.InventoryLockSnapshot{
		RelativePath: harness.InventoryLockArchiveRelativePath,
		FileSHA256:   hex.EncodeToString(fileSum[:]), SchemaVersion: lock.SchemaVersion,
		HashAlgorithm: harness.TaskInventoryHashAlgorithm, DatasetCommit: lock.DatasetCommit,
		Coverage: lock.Coverage, TaskCount: len(lock.Tasks), UniverseTaskCount: lock.UniverseTaskCount,
		TaskInventorySHA256: inventorySHA,
	}
	if _, err := harness.ValidateInventoryLockArchive(archivePath, snapshot); err != nil {
		return err
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.lockBound && backend.lockSnapshot != snapshot {
		return errors.New("inventory-lock archive identity changed before preflight")
	}
	if backend.ready {
		return nil
	}
	backend.lock = lock
	backend.lockSnapshot = snapshot
	backend.lockBound = true
	return nil
}

func readRegularInventoryArchive(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("inventory-lock archive is not a regular file")
	}
	return os.ReadFile(path)
}

func writeExclusiveInventoryArchive(path string, raw []byte) (resultErr error) {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".inventory-lock-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() {
		removeErr := os.Remove(temporaryPath)
		if removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			resultErr = errors.Join(resultErr, removeErr)
		}
	}()
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return err
	}
	written, writeErr := temporary.Write(raw)
	if writeErr == nil && written != len(raw) {
		writeErr = io.ErrShortWrite
	}
	if writeErr == nil {
		writeErr = temporary.Sync()
	}
	closeErr := temporary.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return err
	}
	if err := os.Link(temporaryPath, path); err != nil {
		return err
	}
	directoryHandle, err := os.Open(directory)
	if err != nil {
		return err
	}
	syncErr := directoryHandle.Sync()
	closeErr = directoryHandle.Close()
	return errors.Join(syncErr, closeErr)
}
