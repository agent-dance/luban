package session

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/agent-dance/luban/internal/runtime/goal"
	storepaths "github.com/agent-dance/luban/internal/store/paths"
	"github.com/agent-dance/luban/types"
)

// Ref uniquely identifies a stored session transcript. ProjectDir is a trusted
// store path produced by Repository discovery or ProjectDirForCWD; callers that
// construct refs directly are responsible for preserving that invariant.
type Ref struct {
	ID         string
	ProjectDir string
}

func (r Ref) IsZero() bool {
	return strings.TrimSpace(r.ID) == "" || strings.TrimSpace(r.ProjectDir) == ""
}

// Repository indexes project-scoped session stores.
type Repository struct {
	projectsRoot string
	storesMu     sync.Mutex
	stores       map[string]*FileStore
}

// NewRepository creates a repository rooted at configHome.
func NewRepository(configHome string) *Repository {
	root := strings.TrimSpace(configHome)
	if root == "" {
		root = storepaths.ConfigHomeDir()
	}
	return &Repository{
		projectsRoot: filepath.Join(root, "projects"),
	}
}

// DefaultRepository returns a repository rooted at paths.ConfigHomeDir().
func DefaultRepository() *Repository {
	return NewRepository(storepaths.ConfigHomeDir())
}

func (r *Repository) ProjectsRoot() string {
	return r.projectsRoot
}

func (r *Repository) ProjectDirForCWD(cwd string) string {
	return filepath.Join(r.projectsRoot, storepaths.ProjectKeyForCWD(cwd))
}

func (r *Repository) StoreForProjectDir(projectDir string) *FileStore {
	key := filepath.Clean(projectDir)
	r.storesMu.Lock()
	defer r.storesMu.Unlock()
	if store := r.stores[key]; store != nil {
		return store
	}
	if r.stores == nil {
		r.stores = make(map[string]*FileStore)
	}
	store := NewFileStore(projectDir)
	r.stores[key] = store
	return store
}

func (r *Repository) ResolveLatest(currentProjectDir string) (Ref, error) {
	trimmed := strings.TrimSpace(currentProjectDir)
	if trimmed != "" {
		store := r.StoreForProjectDir(trimmed)
		if latest, err := store.Latest(); err == nil {
			return Ref{ID: latest, ProjectDir: trimmed}, nil
		} else if !errors.Is(err, ErrNoSessions) {
			return Ref{}, err
		}
	}

	sessions, err := r.Search(SearchOptions{AllProjects: true})
	if err != nil {
		return Ref{}, err
	}
	if len(sessions) == 0 {
		return Ref{}, ErrNoSessions
	}
	return Ref{ID: sessions[0].ID, ProjectDir: sessions[0].ProjectDir}, nil
}

func (r *Repository) Resolve(sessionID, currentProjectDir string) (Ref, error) {
	id := strings.TrimSpace(sessionID)
	if id == "" {
		return Ref{}, fmt.Errorf("session ID is required")
	}
	if trimmed := strings.TrimSpace(currentProjectDir); trimmed != "" {
		store := r.StoreForProjectDir(trimmed)
		live, err := storeHasLiveSession(store, id)
		if err != nil {
			return Ref{}, err
		}
		if live {
			return Ref{ID: id, ProjectDir: trimmed}, nil
		}
	}

	var matches []Ref
	for _, dir := range r.allStoreDirs() {
		store := r.StoreForProjectDir(dir)
		live, err := storeHasLiveSession(store, id)
		if err != nil {
			return Ref{}, err
		}
		if live {
			matches = append(matches, Ref{ID: id, ProjectDir: dir})
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return Ref{}, fmt.Errorf("session %q exists in %d projects; refine by title or current project", id, len(matches))
	}
	return Ref{}, fmt.Errorf("session %q not found: %w", id, fs.ErrNotExist)
}

func storeHasLiveSession(store *FileStore, sessionID string) (bool, error) {
	if _, err := store.tightenPrivateRegularFile(store.manifestPath(sessionID), false); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	deleted, err := store.IsDeleted(sessionID)
	if err != nil {
		return false, err
	}
	return !deleted, nil
}

func (r *Repository) Load(ref Ref) ([]types.Message, error) {
	if ref.IsZero() {
		return nil, fmt.Errorf("session ref is incomplete")
	}
	return r.StoreForProjectDir(ref.ProjectDir).Load(ref.ID)
}

func (r *Repository) TranscriptPath(sessionID, currentProjectDir string) (string, error) {
	ref, err := r.Resolve(sessionID, currentProjectDir)
	if err != nil {
		return "", err
	}
	return r.StoreForProjectDir(ref.ProjectDir).TranscriptPath(ref.ID), nil
}

func (r *Repository) LoadByID(sessionID, currentProjectDir string) ([]types.Message, Ref, error) {
	ref, err := r.Resolve(sessionID, currentProjectDir)
	if err != nil {
		return nil, Ref{}, err
	}
	msgs, err := r.Load(ref)
	return msgs, ref, err
}

func (r *Repository) Save(sessionID, currentProjectDir string, messages []types.Message) error {
	projectDir := strings.TrimSpace(currentProjectDir)
	if projectDir == "" {
		return fmt.Errorf("current project dir is required for save")
	}
	return r.StoreForProjectDir(projectDir).Save(sessionID, messages)
}

func (r *Repository) SaveMeta(sessionID, currentProjectDir string, meta SessionMeta) error {
	projectDir := strings.TrimSpace(currentProjectDir)
	if projectDir == "" {
		ref, err := r.Resolve(sessionID, "")
		if err != nil {
			return err
		}
		projectDir = ref.ProjectDir
	}
	store := r.StoreForProjectDir(projectDir)
	if deleted, err := store.IsDeleted(sessionID); err != nil {
		return err
	} else if deleted {
		return fmt.Errorf("%w: %s", ErrSessionDeleted, sessionID)
	}
	if _, err := store.tightenPrivateRegularFile(store.manifestPath(sessionID), false); err != nil {
		return err
	}
	return store.SaveMeta(sessionID, meta)
}

// UpdateGoal applies update under the cached project FileStore lock so the
// goal transition is atomic with SaveMeta calls made through this repository.
func (r *Repository) UpdateGoal(sessionID, currentProjectDir string, update goal.UpdateFunc) (goal.Goal, error) {
	projectDir := strings.TrimSpace(currentProjectDir)
	if projectDir == "" {
		ref, err := r.Resolve(sessionID, "")
		if err != nil {
			return goal.Goal{}, err
		}
		projectDir = ref.ProjectDir
	}
	store := r.StoreForProjectDir(projectDir)
	if deleted, err := store.IsDeleted(sessionID); err != nil {
		return goal.Goal{}, err
	} else if deleted {
		return goal.Goal{}, fmt.Errorf("%w: %s", ErrSessionDeleted, sessionID)
	}
	if _, err := store.tightenPrivateRegularFile(store.manifestPath(sessionID), false); err != nil {
		return goal.Goal{}, err
	}
	return store.UpdateGoal(sessionID, update)
}

func (r *Repository) GetMeta(sessionID, currentProjectDir string) (SessionMeta, Ref, error) {
	ref, err := r.Resolve(sessionID, currentProjectDir)
	if err != nil {
		return SessionMeta{}, Ref{}, err
	}
	meta, err := r.StoreForProjectDir(ref.ProjectDir).GetMeta(ref.ID)
	return meta, ref, err
}

func (r *Repository) Delete(sessionID, currentProjectDir string) error {
	if projectDir := strings.TrimSpace(currentProjectDir); projectDir != "" {
		store := r.StoreForProjectDir(projectDir)
		deleted, err := store.IsDeleted(sessionID)
		if err != nil {
			return err
		}
		if deleted {
			return store.Delete(sessionID)
		}
		if _, err := store.tightenPrivateRegularFile(store.manifestPath(sessionID), false); err == nil {
			return store.Delete(sessionID)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	ref, err := r.Resolve(sessionID, currentProjectDir)
	if err != nil {
		return err
	}
	return r.StoreForProjectDir(ref.ProjectDir).Delete(ref.ID)
}

// IsDeleted reports whether delete-history has committed in the requested
// project namespace. Callers must provide the namespace because session IDs
// are only unique within a project.
func (r *Repository) IsDeleted(sessionID, currentProjectDir string) (bool, error) {
	projectDir := strings.TrimSpace(currentProjectDir)
	if projectDir == "" {
		return false, fmt.Errorf("current project dir is required for deletion lookup")
	}
	return r.StoreForProjectDir(projectDir).IsDeleted(sessionID)
}

func (r *Repository) ArtifactsDir(sessionID, currentProjectDir string) string {
	id := strings.TrimSpace(sessionID)
	if validateStorageID(id) != nil {
		projectDir := strings.TrimSpace(currentProjectDir)
		if projectDir == "" {
			projectDir = r.ProjectDirForCWD("")
		}
		return r.StoreForProjectDir(projectDir).ArtifactsDir(id)
	}
	if ref, err := r.Resolve(id, currentProjectDir); err == nil {
		return r.StoreForProjectDir(ref.ProjectDir).ArtifactsDir(ref.ID)
	}
	projectDir := strings.TrimSpace(currentProjectDir)
	if projectDir == "" {
		return r.StoreForProjectDir(r.ProjectDirForCWD("")).ArtifactsDir(id)
	}
	return r.StoreForProjectDir(projectDir).ArtifactsDir(id)
}

func (r *Repository) Search(opts SearchOptions) ([]SessionInfo, error) {
	if !opts.AllProjects && strings.TrimSpace(opts.CurrentProjectDir) != "" {
		store := r.StoreForProjectDir(opts.CurrentProjectDir)
		return store.Search(opts)
	}

	var sessions []SessionInfo
	for _, dir := range r.allStoreDirs() {
		store := r.StoreForProjectDir(dir)
		items, err := store.Search(opts)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) || errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		sessions = append(sessions, items...)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})
	return sessions, nil
}

func (r *Repository) allStoreDirs() []string {
	dirs := appendProjectStoreDirs(nil, r.projectsRoot)
	dirs = uniqueSessionDirs(dirs)
	sort.Strings(dirs)
	return dirs
}

func appendProjectStoreDirs(dirs []string, projectsRoot string) []string {
	if strings.TrimSpace(projectsRoot) == "" {
		return dirs
	}
	if entries, err := os.ReadDir(projectsRoot); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			dirs = append(dirs, filepath.Join(projectsRoot, entry.Name()))
		}
	}
	return dirs
}

func uniqueSessionDirs(dirs []string) []string {
	seen := make(map[string]struct{}, len(dirs))
	unique := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		clean := filepath.Clean(dir)
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		unique = append(unique, clean)
	}
	return unique
}
