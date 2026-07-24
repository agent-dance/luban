package tools

// agent_memory_snapshot.go exposes verdict-returning + orphan-pruning
// helpers around the agent_memory.go snapshot infrastructure. Mirrors
// the TS checkAgentMemorySnapshot contract from
// src/tools/AgentTool/agentMemorySnapshot.ts:
//
//   "none"          — nothing to do (no snapshot on disk)
//   "initialize"    — memory dir is empty, copy from snapshot
//   "prompt-update" — snapshot has been updated since last sync; ask user
//
// ReplaceAgentMemoryFromSnapshot performs the destructive copy: it
// removes orphaned files (files in memoryDir that no longer exist in
// snapshotDir) before overlaying the snapshot. Without orphan pruning,
// stale topic files survive across versions and continue to influence
// agent behaviour after their definition is gone.

import (
	"os"
	"path/filepath"
	"strings"
)

// AgentMemorySnapshotVerdict is the action a caller should take after
// CheckAgentMemorySnapshot.
type AgentMemorySnapshotVerdict string

const (
	AgentMemorySnapshotNone         AgentMemorySnapshotVerdict = "none"
	AgentMemorySnapshotInitialize   AgentMemorySnapshotVerdict = "initialize"
	AgentMemorySnapshotPromptUpdate AgentMemorySnapshotVerdict = "prompt-update"
)

// CheckAgentMemorySnapshot is the verdict-returning wrapper around the
// existing snapshot machinery. agentType is the canonical name; cwd is
// the project root; memoryDir is the destination directory the agent
// reads its memory from. Returns one of the three verdicts above. The
// non-verdict UpdatedAt slot is populated for prompt-update so the
// caller can render "snapshot from <when>" in the prompt.
type AgentMemorySnapshotReport struct {
	Verdict   AgentMemorySnapshotVerdict
	UpdatedAt string
}

func CheckAgentMemorySnapshot(agentType, cwd, memoryDir string) AgentMemorySnapshotReport {
	snapshotDir := agentMemorySnapshotDir(agentType, cwd)
	meta := readAgentMemorySnapshotMeta(filepath.Join(snapshotDir, agentMemorySnapshotJSON))
	if strings.TrimSpace(meta.UpdatedAt) == "" {
		return AgentMemorySnapshotReport{Verdict: AgentMemorySnapshotNone}
	}
	if !agentMemoryDirHasMarkdown(memoryDir) {
		return AgentMemorySnapshotReport{Verdict: AgentMemorySnapshotInitialize, UpdatedAt: meta.UpdatedAt}
	}
	synced := readAgentMemorySyncedMeta(filepath.Join(memoryDir, agentMemorySyncedJSON))
	if strings.TrimSpace(synced.SyncedFrom) == "" || snapshotTimestampAfter(meta.UpdatedAt, synced.SyncedFrom) {
		return AgentMemorySnapshotReport{Verdict: AgentMemorySnapshotPromptUpdate, UpdatedAt: meta.UpdatedAt}
	}
	return AgentMemorySnapshotReport{Verdict: AgentMemorySnapshotNone}
}

// ReplaceAgentMemoryFromSnapshot does an orphan-pruning copy from the
// snapshot directory into memoryDir. Files present in memoryDir but not
// in the snapshot are removed first. The .snapshot-synced.json marker
// is rewritten to the snapshot's UpdatedAt timestamp.
//
// Returns the number of files copied. If snapshotDir is empty or
// missing the operation is a no-op (returns 0). Errors during deletion
// of individual orphans are swallowed — a stale orphan is unfortunate
// but should not block the snapshot install.
func ReplaceAgentMemoryFromSnapshot(agentType, cwd, memoryDir string) int {
	snapshotDir := agentMemorySnapshotDir(agentType, cwd)
	meta := readAgentMemorySnapshotMeta(filepath.Join(snapshotDir, agentMemorySnapshotJSON))
	if strings.TrimSpace(meta.UpdatedAt) == "" {
		return 0
	}
	snapshotEntries, err := os.ReadDir(snapshotDir)
	if err != nil {
		return 0
	}
	keep := map[string]struct{}{}
	for _, e := range snapshotEntries {
		if e.IsDir() || e.Name() == agentMemorySnapshotJSON {
			continue
		}
		keep[e.Name()] = struct{}{}
	}
	// Prune orphans first.
	if existing, err := os.ReadDir(memoryDir); err == nil {
		for _, e := range existing {
			if e.IsDir() || e.Name() == agentMemorySyncedJSON {
				continue
			}
			if _, kept := keep[e.Name()]; kept {
				continue
			}
			_ = os.Remove(filepath.Join(memoryDir, e.Name()))
		}
	}
	copied := 0
	if err := os.MkdirAll(memoryDir, 0o755); err != nil {
		return 0
	}
	for _, entry := range snapshotEntries {
		if entry.IsDir() || entry.Name() == agentMemorySnapshotJSON {
			continue
		}
		src := filepath.Join(snapshotDir, entry.Name())
		dst := filepath.Join(memoryDir, entry.Name())
		data, err := os.ReadFile(src)
		if err != nil {
			continue
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			continue
		}
		copied++
	}
	writeAgentMemorySyncedMeta(memoryDir, meta.UpdatedAt)
	return copied
}
