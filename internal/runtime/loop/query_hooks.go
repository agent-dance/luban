package loop

import (
	"context"
	"strings"

	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/contracts/stream"
)

// runQueryHook executes every matching query hook as an evidence-bearing,
// fail-closed policy boundary. Query hooks guard model I/O, so an execution
// failure is not allowed to degrade to the non-blocking behavior used by
// observational hooks.
func runQueryHook(ctx context.Context, runner *hooks.Runner, hookType hooks.HookType, input hooks.HookInput, onEvent func(stream.Event)) error {
	if runner == nil || !runner.HasHooks(hookType) {
		return nil
	}
	executions := runner.RunDetailed(ctx, hookType, input)
	if err := ctx.Err(); err != nil {
		return i18n.WrapError(i18n.KeyLoopQueryHookCancelled, err, hookType)
	}
	for _, execution := range executions {
		if onEvent != nil {
			execution = execution.Snapshot()
			summary := newHookExecutionSummary(hookType, execution, localizedHookSummaryDefaults())
			evidence := execution.Input
			onEvent(stream.Event{
				Type:        stream.EventHookSummary,
				ProjectRoot: evidence.ProjectRoot,
				TurnID:      evidence.TurnID,
				ActorID:     evidence.AgentID,
				ActorType:   evidence.AgentType,
				WorkUnitID:  evidence.WorkUnitID,
				ToolUseID:   evidence.ToolUseID,
				HookSummary: &summary,
			})
		}
		output := execution.Output
		failed := output.Block || output.PreventContinuation || output.ExitCode != 0 || output.ExecutionError != "" ||
			strings.EqualFold(output.PermissionBehavior, "deny") || strings.EqualFold(output.PermissionBehavior, "block")
		if !failed {
			continue
		}
		reason := firstNonEmpty(
			strings.TrimSpace(output.Stderr),
			strings.TrimSpace(output.ExecutionError),
			strings.TrimSpace(output.StopReason),
			strings.TrimSpace(output.SystemReminder),
			i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLoopQueryHookRefused),
		)
		return i18n.NewError(i18n.KeyLoopQueryHookFailedClosed, hookType, execution.ExecutionID, reason)
	}
	return nil
}
