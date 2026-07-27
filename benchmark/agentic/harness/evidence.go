package harness

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strings"
	"time"
)

func isCollaborationToolName(normalized string) bool {
	switch normalized {
	case "agent", "team", "collaboration", "spawn_agent", "send_message", "wait_agent", "followup_task", "interrupt_agent", "list_agents":
		return true
	default:
		return strings.HasPrefix(normalized, "collaboration_") || strings.HasPrefix(normalized, "agent_")
	}
}

func toolCallBoundToLocalCatalog(call ToolCallEvidence, definitions []ToolDefinitionEvidence) bool {
	for _, definition := range definitions {
		if definition.BillingOwner != "client" {
			continue
		}
		switch definition.Type {
		case "function":
			if call.Kind == "function_call" && call.Name == definition.Name {
				return true
			}
		case "custom":
			if call.Kind == "custom_tool_call" && call.Name == definition.Name {
				return true
			}
		case "apply_patch":
			if call.Kind == "apply_patch_call" && call.Name == "apply_patch" {
				return true
			}
		case "shell":
			if (call.Kind == "shell_call" || call.Kind == "local_shell_call") && (call.Name == "shell" || call.Name == "local_shell") {
				return true
			}
		}
	}
	return false
}

type formalToolIdentity struct {
	Type string
	Name string
}

func formalToolCatalog(agentID string) []formalToolIdentity {
	switch agentID {
	case "codex":
		return []formalToolIdentity{{Type: "custom", Name: "exec"}, {Type: "function", Name: "wait"}, {Type: "function", Name: "request_user_input"}}
	case "luban":
		return []formalToolIdentity{{Type: "function", Name: "Inspect"}, {Type: "function", Name: "ApplyPatch"}, {Type: "function", Name: "Run"}}
	default:
		return nil
	}
}

func exactFormalToolCatalog(agentID string, definitions []ToolDefinitionEvidence) bool {
	expected := formalToolCatalog(agentID)
	if len(expected) == 0 || len(definitions) != len(expected) {
		return false
	}
	for index, identity := range expected {
		definition := definitions[index]
		if definition.Type != identity.Type || definition.Name != identity.Name || definition.BillingOwner != "client" {
			return false
		}
	}
	return true
}

func validateExpectedFormalToolCatalog(catalog ToolCatalogSpec) (string, error) {
	if catalog.SchemaVersion != FormalToolCatalogSchemaVersion || !hex64Pattern.MatchString(catalog.SemanticSHA256) {
		return "", errors.New("expected model request does not bind a formal semantic tool catalog")
	}
	for _, agentID := range []string{"codex", "luban"} {
		expected := formalToolCatalog(agentID)
		if len(catalog.Tools) != len(expected) {
			continue
		}
		matches := true
		for index, identity := range expected {
			tool := catalog.Tools[index]
			if tool.Type != identity.Type || tool.Name != identity.Name || !hex64Pattern.MatchString(tool.DefinitionSHA256) {
				matches = false
				break
			}
		}
		if matches {
			return agentID, nil
		}
	}
	return "", errors.New("expected model request does not bind an exact formal agent tool catalog")
}

func toolCatalogMatchesSpec(definitions []ToolDefinitionEvidence, catalog ToolCatalogSpec) bool {
	if len(definitions) != len(catalog.Tools) || stableToolCatalogSHA256(definitions) != catalog.SemanticSHA256 {
		return false
	}
	for index, definition := range definitions {
		expected := catalog.Tools[index]
		if definition.Type != expected.Type || definition.Name != expected.Name || definition.DefinitionSHA256 != expected.DefinitionSHA256 {
			return false
		}
	}
	return true
}

type stableToolDefinitionProjection struct {
	Type              string `json:"type"`
	Name              string `json:"name"`
	BillingOwner      string `json:"billing_owner"`
	Strict            *bool  `json:"strict,omitempty"`
	SchemaSHA256      string `json:"schema_sha256,omitempty"`
	SchemaBytes       int64  `json:"schema_bytes"`
	DescriptionSHA256 string `json:"description_sha256,omitempty"`
	DescriptionBytes  int64  `json:"description_bytes"`
	DefinitionSHA256  string `json:"definition_sha256"`
	DefinitionBytes   int64  `json:"definition_bytes"`
}

func stableToolCatalogSHA256(definitions []ToolDefinitionEvidence) string {
	projection := make([]stableToolDefinitionProjection, 0, len(definitions))
	for _, definition := range definitions {
		projection = append(projection, stableToolDefinitionProjection{
			Type: definition.Type, Name: definition.Name, BillingOwner: definition.BillingOwner, Strict: definition.Strict,
			SchemaSHA256: definition.SchemaSHA256, SchemaBytes: definition.SchemaBytes,
			DescriptionSHA256: definition.DescriptionSHA256, DescriptionBytes: definition.DescriptionBytes,
			DefinitionSHA256: definition.DefinitionSHA256, DefinitionBytes: definition.DefinitionBytes,
		})
	}
	raw, err := json.Marshal(projection)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

// StableToolCatalogSHA256 returns the cross-run semantic identity used by the
// formal evidence archive. Unlike the per-run catalog HMAC it contains no
// secret material and can be independently recomputed by artifact validators.
func StableToolCatalogSHA256(definitions []ToolDefinitionEvidence) string {
	return stableToolCatalogSHA256(definitions)
}

// ProviderRoundEvidence is the normalized, auditable unit of model usage. One
// record means one actual provider request, not one UI message or tool event.
type ProviderRoundEvidence struct {
	SchemaVersion                            string                   `json:"schema_version"`
	EvidenceSequence                         uint64                   `json:"evidence_sequence"`
	PreviousEvidenceHash                     string                   `json:"previous_evidence_hash,omitempty"`
	EvidenceHash                             string                   `json:"evidence_hash"`
	Round                                    int                      `json:"round"`
	RunIdentity                              string                   `json:"run_identity"`
	ProviderAttemptStarted                   bool                     `json:"provider_attempt_started"`
	Transport                                string                   `json:"transport"`
	ProviderAttemptKind                      string                   `json:"provider_attempt_kind"`
	WebSocketConnectionHash                  string                   `json:"websocket_connection_hash,omitempty"`
	WebSocketRequestSequence                 uint64                   `json:"websocket_request_sequence,omitempty"`
	WebSocketConnectionReused                bool                     `json:"websocket_connection_reused"`
	WebSocketHandshakeStatus                 int                      `json:"websocket_handshake_status,omitempty"`
	WebSocketHandshakeModel                  string                   `json:"websocket_handshake_model,omitempty"`
	WebSocketChainBound                      bool                     `json:"websocket_chain_bound"`
	GenerateSpecified                        bool                     `json:"generate_specified"`
	Generate                                 bool                     `json:"generate"`
	TransportDisposition                     string                   `json:"transport_disposition"`
	Outcome                                  string                   `json:"outcome"`
	ErrorCode                                string                   `json:"error_code,omitempty"`
	RequestID                                string                   `json:"request_id_hash,omitempty"`
	ResponseIDHash                           string                   `json:"response_id_hash,omitempty"`
	StartedAt                                time.Time                `json:"started_at"`
	UpstreamHeadersAt                        time.Time                `json:"upstream_headers_at"`
	FirstResponseByteAt                      time.Time                `json:"first_response_byte_at"`
	FinishedAt                               time.Time                `json:"finished_at"`
	Provider                                 string                   `json:"provider"`
	Model                                    string                   `json:"model"`
	ReasoningEffort                          string                   `json:"reasoning_effort"`
	RequestedReasoningContext                string                   `json:"requested_reasoning_context"`
	RequestedReasoningMode                   string                   `json:"requested_reasoning_mode,omitempty"`
	RequestedReasoningModeCanonical          string                   `json:"requested_reasoning_mode_canonical"`
	RequestedTextVerbosity                   string                   `json:"requested_text_verbosity,omitempty"`
	MaxOutputTokensSpecified                 bool                     `json:"max_output_tokens_specified"`
	MaxOutputTokens                          *int64                   `json:"max_output_tokens,omitempty"`
	StoreSpecified                           bool                     `json:"store_specified"`
	Store                                    bool                     `json:"store"`
	PreviousResponseIDPresent                bool                     `json:"previous_response_id_present"`
	PreviousResponseIDHash                   string                   `json:"previous_response_id_hash,omitempty"`
	PromptCacheKeyPresent                    bool                     `json:"prompt_cache_key_present"`
	PromptCacheKeyHash                       string                   `json:"prompt_cache_key_hash,omitempty"`
	CachePolicyObserved                      bool                     `json:"cache_policy_observed"`
	PromptCacheOptionsPresent                bool                     `json:"prompt_cache_options_present"`
	PromptCacheOptionsMode                   string                   `json:"prompt_cache_options_mode,omitempty"`
	PromptCacheTTLSeconds                    *int64                   `json:"prompt_cache_ttl_seconds,omitempty"`
	PromptCacheRetentionPresent              bool                     `json:"prompt_cache_retention_present"`
	PromptCacheRetention                     string                   `json:"prompt_cache_retention,omitempty"`
	CacheBreakpointCount                     int                      `json:"cache_breakpoint_count"`
	CacheBreakpointPositionHashes            []string                 `json:"cache_breakpoint_position_hashes,omitempty"`
	EncryptedReasoningRequested              bool                     `json:"encrypted_reasoning_requested"`
	EncryptedReasoningItemCount              int                      `json:"encrypted_reasoning_item_count"`
	EncryptedReasoningHashes                 []string                 `json:"encrypted_reasoning_hashes,omitempty"`
	EncryptedReasoningReplayCount            int                      `json:"encrypted_reasoning_replay_count"`
	EncryptedReasoningReplayHashes           []string                 `json:"encrypted_reasoning_replay_hashes,omitempty"`
	EncryptedReasoningReplayBound            bool                     `json:"encrypted_reasoning_replay_bound"`
	ReplayOutputItemCount                    int                      `json:"replay_output_item_count"`
	ReplayOutputItemHashes                   []string                 `json:"replay_output_item_hashes,omitempty"`
	ReplayOutputItemsBound                   bool                     `json:"replay_output_items_bound"`
	ResponseOutputItemCount                  int                      `json:"response_output_item_count"`
	ResponseOutputItemHashes                 []string                 `json:"response_output_item_hashes,omitempty"`
	ContinuationLineagePresent               bool                     `json:"continuation_lineage_present"`
	ContinuationLineageHash                  string                   `json:"continuation_lineage_hash,omitempty"`
	ContinuationLineageSource                string                   `json:"continuation_lineage_source"`
	ContinuationEpoch                        uint64                   `json:"continuation_epoch,omitempty"`
	ContinuationReset                        bool                     `json:"continuation_reset"`
	ContinuationResetAccepted                bool                     `json:"continuation_reset_accepted"`
	ContinuationResetUnknown                 bool                     `json:"continuation_reset_unknown"`
	RequestedServiceTierRaw                  string                   `json:"requested_service_tier_raw,omitempty"`
	RequestedServiceTierPresent              bool                     `json:"requested_service_tier_present"`
	RequestedServiceTierCanonical            string                   `json:"requested_service_tier_canonical"`
	RequestedServiceTierRepresentation       string                   `json:"requested_service_tier_representation"`
	ClientCanonicalizationProofSHA256        string                   `json:"client_canonicalization_proof_sha256,omitempty"`
	ClientAgentID                            string                   `json:"client_agent_id"`
	OriginalRequestBodySHA256                string                   `json:"original_request_body_sha256"`
	ForwardedRequestBodySHA256               string                   `json:"forwarded_request_body_sha256"`
	OriginalRequestCanonicalSHA256           string                   `json:"original_request_canonical_sha256"`
	ForwardedRequestCanonicalSHA256          string                   `json:"forwarded_request_canonical_sha256"`
	OriginalRequestWithoutServiceTierSHA256  string                   `json:"original_request_without_service_tier_sha256"`
	ForwardedRequestWithoutServiceTierSHA256 string                   `json:"forwarded_request_without_service_tier_sha256"`
	OriginalServiceTierPresent               bool                     `json:"original_service_tier_present"`
	OriginalServiceTier                      string                   `json:"original_service_tier,omitempty"`
	ForwardedServiceTierPresent              bool                     `json:"forwarded_service_tier_present"`
	ForwardedServiceTier                     string                   `json:"forwarded_service_tier,omitempty"`
	ForwardedRequestBytes                    int64                    `json:"forwarded_request_bytes"`
	ServiceTierTransformation                string                   `json:"service_tier_transformation"`
	ServiceTierTransformationExactDiff       bool                     `json:"service_tier_transformation_exact_diff"`
	ServiceTierTransformationProofSHA256     string                   `json:"service_tier_transformation_proof_sha256"`
	ResponseServiceTierRaw                   string                   `json:"response_service_tier_raw,omitempty"`
	ResponseServiceTierCanonical             string                   `json:"response_service_tier_canonical"`
	ServiceTierComparable                    bool                     `json:"service_tier_comparable"`
	ToolDefinitionCount                      int                      `json:"tool_definition_count"`
	ToolDefinitions                          []ToolDefinitionEvidence `json:"tool_definitions,omitempty"`
	ToolCatalogHash                          string                   `json:"tool_catalog_hash,omitempty"`
	ToolCatalogSemanticSHA256                string                   `json:"tool_catalog_semantic_sha256"`
	ToolCatalogCanonicalBytes                int64                    `json:"tool_catalog_canonical_bytes"`
	ToolCatalogCompared                      bool                     `json:"tool_catalog_compared"`
	ToolCatalogStable                        bool                     `json:"tool_catalog_stable"`
	ToolResultHistoryValid                   bool                     `json:"tool_result_history_valid"`
	ResponseCreatedModel                     string                   `json:"response_created_model,omitempty"`
	ResponseModel                            string                   `json:"response_model,omitempty"`
	ResponseCompleted                        bool                     `json:"response_completed"`
	ResponseStatus                           string                   `json:"response_status,omitempty"`
	ResponseFailureCode                      string                   `json:"response_failure_code,omitempty"`
	ResponseFailureEventSHA256               string                   `json:"response_failure_event_sha256,omitempty"`
	HTTPStatus                               int                      `json:"http_status"`
	RequestBytes                             int64                    `json:"request_bytes"`
	ResponseBytes                            int64                    `json:"response_bytes"`
	UsagePresent                             bool                     `json:"usage_present"`
	InputTokens                              *int64                   `json:"input_tokens,omitempty"`
	CachedInputTokens                        *int64                   `json:"cached_input_tokens,omitempty"`
	CacheWriteInputTokens                    *int64                   `json:"cache_write_input_tokens,omitempty"`
	// OutputTokens is the billed total, including reasoning tokens.
	// ReasoningOutputTokens is a diagnostic subset and is never billed twice.
	OutputTokens          *int64             `json:"output_tokens,omitempty"`
	ReasoningOutputTokens *int64             `json:"reasoning_output_tokens,omitempty"`
	ProviderReportedCost  *float64           `json:"provider_reported_cost,omitempty"`
	ToolCalls             []ToolCallEvidence `json:"tool_calls,omitempty"`
	// ToolResultPayloadBytes includes repeated result payloads resent in later
	// stateless requests. Per-call OutputBytes below counts each logical result
	// once, so the two measurements expose context retransmission separately.
	ToolResultPayloadBytes int64 `json:"tool_result_payload_bytes,omitempty"`
	// PhysicalToolOperations counts adapter-reported child operations. It can
	// exceed logical ToolCalls when one model-visible call fans out internally.
	PhysicalToolOperations *int   `json:"physical_tool_operations,omitempty"`
	ToolCriticalPathMS     *int64 `json:"tool_critical_path_ms,omitempty"`
	ToolTotalLatencyMS     *int64 `json:"tool_total_latency_ms,omitempty"`
	ToolQueueMS            *int64 `json:"tool_queue_ms,omitempty"`
}

type ToolCallEvidence struct {
	ID                    string `json:"id"`
	Kind                  string `json:"kind"`
	Name                  string `json:"name"`
	DurationMS            *int64 `json:"duration_ms,omitempty"`
	Error                 *bool  `json:"error,omitempty"`
	InputBytes            int64  `json:"input_bytes"`
	OutputBytes           *int64 `json:"output_bytes,omitempty"`
	AgentTraceOutputBytes *int64 `json:"agent_trace_output_bytes,omitempty"`
	TraceMatch            string `json:"trace_match,omitempty"`
	TraceKind             string `json:"trace_kind,omitempty"`
}

type ToolDefinitionEvidence struct {
	Type              string `json:"type"`
	Name              string `json:"name"`
	BillingOwner      string `json:"billing_owner"`
	Strict            *bool  `json:"strict,omitempty"`
	SchemaHash        string `json:"schema_hash,omitempty"`
	SchemaSHA256      string `json:"schema_sha256,omitempty"`
	SchemaBytes       int64  `json:"schema_bytes"`
	DescriptionSHA256 string `json:"description_sha256,omitempty"`
	DescriptionBytes  int64  `json:"description_bytes"`
	DefinitionSHA256  string `json:"definition_sha256"`
	DefinitionBytes   int64  `json:"definition_bytes"`
}

type UsageMetrics struct {
	TransportAttempts                       int              `json:"transport_attempts"`
	PrewarmAttempts                         int              `json:"prewarm_attempts"`
	PrewarmErrors                           int              `json:"prewarm_errors"`
	BillableInferenceRequests               int              `json:"billable_inference_requests"`
	LLMCallsStarted                         int              `json:"llm_calls_started"`
	CompletedLLMResponses                   int              `json:"completed_llm_responses"`
	RetryAmplification                      *float64         `json:"retry_amplification,omitempty"`
	HTTPInferenceRequests                   int              `json:"http_inference_requests"`
	WebSocketInferenceRequests              int              `json:"websocket_inference_requests"`
	WebSocketConnections                    int              `json:"websocket_connections"`
	PrewarmUsageObservations                int              `json:"prewarm_usage_observations"`
	PrewarmInputTokens                      int64            `json:"prewarm_input_tokens"`
	PrewarmCachedInputTokens                int64            `json:"prewarm_cached_input_tokens"`
	PrewarmOutputTokens                     int64            `json:"prewarm_output_tokens"`
	PrewarmUnknownCostAttempts              int              `json:"prewarm_unknown_cost_attempts"`
	CostReceiptObservations                 int              `json:"cost_receipt_observations"`
	CostReceiptTotal                        int              `json:"cost_receipt_total"`
	UnknownCostAttempts                     int              `json:"unknown_cost_attempts"`
	AllExecutedUsageObservations            int              `json:"all_executed_usage_observations"`
	AllExecutedInputTokens                  int64            `json:"all_executed_input_tokens"`
	AllExecutedCachedInputTokens            int64            `json:"all_executed_cached_input_tokens"`
	AllExecutedOutputTokens                 int64            `json:"all_executed_output_tokens"`
	AllExecutedCacheWriteInputTokens        int64            `json:"all_executed_cache_write_input_tokens"`
	AllExecutedCacheWriteObservations       int              `json:"all_executed_cache_write_observations"`
	AllExecutedUnreportedCacheWriteAttempts int              `json:"all_executed_unreported_cache_write_attempts"`
	AllExecutedCacheHitRate                 *float64         `json:"all_executed_cache_hit_rate,omitempty"`
	AllExecutedCacheHitRatePartial          *float64         `json:"all_executed_cache_hit_rate_partial,omitempty"`
	ProviderRequests                        int              `json:"provider_requests"`
	ProviderRounds                          int              `json:"provider_rounds"`
	ProviderErrors                          int              `json:"provider_errors"`
	ToolBearingRounds                       int              `json:"tool_bearing_rounds"`
	ToolInvocations                         int              `json:"tool_invocations"`
	ToolErrors                              int              `json:"tool_errors"`
	ToolErrorObservations                   int              `json:"tool_error_observations"`
	ToolDurationObservations                int              `json:"tool_duration_observations"`
	ToolOutputObservations                  int              `json:"tool_output_observations"`
	ToolTraceIDMatches                      int              `json:"tool_trace_id_matches"`
	ToolTraceOrderedMatches                 int              `json:"tool_trace_ordered_matches"`
	ToolTraceUnmatched                      int              `json:"tool_trace_unmatched"`
	ToolResultPayloadBytes                  int64            `json:"tool_result_payload_bytes"`
	UniqueToolOutputBytes                   int64            `json:"unique_tool_output_bytes"`
	UsageReceiptObservations                int              `json:"usage_receipt_observations"`
	UsageReceiptTotal                       int              `json:"usage_receipt_total"`
	ReasoningTokenObservations              int              `json:"reasoning_token_observations"`
	CacheWriteTokenObservations             int              `json:"cache_write_token_observations"`
	CacheWriteInputTokens                   int64            `json:"cache_write_input_tokens"`
	UnreportedCacheWriteRounds              int              `json:"unreported_cache_write_rounds"`
	PreviousResponseIDRequests              int              `json:"previous_response_id_requests"`
	PromptCacheKeyRequests                  int              `json:"prompt_cache_key_requests"`
	CachePolicyObservedRequests             int              `json:"cache_policy_observed_requests"`
	CacheKeyPresentRequests                 int              `json:"cache_key_present_requests"`
	CacheUniqueKeyCount                     int              `json:"cache_unique_key_count"`
	CacheKeyTransitions                     int              `json:"cache_key_transitions"`
	CacheLineageStable                      bool             `json:"cache_lineage_stable"`
	EncryptedReasoningRequests              int              `json:"encrypted_reasoning_requests"`
	EncryptedReasoningItems                 int              `json:"encrypted_reasoning_items"`
	EncryptedReasoningReplays               int              `json:"encrypted_reasoning_replays"`
	EncryptedReplayBoundRounds              int              `json:"encrypted_replay_bound_rounds"`
	ReplayOutputItems                       int              `json:"replay_output_items"`
	ResponseOutputItems                     int              `json:"response_output_items"`
	ReplayOutputBoundRounds                 int              `json:"replay_output_bound_rounds"`
	ContinuationResetRounds                 int              `json:"continuation_reset_rounds"`
	ContinuationResetsAccepted              int              `json:"continuation_resets_accepted"`
	PhysicalToolOperations                  int              `json:"physical_tool_operations"`
	PhysicalToolObservations                int              `json:"physical_tool_observations"`
	ToolCriticalPathMS                      int64            `json:"tool_critical_path_ms"`
	ToolCriticalObservations                int              `json:"tool_critical_observations"`
	ToolTotalLatencyMS                      int64            `json:"tool_total_latency_ms"`
	ToolTotalObservations                   int              `json:"tool_total_observations"`
	ToolQueueMS                             int64            `json:"tool_queue_ms"`
	ToolQueueObservations                   int              `json:"tool_queue_observations"`
	InputTokens                             int64            `json:"input_tokens"`
	CachedInputTokens                       int64            `json:"cached_input_tokens"`
	OutputTokens                            int64            `json:"output_tokens"`
	ReasoningOutputTokens                   int64            `json:"reasoning_output_tokens"`
	CacheHitRate                            *float64         `json:"cache_hit_rate,omitempty"`
	CacheHitRatePartial                     *float64         `json:"cache_hit_rate_partial,omitempty"`
	ToolCallsByName                         map[string]int   `json:"tool_calls_by_name"`
	ToolDurationMSByName                    map[string]int64 `json:"tool_duration_ms_by_name"`
	CatalogCost                             *float64         `json:"catalog_cost,omitempty"`
	CatalogCostPartial                      *float64         `json:"catalog_cost_partial,omitempty"`
	KnownCatalogCostLowerBound              float64          `json:"known_catalog_cost_lower_bound"`
	KnownCacheWriteSurcharge                float64          `json:"known_cache_write_surcharge"`
	ProviderReportedCost                    *float64         `json:"provider_reported_cost,omitempty"`
	ProviderReportedCostPartial             *float64         `json:"provider_reported_cost_partial,omitempty"`
	ProviderCostObservations                int              `json:"provider_cost_observations"`
}

func ValidateAndAggregateEvidence(rounds []ProviderRoundEvidence, expected ModelRequestSpec, catalog PricingCatalog) (UsageMetrics, error) {
	if len(rounds) == 0 {
		return UsageMetrics{}, errors.New("provider request evidence is empty")
	}
	if expected.ServiceTier != "default" {
		return UsageMetrics{}, errors.New("expected model request must explicitly pin the default service tier")
	}
	switch expected.TransportRequirement {
	case TransportRequirementHTTPInference, TransportRequirementWebSocket:
	default:
		return UsageMetrics{}, errors.New("expected model request does not bind an exact transport requirement")
	}
	expectedAgentID, err := validateExpectedFormalToolCatalog(expected.ToolCatalog)
	if err != nil {
		return UsageMetrics{}, err
	}
	rate, err := pricingRate(catalog, expected)
	if err != nil {
		return UsageMetrics{}, err
	}
	metrics := UsageMetrics{ToolCallsByName: map[string]int{}, ToolDurationMSByName: map[string]int64{}}
	requestIDs := map[string]struct{}{}
	responseIDs := map[string]struct{}{}
	evidenceSequences := map[uint64]struct{}{}
	roundNumbers := map[int]struct{}{}
	webSocketSequences := map[string][]uint64{}
	webSocketConnections := map[string]struct{}{}
	toolIDs := map[string]struct{}{}
	runIdentity := ""
	previousEvidenceHash := ""
	providerCostSeen := false
	providerCost := 0.0
	catalogCostObserved := 0.0
	catalogLowerBoundObserved := false
	formalCatalogSemanticSHA256 := ""
	for index, round := range rounds {
		if round.SchemaVersion != "agentic-bench/provider-round-v2" {
			return UsageMetrics{}, fmt.Errorf("provider round %d has an unsupported evidence schema", round.Round)
		}
		if round.Round < 0 {
			return UsageMetrics{}, fmt.Errorf("provider round %d has a negative start ordinal", round.Round)
		}
		if _, duplicate := roundNumbers[round.Round]; duplicate {
			return UsageMetrics{}, fmt.Errorf("provider round start ordinal %d is duplicated", round.Round)
		}
		roundNumbers[round.Round] = struct{}{}
		if !hex64Pattern.MatchString(round.RunIdentity) {
			return UsageMetrics{}, fmt.Errorf("provider round %d has an invalid run identity", round.Round)
		}
		if round.EvidenceSequence != uint64(index) {
			return UsageMetrics{}, fmt.Errorf("provider round %d has noncontiguous evidence sequence %d", round.Round, round.EvidenceSequence)
		}
		if _, duplicate := evidenceSequences[round.EvidenceSequence]; duplicate {
			return UsageMetrics{}, fmt.Errorf("provider round %d duplicates evidence sequence %d", round.Round, round.EvidenceSequence)
		}
		evidenceSequences[round.EvidenceSequence] = struct{}{}
		if !round.ProviderAttemptStarted || !hex64Pattern.MatchString(round.EvidenceHash) || (round.PreviousEvidenceHash != "" && !hex64Pattern.MatchString(round.PreviousEvidenceHash)) {
			return UsageMetrics{}, fmt.Errorf("provider round %d lacks sealed transport-chain evidence", round.Round)
		}
		if round.PreviousEvidenceHash != previousEvidenceHash {
			return UsageMetrics{}, fmt.Errorf("provider round %d breaks the transport evidence hash chain", round.Round)
		}
		previousEvidenceHash = round.EvidenceHash
		if round.TransportDisposition != "valid" && round.TransportDisposition != "prewarm_transport" && round.TransportDisposition != "provider_infra_exclusion" && round.TransportDisposition != "agent_context_failure" {
			return UsageMetrics{}, fmt.Errorf("provider round %d has non-scoreable transport disposition %q", round.Round, round.TransportDisposition)
		}
		if runIdentity == "" {
			runIdentity = round.RunIdentity
		} else if round.RunIdentity != runIdentity {
			return UsageMetrics{}, fmt.Errorf("provider round %d crosses evidence run identities", round.Round)
		}
		if round.Outcome != "success" && round.Outcome != "prewarm" && round.Outcome != "error" {
			return UsageMetrics{}, fmt.Errorf("provider round %d has invalid outcome", round.Round)
		}
		if round.RequestID != "" {
			if _, exists := requestIDs[round.RequestID]; exists {
				return UsageMetrics{}, fmt.Errorf("provider request ID %s is duplicated", round.RequestID)
			}
			requestIDs[round.RequestID] = struct{}{}
			if !hex64Pattern.MatchString(round.RequestID) {
				return UsageMetrics{}, fmt.Errorf("provider round %d has an invalid request ID hash", round.Round)
			}
		}
		if round.ResponseIDHash != "" && !hex64Pattern.MatchString(round.ResponseIDHash) {
			return UsageMetrics{}, fmt.Errorf("provider round %d has an invalid response ID hash", round.Round)
		}
		if round.ResponseIDHash != "" && (round.Outcome == "success" || round.Outcome == "prewarm") {
			if _, duplicate := responseIDs[round.ResponseIDHash]; duplicate {
				return UsageMetrics{}, fmt.Errorf("provider response ID %s is duplicated", round.ResponseIDHash)
			}
			responseIDs[round.ResponseIDHash] = struct{}{}
		}
		if round.Provider != expected.Provider || round.Model != expected.Model || round.ReasoningEffort != expected.ReasoningEffort {
			return UsageMetrics{}, fmt.Errorf("provider round %d requested %s/%s/%s, expected %s/%s/%s", round.Round, round.Provider, round.Model, round.ReasoningEffort, expected.Provider, expected.Model, expected.ReasoningEffort)
		}
		if round.RequestedServiceTierRepresentation != expected.ServiceTierRequestEncoding {
			return UsageMetrics{}, fmt.Errorf("provider round %d used service-tier encoding %q, expected %q", round.Round, round.RequestedServiceTierRepresentation, expected.ServiceTierRequestEncoding)
		}
		switch round.RequestedServiceTierRepresentation {
		case "explicit_default":
			if !round.RequestedServiceTierPresent || round.RequestedServiceTierRaw != "default" || round.RequestedServiceTierCanonical != "default" || round.ClientCanonicalizationProofSHA256 != "" || round.ClientAgentID != "luban" ||
				!round.OriginalServiceTierPresent || round.OriginalServiceTier != "default" || !round.ForwardedServiceTierPresent || round.ForwardedServiceTier != "default" ||
				round.ServiceTierTransformation != "none" || !round.ServiceTierTransformationExactDiff ||
				round.OriginalRequestBodySHA256 != round.ForwardedRequestBodySHA256 || round.OriginalRequestCanonicalSHA256 != round.ForwardedRequestCanonicalSHA256 ||
				round.OriginalRequestWithoutServiceTierSHA256 != round.ForwardedRequestWithoutServiceTierSHA256 || round.ForwardedRequestBytes != round.RequestBytes {
				return UsageMetrics{}, fmt.Errorf("provider round %d has invalid explicit-default service-tier evidence", round.Round)
			}
		case "client_canonicalized_default":
			if round.RequestedServiceTierPresent || round.RequestedServiceTierRaw != "" || round.RequestedServiceTierCanonical != "default" || !hex64Pattern.MatchString(round.ClientCanonicalizationProofSHA256) || round.ClientAgentID != "codex" ||
				round.OriginalServiceTierPresent || round.OriginalServiceTier != "" || !round.ForwardedServiceTierPresent || round.ForwardedServiceTier != "default" ||
				round.ServiceTierTransformation != "inject_explicit_default" || !round.ServiceTierTransformationExactDiff ||
				round.OriginalRequestBodySHA256 == round.ForwardedRequestBodySHA256 || round.OriginalRequestCanonicalSHA256 == round.ForwardedRequestCanonicalSHA256 ||
				round.OriginalRequestWithoutServiceTierSHA256 != round.ForwardedRequestWithoutServiceTierSHA256 || round.ForwardedRequestBytes < 1 {
				return UsageMetrics{}, fmt.Errorf("provider round %d has invalid Codex service-tier omission evidence", round.Round)
			}
		default:
			return UsageMetrics{}, fmt.Errorf("provider round %d has non-comparable service-tier representation %q", round.Round, round.RequestedServiceTierRepresentation)
		}
		for _, digest := range []string{
			round.OriginalRequestBodySHA256, round.ForwardedRequestBodySHA256,
			round.OriginalRequestCanonicalSHA256, round.ForwardedRequestCanonicalSHA256,
			round.OriginalRequestWithoutServiceTierSHA256, round.ForwardedRequestWithoutServiceTierSHA256,
			round.ServiceTierTransformationProofSHA256,
		} {
			if !hex64Pattern.MatchString(digest) {
				return UsageMetrics{}, fmt.Errorf("provider round %d has incomplete controller service-tier transformation evidence", round.Round)
			}
		}
		if round.RequestedReasoningModeCanonical != "standard" {
			return UsageMetrics{}, fmt.Errorf("provider round %d used non-comparable reasoning mode %q", round.Round, round.RequestedReasoningModeCanonical)
		}
		if round.MaxOutputTokensSpecified && (round.MaxOutputTokens == nil || *round.MaxOutputTokens < 0) || !round.MaxOutputTokensSpecified && round.MaxOutputTokens != nil {
			return UsageMetrics{}, fmt.Errorf("provider round %d has invalid max-output-token strategy evidence", round.Round)
		}
		if round.ProviderAttemptKind != "inference" && round.ProviderAttemptKind != "prewarm" {
			return UsageMetrics{}, fmt.Errorf("provider round %d has invalid provider-attempt kind %q", round.Round, round.ProviderAttemptKind)
		}
		switch expected.TransportRequirement {
		case TransportRequirementHTTPInference:
			if round.Transport != "http_sse" || round.ProviderAttemptKind != "inference" {
				return UsageMetrics{}, fmt.Errorf("provider round %d violates the HTTP-inference-only transport contract", round.Round)
			}
		case TransportRequirementWebSocket:
			if round.Transport != "websocket" {
				return UsageMetrics{}, fmt.Errorf("provider round %d violates the WebSocket-only transport contract", round.Round)
			}
		}
		switch round.ProviderAttemptKind {
		case "prewarm":
			if round.Transport != "websocket" || !round.GenerateSpecified || round.Generate {
				return UsageMetrics{}, fmt.Errorf("provider round %d has inconsistent generate/prewarm evidence", round.Round)
			}
		case "inference":
			if round.Transport == "websocket" && round.GenerateSpecified && !round.Generate ||
				round.Transport == "http_sse" && round.GenerateSpecified {
				return UsageMetrics{}, fmt.Errorf("provider round %d has inconsistent generate/inference evidence", round.Round)
			}
		}
		switch round.Transport {
		case "http_sse":
			if round.ProviderAttemptKind != "inference" || round.WebSocketConnectionHash != "" || round.WebSocketRequestSequence != 0 || round.WebSocketConnectionReused || round.WebSocketHandshakeStatus != 0 || round.WebSocketHandshakeModel != "" || !round.WebSocketChainBound {
				return UsageMetrics{}, fmt.Errorf("provider round %d has invalid HTTP transport evidence", round.Round)
			}
		case "websocket":
			if !hex64Pattern.MatchString(round.WebSocketConnectionHash) || round.WebSocketHandshakeStatus != http.StatusSwitchingProtocols || round.WebSocketHandshakeModel != expected.Model || !round.WebSocketChainBound || round.WebSocketConnectionReused != (round.WebSocketRequestSequence > 0) {
				return UsageMetrics{}, fmt.Errorf("provider round %d has invalid WebSocket transport evidence", round.Round)
			}
			webSocketSequences[round.WebSocketConnectionHash] = append(webSocketSequences[round.WebSocketConnectionHash], round.WebSocketRequestSequence)
			webSocketConnections[round.WebSocketConnectionHash] = struct{}{}
		default:
			return UsageMetrics{}, fmt.Errorf("provider round %d has invalid transport %q", round.Round, round.Transport)
		}
		if !round.StoreSpecified || round.Store {
			return UsageMetrics{}, fmt.Errorf("provider round %d did not explicitly disable response storage", round.Round)
		}
		if round.PreviousResponseIDPresent != (round.PreviousResponseIDHash != "") || (round.PreviousResponseIDHash != "" && !hex64Pattern.MatchString(round.PreviousResponseIDHash)) {
			return UsageMetrics{}, fmt.Errorf("provider round %d has invalid previous-response evidence", round.Round)
		}
		if round.PromptCacheKeyPresent != (round.PromptCacheKeyHash != "") || (round.PromptCacheKeyHash != "" && !hex64Pattern.MatchString(round.PromptCacheKeyHash)) {
			return UsageMetrics{}, fmt.Errorf("provider round %d has invalid prompt-cache-key evidence", round.Round)
		}
		if err := ValidateCacheRequestEvidence(round); err != nil {
			return UsageMetrics{}, fmt.Errorf("provider round %d has invalid cache-policy evidence: %w", round.Round, err)
		}
		if round.EncryptedReasoningItemCount != len(round.EncryptedReasoningHashes) || round.EncryptedReasoningReplayCount != len(round.EncryptedReasoningReplayHashes) || (round.EncryptedReasoningReplayCount > 0) != round.EncryptedReasoningReplayBound ||
			(!round.EncryptedReasoningRequested && (round.EncryptedReasoningItemCount != 0 || round.EncryptedReasoningReplayCount != 0)) {
			return UsageMetrics{}, fmt.Errorf("provider round %d has invalid encrypted-reasoning evidence", round.Round)
		}
		for _, digest := range append(slices.Clone(round.EncryptedReasoningHashes), round.EncryptedReasoningReplayHashes...) {
			if !hex64Pattern.MatchString(digest) {
				return UsageMetrics{}, fmt.Errorf("provider round %d has an invalid encrypted-reasoning hash", round.Round)
			}
		}
		if round.ReplayOutputItemCount != len(round.ReplayOutputItemHashes) || round.ResponseOutputItemCount != len(round.ResponseOutputItemHashes) || (round.ReplayOutputItemCount > 0) != round.ReplayOutputItemsBound {
			return UsageMetrics{}, fmt.Errorf("provider round %d has invalid output-item replay evidence", round.Round)
		}
		if round.ContinuationLineageSource != "controller_default" && round.ContinuationLineageSource != "agent_header" && round.ContinuationLineageSource != "websocket_connection" && round.ContinuationLineageSource != "websocket_client_metadata" {
			return UsageMetrics{}, fmt.Errorf("provider round %d has invalid continuation-lineage source", round.Round)
		}
		for _, digest := range append(slices.Clone(round.ReplayOutputItemHashes), round.ResponseOutputItemHashes...) {
			if !hex64Pattern.MatchString(digest) {
				return UsageMetrics{}, fmt.Errorf("provider round %d has an invalid output-item hash", round.Round)
			}
		}
		if round.ContinuationLineagePresent != (round.ContinuationLineageHash != "") ||
			(round.ContinuationLineagePresent && (!hex64Pattern.MatchString(round.ContinuationLineageHash) || round.ContinuationEpoch == 0 || round.ContinuationResetAccepted != round.ContinuationReset)) ||
			(!round.ContinuationLineagePresent && (round.ContinuationEpoch != 0 || round.ContinuationReset || round.ContinuationResetAccepted)) {
			return UsageMetrics{}, fmt.Errorf("provider round %d has invalid continuation-lineage evidence", round.Round)
		}
		if round.ContinuationResetUnknown && (round.ContinuationLineageSource == "agent_header" || round.ContinuationResetAccepted) {
			return UsageMetrics{}, fmt.Errorf("provider round %d has invalid controller reset evidence", round.Round)
		}
		if round.ToolDefinitionCount != len(round.ToolDefinitions) || !hex64Pattern.MatchString(round.ToolCatalogHash) || !hex64Pattern.MatchString(round.ToolCatalogSemanticSHA256) || round.ToolCatalogCanonicalBytes < 0 || !round.ToolCatalogStable || !round.ToolResultHistoryValid {
			return UsageMetrics{}, fmt.Errorf("provider round %d has invalid tool-catalog evidence", round.Round)
		}
		if round.ClientAgentID != expectedAgentID || !exactFormalToolCatalog(round.ClientAgentID, round.ToolDefinitions) {
			return UsageMetrics{}, fmt.Errorf("provider round %d does not expose the exact frozen %s local tool catalog", round.Round, round.ClientAgentID)
		}
		catalogCanonicalBytes := int64(0)
		for _, definition := range round.ToolDefinitions {
			if definition.Type == "" || definition.Name == "" || definition.BillingOwner != "client" || definition.SchemaBytes < 0 || definition.DescriptionBytes < 0 || definition.DefinitionBytes < 1 ||
				!hex64Pattern.MatchString(definition.DefinitionSHA256) ||
				(definition.SchemaBytes > 0) != (hex64Pattern.MatchString(definition.SchemaHash) && hex64Pattern.MatchString(definition.SchemaSHA256)) ||
				(definition.DescriptionBytes > 0) != hex64Pattern.MatchString(definition.DescriptionSHA256) {
				return UsageMetrics{}, fmt.Errorf("provider round %d has invalid tool definition", round.Round)
			}
			catalogCanonicalBytes += definition.DefinitionBytes
			normalizedName := strings.ToLower(strings.ReplaceAll(definition.Name, "-", "_"))
			normalizedType := strings.ToLower(strings.ReplaceAll(definition.Type, "-", "_"))
			switch definition.Type {
			case "function", "custom", "apply_patch", "shell":
			default:
				return UsageMetrics{}, fmt.Errorf("provider round %d exposes non-local or unknown tool type %q", round.Round, definition.Type)
			}
			if strings.Contains(normalizedName, "web_search") || strings.Contains(normalizedType, "web_search") {
				return UsageMetrics{}, fmt.Errorf("provider round %d exposes forbidden web-search tooling", round.Round)
			}
			if isCollaborationToolName(normalizedName) || isCollaborationToolName(normalizedType) {
				return UsageMetrics{}, fmt.Errorf("provider round %d exposes forbidden multi-agent tooling %q", round.Round, definition.Name)
			}
		}
		if catalogCanonicalBytes != round.ToolCatalogCanonicalBytes || stableToolCatalogSHA256(round.ToolDefinitions) != round.ToolCatalogSemanticSHA256 {
			return UsageMetrics{}, fmt.Errorf("provider round %d has inconsistent stable tool-catalog evidence", round.Round)
		}
		if !toolCatalogMatchesSpec(round.ToolDefinitions, expected.ToolCatalog) || round.ToolCatalogSemanticSHA256 != expected.ToolCatalog.SemanticSHA256 {
			return UsageMetrics{}, fmt.Errorf("provider round %d changed the manifest-bound tool catalog", round.Round)
		}
		if formalCatalogSemanticSHA256 == "" {
			formalCatalogSemanticSHA256 = round.ToolCatalogSemanticSHA256
		} else if round.ToolCatalogSemanticSHA256 != formalCatalogSemanticSHA256 {
			return UsageMetrics{}, fmt.Errorf("provider round %d changed the frozen semantic tool-catalog identity", round.Round)
		}
		if round.StartedAt.IsZero() || round.FinishedAt.Before(round.StartedAt) || round.RequestBytes < 1 || round.ResponseBytes < 0 || round.ToolResultPayloadBytes < 0 {
			return UsageMetrics{}, fmt.Errorf("provider round %d has invalid transport evidence", round.Round)
		}
		if (round.ResponseFailureCode == "") != (round.ResponseFailureEventSHA256 == "") {
			return UsageMetrics{}, fmt.Errorf("provider round %d has partial response-failure evidence", round.Round)
		}
		if round.TransportDisposition == "agent_context_failure" {
			if round.Outcome != "error" || round.ProviderAttemptKind != "inference" || round.ErrorCode != "provider_context_failure" ||
				round.ResponseFailureCode != "context_length_exceeded" || !hex64Pattern.MatchString(round.ResponseFailureEventSHA256) ||
				round.ResponseCompleted || (round.ResponseStatus != "" && round.ResponseStatus != "failed") {
				return UsageMetrics{}, fmt.Errorf("provider round %d has invalid sealed context-failure evidence", round.Round)
			}
		} else if round.ResponseFailureCode != "" {
			return UsageMetrics{}, fmt.Errorf("provider round %d has response-failure evidence under the wrong disposition", round.Round)
		}
		switch round.Outcome {
		case "success":
			if round.ProviderAttemptKind != "inference" || round.TransportDisposition != "valid" {
				return UsageMetrics{}, fmt.Errorf("provider round %d marks a non-inference response as scoreable success", round.Round)
			}
			if !round.ServiceTierComparable || round.ResponseServiceTierRaw != "default" || round.ResponseServiceTierCanonical != "default" {
				return UsageMetrics{}, fmt.Errorf("provider round %d has non-comparable service tier", round.Round)
			}
			if round.ResponseCreatedModel != "" && round.ResponseCreatedModel != expected.Model || round.ResponseModel != expected.Model {
				return UsageMetrics{}, fmt.Errorf("provider round %d has mismatched served-model evidence", round.Round)
			}
			if round.ErrorCode != "" || !round.ResponseCompleted || round.ResponseStatus != "completed" || !hex64Pattern.MatchString(round.ResponseIDHash) || round.FinishedAt.Before(round.FirstResponseByteAt) || round.ResponseBytes < 1 {
				return UsageMetrics{}, fmt.Errorf("provider round %d lacks complete successful-response evidence", round.Round)
			}
			if round.Transport == "http_sse" {
				if !hex64Pattern.MatchString(round.RequestID) || round.UpstreamHeadersAt.Before(round.StartedAt) || round.FirstResponseByteAt.Before(round.UpstreamHeadersAt) || round.HTTPStatus < 200 || round.HTTPStatus >= 300 {
					return UsageMetrics{}, fmt.Errorf("provider round %d lacks complete HTTP response evidence", round.Round)
				}
			} else if round.HTTPStatus != 0 || round.FirstResponseByteAt.Before(round.StartedAt) {
				return UsageMetrics{}, fmt.Errorf("provider round %d lacks complete WebSocket response evidence", round.Round)
			}
		case "prewarm":
			if round.ProviderAttemptKind != "prewarm" || round.Transport != "websocket" || round.TransportDisposition != "prewarm_transport" || round.ErrorCode != "" || !round.ServiceTierComparable || round.ResponseServiceTierRaw != "default" || round.ResponseServiceTierCanonical != "default" || !round.ResponseCompleted || round.ResponseStatus != "completed" || !hex64Pattern.MatchString(round.ResponseIDHash) || round.ResponseModel != expected.Model || round.HTTPStatus != 0 || round.FirstResponseByteAt.Before(round.StartedAt) || round.FinishedAt.Before(round.FirstResponseByteAt) || round.ResponseBytes < 1 || len(round.ToolCalls) != 0 {
				return UsageMetrics{}, fmt.Errorf("provider round %d lacks complete WebSocket prewarm evidence", round.Round)
			}
		case "error":
			if round.TransportDisposition != "provider_infra_exclusion" && round.TransportDisposition != "agent_context_failure" || round.ErrorCode == "" || round.ErrorCode == "pinned_request_mismatch" || round.ErrorCode == "response_storage_not_disabled" || round.ErrorCode == "reasoning_mode_not_comparable" {
				return UsageMetrics{}, fmt.Errorf("provider round %d has an inadmissible error outcome", round.Round)
			}
		}
		if round.UsagePresent {
			if round.InputTokens == nil || round.CachedInputTokens == nil || round.OutputTokens == nil || *round.InputTokens < 0 || *round.CachedInputTokens < 0 || *round.OutputTokens < 0 || *round.CachedInputTokens > *round.InputTokens {
				return UsageMetrics{}, fmt.Errorf("provider round %d lacks an atomic token-usage receipt", round.Round)
			}
			if round.ReasoningOutputTokens != nil && *round.ReasoningOutputTokens > *round.OutputTokens {
				return UsageMetrics{}, fmt.Errorf("provider round %d has invalid reasoning-token usage", round.Round)
			}
		} else if round.InputTokens != nil || round.CachedInputTokens != nil || round.OutputTokens != nil || round.ReasoningOutputTokens != nil {
			return UsageMetrics{}, fmt.Errorf("provider round %d has token values without a usage receipt", round.Round)
		}
		if round.CacheWriteInputTokens != nil {
			if !round.UsagePresent || *round.CacheWriteInputTokens < 0 || *round.CacheWriteInputTokens > *round.InputTokens-*round.CachedInputTokens {
				return UsageMetrics{}, fmt.Errorf("provider round %d has invalid cache-write token usage", round.Round)
			}
		}
		if round.ReasoningOutputTokens != nil && *round.ReasoningOutputTokens < 0 {
			return UsageMetrics{}, fmt.Errorf("provider round %d has invalid reasoning-token usage", round.Round)
		}
		// Physical child operations can be lower than model-visible calls when a
		// revision barrier cancels or skips a call before it starts. Preserve both
		// measurements; neither is a validity bound for the other.
		if round.PhysicalToolOperations != nil && *round.PhysicalToolOperations < 0 {
			return UsageMetrics{}, fmt.Errorf("provider round %d has invalid operational tool evidence", round.Round)
		}
		for _, value := range []*int64{round.ToolCriticalPathMS, round.ToolTotalLatencyMS, round.ToolQueueMS} {
			if value != nil && *value < 0 {
				return UsageMetrics{}, fmt.Errorf("provider round %d has invalid operational tool evidence", round.Round)
			}
		}
		operationalFields := 0
		if round.PhysicalToolOperations != nil {
			operationalFields++
		}
		for _, value := range []*int64{round.ToolCriticalPathMS, round.ToolTotalLatencyMS, round.ToolQueueMS} {
			if value != nil {
				operationalFields++
			}
		}
		if operationalFields != 0 && operationalFields != 4 {
			return UsageMetrics{}, fmt.Errorf("provider round %d has a partial operational tool receipt", round.Round)
		}
		if len(round.ToolCalls) == 0 && operationalFields == 4 && (*round.PhysicalToolOperations != 0 || *round.ToolCriticalPathMS != 0 || *round.ToolTotalLatencyMS != 0 || *round.ToolQueueMS != 0) {
			return UsageMetrics{}, fmt.Errorf("provider round %d has operations without model-visible tool calls", round.Round)
		}
		metrics.TransportAttempts++
		metrics.CostReceiptTotal++
		exactCostReceipt := false
		if round.UsagePresent {
			metrics.AllExecutedUsageObservations++
			metrics.AllExecutedInputTokens += *round.InputTokens
			metrics.AllExecutedCachedInputTokens += *round.CachedInputTokens
			metrics.AllExecutedOutputTokens += *round.OutputTokens
			if round.CacheWriteInputTokens != nil {
				metrics.AllExecutedCacheWriteObservations++
				metrics.AllExecutedCacheWriteInputTokens += *round.CacheWriteInputTokens
			} else if rate.CacheWriteInputMultiplier > 1 {
				metrics.AllExecutedUnreportedCacheWriteAttempts++
			}
			costAttributionKnown := round.ServiceTierComparable && round.ResponseServiceTierRaw == "default" && round.ResponseServiceTierCanonical == "default" && round.ResponseModel == expected.Model
			if costAttributionKnown {
				roundCost, cacheWriteSurcharge := catalogRoundCost(*round.InputTokens, *round.CachedInputTokens, round.CacheWriteInputTokens, *round.OutputTokens, catalog.UnitTokens, rate)
				catalogCostObserved += roundCost
				metrics.KnownCatalogCostLowerBound += roundCost
				metrics.KnownCacheWriteSurcharge += cacheWriteSurcharge
				catalogLowerBoundObserved = true
				exactCostReceipt = round.CacheWriteInputTokens != nil || rate.CacheWriteInputMultiplier <= 1
			}
		}
		if exactCostReceipt {
			metrics.CostReceiptObservations++
		} else {
			metrics.UnknownCostAttempts++
		}
		if round.ProviderReportedCost != nil {
			if *round.ProviderReportedCost < 0 || math.IsNaN(*round.ProviderReportedCost) || math.IsInf(*round.ProviderReportedCost, 0) {
				return UsageMetrics{}, fmt.Errorf("provider round %d has invalid reported cost", round.Round)
			}
			providerCostSeen = true
			metrics.ProviderCostObservations++
			providerCost += *round.ProviderReportedCost
		}
		if round.ProviderAttemptKind == "prewarm" {
			metrics.PrewarmAttempts++
			if round.Outcome == "error" {
				metrics.PrewarmErrors++
			}
			if round.UsagePresent {
				metrics.PrewarmUsageObservations++
				metrics.PrewarmInputTokens += *round.InputTokens
				metrics.PrewarmCachedInputTokens += *round.CachedInputTokens
				metrics.PrewarmOutputTokens += *round.OutputTokens
			}
			if !exactCostReceipt {
				metrics.PrewarmUnknownCostAttempts++
			}
			continue
		}
		metrics.ProviderRequests++
		metrics.BillableInferenceRequests++
		metrics.LLMCallsStarted++
		if round.ResponseCompleted && round.ResponseStatus == "completed" {
			metrics.CompletedLLMResponses++
		}
		metrics.UsageReceiptTotal++
		if round.Transport == "websocket" {
			metrics.WebSocketInferenceRequests++
		} else {
			metrics.HTTPInferenceRequests++
		}
		if round.Outcome == "success" {
			metrics.ProviderRounds++
		} else {
			metrics.ProviderErrors++
		}
		if round.PreviousResponseIDPresent {
			metrics.PreviousResponseIDRequests++
		}
		if round.PromptCacheKeyPresent {
			metrics.PromptCacheKeyRequests++
		}
		if round.EncryptedReasoningRequested {
			metrics.EncryptedReasoningRequests++
		}
		metrics.EncryptedReasoningItems += round.EncryptedReasoningItemCount
		metrics.EncryptedReasoningReplays += round.EncryptedReasoningReplayCount
		metrics.ReplayOutputItems += round.ReplayOutputItemCount
		metrics.ResponseOutputItems += round.ResponseOutputItemCount
		if round.EncryptedReasoningReplayBound {
			metrics.EncryptedReplayBoundRounds++
		}
		if round.ReplayOutputItemsBound {
			metrics.ReplayOutputBoundRounds++
		}
		if round.ContinuationReset {
			metrics.ContinuationResetRounds++
		}
		if round.ContinuationResetAccepted {
			metrics.ContinuationResetsAccepted++
		}
		metrics.ToolResultPayloadBytes += round.ToolResultPayloadBytes
		if round.UsagePresent {
			metrics.UsageReceiptObservations++
			metrics.InputTokens += *round.InputTokens
			metrics.CachedInputTokens += *round.CachedInputTokens
			metrics.OutputTokens += *round.OutputTokens
			if round.CacheWriteInputTokens != nil {
				metrics.CacheWriteTokenObservations++
				metrics.CacheWriteInputTokens += *round.CacheWriteInputTokens
			} else if rate.CacheWriteInputMultiplier > 1 {
				metrics.UnreportedCacheWriteRounds++
			}
		}
		if round.ReasoningOutputTokens != nil {
			metrics.ReasoningTokenObservations++
			metrics.ReasoningOutputTokens += *round.ReasoningOutputTokens
		}
		if round.PhysicalToolOperations != nil {
			metrics.PhysicalToolObservations++
			metrics.PhysicalToolOperations += *round.PhysicalToolOperations
		}
		if round.ToolCriticalPathMS != nil {
			metrics.ToolCriticalObservations++
			metrics.ToolCriticalPathMS += *round.ToolCriticalPathMS
		}
		if round.ToolTotalLatencyMS != nil {
			metrics.ToolTotalObservations++
			metrics.ToolTotalLatencyMS += *round.ToolTotalLatencyMS
		}
		if round.ToolQueueMS != nil {
			metrics.ToolQueueObservations++
			metrics.ToolQueueMS += *round.ToolQueueMS
		}
		if len(round.ToolCalls) > 0 {
			metrics.ToolBearingRounds++
		}
		for _, call := range round.ToolCalls {
			if call.ID == "" || call.Kind == "" || call.Name == "" || call.InputBytes < 0 || (call.DurationMS != nil && *call.DurationMS < 0) || (call.OutputBytes != nil && *call.OutputBytes < 0) || (call.AgentTraceOutputBytes != nil && *call.AgentTraceOutputBytes < 0) {
				return UsageMetrics{}, fmt.Errorf("provider round %d has invalid tool evidence", round.Round)
			}
			if !toolCallBoundToLocalCatalog(call, round.ToolDefinitions) {
				return UsageMetrics{}, fmt.Errorf("provider round %d contains a tool call outside its exact client-local catalog", round.Round)
			}
			executionClaimed := call.TraceMatch != "" || call.DurationMS != nil || call.Error != nil || call.OutputBytes != nil || call.AgentTraceOutputBytes != nil
			if executionClaimed && (call.TraceMatch == "" || call.OutputBytes == nil) {
				return UsageMetrics{}, fmt.Errorf("provider round %d lacks a closed client execution receipt for tool %s", round.Round, call.Name)
			}
			if _, exists := toolIDs[call.ID]; exists {
				return UsageMetrics{}, fmt.Errorf("tool call ID %s is duplicated", call.ID)
			}
			toolIDs[call.ID] = struct{}{}
			metrics.ToolInvocations++
			metrics.ToolCallsByName[call.Name]++
			if call.DurationMS != nil {
				metrics.ToolDurationObservations++
				metrics.ToolDurationMSByName[call.Name] += *call.DurationMS
			}
			if call.Error != nil {
				metrics.ToolErrorObservations++
			}
			if call.Error != nil && *call.Error {
				metrics.ToolErrors++
			}
			if call.OutputBytes != nil {
				metrics.ToolOutputObservations++
				metrics.UniqueToolOutputBytes += *call.OutputBytes
			}
			switch call.TraceMatch {
			case "id":
				metrics.ToolTraceIDMatches++
			case "ordered_kind":
				metrics.ToolTraceOrderedMatches++
			case "":
				metrics.ToolTraceUnmatched++
			default:
				return UsageMetrics{}, fmt.Errorf("provider round %d has an invalid trace match", round.Round)
			}
		}
	}
	for ordinal := 0; ordinal < len(rounds); ordinal++ {
		if _, present := roundNumbers[ordinal]; !present {
			return UsageMetrics{}, fmt.Errorf("provider start-order view is missing round %d", ordinal)
		}
	}
	for connectionHash, sequences := range webSocketSequences {
		slices.Sort(sequences)
		for ordinal, sequence := range sequences {
			if sequence != uint64(ordinal) {
				return UsageMetrics{}, fmt.Errorf("websocket connection %s has noncontiguous response.create sequence %d", connectionHash, sequence)
			}
		}
	}
	metrics.WebSocketConnections = len(webSocketConnections)
	switch expected.TransportRequirement {
	case TransportRequirementHTTPInference:
		if metrics.HTTPInferenceRequests != metrics.TransportAttempts || metrics.WebSocketInferenceRequests != 0 || metrics.PrewarmAttempts != 0 || metrics.WebSocketConnections != 0 {
			return UsageMetrics{}, errors.New("provider evidence does not close the HTTP-inference-only transport contract")
		}
	case TransportRequirementWebSocket:
		if metrics.HTTPInferenceRequests != 0 || metrics.WebSocketInferenceRequests+metrics.PrewarmAttempts != metrics.TransportAttempts || metrics.WebSocketConnections == 0 {
			return UsageMetrics{}, errors.New("provider evidence does not close the WebSocket-only transport contract")
		}
	}
	cacheLineage := SummarizeProviderCacheLineage(rounds)
	metrics.CachePolicyObservedRequests = cacheLineage.ObservedRequests
	metrics.CacheKeyPresentRequests = cacheLineage.KeyPresentRequests
	metrics.CacheUniqueKeyCount = cacheLineage.UniqueKeyCount
	metrics.CacheKeyTransitions = cacheLineage.KeyTransitions
	metrics.CacheLineageStable = cacheLineage.Stable
	if cacheLineage.ObservedRequests > 0 || cacheLineage.KeyPresentRequests > 0 {
		if cacheLineage.ObservedRequests != metrics.TransportAttempts ||
			cacheLineage.KeyPresentRequests != cacheLineage.ObservedRequests ||
			cacheLineage.UniqueKeyCount != 1 || cacheLineage.KeyTransitions != 0 || !cacheLineage.Stable {
			return UsageMetrics{}, errors.New("provider evidence has incomplete or unstable per-run prompt-cache lineage")
		}
	}
	if metrics.InputTokens > 0 {
		cacheRate := float64(metrics.CachedInputTokens) / float64(metrics.InputTokens)
		if metrics.UsageReceiptObservations == metrics.UsageReceiptTotal {
			metrics.CacheHitRate = &cacheRate
		} else {
			metrics.CacheHitRatePartial = &cacheRate
		}
	}
	if metrics.AllExecutedInputTokens > 0 {
		cacheRate := float64(metrics.AllExecutedCachedInputTokens) / float64(metrics.AllExecutedInputTokens)
		if metrics.AllExecutedUsageObservations == metrics.TransportAttempts {
			metrics.AllExecutedCacheHitRate = &cacheRate
		} else {
			metrics.AllExecutedCacheHitRatePartial = &cacheRate
		}
	}
	if metrics.CostReceiptObservations == metrics.CostReceiptTotal && metrics.AllExecutedUnreportedCacheWriteAttempts == 0 {
		metrics.CatalogCost = &catalogCostObserved
	} else if catalogLowerBoundObserved {
		metrics.CatalogCostPartial = &catalogCostObserved
	}
	if providerCostSeen && metrics.ProviderCostObservations == metrics.TransportAttempts {
		metrics.ProviderReportedCost = &providerCost
	} else if providerCostSeen {
		metrics.ProviderReportedCostPartial = &providerCost
	}
	return metrics, nil
}

func pricingRate(catalog PricingCatalog, expected ModelRequestSpec) (PricingRate, error) {
	for _, rate := range catalog.Rates {
		if rate.Provider == expected.Provider && rate.Model == expected.Model {
			return rate, nil
		}
	}
	return PricingRate{}, fmt.Errorf("pricing catalog has no rate for %s/%s", expected.Provider, expected.Model)
}

func catalogRoundCost(input, cached int64, cacheWrite *int64, output, unit int64, rate PricingRate) (total, cacheWriteSurcharge float64) {
	inputMultiplier, cachedMultiplier, outputMultiplier := 1.0, 1.0, 1.0
	for _, tier := range rate.RequestTiers {
		if input > tier.ThresholdInputTokens {
			inputMultiplier, cachedMultiplier, outputMultiplier = tier.InputMultiplier, tier.CachedInputMultiplier, tier.OutputMultiplier
		}
	}
	uncached := input - cached
	total = float64(uncached)/float64(unit)*rate.Input*inputMultiplier +
		float64(cached)/float64(unit)*rate.CachedInput*cachedMultiplier +
		float64(output)/float64(unit)*rate.Output*outputMultiplier
	if cacheWrite != nil {
		cacheWriteMultiplier := rate.CacheWriteInputMultiplier
		if cacheWriteMultiplier == 0 {
			cacheWriteMultiplier = 1
		}
		cacheWriteSurcharge = float64(*cacheWrite) / float64(unit) * rate.Input * inputMultiplier * (cacheWriteMultiplier - 1)
		total += cacheWriteSurcharge
	}
	return total, cacheWriteSurcharge
}

func MergeUsageMetrics(values []UsageMetrics) UsageMetrics {
	merged := UsageMetrics{ToolCallsByName: map[string]int{}, ToolDurationMSByName: map[string]int64{}}
	providerCostSeen, catalogCostSeen := false, false
	providerCost, catalogCost := 0.0, 0.0
	cacheLineageObserved, cacheLineageAllStable := false, true
	for _, value := range values {
		merged.TransportAttempts += value.TransportAttempts
		merged.PrewarmAttempts += value.PrewarmAttempts
		merged.PrewarmErrors += value.PrewarmErrors
		merged.BillableInferenceRequests += value.BillableInferenceRequests
		merged.LLMCallsStarted += value.LLMCallsStarted
		merged.CompletedLLMResponses += value.CompletedLLMResponses
		merged.HTTPInferenceRequests += value.HTTPInferenceRequests
		merged.WebSocketInferenceRequests += value.WebSocketInferenceRequests
		merged.WebSocketConnections += value.WebSocketConnections
		merged.PrewarmUsageObservations += value.PrewarmUsageObservations
		merged.PrewarmInputTokens += value.PrewarmInputTokens
		merged.PrewarmCachedInputTokens += value.PrewarmCachedInputTokens
		merged.PrewarmOutputTokens += value.PrewarmOutputTokens
		merged.PrewarmUnknownCostAttempts += value.PrewarmUnknownCostAttempts
		merged.CostReceiptObservations += value.CostReceiptObservations
		merged.CostReceiptTotal += value.CostReceiptTotal
		merged.UnknownCostAttempts += value.UnknownCostAttempts
		merged.AllExecutedUsageObservations += value.AllExecutedUsageObservations
		merged.AllExecutedInputTokens += value.AllExecutedInputTokens
		merged.AllExecutedCachedInputTokens += value.AllExecutedCachedInputTokens
		merged.AllExecutedOutputTokens += value.AllExecutedOutputTokens
		merged.AllExecutedCacheWriteInputTokens += value.AllExecutedCacheWriteInputTokens
		merged.AllExecutedCacheWriteObservations += value.AllExecutedCacheWriteObservations
		merged.AllExecutedUnreportedCacheWriteAttempts += value.AllExecutedUnreportedCacheWriteAttempts
		merged.ProviderRequests += value.ProviderRequests
		merged.ProviderRounds += value.ProviderRounds
		merged.ProviderErrors += value.ProviderErrors
		merged.ToolBearingRounds += value.ToolBearingRounds
		merged.ToolInvocations += value.ToolInvocations
		merged.ToolErrors += value.ToolErrors
		merged.ToolErrorObservations += value.ToolErrorObservations
		merged.ToolDurationObservations += value.ToolDurationObservations
		merged.ToolOutputObservations += value.ToolOutputObservations
		merged.ToolTraceIDMatches += value.ToolTraceIDMatches
		merged.ToolTraceOrderedMatches += value.ToolTraceOrderedMatches
		merged.ToolTraceUnmatched += value.ToolTraceUnmatched
		merged.ToolResultPayloadBytes += value.ToolResultPayloadBytes
		merged.UniqueToolOutputBytes += value.UniqueToolOutputBytes
		merged.UsageReceiptObservations += value.UsageReceiptObservations
		merged.UsageReceiptTotal += value.UsageReceiptTotal
		merged.ReasoningTokenObservations += value.ReasoningTokenObservations
		merged.CacheWriteTokenObservations += value.CacheWriteTokenObservations
		merged.CacheWriteInputTokens += value.CacheWriteInputTokens
		merged.UnreportedCacheWriteRounds += value.UnreportedCacheWriteRounds
		merged.PreviousResponseIDRequests += value.PreviousResponseIDRequests
		merged.PromptCacheKeyRequests += value.PromptCacheKeyRequests
		merged.CachePolicyObservedRequests += value.CachePolicyObservedRequests
		merged.CacheKeyPresentRequests += value.CacheKeyPresentRequests
		merged.CacheUniqueKeyCount += value.CacheUniqueKeyCount
		merged.CacheKeyTransitions += value.CacheKeyTransitions
		if value.CachePolicyObservedRequests > 0 {
			cacheLineageObserved = true
			cacheLineageAllStable = cacheLineageAllStable && value.CacheLineageStable
		}
		merged.EncryptedReasoningRequests += value.EncryptedReasoningRequests
		merged.EncryptedReasoningItems += value.EncryptedReasoningItems
		merged.EncryptedReasoningReplays += value.EncryptedReasoningReplays
		merged.EncryptedReplayBoundRounds += value.EncryptedReplayBoundRounds
		merged.ReplayOutputItems += value.ReplayOutputItems
		merged.ResponseOutputItems += value.ResponseOutputItems
		merged.ReplayOutputBoundRounds += value.ReplayOutputBoundRounds
		merged.ContinuationResetRounds += value.ContinuationResetRounds
		merged.ContinuationResetsAccepted += value.ContinuationResetsAccepted
		merged.PhysicalToolOperations += value.PhysicalToolOperations
		merged.PhysicalToolObservations += value.PhysicalToolObservations
		merged.ToolCriticalPathMS += value.ToolCriticalPathMS
		merged.ToolCriticalObservations += value.ToolCriticalObservations
		merged.ToolTotalLatencyMS += value.ToolTotalLatencyMS
		merged.ToolTotalObservations += value.ToolTotalObservations
		merged.ToolQueueMS += value.ToolQueueMS
		merged.ToolQueueObservations += value.ToolQueueObservations
		merged.InputTokens += value.InputTokens
		merged.CachedInputTokens += value.CachedInputTokens
		merged.OutputTokens += value.OutputTokens
		merged.ReasoningOutputTokens += value.ReasoningOutputTokens
		merged.ProviderCostObservations += value.ProviderCostObservations
		merged.KnownCacheWriteSurcharge += value.KnownCacheWriteSurcharge
		merged.KnownCatalogCostLowerBound += value.KnownCatalogCostLowerBound
		for name, count := range value.ToolCallsByName {
			merged.ToolCallsByName[name] += count
		}
		for name, duration := range value.ToolDurationMSByName {
			merged.ToolDurationMSByName[name] += duration
		}
		if value.CatalogCost != nil {
			catalogCostSeen = true
			catalogCost += *value.CatalogCost
		} else if value.CatalogCostPartial != nil {
			catalogCostSeen = true
			catalogCost += *value.CatalogCostPartial
		}
		if value.ProviderReportedCost != nil {
			providerCostSeen = true
			providerCost += *value.ProviderReportedCost
		} else if value.ProviderReportedCostPartial != nil {
			providerCostSeen = true
			providerCost += *value.ProviderReportedCostPartial
		}
	}
	merged.CacheLineageStable = cacheLineageObserved && cacheLineageAllStable
	if merged.InputTokens > 0 {
		cacheRate := float64(merged.CachedInputTokens) / float64(merged.InputTokens)
		if merged.UsageReceiptObservations == merged.UsageReceiptTotal {
			merged.CacheHitRate = &cacheRate
		} else {
			merged.CacheHitRatePartial = &cacheRate
		}
	}
	if merged.AllExecutedInputTokens > 0 {
		cacheRate := float64(merged.AllExecutedCachedInputTokens) / float64(merged.AllExecutedInputTokens)
		if merged.AllExecutedUsageObservations == merged.TransportAttempts {
			merged.AllExecutedCacheHitRate = &cacheRate
		} else {
			merged.AllExecutedCacheHitRatePartial = &cacheRate
		}
	}
	if providerCostSeen {
		if merged.ProviderCostObservations == merged.TransportAttempts {
			merged.ProviderReportedCost = &providerCost
		} else {
			merged.ProviderReportedCostPartial = &providerCost
		}
	}
	if catalogCostSeen {
		if merged.CostReceiptObservations == merged.CostReceiptTotal && merged.AllExecutedUnreportedCacheWriteAttempts == 0 {
			merged.CatalogCost = &catalogCost
		} else {
			merged.CatalogCostPartial = &catalogCost
		}
	}
	return merged
}

func SortedToolNames(metrics UsageMetrics) []string {
	names := make([]string, 0, len(metrics.ToolCallsByName))
	for name := range metrics.ToolCallsByName {
		names = append(names, name)
	}
	slices.SortFunc(names, strings.Compare)
	return names
}
