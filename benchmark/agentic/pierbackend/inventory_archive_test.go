package pierbackend

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

func TestBindInventoryLockArchivePreservesExactBytesAndResumeUsesArchive(t *testing.T) {
	releasePath := filepath.Join("..", "manifests", "deepswe-v1.1-release-full-inventory-lock-8cae5984-v2.json")
	releaseRaw, err := os.ReadFile(releasePath)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	sourcePath := filepath.Join(directory, "controller-source.json")
	archivePath := filepath.Join(directory, harness.InventoryLockArchiveRelativePath)
	if err := os.WriteFile(sourcePath, releaseRaw, 0o644); err != nil {
		t.Fatal(err)
	}

	backend := &Backend{config: Config{InventoryLockPath: sourcePath}}
	if err := backend.BindInventoryLockArchive(context.Background(), archivePath); err != nil {
		t.Fatal(err)
	}
	archivedRaw, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(archivedRaw) != string(releaseRaw) {
		t.Fatal("first-run inventory archive did not preserve exact source bytes")
	}
	want := backend.lockSnapshot
	if want.FileSHA256 != "e23cb7c40f696e191122647295d24ef6a4c2e7d2df2dca359acfaebc05e28263" ||
		want.TaskInventorySHA256 != "85f7f80eb0c48ea3480f95e145d13bacf5782c9aea1c576f79c65a14626d3a7a" ||
		want.TaskCount != 113 || want.UniverseTaskCount != 113 || want.Coverage != "full" {
		t.Fatalf("inventory archive snapshot = %#v", want)
	}
	if _, err := harness.ValidateInventoryLockArchive(archivePath, want); err != nil {
		t.Fatal(err)
	}

	// Resume must not consult or repair from the now-mutated controller path.
	if err := os.WriteFile(sourcePath, []byte("not the frozen lock\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	resumed := &Backend{config: Config{InventoryLockPath: sourcePath}}
	if err := resumed.BindInventoryLockArchive(context.Background(), archivePath); err != nil {
		t.Fatalf("resume consulted mutable controller lock: %v", err)
	}
	if resumed.lockSnapshot != want {
		t.Fatalf("resume snapshot = %#v, want %#v", resumed.lockSnapshot, want)
	}
}

func TestReleasedPilotInventoryLockHasRegisteredV1Identity(t *testing.T) {
	releasePath := filepath.Join("..", "manifests", "deepswe-v1.1-release-pilot-inventory-lock.json")
	releaseRaw, err := os.ReadFile(releasePath)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	sourcePath := filepath.Join(directory, "controller-source.json")
	archivePath := filepath.Join(directory, harness.InventoryLockArchiveRelativePath)
	if err := os.WriteFile(sourcePath, releaseRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	backend := &Backend{config: Config{InventoryLockPath: sourcePath}}
	if err := backend.BindInventoryLockArchive(context.Background(), archivePath); err != nil {
		t.Fatal(err)
	}
	got := backend.lockSnapshot
	if got.FileSHA256 != "82b7be87fe9a25118564319959afac9c4ab8d9033a8c3b01dfed96664887a94e" ||
		got.TaskInventorySHA256 != "0d76a2c978a96350d1dc8468746e56ce25f34526aeffe85094d720979bf6a96b" ||
		got.TaskCount != 5 || got.UniverseTaskCount != 113 || got.Coverage != "tasks" || got.HashAlgorithm != harness.TaskInventoryHashAlgorithm {
		t.Fatalf("released pilot inventory snapshot = %#v", got)
	}
}

func TestBindInventoryLockArchiveRejectsNonRegularOrTamperedArchive(t *testing.T) {
	releasePath := filepath.Join("..", "manifests", "deepswe-v1.1-release-full-inventory-lock-8cae5984-v2.json")
	releaseRaw, err := os.ReadFile(releasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("symlink", func(t *testing.T) {
		directory := t.TempDir()
		sourcePath := filepath.Join(directory, "source.json")
		if err := os.WriteFile(sourcePath, releaseRaw, 0o644); err != nil {
			t.Fatal(err)
		}
		archivePath := filepath.Join(directory, harness.InventoryLockArchiveRelativePath)
		if err := os.Symlink(sourcePath, archivePath); err != nil {
			t.Fatal(err)
		}
		backend := &Backend{config: Config{InventoryLockPath: sourcePath}}
		if err := backend.BindInventoryLockArchive(context.Background(), archivePath); err == nil {
			t.Fatal("symlink inventory archive was accepted")
		}
	})
	t.Run("tampered", func(t *testing.T) {
		directory := t.TempDir()
		sourcePath := filepath.Join(directory, "source.json")
		archivePath := filepath.Join(directory, harness.InventoryLockArchiveRelativePath)
		if err := os.WriteFile(sourcePath, releaseRaw, 0o644); err != nil {
			t.Fatal(err)
		}
		backend := &Backend{config: Config{InventoryLockPath: sourcePath}}
		if err := backend.BindInventoryLockArchive(context.Background(), archivePath); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(archivePath, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		resumed := &Backend{config: Config{InventoryLockPath: sourcePath}}
		if err := resumed.BindInventoryLockArchive(context.Background(), archivePath); err == nil {
			t.Fatal("tampered archived inventory lock was accepted")
		}
	})
}
