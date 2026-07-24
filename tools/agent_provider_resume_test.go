package tools

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type agentResumeIdentityProvider struct {
	name  string
	model string

	mu     sync.Mutex
	params []provider.Params
}

func (p *agentResumeIdentityProvider) Name() string    { return p.name }
func (p *agentResumeIdentityProvider) ModelID() string { return p.model }

func (p *agentResumeIdentityProvider) CreateStream(_ context.Context, params provider.Params) (<-chan types.StreamEvent, error) {
	p.mu.Lock()
	p.params = append(p.params, params)
	p.mu.Unlock()
	return eventStream(agentTextEvents("resumed")), nil
}

func (p *agentResumeIdentityProvider) lastParams() (provider.Params, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.params) == 0 {
		return provider.Params{}, false
	}
	return p.params[len(p.params)-1], true
}

func TestRetainedAgentPersistsProviderIdentity(t *testing.T) {
	root := t.TempDir()
	background := NewBackgroundTaskManager(root)
	t.Cleanup(background.Shutdown)
	current := &agentResumeIdentityProvider{name: "openai", model: "gpt-current"}
	tool := &AgentTool{
		Provider:   provider.NewProviderRef(current),
		Registry:   registry.New(),
		Background: background,
	}

	const agentID = "agent-provider-persistence"
	_, _, err := tool.createRetainedAgentSession(agentID, AgentInput{
		Prompt:      "persist provider identity",
		Description: "persist provider identity",
	})
	if err != nil {
		t.Fatalf("createRetainedAgentSession: %v", err)
	}
	record, ok := background.store.Get(agentID)
	if !ok || record.AgentMetadata == nil {
		t.Fatalf("persisted record=%+v ok=%v", record, ok)
	}
	if record.AgentMetadata.Provider != "openai" || record.AgentMetadata.Model != "gpt-current" {
		t.Fatalf("persisted provider/model=%q/%q, want openai/gpt-current", record.AgentMetadata.Provider, record.AgentMetadata.Model)
	}
}

func TestRestoreRetainedAgentKeepsProviderAndModelConsistent(t *testing.T) {
	tests := []struct {
		name              string
		persistedProvider string
		persistedModel    string
		currentProvider   string
		currentModel      string
		wantModel         string
	}{
		{
			name:              "provider switch migrates to current model",
			persistedProvider: "anthropic",
			persistedModel:    "claude-old",
			currentProvider:   "openai",
			currentModel:      "gpt-current",
			wantModel:         "gpt-current",
		},
		{
			name:              "same provider preserves selected model",
			persistedProvider: "openai",
			persistedModel:    "gpt-persisted-selection",
			currentProvider:   "openai",
			currentModel:      "gpt-current-default",
			wantModel:         "gpt-persisted-selection",
		},
		{
			name:              "legacy record migrates safely to current model and gains identity",
			persistedProvider: "",
			persistedModel:    "legacy-selected-model",
			currentProvider:   "openai",
			currentModel:      "gpt-current-default",
			wantModel:         "gpt-current-default",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			background := NewBackgroundTaskManager(root)
			t.Cleanup(background.Shutdown)
			current := &agentResumeIdentityProvider{name: tc.currentProvider, model: tc.currentModel}
			tool := &AgentTool{
				Provider:   provider.NewProviderRef(current),
				Registry:   registry.New(),
				Background: background,
			}
			record := RuntimeTaskRecord{
				ID:          "agent-provider-resume",
				Type:        backgroundTaskTypeLocalAgent,
				Status:      "completed",
				Description: "provider resume",
				Prompt:      "original prompt",
				StartedAt:   time.Now(),
				AgentInput: &AgentInput{
					Prompt:      "original prompt",
					Description: "provider resume",
				},
				AgentMetadata: &agentSessionMetadata{
					AgentType: "general-purpose",
					Provider:  tc.persistedProvider,
					Model:     tc.persistedModel,
					PermissionSnapshot: &types.ToolRuntimeContext{
						ProjectRoot: root, AllowedDirs: []string{root}, PermissionMode: permissionModeDefault,
					},
				},
			}
			background.rememberTrustedAgentResume(record.ID, *record.AgentInput, *record.AgentMetadata)

			session, _, err := tool.RestoreAgentSessionFromRecord(record.ID, record)
			if err != nil {
				t.Fatalf("RestoreAgentSessionFromRecord: %v", err)
			}
			if _, err := session.runSync(context.Background(), "continue"); err != nil {
				t.Fatalf("runSync: %v", err)
			}
			params, ok := current.lastParams()
			if !ok {
				t.Fatal("restored session did not call the current provider")
			}
			if params.Model != tc.wantModel {
				t.Fatalf("restored request model=%q, want %q", params.Model, tc.wantModel)
			}
			persisted, ok := background.store.Get(record.ID)
			if !ok || persisted.AgentMetadata == nil {
				t.Fatalf("restored record=%+v ok=%v", persisted, ok)
			}
			if persisted.AgentMetadata.Provider != tc.currentProvider || persisted.AgentMetadata.Model != tc.wantModel {
				t.Fatalf("restored provider/model=%q/%q, want %q/%q", persisted.AgentMetadata.Provider, persisted.AgentMetadata.Model, tc.currentProvider, tc.wantModel)
			}
		})
	}
}
