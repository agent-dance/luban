package agent

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
	"github.com/agent-dance/luban/internal/runtime/loop"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type retainedSessionTestProvider struct {
	calls atomic.Int32
}

func (p *retainedSessionTestProvider) Name() string    { return "retained-session-test" }
func (p *retainedSessionTestProvider) ModelID() string { return "retained-session-test-model" }

func (p *retainedSessionTestProvider) CreateStream(context.Context, provider.Params) (<-chan types.StreamEvent, error) {
	p.calls.Add(1)
	stream := make(chan types.StreamEvent, 4)
	stream <- types.StreamEvent{
		Type:         types.EventContentBlockStart,
		Index:        0,
		ContentBlock: &types.ContentDelta{Type: types.ContentTypeText},
	}
	stream <- types.StreamEvent{
		Type:  types.EventContentBlockDelta,
		Index: 0,
		Delta: &types.ContentDelta{Type: "text_delta", Text: "round complete"},
	}
	stream <- types.StreamEvent{Type: types.EventContentBlockStop, Index: 0}
	stream <- types.StreamEvent{Type: types.EventMessageStop}
	close(stream)
	return stream, nil
}

func TestRunSyncCancellationWhileQueuedPreventsLaterExecution(t *testing.T) {
	root := t.TempDir()
	manager := NewBackgroundTaskManager(root)
	provider := &retainedSessionTestProvider{}
	agentID := "queued-cancel-agent"
	origin := manager.currentTaskOrigin()
	task := &BackgroundTask{
		ID:         agentID,
		Type:       agentcontract.TaskTypeLocalAgent,
		Status:     "completed",
		OutputPath: origin.taskOutputPath(agentID),
		done:       closedTaskDoneChannel(),
		origin:     origin,
	}
	parent, cancelSession := context.WithCancel(context.Background())
	defer cancelSession()
	session := &backgroundAgentSession{
		parent:  parent,
		cancel:  cancelSession,
		loop:    loop.New(provider, registry.New(), loop.Config{Model: provider.ModelID(), MaxTokens: 1024, SessionID: agentID}),
		task:    task,
		manager: manager,
		queue:   make(chan agentRunRequest, 1),
		done:    make(chan struct{}),
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	runResult := make(chan error, 1)
	go func() {
		_, err := session.runSync(runCtx, "must never execute")
		runResult <- err
	}()

	deadline := time.Now().Add(2 * time.Second)
	for len(session.queue) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(session.queue) != 1 {
		t.Fatal("synchronous request was not queued")
	}
	task.mu.RLock()
	installedCancel := task.cancel
	task.mu.RUnlock()
	if installedCancel != nil {
		t.Fatal("test precondition failed: queued request already installed task.cancel")
	}

	cancelRun()
	select {
	case err := <-runResult:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("runSync error=%v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runSync did not return after cancellation")
	}

	request := <-session.queue
	session.handleRequest(request)
	if got := provider.calls.Load(); got != 0 {
		t.Fatalf("canceled queued request executed provider %d times", got)
	}
	snapshot := task.snapshot()
	if snapshot.Status != "cancelled" || snapshot.Outcome != agentcontract.RunOutcomeCancelled || snapshot.TerminalReason != "context_cancelled" || snapshot.Error != context.Canceled.Error() {
		t.Fatalf("canceled queued task snapshot=%+v", snapshot)
	}
}
