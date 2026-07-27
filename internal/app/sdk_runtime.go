package app

import (
	"context"
	"errors"
	"sync"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/contracts/permission"
	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/internal/runtime/engine"
	"github.com/agent-dance/luban/prompt"
	"github.com/agent-dance/luban/runtimeevent"
	"github.com/agent-dance/luban/sdk"
	"github.com/agent-dance/luban/types"
)

// sdkRuntimeAdapter adapts the application engine to the public SDK boundary.
type sdkRuntimeAdapter struct {
	engine engine.Engine

	promptMu       sync.RWMutex
	promptOverride string
}

// newSDKRuntime creates the one-way application-to-SDK adapter.
func newSDKRuntime(runtime engine.Engine) sdk.Runtime {
	return &sdkRuntimeAdapter{engine: runtime}
}

func (r *sdkRuntimeAdapter) Query(ctx context.Context, request sdk.QueryRequest) (<-chan sdk.QueryEvent, error) {
	r.promptMu.RLock()
	promptOverride := r.promptOverride
	r.promptMu.RUnlock()

	stream, err := r.engine.Query(ctx, engine.QueryRequest{
		SessionID: request.SessionID, Message: request.Message, SystemPromptOverride: promptOverride,
	})
	if err != nil {
		return nil, sdkRuntimeError(err)
	}
	out := make(chan sdk.QueryEvent)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-stream:
				if !ok {
					return
				}
				converted := sdk.QueryEvent{
					SessionID: event.SessionID,
					Event:     sdkEventFromStream(event.Inner),
					Final:     event.Final,
					Error:     sdkRuntimeError(event.Error),
				}
				select {
				case out <- converted:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}

func (r *sdkRuntimeAdapter) Resume(ctx context.Context, sessionID string) (int, error) {
	count, err := r.engine.Resume(ctx, sessionID)
	return count, sdkRuntimeError(err)
}

func (r *sdkRuntimeAdapter) Compact(ctx context.Context, sessionID string) (sdk.CompactResult, error) {
	result, err := r.engine.Compact(ctx, sessionID)
	if err != nil {
		return sdk.CompactResult{}, sdkRuntimeError(err)
	}
	return sdk.CompactResult{
		Compacted: result.Compacted, BeforeMessageCount: result.BeforeMessageCount,
		AfterMessageCount: result.AfterMessageCount, ContextGeneration: result.ContextGeneration,
	}, nil
}

func (r *sdkRuntimeAdapter) Interrupt(sessionID string) { r.engine.Interrupt(sessionID) }

func (r *sdkRuntimeAdapter) SetModel(sessionID, model string) error {
	return sdkRuntimeError(r.engine.SetModel(sessionID, model))
}

func (r *sdkRuntimeAdapter) SetThinkingConfig(sessionID string, enabled bool, budget int) error {
	return sdkRuntimeError(r.engine.SetThinkingConfig(sessionID, enabled, budget))
}

func (r *sdkRuntimeAdapter) ContextUsage(sessionID string) (*sdk.ContextUsageInfo, error) {
	usage, err := r.engine.ContextUsage(sessionID)
	if err != nil {
		return nil, sdkRuntimeError(err)
	}
	if usage == nil {
		return nil, nil
	}
	return &sdk.ContextUsageInfo{
		TotalTokens: usage.TotalTokens, UsedTokens: usage.UsedTokens, RemainingTokens: usage.RemainingTokens,
		Measurement: usage.Measurement,
	}, nil
}

func (r *sdkRuntimeAdapter) Tools() []string { return append([]string(nil), r.engine.Tools()...) }

func (r *sdkRuntimeAdapter) ModelID() string {
	provider := r.engine.Provider()
	if provider == nil {
		return ""
	}
	return provider.ModelID()
}

func (r *sdkRuntimeAdapter) SetSystemPrompt(custom string) {
	effective := prompt.BuildEffectiveSystemPrompt(prompt.EffectiveSystemPromptInput{Custom: custom})
	if configurable, ok := r.engine.(engine.SystemPromptConfigurable); ok {
		configurable.SetSystemPrompt(effective)
		r.promptMu.Lock()
		r.promptOverride = ""
		r.promptMu.Unlock()
		return
	}
	r.promptMu.Lock()
	r.promptOverride = effective.JoinedText()
	r.promptMu.Unlock()
}

func (r *sdkRuntimeAdapter) SetPermission(handler sdk.PermissionHandler) {
	if handler == nil {
		r.engine.SetPermission(nil)
		return
	}
	r.engine.SetPermission(appSDKPermissionAdapter{handler: handler})
}

type appSDKPermissionAdapter struct {
	handler sdk.PermissionHandler
}

func (a appSDKPermissionAdapter) Check(ctx context.Context, request permission.PermissionRequest) (permission.PermissionDecision, error) {
	decision, err := a.handler.Check(ctx, sdkPermissionRequest(request))
	switch decision {
	case sdk.PermissionAllow:
		return permission.PermissionAllow, err
	case sdk.PermissionDeny:
		return permission.PermissionDeny, err
	case sdk.PermissionAllowOnce:
		return permission.PermissionAllowOnce, err
	default:
		return permission.PermissionDeny, err
	}
}

func sdkPermissionRequest(request permission.PermissionRequest) sdk.PermissionRequest {
	updates := make([]sdk.PermissionUpdate, len(request.Suggestions))
	for index, update := range request.Suggestions {
		rules := make([]sdk.PermissionRuleValue, len(update.Rules))
		for ruleIndex, rule := range update.Rules {
			rules[ruleIndex] = sdk.PermissionRuleValue{ToolName: rule.ToolName, RuleContent: rule.RuleContent}
		}
		updates[index] = sdk.PermissionUpdate{
			Type: string(update.Type), Destination: string(update.Destination), Behavior: string(update.Behavior),
			Rules: rules, Directories: append([]string(nil), update.Directories...), Mode: update.Mode,
		}
	}
	return sdk.PermissionRequest{
		SessionID: request.SessionID, ExecutionSessionID: request.ExecutionSessionID,
		TurnID: request.TurnID, DecisionID: request.DecisionID, ToolUseID: request.ToolUseID,
		ToolName: request.ToolName, Input: cloneSDKMetadata(request.Input), ActorID: request.ActorID, ActorType: request.ActorType,
		WorkUnitID: request.WorkUnitID, Kind: request.Kind, Action: request.Action, Target: request.Target,
		Impact: request.Impact, RiskReason: request.RiskReason, RuleSource: request.RuleSource,
		ApprovalScope: request.ApprovalScope, Choices: append([]string(nil), request.Choices...), Body: request.Body,
		ReviewDetails: append([]string(nil), request.ReviewDetails...), PostMode: request.PostMode,
		Description: request.Description, Mode: request.Mode, AvoidPrompts: request.AvoidPrompts,
		Message: request.Message, Suggestions: updates, BlockedPath: request.BlockedPath,
	}
}

func sdkRuntimeError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, engine.ErrSessionDeleted):
		return i18n.WrapInternalError(i18n.KeyAuxEngineSessionDeleted, err)
	case errors.Is(err, engine.ErrSessionNotFound):
		return i18n.WrapInternalError(i18n.KeyAuxEngineSessionNotFound, err)
	case errors.Is(err, engine.ErrShutdown):
		return i18n.WrapInternalError(i18n.KeyAuxEngineShutdown, err)
	case errors.Is(err, engine.ErrNoProvider):
		return i18n.WrapInternalError(i18n.KeyAuxEngineNoProvider, err)
	default:
		return err
	}
}

func sdkEventFromStream(event stream.Event) sdk.Event {
	out := sdk.Event{
		Type: sdk.EventType(event.Type), Text: event.Text, TurnCount: event.TurnCount,
		ProjectRoot: event.ProjectRoot, TurnID: event.TurnID, ActorID: event.ActorID,
		ActorType: event.ActorType, WorkUnitID: event.WorkUnitID, ToolUseID: event.ToolUseID,
		TerminalReason: event.TerminalReason, Metadata: cloneSDKMetadata(event.Metadata),
		RequestStatus: sdkRequestStatus(event.Type, event.RequestStatus),
		ToolRound:     sdkToolRoundMetrics(event.ToolRound),
	}
	if event.ToolUse != nil {
		out.ToolUse = &sdk.ToolUse{ID: event.ToolUse.ID, Name: event.ToolUse.Name, Input: cloneSDKMetadata(event.ToolUse.Input)}
	}
	if event.ToolResult != nil {
		blocks := make([]any, len(event.ToolResult.ContentBlocks))
		for index, block := range event.ToolResult.ContentBlocks {
			blocks[index] = block
		}
		out.ToolResult = &sdk.ToolResult{
			ToolUseID: event.ToolResult.ToolUseID, Content: event.ToolResult.TextContent(), ContentBlocks: blocks,
			Data: event.ToolResult.Data, Metadata: cloneSDKStringMap(event.ToolResult.Metadata),
			Usage: sdkUsage(event.ToolResult.Usage), Outcome: sdk.ToolOutcome(event.ToolResult.Outcome),
		}
	}
	out.Usage = sdkUsage(event.Usage)
	if event.Error != nil {
		out.Error = &sdk.APIError{
			Type: event.Error.Type, Message: event.Error.Message, Status: event.Error.Status,
			RetryAfter: event.Error.RetryAfter, OriginalModel: event.Error.OriginalModel,
			FallbackModel: event.Error.FallbackModel,
		}
	}
	out.Compact = sdkCompactBoundary(event.Compact)
	if event.MaxTurns != nil {
		out.MaxTurns = &sdk.MaxTurnsEvent{MaxTurns: event.MaxTurns.MaxTurns, TurnCount: event.MaxTurns.TurnCount}
	}
	if event.Tombstone != nil {
		out.Tombstone = &sdk.TombstoneEvent{
			Reason: event.Tombstone.Reason, Summary: event.Tombstone.Summary,
			Metadata: cloneSDKMetadata(event.Tombstone.Metadata),
		}
	}
	if event.ToolSummary != nil {
		out.ToolSummary = &sdk.ToolUseSummaryEvent{
			ToolUseID: event.ToolSummary.ToolUseID, ToolName: event.ToolSummary.ToolName,
			Status: event.ToolSummary.Status, OutputSummary: event.ToolSummary.OutputSummary,
		}
	}
	if event.HookSummary != nil {
		out.HookSummary = &sdk.HookSummaryEvent{
			HookExecutionID: event.HookSummary.HookExecutionID, ToolUseID: event.HookSummary.ToolUseID,
			HookName: event.HookSummary.HookName, Status: event.HookSummary.Status,
			Summary: event.HookSummary.Summary, Metadata: cloneSDKMetadata(event.HookSummary.Metadata),
		}
	}
	if event.Progress != nil {
		out.Progress = &sdk.RuntimeProgressEvent{
			Stage: event.Progress.Stage, Message: event.Progress.Message, Current: event.Progress.Current,
			Total: event.Progress.Total, Disposition: event.Progress.Disposition, Blocker: event.Progress.Blocker,
			MutationEpoch: event.Progress.MutationEpoch, VerifiedEpoch: event.Progress.VerifiedEpoch,
			Metadata: cloneSDKMetadata(event.Progress.Metadata),
		}
	}
	if event.Type == stream.EventSystemWarning {
		warning := runtimeevent.SystemWarningRuntimeEvent(event)
		out.RuntimeEvent = sdkRuntimeEvent(warning)
	} else if event.RuntimeEvent != nil {
		out.RuntimeEvent = sdkRuntimeEvent(*event.RuntimeEvent)
	}
	return out
}

func sdkRequestStatus(eventType stream.EventType, source *stream.RequestStatusEvent) *sdk.RequestStatusEvent {
	var phase, status string
	switch eventType {
	case stream.EventRequestStart:
		phase, status = string(eventType), "started"
	case stream.EventRequestRetry:
		phase, status = string(eventType), "retrying"
	case stream.EventRequestFirstToken:
		phase, status = string(eventType), "streaming"
	case stream.EventRequestEnd:
		phase, status = string(eventType), "completed"
	case stream.EventRequestFailed:
		phase, status = string(eventType), "failed"
	default:
		return nil
	}
	out := &sdk.RequestStatusEvent{Phase: phase, Status: status, Attempt: 1, MaxAttempts: 1}
	if source == nil {
		return out
	}
	out.RequestID = source.RequestID
	out.StartedAt = source.StartedAt
	out.EndedAt = source.EndedAt
	if source.Attempt > 0 {
		out.Attempt = source.Attempt
	}
	if source.MaxRetries > 0 {
		out.MaxAttempts = source.MaxRetries + 1
	}
	out.RetryDelayMilliseconds = source.RetryDelayMilliseconds
	out.RetryCount = source.RetryCount
	out.RequestMilliseconds = source.RequestMilliseconds
	out.FirstTokenMilliseconds = source.FirstTokenMilliseconds
	out.TotalMilliseconds = source.TotalMilliseconds
	out.InputTokens = source.InputTokens
	out.CacheReadInputTokens = source.CacheReadInputTokens
	out.CacheWriteInputTokens = source.CacheWriteInputTokens
	out.OutputTokens = source.OutputTokens
	if source.Error != "" {
		switch eventType {
		case stream.EventRequestRetry:
			out.ErrorCode = "provider_request_retry"
		case stream.EventRequestFailed:
			out.ErrorCode = "provider_request_failed"
		}
	}
	return out
}

func sdkToolRoundMetrics(source *stream.ToolRoundMetricsEvent) *sdk.ToolRoundMetricsEvent {
	if source == nil {
		return nil
	}
	return &sdk.ToolRoundMetricsEvent{
		RoundID:                       source.RoundID,
		LogicalModelVisibleCalls:      source.LogicalModelVisibleCalls,
		PhysicalChildOperations:       source.PhysicalChildOperations,
		Fanout:                        source.Fanout,
		BatchCount:                    source.BatchCount,
		QueueMilliseconds:             source.QueueMilliseconds,
		CriticalPathMilliseconds:      source.CriticalPathMilliseconds,
		TotalChildLatencyMilliseconds: source.TotalChildLatencyMilliseconds,
		ErrorCount:                    source.ErrorCount,
		RevisionFusionCount:           source.RevisionFusionCount,
		RevisionBarrierSkips:          source.RevisionBarrierSkips,
		RevisionMismatchCount:         source.RevisionMismatchCount,
	}
}

func sdkCompactBoundary(event *stream.CompactBoundaryEvent) *sdk.CompactBoundaryEvent {
	if event == nil {
		return nil
	}
	out := &sdk.CompactBoundaryEvent{
		BoundaryID: event.BoundaryID, Trigger: event.Trigger, PreCompactTokenCount: event.PreCompactTokenCount,
		PostCompactTokenCount:     event.PostCompactTokenCount,
		TruePostCompactTokenCount: event.TruePostCompactTokenCount,
		PreviousTailIdentifier:    event.PreviousTailIdentifier,
		PreCompactDiscoveredTools: append([]string(nil), event.PreCompactDiscoveredTools...),
		Summary:                   event.Summary,
		UserDisplayMessage:        event.UserDisplayMessage,
	}
	if event.PreservedSegment != nil {
		out.PreservedSegment = &sdk.PreservedSegmentMetadata{
			StartIndex: event.PreservedSegment.StartIndex,
			Count:      event.PreservedSegment.Count,
			Anchor:     event.PreservedSegment.Anchor,
			Direction:  event.PreservedSegment.Direction,
		}
	}
	return out
}

func sdkRuntimeEvent(event types.RuntimeEvent) *sdk.RuntimeEvent {
	out := &sdk.RuntimeEvent{
		SchemaVersion: event.SchemaVersion, Kind: string(event.Kind),
		RuntimeIdentity: sdk.RuntimeIdentity{
			EventID: event.EventID, SessionID: event.SessionID, Epoch: event.Epoch,
			ContextGeneration: event.ContextGeneration, TurnID: event.TurnID,
			ToolUseID: event.ToolUseID, WorkUnitID: event.WorkUnitID,
			ActorID: event.ActorID, ActorType: event.ActorType,
		},
		Outcome: sdk.ToolOutcome(event.Outcome), PublicKey: string(event.PublicKey),
		PublicArgs: append([]any(nil), event.PublicArgs...), DiagnosticCode: event.DiagnosticCode,
		PrivateCause: event.PrivateCause, PrivateMetadata: cloneSDKMetadata(event.PrivateMetadata),
	}
	if event.EvidenceRef != nil {
		out.EvidenceRef = &sdk.RuntimeEvidenceRef{ID: event.EvidenceRef.ID, Digest: event.EvidenceRef.Digest}
	}
	return out
}

func sdkUsage(usage *types.Usage) *sdk.Usage {
	if usage == nil {
		return nil
	}
	return &sdk.Usage{
		InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens,
		CacheCreationInputTokens: usage.CacheCreationInputTokens,
		CacheReadInputTokens:     usage.CacheReadInputTokens,
		ServerToolUse: sdk.ServerToolUsage{
			WebSearchRequests: usage.ServerToolUse.WebSearchRequests,
			WebFetchRequests:  usage.ServerToolUse.WebFetchRequests,
		},
	}
}

func cloneSDKMetadata(source map[string]any) map[string]any {
	if source == nil {
		return nil
	}
	out := make(map[string]any, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}

func cloneSDKStringMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	out := make(map[string]string, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}
