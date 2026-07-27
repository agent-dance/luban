package report

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/benchmark/agentic/evidenceproxy"
	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

type formalBundleFixture struct {
	inputPath     string
	artifactRoot  string
	ledgerPath    string
	verifierPaths []string
}

const formalFixtureGiB = uint64(1024 * 1024 * 1024)

func writeFormalAgentStreamFixture(
	t *testing.T,
	artifactDir string,
	agentID string,
	durationMS int64,
	outputBytes int64,
	physicalToolOperations int,
	toolQueueMS int64,
) (string, string) {
	t.Helper()
	rawToolID := agentID + "-tool-use-0"
	digest := sha256.Sum256([]byte(rawToolID))
	toolIDHash := hex.EncodeToString(digest[:])
	traceKind := "luban_tool_result"
	var toolEvent any = map[string]any{
		"type": "tool_result", "tool_use_id": rawToolID, "is_error": false,
		"metrics": map[string]any{"content_bytes": outputBytes, "duration_ms": durationMS},
	}
	if agentID == "codex" {
		traceKind = "command_execution"
		toolEvent = map[string]any{
			"type": "item.completed",
			"item": map[string]any{
				"id": rawToolID, "type": traceKind, "status": "completed", "exit_code": 0,
				"aggregated_output": strings.Repeat("x", int(outputBytes)), "duration_ms": durationMS,
			},
		}
	}
	roundEvent := map[string]any{
		"type": "agentic_metrics", "metric": "tool_round",
		"tool_round": map[string]any{
			"logical_model_visible_calls": 1, "physical_child_operations": physicalToolOperations,
			"critical_path_ms": durationMS, "total_child_latency_ms": durationMS, "queue_ms": toolQueueMS,
		},
	}
	var stream []byte
	for _, event := range []any{toolEvent, roundEvent} {
		encoded, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("marshal %s public agent stream event: %v", agentID, err)
		}
		stream = append(stream, encoded...)
		stream = append(stream, '\n')
	}
	writeFileForFormalFixture(t, filepath.Join(artifactDir, "pier", "agent-stream.jsonl"), stream)
	return toolIDHash, traceKind
}

func formalStorageAdmissionForFixture(stage string, observedAt time.Time, availableBytes uint64) harness.StorageAdmissionReceipt {
	return harness.StorageAdmissionReceipt{
		SchemaVersion:     harness.StorageAdmissionSchemaVersion,
		Stage:             stage,
		Enforcement:       harness.FormalStorageEnforcement,
		DeclaredStorageMB: 20480,
		Guard:             harness.FormalHostStorageGuard(),
		Authority:         harness.StorageStatfsAuthority,
		ObservedAt:        observedAt,
		Filesystems: []harness.StorageAdmissionFilesystemReceipt{{
			Group:                0,
			Roles:                []string{"artifact_root", "controller_root", "private_work_root"},
			VolumeIdentitySHA256: formalHex("1", 64),
			FilesystemType:       "apfs",
			DeviceRoleCount:      3,
			ObservedAt:           observedAt.Add(-time.Millisecond),
			MonotonicOffsetMS:    1,
			BlockSizeBytes:       4096,
			TotalBytes:           200 * formalFixtureGiB,
			AvailableBytes:       availableBytes,
			UsedBytes:            70 * formalFixtureGiB,
		}},
		Passed:  true,
		Warning: availableBytes < harness.FormalStorageRuntimeWarningBytes,
	}
}

func writeFormalRunStorageEvidence(
	t *testing.T,
	artifactDir string,
	admission harness.StorageAdmissionReceipt,
	hostStartedAt time.Time,
	hostFinishedAt time.Time,
	providerWALStartedAt time.Time,
	agentStartedAt time.Time,
	agentFinishedAt time.Time,
	verifierStartedAt time.Time,
	verifierFinishedAt time.Time,
	runIdentity string,
) (harness.StorageResourceEvidence, []harness.GuestStorageResourceEvidence) {
	t.Helper()
	hostAvailable, hostUsed := 40*formalFixtureGiB, 70*formalFixtureGiB
	hostSamples, hostMaximumGap := formalStorageSamplesForFixture(hostStartedAt, hostFinishedAt, hostAvailable, hostUsed)
	hostReceipt := harness.StorageResourceReceipt{
		SchemaVersion:             harness.StorageReceiptSchemaVersion,
		Enforcement:               harness.FormalStorageEnforcement,
		DeclaredStorageMB:         20480,
		Guard:                     harness.FormalHostStorageGuard(),
		Authority:                 harness.StorageStatfsAuthority,
		Admission:                 admission,
		StartedAt:                 hostStartedAt,
		FinishedAt:                hostFinishedAt,
		ProviderWALStartedAt:      providerWALStartedAt,
		ProviderWALStartedDeltaMS: providerWALStartedAt.Sub(hostStartedAt).Milliseconds(),
		FinishedDeltaMS:           hostFinishedAt.Sub(hostStartedAt).Milliseconds(),
		Filesystems: []harness.StorageRuntimeFilesystemReceipt{{
			Group:                  0,
			Roles:                  []string{"artifact_root", "controller_root", "private_work_root"},
			VolumeIdentitySHA256:   admission.Filesystems[0].VolumeIdentitySHA256,
			FilesystemType:         admission.Filesystems[0].FilesystemType,
			DeviceRoleCount:        admission.Filesystems[0].DeviceRoleCount,
			BlockSizeBytes:         admission.Filesystems[0].BlockSizeBytes,
			TotalBytes:             admission.Filesystems[0].TotalBytes,
			AvailableBeforeBytes:   hostAvailable,
			AvailableAfterBytes:    hostAvailable,
			MinimumAvailableBytes:  hostAvailable,
			UsedBeforeBytes:        hostUsed,
			UsedAfterBytes:         hostUsed,
			MaximumUsedBytes:       hostUsed,
			Samples:                uint64(len(hostSamples)),
			WarningSamples:         uint64(len(hostSamples)),
			MaximumCompletionGapMS: hostMaximumGap,
			SamplePoints:           hostSamples,
		}},
		Status: harness.StorageStatusCompletedAboveGuard,
	}
	hostPath := filepath.Join(artifactDir, filepath.FromSlash(harness.StorageReceiptRelativePath))
	writeJSONForFormalFixture(t, hostPath, hostReceipt)
	hostSHA, err := harness.HashFile(hostPath)
	if err != nil {
		t.Fatalf("hash host storage receipt: %v", err)
	}
	hostEvidence := harness.StorageResourceEvidence{
		SchemaVersion: harness.StorageEvidenceSchemaVersion, ReceiptRelativePath: harness.StorageReceiptRelativePath,
		ReceiptSHA256: hostSHA, Receipt: hostReceipt,
	}

	guestReceipts := []harness.GuestStorageResourceReceipt{
		formalGuestStorageReceiptForFixture(harness.GuestStoragePhaseAgent, runIdentity, formalHex("2", 64), agentStartedAt, agentFinishedAt, providerWALStartedAt),
		formalGuestStorageReceiptForFixture(harness.GuestStoragePhaseVerifier, runIdentity, formalHex("3", 64), verifierStartedAt, verifierFinishedAt, providerWALStartedAt),
	}
	guestPaths := []string{harness.GuestStorageAgentReceiptRelativePath, harness.GuestStorageVerifierReceiptRelativePath}
	guestEvidence := make([]harness.GuestStorageResourceEvidence, 0, len(guestReceipts))
	for index, receipt := range guestReceipts {
		path := filepath.Join(artifactDir, filepath.FromSlash(guestPaths[index]))
		writeJSONForFormalFixture(t, path, receipt)
		digest, err := harness.HashFile(path)
		if err != nil {
			t.Fatalf("hash guest %s storage receipt: %v", receipt.Phase, err)
		}
		guestEvidence = append(guestEvidence, harness.GuestStorageResourceEvidence{
			SchemaVersion: harness.GuestStorageEvidenceSchemaVersion, ReceiptRelativePath: guestPaths[index],
			ReceiptSHA256: digest, Receipt: receipt,
		})
	}
	resources := harness.ResourceSpec{
		StorageMB: 20480, HostStorageGuard: harness.FormalHostStorageGuard(), GuestStorageGuard: harness.FormalGuestStorageGuard(),
	}
	if err := harness.ValidateStorageResourceEvidence(artifactDir, hostEvidence, admission, resources); err != nil {
		t.Fatalf("self-validate host storage evidence: %v", err)
	}
	if err := harness.ValidateGuestStorageResourceEvidence(artifactDir, guestEvidence, resources); err != nil {
		t.Fatalf("self-validate guest storage evidence: %v", err)
	}
	return hostEvidence, guestEvidence
}

func formalGuestStorageReceiptForFixture(
	phase string,
	sessionIdentity string,
	containerIdentity string,
	startedAt time.Time,
	finishedAt time.Time,
	providerWALStartedAt time.Time,
) harness.GuestStorageResourceReceipt {
	available, used := 32*formalFixtureGiB, 20*formalFixtureGiB
	samples, maximumGap := formalStorageSamplesForFixture(startedAt, finishedAt, available, used)
	return harness.GuestStorageResourceReceipt{
		SchemaVersion:             harness.GuestStorageReceiptSchemaVersion,
		Phase:                     phase,
		SessionIdentitySHA256:     runIdentityDigestForFixture(sessionIdentity),
		ContainerIdentitySHA256:   containerIdentity,
		ConfiguredCapacityBytes:   harness.FormalGuestStorageConfiguredBytes,
		Enforcement:               harness.FormalStorageEnforcement,
		DeclaredStorageMB:         20480,
		Guard:                     harness.FormalGuestStorageGuard(),
		Authority:                 harness.GuestStorageStatfsAuthority,
		StartedAt:                 startedAt,
		FinishedAt:                finishedAt,
		ProviderWALStartedAt:      providerWALStartedAt,
		ProviderWALStartedDeltaMS: providerWALStartedAt.Sub(startedAt).Milliseconds(),
		FinishedDeltaMS:           finishedAt.Sub(startedAt).Milliseconds(),
		Filesystems: []harness.GuestStorageFilesystemReceipt{{
			Group:                  0,
			Roles:                  []string{"guest_app", "guest_root"},
			VolumeIdentitySHA256:   formalHex("4", 64),
			FilesystemType:         "ext4",
			DeviceRoleCount:        2,
			BlockSizeBytes:         4096,
			TotalBytes:             harness.FormalGuestStorageConfiguredBytes,
			MinimumAvailableBytes:  available,
			MaximumUsedBytes:       used,
			MaximumCompletionGapMS: maximumGap,
			Samples:                samples,
		}},
		Status: harness.StorageStatusCompletedAboveGuard,
	}
}

func runIdentityDigestForFixture(value string) string {
	if len(value) == 64 {
		return value
	}
	return formalHex("a", 64)
}

func formalStorageSamplesForFixture(startedAt, finishedAt time.Time, availableBytes, usedBytes uint64) ([]harness.StorageStatfsSample, int64) {
	durationMS := finishedAt.Sub(startedAt).Milliseconds()
	samples := make([]harness.StorageStatfsSample, 0, int(durationMS/1000)+1)
	previousEnd, maximumGap := int64(0), int64(0)
	for end := int64(0); ; end += 1000 {
		if end > durationMS {
			end = durationMS
		}
		start := previousEnd
		samples = append(samples, harness.StorageStatfsSample{
			ObservedAt: startedAt.Add(time.Duration(end) * time.Millisecond), StartDeltaMS: start, EndDeltaMS: end,
			AvailableBytes: availableBytes, UsedBytes: usedBytes,
		})
		if gap := end - previousEnd; gap > maximumGap {
			maximumGap = gap
		}
		previousEnd = end
		if end == durationMS {
			break
		}
	}
	return samples, maximumGap
}

type formalServiceTierCanonicalizationPayload struct {
	SchemaVersion                      string `json:"schema_version"`
	Representation                     string `json:"representation"`
	ClientAgentID                      string `json:"client_agent_id"`
	ClientRuntimeVersion               string `json:"client_runtime_version"`
	RunIdentity                        string `json:"run_identity"`
	RegisteredBinarySHA256             string `json:"registered_binary_sha256"`
	FrozenBundleManifestSHA256         string `json:"frozen_bundle_manifest_sha256"`
	FrozenBundleTreeSHA256             string `json:"frozen_bundle_tree_sha256"`
	AdapterSHA256                      string `json:"adapter_sha256"`
	AdapterVersion                     string `json:"adapter_version"`
	SourceCommandArgvSHA256            string `json:"source_command_argv_sha256"`
	EffectiveArgvSHA256                string `json:"effective_argv_sha256"`
	EffectiveArgvReceiptSHA256         string `json:"effective_argv_receipt_sha256"`
	SandboxCanaryReceiptSHA256         string `json:"sandbox_canary_receipt_sha256"`
	CanonicalCanaryGeneration          string `json:"canonical_canary_generation"`
	FrozenCanonicalCanaryReceiptSHA256 string `json:"frozen_canonical_canary_receipt_sha256"`
	RawProviderEvidenceSHA256          string `json:"raw_provider_evidence_sha256"`
	TransformationEvidenceSHA256       string `json:"transformation_evidence_sha256"`
	TransformedProviderRoundCount      int    `json:"transformed_provider_round_count"`
	StaticProofSHA256                  string `json:"static_proof_sha256"`
}

type formalServiceTierCanonicalizationReceipt struct {
	formalServiceTierCanonicalizationPayload
	BindingSHA256 string `json:"binding_sha256"`
}

func formalServiceTierCanonicalizationStaticProof(t *testing.T, agent harness.AgentSpec) string {
	t.Helper()
	if agent.ID != "codex" {
		return ""
	}
	sourceArgvSHA, err := harness.HashCanonical(agent.Command.Argv)
	if err != nil {
		t.Fatalf("hash Codex source command argv: %v", err)
	}
	proof, err := evidenceproxy.ServiceTierCanonicalizationStaticProof(evidenceproxy.Config{
		AgentID: "codex", RegisteredBinarySHA256: agent.BinarySHA256,
		FrozenBundleManifestSHA256: formalHex("1", 64), FrozenBundleTreeSHA256: formalHex("2", 64),
		FrozenCanonicalCanaryReceiptSHA256: agent.ExecutionCanary.ReceiptSHA256,
		AdapterSHA256:                      formalHex("3", 64), AdapterVersion: "2.4.0",
		SourceCommandArgvSHA256: sourceArgvSHA,
	})
	if err != nil {
		t.Fatalf("hash Codex static service-tier canonicalization proof: %v", err)
	}
	return proof
}

func writeFormalServiceTierCanonicalizationEvidence(
	t *testing.T,
	artifactDir string,
	agent harness.AgentSpec,
	rawEvidenceSHA256 string,
	round *harness.ProviderRoundEvidence,
	staticProofSHA256 string,
) harness.ServiceTierCanonicalizationEvidence {
	t.Helper()
	if agent.ID != "codex" {
		return harness.ServiceTierCanonicalizationEvidence{}
	}
	type transformationProjection struct {
		Round            int    `json:"round"`
		OriginalBodySHA  string `json:"original_body_sha256"`
		ForwardedBodySHA string `json:"forwarded_body_sha256"`
		ProofSHA         string `json:"proof_sha256"`
	}
	transformationSHA, err := harness.HashCanonical([]transformationProjection{{
		Round: round.Round, OriginalBodySHA: round.OriginalRequestBodySHA256,
		ForwardedBodySHA: round.ForwardedRequestBodySHA256, ProofSHA: round.ServiceTierTransformationProofSHA256,
	}})
	if err != nil {
		t.Fatalf("hash service-tier transformation projection: %v", err)
	}
	sourceArgvSHA, err := harness.HashCanonical(agent.Command.Argv)
	if err != nil {
		t.Fatalf("hash source command argv: %v", err)
	}
	payload := formalServiceTierCanonicalizationPayload{
		SchemaVersion:                      "agentic-bench/service-tier-canonicalization-binding-v2",
		Representation:                     harness.ServiceTierEncodingClientCanonical,
		ClientAgentID:                      "codex",
		ClientRuntimeVersion:               "0.145.0",
		RunIdentity:                        round.RunIdentity,
		RegisteredBinarySHA256:             agent.BinarySHA256,
		FrozenBundleManifestSHA256:         formalHex("1", 64),
		FrozenBundleTreeSHA256:             formalHex("2", 64),
		AdapterSHA256:                      formalHex("3", 64),
		AdapterVersion:                     "2.4.0",
		SourceCommandArgvSHA256:            sourceArgvSHA,
		EffectiveArgvSHA256:                formalHex("5", 64),
		EffectiveArgvReceiptSHA256:         formalHex("6", 64),
		SandboxCanaryReceiptSHA256:         formalHex("7", 64),
		CanonicalCanaryGeneration:          harness.FormalExecutionCanaryGeneration,
		FrozenCanonicalCanaryReceiptSHA256: agent.ExecutionCanary.ReceiptSHA256,
		RawProviderEvidenceSHA256:          rawEvidenceSHA256,
		TransformationEvidenceSHA256:       transformationSHA,
		TransformedProviderRoundCount:      1,
		StaticProofSHA256:                  staticProofSHA256,
	}
	bindingSHA, err := harness.HashCanonical(payload)
	if err != nil {
		t.Fatalf("hash service-tier canonicalization binding: %v", err)
	}
	round.ClientCanonicalizationProofSHA256 = bindingSHA
	receipt := formalServiceTierCanonicalizationReceipt{
		formalServiceTierCanonicalizationPayload: payload,
		BindingSHA256:                            bindingSHA,
	}
	relativePath := "pier/service-tier-canonicalization-receipt.json"
	receiptPath := filepath.Join(artifactDir, filepath.FromSlash(relativePath))
	writeJSONForFormalFixture(t, receiptPath, receipt)
	receiptSHA, err := harness.HashFile(receiptPath)
	if err != nil {
		t.Fatalf("hash service-tier canonicalization receipt: %v", err)
	}
	return harness.ServiceTierCanonicalizationEvidence{
		SchemaVersion:                "agentic-bench/service-tier-canonicalization-evidence-v1",
		Representation:               harness.ServiceTierEncodingClientCanonical,
		ReceiptRelativePath:          relativePath,
		ReceiptSHA256:                receiptSHA,
		BindingSHA256:                bindingSHA,
		StaticProofSHA256:            staticProofSHA256,
		TransformationEvidenceSHA256: transformationSHA,
		TransformedRoundCount:        1,
	}
}

func TestCompileAcceptsCompleteFormalHarnessBundle(t *testing.T) {
	fixture := buildFormalBundleFixture(t)

	data, err := compileSyntheticFormalFixture(fixture.inputPath)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if !data.HasFormal || data.HasPilot || data.HasDiagnosticOnly {
		t.Fatalf("formal classification = formal:%t pilot:%t diagnostic-only:%t", data.HasFormal, data.HasPilot, data.HasDiagnosticOnly)
	}
	if len(data.Experiments) != 1 {
		t.Fatalf("experiments = %d, want 1", len(data.Experiments))
	}
	experiment := data.Experiments[0]
	if experiment.Class != ClassFormal || experiment.Manifest == nil {
		t.Fatalf("experiment class/manifest = %q/%v, want formal/non-nil", experiment.Class, experiment.Manifest)
	}
	if len(experiment.Runs) != 2 || len(experiment.ProviderRounds) != 2 {
		t.Fatalf("runs/provider rounds = %d/%d, want 2/2", len(experiment.Runs), len(experiment.ProviderRounds))
	}
	for _, run := range experiment.Runs {
		if run.Passed == nil || !*run.Passed {
			t.Fatalf("formal run %s/%s did not retain its passing verifier result", run.TaskID, run.AgentID)
		}
		if run.Metrics.LLMCallsStarted == nil || *run.Metrics.LLMCallsStarted != 1 ||
			run.Metrics.CompletedLLMResponses == nil || *run.Metrics.CompletedLLMResponses != 1 ||
			run.Metrics.TransportAttempts == nil || *run.Metrics.TransportAttempts != 1 ||
			run.Metrics.PrewarmAttempts == nil || *run.Metrics.PrewarmAttempts != 0 {
			t.Fatalf("formal run %s/%s all-started LLM accounting is incomplete: %+v", run.TaskID, run.AgentID, run.Metrics)
		}
		if run.Metrics.ProviderRequests == nil || *run.Metrics.ProviderRequests != 1 {
			t.Fatalf("formal run %s/%s provider requests = %v, want 1", run.TaskID, run.AgentID, run.Metrics.ProviderRequests)
		}
	}
	for _, gate := range experiment.Gates {
		if gate.Name == "controller_duration" {
			if gate.Status != GateUnknown {
				t.Fatalf("controller duration gate = %s, want unknown until controller span exists", gate.Status)
			}
			continue
		}
		if gate.Status != GatePass {
			t.Fatalf("formal gate %s = %s (%s), want pass", gate.Name, gate.Status, gate.Detail)
		}
	}
	if experiment.PublicScorecard == nil || experiment.PublicScorecard.Profile != harness.ScoringProfileDeepSWEV11PublicCI || len(experiment.PublicScorecard.Agents) != 2 {
		t.Fatalf("public scorecard = %#v, want frozen public profile and two agents", experiment.PublicScorecard)
	}
	for _, score := range experiment.PublicScorecard.Agents {
		if score.Counts.Raw != 1 || score.Counts.Scored != 1 || score.Counts.Excluded != 0 || score.LivePooled.Denominator != 1 {
			t.Fatalf("public score for %s = %+v, want one scored raw attempt", score.AgentID, score)
		}
	}
	var rendered bytes.Buffer
	if err := Render(&rendered, data); err != nil {
		t.Fatalf("Render formal public scorecard: %v", err)
	}
	for _, required := range []string{
		"live_pooled", "task_macro", "all_executed", "public_common_slot_pairing", "execution_matched",
		"Transport accounting uses two fixed universes", "Published reference cost is source-reported historical context",
		"Declared storage: 20480 MB", "configured benchmark gateway", "all_transport_attempts", "llm_calls_started_generate_true",
		"catalog_equivalent", "actual_gateway_invoice=N/A_not_claimed", "frozen_gateway_comparable_rate_card_estimate",
		"source_basis=OpenAI_public_standard_rates", "ordinary_uncached=I-C-W", harness.ServiceTierEncodingClientCanonical,
		harness.ServiceTierEncodingExplicitDefault, "reasoning.mode=standard", "effective_at=", "observed_at=",
	} {
		if !strings.Contains(rendered.String(), required) {
			t.Fatalf("rendered formal report omits %q", required)
		}
	}
	if strings.Contains(rendered.String(), "--responses-websocket") {
		t.Fatal("rendered HTTP-only formal report contains the deprecated WebSocket transport flag")
	}
}

func TestCompileRejectsRewrittenArchivedInventoryEvenWhenLedgerAndFileSHAAreRepinned(t *testing.T) {
	fixture := buildFormalBundleFixture(t)
	rewriteFormalStateAndScorecard(t, fixture, func(state *harness.ExperimentState, _ harness.RunPlan) {
		path := filepath.Join(fixture.artifactRoot, harness.InventoryLockArchiveRelativePath)
		var lock map[string]any
		readJSONForFormalFixture(t, path, &lock)
		tasks := lock["tasks"].([]any)
		task := tasks[0].(map[string]any)
		task["image"] = "registry.example/task:rewritten-after-run"
		writeJSONForFormalFixture(t, path, lock)
		fileSHA, err := harness.HashFile(path)
		if err != nil {
			t.Fatal(err)
		}
		state.Backend.InventoryLock.FileSHA256 = fileSHA
	})
	if _, err := compileSyntheticFormalFixture(fixture.inputPath); err == nil {
		t.Fatal("Compile accepted a repinned inventory archive with a different derived task inventory")
	}
}

func TestCompilePreservesTypedExcludedNoProviderAttemptAsStructuralZero(t *testing.T) {
	fixture := buildFormalBundleFixture(t)
	rewriteFormalStateAndScorecard(t, fixture, func(state *harness.ExperimentState, plan harness.RunPlan) {
		for _, entry := range plan.Entries {
			if entry.AgentID != "codex" {
				continue
			}
			key := harness.RunKey(entry)
			record := state.Runs[key]
			record.Disposition = harness.DeepSWEAttemptExcluded
			record.FailureCategory = harness.DeepSWEFailureNetworkInfrastructure
			record.Verification = nil
			record.Metrics = nil
			record.Execution.EvidencePath = ""
			record.Execution.ProviderEvidence = harness.ProviderEvidenceSeal{}
			record.Execution.ServiceTierCanonicalization = harness.ServiceTierCanonicalizationEvidence{}
			record.Execution.StorageEvidence = harness.StorageResourceEvidence{}
			record.Execution.GuestStorageEvidence = nil
			record.Execution.Lifecycle.ProviderAttemptState = "no_provider_attempt"
			record.Execution.Lifecycle.ProviderAttemptCount = 0
			record.Execution.SubmissionPatch = ""
			record.Execution.AuditWorkspacePatch = ""
			record.Execution.Capture = harness.SubmissionCaptureEvidence{}
			record.Failure = "sealed fixture network infrastructure failure"
			state.Runs[key] = record
		}
	})

	data, err := compileSyntheticFormalFixture(fixture.inputPath)
	if err != nil {
		t.Fatalf("Compile typed exclusion: %v", err)
	}
	experiment := data.Experiments[0]
	var excluded RunData
	for _, run := range experiment.Runs {
		if run.AgentID == "codex" {
			excluded = run
			break
		}
	}
	if excluded.Disposition != string(harness.DeepSWEAttemptExcluded) || excluded.FailureCategory != string(harness.DeepSWEFailureNetworkInfrastructure) || excluded.Passed != nil {
		t.Fatalf("excluded run classification = disposition=%q category=%q passed=%v", excluded.Disposition, excluded.FailureCategory, excluded.Passed)
	}
	if excluded.TrialDurationSeconds == nil || excluded.Metrics.ProviderRequests == nil || *excluded.Metrics.ProviderRequests != 0 ||
		excluded.Metrics.LLMCallsStarted == nil || *excluded.Metrics.LLMCallsStarted != 0 ||
		excluded.Metrics.CatalogCost == nil || *excluded.Metrics.CatalogCost != 0 {
		t.Fatalf("excluded no-provider run lacks structural-zero telemetry: trial=%v requests=%v llm_calls=%v cost=%v", excluded.TrialDurationSeconds, excluded.Metrics.ProviderRequests, excluded.Metrics.LLMCallsStarted, excluded.Metrics.CatalogCost)
	}
	var codexPublic *harness.DeepSWEPublicAgentScore
	for index := range experiment.PublicScorecard.Agents {
		if experiment.PublicScorecard.Agents[index].AgentID == "codex" {
			codexPublic = &experiment.PublicScorecard.Agents[index]
		}
	}
	if codexPublic == nil || codexPublic.Counts.Excluded != 1 || codexPublic.LivePooled.Rate != nil || codexPublic.TaskMacro.Rate != nil || codexPublic.Tasks[0].Rate != nil {
		t.Fatalf("public excluded score was coerced to a numeric rate: %#v", codexPublic)
	}
	var rendered bytes.Buffer
	if err := Render(&rendered, data); err != nil {
		t.Fatalf("Render typed exclusion: %v", err)
	}
	if strings.Contains(rendered.String(), "%!") || !strings.Contains(rendered.String(), "network_infrastructure") {
		t.Fatalf("rendered typed exclusion is malformed or omits its category")
	}
	completeSpendPassed := false
	for _, gate := range experiment.Gates {
		if gate.Name == "complete_spend" && gate.Status == GatePass {
			completeSpendPassed = true
		}
	}
	if !completeSpendPassed {
		t.Fatal("zero-attempt exclusion did not preserve exact zero spend")
	}
}

func TestCompileUsesNormalizedEffectiveTimeoutOutcomeEverywhere(t *testing.T) {
	fixture := buildFormalBundleFixture(t)
	rewriteFormalStateAndScorecard(t, fixture, func(state *harness.ExperimentState, plan harness.RunPlan) {
		for _, entry := range plan.Entries {
			if entry.AgentID != "codex" {
				continue
			}
			key := harness.RunKey(entry)
			record := state.Runs[key]
			rawReward := record.Verification.Reward
			record.Verification.RawReward = &rawReward
			record.Verification.Reward = 0
			record.Execution.ExitClass = "timeout"
			record.FailureCategory = harness.DeepSWEFailureAgentTimeout
			record.Execution.TerminalEvidence = harness.AgentTerminalEvidence{
				SchemaVersion: "agentic-bench/terminal-evidence-v1", Source: "pier_trial", Code: "agent_timeout", EvidenceSHA256: formalHex("e", 64),
			}
			state.Runs[key] = record
		}
	})
	var input Input
	readJSONForFormalFixture(t, fixture.inputPath, &input)
	input.FailureAnnotations = []FailureAnnotation{{
		ExperimentID: "formal-fixture", TaskID: "task-a", AgentID: "codex", Repetition: 0,
		Category: FailureTimeout, Summary: "typed timeout fixture", Evidence: []string{"pier_trial/agent_timeout"},
	}}
	writeJSONForFormalFixture(t, fixture.inputPath, input)

	data, err := compileSyntheticFormalFixture(fixture.inputPath)
	if err != nil {
		t.Fatalf("Compile normalized timeout: %v", err)
	}
	experiment := data.Experiments[0]
	for _, run := range experiment.Runs {
		if run.AgentID == "codex" && (run.Passed == nil || *run.Passed) {
			t.Fatalf("RunData retained raw verifier pass instead of effective timeout failure: %#v", run.Passed)
		}
	}
	for _, agent := range experiment.PublicScorecard.Agents {
		if agent.AgentID == "codex" && (agent.Counts.Passed != 0 || agent.Counts.Failed != 1 || agent.ScoredFailuresByCategory[harness.DeepSWEFailureAgentTimeout] != 1) {
			t.Fatalf("public scorecard did not use effective timeout outcome: %#v", agent)
		}
	}
	if len(experiment.Comparisons) != 1 || experiment.Comparisons[0].QualityWins != 1 || experiment.Comparisons[0].QualityLosses != 0 {
		t.Fatalf("paired quality comparison did not use normalized timeout outcome: %#v", experiment.Comparisons)
	}
}

func TestCompileAcceptsDistinctOfficialAndFullWorkspaceCaptureV2(t *testing.T) {
	fixture := buildFormalBundleFixture(t)
	rewriteFormalStateAndScorecard(t, fixture, func(state *harness.ExperimentState, plan harness.RunPlan) {
		entry := plan.Entries[0]
		key := harness.RunKey(entry)
		record := state.Runs[key]
		officialRaw, err := os.ReadFile(record.Execution.SubmissionPatch)
		if err != nil {
			t.Fatal(err)
		}
		auditRaw := append(bytes.Clone(officialRaw), []byte("audit-only staged, unstaged, and untracked workspace state\n")...)
		writeFileForFormalFixture(t, record.Execution.AuditWorkspacePatch, auditRaw)
		auditSHA, err := harness.HashFile(record.Execution.AuditWorkspacePatch)
		if err != nil {
			t.Fatal(err)
		}
		record.Execution.Capture.AuditPatchSHA256 = auditSHA
		record.Execution.Capture.UncommittedChangesPresent = true
		writeCaptureReceiptForFormalFixture(t, record.ArtifactDir, record.Execution.Capture)
		state.Runs[key] = record
	})

	data, err := compileSyntheticFormalFixture(fixture.inputPath)
	if err != nil {
		t.Fatalf("Compile distinct official/audit capture v2: %v", err)
	}
	if len(data.Experiments) != 1 || len(data.Experiments[0].Runs) != 2 {
		t.Fatalf("compiled capture-v2 fixture shape = %#v", data.Experiments)
	}
}

func TestCompileRejectsInconsistentWorkspaceCaptureV2(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(testing.TB, *harness.RunRecord)
	}{
		{name: "legacy method", mutate: func(t testing.TB, record *harness.RunRecord) {
			record.Execution.Capture.Method = "temporary-git-index-v1"
		}},
		{name: "invalid base commit", mutate: func(t testing.TB, record *harness.RunRecord) {
			record.Execution.Capture.BaseCommit = strings.Repeat("z", 40)
		}},
		{name: "same artifact path", mutate: func(t testing.TB, record *harness.RunRecord) {
			record.Execution.AuditWorkspacePatch = record.Execution.SubmissionPatch
			record.Execution.Capture.AuditPatchSHA256 = record.Execution.Capture.PatchSHA256
			record.Execution.Capture.UncommittedChangesPresent = false
		}},
		{name: "audit digest mismatch", mutate: func(t testing.TB, record *harness.RunRecord) {
			record.Execution.Capture.AuditPatchSHA256 = strings.Repeat("0", 64)
		}},
		{name: "uncommitted flag mismatch", mutate: func(t testing.TB, record *harness.RunRecord) {
			auditRaw := []byte("different full workspace patch\n")
			writeFileForFormalFixture(t, record.Execution.AuditWorkspacePatch, auditRaw)
			auditSHA, err := harness.HashFile(record.Execution.AuditWorkspacePatch)
			if err != nil {
				t.Fatal(err)
			}
			record.Execution.Capture.AuditPatchSHA256 = auditSHA
			record.Execution.Capture.UncommittedChangesPresent = false
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := buildFormalBundleFixture(t)
			rewriteFormalStateAndScorecard(t, fixture, func(state *harness.ExperimentState, plan harness.RunPlan) {
				entry := plan.Entries[0]
				key := harness.RunKey(entry)
				record := state.Runs[key]
				test.mutate(t, &record)
				writeCaptureReceiptForFormalFixture(t, record.ArtifactDir, record.Execution.Capture)
				state.Runs[key] = record
			})
			if _, err := compileSyntheticFormalFixture(fixture.inputPath); err == nil {
				t.Fatal("Compile accepted inconsistent workspace-capture-v2 evidence")
			}
		})
	}
}

func TestFormalReportRejectsPublicReferenceArtifactBypass(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*Input)
	}{
		{name: "missing", mutate: func(input *Input) { input.PublicReferences = nil }},
		{name: "text-only", mutate: func(input *Input) { input.PublicReferences[0].ComputedArtifact = "" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := buildFormalBundleFixture(t)
			var input Input
			readJSONForFormalFixture(t, fixture.inputPath, &input)
			test.mutate(&input)
			writeJSONForFormalFixture(t, fixture.inputPath, input)
			_, err := compileSyntheticFormalFixture(fixture.inputPath)
			if err == nil || !strings.Contains(err.Error(), "exactly one registered, locally recomputed public reference artifact") {
				t.Fatalf("Compile error = %v, want formal public-reference bypass rejection", err)
			}
		})
	}
}

func TestCompileRejectsTamperedFormalHarnessBundle(t *testing.T) {
	t.Run("artifact content", func(t *testing.T) {
		fixture := buildFormalBundleFixture(t)
		if err := os.WriteFile(fixture.verifierPaths[0], []byte("{\"reward\":0}\n"), 0o600); err != nil {
			t.Fatalf("tamper verifier artifact: %v", err)
		}

		_, err := compileSyntheticFormalFixture(fixture.inputPath)
		if err == nil {
			t.Fatal("Compile accepted an artifact whose bytes differ from the content ledger")
		}
		if !strings.Contains(err.Error(), "artifact inventory") {
			t.Fatalf("Compile error = %q, want content-ledger inventory rejection", err)
		}
	})

	t.Run("ledger semantics despite refreshed outer pin", func(t *testing.T) {
		fixture := buildFormalBundleFixture(t)

		var ledger harness.ArtifactLedger
		readJSONForFormalFixture(t, fixture.ledgerPath, &ledger)
		ledger.LedgerSHA256 = strings.Repeat("0", 64)
		writeJSONForFormalFixture(t, fixture.ledgerPath, ledger)

		var input Input
		readJSONForFormalFixture(t, fixture.inputPath, &input)
		ledgerFileSHA, err := harness.HashFile(fixture.ledgerPath)
		if err != nil {
			t.Fatalf("hash tampered ledger: %v", err)
		}
		input.ArtifactSources[0].LedgerFileSHA256 = ledgerFileSHA
		writeJSONForFormalFixture(t, fixture.inputPath, input)

		_, err = compileSyntheticFormalFixture(fixture.inputPath)
		if err == nil {
			t.Fatal("Compile accepted a semantically tampered ledger after its outer file pin was refreshed")
		}
		if !strings.Contains(err.Error(), "canonical ledger hash changed") {
			t.Fatalf("Compile error = %q, want canonical-ledger rejection", err)
		}
	})

	t.Run("raw provider hash chain despite refreshed file pins", func(t *testing.T) {
		fixture := buildFormalBundleFixture(t)
		statePath := filepath.Join(fixture.artifactRoot, "state.json")
		var state harness.ExperimentState
		readJSONForFormalFixture(t, statePath, &state)
		plan := formalPlan(t, fixture)
		key := harness.RunKey(plan.Entries[0])
		record := state.Runs[key]
		rawPath := record.Execution.ProviderEvidence.RawEvidencePath
		var rawRecord evidenceproxy.Record
		readJSONForFormalFixture(t, rawPath, &rawRecord)
		rawRecord.ResponseBytes++
		writeJSONForFormalFixture(t, rawPath, rawRecord)
		refreshedRawSHA, err := harness.HashFile(rawPath)
		if err != nil {
			t.Fatalf("hash rewritten raw provider evidence: %v", err)
		}
		record.Execution.ProviderEvidence.RawEvidenceSHA256 = refreshedRawSHA
		state.Runs[key] = record
		writeJSONForFormalFixture(t, statePath, state)
		repinFormalBundle(t, fixture)

		_, err = compileSyntheticFormalFixture(fixture.inputPath)
		if err == nil || !strings.Contains(err.Error(), "validate archived provider evidence chain") {
			t.Fatalf("Compile error = %v, want raw provider hash-chain rejection", err)
		}
	})

	t.Run("normalized projection despite refreshed file pins", func(t *testing.T) {
		fixture := buildFormalBundleFixture(t)
		statePath := filepath.Join(fixture.artifactRoot, "state.json")
		var state harness.ExperimentState
		readJSONForFormalFixture(t, statePath, &state)
		plan := formalPlan(t, fixture)
		record := state.Runs[harness.RunKey(plan.Entries[0])]
		var normalized harness.ProviderRoundEvidence
		readJSONForFormalFixture(t, record.Execution.EvidencePath, &normalized)
		normalized.RequestBytes++
		encodedNormalized, err := json.Marshal(normalized)
		if err != nil {
			t.Fatalf("marshal rewritten normalized provider evidence: %v", err)
		}
		writeFileForFormalFixture(t, record.Execution.EvidencePath, append(encodedNormalized, '\n'))
		repinFormalBundle(t, fixture)

		_, err = compileSyntheticFormalFixture(fixture.inputPath)
		if err == nil || !strings.Contains(err.Error(), "raw-v6 provider projection") {
			t.Fatalf("Compile error = %v, want raw-v6 projection rejection", err)
		}
	})

	t.Run("report reverses frozen scoring direction", func(t *testing.T) {
		fixture := buildFormalBundleFixture(t)
		var input Input
		readJSONForFormalFixture(t, fixture.inputPath, &input)
		input.Report.BaselineAgentID, input.Report.ContenderAgentID = input.Report.ContenderAgentID, input.Report.BaselineAgentID
		writeJSONForFormalFixture(t, fixture.inputPath, input)

		_, err := compileSyntheticFormalFixture(fixture.inputPath)
		if err == nil || !strings.Contains(err.Error(), "frozen scoring direction") {
			t.Fatalf("Compile error = %v, want frozen scoring-direction rejection", err)
		}
	})

	t.Run("plan separates or reorders a deterministic pair", func(t *testing.T) {
		fixture := buildFormalBundleFixture(t)
		planPath := filepath.Join(fixture.artifactRoot, "plan.json")
		statePath := filepath.Join(fixture.artifactRoot, "state.json")
		var plan harness.RunPlan
		readJSONForFormalFixture(t, planPath, &plan)
		plan.Entries[0], plan.Entries[1] = plan.Entries[1], plan.Entries[0]
		for ordinal := range plan.Entries {
			plan.Entries[ordinal].Ordinal = ordinal
		}
		writeJSONForFormalFixture(t, planPath, plan)
		planSHA, err := harness.HashCanonical(plan)
		if err != nil {
			t.Fatalf("hash reordered plan: %v", err)
		}
		var state harness.ExperimentState
		readJSONForFormalFixture(t, statePath, &state)
		state.PlanSHA256 = planSHA
		for _, entry := range plan.Entries {
			record := state.Runs[harness.RunKey(entry)]
			record.Entry = entry
			state.Runs[harness.RunKey(entry)] = record
		}
		writeJSONForFormalFixture(t, statePath, state)
		repinFormalBundle(t, fixture)

		_, err = compileSyntheticFormalFixture(fixture.inputPath)
		if err == nil || !strings.Contains(err.Error(), "deterministic adjacent paired schedule") {
			t.Fatalf("Compile error = %v, want deterministic paired-schedule rejection", err)
		}
	})
}

func compileSyntheticFormalFixture(inputPath string) (Data, error) {
	return compileWithPublishedBenchmarkValidator(inputPath, func(ExperimentClass, ReportMeta, harness.Manifest, harness.RunPlan, harness.ExperimentState) error {
		return nil
	})
}

func repinFormalBundle(t *testing.T, fixture formalBundleFixture) {
	t.Helper()
	loaded, err := harness.LoadManifest(filepath.Join(fixture.artifactRoot, "manifest.json"))
	if err != nil {
		t.Fatalf("reload fixture manifest: %v", err)
	}
	ledger, err := harness.BuildArtifactLedger(fixture.artifactRoot, loaded.Manifest.Artifacts.LedgerRelativePath, loaded.SHA256)
	if err != nil {
		t.Fatalf("rebuild fixture ledger: %v", err)
	}
	writeJSONForFormalFixture(t, fixture.ledgerPath, ledger)
	ledgerFileSHA, err := harness.HashFile(fixture.ledgerPath)
	if err != nil {
		t.Fatalf("hash rebuilt fixture ledger: %v", err)
	}
	var input Input
	readJSONForFormalFixture(t, fixture.inputPath, &input)
	input.ArtifactSources[0].LedgerFileSHA256 = ledgerFileSHA
	writeJSONForFormalFixture(t, fixture.inputPath, input)
}

func rewriteFormalStateAndScorecard(t *testing.T, fixture formalBundleFixture, mutate func(*harness.ExperimentState, harness.RunPlan)) {
	t.Helper()
	statePath := filepath.Join(fixture.artifactRoot, "state.json")
	planPath := filepath.Join(fixture.artifactRoot, "plan.json")
	var state harness.ExperimentState
	var plan harness.RunPlan
	readJSONForFormalFixture(t, statePath, &state)
	readJSONForFormalFixture(t, planPath, &plan)
	mutate(&state, plan)
	writeJSONForFormalFixture(t, statePath, state)
	loaded, err := harness.LoadManifest(filepath.Join(fixture.artifactRoot, "manifest.json"))
	if err != nil {
		t.Fatalf("reload fixture manifest: %v", err)
	}
	scorecard, err := harness.ScoreExperimentForManifest(loaded, state, plan)
	if err != nil {
		t.Fatalf("rescore mutated formal fixture: %v", err)
	}
	writeJSONForFormalFixture(t, filepath.Join(fixture.artifactRoot, "scorecard.json"), scorecard)
	repinFormalBundle(t, fixture)
}

func buildFormalBundleFixture(t *testing.T) formalBundleFixture {
	t.Helper()
	temporaryRoot := t.TempDir()
	artifactRoot := filepath.Join(temporaryRoot, "formal")
	if err := os.MkdirAll(artifactRoot, 0o755); err != nil {
		t.Fatalf("create artifact root: %v", err)
	}

	binaryPath := filepath.Join(temporaryRoot, "bin", "fixture-agent")
	if err := os.MkdirAll(filepath.Dir(binaryPath), 0o755); err != nil {
		t.Fatalf("create binary directory: %v", err)
	}
	if err := os.WriteFile(binaryPath, []byte("formal fixture agent\n"), 0o755); err != nil {
		t.Fatalf("write fixture binary: %v", err)
	}
	binarySHA, err := harness.HashFile(binaryPath)
	if err != nil {
		t.Fatalf("hash fixture binary: %v", err)
	}

	model := harness.ModelRequestSpec{Provider: "openai", Model: "gpt-5.6-sol", ReasoningEffort: "xhigh", ServiceTier: harness.FormalServiceTier}
	benchmarkTime := time.Date(2026, 7, 26, 1, 0, 0, 0, time.UTC)
	lubanSource := buildFormalSourceFixture(t, temporaryRoot, artifactRoot, binarySHA, benchmarkTime)
	agents := []harness.AgentSpec{
		formalAgentSpec(binaryPath, binarySHA, "codex", nil, model),
		formalAgentSpec(binaryPath, binarySHA, "luban", lubanSource, model),
	}
	task := harness.Task{
		ID: "task-a", BaseCommit: formalHex("a", 40), ManifestSHA256: formalHex("b", 64),
		Image: "registry.example/task:a", ImageDigest: "sha256:" + formalHex("c", 64), InstructionSHA256: formalHex("d", 64),
	}
	inventorySHA, err := harness.HashTaskInventory([]harness.Task{task})
	if err != nil {
		t.Fatalf("hash task inventory: %v", err)
	}
	manifest := harness.Manifest{
		SchemaVersion:    harness.SchemaVersion,
		Experiment:       harness.ExperimentSpec{ID: "formal-fixture", Description: "one-task formal report fixture"},
		ProviderEndpoint: harness.FormalProviderEndpoint(),
		Dataset: harness.SourcePin{
			Name: "deep-swe-v1.1", Repository: "https://github.com/datacurve-ai/deep-swe",
			Commit: formalHex("3", 40), Root: "tasks", TreeSHA256: formalHex("4", 64), ManifestSHA256: inventorySHA,
		},
		Evaluator: harness.EvaluatorSpec{
			SourcePin: harness.SourcePin{
				Name: "pier", Repository: "https://github.com/datacurve-ai/pier",
				Commit: formalHex("6", 40), Root: "src", TreeSHA256: formalHex("7", 64), ManifestSHA256: formalHex("8", 64),
			},
			Protocol: "pier-harbor-separate-verifier", MinimumVersion: "0.3.0", BinarySHA256: formalHex("9", 64),
		},
		Agents:     agents,
		Selection:  harness.SelectionSpec{Mode: "full", ExpectedTaskCount: 1},
		Scheduling: harness.SchedulingSpec{PairAgents: true, Seed: 42, Repetitions: 1, MaxParallelPairs: 1},
		Scoring: harness.ScoringSpec{
			Profile: harness.ScoringProfileDeepSWEV11PublicCI, BaselineAgentID: "codex", ChallengerAgentID: "luban",
		},
		Environment: harness.EnvironmentSpec{
			AgentEgressHosts: []string{"host.docker.internal"}, TaskNetworkMode: "no-network", VerifierNetworkMode: "no-network",
		},
		Timeouts: harness.TimeoutSpec{SetupSeconds: 60, AgentSeconds: 900, VerifierSeconds: 300, TeardownSeconds: 60},
		Resources: harness.ResourceSpec{
			CPUs: 2, MemoryMB: 4096, StorageMB: 20480,
			HostStorageGuard: harness.FormalHostStorageGuard(), GuestStorageGuard: harness.FormalGuestStorageGuard(),
		},
		Pricing: harness.PricingCatalog{
			Currency: "USD", UnitTokens: 1_000_000,
			EffectiveAt: time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC), ObservedAt: time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC),
			SourceURL: "https://developers.openai.com/api/docs/models/gpt-5.6-sol",
			Rates: []harness.PricingRate{{
				Provider: model.Provider, Model: model.Model, Input: 5, CachedInput: 0.5, Output: 30,
				CacheWriteInputMultiplier: 1.25,
				RequestTiers: []harness.PricingTier{{
					Name: "long-context", ThresholdInputTokens: 272000,
					InputMultiplier: 2, CachedInputMultiplier: 2, OutputMultiplier: 1.5,
				}},
			}},
		},
		Artifacts: harness.ArtifactSpec{
			Root: "formal", LedgerRelativePath: "ledger.json", StateRelativePath: "state.json",
			CaptureBinaryDiff: true, CaptureUntracked: true,
		},
		Oracle: harness.OracleSpec{Required: true, SeparateEnvironment: true, SolutionRoot: "solution"},
	}

	inventoryLockPath := filepath.Join(artifactRoot, harness.InventoryLockArchiveRelativePath)
	writeJSONForFormalFixture(t, inventoryLockPath, map[string]any{
		"schema_version": harness.PierInventoryLockSchemaVersion, "dataset_commit": manifest.Dataset.Commit,
		"coverage": "full", "universe_task_count": 1,
		"tasks": []map[string]any{{
			"id": task.ID, "relative_path": task.ID, "base_commit": task.BaseCommit,
			"manifest_sha256": task.ManifestSHA256, "instruction_sha256": task.InstructionSHA256,
			"image": task.Image, "image_digest": task.ImageDigest,
		}},
	})
	inventoryLockSHA, err := harness.HashFile(inventoryLockPath)
	if err != nil {
		t.Fatalf("hash inventory lock: %v", err)
	}
	manifest.Dataset.InventoryLockFileSHA256 = inventoryLockSHA
	manifestPath := filepath.Join(artifactRoot, "manifest.json")
	writeJSONForFormalFixture(t, manifestPath, manifest)
	loadedManifest, err := harness.LoadManifest(manifestPath)
	if err != nil {
		t.Fatalf("load generated manifest: %v", err)
	}
	inventoryLockSnapshot := harness.InventoryLockSnapshot{
		RelativePath: harness.InventoryLockArchiveRelativePath, FileSHA256: inventoryLockSHA,
		SchemaVersion: harness.PierInventoryLockSchemaVersion, HashAlgorithm: harness.TaskInventoryHashAlgorithm,
		DatasetCommit: manifest.Dataset.Commit, Coverage: "full", TaskCount: 1, UniverseTaskCount: 1,
		TaskInventorySHA256: manifest.Dataset.ManifestSHA256,
	}
	if _, err := harness.ValidateInventoryLockArchive(inventoryLockPath, inventoryLockSnapshot); err != nil {
		t.Fatalf("validate inventory lock: %v", err)
	}
	plan, err := harness.BuildPlan(loadedManifest.SHA256, manifest, []harness.Task{task})
	if err != nil {
		t.Fatalf("build paired run plan: %v", err)
	}
	writeJSONForFormalFixture(t, filepath.Join(artifactRoot, "plan.json"), plan)
	planSHA, err := harness.HashCanonical(plan)
	if err != nil {
		t.Fatalf("hash run plan: %v", err)
	}

	completedAt := benchmarkTime.Add(10 * time.Minute)
	storagePreflight := formalStorageAdmissionForFixture(
		harness.StorageStageExperimentPreflight,
		benchmarkTime.Add(-10*time.Minute),
		120*formalFixtureGiB,
	)
	oracleArtifactPath := filepath.Join(artifactRoot, "oracle", task.ID, "verifier", "reward.json")
	writeFileForFormalFixture(t, oracleArtifactPath, []byte("{\"reward\":1}\n"))
	state := harness.ExperimentState{
		SchemaVersion: "agentic-bench/state-v2", ManifestSHA256: loadedManifest.SHA256, PlanSHA256: planSHA,
		Status: harness.ExperimentComplete, StartedAt: benchmarkTime, UpdatedAt: completedAt, CompletedAt: &completedAt,
		Backend: harness.BackendSnapshot{
			Dataset: harness.SourceSnapshot{
				Commit: manifest.Dataset.Commit, TreeSHA256: manifest.Dataset.TreeSHA256, ManifestSHA256: manifest.Dataset.ManifestSHA256,
			},
			Evaluator: harness.SourceSnapshot{
				Commit: manifest.Evaluator.Commit, TreeSHA256: manifest.Evaluator.TreeSHA256, ManifestSHA256: manifest.Evaluator.ManifestSHA256,
			},
			EvaluatorVersion: manifest.Evaluator.MinimumVersion, EvaluatorBinarySHA256: manifest.Evaluator.BinarySHA256,
			InventoryLock:     inventoryLockSnapshot,
			InventoryCoverage: "full", InventoryTaskCount: 1, UniverseTaskCount: 1,
			AgentNetworkDeny: true, VerifierNetworkDeny: true, NetworkAttestation: "formal-fixture-runtime",
			EgressProxyImage:   "registry.example/proxy@sha256:" + strings.Repeat("e", 64),
			EgressProxyImageID: "sha256:" + strings.Repeat("f", 64),
			AdapterImportPath:  "fixture:Agent",
			AdapterVersion:     "1.0.0",
			AdapterSHA256:      strings.Repeat("a", 64),
			ProviderEndpoint: harness.ProviderEndpointSnapshot{
				ApprovedOrigin: harness.FormalProviderOrigin, SemanticsSHA256: harness.FormalProviderEndpointSemanticsSHA256,
				TLSServerName: harness.FormalProviderTLSServerName, TLSVerified: true,
				TLSPeerLeafCertSHA256: strings.Repeat("b", 64), TLSPeerSPKISHA256: strings.Repeat("c", 64),
			},
			AgentExecutionCanaries: []harness.ExecutionCanarySnapshot{
				{AgentID: "codex", Generation: harness.FormalExecutionCanaryGeneration, TransportRequirement: harness.TransportRequirementHTTPInference, ReceiptSHA256: formalHex("8", 64)},
				{AgentID: "luban", Generation: harness.FormalExecutionCanaryGeneration, TransportRequirement: harness.TransportRequirementHTTPInference, ReceiptSHA256: formalHex("9", 64)},
			},
			StorageEnforcement: harness.FormalStorageEnforcement,
			HostStorageGuard:   harness.FormalHostStorageGuard(),
			GuestStorageGuard:  harness.FormalGuestStorageGuard(),
			StoragePreflight:   storagePreflight,
		},
		Agents: []harness.AgentSnapshot{
			{AgentID: "codex", BinarySHA256: binarySHA, CapturedAt: benchmarkTime},
			{AgentID: "luban", BinarySHA256: binarySHA, Source: &harness.AgentSourceSnapshot{
				BaseCommit: lubanSource.BaseCommit, TreeOID: lubanSource.TreeOID,
				PatchSHA256: lubanSource.PatchSHA256, ArchiveSHA256: lubanSource.ArchiveSHA256,
				PathPolicy: lubanSource.PathPolicy, PathPolicySHA256: lubanSource.PathPolicySHA256,
				ExclusionReceiptSHA256: lubanSource.ExclusionReceiptSHA256,
				BuildReceiptSHA256:     lubanSource.BuildReceiptSHA256,
			}, CapturedAt: benchmarkTime},
		},
		Oracle: map[string]harness.OracleRecord{
			task.ID: {
				TaskID: task.ID, Validated: true,
				Verification: harness.VerificationResult{ProtocolValid: true, Reward: 1, ArtifactPaths: []string{oracleArtifactPath}},
			},
		},
		Runs: map[string]harness.RunRecord{},
	}

	verifierPaths := make([]string, 0, len(plan.Entries))
	for _, entry := range plan.Entries {
		agent := formalAgentByID(t, agents, entry.AgentID)
		record, verifierPath := buildFormalRunFixture(t, artifactRoot, task, agent, entry, manifest.Pricing, benchmarkTime)
		state.Runs[harness.RunKey(entry)] = record
		verifierPaths = append(verifierPaths, verifierPath)
	}
	writeJSONForFormalFixture(t, filepath.Join(artifactRoot, manifest.Artifacts.StateRelativePath), state)
	scorecard, err := harness.ScoreExperimentForManifest(loadedManifest, state, plan)
	if err != nil {
		t.Fatalf("score generated state: %v", err)
	}
	writeJSONForFormalFixture(t, filepath.Join(artifactRoot, "scorecard.json"), scorecard)

	ledger, err := harness.BuildArtifactLedger(artifactRoot, manifest.Artifacts.LedgerRelativePath, loadedManifest.SHA256)
	if err != nil {
		t.Fatalf("build content ledger: %v", err)
	}
	ledgerPath := filepath.Join(artifactRoot, manifest.Artifacts.LedgerRelativePath)
	writeJSONForFormalFixture(t, ledgerPath, ledger)
	ledgerFileSHA, err := harness.HashFile(ledgerPath)
	if err != nil {
		t.Fatalf("hash content ledger file: %v", err)
	}

	optimizationPath := filepath.Join(temporaryRoot, "optimization-ledger.json")
	writeJSONForFormalFixture(t, optimizationPath, OptimizationLedger{SchemaVersion: OptimizationSchemaVersion, Entries: []OptimizationEntry{}})
	optimizationSHA, err := harness.HashFile(optimizationPath)
	if err != nil {
		t.Fatalf("hash optimization ledger: %v", err)
	}
	input := Input{
		SchemaVersion: InputSchemaVersion,
		Report: ReportMeta{
			Title: "Formal fixture report", Subtitle: "content-addressed integration test",
			Benchmark: "Synthetic formal fixture", BenchmarkVersion: "v1", BenchmarkContractID: BenchmarkContractDeepSWEV11Full113, Language: "en",
			BaselineAgentID: "codex", ContenderAgentID: "luban", AsOf: completedAt,
		},
		Statistics: StatisticsSpec{
			ConfidenceLevel: ReportConfidenceLevel, Method: ReportStatisticsMethod, Resamples: ReportStatisticsResamples, Seed: ReportStatisticsSeed,
		},
		ArtifactSources: []ArtifactSource{{
			ID: "formal-fixture", Label: "Formal fixture", Class: ClassFormal, Root: "formal",
			LedgerFileSHA256: ledgerFileSHA, Description: "one task, one repetition, and two paired agents",
		}},
		DiagnosticExperiments: []DiagnosticExperiment{},
		OptimizationLedger:    FileReference{Path: "optimization-ledger.json", SHA256: optimizationSHA},
		PublicReferences: []PublicReference{{
			ID: "official-deepswe-reference", Benchmark: "DeepSWE", Version: "v1.1",
			Agent: "mini-swe-agent", Model: "gpt-5.6-sol", ReasoningEffort: "xhigh",
			ComputedArtifact: computedDeepSWEGPT56SolXHighReference, Components: []ReferenceComponent{},
			SourceURL: harness.DeepSWEGPT56SolXHighSourceURL, AccessedAt: benchmarkTime,
			Notes: "locally recomputed formal fixture reference",
		}},
		FailureAnnotations: []FailureAnnotation{},
		Reproduction:       []ReproductionCommand{},
		Limitations:        []string{"The fixture contains one task and exists only to exercise the formal artifact trust chain."},
	}
	inputPath := filepath.Join(temporaryRoot, "report-input.json")
	writeJSONForFormalFixture(t, inputPath, input)

	return formalBundleFixture{
		inputPath: inputPath, artifactRoot: artifactRoot, ledgerPath: ledgerPath, verifierPaths: verifierPaths,
	}
}

func formalAgentSpec(binaryPath, binarySHA, id string, source *harness.AgentSourceSpec, model harness.ModelRequestSpec) harness.AgentSpec {
	model.ServiceTierRequestEncoding = harness.ServiceTierEncodingExplicitDefault
	model.TransportRequirement = harness.TransportRequirementHTTPInference
	definitions := formalToolDefinitionsForFixture(id)
	tools := make([]harness.ToolIdentitySpec, 0, len(definitions))
	for _, definition := range definitions {
		tools = append(tools, harness.ToolIdentitySpec{
			Type: definition.Type, Name: definition.Name, DefinitionSHA256: definition.DefinitionSHA256,
		})
	}
	model.ToolCatalog = harness.ToolCatalogSpec{
		SchemaVersion:  harness.FormalToolCatalogSchemaVersion,
		SemanticSHA256: harness.StableToolCatalogSHA256(definitions),
		Tools:          tools,
	}
	argv := []string{binaryPath, "--print", "--output-format", "stream-json", "--provider", "openai", "--api", "responses",
		"--model", "gpt-5.6-sol", "--reasoning-effort", "xhigh", "--service-tier", "default", "--pinned-model", "--no-model-fallback",
		"--allow-all", "--force-sandbox-tools", "{instruction_path}"}
	if id == "codex" {
		model.ServiceTierRequestEncoding = harness.ServiceTierEncodingClientCanonical
		argv = []string{binaryPath, "--ask-for-approval", "never", "--sandbox", "workspace-write", "exec", "--json", "--ephemeral",
			"--ignore-user-config", "--model", "gpt-5.6-sol", "--config", "model_reasoning_effort=xhigh", "--config", `service_tier="default"`, "--config", `web_search="disabled"`,
			"--config", "agents.enabled=false", "{instruction_path}"}
	}
	return harness.AgentSpec{
		ID: id, Binary: binaryPath, BinarySHA256: binarySHA, SourceSnapshot: source,
		Command: harness.CommandSpec{Argv: argv}, Model: model,
		ExecutionCanary: harness.ExecutionCanarySpec{Generation: harness.FormalExecutionCanaryGeneration, ReceiptSHA256: map[string]string{"codex": formalHex("8", 64), "luban": formalHex("9", 64)}[id]},
		RequestEvidence: harness.RequestEvidenceSpec{RelativePath: "metrics/provider-requests.jsonl", Required: true},
	}
}

func formalToolDefinitionsForFixture(agentID string) []harness.ToolDefinitionEvidence {
	identities := []struct {
		typeName string
		name     string
	}{}
	switch agentID {
	case "codex":
		identities = append(identities,
			struct{ typeName, name string }{"custom", "exec"},
			struct{ typeName, name string }{"function", "wait"},
			struct{ typeName, name string }{"function", "request_user_input"},
		)
	case "luban":
		identities = append(identities,
			struct{ typeName, name string }{"function", "Inspect"},
			struct{ typeName, name string }{"function", "ApplyPatch"},
			struct{ typeName, name string }{"function", "Run"},
		)
	}
	definitions := make([]harness.ToolDefinitionEvidence, 0, len(identities))
	for index, identity := range identities {
		definitions = append(definitions, harness.ToolDefinitionEvidence{
			Type: identity.typeName, Name: identity.name, BillingOwner: "client",
			SchemaHash: formalHex(string(rune('1'+index)), 64), SchemaSHA256: formalHex(string(rune('4'+index)), 64), SchemaBytes: 1,
			DefinitionSHA256: formalHex(string(rune('a'+index)), 64), DefinitionBytes: 1,
		})
	}
	return definitions
}

func formalRawToolDefinitionsForFixture(definitions []harness.ToolDefinitionEvidence) []evidenceproxy.ToolDefinitionEvidence {
	result := make([]evidenceproxy.ToolDefinitionEvidence, 0, len(definitions))
	for _, definition := range definitions {
		result = append(result, evidenceproxy.ToolDefinitionEvidence{
			Type: definition.Type, Name: definition.Name, BillingOwner: definition.BillingOwner, Strict: definition.Strict,
			SchemaHash: definition.SchemaHash, SchemaSHA256: definition.SchemaSHA256, SchemaBytes: definition.SchemaBytes,
			DescriptionSHA256: definition.DescriptionSHA256, DescriptionBytes: definition.DescriptionBytes,
			DefinitionSHA256: definition.DefinitionSHA256, DefinitionBytes: definition.DefinitionBytes,
		})
	}
	return result
}

func buildFormalSourceFixture(t *testing.T, temporaryRoot, artifactRoot, binarySHA string, builtAt time.Time) *harness.AgentSourceSpec {
	t.Helper()
	sourceDir := filepath.Join(artifactRoot, "sources", "luban")
	patchPath := filepath.Join(sourceDir, "source.patch")
	archivePath := filepath.Join(sourceDir, "source.tar")
	writeFileForFormalFixture(t, patchPath, []byte("diff --git a/luban.go b/luban.go\n"))
	writeTarForFormalFixture(t, archivePath, map[string][]byte{
		"provider/compatible.go": []byte("package provider\n\nconst formalSourceFixture = true\n"),
	})
	patchSHA, err := harness.HashFile(patchPath)
	if err != nil {
		t.Fatalf("hash source patch: %v", err)
	}
	archiveSHA, err := harness.HashFile(archivePath)
	if err != nil {
		t.Fatalf("hash source archive: %v", err)
	}
	pathPolicy := harness.FormalSourcePathPolicy()
	pathPolicySHA, err := harness.HashCanonical(pathPolicy)
	if err != nil {
		t.Fatalf("hash source path policy: %v", err)
	}
	if pathPolicySHA != "86059e44a68eb7d36f7d4953d53f90945ca4f2a94a83c98c560d670afbf980b5" {
		t.Fatalf("formal source path policy SHA-256 = %s", pathPolicySHA)
	}
	exclusion := harness.SourceExclusionReceipt{
		SchemaVersion: "agentic-bench/source-exclusion-receipt-v1", PathPolicy: pathPolicy,
		PathPolicySHA256: pathPolicySHA, Applied: true, Implementation: "git-negative-pathspec-before-content-scan-v1",
	}
	exclusionRaw, err := json.Marshal(exclusion)
	if err != nil {
		t.Fatalf("marshal source exclusion receipt: %v", err)
	}
	exclusionPath := filepath.Join(sourceDir, "source-exclusions.json")
	writeFileForFormalFixture(t, exclusionPath, append(exclusionRaw, '\n'))
	exclusionSHA, err := harness.HashFile(exclusionPath)
	if err != nil {
		t.Fatalf("hash source exclusion receipt: %v", err)
	}
	if exclusionSHA != "6ea05139a2686d237ec093866ad5b2223d967977fd9590b742fb79b0c0960020" {
		t.Fatalf("formal source exclusion receipt SHA-256 = %s", exclusionSHA)
	}
	receipt := harness.AgentBuildReceipt{
		SchemaVersion: "agentic-bench/agent-build-receipt-v2", AgentID: "luban",
		BaseCommit: formalHex("2", 40), TreeOID: formalHex("3", 40),
		PatchSHA256: patchSHA, ArchiveSHA256: archiveSHA, BinarySHA256: binarySHA,
		PathPolicy: pathPolicy, PathPolicySHA256: pathPolicySHA, ExclusionReceiptSHA256: exclusionSHA,
		BuildArgv: []string{"go", "build", "./cmd/luban"}, Toolchain: "go-fixture", BuiltAt: builtAt,
	}
	externalReceiptPath := filepath.Join(temporaryRoot, "build", "luban-build-receipt.json")
	writeJSONForFormalFixture(t, externalReceiptPath, receipt)
	receiptRaw, err := os.ReadFile(externalReceiptPath)
	if err != nil {
		t.Fatalf("read source build receipt: %v", err)
	}
	writeFileForFormalFixture(t, filepath.Join(sourceDir, "build-receipt.json"), receiptRaw)
	receiptSHA, err := harness.HashFile(externalReceiptPath)
	if err != nil {
		t.Fatalf("hash source build receipt: %v", err)
	}
	return &harness.AgentSourceSpec{
		Worktree: filepath.Join(temporaryRoot, "source", "luban"), BaseCommit: receipt.BaseCommit,
		TreeOID: receipt.TreeOID, PatchSHA256: patchSHA, ArchiveSHA256: archiveSHA,
		PathPolicy: pathPolicy, PathPolicySHA256: pathPolicySHA, ExclusionReceiptSHA256: exclusionSHA,
		BuildReceipt: externalReceiptPath, BuildReceiptSHA256: receiptSHA,
	}
}

func buildFormalRunFixture(
	t *testing.T,
	artifactRoot string,
	task harness.Task,
	agent harness.AgentSpec,
	entry harness.PlanEntry,
	pricing harness.PricingCatalog,
	benchmarkTime time.Time,
) (harness.RunRecord, string) {
	t.Helper()
	artifactDir := filepath.Join(artifactRoot, "runs", entry.PairID, entry.AgentID, "attempt-001")
	patchPath := filepath.Join(artifactDir, "submission.patch")
	patchContents := []byte("diff --git a/file b/file\n" + entry.AgentID + "\n")
	writeFileForFormalFixture(t, patchPath, patchContents)
	auditPatchPath := filepath.Join(artifactDir, "audit-workspace.patch")
	writeFileForFormalFixture(t, auditPatchPath, patchContents)
	patchSHA, err := harness.HashFile(patchPath)
	if err != nil {
		t.Fatalf("hash %s submission patch: %v", entry.AgentID, err)
	}
	auditSHA, err := harness.HashFile(auditPatchPath)
	if err != nil {
		t.Fatalf("hash %s audit workspace patch: %v", entry.AgentID, err)
	}
	capture := harness.SubmissionCaptureEvidence{
		Method: "official-git-diff+temporary-index-audit-v2", BaseCommit: task.BaseCommit,
		PatchSHA256: patchSHA, AuditPatchSHA256: auditSHA,
		UncommittedChangesPresent: patchSHA != auditSHA,
		IncludesTracked:           true, IncludesUntracked: true, IncludesBinary: true,
	}
	writeJSONForFormalFixture(t, filepath.Join(artifactDir, "pier", "agent-workspace-capture.json"), struct {
		SchemaVersion string `json:"schema_version"`
		harness.SubmissionCaptureEvidence
	}{SchemaVersion: "agentic-bench/workspace-capture-v2", SubmissionCaptureEvidence: capture})
	verifierPath := filepath.Join(artifactDir, "verifier", "reward.json")
	writeFileForFormalFixture(t, verifierPath, []byte("{\"reward\":1}\n"))

	executionStarted := benchmarkTime.Add(time.Duration(entry.Ordinal+1) * time.Minute)
	roundStarted := executionStarted.Add(250 * time.Millisecond)
	durationMS, toolError, outputBytes := int64(30), false, int64(96)
	inputTokens := 1000 + int64(entry.Ordinal)*100
	cachedInputTokens := 800 + int64(entry.Ordinal)*80
	outputTokens := 120 + int64(entry.Ordinal)*10
	reasoningOutputTokens := 20 + int64(entry.Ordinal)
	cacheWriteInputTokens := int64(0)
	physicalToolOperations := 1
	toolQueueMS := int64(5)
	toolCallIDHash, toolTraceKind := writeFormalAgentStreamFixture(
		t, artifactDir, agent.ID, durationMS, outputBytes, physicalToolOperations, toolQueueMS,
	)
	toolDefinitions := formalToolDefinitionsForFixture(agent.ID)
	toolCatalogCanonicalBytes := int64(0)
	for _, definition := range toolDefinitions {
		toolCatalogCanonicalBytes += definition.DefinitionBytes
	}
	toolCallKind, toolCallName := "function_call", "Run"
	if agent.ID == "codex" {
		toolCallKind, toolCallName = "custom_tool_call", "exec"
	}
	promptCacheKeyHash := formalHex(string(rune('6'+entry.Ordinal)), 64)
	cacheTTLSeconds := int64(1800)
	runIdentity := formalHex(string(rune('e'+entry.Ordinal)), 64)
	requestServiceTierPresent := true
	requestServiceTierRaw := harness.FormalServiceTier
	requestServiceTierRepresentation := harness.ServiceTierEncodingExplicitDefault
	clientCanonicalizationProofSHA256 := ""
	if agent.ID == "codex" {
		requestServiceTierPresent = false
		requestServiceTierRaw = ""
		requestServiceTierRepresentation = harness.ServiceTierEncodingClientCanonical
		clientCanonicalizationProofSHA256 = formalServiceTierCanonicalizationStaticProof(t, agent)
	}
	round := harness.ProviderRoundEvidence{
		SchemaVersion: "agentic-bench/provider-round-v2", EvidenceSequence: 0,
		Round: 0, RunIdentity: runIdentity, ProviderAttemptStarted: true,
		Transport: "http_sse", ProviderAttemptKind: "inference", WebSocketChainBound: true,
		TransportDisposition: "valid", Outcome: "success",
		RequestID: formalHex(string(rune('a'+entry.Ordinal)), 64), ResponseIDHash: formalHex(string(rune('c'+entry.Ordinal)), 64),
		StartedAt: roundStarted, UpstreamHeadersAt: roundStarted.Add(100 * time.Millisecond),
		FirstResponseByteAt: roundStarted.Add(200 * time.Millisecond), FinishedAt: roundStarted.Add(1200 * time.Millisecond),
		Provider: agent.Model.Provider, Model: agent.Model.Model, ReasoningEffort: agent.Model.ReasoningEffort,
		RequestedReasoningContext: "all_turns", RequestedReasoningModeCanonical: "standard",
		PromptCacheKeyPresent: true, PromptCacheKeyHash: promptCacheKeyHash, CachePolicyObserved: true,
		ContinuationLineagePresent: true, ContinuationLineageSource: "controller_default",
		ContinuationLineageHash: formalHex(string(rune('8'+entry.Ordinal)), 64), ContinuationEpoch: 1,
		RequestedServiceTierRaw: requestServiceTierRaw, RequestedServiceTierPresent: requestServiceTierPresent,
		RequestedServiceTierCanonical: harness.FormalServiceTier, RequestedServiceTierRepresentation: requestServiceTierRepresentation,
		ClientCanonicalizationProofSHA256: clientCanonicalizationProofSHA256, ClientAgentID: agent.ID,
		ResponseServiceTierRaw: "default", ResponseServiceTierCanonical: "default", ServiceTierComparable: true,
		ToolDefinitionCount: len(toolDefinitions), ToolDefinitions: toolDefinitions,
		ToolCatalogCompared: true, ToolCatalogStable: true, ToolCatalogHash: formalHex("5", 64),
		ToolCatalogSemanticSHA256: harness.StableToolCatalogSHA256(toolDefinitions), ToolCatalogCanonicalBytes: toolCatalogCanonicalBytes,
		ToolResultHistoryValid: true,
		ResponseCreatedModel:   agent.Model.Model, ResponseModel: agent.Model.Model,
		ResponseCompleted: true, ResponseStatus: "completed",
		StoreSpecified: true, Store: false, EncryptedReasoningRequested: true,
		HTTPStatus: 200, RequestBytes: 1024, ResponseBytes: 512,
		UsagePresent: true, InputTokens: &inputTokens, CachedInputTokens: &cachedInputTokens, CacheWriteInputTokens: &cacheWriteInputTokens,
		OutputTokens: &outputTokens, ReasoningOutputTokens: &reasoningOutputTokens,
		ToolCalls: []harness.ToolCallEvidence{{
			ID: toolCallIDHash, Kind: toolCallKind, Name: toolCallName, DurationMS: &durationMS, Error: &toolError,
			InputBytes: 32, OutputBytes: &outputBytes, AgentTraceOutputBytes: &outputBytes, TraceMatch: "id", TraceKind: toolTraceKind,
		}},
		ToolResultPayloadBytes: 0, PhysicalToolOperations: &physicalToolOperations,
		ToolCriticalPathMS: &durationMS, ToolTotalLatencyMS: &durationMS, ToolQueueMS: &toolQueueMS,
	}
	if agent.ID == "luban" {
		round.PromptCacheOptionsPresent = true
		round.PromptCacheOptionsMode = "implicit"
		round.PromptCacheTTLSeconds = &cacheTTLSeconds
		round.CacheBreakpointCount = 1
		round.CacheBreakpointPositionHashes = []string{formalHex("7", 64)}
	}
	originalBodySHA, forwardedBodySHA := formalHex("1", 64), formalHex("1", 64)
	originalCanonicalSHA, forwardedCanonicalSHA := formalHex("2", 64), formalHex("2", 64)
	originalTierPresent, originalTier := true, harness.FormalServiceTier
	transformation, forwardedRequestBytes := "none", int64(1024)
	if agent.ID == "codex" {
		forwardedBodySHA, forwardedCanonicalSHA = formalHex("3", 64), formalHex("4", 64)
		originalTierPresent, originalTier = false, ""
		transformation, forwardedRequestBytes = "inject_explicit_default", 1050
	}
	round.OriginalRequestBodySHA256 = originalBodySHA
	round.ForwardedRequestBodySHA256 = forwardedBodySHA
	round.OriginalRequestCanonicalSHA256 = originalCanonicalSHA
	round.ForwardedRequestCanonicalSHA256 = forwardedCanonicalSHA
	round.OriginalRequestWithoutServiceTierSHA256 = formalHex("5", 64)
	round.ForwardedRequestWithoutServiceTierSHA256 = formalHex("5", 64)
	round.OriginalServiceTierPresent = originalTierPresent
	round.OriginalServiceTier = originalTier
	round.ForwardedServiceTierPresent = true
	round.ForwardedServiceTier = harness.FormalServiceTier
	round.ForwardedRequestBytes = forwardedRequestBytes
	round.ServiceTierTransformation = transformation
	round.ServiceTierTransformationExactDiff = true
	round.ServiceTierTransformationProofSHA256 = formalHex("6", 64)
	rawRecord := evidenceproxy.Record{
		SchemaVersion: "agentic-bench/provider-http-v6", EvidenceSequence: 0,
		Round: 0, RunIdentity: runIdentity, ProviderAttemptStarted: true,
		Transport: "http_sse", ProviderAttemptKind: "inference",
		ApprovedOrigin: harness.FormalProviderOrigin, SemanticsSHA256: harness.FormalProviderEndpointSemanticsSHA256,
		TLSServerName: harness.FormalProviderTLSServerName, TLSVerified: true, TLSObservedAt: roundStarted,
		TLSPeerLeafCertSHA256: formalHex("b", 64), TLSPeerSPKISHA256: formalHex("c", 64), WebSocketChainBound: true,
		GenerateSpecified: false, Generate: false,
		StartedAt: roundStarted, UpstreamHeadersAt: roundStarted.Add(100 * time.Millisecond),
		FirstResponseByteAt: roundStarted.Add(200 * time.Millisecond), FinishedAt: roundStarted.Add(1200 * time.Millisecond),
		Method: "POST", Path: "/v1/responses", RequestBytes: 1024, ForwardedRequestBytes: forwardedRequestBytes, ResponseBytes: 512,
		RequestedModel: agent.Model.Model, RequestedReasoningEffort: agent.Model.ReasoningEffort,
		RequestedReasoningContext: "all_turns", RequestedReasoningModeCanonical: "standard",
		PromptCacheKeyPresent: true, PromptCacheKeyHash: promptCacheKeyHash, CachePolicyObserved: true,
		RequestedServiceTier: requestServiceTierRaw, RequestedServiceTierPresent: requestServiceTierPresent,
		RequestedServiceTierCanonical: harness.FormalServiceTier, RequestedServiceTierRepresentation: requestServiceTierRepresentation,
		ClientCanonicalizationStaticProofSHA256: clientCanonicalizationProofSHA256, ClientAgentID: agent.ID,
		OriginalRequestBodySHA256: originalBodySHA, ForwardedRequestBodySHA256: forwardedBodySHA,
		OriginalRequestCanonicalSHA256: originalCanonicalSHA, ForwardedRequestCanonicalSHA256: forwardedCanonicalSHA,
		OriginalRequestWithoutServiceTierSHA256: formalHex("5", 64), ForwardedRequestWithoutServiceTierSHA256: formalHex("5", 64),
		OriginalServiceTierPresent: originalTierPresent, OriginalServiceTier: originalTier,
		ForwardedServiceTierPresent: true, ForwardedServiceTier: harness.FormalServiceTier,
		ServiceTierTransformation: transformation, ServiceTierTransformationExactDiff: true,
		ServiceTierTransformationProofSHA256: formalHex("6", 64),
		StoreSpecified:                       true, Store: false, EncryptedReasoningRequested: true,
		ContinuationLineagePresent: true, ContinuationLineageHash: round.ContinuationLineageHash,
		ContinuationLineageSource: "controller_default", ContinuationEpoch: 1,
		ToolDefinitionCount: len(toolDefinitions), ToolDefinitions: formalRawToolDefinitionsForFixture(toolDefinitions),
		ToolCatalogHash: round.ToolCatalogHash, ToolCatalogSemanticSHA256: round.ToolCatalogSemanticSHA256,
		ToolCatalogCanonicalBytes: toolCatalogCanonicalBytes,
		ToolCatalogCompared:       true, ToolCatalogStable: true, ToolResultHistoryValid: true,
		HTTPStatus: 200, UpstreamRequestIDHash: round.RequestID, ResponseIDHash: round.ResponseIDHash,
		ResponseCreatedModel: agent.Model.Model, ResponseModel: agent.Model.Model,
		ResponseServiceTier: harness.FormalServiceTier, ResponseServiceTierCanonical: harness.FormalServiceTier, ServiceTierComparable: true,
		ResponseCompleted: true, ResponseStatus: "completed",
		UsagePresent: true, InputTokens: &inputTokens, CachedInputTokens: &cachedInputTokens,
		CacheWriteInputTokens: &cacheWriteInputTokens, OutputTokens: &outputTokens, ReasoningOutputTokens: &reasoningOutputTokens,
		ToolCalls:     []evidenceproxy.ToolCall{{IDHash: toolCallIDHash, Kind: toolCallKind, Name: toolCallName, InputBytes: 32}},
		ProtocolValid: true, Disposition: "valid",
	}
	if agent.ID == "luban" {
		rawRecord.PromptCacheOptionsPresent = true
		rawRecord.PromptCacheOptionsMode = "implicit"
		rawRecord.PromptCacheTTLSeconds = &cacheTTLSeconds
		rawRecord.CacheBreakpointCount = 1
		rawRecord.CacheBreakpointPositionHashes = []string{formalHex("7", 64)}
	}
	transformationProofSHA, err := evidenceproxy.ServiceTierTransformationProofSHA256(rawRecord)
	if err != nil {
		t.Fatalf("hash %s service-tier transformation proof: %v", entry.AgentID, err)
	}
	rawRecord.ServiceTierTransformationProofSHA256 = transformationProofSHA
	round.ServiceTierTransformationProofSHA256 = transformationProofSHA
	if err := evidenceproxy.ValidateServiceTierTransformationProof(rawRecord); err != nil {
		t.Fatalf("self-validate %s service-tier transformation proof: %v", entry.AgentID, err)
	}
	rawEvidenceHash, err := harness.HashCanonical(rawRecord)
	if err != nil {
		t.Fatalf("hash %s raw provider evidence: %v", entry.AgentID, err)
	}
	rawRecord.EvidenceHash = rawEvidenceHash
	round.EvidenceHash = rawEvidenceHash
	evidencePath := filepath.Join(artifactDir, agent.RequestEvidence.RelativePath)
	rawEvidencePath := filepath.Join(artifactDir, "metrics", "provider-http.raw.jsonl")
	attemptJournalPath := rawEvidencePath + ".attempt-starts.jsonl"
	sealPath := rawEvidencePath + ".seal.json"
	encodedRawRecord, err := json.Marshal(rawRecord)
	if err != nil {
		t.Fatalf("marshal %s raw provider evidence: %v", entry.AgentID, err)
	}
	writeFileForFormalFixture(t, rawEvidencePath, append(encodedRawRecord, '\n'))
	journal := evidenceproxy.AttemptStartJournalEntry{
		SchemaVersion: "agentic-bench/provider-attempt-start-v1", RunIdentity: runIdentity, Round: 0,
		StartedAt: roundStarted, Transport: "http_sse", ProviderAttemptKind: "inference",
	}
	encodedJournal, err := json.Marshal(journal)
	if err != nil {
		t.Fatalf("marshal %s provider attempt journal: %v", entry.AgentID, err)
	}
	writeFileForFormalFixture(t, attemptJournalPath, append(encodedJournal, '\n'))
	writeJSONForFormalFixture(t, sealPath, evidenceproxy.EvidenceSeal{
		SchemaVersion: "agentic-bench/provider-evidence-seal-v1", RunIdentity: runIdentity,
		StartedAttemptCount: 1, PersistedAttemptCount: 1, RecordCount: 1, LastEvidenceHash: rawEvidenceHash,
		SealedAt: round.FinishedAt.Add(time.Millisecond),
	})
	rawEvidenceSHA, err := harness.HashFile(rawEvidencePath)
	if err != nil {
		t.Fatalf("hash %s raw provider evidence: %v", entry.AgentID, err)
	}
	serviceTierCanonicalization := writeFormalServiceTierCanonicalizationEvidence(
		t, artifactDir, agent, rawEvidenceSHA, &round, clientCanonicalizationProofSHA256,
	)
	encodedRound, err := json.Marshal(round)
	if err != nil {
		t.Fatalf("marshal %s provider evidence: %v", entry.AgentID, err)
	}
	writeFileForFormalFixture(t, evidencePath, append(encodedRound, '\n'))
	if err := harness.ValidateServiceTierCanonicalizationArchive(artifactDir, agent.ID, harness.AgentExecution{
		EvidenceRunIdentity: runIdentity,
		ProviderEvidence: harness.ProviderEvidenceSeal{
			RawEvidenceSHA256: rawEvidenceSHA,
		},
		ServiceTierCanonicalization: serviceTierCanonicalization,
	}, []harness.ProviderRoundEvidence{round}); err != nil {
		t.Fatalf("self-validate %s service-tier canonicalization evidence: %v", entry.AgentID, err)
	}
	attemptJournalSHA, err := harness.HashFile(attemptJournalPath)
	if err != nil {
		t.Fatalf("hash %s provider attempt journal: %v", entry.AgentID, err)
	}
	sealSHA, err := harness.HashFile(sealPath)
	if err != nil {
		t.Fatalf("hash %s provider evidence seal: %v", entry.AgentID, err)
	}
	metrics, err := harness.ValidateAndAggregateEvidence([]harness.ProviderRoundEvidence{round}, agent.Model, pricing)
	if err != nil {
		t.Fatalf("aggregate %s provider evidence: %v", entry.AgentID, err)
	}
	executionFinished := executionStarted.Add(3 * time.Second)
	trialStarted := executionStarted.Add(-time.Second)
	trialFinished := executionFinished.Add(time.Second)
	controllerStarted := trialStarted.Add(-time.Second)
	controllerFinished := trialFinished.Add(time.Second)
	storageAdmission := formalStorageAdmissionForFixture(
		harness.StorageStageRawSlotAdmission,
		controllerStarted.Add(-time.Second),
		40*formalFixtureGiB,
	)
	hostStorageEvidence, guestStorageEvidence := writeFormalRunStorageEvidence(
		t,
		artifactDir,
		storageAdmission,
		controllerStarted,
		controllerFinished,
		roundStarted,
		trialStarted,
		executionFinished,
		executionFinished,
		trialFinished,
		runIdentity,
	)
	verification := harness.VerificationResult{ProtocolValid: true, Reward: 1, ArtifactPaths: []string{verifierPath}}
	record := harness.RunRecord{
		Entry: entry, Phase: harness.RunComplete, Attempts: 1,
		SlotReservedAt: storageAdmission.ObservedAt.Add(time.Millisecond), StorageAdmission: storageAdmission, AttemptStartedAt: trialStarted,
		Disposition: harness.DeepSWEAttemptScored, FailureCategory: harness.DeepSWEFailureNone, ArtifactDir: artifactDir,
		Execution: &harness.AgentExecution{
			Lifecycle: harness.AttemptLifecycle{
				SchemaVersion: "agentic-bench/attempt-lifecycle-v1", RunIdentity: runIdentity,
				ControllerStartedAt: controllerStarted, ControllerFinishedAt: controllerFinished,
				ProviderAttemptState: "provider_attempt_sealed", ProviderAttemptCount: 1,
			},
			ExitClass: "completed", ExitCode: 0, StartedAt: executionStarted, FinishedAt: executionFinished,
			TrialStartedAt: trialStarted, TrialFinishedAt: trialFinished,
			SubmissionPatch: patchPath, AuditWorkspacePatch: auditPatchPath, EvidencePath: evidencePath,
			EvidenceRunIdentity: runIdentity,
			ProviderEvidence: harness.ProviderEvidenceSeal{
				RawEvidencePath: rawEvidencePath, AttemptJournalPath: attemptJournalPath, SealPath: sealPath,
				RawEvidenceSHA256: rawEvidenceSHA, AttemptJournalSHA256: attemptJournalSHA, SealSHA256: sealSHA,
				StartedAttemptCount: 1, PersistedAttemptCount: 1, RecordCount: 1, LastEvidenceHash: round.EvidenceHash,
			},
			ServiceTierCanonicalization: serviceTierCanonicalization,
			StorageEvidence:             hostStorageEvidence,
			GuestStorageEvidence:        guestStorageEvidence,
			TerminalEvidence:            harness.AgentTerminalEvidence{SchemaVersion: "agentic-bench/terminal-evidence-v1", Source: "process_exit", Code: "completed", EvidenceSHA256: formalHex("6", 64)},
			Capture:                     capture,
		},
		Verification: &verification,
		Metrics:      &metrics,
	}
	return record, verifierPath
}

func formalAgentByID(t *testing.T, agents []harness.AgentSpec, id string) harness.AgentSpec {
	t.Helper()
	for _, agent := range agents {
		if agent.ID == id {
			return agent
		}
	}
	t.Fatalf("agent %s is absent", id)
	return harness.AgentSpec{}
}

func writeJSONForFormalFixture(t *testing.T, path string, value any) {
	t.Helper()
	if err := harness.WriteJSONAtomic(path, value, 0o600); err != nil {
		t.Fatalf("write JSON %s: %v", path, err)
	}
}

func readJSONForFormalFixture(t *testing.T, path string, target any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read JSON %s: %v", path, err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		t.Fatalf("decode JSON %s: %v", path, err)
	}
}

func writeFileForFormalFixture(t testing.TB, path string, contents []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create directory for %s: %v", path, err)
	}
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func writeTarForFormalFixture(t testing.TB, path string, files map[string][]byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create directory for %s: %v", path, err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create tar %s: %v", path, err)
	}
	writer := tar.NewWriter(file)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		contents := files[name]
		header := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(contents)), ModTime: time.Unix(0, 0).UTC()}
		if err := writer.WriteHeader(header); err != nil {
			t.Fatalf("write tar header %s: %v", name, err)
		}
		if _, err := writer.Write(contents); err != nil {
			t.Fatalf("write tar contents %s: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close tar writer %s: %v", path, err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close tar file %s: %v", path, err)
	}
}

func writeCaptureReceiptForFormalFixture(t testing.TB, artifactDir string, capture harness.SubmissionCaptureEvidence) {
	t.Helper()
	writeJSONForFormalFixtureTB(t, filepath.Join(artifactDir, "pier", "agent-workspace-capture.json"), struct {
		SchemaVersion string `json:"schema_version"`
		harness.SubmissionCaptureEvidence
	}{SchemaVersion: "agentic-bench/workspace-capture-v2", SubmissionCaptureEvidence: capture})
}

func writeJSONForFormalFixtureTB(t testing.TB, path string, value any) {
	t.Helper()
	if err := harness.WriteJSONAtomic(path, value, 0o600); err != nil {
		t.Fatalf("write JSON %s: %v", path, err)
	}
}

func formalHex(character string, length int) string { return strings.Repeat(character, length) }
