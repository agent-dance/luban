// Package execution defines the runtime-neutral context carried through one
// model-requested tool invocation.
package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/agent-dance/luban/types"
)

type toolExecutionContextKey struct{}

// ToolExecutionContext is the immutable execution snapshot visible to tools.
// Runtime-owned capabilities are deliberately private; copying the exported
// fields cannot forge an active owner identity or evidence scope.
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

	owner   *Owner
	binding BindSpec
}

// RuntimeOwnerIdentity is the immutable session and workspace identity bound
// by the runtime that created a tool invocation.
type RuntimeOwnerIdentity struct {
	SessionID         string
	SessionProjectDir string
	ProjectRoot       string
	CWD               string
}

// SkillLoadedLedgerState is the neutral evidence projection for one stable
// skill identifier. Digest strings are validated by the skill domain after
// crossing this contract boundary.
type SkillLoadedLedgerState struct {
	ContextEpoch       uint64
	LoadedContextEpoch uint64
	ContentDigest      string
	PayloadDigest      string
}

// SkillLoadedLedgerResolver resolves evidence against the immutable visible
// history captured at the tool invocation boundary.
type SkillLoadedLedgerResolver func(string) SkillLoadedLedgerState

// BindSpec contains the private runtime inputs attached by an Owner.
// Callers receive only the resulting ToolExecutionContext; the binding itself
// is not stored on context.Context.
type BindSpec struct {
	RunToken               string
	Identity               RuntimeOwnerIdentity
	SkillProjectGeneration uint64
	ResolveSkillLedger     SkillLoadedLedgerResolver
	ReadEvidenceOwnerID    string
	ReadEvidenceEpoch      uint64
	ReadEvidenceActorID    string
	CurrentEvidenceEpoch   func() uint64
}

// Owner is the exact runtime authority for one execution stream. It tracks the
// active run token so retained contexts fail ownership checks after the run.
type Owner struct {
	mu             sync.RWMutex
	activeRunToken string
}

// NewOwner creates an execution authority for one runtime loop.
func NewOwner() *Owner { return &Owner{} }

// BeginRun activates one run token, replacing any previous token. Runtime loop
// composition is the sole caller outside this contract package.
func BeginRun(o *Owner, runToken string) {
	if o == nil {
		return
	}
	o.mu.Lock()
	o.activeRunToken = runToken
	o.mu.Unlock()
}

// EndRun clears runToken only when it is still the active run.
func EndRun(o *Owner, runToken string) {
	if o == nil {
		return
	}
	o.mu.Lock()
	if o.activeRunToken == runToken {
		o.activeRunToken = ""
	}
	o.mu.Unlock()
}

// Bind returns an execution context carrying owner's private authority.
// Runtime loop composition is the sole caller outside this contract package.
func Bind(o *Owner, exec ToolExecutionContext, binding BindSpec) ToolExecutionContext {
	if o == nil {
		return exec
	}
	exec.owner = o
	exec.binding = binding
	return exec
}

func (o *Owner) owns(runToken string) bool {
	if o == nil || runToken == "" {
		return false
	}
	o.mu.RLock()
	active := o.activeRunToken
	o.mu.RUnlock()
	return active != "" && active == runToken
}

// HasRuntimeOwner reports whether the context carries an owner capability.
// It does not imply that the originating run is still active.
func (exec ToolExecutionContext) HasRuntimeOwner() bool {
	return exec.owner != nil && exec.binding.RunToken != ""
}

// OwnedBy proves that this context belongs to owner's currently active run.
func (exec ToolExecutionContext) OwnedBy(owner *Owner) bool {
	return owner != nil && exec.HasRuntimeOwner() && exec.owner == owner && owner.owns(exec.binding.RunToken)
}

// IsRuntimeOwned reports whether the originating owner's run is still active.
func (exec ToolExecutionContext) IsRuntimeOwned() bool {
	return exec.OwnedBy(exec.owner)
}

// ApprovalEpoch returns the private run token used to bind tool permission
// preflight and execution. Unowned contexts return an empty value.
func (exec ToolExecutionContext) ApprovalEpoch() string {
	if !exec.IsRuntimeOwned() {
		return ""
	}
	return exec.binding.RunToken
}

// RuntimeIdentityMatches reports whether the active owner's complete bound
// identity still matches the context's exported identity fields. Empty values
// are compared exactly and are therefore valid for embedded session-less runs.
func (exec ToolExecutionContext) RuntimeIdentityMatches() bool {
	if !exec.IsRuntimeOwned() {
		return false
	}
	identity := exec.binding.Identity
	return identity.SessionID == exec.SessionID && identity.SessionProjectDir == exec.SessionProjectDir &&
		identity.ProjectRoot == exec.ProjectRoot && identity.CWD == exec.CWD
}

// ActiveRuntimeOwnerIdentity returns the immutable runtime identity only while
// the originating run is active and all exported identity fields still match.
func (exec ToolExecutionContext) ActiveRuntimeOwnerIdentity() (sessionID, sessionProjectDir, projectRoot, cwd string, ok bool) {
	if !exec.RuntimeIdentityMatches() {
		return "", "", "", "", false
	}
	identity := exec.binding.Identity
	if identity.SessionID == "" || identity.SessionID != exec.SessionID ||
		identity.SessionProjectDir != exec.SessionProjectDir || identity.ProjectRoot != exec.ProjectRoot || identity.CWD != exec.CWD {
		return "", "", "", "", false
	}
	return identity.SessionID, identity.SessionProjectDir, identity.ProjectRoot, identity.CWD, true
}

// ActiveReadEvidenceScope returns the runtime-owned namespace for file read
// evidence while the owner, actor, and visible context epoch still match.
func (exec ToolExecutionContext) ActiveReadEvidenceScope() (string, bool) {
	binding := exec.binding
	if !exec.IsRuntimeOwned() || binding.ReadEvidenceOwnerID == "" ||
		binding.ReadEvidenceActorID == "" || binding.ReadEvidenceActorID != exec.ActorID || binding.CurrentEvidenceEpoch == nil {
		return "", false
	}
	currentEpoch := binding.CurrentEvidenceEpoch()
	if currentEpoch == 0 || binding.ReadEvidenceEpoch != currentEpoch {
		return "", false
	}
	return fmt.Sprintf("%s\x1f%s\x1f%d", binding.ReadEvidenceOwnerID, binding.ReadEvidenceActorID, binding.ReadEvidenceEpoch), true
}

// ResolveSkillLoadedLedger queries the immutable skill evidence captured for
// this invocation. The boolean is false for externally constructed contexts.
func (exec ToolExecutionContext) ResolveSkillLoadedLedger(id string) (SkillLoadedLedgerState, bool) {
	if !exec.IsRuntimeOwned() || exec.binding.ResolveSkillLedger == nil {
		return SkillLoadedLedgerState{}, false
	}
	return exec.binding.ResolveSkillLedger(id), true
}

// SkillProjectGeneration returns the project authority pinned by the runtime.
func (exec ToolExecutionContext) SkillProjectGeneration() (uint64, bool) {
	if !exec.IsRuntimeOwned() || exec.binding.SkillProjectGeneration == 0 || exec.binding.ResolveSkillLedger == nil {
		return 0, false
	}
	return exec.binding.SkillProjectGeneration, true
}

// WithToolExecutionContext returns a child context carrying a detached
// execution snapshot.
func WithToolExecutionContext(ctx context.Context, exec ToolExecutionContext) context.Context {
	return context.WithValue(ctx, toolExecutionContextKey{}, exec.Clone())
}

// ToolExecutionContextFromContext retrieves a detached execution snapshot.
func ToolExecutionContextFromContext(ctx context.Context) (ToolExecutionContext, bool) {
	if ctx == nil {
		return ToolExecutionContext{}, false
	}
	exec, ok := ctx.Value(toolExecutionContextKey{}).(ToolExecutionContext)
	if !ok {
		return ToolExecutionContext{}, false
	}
	return exec.Clone(), true
}

// Clone returns a deep copy of mutable message and tool input data while
// preserving the private immutable runtime capabilities.
func (exec ToolExecutionContext) Clone() ToolExecutionContext {
	exec.Messages = CloneMessages(exec.Messages)
	exec.AssistantMessage = cloneMessage(exec.AssistantMessage)
	exec.ToolUse = cloneToolUseBlock(exec.ToolUse)
	return exec
}

// CloneMessages returns a deep copy suitable for an execution boundary.
func CloneMessages(messages []types.Message) []types.Message {
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
	out := message
	if continuation, ok := message.ValidatedProviderContinuation(); ok {
		out.AttachProviderContinuation(continuation)
	} else {
		out.ClearProviderContinuation()
	}
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
	case types.TextBlock, types.ThinkingBlock, types.ContentReplacementBlock, types.ToolReferenceBlock:
		return typed
	case types.ToolUseBlock:
		return cloneToolUseBlock(typed)
	case types.ToolResultBlock:
		typed.ContentBlocks = cloneContentBlocks(typed.ContentBlocks)
		typed.Data = cloneToolContextValue(typed.Data)
		typed.NewMessages = CloneMessages(typed.NewMessages)
		typed.Metadata = cloneStringMap(typed.Metadata)
		if typed.Usage != nil {
			usage := *typed.Usage
			typed.Usage = &usage
		}
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
