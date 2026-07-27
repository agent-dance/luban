// Package pierbackend binds the generic benchmark contract to a frozen Pier
// runtime and DeepSWE-style Harbor tasks.
package pierbackend

import (
	"net/http"
	"time"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

const (
	InventorySchemaVersion      = "agentic-bench/pier-inventory-v2"
	AdapterImportPath           = "benchmark.agentic.pier.pinned_agent:PinnedCLIAgent"
	DockerEnvironmentImportPath = "benchmark.agentic.pier.docker_environment:AgenticBenchmarkDockerEnvironment"
	PinnedAdapterVersion        = "2.4.0"
	FrozenEgressProxyImage      = "ubuntu/squid@sha256:93d2d581a961f475ca5b23fe47fc3c3afadbe5849a6925a5b5435068502d7051"
)

// Config contains host-only authority. Provider credentials are never placed
// in this structure: RunAgent extracts one credential from the invocation's
// ephemeral allowlisted environment and keeps it only in proxy memory.
type Config struct {
	PierBinary                 string
	DatasetRepositoryRoot      string
	EvaluatorRepositoryRoot    string
	EvaluatorManifestPath      string
	InventoryLockPath          string
	PythonModuleRoot           string
	PrivateWorkRoot            string
	RegistryGatePath           string
	CodexV8CanaryReceiptPath   string
	CodexV8CanaryReceiptSHA256 string
	LubanV8CanaryReceiptPath   string
	LubanV8CanaryReceiptSHA256 string
	EgressProxyImage           string
	ProxyListenAddress         string
	ProxyAdvertiseHost         string
	ProviderUpstream           string
	ProviderCredentialEnv      string
	ProviderTransport          http.RoundTripper
	Now                        func() time.Time
}

type InventoryLock struct {
	SchemaVersion     string       `json:"schema_version"`
	DatasetCommit     string       `json:"dataset_commit"`
	Coverage          string       `json:"coverage"`
	UniverseTaskCount int          `json:"universe_task_count"`
	TaskIDs           []string     `json:"task_ids,omitempty"`
	Tasks             []LockedTask `json:"tasks"`
}

type LockedTask struct {
	ID                string `json:"id"`
	RelativePath      string `json:"relative_path"`
	BaseCommit        string `json:"base_commit"`
	ManifestSHA256    string `json:"manifest_sha256"`
	InstructionSHA256 string `json:"instruction_sha256"`
	Image             string `json:"image"`
	ImageDigest       string `json:"image_digest"`
}

func (task LockedTask) HarnessTask() harness.Task {
	return harness.Task{
		ID: task.ID, BaseCommit: task.BaseCommit,
		ManifestSHA256: task.ManifestSHA256, InstructionSHA256: task.InstructionSHA256,
		Image: task.Image, ImageDigest: task.ImageDigest,
	}
}

type sanitizedTrialResult struct {
	SchemaVersion   string                             `json:"schema_version"`
	TaskName        string                             `json:"task_name"`
	TrialName       string                             `json:"trial_name"`
	TaskChecksum    string                             `json:"task_checksum"`
	AgentName       string                             `json:"agent_name"`
	AgentVersion    string                             `json:"agent_version"`
	Provider        string                             `json:"provider,omitempty"`
	Model           string                             `json:"model,omitempty"`
	StartedAt       time.Time                          `json:"started_at"`
	FinishedAt      time.Time                          `json:"finished_at"`
	AgentStartedAt  time.Time                          `json:"agent_started_at,omitempty"`
	AgentFinishedAt time.Time                          `json:"agent_finished_at,omitempty"`
	VerifierStarted time.Time                          `json:"verifier_started_at,omitempty"`
	VerifierEnded   time.Time                          `json:"verifier_finished_at,omitempty"`
	ExceptionType   string                             `json:"exception_type,omitempty"`
	Rewards         map[string]float64                 `json:"rewards,omitempty"`
	Capture         *harness.SubmissionCaptureEvidence `json:"capture,omitempty"`
}

type runReceipt struct {
	SchemaVersion                                  string                           `json:"schema_version"`
	PierBinarySHA256                               string                           `json:"pier_binary_sha256"`
	PierArgvSHA256                                 string                           `json:"pier_argv_sha256"`
	MaterializedTaskSHA256                         string                           `json:"materialized_task_sha256"`
	AgentBinarySHA256                              string                           `json:"agent_binary_sha256,omitempty"`
	AgentBundleManifestSHA256                      string                           `json:"agent_bundle_manifest_sha256,omitempty"`
	AgentSourceBundleTreeSHA256                    string                           `json:"agent_source_bundle_tree_sha256,omitempty"`
	ProviderMeter                                  string                           `json:"provider_meter,omitempty"`
	VerifierEnvironment                            string                           `json:"verifier_environment"`
	EgressProxyImage                               string                           `json:"egress_proxy_image"`
	EgressProxyImageID                             string                           `json:"egress_proxy_image_id"`
	AdapterImportPath                              string                           `json:"adapter_import_path,omitempty"`
	AdapterVersion                                 string                           `json:"adapter_version,omitempty"`
	AdapterSHA256                                  string                           `json:"adapter_sha256,omitempty"`
	SourceCommandArgvSHA256                        string                           `json:"source_command_argv_sha256,omitempty"`
	EffectiveArgvSHA256                            string                           `json:"effective_argv_sha256,omitempty"`
	ExecutionArgvSHA256                            string                           `json:"execution_argv_sha256,omitempty"`
	EffectiveArgv                                  []string                         `json:"effective_argv,omitempty"`
	EffectiveArgvSemantics                         *effectiveArgvSemanticProjection `json:"effective_argv_semantic_projection,omitempty"`
	ProviderApprovedOrigin                         string                           `json:"provider_approved_origin,omitempty"`
	ProviderEndpointSemanticsSHA256                string                           `json:"provider_endpoint_semantics_sha256,omitempty"`
	ProviderObservationAuthority                   string                           `json:"provider_observation_authority,omitempty"`
	ProviderIdentityAttested                       bool                             `json:"provider_identity_attested"`
	ProviderTLSServerName                          string                           `json:"provider_tls_server_name,omitempty"`
	ProviderTLSObservationComplete                 bool                             `json:"provider_tls_observation_complete"`
	ProviderPreflightTLSPeerLeafCertSHA256         string                           `json:"provider_preflight_tls_peer_leaf_cert_sha256,omitempty"`
	ProviderPreflightTLSPeerSPKISHA256             string                           `json:"provider_preflight_tls_peer_spki_sha256,omitempty"`
	ProviderTLSBackedRoundCount                    int                              `json:"provider_tls_backed_round_count,omitempty"`
	ProviderTLSAbsentTransportFailureCount         int                              `json:"provider_tls_absent_transport_failure_count,omitempty"`
	ProviderTLSPeerObservations                    []providerTLSPeerObservation     `json:"provider_tls_peer_observations,omitempty"`
	CodexCanonicalCanaryGeneration                 string                           `json:"codex_canonical_canary_generation,omitempty"`
	CodexCanonicalCanaryReceiptRelativePath        string                           `json:"codex_canonical_canary_receipt_relative_path,omitempty"`
	CodexCanonicalCanaryReceiptSHA256              string                           `json:"codex_canonical_canary_receipt_sha256,omitempty"`
	ServiceTierCanonicalizationRepresentation      string                           `json:"service_tier_canonicalization_representation,omitempty"`
	ServiceTierCanonicalizationReceiptRelativePath string                           `json:"service_tier_canonicalization_receipt_relative_path,omitempty"`
	ServiceTierCanonicalizationReceiptSHA256       string                           `json:"service_tier_canonicalization_receipt_sha256,omitempty"`
	ServiceTierCanonicalizationBindingSHA256       string                           `json:"service_tier_canonicalization_binding_sha256,omitempty"`
	ServiceTierCanonicalizationStaticProofSHA256   string                           `json:"service_tier_canonicalization_static_proof_sha256,omitempty"`
	ServiceTierTransformationEvidenceSHA256        string                           `json:"service_tier_transformation_evidence_sha256,omitempty"`
	ServiceTierTransformedProviderRoundCount       int                              `json:"service_tier_transformed_provider_round_count,omitempty"`
}

type providerTLSPeerObservation struct {
	TLSPeerLeafCertSHA256 string    `json:"tls_peer_leaf_cert_sha256"`
	TLSPeerSPKISHA256     string    `json:"tls_peer_spki_sha256"`
	FirstObservedAt       time.Time `json:"first_observed_at"`
	LastObservedAt        time.Time `json:"last_observed_at"`
	RoundCount            int       `json:"round_count"`
}
