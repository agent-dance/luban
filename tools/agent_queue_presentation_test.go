package tools

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type queueReasonProvider struct {
	calls   atomic.Int32
	started chan struct{}
	release chan struct{}
}

func (p *queueReasonProvider) Name() string    { return "queue-reason" }
func (p *queueReasonProvider) ModelID() string { return "queue-reason-model" }

func (p *queueReasonProvider) CreateStream(ctx context.Context, _ provider.Params) (<-chan types.StreamEvent, error) {
	call := p.calls.Add(1)
	if call == 1 {
		close(p.started)
		select {
		case <-p.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	stream := make(chan types.StreamEvent, 4)
	stream <- types.StreamEvent{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}}
	stream <- types.StreamEvent{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "text_delta", Text: "done"}}
	stream <- types.StreamEvent{Type: types.EventContentBlockStop, Index: 0}
	stream <- types.StreamEvent{Type: types.EventMessageStop}
	close(stream)
	return stream, nil
}

func TestRetainedAgentQueueExposesCountAndDependencyReason(t *testing.T) {
	root := t.TempDir()
	manager := NewBackgroundTaskManager(root)
	t.Cleanup(manager.Shutdown)
	p := &queueReasonProvider{started: make(chan struct{}), release: make(chan struct{})}
	agentID := "queued-agent"
	query := loop.New(p, registry.New(), loop.Config{Model: p.ModelID(), MaxTokens: 1024, SessionID: agentID})
	session, _, err := manager.RegisterAgentSession(
		agentID, "helper", "initial", "queue reason test",
		AgentInput{Prompt: "initial", Description: "queue reason test"}, query,
		agentSessionMetadata{AgentType: "general-purpose", Model: p.ModelID(), CWD: root}, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	firstDone := make(chan error, 1)
	go func() {
		_, runErr := session.runSync(context.Background(), "first")
		firstDone <- runErr
	}()
	select {
	case <-p.started:
	case <-time.After(2 * time.Second):
		t.Fatal("first run did not start")
	}
	if err := session.enqueue("second", nil); err != nil {
		t.Fatal(err)
	}
	snapshot, ok := manager.Snapshot(agentID)
	if !ok || snapshot.QueuedPrompts != 1 || snapshot.QueueReason != "dependency:active_run" {
		t.Fatalf("queued snapshot=%+v ok=%v", snapshot, ok)
	}
	persisted, ok := manager.store.Get(agentID)
	if !ok || persisted.QueuedPrompts != 1 || persisted.QueueReason != "dependency:active_run" {
		t.Fatalf("queued record=%+v ok=%v", persisted, ok)
	}

	close(p.release)
	select {
	case runErr := <-firstDone:
		if runErr != nil {
			t.Fatal(runErr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("first run did not finish")
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		snapshot, _ = manager.Snapshot(agentID)
		if p.calls.Load() >= 2 && snapshot.Status == "completed" && snapshot.Attempt == 2 && snapshot.QueuedPrompts == 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if snapshot.Attempt != 2 || snapshot.QueuedPrompts != 0 || snapshot.QueueReason != "" {
		t.Fatalf("drained queue snapshot=%+v calls=%d", snapshot, p.calls.Load())
	}
}
