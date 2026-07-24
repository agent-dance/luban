package coordinator

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/observability"
)

// TaskStatus represents the state of a task
type TaskStatus string

const (
	TaskPending TaskStatus = "pending"
	TaskRunning TaskStatus = "running"
	TaskDone    TaskStatus = "done"
	TaskFailed  TaskStatus = "failed"
)

// Task represents a unit of work for an agent
type Task struct {
	ID          string
	RunID       string // stable ID for the current execution attempt
	Description string
	Status      TaskStatus
	AssignedTo  string // agent ID
	Result      string
	Error       error
	Priority    int
	CreatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
	BlockedBy   []string          // task IDs that must complete first
	Metadata    map[string]string // arbitrary key/value payload (e.g. system_prompt)
}

// AgentFunc is the function signature for an agent worker
type AgentFunc func(ctx context.Context, task *Task) (string, error)

// Agent represents a worker that can execute tasks
type Agent struct {
	ID           string
	Name         string
	Capabilities []string
	Execute      AgentFunc
	SystemPrompt string // optional role/system prompt; empty = use default
	busy         bool   // protected by Coordinator.mu — do not access directly
	runID        string // protected by Coordinator.mu; identifies the reserved run
}

// IsBusy returns whether the agent is currently working on a task.
// Must be called while holding Coordinator.mu.
func (a *Agent) IsBusy() bool {
	return a.busy
}

// MessageBus allows inter-agent communication
type MessageBus struct {
	mu           sync.RWMutex
	channels     map[string]chan Message
	nextSequence uint64
	delivered    map[string]uint64
	acknowledged map[string]uint64
}

// Message represents an inter-agent message
type Message struct {
	From     string
	To       string
	Content  string
	Time     time.Time
	Sequence uint64 // assigned by MessageBus when the message is sent
}

// NewMessageBus creates a new message bus
func NewMessageBus() *MessageBus {
	return &MessageBus{
		channels:     make(map[string]chan Message),
		delivered:    make(map[string]uint64),
		acknowledged: make(map[string]uint64),
	}
}

// Subscribe creates a channel for an agent to receive messages
func (mb *MessageBus) Subscribe(agentID string) <-chan Message {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	if previous, ok := mb.channels[agentID]; ok {
		close(previous)
	}
	ch := make(chan Message, 32)
	mb.channels[agentID] = ch
	return ch
}

// Send delivers a message to a specific agent
func (mb *MessageBus) Send(msg Message) error {
	_, err := mb.SendSequenced(msg)
	return err
}

// SendSequenced delivers a message and returns the stable sequence assigned by
// the bus. Send remains available for callers that do not need acknowledgments.
func (mb *MessageBus) SendSequenced(msg Message) (uint64, error) {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	ch, ok := mb.channels[msg.To]
	if !ok {
		return 0, i18n.NewError(i18n.KeyCoordinatorAgentNotSubscribed, msg.To)
	}
	mb.nextSequence++
	msg.Sequence = mb.nextSequence
	select {
	case ch <- msg:
		mb.delivered[msg.To] = msg.Sequence
		return msg.Sequence, nil
	default:
		return msg.Sequence, i18n.NewError(i18n.KeyCoordinatorAgentChannelFull, msg.To)
	}
}

// GetChannel returns the receive channel for an agent, or nil if not subscribed.
func (mb *MessageBus) GetChannel(agentID string) <-chan Message {
	mb.mu.RLock()
	defer mb.mu.RUnlock()
	ch, ok := mb.channels[agentID]
	if !ok {
		return nil
	}
	return ch
}

// Broadcast sends a message to all agents except sender.
// Returns the number of agents whose channels were full (messages dropped).
func (mb *MessageBus) Broadcast(from string, content string) int {
	_, dropped := mb.BroadcastSequenced(from, content)
	return dropped
}

// BroadcastSequenced broadcasts one logical message with the same sequence to
// every recipient and returns that sequence together with the drop count.
func (mb *MessageBus) BroadcastSequenced(from string, content string) (uint64, int) {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	mb.nextSequence++
	sequence := mb.nextSequence
	msg := Message{From: from, Content: content, Time: time.Now(), Sequence: sequence}
	dropped := 0
	for id, ch := range mb.channels {
		if id != from {
			select {
			case ch <- msg:
				mb.delivered[id] = sequence
			default:
				dropped++
			}
		}
	}
	return sequence, dropped
}

// Ack advances agentID's cumulative acknowledgment watermark. Acknowledgments
// are idempotent and cannot move beyond the last message delivered to the agent.
func (mb *MessageBus) Ack(agentID string, sequence uint64) bool {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	if sequence == 0 || sequence > mb.delivered[agentID] {
		return false
	}
	if sequence > mb.acknowledged[agentID] {
		mb.acknowledged[agentID] = sequence
	}
	return true
}

// AckedThrough returns agentID's cumulative acknowledgment watermark.
func (mb *MessageBus) AckedThrough(agentID string) uint64 {
	mb.mu.RLock()
	defer mb.mu.RUnlock()
	return mb.acknowledged[agentID]
}

// Coordinator manages multiple agents and task distribution
type Coordinator struct {
	mu        sync.Mutex
	agents    map[string]*Agent
	tasks     []*Task
	bus       *MessageBus
	nextID    int64
	nextRunID uint64
	results   map[string]string     // taskID -> result
	statusMap map[string]TaskStatus // taskID -> status for O(1) dependency checks
}

// NewCoordinator creates a new multi-agent coordinator
func NewCoordinator() *Coordinator {
	return &Coordinator{
		agents:    make(map[string]*Agent),
		bus:       NewMessageBus(),
		results:   make(map[string]string),
		statusMap: make(map[string]TaskStatus),
	}
}

// RegisterAgent adds an agent to the coordinator
func (c *Coordinator) RegisterAgent(agent *Agent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.agents[agent.ID] = agent
	c.bus.Subscribe(agent.ID)
}

// RemoveAgent unregisters an agent and removes its MessageBus subscription.
// It is safe to call for unknown agent IDs.
func (c *Coordinator) RemoveAgent(agentID string) {
	c.mu.Lock()
	delete(c.agents, agentID)
	c.mu.Unlock()
	c.bus.Unsubscribe(agentID)
}

// AgentIDs returns a stable snapshot of registered agent IDs.
func (c *Coordinator) AgentIDs() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	ids := make([]string, 0, len(c.agents))
	for id := range c.agents {
		ids = append(ids, id)
	}
	return ids
}

// AddTask adds a task to the queue
func (c *Coordinator) AddTask(description string, priority int) *Task {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nextID++
	id := c.nextID
	task := &Task{
		ID:          fmt.Sprintf("task-%d", id),
		Description: description,
		Status:      TaskPending,
		Priority:    priority,
		CreatedAt:   time.Now(),
	}
	c.tasks = append(c.tasks, task)
	c.statusMap[task.ID] = TaskPending
	return task
}

// Dispatch assigns pending tasks to available agents and executes them.
// It loops until all reachable tasks are complete, handling BlockedBy dependencies.
func (c *Coordinator) Dispatch(ctx context.Context) []TaskResult {
	var mu sync.Mutex
	var results []TaskResult

	for {
		if ctx.Err() != nil {
			break
		}

		// Try to assign all currently dispatchable tasks
		var wg sync.WaitGroup
		dispatched := 0

		for {
			assignment := c.reserveAssignment()
			if assignment == nil {
				break
			}
			task, agent, runID := assignment.task, assignment.agent, assignment.runID

			dispatched++
			wg.Add(1)
			go func(t *Task, a *Agent, reservedRunID string) {
				defer wg.Done()
				defer func() {
					if r := recover(); r != nil {
						panicErr := i18n.NewError(i18n.KeyCoordinatorAgentPanicked, a.ID, r)
						if c.completeAssignment(t, a, reservedRunID, "", panicErr) {
							mu.Lock()
							results = append(results, TaskResult{
								TaskID:  t.ID,
								RunID:   reservedRunID,
								AgentID: a.ID,
								Error:   panicErr,
							})
							mu.Unlock()
						}
					}
				}()

				// Inject per-agent system prompt into task metadata before execution.
				if a.SystemPrompt != "" {
					if t.Metadata == nil {
						t.Metadata = make(map[string]string)
					}
					t.Metadata["system_prompt"] = a.SystemPrompt
				}

				result, err := a.Execute(ctx, t)

				if c.completeAssignment(t, a, reservedRunID, result, err) {
					mu.Lock()
					results = append(results, TaskResult{
						TaskID:  t.ID,
						RunID:   reservedRunID,
						AgentID: a.ID,
						Result:  result,
						Error:   err,
					})
					mu.Unlock()
				}
			}(task, agent, runID)
		}

		if dispatched == 0 {
			// No tasks could be dispatched — either all done or permanently blocked.
			// Mark tasks blocked by failed dependencies as failed too.
			c.mu.Lock()
			for _, t := range c.tasks {
				if t.Status == TaskPending {
					for _, depID := range t.BlockedBy {
						if status, ok := c.statusMap[depID]; ok && status == TaskFailed {
							t.Status = TaskFailed
							t.Error = i18n.NewError(i18n.KeyCoordinatorDependencyFailedSkip, depID)
							c.statusMap[t.ID] = TaskFailed
							break
						}
					}
				}
			}
			c.mu.Unlock()
			break
		}

		// Wait for this batch to complete, then re-check for newly unblocked tasks
		wg.Wait()
	}

	return results
}

// TaskResult holds the outcome of a task execution
type TaskResult struct {
	TaskID  string
	RunID   string
	AgentID string
	Result  string
	Error   error
}

type taskAssignment struct {
	task  *Task
	agent *Agent
	runID string
}

// reserveAssignment selects and commits an assignment in one lock transaction.
// No caller can observe an agent as busy while its task is still pending.
func (c *Coordinator) reserveAssignment() *taskAssignment {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Find next pending task (respecting blocked-by)
	var nextTask *Task
	for _, t := range c.tasks {
		if t.Status != TaskPending {
			continue
		}
		if c.isBlocked(t) {
			continue
		}
		if nextTask == nil || t.Priority > nextTask.Priority {
			nextTask = t
		}
	}
	if nextTask == nil {
		return nil
	}

	// Commit every observable part of the assignment under the same lock.
	for _, a := range c.agents {
		if !a.busy {
			c.nextRunID++
			runID := fmt.Sprintf("run-%d", c.nextRunID)
			now := time.Now()
			nextTask.Status = TaskRunning
			nextTask.AssignedTo = a.ID
			nextTask.StartedAt = &now
			nextTask.RunID = runID
			c.statusMap[nextTask.ID] = TaskRunning
			a.busy = true
			a.runID = runID
			return &taskAssignment{task: nextTask, agent: a, runID: runID}
		}
	}
	return nil
}

// completeAssignment applies a result only if it belongs to the currently
// reserved run. This prevents stale completions from releasing or overwriting a
// later assignment if retry support is added.
func (c *Coordinator) completeAssignment(task *Task, agent *Agent, runID, result string, err error) bool {
	c.mu.Lock()
	if task.Status != TaskRunning || task.RunID != runID || agent.runID != runID {
		c.mu.Unlock()
		observability.RecordGenerationDrop(observability.GenerationSurfaceCoordinatorCompletion)
		return false
	}
	completed := time.Now()
	task.CompletedAt = &completed
	if err != nil {
		task.Status = TaskFailed
		task.Error = err
		c.statusMap[task.ID] = TaskFailed
	} else {
		task.Status = TaskDone
		task.Result = result
		c.results[task.ID] = result
		c.statusMap[task.ID] = TaskDone
	}
	agent.busy = false
	agent.runID = ""
	c.mu.Unlock()
	return true
}

func (c *Coordinator) isBlocked(task *Task) bool {
	for _, depID := range task.BlockedBy {
		status, ok := c.statusMap[depID]
		if !ok {
			return true // unknown dependency = blocked (fail-safe)
		}
		if status != TaskDone {
			return true
		}
	}
	return false
}

// GetBus returns the message bus for inter-agent communication
func (c *Coordinator) GetBus() *MessageBus { return c.bus }

// GetTasks returns deep copies of all tasks, safe for concurrent reads.
func (c *Coordinator) GetTasks() []Task {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]Task, len(c.tasks))
	for i, t := range c.tasks {
		result[i] = *t
	}
	return result
}

// PendingCount returns number of pending tasks
func (c *Coordinator) PendingCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	count := 0
	for _, t := range c.tasks {
		if t.Status == TaskPending {
			count++
		}
	}
	return count
}

// PruneCompleted removes completed and failed tasks to bound memory growth.
func (c *Coordinator) PruneCompleted() {
	c.mu.Lock()
	defer c.mu.Unlock()
	active := make([]*Task, 0, len(c.tasks))
	for _, t := range c.tasks {
		if t.Status == TaskPending || t.Status == TaskRunning {
			active = append(active, t)
		} else {
			delete(c.results, t.ID)
			delete(c.statusMap, t.ID)
		}
	}
	c.tasks = active
}

// Drain returns all messages currently queued for agentID without blocking.
// Safe to call concurrently with Send/Broadcast.
func (mb *MessageBus) Drain(agentID string) []Message {
	mb.mu.RLock()
	ch, ok := mb.channels[agentID]
	mb.mu.RUnlock()
	if !ok {
		return nil
	}
	return drainMessages(ch)
}

func drainMessages(ch <-chan Message) []Message {
	var msgs []Message
	for {
		select {
		case msg, open := <-ch:
			if !open {
				return msgs
			}
			msgs = append(msgs, msg)
		default:
			return msgs
		}
	}
}

// IsSubscribed reports whether agentID has a channel on the bus.
func (mb *MessageBus) IsSubscribed(agentID string) bool {
	mb.mu.RLock()
	defer mb.mu.RUnlock()
	_, ok := mb.channels[agentID]
	return ok
}

// Subscribers returns the IDs of all currently subscribed agents.
// The returned slice is a snapshot — safe to mutate without affecting the bus.
func (mb *MessageBus) Subscribers() []string {
	mb.mu.RLock()
	defer mb.mu.RUnlock()
	ids := make([]string, 0, len(mb.channels))
	for id := range mb.channels {
		ids = append(ids, id)
	}
	return ids
}

// Unsubscribe closes and removes the channel for the given agent.
func (mb *MessageBus) Unsubscribe(agentID string) {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	if ch, ok := mb.channels[agentID]; ok {
		close(ch)
		delete(mb.channels, agentID)
	}
}
