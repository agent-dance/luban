package types

// ProviderFailureDiagnosticSchema is the stable schema stored in authorized
// diagnostic projections and opt-in provider debug logs.
const ProviderFailureDiagnosticSchema = "provider-failure/v1"

// ProviderFailurePoint is a closed, implementation-owned failure location.
// Values describe invariants, never provider-controlled text.
type ProviderFailurePoint string

const (
	ProviderFailureUnknown                          ProviderFailurePoint = "unknown"
	ProviderFailureSSEKnownEventParse               ProviderFailurePoint = "responses.sse.known_event_parse"
	ProviderFailureFunctionDeltaWithoutItem         ProviderFailurePoint = "responses.function.delta_without_item"
	ProviderFailureFunctionDeltaWrongItem           ProviderFailurePoint = "responses.function.delta_wrong_item"
	ProviderFailureFunctionDeltaAfterStop           ProviderFailurePoint = "responses.function.delta_after_stop"
	ProviderFailureFunctionDoneWithoutItem          ProviderFailurePoint = "responses.function.done_without_item"
	ProviderFailureFunctionDoneWrongItem            ProviderFailurePoint = "responses.function.done_wrong_item"
	ProviderFailureFunctionDoneAfterStop            ProviderFailurePoint = "responses.function.done_after_stop"
	ProviderFailureFunctionDoneMissingArguments     ProviderFailurePoint = "responses.function.done_missing_arguments"
	ProviderFailureFunctionDoneItemIDMismatch       ProviderFailurePoint = "responses.function.done_item_id_mismatch"
	ProviderFailureFunctionFinalMismatch            ProviderFailurePoint = "responses.function.final_mismatch"
	ProviderFailureFunctionMissingCallID            ProviderFailurePoint = "responses.function.missing_call_id"
	ProviderFailureFunctionMissingName              ProviderFailurePoint = "responses.function.missing_name"
	ProviderFailureFunctionInvalidStatus            ProviderFailurePoint = "responses.function.invalid_status"
	ProviderFailureFunctionIncomplete               ProviderFailurePoint = "responses.function.incomplete"
	ProviderFailureCustomDeltaWithoutItem           ProviderFailurePoint = "responses.custom.delta_without_item"
	ProviderFailureCustomDeltaWrongItem             ProviderFailurePoint = "responses.custom.delta_wrong_item"
	ProviderFailureCustomDeltaAfterStop             ProviderFailurePoint = "responses.custom.delta_after_stop"
	ProviderFailureCustomDoneWithoutItem            ProviderFailurePoint = "responses.custom.done_without_item"
	ProviderFailureCustomDoneWrongItem              ProviderFailurePoint = "responses.custom.done_wrong_item"
	ProviderFailureCustomDoneAfterStop              ProviderFailurePoint = "responses.custom.done_after_stop"
	ProviderFailureCustomDoneMissingInput           ProviderFailurePoint = "responses.custom.done_missing_input"
	ProviderFailureCustomFinalMismatch              ProviderFailurePoint = "responses.custom.final_mismatch"
	ProviderFailureCustomDuplicateStop              ProviderFailurePoint = "responses.custom.duplicate_stop"
	ProviderFailureCustomMissingCallID              ProviderFailurePoint = "responses.custom.missing_call_id"
	ProviderFailureCustomMissingName                ProviderFailurePoint = "responses.custom.missing_name"
	ProviderFailureCustomInvalidStatus              ProviderFailurePoint = "responses.custom.invalid_status"
	ProviderFailureCompletedOutputGap               ProviderFailurePoint = "responses.completed.output_gap"
	ProviderFailureCompletedFunctionItemUnfinalized ProviderFailurePoint = "responses.completed.function_item_unfinalized"
	ProviderFailureCompletedFunctionStatusInvalid   ProviderFailurePoint = "responses.completed.function_status_invalid"
	ProviderFailureCompletedCustomItemUnfinalized   ProviderFailurePoint = "responses.completed.custom_item_unfinalized"
	ProviderFailureCompletedCustomStatusInvalid     ProviderFailurePoint = "responses.completed.custom_status_invalid"
	ProviderFailureCompletedToolBatchConflict       ProviderFailurePoint = "responses.completed.tool_batch_conflict"
	ProviderFailureResponseIncomplete               ProviderFailurePoint = "responses.response_incomplete"
	ProviderFailureContinuationInvalid              ProviderFailurePoint = "responses.continuation.invalid"
	ProviderFailureServiceTierMismatch              ProviderFailurePoint = "responses.service_tier.mismatch"
	ProviderFailureResponseFailed                   ProviderFailurePoint = "responses.response_failed"
	ProviderFailureStreamIdleTimeout                ProviderFailurePoint = "responses.stream.idle_timeout"
	ProviderFailureStreamInterrupted                ProviderFailurePoint = "responses.stream.interrupted"
	ProviderFailureToolInputLimit                   ProviderFailurePoint = "responses.tool_input.limit"
	ProviderFailureRequestEncode                    ProviderFailurePoint = "responses.request.encode"
	ProviderFailureRequestBuild                     ProviderFailurePoint = "responses.request.build"
	ProviderFailureRequestTransport                 ProviderFailurePoint = "responses.request.transport"
	ProviderFailureRequestHTTPStatus                ProviderFailurePoint = "responses.request.http_status"
	ProviderFailureChatEOFBeforeTerminal            ProviderFailurePoint = "chat.eof_before_terminal"
	ProviderFailureChatFinishReasonMismatch         ProviderFailurePoint = "chat.finish_reason_mismatch"
	ProviderFailureChatIdentityConflict             ProviderFailurePoint = "chat.tool_identity_conflict"
	ProviderFailureChatToolInputLimit               ProviderFailurePoint = "chat.tool_input.limit"
	ProviderFailureChatLateDelta                    ProviderFailurePoint = "chat.late_delta"
	ProviderFailureAnthropicBlockProtocol           ProviderFailurePoint = "anthropic.block_protocol"
	ProviderFailureAnthropicOpenBlock               ProviderFailurePoint = "anthropic.open_block"
	ProviderFailureAnthropicStopReasonMismatch      ProviderFailurePoint = "anthropic.stop_reason_mismatch"
	ProviderFailureAnthropicUnsafeFallback          ProviderFailurePoint = "anthropic.unsafe_fallback"
	ProviderFailureAnthropicToolInputLimit          ProviderFailurePoint = "anthropic.tool_input.limit"
	ProviderFailureRuntimeClosedBeforeCommit        ProviderFailurePoint = "runtime.stream.closed_before_commit"
	ProviderFailureRuntimeProviderErrorBeforeCommit ProviderFailurePoint = "runtime.stream.provider_error_before_commit"
	ProviderFailureRuntimeCommitReceiptMissing      ProviderFailurePoint = "runtime.commit_receipt.missing"
	ProviderFailureRuntimeCommitReceiptMismatch     ProviderFailurePoint = "runtime.commit_receipt.mismatch"
	ProviderFailureRuntimeOpenToolAtCommit          ProviderFailurePoint = "runtime.commit_receipt.open_tool"
)

// ProviderFailureDiagnostic is safe-by-construction evidence for one failed
// provider attempt. Presence flags distinguish absent values from output index
// zero without copying the corresponding provider-controlled identifiers.
type ProviderFailureDiagnostic struct {
	SchemaVersion string `json:"schema_version"`

	LocalRequestID           string `json:"local_request_id,omitempty"`
	UpstreamRequestID        string `json:"upstream_request_id,omitempty"`
	ResponseID               string `json:"response_id,omitempty"`
	Provider                 string `json:"provider,omitempty"`
	Model                    string `json:"model,omitempty"`
	APIFormat                string `json:"api_format,omitempty"`
	Transport                string `json:"transport,omitempty"`
	Endpoint                 string `json:"endpoint,omitempty"`
	HTTPStatus               int    `json:"http_status,omitempty"`
	Attempt                  int    `json:"attempt,omitempty"`
	MaxAttempts              int    `json:"max_attempts,omitempty"`
	EffectiveMaxOutputTokens int    `json:"effective_max_output_tokens,omitempty"`
	CatalogMaxOutputTokens   int    `json:"catalog_max_output_tokens,omitempty"`
	DroppedField             string `json:"dropped_field,omitempty"`
	IncompleteReason         string `json:"incomplete_reason,omitempty"`

	FailurePoint ProviderFailurePoint `json:"failure_point"`
	Stage        ProviderErrorStage   `json:"stage,omitempty"`
	Class        ProviderErrorClass   `json:"class,omitempty"`
	ReplaySafety ProviderReplaySafety `json:"replay_safety,omitempty"`
	Decision     string               `json:"decision,omitempty"`

	WireSequence int    `json:"wire_sequence,omitempty"`
	WireEvent    string `json:"wire_event,omitempty"`
	DataBytes    int    `json:"data_bytes,omitempty"`
	OutputIndex  int    `json:"output_index,omitempty"`
	OutputSet    bool   `json:"output_index_set,omitempty"`
	ItemType     string `json:"item_type,omitempty"`
	ItemStatus   string `json:"item_status,omitempty"`

	MessageStarted    bool `json:"message_started"`
	CommitSeen        bool `json:"commit_seen"`
	TrackedItems      int  `json:"tracked_items,omitempty"`
	CompletedItems    int  `json:"completed_items,omitempty"`
	StoppedItems      int  `json:"stopped_items,omitempty"`
	PartialBlocks     int  `json:"partial_blocks,omitempty"`
	OpenBlocks        int  `json:"open_blocks,omitempty"`
	ItemPresent       bool `json:"item_present,omitempty"`
	ItemStopped       bool `json:"item_stopped,omitempty"`
	CallIDPresent     bool `json:"call_id_present,omitempty"`
	NamePresent       bool `json:"name_present,omitempty"`
	ItemIDPresent     bool `json:"item_id_present,omitempty"`
	FinalSeen         bool `json:"final_seen,omitempty"`
	StreamInputBytes  int  `json:"stream_input_bytes,omitempty"`
	FinalInputBytes   int  `json:"final_input_bytes,omitempty"`
	InputMatchChecked bool `json:"input_match_checked,omitempty"`
	InputMatched      bool `json:"input_matched,omitempty"`
}

// Clone returns an independent copy suitable for adding runtime evidence.
func (d *ProviderFailureDiagnostic) Clone() *ProviderFailureDiagnostic {
	if d == nil {
		return nil
	}
	cloned := *d
	return &cloned
}
