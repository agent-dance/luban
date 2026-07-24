package skills

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/brand"
)

// Manager discovers, loads, caches, and provides access to skills from
// multiple prioritised source directories. Thread-safe.
//
// This replaces both the old skills.Loader (dead code) and the
// tools.SkillManager. The tools.SkillTool now depends on this Manager.
//
// The manager performs a stat-based "live rescan" check on each lookup
// (Get/All/Names) so newly created skill directories show up without an
// explicit Refresh() call. We deliberately avoid a goroutine + fsnotify
// dependency: the cost is one filepath.Stat per registered dir per lookup,
// which is negligible compared to skill loading itself.
type Manager struct {
	// txnMu keeps readers from observing the persistent half of a project
	// visibility transaction before its effective registry state is installed.
	// Ordinary discovery state remains protected by mu.
	txnMu sync.RWMutex
	mu    sync.RWMutex

	dirs      []DirSource // ordered by priority (highest first)
	cache     map[string]*Skill
	records   map[SkillID]catalogRecord
	winners   map[string]SkillID
	views     map[string]catalogView
	populated bool

	overrideStore OverrideStore

	// dirSnapshots tracks the most recent stat fingerprint per source dir.
	// Keyed by absolute dir path; value is the file modtime + listing hash
	// captured during the last populate(). populate() rebuilds it; Get()
	// compares against the live filesystem to decide whether to invalidate.
	dirSnapshots map[string]dirSnapshot

	// mcpPrompts holds skills supplied by MCP servers. Wired in by
	// RegisterMCPPrompts; merged on top of on-disk skills during populate().
	mcpPrompts *mcpPromptStore
	// activatedConditional records the per-name activation side effect used by
	// Read/Write/Edit path triggers. It is session-local and survives rescans.
	activatedConditional map[string]struct{}

	// disabledBySession is a runtime availability overlay. Discovery remains a
	// catalog concern: disabled skills stay visible to management surfaces, but
	// model listings and Skill tool execution can filter them per session.
	// Keeping the overlay separate from the loaded Skill values also means a
	// filesystem rescan never silently re-enables a skill.
	disabledBySession       map[string]map[string]struct{}
	disabledOwnersBySession map[string]map[string]legacyDisableOwner

	// projectGeneration identifies the project-source authority currently
	// installed in this Manager. It advances only at the same txnMu writer
	// boundary that publishes a prepared project retarget. Query loops pin the
	// value with their catalog snapshot; model-origin Skill execution presents
	// that pin back to ResolveLatest so an old workspace cannot execute content
	// from a newly retargeted catalog through the shared Manager pointer.
	projectGeneration ProjectSourceGeneration
	projectAuthority  string
}

// ProjectSourceGeneration is a process-local capability version for the
// Manager's project-owned skill roots. Zero means "not pinned" and is accepted
// only by compatibility/user-origin callers that intentionally resolve the
// current authority.
type ProjectSourceGeneration uint64

// Validate requires a real, pinned project-source generation.
func (generation ProjectSourceGeneration) Validate() error {
	if generation == 0 {
		return errors.New("skill project-source generation is zero")
	}
	return nil
}

// CatalogBinding atomically binds an authoritative session projection to the
// exact project-source generation that produced it.
type CatalogBinding struct {
	ProjectGeneration ProjectSourceGeneration
	Snapshot          CatalogSnapshot
}

// Clone returns a defensive copy suitable for retaining for one model run.
func (binding CatalogBinding) Clone() CatalogBinding {
	return CatalogBinding{
		ProjectGeneration: binding.ProjectGeneration,
		Snapshot:          binding.Snapshot.Clone(),
	}
}

// Validate checks both halves of the run-level authority binding.
func (binding CatalogBinding) Validate() error {
	if err := binding.ProjectGeneration.Validate(); err != nil {
		return err
	}
	return binding.Snapshot.Validate()
}

type legacyDisableOwner struct {
	SkillID SkillID
	Source  SkillSource
}

type catalogRecord struct {
	skill    *Skill
	id       SkillID
	locator  SkillLocator
	digest   SkillDigest
	priority int
}

type catalogVersionState struct {
	fingerprint string
	revision    SkillRevision
	present     bool
}

type catalogView struct {
	snapshot  CatalogSnapshot
	versions  map[SkillID]catalogVersionState
	overrides OverrideSnapshot
}

// dirSnapshot captures enough of a directory's state to detect "did
// something change here since the last scan" without having to open every
// file. We track the dir's modtime AND the per-entry modtimes/sizes
// because some filesystems (notably APFS and Windows when files are
// modified atomically) can leave the parent dir mtime untouched.
type dirSnapshot struct {
	dirModTime time.Time
	entries    map[string]entryFingerprint
}

type entryFingerprint struct {
	modTime time.Time
	size    int64
	isDir   bool
}

// DirSource pairs a search directory with its SkillSource classification.
type DirSource struct {
	Dir    string
	Source SkillSource
}

// CatalogEntry is one immutable-at-read-time view of a discovered skill and
// its effective runtime availability for a session.
type CatalogEntry struct {
	Skill   *Skill
	Enabled bool
}

// ResolvedSkill binds immutable effective metadata to the exact parsed skill
// content that produced its digest. Both values are defensive copies.
type ResolvedSkill struct {
	Effective EffectiveSkill
	Skill     *Skill
}

// Validate checks that parsed content still matches the immutable effective
// identity and exact content digest.
func (resolved ResolvedSkill) Validate() error {
	if err := resolved.Effective.Validate(); err != nil {
		return err
	}
	if resolved.Skill == nil {
		return errors.New("resolved skill content is nil")
	}
	if resolved.Skill.Name != resolved.Effective.Name || resolved.Skill.Source != resolved.Effective.Source {
		return errors.New("resolved skill content does not match effective identity")
	}
	if ComputeSkillDigest(resolved.Skill.RawContent) != resolved.Effective.Digest {
		return errors.New("resolved skill content does not match effective digest")
	}
	return nil
}

// SkillResolveRequest describes one execution-time lookup. ExpectedRevision
// zero means "latest"; a non-zero value protects a previously selected row
// from executing after its effective state changes.
type SkillResolveRequest struct {
	SessionID                 string
	Selector                  string
	ExpectedRevision          SkillRevision
	ExpectedProjectGeneration ProjectSourceGeneration
	Origin                    InvocationOrigin
}

func (request SkillResolveRequest) Validate() error {
	if strings.TrimSpace(request.SessionID) != request.SessionID {
		return errors.New("skill resolve session ID is padded")
	}
	if strings.TrimSpace(request.Selector) == "" || strings.TrimSpace(request.Selector) != request.Selector || hasControl(request.Selector) {
		return errors.New("skill resolve selector is empty, padded, or contains control characters")
	}
	if request.ExpectedRevision != 0 {
		if err := request.ExpectedRevision.Validate(); err != nil {
			return err
		}
	}
	if request.ExpectedProjectGeneration != 0 {
		if err := request.ExpectedProjectGeneration.Validate(); err != nil {
			return err
		}
	}
	return request.Origin.Validate()
}

// SkillResolveOutcome is a typed execution decision. Presentation layers map
// these stable codes to localized messages; they must not parse error strings.
type SkillResolveOutcome string

const (
	SkillResolveResolved     SkillResolveOutcome = "resolved"
	SkillResolveNotFound     SkillResolveOutcome = "not-found"
	SkillResolveAmbiguous    SkillResolveOutcome = "ambiguous"
	SkillResolveShadowed     SkillResolveOutcome = "shadowed"
	SkillResolvePolicyDenied SkillResolveOutcome = "policy-denied"
	SkillResolveStale        SkillResolveOutcome = "stale-revision"
)

func (outcome SkillResolveOutcome) Validate() error {
	switch outcome {
	case SkillResolveResolved, SkillResolveNotFound, SkillResolveAmbiguous,
		SkillResolveShadowed, SkillResolvePolicyDenied, SkillResolveStale:
		return nil
	default:
		return fmt.Errorf("invalid skill resolve outcome %q", outcome)
	}
}

// SkillResolveResult includes current immutable state for both accepted and
// rejected selections. Candidates are stable IDs in deterministic order.
type SkillResolveResult struct {
	Outcome         SkillResolveOutcome
	CatalogRevision CatalogRevision
	Candidates      []SkillID
	Resolved        *ResolvedSkill
}

// Validate checks typed outcome shape, deterministic candidates, and any
// attached execution-ready resolution.
func (result SkillResolveResult) Validate() error {
	if err := result.Outcome.Validate(); err != nil {
		return err
	}
	if err := result.CatalogRevision.Validate(); err != nil {
		return err
	}
	var previous SkillID
	for index, id := range result.Candidates {
		if err := id.Validate(); err != nil {
			return err
		}
		if index > 0 && id <= previous {
			return errors.New("skill resolve candidates must be unique and sorted")
		}
		previous = id
	}
	if result.Resolved != nil {
		if err := result.Resolved.Validate(); err != nil {
			return err
		}
	}
	switch result.Outcome {
	case SkillResolveResolved, SkillResolveShadowed, SkillResolvePolicyDenied, SkillResolveStale:
		if result.Resolved == nil {
			return errors.New("skill resolve outcome requires current resolved state")
		}
	case SkillResolveNotFound:
		if result.Resolved != nil {
			return errors.New("not-found skill resolve cannot carry resolved state")
		}
	case SkillResolveAmbiguous:
		if result.Resolved != nil || len(result.Candidates) < 2 {
			return errors.New("ambiguous skill resolve requires multiple candidates and no selected state")
		}
	}
	return nil
}

var (
	ErrSkillNotFound             = errors.New("skill not found")
	ErrSkillOverrideStoreMissing = errors.New("skill override store is not configured")
	// ErrSkillProjectGenerationChanged means a model run pinned an older
	// project authority and must not resolve against the Manager's new roots.
	ErrSkillProjectGenerationChanged = errors.New("skill project-source generation changed")
)

// NewManager creates a Manager that searches the given directories in
// order (first match wins). Use DefaultDirs() for the standard paths.
func NewManager(dirs ...DirSource) *Manager {
	return &Manager{
		dirs:                    dirs,
		cache:                   make(map[string]*Skill),
		records:                 make(map[SkillID]catalogRecord),
		winners:                 make(map[string]SkillID),
		views:                   make(map[string]catalogView),
		dirSnapshots:            make(map[string]dirSnapshot),
		activatedConditional:    make(map[string]struct{}),
		disabledBySession:       make(map[string]map[string]struct{}),
		disabledOwnersBySession: make(map[string]map[string]legacyDisableOwner),
		projectGeneration:       1,
	}
}

// NewManagerWithOverrideStore creates a Manager with an explicit layered
// override store. Runtime composition should prefer this constructor so every
// catalog consumer observes the same persistent and session policy source.
func NewManagerWithOverrideStore(store OverrideStore, dirs ...DirSource) *Manager {
	manager := NewManager(dirs...)
	manager.overrideStore = store
	return manager
}

// SetOverrideStore replaces the policy storage boundary without resetting
// revision history. The next snapshot advances only if the effective catalog
// actually differs.
func (m *Manager) SetOverrideStore(store OverrideStore) {
	if m == nil {
		return
	}
	m.txnMu.Lock()
	m.mu.Lock()
	m.overrideStore = store
	m.mu.Unlock()
	m.txnMu.Unlock()
}

// DefaultDirs returns the conventional skill search paths:
//
//	.luban-code/skills/       (project-level, highest priority)
//	.deepseek-code/skills/  (legacy DeepSeek project-level)
//	.claude/skills/         (legacy Claude project-level)
//	~/.luban-code/skills/     (user-level)
//
// Matches TS loadSkillsDir.ts getSkillDirs() for project + user sources.
func DefaultDirs() []DirSource {
	dirs := []DirSource{
		{Dir: filepath.Join(brand.ConfigDirName, "skills"), Source: SourceProject},
		{Dir: filepath.Join(brand.LegacyDeepSeekConfigDirName, "skills"), Source: SourceProject},
		{Dir: filepath.Join(brand.LegacyConfigDirName, "skills"), Source: SourceProject},
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs,
			DirSource{Dir: filepath.Join(home, brand.ConfigDirName, "skills"), Source: SourceUser},
			DirSource{Dir: filepath.Join(home, brand.LegacyDeepSeekConfigDirName, "skills"), Source: SourceUser},
			DirSource{Dir: filepath.Join(home, brand.LegacyConfigDirName, "skills"), Source: SourceUser},
		)
	}
	return dirs
}

// ProjectDirs returns the canonical project-owned skill roots for one explicit
// workspace. Runtime composition uses absolute roots because a session switch
// deliberately does not mutate the process working directory.
func ProjectDirs(projectRoot string) ([]DirSource, error) {
	root := strings.TrimSpace(projectRoot)
	if root == "" || root != projectRoot {
		return nil, errors.New("skill project root is empty or padded")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve skill project root: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("stat skill project root: %w", err)
	}
	if !info.IsDir() {
		return nil, errors.New("skill project root is not a directory")
	}
	return []DirSource{
		{Dir: filepath.Join(abs, brand.ConfigDirName, "skills"), Source: SourceProject},
		{Dir: filepath.Join(abs, brand.LegacyDeepSeekConfigDirName, "skills"), Source: SourceProject},
		{Dir: filepath.Join(abs, brand.LegacyConfigDirName, "skills"), Source: SourceProject},
	}, nil
}

// ProjectSourcePlan is an immutable, prevalidated workspace replacement. Its
// internals are intentionally private so only the Manager that prepared the
// plan can publish it.
type ProjectSourcePlan struct {
	manager            *Manager
	store              *FileOverrideStore
	dirs               []DirSource
	projectSettings    string
	projectAuthority   string
	userRevision       OverrideStoreRevision
	projectRevision    OverrideStoreRevision
	legacyProjectNames []string
	mcpCatalogInputs   []MCPCatalogInput
	mcpProjectionSet   bool
}

// PrepareProjectSources performs every fallible workspace check before a
// session resume commits. Applying the returned plan performs no filesystem
// I/O and cannot expose a partially retargeted catalog.
func (m *Manager) PrepareProjectSources(projectRoot string) (*ProjectSourcePlan, error) {
	if m == nil {
		return nil, errors.New("nil skill manager")
	}
	projectDirs, err := ProjectDirs(projectRoot)
	if err != nil {
		return nil, err
	}
	paths, err := DefaultOverrideStorePaths(projectRoot)
	if err != nil {
		return nil, err
	}
	projectAuthority, err := canonicalProjectAuthority(projectRoot)
	if err != nil {
		return nil, err
	}

	m.txnMu.RLock()
	defer m.txnMu.RUnlock()
	m.ensurePopulated()
	m.mu.RLock()
	store := m.overrideStore
	legacyProjectNames := make([]string, 0)
	for name, winner := range m.winners {
		if record, found := m.records[winner]; found && record.skill != nil && record.skill.Source == SourceProject {
			legacyProjectNames = append(legacyProjectNames, name)
		}
	}
	m.mu.RUnlock()
	sort.Strings(legacyProjectNames)
	if store == nil {
		return nil, ErrSkillOverrideStoreMissing
	}
	fileStore, ok := store.(*FileOverrideStore)
	if !ok || fileStore == nil {
		return nil, errors.New("skill project override store cannot be retargeted")
	}
	userRevision, projectRevision, err := fileStore.prepareProjectRetarget(paths.ProjectSettings)
	if err != nil {
		return nil, err
	}
	return &ProjectSourcePlan{
		manager: m, store: fileStore, dirs: append([]DirSource(nil), projectDirs...),
		projectSettings: paths.ProjectSettings, projectAuthority: projectAuthority,
		userRevision: userRevision, projectRevision: projectRevision,
		legacyProjectNames: append([]string(nil), legacyProjectNames...),
	}, nil
}

// CommitProjectSources publishes a plan prepared by this Manager. The target
// settings revisions are revalidated while the Manager transaction is held.
// beforePublish is the composition layer's final fallible commit (for example
// an engine resume or worktree rebind); when it fails, neither the store nor
// the Manager changes. Once it succeeds, the remaining publication is
// in-memory-only and cannot fail.
func (m *Manager) CommitProjectSources(plan *ProjectSourcePlan, beforePublish func() error) error {
	return m.CommitProjectSourcesWithAfter(plan, beforePublish, nil)
}

// CommitProjectSourcesWithAfter extends CommitProjectSources with one
// infallible composition callback that runs after the prepared store/catalog
// has been installed but before txnMu is released. This lets the composition
// root publish its matching session/runtime pointers while every Manager
// reader remains blocked, so no caller can observe a new catalog paired with
// the old workspace. afterPublish must not fail, block on external I/O, or
// call back into Manager.
func (m *Manager) CommitProjectSourcesWithAfter(plan *ProjectSourcePlan, beforePublish func() error, afterPublish func()) error {
	if m == nil || plan == nil || plan.manager != m || plan.store == nil || plan.projectSettings == "" {
		return errors.New("invalid skill project-source plan")
	}
	m.txnMu.Lock()
	defer m.txnMu.Unlock()
	return plan.store.commitPreparedProjectRetarget(
		plan.projectSettings,
		plan.userRevision,
		plan.projectRevision,
		beforePublish,
		func() {
			m.applyProjectSourcesLocked(plan)
			if afterPublish != nil {
				afterPublish()
			}
		},
	)
}

// ApplyProjectSources is the direct commit form for callers without an
// adjacent fallible runtime publication.
func (m *Manager) ApplyProjectSources(plan *ProjectSourcePlan) error {
	return m.CommitProjectSources(plan, nil)
}

// applyProjectSourcesLocked performs the infallible Manager half of a staged
// publication. Caller holds txnMu for writing and has already committed the
// FileOverrideStore project owner.
func (m *Manager) applyProjectSourcesLocked(plan *ProjectSourcePlan) {
	m.mu.Lock()
	if plan.mcpProjectionSet {
		if m.mcpPrompts == nil {
			m.mcpPrompts = newMCPPromptStore()
		}
		m.mcpPrompts.replaceAllInputs(plan.mcpCatalogInputs)
	}
	// Re-publish the exact store captured by the validated plan. Runtime
	// composition owns this pointer; retaining it also closes a typed-nil or
	// accidental replacement gap between prepare and apply.
	m.overrideStore = plan.store
	legacyProjectNames := make(map[string]struct{}, len(plan.legacyProjectNames)+len(m.winners))
	for _, name := range plan.legacyProjectNames {
		legacyProjectNames[name] = struct{}{}
	}
	// Prepare freezes the old catalog, but ordinary discovery and legacy
	// SetEnabled calls may legitimately refresh it before Apply obtains the
	// transaction writer. Reconcile only the already-materialized in-memory
	// winners here; calling ensurePopulated would perform forbidden apply-time
	// filesystem I/O.
	for name, winner := range m.winners {
		if record, found := m.records[winner]; found && record.skill != nil && record.skill.Source == SourceProject {
			legacyProjectNames[name] = struct{}{}
		}
	}
	for sessionID, disabled := range m.disabledBySession {
		for name := range legacyProjectNames {
			owner, owned := m.disabledOwnersBySession[sessionID][name]
			if owned && owner.Source != SourceProject {
				continue
			}
			delete(disabled, name)
			delete(m.disabledOwnersBySession[sessionID], name)
		}
		if len(disabled) == 0 {
			delete(m.disabledBySession, sessionID)
			delete(m.disabledOwnersBySession, sessionID)
		}
	}
	next := make([]DirSource, 0, len(m.dirs)+len(plan.dirs))
	next = append(next, plan.dirs...)
	for _, dir := range m.dirs {
		if dir.Source != SourceProject {
			next = append(next, dir)
		}
	}
	m.dirs = next
	m.cache = make(map[string]*Skill)
	m.records = make(map[SkillID]catalogRecord)
	m.winners = make(map[string]SkillID)
	m.dirSnapshots = make(map[string]dirSnapshot)
	m.activatedConditional = make(map[string]struct{})
	m.populated = false
	authorityChanged := m.projectAuthority != plan.projectAuthority
	m.projectAuthority = plan.projectAuthority
	if authorityChanged {
		m.advanceProjectGenerationLocked()
	}
	m.mu.Unlock()
}

func canonicalProjectAuthority(projectRoot string) (string, error) {
	root := strings.TrimSpace(projectRoot)
	if root == "" || root != projectRoot {
		return "", errors.New("skill project root is empty or padded")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve skill project authority: %w", err)
	}
	abs = filepath.Clean(abs)
	if resolved, resolveErr := filepath.EvalSymlinks(abs); resolveErr == nil {
		abs = filepath.Clean(resolved)
	}
	return abs, nil
}

// ReplaceProjectSources is the direct convenience API for callers that do not
// already own a staged session transaction.
func (m *Manager) ReplaceProjectSources(projectRoot string) error {
	plan, err := m.PrepareProjectSources(projectRoot)
	if err != nil {
		return err
	}
	return m.ApplyProjectSources(plan)
}

// Get returns the named skill, scanning directories if not yet cached.
// Returns nil if the skill does not exist.
func (m *Manager) Get(name string) *Skill {
	if m == nil {
		return nil
	}
	m.txnMu.RLock()
	defer m.txnMu.RUnlock()
	m.ensurePopulated()

	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cache[name]
}

// All returns all loaded skills sorted by name.
func (m *Manager) All() []*Skill {
	if m == nil {
		return nil
	}
	m.txnMu.RLock()
	defer m.txnMu.RUnlock()
	m.ensurePopulated()

	m.mu.RLock()
	defer m.mu.RUnlock()

	skills := make([]*Skill, 0, len(m.cache))
	for _, s := range m.cache {
		skills = append(skills, s)
	}
	sort.Slice(skills, func(i, j int) bool {
		return skills[i].Name < skills[j].Name
	})
	return skills
}

// Snapshot returns the immutable effective registry for one session. Revision
// advances only when that session's effective catalog state changes.
func (m *Manager) Snapshot(sessionID string) (CatalogSnapshot, error) {
	if m == nil {
		return CatalogSnapshot{}, errors.New("nil skill manager")
	}
	m.txnMu.RLock()
	defer m.txnMu.RUnlock()
	return m.snapshotCurrent(strings.TrimSpace(sessionID))
}

// SnapshotBinding atomically captures the effective session catalog and the
// project-source generation that produced it. The generation and snapshot are
// read beneath the same transaction lock, so a concurrent workspace retarget
// is observed wholly before or wholly after this call.
func (m *Manager) SnapshotBinding(sessionID string) (CatalogBinding, error) {
	if m == nil {
		return CatalogBinding{}, errors.New("nil skill manager")
	}
	m.txnMu.RLock()
	defer m.txnMu.RUnlock()
	snapshot, err := m.snapshotCurrent(strings.TrimSpace(sessionID))
	if err != nil {
		return CatalogBinding{}, err
	}
	m.mu.RLock()
	generation := m.projectGeneration
	m.mu.RUnlock()
	binding := CatalogBinding{ProjectGeneration: generation, Snapshot: snapshot}
	if err := binding.Validate(); err != nil {
		return CatalogBinding{}, err
	}
	return binding.Clone(), nil
}

// SnapshotAtGeneration returns the latest same-workspace catalog projection
// for a run that pinned expected. Ordinary refreshes and nearby dynamic
// discovery keep the generation stable and can therefore produce in-run
// deltas; a project retarget changes the generation and fails closed before a
// snapshot from the new workspace can be observed.
func (m *Manager) SnapshotAtGeneration(sessionID string, expected ProjectSourceGeneration) (CatalogSnapshot, error) {
	if m == nil {
		return CatalogSnapshot{}, errors.New("nil skill manager")
	}
	if err := expected.Validate(); err != nil {
		return CatalogSnapshot{}, err
	}
	m.txnMu.RLock()
	defer m.txnMu.RUnlock()
	m.mu.RLock()
	current := m.projectGeneration
	m.mu.RUnlock()
	if current != expected {
		return CatalogSnapshot{}, projectGenerationChangedError(expected, current)
	}
	return m.snapshotCurrent(strings.TrimSpace(sessionID))
}

// ConsumeSnapshotAtGeneration is the run-bound counterpart of
// ConsumeSnapshot. The generation check and callback snapshot share one
// Manager read transaction, while the lock is still released between sampling
// boundaries so a project retarget cannot deadlock behind a long model run.
func (m *Manager) ConsumeSnapshotAtGeneration(sessionID string, expected ProjectSourceGeneration, consume func(CatalogSnapshot) error) error {
	if m == nil {
		return errors.New("nil skill manager")
	}
	if err := expected.Validate(); err != nil {
		return err
	}
	m.txnMu.RLock()
	defer m.txnMu.RUnlock()
	m.mu.RLock()
	current := m.projectGeneration
	m.mu.RUnlock()
	if current != expected {
		return projectGenerationChangedError(expected, current)
	}
	snapshot, err := m.snapshotCurrent(strings.TrimSpace(sessionID))
	if err != nil {
		return err
	}
	if consume == nil {
		return nil
	}
	return consume(snapshot.Clone())
}

// ProjectGeneration returns the current project-source authority version. It
// is intended for diagnostics and transition CAS; model runs should use
// SnapshotBinding so the generation cannot be separated from its projection.
func (m *Manager) ProjectGeneration() ProjectSourceGeneration {
	if m == nil {
		return 0
	}
	m.txnMu.RLock()
	defer m.txnMu.RUnlock()
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.projectGeneration
}

// ValidateProjectGeneration is a no-discovery authority gate for model-facing
// tools that may perform side effects before creating a child QueryLoop. A
// stale parent run must be rejected before it reads a retargeted runtime,
// resolves profile skills, or persists child/team state.
func (m *Manager) ValidateProjectGeneration(expected ProjectSourceGeneration) error {
	if m == nil {
		return errors.New("nil skill manager")
	}
	if err := expected.Validate(); err != nil {
		return err
	}
	m.txnMu.RLock()
	defer m.txnMu.RUnlock()
	m.mu.RLock()
	current := m.projectGeneration
	m.mu.RUnlock()
	if current != expected {
		return projectGenerationChangedError(expected, current)
	}
	return nil
}

// WithProjectGenerationLease runs commit while expected still owns the
// Manager's project authority. The read lease closes the validate-then-publish
// gap for short in-memory registrations (for example Agent/Team ownership)
// without pinning a whole child run or provider call. commit must stay bounded,
// must not perform network/provider I/O, and must not call back into Manager.
// A short local durable write may be included when it is itself part of the
// fenced publication; moving that write outside the lease would reopen the
// validate-then-persist race.
func (m *Manager) WithProjectGenerationLease(expected ProjectSourceGeneration, commit func() error) error {
	if m == nil {
		return errors.New("nil skill manager")
	}
	if err := expected.Validate(); err != nil {
		return err
	}
	m.txnMu.RLock()
	defer m.txnMu.RUnlock()
	m.mu.RLock()
	current := m.projectGeneration
	m.mu.RUnlock()
	if current != expected {
		return projectGenerationChangedError(expected, current)
	}
	if commit == nil {
		return nil
	}
	return commit()
}

// ConsumeSnapshot runs consume against one defensive authoritative snapshot
// while refreshes and visibility mutations are excluded by the Manager read
// transaction. This is intended for projection-install boundaries that must
// not expose a snapshot/body combination made stale halfway through install.
//
// consume must not call back into Manager: doing so would attempt to acquire
// txnMu recursively and can deadlock behind a waiting writer. Keep callbacks
// bounded and free of external I/O.
func (m *Manager) ConsumeSnapshot(sessionID string, consume func(CatalogSnapshot) error) error {
	if m == nil {
		return errors.New("nil skill manager")
	}
	m.txnMu.RLock()
	defer m.txnMu.RUnlock()
	snapshot, err := m.snapshotCurrent(strings.TrimSpace(sessionID))
	if err != nil {
		return err
	}
	if consume == nil {
		return nil
	}
	return consume(snapshot.Clone())
}

// LegacySnapshot preserves the pre-registry availability projection while
// command and presentation consumers migrate to EffectiveSkill rows.
func (m *Manager) LegacySnapshot(sessionID string) []CatalogEntry {
	if m == nil {
		return nil
	}
	m.txnMu.RLock()
	defer m.txnMu.RUnlock()
	m.ensurePopulated()

	sessionID = strings.TrimSpace(sessionID)
	m.mu.RLock()
	entries := make([]CatalogEntry, 0, len(m.cache))
	for name, skill := range m.cache {
		_, disabled := m.disabledBySession[sessionID][name]
		entries = append(entries, CatalogEntry{Skill: cloneManagerSkill(skill), Enabled: !disabled})
	}
	m.mu.RUnlock()
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Skill.Name < entries[j].Skill.Name
	})
	return entries
}

// Resolve finds a stable ID directly or the effective winner for an invocation
// name. It returns policy metadata even when the current state is off so the
// caller can enforce and explain the latest authorization decision.
func (m *Manager) Resolve(sessionID, stableIDOrName string) (ResolvedSkill, bool, error) {
	if m == nil {
		return ResolvedSkill{}, false, errors.New("nil skill manager")
	}
	target := strings.TrimSpace(stableIDOrName)
	if target == "" {
		return ResolvedSkill{}, false, nil
	}
	sessionID = strings.TrimSpace(sessionID)

	m.txnMu.RLock()
	defer m.txnMu.RUnlock()
	for {
		snapshot, err := m.snapshotCurrent(sessionID)
		if err != nil {
			return ResolvedSkill{}, false, err
		}
		var effective EffectiveSkill
		var found bool
		if id := SkillID(target); id.IsValid() {
			effective, found = snapshot.Find(id)
		} else {
			for _, candidate := range snapshot.Skills {
				if candidate.Name == target && candidate.ShadowedBy == "" {
					effective, found = candidate, true
					break
				}
			}
		}
		if !found {
			return ResolvedSkill{}, false, nil
		}

		m.mu.RLock()
		view := m.views[sessionID]
		record, recordFound := m.records[effective.ID]
		stillCurrent := view.snapshot.Revision == snapshot.Revision && recordFound && record.digest == effective.Digest
		var skill *Skill
		if stillCurrent {
			skill = cloneManagerSkill(record.skill)
		}
		m.mu.RUnlock()
		if stillCurrent {
			return ResolvedSkill{Effective: effective, Skill: skill}, true, nil
		}
	}
}

// ResolveLatest performs execution authorization and execution preparation at
// one linearization boundary. consume runs while project toggles, scoped
// mutations, and refreshes are excluded; other execution resolutions may run
// concurrently. The callback must not call back into Manager. SkillTool should
// prepare the final invocation body/envelope inside consume, then return that
// immutable result to its caller.
func (m *Manager) ResolveLatest(request SkillResolveRequest, consume func(ResolvedSkill) error) (SkillResolveResult, error) {
	if err := request.Validate(); err != nil {
		return SkillResolveResult{}, err
	}
	if m == nil {
		return SkillResolveResult{}, errors.New("nil skill manager")
	}

	m.txnMu.RLock()
	defer m.txnMu.RUnlock()
	m.mu.RLock()
	projectGeneration := m.projectGeneration
	m.mu.RUnlock()
	if request.ExpectedProjectGeneration != 0 && request.ExpectedProjectGeneration != projectGeneration {
		return SkillResolveResult{}, projectGenerationChangedError(request.ExpectedProjectGeneration, projectGeneration)
	}
	for {
		snapshot, err := m.snapshotCurrent(request.SessionID)
		if err != nil {
			return SkillResolveResult{}, err
		}
		result := SkillResolveResult{CatalogRevision: snapshot.Revision}

		var selected EffectiveSkill
		var found bool
		if id := SkillID(request.Selector); id.IsValid() {
			selected, found = snapshot.Find(id)
			if found {
				result.Candidates = []SkillID{id}
			}
		} else {
			var winners []EffectiveSkill
			for _, candidate := range snapshot.Skills {
				if candidate.Name != request.Selector {
					continue
				}
				result.Candidates = append(result.Candidates, candidate.ID)
				if candidate.ShadowedBy == "" {
					winners = append(winners, candidate)
				}
			}
			switch len(winners) {
			case 0:
			case 1:
				selected, found = winners[0], true
			default:
				result.Outcome = SkillResolveAmbiguous
				return result, nil
			}
		}
		if !found {
			result.Outcome = SkillResolveNotFound
			return result, nil
		}

		m.mu.RLock()
		view := m.views[request.SessionID]
		record, recordFound := m.records[selected.ID]
		stillCurrent := view.snapshot.Revision == snapshot.Revision && recordFound && record.digest == selected.Digest
		var parsed *Skill
		if stillCurrent {
			parsed = cloneManagerSkill(record.skill)
		}
		m.mu.RUnlock()
		if !stillCurrent {
			continue
		}

		resolved := ResolvedSkill{Effective: selected, Skill: parsed}
		result.Resolved = resolvedSkillPointer(resolved)
		if request.ExpectedRevision != 0 && request.ExpectedRevision != selected.Revision {
			result.Outcome = SkillResolveStale
			return result, nil
		}
		if selected.ShadowedBy != "" {
			result.Outcome = SkillResolveShadowed
			return result, nil
		}
		allowed := selected.Executable && selected.Visibility != VisibilityOff
		switch request.Origin {
		case InvocationOriginModel:
			allowed = allowed && selected.ModelVisible
		case InvocationOriginUser:
			allowed = allowed && selected.UserInvocable
		}
		if !allowed {
			result.Outcome = SkillResolvePolicyDenied
			return result, nil
		}

		result.Outcome = SkillResolveResolved
		if consume != nil {
			if err := consume(cloneResolvedSkill(resolved)); err != nil {
				return result, err
			}
		}
		return result, nil
	}
}

func projectGenerationChangedError(expected, current ProjectSourceGeneration) error {
	return fmt.Errorf(
		"%w: expected %d, current %d",
		ErrSkillProjectGenerationChanged,
		expected,
		current,
	)
}

func resolvedSkillPointer(resolved ResolvedSkill) *ResolvedSkill {
	copy := cloneResolvedSkill(resolved)
	return &copy
}

func cloneResolvedSkill(resolved ResolvedSkill) ResolvedSkill {
	resolved.Skill = cloneManagerSkill(resolved.Skill)
	return resolved
}

// SetVisibility stores one explicit scoped state and immediately rebuilds the
// effective session view. Interactive project toggles must use
// ToggleProjectVisibility so they also receive catalog-CAS and compensation.
func (m *Manager) SetVisibility(sessionID string, override VisibilityOverride) (CatalogSnapshot, error) {
	if m == nil {
		return CatalogSnapshot{}, errors.New("nil skill manager")
	}
	m.txnMu.Lock()
	defer m.txnMu.Unlock()
	sessionID = strings.TrimSpace(sessionID)
	current, err := m.snapshotCurrent(sessionID)
	if err != nil {
		return CatalogSnapshot{}, err
	}
	row, found := current.Find(override.SkillID)
	if !found {
		return current, ErrSkillNotFound
	}
	if !row.Mutable {
		return current, ErrManagedOverrideReadOnly
	}
	store := m.currentOverrideStore()
	if store == nil {
		return current, ErrSkillOverrideStoreMissing
	}
	if err := store.Set(sessionID, override); err != nil {
		return current, err
	}
	return m.snapshotCurrent(sessionID)
}

// ResetVisibility deletes one explicit scope so policy inherits the next
// lower layer, then returns the authoritative effective snapshot.
func (m *Manager) ResetVisibility(sessionID string, scope SkillScope, id SkillID) (CatalogSnapshot, error) {
	if m == nil {
		return CatalogSnapshot{}, errors.New("nil skill manager")
	}
	m.txnMu.Lock()
	defer m.txnMu.Unlock()
	sessionID = strings.TrimSpace(sessionID)
	current, err := m.snapshotCurrent(sessionID)
	if err != nil {
		return CatalogSnapshot{}, err
	}
	row, found := current.Find(id)
	if !found {
		return current, ErrSkillNotFound
	}
	if !row.Mutable {
		return current, ErrManagedOverrideReadOnly
	}
	store := m.currentOverrideStore()
	if store == nil {
		return current, ErrSkillOverrideStoreMissing
	}
	if err := store.Reset(sessionID, scope, id); err != nil {
		return current, err
	}
	return m.snapshotCurrent(sessionID)
}

// Names returns a sorted slice of all available skill names.
func (m *Manager) Names() []string {
	if m == nil {
		return nil
	}
	m.txnMu.RLock()
	defer m.txnMu.RUnlock()
	m.ensurePopulated()

	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.cache))
	for n := range m.cache {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// IsEnabled reports whether a discovered skill is enabled for sessionID.
// Unknown skills are not enabled. Session IDs are trimmed so command and tool
// call sites cannot accidentally create distinct policies through whitespace.
func (m *Manager) IsEnabled(sessionID, name string) bool {
	if m == nil || strings.TrimSpace(name) == "" {
		return false
	}
	m.txnMu.RLock()
	defer m.txnMu.RUnlock()
	m.ensurePopulated()

	sessionID = strings.TrimSpace(sessionID)
	name = strings.TrimSpace(name)
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, exists := m.cache[name]; !exists {
		return false
	}
	_, disabled := m.disabledBySession[sessionID][name]
	return !disabled
}

// SetEnabled changes one skill's runtime availability for a session. found is
// false for an unknown skill; changed is false for an idempotent request.
func (m *Manager) SetEnabled(sessionID, name string, enabled bool) (changed, found bool) {
	if m == nil || strings.TrimSpace(name) == "" {
		return false, false
	}
	m.txnMu.Lock()
	defer m.txnMu.Unlock()
	m.ensurePopulated()

	sessionID = strings.TrimSpace(sessionID)
	name = strings.TrimSpace(name)
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, found = m.cache[name]; !found {
		return false, false
	}
	disabled := m.disabledBySession[sessionID]
	if enabled {
		if _, exists := disabled[name]; !exists {
			return false, true
		}
		delete(disabled, name)
		delete(m.disabledOwnersBySession[sessionID], name)
		if len(disabled) == 0 {
			delete(m.disabledBySession, sessionID)
			delete(m.disabledOwnersBySession, sessionID)
		}
		return true, true
	}
	if disabled == nil {
		disabled = make(map[string]struct{})
		m.disabledBySession[sessionID] = disabled
	}
	owners := m.disabledOwnersBySession[sessionID]
	if owners == nil {
		owners = make(map[string]legacyDisableOwner)
		m.disabledOwnersBySession[sessionID] = owners
	}
	owners[name] = m.legacyDisableOwnerLocked(name)
	if _, exists := disabled[name]; exists {
		return false, true
	}
	disabled[name] = struct{}{}
	return true, true
}

// SetAllEnabled applies a session availability overlay to every skill in the
// current catalog. Enabling all also clears stale overrides for skills that
// have since been removed. Newly discovered skills default to enabled.
func (m *Manager) SetAllEnabled(sessionID string, enabled bool) int {
	if m == nil {
		return 0
	}
	m.txnMu.Lock()
	defer m.txnMu.Unlock()
	m.ensurePopulated()

	sessionID = strings.TrimSpace(sessionID)
	m.mu.Lock()
	defer m.mu.Unlock()
	if enabled {
		changed := len(m.disabledBySession[sessionID])
		delete(m.disabledBySession, sessionID)
		delete(m.disabledOwnersBySession, sessionID)
		return changed
	}
	disabled := m.disabledBySession[sessionID]
	if disabled == nil {
		disabled = make(map[string]struct{}, len(m.cache))
		m.disabledBySession[sessionID] = disabled
	}
	owners := m.disabledOwnersBySession[sessionID]
	if owners == nil {
		owners = make(map[string]legacyDisableOwner, len(m.cache))
		m.disabledOwnersBySession[sessionID] = owners
	}
	changed := 0
	for name := range m.cache {
		owners[name] = m.legacyDisableOwnerLocked(name)
		if _, exists := disabled[name]; exists {
			continue
		}
		disabled[name] = struct{}{}
		changed++
	}
	return changed
}

func (m *Manager) legacyDisableOwnerLocked(name string) legacyDisableOwner {
	id := m.winners[name]
	if record, found := m.records[id]; found && record.skill != nil {
		return legacyDisableOwner{SkillID: id, Source: record.skill.Source}
	}
	if skill := m.cache[name]; skill != nil {
		return legacyDisableOwner{Source: skill.Source}
	}
	return legacyDisableOwner{}
}

// EnabledNames returns a stable list of skills available to one session.
func (m *Manager) EnabledNames(sessionID string) []string {
	if m == nil {
		return nil
	}
	m.txnMu.RLock()
	defer m.txnMu.RUnlock()
	m.ensurePopulated()

	sessionID = strings.TrimSpace(sessionID)
	m.mu.RLock()
	names := make([]string, 0, len(m.cache))
	for name := range m.cache {
		if _, disabled := m.disabledBySession[sessionID][name]; !disabled {
			names = append(names, name)
		}
	}
	m.mu.RUnlock()
	sort.Strings(names)
	return names
}

// Refresh discards the cache and re-scans all skill directories.
func (m *Manager) Refresh() {
	if m == nil {
		return
	}
	m.txnMu.Lock()
	defer m.txnMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cache = make(map[string]*Skill)
	m.populated = false
}

// RefreshSnapshot invalidates discovery and immediately returns the resulting
// authoritative effective view without changing the legacy Refresh signature.
func (m *Manager) RefreshSnapshot(sessionID string) (CatalogSnapshot, error) {
	if m == nil {
		return CatalogSnapshot{}, errors.New("nil skill manager")
	}
	m.txnMu.Lock()
	defer m.txnMu.Unlock()
	m.mu.Lock()
	m.cache = make(map[string]*Skill)
	m.populated = false
	m.mu.Unlock()
	return m.snapshotCurrent(strings.TrimSpace(sessionID))
}

// AddDir appends an additional source directory (e.g. from --add-dir CLI flag).
// The cache is invalidated so the next access re-scans.
func (m *Manager) AddDir(dir string, source SkillSource) {
	if m == nil {
		return
	}
	m.txnMu.Lock()
	defer m.txnMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dirs = append(m.dirs, DirSource{Dir: dir, Source: source})
	m.cache = make(map[string]*Skill)
	m.populated = false
}

// AddDirectories registers any number of additional source directories at
// once, classifying them as SourceProject. Duplicates (already-known dirs)
// are skipped. The cache is invalidated only when at least one new directory
// is actually added. Mirrors the TS skillsManager.addSearchDirs entrypoint
// used by the file-read auto-discovery hook.
func (m *Manager) AddDirectories(dirs []string) {
	if m == nil || len(dirs) == 0 {
		return
	}
	m.txnMu.Lock()
	defer m.txnMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.addDirectoriesLocked(dirs)
}

// AddDirectoriesAtGeneration performs dynamic same-workspace discovery only
// when expected still owns the Manager's project authority. Stale file tools
// from a prior workspace therefore cannot add their directories to the newly
// retargeted catalog.
func (m *Manager) AddDirectoriesAtGeneration(expected ProjectSourceGeneration, dirs []string) error {
	if m == nil {
		return errors.New("nil skill manager")
	}
	if err := expected.Validate(); err != nil {
		return err
	}
	if len(dirs) == 0 {
		return nil
	}
	m.txnMu.Lock()
	defer m.txnMu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.projectGeneration != expected {
		return projectGenerationChangedError(expected, m.projectGeneration)
	}
	m.addDirectoriesLocked(dirs)
	return nil
}

func (m *Manager) addDirectoriesLocked(dirs []string) {

	known := make(map[string]struct{}, len(m.dirs))
	for _, d := range m.dirs {
		if abs, err := filepath.Abs(d.Dir); err == nil {
			known[filepath.Clean(abs)] = struct{}{}
		} else {
			known[filepath.Clean(d.Dir)] = struct{}{}
		}
	}

	added := false
	for _, raw := range dirs {
		if raw == "" {
			continue
		}
		clean := filepath.Clean(raw)
		key := clean
		if abs, err := filepath.Abs(clean); err == nil {
			key = filepath.Clean(abs)
		}
		if _, dup := known[key]; dup {
			continue
		}
		known[key] = struct{}{}
		m.dirs = append(m.dirs, DirSource{Dir: clean, Source: SourceProject})
		added = true
	}

	if added {
		m.cache = make(map[string]*Skill)
		m.populated = false
	}
}

// advanceProjectGenerationLocked advances the project authority version.
// Caller must hold txnMu for writing and m.mu. Zero remains reserved for an
// unpinned compatibility request.
func (m *Manager) advanceProjectGenerationLocked() {
	m.projectGeneration++
	if m.projectGeneration == 0 {
		m.projectGeneration = 1
	}
}

// ActivateConditionalForPath completes nearby-directory activation by loading
// newly discovered skills immediately. Exact dynamic path/basename triggers
// are promoted by ActivateConditionalSkill, which records their names for
// prompt attachment consumers.
func (m *Manager) ActivateConditionalForPath(absPath string) {
	if m == nil || absPath == "" {
		return
	}
	m.txnMu.RLock()
	defer m.txnMu.RUnlock()
	// Ensure the cache is warm so subsequent name lookups for this path's
	// nearby directories are fast. AddDirectories already invalidates when
	// new dirs were appended; this just promotes the population to "now".
	m.ensurePopulated()
}

// ActivateConditionalForPathAtGeneration is the generation-fenced form used
// by model tool executions. It performs no discovery side effect after a
// workspace retarget.
func (m *Manager) ActivateConditionalForPathAtGeneration(expected ProjectSourceGeneration, absPath string) error {
	if m == nil {
		return errors.New("nil skill manager")
	}
	if err := expected.Validate(); err != nil {
		return err
	}
	if absPath == "" {
		return nil
	}
	m.txnMu.RLock()
	defer m.txnMu.RUnlock()
	m.mu.RLock()
	current := m.projectGeneration
	m.mu.RUnlock()
	if current != expected {
		return projectGenerationChangedError(expected, current)
	}
	m.ensurePopulated()
	return nil
}

// ActivateConditionalSkill promotes the named conditional skill for the
// session. FileReadTool invokes this for exact dynamic path/basename triggers.
func (m *Manager) ActivateConditionalSkill(name string) {
	if m == nil || name == "" {
		return
	}
	m.txnMu.RLock()
	defer m.txnMu.RUnlock()
	m.ensurePopulated()
	m.mu.Lock()
	if _, exists := m.cache[name]; exists {
		if m.activatedConditional == nil {
			m.activatedConditional = make(map[string]struct{})
		}
		m.activatedConditional[name] = struct{}{}
	}
	m.mu.Unlock()
}

// ActivateConditionalSkillAtGeneration prevents an old workspace tool from
// repopulating the process-global activation set after ApplyProjectSources has
// cleared it for the new authority.
func (m *Manager) ActivateConditionalSkillAtGeneration(expected ProjectSourceGeneration, name string) error {
	if m == nil {
		return errors.New("nil skill manager")
	}
	if err := expected.Validate(); err != nil {
		return err
	}
	if name == "" {
		return nil
	}
	m.txnMu.RLock()
	defer m.txnMu.RUnlock()
	m.mu.RLock()
	current := m.projectGeneration
	m.mu.RUnlock()
	if current != expected {
		return projectGenerationChangedError(expected, current)
	}
	m.ensurePopulated()
	m.mu.Lock()
	if _, exists := m.cache[name]; exists {
		if m.activatedConditional == nil {
			m.activatedConditional = make(map[string]struct{})
		}
		m.activatedConditional[name] = struct{}{}
	}
	m.mu.Unlock()
	return nil
}

// ActivatedConditionalSkillNames returns a stable sorted snapshot for prompt
// attachment consumers and tests.
func (m *Manager) ActivatedConditionalSkillNames() []string {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	names := make([]string, 0, len(m.activatedConditional))
	for name := range m.activatedConditional {
		names = append(names, name)
	}
	m.mu.RUnlock()
	sort.Strings(names)
	return names
}

// ensurePopulated scans directories if not yet done.
func (m *Manager) ensurePopulated() {
	m.mu.RLock()
	populated := m.populated
	m.mu.RUnlock()

	if populated {
		// Cheap stat-based liveness check: only re-scan when the underlying
		// filesystem has actually changed since the last populate(). This
		// avoids the dependency on fsnotify while still catching newly
		// created skill directories within one Get/All/Names call.
		if !m.dirsChanged() {
			return
		}
		m.mu.Lock()
		m.cache = make(map[string]*Skill)
		m.populated = false
		m.mu.Unlock()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock.
	if m.populated {
		return
	}

	m.populate()
	m.populated = true
}

// snapshotDir captures a fingerprint of a single directory: its modtime
// plus the (name, modtime, size, isDir) tuple for each direct entry.
// Returns the zero value when the dir cannot be stat'd or read.
func (m *Manager) snapshotDir(dir string) dirSnapshot {
	info, err := os.Stat(dir)
	if err != nil {
		return dirSnapshot{}
	}
	snap := dirSnapshot{
		dirModTime: info.ModTime(),
		entries:    make(map[string]entryFingerprint),
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return snap
	}
	for _, e := range entries {
		fi, err := e.Info()
		if err != nil {
			continue
		}
		fp := entryFingerprint{
			modTime: fi.ModTime(),
			size:    fi.Size(),
			isDir:   e.IsDir(),
		}
		// For directory entries, also fingerprint the SKILL.md inside so we
		// notice writes to existing skill folders even when the parent dir
		// mtime is unchanged.
		if e.IsDir() {
			child := filepath.Join(dir, e.Name(), "SKILL.md")
			if cinfo, cerr := os.Stat(child); cerr == nil {
				fp.modTime = cinfo.ModTime()
				fp.size = cinfo.Size()
			}
		}
		snap.entries[e.Name()] = fp
	}
	return snap
}

// dirsChanged reports whether any registered dir's current fingerprint
// differs from the last recorded snapshot. Cheap: one Stat + one ReadDir
// per dir per call, plus one Stat per direct entry.
func (m *Manager) dirsChanged() bool {
	m.mu.RLock()
	dirs := make([]DirSource, len(m.dirs))
	copy(dirs, m.dirs)
	prev := make(map[string]dirSnapshot, len(m.dirSnapshots))
	for k, v := range m.dirSnapshots {
		prev[k] = v
	}
	m.mu.RUnlock()

	for _, ds := range dirs {
		current := m.snapshotDir(ds.Dir)
		old, ok := prev[ds.Dir]
		if !ok {
			// Newly stat-able dir (or first-time scan) → treat as changed
			// only when something is actually present.
			if !current.dirModTime.IsZero() && len(current.entries) > 0 {
				return true
			}
			continue
		}
		if !current.dirModTime.Equal(old.dirModTime) {
			return true
		}
		if len(current.entries) != len(old.entries) {
			return true
		}
		for name, fp := range current.entries {
			oldfp, ok := old.entries[name]
			if !ok {
				return true
			}
			if fp.size != oldfp.size || fp.isDir != oldfp.isDir || !fp.modTime.Equal(oldfp.modTime) {
				return true
			}
		}
	}
	return false
}

// populate scans all skill directories and atomically replaces both the
// stable-ID registry and its legacy effective-name cache.
// Caller must hold m.mu write lock.
func (m *Manager) populate() {
	seen := make(map[string]bool)
	nextRecords := make(map[SkillID]catalogRecord)

	if m.dirSnapshots == nil {
		m.dirSnapshots = make(map[string]dirSnapshot)
	}
	for _, ds := range m.dirs {
		m.dirSnapshots[ds.Dir] = m.snapshotDir(ds.Dir)
	}

	for priority, ds := range m.dirs {
		if err := ds.Source.Validate(); err != nil {
			continue
		}
		entries, err := os.ReadDir(ds.Dir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			name := entry.Name()

			if !entry.IsDir() {
				if entry.Type()&os.ModeSymlink != 0 {
					target := filepath.Join(ds.Dir, name)
					if info, err := os.Stat(target); err == nil && info.IsDir() {
					} else {
						continue
					}
				} else {
					continue
				}
			}

			skillFile := filepath.Join(ds.Dir, name, "SKILL.md")
			if _, err := os.Stat(skillFile); err != nil {
				continue
			}
			skill := m.loadSkillFile(skillFile, name, ds.Source, seen)
			if skill == nil || strings.TrimSpace(skill.Name) == "" {
				continue
			}
			locator, err := CanonicalSkillLocator(skill.Source, skill.FilePath)
			if err != nil {
				continue
			}
			id, err := ComputeSkillID(skill.Source, locator)
			if err != nil {
				continue
			}
			if _, duplicate := nextRecords[id]; duplicate {
				continue
			}
			nextRecords[id] = catalogRecord{
				skill: cloneManagerSkill(skill), id: id, locator: locator,
				digest: ComputeSkillDigest(skill.RawContent), priority: priority,
			}
		}
	}

	// MCP entries retain their adapter-produced identity. They rank below local
	// directories, preserving the legacy local-over-remote winner rule without
	// discarding same-name remote alternatives.
	if m.mcpPrompts != nil {
		for _, input := range m.mcpPrompts.inputsSnapshot() {
			if err := input.Validate(); err != nil || input.Skill == nil || strings.TrimSpace(input.Skill.Name) == "" {
				continue
			}
			if _, duplicate := nextRecords[input.ID]; duplicate {
				continue
			}
			nextRecords[input.ID] = catalogRecord{
				skill: cloneManagerSkill(input.Skill), id: input.ID, locator: input.Locator,
				digest: input.Digest, priority: len(m.dirs),
			}
		}
	}

	nextWinners := make(map[string]SkillID)
	for id, record := range nextRecords {
		winnerID, exists := nextWinners[record.skill.Name]
		if !exists || catalogRecordPrecedes(record, nextRecords[winnerID]) {
			nextWinners[record.skill.Name] = id
		}
	}
	nextCache := make(map[string]*Skill, len(nextWinners))
	for name, id := range nextWinners {
		nextCache[name] = cloneManagerSkill(nextRecords[id].skill)
	}

	m.records = nextRecords
	m.winners = nextWinners
	m.cache = nextCache
}

func catalogRecordPrecedes(candidate, current catalogRecord) bool {
	if candidate.priority != current.priority {
		return candidate.priority < current.priority
	}
	return candidate.id < current.id
}

// ToggleProjectVisibility is the single project-persistent interactive
// mutation boundary. It compares the UI's catalog revision, persists by stable
// ID, installs the live effective state, and compensates persistence if live
// installation fails.
func (m *Manager) ToggleProjectVisibility(sessionID string, id SkillID, expected CatalogRevision) (ProjectVisibilityToggleResult, error) {
	request := ProjectVisibilityToggleRequest{
		SessionID: sessionID, SkillID: id, ExpectedRevision: expected,
	}
	if err := request.Validate(); err != nil {
		return ProjectVisibilityToggleResult{}, err
	}
	if m == nil {
		return ProjectVisibilityToggleResult{}, errors.New("nil skill manager")
	}

	m.txnMu.Lock()
	defer m.txnMu.Unlock()
	initial, err := m.snapshotCurrent(sessionID)
	if err != nil {
		return ProjectVisibilityToggleResult{}, err
	}
	if initial.Revision != expected {
		return projectToggleResult(request, initial, ProjectVisibilityToggleRejected, ProjectVisibilityToggleReasonStaleRevision), nil
	}
	row, found := initial.Find(id)
	if !found {
		return projectToggleResult(request, initial, ProjectVisibilityToggleRejected, ProjectVisibilityToggleReasonUnknownSkill), nil
	}

	m.mu.RLock()
	view := m.views[sessionID]
	_, storedSessionOverride := view.overrides.Session[id]
	_, legacySessionOverride := m.disabledBySession[sessionID][row.Name]
	store := m.overrideStore
	m.mu.RUnlock()
	if storedSessionOverride || legacySessionOverride {
		return projectToggleResult(request, initial, ProjectVisibilityToggleRejected, ProjectVisibilityToggleReasonSessionOverride), nil
	}
	if !row.Mutable {
		return projectToggleResult(request, initial, ProjectVisibilityToggleRejected, ProjectVisibilityToggleReasonReadOnly), nil
	}
	if store == nil || !view.overrides.ProjectRevision.Valid() {
		result := projectToggleResult(request, initial, ProjectVisibilityToggleRejected, ProjectVisibilityToggleReasonPersistenceFailed)
		return result, ErrSkillOverrideStoreMissing
	}

	next := nextProjectToggle(row, view.overrides.Project[id])
	restore, persistErr := store.CompareAndSetProject(view.overrides.ProjectRevision, id, &next)
	if persistErr != nil {
		if errors.Is(persistErr, ErrOverrideRevisionConflict) {
			refreshed, refreshErr := m.refreshAuthoritativeSnapshot(sessionID)
			if refreshErr != nil {
				result := projectToggleResult(request, initial, ProjectVisibilityToggleDegraded, ProjectVisibilityToggleReasonAuthoritativeRefresh)
				return result, errors.Join(persistErr, refreshErr)
			}
			reason := ProjectVisibilityToggleReasonPersistenceFailed
			if refreshed.Revision != expected {
				reason = ProjectVisibilityToggleReasonStaleRevision
			}
			result := projectToggleResult(request, refreshed, ProjectVisibilityToggleRejected, reason)
			if reason == ProjectVisibilityToggleReasonStaleRevision {
				return result, nil
			}
			return result, persistErr
		}
		return projectToggleResult(request, initial, ProjectVisibilityToggleRejected, ProjectVisibilityToggleReasonPersistenceFailed), persistErr
	}

	committed, applyErr := m.snapshotCurrent(sessionID)
	if applyErr == nil && projectToggleApplied(committed, id, next) {
		return projectToggleResult(request, committed, ProjectVisibilityToggleCommitted, ProjectVisibilityToggleReasonNone), nil
	}
	if applyErr == nil {
		applyErr = errors.New("project visibility did not become the effective live state")
	}

	rollbackErr := store.RestoreProject(restore)
	if rollbackErr != nil {
		authoritative, refreshErr := m.refreshAuthoritativeSnapshot(sessionID)
		if refreshErr != nil {
			result := projectToggleResult(request, initial, ProjectVisibilityToggleDegraded, ProjectVisibilityToggleReasonAuthoritativeRefresh)
			return result, errors.Join(applyErr, rollbackErr, refreshErr)
		}
		result := projectToggleResult(request, authoritative, ProjectVisibilityToggleDegraded, ProjectVisibilityToggleReasonRollbackFailed)
		return result, errors.Join(applyErr, rollbackErr)
	}

	rolledBack, refreshErr := m.refreshAuthoritativeSnapshot(sessionID)
	if refreshErr != nil {
		result := projectToggleResult(request, initial, ProjectVisibilityToggleDegraded, ProjectVisibilityToggleReasonAuthoritativeRefresh)
		return result, errors.Join(applyErr, refreshErr)
	}
	result := projectToggleResult(request, rolledBack, ProjectVisibilityToggleRejected, ProjectVisibilityToggleReasonLiveApplyRolledBack)
	return result, applyErr
}

func nextProjectToggle(row EffectiveSkill, previous VisibilityOverride) VisibilityOverride {
	if row.Visibility == VisibilityOff {
		visibility := VisibilityAuto
		if previous.Visibility == VisibilityOff {
			visibility = previous.RestoreVisibility()
		}
		return VisibilityOverride{SkillID: row.ID, Scope: SkillScopeProject, Visibility: visibility}
	}
	remembered := VisibilityAuto
	if row.Visibility.IsNonOff() {
		remembered = row.Visibility
	}
	return VisibilityOverride{
		SkillID: row.ID, Scope: SkillScopeProject, Visibility: VisibilityOff, LastNonOff: &remembered,
	}
}

func projectToggleApplied(snapshot CatalogSnapshot, id SkillID, next VisibilityOverride) bool {
	row, found := snapshot.Find(id)
	return found && row.Visibility == next.Visibility && row.VisibilitySource == SkillScopeProject
}

func projectToggleResult(request ProjectVisibilityToggleRequest, snapshot CatalogSnapshot, outcome ProjectVisibilityToggleOutcome, reason ProjectVisibilityToggleReason) ProjectVisibilityToggleResult {
	result := ProjectVisibilityToggleResult{
		Outcome: outcome, Reason: reason, RequestedSkillID: request.SkillID,
		ObservedRevision: request.ExpectedRevision, CurrentRevision: snapshot.Revision,
		Snapshot: snapshot.Clone(),
	}
	if row, found := snapshot.Find(request.SkillID); found {
		copy := row
		result.Skill = &copy
	}
	return result
}

func (m *Manager) refreshAuthoritativeSnapshot(sessionID string) (CatalogSnapshot, error) {
	var combined error
	for attempt := 0; attempt < 3; attempt++ {
		snapshot, err := m.snapshotCurrent(sessionID)
		if err == nil {
			return snapshot, nil
		}
		combined = errors.Join(combined, err)
	}
	return CatalogSnapshot{}, combined
}

func (m *Manager) snapshotCurrent(sessionID string) (CatalogSnapshot, error) {
	for {
		m.ensurePopulated()
		store := m.currentOverrideStore()
		overrides := emptyOverrideSnapshot()
		var err error
		if store != nil {
			overrides, err = store.Snapshot(sessionID)
			if err != nil {
				return CatalogSnapshot{}, err
			}
		}

		m.mu.Lock()
		if !m.populated {
			m.mu.Unlock()
			continue
		}
		snapshot, buildErr := m.buildCatalogSnapshotLocked(sessionID, overrides)
		m.mu.Unlock()
		return snapshot, buildErr
	}
}

// buildCatalogSnapshotLocked constructs all values before installing the view,
// so validation errors never publish a partial revision.
func (m *Manager) buildCatalogSnapshotLocked(sessionID string, overrides OverrideSnapshot) (CatalogSnapshot, error) {
	previous := m.views[sessionID]
	nextVersions := make(map[SkillID]catalogVersionState, len(previous.versions)+len(m.records))
	for id, version := range previous.versions {
		nextVersions[id] = version
	}

	ids := make([]SkillID, 0, len(m.records))
	for id := range m.records {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	effective := make([]EffectiveSkill, 0, len(ids))
	for _, id := range ids {
		record := m.records[id]
		userOverride := overridePointer(overrides.User, id)
		projectOverride := overridePointer(overrides.Project, id)
		sessionOverride := overridePointer(overrides.Session, id)
		if _, disabled := m.disabledBySession[sessionID][record.skill.Name]; disabled {
			legacyOff := VisibilityOverride{SkillID: id, Scope: SkillScopeSession, Visibility: VisibilityOff}
			sessionOverride = &legacyOff
		}
		managedOverride, managed := overrides.Managed[id]
		decision, err := EvaluateCatalogPolicy(CatalogPolicyInput{
			SkillID:             id,
			DefaultModelVisible: defaultModelVisible(record.skill), DefaultUserInvocable: true,
			FrontmatterDisableModelInvocation: record.skill.DisableModelInvocation,
			FrontmatterUserInvocable:          cloneBoolPointer(record.skill.UserInvocable),
			UserOverride:                      userOverride, ProjectOverride: projectOverride, SessionOverride: sessionOverride,
			ManagedDeny:     managed && managedOverride.Visibility == VisibilityOff,
			ManagedReadOnly: managed || record.skill.Source == SourceManaged,
		})
		if err != nil {
			return CatalogSnapshot{}, err
		}

		row := EffectiveSkill{
			ID: id, Name: record.skill.Name, Summary: strings.TrimSpace(record.skill.EffectiveDescription()),
			SummaryGenerated: record.skill.HasGeneratedDescription && strings.TrimSpace(record.skill.WhenToUse) == "",
			Source:           record.skill.Source, Locator: record.locator, Digest: record.digest, Revision: 1,
			Visibility: decision.Visibility, VisibilitySource: decision.VisibilitySource,
			ModelVisible: decision.ModelVisible, DescriptionVisible: decision.DescriptionVisible,
			UserInvocable: decision.UserInvocable, Executable: decision.Executable,
			Mutable: decision.Mutable, ReadOnlyReason: string(decision.ReadOnlyReason),
		}
		if winner := m.winners[row.Name]; winner != "" && winner != id {
			row.ShadowedBy = winner
			row.ModelVisible = false
			row.DescriptionVisible = false
			row.UserInvocable = false
			row.Executable = false
		}
		if !row.Mutable && strings.TrimSpace(row.ReadOnlyReason) == "" {
			row.ReadOnlyReason = string(CatalogPolicyReasonManagedReadOnly)
		}

		fingerprint, err := SkillRevisionFingerprint(row)
		if err != nil {
			return CatalogSnapshot{}, err
		}
		version := nextVersions[id]
		switch {
		case version.revision == 0:
			version.revision = 1
		case !version.present || version.fingerprint != fingerprint:
			version.revision++
		}
		version.fingerprint = fingerprint
		version.present = true
		nextVersions[id] = version
		row.Revision = version.revision
		effective = append(effective, row)
	}
	for id, version := range nextVersions {
		if _, present := m.records[id]; !present {
			version.present = false
			nextVersions[id] = version
		}
	}

	revision := previous.snapshot.Revision
	if revision == 0 {
		revision = 1
	} else if !slices.Equal(previous.snapshot.Skills, effective) {
		revision++
	}
	snapshot, err := NewCatalogSnapshot(revision, effective)
	if err != nil {
		return CatalogSnapshot{}, err
	}
	if m.views == nil {
		m.views = make(map[string]catalogView)
	}
	m.views[sessionID] = catalogView{snapshot: snapshot.Clone(), versions: nextVersions, overrides: overrides}
	return snapshot.Clone(), nil
}

func emptyOverrideSnapshot() OverrideSnapshot {
	return OverrideSnapshot{
		User: make(map[SkillID]VisibilityOverride), Project: make(map[SkillID]VisibilityOverride),
		Managed: make(map[SkillID]VisibilityOverride), Session: make(map[SkillID]VisibilityOverride),
	}
}

func overridePointer(layer map[SkillID]VisibilityOverride, id SkillID) *VisibilityOverride {
	override, exists := layer[id]
	if !exists {
		return nil
	}
	copy := cloneVisibilityOverride(override)
	return &copy
}

func cloneBoolPointer(value *bool) *bool {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func defaultModelVisible(skill *Skill) bool {
	if skill == nil || skill.Source == "builtin" {
		return false
	}
	switch skill.Source {
	case SourceBundled, SourceProject, SourceUser, SourceManaged, SourceCommandsLegacy:
		return true
	default:
		return skill.HasUserSpecifiedDescription || strings.TrimSpace(skill.WhenToUse) != ""
	}
}

func (m *Manager) currentOverrideStore() OverrideStore {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.overrideStore
}

func cloneManagerSkill(skill *Skill) *Skill {
	return cloneMCPSkill(skill)
}

// loadSkillFile reads and parses a single skill .md file.
// Returns nil if the file cannot be read or if it's a duplicate (symlink).
func (m *Manager) loadSkillFile(path, defaultName string, source SkillSource, seen map[string]bool) *Skill {
	// Symlink deduplication: resolve to real path
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		realPath = path
	}
	realPath, err = filepath.Abs(realPath)
	if err != nil {
		realPath = path
	}

	if seen[realPath] {
		return nil // already loaded via another path
	}
	seen[realPath] = true

	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	rawContent := string(data)

	// Parse frontmatter
	parsed := parseFrontmatter(rawContent, path)

	absPath, _ := filepath.Abs(path)

	skill := &Skill{
		Name:                    defaultName,
		Description:             fmt.Sprintf("Skill: %s", defaultName),
		HasGeneratedDescription: true,
		Source:                  source,
		FilePath:                absPath,
		SkillDir:                filepath.Dir(absPath),
		RawContent:              rawContent,
		Content:                 parsed.Content,
		ContentLength:           len(parsed.Content),
	}

	// Apply frontmatter fields (may override Name, Description, etc.)
	applyFrontmatter(skill, parsed.Frontmatter)

	return skill
}
