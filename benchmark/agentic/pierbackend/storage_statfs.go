package pierbackend

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/bits"
	"path/filepath"
	"slices"
	"strings"
)

// storagePathRole is deliberately a closed, content-free vocabulary. Host
// paths and filesystem/device identifiers are controller authority and must
// never be copied into a published benchmark receipt.
type storagePathRole string

const (
	storageRoleArtifact    storagePathRole = "artifact_root"
	storageRoleController  storagePathRole = "controller_root"
	storageRolePrivateWork storagePathRole = "private_work_root"
)

type hostFilesystemSample struct {
	identity             string
	volumeIdentitySHA256 string
	filesystemType       string
	blockSizeBytes       uint64
	totalBytes           uint64
	availableBytes       uint64
	usedBytes            uint64
}

type hostFilesystemIdentity struct {
	privateTuple   string
	publicSHA256   string
	filesystemType string
}

// makeHostFilesystemIdentity produces a canonical, domain-separated identity
// for the exact volume reached through an already-open directory descriptor.
// privateTuple stays inside the controller; only publicSHA256 and the
// non-location-bearing filesystem type may enter a published receipt.
func makeHostFilesystemIdentity(device, filesystemType, fsid0, fsid1 uint64, filesystemTypeName string) hostFilesystemIdentity {
	if filesystemTypeName == "" {
		filesystemTypeName = fmt.Sprintf("0x%016x", filesystemType)
	}
	privateTuple := fmt.Sprintf(
		"agentic-bench/host-volume-identity-v1\ndevice=%016x\nfilesystem_type=%016x\nfilesystem_type_name=%s\nfsid_0=%016x\nfsid_1=%016x",
		device, filesystemType, filesystemTypeName, fsid0, fsid1,
	)
	return hostFilesystemIdentity{
		privateTuple:   privateTuple,
		publicSHA256:   hashHostFilesystemIdentity(privateTuple),
		filesystemType: filesystemTypeName,
	}
}

func hashHostFilesystemIdentity(privateTuple string) string {
	digest := sha256.Sum256([]byte(privateTuple))
	return hex.EncodeToString(digest[:])
}

type hostFilesystemSampler interface {
	sample(path string) (hostFilesystemSample, error)
}

type systemHostFilesystemSampler struct{}

func (systemHostFilesystemSampler) sample(path string) (hostFilesystemSample, error) {
	return sampleHostFilesystem(path)
}

type storagePathTarget struct {
	role storagePathRole
	path string
}

// groupedHostFilesystemSample merges paths that resolve to the same host
// filesystem. identity remains private and is used only to join later samples;
// roles are the sole location metadata admitted to the public receipt.
type groupedHostFilesystemSample struct {
	identity string
	roles    []storagePathRole
	sample   hostFilesystemSample
}

func sampleStorageTargets(sampler hostFilesystemSampler, targets []storagePathTarget) ([]groupedHostFilesystemSample, error) {
	if sampler == nil {
		return nil, errors.New("host filesystem sampler is required")
	}
	if len(targets) != 3 {
		return nil, errors.New("storage observation requires Docker data, private work, and artifact roots")
	}
	wantedRoles := map[storagePathRole]bool{
		storageRoleArtifact:    false,
		storageRoleController:  false,
		storageRolePrivateWork: false,
	}
	byIdentity := make(map[string]*groupedHostFilesystemSample, len(targets))
	for _, target := range targets {
		seen, known := wantedRoles[target.role]
		if !known || seen {
			return nil, fmt.Errorf("storage observation has an invalid or duplicate role %q", target.role)
		}
		wantedRoles[target.role] = true
		if !filepath.IsAbs(target.path) || filepath.Clean(target.path) != target.path {
			return nil, fmt.Errorf("storage observation role %s lacks a clean absolute controller path", target.role)
		}
		sample, err := sampler.sample(target.path)
		if err != nil {
			return nil, fmt.Errorf("observe host filesystem for %s: %w", target.role, err)
		}
		if err := validateHostFilesystemSample(sample, target.role); err != nil {
			return nil, err
		}
		if existing := byIdentity[sample.identity]; existing != nil {
			if !sameHostFilesystemIdentity(existing.sample, sample) {
				return nil, fmt.Errorf("host filesystem identity changed while grouping %s", target.role)
			}
			// Path-based fixture probes can observe the same filesystem a few
			// microseconds apart. Host-wide counters are allowed to move between
			// those reads; retain the conservative extrema instead of requiring an
			// impossible point-in-time equality. The production monitor pins and
			// samples each unique filesystem only once per tick.
			existing.sample.availableBytes = min(existing.sample.availableBytes, sample.availableBytes)
			existing.sample.usedBytes = max(existing.sample.usedBytes, sample.usedBytes)
			existing.roles = append(existing.roles, target.role)
			continue
		}
		copy := sample
		byIdentity[sample.identity] = &groupedHostFilesystemSample{
			identity: sample.identity, roles: []storagePathRole{target.role}, sample: copy,
		}
	}
	result := make([]groupedHostFilesystemSample, 0, len(byIdentity))
	for _, group := range byIdentity {
		slices.SortFunc(group.roles, func(left, right storagePathRole) int {
			return storageRoleRank(left) - storageRoleRank(right)
		})
		result = append(result, *group)
	}
	slices.SortFunc(result, func(left, right groupedHostFilesystemSample) int {
		leftRank, rightRank := storageRoleRank(left.roles[0]), storageRoleRank(right.roles[0])
		if leftRank != rightRank {
			return leftRank - rightRank
		}
		return strings.Compare(storageRoleKey(left.roles), storageRoleKey(right.roles))
	})
	return result, nil
}

func sameHostFilesystemIdentity(left, right hostFilesystemSample) bool {
	return left.identity == right.identity &&
		left.volumeIdentitySHA256 == right.volumeIdentitySHA256 &&
		left.filesystemType == right.filesystemType &&
		left.blockSizeBytes == right.blockSizeBytes &&
		left.totalBytes == right.totalBytes
}

func storageRoleKey(roles []storagePathRole) string {
	values := make([]string, len(roles))
	for index, role := range roles {
		values[index] = string(role)
	}
	return strings.Join(values, ",")
}

func storageRoleRank(role storagePathRole) int {
	switch role {
	case storageRoleArtifact:
		return 0
	case storageRoleController:
		return 1
	case storageRolePrivateWork:
		return 2
	default:
		return 3
	}
}

func checkedFilesystemBytes(blocks, blockSize uint64) (uint64, error) {
	high, low := bits.Mul64(blocks, blockSize)
	if high != 0 {
		return 0, errors.New("host filesystem byte counter overflows uint64")
	}
	return low, nil
}
