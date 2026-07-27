package loop

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/types"
)

// PostSamplingRunner observes a completed assistant sample before tool
// execution begins. Implementations must not mutate the supplied messages.
type PostSamplingRunner interface {
	RunPostSampling(ctx context.Context, messages []types.Message, opts PostSamplingOptions) PostSamplingResult
	RunStopFailure(ctx context.Context, message types.Message, opts StopFailureOptions)
}

type PostSamplingOptions struct {
	SessionID  string
	TurnID     string
	AgentID    string
	AgentType  string
	WorkUnitID string
}

type PostSamplingResult struct {
	Blocked bool
	Failed  bool
	Reason  string
}

type StopFailureOptions struct {
	SessionID  string
	TurnID     string
	AgentID    string
	AgentType  string
	WorkUnitID string
}

// TurnSideEffects runs lightweight end-turn bookkeeping after a successful
// final assistant response. It is fire-and-forget by default.
type TurnSideEffects interface {
	StartTurnSideEffects(ctx context.Context, messages []types.Message, opts TurnSideEffectOptions)
}

type TurnSideEffectOptions struct {
	SessionID string
	AgentID   string
	AgentType string
}

type hookPostSamplingRunner struct {
	runner  *hooks.Runner
	onEvent func(stream.Event)
}

func newHookPostSamplingRunner(runner *hooks.Runner, onEvent func(stream.Event)) *hookPostSamplingRunner {
	return &hookPostSamplingRunner{runner: runner, onEvent: onEvent}
}

func (r *hookPostSamplingRunner) RunPostSampling(ctx context.Context, messages []types.Message, opts PostSamplingOptions) PostSamplingResult {
	if r == nil || r.runner == nil || !r.runner.HasHooks(hooks.HookPostSampling) {
		return PostSamplingResult{}
	}
	executions := r.runner.RunDetailed(ctx, hooks.HookPostSampling, hooks.HookInput{
		SessionID:  opts.SessionID,
		Messages:   hookMessages(messages),
		TurnID:     opts.TurnID,
		WorkUnitID: opts.WorkUnitID,
		AgentID:    opts.AgentID,
		AgentType:  opts.AgentType,
	})
	outputs := hookExecutionOutputs(executions)
	result := summarizePostSamplingOutputs(outputs, i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyAuxPostSamplingBlocked))
	r.emitHookSummaries(hooks.HookPostSampling, executions, opts.TurnID, opts.AgentID, opts.AgentType, opts.WorkUnitID)
	return result
}

func (r *hookPostSamplingRunner) RunStopFailure(ctx context.Context, message types.Message, opts StopFailureOptions) {
	if r == nil || r.runner == nil || !r.runner.HasHooks(hooks.HookStopFailure) {
		return
	}
	executions := r.runner.RunDetailed(ctx, hooks.HookStopFailure, hooks.HookInput{
		SessionID:  opts.SessionID,
		Result:     strings.TrimSpace(message.GetText()),
		Messages:   hookMessages([]types.Message{message}),
		TurnID:     opts.TurnID,
		WorkUnitID: opts.WorkUnitID,
		AgentID:    opts.AgentID,
		AgentType:  opts.AgentType,
	})
	r.emitHookSummaries(hooks.HookStopFailure, executions, opts.TurnID, opts.AgentID, opts.AgentType, opts.WorkUnitID)
}

func hookExecutionOutputs(executions []hooks.HookExecution) []hooks.HookOutput {
	outputs := make([]hooks.HookOutput, 0, len(executions))
	for _, execution := range executions {
		outputs = append(outputs, execution.Output)
	}
	return outputs
}

func summarizePostSamplingOutputs(outputs []hooks.HookOutput, defaultBlockedReason string) PostSamplingResult {
	result := PostSamplingResult{}
	for _, output := range outputs {
		if output.Block || output.ExitCode == 2 {
			result.Blocked = true
			result.Reason = firstNonEmpty(result.Reason, strings.TrimSpace(output.Stderr), strings.TrimSpace(output.ExecutionError), strings.TrimSpace(output.SystemReminder), defaultBlockedReason)
			continue
		}
		if output.ExitCode != 0 || output.ExecutionError != "" {
			result.Failed = true
			result.Reason = firstNonEmpty(result.Reason, strings.TrimSpace(output.Stderr), strings.TrimSpace(output.ExecutionError), strings.TrimSpace(output.SystemReminder), i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyAuxHookExecutionFailed))
		}
	}
	return result
}

func (r *hookPostSamplingRunner) emitHookSummaries(hookType hooks.HookType, executions []hooks.HookExecution, turnID, actorID, actorType, workUnitID string) {
	if r.onEvent == nil {
		return
	}
	for _, execution := range executions {
		summary := newHookExecutionSummary(hookType, execution, localizedHookSummaryDefaults())
		r.onEvent(stream.Event{
			Type: stream.EventHookSummary, TurnID: turnID, ActorID: actorID, ActorType: actorType, WorkUnitID: workUnitID,
			HookSummary: &summary,
		})
	}
}

type hookSummaryDefaults struct {
	Blocked   string
	Prevented string
	Failed    string
}

// NewHookExecutionSummary converts one concrete hook configuration execution
// into the canonical structured event used by UI, SDK, and REPL boundaries.
// Keeping this conversion here prevents call sites from drifting on status,
// reason selection, or exact evidence metadata.
func NewHookExecutionSummary(hookType hooks.HookType, execution hooks.HookExecution) stream.HookSummaryEvent {
	return newHookExecutionSummary(hookType, execution, localizedHookSummaryDefaults())
}

func localizedHookSummaryDefaults() hookSummaryDefaults {
	lang := i18n.DetectOrLoadLanguage()
	return hookSummaryDefaults{
		Blocked:   i18n.Text(lang, i18n.KeyAuxHookBlockedContinuation),
		Prevented: i18n.Text(lang, i18n.KeyAuxHookPreventedContinuation),
		Failed:    i18n.Text(lang, i18n.KeyAuxHookExecutionFailed),
	}
}

func localizedToolHookSummaryDefaults() hookSummaryDefaults {
	lang := i18n.DetectOrLoadLanguage()
	return hookSummaryDefaults{
		Blocked:   i18n.Text(lang, i18n.KeyAuxToolHookBlocked),
		Prevented: i18n.Text(lang, i18n.KeyAuxToolHookPrevented),
		Failed:    i18n.Text(lang, i18n.KeyAuxHookExecutionFailed),
	}
}

func newHookExecutionSummary(hookType hooks.HookType, execution hooks.HookExecution, defaults hookSummaryDefaults) stream.HookSummaryEvent {
	output := execution.Output
	status := "passed"
	summary := firstNonEmpty(strings.TrimSpace(output.SystemReminder), strings.TrimSpace(output.AdditionalContext), strings.TrimSpace(output.UserDisplayMessage))
	switch {
	case output.PreventContinuation:
		status = "prevented"
		summary = firstNonEmpty(strings.TrimSpace(output.StopReason), summary, defaults.Prevented)
	case output.Block || output.ExitCode == 2 || strings.EqualFold(output.PermissionBehavior, "deny") || strings.EqualFold(output.PermissionBehavior, "block"):
		status = "blocked"
		summary = firstNonEmpty(strings.TrimSpace(output.Stderr), strings.TrimSpace(output.ExecutionError), summary, strings.TrimSpace(output.StopReason), defaults.Blocked)
	case output.ExitCode != 0 || output.ExecutionError != "":
		status = "failed"
		summary = firstNonEmpty(strings.TrimSpace(output.Stderr), strings.TrimSpace(output.ExecutionError), summary, defaults.Failed)
	}
	kind := execution.Hook.Kind
	if kind == "" {
		kind = hooks.HookKindCommand
	}
	metadata := map[string]any{
		"config_id":            execution.ConfigID,
		"config_index":         execution.ConfigIndex,
		"hook_kind":            string(kind),
		"exit_code":            output.ExitCode,
		"blocked":              output.Block || output.ExitCode == 2 || strings.EqualFold(output.PermissionBehavior, "deny") || strings.EqualFold(output.PermissionBehavior, "block"),
		"prevent_continuation": output.PreventContinuation,
		"hook_input":           execution.Input,
		"hook_output":          output,
		"hook_config":          execution.Hook,
	}
	if execution.Hook.Matcher != "" {
		metadata["matcher"] = execution.Hook.Matcher
	}
	input := execution.Input
	for key, value := range map[string]string{
		"task_id":           input.TaskID,
		"task_subject":      input.TaskSubject,
		"task_owner":        input.TaskOwner,
		"teammate_name":     input.TeammateName,
		"team_name":         input.TeamName,
		"hook_config_id":    input.HookConfigID,
		"hook_execution_id": input.HookExecutionID,
	} {
		if value != "" {
			metadata[key] = value
		}
	}
	return stream.HookSummaryEvent{
		HookExecutionID: execution.ExecutionID,
		ToolUseID:       execution.Input.ToolUseID,
		HookName:        string(hookType),
		Status:          status,
		Summary:         summary,
		Metadata:        metadata,
	}
}

func hookMessages(messages []types.Message) []any {
	out := make([]any, 0, len(messages))
	for _, msg := range messages {
		out = append(out, hookMessage(msg))
	}
	return out
}

func hookMessage(msg types.Message) any {
	// Provider continuation state is wire-only. Hooks receive the visible
	// projection, never encrypted signatures or provider-native replay items.
	msg = stripThinkingSignatures([]types.Message{msg})[0]
	data, err := json.Marshal(msg)
	if err != nil {
		return map[string]any{
			"role": string(msg.Role),
			"text": msg.GetText(),
		}
	}
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		return map[string]any{
			"role": string(msg.Role),
			"text": msg.GetText(),
		}
	}
	return decoded
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
