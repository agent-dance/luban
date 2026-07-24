package loop

import (
	"errors"
	"strings"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// PartialStreamError indicates the stream was interrupted mid-response but some
// content blocks were already received. The partial message is returned alongside
// this error so the caller can decide whether to proceed with the partial content
// or retry from scratch.
type PartialStreamError struct {
	Cause         error
	PartialBlocks int
}

func (e *PartialStreamError) Error() string {
	return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyLoopPartialStreamInterrupted, e.PartialBlocks, e.Cause)
}

func (e *PartialStreamError) Unwrap() error { return e.Cause }

// MaxTurnsError indicates the agentic loop reached an explicit turn cap. The
// TypeScript runtime surfaces this as a max_turns_reached attachment rather
// than a hard stream failure, so callers can preserve any assistant output
// already produced.
type MaxTurnsError struct {
	MaxTurns  int
	TurnCount int
}

func (e *MaxTurnsError) Error() string {
	return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyLoopMaxTurnsExceeded, e.MaxTurns)
}

// MessageHistoryLimitError means the model-visible history exceeded the
// bounded sampling limit. The history remains intact: callers must compact it
// explicitly before retrying instead of silently dropping ordinary prompts.
type MessageHistoryLimitError struct {
	MessageCount int
	Limit        int
}

func (e *MessageHistoryLimitError) Error() string {
	if e == nil {
		return ""
	}
	return i18n.Format(
		i18n.DetectOrLoadLanguage(),
		i18n.KeyLoopQueryMessageHistoryLimitExceeded,
		e.MessageCount,
		e.Limit,
	)
}

// retryBaseDelay is the time unit used by retryDelay. Override in tests to avoid
// slow test runs.
var retryBaseDelay = time.Second

const maxProviderRequestRetries = 10

// retryDelay returns the wait duration before the given retry attempt (0-indexed).
// Uses exponential backoff capped at 32× retryBaseDelay.
func retryDelay(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	if attempt > 5 {
		attempt = 5
	}
	return time.Duration(1<<attempt) * retryBaseDelay
}

// IsTransient reports whether err is a transient error that may resolve on retry.
// Transient errors include API rate-limits, overloaded servers, and network failures.
func IsTransient(err error) bool {
	if err == nil {
		return false
	}

	// Check well-typed Anthropic API errors first.
	var apiErr *types.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Type {
		case "overloaded_error", "rate_limit_error":
			return true
		}
		msg := strings.ToLower(apiErr.Message)
		return strings.Contains(msg, "overloaded") ||
			strings.Contains(msg, "too many requests") ||
			strings.Contains(msg, "rate limit") ||
			strings.Contains(msg, "service unavailable")
	}

	// Network / transport errors (plain Go errors from the HTTP layer).
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "reset by peer") ||
		strings.Contains(msg, "eof") ||
		strings.Contains(msg, "temporary") ||
		strings.Contains(msg, "dial")
}

// isPreviousResponseNotFound reports whether err indicates that a
// previous_response_id was rejected by the server (expired or not found).
// When this happens, the caller should clear the response chain and retry
// with full message history.
func isPreviousResponseNotFound(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *types.APIError
	if errors.As(err, &apiErr) {
		if apiErr.Type == "previous_response_not_found" {
			return true
		}
		if apiErr.Type == "invalid_request_error" {
			msg := strings.ToLower(apiErr.Message)
			hasPrev := strings.Contains(msg, "previous_response") || strings.Contains(msg, "previous response")
			hasMissing := strings.Contains(msg, "not found") || strings.Contains(msg, "expired") || strings.Contains(msg, "does not exist")
			return hasPrev && hasMissing
		}
	}
	return false
}

// isStreamInterrupted reports whether err represents a stream-level interruption
// that is safe to retry. This covers upstream WebSocket disconnections, SSE
// stream breaks, and similar transient failures that occur *during* streaming
// (as opposed to *before* the request succeeds, which IsTransient handles).
func isStreamInterrupted(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *types.APIError
	if errors.As(err, &apiErr) {
		if apiErr.Type == "stream_interrupted" {
			return true
		}
		msg := strings.ToLower(apiErr.Message)
		return strings.Contains(msg, "upstream") && (strings.Contains(msg, "closed") || strings.Contains(msg, "disconnect")) ||
			strings.Contains(msg, "websocket closed") ||
			strings.Contains(msg, "stream ended") ||
			strings.Contains(msg, "connection reset")
	}
	// Also check raw errors (e.g. from HTTP layer)
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "stream interrupted") ||
		strings.Contains(msg, "upstream") && strings.Contains(msg, "closed") ||
		strings.Contains(msg, "websocket closed")
}

// isResponseFailedRetryable reports whether err is from a response.failed event
// that represents a transient server-side issue worth retrying. This captures
// errors like "server_error", overloaded backend, etc. that come through the
// stream as response.failed events rather than HTTP-level errors.
//
// Important: this is distinct from isStreamInterrupted (network-level) and
// isPreviousResponseNotFound (parameter-level). It specifically catches
// server-side response failures that didn't produce any content.
func isResponseFailedRetryable(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *types.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	// Don't retry parameter errors or quota errors
	switch apiErr.Type {
	case "invalid_request_error", "authentication_error", "permission_error",
		"previous_response_not_found":
		return false
	}
	// Retry server errors and unknown api_error types
	switch apiErr.Type {
	case "api_error", "server_error":
		return true
	}
	// Retry overloaded/rate-limited if they came through the stream
	switch apiErr.Type {
	case "overloaded_error", "rate_limit_error":
		return true
	}
	return false
}
