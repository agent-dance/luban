package coordinator

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestCoordinatorExactlyOnceConcurrentDispatch(t *testing.T) {
	const (
		dispatchers = 32
		agentCount  = 16
		taskCount   = 1000
	)

	coord := NewCoordinator()
	firstWaveEntered := make(chan struct{}, agentCount)
	releaseFirstWave := make(chan struct{})

	var observationsMu sync.Mutex
	executions := make(map[string]int, taskCount)
	observedRunID := make(map[string]string, taskCount)
	allRunIDs := make(map[string]string, taskCount)

	execute := func(_ context.Context, task *Task) (string, error) {
		observationsMu.Lock()
		executions[task.ID]++
		if previousTask, duplicate := allRunIDs[task.RunID]; duplicate {
			t.Errorf("run ID %q reused by tasks %s and %s", task.RunID, previousTask, task.ID)
		} else {
			allRunIDs[task.RunID] = task.ID
		}
		if previous, seen := observedRunID[task.ID]; seen && previous != task.RunID {
			t.Errorf("task %s changed run ID from %q to %q", task.ID, previous, task.RunID)
		}
		observedRunID[task.ID] = task.RunID
		observationsMu.Unlock()

		select {
		case firstWaveEntered <- struct{}{}:
			<-releaseFirstWave
		default:
		}
		return task.RunID, nil
	}

	for i := 0; i < agentCount; i++ {
		coord.RegisterAgent(&Agent{
			ID:      fmt.Sprintf("agent-%d", i),
			Name:    fmt.Sprintf("Agent %d", i),
			Execute: execute,
		})
	}
	for i := 0; i < taskCount; i++ {
		coord.AddTask(fmt.Sprintf("task %d", i), 1)
	}

	start := make(chan struct{})
	var dispatchWG sync.WaitGroup
	dispatchWG.Add(dispatchers)
	for i := 0; i < dispatchers; i++ {
		go func() {
			defer dispatchWG.Done()
			<-start
			coord.Dispatch(context.Background())
		}()
	}
	close(start)

	for i := 0; i < agentCount; i++ {
		<-firstWaveEntered
	}
	close(releaseFirstWave)

	dispatchDone := make(chan struct{})
	go func() {
		dispatchWG.Wait()
		close(dispatchDone)
	}()
	select {
	case <-dispatchDone:
	case <-time.After(10 * time.Second):
		t.Fatal("concurrent dispatch did not complete")
	}

	observationsMu.Lock()
	defer observationsMu.Unlock()
	if len(executions) != taskCount {
		t.Fatalf("executed %d distinct tasks, want %d", len(executions), taskCount)
	}
	for taskID, count := range executions {
		if count != 1 {
			t.Errorf("task %s executed %d times, want exactly once", taskID, count)
		}
	}

	tasks := coord.GetTasks()
	if len(tasks) != taskCount {
		t.Fatalf("coordinator retained %d tasks, want %d", len(tasks), taskCount)
	}
	for _, task := range tasks {
		if task.Status != TaskDone {
			t.Errorf("task %s status = %s, want %s", task.ID, task.Status, TaskDone)
		}
		if task.RunID == "" || task.RunID != observedRunID[task.ID] {
			t.Errorf("task %s final run ID %q, observed %q", task.ID, task.RunID, observedRunID[task.ID])
		}
		if task.AssignedTo == "" || task.StartedAt == nil || task.CompletedAt == nil {
			t.Errorf("task %s has incomplete committed assignment", task.ID)
		}
	}
}

func TestMessageBusDrainClosedChannel(t *testing.T) {
	ch := make(chan Message, 2)
	ch <- Message{Content: "first", Sequence: 1}
	ch <- Message{Content: "second", Sequence: 2}
	close(ch)

	done := make(chan []Message, 1)
	go func() {
		done <- drainMessages(ch)
	}()

	select {
	case messages := <-done:
		if len(messages) != 2 {
			t.Fatalf("drained %d buffered messages from closed channel, want 2", len(messages))
		}
		if messages[0].Sequence != 1 || messages[1].Sequence != 2 {
			t.Fatalf("drain changed message order: %#v", messages)
		}
	case <-time.After(time.Second):
		t.Fatal("draining a closed channel did not terminate")
	}
}

func TestMessageBusDrainVsUnsubscribe(t *testing.T) {
	const rounds = 256
	deadline := time.After(10 * time.Second)
	for round := 0; round < rounds; round++ {
		bus := NewMessageBus()
		agentID := fmt.Sprintf("agent-%d", round)
		bus.Subscribe(agentID)
		for i := 0; i < 8; i++ {
			if err := bus.Send(Message{To: agentID, Content: fmt.Sprintf("message-%d", i)}); err != nil {
				t.Fatalf("round %d send %d: %v", round, i, err)
			}
		}

		start := make(chan struct{})
		drainDone := make(chan struct{})
		unsubscribeDone := make(chan struct{})
		go func() {
			<-start
			bus.Drain(agentID)
			close(drainDone)
		}()
		go func() {
			<-start
			bus.Unsubscribe(agentID)
			close(unsubscribeDone)
		}()
		close(start)

		for _, done := range []<-chan struct{}{drainDone, unsubscribeDone} {
			select {
			case <-done:
			case <-deadline:
				t.Fatalf("Drain/Unsubscribe did not terminate in round %d", round)
			}
		}
	}
}

func TestMessageBusStableSequenceAndAck(t *testing.T) {
	bus := NewMessageBus()
	recipient := bus.Subscribe("recipient")
	peer := bus.Subscribe("peer")

	sequence, err := bus.SendSequenced(Message{From: "sender", To: "recipient", Content: "one"})
	if err != nil {
		t.Fatal(err)
	}
	message := <-recipient
	if sequence == 0 || message.Sequence != sequence {
		t.Fatalf("delivered sequence = %d, returned sequence = %d", message.Sequence, sequence)
	}
	if bus.Ack("recipient", sequence+1) {
		t.Fatal("acknowledged a sequence that was not delivered")
	}
	if !bus.Ack("recipient", sequence) || !bus.Ack("recipient", sequence) {
		t.Fatal("valid acknowledgment was not idempotent")
	}
	if got := bus.AckedThrough("recipient"); got != sequence {
		t.Fatalf("ack watermark = %d, want %d", got, sequence)
	}

	broadcastSequence, dropped := bus.BroadcastSequenced("sender", "two")
	if dropped != 0 {
		t.Fatalf("broadcast dropped %d messages", dropped)
	}
	recipientBroadcast := <-recipient
	peerBroadcast := <-peer
	if broadcastSequence <= sequence {
		t.Fatalf("broadcast sequence %d did not advance past %d", broadcastSequence, sequence)
	}
	if recipientBroadcast.Sequence != broadcastSequence || peerBroadcast.Sequence != broadcastSequence {
		t.Fatalf("broadcast recipients saw different sequences: %d, %d, want %d", recipientBroadcast.Sequence, peerBroadcast.Sequence, broadcastSequence)
	}
	if !bus.Ack("peer", broadcastSequence) {
		t.Fatal("peer could not acknowledge delivered broadcast")
	}

	bus.Unsubscribe("recipient")
	bus.Subscribe("recipient")
	if got := bus.AckedThrough("recipient"); got != sequence {
		t.Fatalf("resubscribe reset ack watermark to %d, want %d", got, sequence)
	}
}
