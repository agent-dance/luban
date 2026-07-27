// Package evidenceproxy implements a prompt-blind reverse proxy used only by
// the benchmark harness. It records request shape, server identity, usage, and
// tool-call metadata without persisting credentials, headers, prompts, or raw
// provider responses.
package evidenceproxy

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/agent-dance/luban/benchmark/agentic/cacheevidence"
)

const defaultMaxRequestBytes = 128 << 20

const attemptJournalSuffix = ".attempt-starts.jsonl"

const codex0145ServiceTierOmissionProof = "codex-0.145.0-config-default-wire-omission"

type AttemptRecoveryState string

const (
	AttemptRecoveryZeroEvidence    AttemptRecoveryState = "zero_provider_evidence"
	AttemptRecoveryStartedUnsealed AttemptRecoveryState = "provider_attempt_started_unsealed"
	AttemptRecoverySealed          AttemptRecoveryState = "provider_attempt_sealed"
)

type AttemptStartJournalEntry struct {
	SchemaVersion            string    `json:"schema_version"`
	RunIdentity              string    `json:"run_identity"`
	Round                    int       `json:"round"`
	StartedAt                time.Time `json:"started_at"`
	Transport                string    `json:"transport,omitempty"`
	ProviderAttemptKind      string    `json:"provider_attempt_kind,omitempty"`
	WebSocketConnectionHash  string    `json:"websocket_connection_hash,omitempty"`
	WebSocketRequestSequence uint64    `json:"websocket_request_sequence,omitempty"`
}

type EvidenceSeal struct {
	SchemaVersion         string    `json:"schema_version"`
	RunIdentity           string    `json:"run_identity"`
	StartedAttemptCount   uint64    `json:"started_attempt_count"`
	PersistedAttemptCount uint64    `json:"persisted_attempt_count"`
	RecordCount           uint64    `json:"record_count"`
	LastEvidenceHash      string    `json:"last_evidence_hash,omitempty"`
	Fatal                 bool      `json:"fatal"`
	FatalErrorHash        string    `json:"fatal_error_hash,omitempty"`
	SealedAt              time.Time `json:"sealed_at"`
}

type Config struct {
	ListenAddress                           string
	Upstream                                string
	ApprovedOrigin                          string
	EndpointSemanticsSHA256                 string
	RequireTLSPeerEvidence                  bool
	EvidencePath                            string
	ReadyPath                               string
	AccessPath                              string
	Credential                              string
	ExpectedModel                           string
	ExpectedEffort                          string
	AgentID                                 string
	ClientCanonicalizationStaticProofSHA256 string
	RegisteredBinarySHA256                  string
	FrozenBundleManifestSHA256              string
	FrozenBundleTreeSHA256                  string
	FrozenCanonicalCanaryReceiptSHA256      string
	AdapterSHA256                           string
	AdapterVersion                          string
	SourceCommandArgvSHA256                 string
	RunIdentity                             string
	MaxRequestBytes                         int64
	Transport                               http.RoundTripper
	WebSocketDialer                         WebSocketDialer
}

type Record struct {
	SchemaVersion                            string                   `json:"schema_version"`
	EvidenceSequence                         uint64                   `json:"evidence_sequence"`
	PreviousEvidenceHash                     string                   `json:"previous_evidence_hash,omitempty"`
	EvidenceHash                             string                   `json:"evidence_hash"`
	Round                                    int                      `json:"round"`
	RunIdentity                              string                   `json:"run_identity"`
	ProviderAttemptStarted                   bool                     `json:"provider_attempt_started"`
	Transport                                string                   `json:"transport"`
	ProviderAttemptKind                      string                   `json:"provider_attempt_kind"`
	ApprovedOrigin                           string                   `json:"approved_origin"`
	SemanticsSHA256                          string                   `json:"semantics_sha256"`
	TLSServerName                            string                   `json:"tls_server_name"`
	TLSVerified                              bool                     `json:"tls_verified"`
	TLSObservedAt                            time.Time                `json:"tls_observed_at"`
	TLSPeerLeafCertSHA256                    string                   `json:"tls_peer_leaf_cert_sha256"`
	TLSPeerSPKISHA256                        string                   `json:"tls_peer_spki_sha256"`
	WebSocketConnectionHash                  string                   `json:"websocket_connection_hash,omitempty"`
	WebSocketRequestSequence                 uint64                   `json:"websocket_request_sequence,omitempty"`
	WebSocketConnectionReused                bool                     `json:"websocket_connection_reused"`
	WebSocketHandshakeStatus                 int                      `json:"websocket_handshake_status,omitempty"`
	WebSocketHandshakeModel                  string                   `json:"websocket_handshake_model,omitempty"`
	WebSocketChainBound                      bool                     `json:"websocket_chain_bound"`
	GenerateSpecified                        bool                     `json:"generate_specified"`
	Generate                                 bool                     `json:"generate"`
	StartedAt                                time.Time                `json:"started_at"`
	UpstreamHeadersAt                        time.Time                `json:"upstream_headers_at,omitempty"`
	FirstResponseByteAt                      time.Time                `json:"first_response_byte_at,omitempty"`
	FinishedAt                               time.Time                `json:"finished_at"`
	Method                                   string                   `json:"method"`
	Path                                     string                   `json:"path"`
	RequestBytes                             int64                    `json:"request_bytes"`
	ResponseBytes                            int64                    `json:"response_bytes"`
	RequestedModel                           string                   `json:"requested_model"`
	RequestedReasoningEffort                 string                   `json:"requested_reasoning_effort"`
	RequestedReasoningContext                string                   `json:"requested_reasoning_context"`
	RequestedReasoningMode                   string                   `json:"requested_reasoning_mode,omitempty"`
	RequestedReasoningModeCanonical          string                   `json:"requested_reasoning_mode_canonical"`
	RequestedTextVerbosity                   string                   `json:"requested_text_verbosity,omitempty"`
	MaxOutputTokensSpecified                 bool                     `json:"max_output_tokens_specified"`
	MaxOutputTokens                          *int64                   `json:"max_output_tokens,omitempty"`
	RequestedServiceTier                     string                   `json:"requested_service_tier,omitempty"`
	RequestedServiceTierPresent              bool                     `json:"requested_service_tier_present"`
	RequestedServiceTierCanonical            string                   `json:"requested_service_tier_canonical"`
	RequestedServiceTierRepresentation       string                   `json:"requested_service_tier_representation"`
	ClientCanonicalizationStaticProofSHA256  string                   `json:"client_canonicalization_static_proof_sha256,omitempty"`
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
	ToolDefinitionCount                      int                      `json:"tool_definition_count"`
	ToolDefinitions                          []ToolDefinitionEvidence `json:"tool_definitions,omitempty"`
	ToolCatalogHash                          string                   `json:"tool_catalog_hash,omitempty"`
	ToolCatalogSemanticSHA256                string                   `json:"tool_catalog_semantic_sha256"`
	ToolCatalogCanonicalBytes                int64                    `json:"tool_catalog_canonical_bytes"`
	ToolCatalogCompared                      bool                     `json:"tool_catalog_compared"`
	ToolCatalogStable                        bool                     `json:"tool_catalog_stable"`
	ToolResultHistoryValid                   bool                     `json:"tool_result_history_valid"`
	HTTPStatus                               int                      `json:"http_status"`
	UpstreamRequestIDHash                    string                   `json:"upstream_request_id_hash,omitempty"`
	ResponseIDHash                           string                   `json:"response_id_hash,omitempty"`
	ResponseCreatedModel                     string                   `json:"response_created_model,omitempty"`
	ResponseModel                            string                   `json:"response_model,omitempty"`
	ResponseServiceTier                      string                   `json:"response_service_tier,omitempty"`
	ResponseServiceTierCanonical             string                   `json:"response_service_tier_canonical"`
	ServiceTierComparable                    bool                     `json:"service_tier_comparable"`
	ResponseCompleted                        bool                     `json:"response_completed"`
	ResponseStatus                           string                   `json:"response_status,omitempty"`
	// ResponseFailureCode and ResponseFailureEventSHA256 are a content-free,
	// hash-chain-sealed projection of response.failed. The digest covers the
	// exact decoded provider-event JSON bytes (after SSE data framing removal)
	// and never any CLI stderr or human-readable error message.
	ResponseFailureCode        string `json:"response_failure_code,omitempty"`
	ResponseFailureEventSHA256 string `json:"response_failure_event_sha256,omitempty"`
	// UsagePresent is an atomic provider receipt. It is true only when the
	// provider explicitly supplied all three billing fields in one usage object;
	// a missing field is never projected as a synthetic zero.
	UsagePresent          bool         `json:"usage_present"`
	InputTokens           *int64       `json:"input_tokens,omitempty"`
	CachedInputTokens     *int64       `json:"cached_input_tokens,omitempty"`
	CacheWriteInputTokens *int64       `json:"cache_write_input_tokens,omitempty"`
	OutputTokens          *int64       `json:"output_tokens,omitempty"`
	ReasoningOutputTokens *int64       `json:"reasoning_output_tokens,omitempty"`
	ToolCalls             []ToolCall   `json:"tool_calls,omitempty"`
	ToolResults           []ToolResult `json:"tool_results,omitempty"`
	ProtocolValid         bool         `json:"protocol_valid"`
	ErrorCode             string       `json:"error_code,omitempty"`
	Disposition           string       `json:"disposition,omitempty"`
}

type ToolCall struct {
	IDHash string `json:"id_hash"`
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	// InputBytes is the UTF-8 size for a string wire payload and canonical
	// JSON size for a structured payload (for example shell.action or
	// apply_patch.operation). It never includes the surrounding call envelope.
	InputBytes int64 `json:"input_bytes"`
}

// ToolDefinitionEvidence is an ordered, content-free projection of one tool
// exposed to the model. SchemaHash is a per-handler HMAC used for within-run
// replay binding. The SHA-256 fields intentionally remain stable across runs
// so two agents' native catalogs can be compared without persisting schemas,
// descriptions, grammars, or specialized-tool configuration.
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

// ToolResult is a content-free projection of a result sent back to the
// provider. OutputBytes is the exact JSON/string payload size visible to the
// provider; the result itself never leaves request memory.
type ToolResult struct {
	IDHash      string `json:"id_hash"`
	Kind        string `json:"kind"`
	PayloadHash string `json:"payload_hash"`
	OutputBytes int64  `json:"output_bytes"`
}

type requestMetadata struct {
	Model                          string
	ReasoningEffort                string
	ReasoningContext               string
	ReasoningMode                  string
	ReasoningModeTypeValid         bool
	TextVerbosity                  string
	MaxOutputTokensSpecified       bool
	MaxOutputTokens                *int64
	ServiceTier                    string
	ServiceTierPresent             bool
	ServiceTierTypeValid           bool
	StoreSpecified                 bool
	StoreTypeValid                 bool
	Store                          bool
	PreviousResponseIDPresent      bool
	PreviousResponseIDHash         string
	PromptCacheKeyPresent          bool
	PromptCacheKeyHash             string
	CachePolicyObserved            bool
	CachePolicyValid               bool
	PromptCacheOptionsPresent      bool
	PromptCacheOptionsMode         string
	PromptCacheTTLSeconds          *int64
	PromptCacheRetentionPresent    bool
	PromptCacheRetention           string
	CacheBreakpointCount           int
	CacheBreakpointPositionHashes  []string
	EncryptedReasoningRequested    bool
	EncryptedReasoningReplayHashes []string
	ReplayOutputItemHashes         []string
	ToolDefinitions                []ToolDefinitionEvidence
	ToolCatalogHash                string
	ToolCatalogSemanticSHA256      string
	ToolCatalogCanonicalBytes      int64
	ToolDefinitionsValid           bool
	ToolResults                    []ToolResult
	ToolResultsValid               bool
}

type continuationLineageState struct {
	epoch              uint64
	model              string
	outputItems        []string
	reasoningItems     []string
	catalogHash        string
	toolResultBindings map[string]string
	toolResultOrder    []string
	toolCallKeys       map[string]struct{}
	commitVersion      uint64
}

type continuationReplayPlan struct {
	lineageHash        string
	epoch              uint64
	baseOutputItems    []string
	baseReasoningItems []string
	priorVersion       uint64
	newLineage         bool
	resetAccepted      bool
	resetUnknown       bool
	catalogCompared    bool
	catalogStable      bool
	catalogHash        string
	toolResultBindings map[string]string
	toolResultOrder    []string
	toolCallKeys       map[string]struct{}
}

type Handler struct {
	target                      *url.URL
	approvedOrigin              string
	semanticsSHA256             string
	tlsServerName               string
	requireTLSPeer              bool
	evidencePath                string
	accessPath                  string
	credential                  string
	expectedModel               string
	expectedEffort              string
	agentID                     string
	canonicalizationStaticProof string
	runIdentity                 string
	bindingKey                  []byte
	defaultLineage              string
	maxRequestBytes             int64
	transport                   http.RoundTripper
	webSocketDialer             WebSocketDialer
	nextRound                   atomic.Int64
	writeMu                     sync.Mutex
	lineageMu                   sync.Mutex
	lineages                    map[string]continuationLineageState
	persistenceMu               sync.Mutex
	persistenceErr              error
	startedAttempts             uint64
	persistedAttempts           uint64
	recordCount                 uint64
	lastEvidenceHash            string
	sealed                      bool
	webSocketMu                 sync.Mutex
	webSocketRelays             map[*webSocketRelay]struct{}
	webSocketWG                 sync.WaitGroup
}

type serviceTierCanonicalizationStaticEnvelope struct {
	SchemaVersion                      string `json:"schema_version"`
	Representation                     string `json:"representation"`
	CanonicalizationRule               string `json:"canonicalization_rule"`
	ClientAgentID                      string `json:"client_agent_id"`
	RegisteredBinarySHA256             string `json:"registered_binary_sha256"`
	FrozenBundleManifestSHA256         string `json:"frozen_bundle_manifest_sha256"`
	FrozenBundleTreeSHA256             string `json:"frozen_bundle_tree_sha256"`
	FrozenCanonicalCanaryReceiptSHA256 string `json:"frozen_canonical_canary_receipt_sha256"`
	AdapterSHA256                      string `json:"adapter_sha256"`
	AdapterVersion                     string `json:"adapter_version"`
	SourceCommandArgvSHA256            string `json:"source_command_argv_sha256"`
}

// ServiceTierCanonicalizationStaticProof returns the content-addressed proof
// for the frozen Codex client/configuration that omits its configured default
// service tier on the wire. The actual effective argv does not exist yet at
// this pre-run boundary; the final controller receipt binds this static proof
// to that later runtime evidence.
func ServiceTierCanonicalizationStaticProof(config Config) (string, error) {
	if config.AgentID != "codex" {
		return "", nil
	}
	for _, digest := range []string{
		config.RegisteredBinarySHA256,
		config.FrozenBundleManifestSHA256,
		config.FrozenBundleTreeSHA256,
		config.FrozenCanonicalCanaryReceiptSHA256,
		config.AdapterSHA256,
		config.SourceCommandArgvSHA256,
	} {
		if !isLowerHex64(digest) {
			return "", errors.New("Codex service-tier canonicalization proof inputs are invalid")
		}
	}
	if config.AdapterVersion == "" || strings.TrimSpace(config.AdapterVersion) != config.AdapterVersion {
		return "", errors.New("Codex service-tier canonicalization proof inputs are invalid")
	}
	envelope := serviceTierCanonicalizationStaticEnvelope{
		SchemaVersion: "service-tier-canonicalization-static-v2", Representation: "client_canonicalized_default",
		CanonicalizationRule: codex0145ServiceTierOmissionProof, ClientAgentID: "codex",
		RegisteredBinarySHA256:             config.RegisteredBinarySHA256,
		FrozenBundleManifestSHA256:         config.FrozenBundleManifestSHA256,
		FrozenBundleTreeSHA256:             config.FrozenBundleTreeSHA256,
		FrozenCanonicalCanaryReceiptSHA256: config.FrozenCanonicalCanaryReceiptSHA256,
		AdapterSHA256:                      config.AdapterSHA256, AdapterVersion: config.AdapterVersion,
		SourceCommandArgvSHA256: config.SourceCommandArgvSHA256,
	}
	canonical, err := json.Marshal(envelope)
	if err != nil {
		return "", err
	}
	return stableSHA256Bytes(canonical), nil
}

func NewHandler(config Config) (*Handler, error) {
	target, err := url.Parse(config.Upstream)
	if err != nil || target.Scheme != "https" || target.Host == "" {
		return nil, errors.New("evidence proxy upstream must be an HTTPS origin")
	}
	targetOrigin := (&url.URL{Scheme: target.Scheme, Host: target.Host}).String()
	approvedOrigin := config.ApprovedOrigin
	if approvedOrigin == "" {
		approvedOrigin = targetOrigin
	}
	approved, approvedErr := url.Parse(approvedOrigin)
	if approvedErr != nil || approved.Scheme != "https" || approved.Host == "" || approved.User != nil ||
		approved.Path != "" || approved.RawPath != "" || approved.RawQuery != "" || approved.Fragment != "" || approved.Opaque != "" ||
		approved.String() != approvedOrigin || target.User != nil || target.Path != "" || target.RawPath != "" ||
		target.RawQuery != "" || target.Fragment != "" || target.Opaque != "" || approvedOrigin != targetOrigin {
		return nil, errors.New("evidence proxy upstream must be an HTTPS origin")
	}
	if config.EndpointSemanticsSHA256 != "" && (!isHex64(config.EndpointSemanticsSHA256) || config.EndpointSemanticsSHA256 != strings.ToLower(config.EndpointSemanticsSHA256)) {
		return nil, errors.New("evidence proxy endpoint semantics must be a lowercase SHA-256")
	}
	if config.RequireTLSPeerEvidence && (config.ApprovedOrigin == "" || config.EndpointSemanticsSHA256 == "") {
		return nil, errors.New("required TLS peer evidence lacks approved endpoint identity")
	}
	if config.EvidencePath == "" || config.Credential == "" || config.ExpectedModel == "" || config.ExpectedEffort == "" || !isHex64(config.RunIdentity) {
		return nil, errors.New("evidence path, credential, expected model, and expected effort are required")
	}
	if config.AgentID != "" && config.AgentID != "codex" && config.AgentID != "luban" {
		return nil, errors.New("evidence proxy agent identity is invalid")
	}
	canonicalizationStaticProof, err := ServiceTierCanonicalizationStaticProof(config)
	if err != nil {
		return nil, err
	}
	if config.AgentID == "codex" {
		if !isLowerHex64(config.ClientCanonicalizationStaticProofSHA256) || config.ClientCanonicalizationStaticProofSHA256 != canonicalizationStaticProof {
			return nil, errors.New("Codex service-tier canonicalization static proof is missing or does not match its frozen inputs")
		}
		canonicalizationStaticProof = config.ClientCanonicalizationStaticProofSHA256
	} else if config.ClientCanonicalizationStaticProofSHA256 != "" && !isLowerHex64(config.ClientCanonicalizationStaticProofSHA256) {
		return nil, errors.New("service-tier canonicalization static proof must be a lowercase SHA-256")
	}
	accessPath := "/" + strings.Trim(strings.TrimSpace(config.AccessPath), "/")
	if len(accessPath) < 33 {
		return nil, errors.New("evidence proxy access path must contain at least 32 unguessable characters")
	}
	if config.MaxRequestBytes <= 0 {
		config.MaxRequestBytes = defaultMaxRequestBytes
	}
	transport := config.Transport
	if transport == nil {
		base, ok := http.DefaultTransport.(*http.Transport)
		if !ok {
			return nil, errors.New("default HTTP transport is unavailable")
		}
		owned := base.Clone()
		owned.Proxy = nil
		tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: approved.Hostname()}
		if owned.TLSClientConfig != nil {
			tlsConfig = owned.TLSClientConfig.Clone()
			if tlsConfig.MinVersion == 0 || tlsConfig.MinVersion < tls.VersionTLS12 {
				tlsConfig.MinVersion = tls.VersionTLS12
			}
			tlsConfig.ServerName = approved.Hostname()
		}
		owned.TLSClientConfig = tlsConfig
		transport = owned
	}
	bindingKey := make([]byte, sha256.Size)
	if _, err := rand.Read(bindingKey); err != nil {
		return nil, err
	}
	handler := &Handler{
		target: target, approvedOrigin: approvedOrigin, semanticsSHA256: config.EndpointSemanticsSHA256,
		tlsServerName: approved.Hostname(), requireTLSPeer: config.RequireTLSPeerEvidence,
		evidencePath: config.EvidencePath, accessPath: accessPath,
		credential: config.Credential, expectedModel: config.ExpectedModel,
		expectedEffort: config.ExpectedEffort, agentID: config.AgentID, canonicalizationStaticProof: canonicalizationStaticProof, runIdentity: strings.ToLower(config.RunIdentity),
		maxRequestBytes: config.MaxRequestBytes, transport: transport, webSocketDialer: config.WebSocketDialer, bindingKey: bindingKey,
		lineages: make(map[string]continuationLineageState), webSocketRelays: make(map[*webSocketRelay]struct{}),
	}
	handler.defaultLineage = handler.hashBindingValue("controller-default-lineage:" + handler.runIdentity)
	return handler, nil
}

type tlsPeerEvidence struct {
	ApprovedOrigin     string
	SemanticsSHA256    string
	TLSServerName      string
	TLSVerified        bool
	TLSObservedAt      time.Time
	PeerLeafCertSHA256 string
	PeerSPKISHA256     string
}

func (handler *Handler) projectTLSPeerEvidence(state *tls.ConnectionState) (tlsPeerEvidence, error) {
	base := tlsPeerEvidence{ApprovedOrigin: handler.approvedOrigin, SemanticsSHA256: handler.semanticsSHA256}
	if state == nil {
		if handler.requireTLSPeer {
			return base, errors.New("TLS peer connection state is missing")
		}
		return base, nil
	}
	invalid := !state.HandshakeComplete || state.Version < tls.VersionTLS12 || len(state.PeerCertificates) == 0 || len(state.VerifiedChains) == 0 ||
		state.ServerName != handler.tlsServerName
	if !invalid {
		leaf := state.PeerCertificates[0]
		if leaf == nil || len(leaf.Raw) == 0 || len(leaf.RawSubjectPublicKeyInfo) == 0 || leaf.VerifyHostname(handler.tlsServerName) != nil {
			invalid = true
		} else {
			for _, chain := range state.VerifiedChains {
				if len(chain) == 0 || chain[0] == nil || !bytes.Equal(chain[0].Raw, leaf.Raw) {
					invalid = true
					break
				}
			}
			if !invalid {
				leafDigest := sha256.Sum256(leaf.Raw)
				spkiDigest := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
				base.TLSServerName = state.ServerName
				base.TLSVerified = true
				base.TLSObservedAt = time.Now().UTC()
				base.PeerLeafCertSHA256 = hex.EncodeToString(leafDigest[:])
				base.PeerSPKISHA256 = hex.EncodeToString(spkiDigest[:])
				return base, nil
			}
		}
	}
	if handler.requireTLSPeer {
		return base, errors.New("TLS peer connection state is not verified for the approved origin")
	}
	return base, nil
}

func applyTLSPeerEvidence(record *Record, evidence tlsPeerEvidence) {
	record.ApprovedOrigin = evidence.ApprovedOrigin
	record.SemanticsSHA256 = evidence.SemanticsSHA256
	record.TLSServerName = evidence.TLSServerName
	record.TLSVerified = evidence.TLSVerified
	record.TLSObservedAt = evidence.TLSObservedAt
	record.TLSPeerLeafCertSHA256 = evidence.PeerLeafCertSHA256
	record.TLSPeerSPKISHA256 = evidence.PeerSPKISHA256
}

func (handler *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Path == "/healthz" {
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	providerPath, ok := strings.CutPrefix(request.URL.Path, handler.accessPath)
	if ok && providerPath == "/v1/responses" && isWebSocketUpgrade(request) {
		handler.serveWebSocket(writer, request, providerPath)
		return
	}
	if !ok || request.Method != http.MethodPost || providerPath != "/v1/responses" {
		http.NotFound(writer, request)
		return
	}
	round := int(handler.nextRound.Add(1) - 1)
	record := Record{
		SchemaVersion: "agentic-bench/provider-http-v6", Round: round, RunIdentity: handler.runIdentity,
		Transport: "http_sse", ProviderAttemptKind: "inference", WebSocketChainBound: true,
		ApprovedOrigin: handler.approvedOrigin, SemanticsSHA256: handler.semanticsSHA256,
		StartedAt: time.Now().UTC(), Method: request.Method, Path: providerPath,
	}
	body, err := readLimited(request.Body, handler.maxRequestBytes)
	if err != nil {
		record.ErrorCode = "request_too_large"
		record.Disposition = "experiment_invalid"
		record.FinishedAt = time.Now().UTC()
		if !handler.appendBeforeUpstream(writer, record) {
			return
		}
		// i18n:allow display-literal protocol -- Stable private proxy wire error; it is consumed by the benchmark agent transport, not rendered by Luban.
		http.Error(writer, "request body exceeds benchmark proxy limit", http.StatusRequestEntityTooLarge)
		return
	}
	record.RequestBytes = int64(len(body))
	forwardedBody, transformationEvidence, err := handler.transformServiceTierRequest(body)
	applyServiceTierRequestTransformationEvidence(&record, transformationEvidence)
	if err != nil {
		record.ErrorCode = "request_body_transformation_invalid"
		record.Disposition = "experiment_invalid"
		record.FinishedAt = time.Now().UTC()
		if !handler.appendBeforeUpstream(writer, record) {
			zero(body)
			zero(forwardedBody)
			return
		}
		zero(body)
		zero(forwardedBody)
		// i18n:allow display-literal protocol -- Stable private proxy wire error; it is consumed by the benchmark agent transport, not rendered by Luban.
		http.Error(writer, "request body transformation cannot be audited", http.StatusBadRequest)
		return
	}
	defer zero(body)
	defer zero(forwardedBody)
	metadata := inspectRequest(body, handler.hashBindingValue)
	record.RequestedModel = metadata.Model
	record.RequestedReasoningEffort = metadata.ReasoningEffort
	record.RequestedReasoningContext = metadata.ReasoningContext
	record.RequestedReasoningMode = metadata.ReasoningMode
	record.RequestedReasoningModeCanonical = canonicalReasoningMode(metadata.ReasoningMode)
	if !metadata.ReasoningModeTypeValid {
		record.RequestedReasoningModeCanonical = "invalid"
	}
	record.RequestedTextVerbosity = metadata.TextVerbosity
	record.MaxOutputTokensSpecified = metadata.MaxOutputTokensSpecified
	record.MaxOutputTokens = metadata.MaxOutputTokens
	record.RequestedServiceTier = metadata.ServiceTier
	record.RequestedServiceTierPresent = metadata.ServiceTierPresent
	record.ClientAgentID = handler.agentID
	serviceTierCanonical, serviceTierRepresentation, serviceTierProof, serviceTierRequestValid := serviceTierRequestEvidence(metadata, handler.agentID, handler.canonicalizationStaticProof)
	serviceTierRequestValid = serviceTierRequestValid &&
		metadata.ServiceTierPresent == transformationEvidence.OriginalServiceTierPresent &&
		metadata.ServiceTier == transformationEvidence.OriginalServiceTier &&
		transformationEvidence.ExactDiff && isLowerHex64(transformationEvidence.ProofSHA256)
	record.RequestedServiceTierCanonical = serviceTierCanonical
	record.RequestedServiceTierRepresentation = serviceTierRepresentation
	record.ClientCanonicalizationStaticProofSHA256 = serviceTierProof
	record.StoreSpecified = metadata.StoreSpecified
	record.Store = metadata.Store
	record.PreviousResponseIDPresent = metadata.PreviousResponseIDPresent
	record.PreviousResponseIDHash = metadata.PreviousResponseIDHash
	record.PromptCacheKeyPresent = metadata.PromptCacheKeyPresent
	record.PromptCacheKeyHash = metadata.PromptCacheKeyHash
	record.CachePolicyObserved = metadata.CachePolicyObserved
	record.PromptCacheOptionsPresent = metadata.PromptCacheOptionsPresent
	record.PromptCacheOptionsMode = metadata.PromptCacheOptionsMode
	record.PromptCacheTTLSeconds = metadata.PromptCacheTTLSeconds
	record.PromptCacheRetentionPresent = metadata.PromptCacheRetentionPresent
	record.PromptCacheRetention = metadata.PromptCacheRetention
	record.CacheBreakpointCount = metadata.CacheBreakpointCount
	record.CacheBreakpointPositionHashes = append([]string(nil), metadata.CacheBreakpointPositionHashes...)
	record.EncryptedReasoningRequested = metadata.EncryptedReasoningRequested
	record.EncryptedReasoningReplayHashes = append([]string(nil), metadata.EncryptedReasoningReplayHashes...)
	record.EncryptedReasoningReplayCount = len(record.EncryptedReasoningReplayHashes)
	record.ReplayOutputItemHashes = append([]string(nil), metadata.ReplayOutputItemHashes...)
	record.ReplayOutputItemCount = len(record.ReplayOutputItemHashes)
	record.ToolDefinitions = append([]ToolDefinitionEvidence(nil), metadata.ToolDefinitions...)
	record.ToolDefinitionCount = len(record.ToolDefinitions)
	record.ToolCatalogHash = metadata.ToolCatalogHash
	record.ToolCatalogSemanticSHA256 = metadata.ToolCatalogSemanticSHA256
	record.ToolCatalogCanonicalBytes = metadata.ToolCatalogCanonicalBytes
	record.ToolResults = metadata.ToolResults
	lineage, lineagePresent, lineageValid := inspectContinuationLineageHeaders(request.Header)
	planLineage := handler.defaultLineage
	planEpoch := uint64(1)
	strictLineage := false
	record.ContinuationLineageSource = "controller_default"
	if lineagePresent {
		record.ContinuationLineageSource = "agent_header"
		record.ContinuationLineagePresent = true
		if lineageValid {
			record.ContinuationLineageHash = handler.hashBindingValue(lineage.value)
			record.ContinuationEpoch = lineage.epoch
			record.ContinuationReset = lineage.reset
			planLineage = record.ContinuationLineageHash
			planEpoch = record.ContinuationEpoch
			strictLineage = true
		}
	}
	replayPlan, replayValid := handler.planContinuationReplay(
		planLineage, planEpoch, record.ContinuationReset, strictLineage,
		record.RequestedModel, record.ReplayOutputItemHashes, record.EncryptedReasoningReplayHashes, record.ToolResults, record.ToolCatalogHash,
	)
	replayValid = replayValid && metadata.ToolResultsValid
	record.ContinuationResetAccepted = replayPlan.resetAccepted
	record.ContinuationResetUnknown = replayPlan.resetUnknown
	record.ToolCatalogCompared = replayPlan.catalogCompared
	record.ToolCatalogStable = replayPlan.catalogStable
	record.ReplayOutputItemsBound = replayValid && len(record.ReplayOutputItemHashes) > 0
	record.EncryptedReasoningReplayBound = replayValid && len(record.EncryptedReasoningReplayHashes) > 0
	record.ToolResultHistoryValid = replayValid
	if !metadata.ToolDefinitionsValid {
		record.ErrorCode = "tool_catalog_uninspectable"
		record.Disposition = "experiment_invalid"
		record.FinishedAt = time.Now().UTC()
		if !handler.appendBeforeUpstream(writer, record) {
			return
		}
		zero(body)
		// i18n:allow display-literal protocol -- Stable private proxy wire error; it is consumed by the benchmark agent transport, not rendered by Luban.
		http.Error(writer, "request tool catalog cannot be audited", http.StatusBadRequest)
		return
	}
	if !metadata.CachePolicyValid {
		record.ErrorCode = "cache_policy_uninspectable"
		record.Disposition = "experiment_invalid"
		record.FinishedAt = time.Now().UTC()
		if !handler.appendBeforeUpstream(writer, record) {
			return
		}
		zero(body)
		// i18n:allow display-literal protocol -- Stable private proxy wire error; it is consumed by the benchmark agent transport, not rendered by Luban.
		http.Error(writer, "request cache policy cannot be audited", http.StatusBadRequest)
		return
	}
	if record.RequestedModel != handler.expectedModel || record.RequestedReasoningEffort != handler.expectedEffort {
		record.ErrorCode = "pinned_request_mismatch"
		record.Disposition = "experiment_invalid"
		record.FinishedAt = time.Now().UTC()
		if !handler.appendBeforeUpstream(writer, record) {
			return
		}
		zero(body)
		// i18n:allow display-literal protocol -- Stable private proxy wire error; it is consumed by the benchmark agent transport, not rendered by Luban.
		http.Error(writer, "request does not match benchmark model pin", http.StatusBadRequest)
		return
	}
	if !metadata.ReasoningModeTypeValid || record.RequestedReasoningModeCanonical != "standard" {
		record.ErrorCode = "reasoning_mode_not_comparable"
		record.Disposition = "experiment_invalid"
		record.FinishedAt = time.Now().UTC()
		if !handler.appendBeforeUpstream(writer, record) {
			return
		}
		zero(body)
		// i18n:allow display-literal protocol -- Stable private proxy wire error; it is consumed by the benchmark agent transport, not rendered by Luban.
		http.Error(writer, "request reasoning mode is outside the benchmark contract", http.StatusBadRequest)
		return
	}
	if !serviceTierRequestValid {
		record.ErrorCode = "service_tier_request_not_comparable"
		record.Disposition = "experiment_invalid"
		record.FinishedAt = time.Now().UTC()
		if !handler.appendBeforeUpstream(writer, record) {
			return
		}
		zero(body)
		// i18n:allow display-literal protocol -- Stable private proxy wire error; it is consumed by the benchmark agent transport, not rendered by Luban.
		http.Error(writer, "request must explicitly select the default service tier", http.StatusBadRequest)
		return
	}
	if !record.StoreSpecified || !metadata.StoreTypeValid || record.Store {
		record.ErrorCode = "response_storage_not_disabled"
		record.Disposition = "experiment_invalid"
		record.FinishedAt = time.Now().UTC()
		if !handler.appendBeforeUpstream(writer, record) {
			return
		}
		zero(body)
		// i18n:allow display-literal protocol -- Stable private proxy wire error; it is consumed by the benchmark agent transport, not rendered by Luban.
		http.Error(writer, "request must explicitly disable provider storage", http.StatusBadRequest)
		return
	}
	if record.PreviousResponseIDPresent {
		record.ErrorCode = "previous_response_id_forbidden_stateless"
		record.Disposition = "experiment_invalid"
		record.FinishedAt = time.Now().UTC()
		if !handler.appendBeforeUpstream(writer, record) {
			return
		}
		zero(body)
		// i18n:allow display-literal protocol -- Stable private proxy wire error; it is consumed by the benchmark agent transport, not rendered by Luban.
		http.Error(writer, "stateless benchmark requests must replay full history", http.StatusBadRequest)
		return
	}
	if !lineageValid {
		record.ErrorCode = "continuation_lineage_invalid"
		record.Disposition = "experiment_invalid"
		record.FinishedAt = time.Now().UTC()
		if !handler.appendBeforeUpstream(writer, record) {
			return
		}
		zero(body)
		// i18n:allow display-literal protocol -- Stable private proxy wire error; it is consumed by the benchmark agent transport, not rendered by Luban.
		http.Error(writer, "stateless continuation lineage is invalid", http.StatusBadRequest)
		return
	}
	if !replayValid {
		record.ErrorCode = "unbound_stateless_output_replay"
		record.Disposition = "experiment_invalid"
		record.FinishedAt = time.Now().UTC()
		if !handler.appendBeforeUpstream(writer, record) {
			return
		}
		zero(body)
		// i18n:allow display-literal protocol -- Stable private proxy wire error; it is consumed by the benchmark agent transport, not rendered by Luban.
		http.Error(writer, "stateless output replay is not bound to an earlier response", http.StatusBadRequest)
		return
	}
	if err := handler.appendAttemptStart(AttemptStartJournalEntry{
		SchemaVersion:       "agentic-bench/provider-attempt-start-v1",
		RunIdentity:         handler.runIdentity,
		Round:               round,
		StartedAt:           record.StartedAt,
		Transport:           record.Transport,
		ProviderAttemptKind: record.ProviderAttemptKind,
	}); err != nil {
		handler.recordPersistenceError(err)
		zero(body)
		// i18n:allow display-literal protocol -- Stable private proxy wire error; it is consumed by the benchmark controller, not rendered by Luban.
		http.Error(writer, "benchmark evidence journal unavailable", http.StatusInternalServerError)
		return
	}
	record.ProviderAttemptStarted = true

	upstreamRequest := request.Clone(request.Context())
	upstreamRequest.URL.Scheme = handler.target.Scheme
	upstreamRequest.URL.Host = handler.target.Host
	upstreamRequest.URL.Path = joinURLPath(handler.target.Path, providerPath)
	upstreamRequest.URL.RawPath = ""
	upstreamRequest.RequestURI = ""
	upstreamRequest.Host = handler.target.Host
	upstreamRequest.Body = io.NopCloser(bytes.NewReader(forwardedBody))
	upstreamRequest.ContentLength = int64(len(forwardedBody))
	removeHopHeaders(upstreamRequest.Header)
	removeContinuationHeaders(upstreamRequest.Header)
	upstreamRequest.Header.Del("Content-Length")
	upstreamRequest.Header.Set("Authorization", "Bearer "+handler.credential)

	response, err := handler.transport.RoundTrip(upstreamRequest)
	upstreamRequest.Header.Del("Authorization")
	zero(body)
	zero(forwardedBody)
	if err != nil || response == nil {
		record.ErrorCode = "upstream_transport"
		record.Disposition = "provider_infra_exclusion"
		record.FinishedAt = time.Now().UTC()
		if !handler.appendBeforeUpstream(writer, record) {
			return
		}
		// i18n:allow display-literal protocol -- Stable private proxy wire error; it is consumed by the benchmark agent transport, not rendered by Luban.
		http.Error(writer, "upstream provider request failed", http.StatusBadGateway)
		return
	}
	defer response.Body.Close()
	tlsEvidence, err := handler.projectTLSPeerEvidence(response.TLS)
	if err != nil {
		record.ErrorCode = "upstream_tls_peer_evidence"
		record.Disposition = "experiment_invalid"
		record.FinishedAt = time.Now().UTC()
		if !handler.appendBeforeUpstream(writer, record) {
			return
		}
		// i18n:allow display-literal protocol -- Stable private proxy wire error; it is consumed by the benchmark agent transport, not rendered by Luban.
		http.Error(writer, "upstream provider request failed", http.StatusBadGateway)
		return
	}
	applyTLSPeerEvidence(&record, tlsEvidence)
	record.UpstreamHeadersAt = time.Now().UTC()
	removeHopHeaders(response.Header)
	copyHeaders(writer.Header(), response.Header)
	record.HTTPStatus = response.StatusCode
	record.UpstreamRequestIDHash = hashOpaque(firstHeader(response.Header, "x-request-id", "request-id", "openai-request-id"))
	writer.WriteHeader(response.StatusCode)

	collector := newStreamCollector(&record, handler.hashBindingValue)
	buffer := make([]byte, 32*1024)
	for {
		count, readErr := response.Body.Read(buffer)
		if count > 0 {
			if record.FirstResponseByteAt.IsZero() {
				record.FirstResponseByteAt = time.Now().UTC()
			}
			chunk := buffer[:count]
			record.ResponseBytes += int64(count)
			collector.Write(chunk)
			if _, err := writer.Write(chunk); err != nil {
				record.ErrorCode = "downstream_write"
				break
			}
			if flusher, ok := writer.(http.Flusher); ok {
				flusher.Flush()
			}
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				record.ErrorCode = "upstream_read"
			}
			break
		}
	}
	collector.Close()
	record.FinishedAt = time.Now().UTC()
	record.ResponseServiceTierCanonical = canonicalServiceTier(record.ResponseServiceTier)
	record.ServiceTierComparable = serviceTierComparable(record)
	if record.ErrorCode == "" && (record.HTTPStatus < 200 || record.HTTPStatus >= 300) {
		record.ErrorCode = "provider_http_error"
	}
	if record.ErrorCode == "" && (!record.ResponseCompleted || record.ResponseStatus != "completed") {
		record.ErrorCode = "provider_response_not_completed"
	}
	if record.ErrorCode == "" && !validAtomicUsageReceipt(record) {
		record.ErrorCode = "provider_usage_receipt_incomplete"
	}
	// A non-empty served model that differs from the pin invalidates the whole
	// experiment regardless of any concurrent HTTP, stream, completion, or
	// usage failure. It must never be downgraded to an infrastructure exclusion.
	if (record.ResponseCreatedModel != "" && record.ResponseCreatedModel != handler.expectedModel) ||
		(record.ResponseModel != "" && record.ResponseModel != handler.expectedModel) {
		record.ErrorCode = "served_model_mismatch"
		record.Disposition = "experiment_invalid"
	}
	if !localToolCallsBoundToCatalog(record.ToolCalls, record.ToolDefinitions) {
		record.ErrorCode = "tool_call_outside_local_catalog"
		record.Disposition = "experiment_invalid"
	}
	if record.ErrorCode == "" && !record.ServiceTierComparable {
		if record.RequestedServiceTierCanonical != record.ResponseServiceTierCanonical {
			record.ErrorCode = "service_tier_mismatch"
		} else {
			record.ErrorCode = "service_tier_not_comparable"
		}
		record.Disposition = "experiment_invalid"
	}
	record.ProtocolValid = record.ErrorCode == "" && (!handler.requireTLSPeer || record.TLSVerified) && record.HTTPStatus >= 200 && record.HTTPStatus < 300 && record.UpstreamRequestIDHash != "" && record.ResponseIDHash != "" && record.ResponseModel == handler.expectedModel && record.ResponseCompleted && record.ResponseStatus == "completed" && validAtomicUsageReceipt(record) && !record.PreviousResponseIDPresent && lineageValid && replayValid
	if record.ProtocolValid && !handler.commitContinuationReplay(replayPlan, record.ResponseOutputItemHashes, record.EncryptedReasoningHashes, record.ToolCalls) {
		record.ProtocolValid = false
		record.ErrorCode = "continuation_lineage_commit_conflict"
	}
	if !record.ProtocolValid && record.ErrorCode == "" {
		record.ErrorCode = "incomplete_server_evidence"
	}
	if record.ProtocolValid {
		record.Disposition = "valid"
	} else if record.Disposition == "" {
		record.Disposition = "provider_infra_exclusion"
	}
	if err := handler.append(record); err != nil {
		handler.recordPersistenceError(err)
	}
}

func (handler *Handler) planContinuationReplay(lineageHash string, epoch uint64, reset, strict bool, model string, outputItems, reasoningItems []string, toolResults []ToolResult, catalogHash string) (continuationReplayPlan, bool) {
	plan := continuationReplayPlan{
		lineageHash: lineageHash, epoch: epoch, catalogHash: catalogHash, catalogStable: true,
		toolResultBindings: make(map[string]string), toolCallKeys: make(map[string]struct{}),
	}
	if lineageHash == "" || epoch == 0 || model == "" {
		return plan, false
	}
	handler.lineageMu.Lock()
	defer handler.lineageMu.Unlock()
	state, exists := handler.lineages[lineageHash]
	if !exists {
		plan.newLineage = true
		return plan, !reset && len(outputItems) == 0 && len(reasoningItems) == 0 && len(toolResults) == 0
	}
	plan.priorVersion = state.commitVersion
	plan.catalogCompared = true
	plan.catalogStable = state.catalogHash == catalogHash
	if state.model != model {
		return plan, false
	}
	bindings, resultOrder, resultValid := validateToolResultHistory(state, toolResults, strict, reset)
	if !resultValid {
		return plan, false
	}
	plan.toolResultBindings = bindings
	plan.toolResultOrder = resultOrder
	for key := range state.toolCallKeys {
		plan.toolCallKeys[key] = struct{}{}
	}
	if !strict {
		if reset || !orderedSubsequence(outputItems, state.outputItems) || !orderedSubsequence(reasoningItems, state.reasoningItems) {
			return plan, false
		}
		// Headerless agents do not declare a compaction epoch. Accept any exact
		// ordered subset as a measured harness strategy, while retaining the
		// controller's complete response ledger so an old/unseen/reordered item
		// can never become authoritative later.
		plan.baseOutputItems = append([]string(nil), state.outputItems...)
		plan.baseReasoningItems = append([]string(nil), state.reasoningItems...)
		plan.resetUnknown = !slices.Equal(outputItems, state.outputItems) || !slices.Equal(reasoningItems, state.reasoningItems)
		return plan, true
	}
	switch {
	case epoch == state.epoch && !reset:
		if !slices.Equal(outputItems, state.outputItems) || !slices.Equal(reasoningItems, state.reasoningItems) {
			return plan, false
		}
		plan.baseOutputItems = append([]string(nil), outputItems...)
		plan.baseReasoningItems = append([]string(nil), reasoningItems...)
		return plan, true
	case epoch > state.epoch && reset:
		if !orderedSubsequence(outputItems, state.outputItems) || !orderedSubsequence(reasoningItems, state.reasoningItems) {
			return plan, false
		}
		plan.baseOutputItems = append([]string(nil), outputItems...)
		plan.baseReasoningItems = append([]string(nil), reasoningItems...)
		plan.resetAccepted = true
		return plan, true
	default:
		return plan, false
	}
}

func validateToolResultHistory(state continuationLineageState, results []ToolResult, strict, reset bool) (map[string]string, []string, bool) {
	bindings := make(map[string]string, len(state.toolResultBindings)+len(results))
	for key, value := range state.toolResultBindings {
		bindings[key] = value
	}
	order := append([]string(nil), state.toolResultOrder...)
	seen := make(map[string]struct{}, len(results))
	knownOrder := make([]string, 0, len(results))
	seenNew := false
	for _, result := range results {
		key := result.Kind + ":" + result.IDHash
		if result.IDHash == "" || result.Kind == "" || result.PayloadHash == "" {
			return nil, nil, false
		}
		if _, duplicate := seen[key]; duplicate {
			return nil, nil, false
		}
		seen[key] = struct{}{}
		if existing, known := bindings[key]; known {
			if seenNew || existing != result.PayloadHash {
				return nil, nil, false
			}
			knownOrder = append(knownOrder, key)
			continue
		}
		callKind := strings.TrimSuffix(result.Kind, "_output")
		if _, authorized := state.toolCallKeys[callKind+":"+result.IDHash]; !authorized {
			return nil, nil, false
		}
		seenNew = true
		bindings[key] = result.PayloadHash
		order = append(order, key)
	}
	if strict && !reset {
		if !slices.Equal(knownOrder, state.toolResultOrder) {
			return nil, nil, false
		}
	} else if !orderedSubsequence(knownOrder, state.toolResultOrder) {
		return nil, nil, false
	}
	return bindings, order, true
}

func (handler *Handler) commitContinuationReplay(plan continuationReplayPlan, responseOutputItems, responseReasoningItems []string, responseToolCalls []ToolCall) bool {
	if plan.lineageHash == "" || plan.epoch == 0 {
		return false
	}
	handler.lineageMu.Lock()
	defer handler.lineageMu.Unlock()
	state, exists := handler.lineages[plan.lineageHash]
	if plan.newLineage {
		if exists {
			return false
		}
		state = continuationLineageState{model: handler.expectedModel}
	} else if !exists || state.commitVersion != plan.priorVersion {
		return false
	}
	state.epoch = plan.epoch
	state.model = handler.expectedModel
	state.outputItems = append(append([]string(nil), plan.baseOutputItems...), responseOutputItems...)
	state.reasoningItems = append(append([]string(nil), plan.baseReasoningItems...), responseReasoningItems...)
	state.catalogHash = plan.catalogHash
	state.toolResultBindings = plan.toolResultBindings
	state.toolResultOrder = append([]string(nil), plan.toolResultOrder...)
	state.toolCallKeys = make(map[string]struct{}, len(plan.toolCallKeys)+len(responseToolCalls))
	for key := range plan.toolCallKeys {
		state.toolCallKeys[key] = struct{}{}
	}
	for _, call := range responseToolCalls {
		state.toolCallKeys[call.Kind+":"+call.IDHash] = struct{}{}
	}
	state.commitVersion++
	handler.lineages[plan.lineageHash] = state
	return true
}

func orderedSubsequence(candidate, source []string) bool {
	if len(candidate) == 0 {
		return true
	}
	position := 0
	for _, value := range source {
		if value == candidate[position] {
			position++
			if position == len(candidate) {
				return true
			}
		}
	}
	return false
}

func (handler *Handler) hashBindingValue(value any) string {
	canonical, err := canonicalBindingValue(value)
	if err != nil || len(canonical) == 0 {
		return ""
	}
	mac := hmac.New(sha256.New, handler.bindingKey)
	_, _ = mac.Write(canonical)
	return hex.EncodeToString(mac.Sum(nil))
}

func canonicalBindingValue(value any) ([]byte, error) {
	switch typed := value.(type) {
	case json.RawMessage:
		var decoded any
		decoder := json.NewDecoder(bytes.NewReader(typed))
		decoder.UseNumber()
		if err := decoder.Decode(&decoded); err != nil {
			return nil, err
		}
		return json.Marshal(decoded)
	case []byte:
		return append([]byte(nil), typed...), nil
	default:
		return json.Marshal(value)
	}
}

const (
	serviceTierTransformationNone          = "none"
	serviceTierTransformationInjectDefault = "inject_explicit_default"
)

type serviceTierRequestTransformationEvidence struct {
	OriginalRequestBodySHA256                string
	ForwardedRequestBodySHA256               string
	OriginalRequestCanonicalSHA256           string
	ForwardedRequestCanonicalSHA256          string
	OriginalRequestWithoutServiceTierSHA256  string
	ForwardedRequestWithoutServiceTierSHA256 string
	OriginalServiceTierPresent               bool
	OriginalServiceTier                      string
	ForwardedServiceTierPresent              bool
	ForwardedServiceTier                     string
	ForwardedRequestBytes                    int64
	Transformation                           string
	ExactDiff                                bool
	ProofSHA256                              string
}

type serviceTierRequestTransformationProofEnvelope struct {
	SchemaVersion                            string `json:"schema_version"`
	ClientAgentID                            string `json:"client_agent_id"`
	ClientCanonicalizationStaticProofSHA256  string `json:"client_canonicalization_static_proof_sha256,omitempty"`
	OriginalRequestBodySHA256                string `json:"original_request_body_sha256"`
	ForwardedRequestBodySHA256               string `json:"forwarded_request_body_sha256"`
	OriginalRequestCanonicalSHA256           string `json:"original_request_canonical_sha256"`
	ForwardedRequestCanonicalSHA256          string `json:"forwarded_request_canonical_sha256"`
	OriginalRequestWithoutServiceTierSHA256  string `json:"original_request_without_service_tier_sha256"`
	ForwardedRequestWithoutServiceTierSHA256 string `json:"forwarded_request_without_service_tier_sha256"`
	OriginalServiceTierPresent               bool   `json:"original_service_tier_present"`
	OriginalServiceTier                      string `json:"original_service_tier,omitempty"`
	ForwardedServiceTierPresent              bool   `json:"forwarded_service_tier_present"`
	ForwardedServiceTier                     string `json:"forwarded_service_tier,omitempty"`
	Transformation                           string `json:"transformation"`
	ExactDiff                                bool   `json:"exact_diff"`
}

func hashServiceTierRequestTransformationProof(envelope serviceTierRequestTransformationProofEnvelope) (string, error) {
	canonical, err := json.Marshal(envelope)
	if err != nil {
		return "", err
	}
	return stableSHA256Bytes(canonical), nil
}

// ServiceTierTransformationProofSHA256 independently reconstructs the
// content-free proof committed by one provider record. It intentionally hashes
// only request-shape evidence: raw request or response content is neither
// required nor exposed.
func ServiceTierTransformationProofSHA256(record Record) (string, error) {
	return hashServiceTierRequestTransformationProof(serviceTierRequestTransformationProofEnvelope{
		SchemaVersion: "service-tier-request-transformation-v1", ClientAgentID: record.ClientAgentID,
		ClientCanonicalizationStaticProofSHA256:  record.ClientCanonicalizationStaticProofSHA256,
		OriginalRequestBodySHA256:                record.OriginalRequestBodySHA256,
		ForwardedRequestBodySHA256:               record.ForwardedRequestBodySHA256,
		OriginalRequestCanonicalSHA256:           record.OriginalRequestCanonicalSHA256,
		ForwardedRequestCanonicalSHA256:          record.ForwardedRequestCanonicalSHA256,
		OriginalRequestWithoutServiceTierSHA256:  record.OriginalRequestWithoutServiceTierSHA256,
		ForwardedRequestWithoutServiceTierSHA256: record.ForwardedRequestWithoutServiceTierSHA256,
		OriginalServiceTierPresent:               record.OriginalServiceTierPresent,
		OriginalServiceTier:                      record.OriginalServiceTier,
		ForwardedServiceTierPresent:              record.ForwardedServiceTierPresent,
		ForwardedServiceTier:                     record.ForwardedServiceTier,
		Transformation:                           record.ServiceTierTransformation,
		ExactDiff:                                record.ServiceTierTransformationExactDiff,
	})
}

// ValidateServiceTierTransformationProof rejects a syntactically valid but
// unbound digest. Consumers must call this before treating a record's claimed
// service-tier transformation as evidence.
func ValidateServiceTierTransformationProof(record Record) error {
	if !isLowerHex64(record.ServiceTierTransformationProofSHA256) {
		return errors.New("service-tier request transformation proof is invalid")
	}
	expected, err := ServiceTierTransformationProofSHA256(record)
	if err != nil {
		return err
	}
	if record.ServiceTierTransformationProofSHA256 != expected {
		return errors.New("service-tier request transformation proof is not bound to the recorded request shape")
	}
	return nil
}

func (handler *Handler) transformServiceTierRequest(body []byte) ([]byte, serviceTierRequestTransformationEvidence, error) {
	evidence := serviceTierRequestTransformationEvidence{
		OriginalRequestBodySHA256: stableSHA256Bytes(body),
		Transformation:            serviceTierTransformationNone,
	}
	decoded, err := decodeUniqueJSONObject(body)
	if err != nil {
		return nil, evidence, err
	}
	originalCanonical, err := json.Marshal(decoded)
	if err != nil {
		return nil, evidence, err
	}
	evidence.OriginalRequestCanonicalSHA256 = stableSHA256Bytes(originalCanonical)
	if raw, present := decoded["service_tier"]; present {
		evidence.OriginalServiceTierPresent = true
		value, ok := raw.(string)
		if !ok {
			return nil, evidence, errors.New("request service tier must be a string")
		}
		evidence.OriginalServiceTier = value
	}

	forwardedObject := cloneJSONObject(decoded)
	forwarded := body
	if handler.agentID == "codex" && !evidence.OriginalServiceTierPresent {
		if !isLowerHex64(handler.canonicalizationStaticProof) {
			return nil, evidence, errors.New("Codex service-tier omission lacks its frozen static proof")
		}
		forwardedObject["service_tier"] = "default"
		forwarded, err = json.Marshal(forwardedObject)
		if err != nil {
			return nil, evidence, err
		}
		evidence.Transformation = serviceTierTransformationInjectDefault
	}
	if raw, present := forwardedObject["service_tier"]; present {
		evidence.ForwardedServiceTierPresent = true
		value, ok := raw.(string)
		if !ok {
			return nil, evidence, errors.New("forwarded request service tier must be a string")
		}
		evidence.ForwardedServiceTier = value
	}
	forwardedCanonical, err := json.Marshal(forwardedObject)
	if err != nil {
		return nil, evidence, err
	}
	evidence.ForwardedRequestBodySHA256 = stableSHA256Bytes(forwarded)
	evidence.ForwardedRequestCanonicalSHA256 = stableSHA256Bytes(forwardedCanonical)
	evidence.ForwardedRequestBytes = int64(len(forwarded))

	originalWithoutTier := cloneJSONObject(decoded)
	delete(originalWithoutTier, "service_tier")
	forwardedWithoutTier := cloneJSONObject(forwardedObject)
	delete(forwardedWithoutTier, "service_tier")
	originalWithoutCanonical, err := json.Marshal(originalWithoutTier)
	if err != nil {
		return nil, evidence, err
	}
	forwardedWithoutCanonical, err := json.Marshal(forwardedWithoutTier)
	if err != nil {
		return nil, evidence, err
	}
	evidence.OriginalRequestWithoutServiceTierSHA256 = stableSHA256Bytes(originalWithoutCanonical)
	evidence.ForwardedRequestWithoutServiceTierSHA256 = stableSHA256Bytes(forwardedWithoutCanonical)
	switch evidence.Transformation {
	case serviceTierTransformationNone:
		evidence.ExactDiff = bytes.Equal(body, forwarded) &&
			evidence.OriginalRequestCanonicalSHA256 == evidence.ForwardedRequestCanonicalSHA256 &&
			evidence.OriginalServiceTierPresent == evidence.ForwardedServiceTierPresent &&
			evidence.OriginalServiceTier == evidence.ForwardedServiceTier
	case serviceTierTransformationInjectDefault:
		evidence.ExactDiff = !evidence.OriginalServiceTierPresent && evidence.OriginalServiceTier == "" &&
			evidence.ForwardedServiceTierPresent && evidence.ForwardedServiceTier == "default" &&
			evidence.OriginalRequestWithoutServiceTierSHA256 == evidence.ForwardedRequestWithoutServiceTierSHA256 &&
			len(forwardedObject) == len(decoded)+1
	}
	if !evidence.ExactDiff {
		return nil, evidence, errors.New("service-tier request transformation is not an exact permitted diff")
	}
	proofEnvelope := serviceTierRequestTransformationProofEnvelope{
		SchemaVersion: "service-tier-request-transformation-v1", ClientAgentID: handler.agentID,
		ClientCanonicalizationStaticProofSHA256:  handler.canonicalizationStaticProof,
		OriginalRequestBodySHA256:                evidence.OriginalRequestBodySHA256,
		ForwardedRequestBodySHA256:               evidence.ForwardedRequestBodySHA256,
		OriginalRequestCanonicalSHA256:           evidence.OriginalRequestCanonicalSHA256,
		ForwardedRequestCanonicalSHA256:          evidence.ForwardedRequestCanonicalSHA256,
		OriginalRequestWithoutServiceTierSHA256:  evidence.OriginalRequestWithoutServiceTierSHA256,
		ForwardedRequestWithoutServiceTierSHA256: evidence.ForwardedRequestWithoutServiceTierSHA256,
		OriginalServiceTierPresent:               evidence.OriginalServiceTierPresent, OriginalServiceTier: evidence.OriginalServiceTier,
		ForwardedServiceTierPresent: evidence.ForwardedServiceTierPresent, ForwardedServiceTier: evidence.ForwardedServiceTier,
		Transformation: evidence.Transformation, ExactDiff: evidence.ExactDiff,
	}
	evidence.ProofSHA256, err = hashServiceTierRequestTransformationProof(proofEnvelope)
	if err != nil {
		return nil, evidence, err
	}
	return forwarded, evidence, nil
}

func applyServiceTierRequestTransformationEvidence(record *Record, evidence serviceTierRequestTransformationEvidence) {
	record.OriginalRequestBodySHA256 = evidence.OriginalRequestBodySHA256
	record.ForwardedRequestBodySHA256 = evidence.ForwardedRequestBodySHA256
	record.OriginalRequestCanonicalSHA256 = evidence.OriginalRequestCanonicalSHA256
	record.ForwardedRequestCanonicalSHA256 = evidence.ForwardedRequestCanonicalSHA256
	record.OriginalRequestWithoutServiceTierSHA256 = evidence.OriginalRequestWithoutServiceTierSHA256
	record.ForwardedRequestWithoutServiceTierSHA256 = evidence.ForwardedRequestWithoutServiceTierSHA256
	record.OriginalServiceTierPresent = evidence.OriginalServiceTierPresent
	record.OriginalServiceTier = evidence.OriginalServiceTier
	record.ForwardedServiceTierPresent = evidence.ForwardedServiceTierPresent
	record.ForwardedServiceTier = evidence.ForwardedServiceTier
	record.ForwardedRequestBytes = evidence.ForwardedRequestBytes
	record.ServiceTierTransformation = evidence.Transformation
	record.ServiceTierTransformationExactDiff = evidence.ExactDiff
	record.ServiceTierTransformationProofSHA256 = evidence.ProofSHA256
}

func cloneJSONObject(source map[string]any) map[string]any {
	result := make(map[string]any, len(source)+1)
	for key, value := range source {
		result[key] = value
	}
	return result
}

func decodeUniqueJSONObject(raw []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	value, err := decodeUniqueJSONValue(decoder)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("request body contains trailing JSON")
		}
		return nil, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("request body must be a JSON object")
	}
	return object, nil
}

func decodeUniqueJSONValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delimiter, composite := token.(json.Delim)
	if !composite {
		return token, nil
	}
	switch delimiter {
	case '{':
		result := make(map[string]any)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, errors.New("JSON object key is not a string")
			}
			if _, duplicate := result[key]; duplicate {
				return nil, errors.New("request body contains a duplicate JSON object key")
			}
			value, err := decodeUniqueJSONValue(decoder)
			if err != nil {
				return nil, err
			}
			result[key] = value
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim('}') {
			return nil, errors.New("request body contains an invalid JSON object")
		}
		return result, nil
	case '[':
		result := make([]any, 0)
		for decoder.More() {
			value, err := decodeUniqueJSONValue(decoder)
			if err != nil {
				return nil, err
			}
			result = append(result, value)
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') {
			return nil, errors.New("request body contains an invalid JSON array")
		}
		return result, nil
	default:
		return nil, errors.New("request body contains an invalid JSON delimiter")
	}
}

func canonicalServiceTier(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "default":
		return "default"
	case "":
		return "omitted"
	case "auto":
		return "auto"
	case "priority":
		return "priority"
	case "flex":
		return "flex"
	default:
		return "unknown"
	}
}

func serviceTierRequestEvidence(metadata requestMetadata, agentID, staticProof string) (canonical, representation, proof string, valid bool) {
	if metadata.ServiceTierPresent {
		if metadata.ServiceTierTypeValid && metadata.ServiceTier == "default" {
			return "default", "explicit_default", "", agentID != "codex"
		}
		return canonicalServiceTier(metadata.ServiceTier), "explicit_nondefault", "", false
	}
	if agentID == "codex" {
		return "default", "client_canonicalized_default", staticProof, isLowerHex64(staticProof)
	}
	return "omitted", "omitted", "", false
}

func serviceTierComparable(record Record) bool {
	responseComparable := record.ResponseServiceTier == "default" && record.ResponseServiceTierCanonical == "default"
	transformationEvidenceValid := record.ForwardedRequestBytes > 0 &&
		isLowerHex64(record.OriginalRequestBodySHA256) && isLowerHex64(record.ForwardedRequestBodySHA256) &&
		isLowerHex64(record.OriginalRequestCanonicalSHA256) && isLowerHex64(record.ForwardedRequestCanonicalSHA256) &&
		isLowerHex64(record.OriginalRequestWithoutServiceTierSHA256) &&
		record.OriginalRequestWithoutServiceTierSHA256 == record.ForwardedRequestWithoutServiceTierSHA256 &&
		ValidateServiceTierTransformationProof(record) == nil
	if !responseComparable || record.RequestedServiceTierCanonical != "default" || !transformationEvidenceValid {
		return false
	}
	switch record.RequestedServiceTierRepresentation {
	case "explicit_default":
		return record.ClientAgentID != "codex" && record.RequestedServiceTierPresent && record.RequestedServiceTier == "default" &&
			record.ClientCanonicalizationStaticProofSHA256 == "" && record.ServiceTierTransformation == serviceTierTransformationNone &&
			record.ServiceTierTransformationExactDiff && record.OriginalServiceTierPresent && record.OriginalServiceTier == "default" &&
			record.ForwardedServiceTierPresent && record.ForwardedServiceTier == "default" &&
			record.OriginalRequestBodySHA256 == record.ForwardedRequestBodySHA256 &&
			record.OriginalRequestCanonicalSHA256 == record.ForwardedRequestCanonicalSHA256
	case "client_canonicalized_default":
		return record.ClientAgentID == "codex" && !record.RequestedServiceTierPresent && record.RequestedServiceTier == "" &&
			isLowerHex64(record.ClientCanonicalizationStaticProofSHA256) && record.ServiceTierTransformation == serviceTierTransformationInjectDefault &&
			record.ServiceTierTransformationExactDiff && !record.OriginalServiceTierPresent && record.OriginalServiceTier == "" &&
			record.ForwardedServiceTierPresent && record.ForwardedServiceTier == "default" &&
			record.OriginalRequestBodySHA256 != record.ForwardedRequestBodySHA256 &&
			record.OriginalRequestCanonicalSHA256 != record.ForwardedRequestCanonicalSHA256
	default:
		return false
	}
}

func canonicalReasoningMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "default", "standard":
		return "standard"
	case "pro":
		return "pro"
	default:
		return "unknown"
	}
}

type continuationLineageHeaders struct {
	value string
	epoch uint64
	reset bool
}

func inspectContinuationLineageHeaders(headers http.Header) (continuationLineageHeaders, bool, bool) {
	values := headers.Values("X-Luban-Stateless-Lineage")
	epochValues := headers.Values("X-Luban-Stateless-Epoch")
	resetValues := headers.Values("X-Luban-Stateless-Reset")
	present := len(values) > 0 || len(epochValues) > 0 || len(resetValues) > 0
	if !present {
		return continuationLineageHeaders{}, false, true
	}
	if len(values) != 1 || len(epochValues) != 1 || len(resetValues) > 1 {
		return continuationLineageHeaders{}, true, false
	}
	value := strings.TrimSpace(values[0])
	if len(value) < 16 || len(value) > 128 {
		return continuationLineageHeaders{}, true, false
	}
	for _, char := range value {
		if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || strings.ContainsRune("-_.:", char)) {
			return continuationLineageHeaders{}, true, false
		}
	}
	epoch, err := strconv.ParseUint(strings.TrimSpace(epochValues[0]), 10, 64)
	if err != nil || epoch == 0 {
		return continuationLineageHeaders{}, true, false
	}
	reset := false
	if len(resetValues) == 1 {
		if strings.TrimSpace(resetValues[0]) != "1" {
			return continuationLineageHeaders{}, true, false
		}
		reset = true
	}
	return continuationLineageHeaders{value: value, epoch: epoch, reset: reset}, true, true
}

func removeContinuationHeaders(headers http.Header) {
	for _, name := range []string{"X-Luban-Stateless-Lineage", "X-Luban-Stateless-Epoch", "X-Luban-Stateless-Reset"} {
		headers.Del(name)
	}
}

func Run(ctx context.Context, config Config) (resultErr error) {
	handler, err := NewHandler(config)
	if err != nil {
		return err
	}
	defer func() {
		handler.shutdownWebSockets()
		handler.webSocketWG.Wait()
		resultErr = errors.Join(resultErr, handler.SealEvidence())
	}()
	listener, err := net.Listen("tcp", config.ListenAddress)
	if err != nil {
		return err
	}
	defer listener.Close()
	if config.ReadyPath != "" {
		if err := os.MkdirAll(filepath.Dir(config.ReadyPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(config.ReadyPath, []byte(listener.Addr().String()+"\n"), 0o600); err != nil {
			return err
		}
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 30 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownContext)
	}()
	err = server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (handler *Handler) append(record Record) error {
	handler.writeMu.Lock()
	defer handler.writeMu.Unlock()
	if handler.PersistenceError() != nil {
		return errors.New("evidence proxy persistence is fatal")
	}
	if handler.sealed {
		return errors.New("evidence proxy is already sealed")
	}
	record.EvidenceSequence = handler.recordCount
	record.PreviousEvidenceHash = handler.lastEvidenceHash
	record.EvidenceHash = ""
	canonical, err := json.Marshal(record)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(canonical)
	record.EvidenceHash = hex.EncodeToString(sum[:])
	encoded, err := json.Marshal(record)
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	if err := appendBytesSync(handler.evidencePath, encoded); err != nil {
		return err
	}
	handler.recordCount++
	handler.lastEvidenceHash = record.EvidenceHash
	if record.ProviderAttemptStarted {
		handler.persistedAttempts++
	}
	return nil
}

func (handler *Handler) appendAttemptStart(entry AttemptStartJournalEntry) error {
	handler.writeMu.Lock()
	defer handler.writeMu.Unlock()
	if handler.PersistenceError() != nil {
		return errors.New("evidence proxy persistence is fatal")
	}
	if handler.sealed {
		return errors.New("evidence proxy is already sealed")
	}
	encoded, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	if err := appendBytesSync(AttemptJournalPath(handler.evidencePath), encoded); err != nil {
		return err
	}
	handler.startedAttempts++
	return nil
}

func appendBytesSync(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	_, statErr := os.Stat(path)
	created := errors.Is(statErr, os.ErrNotExist)
	if statErr != nil && !created {
		return statErr
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	written, writeErr := file.Write(data)
	if writeErr == nil && written != len(data) {
		writeErr = io.ErrShortWrite
	}
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return err
	}
	if !created {
		return nil
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	syncErr := directory.Sync()
	closeErr = directory.Close()
	return errors.Join(syncErr, closeErr)
}

func (handler *Handler) appendBeforeUpstream(writer http.ResponseWriter, record Record) bool {
	if err := handler.append(record); err != nil {
		handler.recordPersistenceError(err)
		// i18n:allow display-literal protocol -- Stable private proxy wire error; it is consumed by the benchmark controller, not rendered by Luban.
		http.Error(writer, "benchmark evidence persistence unavailable", http.StatusInternalServerError)
		return false
	}
	return true
}

func (handler *Handler) recordPersistenceError(err error) {
	if err == nil {
		return
	}
	handler.persistenceMu.Lock()
	if handler.persistenceErr == nil {
		handler.persistenceErr = err
	}
	handler.persistenceMu.Unlock()
}

func (handler *Handler) PersistenceError() error {
	if handler == nil {
		return nil
	}
	handler.persistenceMu.Lock()
	defer handler.persistenceMu.Unlock()
	return handler.persistenceErr
}

func AttemptJournalPath(evidencePath string) string { return evidencePath + attemptJournalSuffix }

func EvidenceSealPath(evidencePath string) string { return evidencePath + ".seal.json" }

func (handler *Handler) SealEvidence() error {
	if handler == nil {
		return errors.New("nil evidence proxy handler")
	}
	handler.writeMu.Lock()
	defer handler.writeMu.Unlock()
	if handler.sealed {
		return handler.PersistenceError()
	}
	persistErr := handler.PersistenceError()
	seal := EvidenceSeal{
		SchemaVersion: "agentic-bench/provider-evidence-seal-v1", RunIdentity: handler.runIdentity,
		StartedAttemptCount: handler.startedAttempts, PersistedAttemptCount: handler.persistedAttempts,
		RecordCount: handler.recordCount, LastEvidenceHash: handler.lastEvidenceHash,
		Fatal:    persistErr != nil || handler.startedAttempts != handler.persistedAttempts,
		SealedAt: time.Now().UTC(),
	}
	if persistErr != nil {
		seal.FatalErrorHash = hashOpaque(persistErr.Error())
	}
	encoded, err := json.Marshal(seal)
	if err == nil {
		encoded = append(encoded, '\n')
		err = writeFileAtomicSync(EvidenceSealPath(handler.evidencePath), encoded)
	}
	if err != nil {
		handler.recordPersistenceError(err)
		return err
	}
	handler.sealed = true
	if seal.Fatal {
		return errors.New("evidence proxy sealed with an incomplete provider attempt")
	}
	return nil
}

func writeFileAtomicSync(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary := path + ".tmp"
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	written, writeErr := file.Write(data)
	if writeErr == nil && written != len(data) {
		writeErr = io.ErrShortWrite
	}
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	syncErr := directory.Sync()
	closeErr = directory.Close()
	return errors.Join(syncErr, closeErr)
}

// InspectAttemptRecoveryState distinguishes a request that provably never
// crossed the provider boundary from one whose durable start journal proves
// that replay could duplicate work or cost. It intentionally does not infer
// safety from a missing final receipt.
func InspectAttemptRecoveryState(evidencePath, runIdentity string, round int) (AttemptRecoveryState, error) {
	journal, err := readJSONLines[AttemptStartJournalEntry](AttemptJournalPath(evidencePath))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	started := false
	for _, entry := range journal {
		if entry.RunIdentity == runIdentity && entry.Round == round {
			started = true
			break
		}
	}
	if !started {
		return AttemptRecoveryZeroEvidence, nil
	}
	records, err := readJSONLines[Record](evidencePath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	for _, record := range records {
		if record.RunIdentity == runIdentity && record.Round == round && record.ProviderAttemptStarted {
			return AttemptRecoverySealed, nil
		}
	}
	return AttemptRecoveryStartedUnsealed, nil
}

func ValidateEvidenceSeal(evidencePath, runIdentity string) (EvidenceSeal, error) {
	var seal EvidenceSeal
	rawSeal, err := os.ReadFile(EvidenceSealPath(evidencePath))
	if err != nil {
		return seal, err
	}
	if err := json.Unmarshal(rawSeal, &seal); err != nil {
		return seal, err
	}
	if seal.SchemaVersion != "agentic-bench/provider-evidence-seal-v1" || seal.RunIdentity != runIdentity || seal.Fatal {
		return seal, errors.New("provider evidence seal is invalid or fatal")
	}
	records, err := readJSONLines[Record](evidencePath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return seal, err
	}
	previous := ""
	persistedAttempts := uint64(0)
	for index, record := range records {
		if record.RunIdentity != runIdentity || record.EvidenceSequence != uint64(index) || record.PreviousEvidenceHash != previous || record.EvidenceHash == "" {
			return seal, errors.New("provider evidence hash chain identity or sequence mismatch")
		}
		claimed := record.EvidenceHash
		record.EvidenceHash = ""
		canonical, err := json.Marshal(record)
		if err != nil {
			return seal, err
		}
		sum := sha256.Sum256(canonical)
		if claimed != hex.EncodeToString(sum[:]) {
			return seal, errors.New("provider evidence hash chain digest mismatch")
		}
		previous = claimed
		if record.ProviderAttemptStarted {
			persistedAttempts++
		}
	}
	journal, err := readJSONLines[AttemptStartJournalEntry](AttemptJournalPath(evidencePath))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return seal, err
	}
	startedAttempts := uint64(0)
	for _, entry := range journal {
		if entry.SchemaVersion != "agentic-bench/provider-attempt-start-v1" || entry.RunIdentity != runIdentity {
			return seal, errors.New("provider attempt journal identity mismatch")
		}
		startedAttempts++
	}
	if seal.RecordCount != uint64(len(records)) || seal.StartedAttemptCount != startedAttempts || seal.PersistedAttemptCount != persistedAttempts || startedAttempts != persistedAttempts || seal.LastEvidenceHash != previous {
		return seal, errors.New("provider evidence seal counts or terminal hash mismatch")
	}
	return seal, nil
}

func readJSONLines[T any](path string) ([]T, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var result []T
	for _, line := range bytes.Split(bytes.TrimSpace(raw), []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var value T
		if err := json.Unmarshal(line, &value); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, nil
}

type streamCollector struct {
	record            *Record
	pending           []byte
	directJSON        []byte
	toolIDs           map[string]struct{}
	reasoningSeen     map[string]struct{}
	doneItemIDs       map[string]struct{}
	doneOutputItems   []outputItemEvidence
	createdResponseID string
	hashValue         func(any) string
}

type outputItemEvidence struct {
	hash          string
	itemID        string
	reasoningHash string
	reasoningKey  string
	toolCall      ToolCall
	isToolCall    bool
}

func newStreamCollector(record *Record, hashValue func(any) string) *streamCollector {
	return &streamCollector{
		record: record, toolIDs: map[string]struct{}{}, reasoningSeen: map[string]struct{}{},
		doneItemIDs: map[string]struct{}{}, hashValue: hashValue,
	}
}

func (collector *streamCollector) Write(chunk []byte) {
	collector.pending = append(collector.pending, chunk...)
	for {
		index := bytes.IndexByte(collector.pending, '\n')
		if index < 0 {
			if len(collector.pending) > 16<<20 {
				collector.record.ErrorCode = "oversized_sse_line"
				collector.pending = collector.pending[:0]
			}
			return
		}
		line := bytes.TrimSpace(collector.pending[:index])
		collector.pending = collector.pending[index+1:]
		if bytes.HasPrefix(line, []byte("data:")) {
			data := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
			if len(data) > 0 && !bytes.Equal(data, []byte("[DONE]")) {
				collector.consume(data)
			}
		} else if len(line) > 0 && len(collector.directJSON) < 16<<20 {
			collector.directJSON = append(collector.directJSON, line...)
		}
	}
}

func (collector *streamCollector) Close() {
	if len(bytes.TrimSpace(collector.pending)) > 0 {
		collector.Write([]byte{'\n'})
	}
	if collector.record.ResponseIDHash == "" && len(collector.directJSON) > 0 {
		collector.consume(collector.directJSON)
	}
	collector.pending = nil
	collector.directJSON = nil
	collector.reasoningSeen = nil
	collector.toolIDs = nil
	collector.doneItemIDs = nil
	collector.doneOutputItems = nil
}

func (collector *streamCollector) consume(data []byte) {
	var payload map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if decoder.Decode(&payload) != nil {
		collector.record.ErrorCode = "response_event_invalid_json"
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		collector.record.ErrorCode = "response_event_invalid_json"
		return
	}
	eventType, _ := payload["type"].(string)
	if eventType == "response.created" || eventType == "response.in_progress" {
		if response, ok := payload["response"].(map[string]any); ok {
			if responseID, _ := response["id"].(string); responseID != "" {
				if collector.createdResponseID != "" && collector.createdResponseID != responseID {
					collector.record.ErrorCode = "response_id_drift"
				}
				collector.createdResponseID = responseID
			}
			if model, ok := response["model"].(string); ok && model != "" {
				if collector.record.ResponseCreatedModel != "" && collector.record.ResponseCreatedModel != model {
					collector.record.ErrorCode = "response_model_drift"
				}
				collector.record.ResponseCreatedModel = model
			}
		}
		return
	}
	if eventType == "response.output_item.done" {
		item, ok := payload["item"].(map[string]any)
		if !ok {
			collector.record.ErrorCode = "response_output_item_invalid"
			return
		}
		projection, ok := collector.projectOutputItem(item)
		if !ok {
			return
		}
		if projection.itemID != "" {
			if _, duplicate := collector.doneItemIDs[projection.itemID]; duplicate {
				collector.record.ErrorCode = "response_output_item_duplicate_id"
				return
			}
			collector.doneItemIDs[projection.itemID] = struct{}{}
		}
		collector.doneOutputItems = append(collector.doneOutputItems, projection)
		return
	}
	if eventType == "response.failed" {
		collector.consumeResponseFailure(payload, data)
		return
	}
	if eventType != "response.completed" {
		return
	}
	response, ok := payload["response"].(map[string]any)
	if !ok {
		return
	}
	if collector.record.ResponseCompleted {
		collector.record.ErrorCode = "duplicate_response_completed"
		return
	}
	collector.record.ResponseCompleted = true
	collector.record.ResponseStatus, _ = response["status"].(string)
	if value, ok := response["id"].(string); ok && value != "" {
		if collector.createdResponseID != "" && collector.createdResponseID != value {
			collector.record.ErrorCode = "response_id_drift"
		}
		collector.record.ResponseIDHash = hashOpaque(value)
	}
	if value, ok := response["model"].(string); ok && value != "" {
		if collector.record.ResponseCreatedModel != "" && collector.record.ResponseCreatedModel != value {
			collector.record.ErrorCode = "response_model_drift"
		}
		collector.record.ResponseModel = value
	}
	if value, ok := response["service_tier"].(string); ok {
		collector.record.ResponseServiceTier = value
	}
	if usage, ok := response["usage"].(map[string]any); ok {
		input, cached, output, complete := atomicUsageReceipt(usage)
		if complete {
			collector.record.UsagePresent = true
			collector.record.InputTokens = input
			collector.record.CachedInputTokens = cached
			collector.record.OutputTokens = output
		}
		if details, ok := usage["output_tokens_details"].(map[string]any); ok {
			if reasoning := exactNonNegativeInt64(details["reasoning_tokens"]); reasoning != nil {
				collector.record.ReasoningOutputTokens = reasoning
			}
		}
		if details, ok := usage["input_tokens_details"].(map[string]any); ok {
			if cacheWrite := exactNonNegativeInt64(details["cache_write_tokens"]); cacheWrite != nil {
				collector.record.CacheWriteInputTokens = cacheWrite
			}
		}
	}
	selected := collector.doneOutputItems
	if rawOutputs, present := response["output"]; present {
		outputs, ok := rawOutputs.([]any)
		if !ok {
			collector.record.ErrorCode = "response_output_item_invalid"
			outputs = nil
		}
		completed := make([]outputItemEvidence, 0, len(outputs))
		for _, rawOutput := range outputs {
			output, ok := rawOutput.(map[string]any)
			if !ok {
				collector.record.ErrorCode = "response_output_item_invalid"
				continue
			}
			projection, ok := collector.projectOutputItem(output)
			if ok {
				completed = append(completed, projection)
			}
		}
		if len(collector.doneOutputItems) > 0 && !sameOutputItemEvidence(collector.doneOutputItems, completed) {
			collector.record.ErrorCode = "response_output_item_source_mismatch"
		}
		selected = completed
	}
	collector.applyOutputItems(selected)
}

func (collector *streamCollector) consumeResponseFailure(payload map[string]any, raw []byte) {
	if collector.record.ResponseFailureEventSHA256 != "" {
		collector.record.ErrorCode = "duplicate_response_failed"
		collector.record.Disposition = "experiment_invalid"
		return
	}
	collector.record.ResponseFailureEventSHA256 = hashRawBytes(raw)
	response, ok := payload["response"].(map[string]any)
	if !ok {
		collector.record.ErrorCode = "response_failed_code_missing"
		collector.record.Disposition = "experiment_invalid"
		return
	}
	collector.record.ResponseStatus, _ = response["status"].(string)
	if value, ok := response["id"].(string); ok && value != "" {
		if collector.createdResponseID != "" && collector.createdResponseID != value {
			collector.record.ErrorCode = "response_id_drift"
			collector.record.Disposition = "experiment_invalid"
			return
		}
		collector.record.ResponseIDHash = hashOpaque(value)
	}
	if value, ok := response["model"].(string); ok && value != "" {
		if collector.record.ResponseCreatedModel != "" && collector.record.ResponseCreatedModel != value {
			collector.record.ErrorCode = "response_model_drift"
			collector.record.Disposition = "experiment_invalid"
			return
		}
		collector.record.ResponseModel = value
	}
	errorObject, ok := response["error"].(map[string]any)
	if !ok {
		collector.record.ErrorCode = "response_failed_code_missing"
		collector.record.Disposition = "experiment_invalid"
		return
	}
	code, _ := errorObject["code"].(string)
	collector.record.ResponseFailureCode = code
	switch code {
	case "context_length_exceeded":
		collector.record.ErrorCode = "provider_context_failure"
		collector.record.Disposition = "agent_context_failure"
	case "":
		collector.record.ErrorCode = "response_failed_code_missing"
		collector.record.Disposition = "experiment_invalid"
	default:
		collector.record.ErrorCode = "response_failed_code_unknown"
		collector.record.Disposition = "experiment_invalid"
	}
}

func hashRawBytes(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (collector *streamCollector) projectOutputItem(item map[string]any) (outputItemEvidence, bool) {
	projection := outputItemEvidence{hash: collector.hashValue(item)}
	if projection.hash == "" {
		collector.record.ErrorCode = "response_output_item_invalid"
		return outputItemEvidence{}, false
	}
	projection.itemID, _ = item["id"].(string)
	kind, _ := item["type"].(string)
	if kind == "reasoning" {
		encrypted, _ := item["encrypted_content"].(string)
		if encrypted != "" {
			projection.reasoningHash = collector.hashValue(encrypted)
			projection.reasoningKey = "hash:" + projection.reasoningHash
			if projection.itemID != "" {
				projection.reasoningKey = "id:" + projection.itemID
			}
		}
	}
	toolCall, isCall, known := terminalToolCallEvidence(item)
	if !known {
		collector.record.ErrorCode = "response_output_item_unknown"
		// Retain the content-free HMAC/count so an unknown provider item cannot
		// disappear from evidence even though the round is fail-closed.
		return projection, true
	}
	if isCall && toolCall.IDHash == "" {
		collector.record.ErrorCode = "response_tool_call_invalid"
		return outputItemEvidence{}, false
	}
	projection.toolCall = toolCall
	projection.isToolCall = isCall
	return projection, true
}

func sameOutputItemEvidence(left, right []outputItemEvidence) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].hash != right[index].hash {
			return false
		}
	}
	return true
}

func (collector *streamCollector) applyOutputItems(items []outputItemEvidence) {
	for _, item := range items {
		collector.record.ResponseOutputItemHashes = append(collector.record.ResponseOutputItemHashes, item.hash)
		if item.reasoningHash != "" {
			if _, duplicate := collector.reasoningSeen[item.reasoningKey]; duplicate {
				collector.record.ErrorCode = "response_reasoning_item_duplicate"
			} else {
				collector.reasoningSeen[item.reasoningKey] = struct{}{}
				collector.record.EncryptedReasoningHashes = append(collector.record.EncryptedReasoningHashes, item.reasoningHash)
			}
		}
		if item.isToolCall {
			if _, duplicate := collector.toolIDs[item.toolCall.IDHash]; duplicate {
				collector.record.ErrorCode = "response_tool_call_duplicate_id"
			} else {
				collector.toolIDs[item.toolCall.IDHash] = struct{}{}
				collector.record.ToolCalls = append(collector.record.ToolCalls, item.toolCall)
			}
		}
	}
	collector.record.ResponseOutputItemCount = len(collector.record.ResponseOutputItemHashes)
	collector.record.EncryptedReasoningItemCount = len(collector.record.EncryptedReasoningHashes)
}

func terminalToolCallEvidence(item map[string]any) (ToolCall, bool, bool) {
	kind, _ := item["type"].(string)
	switch kind {
	case "message", "agent_message", "reasoning", "compaction", "compaction_summary", "context_compaction",
		"function_call_output", "custom_tool_call_output", "local_shell_call_output", "shell_call_output",
		"apply_patch_call_output", "computer_call_output", "tool_search_output", "mcp_approval_response":
		return ToolCall{}, false, true
	}

	name := ""
	payloadField := ""
	payloadRequired := true
	switch kind {
	case "function_call":
		name, _ = item["name"].(string)
		payloadField = "arguments"
	case "custom_tool_call":
		name, _ = item["name"].(string)
		payloadField = "input"
	case "local_shell_call":
		name, payloadField = "local_shell", "action"
	case "shell_call":
		name, payloadField = "shell", "action"
	case "apply_patch_call":
		name, payloadField = "apply_patch", "operation"
	case "computer_call":
		name, payloadField = "computer", "action"
	case "web_search_call":
		name, payloadField = "web_search", "action"
	case "file_search_call":
		name, payloadField, payloadRequired = "file_search", "queries", false
	case "code_interpreter_call":
		name, payloadField, payloadRequired = "code_interpreter", "code", false
	case "image_generation_call":
		name, payloadField, payloadRequired = "image_generation", "revised_prompt", false
	case "tool_search_call":
		name, payloadField = "tool_search", "arguments"
	case "mcp_call":
		name, _ = item["name"].(string)
		if name == "" {
			name = "mcp"
		}
		payloadField = "arguments"
	case "mcp_list_tools":
		name, payloadField, payloadRequired = "mcp_list_tools", "server_label", false
	case "mcp_approval_request":
		name, payloadField, payloadRequired = "mcp_approval_request", "arguments", false
	default:
		return ToolCall{}, false, false
	}

	id, _ := item["call_id"].(string)
	if id == "" {
		id, _ = item["id"].(string)
	}
	payload, payloadPresent := item[payloadField]
	inputBytes, payloadValid := toolPayloadBytes(payload, payloadPresent)
	if id == "" || name == "" || (payloadRequired && !payloadValid) {
		return ToolCall{}, true, true
	}
	return ToolCall{IDHash: hashOpaque(id), Kind: kind, Name: name, InputBytes: inputBytes}, true, true
}

func toolPayloadBytes(value any, present bool) (int64, bool) {
	if !present {
		return 0, false
	}
	if text, ok := value.(string); ok {
		return int64(len([]byte(text))), true
	}
	canonical, err := canonicalBindingValue(value)
	if err != nil {
		return 0, false
	}
	return int64(len(canonical)), true
}

func inspectRequest(body []byte, hashValue func(any) string) requestMetadata {
	var request map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if decoder.Decode(&request) != nil {
		return requestMetadata{}
	}
	metadata := requestMetadata{ReasoningModeTypeValid: true, ServiceTierTypeValid: true, ToolDefinitionsValid: true, ToolResultsValid: true, CachePolicyValid: true}
	cachePolicy, cacheObserved := cacheevidence.InspectRequest(body)
	metadata.CachePolicyObserved = cacheObserved && cachePolicy.Observed
	metadata.CachePolicyValid = cacheObserved && comparableCachePolicy(cachePolicy)
	metadata.PromptCacheKeyPresent = cachePolicy.PromptCacheKeyPresent
	metadata.PromptCacheKeyHash = cachePolicy.PromptCacheKeySHA256
	metadata.PromptCacheOptionsPresent = cachePolicy.PromptCacheOptionsPresent
	metadata.PromptCacheOptionsMode = cachePolicy.PromptCacheOptionsMode
	metadata.PromptCacheTTLSeconds = cachePolicy.PromptCacheOptionsTTLSeconds
	metadata.PromptCacheRetentionPresent = cachePolicy.PromptCacheRetentionPresent
	metadata.PromptCacheRetention = cachePolicy.PromptCacheRetention
	metadata.CacheBreakpointCount = cachePolicy.PromptCacheBreakpointCount
	metadata.CacheBreakpointPositionHashes = append([]string(nil), cachePolicy.PromptCacheBreakpointPositions...)
	metadata.Model, _ = request["model"].(string)
	if value, present := request["service_tier"]; present {
		metadata.ServiceTierPresent = true
		metadata.ServiceTier, metadata.ServiceTierTypeValid = value.(string)
	}
	metadata.ReasoningEffort, _ = request["reasoning_effort"].(string)
	if reasoning, ok := request["reasoning"].(map[string]any); ok {
		if nested, ok := reasoning["effort"].(string); ok {
			metadata.ReasoningEffort = nested
		}
		metadata.ReasoningContext, _ = reasoning["context"].(string)
		if value, present := reasoning["mode"]; present {
			metadata.ReasoningMode, metadata.ReasoningModeTypeValid = value.(string)
		}
	} else if _, present := request["reasoning"]; present && request["reasoning"] != nil {
		metadata.ReasoningModeTypeValid = false
	}
	if textControls, ok := request["text"].(map[string]any); ok {
		metadata.TextVerbosity, _ = textControls["verbosity"].(string)
	}
	if value, present := request["max_output_tokens"]; present {
		metadata.MaxOutputTokensSpecified = true
		metadata.MaxOutputTokens = exactNonNegativeInt64(value)
	}
	if value, exists := request["store"]; exists {
		metadata.StoreSpecified = true
		metadata.Store, metadata.StoreTypeValid = value.(bool)
	}
	if value, ok := request["previous_response_id"].(string); ok && value != "" {
		metadata.PreviousResponseIDPresent = true
		metadata.PreviousResponseIDHash = hashOpaque(value)
	}
	if include, ok := request["include"].([]any); ok {
		for _, value := range include {
			if value == "reasoning.encrypted_content" {
				metadata.EncryptedReasoningRequested = true
				break
			}
		}
	}
	if rawTools, present := request["tools"]; present {
		definitions, valid := inspectToolDefinitions(rawTools, hashValue)
		metadata.ToolDefinitions = append(metadata.ToolDefinitions, definitions...)
		metadata.ToolDefinitionsValid = metadata.ToolDefinitionsValid && valid
	}
	inputs, _ := request["input"].([]any)
	for _, rawInput := range inputs {
		input, ok := rawInput.(map[string]any)
		if !ok {
			continue
		}
		kind, _ := input["type"].(string)
		if kind == "additional_tools" {
			definitions, valid := inspectToolDefinitions(input["tools"], hashValue)
			metadata.ToolDefinitions = append(metadata.ToolDefinitions, definitions...)
			metadata.ToolDefinitionsValid = metadata.ToolDefinitionsValid && valid
		}
		if isReplayedOutputItem(input) {
			if hash := hashValue(input); hash != "" {
				metadata.ReplayOutputItemHashes = append(metadata.ReplayOutputItemHashes, hash)
			}
		}
		if kind == "reasoning" {
			if encrypted, _ := input["encrypted_content"].(string); encrypted != "" {
				// Preserve wire order: this proves the exact sequence of opaque
				// reasoning items without retaining any provider content.
				metadata.EncryptedReasoningReplayHashes = append(metadata.EncryptedReasoningReplayHashes, hashValue(encrypted))
			}
			continue
		}
		switch kind {
		case "function_call_output", "custom_tool_call_output", "local_shell_call_output", "shell_call_output", "apply_patch_call_output", "computer_call_output":
		default:
			continue
		}
		if !knownToolResultFields(kind, input) {
			metadata.ToolResultsValid = false
			continue
		}
		callID, _ := input["call_id"].(string)
		if callID == "" {
			callID, _ = input["id"].(string)
		}
		if callID == "" {
			metadata.ToolResultsValid = false
			continue
		}
		outputBytes := int64(0)
		if value, exists := input["output"]; exists {
			outputBytes = jsonValueBytes(value)
		}
		payloadHash := hashValue(input)
		if payloadHash == "" {
			metadata.ToolResultsValid = false
			continue
		}
		metadata.ToolResults = append(metadata.ToolResults, ToolResult{IDHash: hashOpaque(callID), Kind: kind, PayloadHash: payloadHash, OutputBytes: outputBytes})
	}
	metadata.ToolCatalogHash = hashValue(metadata.ToolDefinitions)
	metadata.ToolCatalogSemanticSHA256 = stableToolCatalogSHA256(metadata.ToolDefinitions)
	for _, definition := range metadata.ToolDefinitions {
		metadata.ToolCatalogCanonicalBytes += definition.DefinitionBytes
	}
	return metadata
}

func comparableCachePolicy(policy cacheevidence.RequestPolicy) bool {
	if !policy.Observed || !policy.ShapeValid || policy.PromptCacheBreakpointCount != len(policy.PromptCacheBreakpointPositions) {
		return false
	}
	if policy.PromptCacheOptionsPresent {
		if policy.PromptCacheOptionsMode != "implicit" && policy.PromptCacheOptionsMode != "explicit" {
			return false
		}
	} else if policy.PromptCacheOptionsMode != "" || policy.PromptCacheOptionsTTLPresent || policy.PromptCacheOptionsTTLSeconds != nil {
		return false
	}
	if policy.PromptCacheOptionsTTLPresent != (policy.PromptCacheOptionsTTLSeconds != nil) ||
		policy.PromptCacheOptionsTTLSeconds != nil && *policy.PromptCacheOptionsTTLSeconds <= 0 {
		return false
	}
	if policy.PromptCacheRetentionPresent {
		if policy.PromptCacheRetention != "24h" && policy.PromptCacheRetention != "in_memory" {
			return false
		}
	} else if policy.PromptCacheRetention != "" {
		return false
	}
	return !(policy.PromptCacheOptionsPresent && policy.PromptCacheRetentionPresent)
}

func knownToolResultFields(kind string, input map[string]any) bool {
	allowed := map[string]struct{}{
		"type": {}, "call_id": {}, "id": {}, "output": {}, "status": {},
		"name": {}, "acknowledged_safety_checks": {},
		"internal_chat_message_metadata_passthrough": {},
	}
	for field := range input {
		if _, ok := allowed[field]; !ok {
			return false
		}
	}
	if _, present := input["output"]; !present {
		return false
	}
	if status, present := input["status"]; present {
		value, ok := status.(string)
		if !ok || value == "" {
			return false
		}
	}
	if name, present := input["name"]; present {
		if _, ok := name.(string); !ok {
			return false
		}
	}
	if kind != "computer_call_output" {
		if _, present := input["acknowledged_safety_checks"]; present {
			return false
		}
	}
	return true
}

func inspectToolDefinitions(raw any, hashValue func(any) string) ([]ToolDefinitionEvidence, bool) {
	definitions, ok := raw.([]any)
	if !ok {
		return nil, false
	}
	result := make([]ToolDefinitionEvidence, 0, len(definitions))
	valid := true
	for _, rawDefinition := range definitions {
		definition, ok := rawDefinition.(map[string]any)
		if !ok {
			valid = false
			continue
		}
		kind, _ := definition["type"].(string)
		name, _ := definition["name"].(string)
		if name == "" {
			name = kind
		}
		if kind == "" || name == "" {
			valid = false
		}
		evidence := ToolDefinitionEvidence{Type: kind, Name: name, BillingOwner: "client"}
		switch kind {
		case "function", "custom":
			if explicitName, _ := definition["name"].(string); explicitName == "" {
				valid = false
			}
			if _, providerCallable := definition["allowed_callers"]; providerCallable {
				evidence.BillingOwner = "unknown"
				valid = false
			}
		case "apply_patch":
		case "shell":
			environment, environmentValid := definition["environment"].(map[string]any)
			if !environmentValid || len(environment) != 1 || environment["type"] != "local" {
				evidence.BillingOwner = "unknown"
				valid = false
			}
		default:
			evidence.BillingOwner = "unknown"
			valid = false
		}
		if rawStrict, present := definition["strict"]; present {
			strict, strictValid := rawStrict.(bool)
			if !strictValid {
				valid = false
			} else {
				value := strict
				evidence.Strict = &value
			}
		}
		if rawDescription, present := definition["description"]; present {
			description, descriptionValid := rawDescription.(string)
			if !descriptionValid {
				valid = false
			} else if description != "" {
				evidence.DescriptionBytes = int64(len([]byte(description)))
				evidence.DescriptionSHA256 = stableSHA256Bytes([]byte(description))
			}
		}
		var schema any
		for _, field := range []string{"parameters", "input_schema", "schema", "format"} {
			if value, present := definition[field]; present {
				schema = value
				break
			}
		}
		if schema != nil {
			if canonical, err := canonicalBindingValue(schema); err == nil {
				evidence.SchemaBytes = int64(len(canonical))
				evidence.SchemaHash = hashValue(schema)
				evidence.SchemaSHA256 = stableSHA256Bytes(canonical)
			} else {
				valid = false
			}
		}
		canonicalDefinition, err := canonicalBindingValue(definition)
		if err != nil || len(canonicalDefinition) == 0 {
			valid = false
		} else {
			evidence.DefinitionBytes = int64(len(canonicalDefinition))
			evidence.DefinitionSHA256 = stableSHA256Bytes(canonicalDefinition)
		}
		result = append(result, evidence)
	}
	return result, valid
}

func localToolCallsBoundToCatalog(calls []ToolCall, definitions []ToolDefinitionEvidence) bool {
	for _, call := range calls {
		bound := false
		for _, definition := range definitions {
			if definition.BillingOwner != "client" {
				continue
			}
			switch definition.Type {
			case "function":
				bound = call.Kind == "function_call" && call.Name == definition.Name
			case "custom":
				bound = call.Kind == "custom_tool_call" && call.Name == definition.Name
			case "apply_patch":
				bound = call.Kind == "apply_patch_call" && call.Name == "apply_patch"
			case "shell":
				bound = (call.Kind == "shell_call" || call.Kind == "local_shell_call") && (call.Name == "shell" || call.Name == "local_shell")
			}
			if bound {
				break
			}
		}
		if !bound {
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
	canonical, err := canonicalBindingValue(projection)
	if err != nil {
		return ""
	}
	return stableSHA256Bytes(canonical)
}

func stableSHA256Bytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func isReplayedOutputItem(item map[string]any) bool {
	kind, _ := item["type"].(string)
	switch kind {
	case "reasoning", "agent_message", "compaction", "compaction_summary", "context_compaction",
		"function_call", "custom_tool_call", "local_shell_call", "shell_call", "apply_patch_call",
		"computer_call", "web_search_call", "file_search_call", "code_interpreter_call", "image_generation_call",
		"tool_search_call", "mcp_call", "mcp_list_tools", "mcp_approval_request":
		return true
	case "message":
		role, _ := item["role"].(string)
		return role == "assistant"
	case "function_call_output", "custom_tool_call_output", "local_shell_call_output", "shell_call_output", "apply_patch_call_output", "computer_call_output", "tool_search_output", "mcp_approval_response":
		return false
	}
	// Future provider output items are replayable when they carry the output
	// identity/status pair. This avoids silently dropping new item kinds while
	// excluding ordinary locally-generated input parts.
	id, _ := item["id"].(string)
	status, _ := item["status"].(string)
	return kind != "" && id != "" && status != ""
}

func jsonValueBytes(value any) int64 {
	if text, ok := value.(string); ok {
		return int64(len([]byte(text)))
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return 0
	}
	return int64(len(raw))
}

func readLimited(reader io.ReadCloser, limit int64) ([]byte, error) {
	if reader == nil {
		return nil, nil
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("request body exceeds configured limit")
	}
	return data, nil
}

func hashOpaque(value string) string {
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func isHex64(value string) bool {
	if len(value) != 64 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func isLowerHex64(value string) bool {
	return value == strings.ToLower(value) && isHex64(value)
}

func zero(data []byte) {
	for index := range data {
		data[index] = 0
	}
}

func joinURLPath(base, request string) string {
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(request, "/")
}

func removeHopHeaders(headers http.Header) {
	for _, name := range []string{"Connection", "Proxy-Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade"} {
		headers.Del(name)
	}
}

func copyHeaders(destination, source http.Header) {
	for name, values := range source {
		for _, value := range values {
			destination.Add(name, value)
		}
	}
}

func firstHeader(headers http.Header, names ...string) string {
	for _, name := range names {
		if value := headers.Get(name); value != "" {
			return value
		}
	}
	return ""
}

func atomicUsageReceipt(usage map[string]any) (input, cached, output *int64, complete bool) {
	input = exactNonNegativeInt64(usage["input_tokens"])
	output = exactNonNegativeInt64(usage["output_tokens"])
	details, detailsPresent := usage["input_tokens_details"].(map[string]any)
	if detailsPresent {
		cached = exactNonNegativeInt64(details["cached_tokens"])
	}
	if input == nil || cached == nil || output == nil {
		return nil, nil, nil, false
	}
	return input, cached, output, true
}

func validAtomicUsageReceipt(record Record) bool {
	return record.UsagePresent && record.InputTokens != nil && record.CachedInputTokens != nil && record.OutputTokens != nil &&
		*record.InputTokens >= 0 && *record.CachedInputTokens >= 0 && *record.OutputTokens >= 0 &&
		*record.CachedInputTokens <= *record.InputTokens
}

func exactNonNegativeInt64(value any) *int64 {
	var parsed int64
	switch typed := value.(type) {
	case float64:
		if typed < 0 || typed >= math.Exp2(63) || math.Trunc(typed) != typed {
			return nil
		}
		parsed = int64(typed)
	case json.Number:
		value, err := strconv.ParseInt(typed.String(), 10, 64)
		if err != nil || value < 0 {
			return nil
		}
		parsed = value
	default:
		return nil
	}
	return &parsed
}
