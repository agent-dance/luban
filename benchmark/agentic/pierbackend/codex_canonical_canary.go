package pierbackend

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
	"github.com/agent-dance/luban/i18n"
)

const (
	formalCodexV8CanaryGeneration          = "v8"
	formalCodexV8CanaryReceiptRelativePath = "benchmark/agentic/pier/codex-exec-v8-multi-agent-wire.receipt.json"
	formalCodexV8CanaryReceiptSHA256       = ""
	Codex0145BinarySHA256                  = "a2a05dafaa1acb002a45eaec0a462de5b13694fcfcd7bc43305f14781ce7be14"
)

type codexCanaryAuthorityState string

const (
	codexCanaryCandidatePending codexCanaryAuthorityState = "candidate_pending"
	codexCanaryVerifiedFormal   codexCanaryAuthorityState = "verified_formal"
)

// ErrCodexV8CanonicalCanaryPending is an internal, typed readiness marker.
// User-visible rendering is provided by the semantic i18n error that wraps it.
var ErrCodexV8CanonicalCanaryPending = errors.New("codex-v8-canonical-canary-pending")

type codexCanaryAuthoritySpec struct {
	Generation    string
	RelativePath  string
	ReceiptSHA256 string
	State         codexCanaryAuthorityState
}

type formalExecutionCanaryBinding struct {
	AgentID              string
	Generation           string
	TransportRequirement string
	Path                 string
	SHA256               string
}

type formalCodexCanonicalCanaryBinding = formalExecutionCanaryBinding

type configuredExecutionCanary struct {
	AgentID       string
	ReceiptPath   string
	ReceiptSHA256 string
}

func configuredExecutionCanaryPins(config Config) []configuredExecutionCanary {
	return []configuredExecutionCanary{
		{AgentID: "codex", ReceiptPath: config.CodexV8CanaryReceiptPath, ReceiptSHA256: config.CodexV8CanaryReceiptSHA256},
		{AgentID: "luban", ReceiptPath: config.LubanV8CanaryReceiptPath, ReceiptSHA256: config.LubanV8CanaryReceiptSHA256},
	}
}

// validateConfiguredCanaryPinShape is filesystem-free so malformed or absent
// authority cannot trigger source inspection, TLS, registry, or provider work.
// New permits an entirely absent pair for the inventory-lock-only command;
// Preflight always calls this with allowAbsent=false.
func validateConfiguredCanaryPinShape(config Config, allowAbsent bool) error {
	pins := configuredExecutionCanaryPins(config)
	absent := true
	for _, pin := range pins {
		if pin.ReceiptPath != "" || pin.ReceiptSHA256 != "" {
			absent = false
		}
	}
	if absent {
		if allowAbsent {
			return nil
		}
		return i18n.WrapInternalError(i18n.KeyBenchmarkCodexV8CanaryPending, ErrCodexV8CanonicalCanaryPending)
	}
	for _, pin := range pins {
		if !filepath.IsAbs(pin.ReceiptPath) || !lowerHexSHA256(pin.ReceiptSHA256) {
			return errors.New("formal execution canary configuration lacks an absolute path or exact SHA-256 pin")
		}
	}
	if filepath.Clean(pins[0].ReceiptPath) == filepath.Clean(pins[1].ReceiptPath) {
		return errors.New("formal Codex and Luban execution canaries reuse one receipt path")
	}
	if pins[0].ReceiptSHA256 == pins[1].ReceiptSHA256 {
		return errors.New("formal Codex and Luban execution canaries reuse one receipt SHA-256")
	}
	return nil
}

func validateConfiguredExecutionCanaries(config Config, manifest harness.Manifest) (map[string]configuredExecutionCanary, error) {
	if err := validateConfiguredCanaryPinShape(config, false); err != nil {
		return nil, err
	}
	if len(manifest.Agents) != 2 {
		return nil, errors.New("formal execution canary binding requires exactly two manifest agents")
	}
	pins := configuredExecutionCanaryPins(config)
	byID := make(map[string]configuredExecutionCanary, len(pins))
	for _, pin := range pins {
		byID[pin.AgentID] = pin
	}
	seen := make(map[string]struct{}, len(manifest.Agents))
	for _, agent := range manifest.Agents {
		if _, duplicate := seen[agent.ID]; duplicate {
			return nil, errors.New("formal execution canary binding contains a duplicate manifest agent")
		}
		seen[agent.ID] = struct{}{}
		pin, exists := byID[agent.ID]
		if !exists || agent.ExecutionCanary.Generation != formalCodexV8CanaryGeneration ||
			agent.Model.TransportRequirement != harness.TransportRequirementHTTPInference ||
			agent.ExecutionCanary.ReceiptSHA256 != pin.ReceiptSHA256 {
			return nil, errors.New("formal execution canary differs from its v8 HTTP agent binding")
		}
	}
	if len(seen) != len(byID) {
		return nil, errors.New("formal execution canary binding is missing Codex or Luban")
	}
	return byID, nil
}

// validateConfiguredExecutionCanaryHeaders rejects a stale, swapped, pending,
// symlinked, or byte-drifted archive before any source, network, registry, or
// provider preflight work. Full adapter/bundle/tool validation follows after
// those immutable identities have been resolved.
func validateConfiguredExecutionCanaryHeaders(pins map[string]configuredExecutionCanary) error {
	for _, agentID := range []string{"codex", "luban"} {
		pin, exists := pins[agentID]
		if !exists || pin.AgentID != agentID {
			return errors.New("formal execution canary header set is missing its agent binding")
		}
		info, err := os.Lstat(pin.ReceiptPath)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("formal execution canary header is not a regular non-symlink file")
		}
		digest, err := harness.HashFile(pin.ReceiptPath)
		if err != nil {
			return err
		}
		if digest != pin.ReceiptSHA256 {
			return errors.New("formal execution canary header differs from its exact-byte pin")
		}
		raw, err := os.ReadFile(pin.ReceiptPath)
		if err != nil {
			return err
		}
		if err := validateStrictJSON(raw); err != nil {
			return err
		}
		var header struct {
			SchemaVersion           string          `json:"schema_version"`
			AgentKind               string          `json:"agent_kind"`
			ProviderCanaryTransport string          `json:"provider_canary_transport"`
			CanonicalAuthority      json.RawMessage `json:"canonical_authority"`
		}
		if err := json.Unmarshal(raw, &header); err != nil {
			return err
		}
		if header.SchemaVersion != "agentic-bench/sandbox-canary-v4" || header.AgentKind != agentID ||
			header.ProviderCanaryTransport != "responses-http-inference-required" {
			return errors.New("formal execution canary header is stale, swapped, or not HTTP-only")
		}
		if agentID == "codex" {
			authority, err := strictDecodeCanaryObject[codexCanonicalAuthorityReceipt](header.CanonicalAuthority, "Codex v8 preflight authority", "generation", "authority_scope", "responses_transport_requirement")
			if err != nil {
				return err
			}
			if authority.Generation != formalCodexV8CanaryGeneration || authority.AuthorityScope != string(codexCanaryVerifiedFormal) ||
				authority.ResponsesTransportRequirement != harness.TransportRequirementHTTPInference {
				return i18n.WrapInternalError(i18n.KeyBenchmarkCodexV8CanaryPending, ErrCodexV8CanonicalCanaryPending)
			}
		} else if len(header.CanonicalAuthority) != 0 && string(header.CanonicalAuthority) != "null" {
			return errors.New("Luban execution canary header contains Codex authority")
		}
	}
	return nil
}

func executionCanarySnapshots(bindings map[string]formalExecutionCanaryBinding) []harness.ExecutionCanarySnapshot {
	result := make([]harness.ExecutionCanarySnapshot, 0, 2)
	for _, agentID := range []string{"codex", "luban"} {
		binding := bindings[agentID]
		result = append(result, harness.ExecutionCanarySnapshot{
			AgentID: binding.AgentID, Generation: binding.Generation,
			TransportRequirement: binding.TransportRequirement, ReceiptSHA256: binding.SHA256,
		})
	}
	return result
}

type codexCanonicalAuthorityReceipt struct {
	Generation                    string `json:"generation"`
	AuthorityScope                string `json:"authority_scope"`
	ResponsesTransportRequirement string `json:"responses_transport_requirement"`
}

func (authority *codexCanonicalAuthorityReceipt) UnmarshalJSON(raw []byte) error {
	type wire codexCanonicalAuthorityReceipt
	decoded, err := strictDecodeCanaryObject[wire](raw, "Codex v8 canonical authority", "generation", "authority_scope", "responses_transport_requirement")
	if err != nil {
		return err
	}
	*authority = codexCanonicalAuthorityReceipt(decoded)
	return nil
}

func formalCodexV8CanaryAuthority() codexCanaryAuthoritySpec {
	return codexCanaryAuthoritySpec{
		Generation: formalCodexV8CanaryGeneration, RelativePath: formalCodexV8CanaryReceiptRelativePath,
		ReceiptSHA256: formalCodexV8CanaryReceiptSHA256, State: codexCanaryCandidatePending,
	}
}

// requireFormalCodexV8CanaryReady retains the original filesystem-free pending
// sentinel for historical-authority audits. Production Preflight applies the
// equivalent gate to the two Config pins and their exact receipt headers.
func requireFormalCodexV8CanaryReady() (codexCanaryAuthoritySpec, error) {
	spec := formalCodexV8CanaryAuthority()
	if spec.Generation != formalCodexV8CanaryGeneration || spec.RelativePath != formalCodexV8CanaryReceiptRelativePath {
		return codexCanaryAuthoritySpec{}, errors.New("formal Codex v8 canary authority specification is invalid")
	}
	if spec.State == codexCanaryCandidatePending {
		return codexCanaryAuthoritySpec{}, i18n.WrapInternalError(i18n.KeyBenchmarkCodexV8CanaryPending, ErrCodexV8CanonicalCanaryPending)
	}
	if spec.State != codexCanaryVerifiedFormal || !lowerHexSHA256(spec.ReceiptSHA256) {
		return codexCanaryAuthoritySpec{}, errors.New("formal Codex v8 canary authority is not a verified exact-byte pin")
	}
	return spec, nil
}

func resolveFormalCodexV8CanaryBinding(config Config, adapter adapterBinding, bundle codexBundleBinding) (formalCodexCanonicalCanaryBinding, error) {
	pins := configuredExecutionCanaryPins(config)
	if err := validateConfiguredCanaryPinShape(config, false); err != nil {
		return formalCodexCanonicalCanaryBinding{}, err
	}
	return resolvePinnedExecutionCanary(pins[0], harness.AgentSpec{
		ID: "codex", BinarySHA256: Codex0145BinarySHA256,
		Model: harness.ModelRequestSpec{
			Model: "gpt-5.6-sol", ReasoningEffort: "xhigh", ServiceTier: harness.FormalServiceTier,
			ServiceTierRequestEncoding: harness.ServiceTierEncodingClientCanonical,
			TransportRequirement:       harness.TransportRequirementHTTPInference,
		},
	}, adapter, bundle)
}

func resolveFormalCodexV8CanaryBindingWithSpec(config Config, adapter adapterBinding, bundle codexBundleBinding, spec codexCanaryAuthoritySpec) (formalCodexCanonicalCanaryBinding, error) {
	if spec.Generation != formalCodexV8CanaryGeneration || spec.State != codexCanaryVerifiedFormal ||
		spec.RelativePath != formalCodexV8CanaryReceiptRelativePath || !lowerHexSHA256(spec.ReceiptSHA256) {
		return formalCodexCanonicalCanaryBinding{}, errors.New("Codex canonical canary authority is not the frozen formal v8 specification")
	}
	path := filepath.Join(config.PythonModuleRoot, filepath.FromSlash(spec.RelativePath))
	info, err := os.Lstat(path)
	if err != nil {
		return formalCodexCanonicalCanaryBinding{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return formalCodexCanonicalCanaryBinding{}, errors.New("Codex v8 canonical canary receipt must be a regular non-symlink file")
	}
	digest, err := harness.HashFile(path)
	if err != nil {
		return formalCodexCanonicalCanaryBinding{}, err
	}
	if digest != spec.ReceiptSHA256 {
		return formalCodexCanonicalCanaryBinding{}, errors.New("Codex v8 canonical canary receipt differs from its exact-byte authority pin")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return formalCodexCanonicalCanaryBinding{}, err
	}
	if err := validateFormalCodexV8CanonicalCanaryReceipt(raw, adapter, bundle); err != nil {
		return formalCodexCanonicalCanaryBinding{}, err
	}
	return formalCodexCanonicalCanaryBinding{
		AgentID: "codex", Generation: spec.Generation, TransportRequirement: harness.TransportRequirementHTTPInference,
		Path: path, SHA256: digest,
	}, nil
}

func resolveFormalExecutionCanaryBindings(config Config, manifest harness.Manifest, adapter adapterBinding, bundle codexBundleBinding) (map[string]formalExecutionCanaryBinding, error) {
	pins, err := validateConfiguredExecutionCanaries(config, manifest)
	if err != nil {
		return nil, err
	}
	result := make(map[string]formalExecutionCanaryBinding, len(pins))
	for _, agentID := range []string{"codex", "luban"} {
		agent, found := manifestAgent(manifest, agentID)
		if !found {
			return nil, errors.New("formal execution canary binding is missing its manifest agent")
		}
		binding, err := resolvePinnedExecutionCanary(pins[agentID], agent, adapter, bundle)
		if err != nil {
			return nil, err
		}
		result[agentID] = binding
	}
	if result["codex"].Path == result["luban"].Path || result["codex"].SHA256 == result["luban"].SHA256 {
		return nil, errors.New("resolved execution canary authority is not unique per agent")
	}
	return result, nil
}

func resolvePinnedExecutionCanary(pin configuredExecutionCanary, agent harness.AgentSpec, adapter adapterBinding, bundle codexBundleBinding) (formalExecutionCanaryBinding, error) {
	if pin.AgentID != agent.ID || !filepath.IsAbs(pin.ReceiptPath) || !lowerHexSHA256(pin.ReceiptSHA256) {
		return formalExecutionCanaryBinding{}, errors.New("execution canary pin is not bound to its manifest agent")
	}
	info, err := os.Lstat(pin.ReceiptPath)
	if err != nil {
		return formalExecutionCanaryBinding{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return formalExecutionCanaryBinding{}, errors.New("execution canary receipt must be a regular non-symlink file")
	}
	digest, err := harness.HashFile(pin.ReceiptPath)
	if err != nil {
		return formalExecutionCanaryBinding{}, err
	}
	if digest != pin.ReceiptSHA256 || digest != agent.ExecutionCanary.ReceiptSHA256 {
		return formalExecutionCanaryBinding{}, errors.New("execution canary receipt differs from its config or manifest exact-byte pin")
	}
	raw, err := os.ReadFile(pin.ReceiptPath)
	if err != nil {
		return formalExecutionCanaryBinding{}, err
	}
	switch agent.ID {
	case "codex":
		if err := validateFormalCodexV8CanonicalCanaryReceiptForAgent(raw, agent, adapter, bundle); err != nil {
			return formalExecutionCanaryBinding{}, err
		}
	case "luban":
		if err := validateFormalLubanV8CanaryReceipt(raw, agent, adapter, bundle); err != nil {
			return formalExecutionCanaryBinding{}, err
		}
	default:
		return formalExecutionCanaryBinding{}, errors.New("execution canary receipt has an unsupported agent binding")
	}
	return formalExecutionCanaryBinding{
		AgentID: agent.ID, Generation: formalCodexV8CanaryGeneration,
		TransportRequirement: harness.TransportRequirementHTTPInference, Path: pin.ReceiptPath, SHA256: digest,
	}, nil
}

func manifestAgent(manifest harness.Manifest, agentID string) (harness.AgentSpec, bool) {
	for _, agent := range manifest.Agents {
		if agent.ID == agentID {
			return agent, true
		}
	}
	return harness.AgentSpec{}, false
}

func validateFormalCodexV8CanonicalCanaryReceipt(raw []byte, adapter adapterBinding, bundle codexBundleBinding) error {
	var receipt struct {
		SchemaVersion            string                         `json:"schema_version"`
		CanonicalAuthority       codexCanonicalAuthorityReceipt `json:"canonical_authority"`
		AdapterSHA256            string                         `json:"adapter_sha256"`
		AgentKind                string                         `json:"agent_kind"`
		BinarySHA256             string                         `json:"binary_sha256"`
		BundleManifestSHA256     string                         `json:"bundle_manifest_sha256"`
		RuntimePayloadTreeSHA256 string                         `json:"runtime_payload_tree_sha256"`
		ProviderCanaryTransport  string                         `json:"provider_canary_transport"`
		EffectiveArgvReceipt     struct {
			AdapterVersion     string `json:"adapter_version"`
			SemanticProjection struct {
				ServiceTier                   string `json:"service_tier"`
				ResponsesTransportRequirement string `json:"responses_transport_requirement"`
			} `json:"semantic_projection"`
		} `json:"effective_argv_receipt"`
		ProviderCanaryRequests []struct {
			RequestServiceTierPresent    bool    `json:"request_service_tier_present"`
			RequestServiceTier           *string `json:"request_service_tier"`
			RequestServiceTierSource     string  `json:"request_service_tier_source"`
			RequestServiceTierCanonical  string  `json:"request_service_tier_canonical"`
			ResponseServiceTier          string  `json:"response_service_tier"`
			ResponseServiceTierCanonical string  `json:"response_service_tier_canonical"`
		} `json:"provider_canary_requests"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&receipt); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("Codex v8 canonical canary receipt contains trailing JSON")
	}
	if receipt.SchemaVersion != "agentic-bench/sandbox-canary-v4" ||
		receipt.CanonicalAuthority.Generation != formalCodexV8CanaryGeneration ||
		receipt.CanonicalAuthority.AuthorityScope != string(codexCanaryVerifiedFormal) ||
		receipt.CanonicalAuthority.ResponsesTransportRequirement != harness.TransportRequirementHTTPInference ||
		receipt.ProviderCanaryTransport != "responses-http-inference-required" ||
		receipt.AdapterSHA256 != adapter.SHA256 || receipt.AgentKind != "codex" ||
		receipt.BinarySHA256 != Codex0145BinarySHA256 || receipt.BundleManifestSHA256 != bundle.ManifestSHA256 ||
		receipt.RuntimePayloadTreeSHA256 != bundle.TreeSHA256 || receipt.EffectiveArgvReceipt.AdapterVersion != PinnedAdapterVersion ||
		receipt.EffectiveArgvReceipt.SemanticProjection.ServiceTier != harness.FormalServiceTier ||
		receipt.EffectiveArgvReceipt.SemanticProjection.ResponsesTransportRequirement != harness.TransportRequirementHTTPInference ||
		len(receipt.ProviderCanaryRequests) != 2 {
		return errors.New("Codex v8 canonical canary receipt is not bound to the frozen HTTP runtime")
	}
	for _, request := range receipt.ProviderCanaryRequests {
		if request.RequestServiceTierPresent || request.RequestServiceTier != nil ||
			request.RequestServiceTierSource != serviceTierCanonicalizationRepresentation ||
			request.RequestServiceTierCanonical != harness.FormalServiceTier ||
			request.ResponseServiceTier != harness.FormalServiceTier ||
			request.ResponseServiceTierCanonical != harness.FormalServiceTier {
			return errors.New("Codex v8 canonical canary does not prove client-wire service-tier omission")
		}
	}
	return nil
}

type formalHTTPTransportReceipt struct {
	SchemaVersion                 string `json:"schema_version"`
	Requirement                   string `json:"requirement"`
	HTTPInferenceRequestCount     int    `json:"http_inference_request_count"`
	WebSocketUpgradeRequestCount  int    `json:"websocket_upgrade_request_count"`
	WebSocketGenerationFrameCount int    `json:"websocket_generation_frame_count"`
	PrewarmRequestCount           int    `json:"prewarm_request_count"`
}

func (receipt *formalHTTPTransportReceipt) UnmarshalJSON(raw []byte) error {
	type wire formalHTTPTransportReceipt
	decoded, err := strictDecodeCanaryObject[wire](raw, "formal HTTP transport", "schema_version", "requirement", "http_inference_request_count", "websocket_upgrade_request_count", "websocket_generation_frame_count", "prewarm_request_count")
	if err != nil {
		return err
	}
	*receipt = formalHTTPTransportReceipt(decoded)
	return nil
}

type formalCachePolicy struct {
	Observed                            bool     `json:"observed"`
	ShapeValid                          bool     `json:"shape_valid"`
	PromptCacheKeyPresent               bool     `json:"prompt_cache_key_present"`
	PromptCacheKeySHA256                string   `json:"prompt_cache_key_sha256"`
	PromptCacheOptionsPresent           bool     `json:"prompt_cache_options_present"`
	PromptCacheOptionsMode              string   `json:"prompt_cache_options_mode"`
	PromptCacheOptionsTTLPresent        bool     `json:"prompt_cache_options_ttl_present"`
	PromptCacheOptionsTTL               string   `json:"prompt_cache_options_ttl"`
	PromptCacheOptionsTTLSeconds        *int     `json:"prompt_cache_options_ttl_seconds"`
	PromptCacheRetentionPresent         bool     `json:"prompt_cache_retention_present"`
	PromptCacheRetention                string   `json:"prompt_cache_retention"`
	PromptCacheBreakpointCount          int      `json:"prompt_cache_breakpoint_count"`
	PromptCacheBreakpointPositionHashes []string `json:"prompt_cache_breakpoint_position_hashes"`
}

func (policy *formalCachePolicy) UnmarshalJSON(raw []byte) error {
	type wire formalCachePolicy
	decoded, err := strictDecodeCanaryObject[wire](raw, "formal cache policy",
		"observed", "shape_valid", "prompt_cache_key_present", "prompt_cache_key_sha256",
		"prompt_cache_options_present", "prompt_cache_options_mode", "prompt_cache_options_ttl_present",
		"prompt_cache_options_ttl", "prompt_cache_options_ttl_seconds", "prompt_cache_retention_present",
		"prompt_cache_retention", "prompt_cache_breakpoint_count", "prompt_cache_breakpoint_position_hashes")
	if err != nil {
		return err
	}
	*policy = formalCachePolicy(decoded)
	return nil
}

type formalCacheWireReceipt struct {
	SchemaVersion                string     `json:"schema_version"`
	ContentRetained              bool       `json:"content_retained"`
	ObservedRequests             int        `json:"observed_requests"`
	ShapeValidRequests           int        `json:"shape_valid_requests"`
	KeyPresentRequests           int        `json:"key_present_requests"`
	UniqueKeyCount               int        `json:"unique_key_count"`
	KeyTransitions               int        `json:"key_transitions"`
	FirstKeySHA256               string     `json:"first_key_sha256"`
	Stable                       bool       `json:"stable"`
	PromptCacheOptionsModes      []string   `json:"prompt_cache_options_modes"`
	PromptCacheOptionsTTLs       []string   `json:"prompt_cache_options_ttls"`
	PromptCacheOptionsTTLSeconds []*int     `json:"prompt_cache_options_ttl_seconds"`
	PromptCacheRetentions        []string   `json:"prompt_cache_retentions"`
	BreakpointCounts             []int      `json:"breakpoint_counts"`
	BreakpointPositionHashes     [][]string `json:"breakpoint_position_hashes"`
}

func (receipt *formalCacheWireReceipt) UnmarshalJSON(raw []byte) error {
	type wire formalCacheWireReceipt
	decoded, err := strictDecodeCanaryObject[wire](raw, "formal cache wire receipt",
		"schema_version", "content_retained", "observed_requests", "shape_valid_requests", "key_present_requests",
		"unique_key_count", "key_transitions", "first_key_sha256", "stable", "prompt_cache_options_modes",
		"prompt_cache_options_ttls", "prompt_cache_options_ttl_seconds", "prompt_cache_retentions",
		"breakpoint_counts", "breakpoint_position_hashes")
	if err != nil {
		return err
	}
	*receipt = formalCacheWireReceipt(decoded)
	return nil
}

type formalEnrichedCanaryReceipt struct {
	SchemaVersion                string                         `json:"schema_version"`
	AgentKind                    string                         `json:"agent_kind"`
	BinarySHA256                 string                         `json:"binary_sha256"`
	BaseCommit                   string                         `json:"base_commit"`
	ControllerProxyReachable     bool                           `json:"controller_proxy_reachable"`
	ToolProxyReachable           bool                           `json:"tool_proxy_reachable"`
	CredentialInAgent            bool                           `json:"credential_in_agent"`
	AdapterSHA256                string                         `json:"adapter_sha256"`
	BundleManifestSHA256         string                         `json:"bundle_manifest_sha256"`
	EffectiveArgvReceiptSHA256   string                         `json:"effective_argv_receipt_sha256"`
	SourceBundleTreeSHA256       string                         `json:"source_bundle_tree_sha256"`
	RuntimePayloadTreeSHA256     string                         `json:"runtime_payload_tree_sha256"`
	ProviderCanaryRequests       []json.RawMessage              `json:"provider_canary_requests"`
	ProviderCanaryTransport      string                         `json:"provider_canary_transport"`
	HTTPTransport                formalHTTPTransportReceipt     `json:"http_transport"`
	CacheWire                    formalCacheWireReceipt         `json:"cache_wire"`
	EffectiveArgvReceipt         effectiveArgvReceipt           `json:"effective_argv_receipt"`
	Overlay                      json.RawMessage                `json:"overlay"`
	EgressProxyImage             string                         `json:"egress_proxy_image"`
	CanonicalAuthority           codexCanonicalAuthorityReceipt `json:"canonical_authority,omitempty"`
	SandboxNegativeControl       json.RawMessage                `json:"sandbox_negative_control,omitempty"`
	WebSearchConfigurationCanary json.RawMessage                `json:"web_search_configuration_canary,omitempty"`
	WorkspaceState               json.RawMessage                `json:"workspace_state,omitempty"`
	LubanRuntimeVersions         []string                       `json:"luban_runtime_versions,omitempty"`
}

func decodeFormalEnrichedCanary(raw []byte, agentID string) (formalEnrichedCanaryReceipt, error) {
	fields := []string{
		"schema_version", "agent_kind", "binary_sha256", "base_commit", "controller_proxy_reachable",
		"tool_proxy_reachable", "credential_in_agent", "adapter_sha256", "bundle_manifest_sha256",
		"effective_argv_receipt_sha256", "source_bundle_tree_sha256", "runtime_payload_tree_sha256",
		"provider_canary_requests", "provider_canary_transport", "http_transport", "cache_wire",
		"effective_argv_receipt", "overlay", "egress_proxy_image",
	}
	switch agentID {
	case "codex":
		fields = append(fields, "canonical_authority", "sandbox_negative_control", "web_search_configuration_canary", "workspace_state")
	case "luban":
		fields = append(fields, "luban_runtime_versions")
	default:
		return formalEnrichedCanaryReceipt{}, errors.New("formal execution canary has an unsupported agent kind")
	}
	return strictDecodeCanaryObject[formalEnrichedCanaryReceipt](raw, "formal enriched execution canary", fields...)
}

func validateFormalCodexV8CanonicalCanaryReceiptForAgent(raw []byte, agent harness.AgentSpec, adapter adapterBinding, bundle codexBundleBinding) error {
	if agent.ID != "codex" {
		return errors.New("Codex v8 canary validator received a different agent")
	}
	if err := validateFormalCodexV8CanonicalCanaryReceipt(raw, adapter, bundle); err != nil {
		return err
	}
	receipt, err := decodeFormalEnrichedCanary(raw, agent.ID)
	if err != nil {
		return err
	}
	if receipt.CanonicalAuthority.Generation != formalCodexV8CanaryGeneration ||
		receipt.CanonicalAuthority.AuthorityScope != string(codexCanaryVerifiedFormal) ||
		receipt.CanonicalAuthority.ResponsesTransportRequirement != harness.TransportRequirementHTTPInference {
		return errors.New("Codex v8 canonical canary is pending or bound to a stale authority")
	}
	return validateFormalEnrichedCanary(receipt, agent, adapter, bundle)
}

func validateFormalLubanV8CanaryReceipt(raw []byte, agent harness.AgentSpec, adapter adapterBinding, bundle codexBundleBinding) error {
	if agent.ID != "luban" {
		return errors.New("Luban v8 canary validator received a different agent")
	}
	receipt, err := decodeFormalEnrichedCanary(raw, agent.ID)
	if err != nil {
		return err
	}
	return validateFormalEnrichedCanary(receipt, agent, adapter, bundle)
}

func validateFormalEnrichedCanary(receipt formalEnrichedCanaryReceipt, agent harness.AgentSpec, adapter adapterBinding, bundle codexBundleBinding) error {
	expectedRuntimeTree := bundle.TreeSHA256
	if agent.ID == "luban" {
		expectedRuntimeTree = LubanRuntimeTreeSHA256
	}
	if receipt.SchemaVersion != "agentic-bench/sandbox-canary-v4" || receipt.AgentKind != agent.ID ||
		receipt.BinarySHA256 != agent.BinarySHA256 || !lowerHexCommit(receipt.BaseCommit) ||
		!receipt.ControllerProxyReachable || receipt.ToolProxyReachable || receipt.CredentialInAgent ||
		receipt.AdapterSHA256 != adapter.SHA256 || receipt.BundleManifestSHA256 != bundle.ManifestSHA256 ||
		receipt.SourceBundleTreeSHA256 != bundle.TreeSHA256 || receipt.RuntimePayloadTreeSHA256 != expectedRuntimeTree ||
		receipt.ProviderCanaryTransport != "responses-http-inference-required" || receipt.EgressProxyImage != FrozenEgressProxyImage {
		return errors.New("formal execution canary differs from its frozen agent, bundle, sandbox, or HTTP identity")
	}
	if receipt.HTTPTransport.SchemaVersion != "agentic-bench/http-inference-transport-v1" ||
		receipt.HTTPTransport.Requirement != harness.TransportRequirementHTTPInference ||
		receipt.HTTPTransport.HTTPInferenceRequestCount != 2 || receipt.HTTPTransport.WebSocketUpgradeRequestCount != 0 ||
		receipt.HTTPTransport.WebSocketGenerationFrameCount != 0 || receipt.HTTPTransport.PrewarmRequestCount != 0 {
		return errors.New("formal execution canary does not prove HTTP-only inference")
	}
	var overlay map[string]json.RawMessage
	if err := validateStrictJSON(receipt.Overlay); err != nil || json.Unmarshal(receipt.Overlay, &overlay) != nil || len(overlay) == 0 {
		return errors.New("formal execution canary lacks its exact sandbox overlay")
	}
	if agent.ID == "luban" {
		if len(receipt.LubanRuntimeVersions) < 2 || slices.Contains(receipt.LubanRuntimeVersions, "") {
			return errors.New("Luban v8 canary lacks pinned runtime version evidence")
		}
	} else if len(receipt.LubanRuntimeVersions) != 0 {
		return errors.New("Codex v8 canary contains Luban runtime evidence")
	}
	if err := validateFormalEffectiveArgvReceipt(receipt, agent, adapter, bundle); err != nil {
		return err
	}
	policies, err := validateFormalHTTPRequestEvidence(receipt.ProviderCanaryRequests, agent)
	if err != nil {
		return err
	}
	return validateFormalCacheWire(receipt.CacheWire, policies)
}

func lowerHexCommit(value string) bool {
	if len(value) != 40 {
		return false
	}
	for _, character := range value {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return false
		}
	}
	return true
}

func validateFormalEffectiveArgvReceipt(canary formalEnrichedCanaryReceipt, agent harness.AgentSpec, adapter adapterBinding, bundle codexBundleBinding) error {
	receipt := canary.EffectiveArgvReceipt
	canonical, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	commandSHA, err := harness.HashCanonical(agent.Command.Argv)
	if err != nil {
		return err
	}
	safeArgv, err := expectedEffectiveArgv(agent, "", true)
	if err != nil {
		return err
	}
	safeSHA, err := harness.HashCanonical(safeArgv)
	if err != nil {
		return err
	}
	projection, err := projectEffectiveArgv(agent.ID, safeArgv)
	if err != nil {
		return err
	}
	projectionSHA, err := harness.HashCanonical(projection)
	if err != nil {
		return err
	}
	if sha256Hex(canonical) != canary.EffectiveArgvReceiptSHA256 || receipt.SchemaVersion != effectiveArgvSchemaVersion ||
		receipt.AgentKind != agent.ID || receipt.AdapterSHA256 != adapter.SHA256 || receipt.AdapterVersion != PinnedAdapterVersion ||
		receipt.BundleManifestSHA256 != bundle.ManifestSHA256 || receipt.BundleTreeSHA256 != bundle.TreeSHA256 ||
		receipt.SourceCommandArgvSHA256 != commandSHA || !slices.Equal(receipt.EffectiveArgv, safeArgv) ||
		receipt.EffectiveArgvSHA256 != safeSHA || !lowerHexSHA256(receipt.ExecutionArgvSHA256) ||
		!lowerHexSHA256(receipt.PrivateProxyBaseURLSHA256) || receipt.SemanticProjection != projection ||
		receipt.SemanticProjectionSHA256 != projectionSHA || projection.ResponsesTransportRequirement != harness.TransportRequirementHTTPInference {
		return errors.New("formal execution canary effective argv differs from the frozen adapter semantics")
	}
	return nil
}

type formalCodexHTTPRequest struct {
	codexLiteCanaryRequest
	Transport                 string                 `json:"transport"`
	PrewarmRequested          bool                   `json:"prewarm_requested"`
	CachePolicy               formalCachePolicy      `json:"cache_policy"`
	ToolDefinitions           []formalToolDefinition `json:"tool_definitions"`
	ToolCatalogSemanticSHA256 string                 `json:"tool_catalog_semantic_sha256"`
	ToolCatalogCanonicalBytes int64                  `json:"tool_catalog_canonical_bytes"`
}

type formalLubanHTTPRequest struct {
	lubanCanaryRequest
	Transport                          string                 `json:"transport"`
	PrewarmRequested                   bool                   `json:"prewarm_requested"`
	WebSocketUpgradeCountBeforeRequest int                    `json:"websocket_upgrade_count_before_request"`
	CachePolicy                        formalCachePolicy      `json:"cache_policy"`
	ToolDefinitions                    []formalToolDefinition `json:"tool_definitions"`
	ToolCatalogSemanticSHA256          string                 `json:"tool_catalog_semantic_sha256"`
	ToolCatalogCanonicalBytes          int64                  `json:"tool_catalog_canonical_bytes"`
}

type formalToolDefinition struct {
	Type             string `json:"type"`
	Name             string `json:"name"`
	DefinitionSHA256 string `json:"definition_sha256"`
	DefinitionBytes  int64  `json:"definition_bytes"`
}

func (definition *formalToolDefinition) UnmarshalJSON(raw []byte) error {
	type wire formalToolDefinition
	decoded, err := strictDecodeCanaryObject[wire](raw, "formal tool definition", "type", "name", "definition_sha256", "definition_bytes")
	if err != nil {
		return err
	}
	*definition = formalToolDefinition(decoded)
	return nil
}

func validateFormalHTTPRequestEvidence(rawRequests []json.RawMessage, agent harness.AgentSpec) ([]formalCachePolicy, error) {
	if len(rawRequests) != 2 {
		return nil, errors.New("formal execution canary must contain exactly two HTTP inference requests")
	}
	policies := make([]formalCachePolicy, 0, len(rawRequests))
	switch agent.ID {
	case "codex":
		var baselineDefinitions []formalToolDefinition
		var baselineSemantic string
		var baselineBytes int64
		fields := []string{
			"request_index", "model", "store", "reasoning_effort", "reasoning_context", "include_encrypted_reasoning", "stream",
			"transport", "prewarm_requested", "request_service_tier_present", "request_service_tier", "request_service_tier_canonical",
			"request_service_tier_source", "top_level_tool_count", "tool_catalog", "web_search_tool_present", "web_search_tool_count",
			"collaboration_namespace_present", "subagent_tool_present", "exec_cell_wait_present", "websocket_upgrade_count_before_request",
			"websocket_upgrade_header_present", "websocket_key_header_present", "responses_lite_header_present", "authorization_header_present",
			"originator", "user_agent_present", "previous_response_id_present", "custom_tool_output_count", "tool_output_exit_code",
			"response_model", "response_service_tier", "response_service_tier_canonical", "response_request_id_present", "response_usage", "cache_policy",
			"tool_definitions", "tool_catalog_semantic_sha256", "tool_catalog_canonical_bytes",
		}
		wantUsage := canaryUsage{InputTokens: 11, CachedInputTokens: 3, CacheWriteInputTokens: 2, OutputTokens: 5, ReasoningOutputTokens: 1}
		for index, raw := range rawRequests {
			request, err := strictDecodeCanaryObject[formalCodexHTTPRequest](raw, "formal Codex HTTP canary request", fields...)
			if err != nil {
				return nil, err
			}
			if request.RequestIndex != index || request.Transport != "http_sse" || request.PrewarmRequested ||
				request.Model != agent.Model.Model || request.Store == nil || *request.Store || request.ReasoningEffort != agent.Model.ReasoningEffort ||
				request.ReasoningContext != "all_turns" || !request.IncludeEncryptedReasoning || !request.Stream ||
				request.RequestServiceTierPresent || request.RequestServiceTier != nil || request.RequestServiceTierCanonical != harness.FormalServiceTier ||
				request.RequestServiceTierSource != serviceTierCanonicalizationRepresentation || request.TopLevelToolCount != 0 ||
				request.WebSearchToolPresent || request.WebSearchToolCount != 0 || request.CollaborationNamespacePresent || request.SubagentToolPresent ||
				!request.ExecCellWaitPresent || request.WebSocketUpgradeCountBeforeRequest != 0 || request.WebSocketUpgradeHeaderPresent || request.WebSocketKeyHeaderPresent ||
				!request.ResponsesLiteHeaderPresent || !request.AuthorizationHeaderPresent || request.Originator != "codex_exec" || !request.UserAgentPresent ||
				request.PreviousResponseIDPresent || request.ResponseModel != agent.Model.Model || request.ResponseServiceTier != harness.FormalServiceTier ||
				request.ResponseServiceTierCanonical != harness.FormalServiceTier || !request.ResponseRequestIDPresent || request.ResponseUsage != wantUsage ||
				!canaryToolCatalogMatchesManifest(request.ToolCatalog, agent.Model.ToolCatalog) {
				return nil, errors.New("formal Codex v8 request violates its frozen HTTP Responses Lite contract")
			}
			if index == 0 && (request.CustomToolOutputCount != 0 || request.ToolOutputExitCode != nil) {
				return nil, errors.New("first formal Codex v8 request unexpectedly contains a tool result")
			}
			if index == 1 && (request.CustomToolOutputCount != 1 || request.ToolOutputExitCode == nil || *request.ToolOutputExitCode != 0) {
				return nil, errors.New("second formal Codex v8 request lacks its exact successful exec result")
			}
			if err := validateFormalToolDefinitionEvidence(request.ToolDefinitions, request.ToolCatalogSemanticSHA256, request.ToolCatalogCanonicalBytes, agent.Model.ToolCatalog); err != nil {
				return nil, err
			}
			if index == 0 {
				baselineDefinitions = slices.Clone(request.ToolDefinitions)
				baselineSemantic, baselineBytes = request.ToolCatalogSemanticSHA256, request.ToolCatalogCanonicalBytes
			} else if !reflect.DeepEqual(request.ToolDefinitions, baselineDefinitions) || request.ToolCatalogSemanticSHA256 != baselineSemantic || request.ToolCatalogCanonicalBytes != baselineBytes {
				return nil, errors.New("formal Codex tool definition evidence changed between HTTP requests")
			}
			if err := validateFormalCachePolicy(request.CachePolicy); err != nil {
				return nil, err
			}
			policies = append(policies, request.CachePolicy)
		}
	case "luban":
		var baselineDefinitions []formalToolDefinition
		var baselineSemantic string
		var baselineBytes int64
		fields := []string{
			"request_index", "transport", "prewarm_requested", "websocket_upgrade_count_before_request", "model", "store", "reasoning_effort", "reasoning_context",
			"request_service_tier_present", "request_service_tier", "request_service_tier_canonical", "request_service_tier_source", "tool_names",
			"responses_lite_header", "additional_tools_prefixes", "previous_response_id_present", "response_model", "response_service_tier",
			"response_service_tier_canonical", "response_request_id_present", "cache_policy",
			"tool_definitions", "tool_catalog_semantic_sha256", "tool_catalog_canonical_bytes",
		}
		wantNames := make([]string, 0, len(agent.Model.ToolCatalog.Tools))
		for _, tool := range agent.Model.ToolCatalog.Tools {
			if tool.Type != "function" {
				return nil, errors.New("formal Luban manifest contains a non-function tool")
			}
			wantNames = append(wantNames, tool.Name)
		}
		for index, raw := range rawRequests {
			request, err := strictDecodeCanaryObject[formalLubanHTTPRequest](raw, "formal Luban HTTP canary request", fields...)
			if err != nil {
				return nil, err
			}
			if request.RequestIndex != index || request.Transport != "http_sse" || request.PrewarmRequested || request.WebSocketUpgradeCountBeforeRequest != 0 ||
				request.Model != agent.Model.Model || request.Store == nil || *request.Store || request.ReasoningEffort != agent.Model.ReasoningEffort ||
				(request.ReasoningContext != "" && request.ReasoningContext != "all_turns") || !request.RequestServiceTierPresent ||
				request.RequestServiceTier == nil || *request.RequestServiceTier != harness.FormalServiceTier || request.RequestServiceTierCanonical != harness.FormalServiceTier ||
				request.RequestServiceTierSource != "wire_explicit_default" || !slices.Equal(request.ToolNames, wantNames) || string(request.ResponsesLiteHeader) != "null" ||
				request.AdditionalToolsPrefixes != 0 || request.PreviousResponseIDPresent || request.ResponseModel != agent.Model.Model ||
				request.ResponseServiceTier != harness.FormalServiceTier || request.ResponseServiceTierCanonical != harness.FormalServiceTier || !request.ResponseRequestIDPresent {
				return nil, errors.New("formal Luban v8 request violates its frozen public HTTP Responses contract")
			}
			if err := validateFormalToolDefinitionEvidence(request.ToolDefinitions, request.ToolCatalogSemanticSHA256, request.ToolCatalogCanonicalBytes, agent.Model.ToolCatalog); err != nil {
				return nil, err
			}
			if index == 0 {
				baselineDefinitions = slices.Clone(request.ToolDefinitions)
				baselineSemantic, baselineBytes = request.ToolCatalogSemanticSHA256, request.ToolCatalogCanonicalBytes
			} else if !reflect.DeepEqual(request.ToolDefinitions, baselineDefinitions) || request.ToolCatalogSemanticSHA256 != baselineSemantic || request.ToolCatalogCanonicalBytes != baselineBytes {
				return nil, errors.New("formal Luban tool definition evidence changed between HTTP requests")
			}
			if err := validateFormalCachePolicy(request.CachePolicy); err != nil {
				return nil, err
			}
			policies = append(policies, request.CachePolicy)
		}
	default:
		return nil, errors.New("formal HTTP canary has an unsupported agent")
	}
	return policies, nil
}

func validateFormalToolDefinitionEvidence(definitions []formalToolDefinition, semanticSHA string, canonicalBytes int64, manifest harness.ToolCatalogSpec) error {
	if semanticSHA != manifest.SemanticSHA256 || !lowerHexSHA256(semanticSHA) || len(definitions) != len(manifest.Tools) || canonicalBytes <= 0 {
		return errors.New("formal execution canary tool catalog lacks its exact semantic binding")
	}
	seen := make(map[string]struct{}, len(definitions))
	var totalBytes int64
	for index, definition := range definitions {
		expected := manifest.Tools[index]
		identity := definition.Type + "\x00" + definition.Name
		if _, duplicate := seen[identity]; duplicate {
			return errors.New("formal execution canary tool catalog contains a duplicate identity")
		}
		seen[identity] = struct{}{}
		if definition.Type != expected.Type || definition.Name != expected.Name ||
			definition.DefinitionSHA256 != expected.DefinitionSHA256 || !lowerHexSHA256(definition.DefinitionSHA256) || definition.DefinitionBytes <= 0 {
			return errors.New("formal execution canary tool definition differs from the manifest order or exact hash")
		}
		if definition.DefinitionBytes > canonicalBytes-totalBytes {
			return errors.New("formal execution canary tool definition byte accounting overflowed its catalog")
		}
		totalBytes += definition.DefinitionBytes
	}
	if totalBytes != canonicalBytes {
		return errors.New("formal execution canary tool definition bytes differ from the catalog total")
	}
	return nil
}

func canaryToolCatalogMatchesManifest(got []canaryTool, want harness.ToolCatalogSpec) bool {
	if len(got) != len(want.Tools) {
		return false
	}
	for index, tool := range got {
		if tool.Name == nil || tool.Type != want.Tools[index].Type || *tool.Name != want.Tools[index].Name {
			return false
		}
	}
	return true
}

func validateFormalCachePolicy(policy formalCachePolicy) error {
	if !policy.Observed || !policy.ShapeValid || !policy.PromptCacheKeyPresent || !lowerHexSHA256(policy.PromptCacheKeySHA256) ||
		policy.PromptCacheBreakpointCount != len(policy.PromptCacheBreakpointPositionHashes) {
		return errors.New("formal execution canary cache policy lacks a stable content-free key lineage")
	}
	for _, digest := range policy.PromptCacheBreakpointPositionHashes {
		if !lowerHexSHA256(digest) {
			return errors.New("formal execution canary cache breakpoint position hash is invalid")
		}
	}
	if !policy.PromptCacheOptionsPresent && (policy.PromptCacheOptionsMode != "" || policy.PromptCacheOptionsTTLPresent || policy.PromptCacheOptionsTTL != "" || policy.PromptCacheOptionsTTLSeconds != nil) {
		return errors.New("formal execution canary cache options evidence is internally inconsistent")
	}
	if policy.PromptCacheOptionsTTLPresent && (policy.PromptCacheOptionsTTL == "" || policy.PromptCacheOptionsTTLSeconds == nil || *policy.PromptCacheOptionsTTLSeconds <= 0) {
		return errors.New("formal execution canary cache TTL evidence is invalid")
	}
	if !policy.PromptCacheRetentionPresent && policy.PromptCacheRetention != "" {
		return errors.New("formal execution canary cache retention evidence is internally inconsistent")
	}
	return nil
}

func validateFormalCacheWire(wire formalCacheWireReceipt, policies []formalCachePolicy) error {
	if len(policies) != 2 || !reflect.DeepEqual(policies[0], policies[1]) {
		return errors.New("formal execution canary cache lineage changed between HTTP requests")
	}
	policy := policies[0]
	expected := formalCacheWireReceipt{
		SchemaVersion: "agentic-bench/content-free-cache-wire-v1", ContentRetained: false,
		ObservedRequests: 2, ShapeValidRequests: 2, KeyPresentRequests: 2, UniqueKeyCount: 1,
		KeyTransitions: 0, FirstKeySHA256: policy.PromptCacheKeySHA256, Stable: true,
		PromptCacheOptionsModes:      []string{policy.PromptCacheOptionsMode, policy.PromptCacheOptionsMode},
		PromptCacheOptionsTTLs:       []string{policy.PromptCacheOptionsTTL, policy.PromptCacheOptionsTTL},
		PromptCacheOptionsTTLSeconds: []*int{policy.PromptCacheOptionsTTLSeconds, policy.PromptCacheOptionsTTLSeconds},
		PromptCacheRetentions:        []string{policy.PromptCacheRetention, policy.PromptCacheRetention},
		BreakpointCounts:             []int{policy.PromptCacheBreakpointCount, policy.PromptCacheBreakpointCount},
		BreakpointPositionHashes: [][]string{
			slices.Clone(policy.PromptCacheBreakpointPositionHashes), slices.Clone(policy.PromptCacheBreakpointPositionHashes),
		},
	}
	if !reflect.DeepEqual(wire, expected) {
		return errors.New("formal execution canary cache summary differs from its content-free request evidence")
	}
	return nil
}

func (backend *Backend) codexCanonicalCanarySnapshot() (formalCodexCanonicalCanaryBinding, error) {
	backend.mu.RLock()
	expected, adapter, bundle, manifest, ready, development := backend.codexCanary, backend.adapter, backend.bundle, backend.manifest, backend.ready, backend.development
	backend.mu.RUnlock()
	if !ready || expected.AgentID != "codex" || expected.Generation != formalCodexV8CanaryGeneration ||
		expected.TransportRequirement != harness.TransportRequirementHTTPInference || expected.Path == "" || !lowerHexSHA256(expected.SHA256) {
		return formalCodexCanonicalCanaryBinding{}, errors.New("Codex v8 canonical canary requested before verified preflight")
	}
	if development {
		// The development runner binds a manifest-declared, explicitly non-formal
		// placeholder into request/cost evidence. It is never exposed through the
		// formal Backend.Preflight snapshot or accepted by the formal runner.
		return expected, nil
	}
	agent, found := manifestAgent(manifest, "codex")
	if !found {
		return formalCodexCanonicalCanaryBinding{}, errors.New("Codex v8 canonical canary lost its manifest binding")
	}
	actual, err := resolvePinnedExecutionCanary(configuredExecutionCanary{
		AgentID: "codex", ReceiptPath: backend.config.CodexV8CanaryReceiptPath,
		ReceiptSHA256: backend.config.CodexV8CanaryReceiptSHA256,
	}, agent, adapter, bundle)
	if err != nil {
		return formalCodexCanonicalCanaryBinding{}, err
	}
	if actual != expected {
		return formalCodexCanonicalCanaryBinding{}, errors.New("Codex v8 canonical canary receipt changed after benchmark preflight")
	}
	return actual, nil
}
