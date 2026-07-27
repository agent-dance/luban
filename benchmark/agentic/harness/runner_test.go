package harness

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fixtureBackend struct {
	manifest          Manifest
	oracleResult      VerificationResult
	oracleError       error
	verifierFailures  int
	infraFailures     int
	infraCategory     DeepSWEFailureCategory
	agentRuns         int
	agentRunsByID     map[string]int
	verifierRuns      int
	recoverRuns       int
	agentExitClass    string
	snapshotMutator   func(*BackendSnapshot)
	protocolFailures  int
	agentCrash        bool
	recoverController bool
	inventoryLock     InventoryLockSnapshot
	storagePreflight  StorageAdmissionReceipt
}

func (backend *fixtureBackend) CheckHostStoragePreflight(_ context.Context, request StorageAdmissionRequest) (StorageAdmissionReceipt, error) {
	receipt := fixtureStorageAdmission(request.Resources, StorageStageExperimentPreflight)
	backend.storagePreflight = receipt
	return receipt, nil
}

func (backend *fixtureBackend) CheckStorageAdmission(_ context.Context, request StorageAdmissionRequest) (StorageAdmissionReceipt, error) {
	return fixtureStorageAdmission(request.Resources, StorageStageRawSlotAdmission), nil
}

func fixtureStorageAdmission(resources ResourceSpec, stage string) StorageAdmissionReceipt {
	now := time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC)
	return StorageAdmissionReceipt{
		SchemaVersion: StorageAdmissionSchemaVersion, Stage: stage, Enforcement: FormalStorageEnforcement,
		DeclaredStorageMB: resources.StorageMB, Guard: resources.HostStorageGuard, Authority: StorageStatfsAuthority,
		ObservedAt: now.Add(time.Millisecond), Passed: true,
		Filesystems: []StorageAdmissionFilesystemReceipt{{
			Group: 0, Roles: []string{"artifact_root", "controller_root", "private_work_root"},
			VolumeIdentitySHA256: strings.Repeat("d", 64), FilesystemType: "apfs", DeviceRoleCount: 3,
			ObservedAt: now, MonotonicOffsetMS: 1, BlockSizeBytes: 4096,
			TotalBytes: 200 * 1024 * 1024 * 1024, AvailableBytes: 150 * 1024 * 1024 * 1024, UsedBytes: 40 * 1024 * 1024 * 1024,
		}},
	}
}

func fixtureStorageResourceEvidence(invocation AgentInvocation, now time.Time) (StorageResourceEvidence, []GuestStorageResourceEvidence, error) {
	hostStartedAt := now.Add(-2 * time.Second)
	hostFinishedAt := now.Add(3 * time.Second)
	hostAvailable := uint64(150 * 1024 * 1024 * 1024)
	hostUsed := uint64(40 * 1024 * 1024 * 1024)
	hostSamples := []StorageStatfsSample{
		fixtureStorageSample(hostStartedAt, 0, 0, hostAvailable, hostUsed),
		fixtureStorageSample(hostStartedAt, 0, 2_000, hostAvailable, hostUsed),
		fixtureStorageSample(hostStartedAt, 2_000, 4_000, hostAvailable, hostUsed),
		fixtureStorageSample(hostStartedAt, 4_000, 5_000, hostAvailable, hostUsed),
	}
	hostReceipt := StorageResourceReceipt{
		SchemaVersion: StorageReceiptSchemaVersion, Enforcement: FormalStorageEnforcement,
		DeclaredStorageMB: invocation.Resources.StorageMB, Guard: invocation.Resources.HostStorageGuard,
		Authority: StorageStatfsAuthority, Admission: invocation.StorageAdmission,
		StartedAt: hostStartedAt, FinishedAt: hostFinishedAt, ProviderWALStartedAt: now,
		ProviderWALStartedDeltaMS: 2_000, FinishedDeltaMS: 5_000,
		Filesystems: []StorageRuntimeFilesystemReceipt{{
			Group: 0, Roles: []string{"artifact_root", "controller_root", "private_work_root"},
			VolumeIdentitySHA256: strings.Repeat("d", 64), FilesystemType: "apfs", DeviceRoleCount: 3,
			BlockSizeBytes: 4096, TotalBytes: 200 * 1024 * 1024 * 1024,
			AvailableBeforeBytes: hostAvailable, AvailableAfterBytes: hostAvailable, MinimumAvailableBytes: hostAvailable,
			UsedBeforeBytes: hostUsed, UsedAfterBytes: hostUsed, MaximumUsedBytes: hostUsed,
			Samples: uint64(len(hostSamples)), MaximumCompletionGapMS: 2_000, SamplePoints: hostSamples,
		}},
		Status: StorageStatusCompletedAboveGuard,
	}
	hostEvidence, err := fixtureWriteHostStorageReceipt(invocation.ArtifactDir, hostReceipt)
	if err != nil {
		return StorageResourceEvidence{}, nil, err
	}

	sessionIdentity := strings.Repeat("1", 64)
	providerWALStartedAt := now
	agentReceipt := fixtureGuestStorageReceipt(
		invocation.Resources, GuestStoragePhaseAgent, sessionIdentity, strings.Repeat("2", 64),
		now.Add(-time.Second), now.Add(time.Second), providerWALStartedAt,
	)
	verifierReceipt := fixtureGuestStorageReceipt(
		invocation.Resources, GuestStoragePhaseVerifier, sessionIdentity, strings.Repeat("3", 64),
		now.Add(time.Second), now.Add(2*time.Second), providerWALStartedAt,
	)
	guestEvidence := make([]GuestStorageResourceEvidence, 0, 2)
	for _, value := range []struct {
		path    string
		receipt GuestStorageResourceReceipt
	}{
		{path: GuestStorageAgentReceiptRelativePath, receipt: agentReceipt},
		{path: GuestStorageVerifierReceiptRelativePath, receipt: verifierReceipt},
	} {
		evidence, err := fixtureWriteGuestStorageReceipt(invocation.ArtifactDir, value.path, value.receipt)
		if err != nil {
			return StorageResourceEvidence{}, nil, err
		}
		guestEvidence = append(guestEvidence, evidence)
	}
	return hostEvidence, guestEvidence, nil
}

func fixtureStorageSample(startedAt time.Time, startDeltaMS, endDeltaMS int64, availableBytes, usedBytes uint64) StorageStatfsSample {
	return StorageStatfsSample{
		ObservedAt:   startedAt.Add(time.Duration(endDeltaMS) * time.Millisecond),
		StartDeltaMS: startDeltaMS, EndDeltaMS: endDeltaMS,
		AvailableBytes: availableBytes, UsedBytes: usedBytes,
	}
}

func fixtureWriteHostStorageReceipt(artifactDir string, receipt StorageResourceReceipt) (StorageResourceEvidence, error) {
	path := filepath.Join(artifactDir, filepath.FromSlash(StorageReceiptRelativePath))
	if err := WriteJSONAtomic(path, receipt, 0o600); err != nil {
		return StorageResourceEvidence{}, err
	}
	digest, err := HashFile(path)
	if err != nil {
		return StorageResourceEvidence{}, err
	}
	return StorageResourceEvidence{
		SchemaVersion: StorageEvidenceSchemaVersion, ReceiptRelativePath: StorageReceiptRelativePath,
		ReceiptSHA256: digest, Receipt: receipt,
	}, nil
}

func fixtureGuestStorageReceipt(resources ResourceSpec, phase, sessionIdentity, containerIdentity string, startedAt, finishedAt, providerWALStartedAt time.Time) GuestStorageResourceReceipt {
	guestAvailable := uint64(32 * 1024 * 1024 * 1024)
	guestUsed := uint64(20 * 1024 * 1024 * 1024)
	finishedDeltaMS := finishedAt.Sub(startedAt).Milliseconds()
	middleDeltaMS := finishedDeltaMS / 2
	samples := []StorageStatfsSample{
		fixtureStorageSample(startedAt, 0, 0, guestAvailable, guestUsed),
		fixtureStorageSample(startedAt, 0, middleDeltaMS, guestAvailable, guestUsed),
		fixtureStorageSample(startedAt, middleDeltaMS, finishedDeltaMS, guestAvailable, guestUsed),
	}
	return GuestStorageResourceReceipt{
		SchemaVersion: GuestStorageReceiptSchemaVersion, Phase: phase,
		SessionIdentitySHA256: sessionIdentity, ContainerIdentitySHA256: containerIdentity,
		ConfiguredCapacityBytes: 64 * 1024 * 1024 * 1024,
		Enforcement:             FormalStorageEnforcement, DeclaredStorageMB: resources.StorageMB,
		Guard: resources.GuestStorageGuard, Authority: GuestStorageStatfsAuthority,
		StartedAt: startedAt, FinishedAt: finishedAt, ProviderWALStartedAt: providerWALStartedAt,
		ProviderWALStartedDeltaMS: providerWALStartedAt.Sub(startedAt).Milliseconds(),
		FinishedDeltaMS:           finishedDeltaMS,
		Filesystems: []GuestStorageFilesystemReceipt{{
			Group: 0, Roles: []string{"guest_app", "guest_root"}, VolumeIdentitySHA256: strings.Repeat("4", 64),
			FilesystemType: "ext4", DeviceRoleCount: 2, BlockSizeBytes: 4096, TotalBytes: 64 * 1024 * 1024 * 1024,
			MinimumAvailableBytes: guestAvailable, MaximumUsedBytes: guestUsed,
			MaximumCompletionGapMS: middleDeltaMS, Samples: samples,
		}},
		Status: StorageStatusCompletedAboveGuard,
	}
}

func fixtureWriteGuestStorageReceipt(artifactDir, relativePath string, receipt GuestStorageResourceReceipt) (GuestStorageResourceEvidence, error) {
	path := filepath.Join(artifactDir, filepath.FromSlash(relativePath))
	if err := WriteJSONAtomic(path, receipt, 0o600); err != nil {
		return GuestStorageResourceEvidence{}, err
	}
	digest, err := HashFile(path)
	if err != nil {
		return GuestStorageResourceEvidence{}, err
	}
	return GuestStorageResourceEvidence{
		SchemaVersion: GuestStorageEvidenceSchemaVersion, ReceiptRelativePath: relativePath,
		ReceiptSHA256: digest, Receipt: receipt,
	}, nil
}

func fixtureServiceTierCanonicalization(invocation AgentInvocation, rawEvidenceSHA string, round *ProviderRoundEvidence) (ServiceTierCanonicalizationEvidence, error) {
	if invocation.Agent.ID != "codex" {
		return ServiceTierCanonicalizationEvidence{}, nil
	}
	type transformationProjection struct {
		Round            int    `json:"round"`
		OriginalBodySHA  string `json:"original_body_sha256"`
		ForwardedBodySHA string `json:"forwarded_body_sha256"`
		ProofSHA         string `json:"proof_sha256"`
	}
	transformationSHA, err := HashCanonical([]transformationProjection{{
		Round: round.Round, OriginalBodySHA: round.OriginalRequestBodySHA256,
		ForwardedBodySHA: round.ForwardedRequestBodySHA256, ProofSHA: round.ServiceTierTransformationProofSHA256,
	}})
	if err != nil {
		return ServiceTierCanonicalizationEvidence{}, err
	}
	payload := archivedServiceTierCanonicalizationPayload{
		SchemaVersion:  "agentic-bench/service-tier-canonicalization-binding-v2",
		Representation: ServiceTierEncodingClientCanonical, ClientAgentID: "codex",
		ClientRuntimeVersion: "0.145.0", RunIdentity: round.RunIdentity,
		RegisteredBinarySHA256:     invocation.Agent.BinarySHA256,
		FrozenBundleManifestSHA256: strings.Repeat("1", 64), FrozenBundleTreeSHA256: strings.Repeat("2", 64),
		AdapterSHA256: strings.Repeat("3", 64), AdapterVersion: "2.4.0",
		SourceCommandArgvSHA256: strings.Repeat("4", 64), EffectiveArgvSHA256: strings.Repeat("5", 64),
		EffectiveArgvReceiptSHA256: strings.Repeat("6", 64), SandboxCanaryReceiptSHA256: strings.Repeat("7", 64),
		CanonicalCanaryGeneration:          FormalExecutionCanaryGeneration,
		FrozenCanonicalCanaryReceiptSHA256: invocation.Agent.ExecutionCanary.ReceiptSHA256,
		RawProviderEvidenceSHA256:          rawEvidenceSHA, TransformationEvidenceSHA256: transformationSHA,
		TransformedProviderRoundCount: 1, StaticProofSHA256: strings.Repeat("a", 64),
	}
	bindingSHA, err := HashCanonical(payload)
	if err != nil {
		return ServiceTierCanonicalizationEvidence{}, err
	}
	round.ClientCanonicalizationProofSHA256 = bindingSHA
	receipt := archivedServiceTierCanonicalizationReceipt{
		archivedServiceTierCanonicalizationPayload: payload,
		BindingSHA256: bindingSHA,
	}
	relativePath := "pier/service-tier-canonicalization-receipt.json"
	path := filepath.Join(invocation.ArtifactDir, filepath.FromSlash(relativePath))
	if err := WriteJSONAtomic(path, receipt, 0o600); err != nil {
		return ServiceTierCanonicalizationEvidence{}, err
	}
	receiptSHA, err := HashFile(path)
	if err != nil {
		return ServiceTierCanonicalizationEvidence{}, err
	}
	var archived archivedServiceTierCanonicalizationReceipt
	if err := decodeStrictReceipt(path, receiptSHA, &archived); err != nil {
		return ServiceTierCanonicalizationEvidence{}, err
	}
	recomputedBinding, err := HashCanonical(archived.archivedServiceTierCanonicalizationPayload)
	if err != nil {
		return ServiceTierCanonicalizationEvidence{}, err
	}
	if archived.BindingSHA256 != bindingSHA || recomputedBinding != bindingSHA {
		return ServiceTierCanonicalizationEvidence{}, errors.New("fixture service-tier receipt binding changed during archival")
	}
	return ServiceTierCanonicalizationEvidence{
		SchemaVersion:  ServiceTierCanonicalizationEvidenceSchemaVersion,
		Representation: ServiceTierEncodingClientCanonical, ReceiptRelativePath: relativePath,
		ReceiptSHA256: receiptSHA, BindingSHA256: bindingSHA, StaticProofSHA256: payload.StaticProofSHA256,
		TransformationEvidenceSHA256: transformationSHA, TransformedRoundCount: 1,
	}, nil
}

func (backend *fixtureBackend) BindInventoryLockArchive(_ context.Context, archivePath string) error {
	if _, err := os.Stat(archivePath); errors.Is(err, os.ErrNotExist) {
		task := fixtureTasks(1)[0]
		lock := map[string]any{
			"schema_version":      PierInventoryLockSchemaVersion,
			"dataset_commit":      backend.manifest.Dataset.Commit,
			"coverage":            "full",
			"universe_task_count": 1,
			"tasks": []map[string]any{{
				"id": task.ID, "relative_path": task.ID, "base_commit": task.BaseCommit,
				"manifest_sha256": task.ManifestSHA256, "instruction_sha256": task.InstructionSHA256,
				"image": task.Image, "image_digest": task.ImageDigest,
			}},
		}
		if err := WriteJSONAtomic(archivePath, lock, 0o644); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	fileSHA, err := HashFile(archivePath)
	if err != nil {
		return err
	}
	backend.inventoryLock = InventoryLockSnapshot{
		RelativePath: InventoryLockArchiveRelativePath, FileSHA256: fileSHA,
		SchemaVersion: PierInventoryLockSchemaVersion, HashAlgorithm: TaskInventoryHashAlgorithm,
		DatasetCommit: backend.manifest.Dataset.Commit, Coverage: "full", TaskCount: 1, UniverseTaskCount: 1,
		TaskInventorySHA256: backend.manifest.Dataset.ManifestSHA256,
	}
	_, err = ValidateInventoryLockArchive(archivePath, backend.inventoryLock)
	return err
}

func (backend *fixtureBackend) Preflight(context.Context, Manifest) (BackendSnapshot, error) {
	snapshot := BackendSnapshot{
		Dataset:               SourceSnapshot{Commit: backend.manifest.Dataset.Commit, TreeSHA256: backend.manifest.Dataset.TreeSHA256, RawTreeSHA256: strings.Repeat("1", 64), ManifestSHA256: backend.manifest.Dataset.ManifestSHA256},
		Evaluator:             SourceSnapshot{Commit: backend.manifest.Evaluator.Commit, TreeSHA256: backend.manifest.Evaluator.TreeSHA256, RawTreeSHA256: strings.Repeat("2", 64), ManifestSHA256: backend.manifest.Evaluator.ManifestSHA256},
		EvaluatorVersion:      backend.manifest.Evaluator.MinimumVersion,
		EvaluatorBinarySHA256: backend.manifest.Evaluator.BinarySHA256,
		InventoryLock:         backend.inventoryLock,
		InventoryCoverage:     "full", InventoryTaskCount: 1, UniverseTaskCount: 1,
		AgentNetworkDeny: true, VerifierNetworkDeny: true, NetworkAttestation: "fixture-runtime-parser",
		EgressProxyImage:       "registry.example/proxy@sha256:" + strings.Repeat("e", 64),
		EgressProxyImageID:     "sha256:" + strings.Repeat("f", 64),
		AdapterImportPath:      "fixture:Agent",
		AdapterVersion:         "1.0.0",
		AdapterSHA256:          strings.Repeat("a", 64),
		AgentExecutionCanaries: fixtureExecutionCanarySnapshots(backend.manifest.Agents),
		ProviderEndpoint: ProviderEndpointSnapshot{
			ApprovedOrigin: FormalProviderOrigin, SemanticsSHA256: FormalProviderEndpointSemanticsSHA256,
			TLSServerName: FormalProviderTLSServerName, TLSVerified: true,
			TLSPeerLeafCertSHA256: strings.Repeat("b", 64), TLSPeerSPKISHA256: strings.Repeat("c", 64),
		},
		StorageEnforcement: FormalStorageEnforcement,
		HostStorageGuard:   FormalHostStorageGuard(),
		GuestStorageGuard:  FormalGuestStorageGuard(),
		StoragePreflight:   backend.storagePreflight,
	}
	if backend.snapshotMutator != nil {
		backend.snapshotMutator(&snapshot)
	}
	return snapshot, nil
}

func fixtureExecutionCanarySnapshots(agents []AgentSpec) []ExecutionCanarySnapshot {
	snapshots := make([]ExecutionCanarySnapshot, 0, len(agents))
	for _, agent := range agents {
		snapshots = append(snapshots, ExecutionCanarySnapshot{AgentID: agent.ID, Generation: agent.ExecutionCanary.Generation, TransportRequirement: agent.Model.TransportRequirement, ReceiptSHA256: agent.ExecutionCanary.ReceiptSHA256})
	}
	return snapshots
}

func (backend *fixtureBackend) Inventory(context.Context, SourcePin) ([]Task, error) {
	return fixtureTasks(1), nil
}

func (backend *fixtureBackend) PublicTask(context.Context, string) (PublicTaskView, error) {
	return PublicTaskView{ID: "task-a", BaseCommit: strings.Repeat("d", 40), InstructionSHA256: strings.Repeat("b", 64), InstructionPath: "/public/instruction.md", WorkspacePath: "/sandbox/app", Image: "registry.example/task:a", ImageDigest: "sha256:" + strings.Repeat("c", 64)}, nil
}

func (backend *fixtureBackend) VerifyOracle(_ context.Context, request OracleRequest) (VerificationResult, error) {
	if backend.oracleError != nil {
		return VerificationResult{}, backend.oracleError
	}
	if err := os.MkdirAll(request.ArtifactDir, 0o755); err != nil {
		return VerificationResult{}, err
	}
	rewardPath := filepath.Join(request.ArtifactDir, "reward.json")
	if err := os.WriteFile(rewardPath, []byte("{\"reward\":1}\n"), 0o644); err != nil {
		return VerificationResult{}, err
	}
	result := backend.oracleResult
	result.ArtifactPaths = []string{rewardPath}
	return result, nil
}

func (backend *fixtureBackend) RunAgent(_ context.Context, invocation AgentInvocation) (AgentExecution, error) {
	backend.agentRuns++
	if backend.agentRunsByID == nil {
		backend.agentRunsByID = map[string]int{}
	}
	backend.agentRunsByID[invocation.Agent.ID]++
	if backend.agentCrash {
		backend.agentCrash = false
		return AgentExecution{}, errors.New("fixture controller crash after durable provider WAL")
	}
	if backend.protocolFailures > 0 {
		backend.protocolFailures--
		return AgentExecution{}, AttemptProtocolError{Err: errors.New("fixture pinned model mismatch")}
	}
	if backend.infraFailures > 0 && backend.infraCategory == DeepSWEFailureNetworkInfrastructure {
		backend.infraFailures--
		now := time.Date(2026, 7, 26, 1, 2, 3, 0, time.UTC)
		runIdentity := strings.Repeat("6", 64)
		return AgentExecution{
				Lifecycle: AttemptLifecycle{
					SchemaVersion: "agentic-bench/attempt-lifecycle-v1", RunIdentity: runIdentity,
					ControllerStartedAt: now.Add(-time.Second), ControllerFinishedAt: now.Add(2 * time.Second), ProviderAttemptState: "no_provider_attempt",
				},
				TrialStartedAt: now, TrialFinishedAt: now.Add(time.Second), EvidenceRunIdentity: runIdentity,
			}, AttemptInfrastructureError{
				Category: DeepSWEFailureNetworkInfrastructure, Err: errors.New("fixture controller network outage"),
			}
	}
	patchPath := filepath.Join(invocation.ArtifactDir, "submission.patch")
	if err := os.WriteFile(patchPath, []byte("fixture patch\n"), 0o644); err != nil {
		return AgentExecution{}, err
	}
	evidencePath := filepath.Join(invocation.ArtifactDir, "metrics", "provider-requests.jsonl")
	if err := os.MkdirAll(filepath.Dir(evidencePath), 0o755); err != nil {
		return AgentExecution{}, err
	}
	now := time.Date(2026, 7, 26, 1, 2, 3, 0, time.UTC)
	rawEvidencePath := filepath.Join(invocation.ArtifactDir, "metrics", "provider-http.raw.jsonl")
	journalPath := rawEvidencePath + ".attempt-starts.jsonl"
	sealPath := rawEvidencePath + ".seal.json"
	for path, value := range map[string]any{
		rawEvidencePath: map[string]any{"schema_version": "fixture-provider-wire-v1", "request_count": 1},
		journalPath:     map[string]any{"schema_version": "fixture-attempt-start", "attempt_count": 1},
		sealPath:        map[string]any{"schema_version": "fixture-seal", "sealed": true},
	} {
		if err := WriteJSONAtomic(path, value, 0o600); err != nil {
			return AgentExecution{}, err
		}
	}
	rawEvidenceSHA, err := HashFile(rawEvidencePath)
	if err != nil {
		return AgentExecution{}, err
	}
	journalSHA, err := HashFile(journalPath)
	if err != nil {
		return AgentExecution{}, err
	}
	sealSHA, err := HashFile(sealPath)
	if err != nil {
		return AgentExecution{}, err
	}
	tool := invocation.Agent.Model.ToolCatalog.Tools[0]
	toolCallKind := "function_call"
	if tool.Type == "custom" {
		toolCallKind = "custom_tool_call"
	}
	round := ProviderRoundEvidence{
		SchemaVersion: "agentic-bench/provider-round-v2", Round: 0, Outcome: "success", RequestID: strings.Repeat("a", 64), ResponseIDHash: strings.Repeat("b", 64),
		StartedAt: now, UpstreamHeadersAt: now.Add(100 * time.Millisecond), FirstResponseByteAt: now.Add(200 * time.Millisecond), FinishedAt: now.Add(time.Second),
		Provider: invocation.Agent.Model.Provider, Model: invocation.Agent.Model.Model, ReasoningEffort: invocation.Agent.Model.ReasoningEffort, StoreSpecified: true,
		ClientAgentID: invocation.Agent.ID, Transport: "http_sse", ProviderAttemptKind: "inference",
		EncryptedReasoningRequested: true,
		HTTPStatus:                  200, RequestBytes: 1000, ResponseBytes: 100,
		UsagePresent: true, InputTokens: int64TestPointer(100), CachedInputTokens: int64TestPointer(80), CacheWriteInputTokens: int64TestPointer(0), OutputTokens: int64TestPointer(10),
		ToolCalls: []ToolCallEvidence{{ID: invocation.Agent.ID + "-tool", Kind: toolCallKind, Name: tool.Name}},
	}
	completeEvidenceTestRound(&round)
	canonicalizationEvidence, err := fixtureServiceTierCanonicalization(invocation, rawEvidenceSHA, &round)
	if err != nil {
		return AgentExecution{}, err
	}
	encoded, err := json.Marshal(round)
	if err != nil {
		return AgentExecution{}, err
	}
	if err := WriteBytesAtomic(evidencePath, append(encoded, '\n'), 0o644); err != nil {
		return AgentExecution{}, err
	}
	patchSHA, err := HashFile(patchPath)
	if err != nil {
		return AgentExecution{}, err
	}
	hostStorageEvidence, guestStorageEvidence, err := fixtureStorageResourceEvidence(invocation, now)
	if err != nil {
		return AgentExecution{}, err
	}
	exitClass, exitCode := backend.agentExitClass, 0
	if exitClass == "" {
		exitClass = "completed"
	}
	if exitClass == "timeout" {
		exitCode = 124
	}
	terminalSource, terminalCode := "process_exit", "completed"
	switch exitClass {
	case "timeout":
		terminalSource, terminalCode = "pier_trial", "agent_timeout"
	case "context_failure":
		terminalSource, terminalCode = "provider_event", "context_length_exceeded"
	case "nonzero":
		terminalCode = "nonzero_exit"
	}
	execution := AgentExecution{
		Lifecycle: AttemptLifecycle{
			SchemaVersion: "agentic-bench/attempt-lifecycle-v1", RunIdentity: round.RunIdentity,
			ControllerStartedAt: now.Add(-2 * time.Second), ControllerFinishedAt: now.Add(3 * time.Second),
			ProviderAttemptState: "provider_attempt_sealed", ProviderAttemptCount: 1,
		},
		ExitClass: exitClass, ExitCode: exitCode, StartedAt: now, FinishedAt: now.Add(time.Second), TrialStartedAt: now.Add(-time.Second), TrialFinishedAt: now.Add(2 * time.Second), SubmissionPatch: patchPath, AuditWorkspacePatch: patchPath, EvidencePath: evidencePath,
		EvidenceRunIdentity: round.RunIdentity,
		ProviderEvidence: ProviderEvidenceSeal{
			RawEvidencePath: rawEvidencePath, AttemptJournalPath: journalPath, SealPath: sealPath,
			RawEvidenceSHA256: rawEvidenceSHA, AttemptJournalSHA256: journalSHA, SealSHA256: sealSHA,
			StartedAttemptCount: 1, PersistedAttemptCount: 1, RecordCount: 1, LastEvidenceHash: round.EvidenceHash,
		},
		ServiceTierCanonicalization: canonicalizationEvidence,
		StorageEvidence:             hostStorageEvidence,
		GuestStorageEvidence:        guestStorageEvidence,
		TerminalEvidence: AgentTerminalEvidence{
			SchemaVersion: "agentic-bench/terminal-evidence-v1", Source: terminalSource, Code: terminalCode, EvidenceSHA256: strings.Repeat("7", 64),
		},
		Capture: SubmissionCaptureEvidence{Method: "official-git-diff+temporary-index-audit-v2", BaseCommit: invocation.Task.BaseCommit, PatchSHA256: patchSHA, AuditPatchSHA256: patchSHA, IncludesTracked: true, IncludesUntracked: true, IncludesBinary: true},
	}
	if backend.infraFailures > 0 {
		backend.infraFailures--
		return execution, AttemptInfrastructureError{Category: backend.infraCategory, Err: errors.New("fixture typed infrastructure failure")}
	}
	backend.verifierRuns++
	if backend.verifierFailures > 0 {
		backend.verifierFailures--
		return execution, AttemptInfrastructureError{Category: DeepSWEFailureVerifierInfrastructure, Err: errors.New("temporary verifier outage")}
	}
	verifierDir := filepath.Join(invocation.ArtifactDir, "verifier")
	if err := os.MkdirAll(verifierDir, 0o755); err != nil {
		return AgentExecution{}, err
	}
	rewardPath := filepath.Join(verifierDir, "reward.json")
	if err := os.WriteFile(rewardPath, []byte("{\"reward\":1}\n"), 0o644); err != nil {
		return AgentExecution{}, err
	}
	execution.Verification = &VerificationResult{ProtocolValid: true, Reward: 1, ArtifactPaths: []string{rewardPath}}
	return execution, nil
}

func (backend *fixtureBackend) RecoverAgent(context.Context, AgentInvocation) (AgentExecution, error) {
	backend.recoverRuns++
	if backend.recoverController {
		return AgentExecution{Lifecycle: AttemptLifecycle{
			SchemaVersion: "agentic-bench/attempt-lifecycle-v1", RunIdentity: strings.Repeat("8", 64),
			ControllerStartedAt:  time.Date(2026, 7, 26, 1, 2, 3, 0, time.UTC),
			ProviderAttemptState: "provider_attempt_started_unsealed", ProviderAttemptCount: 1, Recovered: true,
		}}, AttemptInfrastructureError{Category: DeepSWEFailureControllerInfrastructure, Err: errors.New("fixture recovered controller crash")}
	}
	return AgentExecution{}, errors.New("fixture has no sealed attempt to recover")
}

func TestRunnerValidatesAllOraclesBeforeLaunchingAgents(t *testing.T) {
	manifest := fixtureManifest(t)
	loaded := fixtureLoaded(t, manifest)
	backend := &fixtureBackend{manifest: manifest, oracleResult: VerificationResult{ProtocolValid: true, Reward: 0}}
	runner := Runner{Loaded: loaded, Backend: backend, WorkDir: t.TempDir(), HostEnvironment: []string{"OPENAI_API_KEY=test"}, Now: fixedClock()}
	state, _, err := runner.Run(context.Background())
	var invalid InvalidExperimentError
	if !errors.As(err, &invalid) {
		t.Fatalf("oracle failure returned %T %v, want InvalidExperimentError", err, err)
	}
	if state.Status != ExperimentInvalid || backend.agentRuns != 0 {
		t.Fatalf("oracle failure did not fail closed: status=%s agentRuns=%d", state.Status, backend.agentRuns)
	}
}

func TestRunnerResumeNeverRedispatchesReservedSlotAfterControllerCrash(t *testing.T) {
	manifest := fixtureManifest(t)
	loaded := fixtureLoaded(t, manifest)
	backend := &fixtureBackend{
		manifest: manifest, oracleResult: VerificationResult{ProtocolValid: true, Reward: 1},
		agentCrash: true, recoverController: true,
	}
	workDir := t.TempDir()
	runner := Runner{Loaded: loaded, Backend: backend, WorkDir: workDir, HostEnvironment: []string{"OPENAI_API_KEY=test"}, Now: fixedClock()}
	state, _, firstErr := runner.Run(context.Background())
	var infrastructure InfrastructureError
	if !errors.As(firstErr, &infrastructure) || state.Status != ExperimentRunning || backend.agentRuns != 1 {
		t.Fatalf("first crash run = status %s runs %d err %v", state.Status, backend.agentRuns, firstErr)
	}
	state, plan, secondErr := runner.Run(context.Background())
	if secondErr != nil {
		t.Fatal(secondErr)
	}
	if state.Status != ExperimentComplete || backend.agentRuns != 2 || backend.recoverRuns != 1 || backend.agentRunsByID[plan.Entries[0].AgentID] != 1 {
		t.Fatalf("resume redispatched immutable slot: status=%s agentRuns=%d recoverRuns=%d", state.Status, backend.agentRuns, backend.recoverRuns)
	}
	record := state.Runs[RunKey(plan.Entries[0])]
	if record.Disposition != DeepSWEAttemptExcluded || record.FailureCategory != DeepSWEFailureControllerInfrastructure || record.Execution == nil || !record.Execution.Lifecycle.Recovered || record.Metrics != nil {
		t.Fatalf("controller recovery ledger = %#v", record)
	}
}

func TestValidateRecoveredControllerAttemptRejectsSealedOrFabricatedOutput(t *testing.T) {
	base := AgentExecution{Lifecycle: AttemptLifecycle{
		SchemaVersion: "agentic-bench/attempt-lifecycle-v1", RunIdentity: strings.Repeat("8", 64),
		ControllerStartedAt:  time.Date(2026, 7, 26, 1, 2, 3, 0, time.UTC),
		ProviderAttemptState: "provider_attempt_started_unsealed", ProviderAttemptCount: 1, Recovered: true,
	}}
	if err := ValidateRecoveredControllerAttempt(base); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*AgentExecution){
		"sealed lifecycle": func(value *AgentExecution) {
			value.Lifecycle.ProviderAttemptState = "provider_attempt_sealed"
		},
		"capture": func(value *AgentExecution) {
			value.Capture.Method = "fabricated"
		},
		"provider seal": func(value *AgentExecution) {
			value.ProviderEvidence.RecordCount = 1
		},
		"evidence run identity": func(value *AgentExecution) {
			value.EvidenceRunIdentity = value.Lifecycle.RunIdentity
		},
	} {
		t.Run(name, func(t *testing.T) {
			invalid := base
			mutate(&invalid)
			if err := ValidateRecoveredControllerAttempt(invalid); err == nil {
				t.Fatal("invalid controller recovery was accepted")
			}
		})
	}
}

func TestRunnerExcludesVerifierInfrastructureWithoutReplacingRawSlot(t *testing.T) {
	manifest := fixtureManifest(t)
	loaded := fixtureLoaded(t, manifest)
	backend := &fixtureBackend{manifest: manifest, oracleResult: VerificationResult{ProtocolValid: true, Reward: 1}, verifierFailures: 1}
	workDir := t.TempDir()
	runner := Runner{Loaded: loaded, Backend: backend, WorkDir: workDir, HostEnvironment: []string{"OPENAI_API_KEY=test"}, Now: fixedClock()}
	state, plan, err := runner.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != ExperimentComplete || backend.agentRuns != 2 || backend.recoverRuns != 0 {
		t.Fatalf("resume produced status=%s agentRuns=%d", state.Status, backend.agentRuns)
	}
	if backend.verifierRuns != 2 {
		t.Fatalf("verifier runs = %d, want one immutable trial per agent", backend.verifierRuns)
	}
	excluded := state.Runs[RunKey(plan.Entries[0])]
	if excluded.Attempts != 1 || excluded.Phase != RunComplete || excluded.Disposition != DeepSWEAttemptExcluded || excluded.FailureCategory != DeepSWEFailureVerifierInfrastructure || excluded.Execution == nil || excluded.Metrics == nil || excluded.Verification != nil {
		t.Fatalf("verifier infra slot was not preserved as a typed exclusion: %#v", excluded)
	}
	ledgerPath := filepath.Join(workDir, manifest.Artifacts.Root, manifest.Artifacts.LedgerRelativePath)
	if _, err := os.Stat(ledgerPath); err != nil {
		t.Fatalf("artifact ledger missing: %v", err)
	}
	scorecardRaw, err := os.ReadFile(filepath.Join(workDir, manifest.Artifacts.Root, "scorecard.json"))
	if err != nil {
		t.Fatalf("formal scorecard missing: %v", err)
	}
	var formal Scorecard
	if err := json.Unmarshal(scorecardRaw, &formal); err != nil {
		t.Fatal(err)
	}
	if formal.SchemaVersion != "agentic-bench/scorecard-v2" || formal.Profile != ScoringProfileDeepSWEV11PublicCI || formal.DeepSWEPublic == nil {
		t.Fatalf("runner bypassed the public scoring profile: %#v", formal)
	}
	if formal.DeepSWEPublic.Agents[0].PassAt4 != nil || formal.DeepSWEPublic.Agents[0].FourRunStatistics != nil {
		t.Fatalf("one-run runner output claimed four-run statistics: %#v", formal.DeepSWEPublic.Agents[0])
	}
}

func TestRunnerScoresPassingPartialPatchAsFailureAfterAgentTerminalFailure(t *testing.T) {
	for _, exitClass := range []string{"timeout", "context_failure"} {
		t.Run(exitClass, func(t *testing.T) {
			manifest := fixtureManifest(t)
			loaded := fixtureLoaded(t, manifest)
			backend := &fixtureBackend{
				manifest: manifest, oracleResult: VerificationResult{ProtocolValid: true, Reward: 1},
				agentExitClass: exitClass,
			}
			runner := Runner{Loaded: loaded, Backend: backend, WorkDir: t.TempDir(), HostEnvironment: []string{"OPENAI_API_KEY=test"}, Now: fixedClock()}
			state, plan, err := runner.Run(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range plan.Entries {
				record := state.Runs[RunKey(entry)]
				wantCategory := DeepSWEFailureContext
				if exitClass == "timeout" {
					wantCategory = DeepSWEFailureAgentTimeout
				}
				if record.Disposition != DeepSWEAttemptScored || record.FailureCategory != wantCategory || record.Verification == nil || record.Verification.Reward != 0 || record.Verification.RawReward == nil || *record.Verification.RawReward != 1 {
					t.Fatalf("terminal partial-patch classification = %#v", record)
				}
			}
		})
	}
}

func TestRunnerExcludesProviderAndNetworkInfrastructureWithoutReplacement(t *testing.T) {
	for _, category := range []DeepSWEFailureCategory{DeepSWEFailureProviderInfrastructure, DeepSWEFailureNetworkInfrastructure} {
		t.Run(string(category), func(t *testing.T) {
			manifest := fixtureManifest(t)
			loaded := fixtureLoaded(t, manifest)
			backend := &fixtureBackend{
				manifest: manifest, oracleResult: VerificationResult{ProtocolValid: true, Reward: 1},
				infraFailures: 1, infraCategory: category,
			}
			runner := Runner{Loaded: loaded, Backend: backend, WorkDir: t.TempDir(), HostEnvironment: []string{"OPENAI_API_KEY=test"}, Now: fixedClock()}
			state, plan, err := runner.Run(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if state.Status != ExperimentComplete || backend.agentRuns != len(plan.Entries) || backend.recoverRuns != 0 {
				t.Fatalf("typed exclusion launched replacement work: status=%s runs=%d recover=%d", state.Status, backend.agentRuns, backend.recoverRuns)
			}
			record := state.Runs[RunKey(plan.Entries[0])]
			if record.Attempts != 1 || record.Disposition != DeepSWEAttemptExcluded || record.FailureCategory != category || record.Execution == nil || record.Verification != nil {
				t.Fatalf("typed exclusion = %#v", record)
			}
			if category == DeepSWEFailureProviderInfrastructure && record.Metrics == nil {
				t.Fatal("provider exclusion discarded paid request evidence")
			}
			if category == DeepSWEFailureNetworkInfrastructure && record.Metrics != nil {
				t.Fatal("network exclusion synthesized nonexistent provider evidence")
			}
		})
	}
}

func TestRunnerRejectsBackendIdentityDriftBeforeResumeWork(t *testing.T) {
	mutations := map[string]func(*BackendSnapshot){
		"proxy image ID":      func(value *BackendSnapshot) { value.EgressProxyImageID = "sha256:" + strings.Repeat("9", 64) },
		"evaluator version":   func(value *BackendSnapshot) { value.EvaluatorVersion = "0.3.1" },
		"network attestation": func(value *BackendSnapshot) { value.NetworkAttestation = "changed-parser" },
		"dataset raw tree":    func(value *BackendSnapshot) { value.Dataset.RawTreeSHA256 = strings.Repeat("8", 64) },
		"endpoint semantics":  func(value *BackendSnapshot) { value.ProviderEndpoint.SemanticsSHA256 = strings.Repeat("7", 64) },
		"TLS server name":     func(value *BackendSnapshot) { value.ProviderEndpoint.TLSServerName = "other.example" },
		"TLS verification":    func(value *BackendSnapshot) { value.ProviderEndpoint.TLSVerified = false },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			manifest := fixtureManifest(t)
			loaded := fixtureLoaded(t, manifest)
			backend := &fixtureBackend{manifest: manifest, oracleResult: VerificationResult{ProtocolValid: true, Reward: 1}}
			workDir := t.TempDir()
			runner := Runner{Loaded: loaded, Backend: backend, WorkDir: workDir, HostEnvironment: []string{"OPENAI_API_KEY=test"}, Now: fixedClock(), StopAfterOracles: true}
			if _, _, err := runner.Run(context.Background()); err != nil {
				t.Fatal(err)
			}
			backend.snapshotMutator = mutate
			runner.StopAfterOracles = false
			_, _, err := runner.Run(context.Background())
			var invalid InvalidExperimentError
			if !errors.As(err, &invalid) || backend.agentRuns != 0 {
				t.Fatalf("backend drift resumed work: err=%v agentRuns=%d", err, backend.agentRuns)
			}
		})
	}
}

func TestRunnerAllowsVerifiedProviderCertificateRotationOnResume(t *testing.T) {
	manifest := fixtureManifest(t)
	loaded := fixtureLoaded(t, manifest)
	backend := &fixtureBackend{manifest: manifest, oracleResult: VerificationResult{ProtocolValid: true, Reward: 1}}
	workDir := t.TempDir()
	runner := Runner{Loaded: loaded, Backend: backend, WorkDir: workDir, HostEnvironment: []string{"OPENAI_API_KEY=test"}, Now: fixedClock(), StopAfterOracles: true}
	if _, _, err := runner.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	backend.snapshotMutator = func(value *BackendSnapshot) {
		value.ProviderEndpoint.TLSPeerLeafCertSHA256 = strings.Repeat("6", 64)
		value.ProviderEndpoint.TLSPeerSPKISHA256 = strings.Repeat("5", 64)
	}
	runner.StopAfterOracles = false
	state, plan, err := runner.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != ExperimentComplete || backend.agentRuns != len(plan.Entries) {
		t.Fatalf("verified TLS peer rotation did not resume: status=%s agentRuns=%d", state.Status, backend.agentRuns)
	}
}

func TestRunnerRejectsTamperedInventoryLockBeforeResumeWork(t *testing.T) {
	manifest := fixtureManifest(t)
	loaded := fixtureLoaded(t, manifest)
	backend := &fixtureBackend{manifest: manifest, oracleResult: VerificationResult{ProtocolValid: true, Reward: 1}}
	workDir := t.TempDir()
	runner := Runner{
		Loaded: loaded, Backend: backend, WorkDir: workDir,
		HostEnvironment: []string{"OPENAI_API_KEY=test"}, Now: fixedClock(), StopAfterOracles: true,
	}
	if _, _, err := runner.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(workDir, manifest.Artifacts.Root, InventoryLockArchiveRelativePath)
	if err := os.WriteFile(archivePath, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner.StopAfterOracles = false
	_, _, err := runner.Run(context.Background())
	var invalid InvalidExperimentError
	if !errors.As(err, &invalid) || backend.agentRuns != 0 {
		t.Fatalf("tampered inventory lock resumed work: err=%v agentRuns=%d", err, backend.agentRuns)
	}
}

func TestRunnerInvalidatesTypedAttemptProtocolErrorWithoutReplacement(t *testing.T) {
	manifest := fixtureManifest(t)
	loaded := fixtureLoaded(t, manifest)
	backend := &fixtureBackend{
		manifest: manifest, oracleResult: VerificationResult{ProtocolValid: true, Reward: 1}, protocolFailures: 1,
	}
	workDir := t.TempDir()
	runner := Runner{Loaded: loaded, Backend: backend, WorkDir: workDir, HostEnvironment: []string{"OPENAI_API_KEY=test"}, Now: fixedClock()}
	state, _, err := runner.Run(context.Background())
	var invalid InvalidExperimentError
	if !errors.As(err, &invalid) || state.Status != ExperimentInvalid || backend.agentRuns != 1 {
		t.Fatalf("protocol violation = status=%s runs=%d err=%v", state.Status, backend.agentRuns, err)
	}
	if _, _, err := runner.Run(context.Background()); !errors.As(err, &invalid) || backend.agentRuns != 1 {
		t.Fatalf("invalid resume launched replacement: runs=%d err=%v", backend.agentRuns, err)
	}
}

func TestScoreExperimentRejectsInfrastructureAsZero(t *testing.T) {
	manifest := fixtureManifest(t)
	plan := RunPlan{Entries: []PlanEntry{{TaskID: "task-a", AgentID: "codex"}}}
	state := ExperimentState{Status: ExperimentComplete, Oracle: map[string]OracleRecord{"task-a": {TaskID: "task-a", Validated: true, Verification: VerificationResult{ProtocolValid: true, Reward: 1}}}, Runs: map[string]RunRecord{}}
	if _, err := ScoreExperiment(state, plan); err == nil {
		t.Fatal("missing run was silently scored as zero")
	}
	_ = manifest
}

func fixedClock() func() time.Time {
	now := time.Date(2026, 7, 26, 1, 2, 3, 0, time.UTC)
	return func() time.Time {
		now = now.Add(time.Millisecond)
		return now
	}
}
