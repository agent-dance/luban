package engine

import (
	"errors"
	"testing"

	"github.com/agent-dance/luban/internal/store/session"
	"github.com/agent-dance/luban/types"
)

func TestEngineSessionAdapterRejectsCommitFromPreparedOldGeneration(t *testing.T) {
	dir := t.TempDir()
	seed := session.NewFileStore(dir)
	const sessionID = "engine-generation-fence"
	initial := []types.Message{types.UserMessage("initial")}
	if err := seed.Save(sessionID, initial); err != nil {
		t.Fatal(err)
	}
	manager := newFileSessionManager(dir)
	if err := manager.prepareContextGeneration(sessionID, ""); err != nil {
		t.Fatal(err)
	}
	prepared, err := seed.GetCompactionManifest(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	concurrent := append(initial, types.AssistantMessage("newer writer"))
	if _, err := seed.CommitModelContext(sessionID, prepared.ContextGeneration, concurrent, []types.Message{concurrent[1]}); err != nil {
		t.Fatal(err)
	}
	late := append(initial, types.AssistantMessage("late old writer"))
	if err := manager.Save(sessionID, late); !errors.Is(err, session.ErrStaleContextGeneration) {
		t.Fatalf("late Save error = %v, want ErrStaleContextGeneration", err)
	}
	loaded, err := seed.Load(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded[len(loaded)-1].GetText() != "newer writer" {
		t.Fatalf("late writer replaced newer generation: %#v", loaded)
	}
}

func TestGenerationTrackerAdvancesOnPostPublishError(t *testing.T) {
	var tracker sessionGenerationTracker
	const key = "project\x00session"
	tracker.store(key, ContextGenerationState{Generation: 4, Persisted: true})
	manifest := session.CompactionManifestV2{ContextGeneration: 5, Digest: "sha256:committed"}
	injected := errors.New("metadata fsync failed")
	err := tracker.recordSaveResult(key, manifest, &session.ContextCommitError{Manifest: manifest, Cause: injected})
	if !errors.Is(err, injected) {
		t.Fatalf("recordSaveResult error = %v", err)
	}
	if state, ok := tracker.load(key); !ok || state.Generation != 5 || !state.Persisted {
		t.Fatalf("tracked generation = %+v, %v; want committed generation 5", state, ok)
	}
}

func TestGenerationTrackerFailsClosedWhenObservedManifestDisappears(t *testing.T) {
	dir := t.TempDir()
	store := session.NewFileStore(dir)
	const sessionID = "generation-manifest-disappears"
	if err := store.Save(sessionID, []types.Message{types.UserMessage("seed")}); err != nil {
		t.Fatal(err)
	}
	var tracker sessionGenerationTracker
	const key = "project\x00generation-manifest-disappears"
	state, err := tracker.current(store, key, sessionID)
	if err != nil || !state.Persisted || state.Generation != 1 {
		t.Fatalf("initial state = %+v, %v", state, err)
	}
	if err := store.Delete(sessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := tracker.current(store, key, sessionID); err == nil {
		t.Fatal("observed durable generation silently became unpersisted")
	}
}

func TestCoreEngineExposesAuthoritativeContextGeneration(t *testing.T) {
	dir := t.TempDir()
	store := session.NewFileStore(dir)
	const sessionID = "generation-provider"
	if err := store.Save(sessionID, []types.Message{types.UserMessage("seed")}); err != nil {
		t.Fatal(err)
	}
	manager := newFileSessionManager(dir)
	engine, err := New(Config{Provider: &mockProvider{name: "mock", modelID: "model"}, Sessions: manager})
	if err != nil {
		t.Fatal(err)
	}
	state, err := engine.ContextGenerationState(sessionID)
	if err != nil || !state.Persisted || state.Generation != 1 {
		t.Fatalf("ContextGenerationState = %+v, %v; want persisted generation 1", state, err)
	}
}
