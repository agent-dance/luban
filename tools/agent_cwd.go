package tools

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/permissions"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/sandbox"
	"github.com/agent-dance/luban/types"
)

var agentCWDWrappedTools = map[string]bool{
	"Read":         true,
	"Write":        true,
	"Edit":         true,
	"Glob":         true,
	"Grep":         true,
	"NotebookEdit": true,
	"FileDelete":   true,
	"FileMove":     true,
	"FileAppend":   true,
	"FileLink":     true,
}

var agentCWDPathKeys = []string{
	"file_path",
	"path",
	"source",
	"destination",
	"target",
	"link_path",
	"notebook_path",
}

type agentCWDToolWrapper struct {
	base string
	tool types.Tool
}

type agentRuntimeContextProvider struct {
	snapshot types.ToolRuntimeContext
	agentID  string
	cwd      string
	model    string
}

type agentRegistryRuntimeContextProvider struct {
	registry *registry.Registry
	cwd      string
}

type pinnedAgentPlanModeRuntime struct {
	mu      sync.RWMutex
	runtime types.ToolRuntimeContext
}

func newPinnedAgentPlanModeRuntime(runtime types.ToolRuntimeContext) *pinnedAgentPlanModeRuntime {
	return &pinnedAgentPlanModeRuntime{runtime: cloneToolRuntimeContext(runtime)}
}

func (r *pinnedAgentPlanModeRuntime) ToolRuntimeContext() types.ToolRuntimeContext {
	if r == nil {
		return types.ToolRuntimeContext{}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return cloneToolRuntimeContext(r.runtime)
}

func (r *pinnedAgentPlanModeRuntime) TransitionPermissionMode(mode string) error {
	return r.setPermissionMode(mode)
}

func (r *pinnedAgentPlanModeRuntime) RestorePermissionMode(mode string) error {
	return r.setPermissionMode(mode)
}

func (r *pinnedAgentPlanModeRuntime) setPermissionMode(mode string) error {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		return i18n.NewError(i18n.KeyToolPermissionModeRequired)
	}
	r.mu.Lock()
	r.runtime.PermissionMode = mode
	r.mu.Unlock()
	return nil
}

func (p agentRegistryRuntimeContextProvider) ToolRuntimeContext() types.ToolRuntimeContext {
	if p.registry == nil {
		return types.ToolRuntimeContext{}
	}
	runtime := p.registry.RuntimeContext()
	if cwd := strings.TrimSpace(p.cwd); cwd != "" {
		runtime.ProjectRoot = cwd
		runtime.AllowedDirs = []string{filepath.Clean(cwd)}
	}
	return runtime
}

func (p agentRuntimeContextProvider) ToolRuntimeContext() types.ToolRuntimeContext {
	runtime := cloneToolRuntimeContext(p.snapshot)
	runtime.AgentID = strings.TrimSpace(p.agentID)
	runtime.Interactive = false
	if cwd := strings.TrimSpace(p.cwd); cwd != "" {
		runtime.ProjectRoot = cwd
		// A worktree/custom-CWD agent is a separate filesystem scope. Retaining
		// the parent's checkout here would let an isolated child address it by
		// absolute path even though relative paths resolve inside the child.
		runtime.AllowedDirs = []string{filepath.Clean(cwd)}
	}
	if model := strings.TrimSpace(p.model); model != "" {
		runtime.Model = model
	}
	return runtime
}

func cloneToolRuntimeContext(runtime types.ToolRuntimeContext) types.ToolRuntimeContext {
	runtime.AllowedDirs = append([]string(nil), runtime.AllowedDirs...)
	runtime.Features = cloneStringBoolMap(runtime.Features)
	runtime.AllowedTools = cloneStringBoolMap(runtime.AllowedTools)
	runtime.DeniedTools = cloneStringBoolMap(runtime.DeniedTools)
	runtime.AllowedRules = append([]types.PermissionRuleValue(nil), runtime.AllowedRules...)
	runtime.DeniedRules = append([]types.PermissionRuleValue(nil), runtime.DeniedRules...)
	runtime.AskRules = append([]types.PermissionRuleValue(nil), runtime.AskRules...)
	return runtime
}

func cloneStringBoolMap(values map[string]bool) map[string]bool {
	if values == nil {
		return nil
	}
	cloned := make(map[string]bool, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

// agentCWDReadToolWrapper preserves Read's optional contract surfaces while
// resolving relative paths against the agent-local cwd. A generic wrapper
// cannot conditionally implement these interfaces without changing unrelated
// tools that return typed Data.
type agentCWDReadToolWrapper struct {
	*agentCWDToolWrapper
}

// agentCWDLifecycleToolWrapper keeps optional tool contracts visible while
// rewriting path inputs before hooks, permission checks, and execution.
type agentCWDLifecycleToolWrapper struct {
	*agentCWDToolWrapper
}

type agentCWDBashToolWrapper struct {
	*BashTool
}

func (w *agentCWDBashToolWrapper) CheckPermissions(ctx context.Context, input map[string]any, request types.ToolPermissionRequest) (types.ToolPermissionResult, error) {
	result, err := w.BashTool.CheckPermissions(ctx, input, request)
	if err == nil && result.Behavior == types.PermissionBehaviorAllow {
		result.Behavior = types.PermissionBehaviorPassthrough
	}
	return result, err
}

func (w *agentCWDLifecycleToolWrapper) rewritten(input map[string]any) map[string]any {
	return rewriteAgentCWDInput(w.base, w.tool.Name(), input)
}

func (w *agentCWDLifecycleToolWrapper) ToolContract() types.ToolContract {
	return w.tool.(types.ToolContractProvider).ToolContract()
}

func (w *agentCWDLifecycleToolWrapper) ToolMetadata(input map[string]any) types.ToolMetadata {
	return w.tool.(types.ToolMetadataProvider).ToolMetadata(w.rewritten(input))
}

func (w *agentCWDLifecycleToolWrapper) CheckPermissions(ctx context.Context, input map[string]any, request types.ToolPermissionRequest) (types.ToolPermissionResult, error) {
	result, err := w.tool.(types.ToolPermissionChecker).CheckPermissions(ctx, w.rewritten(input), request)
	if err == nil && result.Behavior == types.PermissionBehaviorAllow {
		result.Behavior = types.PermissionBehaviorPassthrough
	}
	return result, err
}

func (w *agentCWDLifecycleToolWrapper) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	return w.tool.(types.ToolResultMapper).MapToolResultToToolResultBlock(data, toolUseID)
}

func (w *agentCWDLifecycleToolWrapper) BackfillObservableInput(input map[string]any) (map[string]any, error) {
	rewritten := w.rewritten(input)
	if backfiller, ok := w.tool.(interface {
		BackfillObservableInput(map[string]any) (map[string]any, error)
	}); ok {
		return backfiller.BackfillObservableInput(rewritten)
	}
	return rewritten, nil
}

func (w *agentCWDLifecycleToolWrapper) NormalizeToolInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	rewritten := w.rewritten(input)
	if normalizer, ok := w.tool.(interface {
		NormalizeToolInput(context.Context, map[string]any) (map[string]any, error)
	}); ok {
		return normalizer.NormalizeToolInput(ctx, rewritten)
	}
	return rewritten, nil
}

func (w *agentCWDLifecycleToolWrapper) ToAutoClassifierInput(input map[string]any) string {
	return types.ToolAutoClassifierInput(w.tool, w.rewritten(input))
}

func (w *agentCWDLifecycleToolWrapper) SearchReadClassification(input map[string]any) types.ToolSearchReadClassification {
	return types.ToolSearchRead(w.tool, w.rewritten(input))
}

func supportsAgentCWDLifecycle(tool types.Tool) bool {
	if tool == nil {
		return false
	}
	_, contract := tool.(types.ToolContractProvider)
	_, metadata := tool.(types.ToolMetadataProvider)
	_, permissions := tool.(types.ToolPermissionChecker)
	_, mapper := tool.(types.ToolResultMapper)
	return contract && metadata && permissions && mapper
}

func (w *agentCWDReadToolWrapper) rewritten(input map[string]any) map[string]any {
	return rewriteAgentCWDInput(w.base, w.tool.Name(), input)
}

func (w *agentCWDReadToolWrapper) ToolContract() types.ToolContract {
	if provider, ok := w.tool.(types.ToolContractProvider); ok {
		return provider.ToolContract()
	}
	return types.ToolContract{}
}

func (w *agentCWDReadToolWrapper) ToolMetadata(input map[string]any) types.ToolMetadata {
	if provider, ok := w.tool.(types.ToolMetadataProvider); ok {
		return provider.ToolMetadata(w.rewritten(input))
	}
	return types.ToolMetadata{}
}

func (w *agentCWDReadToolWrapper) CheckPermissions(ctx context.Context, input map[string]any, request types.ToolPermissionRequest) (types.ToolPermissionResult, error) {
	checker, ok := w.tool.(types.ToolPermissionChecker)
	if !ok {
		return types.ToolPermissionResult{Behavior: types.PermissionBehaviorPassthrough}, nil
	}
	result, err := checker.CheckPermissions(ctx, w.rewritten(input), request)
	if err != nil {
		return result, err
	}
	// A path-local allow only proves that Read's own sandbox permits the
	// access. Sub-agents must still bubble the request through their inherited
	// permission handler so the inherited parent snapshot is preserved. Keep
	// the normalized input produced by Read while delegating
	// the final decision to the loop-level handler.
	if result.Behavior == types.PermissionBehaviorAllow {
		result.Behavior = types.PermissionBehaviorPassthrough
	}
	return result, nil
}

func (w *agentCWDReadToolWrapper) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	mapper, ok := w.tool.(types.ToolResultMapper)
	if !ok {
		return types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: toolUseID, Content: toolRuntimeText(i18n.KeyToolLegacyAAgentReadUnmappable), IsError: true}
	}
	return mapper.MapToolResultToToolResultBlock(data, toolUseID)
}

func (w *agentCWDReadToolWrapper) ToAutoClassifierInput(input map[string]any) string {
	return types.ToolAutoClassifierInput(w.tool, w.rewritten(input))
}

func (w *agentCWDReadToolWrapper) SearchReadClassification(input map[string]any) types.ToolSearchReadClassification {
	return types.ToolSearchRead(w.tool, w.rewritten(input))
}

func (w *agentCWDReadToolWrapper) BackfillObservableInput(input map[string]any) (map[string]any, error) {
	rewritten := w.rewritten(input)
	if provider, ok := w.tool.(interface {
		BackfillObservableInput(map[string]any) (map[string]any, error)
	}); ok {
		return provider.BackfillObservableInput(rewritten)
	}
	return rewritten, nil
}

func (w *agentCWDReadToolWrapper) NormalizeToolInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	rewritten := w.rewritten(input)
	if normalizer, ok := w.tool.(interface {
		NormalizeToolInput(context.Context, map[string]any) (map[string]any, error)
	}); ok {
		return normalizer.NormalizeToolInput(ctx, rewritten)
	}
	return w.BackfillObservableInput(rewritten)
}

func (w *agentCWDToolWrapper) Name() string        { return w.tool.Name() }
func (w *agentCWDToolWrapper) Description() string { return w.tool.Description() }
func (w *agentCWDToolWrapper) Schema() types.JSONSchema {
	return w.tool.Schema()
}

func (w *agentCWDToolWrapper) Aliases() []string {
	if aliased, ok := w.tool.(types.AliasedTool); ok {
		return aliased.Aliases()
	}
	return nil
}

func (w *agentCWDToolWrapper) BackfillObservableInput(input map[string]any) (map[string]any, error) {
	rewritten := rewriteAgentCWDInput(w.base, w.tool.Name(), input)
	if backfiller, ok := w.tool.(interface {
		BackfillObservableInput(map[string]any) (map[string]any, error)
	}); ok {
		return backfiller.BackfillObservableInput(rewritten)
	}
	return rewritten, nil
}

func (w *agentCWDToolWrapper) NormalizeToolInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	rewritten := rewriteAgentCWDInput(w.base, w.tool.Name(), input)
	if normalizer, ok := w.tool.(interface {
		NormalizeToolInput(context.Context, map[string]any) (map[string]any, error)
	}); ok {
		return normalizer.NormalizeToolInput(ctx, rewritten)
	}
	return rewritten, nil
}

func (w *agentCWDToolWrapper) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	rewritten := rewriteAgentCWDInput(w.base, w.tool.Name(), input)
	return w.tool.Execute(ctx, rewritten)
}

func rewriteAgentCWDInput(base, toolName string, input map[string]any) map[string]any {
	rewritten := cloneToolInput(input)
	if _, ok := rewritten["path"]; !ok && (toolName == "Glob" || toolName == "Grep") {
		rewritten["path"] = base
	}
	for _, key := range agentCWDPathKeys {
		raw, ok := rewritten[key]
		if !ok {
			continue
		}
		value, ok := raw.(string)
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" {
			if key == "path" && (toolName == "Glob" || toolName == "Grep") {
				rewritten[key] = base
			}
			continue
		}
		if filepath.IsAbs(value) {
			continue
		}
		rewritten[key] = filepath.Join(base, value)
	}
	return rewritten
}

func cloneToolInput(input map[string]any) map[string]any {
	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

func wrapRegistryForAgentCWD(reg *registry.Registry, cwd string) {
	if reg == nil {
		return
	}
	runtime := agentRegistryRuntimeContextProvider{registry: reg, cwd: cwd}
	planStates := make(map[*PlanState]*PlanState)
	readStates := make(map[*ReadFileState]*ReadFileState)
	for _, name := range reg.Names() {
		tool := reg.Get(name)
		if tool == nil {
			continue
		}
		tool = unwrapAgentCWDTool(tool)
		if name == "Bash" {
			if bashTool, ok := tool.(*BashTool); ok {
				if _, capabilityOK := sandbox.Snapshot(bashTool.Sandbox); !capabilityOK {
					reg.Unregister(name)
					continue
				}
				cloned := bashTool.cloneBashTool()
				cloned.CWD = cwd
				cloned.OriginalCWD = cwd
				cloned.AllowedDirs = []string{filepath.Clean(cwd)}
				cloned.PlanState = pinnedAgentPlanState(bashTool.PlanState, cwd, planStates)
				cloned.ReadFileState = pinnedAgentReadFileState(bashTool.ReadFileState, readStates)
				cloned.ForceSandbox = true
				reg.Register(&agentCWDBashToolWrapper{BashTool: cloned})
			}
			continue
		}
		if name == "PowerShell" {
			// Windows has no filesystem sandbox backend in this repository.
			// PowerShell can compute paths or launch arbitrary child processes, so
			// token validation cannot enforce a custom-CWD/worktree boundary.
			reg.Unregister(name)
			continue
		}
		if agentCWDWrappedTools[name] {
			tool = cloneAgentRuntimeFileTool(tool, runtime, []string{filepath.Clean(cwd)}, cwd, planStates, readStates)
			wrapper := &agentCWDToolWrapper{base: cwd, tool: tool}
			if name == "Read" {
				reg.Register(&agentCWDReadToolWrapper{agentCWDToolWrapper: wrapper})
			} else if supportsAgentCWDLifecycle(tool) {
				reg.Register(&agentCWDLifecycleToolWrapper{agentCWDToolWrapper: wrapper})
			} else {
				reg.Register(wrapper)
			}
		}
	}
}

func pinRegistryForAgentRuntime(reg *registry.Registry, runtime types.ToolRuntimeContextProvider, snapshot types.ToolRuntimeContext) {
	if reg == nil {
		return
	}
	effective := cloneToolRuntimeContext(snapshot)
	if runtime != nil {
		effective = cloneToolRuntimeContext(runtime.ToolRuntimeContext())
	}
	root := strings.TrimSpace(effective.ProjectRoot)
	allowed := append([]string(nil), effective.AllowedDirs...)
	bindInProcessAgentScopedTools(reg, effective.AgentID, root)
	planStates := make(map[*PlanState]*PlanState)
	readStates := make(map[*ReadFileState]*ReadFileState)
	planRuntime := newPinnedAgentPlanModeRuntime(effective)
	for _, name := range reg.Names() {
		tool := unwrapAgentCWDTool(reg.Get(name))
		if tool == nil {
			continue
		}
		switch typed := tool.(type) {
		case *BashTool:
			cloned := typed.cloneBashTool()
			cloned.CWD = root
			cloned.OriginalCWD = root
			cloned.AllowedDirs = append([]string(nil), allowed...)
			cloned.PermissionRules = agentPermissionRulesForRuntime(effective)
			cloned.PlanState = pinnedAgentPlanState(typed.PlanState, root, planStates)
			cloned.ReadFileState = pinnedAgentReadFileState(typed.ReadFileState, readStates)
			reg.Register(cloned)
			continue
		case *PowerShellTool:
			cloned := *typed
			cloned.CWD = root
			cloned.AllowedDirs = append([]string(nil), allowed...)
			cloned.PlanState = pinnedAgentPlanState(typed.PlanState, root, planStates)
			reg.Register(&cloned)
			continue
		case *AskUserQuestionTool:
			cloned := &AskUserQuestionTool{
				Reader: typed.Reader, Writer: typed.Writer,
				PlanState: pinnedAgentPlanState(typed.PlanState, root, planStates),
				Metadata:  cloneAskUserInput(typed.Metadata),
			}
			cloned.SetInteractionRequester(typed.interactionRequester())
			reg.Register(cloned)
			continue
		case *EnterPlanModeTool:
			state := pinnedAgentPlanState(typed.State, root, planStates)
			state.bindPermissionRuntime(planRuntime)
			reg.Register(&EnterPlanModeTool{State: state, Runtime: planRuntime})
			continue
		case *ExitPlanModeTool:
			state := pinnedAgentPlanState(typed.State, root, planStates)
			state.bindPermissionRuntime(planRuntime)
			reg.Register(&ExitPlanModeTool{
				State: state, Runtime: planRuntime, AgentID: typed.AgentID,
				PlanModeRequired: typed.PlanModeRequired, TeamManager: typed.TeamManager,
			})
			continue
		}
		if agentCWDWrappedTools[name] {
			reg.Register(cloneAgentRuntimeFileTool(tool, runtime, allowed, root, planStates, readStates))
		}
	}
}

func pinAgentRegistryPermissionRules(reg *registry.Registry, snapshot types.ToolRuntimeContext) {
	if reg == nil {
		return
	}
	for _, name := range reg.Names() {
		bash, ok := unwrapAgentCWDTool(reg.Get(name)).(*BashTool)
		if !ok || bash == nil {
			continue
		}
		cloned := bash.cloneBashTool()
		cloned.PermissionRules = agentPermissionRulesForRuntime(snapshot)
		reg.Register(cloned)
	}
}

func agentPermissionRulesForRuntime(snapshot types.ToolRuntimeContext) []permissions.Rule {
	rules := make([]permissions.Rule, 0, len(snapshot.DeniedRules)+len(snapshot.AskRules))
	appendRules := func(values []types.PermissionRuleValue, decision permissions.Decision) {
		for _, value := range values {
			toolName := strings.TrimSpace(value.ToolName)
			if toolName == "" {
				continue
			}
			rules = append(rules, permissions.Rule{
				Tool: toolName, Pattern: strings.TrimSpace(value.RuleContent), Decision: decision,
				Source: "subagent spawn-time permission snapshot",
			})
		}
	}
	appendRules(snapshot.DeniedRules, permissions.DecisionDeny)
	appendRules(snapshot.AskRules, permissions.DecisionAsk)
	// Allow rules are evaluated by the snapshot-aware parent handler after
	// bypass-immune mandatory checks; injecting them into Bash would let a
	// tool-local allow short-circuit those checks.
	return rules
}

func pinnedAgentPlanState(source *PlanState, projectRoot string, cached map[*PlanState]*PlanState) *PlanState {
	if source == nil {
		return nil
	}
	if cloned := cached[source]; cloned != nil {
		return cloned
	}
	source.mu.RLock()
	var enteredAt *time.Time
	if source.enteredAt != nil {
		value := *source.enteredAt
		enteredAt = &value
	}
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		root = source.projectRoot
	}
	cloned := &PlanState{
		active:                      source.active,
		planFile:                    source.planFile,
		allowedPrompts:              append([]PlanAllowedPrompt(nil), source.allowedPrompts...),
		enteredAt:                   enteredAt,
		projectRoot:                 root,
		stateFile:                   planStateFile(root),
		interviewPhase:              source.interviewPhase,
		prePlanState:                clonePlanSnapshot(source.prePlanState),
		hasExitedPlanMode:           source.hasExitedPlanMode,
		needsPlanModeExitAttachment: source.needsPlanModeExitAttachment,
		needsAutoModeExitAttachment: source.needsAutoModeExitAttachment,
	}
	source.mu.RUnlock()
	cached[source] = cloned
	return cloned
}

func unwrapAgentCWDTool(tool types.Tool) types.Tool {
	for tool != nil {
		switch wrapped := tool.(type) {
		case *agentCWDBashToolWrapper:
			tool = wrapped.BashTool
		case *agentCWDReadToolWrapper:
			if wrapped.agentCWDToolWrapper == nil {
				return nil
			}
			tool = wrapped.agentCWDToolWrapper.tool
		case *agentCWDLifecycleToolWrapper:
			if wrapped.agentCWDToolWrapper == nil {
				return nil
			}
			tool = wrapped.agentCWDToolWrapper.tool
		case *agentCWDToolWrapper:
			tool = wrapped.tool
		default:
			return tool
		}
	}
	return nil
}

func cloneAgentRuntimeFileTool(tool types.Tool, runtime types.ToolRuntimeContextProvider, allowed []string, projectRoot string, planStates map[*PlanState]*PlanState, readStates map[*ReadFileState]*ReadFileState) types.Tool {
	switch typed := tool.(type) {
	case *FileReadTool:
		return cloneAgentCWDReadTool(typed, runtime, allowed, pinnedAgentReadFileState(typed.ReadState, readStates))
	case *FileWriteTool:
		cloned := *typed
		cloned.AllowedDirs = allowed
		cloned.Runtime = runtime
		cloned.PlanState = pinnedAgentPlanState(typed.PlanState, projectRoot, planStates)
		cloned.ReadState = pinnedAgentReadFileState(typed.ReadState, readStates)
		return &cloned
	case *FileEditTool:
		cloned := *typed
		cloned.AllowedDirs = allowed
		cloned.Runtime = runtime
		cloned.PlanState = pinnedAgentPlanState(typed.PlanState, projectRoot, planStates)
		cloned.ReadState = pinnedAgentReadFileState(typed.ReadState, readStates)
		return &cloned
	case *NotebookEditTool:
		cloned := *typed
		cloned.AllowedDirs = allowed
		cloned.PlanState = pinnedAgentPlanState(typed.PlanState, projectRoot, planStates)
		cloned.ReadState = pinnedAgentReadFileState(typed.ReadState, readStates)
		return &cloned
	case *FileAppendTool:
		cloned := *typed
		cloned.AllowedDirs = allowed
		cloned.PlanState = pinnedAgentPlanState(typed.PlanState, projectRoot, planStates)
		return &cloned
	case *FileDeleteTool:
		cloned := *typed
		cloned.AllowedDirs = allowed
		cloned.PlanState = pinnedAgentPlanState(typed.PlanState, projectRoot, planStates)
		return &cloned
	case *FileMoveTool:
		cloned := *typed
		cloned.AllowedDirs = allowed
		cloned.PlanState = pinnedAgentPlanState(typed.PlanState, projectRoot, planStates)
		return &cloned
	case *LinkTool:
		cloned := *typed
		cloned.AllowedDirs = allowed
		cloned.PlanState = pinnedAgentPlanState(typed.PlanState, projectRoot, planStates)
		return &cloned
	case *GlobTool:
		return NewGlobTool(runtime)
	case *GrepTool:
		return NewGrepTool(runtime)
	default:
		return tool
	}
}

func cloneAgentCWDReadTool(source *FileReadTool, runtime types.ToolRuntimeContextProvider, allowed []string, readState *ReadFileState) *FileReadTool {
	if source == nil {
		return nil
	}
	source.listenerMu.Lock()
	defer source.listenerMu.Unlock()

	dynamicSkillDirs := make(map[string]struct{}, len(source.dynamicSkillDirs))
	for dir := range source.dynamicSkillDirs {
		dynamicSkillDirs[dir] = struct{}{}
	}
	return &FileReadTool{
		AllowedDirs:                  append([]string(nil), allowed...),
		Runtime:                      runtime,
		ModelProvider:                source.ModelProvider,
		ReadState:                    readState,
		SkillManager:                 source.SkillManager,
		TokenBudgetProvider:          source.TokenBudgetProvider,
		PreciseTokenCounter:          source.PreciseTokenCounter,
		DefaultReadingLimitsProvider: source.DefaultReadingLimitsProvider,
		ReadingLimitsProvider:        source.ReadingLimitsProvider,
		ToolResultsDirProvider:       source.ToolResultsDirProvider,
		AnalyticsHook:                source.AnalyticsHook,
		listeners:                    append([]FileReadListener(nil), source.listeners...),
		textListeners:                append([]FileReadTextListener(nil), source.textListeners...),
		richListeners:                append([]FileReadListenerRich(nil), source.richListeners...),
		dynamicSkillDirs:             dynamicSkillDirs,
	}
}

func pinnedAgentReadFileState(source *ReadFileState, cached map[*ReadFileState]*ReadFileState) *ReadFileState {
	if source == nil {
		return nil
	}
	if cloned := cached[source]; cloned != nil {
		return cloned
	}
	cloned := source.Clone()
	cached[source] = cloned
	return cloned
}
