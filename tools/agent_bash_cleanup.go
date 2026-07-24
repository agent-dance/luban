package tools

// agent_bash_cleanup.go mirrors the TS killShellTasksForAgent behavior in
// src/tools/AgentTool/runAgent.ts. When an agent terminates (success, abort,
// timeout) the parent walks the BackgroundTaskManager, picks every shell-type
// task tagged with the agent's id, and stops them. Without this sweep an
// agent that ran `npm run dev &` leaves the dev server alive forever, eating
// the port across CLI restarts and confusing every subsequent run.

import (
	"strings"
)

// TagShellTaskForAgent attaches the spawning agent's id to a freshly-started
// shell background task. The bash tool calls this immediately after
// StartShellTask returns when a sub-agent context is in scope.
func (m *BackgroundTaskManager) TagShellTaskForAgent(taskID, agentID string) {
	if m == nil {
		return
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" || taskID == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	task, ok := m.tasks[taskID]
	if !ok || task == nil {
		return
	}
	task.mu.Lock()
	task.OwnerAgentID = agentID
	task.mu.Unlock()
}

// CleanupShellTasksForAgent stops every running shell-type background task
// owned by agentID. Returns the IDs that were actively cancelled. Tasks
// already finished are left untouched. Mirrors TS killShellTasksForAgent.
func (m *BackgroundTaskManager) CleanupShellTasksForAgent(agentID string) []string {
	if m == nil {
		return nil
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil
	}
	var victims []string
	m.mu.Lock()
	for id, task := range m.tasks {
		if task == nil {
			continue
		}
		task.mu.Lock()
		owner := task.OwnerAgentID
		taskType := task.Type
		status := task.Status
		task.mu.Unlock()
		if owner != agentID {
			continue
		}
		if taskType != backgroundTaskTypeLocalBash {
			continue
		}
		if status != "running" {
			continue
		}
		victims = append(victims, id)
	}
	m.mu.Unlock()

	cancelled := make([]string, 0, len(victims))
	for _, id := range victims {
		if _, err := m.Stop(id); err == nil {
			cancelled = append(cancelled, id)
		}
	}
	return cancelled
}
