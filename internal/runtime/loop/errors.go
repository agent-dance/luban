package loop

import (
	"errors"
	"strings"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/types"
)

// PartialStreamError indicates that a provider response ended before its
// explicit message/response commit event. Any blocks counted here are
// uncommitted observations: callers must not append them to history or execute
// their tool calls. Because processStream never starts tools while receiving
// deltas, the complete generation is safe to replay from the last committed
// history boundary.
type PartialStreamError struct {
	Cause         error
	PartialBlocks int
	OpenBlocks    int
}

func (e *PartialStreamError) Error() string {
	return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyLoopPartialStreamInterrupted, e.PartialBlocks, e.Cause)
}

func (e *PartialStreamError) Unwrap() error { return e.Cause }

// SafeToReplay reports the response-transaction invariant guaranteed by
// processStream. It is a typed extension point for a future provider-native
// resume strategy; today recovery always replays the complete generation.
func (e *PartialStreamError) SafeToReplay() bool { return true }

// AttemptErrorStage, AttemptErrorClass, and AttemptReplaySafety bind the
// runtime's response-transaction evidence into provider.AttemptController's
// transport-neutral retry contract. A partial stream is replay-safe locally
// because neither its message nor its tool batch has been committed.
func (e *PartialStreamError) AttemptErrorStage() types.ProviderErrorStage {
	return types.ProviderErrorStageStream
}

func (e *PartialStreamError) AttemptErrorClass() types.ProviderErrorClass {
	contract := provider.ClassifyAttemptError(e.Cause)
	if contract.Class == types.ProviderErrorClassUnknown {
		// An explicit but unclassified provider failure is not transport
		// evidence. Only a locally observed channel close/reader failure may
		// be promoted to the replayable transport class.
		if _, providerDeclared := provider.AsAPIError(e.Cause); providerDeclared {
			return types.ProviderErrorClassUnknown
		}
		return types.ProviderErrorClassTransport
	}
	return contract.Class
}

func (e *PartialStreamError) AttemptReplaySafety() types.ProviderReplaySafety {
	if apiErr, ok := provider.AsAPIError(e.Cause); ok {
		switch apiErr.ReplaySafety {
		case types.ProviderReplayAmbiguous, types.ProviderReplayUnsafe:
			return apiErr.ReplaySafety
		}
	}
	return types.ProviderReplaySafe
}

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

// terminalProviderErrorEvent preserves a typed provider cause until the
// presentation boundary. Public renderers still redact Message and arbitrary
// Type values; runtimeevent.NewErrorEvent exposes only its small allowlist of
// semantic machine codes. Keeping the typed cause here is what lets the
// benchmark distinguish a provider-declared context exhaustion without
// parsing localized error prose.
func terminalProviderErrorEvent(err error, turnCount int) stream.Event {
	event := stream.Event{Type: stream.EventError, TurnCount: turnCount}
	if err == nil {
		return event
	}
	event.Text = err.Error()
	if apiErr, ok := provider.AsAPIError(err); ok {
		event.Error = apiErr
	}
	return event
}

// retryBaseDelay is the fallback policy for raw providers that are not wrapped
// by RetryProvider. Tests lower it to avoid sleeping on transient failures.
var retryBaseDelay = time.Second

// IsTransient reports whether err is a transient error that may resolve on retry.
// Transient errors include API rate-limits, overloaded servers, and network failures.
func IsTransient(err error) bool {
	return provider.IsRetryable(err)
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
		return strings.EqualFold(strings.TrimSpace(apiErr.Type), "previous_response_not_found") ||
			strings.EqualFold(strings.TrimSpace(apiErr.Code), "previous_response_not_found")
	}
	return false
}

// isStreamInterrupted reports whether err represents a stream-level interruption
// that is safe to retry. This covers upstream WebSocket disconnections, SSE
// stream breaks, and similar transient failures that occur *during* streaming
// (as opposed to *before* the request succeeds, which IsTransient handles).
func isStreamInterrupted(err error) bool {
	contract := provider.ClassifyAttemptError(err)
	return contract.Stage == types.ProviderErrorStageStream &&
		contract.Class == types.ProviderErrorClassTransport &&
		contract.ReplaySafety == types.ProviderReplaySafe
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
	contract := provider.ClassifyAttemptError(err)
	if contract.ReplaySafety != types.ProviderReplaySafe {
		return false
	}
	return contract.Class == types.ProviderErrorClassOverload ||
		contract.Class == types.ProviderErrorClassThrottle
}
