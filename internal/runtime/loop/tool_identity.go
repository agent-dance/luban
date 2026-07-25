package loop

import (
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/types"
)

type ToolUseIdentityErrorKind string

const (
	ToolUseIdentityMissing   ToolUseIdentityErrorKind = "missing_tool_use_id"
	ToolUseIdentityDuplicate ToolUseIdentityErrorKind = "duplicate_tool_use_id"
	ToolUseIdentityReused    ToolUseIdentityErrorKind = "reused_tool_use_id"
)

// ToolUseIdentityError rejects a malformed live tool batch before any tool,
// permission, or hook side effect can be attributed to an ambiguous identity.
type ToolUseIdentityError struct {
	Kind       ToolUseIdentityErrorKind
	ToolUseID  string
	ToolName   string
	Index      int
	FirstIndex int
}

func (e *ToolUseIdentityError) Error() string {
	if e == nil {
		return i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLoopToolIdentityInvalid)
	}
	if e.Kind == ToolUseIdentityDuplicate {
		return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyLoopToolIdentityDuplicate, e.Kind, e.ToolUseID, e.Index, e.FirstIndex)
	}
	if e.Kind == ToolUseIdentityReused {
		return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyLoopToolIdentityReused, e.Kind, e.ToolUseID, e.Index)
	}
	return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyLoopToolIdentityAtIndex, e.Kind, e.Index, e.ToolName)
}

func validateToolUseIdentities(toolUses []types.ToolUseBlock, sessionSeen ...map[string]struct{}) *ToolUseIdentityError {
	seen := make(map[string]int, len(toolUses))
	var historical map[string]struct{}
	if len(sessionSeen) > 0 {
		historical = sessionSeen[0]
	}
	for index, toolUse := range toolUses {
		if strings.TrimSpace(toolUse.ID) == "" {
			return &ToolUseIdentityError{
				Kind: ToolUseIdentityMissing, ToolName: toolUse.Name,
				Index: index, FirstIndex: -1,
			}
		}
		if firstIndex, exists := seen[toolUse.ID]; exists {
			return &ToolUseIdentityError{
				Kind: ToolUseIdentityDuplicate, ToolUseID: toolUse.ID, ToolName: toolUse.Name,
				Index: index, FirstIndex: firstIndex,
			}
		}
		if _, exists := historical[toolUse.ID]; exists {
			return &ToolUseIdentityError{
				Kind: ToolUseIdentityReused, ToolUseID: toolUse.ID, ToolName: toolUse.Name,
				Index: index, FirstIndex: -1,
			}
		}
		seen[toolUse.ID] = index
	}
	return nil
}

func collectToolUseIDs(messages []types.Message) map[string]struct{} {
	seen := make(map[string]struct{})
	for _, message := range messages {
		for _, toolUse := range message.GetToolUses() {
			if strings.TrimSpace(toolUse.ID) != "" {
				seen[toolUse.ID] = struct{}{}
			}
		}
		for _, block := range message.Content {
			if result, ok := block.(types.ToolResultBlock); ok && strings.TrimSpace(result.ToolUseID) != "" {
				seen[result.ToolUseID] = struct{}{}
			}
		}
	}
	return seen
}

func toolUseIdentityErrorEvent(err *ToolUseIdentityError, turnCount int) stream.Event {
	metadata := map[string]any{
		"reason":    string(err.Kind),
		"index":     err.Index,
		"tool_name": err.ToolName,
		"outcome":   string(types.ToolOutcomeFailed),
	}
	if err.FirstIndex >= 0 {
		metadata["first_index"] = err.FirstIndex
	}
	return stream.Event{
		Type: stream.EventError, Text: err.Error(), Error: &types.APIError{
			Type: "invalid_tool_use_identity", Message: err.Error(),
		},
		TurnCount: turnCount, ToolUseID: err.ToolUseID,
		TerminalReason: "invalid_tool_use_identity", Metadata: metadata,
	}
}
