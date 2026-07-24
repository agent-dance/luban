package provider

import (
	"context"
	"time"
)

// RetryEvent describes a transient provider failure immediately before the
// provider waits and tries the request again.
type RetryEvent struct {
	Attempt    int
	MaxRetries int
	Delay      time.Duration
	Err        error
}

type retryObserverContextKey struct{}

// WithRetryObserver attaches a request-scoped retry observer. It is used by
// interactive clients to project retry progress without changing provider
// configuration shared by other sessions.
func WithRetryObserver(ctx context.Context, observer func(RetryEvent)) context.Context {
	if observer == nil {
		return ctx
	}
	return context.WithValue(ctx, retryObserverContextKey{}, observer)
}

func notifyRetryObserver(ctx context.Context, event RetryEvent) {
	if ctx == nil {
		return
	}
	observer, _ := ctx.Value(retryObserverContextKey{}).(func(RetryEvent))
	if observer != nil {
		observer(event)
	}
}
