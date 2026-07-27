// Package harness defines the reproducible Agentic Coding benchmark protocol.
//
// It deliberately does not know how Codex, Luban, Pier, or Harbor are launched.
// Those details live behind Backend so that the agent process cannot receive the
// oracle solution or the verifier filesystem by construction.
package harness

import (
	"context"
	"time"
)

const SchemaVersion = "agentic-bench/v2"

// Manifest is the immutable input to an experiment. LoadManifest returns the
// SHA-256 of its exact bytes; runners must bind resumable state to that hash.
type Manifest struct {
	SchemaVersion    string               `json:"schema_version"`
	Experiment       ExperimentSpec       `json:"experiment"`
	Dataset          SourcePin            `json:"dataset"`
	Evaluator        EvaluatorSpec        `json:"evaluator"`
	Agents           []AgentSpec          `json:"agents"`
	Selection        SelectionSpec        `json:"selection"`
	Scheduling       SchedulingSpec       `json:"scheduling"`
	Scoring          ScoringSpec          `json:"scoring"`
	ProviderEndpoint ProviderEndpointSpec `json:"provider_endpoint"`
	Environment      EnvironmentSpec      `json:"environment"`
	Timeouts         TimeoutSpec          `json:"timeouts"`
	Resources        ResourceSpec         `json:"resources"`
	Pricing          PricingCatalog       `json:"pricing"`
	Artifacts        ArtifactSpec         `json:"artifacts"`
	Oracle           OracleSpec           `json:"oracle"`
}

type ExperimentSpec struct {
	ID          string `json:"id"`
	Description string `json:"description,omitempty"`
}

// SourcePin freezes both Git identity and the content-addressed inventory
// covered by the run's immutable backend lock. A full run covers the whole
// declared universe; an explicit pilot covers exactly its preregistered task
// IDs. The repository commit alone is insufficient when mutable OCI tags are
// involved.
type SourcePin struct {
	Name                    string `json:"name"`
	Repository              string `json:"repository"`
	Commit                  string `json:"commit"`
	Root                    string `json:"root"`
	TreeSHA256              string `json:"tree_sha256"`
	ManifestSHA256          string `json:"manifest_sha256"`
	InventoryLockFileSHA256 string `json:"inventory_lock_file_sha256,omitempty"`
}

type EvaluatorSpec struct {
	SourcePin
	Protocol       string `json:"protocol"`
	MinimumVersion string `json:"minimum_version"`
	BinarySHA256   string `json:"binary_sha256"`
}

type AgentSpec struct {
	ID              string              `json:"id"`
	Binary          string              `json:"binary"`
	BinarySHA256    string              `json:"binary_sha256"`
	SourceSnapshot  *AgentSourceSpec    `json:"source_snapshot,omitempty"`
	Command         CommandSpec         `json:"command"`
	Model           ModelRequestSpec    `json:"model"`
	ExecutionCanary ExecutionCanarySpec `json:"execution_canary"`
	RequestEvidence RequestEvidenceSpec `json:"request_evidence"`
}

// ExecutionCanarySpec pins a current-generation, transport-specific live
// canary. Historical canaries cannot silently authorize a later formal run.
type ExecutionCanarySpec struct {
	Generation    string `json:"generation"`
	ReceiptSHA256 string `json:"receipt_sha256"`
}

// AgentSourceSpec freezes the exact source tree used to build an agent without
// requiring (or pretending) that its development worktree is clean. TreeOID,
// PatchSHA256, and ArchiveSHA256 are all derived from a temporary Git index;
// the user's real index is never read as authority or modified.
type AgentSourceSpec struct {
	Worktree               string           `json:"worktree"`
	BaseCommit             string           `json:"base_commit"`
	TreeOID                string           `json:"tree_oid"`
	PatchSHA256            string           `json:"patch_sha256"`
	ArchiveSHA256          string           `json:"archive_sha256"`
	PathPolicy             SourcePathPolicy `json:"path_policy"`
	PathPolicySHA256       string           `json:"path_policy_sha256"`
	ExclusionReceiptSHA256 string           `json:"exclusion_receipt_sha256"`
	BuildReceipt           string           `json:"build_receipt"`
	BuildReceiptSHA256     string           `json:"build_receipt_sha256"`
}

type SourcePathPolicy struct {
	SchemaVersion    string   `json:"schema_version"`
	ExcludedPrefixes []string `json:"excluded_prefixes"`
}

type SourceExclusionReceipt struct {
	SchemaVersion    string           `json:"schema_version"`
	PathPolicy       SourcePathPolicy `json:"path_policy"`
	PathPolicySHA256 string           `json:"path_policy_sha256"`
	Applied          bool             `json:"applied"`
	Implementation   string           `json:"implementation"`
}

// CommandSpec is argv-based; shell command strings are intentionally not part
// of the protocol. Adapters may replace the documented tokens in individual
// arguments but must never evaluate the result through a shell.
type CommandSpec struct {
	Argv        []string `json:"argv"`
	RequiredEnv []string `json:"required_env"`
}

type ModelRequestSpec struct {
	Provider        string `json:"provider"`
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoning_effort"`
	// ServiceTier freezes the effective launcher configuration. RequestEncoding
	// separately records whether the client emits default on the wire or applies
	// a frozen, source-proven canonical omission.
	ServiceTier                string          `json:"service_tier"`
	ServiceTierRequestEncoding string          `json:"service_tier_request_encoding"`
	TransportRequirement       string          `json:"transport_requirement"`
	ToolCatalog                ToolCatalogSpec `json:"tool_catalog"`
}

// ToolCatalogSpec freezes the ordered, exact client-local tool surface. The
// semantic digest binds the complete normalized definitions; each per-tool
// digest prevents identity-only allowlists from accepting changed schemas.
type ToolCatalogSpec struct {
	SchemaVersion  string             `json:"schema_version"`
	SemanticSHA256 string             `json:"semantic_sha256"`
	Tools          []ToolIdentitySpec `json:"tools"`
}

type ToolIdentitySpec struct {
	Type             string `json:"type"`
	Name             string `json:"name"`
	DefinitionSHA256 string `json:"definition_sha256"`
}

// RequestEvidenceSpec points to normalized provider-request JSONL produced by
// the adapter. Raw UI labels or CLI flags are not accepted as model evidence.
type RequestEvidenceSpec struct {
	RelativePath string `json:"relative_path"`
	Required     bool   `json:"required"`
}

type SelectionSpec struct {
	Mode        string   `json:"mode"`
	TaskIDs     []string `json:"task_ids,omitempty"`
	SampleCount int      `json:"sample_count,omitempty"`
	SampleSeed  uint64   `json:"sample_seed,omitempty"`
	// ExpectedTaskCount is the size of the benchmark universe, even when an
	// explicit pilot lock contains fewer tasks.
	ExpectedTaskCount int `json:"expected_task_count"`
}

type SchedulingSpec struct {
	PairAgents       bool   `json:"pair_agents"`
	Seed             uint64 `json:"seed"`
	Repetitions      int    `json:"repetitions"`
	MaxParallelPairs int    `json:"max_parallel_pairs"`
}

const ScoringProfileDeepSWEV11PublicCI = "deepswe-v1.1-public-ci"

// ScoringSpec fixes both the public scoring rules and the direction of the
// paired comparison. A report must never infer which agent is the challenger
// from array order or a display label.
type ScoringSpec struct {
	Profile           string `json:"profile"`
	BaselineAgentID   string `json:"baseline_agent_id"`
	ChallengerAgentID string `json:"challenger_agent_id"`
}

// EnvironmentSpec is an allowlist, not a denylist. Environment values are
// never serialized into manifests, state, logs, or artifact ledgers.
type EnvironmentSpec struct {
	HostEnvAllowlist    []string `json:"host_env_allowlist"`
	AgentEgressHosts    []string `json:"agent_egress_hosts"`
	TaskNetworkMode     string   `json:"task_network_mode"`
	VerifierNetworkMode string   `json:"verifier_network_mode"`
}

type ProviderEndpointSpec struct {
	ApprovedOrigin  string                    `json:"approved_origin"`
	Semantics       ProviderEndpointSemantics `json:"semantics"`
	SemanticsSHA256 string                    `json:"semantics_sha256"`
}

type ProviderEndpointSemantics struct {
	SchemaVersion            string `json:"schema_version"`
	APIProtocol              string `json:"api_protocol"`
	ObservationAuthority     string `json:"observation_authority"`
	ProviderIdentityAttested bool   `json:"provider_identity_attested"`
	TLSRequired              bool   `json:"tls_required"`
	WebSocketAllowed         bool   `json:"websocket_allowed"`
}

type TimeoutSpec struct {
	SetupSeconds    int `json:"setup_seconds"`
	AgentSeconds    int `json:"agent_seconds"`
	VerifierSeconds int `json:"verifier_seconds"`
	TeardownSeconds int `json:"teardown_seconds"`
}

type ResourceSpec struct {
	CPUs              int                   `json:"cpus"`
	MemoryMB          int                   `json:"memory_mb"`
	StorageMB         int                   `json:"storage_mb"`
	GPUs              int                   `json:"gpus"`
	HostStorageGuard  HostStorageGuardSpec  `json:"host_storage_guard"`
	GuestStorageGuard GuestStorageGuardSpec `json:"guest_storage_guard"`
}

type HostStorageGuardSpec struct {
	SchemaVersion                     string `json:"schema_version"`
	AdmissionMinimumAvailableBytes    uint64 `json:"admission_minimum_available_bytes"`
	RuntimeWarningBelowAvailableBytes uint64 `json:"runtime_warning_below_available_bytes"`
	RuntimeHardFloorAvailableBytes    uint64 `json:"runtime_hard_floor_available_bytes"`
	PollIntervalMS                    int    `json:"poll_interval_ms"`
	MonitoringGapThresholdMS          int    `json:"monitoring_gap_threshold_ms"`
	Measurement                       string `json:"measurement"`
}

type GuestStorageGuardSpec struct {
	SchemaVersion                   string `json:"schema_version"`
	StartMinimumAvailableBytes      uint64 `json:"start_minimum_available_bytes"`
	RuntimeAbortBelowAvailableBytes uint64 `json:"runtime_abort_below_available_bytes"`
	PollIntervalMS                  int    `json:"poll_interval_ms"`
	MonitoringGapThresholdMS        int    `json:"monitoring_gap_threshold_ms"`
	Measurement                     string `json:"measurement"`
}

type PricingCatalog struct {
	Currency   string `json:"currency"`
	UnitTokens int64  `json:"unit_tokens"`
	// EffectiveAt is the documented availability/effective date of the pinned
	// price; ObservedAt is when this benchmark froze the public source.
	EffectiveAt time.Time     `json:"effective_at"`
	ObservedAt  time.Time     `json:"observed_at"`
	SourceURL   string        `json:"source_url"`
	Rates       []PricingRate `json:"rates"`
}

type PricingRate struct {
	Provider                  string        `json:"provider"`
	Model                     string        `json:"model"`
	Input                     float64       `json:"input"`
	CachedInput               float64       `json:"cached_input"`
	Output                    float64       `json:"output"`
	CacheWriteInputMultiplier float64       `json:"cache_write_input_multiplier"`
	RequestTiers              []PricingTier `json:"request_tiers"`
}

// PricingTier applies to the whole provider request when its input token count
// is strictly greater than ThresholdInputTokens. Tiers are evaluated in
// ascending order, so a later matching threshold replaces an earlier one.
type PricingTier struct {
	Name                  string  `json:"name"`
	ThresholdInputTokens  int64   `json:"threshold_input_tokens"`
	InputMultiplier       float64 `json:"input_multiplier"`
	CachedInputMultiplier float64 `json:"cached_input_multiplier"`
	OutputMultiplier      float64 `json:"output_multiplier"`
}

type ArtifactSpec struct {
	Root               string `json:"root"`
	LedgerRelativePath string `json:"ledger_relative_path"`
	StateRelativePath  string `json:"state_relative_path"`
	CaptureBinaryDiff  bool   `json:"capture_binary_diff"`
	CaptureUntracked   bool   `json:"capture_untracked"`
}

type OracleSpec struct {
	Required            bool   `json:"required"`
	SeparateEnvironment bool   `json:"separate_environment"`
	SolutionRoot        string `json:"solution_root"`
}

type Task struct {
	ID                string `json:"id"`
	BaseCommit        string `json:"base_commit"`
	ManifestSHA256    string `json:"manifest_sha256"`
	Image             string `json:"image"`
	ImageDigest       string `json:"image_digest"`
	InstructionSHA256 string `json:"instruction_sha256"`
}

type SourceSnapshot struct {
	Commit         string `json:"commit"`
	TreeSHA256     string `json:"tree_sha256"`
	RawTreeSHA256  string `json:"raw_tree_sha256"`
	ManifestSHA256 string `json:"manifest_sha256"`
}

// InventoryLockSnapshot binds the exact archived task-lock bytes to the
// normalized inventory identity used by BuildPlan. The archive is the only
// inventory authority after the first run starts; mutable controller paths are
// deliberately absent from this receipt.
type InventoryLockSnapshot struct {
	RelativePath        string `json:"relative_path"`
	FileSHA256          string `json:"file_sha256"`
	SchemaVersion       string `json:"schema_version"`
	HashAlgorithm       string `json:"hash_algorithm"`
	DatasetCommit       string `json:"dataset_commit"`
	Coverage            string `json:"coverage"`
	TaskCount           int    `json:"task_count"`
	UniverseTaskCount   int    `json:"universe_task_count"`
	TaskInventorySHA256 string `json:"task_inventory_sha256"`
}

type BackendSnapshot struct {
	Dataset                SourceSnapshot            `json:"dataset"`
	Evaluator              SourceSnapshot            `json:"evaluator"`
	InventoryLock          InventoryLockSnapshot     `json:"inventory_lock"`
	EvaluatorVersion       string                    `json:"evaluator_version"`
	EvaluatorBinarySHA256  string                    `json:"evaluator_binary_sha256"`
	InventoryCoverage      string                    `json:"inventory_coverage"`
	InventoryTaskCount     int                       `json:"inventory_task_count"`
	UniverseTaskCount      int                       `json:"universe_task_count"`
	AgentNetworkDeny       bool                      `json:"agent_network_deny"`
	VerifierNetworkDeny    bool                      `json:"verifier_network_deny"`
	NetworkAttestation     string                    `json:"network_attestation"`
	EgressProxyImage       string                    `json:"egress_proxy_image"`
	EgressProxyImageID     string                    `json:"egress_proxy_image_id"`
	AdapterImportPath      string                    `json:"adapter_import_path"`
	AdapterVersion         string                    `json:"adapter_version"`
	AdapterSHA256          string                    `json:"adapter_sha256"`
	ProviderEndpoint       ProviderEndpointSnapshot  `json:"provider_endpoint"`
	AgentExecutionCanaries []ExecutionCanarySnapshot `json:"agent_execution_canaries"`
	StorageEnforcement     string                    `json:"storage_enforcement"`
	HostStorageGuard       HostStorageGuardSpec      `json:"host_storage_guard"`
	GuestStorageGuard      GuestStorageGuardSpec     `json:"guest_storage_guard"`
	StoragePreflight       StorageAdmissionReceipt   `json:"storage_preflight"`
}

type ExecutionCanarySnapshot struct {
	AgentID              string `json:"agent_id"`
	Generation           string `json:"generation"`
	TransportRequirement string `json:"transport_requirement"`
	ReceiptSHA256        string `json:"receipt_sha256"`
}

type ProviderEndpointSnapshot struct {
	ApprovedOrigin        string `json:"approved_origin"`
	SemanticsSHA256       string `json:"semantics_sha256"`
	TLSServerName         string `json:"tls_server_name"`
	TLSVerified           bool   `json:"tls_verified"`
	TLSPeerLeafCertSHA256 string `json:"tls_peer_leaf_cert_sha256"`
	TLSPeerSPKISHA256     string `json:"tls_peer_spki_sha256"`
}

type PlanEntry struct {
	Ordinal    int    `json:"ordinal"`
	PairID     string `json:"pair_id"`
	TaskID     string `json:"task_id"`
	AgentID    string `json:"agent_id"`
	Repetition int    `json:"repetition"`
}

type RunPlan struct {
	SchemaVersion  string      `json:"schema_version"`
	ManifestSHA256 string      `json:"manifest_sha256"`
	Entries        []PlanEntry `json:"entries"`
}

type AgentSnapshot struct {
	AgentID      string               `json:"agent_id"`
	BinarySHA256 string               `json:"binary_sha256"`
	Source       *AgentSourceSnapshot `json:"source,omitempty"`
	CapturedAt   time.Time            `json:"captured_at"`
}

type AgentSourceSnapshot struct {
	BaseCommit             string           `json:"base_commit"`
	TreeOID                string           `json:"tree_oid"`
	PatchSHA256            string           `json:"patch_sha256"`
	ArchiveSHA256          string           `json:"archive_sha256"`
	PathPolicy             SourcePathPolicy `json:"path_policy"`
	PathPolicySHA256       string           `json:"path_policy_sha256"`
	ExclusionReceiptSHA256 string           `json:"exclusion_receipt_sha256"`
	BuildReceiptSHA256     string           `json:"build_receipt_sha256"`
}

// AgentBuildReceipt is created by the frozen build step outside the source
// worktree. It binds the executed build command, resulting binary, and all
// three independent source identities into one immutable, hashed record.
type AgentBuildReceipt struct {
	SchemaVersion          string           `json:"schema_version"`
	AgentID                string           `json:"agent_id"`
	BaseCommit             string           `json:"base_commit"`
	TreeOID                string           `json:"tree_oid"`
	PatchSHA256            string           `json:"patch_sha256"`
	ArchiveSHA256          string           `json:"archive_sha256"`
	PathPolicy             SourcePathPolicy `json:"path_policy"`
	PathPolicySHA256       string           `json:"path_policy_sha256"`
	ExclusionReceiptSHA256 string           `json:"exclusion_receipt_sha256"`
	BinarySHA256           string           `json:"binary_sha256"`
	BuildArgv              []string         `json:"build_argv"`
	Toolchain              string           `json:"toolchain"`
	BuiltAt                time.Time        `json:"built_at"`
}

// PublicTaskView contains only information permitted in an agent sandbox. It
// intentionally has no verifier, tests, solution, reward, or oracle fields.
type PublicTaskView struct {
	ID                string
	BaseCommit        string
	InstructionSHA256 string
	InstructionPath   string
	WorkspacePath     string
	Image             string
	ImageDigest       string
}

type AgentInvocation struct {
	PlanEntry        PlanEntry
	Agent            AgentSpec
	Task             PublicTaskView
	ArtifactDir      string
	Environment      []string
	Timeout          time.Duration
	Resources        ResourceSpec
	AllowedEgress    []string
	StorageAdmission StorageAdmissionReceipt
}

type AgentExecution struct {
	Lifecycle       AttemptLifecycle `json:"lifecycle"`
	ExitClass       string           `json:"exit_class"`
	ExitCode        int              `json:"exit_code"`
	StartedAt       time.Time        `json:"started_at"`
	FinishedAt      time.Time        `json:"finished_at"`
	TrialStartedAt  time.Time        `json:"trial_started_at"`
	TrialFinishedAt time.Time        `json:"trial_finished_at"`
	// SubmissionPatch is the exact official model.patch produced by the
	// benchmark's unmodified pre_artifacts hook. AuditWorkspacePatch is a
	// content-complete temporary-index capture and is never passed to the
	// verifier.
	SubmissionPatch             string                              `json:"official_submission_patch"`
	AuditWorkspacePatch         string                              `json:"audit_workspace_patch"`
	Capture                     SubmissionCaptureEvidence           `json:"capture"`
	EvidencePath                string                              `json:"evidence_path"`
	EvidenceRunIdentity         string                              `json:"evidence_run_identity"`
	ProviderEvidence            ProviderEvidenceSeal                `json:"provider_evidence"`
	ServiceTierCanonicalization ServiceTierCanonicalizationEvidence `json:"service_tier_canonicalization"`
	StorageEvidence             StorageResourceEvidence             `json:"storage_evidence"`
	GuestStorageEvidence        []GuestStorageResourceEvidence      `json:"guest_storage_evidence"`
	TerminalEvidence            AgentTerminalEvidence               `json:"terminal_evidence"`
	Verification                *VerificationResult                 `json:"verification,omitempty"`
}

type ServiceTierCanonicalizationEvidence struct {
	SchemaVersion                string `json:"schema_version"`
	Representation               string `json:"representation"`
	ReceiptRelativePath          string `json:"receipt_relative_path"`
	ReceiptSHA256                string `json:"receipt_sha256"`
	BindingSHA256                string `json:"binding_sha256"`
	StaticProofSHA256            string `json:"static_proof_sha256"`
	TransformationEvidenceSHA256 string `json:"transformation_evidence_sha256"`
	TransformedRoundCount        uint64 `json:"transformed_round_count"`
}

type StorageAdmissionRequest struct {
	ControllerRoot string
	ArtifactRoot   string
	Resources      ResourceSpec
}

// StorageAdmissionReceipt is measured before a raw attempt slot or provider
// WAL is reserved. It is host-wide capacity evidence, not agent attribution.
type StorageAdmissionReceipt struct {
	SchemaVersion     string                              `json:"schema_version"`
	Stage             string                              `json:"stage"`
	Enforcement       string                              `json:"enforcement"`
	DeclaredStorageMB int                                 `json:"declared_storage_mb"`
	Guard             HostStorageGuardSpec                `json:"host_storage_guard"`
	Authority         string                              `json:"authority"`
	ObservedAt        time.Time                           `json:"observed_at"`
	Filesystems       []StorageAdmissionFilesystemReceipt `json:"filesystems"`
	Passed            bool                                `json:"passed"`
	Warning           bool                                `json:"warning"`
}

type StorageAdmissionFilesystemReceipt struct {
	Group                int       `json:"group"`
	Roles                []string  `json:"roles"`
	VolumeIdentitySHA256 string    `json:"volume_identity_sha256"`
	FilesystemType       string    `json:"filesystem_type"`
	DeviceRoleCount      int       `json:"device_role_count"`
	ObservedAt           time.Time `json:"observed_at"`
	MonotonicOffsetMS    int64     `json:"monotonic_offset_ms"`
	BlockSizeBytes       uint64    `json:"block_size_bytes"`
	TotalBytes           uint64    `json:"total_bytes"`
	AvailableBytes       uint64    `json:"available_bytes"`
	UsedBytes            uint64    `json:"used_bytes"`
}

// StorageResourceEvidence binds the content-free host Statfs receipt archived
// for one physical trial. Deltas are diagnostic only and never attributed to
// an agent because other host workloads can share the same filesystem.
type StorageResourceEvidence struct {
	SchemaVersion       string                 `json:"schema_version"`
	ReceiptRelativePath string                 `json:"receipt_relative_path"`
	ReceiptSHA256       string                 `json:"receipt_sha256"`
	Receipt             StorageResourceReceipt `json:"receipt"`
}

type StorageResourceReceipt struct {
	SchemaVersion             string                            `json:"schema_version"`
	Enforcement               string                            `json:"enforcement"`
	DeclaredStorageMB         int                               `json:"declared_storage_mb"`
	Guard                     HostStorageGuardSpec              `json:"host_storage_guard"`
	Authority                 string                            `json:"authority"`
	Admission                 StorageAdmissionReceipt           `json:"admission"`
	StartedAt                 time.Time                         `json:"started_at"`
	FinishedAt                time.Time                         `json:"finished_at"`
	ProviderWALStartedAt      time.Time                         `json:"provider_wal_started_at"`
	ProviderWALStartedDeltaMS int64                             `json:"provider_wal_started_delta_ms"`
	FinishedDeltaMS           int64                             `json:"finished_delta_ms"`
	Filesystems               []StorageRuntimeFilesystemReceipt `json:"filesystems"`
	Status                    string                            `json:"status"`
}

type StorageRuntimeFilesystemReceipt struct {
	Group                  int                   `json:"group"`
	Roles                  []string              `json:"roles"`
	VolumeIdentitySHA256   string                `json:"volume_identity_sha256"`
	FilesystemType         string                `json:"filesystem_type"`
	DeviceRoleCount        int                   `json:"device_role_count"`
	BlockSizeBytes         uint64                `json:"block_size_bytes"`
	TotalBytes             uint64                `json:"total_bytes"`
	AvailableBeforeBytes   uint64                `json:"available_before_bytes"`
	AvailableAfterBytes    uint64                `json:"available_after_bytes"`
	MinimumAvailableBytes  uint64                `json:"minimum_available_bytes"`
	UsedBeforeBytes        uint64                `json:"used_before_bytes"`
	UsedAfterBytes         uint64                `json:"used_after_bytes"`
	MaximumUsedBytes       uint64                `json:"maximum_used_bytes"`
	Samples                uint64                `json:"samples"`
	WarningSamples         uint64                `json:"warning_samples"`
	MaximumCompletionGapMS int64                 `json:"maximum_completion_gap_ms"`
	SamplePoints           []StorageStatfsSample `json:"sample_points"`
}

type StorageStatfsSample struct {
	ObservedAt     time.Time `json:"observed_at"`
	StartDeltaMS   int64     `json:"start_delta_ms"`
	EndDeltaMS     int64     `json:"end_delta_ms"`
	AvailableBytes uint64    `json:"available_bytes"`
	UsedBytes      uint64    `json:"used_bytes"`
}

type GuestStorageResourceEvidence struct {
	SchemaVersion       string                      `json:"schema_version"`
	ReceiptRelativePath string                      `json:"receipt_relative_path"`
	ReceiptSHA256       string                      `json:"receipt_sha256"`
	Receipt             GuestStorageResourceReceipt `json:"receipt"`
}

type GuestStorageResourceReceipt struct {
	SchemaVersion             string                          `json:"schema_version"`
	Phase                     string                          `json:"phase"`
	SessionIdentitySHA256     string                          `json:"session_identity_sha256"`
	ContainerIdentitySHA256   string                          `json:"container_identity_sha256"`
	ConfiguredCapacityBytes   uint64                          `json:"configured_capacity_bytes"`
	Enforcement               string                          `json:"enforcement"`
	DeclaredStorageMB         int                             `json:"declared_storage_mb"`
	Guard                     GuestStorageGuardSpec           `json:"guest_storage_guard"`
	Authority                 string                          `json:"authority"`
	StartedAt                 time.Time                       `json:"started_at"`
	FinishedAt                time.Time                       `json:"finished_at"`
	ProviderWALStartedAt      time.Time                       `json:"provider_wal_started_at"`
	ProviderWALStartedDeltaMS int64                           `json:"provider_wal_started_delta_ms"`
	FinishedDeltaMS           int64                           `json:"finished_delta_ms"`
	Filesystems               []GuestStorageFilesystemReceipt `json:"filesystems"`
	Status                    string                          `json:"status"`
}

type GuestStorageFilesystemReceipt struct {
	Group                  int                   `json:"group"`
	Roles                  []string              `json:"roles"`
	VolumeIdentitySHA256   string                `json:"volume_identity_sha256"`
	FilesystemType         string                `json:"filesystem_type"`
	DeviceRoleCount        int                   `json:"device_role_count"`
	BlockSizeBytes         uint64                `json:"block_size_bytes"`
	TotalBytes             uint64                `json:"total_bytes"`
	MinimumAvailableBytes  uint64                `json:"minimum_available_bytes"`
	MaximumUsedBytes       uint64                `json:"maximum_used_bytes"`
	MaximumCompletionGapMS int64                 `json:"maximum_completion_gap_ms"`
	Samples                []StorageStatfsSample `json:"samples"`
}

type AttemptLifecycle struct {
	SchemaVersion        string    `json:"schema_version"`
	RunIdentity          string    `json:"run_identity"`
	ControllerStartedAt  time.Time `json:"controller_started_at"`
	ControllerFinishedAt time.Time `json:"controller_finished_at,omitempty"`
	ProviderAttemptState string    `json:"provider_attempt_state"`
	ProviderAttemptCount uint64    `json:"provider_attempt_count"`
	Recovered            bool      `json:"recovered"`
}

// ProviderEvidenceSeal binds the normalized usage ledger to the proxy's raw
// append-only chain, durable attempt-start WAL, and final seal. The raw files
// contain content-free evidence only and remain inside the attempt artifact.
type ProviderEvidenceSeal struct {
	RawEvidencePath       string `json:"raw_evidence_path"`
	AttemptJournalPath    string `json:"attempt_journal_path"`
	SealPath              string `json:"seal_path"`
	RawEvidenceSHA256     string `json:"raw_evidence_sha256"`
	AttemptJournalSHA256  string `json:"attempt_journal_sha256"`
	SealSHA256            string `json:"seal_sha256"`
	StartedAttemptCount   uint64 `json:"started_attempt_count"`
	PersistedAttemptCount uint64 `json:"persisted_attempt_count"`
	RecordCount           uint64 `json:"record_count"`
	LastEvidenceHash      string `json:"last_evidence_hash"`
}

// AgentTerminalEvidence prevents free-form stderr or unknown process states
// from being reclassified as a benchmark timeout/context failure.
type AgentTerminalEvidence struct {
	SchemaVersion  string `json:"schema_version"`
	Source         string `json:"source"`
	Code           string `json:"code"`
	EvidenceSHA256 string `json:"evidence_sha256"`
}

type SubmissionCaptureEvidence struct {
	Method                    string `json:"method"`
	BaseCommit                string `json:"base_commit"`
	PatchSHA256               string `json:"patch_sha256"`
	AuditPatchSHA256          string `json:"audit_patch_sha256"`
	UncommittedChangesPresent bool   `json:"uncommitted_changes_present"`
	IncludesTracked           bool   `json:"includes_tracked"`
	IncludesUntracked         bool   `json:"includes_untracked"`
	IncludesBinary            bool   `json:"includes_binary"`
}

type VerificationResult struct {
	ProtocolValid bool `json:"protocol_valid"`
	// Reward is the effective benchmark reward. RawReward is populated when a
	// verifier pass is overridden by a scored agent terminal failure such as a
	// timeout or context exhaustion.
	Reward        float64            `json:"reward"`
	RawReward     *float64           `json:"raw_reward,omitempty"`
	Scores        map[string]float64 `json:"scores,omitempty"`
	ArtifactPaths []string           `json:"artifact_paths"`
}

type OracleRequest struct {
	TaskID      string
	ArtifactDir string
	Timeout     time.Duration
	Resources   ResourceSpec
}

// Backend is the only component allowed to know both the public task source
// and the held-out verifier. RunAgent owns one Pier trial whose agent and
// verifier phases use distinct pristine environments; RecoverAgent is a
// sealed-artifact read and must never launch external work.
type Backend interface {
	Preflight(ctx context.Context, manifest Manifest) (BackendSnapshot, error)
	Inventory(ctx context.Context, dataset SourcePin) ([]Task, error)
	PublicTask(ctx context.Context, taskID string) (PublicTaskView, error)
	VerifyOracle(ctx context.Context, request OracleRequest) (VerificationResult, error)
	RunAgent(ctx context.Context, invocation AgentInvocation) (AgentExecution, error)
	// RecoverAgent must only read an already-sealed attempt. It must never start
	// a model, provider request, task container, or verifier.
	RecoverAgent(ctx context.Context, invocation AgentInvocation) (AgentExecution, error)
}

// StorageAdmissionBackend performs the host-wide free-space check before the
// runner reserves an immutable attempt slot. An admission error pauses the run
// without consuming a slot or creating provider WAL.
type StorageAdmissionBackend interface {
	CheckStorageAdmission(ctx context.Context, request StorageAdmissionRequest) (StorageAdmissionReceipt, error)
}

type HostStoragePreflightBackend interface {
	CheckHostStoragePreflight(ctx context.Context, request StorageAdmissionRequest) (StorageAdmissionReceipt, error)
}

// AttemptInfrastructureError is the only admissible way for a backend to
// exclude a preregistered raw slot. The public scorer never infers categories
// from free-form diagnostics.
type AttemptInfrastructureError struct {
	Category DeepSWEFailureCategory
	Err      error
}

func (e AttemptInfrastructureError) Error() string {
	return string(e.Category) + ": " + e.Err.Error()
}

func (e AttemptInfrastructureError) Unwrap() error { return e.Err }

// AttemptProtocolError marks a nonrecoverable benchmark-contract violation.
// It is never an infrastructure exclusion and never receives a replacement
// attempt; the entire experiment becomes invalid.
type AttemptProtocolError struct{ Err error }

func (err AttemptProtocolError) Error() string {
	if err.Err == nil {
		return "raw attempt violated the benchmark protocol"
	}
	return err.Err.Error()
}
func (err AttemptProtocolError) Unwrap() error { return err.Err }

// SafeRestartAttemptError is valid only during RecoverAgent when the sealed
// artifacts prove that the reserved slot emitted zero provider requests. The
// runner may restart that same slot without incrementing Attempts.
type SafeRestartAttemptError struct{ Err error }

func (e SafeRestartAttemptError) Error() string {
	return "reserved slot has no provider request: " + e.Err.Error()
}
func (e SafeRestartAttemptError) Unwrap() error { return e.Err }
