package agent

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
)

func TestBackgroundTaskManagerShutdownDeadlineDoesNotAbandonFinalizer(t *testing.T) {
	manager := NewBackgroundTaskManager(t.TempDir())
	runnerStarted := make(chan struct{})
	runnerCanceled := make(chan struct{})
	releaseRunner := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(releaseRunner) })
		_ = manager.Shutdown(context.Background())
	})

	_, err := manager.StartAgentTask(context.Background(), "shutdown", "shutdown", func(ctx context.Context, _ io.Writer) (string, error) {
		close(runnerStarted)
		<-ctx.Done()
		close(runnerCanceled)
		<-releaseRunner
		return "", ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	<-runnerStarted

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := manager.Shutdown(shutdownCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown error = %v, want context deadline", err)
	}
	select {
	case <-runnerCanceled:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not cancel the active task context")
	}
	select {
	case <-manager.shutdownDone:
		t.Fatal("shutdown finalizer completed before the admitted task drained")
	default:
	}

	canceledCtx, cancelImmediately := context.WithCancel(context.Background())
	cancelImmediately()
	if err := manager.Shutdown(canceledCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("second Shutdown error = %v, want context cancellation", err)
	}

	releaseOnce.Do(func() { close(releaseRunner) })
	if err := manager.Shutdown(context.Background()); err != nil {
		t.Fatalf("wait for finalizer: %v", err)
	}
	select {
	case <-manager.shutdownDone:
	default:
		t.Fatal("shutdown finalizer did not close its completion channel")
	}
	if err := manager.Shutdown(context.Background()); err != nil {
		t.Fatalf("completed shutdown was not idempotent: %v", err)
	}
}

func TestBackgroundTaskManagerShutdownCancelsAndDrainsSessions(t *testing.T) {
	manager := NewBackgroundTaskManager(t.TempDir())
	sessionCtx, sessionCancel := manager.managedContext(context.Background())
	sessionDone := make(chan struct{})
	sessionCanceled := make(chan struct{})
	releaseSession := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(releaseSession) })
		_ = manager.Shutdown(context.Background())
	})

	session := &backgroundAgentSession{
		parent: sessionCtx,
		cancel: sessionCancel,
		done:   sessionDone,
	}
	manager.mu.Lock()
	manager.sessions["session"] = session
	manager.mu.Unlock()
	go func() {
		<-sessionCtx.Done()
		close(sessionCanceled)
		<-releaseSession
		close(sessionDone)
	}()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := manager.Shutdown(shutdownCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown error = %v, want context deadline", err)
	}
	select {
	case <-sessionCanceled:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not cancel retained session")
	}
	select {
	case <-manager.shutdownDone:
		t.Fatal("shutdown finalizer completed before retained session drained")
	default:
	}

	releaseOnce.Do(func() { close(releaseSession) })
	if err := manager.Shutdown(context.Background()); err != nil {
		t.Fatalf("wait for session drain: %v", err)
	}
}

func TestBackgroundTaskManagerShutdownCancelsFollowUpDelivery(t *testing.T) {
	manager := NewBackgroundTaskManager(t.TempDir())
	deliveryStarted := make(chan struct{})
	deliveryCanceled := make(chan struct{})
	manager.mu.Lock()
	manager.notificationFollowUp = RuntimeNotificationSinkFunc(func(ctx context.Context, _ agentcontract.RuntimeNotification) error {
		close(deliveryStarted)
		<-ctx.Done()
		close(deliveryCanceled)
		return ctx.Err()
	})
	manager.mu.Unlock()

	queueKey := "session"
	notification := agentcontract.RuntimeNotification{ID: "notification", TaskID: "task"}
	manager.notificationMu.Lock()
	manager.followUpQueues[queueKey] = &runtimeNotificationFollowUpQueue{
		items: []runtimeNotificationFollowUpItem{{notification: notification}},
	}
	manager.followUpQueued[runtimeNotificationFollowUpIdentity(notification)] = queueKey
	if !manager.startRuntimeNotificationFollowUpWorkerLocked(queueKey) {
		manager.notificationMu.Unlock()
		t.Fatal("follow-up worker was not admitted")
	}
	manager.notificationMu.Unlock()
	<-deliveryStarted

	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := manager.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	select {
	case <-deliveryCanceled:
	default:
		t.Fatal("follow-up delivery did not observe manager cancellation")
	}
	manager.notificationMu.Lock()
	queue := manager.followUpQueues[queueKey]
	workerRunning := queue != nil && queue.worker
	manager.notificationMu.Unlock()
	if workerRunning {
		t.Fatal("follow-up worker remained active after shutdown")
	}
}

func TestBackgroundTaskManagerShutdownConcurrentCallers(t *testing.T) {
	manager := NewBackgroundTaskManager(t.TempDir())
	runnerStarted := make(chan struct{})
	releaseRunner := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(releaseRunner) })
		_ = manager.Shutdown(context.Background())
	})

	_, err := manager.StartAgentTask(context.Background(), "concurrent", "concurrent", func(ctx context.Context, _ io.Writer) (string, error) {
		close(runnerStarted)
		<-ctx.Done()
		<-releaseRunner
		return "", ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	<-runnerStarted

	const callers = 32
	errs := make(chan error, callers)
	var callersWG sync.WaitGroup
	for range callers {
		callersWG.Add(1)
		go func() {
			defer callersWG.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			errs <- manager.Shutdown(ctx)
		}()
	}
	callersWG.Wait()
	close(errs)
	for err := range errs {
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("concurrent Shutdown error = %v, want context deadline", err)
		}
	}

	releaseOnce.Do(func() { close(releaseRunner) })
	if err := manager.Shutdown(context.Background()); err != nil {
		t.Fatalf("wait for concurrent shutdown finalizer: %v", err)
	}

	errs = make(chan error, callers)
	for range callers {
		callersWG.Add(1)
		go func() {
			defer callersWG.Done()
			errs <- manager.Shutdown(context.Background())
		}()
	}
	callersWG.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("completed concurrent Shutdown error = %v", err)
		}
	}
}
