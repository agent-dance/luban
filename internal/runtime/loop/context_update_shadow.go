package loop

import (
	"strings"

	contextcontract "github.com/agent-dance/luban/internal/contracts/contextupdate"
	streamevent "github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/internal/runtime/compact"
	"github.com/agent-dance/luban/types"
)

func (q *QueryLoop) emitContextUpdateShadow(messages []types.Message, uses []types.ToolUseBlock, results []types.ToolResultBlock, turnCount int, onEvent func(streamevent.Event)) {
	if onEvent == nil {
		return
	}
	for index, use := range uses {
		if use.Name != "ContextUpdate" || index >= len(results) {
			continue
		}
		provider, ok := results[index].Data.(contextcontract.Provider)
		if !ok || results[index].IsError {
			continue
		}
		decision := provider.ContextUpdateDecision()
		toolName, target, found := findContextUpdateTarget(messages, decision.TargetIndex, decision.TargetTool)
		runtimeCandidate := found && compact.ProgressiveToolResultSupportsAction(toolName, target, string(decision.Action))
		onEvent(streamevent.Event{
			Type: streamevent.EventProgress, TurnCount: turnCount,
			Progress: &streamevent.ProgressEvent{Stage: "context_update_shadow", Metadata: map[string]any{
				"schema": decision.Schema, "action": string(decision.Action), "reason_code": decision.ReasonCode, "target_tool": toolName,
				"target_found": found, "runtime_candidate": runtimeCandidate,
				"target_index": decision.TargetIndex, "confidence": decision.Confidence,
				"applied": false,
			}},
		})
	}
}

// findContextUpdateTarget resolves a stable selector without asking the model
// to reproduce provider-generated opaque call IDs. targetIndex is the
// zero-based position in the complete immediately preceding tool-result batch;
// targetTool must match as a cross-check and ContextUpdate itself is rejected.
func findContextUpdateTarget(messages []types.Message, targetIndex int, targetTool string) (string, types.ToolResultBlock, bool) {
	if targetIndex < 0 || strings.TrimSpace(targetTool) == "" {
		return "", types.ToolResultBlock{}, false
	}
	names := buildContextUpdateToolNames(messages)
	for messageIndex := len(messages) - 1; messageIndex >= 0; messageIndex-- {
		results := make([]types.ToolResultBlock, 0)
		for _, block := range messages[messageIndex].Content {
			result, ok := block.(types.ToolResultBlock)
			if !ok {
				continue
			}
			results = append(results, result)
		}
		if len(results) == 0 {
			continue
		}
		if targetIndex >= len(results) {
			return "", types.ToolResultBlock{}, false
		}
		target := results[targetIndex]
		toolName := names[target.ToolUseID]
		if toolName == "" || toolName == "ContextUpdate" || !strings.EqualFold(toolName, strings.TrimSpace(targetTool)) {
			return toolName, target, false
		}
		return toolName, target, true
	}
	return "", types.ToolResultBlock{}, false
}

func buildContextUpdateToolNames(messages []types.Message) map[string]string {
	names := make(map[string]string)
	for _, message := range messages {
		for _, use := range message.GetToolUses() {
			names[use.ID] = use.Name
		}
	}
	return names
}
