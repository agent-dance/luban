package provider

import (
	"errors"
	"fmt"
	"strings"

	"github.com/agent-dance/luban/types"
)

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

// IsRetryable reports whether the error is worth retrying.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	apiErr, ok := AsAPIError(err)
	if !ok {
		// Generic network/connection errors are retryable.
		msg := strings.ToLower(err.Error())
		return isConnectionError(msg)
	}
	return apiErrRetryable(apiErr)
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

func apiErrRetryable(e *types.APIError) bool {
	// connection_error from OpenAI/network layers is always retryable
	if e.Type == "connection_error" {
		return true
	}
	switch e.Status {
	case 408, 409, 429, 529:
		return true
	case 401:
		return false // no auth refresh mechanism; fail fast
	case 400:
		// Only retryable if it's a max_tokens context overflow
		return isMaxTokensOverflow(e)
	case 403:
		return false
	}
	// 5xx server errors
	if e.Status >= 500 && e.Status < 600 {
		return true
	}
	// Status == 0 means we don't know; treat as connection error
	if e.Status == 0 {
		return isConnectionError(strings.ToLower(e.Message))
	}
	return false
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
	msg := strings.ToLower(ae.Message)
	return strings.Contains(msg, "prompt is too long") ||
		ae.Type == "prompt_too_long"
}

// isMaxTokensOverflow reports whether a 400 error is due to max_tokens overflow.
func isMaxTokensOverflow(e *types.APIError) bool {
	if e.Status != 400 {
		return false
	}
	msg := strings.ToLower(e.Message)
	return strings.Contains(msg, "max_tokens") &&
		(strings.Contains(msg, "exceed") || strings.Contains(msg, "context") || strings.Contains(msg, "overflow"))
}

// isConnectionError reports whether a lowercase error message looks like a
// transient network / connection issue.
func isConnectionError(msg string) bool {
	keywords := []string{
		"connection refused",
		"connection reset",
		"connection closed",
		"no such host",
		"network is unreachable",
		"i/o timeout",
		"eof",
		"broken pipe",
		"tls handshake",
		"dial tcp",
	}
	for _, kw := range keywords {
		if strings.Contains(msg, kw) {
			return true
		}
	}
	return false
}
