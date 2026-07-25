package agent

import (
	"context"

	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
	"github.com/agent-dance/luban/internal/runtime/skillauthority"
	"github.com/agent-dance/luban/skills"
)

// QueuePrompt resumes a retained agent under the current execution authority.
func (m *BackgroundTaskManager) QueuePrompt(ctx context.Context, target, prompt string, manager *skills.Manager) (agentcontract.TaskSnapshot, bool, error) {
	authority, err := skillauthority.Capture(ctx, manager)
	if err != nil {
		return agentcontract.TaskSnapshot{}, true, err
	}
	return m.queueAgentPromptWithAuthority(ctx, target, prompt, authority, manager)
}
