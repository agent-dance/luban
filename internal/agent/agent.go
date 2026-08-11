package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/i18n"
	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"
	"github.com/agent-dance/luban/internal/contracts/permission"
	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/internal/gitutil"
	"github.com/agent-dance/luban/internal/mcp/catalog"
	mcpmanager "github.com/agent-dance/luban/internal/mcp/manager"
	"github.com/agent-dance/luban/internal/runtime/loop"
	"github.com/agent-dance/luban/internal/runtime/skillauthority"
	storepaths "github.com/agent-dance/luban/internal/store/paths"
	runtimestore "github.com/agent-dance/luban/internal/store/runtime"
	"github.com/agent-dance/luban/internal/store/secureio"
	toolmcp "github.com/agent-dance/luban/internal/tools/mcp"
	"github.com/agent-dance/luban/internal/tools/toolbase"
	toolworktree "github.com/agent-dance/luban/internal/tools/worktree"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/runtimeevent"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

// DefaultMaxAgentDepth is the maximum nesting depth for sub-agents
const DefaultMaxAgentDepth = 3

var agentAutoBackgroundDelay = defaultAgentAutoBackgroundDelay

var errAgentPermissionSnapshotUnavailable = i18n.NewError(i18n.KeyToolAgentDeepPermissionSnapshotUnavailable)
var errAgentResumeContextUntrusted = i18n.NewError(i18n.KeyToolAgentDeepResumeContextUntrusted)

const (
	forkSubagentType    = "fork"
	forkBoilerplateTag  = "fork-boilerplate"
	forkDirectivePrefix = "Your directive: "
)

func forkPlaceholderToolResultText() string {
	return toolRuntimeText(i18n.KeyToolAgentForkStarted)
}

var agentColorNames = map[string]struct{}{
	"red":    {},
	"blue":   {},
	"green":  {},
	"yellow": {},
	"purple": {},
	"orange": {},
	"pink":   {},
	"cyan":   {},
}

// AgentTool spawns a sub-agent with its own conversation
type AgentTool struct {
	Provider           provider.Provider
	Registry           *registry.Registry
	System             string
	Model              string
	ProgressiveContext loop.ProgressiveContextConfig
	// ServiceTier is inherited by every nested model generation so a
	// contract-bound parent cannot spawn an unpinned scheduling lane.
	ServiceTier provider.ServiceTier
	// Background tracks async/background agent executions.
	Background *BackgroundTaskManager
	// Collaboration owns team persistence and teammate transaction semantics.
	Collaboration agentcontract.CollaborationSpawner
	// SkillManager lets sub-agents receive the same model-invocable skill listing as the parent loop.
	SkillManager *skills.Manager
	// HookRunner lets sub-agents execute the same hook lifecycle as the parent loop.
	HookRunner *hooks.Runner
	// PermissionHandler lets sub-agents inherit the parent loop's permission gate.
	PermissionHandler permission.PermissionHandler
	// InlineProfiles contains JSON-defined agents injected by CLI/SDK options.
	InlineProfiles map[string]agentProfile
	// AllowedAgentTypes restricts Agent(...) nested invocations when the parent profile allowed a scoped Agent tool.
	AllowedAgentTypes []string
	// TeamMember marks an Agent tool running inside a teammate session.
	TeamMember   bool
	TeamMemberID string
	// NonInteractive matches SDK/piped/runtime subagent sessions where upstream can disable built-in agents.
	NonInteractive bool
	Depth          int // current nesting depth (0 = top-level)
	MaxDepth       int // max allowed depth (0 = use DefaultMaxAgentDepth)

	// --- Runtime hook accessors (alignment audit agent-04/07/08/10) -------
	// These wire the helpers that already live in agent_progress.go,
	// agent_isolation.go, agent_mcp_readiness.go and the profile loader so
	// that the parent loop can observe sub-agent lifecycle events. They are
	// initialised lazily on first access; callers may pre-populate them via
	// the matching setters before Execute runs.

	runtimeMu              sync.Mutex
	sessionRuntime         AgentSessionRuntime
	runtimeSet             bool
	barrierMu              sync.RWMutex
	sessionBarrier         *sync.RWMutex
	progressObservers      map[uint64]func(agentcontract.ProgressEvent)
	nextProgressObserverID uint64
	mcpProbe               MCPReadinessProbe
}

// SetSessionBarrier installs the registry publication barrier. It is expected
// to be configured during bootstrap before session switching begins.
func (t *AgentTool) SetSessionBarrier(barrier *sync.RWMutex) {
	if t == nil {
		return
	}
	t.barrierMu.Lock()
	t.sessionBarrier = barrier
	t.barrierMu.Unlock()
}

func (t *AgentTool) lockSessionSnapshot() (func(), bool) {
	t.barrierMu.RLock()
	barrier := t.sessionBarrier
	t.barrierMu.RUnlock()
	if barrier == nil {
		return func() {}, false
	}
	barrier.RLock()
	return barrier.RUnlock, true
}

// AgentSessionRuntime is the workspace-specific prompt/hook pair used when a
// new sub-agent is created. Keeping the pair behind one lock prevents a
// session switch from exposing a prompt from one workspace with hooks from
// another.
type AgentSessionRuntime struct {
	System      string
	HookRunner  *hooks.Runner
	ToolRuntime types.ToolRuntimeContext
}

type agentLaunchRuntime struct {
	session           AgentSessionRuntime
	provider          provider.Provider
	registry          *registry.Registry
	permissionHandler permission.PermissionHandler
}

func (t *AgentTool) SetSessionRuntime(runtime AgentSessionRuntime) {
	if t == nil {
		return
	}
	t.runtimeMu.Lock()
	runtime.ToolRuntime = cloneToolRuntimeContext(runtime.ToolRuntime)
	t.sessionRuntime = runtime
	t.runtimeSet = true
	t.runtimeMu.Unlock()
}

// SetSessionHookRunner updates only the hook half of the workspace snapshot.
// RegistryDeps calls it while holding the shared publication write barrier.
func (t *AgentTool) SetSessionHookRunner(runner *hooks.Runner) {
	if t == nil {
		return
	}
	t.runtimeMu.Lock()
	if !t.runtimeSet {
		t.sessionRuntime = AgentSessionRuntime{System: t.System, HookRunner: t.HookRunner}
		t.runtimeSet = true
	}
	t.sessionRuntime.HookRunner = runner
	t.runtimeMu.Unlock()
}

func (t *AgentTool) SetSessionToolRuntime(runtime types.ToolRuntimeContext) {
	if t == nil {
		return
	}
	t.runtimeMu.Lock()
	if !t.runtimeSet {
		t.sessionRuntime = AgentSessionRuntime{System: t.System, HookRunner: t.HookRunner}
		t.runtimeSet = true
	}
	t.sessionRuntime.ToolRuntime = cloneToolRuntimeContext(runtime)
	t.runtimeMu.Unlock()
}

// SetChildPermissionHandler keeps dynamically selected parent permission
// handlers (notably SDK modes) authoritative for subsequently spawned agents.
func (t *AgentTool) SetChildPermissionHandler(handler permission.PermissionHandler) {
	if t == nil {
		return
	}
	t.runtimeMu.Lock()
	t.PermissionHandler = handler
	t.runtimeMu.Unlock()
}

func (t *AgentTool) SessionRuntime() AgentSessionRuntime {
	if t == nil {
		return AgentSessionRuntime{}
	}
	unlock, _ := t.lockSessionSnapshot()
	defer unlock()
	t.runtimeMu.Lock()
	defer t.runtimeMu.Unlock()
	if t.runtimeSet {
		runtime := t.sessionRuntime
		runtime.ToolRuntime = cloneToolRuntimeContext(runtime.ToolRuntime)
		return runtime
	}
	return AgentSessionRuntime{System: t.System, HookRunner: t.HookRunner}
}

// captureLaunchRuntime takes the workspace prompt, hook runner, tool runtime,
// provider, registry, and permission gate under the same publication barrier.
// The registry is immediately pinned while the barrier is held so later
// foreground retargeting cannot mutate shared file/shell/plan tool fields.
func (t *AgentTool) captureLaunchRuntime() agentLaunchRuntime {
	if t == nil {
		return agentLaunchRuntime{}
	}
	unlock, sessionBarrierHeld := t.lockSessionSnapshot()
	defer unlock()

	t.runtimeMu.Lock()
	session := AgentSessionRuntime{System: t.System, HookRunner: t.HookRunner}
	if t.runtimeSet {
		session = t.sessionRuntime
	}
	session.ToolRuntime = cloneToolRuntimeContext(session.ToolRuntime)
	permissionHandler := t.PermissionHandler
	t.runtimeMu.Unlock()

	var pinnedRegistry *registry.Registry
	if t.Registry != nil {
		pinnedRegistry = t.Registry.Clone()
		// Permission state is mutable within a session (Ask/Auto/Plan). Capture
		// the live parent policy at the exact spawn boundary instead of reusing
		// the session-publication cache, which may predate the latest mode switch.
		if runtime, ok := pinnedRegistry.RuntimeContextWithinSessionBarrier(); ok {
			session.ToolRuntime = cloneToolRuntimeContext(runtime)
		} else if !sessionBarrierHeld && pinnedRegistry.HasRuntimeContextProvider() {
			// Standalone/custom registries without a publication barrier can still
			// provide a live runtime through the general provider contract.
			session.ToolRuntime = cloneToolRuntimeContext(pinnedRegistry.RuntimeContext())
		}
		provider := agentRuntimeContextProvider{
			snapshot: session.ToolRuntime,
		}
		pinRegistryForAgentRuntime(pinnedRegistry, provider, session.ToolRuntime)
		pinnedRegistry.SetRuntimeContextProvider(provider)
	}
	return agentLaunchRuntime{
		session:           session,
		provider:          snapshotAgentProvider(t.Provider),
		registry:          pinnedRegistry,
		permissionHandler: permissionHandler,
	}
}

func (t *AgentTool) publishProgress(event agentcontract.ProgressEvent) {
	if t == nil {
		return
	}
	t.runtimeMu.Lock()
	observers := make([]func(agentcontract.ProgressEvent), 0, len(t.progressObservers))
	for _, observer := range t.progressObservers {
		observers = append(observers, observer)
	}
	t.runtimeMu.Unlock()
	for _, observer := range observers {
		observer(event)
	}
}

// SubscribeProgress observes every AgentTool run, including concurrent runs
// that receive distinct emitters. It is the supported presentation bridge.
func (t *AgentTool) SubscribeProgress(observer func(agentcontract.ProgressEvent)) func() {
	if t == nil || observer == nil {
		return func() {}
	}
	t.runtimeMu.Lock()
	if t.progressObservers == nil {
		t.progressObservers = make(map[uint64]func(agentcontract.ProgressEvent))
	}
	t.nextProgressObserverID++
	id := t.nextProgressObserverID
	t.progressObservers[id] = observer
	t.runtimeMu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			t.runtimeMu.Lock()
			delete(t.progressObservers, id)
			t.runtimeMu.Unlock()
		})
	}
}

func (t *AgentTool) progressForAgentRun(agentID, agentType string) *AgentProgressEmitter {
	if t == nil {
		return nil
	}
	emitter := newAgentProgressEmitter(agentID, agentType)
	emitter.AddObserver(t.publishProgress)
	return emitter
}

// SetMCPReadinessProbe registers a probe used by Execute to gate sub-agent
// startup behind WaitForMCPReadiness when the resolved profile references
// MCP-namespaced tools (agent-07).
func (t *AgentTool) SetMCPReadinessProbe(probe MCPReadinessProbe) {
	if t == nil {
		return
	}
	t.runtimeMu.Lock()
	defer t.runtimeMu.Unlock()
	t.mcpProbe = probe
}

type agentRunSummary struct {
	AgentID          string
	AgentType        string
	Provider         string
	Model            string
	Prompt           string
	Output           string
	ToolUseCount     int
	TotalTokens      int
	TotalDuration    int64
	Usage            *types.Usage
	CWD              string
	Mode             string
	Isolation        string
	WorktreePath     string
	WorktreeBranch   string
	TranscriptPath   string
	LatestToolUse    string
	Outcome          agentcontract.RunOutcome
	TerminalReason   string
	ArtifactRefs     []string
	VerificationRefs []string
}

// ErrAgentRunInterrupted identifies a run whose owning runtime/session ended
// without a normal cancellation request. Callers can preserve this separately
// from provider failures and user cancellation.
var ErrAgentRunInterrupted = errors.New("agent_run_interrupted")

func classifyAgentRunTermination(err error, hasOutput bool) (agentcontract.RunOutcome, string) {
	if err == nil {
		return agentcontract.RunOutcomeSucceeded, "completed"
	}
	var maxTurns *loop.MaxTurnsError
	switch {
	case errors.As(err, &maxTurns):
		return agentcontract.RunOutcomePartial, "max_turns"
	case errors.Is(err, ErrAgentRunInterrupted):
		return agentcontract.RunOutcomeInterrupted, "runtime_interrupted"
	case errors.Is(err, context.DeadlineExceeded):
		return agentcontract.RunOutcomeTimedOut, "deadline_exceeded"
	case errors.Is(err, context.Canceled):
		return agentcontract.RunOutcomeCancelled, "context_cancelled"
	case hasOutput:
		return agentcontract.RunOutcomePartial, "error_after_partial_result"
	default:
		return agentcontract.RunOutcomeFailed, "error"
	}
}

func agentRunOutcomeError(summary agentRunSummary) error {
	if summary.Outcome == "" || summary.Outcome == agentcontract.RunOutcomeSucceeded {
		return nil
	}
	reason := strings.ReplaceAll(strings.TrimSpace(summary.TerminalReason), "_", " ")
	if reason == "" {
		reason = string(summary.Outcome)
	}
	return i18n.NewError(i18n.KeyToolAgentDeepRunOutcome, summary.Outcome, reason)
}

type agentLoopBundle struct {
	Loop              *loop.QueryLoop
	Metadata          agentcontract.SessionMetadata
	Cleanup           func()
	PermissionHandler *agentPermissionSnapshotHandler
	Progress          *AgentProgressEmitter
}

func cloneAgentSessionMetadata(metadata agentcontract.SessionMetadata) agentcontract.SessionMetadata {
	cloned := metadata
	if metadata.PermissionSnapshot != nil {
		snapshot := cloneToolRuntimeContext(*metadata.PermissionSnapshot)
		cloned.PermissionSnapshot = &snapshot
	}
	return cloned
}

type agentProfile struct {
	Name                  string
	WhenToUse             string
	SystemPrefix          string
	AllowedTools          map[string]struct{}
	DisallowedTools       map[string]struct{}
	AllowedToolRules      []agentPermissionRule
	DisallowedToolRules   []agentPermissionRule
	AllowedToolSpecs      []string
	DisallowedToolSpecs   []string
	AllowedToolsSpecified bool
	Model                 string
	ReasoningEffort       string
	MaxTurns              int
	Isolation             string
	Skills                []string
	MCPServers            []string
	MCPServerConfigs      map[string]catalog.MCPServerConfig
	RequiredMCPServers    []string
	HookRunner            *hooks.Runner
	InitialPrompt         string
	Background            bool
	Memory                string
	Color                 string
	OmitBaseSystem        bool
	// OmitInstructions, when true, strips the user-instructions block
	// from the inherited base system prompt while keeping the rest of the
	// parent's identity, tools, and policy text. Read-only Explore/Plan
	// agents set this to save 10-30k tokens per spawn since they only
	// answer questions and don't apply project conventions to writes.
	OmitInstructions bool
}

type agentWorktree struct {
	RepoRoot   string
	Path       string
	Branch     string
	HeadCommit string
}

type agentLoopOptions struct {
	Context                 context.Context
	ReuseCWD                string
	ReuseIsolation          string
	ReuseWorktreePath       string
	ReuseWorktreeBranch     string
	ReuseWorktreeHeadCommit string
	Profile                 *agentProfile
	InitialMessages         []types.Message
	OverrideSystem          string
	OverrideModel           string
	OverrideProvider        provider.Provider
	CacheLineageID          string
	UseExactTools           bool
	ForkParentCWD           string
	SkipInitialPrompt       bool
	TeamMember              bool
	Progress                *AgentProgressEmitter
	PermissionSnapshot      *types.ToolRuntimeContext
	ApprovalRouting         agentcontract.ApprovalRouting
	PresentationSessionID   string
	SkillProjectGeneration  skills.ProjectSourceGeneration
}

type agentToolRegistryOptions struct {
	IsAsync    bool
	AllowAgent bool
}

type agentPermissionRule struct {
	ToolName       string
	RuleContent    string
	HasRuleContent bool
	Raw            string
}

type agentToolContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type agentToolUsage struct {
	InputTokens              int  `json:"input_tokens"`
	OutputTokens             int  `json:"output_tokens"`
	CacheCreationInputTokens *int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     *int `json:"cache_read_input_tokens"`
	ServerToolUse            any  `json:"server_tool_use"`
	ServiceTier              any  `json:"service_tier"`
	CacheCreation            any  `json:"cache_creation"`
}

type agentCompletedToolResult struct {
	Status            string                  `json:"status"`
	Kind              string                  `json:"kind,omitempty"`
	Prompt            string                  `json:"prompt"`
	AgentID           string                  `json:"agentId"`
	AgentType         string                  `json:"agentType"`
	Content           []agentToolContentBlock `json:"content"`
	TotalDurationMs   int64                   `json:"totalDurationMs"`
	TotalTokens       int                     `json:"totalTokens"`
	TotalToolUseCount int                     `json:"totalToolUseCount"`
	Usage             agentToolUsage          `json:"usage"`
	CWD               string                  `json:"cwd,omitempty"`
	Mode              string                  `json:"mode,omitempty"`
	Isolation         string                  `json:"isolation,omitempty"`
	Model             string                  `json:"model,omitempty"`
	WorktreePath      string                  `json:"worktreePath,omitempty"`
	WorktreeBranch    string                  `json:"worktreeBranch,omitempty"`
	TranscriptPath    string                  `json:"transcriptPath,omitempty"`
	LatestToolUse     string                  `json:"latestToolUse,omitempty"`
}

type agentAsyncToolResult struct {
	IsAsync           bool   `json:"isAsync"`
	Status            string `json:"status"`
	Kind              string `json:"kind,omitempty"`
	Prompt            string `json:"prompt"`
	Description       string `json:"description"`
	AgentID           string `json:"agentId"`
	OutputFile        string `json:"outputFile"`
	CanReadOutputFile bool   `json:"canReadOutputFile"`
	Message           string `json:"message"`
}

func (t *AgentTool) Name() string { return "Agent" }

func (t *AgentTool) Description() string {
	profiles := t.availableAgentProfilesForDescription()
	agentLines := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		agentLines = append(agentLines, formatAgentDescriptionLine(profile))
	}
	availableAgents := "No agents are currently available."
	if len(agentLines) > 0 {
		availableAgents = strings.Join(agentLines, "\n")
	}

	forkEnabled := isForkSubagentEnabled()
	parts := []string{
		"Launch a new agent to handle complex, multi-step tasks autonomously.",
		"Available agent types and the tools they have access to:\n" + availableAgents,
		agentTypeSelectionDescription(forkEnabled),
	}
	if !forkEnabled {
		parts = append(parts,
			"When NOT to use the Agent tool:\n"+
				"- If you want to read a specific file path, use the Read tool or the Glob tool instead of Agent to find the match more quickly.\n"+
				"- If you are searching for a specific class definition like \"class Foo\", use the Glob tool instead, to find the match more quickly.\n"+
				"- If you are searching for code within a specific file or set of 2-3 files, use the Read tool instead of Agent to find the match more quickly.\n"+
				"- Other tasks that are not related to the agent descriptions above.",
		)
	}
	parts = append(parts,
		"Writing the prompt:\n"+
			"Brief the agent like a smart colleague who just walked into the room. Fresh specialized agents start without this conversation, so include the goal, why it matters, what you already learned or ruled out, relevant paths or commands, and the exact evidence you expect back. Do not delegate understanding with vague phrases like \"based on your findings, fix it\"; write prompts that prove you understood what should change.",
		"Usage notes:\n"+
			"- Always include a short description (3-5 words) summarizing what the agent will do.\n"+
			"- Clearly tell the agent whether you expect it to write code or only do research.\n"+
			"- Subagents always inherit the current session model.\n"+
			"- When the agent is done, it returns a single message to you. The result is not visible to the user; send a concise summary for any user-relevant result.\n"+
			"- The agent's outputs should generally be trusted, but review important claims before relying on them.\n"+
			"- If the agent description says it should be used proactively, use judgment and try to use it without the user having to ask first.\n"+
			"- If the user specifies that they want you to run agents \"in parallel\", you MUST send a single assistant message with multiple Agent tool calls. For example, if you need to launch both a build-validator agent and a test-runner agent in parallel, send a single assistant message with both tool calls.\n"+
			"- Do not set team_name for ordinary subagents, including parallel Agent calls. Use team_name only after TeamCreate or when deliberately spawning a teammate into an existing team.\n"+
			"- You can optionally set isolation=\"worktree\" to run the agent in a temporary git worktree. The worktree is automatically cleaned up if the agent makes no changes; if changes are made, the worktree path and branch are returned in the result.\n"+
			"- Omit isolation for read-only research or comparison agents, including built-in Explore and Plan, unless the user explicitly asks for a worktree; worktree isolation changes relative path context.",
		"Use specialized agents when their description matches the task. Use general-purpose for broad research, multi-step code searches, or tasks that do not fit a specialist.",
	)
	if t.TeamMember {
		parts = append(parts,
			"Teammate context:\n"+
				"- The run_in_background, name, and team_name parameters are not available in this context. Only synchronous subagents are supported.\n"+
				"- Teammates cannot spawn other teammates. Omit those parameters to spawn a regular subagent.",
		)
	}
	if !agentBackgroundTasksDisabled() && !forkEnabled && !t.TeamMember {
		parts = append(parts,
			"Background agents:\n"+
				"- You can optionally run agents in the background using run_in_background=true. When an agent runs in the background, you will be automatically notified when it completes; do NOT sleep, poll, or proactively check on its progress.\n"+
				"- Use foreground by default when you need the agent's result before proceeding, including research or comparison agents whose findings you must summarize to the user.\n"+
				"- Parallel Agent calls are not the same as background agents: if the user asks to run multiple agents in parallel and you need their results, send multiple foreground Agent tool calls in one assistant message and omit run_in_background.\n"+
				"- Use background only when you have independent work to do or can end your response without the agent's result.\n"+
				"- To continue a previously spawned agent, use SendMessage with the agent's ID or name as the to field. The agent resumes with its full context preserved. Each Agent invocation starts fresh, so provide a complete task description.",
		)
	} else {
		parts = append(parts, "To continue a previously spawned agent, use SendMessage with the agent's ID or name as the to field. The agent resumes with its full context preserved.")
	}
	if forkEnabled {
		parts = append(parts, forkSubagentPromptDescription())
	}
	return strings.Join(parts, "\n\n")
}

func agentTypeSelectionDescription(forkEnabled bool) string {
	if forkEnabled {
		return "When using this tool, specify a subagent_type to use a specialized agent, or omit it to fork yourself. A fork inherits your full conversation context."
	}
	return "When using this tool, specify a subagent_type parameter to select which agent type to use. If omitted, the general-purpose agent is used."
}

func forkSubagentPromptDescription() string {
	return "When to fork:\n" +
		"Fork yourself by omitting subagent_type when intermediate tool output is not worth keeping in your context. Use forks for open-ended research that can run independently or implementation work that needs more than a couple of edits. Do not set a model on a fork because a different model cannot reuse the parent's cache. Pass a short name so the user can identify the fork.\n\n" +
		"Do not read or tail a fork's output file unless the user explicitly asks for a progress check. You will receive a completion notification. After launching, never fabricate or predict fork results; if the user asks before the notification arrives, report status only."
}

func (t *AgentTool) Schema() types.JSONSchema {
	properties := map[string]any{
		"description": map[string]any{
			"type":        "string",
			"description": "A short (3-5 word) description of the task",
		},
		"prompt": map[string]any{
			"type":        "string",
			"description": "The task for the agent to perform",
		},
		"subagent_type": map[string]any{
			"type":        "string",
			"description": "The type of specialized agent to use for this task",
		},
		"isolation": map[string]any{
			"type":        "string",
			"enum":        []string{"worktree"},
			"description": `Isolation mode. "worktree" creates a temporary git worktree so the agent works on an isolated copy of the repo. Omit for read-only Explore/Plan or comparison agents unless the user explicitly asks for isolation.`,
		},
	}
	if !t.TeamMember {
		properties["name"] = map[string]any{
			"type":        "string",
			"description": "Optional display/addressing name for the spawned agent. This does not require a team by itself.",
		}
		properties["team_name"] = map[string]any{
			"type":        "string",
			"description": "Existing TeamCreate team for teammate spawning. Do not set this for ordinary subagents, including parallel Agent calls. Use team_name only after TeamCreate or when deliberately spawning a teammate into an existing team.",
		}
	}
	if !agentBackgroundTasksDisabled() && !isForkSubagentEnabled() && !t.TeamMember {
		properties["run_in_background"] = map[string]any{
			"type":        "boolean",
			"description": "Set to true only when the agent can run in the background and you can continue or end without its result. Omit for parallel research/comparison agents whose results you need before responding.",
		}
	}
	return types.JSONSchema{
		Type:                 "object",
		Properties:           properties,
		Required:             []string{"description", "prompt"},
		AdditionalProperties: false,
	}
}

func (t *AgentTool) availableAgentProfilesForDescription() []agentProfile {
	profiles := t.builtinAgentProfilesForRuntime()
	if !isTruthyAgentEnv(os.Getenv("LUBAN_CODE_SIMPLE")) {
		cwd, _ := os.Getwd()
		if pluginProfiles, err := loadPluginAgentProfiles(cwd); err == nil {
			profiles = mergeAgentDescriptionProfiles(profiles, pluginProfiles, true)
		}
		customDirs, managedDirs := agentSearchDirGroups(cwd)
		if customProfiles, err := loadCustomAgentProfilesFromDirs(cwd, customDirs); err == nil {
			profiles = mergeAgentDescriptionProfiles(profiles, customProfiles, true)
		}
		if t != nil && len(t.InlineProfiles) > 0 {
			profiles = mergeAgentDescriptionProfiles(profiles, t.inlineProfilesForDescription(), true)
		}
		if managedProfiles, err := loadCustomAgentProfilesFromDirs(cwd, managedDirs); err == nil {
			profiles = mergeAgentDescriptionProfiles(profiles, managedProfiles, true)
		}
	} else if t != nil && len(t.InlineProfiles) > 0 {
		profiles = mergeAgentDescriptionProfiles(profiles, t.inlineProfilesForDescription(), true)
	}
	var source *registry.Registry
	if t != nil {
		source = t.Registry
	}
	profiles = filterAgentDescriptionProfilesByMCPRequirements(profiles, source)
	if t != nil && len(t.AllowedAgentTypes) > 0 {
		profiles = filterAgentDescriptionProfiles(profiles, t.AllowedAgentTypes)
	}
	return profiles
}

func (t *AgentTool) inlineProfilesForDescription() []agentProfile {
	if t == nil || len(t.InlineProfiles) == 0 {
		return nil
	}
	keys := make([]string, 0, len(t.InlineProfiles))
	for key := range t.InlineProfiles {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	inlineProfiles := make([]agentProfile, 0, len(keys))
	for _, key := range keys {
		inlineProfiles = append(inlineProfiles, t.InlineProfiles[key])
	}
	return inlineProfiles
}

func (t *AgentTool) builtinAgentProfilesForRuntime() []agentProfile {
	if t != nil && t.disableBuiltInAgentProfiles() {
		return nil
	}
	return filterBuiltinAgentProfilesByFeatureGates(builtinAgentProfiles())
}

func (t *AgentTool) disableBuiltInAgentProfiles() bool {
	return t != nil && t.NonInteractive && isTruthyAgentEnv(os.Getenv("LUBAN_AGENT_SDK_DISABLE_BUILTIN_AGENTS"))
}

func mergeAgentDescriptionProfiles(base []agentProfile, additions []agentProfile, replaceExisting bool) []agentProfile {
	out := append([]agentProfile(nil), base...)
	indexByName := make(map[string]int, len(out)+len(additions))
	for i, profile := range out {
		if key := strings.ToLower(strings.TrimSpace(profile.Name)); key != "" {
			indexByName[key] = i
		}
	}
	for _, profile := range additions {
		key := strings.ToLower(strings.TrimSpace(profile.Name))
		if key == "" {
			continue
		}
		if existing, ok := indexByName[key]; ok {
			if replaceExisting {
				out[existing] = profile
			}
			continue
		}
		indexByName[key] = len(out)
		out = append(out, profile)
	}
	return out
}

func filterAgentDescriptionProfiles(profiles []agentProfile, allowedTypes []string) []agentProfile {
	allowed := make(map[string]struct{}, len(allowedTypes))
	for _, value := range allowedTypes {
		if trimmed := strings.ToLower(strings.TrimSpace(value)); trimmed != "" {
			allowed[trimmed] = struct{}{}
		}
	}
	if len(allowed) == 0 {
		return profiles
	}
	out := make([]agentProfile, 0, len(profiles))
	for _, profile := range profiles {
		if _, ok := allowed[strings.ToLower(strings.TrimSpace(profile.Name))]; ok {
			out = append(out, profile)
		}
	}
	return out
}

func filterAgentDescriptionProfilesByMCPRequirements(profiles []agentProfile, source *registry.Registry) []agentProfile {
	available := availableAgentMCPServers(source)
	out := make([]agentProfile, 0, len(profiles))
	for _, profile := range profiles {
		if agentMCPRequirementsSatisfied(profile, available) {
			out = append(out, profile)
		}
	}
	return out
}

func formatAgentDescriptionLine(profile agentProfile) string {
	whenToUse := strings.TrimSpace(profile.WhenToUse)
	if whenToUse == "" {
		whenToUse = toolRuntimeText(i18n.KeyToolAgentProfileDescriptionMissing)
	}
	return toolRuntimeFormat(i18n.KeyToolAgentProfileLine, profile.Name, whenToUse, describeAgentProfileTools(profile))
}

func describeAgentProfileTools(profile agentProfile) string {
	allowedSpecs := displayAgentToolSpecs(profile.AllowedToolSpecs, profile.AllowedTools)
	disallowedSpecs := displayAgentToolSpecs(profile.DisallowedToolSpecs, profile.DisallowedTools)
	if profile.AllowedToolsSpecified {
		if len(allowedSpecs) == 0 {
			return toolRuntimeText(i18n.KeyToolAgentProfileNoTools)
		}
		if len(disallowedSpecs) == 0 {
			return strings.Join(allowedSpecs, ", ")
		}
		disallowed := make(map[string]struct{}, len(disallowedSpecs))
		for _, spec := range disallowedSpecs {
			if name := normalizedToolNameFromPermissionSpec(spec); name != "" {
				disallowed[name] = struct{}{}
			}
		}
		filtered := make([]string, 0, len(allowedSpecs))
		for _, spec := range allowedSpecs {
			if _, blocked := disallowed[normalizedToolNameFromPermissionSpec(spec)]; !blocked {
				filtered = append(filtered, spec)
			}
		}
		if len(filtered) == 0 {
			return toolRuntimeText(i18n.KeyToolAgentProfileNoTools)
		}
		return strings.Join(filtered, ", ")
	}
	if len(disallowedSpecs) > 0 {
		return toolRuntimeFormat(i18n.KeyToolAgentProfileAllToolsExcept, strings.Join(disallowedSpecs, ", "))
	}
	return toolRuntimeText(i18n.KeyToolAgentProfileAllTools)
}

func displayAgentToolSpecs(specs []string, set map[string]struct{}) []string {
	if len(specs) > 0 {
		out := make([]string, 0, len(specs))
		seen := map[string]struct{}{}
		for _, spec := range specs {
			trimmed := strings.TrimSpace(spec)
			key := strings.ToLower(trimmed)
			if trimmed == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, trimmed)
		}
		return out
	}
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for name := range set {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	sort.Strings(out)
	return out
}

func (t *AgentTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	runStarted := time.Now()
	agentID := "agent-" + uuid.NewString()
	in, toolErr := toolbase.ParseInputOrError[agentcontract.Input](input)
	if toolErr != nil {
		toolErr.Data = AgentResultFromError(agentID, "", time.Since(runStarted).Milliseconds(), errors.New(toolErr.Content))
		return *toolErr, nil
	}

	if in.Prompt == "" {
		return agentFailureToolResult(ctx, agentID, in.SubagentType, "", runStarted, i18n.NewError(i18n.KeyToolAgentPromptRequired)), nil
	}
	if strings.TrimSpace(in.Description) == "" {
		return agentFailureToolResult(ctx, agentID, in.SubagentType, "", runStarted, i18n.NewError(i18n.KeyToolAgentDescriptionRequired)), nil
	}
	skillAuthority, authorityErr := skillauthority.Capture(ctx, t.SkillManager)
	if authorityErr != nil {
		return agentFailureToolResult(ctx, agentID, in.SubagentType, "", runStarted, authorityErr), nil
	}

	maxDepth := t.MaxDepth
	if maxDepth == 0 {
		maxDepth = DefaultMaxAgentDepth
	}
	if t.Depth >= maxDepth {
		return agentFailureToolResult(ctx, agentID, in.SubagentType, "", runStarted, i18n.NewError(i18n.KeyToolAgentMaxDepth, maxDepth)), nil
	}

	if err := t.validateAgentInvocation(in); err != nil {
		return agentFailureToolResult(ctx, agentID, in.SubagentType, "", runStarted, err), nil
	}
	if err := t.validateAllowedSubagentType(in); err != nil {
		return agentFailureToolResult(ctx, agentID, in.SubagentType, "", runStarted, err), nil
	}

	// Reject unsupported isolation modes before any provider call.
	if iso := strings.TrimSpace(strings.ToLower(in.Isolation)); iso != "" {
		if res, ok := t.checkIsolationSupported(ctx, iso); !ok {
			res.Data = AgentResultFromError(agentID, in.SubagentType, time.Since(runStarted).Milliseconds(), errors.New(res.Content))
			return res, nil
		}
	}

	parentModel := t.parentModelFromContext(ctx)
	requestedModel := parentModel
	if t.TeamMember {
		if strings.TrimSpace(in.Name) != "" {
			return ErrorResponsef("%s", toolRuntimeText(i18n.KeyToolAgentTeammateCannotSpawn)), nil
		}
		if strings.TrimSpace(in.TeamName) != "" {
			return ErrorResponsef("%s", toolRuntimeText(i18n.KeyToolAgentTeamNameUnavailable)), nil
		}
		if in.RunInBackground {
			return ErrorResponsef("%s", toolRuntimeText(i18n.KeyToolAgentTeammateBackground)), nil
		}
	}
	teamName := t.resolveSpawnTeamName(in)
	if strings.TrimSpace(in.TeamName) != "" && strings.TrimSpace(in.Name) == "" {
		return ErrorResponsef("%s", toolRuntimeText(i18n.KeyToolAgentTeammateNameRequired)), nil
	}
	if teamName != "" && strings.TrimSpace(in.Name) != "" {
		if t.shouldSpawnTeammate(teamName, in) {
			profile, err := t.resolveProfileForInput(in)
			if err != nil {
				return ErrorResponse(err), nil
			}
			in, err = t.applyAgentProfileDefaults(agentID, in, profile, skillAuthority)
			if err != nil {
				return agentFailureToolResult(ctx, agentID, in.SubagentType, "", runStarted, err), nil
			}
			return t.spawnTeammate(ctx, agentID, teamName, in, parentModel)
		}
		if strings.TrimSpace(in.TeamName) != "" {
			return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolAgentTeamMissing, teamName)), nil
		}
	}

	runOpts := agentLoopOptions{OverrideModel: requestedModel}
	profile, err := t.resolveProfileForInput(in)
	if err != nil {
		return ErrorResponse(err), nil
	}
	if shouldUseForkSubagent(in) {
		forkProfile, forkOpts, err := t.prepareForkSubagent(ctx, in)
		if err != nil {
			return ErrorResponse(err), nil
		}
		profile = forkProfile
		if strings.TrimSpace(forkOpts.OverrideModel) == "" {
			forkOpts.OverrideModel = requestedModel
		}
		runOpts = forkOpts
		in.SubagentType = forkSubagentType
		in.RunInBackground = true
	}
	runOpts.Profile = &profile
	in, err = t.applyAgentProfileDefaults(agentID, in, profile, skillAuthority)
	if err != nil {
		return agentFailureToolResult(ctx, agentID, in.SubagentType, "", runStarted, err), nil
	}
	emitter := t.progressForAgentRun(agentID, firstNonEmpty(in.SubagentType, profile.Name, "general-purpose"))
	if execution, ok := executioncontract.ToolExecutionContextFromContext(ctx); ok {
		emitter.ConfigureCorrelation(execution.SessionID, execution.TurnID, execution.WorkUnitID, execution.ToolUse.ID)
	}
	runOpts.Progress = emitter
	runOpts.Context = ctx
	if agentBackgroundTasksDisabled() {
		in.RunInBackground = false
	}

	if in.RunInBackground {
		if t.Background == nil {
			return ErrorResponsef("%s", toolRuntimeText(i18n.KeyToolRuntimeBackgroundUnavailable)), nil
		}
		session, snap, err := t.createRetainedAgentSessionWithOptions(agentID, in, runOpts)
		if err != nil {
			return agentFailureToolResult(ctx, agentID, in.SubagentType, "", runStarted, i18n.WrapError(i18n.KeyToolAgentBackgroundStartFailed, err)), nil
		}
		if err := session.enqueue(in.Prompt, nil); err != nil {
			return agentFailureToolResult(ctx, agentID, in.SubagentType, "", runStarted, i18n.WrapError(i18n.KeyToolAgentBackgroundStartFailed, err)), nil
		}
		canRead := t.canReadAgentOutputFileForInput(in)
		partial := AgentResultFromAsyncLaunch(agentID, in.SubagentType, firstNonEmpty(in.Description, in.Prompt), in.Prompt, snap.OutputPath, canRead)
		return agentToolResult(partial, formatAsyncAgentLaunchResult(agentID, firstNonEmpty(in.Description, in.Prompt), in.Prompt, snap.OutputPath, canRead), false), nil
	}

	if delay := agentAutoBackgroundDelay(); delay > 0 && t.Background != nil && !isOneShotBuiltInAgentType(in.SubagentType) {
		return t.runSubAgentWithAutoBackground(ctx, agentID, in, delay, runOpts)
	}

	transcriptWriter, transcriptCloser := openAgentTranscriptWriterForRun(agentID)
	if transcriptCloser != nil {
		defer transcriptCloser()
	}
	runCtx := withAgentProgressEmitter(ctx, emitter)
	runCtx = withAgentTranscriptWriter(runCtx, transcriptWriter)
	summary, err := t.runSubAgentWithOptions(runCtx, agentID, in, nil, runOpts)
	if err != nil {
		return agentIncompleteToolResult(summary, agentID, in.SubagentType, agentTranscriptPathFromWriter(transcriptWriter), runStarted, err, agentUsageIdentity{Provider: summary.Provider, Model: summary.Model}), nil
	}
	completed := AgentResultFromCompleted(summary, summary.TranscriptPath, summary.LatestToolUse)
	return agentToolResultWithUsage(completed, formatCompletedAgentResult(summary), false, summary.Usage, agentUsageIdentity{Provider: summary.Provider, Model: summary.Model}), nil
}

// RunScheduledPrompt is the synchronous Agent boundary used by the scheduler.
func (t *AgentTool) RunScheduledPrompt(ctx context.Context, agentID string, input agentcontract.Input) (string, error) {
	summary, err := t.runSubAgentWithOptions(ctx, agentID, agentcontract.Input{
		Description:  input.Description,
		Prompt:       input.Prompt,
		SubagentType: input.SubagentType,
		Name:         input.Name,
		TeamName:     input.TeamName,
		Isolation:    input.Isolation,
		CWD:          input.CWD,
		Color:        input.Color,
	}, nil, agentLoopOptions{OverrideModel: input.Model})
	return summary.Output, err
}

func (t *AgentTool) runSubAgentWithOptions(ctx context.Context, agentID string, in agentcontract.Input, writer io.Writer, opts agentLoopOptions) (agentRunSummary, error) {
	if opts.Context == nil {
		opts.Context = ctx
	}
	if t.Background != nil && !isOneShotBuiltInAgentType(in.SubagentType) {
		session, _, err := t.createRetainedAgentSessionWithOptions(agentID, in, opts)
		if err != nil {
			return agentRunSummary{}, err
		}
		return session.runSync(ctx, in.Prompt)
	}
	bundle, err := t.buildSubAgentLoopWithOptions(agentID, in, opts)
	if err != nil {
		return agentRunSummary{}, err
	}
	defer runAgentCleanup(bundle.Cleanup)
	summary, err := runAgentQueryLoop(ctx, bundle.Loop, bundle.Metadata, agentID, in.Prompt, writer)
	bundle.Metadata, _ = finalizeAgentWorktreeMetadata(bundle.Metadata)
	finalSummary := applyAgentSessionMetadata(summary, bundle.Metadata)
	if err == nil {
		err = agentRunOutcomeError(finalSummary)
	}
	return finalSummary, err
}

func (t *AgentTool) createRetainedAgentSessionWithOptions(agentID string, in agentcontract.Input, opts agentLoopOptions) (*backgroundAgentSession, *agentcontract.TaskSnapshot, error) {
	if t.Background == nil {
		return nil, nil, i18n.NewError(i18n.KeyToolAgentDeepBackgroundManagerUnavailable)
	}
	if runner := t.SessionRuntime().HookRunner; runner != nil {
		t.Background.SetHookRunner(runner)
	}
	bundle, err := t.buildSubAgentLoopWithOptions(agentID, in, opts)
	if err != nil {
		return nil, nil, err
	}
	var registrationLease func(func() error) error
	if t.SkillManager != nil && bundle.Metadata.SkillProjectGeneration != 0 {
		generation := skills.ProjectSourceGeneration(bundle.Metadata.SkillProjectGeneration)
		registrationLease = func(commit func() error) error {
			return t.SkillManager.WithProjectGenerationLease(generation, commit)
		}
	}
	session, snapshot, err := t.Background.registerAgentSession(
		agentID, strings.TrimSpace(in.Name), in.Prompt, firstNonEmpty(in.Description, in.Prompt),
		in, bundle.Loop, bundle.Metadata, bundle.Cleanup, bundle.Progress, registrationLease, opts.Context,
	)
	if err != nil {
		runAgentCleanup(bundle.Cleanup)
	}
	if session != nil {
		session.permissionHandler = bundle.PermissionHandler
	}
	if err == nil {
		if sessionID := backgroundTaskOwnerSessionID(opts.Context); sessionID != "" {
			t.Background.SetTaskOwnerSession(agentID, sessionID)
			if snapshot != nil {
				snapshot.OwnerSessionID = sessionID
			}
		}
	}
	if err == nil {
		if parentID := firstNonEmpty(t.TeamMemberID, strings.TrimSpace(os.Getenv("LUBAN_CODE_AGENT_ID"))); parentID != "" {
			t.Background.RegisterChildTask(parentID, agentID)
		}
	}
	return session, snapshot, err
}

func (t *AgentTool) runSubAgentWithAutoBackground(ctx context.Context, agentID string, in agentcontract.Input, delay time.Duration, opts agentLoopOptions) (types.ToolResult, error) {
	started := time.Now()
	// This run may become unattended while a permission prompt is already in
	// flight. Start fail-closed so detachment cannot strand an interactive ask.
	opts.ApprovalRouting = agentcontract.ApprovalFailClosed
	opts.PresentationSessionID = ""
	session, snap, err := t.createRetainedAgentSessionWithOptions(agentID, in, opts)
	if err != nil {
		return agentFailureToolResult(ctx, agentID, in.SubagentType, "", started, i18n.WrapError(i18n.KeyToolAgentDeepAutoBackgroundStartFailed, err)), nil
	}
	response := make(chan agentRunResponse, 1)
	if err := session.enqueue(in.Prompt, response); err != nil {
		return agentFailureToolResult(ctx, agentID, in.SubagentType, "", started, i18n.WrapError(i18n.KeyToolAgentDeepAutoBackgroundStartFailed, err)), nil
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		t.Background.AbortAgent(agentID)
		return agentIncompleteToolResult(agentRunSummary{}, agentID, in.SubagentType, "", started, ctx.Err()), nil
	case result := <-response:
		if result.err != nil {
			return agentIncompleteToolResult(result.summary, agentID, in.SubagentType, result.summary.TranscriptPath, started, result.err, agentUsageIdentity{Provider: result.summary.Provider, Model: result.summary.Model}), nil
		}
		completed := AgentResultFromCompleted(result.summary, result.summary.TranscriptPath, result.summary.LatestToolUse)
		return agentToolResultWithUsage(completed, formatCompletedAgentResult(result.summary), false, result.summary.Usage, agentUsageIdentity{Provider: result.summary.Provider, Model: result.summary.Model}), nil
	case <-timer.C:
		session.detachApprovalRouting()
		t.Background.MarkAgentDetached(agentID)
		t.Background.runManagedAsync(func() {
			t.forwardAutoBackgroundCompletion(agentID, session, response)
		})
		canRead := t.canReadAgentOutputFileForInput(in)
		partial := AgentResultFromAsyncLaunch(agentID, in.SubagentType, firstNonEmpty(in.Description, in.Prompt), in.Prompt, snap.OutputPath, canRead)
		return agentToolResult(partial, formatAsyncAgentLaunchResult(agentID, firstNonEmpty(in.Description, in.Prompt), in.Prompt, snap.OutputPath, canRead), false), nil
	}
}

func (t *AgentTool) forwardAutoBackgroundCompletion(agentID string, session *backgroundAgentSession, response <-chan agentRunResponse) {
	if t == nil || t.Background == nil || session == nil {
		return
	}
	var result agentRunResponse
	select {
	case result = <-response:
	case <-session.done:
		select {
		case result = <-response:
		default:
			return
		}
	}

	t.Background.mu.Lock()
	task := t.Background.tasks[agentID]
	t.Background.mu.Unlock()
	if task == nil {
		return
	}
	snapshot := task.snapshot()
	status := snapshot.Status
	if status == "" {
		status = "failed"
	}
	exitCode := -1
	if snapshot.ExitCode != nil {
		exitCode = *snapshot.ExitCode
	}
	t.Background.emitAgentCompletionNotification(t.Background.completionContextForTask(task), task, status, exitCode, result.summary)
}

func (t *AgentTool) buildSubAgentLoopWithOptions(agentID string, in agentcontract.Input, opts agentLoopOptions) (agentLoopBundle, error) {
	launch := t.captureLaunchRuntime()
	if opts.Context != nil {
		authority, err := skillauthority.Capture(opts.Context, t.SkillManager)
		if err != nil {
			return agentLoopBundle{}, err
		}
		if err := authority.ValidateRuntime(launch.session.ToolRuntime); err != nil {
			return agentLoopBundle{}, err
		}
	}
	skillProjectGeneration := opts.SkillProjectGeneration
	if skillProjectGeneration == 0 && opts.Context != nil {
		if exec, ok := executioncontract.ToolExecutionContextFromContext(opts.Context); ok {
			if generation, pinned := exec.SkillProjectGeneration(); pinned {
				skillProjectGeneration = skills.ProjectSourceGeneration(generation)
			}
		}
	}
	maxDepth := t.MaxDepth
	if maxDepth == 0 {
		maxDepth = DefaultMaxAgentDepth
	}
	if launch.provider == nil && opts.OverrideProvider == nil {
		return agentLoopBundle{}, i18n.NewError(i18n.KeyToolAgentDeepProviderNotConfigured)
	}
	if launch.registry == nil {
		return agentLoopBundle{}, i18n.NewError(i18n.KeyToolAgentDeepRegistryNotConfigured)
	}
	sourceRegistry := launch.registry
	permissionSnapshot := cloneToolRuntimeContext(launch.session.ToolRuntime)
	if opts.PermissionSnapshot != nil {
		permissionSnapshot = cloneToolRuntimeContext(*opts.PermissionSnapshot)
	}
	effectiveMode := canonicalAgentMode(permissionSnapshot.PermissionMode)
	if effectiveMode == "" {
		effectiveMode = permissionModeDefault
	}
	permissionSnapshot.PermissionMode = effectiveMode
	contextCacheLineageID := ""
	if opts.Context != nil {
		if execution, ok := executioncontract.ToolExecutionContextFromContext(opts.Context); ok {
			contextCacheLineageID = strings.TrimSpace(execution.CacheLineageID)
		}
	}
	cacheLineageID := firstNonEmpty(
		opts.CacheLineageID,
		contextCacheLineageID,
		permissionSnapshot.SessionID,
	)
	var err error
	profileCWD := strings.TrimSpace(in.CWD)
	if profileCWD == "" {
		profileCWD, _ = os.Getwd()
	}
	var profile agentProfile
	if opts.Profile != nil {
		profile = *opts.Profile
	} else {
		profileInput := in
		if strings.TrimSpace(profileInput.CWD) == "" {
			profileInput.CWD = profileCWD
		}
		profile, err = t.resolveProfileForInput(profileInput)
		if err != nil {
			return agentLoopBundle{}, err
		}
	}
	var mcpCleanup func()
	cleanupOnError := true
	defer func() {
		if cleanupOnError {
			runAgentCleanup(mcpCleanup)
		}
	}()
	isTeammateSession := t.TeamMember || opts.TeamMember
	teammateAgentID := agentID
	if t.TeamMember && strings.TrimSpace(t.TeamMemberID) != "" {
		teammateAgentID = strings.TrimSpace(t.TeamMemberID)
	}
	effectiveIsolation := firstNonEmpty(in.Isolation, profile.Isolation)
	if shouldSuppressOneShotWorktreeIsolation(in, profile) {
		effectiveIsolation = ""
	}
	agentCWD := ""
	var worktree *agentWorktree
	defer func() {
		if cleanupOnError && worktree != nil {
			_, _ = cleanupAgentWorktreeIfClean(agentcontract.SessionMetadata{
				CWD: agentCWD, WorktreeRepoRoot: worktree.RepoRoot,
				WorktreePath: worktree.Path, WorktreeBranch: worktree.Branch,
				WorktreeHeadCommit: worktree.HeadCommit,
			})
		}
	}()
	if strings.TrimSpace(opts.ReuseCWD) != "" {
		agentCWD, err = validateAgentCWD(opts.ReuseCWD, permissionSnapshot)
		if err != nil {
			return agentLoopBundle{}, err
		}
		effectiveIsolation = strings.TrimSpace(opts.ReuseIsolation)
		if effectiveIsolation == "" && strings.TrimSpace(opts.ReuseWorktreePath) != "" {
			effectiveIsolation = "worktree"
		}
	} else {
		if strings.TrimSpace(in.CWD) != "" && strings.EqualFold(effectiveIsolation, "worktree") {
			return agentLoopBundle{}, i18n.NewError(i18n.KeyToolAgentDeepCWDWorktreeConflict)
		}
		if strings.EqualFold(strings.TrimSpace(effectiveIsolation), "worktree") {
			parentProjectRoot, rootErr := validateAgentCWD(permissionSnapshot.ProjectRoot, permissionSnapshot)
			if rootErr != nil {
				return agentLoopBundle{}, i18n.WrapError(i18n.KeyToolAgentDeepWorktreeTrustedRootRequired, rootErr)
			}
			worktree, err = createAgentWorktree(agentID, parentProjectRoot)
			if err != nil {
				return agentLoopBundle{}, err
			}
			agentCWD = worktree.Path
		} else {
			agentCWD, err = validateAgentCWD(in.CWD, permissionSnapshot)
			if err != nil {
				return agentLoopBundle{}, err
			}
		}
	}
	// From this point onward the child registry, permission handler, and
	// persisted metadata must share one authority snapshot. The broader parent
	// snapshot above is used only to validate or create the trusted child CWD.
	permissionSnapshot = effectiveAgentPermissionSnapshot(permissionSnapshot, agentCWD)

	childProvider := opts.OverrideProvider
	if childProvider == nil {
		childProvider = launch.provider
	}
	if childProvider == nil {
		return agentLoopBundle{}, i18n.NewError(i18n.KeyToolAgentDeepProviderNotConfigured)
	}
	model := t.inheritedSubagentModel(opts.OverrideModel, childProvider)
	var subReg *registry.Registry
	if opts.UseExactTools {
		subReg = sourceRegistry.Clone()
	} else {
		subReg = registryForAgentProfileWithOptions(sourceRegistry, profile, agentToolRegistryOptions{
			IsAsync:    in.RunInBackground,
			AllowAgent: isTeammateSession && !in.RunInBackground,
		})
	}
	removePermissionTransitionToolsFromAgentRegistry(subReg)
	mcpSource, mcpCleanup, mcpPrepareErr := prepareAgentMCPRegistry(sourceRegistry, subReg, profile)
	if mcpPrepareErr != nil {
		return agentLoopBundle{}, mcpPrepareErr
	}
	if err := validateAgentMCPRequirements(mcpSource, profile); err != nil {
		return agentLoopBundle{}, err
	}
	readinessCtx := opts.Context
	if readinessCtx == nil {
		readinessCtx = context.Background()
	}
	readiness, readinessErr := t.waitForMCPReadiness(readinessCtx, profile)
	if readinessErr != nil {
		if opts.Progress != nil {
			opts.Progress.Finish(agentcontract.ProgressError, readinessErr.Error())
		}
		return agentLoopBundle{}, readinessErr
	}
	if opts.Progress != nil && len(readiness.Required) > 0 {
		opts.Progress.Emit(agentcontract.ProgressEvent{
			AgentID:      agentID,
			AgentType:    firstNonEmpty(in.SubagentType, profile.Name, "general-purpose"),
			Phase:        agentcontract.ProgressMCPReady,
			MessageCount: len(readiness.Ready),
			Detail:       strings.Join(readiness.Ready, ","),
		})
	}
	if len(profile.MCPServerConfigs) > 0 {
		parentProfile := profile
		parentProfile.MCPServerConfigs = nil
		parentProfile.MCPServers = agentMCPServersWithoutOverrides(profile.MCPServers, profile.MCPServerConfigs)
		registerAgentMCPDynamicTools(sourceRegistry, subReg, parentProfile)
		configProfile := profile
		configProfile.MCPServers = nil
		registerAgentMCPDynamicTools(mcpSource, subReg, configProfile)
	} else {
		registerAgentMCPDynamicTools(mcpSource, subReg, profile)
	}
	parentRuntime := launch.session
	toolRuntime := cloneToolRuntimeContext(permissionSnapshot)
	agentRuntimeProvider := agentRuntimeContextProvider{
		snapshot: toolRuntime,
		agentID:  agentID,
		cwd:      agentCWD,
		model:    model,
	}
	pinAgentRegistryPermissionRules(subReg, permissionSnapshot)
	bindInProcessAgentScopedTools(subReg, agentID, agentCWD)
	if agentCWD != "" {
		wrapRegistryForAgentCWD(subReg, agentCWD)
	} else {
		pinRegistryForAgentRuntime(subReg, agentRuntimeProvider, toolRuntime)
	}
	subReg.SetRuntimeContextProvider(agentRuntimeProvider)
	agentHooks := hookRunnerForProfile(parentRuntime.HookRunner, profile)
	approvalRouting := normalizeAgentApprovalRouting(opts.ApprovalRouting, agentcontract.ApprovalAttached)
	if in.RunInBackground && approvalRouting == agentcontract.ApprovalAttached {
		approvalRouting = agentcontract.ApprovalFailClosed
	}
	presentationSessionID := strings.TrimSpace(opts.PresentationSessionID)
	if presentationSessionID == "" {
		presentationSessionID = backgroundTaskOwnerSessionID(opts.Context)
	}
	if presentationSessionID == "" {
		presentationSessionID = strings.TrimSpace(permissionSnapshot.SessionID)
	}
	approvalRouting, presentationSessionID = safeAgentApprovalPresentation(approvalRouting, presentationSessionID)
	permissionHandler := agentPermissionHandlerForSnapshot(
		permissionSnapshot,
		launch.permissionHandler,
		approvalRouting,
		profile,
		presentationSessionID,
	)
	subAgentTool := &AgentTool{
		Provider:           childProvider,
		Registry:           subReg,
		System:             parentRuntime.System,
		Model:              model,
		ProgressiveContext: t.ProgressiveContext,
		ServiceTier:        t.ServiceTier,
		Background:         t.Background,
		Collaboration:      t.Collaboration,
		SkillManager:       t.SkillManager,
		HookRunner:         agentHooks,
		PermissionHandler:  permissionHandler,
		InlineProfiles:     t.InlineProfiles,
		AllowedAgentTypes:  allowedAgentTypesFromRules(profile.AllowedToolRules),
		TeamMember:         isTeammateSession,
		TeamMemberID:       teammateAgentID,
		NonInteractive:     true,
		Depth:              t.Depth + 1,
		MaxDepth:           maxDepth,
	}
	if subReg.Get(subAgentTool.Name()) != nil {
		subReg.Register(subAgentTool)
	}

	maxTurns := profile.MaxTurns
	systemPrompt := strings.TrimSpace(opts.OverrideSystem)
	if systemPrompt == "" {
		systemPrompt = buildAgentSystemPrompt(parentRuntime.System, profile, effectiveMode, agentCWD)
	}
	if strings.TrimSpace(agentCWD) != "" && subReg.Get("Bash") == nil && subReg.Get("PowerShell") == nil {
		notice := "Shell command tools are unavailable in this custom-CWD/worktree agent because the runtime has no enforceable filesystem sandbox. Use Read, Write, Edit, Glob, and Grep inside the assigned working directory."
		if strings.TrimSpace(systemPrompt) == "" {
			systemPrompt = notice
		} else {
			systemPrompt = strings.TrimSpace(systemPrompt) + "\n\n" + notice
		}
	}
	cfg := loop.Config{
		MaxTurns:               maxTurns,
		DisableMaxTurns:        maxTurns == 0,
		System:                 systemPrompt,
		Model:                  model,
		ReasoningEffort:        profile.ReasoningEffort,
		ServiceTier:            t.ServiceTier,
		MaxTokens:              16384,
		MaxContextTokens:       provider.LookupMaxContext(model),
		MaxOutputTokens:        16384,
		ProgressiveContext:     t.ProgressiveContext,
		SessionID:              agentID,
		CacheLineageID:         cacheLineageID,
		AgentID:                agentID,
		AgentType:              profile.Name,
		ProjectRoot:            toolRuntime.ProjectRoot,
		CWD:                    agentCWD,
		SkillManager:           t.SkillManager,
		SkillProjectGeneration: skillProjectGeneration,
		HookRunner:             agentHooks,
		PermissionHandler:      permissionHandler,
	}
	metadata := agentcontract.SessionMetadata{
		AgentType:              profile.Name,
		Provider:               agentProviderIdentity(childProvider),
		Model:                  model,
		CacheLineageID:         cacheLineageID,
		CWD:                    agentCWD,
		Mode:                   effectiveMode,
		Isolation:              strings.ToLower(strings.TrimSpace(effectiveIsolation)),
		SkipInitialPrompt:      opts.SkipInitialPrompt,
		TeamMember:             isTeammateSession,
		ApprovalRouting:        approvalRouting,
		PresentationSessionID:  presentationSessionID,
		SkillProjectGeneration: uint64(skillProjectGeneration),
	}
	metadataSnapshot := cloneToolRuntimeContext(permissionSnapshot)
	metadata.PermissionSnapshot = &metadataSnapshot
	if worktree != nil {
		metadata.WorktreeRepoRoot = worktree.RepoRoot
		metadata.WorktreePath = worktree.Path
		metadata.WorktreeBranch = worktree.Branch
		metadata.WorktreeHeadCommit = worktree.HeadCommit
	}
	if strings.TrimSpace(opts.ReuseWorktreePath) != "" {
		metadata.WorktreeRepoRoot = inferRepoRootFromAgentWorktree(opts.ReuseWorktreePath)
		metadata.WorktreePath = filepath.Clean(opts.ReuseWorktreePath)
		metadata.WorktreeBranch = strings.TrimSpace(opts.ReuseWorktreeBranch)
		metadata.WorktreeHeadCommit = strings.TrimSpace(opts.ReuseWorktreeHeadCommit)
	}
	ql := loop.New(childProvider, subReg, cfg)
	if len(opts.InitialMessages) > 0 {
		initialMessages := cloneAgentMessages(opts.InitialMessages)
		if shouldAppendForkWorktreeNotice(metadata) {
			initialMessages = append(initialMessages, types.UserMessage(buildForkWorktreeNotice(firstNonEmpty(opts.ForkParentCWD, profileCWD), metadata.WorktreePath)))
		}
		ql.SetMessages(initialMessages)
	}
	cleanupOnError = false
	snapshotHandler, _ := permissionHandler.(*agentPermissionSnapshotHandler)
	return agentLoopBundle{Loop: ql, Metadata: metadata, Cleanup: mcpCleanup, PermissionHandler: snapshotHandler, Progress: opts.Progress}, nil
}

func (t *AgentTool) RestoreAgentSession(agentID string, record runtimestore.RuntimeTaskRecord) error {
	if t.Background == nil {
		return i18n.NewError(i18n.KeyToolAgentDeepBackgroundManagerUnavailable)
	}
	if record.AgentInput == nil {
		return i18n.NewError(i18n.KeyToolAgentDeepSessionMissingInput, agentID)
	}
	metadata := record.AgentMetadata
	if metadata == nil {
		metadata = &agentcontract.SessionMetadata{}
	}
	if metadata.PermissionSnapshot == nil {
		return i18n.WrapError(i18n.KeyToolAgentDeepErrorCause, errAgentPermissionSnapshotUnavailable)
	}
	trusted, ok := t.Background.trustedAgentResume(agentID)
	if !ok || !trustedAgentResumeMatchesRecord(trusted, *record.AgentInput, *metadata) {
		return i18n.WrapError(i18n.KeyToolAgentDeepErrorCause, errAgentResumeContextUntrusted)
	}
	if err := ensureAgentWorktreePath(*metadata); err != nil {
		return err
	}
	currentProvider := snapshotAgentProvider(t.Provider)
	if currentProvider == nil {
		return i18n.NewError(i18n.KeyToolAgentDeepProviderNotConfigured)
	}
	overrideModel := strings.TrimSpace(metadata.Model)
	currentProviderID := agentProviderIdentity(currentProvider)
	persistedProviderID := strings.TrimSpace(metadata.Provider)
	validApprovalRouting := metadata.ApprovalRouting == agentcontract.ApprovalAttached ||
		metadata.ApprovalRouting == agentcontract.ApprovalFailClosed || metadata.ApprovalRouting == agentcontract.ApprovalParentSession
	if persistedProviderID == "" || strings.TrimSpace(metadata.Model) == "" || currentProviderID == "" || !validApprovalRouting {
		return i18n.WrapError(i18n.KeyToolAgentDeepErrorCause, errAgentResumeContextUntrusted)
	}
	if !strings.EqualFold(persistedProviderID, currentProviderID) {
		overrideModel = strings.TrimSpace(currentProvider.ModelID())
	}
	persistedSnapshot := cloneToolRuntimeContext(*metadata.PermissionSnapshot)
	approvalRouting := metadata.ApprovalRouting
	presentationSessionID := firstNonEmpty(metadata.PresentationSessionID, record.OwnerSessionID)
	bundle, err := t.buildSubAgentLoopWithOptions(agentID, *record.AgentInput, agentLoopOptions{
		ReuseCWD:                metadata.CWD,
		ReuseIsolation:          metadata.Isolation,
		ReuseWorktreePath:       metadata.WorktreePath,
		ReuseWorktreeBranch:     metadata.WorktreeBranch,
		ReuseWorktreeHeadCommit: metadata.WorktreeHeadCommit,
		OverrideModel:           overrideModel,
		OverrideProvider:        currentProvider,
		CacheLineageID:          firstNonEmpty(metadata.CacheLineageID, record.OwnerSessionID),
		TeamMember:              metadata.TeamMember,
		PermissionSnapshot:      &persistedSnapshot,
		ApprovalRouting:         approvalRouting,
		PresentationSessionID:   presentationSessionID,
		SkillProjectGeneration:  skills.ProjectSourceGeneration(metadata.SkillProjectGeneration),
	})
	if err != nil {
		return err
	}
	if len(record.AgentMessages) > 0 {
		// Drop orphaned tool_use blocks before replaying — if the agent
		// was paused mid tool-call the API will 400 with
		// unmatched-tool_use_id when we resume, leaving the session stuck.
		filtered := FilterIncompleteToolCalls(record.AgentMessages)
		bundle.Loop.SetMessages(filtered)
	}
	// Bump worktree mtime so stale-cleanup sweepers don't garbage-collect a
	// directory that's still referenced by this paused session.
	if metadata != nil && strings.TrimSpace(metadata.WorktreePath) != "" {
		_ = touchAgentWorktreePath(metadata.WorktreePath)
	}
	if bundle.Progress != nil && record.LatestProgress != nil {
		progress := record.LatestProgress
		bundle.Progress.ConfigureCorrelation(progress.SessionID, progress.TurnID, progress.WorkUnitID, progress.ParentToolUseID)
	}
	session, _, err := t.Background.registerAgentSessionFromRecord(record, bundle.Loop, bundle.Metadata, bundle.Cleanup, bundle.Progress)
	if session != nil {
		session.permissionHandler = bundle.PermissionHandler
	}
	return err
}

func validateAgentCWD(raw string, parentScope types.ToolRuntimeContext) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	if !filepath.IsAbs(raw) {
		return "", i18n.NewError(i18n.KeyToolAgentDeepCWDAbsoluteRequired)
	}
	info, err := os.Stat(raw)
	if err != nil {
		return "", i18n.WrapError(i18n.KeyToolAgentDeepCWDInaccessible, err)
	}
	if !info.IsDir() {
		return "", i18n.NewError(i18n.KeyToolAgentDeepCWDDirectoryRequired)
	}
	cleaned := filepath.Clean(raw)
	allowedDirs := append([]string(nil), parentScope.AllowedDirs...)
	if len(allowedDirs) == 0 && strings.TrimSpace(parentScope.ProjectRoot) != "" {
		allowedDirs = []string{parentScope.ProjectRoot}
	}
	if len(allowedDirs) > 0 {
		resolved := cleaned
		if evaluated, evalErr := filepath.EvalSymlinks(cleaned); evalErr == nil {
			resolved = evaluated
		}
		if !toolbase.PathWithinAllowedDirs(cleaned, allowedDirs) || !toolbase.PathWithinAllowedDirs(resolved, allowedDirs) {
			return "", i18n.NewError(i18n.KeyToolAgentDeepCWDOutsideParentScope)
		}
	}
	return cleaned, nil
}

func runAgentQueryLoop(ctx context.Context, subLoop *loop.QueryLoop, metadata agentcontract.SessionMetadata, agentID, prompt string, writer io.Writer) (summary agentRunSummary, returnErr error) {
	var result strings.Builder
	var liveOutput agentLiveOutputBuffer
	var lastError string
	var lastRuntimeError error
	var modelWarnings []string
	var totalUsage *types.Usage
	var lastRequestUsage *types.Usage
	var latestToolUse string
	verificationLabels := make(map[string]string)
	toolNamesByUseID := make(map[string]string)
	var verificationRefs []string
	toolUseCount := 0
	messageCount := 0
	startTime := time.Now()
	agentType := firstNonEmpty(metadata.AgentType, "general-purpose")
	transcriptWriter := agentTranscriptWriterFromContext(ctx)
	transcriptPath := firstNonEmpty(agentTranscriptPathFromWriter(transcriptWriter), agentTranscriptPathFromWriter(writer))
	emitter := agentProgressEmitterFromContext(ctx)
	lastProgressAt := time.Time{}
	emitAssistantProgress := func(force bool) {
		if emitter == nil {
			return
		}
		now := time.Now()
		if !force && !lastProgressAt.IsZero() && now.Sub(lastProgressAt) < 100*time.Millisecond {
			return
		}
		lastProgressAt = now
		tokens := 0
		var usage *types.Usage
		var requestUsage *types.Usage
		if totalUsage != nil {
			tokens = totalUsage.InputTokens + totalUsage.OutputTokens
			usageCopy := *totalUsage
			usage = &usageCopy
		}
		if lastRequestUsage != nil {
			requestUsageCopy := *lastRequestUsage
			requestUsage = &requestUsageCopy
		}
		emitter.Emit(agentcontract.ProgressEvent{
			AgentID: agentID, AgentType: agentType, Phase: agentcontract.ProgressAssistant,
			MessageCount: messageCount, LatestTool: latestToolUse, TokensUsed: tokens,
			Provider: metadata.Provider, Model: metadata.Model, Usage: usage, LastRequestUsage: requestUsage,
			PartialText: boundedAgentProgressTail(liveOutput.snapshot()),
		})
	}
	if emitter != nil {
		emitter.Emit(agentcontract.ProgressEvent{AgentID: agentID, AgentType: agentType, Phase: agentcontract.ProgressStart})
	}
	defer func() {
		phase := agentcontract.ProgressCompleted
		detail := ""
		switch summary.Outcome {
		case agentcontract.RunOutcomeCancelled:
			phase = agentcontract.ProgressAborted
			detail = summary.TerminalReason
		case agentcontract.RunOutcomePartial, agentcontract.RunOutcomeFailed, agentcontract.RunOutcomeTimedOut, agentcontract.RunOutcomeInterrupted:
			phase = agentcontract.ProgressError
			detail = summary.TerminalReason
		default:
			if ctx.Err() != nil {
				phase = agentcontract.ProgressAborted
				detail = ctx.Err().Error()
			} else if returnErr != nil {
				phase = agentcontract.ProgressError
				detail = returnErr.Error()
			}
		}
		if emitter != nil {
			emitter.Finish(phase, detail)
		}
		_ = writeAgentTranscriptRecord(transcriptWriter, map[string]any{
			"type":    "terminal",
			"agentId": agentID,
			"phase":   phase,
			"detail":  detail,
		})
	}()

	agentCtx := ctx
	handleEvent := func(event stream.Event) {
		switch event.Type {
		case stream.EventText:
			result.WriteString(event.Text)
			liveOutput.appendAssistant(event.Text)
			if writer != nil {
				_, _ = io.WriteString(writer, event.Text)
			}
			if event.Text != "" {
				_ = writeAgentTranscriptRecord(transcriptWriter, map[string]any{
					"type":    "assistant",
					"agentId": agentID,
					"message": map[string]any{"role": "assistant", "content": event.Text},
				})
				emitAssistantProgress(false)
			}
		case stream.EventToolUse:
			toolUseCount++
			name := strings.TrimSpace(event.Text)
			if event.ToolUse != nil {
				name = strings.TrimSpace(event.ToolUse.Name)
			}
			if name != "" {
				latestToolUse = name
			}
			if event.ToolUse != nil {
				toolNamesByUseID[strings.TrimSpace(event.ToolUse.ID)] = name
				liveOutput.appendToolCall(*event.ToolUse)
				if label := agentVerificationLabel(*event.ToolUse); label != "" {
					verificationLabels[event.ToolUse.ID] = label
				}
			}
			if emitter != nil {
				emitter.Emit(agentcontract.ProgressEvent{AgentID: agentID, AgentType: agentType, Phase: agentcontract.ProgressToolUse, MessageCount: messageCount, LatestTool: latestToolUse, PartialText: boundedAgentProgressTail(liveOutput.snapshot())})
			}
			_ = writeAgentTranscriptRecord(transcriptWriter, map[string]any{
				"type":    "tool_use",
				"agentId": agentID,
				"tool":    latestToolUse,
				"event":   event,
			})
		case stream.EventToolResult:
			if event.ToolResult != nil {
				toolUseID := strings.TrimSpace(event.ToolResult.ToolUseID)
				toolName := strings.TrimSpace(toolNamesByUseID[toolUseID])
				if toolName == "" {
					toolName = latestToolUse
				}
				liveOutput.appendToolResult(toolName, *event.ToolResult)
				if emitter != nil {
					emitter.Emit(agentcontract.ProgressEvent{AgentID: agentID, AgentType: agentType, Phase: agentcontract.ProgressToolUse, MessageCount: messageCount, LatestTool: toolName, PartialText: boundedAgentProgressTail(liveOutput.snapshot())})
				}
				delete(toolNamesByUseID, toolUseID)
			}
			if event.ToolResult != nil && agentToolResultSucceeded(*event.ToolResult) {
				if label := verificationLabels[event.ToolResult.ToolUseID]; label != "" {
					anchor := "tool_result:" + event.ToolResult.ToolUseID
					if transcriptPath != "" {
						anchor = transcriptPath + "#tool_result:" + event.ToolResult.ToolUseID
					}
					verificationRefs = appendUniqueAgentRunRef(verificationRefs, fmt.Sprintf("%s (%s)", label, anchor))
				}
			}
			_ = writeAgentTranscriptRecord(transcriptWriter, map[string]any{
				"type":    "tool_result",
				"agentId": agentID,
				"event":   event,
			})
		case stream.EventProviderUsage:
			accumulateAgentUsage(&totalUsage, event.Usage)
		case stream.EventTurnEnd:
			accumulateAgentUsage(&totalUsage, event.Usage)
			lastRequestUsage = cloneUsagePointer(event.Usage)
			messageCount++
			emitAssistantProgress(true)
		case stream.EventError:
			lastError, lastRuntimeError = agentRuntimeErrorModelMessage(agentID, event)
		case stream.EventSystemWarning:
			message, _ := agentRuntimeWarningModelMessage(agentID, event)
			modelWarnings = append(modelWarnings, message)
			_ = writeAgentTranscriptRecord(transcriptWriter, map[string]any{
				"type": "system_warning", "agentId": agentID, "message": message,
			})
		}
	}

	runOnce := func(message string) error {
		lastError = ""
		lastRuntimeError = nil
		_ = writeAgentTranscriptRecord(transcriptWriter, map[string]any{
			"type":    "user",
			"agentId": agentID,
			"message": map[string]any{"role": "user", "content": message},
		})
		return subLoop.Run(agentCtx, message, handleEvent)
	}
	runPrepared := func() error {
		lastError = ""
		lastRuntimeError = nil
		return subLoop.RunPrepared(agentCtx, handleEvent)
	}
	finishSummary := func(output string, outcome agentcontract.RunOutcome, reason string) agentRunSummary {
		if len(modelWarnings) > 0 {
			warningText := strings.Join(modelWarnings, "\n")
			if strings.TrimSpace(output) == "" {
				output = warningText
			} else {
				output = warningText + "\n\n" + output
			}
		}
		totalTokens := 0
		if totalUsage != nil {
			totalTokens = totalUsage.InputTokens + totalUsage.OutputTokens
		}
		return agentRunSummary{
			AgentID:          agentID,
			AgentType:        metadata.AgentType,
			Provider:         metadata.Provider,
			Model:            metadata.Model,
			Prompt:           prompt,
			Output:           output,
			ToolUseCount:     toolUseCount,
			TotalTokens:      totalTokens,
			TotalDuration:    time.Since(startTime).Milliseconds(),
			Usage:            totalUsage,
			TranscriptPath:   transcriptPath,
			LatestToolUse:    latestToolUse,
			Outcome:          outcome,
			TerminalReason:   reason,
			VerificationRefs: append([]string(nil), verificationRefs...),
		}
	}

	startContext := subagentStartHookContext(agentCtx, subLoop.HookRunner(), agentID, agentType)
	var err error
	if metadata.SkipInitialPrompt {
		if strings.TrimSpace(startContext) != "" {
			err = runOnce(formatSystemReminder(startContext))
		} else {
			err = runPrepared()
		}
	} else {
		runPrompt := prompt
		if strings.TrimSpace(startContext) != "" {
			runPrompt = formatSystemReminder(startContext) + "\n\n" + prompt
		}
		err = runOnce(runPrompt)
	}
	if err == nil {
		for i := 0; i < subagentStopHookContinuationLimit; i++ {
			continuation := subagentStopHookContinuation(agentCtx, subLoop.HookRunner(), agentID, agentType, transcriptPath, result.String())
			if continuation == "" {
				break
			}
			err = runOnce(formatSystemReminder(continuation))
			if err != nil {
				break
			}
		}
	}

	if err != nil {
		outcome, reason := classifyAgentRunTermination(err, result.Len() > 0)
		var maxTurnsErr *loop.MaxTurnsError
		if errors.As(err, &maxTurnsErr) {
			return finishSummary(result.String(), outcome, reason), nil
		}
		if lastRuntimeError != nil {
			privateCause := errors.Join(err, lastRuntimeError)
			if result.Len() > 0 {
				output := strings.TrimRight(result.String(), "\n") + "\n\n" + i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolAgentDeepEncounteredError, lastError)
				return finishSummary(output, outcome, reason), i18n.WrapInternalError(i18n.KeyRuntimeErrorPublicSummary, privateCause)
			}
			return finishSummary("", outcome, reason), i18n.WrapInternalError(i18n.KeyRuntimeErrorPublicSummary, privateCause)
		}
		if result.Len() > 0 {
			output := strings.TrimRight(result.String(), "\n") + "\n\n" + i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolAgentDeepEncounteredError, err)
			detail := any(err)
			if lastError != "" {
				detail = lastError
			}
			return finishSummary(output, outcome, reason), i18n.WrapError(i18n.KeyToolAgentDeepRunFailedWithDetail, err, detail)
		}
		detail := any(err)
		if lastError != "" {
			detail = lastError
		}
		return finishSummary("", outcome, reason), i18n.WrapError(i18n.KeyToolAgentDeepRunFailedWithDetail, err, detail)
	}

	return finishSummary(result.String(), agentcontract.RunOutcomeSucceeded, "completed"), nil
}

// agentRuntimeErrorModelMessage projects a subagent EventError for the parent
// model while returning the private RuntimeEvent separately for error-chain and
// explicitly authorized audit use. ProjectRoot is intentionally excluded from
// the identity shared with RuntimeEvent.
func agentRuntimeErrorModelMessage(agentID string, event stream.Event) (string, error) {
	sessionID := ""
	if queryMarker := strings.Index(event.TurnID, ":query-"); queryMarker > 0 {
		sessionID = event.TurnID[:queryMarker]
	}
	runtimeError := runtimeevent.NewErrorEvent(types.RuntimeIdentity{
		SessionID: sessionID, TurnID: event.TurnID, ToolUseID: event.ToolUseID,
		WorkUnitID: event.WorkUnitID, ActorID: firstNonEmpty(event.ActorID, agentID), ActorType: event.ActorType,
	}, event.Text, event.Error, event.Metadata)
	projection, err := runtimeevent.NewAudienceProjector().Project(runtimeError, runtimeevent.ProjectionOptions{
		Audience: runtimeevent.AudienceModel, Redaction: runtimeevent.RedactionStrict,
	})
	if err != nil {
		return i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeErrorPublicSummary), runtimeError
	}
	return projection.Message, runtimeError
}

// agentRuntimeWarningModelMessage performs the Model/Strict projection used
// when a subagent warning is carried back to its parent model. The returned
// RuntimeEvent preserves private error identity for explicit diagnostics while
// the string contains semantic public copy only.
func agentRuntimeWarningModelMessage(agentID string, event stream.Event) (string, error) {
	warning := runtimeevent.SystemWarningRuntimeEvent(event)
	if warning.ActorID == "" {
		warning.ActorID = agentID
	}
	projection, err := runtimeevent.NewAudienceProjector().Project(warning, runtimeevent.ProjectionOptions{
		Audience: runtimeevent.AudienceModel, Redaction: runtimeevent.RedactionStrict,
	})
	if err != nil {
		return i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeWarningPublicSummary), warning
	}
	return projection.Message, warning
}

const maxAgentProgressTailRunes = 2400

func boundedAgentProgressTail(value string) string {
	runes := []rune(value)
	if len(runes) <= maxAgentProgressTailRunes {
		return value
	}
	return string(runes[len(runes)-maxAgentProgressTailRunes:])
}

var agentShellCommandSeparators = regexp.MustCompile(`(?:\r?\n|&&|\|\||;)`)

func agentVerificationLabel(toolUse types.ToolUseBlock) string {
	toolName := strings.TrimSpace(toolUse.Name)
	switch strings.ToLower(toolName) {
	case "bash", "shell", "powershell":
	default:
		return ""
	}
	command, _ := toolUse.Input["command"].(string)
	for _, segment := range agentShellCommandSeparators.Split(command, -1) {
		if label := agentVerificationCommandLabel(segment); label != "" {
			return toolName + ":" + label
		}
	}
	return ""
}

func agentVerificationCommandLabel(segment string) string {
	fields := strings.Fields(strings.TrimSpace(segment))
	for len(fields) > 0 && strings.Contains(fields[0], "=") && !strings.HasPrefix(fields[0], "=") {
		fields = fields[1:]
	}
	if len(fields) > 0 && strings.EqualFold(path.Base(fields[0]), "env") {
		fields = fields[1:]
		for len(fields) > 0 && strings.Contains(fields[0], "=") && !strings.HasPrefix(fields[0], "=") {
			fields = fields[1:]
		}
	}
	if len(fields) == 0 {
		return ""
	}
	executable := strings.ToLower(path.Base(strings.Trim(fields[0], `"'`)))
	args := make([]string, len(fields)-1)
	for index, arg := range fields[1:] {
		args[index] = strings.ToLower(strings.Trim(arg, `"'`))
	}
	hasArg := func(want string) bool {
		for _, arg := range args {
			if arg == want {
				return true
			}
		}
		return false
	}
	switch executable {
	case "go":
		if len(args) > 0 && (args[0] == "test" || args[0] == "vet") {
			return "go " + args[0]
		}
	case "pytest", "pytest.exe":
		return "pytest"
	case "python", "python3", "python.exe":
		if len(args) > 1 && args[0] == "-m" && args[1] == "pytest" {
			return "pytest"
		}
	case "npm", "pnpm", "yarn", "bun":
		if hasArg("test") || hasArg("lint") || hasArg("typecheck") || hasArg("check") {
			return executable + " verification"
		}
	case "cargo":
		if len(args) > 0 && (args[0] == "test" || args[0] == "clippy") {
			return "cargo " + args[0]
		}
	case "dotnet":
		if len(args) > 0 && args[0] == "test" {
			return "dotnet test"
		}
	case "mvn", "mvnw", "gradle", "gradlew", "make":
		if hasArg("test") || hasArg("check") || hasArg("verify") {
			return executable + " verification"
		}
	case "golangci-lint", "staticcheck", "tsc":
		return executable
	}
	return ""
}

func agentToolResultSucceeded(result types.ToolResultBlock) bool {
	if result.IsError {
		return false
	}
	switch result.Outcome {
	case "", types.ToolOutcomeSucceeded:
		return true
	default:
		return false
	}
}

func accumulateAgentUsage(total **types.Usage, usage *types.Usage) {
	if total == nil || usage == nil {
		return
	}
	if *total == nil {
		*total = &types.Usage{}
	}
	(*total).InputTokens += usage.InputTokens
	(*total).OutputTokens += usage.OutputTokens
	(*total).CacheCreationInputTokens += usage.CacheCreationInputTokens
	(*total).CacheReadInputTokens += usage.CacheReadInputTokens
	(*total).ServerToolUse.WebSearchRequests += usage.ServerToolUse.WebSearchRequests
	(*total).ServerToolUse.WebFetchRequests += usage.ServerToolUse.WebFetchRequests
}

const subagentStopHookContinuationLimit = 3

func formatSystemReminder(text string) string {
	return "<system-reminder>\n" + strings.TrimSpace(text) + "\n</system-reminder>"
}

func subagentStartHookContext(ctx context.Context, runner *hooks.Runner, agentID, agentType string) string {
	if runner == nil || !runner.HasHooks(hooks.HookSubagentStart) {
		return ""
	}
	executions := runner.RunDetailedObserved(ctx, hooks.HookSubagentStart, hooks.HookInput{
		AgentID:   agentID,
		AgentType: agentType,
	})
	var parts []string
	for _, execution := range executions {
		output := execution.Output
		if text := strings.TrimSpace(output.SystemReminder); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func subagentStopHookContinuation(ctx context.Context, runner *hooks.Runner, agentID, agentType, transcriptPath, lastAssistantMessage string) string {
	if runner == nil || !runner.HasHooks(hooks.HookSubagentStop) {
		return ""
	}
	executions := runner.RunDetailedObserved(ctx, hooks.HookSubagentStop, hooks.HookInput{
		AgentID:              agentID,
		AgentType:            agentType,
		AgentTranscriptPath:  transcriptPath,
		LastAssistantMessage: strings.TrimSpace(lastAssistantMessage),
		Result:               strings.TrimSpace(lastAssistantMessage),
	})
	var parts []string
	for _, execution := range executions {
		output := execution.Output
		if output.ExitCode != 2 && !output.Block {
			continue
		}
		text := strings.TrimSpace(output.Stderr)
		if text == "" {
			text = strings.TrimSpace(output.ExecutionError)
		}
		if text == "" {
			text = strings.TrimSpace(output.SystemReminder)
		}
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func agentTranscriptPathFromWriter(writer io.Writer) string {
	if writer == nil {
		return ""
	}
	type named interface {
		Name() string
	}
	if n, ok := writer.(named); ok {
		return n.Name()
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func defaultAgentAutoBackgroundDelay() time.Duration {
	if isTruthyEnv(os.Getenv("LUBAN_AUTO_BACKGROUND_TASKS")) {
		return 120 * time.Second
	}
	return 0
}

func shouldUseForkSubagent(in agentcontract.Input) bool {
	return strings.TrimSpace(in.SubagentType) == "" && isForkSubagentEnabled()
}

func (t *AgentTool) validateAllowedSubagentType(in agentcontract.Input) error {
	if len(t.AllowedAgentTypes) == 0 || shouldUseForkSubagent(in) {
		return nil
	}
	requested := strings.TrimSpace(in.SubagentType)
	if requested == "" {
		requested = "general-purpose"
	}
	for _, allowed := range t.AllowedAgentTypes {
		if strings.EqualFold(strings.TrimSpace(allowed), requested) {
			return nil
		}
	}
	return i18n.NewError(
		i18n.KeyToolAgentDeepSubagentTypeNotAllowed,
		requested,
		strings.Join(t.AllowedAgentTypes, ", "),
	)
}

func isForkSubagentEnabled() bool {
	return isTruthyEnv(os.Getenv("LUBAN_CODE_FORK_SUBAGENT"))
}

func forkAgentProfile() agentProfile {
	return agentProfile{
		Name:            forkSubagentType,
		SystemPrefix:    "",
		AllowedTools:    nil,
		DisallowedTools: map[string]struct{}{},
		Model:           "inherit",
		MaxTurns:        200,
	}
}

func (t *AgentTool) prepareForkSubagent(ctx context.Context, in agentcontract.Input) (agentProfile, agentLoopOptions, error) {
	execCtx, ok := executioncontract.ToolExecutionContextFromContext(ctx)
	if !ok {
		return agentProfile{}, agentLoopOptions{}, i18n.NewError(i18n.KeyToolAgentDeepForkContextRequired)
	}
	if isInForkChildMessages(execCtx.Messages) || messageContainsForkBoilerplate(execCtx.AssistantMessage) {
		return agentProfile{}, agentLoopOptions{}, i18n.NewError(i18n.KeyToolAgentDeepForkNestedUnavailable)
	}
	profile := forkAgentProfile()
	parentCWD, _ := os.Getwd()
	return profile, agentLoopOptions{
		InitialMessages:   buildForkedMessages(in.Prompt, execCtx),
		OverrideSystem:    firstNonEmpty(execCtx.System, t.SessionRuntime().System),
		OverrideModel:     firstNonEmpty(execCtx.Model, t.Model),
		UseExactTools:     true,
		ForkParentCWD:     parentCWD,
		SkipInitialPrompt: true,
		ApprovalRouting:   agentcontract.ApprovalParentSession,
	}, nil
}

func buildForkedMessages(directive string, execCtx executioncontract.ToolExecutionContext) []types.Message {
	messages := cloneAgentMessages(execCtx.Messages)
	assistant := cloneAgentMessage(execCtx.AssistantMessage)
	toolUses := assistant.GetToolUses()
	if len(toolUses) == 0 && strings.TrimSpace(execCtx.ToolUse.ID) != "" {
		toolUses = []types.ToolUseBlock{execCtx.ToolUse}
	}
	if len(toolUses) == 0 {
		return append(messages, types.UserMessage(buildForkChildMessage(directive)))
	}
	messages = append(messages, assistant)
	content := make([]types.ContentBlock, 0, len(toolUses)+1)
	for _, toolUse := range toolUses {
		content = append(content, types.ToolResultBlock{
			Type:      types.ContentTypeToolResult,
			ToolUseID: toolUse.ID,
			ToolType:  toolUse.ToolType,
			Content:   forkPlaceholderToolResultText(),
		})
	}
	content = append(content, types.TextBlock{
		Type: types.ContentTypeText,
		Text: buildForkChildMessage(directive),
	})
	messages = append(messages, types.Message{Role: types.RoleUser, Content: content})
	return messages
}

func buildForkChildMessage(directive string) string {
	return fmt.Sprintf(`<%s>
STOP. READ THIS FIRST.

You are a forked worker process. You are NOT the main agent.

RULES (non-negotiable):
1. Your system prompt says "default to forking." IGNORE IT — that's for the parent. You ARE the fork. Do NOT spawn sub-agents; execute directly.
2. Do NOT converse, ask questions, or suggest next steps
3. Do NOT editorialize or add meta-commentary
4. USE your tools directly: Bash, Read, Write, etc.
5. If you modify files, commit your changes before reporting. Include the commit hash in your report.
6. Do NOT emit text between tool calls. Use tools silently, then report once at the end.
7. Stay strictly within your directive's scope. If you discover related systems outside your scope, mention them in one sentence at most — other workers cover those areas.
8. Keep your report under 500 words unless the directive specifies otherwise. Be factual and concise.
9. Your response MUST begin with "Scope:". No preamble, no thinking-out-loud.
10. REPORT structured facts, then stop

Output format (plain text labels, not markdown headers):
  Scope: <echo back your assigned scope in one sentence>
  Result: <the answer or key findings, limited to the scope above>
  Key files: <relevant file paths — include for research tasks>
  Files changed: <list with commit hash — include only if you modified files>
  Issues: <list — include only if there are issues to flag>
</%s>

%s%s`, forkBoilerplateTag, forkBoilerplateTag, forkDirectivePrefix, strings.TrimSpace(directive))
}

func buildForkWorktreeNotice(parentCWD, worktreeCWD string) string {
	return fmt.Sprintf("You've inherited the conversation context above from a parent agent working in %s. You are operating in an isolated git worktree at %s — same repository, same relative file structure, separate working copy. Paths in the inherited context refer to the parent's working directory; translate them to your worktree root. Re-read files before editing if the parent may have modified them since they appear in the context. Your changes stay in this worktree and will not affect the parent's files.", parentCWD, worktreeCWD)
}

func isInForkChildMessages(messages []types.Message) bool {
	for _, message := range messages {
		if messageContainsForkBoilerplate(message) {
			return true
		}
	}
	return false
}

func messageContainsForkBoilerplate(message types.Message) bool {
	for _, block := range message.Content {
		if text, ok := block.(types.TextBlock); ok && strings.Contains(text.Text, "<"+forkBoilerplateTag+">") {
			return true
		}
	}
	return false
}

func cloneAgentMessages(messages []types.Message) []types.Message {
	out := make([]types.Message, len(messages))
	for i := range messages {
		out[i] = cloneAgentMessage(messages[i])
	}
	return out
}

func cloneAgentMessage(message types.Message) types.Message {
	// Preserve the complete value, including content-bound internal provenance;
	// the cloned content is byte-identical so the authenticator remains valid.
	out := message
	out.Content = make([]types.ContentBlock, len(message.Content))
	copy(out.Content, message.Content)
	return out
}

func (t *AgentTool) parentModelFromContext(ctx context.Context) string {
	if execCtx, ok := executioncontract.ToolExecutionContextFromContext(ctx); ok {
		return strings.TrimSpace(execCtx.Model)
	}
	return ""
}

func (t *AgentTool) inheritedSubagentModel(override string, childProvider provider.Provider) string {
	if model := strings.TrimSpace(override); model != "" {
		return model
	}
	if model := strings.TrimSpace(t.Model); model != "" {
		return model
	}
	if childProvider != nil {
		return strings.TrimSpace(childProvider.ModelID())
	}
	return ""
}

func agentProviderIdentity(p provider.Provider) string {
	if p == nil {
		return ""
	}
	return provider.CanonicalProviderName(p.Name())
}

func snapshotAgentProvider(p provider.Provider) provider.Provider {
	if ref, ok := p.(*provider.ProviderRef); ok {
		return ref.Get()
	}
	return p
}

type agentPermissionSnapshotHandler struct {
	mu                    sync.RWMutex
	parent                permission.PermissionHandler
	snapshot              types.ToolRuntimeContext
	presentationSessionID string
	approvalRouting       agentcontract.ApprovalRouting
	profile               agentProfile
}

// CheckHookGrantedPermissions prevents profile hooks from granting beyond the
// inherited policy. Tool-local allows still follow the parent's normal path.
func (*agentPermissionSnapshotHandler) CheckHookGrantedPermissions() bool { return true }

func (h *agentPermissionSnapshotHandler) Check(ctx context.Context, req permission.PermissionRequest) (permission.PermissionDecision, error) {
	h.mu.RLock()
	presentationSessionID := h.presentationSessionID
	approvalRouting := h.approvalRouting
	h.mu.RUnlock()
	if matchingAgentPermissionRule(h.profile.DisallowedToolRules, req.ToolName, req.Input) != nil {
		return permission.PermissionDeny, nil
	}
	mode := canonicalAgentMode(h.snapshot.PermissionMode)
	if mode == "" {
		mode = permissionModeDefault
	}
	req.Mode = mode
	req.AvoidPrompts = approvalRouting == agentcontract.ApprovalFailClosed
	if presentationSessionID != "" {
		if req.ExecutionSessionID == "" {
			req.ExecutionSessionID = req.SessionID
		}
		req.SessionID = presentationSessionID
	}
	snapshot := cloneToolRuntimeContext(h.snapshot)
	req.PermissionSnapshot = &snapshot
	return h.parent.Check(ctx, req)
}

func (h *agentPermissionSnapshotHandler) setApprovalRouting(routing agentcontract.ApprovalRouting, presentationSessionID string) {
	if h == nil {
		return
	}
	routing = normalizeAgentApprovalRouting(routing, agentcontract.ApprovalAttached)
	routing, presentationSessionID = safeAgentApprovalPresentation(routing, presentationSessionID)
	h.mu.Lock()
	h.approvalRouting = routing
	h.presentationSessionID = presentationSessionID
	h.mu.Unlock()
}

func agentPermissionHandlerForSnapshot(snapshot types.ToolRuntimeContext, parent permission.PermissionHandler, routing agentcontract.ApprovalRouting, profile agentProfile, presentationSessionIDs ...string) permission.PermissionHandler {
	if parent == nil {
		return nil
	}
	snapshot = cloneToolRuntimeContext(snapshot)
	mode := canonicalAgentMode(snapshot.PermissionMode)
	if mode == "" {
		mode = permissionModeDefault
	}
	snapshot.PermissionMode = mode
	routing = normalizeAgentApprovalRouting(routing, agentcontract.ApprovalAttached)
	presentationSessionID := ""
	if len(presentationSessionIDs) > 0 {
		presentationSessionID = strings.TrimSpace(presentationSessionIDs[0])
	}
	routing, presentationSessionID = safeAgentApprovalPresentation(routing, presentationSessionID)
	return &agentPermissionSnapshotHandler{
		parent:                parent,
		snapshot:              snapshot,
		presentationSessionID: presentationSessionID,
		approvalRouting:       routing,
		profile:               profile,
	}
}

func safeAgentApprovalPresentation(routing agentcontract.ApprovalRouting, presentationSessionID string) (agentcontract.ApprovalRouting, string) {
	presentationSessionID = strings.TrimSpace(presentationSessionID)
	if routing == agentcontract.ApprovalFailClosed {
		return routing, ""
	}
	if presentationSessionID == "" {
		return agentcontract.ApprovalFailClosed, ""
	}
	return routing, presentationSessionID
}

func normalizeAgentApprovalRouting(routing, emptyDefault agentcontract.ApprovalRouting) agentcontract.ApprovalRouting {
	switch routing {
	case agentcontract.ApprovalAttached, agentcontract.ApprovalFailClosed, agentcontract.ApprovalParentSession:
		return routing
	case "":
		return emptyDefault
	default:
		return agentcontract.ApprovalFailClosed
	}
}

func matchingAgentPermissionRule(rules []agentPermissionRule, toolName string, input map[string]any) *agentPermissionRule {
	normalized := normalizedToolNameFromPermissionSpec(toolName)
	for i := range rules {
		rule := &rules[i]
		if rule.ToolName != normalized {
			continue
		}
		if !rule.HasRuleContent {
			return rule
		}
		if agentPermissionRuleContentMatches(normalized, rule.RuleContent, input) {
			return rule
		}
	}
	return nil
}

func agentPermissionRuleContentMatches(toolName, content string, input map[string]any) bool {
	switch toolName {
	case "bash", "powershell":
		if command := stringInputValue(input, "command"); command != "" {
			return shellPermissionRuleMatches(content, command)
		}
	case "agent":
		for _, key := range []string{"subagent_type", "agent_type", "type", "name"} {
			if value := stringInputValue(input, key); agentPermissionValueMatches(content, value) {
				return true
			}
		}
	default:
		for _, key := range []string{"file_path", "path", "notebook_path", "source", "destination", "target", "link_path", "url"} {
			if value := stringInputValue(input, key); agentPermissionValueMatches(content, value) {
				return true
			}
		}
		for _, value := range input {
			if text, ok := value.(string); ok && agentPermissionValueMatches(content, text) {
				return true
			}
		}
	}
	return false
}

func stringInputValue(input map[string]any, key string) string {
	if input == nil {
		return ""
	}
	value, ok := input[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func agentPermissionValueMatches(ruleContent, value string) bool {
	ruleContent = strings.TrimSpace(ruleContent)
	value = strings.TrimSpace(value)
	if ruleContent == "" || value == "" {
		return false
	}
	if wildcardPatternHasUnescapedStar(ruleContent) {
		return matchAgentWildcardPattern(ruleContent, value)
	}
	return ruleContent == value
}

func shellPermissionRuleMatches(ruleContent, command string) bool {
	ruleContent = strings.TrimSpace(ruleContent)
	command = strings.TrimSpace(command)
	if ruleContent == "" || command == "" {
		return false
	}
	if prefix, ok := shellPermissionRulePrefix(ruleContent); ok {
		return command == prefix ||
			strings.HasPrefix(command, prefix+" ") ||
			command == "xargs "+prefix ||
			strings.HasPrefix(command, "xargs "+prefix+" ")
	}
	if wildcardPatternHasUnescapedStar(ruleContent) {
		return matchAgentWildcardPattern(ruleContent, command)
	}
	return command == ruleContent
}

func shellPermissionRulePrefix(ruleContent string) (string, bool) {
	if !strings.HasSuffix(ruleContent, ":*") {
		return "", false
	}
	prefix := strings.TrimSpace(ruleContent[:len(ruleContent)-2])
	return prefix, prefix != ""
}

func wildcardPatternHasUnescapedStar(pattern string) bool {
	if strings.HasSuffix(pattern, ":*") {
		return false
	}
	for i := 0; i < len(pattern); i++ {
		if pattern[i] == '*' && !isEscapedAt(pattern, i) {
			return true
		}
	}
	return false
}

func matchAgentWildcardPattern(pattern, value string) bool {
	pattern = strings.TrimSpace(pattern)
	value = strings.TrimSpace(value)
	if pattern == "" {
		return false
	}

	if strings.HasSuffix(pattern, " *") && countUnescapedByte(pattern, '*') == 1 {
		prefix := strings.TrimSpace(strings.TrimSuffix(pattern, " *"))
		return value == prefix || strings.HasPrefix(value, prefix+" ")
	}

	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		if ch == '\\' && i+1 < len(pattern) {
			next := pattern[i+1]
			if next == '*' || next == '\\' {
				b.WriteString(regexp.QuoteMeta(string(next)))
				i++
				continue
			}
		}
		if ch == '*' && !isEscapedAt(pattern, i) {
			b.WriteString(".*")
			continue
		}
		b.WriteString(regexp.QuoteMeta(string(ch)))
	}
	b.WriteString("$")
	re, err := regexp.Compile("(?s)" + b.String())
	if err != nil {
		return false
	}
	return re.MatchString(value)
}

func countUnescapedByte(value string, target byte) int {
	count := 0
	for i := 0; i < len(value); i++ {
		if value[i] == target && !isEscapedAt(value, i) {
			count++
		}
	}
	return count
}

func (t *AgentTool) resolveProfileForInput(in agentcontract.Input) (agentProfile, error) {
	profileCWD := strings.TrimSpace(in.CWD)
	if profileCWD == "" {
		profileCWD, _ = os.Getwd()
	}
	if !isTruthyAgentEnv(os.Getenv("LUBAN_CODE_SIMPLE")) {
		customLookup := strings.TrimSpace(in.SubagentType)
		if customLookup == "" {
			customLookup = "general-purpose"
		}
		if profile, ok, err := loadManagedAgentProfile(customLookup, profileCWD); err != nil {
			return agentProfile{}, err
		} else if ok {
			return profile, nil
		}
	}
	if t != nil && len(t.InlineProfiles) > 0 {
		key := strings.ToLower(strings.TrimSpace(in.SubagentType))
		if key == "" {
			key = "general-purpose"
		}
		if profile, ok := t.InlineProfiles[key]; ok {
			return profile, nil
		}
	}
	return resolveAgentProfileWithOptions(in.SubagentType, profileCWD, agentProfileResolveOptions{
		DisableBuiltInAgents: t != nil && t.disableBuiltInAgentProfiles(),
	})
}

func (t *AgentTool) applyAgentProfileDefaults(agentID string, in agentcontract.Input, profile agentProfile, authority skillauthority.Authority) (agentcontract.Input, error) {
	if profile.Background {
		in.RunInBackground = true
	}
	prefixes := make([]string, 0, 1+len(profile.Skills))
	if strings.TrimSpace(profile.InitialPrompt) != "" {
		prefixes = append(prefixes, strings.TrimSpace(profile.InitialPrompt))
	}
	if t.SkillManager != nil {
		for _, name := range profile.Skills {
			skillName := strings.TrimSpace(name)
			if skillName == "" {
				continue
			}
			// 3-strategy resolution mirrors TS resolveSkillName: direct,
			// plugin-prefix, then suffix match. Without it, plugin-namespaced
			// or renamed skills silently fail to preload.
			skillDef, ok, err := resolveProfileSkill(t.SkillManager, authority, skillName)
			if err != nil {
				return in, err
			}
			if !ok || skillDef == nil {
				continue
			}
			content := strings.TrimSpace(skills.PrepareSkillContent(skillDef, nil, agentID))
			if content == "" {
				continue
			}
			prefixes = append(prefixes, fmt.Sprintf("<skill name=%q>\n%s\n</skill>", skillDef.Name, content))
		}
	}
	if len(prefixes) > 0 {
		prefixes = append(prefixes, in.Prompt)
		in.Prompt = strings.Join(prefixes, "\n\n")
	}
	return in, nil
}

func agentBackgroundTasksDisabled() bool {
	return isTruthyAgentEnv(os.Getenv("LUBAN_CODE_DISABLE_BACKGROUND_TASKS"))
}

func hookRunnerForProfile(parent *hooks.Runner, profile agentProfile) *hooks.Runner {
	if profile.HookRunner == nil {
		return parent
	}
	return parent.Merge(profile.HookRunner)
}

func (t *AgentTool) validateAgentInvocation(in agentcontract.Input) error {
	if strings.TrimSpace(in.CWD) != "" && strings.EqualFold(strings.TrimSpace(in.Isolation), "worktree") {
		return i18n.NewError(i18n.KeyToolAgentDeepCWDWorktreeConflict)
	}
	switch strings.ToLower(strings.TrimSpace(in.Isolation)) {
	case "", "worktree":
	default:
		return i18n.NewError(i18n.KeyToolAgentDeepIsolationUnsupported, in.Isolation)
	}
	return nil
}

func canonicalAgentMode(raw string) string {
	trimmed := strings.TrimSpace(raw)
	switch trimmed {
	case "":
		return ""
	case "default":
		return "default"
	case "acceptEdits":
		return "acceptEdits"
	case "bypassPermissions":
		return "bypassPermissions"
	case "dontAsk":
		return "dontAsk"
	case "plan":
		return "plan"
	case "bubble":
		return "bubble"
	default:
		return trimmed
	}
}

func builtInGeneralPurposePrompt() string {
	return strings.TrimSpace(`You are an agent for LUBAN Code, an agentic coding CLI. Given the user's message, you should use the tools available to complete the task. Complete the task fully; don't gold-plate, but don't leave it half-done. When you complete the task, respond with a concise report covering what was done and any key findings. The caller will relay this to the user, so it only needs the essentials.

Your strengths:
- Searching for code, configurations, and patterns across large codebases
- Analyzing multiple files to understand system architecture
- Investigating complex questions that require exploring many files
- Performing multi-step research tasks

Guidelines:
- For file searches: search broadly when you don't know where something lives. Use Read when you know the specific file path.
- For analysis: start broad and narrow down. Use multiple search strategies if the first doesn't yield results.
- Be thorough: check multiple locations, consider different naming conventions, and look for related files.
- NEVER create files unless they're absolutely necessary for achieving your goal. ALWAYS prefer editing an existing file to creating a new one.
- NEVER proactively create documentation files (*.md) or README files. Only create documentation files if explicitly requested.`)
}

func builtInExplorePrompt() string {
	shellGuidance, shellDenyGuidance := builtInReadOnlyShellGuidance()
	return strings.TrimSpace(`You are a file search specialist for LUBAN Code. You excel at thoroughly navigating and exploring codebases.

=== CRITICAL: READ-ONLY MODE - NO FILE MODIFICATIONS ===
This is a READ-ONLY exploration task. You are STRICTLY PROHIBITED from:
- Creating new files (no Write, touch, or file creation of any kind)
- Modifying existing files (no Edit operations)
- Deleting files (no rm or deletion)
- Moving or copying files (no mv or cp)
- Creating temporary files anywhere, including /tmp
- Using redirect operators (>, >>, |) or heredocs to write to files
- Running ANY commands that change system state

Your role is EXCLUSIVELY to search and analyze existing code. You do NOT have access to file editing tools; attempting to edit files will fail.

Your strengths:
- Rapidly finding files using glob patterns
- Searching code and text with powerful regex patterns
- Reading and analyzing file contents

Guidelines:
- Use Glob for broad file pattern matching
- Use Grep for searching file contents with regex
- Use Read when you know the specific file path you need to read
- ` + shellGuidance + `
- ` + shellDenyGuidance + `
- Adapt your search approach based on the thoroughness level specified by the caller
- Communicate your final report directly as a regular message; do NOT attempt to create files

NOTE: You are meant to be a fast agent that returns output as quickly as possible. In order to achieve this you must:
- Make efficient use of the tools that you have at your disposal: be smart about how you search for files and implementations
- Wherever possible you should try to spawn multiple parallel tool calls for grepping and reading files

Complete the user's search request efficiently and report your findings clearly.`)
}

func builtInPlanPrompt() string {
	shellGuidance, shellDenyGuidance := builtInReadOnlyShellGuidance()
	return strings.TrimSpace(`You are a software architect and planning specialist for LUBAN Code. Your role is to explore the codebase and design implementation plans.

=== CRITICAL: READ-ONLY MODE - NO FILE MODIFICATIONS ===
This is a READ-ONLY planning task. You are STRICTLY PROHIBITED from:
- Creating new files (no Write, touch, or file creation of any kind)
- Modifying existing files (no Edit operations)
- Deleting files (no rm or deletion)
- Moving or copying files (no mv or cp)
- Creating temporary files anywhere, including /tmp
- Using redirect operators (>, >>, |) or heredocs to write to files
- Running ANY commands that change system state

Your role is EXCLUSIVELY to explore the codebase and design implementation plans. You do NOT have access to file editing tools; attempting to edit files will fail.

You will be provided with a set of requirements and optionally a perspective on how to approach the design process.

## Your Process

1. Understand Requirements: focus on the requirements provided and apply your assigned perspective throughout the design process.

2. Explore Thoroughly:
   - Read any files provided to you in the initial prompt
   - Find existing patterns and conventions using Glob, Grep, and Read
   - Understand the current architecture
   - Identify similar features as reference
   - Trace through relevant code paths
   - ` + shellGuidance + `
   - ` + shellDenyGuidance + `

3. Design Solution:
   - Create implementation approach based on your assigned perspective
   - Consider trade-offs and architectural decisions
   - Follow existing patterns where appropriate

4. Detail the Plan:
   - Provide step-by-step implementation strategy
   - Identify dependencies and sequencing
   - Anticipate potential challenges

## Required Output

End your response with:

	### Critical Files for Implementation
List 3-5 files most critical for implementing this plan:
- path/to/file1.ts
- path/to/file2.ts
- path/to/file3.ts

	REMEMBER: You can ONLY explore and plan. You CANNOT and MUST NOT write, edit, or modify any files. You do NOT have access to file editing tools.`)
}

func builtInReadOnlyShellGuidance() (string, string) {
	if runtime.GOOS == "windows" {
		return "Use PowerShell ONLY for read-only operations (Get-ChildItem, Select-String, Get-Content, git status, git log, git diff); prefer Glob, Grep, and Read for code search",
			"NEVER use PowerShell for: New-Item, Set-Content, Add-Content, Remove-Item, Move-Item, Copy-Item, git add, git commit, npm install, pip install, redirects that write files, or any file creation/modification"
	}
	return "Use Bash ONLY for read-only operations (ls, git status, git log, git diff, find, cat, head, tail)",
		"NEVER use Bash for: mkdir, touch, rm, cp, mv, git add, git commit, npm install, pip install, or any file creation/modification"
}

func builtInVerificationPrompt() string {
	fence := "```"
	return strings.TrimSpace(`You are a verification specialist. Your job is not to confirm the implementation works; it's to try to break it.

You have two documented failure patterns. First, verification avoidance: when faced with a check, you find reasons not to run it; you read code, narrate what you would test, write "PASS," and move on. Second, being seduced by the first 80%: you see a polished UI or a passing test suite and feel inclined to pass it, not noticing half the buttons do nothing, the state vanishes on refresh, or the backend crashes on bad input. The first 80% is the easy part. Your entire value is in finding the last 20%. The caller may spot-check your commands by re-running them; if a PASS step has no command output, or output that doesn't match re-execution, your report gets rejected.

=== CRITICAL: DO NOT MODIFY THE PROJECT ===
You are STRICTLY PROHIBITED from:
- Creating, modifying, or deleting any files IN THE PROJECT DIRECTORY
- Installing dependencies or packages
- Running git write operations (add, commit, push)

You MAY write ephemeral test scripts to a temp directory (/tmp or $TMPDIR) via Bash redirection when inline commands aren't sufficient, such as a multi-step race harness or a Playwright test. Clean up after yourself.

Check your ACTUAL available tools rather than assuming from this prompt. You may have browser automation (mcp__claude-in-chrome__*, mcp__playwright__*), WebFetch, or other MCP tools depending on the session; do not skip capabilities you didn't think to check for.

=== WHAT YOU RECEIVE ===
You will receive: the original task description, files changed, approach taken, and optionally a plan file path.

=== VERIFICATION STRATEGY ===
Adapt your strategy based on what was changed:

Frontend changes: Start dev server, check your tools for browser automation (mcp__claude-in-chrome__*, mcp__playwright__*) and USE them to navigate, screenshot, click, and read console; do NOT say "needs a real browser" without attempting; curl a sample of page subresources; run frontend tests.
Backend/API changes: Start server, curl/fetch endpoints, verify response shapes against expected values, test error handling, and check edge cases.
CLI/script changes: Run with representative inputs, verify stdout/stderr/exit codes, test edge inputs (empty, malformed, boundary), and verify --help / usage output is accurate.
Infrastructure/config changes: Validate syntax, dry-run where possible, and check env vars / secrets are actually referenced, not just defined.
Library/package changes: Build, run the full test suite, import the library from a fresh context, exercise the public API as a consumer would, and verify exported types match examples.
Bug fixes: Reproduce the original bug, verify the fix, run regression tests, and check related functionality for side effects.
Mobile (iOS/Android): Clean build, install on simulator/emulator, inspect the UI tree, tap by tree coords, re-dump to verify, test persistence after relaunch, and check crash logs.
Data/ML pipeline: Run with sample input, verify output shape/schema/types, test empty input, single row, NaN/null handling, and check for silent data loss.
Database migrations: Run migration up, verify schema matches intent, run migration down, and test against existing data.
Refactoring (no behavior change): Existing test suite MUST pass unchanged, diff the public API surface, and spot-check observable behavior is identical.
Other change types: The pattern is always the same: figure out how to exercise the change directly, check outputs against expectations, and try to break it with inputs/conditions the implementer didn't test.

=== REQUIRED STEPS (universal baseline) ===
1. Read the project's AGENTS.md / LUBAN.md / README for build/test commands and conventions. Check package.json / Makefile / pyproject.toml for script names. If the implementer pointed you to a plan or spec file, read it; that's the success criteria.
2. Run the build (if applicable). A broken build is an automatic FAIL.
3. Run the project's test suite (if it has one). Failing tests are an automatic FAIL.
4. Run linters/type-checkers if configured.
5. Check for regressions in related code.

Test suite results are context, not evidence. Run the suite, note pass/fail, then move on to your real verification. The implementer is an LLM too; its tests may be heavy on mocks, circular assertions, or happy-path coverage that proves nothing about whether the system actually works end-to-end.

=== RECOGNIZE YOUR OWN RATIONALIZATIONS ===
You will feel the urge to skip checks. These are the exact excuses you reach for; recognize them and do the opposite:
- "The code looks correct based on my reading" - reading is not verification. Run it.
- "The implementer's tests already pass" - the implementer is an LLM. Verify independently.
- "This is probably fine" - probably is not verified. Run it.
- "Let me start the server and check the code" - no. Start the server and hit the endpoint.
- "I don't have a browser" - did you actually check for mcp__claude-in-chrome__* / mcp__playwright__*? If present, use them. If an MCP tool fails, troubleshoot.
- "This would take too long" - not your call.
If you catch yourself writing an explanation instead of a command, stop. Run the command.

=== ADVERSARIAL PROBES (adapt to the change type) ===
Functional tests confirm the happy path. Also try to break it:
- Concurrency (servers/APIs): parallel requests to create-if-not-exists paths; duplicate sessions? lost writes?
- Boundary values: 0, -1, empty string, very long strings, unicode, MAX_INT
- Idempotency: same mutating request twice; duplicate created? error? correct no-op?
- Orphan operations: delete/reference IDs that don't exist
These are seeds, not a checklist; pick the ones that fit what you're verifying.

=== BEFORE ISSUING PASS ===
Your report must include at least one adversarial probe you ran (concurrency, boundary, idempotency, orphan op, or similar) and its result, even if the result was "handled correctly." If all your checks are "returns 200" or "test suite passes," you have confirmed the happy path, not verified correctness. Go back and try to break something.

=== BEFORE ISSUING FAIL ===
You found something that looks broken. Before reporting FAIL, check you haven't missed why it's actually fine:
- Already handled: is there defensive code elsewhere that prevents this?
- Intentional: do AGENTS.md, LUBAN.md, comments, or the commit message explain this as deliberate?
- Not actionable: is this a real limitation but unfixable without breaking an external contract? If so, note it as an observation, not a FAIL.
Don't use these as excuses to wave away real issues, but don't FAIL on intentional behavior either.

=== OUTPUT FORMAT (REQUIRED) ===
Every check MUST follow this structure. A check without a Command run block is not a PASS; it's a skip.

` + fence + `
### Check: [what you're verifying]
**Command run:**
  [exact command you executed]
**Output observed:**
  [actual terminal output - copy-paste, not paraphrased. Truncate if very long but keep the relevant part.]
**Result: PASS** (or FAIL - with Expected vs Actual)
` + fence + `

End with exactly this line (parsed by caller):

VERDICT: PASS
or
VERDICT: FAIL
or
VERDICT: PARTIAL

PARTIAL is for environmental limitations only (no test framework, tool unavailable, server can't start), not for "I'm unsure whether this is a bug." If you can run the check, you must decide PASS or FAIL.

Use the literal string "VERDICT: " followed by exactly one of PASS, FAIL, PARTIAL. No markdown bold, no punctuation, no variation.
- FAIL: include what failed, exact error output, reproduction steps.
- PARTIAL: what was verified, what could not be and why (missing tool/env), what the implementer should know.`)
}

func builtInStatuslineSetupPrompt() string {
	return strings.TrimSpace(`You are a status line setup agent for LUBAN Code. Your job is to create or update the statusLine command in the user's LUBAN Code settings.

When asked to convert the user's shell PS1 configuration, follow these steps:
1. Read the user's shell configuration files in this order of preference:
   - ~/.zshrc
   - ~/.bashrc
   - ~/.bash_profile
   - ~/.profile

2. Extract the PS1 value using this regex pattern: /(?:^|\n)\s*(?:export\s+)?PS1\s*=\s*["']([^"']+)["']/m

3. Convert PS1 escape sequences to shell commands:
   - \u maps to $(whoami)
   - \h maps to $(hostname -s)
   - \H maps to $(hostname)
   - \w maps to $(pwd)
   - \W maps to $(basename "$(pwd)")
   - \$ maps to $
   - \n maps to \n
   - \t maps to $(date +%H:%M:%S)
   - \d maps to $(date "+%a %b %d")
   - \@ maps to $(date +%I:%M%p)
   - \# maps to #
   - \! maps to !

4. When using ANSI color codes, be sure to use printf. Do not remove colors. The status line will be printed in a terminal using dimmed colors.

5. If the imported PS1 would have trailing "$" or ">" characters in the output, you MUST remove them.

6. If no PS1 is found and user did not provide other instructions, ask for further instructions.

How to use the statusLine command:
1. The statusLine command will receive the following JSON input via stdin:
   {
     "session_id": "string",
     "session_name": "string",
     "transcript_path": "string",
     "cwd": "string",
     "model": {
       "id": "string",
       "display_name": "string"
     },
     "workspace": {
       "current_dir": "string",
       "project_dir": "string",
       "added_dirs": ["string"]
     },
     "version": "string",
     "output_style": {
       "name": "string"
     },
     "context_window": {
       "total_input_tokens": number,
       "total_output_tokens": number,
       "context_window_size": number,
       "current_usage": {
         "input_tokens": number,
         "output_tokens": number,
         "cache_creation_input_tokens": number,
         "cache_read_input_tokens": number
       } | null,
       "used_percentage": number | null,
       "remaining_percentage": number | null
     },
     "rate_limits": {
       "five_hour": {
         "used_percentage": number,
         "resets_at": number
       },
       "seven_day": {
         "used_percentage": number,
         "resets_at": number
       }
     },
     "vim": {
       "mode": "INSERT" | "NORMAL"
     },
     "agent": {
       "name": "string",
       "type": "string"
     },
     "worktree": {
       "name": "string",
       "path": "string",
       "branch": "string",
       "original_cwd": "string",
       "original_branch": "string"
     }
   }

   You can use this JSON data in your command like:
   - $(cat | jq -r '.model.display_name')
   - $(cat | jq -r '.workspace.current_dir')
   - $(cat | jq -r '.output_style.name')

   Or store it in a variable first:
   - input=$(cat); echo "$(echo "$input" | jq -r '.model.display_name') in $(echo "$input" | jq -r '.workspace.current_dir')"

   To display context remaining percentage:
   - input=$(cat); remaining=$(echo "$input" | jq -r '.context_window.remaining_percentage // empty'); [ -n "$remaining" ] && echo "Context: $remaining% remaining"

   To display context used percentage:
   - input=$(cat); used=$(echo "$input" | jq -r '.context_window.used_percentage // empty'); [ -n "$used" ] && echo "Context: $used% used"

   To display the active provider's rate-limit usage:
   - input=$(cat); pct=$(echo "$input" | jq -r '.rate_limits.five_hour.used_percentage // empty'); [ -n "$pct" ] && printf "5h: %.0f%%" "$pct"

   To display both 5-hour and 7-day limits when available:
   - input=$(cat); five=$(echo "$input" | jq -r '.rate_limits.five_hour.used_percentage // empty'); week=$(echo "$input" | jq -r '.rate_limits.seven_day.used_percentage // empty'); out=""; [ -n "$five" ] && out="5h:$(printf '%.0f' "$five")%"; [ -n "$week" ] && out="$out 7d:$(printf '%.0f' "$week")%"; echo "$out"

2. For longer commands, you can save a new file in the user's ~/.luban-code directory, for example ~/.luban-code/statusline-command.sh, and reference that file in the settings.

3. Update the user's ~/.luban-code/settings.json with:
   {
     "statusLine": {
       "type": "command",
       "command": "your_command_here"
     }
   }

4. If ~/.luban-code/settings.json is a symlink, update the target file instead.

Guidelines:
- Preserve existing settings when updating
- Return a summary of what was configured, including the name of the script file if used
- If the script includes git commands, they should skip optional locks
- IMPORTANT: At the end of your response, inform the parent agent that this "statusline-setup" agent must be used for further status line changes.
- Also ensure that the user is informed that they can ask LUBAN Code to continue to make changes to the status line.`)
}

func builtinAgentProfiles() []agentProfile {
	readOnlySpecs := []string{"Agent", "Edit", "Write", "NotebookEdit", "ExitPlanMode", "EnterPlanMode"}
	readOnlyTools := lowerToolNameSet(readOnlySpecs...)
	statuslineSpecs := []string{"Read", "Edit"}
	profiles := []agentProfile{
		{
			Name:            "general-purpose",
			WhenToUse:       "General-purpose agent for researching complex questions, searching for code, and executing multi-step tasks. When you are searching for a keyword or file and are not confident that you will find the right match in the first few tries use this agent to perform the search for you.",
			SystemPrefix:    builtInGeneralPurposePrompt(),
			DisallowedTools: map[string]struct{}{},
		},
		{
			Name:                  "statusline-setup",
			WhenToUse:             "Use this agent to configure the user's LUBAN Code status line setting.",
			SystemPrefix:          builtInStatuslineSetupPrompt(),
			AllowedTools:          lowerToolNameSet(statuslineSpecs...),
			AllowedToolRules:      toolPermissionRulesFromNames(statuslineSpecs...),
			AllowedToolSpecs:      append([]string(nil), statuslineSpecs...),
			AllowedToolsSpecified: true,
			Model:                 "inherit",
			DisallowedTools:       map[string]struct{}{},
			Color:                 "orange",
		},
	}
	if areBuiltinExplorePlanAgentsEnabled() {
		profiles = append(profiles,
			agentProfile{
				Name:                "Explore",
				WhenToUse:           `Fast agent specialized for exploring codebases. Use this when you need to quickly find files by patterns (eg. "src/components/**/*.tsx"), search code for keywords (eg. "API endpoints"), or answer questions about the codebase (eg. "how do API endpoints work?"). When calling this agent, specify the desired thoroughness level: "quick" for basic searches, "medium" for moderate exploration, or "very thorough" for comprehensive analysis across multiple locations and naming conventions.`,
				SystemPrefix:        builtInExplorePrompt(),
				DisallowedTools:     readOnlyTools,
				DisallowedToolSpecs: append([]string(nil), readOnlySpecs...),
				Model:               builtinExploreModel(),
				OmitBaseSystem:      true,
				OmitInstructions:    true,
			},
			agentProfile{
				Name:                "Plan",
				WhenToUse:           "Software architect agent for designing implementation plans. Use this when you need to plan the implementation strategy for a task. Returns step-by-step plans, identifies critical files, and considers architectural trade-offs.",
				SystemPrefix:        builtInPlanPrompt(),
				DisallowedTools:     readOnlyTools,
				DisallowedToolSpecs: append([]string(nil), readOnlySpecs...),
				Model:               "inherit",
				OmitBaseSystem:      true,
				OmitInstructions:    true,
			},
		)
	}
	if isVerificationAgentEnabled() {
		profiles = append(profiles,
			agentProfile{
				Name:                "verification",
				WhenToUse:           "Use this agent to verify that implementation work is correct before reporting completion. Invoke after non-trivial tasks (3+ file edits, backend/API changes, infrastructure changes). Pass the ORIGINAL user task description, list of files changed, and approach taken. The agent runs builds, tests, linters, and checks to produce a PASS/FAIL/PARTIAL verdict with evidence.",
				SystemPrefix:        builtInVerificationPrompt(),
				DisallowedTools:     lowerToolNameSet("Agent", "ExitPlanMode", "Edit", "Write", "NotebookEdit"),
				DisallowedToolSpecs: []string{"Agent", "ExitPlanMode", "Edit", "Write", "NotebookEdit"},
				Background:          true,
				Model:               "inherit",
				Color:               "red",
			},
		)
	}
	return profiles
}

func builtinExploreModel() string {
	return "inherit"
}

func builtinAgentProfileForKey(key string) (agentProfile, bool) {
	profiles := builtinAgentProfiles()
	switch key {
	case "general-purpose", "general", "default":
		return findBuiltinAgentProfile(profiles, "general-purpose")
	case "explore", "explorer":
		return findBuiltinAgentProfile(profiles, "Explore")
	case "plan", "planner":
		return findBuiltinAgentProfile(profiles, "Plan")
	case "verification", "verifier":
		return findBuiltinAgentProfile(profiles, "verification")
	case "statusline-setup":
		return findBuiltinAgentProfile(profiles, "statusline-setup")
	default:
		return agentProfile{}, false
	}
}

func findBuiltinAgentProfile(profiles []agentProfile, names ...string) (agentProfile, bool) {
	for _, profile := range profiles {
		for _, name := range names {
			if strings.EqualFold(profile.Name, name) {
				return profile, true
			}
		}
	}
	return agentProfile{}, false
}

func builtinAgentNames() []string {
	profiles := builtinAgentProfiles()
	names := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		names = append(names, profile.Name)
	}
	return names
}

type agentProfileResolveOptions struct {
	DisableBuiltInAgents bool
}

func resolveAgentProfileWithOptions(raw string, cwd string, opts agentProfileResolveOptions) (agentProfile, error) {
	key := strings.ToLower(strings.TrimSpace(raw))
	if key == "" {
		key = "general-purpose"
	}
	customLookup := strings.TrimSpace(raw)
	if customLookup == "" {
		customLookup = key
	}
	if !isTruthyAgentEnv(os.Getenv("LUBAN_CODE_SIMPLE")) {
		if profile, ok, err := loadCustomAgentProfile(customLookup, cwd); err != nil {
			return agentProfile{}, err
		} else if ok {
			return profile, nil
		}
		if profile, ok, err := loadPluginAgentProfile(customLookup, cwd); err != nil {
			return agentProfile{}, err
		} else if ok {
			return profile, nil
		}
	}
	switch key {
	case forkSubagentType:
		return forkAgentProfile(), nil
	default:
		if !opts.DisableBuiltInAgents {
			if profile, ok := builtinAgentProfileForKey(key); ok {
				return profile, nil
			}
		}
		available := "none"
		if !opts.DisableBuiltInAgents {
			available = strings.Join(builtinAgentNames(), ", ")
		}
		if available == "" {
			available = "none"
		}
		return agentProfile{}, i18n.NewError(i18n.KeyToolAgentDeepUnknownSubagentType, raw, available)
	}
}

func bindInProcessAgentScopedTools(reg *registry.Registry, agentID string, projectRoots ...string) {
	if reg == nil || strings.TrimSpace(agentID) == "" {
		return
	}
	projectRoot := ""
	if len(projectRoots) > 0 {
		projectRoot = projectRoots[0]
	}
	for _, tool := range reg.All() {
		if scoped, ok := tool.(agentcontract.ScopedTool); ok {
			reg.Register(scoped.BindAgentScope(agentID, projectRoot))
		}
	}
}

func registryForAgentProfileWithOptions(source *registry.Registry, profile agentProfile, opts agentToolRegistryOptions) *registry.Registry {
	if source == nil {
		return registry.New()
	}
	filtered := source.NewDerived()
	for _, tool := range source.All() {
		if !agentToolAllowedByBaseFilters(tool.Name(), opts.IsAsync, opts.AllowAgent) {
			continue
		}
		if !agentProfileAllowsTool(profile, tool.Name()) {
			continue
		}
		filtered.Register(tool)
	}
	if len(profile.MCPServers) > 0 || len(profile.MCPServerConfigs) > 0 {
		ensureProfileExtraTool(source, filtered, profile, "ListMcpResourcesTool")
		ensureProfileExtraTool(source, filtered, profile, "ReadMcpResourceTool")
	}
	if search, ok := filtered.Get("ToolSearch").(interface {
		WithRegistry(*registry.Registry) types.Tool
	}); ok {
		filtered.Register(search.WithRegistry(filtered))
	}
	return filtered
}

func agentToolAllowedByBaseFilters(name string, isAsync, allowAgent bool) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "" {
		return false
	}
	if isMCPToolName(lower) {
		return true
	}
	if lower == "agent" && allowAgent && !isAsync {
		return true
	}
	if _, blocked := allAgentDisallowedTools()[lower]; blocked {
		return false
	}
	if isAsync {
		if _, allowed := asyncAgentAllowedTools()[lower]; !allowed {
			return false
		}
	}
	return true
}

func isMCPToolName(lowerName string) bool {
	return strings.HasPrefix(lowerName, "mcp__") ||
		lowerName == "listmcpresourcestool" ||
		lowerName == "readmcpresourcetool"
}

func allAgentDisallowedTools() map[string]struct{} {
	names := []string{
		"TaskOutput",
		"ExitPlanMode",
		"EnterPlanMode",
		"AskUserQuestion",
		"TaskStop",
		"Workflow",
		"EnterWorktree",
		"ExitWorktree",
	}
	if os.Getenv("USER_TYPE") != "ant" {
		names = append(names, "Agent")
	}
	return lowerToolNameSet(names...)
}

func removePermissionTransitionToolsFromAgentRegistry(reg *registry.Registry) {
	if reg == nil {
		return
	}
	disallowed := lowerToolNameSet("EnterPlanMode", "ExitPlanMode", "AskUserQuestion", "EnterWorktree", "ExitWorktree")
	for _, name := range reg.Names() {
		if _, blocked := disallowed[strings.ToLower(strings.TrimSpace(name))]; blocked {
			reg.Unregister(name)
		}
	}
}

func asyncAgentAllowedTools() map[string]struct{} {
	return lowerToolNameSet(
		"Read",
		"WebSearch",
		"Grep",
		"WebFetch",
		"Glob",
		"Bash",
		"Run",
		"PowerShell",
		"Edit",
		"Write",
		"NotebookEdit",
		"Skill",
		"ToolSearch",
		"EnterWorktree",
		"ExitWorktree",
	)
}

func lowerToolNameSet(names ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(names))
	for _, name := range names {
		if trimmed := strings.ToLower(strings.TrimSpace(name)); trimmed != "" {
			out[trimmed] = struct{}{}
		}
	}
	return out
}

func toolPermissionRulesFromNames(names ...string) []agentPermissionRule {
	if len(names) == 0 {
		return nil
	}
	return toolPermissionRulesFromYAML(names)
}

func allowedAgentTypesFromRules(rules []agentPermissionRule) []string {
	if len(rules) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	for _, rule := range rules {
		if rule.ToolName != "agent" || !rule.HasRuleContent {
			continue
		}
		for _, agentType := range splitAgentPermissionTypes(rule.RuleContent) {
			key := strings.ToLower(agentType)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, agentType)
		}
	}
	return out
}

func splitAgentPermissionTypes(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func ensureProfileExtraTool(source, filtered *registry.Registry, profile agentProfile, name string) {
	if source == nil || filtered == nil {
		return
	}
	if !agentProfileAllowsTool(profile, name) {
		return
	}
	if filtered.Get(name) != nil {
		return
	}
	if tool := source.Get(name); tool != nil {
		filtered.Register(tool)
	}
}

func prepareAgentMCPRegistry(source, filtered *registry.Registry, profile agentProfile) (*registry.Registry, func(), error) {
	if len(profile.MCPServerConfigs) == 0 || source == nil || filtered == nil {
		return source, nil, nil
	}
	manager := mcpmanager.NewManager()
	for name, cfg := range profile.MCPServerConfigs {
		if strings.TrimSpace(name) != "" {
			manager.AddConfig(name, cloneAgentMCPServerConfig(cfg))
		}
	}
	privateSource := registry.New()
	ctx, cancel := context.WithTimeout(context.Background(), defaultAgentMCPReadinessTimeout)
	err := toolmcp.RefreshDynamicMCPTools(ctx, privateSource, manager, nil)
	cancel()
	if err != nil {
		_ = manager.Shutdown(context.Background())
		return nil, nil, err
	}
	privateSource.Register(toolmcp.NewListMcpResourcesTool(manager))
	privateSource.Register(toolmcp.NewReadMcpResourceTool(manager))
	for _, tool := range privateSource.All() {
		if agentProfileAllowsTool(profile, tool.Name()) {
			filtered.Register(tool)
		}
	}
	cleanup := func() {
		_ = manager.Shutdown(context.Background())
	}
	return privateSource, cleanup, nil
}

func agentMCPServersWithoutOverrides(servers []string, overrides map[string]catalog.MCPServerConfig) []string {
	out := make([]string, 0, len(servers))
	for _, server := range servers {
		trimmed := strings.TrimSpace(server)
		if trimmed == "" {
			continue
		}
		overridden := false
		for name := range overrides {
			if strings.EqualFold(strings.TrimSpace(name), trimmed) {
				overridden = true
				break
			}
		}
		if !overridden {
			out = append(out, trimmed)
		}
	}
	return out
}

func cloneAgentMCPServerConfig(cfg catalog.MCPServerConfig) catalog.MCPServerConfig {
	clone := cfg
	clone.Args = append([]string(nil), cfg.Args...)
	clone.Env = make(map[string]string, len(cfg.Env))
	for key, value := range cfg.Env {
		clone.Env[key] = value
	}
	clone.Headers = make(map[string]string, len(cfg.Headers))
	for key, value := range cfg.Headers {
		clone.Headers[key] = value
	}
	if cfg.OAuth != nil {
		oauth := *cfg.OAuth
		clone.OAuth = &oauth
	}
	if cfg.IDERunningInWindows != nil {
		value := *cfg.IDERunningInWindows
		clone.IDERunningInWindows = &value
	}
	return clone
}

func validateAgentMCPRequirements(source *registry.Registry, profile agentProfile) error {
	if len(profile.RequiredMCPServers) == 0 || source == nil {
		return nil
	}
	available := availableAgentMCPServers(source)
	var missing []string
	for _, required := range profile.RequiredMCPServers {
		pattern := strings.ToLower(strings.TrimSpace(required))
		if pattern == "" {
			continue
		}
		found := false
		for server := range available {
			if strings.Contains(server, pattern) {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, required)
		}
	}
	if len(missing) > 0 {
		return i18n.NewError(i18n.KeyToolAgentDeepMCPServersRequired, profile.Name, strings.Join(missing, ", "))
	}
	return nil
}

func availableAgentMCPServers(source *registry.Registry) map[string]struct{} {
	available := make(map[string]struct{})
	if source == nil {
		return available
	}
	for _, tool := range source.All() {
		parts := strings.Split(tool.Name(), "__")
		if len(parts) >= 3 && strings.EqualFold(parts[0], "mcp") {
			available[strings.ToLower(parts[1])] = struct{}{}
		}
	}
	return available
}

func agentMCPRequirementsSatisfied(profile agentProfile, available map[string]struct{}) bool {
	for _, required := range profile.RequiredMCPServers {
		pattern := strings.ToLower(strings.TrimSpace(required))
		if pattern == "" {
			continue
		}
		found := false
		for server := range available {
			if strings.Contains(server, pattern) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func registerAgentMCPDynamicTools(source, filtered *registry.Registry, profile agentProfile) {
	if filtered == nil || source == nil {
		return
	}
	if len(profile.MCPServers) == 0 && len(profile.MCPServerConfigs) == 0 {
		return
	}
	seenServers := map[string]struct{}{}
	for _, server := range profile.MCPServers {
		server = strings.TrimSpace(server)
		if server == "" {
			continue
		}
		seenServers[server] = struct{}{}
	}
	for server := range profile.MCPServerConfigs {
		server = strings.TrimSpace(server)
		if server == "" {
			continue
		}
		seenServers[server] = struct{}{}
	}
	for _, tool := range source.All() {
		dynamic, ok := tool.(*toolmcp.DynamicMCPTool)
		if !ok {
			continue
		}
		if _, allowedServer := seenServers[dynamic.MCPServerName()]; !allowedServer {
			continue
		}
		if agentProfileAllowsTool(profile, dynamic.Name()) {
			filtered.Register(dynamic)
		}
	}
}

func buildAgentSystemPrompt(base string, profile agentProfile, mode string, cwd string) string {
	parts := make([]string, 0, 4)
	if strings.TrimSpace(profile.SystemPrefix) != "" {
		parts = append(parts, strings.TrimSpace(profile.SystemPrefix))
	}
	if memoryPrompt := loadAgentMemoryPrompt(profile.Name, profile.Memory, cwd); memoryPrompt != "" {
		parts = append(parts, memoryPrompt)
	}
	if strings.TrimSpace(mode) != "" {
		parts = append(parts, "Permission mode: "+mode+".")
	}
	if len(profile.Skills) > 0 {
		parts = append(parts, "Configured skills: "+strings.Join(profile.Skills, ", ")+". Use the Skill tool to load these skills before applying their instructions.")
	}
	if len(profile.MCPServers) > 0 {
		parts = append(parts, "Configured MCP servers for this agent: "+strings.Join(profile.MCPServers, ", ")+". Prefer their resources and tools when available in the registry.")
	}
	if strings.TrimSpace(base) != "" && !profile.OmitBaseSystem {
		baseText := base
		if profile.OmitInstructions {
			baseText = stripInstructionsSection(baseText)
		}
		if strings.TrimSpace(baseText) != "" {
			parts = append(parts, strings.TrimSpace(baseText))
		}
	}
	return strings.Join(parts, "\n\n")
}

func agentProfileAllowsTool(profile agentProfile, toolName string) bool {
	normalized := normalizedToolNameFromPermissionSpec(toolName)
	if normalized == "" {
		normalized = strings.ToLower(strings.TrimSpace(toolName))
	}
	literal := strings.ToLower(strings.TrimSpace(toolName))
	if profile.AllowedToolsSpecified {
		if _, allowed := profile.AllowedTools[normalized]; !allowed {
			if _, allowed := profile.AllowedTools[literal]; !allowed {
				return false
			}
		}
	}
	if _, blocked := profile.DisallowedTools[normalized]; blocked {
		return false
	}
	if _, blocked := profile.DisallowedTools[literal]; blocked {
		return false
	}
	return true
}

func loadCustomAgentProfile(agentType string, cwd string) (agentProfile, bool, error) {
	profiles, err := loadCustomAgentProfiles(cwd)
	if err != nil {
		return agentProfile{}, false, err
	}
	return findAgentProfileByName(profiles, agentType)
}

func loadManagedAgentProfile(agentType string, cwd string) (agentProfile, bool, error) {
	_, managedDirs := agentSearchDirGroups(cwd)
	profiles, err := loadCustomAgentProfilesFromDirs(cwd, managedDirs)
	if err != nil {
		return agentProfile{}, false, err
	}
	return findAgentProfileByName(profiles, agentType)
}

func findAgentProfileByName(profiles []agentProfile, agentType string) (agentProfile, bool, error) {
	for _, profile := range profiles {
		if strings.EqualFold(profile.Name, agentType) {
			return profile, true, nil
		}
	}
	return agentProfile{}, false, nil
}

func loadCustomAgentProfiles(cwd string) ([]agentProfile, error) {
	return loadCustomAgentProfilesFromDirs(cwd, agentSearchDirs(cwd))
}

func loadCustomAgentProfilesFromDirs(cwd string, dirs []string) ([]agentProfile, error) {
	indexByName := map[string]int{}
	seenFiles := map[string]struct{}{}
	var profiles []agentProfile
	for _, dir := range dirs {
		paths, err := customAgentMarkdownFiles(dir)
		if err != nil {
			return nil, err
		}
		for _, path := range paths {
			fileKey := agentPathIdentity(path)
			if _, exists := seenFiles[fileKey]; exists {
				continue
			}
			seenFiles[fileKey] = struct{}{}
			profile, ok, err := parseCustomAgentProfileFile(path, cwd)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			key := strings.ToLower(strings.TrimSpace(profile.Name))
			if key == "" {
				continue
			}
			if existing, exists := indexByName[key]; exists {
				profiles[existing] = profile
				continue
			}
			indexByName[key] = len(profiles)
			profiles = append(profiles, profile)
		}
	}
	return profiles, nil
}

func agentSearchDirs(cwd string) []string {
	dirs, managedDirs := agentSearchDirGroups(cwd)
	dirs = append(dirs, managedDirs...)
	return dirs
}

func agentSearchDirGroups(cwd string) ([]string, []string) {
	var projectDirs []string
	start := strings.TrimSpace(cwd)
	if start == "" {
		start, _ = os.Getwd()
	}
	if abs, err := filepath.Abs(start); err == nil {
		start = abs
	}
	if info, err := os.Stat(start); err == nil && !info.IsDir() {
		start = filepath.Dir(start)
	}
	stop := ""
	if out, err := gitutil.Run(start, "rev-parse", "--show-toplevel"); err == nil {
		stop = strings.TrimSpace(out)
	}
	canonicalRoot, hasCanonicalRoot := canonicalAgentGitRoot(start)
	current := filepath.Clean(start)
	for {
		projectDirs = appendExistingAgentDir(projectDirs, filepath.Join(current, brand.ConfigDirName, "agents"))
		if stop != "" && strings.EqualFold(filepath.Clean(current), filepath.Clean(stop)) {
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	if hasCanonicalRoot && stop != "" && !sameAgentPath(stop, canonicalRoot) {
		worktreeAgents := filepath.Join(stop, brand.ConfigDirName, "agents")
		if !agentDirExists(worktreeAgents) {
			projectDirs = appendExistingAgentDir(projectDirs, filepath.Join(canonicalRoot, brand.ConfigDirName, "agents"))
		}
	}
	var dirs []string
	if home, err := userHomeDirForAgentProfiles(); err == nil && strings.TrimSpace(home) != "" {
		dirs = appendExistingAgentDir(dirs, filepath.Join(home, brand.ConfigDirName, "agents"))
	}
	if configHome := agentConfigHomeDir(); configHome != "" {
		userDir := filepath.Join(configHome, "agents")
		dirs = appendExistingAgentDir(dirs, userDir)
	}
	dirs = append(dirs, projectDirs...)
	var managedDirs []string
	for _, managedDir := range managedAgentDirs() {
		managedDirs = appendExistingAgentDir(managedDirs, managedDir)
	}
	return dirs, managedDirs
}

func canonicalAgentGitRoot(start string) (string, bool) {
	commonDir, err := gitutil.Run(start, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", false
	}
	commonDir = strings.TrimSpace(commonDir)
	if commonDir == "" {
		return "", false
	}
	if filepath.Base(commonDir) != ".git" {
		return filepath.Clean(commonDir), true
	}
	return filepath.Dir(filepath.Clean(commonDir)), true
}

func appendExistingAgentDir(dirs []string, candidate string) []string {
	if !agentDirExists(candidate) {
		return dirs
	}
	for _, existing := range dirs {
		if sameAgentPath(existing, candidate) {
			return dirs
		}
	}
	return append(dirs, candidate)
}

func agentDirExists(dir string) bool {
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}

func sameAgentPath(left, right string) bool {
	return agentPathIdentity(left) == agentPathIdentity(right)
}

func managedAgentDirs() []string {
	base := ""
	if os.Getenv("USER_TYPE") == "ant" {
		base = strings.TrimSpace(os.Getenv("LUBAN_CODE_MANAGED_SETTINGS_PATH"))
	}
	if base == "" {
		switch runtime.GOOS {
		case "darwin":
			base = "/Library/Application Support/LUBAN Code"
		case "windows":
			base = `C:\Program Files\LUBAN Code`
		default:
			base = "/etc/luban-code"
		}
	}
	if strings.TrimSpace(base) == "" {
		return nil
	}
	return []string{filepath.Join(filepath.Clean(base), brand.ConfigDirName, "agents")}
}

func userHomeDirForAgentProfiles() (string, error) {
	if home := os.Getenv("HOME"); strings.TrimSpace(home) != "" {
		return home, nil
	}
	return os.UserHomeDir()
}

func customAgentMarkdownFiles(dir string) ([]string, error) {
	var paths []string
	visitedDirs := map[string]struct{}{}
	seenFiles := map[string]struct{}{}
	var walk func(string) error
	walk = func(current string) error {
		info, err := os.Stat(current)
		if err != nil {
			if os.IsNotExist(err) || os.IsPermission(err) {
				return nil
			}
			return err
		}
		if !info.IsDir() {
			return nil
		}
		dirKey := agentPathIdentity(current)
		if _, exists := visitedDirs[dirKey]; exists {
			return nil
		}
		visitedDirs[dirKey] = struct{}{}
		entries, err := os.ReadDir(current)
		if err != nil {
			if os.IsNotExist(err) || os.IsPermission(err) {
				return nil
			}
			return err
		}
		for _, entry := range entries {
			path := filepath.Join(current, entry.Name())
			info, err := os.Stat(path)
			if err != nil {
				if os.IsNotExist(err) || os.IsPermission(err) {
					continue
				}
				return err
			}
			if info.IsDir() {
				if err := walk(path); err != nil {
					return err
				}
				continue
			}
			if strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
				fileKey := agentPathIdentity(path)
				if _, exists := seenFiles[fileKey]; exists {
					continue
				}
				seenFiles[fileKey] = struct{}{}
				paths = append(paths, path)
			}
		}
		return nil
	}
	if err := walk(dir); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

func agentPathIdentity(value string) string {
	pathValue := value
	if abs, err := filepath.Abs(pathValue); err == nil {
		pathValue = abs
	}
	if resolved, err := filepath.EvalSymlinks(pathValue); err == nil {
		pathValue = resolved
	}
	pathValue = filepath.Clean(pathValue)
	if runtime.GOOS == "windows" {
		pathValue = strings.ToLower(pathValue)
	}
	return pathValue
}

type customAgentFrontmatterSchema struct {
	Name               any `yaml:"name"`
	Description        any `yaml:"description"`
	Tools              any `yaml:"tools"`
	DisallowedTools    any `yaml:"disallowedTools"`
	Model              any `yaml:"model"`
	Effort             any `yaml:"effort"`
	MaxTurns           any `yaml:"maxTurns"`
	Skills             any `yaml:"skills"`
	OmitInstructions   any `yaml:"omitInstructions"`
	OmitBaseSystem     any `yaml:"omitBaseSystem"`
	MCPServers         any `yaml:"mcpServers"`
	RequiredMCPServers any `yaml:"requiredMcpServers"`
	Hooks              any `yaml:"hooks"`
	InitialPrompt      any `yaml:"initialPrompt"`
	Background         any `yaml:"background"`
	Memory             any `yaml:"memory"`
	Color              any `yaml:"color"`
	Isolation          any `yaml:"isolation"`
}

func validateAgentFrontmatter[T any](text string) error {
	var schema T
	decoder := yaml.NewDecoder(strings.NewReader(text))
	decoder.KnownFields(true)
	return decoder.Decode(&schema)
}

func parseCustomAgentProfileFile(path string, cwd string) (agentProfile, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return agentProfile{}, false, err
	}
	text := string(data)
	if !strings.HasPrefix(text, "---") {
		return agentProfile{}, false, nil
	}
	rest := strings.TrimPrefix(text, "---")
	parts := strings.SplitN(rest, "---", 2)
	if len(parts) != 2 {
		return agentProfile{}, false, nil
	}
	if err := validateAgentFrontmatter[customAgentFrontmatterSchema](parts[0]); err != nil {
		return agentProfile{}, false, i18n.WrapError(i18n.KeyToolAgentDeepFrontmatterParseFailed, err, path)
	}
	var frontmatter map[string]any
	if err := yaml.Unmarshal([]byte(parts[0]), &frontmatter); err != nil {
		return agentProfile{}, false, i18n.WrapError(i18n.KeyToolAgentDeepFrontmatterParseFailed, err, path)
	}
	name, _ := frontmatter["name"].(string)
	description, _ := frontmatter["description"].(string)
	if strings.TrimSpace(name) == "" || strings.TrimSpace(description) == "" {
		return agentProfile{}, false, nil
	}
	prompt := strings.TrimSpace(parts[1])
	if prompt == "" {
		return agentProfile{}, false, i18n.NewError(i18n.KeyToolAgentDeepCustomPromptEmpty, name)
	}
	allowedToolsValue, allowedToolsPresent := yamlValueWithPresence(frontmatter, "tools")
	disallowedToolsValue := yamlValue(frontmatter, "disallowedTools")
	allowedTools, allowedRules, allowedSpecs, allowedSpecified := allowedToolProfileFieldsFromYAML(allowedToolsValue, allowedToolsPresent)
	disallowedTools, disallowedRules, disallowedSpecs := disallowedToolProfileFieldsFromYAML(disallowedToolsValue)
	profile := agentProfile{
		Name:                  strings.TrimSpace(name),
		WhenToUse:             normalizeAgentWhenToUse(description),
		SystemPrefix:          prompt,
		AllowedTools:          allowedTools,
		DisallowedTools:       disallowedTools,
		AllowedToolRules:      allowedRules,
		DisallowedToolRules:   disallowedRules,
		AllowedToolSpecs:      allowedSpecs,
		DisallowedToolSpecs:   disallowedSpecs,
		AllowedToolsSpecified: allowedSpecified,
	}
	if model, ok := stringFromYAML(frontmatter, "model"); ok {
		if normalized, valid := agentModelFromString(model); valid {
			profile.Model = normalized
		}
	}
	if effort := reasoningEffortFromValue(yamlValue(frontmatter, "effort")); effort != "" {
		profile.ReasoningEffort = effort
	}
	if maxTurns, ok := positiveIntFromYAML(yamlValue(frontmatter, "maxTurns")); ok {
		profile.MaxTurns = maxTurns
	}
	if skills := stringsFromYAML(yamlValue(frontmatter, "skills")); len(skills) > 0 {
		profile.Skills = skills
	}
	if omitInstructions, ok := boolFromYAML(yamlValue(frontmatter, "omitInstructions")); ok {
		profile.OmitInstructions = omitInstructions
	}
	if omitBase, ok := boolFromYAML(yamlValue(frontmatter, "omitBaseSystem")); ok {
		profile.OmitBaseSystem = omitBase
	}
	if servers, configs := agentMCPServersFromMarkdownYAML(yamlValue(frontmatter, "mcpServers")); len(servers) > 0 || len(configs) > 0 {
		profile.MCPServers = servers
		// Only admin-trusted agents (built-in/managed/plugin) may register
		// new MCP server *configs* — user-authored agents can still
		// reference servers by name in their tool allow-list, but cannot
		// add transports/credentials. This mirrors TS
		// isRestrictedToPluginOnly('mcp').
		if isAgentSourceAdminTrusted(classifyAgentSource(path)) {
			profile.MCPServerConfigs = configs
		}
	}
	if required := stringsFromYAML(yamlValue(frontmatter, "requiredMcpServers")); len(required) > 0 {
		profile.RequiredMCPServers = required
	}
	if hookValue := yamlValue(frontmatter, "hooks"); hookValue != nil {
		// Admin-trust gate: only built-in, plugin-managed, or enterprise-
		// managed agents may register process-level hooks. Project- or
		// user-authored agents have their `hooks` block silently ignored
		// to prevent privilege escalation via .luban-code/agents/*.md.
		if isAgentSourceAdminTrusted(classifyAgentSource(path)) {
			if hookRunner, err := hookRunnerFromYAML(hookValue, path); err == nil {
				profile.HookRunner = hookRunner.WithHookTypeMapped(hooks.HookStop, hooks.HookSubagentStop)
			}
		}
	}
	if initialPrompt, ok := stringFromYAML(frontmatter, "initialPrompt"); ok {
		profile.InitialPrompt = initialPrompt
	}
	if agentMarkdownBackgroundFromYAML(yamlValue(frontmatter, "background")) {
		profile.Background = true
	}
	if memory, ok := stringFromYAML(frontmatter, "memory"); ok {
		if isValidAgentMemoryScope(memory) {
			profile.Memory = memory
		}
	}
	if color, ok := stringFromYAML(frontmatter, "color"); ok {
		if isValidAgentColor(color) {
			profile.Color = color
		}
	}
	if isolation, ok := stringFromYAML(frontmatter, "isolation"); ok && strings.TrimSpace(isolation) != "" {
		if isolation == "worktree" {
			profile.Isolation = isolation
		}
	}
	applyAgentMemoryToolAccess(&profile, cwd)
	return profile, true, nil
}

func yamlValue(frontmatter map[string]any, keys ...string) any {
	value, _ := yamlValueWithPresence(frontmatter, keys...)
	return value
}

func yamlValueWithPresence(frontmatter map[string]any, keys ...string) (any, bool) {
	if len(frontmatter) == 0 {
		return nil, false
	}
	for _, key := range keys {
		if value, ok := frontmatter[key]; ok {
			return value, true
		}
	}
	return nil, false
}

func stringFromYAML(frontmatter map[string]any, keys ...string) (string, bool) {
	value := yamlValue(frontmatter, keys...)
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return "", false
	}
	return strings.TrimSpace(text), true
}

func normalizeAgentWhenToUse(description string) string {
	return strings.ReplaceAll(strings.TrimSpace(description), `\n`, "\n")
}

func boolFromYAML(value any) (bool, bool) {
	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true":
			return true, true
		case "false":
			return false, true
		default:
			return false, false
		}
	default:
		return false, false
	}
}

func agentMarkdownBackgroundFromYAML(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return v == "true"
	default:
		return false
	}
}

func isValidAgentMemoryScope(value string) bool {
	switch value {
	case "user", "project", "local":
		return true
	default:
		return false
	}
}

func isValidAgentColor(value string) bool {
	_, ok := agentColorNames[value]
	return ok
}

func normalizeReasoningEffort(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "low", "medium", "high", "max":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		if value, ok := parseLeadingIntegerString(raw); ok {
			return strconv.Itoa(value)
		}
		return ""
	}
}

func reasoningEffortFromValue(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return normalizeReasoningEffort(v)
	case int:
		return strconv.Itoa(v)
	case int8:
		return strconv.FormatInt(int64(v), 10)
	case int16:
		return strconv.FormatInt(int64(v), 10)
	case int32:
		return strconv.FormatInt(int64(v), 10)
	case int64:
		return strconv.FormatInt(v, 10)
	case uint:
		return strconv.FormatUint(uint64(v), 10)
	case uint8:
		return strconv.FormatUint(uint64(v), 10)
	case uint16:
		return strconv.FormatUint(uint64(v), 10)
	case uint32:
		return strconv.FormatUint(uint64(v), 10)
	case uint64:
		return strconv.FormatUint(v, 10)
	case float32:
		return integerFloatReasoningEffort(float64(v))
	case float64:
		return integerFloatReasoningEffort(v)
	default:
		return ""
	}
}

func integerFloatReasoningEffort(value float64) string {
	if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value {
		return ""
	}
	return strconv.FormatFloat(value, 'f', 0, 64)
}

func parseLeadingIntegerString(raw string) (int, bool) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return 0, false
	}
	i := 0
	if text[i] == '+' || text[i] == '-' {
		i++
	}
	start := i
	for i < len(text) && text[i] >= '0' && text[i] <= '9' {
		i++
	}
	if i == start {
		return 0, false
	}
	value, err := strconv.Atoi(text[:i])
	if err != nil {
		return 0, false
	}
	return value, true
}

func toolNameSetFromYAML(value any) map[string]struct{} {
	names := stringsFromYAML(value)
	if len(names) == 0 {
		return nil
	}
	if containsWildcardToolName(names) {
		return nil
	}
	out := make(map[string]struct{}, len(names))
	for _, name := range names {
		if trimmed := normalizedToolNameFromPermissionSpec(name); trimmed != "" {
			out[trimmed] = struct{}{}
		}
	}
	return out
}

func toolPermissionRulesFromYAML(value any) []agentPermissionRule {
	names := stringsFromYAML(value)
	if len(names) == 0 {
		return nil
	}
	if containsWildcardToolName(names) {
		return nil
	}
	rules := make([]agentPermissionRule, 0, len(names))
	for _, name := range names {
		rule, ok := parseAgentPermissionRule(name)
		if ok {
			rules = append(rules, rule)
		}
	}
	return rules
}

func allowedToolProfileFieldsFromYAML(value any, present bool) (map[string]struct{}, []agentPermissionRule, []string, bool) {
	if !present {
		return nil, nil, nil, false
	}
	names := stringsFromYAML(value)
	if containsWildcardToolName(names) {
		return nil, nil, nil, false
	}
	return toolNameSetFromYAML(names), toolPermissionRulesFromYAML(names), append([]string(nil), names...), true
}

func disallowedToolProfileFieldsFromYAML(value any) (map[string]struct{}, []agentPermissionRule, []string) {
	names := stringsFromYAML(value)
	if len(names) == 0 || containsWildcardToolName(names) {
		return nil, nil, nil
	}
	return toolNameSetFromYAML(names), toolPermissionRulesFromYAML(names), append([]string(nil), names...)
}

func applyAgentMemoryToolAccess(profile *agentProfile, cwd string) {
	if profile == nil || strings.TrimSpace(profile.Memory) == "" || !profile.AllowedToolsSpecified || !isAgentAutoMemoryEnabled(cwd) {
		return
	}
	for _, toolName := range []string{"Read", "Edit", "Write"} {
		addAgentProfileAllowedTool(profile, toolName)
	}
}

func addAgentProfileAllowedTool(profile *agentProfile, toolName string) {
	normalized := strings.ToLower(strings.TrimSpace(toolName))
	if normalized == "" {
		return
	}
	if profile.AllowedTools == nil {
		profile.AllowedTools = map[string]struct{}{}
	}
	profile.AllowedToolsSpecified = true
	if _, ok := profile.AllowedTools[normalized]; ok {
		if !agentToolSpecsContain(profile.AllowedToolSpecs, toolName) {
			profile.AllowedToolSpecs = append(profile.AllowedToolSpecs, toolName)
		}
		return
	}
	profile.AllowedTools[normalized] = struct{}{}
	if !agentToolSpecsContain(profile.AllowedToolSpecs, toolName) {
		profile.AllowedToolSpecs = append(profile.AllowedToolSpecs, toolName)
	}
	if rule, ok := parseAgentPermissionRule(toolName); ok {
		profile.AllowedToolRules = append(profile.AllowedToolRules, rule)
	}
}

func agentToolSpecsContain(specs []string, toolName string) bool {
	normalized := normalizedToolNameFromPermissionSpec(toolName)
	for _, spec := range specs {
		if normalizedToolNameFromPermissionSpec(spec) == normalized {
			return true
		}
	}
	return false
}

func containsWildcardToolName(names []string) bool {
	for _, name := range names {
		if strings.TrimSpace(name) == "*" {
			return true
		}
	}
	return false
}

func normalizedToolNameFromPermissionSpec(spec string) string {
	toolName := parsePermissionToolName(strings.TrimSpace(spec))
	return strings.ToLower(strings.TrimSpace(toolName))
}

func parseAgentPermissionRule(spec string) (agentPermissionRule, bool) {
	raw := strings.TrimSpace(spec)
	if raw == "" || raw == "*" {
		return agentPermissionRule{}, false
	}
	toolName := parsePermissionToolName(raw)
	rule := agentPermissionRule{
		ToolName: normalizedToolNameFromPermissionSpec(raw),
		Raw:      raw,
	}
	open := findFirstUnescapedRune(raw, '(')
	close := findLastUnescapedRune(raw, ')')
	if open != -1 && close > open && close == len(raw)-1 && open > 0 && parsePermissionToolName(raw) == toolName {
		rawContent := raw[open+1 : close]
		if rawContent != "" && rawContent != "*" {
			rule.RuleContent = unescapePermissionRuleContent(rawContent)
			rule.HasRuleContent = true
		}
	}
	if rule.ToolName == "" {
		return agentPermissionRule{}, false
	}
	return rule, true
}

func parsePermissionToolName(rule string) string {
	if rule == "" {
		return ""
	}
	open := findFirstUnescapedRune(rule, '(')
	if open == -1 {
		return rule
	}
	close := findLastUnescapedRune(rule, ')')
	if close == -1 || close <= open || close != len(rule)-1 || open == 0 {
		return rule
	}
	rawContent := rule[open+1 : close]
	if rawContent == "" || rawContent == "*" {
		return rule[:open]
	}
	return rule[:open]
}

func unescapePermissionRuleContent(content string) string {
	content = strings.ReplaceAll(content, `\(`, "(")
	content = strings.ReplaceAll(content, `\)`, ")")
	content = strings.ReplaceAll(content, `\\`, `\`)
	return content
}

func findFirstUnescapedRune(s string, target rune) int {
	for i, r := range s {
		if r == target && !isEscapedAt(s, i) {
			return i
		}
	}
	return -1
}

func findLastUnescapedRune(s string, target rune) int {
	for i := len(s) - 1; i >= 0; i-- {
		if rune(s[i]) == target && !isEscapedAt(s, i) {
			return i
		}
	}
	return -1
}

func isEscapedAt(s string, index int) bool {
	count := 0
	for i := index - 1; i >= 0 && s[i] == '\\'; i-- {
		count++
	}
	return count%2 == 1
}

func stringsFromYAML(value any) []string {
	switch v := value.(type) {
	case nil:
		return nil
	case string:
		var out []string
		for _, part := range splitToolListYAMLString(v) {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out
	case []string:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	default:
		return nil
	}
}

func splitToolListYAMLString(value string) []string {
	var out []string
	start := 0
	depth := 0
	var quote rune
	escaped := false
	for index, r := range value {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		switch r {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',', ' ':
			if depth == 0 {
				out = append(out, value[start:index])
				start = index + len(string(r))
			}
		}
	}
	out = append(out, value[start:])
	return out
}

func agentMCPServersFromYAML(value any) ([]string, map[string]catalog.MCPServerConfig, error) {
	configs := make(map[string]catalog.MCPServerConfig)
	var names []string
	addName := func(name string) {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			names = append(names, trimmed)
		}
	}
	addConfig := func(name string, raw any) error {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			return nil
		}
		cfg, err := mcpServerConfigFromYAML(raw)
		if err != nil {
			return i18n.WrapError(i18n.KeyToolAgentDeepMCPServerNamedError, err, trimmed)
		}
		configs[trimmed] = cfg
		addName(trimmed)
		return nil
	}

	switch v := value.(type) {
	case nil:
		return nil, nil, nil
	case string:
		for _, name := range stringsFromYAML(v) {
			addName(name)
		}
	case []string:
		for _, name := range v {
			addName(name)
		}
	case []any:
		for _, item := range v {
			switch typed := item.(type) {
			case string:
				for _, name := range stringsFromYAML(typed) {
					addName(name)
				}
			case map[string]any:
				for name, raw := range typed {
					if err := addConfig(name, raw); err != nil {
						return nil, nil, err
					}
				}
			case map[any]any:
				for key, raw := range typed {
					if err := addConfig(fmt.Sprint(key), raw); err != nil {
						return nil, nil, err
					}
				}
			default:
				return nil, nil, i18n.NewError(i18n.KeyToolAgentDeepMCPServerConfigExpected)
			}
		}
	case map[string]any:
		for name, raw := range v {
			if err := addConfig(name, raw); err != nil {
				return nil, nil, err
			}
		}
	case map[any]any:
		for key, raw := range v {
			if err := addConfig(fmt.Sprint(key), raw); err != nil {
				return nil, nil, err
			}
		}
	default:
		return nil, nil, i18n.NewError(i18n.KeyToolAgentDeepMCPServersValueExpected)
	}
	if len(configs) == 0 {
		configs = nil
	}
	return names, configs, nil
}

func agentMCPServersFromMarkdownYAML(value any) ([]string, map[string]catalog.MCPServerConfig) {
	switch v := value.(type) {
	case nil:
		return nil, nil
	case []any:
		var names []string
		configs := map[string]catalog.MCPServerConfig{}
		for _, item := range v {
			itemNames, itemConfigs, err := agentMCPServersFromYAML([]any{item})
			if err != nil {
				continue
			}
			names = append(names, itemNames...)
			for name, cfg := range itemConfigs {
				configs[name] = cfg
			}
		}
		if len(configs) == 0 {
			configs = nil
		}
		return names, configs
	case []string:
		names, configs, err := agentMCPServersFromYAML(v)
		if err != nil {
			return nil, nil
		}
		return names, configs
	default:
		return nil, nil
	}
}

func mcpServerConfigFromYAML(value any) (catalog.MCPServerConfig, error) {
	normalized := normalizeYAMLForJSON(value)
	data, err := json.Marshal(normalized)
	if err != nil {
		return catalog.MCPServerConfig{}, err
	}
	var cfg catalog.MCPServerConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return catalog.MCPServerConfig{}, err
	}
	if strings.TrimSpace(cfg.Command) == "" {
		return catalog.MCPServerConfig{}, i18n.NewError(i18n.KeyToolAgentDeepMCPCommandRequired)
	}
	return cfg, nil
}

func hookRunnerFromYAML(value any, source string) (*hooks.Runner, error) {
	data, err := json.Marshal(normalizeYAMLForJSON(map[string]any{"hooks": value}))
	if err != nil {
		return nil, err
	}
	return hooks.LoadConfigData(data, source)
}

type agentProfileJSON struct {
	Description        string    `json:"description"`
	Tools              *[]string `json:"tools"`
	DisallowedTools    []string  `json:"disallowedTools"`
	Prompt             string    `json:"prompt"`
	Model              *string   `json:"model"`
	Effort             any       `json:"effort"`
	MCPServers         any       `json:"mcpServers"`
	Hooks              any       `json:"hooks"`
	MaxTurns           *int      `json:"maxTurns"`
	Skills             []string  `json:"skills"`
	InitialPrompt      string    `json:"initialPrompt"`
	Memory             string    `json:"memory"`
	Background         *bool     `json:"background"`
	Isolation          string    `json:"isolation"`
	OmitInstructions   *bool     `json:"omitInstructions"`
	RequiredMCPServers []string  `json:"requiredMcpServers"`
}

// SetInlineProfilesFromJSON installs JSON-defined agents, matching the TS
// --agents option shape: { "agent-name": { description, prompt, ... } }.
func (t *AgentTool) SetInlineProfilesFromJSON(raw string) error {
	if strings.TrimSpace(raw) == "" {
		t.InlineProfiles = nil
		return nil
	}
	profiles, err := parseAgentProfilesJSON([]byte(raw))
	if err != nil {
		return err
	}
	t.InlineProfiles = profiles
	return nil
}

func parseAgentProfilesJSON(data []byte) (map[string]agentProfile, error) {
	var raw map[string]agentProfileJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, i18n.WrapError(i18n.KeyToolAgentDeepAgentsJSONParseFailed, err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return nil, i18n.WrapError(i18n.KeyToolAgentDeepAgentsJSONParseFailed, err)
	}
	profiles := make(map[string]agentProfile, len(raw))
	for name, def := range raw {
		profile, err := parseAgentProfileJSON(name, def)
		if err != nil {
			return nil, err
		}
		profiles[strings.ToLower(strings.TrimSpace(name))] = profile
	}
	return profiles, nil
}

func parseAgentProfileJSON(name string, def agentProfileJSON) (agentProfile, error) {
	agentName := strings.TrimSpace(name)
	if agentName == "" {
		return agentProfile{}, i18n.NewError(i18n.KeyToolAgentDeepJSONNameEmpty)
	}
	if strings.TrimSpace(def.Description) == "" {
		return agentProfile{}, i18n.NewError(i18n.KeyToolAgentDeepJSONDescriptionMissing, agentName)
	}
	if strings.TrimSpace(def.Prompt) == "" {
		return agentProfile{}, i18n.NewError(i18n.KeyToolAgentDeepJSONPromptMissing, agentName)
	}
	allowedToolsValue := any(nil)
	allowedToolsPresent := false
	if def.Tools != nil {
		allowedToolsValue = *def.Tools
		allowedToolsPresent = true
	}
	allowedTools, allowedRules, allowedSpecs, allowedSpecified := allowedToolProfileFieldsFromYAML(allowedToolsValue, allowedToolsPresent)
	disallowedTools, disallowedRules, disallowedSpecs := disallowedToolProfileFieldsFromYAML(def.DisallowedTools)
	profile := agentProfile{
		Name:                  agentName,
		WhenToUse:             normalizeAgentWhenToUse(def.Description),
		SystemPrefix:          strings.TrimSpace(def.Prompt),
		AllowedTools:          allowedTools,
		DisallowedTools:       disallowedTools,
		AllowedToolRules:      allowedRules,
		DisallowedToolRules:   disallowedRules,
		AllowedToolSpecs:      allowedSpecs,
		DisallowedToolSpecs:   disallowedSpecs,
		AllowedToolsSpecified: allowedSpecified,
		Skills:                append([]string(nil), def.Skills...),
		InitialPrompt:         def.InitialPrompt,
		RequiredMCPServers:    append([]string(nil), def.RequiredMCPServers...),
	}
	if def.OmitInstructions != nil {
		profile.OmitInstructions = *def.OmitInstructions
	}
	if def.Model != nil {
		model, valid := agentModelFromString(*def.Model)
		if !valid {
			return agentProfile{}, i18n.NewError(i18n.KeyToolAgentDeepJSONModelEmpty, agentName)
		}
		profile.Model = model
	}
	if effort := reasoningEffortFromJSON(def.Effort); effort != "" {
		profile.ReasoningEffort = effort
	}
	if def.MaxTurns != nil {
		if *def.MaxTurns <= 0 {
			return agentProfile{}, i18n.NewError(i18n.KeyToolAgentDeepJSONMaxTurnsUnsupported, agentName, *def.MaxTurns)
		}
		profile.MaxTurns = *def.MaxTurns
	}
	if def.MCPServers != nil {
		servers, configs, err := agentMCPServersFromJSON(def.MCPServers)
		if err != nil {
			return agentProfile{}, i18n.WrapError(i18n.KeyToolAgentDeepJSONMCPServersInvalid, err, agentName)
		}
		profile.MCPServers = servers
		profile.MCPServerConfigs = configs
	}
	if def.Hooks != nil {
		runner, err := hookRunnerFromYAML(def.Hooks, "--agents:"+agentName)
		if err != nil {
			return agentProfile{}, i18n.WrapError(i18n.KeyToolAgentDeepJSONHooksInvalid, err, agentName)
		}
		profile.HookRunner = runner.WithHookTypeMapped(hooks.HookStop, hooks.HookSubagentStop)
	}
	if def.Background != nil {
		profile.Background = *def.Background
	}
	if memory := strings.TrimSpace(def.Memory); memory != "" {
		if !isValidAgentMemoryScope(memory) {
			return agentProfile{}, i18n.NewError(i18n.KeyToolAgentDeepJSONMemoryUnsupported, agentName, def.Memory)
		}
		profile.Memory = memory
	}
	if isolation := strings.TrimSpace(def.Isolation); isolation != "" {
		if isolation != "worktree" {
			return agentProfile{}, i18n.NewError(i18n.KeyToolAgentDeepJSONIsolationUnsupported, agentName, isolation)
		}
		profile.Isolation = isolation
	}
	applyAgentMemoryToolAccess(&profile, "")
	return profile, nil
}

func reasoningEffortFromJSON(value any) string {
	return reasoningEffortFromValue(value)
}

func agentModelFromString(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", false
	}
	if strings.EqualFold(trimmed, "inherit") {
		return "inherit", true
	}
	return trimmed, true
}

func agentMCPServersFromJSON(value any) ([]string, map[string]catalog.MCPServerConfig, error) {
	switch value.(type) {
	case []any, []string:
		return agentMCPServersFromYAML(value)
	default:
		return nil, nil, i18n.NewError(i18n.KeyToolAgentDeepJSONArrayExpected)
	}
}

func normalizeYAMLForJSON(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = normalizeYAMLForJSON(item)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[fmt.Sprint(key)] = normalizeYAMLForJSON(item)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = normalizeYAMLForJSON(item)
		}
		return out
	default:
		return value
	}
}

func positiveIntFromYAML(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, v > 0
	case int64:
		return int(v), v > 0
	case float64:
		i := int(v)
		return i, v > 0 && float64(i) == v
	default:
		return 0, false
	}
}

func applyAgentSessionMetadata(summary agentRunSummary, metadata agentcontract.SessionMetadata) agentRunSummary {
	summary.AgentType = metadata.AgentType
	summary.Model = metadata.Model
	summary.CWD = metadata.CWD
	summary.Mode = metadata.Mode
	summary.Isolation = metadata.Isolation
	summary.WorktreePath = metadata.WorktreePath
	summary.WorktreeBranch = metadata.WorktreeBranch
	if path := strings.TrimSpace(metadata.WorktreePath); path != "" {
		summary.ArtifactRefs = appendUniqueAgentRunRef(summary.ArtifactRefs, path)
	}
	if branch := strings.TrimSpace(metadata.WorktreeBranch); branch != "" {
		summary.ArtifactRefs = appendUniqueAgentRunRef(summary.ArtifactRefs, "git-branch:"+branch)
	}
	return summary
}

func appendUniqueAgentRunRef(refs []string, ref string) []string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return refs
	}
	for _, current := range refs {
		if current == ref {
			return refs
		}
	}
	return append(refs, ref)
}

func agentWorktreeRepoRoot(parentProjectRoot string) (string, error) {
	parentProjectRoot = strings.TrimSpace(parentProjectRoot)
	if parentProjectRoot == "" {
		return "", i18n.NewError(i18n.KeyToolAgentDeepParentProjectRootEmpty)
	}
	repoRoot, err := gitutil.CanonicalRoot(parentProjectRoot)
	if err != nil {
		return "", err
	}
	if runtime.GOOS != "darwin" {
		return repoRoot, nil
	}
	for _, pathPair := range []struct {
		private string
		visible string
	}{
		{private: "/private/var/", visible: "/var/"},
		{private: "/private/tmp/", visible: "/tmp/"},
	} {
		if !strings.HasPrefix(repoRoot, pathPair.private) {
			continue
		}
		visibleRoot := pathPair.visible + strings.TrimPrefix(repoRoot, pathPair.private)
		repoInfo, repoErr := os.Stat(repoRoot)
		visibleInfo, visibleErr := os.Stat(visibleRoot)
		if repoErr == nil && visibleErr == nil && os.SameFile(repoInfo, visibleInfo) {
			return visibleRoot, nil
		}
	}
	return repoRoot, nil
}

func createAgentWorktree(agentID, parentProjectRoot string) (*agentWorktree, error) {
	repoRoot, err := agentWorktreeRepoRoot(parentProjectRoot)
	if err != nil {
		return nil, i18n.NewError(i18n.KeyToolAgentDeepWorktreeGitRepoRequired)
	}
	headCommit, err := gitutil.Run(repoRoot, "rev-parse", "HEAD")
	if err != nil || strings.TrimSpace(headCommit) == "" {
		return nil, i18n.NewError(i18n.KeyToolAgentDeepWorktreeCommitRequired)
	}
	slug := sanitizeAgentIdentifier(agentID, fmt.Sprintf("agent-%d", time.Now().UnixNano()))
	flattened, err := toolworktree.NormalizeSlug(slug)
	if err != nil {
		return nil, err
	}
	branch := "luban-agent-" + flattened
	worktreePath := filepath.Join(storepaths.RuntimeServiceDir(repoRoot, "worktrees"), "agents", flattened)
	if err := secureio.EnsurePrivateRuntimeDirectory(filepath.Dir(worktreePath)); err != nil {
		return nil, i18n.NewError(i18n.KeyToolAgentDeepWorktreeCreateFailed, err)
	}
	if out, err := gitutil.Run(repoRoot, "worktree", "add", worktreePath, "-b", branch, "HEAD"); err != nil {
		return nil, i18n.NewError(i18n.KeyToolAgentDeepWorktreeCreateFailed, out)
	}
	absPath, err := filepath.Abs(worktreePath)
	if err != nil {
		absPath = worktreePath
	}
	performAgentWorktreePostCreationSetup(repoRoot, filepath.Clean(absPath))
	return &agentWorktree{RepoRoot: repoRoot, Path: filepath.Clean(absPath), Branch: branch, HeadCommit: strings.TrimSpace(headCommit)}, nil
}

func performAgentWorktreePostCreationSetup(repoRoot, worktreePath string) {
	copyAgentWorktreeLocalSettings(repoRoot, worktreePath)
	configureAgentWorktreeHooks(repoRoot, worktreePath)
	symlinkAgentWorktreeDirectories(repoRoot, worktreePath, loadAgentWorktreeSymlinkDirectories(repoRoot))
	copyAgentWorktreeIncludeFiles(repoRoot, worktreePath)
}

func copyAgentWorktreeLocalSettings(repoRoot, worktreePath string) {
	src := filepath.Join(repoRoot, brand.ConfigDirName, "settings.local.json")
	dst := filepath.Join(worktreePath, brand.ConfigDirName, "settings.local.json")
	_ = copyAgentWorktreeFile(src, dst)
}

func copyAgentWorktreeFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if info, err := os.Stat(src); err == nil {
		mode = info.Mode().Perm()
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, mode)
}

func configureAgentWorktreeHooks(repoRoot, worktreePath string) {
	hooksPath := ""
	for _, candidate := range []string{
		filepath.Join(repoRoot, ".husky"),
		filepath.Join(repoRoot, ".git", "hooks"),
	} {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			hooksPath = candidate
			break
		}
	}
	if hooksPath == "" {
		return
	}
	_, _ = gitutil.Run(worktreePath, "config", "core.hooksPath", hooksPath)
}

func loadAgentWorktreeSymlinkDirectories(repoRoot string) []string {
	data, err := os.ReadFile(filepath.Join(repoRoot, brand.ConfigDirName, "settings.json"))
	if err != nil {
		return nil
	}
	var settings struct {
		Worktree struct {
			SymlinkDirectories []string `json:"symlinkDirectories"`
		} `json:"worktree"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil
	}
	return settings.Worktree.SymlinkDirectories
}

func symlinkAgentWorktreeDirectories(repoRoot, worktreePath string, dirs []string) {
	for _, dir := range dirs {
		rel, ok := safeRelativeWorktreePath(dir)
		if !ok {
			continue
		}
		src := filepath.Join(repoRoot, rel)
		if info, err := os.Stat(src); err != nil || !info.IsDir() {
			continue
		}
		dst := filepath.Join(worktreePath, rel)
		if _, err := os.Lstat(dst); err == nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			continue
		}
		_ = os.Symlink(src, dst)
	}
}

func safeRelativeWorktreePath(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || filepath.IsAbs(trimmed) {
		return "", false
	}
	clean := filepath.Clean(trimmed)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", false
	}
	return clean, true
}

func copyAgentWorktreeIncludeFiles(repoRoot, worktreePath string) {
	patterns := loadAgentWorktreeIncludePatterns(repoRoot)
	if len(patterns) == 0 {
		return
	}
	out, err := gitutil.Run(repoRoot, "ls-files", "--others", "--ignored", "--exclude-standard")
	if err != nil {
		return
	}
	for _, line := range strings.Split(out, "\n") {
		entry := strings.TrimSpace(line)
		if entry == "" {
			continue
		}
		entry = filepath.ToSlash(entry)
		if !agentWorktreeIncludeMatches(patterns, entry) {
			continue
		}
		src := filepath.Join(repoRoot, filepath.FromSlash(entry))
		dst := filepath.Join(worktreePath, filepath.FromSlash(entry))
		_ = copyAgentWorktreePath(src, dst)
	}
}

func loadAgentWorktreeIncludePatterns(repoRoot string) []string {
	data, err := os.ReadFile(filepath.Join(repoRoot, ".worktreeinclude"))
	if err != nil {
		return nil
	}
	patterns := []string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, strings.TrimPrefix(filepath.ToSlash(line), "/"))
	}
	return patterns
}

func agentWorktreeIncludeMatches(patterns []string, entry string) bool {
	entry = strings.TrimPrefix(filepath.ToSlash(entry), "/")
	for _, pattern := range patterns {
		if agentWorktreeIncludePatternMatches(pattern, entry) {
			return true
		}
	}
	return false
}

func agentWorktreeIncludePatternMatches(pattern, entry string) bool {
	pattern = strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(pattern)), "/")
	if pattern == "" {
		return false
	}
	if strings.HasSuffix(pattern, "/") {
		return strings.HasPrefix(entry, strings.TrimSuffix(pattern, "/")+"/")
	}
	if !strings.ContainsAny(pattern, "*?[") {
		return entry == pattern || strings.HasPrefix(entry, pattern+"/")
	}
	if ok, _ := path.Match(pattern, entry); ok {
		return true
	}
	if !strings.Contains(pattern, "/") {
		if ok, _ := path.Match(pattern, path.Base(entry)); ok {
			return true
		}
	}
	return false
}

func copyAgentWorktreePath(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return filepath.WalkDir(src, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			rel, err := filepath.Rel(src, path)
			if err != nil {
				return err
			}
			target := filepath.Join(dst, rel)
			if entry.IsDir() {
				return os.MkdirAll(target, 0o755)
			}
			return copyAgentWorktreeFile(path, target)
		})
	}
	if info.Mode().Type() != 0 {
		return nil
	}
	return copyAgentWorktreeFile(src, dst)
}

func ensureAgentWorktreePath(metadata agentcontract.SessionMetadata) error {
	path := strings.TrimSpace(metadata.WorktreePath)
	if path == "" {
		return nil
	}
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return nil
	}
	branch := strings.TrimSpace(metadata.WorktreeBranch)
	if branch == "" {
		return i18n.NewError(i18n.KeyToolAgentDeepPersistedWorktreeBranchMissing, path)
	}
	repoRoot := strings.TrimSpace(metadata.WorktreeRepoRoot)
	if repoRoot == "" {
		repoRoot = inferRepoRootFromAgentWorktree(path)
	}
	if repoRoot == "" {
		return i18n.NewError(i18n.KeyToolAgentDeepPersistedWorktreeRepoRootMissing, path)
	}
	if out, err := gitutil.Run(repoRoot, "worktree", "add", path, branch); err != nil {
		return i18n.NewError(i18n.KeyToolAgentDeepWorktreeRestoreFailed, path, branch, out)
	}
	return nil
}

func cleanupAgentWorktreeIfClean(metadata agentcontract.SessionMetadata) (bool, error) {
	path := strings.TrimSpace(metadata.WorktreePath)
	if path == "" {
		return false, nil
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		return false, nil
	}
	headCommit := strings.TrimSpace(metadata.WorktreeHeadCommit)
	if headCommit == "" {
		return false, nil
	}
	if agentWorktreeHasChanges(path, headCommit) {
		return false, nil
	}
	repoRoot := strings.TrimSpace(metadata.WorktreeRepoRoot)
	if repoRoot == "" {
		repoRoot = inferRepoRootFromAgentWorktree(path)
	}
	if repoRoot == "" {
		return false, nil
	}
	if out, err := gitutil.Run(repoRoot, "worktree", "remove", "--force", path); err != nil {
		return false, i18n.NewError(i18n.KeyToolAgentDeepWorktreeRemoveFailed, path, out)
	}
	if branch := strings.TrimSpace(metadata.WorktreeBranch); branch != "" {
		_, _ = gitutil.Run(repoRoot, "branch", "-D", branch)
	}
	return true, nil
}

func agentWorktreeHasChanges(path, headCommit string) bool {
	status, err := gitutil.Run(path, "status", "--porcelain")
	if err != nil || strings.TrimSpace(status) != "" {
		return true
	}
	countOut, err := gitutil.Run(path, "rev-list", "--count", strings.TrimSpace(headCommit)+"..HEAD")
	if err != nil {
		return true
	}
	count, err := strconv.Atoi(strings.TrimSpace(countOut))
	return err != nil || count > 0
}

func finalizeAgentWorktreeMetadata(metadata agentcontract.SessionMetadata) (agentcontract.SessionMetadata, bool) {
	removed, err := cleanupAgentWorktreeIfClean(metadata)
	if err != nil || !removed {
		return metadata, false
	}
	removedPath := filepath.Clean(metadata.WorktreePath)
	if filepath.Clean(metadata.CWD) == removedPath {
		metadata.CWD = ""
	}
	metadata.WorktreeRepoRoot = ""
	metadata.WorktreePath = ""
	metadata.WorktreeBranch = ""
	metadata.WorktreeHeadCommit = ""
	return metadata, true
}

func inferRepoRootFromAgentWorktree(path string) string {
	clean := filepath.Clean(path)
	if filepath.Base(filepath.Dir(clean)) != "worktrees" {
		return ""
	}
	configDir := filepath.Dir(filepath.Dir(clean))
	if filepath.Base(configDir) != brand.ConfigDirName {
		return ""
	}
	return filepath.Dir(configDir)
}

func (t *AgentTool) resolveSpawnTeamName(in agentcontract.Input) string {
	if teamName := strings.TrimSpace(in.TeamName); teamName != "" {
		return teamName
	}
	if t.Collaboration == nil {
		return ""
	}
	return strings.TrimSpace(t.Collaboration.CurrentTeamName())
}

func (t *AgentTool) shouldSpawnTeammate(teamName string, in agentcontract.Input) bool {
	teamName = strings.TrimSpace(teamName)
	if teamName == "" || strings.TrimSpace(in.Name) == "" {
		return false
	}
	if t.Collaboration != nil && strings.TrimSpace(t.Collaboration.CurrentTeamName()) != "" {
		return true
	}
	if strings.TrimSpace(in.TeamName) == "" {
		return false
	}
	return t.Collaboration != nil && t.Collaboration.TeamExists(teamName)
}

func (t *AgentTool) spawnTeammate(ctx context.Context, agentID, teamName string, in agentcontract.Input, parentModel string) (types.ToolResult, error) {
	if t.Background == nil {
		return ErrorResponsef("%s", toolRuntimeText(i18n.KeyToolAgentTeammateSessionsRequired)), nil
	}
	if t.Collaboration == nil {
		return ErrorResponsef("%s", toolRuntimeText(i18n.KeyToolAgentTeamManagerRequired)), nil
	}
	request := agentcontract.TeammateSpawnRequest{
		SpawnID: strings.TrimSpace(agentID), TeamName: teamName, Input: in, ParentModel: parentModel,
		ParentSession: backgroundTaskOwnerSessionID(ctx),
	}
	result, err := t.Collaboration.SpawnTeammate(ctx, request, func(ctx context.Context, identity agentcontract.TeammateIdentity) (agentcontract.TeammateLaunch, error) {
		return t.prepareTeammateLaunch(ctx, request, identity)
	})
	if err != nil {
		return ErrorResponse(i18n.WrapInternalError(i18n.KeyAuxSwarmFailed, err)), nil
	}
	return result, nil
}

func (t *AgentTool) prepareTeammateLaunch(ctx context.Context, request agentcontract.TeammateSpawnRequest, identity agentcontract.TeammateIdentity) (agentcontract.TeammateLaunch, error) {
	spawnInput := request.Input
	spawnInput.Name = identity.Name
	session, snap, err := t.createRetainedAgentSessionWithOptions(identity.AgentID, spawnInput, agentLoopOptions{
		Context:               ctx,
		OverrideModel:         request.ParentModel,
		TeamMember:            true,
		ApprovalRouting:       agentcontract.ApprovalParentSession,
		PresentationSessionID: request.ParentSession,
	})
	if err != nil {
		return agentcontract.TeammateLaunch{}, err
	}
	sessionMetadata := session.metadataSnapshot()
	payload := map[string]any{
		"status":             "teammate_spawned",
		"prompt":             request.Input.Prompt,
		"teammate_id":        identity.AgentID,
		"agent_id":           identity.AgentID,
		"agent_type":         firstNonEmpty(spawnInput.SubagentType, "general-purpose"),
		"model":              sessionMetadata.Model,
		"name":               identity.Name,
		"tmux_session_name":  "local-agent",
		"tmux_window_name":   "local-agent",
		"tmux_pane_id":       identity.AgentID,
		"team_name":          identity.Team,
		"is_splitpane":       false,
		"plan_mode_required": false,
		"output_file":        snap.OutputPath,
		"worktree_path":      sessionMetadata.WorktreePath,
		"worktree_branch":    sessionMetadata.WorktreeBranch,
	}
	content, err := json.Marshal(payload)
	if err != nil {
		_ = t.Background.rollbackRegisteredAgentSession(identity.AgentID, session)
		return agentcontract.TeammateLaunch{}, err
	}
	partial := AgentPartial{
		AgentResultBase: AgentResultBase{Kind: AgentResultKindPartial},
		AgentID:         identity.AgentID,
		AgentType:       firstNonEmpty(spawnInput.SubagentType, "general-purpose"),
		OutputFile:      snap.OutputPath,
		Description:     firstNonEmpty(request.Input.Description, request.Input.Prompt),
		Prompt:          request.Input.Prompt,
		IsAsync:         true,
		Message:         toolRuntimeText(i18n.KeyToolAgentTeammateSpawned),
		WireStatus:      "teammate_spawned",
	}
	return agentcontract.TeammateLaunch{
		Result: agentToolResult(partial, string(content), false),
		CWD:    firstNonEmpty(sessionMetadata.CWD, spawnInput.CWD), Model: request.ParentModel,
		Start:    func() error { return session.enqueue(request.Input.Prompt, nil) },
		Rollback: func() error { return t.Background.rollbackRegisteredAgentSession(identity.AgentID, session) },
	}, nil
}

func (t *AgentTool) canReadAgentOutputFile() bool {
	if t == nil || t.Registry == nil {
		return false
	}
	return t.Registry.Get("Read") != nil || t.Registry.Get("Bash") != nil
}

func (t *AgentTool) canReadAgentOutputFileForInput(in agentcontract.Input) bool {
	if strings.EqualFold(strings.TrimSpace(in.SubagentType), forkSubagentType) {
		return false
	}
	return t.canReadAgentOutputFile()
}

func shouldAppendForkWorktreeNotice(metadata agentcontract.SessionMetadata) bool {
	return metadata.AgentType == forkSubagentType && strings.TrimSpace(metadata.WorktreePath) != ""
}

func runAgentCleanup(cleanup func()) {
	if cleanup != nil {
		cleanup()
	}
}

func isOneShotBuiltInAgentType(agentType string) bool {
	switch strings.ToLower(strings.TrimSpace(agentType)) {
	case "explore", "plan":
		return true
	default:
		return false
	}
}

func shouldSuppressOneShotWorktreeIsolation(in agentcontract.Input, profile agentProfile) bool {
	if !isOneShotBuiltInAgentType(profile.Name) || !strings.EqualFold(strings.TrimSpace(firstNonEmpty(in.Isolation, profile.Isolation)), "worktree") {
		return false
	}
	return !agentPromptExplicitlyRequestsWorktree(in.Prompt)
}

func agentPromptExplicitlyRequestsWorktree(prompt string) bool {
	lower := strings.ToLower(prompt)
	if !strings.Contains(lower, "worktree") && !strings.Contains(lower, "isolated working tree") && !strings.Contains(lower, "isolated copy") {
		return false
	}
	for _, negation := range []string{
		"omit isolation",
		"without isolation",
		"no isolation",
		"do not use worktree",
		"don't use worktree",
		"without worktree",
		"no worktree",
		"not use worktree",
	} {
		if strings.Contains(lower, negation) {
			return false
		}
	}
	return true
}

func formatAsyncAgentLaunchResult(agentID, description, prompt, outputPath string, canReadOutputFile bool) string {
	result := agentAsyncToolResult{
		IsAsync:           true,
		Status:            "async_launched",
		Kind:              string(AgentResultKindPartial),
		Prompt:            prompt,
		Description:       description,
		AgentID:           agentID,
		OutputFile:        outputPath,
		CanReadOutputFile: canReadOutputFile,
		Message:           toolRuntimeFormat(i18n.KeyToolAgentContinueAsync, agentID),
	}
	data, _ := json.Marshal(result)
	return string(data)
}

func formatCompletedAgentResult(summary agentRunSummary) string {
	content := strings.TrimRight(summary.Output, "\n")
	if strings.TrimSpace(content) == "" {
		content = toolRuntimeText(i18n.KeyToolAgentEmptyOutput)
	}
	if isOneShotBuiltInAgentType(summary.AgentType) && strings.TrimSpace(summary.WorktreePath) == "" && strings.TrimSpace(summary.WorktreeBranch) == "" {
		return content
	}
	result := agentCompletedToolResult{
		Status:  "completed",
		Kind:    string(AgentResultKindCompleted),
		Prompt:  summary.Prompt,
		AgentID: summary.AgentID,
		AgentType: firstNonEmpty(
			summary.AgentType,
			"general-purpose",
		),
		Content: []agentToolContentBlock{{
			Type: "text",
			Text: content,
		}},
		TotalDurationMs:   summary.TotalDuration,
		TotalTokens:       summary.TotalTokens,
		TotalToolUseCount: summary.ToolUseCount,
		Usage:             formatAgentUsage(summary.Usage),
		CWD:               summary.CWD,
		Mode:              summary.Mode,
		Isolation:         summary.Isolation,
		Model:             summary.Model,
		WorktreePath:      summary.WorktreePath,
		WorktreeBranch:    summary.WorktreeBranch,
		TranscriptPath:    summary.TranscriptPath,
		LatestToolUse:     summary.LatestToolUse,
	}
	data, _ := json.Marshal(result)
	return string(data)
}

func formatAgentUsage(usage *types.Usage) agentToolUsage {
	if usage == nil {
		return agentToolUsage{}
	}
	return agentToolUsage{
		InputTokens:              usage.InputTokens,
		OutputTokens:             usage.OutputTokens,
		CacheCreationInputTokens: optionalNonZeroInt(usage.CacheCreationInputTokens),
		CacheReadInputTokens:     optionalNonZeroInt(usage.CacheReadInputTokens),
	}
}

func optionalNonZeroInt(value int) *int {
	if value == 0 {
		return nil
	}
	return &value
}
