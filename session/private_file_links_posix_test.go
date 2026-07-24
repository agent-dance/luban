//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package session

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/agent-dance/luban/types"
)

func TestFileStoreRejectsMultiplyLinkedManagedFiles(t *testing.T) {
	const sessionID = "multiply-linked"

	t.Run("compatibility transcript", func(t *testing.T) {
		store := newLinkedFileTestStore(t, sessionID)
		linkManagedFileForTest(t, store.sessionPath(sessionID))

		if _, err := store.Load(sessionID); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("Load error = %v, want fs.ErrInvalid", err)
		}
		if live, err := storeHasLiveSession(store, sessionID); live || !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("storeHasLiveSession = (%v, %v), want (false, fs.ErrInvalid)", live, err)
		}
		if got := store.TranscriptPath(sessionID); got != "" {
			t.Fatalf("TranscriptPath = %q, want empty", got)
		}
	})

	t.Run("metadata", func(t *testing.T) {
		store := newLinkedFileTestStore(t, sessionID)
		linkManagedFileForTest(t, store.metaPath(sessionID))

		if _, err := store.GetMeta(sessionID); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("GetMeta error = %v, want fs.ErrInvalid", err)
		}
	})

	t.Run("current manifest", func(t *testing.T) {
		store := newLinkedFileTestStore(t, sessionID)
		linkManagedFileForTest(t, store.manifestPath(sessionID))

		if _, err := store.Load(sessionID); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("Load error = %v, want fs.ErrInvalid", err)
		}
	})

	t.Run("history lock", func(t *testing.T) {
		store := newLinkedFileTestStore(t, sessionID)
		linkManagedFileForTest(t, filepath.Join(store.dir, "."+sessionID+".history.lock"))

		if _, err := store.Load(sessionID); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("Load error = %v, want fs.ErrInvalid", err)
		}
	})
}

func TestFileStoreRejectsFIFOWithoutBlocking(t *testing.T) {
	store := NewFileStore(t.TempDir())
	const sessionID = "fifo-session"
	if err := syscall.Mkfifo(store.sessionPath(sessionID), 0o600); err != nil {
		t.Skipf("FIFOs are unavailable: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := store.Load(sessionID)
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("Load FIFO error = %v, want fs.ErrInvalid", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Load blocked while opening a FIFO")
	}
}

func TestFileStoreRejectsMultiplyLinkedImmutableHistory(t *testing.T) {
	const sessionID = "multiply-linked-history"
	tests := []struct {
		name string
		path func(*FileStore, CompactionManifestV2) string
	}{
		{
			name: "model context view",
			path: func(store *FileStore, manifest CompactionManifestV2) string {
				name, err := digestFileName(manifest.ModelContextView.Digest, ".jsonl")
				if err != nil {
					t.Fatal(err)
				}
				return filepath.Join(store.viewDir(sessionID), name)
			},
		},
		{
			name: "audit segment",
			path: func(store *FileStore, manifest CompactionManifestV2) string {
				name, err := digestFileName(manifest.AuditSegments[0].Digest, ".json")
				if err != nil {
					t.Fatal(err)
				}
				return filepath.Join(store.auditDir(sessionID), name)
			},
		},
		{
			name: "audit transcript",
			path: func(store *FileStore, manifest CompactionManifestV2) string {
				name, err := digestFileName(manifest.AuditTranscript.Digest, ".jsonl")
				if err != nil {
					t.Fatal(err)
				}
				return filepath.Join(store.auditTranscriptDir(sessionID), name)
			},
		},
		{
			name: "immutable manifest",
			path: func(store *FileStore, manifest CompactionManifestV2) string {
				name, err := digestFileName(manifest.Digest, ".json")
				if err != nil {
					t.Fatal(err)
				}
				return filepath.Join(store.immutableManifestDir(sessionID), name)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newLinkedFileTestStore(t, sessionID)
			manifest, err := store.GetCompactionManifest(sessionID)
			if err != nil {
				t.Fatal(err)
			}
			linkManagedFileForTest(t, test.path(store, manifest))

			if _, err := store.Load(sessionID); !errors.Is(err, ErrCorruptSessionHistory) {
				t.Fatalf("Load error = %v, want ErrCorruptSessionHistory", err)
			}
		})
	}
}

func TestRepositoryForkRejectsMultiplyLinkedSourceAndArtifact(t *testing.T) {
	newSource := func(t *testing.T, id string) (*Repository, *FileStore, Ref) {
		t.Helper()
		repo := NewRepository(t.TempDir())
		projectDir := repo.ProjectDirForCWD(t.TempDir())
		store := repo.StoreForProjectDir(projectDir)
		if err := store.Save(id, []types.Message{types.UserMessage("fork source")}); err != nil {
			t.Fatal(err)
		}
		return repo, store, Ref{ID: id, ProjectDir: projectDir}
	}

	t.Run("source transcript", func(t *testing.T) {
		repo, store, source := newSource(t, "linked-fork-source")
		linkManagedFileForTest(t, store.sessionPath(source.ID))
		if _, err := repo.Fork(source, []types.Message{types.UserMessage("fork source")}); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("Fork error = %v, want fs.ErrInvalid", err)
		}
	})

	t.Run("referenced artifact", func(t *testing.T) {
		repo, store, source := newSource(t, "linked-fork-artifact")
		artifact := filepath.Join(store.ArtifactsDir(source.ID), "tool-results", "evidence.txt")
		if err := os.MkdirAll(filepath.Dir(artifact), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(artifact, []byte("private evidence"), 0o600); err != nil {
			t.Fatal(err)
		}
		linkManagedFileForTest(t, artifact)
		messages := []types.Message{types.UserMessage("evidence: " + artifact)}
		if _, err := repo.Fork(source, messages); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("Fork error = %v, want fs.ErrInvalid", err)
		}
	})
}

func newLinkedFileTestStore(t *testing.T, sessionID string) *FileStore {
	t.Helper()
	store := NewFileStore(t.TempDir())
	if err := store.Save(sessionID, []types.Message{types.UserMessage("private session")}); err != nil {
		t.Fatal(err)
	}
	return store
}

func linkManagedFileForTest(t *testing.T, path string) {
	t.Helper()
	alias := filepath.Join(t.TempDir(), filepath.Base(path)+".alias")
	if err := os.Link(path, alias); err != nil {
		t.Skipf("hard links are unavailable: %v", err)
	}
}
