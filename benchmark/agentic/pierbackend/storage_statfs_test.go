package pierbackend

import (
	"errors"
	"math"
	"reflect"
	"testing"
)

type fixtureHostFilesystemSampler struct {
	samples map[string]hostFilesystemSample
	errors  map[string]error
}

func (fixture fixtureHostFilesystemSampler) sample(path string) (hostFilesystemSample, error) {
	if err := fixture.errors[path]; err != nil {
		return hostFilesystemSample{}, err
	}
	sample, ok := fixture.samples[path]
	if !ok {
		return hostFilesystemSample{}, errors.New("unexpected fixture path")
	}
	return sample, nil
}

func fixtureStorageSample(identity string, blockSize, total, available, used uint64) hostFilesystemSample {
	return hostFilesystemSample{
		identity: identity, volumeIdentitySHA256: hashHostFilesystemIdentity(identity), filesystemType: "fixturefs",
		blockSizeBytes: blockSize, totalBytes: total, availableBytes: available, usedBytes: used,
	}
}

func TestSampleStorageTargetsDeduplicatesWithoutPublishingAuthority(t *testing.T) {
	private := fixtureStorageSample("private-device-7", 4096, 100, 40, 50)
	artifacts := fixtureStorageSample("private-device-7", 4096, 100, 39, 51)
	docker := fixtureStorageSample("private-device-3", 4096, 200, 80, 100)
	groups, err := sampleStorageTargets(fixtureHostFilesystemSampler{samples: map[string]hostFilesystemSample{
		"/docker": docker, "/private": private, "/artifacts": artifacts,
	}}, []storagePathTarget{
		{role: storageRolePrivateWork, path: "/private"},
		{role: storageRoleController, path: "/docker"},
		{role: storageRoleArtifact, path: "/artifacts"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 {
		t.Fatalf("group count = %d, want 2", len(groups))
	}
	if got := groups[1].roles; !reflect.DeepEqual(got, []storagePathRole{storageRoleController}) {
		t.Fatalf("first roles = %#v", got)
	}
	if got := groups[0].roles; !reflect.DeepEqual(got, []storagePathRole{storageRoleArtifact, storageRolePrivateWork}) {
		t.Fatalf("second roles = %#v", got)
	}
	if groups[0].sample.availableBytes != 39 || groups[0].sample.usedBytes != 51 {
		t.Fatalf("dynamic counters were not conservatively merged: %#v", groups[0].sample)
	}
	// The identity is retained only in the private group join key; the only
	// location-bearing public projection is the fixed role vocabulary.
	for _, group := range groups {
		if storageRoleKey(group.roles) == "" {
			t.Fatal("group lacks public roles")
		}
	}
}

func TestSampleStorageTargetsFailsClosed(t *testing.T) {
	valid := fixtureStorageSample("fs", 4096, 100, 40, 50)
	tests := []struct {
		name    string
		targets []storagePathTarget
		fixture fixtureHostFilesystemSampler
	}{
		{
			name:    "missing role",
			targets: []storagePathTarget{{storageRoleController, "/docker"}, {storageRolePrivateWork, "/private"}},
			fixture: fixtureHostFilesystemSampler{samples: map[string]hostFilesystemSample{"/docker": valid, "/private": valid}},
		},
		{
			name:    "duplicate role",
			targets: []storagePathTarget{{storageRoleController, "/docker"}, {storageRoleController, "/other"}, {storageRoleArtifact, "/artifacts"}},
			fixture: fixtureHostFilesystemSampler{samples: map[string]hostFilesystemSample{"/docker": valid, "/other": valid, "/artifacts": valid}},
		},
		{
			name:    "relative authority",
			targets: []storagePathTarget{{storageRoleController, "docker"}, {storageRolePrivateWork, "/private"}, {storageRoleArtifact, "/artifacts"}},
			fixture: fixtureHostFilesystemSampler{samples: map[string]hostFilesystemSample{"docker": valid, "/private": valid, "/artifacts": valid}},
		},
		{
			name:    "statfs failure",
			targets: []storagePathTarget{{storageRoleController, "/docker"}, {storageRolePrivateWork, "/private"}, {storageRoleArtifact, "/artifacts"}},
			fixture: fixtureHostFilesystemSampler{
				samples: map[string]hostFilesystemSample{"/docker": valid, "/private": valid, "/artifacts": valid},
				errors:  map[string]error{"/docker": errors.New("fixture statfs failure")},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := sampleStorageTargets(test.fixture, test.targets); err == nil {
				t.Fatal("sampleStorageTargets unexpectedly succeeded")
			}
		})
	}
}

func TestCheckedFilesystemBytesRejectsOverflow(t *testing.T) {
	if got, err := checkedFilesystemBytes(3, 4096); err != nil || got != 12288 {
		t.Fatalf("checkedFilesystemBytes = %d, %v", got, err)
	}
	if _, err := checkedFilesystemBytes(math.MaxUint64, 2); err == nil {
		t.Fatal("overflow was accepted")
	}
}

type fixturePinnedHostFilesystem struct {
	id         string
	sampleNow  hostFilesystemSample
	sampleErr  error
	closeCount *int
}

func (fixture *fixturePinnedHostFilesystem) identity() string { return fixture.id }

func (fixture *fixturePinnedHostFilesystem) sample() (hostFilesystemSample, error) {
	return fixture.sampleNow, fixture.sampleErr
}

func (fixture *fixturePinnedHostFilesystem) close() error {
	*fixture.closeCount++
	return nil
}

type fixtureHostFilesystemProbe struct {
	handles map[string]*fixturePinnedHostFilesystem
}

func (fixture fixtureHostFilesystemProbe) pin(path string) (pinnedHostFilesystem, error) {
	handle := fixture.handles[path]
	if handle == nil {
		return nil, errors.New("fixture pin failure")
	}
	return handle, nil
}

func TestPinStorageTargetsPinsOneHandlePerUniqueFilesystem(t *testing.T) {
	dockerCloses, privateCloses, artifactCloses := 0, 0, 0
	dockerSample := fixtureStorageSample("dev-1:fsid-0", 4096, 200, 80, 100)
	privateSample := fixtureStorageSample("dev-2:fsid-0", 4096, 100, 40, 50)
	artifactSample := privateSample
	artifactSample.availableBytes--
	artifactSample.usedBytes++
	groups, err := pinStorageTargets(fixtureHostFilesystemProbe{handles: map[string]*fixturePinnedHostFilesystem{
		"/docker":    {id: dockerSample.identity, sampleNow: dockerSample, closeCount: &dockerCloses},
		"/private":   {id: privateSample.identity, sampleNow: privateSample, closeCount: &privateCloses},
		"/artifacts": {id: artifactSample.identity, sampleNow: artifactSample, closeCount: &artifactCloses},
	}}, []storagePathTarget{
		{role: storageRoleController, path: "/docker"},
		{role: storageRolePrivateWork, path: "/private"},
		{role: storageRoleArtifact, path: "/artifacts"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 || artifactCloses != 1 || dockerCloses != 0 || privateCloses != 0 {
		t.Fatalf("pin result groups=%d closes=%d/%d/%d", len(groups), dockerCloses, privateCloses, artifactCloses)
	}
	if got := groups[0].roles; !reflect.DeepEqual(got, []storagePathRole{storageRoleArtifact, storageRolePrivateWork}) {
		t.Fatalf("deduplicated roles = %#v", got)
	}
	if groups[0].start.availableBytes != 39 || groups[0].start.usedBytes != 51 {
		t.Fatalf("deduplicated conservative sample = %#v", groups[0].start)
	}
	if err := closePinnedStorageFilesystems(groups); err != nil {
		t.Fatal(err)
	}
	if dockerCloses != 1 || privateCloses != 1 || artifactCloses != 1 {
		t.Fatalf("terminal closes=%d/%d/%d", dockerCloses, privateCloses, artifactCloses)
	}
	if err := closePinnedStorageFilesystems(groups); err != nil {
		t.Fatal(err)
	}
	if dockerCloses != 1 || privateCloses != 1 || artifactCloses != 1 {
		t.Fatal("terminal close was not idempotent")
	}
}

func TestPinStorageTargetsDoesNotMergeEqualFSIDAcrossDevices(t *testing.T) {
	closes := [3]int{}
	sample := func(identity string) hostFilesystemSample {
		return fixtureStorageSample(identity, 4096, 100, 40, 50)
	}
	groups, err := pinStorageTargets(fixtureHostFilesystemProbe{handles: map[string]*fixturePinnedHostFilesystem{
		"/docker":    {id: "dev-1:fsid-0", sampleNow: sample("dev-1:fsid-0"), closeCount: &closes[0]},
		"/private":   {id: "dev-2:fsid-0", sampleNow: sample("dev-2:fsid-0"), closeCount: &closes[1]},
		"/artifacts": {id: "dev-3:fsid-0", sampleNow: sample("dev-3:fsid-0"), closeCount: &closes[2]},
	}}, []storagePathTarget{
		{role: storageRoleController, path: "/docker"},
		{role: storageRolePrivateWork, path: "/private"},
		{role: storageRoleArtifact, path: "/artifacts"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = closePinnedStorageFilesystems(groups) }()
	if len(groups) != 3 {
		t.Fatalf("filesystem count = %d, want 3", len(groups))
	}
}

func TestSystemHostFilesystemProbePinsDirectoryDescriptor(t *testing.T) {
	handle, err := (systemHostFilesystemProbe{}).pin(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sample, err := handle.sample()
	if err != nil {
		t.Fatal(err)
	}
	if sample.identity == "" || sample.volumeIdentitySHA256 == "" || sample.filesystemType == "" ||
		sample.blockSizeBytes == 0 || sample.totalBytes == 0 || sample.availableBytes == 0 {
		t.Fatalf("system sample is incomplete: %#v", sample)
	}
	if err := handle.close(); err != nil {
		t.Fatal(err)
	}
	if _, err := handle.sample(); err == nil {
		t.Fatal("closed descriptor remained observable")
	}
}
