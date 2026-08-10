package types

import (
	"encoding/json"
	"fmt"
)

// StreamEventType represents the type of a streaming event from the API
type StreamEventType string

const (
	EventMessageStart      StreamEventType = "message_start"
	EventContentBlockStart StreamEventType = "content_block_start"
	EventContentBlockDelta StreamEventType = "content_block_delta"
	EventContentBlockStop  StreamEventType = "content_block_stop"
	EventMessageDelta      StreamEventType = "message_delta"
	EventMessageStop       StreamEventType = "message_stop"
	EventError             StreamEventType = "error"
)

// StreamEvent represents a single event from the SSE stream
type StreamEvent struct {
	Type StreamEventType `json:"type"`

	// For content_block_start
	Index        int           `json:"index,omitempty"`
	ContentBlock *ContentDelta `json:"content_block,omitempty"`

	// For content_block_delta
	Delta *ContentDelta `json:"delta,omitempty"`

	// For message_delta
	Usage      *Usage      `json:"usage,omitempty"`
	StopReason *StopReason `json:"stop_reason,omitempty"`

	// For error
	Error *APIError `json:"error,omitempty"`

	// ResponseID is populated in EventMessageStop by providers that return a
	// per-response identifier (e.g. OpenAI Responses API). Callers that need
	// multi-turn chaining should capture this value and pass it back as
	// Params.PreviousResponseID on the next request.
	ResponseID string `json:"response_id,omitempty"`

	// ProviderContinuation is an internal, non-projectable completion receipt.
	// Only the query loop consumes it to attach private replay state to the
	// committed assistant message. It must never enter machine/debug events.
	ProviderContinuation *ProviderContinuation `json:"-"`

	// SystemFingerprint identifies the provider-side serving configuration when
	// the API exposes it. Cache diagnostics use it to distinguish prompt drift
	// from a backend configuration change.
	SystemFingerprint string `json:"system_fingerprint,omitempty"`
}

// ContentDelta represents delta content in streaming
type ContentDelta struct {
	Type ContentType `json:"type"`

	// For text deltas
	Text string `json:"text,omitempty"`

	// For thinking deltas
	Thinking string `json:"thinking,omitempty"`

	// For thinking block start (signature needed for round-trip)
	Signature      string                `json:"signature,omitempty"`
	SignatureKind  ThinkingSignatureKind `json:"signature_kind,omitempty"`
	SignatureModel string                `json:"signature_model,omitempty"`
	ProviderStatus string                `json:"provider_status,omitempty"`
	ThinkingKind   ThinkingKind          `json:"thinking_kind,omitempty"`

	// For tool_use start
	ID       string             `json:"id,omitempty"`
	Name     string             `json:"name,omitempty"`
	ToolType ToolDefinitionType `json:"tool_type,omitempty"`
	// ProviderItemID is the Responses output-item identity. It is distinct
	// from ID, which is the tool call ID used to correlate local results.
	ProviderItemID string `json:"provider_item_id,omitempty"`

	// For tool_use input delta (partial JSON)
	PartialJSON string `json:"partial_json,omitempty"`

	// For custom-tool input deltas. Raw text remains separate from PartialJSON
	// so the runtime can never silently reinterpret an invalid JSON fragment as
	// a freeform patch (or vice versa).
	PartialText string `json:"partial_text,omitempty"`

	// For provider-hosted server tools. RawJSON preserves block variants and
	// fields the common layer does not yet model.
	ToolUseID string          `json:"tool_use_id,omitempty"`
	RawJSON   json.RawMessage `json:"raw_json,omitempty"`
}

// Usage represents provider-normalized token usage. InputTokens is the complete
// prompt size; cache fields are details within that total.
type Usage struct {
	InputTokens              int             `json:"input_tokens"`
	OutputTokens             int             `json:"output_tokens"`
	CacheCreationInputTokens int             `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int             `json:"cache_read_input_tokens,omitempty"`
	ServerToolUse            ServerToolUsage `json:"server_tool_use,omitempty"`
}

// ProviderErrorStage identifies the last provider-request boundary known to
// have been crossed when an error was observed. The value is deliberately
// transport-neutral so HTTP/SSE and Responses WebSocket attempts share one
// retry contract.
type ProviderErrorStage string

const (
	ProviderErrorStageConnect   ProviderErrorStage = "connect"
	ProviderErrorStageHeaders   ProviderErrorStage = "headers"
	ProviderErrorStageStream    ProviderErrorStage = "stream"
	ProviderErrorStageCommitted ProviderErrorStage = "committed"
)

// ProviderErrorClass is the machine-actionable cause class used by the single
// generation-scoped attempt controller. Provider diagnostics remain in Type,
// Code, and Message; retry policy must not parse Message.
type ProviderErrorClass string

const (
	ProviderErrorClassThrottle  ProviderErrorClass = "throttle"
	ProviderErrorClassOverload  ProviderErrorClass = "overload"
	ProviderErrorClassTransport ProviderErrorClass = "transport"
	ProviderErrorClassContext   ProviderErrorClass = "context"
	ProviderErrorClassAuth      ProviderErrorClass = "auth"
	ProviderErrorClassQuota     ProviderErrorClass = "quota"
	ProviderErrorClassPermanent ProviderErrorClass = "permanent"
	ProviderErrorClassUnknown   ProviderErrorClass = "unknown"
)

// ProviderReplaySafety records whether replaying the complete logical
// generation from committed local history is known to be side-effect safe.
type ProviderReplaySafety string

const (
	ProviderReplaySafe      ProviderReplaySafety = "safe"
	ProviderReplayAmbiguous ProviderReplaySafety = "ambiguous"
	ProviderReplayUnsafe    ProviderReplaySafety = "unsafe"
)

// TotalInputTokens returns the complete prompt size processed by the model.
func (u Usage) TotalInputTokens() int {
	if u.InputTokens < 0 {
		return 0
	}
	return u.InputTokens
}

// UncachedInputTokens returns the normal-rate input after removing cache
// details. Malformed provider counts are clamped instead of becoming negative.
func (u Usage) UncachedInputTokens() int {
	uncached := u.TotalInputTokens() - max(u.CacheCreationInputTokens, 0) - max(u.CacheReadInputTokens, 0)
	return max(uncached, 0)
}

// ServerToolUsage mirrors Anthropic usage.server_tool_use.
type ServerToolUsage struct {
	WebSearchRequests int `json:"web_search_requests,omitempty"`
	WebFetchRequests  int `json:"web_fetch_requests,omitempty"`
}

// APIError represents an error from the API
type APIError struct {
	Type                string               `json:"type"`
	Code                string               `json:"code,omitempty"`
	Message             string               `json:"message"`
	Status              int                  `json:"status,omitempty"`      // HTTP status code (0 = unknown)
	RetryAfter          string               `json:"retry_after,omitempty"` // Retry-After header value
	Stage               ProviderErrorStage   `json:"stage,omitempty"`
	Class               ProviderErrorClass   `json:"class,omitempty"`
	ReplaySafety        ProviderReplaySafety `json:"replay_safety,omitempty"`
	OriginalModel       string               `json:"original_model,omitempty"`
	FallbackModel       string               `json:"fallback_model,omitempty"`
	Provider            string               `json:"provider,omitempty"`
	APIFormat           string               `json:"api_format,omitempty"`
	Endpoint            string               `json:"endpoint,omitempty"`
	RequestID           string               `json:"request_id,omitempty"`
	SuggestedAPIFormat  string               `json:"suggested_api_format,omitempty"`
	AttemptedAPIFormats []string             `json:"attempted_api_formats,omitempty"`
}

func (e *APIError) Error() string {
	if e.Status > 0 {
		return fmt.Sprintf("HTTP %d: %s", e.Status, e.Message)
	}
	return e.Message
}
