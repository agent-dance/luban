package types

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/agent-dance/luban/i18n"
)

// RuntimeEventSchemaVersion is the durable schema shared by presentation,
// SDK, model, and audit projections. Projection schema changes must increment
// this value instead of silently changing the meaning of existing fields.
const RuntimeEventSchemaVersion = "runtime-event/v2"

// RuntimeEventKind identifies the authoritative runtime transition represented
// by an event. It is deliberately independent from localized presentation text.
type RuntimeEventKind string

const (
	RuntimeEventKindError      RuntimeEventKind = "runtime_error"
	RuntimeEventKindWarning    RuntimeEventKind = "runtime_warning"
	RuntimeEventKindToolResult RuntimeEventKind = "tool_result"
)

// RuntimeIdentity is immutable correlation data captured at the event source.
// Epoch fences presentation sessions while ContextGeneration fences model
// context after compaction or another context-replacing transition.
type RuntimeIdentity struct {
	EventID           string `json:"event_id"`
	SessionID         string `json:"session_id,omitempty"`
	Epoch             uint64 `json:"epoch,omitempty"`
	ContextGeneration uint64 `json:"context_generation,omitempty"`
	TurnID            string `json:"turn_id,omitempty"`
	ToolUseID         string `json:"tool_use_id,omitempty"`
	WorkUnitID        string `json:"work_unit_id,omitempty"`
	ActorID           string `json:"actor_id,omitempty"`
	ActorType         string `json:"actor_type,omitempty"`
}

// RuntimeEvidenceRef is an opaque immutable evidence reference. It deliberately
// carries no filesystem path; resolving it remains an audit/evidence capability.
type RuntimeEvidenceRef struct {
	ID     string `json:"id"`
	Digest string `json:"digest,omitempty"`
}

// RuntimeEvent separates stable identity and authoritative outcome from public
// semantics and private diagnostics. Private fields are never JSON encoded
// directly; callers must use the audience projector.
type RuntimeEvent struct {
	SchemaVersion string           `json:"schema_version"`
	Kind          RuntimeEventKind `json:"kind"`
	RuntimeIdentity
	Outcome ToolOutcome `json:"outcome,omitempty"`

	PublicKey  i18n.Key `json:"public_key"`
	PublicArgs []any    `json:"public_args,omitempty"`

	DiagnosticCode  string              `json:"diagnostic_code"`
	PrivateCause    error               `json:"-"`
	PrivateMetadata map[string]any      `json:"-"`
	EvidenceRef     *RuntimeEvidenceRef `json:"evidence_ref,omitempty"`
}

var runtimeEventFallbackSequence atomic.Uint64

// NewRuntimeEventID returns an opaque event ID without embedding session,
// actor, tool, path, or diagnostic information.
func NewRuntimeEventID() string {
	var random [16]byte
	if _, err := rand.Read(random[:]); err == nil {
		return "evt_" + hex.EncodeToString(random[:])
	}
	// crypto/rand failure is exceptionally rare. The fallback remains unique
	// within the process and deliberately contains no caller-controlled data.
	return fmt.Sprintf("evt_%x_%x", time.Now().UnixNano(), runtimeEventFallbackSequence.Add(1))
}

// NewRuntimeEvent constructs a safe event. An internal cause remains available
// through errors.Is/As while its text is hidden behind the event's semantic
// public key.
func NewRuntimeEvent(kind RuntimeEventKind, identity RuntimeIdentity, outcome ToolOutcome, publicKey i18n.Key, publicArgs []any, diagnosticCode string, cause error) RuntimeEvent {
	if identity.EventID == "" {
		identity.EventID = NewRuntimeEventID()
	}
	if publicKey == "" {
		publicKey = i18n.KeyRuntimeErrorPublicSummary
	}
	privateCause := cause
	if cause != nil {
		privateCause = i18n.WrapInternalError(publicKey, cause, publicArgs...)
	}
	return RuntimeEvent{
		SchemaVersion: RuntimeEventSchemaVersion,
		Kind:          kind, RuntimeIdentity: identity, Outcome: outcome,
		PublicKey: publicKey, PublicArgs: append([]any(nil), publicArgs...),
		DiagnosticCode: diagnosticCode, PrivateCause: privateCause,
	}
}

// NewToolResultRuntimeEvent adapts a tool result without inferring outcome from
// IsError, payload fields, status strings, or diagnostic codes. An unassigned
// Outcome remains invalid so audience projection rejects the incomplete event.
func NewToolResultRuntimeEvent(identity RuntimeIdentity, result ToolResultBlock, publicKey i18n.Key, publicArgs []any) RuntimeEvent {
	event := NewRuntimeEvent(RuntimeEventKindToolResult, identity, result.Outcome, publicKey, publicArgs, "tool.result", nil)
	event.ToolUseID = result.ToolUseID
	return event
}

// Error returns only semantic public copy. It never includes PrivateCause.
func (e RuntimeEvent) Error() string {
	key := e.PublicKey
	if key == "" {
		key = i18n.KeyRuntimeErrorPublicSummary
	}
	return i18n.Format(i18n.DetectOrLoadLanguage(), key, e.PublicArgs...)
}

// Unwrap preserves private diagnostic identity for errors.Is/errors.As without
// making its text part of the public error string.
func (e RuntimeEvent) Unwrap() error { return e.PrivateCause }

// HasAuthoritativeOutcome reports whether Outcome is one of the runtime result
// states. It intentionally does not inspect any generic payload field.
func (e RuntimeEvent) HasAuthoritativeOutcome() bool {
	switch e.Outcome {
	case ToolOutcomeSucceeded, ToolOutcomeFailed, ToolOutcomePartial, ToolOutcomeDenied,
		ToolOutcomeCancelled, ToolOutcomeTimedOut:
		return true
	default:
		return false
	}
}

// MarshalJSON fails closed so a caller cannot accidentally create a JSON or
// audit stream without declaring audience, redaction level, and raw-audit
// authority. Use runtimeevent.AudienceProjector at the final boundary.
func (e RuntimeEvent) MarshalJSON() ([]byte, error) {
	return nil, i18n.NewError(i18n.KeyRuntimeEventProjectionRejected)
}
