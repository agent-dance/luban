package tools

// agent_todos_leak_cleanup.go mirrors the TS runAgent.ts todos-leak
// cleanup: when a sub-agent finishes, prune any todo entries it
// authored that referenced the agent's transient state. Without this
// sweep the parent's todo list slowly fills with stale entries from
// finished agents — UI clutter and confusion about what's actually
// pending.
//
// In TS each todo carries an agent_id stamped at create time and the
// cleanup sweeps by that field. The Go TodoItem currently does not
// have a typed AgentID field (added by a future wave); until then the
// cleanup matches by an "[agent:<id>]" tag the TodoWriteTool can
// inject into the Content / ActiveForm string. Items without the tag
// are left alone — they belong to the parent session.

import (
	"fmt"
	"strings"
)

// AgentTodoStore is the minimal contract CleanupAgentTodos needs.
// *TodoStore satisfies it via the wrapper below.
type AgentTodoStore interface {
	LoadAndSave(mutator func(prior []TodoItem) ([]TodoItem, error)) ([]TodoItem, []TodoItem, error)
}

// AgentTodoTag returns the canonical "[agent:<id>]" tag the cleanup
// matches against. Tools that author todos on behalf of a sub-agent
// should append this tag to Content (or ActiveForm) so the cleanup
// can find them after the agent terminates.
func AgentTodoTag(agentID string) string {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return ""
	}
	return fmt.Sprintf("[agent:%s]", agentID)
}

// CleanupAgentTodosForAgent removes every todo item authored by
// agentID. Returns the count of pruned entries and any error from the
// underlying store. Pass empty agentID to no-op.
func CleanupAgentTodosForAgent(store AgentTodoStore, agentID string) (int, error) {
	agentID = strings.TrimSpace(agentID)
	tag := AgentTodoTag(agentID)
	if store == nil || tag == "" {
		return 0, nil
	}
	pruned := 0
	_, _, err := store.LoadAndSave(func(prior []TodoItem) ([]TodoItem, error) {
		out := make([]TodoItem, 0, len(prior))
		for _, t := range prior {
			if strings.Contains(t.Content, tag) || strings.Contains(t.ActiveForm, tag) {
				pruned++
				continue
			}
			out = append(out, t)
		}
		return out, nil
	})
	return pruned, err
}
