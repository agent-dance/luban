package registry

import (
	"context"
	"sync"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/approvalcommit"
	"github.com/agent-dance/luban/types"
)

// Registry holds registered tools and provides lookup and execution
type Registry struct {
	mu                 sync.RWMutex
	tools              map[string]types.Tool
	order              []string // preserve registration order
	toolGenerations    map[string]uint64
	nextToolGeneration uint64
	catalogGeneration  uint64
	modelToolProfile   ModelToolProfile
	runtimeProvider    types.ToolRuntimeContextProvider
	permissionGrantMu  sync.Mutex
	permissionGrants   map[string]permissionGrantRecord
}

// registryDispatchBinder is implemented by security-sensitive tools that may
// still be returned as their concrete type from Get. Binding is monotonic: once
// a tool has entered a Registry, its Execute method must require the private
// commit installed by ExecuteToolWithError. This keeps concrete-type SDK access
// from turning Get/All into an authorization bypass.
type registryDispatchBinder interface {
	RequireRegistryDispatch()
}

// SetRuntimeContextProvider connects visibility and permission checks to the
// same mutable session scope. The provider is queried per operation so cwd and
// feature changes do not require rebuilding the registry.
func (r *Registry) SetRuntimeContextProvider(provider types.ToolRuntimeContextProvider) {
	r.mu.Lock()
	r.runtimeProvider = provider
	r.mu.Unlock()
}

func (r *Registry) RuntimeContext() types.ToolRuntimeContext {
	r.mu.RLock()
	provider := r.runtimeProvider
	r.mu.RUnlock()
	if provider == nil {
		return types.ToolRuntimeContext{}
	}
	return provider.ToolRuntimeContext()
}

// HasRuntimeContextProvider reports whether RuntimeContext has a configured
// source. It lets snapshotting callers distinguish an intentionally empty
// runtime from the absence of a provider without discarding a pinned fallback.
func (r *Registry) HasRuntimeContextProvider() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.runtimeProvider != nil
}

// RuntimeContextWithinSessionBarrier snapshots the current runtime policy
// while the caller already owns the shared session publication barrier. The
// specialized provider path avoids recursively acquiring that same RWMutex,
// which can deadlock when a writer is queued. The bool is false when the
// registry has no runtime provider and the caller should retain its fallback.
func (r *Registry) RuntimeContextWithinSessionBarrier() (types.ToolRuntimeContext, bool) {
	r.mu.RLock()
	provider := r.runtimeProvider
	r.mu.RUnlock()
	if provider == nil {
		return types.ToolRuntimeContext{}, false
	}
	unbarriered, ok := provider.(interface {
		ToolRuntimeContextUnbarriered() types.ToolRuntimeContext
	})
	if !ok {
		// An arbitrary provider may acquire the same session barrier. Calling it
		// here would recursively RLock that barrier and can deadlock behind a
		// queued writer, so only explicitly barrier-safe providers are sampled.
		return types.ToolRuntimeContext{}, false
	}
	return unbarriered.ToolRuntimeContextUnbarriered(), true
}

// New creates a new empty tool registry
func New() *Registry {
	return &Registry{
		tools:            make(map[string]types.Tool),
		toolGenerations:  make(map[string]uint64),
		permissionGrants: make(map[string]permissionGrantRecord),
	}
}

// Register adds a tool to the registry. If a tool with the same name is already
// registered, it is silently replaced (useful for e.g. replacing AgentTool in a
// cloned registry with a depth-incremented version).
func (r *Registry) Register(tool types.Tool) {
	// Bind before publishing the tool. A concurrent Get can therefore never
	// observe a registered security-sensitive tool in its standalone mode.
	if binder, ok := tool.(registryDispatchBinder); ok {
		binder.RequireRegistryDispatch()
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	name := tool.Name()
	r.nextToolGeneration++
	if r.nextToolGeneration == 0 {
		r.nextToolGeneration++
	}
	r.toolGenerations[name] = r.nextToolGeneration
	r.catalogGeneration++
	if r.catalogGeneration == 0 {
		r.catalogGeneration++
	}
	if _, exists := r.tools[name]; exists {
		r.tools[name] = tool // update existing entry; order already recorded
		return
	}
	r.tools[name] = tool
	r.order = append(r.order, name)
}

// Unregister removes a tool by its canonical name.
func (r *Registry) Unregister(name string) bool {
	if r == nil || name == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.tools[name]; !ok {
		return false
	}
	delete(r.tools, name)
	delete(r.toolGenerations, name)
	r.catalogGeneration++
	if r.catalogGeneration == 0 {
		r.catalogGeneration++
	}
	kept := r.order[:0]
	for _, registered := range r.order {
		if registered != name {
			kept = append(kept, registered)
		}
	}
	r.order = kept
	return true
}

// Get retrieves a tool by name. Returns nil if not found.
func (r *Registry) Get(name string) types.Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.tools[name]
}

func (r *Registry) getWithGeneration(name string) (types.Tool, string, uint64) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool := r.tools[name]
	if tool == nil {
		return nil, "", 0
	}
	return tool, name, r.toolGenerations[name]
}

func (r *Registry) generationMatches(canonical string, generation uint64) bool {
	if r == nil || canonical == "" || generation == 0 {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.tools[canonical] != nil && r.toolGenerations[canonical] == generation
}

// All returns all registered tools in registration order
func (r *Registry) All() []types.Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tools := make([]types.Tool, 0, len(r.order))
	for _, name := range r.order {
		tools = append(tools, r.tools[name])
	}
	return tools
}

// Definitions returns API tool definitions for all registered tools
func (r *Registry) Definitions() []types.ToolDefinition {
	return types.ToDefinitions(r.All())
}

// ExecuteTool finds and executes a tool by name with the given input.
// Both infrastructure errors and business errors are returned as ToolResultBlock
// with IsError=true so the LLM always sees a result. Use ExecuteToolWithError
// if you need to distinguish infrastructure errors from business errors.
func (r *Registry) ExecuteTool(ctx context.Context, name string, input map[string]any) types.ToolResultBlock {
	result, err := r.ExecuteToolWithError(ctx, name, input)
	if err != nil {
		return types.ToolResultBlock{
			Type:    types.ContentTypeToolResult,
			Content: i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyRegistryToolExecuteFailed, name, err.Error()),
			IsError: true,
		}
	}
	return result
}

// ExecuteToolWithError finds and executes a tool by name, preserving the
// distinction between infrastructure errors (returned as error) and business
// errors (returned in ToolResultBlock.IsError). Callers can use this to
// abort the conversation turn on infrastructure failures while passing
// business errors back to the LLM.
func (r *Registry) ExecuteToolWithError(ctx context.Context, name string, input map[string]any) (types.ToolResultBlock, error) {
	tool, canonicalToolName, dispatchGeneration := r.getWithGeneration(name)
	if tool == nil {
		return types.ToolResultBlock{
			Type:    types.ContentTypeToolResult,
			Content: i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyRegistryToolUnknown, name, r.EnabledNames()),
			IsError: true,
		}, nil
	}
	if !r.IsToolEnabled(tool) {
		return types.ToolResultBlock{
			Type:    types.ContentTypeToolResult,
			Content: i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeToolDisabled, tool.Name()),
			IsError: true,
		}, nil
	}
	if backfiller, ok := tool.(interface {
		BackfillObservableInput(map[string]any) (map[string]any, error)
	}); ok {
		updated, err := backfiller.BackfillObservableInput(input)
		if err != nil {
			return types.ToolResultBlock{Type: types.ContentTypeToolResult, Content: err.Error(), IsError: true}, nil
		}
		if updated != nil {
			input = updated
		}
	}
	if err := types.ValidateToolInput(tool, input); err != nil {
		return types.ToolResultBlock{
			Type:    types.ContentTypeToolResult,
			Content: i18n.FormatToolInputValidationError(i18n.DetectOrLoadLanguage(), err),
			IsError: true,
		}, nil
	}
	pending, prepared := approvalcommit.FromContext(ctx)
	if !prepared {
		request := types.ToolPermissionRequest{}
		decision, err := r.CheckToolPermissions(ctx, tool.Name(), input, request)
		if err != nil {
			return types.ToolResultBlock{}, err
		}
		if decision.UpdatedInput != nil {
			input = decision.UpdatedInput
		}
		switch decision.Behavior {
		case types.PermissionBehaviorDeny, types.PermissionBehaviorAsk:
			r.RevokePermissionGrant(decision.PermissionGrant)
			message := decision.Message
			if message == "" {
				message = i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeToolPermissionRequired, tool.Name())
			}
			return types.ToolResultBlock{Type: types.ContentTypeToolResult, Content: message, IsError: true}, nil
		case types.PermissionBehaviorPassthrough:
			if tool.Name() == "Bash" {
				r.RevokePermissionGrant(decision.PermissionGrant)
				return types.ToolResultBlock{
					Type:    types.ContentTypeToolResult,
					Content: i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeToolPermissionRequired, tool.Name()),
					IsError: true,
				}, nil
			}
		}
		policyCode := decision.ExecutionPolicyCode
		if decision.PolicyDecision != nil {
			if policyCode == "" {
				policyCode = decision.PolicyDecision.Code
			}
		}
		executionGrant := r.authorizePermissionGrant(
			decision.PermissionGrant, tool.Name(), input, decision.PermissionBinding, policyCode,
		)
		pending = approvalcommit.Pending{Token: executionGrant, Binding: decision.PermissionBinding, PolicyCode: policyCode}
		ctx = approvalcommit.WithPending(ctx, pending)
	}
	record, validGrant := r.consumePermissionGrant(pending.Token, tool.Name(), input, pending.Binding, pending.PolicyCode, canonicalToolName, dispatchGeneration)
	if !validGrant {
		return types.ToolResultBlock{
			Type:    types.ContentTypeToolResult,
			Content: i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeToolPermissionRequired, tool.Name()),
			IsError: true,
		}, nil
	}
	ctx = approvalcommit.Bind(ctx, record.toolName, input, record.policyCode)
	result, err := tool.Execute(ctx, input)
	if err != nil {
		return types.ToolResultBlock{}, err
	}

	return types.MapToolResult(tool, result, ""), nil
}

// Names returns the names of all registered tools
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, len(r.order))
	copy(names, r.order)
	return names
}

// Count returns the number of registered tools
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tools)
}

// Clone creates a shallow copy of the registry. Tools are shared (not deep-copied),
// but the new registry can have tools replaced (e.g. a depth-incremented AgentTool)
// without affecting the original.
func (r *Registry) Clone() *Registry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	newReg := &Registry{
		tools:              make(map[string]types.Tool, len(r.tools)),
		order:              make([]string, len(r.order)),
		toolGenerations:    make(map[string]uint64, len(r.toolGenerations)),
		nextToolGeneration: r.nextToolGeneration,
		catalogGeneration:  r.catalogGeneration,
		modelToolProfile:   r.modelToolProfile,
		runtimeProvider:    r.runtimeProvider,
		permissionGrants:   make(map[string]permissionGrantRecord),
	}
	for k, v := range r.tools {
		newReg.tools[k] = v
	}
	for k, v := range r.toolGenerations {
		newReg.toolGenerations[k] = v
	}
	copy(newReg.order, r.order)
	return newReg
}

// NewDerived creates an empty registry that retains the parent's runtime
// context provider. Callers can then register a scoped subset of tools without
// dropping session-level visibility and permission policy.
func (r *Registry) NewDerived() *Registry {
	if r == nil {
		return New()
	}
	r.mu.RLock()
	provider := r.runtimeProvider
	profile := r.modelToolProfile
	r.mu.RUnlock()
	return &Registry{
		tools:            make(map[string]types.Tool),
		toolGenerations:  make(map[string]uint64),
		permissionGrants: make(map[string]permissionGrantRecord),
		modelToolProfile: profile,
		runtimeProvider:  provider,
	}
}
