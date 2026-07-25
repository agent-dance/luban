package runtimeevent

import (
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/types"
)

// SystemWarningRuntimeEvent returns the authoritative warning projection from
// a stream event. Malformed carriers fail closed to generic semantic copy.
func SystemWarningRuntimeEvent(event stream.Event) types.RuntimeEvent {
	if event.RuntimeEvent != nil && event.RuntimeEvent.Kind == types.RuntimeEventKindWarning {
		warning := *event.RuntimeEvent
		warning.RuntimeIdentity = mergeStreamIdentity(warning.RuntimeIdentity, event)
		return warning
	}
	return NewWarningEvent(
		mergeStreamIdentity(types.RuntimeIdentity{}, event),
		i18n.KeyRuntimeWarningPublicSummary,
		nil,
		nil,
		nil,
	)
}

func mergeStreamIdentity(identity types.RuntimeIdentity, event stream.Event) types.RuntimeIdentity {
	if identity.TurnID == "" {
		identity.TurnID = event.TurnID
	}
	if identity.ToolUseID == "" {
		identity.ToolUseID = event.ToolUseID
	}
	if identity.WorkUnitID == "" {
		identity.WorkUnitID = event.WorkUnitID
	}
	if identity.ActorID == "" {
		identity.ActorID = event.ActorID
	}
	if identity.ActorType == "" {
		identity.ActorType = event.ActorType
	}
	return identity
}
