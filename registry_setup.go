package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/coordinator"
	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/sandbox"
	svcmcp "github.com/agent-dance/luban/services/mcp"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/tools"
	"github.com/agent-dance/luban/types"
)

// RegistryDeps holds shared objects created during tool registration.
type RegistryDeps struct {
	Registry              *registry.Registry
	AgentTool             *tools.AgentTool
	TaskCreateTool        *tools.TaskCreateTool
	TaskUpdateTool        *tools.TaskUpdateTool
	GetGoalTool           *tools.GetGoalTool
	CreateGoalTool        *tools.CreateGoalTool
	UpdateGoalTool        *tools.UpdateGoalTool
	TeamManager           *tools.TeamManager
	CronStore             *tools.CronStore // caller must defer .Stop()
	SkillManager          *skills.Manager  // shared skill manager (used by SkillTool and skill listing injection)
	SkillTool             *tools.SkillTool
	SkillOverrideStore    *skills.FileOverrideStore
	SkillSessionOverrides *skills.MemorySessionOverrideLayer
	PlanState             *tools.PlanState // shared plan state (used by TUI mode switching)
	AskUserQuestionTool   *tools.AskUserQuestionTool
	RuntimeScope          *tools.RuntimeScope
	BashTool              *tools.BashTool
	PowerShellTool        *tools.PowerShellTool
	FileReadTool          *tools.FileReadTool
	FileWriteTool         *tools.FileWriteTool
	FileEditTool          *tools.FileEditTool
	NotebookEditTool      *tools.NotebookEditTool
	GlobTool              *tools.GlobTool
	GrepTool              *tools.GrepTool
	WebFetchTool          *tools.WebFetchTool
	WebSearchTool         *tools.WebSearchTool
	WebFetchCache         *tools.WebFetchCache
	BackgroundTasks       *tools.BackgroundTaskManager
	TodoStore             *tools.TodoStore
	MCPManager            *tools.MCPManager // legacy shim until task_08 migrates tool registration
	ServiceMCP            *svcmcp.Manager
	EnterWorktreeTool     *tools.EnterWorktreeTool
	ExitWorktreeTool      *tools.ExitWorktreeTool
	WorktreeManager       *tools.WorktreeManager
	WorktreeRuntime       *tools.WorktreeRuntime

	sessionMu                sync.RWMutex
	runtimePublishMu         sync.RWMutex
	activeSessionID          string
	sessionBound             bool
	skillInitErr             error
	mcpSkillRuntime          *mcpSkillRuntimeBridge
	mcpListChangedUnregister func()
	mcpRuntimeStopOnce       sync.Once
}

type preparedRegistrySessionContext struct {
	cwd                string
	mcpConfigs         map[string]svcmcp.MCPServerConfig
	skillProjectPlan   *skills.ProjectSourcePlan
	mcpSkillProjection *preparedMCPSkillProjection
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
	if d.TeamManager != nil {
		d.TeamManager.SetSessionHookRunner(runner)
	}
	if d.BackgroundTasks != nil {
		d.BackgroundTasks.SetHookRunner(runner)
	}
}

// SetGoalRuntime connects every model-facing goal tool to the same
// session-scoped persistence adapter used by slash commands and the query loop.
func (d *RegistryDeps) SetGoalRuntime(runtime tools.GoalRuntime) {
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
	if d.TeamManager != nil {
		d.TeamManager.SetSessionBarrier(&d.runtimePublishMu)
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
	if d.TeamManager != nil {
		d.TeamManager.SetSessionIDResolver(d.currentSessionID)
	}
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
			d.AgentTool.SetSessionRuntime(tools.AgentSessionRuntime{System: system, HookRunner: runner, ToolRuntime: toolRuntime})
		}
		if d.TeamManager != nil {
			d.TeamManager.SetSessionRuntime(tools.TeamSessionRuntime{System: system, HookRunner: runner, SessionID: strings.TrimSpace(sessionID), CWD: strings.TrimSpace(cwd), ToolRuntime: toolRuntime})
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
		if prepared.skillProjectPlan != nil && d.SkillManager != nil {
			if err := d.SkillManager.CommitProjectSourcesWithAfter(prepared.skillProjectPlan, beforePublish, publish); err != nil {
				return err
			}
		} else {
			if beforePublish != nil {
				if err := beforePublish(); err != nil {
					return err
				}
			}
			publish()
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

// StopMCPRuntimeBridge unregisters manager lifecycle callbacks and breaks the
// manager/bridge closure cycle during process or test teardown.
func (d *RegistryDeps) StopMCPRuntimeBridge() {
	if d == nil {
		return
	}
	d.mcpRuntimeStopOnce.Do(func() {
		if d.mcpListChangedUnregister != nil {
			d.mcpListChangedUnregister()
		}
		if d.mcpSkillRuntime != nil {
			d.mcpSkillRuntime.close()
		}
	})
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

func hasEmbeddedSearchTools() bool {
	if !isEnvTruthy(os.Getenv("EMBEDDED_SEARCH_TOOLS")) {
		return false
	}
	switch os.Getenv("CLAUDE_CODE_ENTRYPOINT") {
	case "sdk-ts", "sdk-py", "sdk-cli", "local-agent":
		return false
	default:
		return true
	}
}

func isToolSearchEnabledOptimistic() bool {
	if isEnvTruthy(os.Getenv("CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS")) {
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
	if isEnvTruthy(os.Getenv("CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS")) {
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
	trimmedCWD := strings.TrimSpace(cwd)
	if trimmedCWD == "" {
		return nil, rootRuntimeError(i18n.KeyRootWorkspaceRequired)
	}
	if d != nil && d.hasCrossWorkspaceSkillConsumer(trimmedCWD) {
		return nil, rootRuntimeError(i18n.KeyRootAgentQueueActiveRun)
	}
	configs, err := loadWorkspaceMCPConfigs(trimmedCWD)
	if err != nil {
		return nil, err
	}
	if d == nil || d.SkillManager == nil {
		// Legacy embedders may construct RegistryDeps without installing the
		// skill subsystem. There is no stale skill authority to retarget in that
		// shape, so preserve the rest of the session runtime transition.
		return &preparedRegistrySessionContext{cwd: trimmedCWD, mcpConfigs: configs}, nil
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
		mcpSkillProjection: mcpSkillProjection,
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

func loadWorkspaceMCPConfigs(cwd string) (map[string]svcmcp.MCPServerConfig, error) {
	type candidate struct {
		path  string
		scope svcmcp.ConfigScope
	}
	candidates := []candidate{
		{path: filepath.Join(cwd, ".mcp.json"), scope: svcmcp.ScopeProject},
		{path: filepath.Join(cwd, brand.LegacyConfigDirName, "settings.json"), scope: svcmcp.ScopeLocal},
		{path: filepath.Join(cwd, brand.LegacyDeepSeekConfigDirName, "settings.json"), scope: svcmcp.ScopeLocal},
		{path: filepath.Join(cwd, brand.ConfigDirName, "settings.json"), scope: svcmcp.ScopeLocal},
	}
	configs := make(map[string]svcmcp.MCPServerConfig)
	for _, candidate := range candidates {
		parsed, err := svcmcp.ParseMCPConfigFile(candidate.path, svcmcp.ParseOptions{
			Scope: candidate.scope, ExpandVars: true, FilePath: candidate.path,
		})
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, rootRuntimeWrap(i18n.KeyRootMCPSettingsLoad, err, candidate.path)
		}
		if len(parsed.Errors) > 0 {
			validation := parsed.Errors[0]
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
		if prepared.skillProjectPlan != nil && d.SkillManager != nil {
			if err := d.SkillManager.ApplyProjectSources(prepared.skillProjectPlan); err != nil {
				return err
			}
		}
		d.updateSessionContextAfterSkillCommit(trimmedCWD, allowedDirs, prepared)
		return nil
	}
	if d.mcpSkillRuntime != nil {
		return d.mcpSkillRuntime.withPrepared(prepared.mcpSkillProjection, prepared.skillProjectPlan, transition)
	}
	return transition()
}

// updateSessionContextAfterSkillCommit advances only infallible, in-memory
// consumers after the Manager/store transaction has committed.
func (d *RegistryDeps) updateSessionContextAfterSkillCommit(cwd string, allowedDirs []string, prepared *preparedRegistrySessionContext) {
	trimmedCWD := strings.TrimSpace(cwd)
	if trimmedCWD == "" || prepared == nil {
		return
	}
	if d.RuntimeScope != nil {
		d.RuntimeScope.SetProjectRoot(trimmedCWD)
		d.RuntimeScope.SetAllowedDirs(allowedDirs)
	}
	if d.PlanState != nil {
		d.PlanState.SetProjectRoot(trimmedCWD)
	}
	if d.BackgroundTasks != nil {
		d.BackgroundTasks.SetProjectRoot(trimmedCWD)
	}
	if d.TodoStore != nil {
		d.TodoStore.SetProjectRoot(trimmedCWD)
	}
	if d.CronStore != nil {
		d.CronStore.SetProjectRoot(trimmedCWD)
	}
	if d.BashTool != nil {
		d.BashTool.SetExecutionScope(trimmedCWD, allowedDirs)
	}
	if d.PowerShellTool != nil {
		d.PowerShellTool.SetCWD(trimmedCWD)
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
	if d.TeamManager != nil {
		d.TeamManager.SetProjectRoot(trimmedCWD)
	}
	historyRoot := filepath.Join(trimmedCWD, brand.ConfigDirName, "file-history")
	var historyStores []*tools.FileHistoryStore
	if d.FileWriteTool != nil {
		historyStores = append(historyStores, d.FileWriteTool.HistoryStore)
	}
	if d.FileEditTool != nil {
		historyStores = append(historyStores, d.FileEditTool.HistoryStore)
	}
	if d.NotebookEditTool != nil {
		historyStores = append(historyStores, d.NotebookEditTool.HistoryStore)
	}
	seenHistoryStores := make(map[*tools.FileHistoryStore]struct{})
	for _, store := range historyStores {
		if store == nil {
			continue
		}
		if _, seen := seenHistoryStores[store]; seen {
			continue
		}
		seenHistoryStores[store] = struct{}{}
		store.SetRoot(historyRoot)
	}
	if d.MCPManager != nil {
		d.MCPManager.SetProjectRoot(trimmedCWD)
		d.MCPManager.ReplaceWorkspaceServiceConfigs(prepared.mcpConfigs)
	}
	if d.ServiceMCP != nil {
		d.ServiceMCP.SetWorkingDirectory(trimmedCWD)
		d.ServiceMCP.ReplaceWorkspaceConfigs(prepared.mcpConfigs)
		tools.RegisterDynamicMCPTools(d.Registry, d.ServiceMCP, nil)
	}
	d.publishSessionToolRuntime(d.currentSessionID(), trimmedCWD)
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
		d.TeamManager.SetSessionIdentityRuntime(sessionID, cwd, runtime)
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
	return isAgentTriggersEnabled() && !isEnvTruthy(os.Getenv("CLAUDE_CODE_DISABLE_CRON"))
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

func isWebSearchRuntimeEnabled(providerName, _ string) bool {
	// Every configured provider can use the local DuckDuckGo fallback. Native
	// server-tool support is checked separately before making provider calls.
	return strings.TrimSpace(providerName) != ""
}

// SetupRegistry creates the tool registry with all built-in tools.
// AgentTool.System and TeamManager.System are NOT set here — the caller
// wires the system prompt after construction.
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
	runtimeScope := tools.NewRuntimeScope(cwd, interactive)
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
	runtimeScope.SetFeatureGate(types.ToolFeatureBrief, isEnvTruthy(os.Getenv("CLAUDE_CODE_BRIEF")))
	applyProvider := func(p provider.Provider) {
		if p == nil {
			// Empty provider refs cannot execute the TS provider-native tool.
			runtimeScope.SetProviderInfo("", "")
			runtimeScope.SetFeatureGate(types.ToolFeatureWebSearch, false)
			return
		}
		runtimeScope.SetProviderInfo(p.Name(), p.ModelID())
		runtimeScope.SetFeatureGate(types.ToolFeatureWebSearch, isWebSearchRuntimeEnabled(p.Name(), p.ModelID()))
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
	planState := tools.NewPlanState(cwd)
	backgroundTasks := tools.NewBackgroundTaskManager(cwd)

	// Shared read-file state: used by Read/Edit/Write to coordinate
	// file_unchanged dedup and stale-write detection.
	readState := tools.NewReadFileState()

	// File & shell tools
	bashTool := &tools.BashTool{
		CWD:                  cwd,
		Sandbox:              sb,
		PlanState:            planState,
		Background:           backgroundTasks,
		AllowedDirs:          append([]string(nil), allowedDirs...),
		ReadFileState:        readState,
		SedValidationEnabled: true,
	}
	powerShellTool := &tools.PowerShellTool{CWD: cwd, PlanState: planState, Background: backgroundTasks}
	fileReadTool := &tools.FileReadTool{
		AllowedDirs: append([]string(nil), allowedDirs...),
		ReadState:   readState,
		Runtime:     runtimeScope,
		ModelProvider: func() string {
			if pRef == nil {
				return ""
			}
			if current := pRef.Get(); current != nil {
				return current.ModelID()
			}
			return ""
		},
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
			return filepath.Join(runtimeScope.ProjectRoot(), brand.LegacyConfigDirName, "tool-results")
		},
	}
	historyRoot := filepath.Join(cwd, brand.ConfigDirName, "file-history")
	historyStore := tools.NewFileHistoryStore(historyRoot)
	fileWriteTool := &tools.FileWriteTool{
		AllowedDirs:  allowedDirs,
		PlanState:    planState,
		ReadState:    readState,
		HistoryStore: historyStore,
		LSP:          tools.NewNoopLSPDiagnoser(),
	}
	fileEditTool := &tools.FileEditTool{
		AllowedDirs:  allowedDirs,
		Runtime:      runtimeScope,
		PlanState:    planState,
		ReadState:    readState,
		HistoryStore: historyStore,
		LSP:          tools.NewNoopLSPDiagnoser(),
	}
	reg.Register(bashTool)
	if runtime.GOOS == "windows" || isEnvTruthy(os.Getenv("ENABLE_POWERSHELL_TOOL")) {
		reg.Register(powerShellTool)
	}
	reg.Register(fileReadTool)
	reg.Register(fileWriteTool)
	reg.Register(fileEditTool)
	globTool := tools.NewGlobTool(runtimeScope)
	grepTool := tools.NewGrepTool(runtimeScope)
	if !hasEmbeddedSearchTools() {
		reg.Register(globTool)
		reg.Register(grepTool)
	}

	// Agent tool
	agentTool := &tools.AgentTool{
		Provider:       pRef,
		Registry:       reg,
		Background:     backgroundTasks,
		NonInteractive: !interactive,
	}
	backgroundTasks.SetAgentSessionFactory(agentTool.RestoreAgentSessionFromRecord)
	reg.Register(agentTool)

	// Task tools
	taskStore := tools.NewTaskStore()
	taskStore.SetScopeResolver(runtimeScope)
	todoStore := tools.NewTodoStore(cwd)
	todoStore.SetScopeResolver(runtimeScope)
	taskCreateTool := tools.NewTaskCreateTool(taskStore)
	taskUpdateTool := tools.NewTaskUpdateTool(taskStore)
	taskUpdateTool.Runtime = runtimeScope
	if runtimeScope.IsTodoV2Enabled() {
		reg.Register(taskCreateTool)
		reg.Register(tools.NewTaskListTool(taskStore))
		reg.Register(taskUpdateTool)
		reg.Register(tools.NewTaskGetTool(taskStore))
	}
	reg.Register(tools.NewTaskStopTool(backgroundTasks))
	reg.Register(tools.NewTaskOutputTool(backgroundTasks))
	reg.Register(tools.NewTodoWriteTool(todoStore))

	// Goal tools are always registered but receive their session runtime after
	// repository and session identity wiring is available.
	getGoalTool, createGoalTool, updateGoalTool := tools.NewGoalTools(nil)
	reg.Register(getGoalTool)
	reg.Register(createGoalTool)
	reg.Register(updateGoalTool)

	// Plan tools
	reg.Register(tools.NewEnterPlanModeTool(planState, runtimeScope))
	exitPlanModeTool := tools.NewExitPlanModeTool(planState, runtimeScope)
	reg.Register(exitPlanModeTool)

	// User interaction tools
	askUserQuestionTool := &tools.AskUserQuestionTool{PlanState: planState}
	reg.Register(askUserQuestionTool)
	sendUserMessageTool := tools.NewSendUserMessageTool()
	sendUserMessageTool.WorkingDirectory = runtimeScope.ProjectRoot
	reg.Register(sendUserMessageTool)

	// Web tools — H5: create caches here and inject to avoid global singletons.
	webFetchCache := tools.NewWebFetchCache()
	webSearchCache := tools.NewSearchCache()
	webFetch := tools.NewWebFetchTool(webFetchCache)
	webFetch.Summariser = newWebFetchProviderSummariser(pRef)
	webFetch.SkipWebFetchPreflight = isEnvTruthy(os.Getenv("CLAUDE_CODE_SKIP_WEBFETCH_PREFLIGHT"))
	webSearch := tools.NewWebSearchTool(webSearchCache)
	webSearch.SetWebSearchServerToolProvider(newWebSearchServerToolProvider(pRef), true)
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

	// Cron/scheduling tools
	// NOTE: Cron job execution is not yet wired into the REPL query loop.
	// Jobs are stored and scheduled, but the fire callback only logs.
	cronStore := tools.NewCronStore(cwd, runtimeScope)
	if isCronRuntimeEnabled() {
		cronStore.Start(func(job *tools.CronJob) {
			if err := tools.StartCronPromptExecution(agentTool, backgroundTasks, job); err != nil {
				log.Print(i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyRootLogCronExecutionFailed, job.ID, err))
			}
		})
		reg.Register(tools.NewCronCreateTool(cronStore))
		reg.Register(tools.NewCronDeleteTool(cronStore))
		reg.Register(tools.NewCronListTool(cronStore))
	}

	// Git worktree tools
	worktreeState := &tools.WorktreeState{}
	worktreeManager := tools.NewWorktreeManager()
	worktreeRuntime := tools.NewWorktreeRuntime(cwd, nil)
	worktreeHooks, worktreeHookErr := tools.LoadWorktreeHookBridge(cwd)
	if worktreeHookErr != nil {
		log.Print(i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyRootLogWorktreeHooksFailed, worktreeHookErr))
		worktreeHooks = tools.NewInMemoryWorktreeHookBridge()
	}
	enterWorktreeTool := &tools.EnterWorktreeTool{
		State: worktreeState, Manager: worktreeManager, Runtime: worktreeRuntime,
		SessionID: runtimeScope.SessionID, HookBridge: worktreeHooks,
	}
	exitWorktreeTool := &tools.ExitWorktreeTool{
		State: worktreeState, Manager: worktreeManager, Runtime: worktreeRuntime,
		SessionID: runtimeScope.SessionID, HookBridge: worktreeHooks,
	}
	reg.Register(enterWorktreeTool)
	reg.Register(exitWorktreeTool)

	// Config tool
	if os.Getenv("USER_TYPE") == "ant" {
		configStore := tools.NewConfigStore()
		reg.Register(tools.NewConfigTool(configStore))
	}

	// MCP tools
	serviceMCP := svcmcp.NewManager(svcmcp.WithWorkingDirectory(cwd))
	serviceMCP.LoadFromSettings(filepath.Join(cwd, brand.LegacyConfigDirName, "settings.json"))         //nolint:errcheck
	serviceMCP.LoadFromSettings(filepath.Join(cwd, brand.LegacyDeepSeekConfigDirName, "settings.json")) //nolint:errcheck
	serviceMCP.LoadFromSettings(filepath.Join(cwd, brand.ConfigDirName, "settings.json"))               //nolint:errcheck
	mcpManager := tools.NewMCPManager()
	mcpManager.LoadFromSettings(filepath.Join(cwd, brand.LegacyConfigDirName, "settings.json"))         //nolint:errcheck
	mcpManager.LoadFromSettings(filepath.Join(cwd, brand.LegacyDeepSeekConfigDirName, "settings.json")) //nolint:errcheck
	mcpManager.LoadFromSettings(filepath.Join(cwd, brand.ConfigDirName, "settings.json"))               //nolint:errcheck
	reg.Register(tools.NewMCPTool(mcpManager))
	reg.Register(tools.NewListMcpResourcesTool(mcpManager, serviceMCP))
	reg.Register(tools.NewReadMcpResourceTool(mcpManager, serviceMCP))
	tools.SetMCPServerStatesFn(func() []tools.MCPServerVisibilityState {
		return mcpVisibilityStatesFromHealth(serviceMCP.HealthSnapshot())
	})
	if len(serviceMCP.ServerNames()) > 0 {
		mcpCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := tools.RefreshDynamicMCPTools(mcpCtx, reg, serviceMCP, nil); err != nil {
			log.Print(i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyRootLogMCPRefreshFailed, err))
		}
		cancel()
	}

	// LSP tool — H4: inject per-registry state and manager to eliminate global singletons.
	if isEnvTruthy(os.Getenv("ENABLE_LSP_TOOL")) {
		lspState := tools.NewLSPState()
		lspManager := tools.NewLSPServerManager()
		reg.Register(&tools.LSPTool{State: lspState, Manager: lspManager})
	}

	// Team/coordination tools
	coord := coordinator.NewCoordinator()
	teamManager := tools.NewTeamManager(coord)
	teamManager.CWD = cwd
	teamManager.Provider = pRef
	teamManager.Registry = reg
	teamManager.Background = backgroundTasks
	teamManager.Runtime = runtimeScope
	runtimeScope.SetTeamNameFunc(teamManager.CurrentTeamName)
	teamManager.SetTaskListChangeNotifier(taskStore.NotifyScopeChanged)
	agentTool.TeamManager = teamManager
	exitPlanModeTool.TeamManager = teamManager
	reg.Register(tools.NewSendMessageTool(teamManager))
	if isAgentSwarmsEnabled() {
		reg.Register(tools.NewTeamCreateTool(teamManager))
		reg.Register(tools.NewTeamDeleteTool(teamManager))
	}

	// Skill tool — create the policy state once, then share the exact Manager
	// pointer with every discovery and execution consumer.
	skillTool := tools.NewSkillTool()
	skillSessionOverrides := skills.NewMemorySessionOverrideLayer()
	skillOverrideStore, skillInitErr := skills.NewFileOverrideStore(cwd, nil, skillSessionOverrides)
	if skillInitErr == nil {
		skillTool.Manager.SetOverrideStore(skillOverrideStore)
		skillInitErr = skillTool.Manager.ReplaceProjectSources(cwd)
	}
	if skillInitErr != nil {
		skillTool.Manager.SetOverrideStore(nil)
		skillTool.Manager = nil
		skillOverrideStore = nil
	}
	agentTool.SkillManager = skillTool.Manager
	teamManager.SkillManager = skillTool.Manager
	// Wire file lifecycle skill discovery to the shared manager so reads and
	// edits near a SKILL.md surface that skill into the active set.
	if skillTool.Manager != nil {
		fileReadTool.SkillManager = skillTool.Manager
		fileEditTool.SkillManager = skillTool.Manager
	}
	reg.Register(skillTool)

	// Notebook editing
	notebookEditTool := &tools.NotebookEditTool{
		AllowedDirs:  append([]string(nil), allowedDirs...),
		PlanState:    planState,
		ReadState:    readState,
		HistoryStore: historyStore,
	}
	reg.Register(notebookEditTool)

	// Misc tools
	var toolSearch *tools.ToolSearchTool
	if isToolSearchEnabledOptimistic() {
		toolSearch = &tools.ToolSearchTool{Registry: reg}
		reg.Register(toolSearch)
	}
	mcpListChangedUnregister := tools.RegisterMCPListChangedInvalidators(reg, serviceMCP, nil, toolSearch)
	if isAgentTriggersRemoteEnabled() {
		reg.Register(&tools.RemoteTriggerTool{Availability: tools.NewCachedRemoteTriggerAvailability()})
	}
	if os.Getenv("NODE_ENV") == "test" {
		reg.Register(&tools.TestingPermissionTool{})
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
		CronStore:                cronStore,
		SkillManager:             skillTool.Manager,
		SkillTool:                skillTool,
		SkillOverrideStore:       skillOverrideStore,
		SkillSessionOverrides:    skillSessionOverrides,
		skillInitErr:             skillInitErr,
		PlanState:                planState,
		AskUserQuestionTool:      askUserQuestionTool,
		RuntimeScope:             runtimeScope,
		BashTool:                 bashTool,
		PowerShellTool:           powerShellTool,
		FileReadTool:             fileReadTool,
		FileWriteTool:            fileWriteTool,
		FileEditTool:             fileEditTool,
		NotebookEditTool:         notebookEditTool,
		GlobTool:                 globTool,
		GrepTool:                 grepTool,
		WebFetchTool:             webFetch,
		WebSearchTool:            webSearch,
		WebFetchCache:            webFetchCache,
		BackgroundTasks:          backgroundTasks,
		TodoStore:                todoStore,
		MCPManager:               mcpManager,
		ServiceMCP:               serviceMCP,
		EnterWorktreeTool:        enterWorktreeTool,
		ExitWorktreeTool:         exitWorktreeTool,
		WorktreeManager:          worktreeManager,
		WorktreeRuntime:          worktreeRuntime,
		mcpListChangedUnregister: mcpListChangedUnregister,
	}
	deps.mcpSkillRuntime = newMCPSkillRuntimeBridge(deps.SkillManager, deps.ServiceMCP)
	if deps.mcpSkillRuntime != nil {
		mcpSkillCtx, cancel := context.WithTimeout(context.Background(), mcpSkillCatalogRefreshTimeout)
		_ = deps.mcpSkillRuntime.refresh(mcpSkillCtx)
		cancel()
	}
	worktreeRuntime.SetSwitcher(func(nextCWD string) error {
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

func newWebSearchServerToolProvider(ref *provider.ProviderRef) tools.WebSearchServerToolProvider {
	return tools.WebSearchServerToolFunc(func(ctx context.Context, req tools.WebSearchServerToolRequest) (tools.WebSearchServerToolResponse, error) {
		if ref == nil {
			return tools.WebSearchServerToolResponse{}, tools.ErrWebSearchServerToolUnavailable
		}
		current := ref.Get()
		if current == nil || !supportsProviderNativeWebSearch(current.Name(), current.ModelID()) {
			return tools.WebSearchServerToolResponse{}, tools.ErrWebSearchServerToolUnavailable
		}

		started := time.Now()
		stream, err := current.CreateStream(ctx, provider.Params{
			Model:     current.ModelID(),
			MaxTokens: 4096,
			System:    "You are an assistant for performing a web search tool use",
			Messages:  []types.Message{types.UserMessage("Perform a web search for the query: " + req.Query)},
			ExtraToolSchemas: []types.ServerToolDefinition{{
				Type:           tools.WebSearchServerToolName,
				Name:           "web_search",
				AllowedDomains: append([]string(nil), req.AllowedDomains...),
				BlockedDomains: append([]string(nil), req.BlockedDomains...),
				MaxUses:        req.MaxUses,
			}},
		})
		if err != nil {
			return tools.WebSearchServerToolResponse{}, err
		}
		if stream == nil {
			return tools.WebSearchServerToolResponse{}, i18n.NewError(i18n.KeyRegistryWebSearchProviderNilStream)
		}

		response := tools.WebSearchServerToolResponse{}
		serverToolIDs := make(map[int]string)
		serverToolJSON := make(map[int]*strings.Builder)
		queries := make(map[string]string)
		textBlocks := make(map[int]*strings.Builder)
		for {
			select {
			case <-ctx.Done():
				return tools.WebSearchServerToolResponse{}, ctx.Err()
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
							return tools.WebSearchServerToolResponse{}, parseErr
						}
						if result.ToolUseID == "" {
							result.ToolUseID = event.ContentBlock.ToolUseID
						}
						response.ResultBlocks = append(response.ResultBlocks, result)
						resultCopy := result
						response.Entries = append(response.Entries, tools.WebSearchServerToolEntry{Result: &resultCopy})
						response.Citations = append(response.Citations, string(event.ContentBlock.RawJSON))
						actualQuery := queries[result.ToolUseID]
						if actualQuery == "" {
							actualQuery = req.Query
						}
						if req.OnProgress != nil {
							req.OnProgress(tools.WebSearchProgressEvent{
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
								req.OnProgress(tools.WebSearchProgressEvent{Type: "query_update", ToolUseID: toolUseID, Query: query})
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
							response.Entries = append(response.Entries, tools.WebSearchServerToolEntry{Text: text})
						}
						delete(textBlocks, event.Index)
					}
				case types.EventError:
					if event.Error != nil {
						return tools.WebSearchServerToolResponse{}, event.Error
					}
					return tools.WebSearchServerToolResponse{}, i18n.NewError(i18n.KeyRegistryWebSearchProviderStreamFailed)
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

func parseWebSearchServerResult(raw json.RawMessage) (tools.WebSearchServerToolResult, error) {
	var envelope struct {
		ToolUseID string          `json:"tool_use_id"`
		Content   json.RawMessage `json:"content"`
	}
	if len(raw) == 0 {
		return tools.WebSearchServerToolResult{}, i18n.NewError(i18n.KeyRegistryWebSearchResultMissingRawContent)
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return tools.WebSearchServerToolResult{}, i18n.WrapError(i18n.KeyRegistryWebSearchDecodeResultBlock, err)
	}
	result := tools.WebSearchServerToolResult{ToolUseID: envelope.ToolUseID}
	trimmed := strings.TrimSpace(string(envelope.Content))
	if strings.HasPrefix(trimmed, "[") {
		var hits []struct {
			Title     string `json:"title"`
			URL       string `json:"url"`
			PageAge   string `json:"page_age"`
			CitedText string `json:"cited_text"`
		}
		if err := json.Unmarshal(envelope.Content, &hits); err != nil {
			return tools.WebSearchServerToolResult{}, i18n.WrapError(i18n.KeyRegistryWebSearchDecodeHits, err)
		}
		for _, hit := range hits {
			result.Results = append(result.Results, tools.WebSearchResult{
				Title: hit.Title, URL: hit.URL, PageAge: hit.PageAge, CitedText: hit.CitedText,
			})
		}
		return result, nil
	}
	var failure struct {
		ErrorCode string `json:"error_code"`
	}
	if err := json.Unmarshal(envelope.Content, &failure); err != nil {
		return tools.WebSearchServerToolResult{}, i18n.WrapError(i18n.KeyRegistryWebSearchDecodeError, err)
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

func newWebFetchProviderSummariser(ref *provider.ProviderRef) tools.SummariserClient {
	return tools.SummariserFunc(func(ctx context.Context, req tools.SummariserRequest) (string, error) {
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

func mcpVisibilityStatesFromHealth(snapshot svcmcp.HealthSnapshot) []tools.MCPServerVisibilityState {
	out := make([]tools.MCPServerVisibilityState, 0, len(snapshot.Servers))
	for _, server := range snapshot.Servers {
		if server.State == svcmcp.MCPStateConnected {
			continue
		}
		out = append(out, tools.MCPServerVisibilityState{
			Name:                 server.Name,
			State:                string(server.State),
			ReconnectAttempt:     server.ReconnectAttempt,
			MaxReconnectAttempts: server.MaxReconnectAttempts,
			Error:                server.Error,
		})
	}
	return out
}
