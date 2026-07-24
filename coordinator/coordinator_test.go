package coordinator

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCoordinatorDispatch(t *testing.T) {
	coord := NewCoordinator()

	var executed int64
	agentFunc := func(ctx context.Context, task *Task) (string, error) {
		atomic.AddInt64(&executed, 1)
		time.Sleep(10 * time.Millisecond) // simulate work
		return fmt.Sprintf("done: %s", task.Description), nil
	}

	coord.RegisterAgent(&Agent{ID: "agent-1", Name: "Worker 1", Execute: agentFunc})
	coord.RegisterAgent(&Agent{ID: "agent-2", Name: "Worker 2", Execute: agentFunc})

	coord.AddTask("task A", 1)
	coord.AddTask("task B", 1)
	coord.AddTask("task C", 1)

	results := coord.Dispatch(context.Background())

	// At least 2 tasks should be dispatched (2 agents available)
	if len(results) < 2 {
		t.Errorf("expected at least 2 results, got %d", len(results))
	}

	for _, r := range results {
		if r.Error != nil {
			t.Errorf("task %s failed: %v", r.TaskID, r.Error)
		}
		if r.Result == "" {
			t.Errorf("task %s has empty result", r.TaskID)
		}
	}
}

func TestCoordinatorBlockedTasks(t *testing.T) {
	coord := NewCoordinator()
	var order []string
	var mu sync.Mutex
	agentFunc := func(ctx context.Context, task *Task) (string, error) {
		mu.Lock()
		order = append(order, task.ID)
		mu.Unlock()
		return "ok", nil
	}
	coord.RegisterAgent(&Agent{ID: "a1", Name: "Worker", Execute: agentFunc})

	t1 := coord.AddTask("first", 1)
	t2 := coord.AddTask("second", 1)
	t2.BlockedBy = []string{t1.ID}

	// Single dispatch should handle both: t1 first, then t2 after t1 completes
	results := coord.Dispatch(context.Background())
	if len(results) != 2 {
		t.Fatalf("expected 2 results (both tasks), got %d", len(results))
	}

	// t1 must have executed before t2
	mu.Lock()
	defer mu.Unlock()
	if len(order) != 2 {
		t.Fatalf("expected 2 executions, got %d", len(order))
	}
	if order[0] != t1.ID {
		t.Errorf("expected t1 first, got %s", order[0])
	}
	if order[1] != t2.ID {
		t.Errorf("expected t2 second, got %s", order[1])
	}
}

func TestMessageBus(t *testing.T) {
	bus := NewMessageBus()
	ch := bus.Subscribe("agent-1")

	err := bus.Send(Message{From: "agent-2", To: "agent-1", Content: "hello"})
	if err != nil {
		t.Fatal(err)
	}

	select {
	case msg := <-ch:
		if msg.Content != "hello" {
			t.Errorf("expected 'hello', got '%s'", msg.Content)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestPendingCount(t *testing.T) {
	coord := NewCoordinator()
	if coord.PendingCount() != 0 {
		t.Error("expected 0 pending")
	}
	coord.AddTask("a", 1)
	coord.AddTask("b", 1)
	if coord.PendingCount() != 2 {
		t.Errorf("expected 2 pending, got %d", coord.PendingCount())
	}
}

func TestCoordinatorConcurrentDispatchRace(t *testing.T) {
	t.Parallel()

	var totalExecuted atomic.Int64
	agentFunc := func(ctx context.Context, task *Task) (string, error) {
		totalExecuted.Add(1)
		time.Sleep(5 * time.Millisecond)
		return fmt.Sprintf("result: %s", task.Description), nil
	}

	coord := NewCoordinator()
	for i := 0; i < 5; i++ {
		coord.RegisterAgent(&Agent{
			ID:      fmt.Sprintf("agent-%d", i),
			Name:    fmt.Sprintf("Worker %d", i),
			Execute: agentFunc,
		})
	}

	for i := 0; i < 20; i++ {
		coord.AddTask(fmt.Sprintf("task %d", i), 1)
	}

	// Dispatch from multiple goroutines to stress-test concurrent access
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			coord.Dispatch(context.Background())
		}()
	}
	wg.Wait()

	if totalExecuted.Load() == 0 {
		t.Error("expected at least some tasks to execute")
	}
}

func TestMessageBusConcurrentRace(t *testing.T) {
	t.Parallel()

	bus := NewMessageBus()
	const numAgents = 10

	// Subscribe all agents
	channels := make([]<-chan Message, numAgents)
	for i := 0; i < numAgents; i++ {
		channels[i] = bus.Subscribe(fmt.Sprintf("agent-%d", i))
	}

	// Concurrently send messages
	var wg sync.WaitGroup
	for i := 0; i < numAgents; i++ {
		wg.Add(1)
		go func(from int) {
			defer wg.Done()
			to := (from + 1) % numAgents
			bus.Send(Message{
				From:    fmt.Sprintf("agent-%d", from),
				To:      fmt.Sprintf("agent-%d", to),
				Content: fmt.Sprintf("hello from %d", from),
			})
		}(i)
	}
	wg.Wait()

	// Verify at least some messages were received
	received := 0
	for i := 0; i < numAgents; i++ {
		select {
		case <-channels[i]:
			received++
		default:
		}
	}
	if received == 0 {
		t.Error("expected at least some messages to be received")
	}
}
