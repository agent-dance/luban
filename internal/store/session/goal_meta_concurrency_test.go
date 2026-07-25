package session

import (
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/internal/runtime/goal"
	"github.com/agent-dance/luban/types"
)

func TestRepositoryConcurrentPartialMetaUpdatesShareProjectSynchronization(t *testing.T) {
	repo := NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD("/workspace/concurrent-goal-meta")
	const sessionID = "concurrent-goal-meta"
	if err := repo.Save(sessionID, projectDir, []types.Message{types.UserMessage("keep every metadata field")}); err != nil {
		t.Fatal(err)
	}

	goalState, err := goal.CreateWithCriteria("finish concurrent metadata persistence", []string{"finish concurrent metadata persistence"}, 2048, time.Unix(100, 0))
	if err != nil {
		t.Fatal(err)
	}
	usage := &SessionUsageMeta{InputTokens: 17, OutputTokens: 9}
	presentation := &SessionPresentationMeta{PermissionMode: "plan"}
	wantSeen := []string{"tool-a", "tool-z"}
	updates := []SessionMeta{
		{Goal: &goalState},
		{Usage: usage},
		{Presentation: presentation},
		{SeenToolUseIDs: []string{"tool-z", "tool-a", "tool-z"}},
	}

	const copiesPerField = 16
	start := make(chan struct{})
	errs := make(chan error, len(updates)*copiesPerField)
	var writers sync.WaitGroup
	for _, update := range updates {
		for range copiesPerField {
			update := update
			writers.Add(1)
			go func() {
				defer writers.Done()
				<-start
				errs <- repo.SaveMeta(sessionID, projectDir, update)
			}()
		}
	}
	close(start)
	writers.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent SaveMeta returned an error: %v", err)
		}
	}

	got, _, err := repo.GetMeta(sessionID, projectDir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Goal == nil || !reflect.DeepEqual(*got.Goal, goalState) {
		t.Fatalf("goal = %+v, want %+v", got.Goal, goalState)
	}
	if !reflect.DeepEqual(got.Usage, usage) {
		t.Fatalf("usage = %+v, want %+v", got.Usage, usage)
	}
	if !reflect.DeepEqual(got.Presentation, presentation) {
		t.Fatalf("presentation = %+v, want %+v", got.Presentation, presentation)
	}
	if !reflect.DeepEqual(got.SeenToolUseIDs, wantSeen) {
		t.Fatalf("seen tool IDs = %v, want %v", got.SeenToolUseIDs, wantSeen)
	}
}

func TestFileStoreConcurrentMetadataWritesUseIndependentTemporaryFiles(t *testing.T) {
	dir := t.TempDir()
	seed := NewFileStore(dir)
	const sessionID = "independent-meta-writers"
	if err := seed.Save(sessionID, []types.Message{types.UserMessage("publish metadata atomically")}); err != nil {
		t.Fatal(err)
	}

	const writerCount = 64
	start := make(chan struct{})
	errs := make(chan error, writerCount)
	var writers sync.WaitGroup
	for i := range writerCount {
		store := NewFileStore(dir)
		writers.Add(1)
		go func() {
			defer writers.Done()
			<-start
			errs <- store.SaveMeta(sessionID, SessionMeta{Title: fmt.Sprintf("writer-%d", i)})
		}()
	}
	close(start)
	writers.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent SaveMeta returned an error: %v", err)
		}
	}
	if _, err := seed.GetMeta(sessionID); err != nil {
		t.Fatalf("metadata was not readable after concurrent writes: %v", err)
	}
}
