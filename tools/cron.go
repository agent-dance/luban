package tools

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
	"github.com/rivo/uniseg"
)

var errCronTooManyJobs = i18n.NewError(i18n.KeyToolLegacyACronTooMany)

// ─── Cron expression parsing ──────────────────────────────────────────────────

// CronField represents a parsed cron field (minute, hour, etc.)
type CronField struct {
	values map[int]bool // set of valid values
}

// Has returns true if v is a valid value for this field.
func (f CronField) Has(v int) bool {
	return f.values[v]
}

// CronSchedule holds the parsed 5-field cron schedule.
type CronSchedule struct {
	Minute     CronField // 0-59
	Hour       CronField // 0-23
	DayOfMonth CronField // 1-31
	Month      CronField // 1-12
	DayOfWeek  CronField // 0-6 (Sunday=0)
}

// Matches returns true if the given time matches this schedule.
func (s *CronSchedule) Matches(t time.Time) bool {
	return s.Minute.Has(t.Minute()) &&
		s.Hour.Has(t.Hour()) &&
		s.DayOfMonth.Has(t.Day()) &&
		s.Month.Has(int(t.Month())) &&
		s.DayOfWeek.Has(int(t.Weekday()))
}

// ParseCron parses a 5-field cron expression into an executable schedule.
// Supports: numbers (5), ranges (1-5), steps (*/15), lists (1,3,5), wildcards (*)
func ParseCron(expr string) (*CronSchedule, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("cron expression must have exactly 5 fields (got %d): %q", len(fields), expr)
	}

	var (
		sched CronSchedule
		err   error
	)

	if sched.Minute, err = parseCronField(fields[0], 0, 59); err != nil {
		return nil, fmt.Errorf("minute field: %w", err)
	}
	if sched.Hour, err = parseCronField(fields[1], 0, 23); err != nil {
		return nil, fmt.Errorf("hour field: %w", err)
	}
	if sched.DayOfMonth, err = parseCronField(fields[2], 1, 31); err != nil {
		return nil, fmt.Errorf("day-of-month field: %w", err)
	}
	if sched.Month, err = parseCronField(fields[3], 1, 12); err != nil {
		return nil, fmt.Errorf("month field: %w", err)
	}
	if sched.DayOfWeek, err = parseCronField(fields[4], 0, 6); err != nil {
		return nil, fmt.Errorf("day-of-week field: %w", err)
	}

	return &sched, nil
}

// parseCronField parses a single cron field that may contain comma-separated parts.
func parseCronField(s string, min, max int) (CronField, error) {
	field := CronField{values: make(map[int]bool)}
	for _, part := range strings.Split(s, ",") {
		if err := parseCronPart(part, min, max, field.values); err != nil {
			return field, err
		}
	}
	return field, nil
}

// parseCronPart parses one element of a cron field: *, N, N-M, */S, N-M/S.
func parseCronPart(s string, min, max int, values map[int]bool) error {
	step := 1

	// Extract optional /step suffix.
	if idx := strings.Index(s, "/"); idx >= 0 {
		stepStr := s[idx+1:]
		s = s[:idx]
		var err error
		step, err = strconv.Atoi(stepStr)
		if err != nil || step <= 0 {
			return fmt.Errorf("invalid step %q", stepStr)
		}
	}

	var lo, hi int
	switch {
	case s == "*":
		lo, hi = min, max

	case strings.Contains(s, "-"):
		parts := strings.SplitN(s, "-", 2)
		var err1, err2 error
		lo, err1 = strconv.Atoi(parts[0])
		hi, err2 = strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil {
			return fmt.Errorf("invalid range %q", s+"-"+parts[1])
		}
		if lo < min || hi > max || lo > hi {
			return fmt.Errorf("range %d-%d out of bounds [%d, %d]", lo, hi, min, max)
		}

	default:
		v, err := strconv.Atoi(s)
		if err != nil {
			return fmt.Errorf("invalid value %q", s)
		}
		if v < min || v > max {
			return fmt.Errorf("value %d out of bounds [%d, %d]", v, min, max)
		}
		lo, hi = v, v
	}

	for i := lo; i <= hi; i += step {
		values[i] = true
	}
	return nil
}

// ─── CronJob / CronStore ──────────────────────────────────────────────────────

// CronJob represents a scheduled job.
type CronJob struct {
	ID          string     `json:"id"`
	Cron        string     `json:"cron"`
	Prompt      string     `json:"prompt"`
	Recurring   bool       `json:"recurring"`
	Durable     bool       `json:"durable"`
	Permanent   bool       `json:"permanent,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	LastFiredAt *time.Time `json:"last_fired_at,omitempty"`
	AgentID     string     `json:"-"`
}

type persistedCronJob struct {
	ID          string `json:"id"`
	Cron        string `json:"cron"`
	Prompt      string `json:"prompt"`
	CreatedAt   int64  `json:"createdAt"`
	LastFiredAt *int64 `json:"lastFiredAt,omitempty"`
	Recurring   bool   `json:"recurring,omitempty"`
	Permanent   bool   `json:"permanent,omitempty"`
}

type cronFile struct {
	Tasks []persistedCronJob `json:"tasks"`
}

type CronStore struct {
	controlMu        sync.Mutex
	mu               sync.Mutex
	projectRoot      string
	scope            *RuntimeScope
	lifecycle        *RuntimeLifecycle
	sessionJobs      map[string]*CronJob
	sessionOrder     []string
	sessionSchedules map[string]*CronSchedule
	cancelFn         context.CancelFunc
	running          bool
	schedulerDone    chan struct{}

	// inFlight tracks jobs currently being delivered. Guards against
	// double-fire when the scheduler wakes spuriously within the same
	// tick window (CR-03).
	inFlight   map[string]bool
	inFlightMu sync.Mutex

	// schedLock implements the multi-session leader lock. Only the
	// holder dispatches durable jobs; loser sessions skip the durable
	// portion of collectDueJobs entirely (CR-01).
	schedLock *SchedulerLock
	probeStop chan struct{}
	probeDone chan struct{}

	// fileWatchStop terminates the file-watch poll loop (CR-03).
	fileWatchStop chan struct{}
	fileWatchDone chan struct{}
	fileWatchMod  time.Time

	// onMissedRun is invoked when the scheduler detects a recurring job
	// that should have fired while offline (CR-02). The default formatter
	// is BuildMissedTaskNotification.
	onMissedRun func([]MissedRun)

	// timezone overrides time.Local for cron schedule matching (CR-07).
	tz *time.Location

	// Tunables sourced from feature config (CR-06). When nil, fall back
	// to the package defaults.
	maxAgeFn func() time.Duration

	// nextFireAt stores the scheduler's per-task next-fire timestamps.
	// It mirrors the TS scheduler: first sight is anchored from
	// lastFiredAt ?? createdAt for recurring jobs and createdAt for
	// one-shots, with deterministic jitter applied.
	nextFireMu sync.Mutex
	nextFireAt map[string]time.Time
}

func NewCronStore(projectRoot string, scope *RuntimeScope) *CronStore {
	s := &CronStore{
		projectRoot:      projectRoot,
		scope:            scope,
		sessionJobs:      make(map[string]*CronJob),
		sessionOrder:     make([]string, 0),
		sessionSchedules: make(map[string]*CronSchedule),
		inFlight:         make(map[string]bool),
		nextFireAt:       make(map[string]time.Time),
	}
	if strings.TrimSpace(projectRoot) != "" {
		s.lifecycle = NewRuntimeLifecycle(projectRoot)
	}
	s.schedLock = NewSchedulerLock(s.cronFilePath() + ".scheduler.lock")
	return s
}

// SetMissedRunHandler installs a callback invoked with the list of missed
// runs detected on Start (CR-02). When nil, missed runs are still removed from
// durable storage but no presentation is attempted. Product surfaces must
// install a handler instead of bypassing their output owner.
func (s *CronStore) SetMissedRunHandler(fn func([]MissedRun)) {
	s.mu.Lock()
	s.onMissedRun = fn
	s.mu.Unlock()
}

// SetTimezone configures the IANA timezone used for cron schedule matching
// (CR-07). Default is time.Local.
func (s *CronStore) SetTimezone(loc *time.Location) {
	s.mu.Lock()
	s.tz = loc
	s.mu.Unlock()
}

// SetMaxAgeProvider injects a tunable max-age callback (CR-06) so ops can
// adjust recurringJobMaxAge from feature config without a release.
func (s *CronStore) SetMaxAgeProvider(fn func() time.Duration) {
	s.mu.Lock()
	s.maxAgeFn = fn
	s.mu.Unlock()
}

// effectiveMaxAge returns the active recurring-job max age. Falls back to
// the package default when no provider is wired.
func (s *CronStore) effectiveMaxAge() time.Duration {
	s.mu.Lock()
	fn := s.maxAgeFn
	s.mu.Unlock()
	if fn != nil {
		if v := fn(); v > 0 {
			return v
		}
	}
	return recurringJobMaxAge
}

// effectiveTZ returns the timezone used for cron matching. Defaults to
// time.Local.
func (s *CronStore) effectiveTZ() *time.Location {
	s.mu.Lock()
	loc := s.tz
	s.mu.Unlock()
	if loc != nil {
		return loc
	}
	return time.Local
}

// IsRecurringTaskAged reports whether a recurring job has aged past the
// configured max age and is on its final fire (CR-06 — pulled out as a
// separate predicate).
func (s *CronStore) IsRecurringTaskAged(job *CronJob, now time.Time) bool {
	return isRecurringTaskAgedWithMaxAge(job, now, s.effectiveMaxAge())
}

func isRecurringTaskAgedWithMaxAge(job *CronJob, now time.Time, maxAge time.Duration) bool {
	if job == nil || !job.Recurring {
		return false
	}
	if job.Permanent {
		return false
	}
	return now.Sub(job.CreatedAt) > maxAge
}

func (s *CronStore) Start(callback func(job *CronJob)) {
	s.controlMu.Lock()
	defer s.controlMu.Unlock()

	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	ctx, cancel := context.WithCancel(context.Background())
	s.cancelFn = cancel
	done := make(chan struct{})
	s.schedulerDone = done
	s.mu.Unlock()

	// CR-01: take leader lock or run the probe loop. Only the leader
	// fires durable jobs; loser sessions still keep the scheduler
	// running for their own session-scoped (non-durable) jobs.
	s.startLeaderProbe()

	// CR-02: surface missed recurring runs at boot.
	s.detectMissedRuns()

	// CR-03: poll the cron file for cross-process edits and reload.
	s.startFileWatcher()

	go func() {
		defer close(done)
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		s.runScheduler(ctx, callback, ticker.C, nil)
	}()
}

func (s *CronStore) startWith(callback func(job *CronJob), tickCh <-chan time.Time) {
	s.startWithCompletion(callback, tickCh, nil)
}

// startWithCompletion starts the scheduler with an injected tick source and
// reports each tick only after collection, persistence, lifecycle publication,
// and callbacks have all completed. It is the deterministic synchronization
// seam used by scheduler acceptance tests; production Start uses a real ticker.
func (s *CronStore) startWithCompletion(callback func(job *CronJob), tickCh <-chan time.Time, tickDone chan<- time.Time) {
	s.controlMu.Lock()
	defer s.controlMu.Unlock()

	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	ctx, cancel := context.WithCancel(context.Background())
	s.cancelFn = cancel
	done := make(chan struct{})
	s.schedulerDone = done
	s.mu.Unlock()

	go func() {
		defer close(done)
		s.runScheduler(ctx, callback, tickCh, tickDone)
	}()
}

func (s *CronStore) Stop() {
	s.controlMu.Lock()
	defer s.controlMu.Unlock()

	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	if s.cancelFn != nil {
		s.cancelFn()
		s.cancelFn = nil
	}
	probeStop := s.probeStop
	probeDone := s.probeDone
	fileWatchStop := s.fileWatchStop
	fileWatchDone := s.fileWatchDone
	schedulerDone := s.schedulerDone
	s.probeStop = nil
	s.probeDone = nil
	s.fileWatchStop = nil
	s.fileWatchDone = nil
	s.schedulerDone = nil
	lock := s.schedLock
	s.mu.Unlock()

	// Stop all producers first, then join them before releasing shared state or
	// returning. In particular, collectDueJobs may still be persisting a
	// lifecycle event when cancellation wins the next scheduler select.
	if probeStop != nil {
		close(probeStop)
	}
	if fileWatchStop != nil {
		close(fileWatchStop)
	}
	if schedulerDone != nil {
		<-schedulerDone
	}
	if probeDone != nil {
		<-probeDone
	}
	if fileWatchDone != nil {
		<-fileWatchDone
	}
	if lock != nil {
		lock.Release()
	}
}

func (s *CronStore) runScheduler(ctx context.Context, callback func(job *CronJob), tickCh <-chan time.Time, tickDone chan<- time.Time) {
	for {
		select {
		case <-ctx.Done():
			return
		case t, ok := <-tickCh:
			if !ok {
				return
			}
			toFire := s.collectDueJobs(t)
			for _, job := range toFire {
				callback(job)
			}
			if tickDone != nil {
				select {
				case tickDone <- t:
				case <-ctx.Done():
					return
				}
			}
		}
	}
}

func (s *CronStore) cronFilePath() string {
	if s.scope != nil {
		return s.scope.CronFilePath(s.projectRoot)
	}
	return filepath.Join(s.projectRoot, ".claude", "scheduled_tasks.json")
}

func (s *CronStore) lockPath() string {
	return s.cronFilePath() + ".lock"
}

func (s *CronStore) SetProjectRoot(root string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(root) == "" {
		return
	}
	s.projectRoot = filepath.Clean(root)
	s.lifecycle = NewRuntimeLifecycle(s.projectRoot)
}

func newCronID() string {
	var raw [4]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("%08x", time.Now().UnixNano())
	}
	return hex.EncodeToString(raw[:])
}

func cloneCronJob(job *CronJob) *CronJob {
	if job == nil {
		return nil
	}
	cp := *job
	if job.LastFiredAt != nil {
		t := *job.LastFiredAt
		cp.LastFiredAt = &t
	}
	return &cp
}

func (s *CronStore) cachedNextFire(id string) (time.Time, bool) {
	s.nextFireMu.Lock()
	defer s.nextFireMu.Unlock()
	next, ok := s.nextFireAt[id]
	return next, ok
}

func (s *CronStore) cacheNextFire(id string, next time.Time) {
	s.nextFireMu.Lock()
	s.nextFireAt[id] = next
	s.nextFireMu.Unlock()
}

func (s *CronStore) removeCachedNextFire(id string) {
	s.nextFireMu.Lock()
	delete(s.nextFireAt, id)
	s.nextFireMu.Unlock()
}

// recurringJobMaxAge is the duration after which a recurring job auto-expires.
// On its first fire after this age, the job is fired one final time and
// then removed from the store. Mirrors the TS scheduler.
const recurringJobMaxAge = 7 * 24 * time.Hour

func nextCronRun(expr string, from time.Time) (time.Time, bool) {
	sched, err := ParseCron(expr)
	if err != nil {
		return time.Time{}, false
	}
	candidate := from.Truncate(time.Minute).Add(time.Minute)
	deadline := candidate.Add(366 * 24 * time.Hour)
	for !candidate.After(deadline) {
		if sched.Matches(candidate) {
			return candidate, true
		}
		candidate = candidate.Add(time.Minute)
	}
	return time.Time{}, false
}

func (s *CronStore) readDurableJobsLocked() ([]*CronJob, map[string]*CronSchedule, error) {
	data, err := os.ReadFile(s.cronFilePath())
	if err != nil {
		if os.IsNotExist(err) || os.IsPermission(err) {
			return nil, map[string]*CronSchedule{}, nil
		}
		return nil, map[string]*CronSchedule{}, nil
	}
	var body cronFile
	if err := json.Unmarshal(data, &body); err != nil {
		return nil, map[string]*CronSchedule{}, nil
	}

	jobs := make([]*CronJob, 0, len(body.Tasks))
	schedules := make(map[string]*CronSchedule, len(body.Tasks))
	for _, item := range body.Tasks {
		if strings.TrimSpace(item.ID) == "" ||
			strings.TrimSpace(item.Cron) == "" ||
			strings.TrimSpace(item.Prompt) == "" ||
			item.CreatedAt <= 0 {
			continue
		}
		sched, err := ParseCron(item.Cron)
		if err != nil {
			continue
		}
		createdAt := time.UnixMilli(item.CreatedAt).UTC()
		job := &CronJob{
			ID:        item.ID,
			Cron:      item.Cron,
			Prompt:    item.Prompt,
			Recurring: item.Recurring,
			Durable:   true,
			Permanent: item.Permanent,
			CreatedAt: createdAt,
		}
		if item.LastFiredAt != nil {
			last := time.UnixMilli(*item.LastFiredAt).UTC()
			job.LastFiredAt = &last
		}
		jobs = append(jobs, job)
		schedules[job.ID] = sched
	}
	return jobs, schedules, nil
}

func (s *CronStore) writeDurableJobsLocked(jobs []*CronJob) error {
	if err := os.MkdirAll(filepath.Dir(s.cronFilePath()), 0755); err != nil {
		return err
	}
	body := cronFile{Tasks: make([]persistedCronJob, 0, len(jobs))}
	for _, job := range jobs {
		item := persistedCronJob{
			ID:        job.ID,
			Cron:      job.Cron,
			Prompt:    job.Prompt,
			CreatedAt: job.CreatedAt.UTC().UnixMilli(),
			Recurring: job.Recurring,
			Permanent: job.Permanent,
		}
		if job.LastFiredAt != nil {
			last := job.LastFiredAt.UTC().UnixMilli()
			item.LastFiredAt = &last
		}
		body.Tasks = append(body.Tasks, item)
	}
	data, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return atomicWriteFile(s.cronFilePath(), data, 0644)
}

func (s *CronStore) sessionCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sessionJobs)
}

func (s *CronStore) create(cron, prompt string, recurring, durable bool, sched *CronSchedule) (string, error) {
	return s.createForAgent(cron, prompt, recurring, durable, sched, s.currentAgentID())
}

func (s *CronStore) createForAgent(cron, prompt string, recurring, durable bool, sched *CronSchedule, agentID string) (string, error) {
	createdAt := time.Now().UTC()
	id := newCronID()
	job := &CronJob{
		ID:        id,
		Cron:      cron,
		Prompt:    prompt,
		Recurring: recurring,
		Durable:   durable,
		CreatedAt: createdAt,
	}
	if durable {
		_, err := withRuntimeFileLockResult(s.lockPath(), func() (any, error) {
			jobs, _, err := s.readDurableJobsLocked()
			if err != nil {
				return nil, err
			}
			if len(jobs)+s.sessionCount() >= 50 {
				return nil, errCronTooManyJobs
			}
			jobs = append(jobs, job)
			return nil, s.writeDurableJobsLocked(jobs)
		})
		if err != nil {
			return "", err
		}
		if next, ok := cronNextFireAt(job, createdAt, s.effectiveTZ()); ok {
			s.cacheNextFire(id, next)
		}
		return id, nil
	}

	job.AgentID = strings.TrimSpace(agentID)
	_, err := withRuntimeFileLockResult(s.lockPath(), func() (any, error) {
		jobs, _, err := s.readDurableJobsLocked()
		if err != nil {
			return nil, err
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if len(s.sessionJobs)+len(jobs) >= 50 {
			return nil, errCronTooManyJobs
		}
		s.sessionJobs[id] = job
		s.sessionOrder = append(s.sessionOrder, id)
		if sched != nil {
			s.sessionSchedules[id] = sched
		}
		return nil, nil
	})
	if err != nil {
		return "", err
	}
	if next, ok := cronNextFireAt(job, createdAt, s.effectiveTZ()); ok {
		s.cacheNextFire(id, next)
	}
	return id, nil
}

func (s *CronStore) removeSessionOrderLocked(id string) {
	for i, existing := range s.sessionOrder {
		if existing == id {
			copy(s.sessionOrder[i:], s.sessionOrder[i+1:])
			s.sessionOrder = s.sessionOrder[:len(s.sessionOrder)-1]
			return
		}
	}
}

func (s *CronStore) currentAgentID() string {
	if s != nil && s.scope != nil {
		if agentID := s.scope.AgentID(); agentID != "" {
			return agentID
		}
	}
	return strings.TrimSpace(os.Getenv("CLAUDE_CODE_AGENT_ID"))
}

func cronPeriod(expr string, from time.Time, loc *time.Location) (time.Duration, bool) {
	first, ok := nextCronRunInLocation(expr, from, loc)
	if !ok {
		return 0, false
	}
	second, ok := nextCronRunInLocation(expr, first, loc)
	if !ok {
		return 0, false
	}
	return second.Sub(first), true
}

func cronNextFireAt(job *CronJob, now time.Time, loc *time.Location) (time.Time, bool) {
	if job == nil {
		return time.Time{}, false
	}
	anchor := job.CreatedAt
	if job.Recurring && job.LastFiredAt != nil {
		anchor = *job.LastFiredAt
	}
	_ = now
	next, ok := nextCronRunInLocation(job.Cron, anchor, loc)
	if !ok {
		return time.Time{}, false
	}
	if job.Recurring {
		if period, ok := cronPeriod(job.Cron, anchor, loc); ok {
			next = next.Add(RecurringJitter(period, job.ID))
		}
	} else {
		jittered := next.Add(OneshotJitter(next, job.ID))
		if jittered.Before(job.CreatedAt) {
			jittered = job.CreatedAt
		}
		next = jittered
	}
	return next, true
}

func cronTaskMissedOneShot(job *CronJob, now time.Time, loc *time.Location) bool {
	if job == nil || job.Recurring {
		return false
	}
	next, ok := nextCronRunInLocation(job.Cron, job.CreatedAt, loc)
	return ok && next.Before(now)
}

type cronDeleteState int

const (
	cronDeleteMissing cronDeleteState = iota
	cronDeleteDeleted
	cronDeleteOwnerDenied
)

type cronDeleteResult struct {
	state cronDeleteState
	err   error
}

func (s *CronStore) delete(id string) cronDeleteResult {
	return s.deleteForAgent(id, s.currentAgentID())
}

func (s *CronStore) deleteForAgent(id, agentID string) cronDeleteResult {
	currentAgentID := strings.TrimSpace(agentID)
	s.mu.Lock()
	if job, ok := s.sessionJobs[id]; ok {
		if currentAgentID != "" && job.AgentID != currentAgentID {
			s.mu.Unlock()
			return cronDeleteResult{state: cronDeleteOwnerDenied}
		}
		delete(s.sessionJobs, id)
		delete(s.sessionSchedules, id)
		s.removeCachedNextFire(id)
		s.removeSessionOrderLocked(id)
		s.mu.Unlock()
		return cronDeleteResult{state: cronDeleteDeleted}
	}
	s.mu.Unlock()

	value, err := withRuntimeFileLockResult(s.lockPath(), func() (any, error) {
		jobs, _, err := s.readDurableJobsForDeleteLocked()
		if err != nil {
			return cronDeleteMissing, err
		}
		filtered := jobs[:0]
		state := cronDeleteMissing
		for _, job := range jobs {
			if job.ID == id {
				if currentAgentID != "" && job.AgentID != currentAgentID {
					state = cronDeleteOwnerDenied
					filtered = append(filtered, job)
					continue
				}
				state = cronDeleteDeleted
				continue
			}
			filtered = append(filtered, job)
		}
		if state != cronDeleteDeleted {
			return state, nil
		}
		return state, s.writeDurableJobsLocked(filtered)
	})
	if err != nil {
		return cronDeleteResult{state: cronDeleteMissing, err: err}
	}
	state, ok := value.(cronDeleteState)
	if !ok {
		return cronDeleteResult{state: cronDeleteMissing}
	}
	if state == cronDeleteDeleted {
		s.removeCachedNextFire(id)
	}
	return cronDeleteResult{state: state}
}

func (s *CronStore) readDurableJobsForDeleteLocked() ([]*CronJob, map[string]*CronSchedule, error) {
	data, err := os.ReadFile(s.cronFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, map[string]*CronSchedule{}, nil
		}
		return nil, map[string]*CronSchedule{}, err
	}
	var body cronFile
	if err := json.Unmarshal(data, &body); err != nil {
		return nil, map[string]*CronSchedule{}, err
	}

	jobs := make([]*CronJob, 0, len(body.Tasks))
	schedules := make(map[string]*CronSchedule, len(body.Tasks))
	for _, item := range body.Tasks {
		if strings.TrimSpace(item.ID) == "" ||
			strings.TrimSpace(item.Cron) == "" ||
			strings.TrimSpace(item.Prompt) == "" ||
			item.CreatedAt <= 0 {
			continue
		}
		sched, err := ParseCron(item.Cron)
		if err != nil {
			continue
		}
		createdAt := time.UnixMilli(item.CreatedAt).UTC()
		job := &CronJob{
			ID:        item.ID,
			Cron:      item.Cron,
			Prompt:    item.Prompt,
			Recurring: item.Recurring,
			Durable:   true,
			Permanent: item.Permanent,
			CreatedAt: createdAt,
		}
		if item.LastFiredAt != nil {
			last := time.UnixMilli(*item.LastFiredAt).UTC()
			job.LastFiredAt = &last
		}
		jobs = append(jobs, job)
		schedules[job.ID] = sched
	}
	return jobs, schedules, nil
}

func (s *CronStore) list() []*CronJob {
	items := make([]*CronJob, 0)
	if value, err := withRuntimeFileLockResult(s.lockPath(), func() (any, error) {
		jobs, _, err := s.readDurableJobsLocked()
		return jobs, err
	}); err == nil {
		if durable, ok := value.([]*CronJob); ok {
			items = append(items, durable...)
		}
	}

	s.mu.Lock()
	for _, id := range s.sessionOrder {
		if job, ok := s.sessionJobs[id]; ok {
			items = append(items, cloneCronJob(job))
		}
	}
	s.mu.Unlock()
	return items
}

func (s *CronStore) collectDueJobs(minute time.Time) []*CronJob {
	if cronDisabledByFeatureFlag() {
		return nil
	}
	var toFire []*CronJob
	maxAge := s.effectiveMaxAge()

	// Convert tick time to the configured timezone for matching (CR-07).
	tz := s.effectiveTZ()

	s.mu.Lock()
	for id, job := range s.sessionJobs {
		next, ok := s.cachedNextFire(id)
		if !ok {
			next, ok = cronNextFireAt(job, minute, tz)
			if ok {
				s.cacheNextFire(id, next)
			}
		}
		if !ok || minute.Before(next) {
			continue
		}
		if job.LastFiredAt != nil && job.LastFiredAt.Equal(minute) {
			continue
		}
		if !IdleGateConsult(context.Background(), id) {
			continue
		}
		// CR-03: inFlight guard prevents double-fire if the scheduler
		// wakes spuriously within the same tick window.
		if !s.acquireInFlight(id, minute) {
			continue
		}
		firedAt := minute
		job.LastFiredAt = &firedAt
		toFire = append(toFire, cloneCronJob(job))
		// Recurring jobs auto-expire after recurringJobMaxAge: this is
		// their last fire; remove them. One-shot jobs always remove.
		expired := isRecurringTaskAgedWithMaxAge(job, minute, maxAge)
		if !job.Recurring || expired {
			delete(s.sessionJobs, id)
			delete(s.sessionSchedules, id)
			s.removeCachedNextFire(id)
			s.removeSessionOrderLocked(id)
		} else if next, ok := cronNextFireAt(job, minute, tz); ok {
			s.cacheNextFire(id, next)
		}
	}
	s.mu.Unlock()

	// CR-01: durable jobs only fire on the leader. Loser sessions skip
	// the durable section entirely so multi-session deployments do not
	// multiply side-effects.
	leader := false
	if s.schedLock != nil {
		leader = s.schedLock.IsHolder()
	} else {
		leader = true
	}
	if !leader {
		s.publishCronFires(minute, toFire)
		return toFire
	}

	_, err := withRuntimeFileLockResult(s.lockPath(), func() (any, error) {
		jobs, schedules, err := s.readDurableJobsLocked()
		if err != nil {
			return nil, err
		}
		mutated := false
		filtered := jobs[:0]
		for _, job := range jobs {
			_ = schedules
			next, ok := s.cachedNextFire(job.ID)
			if !ok {
				next, ok = cronNextFireAt(job, minute, tz)
				if ok {
					s.cacheNextFire(job.ID, next)
				}
			}
			if !ok || minute.Before(next) {
				filtered = append(filtered, job)
				continue
			}
			if job.LastFiredAt != nil && job.LastFiredAt.Equal(minute) {
				filtered = append(filtered, job)
				continue
			}
			if !IdleGateConsult(context.Background(), job.ID) {
				filtered = append(filtered, job)
				continue
			}
			if !s.acquireInFlight(job.ID, minute) {
				filtered = append(filtered, job)
				continue
			}
			firedAt := minute
			job.LastFiredAt = &firedAt
			toFire = append(toFire, cloneCronJob(job))
			expired := isRecurringTaskAgedWithMaxAge(job, minute, maxAge)
			switch {
			case !job.Recurring:
				// One-shot: drop after firing.
				mutated = true
				s.removeCachedNextFire(job.ID)
			case expired:
				// Recurring but past 7-day max: this was the last fire.
				mutated = true
				s.removeCachedNextFire(job.ID)
			default:
				filtered = append(filtered, job)
				mutated = true // LastFiredAt changed, persist.
				if next, ok := cronNextFireAt(job, minute, tz); ok {
					s.cacheNextFire(job.ID, next)
				}
			}
		}
		if mutated {
			if err := s.writeDurableJobsLocked(filtered); err != nil {
				return nil, err
			}
		}
		return nil, nil
	})
	_ = err
	s.publishCronFires(minute, toFire)
	return toFire
}

func (s *CronStore) publishCronFires(firedAt time.Time, jobs []*CronJob) {
	if len(jobs) == 0 {
		return
	}
	s.mu.Lock()
	lifecycle := s.lifecycle
	s.mu.Unlock()
	if lifecycle == nil {
		return
	}
	for _, job := range jobs {
		if job == nil {
			continue
		}
		_ = lifecycle.Publish(context.Background(), RuntimeLifecycleEvent{
			Type:      LifecycleCronFire,
			EntityID:  job.ID,
			ToolName:  "CronCreate",
			Status:    "fired",
			CreatedAt: firedAt,
			Payload: map[string]any{
				"cron":      job.Cron,
				"prompt":    job.Prompt,
				"recurring": job.Recurring,
				"durable":   job.Durable,
			},
		})
	}
}

// acquireInFlight registers (jobID, tick) in the inFlight map; returns true
// if no other call has claimed this tick yet. The set is keyed by
// "id|unix-minute" so two distinct ticks for the same job both succeed.
func (s *CronStore) acquireInFlight(id string, tick time.Time) bool {
	key := id + "|" + strconv.FormatInt(tick.Unix(), 10)
	s.inFlightMu.Lock()
	defer s.inFlightMu.Unlock()
	if s.inFlight[key] {
		return false
	}
	s.inFlight[key] = true
	// Self-evict after 5 minutes so the map can't grow unbounded.
	go func() {
		time.Sleep(5 * time.Minute)
		s.inFlightMu.Lock()
		delete(s.inFlight, key)
		s.inFlightMu.Unlock()
	}()
	return true
}

// CronErrorCode is the structured error code surfaced for cron failures.
// Mirrors the TS scheduler's errorCode contract so UIs can pattern-match
// regardless of the underlying message text.
type CronErrorCode int

const (
	CronErrCodeInvalidSyntax   CronErrorCode = 1
	CronErrCodeNoFutureMatch   CronErrorCode = 2
	CronErrCodeTooManyJobs     CronErrorCode = 3
	CronErrCodeDurableTeammate CronErrorCode = 4
)

const (
	CronErrCodeDeleteNotFound    CronErrorCode = 1
	CronErrCodeDeleteOwnerDenied CronErrorCode = 2
)

// guardDurableCreation enforces that durable jobs are only accepted in a
// session that's eligible to drive them. RuntimeScope identifies in-process
// teammates; the environment marker covers process-based teammate sessions.
// Returns nil when creation is allowed.
func (s *CronStore) guardDurableCreation() error {
	return s.guardDurableCreationForAgent(s.currentAgentID())
}

func (s *CronStore) guardDurableCreationForAgent(agentID string) error {
	if strings.TrimSpace(agentID) != "" {
		return i18n.NewError(i18n.KeyToolLegacyACronDurableLeaderOnly, CronErrCodeDurableTeammate)
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("CLAUDE_CODE_CRON_NON_DURABLE_TEAMMATE"))) {
	case "1", "true", "yes", "on":
		return i18n.NewError(i18n.KeyToolLegacyACronDurableLeaderOnly, CronErrCodeDurableTeammate)
	}
	return nil
}

// validateCron checks that expr is a 5-field cron expression (field-count only).
func validateCron(expr string) error {
	_, err := ParseCron(expr)
	return err
}

// startLeaderProbe runs a goroutine that tries to acquire the scheduler
// lock at boot and re-tries every probeInterval. The leader holds the lock
// indefinitely until Stop. Loser sessions take over within staleTimeout
// of the leader exiting (CR-01).
func (s *CronStore) startLeaderProbe() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	lock := s.schedLock
	if lock == nil {
		s.mu.Unlock()
		return
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	s.probeStop = stop
	s.probeDone = done
	s.mu.Unlock()

	go func() {
		defer close(done)
		// First-shot: try immediately.
		_, _ = lock.TryAcquire()
		ticker := time.NewTicker(lock.ProbeInterval())
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				if !lock.IsHolder() {
					_, _ = lock.TryAcquire()
				}
			}
		}
	}()
}

// detectMissedRuns scans the durable cron file for recurring jobs whose
// `LastFiredAt + interval < now` and surfaces a startup notification (CR-02).
// The notification is delivered via the configured handler when set. With no
// handler it remains silent rather than bypassing the active presentation
// owner through process stderr.
func (s *CronStore) detectMissedRuns() {
	jobs, _, err := s.snapshotDurable()
	if err != nil || len(jobs) == 0 {
		return
	}
	now := time.Now()
	tz := s.effectiveTZ()
	missedOneShot := make([]*CronJob, 0)
	for _, job := range jobs {
		if cronTaskMissedOneShot(job, now, tz) {
			missedOneShot = append(missedOneShot, job)
		}
	}
	if len(missedOneShot) > 0 {
		ids := make([]string, 0, len(missedOneShot))
		missed := make([]MissedRun, 0, len(missedOneShot))
		for _, job := range missedOneShot {
			ids = append(ids, job.ID)
			next, _ := cronNextFireAt(job, now, tz)
			missed = append(missed, MissedRun{
				JobID:       job.ID,
				Cron:        job.Cron,
				Prompt:      job.Prompt,
				CreatedAt:   job.CreatedAt,
				MissedSince: next,
				MissedCount: 1,
			})
		}
		_, _ = withRuntimeFileLockResult(s.lockPath(), func() (any, error) {
			current, _, err := s.readDurableJobsLocked()
			if err != nil {
				return nil, err
			}
			idSet := make(map[string]bool, len(ids))
			for _, id := range ids {
				idSet[id] = true
			}
			filtered := current[:0]
			for _, job := range current {
				if !idSet[job.ID] {
					filtered = append(filtered, job)
				}
			}
			return nil, s.writeDurableJobsLocked(filtered)
		})
		s.mu.Lock()
		handler := s.onMissedRun
		s.mu.Unlock()
		if handler != nil {
			handler(missed)
		}
	}

	// TS no longer surfaces recurring jobs as missed on startup; the
	// normal next-fire path handles them from createdAt/lastFiredAt.
}

// snapshotDurable returns the on-disk job list under the file lock.
func (s *CronStore) snapshotDurable() ([]*CronJob, map[string]*CronSchedule, error) {
	value, err := withRuntimeFileLockResult(s.lockPath(), func() (any, error) {
		jobs, schedules, err := s.readDurableJobsLocked()
		if err != nil {
			return nil, err
		}
		return [2]any{jobs, schedules}, nil
	})
	if err != nil {
		return nil, nil, err
	}
	pair, ok := value.([2]any)
	if !ok {
		return nil, map[string]*CronSchedule{}, nil
	}
	jobs, _ := pair[0].([]*CronJob)
	schedules, _ := pair[1].(map[string]*CronSchedule)
	return jobs, schedules, nil
}

// startFileWatcher polls the cron file for cross-process edits (CR-03).
// fsnotify is unavailable so we fall back to a 2s mtime poll.
func (s *CronStore) startFileWatcher() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	s.fileWatchStop = stop
	s.fileWatchDone = done
	if info, err := os.Stat(s.cronFilePath()); err == nil {
		s.fileWatchMod = info.ModTime()
	}
	s.mu.Unlock()

	go func() {
		defer close(done)
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				info, err := os.Stat(s.cronFilePath())
				if err != nil {
					continue
				}
				s.mu.Lock()
				prev := s.fileWatchMod
				s.fileWatchMod = info.ModTime()
				s.mu.Unlock()
				if !prev.IsZero() && info.ModTime().After(prev) {
					// External edit. The next collectDueJobs tick will
					// re-read the file under the lock — no in-memory
					// state needs invalidation here, but signal a watch
					// event for tests.
					_ = info
				}
			}
		}
	}()
}

// nextCronRunInLocation is like nextCronRun but evaluates the schedule in
// the supplied timezone (CR-07). DST transitions are handled implicitly
// because the candidate time is converted to the location before matching.
func nextCronRunInLocation(expr string, from time.Time, loc *time.Location) (time.Time, bool) {
	if loc == nil {
		loc = time.Local
	}
	sched, err := ParseCron(expr)
	if err != nil {
		return time.Time{}, false
	}
	candidate := from.In(loc).Truncate(time.Minute).Add(time.Minute)
	deadline := candidate.Add(366 * 24 * time.Hour)
	for !candidate.After(deadline) {
		if sched.Matches(candidate) {
			return candidate, true
		}
		candidate = candidate.Add(time.Minute)
	}
	return time.Time{}, false
}

// cronDisabledByFeatureFlag reports whether the CLAUDE_CODE_DISABLE_CRON
// kill-switch is set. Mirrors the TS `tengu_*` feature flag wiring.
func cronDisabledByFeatureFlag() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("CLAUDE_CODE_DISABLE_CRON"))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// cronFeatureFlagRefusal returns a refusal ToolResult when cron is disabled
// by feature flag.
func cronFeatureFlagRefusal() types.ToolResult {
	return types.ToolResult{
		Content: toolRuntimeText(i18n.KeyToolRuntimeCronDisabled),
		IsError: true,
	}
}

func durableCronEnabled() bool {
	for _, name := range []string{
		"CLAUDE_CODE_DISABLE_DURABLE_CRON",
		"CLAUDE_CODE_DISABLE_CRON_DURABLE",
	} {
		switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
		case "1", "true", "yes", "on":
			return false
		}
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("TENGU_KAIROS_CRON_DURABLE"))) {
	case "0", "false", "no", "off", "disabled":
		return false
	}
	return true
}

type cronCreateOutput struct {
	ID            string `json:"id"`
	HumanSchedule string `json:"humanSchedule"`
	Recurring     bool   `json:"recurring"`
	Durable       bool   `json:"durable"`
}

type cronDeleteOutput struct {
	ID string `json:"id"`
}

type cronListOutput struct {
	Jobs []cronListJobOutput `json:"jobs"`
}

type cronListJobOutput struct {
	ID            string `json:"id"`
	Cron          string `json:"cron"`
	HumanSchedule string `json:"humanSchedule"`
	Prompt        string `json:"prompt"`
	Recurring     *bool  `json:"recurring,omitempty"`
	Durable       *bool  `json:"durable,omitempty"`
}

func cronErrorResult(message string, code CronErrorCode) types.ToolResult {
	return types.ToolResult{
		Content: toolRuntimeFormat(i18n.KeyToolLegacyACronErrorCode, message, code),
		IsError: true,
		Metadata: map[string]string{
			"errorCode": strconv.Itoa(int(code)),
		},
	}
}

func parseSemanticBool(input map[string]any, key string) (*bool, *types.ToolResult) {
	raw, ok := input[key]
	if !ok || raw == nil {
		return nil, nil
	}
	switch v := raw.(type) {
	case bool:
		value := v
		return &value, nil
	case string:
		switch v {
		case "true":
			value := true
			return &value, nil
		case "false":
			value := false
			return &value, nil
		}
	}
	return nil, &types.ToolResult{
		Content: toolRuntimeFormat(i18n.KeyToolRuntimeCronBooleanRequired, key),
		IsError: true,
	}
}

func parseCronCreateInput(input map[string]any) (CronCreateInput, *types.ToolResult) {
	var in CronCreateInput
	allowed := map[string]bool{"cron": true, "prompt": true, "recurring": true, "durable": true}
	unknown := make([]string, 0)
	for key := range input {
		if !allowed[key] {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return in, &types.ToolResult{
			Content: toolRuntimeFormat(i18n.KeyToolRuntimeCronUnexpectedParameter, unknown[0]),
			IsError: true,
		}
	}
	if raw, ok := input["cron"]; ok {
		cron, ok := raw.(string)
		if !ok {
			return in, &types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolRuntimeFieldStringRequired, "cron"), IsError: true}
		}
		in.Cron = cron
	}
	if raw, ok := input["prompt"]; ok {
		prompt, ok := raw.(string)
		if !ok {
			return in, &types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolRuntimeFieldStringRequired, "prompt"), IsError: true}
		}
		in.Prompt = prompt
	}
	recurring, errResult := parseSemanticBool(input, "recurring")
	if errResult != nil {
		return in, errResult
	}
	durable, errResult := parseSemanticBool(input, "durable")
	if errResult != nil {
		return in, errResult
	}
	in.Recurring = recurring
	in.Durable = durable
	return in, nil
}

// ─── CronCreateTool ───────────────────────────────────────────────────────────

// CronCreateTool creates a new cron job in the shared CronStore.
type CronCreateTool struct {
	Store   *CronStore
	agentID string
}

func NewCronCreateTool(store *CronStore) *CronCreateTool { return &CronCreateTool{Store: store} }

func (t *CronCreateTool) withInProcessAgentID(agentID string) types.Tool {
	clone := *t
	clone.agentID = strings.TrimSpace(agentID)
	return &clone
}

func (t *CronCreateTool) currentAgentID() string {
	if t != nil && t.agentID != "" {
		return t.agentID
	}
	if t != nil && t.Store != nil {
		return t.Store.currentAgentID()
	}
	return ""
}

func (t *CronCreateTool) Name() string { return "CronCreate" }

func (t *CronCreateTool) Description() string {
	if durableCronEnabled() {
		return "Schedule a prompt to run at a future time — either recurring on a cron schedule, or once at a specific time. Pass durable: true to persist to .claude/scheduled_tasks.json; otherwise session-only."
	}
	return "Schedule a prompt to run at a future time within this Claude session — either recurring on a cron schedule, or once at a specific time."
}

func (t *CronCreateTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(
		map[string]any{
			"cron": map[string]any{
				"type":        "string",
				"description": "Standard 5-field cron expression in local time: M H DoM Mon DoW",
			},
			"prompt": map[string]any{
				"type":        "string",
				"description": "The prompt to enqueue at each fire time.",
			},
			"recurring": map[string]any{
				"type":        "boolean",
				"description": "true = repeating job (default), false = one-shot",
			},
			"durable": map[string]any{
				"type":        "boolean",
				"description": "Persist job to disk (default false)",
			},
		},
		"cron", "prompt",
	)
}

func (t *CronCreateTool) ToolContract() types.ToolContract {
	outputSchema := types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"id":            map[string]any{"type": "string"},
			"humanSchedule": map[string]any{"type": "string"},
			"recurring":     map[string]any{"type": "boolean"},
			"durable":       map[string]any{"type": "boolean"},
		},
		Required: []string{"id", "humanSchedule", "recurring"},
	}
	return types.ToolContract{
		OutputSchema:       &outputSchema,
		Strict:             true,
		MaxResultSizeChars: 100_000,
	}
}

func cronCreateContent(output cronCreateOutput) string {
	where := toolRuntimeText(i18n.KeyToolRuntimeCronSessionOnly)
	if output.Durable {
		where = toolRuntimeText(i18n.KeyToolRuntimeCronPersisted)
	}
	if output.Recurring {
		return toolRuntimeFormat(i18n.KeyToolRuntimeCronRecurringCreated, output.ID, output.HumanSchedule, where)
	}
	return toolRuntimeFormat(i18n.KeyToolRuntimeCronOneShotCreated, output.ID, output.HumanSchedule, where)
}

func (t *CronCreateTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	output, ok := data.(cronCreateOutput)
	if !ok {
		return types.ToolResultBlock{
			Type:      types.ContentTypeToolResult,
			ToolUseID: toolUseID,
			Content:   toolRuntimeText(i18n.KeyToolRuntimeCronCreateInvalidResult),
			IsError:   true,
		}
	}
	return types.ToolResultBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: toolUseID,
		Content:   cronCreateContent(output),
	}
}

func (t *CronCreateTool) Execute(_ context.Context, input map[string]any) (types.ToolResult, error) {
	if cronDisabledByFeatureFlag() {
		return cronFeatureFlagRefusal(), nil
	}
	in, toolErr := parseCronCreateInput(input)
	if toolErr != nil {
		return *toolErr, nil
	}

	if strings.TrimSpace(in.Cron) == "" {
		return cronErrorResult(toolRuntimeFormat(i18n.KeyToolRuntimeCronInvalidExpression, in.Cron), CronErrCodeInvalidSyntax), nil
	}

	// Parse and validate fully (fail-fast on invalid expressions).
	sched, err := ParseCron(in.Cron)
	if err != nil {
		return cronErrorResult(toolRuntimeFormat(i18n.KeyToolRuntimeCronInvalidExpression, in.Cron), CronErrCodeInvalidSyntax), nil
	}

	if strings.TrimSpace(in.Prompt) == "" {
		return types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolRuntimeRequiredFieldMissing, "prompt"), IsError: true}, nil
	}

	// Default recurring = true when omitted.
	recurring := true
	if in.Recurring != nil {
		recurring = *in.Recurring
	}
	durable := false
	if in.Durable != nil {
		durable = *in.Durable
	}

	effectiveDurable := durable && durableCronEnabled()
	currentAgentID := t.currentAgentID()

	// CR-04: durable jobs require a leader-eligible session. A non-leader
	// teammate that creates a durable job produces a silently
	// unfulfilled schedule because no scheduler will ever drive it.
	if effectiveDurable {
		if guardErr := t.Store.guardDurableCreationForAgent(currentAgentID); guardErr != nil {
			return types.ToolResult{
				Content: guardErr.Error(),
				IsError: true,
				Metadata: map[string]string{
					"errorCode": strconv.Itoa(int(CronErrCodeDurableTeammate)),
				},
			}, nil
		}
	}

	if _, ok := nextCronRunInLocation(in.Cron, time.Now(), t.Store.effectiveTZ()); !ok {
		return cronErrorResult(toolRuntimeFormat(i18n.KeyToolRuntimeCronNoFutureMatch, in.Cron), CronErrCodeNoFutureMatch), nil
	}

	// Validate any sentinel in the prompt up-front so users get an
	// immediate error rather than discovering it at first fire.
	if _, err := ResolvePrompt(in.Prompt); err != nil {
		return ErrorResponse(err), nil
	}

	id, err := t.Store.createForAgent(in.Cron, in.Prompt, recurring, effectiveDurable, sched, currentAgentID)
	if err != nil {
		if err == errCronTooManyJobs {
			return cronErrorResult(err.Error(), CronErrCodeTooManyJobs), nil
		}
		return ErrorResponse(err), nil
	}

	// Compute the actual next-fire (with jitter) so the response is
	// useful — users want to know when their job will run.
	// Stable metadata token; presentation resolves it in the active language.
	nextFireStr := "unknown"
	loc := t.Store.effectiveTZ()
	if next, ok := t.Store.cachedNextFire(id); ok {
		nextFireStr = next.In(loc).Format(time.RFC3339Nano)
	}

	// Surface the resolved timezone so users can verify scheduling.
	tzName := loc.String()
	if tzName == "" {
		tzName = "Local"
	}

	humanSchedule := cronToHuman(in.Cron)
	output := cronCreateOutput{
		ID:            id,
		HumanSchedule: humanSchedule,
		Recurring:     recurring,
		Durable:       effectiveDurable,
	}
	return types.ToolResult{
		Content: cronCreateContent(output),
		Data:    output,
		Metadata: map[string]string{
			"id":            id,
			"humanSchedule": humanSchedule,
			"recurring":     strconv.FormatBool(recurring),
			"durable":       strconv.FormatBool(effectiveDurable),
			"next_fire":     nextFireStr,
			"tz":            tzName,
		},
	}, nil
}

// ─── CronDeleteTool ───────────────────────────────────────────────────────────

// CronDeleteTool removes a cron job from the shared CronStore.
type CronDeleteTool struct {
	Store   *CronStore
	agentID string
}

func NewCronDeleteTool(store *CronStore) *CronDeleteTool { return &CronDeleteTool{Store: store} }

func (t *CronDeleteTool) withInProcessAgentID(agentID string) types.Tool {
	clone := *t
	clone.agentID = strings.TrimSpace(agentID)
	return &clone
}

func (t *CronDeleteTool) currentAgentID() string {
	if t != nil && t.agentID != "" {
		return t.agentID
	}
	if t != nil && t.Store != nil {
		return t.Store.currentAgentID()
	}
	return ""
}

func (t *CronDeleteTool) Name() string { return "CronDelete" }

func (t *CronDeleteTool) Description() string {
	return "Delete a scheduled cron job by ID."
}

func (t *CronDeleteTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(
		map[string]any{
			"id": map[string]any{
				"type":        "string",
				"description": "The cron job ID to delete",
			},
		},
		"id",
	)
}

func (t *CronDeleteTool) ToolContract() types.ToolContract {
	outputSchema := types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"id": map[string]any{"type": "string"},
		},
		Required: []string{"id"},
	}
	return types.ToolContract{
		OutputSchema:       &outputSchema,
		Strict:             true,
		MaxResultSizeChars: 100_000,
	}
}

func (t *CronDeleteTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	output, ok := data.(cronDeleteOutput)
	if !ok {
		return types.ToolResultBlock{
			Type:      types.ContentTypeToolResult,
			ToolUseID: toolUseID,
			Content:   fmt.Sprintf("%v", data),
		}
	}
	return types.ToolResultBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: toolUseID,
		Content:   toolRuntimeFormat(i18n.KeyToolRuntimeCronCancelled, output.ID),
	}
}

func (t *CronDeleteTool) Execute(_ context.Context, input map[string]any) (types.ToolResult, error) {
	if cronDisabledByFeatureFlag() {
		return cronFeatureFlagRefusal(), nil
	}
	in, err := types.DecodeStrictToolInput[CronDeleteInput](input)
	if err != nil {
		return types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolRuntimeInvalidInput, err), IsError: true}, nil
	}
	if strings.TrimSpace(in.ID) == "" {
		return types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolRuntimeRequiredFieldMissing, "id"), IsError: true}, nil
	}
	result := t.Store.deleteForAgent(in.ID, t.currentAgentID())
	if result.err != nil {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyACronDeleteFailed, in.ID, result.err)), nil
	}
	switch result.state {
	case cronDeleteDeleted:
		output := cronDeleteOutput{ID: in.ID}
		return types.ToolResult{
			Content: toolRuntimeFormat(i18n.KeyToolRuntimeCronCancelled, in.ID),
			Data:    output,
			Metadata: map[string]string{
				"id": in.ID,
			},
		}, nil
	case cronDeleteOwnerDenied:
		return cronErrorResult(toolRuntimeFormat(i18n.KeyToolRuntimeCronDeleteOwnedByOther, in.ID), CronErrCodeDeleteOwnerDenied), nil
	default:
		return cronErrorResult(toolRuntimeFormat(i18n.KeyToolRuntimeCronDeleteNotFound, in.ID), CronErrCodeDeleteNotFound), nil
	}
}

// ─── CronListTool ─────────────────────────────────────────────────────────────

// CronListTool lists all cron jobs in the shared CronStore.
type CronListTool struct {
	Store   *CronStore
	agentID string
}

func NewCronListTool(store *CronStore) *CronListTool { return &CronListTool{Store: store} }

func (t *CronListTool) withInProcessAgentID(agentID string) types.Tool {
	clone := *t
	clone.agentID = strings.TrimSpace(agentID)
	return &clone
}

func (t *CronListTool) currentAgentID() string {
	if t != nil && t.agentID != "" {
		return t.agentID
	}
	if t != nil && t.Store != nil {
		return t.Store.currentAgentID()
	}
	return ""
}

func (t *CronListTool) Name() string           { return "CronList" }
func (t *CronListTool) IsConcurrentSafe() bool { return true }
func (t *CronListTool) IsReadOnly() bool       { return true }

func (t *CronListTool) Description() string {
	if durableCronEnabled() {
		return "List scheduled cron jobs, both durable (.claude/scheduled_tasks.json) and session-only."
	}
	return "List scheduled cron jobs in this session."
}

func (t *CronListTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(map[string]any{})
}

func (t *CronListTool) ToolContract() types.ToolContract {
	outputSchema := types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"jobs": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":            map[string]any{"type": "string"},
						"cron":          map[string]any{"type": "string"},
						"humanSchedule": map[string]any{"type": "string"},
						"prompt":        map[string]any{"type": "string"},
						"recurring":     map[string]any{"type": "boolean"},
						"durable":       map[string]any{"type": "boolean"},
					},
					"required": []string{"id", "cron", "humanSchedule", "prompt"},
				},
			},
		},
		Required: []string{"jobs"},
	}
	return types.ToolContract{
		OutputSchema:       &outputSchema,
		Strict:             true,
		ReadOnly:           true,
		ConcurrencySafe:    true,
		MaxResultSizeChars: 100_000,
	}
}

func (t *CronListTool) Execute(_ context.Context, input map[string]any) (types.ToolResult, error) {
	if cronDisabledByFeatureFlag() {
		return cronFeatureFlagRefusal(), nil
	}
	if err := types.ValidateToolInput(t, input); err != nil {
		return types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolRuntimeInvalidInput, err), IsError: true}, nil
	}

	output := cronListOutput{Jobs: make([]cronListJobOutput, 0)}
	currentAgentID := t.currentAgentID()
	for _, job := range t.Store.list() {
		if currentAgentID != "" && job.AgentID != currentAgentID {
			continue
		}
		output.Jobs = append(output.Jobs, cronListProjection(job))
	}
	return types.ToolResult{
		Content: cronListContent(output),
		Data:    output,
	}, nil
}

func (t *CronListTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	output, ok := data.(cronListOutput)
	if !ok {
		return types.ToolResultBlock{
			Type:      types.ContentTypeToolResult,
			ToolUseID: toolUseID,
			Content:   fmt.Sprintf("%v", data),
		}
	}
	return types.ToolResultBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: toolUseID,
		Content:   cronListContent(output),
	}
}

func cronListProjection(job *CronJob) cronListJobOutput {
	out := cronListJobOutput{
		ID:            job.ID,
		Cron:          job.Cron,
		HumanSchedule: cronToHuman(job.Cron),
		Prompt:        job.Prompt,
	}
	if job.Recurring {
		value := true
		out.Recurring = &value
	}
	if !job.Durable {
		value := false
		out.Durable = &value
	}
	return out
}

func cronListContent(output cronListOutput) string {
	if len(output.Jobs) == 0 {
		return toolRuntimeText(i18n.KeyToolRuntimeCronNoJobs)
	}
	var sb strings.Builder
	for i, job := range output.Jobs {
		if i > 0 {
			sb.WriteByte('\n')
		}
		kind := toolRuntimeText(i18n.KeyToolRuntimeCronKindOneShot)
		if job.Recurring != nil && *job.Recurring {
			kind = toolRuntimeText(i18n.KeyToolRuntimeCronKindRecurring)
		}
		scope := ""
		if job.Durable != nil && !*job.Durable {
			scope = toolRuntimeText(i18n.KeyToolRuntimeCronScopeSessionOnly)
		}
		fmt.Fprintf(&sb, "%s — %s%s%s: %s", job.ID, job.HumanSchedule, kind, scope, truncateCronListPrompt(job.Prompt))
	}
	return sb.String()
}

func truncateCronListPrompt(prompt string) string {
	const maxWidth = 80
	if idx := strings.IndexByte(prompt, '\n'); idx >= 0 {
		line := prompt[:idx]
		if uniseg.StringWidth(line)+1 > maxWidth {
			return truncateCronTextToWidth(line, maxWidth)
		}
		return line + "…"
	}
	if uniseg.StringWidth(prompt) <= maxWidth {
		return prompt
	}
	return truncateCronTextToWidth(prompt, maxWidth)
}

func truncateCronTextToWidth(s string, maxWidth int) string {
	if uniseg.StringWidth(s) <= maxWidth {
		return s
	}
	if maxWidth <= 1 {
		return "…"
	}

	var truncated strings.Builder
	width := 0
	graphemes := uniseg.NewGraphemes(s)
	for graphemes.Next() {
		segment := graphemes.Str()
		segmentWidth := graphemes.Width()
		if width+segmentWidth > maxWidth-1 {
			break
		}
		truncated.WriteString(segment)
		width += segmentWidth
	}
	truncated.WriteRune('…')
	return truncated.String()
}

func cronToHuman(cron string) string {
	parts := strings.Fields(strings.TrimSpace(cron))
	if len(parts) != 5 {
		return cron
	}
	minute, hour, dayOfMonth, month, dayOfWeek := parts[0], parts[1], parts[2], parts[3], parts[4]
	if strings.HasPrefix(minute, "*/") && hour == "*" && dayOfMonth == "*" && month == "*" && dayOfWeek == "*" {
		n := strings.TrimPrefix(minute, "*/")
		if n == "1" {
			return toolRuntimeText(i18n.KeyToolRuntimeCronEveryMinute)
		}
		if n != "" {
			return toolRuntimeFormat(i18n.KeyToolRuntimeCronEveryMinutes, n)
		}
	}
	if isCronNumber(minute) && hour == "*" && dayOfMonth == "*" && month == "*" && dayOfWeek == "*" {
		if minute == "0" {
			return toolRuntimeText(i18n.KeyToolRuntimeCronEveryHour)
		}
		return toolRuntimeFormat(i18n.KeyToolRuntimeCronEveryHourAtMinute, leftPad2(minute))
	}
	if strings.HasPrefix(hour, "*/") && isCronNumber(minute) && dayOfMonth == "*" && month == "*" && dayOfWeek == "*" {
		n := strings.TrimPrefix(hour, "*/")
		suffix := ""
		if minute != "0" {
			suffix = toolRuntimeFormat(i18n.KeyToolRuntimeCronAtMinuteSuffix, leftPad2(minute))
		}
		if n == "1" {
			return toolRuntimeText(i18n.KeyToolRuntimeCronEveryHour) + suffix
		}
		if n != "" {
			return toolRuntimeFormat(i18n.KeyToolRuntimeCronEveryHours, n) + suffix
		}
	}
	if !isCronNumber(minute) || !isCronNumber(hour) {
		return cron
	}
	m, _ := strconv.Atoi(minute)
	h, _ := strconv.Atoi(hour)
	if dayOfMonth == "*" && month == "*" && dayOfWeek == "*" {
		return toolRuntimeFormat(i18n.KeyToolRuntimeCronEveryDayAt, formatCronLocalTime(m, h))
	}
	if dayOfMonth == "*" && month == "*" && isCronSingleDayOfWeek(dayOfWeek) {
		day, _ := strconv.Atoi(dayOfWeek)
		return toolRuntimeFormat(i18n.KeyToolRuntimeCronEveryWeekdayAt, cronDayName(day%7), formatCronLocalTime(m, h))
	}
	if dayOfMonth == "*" && month == "*" && dayOfWeek == "1-5" {
		return toolRuntimeFormat(i18n.KeyToolRuntimeCronWeekdaysAt, formatCronLocalTime(m, h))
	}
	return cron
}

func isCronNumber(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isCronSingleDayOfWeek(value string) bool {
	if !isCronNumber(value) {
		return false
	}
	day, err := strconv.Atoi(value)
	return err == nil && day >= 0 && day <= 7
}

func cronDayName(day int) string {
	names := []i18n.Key{
		i18n.KeyToolRuntimeCronSunday, i18n.KeyToolRuntimeCronMonday,
		i18n.KeyToolRuntimeCronTuesday, i18n.KeyToolRuntimeCronWednesday,
		i18n.KeyToolRuntimeCronThursday, i18n.KeyToolRuntimeCronFriday,
		i18n.KeyToolRuntimeCronSaturday,
	}
	if day < 0 || day >= len(names) {
		return ""
	}
	return toolRuntimeText(names[day])
}

func leftPad2(value string) string {
	if len(value) >= 2 {
		return value
	}
	return "0" + value
}

func formatCronLocalTime(minute, hour int) string {
	suffix := toolRuntimeText(i18n.KeyToolRuntimeCronAM)
	displayHour := hour
	if displayHour >= 12 {
		suffix = toolRuntimeText(i18n.KeyToolRuntimeCronPM)
	}
	displayHour = displayHour % 12
	if displayHour == 0 {
		displayHour = 12
	}
	return fmt.Sprintf("%d:%02d %s", displayHour, minute, suffix)
}
