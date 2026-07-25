package schedule

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	schedulerTickInterval = time.Second
	deliveryWorkers       = 4
	deliveryQueueSize     = maxJobs
	initialRetryDelay     = 5 * time.Second
	maximumRetryDelay     = 5 * time.Minute
)

// Service stores scheduled jobs and delivers due work to an Executor.
type Service struct {
	controlMu  sync.Mutex
	bindingMu  sync.RWMutex
	mu         sync.Mutex
	deliveryMu sync.Mutex

	repository     *repository
	root           string
	location       *time.Location
	executor       Executor
	fireSink       FireSink
	currentAgentID func() string
	clock          func() time.Time

	sessionJobs  map[string]*storedJob
	sessionOrder []string
	inProgress   map[string]struct{}
	retryAt      map[string]time.Time

	running   bool
	runParent context.Context
	runCtx    context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	delivery  chan Execution
	leader    *leaderLock
}

// NewService constructs a schedule service rooted in one project. Start must
// be called before jobs can be delivered.
func NewService(projectRoot string, executor Executor, fireSink FireSink, currentAgentID func() string) (*Service, error) {
	if executor == nil {
		return nil, newDomainError(errorKindStoreInvalid, fs.ErrInvalid)
	}
	repository, err := newRepository(projectRoot)
	if err != nil {
		return nil, err
	}
	leader, err := newLeaderLock(filepath.Join(repository.dir, "leader.json"))
	if err != nil {
		return nil, newDomainError(errorKindID, err)
	}
	return &Service{
		repository:     repository,
		root:           repository.root,
		location:       time.Local,
		executor:       executor,
		fireSink:       fireSink,
		currentAgentID: currentAgentID,
		clock:          time.Now,
		sessionJobs:    make(map[string]*storedJob),
		sessionOrder:   make([]string, 0),
		inProgress:     make(map[string]struct{}),
		retryAt:        make(map[string]time.Time),
		leader:         leader,
	}, nil
}

// Start begins leader probing, due-job collection, and bounded delivery
// workers. The supplied context owns the runtime lifetime.
func (s *Service) Start(ctx context.Context) error {
	if s == nil || ctx == nil {
		return newDomainError(errorKindStoreInvalid, fs.ErrInvalid)
	}
	s.controlMu.Lock()
	defer s.controlMu.Unlock()
	return s.startLocked(ctx)
}

func (s *Service) startLocked(parent context.Context) error {
	if s.running {
		return nil
	}
	if err := parent.Err(); err != nil {
		return err
	}
	if _, err := s.repository.read(); err != nil {
		return err
	}
	if _, err := s.leader.tryAcquire(parent); err != nil {
		return err
	}
	runCtx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	delivery := make(chan Execution, deliveryQueueSize)
	s.running = true
	s.runParent = parent
	s.runCtx = runCtx
	s.cancel = cancel
	s.done = done
	s.delivery = delivery

	var workers sync.WaitGroup
	workers.Add(deliveryWorkers + 1)
	for range deliveryWorkers {
		go func() {
			defer workers.Done()
			s.deliveryLoop(runCtx, delivery)
		}()
	}
	go func() {
		defer workers.Done()
		s.schedulerLoop(runCtx, delivery)
	}()
	go func() {
		workers.Wait()
		close(done)
	}()
	return nil
}

// Close cancels active scheduling and waits for all context-aware executor
// calls to return. A caller deadline bounds the wait.
func (s *Service) Close(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if ctx == nil {
		return newDomainError(errorKindStoreInvalid, fs.ErrInvalid)
	}
	s.controlMu.Lock()
	defer s.controlMu.Unlock()
	return s.stopLocked(ctx)
}

func (s *Service) stopLocked(ctx context.Context) error {
	if !s.running {
		return s.leader.release(ctx)
	}
	cancel := s.cancel
	done := s.done
	if cancel != nil {
		cancel()
	}
	// Leadership is surrendered before waiting for executor calls. Even a
	// misbehaving executor must not keep this stopped service eligible to
	// dispatch durable work in another process.
	releaseErr := s.leader.release(ctx)
	select {
	case <-done:
	case <-ctx.Done():
		return errors.Join(releaseErr, ctx.Err())
	}
	s.running = false
	s.cancel = nil
	s.done = nil
	s.delivery = nil
	s.deliveryMu.Lock()
	clear(s.inProgress)
	clear(s.retryAt)
	s.deliveryMu.Unlock()
	return releaseErr
}

// Rebind atomically stops scheduling, clears all session-scoped jobs, and
// starts a fresh runtime rooted at projectRoot. Durable jobs are not migrated.
func (s *Service) Rebind(ctx context.Context, projectRoot string) error {
	if s == nil || ctx == nil {
		return newDomainError(errorKindStoreInvalid, fs.ErrInvalid)
	}
	repository, err := newRepository(projectRoot)
	if err != nil {
		return err
	}
	leader, err := newLeaderLock(filepath.Join(repository.dir, "leader.json"))
	if err != nil {
		return newDomainError(errorKindID, err)
	}
	s.controlMu.Lock()
	defer s.controlMu.Unlock()
	if repository.root == s.root {
		return nil
	}
	wasRunning := s.running
	parent := s.runParent
	if err := s.stopLocked(ctx); err != nil {
		return err
	}
	s.bindingMu.Lock()
	s.repository = repository
	s.root = repository.root
	s.leader = leader
	s.bindingMu.Unlock()
	s.mu.Lock()
	s.sessionJobs = make(map[string]*storedJob)
	s.sessionOrder = nil
	s.mu.Unlock()
	s.deliveryMu.Lock()
	s.inProgress = make(map[string]struct{})
	s.retryAt = make(map[string]time.Time)
	s.deliveryMu.Unlock()
	if wasRunning {
		return s.startLocked(parent)
	}
	return nil
}

func (s *Service) schedulerLoop(ctx context.Context, delivery chan<- Execution) {
	ticker := time.NewTicker(schedulerTickInterval)
	probe := time.NewTicker(s.leader.probeInterval())
	defer ticker.Stop()
	defer probe.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-probe.C:
			if !s.leader.isHolder() {
				_, _ = s.leader.tryAcquire(ctx)
			}
		case <-ticker.C:
			now := s.clock().UTC()
			executions, _ := s.claimDue(now)
			for _, execution := range executions {
				select {
				case delivery <- execution:
				case <-ctx.Done():
					s.releaseInProgress(execution.DeliveryID, time.Time{})
					return
				}
			}
		}
	}
}

func (s *Service) deliveryLoop(ctx context.Context, delivery <-chan Execution) {
	for {
		select {
		case <-ctx.Done():
			return
		case execution := <-delivery:
			err := s.executor.Enqueue(ctx, execution)
			if err != nil {
				_ = s.failDelivery(execution, s.clock().UTC())
				continue
			}
			if s.fireSink != nil {
				if err := s.fireSink.PublishScheduleFire(ctx, FireEvent{
					DeliveryID: execution.DeliveryID,
					JobID:      execution.Job.ID, Expression: execution.Job.Expression,
					Recurring: execution.Job.Recurring, Durable: execution.Job.Durable,
					ScheduledAt: execution.ScheduledAt, ProjectRoot: execution.Job.ProjectRoot,
				}); err != nil {
					_ = s.failDelivery(execution, s.clock().UTC())
					continue
				}
			}
			if err := s.ackDelivery(execution); err != nil {
				s.releaseInProgress(execution.DeliveryID, s.clock().UTC().Add(initialRetryDelay))
				continue
			}
		}
	}
}

func (s *Service) claimDue(now time.Time) ([]Execution, error) {
	s.bindingMu.RLock()
	defer s.bindingMu.RUnlock()
	claimed := make([]Execution, 0)
	s.mu.Lock()
	for _, id := range s.sessionOrder {
		job := s.sessionJobs[id]
		execution, ok, err := s.claimJobLocked(job, now)
		if err != nil {
			s.mu.Unlock()
			return nil, err
		}
		if ok {
			claimed = append(claimed, execution)
		}
	}
	s.mu.Unlock()

	if !s.leader.isHolder() {
		return claimed, nil
	}
	durableClaimed := make([]Execution, 0)
	_, err := s.repository.update(func(jobs []*storedJob) ([]*storedJob, bool, error) {
		changed := false
		for _, job := range jobs {
			execution, ok, err := s.claimJobLocked(job, now)
			if err != nil {
				return nil, false, err
			}
			if ok {
				changed = true
				durableClaimed = append(durableClaimed, execution)
			}
		}
		return jobs, changed, nil
	})
	if err != nil {
		for _, execution := range durableClaimed {
			s.releaseInProgress(execution.DeliveryID, now.Add(initialRetryDelay))
		}
		return claimed, err
	}
	return append(claimed, durableClaimed...), nil
}

func (s *Service) claimJobLocked(job *storedJob, now time.Time) (Execution, bool, error) {
	if job == nil {
		return Execution{}, false, nil
	}
	if job.Pending == nil {
		scheduledAt, wallKey, ok, err := s.nextScheduled(job)
		if err != nil || !ok || now.Before(scheduledAt) {
			return Execution{}, false, err
		}
		deliveryID := deliveryID(job.ID, scheduledAt)
		job.Pending = &pendingDelivery{
			ID: deliveryID, ScheduledAt: scheduledAt.UTC(), WallKey: wallKey,
			NextAttemptAt: now, Attempt: 1,
		}
	}
	if now.Before(job.Pending.NextAttemptAt) {
		return Execution{}, false, nil
	}
	s.deliveryMu.Lock()
	defer s.deliveryMu.Unlock()
	if retryAt := s.retryAt[job.Pending.ID]; !retryAt.IsZero() && now.Before(retryAt) {
		return Execution{}, false, nil
	}
	if _, exists := s.inProgress[job.Pending.ID]; exists {
		return Execution{}, false, nil
	}
	s.inProgress[job.Pending.ID] = struct{}{}
	return Execution{
		DeliveryID:  job.Pending.ID,
		ScheduledAt: job.Pending.ScheduledAt,
		Job:         job.Job,
	}, true, nil
}

func (s *Service) nextScheduled(job *storedJob) (time.Time, string, bool, error) {
	parsed, err := parseExpression(job.Expression)
	if err != nil {
		return time.Time{}, "", false, err
	}
	anchor := job.CreatedAt
	if job.Recurring && job.LastFiredAt != nil {
		anchor = *job.LastFiredAt
	}
	next, wallKey, ok := nextRun(parsed, anchor, s.location, job.LastWallKey)
	if !ok {
		return time.Time{}, "", false, nil
	}
	if job.Recurring {
		following, _, found := nextRun(parsed, next, s.location, wallKey)
		if found {
			next = next.Add(recurringJitter(following.Sub(next), job.ID))
		}
	} else {
		jittered := next.Add(oneshotJitter(next, job.ID))
		if jittered.Before(job.CreatedAt) {
			jittered = job.CreatedAt
		}
		next = jittered
	}
	return next.UTC(), wallKey, true, nil
}

func deliveryID(jobID string, scheduledAt time.Time) string {
	return jobID + ":" + scheduledAt.UTC().Format("20060102T150405.000000000Z")
}

func (s *Service) ackDelivery(execution Execution) error {
	if execution.Job.Durable {
		_, err := s.repository.update(func(jobs []*storedJob) ([]*storedJob, bool, error) {
			updated, found := acknowledge(jobs, execution)
			return updated, found, nil
		})
		if err != nil {
			return err
		}
	} else {
		s.mu.Lock()
		jobs := make([]*storedJob, 0, len(s.sessionOrder))
		for _, id := range s.sessionOrder {
			if job := s.sessionJobs[id]; job != nil {
				jobs = append(jobs, job)
			}
		}
		remaining, _ := acknowledge(jobs, execution)
		s.sessionJobs = make(map[string]*storedJob, len(remaining))
		s.sessionOrder = s.sessionOrder[:0]
		for _, job := range remaining {
			s.sessionJobs[job.ID] = job
			s.sessionOrder = append(s.sessionOrder, job.ID)
		}
		s.mu.Unlock()
	}
	s.releaseInProgress(execution.DeliveryID, time.Time{})
	return nil
}

func acknowledge(jobs []*storedJob, execution Execution) ([]*storedJob, bool) {
	remaining := jobs[:0]
	found := false
	for _, job := range jobs {
		if job.ID != execution.Job.ID || job.Pending == nil || job.Pending.ID != execution.DeliveryID {
			remaining = append(remaining, job)
			continue
		}
		found = true
		if !job.Recurring {
			continue
		}
		firedAt := execution.ScheduledAt.UTC()
		job.LastFiredAt = &firedAt
		job.LastWallKey = job.Pending.WallKey
		job.Pending = nil
		remaining = append(remaining, job)
	}
	return remaining, found
}

func (s *Service) failDelivery(execution Execution, now time.Time) error {
	if execution.Job.Durable {
		_, err := s.repository.update(func(jobs []*storedJob) ([]*storedJob, bool, error) {
			changed := markDeliveryFailed(jobs, execution, now)
			return jobs, changed, nil
		})
		if err != nil {
			s.releaseInProgress(execution.DeliveryID, now.Add(initialRetryDelay))
			return err
		}
	} else {
		s.mu.Lock()
		if job := s.sessionJobs[execution.Job.ID]; job != nil {
			_ = markDeliveryFailed([]*storedJob{job}, execution, now)
		}
		s.mu.Unlock()
	}
	s.releaseInProgress(execution.DeliveryID, time.Time{})
	return nil
}

func markDeliveryFailed(jobs []*storedJob, execution Execution, now time.Time) bool {
	for _, job := range jobs {
		if job.ID != execution.Job.ID || job.Pending == nil || job.Pending.ID != execution.DeliveryID {
			continue
		}
		job.Pending.Attempt++
		job.Pending.NextAttemptAt = now.Add(retryDelay(job.Pending.Attempt))
		return true
	}
	return false
}

func retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := initialRetryDelay
	for n := 1; n < attempt && delay < maximumRetryDelay; n++ {
		delay *= 2
		if delay > maximumRetryDelay {
			delay = maximumRetryDelay
		}
	}
	return delay
}

func (s *Service) releaseInProgress(id string, retryAt time.Time) {
	s.deliveryMu.Lock()
	delete(s.inProgress, id)
	if retryAt.IsZero() {
		delete(s.retryAt, id)
	} else {
		s.retryAt[id] = retryAt
	}
	s.deliveryMu.Unlock()
}

func newJobID() (string, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", newDomainError(errorKindID, err)
	}
	return hex.EncodeToString(raw[:]), nil
}

func (s *Service) create(expression, prompt string, recurring, durable bool, agentID, projectRoot string, now time.Time) (*storedJob, error) {
	s.bindingMu.RLock()
	defer s.bindingMu.RUnlock()
	parsed, err := parseExpression(expression)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(prompt) == "" {
		return nil, newDomainError(errorKindStoreInvalid, fs.ErrInvalid)
	}
	if durable && strings.TrimSpace(agentID) != "" {
		return nil, newDomainError(errorKindDurableDenied, fs.ErrPermission)
	}
	id, err := newJobID()
	if err != nil {
		return nil, err
	}
	jobRoot := strings.TrimSpace(projectRoot)
	if jobRoot == "" {
		jobRoot = s.root
	} else if absolute, absoluteErr := filepath.Abs(filepath.Clean(jobRoot)); absoluteErr == nil {
		jobRoot = absolute
	} else {
		return nil, newDomainError(errorKindStoreInvalid, absoluteErr)
	}
	job := &storedJob{Job: Job{
		ID: id, Expression: expression, Prompt: prompt, Recurring: recurring,
		Durable: durable, CreatedAt: now.UTC(), AgentID: strings.TrimSpace(agentID), ProjectRoot: jobRoot,
	}}
	if _, _, ok := nextRun(parsed, now, s.location, ""); !ok {
		return nil, newDomainError(errorKindStoreInvalid, fs.ErrInvalid)
	}
	if durable {
		_, err := s.repository.update(func(jobs []*storedJob) ([]*storedJob, bool, error) {
			s.mu.Lock()
			sessionCount := len(s.sessionJobs)
			s.mu.Unlock()
			if len(jobs)+sessionCount >= maxJobs {
				return nil, false, newDomainError(errorKindTooMany, fs.ErrInvalid)
			}
			return append(jobs, job), true, nil
		})
		if err != nil {
			return nil, err
		}
		return cloneStoredJob(job), nil
	}
	durableJobs, err := s.repository.read()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(durableJobs)+len(s.sessionJobs) >= maxJobs {
		return nil, newDomainError(errorKindTooMany, fs.ErrInvalid)
	}
	s.sessionJobs[id] = job
	s.sessionOrder = append(s.sessionOrder, id)
	return cloneStoredJob(job), nil
}

func (s *Service) list(agentID string) ([]*storedJob, error) {
	s.bindingMu.RLock()
	defer s.bindingMu.RUnlock()
	durable, err := s.repository.read()
	if err != nil {
		return nil, err
	}
	current := strings.TrimSpace(agentID)
	items := make([]*storedJob, 0, len(durable))
	if current == "" {
		for _, job := range durable {
			items = append(items, cloneStoredJob(job))
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range s.sessionOrder {
		job := s.sessionJobs[id]
		if job == nil || (current != "" && job.AgentID != current) {
			continue
		}
		items = append(items, cloneStoredJob(job))
	}
	return items, nil
}

func (s *Service) delete(id, agentID string) error {
	s.bindingMu.RLock()
	defer s.bindingMu.RUnlock()
	trimmedID := strings.TrimSpace(id)
	current := strings.TrimSpace(agentID)
	s.mu.Lock()
	if job := s.sessionJobs[trimmedID]; job != nil {
		if current != "" && job.AgentID != current {
			s.mu.Unlock()
			return newDomainError(errorKindOwnerDenied, fs.ErrPermission)
		}
		delete(s.sessionJobs, trimmedID)
		for index, orderedID := range s.sessionOrder {
			if orderedID == trimmedID {
				s.sessionOrder = append(s.sessionOrder[:index], s.sessionOrder[index+1:]...)
				break
			}
		}
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()
	if current != "" {
		return newDomainError(errorKindOwnerDenied, fs.ErrPermission)
	}
	found := false
	_, err := s.repository.update(func(jobs []*storedJob) ([]*storedJob, bool, error) {
		filtered := jobs[:0]
		for _, job := range jobs {
			if job.ID == trimmedID {
				found = true
				continue
			}
			filtered = append(filtered, job)
		}
		if !found {
			return nil, false, newDomainError(errorKindNotFound, fs.ErrNotExist)
		}
		return filtered, true, nil
	})
	return err
}

func (s *Service) agentID() string {
	if s != nil && s.currentAgentID != nil {
		return strings.TrimSpace(s.currentAgentID())
	}
	return ""
}
