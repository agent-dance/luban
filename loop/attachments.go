package loop

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/prompt"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

// CommandPriority mirrors the original command queue's "next" and "later"
// buckets. The Sleep tool drains through "later"; ordinary tool turns drain
// only "next".
type CommandPriority string

const (
	CommandPriorityNext  CommandPriority = "next"
	CommandPriorityLater CommandPriority = "later"
)

// QueryScope identifies which loop instance may consume process-global queued
// commands. The main loop consumes main-thread prompts and notifications;
// subagents only consume task notifications addressed to their agent ID.
type QueryScope struct {
	IsSubagent bool
	AgentID    string
}

// QueuedCommand is the minimal queue surface needed by the tool-post
// attachment pipeline.
type QueuedCommand struct {
	UUID     string
	Mode     string
	Content  string
	AgentID  string
	Priority CommandPriority
}

// CommandQueue provides a snapshot-and-ack interface for queued commands.
type CommandQueue interface {
	Snapshot(maxPriority CommandPriority) []QueuedCommand
	MarkStarted(uuid string)
	MarkCompleted(uuid string)
	Remove(commands []QueuedCommand)
}

// AttachmentProvider allows callers to convert queued commands into model
// messages. It is intentionally optional; when absent, queued command content
// is wrapped in a user message.
type AttachmentProvider interface {
	CommandAttachment(ctx context.Context, command QueuedCommand) (types.Message, bool, error)
}

// PendingAttachmentPrefetch is a zero-wait prefetch handle. Collect is called
// only when Ready reports true, so an unfinished memory prefetch never blocks
// the current model turn.
type PendingAttachmentPrefetch interface {
	Ready() bool
	Collect(ctx context.Context) ([]types.Message, error)
}

type MemoryPrefetcher interface {
	StartMemoryPrefetch(ctx context.Context, messages []types.Message) PendingAttachmentPrefetch
}

type SkillPrefetcher interface {
	StartSkillPrefetch(ctx context.Context, messages []types.Message) PendingAttachmentPrefetch
}

// ToolRefresher refreshes dynamic tool state, such as newly connected MCP
// tools, after tool results have entered history and before the next provider
// request is built.
type ToolRefresher interface {
	RefreshTools(ctx context.Context, current *registry.Registry) (*registry.Registry, error)
}

func commandPriorityAllowed(priority, max CommandPriority) bool {
	if max == CommandPriorityLater {
		return priority == CommandPriorityNext || priority == CommandPriorityLater || priority == ""
	}
	return priority == CommandPriorityNext || priority == ""
}

func filterQueuedCommands(commands []QueuedCommand, scope QueryScope, maxPriority CommandPriority) []QueuedCommand {
	out := make([]QueuedCommand, 0, len(commands))
	for _, cmd := range commands {
		if !commandPriorityAllowed(cmd.Priority, maxPriority) {
			continue
		}
		if cmd.Mode == "slash" {
			continue
		}
		if scope.IsSubagent {
			if cmd.Mode == "task-notification" && cmd.AgentID == scope.AgentID {
				out = append(out, cmd)
			}
			continue
		}
		if cmd.AgentID == "" {
			out = append(out, cmd)
		}
	}
	return out
}

func commandAttachmentMessage(cmd QueuedCommand) types.Message {
	return types.UserMessage(cmd.Content)
}

func (q *QueryLoop) appendPostToolAttachments(ctx context.Context, state *QueryState, snapshot QueryConfigSnapshot, toolUses []types.ToolUseBlock, toolResults []types.ToolResultBlock, reminders []string) error {
	runtimeMessages := make([]types.Message, 0)
	for _, result := range toolResults {
		for _, message := range result.NewMessages {
			if message.HasInternalControlProvenance() {
				scope, bound := message.InternalControlProvenanceScope()
				switch {
				case !bound:
					// Tools can authenticate an in-process control attachment, but
					// only the owning QueryLoop knows the durable session scope.
					// Bind it here before it reaches another provider turn or save.
					message = q.sealRuntimeControlMessage(message)
				case !scope.Equal(q.internalControlScope):
					return i18n.NewError(i18n.KeyLoopQueryControlScopeInvalid)
				}
			}
			runtimeMessages = append(runtimeMessages, message)
		}
	}
	state.Messages = append(state.Messages, runtimeMessages...)

	maxPriority := CommandPriorityNext
	for _, toolUse := range toolUses {
		if toolUse.Name == "Sleep" {
			maxPriority = CommandPriorityLater
			break
		}
	}
	if snapshot.CommandQueue != nil {
		snapshotCommands := snapshot.CommandQueue.Snapshot(maxPriority)
		commands := filterQueuedCommands(snapshotCommands, snapshot.QueryScope, maxPriority)
		for _, cmd := range commands {
			msg := commandAttachmentMessage(cmd)
			if provider, ok := snapshot.CommandQueue.(AttachmentProvider); ok {
				var use bool
				var err error
				msg, use, err = provider.CommandAttachment(ctx, cmd)
				if err != nil {
					return i18n.WrapInternalError(i18n.KeyLoopAttachmentCommandFailed, err)
				}
				if !use {
					continue
				}
			}
			state.Messages = append(state.Messages, msg)
		}
		if len(commands) > 0 {
			for _, cmd := range commands {
				if cmd.UUID == "" {
					continue
				}
				snapshot.CommandQueue.MarkStarted(cmd.UUID)
				snapshot.CommandQueue.MarkCompleted(cmd.UUID)
			}
			snapshot.CommandQueue.Remove(commands)
		}
	}

	if state.PendingMemoryPrefetch != nil && !state.MemoryPrefetchConsumed && state.PendingMemoryPrefetch.Ready() {
		messages, err := state.PendingMemoryPrefetch.Collect(ctx)
		if err != nil {
			return i18n.WrapInternalError(i18n.KeyLoopAttachmentMemoryPrefetchFailed, err)
		}
		state.Messages = append(state.Messages, messages...)
		state.MemoryPrefetchConsumed = true
	}

	if state.PendingSkillPrefetch != nil {
		messages, err := state.PendingSkillPrefetch.Collect(ctx)
		if err != nil {
			return i18n.WrapInternalError(i18n.KeyLoopAttachmentSkillPrefetchFailed, err)
		}
		state.Messages = append(state.Messages, messages...)
		state.PendingSkillPrefetch = nil
	}

	if len(reminders) > 0 {
		state.Messages = append(state.Messages, types.UserMessage("<system-reminder>\n"+joinLines(reminders)+"\n</system-reminder>"))
	}

	if snapshot.ToolRefresher != nil {
		refreshed, err := snapshot.ToolRefresher.RefreshTools(ctx, q.registry)
		if err != nil {
			return i18n.WrapInternalError(i18n.KeyLoopAttachmentToolRefreshFailed, err)
		}
		if refreshed != nil && refreshed != q.registry {
			q.registry = refreshed
		}
	}
	return nil
}

func (q *QueryLoop) injectMCPInstructionsDelta(messages []types.Message) []types.Message {
	if q == nil || q.config.MCPState == nil {
		return messages
	}

	servers := q.config.MCPState.PostCompactMCPServers()
	connected := make(map[string]prompt.MCPServerInstruction, len(servers))
	connectedNames := make(map[string]struct{}, len(servers))
	for _, server := range servers {
		name := strings.TrimSpace(server.Name)
		if name == "" {
			continue
		}
		connectedNames[name] = struct{}{}
		instructions := strings.TrimSpace(server.Instructions)
		if instructions == "" {
			continue
		}
		connected[name] = prompt.MCPServerInstruction{Name: name, Instructions: instructions}
	}

	if q.mcpInstructionAnnouncements == nil {
		q.mcpInstructionAnnouncements = make(map[string]struct{})
	}

	addedNames := make([]string, 0)
	for name := range connected {
		if _, announced := q.mcpInstructionAnnouncements[name]; !announced {
			addedNames = append(addedNames, name)
		}
	}
	sort.Strings(addedNames)

	removedNames := make([]string, 0)
	for name := range q.mcpInstructionAnnouncements {
		if _, stillConnected := connectedNames[name]; !stillConnected {
			removedNames = append(removedNames, name)
		}
	}
	sort.Strings(removedNames)

	if len(addedNames) == 0 && len(removedNames) == 0 {
		return messages
	}

	parts := make([]string, 0, 2)
	if len(addedNames) > 0 {
		added := make([]prompt.MCPServerInstruction, 0, len(addedNames))
		for _, name := range addedNames {
			added = append(added, connected[name])
		}
		if section := prompt.MCPInstructionsSectionForLanguage(i18n.DetectOrLoadLanguage(), added); section != "" {
			parts = append(parts, section)
		}
	}
	if len(removedNames) > 0 {
		parts = append(parts, i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyLoopVisibleMCPDisconnected, strings.Join(removedNames, "\n")))
	}
	if len(parts) == 0 {
		return messages
	}

	messages = append(messages, types.UserMessage("<system-reminder>\n"+strings.Join(parts, "\n\n")+"\n</system-reminder>"))
	for _, name := range addedNames {
		q.mcpInstructionAnnouncements[name] = struct{}{}
	}
	for _, name := range removedNames {
		delete(q.mcpInstructionAnnouncements, name)
	}
	return messages
}

func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	n := 0
	for _, line := range lines {
		n += len(line)
	}
	n += len(lines) - 1
	buf := make([]byte, 0, n)
	for i, line := range lines {
		if i > 0 {
			buf = append(buf, '\n')
		}
		buf = append(buf, line...)
	}
	return string(buf)
}

// MemoryCommandQueue is a small in-memory implementation used by tests and by
// callers that need parity queue behavior without a larger command subsystem.
type MemoryCommandQueue struct {
	mu        sync.Mutex
	commands  []QueuedCommand
	started   []string
	completed []string
}

func NewMemoryCommandQueue(commands ...QueuedCommand) *MemoryCommandQueue {
	q := &MemoryCommandQueue{}
	q.commands = append(q.commands, commands...)
	return q
}

func (q *MemoryCommandQueue) Snapshot(maxPriority CommandPriority) []QueuedCommand {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]QueuedCommand, 0, len(q.commands))
	for _, cmd := range q.commands {
		if commandPriorityAllowed(cmd.Priority, maxPriority) {
			out = append(out, cmd)
		}
	}
	return out
}

func (q *MemoryCommandQueue) MarkStarted(uuid string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.started = append(q.started, uuid)
}

func (q *MemoryCommandQueue) MarkCompleted(uuid string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.completed = append(q.completed, uuid)
}

func (q *MemoryCommandQueue) Remove(commands []QueuedCommand) {
	q.mu.Lock()
	defer q.mu.Unlock()
	remove := make(map[string]struct{}, len(commands))
	for _, cmd := range commands {
		if cmd.UUID != "" {
			remove[cmd.UUID] = struct{}{}
		}
	}
	kept := q.commands[:0]
	for _, cmd := range q.commands {
		if _, ok := remove[cmd.UUID]; ok && cmd.UUID != "" {
			continue
		}
		kept = append(kept, cmd)
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
