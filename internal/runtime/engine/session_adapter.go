package engine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/internal/runtime/goal"
	"github.com/agent-dance/luban/internal/store/session"
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

func (m *repositorySessionManager) projectDirForRoot(projectRoot string) string {
	trimmed := strings.TrimSpace(projectRoot)
	if trimmed == "" {
		return ""
	}
	return m.repo.ProjectDirForCWD(trimmed)
}

func (m *repositorySessionManager) prepareContextGeneration(sessionID, projectDir string) error {
	projectDir, err := requireProjectDir(projectDir)
	if err != nil {
		return err
	}
	return m.generations.prepare(m.repo.StoreForProjectDir(projectDir), generationKey(projectDir, sessionID), sessionID)
}

func (m *repositorySessionManager) contextGenerationState(sessionID, projectDir string) (ContextGenerationState, error) {
	projectDir, err := requireProjectDir(projectDir)
	if err != nil {
		return ContextGenerationState{}, err
	}
	return m.generations.current(m.repo.StoreForProjectDir(projectDir), generationKey(projectDir, sessionID), sessionID)
}

func (m *repositorySessionManager) internalControlScope(sessionID, projectDir string) (messagecontrol.Scope, error) {
	projectDir, err := requireProjectDir(projectDir)
	if err != nil {
		return messagecontrol.Scope{}, err
	}
	return m.repo.StoreForProjectDir(projectDir).MessageControlScope(sessionID)
}

func (m *repositorySessionManager) Save(sessionID string, messages []types.Message) error {
	projectDir, err := requireProjectDir(m.projectDir())
	if err != nil {
		return err
	}
	return m.generations.save(m.repo.StoreForProjectDir(projectDir), generationKey(projectDir, sessionID), sessionID, messages)
}

func (m *repositorySessionManager) Load(sessionID string) ([]types.Message, error) {
	projectDir, err := requireProjectDir(m.projectDir())
	if err != nil {
		return nil, err
	}
	if deleted, deletedErr := m.repo.IsDeleted(sessionID, projectDir); deletedErr == nil && deleted {
		return nil, fmt.Errorf("%w: %w: %s", ErrSessionNotFound, ErrSessionDeleted, sessionID)
	} else if deletedErr != nil {
		return nil, deletedErr
	}
	store := m.repo.StoreForProjectDir(projectDir)
	msgs, err := store.Load(sessionID)
	if err != nil {
		return nil, exactProjectSessionReadError(sessionID, err)
	}
	if err := m.generations.recordLoaded(store, generationKey(projectDir, sessionID), sessionID); err != nil {
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
	projectDir, err := requireProjectDir(projectDir)
	if err != nil {
		return err
	}
	return m.generations.save(m.repo.StoreForProjectDir(projectDir), generationKey(projectDir, sessionID), sessionID, messages)
}

func (m *repositorySessionManager) saveConversationMetaToProject(sessionID, projectDir string, meta session.SessionMeta) error {
	store, err := m.exactProjectStore(projectDir)
	if err != nil {
		return err
	}
	return store.SaveMeta(sessionID, meta)
}

func (m *repositorySessionManager) saveProviderModelToProject(sessionID, projectDir, providerName, modelID string) error {
	store, err := m.exactProjectStore(projectDir)
	if err != nil {
		return err
	}
	return store.SaveMeta(sessionID, session.SessionMeta{Provider: providerName, Model: modelID})
}

func (m *repositorySessionManager) saveSessionContextToProject(sessionID, projectDir, cwd, gitBranch string) error {
	store, err := m.exactProjectStore(projectDir)
	if err != nil {
		return err
	}
	return store.SaveMeta(sessionID, session.SessionMeta{CWD: cwd, GitBranch: gitBranch})
}

func (m *repositorySessionManager) saveToolUseLedgerToProject(sessionID, projectDir string, seenToolUseIDs []string) error {
	store, err := m.exactProjectStore(projectDir)
	if err != nil {
		return err
	}
	return store.SaveMeta(sessionID, session.SessionMeta{SeenToolUseIDs: append([]string(nil), seenToolUseIDs...)})
}

func (m *repositorySessionManager) loadToolUseLedgerFromProject(sessionID, projectDir string) ([]string, error) {
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
	store, err := m.exactProjectStore(projectDir)
	if err != nil {
		return err
	}
	return store.SaveMeta(sessionID, session.SessionMeta{LoadedToolNames: append([]string(nil), loadedToolNames...)})
}

func (m *repositorySessionManager) loadLoadedToolNamesFromProject(sessionID, projectDir string) ([]string, error) {
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
	store, err := m.exactProjectStore(projectDir)
	if err != nil {
		return "", err
	}
	meta, err := store.GetMeta(sessionID)
	if err != nil {
		return "", exactProjectSessionReadError(sessionID, err)
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
	store, err := m.exactProjectStore(projectDir)
	if err != nil {
		return err
	}
	return store.SaveMeta(sessionID, session.SessionMeta{Skills: &cloned})
}

func (m *repositorySessionManager) loadSkillsMetaFromProject(sessionID, projectDir string) (*session.SessionSkillsMeta, error) {
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
	projectDir, err := requireProjectDir(projectDir)
	if err != nil {
		return nil, err
	}
	return m.repo.StoreForProjectDir(projectDir), nil
}

func requireProjectDir(projectDir string) (string, error) {
	projectDir = cleanProjectDir(projectDir)
	if projectDir == "" {
		return "", fmt.Errorf("%w: explicit project directory is required", ErrSessionNotFound)
	}
	return projectDir, nil
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
	store, err := m.exactProjectStore(projectDir)
	if err != nil {
		return err
	}
	stateCopy := cloneGoal(&state)
	return store.SaveMeta(sessionID, session.SessionMeta{Goal: stateCopy})
}

func (m *repositorySessionManager) updateGoalInProject(sessionID, projectDir string, update goal.UpdateFunc) (goal.Goal, error) {
	store, err := m.exactProjectStore(projectDir)
	if err != nil {
		return goal.Goal{}, err
	}
	return store.UpdateGoal(sessionID, update)
}

func (m *repositorySessionManager) deleteFromProject(sessionID, projectDir string) error {
	store, err := m.exactProjectStore(projectDir)
	if err != nil {
		return err
	}
	projectDir = cleanProjectDir(projectDir)
	err = store.Delete(sessionID)
	if err == nil {
		m.generations.remove(generationKey(projectDir, sessionID))
	}
	return err
}

func (m *repositorySessionManager) isDeletedInProject(sessionID, projectDir string) (bool, error) {
	store, err := m.exactProjectStore(projectDir)
	if err != nil {
		return false, err
	}
	return store.IsDeleted(sessionID)
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
	store, err := m.exactProjectStore(m.projectDir())
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrSessionNotFound, err)
	}
	id, err := store.Latest()
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrSessionNotFound, err)
	}
	return id, nil
}

func (m *repositorySessionManager) Delete(sessionID string) error {
	projectDir, err := requireProjectDir(m.projectDir())
	if err != nil {
		return err
	}
	if err := m.repo.StoreForProjectDir(projectDir).Delete(sessionID); err != nil {
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
	return m.saveProviderModelToProject(sessionID, m.projectDir(), providerName, modelID)
}

func (m *repositorySessionManager) ArtifactsDir(sessionID string) string {
	return m.artifactsDirForProject(sessionID, m.projectDir())
}

func (m *repositorySessionManager) artifactsDirForProject(sessionID, projectDir string) string {
	store, err := m.exactProjectStore(projectDir)
	if err != nil {
		return ""
	}
	return store.ArtifactsDir(sessionID)
}

func (m *repositorySessionManager) TranscriptPath(sessionID string) string {
	return m.transcriptPathForProject(sessionID, m.projectDir())
}

func (m *repositorySessionManager) transcriptPathForProject(sessionID, projectDir string) string {
	store, err := m.exactProjectStore(projectDir)
	if err != nil {
		return ""
	}
	return store.TranscriptPath(sessionID)
}

func (m *repositorySessionManager) SaveSessionContext(sessionID, cwd, gitBranch string) error {
	return m.saveSessionContextToProject(sessionID, m.projectDir(), cwd, gitBranch)
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

func generationKey(projectDir, sessionID string) string {
	return filepath.Clean(strings.TrimSpace(projectDir)) + "\x00" + strings.TrimSpace(sessionID)
}
