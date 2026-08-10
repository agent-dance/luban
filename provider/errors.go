package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"

	"github.com/agent-dance/luban/types"
)

// AttemptErrorContract is the complete machine-actionable retry disposition
// for one failed raw provider attempt. It is intentionally independent from
// provider-controlled diagnostic prose.
type AttemptErrorContract struct {
	Stage        types.ProviderErrorStage
	Class        types.ProviderErrorClass
	ReplaySafety types.ProviderReplaySafety
}

// Retryable reports whether policy permits another raw call. Attempt budget,
// delay, and authentication refresh are still owned by AttemptController.
func (c AttemptErrorContract) Retryable() bool {
	if c.ReplaySafety != types.ProviderReplaySafe {
		return false
	}
	switch c.Class {
	case types.ProviderErrorClassThrottle, types.ProviderErrorClassOverload, types.ProviderErrorClassTransport:
		return true
	default:
		return false
	}
}

// attemptErrorMetadata lets transaction-boundary wrappers (notably the
// runtime's uncommitted PartialStreamError) provide stronger stage and replay
// evidence without introducing a provider -> runtime import cycle.
type attemptErrorMetadata interface {
	AttemptErrorStage() types.ProviderErrorStage
	AttemptErrorClass() types.ProviderErrorClass
	AttemptReplaySafety() types.ProviderReplaySafety
}

// FallbackTriggeredError signals that the current model should be abandoned and
// the same request retried with FallbackModel.
type FallbackTriggeredError struct {
	OriginalModel string
	FallbackModel string
	Cause         error
}

func (e *FallbackTriggeredError) Error() string {
	if e == nil {
		return "model fallback triggered"
	}
	msg := fmt.Sprintf("model fallback triggered: %s -> %s", e.OriginalModel, e.FallbackModel)
	if e.Cause != nil {
		msg += ": " + e.Cause.Error()
	}
	return msg
}

func (e *FallbackTriggeredError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// AsFallbackTriggeredError extracts fallback metadata from either the provider
// typed error or the stream/API error shape used by providers that can only
// surface errors inside StreamEvent.
func AsFallbackTriggeredError(err error) (*FallbackTriggeredError, bool) {
	if err == nil {
		return nil, false
	}
	if fallback, ok := err.(*FallbackTriggeredError); ok {
		return fallback, true
	}
	if apiErr, ok := AsAPIError(err); ok && apiErr.Type == "fallback_triggered" && apiErr.FallbackModel != "" {
		return &FallbackTriggeredError{
			OriginalModel: apiErr.OriginalModel,
			FallbackModel: apiErr.FallbackModel,
			Cause:         apiErr,
		}, true
	}
	return nil, false
}

// ClassifyAttemptError derives the typed retry contract using structured
// fields only. Message is diagnostic data and is never consulted. Explicit
// metadata supplied by a transaction wrapper has highest priority, followed
// by API code/type/status and concrete Go transport interfaces.
func ClassifyAttemptError(err error) AttemptErrorContract {
	contract := AttemptErrorContract{
		Stage:        types.ProviderErrorStageConnect,
		Class:        types.ProviderErrorClassUnknown,
		ReplaySafety: types.ProviderReplayUnsafe,
	}
	if err == nil {
		return contract
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		contract.Class = types.ProviderErrorClassPermanent
		return contract
	}

	var metadata attemptErrorMetadata
	if errors.As(err, &metadata) {
		contract.Stage = normalizedErrorStage(metadata.AttemptErrorStage(), contract.Stage)
		contract.Class = normalizedErrorClass(metadata.AttemptErrorClass(), contract.Class)
		contract.ReplaySafety = normalizedReplaySafety(metadata.AttemptReplaySafety(), contract.ReplaySafety)
		return contract
	}

	if apiErr, ok := AsAPIError(err); ok {
		contract.Stage = apiErrorStage(apiErr)
		contract.Class = apiErrorClass(apiErr)
		contract.ReplaySafety = apiErrorReplaySafety(apiErr, contract)
		return contract
	}

	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		contract.Class = types.ProviderErrorClassTransport
		contract.ReplaySafety = types.ProviderReplaySafe
		return contract
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		contract.Class = types.ProviderErrorClassTransport
		contract.ReplaySafety = types.ProviderReplaySafe
		return contract
	}
	return contract
}

// IsRetryable reports whether the typed contract permits a retry. The caller
// must still ask its generation-scoped AttemptController for budget and delay.
func IsRetryable(err error) bool {
	return ClassifyAttemptError(err).Retryable()
}

// AsAPIError extracts a *types.APIError from an error value, if present.
func AsAPIError(err error) (*types.APIError, bool) {
	if err == nil {
		return nil, false
	}
	var ae *types.APIError
	if errors.As(err, &ae) {
		return ae, true
	}
	return nil, false
}

// typedTransportAPIError converts only concrete Go transport failures. It
// never promotes diagnostic text into retry authority.
func typedTransportAPIError(err error, stage types.ProviderErrorStage) (*types.APIError, bool) {
	if err == nil {
		return nil, false
	}
	if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		var netErr net.Error
		if !errors.As(err, &netErr) {
			return nil, false
		}
	}
	return &types.APIError{
		Type:         "transport_error",
		Message:      err.Error(),
		Stage:        normalizedErrorStage(stage, types.ProviderErrorStageConnect),
		Class:        types.ProviderErrorClassTransport,
		ReplaySafety: types.ProviderReplaySafe,
	}, true
}

func apiErrRetryable(e *types.APIError) bool {
	return ClassifyAttemptError(e).Retryable()
}

func normalizedErrorStage(value, fallback types.ProviderErrorStage) types.ProviderErrorStage {
	switch value {
	case types.ProviderErrorStageConnect, types.ProviderErrorStageHeaders,
		types.ProviderErrorStageStream, types.ProviderErrorStageCommitted:
		return value
	default:
		return fallback
	}
}

func normalizedErrorClass(value, fallback types.ProviderErrorClass) types.ProviderErrorClass {
	switch value {
	case types.ProviderErrorClassThrottle, types.ProviderErrorClassOverload,
		types.ProviderErrorClassTransport, types.ProviderErrorClassContext,
		types.ProviderErrorClassAuth, types.ProviderErrorClassQuota,
		types.ProviderErrorClassPermanent, types.ProviderErrorClassUnknown:
		return value
	default:
		return fallback
	}
}

func normalizedReplaySafety(value, fallback types.ProviderReplaySafety) types.ProviderReplaySafety {
	switch value {
	case types.ProviderReplaySafe, types.ProviderReplayAmbiguous, types.ProviderReplayUnsafe:
		return value
	default:
		return fallback
	}
}

func apiErrorStage(err *types.APIError) types.ProviderErrorStage {
	if err == nil {
		return types.ProviderErrorStageConnect
	}
	if stage := normalizedErrorStage(err.Stage, ""); stage != "" {
		return stage
	}
	identifier := normalizedAPIIdentifier(err.Code, err.Type)
	switch identifier {
	case "stream_interrupted", "stream_idle_timeout", "response_failed":
		return types.ProviderErrorStageStream
	}
	if err.Status != 0 {
		return types.ProviderErrorStageHeaders
	}
	return types.ProviderErrorStageConnect
}

func apiErrorClass(err *types.APIError) types.ProviderErrorClass {
	if err == nil {
		return types.ProviderErrorClassUnknown
	}
	if class := normalizedErrorClass(err.Class, ""); class != "" {
		return class
	}
	// Permanent structured codes/types remain authoritative even when a proxy
	// supplies a contradictory 5xx. Conversely, a hard permanent 4xx must not
	// be replayed merely because its diagnostic type says server_error.
	codeClass, codeSpecific := classForAPIIdentifier(err.Code)
	typeClass, typeSpecific := classForAPIIdentifier(err.Type)
	if codeSpecific && !isTransientErrorClass(codeClass) {
		return codeClass
	}
	if typeSpecific && !isTransientErrorClass(typeClass) {
		return typeClass
	}
	switch err.Status {
	case 401:
		return types.ProviderErrorClassAuth
	case 402:
		return types.ProviderErrorClassQuota
	case 408:
		return types.ProviderErrorClassTransport
	case 429:
		return types.ProviderErrorClassThrottle
	case 503, 529:
		return types.ProviderErrorClassOverload
	case 400, 403, 404, 405, 409, 422:
		return types.ProviderErrorClassPermanent
	}
	if codeSpecific {
		return codeClass
	}
	if typeSpecific {
		return typeClass
	}
	if err.Status >= 500 && err.Status < 600 {
		return types.ProviderErrorClassOverload
	}
	if strings.EqualFold(strings.TrimSpace(err.Type), "api_error") ||
		strings.EqualFold(strings.TrimSpace(err.Code), "api_error") {
		return types.ProviderErrorClassPermanent
	}
	return types.ProviderErrorClassUnknown
}

func isTransientErrorClass(class types.ProviderErrorClass) bool {
	return class == types.ProviderErrorClassThrottle ||
		class == types.ProviderErrorClassOverload ||
		class == types.ProviderErrorClassTransport
}

func classForAPIIdentifier(value string) (types.ProviderErrorClass, bool) {
	identifier := strings.ToLower(strings.TrimSpace(value))
	if identifier == "" || identifier == "api_error" {
		return types.ProviderErrorClassUnknown, false
	}
	for _, marker := range []string{"context_length", "context_window", "prompt_too_long", "max_tokens_exceeded"} {
		if strings.Contains(identifier, marker) {
			return types.ProviderErrorClassContext, true
		}
	}
	for _, marker := range []string{"authentication", "invalid_api_key", "unauthorized"} {
		if strings.Contains(identifier, marker) {
			return types.ProviderErrorClassAuth, true
		}
	}
	for _, marker := range []string{"insufficient_quota", "quota_exceeded", "billing", "payment_required"} {
		if strings.Contains(identifier, marker) {
			return types.ProviderErrorClassQuota, true
		}
	}
	for _, marker := range []string{"rate_limit", "too_many_requests", "throttl"} {
		if strings.Contains(identifier, marker) {
			return types.ProviderErrorClassThrottle, true
		}
	}
	for _, marker := range []string{"overloaded", "server_error", "internal_error", "service_unavailable"} {
		if strings.Contains(identifier, marker) {
			return types.ProviderErrorClassOverload, true
		}
	}
	for _, marker := range []string{"connection_error", "stream_interrupted", "stream_idle_timeout", "transport_error", "response_failed"} {
		if strings.Contains(identifier, marker) {
			return types.ProviderErrorClassTransport, true
		}
	}
	for _, marker := range []string{
		"invalid_request", "bad_request", "permission", "forbidden",
		"model_not_found", "invalid_model", "unsupported_model",
		"fallback_triggered", "previous_response_not_found",
		"invalid_custom_tool_call", "parse_error",
	} {
		if strings.Contains(identifier, marker) {
			return types.ProviderErrorClassPermanent, true
		}
	}
	return types.ProviderErrorClassUnknown, false
}

func normalizedAPIIdentifier(code, typeName string) string {
	if code = strings.ToLower(strings.TrimSpace(code)); code != "" {
		return code
	}
	return strings.ToLower(strings.TrimSpace(typeName))
}

func apiErrorReplaySafety(err *types.APIError, contract AttemptErrorContract) types.ProviderReplaySafety {
	if err == nil {
		return types.ProviderReplayUnsafe
	}
	if safety := normalizedReplaySafety(err.ReplaySafety, ""); safety != "" {
		return safety
	}
	if contract.Stage == types.ProviderErrorStageCommitted {
		return types.ProviderReplayUnsafe
	}
	switch contract.Class {
	case types.ProviderErrorClassThrottle, types.ProviderErrorClassOverload, types.ProviderErrorClassTransport:
		return types.ProviderReplaySafe
	default:
		return types.ProviderReplayUnsafe
	}
}

// is401Error reports whether err is an HTTP 401 Unauthorized error.
func is401Error(err error) bool {
	ae, ok := AsAPIError(err)
	return ok && ae.Status == 401
}

// Is529Error reports whether err is an HTTP 529 overloaded error.
func Is529Error(err error) bool {
	ae, ok := AsAPIError(err)
	if ok && ae.Status == 529 {
		return true
	}
	// Some providers surface overload as a typed error without a 529 code.
	if ok && ae.Type == "overloaded_error" {
		return true
	}
	return false
}

// IsPromptTooLong reports whether err signals that the prompt is too long
// to be processed (distinct from retryable context-overflow).
func IsPromptTooLong(err error) bool {
	ae, ok := AsAPIError(err)
	if !ok {
		return false
	}
	return ClassifyAttemptError(ae).Class == types.ProviderErrorClassContext
}

// isMaxTokensOverflow reports whether a 400 error is due to max_tokens overflow.
func isMaxTokensOverflow(e *types.APIError) bool {
	return e != nil && e.Status == 400 &&
		(strings.EqualFold(strings.TrimSpace(e.Code), "max_tokens_exceeded") ||
			strings.EqualFold(strings.TrimSpace(e.Type), "max_tokens_exceeded"))
}
