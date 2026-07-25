package agent

import (
	"context"
	"testing"

	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/internal/runtime/loop"
	"github.com/agent-dance/luban/internal/runtime/skillauthority"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

type captureSkillAuthorityTool struct {
	manager    *skills.Manager
	captured   chan skillauthority.Authority
	captureErr chan error
}

func (*captureSkillAuthorityTool) Name() string { return "CaptureSkillAuthorityForTest" }
func (*captureSkillAuthorityTool) Description() string {
	return "Capture the runtime skill authority for a focused test."
}
func (*captureSkillAuthorityTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(map[string]any{})
}
func (tool *captureSkillAuthorityTool) Execute(ctx context.Context, _ map[string]any) (types.ToolResult, error) {
	authority, err := skillauthority.Capture(ctx, tool.manager)
	if err != nil {
		tool.captureErr <- err
		return types.ToolResult{}, err
	}
	tool.captured <- authority
	return types.ToolResult{Content: "captured"}, nil
}

func captureSkillAuthorityForTest(t *testing.T, manager *skills.Manager, sessionID, projectRoot string) skillauthority.Authority {
	t.Helper()
	captured := make(chan skillauthority.Authority, 1)
	captureErr := make(chan error, 1)
	reg := registry.New()
	reg.Register(&captureSkillAuthorityTool{
		manager: manager, captured: captured, captureErr: captureErr,
	})
	model := &sequencedAgentProvider{responses: [][]types.StreamEvent{
		agentToolEvents("CaptureSkillAuthorityForTest", "capture-authority"),
		agentTextEvents("done"),
	}}
	query := loop.New(model, reg, loop.Config{
		Model:             model.ModelID(),
		MaxTurns:          2,
		MaxTokens:         64,
		SessionID:         sessionID,
		SessionProjectDir: "skill-authority-test-session",
		ProjectRoot:       projectRoot,
		CWD:               projectRoot,
		SkillManager:      manager,
	})
	if err := query.Run(context.Background(), "capture authority", func(stream.Event) {}); err != nil {
		select {
		case captureErr := <-captureErr:
			t.Fatalf("capture skill authority: %v", captureErr)
		default:
			t.Fatalf("run skill authority capture: %v", err)
		}
	}
	select {
	case authority := <-captured:
		return authority
	default:
		t.Fatal("runtime tool did not capture skill authority")
		return skillauthority.Authority{}
	}
}
