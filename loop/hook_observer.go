package loop

import (
	"context"

	"github.com/agent-dance/luban/hooks"
)

// withHookExecutionEventEmitter adapts execution-scoped hook evidence onto the
// same structured event stream as tool and turn activity. Only call sites that
// opt into RunDetailedObserved publish here, so legacy explicit summary paths
// are not duplicated.
func withHookExecutionEventEmitter(ctx context.Context, onEvent func(Event)) context.Context {
	if onEvent == nil {
		return ctx
	}
	return hooks.WithExecutionObserver(ctx, func(hookType hooks.HookType, execution hooks.HookExecution) {
		execution = execution.Snapshot()
		input := execution.Input
		summary := newHookExecutionSummary(hookType, execution, localizedHookSummaryDefaults())
		onEvent(Event{
			Type:        EventHookSummary,
			ProjectRoot: input.ProjectRoot,
			TurnID:      input.TurnID,
			ActorID:     input.AgentID,
			ActorType:   input.AgentType,
			WorkUnitID:  input.WorkUnitID,
			ToolUseID:   input.ToolUseID,
			HookSummary: &summary,
		})
	})
}
