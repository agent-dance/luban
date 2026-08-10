package compact

import (
	"context"
	"errors"
	"testing"

	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/types"
)

type compactCancelEOFProvider struct {
	started chan struct{}
	release <-chan struct{}
}

func (p *compactCancelEOFProvider) Name() string    { return "cancel-eof" }
func (p *compactCancelEOFProvider) ModelID() string { return "cancel-eof-model" }
func (p *compactCancelEOFProvider) CreateStream(context.Context, provider.Params) (<-chan types.StreamEvent, error) {
	ch := make(chan types.StreamEvent)
	close(p.started)
	go func() {
		<-p.release
		close(ch)
	}()
	return ch, nil
}

func TestStreamCompactSummaryClassifiesCancelledEmptyEOFAsUserAbort(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	p := &compactCancelEOFProvider{started: started, release: release}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := streamCompactSummary(ctx, p, provider.Params{})
		done <- err
	}()
	<-started
	cancel()
	close(release)
	err := <-done
	if !errors.Is(err, context.Canceled) || !IsCompactUserAbortError(err) || IsCompactNoSummaryError(err) {
		t.Fatalf("cancelled EOF error = %T %v, want user-abort cancellation and not no-summary", err, err)
	}
}
