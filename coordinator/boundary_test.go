package coordinator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/i18n"
)

func TestCoordinatorTaskFailure(t *testing.T) {
	coord := NewCoordinator()
	coord.RegisterAgent(&Agent{
		ID: "a1", Name: "Failing Worker",
		Execute: func(ctx context.Context, task *Task) (string, error) {
			return "", errors.New("agent crashed")
		},
	})
	coord.AddTask("will fail", 1)
	results := coord.Dispatch(context.Background())
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Error == nil {
		t.Error("expected error in result")
	}
	if results[0].Error.Error() != "agent crashed" {
		t.Errorf("unexpected error: %s", results[0].Error)
	}
}

func TestCoordinatorContextCancel(t *testing.T) {
	coord := NewCoordinator()
	coord.RegisterAgent(&Agent{
		ID: "a1", Name: "Slow Worker",
		Execute: func(ctx context.Context, task *Task) (string, error) {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(5 * time.Second):
				return "done", nil
			}
		},
	})
	coord.AddTask("slow task", 1)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	results := coord.Dispatch(ctx)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Error == nil {
		t.Error("expected context cancellation error")
	}
}

func TestCoordinatorCircularDependency(t *testing.T) {
	coord := NewCoordinator()
	coord.RegisterAgent(&Agent{
		ID: "a1", Name: "Worker",
		Execute: func(ctx context.Context, task *Task) (string, error) {
			return "ok", nil
		},
	})
	tA := coord.AddTask("A", 1)
	tB := coord.AddTask("B", 1)
	tA.BlockedBy = []string{tB.ID}
	tB.BlockedBy = []string{tA.ID}

	// Should return without hanging — both tasks are permanently blocked
	results := coord.Dispatch(context.Background())
	if len(results) != 0 {
		t.Errorf("expected 0 results for circular dependency, got %d", len(results))
	}
}

func TestCoordinatorPriorityDispatch(t *testing.T) {
	coord := NewCoordinator()
	var mu sync.Mutex
	var order []string
	coord.RegisterAgent(&Agent{
		ID: "a1", Name: "Worker",
		Execute: func(ctx context.Context, task *Task) (string, error) {
			mu.Lock()
			order = append(order, task.Description)
			mu.Unlock()
			return "ok", nil
		},
	})

	coord.AddTask("low", 1)
	coord.AddTask("high", 5)
	coord.AddTask("mid", 3)

	coord.Dispatch(context.Background())

	mu.Lock()
	defer mu.Unlock()
	if len(order) != 3 {
		t.Fatalf("expected 3 executions, got %d", len(order))
	}
	if order[0] != "high" {
		t.Errorf("expected 'high' first, got '%s'", order[0])
	}
	if order[1] != "mid" {
		t.Errorf("expected 'mid' second, got '%s'", order[1])
	}
	if order[2] != "low" {
		t.Errorf("expected 'low' third, got '%s'", order[2])
	}
}

func TestMessageBusSendToUnsubscribed(t *testing.T) {
	bus := NewMessageBus()
	err := bus.Send(Message{From: "a", To: "nonexistent", Content: "hi"})
	if err == nil {
		t.Error("expected error sending to unsubscribed agent")
	}
}

func TestMessageBusChannelFull(t *testing.T) {
	bus := NewMessageBus()
	bus.Subscribe("agent-1") // buffer size 32

	// Fill the buffer
	for i := 0; i < 32; i++ {
		bus.Send(Message{From: "x", To: "agent-1", Content: "msg"})
	}
	// 33rd should fail
	err := bus.Send(Message{From: "x", To: "agent-1", Content: "overflow"})
	if err == nil {
		t.Error("expected channel full error")
	}
}

func TestCoordinatorAgentPanicRecovery(t *testing.T) {
	coord := NewCoordinator()
	coord.RegisterAgent(&Agent{
		ID:   "panic-agent",
		Name: "Panicking Worker",
		Execute: func(ctx context.Context, task *Task) (string, error) {
			panic("something went very wrong")
		},
	})
	coord.AddTask("trigger panic", 1)

	// Should not crash the process — panic must be caught and returned as an error.
	results := coord.Dispatch(context.Background())
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r.Error == nil {
		t.Fatal("expected error from panic recovery, got nil")
	}
	want := i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyCoordinatorAgentPanicked, "panic-agent", "something went very wrong")
	if r.Error.Error() != want {
		t.Errorf("unexpected error message:\n  got:  %s\n  want: %s", r.Error.Error(), want)
	}
}

func TestCoordinatorSystemPromptInjection(t *testing.T) {
	coord := NewCoordinator()
	var capturedPrompt string
	coord.RegisterAgent(&Agent{
		ID:           "sp-agent",
		Name:         "System Prompt Worker",
		SystemPrompt: "You are a helpful coding assistant.",
		Execute: func(ctx context.Context, task *Task) (string, error) {
			capturedPrompt = task.Metadata["system_prompt"]
			return "ok", nil
		},
	})
	coord.AddTask("check system prompt", 1)
	coord.Dispatch(context.Background())

	want := "You are a helpful coding assistant."
	if capturedPrompt != want {
		t.Errorf("system prompt not injected:\n  got:  %q\n  want: %q", capturedPrompt, want)
	}
}

func TestCoordinatorSystemPromptEmptySkipsMetadata(t *testing.T) {
	coord := NewCoordinator()
	coord.RegisterAgent(&Agent{
		ID:   "no-sp-agent",
		Name: "No System Prompt Worker",
		// SystemPrompt intentionally left empty
		Execute: func(ctx context.Context, task *Task) (string, error) {
			if task.Metadata != nil {
				if _, ok := task.Metadata["system_prompt"]; ok {
					return "", errors.New("system_prompt key should not be set when SystemPrompt is empty")
				}
			}
			return "ok", nil
		},
	})
	coord.AddTask("no system prompt task", 1)
	results := coord.Dispatch(context.Background())
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Error != nil {
		t.Errorf("unexpected error: %v", results[0].Error)
	}
}

func TestMessageBusBroadcastSkipsSender(t *testing.T) {
	bus := NewMessageBus()
	ch1 := bus.Subscribe("agent-1")
	ch2 := bus.Subscribe("agent-2")

	bus.Broadcast("agent-1", "hello all")

	// agent-2 should receive
	select {
	case msg := <-ch2:
		if msg.Content != "hello all" {
			t.Errorf("expected 'hello all', got '%s'", msg.Content)
		}
	case <-time.After(time.Second):
		t.Error("agent-2 should have received broadcast")
	}

	// agent-1 (sender) should NOT receive
	select {
	case <-ch1:
		t.Error("sender should not receive own broadcast")
	default:
		// OK — channel is empty
	}
}
