package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/i18n"
	agentruntime "github.com/agent-dance/luban/internal/agent"
	"github.com/agent-dance/luban/internal/mcp/catalog"
	mcpmanager "github.com/agent-dance/luban/internal/mcp/manager"
	"github.com/agent-dance/luban/internal/runtime/compact"
	runtimescope "github.com/agent-dance/luban/internal/runtime/scope"
	runtimestore "github.com/agent-dance/luban/internal/store/runtime"
	settingsstore "github.com/agent-dance/luban/internal/store/settings"
	taskstore "github.com/agent-dance/luban/internal/store/tasks"
	toolcollaboration "github.com/agent-dance/luban/internal/tools/collaboration"
	toolconfig "github.com/agent-dance/luban/internal/tools/config"
	toolfile "github.com/agent-dance/luban/internal/tools/file"
	goaltool "github.com/agent-dance/luban/internal/tools/goal"
	toolinteraction "github.com/agent-dance/luban/internal/tools/interaction"
	toollsp "github.com/agent-dance/luban/internal/tools/lsp"
	toolmcp "github.com/agent-dance/luban/internal/tools/mcp"
	toolremote "github.com/agent-dance/luban/internal/tools/remote"
	toolschedule "github.com/agent-dance/luban/internal/tools/schedule"
	toolsearch "github.com/agent-dance/luban/internal/tools/search"
	toolshell "github.com/agent-dance/luban/internal/tools/shell"
	toolskill "github.com/agent-dance/luban/internal/tools/skill"
	tooltasks "github.com/agent-dance/luban/internal/tools/tasks"
	toolweb "github.com/agent-dance/luban/internal/tools/web"
	toolworktree "github.com/agent-dance/luban/internal/tools/worktree"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/sandbox"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

// RegistryDeps holds shared objects created during tool registration.
type RegistryDeps struct {
	Registry              *registry.Registry
	AgentTool             *agentruntime.AgentTool
	TaskCreateTool        *tooltasks.TaskCreateTool
	TaskUpdateTool        *tooltasks.TaskUpdateTool
	GetGoalTool           *goaltool.GetGoalTool
	CreateGoalTool        *goaltool.CreateGoalTool
	UpdateGoalTool        *goaltool.UpdateGoalTool
	TeamManager           *toolcollaboration.TeamManager
	Schedule              *toolschedule.Service
	SkillManager          *skills.Manager // shared skill manager (used by SkillTool and skill listing injection)
	SkillTool             *toolskill.SkillTool
	SkillOverrideStore    *skills.FileOverrideStore
	SkillSessionOverrides *skills.MemorySessionOverrideLayer
	PlanState             *toolinteraction.PlanState // shared plan state (used by TUI mode switching)
	AskUserQuestionTool   *toolinteraction.AskUserQuestionTool
	RuntimeScope          *runtimescope.RuntimeScope
	BashTool              *toolshell.BashTool
	PowerShellTool        *toolshell.PowerShellTool
	FileReadTool          *toolfile.FileReadTool
	FileWriteTool         *toolfile.FileWriteTool
	FileEditTool          *toolfile.FileEditTool
	NotebookEditTool      *toolfile.NotebookEditTool
	WebFetchTool          *toolweb.WebFetchTool
	WebSearchTool         *toolweb.WebSearchTool
	WebFetchCache         *toolweb.WebFetchCache
	BackgroundTasks       *agentruntime.BackgroundTaskManager
	ServiceMCP            *mcpmanager.Manager
	EnterWorktreeTool     *toolworktree.EnterWorktreeTool
	ExitWorktreeTool      *toolworktree.ExitWorktreeTool
	WorktreeManager       *toolworktree.WorktreeManager
	WorktreeRuntime       *toolworktree.WorktreeRuntime

	lspManager               *toollsp.LSPServerManager
	sessionMu                sync.RWMutex
	runtimePublishMu         sync.RWMutex
	scheduleMu               sync.Mutex
	activeSessionID          string
	sessionBound             bool
	scheduleStarted          bool
	skillInitErr             error
	mcpInitErr               error
	planInitErr              error
	scheduleInitErr          error
	scheduleExecutor         *appScheduleExecutor
	mcpSkillRuntime          *mcpSkillRuntimeBridge
	mcpListChangedUnregister func()
	mcpRuntimeStopOnce       sync.Once
	mcpRuntimeStopErr        error
}

// fileSkillActivator adapts the application-owned skill manager to the narrow
// generation-pinned capability consumed by the file domain.
type fileSkillActivator struct {
	manager *skills.Manager
}

func (a *fileSkillActivator) AddDirectoriesAtGeneration(generation uint64, dirs []string) error {
	if a == nil || a.manager == nil {
		return nil
	}
	return a.manager.AddDirectoriesAtGeneration(skills.ProjectSourceGeneration(generation), dirs)
}

func (a *fileSkillActivator) ActivateConditionalForPathAtGeneration(generation uint64, path string) error {
	if a == nil || a.manager == nil {
		return nil
	}
	return a.manager.ActivateConditionalForPathAtGeneration(skills.ProjectSourceGeneration(generation), path)
}

type preparedRegistrySessionContext struct {
	cwd                string
	mcpConfigs         map[string]catalog.MCPServerConfig
	skillProjectPlan   *skills.ProjectSourcePlan
	mcpSkillProjection *preparedMCPSkillProjection
	planState          *toolinteraction.PreparedPlanState
}

func newWorktreeLifecyclePublisher(repoRoot string) toolworktree.LifecyclePublisher {
	lifecycle := runtimestore.NewRuntimeLifecycle(repoRoot)
	return toolworktree.LifecyclePublisherFunc(func(ctx context.Context, event toolworktree.LifecycleEvent) error {
		return lifecycle.Publish(ctx, runtimestore.RuntimeLifecycleEvent{
			Type:     runtimestore.RuntimeLifecycleEventType(event.Type),
			EntityID: event.EntityID,
			ToolName: event.ToolName,
			Status:   event.Status,
			Payload: map[string]any{
				"repo_root":    event.RepoRoot,
				"branch":       event.Branch,
				"path":         event.Path,
				"created_here": event.CreatedHere,
			},
		})
	})
}

func (d *RegistryDeps) PostCompactMCPServers() []compact.MCPServerSnapshot {
	if d == nil || d.ServiceMCP == nil {
		return nil
	}
	states := d.ServiceMCP.Snapshot()
	out := make([]compact.MCPServerSnapshot, 0, len(states))
	for _, state := range states {
		if state.Type != mcpmanager.MCPStateConnected {
			continue
		}
		toolNames := make([]string, 0, len(state.Tools))
		for _, tool := range state.Tools {
			if name := strings.TrimSpace(tool.Name); name != "" {
				toolNames = append(toolNames, name)
			}
		}
		sort.Strings(toolNames)
		out = append(out, compact.MCPServerSnapshot{Name: state.Name, Tools: toolNames, Instructions: state.Instructions})
	}
	return out
}

// SetHookRunner atomically retargets every registry-owned hook consumer. Both
// initial bootstrap and session switching use this path so TaskCreate cannot
// retain hooks from the previous workspace.
func (d *RegistryDeps) SetHookRunner(runner *hooks.Runner) {
	if d == nil {
		return
	}
	d.runtimePublishMu.Lock()
	defer d.runtimePublishMu.Unlock()
	if d.AgentTool != nil {
		d.AgentTool.SetSessionHookRunner(runner)
	}
	if d.TaskCreateTool != nil {
		d.TaskCreateTool.SetHookRunner(runner)
	}
	if d.TaskUpdateTool != nil {
		d.TaskUpdateTool.SetHookRunner(runner)
	}
	if d.BackgroundTasks != nil {
		d.BackgroundTasks.SetHookRunner(runner)
	}
}

// SetGoalRuntime connects every model-facing goal tool to the same
// session-scoped persistence adapter used by slash commands and the query loop.
func (d *RegistryDeps) SetGoalRuntime(runtime goaltool.GoalRuntime) {
	if d == nil {
		return
	}
	d.runtimePublishMu.Lock()
	defer d.runtimePublishMu.Unlock()
	if d.GetGoalTool != nil {
		d.GetGoalTool.SetRuntime(runtime)
	}
	if d.CreateGoalTool != nil {
		d.CreateGoalTool.SetRuntime(runtime)
	}
	if d.UpdateGoalTool != nil {
		d.UpdateGoalTool.SetRuntime(runtime)
	}
}

// BindSessionIdentity moves tool-side session resolution away from entrypoint
// string closures and onto a synchronized owner before the first live switch.
func (d *RegistryDeps) BindSessionIdentity(sessionID string) {
	if d == nil {
		return
	}
	if d.AgentTool != nil {
		d.AgentTool.SetSessionBarrier(&d.runtimePublishMu)
	}
	if d.RuntimeScope != nil {
		d.RuntimeScope.SetSessionBarrier(&d.runtimePublishMu)
	}
	d.runtimePublishMu.Lock()
	defer d.runtimePublishMu.Unlock()
	d.sessionMu.Lock()
	if !d.sessionBound {
		d.activeSessionID = strings.TrimSpace(sessionID)
		d.sessionBound = true
	}
	d.sessionMu.Unlock()
	if d.RuntimeScope != nil {
		d.RuntimeScope.SetSessionIDFunc(d.currentSessionID)
	}
	d.publishSessionToolRuntime(currentSessionIDOr(d, sessionID), currentRuntimeCWD(d))
}

func (d *RegistryDeps) CurrentSessionID() string {
	if d == nil {
		return ""
	}
	d.runtimePublishMu.RLock()
	defer d.runtimePublishMu.RUnlock()
	return d.currentSessionID()
}

// PublishSessionID advances identity for a new conversation in the current
// workspace while preserving the already-published runtime snapshot.
func (d *RegistryDeps) PublishSessionID(sessionID string) {
	if d == nil {
		return
	}
	d.runtimePublishMu.Lock()
	d.sessionMu.Lock()
	d.activeSessionID = strings.TrimSpace(sessionID)
	d.sessionBound = true
	d.sessionMu.Unlock()
	d.publishSessionToolRuntime(strings.TrimSpace(sessionID), currentRuntimeCWD(d))
	d.runtimePublishMu.Unlock()
}

func (d *RegistryDeps) currentSessionID() string {
	d.sessionMu.RLock()
	defer d.sessionMu.RUnlock()
	return d.activeSessionID
}

// ApplySessionRuntime publishes the already-resumed target workspace to all
// registry-owned consumers. Agent and Team inherit prompt/hooks from one
// snapshot, so their reads cannot observe a mixed workspace pair.
func (d *RegistryDeps) ApplySessionRuntime(sessionID, cwd string, allowedDirs []string, system string, runner *hooks.Runner) error {
	if d == nil {
		return nil
	}
	prepared, err := d.PrepareSessionContext(cwd)
	if err != nil {
		return err
	}
	return d.applyPreparedSessionRuntime(sessionID, cwd, allowedDirs, system, runner, prepared)
}

func (d *RegistryDeps) applyPreparedSessionRuntime(sessionID, cwd string, allowedDirs []string, system string, runner *hooks.Runner, prepared *preparedRegistrySessionContext) error {
	return d.commitPreparedSessionRuntime(sessionID, cwd, allowedDirs, system, runner, prepared, nil)
}

// commitPreparedSessionRuntime revalidates and publishes the staged skill
// project owner in the same registry barrier as the adjacent fallible engine
// transition. If beforePublish fails, no registry consumer advances. If the
// staged settings revisions conflict, beforePublish is never called.
func (d *RegistryDeps) commitPreparedSessionRuntime(
	sessionID, cwd string,
	allowedDirs []string,
	system string,
	runner *hooks.Runner,
	prepared *preparedRegistrySessionContext,
	beforePublish func() error,
) error {
	return d.commitPreparedSessionRuntimeWithAfter(sessionID, cwd, allowedDirs, system, runner, prepared, beforePublish, nil)
}

func (d *RegistryDeps) commitPreparedSessionRuntimeWithAfter(
	sessionID, cwd string,
	allowedDirs []string,
	system string,
	runner *hooks.Runner,
	prepared *preparedRegistrySessionContext,
	beforePublish func() error,
	afterPublish func(),
) error {
	if d == nil || prepared == nil {
		return nil
	}
	d.runtimePublishMu.Lock()
	defer d.runtimePublishMu.Unlock()
	return d.commitPreparedSessionRuntimeLockedWithAfter(sessionID, cwd, allowedDirs, system, runner, prepared, beforePublish, afterPublish)
}

// commitPreparedSessionRuntimeLocked is the publication core for callers that
// already hold runtimePublishMu while repeating target preparation.
func (d *RegistryDeps) commitPreparedSessionRuntimeLocked(
	sessionID, cwd string,
	allowedDirs []string,
	system string,
	runner *hooks.Runner,
	prepared *preparedRegistrySessionContext,
	beforePublish func() error,
) error {
	return d.commitPreparedSessionRuntimeLockedWithAfter(sessionID, cwd, allowedDirs, system, runner, prepared, beforePublish, nil)
}

func (d *RegistryDeps) commitPreparedSessionRuntimeLockedWithAfter(
	sessionID, cwd string,
	allowedDirs []string,
	system string,
	runner *hooks.Runner,
	prepared *preparedRegistrySessionContext,
	beforePublish func() error,
	afterPublish func(),
) error {
	publish := func() {
		// This callback is executed beneath SkillManager.txnMu when a staged
		// project plan exists. Manager readers therefore remain blocked until the
		// matching active identity and every registry runtime consumer is B too.
		d.sessionMu.Lock()
		d.activeSessionID = strings.TrimSpace(sessionID)
		d.sessionBound = true
		d.sessionMu.Unlock()

		d.updateSessionContextAfterSkillCommit(cwd, allowedDirs, prepared)
		toolRuntime := types.ToolRuntimeContext{SessionID: strings.TrimSpace(sessionID), ProjectRoot: strings.TrimSpace(cwd), AllowedDirs: append([]string(nil), allowedDirs...)}
		if d.RuntimeScope != nil {
			toolRuntime = d.RuntimeScope.ToolRuntimeContextUnbarriered()
		}
		if d.AgentTool != nil {
			d.AgentTool.SetSessionRuntime(agentruntime.AgentSessionRuntime{System: system, HookRunner: runner, ToolRuntime: toolRuntime})
		}
		if d.TeamManager != nil {
			d.TeamManager.PublishRuntimeIdentity(toolcollaboration.RuntimeIdentity{
				SessionID:   toolRuntime.SessionID,
				ProjectRoot: toolRuntime.ProjectRoot,
				AgentID:     toolRuntime.AgentID,
				Model:       toolRuntime.Model,
			})
		}
		if d.TaskCreateTool != nil {
			d.TaskCreateTool.SetHookRunner(runner)
		}
		if d.TaskUpdateTool != nil {
			d.TaskUpdateTool.SetHookRunner(runner)
		}
		if d.BackgroundTasks != nil {
			d.BackgroundTasks.SetHookRunner(runner)
		}
		if afterPublish != nil {
			afterPublish()
		}
	}
	transition := func() error {
		if d.SkillManager == nil || prepared.skillProjectPlan == nil {
			return rootRuntimeError(i18n.KeyCommandSkillsUnavailable)
		}
		if err := d.SkillManager.CommitProjectSourcesWithAfter(prepared.skillProjectPlan, beforePublish, publish); err != nil {
			return err
		}
		return nil
	}
	if d.mcpSkillRuntime != nil {
		if err := d.mcpSkillRuntime.withPrepared(prepared.mcpSkillProjection, prepared.skillProjectPlan, transition); err != nil {
			return err
		}
	} else if err := transition(); err != nil {
		return err
	}
	return nil
}

// WebDomainConfig holds domain restriction settings for web tools (Task 7).
type WebDomainConfig struct {
	AllowedDomains        []string // nil = all allowed
	DisallowedDomains     []string // these domains always blocked
	SkipWebFetchPreflight bool     // enterprise setting equivalent to TS skipWebFetchPreflight
}

// ClearWebFetchCache is the session clear-cache lifecycle hook. It clears
// both raw URL content and allowed domain-info verdicts without stopping the
// registry-owned cache.
func (d *RegistryDeps) ClearWebFetchCache() {
	if d != nil && d.WebFetchTool != nil {
		d.WebFetchTool.ClearWebFetchCache()
	}
}

// StopWebFetchCache releases the cache purge goroutine during application
// shutdown. It is idempotent.
func (d *RegistryDeps) StopWebFetchCache() {
	if d != nil && d.WebFetchCache != nil {
		d.WebFetchCache.Stop()
	}
}

// StopSchedule releases schedule leadership and waits for context-aware
// executor calls during application and test teardown. The caller owns the
// shutdown deadline and the returned error.
func (d *RegistryDeps) StopSchedule(ctx context.Context) error {
	if d == nil || d.Schedule == nil {
		return nil
	}
	d.scheduleMu.Lock()
	defer d.scheduleMu.Unlock()
	if err := d.Schedule.Close(ctx); err != nil {
		return err
	}
	d.scheduleStarted = false
	return nil
}

// StartSchedule begins delivery only after Agent's system prompt, hooks, and
// permission handler have been installed by the application composition root.
func (d *RegistryDeps) StartSchedule(ctx context.Context) error {
	if d == nil || d.Schedule == nil {
		return nil
	}
	if ctx == nil {
		return i18n.WrapError(i18n.KeyToolScheduleStartFailed, context.Canceled)
	}
	d.scheduleMu.Lock()
	defer d.scheduleMu.Unlock()
	if d.scheduleStarted {
		return nil
	}
	if d.scheduleInitErr != nil {
		return d.scheduleInitErr
	}
	if d.scheduleExecutor != nil {
		if err := d.scheduleExecutor.Resume(ctx); err != nil {
			return i18n.WrapError(i18n.KeyToolScheduleEnqueueFailed, err, "resume")
		}
	}
	if err := d.Schedule.Start(ctx); err != nil {
		return i18n.WrapError(i18n.KeyToolScheduleStartFailed, err)
	}
	d.scheduleStarted = true
	return nil
}

// StopMCPRuntimeBridge unregisters manager lifecycle callbacks, breaks the
// manager/bridge closure cycle, and stops MCP transports. The caller owns the
// shutdown deadline and the returned error.
func (d *RegistryDeps) StopMCPRuntimeBridge(ctx context.Context) error {
	if d == nil {
		return nil
	}
	d.mcpRuntimeStopOnce.Do(func() {
		if d.mcpListChangedUnregister != nil {
			d.mcpListChangedUnregister()
		}
		if d.mcpSkillRuntime != nil {
			d.mcpSkillRuntime.close()
		}
		if d.ServiceMCP != nil {
			d.mcpRuntimeStopErr = d.ServiceMCP.Shutdown(ctx)
		}
	})
	return d.mcpRuntimeStopErr
}

// ShutdownLSP stops and reaps every language server owned by this registry.
// The caller owns the lifecycle deadline so LSP teardown can participate in a
// shared, context-bounded application shutdown.
func (d *RegistryDeps) ShutdownLSP(ctx context.Context) error {
	if d == nil || d.lspManager == nil {
		return nil
	}
	return d.lspManager.Shutdown(ctx)
}

func isEnvTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func isEnvDefinedFalsy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "0", "false", "no", "off":
		return true
	default:
		return false
	}
}

func isToolSearchEnabledOptimistic() bool {
	if isEnvTruthy(os.Getenv("LUBAN_CODE_DISABLE_EXPERIMENTAL_BETAS")) {
		return false
	}
	value := strings.TrimSpace(os.Getenv("ENABLE_TOOL_SEARCH"))
	if value == "" {
		return true
	}
	if isEnvTruthy(value) {
		return true
	}
	if isEnvDefinedFalsy(value) || strings.EqualFold(value, "auto:100") {
		return false
	}
	if strings.EqualFold(value, "auto") || strings.HasPrefix(strings.ToLower(value), "auto:") {
		return !strings.EqualFold(value, "auto:100")
	}
	return true
}

func isAgentSwarmsEnabled() bool {
	if os.Getenv("USER_TYPE") == "ant" {
		return true
	}
	if isEnvTruthy(os.Getenv("LUBAN_CODE_EXPERIMENTAL_AGENT_TEAMS")) {
		return true
	}
	for _, arg := range os.Args {
		if arg == "--agent-teams" {
			return true
		}
	}
	return false
}

func (d *RegistryDeps) PrepareSessionContext(cwd string) (*preparedRegistrySessionContext, error) {
	if d != nil && d.skillInitErr != nil {
		return nil, d.skillInitErr
	}
	if d != nil && d.mcpInitErr != nil {
		return nil, d.mcpInitErr
	}
	if d != nil && d.planInitErr != nil {
		return nil, d.planInitErr
	}
	if d != nil && d.scheduleInitErr != nil {
		return nil, d.scheduleInitErr
	}
	trimmedCWD := strings.TrimSpace(cwd)
	if trimmedCWD == "" {
		return nil, rootRuntimeError(i18n.KeyRootWorkspaceRequired)
	}
	if d != nil && d.hasCrossWorkspaceSkillConsumer(trimmedCWD) {
		return nil, rootRuntimeError(i18n.KeyRootAgentQueueActiveRun)
	}
	var preparedPlanState *toolinteraction.PreparedPlanState
	if d != nil && d.PlanState != nil {
		var err error
		preparedPlanState, err = d.PlanState.PrepareProjectRoot(trimmedCWD)
		if err != nil {
			return nil, err
		}
	}
	configs, err := loadWorkspaceMCPConfigs(trimmedCWD)
	if err != nil {
		return nil, err
	}
	if d == nil || d.SkillManager == nil {
		return nil, rootRuntimeError(i18n.KeyCommandSkillsUnavailable)
	}
	skillProjectPlan, err := d.SkillManager.PrepareProjectSources(trimmedCWD)
	if err != nil {
		return nil, err
	}
	var mcpSkillProjection *preparedMCPSkillProjection
	if d.mcpSkillRuntime != nil {
		mcpCtx, cancel := context.WithTimeout(context.Background(), mcpSkillCatalogRefreshTimeout)
		mcpSkillProjection = d.mcpSkillRuntime.prepare(mcpCtx, configs, sameWorkspaceRoot(currentRuntimeCWD(d), trimmedCWD))
		cancel()
		if mcpSkillProjection != nil {
			if err := d.SkillManager.StageMCPCatalogInputs(skillProjectPlan, mcpSkillProjection.inputs); err != nil {
				return nil, err
			}
		}
	}
	return &preparedRegistrySessionContext{
		cwd: trimmedCWD, mcpConfigs: configs, skillProjectPlan: skillProjectPlan,
		mcpSkillProjection: mcpSkillProjection, planState: preparedPlanState,
	}, nil
}

func (d *RegistryDeps) hasCrossWorkspaceSkillConsumer(targetRoot string) bool {
	if d == nil || d.BackgroundTasks == nil {
		return false
	}
	target, err := filepath.Abs(strings.TrimSpace(targetRoot))
	if err != nil {
		return true
	}
	target = filepath.Clean(target)
	for _, snapshot := range d.BackgroundTasks.InMemorySnapshots() {
		if !strings.EqualFold(strings.TrimSpace(snapshot.Type), "local_agent") ||
			!strings.EqualFold(strings.TrimSpace(snapshot.Status), "running") {
			continue
		}
		owner := strings.TrimSpace(snapshot.OwnerProjectRoot)
		if owner == "" || !filepath.IsAbs(owner) {
			return true
		}
		owner = filepath.Clean(owner)
		if runtime.GOOS == "windows" {
			if !strings.EqualFold(owner, target) {
				return true
			}
			continue
		}
		if owner != target {
			return true
		}
	}
	return false
}

func loadWorkspaceMCPConfigs(cwd string) (map[string]catalog.MCPServerConfig, error) {
	type candidate struct {
		path  string
		scope catalog.ConfigScope
	}
	candidates := []candidate{
		{path: filepath.Join(cwd, ".mcp.json"), scope: catalog.ScopeProject},
		{path: filepath.Join(cwd, brand.ConfigDirName, "settings.json"), scope: catalog.ScopeLocal},
	}
	configs := make(map[string]catalog.MCPServerConfig)
	for _, candidate := range candidates {
		parsed, err := catalog.ParseMCPConfigFile(candidate.path, catalog.ParseOptions{
			Scope: candidate.scope, ExpandVars: true, FilePath: candidate.path,
		})
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, rootRuntimeWrap(i18n.KeyRootMCPSettingsLoad, err, candidate.path)
		}
		if validation, ok := parsed.FirstFatalValidation(); ok {
			return nil, rootRuntimeError(i18n.KeyRootMCPSettingsInvalid, candidate.path, validation.Path, validation.Message)
		}
		for name, config := range parsed.Servers {
			configs[name] = config
		}
	}
	return configs, nil
}

func (d *RegistryDeps) UpdateSessionContext(cwd string, allowedDirs []string) error {
	if d == nil {
		return nil
	}
	prepared, err := d.PrepareSessionContext(cwd)
	if err != nil {
		return err
	}
	d.runtimePublishMu.Lock()
	defer d.runtimePublishMu.Unlock()
	return d.updateSessionContext(cwd, allowedDirs, prepared)
}

func (d *RegistryDeps) updateSessionContext(cwd string, allowedDirs []string, prepared *preparedRegistrySessionContext) error {
	trimmedCWD := strings.TrimSpace(cwd)
	if trimmedCWD == "" || prepared == nil {
		return nil
	}
	transition := func() error {
		if d.SkillManager == nil || prepared.skillProjectPlan == nil {
			return rootRuntimeError(i18n.KeyCommandSkillsUnavailable)
		}
		if err := d.SkillManager.ApplyProjectSources(prepared.skillProjectPlan); err != nil {
			return err
		}
		return d.updateSessionContextAfterSkillCommit(trimmedCWD, allowedDirs, prepared)
	}
	if d.mcpSkillRuntime != nil {
		return d.mcpSkillRuntime.withPrepared(prepared.mcpSkillProjection, prepared.skillProjectPlan, transition)
	}
	return transition()
}

// updateSessionContextAfterSkillCommit advances workspace-bound consumers
// after the Manager/store transaction has committed.
func (d *RegistryDeps) updateSessionContextAfterSkillCommit(cwd string, allowedDirs []string, prepared *preparedRegistrySessionContext) error {
	trimmedCWD := strings.TrimSpace(cwd)
	if trimmedCWD == "" || prepared == nil {
		return nil
	}
	if d.Schedule != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := d.Schedule.Rebind(ctx, trimmedCWD)
		cancel()
		if err != nil {
			return i18n.WrapError(i18n.KeyToolScheduleStartFailed, err)
		}
	}
	if d.RuntimeScope != nil {
		d.RuntimeScope.SetProjectRoot(trimmedCWD)
		d.RuntimeScope.SetAllowedDirs(allowedDirs)
	}
	if d.PlanState != nil {
		d.PlanState.ApplyPreparedProject(prepared.planState)
	}
	if d.BackgroundTasks != nil {
		d.BackgroundTasks.SetProjectRoot(trimmedCWD)
	}
	if d.BashTool != nil {
		d.BashTool.SetExecutionScope(trimmedCWD, allowedDirs)
	}
	if d.PowerShellTool != nil {
		d.PowerShellTool.SetExecutionScope(trimmedCWD, allowedDirs)
	}
	if d.FileReadTool != nil {
		d.FileReadTool.SetAllowedDirs(allowedDirs)
	}
	if d.FileWriteTool != nil {
		d.FileWriteTool.SetAllowedDirs(allowedDirs)
	}
	if d.FileEditTool != nil {
		d.FileEditTool.SetAllowedDirs(allowedDirs)
	}
	if d.NotebookEditTool != nil {
		d.NotebookEditTool.SetAllowedDirs(allowedDirs)
	}
	if d.ServiceMCP != nil {
		d.ServiceMCP.SetWorkingDirectory(trimmedCWD)
		d.ServiceMCP.ReplaceWorkspaceConfigs(prepared.mcpConfigs)
		toolmcp.RegisterDynamicMCPTools(d.Registry, d.ServiceMCP, nil)
	}
	d.scheduleMu.Lock()
	scheduleStarted := d.scheduleStarted
	d.scheduleMu.Unlock()
	if scheduleStarted && d.scheduleExecutor != nil {
		if err := d.scheduleExecutor.Resume(context.Background()); err != nil {
			return i18n.WrapError(i18n.KeyToolScheduleEnqueueFailed, err, "resume")
		}
	}
	d.publishSessionToolRuntime(d.currentSessionID(), trimmedCWD)
	return nil
}

func (d *RegistryDeps) publishSessionToolRuntime(sessionID, cwd string) {
	runtime := types.ToolRuntimeContext{SessionID: strings.TrimSpace(sessionID), ProjectRoot: strings.TrimSpace(cwd)}
	if d.RuntimeScope != nil {
		runtime = d.RuntimeScope.ToolRuntimeContextUnbarriered()
	}
	if d.AgentTool != nil {
		d.AgentTool.SetSessionToolRuntime(runtime)
	}
	if d.TeamManager != nil {
		d.TeamManager.PublishRuntimeIdentity(toolcollaboration.RuntimeIdentity{
			SessionID:   runtime.SessionID,
			ProjectRoot: runtime.ProjectRoot,
			AgentID:     runtime.AgentID,
			Model:       runtime.Model,
		})
	}
}

func currentSessionIDOr(d *RegistryDeps, fallback string) string {
	if d == nil {
		return strings.TrimSpace(fallback)
	}
	if current := d.currentSessionID(); current != "" {
		return current
	}
	return strings.TrimSpace(fallback)
}

func currentRuntimeCWD(d *RegistryDeps) string {
	if d == nil || d.RuntimeScope == nil {
		return ""
	}
	return d.RuntimeScope.ToolRuntimeContextUnbarriered().ProjectRoot
}

func isAgentTriggersEnabled() bool {
	return isEnvTruthy(os.Getenv("AGENT_TRIGGERS"))
}

func isCronRuntimeEnabled() bool {
	return isAgentTriggersEnabled() && !isEnvTruthy(os.Getenv("LUBAN_CODE_DISABLE_CRON"))
}

func isAgentTriggersRemoteEnabled() bool {
	return isEnvTruthy(os.Getenv("AGENT_TRIGGERS_REMOTE"))
}

func supportsProviderNativeWebSearch(providerName, model string) bool {
	switch strings.ToLower(strings.TrimSpace(providerName)) {
	case "anthropic", "firstparty", "first-party", "foundry":
		return true
	case "vertex":
		model = strings.ToLower(model)
		return strings.Contains(model, "claude-opus-4") ||
			strings.Contains(model, "claude-sonnet-4") ||
			strings.Contains(model, "claude-haiku-4")
	default:
		return false
	}
}

// SetupRegistry creates the tool registry with all built-in tools.
// AgentTool.System is not set here; the caller wires the system prompt after
// construction.
// pRef is a ProviderRef (implements Provider interface) so that sub-agents
// automatically use the latest provider after a runtime Swap().
// sb may be nil (or sandbox.NoopBackend{}) when sandboxing is disabled.
// webDomains may be nil when no domain restrictions are needed.
func SetupRegistry(pRef *provider.ProviderRef, cwd string, allowedDirs []string, sb sandbox.Backend, webDomains *WebDomainConfig, interactiveOpt ...bool) *RegistryDeps {
	reg := registry.New()
	interactive := true
	if len(interactiveOpt) > 0 {
		interactive = interactiveOpt[0]
	}
	runtimeScope := runtimescope.NewRuntimeScope(cwd, interactive)
	runtimeScope.SetAllowedDirs(allowedDirs)
	runtimeScope.SetFeatureGate(types.ToolFeatureTeams, isAgentSwarmsEnabled())
	runtimeScope.SetFeatureGate(types.ToolFeatureRemoteTrigger, isAgentTriggersRemoteEnabled())
	runtimeScope.SetFeatureGate(types.ToolFeatureCron, isCronRuntimeEnabled())
	runtimeScope.SetFeatureGate(types.ToolFeatureToolSearch, isToolSearchEnabledOptimistic())
	runtimeScope.SetFeatureGate(types.ToolFeaturePlanMode, true)
	runtimeScope.SetFeatureGate(types.ToolFeatureWorktree, true)
	// Go currently has no --brief/defaultView setting surface. The TS dev/test
	// opt-in env is therefore the explicit activation source; hosts can update
	// the RuntimeScope feature dynamically when they add another opt-in UI.
	runtimeScope.SetFeatureGate(types.ToolFeatureSendUserMessage, isEnvTruthy(os.Getenv("LUBAN_CODE_SEND_USER_MESSAGE")))
	applyProvider := func(p provider.Provider) {
		if p == nil {
			// Empty provider refs cannot execute the TS provider-native tool.
			runtimeScope.SetProviderInfo("", "")
			runtimeScope.SetFeatureGate(types.ToolFeatureWebSearch, false)
			return
		}
		runtimeScope.SetProviderInfo(p.Name(), p.ModelID())
		runtimeScope.SetFeatureGate(types.ToolFeatureWebSearch, supportsProviderNativeWebSearch(p.Name(), p.ModelID()))
	}
	if pRef != nil {
		applyProvider(pRef.Get())
		// Connect/auth flows can install the provider after registry bootstrap.
		// Read must immediately use the new model for PDF and reminder gating.
		pRef.OnChange(applyProvider)
	} else {
		applyProvider(nil)
	}
	reg.SetRuntimeContextProvider(runtimeScope)

	// Plan state — declared early because file/shell tools reference it.
	planState, planInitErr := toolinteraction.NewPlanState(cwd)
	backgroundTasks := agentruntime.NewBackgroundTaskManager(cwd)

	// Shared read-file state: used by Read/Edit/Write to coordinate
	// file_unchanged dedup and stale-write detection.
	readState := toolfile.NewReadFileState()

	// File & shell tools
	bashTool := &toolshell.BashTool{
		CWD:             cwd,
		Sandbox:         sb,
		PlanState:       planState,
		Background:      backgroundTasks,
		AllowedDirs:     append([]string(nil), allowedDirs...),
		FileMutations:   toolfile.NewFileMutationCoordinator(readState),
		OutputPersister: shellOutputPersister{},
	}
	powerShellTool := &toolshell.PowerShellTool{
		CWD: cwd, AllowedDirs: append([]string(nil), allowedDirs...),
		PlanState: planState, Background: backgroundTasks,
	}
	fileReadTool := &toolfile.FileReadTool{
		AllowedDirs: append([]string(nil), allowedDirs...),
		ReadState:   readState,
		Runtime:     runtimeScope,
		PreciseTokenCounter: func(ctx context.Context, content string) (int, error) {
			if pRef != nil {
				if current := pRef.Get(); current != nil {
					if counter, ok := current.(provider.TokenCountingProvider); ok {
						return counter.CountTokens(ctx, content)
					}
				}
			}
			return 0, fmt.Errorf("active provider does not expose token counting")
		},
		ToolResultsDirProvider: func() string {
			return filepath.Join(runtimeScope.ProjectRoot(), brand.ConfigDirName, "tool-results")
		},
	}
	fileWriteTool := &toolfile.FileWriteTool{
		AllowedDirs: allowedDirs,
		Runtime:     runtimeScope,
		PlanState:   planState,
		ReadState:   readState,
	}
	fileEditTool := &toolfile.FileEditTool{
		AllowedDirs: allowedDirs,
		Runtime:     runtimeScope,
		PlanState:   planState,
		ReadState:   readState,
	}
	if runtime.GOOS != "windows" {
		reg.Register(bashTool)
	} else if _, err := exec.LookPath("bash"); err == nil {
		reg.Register(bashTool)
	}
	if runtime.GOOS == "windows" {
		reg.Register(powerShellTool)
	}
	reg.Register(fileReadTool)
	reg.Register(fileWriteTool)
	reg.Register(fileEditTool)
	reg.Register(toolsearch.NewGlob(runtimeScope))
	reg.Register(toolsearch.NewGrep(runtimeScope))

	// Agent tool
	agentTool := &agentruntime.AgentTool{
		Provider:       pRef,
		Registry:       reg,
		Background:     backgroundTasks,
		NonInteractive: !interactive,
	}
	backgroundTasks.SetAgentSessionRestorer(agentTool.RestoreAgentSession)
	reg.Register(agentTool)

	// Task tools
	taskStore := taskstore.New(runtimeScope.TaskListID)
	taskCreateTool := tooltasks.NewTaskCreateTool(taskStore, runtimeScope)
	taskUpdateTool := tooltasks.NewTaskUpdateTool(taskStore, runtimeScope, agentruntime.VerificationAgentEnabled)
	taskUpdateTool.Runtime = runtimeScope
	reg.Register(taskCreateTool)
	reg.Register(tooltasks.NewTaskListTool(taskStore))
	reg.Register(taskUpdateTool)
	reg.Register(tooltasks.NewTaskGetTool(taskStore))
	taskBackground := newTaskBackgroundAdapter(backgroundTasks)
	reg.Register(tooltasks.NewTaskStopTool(taskBackground))
	reg.Register(tooltasks.NewTaskOutputTool(taskBackground))

	// Goal tools are always registered but receive their session runtime after
	// repository and session identity wiring is available.
	getGoalTool, createGoalTool, updateGoalTool := goaltool.NewGoalTools(nil)
	reg.Register(getGoalTool)
	reg.Register(createGoalTool)
	reg.Register(updateGoalTool)

	// Plan tools
	reg.Register(toolinteraction.NewEnterPlanModeTool(planState, runtimeScope))
	exitPlanModeTool := toolinteraction.NewExitPlanModeTool(planState, runtimeScope)
	reg.Register(exitPlanModeTool)

	// User interaction tools
	askUserQuestionTool := toolinteraction.NewAskUserQuestionTool(planState)
	reg.Register(askUserQuestionTool)
	sendUserMessageTool := toolinteraction.NewSendUserMessageTool(runtimeScope.ProjectRoot)
	reg.Register(sendUserMessageTool)

	// Web tools — WebFetch keeps its bounded response cache; WebSearch executes
	// only through the active provider's server-tool path.
	webFetchCache := toolweb.NewWebFetchCache()
	webFetch := toolweb.NewWebFetchTool(webFetchCache)
	webFetch.Summariser = newWebFetchProviderSummariser(pRef)
	webFetch.SkipWebFetchPreflight = isEnvTruthy(os.Getenv("LUBAN_CODE_SKIP_WEBFETCH_PREFLIGHT"))
	webSearch := toolweb.NewWebSearchTool()
	webSearch.SetWebSearchServerToolProvider(newWebSearchServerToolProvider(pRef))
	// Task 7: apply domain restrictions from CLI if configured.
	if webDomains != nil {
		webFetch.AllowedDomains = webDomains.AllowedDomains
		webFetch.DisallowedDomains = webDomains.DisallowedDomains
		webFetch.SkipWebFetchPreflight = webDomains.SkipWebFetchPreflight || webFetch.SkipWebFetchPreflight
		webSearch.AllowedDomains = webDomains.AllowedDomains
		webSearch.DisallowedDomains = webDomains.DisallowedDomains
	}
	reg.Register(webFetch)
	reg.Register(webSearch)

	// Scheduling tools and their durable Agent execution boundary.
	var scheduleService *toolschedule.Service
	var scheduleExecutor *appScheduleExecutor
	var scheduleInitErr error
	if isCronRuntimeEnabled() {
		scheduleExecutor = &appScheduleExecutor{agent: agentTool, background: backgroundTasks}
		scheduleService, scheduleInitErr = toolschedule.NewService(cwd, scheduleExecutor, appScheduleFireSink{}, runtimeScope.AgentID)
		if scheduleInitErr == nil {
			reg.Register(toolschedule.NewCreateTool(scheduleService))
			reg.Register(toolschedule.NewDeleteTool(scheduleService))
			reg.Register(toolschedule.NewListTool(scheduleService))
		} else {
			scheduleInitErr = i18n.WrapError(i18n.KeyToolScheduleStartFailed, scheduleInitErr)
		}
	}

	// Git worktree tools
	worktreeState := &toolworktree.WorktreeState{LifecycleFactory: newWorktreeLifecyclePublisher}
	worktreeManager := toolworktree.NewWorktreeManager()
	worktreeRuntime := toolworktree.NewWorktreeRuntime(cwd)
	worktreeHooks, worktreeHookErr := toolworktree.LoadWorktreeHookBridge(cwd)
	if worktreeHookErr != nil {
		log.Print(i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyRootLogWorktreeHooksFailed, worktreeHookErr))
		worktreeHooks = toolworktree.NewInMemoryWorktreeHookBridge()
	}
	enterWorktreeTool := &toolworktree.EnterWorktreeTool{
		State: worktreeState, Manager: worktreeManager, Runtime: worktreeRuntime,
		SessionID: runtimeScope.SessionID, HookBridge: worktreeHooks,
	}
	exitWorktreeTool := &toolworktree.ExitWorktreeTool{
		State: worktreeState, Manager: worktreeManager, Runtime: worktreeRuntime,
		SessionID: runtimeScope.SessionID, HookBridge: worktreeHooks,
	}
	reg.Register(enterWorktreeTool)
	reg.Register(exitWorktreeTool)

	// Config tool
	if os.Getenv("USER_TYPE") == "ant" {
		configStore := settingsstore.NewConfigStore()
		reg.Register(toolconfig.NewConfigTool(configStore))
	}

	// MCP tools
	serviceMCP := mcpmanager.NewManager(mcpmanager.WithWorkingDirectory(cwd))
	mcpSettingsPath := filepath.Join(cwd, brand.ConfigDirName, "settings.json")
	mcpInitErr := serviceMCP.LoadFromSettings(mcpSettingsPath)
	var validationErr *catalog.FatalConfigValidationError
	if errors.As(mcpInitErr, &validationErr) {
		validation := validationErr.Validation
		mcpInitErr = rootRuntimeError(i18n.KeyRootMCPSettingsInvalid, mcpSettingsPath, validation.Path, validation.Message)
	} else if mcpInitErr != nil {
		mcpInitErr = rootRuntimeWrap(i18n.KeyRootMCPSettingsLoad, mcpInitErr, mcpSettingsPath)
	}
	agentTool.SetMCPReadinessProbe(serviceMCP)
	reg.Register(toolmcp.NewListMcpResourcesTool(serviceMCP))
	reg.Register(toolmcp.NewReadMcpResourceTool(serviceMCP))
	if len(serviceMCP.ServerNames()) > 0 {
		mcpCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := toolmcp.RefreshDynamicMCPTools(mcpCtx, reg, serviceMCP, nil); err != nil {
			log.Print(i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyRootLogMCPRefreshFailed, err))
		}
		cancel()
	}

	// LSP tool — H4: inject per-registry state and manager to eliminate global singletons.
	var lspManager *toollsp.LSPServerManager
	if isEnvTruthy(os.Getenv("ENABLE_LSP_TOOL")) {
		lspState := toollsp.NewLSPState()
		lspManager = toollsp.NewLSPServerManager()
		reg.Register(&toollsp.LSPTool{State: lspState, Manager: lspManager})
	}

	// Skill tool — create the policy state once, then share the exact Manager
	// pointer with every discovery and execution consumer.
	skillTool := toolskill.NewSkillTool()
	skillSessionOverrides := skills.NewMemorySessionOverrideLayer()
	skillOverrideStore, skillInitErr := skills.NewFileOverrideStore(cwd, nil, skillSessionOverrides)
	if skillInitErr == nil {
		skillTool.Manager.SetOverrideStore(skillOverrideStore)
		skillInitErr = skillTool.Manager.ReplaceProjectSources(cwd)
	}
	if skillInitErr != nil {
		skillTool.Manager.SetOverrideStore(nil)
		skillOverrideStore = nil
	}
	agentTool.SkillManager = skillTool.Manager
	// Wire file lifecycle skill discovery to the shared manager so reads and
	// edits near a SKILL.md surface that skill into the active set.
	fileSkills := &fileSkillActivator{manager: skillTool.Manager}
	fileReadTool.SkillManager = fileSkills
	fileEditTool.SkillManager = fileSkills
	reg.Register(skillTool)

	// Team and messaging tools consume only the application-owned skill and
	// retained-agent ports. The durable team manager does not construct agent
	// runtimes or retain permission/provider state.
	teamManager := toolcollaboration.NewTeamManager(skillTool.Manager)
	initialToolRuntime := runtimeScope.ToolRuntimeContext()
	teamManager.PublishRuntimeIdentity(toolcollaboration.RuntimeIdentity{
		SessionID:   initialToolRuntime.SessionID,
		ProjectRoot: initialToolRuntime.ProjectRoot,
		AgentID:     initialToolRuntime.AgentID,
		Model:       initialToolRuntime.Model,
	})
	runtimeScope.SetTeamNameFunc(teamManager.CurrentTeamName)
	teamManager.SetTaskListChangeNotifier(taskStore.Invalidate)
	agentTool.Collaboration = toolcollaboration.NewAgentCollaborationSpawner(teamManager)
	retainedAgentAdapter := newRetainedAgentCollaborationAdapter(backgroundTasks, skillTool.Manager)
	var retainedMessenger toolcollaboration.RetainedAgentMessenger
	var retainedStopper toolcollaboration.RetainedAgentStopper
	if retainedAgentAdapter != nil {
		retainedMessenger = retainedAgentAdapter
		retainedStopper = retainedAgentAdapter
	}
	reg.Register(toolcollaboration.NewSendMessageTool(teamManager, retainedMessenger, retainedStopper))
	if isAgentSwarmsEnabled() {
		reg.Register(toolcollaboration.NewTeamCreateTool(teamManager))
		reg.Register(toolcollaboration.NewTeamDeleteTool(teamManager))
	}

	// Notebook editing
	notebookEditTool := &toolfile.NotebookEditTool{
		AllowedDirs: append([]string(nil), allowedDirs...),
		Runtime:     runtimeScope,
		PlanState:   planState,
		ReadState:   readState,
	}
	reg.Register(notebookEditTool)

	// Misc tools
	var toolSearch types.Tool
	if isToolSearchEnabledOptimistic() {
		toolSearch = toolsearch.NewToolSearch(reg, func() []toolsearch.MCPServerVisibilityState {
			return mcpVisibilityStatesFromSnapshot(serviceMCP.Snapshot())
		})
		reg.Register(toolSearch)
	}
	var mcpListChangedUnregister func()
	if invalidator, ok := toolSearch.(interface{ Invalidate() }); ok {
		mcpListChangedUnregister = toolmcp.RegisterMCPListChangedInvalidators(reg, serviceMCP, nil, invalidator)
	} else {
		mcpListChangedUnregister = toolmcp.RegisterMCPListChangedInvalidators(reg, serviceMCP, nil)
	}
	if isAgentTriggersRemoteEnabled() {
		reg.Register(&toolremote.Trigger{})
	}

	deps := &RegistryDeps{
		Registry:                 reg,
		AgentTool:                agentTool,
		TaskCreateTool:           taskCreateTool,
		TaskUpdateTool:           taskUpdateTool,
		GetGoalTool:              getGoalTool,
		CreateGoalTool:           createGoalTool,
		UpdateGoalTool:           updateGoalTool,
		TeamManager:              teamManager,
		Schedule:                 scheduleService,
		SkillManager:             skillTool.Manager,
		SkillTool:                skillTool,
		SkillOverrideStore:       skillOverrideStore,
		SkillSessionOverrides:    skillSessionOverrides,
		skillInitErr:             skillInitErr,
		mcpInitErr:               mcpInitErr,
		planInitErr:              planInitErr,
		scheduleInitErr:          scheduleInitErr,
		scheduleExecutor:         scheduleExecutor,
		PlanState:                planState,
		AskUserQuestionTool:      askUserQuestionTool,
		RuntimeScope:             runtimeScope,
		BashTool:                 bashTool,
		PowerShellTool:           powerShellTool,
		FileReadTool:             fileReadTool,
		FileWriteTool:            fileWriteTool,
		FileEditTool:             fileEditTool,
		NotebookEditTool:         notebookEditTool,
		WebFetchTool:             webFetch,
		WebSearchTool:            webSearch,
		WebFetchCache:            webFetchCache,
		BackgroundTasks:          backgroundTasks,
		ServiceMCP:               serviceMCP,
		EnterWorktreeTool:        enterWorktreeTool,
		ExitWorktreeTool:         exitWorktreeTool,
		WorktreeManager:          worktreeManager,
		WorktreeRuntime:          worktreeRuntime,
		lspManager:               lspManager,
		mcpListChangedUnregister: mcpListChangedUnregister,
	}
	deps.mcpSkillRuntime = newMCPSkillRuntimeBridge(deps.SkillManager, deps.ServiceMCP)
	if deps.mcpSkillRuntime != nil {
		mcpSkillCtx, cancel := context.WithTimeout(context.Background(), mcpSkillCatalogRefreshTimeout)
		_ = deps.mcpSkillRuntime.refresh(mcpSkillCtx)
		cancel()
	}
	worktreeRuntime.SetContextSwitcher(func(_ context.Context, nextCWD string) error {
		nextAllowed := make([]string, 0, len(allowedDirs)+1)
		nextAllowed = append(nextAllowed, nextCWD)
		for _, allowed := range allowedDirs {
			allowed = strings.TrimSpace(allowed)
			if allowed != "" && allowed != nextCWD {
				nextAllowed = append(nextAllowed, allowed)
			}
		}
		return deps.UpdateSessionContext(nextCWD, nextAllowed)
	})
	return deps
}

var webSearchQueryDeltaPattern = regexp.MustCompile(`"query"\s*:\s*"((?:[^"\\]|\\.)*)"`)

func newWebSearchServerToolProvider(ref *provider.ProviderRef) toolweb.WebSearchServerToolProvider {
	return toolweb.WebSearchServerToolFunc(func(ctx context.Context, req toolweb.WebSearchServerToolRequest) (toolweb.WebSearchServerToolResponse, error) {
		if ref == nil {
			return toolweb.WebSearchServerToolResponse{}, toolweb.ErrWebSearchServerToolUnavailable
		}
		current := ref.Get()
		if current == nil || !supportsProviderNativeWebSearch(current.Name(), current.ModelID()) {
			return toolweb.WebSearchServerToolResponse{}, toolweb.ErrWebSearchServerToolUnavailable
		}

		started := time.Now()
		stream, err := current.CreateStream(ctx, provider.Params{
			Model:     current.ModelID(),
			MaxTokens: 4096,
			System:    "You are an assistant for performing a web search tool use",
			Messages:  []types.Message{types.UserMessage("Perform a web search for the query: " + req.Query)},
			ExtraToolSchemas: []types.ServerToolDefinition{{
				Type:           toolweb.WebSearchServerToolName,
				Name:           "web_search",
				AllowedDomains: append([]string(nil), req.AllowedDomains...),
				BlockedDomains: append([]string(nil), req.BlockedDomains...),
				MaxUses:        req.MaxUses,
			}},
		})
		if err != nil {
			return toolweb.WebSearchServerToolResponse{}, err
		}
		if stream == nil {
			return toolweb.WebSearchServerToolResponse{}, i18n.NewError(i18n.KeyRegistryWebSearchProviderNilStream)
		}

		response := toolweb.WebSearchServerToolResponse{}
		serverToolIDs := make(map[int]string)
		serverToolJSON := make(map[int]*strings.Builder)
		queries := make(map[string]string)
		textBlocks := make(map[int]*strings.Builder)
		for {
			select {
			case <-ctx.Done():
				return toolweb.WebSearchServerToolResponse{}, ctx.Err()
			case event, ok := <-stream:
				if !ok {
					response.DurationMs = time.Since(started).Milliseconds()
					return response, nil
				}
				mergeWebSearchUsage(&response.Usage, event.Usage)
				switch event.Type {
				case types.EventContentBlockStart:
					if event.ContentBlock == nil {
						continue
					}
					switch event.ContentBlock.Type {
					case types.ContentTypeServerToolUse:
						serverToolIDs[event.Index] = event.ContentBlock.ID
						serverToolJSON[event.Index] = &strings.Builder{}
					case types.ContentTypeWebSearchToolResult:
						result, parseErr := parseWebSearchServerResult(event.ContentBlock.RawJSON)
						if parseErr != nil {
							return toolweb.WebSearchServerToolResponse{}, parseErr
						}
						if result.ToolUseID == "" {
							result.ToolUseID = event.ContentBlock.ToolUseID
						}
						response.ResultBlocks = append(response.ResultBlocks, result)
						resultCopy := result
						response.Entries = append(response.Entries, toolweb.WebSearchServerToolEntry{Result: &resultCopy})
						response.Citations = append(response.Citations, string(event.ContentBlock.RawJSON))
						actualQuery := queries[result.ToolUseID]
						if actualQuery == "" {
							actualQuery = req.Query
						}
						if req.OnProgress != nil {
							req.OnProgress(toolweb.WebSearchProgressEvent{
								Type:        "search_results_received",
								ToolUseID:   result.ToolUseID,
								Query:       actualQuery,
								ResultCount: len(result.Results),
							})
						}
					case types.ContentTypeText:
						textBlocks[event.Index] = &strings.Builder{}
					}
				case types.EventContentBlockDelta:
					if event.Delta == nil {
						continue
					}
					switch event.Delta.Type {
					case "input_json_delta":
						builder := serverToolJSON[event.Index]
						if builder == nil {
							builder = &strings.Builder{}
							serverToolJSON[event.Index] = builder
						}
						builder.WriteString(event.Delta.PartialJSON)
						toolUseID := serverToolIDs[event.Index]
						if query, ok := webSearchQueryFromPartialJSON(builder.String()); ok && queries[toolUseID] != query {
							queries[toolUseID] = query
							if req.OnProgress != nil {
								req.OnProgress(toolweb.WebSearchProgressEvent{Type: "query_update", ToolUseID: toolUseID, Query: query})
							}
						}
					case "text_delta":
						if builder := textBlocks[event.Index]; builder != nil {
							builder.WriteString(event.Delta.Text)
						}
					}
				case types.EventContentBlockStop:
					if builder := textBlocks[event.Index]; builder != nil {
						if text := strings.TrimSpace(builder.String()); text != "" {
							response.Entries = append(response.Entries, toolweb.WebSearchServerToolEntry{Text: text})
						}
						delete(textBlocks, event.Index)
					}
				case types.EventError:
					if event.Error != nil {
						return toolweb.WebSearchServerToolResponse{}, event.Error
					}
					return toolweb.WebSearchServerToolResponse{}, i18n.NewError(i18n.KeyRegistryWebSearchProviderStreamFailed)
				}
			}
		}
	})
}

func webSearchQueryFromPartialJSON(partial string) (string, bool) {
	match := webSearchQueryDeltaPattern.FindStringSubmatch(partial)
	if len(match) != 2 {
		return "", false
	}
	var query string
	if err := json.Unmarshal([]byte(`"`+match[1]+`"`), &query); err != nil {
		return "", false
	}
	return query, true
}

func parseWebSearchServerResult(raw json.RawMessage) (toolweb.WebSearchServerToolResult, error) {
	var envelope struct {
		ToolUseID string          `json:"tool_use_id"`
		Content   json.RawMessage `json:"content"`
	}
	if len(raw) == 0 {
		return toolweb.WebSearchServerToolResult{}, i18n.NewError(i18n.KeyRegistryWebSearchResultMissingRawContent)
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return toolweb.WebSearchServerToolResult{}, i18n.WrapError(i18n.KeyRegistryWebSearchDecodeResultBlock, err)
	}
	result := toolweb.WebSearchServerToolResult{ToolUseID: envelope.ToolUseID}
	trimmed := strings.TrimSpace(string(envelope.Content))
	if strings.HasPrefix(trimmed, "[") {
		var hits []struct {
			Title     string `json:"title"`
			URL       string `json:"url"`
			PageAge   string `json:"page_age"`
			CitedText string `json:"cited_text"`
		}
		if err := json.Unmarshal(envelope.Content, &hits); err != nil {
			return toolweb.WebSearchServerToolResult{}, i18n.WrapError(i18n.KeyRegistryWebSearchDecodeHits, err)
		}
		for _, hit := range hits {
			result.Results = append(result.Results, toolweb.WebSearchResult{
				Title: hit.Title, URL: hit.URL, PageAge: hit.PageAge, CitedText: hit.CitedText,
			})
		}
		return result, nil
	}
	var failure struct {
		ErrorCode string `json:"error_code"`
	}
	if err := json.Unmarshal(envelope.Content, &failure); err != nil {
		return toolweb.WebSearchServerToolResult{}, i18n.WrapError(i18n.KeyRegistryWebSearchDecodeError, err)
	}
	result.ErrorCode = failure.ErrorCode
	return result, nil
}

func mergeWebSearchUsage(dst *types.Usage, src *types.Usage) {
	if dst == nil || src == nil {
		return
	}
	if src.InputTokens != 0 {
		dst.InputTokens = src.InputTokens
	}
	if src.OutputTokens != 0 {
		dst.OutputTokens = src.OutputTokens
	}
	if src.CacheCreationInputTokens != 0 {
		dst.CacheCreationInputTokens = src.CacheCreationInputTokens
	}
	if src.CacheReadInputTokens != 0 {
		dst.CacheReadInputTokens = src.CacheReadInputTokens
	}
	if src.ServerToolUse.WebSearchRequests != 0 {
		dst.ServerToolUse.WebSearchRequests = src.ServerToolUse.WebSearchRequests
	}
	if src.ServerToolUse.WebFetchRequests != 0 {
		dst.ServerToolUse.WebFetchRequests = src.ServerToolUse.WebFetchRequests
	}
}

func newWebFetchProviderSummariser(ref *provider.ProviderRef) toolweb.SummariserClient {
	return toolweb.SummariserFunc(func(ctx context.Context, req toolweb.SummariserRequest) (string, error) {
		if ref == nil {
			return "", i18n.NewError(i18n.KeyRegistryWebFetchSecondaryProviderUnavailable)
		}
		current := ref.Get()
		if current == nil {
			return "", i18n.NewError(i18n.KeyRegistryWebFetchSecondaryProviderUnavailable)
		}
		stream, err := current.CreateStream(ctx, provider.Params{
			Model:     webFetchSmallFastModel(current),
			MaxTokens: req.MaxTokens,
			System:    req.SystemPrompt,
			Messages:  []types.Message{types.UserMessage(req.UserPrompt)},
		})
		if err != nil {
			return "", err
		}
		if stream == nil {
			return "", i18n.NewError(i18n.KeyRegistryWebFetchSecondaryModelNilStream)
		}
		var result strings.Builder
		for {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case event, ok := <-stream:
				if !ok {
					if err := ctx.Err(); err != nil {
						return "", err
					}
					return result.String(), nil
				}
				switch event.Type {
				case types.EventContentBlockDelta:
					if event.Delta != nil && event.Delta.Type == "text_delta" {
						result.WriteString(event.Delta.Text)
					}
				case types.EventError:
					if event.Error != nil {
						return "", event.Error
					}
					return "", i18n.NewError(i18n.KeyRegistryWebFetchSecondaryModelStreamFailed)
				}
			}
		}
	})
}

func webFetchSmallFastModel(current provider.Provider) string {
	if override := strings.TrimSpace(os.Getenv("ANTHROPIC_SMALL_FAST_MODEL")); override != "" {
		return override
	}
	if current == nil {
		return ""
	}
	providerName := provider.CanonicalProviderName(current.Name())
	for _, model := range provider.DefaultCatalog().ListByProvider(providerName) {
		if strings.Contains(strings.ToLower(model.ID), "haiku") {
			return model.ID
		}
	}
	// Non-Claude backends may not expose a separate small model. Their active
	// model remains the only provider-compatible secondary-model adapter.
	return current.ModelID()
}

func mcpVisibilityStatesFromSnapshot(snapshot []mcpmanager.MCPServerConnection) []toolsearch.MCPServerVisibilityState {
	out := make([]toolsearch.MCPServerVisibilityState, 0, len(snapshot))
	for _, server := range snapshot {
		if server.Type == mcpmanager.MCPStateConnected {
			continue
		}
		out = append(out, toolsearch.MCPServerVisibilityState{
			Name:                 server.Name,
			State:                string(server.Type),
			ReconnectAttempt:     server.ReconnectAttempt,
			MaxReconnectAttempts: server.MaxReconnectAttempts,
			Error:                server.Error,
		})
	}
	return out
}
