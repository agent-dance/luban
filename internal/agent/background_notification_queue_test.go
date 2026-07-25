package agent

import (
	"context"
	"errors"
	agent "github.com/agent-dance/luban/internal/contracts/agent"
	runtimestore "github.com/agent-dance/luban/internal/store/runtime"
	"io"
	"sync"
	"testing"
	"time"
)

func persistPendingFollowUpForTest(t *testing.T, manager *BackgroundTaskManager, taskID, sessionID string, createdAt, startedAt time.Time) agent.RuntimeNotification {
	t.Helper()
	notification := agent.RuntimeNotification{
		ID:               "notification-" + taskID,
		Kind:             "task-notification",
		TaskID:           taskID,
		SessionID:        sessionID,
		ProjectRoot:      manager.CurrentProjectRoot(),
		Title:            taskID,
		Message:          taskID,
		CreatedAt:        createdAt,
		FollowUpRequired: true,
	}
	record := runtimestore.RuntimeTaskRecord{
		ID:               taskID,
		Type:             agent.TaskTypeLocalAgent,
		Status:           "completed",
		OwnerSessionID:   sessionID,
		OwnerProjectRoot: manager.CurrentProjectRoot(),
		StartedAt:        startedAt,
		UpdatedAt:        createdAt,
		Notification:     &notification,
	}
	if err := manager.store.Save(record); err != nil {
		t.Fatal(err)
	}
	return notification
}

func awaitNotificationState(t *testing.T, manager *BackgroundTaskManager, taskID string, ready func(*agent.RuntimeNotification) bool) runtimestore.RuntimeTaskRecord {
	t.Helper()
	updates, unsubscribe := manager.SubscribeSnapshots()
	defer unsubscribe()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		record, ok := manager.store.Get(taskID)
		if ok && record.Notification != nil && ready(record.Notification) {
			return record
		}
		select {
		case <-updates:
		case <-deadline.C:
			t.Fatalf("notification %q did not reach expected state: %#v", taskID, record.Notification)
		}
	}
}

func receiveFollowUpTask(t *testing.T, calls <-chan string) string {
	t.Helper()
	select {
	case taskID := <-calls:
		return taskID
	case <-time.After(5 * time.Second):
		t.Fatal("follow-up did not start")
		return ""
	}
}

func assertNoFollowUpReady(t *testing.T, calls <-chan string) {
	t.Helper()
	select {
	case taskID := <-calls:
		t.Fatalf("unexpected follow-up %q", taskID)
	default:
	}
}

func TestRuntimeNotificationFollowUpsAreFIFOWithinOwningSession(t *testing.T) {
	root := t.TempDir()
	manager := NewBackgroundTaskManager(root)
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	base := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	// Deliberately invert task start order. Durable FIFO is completion
	// notification order, not task launch order.
	persistPendingFollowUpForTest(t, manager, "first", "session-a", base, base.Add(3*time.Minute))
	persistPendingFollowUpForTest(t, manager, "second", "session-a", base.Add(time.Second), base.Add(2*time.Minute))
	persistPendingFollowUpForTest(t, manager, "third", "session-a", base.Add(2*time.Second), base.Add(time.Minute))

	calls := make(chan string, 3)
	releases := map[string]chan struct{}{
		"first": make(chan struct{}), "second": make(chan struct{}), "third": make(chan struct{}),
	}
	manager.SetNotificationConsumers(nil, RuntimeNotificationSinkFunc(func(_ context.Context, notification agent.RuntimeNotification) error {
		calls <- notification.TaskID
		<-releases[notification.TaskID]
		return nil
	}))

	if got := receiveFollowUpTask(t, calls); got != "first" {
		t.Fatalf("first call = %q", got)
	}
	assertNoFollowUpReady(t, calls)
	close(releases["first"])
	if got := receiveFollowUpTask(t, calls); got != "second" {
		t.Fatalf("second call = %q", got)
	}
	assertNoFollowUpReady(t, calls)
	close(releases["second"])
	if got := receiveFollowUpTask(t, calls); got != "third" {
		t.Fatalf("third call = %q", got)
	}
	close(releases["third"])
	for _, taskID := range []string{"first", "second", "third"} {
		awaitNotificationState(t, manager, taskID, func(notification *agent.RuntimeNotification) bool {
			return notification.FollowUpDeliveredAt != nil && notification.DeliveredAt != nil
		})
	}
}

func TestRuntimeNotificationFollowUpsRunConcurrentlyAcrossOwningSessions(t *testing.T) {
	manager := NewBackgroundTaskManager(t.TempDir())
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	base := time.Date(2026, 7, 18, 11, 0, 0, 0, time.UTC)
	persistPendingFollowUpForTest(t, manager, "session-a-task", "session-a", base, base)
	persistPendingFollowUpForTest(t, manager, "session-b-task", "session-b", base.Add(time.Second), base.Add(time.Second))

	calls := make(chan string, 2)
	release := make(chan struct{})
	manager.SetNotificationConsumers(nil, RuntimeNotificationSinkFunc(func(_ context.Context, notification agent.RuntimeNotification) error {
		calls <- notification.TaskID
		<-release
		return nil
	}))

	seen := map[string]bool{
		receiveFollowUpTask(t, calls): true,
		receiveFollowUpTask(t, calls): true,
	}
	if !seen["session-a-task"] || !seen["session-b-task"] {
		t.Fatalf("parallel starts = %#v", seen)
	}
	close(release)
}

func TestRuntimeNotificationFollowUpsDoNotMergeSameSessionIDAcrossProjects(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	manager := NewBackgroundTaskManager(rootA)
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	base := time.Date(2026, 7, 18, 11, 30, 0, 0, time.UTC)
	persistPendingFollowUpForTest(t, manager, "root-a-task", "shared-session", base, base)

	originB := &backgroundTaskOrigin{
		projectRoot: rootB,
		store:       runtimestore.NewRuntimeTaskStore(rootB),
	}
	notificationB := agent.RuntimeNotification{
		ID: "notification-root-b-task", Kind: "task-notification", TaskID: "root-b-task",
		SessionID: "shared-session", ProjectRoot: rootB, Title: "root-b-task", Message: "root-b-task",
		CreatedAt: base.Add(time.Second), FollowUpRequired: true,
	}
	if err := originB.store.Save(runtimestore.RuntimeTaskRecord{
		ID: "root-b-task", Type: agent.TaskTypeLocalAgent, Status: "completed",
		OwnerSessionID: "shared-session", OwnerProjectRoot: rootB,
		StartedAt: base.Add(time.Second), UpdatedAt: base.Add(time.Second), Notification: &notificationB,
	}); err != nil {
		t.Fatal(err)
	}

	calls := make(chan string, 2)
	release := make(chan struct{})
	manager.SetNotificationConsumers(nil, RuntimeNotificationSinkFunc(func(_ context.Context, notification agent.RuntimeNotification) error {
		calls <- notification.TaskID
		<-release
		return nil
	}))
	if got := receiveFollowUpTask(t, calls); got != "root-a-task" {
		t.Fatalf("root A call = %q", got)
	}
	if err := manager.deliverRuntimeNotificationAtOrigin(context.Background(), nil, notificationB, originB); err != nil {
		t.Fatal(err)
	}
	if got := receiveFollowUpTask(t, calls); got != "root-b-task" {
		t.Fatalf("root B call = %q", got)
	}
	close(release)
}

func TestRuntimeNotificationFailedHeadBlocksUntilExplicitReplay(t *testing.T) {
	manager := NewBackgroundTaskManager(t.TempDir())
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	base := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	persistPendingFollowUpForTest(t, manager, "head", "session-a", base, base)
	persistPendingFollowUpForTest(t, manager, "tail", "session-a", base.Add(time.Second), base.Add(time.Second))

	calls := make(chan string, 4)
	releaseHead := make(chan struct{})
	releaseTail := make(chan struct{})
	var mu sync.Mutex
	failHead := true
	manager.SetNotificationConsumers(nil, RuntimeNotificationSinkFunc(func(_ context.Context, notification agent.RuntimeNotification) error {
		calls <- notification.TaskID
		mu.Lock()
		fail := notification.TaskID == "head" && failHead
		mu.Unlock()
		if fail {
			return errors.New("follow-up temporarily unavailable")
		}
		if notification.TaskID == "head" {
			<-releaseHead
		} else {
			<-releaseTail
		}
		return nil
	}))

	if got := receiveFollowUpTask(t, calls); got != "head" {
		t.Fatalf("initial call = %q", got)
	}
	awaitNotificationState(t, manager, "head", func(notification *agent.RuntimeNotification) bool {
		return notification.FollowUpDeliveredAt == nil && notification.LastError != ""
	})
	assertNoFollowUpReady(t, calls)

	mu.Lock()
	failHead = false
	mu.Unlock()
	if err := manager.ReplayPendingNotifications(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := receiveFollowUpTask(t, calls); got != "head" {
		t.Fatalf("retry call = %q", got)
	}
	assertNoFollowUpReady(t, calls)
	close(releaseHead)
	if got := receiveFollowUpTask(t, calls); got != "tail" {
		t.Fatalf("tail call = %q", got)
	}
	close(releaseTail)
	awaitNotificationState(t, manager, "tail", func(notification *agent.RuntimeNotification) bool {
		return notification.FollowUpDeliveredAt != nil
	})
}

func TestRuntimeNotificationDuplicateUsesCanonicalStableIDExactlyOnce(t *testing.T) {
	manager := NewBackgroundTaskManager(t.TempDir())
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	base := time.Date(2026, 7, 18, 13, 0, 0, 0, time.UTC)
	canonical := persistPendingFollowUpForTest(t, manager, "duplicate", "session-a", base, base)
	canonical.RunID = "run-1"
	canonical.Attempt = 1
	record, ok := manager.store.Get("duplicate")
	if !ok {
		t.Fatal("canonical notification record missing")
	}
	record.CurrentRunID = canonical.RunID
	record.Attempt = canonical.Attempt
	record.Notification = &canonical
	if err := manager.store.Save(record); err != nil {
		t.Fatal(err)
	}

	calls := make(chan agent.RuntimeNotification, 2)
	release := make(chan struct{})
	manager.SetNotificationConsumers(nil, RuntimeNotificationSinkFunc(func(_ context.Context, notification agent.RuntimeNotification) error {
		calls <- notification
		<-release
		return nil
	}))
	first := func() agent.RuntimeNotification {
		select {
		case notification := <-calls:
			return notification
		case <-time.After(5 * time.Second):
			t.Fatal("canonical follow-up did not start")
			return agent.RuntimeNotification{}
		}
	}()
	if first.ID != canonical.ID {
		t.Fatalf("delivered ID = %q, want canonical %q", first.ID, canonical.ID)
	}

	duplicate := canonical
	duplicate.ID = "incoming-noncanonical-id"
	if err := manager.deliverRuntimeNotificationAtOrigin(context.Background(), nil, duplicate, manager.currentTaskOrigin()); err != nil {
		t.Fatal(err)
	}
	assertNoNotificationCall := func() {
		select {
		case notification := <-calls:
			t.Fatalf("duplicate follow-up delivered: %#v", notification)
		default:
		}
	}
	assertNoNotificationCall()
	close(release)
	awaitNotificationState(t, manager, "duplicate", func(notification *agent.RuntimeNotification) bool {
		return notification.ID == canonical.ID && notification.FollowUpDeliveredAt != nil
	})
	if err := manager.ReplayPendingNotifications(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertNoNotificationCall()
}

func TestRuntimeNotificationNewAgentRunGetsNewStableIDAndAckLifecycle(t *testing.T) {
	manager := NewBackgroundTaskManager(t.TempDir())
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	now := time.Date(2026, 7, 18, 14, 0, 0, 0, time.UTC)
	oldDeliveredAt := now.Add(-time.Minute)
	old := agent.RuntimeNotification{
		ID: "notification-run-1", Kind: "task-notification", TaskID: "retained-agent",
		RunID: "run-1", Attempt: 1, SessionID: "session-a", ProjectRoot: manager.CurrentProjectRoot(),
		CreatedAt: now.Add(-time.Minute), FollowUpRequired: true,
		FollowUpDeliveredAt: &oldDeliveredAt, DeliveredAt: &oldDeliveredAt,
	}
	if err := manager.store.Save(runtimestore.RuntimeTaskRecord{
		ID: "retained-agent", Type: agent.TaskTypeLocalAgent, Status: "completed",
		OwnerSessionID: "session-a", OwnerProjectRoot: manager.CurrentProjectRoot(),
		CurrentRunID: "run-2", Attempt: 2, StartedAt: now, UpdatedAt: now, Notification: &old,
	}); err != nil {
		t.Fatal(err)
	}

	calls := make(chan agent.RuntimeNotification, 1)
	manager.SetNotificationConsumers(nil, RuntimeNotificationSinkFunc(func(_ context.Context, notification agent.RuntimeNotification) error {
		calls <- notification
		return nil
	}))
	assertNoRun2Call := func() {
		select {
		case notification := <-calls:
			t.Fatalf("old delivered run was replayed: %#v", notification)
		default:
		}
	}
	assertNoRun2Call()

	second := agent.RuntimeNotification{
		ID: "notification-run-2", Kind: "task-notification", TaskID: "retained-agent",
		RunID: "run-2", Attempt: 2, SessionID: "session-a", ProjectRoot: manager.CurrentProjectRoot(),
		CreatedAt: now,
	}
	if err := manager.deliverRuntimeNotificationAtOrigin(context.Background(), nil, second, manager.currentTaskOrigin()); err != nil {
		t.Fatal(err)
	}
	var delivered agent.RuntimeNotification
	select {
	case delivered = <-calls:
	case <-time.After(5 * time.Second):
		t.Fatal("new retained-agent run did not receive a follow-up")
	}
	if delivered.ID != second.ID || delivered.RunID != "run-2" || delivered.Attempt != 2 {
		t.Fatalf("new-run identity = %#v", delivered)
	}
	record := awaitNotificationState(t, manager, "retained-agent", func(notification *agent.RuntimeNotification) bool {
		return notification.ID == second.ID && notification.RunID == "run-2" && notification.FollowUpDeliveredAt != nil
	})
	if record.Notification.DeliveredAt == nil {
		t.Fatalf("new run was not fully acknowledged: %#v", record.Notification)
	}
}

func TestRuntimeNotificationForTaskCarriesAgentRunIdentity(t *testing.T) {
	task := &BackgroundTask{
		ID: "agent", Type: agent.TaskTypeLocalAgent, Status: "completed",
		CurrentRunID: "agent:run-7", Attempt: 7,
	}
	notification := runtimeNotificationForTask(task, "completed", 0)
	if notification.RunID != task.CurrentRunID || notification.Attempt != task.Attempt {
		t.Fatalf("notification run identity = %#v", notification)
	}
}

func TestBackgroundTaskManagerShutdownDrainsOrphanedTaskBeforeReturn(t *testing.T) {
	root := t.TempDir()
	manager := NewBackgroundTaskManager(root)
	runnerStarted := make(chan struct{})
	releaseRunner := make(chan struct{})
	sinkStarted := make(chan struct{})
	releaseSink := make(chan struct{})
	var releaseRunnerOnce sync.Once
	var releaseSinkOnce sync.Once
	t.Cleanup(func() {
		releaseRunnerOnce.Do(func() { close(releaseRunner) })
		releaseSinkOnce.Do(func() { close(releaseSink) })
		_ = manager.Shutdown(context.Background())
	})

	manager.SetNotificationSink(RuntimeNotificationSinkFunc(func(_ context.Context, _ agent.RuntimeNotification) error {
		close(sinkStarted)
		<-releaseSink
		return nil
	}))
	snapshot, err := manager.StartAgentTask(context.Background(), "drain", "drain", func(context.Context, io.Writer) (string, error) {
		close(runnerStarted)
		<-releaseRunner
		return "durable result", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	<-runnerStarted

	// Simulate a task-map replacement such as project/root ID reuse. Shutdown
	// must rely on the managed-work lease, not only its mutable task snapshot.
	manager.mu.Lock()
	manager.tasks[snapshot.ID] = &BackgroundTask{ID: snapshot.ID, Status: "completed"}
	manager.mu.Unlock()

	shutdownDone := make(chan struct{})
	go func() {
		_ = manager.Shutdown(context.Background())
		close(shutdownDone)
	}()
	releaseRunnerOnce.Do(func() { close(releaseRunner) })
	select {
	case <-sinkStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("terminal notification sink did not start")
	}
	select {
	case <-shutdownDone:
		t.Fatal("Shutdown returned while an orphaned task was still persisting its notification")
	default:
	}
	releaseSinkOnce.Do(func() { close(releaseSink) })
	select {
	case <-shutdownDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown did not drain the managed task")
	}

	store := runtimestore.NewRuntimeTaskStore(root)
	record, ok := store.Get(snapshot.ID)
	if !ok || record.Notification == nil || record.Notification.SinkDeliveredAt == nil {
		t.Fatalf("shutdown returned before durable notification acknowledgement: %#v", record.Notification)
	}
	attempts := record.Notification.Attempts
	manager.SetNotificationSink(RuntimeNotificationSinkFunc(func(context.Context, agent.RuntimeNotification) error {
		t.Fatal("consumer was admitted after Shutdown")
		return nil
	}))
	if err := manager.ReplayPendingNotifications(context.Background()); err != nil {
		t.Fatal(err)
	}
	after, ok := store.Get(snapshot.ID)
	if !ok || after.Notification == nil || after.Notification.Attempts != attempts {
		t.Fatalf("notification store changed after Shutdown: before=%#v after=%#v", record.Notification, after.Notification)
	}
}
