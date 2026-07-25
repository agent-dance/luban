package agent

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

	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
	runtimestore "github.com/agent-dance/luban/internal/store/runtime"
	"github.com/agent-dance/luban/internal/store/secureio"
)

func TestPrivateRuntimeOutputAndTranscriptModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not enforced on Windows")
	}
	root := t.TempDir()
	outputDir := filepath.Join(root, ".luban-code", "task-output")
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
	t.Setenv("LUBAN_AGENT_TRANSCRIPT", transcriptBase)
	transcriptPath := agentTranscriptPathForRun("private-run")
	if err := os.WriteFile(transcriptPath, nil, 0o644); err != nil {
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
	assertPrivateRuntimeMode(t, filepath.Dir(transcriptPath), 0o755)
	assertPrivateRuntimeMode(t, transcriptPath, 0o600)

	t.Setenv("LUBAN_AGENT_TRANSCRIPT", "")
	managed, closeManaged := openAgentTranscriptWriterForRun("managed-private-run")
	if managed == nil || closeManaged == nil {
		t.Fatal("managed transcript did not open")
	}
	managedPath := agentTranscriptPathFromWriter(managed)
	closeManaged()
	assertPrivateRuntimeMode(t, filepath.Dir(managedPath), 0o700)
	assertPrivateRuntimeMode(t, managedPath, 0o600)
}

func TestPrivateRuntimeOutputAndTranscriptRejectLinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("link setup requires additional privileges on Windows")
	}
	root := t.TempDir()
	outsideRoot := t.TempDir()
	outside := filepath.Join(outsideRoot, "outside")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	outputDir := filepath.Join(root, ".luban-code", "task-output")
	if err := secureio.EnsurePrivateRuntimeDirectory(outputDir); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(outputDir, "linked.output")
	if err := os.Symlink(outside, outputPath); err != nil {
		t.Fatal(err)
	}
	if _, err := newRotatingFileWriter(outputPath, 1024); !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("output symlink open error = %v, want fs.ErrInvalid", err)
	}
	if _, err := ReadTaskOutput(outputPath, 1024); !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("output symlink read error = %v, want fs.ErrInvalid", err)
	}
	assertPrivateRuntimeContents(t, outside, "outside", 0o644)

	if err := os.Remove(outputPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(outside, outputPath); err != nil {
		t.Skipf("hard links are unavailable: %v", err)
	}
	if _, err := newRotatingFileWriter(outputPath, 1024); !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("hard-linked output error = %v, want fs.ErrInvalid", err)
	}
	assertPrivateRuntimeContents(t, outside, "outside", 0o644)

	base := filepath.Join(root, "transcripts", "agent.jsonl")
	if err := os.MkdirAll(filepath.Dir(base), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LUBAN_AGENT_TRANSCRIPT", base)
	path := agentTranscriptPathForRun("linked")
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}
	if writer, _ := openAgentTranscriptWriterForRun("linked"); writer != nil {
		t.Fatal("transcript leaf symlink was accepted")
	}
	assertPrivateRuntimeContents(t, outside, "outside", 0o644)
}

func TestRuntimeRestoreAndTranscriptRejectTraversal(t *testing.T) {
	root := t.TempDir()
	manager := NewBackgroundTaskManager(root)
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	if _, _, err := manager.registerAgentSessionFromRecord(
		runtimestore.RuntimeTaskRecord{ID: "../escape"}, nil, agentcontract.SessionMetadata{}, nil, nil,
	); !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("restore traversal error = %v, want fs.ErrInvalid", err)
	}
	t.Setenv("LUBAN_AGENT_TRANSCRIPT", "../outside.jsonl")
	if path := agentTranscriptPathForRun("run"); path != "" {
		t.Fatalf("relative transcript traversal was accepted: %q", path)
	}
}

func TestAgentTranscriptCarriesRunIdentityAndWritesOneJSONLRecord(t *testing.T) {
	firstRunID := agentRunID("same-agent", 1)
	secondRunID := agentRunID("same-agent", 1)
	if firstRunID == secondRunID {
		t.Fatalf("independent agent runs reused ID %q", firstRunID)
	}
	t.Setenv("LUBAN_AGENT_TRANSCRIPT", "")
	if firstPath, secondPath := agentTranscriptPathForRun(firstRunID), agentTranscriptPathForRun(secondRunID); firstPath == secondPath ||
		!strings.Contains(firstPath, agentTranscriptProcessNamespace) || !strings.Contains(secondPath, agentTranscriptProcessNamespace) {
		t.Fatalf("default transcript paths are not process/run scoped: %q %q", firstPath, secondPath)
	}

	root := t.TempDir()
	t.Setenv("LUBAN_AGENT_TRANSCRIPT", filepath.Join(root, "agent.jsonl"))
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
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(lines) != 1 {
		t.Fatalf("transcript records = %d, want 1: %q", len(lines), body)
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
	if counting.writes != 1 || !strings.HasSuffix(counting.data.String(), "\n") {
		t.Fatalf("JSONL record was not emitted in one newline-terminated write: writes=%d data=%q", counting.writes, counting.data.String())
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
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != want {
		t.Fatalf("contents(%s) = %q, want %q", path, body, want)
	}
	assertPrivateRuntimeMode(t, path, wantMode)
}
