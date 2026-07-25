package agent

import (
	"context"
	"errors"
	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
	runtimestore "github.com/agent-dance/luban/internal/store/runtime"
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
	t.Cleanup(func() { _ = background.Shutdown(context.Background()) })
	current := &agentResumeIdentityProvider{name: "openai", model: "gpt-current"}
	tool := &AgentTool{
		Provider:   provider.NewProviderRef(current),
		Registry:   registry.New(),
		Background: background,
	}

	const agentID = "agent-provider-persistence"
	_, _, err := tool.createRetainedAgentSession(agentID, agentcontract.Input{
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
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			background := NewBackgroundTaskManager(root)
			t.Cleanup(func() { _ = background.Shutdown(context.Background()) })
			current := &agentResumeIdentityProvider{name: tc.currentProvider, model: tc.currentModel}
			tool := &AgentTool{
				Provider:   provider.NewProviderRef(current),
				Registry:   registry.New(),
				Background: background,
			}
			record := runtimestore.RuntimeTaskRecord{
				ID:          "agent-provider-resume",
				Type:        agentcontract.TaskTypeLocalAgent,
				Status:      "completed",
				Description: "provider resume",
				Prompt:      "original prompt",
				StartedAt:   time.Now(),
				AgentInput: &agentcontract.Input{
					Prompt:      "original prompt",
					Description: "provider resume",
				},
				AgentMetadata: &agentcontract.SessionMetadata{
					AgentType:       "general-purpose",
					Provider:        tc.persistedProvider,
					Model:           tc.persistedModel,
					ApprovalRouting: agentcontract.ApprovalFailClosed,
					PermissionSnapshot: &types.ToolRuntimeContext{
						ProjectRoot: root, AllowedDirs: []string{root}, PermissionMode: permissionModeDefault,
					},
				},
			}
			trustAgentResumeForTest(background, record.ID, *record.AgentInput, *record.AgentMetadata)

			err := tool.RestoreAgentSession(record.ID, record)
			if err != nil {
				t.Fatalf("RestoreAgentSession: %v", err)
			}
			session := restoredAgentSessionForTest(background, record.ID)
			if session == nil {
				t.Fatal("restored session was not registered")
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

func TestRestoreRetainedAgentRejectsMetadataWithoutProviderIdentity(t *testing.T) {
	root := t.TempDir()
	background := NewBackgroundTaskManager(root)
	t.Cleanup(func() { _ = background.Shutdown(context.Background()) })
	current := &agentResumeIdentityProvider{name: "openai", model: "gpt-current"}
	tool := &AgentTool{
		Provider: provider.NewProviderRef(current), Registry: registry.New(), Background: background,
	}
	permissionSnapshot := types.ToolRuntimeContext{
		ProjectRoot: root, AllowedDirs: []string{root}, PermissionMode: permissionModeDefault,
	}
	record := runtimestore.RuntimeTaskRecord{
		ID: "agent-provider-missing", Type: agentcontract.TaskTypeLocalAgent, Status: "completed",
		AgentInput: &agentcontract.Input{Prompt: "continue", Description: "resume"},
		AgentMetadata: &agentcontract.SessionMetadata{
			AgentType: "general-purpose", Model: "old-model", ApprovalRouting: agentcontract.ApprovalFailClosed,
			PermissionSnapshot: &permissionSnapshot,
		},
	}
	trustAgentResumeForTest(background, record.ID, *record.AgentInput, *record.AgentMetadata)

	if err := tool.RestoreAgentSession(record.ID, record); !errors.Is(err, errAgentResumeContextUntrusted) {
		t.Fatalf("restore error = %v, want untrusted resume context", err)
	}
}
