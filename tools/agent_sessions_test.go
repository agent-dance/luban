package tools

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agent-dance/luban/loop"
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

func TestRetainedAgentTodosSurviveRoundsUntilSessionEnds(t *testing.T) {
	for _, tc := range []struct {
		name string
		end  func(*BackgroundTaskManager, string, *backgroundAgentSession)
	}{
		{
			name: "retirement",
			end: func(manager *BackgroundTaskManager, agentID string, session *backgroundAgentSession) {
				manager.retireAgentSession(agentID, session)
			},
		},
		{
			name: "shutdown",
			end: func(manager *BackgroundTaskManager, _ string, _ *backgroundAgentSession) {
				manager.Shutdown()
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			manager := NewBackgroundTaskManager(root)
			t.Cleanup(manager.Shutdown)
			provider := &retainedSessionTestProvider{}
			agentID := "todo-retained-" + tc.name
			queryLoop := loop.New(provider, registry.New(), loop.Config{
				Model:     provider.ModelID(),
				MaxTokens: 1024,
				SessionID: agentID,
			})
			session, _, err := manager.RegisterAgentSession(
				agentID,
				"",
				"initial prompt",
				"retained todo test",
				AgentInput{Prompt: "initial prompt", Description: "retained todo test"},
				queryLoop,
				agentSessionMetadata{AgentType: "general-purpose", Model: provider.ModelID(), CWD: root},
				nil,
				nil,
			)
			if err != nil {
				t.Fatalf("register retained session: %v", err)
			}

			store := NewTodoStore(root).withAgentID(agentID)
			if err := store.Save([]TodoItem{{Content: "keep across rounds", Status: "pending", ActiveForm: "working"}}); err != nil {
				t.Fatalf("save scoped todo: %v", err)
			}
			todoPath := filepath.Join(root, ".claude", "todos", agentID+".json")

			for _, prompt := range []string{"first round", "second round"} {
				if _, err := session.runSync(context.Background(), prompt); err != nil {
					t.Fatalf("run %q: %v", prompt, err)
				}
				if _, err := os.Stat(todoPath); err != nil {
					t.Fatalf("retained todo disappeared after %q: %v", prompt, err)
				}
			}
			if got := provider.calls.Load(); got != 2 {
				t.Fatalf("provider calls=%d, want 2", got)
			}

			tc.end(manager, agentID, session)
			select {
			case <-session.done:
			case <-time.After(2 * time.Second):
				t.Fatal("retained session did not finish")
			}
			if _, err := os.Stat(todoPath); !os.IsNotExist(err) {
				t.Fatalf("scoped todo still exists after session %s: %v", tc.name, err)
			}
		})
	}
}

func TestRunSyncCancellationWhileQueuedPreventsLaterExecution(t *testing.T) {
	root := t.TempDir()
	manager := NewBackgroundTaskManager(root)
	provider := &retainedSessionTestProvider{}
	agentID := "queued-cancel-agent"
	origin := manager.currentTaskOrigin()
	task := &BackgroundTask{
		ID:         agentID,
		Type:       backgroundTaskTypeLocalAgent,
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
	if snapshot.Status != "cancelled" || snapshot.Outcome != AgentRunOutcomeCancelled || snapshot.TerminalReason != "context_cancelled" || snapshot.Error != context.Canceled.Error() {
		t.Fatalf("canceled queued task snapshot=%+v", snapshot)
	}
}
