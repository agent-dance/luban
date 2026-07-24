package engine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/agent-dance/luban/goal"
	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/session"
	"github.com/agent-dance/luban/types"
)

// repositorySessionManager adapts session.Repository to the engine.SessionManager
// interface while keeping the active project directory dynamic.
type repositorySessionManager struct {
	repo              *session.Repository
	currentProjectDir func() string
	generations       sessionGenerationTracker
}

func newRepositorySessionManager(repo *session.Repository, currentProjectDir func() string) *repositorySessionManager {
	if repo == nil {
		repo = session.DefaultRepository()
	}
	if currentProjectDir == nil {
		currentProjectDir = func() string { return "" }
	}
	return &repositorySessionManager{
		repo:              repo,
		currentProjectDir: currentProjectDir,
	}
}

// NewRepositorySessionManager exposes the repository-backed session manager to
// entrypoints that need a shared manager with a dynamic current project.
func NewRepositorySessionManager(repo *session.Repository, currentProjectDir func() string) SessionManager {
	return newRepositorySessionManager(repo, currentProjectDir)
}

func (m *repositorySessionManager) projectDir() string {
	return strings.TrimSpace(m.currentProjectDir())
}

func (m *repositorySessionManager) projectDirForCWD(cwd string) string {
	trimmed := strings.TrimSpace(cwd)
	if trimmed == "" {
		return ""
	}
	return m.repo.ProjectDirForCWD(trimmed)
}

func (m *repositorySessionManager) prepareContextGeneration(sessionID, projectDir string) error {
	projectDir = strings.TrimSpace(projectDir)
	if projectDir == "" {
		projectDir = m.projectDir()
	}
	if projectDir == "" {
		return fmt.Errorf("current project dir is required for context generation")
	}
	return m.generations.prepare(m.repo.StoreForProjectDir(projectDir), generationKey(projectDir, sessionID), sessionID)
}

func (m *repositorySessionManager) contextGeneration(sessionID, projectDir string) (uint64, error) {
	state, err := m.contextGenerationState(sessionID, projectDir)
	return state.Generation, err
}

func (m *repositorySessionManager) contextGenerationState(sessionID, projectDir string) (ContextGenerationState, error) {
	projectDir = strings.TrimSpace(projectDir)
	if projectDir == "" {
		projectDir = m.projectDir()
	}
	if projectDir == "" {
		return ContextGenerationState{}, fmt.Errorf("current project dir is required for context generation")
	}
	return m.generations.current(m.repo.StoreForProjectDir(projectDir), generationKey(projectDir, sessionID), sessionID)
}

func (m *repositorySessionManager) internalControlScope(sessionID, projectDir string) (messagecontrol.Scope, error) {
	projectDir = strings.TrimSpace(projectDir)
	if projectDir == "" {
		projectDir = m.projectDir()
	}
	if projectDir == "" {
		ref, err := m.repo.Resolve(sessionID, "")
		if err != nil {
			return messagecontrol.Scope{}, err
		}
		projectDir = ref.ProjectDir
	}
	return m.repo.StoreForProjectDir(projectDir).MessageControlScope(sessionID)
}

func (m *repositorySessionManager) Save(sessionID string, messages []types.Message) error {
	projectDir := m.projectDir()
	if projectDir == "" {
		return fmt.Errorf("current project dir is required for save")
	}
	return m.generations.save(m.repo.StoreForProjectDir(projectDir), generationKey(projectDir, sessionID), sessionID, messages)
}

func (m *repositorySessionManager) Load(sessionID string) ([]types.Message, error) {
	projectDir := m.projectDir()
	if projectDir != "" {
		if deleted, err := m.repo.IsDeleted(sessionID, projectDir); err == nil && deleted {
			return nil, fmt.Errorf("%w: %w: %s", ErrSessionNotFound, ErrSessionDeleted, sessionID)
		} else if err != nil {
			return nil, err
		}
	}
	msgs, ref, err := m.repo.LoadByID(sessionID, projectDir)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
	}
	if err := m.generations.recordLoaded(m.repo.StoreForProjectDir(ref.ProjectDir), generationKey(ref.ProjectDir, sessionID), sessionID); err != nil {
		return nil, err
	}
	return msgs, nil
}

func (m *repositorySessionManager) loadFromProject(sessionID, projectDir string) ([]types.Message, error) {
	store, err := m.exactProjectStore(projectDir)
	if err != nil {
		return nil, err
	}
	messages, err := store.Load(sessionID)
	if err != nil {
		return nil, exactProjectSessionReadError(sessionID, err)
	}
	if err := m.generations.recordLoaded(store, generationKey(projectDir, sessionID), sessionID); err != nil {
		return nil, exactProjectSessionReadError(sessionID, err)
	}
	return messages, nil
}

func (m *repositorySessionManager) saveToProject(sessionID, projectDir string, messages []types.Message) error {
	projectDir = strings.TrimSpace(projectDir)
	if projectDir == "" {
		return fmt.Errorf("current project dir is required for save")
	}
	return m.generations.save(m.repo.StoreForProjectDir(projectDir), generationKey(projectDir, sessionID), sessionID, messages)
}

func (m *repositorySessionManager) saveProviderModelToProject(sessionID, projectDir, providerName, modelID string) error {
	return m.repo.SaveMeta(sessionID, strings.TrimSpace(projectDir), session.SessionMeta{Provider: providerName, Model: modelID})
}

func (m *repositorySessionManager) saveSessionContextToProject(sessionID, projectDir, cwd, gitBranch string) error {
	return m.repo.SaveMeta(sessionID, strings.TrimSpace(projectDir), session.SessionMeta{CWD: cwd, GitBranch: gitBranch})
}

func (m *repositorySessionManager) saveToolUseLedgerToProject(sessionID, projectDir string, seenToolUseIDs []string) error {
	return m.repo.SaveMeta(sessionID, strings.TrimSpace(projectDir), session.SessionMeta{SeenToolUseIDs: append([]string(nil), seenToolUseIDs...)})
}

func (m *repositorySessionManager) loadToolUseLedgerFromProject(sessionID, projectDir string) ([]string, error) {
	if cleanProjectDir(projectDir) == "" {
		meta, _, err := m.repo.GetMeta(sessionID, "")
		if err != nil {
			return nil, err
		}
		return append([]string(nil), meta.SeenToolUseIDs...), nil
	}
	store, err := m.exactProjectStore(projectDir)
	if err != nil {
		return nil, err
	}
	meta, err := store.GetMeta(sessionID)
	if err != nil {
		return nil, exactProjectSessionReadError(sessionID, err)
	}
	return append([]string(nil), meta.SeenToolUseIDs...), nil
}

func (m *repositorySessionManager) saveLoadedToolNamesToProject(sessionID, projectDir string, loadedToolNames []string) error {
	return m.repo.SaveMeta(sessionID, strings.TrimSpace(projectDir), session.SessionMeta{LoadedToolNames: append([]string(nil), loadedToolNames...)})
}

func (m *repositorySessionManager) loadLoadedToolNamesFromProject(sessionID, projectDir string) ([]string, error) {
	if cleanProjectDir(projectDir) == "" {
		meta, _, err := m.repo.GetMeta(sessionID, "")
		if err != nil {
			return nil, err
		}
		return append([]string(nil), meta.LoadedToolNames...), nil
	}
	store, err := m.exactProjectStore(projectDir)
	if err != nil {
		return nil, err
	}
	meta, err := store.GetMeta(sessionID)
	if err != nil {
		return nil, exactProjectSessionReadError(sessionID, err)
	}
	return append([]string(nil), meta.LoadedToolNames...), nil
}

func (m *repositorySessionManager) loadCacheLineageIDFromProject(sessionID, projectDir string) (string, error) {
	var (
		meta session.SessionMeta
		err  error
	)
	if cleanProjectDir(projectDir) == "" {
		meta, _, err = m.repo.GetMeta(sessionID, "")
	} else {
		var store *session.FileStore
		store, err = m.exactProjectStore(projectDir)
		if err == nil {
			meta, err = store.GetMeta(sessionID)
		}
	}
	if err != nil {
		return "", err
	}
	if lineageID := strings.TrimSpace(meta.CacheLineageID); lineageID != "" {
		return lineageID, nil
	}
	return strings.TrimSpace(sessionID), nil
}

func (m *repositorySessionManager) saveSkillsMetaToProject(sessionID, projectDir string, skillsMeta *session.SessionSkillsMeta) error {
	if skillsMeta == nil {
		return nil
	}
	cloned := skillsMeta.Clone()
	return m.repo.SaveMeta(sessionID, strings.TrimSpace(projectDir), session.SessionMeta{Skills: &cloned})
}

func (m *repositorySessionManager) loadSkillsMetaFromProject(sessionID, projectDir string) (*session.SessionSkillsMeta, error) {
	if cleanProjectDir(projectDir) == "" {
		meta, _, err := m.repo.GetMeta(sessionID, "")
		if err != nil {
			return nil, err
		}
		if meta.Skills == nil {
			return nil, nil
		}
		cloned := meta.Skills.Clone()
		return &cloned, nil
	}
	store, err := m.exactProjectStore(projectDir)
	if err != nil {
		return nil, err
	}
	meta, err := store.GetMeta(sessionID)
	if err != nil {
		return nil, exactProjectSessionReadError(sessionID, err)
	}
	if meta.Skills == nil {
		return nil, nil
	}
	cloned := meta.Skills.Clone()
	return &cloned, nil
}

func (m *repositorySessionManager) exactProjectStore(projectDir string) (*session.FileStore, error) {
	projectDir = cleanProjectDir(projectDir)
	if projectDir == "" {
		return nil, fmt.Errorf("%w: explicit project directory is required", ErrSessionNotFound)
	}
	return m.repo.StoreForProjectDir(projectDir), nil
}

func exactProjectSessionReadError(sessionID string, err error) error {
	if errors.Is(err, session.ErrSessionDeleted) {
		return fmt.Errorf("%w: %w: %s", ErrSessionNotFound, ErrSessionDeleted, sessionID)
	}
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
	}
	return err
}

func (m *repositorySessionManager) loadGoalFromProject(sessionID, projectDir string) (*goal.Goal, error) {
	if cleanProjectDir(projectDir) == "" {
		meta, _, err := m.repo.GetMeta(sessionID, "")
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil, nil
			}
			return nil, err
		}
		return cloneGoal(meta.Goal), nil
	}
	store, err := m.exactProjectStore(projectDir)
	if err != nil {
		return nil, err
	}
	meta, err := store.GetMeta(sessionID)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return cloneGoal(meta.Goal), nil
}

func (m *repositorySessionManager) saveGoalToProject(sessionID, projectDir string, state goal.Goal) error {
	projectDir = strings.TrimSpace(projectDir)
	stateCopy := cloneGoal(&state)
	err := m.repo.SaveMeta(sessionID, projectDir, session.SessionMeta{Goal: stateCopy})
	if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := m.saveToProject(sessionID, projectDir, nil); err != nil {
		return err
	}
	return m.repo.SaveMeta(sessionID, projectDir, session.SessionMeta{Goal: stateCopy})
}

func (m *repositorySessionManager) updateGoalInProject(sessionID, projectDir string, update goal.UpdateFunc) (goal.Goal, error) {
	projectDir = strings.TrimSpace(projectDir)
	next, err := m.repo.UpdateGoal(sessionID, projectDir, update)
	if !errors.Is(err, fs.ErrNotExist) {
		return next, err
	}
	if err := m.saveToProject(sessionID, projectDir, nil); err != nil {
		return goal.Goal{}, err
	}
	return m.repo.UpdateGoal(sessionID, projectDir, update)
}

func (m *repositorySessionManager) deleteFromProject(sessionID, projectDir string) error {
	projectDir = strings.TrimSpace(projectDir)
	err := m.repo.Delete(sessionID, projectDir)
	if err == nil {
		m.generations.remove(generationKey(projectDir, sessionID))
	}
	return err
}

func (m *repositorySessionManager) isDeletedInProject(sessionID, projectDir string) (bool, error) {
	return m.repo.IsDeleted(sessionID, strings.TrimSpace(projectDir))
}

func (m *repositorySessionManager) List() ([]SessionInfo, error) {
	infos, err := m.repo.Search(session.SearchOptions{
		CurrentProjectDir: m.projectDir(),
		AllProjects:       true,
	})
	if err != nil {
		return nil, err
	}
	out := make([]SessionInfo, len(infos))
	for i, si := range infos {
		out[i] = SessionInfo{
			ID:        si.ID,
			UpdatedAt: si.UpdatedAt.Unix(),
			Messages:  si.MessageCount,
		}
	}
	return out, nil
}

func (m *repositorySessionManager) Latest() (string, error) {
	ref, err := m.repo.ResolveLatest(m.projectDir())
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrSessionNotFound, err)
	}
	return ref.ID, nil
}

func (m *repositorySessionManager) Delete(sessionID string) error {
	projectDir := m.projectDir()
	if err := m.repo.Delete(sessionID, projectDir); err != nil {
		if errors.Is(err, session.ErrSessionDeleted) {
			return fmt.Errorf("%w: %s", ErrSessionDeleted, sessionID)
		}
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
		}
		return fmt.Errorf("delete session %s: %w", sessionID, err)
	}
	m.generations.remove(generationKey(projectDir, sessionID))
	return nil
}

func (m *repositorySessionManager) SaveProviderModel(sessionID, providerName, modelID string) error {
	return m.repo.SaveMeta(sessionID, m.projectDir(), session.SessionMeta{
		Provider: providerName,
		Model:    modelID,
	})
}

func (m *repositorySessionManager) ArtifactsDir(sessionID string) string {
	return m.repo.ArtifactsDir(sessionID, m.projectDir())
}

func (m *repositorySessionManager) artifactsDirForProject(sessionID, projectDir string) string {
	return m.repo.ArtifactsDir(sessionID, strings.TrimSpace(projectDir))
}

func (m *repositorySessionManager) TranscriptPath(sessionID string) string {
	path, err := m.repo.TranscriptPath(sessionID, m.projectDir())
	if err != nil {
		return ""
	}
	return path
}

func (m *repositorySessionManager) transcriptPathForProject(sessionID, projectDir string) string {
	path, err := m.repo.TranscriptPath(sessionID, strings.TrimSpace(projectDir))
	if err != nil {
		return ""
	}
	return path
}

func (m *repositorySessionManager) SaveSessionContext(sessionID, cwd, gitBranch string) error {
	return m.repo.SaveMeta(sessionID, m.projectDir(), session.SessionMeta{
		CWD:       cwd,
		GitBranch: gitBranch,
	})
}

func (m *repositorySessionManager) SaveToolUseLedger(sessionID string, seenToolUseIDs []string) error {
	return m.saveToolUseLedgerToProject(sessionID, m.projectDir(), seenToolUseIDs)
}

func (m *repositorySessionManager) LoadToolUseLedger(sessionID string) ([]string, error) {
	return m.loadToolUseLedgerFromProject(sessionID, m.projectDir())
}

func (m *repositorySessionManager) SaveLoadedToolNames(sessionID string, loadedToolNames []string) error {
	return m.saveLoadedToolNamesToProject(sessionID, m.projectDir(), loadedToolNames)
}

func (m *repositorySessionManager) LoadLoadedToolNames(sessionID string) ([]string, error) {
	return m.loadLoadedToolNamesFromProject(sessionID, m.projectDir())
}

var _ SessionMetaSaver = (*repositorySessionManager)(nil)
var _ SessionArtifactsDirProvider = (*repositorySessionManager)(nil)
var _ SessionTranscriptPathProvider = (*repositorySessionManager)(nil)
var _ SessionContextSaver = (*repositorySessionManager)(nil)
var _ SessionToolUseLedgerStore = (*repositorySessionManager)(nil)
var _ SessionLoadedToolStore = (*repositorySessionManager)(nil)

// fileSessionManager is kept for tests and call sites that operate on a single
// explicit store directory rather than the project-scoped repository.
type fileSessionManager struct {
	store       *session.FileStore
	dir         string
	generations sessionGenerationTracker
}

func newFileSessionManager(dir string) *fileSessionManager {
	return &fileSessionManager{
		store: session.NewFileStore(dir),
		dir:   dir,
	}
}

func (m *fileSessionManager) Save(sessionID string, messages []types.Message) error {
	return m.generations.save(m.store, generationKey(m.dir, sessionID), sessionID, messages)
}

func (m *fileSessionManager) prepareContextGeneration(sessionID, _ string) error {
	return m.generations.prepare(m.store, generationKey(m.dir, sessionID), sessionID)
}

func (m *fileSessionManager) contextGeneration(sessionID, _ string) (uint64, error) {
	state, err := m.contextGenerationState(sessionID, "")
	return state.Generation, err
}

func (m *fileSessionManager) contextGenerationState(sessionID, _ string) (ContextGenerationState, error) {
	return m.generations.current(m.store, generationKey(m.dir, sessionID), sessionID)
}

func (m *fileSessionManager) internalControlScope(sessionID, _ string) (messagecontrol.Scope, error) {
	return m.store.MessageControlScope(sessionID)
}

func (m *fileSessionManager) Load(sessionID string) ([]types.Message, error) {
	msgs, err := m.store.Load(sessionID)
	if err != nil {
		if errors.Is(err, session.ErrSessionDeleted) {
			return nil, fmt.Errorf("%w: %w: %s", ErrSessionNotFound, ErrSessionDeleted, sessionID)
		}
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
		}
		return nil, err
	}
	if err := m.generations.recordLoaded(m.store, generationKey(m.dir, sessionID), sessionID); err != nil {
		return nil, err
	}
	return msgs, nil
}

func (m *fileSessionManager) List() ([]SessionInfo, error) {
	infos, err := m.store.List()
	if err != nil {
		return nil, err
	}
	out := make([]SessionInfo, len(infos))
	for i, si := range infos {
		out[i] = SessionInfo{
			ID:        si.ID,
			UpdatedAt: si.UpdatedAt.Unix(),
			Messages:  si.MessageCount,
		}
	}
	return out, nil
}

func (m *fileSessionManager) Latest() (string, error) {
	id, err := m.store.Latest()
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrSessionNotFound, err)
	}
	return id, nil
}

func (m *fileSessionManager) Delete(sessionID string) error {
	if err := m.store.Delete(sessionID); err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
		}
		return err
	}
	m.generations.remove(generationKey(m.dir, sessionID))
	return nil
}

func generationKey(projectDir, sessionID string) string {
	return filepath.Clean(strings.TrimSpace(projectDir)) + "\x00" + strings.TrimSpace(sessionID)
}

func (m *fileSessionManager) isDeleted(sessionID string) (bool, error) {
	return m.store.IsDeleted(sessionID)
}

func (m *fileSessionManager) SaveProviderModel(sessionID, providerName, modelID string) error {
	return m.store.SaveMeta(sessionID, session.SessionMeta{
		Provider: providerName,
		Model:    modelID,
	})
}

func (m *fileSessionManager) ArtifactsDir(sessionID string) string {
	return filepath.Join(m.dir, sessionID)
}

func (m *fileSessionManager) TranscriptPath(sessionID string) string {
	return m.store.TranscriptPath(sessionID)
}

func (m *fileSessionManager) SaveSessionContext(sessionID, cwd, gitBranch string) error {
	return m.store.SaveMeta(sessionID, session.SessionMeta{
		CWD:       cwd,
		GitBranch: gitBranch,
	})
}

func (m *fileSessionManager) SaveToolUseLedger(sessionID string, seenToolUseIDs []string) error {
	return m.store.SaveMeta(sessionID, session.SessionMeta{SeenToolUseIDs: append([]string(nil), seenToolUseIDs...)})
}

func (m *fileSessionManager) LoadToolUseLedger(sessionID string) ([]string, error) {
	meta, err := m.store.GetMeta(sessionID)
	if err != nil {
		return nil, err
	}
	return append([]string(nil), meta.SeenToolUseIDs...), nil
}

func (m *fileSessionManager) SaveLoadedToolNames(sessionID string, loadedToolNames []string) error {
	return m.store.SaveMeta(sessionID, session.SessionMeta{LoadedToolNames: append([]string(nil), loadedToolNames...)})
}

func (m *fileSessionManager) LoadLoadedToolNames(sessionID string) ([]string, error) {
	meta, err := m.store.GetMeta(sessionID)
	if err != nil {
		return nil, err
	}
	return append([]string(nil), meta.LoadedToolNames...), nil
}

// The file-backed adapter is rooted at one immutable directory, so the
// explicit projectDir is intentionally ignored. Keeping it in the method
// contract prevents callers from accidentally falling back to a mutable
// current-project resolver when using the repository-backed adapter.
func (m *fileSessionManager) saveSkillsMetaToProject(sessionID, _ string, skillsMeta *session.SessionSkillsMeta) error {
	if skillsMeta == nil {
		return nil
	}
	cloned := skillsMeta.Clone()
	return m.store.SaveMeta(sessionID, session.SessionMeta{Skills: &cloned})
}

func (m *fileSessionManager) loadSkillsMetaFromProject(sessionID, _ string) (*session.SessionSkillsMeta, error) {
	meta, err := m.store.GetMeta(sessionID)
	if err != nil {
		return nil, err
	}
	if meta.Skills == nil {
		return nil, nil
	}
	cloned := meta.Skills.Clone()
	return &cloned, nil
}

var _ SessionMetaSaver = (*fileSessionManager)(nil)
var _ SessionArtifactsDirProvider = (*fileSessionManager)(nil)
var _ SessionTranscriptPathProvider = (*fileSessionManager)(nil)
var _ SessionContextSaver = (*fileSessionManager)(nil)
var _ SessionToolUseLedgerStore = (*fileSessionManager)(nil)
var _ SessionLoadedToolStore = (*fileSessionManager)(nil)
