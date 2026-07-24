package main

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/goal"
	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/sandbox"
	"github.com/agent-dance/luban/session"
	"github.com/agent-dance/luban/types"
)

func TestGoalRuntimeReadsAndWritesSessionMetadata(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD(t.TempDir())
	sessionID := "goal-runtime-session"
	seedGoalRuntimeSession(t, repo, sessionID, projectDir)

	runtime := newDynamicSessionGoalRuntime(
		repo,
		func() string { return sessionID },
		func() string { return projectDir },
	)
	want := mustGoalRuntimeGoal(t, "persist one goal")
	if err := runtime.SaveGoal(want); err != nil {
		t.Fatal(err)
	}

	got, err := runtime.LoadGoal()
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Objective != want.Objective || got.Status != want.Status {
		t.Fatalf("loaded goal = %+v, want %+v", got, want)
	}
	got.Objective = "mutated caller copy"
	reloaded, err := runtime.LoadGoal()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded == nil || reloaded.Objective != want.Objective {
		t.Fatalf("runtime leaked mutable goal alias: %+v", reloaded)
	}

	meta, _, err := repo.GetMeta(sessionID, projectDir)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Goal == nil || meta.Goal.Objective != want.Objective {
		t.Fatalf("persisted session goal = %+v, want %+v", meta.Goal, want)
	}
}

func TestGoalRuntimeFollowsLiveSessionAndProjectClosures(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	projectA := repo.ProjectDirForCWD(t.TempDir())
	projectB := repo.ProjectDirForCWD(t.TempDir())
	sharedID := "same-session-id"
	otherID := "other-session-id"
	seedGoalRuntimeSession(t, repo, sharedID, projectA)
	seedGoalRuntimeSession(t, repo, sharedID, projectB)
	seedGoalRuntimeSession(t, repo, otherID, projectB)

	activeSessionID := sharedID
	activeProjectDir := projectA
	runtime := newDynamicSessionGoalRuntime(
		repo,
		func() string { return activeSessionID },
		func() string { return activeProjectDir },
	)

	goalA := mustGoalRuntimeGoal(t, "project A goal")
	if err := runtime.SaveGoal(goalA); err != nil {
		t.Fatal(err)
	}

	activeProjectDir = projectB
	goalB := mustGoalRuntimeGoal(t, "project B goal")
	if err := runtime.SaveGoal(goalB); err != nil {
		t.Fatal(err)
	}
	loadedB, err := runtime.LoadGoal()
	if err != nil || loadedB == nil || loadedB.Objective != goalB.Objective {
		t.Fatalf("goal after project switch = %+v, err %v", loadedB, err)
	}

	activeSessionID = otherID
	goalC := mustGoalRuntimeGoal(t, "other session goal")
	if err := runtime.SaveGoal(goalC); err != nil {
		t.Fatal(err)
	}
	loadedC, err := runtime.LoadGoal()
	if err != nil || loadedC == nil || loadedC.Objective != goalC.Objective {
		t.Fatalf("goal after session switch = %+v, err %v", loadedC, err)
	}

	assertPersistedGoalRuntimeObjective(t, repo, sharedID, projectA, goalA.Objective)
	assertPersistedGoalRuntimeObjective(t, repo, sharedID, projectB, goalB.Objective)
	assertPersistedGoalRuntimeObjective(t, repo, otherID, projectB, goalC.Objective)
}

func TestGoalRuntimeRejectsMissingRepositoryOrSessionIdentity(t *testing.T) {
	goalState := mustGoalRuntimeGoal(t, "cannot persist without identity")
	tests := []struct {
		name      string
		repo      *session.Repository
		sessionID func() string
		project   func() string
		want      string
	}{
		{
			name:      "nil repository",
			sessionID: func() string { return "session" },
			project:   func() string { return "project" },
			want:      "repository",
		},
		{
			name:    "nil session resolver",
			repo:    session.NewRepository(t.TempDir()),
			project: func() string { return "project" },
			want:    "session",
		},
		{
			name:      "empty session identity",
			repo:      session.NewRepository(t.TempDir()),
			sessionID: func() string { return "  " },
			project:   func() string { return "project" },
			want:      "session",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := newDynamicSessionGoalRuntime(test.repo, test.sessionID, test.project)
			if _, err := runtime.LoadGoal(); err == nil || !strings.Contains(strings.ToLower(err.Error()), test.want) {
				t.Fatalf("LoadGoal error = %v, want explicit %s error", err, test.want)
			}
			if err := runtime.SaveGoal(goalState); err == nil || !strings.Contains(strings.ToLower(err.Error()), test.want) {
				t.Fatalf("SaveGoal error = %v, want explicit %s error", err, test.want)
			}
		})
	}
}

func TestGoalRuntimeSerializesConcurrentCreateAndUpdateTools(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD(t.TempDir())
	sessionID := "concurrent-goal-runtime"
	seedGoalRuntimeSession(t, repo, sessionID, projectDir)
	runtime := newDynamicSessionGoalRuntime(
		repo,
		func() string { return sessionID },
		func() string { return projectDir },
	)
	initial := mustGoalRuntimeGoal(t, "already completed goal")
	initial, err := goal.Achieve(initial, "seed terminal state", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.SaveGoal(initial); err != nil {
		t.Fatal(err)
	}

	deps := SetupRegistry(provider.NewProviderRef(nil), t.TempDir(), nil, sandbox.NoopBackend{}, nil)
	cleanupGoalRuntimeRegistry(t, deps)
	deps.SetGoalRuntime(runtime)

	type outcome struct {
		name    string
		isError bool
		err     error
	}
	start := make(chan struct{})
	outcomes := make(chan outcome, 2)
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		<-start
		result, runErr := deps.Registry.ExecuteToolWithError(context.Background(), "CreateGoal", map[string]any{
			"objective": "concurrent replacement", "acceptance_criteria": []string{"replacement is verified"},
		})
		outcomes <- outcome{name: "create", isError: result.IsError, err: runErr}
	}()
	go func() {
		defer workers.Done()
		<-start
		result, runErr := deps.Registry.ExecuteToolWithError(context.Background(), "UpdateGoal", map[string]any{
			"status": "complete",
		})
		outcomes <- outcome{name: "update", isError: result.IsError, err: runErr}
	}()
	close(start)
	workers.Wait()
	close(outcomes)

	updateFailed := false
	for result := range outcomes {
		if result.err != nil {
			t.Fatalf("%s infrastructure error: %v", result.name, result.err)
		}
		switch result.name {
		case "create":
			if result.isError {
				t.Fatal("CreateGoal must succeed from the initial terminal state")
			}
		case "update":
			updateFailed = result.isError
		}
	}

	final, err := runtime.LoadGoal()
	if err != nil {
		t.Fatal(err)
	}
	if final == nil || final.Objective != "concurrent replacement" {
		t.Fatalf("concurrent CreateGoal state was lost: %+v", final)
	}
	if updateFailed && final.Status != goal.StatusActive {
		t.Fatalf("failed UpdateGoal left status %s, want active", final.Status)
	}
	if !updateFailed && final.Status != goal.StatusAchieved {
		t.Fatalf("successful UpdateGoal left status %s, want achieved", final.Status)
	}
	assertPersistedGoalRuntimeObjective(t, repo, sessionID, projectDir, "concurrent replacement")
}

func TestGoalRuntimeFeedsRootRegistryGoalTools(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD(t.TempDir())
	sessionID := "root-registry-goal-runtime"
	seedGoalRuntimeSession(t, repo, sessionID, projectDir)
	runtime := newDynamicSessionGoalRuntime(
		repo,
		func() string { return sessionID },
		func() string { return projectDir },
	)

	deps := SetupRegistry(provider.NewProviderRef(nil), t.TempDir(), nil, sandbox.NoopBackend{}, nil)
	cleanupGoalRuntimeRegistry(t, deps)
	deps.SetGoalRuntime(runtime)
	result, err := deps.Registry.ExecuteToolWithError(context.Background(), "CreateGoal", map[string]any{
		"objective":           "prove dynamic root wiring",
		"acceptance_criteria": []string{"dynamic root wiring is verified"},
		"token_budget":        2048,
	})
	if err != nil || result.IsError {
		t.Fatalf("CreateGoal through root registry: result=%+v err=%v", result, err)
	}

	meta, _, err := repo.GetMeta(sessionID, projectDir)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Goal == nil || meta.Goal.Objective != "prove dynamic root wiring" || meta.Goal.TokenBudget != 2048 {
		t.Fatalf("root Goal tools persisted state = %+v", meta.Goal)
	}
}

func TestGoalRuntimeRoutesBackgroundToolExecutionAwayFromFocusedSession(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	workspaceA := t.TempDir()
	workspaceB := t.TempDir()
	projectA := repo.ProjectDirForCWD(workspaceA)
	projectB := repo.ProjectDirForCWD(workspaceB)
	sessionA := "background-session-a"
	sessionB := "focused-session-b"
	seedGoalRuntimeSession(t, repo, sessionA, projectA)
	seedGoalRuntimeSession(t, repo, sessionB, projectB)

	runtime := newDynamicSessionGoalRuntime(
		repo,
		func() string { return sessionB },
		func() string { return projectB },
	)
	focusedGoal := mustGoalRuntimeGoal(t, "preserve focused session B")
	focusedGoal, err := goal.Achieve(focusedGoal, "seed terminal state", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.SaveGoal(focusedGoal); err != nil {
		t.Fatal(err)
	}

	deps := SetupRegistry(provider.NewProviderRef(nil), t.TempDir(), nil, sandbox.NoopBackend{}, nil)
	cleanupGoalRuntimeRegistry(t, deps)
	deps.SetGoalRuntime(runtime)
	backgroundCtx := loop.WithToolExecutionContext(context.Background(), loop.ToolExecutionContext{
		SessionID:         sessionA,
		SessionProjectDir: projectA,
		ProjectRoot:       workspaceA,
		CWD:               workspaceA,
		ActorID:           "background-worker",
		ActorType:         "background",
	})
	result, err := deps.Registry.ExecuteToolWithError(backgroundCtx, "CreateGoal", map[string]any{
		"objective": "persist only in background session A", "acceptance_criteria": []string{"only background session A changes"},
	})
	if err != nil || result.IsError {
		t.Fatalf("background CreateGoal: result=%+v err=%v", result, err)
	}

	metaA, _, err := repo.GetMeta(sessionA, projectA)
	if err != nil {
		t.Fatal(err)
	}
	metaB, _, err := repo.GetMeta(sessionB, projectB)
	if err != nil {
		t.Fatal(err)
	}
	if metaA.Goal == nil || metaA.Goal.Objective != "persist only in background session A" ||
		metaB.Goal == nil || metaB.Goal.Objective != focusedGoal.Objective || metaB.Goal.Status != focusedGoal.Status {
		t.Fatalf("background routing persisted A=%+v B=%+v; want new A goal and unchanged focused B=%+v", metaA.Goal, metaB.Goal, focusedGoal)
	}
}

func TestGoalRuntimeToolContextFallsBackToProjectRootMapping(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	workspace := t.TempDir()
	projectDir := repo.ProjectDirForCWD(workspace)
	const sessionID = "goal-runtime-project-root-fallback"
	seedGoalRuntimeSession(t, repo, sessionID, projectDir)
	runtime := newDynamicSessionGoalRuntime(
		repo,
		func() string { return "focused-session" },
		func() string { return repo.ProjectDirForCWD(t.TempDir()) },
	)
	ctx := loop.WithToolExecutionContext(context.Background(), loop.ToolExecutionContext{
		SessionID:   sessionID,
		ProjectRoot: workspace,
	})
	want := mustGoalRuntimeGoal(t, "preserve legacy project-root routing")
	if err := runtime.SaveGoalForContext(ctx, want); err != nil {
		t.Fatalf("save contextual goal through compatibility mapping: %v", err)
	}
	assertPersistedGoalRuntimeObjective(t, repo, sessionID, projectDir, want.Objective)
}

func seedGoalRuntimeSession(t *testing.T, repo *session.Repository, sessionID, projectDir string) {
	t.Helper()
	if err := repo.Save(sessionID, projectDir, []types.Message{types.UserMessage("seed")}); err != nil {
		t.Fatal(err)
	}
}

func mustGoalRuntimeGoal(t *testing.T, objective string) goal.Goal {
	t.Helper()
	created, err := goal.Create(objective, 0, time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	created, err = goal.RecordAcceptanceEvaluation(created, created.Revision, []goal.AcceptanceCriterionEvaluation{{
		CriterionID: "AC-1", Met: true, Reason: "verified",
	}}, "verified", created.CreatedAt)
	if err != nil {
		t.Fatal(err)
	}
	return created
}

func assertPersistedGoalRuntimeObjective(t *testing.T, repo *session.Repository, sessionID, projectDir, want string) {
	t.Helper()
	meta, _, err := repo.GetMeta(sessionID, projectDir)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Goal == nil || meta.Goal.Objective != want {
		t.Fatalf("persisted goal for %s in %s = %+v, want objective %q", sessionID, projectDir, meta.Goal, want)
	}
}

func cleanupGoalRuntimeRegistry(t *testing.T, deps *RegistryDeps) {
	t.Helper()
	if deps.CronStore != nil {
		t.Cleanup(deps.CronStore.Stop)
	}
	t.Cleanup(deps.StopWebFetchCache)
}
