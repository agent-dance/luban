package runtimeevent

import (
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// NewWarningEvent constructs a warning whose public presentation is described
// only by a semantic key and explicitly public arguments. PrivateCause and
// PrivateMetadata remain available to an explicitly authorized audit
// projection and through errors.Is/errors.As, but are never part of the public
// warning payload.
func NewWarningEvent(identity types.RuntimeIdentity, publicKey i18n.Key, publicArgs []any, cause error, metadata map[string]any) types.RuntimeEvent {
	if publicKey == "" {
		publicKey = i18n.KeyRuntimeWarningPublicSummary
	}
	event := types.NewRuntimeEvent(
		types.RuntimeEventKindWarning,
		identity,
		"",
		publicKey,
		publicArgs,
		string(publicKey),
		cause,
	)
	if len(metadata) > 0 {
		event.PrivateMetadata = make(map[string]any, len(metadata))
		for key, value := range metadata {
			event.PrivateMetadata[key] = value
		}
	}
	event.EvidenceRef = &types.RuntimeEvidenceRef{ID: event.EventID}
	return event
}
