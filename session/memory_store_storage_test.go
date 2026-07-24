package session

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestMemoryStoreStorageUsesPrivateModesAndTightensLegacyFiles(t *testing.T) {
	t.Run("new storage", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "private-memory")
		path := filepath.Join(dir, "memory.json")
		store := NewMemoryStore(path)
		if err := store.Add("private fact", "test", "project"); err != nil {
			t.Fatal(err)
		}
		assertMemoryStorageMode(t, dir, 0o700)
		assertMemoryStorageMode(t, path, 0o600)
	})

	t.Run("legacy storage", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "legacy-memory")
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "memory.json")
		payload := marshalMemoryFixture(t, "legacy fact")
		if err := os.WriteFile(path, payload, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0o644); err != nil {
			t.Fatal(err)
		}

		store := NewMemoryStore(path)
		memories := store.Memories()
		if len(memories) != 1 || memories[0].Fact != "legacy fact" {
			t.Fatalf("loaded memories = %#v", memories)
		}
		assertMemoryStorageMode(t, dir, 0o700)
		assertMemoryStorageMode(t, path, 0o600)
	})
}

func TestMemoryStoreStorageRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(root, "outside-memory.json")
	path := filepath.Join(root, "managed") + string(os.PathSeparator) + ".." + string(os.PathSeparator) + filepath.Base(outside)
	store := NewMemoryStore(path)
	if err := store.Add("must not escape", "test", "security"); !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("Add traversal error = %v, want fs.ErrInvalid", err)
	}
	if _, err := os.Lstat(outside); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("traversal target exists: %v", err)
	}
}

func TestMemoryStoreStorageDoesNotOverwriteCorruptInput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.json")
	want := []byte("{not-json")
	if err := os.WriteFile(path, want, 0o600); err != nil {
		t.Fatal(err)
	}
	store := NewMemoryStore(path)
	if err := store.Add("must not overwrite", "test", "security"); err == nil {
		t.Fatal("Add unexpectedly replaced corrupt memory storage")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("corrupt storage changed: got %q, want %q", got, want)
	}
}

func TestMemoryStoreAtomicPublishNeverExposesPartialJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.json")
	store := NewMemoryStore(path)
	if err := store.Add("seed", "test", "project"); err != nil {
		t.Fatal(err)
	}

	stop := make(chan struct{})
	readerStarted := make(chan struct{})
	readerErr := make(chan error, 1)
	var reader sync.WaitGroup
	reader.Add(1)
	go func() {
		defer reader.Done()
		close(readerStarted)
		for {
			select {
			case <-stop:
				return
			default:
			}
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			var wrapper struct {
				Memories []Memory `json:"memories"`
			}
			if err := json.Unmarshal(data, &wrapper); err != nil {
				select {
				case readerErr <- err:
				default:
				}
				return
			}
		}
	}()
	<-readerStarted
	for i := 0; i < 8; i++ {
		if err := store.Add("atomic fact", "test", "project"); err != nil {
			close(stop)
			reader.Wait()
			t.Fatal(err)
		}
	}
	close(stop)
	reader.Wait()
	select {
	case err := <-readerErr:
		t.Fatalf("reader observed partial JSON: %v", err)
	default:
	}
}

func TestMemoryStoreConcurrentPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.json")
	store := NewMemoryStore(path)
	const writers = 10
	start := make(chan struct{})
	errs := make(chan error, writers)
	var wait sync.WaitGroup
	for i := 0; i < writers; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			errs <- store.Add("concurrent fact", "test", "project")
		}()
	}
	close(start)
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	reloaded := NewMemoryStore(path)
	if got := len(reloaded.Memories()); got != writers {
		t.Fatalf("persisted memories = %d, want %d", got, writers)
	}
}

func marshalMemoryFixture(t *testing.T, fact string) []byte {
	t.Helper()
	wrapper := struct {
		Memories []Memory `json:"memories"`
	}{Memories: []Memory{{
		Fact:      fact,
		Source:    "test",
		CreatedAt: time.Unix(1, 0).UTC(),
		Category:  "project",
	}}}
	data, err := json.MarshalIndent(wrapper, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func assertMemoryStorageMode(t *testing.T, path string, want fs.FileMode) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode for %s = %#o, want %#o", path, got, want)
	}
}
