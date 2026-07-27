package pierbackend

import (
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

const sha256HexLength = 64

type pinnedHostFilesystem interface {
	identity() string
	sample() (hostFilesystemSample, error)
	close() error
}

type hostFilesystemProbe interface {
	pin(path string) (pinnedHostFilesystem, error)
}

type systemHostFilesystemProbe struct{}

func (systemHostFilesystemProbe) pin(path string) (pinnedHostFilesystem, error) {
	return pinHostFilesystem(path)
}

// pinnedStorageFilesystem holds one descriptor per unique host filesystem.
// Roles are the only location metadata that may enter a public receipt; the
// descriptor identity is private controller state used to detect drift.
type pinnedStorageFilesystem struct {
	roles  []storagePathRole
	handle pinnedHostFilesystem
	start  hostFilesystemSample
}

func pinStorageTargets(probe hostFilesystemProbe, targets []storagePathTarget) ([]pinnedStorageFilesystem, error) {
	if probe == nil {
		return nil, errors.New("host filesystem probe is required")
	}
	if err := validateStorageTargets(targets); err != nil {
		return nil, err
	}
	byIdentity := make(map[string]*pinnedStorageFilesystem, len(targets))
	opened := make([]pinnedHostFilesystem, 0, len(targets))
	closeOpened := func() error {
		var joined error
		for _, handle := range opened {
			joined = errors.Join(joined, handle.close())
		}
		return joined
	}
	for _, target := range targets {
		handle, err := probe.pin(target.path)
		if err != nil {
			return nil, errors.Join(fmt.Errorf("pin host filesystem for %s: %w", target.role, err), closeOpened())
		}
		opened = append(opened, handle)
		identity := handle.identity()
		if identity == "" {
			return nil, errors.Join(fmt.Errorf("pinned host filesystem for %s lacks an identity", target.role), closeOpened())
		}
		sample, err := handle.sample()
		if err != nil {
			return nil, errors.Join(fmt.Errorf("sample pinned host filesystem for %s: %w", target.role, err), closeOpened())
		}
		if err := validateHostFilesystemSample(sample, target.role); err != nil {
			return nil, errors.Join(err, closeOpened())
		}
		if sample.identity != identity {
			return nil, errors.Join(fmt.Errorf("pinned host filesystem identity changed for %s", target.role), closeOpened())
		}
		if existing := byIdentity[identity]; existing != nil {
			if !sameHostFilesystemIdentity(existing.start, sample) {
				return nil, errors.Join(fmt.Errorf("pinned host filesystem metadata changed while grouping %s", target.role), closeOpened())
			}
			// The three descriptors are opened sequentially; unrelated host IO may
			// move dynamic counters between reads. Preserve conservative extrema.
			existing.start.availableBytes = min(existing.start.availableBytes, sample.availableBytes)
			existing.start.usedBytes = max(existing.start.usedBytes, sample.usedBytes)
			existing.roles = append(existing.roles, target.role)
			if err := handle.close(); err != nil {
				return nil, errors.Join(fmt.Errorf("close duplicate host filesystem handle for %s: %w", target.role, err), closeOpened())
			}
			opened = opened[:len(opened)-1]
			continue
		}
		byIdentity[identity] = &pinnedStorageFilesystem{
			roles: []storagePathRole{target.role}, handle: handle, start: sample,
		}
	}
	result := make([]pinnedStorageFilesystem, 0, len(byIdentity))
	for _, filesystem := range byIdentity {
		slices.SortFunc(filesystem.roles, func(left, right storagePathRole) int {
			return storageRoleRank(left) - storageRoleRank(right)
		})
		result = append(result, *filesystem)
	}
	slices.SortFunc(result, func(left, right pinnedStorageFilesystem) int {
		leftRank, rightRank := storageRoleRank(left.roles[0]), storageRoleRank(right.roles[0])
		if leftRank != rightRank {
			return leftRank - rightRank
		}
		return strings.Compare(storageRoleKey(left.roles), storageRoleKey(right.roles))
	})
	return result, nil
}

func validateStorageTargets(targets []storagePathTarget) error {
	if len(targets) != 3 {
		return errors.New("storage observation requires Docker data, private work, and artifact roots")
	}
	wantedRoles := map[storagePathRole]bool{
		storageRoleArtifact:    false,
		storageRoleController:  false,
		storageRolePrivateWork: false,
	}
	for _, target := range targets {
		seen, known := wantedRoles[target.role]
		if !known || seen {
			return fmt.Errorf("storage observation has an invalid or duplicate role %q", target.role)
		}
		wantedRoles[target.role] = true
		if !filepath.IsAbs(target.path) || filepath.Clean(target.path) != target.path {
			return fmt.Errorf("storage observation role %s lacks a clean absolute controller path", target.role)
		}
	}
	return nil
}

func validateHostFilesystemSample(sample hostFilesystemSample, role storagePathRole) error {
	if sample.identity == "" || len(sample.volumeIdentitySHA256) != sha256HexLength || sample.filesystemType == "" ||
		sample.blockSizeBytes == 0 || sample.totalBytes == 0 || sample.availableBytes > sample.totalBytes || sample.usedBytes > sample.totalBytes {
		return fmt.Errorf("host filesystem sample for %s is incomplete", role)
	}
	if _, err := hex.DecodeString(sample.volumeIdentitySHA256); err != nil {
		return fmt.Errorf("host filesystem sample for %s has an invalid public identity", role)
	}
	if sample.volumeIdentitySHA256 != hashHostFilesystemIdentity(sample.identity) {
		return fmt.Errorf("host filesystem sample for %s has an inconsistent public identity", role)
	}
	return nil
}

func closePinnedStorageFilesystems(filesystems []pinnedStorageFilesystem) error {
	var joined error
	for index := range filesystems {
		if filesystems[index].handle == nil {
			continue
		}
		joined = errors.Join(joined, filesystems[index].handle.close())
		filesystems[index].handle = nil
	}
	return joined
}
