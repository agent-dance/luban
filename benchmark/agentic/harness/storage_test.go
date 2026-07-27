package harness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const gib = uint64(1024 * 1024 * 1024)

func TestFormalHostStorageAdmissionAndPreflight(t *testing.T) {
	resources := formalStorageTestResources()

	preflight := validStorageAdmissionReceipt(StorageStageExperimentPreflight, 120*gib)
	if err := ValidateStoragePreflightReceipt(preflight, resources); err != nil {
		t.Fatalf("validate formal preflight: %v", err)
	}
	admission := validStorageAdmissionReceipt(StorageStageRawSlotAdmission, 40*gib)
	if err := ValidateStorageAdmissionReceipt(admission, resources); err != nil {
		t.Fatalf("validate formal raw-slot admission: %v", err)
	}
	if !admission.Warning {
		t.Fatal("40 GiB raw-slot admission must carry the below-50-GiB warning")
	}
}

func TestHostStorageAdmissionRejectsNonFormalEvidence(t *testing.T) {
	resources := formalStorageTestResources()
	tests := map[string]func(*StorageAdmissionReceipt, *ResourceSpec){
		"development guard": func(_ *StorageAdmissionReceipt, resources *ResourceSpec) {
			resources.HostStorageGuard.RuntimeHardFloorAvailableBytes--
		},
		"guest authority": func(receipt *StorageAdmissionReceipt, _ *ResourceSpec) {
			receipt.Authority = formalGuestSupervisorAuthority
		},
		"not apfs": func(receipt *StorageAdmissionReceipt, _ *ResourceSpec) {
			receipt.Filesystems[0].FilesystemType = "overlay"
		},
		"below floor": func(receipt *StorageAdmissionReceipt, _ *ResourceSpec) {
			receipt.Filesystems[0].AvailableBytes = 29 * gib
			receipt.Warning = true
		},
		"missing role": func(receipt *StorageAdmissionReceipt, _ *ResourceSpec) {
			receipt.Filesystems[0].Roles = receipt.Filesystems[0].Roles[:2]
			receipt.Filesystems[0].DeviceRoleCount = 2
		},
		"noncanonical group": func(receipt *StorageAdmissionReceipt, _ *ResourceSpec) {
			receipt.Filesystems[0].Group = 7
		},
		"warning mismatch": func(receipt *StorageAdmissionReceipt, _ *ResourceSpec) {
			receipt.Warning = false
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			receipt := validStorageAdmissionReceipt(StorageStageRawSlotAdmission, 40*gib)
			candidateResources := resources
			mutate(&receipt, &candidateResources)
			if err := ValidateStorageAdmissionReceipt(receipt, candidateResources); err == nil {
				t.Fatal("expected formal admission rejection")
			}
		})
	}
}

func TestHostStorageRuntimeEvidenceValidatesStrictArchiveAndBoundaries(t *testing.T) {
	resources := formalStorageTestResources()
	admission := validStorageAdmissionReceipt(StorageStageRawSlotAdmission, 40*gib)
	receipt := validHostStorageRuntimeReceipt(admission)
	artifactDir := t.TempDir()
	evidence := writeHostStorageEvidence(t, artifactDir, receipt)
	if err := ValidateStorageResourceEvidence(artifactDir, evidence, admission, resources); err != nil {
		t.Fatalf("validate formal host runtime evidence: %v", err)
	}

	t.Run("hash mismatch", func(t *testing.T) {
		candidate := evidence
		candidate.ReceiptSHA256 = strings.Repeat("a", 64)
		if candidate.ReceiptSHA256 == evidence.ReceiptSHA256 {
			candidate.ReceiptSHA256 = strings.Repeat("b", 64)
		}
		if err := ValidateStorageResourceEvidence(artifactDir, candidate, admission, resources); err == nil {
			t.Fatal("expected archive hash rejection")
		}
	})

	t.Run("embedded mismatch", func(t *testing.T) {
		candidate := evidence
		candidate.Receipt.Status = "development_ok"
		if err := ValidateStorageResourceEvidence(artifactDir, candidate, admission, resources); err == nil {
			t.Fatal("expected embedded/archive equality rejection")
		}
	})

	t.Run("trailing json", func(t *testing.T) {
		candidateDir := t.TempDir()
		candidate := writeHostStorageEvidence(t, candidateDir, receipt)
		path := filepath.Join(candidateDir, filepath.FromSlash(candidate.ReceiptRelativePath))
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		raw = append(raw, []byte("\n{}")...)
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		candidate.ReceiptSHA256, err = HashFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := ValidateStorageResourceEvidence(candidateDir, candidate, admission, resources); err == nil {
			t.Fatal("expected trailing JSON rejection")
		}
	})

	t.Run("duplicate json key", func(t *testing.T) {
		candidateDir := t.TempDir()
		candidate := writeHostStorageEvidence(t, candidateDir, receipt)
		path := filepath.Join(candidateDir, filepath.FromSlash(candidate.ReceiptRelativePath))
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		prefix := []byte(`{"schema_version":"agentic-bench/storage-resource-receipt-v1",`)
		raw = append(prefix, raw[1:]...)
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		candidate.ReceiptSHA256, err = HashFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := ValidateStorageResourceEvidence(candidateDir, candidate, admission, resources); err == nil {
			t.Fatal("expected duplicate JSON key rejection")
		}
	})

	tests := map[string]func(*StorageResourceReceipt){
		"provider wal delta mismatch": func(receipt *StorageResourceReceipt) {
			receipt.ProviderWALStartedDeltaMS++
		},
		"runtime identity drift": func(receipt *StorageResourceReceipt) {
			receipt.Filesystems[0].VolumeIdentitySHA256 = strings.Repeat("2", 64)
		},
		"runtime floor breach": func(receipt *StorageResourceReceipt) {
			receipt.Filesystems[0].SamplePoints[1].AvailableBytes = 29 * gib
			receipt.Filesystems[0].MinimumAvailableBytes = 29 * gib
		},
		"first boundary gap": func(receipt *StorageResourceReceipt) {
			points := receipt.Filesystems[0].SamplePoints
			points[0].StartDeltaMS, points[0].EndDeltaMS = 2500, 2601
			points[0].ObservedAt = receipt.StartedAt.Add(2601 * time.Millisecond)
			points[1].StartDeltaMS, points[1].EndDeltaMS = 2700, 3400
			points[1].ObservedAt = receipt.StartedAt.Add(3400 * time.Millisecond)
			points[2].StartDeltaMS, points[2].EndDeltaMS = 3500, 4400
			points[2].ObservedAt = receipt.StartedAt.Add(4400 * time.Millisecond)
			receipt.Filesystems[0].MaximumCompletionGapMS = 2601
		},
		"last boundary gap": func(receipt *StorageResourceReceipt) {
			receipt.FinishedAt = receipt.StartedAt.Add(7001 * time.Millisecond)
			receipt.FinishedDeltaMS = 7001
			receipt.Filesystems[0].MaximumCompletionGapMS = 3001
		},
		"wall time outside phase": func(receipt *StorageResourceReceipt) {
			receipt.Filesystems[0].SamplePoints[0].ObservedAt = receipt.StartedAt.Add(-time.Millisecond)
		},
		"development status": func(receipt *StorageResourceReceipt) {
			receipt.Status = "pilot_completed"
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := cloneHostReceipt(t, receipt)
			mutate(&candidate)
			candidateDir := t.TempDir()
			candidateEvidence := writeHostStorageEvidence(t, candidateDir, candidate)
			if err := ValidateStorageResourceEvidence(candidateDir, candidateEvidence, admission, resources); err == nil {
				t.Fatal("expected formal host runtime evidence rejection")
			}
		})
	}
}

func TestGuestStorageAgentAndVerifierEvidence(t *testing.T) {
	resources := formalStorageTestResources()
	artifactDir := t.TempDir()
	receipts := validGuestStorageReceipts()
	evidence := writeGuestStorageEvidence(t, artifactDir, receipts)
	if err := ValidateGuestStorageResourceEvidence(artifactDir, evidence, resources); err != nil {
		t.Fatalf("validate formal guest storage evidence: %v", err)
	}

	if err := ValidateGuestStorageResourceEvidence(artifactDir, evidence[:1], resources); err == nil {
		t.Fatal("expected missing verifier receipt rejection")
	}
	swapped := []GuestStorageResourceEvidence{evidence[1], evidence[0]}
	if err := ValidateGuestStorageResourceEvidence(artifactDir, swapped, resources); err == nil {
		t.Fatal("expected noncanonical phase order rejection")
	}

	tests := map[string]func([]GuestStorageResourceReceipt){
		"session mismatch": func(receipts []GuestStorageResourceReceipt) {
			receipts[1].SessionIdentitySHA256 = strings.Repeat("4", 64)
		},
		"start below 28 GiB": func(receipts []GuestStorageResourceReceipt) {
			receipts[0].Filesystems[0].Samples[0].AvailableBytes = 27 * gib
			receipts[0].Filesystems[0].MinimumAvailableBytes = 27 * gib
		},
		"runtime below 8 GiB": func(receipts []GuestStorageResourceReceipt) {
			receipts[0].Filesystems[0].Samples[1].AvailableBytes = 7 * gib
			receipts[0].Filesystems[0].MinimumAvailableBytes = 7 * gib
		},
		"first boundary monitoring gap": func(receipts []GuestStorageResourceReceipt) {
			points := receipts[0].Filesystems[0].Samples
			points[0].StartDeltaMS, points[0].EndDeltaMS = 2500, 2601
			points[0].ObservedAt = receipts[0].StartedAt.Add(2601 * time.Millisecond)
			points[1].StartDeltaMS, points[1].EndDeltaMS = 2700, 3100
			points[1].ObservedAt = receipts[0].StartedAt.Add(3100 * time.Millisecond)
			points[2].StartDeltaMS, points[2].EndDeltaMS = 3200, 3600
			points[2].ObservedAt = receipts[0].StartedAt.Add(3600 * time.Millisecond)
			receipts[0].Filesystems[0].MaximumCompletionGapMS = 2601
		},
		"not configured for 64 GiB": func(receipts []GuestStorageResourceReceipt) {
			receipts[0].ConfiguredCapacityBytes--
		},
		"unknown role": func(receipts []GuestStorageResourceReceipt) {
			receipts[0].Filesystems[0].Roles[0] = "guest_logs"
		},
		"identity not pinned": func(receipts []GuestStorageResourceReceipt) {
			receipts[0].Filesystems[0].VolumeIdentitySHA256 = "not-a-digest"
		},
		"nonformal failure status": func(receipts []GuestStorageResourceReceipt) {
			receipts[0].Status = "aborted_below_floor_after_provider_wal"
		},
		"verifier before provider wal": func(receipts []GuestStorageResourceReceipt) {
			receipts[1].ProviderWALStartedAt = receipts[1].StartedAt.Add(time.Second)
			receipts[1].ProviderWALStartedDeltaMS = 1000
		},
		"different provider wal boundary": func(receipts []GuestStorageResourceReceipt) {
			receipts[1].ProviderWALStartedAt = receipts[1].ProviderWALStartedAt.Add(time.Millisecond)
			receipts[1].ProviderWALStartedDeltaMS++
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := cloneGuestReceipts(t, receipts)
			mutate(candidate)
			candidateDir := t.TempDir()
			candidateEvidence := writeGuestStorageEvidence(t, candidateDir, candidate)
			if err := ValidateGuestStorageResourceEvidence(candidateDir, candidateEvidence, resources); err == nil {
				t.Fatal("expected formal guest storage evidence rejection")
			}
		})
	}
}

func TestGuestStorageRejectsDevelopmentGuardAndNonRegularArchive(t *testing.T) {
	resources := formalStorageTestResources()
	artifactDir := t.TempDir()
	receipts := validGuestStorageReceipts()
	evidence := writeGuestStorageEvidence(t, artifactDir, receipts)

	resources.GuestStorageGuard.RuntimeAbortBelowAvailableBytes--
	if err := ValidateGuestStorageResourceEvidence(artifactDir, evidence, resources); err == nil {
		t.Fatal("expected development guest guard rejection")
	}

	resources = formalStorageTestResources()
	badDir := t.TempDir()
	path := filepath.Join(badDir, filepath.FromSlash(GuestStorageAgentReceiptRelativePath))
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	badEvidence := append([]GuestStorageResourceEvidence(nil), evidence...)
	badEvidence[0].ReceiptRelativePath = GuestStorageAgentReceiptRelativePath
	if err := ValidateGuestStorageResourceEvidence(badDir, badEvidence, resources); err == nil {
		t.Fatal("expected non-regular archive rejection")
	}
}

func formalStorageTestResources() ResourceSpec {
	return ResourceSpec{
		StorageMB:         formalDeclaredStorageMB,
		HostStorageGuard:  FormalHostStorageGuard(),
		GuestStorageGuard: FormalGuestStorageGuard(),
	}
}

func validStorageAdmissionReceipt(stage string, available uint64) StorageAdmissionReceipt {
	observedAt := time.Date(2026, time.July, 26, 0, 0, 1, 0, time.UTC)
	return StorageAdmissionReceipt{
		SchemaVersion:     StorageAdmissionSchemaVersion,
		Stage:             stage,
		Enforcement:       FormalStorageEnforcement,
		DeclaredStorageMB: formalDeclaredStorageMB,
		Guard:             FormalHostStorageGuard(),
		Authority:         formalHostSupervisorAuthority,
		ObservedAt:        observedAt,
		Filesystems: []StorageAdmissionFilesystemReceipt{{
			Group:                0,
			Roles:                append([]string(nil), formalHostStorageRoles...),
			VolumeIdentitySHA256: strings.Repeat("1", 64),
			FilesystemType:       formalHostFilesystemType,
			DeviceRoleCount:      len(formalHostStorageRoles),
			ObservedAt:           observedAt.Add(-time.Millisecond),
			MonotonicOffsetMS:    1,
			BlockSizeBytes:       4096,
			TotalBytes:           200 * gib,
			AvailableBytes:       available,
			UsedBytes:            70 * gib,
		}},
		Passed:  true,
		Warning: available < FormalStorageRuntimeWarningBytes,
	}
}

func validHostStorageRuntimeReceipt(admission StorageAdmissionReceipt) StorageResourceReceipt {
	startedAt := admission.ObservedAt.Add(time.Second)
	finishedAt := startedAt.Add(5 * time.Second)
	points := []StorageStatfsSample{
		{ObservedAt: startedAt.Add(500 * time.Millisecond), StartDeltaMS: 450, EndDeltaMS: 500, AvailableBytes: 40 * gib, UsedBytes: 70 * gib},
		{ObservedAt: startedAt.Add(2 * time.Second), StartDeltaMS: 1950, EndDeltaMS: 2000, AvailableBytes: 39 * gib, UsedBytes: 71 * gib},
		{ObservedAt: startedAt.Add(4 * time.Second), StartDeltaMS: 3950, EndDeltaMS: 4000, AvailableBytes: 38 * gib, UsedBytes: 72 * gib},
	}
	return StorageResourceReceipt{
		SchemaVersion:             StorageReceiptSchemaVersion,
		Enforcement:               FormalStorageEnforcement,
		DeclaredStorageMB:         formalDeclaredStorageMB,
		Guard:                     FormalHostStorageGuard(),
		Authority:                 formalHostSupervisorAuthority,
		Admission:                 admission,
		StartedAt:                 startedAt,
		FinishedAt:                finishedAt,
		ProviderWALStartedAt:      startedAt.Add(time.Second),
		ProviderWALStartedDeltaMS: 1000,
		FinishedDeltaMS:           5000,
		Filesystems: []StorageRuntimeFilesystemReceipt{{
			Group:                  0,
			Roles:                  append([]string(nil), formalHostStorageRoles...),
			VolumeIdentitySHA256:   admission.Filesystems[0].VolumeIdentitySHA256,
			FilesystemType:         admission.Filesystems[0].FilesystemType,
			DeviceRoleCount:        admission.Filesystems[0].DeviceRoleCount,
			BlockSizeBytes:         admission.Filesystems[0].BlockSizeBytes,
			TotalBytes:             admission.Filesystems[0].TotalBytes,
			AvailableBeforeBytes:   points[0].AvailableBytes,
			AvailableAfterBytes:    points[2].AvailableBytes,
			MinimumAvailableBytes:  points[2].AvailableBytes,
			UsedBeforeBytes:        points[0].UsedBytes,
			UsedAfterBytes:         points[2].UsedBytes,
			MaximumUsedBytes:       points[2].UsedBytes,
			Samples:                uint64(len(points)),
			WarningSamples:         uint64(len(points)),
			MaximumCompletionGapMS: 2000,
			SamplePoints:           points,
		}},
		Status: StorageStatusCompletedAboveGuard,
	}
}

func validGuestStorageReceipts() []GuestStorageResourceReceipt {
	session := strings.Repeat("a", 64)
	base := time.Date(2026, time.July, 26, 1, 0, 0, 0, time.UTC)
	agent := validGuestStorageReceipt(GuestStoragePhaseAgent, session, strings.Repeat("b", 64), base, base.Add(time.Second), 4*time.Second)
	verifierStart := base.Add(10 * time.Second)
	verifier := validGuestStorageReceipt(GuestStoragePhaseVerifier, session, strings.Repeat("c", 64), verifierStart, agent.ProviderWALStartedAt, 3*time.Second)
	return []GuestStorageResourceReceipt{agent, verifier}
}

func validGuestStorageReceipt(phase, session, container string, startedAt, providerWALStartedAt time.Time, duration time.Duration) GuestStorageResourceReceipt {
	finishedAt := startedAt.Add(duration)
	endDeltas := []int64{500, 2000, duration.Milliseconds() - 500}
	available := []uint64{40 * gib, 39 * gib, 38 * gib}
	used := []uint64{20 * gib, 21 * gib, 22 * gib}
	points := make([]StorageStatfsSample, len(endDeltas))
	previousEnd := int64(0)
	maximumGap := int64(0)
	for index, end := range endDeltas {
		start := max(previousEnd, end-50)
		points[index] = StorageStatfsSample{
			ObservedAt:     startedAt.Add(time.Duration(end) * time.Millisecond),
			StartDeltaMS:   start,
			EndDeltaMS:     end,
			AvailableBytes: available[index],
			UsedBytes:      used[index],
		}
		maximumGap = max(maximumGap, end-previousEnd)
		previousEnd = end
	}
	maximumGap = max(maximumGap, duration.Milliseconds()-previousEnd)
	return GuestStorageResourceReceipt{
		SchemaVersion:             GuestStorageReceiptSchemaVersion,
		Phase:                     phase,
		SessionIdentitySHA256:     session,
		ContainerIdentitySHA256:   container,
		Enforcement:               FormalStorageEnforcement,
		DeclaredStorageMB:         formalDeclaredStorageMB,
		ConfiguredCapacityBytes:   FormalGuestStorageConfiguredBytes,
		Guard:                     FormalGuestStorageGuard(),
		Authority:                 formalGuestSupervisorAuthority,
		StartedAt:                 startedAt,
		FinishedAt:                finishedAt,
		ProviderWALStartedAt:      providerWALStartedAt,
		ProviderWALStartedDeltaMS: providerWALStartedAt.Sub(startedAt).Milliseconds(),
		FinishedDeltaMS:           duration.Milliseconds(),
		Filesystems: []GuestStorageFilesystemReceipt{{
			Group:                  0,
			Roles:                  append([]string(nil), formalGuestStorageRoles...),
			VolumeIdentitySHA256:   strings.Repeat("d", 64),
			FilesystemType:         "overlay",
			DeviceRoleCount:        len(formalGuestStorageRoles),
			BlockSizeBytes:         4096,
			TotalBytes:             63*gib + 900*1024*1024,
			MinimumAvailableBytes:  available[len(available)-1],
			MaximumUsedBytes:       used[len(used)-1],
			MaximumCompletionGapMS: maximumGap,
			Samples:                points,
		}},
		Status: StorageStatusCompletedAboveGuard,
	}
}

func writeHostStorageEvidence(t *testing.T, artifactDir string, receipt StorageResourceReceipt) StorageResourceEvidence {
	t.Helper()
	path := filepath.Join(artifactDir, filepath.FromSlash(StorageReceiptRelativePath))
	archived := writeReceipt(t, path, receipt)
	var decoded StorageResourceReceipt
	decodeReceiptForTest(t, archived, &decoded)
	digest, err := HashFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return StorageResourceEvidence{
		SchemaVersion:       StorageEvidenceSchemaVersion,
		ReceiptRelativePath: StorageReceiptRelativePath,
		ReceiptSHA256:       digest,
		Receipt:             decoded,
	}
}

func writeGuestStorageEvidence(t *testing.T, artifactDir string, receipts []GuestStorageResourceReceipt) []GuestStorageResourceEvidence {
	t.Helper()
	evidence := make([]GuestStorageResourceEvidence, len(receipts))
	for index, receipt := range receipts {
		path, exists := formalGuestReceiptPaths[receipt.Phase]
		if !exists {
			path = GuestStorageAgentReceiptRelativePath
		}
		archivePath := filepath.Join(artifactDir, filepath.FromSlash(path))
		archived := writeReceipt(t, archivePath, receipt)
		var decoded GuestStorageResourceReceipt
		decodeReceiptForTest(t, archived, &decoded)
		digest, err := HashFile(archivePath)
		if err != nil {
			t.Fatal(err)
		}
		evidence[index] = GuestStorageResourceEvidence{
			SchemaVersion:       GuestStorageEvidenceSchemaVersion,
			ReceiptRelativePath: path,
			ReceiptSHA256:       digest,
			Receipt:             decoded,
		}
	}
	return evidence
}

func writeReceipt(t *testing.T, path string, receipt any) []byte {
	t.Helper()
	raw, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return raw
}

func decodeReceiptForTest(t *testing.T, raw []byte, destination any) {
	t.Helper()
	if err := json.Unmarshal(raw, destination); err != nil {
		t.Fatal(err)
	}
}

func cloneHostReceipt(t *testing.T, receipt StorageResourceReceipt) StorageResourceReceipt {
	t.Helper()
	raw, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	var clone StorageResourceReceipt
	decodeReceiptForTest(t, raw, &clone)
	return clone
}

func cloneGuestReceipts(t *testing.T, receipts []GuestStorageResourceReceipt) []GuestStorageResourceReceipt {
	t.Helper()
	raw, err := json.Marshal(receipts)
	if err != nil {
		t.Fatal(err)
	}
	var clone []GuestStorageResourceReceipt
	decodeReceiptForTest(t, raw, &clone)
	return clone
}
