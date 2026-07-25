package agent

import (
	"context"
	"errors"
	agent "github.com/agent-dance/luban/internal/contracts/agent"
	runtimestore "github.com/agent-dance/luban/internal/store/runtime"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"

	"github.com/agent-dance/luban/hooks"
)

type recordingRuntimeNotificationSink struct {
	err           error
	notifications []agent.RuntimeNotification
	store         *runtimestore.RuntimeTaskStore
}

type retryingRuntimeNotificationSink struct {
	mu       sync.Mutex
	calls    int
	failures int
	called   chan struct{}
}

func (s *retryingRuntimeNotificationSink) DeliverRuntimeNotification(context.Context, agent.RuntimeNotification) error {
	s.mu.Lock()
	s.calls++
	call := s.calls
	failures := s.failures
	s.mu.Unlock()
	select {
	case s.called <- struct{}{}:
	default:
	}
	if call <= failures {
		return errors.New("follow-up unavailable")
	}
	return nil
}

func (s *retryingRuntimeNotificationSink) setFailures(failures int) {
	s.mu.Lock()
	s.failures = failures
	s.mu.Unlock()
}

func (s *retryingRuntimeNotificationSink) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func (s *recordingRuntimeNotificationSink) DeliverRuntimeNotification(_ context.Context, notification agent.RuntimeNotification) error {
	if s.store != nil {
		record, ok := s.store.Get(notification.TaskID)
		if !ok || record.Notification == nil || record.Notification.ID != notification.ID {
			return errors.New("notification was dispatched before durable task state")
		}
	}
	s.notifications = append(s.notifications, notification)
	return s.err
}

func TestBackgroundNotificationPersistsBeforeDeliveryAndReplaysAfterResume(t *testing.T) {
	root := t.TempDir()
	manager := NewBackgroundTaskManager(root)
	manager.lifecycle = nil // Isolate the notification assertion from lifecycle journaling.

	failing := &recordingRuntimeNotificationSink{err: errors.New("offline"), store: manager.store}
	manager.SetNotificationSink(failing)
	task := &BackgroundTask{
		ID:        "task-1",
		Type:      agent.TaskTypeLocalBash,
		Status:    "completed",
		StartedAt: time.Now().Add(-time.Second),
	}
	code := 0
	task.ExitCode = &code
	finished := time.Now().UTC()
	task.FinishedAt = &finished
	manager.registerTask(task)
	manager.emitTaskCompletionNotification(context.Background(), task, "completed", 0)

	record, ok := manager.store.Get(task.ID)
	if !ok || record.Notification == nil {
		t.Fatalf("pending notification not persisted: ok=%v record=%#v", ok, record)
	}
	if record.Notification.DeliveredAt != nil || record.Notification.Attempts != 1 {
		t.Fatalf("failed delivery should remain pending with one attempt: %#v", record.Notification)
	}

	resumed := NewBackgroundTaskManager(root)
	resumed.lifecycle = nil
	success := &recordingRuntimeNotificationSink{store: resumed.store}
	resumed.SetNotificationSink(success)
	if err := resumed.ReplayPendingNotifications(context.Background()); err != nil {
		t.Fatalf("ReplayPendingNotifications: %v", err)
	}
	if len(success.notifications) != 1 || success.notifications[0].TaskID != task.ID {
		t.Fatalf("resume did not replay pending notification: %#v", success.notifications)
	}

	record, ok = resumed.store.Get(task.ID)
	if !ok || record.Notification == nil || record.Notification.DeliveredAt == nil {
		t.Fatalf("successful replay was not acknowledged: %#v", record)
	}
	if err := resumed.ReplayPendingNotifications(context.Background()); err != nil {
		t.Fatalf("second replay: %v", err)
	}
	if len(success.notifications) != 1 {
		t.Fatalf("delivered notification replayed twice: %#v", success.notifications)
	}
}

func TestBackgroundNotificationHookCarriesStableSessionAndTaskEvidence(t *testing.T) {
	root := t.TempDir()
	manager := NewBackgroundTaskManager(root)
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	var observed []hooks.HookExecution
	var observedMu sync.Mutex
	ctx := hooks.WithExecutionObserver(context.Background(), func(hookType hooks.HookType, execution hooks.HookExecution) {
		if hookType == hooks.HookNotification {
			observedMu.Lock()
			observed = append(observed, execution)
			observedMu.Unlock()
		}
	})
	ctx = executioncontract.WithToolExecutionContext(ctx, executioncontract.ToolExecutionContext{SessionID: "session-identity", ProjectRoot: root, CWD: filepath.Join(root, "nested"), WorkUnitID: "parent-work"})
	runner := hooks.NewRunner([]hooks.Hook{{Type: hooks.HookNotification, Command: "true", Timeout: 1}})
	manager.SetHookRunner(runner)
	task, err := manager.StartAgentTask(ctx, "identity task", "identity task", func(context.Context, io.Writer) (string, error) {
		return "done", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, status := manager.Wait(task.ID, time.Second); status != "success" {
		t.Fatalf("wait status = %q", status)
	}
	waitForBackgroundTaskDoneForTest(t, manager, task.ID)
	observedMu.Lock()
	defer observedMu.Unlock()
	if len(observed) != 1 {
		t.Fatalf("observed notification executions = %d, want one", len(observed))
	}
	input := observed[0].Input
	if input.SessionID != "session-identity" || filepath.Clean(input.ProjectRoot) != filepath.Clean(root) || input.TaskID != task.ID || input.WorkUnitID != "parent-work" {
		t.Fatalf("notification hook identity = session:%q project:%q task:%q work:%q", input.SessionID, input.ProjectRoot, input.TaskID, input.WorkUnitID)
	}
	if input.HookExecutionID == "" || input.HookConfigID == "" {
		t.Fatalf("notification hook lacks stable execution/config identity: %#v", input)
	}
}

func TestBackgroundHookNotificationUsesUnifiedDeliveryOnce(t *testing.T) {
	root := t.TempDir()
	manager := NewBackgroundTaskManager(root)
	countPath := filepath.Join(root, "notification-count")
	runner := hooks.NewRunner([]hooks.Hook{{
		Type:    hooks.HookNotification,
		Command: "printf x >> " + strconv.Quote(countPath),
		Timeout: 1,
	}})
	manager.SetHookRunner(runner)

	task := &BackgroundTask{
		ID:        "task-hook",
		Type:      agent.TaskTypeLocalBash,
		Status:    "completed",
		StartedAt: time.Now().Add(-time.Second),
	}
	code := 0
	task.ExitCode = &code
	finished := time.Now().UTC()
	task.FinishedAt = &finished
	manager.registerTask(task)
	manager.emitTaskCompletionNotification(context.Background(), task, "completed", 0)

	data, err := os.ReadFile(countPath)
	if err != nil {
		t.Fatalf("read notification count: %v", err)
	}
	if string(data) != "x" {
		t.Fatalf("completion notification used more than one path: %q", data)
	}
}

func TestBackgroundNotificationUsesOriginHookRunner(t *testing.T) {
	originRoot := t.TempDir()
	foregroundRoot := t.TempDir()
	manager := NewBackgroundTaskManager(originRoot)
	defer func() { _ = manager.Shutdown(context.Background()) }()

	originCountPath := filepath.Join(originRoot, "notification-count")
	foregroundCountPath := filepath.Join(foregroundRoot, "notification-count")
	originRunner := hooks.NewRunner([]hooks.Hook{{
		Type:    hooks.HookNotification,
		Command: "printf x >> " + strconv.Quote(originCountPath),
		Timeout: 1,
	}})
	foregroundRunner := hooks.NewRunner([]hooks.Hook{{
		Type:    hooks.HookNotification,
		Command: "printf x >> " + strconv.Quote(foregroundCountPath),
		Timeout: 1,
	}})
	manager.SetHookRunner(originRunner)

	started := make(chan struct{})
	release := make(chan struct{})
	task, err := manager.StartAgentTask(context.Background(), "origin task", "origin task", func(context.Context, io.Writer) (string, error) {
		close(started)
		<-release
		return "done", nil
	})
	if err != nil {
		t.Fatalf("start origin task: %v", err)
	}
	<-started

	manager.SetProjectRoot(foregroundRoot)
	manager.SetHookRunner(foregroundRunner)
	close(release)
	if snapshot, status := manager.Wait(task.ID, 5*time.Second); status != "success" || snapshot.Status != "completed" {
		t.Fatalf("wait status=%q snapshot=%#v", status, snapshot)
	}

	originCount, err := os.ReadFile(originCountPath)
	if err != nil {
		t.Fatalf("read origin hook count: %v", err)
	}
	if string(originCount) != "x" {
		t.Fatalf("origin hook count = %q, want one delivery", originCount)
	}
	if foregroundCount, err := os.ReadFile(foregroundCountPath); err == nil {
		t.Fatalf("foreground hook received origin completion: %q", foregroundCount)
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read foreground hook count: %v", err)
	}
}

func TestBackgroundNotificationObserverDoesNotReplaceHookSink(t *testing.T) {
	root := t.TempDir()
	manager := NewBackgroundTaskManager(root)
	countPath := filepath.Join(root, "notification-observer-count")
	runner := hooks.NewRunner([]hooks.Hook{{
		Type:    hooks.HookNotification,
		Command: "printf x >> " + strconv.Quote(countPath),
		Timeout: 1,
	}})
	observer := &recordingRuntimeNotificationSink{}
	manager.SetHookRunner(runner)
	manager.SetNotificationConsumers(observer, nil)

	task := &BackgroundTask{
		ID:        "task-observer",
		Type:      agent.TaskTypeLocalAgent,
		Status:    "completed",
		StartedAt: time.Now().Add(-time.Second),
	}
	code := 0
	task.ExitCode = &code
	finished := time.Now().UTC()
	task.FinishedAt = &finished
	manager.registerTask(task)
	manager.emitTaskCompletionNotification(context.Background(), task, "completed", 0)

	data, err := os.ReadFile(countPath)
	if err != nil {
		t.Fatalf("read hook notification count: %v", err)
	}
	if string(data) != "x" {
		t.Fatalf("hook delivery count = %q, want one", data)
	}
	if len(observer.notifications) != 1 || observer.notifications[0].TaskID != task.ID {
		t.Fatalf("observer notifications = %#v", observer.notifications)
	}
}

func TestBackgroundNotificationRetriesOnlyConsumerThatFailed(t *testing.T) {
	root := t.TempDir()
	manager := NewBackgroundTaskManager(root)
	sink := &recordingRuntimeNotificationSink{err: errors.New("hook offline")}
	observer := &recordingRuntimeNotificationSink{}
	manager.SetNotificationSink(sink)
	manager.SetNotificationConsumers(observer, nil)
	task := &BackgroundTask{ID: "task-partial-delivery", Type: agent.TaskTypeLocalAgent, Status: "completed", StartedAt: time.Now().Add(-time.Second)}
	manager.registerTask(task)
	manager.emitTaskCompletionNotification(context.Background(), task, "completed", 0)

	record, ok := manager.store.Get(task.ID)
	if !ok || record.Notification == nil {
		t.Fatalf("notification was not persisted: %#v", record)
	}
	if record.Notification.ObserverDeliveredAt == nil || record.Notification.SinkDeliveredAt != nil || record.Notification.DeliveredAt != nil {
		t.Fatalf("partial acknowledgements = %#v", record.Notification)
	}
	if len(observer.notifications) != 1 {
		t.Fatalf("observer calls = %d, want one", len(observer.notifications))
	}

	sink.err = nil
	if err := manager.ReplayPendingNotifications(context.Background()); err != nil {
		t.Fatalf("ReplayPendingNotifications: %v", err)
	}
	if len(observer.notifications) != 1 {
		t.Fatalf("successful observer was replayed: %d calls", len(observer.notifications))
	}
	if len(sink.notifications) != 2 {
		t.Fatalf("failed sink calls = %d, want initial plus retry", len(sink.notifications))
	}
	record, _ = manager.store.Get(task.ID)
	if record.Notification == nil || record.Notification.DeliveredAt == nil || record.Notification.SinkDeliveredAt == nil {
		t.Fatalf("retry was not fully acknowledged: %#v", record.Notification)
	}
}

func TestBackgroundNotificationModelFollowUpAcknowledgesOnlyAfterSuccess(t *testing.T) {
	root := t.TempDir()
	manager := NewBackgroundTaskManager(root)
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	observer := &recordingRuntimeNotificationSink{}
	followUp := &retryingRuntimeNotificationSink{failures: 1, called: make(chan struct{}, 2)}
	manager.SetNotificationConsumers(observer, followUp)
	task := &BackgroundTask{
		ID:             "task-follow-up-retry",
		Type:           agent.TaskTypeLocalAgent,
		Status:         "completed",
		OwnerSessionID: "parent-session",
		StartedAt:      time.Now().Add(-time.Second),
	}
	manager.registerTask(task)
	manager.emitTaskCompletionNotification(context.Background(), task, "completed", 0)

	select {
	case <-followUp.called:
	case <-time.After(time.Second):
		t.Fatal("model follow-up was not attempted")
	}
	waitForNotificationRecord(t, manager.store, task.ID, func(notification *agent.RuntimeNotification) bool {
		return notification.FollowUpRequired && notification.FollowUpDeliveredAt == nil && notification.DeliveredAt == nil && notification.LastError != ""
	})
	if len(observer.notifications) != 1 {
		t.Fatalf("observer calls = %d, want one", len(observer.notifications))
	}

	followUp.setFailures(0)
	if err := manager.ReplayPendingNotifications(context.Background()); err != nil {
		t.Fatalf("ReplayPendingNotifications: %v", err)
	}
	select {
	case <-followUp.called:
	case <-time.After(time.Second):
		t.Fatal("model follow-up was not retried")
	}
	waitForNotificationRecord(t, manager.store, task.ID, func(notification *agent.RuntimeNotification) bool {
		return notification.FollowUpDeliveredAt != nil && notification.DeliveredAt != nil
	})
	if followUp.callCount() != 2 {
		t.Fatalf("follow-up calls = %d, want initial plus retry", followUp.callCount())
	}
	if len(observer.notifications) != 1 {
		t.Fatalf("successful observer was replayed during model retry: %d", len(observer.notifications))
	}
}

func waitForNotificationRecord(t *testing.T, store *runtimestore.RuntimeTaskStore, taskID string, ready func(*agent.RuntimeNotification) bool) runtimestore.RuntimeTaskRecord {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		record, ok := store.Get(taskID)
		if ok && record.Notification != nil && ready(record.Notification) {
			return record
		}
		time.Sleep(5 * time.Millisecond)
	}
	record, _ := store.Get(taskID)
	t.Fatalf("notification did not reach expected state: %#v", record.Notification)
	return runtimestore.RuntimeTaskRecord{}
}

func TestBackgroundNotificationFollowUpTargetsOwningSession(t *testing.T) {
	manager := NewBackgroundTaskManager(t.TempDir())
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	ctx := executioncontract.WithToolExecutionContext(context.Background(), executioncontract.ToolExecutionContext{SessionID: "parent-session"})
	snapshot, err := manager.StartAgentTask(ctx, "research", "sector research", func(context.Context, io.Writer) (string, error) {
		return "semiconductor and utilities analysis", nil
	})
	if err != nil {
		t.Fatalf("StartAgentTask: %v", err)
	}
	if _, status := manager.Wait(snapshot.ID, time.Second); status != "success" {
		t.Fatalf("Wait status = %q", status)
	}
	record, ok := manager.store.Get(snapshot.ID)
	if !ok || record.Notification == nil {
		t.Fatalf("notification record = %#v", record)
	}
	target, ok := manager.NotificationFollowUpTarget(*record.Notification)
	if !ok || target.SessionID != "parent-session" {
		t.Fatalf("follow-up target = %q, ok=%v", target.SessionID, ok)
	}
	for _, want := range []string{"<task-notification>", `"notification_id":"` + record.Notification.ID + `"`, "sector research", "semiconductor and utilities analysis"} {
		if !strings.Contains(target.Message, want) {
			t.Fatalf("follow-up message missing %q: %s", want, target.Message)
		}
	}
}
