package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

func agentOutputVariantSchema(kind string, properties map[string]any, required ...string) map[string]any {
	base := map[string]any{
		"kind":           map[string]any{"type": "string", "enum": []string{kind}},
		"transcriptPath": map[string]any{"type": "string"},
		"durationMs":     map[string]any{"type": "integer"},
		"totalTokens":    map[string]any{"type": "integer"},
	}
	for name, schema := range properties {
		base[name] = schema
	}
	allRequired := append([]string{"kind", "durationMs", "totalTokens"}, required...)
	return map[string]any{
		"type":                 "object",
		"properties":           base,
		"required":             allRequired,
		"additionalProperties": false,
	}
}

func agentOutputSchema() types.JSONSchema {
	incompleteProperties := func(outcome string) map[string]any {
		return map[string]any{
			"status":            map[string]any{"type": "string", "enum": []string{outcome}},
			"agentId":           map[string]any{"type": "string"},
			"agentType":         map[string]any{"type": "string"},
			"prompt":            map[string]any{"type": "string"},
			"content":           map[string]any{"type": "array"},
			"outcome":           map[string]any{"type": "string", "enum": []string{outcome}},
			"reason":            map[string]any{"type": "string"},
			"totalToolUseCount": map[string]any{"type": "integer"},
			"usage":             map[string]any{"type": "object"},
			"latestToolUse":     map[string]any{"type": "string"},
			"artifactRefs":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"verificationRefs":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		}
	}
	return types.JSONSchema{Type: "object", AnyOf: []any{
		agentOutputVariantSchema("completed", map[string]any{
			"status":            map[string]any{"type": "string", "enum": []string{"completed"}},
			"agentId":           map[string]any{"type": "string"},
			"agentType":         map[string]any{"type": "string"},
			"prompt":            map[string]any{"type": "string"},
			"content":           map[string]any{"type": "array"},
			"totalToolUseCount": map[string]any{"type": "integer"},
			"usage":             map[string]any{"type": "object"},
			"cwd":               map[string]any{"type": "string"},
			"mode":              map[string]any{"type": "string"},
			"isolation":         map[string]any{"type": "string"},
			"model":             map[string]any{"type": "string"},
			"worktreePath":      map[string]any{"type": "string"},
			"worktreeBranch":    map[string]any{"type": "string"},
			"latestToolUse":     map[string]any{"type": "string"},
		}, "agentId", "content"),
		agentOutputVariantSchema("error", map[string]any{
			"status":     map[string]any{"type": "string", "enum": []string{"error"}},
			"agentId":    map[string]any{"type": "string"},
			"agentType":  map[string]any{"type": "string"},
			"message":    map[string]any{"type": "string"},
			"exitReason": map[string]any{"type": "string"},
		}, "message"),
		agentOutputVariantSchema("aborted", map[string]any{
			"status":        map[string]any{"type": "string", "enum": []string{"aborted"}},
			"agentId":       map[string]any{"type": "string"},
			"agentType":     map[string]any{"type": "string"},
			"reason":        map[string]any{"type": "string"},
			"latestToolUse": map[string]any{"type": "string"},
		}, "reason"),
		agentOutputVariantSchema("partial", map[string]any{
			"status":            map[string]any{"type": "string", "enum": []string{"async_launched", "remote_launched", "teammate_spawned", "partial"}},
			"agentId":           map[string]any{"type": "string"},
			"agentType":         map[string]any{"type": "string"},
			"taskId":            map[string]any{"type": "string"},
			"description":       map[string]any{"type": "string"},
			"prompt":            map[string]any{"type": "string"},
			"outputFile":        map[string]any{"type": "string"},
			"canReadOutputFile": map[string]any{"type": "boolean"},
			"isAsync":           map[string]any{"type": "boolean"},
			"message":           map[string]any{"type": "string"},
			"sessionUrl":        map[string]any{"type": "string"},
			"latestToolUse":     map[string]any{"type": "string"},
			"content":           map[string]any{"type": "array"},
			"outcome":           map[string]any{"type": "string", "enum": []string{"partial"}},
			"reason":            map[string]any{"type": "string"},
			"totalToolUseCount": map[string]any{"type": "integer"},
			"usage":             map[string]any{"type": "object"},
			"artifactRefs":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"verificationRefs":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		}, "status"),
		agentOutputVariantSchema("timed_out", incompleteProperties("timed_out"), "status", "outcome"),
		agentOutputVariantSchema("cancelled", incompleteProperties("cancelled"), "status", "outcome"),
		agentOutputVariantSchema("interrupted", incompleteProperties("interrupted"), "status", "outcome"),
	}}
}

func (t *AgentTool) ToolContract() types.ToolContract {
	outputSchema := agentOutputSchema()
	return types.ToolContract{
		OutputSchema:       &outputSchema,
		Strict:             true,
		ReadOnly:           true,
		ConcurrencySafe:    true,
		MaxResultSizeChars: 100_000,
	}
}

func (t *AgentTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{ReadOnly: true, ConcurrencySafe: true, MaxResultSizeChars: 100_000}
}

func (t *AgentTool) IsReadOnly() bool       { return true }
func (t *AgentTool) IsConcurrentSafe() bool { return true }

// CheckPermissions mirrors the TS Agent contract: spawning is locally allowed
// because the child tools retain their own permission lifecycle. Auto mode
// remains a passthrough so its classifier can make the final decision.
func (t *AgentTool) CheckPermissions(_ context.Context, input map[string]any, request types.ToolPermissionRequest) (types.ToolPermissionResult, error) {
	mode := strings.ToLower(strings.TrimSpace(request.Runtime.PermissionMode))
	if mode == "auto" || strings.HasPrefix(mode, "auto:") {
		return types.ToolPermissionResult{
			Behavior:     types.PermissionBehaviorPassthrough,
			Message:      toolPermissionText(i18n.KeyToolPermissionAgentSpawn),
			UpdatedInput: input,
		}, nil
	}
	return types.ToolPermissionResult{Behavior: types.PermissionBehaviorAllow, UpdatedInput: input}, nil
}

// ToAutoClassifierInput connects Agent's typed helper to the shared registry
// classifier surface. Keeping this adapter separate avoids every caller having
// to decode AgentInput itself.
func (t *AgentTool) ToAutoClassifierInput(input map[string]any) string {
	in, err := parseInput[AgentInput](input)
	if err != nil {
		return ""
	}
	return t.AutoClassifierInput(in)
}

func agentResultContentBlocks(content []agentToolContentBlock) []types.ContentBlock {
	blocks := make([]types.ContentBlock, 0, len(content))
	for _, block := range content {
		if block.Type == "text" {
			blocks = append(blocks, types.TextBlock{Type: types.ContentTypeText, Text: block.Text})
		}
	}
	if len(blocks) == 0 {
		blocks = append(blocks, types.TextBlock{Type: types.ContentTypeText, Text: toolRuntimeText(i18n.KeyToolLegacyAAgentEmptyOutput)})
	}
	return blocks
}

func (t *AgentTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	block := types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: toolUseID, Data: data}
	switch result := data.(type) {
	case AgentCompleted:
		content := agentResultContentBlocks(result.Content)
		if isOneShotBuiltInAgentType(result.AgentType) && result.WorktreePath == "" && result.WorktreeBranch == "" {
			block.ContentBlocks = content
			return block
		}
		worktree := ""
		if result.WorktreePath != "" {
			worktree = fmt.Sprintf("\nworktreePath: %s\nworktreeBranch: %s", result.WorktreePath, result.WorktreeBranch)
		}
		content = append(content, types.TextBlock{Type: types.ContentTypeText, Text: toolRuntimeFormat(
			i18n.KeyToolLegacyAAgentCompletedMetadata,
			result.AgentID, result.AgentID, worktree, result.TotalTokens, result.ToolUseCount, result.DurationMs,
		)})
		block.ContentBlocks = content
		return block
	case *AgentCompleted:
		if result == nil {
			break
		}
		return t.MapToolResultToToolResultBlock(*result, toolUseID)
	case AgentPartial:
		switch result.WireStatus {
		case "remote_launched":
			block.Content = toolRuntimeFormat(i18n.KeyToolLegacyAAgentRemotePartial, result.TaskID, result.SessionURL, result.OutputFile)
		default:
			block.Content = toolRuntimeFormat(i18n.KeyToolLegacyAAgentAsyncPartial, result.AgentID, result.AgentID)
			if result.CanReadOutputFile {
				block.Content += toolRuntimeFormat(i18n.KeyToolLegacyAAgentAsyncOutputHint, result.OutputFile)
			} else {
				block.Content += toolRuntimeText(i18n.KeyToolLegacyAAgentAsyncCompletionHint)
			}
		}
		return block
	case *AgentPartial:
		if result == nil {
			break
		}
		return t.MapToolResultToToolResultBlock(*result, toolUseID)
	case AgentIncomplete:
		content := make([]types.ContentBlock, 0, len(result.Content)+1)
		for _, contentBlock := range result.Content {
			if contentBlock.Type == "text" && strings.TrimSpace(contentBlock.Text) != "" {
				content = append(content, types.TextBlock{Type: types.ContentTypeText, Text: contentBlock.Text})
			}
		}
		content = append(content, types.TextBlock{Type: types.ContentTypeText, Text: fmt.Sprintf(
			"agentId: %s\noutcome: %s\nreason: %s\n<usage>total_tokens: %d\ntool_uses: %d\nduration_ms: %d</usage>",
			result.AgentID, result.Outcome, result.Reason, result.TotalTokens, result.ToolUseCount, result.DurationMs,
		)})
		block.ContentBlocks = content
		block.IsError = true
		block.Outcome = toolOutcomeForAgentRun(result.Outcome)
		return block
	case *AgentIncomplete:
		if result == nil {
			break
		}
		return t.MapToolResultToToolResultBlock(*result, toolUseID)
	case AgentError:
		block.Content = result.Message
		block.IsError = true
		return block
	case *AgentError:
		if result == nil {
			break
		}
		return t.MapToolResultToToolResultBlock(*result, toolUseID)
	case AgentAborted:
		block.Content = result.Reason
		block.IsError = true
		return block
	case *AgentAborted:
		if result == nil {
			break
		}
		return t.MapToolResultToToolResultBlock(*result, toolUseID)
	}
	raw, err := json.Marshal(data)
	if err != nil {
		block.Content = toolRuntimeText(i18n.KeyToolLegacyAAgentInvalidTyped)
	} else {
		block.Content = string(raw)
	}
	block.IsError = true
	return block
}

func agentIncompleteToolResult(summary agentRunSummary, agentID, agentType, transcriptPath string, started time.Time, err error, identities ...agentUsageIdentity) types.ToolResult {
	if summary.AgentID == "" {
		summary.AgentID = agentID
	}
	if summary.AgentType == "" {
		summary.AgentType = agentType
	}
	if summary.TranscriptPath == "" {
		summary.TranscriptPath = transcriptPath
	}
	if summary.TotalDuration <= 0 {
		summary.TotalDuration = time.Since(started).Milliseconds()
	}
	if summary.Outcome == "" || summary.Outcome == AgentRunOutcomeSucceeded {
		summary.Outcome, summary.TerminalReason = classifyAgentRunTermination(err, strings.TrimSpace(summary.Output) != "")
	}
	switch summary.Outcome {
	case AgentRunOutcomePartial, AgentRunOutcomeTimedOut, AgentRunOutcomeCancelled, AgentRunOutcomeInterrupted:
		result := AgentResultFromIncomplete(summary, transcriptPath)
		data, marshalErr := MarshalAgentResult(result)
		content := string(data)
		if marshalErr != nil {
			content = firstNonEmpty(summary.Output, result.Reason)
		}
		toolResult := agentToolResultWithUsage(result, content, true, summary.Usage, identities...)
		toolResult.Outcome = toolOutcomeForAgentRun(result.Outcome)
		return toolResult
	default:
		return agentFailureToolResultWithUsage(context.Background(), agentID, agentType, transcriptPath, started, err, summary.Usage, identities...)
	}
}

func toolOutcomeForAgentRun(outcome AgentRunOutcome) types.ToolOutcome {
	switch outcome {
	case AgentRunOutcomePartial:
		return types.ToolOutcomePartial
	case AgentRunOutcomeTimedOut:
		return types.ToolOutcomeTimedOut
	case AgentRunOutcomeCancelled:
		return types.ToolOutcomeCancelled
	case AgentRunOutcomeInterrupted:
		// ToolOutcome has no interrupted variant; the typed AgentIncomplete
		// payload remains authoritative while the generic projection is failed.
		return types.ToolOutcomeFailed
	default:
		return types.ToolOutcomeFailed
	}
}

func agentToolResult(result AgentResult, content string, isError bool) types.ToolResult {
	return agentToolResultWithUsage(result, content, isError, nil)
}

type agentUsageIdentity struct {
	Provider string
	Model    string
}

func agentToolResultWithUsage(result AgentResult, content string, isError bool, usage *types.Usage, identities ...agentUsageIdentity) types.ToolResult {
	var usageCopy *types.Usage
	if usage != nil {
		cloned := *usage
		usageCopy = &cloned
	}
	toolResult := types.ToolResult{Content: content, Data: result, IsError: isError, Usage: usageCopy}
	if usageCopy != nil && len(identities) > 0 {
		identity := identities[0]
		if identity.Provider != "" || identity.Model != "" {
			toolResult.Metadata = map[string]string{
				"usage.provider": identity.Provider,
				"usage.model":    identity.Model,
			}
		}
	}
	return toolResult
}

func agentFailureToolResult(ctx context.Context, agentID, agentType, transcriptPath string, started time.Time, err error) types.ToolResult {
	return agentFailureToolResultWithUsage(ctx, agentID, agentType, transcriptPath, started, err, nil)
}

func agentFailureToolResultWithUsage(ctx context.Context, agentID, agentType, transcriptPath string, started time.Time, err error, usage *types.Usage, identities ...agentUsageIdentity) types.ToolResult {
	if err == nil {
		err = i18n.NewError(i18n.KeyToolBackgroundAgentFailed)
	}
	durationMs := time.Since(started).Milliseconds()
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || (ctx != nil && ctx.Err() != nil) {
		reason := agentAbortDisplayError(ctx, err).Error()
		result := AgentResultFromAborted(agentID, agentType, reason, "", durationMs, 0)
		result.TranscriptPath = transcriptPath
		return agentToolResultWithUsage(result, reason, true, usage, identities...)
	}
	result := AgentResultFromError(agentID, agentType, durationMs, err)
	result.TranscriptPath = transcriptPath
	return agentToolResultWithUsage(result, err.Error(), true, usage, identities...)
}

// agentAbortDisplayError localizes the first-party cancellation or timeout
// framing while keeping the runtime cause available to errors.Is/errors.As.
func agentAbortDisplayError(ctx context.Context, err error) error {
	cause := err
	if ctx != nil && ctx.Err() != nil {
		cause = ctx.Err()
	}
	if cause == nil {
		return i18n.NewError(i18n.KeyToolBackgroundAgentFailed)
	}
	if errors.Is(cause, context.DeadlineExceeded) {
		return i18n.WrapError(i18n.KeyToolBackgroundAgentTimedOutWithCause, cause)
	}
	return i18n.WrapError(i18n.KeyToolBackgroundAgentCanceledWithCause, cause)
}
