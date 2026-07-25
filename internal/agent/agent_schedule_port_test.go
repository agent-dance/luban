package agent

import (
	"context"
	"testing"

	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
	"github.com/agent-dance/luban/internal/tools/schedule"
	"github.com/agent-dance/luban/registry"
)

type scheduleExecutorFunc func(context.Context, schedule.Execution) error

func (f scheduleExecutorFunc) Enqueue(ctx context.Context, execution schedule.Execution) error {
	return f(ctx, execution)
}

func TestAgentToolImplementsScheduledPromptBoundary(t *testing.T) {
	type scheduledPromptRunner interface {
		RunScheduledPrompt(context.Context, string, agentcontract.Input) (string, error)
	}
	var _ scheduledPromptRunner = (*AgentTool)(nil)
}

func TestAgentRegistryBindsFormalScheduleScope(t *testing.T) {
	root := t.TempDir()
	service, err := schedule.NewService(root, scheduleExecutorFunc(func(context.Context, schedule.Execution) error { return nil }), nil, nil)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	reg := registry.New()
	reg.Register(schedule.NewCreateTool(service))
	bindInProcessAgentScopedTools(reg, "subagent-1", root)
	result, err := reg.Get("CronCreate").Execute(context.Background(), map[string]any{
		"cron": "* * * * *", "prompt": "must remain session scoped", "durable": true,
	})
	if err != nil {
		t.Fatalf("CronCreate: %v", err)
	}
	if !result.IsError || result.Metadata["error_code"] != "durable_agent_denied" {
		t.Fatalf("formal Agent scope was not applied: %#v", result)
	}
}
