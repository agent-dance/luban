package app

import (
	"errors"
	"testing"

	"github.com/agent-dance/luban/internal/runtime/engine"
	"github.com/agent-dance/luban/internal/store/session"
	"github.com/agent-dance/luban/types"
)

type scopedGenerationProbeEngine struct {
	engine.Engine
	state         engine.ContextGenerationState
	err           error
	unscopedCalls int
}

func (e *scopedGenerationProbeEngine) ContextGenerationStateForSession(string, string) (engine.ContextGenerationState, error) {
	return e.state, e.err
}

func (e *scopedGenerationProbeEngine) ContextGenerationState(string) (engine.ContextGenerationState, error) {
	e.unscopedCalls++
	return engine.ContextGenerationState{Generation: 99, Persisted: true}, nil
}

func TestTUIContextGenerationExactScopePropagatesErrorWithoutFallback(t *testing.T) {
	want := errors.New("corrupt manifest")
	eng := &scopedGenerationProbeEngine{err: want}
	if _, err := tuiContextGeneration(eng, "session", "/exact/project"); !errors.Is(err, want) {
		t.Fatalf("generation error = %v, want %v", err, want)
	}
	if eng.unscopedCalls != 0 {
		t.Fatalf("exact project lookup fell back to unscoped provider %d times", eng.unscopedCalls)
	}
}

func TestTUIContextGenerationRejectsInvalidExplicitState(t *testing.T) {
	eng := &scopedGenerationProbeEngine{state: engine.ContextGenerationState{Generation: 0, Persisted: true}}
	if _, err := tuiContextGeneration(eng, "session", "/exact/project"); !errors.Is(err, session.ErrCorruptSessionHistory) {
		t.Fatalf("invalid generation state error = %v, want ErrCorruptSessionHistory", err)
	}
}

type unscopedOnlyGenerationEngine struct {
	engine.Engine
	calls int
}

func (e *unscopedOnlyGenerationEngine) ContextGenerationState(string) (engine.ContextGenerationState, error) {
	e.calls++
	return engine.ContextGenerationState{Generation: 7, Persisted: true}, nil
}

func TestTUIContextGenerationForbidsUnscopedFallbackForProject(t *testing.T) {
	eng := &unscopedOnlyGenerationEngine{}
	if _, err := tuiContextGeneration(eng, "session", "/exact/project"); !errors.Is(err, engine.ErrContextGenerationUnavailable) {
		t.Fatalf("exact project generation error = %v, want ErrContextGenerationUnavailable", err)
	}
	if eng.calls != 0 {
		t.Fatalf("unscoped provider called %d times for exact project", eng.calls)
	}
}

func TestPrepareTUISessionSnapshotUsesRepositoryGenerationWithoutEngine(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD(t.TempDir())
	const sessionID = "repo-generation-fallback"
	messages := []types.Message{types.UserMessage("current transcript")}
	if err := repo.Save(sessionID, projectDir, messages); err != nil {
		t.Fatal(err)
	}
	saveCanonicalTUISessionCheckpoint(t, repo, sessionID, projectDir, messages, 1)
	scope, err := repo.StoreForProjectDir(projectDir).MessageControlScope(sessionID)
	if err != nil || !scope.Bound() {
		t.Fatalf("scope = %#v err=%v", scope, err)
	}
	snapshot, err := prepareTUISessionSnapshot(
		TUIREPLConfig{Repo: repo}, sessionID, projectDir, 1, messages,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.ContextGenerationPersisted || snapshot.ContextGeneration != scope.ContextGeneration() {
		t.Fatalf("snapshot generation = %d persisted=%v, want %d", snapshot.ContextGeneration, snapshot.ContextGenerationPersisted, scope.ContextGeneration())
	}
}

func TestPrepareTUISessionSnapshotRejectsEngineRepositoryGenerationConflict(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD(t.TempDir())
	const sessionID = "generation-source-conflict"
	messages := []types.Message{types.UserMessage("current transcript")}
	if err := repo.Save(sessionID, projectDir, messages); err != nil {
		t.Fatal(err)
	}
	scope, err := repo.StoreForProjectDir(projectDir).MessageControlScope(sessionID)
	if err != nil || !scope.Bound() {
		t.Fatalf("scope = %#v err=%v", scope, err)
	}
	eng := &scopedGenerationProbeEngine{state: engine.ContextGenerationState{
		Generation: scope.ContextGeneration() + 1, Persisted: true,
	}}
	_, err = prepareTUISessionSnapshot(
		TUIREPLConfig{Engine: eng, Repo: repo}, sessionID, projectDir, 1, messages,
	)
	if !errors.Is(err, session.ErrCorruptSessionHistory) {
		t.Fatalf("generation conflict error = %v, want ErrCorruptSessionHistory", err)
	}
}
