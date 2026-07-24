package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/agent-dance/luban/engine"
	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/sandbox"
	"github.com/agent-dance/luban/session"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

type task23SkillRuntimeProvider struct {
	mu    sync.Mutex
	steps [][]types.StreamEvent
	calls []provider.Params
}

func (p *task23SkillRuntimeProvider) Name() string    { return "task23-child-runtime" }
func (p *task23SkillRuntimeProvider) ModelID() string { return "task23-child-model" }

func (p *task23SkillRuntimeProvider) CreateStream(ctx context.Context, params provider.Params) (<-chan types.StreamEvent, error) {
	p.mu.Lock()
	index := len(p.calls)
	params.Messages = append([]types.Message(nil), params.Messages...)
	p.calls = append(p.calls, params)
	var events []types.StreamEvent
	if index < len(p.steps) {
		events = append([]types.StreamEvent(nil), p.steps[index]...)
	}
	p.mu.Unlock()

	stream := make(chan types.StreamEvent, len(events))
	go func() {
		defer close(stream)
		for _, event := range events {
			select {
			case <-ctx.Done():
				return
			case stream <- event:
			}
		}
	}()
	return stream, nil
}

func (p *task23SkillRuntimeProvider) Calls() []provider.Params {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]provider.Params(nil), p.calls...)
}

func TestSkillRuntimeUsesExactParentAndChildLoopLedgers(t *testing.T) {
	root := t.TempDir()
	writeRootTask23Skill(t, root, "task23-child-skill")

	parentProvider := &task23SkillRuntimeProvider{steps: task23SkillRuntimeSteps(
		"skill:project:task23-child-skill", 0,
		[]map[string]any{{}, nil},
	)}
	providerRef := provider.NewProviderRef(parentProvider)
	deps := SetupRegistry(providerRef, root, []string{root}, sandbox.NoopBackend{}, nil)
	t.Cleanup(func() {
		deps.CronStore.Stop()
		deps.StopWebFetchCache()
	})
	if err := prepareInitialRegistryRuntime(deps, root, []string{root}); err != nil {
		t.Fatalf("prepare skill runtime: %v", err)
	}

	const parentSession = "task23-parent"
	parentSnapshot, err := deps.SkillManager.Snapshot(parentSession)
	if err != nil {
		t.Fatal(err)
	}
	row, found := rootTask23Skill(parentSnapshot, "task23-child-skill")
	if !found {
		t.Fatalf("child skill missing from catalog: %+v", parentSnapshot.Skills)
	}
	// Replace the provisional stable ID/revision in the scripted provider with
	// the exact discovered identity.
	parentProvider.mu.Lock()
	parentProvider.steps = task23SkillRuntimeSteps(string(row.ID), row.Revision, []map[string]any{{}, nil})
	parentProvider.mu.Unlock()

	repo := session.NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD(root)
	eng, err := engine.New(engine.Config{
		Provider: parentProvider, ProviderRef: providerRef, Registry: deps.Registry,
		Sessions:              engine.NewRepositorySessionManager(repo, func() string { return projectDir }),
		ProjectRoot:           root,
		CWD:                   root,
		SkillManager:          deps.SkillManager,
		SkillSessionOverrides: deps.SkillSessionOverrides,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = eng.Shutdown(context.Background()) })
	deps.BindSessionIdentity(parentSession)
	_ = configureSkillRuntime(deps, eng)

	events, err := eng.Query(context.Background(), engine.QueryRequest{
		SessionID: parentSession, CWD: root, Message: "load the parent skill",
	})
	if err != nil {
		t.Fatal(err)
	}
	for range events {
	}
	parentLedger := eng.ResolveSkillLoadedLedger(context.Background(), parentSession, row.ID)
	if parentLedger.LoadedContextEpoch == 0 || parentLedger.LoadedContextEpoch != parentLedger.ContextEpoch {
		t.Fatalf("parent engine did not load skill body: %#v", parentLedger)
	}

	// The same bare session ID in a context not created by a QueryLoop must not
	// fall back to the now-loaded CoreEngine conversation.
	capabilityless := loop.WithToolExecutionContext(context.Background(), loop.ToolExecutionContext{SessionID: parentSession})
	if got := deps.SkillTool.LoadedLedgerResolver(capabilityless, parentSession, row.ID); got.ContextEpoch != 0 {
		t.Fatalf("capabilityless model context fell back to parent engine: %#v", got)
	}
	mismatched := loop.WithToolExecutionContext(context.Background(), loop.ToolExecutionContext{SessionID: "other-child"})
	if got := deps.SkillTool.LoadedLedgerResolver(mismatched, parentSession, row.ID); got.ContextEpoch != 0 {
		t.Fatalf("mismatched child session fell back to parent engine: %#v", got)
	}

	newChild := func(sessionID string, streaming bool, inputs []map[string]any) (*loop.QueryLoop, *task23SkillRuntimeProvider) {
		childProvider := &task23SkillRuntimeProvider{steps: task23SkillRuntimeSteps(string(row.ID), row.Revision, inputs)}
		childRegistry := registry.New()
		childRegistry.Register(deps.SkillTool)
		query := loop.New(childProvider, childRegistry, loop.Config{
			MaxTurns: 3, MaxTokens: 1024, SessionID: sessionID,
			AgentID: sessionID, AgentType: "task23-child", ProjectRoot: root, CWD: root,
			SkillManager: deps.SkillManager, StreamingToolExecution: streaming,
		})
		return query, childProvider
	}

	const childA = "task23-agent-a"
	childAInputs := []map[string]any{{}, {}, {"args": "changed"}, {"args": "changed"}}
	queryA, providerA := newChild(childA, false, childAInputs)
	for index := range childAInputs {
		if index == 3 {
			// A wholesale history install is an epoch fence. The previous turn
			// loaded the same changed payload, so this call would be an ack without
			// the fence and must be full with it.
			queryA.SetMessages(queryA.Messages())
		}
		if err := queryA.Run(context.Background(), fmt.Sprintf("agent turn %d", index+1), func(loop.Event) {}); err != nil {
			t.Fatalf("agent child turn %d: %v", index+1, err)
		}
	}
	if got, want := task23SkillEnvelopeKinds(queryA.Messages()), []string{"full", "already_loaded", "full", "full"}; !equalTask23Strings(got, want) {
		t.Fatalf("agent child envelope kinds = %v, want %v", got, want)
	}
	task23AssertChildCalls(t, providerA.Calls(), childA)

	const childB = "team-lead@task23-team"
	childBInputs := []map[string]any{{}, {}}
	queryB, providerB := newChild(childB, true, childBInputs)
	for index := range childBInputs {
		if err := queryB.Run(context.Background(), fmt.Sprintf("team turn %d", index+1), func(loop.Event) {}); err != nil {
			t.Fatalf("streaming team-shaped child turn %d: %v", index+1, err)
		}
	}
	if got, want := task23SkillEnvelopeKinds(queryB.Messages()), []string{"full", "already_loaded"}; !equalTask23Strings(got, want) {
		t.Fatalf("streaming child envelope kinds = %v, want %v", got, want)
	}
	task23AssertChildCalls(t, providerB.Calls(), childB)

	if changed, found := deps.SkillManager.SetEnabled(childA, row.Name, false); !changed || !found {
		t.Fatalf("disable agent child overlay = changed %t, found %t", changed, found)
	}
	if deps.SkillManager.IsEnabled(childA, row.Name) || !deps.SkillManager.IsEnabled(childB, row.Name) {
		t.Fatalf("child session overlays leaked: childA=%t childB=%t",
			deps.SkillManager.IsEnabled(childA, row.Name), deps.SkillManager.IsEnabled(childB, row.Name))
	}
}

func task23SkillRuntimeSteps(id string, revision skills.SkillRevision, inputs []map[string]any) [][]types.StreamEvent {
	steps := make([][]types.StreamEvent, 0, len(inputs)*2)
	for index, input := range inputs {
		if input == nil {
			steps = append(steps, task23SkillRuntimeTextEvents("done"))
			continue
		}
		call := map[string]any{"skill": id}
		if revision != 0 {
			call["revision"] = uint64(revision)
		}
		for key, value := range input {
			call[key] = value
		}
		steps = append(steps, task23SkillRuntimeToolEvents(fmt.Sprintf("task23-skill-%d", index+1), call))
		steps = append(steps, task23SkillRuntimeTextEvents("done"))
	}
	return steps
}

func task23SkillRuntimeToolEvents(id string, input map[string]any) []types.StreamEvent {
	encoded, _ := json.Marshal(input)
	stop := types.StopReasonToolUse
	return []types.StreamEvent{
		{Type: types.EventMessageStart, Message: &types.APIMessage{Role: types.RoleAssistant}},
		{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeToolUse, ID: id, Name: "Skill"}},
		{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: string(encoded)}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageDelta, StopReason: &stop},
		{Type: types.EventMessageStop},
	}
}

func task23SkillRuntimeTextEvents(text string) []types.StreamEvent {
	stop := types.StopReasonEndTurn
	return []types.StreamEvent{
		{Type: types.EventMessageStart, Message: &types.APIMessage{Role: types.RoleAssistant}},
		{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
		{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "text_delta", Text: text}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageDelta, StopReason: &stop},
		{Type: types.EventMessageStop},
	}
}

func task23SkillEnvelopeKinds(messages []types.Message) []string {
	var kinds []string
	for _, message := range messages {
		for _, content := range message.Content {
			result, ok := content.(types.ToolResultBlock)
			if !ok || result.Metadata["commandName"] != "task23-child-skill" {
				continue
			}
			kinds = append(kinds, result.Metadata["envelopeKind"])
		}
	}
	return kinds
}

func task23AssertChildCalls(t *testing.T, calls []provider.Params, sessionID string) {
	t.Helper()
	if len(calls) == 0 {
		t.Fatal("child provider was not called")
	}
	for index, call := range calls {
		if call.PromptCacheKey != sessionID {
			t.Fatalf("child call %d prompt cache key = %q, want %q", index, call.PromptCacheKey, sessionID)
		}
	}
}

func equalTask23Strings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
