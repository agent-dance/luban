package buildinfo

import (
	"errors"
	"runtime/debug"
	"strings"
	"testing"
	"time"
)

const (
	revisionA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	revisionB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

type fakeSource struct {
	info       *debug.BuildInfo
	infoOK     bool
	executable string
	repo       string
	repoErr    error
}

func (s fakeSource) ReadBuildInfo() (*debug.BuildInfo, bool) { return s.info, s.infoOK }
func (s fakeSource) Executable() (string, error)             { return s.executable, nil }
func (s fakeSource) RepositoryRevision(string) (string, error) {
	return s.repo, s.repoErr
}

func TestDetectorPrefersLinkValuesAndCapturesStartOnce(t *testing.T) {
	fixed := time.Date(2026, 7, 18, 9, 10, 11, 12, time.FixedZone("test", 8*60*60))
	calls := 0
	diagnostic := (Detector{
		Source: fakeSource{
			infoOK:     true,
			info:       buildInfo("module-v", revisionB, "false", "2025-01-02T03:04:05Z"),
			executable: "/tmp/luban", repo: revisionA,
		},
		Now: func() time.Time { calls++; return fixed },
		Link: LinkValues{
			Version: "release-v", Revision: revisionA, Dirty: "true", BuildTime: "2026-07-17T01:02:03+08:00",
		},
	}).Capture("/repo")

	if calls != 1 {
		t.Fatalf("Now calls = %d, want 1", calls)
	}
	got := diagnostic.Fingerprint
	if got.Version != "release-v" || got.Revision != revisionA || got.Dirty == nil || !*got.Dirty {
		t.Fatalf("link precedence fingerprint = %#v", got)
	}
	if got.BuildTime == nil || got.BuildTime.Format(time.RFC3339) != "2026-07-16T17:02:03Z" {
		t.Fatalf("build time = %v", got.BuildTime)
	}
	if !got.ProcessStart.Equal(fixed.UTC()) || got.Executable != "/tmp/luban" {
		t.Fatalf("runtime identity = %#v", got)
	}
	if diagnostic.RevisionStatus != RevisionUnknown {
		t.Fatalf("dirty same revision status = %q, want unknown", diagnostic.RevisionStatus)
	}
}

func TestDetectorUsesBuildInfoPerMissingLinkField(t *testing.T) {
	fixed := time.Date(2026, 7, 18, 1, 2, 3, 0, time.UTC)
	diagnostic := (Detector{
		Source: fakeSource{
			infoOK: true, info: buildInfo("module-v", revisionA, "false", "2026-07-17T01:02:03Z"),
			repo: revisionA,
		},
		ProcessStart:    fixed,
		Link:            LinkValues{Revision: revisionA},
		FallbackVersion: "fallback-v",
	}).Capture("/repo")

	got := diagnostic.Fingerprint
	if got.Version != "module-v" || got.Dirty == nil || *got.Dirty || got.BuildTime == nil {
		t.Fatalf("build-info fallback fingerprint = %#v", got)
	}
	if diagnostic.RevisionStatus != RevisionMatch {
		t.Fatalf("status = %q, want match", diagnostic.RevisionStatus)
	}
}

func TestDetectorMarksCurrentExecutableStaleAgainstDifferentHEAD(t *testing.T) {
	diagnostic := (Detector{
		Source: fakeSource{
			infoOK: true, info: buildInfo("module-v", revisionA, "false", "2026-07-17T01:02:03Z"),
			executable: "/tmp/old-luban-binary", repo: revisionB,
		},
		ProcessStart: time.Date(2026, 7, 18, 1, 2, 3, 0, time.UTC),
	}).Capture("/repo")

	if diagnostic.Fingerprint.Executable != "/tmp/old-luban-binary" || diagnostic.Fingerprint.Revision != revisionA {
		t.Fatalf("executable fingerprint = %#v", diagnostic.Fingerprint)
	}
	if diagnostic.RepositoryRevision != revisionB || diagnostic.RevisionStatus != RevisionStale {
		t.Fatalf("executable/HEAD comparison = %#v, want stale", diagnostic)
	}
}

func TestDetectorKeepsMalformedAndMissingValuesUnknown(t *testing.T) {
	diagnostic := (Detector{
		Source: fakeSource{
			infoOK: true,
			info:   buildInfo("(devel)", revisionA, "false", "2026-01-01T00:00:00Z"),
			repo:   revisionA,
		},
		ProcessStart:    time.Unix(123, 0),
		Link:            LinkValues{Revision: "short-sha", Dirty: "yes", BuildTime: "not-time"},
		FallbackVersion: "fallback-v",
	}).Capture("/repo")

	got := diagnostic.Fingerprint
	if got.Version != "fallback-v" || got.Revision != "" || got.Dirty != nil || got.BuildTime != nil {
		t.Fatalf("malformed explicit values did not fail closed: %#v", got)
	}
	if diagnostic.RevisionStatus != RevisionUnknown {
		t.Fatalf("status = %q, want unknown", diagnostic.RevisionStatus)
	}

	missing := (Detector{Source: fakeSource{repoErr: errors.New("no repo")}, ProcessStart: time.Unix(123, 0)}).Capture("/repo")
	if missing.Fingerprint.Revision != "" || missing.Fingerprint.Dirty != nil || missing.RevisionStatus != RevisionUnknown {
		t.Fatalf("missing build info = %#v", missing)
	}
}

func TestDirtyParserRejectsNonCanonicalBooleanStamps(t *testing.T) {
	for _, value := range []string{"1", "0", "t", "yes", "clean"} {
		if got := parseBool(value); got != nil {
			t.Errorf("parseBool(%q) = %v, want unknown", value, *got)
		}
	}
}

func TestCompareRevisionNeverTreatsUnknownAsMatch(t *testing.T) {
	clean, dirty := false, true
	tests := []struct {
		name string
		fp   Fingerprint
		repo string
		want RevisionStatus
	}{
		{"same clean", Fingerprint{Revision: revisionA, Dirty: &clean}, revisionA, RevisionMatch},
		{"different clean", Fingerprint{Revision: revisionA, Dirty: &clean}, revisionB, RevisionStale},
		{"same dirty", Fingerprint{Revision: revisionA, Dirty: &dirty}, revisionA, RevisionUnknown},
		{"different dirty", Fingerprint{Revision: revisionA, Dirty: &dirty}, revisionB, RevisionStale},
		{"different unknown dirty", Fingerprint{Revision: revisionA}, revisionB, RevisionStale},
		{"unknown dirty", Fingerprint{Revision: revisionA}, revisionA, RevisionUnknown},
		{"short build", Fingerprint{Revision: "aaaaaaa", Dirty: &clean}, revisionA, RevisionUnknown},
		{"short repo", Fingerprint{Revision: revisionA, Dirty: &clean}, "aaaaaaa", RevisionUnknown},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := CompareRevision(test.fp, test.repo); got != test.want {
				t.Fatalf("CompareRevision() = %q, want %q", got, test.want)
			}
		})
	}
}

func buildInfo(version, revision, modified, buildTime string) *debug.BuildInfo {
	return &debug.BuildInfo{
		Main: debug.Module{Version: version},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: strings.TrimSpace(revision)},
			{Key: "vcs.modified", Value: modified},
			{Key: "vcs.time", Value: buildTime},
		},
	}
}
