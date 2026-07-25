// Package schedule implements the CronCreate, CronDelete, and CronList tools
// and the runtime that delivers their scheduled work.
package schedule

import (
	"context"
	"errors"
	"strconv"
	"time"
)

const maxJobs = 50

// Job is an immutable snapshot passed to a scheduled execution. ProjectRoot
// is captured when the job is created so a later session switch cannot move
// the execution into a different project.
type Job struct {
	ID          string
	Expression  string
	Prompt      string
	Recurring   bool
	Durable     bool
	CreatedAt   time.Time
	LastFiredAt *time.Time
	AgentID     string
	ProjectRoot string
}

// Execution identifies one scheduled delivery. DeliveryID is stable across
// retries and process restarts; implementations must treat repeated enqueue
// calls with the same ID as idempotent.
type Execution struct {
	DeliveryID  string
	ScheduledAt time.Time
	Job         Job
}

// Executor durably accepts scheduled work. A successful return means that a
// repeated call with the same DeliveryID will not start the work twice.
type Executor interface {
	Enqueue(context.Context, Execution) error
}

// FireEvent is emitted only after the executor has accepted a delivery.
type FireEvent struct {
	DeliveryID  string
	JobID       string
	Expression  string
	Recurring   bool
	Durable     bool
	ScheduledAt time.Time
	ProjectRoot string
}

// FireSink observes accepted schedule deliveries. Notification delivery
// remains the responsibility of the background-agent runtime.
type FireSink interface {
	PublishScheduleFire(context.Context, FireEvent) error
}

type errorKind uint8

const (
	errorKindStoreRead errorKind = iota + 1
	errorKindStoreWrite
	errorKindStoreInvalid
	errorKindStoreVersion
	errorKindTooMany
	errorKindNotFound
	errorKindOwnerDenied
	errorKindDurableDenied
	errorKindID
)

type domainError struct {
	kind  errorKind
	cause error
}

type schemaVersionError int

func (e schemaVersionError) Error() string { return strconv.Itoa(int(e)) }

func (e *domainError) Error() string {
	if e == nil || e.cause == nil {
		return "schedule operation failed"
	}
	return e.cause.Error()
}

func (e *domainError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func newDomainError(kind errorKind, cause error) error {
	if cause == nil {
		cause = errors.New("schedule operation failed")
	}
	return &domainError{kind: kind, cause: cause}
}

func domainErrorKind(err error) errorKind {
	var target *domainError
	if errors.As(err, &target) {
		return target.kind
	}
	return 0
}

type pendingDelivery struct {
	ID            string
	ScheduledAt   time.Time
	WallKey       string
	NextAttemptAt time.Time
	Attempt       int
}

type storedJob struct {
	Job
	LastWallKey string
	Pending     *pendingDelivery
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneStoredJob(job *storedJob) *storedJob {
	if job == nil {
		return nil
	}
	cloned := *job
	cloned.LastFiredAt = cloneTime(job.LastFiredAt)
	if job.Pending != nil {
		pending := *job.Pending
		cloned.Pending = &pending
	}
	return &cloned
}
