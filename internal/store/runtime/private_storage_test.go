package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"
	"time"

	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
	storepaths "github.com/agent-dance/luban/internal/store/paths"
)

func TestDefaultRuntimeStorageLeavesProjectUntouchedAndIsolatesSessions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	project := filepath.Join(t.TempDir(), "repo")
	if err := os.Mkdir(project, 0o755); err != nil {
		t.Fatal(err)
	}
	tracked := filepath.Join(project, "tracked.txt")
	if err := os.WriteFile(tracked, []byte("unchanged"), 0o644); err != nil {
		t.Fatal(err)
	}

	sessionA := NewRuntimeLifecycleForSession(project, "session-a")
	sessionB := NewRuntimeLifecycleForSession(project, "session-b")
	if sessionA.StorageRoot() == sessionB.StorageRoot() {
		t.Fatalf("sessions share storage root %q", sessionA.StorageRoot())
	}
	for _, lifecycle := range []*RuntimeLifecycle{sessionA, sessionB} {
		if err := lifecycle.Publish(context.Background(), RuntimeLifecycleEvent{Type: LifecycleTaskCreated, EntityID: "task"}); err != nil {
			t.Fatal(err)
		}
		if !filepath.IsAbs(lifecycle.StorageRoot()) || filepath.Clean(lifecycle.Root()) != filepath.Clean(project) {
			t.Fatalf("logical/external roots = %q / %q", lifecycle.Root(), lifecycle.StorageRoot())
		}
		if wantPrefix := storepaths.RuntimeProjectDir(project); !pathWithin(lifecycle.StorageRoot(), wantPrefix) {
			t.Fatalf("storage root %q is outside %q", lifecycle.StorageRoot(), wantPrefix)
		}
		assertPrivateMode(t, lifecycle.StorageRoot(), 0o700)
		assertPrivateMode(t, lifecycle.path, 0o600)
		assertPrivateMode(t, lifecycle.lockPath(), 0o600)
	}

	if body, err := os.ReadFile(tracked); err != nil || string(body) != "unchanged" {
		t.Fatalf("project file changed: body=%q err=%v", body, err)
	}
	if _, err := os.Lstat(filepath.Join(project, ".luban-code")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("runtime created project-local state: %v", err)
	}
}

func pathWithin(path, root string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func TestPrivateStoresTightenRuntimeState(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not enforced on Windows")
	}
	root := t.TempDir()
	storageRoot := filepath.Join(t.TempDir(), "session-runtime")
	storeDir := filepath.Join(storageRoot, "runtime-tasks")
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	store := NewRuntimeTaskStoreAt(root, storageRoot)
	assertPrivateMode(t, storeDir, 0o700)
	record := RuntimeTaskRecord{ID: "private-agent", Type: agentcontract.TaskTypeLocalAgent, Status: "completed", StartedAt: time.Now().UTC()}
	body, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.path(record.ID), body, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.Get(record.ID); !ok {
		t.Fatal("runtime task was not loaded")
	}
	assertPrivateMode(t, store.path(record.ID), 0o600)
	assertPrivateMode(t, store.lockPath(record.ID), 0o600)

	lifecycle := NewRuntimeLifecycleAt(root, storageRoot)
	if err := lifecycle.Publish(context.Background(), RuntimeLifecycleEvent{Type: LifecycleTaskCreated, EntityID: "private-task"}); err != nil {
		t.Fatal(err)
	}
	assertPrivateMode(t, filepath.Dir(lifecycle.path), 0o700)
	assertPrivateMode(t, lifecycle.path, 0o600)
	assertPrivateMode(t, lifecycle.lockPath(), 0o600)
}

func TestPrivateStoresRejectLinkedAndNonRegularPaths(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("link setup requires additional privileges on Windows")
	}

	t.Run("task record and lock symlinks", func(t *testing.T) {
		root := t.TempDir()
		store := NewRuntimeTaskStoreAt(root, filepath.Join(t.TempDir(), "runtime"))
		outside := filepath.Join(t.TempDir(), "outside")
		if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, store.path("linked")); err != nil {
			t.Fatal(err)
		}
		if store.Exists("linked") {
			t.Fatal("symlink record was accepted")
		}
		if err := store.Save(RuntimeTaskRecord{ID: "linked", Type: agentcontract.TaskTypeLocalAgent}); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("symlink record error = %v, want fs.ErrInvalid", err)
		}
		assertContents(t, outside, "outside", 0o644)

		if err := os.Symlink(outside, store.lockPath("locked")); err != nil {
			t.Fatal(err)
		}
		if err := store.Save(RuntimeTaskRecord{ID: "locked", Type: agentcontract.TaskTypeLocalAgent}); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("symlink lock error = %v, want fs.ErrInvalid", err)
		}
		assertContents(t, outside, "outside", 0o644)
	})

	t.Run("hard linked record", func(t *testing.T) {
		root := t.TempDir()
		store := NewRuntimeTaskStoreAt(root, filepath.Join(t.TempDir(), "runtime"))
		outside := filepath.Join(t.TempDir(), "outside")
		if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(outside, store.path("linked")); err != nil {
			t.Skipf("hard links are unavailable: %v", err)
		}
		if store.Exists("linked") {
			t.Fatal("hard-linked record was accepted")
		}
		if err := store.Save(RuntimeTaskRecord{ID: "linked", Type: agentcontract.TaskTypeLocalAgent}); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("hard-linked record error = %v, want fs.ErrInvalid", err)
		}
		assertContents(t, outside, "outside", 0o644)
	})

	t.Run("managed directory symlink", func(t *testing.T) {
		root := t.TempDir()
		storageRoot := filepath.Join(t.TempDir(), "runtime")
		outside := t.TempDir()
		if err := os.Mkdir(storageRoot, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(storageRoot, "runtime-tasks")); err != nil {
			t.Fatal(err)
		}
		store := NewRuntimeTaskStoreAt(root, storageRoot)
		if err := store.Save(RuntimeTaskRecord{ID: "escape", Type: agentcontract.TaskTypeLocalAgent}); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("symlink directory error = %v, want fs.ErrInvalid", err)
		}
		if _, err := os.Stat(filepath.Join(outside, "escape.json")); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("symlink directory received a record: %v", err)
		}
	})

	t.Run("managed ancestor symlink", func(t *testing.T) {
		root := t.TempDir()
		outside := t.TempDir()
		if err := os.Mkdir(filepath.Join(outside, "runtime-tasks"), 0o700); err != nil {
			t.Fatal(err)
		}
		storageRoot := filepath.Join(t.TempDir(), "runtime-link")
		if err := os.Symlink(outside, storageRoot); err != nil {
			t.Fatal(err)
		}
		store := NewRuntimeTaskStoreAt(root, storageRoot)
		if err := store.Save(RuntimeTaskRecord{ID: "escape", Type: agentcontract.TaskTypeLocalAgent}); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("ancestor symlink error = %v, want fs.ErrInvalid", err)
		}
		if _, err := os.Stat(filepath.Join(outside, "runtime-tasks", "escape.json")); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("ancestor symlink received a record: %v", err)
		}
	})

	t.Run("lifecycle symlink", func(t *testing.T) {
		root := t.TempDir()
		storageRoot := filepath.Join(t.TempDir(), "runtime")
		outside := filepath.Join(t.TempDir(), "outside")
		if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
			t.Fatal(err)
		}
		lifecycle := NewRuntimeLifecycleAt(root, storageRoot)
		if err := os.Symlink(outside, lifecycle.path); err != nil {
			t.Fatal(err)
		}
		if _, err := lifecycle.Events(); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("lifecycle symlink error = %v, want fs.ErrInvalid", err)
		}
		assertContents(t, outside, "outside", 0o644)
	})

	t.Run("non regular record", func(t *testing.T) {
		store := NewRuntimeTaskStoreAt(t.TempDir(), filepath.Join(t.TempDir(), "runtime"))
		if err := os.Mkdir(store.path("directory"), 0o700); err != nil {
			t.Fatal(err)
		}
		if store.Exists("directory") {
			t.Fatal("directory was accepted as a runtime task record")
		}
		if err := store.Save(RuntimeTaskRecord{ID: "directory", Type: agentcontract.TaskTypeLocalAgent}); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("directory record error = %v, want fs.ErrInvalid", err)
		}
	})
}

func TestTaskStoreRejectsTraversalAndNormalizesOutputPath(t *testing.T) {
	root := t.TempDir()
	store := NewRuntimeTaskStoreAt(root, filepath.Join(t.TempDir(), "runtime"))
	for _, id := range []string{"../outside", "nested/task", `nested\task`, ".", "..", " leading", "trailing ", "bad\nline"} {
		if err := store.Save(RuntimeTaskRecord{ID: id, Type: agentcontract.TaskTypeLocalAgent}); !errors.Is(err, fs.ErrInvalid) {
			t.Errorf("Save(%q) error = %v, want fs.ErrInvalid", id, err)
		}
		if _, ok := store.Get(id); ok {
			t.Errorf("Get(%q) accepted traversal ID", id)
		}
	}

	forgedID := "forged-agent"
	forged := RuntimeTaskRecord{
		ID: forgedID, Type: agentcontract.TaskTypeLocalAgent, Status: "completed",
		OutputPath: filepath.Join(root, "..", "outside-secret"), StartedAt: time.Now().UTC(),
	}
	body, err := json.Marshal(forged)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.path(forgedID), body, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, ok := store.Get(forgedID)
	if !ok {
		t.Fatal("forged record was not decoded")
	}
	if loaded.OutputPath != store.outputPath(forgedID) {
		t.Fatalf("untrusted output path survived: %q", loaded.OutputPath)
	}
	if first, second := store.outputPath("agent@team"), store.outputPath("agent-team"); first == second {
		t.Fatalf("sanitizer-colliding task IDs share output path %q", first)
	}
}

func assertPrivateMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode(%s) = %04o, want %04o", path, got, want)
	}
}

func assertContents(t *testing.T, path, want string, wantMode os.FileMode) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != want {
		t.Fatalf("contents(%s) = %q, want %q", path, body, want)
	}
	assertPrivateMode(t, path, wantMode)
}
