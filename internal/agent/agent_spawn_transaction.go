package agent

import (
	"errors"
	"strings"
	"time"

	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
)

const agentSpawnRollbackReason = "spawn_rollback"

// rollbackRegisteredAgentSession compensates a retained-session registration
// that was never published as a usable teammate. It terminalizes the durable
// audit record, removes every live lookup/alias/lineage entry, stops the serve
// goroutine (which runs its owned cleanup exactly once), and removes a pristine
// worktree. A terminal audit record/output file may remain as evidence, but no
// runnable task, session, alias, MCP lease, or worktree is left behind.
func (m *BackgroundTaskManager) rollbackRegisteredAgentSession(agentID string, session *backgroundAgentSession) error {
	if m == nil || session == nil {
		return nil
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil
	}

	task := session.task
	if task != nil {
		task.mu.Lock()
		now := time.Now().UTC()
		task.Status = "cancelled"
		task.Outcome = agentcontract.RunOutcomeCancelled
		task.TerminalReason = agentSpawnRollbackReason
		task.QueuedPrompts = 0
		task.QueueReason = ""
		code := -1
		task.ExitCode = &code
		task.FinishedAt = &now
		record := task.recordLocked()
		task.mu.Unlock()
		m.persistRecordForTask(task, record)
	}

	m.mu.Lock()
	if m.sessions[agentID] == session {
		delete(m.sessions, agentID)
	}
	if task == nil || m.tasks[agentID] == task {
		delete(m.tasks, agentID)
	}
	delete(m.trustedAgentResumes, agentID)
	for alias, target := range m.aliases {
		if target == agentID {
			delete(m.aliases, alias)
		}
	}
	delete(m.children, agentID)
	for parent, children := range m.children {
		delete(children, agentID)
		if len(children) == 0 {
			delete(m.children, parent)
		}
	}
	m.mu.Unlock()

	session.cancel()
	<-session.done
	_, worktreeErr := cleanupAgentWorktreeIfClean(session.metadataSnapshot())
	m.notifySnapshotSubscribers()
	return errors.Join(worktreeErr)
}
