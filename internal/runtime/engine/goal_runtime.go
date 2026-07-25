package engine

import (
	"github.com/agent-dance/luban/internal/runtime/goal"
	"github.com/agent-dance/luban/internal/runtime/loop"
)

type projectGoalStore interface {
	loadGoalFromProject(sessionID, projectDir string) (*goal.Goal, error)
	saveGoalToProject(sessionID, projectDir string, state goal.Goal) error
	updateGoalInProject(sessionID, projectDir string, update goal.UpdateFunc) (goal.Goal, error)
}

type sessionGoalRuntime struct {
	store      projectGoalStore
	sessionID  string
	projectDir string
}

func newSessionGoalRuntime(manager SessionManager, sessionID, projectDir string) loop.GoalRuntime {
	store, ok := manager.(projectGoalStore)
	if !ok {
		return nil
	}
	return &sessionGoalRuntime{store: store, sessionID: sessionID, projectDir: projectDir}
}

func (r *sessionGoalRuntime) LoadGoal() (*goal.Goal, error) {
	return r.store.loadGoalFromProject(r.sessionID, r.projectDir)
}

func (r *sessionGoalRuntime) SaveGoal(state goal.Goal) error {
	return r.store.saveGoalToProject(r.sessionID, r.projectDir, state)
}

func (r *sessionGoalRuntime) UpdateGoal(update goal.UpdateFunc) (goal.Goal, error) {
	return r.store.updateGoalInProject(r.sessionID, r.projectDir, update)
}

func cloneGoal(state *goal.Goal) *goal.Goal {
	if state == nil {
		return nil
	}
	cloned := *state
	if state.AchievedAt != nil {
		achievedAt := *state.AchievedAt
		cloned.AchievedAt = &achievedAt
	}
	if state.BlockedAt != nil {
		blockedAt := *state.BlockedAt
		cloned.BlockedAt = &blockedAt
	}
	return &cloned
}

var _ loop.GoalRuntime = (*sessionGoalRuntime)(nil)
var _ goal.Updater = (*sessionGoalRuntime)(nil)
var _ projectGoalStore = (*repositorySessionManager)(nil)
