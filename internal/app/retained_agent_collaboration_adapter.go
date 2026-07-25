package app

import (
	"context"

	agentruntime "github.com/agent-dance/luban/internal/agent"
	toolcollaboration "github.com/agent-dance/luban/internal/tools/collaboration"
	"github.com/agent-dance/luban/skills"
)

// retainedAgentCollaborationAdapter keeps collaboration tools independent of
// the concrete agent runtime while the application owns both implementations.
type retainedAgentCollaborationAdapter struct {
	background *agentruntime.BackgroundTaskManager
	skills     *skills.Manager
}

func newRetainedAgentCollaborationAdapter(
	background *agentruntime.BackgroundTaskManager,
	skillManager *skills.Manager,
) *retainedAgentCollaborationAdapter {
	if background == nil {
		return nil
	}
	return &retainedAgentCollaborationAdapter{background: background, skills: skillManager}
}

func (a *retainedAgentCollaborationAdapter) ResumeAgent(
	ctx context.Context,
	target string,
	prompt string,
) (toolcollaboration.RetainedAgentResume, bool, error) {
	snapshot, handled, err := a.background.QueuePrompt(ctx, target, prompt, a.skills)
	return toolcollaboration.RetainedAgentResume{
		Status:     snapshot.Status,
		OutputPath: snapshot.OutputPath,
	}, handled, err
}

func (a *retainedAgentCollaborationAdapter) AbortAgent(target string) bool {
	return a.background.AbortAgent(target)
}
