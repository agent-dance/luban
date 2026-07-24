package tools

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestP0RuntimePrivateStoresUsePrivateModesAndTightenLegacyFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not enforced on Windows")
	}
	root := t.TempDir()

	storeDir := filepath.Join(root, ".claude", "runtime-tasks")
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	store := NewRuntimeTaskStore(root)
	assertPrivateRuntimeMode(t, storeDir, 0o700)
	record := RuntimeTaskRecord{ID: "legacy-agent", Type: backgroundTaskTypeLocalAgent, Status: "completed", StartedAt: time.Now().UTC()}
	legacyBody, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.path(record.ID), legacyBody, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.Get(record.ID); !ok {
		t.Fatal("legacy runtime task was not loaded")
	}
	assertPrivateRuntimeMode(t, store.path(record.ID), 0o600)
	assertPrivateRuntimeMode(t, store.lockPath(record.ID), 0o600)

	lifecycle := NewRuntimeLifecycle(root)
	if err := lifecycle.Publish(context.Background(), RuntimeLifecycleEvent{Type: LifecycleTaskCreated, EntityID: "private-task"}); err != nil {
		t.Fatal(err)
	}
	assertPrivateRuntimeMode(t, filepath.Dir(lifecycle.path), 0o700)
	assertPrivateRuntimeMode(t, lifecycle.path, 0o600)
	assertPrivateRuntimeMode(t, lifecycle.lockPath(), 0o600)

	outputDir := filepath.Join(root, ".claude", "task-output")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(outputDir, "private-agent.output")
	if err := os.WriteFile(outputPath, []byte("legacy"), 0o644); err != nil {
		t.Fatal(err)
	}
	writer, err := newRotatingFileWriter(outputPath, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.WriteString("-updated"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	assertPrivateRuntimeMode(t, outputDir, 0o700)
	assertPrivateRuntimeMode(t, outputPath, 0o600)
	rotating, err := newRotatingFileWriter(outputPath, 4)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rotating.Write([]byte("next")); err != nil {
		t.Fatal(err)
	}
	if err := rotating.Close(); err != nil {
		t.Fatal(err)
	}
	assertPrivateRuntimeMode(t, outputPath+".1", 0o600)
	assertPrivateRuntimeMode(t, outputPath, 0o600)

	transcriptBase := filepath.Join(root, "transcripts", "agent.jsonl")
	if err := os.MkdirAll(filepath.Dir(transcriptBase), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_AGENT_TRANSCRIPT", transcriptBase)
	transcriptPath := agentTranscriptPathForRun("private-run")
	if err := os.WriteFile(transcriptPath, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	transcript, closeTranscript := openAgentTranscriptWriterForRun("private-run")
	if transcript == nil || closeTranscript == nil {
		t.Fatal("private transcript did not open")
	}
	if err := writeAgentTranscriptRecord(transcript, map[string]any{"type": "test"}); err != nil {
		t.Fatal(err)
	}
	closeTranscript()
	// Explicit harness overrides remain user-managed; only the transcript leaf
	// is tightened.
	assertPrivateRuntimeMode(t, filepath.Dir(transcriptPath), 0o755)
	assertPrivateRuntimeMode(t, transcriptPath, 0o600)

	t.Setenv("CLAUDE_AGENT_TRANSCRIPT", "")
	managedTranscript, closeManaged := openAgentTranscriptWriterForRun("managed-private-run")
	if managedTranscript == nil || closeManaged == nil {
		t.Fatal("managed transcript did not open")
	}
	managedPath := agentTranscriptPathFromWriter(managedTranscript)
	closeManaged()
	assertPrivateRuntimeMode(t, filepath.Dir(managedPath), 0o700)
	assertPrivateRuntimeMode(t, managedPath, 0o600)
}

func TestP0RuntimePrivateStoresRejectSymlinkAndNonRegularPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink setup requires additional privileges on Windows")
	}

	t.Run("runtime task record and lock", func(t *testing.T) {
		root := t.TempDir()
		store := NewRuntimeTaskStore(root)
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
		if err := store.Save(RuntimeTaskRecord{ID: "linked", Type: backgroundTaskTypeLocalAgent}); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("symlink Save error = %v, want fs.ErrInvalid", err)
		}
		assertPrivateRuntimeContents(t, outside, "outside", 0o644)

		if err := os.Symlink(outside, store.lockPath("locked")); err != nil {
			t.Fatal(err)
		}
		if err := store.Save(RuntimeTaskRecord{ID: "locked", Type: backgroundTaskTypeLocalAgent}); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("symlink lock error = %v, want fs.ErrInvalid", err)
		}
		assertPrivateRuntimeContents(t, outside, "outside", 0o644)
	})

	t.Run("hard linked runtime files", func(t *testing.T) {
		root := t.TempDir()
		store := NewRuntimeTaskStore(root)
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
		if err := store.Save(RuntimeTaskRecord{ID: "linked", Type: backgroundTaskTypeLocalAgent}); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("hard-linked Save error = %v, want fs.ErrInvalid", err)
		}
		assertPrivateRuntimeContents(t, outside, "outside", 0o644)

		outputDir := filepath.Join(root, ".claude", "task-output")
		if err := ensurePrivateRuntimeDirectory(outputDir); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(outside, filepath.Join(outputDir, "linked.output")); err != nil {
			t.Fatal(err)
		}
		if _, err := newRotatingFileWriter(filepath.Join(outputDir, "linked.output"), 1024); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("hard-linked output error = %v, want fs.ErrInvalid", err)
		}
		assertPrivateRuntimeContents(t, outside, "outside", 0o644)
	})

	t.Run("runtime directory symlink", func(t *testing.T) {
		root := t.TempDir()
		outside := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, ".claude"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(root, ".claude", "runtime-tasks")); err != nil {
			t.Fatal(err)
		}
		store := NewRuntimeTaskStore(root)
		if err := store.Save(RuntimeTaskRecord{ID: "escape", Type: backgroundTaskTypeLocalAgent}); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("symlink directory Save error = %v, want fs.ErrInvalid", err)
		}
		if _, err := os.Stat(filepath.Join(outside, "escape.json")); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("symlink directory received a record: %v", err)
		}
	})

	t.Run("runtime ancestor symlink", func(t *testing.T) {
		root := t.TempDir()
		outside := t.TempDir()
		if err := os.Mkdir(filepath.Join(outside, "runtime-tasks"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(root, ".claude")); err != nil {
			t.Fatal(err)
		}
		store := NewRuntimeTaskStore(root)
		if err := store.Save(RuntimeTaskRecord{ID: "escape", Type: backgroundTaskTypeLocalAgent}); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("ancestor symlink Save error = %v, want fs.ErrInvalid", err)
		}
		if _, err := os.Stat(filepath.Join(outside, "runtime-tasks", "escape.json")); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("ancestor symlink received a record: %v", err)
		}
	})

	t.Run("lifecycle and output", func(t *testing.T) {
		root := t.TempDir()
		outside := filepath.Join(t.TempDir(), "outside")
		if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
			t.Fatal(err)
		}
		lifecycle := NewRuntimeLifecycle(root)
		if err := os.Symlink(outside, lifecycle.path); err != nil {
			t.Fatal(err)
		}
		if _, err := lifecycle.Events(); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("lifecycle symlink error = %v, want fs.ErrInvalid", err)
		}
		assertPrivateRuntimeContents(t, outside, "outside", 0o644)

		outputDir := filepath.Join(root, ".claude", "task-output")
		if err := ensurePrivateRuntimeDirectory(outputDir); err != nil {
			t.Fatal(err)
		}
		outputPath := filepath.Join(outputDir, "linked.output")
		if err := os.Symlink(outside, outputPath); err != nil {
			t.Fatal(err)
		}
		if _, err := newRotatingFileWriter(outputPath, 1024); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("output symlink open error = %v, want fs.ErrInvalid", err)
		}
		if _, err := readBackgroundTaskOutput(outputPath, 1024); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("output symlink read error = %v, want fs.ErrInvalid", err)
		}
		assertPrivateRuntimeContents(t, outside, "outside", 0o644)
	})

	t.Run("transcript leaf and directory symlink", func(t *testing.T) {
		root := t.TempDir()
		outsideRoot := t.TempDir()
		outside := filepath.Join(outsideRoot, "outside.jsonl")
		if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
			t.Fatal(err)
		}

		base := filepath.Join(root, "transcripts", "agent.jsonl")
		if err := os.MkdirAll(filepath.Dir(base), 0o700); err != nil {
			t.Fatal(err)
		}
		t.Setenv("CLAUDE_AGENT_TRANSCRIPT", base)
		path := agentTranscriptPathForRun("linked")
		if err := os.Symlink(outside, path); err != nil {
			t.Fatal(err)
		}
		if writer, _ := openAgentTranscriptWriterForRun("linked"); writer != nil {
			t.Fatal("transcript leaf symlink was accepted")
		}
		assertPrivateRuntimeContents(t, outside, "outside", 0o644)

		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(filepath.Dir(base)); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outsideRoot, filepath.Dir(base)); err != nil {
			t.Fatal(err)
		}
		if writer, _ := openAgentTranscriptWriterForRun("ancestor"); writer != nil {
			t.Fatal("transcript directory symlink was accepted")
		}
		if _, err := os.Stat(filepath.Join(outsideRoot, "agent.ancestor.jsonl")); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("transcript escaped through directory symlink: %v", err)
		}
	})

	t.Run("non regular leaf", func(t *testing.T) {
		root := t.TempDir()
		store := NewRuntimeTaskStore(root)
		if err := os.Mkdir(store.path("directory"), 0o700); err != nil {
			t.Fatal(err)
		}
		if store.Exists("directory") {
			t.Fatal("directory was accepted as a runtime task record")
		}
		if err := store.Save(RuntimeTaskRecord{ID: "directory", Type: backgroundTaskTypeLocalAgent}); !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("directory record error = %v, want fs.ErrInvalid", err)
		}
	})
}

func TestP0RuntimeStorageRejectsTraversalAndUntrustedOutputPath(t *testing.T) {
	root := t.TempDir()
	store := NewRuntimeTaskStore(root)
	for _, id := range []string{"../outside", "nested/task", `nested\task`, ".", "..", " leading", "trailing ", "bad\nline"} {
		if err := store.Save(RuntimeTaskRecord{ID: id, Type: backgroundTaskTypeLocalAgent}); !errors.Is(err, fs.ErrInvalid) {
			t.Errorf("Save(%q) error = %v, want fs.ErrInvalid", id, err)
		}
		if _, ok := store.Get(id); ok {
			t.Errorf("Get(%q) accepted traversal ID", id)
		}
	}

	forgedID := "forged-agent"
	forgedPath := store.path(forgedID)
	forged := RuntimeTaskRecord{
		ID: forgedID, Type: backgroundTaskTypeLocalAgent, Status: "completed",
		OutputPath: filepath.Join(root, "..", "outside-secret"), StartedAt: time.Now().UTC(),
	}
	body, err := json.Marshal(forged)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(forgedPath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, ok := store.Get(forgedID)
	if !ok {
		t.Fatal("forged legacy record was not decoded")
	}
	if loaded.OutputPath != store.outputPath(forgedID) {
		t.Fatalf("untrusted output path survived: %q", loaded.OutputPath)
	}
	if first, second := store.outputPath("agent@team"), store.outputPath("agent-team"); first == second {
		t.Fatalf("sanitizer-colliding task IDs share output path %q", first)
	}

	manager := NewBackgroundTaskManager(root)
	t.Cleanup(manager.Shutdown)
	if _, _, err := manager.RegisterAgentSessionFromRecord(RuntimeTaskRecord{ID: "../escape"}, nil, agentSessionMetadata{}, nil, nil); !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("restore traversal error = %v, want fs.ErrInvalid", err)
	}

	t.Setenv("CLAUDE_AGENT_TRANSCRIPT", "../outside.jsonl")
	if path := agentTranscriptPathForRun("run"); path != "" {
		t.Fatalf("relative transcript traversal was accepted: %q", path)
	}
}

func TestP0AgentTranscriptCarriesRunIdentityAndWritesOneJSONLRecord(t *testing.T) {
	firstRunID := agentRunID("same-agent", 1)
	secondRunID := agentRunID("same-agent", 1)
	if firstRunID == secondRunID {
		t.Fatalf("independent agent runs reused ID %q", firstRunID)
	}
	t.Setenv("CLAUDE_AGENT_TRANSCRIPT", "")
	if firstPath, secondPath := agentTranscriptPathForRun(firstRunID), agentTranscriptPathForRun(secondRunID); firstPath == secondPath ||
		!strings.Contains(firstPath, agentTranscriptProcessNamespace) || !strings.Contains(secondPath, agentTranscriptProcessNamespace) {
		t.Fatalf("default transcript paths are not process/run scoped: %q %q", firstPath, secondPath)
	}

	root := t.TempDir()
	t.Setenv("CLAUDE_AGENT_TRANSCRIPT", filepath.Join(root, "agent.jsonl"))
	w, closeTranscript := openAgentTranscriptWriterForRunIdentity("run-7", agentTranscriptIdentity{
		SessionID: "session-7", ContextEpoch: "42", ActorID: "agent-7", ActorType: "agent", RunID: "run-7",
	})
	if w == nil || closeTranscript == nil {
		t.Fatal("transcript writer did not open")
	}
	if err := writeAgentTranscriptRecord(w, map[string]any{"type": "assistant", "message": "done"}); err != nil {
		t.Fatal(err)
	}
	path := agentTranscriptPathFromWriter(w)
	closeTranscript()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("transcript records = %d, want 1: %q", len(lines), data)
	}
	var record map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &record); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{
		"sessionId": "session-7", "contextEpoch": "42", "actorId": "agent-7", "actorType": "agent", "runId": "run-7",
	} {
		if record[key] != want {
			t.Errorf("%s = %#v, want %q", key, record[key], want)
		}
	}

	counting := &countingTranscriptWriter{}
	if err := writeAgentTranscriptRecord(counting, map[string]any{"type": "terminal"}); err != nil {
		t.Fatal(err)
	}
	if counting.writes != 1 {
		t.Fatalf("JSONL record used %d writes, want 1", counting.writes)
	}
	if !strings.HasSuffix(counting.data.String(), "\n") {
		t.Fatalf("JSONL record has no trailing newline: %q", counting.data.String())
	}
	short := &shortTranscriptWriter{limit: 3}
	if err := writeAgentTranscriptRecord(short, map[string]any{"type": "short"}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(short.data.String(), "\n") {
		t.Fatalf("short writes did not complete one JSONL record: %q", short.data.String())
	}
}

type countingTranscriptWriter struct {
	writes int
	data   strings.Builder
}

func (w *countingTranscriptWriter) Write(data []byte) (int, error) {
	w.writes++
	return w.data.Write(data)
}

type shortTranscriptWriter struct {
	limit int
	data  strings.Builder
}

func (w *shortTranscriptWriter) Write(data []byte) (int, error) {
	if len(data) > w.limit {
		data = data[:w.limit]
	}
	return w.data.Write(data)
}

func assertPrivateRuntimeMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode(%s) = %04o, want %04o", path, got, want)
	}
}

func assertPrivateRuntimeContents(t *testing.T, path, want string, wantMode os.FileMode) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Fatalf("contents(%s) = %q, want %q", path, data, want)
	}
	assertPrivateRuntimeMode(t, path, wantMode)
}
