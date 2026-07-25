package agent

import (
	"context"
	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"

	"github.com/agent-dance/luban/internal/runtime/loop"
	"github.com/agent-dance/luban/registry"
)

func (m *BackgroundTaskManager) RegisterAgentSession(agentID, alias, prompt, description string, input agentcontract.Input, ql *loop.QueryLoop, metadata agentcontract.SessionMetadata, cleanup func(), progress *AgentProgressEmitter, executionContext context.Context) (*backgroundAgentSession, *agentcontract.TaskSnapshot, error) {
	return m.registerAgentSession(agentID, alias, prompt, description, input, ql, metadata, cleanup, progress, nil, executionContext)
}

func (t *AgentTool) createRetainedAgentSession(agentID string, in agentcontract.Input) (*backgroundAgentSession, *agentcontract.TaskSnapshot, error) {
	return t.createRetainedAgentSessionWithOptions(agentID, in, agentLoopOptions{})
}

func (t *AgentTool) buildSubAgentLoop(agentID string, in agentcontract.Input) (agentLoopBundle, error) {
	return t.buildSubAgentLoopWithOptions(agentID, in, agentLoopOptions{})
}

func resolveAgentProfile(raw string, cwd string) (agentProfile, error) {
	return resolveAgentProfileWithOptions(raw, cwd, agentProfileResolveOptions{})
}

func registryForAgentProfile(source *registry.Registry, profile agentProfile) *registry.Registry {
	return registryForAgentProfileWithOptions(source, profile, agentToolRegistryOptions{})
}

func trustAgentResumeForTest(manager *BackgroundTaskManager, agentID string, input agentcontract.Input, metadata agentcontract.SessionMetadata) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.trustedAgentResumes == nil {
		manager.trustedAgentResumes = make(map[string]trustedAgentResumeContext)
	}
	manager.trustedAgentResumes[agentID] = trustedAgentResumeContext{
		Input:    input,
		Metadata: cloneAgentSessionMetadata(metadata),
	}
}

func restoredAgentSessionForTest(manager *BackgroundTaskManager, agentID string) *backgroundAgentSession {
	if manager == nil {
		return nil
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.sessions[agentID]
}
