package loop

import (
	"context"
	"strings"

	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/types"
)

type stopHookResult struct {
	BlockingMessages    []types.Message
	ExecutionSummaries  []stream.HookSummaryEvent
	PreventContinuation bool
	Blocked             bool
	PreventedStopReason string
	RanHook             bool
	HookName            string
	Failed              bool
	FailureSummary      string
}

func runStopHooks(ctx context.Context, runner *hooks.Runner, opts stopHookOptions) (stopHookResult, error) {
	if runner == nil {
		return stopHookResult{}, nil
	}

	result := stopHookResult{}
	hookType := hooks.HookStop
	if opts.AgentID != "" && runner.HasHooks(hooks.HookSubagentStop) {
		hookType = hooks.HookSubagentStop
	}
	if runner.HasHooks(hookType) {
		input := hooks.HookInput{
			SessionID:            opts.SessionID,
			TurnID:               opts.TurnID,
			WorkUnitID:           opts.WorkUnitID,
			AgentID:              opts.AgentID,
			AgentType:            opts.AgentType,
			AgentTranscriptPath:  opts.AgentTranscriptPath,
			LastAssistantMessage: strings.TrimSpace(opts.AssistantMessage.GetText()),
			Result:               strings.TrimSpace(opts.AssistantMessage.GetText()),
			StopHookActive:       opts.StopHookActive,
		}

		executions := runner.RunDetailed(ctx, hookType, input)
		if err := ctx.Err(); err != nil {
			return stopHookResult{}, err
		}
		result.mergeHookExecutions(hookType, localizedHookFeedback("Stop"), i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyAuxHookBlockedContinuation), executions)
		if result.PreventContinuation || result.Blocked {
			return result, nil
		}
	}

	// Stop hooks receive stop_hook_active on the follow-up turn and may choose
	// whether to run again. Teammate lifecycle hooks are one-shot end-turn
	// side effects; re-running them on the hook feedback turn can loop forever
	// for stateless blocking hooks.
	if opts.StopHookActive {
		return result, nil
	}

	teammateResult, err := runTeammateHooks(ctx, runner, opts)
	if err != nil {
		return stopHookResult{}, err
	}
	result.merge(teammateResult)

	return result, nil
}

type stopHookOptions struct {
	AssistantMessage    types.Message
	StopHookActive      bool
	SessionID           string
	AgentID             string
	AgentType           string
	TurnID              string
	WorkUnitID          string
	AgentTranscriptPath string
	TeammateContext     TeammateContextProvider
}

func runTeammateHooks(ctx context.Context, runner *hooks.Runner, opts stopHookOptions) (stopHookResult, error) {
	provider := opts.TeammateContext
	if provider == nil || (!runner.HasHooks(hooks.HookTaskCompleted) && !runner.HasHooks(hooks.HookTeammateIdle)) {
		return stopHookResult{}, nil
	}

	teammate, ok, err := provider.CurrentTeammateContext(ctx)
	if err != nil {
		return stopHookResult{}, err
	}
	if !ok || strings.TrimSpace(teammate.TeammateName) == "" {
		return stopHookResult{}, nil
	}

	result := stopHookResult{}
	if runner.HasHooks(hooks.HookTaskCompleted) {
		for _, task := range teammate.Tasks {
			if task.Status != "in_progress" || task.Owner != teammate.TeammateName {
				continue
			}
			input := hooks.HookInput{
				SessionID:       opts.SessionID,
				TurnID:          opts.TurnID,
				WorkUnitID:      opts.WorkUnitID,
				AgentID:         opts.AgentID,
				AgentType:       opts.AgentType,
				TeammateName:    teammate.TeammateName,
				TeamName:        teammate.TeamName,
				TaskID:          task.ID,
				TaskSubject:     task.Subject,
				TaskDescription: task.Description,
				TaskOwner:       task.Owner,
				Owner:           task.Owner,
			}
			executions := runner.RunDetailed(ctx, hooks.HookTaskCompleted, input)
			if err := ctx.Err(); err != nil {
				return stopHookResult{}, err
			}
			result.mergeHookExecutions(hooks.HookTaskCompleted, localizedHookFeedback("TaskCompleted"), localizedNamedHookBlocked("TaskCompleted"), executions)
		}
	}

	if runner.HasHooks(hooks.HookTeammateIdle) {
		input := hooks.HookInput{
			SessionID:    opts.SessionID,
			TurnID:       opts.TurnID,
			WorkUnitID:   opts.WorkUnitID,
			AgentID:      opts.AgentID,
			AgentType:    opts.AgentType,
			TeammateName: teammate.TeammateName,
			TeamName:     teammate.TeamName,
		}
		executions := runner.RunDetailed(ctx, hooks.HookTeammateIdle, input)
		if err := ctx.Err(); err != nil {
			return stopHookResult{}, err
		}
		result.mergeHookExecutions(hooks.HookTeammateIdle, localizedHookFeedback("TeammateIdle"), localizedNamedHookBlocked("TeammateIdle"), executions)
	}

	return result, nil
}

func (r *stopHookResult) merge(other stopHookResult) {
	if other.RanHook {
		r.RanHook = true
		r.HookName = appendHookName(r.HookName, other.HookName)
	}
	r.BlockingMessages = append(r.BlockingMessages, other.BlockingMessages...)
	r.ExecutionSummaries = append(r.ExecutionSummaries, other.ExecutionSummaries...)
	r.Blocked = r.Blocked || other.Blocked
	r.Failed = r.Failed || other.Failed
	if r.FailureSummary == "" {
		r.FailureSummary = other.FailureSummary
	}
	if other.PreventContinuation {
		r.PreventContinuation = true
		r.PreventedStopReason = other.PreventedStopReason
	}
}

func (r *stopHookResult) mergeHookExecutions(hookType hooks.HookType, feedbackPrefix, defaultText string, executions []hooks.HookExecution) {
	if len(executions) > 0 {
		r.RanHook = true
		r.HookName = appendHookName(r.HookName, string(hookType))
	}
	for _, execution := range executions {
		output := execution.Output
		r.ExecutionSummaries = append(r.ExecutionSummaries, newHookExecutionSummary(hookType, execution, hookSummaryDefaults{
			Blocked:   defaultText,
			Prevented: defaultText,
			Failed:    i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyAuxHookExecutionFailed),
		}))
		if output.PreventContinuation {
			r.PreventContinuation = true
			stopReason := strings.TrimSpace(output.StopReason)
			if stopReason == "" {
				stopReason = defaultText
			}
			r.PreventedStopReason = stopReason
			continue
		}
		if !output.Block && output.ExitCode != 2 {
			if output.ExitCode != 0 || output.ExecutionError != "" {
				r.Failed = true
				r.FailureSummary = firstNonEmpty(r.FailureSummary, strings.TrimSpace(output.Stderr), strings.TrimSpace(output.ExecutionError), strings.TrimSpace(output.SystemReminder), i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyAuxHookExecutionFailed))
			}
			continue
		}
		text := strings.TrimSpace(output.Stderr)
		if text == "" {
			text = strings.TrimSpace(output.ExecutionError)
		}
		if text == "" {
			text = strings.TrimSpace(output.SystemReminder)
		}
		if text == "" {
			text = defaultText
		}
		r.BlockingMessages = append(r.BlockingMessages, types.UserMessage(feedbackPrefix+":\n"+text))
		r.Blocked = true
	}
}

func localizedHookFeedback(hookName string) string {
	return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyAuxHookNamedFeedback, hookName)
}

func localizedNamedHookBlocked(hookName string) string {
	return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyAuxHookNamedBlocked, hookName)
}

func appendHookName(existing, next string) string {
	if next == "" {
		return existing
	}
	if existing == "" {
		return next
	}
	if existing == next || strings.Contains(existing, next) {
		return existing
	}
	return existing + "," + next
}
