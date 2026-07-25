package loop

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"

	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type task23RunTokenProbe struct {
	query        *QueryLoop
	retained     chan executioncontract.ToolExecutionContext
	activeDuring chan bool
	ownerDuring  chan task23RuntimeOwnerIdentity
	forgedDuring chan bool
}

type task23RuntimeOwnerIdentity struct {
	sessionID         string
	sessionProjectDir string
	projectRoot       string
	cwd               string
	ok                bool
}

func (*task23RunTokenProbe) Name() string        { return "ProjectIdentityProbe" }
func (*task23RunTokenProbe) Description() string { return "captures one loop-owned run capability" }
func (*task23RunTokenProbe) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object"}
}
func (probe *task23RunTokenProbe) Execute(ctx context.Context, _ map[string]any) (types.ToolResult, error) {
	exec, _ := executioncontract.ToolExecutionContextFromContext(ctx)
	probe.activeDuring <- probe.query.OwnsToolExecution(exec)
	sessionID, sessionProjectDir, projectRoot, cwd, ok := exec.ActiveRuntimeOwnerIdentity()
	probe.ownerDuring <- task23RuntimeOwnerIdentity{
		sessionID: sessionID, sessionProjectDir: sessionProjectDir,
		projectRoot: projectRoot, cwd: cwd, ok: ok,
	}
	forged := exec
	forged.SessionProjectDir = "/sessions/forged"
	forgedContext := executioncontract.WithToolExecutionContext(ctx, forged)
	rewrapped, _ := executioncontract.ToolExecutionContextFromContext(forgedContext)
	_, _, _, _, forgedActive := rewrapped.ActiveRuntimeOwnerIdentity()
	probe.forgedDuring <- forgedActive
	probe.retained <- exec
	return types.ToolResult{Content: "captured"}, nil
}

func TestTask23ToolExecutionCapabilityExpiresAtRunBoundary(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(map[bool]string{false: "after_message", true: "streaming"}[streaming], func(t *testing.T) {
			probe := &task23RunTokenProbe{
				retained: make(chan executioncontract.ToolExecutionContext, 1), activeDuring: make(chan bool, 1),
				ownerDuring: make(chan task23RuntimeOwnerIdentity, 1), forgedDuring: make(chan bool, 1),
			}
			reg := registry.New()
			reg.Register(probe)
			query := New(&projectToolIdentityProvider{}, reg, Config{
				SessionID: "task23-active-run", SessionProjectDir: "/sessions/task23",
				ProjectRoot: "/workspace/a", CWD: "/workspace/a", MaxTurns: 2,
				StreamingToolExecution: streaming,
			})
			probe.query = query
			if err := query.Run(context.Background(), "capture", func(stream.Event) {}); err != nil {
				t.Fatal(err)
			}
			if active := <-probe.activeDuring; !active {
				t.Fatal("loop-owned execution context was not active during its Run")
			}
			identity := <-probe.ownerDuring
			if !identity.ok || identity.sessionID != "task23-active-run" || identity.sessionProjectDir != "/sessions/task23" ||
				identity.projectRoot != "/workspace/a" || identity.cwd != "/workspace/a" {
				t.Fatalf("active runtime owner identity = %+v", identity)
			}
			if forgedActive := <-probe.forgedDuring; forgedActive {
				t.Fatal("rewrapped context with a forged SessionProjectDir retained runtime authority")
			}
			retained := <-probe.retained
			if !retained.HasRuntimeOwner() {
				t.Fatal("retained context lost its private provenance; expiry was not tested")
			}
			if retained.IsRuntimeOwned() || query.OwnsToolExecution(retained) {
				t.Fatal("context retained after Run still authorizes active-run operations")
			}
			if _, _, _, _, ok := retained.ActiveRuntimeOwnerIdentity(); ok {
				t.Fatal("expired context returned an active runtime identity")
			}

			executioncontract.BeginRun(query.executionOwner, "a-later-run")
			t.Cleanup(func() { executioncontract.EndRun(query.executionOwner, "a-later-run") })
			if query.OwnsToolExecution(retained) {
				t.Fatal("old run token was accepted by a later Run of the same QueryLoop")
			}
		})
	}
}

func TestToolExecutionContextDeepClonePreservesIdentityAndMutableBlocks(t *testing.T) {
	metadata := &types.DeveloperMessageMetadata{
		Kind:     types.DeveloperMessageKindSkillCatalogSnapshot,
		Revision: 7,
	}
	usage := &types.Usage{InputTokens: 11, OutputTokens: 3}
	original := types.Message{
		ID:   "skill-body:3:sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Role: types.RoleUser, IsMeta: true, DeveloperMetadata: metadata,
		Content: []types.ContentBlock{
			types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "use-1", Name: "Skill", Input: map[string]any{
				"nested": map[string]any{"items": []any{"stable", map[string]any{"value": "before"}}},
			}},
			types.ToolResultBlock{
				Type: types.ContentTypeToolResult, ToolUseID: "use-1",
				ContentBlocks: []types.ContentBlock{
					types.ImageBlock{Type: types.ContentTypeImage, Source: &types.ImageSource{Type: "base64", Data: "before"}},
					types.DocumentBlock{Type: types.ContentTypeDocument, Source: &types.DocumentSource{Type: "base64", Data: "before"}},
					types.UnknownBlock{Type: "future", Raw: json.RawMessage(`{"type":"future","value":"before"}`)},
				},
				Data: map[string]any{"bytes": []byte("before")},
				NewMessages: []types.Message{{Role: types.RoleAssistant, Content: []types.ContentBlock{
					types.ToolUseBlock{Type: types.ContentTypeToolUse, Input: map[string]any{"child": "before"}},
				}}},
				Metadata: map[string]string{"state": "before"}, Usage: usage,
			},
		},
	}

	directToolUse := types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "direct", Name: "Skill", Input: map[string]any{
		"nested": map[string]any{"value": "before"},
	}}
	ctx := executioncontract.WithToolExecutionContext(context.Background(), executioncontract.ToolExecutionContext{
		Messages: []types.Message{original}, AssistantMessage: original, ToolUse: directToolUse,
	})

	mutateTask23ContextMessage(&original, "original-after")
	metadata.Revision = 99
	usage.InputTokens = 99
	directToolUse.Input["nested"].(map[string]any)["value"] = "original-after"

	first, ok := executioncontract.ToolExecutionContextFromContext(ctx)
	if !ok || len(first.Messages) != 1 {
		t.Fatalf("context snapshot = %#v, ok=%t", first, ok)
	}
	assertTask23ContextMessage(t, first.Messages[0], "before", 7)
	assertTask23ContextMessage(t, first.AssistantMessage, "before", 7)
	if got := first.ToolUse.Input["nested"].(map[string]any)["value"]; got != "before" {
		t.Fatalf("direct tool use input = %v, want before", got)
	}

	mutateTask23ContextMessage(&first.Messages[0], "returned-after")
	first.ToolUse.Input["nested"].(map[string]any)["value"] = "returned-after"
	second, ok := executioncontract.ToolExecutionContextFromContext(ctx)
	if !ok {
		t.Fatal("second context snapshot missing")
	}
	assertTask23ContextMessage(t, second.Messages[0], "before", 7)
	assertTask23ContextMessage(t, original, "original-after", 99)
	if got := second.ToolUse.Input["nested"].(map[string]any)["value"]; got != "before" {
		t.Fatalf("stored direct tool use input = %v, want before", got)
	}
}

func TestToolExecutionContextCloneHasNoMutableAliasesUnderRace(t *testing.T) {
	original := types.Message{Role: types.RoleAssistant, Content: []types.ContentBlock{
		types.ToolUseBlock{Type: types.ContentTypeToolUse, Input: map[string]any{
			"nested": map[string]any{"items": []any{"before"}},
		}},
	}}
	direct := original.Content[0].(types.ToolUseBlock)
	ctx := executioncontract.WithToolExecutionContext(context.Background(), executioncontract.ToolExecutionContext{Messages: []types.Message{original}, ToolUse: direct})

	var wait sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wait.Add(1)
		go func(value string) {
			defer wait.Done()
			for iteration := 0; iteration < 200; iteration++ {
				snapshot, ok := executioncontract.ToolExecutionContextFromContext(ctx)
				if !ok {
					t.Error("context snapshot missing")
					return
				}
				use := snapshot.Messages[0].Content[0].(types.ToolUseBlock)
				use.Input["nested"].(map[string]any)["items"].([]any)[0] = value
				snapshot.ToolUse.Input["nested"].(map[string]any)["items"].([]any)[0] = value
			}
		}(string(rune('a' + worker)))
	}
	wait.Wait()

	use := original.Content[0].(types.ToolUseBlock)
	if got := use.Input["nested"].(map[string]any)["items"].([]any)[0]; got != "before" {
		t.Fatalf("original nested input = %v, want before", got)
	}
}

func mutateTask23ContextMessage(message *types.Message, value string) {
	message.ID = value
	message.DeveloperMetadata.Revision = 99
	use := message.Content[0].(types.ToolUseBlock)
	use.Input["nested"].(map[string]any)["items"].([]any)[1].(map[string]any)["value"] = value
	result := message.Content[1].(types.ToolResultBlock)
	result.ContentBlocks[0].(types.ImageBlock).Source.Data = value
	result.ContentBlocks[1].(types.DocumentBlock).Source.Data = value
	result.ContentBlocks[2].(types.UnknownBlock).Raw[0] = '['
	result.Data.(map[string]any)["bytes"].([]byte)[0] = 'X'
	result.NewMessages[0].Content[0].(types.ToolUseBlock).Input["child"] = value
	result.Metadata["state"] = value
	result.Usage.InputTokens = 99
}

func assertTask23ContextMessage(t *testing.T, message types.Message, value string, revision uint64) {
	t.Helper()
	if message.ID != "skill-body:3:sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" && value == "before" {
		t.Fatalf("message ID = %q, provenance was not preserved", message.ID)
	}
	if message.Role != types.RoleUser || !message.IsMeta || message.DeveloperMetadata == nil || message.DeveloperMetadata.Revision != revision {
		t.Fatalf("message identity = %#v", message)
	}
	use := message.Content[0].(types.ToolUseBlock)
	if got := use.Input["nested"].(map[string]any)["items"].([]any)[1].(map[string]any)["value"]; got != value {
		t.Fatalf("nested tool input = %v, want %q", got, value)
	}
	result := message.Content[1].(types.ToolResultBlock)
	wantRaw, wantData, wantUsage := byte('['), byte('X'), 99
	if value == "before" {
		wantRaw, wantData, wantUsage = '{', 'b', 11
	}
	if result.ContentBlocks[0].(types.ImageBlock).Source.Data != value ||
		result.ContentBlocks[1].(types.DocumentBlock).Source.Data != value ||
		result.NewMessages[0].Content[0].(types.ToolUseBlock).Input["child"] != value ||
		result.Metadata["state"] != value || result.Usage.InputTokens != wantUsage ||
		result.ContentBlocks[2].(types.UnknownBlock).Raw[0] != wantRaw ||
		result.Data.(map[string]any)["bytes"].([]byte)[0] != wantData {
		t.Fatalf("nested tool result was aliased: %#v", result)
	}
}
