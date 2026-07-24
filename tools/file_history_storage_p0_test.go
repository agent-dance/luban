//go:build darwin || linux

package tools

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestFileHistoryStorageP0MigratesLegacyModesWithoutLosingContent(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".luban-code", "file-history")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	tracked := filepath.Join(t.TempDir(), "target<&>.txt")
	name := historyFileName(tracked)
	historyPath := filepath.Join(root, name)
	lockPath := historyPath + ".lock"
	legacy := FileHistoryEntry{
		Path: tracked, Before: "legacy <before>", After: "legacy &after", Hash: "legacy-hash",
		Tool: "Edit", Ts: 101, EditID: "legacy-edit",
	}
	legacyLine := fmt.Sprintf("{\"path\":%q,\"before\":%q,\"after\":%q,\"hash\":%q,\"tool\":%q,\"ts\":%d,\"editId\":%q}\n",
		legacy.Path, legacy.Before, legacy.After, legacy.Hash, legacy.Tool, legacy.Ts, legacy.EditID)
	if err := os.WriteFile(historyPath, []byte(legacyLine), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(historyPath, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(lockPath, 0o644); err != nil {
		t.Fatal(err)
	}

	store := NewFileHistoryStore(root)
	got, err := store.ListEdits(tracked)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != legacy {
		t.Fatalf("legacy history changed during migration: %#v", got)
	}
	assertPrivateFileHistoryMode(t, root, 0o700)
	assertPrivateFileHistoryMode(t, historyPath, 0o600)
	assertPrivateFileHistoryMode(t, lockPath, 0o600)

	current := FileHistoryEntry{
		Path: tracked, Before: "完整 before <&>", After: "完整 after <&>", Tool: "Write", Ts: 102, EditID: "current-edit",
	}
	if err := store.TrackEdit(current); err != nil {
		t.Fatal(err)
	}
	got, err = store.ListEdits(tracked)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("history entries = %d, want 2: %#v", len(got), got)
	}
	if got[1].Path != current.Path || got[1].Before != current.Before || got[1].After != current.After || got[1].EditID != current.EditID {
		t.Fatalf("complete file-history record was not preserved: %#v", got[1])
	}
	assertPrivateFileHistoryMode(t, root, 0o700)
	assertPrivateFileHistoryMode(t, historyPath, 0o600)
	assertPrivateFileHistoryMode(t, lockPath, 0o600)
}

func TestFileHistoryStorageP0RejectsTraversalAndDirectorySymlinks(t *testing.T) {
	entry := testPrivateFileHistoryEntry(filepath.Join(t.TempDir(), "target.txt"), "entry")

	t.Run("lexical traversal", func(t *testing.T) {
		base := t.TempDir()
		root := base + string(os.PathSeparator) + "untrusted" + string(os.PathSeparator) + ".." + string(os.PathSeparator) + "history"
		err := NewFileHistoryStore(root).TrackEdit(entry)
		if !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("TrackEdit traversal error = %v, want fs.ErrInvalid", err)
		}
		if _, err := os.Lstat(filepath.Join(base, "history")); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("traversal target was created: %v", err)
		}
	})

	t.Run("history root symlink", func(t *testing.T) {
		base := t.TempDir()
		target := filepath.Join(base, "target")
		if err := os.Mkdir(target, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(target, 0o755); err != nil {
			t.Fatal(err)
		}
		root := filepath.Join(base, "history")
		if err := os.Symlink(target, root); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		err := NewFileHistoryStore(root).TrackEdit(entry)
		if !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("TrackEdit root symlink error = %v, want fs.ErrInvalid", err)
		}
		assertPrivateFileHistoryMode(t, target, 0o755)
		entries, err := os.ReadDir(target)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Fatalf("root symlink target was written: %v", entries)
		}
	})

	t.Run("parent symlink", func(t *testing.T) {
		base := t.TempDir()
		target := filepath.Join(base, "target")
		if err := os.Mkdir(target, 0o755); err != nil {
			t.Fatal(err)
		}
		parent := filepath.Join(base, ".luban-code")
		if err := os.Symlink(target, parent); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		err := NewFileHistoryStore(filepath.Join(parent, "file-history")).TrackEdit(entry)
		if !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("TrackEdit parent symlink error = %v, want fs.ErrInvalid", err)
		}
		if _, err := os.Lstat(filepath.Join(target, "file-history")); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("parent symlink target was written: %v", err)
		}
	})
}

func TestFileHistoryStorageP0RejectsUnsafeHistoryEntries(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, root, path string) string
	}{
		{
			name: "symlink",
			setup: func(t *testing.T, _, path string) string {
				target := filepath.Join(t.TempDir(), "symlink-target")
				if err := os.WriteFile(target, []byte("do not change"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(target, 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
				return target
			},
		},
		{
			name: "hardlink",
			setup: func(t *testing.T, _, path string) string {
				target := filepath.Join(t.TempDir(), "hardlink-target")
				if err := os.WriteFile(target, []byte("do not change"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(target, 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Link(target, path); err != nil {
					t.Skipf("hard links unavailable: %v", err)
				}
				return target
			},
		},
		{
			name: "fifo",
			setup: func(t *testing.T, _, path string) string {
				if err := syscall.Mkfifo(path, 0o644); err != nil {
					t.Skipf("FIFOs unavailable: %v", err)
				}
				return ""
			},
		},
		{
			name: "directory",
			setup: func(t *testing.T, _, path string) string {
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatal(err)
				}
				return ""
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "history")
			if err := os.Mkdir(root, 0o700); err != nil {
				t.Fatal(err)
			}
			tracked := filepath.Join(t.TempDir(), "target.txt")
			path := filepath.Join(root, historyFileName(tracked))
			target := test.setup(t, root, path)
			store := NewFileHistoryStore(root)

			done := make(chan error, 1)
			go func() {
				done <- store.TrackEdit(testPrivateFileHistoryEntry(tracked, test.name))
			}()
			select {
			case err := <-done:
				if !errors.Is(err, fs.ErrInvalid) {
					t.Fatalf("TrackEdit error = %v, want fs.ErrInvalid", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("TrackEdit blocked on a non-regular history entry")
			}

			if target != "" {
				content, err := os.ReadFile(target)
				if err != nil {
					t.Fatal(err)
				}
				if string(content) != "do not change" {
					t.Fatalf("unsafe target content changed: %q", content)
				}
				assertPrivateFileHistoryMode(t, target, 0o644)
			}
		})
	}
}

func TestFileHistoryStorageP0RejectsUnsafeLockEntries(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, path string) string
	}{
		{
			name: "symlink",
			setup: func(t *testing.T, path string) string {
				target := filepath.Join(t.TempDir(), "lock-target")
				if err := os.WriteFile(target, []byte("lock target"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(target, 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
				return target
			},
		},
		{
			name: "hardlink",
			setup: func(t *testing.T, path string) string {
				target := filepath.Join(t.TempDir(), "lock-target")
				if err := os.WriteFile(target, []byte("lock target"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(target, 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Link(target, path); err != nil {
					t.Skipf("hard links unavailable: %v", err)
				}
				return target
			},
		},
		{
			name: "fifo",
			setup: func(t *testing.T, path string) string {
				if err := syscall.Mkfifo(path, 0o644); err != nil {
					t.Skipf("FIFOs unavailable: %v", err)
				}
				return ""
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "history")
			if err := os.Mkdir(root, 0o700); err != nil {
				t.Fatal(err)
			}
			tracked := filepath.Join(t.TempDir(), "target.txt")
			lockPath := filepath.Join(root, historyFileName(tracked)+".lock")
			target := test.setup(t, lockPath)

			done := make(chan error, 1)
			go func() {
				done <- NewFileHistoryStore(root).TrackEdit(testPrivateFileHistoryEntry(tracked, test.name))
			}()
			select {
			case err := <-done:
				if !errors.Is(err, fs.ErrInvalid) {
					t.Fatalf("TrackEdit error = %v, want fs.ErrInvalid", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("TrackEdit blocked on a non-regular lock entry")
			}

			if target != "" {
				content, err := os.ReadFile(target)
				if err != nil {
					t.Fatal(err)
				}
				if string(content) != "lock target" {
					t.Fatalf("unsafe lock target changed: %q", content)
				}
				assertPrivateFileHistoryMode(t, target, 0o644)
			}
		})
	}
}

func TestFileHistoryConcurrentAtomicAppendP0(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".luban-code", "file-history")
	tracked := filepath.Join(t.TempDir(), "target.txt")
	const workers = 10
	const perWorker = 4
	stores := make([]*FileHistoryStore, workers)
	for i := range stores {
		stores[i] = NewFileHistoryStore(root)
	}

	start := make(chan struct{})
	errs := make(chan error, workers*perWorker)
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		worker := worker
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for sequence := 0; sequence < perWorker; sequence++ {
				id := fmt.Sprintf("worker-%02d-entry-%02d", worker, sequence)
				payload := id + ":" + strings.Repeat(string(rune('a'+worker)), 2048)
				errs <- stores[worker].TrackEdit(FileHistoryEntry{
					Path: tracked, Before: "before-" + payload, After: "after-" + payload,
					Tool: "Edit", Ts: int64(worker*perWorker + sequence + 1), EditID: id,
				})
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent TrackEdit: %v", err)
		}
	}

	got, err := NewFileHistoryStore(root).ListEdits(tracked)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != workers*perWorker {
		t.Fatalf("history entries = %d, want %d", len(got), workers*perWorker)
	}
	seen := make(map[string]FileHistoryEntry, len(got))
	for _, entry := range got {
		if _, duplicate := seen[entry.EditID]; duplicate {
			t.Fatalf("duplicate or torn edit ID %q", entry.EditID)
		}
		seen[entry.EditID] = entry
	}
	for worker := 0; worker < workers; worker++ {
		for sequence := 0; sequence < perWorker; sequence++ {
			id := fmt.Sprintf("worker-%02d-entry-%02d", worker, sequence)
			entry, ok := seen[id]
			if !ok {
				t.Fatalf("lost concurrent history entry %q", id)
			}
			payload := id + ":" + strings.Repeat(string(rune('a'+worker)), 2048)
			if entry.Path != tracked || entry.Before != "before-"+payload || entry.After != "after-"+payload {
				t.Fatalf("torn concurrent history entry %q: %#v", id, entry)
			}
		}
	}
	assertPrivateFileHistoryMode(t, root, 0o700)
	assertPrivateFileHistoryMode(t, filepath.Join(root, historyFileName(tracked)), 0o600)
	assertPrivateFileHistoryMode(t, filepath.Join(root, historyFileName(tracked)+".lock"), 0o600)
}

func TestFileHistoryConcurrentProcessWritersP0(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".luban-code", "file-history")
	tracked := filepath.Join(t.TempDir(), "target.txt")
	const writers = 6
	const perWriter = 3
	commands := make([]*exec.Cmd, writers)
	outputs := make([]bytes.Buffer, writers)
	for writer := 0; writer < writers; writer++ {
		command := exec.Command(os.Args[0], "-test.run=^TestFileHistoryConcurrentProcessWriterHelperP0$")
		command.Env = append(os.Environ(),
			"LUBAN_FILE_HISTORY_HELPER=1",
			"LUBAN_FILE_HISTORY_ROOT="+root,
			"LUBAN_FILE_HISTORY_TRACKED="+tracked,
			fmt.Sprintf("LUBAN_FILE_HISTORY_WRITER=%d", writer),
		)
		command.Stderr = &outputs[writer]
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		commands[writer] = command
	}
	for writer, command := range commands {
		if err := command.Wait(); err != nil {
			t.Fatalf("history writer %d failed: %v\n%s", writer, err, outputs[writer].String())
		}
	}

	got, err := NewFileHistoryStore(root).ListEdits(tracked)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != writers*perWriter {
		t.Fatalf("cross-process history entries = %d, want %d", len(got), writers*perWriter)
	}
	seen := make(map[string]bool, len(got))
	for _, entry := range got {
		if entry.Path != tracked || entry.Before == "" || entry.After == "" || seen[entry.EditID] {
			t.Fatalf("lost, duplicate, or torn cross-process entry: %#v", entry)
		}
		seen[entry.EditID] = true
	}
}

func TestFileHistoryConcurrentProcessWriterHelperP0(t *testing.T) {
	if os.Getenv("LUBAN_FILE_HISTORY_HELPER") != "1" {
		return
	}
	root := os.Getenv("LUBAN_FILE_HISTORY_ROOT")
	tracked := os.Getenv("LUBAN_FILE_HISTORY_TRACKED")
	writer := os.Getenv("LUBAN_FILE_HISTORY_WRITER")
	store := NewFileHistoryStore(root)
	for sequence := 0; sequence < 3; sequence++ {
		id := fmt.Sprintf("process-%s-entry-%d", writer, sequence)
		if err := store.TrackEdit(FileHistoryEntry{
			Path: tracked, Before: "before-" + id, After: "after-" + id,
			Tool: "Edit", Ts: int64(sequence + 1), EditID: id,
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFileHistoryStorageP0RejectsPartialLegacyTailWithoutDeletingIt(t *testing.T) {
	root := filepath.Join(t.TempDir(), "history")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	tracked := filepath.Join(t.TempDir(), "target.txt")
	path := filepath.Join(root, historyFileName(tracked))
	partial := []byte(`{"path":"incomplete"`)
	if err := os.WriteFile(path, partial, 0o644); err != nil {
		t.Fatal(err)
	}
	err := NewFileHistoryStore(root).TrackEdit(testPrivateFileHistoryEntry(tracked, "new"))
	if !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("TrackEdit partial legacy tail error = %v, want fs.ErrInvalid", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(partial) {
		t.Fatalf("partial legacy history was changed: %q", got)
	}
	assertPrivateFileHistoryMode(t, path, 0o600)
}

func testPrivateFileHistoryEntry(path, id string) FileHistoryEntry {
	return FileHistoryEntry{
		Path: path, Before: "before-" + id, After: "after-" + id,
		Tool: "Edit", Ts: 1, EditID: id,
	}
}

func assertPrivateFileHistoryMode(t *testing.T, path string, want fs.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %#o, want %#o", path, got, want)
	}
}
