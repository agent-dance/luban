package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

func TestValidateArtifactInventoryLockRejectsRepinnedDifferentInventory(t *testing.T) {
	root := t.TempDir()
	task := harness.Task{
		ID: "task-a", BaseCommit: strings.Repeat("a", 40),
		ManifestSHA256: strings.Repeat("b", 64), InstructionSHA256: strings.Repeat("c", 64),
		Image: "registry.example/task:a", ImageDigest: "sha256:" + strings.Repeat("d", 64),
	}
	inventorySHA, err := harness.HashTaskInventory([]harness.Task{task})
	if err != nil {
		t.Fatal(err)
	}
	datasetCommit := strings.Repeat("e", 40)
	lockPath := filepath.Join(root, harness.InventoryLockArchiveRelativePath)
	writeLock := func(image string) string {
		t.Helper()
		if err := harness.WriteJSONAtomic(lockPath, map[string]any{
			"schema_version": harness.PierInventoryLockSchemaVersion, "dataset_commit": datasetCommit,
			"coverage": "full", "universe_task_count": 1,
			"tasks": []map[string]any{{
				"id": task.ID, "relative_path": task.ID, "base_commit": task.BaseCommit,
				"manifest_sha256": task.ManifestSHA256, "instruction_sha256": task.InstructionSHA256,
				"image": image, "image_digest": task.ImageDigest,
			}},
		}, 0o644); err != nil {
			t.Fatal(err)
		}
		fileSHA, err := harness.HashFile(lockPath)
		if err != nil {
			t.Fatal(err)
		}
		return fileSHA
	}
	fileSHA := writeLock(task.Image)
	loaded := harness.LoadedManifest{Manifest: harness.Manifest{
		Dataset:   harness.SourcePin{Commit: datasetCommit, ManifestSHA256: inventorySHA},
		Selection: harness.SelectionSpec{Mode: "full", ExpectedTaskCount: 1},
	}}
	state := harness.ExperimentState{Backend: harness.BackendSnapshot{
		InventoryLock: harness.InventoryLockSnapshot{
			RelativePath: harness.InventoryLockArchiveRelativePath, FileSHA256: fileSHA,
			SchemaVersion: harness.PierInventoryLockSchemaVersion, HashAlgorithm: harness.TaskInventoryHashAlgorithm,
			DatasetCommit: datasetCommit, Coverage: "full", TaskCount: 1, UniverseTaskCount: 1,
			TaskInventorySHA256: inventorySHA,
		},
		InventoryCoverage: "full", InventoryTaskCount: 1, UniverseTaskCount: 1,
	}}
	if err := validateArtifactInventoryLock(root, loaded, state); err != nil {
		t.Fatalf("valid archived inventory: %v", err)
	}

	changedTask := task
	changedTask.Image = "registry.example/task:repinned"
	changedInventorySHA, err := harness.HashTaskInventory([]harness.Task{changedTask})
	if err != nil {
		t.Fatal(err)
	}
	state.Backend.InventoryLock.FileSHA256 = writeLock(changedTask.Image)
	state.Backend.InventoryLock.TaskInventorySHA256 = changedInventorySHA
	if err := validateArtifactInventoryLock(root, loaded, state); err == nil {
		t.Fatal("repinned state and lock escaped the frozen manifest inventory")
	}
}
