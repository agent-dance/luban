//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package session

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestMemoryStoreStorageRejectsSymlinkNonRegularAndMultiplyLinkedPaths(t *testing.T) {
	t.Run("memory symlink", func(t *testing.T) {
		dir := t.TempDir()
		outside := filepath.Join(t.TempDir(), "outside-memory.json")
		want := marshalMemoryFixture(t, "outside")
		if err := os.WriteFile(outside, want, 0o600); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "memory.json")
		if err := os.Symlink(outside, path); err != nil {
			t.Skipf("symlinks are unavailable: %v", err)
		}
		store := NewMemoryStore(path)
		if err := store.Add("escaped", "test", "security"); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("Add symlink error = %v, want fs.ErrInvalid", err)
		}
		got, err := os.ReadFile(outside)
		if err != nil || string(got) != string(want) {
			t.Fatalf("outside memory changed: %q, %v", got, err)
		}
	})

	t.Run("memory directory", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "memory.json")
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
		store := NewMemoryStore(path)
		if err := store.Add("invalid", "test", "security"); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("Add directory error = %v, want fs.ErrInvalid", err)
		}
	})

	t.Run("memory FIFO", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "memory.json")
		if err := syscall.Mkfifo(path, 0o600); err != nil {
			t.Skipf("FIFOs are unavailable: %v", err)
		}
		done := make(chan *MemoryStore, 1)
		go func() {
			done <- NewMemoryStore(path)
		}()
		select {
		case store := <-done:
			if err := store.Add("invalid", "test", "security"); !errors.Is(err, fs.ErrInvalid) {
				t.Fatalf("Add FIFO error = %v, want fs.ErrInvalid", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("NewMemoryStore blocked while opening a FIFO")
		}
	})

	t.Run("multiply linked memory", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "memory.json")
		want := marshalMemoryFixture(t, "linked")
		if err := os.WriteFile(path, want, 0o600); err != nil {
			t.Fatal(err)
		}
		alias := filepath.Join(t.TempDir(), "memory-alias.json")
		if err := os.Link(path, alias); err != nil {
			t.Skipf("hard links are unavailable: %v", err)
		}
		store := NewMemoryStore(path)
		if err := store.Add("invalid", "test", "security"); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("Add multiply-linked error = %v, want fs.ErrInvalid", err)
		}
		for _, candidate := range []string{path, alias} {
			got, err := os.ReadFile(candidate)
			if err != nil || string(got) != string(want) {
				t.Fatalf("linked memory %s changed: %q, %v", candidate, got, err)
			}
		}
	})

	t.Run("parent directory symlink", func(t *testing.T) {
		outsideDir := t.TempDir()
		link := filepath.Join(t.TempDir(), "memory-dir-link")
		if err := os.Symlink(outsideDir, link); err != nil {
			t.Skipf("symlinks are unavailable: %v", err)
		}
		store := NewMemoryStore(filepath.Join(link, "memory.json"))
		if err := store.Add("escaped", "test", "security"); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("Add through directory symlink error = %v, want fs.ErrInvalid", err)
		}
		if _, err := os.Lstat(filepath.Join(outsideDir, "memory.json")); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("directory symlink target was written: %v", err)
		}
	})
}

func TestMemoryStoreStorageSwapRaceCannotModifyOutsideTarget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.json")
	store := NewMemoryStore(path)
	if err := store.Add("seed", "test", "project"); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside-memory.json")
	want := []byte("outside sentinel")
	if err := os.WriteFile(outside, want, 0o600); err != nil {
		t.Fatal(err)
	}

	stop := make(chan struct{})
	attackerDone := make(chan struct{})
	go func() {
		defer close(attackerDone)
		for {
			select {
			case <-stop:
				return
			default:
			}
			_ = os.Remove(path)
			_ = os.Symlink(outside, path)
			_ = os.Remove(path)
			_ = os.Link(outside, path)
		}
	}()

	for i := 0; i < 8; i++ {
		_ = store.Add("race fact", "test", "security")
	}
	close(stop)
	<-attackerDone
	_ = os.Remove(path)
	got, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("outside target changed: got %q, want %q", got, want)
	}
}

func TestMemoryStoreStorageParentSwapPublishesOnlyThroughHeldDirectory(t *testing.T) {
	root := t.TempDir()
	managed := filepath.Join(root, "managed")
	path := filepath.Join(managed, "memory.json")
	store := NewMemoryStore(path)
	if err := store.Add("seed", "test", "project"); err != nil {
		t.Fatal(err)
	}

	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "memory.json")
	want := []byte("outside sentinel")
	if err := os.WriteFile(outside, want, 0o600); err != nil {
		t.Fatal(err)
	}
	parked := filepath.Join(root, "managed-parked")
	store.storageBeforePublish = func() {
		if err := os.Rename(managed, parked); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outsideDir, managed); err != nil {
			t.Fatal(err)
		}
	}

	if err := store.Add("held-directory fact", "test", "security"); !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("Add after parent swap error = %v, want fs.ErrInvalid", err)
	}
	got, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("outside target changed: got %q, want %q", got, want)
	}
	parkedData, err := os.ReadFile(filepath.Join(parked, "memory.json"))
	if err != nil {
		t.Fatal(err)
	}
	var wrapper struct {
		Memories []Memory `json:"memories"`
	}
	if err := json.Unmarshal(parkedData, &wrapper); err != nil {
		t.Fatalf("held-directory publication is corrupt: %v", err)
	}
	if len(wrapper.Memories) != 2 {
		t.Fatalf("held-directory memories = %d, want 2", len(wrapper.Memories))
	}
}
