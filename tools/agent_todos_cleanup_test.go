package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCleanupAgentTodos_RemovesFile(t *testing.T) {
	root := t.TempDir()
	id := "agent-xyz"
	dir := filepath.Join(root, ".claude", "todos")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, id+".json")
	if err := os.WriteFile(path, []byte("[]"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if !CleanupAgentTodos(root, id) {
		t.Fatalf("expected cleanup to remove file")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file should be gone, stat err=%v", err)
	}
}

func TestCleanupAgentTodos_MissingFileIsNoOp(t *testing.T) {
	root := t.TempDir()
	if CleanupAgentTodos(root, "no-such-agent") {
		t.Fatalf("expected false when no file")
	}
}

func TestCleanupAgentTodos_EmptyAgentIDIgnored(t *testing.T) {
	root := t.TempDir()
	if CleanupAgentTodos(root, "  ") {
		t.Fatalf("blank agent id must not trigger cleanup")
	}
}

func TestCleanupAgentTodosForSummary_HonorsSummaryCWD(t *testing.T) {
	root := t.TempDir()
	id := "agent-task-42"
	dir := filepath.Join(root, ".claude", "todos")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, id+".json")
	if err := os.WriteFile(path, []byte("[]"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cleanupAgentTodosForSummary(agentRunSummary{AgentID: id, CWD: root})
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected file removed, err=%v", err)
	}
}
