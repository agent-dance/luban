// Package buildinfo captures the identity of the running executable and
// compares that immutable identity with a repository checkout.
package buildinfo

import (
	"context"
	"os"
	"os/exec"
	"runtime/debug"
	"strings"
	"time"
)

// DefaultVersion is the source-tree version used when neither ldflags nor Go
// module metadata provides a release version.
const DefaultVersion = "v0.1.0"

// These values are intended to be populated with go build -ldflags -X. Empty
// values deliberately mean unknown so debug.ReadBuildInfo can provide the
// standard Go VCS fallback.
var (
	Version   string
	Revision  string
	Dirty     string
	BuildTime string
)

var processStart = time.Now().UTC()

// Fingerprint is the immutable identity of one writer executable. Dirty is a
// pointer because an absent VCS stamp must never be interpreted as clean.
type Fingerprint struct {
	Version      string     `json:"version,omitempty"`
	Revision     string     `json:"revision,omitempty"`
	Dirty        *bool      `json:"dirty,omitempty"`
	BuildTime    *time.Time `json:"build_time,omitempty"`
	ProcessStart time.Time  `json:"process_start"`
	Executable   string     `json:"executable,omitempty"`
}

// RevisionStatus describes whether the executable can be proven to match the
// repository HEAD. Unknown is intentionally distinct from Match.
type RevisionStatus string

const (
	RevisionUnknown RevisionStatus = "unknown"
	RevisionMatch   RevisionStatus = "match"
	RevisionStale   RevisionStatus = "stale"
)

// Diagnostic adds repository comparison data to a process fingerprint.
type Diagnostic struct {
	Fingerprint        Fingerprint    `json:"fingerprint"`
	RepositoryRevision string         `json:"repository_revision,omitempty"`
	RevisionStatus     RevisionStatus `json:"revision_status"`
}

// LinkValues contains values supplied by the release build. Each non-empty
// value takes precedence over the corresponding Go build-info setting.
type LinkValues struct {
	Version   string
	Revision  string
	Dirty     string
	BuildTime string
}

// Source isolates the process and repository probes so capture and comparison
// can be tested without relying on the test binary or a live Git checkout.
type Source interface {
	ReadBuildInfo() (*debug.BuildInfo, bool)
	Executable() (string, error)
	RepositoryRevision(string) (string, error)
}

// Detector captures a fingerprint from injected sources. Now is called at
// most once and only when ProcessStart is zero.
type Detector struct {
	Source          Source
	Now             func() time.Time
	ProcessStart    time.Time
	Link            LinkValues
	FallbackVersion string
}

// Current captures the running process and compares it with repoDir. Passing
// an empty repoDir skips the repository probe and yields an unknown status.
func Current(repoDir string) Diagnostic {
	return Detector{
		Source:       systemSource{},
		Now:          time.Now,
		ProcessStart: processStart,
		Link: LinkValues{
			Version: Version, Revision: Revision, Dirty: Dirty, BuildTime: BuildTime,
		},
		FallbackVersion: DefaultVersion,
	}.Capture(repoDir)
}

// Capture reads one deterministic process fingerprint and optional repository
// revision. Malformed explicit linker values fail closed instead of silently
// falling back to a different provenance.
func (d Detector) Capture(repoDir string) Diagnostic {
	source := d.Source
	if source == nil {
		source = systemSource{}
	}
	started := d.ProcessStart
	if started.IsZero() {
		now := d.Now
		if now == nil {
			now = time.Now
		}
		started = now()
	}
	started = started.UTC()

	settings := make(map[string]string)
	moduleVersion := ""
	if info, ok := source.ReadBuildInfo(); ok && info != nil {
		if value := strings.TrimSpace(info.Main.Version); value != "" && value != "(devel)" {
			moduleVersion = value
		}
		for _, setting := range info.Settings {
			settings[setting.Key] = setting.Value
		}
	}

	version := firstNonEmpty(d.Link.Version, moduleVersion, d.FallbackVersion)
	revision := resolveRevision(d.Link.Revision, settings["vcs.revision"])
	dirty := resolveDirty(d.Link.Dirty, settings["vcs.modified"])
	buildTime := resolveTime(d.Link.BuildTime, settings["vcs.time"])
	executable, _ := source.Executable()

	fingerprint := Fingerprint{
		Version: version, Revision: revision, Dirty: dirty, BuildTime: buildTime,
		ProcessStart: started, Executable: strings.TrimSpace(executable),
	}
	diagnostic := Diagnostic{Fingerprint: fingerprint, RevisionStatus: RevisionUnknown}
	if strings.TrimSpace(repoDir) == "" {
		return diagnostic
	}
	repoRevision, err := source.RepositoryRevision(repoDir)
	if err != nil {
		return diagnostic
	}
	diagnostic.RepositoryRevision = normalizeRevision(repoRevision)
	diagnostic.RevisionStatus = CompareRevision(fingerprint, diagnostic.RepositoryRevision)
	return diagnostic
}

// CompareRevision returns Match only when both full revisions are known,
// identical, and the writer was explicitly stamped clean.
func CompareRevision(fingerprint Fingerprint, repositoryRevision string) RevisionStatus {
	buildRevision := normalizeRevision(fingerprint.Revision)
	repoRevision := normalizeRevision(repositoryRevision)
	if buildRevision == "" || repoRevision == "" {
		return RevisionUnknown
	}
	if buildRevision != repoRevision {
		return RevisionStale
	}
	if fingerprint.Dirty == nil || *fingerprint.Dirty {
		return RevisionUnknown
	}
	return RevisionMatch
}

// ShortRevision formats a known full revision for compact diagnostics.
func ShortRevision(revision string) string {
	revision = normalizeRevision(revision)
	if len(revision) > 12 {
		return revision[:12]
	}
	return revision
}

func resolveRevision(explicit, fallback string) string {
	if strings.TrimSpace(explicit) != "" {
		return normalizeRevision(explicit)
	}
	return normalizeRevision(fallback)
}

func resolveDirty(explicit, fallback string) *bool {
	if strings.TrimSpace(explicit) != "" {
		return parseBool(explicit)
	}
	return parseBool(fallback)
}

func resolveTime(explicit, fallback string) *time.Time {
	if strings.TrimSpace(explicit) != "" {
		return parseTime(explicit)
	}
	return parseTime(fallback)
}

func parseBool(value string) *bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true":
		parsed := true
		return &parsed
	case "false":
		parsed := false
		return &parsed
	default:
		return nil
	}
}

func parseTime(value string) *time.Time {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return nil
	}
	parsed = parsed.UTC()
	return &parsed
}

func normalizeRevision(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) != 40 && len(value) != 64 {
		return ""
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return ""
		}
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

type systemSource struct{}

func (systemSource) ReadBuildInfo() (*debug.BuildInfo, bool) { return debug.ReadBuildInfo() }
func (systemSource) Executable() (string, error)             { return os.Executable() }
func (systemSource) RepositoryRevision(repoDir string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "git", "-C", repoDir, "rev-parse", "--verify", "HEAD").Output()
	return strings.TrimSpace(string(output)), err
}
