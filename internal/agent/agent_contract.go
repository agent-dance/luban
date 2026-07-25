package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agent-dance/luban/i18n"
	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
	"github.com/agent-dance/luban/types"
)

func (t *AgentTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{ReadOnly: true, ConcurrencySafe: true, MaxResultSizeChars: 100_000}
}

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

func agentResultContentBlocks(content []agentToolContentBlock) []types.ContentBlock {
	blocks := make([]types.ContentBlock, 0, len(content))
	for _, block := range content {
		if block.Type == "text" {
			blocks = append(blocks, types.TextBlock{Type: types.ContentTypeText, Text: block.Text})
		}
	}
	if len(blocks) == 0 {
		blocks = append(blocks, types.TextBlock{Type: types.ContentTypeText, Text: toolRuntimeText(i18n.KeyToolAgentEmptyOutput)})
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
			i18n.KeyToolAgentCompletedMetadata,
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
		block.Content = toolRuntimeFormat(i18n.KeyToolAgentAsyncPartial, result.AgentID, result.AgentID)
		if result.CanReadOutputFile {
			block.Content += toolRuntimeFormat(i18n.KeyToolAgentAsyncOutputHint, result.OutputFile)
		} else {
			block.Content += toolRuntimeText(i18n.KeyToolAgentAsyncCompletionHint)
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
		block.Content = toolRuntimeText(i18n.KeyToolAgentInvalidTyped)
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
	if summary.Outcome == "" || summary.Outcome == agentcontract.RunOutcomeSucceeded {
		summary.Outcome, summary.TerminalReason = classifyAgentRunTermination(err, strings.TrimSpace(summary.Output) != "")
	}
	switch summary.Outcome {
	case agentcontract.RunOutcomePartial, agentcontract.RunOutcomeTimedOut, agentcontract.RunOutcomeCancelled, agentcontract.RunOutcomeInterrupted:
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

func toolOutcomeForAgentRun(outcome agentcontract.RunOutcome) types.ToolOutcome {
	switch outcome {
	case agentcontract.RunOutcomePartial:
		return types.ToolOutcomePartial
	case agentcontract.RunOutcomeTimedOut:
		return types.ToolOutcomeTimedOut
	case agentcontract.RunOutcomeCancelled:
		return types.ToolOutcomeCancelled
	case agentcontract.RunOutcomeInterrupted:
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
