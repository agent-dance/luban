package loop

import (
	"errors"

	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/internal/runtime/compact"
	"github.com/agent-dance/luban/types"
)

var errInternalControlScopeTransition = errors.New("invalid internal control scope transition")

// sealRuntimeControlMessage is deliberately package-private. Public message
// constructors describe wire shape only; trusted loop producers must make the
// separate, auditable decision to attach runtime authority.
func (q *QueryLoop) sealRuntimeControlMessage(message types.Message) types.Message {
	if q == nil {
		return message
	}
	q.ensureInternalControlScope()
	return message.WithInternalControlProvenance(messagecontrol.Runtime(), q.internalControlScope)
}

// SealRuntimeControlMessage is the capability-gated bridge for runtime
// producers outside package loop (for example, the engine's background
// follow-up queue). The message is always sealed to this exact loop scope.
func (q *QueryLoop) SealRuntimeControlMessage(capability messagecontrol.Capability, message types.Message) (types.Message, bool) {
	if q == nil || !capability.Valid() {
		return message, false
	}
	return q.sealRuntimeControlMessage(message), true
}

// installContentReplacementRecords is the private authority boundary between
// persisted tool-result storage and replacement-state reconstruction. Public
// compact helpers only construct descriptors; they cannot mint trusted state.
func (q *QueryLoop) installContentReplacementRecords(messages []types.Message, records []compact.ContentReplacementRecord) []types.Message {
	q.ensureInternalControlScope()
	return compact.AppendContentReplacementRecordsForScope(messages, records, messagecontrol.Runtime(), q.internalControlScope)
}

func (q *QueryLoop) ensureInternalControlScope() {
	if q != nil && !q.internalControlScope.Bound() {
		q.internalControlScope = messagecontrol.NewLoopScope(messagecontrol.Runtime())
	}
}

// SetInternalControlScope installs the authoritative persisted scope obtained
// from the session store. The capability parameter prevents SDK callers from
// choosing the expected scope used by provider projections.
func (q *QueryLoop) SetInternalControlScope(capability messagecontrol.Capability, scope messagecontrol.Scope) bool {
	if q == nil || !capability.Valid() || !scope.Bound() {
		return false
	}
	// A scope transition is only valid before controls exist, or when every
	// existing authenticated control already belongs to the target scope.
	// This prevents a target loop from adopting another loop's pre-commit
	// bearer by installing a durable namespace around copied messages.
	for _, message := range q.messages {
		if messageContainsForeignControl(message, scope) {
			return false
		}
	}
	q.internalControlScope = scope
	return true
}

// AcknowledgeCommittedControlScope advances the live history to the exact
// scope just published by session CAS. It changes only private authenticators
// and the expected scope: catalog epoch, response chaining, replacement state,
// tool identities, and model-visible JSON remain untouched.
func (q *QueryLoop) AcknowledgeCommittedControlScope(capability messagecontrol.Capability, next messagecontrol.Scope) error {
	if q == nil || !capability.Valid() || !next.Bound() {
		return errInternalControlScopeTransition
	}
	current := q.internalControlScope
	if current.Bound() && (!current.SameNamespace(next) ||
		(next.ContextGeneration() != current.ContextGeneration() && next.ContextGeneration() != current.ContextGeneration()+1)) {
		return errInternalControlScopeTransition
	}
	for _, message := range q.messages {
		if messageContainsForeignControl(message, current) {
			return errInternalControlScopeTransition
		}
	}

	acknowledged := append([]types.Message(nil), q.messages...)
	for index, message := range acknowledged {
		content := append([]types.ContentBlock(nil), message.Content...)
		changed := false
		for blockIndex, raw := range content {
			replacement, ok := raw.(types.ContentReplacementBlock)
			if !ok || !replacement.HasInternalReplacementProvenance() {
				continue
			}
			content[blockIndex] = replacement.WithInternalReplacementProvenance(capability, next)
			changed = true
		}
		if changed {
			message.Content = content
		}
		if message.HasInternalControlProvenance() {
			message = message.WithInternalControlProvenance(capability, next)
		}
		acknowledged[index] = message
	}
	q.messages = acknowledged
	q.internalControlScope = next
	return nil
}

func (q *QueryLoop) validateInternalControlScope() error {
	if q == nil {
		return errInternalControlScopeTransition
	}
	for _, message := range q.messages {
		if messageContainsForeignControl(message, q.internalControlScope) {
			return errInternalControlScopeTransition
		}
	}
	return nil
}
