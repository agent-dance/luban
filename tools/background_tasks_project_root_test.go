package tools

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type projectRootSessionProvider struct{ response string }

func (p projectRootSessionProvider) Name() string    { return "project-root-session" }
func (p projectRootSessionProvider) ModelID() string { return "project-root-session-model" }

func (p projectRootSessionProvider) CreateStream(context.Context, provider.Params) (<-chan types.StreamEvent, error) {
	stream := make(chan types.StreamEvent, 4)
	stream <- types.StreamEvent{
		Type:         types.EventContentBlockStart,
		Index:        0,
		ContentBlock: &types.ContentDelta{Type: types.ContentTypeText},
	}
	stream <- types.StreamEvent{
		Type:  types.EventContentBlockDelta,
		Index: 0,
		Delta: &types.ContentDelta{Type: "text_delta", Text: p.response},
	}
	stream <- types.StreamEvent{Type: types.EventContentBlockStop, Index: 0}
	stream <- types.StreamEvent{Type: types.EventMessageStop}
	close(stream)
	return stream, nil
}

func TestBackgroundTaskKeepsOriginAfterProjectRootSwitch(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	manager := NewBackgroundTaskManager(rootA)

	started := make(chan struct{})
	release := make(chan struct{})
	snapshot, err := manager.StartAgentTask(context.Background(), "origin-a", "origin-a", func(context.Context, io.Writer) (string, error) {
		close(started)
		<-release
		return "completed in a", nil
	})
	if err != nil {
		t.Fatalf("start root A task: %v", err)
	}
	<-started

	manager.SetProjectRoot(rootB)
	close(release)
	completed, waitStatus := manager.Wait(snapshot.ID, 5*time.Second)
	if waitStatus != "success" || completed.Status != "completed" {
		t.Fatalf("wait status=%q task=%+v", waitStatus, completed)
	}
	waitForBackgroundTaskDoneForTest(t, manager, snapshot.ID)

	assertTaskOwnedByRoot(t, snapshot.ID, completed.OutputPath, rootA, rootB)

	eventsA, err := NewRuntimeLifecycle(rootA).Events()
	if err != nil {
		t.Fatalf("read root A lifecycle: %v", err)
	}
	var sawCompleted bool
	for _, event := range eventsA {
		if event.EntityID == snapshot.ID && event.Type == LifecycleTaskCompleted {
			sawCompleted = true
		}
	}
	if !sawCompleted {
		t.Fatalf("root A lifecycle missing completion for task %s: %+v", snapshot.ID, eventsA)
	}
	eventsB, err := NewRuntimeLifecycle(rootB).Events()
	if err != nil {
		t.Fatalf("read root B lifecycle: %v", err)
	}
	for _, event := range eventsB {
		if event.EntityID == snapshot.ID {
			t.Fatalf("root B received lifecycle event for root A task %s: %+v", snapshot.ID, event)
		}
	}
}

func TestBackgroundTaskOwnerRootIgnoresNestedExecutionCWD(t *testing.T) {
	projectRoot := t.TempDir()
	nestedCWD := filepath.Join(projectRoot, "packages", "worker")
	if err := os.MkdirAll(nestedCWD, 0o755); err != nil {
		t.Fatal(err)
	}
	manager := NewBackgroundTaskManager(projectRoot)
	t.Cleanup(manager.Shutdown)

	ctx := loop.WithToolExecutionContext(context.Background(), loop.ToolExecutionContext{
		SessionID: "nested-session",
		CWD:       nestedCWD,
	})
	snapshot, err := manager.StartAgentTask(ctx, "nested task", "nested task", func(context.Context, io.Writer) (string, error) {
		return "done", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	completed, status := manager.Wait(snapshot.ID, 5*time.Second)
	if status != "success" {
		t.Fatalf("wait status = %q", status)
	}
	if got := filepath.Clean(completed.OwnerProjectRoot); got != filepath.Clean(projectRoot) {
		t.Fatalf("owner project root = %q, want manager origin %q (execution cwd was %q)", got, projectRoot, nestedCWD)
	}
	record, ok := NewRuntimeTaskStore(projectRoot).Get(snapshot.ID)
	if !ok || filepath.Clean(record.OwnerProjectRoot) != filepath.Clean(projectRoot) {
		t.Fatalf("durable owner root = %q, found=%v", record.OwnerProjectRoot, ok)
	}
	if _, exists := NewRuntimeTaskStore(nestedCWD).Get(snapshot.ID); exists {
		t.Fatal("nested execution cwd was incorrectly used as a runtime-task project root")
	}
}

func TestBackgroundShellTaskKeepsOriginAfterProjectRootSwitch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX shell")
	}
	rootA := t.TempDir()
	rootB := t.TempDir()
	releasePath := filepath.Join(t.TempDir(), "release")
	manager := NewBackgroundTaskManager(rootA)

	command := `while [ ! -f "$1" ]; do sleep 0.01; done; printf 'shell complete'`
	snapshot, err := manager.StartShellTask(
		context.Background(),
		command,
		"origin-a-shell",
		exec.Command("sh", "-c", command, "sh", releasePath),
	)
	if err != nil {
		t.Fatalf("start root A shell task: %v", err)
	}

	manager.SetProjectRoot(rootB)
	if err := os.WriteFile(releasePath, []byte("release"), 0o600); err != nil {
		t.Fatalf("release shell task: %v", err)
	}
	completed, waitStatus := manager.Wait(snapshot.ID, 5*time.Second)
	if waitStatus != "success" || completed.Status != "completed" {
		t.Fatalf("wait status=%q task=%+v", waitStatus, completed)
	}
	waitForBackgroundTaskDoneForTest(t, manager, snapshot.ID)
	assertTaskOwnedByRoot(t, snapshot.ID, completed.OutputPath, rootA, rootB)
}

func TestRetainedAgentSessionKeepsOriginAfterProjectRootSwitch(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	manager := NewBackgroundTaskManager(rootA)
	defer manager.Shutdown()

	const agentID = "retained-origin-agent"
	const response = "retained session completed in root A"
	queryLoop := loop.New(
		projectRootSessionProvider{response: response},
		registry.New(),
		loop.Config{Model: "project-root-session-model", MaxTokens: 1024, SessionID: agentID},
	)
	session, snapshot, err := manager.RegisterAgentSession(
		agentID,
		"",
		"initial prompt",
		"retained root A session",
		AgentInput{Prompt: "initial prompt", Description: "retained root A session"},
		queryLoop,
		agentSessionMetadata{AgentType: "general-purpose", Model: "project-root-session-model"},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("register root A retained session: %v", err)
	}

	manager.SetProjectRoot(rootB)
	if err := session.enqueue("finish after project switch", nil); err != nil {
		t.Fatalf("enqueue retained session: %v", err)
	}
	completed, waitStatus := manager.Wait(snapshot.ID, 5*time.Second)
	if waitStatus != "success" || completed.Status != "completed" {
		t.Fatalf("wait status=%q task=%+v", waitStatus, completed)
	}
	waitForBackgroundTaskDoneForTest(t, manager, snapshot.ID)
	if completed.Result != response {
		t.Fatalf("retained session result=%q, want %q", completed.Result, response)
	}
	assertTaskOwnedByRoot(t, snapshot.ID, completed.OutputPath, rootA, rootB)
}

func TestBackgroundTaskOriginSurvivesIDReuseAcrossRoots(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	manager := NewBackgroundTaskManager(rootA)
	defer manager.Shutdown()

	release := make(chan struct{})
	snapshotA, err := manager.StartAgentTask(context.Background(), "root-a-prompt", "root-a-task", func(context.Context, io.Writer) (string, error) {
		<-release
		return "root-a-result", nil
	})
	if err != nil {
		t.Fatalf("start root A task: %v", err)
	}
	manager.mu.Lock()
	taskA := manager.tasks[snapshotA.ID]
	manager.mu.Unlock()

	manager.SetProjectRoot(rootB)
	_, snapshotB, err := manager.RegisterAgentSession(
		snapshotA.ID,
		"",
		"root-b-prompt",
		"root-b-task",
		AgentInput{},
		nil,
		agentSessionMetadata{},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("register same-ID root B session: %v", err)
	}
	if snapshotA.OutputPath == snapshotB.OutputPath {
		t.Fatalf("same-ID tasks unexpectedly share output path %q", snapshotA.OutputPath)
	}

	close(release)
	waitForTaskDoneChannelForTest(t, taskA, snapshotA.ID)

	recordA, ok := NewRuntimeTaskStore(rootA).Get(snapshotA.ID)
	if !ok || recordA.Status != "completed" || recordA.Description != "root-a-task" || recordA.Result != "root-a-result" {
		t.Fatalf("root A record lost its task ownership: %+v, found=%v", recordA, ok)
	}
	recordB, ok := NewRuntimeTaskStore(rootB).Get(snapshotB.ID)
	if !ok || recordB.Status != "completed" || recordB.Description != "root-b-task" || recordB.Prompt != "root-b-prompt" {
		t.Fatalf("root B record was overwritten by root A completion: %+v, found=%v", recordB, ok)
	}
}

func TestBackgroundNotificationFollowUpUsesOriginRecordAcrossRootIDReuse(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	manager := NewBackgroundTaskManager(rootA)
	defer manager.Shutdown()

	followUp := make(chan struct {
		sessionID   string
		projectRoot string
		message     string
		ok          bool
	}, 1)
	manager.SetNotificationFollowUp(RuntimeNotificationSinkFunc(func(_ context.Context, notification RuntimeNotification) error {
		target, ok := manager.NotificationFollowUpTarget(notification)
		followUp <- struct {
			sessionID   string
			projectRoot string
			message     string
			ok          bool
		}{sessionID: target.SessionID, projectRoot: target.ProjectRoot, message: target.Message, ok: ok}
		return nil
	}))

	started := make(chan struct{})
	release := make(chan struct{})
	parent := loop.WithToolExecutionContext(context.Background(), loop.ToolExecutionContext{SessionID: "session-a"})
	snapshotA, err := manager.StartAgentTask(parent, "root-a-prompt", "root-a-task", func(context.Context, io.Writer) (string, error) {
		close(started)
		<-release
		return "root-a-result", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	<-started

	manager.SetProjectRoot(rootB)
	manager.mu.Lock()
	manager.tasks[snapshotA.ID] = &BackgroundTask{
		ID: snapshotA.ID, Type: backgroundTaskTypeLocalAgent, Status: "completed",
		Description: "root-b-task", Result: "root-b-result", OwnerSessionID: "session-b",
		origin: manager.currentTaskOriginLocked(),
	}
	manager.mu.Unlock()
	close(release)

	select {
	case got := <-followUp:
		if !got.ok || got.sessionID != "session-a" || filepath.Clean(got.projectRoot) != filepath.Clean(rootA) {
			t.Fatalf("origin follow-up target = session:%q root:%q ok:%v", got.sessionID, got.projectRoot, got.ok)
		}
		if !strings.Contains(got.message, `"result":"root-a-result"`) || strings.Contains(got.message, "root-b-result") {
			t.Fatalf("origin follow-up used reused root-B record: %s", got.message)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("origin follow-up was not delivered")
	}
}

func TestLegacyBackgroundFollowUpWithoutRootUsesSingleKnownOrigin(t *testing.T) {
	root := t.TempDir()
	manager := NewBackgroundTaskManager(root)
	defer manager.Shutdown()
	const taskID = "legacy-single-origin"
	if err := NewRuntimeTaskStore(root).Save(RuntimeTaskRecord{
		ID: taskID, Type: backgroundTaskTypeLocalAgent, Status: "completed",
		Result: "legacy-result", OwnerSessionID: "legacy-session", StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	target, ok := manager.NotificationFollowUpTarget(RuntimeNotification{TaskID: taskID, SessionID: "legacy-session"})
	if !ok || target.SessionID != "legacy-session" || filepath.Clean(target.ProjectRoot) != filepath.Clean(root) || !strings.Contains(target.Message, "legacy-result") {
		t.Fatalf("legacy single-origin target = %+v ok=%v", target, ok)
	}
}

func TestLegacyBackgroundFollowUpWithoutRootFailsClosedWhenTaskIDIsAmbiguous(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	manager := NewBackgroundTaskManager(rootA)
	defer manager.Shutdown()

	const taskID = "legacy-duplicate-task"
	for _, target := range []struct {
		root   string
		result string
	}{{rootA, "result-a"}, {rootB, "result-b"}} {
		manager.SetProjectRoot(target.root)
		if err := NewRuntimeTaskStore(target.root).Save(RuntimeTaskRecord{
			ID: taskID, Type: backgroundTaskTypeLocalAgent, Status: "completed",
			Result: target.result, OwnerSessionID: "legacy-session", StartedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatal(err)
		}
	}

	if sessionID, message, ok := manager.NotificationFollowUp(RuntimeNotification{TaskID: taskID, SessionID: "legacy-session"}); ok {
		t.Fatalf("ambiguous legacy follow-up resolved to session=%q message=%q", sessionID, message)
	}
}

func TestBackgroundTasksKeepOriginDuringConcurrentProjectRootSwitches(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	manager := NewBackgroundTaskManager(rootA)
	defer manager.Shutdown()

	const taskCount = 24
	release := make(chan struct{})
	snapshots := make(chan BackgroundTaskSnapshot, taskCount)
	errs := make(chan error, taskCount)

	var switchers sync.WaitGroup
	switchers.Add(1)
	go func() {
		defer switchers.Done()
		for i := 0; i < taskCount*8; i++ {
			if i%2 == 0 {
				manager.SetProjectRoot(rootA)
			} else {
				manager.SetProjectRoot(rootB)
			}
		}
	}()

	var starters sync.WaitGroup
	for i := 0; i < taskCount; i++ {
		starters.Add(1)
		go func(index int) {
			defer starters.Done()
			if index%3 == 0 {
				_, snapshot, err := manager.RegisterAgentSession(
					fmt.Sprintf("session-%d", index),
					"",
					fmt.Sprintf("session-task-%d", index),
					"retained root ownership",
					AgentInput{},
					nil,
					agentSessionMetadata{},
					nil,
					nil,
				)
				if err != nil {
					errs <- err
					return
				}
				snapshots <- *snapshot
				return
			}
			snapshot, err := manager.StartAgentTask(context.Background(), fmt.Sprintf("task-%d", index), "root ownership", func(context.Context, io.Writer) (string, error) {
				<-release
				return fmt.Sprintf("result-%d", index), nil
			})
			if err != nil {
				errs <- err
				return
			}
			snapshots <- *snapshot
		}(i)
	}
	starters.Wait()
	switchers.Wait()
	close(release)
	close(snapshots)
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("start concurrent task: %v", err)
		}
	}
	for snapshot := range snapshots {
		completed, waitStatus := manager.Wait(snapshot.ID, 5*time.Second)
		if waitStatus != "success" || completed.Status != "completed" {
			t.Fatalf("task %s wait status=%q snapshot=%+v", snapshot.ID, waitStatus, completed)
		}
		waitForBackgroundTaskDoneForTest(t, manager, snapshot.ID)
		ownerRoot := rootA
		otherRoot := rootB
		if filepath.Clean(filepath.Dir(snapshot.OutputPath)) == filepath.Join(rootB, ".claude", "task-output") {
			ownerRoot, otherRoot = rootB, rootA
		} else if filepath.Clean(filepath.Dir(snapshot.OutputPath)) != filepath.Join(rootA, ".claude", "task-output") {
			t.Fatalf("task %s has output outside either project root: %q", snapshot.ID, snapshot.OutputPath)
		}
		assertTaskOwnedByRoot(t, snapshot.ID, completed.OutputPath, ownerRoot, otherRoot)
	}
}

func waitForBackgroundTaskDoneForTest(t *testing.T, manager *BackgroundTaskManager, taskID string) {
	t.Helper()
	manager.mu.Lock()
	task := manager.tasks[taskID]
	manager.mu.Unlock()
	if task == nil {
		t.Fatalf("task %s missing from manager", taskID)
	}
	waitForTaskDoneChannelForTest(t, task, taskID)
}

func waitForTaskDoneChannelForTest(t *testing.T, task *BackgroundTask, taskID string) {
	t.Helper()
	task.mu.RLock()
	done := task.done
	task.mu.RUnlock()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("task %s did not finish", taskID)
	}
}

func assertTaskOwnedByRoot(t *testing.T, taskID, outputPath, ownerRoot, otherRoot string) {
	t.Helper()
	wantOutputDir := filepath.Join(ownerRoot, ".claude", "task-output")
	if filepath.Clean(filepath.Dir(outputPath)) != filepath.Clean(wantOutputDir) {
		t.Fatalf("task %s output path %q is not owned by %q", taskID, outputPath, ownerRoot)
	}

	ownerRecord, ok := NewRuntimeTaskStore(ownerRoot).Get(taskID)
	if !ok {
		t.Fatalf("owner root %q missing task %s", ownerRoot, taskID)
	}
	if ownerRecord.Status != "completed" {
		t.Fatalf("owner root %q task %s status=%q, want completed", ownerRoot, taskID, ownerRecord.Status)
	}
	if _, ok := NewRuntimeTaskStore(otherRoot).Get(taskID); ok {
		t.Fatalf("non-owner root %q unexpectedly contains task %s", otherRoot, taskID)
	}
}
