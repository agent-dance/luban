// Package runtimeevent projects private runtime events into explicit audience
// schemas. It is the only supported path from types.RuntimeEvent to JSON or
// user/model-visible copy.
package runtimeevent

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

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

// ProjectionOptions declares all authority used for projection. Audience and
// Redaction are required; the projector never supplies implicit defaults.
// AllowRawAudit is intentionally separate from RedactionRaw so
// deserializing or forwarding a raw redaction value cannot silently enable
// private data disclosure.
type ProjectionOptions struct {
	Audience      Audience
	Redaction     RedactionLevel
	Language      i18n.Language
	LanguageSet   bool
	AllowRawAudit bool
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

	Outcome         types.ToolOutcome          `json:"outcome,omitempty"`
	Code            string                     `json:"code"`
	Message         string                     `json:"message"`
	PublicKey       i18n.Key                   `json:"public_key,omitempty"`
	PublicArgs      []any                      `json:"public_args,omitempty"`
	EvidenceRef     *types.RuntimeEvidenceRef  `json:"evidence_ref,omitempty"`
	ProviderRequest *ProviderRequestDiagnostic `json:"provider_request,omitempty"`

	PrivateCause    string         `json:"private_cause,omitempty"`
	PrivateMetadata map[string]any `json:"private_metadata,omitempty"`
}

// ProviderRequestDiagnostic contains a deliberately narrow, display-safe view
// of one provider request failure. Provider-controlled prose and raw endpoint
// URLs remain private evidence.
type ProviderRequestDiagnostic struct {
	Provider            string   `json:"provider,omitempty"`
	APIFormat           string   `json:"api_format,omitempty"`
	Endpoint            string   `json:"endpoint,omitempty"`
	RequestID           string   `json:"request_id,omitempty"`
	AttemptedAPIFormats []string `json:"attempted_api_formats,omitempty"`
	Suggestion          string   `json:"suggestion,omitempty"`
}

// AudienceProjector is stateless and safe for concurrent use.
type AudienceProjector struct{}

func NewAudienceProjector() AudienceProjector { return AudienceProjector{} }

// Project validates event authority and emits only fields permitted by the
// requested audience/redaction pair.
func (AudienceProjector) Project(event types.RuntimeEvent, options ProjectionOptions) (ProjectedRuntimeEvent, error) {
	if err := validateProjectionOptions(options); err != nil {
		return ProjectedRuntimeEvent{}, i18n.WrapInternalError(i18n.KeyRuntimeEventProjectionRejected, err)
	}
	eventType, err := validateRuntimeEvent(event)
	if err != nil {
		return ProjectedRuntimeEvent{}, i18n.WrapInternalError(i18n.KeyRuntimeEventInvalid, err)
	}

	lang := options.Language
	if !options.LanguageSet {
		lang = i18n.DetectOrLoadLanguage()
	}
	projected := ProjectedRuntimeEvent{
		Type: eventType, SchemaVersion: types.RuntimeEventSchemaVersion,
		Audience: options.Audience, RedactionLevel: options.Redaction,
		Kind: event.Kind, Outcome: event.Outcome,
		Code: event.DiagnosticCode, Message: i18n.Format(lang, event.PublicKey, event.PublicArgs...),
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
		if apiError := runtimeEventAPIError(event); apiError != nil {
			projected.ProviderRequest = projectProviderRequestDiagnostic(apiError, lang)
		}
	}
	if options.Redaction == RedactionRaw {
		projected.PrivateCause = rawCauseText(event.PrivateCause)
		projected.PrivateMetadata = cloneMetadata(event.PrivateMetadata)
	}
	return projected, nil
}

func runtimeEventAPIError(event types.RuntimeEvent) *types.APIError {
	apiError, _ := event.PrivateMetadata["api_error"].(*types.APIError)
	return apiError
}

func projectProviderRequestDiagnostic(apiError *types.APIError, lang i18n.Language) *ProviderRequestDiagnostic {
	if apiError == nil {
		return nil
	}
	providerName := safeDiagnosticIdentifier(apiError.Provider)
	apiFormat := safeAPIFormat(apiError.APIFormat)
	endpoint := safeDiagnosticEndpoint(apiError.Endpoint)
	requestID := safeDiagnosticIdentifier(apiError.RequestID)
	attempted := make([]string, 0, len(apiError.AttemptedAPIFormats))
	for _, candidate := range apiError.AttemptedAPIFormats {
		if format := safeAPIFormat(candidate); format != "" {
			attempted = append(attempted, format)
		}
	}
	suggested := safeAPIFormat(apiError.SuggestedAPIFormat)
	diagnostic := &ProviderRequestDiagnostic{
		Provider: providerName, APIFormat: apiFormat, Endpoint: endpoint,
		RequestID: requestID, AttemptedAPIFormats: attempted,
	}
	switch {
	case len(attempted) >= 2:
		diagnostic.Suggestion = i18n.Format(lang, i18n.KeyRuntimeErrorProviderAPIsExhausted, attempted[0], attempted[1])
	case apiFormat != "" && suggested != "":
		diagnostic.Suggestion = i18n.Format(lang, i18n.KeyRuntimeErrorProviderAPISuggestion, apiFormat, suggested, apiFormat)
	}
	if diagnostic.Provider == "" && diagnostic.APIFormat == "" && diagnostic.Endpoint == "" &&
		diagnostic.RequestID == "" && len(diagnostic.AttemptedAPIFormats) == 0 && diagnostic.Suggestion == "" {
		return nil
	}
	return diagnostic
}

func safeAPIFormat(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "responses":
		return "responses"
	case "chat-completions":
		return "chat-completions"
	default:
		return ""
	}
}

func safeDiagnosticIdentifier(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return ""
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || strings.ContainsRune("._:-", char) {
			continue
		}
		return ""
	}
	return value
}

func safeDiagnosticEndpoint(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || !strings.HasPrefix(parsed.Path, "/…") {
		return ""
	}
	return strings.TrimSpace(value)
}

func validateProjectionOptions(options ProjectionOptions) error {
	switch options.Audience {
	case AudienceUser, AudienceModel, AudienceAudit, AudienceSDK:
	default:
		return fmt.Errorf("%w: %q", ErrInvalidAudience, options.Audience)
	}
	switch options.Redaction {
	case RedactionStrict, RedactionDiagnostic, RedactionRaw:
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

func validateRuntimeEvent(event types.RuntimeEvent) (string, error) {
	if event.SchemaVersion != types.RuntimeEventSchemaVersion {
		return "", fmt.Errorf("%w: %q", ErrUnsupportedSchema, event.SchemaVersion)
	}
	if event.EventID == "" {
		return "", ErrMissingEventID
	}
	var eventType string
	switch event.Kind {
	case types.RuntimeEventKindError:
		eventType = "error"
	case types.RuntimeEventKindWarning:
		eventType = "warning"
	case types.RuntimeEventKindToolResult:
		eventType = "tool_result"
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidEventKind, event.Kind)
	}
	if event.PublicKey == "" {
		return "", ErrMissingPublicKey
	}
	if event.Kind != types.RuntimeEventKindWarning && !event.HasAuthoritativeOutcome() {
		return "", ErrMissingOutcome
	}
	if event.DiagnosticCode == "" {
		return "", ErrMissingDiagnosticCode
	}
	return eventType, nil
}

func exposesStableIdentity(options ProjectionOptions) bool {
	return options.Audience == AudienceSDK || options.Audience == AudienceAudit
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
