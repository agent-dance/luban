package session

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/agent-dance/luban/internal/runtime/goal"
	"github.com/agent-dance/luban/types"
)

func TestRepositoryUpdateGoalSerializesReadModifyWriteWithSaveMeta(t *testing.T) {
	repo := NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD("/workspace/atomic-goal")
	const sessionID = "atomic-goal"
	if err := repo.Save(sessionID, projectDir, []types.Message{types.UserMessage("preserve metadata")}); err != nil {
		t.Fatal(err)
	}

	createdAt := time.Date(2026, time.July, 14, 12, 0, 0, 0, time.UTC)
	active, err := goal.CreateWithCriteria("finish the atomic goal update", []string{"finish the atomic goal update"}, 4096, createdAt)
	if err != nil {
		t.Fatal(err)
	}
	usage := &SessionUsageMeta{InputTokens: 17, OutputTokens: 9}
	if err := repo.SaveMeta(sessionID, projectDir, SessionMeta{
		Title: "original title",
		Goal:  &active,
		Usage: usage,
	}); err != nil {
		t.Fatal(err)
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	type updateResult struct {
		goal goal.Goal
		err  error
	}
	updated := make(chan updateResult, 1)
	update := goal.UpdateFunc(func(current *goal.Goal) (goal.Goal, error) {
		if current == nil {
			return goal.Goal{}, fmt.Errorf("atomic update received nil goal")
		}
		close(entered)
		<-release
		return goal.Pause(*current, createdAt.Add(time.Minute))
	})
	go func() {
		next, updateErr := repo.UpdateGoal(sessionID, projectDir, update)
		updated <- updateResult{goal: next, err: updateErr}
	}()

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("UpdateGoal did not invoke callback")
	}

	cleared, err := goal.Clear(active, createdAt.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	saveStarted := make(chan struct{})
	saveDone := make(chan error, 1)
	go func() {
		close(saveStarted)
		saveDone <- repo.SaveMeta(sessionID, projectDir, SessionMeta{
			Title: "concurrent title",
			Goal:  &cleared,
		})
	}()
	<-saveStarted

	// The updater callback is part of the atomic transaction. SaveMeta shares
	// the FileStore lock and therefore cannot publish until the callback returns.
	saveCompletedBeforeRelease := false
	select {
	case err := <-saveDone:
		saveCompletedBeforeRelease = true
		if err != nil {
			t.Errorf("concurrent SaveMeta: %v", err)
		}
	case <-time.After(100 * time.Millisecond):
	}
	close(release)

	result := <-updated
	if result.err != nil {
		t.Fatalf("UpdateGoal: %v", result.err)
	}
	if result.goal.Status != goal.StatusPaused {
		t.Fatalf("UpdateGoal result status = %q, want paused", result.goal.Status)
	}
	if !saveCompletedBeforeRelease {
		if err := <-saveDone; err != nil {
			t.Fatalf("concurrent SaveMeta: %v", err)
		}
	}
	if saveCompletedBeforeRelease {
		t.Fatal("SaveMeta completed while UpdateGoal callback was still running; goal read-modify-write is not protected by the shared FileStore lock")
	}

	meta, _, err := repo.GetMeta(sessionID, projectDir)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Goal == nil || meta.Goal.Status != goal.StatusCleared {
		t.Fatalf("final goal = %+v, want the later concurrent clear", meta.Goal)
	}
	if meta.Title != "concurrent title" {
		t.Fatalf("final title = %q, want concurrent title", meta.Title)
	}
	if !reflect.DeepEqual(meta.Usage, usage) {
		t.Fatalf("final usage = %+v, want preserved %+v", meta.Usage, usage)
	}
}
