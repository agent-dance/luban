package engine

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/internal/store/session"
	"github.com/agent-dance/luban/types"
)

func (e *CoreEngine) ResumeWithRuntimeContext(ctx context.Context, sessionID, projectDir string, runtime RuntimeContext) (int, error) {
	prepared, err := e.PrepareRuntimeContextResume(ctx, sessionID, projectDir, runtime)
	if err != nil {
		return 0, err
	}
	if err := prepared.Commit(); err != nil {
		prepared.Abort()
		return 0, err
	}
	return prepared.MessageCount(), nil
}

func (e *CoreEngine) saveConversation(sessionID string, conv *conversation) error {
	return e.saveConversationWithContext(context.Background(), sessionID, conv)
}

type fileSessionManager struct {
	store       *session.FileStore
	dir         string
	generations sessionGenerationTracker
}

func newFileSessionManager(dir string) *fileSessionManager {
	return &fileSessionManager{store: session.NewFileStore(dir), dir: dir}
}

func (m *fileSessionManager) Save(sessionID string, messages []types.Message) error {
	return m.generations.save(m.store, generationKey(m.dir, sessionID), sessionID, messages)
}

func (m *fileSessionManager) prepareContextGeneration(sessionID, _ string) error {
	return m.generations.prepare(m.store, generationKey(m.dir, sessionID), sessionID)
}

func (m *fileSessionManager) contextGenerationState(sessionID, _ string) (ContextGenerationState, error) {
	return m.generations.current(m.store, generationKey(m.dir, sessionID), sessionID)
}

func (m *fileSessionManager) internalControlScope(sessionID, _ string) (messagecontrol.Scope, error) {
	return m.store.MessageControlScope(sessionID)
}

func (m *fileSessionManager) Load(sessionID string) ([]types.Message, error) {
	messages, err := m.store.Load(sessionID)
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
	return messages, nil
}

func (m *fileSessionManager) List() ([]SessionInfo, error) {
	infos, err := m.store.List()
	if err != nil {
		return nil, err
	}
	out := make([]SessionInfo, len(infos))
	for index, info := range infos {
		out[index] = SessionInfo{ID: info.ID, UpdatedAt: info.UpdatedAt.Unix(), Messages: info.MessageCount}
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

func (m *fileSessionManager) isDeleted(sessionID string) (bool, error) {
	return m.store.IsDeleted(sessionID)
}

func (m *fileSessionManager) SaveProviderModel(sessionID, providerName, modelID string) error {
	return m.store.SaveMeta(sessionID, session.SessionMeta{Provider: providerName, Model: modelID})
}

func (m *fileSessionManager) ArtifactsDir(sessionID string) string {
	return filepath.Join(m.dir, sessionID)
}

func (m *fileSessionManager) TranscriptPath(sessionID string) string {
	return m.store.TranscriptPath(sessionID)
}

func (m *fileSessionManager) SaveSessionContext(sessionID, cwd, gitBranch string) error {
	return m.store.SaveMeta(sessionID, session.SessionMeta{CWD: cwd, GitBranch: gitBranch})
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

func (m *fileSessionManager) SaveLoadedToolNames(sessionID string, names []string) error {
	return m.store.SaveMeta(sessionID, session.SessionMeta{LoadedToolNames: append([]string(nil), names...)})
}

func (m *fileSessionManager) LoadLoadedToolNames(sessionID string) ([]string, error) {
	meta, err := m.store.GetMeta(sessionID)
	if err != nil {
		return nil, err
	}
	return append([]string(nil), meta.LoadedToolNames...), nil
}

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
