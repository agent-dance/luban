package session

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/agent-dance/luban/types"
)

func TestFileStoreSaveMetaRejectsCorruptMetadataWithoutOverwrite(t *testing.T) {
	store := NewFileStore(t.TempDir())
	const sessionID = "corrupt-save-meta"
	if err := store.Save(sessionID, []types.Message{types.UserMessage("original")}); err != nil {
		t.Fatal(err)
	}
	metaPath := store.metaPath(sessionID)
	wantMeta := []byte("{broken metadata\n")
	if err := os.WriteFile(metaPath, wantMeta, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := store.SaveMeta(sessionID, SessionMeta{CWD: "/new/cwd"}); err == nil {
		t.Fatal("SaveMeta unexpectedly overwrote corrupt metadata")
	}
	gotMeta, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotMeta, wantMeta) {
		t.Fatalf("metadata changed after rejected SaveMeta: got %q want %q", gotMeta, wantMeta)
	}
}

func TestFileStoreSaveRejectsCorruptMetadataBeforeTranscriptMutation(t *testing.T) {
	store := NewFileStore(t.TempDir())
	const sessionID = "corrupt-save"
	if err := store.Save(sessionID, []types.Message{types.UserMessage("original")}); err != nil {
		t.Fatal(err)
	}
	transcriptPath := store.manifestPath(sessionID)
	wantTranscript, err := os.ReadFile(transcriptPath)
	if err != nil {
		t.Fatal(err)
	}
	wantMeta := []byte("{broken metadata\n")
	if err := os.WriteFile(store.metaPath(sessionID), wantMeta, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := store.Save(sessionID, []types.Message{types.UserMessage("replacement")}); err == nil {
		t.Fatal("Save unexpectedly accepted corrupt metadata")
	}
	gotTranscript, err := os.ReadFile(transcriptPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotTranscript, wantTranscript) {
		t.Fatalf("transcript mutated before corrupt metadata was rejected: got %q want %q", gotTranscript, wantTranscript)
	}
	gotMeta, err := os.ReadFile(store.metaPath(sessionID))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotMeta, wantMeta) {
		t.Fatalf("corrupt metadata changed: got %q want %q", gotMeta, wantMeta)
	}
}

func TestFileStoreDeletePersistsTombstoneAndSurfacesAllCleanupErrors(t *testing.T) {
	store := NewFileStore(t.TempDir())
	const sessionID = "delete-cleanup-errors"
	if err := store.Save(sessionID, []types.Message{types.UserMessage("delete me")}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(store.ArtifactsDir(sessionID), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.ArtifactsDir(sessionID), "evidence"), []byte("proof"), 0o644); err != nil {
		t.Fatal(err)
	}

	metaErr := errors.New("injected meta cleanup failure")
	artifactErr := errors.New("injected artifact cleanup failure")
	store.removeFile = func(path string) error {
		if path == store.metaPath(sessionID) {
			return metaErr
		}
		return os.Remove(path)
	}
	store.removeTree = func(path string) error {
		if path == store.ArtifactsDir(sessionID) {
			return artifactErr
		}
		return os.RemoveAll(path)
	}

	err := store.Delete(sessionID)
	if !errors.Is(err, metaErr) || !errors.Is(err, artifactErr) {
		t.Fatalf("Delete error = %v, want joined meta and artifact failures", err)
	}
	deleted, checkErr := store.IsDeleted(sessionID)
	if checkErr != nil {
		t.Fatal(checkErr)
	}
	if !deleted {
		t.Fatal("durable deletion marker was not retained after cleanup failure")
	}
	if _, err := store.Load(sessionID); !errors.Is(err, ErrSessionDeleted) {
		t.Fatalf("Load after logical deletion = %v, want ErrSessionDeleted", err)
	}
	if err := store.Save(sessionID, []types.Message{types.UserMessage("resurrect")}); !errors.Is(err, ErrSessionDeleted) {
		t.Fatalf("Save after logical deletion = %v, want ErrSessionDeleted", err)
	}

	store.removeFile = os.Remove
	store.removeTree = os.RemoveAll
	if err := store.Delete(sessionID); err != nil {
		t.Fatalf("retry cleanup: %v", err)
	}
	for _, path := range []string{store.manifestPath(sessionID), store.metaPath(sessionID), store.ArtifactsDir(sessionID)} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("cleanup retry left %s: %v", path, err)
		}
	}
	if deleted, err := store.IsDeleted(sessionID); err != nil || !deleted {
		t.Fatalf("permanent marker after retry = %v, %v", deleted, err)
	}
}

func TestFileStoreDeleteMarkerFailurePreservesAllHistory(t *testing.T) {
	store := NewFileStore(t.TempDir())
	const sessionID = "delete-marker-failure"
	if err := store.Save(sessionID, []types.Message{types.UserMessage("preserve me")}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(store.ArtifactsDir(sessionID), 0o755); err != nil {
		t.Fatal(err)
	}
	paths := []string{store.manifestPath(sessionID), store.metaPath(sessionID), store.ArtifactsDir(sessionID)}
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			t.Fatal(err)
		}
	}
	markerErr := errors.New("injected marker commit failure")
	store.writeDeleteMarker = func(string) error { return markerErr }

	if err := store.Delete(sessionID); !errors.Is(err, markerErr) {
		t.Fatalf("Delete error = %v, want marker failure", err)
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("marker failure mutated %s: %v", path, err)
		}
	}
	if deleted, err := store.IsDeleted(sessionID); err != nil || deleted {
		t.Fatalf("deleted after marker failure = %v, %v", deleted, err)
	}
	if messages, err := store.Load(sessionID); err != nil || len(messages) != 1 {
		t.Fatalf("history not readable after marker failure: %v, %#v", err, messages)
	}
}

func TestFileStoreSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)

	msgs := []types.Message{
		types.UserMessage("hello"),
		types.AssistantMessage("world"),
	}

	if err := store.Save("test-1", msgs); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := store.Load("test-1")
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if len(loaded) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(loaded))
	}
	if loaded[0].GetText() != "hello" {
		t.Errorf("expected 'hello', got '%s'", loaded[0].GetText())
	}
}

func TestFileStoreSaveAtomic(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)

	msgs := []types.Message{types.UserMessage("data")}
	if err := store.Save("atomic-test", msgs); err != nil {
		t.Fatal(err)
	}

	matches, _ := filepath.Glob(filepath.Join(dir, "*.tmp"))
	if len(matches) > 0 {
		t.Errorf("expected no tmp files, found: %v", matches)
	}

	if _, err := os.Stat(store.manifestPath("atomic-test")); err != nil {
		t.Errorf("expected session file to exist: %v", err)
	}
	if _, err := os.Stat(store.metaPath("atomic-test")); err != nil {
		t.Errorf("expected session meta file to exist: %v", err)
	}
}

func TestFileStoreTranscriptPathRequiresReadableTranscript(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)

	if got := store.TranscriptPath("missing"); got != "" {
		t.Fatalf("expected no transcript path for missing session, got %q", got)
	}

	if err := store.Save("with-transcript", []types.Message{types.UserMessage("hello")}); err != nil {
		t.Fatal(err)
	}
	path := store.TranscriptPath("with-transcript")
	if path == "" {
		t.Fatal("expected transcript path after save")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected readable transcript path %q: %v", path, err)
	}
}

func TestFileStoreList(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)

	store.Save("s1", []types.Message{types.UserMessage("a")})
	store.Save("s2", []types.Message{types.UserMessage("b")})

	sessions, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestFileStoreMetadataPersistsAndSearches(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)
	msgs := []types.Message{
		types.UserMessage("fix login bug please"),
		types.AssistantMessage("working on it"),
	}
	if err := store.Save("sess-a", msgs); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveMeta("sess-a", SessionMeta{
		Title:     "fix-login-bug",
		CWD:       "/repo/app",
		GitBranch: "feature/login",
	}); err != nil {
		t.Fatal(err)
	}

	meta, err := store.GetMeta("sess-a")
	if err != nil {
		t.Fatal(err)
	}
	if meta.Title != "fix-login-bug" {
		t.Fatalf("expected title to persist, got %q", meta.Title)
	}
	if meta.MessageCount != 2 {
		t.Fatalf("expected message count 2, got %d", meta.MessageCount)
	}
	if meta.PreviewText == "" {
		t.Fatal("expected derived preview text")
	}

	results, err := store.Search(SearchOptions{Query: "login", AllProjects: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ID != "sess-a" {
		t.Fatalf("expected search to find sess-a, got %+v", results)
	}
}

func TestFileStoreCacheLineageDefaultsAndSurvivesPartialMetadataUpdates(t *testing.T) {
	store := NewFileStore(t.TempDir())
	const sessionID = "cache-lineage-session"
	if err := store.Save(sessionID, []types.Message{types.UserMessage("hello")}); err != nil {
		t.Fatal(err)
	}

	meta, err := store.GetMeta(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if meta.SchemaVersion != sessionMetaSchemaV1 {
		t.Fatalf("new session schema = %q, want %q", meta.SchemaVersion, sessionMetaSchemaV1)
	}
	if meta.CacheLineageID != sessionID {
		t.Fatalf("new session CacheLineageID = %q, want %q", meta.CacheLineageID, sessionID)
	}

	const inheritedLineage = "root-cache-lineage"
	if err := store.SaveMeta(sessionID, SessionMeta{CacheLineageID: inheritedLineage}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveMeta(sessionID, SessionMeta{Title: "renamed"}); err != nil {
		t.Fatal(err)
	}
	meta, err = store.GetMeta(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if meta.CacheLineageID != inheritedLineage {
		t.Fatalf("partial metadata update changed CacheLineageID to %q, want %q", meta.CacheLineageID, inheritedLineage)
	}
}

func TestFileStoreRejectsObsoleteSessionMetadata(t *testing.T) {
	const timestamp = `"2026-07-25T00:00:00Z"`
	tests := map[string]string{
		"missing lineage": `{"schema_version":"session-meta/v1","id":"obsolete","created_at":` + timestamp + `,"updated_at":` + timestamp + `}`,
		"mismatched ID":   `{"schema_version":"session-meta/v1","id":"other","cache_lineage_id":"obsolete","created_at":` + timestamp + `,"updated_at":` + timestamp + `}`,
		"unknown field":   `{"schema_version":"session-meta/v1","id":"obsolete","cache_lineage_id":"obsolete","created_at":` + timestamp + `,"updated_at":` + timestamp + `,"legacy_value":true}`,
		"old presentation": `{"schema_version":"session-meta/v1","id":"obsolete","cache_lineage_id":"obsolete","created_at":` + timestamp + `,"updated_at":` + timestamp +
			`,"presentation":{"permission_mode":"plan","input_draft":"stale"}}`,
		"missing fingerprints": `{"schema_version":"session-meta/v1","id":"obsolete","cache_lineage_id":"obsolete","created_at":` + timestamp + `,"updated_at":` + timestamp + `}`,
	}
	for name, payload := range tests {
		t.Run(name, func(t *testing.T) {
			store := NewFileStore(t.TempDir())
			const sessionID = "obsolete"
			if err := store.Save(sessionID, []types.Message{types.UserMessage("canonical")}); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(store.metaPath(sessionID), []byte(payload), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := store.GetMeta(sessionID); err == nil {
				t.Fatal("obsolete metadata unexpectedly loaded")
			}
		})
	}
}

func TestFileStoreLoadsAndLazilyMigratesKnownLegacyMetadata(t *testing.T) {
	store := NewFileStore(t.TempDir())
	const sessionID = "legacy"
	if err := store.Save(sessionID, []types.Message{types.UserMessage("legacy transcript")}); err != nil {
		t.Fatal(err)
	}
	legacy := `{
  "id":"legacy",
  "cache_lineage_id":"legacy",
  "title":"legacy title",
  "created_at":"2026-07-25T00:00:00Z",
  "updated_at":"2026-07-25T01:00:00Z",
  "usage":{"input_tokens":17,"output_tokens":5},
  "presentation":{"version":3,"input_cursor_set":true,"permission_mode":"ask"},
  "activities":[{"id":"obsolete-projection"}],
  "evidence":[{"observation_id":"obsolete-evidence"}]
}`
	if err := os.WriteFile(store.metaPath(sessionID), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}

	meta, err := store.GetMeta(sessionID)
	if err != nil {
		t.Fatalf("load legacy metadata: %v", err)
	}
	if meta.SchemaVersion != sessionMetaSchemaV1 || meta.Title != "legacy title" ||
		meta.Usage == nil || meta.Usage.InputTokens != 17 || meta.Presentation == nil ||
		meta.Presentation.PermissionMode != "ask" {
		t.Fatalf("legacy metadata was not normalized: %+v", meta)
	}
	if meta.FirstWriterBuild != nil || meta.LastWriterBuild != nil {
		t.Fatalf("legacy read fabricated writer fingerprints: %+v", meta)
	}

	if err := store.SaveMeta(sessionID, SessionMeta{Title: "migrated title"}); err != nil {
		t.Fatalf("lazy migration: %v", err)
	}
	migrated, err := os.ReadFile(store.metaPath(sessionID))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(migrated, &fields); err != nil {
		t.Fatal(err)
	}
	if _, exists := fields["activities"]; exists {
		t.Fatal("legacy activities survived migration")
	}
	if _, exists := fields["evidence"]; exists {
		t.Fatal("legacy evidence survived migration")
	}
	meta, err = store.GetMeta(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Title != "migrated title" || meta.FirstWriterBuild == nil || meta.LastWriterBuild == nil {
		t.Fatalf("migrated metadata is incomplete: %+v", meta)
	}
}

func TestFileStoreListIsolatesCorruptAndIncompatibleMetadata(t *testing.T) {
	store := NewFileStore(t.TempDir())
	for _, id := range []string{"good", "corrupt", "future"} {
		if err := store.Save(id, []types.Message{types.UserMessage(id)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(store.metaPath("corrupt"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.metaPath("future"), []byte(`{"schema_version":"session-meta/v2"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	items, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "good" {
		t.Fatalf("isolated list = %+v, want only good session", items)
	}
	if _, err := store.GetMeta("corrupt"); !errors.Is(err, ErrCorruptSessionMetadata) {
		t.Fatalf("precise corrupt lookup = %v", err)
	}
	if _, err := store.GetMeta("future"); !errors.Is(err, ErrIncompatibleSessionMetadata) {
		t.Fatalf("precise incompatible lookup = %v", err)
	}
}

func TestFileStoreListDoesNotHideMetadataPathSafetyFailure(t *testing.T) {
	store := NewFileStore(t.TempDir())
	const sessionID = "unsafe-meta"
	if err := store.Save(sessionID, []types.Message{types.UserMessage("safe transcript")}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(store.metaPath(sessionID)); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(store.metaPath(sessionID), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := store.List(); !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("List metadata path safety error = %v, want fs.ErrInvalid", err)
	}
}

func TestFileStoreSearchHonorsProjectScope(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)
	store.Save("same", []types.Message{types.UserMessage("same repo")})
	store.SaveMeta("same", SessionMeta{CWD: "/repo/a", Title: "same"})
	store.Save("other", []types.Message{types.UserMessage("other repo")})
	store.SaveMeta("other", SessionMeta{CWD: "/repo/b", Title: "other"})

	results, err := store.Search(SearchOptions{CurrentCWD: "/repo/a", AllProjects: false})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ID != "same" {
		t.Fatalf("expected only same-project session, got %+v", results)
	}
}

func TestFileStoreRename(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)
	store.Save("rename-me", []types.Message{types.UserMessage("hello")})
	if err := store.Rename("rename-me", "new-title"); err != nil {
		t.Fatal(err)
	}
	meta, err := store.GetMeta("rename-me")
	if err != nil {
		t.Fatal(err)
	}
	if meta.Title != "new-title" {
		t.Fatalf("expected renamed title, got %q", meta.Title)
	}
}

func TestFileStoreLatest(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)

	store.Save("old", []types.Message{types.UserMessage("a")})
	time.Sleep(10 * time.Millisecond)
	store.Save("new", []types.Message{types.UserMessage("b")})

	latest, err := store.Latest()
	if err != nil {
		t.Fatal(err)
	}
	if latest != "new" {
		t.Errorf("expected 'new', got '%s'", latest)
	}
}

func TestFileStoreLatestEmpty(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)

	_, err := store.Latest()
	if err == nil {
		t.Error("expected error for empty store")
	}
}

func TestFileStoreLoadNonexistent(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)

	_, err := store.Load("nonexistent")
	if err == nil {
		t.Error("expected error loading nonexistent session")
	}
}

func TestFileStoreUsesPrivateModesAndTightensManagedFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not enforced on Windows")
	}
	root := filepath.Join(t.TempDir(), "sessions")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	store := NewFileStore(root)
	assertStorageMode(t, root, 0o700)

	const id = "private-session"
	if err := store.Save(id, []types.Message{types.UserMessage("private")}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{store.manifestPath(id), store.metaPath(id)} {
		assertStorageMode(t, path, 0o600)
		if err := os.Chmod(path, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.Load(id); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetMeta(id); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{store.manifestPath(id), store.metaPath(id)} {
		assertStorageMode(t, path, 0o600)
	}
}

func TestFileStoreRejectsTraversalSessionIDs(t *testing.T) {
	root := t.TempDir()
	store := NewFileStore(root)
	outside := filepath.Join(filepath.Dir(root), "outside.jsonl")

	for _, id := range []string{"../outside", "nested/session", ".", " session "} {
		if err := store.Save(id, []types.Message{types.UserMessage("escape")}); !errors.Is(err, fs.ErrInvalid) {
			t.Errorf("Save(%q) error = %v, want fs.ErrInvalid", id, err)
		}
		if _, err := store.Load(id); !errors.Is(err, fs.ErrInvalid) {
			t.Errorf("Load(%q) error = %v, want fs.ErrInvalid", id, err)
		}
		if _, err := store.GetMeta(id); !errors.Is(err, fs.ErrInvalid) {
			t.Errorf("GetMeta(%q) error = %v, want fs.ErrInvalid", id, err)
		}
		if err := store.SaveMeta(id, SessionMeta{Title: "escape"}); !errors.Is(err, fs.ErrInvalid) {
			t.Errorf("SaveMeta(%q) error = %v, want fs.ErrInvalid", id, err)
		}
		if got := store.TranscriptPath(id); got != "" {
			t.Errorf("TranscriptPath(%q) = %q, want empty", id, got)
		}
	}
	if _, err := os.Lstat(outside); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("traversal created %q: %v", outside, err)
	}
	if got := store.ArtifactsDir("../outside"); filepath.Dir(got) != root {
		t.Fatalf("invalid artifacts path escaped store: %q", got)
	}
}

func TestFileStoreRejectsSymlinkAndNonRegularManagedPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink setup requires additional privileges on Windows")
	}
	t.Run("manifest symlink", func(t *testing.T) {
		root := t.TempDir()
		store := NewFileStore(root)
		outside := filepath.Join(t.TempDir(), "outside.jsonl")
		if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, store.manifestPath("linked")); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Load("linked"); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("Load symlink error = %v, want fs.ErrInvalid", err)
		}
		if err := store.Save("linked", []types.Message{types.UserMessage("replace")}); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("Save symlink error = %v, want fs.ErrInvalid", err)
		}
		got, err := os.ReadFile(outside)
		if err != nil || string(got) != "outside" {
			t.Fatalf("outside target changed: %q, %v", got, err)
		}
	})

	t.Run("non-regular manifest", func(t *testing.T) {
		store := NewFileStore(t.TempDir())
		if err := os.Mkdir(store.manifestPath("directory"), 0o700); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Load("directory"); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("Load directory error = %v, want fs.ErrInvalid", err)
		}
	})

	t.Run("metadata symlink", func(t *testing.T) {
		store := NewFileStore(t.TempDir())
		if err := store.Save("linked-meta", []types.Message{types.UserMessage("seed")}); err != nil {
			t.Fatal(err)
		}
		outside := filepath.Join(t.TempDir(), "outside-meta.json")
		if err := os.WriteFile(outside, []byte(`{"id":"outside"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(store.metaPath("linked-meta")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, store.metaPath("linked-meta")); err != nil {
			t.Fatal(err)
		}
		if _, err := store.GetMeta("linked-meta"); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("GetMeta symlink error = %v, want fs.ErrInvalid", err)
		}
	})

	t.Run("deletion marker symlink", func(t *testing.T) {
		store := NewFileStore(t.TempDir())
		outside := filepath.Join(t.TempDir(), "outside-marker")
		if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, store.tombstonePath("linked-marker")); err != nil {
			t.Fatal(err)
		}
		if _, err := store.IsDeleted("linked-marker"); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("IsDeleted symlink error = %v, want fs.ErrInvalid", err)
		}
	})

	t.Run("store root symlink", func(t *testing.T) {
		target := t.TempDir()
		link := filepath.Join(t.TempDir(), "sessions-link")
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		store := NewFileStore(link)
		if err := store.Save("linked-root", []types.Message{types.UserMessage("blocked")}); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("Save through symlink root error = %v, want fs.ErrInvalid", err)
		}
		if _, err := os.Lstat(filepath.Join(target, "linked-root.jsonl")); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("symlink root write escaped: %v", err)
		}
	})
}

func assertStorageMode(t *testing.T, path string, want fs.FileMode) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode for %s = %#o, want %#o", path, got, want)
	}
}
