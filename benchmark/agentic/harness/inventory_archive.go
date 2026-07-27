package harness

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	InventoryLockArchiveRelativePath = "inventory-lock.json"
	TaskInventoryHashAlgorithm       = "sha256-canonical-task-array-v1"
	PierInventoryLockSchemaVersion   = "agentic-bench/pier-inventory-v2"
)

// InventoryLockArchiver is implemented by formal backends that own a mutable
// controller-side inventory source. BindInventoryLockArchive must atomically
// create the archive on first use, and must use only the existing archive on
// resume. Preflight then binds its BackendSnapshot to those archived bytes.
type InventoryLockArchiver interface {
	BindInventoryLockArchive(ctx context.Context, archivePath string) error
}

type archivedInventoryLock struct {
	SchemaVersion     string                  `json:"schema_version"`
	DatasetCommit     string                  `json:"dataset_commit"`
	Coverage          string                  `json:"coverage"`
	UniverseTaskCount int                     `json:"universe_task_count"`
	TaskIDs           []string                `json:"task_ids,omitempty"`
	Tasks             []archivedInventoryTask `json:"tasks"`
}

type archivedInventoryTask struct {
	ID                string `json:"id"`
	RelativePath      string `json:"relative_path"`
	BaseCommit        string `json:"base_commit"`
	ManifestSHA256    string `json:"manifest_sha256"`
	InstructionSHA256 string `json:"instruction_sha256"`
	Image             string `json:"image"`
	ImageDigest       string `json:"image_digest"`
}

// ValidateInventoryLockArchive reparses the exact archived bytes and derives
// HashTaskInventory independently. It is shared by resume and report loading
// so neither path trusts fields copied from state.json alone.
func ValidateInventoryLockArchive(path string, expected InventoryLockSnapshot) ([]Task, error) {
	if expected.RelativePath != InventoryLockArchiveRelativePath ||
		expected.SchemaVersion != PierInventoryLockSchemaVersion ||
		expected.HashAlgorithm != TaskInventoryHashAlgorithm ||
		!hex64Pattern.MatchString(expected.FileSHA256) ||
		!hex64Pattern.MatchString(expected.TaskInventorySHA256) {
		return nil, errors.New("inventory-lock snapshot identity is incomplete")
	}
	if filepath.Base(path) != InventoryLockArchiveRelativePath {
		return nil, errors.New("inventory-lock archive path is not canonical")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	fileSum := sha256.Sum256(raw)
	if hex.EncodeToString(fileSum[:]) != expected.FileSHA256 {
		return nil, errors.New("inventory-lock archive bytes differ from the backend snapshot")
	}
	lock, err := decodeArchivedInventoryLock(raw)
	if err != nil {
		return nil, err
	}
	if lock.SchemaVersion != expected.SchemaVersion ||
		lock.DatasetCommit != expected.DatasetCommit ||
		lock.Coverage != expected.Coverage ||
		len(lock.Tasks) != expected.TaskCount ||
		lock.UniverseTaskCount != expected.UniverseTaskCount {
		return nil, errors.New("inventory-lock archive metadata differs from the backend snapshot")
	}
	if err := validateArchivedInventoryStructure(lock); err != nil {
		return nil, err
	}
	tasks := make([]Task, 0, len(lock.Tasks))
	for _, task := range lock.Tasks {
		tasks = append(tasks, Task{
			ID: task.ID, BaseCommit: task.BaseCommit,
			ManifestSHA256: task.ManifestSHA256, InstructionSHA256: task.InstructionSHA256,
			Image: task.Image, ImageDigest: task.ImageDigest,
		})
	}
	inventorySHA, err := HashTaskInventory(tasks)
	if err != nil {
		return nil, err
	}
	if inventorySHA != expected.TaskInventorySHA256 {
		return nil, errors.New("inventory-lock archive derives a different task inventory hash")
	}
	return tasks, nil
}

func decodeArchivedInventoryLock(raw []byte) (archivedInventoryLock, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var lock archivedInventoryLock
	if err := decoder.Decode(&lock); err != nil {
		return archivedInventoryLock{}, fmt.Errorf("decode archived inventory lock: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return archivedInventoryLock{}, errors.New("archived inventory lock contains trailing JSON")
	}
	return lock, nil
}

func validateArchivedInventoryStructure(lock archivedInventoryLock) error {
	if lock.SchemaVersion != PierInventoryLockSchemaVersion ||
		!hex40Pattern.MatchString(lock.DatasetCommit) || lock.UniverseTaskCount < 1 || len(lock.Tasks) == 0 {
		return errors.New("archived inventory lock has an invalid schema or dataset identity")
	}
	if !slices.IsSortedFunc(lock.Tasks, func(left, right archivedInventoryTask) int {
		return strings.Compare(left.ID, right.ID)
	}) {
		return errors.New("archived inventory lock tasks are not in canonical order")
	}
	for index, task := range lock.Tasks {
		if index > 0 && lock.Tasks[index-1].ID == task.ID {
			return errors.New("archived inventory lock contains duplicate task IDs")
		}
		if task.RelativePath == "" || filepath.IsAbs(task.RelativePath) || filepath.Clean(task.RelativePath) != task.RelativePath || strings.HasPrefix(task.RelativePath, "..") || strings.TrimSpace(task.Image) == "" {
			return fmt.Errorf("archived inventory task %q has an invalid path or image", task.ID)
		}
	}
	switch lock.Coverage {
	case "full":
		if len(lock.Tasks) != lock.UniverseTaskCount || len(lock.TaskIDs) != 0 {
			return errors.New("archived full inventory lock does not cover its universe")
		}
	case "tasks":
		if len(lock.Tasks) >= lock.UniverseTaskCount || len(lock.TaskIDs) != len(lock.Tasks) {
			return errors.New("archived partial inventory lock has invalid cardinality")
		}
		for index, task := range lock.Tasks {
			if lock.TaskIDs[index] != task.ID || (index > 0 && strings.Compare(lock.TaskIDs[index-1], lock.TaskIDs[index]) >= 0) {
				return errors.New("archived partial inventory IDs are not exact and canonical")
			}
		}
	default:
		return errors.New("archived inventory lock has unsupported coverage")
	}
	return nil
}
