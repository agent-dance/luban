package engine_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/engine"
	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/session"
	"github.com/agent-dance/luban/tools"
	"github.com/agent-dance/luban/types"
)

func backgroundTextEvents(text string) []types.StreamEvent {
	return []types.StreamEvent{
		{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
		{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "text_delta", Text: text}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageStop},
	}
}

func backgroundToolCallEvents(id, name string, input map[string]any) []types.StreamEvent {
	raw, _ := json.Marshal(input)
	return []types.StreamEvent{
		{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeToolUse, ID: id, Name: name}},
		{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: string(raw)}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageStop},
	}
}

type backgroundScriptedProvider struct {
	mu        sync.Mutex
	responses [][]types.StreamEvent
	call      int
}

func (*backgroundScriptedProvider) Name() string    { return "background-parent" }
func (*backgroundScriptedProvider) ModelID() string { return "background-parent-model" }
func (p *backgroundScriptedProvider) CreateStream(context.Context, provider.Params) (<-chan types.StreamEvent, error) {
	p.mu.Lock()
	index := p.call
	p.call++
	p.mu.Unlock()
	events := backgroundTextEvents("unexpected extra parent turn")
	if index < len(p.responses) {
		events = p.responses[index]
	}
	stream := make(chan types.StreamEvent, len(events))
	for _, event := range events {
		stream <- event
	}
	close(stream)
	return stream, nil
}

func drainBackgroundEvents(t *testing.T, stream <-chan engine.Event, timeout time.Duration) []engine.Event {
	t.Helper()
	var events []engine.Event
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case event, ok := <-stream:
			if !ok {
				return events
			}
			events = append(events, event)
		case <-timer.C:
			t.Fatal("timed out draining parent event stream")
		}
	}
}

type delayedBackgroundAgentProvider struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (*delayedBackgroundAgentProvider) Name() string    { return "delayed-background-agent" }
func (*delayedBackgroundAgentProvider) ModelID() string { return "delayed-background-agent-model" }
func (p *delayedBackgroundAgentProvider) CreateStream(ctx context.Context, _ provider.Params) (<-chan types.StreamEvent, error) {
	p.once.Do(func() { close(p.started) })
	stream := make(chan types.StreamEvent, 8)
	go func() {
		defer close(stream)
		select {
		case <-p.release:
			for _, event := range backgroundTextEvents("delayed agent complete") {
				select {
				case stream <- event:
				case <-ctx.Done():
					return
				}
			}
		case <-ctx.Done():
		}
	}()
	return stream, nil
}

func TestDelayedBackgroundAgentNotificationAfterFinalDoesNotWriteClosedQueryStream(t *testing.T) {
	projectRoot := t.TempDir()
	background := tools.NewBackgroundTaskManager(projectRoot)
	t.Cleanup(background.Shutdown)
	runner := hooks.NewRunner([]hooks.Hook{{
		Type: hooks.HookNotification, Command: `printf 'durable delayed hook evidence'`, Timeout: 2,
	}})
	childProvider := &delayedBackgroundAgentProvider{started: make(chan struct{}), release: make(chan struct{})}
	childRegistry := registry.New()
	parentRegistry := registry.New()
	parentRegistry.Register(&tools.AgentTool{
		Provider: childProvider, Registry: childRegistry, Background: background, HookRunner: runner,
	})
	parentProvider := &backgroundScriptedProvider{
		responses: [][]types.StreamEvent{
			backgroundToolCallEvents("parent-agent-tool", "Agent", map[string]any{
				"description": "delayed retained agent", "prompt": "finish after the parent", "run_in_background": true,
			}),
			backgroundTextEvents("parent query complete"),
		},
	}
	repo := session.NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD(projectRoot)
	eng, err := engine.New(engine.Config{
		Provider: parentProvider, Registry: parentRegistry,
		Sessions:   engine.NewRepositorySessionManager(repo, func() string { return projectDir }),
		HookRunner: runner, BackgroundTasks: background,
		ProjectRoot: projectRoot, CWD: projectRoot, EventBufferSize: 64, MaxTurns: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = eng.Shutdown(context.Background()) })

	eventStream, err := eng.Query(context.Background(), engine.QueryRequest{
		SessionID: "parent-session", Message: "launch delayed work", ProjectRoot: projectRoot, CWD: projectRoot,
	})
	if err != nil {
		t.Fatal(err)
	}
	events := drainBackgroundEvents(t, eventStream, 5*time.Second)
	if final := events[len(events)-1]; !final.Final || final.Error != nil {
		t.Fatalf("parent final = %#v", final)
	}
	select {
	case <-childProvider.started:
	case <-time.After(2 * time.Second):
		t.Fatal("background Agent did not start before release")
	}

	snapshots := background.Snapshots()
	if len(snapshots) != 1 || snapshots[0].ID == "" {
		t.Fatalf("background snapshots = %#v, want one retained Agent", snapshots)
	}
	close(childProvider.release)
	if snapshot, status := background.Wait(snapshots[0].ID, 5*time.Second); status != "success" || snapshot.Status != "completed" {
		t.Fatalf("background completion status=%q snapshot=%#v", status, snapshot)
	}

	record, ok := tools.NewRuntimeTaskStore(projectRoot).Get(snapshots[0].ID)
	if !ok || record.Notification == nil || record.Notification.DeliveredAt == nil {
		t.Fatalf("durable notification record = %#v", record)
	}
	if len(record.Notification.HookExecutions) != 1 {
		t.Fatalf("durable hook execution receipts = %#v", record.Notification.HookExecutions)
	}
	receipt := record.Notification.HookExecutions[0]
	if receipt.HookType != hooks.HookNotification || receipt.ExecutionID == "" || receipt.ConfigID == "" || receipt.ConfigIndex != 1 {
		t.Fatalf("durable hook identity = %#v", receipt)
	}
	input := receipt.Input
	if input.SessionID != "parent-session" || input.ProjectRoot != projectRoot || input.ToolName != "Agent" ||
		input.ToolUseID != "parent-agent-tool" || input.TaskID != snapshots[0].ID || input.AgentID != "assistant" ||
		input.TurnID == "" || input.WorkUnitID == "" {
		t.Fatalf("durable parent causality = %#v", input)
	}
	if receipt.Output.Stdout != "durable delayed hook evidence" || receipt.Output.ExitCode != 0 || receipt.RecordedAt.IsZero() {
		t.Fatalf("durable raw hook evidence = %#v", receipt)
	}
}
