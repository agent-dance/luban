package loop

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

type toolExecutionContextKey struct{}

// ToolExecutionContext is attached to each tool execution. It gives tools that
// need conversation-aware behavior, such as Agent fork mode, a read-only
// snapshot of the parent loop state at the moment the assistant requested the
// tool.
type ToolExecutionContext struct {
	Messages          []types.Message
	AssistantMessage  types.Message
	ToolUse           types.ToolUseBlock
	SessionID         string
	CacheLineageID    string
	TurnID            string
	ActorID           string
	ActorType         string
	WorkUnitID        string
	RunID             string
	BatchID           string
	ParentRunID       string
	AgentPath         string
	SessionProjectDir string
	ProjectRoot       string
	CWD               string
	System            string
	Model             string

	// loadedSkillLedger is installed only by the QueryLoop that owns this
	// execution. Keeping the capability private prevents unrelated callers from
	// forging loaded-body evidence while still allowing runtime composition to
	// consume it through ResolveSkillLoadedLedger. The closure captures an
	// immutable message snapshot and never calls skills.Manager.
	loadedSkillLedger func(skills.SkillID) SkillLoadedLedgerState
	// skillProjectGeneration is installed only by the owning QueryLoop. Model
	// tools use the exported accessor, so callers cannot forge a generation by
	// constructing ToolExecutionContext literals in another package.
	skillProjectGeneration skills.ProjectSourceGeneration
	owner                  *QueryLoop
	runToken               string
	ownedSessionID         string
	ownedSessionProjectDir string
	ownedProjectRoot       string
	ownedCWD               string
	readEvidenceOwnerID    string
	readEvidenceEpoch      uint64
	ownedReadEvidenceActor string
}

// ActiveReadEvidenceScope returns an opaque, loop-owned namespace for file
// observation evidence. Exported identity fields are not trusted: the active
// run capability, private owner, actor, and current context epoch must all
// still match the QueryLoop that created the tool execution.
func (exec ToolExecutionContext) ActiveReadEvidenceScope() (string, bool) {
	if exec.owner == nil || !exec.OwnedBy(exec.owner) || exec.readEvidenceOwnerID == "" ||
		exec.readEvidenceOwnerID != exec.owner.readEvidenceOwnerID ||
		exec.ownedReadEvidenceActor == "" || exec.ownedReadEvidenceActor != exec.ActorID {
		return "", false
	}
	exec.owner.skillCatalogMu.RLock()
	currentEpoch := exec.owner.skillCatalogEpoch
	exec.owner.skillCatalogMu.RUnlock()
	if currentEpoch == 0 || exec.readEvidenceEpoch != currentEpoch {
		return "", false
	}
	return fmt.Sprintf("%s\x1f%s\x1f%d", exec.readEvidenceOwnerID, exec.ownedReadEvidenceActor, exec.readEvidenceEpoch), true
}

// ResolveSkillLoadedLedger resolves loaded-body evidence through the exact
// QueryLoop that created this execution context. The boolean is false for
// contexts constructed outside a running loop; callers must fail closed rather
// than fall back to another conversation with the same bare session ID.
func (exec ToolExecutionContext) ResolveSkillLoadedLedger(id skills.SkillID) (SkillLoadedLedgerState, bool) {
	if exec.loadedSkillLedger == nil {
		return SkillLoadedLedgerState{}, false
	}
	return exec.loadedSkillLedger(id), true
}

// SkillProjectGeneration returns the exact project authority pinned by the
// owning model run. The boolean is false for externally constructed contexts.
func (exec ToolExecutionContext) SkillProjectGeneration() (skills.ProjectSourceGeneration, bool) {
	if exec.skillProjectGeneration == 0 || exec.loadedSkillLedger == nil {
		return 0, false
	}
	return exec.skillProjectGeneration, true
}

// IsLoopOwned reports whether this execution context carries a private
// QueryLoop capability. Runtime workspace rebinds require this proof rather
// than trusting exported identity strings alone.
func (exec ToolExecutionContext) IsLoopOwned() bool {
	return exec.loadedSkillLedger != nil && exec.owner != nil && exec.runToken != ""
}

// OwnedBy proves that this context came from q's currently active Run. A
// retained context from an earlier run of the same QueryLoop is rejected.
func (exec ToolExecutionContext) OwnedBy(q *QueryLoop) bool {
	if q == nil || !exec.IsLoopOwned() || exec.owner != q {
		return false
	}
	q.activeRunTokenMu.RLock()
	active := q.activeRunToken
	q.activeRunTokenMu.RUnlock()
	return active != "" && active == exec.runToken
}

// ActiveRuntimeIdentity returns the immutable session/workspace identity that
// the owning QueryLoop bound to this currently active Run. Exported context
// fields are intentionally not trusted because a caller can copy and rewrap a
// ToolExecutionContext while retaining its private capability fields.
func (exec ToolExecutionContext) ActiveRuntimeIdentity() (sessionID, projectRoot, cwd string, ok bool) {
	sessionID, _, projectRoot, cwd, ok = exec.ActiveRuntimeOwnerIdentity()
	return sessionID, projectRoot, cwd, ok
}

// ActiveRuntimeOwnerIdentity returns the immutable session, durable session
// namespace, and workspace identity bound by the owning QueryLoop. Unlike the
// exported ToolExecutionContext fields, these values cannot be changed by
// copying and rewrapping a live execution context.
func (exec ToolExecutionContext) ActiveRuntimeOwnerIdentity() (sessionID, sessionProjectDir, projectRoot, cwd string, ok bool) {
	if exec.owner == nil || !exec.OwnedBy(exec.owner) {
		return "", "", "", "", false
	}
	if exec.ownedSessionID == "" || exec.ownedSessionID != exec.SessionID ||
		exec.ownedSessionProjectDir != exec.SessionProjectDir ||
		exec.ownedProjectRoot != exec.ProjectRoot || exec.ownedCWD != exec.CWD {
		return "", "", "", "", false
	}
	return exec.ownedSessionID, exec.ownedSessionProjectDir, exec.ownedProjectRoot, exec.ownedCWD, true
}

// WithToolExecutionContext returns a child context carrying tool execution
// metadata.
func WithToolExecutionContext(ctx context.Context, exec ToolExecutionContext) context.Context {
	exec.Messages = cloneMessages(exec.Messages)
	exec.AssistantMessage = cloneMessage(exec.AssistantMessage)
	exec.ToolUse = cloneToolUseBlock(exec.ToolUse)
	ctx = context.WithValue(ctx, toolExecutionContextKey{}, exec)
	return hooks.WithCorrelation(ctx, hooks.HookInput{
		SessionID: exec.SessionID, ProjectRoot: exec.ProjectRoot, TurnID: exec.TurnID,
		WorkUnitID: exec.WorkUnitID, AgentID: exec.ActorID, AgentType: exec.ActorType,
		ToolName: exec.ToolUse.Name, ToolUseID: exec.ToolUse.ID,
	})
}

// ToolExecutionContextFromContext retrieves tool execution metadata.
func ToolExecutionContextFromContext(ctx context.Context) (ToolExecutionContext, bool) {
	exec, ok := ctx.Value(toolExecutionContextKey{}).(ToolExecutionContext)
	if !ok {
		return ToolExecutionContext{}, false
	}
	exec.Messages = cloneMessages(exec.Messages)
	exec.AssistantMessage = cloneMessage(exec.AssistantMessage)
	exec.ToolUse = cloneToolUseBlock(exec.ToolUse)
	return exec, true
}

func cloneMessages(messages []types.Message) []types.Message {
	if messages == nil {
		return nil
	}
	out := make([]types.Message, len(messages))
	for i := range messages {
		out[i] = cloneMessage(messages[i])
	}
	return out
}

func cloneMessage(message types.Message) types.Message {
	// Start from the complete value: the unexported provenance authenticator is
	// copy-safe and remains valid only if the deep clone is byte-identical.
	out := message
	if message.DeveloperMetadata != nil {
		metadata := *message.DeveloperMetadata
		out.DeveloperMetadata = &metadata
	}
	if message.Content != nil {
		out.Content = make([]types.ContentBlock, len(message.Content))
		for index, block := range message.Content {
			out.Content[index] = cloneContentBlock(block)
		}
	}
	return out
}

func cloneContentBlock(block types.ContentBlock) types.ContentBlock {
	switch typed := block.(type) {
	case types.TextBlock:
		return typed
	case types.ThinkingBlock:
		return typed
	case types.ToolUseBlock:
		return cloneToolUseBlock(typed)
	case types.ToolResultBlock:
		typed.ContentBlocks = cloneContentBlocks(typed.ContentBlocks)
		typed.Data = cloneToolContextValue(typed.Data)
		typed.NewMessages = cloneMessages(typed.NewMessages)
		typed.Metadata = cloneStringMap(typed.Metadata)
		if typed.Usage != nil {
			usage := *typed.Usage
			typed.Usage = &usage
		}
		return typed
	case types.ContentReplacementBlock:
		return typed
	case types.ToolReferenceBlock:
		return typed
	case types.ImageBlock:
		if typed.Source != nil {
			source := *typed.Source
			typed.Source = &source
		}
		return typed
	case types.DocumentBlock:
		if typed.Source != nil {
			source := *typed.Source
			typed.Source = &source
		}
		return typed
	case types.UnknownBlock:
		typed.Raw = append(json.RawMessage(nil), typed.Raw...)
		return typed
	default:
		// ContentBlock implementations outside types are treated as immutable.
		// Keeping the concrete value preserves forward-compatible blocks instead
		// of erasing them through a JSON round trip.
		return block
	}
}

func cloneToolUseBlock(block types.ToolUseBlock) types.ToolUseBlock {
	block.Input = cloneStringAnyMap(block.Input)
	return block
}

func cloneContentBlocks(blocks []types.ContentBlock) []types.ContentBlock {
	if blocks == nil {
		return nil
	}
	out := make([]types.ContentBlock, len(blocks))
	for index, block := range blocks {
		out[index] = cloneContentBlock(block)
	}
	return out
}

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func cloneStringAnyMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = cloneToolContextValue(value)
	}
	return out
}

func cloneToolContextValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneStringAnyMap(typed)
	case map[string]string:
		return cloneStringMap(typed)
	case []any:
		if typed == nil {
			return []any(nil)
		}
		out := make([]any, len(typed))
		for index, item := range typed {
			out[index] = cloneToolContextValue(item)
		}
		return out
	case []string:
		return append([]string(nil), typed...)
	case []byte:
		return append([]byte(nil), typed...)
	case json.RawMessage:
		return append(json.RawMessage(nil), typed...)
	default:
		return value
	}
}
