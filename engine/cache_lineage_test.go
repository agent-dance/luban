package engine

import (
	"context"
	"testing"
	"time"

	"github.com/agent-dance/luban/session"
	"github.com/agent-dance/luban/types"
)

func TestForkCacheLineageReachesProviderAfterResume(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD(t.TempDir())
	source := session.Ref{ID: "cache-lineage-source", ProjectDir: projectDir}
	messages := []types.Message{types.UserMessage("source prompt")}
	if err := repo.Save(source.ID, projectDir, messages); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveMeta(source.ID, projectDir, session.SessionMeta{CacheLineageID: "stable-cache-lineage"}); err != nil {
		t.Fatal(err)
	}
	fork, err := repo.Fork(source, messages)
	if err != nil {
		t.Fatal(err)
	}

	prov := &mockProvider{name: "openai", modelID: "gpt-4o"}
	eng, err := New(Config{
		Provider: prov,
		Sessions: NewRepositorySessionManager(repo, func() string { return projectDir }),
		MaxTurns: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Resume(context.Background(), fork.ID); err != nil {
		t.Fatal(err)
	}
	events, err := eng.Query(context.Background(), QueryRequest{SessionID: fork.ID, Message: "continue"})
	if err != nil {
		t.Fatal(err)
	}
	drainEvents(t, events, 2*time.Second)

	prov.mu.Lock()
	params := prov.lastParams
	prov.mu.Unlock()
	if params.PromptCacheKey != "stable-cache-lineage" || !params.UsePromptCache {
		t.Fatalf("provider cache lineage = %q enabled=%v, want stable-cache-lineage/true", params.PromptCacheKey, params.UsePromptCache)
	}
}
