package schedule

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type ExecutorFunc func(context.Context, Execution) error

func (f ExecutorFunc) Enqueue(ctx context.Context, execution Execution) error {
	return f(ctx, execution)
}

type FireSinkFunc func(context.Context, FireEvent) error

func (f FireSinkFunc) PublishScheduleFire(ctx context.Context, event FireEvent) error {
	return f(ctx, event)
}

func TestSuccessfulDeliveryAcknowledgesOneShotAndAdvancesRecurring(t *testing.T) {
	accepted := make(chan Execution, 2)
	service := newTestService(t, ExecutorFunc(func(_ context.Context, execution Execution) error {
		accepted <- execution
		return nil
	}))
	now := time.Date(2026, time.July, 25, 10, 5, 0, 0, time.UTC)
	service.clock = func() time.Time { return now }

	ctx, cancel := context.WithCancel(context.Background())
	delivery := make(chan Execution, 2)
	go service.deliveryLoop(ctx, delivery)
	t.Cleanup(cancel)

	oneShot := addDueSessionJob(t, service, "one-shot", false, now)
	oneShotExecution := claimTestJob(t, service, oneShot, now)
	wantOneShotID := deliveryID(oneShot.ID, oneShotExecution.ScheduledAt)
	if oneShotExecution.DeliveryID != wantOneShotID {
		t.Fatalf("one-shot delivery ID = %q, want %q", oneShotExecution.DeliveryID, wantOneShotID)
	}
	delivery <- oneShotExecution
	if got := receiveExecution(t, accepted); got.DeliveryID != wantOneShotID {
		t.Fatalf("executor accepted delivery %q, want %q", got.DeliveryID, wantOneShotID)
	}
	waitScheduleCondition(t, func() bool {
		service.mu.Lock()
		defer service.mu.Unlock()
		return service.sessionJobs[oneShot.ID] == nil
	})

	recurring := addDueSessionJob(t, service, "recurring", true, now)
	recurringExecution := claimTestJob(t, service, recurring, now)
	wantRecurringID := deliveryID(recurring.ID, recurringExecution.ScheduledAt)
	delivery <- recurringExecution
	if got := receiveExecution(t, accepted); got.DeliveryID != wantRecurringID {
		t.Fatalf("executor accepted recurring delivery %q, want %q", got.DeliveryID, wantRecurringID)
	}
	waitScheduleCondition(t, func() bool {
		service.mu.Lock()
		defer service.mu.Unlock()
		job := service.sessionJobs[recurring.ID]
		return job != nil && job.Pending == nil && job.LastFiredAt != nil
	})
	service.mu.Lock()
	advanced := cloneStoredJob(service.sessionJobs[recurring.ID])
	service.mu.Unlock()
	if !advanced.LastFiredAt.Equal(recurringExecution.ScheduledAt) {
		t.Fatalf("last fired time = %v, want %v", advanced.LastFiredAt, recurringExecution.ScheduledAt)
	}
	if advanced.LastWallKey == "" {
		t.Fatal("recurring acknowledgement did not preserve the delivered wall-minute key")
	}
}

func TestFailedDeliveryKeepsStableIDAndAppliesBackoff(t *testing.T) {
	failed := errors.New("executor unavailable")
	called := make(chan Execution, 1)
	service := newTestService(t, ExecutorFunc(func(_ context.Context, execution Execution) error {
		called <- execution
		return failed
	}))
	now := time.Date(2026, time.July, 25, 10, 5, 0, 0, time.UTC)
	service.clock = func() time.Time { return now }
	job := addDueSessionJob(t, service, "retry", false, now)
	first := claimTestJob(t, service, job, now)

	ctx, cancel := context.WithCancel(context.Background())
	delivery := make(chan Execution, 1)
	go service.deliveryLoop(ctx, delivery)
	t.Cleanup(cancel)
	delivery <- first
	if got := receiveExecution(t, called); got.DeliveryID != first.DeliveryID {
		t.Fatalf("failed executor received delivery %q, want %q", got.DeliveryID, first.DeliveryID)
	}

	wantRetryAt := now.Add(retryDelay(2))
	waitScheduleCondition(t, func() bool {
		service.mu.Lock()
		defer service.mu.Unlock()
		pending := service.sessionJobs[job.ID].Pending
		return pending != nil && pending.Attempt == 2 && pending.NextAttemptAt.Equal(wantRetryAt)
	})
	service.mu.Lock()
	stored := service.sessionJobs[job.ID]
	if stored.Pending.ID != first.DeliveryID {
		service.mu.Unlock()
		t.Fatalf("failed delivery ID changed to %q, want %q", stored.Pending.ID, first.DeliveryID)
	}
	_, early, err := service.claimJobLocked(stored, wantRetryAt.Add(-time.Nanosecond))
	service.mu.Unlock()
	if err != nil {
		t.Fatalf("claim before retry: %v", err)
	}
	if early {
		t.Fatal("delivery was claimable before its retry backoff elapsed")
	}

	service.mu.Lock()
	retry, ok, err := service.claimJobLocked(service.sessionJobs[job.ID], wantRetryAt)
	service.mu.Unlock()
	if err != nil {
		t.Fatalf("claim at retry time: %v", err)
	}
	if !ok || retry.DeliveryID != first.DeliveryID {
		t.Fatalf("retry = (%t, %q), want stable delivery %q", ok, retry.DeliveryID, first.DeliveryID)
	}
}

func TestFirePersistenceFailureRetriesBeforeAcknowledgement(t *testing.T) {
	now := time.Date(2026, time.July, 25, 10, 5, 0, 0, time.UTC)
	executorCalls := make(chan Execution, 2)
	service := newTestService(t, ExecutorFunc(func(_ context.Context, execution Execution) error {
		executorCalls <- execution
		return nil
	}))
	service.clock = func() time.Time { return now }
	var sinkCalls int
	service.fireSink = FireSinkFunc(func(_ context.Context, _ FireEvent) error {
		sinkCalls++
		if sinkCalls == 1 {
			return errors.New("lifecycle storage unavailable")
		}
		return nil
	})
	job := addDueSessionJob(t, service, "fire-retry", false, now)
	first := claimTestJob(t, service, job, now)
	ctx, cancel := context.WithCancel(context.Background())
	delivery := make(chan Execution, 1)
	go service.deliveryLoop(ctx, delivery)
	t.Cleanup(cancel)
	delivery <- first
	if got := receiveExecution(t, executorCalls); got.DeliveryID != first.DeliveryID {
		t.Fatalf("first executor delivery = %q, want %q", got.DeliveryID, first.DeliveryID)
	}
	wantRetryAt := now.Add(retryDelay(2))
	waitScheduleCondition(t, func() bool {
		service.mu.Lock()
		defer service.mu.Unlock()
		pending := service.sessionJobs[job.ID].Pending
		return pending != nil && pending.Attempt == 2 && pending.NextAttemptAt.Equal(wantRetryAt)
	})
	service.mu.Lock()
	retry, ok, err := service.claimJobLocked(service.sessionJobs[job.ID], wantRetryAt)
	service.mu.Unlock()
	if err != nil || !ok {
		t.Fatalf("claim fire retry = (%t, %v)", ok, err)
	}
	delivery <- retry
	if got := receiveExecution(t, executorCalls); got.DeliveryID != first.DeliveryID {
		t.Fatalf("retry executor delivery = %q, want stable %q", got.DeliveryID, first.DeliveryID)
	}
	waitScheduleCondition(t, func() bool {
		service.mu.Lock()
		defer service.mu.Unlock()
		return service.sessionJobs[job.ID] == nil
	})
	if sinkCalls != 2 {
		t.Fatalf("fire sink calls = %d, want 2", sinkCalls)
	}
}

func TestPendingDurableDeliveryReloadsWithStableID(t *testing.T) {
	root := t.TempDir()
	repo, err := newRepository(root)
	if err != nil {
		t.Fatalf("new repository: %v", err)
	}
	now := time.Date(2026, time.July, 25, 10, 5, 0, 0, time.UTC)
	scheduledAt := now.Add(-time.Minute)
	wantID := deliveryID("durable-reload", scheduledAt)
	job := &storedJob{Job: Job{
		ID: "durable-reload", Expression: "* * * * *", Prompt: "resume pending work",
		Recurring: false, Durable: true, CreatedAt: now.Add(-2 * time.Minute), ProjectRoot: repo.root,
	}, Pending: &pendingDelivery{
		ID: wantID, ScheduledAt: scheduledAt, WallKey: "20260725T1004", NextAttemptAt: now.Add(-time.Second), Attempt: 3,
	}}
	if _, err := repo.update(func(jobs []*storedJob) ([]*storedJob, bool, error) {
		return append(jobs, job), true, nil
	}); err != nil {
		t.Fatalf("persist pending delivery: %v", err)
	}

	restarted := newTestServiceAtRoot(t, root, ExecutorFunc(func(context.Context, Execution) error { return nil }))
	jobs, err := restarted.repository.read()
	if err != nil {
		t.Fatalf("reload pending delivery: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Pending == nil {
		t.Fatalf("pending delivery missing after reload: %#v", jobs)
	}
	execution, ok, err := restarted.claimJobLocked(jobs[0], now)
	if err != nil {
		t.Fatalf("claim reloaded pending delivery: %v", err)
	}
	if !ok || execution.DeliveryID != wantID || execution.ScheduledAt != scheduledAt {
		t.Fatalf("reloaded execution = (%t, %q, %v), want (%q, %v)", ok, execution.DeliveryID, execution.ScheduledAt, wantID, scheduledAt)
	}
}

func TestRebindClearsSessionJobsAndDoesNotMoveClaimedJob(t *testing.T) {
	oldRoot := t.TempDir()
	newRoot := t.TempDir()
	service := newTestServiceAtRoot(t, oldRoot, ExecutorFunc(func(context.Context, Execution) error { return nil }))
	now := time.Date(2026, time.July, 25, 10, 5, 0, 0, time.UTC)
	job, err := service.create("* * * * *", "old project work", false, false, "agent-1", "", now.Add(-2*time.Minute))
	if err != nil {
		t.Fatalf("create old-root session job: %v", err)
	}
	execution := claimTestJob(t, service, job, now)
	wantOldRoot, err := filepath.Abs(oldRoot)
	if err != nil {
		t.Fatalf("resolve old root: %v", err)
	}
	if execution.Job.ProjectRoot != wantOldRoot {
		t.Fatalf("claimed project root = %q, want %q", execution.Job.ProjectRoot, wantOldRoot)
	}

	if err := service.Rebind(context.Background(), newRoot); err != nil {
		t.Fatalf("rebind service: %v", err)
	}
	jobs, err := service.list("agent-1")
	if err != nil {
		t.Fatalf("list after rebind: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("session jobs survived rebind: %#v", jobs)
	}
	if execution.Job.ProjectRoot != wantOldRoot {
		t.Fatalf("already-claimed job moved during rebind: got %q, want %q", execution.Job.ProjectRoot, wantOldRoot)
	}
	created, err := service.create("* * * * *", "new project work", false, false, "agent-1", "", now)
	if err != nil {
		t.Fatalf("create new-root session job: %v", err)
	}
	wantNewRoot, err := filepath.Abs(newRoot)
	if err != nil {
		t.Fatalf("resolve new root: %v", err)
	}
	if created.ProjectRoot != wantNewRoot {
		t.Fatalf("new job project root = %q, want %q", created.ProjectRoot, wantNewRoot)
	}
}

func TestCloseCancelsBlockingExecutor(t *testing.T) {
	started := make(chan struct{})
	exited := make(chan struct{})
	var once sync.Once
	service := newTestService(t, ExecutorFunc(func(ctx context.Context, _ Execution) error {
		once.Do(func() { close(started) })
		<-ctx.Done()
		close(exited)
		return ctx.Err()
	}))
	if err := service.Start(context.Background()); err != nil {
		t.Fatalf("start service: %v", err)
	}

	service.delivery <- Execution{
		DeliveryID: "blocking-delivery", ScheduledAt: time.Now().UTC(),
		Job: Job{ID: "blocking", Expression: "* * * * *", Prompt: "block", ProjectRoot: service.root},
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("executor did not start")
	}

	closeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := service.Close(closeCtx); err != nil {
		t.Fatalf("close service with blocking executor: %v", err)
	}
	select {
	case <-exited:
	default:
		t.Fatal("Close returned before the blocking executor observed cancellation")
	}
}

func TestConcurrentCreateListDeleteIsRaceSafe(t *testing.T) {
	service := newTestService(t, ExecutorFunc(func(context.Context, Execution) error { return nil }))
	const workers = 12
	const iterations = 12
	start := make(chan struct{})
	errCh := make(chan error, workers)
	var group sync.WaitGroup
	group.Add(workers)
	for worker := 0; worker < workers; worker++ {
		worker := worker
		go func() {
			defer group.Done()
			<-start
			agentID := fmt.Sprintf("agent-%d", worker)
			for iteration := 0; iteration < iterations; iteration++ {
				now := time.Date(2026, time.July, 25, 10, worker, iteration, 0, time.UTC)
				job, err := service.create("* * * * *", "concurrent work", false, false, agentID, "", now)
				if err != nil {
					errCh <- fmt.Errorf("worker %d create: %w", worker, err)
					return
				}
				if _, err := service.list(agentID); err != nil {
					errCh <- fmt.Errorf("worker %d list: %w", worker, err)
					return
				}
				if err := service.delete(job.ID, agentID); err != nil {
					errCh <- fmt.Errorf("worker %d delete: %w", worker, err)
					return
				}
			}
		}()
	}
	close(start)
	group.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
}

func newTestService(t *testing.T, executor Executor) *Service {
	t.Helper()
	return newTestServiceAtRoot(t, t.TempDir(), executor)
}

func newTestServiceAtRoot(t *testing.T, root string, executor Executor) *Service {
	t.Helper()
	service, err := NewService(root, executor, nil, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	service.location = time.UTC
	return service
}

func addDueSessionJob(t *testing.T, service *Service, id string, recurring bool, now time.Time) *storedJob {
	t.Helper()
	job := &storedJob{Job: Job{
		ID: id, Expression: "* * * * *", Prompt: "scheduled work", Recurring: recurring,
		CreatedAt: now.Add(-3 * time.Minute), ProjectRoot: service.root,
	}}
	service.mu.Lock()
	service.sessionJobs[id] = job
	service.sessionOrder = append(service.sessionOrder, id)
	service.mu.Unlock()
	return job
}

func claimTestJob(t *testing.T, service *Service, job *storedJob, now time.Time) Execution {
	t.Helper()
	service.mu.Lock()
	execution, ok, err := service.claimJobLocked(job, now)
	service.mu.Unlock()
	if err != nil {
		t.Fatalf("claim due job: %v", err)
	}
	if !ok {
		t.Fatal("expected job to be due")
	}
	return execution
}

func receiveExecution(t *testing.T, executions <-chan Execution) Execution {
	t.Helper()
	select {
	case execution := <-executions:
		return execution
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for executor")
		return Execution{}
	}
}

func waitScheduleCondition(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for schedule state")
}
