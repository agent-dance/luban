// Package ui provides terminal rendering abstractions for the CLI layer.
// Future TUI frameworks (e.g. Bubble Tea) only need a new presentation.Renderer implementation.
package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"sync"

	"github.com/agent-dance/luban/internal/contracts/interaction"
	"github.com/agent-dance/luban/internal/presentation"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/permissions"
	"github.com/agent-dance/luban/runtimeevent"
	"github.com/agent-dance/luban/types"
)

// JSONRenderer writes each rendering event as a newline-delimited JSON object
// (NDJSON) to the supplied writer. It implements the presentation.Renderer interface and is
// used when --output-format=json or --output-format=stream-json is requested.
//
// Line format examples:
//
//	{"type":"text","content":"Hello"}
//	{"type":"tool_use","name":"Bash","input":{"command":"ls"}}
//	{"type":"tool_result","name":"Bash","output":"file.txt","is_error":false}
//	{"type":"thinking","content":"..."}
//	{"type":"cost","turn":0.003,"total":0.12}
type JSONRenderer struct {
	mu        sync.Mutex
	w         io.Writer
	sessionID string
}

// NewJSONRenderer creates a JSONRenderer that writes NDJSON to w.
func NewJSONRenderer(w io.Writer) *JSONRenderer {
	return &JSONRenderer{w: w}
}

// writeLine marshals v to JSON and writes it followed by a newline.
// Marshal errors are logged; write errors are logged and silently ignored
// to keep the hot path clean.
func (r *JSONRenderer) writeLine(v any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.writeLineLocked(v)
}

func (r *JSONRenderer) writeLineLocked(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		log.Print(i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeJSONMarshalLog, err))
		return
	}
	if _, err := fmt.Fprintf(r.w, "%s\n", b); err != nil {
		log.Print(i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeJSONWriteLog, err))
	}
}

func (r *JSONRenderer) eventIdentity(ctx presentation.ToolEventContext) map[string]any {
	sessionID := ctx.SessionID
	if sessionID == "" {
		r.mu.Lock()
		sessionID = r.sessionID
		r.mu.Unlock()
	}
	identity := make(map[string]any, 6)
	if sessionID != "" {
		identity["session_id"] = sessionID
	}
	if ctx.ProjectRoot != "" {
		identity["project_root"] = ctx.ProjectRoot
	}
	if ctx.TurnID != "" {
		identity["turn_id"] = ctx.TurnID
	}
	if ctx.ActorID != "" {
		identity["actor_id"] = ctx.ActorID
	}
	if ctx.ActorType != "" {
		identity["actor_type"] = ctx.ActorType
	}
	if ctx.WorkUnitID != "" {
		identity["work_unit_id"] = ctx.WorkUnitID
	}
	return identity
}

// --- presentation.Renderer interface implementation ---

func (r *JSONRenderer) Text(s string) {
	r.writeLine(map[string]any{"type": "text", "content": s})
}

func (r *JSONRenderer) Thinking(s string) {
	r.writeLine(map[string]any{"type": "thinking", "content": s})
}

func (r *JSONRenderer) Error(s string) {
	r.writeLine(map[string]any{"type": "error", "message": s})
}

func (r *JSONRenderer) Info(s string) {
	r.writeLine(map[string]any{"type": "info", "message": s})
}

func (r *JSONRenderer) Success(s string) {
	r.writeLine(map[string]any{"type": "success", "message": s})
}

func (r *JSONRenderer) Warning(s string) {
	r.writeLine(map[string]any{"type": "warning", "message": s})
}

func (r *JSONRenderer) Bold(s string) {
	r.writeLine(map[string]any{"type": "bold", "content": s})
}

// RenderToolCall preserves the durable execution identity and complete input
// for machine consumers.
func (r *JSONRenderer) RenderToolCall(ctx presentation.ToolEventContext, call types.ToolUseBlock) {
	event := r.eventIdentity(ctx)
	event["type"] = "tool_use"
	event["tool_use_id"] = call.ID
	event["name"] = call.Name
	event["input"] = call.Input
	r.writeLine(event)
}

// RenderToolResult keeps the explicit outcome and lossless result envelope in
// NDJSON instead of reducing it to an uncorrelated output string.
func (r *JSONRenderer) RenderToolResult(ctx presentation.ToolEventContext, result types.ToolResultBlock) {
	runtimeResult := types.NewToolResultRuntimeEvent(ctx.RuntimeIdentity(result.ToolUseID), result, i18n.KeyRuntimeToolResultPublicSummary, nil)
	projection, err := runtimeevent.NewAudienceProjector().Project(runtimeResult, runtimeevent.ProjectionOptions{
		Audience: runtimeevent.AudienceSDK, Redaction: runtimeevent.RedactionStrict,
	})
	if err != nil {
		// A result without an authoritative outcome is not emitted through the
		// retired flat-only envelope. Surface a safe semantic invariant failure.
		r.RuntimeErrorEvent(ctx, result.ToolUseID, "", nil, nil)
		return
	}

	event := r.eventIdentity(ctx)
	event["type"] = "tool_result"
	event["tool_use_id"] = result.ToolUseID
	event["output"] = result.TextContent()
	event["is_error"] = presentation.ToolOutcomeIsError(result.Outcome)
	if result.Content != "" {
		event["content"] = result.Content
	}
	if result.Outcome != "" {
		event["outcome"] = result.Outcome
	}
	if len(result.ContentBlocks) > 0 {
		event["content_blocks"] = result.ContentBlocks
	}
	if result.Data != nil {
		event["data"] = result.Data
	}
	if len(result.Metadata) > 0 {
		event["metadata"] = result.Metadata
	}
	if result.Usage != nil {
		event["usage"] = result.Usage
	}
	if len(result.NewMessages) > 0 {
		event["new_messages"] = result.NewMessages
	}
	attachRuntimeProjection(event, projection)
	r.writeLine(event)
}

func attachRuntimeProjection(event map[string]any, projection runtimeevent.ProjectedRuntimeEvent) {
	event["schema_version"] = projection.SchemaVersion
	event["audience"] = projection.Audience
	event["redaction_level"] = projection.RedactionLevel
	event["event_id"] = projection.EventID
	event["kind"] = projection.Kind
	event["code"] = projection.Code
	event["message"] = projection.Message
	if projection.PublicKey != "" {
		event["public_key"] = projection.PublicKey
	}
	if len(projection.PublicArgs) > 0 {
		event["public_args"] = projection.PublicArgs
	}
	if projection.Epoch != 0 {
		event["epoch"] = projection.Epoch
	}
	if projection.ContextGeneration != 0 {
		event["context_generation"] = projection.ContextGeneration
	}
	if projection.Outcome != "" {
		event["outcome"] = projection.Outcome
	}
}

// RenderHookSummary keeps hook execution identity separate from its source
// tool identity so concurrent hooks remain independently traceable.
func (r *JSONRenderer) RenderHookSummary(ctx presentation.ToolEventContext, summary presentation.HookSummary) {
	event := r.eventIdentity(ctx)
	event["type"] = "hook_summary"
	event["hook_execution_id"] = summary.ExecutionID
	if summary.ToolUseID != "" {
		event["tool_use_id"] = summary.ToolUseID
	}
	event["hook_name"] = summary.Name
	event["status"] = summary.Status
	event["summary"] = summary.Summary
	if len(summary.Metadata) > 0 {
		event["metadata"] = summary.Metadata
	}
	r.writeLine(event)
}

// RuntimeErrorEvent is a user-audience projection. Private API errors,
// metadata, paths, and correlation identifiers belong in the audit sink, not
// the default JSON presentation stream.
func (r *JSONRenderer) RuntimeErrorEvent(ctx presentation.ToolEventContext, toolUseID, message string, apiError *types.APIError, metadata map[string]any) {
	event := presentation.NewRuntimeErrorEvent(ctx, toolUseID, message, apiError, metadata)
	projection, err := runtimeevent.NewAudienceProjector().Project(event, runtimeevent.ProjectionOptions{
		Audience: runtimeevent.AudienceUser, Redaction: runtimeevent.RedactionStrict,
	})
	if err != nil {
		// Constructor-produced runtime errors are valid by construction. Keep a
		// semantic fail-closed fallback in case a future schema migration drifts.
		r.writeLine(map[string]any{
			"type": "error", "schema_version": types.RuntimeEventSchemaVersion,
			"audience": runtimeevent.AudienceUser, "redaction_level": runtimeevent.RedactionStrict,
			"kind": types.RuntimeEventKindError, "outcome": types.ToolOutcomeFailed,
			"code":    "runtime.operation_failed",
			"message": i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeErrorPublicSummary),
		})
		return
	}
	r.writeLine(projection)
}

func (r *JSONRenderer) sendUserMessageEvent(ctx presentation.ToolEventContext, output interaction.SendUserMessageOutput) map[string]any {
	event := r.eventIdentity(ctx)
	event["type"] = "assistant_message"
	event["message"] = output.Message
	if len(output.Attachments) > 0 {
		event["attachments"] = output.Attachments
	}
	if output.SentAt != "" {
		event["sent_at"] = output.SentAt
	}
	return event
}

func (r *JSONRenderer) RenderSendUserMessageEvent(ctx presentation.ToolEventContext, output interaction.SendUserMessageOutput, _ presentation.SendUserMessageRenderOptions) {
	r.writeLine(r.sendUserMessageEvent(ctx, output))
}

// RenderHiddenToolCall retains the protocol identity of Brief while marking it
// hidden so presentation clients can omit generic tool chrome.
func (r *JSONRenderer) RenderHiddenToolCall(ctx presentation.ToolEventContext, call types.ToolUseBlock) {
	event := r.eventIdentity(ctx)
	event["type"] = "tool_use"
	event["tool_use_id"] = call.ID
	event["name"] = call.Name
	event["input"] = call.Input
	event["hidden"] = true
	r.writeLine(event)
}

// RenderSendUserMessageToolEvent correlates the visible message with the
// canonical tool-result projection without embedding the retired raw result
// envelope or exposing the internal acknowledgement text.
func (r *JSONRenderer) RenderSendUserMessageToolEvent(ctx presentation.ToolEventContext, result types.ToolResultBlock, output interaction.SendUserMessageOutput, _ presentation.SendUserMessageRenderOptions) {
	runtimeResult := types.NewToolResultRuntimeEvent(ctx.RuntimeIdentity(result.ToolUseID), result, i18n.KeyRuntimeToolResultPublicSummary, nil)
	projection, err := runtimeevent.NewAudienceProjector().Project(runtimeResult, runtimeevent.ProjectionOptions{
		Audience: runtimeevent.AudienceSDK, Redaction: runtimeevent.RedactionStrict,
	})
	if err != nil {
		r.RuntimeErrorEvent(ctx, result.ToolUseID, "", nil, nil)
		return
	}
	event := r.sendUserMessageEvent(ctx, output)
	event["tool_use_id"] = result.ToolUseID
	event["runtime_event"] = projection
	r.writeLine(event)
}

func (r *JSONRenderer) Usage(u *types.Usage) {
	if u == nil {
		return
	}
	r.writeLine(map[string]any{
		"type":                        "usage",
		"scope":                       presentation.UsageScopeLastRequest,
		"known":                       true,
		"input_tokens":                u.InputTokens,
		"output_tokens":               u.OutputTokens,
		"cache_read_input_tokens":     u.CacheReadInputTokens,
		"cache_creation_input_tokens": u.CacheCreationInputTokens,
		"cache_hit_percent":           usageCacheHitPercent(u.TotalInputTokens(), u.CacheReadInputTokens),
	})
}

// UsageSemantics emits one scope-safe usage event. Consumers no longer need
// to infer which request/session/context denominator belongs to a number.
func (r *JSONRenderer) UsageSemantics(snapshot presentation.UsageSemanticsSnapshot) {
	r.writeLine(map[string]any{
		"type":               "usage_summary",
		"schema_version":     snapshot.SchemaVersion,
		"last_request":       snapshot.LastRequest,
		"cumulative_session": snapshot.CumulativeSession,
		"model_context":      snapshot.ModelContext,
	})
}

func (r *JSONRenderer) ModelContext(context presentation.ModelContextProjection) {
	r.writeLine(map[string]any{
		"type": "context_usage", "schema_version": presentation.UsageSemanticsSchemaVersion,
		"model_context": context,
	})
}

func (r *JSONRenderer) Banner(provider, model string) {
	r.writeLine(map[string]any{"type": "banner", "provider": provider, "model": model})
}

func (r *JSONRenderer) SessionInfo(id string, tools []string) {
	r.mu.Lock()
	r.sessionID = id
	r.writeLineLocked(map[string]any{"type": "session_info", "session_id": id, "tools": tools})
	r.mu.Unlock()
}

func (r *JSONRenderer) Prompt() string { return "" }

func (r *JSONRenderer) Newline() {
	// Newlines have no meaning in NDJSON output; emit nothing.
}

func (r *JSONRenderer) Goodbye() {
	r.writeLine(map[string]any{"type": "goodbye"})
}

func (r *JSONRenderer) CostSummary(turnCost, cumulativeCost float64, inputTokens, outputTokens int) {
	r.writeLine(map[string]any{
		"type":                       "cost",
		"turn":                       turnCost,
		"turn_scope":                 presentation.UsageScopeLastRequest,
		"total":                      cumulativeCost,
		"total_scope":                presentation.UsageScopeCumulativeSession,
		"last_request_input_tokens":  inputTokens,
		"last_request_output_tokens": outputTokens,
	})
}

func (r *JSONRenderer) ContextBar(usedTokens, maxTokens int) {
	r.writeLine(map[string]any{
		"type":            "context_bar",
		"scope":           presentation.UsageScopeModelContext,
		"used_tokens":     usedTokens,
		"capacity_tokens": maxTokens,
		"percent_used":    boundedUsagePercent(usedTokens, maxTokens),
	})
}

// SpinnerStart is a no-op in JSON mode; spinners have no meaning in structured
// output. Returns a no-op stop function.
func (r *JSONRenderer) SpinnerStart(_ string) func() { return func() {} }

// DecisionRequest emits the complete permission/plan review surface and an
// explicit non-interactive result. JSON output cannot collect terminal input,
// so it deterministically rejects while preserving the audit identity.
func (r *JSONRenderer) DecisionRequest(_ context.Context, request permissions.PromptRequest) permissions.PromptResponse {
	r.writeLine(map[string]any{
		"type":                 "decision_request",
		"decision_id":          request.DecisionID,
		"session_id":           request.SessionID,
		"execution_session_id": request.ExecutionSessionID,
		"turn_id":              request.TurnID,
		"tool_use_id":          request.ToolUseID,
		"tool":                 request.ToolName,
		"input":                request.Input,
		"actor_id":             request.ActorID,
		"actor_type":           request.ActorType,
		"work_unit_id":         request.WorkUnitID,
		"kind":                 request.Kind,
		"action":               request.Action,
		"target":               request.Target,
		"impact":               request.Impact,
		"risk_level":           request.RiskLevel,
		"risk_reason":          request.RiskReason,
		"rule_source":          request.RuleSource,
		"approval_scope":       request.ApprovalScope,
		"choices":              request.Choices,
		"body":                 request.Body,
		"review_details":       request.ReviewDetails,
		"post_mode":            request.PostMode,
		"message":              request.Message,
	})
	response := permissions.PromptResponse{
		DecisionID: request.DecisionID,
		Decision:   permissions.DecisionDeny,
		Outcome:    permissions.PromptOutcomeRejected,
	}
	r.writeLine(map[string]any{
		"type":         "decision_result",
		"decision_id":  request.DecisionID,
		"session_id":   request.SessionID,
		"turn_id":      request.TurnID,
		"tool_use_id":  request.ToolUseID,
		"actor_id":     request.ActorID,
		"actor_type":   request.ActorType,
		"work_unit_id": request.WorkUnitID,
		"decision":     "deny",
		"outcome":      response.Outcome,
	})
	return response
}
