package loop

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/internal/runtime/compact"
)

func TestManualCompactionPublishesOrderedNonTerminalPhases(t *testing.T) {
	q := New(nil, nil, Config{MaxContextTokens: 100})
	var stages []string
	_, err := q.runCompaction(context.Background(), "manual", 0, func(event stream.Event) {
		if event.Type == stream.EventProgress && event.Progress != nil {
			stages = append(stages, event.Progress.Stage)
		}
	}, func() (*compact.CompactionResult, error) {
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"compact_start", "compact_preparing", "compact_summarizing", "compact_installing", "compact_end"}
	if !reflect.DeepEqual(stages, want) {
		t.Fatalf("manual compaction stages = %v, want %v", stages, want)
	}
}

func TestManualCompactionCancellationOverridesConcurrentProviderFailure(t *testing.T) {
	q := New(nil, nil, Config{MaxContextTokens: 100})
	ctx, cancel := context.WithCancel(context.Background())
	started, release := make(chan struct{}), make(chan struct{})
	providerErr := errors.New("invalid provider summary")
	var events []stream.Event
	done := make(chan error, 1)
	go func() {
		_, _, err := q.runCompactionAgainst(ctx, "manual", 0, func(event stream.Event) {
			events = append(events, event)
		}, nil, func() (*compact.CompactionResult, error) {
			close(started)
			<-release
			return &compact.CompactionResult{}, providerErr
		})
		done <- err
	}()
	<-started
	cancel()
	close(release)
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("compaction error = %v, want cancellation to override %v", err, providerErr)
	}
	var cancelled, failed int
	for _, event := range events {
		if event.Progress == nil {
			continue
		}
		switch event.Progress.Stage {
		case "compact_cancelled":
			cancelled++
		case "compact_failed":
			failed++
		}
	}
	if cancelled != 1 || failed != 0 {
		t.Fatalf("terminal lifecycle cancelled=%d failed=%d events=%+v", cancelled, failed, events)
	}
}
