package engine

import (
	"context"
	"testing"
	"time"

	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/internal/runtime/compact"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type runtimeControlAttachmentTool struct{}

func (runtimeControlAttachmentTool) Name() string { return "RuntimeControlAttachment" }
func (runtimeControlAttachmentTool) Description() string {
	return "returns an authenticated runtime control attachment"
}
func (runtimeControlAttachmentTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object"}
}
func (runtimeControlAttachmentTool) Execute(context.Context, map[string]any) (types.ToolResult, error) {
	message := types.UserMessage("runtime goal continuation")
	message.ID = "goal:continuation:v1"
	message.IsMeta = true
	message.InternalKind = types.InternalMessageKindGoalContinuation
	message = message.WithInternalControlProvenance(messagecontrol.Runtime())
	return types.ToolResult{Content: "file contents", NewMessages: []types.Message{message}}, nil
}

func TestResumeQuerySaveTwiceAdvancesLiveControlScope(t *testing.T) {
	const sessionID = "control-scope-save-twice"
	sessions := newFileSessionManager(t.TempDir())
	seedScope, err := sessions.store.MessageControlScope(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	developer := types.DeveloperMessage("trusted runtime context", types.DeveloperMessageMetadata{
		Kind:     types.DeveloperMessageKindSkillCatalogSnapshot,
		Revision: 1,
	}).WithInternalControlProvenance(messagecontrol.Runtime(), seedScope)
	replacementMessage := compact.AppendContentReplacementRecordsForScope(
		[]types.Message{types.ToolResultMessage(types.ToolResultBlock{
			ToolUseID: "tool-scope-lifecycle",
			Content:   "original tool result",
		})},
		[]compact.ContentReplacementRecord{{
			Kind:        "tool-result",
			ToolUseID:   "tool-scope-lifecycle",
			Replacement: "persisted replacement",
		}},
		messagecontrol.Runtime(), seedScope,
	)[0]
	seed := []types.Message{
		developer,
		{
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{types.ToolUseBlock{
				Type: types.ContentTypeToolUse,
				ID:   "tool-scope-lifecycle",
				Name: "Tool",
			}},
		},
		replacementMessage,
	}
	if err := sessions.Save(sessionID, seed); err != nil {
		t.Fatalf("seed Save: %v", err)
	}

	provider := &mockProvider{name: "mock", modelID: "mock-model"}
	engine, err := New(Config{Provider: provider, Sessions: sessions})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Resume(context.Background(), sessionID); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	for _, input := range []string{"first turn", "second turn"} {
		stream, err := engine.Query(context.Background(), QueryRequest{SessionID: sessionID, Message: input})
		if err != nil {
			t.Fatalf("Query(%q): %v", input, err)
		}
		events := drainEvents(t, stream, 5*time.Second)
		if len(events) == 0 || !events[len(events)-1].Final || events[len(events)-1].Error != nil {
			t.Fatalf("Query(%q) terminal event = %#v", input, events)
		}
	}
	if provider.callCount != 2 {
		t.Fatalf("provider calls = %d, want 2", provider.callCount)
	}

	wantScope, err := sessions.store.MessageControlScope(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if wantScope.ContextGeneration() != 3 {
		t.Fatalf("context generation = %d, want seed + two saves = 3", wantScope.ContextGeneration())
	}
	conv := engine.convs[engine.currentConversationKey(sessionID)]
	if conv == nil {
		t.Fatal("resumed conversation is missing")
	}
	assertControlScope := func(label string, messages []types.Message) {
		t.Helper()
		if !messages[0].HasInternalControlProvenanceForScope(wantScope) {
			t.Fatalf("%s developer message was not advanced to generation %d", label, wantScope.ContextGeneration())
		}
		block, ok := messages[2].Content[1].(types.ContentReplacementBlock)
		if !ok || !block.HasInternalReplacementProvenanceForScope(wantScope) {
			t.Fatalf("%s replacement receipt was not advanced to generation %d", label, wantScope.ContextGeneration())
		}
	}
	assertControlScope("live", conv.ql.Messages())
	loaded, err := sessions.Load(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	assertControlScope("durable", loaded)
}

func TestQuerySaveBindsRuntimeToolControlAttachmentToSessionScope(t *testing.T) {
	const sessionID = "runtime-tool-control-attachment"
	sessions := newFileSessionManager(t.TempDir())
	reg := registry.New()
	reg.Register(runtimeControlAttachmentTool{})
	provider := &mockProvider{
		name:    "mock",
		modelID: "mock-model",
		responses: [][]types.StreamEvent{
			toolCallEvents("read-tool-use", "RuntimeControlAttachment", map[string]any{}),
			textEvents("finished"),
		},
	}
	engine, err := New(Config{Provider: provider, Sessions: sessions, Registry: reg})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := engine.Query(context.Background(), QueryRequest{SessionID: sessionID, Message: "read a file"})
	if err != nil {
		t.Fatal(err)
	}
	events := drainEvents(t, stream, 5*time.Second)
	if final := events[len(events)-1]; !final.Final || final.Error != nil {
		t.Fatalf("terminal event = %#v", final)
	}

	loaded, err := sessions.Load(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := sessions.internalControlScope(sessionID, "")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, message := range loaded {
		if message.InternalKind != types.InternalMessageKindGoalContinuation {
			continue
		}
		found = true
		if !message.HasInternalControlProvenanceForScope(scope) {
			t.Fatal("persisted runtime control attachment does not have the committed session scope")
		}
	}
	if !found {
		t.Fatal("persisted history does not contain the runtime control attachment")
	}
}
