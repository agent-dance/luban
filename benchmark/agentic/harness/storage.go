package harness

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"time"
)

const (
	formalDeclaredStorageMB        = 20480
	formalHostFilesystemType       = "apfs"
	formalHostSupervisorAuthority  = "external-authenticated-host-supervisor-statfs-bavail-v1"
	formalGuestSupervisorAuthority = "guest-pinned-fd-statfs-bavail-v1"
)

var (
	formalHostStorageRoles  = []string{"artifact_root", "controller_root", "private_work_root"}
	formalGuestStorageRoles = []string{"guest_app", "guest_root"}
	formalGuestPhases       = []string{GuestStoragePhaseAgent, GuestStoragePhaseVerifier}
	formalGuestReceiptPaths = map[string]string{
		GuestStoragePhaseAgent:    GuestStorageAgentReceiptRelativePath,
		GuestStoragePhaseVerifier: GuestStorageVerifierReceiptRelativePath,
	}
)

// ValidateStorageAdmissionReceipt verifies the content-free host-wide Statfs
// observation taken before a raw attempt slot or provider WAL is reserved.
func ValidateStorageAdmissionReceipt(receipt StorageAdmissionReceipt, resources ResourceSpec) error {
	return validateStorageCapacityReceipt(receipt, resources, StorageStageRawSlotAdmission, resources.HostStorageGuard.RuntimeHardFloorAvailableBytes)
}

// ValidateStoragePreflightReceipt verifies the 100 GiB whole-experiment gate,
// which is re-run on every resume before inventory or provider preflight.
func ValidateStoragePreflightReceipt(receipt StorageAdmissionReceipt, resources ResourceSpec) error {
	return validateStorageCapacityReceipt(receipt, resources, StorageStageExperimentPreflight, resources.HostStorageGuard.AdmissionMinimumAvailableBytes)
}

func validateStorageCapacityReceipt(receipt StorageAdmissionReceipt, resources ResourceSpec, stage string, minimum uint64) error {
	if err := validateFormalHostResources(resources); err != nil {
		return err
	}
	if StorageStatfsAuthority != formalHostSupervisorAuthority ||
		receipt.SchemaVersion != StorageAdmissionSchemaVersion ||
		receipt.Enforcement != FormalStorageEnforcement ||
		receipt.DeclaredStorageMB != formalDeclaredStorageMB ||
		receipt.Guard != FormalHostStorageGuard() ||
		receipt.Authority != formalHostSupervisorAuthority ||
		receipt.Stage != stage || receipt.ObservedAt.IsZero() || !receipt.Passed || len(receipt.Filesystems) == 0 {
		return errors.New("host storage admission receipt is incomplete, non-formal, or not issued by the authenticated external supervisor")
	}

	roleCounts := map[string]int{}
	volumeIdentities := map[string]struct{}{}
	warning := false
	var previousObservedAt time.Time
	var previousOffset int64
	for index, filesystem := range receipt.Filesystems {
		if filesystem.Group != index || filesystem.BlockSizeBytes == 0 || filesystem.TotalBytes == 0 ||
			filesystem.FilesystemType != formalHostFilesystemType || !hex64Pattern.MatchString(filesystem.VolumeIdentitySHA256) ||
			filesystem.DeviceRoleCount != len(filesystem.Roles) || filesystem.ObservedAt.IsZero() || filesystem.ObservedAt.After(receipt.ObservedAt) ||
			filesystem.MonotonicOffsetMS < 0 || filesystem.AvailableBytes > filesystem.TotalBytes || filesystem.UsedBytes > filesystem.TotalBytes ||
			filesystem.AvailableBytes+filesystem.UsedBytes < filesystem.AvailableBytes || filesystem.AvailableBytes+filesystem.UsedBytes > filesystem.TotalBytes ||
			filesystem.AvailableBytes < minimum || len(filesystem.Roles) == 0 || !slices.IsSorted(filesystem.Roles) {
			return errors.New("host storage admission filesystem metrics are invalid")
		}
		if index > 0 && (filesystem.ObservedAt.Before(previousObservedAt) || filesystem.MonotonicOffsetMS < previousOffset) {
			return errors.New("host storage admission observations are not canonical")
		}
		if _, duplicate := volumeIdentities[filesystem.VolumeIdentitySHA256]; duplicate {
			return errors.New("host storage admission repeats a volume in multiple groups")
		}
		volumeIdentities[filesystem.VolumeIdentitySHA256] = struct{}{}
		if filesystem.AvailableBytes < resources.HostStorageGuard.RuntimeWarningBelowAvailableBytes {
			warning = true
		}
		for _, role := range filesystem.Roles {
			roleCounts[role]++
		}
		previousObservedAt = filesystem.ObservedAt
		previousOffset = filesystem.MonotonicOffsetMS
	}
	if !exactStorageRoles(roleCounts, formalHostStorageRoles) {
		return errors.New("host storage admission does not cover each formal role exactly once")
	}
	if receipt.Warning != warning {
		return errors.New("host storage admission warning state is inconsistent")
	}
	return nil
}

// ValidateStorageResourceEvidence independently hashes, strictly decodes, and
// validates the archived host receipt for one physical trial. The host receipt
// is accepted only from the authenticated macOS outer supervisor; Pier or a
// guest-side Statfs observation cannot stand in for it.
func ValidateStorageResourceEvidence(artifactDir string, evidence StorageResourceEvidence, admission StorageAdmissionReceipt, resources ResourceSpec) error {
	if err := validateFormalHostResources(resources); err != nil {
		return err
	}
	if evidence.SchemaVersion != StorageEvidenceSchemaVersion {
		return errors.New("host storage resource evidence schema is invalid")
	}
	if evidence.ReceiptRelativePath != StorageReceiptRelativePath {
		return errors.New("host storage resource evidence archive path is invalid")
	}
	if !hex64Pattern.MatchString(evidence.ReceiptSHA256) {
		return errors.New("host storage resource evidence digest is invalid")
	}
	path, err := validatedArchivePath(artifactDir, evidence.ReceiptRelativePath)
	if err != nil {
		return fmt.Errorf("host storage resource receipt path: %w", err)
	}
	var receipt StorageResourceReceipt
	if err := decodeStrictReceipt(path, evidence.ReceiptSHA256, &receipt); err != nil {
		return fmt.Errorf("decode host storage resource receipt: %w", err)
	}
	if !reflect.DeepEqual(receipt, evidence.Receipt) {
		return errors.New("embedded host storage resource receipt differs from archived bytes")
	}
	if err := validateHostRuntimeReceipt(receipt, admission, resources); err != nil {
		return err
	}
	return nil
}

func validateHostRuntimeReceipt(receipt StorageResourceReceipt, admission StorageAdmissionReceipt, resources ResourceSpec) error {
	if StorageStatfsAuthority != formalHostSupervisorAuthority ||
		receipt.SchemaVersion != StorageReceiptSchemaVersion || receipt.Enforcement != FormalStorageEnforcement ||
		receipt.DeclaredStorageMB != formalDeclaredStorageMB || receipt.Guard != FormalHostStorageGuard() ||
		receipt.Authority != formalHostSupervisorAuthority || receipt.Status != StorageStatusCompletedAboveGuard ||
		receipt.StartedAt.IsZero() || !receipt.FinishedAt.After(receipt.StartedAt) ||
		receipt.ProviderWALStartedAt.Before(receipt.StartedAt) || receipt.ProviderWALStartedAt.After(receipt.FinishedAt) ||
		receipt.ProviderWALStartedDeltaMS != receipt.ProviderWALStartedAt.Sub(receipt.StartedAt).Milliseconds() ||
		receipt.FinishedDeltaMS != receipt.FinishedAt.Sub(receipt.StartedAt).Milliseconds() || receipt.FinishedDeltaMS <= 0 ||
		!reflect.DeepEqual(receipt.Admission, admission) || admission.ObservedAt.After(receipt.StartedAt) || len(receipt.Filesystems) == 0 {
		return errors.New("host storage resource receipt contract is invalid")
	}
	if err := ValidateStorageAdmissionReceipt(receipt.Admission, resources); err != nil {
		return err
	}

	admissionByGroup := make(map[int]StorageAdmissionFilesystemReceipt, len(admission.Filesystems))
	for _, filesystem := range admission.Filesystems {
		admissionByGroup[filesystem.Group] = filesystem
	}
	roleCounts := map[string]int{}
	for index, filesystem := range receipt.Filesystems {
		if filesystem.Group != index || filesystem.BlockSizeBytes == 0 || filesystem.TotalBytes == 0 ||
			filesystem.FilesystemType != formalHostFilesystemType || !hex64Pattern.MatchString(filesystem.VolumeIdentitySHA256) ||
			filesystem.DeviceRoleCount != len(filesystem.Roles) || filesystem.Samples < 2 || filesystem.Samples != uint64(len(filesystem.SamplePoints)) ||
			len(filesystem.Roles) == 0 || !slices.IsSorted(filesystem.Roles) ||
			filesystem.AvailableBeforeBytes > filesystem.TotalBytes || filesystem.AvailableAfterBytes > filesystem.TotalBytes || filesystem.MinimumAvailableBytes > filesystem.TotalBytes ||
			filesystem.UsedBeforeBytes > filesystem.TotalBytes || filesystem.UsedAfterBytes > filesystem.TotalBytes || filesystem.MaximumUsedBytes > filesystem.TotalBytes ||
			filesystem.MinimumAvailableBytes > filesystem.AvailableBeforeBytes || filesystem.MinimumAvailableBytes > filesystem.AvailableAfterBytes ||
			filesystem.MaximumUsedBytes < filesystem.UsedBeforeBytes || filesystem.MaximumUsedBytes < filesystem.UsedAfterBytes ||
			filesystem.MinimumAvailableBytes < resources.HostStorageGuard.RuntimeHardFloorAvailableBytes ||
			filesystem.MaximumCompletionGapMS > int64(resources.HostStorageGuard.MonitoringGapThresholdMS) {
			return errors.New("host storage resource filesystem metrics are invalid")
		}
		before, exists := admissionByGroup[filesystem.Group]
		if !exists || before.BlockSizeBytes != filesystem.BlockSizeBytes || before.TotalBytes != filesystem.TotalBytes ||
			before.FilesystemType != filesystem.FilesystemType || before.VolumeIdentitySHA256 != filesystem.VolumeIdentitySHA256 ||
			before.DeviceRoleCount != filesystem.DeviceRoleCount || !slices.Equal(before.Roles, filesystem.Roles) {
			return errors.New("host storage runtime receipt does not preserve its pinned admission identity")
		}
		if err := validateHostStorageSamples(filesystem, receipt.StartedAt, receipt.FinishedAt, receipt.FinishedDeltaMS, resources.HostStorageGuard); err != nil {
			return err
		}
		for _, role := range filesystem.Roles {
			roleCounts[role]++
		}
	}
	if len(receipt.Filesystems) != len(admissionByGroup) || !exactStorageRoles(roleCounts, formalHostStorageRoles) {
		return errors.New("host storage resource receipt does not cover each admitted filesystem and formal role exactly once")
	}
	return nil
}

func validateHostStorageSamples(filesystem StorageRuntimeFilesystemReceipt, startedAt, finishedAt time.Time, finishedDeltaMS int64, guard HostStorageGuardSpec) error {
	minimum := ^uint64(0)
	maximumUsed := uint64(0)
	warnings := uint64(0)
	maximumGap := int64(0)
	previousEnd := int64(0)
	var previousWall time.Time
	for index, sample := range filesystem.SamplePoints {
		if err := validateStorageSample(sample, filesystem.TotalBytes, startedAt, finishedAt, finishedDeltaMS, previousEnd, previousWall); err != nil {
			return fmt.Errorf("host storage sample %d: %w", index, err)
		}
		gap := sample.EndDeltaMS - previousEnd
		if gap > int64(guard.MonitoringGapThresholdMS) {
			return errors.New("host storage resource monitoring completion gap exceeds the formal threshold")
		}
		maximumGap = max(maximumGap, gap)
		minimum = min(minimum, sample.AvailableBytes)
		maximumUsed = max(maximumUsed, sample.UsedBytes)
		if sample.AvailableBytes < guard.RuntimeWarningBelowAvailableBytes {
			warnings++
		}
		previousEnd = sample.EndDeltaMS
		previousWall = sample.ObservedAt
	}
	lastBoundaryGap := finishedDeltaMS - previousEnd
	if lastBoundaryGap < 0 || lastBoundaryGap > int64(guard.MonitoringGapThresholdMS) {
		return errors.New("host storage receipt has an invalid final monitoring boundary gap")
	}
	maximumGap = max(maximumGap, lastBoundaryGap)
	first, last := filesystem.SamplePoints[0], filesystem.SamplePoints[len(filesystem.SamplePoints)-1]
	if filesystem.AvailableBeforeBytes != first.AvailableBytes || filesystem.UsedBeforeBytes != first.UsedBytes ||
		filesystem.AvailableAfterBytes != last.AvailableBytes || filesystem.UsedAfterBytes != last.UsedBytes ||
		filesystem.MinimumAvailableBytes != minimum || filesystem.MaximumUsedBytes != maximumUsed ||
		filesystem.WarningSamples != warnings || filesystem.MaximumCompletionGapMS != maximumGap {
		return errors.New("host storage resource sample aggregates are inconsistent")
	}
	return nil
}

// ValidateGuestStorageResourceEvidence validates exactly two independently
// archived phase receipts in canonical agent/verifier order. Both are produced
// inside the 64 GiB guest using pinned descriptors for guest_root and
// guest_app; host bind mounts such as /logs are deliberately outside the role
// catalog.
func ValidateGuestStorageResourceEvidence(artifactDir string, evidence []GuestStorageResourceEvidence, resources ResourceSpec) error {
	if err := validateFormalGuestResources(resources); err != nil {
		return err
	}
	if len(evidence) != len(formalGuestPhases) {
		return errors.New("guest storage evidence must contain exactly the agent and verifier phase receipts")
	}
	var sessionIdentity string
	var agentFinishedAt time.Time
	var providerWALStartedAt time.Time
	for index, phase := range formalGuestPhases {
		receipt, err := validateGuestStoragePhaseEvidence(artifactDir, evidence[index], phase, resources)
		if err != nil {
			return err
		}
		if index == 0 {
			sessionIdentity = receipt.SessionIdentitySHA256
			agentFinishedAt = receipt.FinishedAt
			providerWALStartedAt = receipt.ProviderWALStartedAt
			continue
		}
		if receipt.SessionIdentitySHA256 != sessionIdentity {
			return errors.New("guest storage phase receipts do not share a trial session identity")
		}
		if receipt.StartedAt.Before(agentFinishedAt) {
			return errors.New("guest verifier storage monitoring overlaps or precedes the agent phase")
		}
		if !receipt.ProviderWALStartedAt.Equal(providerWALStartedAt) {
			return errors.New("guest storage phase receipts do not bind the same provider WAL boundary")
		}
	}
	return nil
}

func validateGuestStoragePhaseEvidence(artifactDir string, evidence GuestStorageResourceEvidence, phase string, resources ResourceSpec) (GuestStorageResourceReceipt, error) {
	expectedPath := formalGuestReceiptPaths[phase]
	if evidence.SchemaVersion != GuestStorageEvidenceSchemaVersion {
		return GuestStorageResourceReceipt{}, fmt.Errorf("guest %s storage evidence schema is invalid", phase)
	}
	if evidence.ReceiptRelativePath != expectedPath {
		return GuestStorageResourceReceipt{}, fmt.Errorf("guest %s storage evidence archive path is invalid", phase)
	}
	if !hex64Pattern.MatchString(evidence.ReceiptSHA256) {
		return GuestStorageResourceReceipt{}, fmt.Errorf("guest %s storage evidence digest is invalid", phase)
	}
	path, err := validatedArchivePath(artifactDir, evidence.ReceiptRelativePath)
	if err != nil {
		return GuestStorageResourceReceipt{}, fmt.Errorf("guest %s storage receipt path: %w", phase, err)
	}
	var receipt GuestStorageResourceReceipt
	if err := decodeStrictReceipt(path, evidence.ReceiptSHA256, &receipt); err != nil {
		return GuestStorageResourceReceipt{}, fmt.Errorf("decode guest %s storage receipt: %w", phase, err)
	}
	if !reflect.DeepEqual(receipt, evidence.Receipt) {
		return GuestStorageResourceReceipt{}, fmt.Errorf("embedded guest %s storage receipt differs from archived bytes", phase)
	}
	if err := validateGuestStoragePhaseReceipt(receipt, phase, resources); err != nil {
		return GuestStorageResourceReceipt{}, err
	}
	return receipt, nil
}

func validateGuestStoragePhaseReceipt(receipt GuestStorageResourceReceipt, phase string, resources ResourceSpec) error {
	if GuestStorageStatfsAuthority != formalGuestSupervisorAuthority ||
		receipt.SchemaVersion != GuestStorageReceiptSchemaVersion || receipt.Phase != phase ||
		!hex64Pattern.MatchString(receipt.SessionIdentitySHA256) || !hex64Pattern.MatchString(receipt.ContainerIdentitySHA256) ||
		receipt.Enforcement != FormalStorageEnforcement || receipt.DeclaredStorageMB != formalDeclaredStorageMB ||
		receipt.ConfiguredCapacityBytes != FormalGuestStorageConfiguredBytes ||
		receipt.Guard != FormalGuestStorageGuard() || receipt.Authority != formalGuestSupervisorAuthority ||
		receipt.Status != StorageStatusCompletedAboveGuard || receipt.StartedAt.IsZero() || !receipt.FinishedAt.After(receipt.StartedAt) ||
		receipt.ProviderWALStartedAt.IsZero() || receipt.ProviderWALStartedAt.After(receipt.FinishedAt) ||
		receipt.ProviderWALStartedDeltaMS != receipt.ProviderWALStartedAt.Sub(receipt.StartedAt).Milliseconds() ||
		receipt.FinishedDeltaMS != receipt.FinishedAt.Sub(receipt.StartedAt).Milliseconds() || receipt.FinishedDeltaMS <= 0 ||
		len(receipt.Filesystems) == 0 {
		return fmt.Errorf("guest %s storage receipt contract is invalid", phase)
	}
	if phase == GuestStoragePhaseAgent && receipt.ProviderWALStartedAt.Before(receipt.StartedAt) {
		return errors.New("guest agent storage receipt claims provider WAL before the monitored phase")
	}
	if phase == GuestStoragePhaseVerifier && receipt.ProviderWALStartedAt.After(receipt.StartedAt) {
		return errors.New("guest verifier storage receipt must begin after provider WAL is durable")
	}

	roleCounts := map[string]int{}
	volumeIdentities := map[string]struct{}{}
	for index, filesystem := range receipt.Filesystems {
		if filesystem.Group != index || len(filesystem.Roles) == 0 || !slices.IsSorted(filesystem.Roles) ||
			!hex64Pattern.MatchString(filesystem.VolumeIdentitySHA256) || filesystem.FilesystemType == "" ||
			filesystem.DeviceRoleCount != len(filesystem.Roles) || filesystem.BlockSizeBytes == 0 || filesystem.TotalBytes == 0 ||
			filesystem.MinimumAvailableBytes > filesystem.TotalBytes || filesystem.MaximumUsedBytes > filesystem.TotalBytes ||
			filesystem.MinimumAvailableBytes < resources.GuestStorageGuard.RuntimeAbortBelowAvailableBytes || len(filesystem.Samples) < 2 ||
			filesystem.MaximumCompletionGapMS > int64(resources.GuestStorageGuard.MonitoringGapThresholdMS) {
			return fmt.Errorf("guest %s storage filesystem metrics are invalid", phase)
		}
		if _, duplicate := volumeIdentities[filesystem.VolumeIdentitySHA256]; duplicate {
			return fmt.Errorf("guest %s storage receipt repeats a volume in multiple groups", phase)
		}
		volumeIdentities[filesystem.VolumeIdentitySHA256] = struct{}{}
		if err := validateGuestStorageSamples(filesystem, receipt.StartedAt, receipt.FinishedAt, receipt.FinishedDeltaMS, resources.GuestStorageGuard); err != nil {
			return fmt.Errorf("guest %s: %w", phase, err)
		}
		for _, role := range filesystem.Roles {
			roleCounts[role]++
		}
	}
	if !exactStorageRoles(roleCounts, formalGuestStorageRoles) {
		return fmt.Errorf("guest %s storage receipt does not cover each formal guest role exactly once", phase)
	}
	return nil
}

func validateGuestStorageSamples(filesystem GuestStorageFilesystemReceipt, startedAt, finishedAt time.Time, finishedDeltaMS int64, guard GuestStorageGuardSpec) error {
	minimum := ^uint64(0)
	maximumUsed := uint64(0)
	maximumGap := int64(0)
	previousEnd := int64(0)
	var previousWall time.Time
	for index, sample := range filesystem.Samples {
		if err := validateStorageSample(sample, filesystem.TotalBytes, startedAt, finishedAt, finishedDeltaMS, previousEnd, previousWall); err != nil {
			return fmt.Errorf("storage sample %d: %w", index, err)
		}
		if index == 0 && sample.AvailableBytes < guard.StartMinimumAvailableBytes {
			return errors.New("storage phase started below the formal 28 GiB admission floor")
		}
		if sample.AvailableBytes < guard.RuntimeAbortBelowAvailableBytes {
			return errors.New("storage phase crossed the formal 8 GiB runtime abort floor")
		}
		gap := sample.EndDeltaMS - previousEnd
		if gap > int64(guard.MonitoringGapThresholdMS) {
			return errors.New("storage monitoring completion gap exceeds the formal threshold")
		}
		maximumGap = max(maximumGap, gap)
		minimum = min(minimum, sample.AvailableBytes)
		maximumUsed = max(maximumUsed, sample.UsedBytes)
		previousEnd = sample.EndDeltaMS
		previousWall = sample.ObservedAt
	}
	lastBoundaryGap := finishedDeltaMS - previousEnd
	if lastBoundaryGap < 0 || lastBoundaryGap > int64(guard.MonitoringGapThresholdMS) {
		return errors.New("storage receipt has an invalid final monitoring boundary gap")
	}
	maximumGap = max(maximumGap, lastBoundaryGap)
	if filesystem.MinimumAvailableBytes != minimum || filesystem.MaximumUsedBytes != maximumUsed || filesystem.MaximumCompletionGapMS != maximumGap {
		return errors.New("storage sample aggregates are inconsistent")
	}
	return nil
}

func validateStorageSample(sample StorageStatfsSample, totalBytes uint64, startedAt, finishedAt time.Time, finishedDeltaMS, previousEnd int64, previousWall time.Time) error {
	if sample.ObservedAt.IsZero() || sample.ObservedAt.Before(startedAt) || sample.ObservedAt.After(finishedAt) ||
		sample.StartDeltaMS < previousEnd || sample.EndDeltaMS < sample.StartDeltaMS || sample.EndDeltaMS > finishedDeltaMS ||
		(!previousWall.IsZero() && sample.ObservedAt.Before(previousWall)) ||
		sample.AvailableBytes > totalBytes || sample.UsedBytes > totalBytes ||
		sample.AvailableBytes+sample.UsedBytes < sample.AvailableBytes || sample.AvailableBytes+sample.UsedBytes > totalBytes {
		return errors.New("sample timing, clock order, or capacity is invalid")
	}
	return nil
}

func validateFormalHostResources(resources ResourceSpec) error {
	if resources.StorageMB != formalDeclaredStorageMB || resources.HostStorageGuard != FormalHostStorageGuard() {
		return errors.New("host storage validator only accepts the preregistered formal guard; development or pilot guards are forbidden")
	}
	return nil
}

func validateFormalGuestResources(resources ResourceSpec) error {
	if resources.StorageMB != formalDeclaredStorageMB || resources.GuestStorageGuard != FormalGuestStorageGuard() {
		return errors.New("guest storage validator only accepts the preregistered formal guard; development or pilot guards are forbidden")
	}
	return nil
}

func validatedArchivePath(artifactDir, relativePath string) (string, error) {
	path := filepath.Join(artifactDir, filepath.FromSlash(relativePath))
	if err := requirePathWithin(artifactDir, path); err != nil {
		return "", err
	}
	artifactAbsolute, err := filepath.Abs(artifactDir)
	if err != nil {
		return "", err
	}
	pathAbsolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	artifactResolved, err := filepath.EvalSymlinks(artifactAbsolute)
	if err != nil {
		return "", err
	}
	pathResolved, err := filepath.EvalSymlinks(pathAbsolute)
	if err != nil {
		return "", err
	}
	relativeToArtifact, err := filepath.Rel(artifactAbsolute, pathAbsolute)
	if err != nil {
		return "", err
	}
	expectedResolved := filepath.Join(artifactResolved, relativeToArtifact)
	if filepath.Clean(pathResolved) != filepath.Clean(expectedResolved) {
		return "", errors.New("receipt archive traverses an internal symbolic link")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("receipt archive is not a regular file")
	}
	return path, nil
}

func decodeStrictReceipt(path, expectedSHA256 string, destination any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("receipt archive descriptor is not a regular file")
	}
	raw, err := io.ReadAll(file)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(raw)
	if fmt.Sprintf("%x", digest) != expectedSHA256 {
		return errors.New("receipt archive hash mismatch")
	}
	if err := rejectDuplicateJSONKeys(raw); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailer any
	if err := decoder.Decode(&trailer); !errors.Is(err, io.EOF) {
		return errors.New("receipt archive has trailing JSON")
	}
	return nil
}

func rejectDuplicateJSONKeys(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := scanStrictJSONValue(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("receipt archive has trailing JSON")
		}
		return err
	}
	return nil
}

func scanStrictJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]struct{}{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("receipt archive object key is not a string")
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("receipt archive contains duplicate JSON key %q", key)
			}
			seen[key] = struct{}{}
			if err := scanStrictJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim('}') {
			return errors.New("receipt archive object is not closed")
		}
	case '[':
		for decoder.More() {
			if err := scanStrictJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim(']') {
			return errors.New("receipt archive array is not closed")
		}
	default:
		return errors.New("receipt archive starts with an unexpected closing delimiter")
	}
	return nil
}

func exactStorageRoles(counts map[string]int, expected []string) bool {
	if len(counts) != len(expected) {
		return false
	}
	for _, role := range expected {
		if counts[role] != 1 {
			return false
		}
	}
	return true
}
