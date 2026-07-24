// Package runtimeevent projects private runtime events into explicit audience
// schemas. It is the only supported path from types.RuntimeEvent to JSON or
// user/model-visible copy.
package runtimeevent

import (
	"errors"
	"fmt"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// Audience identifies the consumer authority of a projection.
type Audience string

const (
	AudienceUser  Audience = "user"
	AudienceModel Audience = "model"
	AudienceAudit Audience = "audit"
	AudienceSDK   Audience = "sdk"
)

// RedactionLevel is an explicit disclosure contract, not a display preference.
type RedactionLevel string

const (
	RedactionStrict     RedactionLevel = "strict"
	RedactionPublic     RedactionLevel = "public"
	RedactionDiagnostic RedactionLevel = "diagnostic"
	RedactionRaw        RedactionLevel = "raw"
)

var (
	ErrInvalidAudience       = errors.New("invalid runtime-event audience")
	ErrInvalidRedaction      = errors.New("invalid runtime-event redaction level")
	ErrRawAuditNotEnabled    = errors.New("raw runtime-event audit was not explicitly enabled")
	ErrRawNonAuditAudience   = errors.New("raw runtime-event projection requires the audit audience")
	ErrMissingEventID        = errors.New("runtime event is missing an event ID")
	ErrMissingPublicKey      = errors.New("runtime event is missing a public key")
	ErrMissingOutcome        = errors.New("runtime event is missing an authoritative outcome")
	ErrUnsupportedSchema     = errors.New("unsupported runtime-event schema")
	ErrInvalidEventKind      = errors.New("invalid runtime-event kind")
	ErrMissingDiagnosticCode = errors.New("runtime event is missing a diagnostic code")
)

// ProjectionOptions declares all authority used for projection. AllowRawAudit
// is intentionally separate from RedactionRaw so deserializing or forwarding a
// raw redaction value cannot silently enable private data disclosure.
type ProjectionOptions struct {
	Audience      Audience
	Redaction     RedactionLevel
	Language      i18n.Language
	LanguageSet   bool
	AllowRawAudit bool
}

// ProjectedRemediation is localized only at the final projection boundary.
type ProjectedRemediation struct {
	Action  string `json:"action"`
	Message string `json:"message"`
}

// ProjectedRuntimeEvent is the versioned wire schema. PrivateCause and
// PrivateMetadata are populated only by an explicitly enabled raw audit.
type ProjectedRuntimeEvent struct {
	Type           string                 `json:"type"`
	SchemaVersion  string                 `json:"schema_version"`
	Audience       Audience               `json:"audience"`
	RedactionLevel RedactionLevel         `json:"redaction_level"`
	Kind           types.RuntimeEventKind `json:"kind,omitempty"`

	EventID           string `json:"event_id,omitempty"`
	SessionID         string `json:"session_id,omitempty"`
	Epoch             uint64 `json:"epoch,omitempty"`
	ContextGeneration uint64 `json:"context_generation,omitempty"`
	TurnID            string `json:"turn_id,omitempty"`
	ToolUseID         string `json:"tool_use_id,omitempty"`
	WorkUnitID        string `json:"work_unit_id,omitempty"`
	ActorID           string `json:"actor_id,omitempty"`
	ActorType         string `json:"actor_type,omitempty"`

	Outcome     types.ToolOutcome         `json:"outcome,omitempty"`
	Code        string                    `json:"code"`
	Message     string                    `json:"message"`
	PublicKey   i18n.Key                  `json:"public_key,omitempty"`
	PublicArgs  []any                     `json:"public_args,omitempty"`
	Remediation *ProjectedRemediation     `json:"remediation,omitempty"`
	EvidenceRef *types.RuntimeEvidenceRef `json:"evidence_ref,omitempty"`

	PrivateCause    string         `json:"private_cause,omitempty"`
	PrivateMetadata map[string]any `json:"private_metadata,omitempty"`
}

// AudienceProjector is stateless and safe for concurrent use.
type AudienceProjector struct{}

func NewAudienceProjector() AudienceProjector { return AudienceProjector{} }

// Project validates event authority and emits only fields permitted by the
// requested audience/redaction pair.
func (AudienceProjector) Project(event types.RuntimeEvent, options ProjectionOptions) (ProjectedRuntimeEvent, error) {
	options = defaultProjectionOptions(options)
	if err := validateProjectionOptions(options); err != nil {
		return ProjectedRuntimeEvent{}, i18n.WrapInternalError(i18n.KeyRuntimeEventProjectionRejected, err)
	}
	if err := validateRuntimeEvent(event); err != nil {
		return ProjectedRuntimeEvent{}, i18n.WrapInternalError(i18n.KeyRuntimeEventInvalid, err)
	}

	lang := options.Language
	if !options.LanguageSet {
		lang = i18n.DetectOrLoadLanguage()
	}
	projected := ProjectedRuntimeEvent{
		Type: eventType(event.Kind), SchemaVersion: types.RuntimeEventSchemaVersion,
		Audience: options.Audience, RedactionLevel: options.Redaction,
		Kind: event.Kind, Outcome: event.Outcome,
		Code: event.DiagnosticCode, Message: i18n.Format(lang, event.PublicKey, event.PublicArgs...),
	}

	if options.Redaction != RedactionStrict {
		if event.Remediation != nil {
			projected.Remediation = &ProjectedRemediation{
				Action:  event.Remediation.Action,
				Message: i18n.Format(lang, event.Remediation.PublicKey, event.Remediation.PublicArgs...),
			}
		}
	}

	if exposesStableIdentity(options) {
		projected.EventID = event.EventID
		projected.SessionID = event.SessionID
		projected.Epoch = event.Epoch
		projected.ContextGeneration = event.ContextGeneration
		projected.TurnID = event.TurnID
		projected.ToolUseID = event.ToolUseID
		projected.WorkUnitID = event.WorkUnitID
		projected.ActorID = event.ActorID
		projected.ActorType = event.ActorType
	}

	if options.Audience == AudienceSDK || options.Audience == AudienceAudit {
		projected.PublicKey = event.PublicKey
		projected.PublicArgs = append([]any(nil), event.PublicArgs...)
	}
	if options.Redaction == RedactionDiagnostic || options.Redaction == RedactionRaw {
		projected.EvidenceRef = cloneEvidenceRef(event.EvidenceRef)
	}
	if options.Redaction == RedactionRaw {
		projected.PrivateCause = rawCauseText(event.PrivateCause)
		projected.PrivateMetadata = cloneMetadata(event.PrivateMetadata)
	}
	return projected, nil
}

func defaultProjectionOptions(options ProjectionOptions) ProjectionOptions {
	if options.Audience == "" {
		options.Audience = AudienceUser
	}
	if options.Redaction == "" {
		switch options.Audience {
		case AudienceSDK:
			options.Redaction = RedactionPublic
		case AudienceAudit:
			options.Redaction = RedactionDiagnostic
		default:
			options.Redaction = RedactionStrict
		}
	}
	return options
}

func validateProjectionOptions(options ProjectionOptions) error {
	switch options.Audience {
	case AudienceUser, AudienceModel, AudienceAudit, AudienceSDK:
	default:
		return fmt.Errorf("%w: %q", ErrInvalidAudience, options.Audience)
	}
	switch options.Redaction {
	case RedactionStrict, RedactionPublic, RedactionDiagnostic, RedactionRaw:
	default:
		return fmt.Errorf("%w: %q", ErrInvalidRedaction, options.Redaction)
	}
	if options.Redaction == RedactionRaw {
		if options.Audience != AudienceAudit {
			return ErrRawNonAuditAudience
		}
		if !options.AllowRawAudit {
			return ErrRawAuditNotEnabled
		}
	}
	if (options.Audience == AudienceUser || options.Audience == AudienceModel) &&
		(options.Redaction == RedactionDiagnostic || options.Redaction == RedactionRaw) {
		return fmt.Errorf("%w: %s/%s", ErrInvalidRedaction, options.Audience, options.Redaction)
	}
	return nil
}

func validateRuntimeEvent(event types.RuntimeEvent) error {
	if event.SchemaVersion != types.RuntimeEventSchemaVersion {
		return fmt.Errorf("%w: %q", ErrUnsupportedSchema, event.SchemaVersion)
	}
	if event.EventID == "" {
		return ErrMissingEventID
	}
	switch event.Kind {
	case types.RuntimeEventKindError, types.RuntimeEventKindWarning, types.RuntimeEventKindToolResult:
	default:
		return fmt.Errorf("%w: %q", ErrInvalidEventKind, event.Kind)
	}
	if event.PublicKey == "" {
		return ErrMissingPublicKey
	}
	if event.Kind != types.RuntimeEventKindWarning && !event.HasAuthoritativeOutcome() {
		return ErrMissingOutcome
	}
	if event.DiagnosticCode == "" {
		return ErrMissingDiagnosticCode
	}
	return nil
}

func exposesStableIdentity(options ProjectionOptions) bool {
	return options.Audience == AudienceSDK || options.Audience == AudienceAudit
}

func eventType(kind types.RuntimeEventKind) string {
	switch kind {
	case types.RuntimeEventKindError:
		return "error"
	case types.RuntimeEventKindWarning:
		return "warning"
	case types.RuntimeEventKindToolResult:
		return "tool_result"
	default:
		return "runtime_event"
	}
}

func cloneEvidenceRef(ref *types.RuntimeEvidenceRef) *types.RuntimeEvidenceRef {
	if ref == nil {
		return nil
	}
	copy := *ref
	return &copy
}

func cloneMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	copy := make(map[string]any, len(metadata))
	for key, value := range metadata {
		copy[key] = value
	}
	return copy
}

func rawCauseText(err error) string {
	if err == nil {
		return ""
	}
	if semantic, ok := i18n.DescribeSemanticError(err); ok && semantic.Cause != nil {
		return rawCauseText(semantic.Cause)
	}
	return err.Error()
}
