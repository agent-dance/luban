package loop

import (
	"github.com/agent-dance/luban/i18n"
	streamevent "github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

// ToolUseCatalogError is a fail-closed provider protocol violation. A tool may
// exist in the larger private execution registry without belonging to the
// immutable catalog supplied to the model for this turn.
type ToolUseCatalogError struct {
	ToolUseID string
	ToolName  string
	Index     int
}

func (e *ToolUseCatalogError) Error() string {
	name := ""
	if e != nil {
		name = e.ToolName
	}
	return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyLoopQueryToolOutsideVisibleCatalog, name)
}

func validateToolUsesAgainstVisibleCatalog(toolUses []types.ToolUseBlock, snapshot registry.VisibleToolSnapshot) *ToolUseCatalogError {
	// A missing snapshot belongs to the legacy embedding API. Production coding
	// profiles always carry a valid immutable snapshot and are guarded here.
	if !snapshot.Valid() {
		return nil
	}
	for index, toolUse := range toolUses {
		if !snapshot.Allows(toolUse.Name) {
			return &ToolUseCatalogError{
				ToolUseID: toolUse.ID,
				ToolName:  toolUse.Name,
				Index:     index,
			}
		}
	}
	return nil
}

func toolUseCatalogErrorEvent(err *ToolUseCatalogError, turnCount int) streamevent.Event {
	message := err.Error()
	return streamevent.Event{
		Type: streamevent.EventError,
		Text: message,
		Error: &types.APIError{
			Type:    "tool_outside_visible_catalog",
			Message: message,
		},
		TurnCount:      turnCount,
		ToolUseID:      err.ToolUseID,
		TerminalReason: "tool_outside_visible_catalog",
		Metadata: map[string]any{
			"reason":    "tool_outside_visible_catalog",
			"index":     err.Index,
			"tool_name": err.ToolName,
			"outcome":   string(types.ToolOutcomeFailed),
		},
	}
}
