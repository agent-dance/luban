package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"github.com/agent-dance/luban/brand"
)

const skillOverridesSettingsKey = "skillOverrides"

var (
	// ErrOverrideRevisionConflict means that a persistent settings file changed
	// after the caller observed it. Callers must refresh instead of overwriting
	// the newer file.
	ErrOverrideRevisionConflict = errors.New("skill override revision conflict")
	// ErrManagedOverrideReadOnly prevents lower scopes from mutating a skill
	// whose visibility is owned by managed policy.
	ErrManagedOverrideReadOnly = errors.New("managed skill override is read-only")
	// ErrUnsupportedOverrideScope identifies default/frontmatter/managed writes;
	// those scopes are inputs to policy evaluation, not writable user settings.
	ErrUnsupportedOverrideScope = errors.New("unsupported writable skill override scope")
	// ErrInvalidOverrideSession identifies a missing or padded session ID on a
	// session-scoped mutation.
	ErrInvalidOverrideSession = errors.New("invalid skill override session ID")
)

// OverrideStoreRevision is an opaque fingerprint of one complete settings
// file. It is intentionally distinct from CatalogRevision: the catalog and
// settings file have independent lifecycles.
type OverrideStoreRevision string

// Valid reports whether revision can be used for a compare-and-swap write.
func (revision OverrideStoreRevision) Valid() bool { return revision != "" }

// OverrideStorePaths identifies the existing settings trees used by skill
// overrides. No skill-specific configuration directory is introduced.
type OverrideStorePaths struct {
	UserSettings    string
	ProjectSettings string
}

// DefaultOverrideStorePaths returns the canonical branded settings paths.
// Existing legacy configuration remains readable by its existing owners; new
// skill override writes always target the current branded settings files.
func DefaultOverrideStorePaths(cwd string) (OverrideStorePaths, error) {
	if strings.TrimSpace(cwd) == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return OverrideStorePaths{}, fmt.Errorf("skill overrides: determine working directory: %w", err)
		}
	}
	projectPath, err := filepath.Abs(filepath.Join(cwd, brand.ConfigDirName, "settings.json"))
	if err != nil {
		return OverrideStorePaths{}, fmt.Errorf("skill overrides: resolve project settings: %w", err)
	}
	userPath := filepath.Join(brand.UserConfigDir(), "settings.json")
	userPath, err = filepath.Abs(userPath)
	if err != nil {
		return OverrideStorePaths{}, fmt.Errorf("skill overrides: resolve user settings: %w", err)
	}
	return OverrideStorePaths{UserSettings: userPath, ProjectSettings: projectPath}, nil
}

// ResolvedOverride reports both the selected override and the layer that owns
// it. Managed values are visible but immutable.
type ResolvedOverride struct {
	Override VisibilityOverride
	Source   SkillScope
	Mutable  bool
}

// OverrideSnapshot is a defensive copy of every storage layer at one read.
// UserRevision and ProjectRevision fingerprint the complete corresponding
// settings files, including unrelated settings, so a CAS cannot erase a
// concurrent settings edit.
type OverrideSnapshot struct {
	User            map[SkillID]VisibilityOverride
	Project         map[SkillID]VisibilityOverride
	Managed         map[SkillID]VisibilityOverride
	Session         map[SkillID]VisibilityOverride
	UserRevision    OverrideStoreRevision
	ProjectRevision OverrideStoreRevision
}

// Resolve selects the highest explicit override. Frontmatter and default
// policy are intentionally outside the store and are evaluated only when no
// stored override exists.
func (snapshot OverrideSnapshot) Resolve(id SkillID) (ResolvedOverride, bool) {
	for _, layer := range []struct {
		scope     SkillScope
		overrides map[SkillID]VisibilityOverride
		mutable   bool
	}{
		{SkillScopeManaged, snapshot.Managed, false},
		{SkillScopeSession, snapshot.Session, true},
		{SkillScopeProject, snapshot.Project, true},
		{SkillScopeUser, snapshot.User, true},
	} {
		if override, ok := layer.overrides[id]; ok {
			return ResolvedOverride{
				Override: cloneVisibilityOverride(override),
				Source:   layer.scope,
				Mutable:  layer.mutable,
			}, true
		}
	}
	return ResolvedOverride{}, false
}

// SessionOverrideLayer makes session lifetime an injectable concern. A layer
// must return defensive snapshots and isolate records by session ID.
type SessionOverrideLayer interface {
	Snapshot(sessionID string) (map[SkillID]VisibilityOverride, error)
	Set(sessionID string, override VisibilityOverride) error
	Reset(sessionID string, id SkillID) error
}

// MemorySessionOverrideLayer is the default race-safe session implementation.
type MemorySessionOverrideLayer struct {
	mu        sync.RWMutex
	bySession map[string]map[SkillID]VisibilityOverride
}

// NewMemorySessionOverrideLayer creates an empty session layer.
func NewMemorySessionOverrideLayer() *MemorySessionOverrideLayer {
	return &MemorySessionOverrideLayer{bySession: make(map[string]map[SkillID]VisibilityOverride)}
}

// Snapshot returns one session's records without sharing mutable map or pointer
// storage with the caller. An empty ID means that no session layer is active.
func (layer *MemorySessionOverrideLayer) Snapshot(sessionID string) (map[SkillID]VisibilityOverride, error) {
	if sessionID == "" {
		return make(map[SkillID]VisibilityOverride), nil
	}
	if err := validateOverrideSessionID(sessionID); err != nil {
		return nil, err
	}
	layer.mu.RLock()
	defer layer.mu.RUnlock()
	return cloneOverrideMap(layer.bySession[sessionID]), nil
}

// ReplaceSession atomically replaces the complete session overlay. The input
// is normalized before the write lock is acquired, so readers observe either
// the previous map or the complete replacement and never a partially restored
// resume state.
func (layer *MemorySessionOverrideLayer) ReplaceSession(sessionID string, replacements map[SkillID]VisibilityOverride) error {
	if err := validateOverrideSessionID(sessionID); err != nil {
		return err
	}
	next, err := normalizeOverrideMap(replacements, SkillScopeSession)
	if err != nil {
		return fmt.Errorf("skill overrides: invalid session replacement: %w", err)
	}

	layer.mu.Lock()
	defer layer.mu.Unlock()
	if layer.bySession == nil {
		layer.bySession = make(map[string]map[SkillID]VisibilityOverride)
	}
	if len(next) == 0 {
		delete(layer.bySession, sessionID)
		return nil
	}
	layer.bySession[sessionID] = next
	return nil
}

// Set replaces one session-local record.
func (layer *MemorySessionOverrideLayer) Set(sessionID string, override VisibilityOverride) error {
	if err := validateOverrideSessionID(sessionID); err != nil {
		return err
	}
	if override.Scope != SkillScopeSession {
		return fmt.Errorf("%w: %s", ErrUnsupportedOverrideScope, override.Scope)
	}
	if err := override.Validate(); err != nil {
		return fmt.Errorf("skill overrides: invalid session override: %w", err)
	}
	layer.mu.Lock()
	defer layer.mu.Unlock()
	if layer.bySession == nil {
		layer.bySession = make(map[string]map[SkillID]VisibilityOverride)
	}
	if layer.bySession[sessionID] == nil {
		layer.bySession[sessionID] = make(map[SkillID]VisibilityOverride)
	}
	layer.bySession[sessionID][override.SkillID] = cloneVisibilityOverride(override)
	return nil
}

// Reset removes one session-local record without affecting other sessions.
func (layer *MemorySessionOverrideLayer) Reset(sessionID string, id SkillID) error {
	if err := validateOverrideSessionID(sessionID); err != nil {
		return err
	}
	if err := id.Validate(); err != nil {
		return err
	}
	layer.mu.Lock()
	defer layer.mu.Unlock()
	delete(layer.bySession[sessionID], id)
	if len(layer.bySession[sessionID]) == 0 {
		delete(layer.bySession, sessionID)
	}
	return nil
}

// OverrideStore is the storage boundary consumed by the revisioned Manager.
// CompareAndSetProject and RestoreProject form the persistent half of the
// Manager-owned persist/apply/compensate transaction.
type OverrideStore interface {
	Snapshot(sessionID string) (OverrideSnapshot, error)
	Set(sessionID string, override VisibilityOverride) error
	Toggle(sessionID string, scope SkillScope, id SkillID) (VisibilityOverride, error)
	Reset(sessionID string, scope SkillScope, id SkillID) error
	CompareAndSetProject(expected OverrideStoreRevision, id SkillID, next *VisibilityOverride) (ProjectOverrideRestore, error)
	RestoreProject(restore ProjectOverrideRestore) error
}

type atomicOverrideWriter func(path string, data []byte) error

// FileOverrideStore stores user/project overrides inside their existing
// settings.json files and composes injected managed/session layers.
type FileOverrideStore struct {
	stateMu                 sync.RWMutex
	paths                   OverrideStorePaths
	managed                 map[SkillID]VisibilityOverride
	session                 SessionOverrideLayer
	atomicWrite             atomicOverrideWriter
	removeFile              func(string) error
	preparedUserRevision    OverrideStoreRevision
	preparedProjectRevision OverrideStoreRevision
	preparedRevisionPending bool
}

// NewFileOverrideStore creates a store using the canonical branded paths.
func NewFileOverrideStore(cwd string, managed map[SkillID]VisibilityOverride, session SessionOverrideLayer) (*FileOverrideStore, error) {
	paths, err := DefaultOverrideStorePaths(cwd)
	if err != nil {
		return nil, err
	}
	return NewFileOverrideStoreAt(paths, managed, session)
}

// NewFileOverrideStoreAt creates a store with explicit settings paths. It is
// useful for runtime composition and isolated tests; both paths still point to
// ordinary settings.json files rather than a separate skill store.
func NewFileOverrideStoreAt(paths OverrideStorePaths, managed map[SkillID]VisibilityOverride, session SessionOverrideLayer) (*FileOverrideStore, error) {
	var err error
	paths.UserSettings, err = normalizeOverridePath(paths.UserSettings)
	if err != nil {
		return nil, fmt.Errorf("skill overrides: user settings path: %w", err)
	}
	paths.ProjectSettings, err = normalizeOverridePath(paths.ProjectSettings)
	if err != nil {
		return nil, fmt.Errorf("skill overrides: project settings path: %w", err)
	}
	managedCopy, err := normalizeOverrideMap(managed, SkillScopeManaged)
	if err != nil {
		return nil, fmt.Errorf("skill overrides: managed layer: %w", err)
	}
	if isNilLike(session) {
		session = NewMemorySessionOverrideLayer()
	}
	return &FileOverrideStore{
		paths:       paths,
		managed:     managedCopy,
		session:     session,
		atomicWrite: atomicWriteOverrideSettings,
		removeFile:  os.Remove,
	}, nil
}

// isNilLike handles interfaces containing typed nil pointers (and other
// nil-capable kinds). Interface equality with nil alone is insufficient at a
// dependency-injection boundary and would defer the failure to the first
// session Snapshot call.
func isNilLike(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

// Snapshot reads persistent layers afresh so external settings changes are
// visible and supplies independent revision tokens for later CAS writes.
func (store *FileOverrideStore) Snapshot(sessionID string) (OverrideSnapshot, error) {
	store.stateMu.Lock()
	defer store.stateMu.Unlock()
	user, userRevision, err := loadOverrideLayerLocked(store.paths.UserSettings, SkillScopeUser)
	if err != nil {
		return OverrideSnapshot{}, err
	}
	project, projectRevision, err := loadOverrideLayerLocked(store.paths.ProjectSettings, SkillScopeProject)
	if err != nil {
		return OverrideSnapshot{}, err
	}
	if err := store.acceptPreparedRevisionsLocked(userRevision, projectRevision); err != nil {
		return OverrideSnapshot{}, err
	}
	session, err := store.session.Snapshot(sessionID)
	if err != nil {
		return OverrideSnapshot{}, err
	}
	session, err = normalizeOverrideMap(session, SkillScopeSession)
	if err != nil {
		return OverrideSnapshot{}, fmt.Errorf("skill overrides: session layer: %w", err)
	}
	return OverrideSnapshot{
		User:            user,
		Project:         project,
		Managed:         cloneOverrideMap(store.managed),
		Session:         session,
		UserRevision:    userRevision,
		ProjectRevision: projectRevision,
	}, nil
}

// Set stores a direct four-state override. Turning a non-off record off
// remembers its previous state; a first off record remembers auto.
func (store *FileOverrideStore) Set(sessionID string, override VisibilityOverride) error {
	store.stateMu.Lock()
	defer store.stateMu.Unlock()
	if err := store.validateMutable(override.SkillID, override.Scope); err != nil {
		return err
	}
	if err := override.Validate(); err != nil {
		return fmt.Errorf("skill overrides: invalid override: %w", err)
	}
	if err := store.verifyPreparedRevisionsLocked(); err != nil {
		return err
	}
	if override.Scope == SkillScopeSession {
		current, err := store.session.Snapshot(sessionID)
		if err != nil {
			return err
		}
		next, err := prepareStoredOverride(current[override.SkillID], override)
		if err != nil {
			return err
		}
		return store.session.Set(sessionID, next)
	}
	_, err := store.mutatePersistent(override.Scope, "", override.SkillID, &override)
	return err
}

// Toggle switches one stored scope between off and its remembered non-off
// value. A missing record is treated as auto before being turned off.
func (store *FileOverrideStore) Toggle(sessionID string, scope SkillScope, id SkillID) (VisibilityOverride, error) {
	store.stateMu.Lock()
	defer store.stateMu.Unlock()
	if err := store.validateMutable(id, scope); err != nil {
		return VisibilityOverride{}, err
	}
	if err := store.verifyPreparedRevisionsLocked(); err != nil {
		return VisibilityOverride{}, err
	}
	if scope == SkillScopeSession {
		current, err := store.session.Snapshot(sessionID)
		if err != nil {
			return VisibilityOverride{}, err
		}
		next := toggledOverride(id, scope, current[id])
		if err := store.session.Set(sessionID, next); err != nil {
			return VisibilityOverride{}, err
		}
		return cloneVisibilityOverride(next), nil
	}
	return store.togglePersistent(scope, id)
}

// Reset deletes one explicit record so the next lower layer is inherited.
func (store *FileOverrideStore) Reset(sessionID string, scope SkillScope, id SkillID) error {
	store.stateMu.Lock()
	defer store.stateMu.Unlock()
	if err := store.validateMutable(id, scope); err != nil {
		return err
	}
	if err := store.verifyPreparedRevisionsLocked(); err != nil {
		return err
	}
	if scope == SkillScopeSession {
		return store.session.Reset(sessionID, id)
	}
	_, err := store.mutatePersistent(scope, "", id, nil)
	return err
}

// ProjectOverrideRestore is an opaque compensation receipt. Its contents are
// deliberately private because they may contain unrelated settings values.
type ProjectOverrideRestore struct {
	path           string
	before         []byte
	beforeExists   bool
	beforeRevision OverrideStoreRevision
	afterRevision  OverrideStoreRevision
}

// BeforeRevision reports the settings token observed by the successful CAS.
func (restore ProjectOverrideRestore) BeforeRevision() OverrideStoreRevision {
	return restore.beforeRevision
}

// AfterRevision reports the token that must still be current before restore.
func (restore ProjectOverrideRestore) AfterRevision() OverrideStoreRevision {
	return restore.afterRevision
}

// CompareAndSetProject changes one project record only if the complete project
// settings file still matches expected. next == nil resets to inherited.
func (store *FileOverrideStore) CompareAndSetProject(expected OverrideStoreRevision, id SkillID, next *VisibilityOverride) (ProjectOverrideRestore, error) {
	store.stateMu.Lock()
	defer store.stateMu.Unlock()
	if !expected.Valid() {
		return ProjectOverrideRestore{}, fmt.Errorf("%w: empty expected revision", ErrOverrideRevisionConflict)
	}
	if err := store.validateMutable(id, SkillScopeProject); err != nil {
		return ProjectOverrideRestore{}, err
	}
	if next != nil {
		if next.SkillID != id || next.Scope != SkillScopeProject {
			return ProjectOverrideRestore{}, fmt.Errorf("skill overrides: project CAS target does not match override")
		}
		if err := next.Validate(); err != nil {
			return ProjectOverrideRestore{}, err
		}
	}
	if err := store.verifyPreparedRevisionsLocked(); err != nil {
		return ProjectOverrideRestore{}, err
	}

	pathLock := overrideLockForPath(store.paths.ProjectSettings)
	pathLock.Lock()
	defer pathLock.Unlock()
	document, current, exists, currentRevision, err := readOverrideSettings(store.paths.ProjectSettings, SkillScopeProject)
	if err != nil {
		return ProjectOverrideRestore{}, err
	}
	if currentRevision != expected {
		return ProjectOverrideRestore{}, fmt.Errorf("%w: expected %s, current %s", ErrOverrideRevisionConflict, expected, currentRevision)
	}
	before := append([]byte(nil), document.raw...)
	if next != nil {
		prepared, prepareErr := prepareStoredOverride(current[id], *next)
		if prepareErr != nil {
			return ProjectOverrideRestore{}, prepareErr
		}
		next = &prepared
	}
	if err := applyOverrideMutation(document, current, id, next); err != nil {
		return ProjectOverrideRestore{}, err
	}
	encoded, err := encodeOverrideSettings(document)
	if err != nil {
		return ProjectOverrideRestore{}, err
	}
	if err := store.atomicWrite(store.paths.ProjectSettings, encoded); err != nil {
		return ProjectOverrideRestore{}, fmt.Errorf("skill overrides: write project settings: %w", err)
	}
	return ProjectOverrideRestore{
		path:           store.paths.ProjectSettings,
		before:         before,
		beforeExists:   exists,
		beforeRevision: currentRevision,
		afterRevision:  overrideRevision(true, encoded),
	}, nil
}

// RestoreProject precisely restores the pre-CAS file if no intervening writer
// changed it. A stale receipt is rejected instead of erasing newer settings.
func (store *FileOverrideStore) RestoreProject(restore ProjectOverrideRestore) error {
	store.stateMu.Lock()
	defer store.stateMu.Unlock()
	if restore.path == "" || restore.path != store.paths.ProjectSettings || !restore.afterRevision.Valid() {
		return errors.New("skill overrides: invalid project restore receipt")
	}
	if err := store.verifyPreparedRevisionsLocked(); err != nil {
		return err
	}
	pathLock := overrideLockForPath(store.paths.ProjectSettings)
	pathLock.Lock()
	defer pathLock.Unlock()
	_, _, exists, currentRevision, err := readOverrideSettings(store.paths.ProjectSettings, SkillScopeProject)
	if err != nil {
		return err
	}
	if currentRevision != restore.afterRevision {
		return fmt.Errorf("%w: project settings changed after persistent commit", ErrOverrideRevisionConflict)
	}
	if restore.beforeExists {
		if err := store.atomicWrite(store.paths.ProjectSettings, append([]byte(nil), restore.before...)); err != nil {
			return fmt.Errorf("skill overrides: restore project settings: %w", err)
		}
		return nil
	}
	if !exists {
		return nil
	}
	if err := store.removeFile(store.paths.ProjectSettings); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("skill overrides: remove compensated project settings: %w", err)
	}
	return nil
}

// commitPreparedProjectRetarget validates a staged project owner at the exact
// publication boundary. beforePublish runs only after both settings layers
// still match the prepared revisions; a failure leaves the store untouched.
// publish is in-memory-only and runs after beforePublish succeeds while all
// store readers and writers remain excluded.
func (store *FileOverrideStore) commitPreparedProjectRetarget(
	projectSettings string,
	userRevision, projectRevision OverrideStoreRevision,
	beforePublish func() error,
	publish func(),
) error {
	if store == nil || strings.TrimSpace(projectSettings) == "" || !userRevision.Valid() || !projectRevision.Valid() {
		return errors.New("skill overrides: invalid prepared project retarget")
	}
	store.stateMu.Lock()
	defer store.stateMu.Unlock()

	_, currentUserRevision, err := loadOverrideLayerLocked(store.paths.UserSettings, SkillScopeUser)
	if err != nil {
		return err
	}
	_, currentProjectRevision, err := loadOverrideLayerLocked(projectSettings, SkillScopeProject)
	if err != nil {
		return err
	}
	if currentUserRevision != userRevision || currentProjectRevision != projectRevision {
		return fmt.Errorf("%w: skill settings changed after project source preparation", ErrOverrideRevisionConflict)
	}
	if beforePublish != nil {
		if err := beforePublish(); err != nil {
			return err
		}
	}

	store.paths.ProjectSettings = projectSettings
	// The prepared revisions were accepted at commit time. A later filesystem
	// change is a new live settings update, not a stale-plan poison pill for the
	// first snapshot after a successful workspace publication.
	store.preparedUserRevision = ""
	store.preparedProjectRevision = ""
	store.preparedRevisionPending = false
	if publish != nil {
		publish()
	}
	return nil
}

// prepareProjectRetarget validates the current user layer and the target
// project document with the exact parser used by Snapshot. The revisions are
// carried by ProjectSourcePlan so the first post-apply observation can detect
// an external prepare/apply race without making apply itself fallible.
func (store *FileOverrideStore) prepareProjectRetarget(projectSettings string) (OverrideStoreRevision, OverrideStoreRevision, error) {
	store.stateMu.RLock()
	defer store.stateMu.RUnlock()
	_, userRevision, err := loadOverrideLayerLocked(store.paths.UserSettings, SkillScopeUser)
	if err != nil {
		return "", "", err
	}
	_, projectRevision, err := loadOverrideLayerLocked(projectSettings, SkillScopeProject)
	if err != nil {
		return "", "", err
	}
	return userRevision, projectRevision, nil
}

func (store *FileOverrideStore) verifyPreparedRevisionsLocked() error {
	if !store.preparedRevisionPending {
		return nil
	}
	_, userRevision, err := loadOverrideLayerLocked(store.paths.UserSettings, SkillScopeUser)
	if err != nil {
		return err
	}
	_, projectRevision, err := loadOverrideLayerLocked(store.paths.ProjectSettings, SkillScopeProject)
	if err != nil {
		return err
	}
	return store.acceptPreparedRevisionsLocked(userRevision, projectRevision)
}

func (store *FileOverrideStore) acceptPreparedRevisionsLocked(userRevision, projectRevision OverrideStoreRevision) error {
	if !store.preparedRevisionPending {
		return nil
	}
	if userRevision != store.preparedUserRevision || projectRevision != store.preparedProjectRevision {
		return fmt.Errorf("%w: skill settings changed after project source preparation", ErrOverrideRevisionConflict)
	}
	store.preparedRevisionPending = false
	store.preparedUserRevision = ""
	store.preparedProjectRevision = ""
	return nil
}

func (store *FileOverrideStore) validateMutable(id SkillID, scope SkillScope) error {
	if err := id.Validate(); err != nil {
		return err
	}
	switch scope {
	case SkillScopeUser, SkillScopeProject, SkillScopeSession:
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedOverrideScope, scope)
	}
	if _, managed := store.managed[id]; managed {
		return fmt.Errorf("%w: %s", ErrManagedOverrideReadOnly, id)
	}
	return nil
}

func (store *FileOverrideStore) togglePersistent(scope SkillScope, id SkillID) (VisibilityOverride, error) {
	path, err := store.pathForScope(scope)
	if err != nil {
		return VisibilityOverride{}, err
	}
	pathLock := overrideLockForPath(path)
	pathLock.Lock()
	defer pathLock.Unlock()
	document, current, _, _, err := readOverrideSettings(path, scope)
	if err != nil {
		return VisibilityOverride{}, err
	}
	next := toggledOverride(id, scope, current[id])
	if err := applyOverrideMutation(document, current, id, &next); err != nil {
		return VisibilityOverride{}, err
	}
	encoded, err := encodeOverrideSettings(document)
	if err != nil {
		return VisibilityOverride{}, err
	}
	if err := store.atomicWrite(path, encoded); err != nil {
		return VisibilityOverride{}, fmt.Errorf("skill overrides: write %s settings: %w", scope, err)
	}
	return cloneVisibilityOverride(next), nil
}

func (store *FileOverrideStore) mutatePersistent(scope SkillScope, expected OverrideStoreRevision, id SkillID, next *VisibilityOverride) (OverrideStoreRevision, error) {
	path, err := store.pathForScope(scope)
	if err != nil {
		return "", err
	}
	pathLock := overrideLockForPath(path)
	pathLock.Lock()
	defer pathLock.Unlock()
	document, current, _, currentRevision, err := readOverrideSettings(path, scope)
	if err != nil {
		return "", err
	}
	if expected.Valid() && currentRevision != expected {
		return "", fmt.Errorf("%w: expected %s, current %s", ErrOverrideRevisionConflict, expected, currentRevision)
	}
	if next != nil {
		prepared, prepareErr := prepareStoredOverride(current[id], *next)
		if prepareErr != nil {
			return "", prepareErr
		}
		next = &prepared
	}
	if err := applyOverrideMutation(document, current, id, next); err != nil {
		return "", err
	}
	encoded, err := encodeOverrideSettings(document)
	if err != nil {
		return "", err
	}
	if err := store.atomicWrite(path, encoded); err != nil {
		return "", fmt.Errorf("skill overrides: write %s settings: %w", scope, err)
	}
	return overrideRevision(true, encoded), nil
}

func (store *FileOverrideStore) pathForScope(scope SkillScope) (string, error) {
	switch scope {
	case SkillScopeUser:
		return store.paths.UserSettings, nil
	case SkillScopeProject:
		return store.paths.ProjectSettings, nil
	default:
		return "", fmt.Errorf("%w: %s", ErrUnsupportedOverrideScope, scope)
	}
}

type overrideSettingsDocument struct {
	values map[string]json.RawMessage
	raw    []byte
}

func loadOverrideLayerLocked(path string, scope SkillScope) (map[SkillID]VisibilityOverride, OverrideStoreRevision, error) {
	pathLock := overrideLockForPath(path)
	pathLock.Lock()
	defer pathLock.Unlock()
	_, overrides, _, revision, err := readOverrideSettings(path, scope)
	return overrides, revision, err
}

func readOverrideSettings(path string, scope SkillScope) (*overrideSettingsDocument, map[SkillID]VisibilityOverride, bool, OverrideStoreRevision, error) {
	raw, err := os.ReadFile(path)
	exists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return nil, nil, false, "", fmt.Errorf("skill overrides: read %s settings %s: %w", scope, path, err)
	}
	values := make(map[string]json.RawMessage)
	if exists && strings.TrimSpace(string(raw)) != "" {
		if err := json.Unmarshal(raw, &values); err != nil {
			return nil, nil, true, "", fmt.Errorf("skill overrides: parse %s settings %s: %w", scope, path, err)
		}
		if values == nil {
			return nil, nil, true, "", fmt.Errorf("skill overrides: parse %s settings %s: top-level value must be an object", scope, path)
		}
	}
	document := &overrideSettingsDocument{values: values, raw: append([]byte(nil), raw...)}
	overrides, err := decodeOverrideMap(values[skillOverridesSettingsKey], scope)
	if err != nil {
		return nil, nil, exists, "", fmt.Errorf("skill overrides: parse %s in %s: %w", skillOverridesSettingsKey, path, err)
	}
	return document, overrides, exists, overrideRevision(exists, raw), nil
}

func decodeOverrideMap(raw json.RawMessage, scope SkillScope) (map[SkillID]VisibilityOverride, error) {
	result := make(map[SkillID]VisibilityOverride)
	if len(raw) == 0 {
		return result, nil
	}
	var encoded map[string]json.RawMessage
	if err := json.Unmarshal(raw, &encoded); err != nil {
		return nil, fmt.Errorf("must be an object: %w", err)
	}
	if encoded == nil {
		return nil, errors.New("must be an object, not null")
	}
	for key, value := range encoded {
		id := SkillID(key)
		if err := id.Validate(); err != nil {
			return nil, fmt.Errorf("key %q: %w", key, err)
		}
		var override VisibilityOverride
		if err := json.Unmarshal(value, &override); err != nil {
			return nil, fmt.Errorf("%s: %w", id, err)
		}
		if override.SkillID != "" && override.SkillID != id {
			return nil, fmt.Errorf("%s: embedded skill ID does not match map key", id)
		}
		if override.Scope != "" && override.Scope != scope {
			return nil, fmt.Errorf("%s: embedded scope %s does not match %s settings", id, override.Scope, scope)
		}
		override.SkillID = id
		override.Scope = scope
		if err := override.Validate(); err != nil {
			return nil, fmt.Errorf("%s: %w", id, err)
		}
		result[id] = cloneVisibilityOverride(override)
	}
	return result, nil
}

func applyOverrideMutation(document *overrideSettingsDocument, current map[SkillID]VisibilityOverride, id SkillID, next *VisibilityOverride) error {
	if document == nil {
		return errors.New("skill overrides: nil settings document")
	}
	if next == nil {
		delete(current, id)
	} else {
		if next.SkillID != id {
			return errors.New("skill overrides: mutation ID mismatch")
		}
		if err := next.Validate(); err != nil {
			return err
		}
		current[id] = cloneVisibilityOverride(*next)
	}
	if len(current) == 0 {
		delete(document.values, skillOverridesSettingsKey)
		return nil
	}
	wire := make(map[string]VisibilityOverride, len(current))
	for currentID, override := range current {
		copy := cloneVisibilityOverride(override)
		copy.SkillID = ""
		copy.Scope = ""
		wire[string(currentID)] = copy
	}
	data, err := json.Marshal(wire)
	if err != nil {
		return fmt.Errorf("skill overrides: marshal records: %w", err)
	}
	document.values[skillOverridesSettingsKey] = data
	return nil
}

func encodeOverrideSettings(document *overrideSettingsDocument) ([]byte, error) {
	data, err := json.MarshalIndent(document.values, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("skill overrides: marshal settings: %w", err)
	}
	return append(data, '\n'), nil
}

func prepareStoredOverride(previous VisibilityOverride, requested VisibilityOverride) (VisibilityOverride, error) {
	requested = cloneVisibilityOverride(requested)
	if requested.Visibility.IsNonOff() {
		requested.LastNonOff = nil
	} else if requested.Visibility == VisibilityOff && requested.LastNonOff == nil {
		remembered := VisibilityAuto
		switch {
		case previous.Visibility.IsNonOff():
			remembered = previous.Visibility
		case previous.Visibility == VisibilityOff && previous.LastNonOff != nil && previous.LastNonOff.IsNonOff():
			remembered = *previous.LastNonOff
		}
		requested.LastNonOff = &remembered
	}
	if err := requested.Validate(); err != nil {
		return VisibilityOverride{}, err
	}
	return requested, nil
}

func toggledOverride(id SkillID, scope SkillScope, previous VisibilityOverride) VisibilityOverride {
	if previous.Visibility == VisibilityOff {
		return VisibilityOverride{SkillID: id, Scope: scope, Visibility: previous.RestoreVisibility()}
	}
	remembered := VisibilityAuto
	if previous.Visibility.IsNonOff() {
		remembered = previous.Visibility
	}
	return VisibilityOverride{SkillID: id, Scope: scope, Visibility: VisibilityOff, LastNonOff: &remembered}
}

func normalizeOverrideMap(input map[SkillID]VisibilityOverride, scope SkillScope) (map[SkillID]VisibilityOverride, error) {
	result := make(map[SkillID]VisibilityOverride, len(input))
	for id, override := range input {
		if err := id.Validate(); err != nil {
			return nil, err
		}
		if override.SkillID != "" && override.SkillID != id {
			return nil, fmt.Errorf("%s: embedded skill ID does not match map key", id)
		}
		if override.Scope != "" && override.Scope != scope {
			return nil, fmt.Errorf("%s: embedded scope %s does not match %s", id, override.Scope, scope)
		}
		override.SkillID = id
		override.Scope = scope
		if err := override.Validate(); err != nil {
			return nil, err
		}
		result[id] = cloneVisibilityOverride(override)
	}
	return result, nil
}

func cloneOverrideMap(input map[SkillID]VisibilityOverride) map[SkillID]VisibilityOverride {
	result := make(map[SkillID]VisibilityOverride, len(input))
	for id, override := range input {
		result[id] = cloneVisibilityOverride(override)
	}
	return result
}

func cloneVisibilityOverride(override VisibilityOverride) VisibilityOverride {
	if override.LastNonOff != nil {
		remembered := *override.LastNonOff
		override.LastNonOff = &remembered
	}
	return override
}

func validateOverrideSessionID(sessionID string) error {
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(sessionID) != sessionID {
		return ErrInvalidOverrideSession
	}
	return nil
}

func normalizeOverridePath(path string) (string, error) {
	if strings.TrimSpace(path) == "" || strings.TrimSpace(path) != path {
		return "", errors.New("path is empty or padded")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(absolute), nil
}

func overrideRevision(exists bool, data []byte) OverrideStoreRevision {
	hash := sha256.New()
	if exists {
		_, _ = hash.Write([]byte{1})
	} else {
		_, _ = hash.Write([]byte{0})
	}
	_, _ = hash.Write(data)
	return OverrideStoreRevision("sha256:" + hex.EncodeToString(hash.Sum(nil)))
}

var overridePathLocks sync.Map

func overrideLockForPath(path string) *sync.Mutex {
	value, _ := overridePathLocks.LoadOrStore(filepath.Clean(path), &sync.Mutex{})
	return value.(*sync.Mutex)
}

func atomicWriteOverrideSettings(path string, data []byte) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create settings directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".skill-overrides-*.tmp")
	if err != nil {
		return fmt.Errorf("create settings temp file: %w", err)
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		if !committed {
			_ = temporary.Close()
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("chmod settings temp file: %w", err)
	}
	if written, err := temporary.Write(data); err != nil {
		return fmt.Errorf("write settings temp file: %w", err)
	} else if written != len(data) {
		return fmt.Errorf("write settings temp file: short write: wrote %d of %d bytes", written, len(data))
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync settings temp file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close settings temp file: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace settings file: %w", err)
	}
	committed = true
	return nil
}

var _ SessionOverrideLayer = (*MemorySessionOverrideLayer)(nil)
var _ OverrideStore = (*FileOverrideStore)(nil)
