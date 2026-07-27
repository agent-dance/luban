package harness

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

var (
	idPattern     = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	envPattern    = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)
	hex40Pattern  = regexp.MustCompile(`^[0-9a-f]{40}$`)
	hex64Pattern  = regexp.MustCompile(`^[0-9a-f]{64}$`)
	digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

const (
	// FormalProviderOrigin is the only upstream origin admitted by the current
	// paired protocol. It is a configured gateway observation point, not proof
	// that the upstream service is operated or signed by OpenAI.
	FormalProviderOrigin                           = "https://sub.blurooo.com"
	FormalProviderTLSServerName                    = "sub.blurooo.com"
	FormalProviderEndpointSemanticsSHA256          = "80bdbbb0289e7290fb96a62bd02ee85610d0e9e804a4364215f7d7a4274c1233"
	FormalServiceTier                              = "default"
	ServiceTierEncodingExplicitDefault             = "explicit_default"
	ServiceTierEncodingClientCanonical             = "client_canonicalized_default"
	TransportRequirementHTTPInference              = "http_inference_required"
	TransportRequirementWebSocket                  = "websocket_required"
	FormalToolCatalogSchemaVersion                 = "agentic-bench/formal-tool-catalog-v1"
	FormalExecutionCanaryGeneration                = "v8"
	FormalStorageEnforcement                       = "declared_by_task_unenforced_by_pier-0.3"
	HostStorageGuardSchemaVersion                  = "agentic-bench/host-storage-guard-v1"
	GuestStorageGuardSchemaVersion                 = "agentic-bench/guest-storage-guard-v1"
	StorageStatfsAuthority                         = "external-authenticated-host-supervisor-statfs-bavail-v1"
	GuestStorageStatfsAuthority                    = "guest-pinned-fd-statfs-bavail-v1"
	StorageReceiptSchemaVersion                    = "agentic-bench/storage-resource-receipt-v1"
	StorageEvidenceSchemaVersion                   = "agentic-bench/storage-resource-evidence-v1"
	StorageAdmissionSchemaVersion                  = "agentic-bench/storage-admission-receipt-v1"
	StorageStatusCompletedAboveGuard               = "completed_above_guard"
	StorageMeasurementStatfsBavail                 = "statfs_bavail_times_block_size"
	StorageReceiptRelativePath                     = "pier/storage-resource-receipt.json"
	GuestStorageReceiptSchemaVersion               = "agentic-bench/guest-storage-resource-receipt-v1"
	GuestStorageEvidenceSchemaVersion              = "agentic-bench/guest-storage-resource-evidence-v1"
	GuestStorageAgentReceiptRelativePath           = "pier/guest-storage-agent-receipt.json"
	GuestStorageVerifierReceiptRelativePath        = "pier/guest-storage-verifier-receipt.json"
	GuestStoragePhaseAgent                         = "agent"
	GuestStoragePhaseVerifier                      = "verifier"
	StorageStageExperimentPreflight                = "experiment_preflight"
	StorageStageRawSlotAdmission                   = "raw_slot_admission"
	FormalStorageExperimentMinimumBytes     uint64 = 107_374_182_400
	FormalStorageRuntimeWarningBytes        uint64 = 53_687_091_200
	FormalStorageNewSlotMinimumBytes        uint64 = 32_212_254_720
	FormalStoragePollIntervalMS                    = 1000
	FormalStorageGapThresholdMS                    = 2500
	FormalGuestStorageStartMinimumBytes     uint64 = 30_064_771_072
	FormalGuestStorageRuntimeFloorBytes     uint64 = 8_589_934_592
	FormalGuestStorageConfiguredBytes       uint64 = 68_719_476_736
)

// FormalProviderEndpoint returns the exact endpoint contract preregistered by
// this protocol. Keeping the semantics explicit avoids treating a gateway's
// observation as provider-signed identity evidence.
func FormalProviderEndpoint() ProviderEndpointSpec {
	return ProviderEndpointSpec{
		ApprovedOrigin: FormalProviderOrigin,
		Semantics: ProviderEndpointSemantics{
			SchemaVersion:            "agentic-bench/provider-endpoint-semantics-v1",
			APIProtocol:              "openai-responses",
			ObservationAuthority:     "configured-gateway",
			ProviderIdentityAttested: false,
			TLSRequired:              true,
			WebSocketAllowed:         true,
		},
		SemanticsSHA256: FormalProviderEndpointSemanticsSHA256,
	}
}

func FormalHostStorageGuard() HostStorageGuardSpec {
	return HostStorageGuardSpec{
		SchemaVersion:                     HostStorageGuardSchemaVersion,
		AdmissionMinimumAvailableBytes:    FormalStorageExperimentMinimumBytes,
		RuntimeWarningBelowAvailableBytes: FormalStorageRuntimeWarningBytes,
		RuntimeHardFloorAvailableBytes:    FormalStorageNewSlotMinimumBytes,
		PollIntervalMS:                    FormalStoragePollIntervalMS,
		MonitoringGapThresholdMS:          FormalStorageGapThresholdMS,
		Measurement:                       StorageMeasurementStatfsBavail,
	}
}

func FormalGuestStorageGuard() GuestStorageGuardSpec {
	return GuestStorageGuardSpec{
		SchemaVersion:                   GuestStorageGuardSchemaVersion,
		StartMinimumAvailableBytes:      FormalGuestStorageStartMinimumBytes,
		RuntimeAbortBelowAvailableBytes: FormalGuestStorageRuntimeFloorBytes,
		PollIntervalMS:                  FormalStoragePollIntervalMS,
		MonitoringGapThresholdMS:        FormalStorageGapThresholdMS,
		Measurement:                     StorageMeasurementStatfsBavail,
	}
}

type LoadedManifest struct {
	Manifest Manifest
	SHA256   string
	Raw      []byte
}

func LoadManifest(path string) (LoadedManifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return LoadedManifest{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return LoadedManifest{}, fmt.Errorf("decode benchmark manifest: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return LoadedManifest{}, errors.New("decode benchmark manifest: trailing JSON value")
		}
		return LoadedManifest{}, fmt.Errorf("decode benchmark manifest trailer: %w", err)
	}
	if err := ValidateManifest(manifest); err != nil {
		return LoadedManifest{}, err
	}
	sum := sha256.Sum256(raw)
	return LoadedManifest{Manifest: manifest, SHA256: hex.EncodeToString(sum[:]), Raw: raw}, nil
}

func ValidateManifest(m Manifest) error {
	var problems []string
	if m.SchemaVersion != SchemaVersion {
		problems = append(problems, "schema_version must be "+SchemaVersion)
	}
	if !idPattern.MatchString(m.Experiment.ID) {
		problems = append(problems, "experiment.id is invalid")
	}
	validateSourcePin("dataset", m.Dataset, &problems)
	validateSourcePin("evaluator", m.Evaluator.SourcePin, &problems)
	if m.Evaluator.Protocol != "pier-harbor-separate-verifier" {
		problems = append(problems, "evaluator.protocol must require the separate verifier")
	}
	if m.Evaluator.MinimumVersion == "" {
		problems = append(problems, "evaluator.minimum_version is required")
	}
	if !hex64Pattern.MatchString(m.Evaluator.BinarySHA256) {
		problems = append(problems, "evaluator.binary_sha256 must be a SHA-256")
	}
	validateProviderEndpoint(m.ProviderEndpoint, &problems)
	if len(m.Agents) < 2 {
		problems = append(problems, "at least two agents are required")
	}
	agentIDs := make(map[string]struct{}, len(m.Agents))
	pairedTransportRequirement := ""
	if len(m.Agents) != 2 {
		problems = append(problems, "paired benchmark requires exactly two agents")
	}
	for i, agent := range m.Agents {
		prefix := fmt.Sprintf("agents[%d]", i)
		if !idPattern.MatchString(agent.ID) {
			problems = append(problems, prefix+".id is invalid")
		}
		if _, exists := agentIDs[agent.ID]; exists {
			problems = append(problems, prefix+".id is duplicated")
		}
		agentIDs[agent.ID] = struct{}{}
		if !filepath.IsAbs(agent.Binary) {
			problems = append(problems, prefix+".binary must be absolute")
		}
		if !hex64Pattern.MatchString(agent.BinarySHA256) {
			problems = append(problems, prefix+".binary_sha256 must be a SHA-256")
		}
		if agent.SourceSnapshot != nil {
			source := agent.SourceSnapshot
			if !filepath.IsAbs(source.Worktree) {
				problems = append(problems, prefix+".source_snapshot.worktree must be absolute")
			}
			if !hex40Pattern.MatchString(source.BaseCommit) {
				problems = append(problems, prefix+".source_snapshot.base_commit must be a full Git commit")
			}
			if !hex40Pattern.MatchString(source.TreeOID) {
				problems = append(problems, prefix+".source_snapshot.tree_oid must be a full Git tree OID")
			}
			if !hex64Pattern.MatchString(source.PatchSHA256) || !hex64Pattern.MatchString(source.ArchiveSHA256) {
				problems = append(problems, prefix+".source_snapshot patch and archive hashes must be SHA-256")
			}
			if err := validateFormalSourcePathPolicy(source.PathPolicy, source.PathPolicySHA256); err != nil {
				problems = append(problems, prefix+".source_snapshot path policy is invalid")
			}
			if !hex64Pattern.MatchString(source.ExclusionReceiptSHA256) {
				problems = append(problems, prefix+".source_snapshot.exclusion_receipt_sha256 must be a SHA-256")
			} else if _, _, digest, err := captureSourceExclusionReceipt(source.PathPolicy, source.PathPolicySHA256); err != nil || digest != source.ExclusionReceiptSHA256 {
				problems = append(problems, prefix+".source_snapshot exclusion receipt does not bind the formal path policy")
			}
			if !filepath.IsAbs(source.BuildReceipt) {
				problems = append(problems, prefix+".source_snapshot.build_receipt must be absolute")
			}
			if !hex64Pattern.MatchString(source.BuildReceiptSHA256) {
				problems = append(problems, prefix+".source_snapshot.build_receipt_sha256 must be a SHA-256")
			}
		}
		if len(agent.Command.Argv) == 0 {
			problems = append(problems, prefix+".command.argv is required")
		} else if agent.Command.Argv[0] != agent.Binary {
			problems = append(problems, prefix+".command.argv[0] must equal the frozen binary path")
		}
		if agent.Model.Provider == "" || agent.Model.Model == "" || agent.Model.ReasoningEffort == "" {
			problems = append(problems, prefix+".model provider, model, and reasoning_effort are required")
		}
		if agent.Model.ServiceTier != FormalServiceTier {
			problems = append(problems, prefix+".model.service_tier must be explicitly default")
		}
		expectedTierEncoding := ""
		switch agent.ID {
		case "codex":
			expectedTierEncoding = ServiceTierEncodingClientCanonical
		case "luban":
			expectedTierEncoding = ServiceTierEncodingExplicitDefault
		default:
			problems = append(problems, prefix+".id must be codex or luban for the formal paired contract")
		}
		if agent.Model.ServiceTierRequestEncoding != expectedTierEncoding {
			problems = append(problems, prefix+".model.service_tier_request_encoding does not match the frozen client behavior")
		}
		switch agent.Model.TransportRequirement {
		case TransportRequirementHTTPInference, TransportRequirementWebSocket:
		default:
			problems = append(problems, prefix+".model.transport_requirement is invalid")
		}
		if pairedTransportRequirement == "" {
			pairedTransportRequirement = agent.Model.TransportRequirement
		} else if agent.Model.TransportRequirement != pairedTransportRequirement {
			problems = append(problems, "paired agents must use the same transport_requirement")
		}
		if agent.ExecutionCanary.Generation != FormalExecutionCanaryGeneration || !hex64Pattern.MatchString(agent.ExecutionCanary.ReceiptSHA256) {
			problems = append(problems, prefix+".execution_canary must pin a v8 receipt SHA-256")
		}
		validateToolCatalogSpec(prefix+".model.tool_catalog", agent.ID, agent.Model.ToolCatalog, &problems)
		if !agent.RequestEvidence.Required {
			problems = append(problems, prefix+".request_evidence.required must be true")
		}
		if err := validateRelativePath(agent.RequestEvidence.RelativePath); err != nil {
			problems = append(problems, prefix+".request_evidence.relative_path "+err.Error())
		}
		for _, name := range agent.Command.RequiredEnv {
			if !envPattern.MatchString(name) {
				problems = append(problems, prefix+".command.required_env contains an invalid name")
			}
			if !slices.Contains(m.Environment.HostEnvAllowlist, name) {
				problems = append(problems, prefix+".command.required_env is not in the environment allowlist")
			}
		}
	}
	validateSelection(m.Selection, &problems)
	validateScoring(m.Scoring, m.Scheduling, agentIDs, &problems)
	if !m.Scheduling.PairAgents {
		problems = append(problems, "scheduling.pair_agents must be true")
	}
	if m.Scheduling.Repetitions < 1 {
		problems = append(problems, "scheduling.repetitions must be positive")
	}
	if m.Scheduling.MaxParallelPairs < 1 {
		problems = append(problems, "scheduling.max_parallel_pairs must be positive")
	}
	if m.Environment.TaskNetworkMode != "no-network" || m.Environment.VerifierNetworkMode != "no-network" {
		problems = append(problems, "task and verifier network modes must be no-network")
	}
	seenEnv := map[string]struct{}{}
	for _, name := range m.Environment.HostEnvAllowlist {
		if !envPattern.MatchString(name) {
			problems = append(problems, "environment.host_env_allowlist contains an invalid name")
		}
		if _, exists := seenEnv[name]; exists {
			problems = append(problems, "environment.host_env_allowlist contains a duplicate")
		}
		seenEnv[name] = struct{}{}
	}
	for _, host := range m.Environment.AgentEgressHosts {
		if host == "" || strings.ContainsAny(host, "/: ") {
			problems = append(problems, "environment.agent_egress_hosts must contain bare DNS names")
		}
	}
	if m.Timeouts.SetupSeconds < 1 || m.Timeouts.AgentSeconds < 1 || m.Timeouts.VerifierSeconds < 1 || m.Timeouts.TeardownSeconds < 1 {
		problems = append(problems, "all timeout values must be positive")
	}
	if m.Resources.CPUs < 1 || m.Resources.MemoryMB < 1 || m.Resources.StorageMB != 20480 || m.Resources.GPUs < 0 {
		problems = append(problems, "resource limits are invalid")
	}
	if m.Resources.HostStorageGuard != FormalHostStorageGuard() {
		problems = append(problems, "resources.host_storage_guard must equal the preregistered 100 GiB experiment admission, 50 GiB warning, 30 GiB raw-slot floor, and 1000 ms poll contract")
	}
	if m.Resources.GuestStorageGuard != FormalGuestStorageGuard() {
		problems = append(problems, "resources.guest_storage_guard must equal the preregistered 28 GiB start, 8 GiB runtime floor, and 1000 ms poll contract")
	}
	validatePricing(m.Pricing, m.Agents, &problems)
	if m.Scoring.Profile == ScoringProfileDeepSWEV11PublicCI && m.Pricing.Currency != "USD" {
		problems = append(problems, "deepswe-v1.1-public-ci pricing currency must be USD")
	}
	if err := validateRelativePath(m.Artifacts.Root); err != nil {
		problems = append(problems, "artifacts.root "+err.Error())
	}
	if err := validateRelativePath(m.Artifacts.LedgerRelativePath); err != nil {
		problems = append(problems, "artifacts.ledger_relative_path "+err.Error())
	}
	if err := validateRelativePath(m.Artifacts.StateRelativePath); err != nil {
		problems = append(problems, "artifacts.state_relative_path "+err.Error())
	}
	if !m.Artifacts.CaptureBinaryDiff || !m.Artifacts.CaptureUntracked {
		problems = append(problems, "binary and untracked diff capture must be enabled")
	}
	if !m.Oracle.Required || !m.Oracle.SeparateEnvironment {
		problems = append(problems, "oracle validation in a separate environment is required")
	}
	if err := validateRelativePath(m.Oracle.SolutionRoot); err != nil {
		problems = append(problems, "oracle.solution_root "+err.Error())
	}
	if len(problems) > 0 {
		return fmt.Errorf("invalid benchmark manifest: %s", strings.Join(problems, "; "))
	}
	return nil
}

func validateToolCatalogSpec(prefix, agentID string, catalog ToolCatalogSpec, problems *[]string) {
	if catalog.SchemaVersion != FormalToolCatalogSchemaVersion || !hex64Pattern.MatchString(catalog.SemanticSHA256) {
		*problems = append(*problems, prefix+" must bind the formal semantic catalog")
	}
	expected := formalToolCatalog(agentID)
	if len(expected) == 0 || len(catalog.Tools) != len(expected) {
		*problems = append(*problems, prefix+" must contain the exact ordered agent catalog")
		return
	}
	for index, tool := range catalog.Tools {
		if tool.Type != expected[index].Type || tool.Name != expected[index].Name || !hex64Pattern.MatchString(tool.DefinitionSHA256) {
			*problems = append(*problems, prefix+" contains a changed identity or definition digest")
			return
		}
	}
}

func validateProviderEndpoint(endpoint ProviderEndpointSpec, problems *[]string) {
	expected := FormalProviderEndpoint()
	if endpoint.ApprovedOrigin != expected.ApprovedOrigin {
		*problems = append(*problems, "provider_endpoint.approved_origin must be "+FormalProviderOrigin)
	}
	if endpoint.Semantics != expected.Semantics {
		*problems = append(*problems, "provider_endpoint.semantics must describe the configured Responses gateway without provider identity attestation")
	}
	digest, err := HashCanonical(endpoint.Semantics)
	if err != nil || digest != endpoint.SemanticsSHA256 || endpoint.SemanticsSHA256 != expected.SemanticsSHA256 {
		*problems = append(*problems, "provider_endpoint.semantics_sha256 must bind the formal endpoint semantics")
	}
}

func validateScoring(scoring ScoringSpec, scheduling SchedulingSpec, agentIDs map[string]struct{}, problems *[]string) {
	if scoring.Profile != ScoringProfileDeepSWEV11PublicCI {
		*problems = append(*problems, "scoring.profile must be "+ScoringProfileDeepSWEV11PublicCI)
	}
	if _, exists := agentIDs[scoring.BaselineAgentID]; !exists {
		*problems = append(*problems, "scoring.baseline_agent_id must name a configured agent")
	}
	if _, exists := agentIDs[scoring.ChallengerAgentID]; !exists {
		*problems = append(*problems, "scoring.challenger_agent_id must name a configured agent")
	}
	if scoring.BaselineAgentID == scoring.ChallengerAgentID {
		*problems = append(*problems, "scoring baseline and challenger must differ")
	}
	if scheduling.Repetitions != 1 && scheduling.Repetitions != 4 {
		*problems = append(*problems, "deepswe-v1.1-public-ci supports exactly one pilot run or four public runs")
	}
}

func validateSourcePin(prefix string, source SourcePin, problems *[]string) {
	if source.Name == "" {
		*problems = append(*problems, prefix+".name is required")
	}
	parsed, err := url.Parse(source.Repository)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		*problems = append(*problems, prefix+".repository must be an HTTPS URL")
	}
	if !hex40Pattern.MatchString(source.Commit) {
		*problems = append(*problems, prefix+".commit must be a full Git commit")
	}
	if err := validateRelativePath(source.Root); err != nil {
		*problems = append(*problems, prefix+".root "+err.Error())
	}
	if !hex64Pattern.MatchString(source.TreeSHA256) {
		*problems = append(*problems, prefix+".tree_sha256 must be a SHA-256")
	}
	if !hex64Pattern.MatchString(source.ManifestSHA256) {
		*problems = append(*problems, prefix+".manifest_sha256 must be a SHA-256")
	}
	if prefix == "dataset" {
		if !hex64Pattern.MatchString(source.InventoryLockFileSHA256) {
			*problems = append(*problems, "dataset.inventory_lock_file_sha256 must be a SHA-256")
		}
	} else if source.InventoryLockFileSHA256 != "" {
		*problems = append(*problems, prefix+".inventory_lock_file_sha256 is not applicable")
	}
}

func validateSelection(selection SelectionSpec, problems *[]string) {
	if selection.ExpectedTaskCount < 1 {
		*problems = append(*problems, "selection.expected_task_count must be positive")
	}
	switch selection.Mode {
	case "full":
		if len(selection.TaskIDs) != 0 || selection.SampleCount != 0 {
			*problems = append(*problems, "full selection cannot include task_ids or sample_count")
		}
	case "tasks":
		if len(selection.TaskIDs) == 0 || selection.SampleCount != 0 {
			*problems = append(*problems, "tasks selection requires task_ids and no sample_count")
		}
		seen := map[string]struct{}{}
		for _, id := range selection.TaskIDs {
			if !idPattern.MatchString(id) {
				*problems = append(*problems, "selection.task_ids contains an invalid ID")
			}
			if _, exists := seen[id]; exists {
				*problems = append(*problems, "selection.task_ids contains a duplicate")
			}
			seen[id] = struct{}{}
		}
	case "sample":
		if len(selection.TaskIDs) != 0 || selection.SampleCount < 1 || selection.SampleCount > selection.ExpectedTaskCount {
			*problems = append(*problems, "sample selection requires a valid sample_count and no task_ids")
		}
	default:
		*problems = append(*problems, "selection.mode must be full, tasks, or sample")
	}
}

func validatePricing(catalog PricingCatalog, agents []AgentSpec, problems *[]string) {
	if catalog.Currency == "" || catalog.UnitTokens < 1 || catalog.EffectiveAt.IsZero() || catalog.ObservedAt.IsZero() {
		*problems = append(*problems, "pricing currency, unit_tokens, effective_at, and observed_at are required")
	}
	if !catalog.ObservedAt.IsZero() && !catalog.EffectiveAt.IsZero() && catalog.ObservedAt.Before(catalog.EffectiveAt) {
		*problems = append(*problems, "pricing.observed_at cannot precede effective_at")
	}
	parsed, err := url.Parse(catalog.SourceURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		*problems = append(*problems, "pricing.source_url must be an HTTPS URL")
	}
	for _, agent := range agents {
		found := false
		for _, rate := range catalog.Rates {
			if rate.Provider == agent.Model.Provider && rate.Model == agent.Model.Model {
				found = true
				if rate.Input < 0 || rate.CachedInput < 0 || rate.Output < 0 || rate.CacheWriteInputMultiplier < 1 {
					*problems = append(*problems, "pricing rates and cache-write multiplier are invalid")
				}
				if len(rate.RequestTiers) == 0 {
					*problems = append(*problems, "pricing request tiers are required")
				}
				previousThreshold := int64(-1)
				for _, tier := range rate.RequestTiers {
					if tier.Name == "" || tier.ThresholdInputTokens <= previousThreshold || tier.InputMultiplier <= 0 || tier.CachedInputMultiplier <= 0 || tier.OutputMultiplier <= 0 {
						*problems = append(*problems, "pricing request tiers must be named, positive, and strictly ordered")
					}
					previousThreshold = tier.ThresholdInputTokens
				}
			}
		}
		if !found {
			*problems = append(*problems, "pricing has no rate for agent "+agent.ID)
		}
	}
}

func validateRelativePath(path string) error {
	if path == "" {
		return errors.New("is required")
	}
	if filepath.IsAbs(path) || filepath.Clean(path) != path || path == "." || strings.HasPrefix(path, ".."+string(filepath.Separator)) {
		return errors.New("must be a clean relative path")
	}
	return nil
}

// FilterEnvironment copies only explicitly allowed host variables. Missing
// required variables fail before an agent sandbox is created. Returned values
// must not be persisted by callers.
func FilterEnvironment(host []string, allowlist, required []string) ([]string, error) {
	allowed := make(map[string]struct{}, len(allowlist))
	for _, name := range allowlist {
		allowed[name] = struct{}{}
	}
	values := make(map[string]string, len(host))
	for _, item := range host {
		name, value, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		if _, ok := allowed[name]; ok {
			values[name] = value
		}
	}
	for _, name := range required {
		if _, ok := values[name]; !ok {
			return nil, fmt.Errorf("required environment variable %s is missing", name)
		}
	}
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	slices.Sort(names)
	filtered := make([]string, 0, len(names))
	for _, name := range names {
		filtered = append(filtered, name+"="+values[name])
	}
	return filtered, nil
}

// IsImageDigest reports whether a task image is content-addressed. Mutable image
// tags are never accepted for a formal run.
func IsImageDigest(value string) bool { return digestPattern.MatchString(value) }
