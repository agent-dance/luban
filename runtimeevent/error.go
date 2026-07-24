package runtimeevent

import (
	"errors"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// NewErrorEvent adapts private provider/runtime failure material into the
// shared RuntimeEvent contract. Callers must project the result for an explicit
// audience before presenting or serializing it.
func NewErrorEvent(identity types.RuntimeIdentity, message string, apiError *types.APIError, metadata map[string]any) types.RuntimeEvent {
	privateMetadata := make(map[string]any, len(metadata)+2)
	for key, value := range metadata {
		privateMetadata[key] = value
	}
	if message != "" {
		privateMetadata["runtime_message"] = message
	}
	if apiError != nil {
		privateMetadata["api_error"] = apiError
	}
	var cause error
	if apiError != nil && message != "" && message != apiError.Message {
		cause = errors.Join(apiError, errors.New(message))
	} else if apiError != nil {
		cause = apiError
	} else if message != "" {
		cause = errors.New(message)
	}
	event := types.NewRuntimeEvent(
		types.RuntimeEventKindError, identity, types.ToolOutcomeFailed,
		i18n.KeyRuntimeErrorPublicSummary, nil, "runtime.operation_failed", cause,
	)
	event.PrivateMetadata = privateMetadata
	event.EvidenceRef = &types.RuntimeEvidenceRef{ID: event.EventID}
	return event
}
