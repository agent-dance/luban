package tools

// agent_todos_cleanup.go implements the todos-leak cleanup that the TS
// runAgent.ts performs in its onCleanup hook:
//
//   onCleanup: () => {
//     setAppState(prev => {
//       if (!(agentId in prev.todos)) return prev
//       const { [agentId]: _removed, ...todos } = prev.todos
//       return { ...prev, todos }
//     })
//   }
//
// Without this, every sub-agent that called TodoWrite leaves a key in the
// app-state map (or an orphan JSON file under .claude/todos/) forever — UI
// clutter and confusion about what's actually pending.

import (
	"os"
	"path/filepath"
	"strings"
)

// CleanupAgentTodos removes the on-disk todo file scoped to the given agentID
// inside projectRoot. Returns true when a file was actually removed. Missing
// files are silently ignored. Best-effort: any error is swallowed so a slow
// or read-only filesystem cannot prevent agent finalisation.
func CleanupAgentTodos(projectRoot, agentID string) bool {
	id := strings.TrimSpace(agentID)
	if id == "" {
		return false
	}
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		root = "."
	}
	path := filepath.Join(root, ".claude", "todos", sanitizeTaskPathComponent(id)+".json")
	if _, err := os.Stat(path); err != nil {
		return false
	}
	if err := os.Remove(path); err != nil {
		return false
	}
	// Also clean up any stale lock file from the same scope.
	_ = os.Remove(path + ".lock")
	return true
}

// cleanupAgentTodosForSummary is the finalize-time hook called from
// runSubAgentWithOptions. It picks projectRoot from the run summary's CWD
// (falling back to the process working directory) so the cleanup matches
// whatever scope TodoWrite used during the agent run.
func cleanupAgentTodosForSummary(summary agentRunSummary) {
	root := strings.TrimSpace(summary.CWD)
	if root == "" {
		root, _ = os.Getwd()
	}
	if root == "" {
		return
	}
	CleanupAgentTodos(root, summary.AgentID)
}
