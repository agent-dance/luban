package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckAgentMemorySnapshot_NoneWhenNoSnapshot(t *testing.T) {
	cwd := t.TempDir()
	mem := t.TempDir()
	rep := CheckAgentMemorySnapshot("foo", cwd, mem)
	if rep.Verdict != AgentMemorySnapshotNone {
		t.Fatalf("expected none, got %q", rep.Verdict)
	}
}

func TestCheckAgentMemorySnapshot_InitializeWhenMemoryEmpty(t *testing.T) {
	cwd := t.TempDir()
	mem := t.TempDir()
	snapshotDir := filepath.Join(cwd, ".claude", agentMemorySnapshotBase, sanitizeAgentMemoryName("foo"))
	if err := os.MkdirAll(snapshotDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(snapshotDir, "MEMORY.md"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	meta := agentMemorySnapshotMeta{UpdatedAt: "2026-01-01T00:00:00Z"}
	data, _ := json.Marshal(meta)
	if err := os.WriteFile(filepath.Join(snapshotDir, agentMemorySnapshotJSON), data, 0o644); err != nil {
		t.Fatalf("write meta: %v", err)
	}
	rep := CheckAgentMemorySnapshot("foo", cwd, mem)
	if rep.Verdict != AgentMemorySnapshotInitialize {
		t.Fatalf("expected initialize, got %q", rep.Verdict)
	}
}

func TestReplaceAgentMemoryFromSnapshot_PrunesOrphans(t *testing.T) {
	cwd := t.TempDir()
	mem := t.TempDir()
	snapshotDir := filepath.Join(cwd, ".claude", agentMemorySnapshotBase, sanitizeAgentMemoryName("foo"))
	if err := os.MkdirAll(snapshotDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Snapshot contains exactly one new MEMORY.md.
	if err := os.WriteFile(filepath.Join(snapshotDir, "MEMORY.md"), []byte("new"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	meta := agentMemorySnapshotMeta{UpdatedAt: "2026-02-01T00:00:00Z"}
	data, _ := json.Marshal(meta)
	if err := os.WriteFile(filepath.Join(snapshotDir, agentMemorySnapshotJSON), data, 0o644); err != nil {
		t.Fatalf("write meta: %v", err)
	}
	// memoryDir already has an orphan + a stale MEMORY.md.
	if err := os.WriteFile(filepath.Join(mem, "STALE.md"), []byte("orphan"), 0o644); err != nil {
		t.Fatalf("seed orphan: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mem, "MEMORY.md"), []byte("old"), 0o644); err != nil {
		t.Fatalf("seed memory: %v", err)
	}

	copied := ReplaceAgentMemoryFromSnapshot("foo", cwd, mem)
	if copied != 1 {
		t.Fatalf("expected 1 file copied, got %d", copied)
	}
	if _, err := os.Stat(filepath.Join(mem, "STALE.md")); !os.IsNotExist(err) {
		t.Fatalf("orphan STALE.md must be pruned, stat err=%v", err)
	}
	got, err := os.ReadFile(filepath.Join(mem, "MEMORY.md"))
	if err != nil {
		t.Fatalf("read MEMORY.md: %v", err)
	}
	if string(got) != "new" {
		t.Fatalf("MEMORY.md not overwritten with snapshot content, got %q", string(got))
	}
}
