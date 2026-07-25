package app

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"sync"

	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/runtime/goal"
	"github.com/agent-dance/luban/internal/store/session"
)

type dynamicSessionGoalRuntime struct {
	mu         sync.Mutex
	repo       *session.Repository
	sessionID  func() string
	projectDir func() string
}

func newDynamicSessionGoalRuntime(repo *session.Repository, sessionID, projectDir func() string) *dynamicSessionGoalRuntime {
	return &dynamicSessionGoalRuntime{
		repo:       repo,
		sessionID:  sessionID,
		projectDir: projectDir,
	}
}

func (r *dynamicSessionGoalRuntime) LoadGoal() (*goal.Goal, error) {
	if r == nil {
		return nil, rootRuntimeError(i18n.KeyRootGoalRuntimeRepositoryMissing)
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	repo, sessionID, projectDir, err := r.resolve()
	if err != nil {
		return nil, err
	}
	return loadDynamicGoal(repo, sessionID, projectDir)
}

func (r *dynamicSessionGoalRuntime) LoadGoalForContext(ctx context.Context) (*goal.Goal, error) {
	if r == nil {
		return nil, rootRuntimeError(i18n.KeyRootGoalRuntimeRepositoryMissing)
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	repo, sessionID, projectDir, err := r.resolveContext(ctx)
	if err != nil {
		return nil, err
	}
	return loadDynamicGoal(repo, sessionID, projectDir)
}

func loadDynamicGoal(repo *session.Repository, sessionID, projectDir string) (*goal.Goal, error) {
	meta, _, err := repo.GetMeta(sessionID, projectDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, rootRuntimeWrap(i18n.KeyRootGoalRuntimeMetadataLoad, err)
	}
	return cloneDynamicGoal(meta.Goal), nil
}

func (r *dynamicSessionGoalRuntime) SaveGoal(next goal.Goal) error {
	if r == nil {
		return rootRuntimeError(i18n.KeyRootGoalRuntimeRepositoryMissing)
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	repo, sessionID, projectDir, err := r.resolve()
	if err != nil {
		return err
	}
	return saveDynamicGoal(repo, sessionID, projectDir, next)
}

func (r *dynamicSessionGoalRuntime) SaveGoalForContext(ctx context.Context, next goal.Goal) error {
	if r == nil {
		return rootRuntimeError(i18n.KeyRootGoalRuntimeRepositoryMissing)
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	repo, sessionID, projectDir, err := r.resolveContext(ctx)
	if err != nil {
		return err
	}
	return saveDynamicGoal(repo, sessionID, projectDir, next)
}

func (r *dynamicSessionGoalRuntime) UpdateGoal(update goal.UpdateFunc) (goal.Goal, error) {
	if r == nil {
		return goal.Goal{}, rootRuntimeError(i18n.KeyRootGoalRuntimeRepositoryMissing)
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	repo, sessionID, projectDir, err := r.resolve()
	if err != nil {
		return goal.Goal{}, err
	}
	return updateDynamicGoal(repo, sessionID, projectDir, update)
}

func (r *dynamicSessionGoalRuntime) UpdateGoalForContext(ctx context.Context, update goal.UpdateFunc) (goal.Goal, error) {
	if r == nil {
		return goal.Goal{}, rootRuntimeError(i18n.KeyRootGoalRuntimeRepositoryMissing)
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	repo, sessionID, projectDir, err := r.resolveContext(ctx)
	if err != nil {
		return goal.Goal{}, err
	}
	return updateDynamicGoal(repo, sessionID, projectDir, update)
}

func saveDynamicGoal(repo *session.Repository, sessionID, projectDir string, next goal.Goal) error {
	saved := cloneDynamicGoal(&next)
	meta := session.SessionMeta{Goal: saved}
	if err := repo.SaveMeta(sessionID, projectDir, meta); err == nil {
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return rootRuntimeWrap(i18n.KeyRootGoalRuntimeMetadataSave, err)
	}

	if err := repo.Save(sessionID, projectDir, nil); err != nil {
		return rootRuntimeWrap(i18n.KeyRootGoalRuntimeTranscriptCreate, err)
	}
	if err := repo.SaveMeta(sessionID, projectDir, meta); err != nil {
		return rootRuntimeWrap(i18n.KeyRootGoalRuntimeMetadataSave, err)
	}
	return nil
}

func updateDynamicGoal(repo *session.Repository, sessionID, projectDir string, update goal.UpdateFunc) (goal.Goal, error) {
	next, err := repo.UpdateGoal(sessionID, projectDir, update)
	if err == nil {
		return next, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return goal.Goal{}, rootRuntimeWrap(i18n.KeyRootGoalRuntimeMetadataUpdate, err)
	}
	if err := repo.Save(sessionID, projectDir, nil); err != nil {
		return goal.Goal{}, rootRuntimeWrap(i18n.KeyRootGoalRuntimeTranscriptCreate, err)
	}
	next, err = repo.UpdateGoal(sessionID, projectDir, update)
	if err != nil {
		return goal.Goal{}, rootRuntimeWrap(i18n.KeyRootGoalRuntimeMetadataUpdate, err)
	}
	return next, nil
}

func (r *dynamicSessionGoalRuntime) resolve() (*session.Repository, string, string, error) {
	if r == nil || r.repo == nil {
		return nil, "", "", rootRuntimeError(i18n.KeyRootGoalRuntimeRepositoryMissing)
	}
	if r.sessionID == nil {
		return nil, "", "", rootRuntimeError(i18n.KeyRootGoalRuntimeSessionResolver)
	}
	sessionID := strings.TrimSpace(r.sessionID())
	if sessionID == "" {
		return nil, "", "", rootRuntimeError(i18n.KeyRootGoalRuntimeSessionID)
	}
	if r.projectDir == nil {
		return nil, "", "", rootRuntimeError(i18n.KeyRootGoalRuntimeProjectResolver)
	}
	projectDir := strings.TrimSpace(r.projectDir())
	if projectDir == "" {
		return nil, "", "", rootRuntimeError(i18n.KeyRootGoalRuntimeProjectDirectory)
	}
	return r.repo, sessionID, projectDir, nil
}

func (r *dynamicSessionGoalRuntime) resolveContext(ctx context.Context) (*session.Repository, string, string, error) {
	if r == nil || r.repo == nil {
		return nil, "", "", rootRuntimeError(i18n.KeyRootGoalRuntimeRepositoryMissing)
	}
	exec, ok := executioncontract.ToolExecutionContextFromContext(ctx)
	if !ok {
		return nil, "", "", rootRuntimeError(i18n.KeyRootGoalRuntimeExecutionContext)
	}
	sessionID := strings.TrimSpace(exec.SessionID)
	if sessionID == "" {
		return nil, "", "", rootRuntimeError(i18n.KeyRootGoalRuntimeSessionID)
	}
	if projectDir := strings.TrimSpace(exec.SessionProjectDir); projectDir != "" {
		return r.repo, sessionID, projectDir, nil
	}
	projectRoot := strings.TrimSpace(exec.ProjectRoot)
	if projectRoot == "" {
		return nil, "", "", rootRuntimeError(i18n.KeyRootGoalRuntimeProjectRoot)
	}
	return r.repo, sessionID, r.repo.ProjectDirForCWD(projectRoot), nil
}

func cloneDynamicGoal(current *goal.Goal) *goal.Goal {
	if current == nil {
		return nil
	}
	cloned := *current
	if current.AchievedAt != nil {
		achievedAt := *current.AchievedAt
		cloned.AchievedAt = &achievedAt
	}
	if current.BlockedAt != nil {
		blockedAt := *current.BlockedAt
		cloned.BlockedAt = &blockedAt
	}
	return &cloned
}

var _ goal.Updater = (*dynamicSessionGoalRuntime)(nil)
var _ goal.ContextUpdater = (*dynamicSessionGoalRuntime)(nil)
