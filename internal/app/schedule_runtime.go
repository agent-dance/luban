package app

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"strings"

	"github.com/agent-dance/luban/i18n"
	agentruntime "github.com/agent-dance/luban/internal/agent"
	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
	runtimestore "github.com/agent-dance/luban/internal/store/runtime"
	"github.com/agent-dance/luban/internal/tools/schedule"
)

// appScheduleExecutor is the application composition boundary between the
// schedule domain and Agent's durable background-task port.
type scheduledPromptRunner interface {
	RunScheduledPrompt(context.Context, string, agentcontract.Input) (string, error)
}

type appScheduleExecutor struct {
	agent      scheduledPromptRunner
	background *agentruntime.BackgroundTaskManager
}

func (e *appScheduleExecutor) Enqueue(ctx context.Context, execution schedule.Execution) error {
	if e == nil || e.agent == nil || e.background == nil || strings.TrimSpace(execution.DeliveryID) == "" {
		return i18n.NewError(i18n.KeyToolScheduleExecutorUnavailable)
	}
	input := agentcontract.Input{
		Description: i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolScheduleExecutionDescription, execution.Job.ID),
		Prompt:      execution.Job.Prompt,
		CWD:         execution.Job.ProjectRoot,
	}
	_, err := e.background.StartScheduledAgentTask(
		ctx, execution.DeliveryID, scheduleAgentID(execution.Job.ProjectRoot, execution.DeliveryID), input, e.run,
	)
	return err
}

func (e *appScheduleExecutor) Resume(ctx context.Context) error {
	if e == nil || e.agent == nil || e.background == nil {
		return nil
	}
	return e.background.ResumeScheduledAgentTasks(ctx, e.run)
}

func (e *appScheduleExecutor) run(ctx context.Context, agentID string, input agentcontract.Input, _ io.Writer) (string, error) {
	return e.agent.RunScheduledPrompt(ctx, agentID, input)
}

func scheduleAgentID(projectRoot, deliveryID string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(projectRoot) + "\x00" + deliveryID))
	return fmt.Sprintf("schedule-%x", digest[:16])
}

type appScheduleFireSink struct{}

func (appScheduleFireSink) PublishScheduleFire(ctx context.Context, event schedule.FireEvent) error {
	lifecycle := runtimestore.NewRuntimeLifecycle(event.ProjectRoot)
	return lifecycle.Publish(ctx, runtimestore.RuntimeLifecycleEvent{
		ID: event.DeliveryID, Type: runtimestore.LifecycleCronFire,
		EntityID: event.JobID, ToolName: "CronCreate", Status: "accepted",
		Payload: map[string]any{
			"delivery_id": event.DeliveryID, "expression": event.Expression,
			"recurring": event.Recurring, "durable": event.Durable,
			"scheduled_at": event.ScheduledAt.UTC(),
		},
	})
}
