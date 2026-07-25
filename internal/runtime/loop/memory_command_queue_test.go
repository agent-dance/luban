package loop

import "sync"

type MemoryCommandQueue struct {
	mu        sync.Mutex
	commands  []QueuedCommand
	started   []string
	completed []string
}

func NewMemoryCommandQueue(commands ...QueuedCommand) *MemoryCommandQueue {
	return &MemoryCommandQueue{commands: append([]QueuedCommand(nil), commands...)}
}

func (q *MemoryCommandQueue) Snapshot(maxPriority CommandPriority) []QueuedCommand {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]QueuedCommand, 0, len(q.commands))
	for _, command := range q.commands {
		if commandPriorityAllowed(command.Priority, maxPriority) {
			out = append(out, command)
		}
	}
	return out
}

func (q *MemoryCommandQueue) MarkStarted(id string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.started = append(q.started, id)
}

func (q *MemoryCommandQueue) MarkCompleted(id string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.completed = append(q.completed, id)
}

func (q *MemoryCommandQueue) Remove(commands []QueuedCommand) {
	q.mu.Lock()
	defer q.mu.Unlock()
	remove := make(map[string]struct{}, len(commands))
	for _, command := range commands {
		if command.UUID != "" {
			remove[command.UUID] = struct{}{}
		}
	}
	kept := q.commands[:0]
	for _, command := range q.commands {
		if _, ok := remove[command.UUID]; ok && command.UUID != "" {
			continue
		}
		kept = append(kept, command)
	}
	q.commands = kept
}

func (q *MemoryCommandQueue) Started() []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]string(nil), q.started...)
}

func (q *MemoryCommandQueue) Completed() []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]string(nil), q.completed...)
}

func (q *MemoryCommandQueue) Remaining() []QueuedCommand {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]QueuedCommand(nil), q.commands...)
}
