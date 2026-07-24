package tools

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/agent-dance/luban/coordinator"
	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/swarm"
	"github.com/agent-dance/luban/types"
)

// TeamInfo holds metadata about a created team.
type TeamInfo struct {
	ID          string
	Name        string
	StorageName string
	Description string
	LeadAgentID string
	FilePath    string
	Agents      []string // agent IDs

	// OwnerSessionID and OwnerProjectRoot form the durable in-process owner.
	// ProjectRoot alone is insufficient because two resumed conversations may
	// legitimately share one checkout without sharing Team state.
	OwnerSessionID   string
	OwnerProjectRoot string
	coordinator      *coordinator.Coordinator
}

type teamOwnerKey struct {
	SessionID   string
	ProjectRoot string
}

// TeamManager holds shared state for team tools.
type TeamManager struct {
	mu            sync.Mutex
	mutationMu    sync.Mutex
	coordinator   *coordinator.Coordinator
	teams         map[string]*TeamInfo
	nextTeamID    int
	activeTeamID  string
	activeByOwner map[teamOwnerKey]string
	CWD           string
	SessionID     func() string
	Lifecycle     *RuntimeLifecycle

	// Provider, Registry and System are needed to create functional sub-agents.
	// They are optional: if nil, agents fall back to a stub Execute.
	Provider          provider.Provider
	Registry          *registry.Registry
	System            string // system prompt passed to sub-agent loops
	SkillManager      *skills.Manager
	HookRunner        *hooks.Runner
	PermissionHandler loop.PermissionHandler
	Runtime           types.ToolRuntimeContextProvider
	taskListChanged   func()
	taskStore         *TaskStore

	// Background gives SendMessage access to local agent sessions spawned by Agent.
	Background *BackgroundTaskManager

	sessionRuntime    TeamSessionRuntime
	runtimeSet        bool
	sessionIDResolver func() string
	barrierMu         sync.RWMutex
	sessionBarrier    *sync.RWMutex
}

func (m *TeamManager) SetSessionBarrier(barrier *sync.RWMutex) {
	if m == nil {
		return
	}
	m.barrierMu.Lock()
	m.sessionBarrier = barrier
	m.barrierMu.Unlock()
}

func (m *TeamManager) lockSessionSnapshot() (func(), bool) {
	m.barrierMu.RLock()
	barrier := m.sessionBarrier
	m.barrierMu.RUnlock()
	if barrier == nil {
		return func() {}, false
	}
	barrier.RLock()
	return barrier.RUnlock, true
}

// TeamSessionRuntime is the workspace-specific prompt/hook pair inherited by
// teammates. It is published and read as one snapshot during session changes.
type TeamSessionRuntime struct {
	System      string
	HookRunner  *hooks.Runner
	SessionID   string
	CWD         string
	ToolRuntime types.ToolRuntimeContext
}

type teamLaunchRuntime struct {
	session           TeamSessionRuntime
	provider          provider.Provider
	registry          *registry.Registry
	permissionHandler loop.PermissionHandler
	skillManager      *skills.Manager
	background        *BackgroundTaskManager
}

func (m *TeamManager) SetSessionRuntime(runtime TeamSessionRuntime) {
	if m == nil {
		return
	}
	m.mu.Lock()
	runtime.ToolRuntime = cloneToolRuntimeContext(runtime.ToolRuntime)
	m.sessionRuntime = runtime
	m.runtimeSet = true
	m.selectCurrentTeamLocked(true)
	m.mu.Unlock()
}

func (m *TeamManager) SetSessionHookRunner(runner *hooks.Runner) {
	if m == nil {
		return
	}
	m.mu.Lock()
	if !m.runtimeSet {
		m.sessionRuntime = TeamSessionRuntime{System: m.System, HookRunner: m.HookRunner}
		m.runtimeSet = true
	}
	m.sessionRuntime.HookRunner = runner
	m.mu.Unlock()
}

func (m *TeamManager) SetSessionIdentityRuntime(sessionID, cwd string, runtime types.ToolRuntimeContext) {
	if m == nil {
		return
	}
	m.mu.Lock()
	if !m.runtimeSet {
		m.sessionRuntime = TeamSessionRuntime{System: m.System, HookRunner: m.HookRunner}
		m.runtimeSet = true
	}
	m.sessionRuntime.SessionID = strings.TrimSpace(sessionID)
	m.sessionRuntime.CWD = strings.TrimSpace(cwd)
	m.sessionRuntime.ToolRuntime = cloneToolRuntimeContext(runtime)
	m.selectCurrentTeamLocked(true)
	m.mu.Unlock()
}

func (m *TeamManager) SetChildPermissionHandler(handler loop.PermissionHandler) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.PermissionHandler = handler
	m.mu.Unlock()
}

func (m *TeamManager) SessionRuntime() TeamSessionRuntime {
	if m == nil {
		return TeamSessionRuntime{}
	}
	unlock, _ := m.lockSessionSnapshot()
	defer unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.runtimeSet {
		runtime := m.sessionRuntime
		runtime.ToolRuntime = cloneToolRuntimeContext(runtime.ToolRuntime)
		return runtime
	}
	return TeamSessionRuntime{System: m.System, HookRunner: m.HookRunner}
}

// captureLaunchRuntime freezes every workspace-sensitive Team execution field
// beneath the same session publication barrier. The cloned registry is pinned
// before releasing the barrier so mutable file/shell/plan tools cannot follow
// a later foreground retarget.
func (m *TeamManager) captureLaunchRuntime() teamLaunchRuntime {
	if m == nil {
		return teamLaunchRuntime{}
	}
	unlock, sessionBarrierHeld := m.lockSessionSnapshot()
	defer unlock()

	m.mu.Lock()
	session := TeamSessionRuntime{System: m.System, HookRunner: m.HookRunner}
	if m.runtimeSet {
		session = m.sessionRuntime
	}
	if strings.TrimSpace(session.CWD) == "" {
		session.CWD = strings.TrimSpace(m.CWD)
	}
	if strings.TrimSpace(session.SessionID) == "" {
		session.SessionID = m.currentSessionIDLocked()
	}
	session.ToolRuntime = cloneToolRuntimeContext(session.ToolRuntime)
	providerSnapshot := snapshotAgentProvider(m.Provider)
	registrySource := m.Registry
	permissionHandler := m.PermissionHandler
	skillManager := m.SkillManager
	background := m.Background
	m.mu.Unlock()

	var pinnedRegistry *registry.Registry
	if registrySource != nil {
		pinnedRegistry = registrySource.Clone()
		// Team workers are subagents too: snapshot the live parent permission
		// policy at the team creation boundary, not when the session opened.
		if runtime, ok := pinnedRegistry.RuntimeContextWithinSessionBarrier(); ok {
			session.ToolRuntime = cloneToolRuntimeContext(runtime)
		} else if !sessionBarrierHeld && pinnedRegistry.HasRuntimeContextProvider() {
			session.ToolRuntime = cloneToolRuntimeContext(pinnedRegistry.RuntimeContext())
		}
	}
	if strings.TrimSpace(session.ToolRuntime.ProjectRoot) == "" {
		session.ToolRuntime.ProjectRoot = session.CWD
	}
	if len(session.ToolRuntime.AllowedDirs) == 0 && strings.TrimSpace(session.CWD) != "" {
		session.ToolRuntime.AllowedDirs = []string{session.CWD}
	}
	if strings.TrimSpace(session.ToolRuntime.SessionID) == "" {
		session.ToolRuntime.SessionID = session.SessionID
	}
	if pinnedRegistry != nil {
		baseRuntime := agentRuntimeContextProvider{
			snapshot: session.ToolRuntime,
			model:    currentTeamProviderModel(providerSnapshot),
		}
		pinRegistryForAgentRuntime(pinnedRegistry, baseRuntime, session.ToolRuntime)
		pinnedRegistry.SetRuntimeContextProvider(baseRuntime)
	}
	return teamLaunchRuntime{
		session:           session,
		provider:          providerSnapshot,
		registry:          pinnedRegistry,
		permissionHandler: permissionHandler,
		skillManager:      skillManager,
		background:        background,
	}
}

func (m *TeamManager) SetSessionIDResolver(resolver func() string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.sessionIDResolver = resolver
	m.mu.Unlock()
}

func (m *TeamManager) CurrentSessionID() string {
	if m == nil {
		return ""
	}
	unlock, _ := m.lockSessionSnapshot()
	defer unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.currentSessionIDLocked()
}

func (m *TeamManager) currentSessionIDLocked() string {
	resolver := m.sessionIDResolver
	if resolver == nil {
		resolver = m.SessionID
	}
	if resolver == nil {
		return ""
	}
	return strings.TrimSpace(resolver())
}

func (m *TeamManager) CurrentCWD() string {
	if m == nil {
		return ""
	}
	unlock, _ := m.lockSessionSnapshot()
	defer unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	return strings.TrimSpace(m.CWD)
}

// NewTeamManager creates a TeamManager backed by the given Coordinator.
func NewTeamManager(c *coordinator.Coordinator) *TeamManager {
	return &TeamManager{
		coordinator:   c,
		teams:         make(map[string]*TeamInfo),
		activeByOwner: make(map[teamOwnerKey]string),
	}
}

func currentTeamProviderModel(p provider.Provider) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(p.ModelID())
}

func currentTeamExecutionModel(ctx context.Context, p provider.Provider) string {
	if execCtx, ok := loop.ToolExecutionContextFromContext(ctx); ok {
		if model := strings.TrimSpace(execCtx.Model); model != "" {
			return model
		}
	}
	return currentTeamProviderModel(p)
}

func (m *TeamManager) currentTeamID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.currentTeamIDLocked()
}

func (m *TeamManager) currentTeamInfo() *TeamInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	teamID := m.currentTeamIDLocked()
	if teamID == "" {
		return nil
	}
	return m.teams[teamID]
}

func (m *TeamManager) CurrentTeamName() string {
	info := m.currentTeamInfo()
	if info == nil {
		return ""
	}
	return info.Name
}

func (m *TeamManager) findTeamByName(name string) *TeamInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	owner := m.currentTeamOwnerLocked()
	for _, info := range m.teams {
		if info != nil && teamInfoOwnedBy(info, owner) && strings.EqualFold(info.Name, name) {
			return info
		}
	}
	return nil
}

// SetProjectRoot retargets team lifecycle persistence during startup/session
// switches and restores the active team for the current session only.
func (m *TeamManager) SetProjectRoot(root string) {
	if m == nil || strings.TrimSpace(root) == "" {
		return
	}
	m.mu.Lock()
	before := m.currentTeamNameLocked()
	m.CWD = strings.TrimSpace(root)
	m.Lifecycle = NewRuntimeLifecycle(m.CWD)
	if m.runtimeSet {
		// Session identity/runtime is published immediately after the root in the
		// same outer barrier. Do not project the old session onto the new root.
		m.activeTeamID = ""
	} else {
		m.selectCurrentTeamLocked(true)
	}
	after := m.currentTeamNameLocked()
	m.mu.Unlock()
	if before != after {
		m.notifyTaskListChanged()
	}
}

func (m *TeamManager) lifecycleForRoot(root string) *RuntimeLifecycle {
	if m == nil {
		return nil
	}
	root = strings.TrimSpace(root)
	if root == "" {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Lifecycle == nil {
		m.Lifecycle = NewRuntimeLifecycle(root)
		return m.Lifecycle
	}
	if canonicalTeamOwnerRoot(m.Lifecycle.root) != canonicalTeamOwnerRoot(root) {
		// An operation can finish after the foreground moves to another
		// workspace. Return an exact immutable owner journal without retargeting
		// the manager's current lifecycle pointer back to the old root.
		return NewRuntimeLifecycle(root)
	}
	return m.Lifecycle
}

// restoreLifecycleTeamLocked reconstructs the manager's active team from the
// durable lifecycle journal. The team config remains the source of member and
// mailbox details; this restores the in-process pointer after session resume.
func (m *TeamManager) restoreLifecycleTeamLocked() {
	if m == nil {
		return
	}
	owner := m.currentTeamOwnerLocked()
	if owner == (teamOwnerKey{}) {
		return
	}
	if id := m.activeByOwner[owner]; id != "" && m.teams[id] != nil {
		m.activeTeamID = id
		return
	}
	root := owner.ProjectRoot
	if root == "" {
		root = strings.TrimSpace(m.CWD)
	}
	if root != "" && (m.Lifecycle == nil || canonicalTeamOwnerRoot(m.Lifecycle.root) != canonicalTeamOwnerRoot(root)) {
		m.Lifecycle = NewRuntimeLifecycle(root)
	}
	if m.Lifecycle == nil {
		return
	}
	active, err := m.Lifecycle.ActiveState()
	if err != nil {
		return
	}
	for i := len(active) - 1; i >= 0; i-- {
		event := active[i]
		if event.Type != LifecycleTeamCreate {
			continue
		}
		if owner.SessionID != "" && event.SessionID != "" && event.SessionID != owner.SessionID {
			continue
		}
		if eventRoot := canonicalTeamOwnerRoot(lifecyclePayloadString(event.Payload, "owner_project_root")); eventRoot != "" && eventRoot != owner.ProjectRoot {
			continue
		}
		name, _ := event.Payload["name"].(string)
		if strings.TrimSpace(name) == "" {
			continue
		}
		info := &TeamInfo{
			ID:               event.EntityID,
			Name:             name,
			StorageName:      lifecyclePayloadString(event.Payload, "storage_name"),
			Description:      lifecyclePayloadString(event.Payload, "description"),
			LeadAgentID:      lifecyclePayloadString(event.Payload, "lead_agent_id"),
			FilePath:         lifecyclePayloadString(event.Payload, "file_path"),
			Agents:           lifecyclePayloadStrings(event.Payload, "agents"),
			OwnerSessionID:   owner.SessionID,
			OwnerProjectRoot: owner.ProjectRoot,
			coordinator:      coordinator.NewCoordinator(),
		}
		if info.StorageName != "" {
			if _, err := swarm.LoadTeamConfig(info.StorageName); err != nil {
				continue
			}
		}
		if m.teams == nil {
			m.teams = make(map[string]*TeamInfo)
		}
		m.teams[info.ID] = info
		if m.activeByOwner == nil {
			m.activeByOwner = make(map[teamOwnerKey]string)
		}
		m.activeByOwner[owner] = info.ID
		m.activeTeamID = info.ID
		m.coordinator = info.coordinator
		return
	}
}

func (m *TeamManager) selectCurrentTeamLocked(restore bool) {
	if m == nil {
		return
	}
	owner := m.currentTeamOwnerLocked()
	m.activeTeamID = ""
	if id := m.activeByOwner[owner]; id != "" && m.teams[id] != nil {
		m.activeTeamID = id
		if m.teams[id].coordinator == nil {
			m.teams[id].coordinator = coordinator.NewCoordinator()
		}
		m.coordinator = m.teams[id].coordinator
		return
	}
	m.coordinator = coordinator.NewCoordinator()
	if restore {
		m.restoreLifecycleTeamLocked()
	}
}

func (m *TeamManager) currentTeamIDLocked() string {
	if m == nil {
		return ""
	}
	owner := m.currentTeamOwnerLocked()
	if id := m.activeByOwner[owner]; id != "" && m.teams[id] != nil {
		return id
	}
	if info := m.teams[m.activeTeamID]; info != nil && teamInfoOwnedBy(info, owner) {
		return m.activeTeamID
	}
	return ""
}

func (m *TeamManager) teamIDForOwnerLocked(owner teamOwnerKey) string {
	if m == nil {
		return ""
	}
	if id := m.activeByOwner[owner]; id != "" && m.teams[id] != nil {
		return id
	}
	if info := m.teams[m.activeTeamID]; info != nil && teamInfoOwnedBy(info, owner) {
		return m.activeTeamID
	}
	return ""
}

func (m *TeamManager) teamInfoForOwnerLocked(owner teamOwnerKey) *TeamInfo {
	return m.teams[m.teamIDForOwnerLocked(owner)]
}

func (m *TeamManager) currentTeamOwnerLocked() teamOwnerKey {
	if m == nil {
		return teamOwnerKey{}
	}
	sessionID := ""
	projectRoot := ""
	if m.runtimeSet {
		sessionID = strings.TrimSpace(m.sessionRuntime.SessionID)
		projectRoot = firstNonEmpty(m.sessionRuntime.ToolRuntime.ProjectRoot, m.sessionRuntime.CWD)
	}
	if sessionID == "" {
		sessionID = m.currentSessionIDLocked()
	}
	if strings.TrimSpace(projectRoot) == "" {
		projectRoot = m.CWD
	}
	return teamOwnerKey{SessionID: sessionID, ProjectRoot: canonicalTeamOwnerRoot(projectRoot)}
}

func teamOwnerFromRuntime(runtime TeamSessionRuntime) teamOwnerKey {
	return teamOwnerKey{
		SessionID:   strings.TrimSpace(runtime.SessionID),
		ProjectRoot: canonicalTeamOwnerRoot(firstNonEmpty(runtime.ToolRuntime.ProjectRoot, runtime.CWD)),
	}
}

func teamInfoOwnedBy(info *TeamInfo, owner teamOwnerKey) bool {
	if info == nil {
		return false
	}
	// In-memory teams created before owner tracking are adopted only by the
	// currently projected owner, preserving embedders that seed TeamInfo.
	if strings.TrimSpace(info.OwnerSessionID) == "" && strings.TrimSpace(info.OwnerProjectRoot) == "" {
		return true
	}
	return strings.TrimSpace(info.OwnerSessionID) == owner.SessionID &&
		canonicalTeamOwnerRoot(info.OwnerProjectRoot) == owner.ProjectRoot
}

func canonicalTeamOwnerRoot(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return ""
	}
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	root = filepath.Clean(root)
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = filepath.Clean(resolved)
	}
	return root
}

func lifecyclePayloadString(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}

func lifecyclePayloadStrings(payload map[string]any, key string) []string {
	value := payload[key]
	switch items := value.(type) {
	case []string:
		return append([]string(nil), items...)
	case []any:
		out := make([]string, 0, len(items))
		for _, item := range items {
			if text, ok := item.(string); ok {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func formatLeadAgentID(teamName string) string {
	return fmt.Sprintf("team-lead@%s", strings.TrimSpace(teamName))
}

func broadcastDropSuffix(dropped int) string {
	if dropped <= 0 {
		return ""
	}
	return fmt.Sprintf(" (%d delivery slot(s) were full)", dropped)
}

// ─── SendMessageTool ──────────────────────────────────────────────────────────

// SendMessageTool sends a message to a teammate agent via the coordinator's MessageBus.
type SendMessageTool struct{ Manager *TeamManager }

func NewSendMessageTool(m *TeamManager) *SendMessageTool { return &SendMessageTool{Manager: m} }

func (t *SendMessageTool) Name() string { return "SendMessage" }

func legacyDToolError(lang i18n.Language, key i18n.Key, args ...any) types.ToolResult {
	return types.ToolResult{Content: i18n.Format(lang, key, args...), IsError: true}
}

func (t *SendMessageTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	lang := i18n.DetectOrLoadLanguage()
	in, err := types.DecodeStrictToolInput[SendMessageInput](input)
	if err != nil {
		return ErrorResponse(err), nil
	}
	// SendMessage can resume a retained local agent or mutate a team mailbox.
	// Revalidate the private run generation before either side effect so an old
	// A run cannot keep operating after a worktree/session publication moved the
	// shared skill/runtime authority to B. The narrower retained-agent owner
	// check below additionally binds the target to the exact session and root.
	skillAuthority := toolSkillAuthority{}
	if t.Manager != nil && t.Manager.SkillManager != nil {
		var authorityErr error
		skillAuthority, authorityErr = validateToolSkillAuthority(ctx, t.Manager.SkillManager)
		if authorityErr != nil {
			return ErrorResponse(authorityErr), nil
		}
	}
	if strings.TrimSpace(in.To) == "" {
		return legacyDToolError(lang, i18n.KeyToolLegacyDSendToRequired), nil
	}
	if scheme := unsupportedPeerScheme(in.To); scheme != "" {
		return legacyDToolError(lang, i18n.KeyToolLegacyDSendSchemeUnsupported, scheme), nil
	}
	addr := parsePeerAddress(in.To)
	if addr.scheme == "uds" && strings.TrimSpace(addr.target) == "" {
		return legacyDToolError(lang, i18n.KeyToolLegacyDSendAddressRequired), nil
	}
	if addr.scheme == "bridge" {
		// SM-01: bridge:<session-id> demands explicit user consent that
		// bypassPermissions cannot auto-approve. The runtime layer must
		// surface a prompt before the message is delivered. We refuse the
		// send here when the bridge has not been pre-authorised; the host
		// owns the actual approval flow (SendMessageTool integrates with
		// HostApprovalSink which is created out-of-band).
		sessionID, _ := RequiresBridgePermission(in.To)
		if !bridgePermissionGranted(sessionID) {
			return legacyDToolError(lang, i18n.KeyToolLegacyDSendBridgeConsent, sessionID), nil
		}
		// Remote Control transport was removed from the current alignment scope.
		// Keep the legacy consent gate isolated, but never reinterpret a bridge
		// address as a teammate mailbox target.
		return legacyDToolError(lang, i18n.KeyToolLegacyDSendBridgeUnavailable), nil
	}
	if strings.Contains(in.To, "@") {
		return legacyDToolError(lang, i18n.KeyToolLegacyDSendBareRecipientRequired), nil
	}

	content, isPlainTextMessage := in.Message.(string)
	structured, isStructured, err := decodeStructuredSendMessage(in.Message)
	if err != nil {
		return legacyDToolError(lang, i18n.KeyToolLegacyDSendDecodeFailed, err), nil
	}
	if !isPlainTextMessage && !isStructured {
		return legacyDToolError(lang, i18n.KeyToolLegacyDRequiredFieldQuoted, "message"), nil
	}
	if isStructured {
		if err := validateStructuredSendMessageInput(t, in.Message, structured); err != nil {
			return ErrorResponse(err), nil
		}
	}
	if isPlainTextMessage && addr.scheme != "uds" && strings.TrimSpace(in.Summary) == "" {
		return legacyDToolError(lang, i18n.KeyToolLegacyDSendSummaryRequired), nil
	}
	if isStructured && in.To == "*" {
		return legacyDToolError(lang, i18n.KeyToolLegacyDSendStructuredBroadcast), nil
	}
	if isStructured && addr.scheme != "other" {
		return legacyDToolError(lang, i18n.KeyToolLegacyDSendStructuredCrossSession), nil
	}
	if isStructured && structured.Type == "shutdown_response" && !strings.EqualFold(in.To, teamLeadName) {
		return legacyDToolError(lang, i18n.KeyToolLegacyDSendShutdownTarget, teamLeadName), nil
	}
	if isStructured && structured.Type == "shutdown_response" && structured.Approve != nil && !*structured.Approve && strings.TrimSpace(structured.Reason) == "" {
		return legacyDToolError(lang, i18n.KeyToolLegacyDSendShutdownRejectReason), nil
	}

	if addr.scheme == "uds" && isPlainTextMessage {
		return t.sendToUnixSocket(ctx, addr.target, content, in.Summary)
	}

	if isPlainTextMessage && in.To != "*" && t.Manager != nil && t.Manager.Background != nil {
		if snap, handled, err := t.Manager.Background.queueAgentPromptWithAuthority(ctx, in.To, content, skillAuthority, t.Manager.SkillManager); handled {
			if err != nil {
				return sendMessageResponse(SendMessageResult{Success: false, Message: i18n.Format(lang, i18n.KeyToolLegacyDSendAgentResumeFailed, in.To, err)})
			}
			if snap.Status == "running" {
				return sendMessageResponse(SendMessageResult{Success: true, Message: i18n.Format(lang, i18n.KeyToolLegacyDSendQueued, in.To)})
			}
			return sendMessageResponse(SendMessageResult{Success: true, Message: i18n.Format(lang, i18n.KeyToolLegacyDSendAgentResumed, in.To, snap.Status, snap.OutputPath)})
		}
	}

	teamContext, err := resolveSendMessageTeamContext(t.Manager)
	if err != nil {
		return ErrorResponse(err), nil
	}
	if in.To == "*" {
		if !teamContext.Active {
			return legacyDToolError(lang, i18n.KeyToolLegacyDSendNoTeamContext), nil
		}
		persisted, loadErr := swarm.LoadTeamConfig(teamContext.Name)
		if loadErr != nil {
			return legacyDToolError(lang, i18n.KeyToolLegacyDSendTeamMissing, teamContext.Name), nil
		}
		return t.executeMailboxMessage(ctx, persisted, in, content)
	}

	cfg := teamContext.Config
	if cfg == nil {
		cfg = &swarm.TeamConfig{Name: teamContext.Name}
	}
	if isPlainTextMessage {
		return t.executeMailboxMessage(ctx, cfg, in, content)
	}
	if isStructured {
		return t.executeStructuredMailboxMessage(ctx, cfg, in, structured, teamContext.Active)
	}
	return legacyDToolError(lang, i18n.KeyToolLegacyDSendMessageTypeInvalid), nil
}

// ─── TeamCreateTool ───────────────────────────────────────────────────────────

// TeamCreateTool registers agents in the coordinator and creates a named team.
type TeamCreateTool struct{ Manager *TeamManager }

func NewTeamCreateTool(m *TeamManager) *TeamCreateTool { return &TeamCreateTool{Manager: m} }

func (t *TeamCreateTool) SetChildPermissionHandler(handler loop.PermissionHandler) {
	if t != nil && t.Manager != nil {
		t.Manager.SetChildPermissionHandler(handler)
	}
}

func (t *TeamCreateTool) Name() string        { return "TeamCreate" }
func (t *TeamCreateTool) Description() string { return "Create a team of agents for parallel work" }

func (t *TeamCreateTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(
		map[string]any{
			"team_name": map[string]any{
				"type":        "string",
				"description": "Name for the new team to create.",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "Team description/purpose.",
			},
			"agent_type": map[string]any{
				"type":        "string",
				"description": `Type/role of the team lead (e.g., "researcher", "test-runner"). Used for team file and inter-agent coordination.`,
			},
		},
		"team_name",
	)
}

func (t *TeamCreateTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	lang := i18n.DetectOrLoadLanguage()
	in, toolErr := parseStrictInputOrError[TeamCreateInput](input)
	if toolErr != nil {
		return *toolErr, nil
	}
	teamName := strings.TrimSpace(in.TeamName)
	if teamName == "" {
		return legacyDToolError(lang, i18n.KeyToolLegacyDRequiredFieldQuoted, "team_name"), nil
	}
	t.Manager.mutationMu.Lock()
	defer t.Manager.mutationMu.Unlock()
	skillAuthority, authorityErr := validateToolSkillAuthority(ctx, t.Manager.SkillManager)
	if authorityErr != nil {
		return ErrorResponse(authorityErr), nil
	}
	teamLaunch := t.Manager.captureLaunchRuntime()
	if err := skillAuthority.validateRuntime(teamLaunch.session.ToolRuntime); err != nil {
		return ErrorResponse(err), nil
	}
	teamOwner := teamOwnerFromRuntime(teamLaunch.session)
	teamSkillGeneration := skillAuthority.generation
	t.Manager.mu.Lock()
	existing := t.Manager.teamInfoForOwnerLocked(teamOwner)
	t.Manager.mu.Unlock()
	if existing != nil {
		return legacyDToolError(lang, i18n.KeyToolLegacyDTeamAlreadyLeading, existing.Name), nil
	}
	finalTeamName, err := uniqueTeamName(teamName)
	if err != nil {
		return ErrorResponse(err), nil
	}

	leadAgentID := formatLeadAgentID(finalTeamName)
	storageName := teamStorageName(finalTeamName)
	teamFilePath, pathErr := swarm.TeamConfigPath(storageName)
	if pathErr != nil {
		return swarmErrorResponse(pathErr), nil
	}
	teamRuntime := teamLaunch.session
	cacheLineageID := firstNonEmpty(teamRuntime.SessionID, teamRuntime.ToolRuntime.SessionID)
	if execution, ok := loop.ToolExecutionContextFromContext(ctx); ok {
		if lineageID := strings.TrimSpace(execution.CacheLineageID); lineageID != "" {
			cacheLineageID = lineageID
		}
	}
	teamCWD := teamRuntime.CWD
	if strings.TrimSpace(teamCWD) == "" {
		teamCWD = currentTeamCWD(t.Manager)
	}
	leadSessionID := teamRuntime.SessionID
	if strings.TrimSpace(leadSessionID) == "" {
		leadSessionID = currentTeamSessionID(t.Manager)
	}
	teamProvider := teamLaunch.provider
	teamModel := currentTeamExecutionModel(ctx, teamProvider)
	teamRegistry := teamLaunch.registry
	teamPermissionHandler := agentPermissionHandlerForSnapshot(
		teamRuntime.ToolRuntime,
		teamLaunch.permissionHandler,
		approvalRouteParentSession,
		agentProfile{},
		leadSessionID,
	)

	role := strings.TrimSpace(in.AgentType)
	if role == "" {
		role = "team-lead"
	}
	agentSpecs := []TeamAgentSpec{{ID: leadAgentID, Role: role}}

	agentIDs := make([]string, 0, len(agentSpecs))
	agentsToRegister := make([]*coordinator.Agent, 0, len(agentSpecs))
	for _, spec := range agentSpecs {
		id := strings.TrimSpace(spec.ID)
		if id == "" {
			return legacyDToolError(lang, i18n.KeyToolLegacyDTeamAgentIDRequired), nil
		}

		// Capture loop vars for the closure.
		agentID := id
		agentType := strings.TrimSpace(spec.Role)
		mgr := t.Manager

		var executeFn coordinator.AgentFunc
		if teamProvider != nil && teamRegistry != nil {
			// Real sub-agent: spawns a QueryLoop for each task.
			executeFn = func(ctx context.Context, task *coordinator.Task) (string, error) {
				agentCtx := ctx
				subReg := teamRegistry.Clone()
				removePermissionTransitionToolsFromAgentRegistry(subReg)
				model := teamModel
				childToolRuntime := cloneToolRuntimeContext(teamRuntime.ToolRuntime)
				childToolRuntime.SessionID = agentID
				childToolRuntime.AgentID = agentID
				runtimeProvider := agentRuntimeContextProvider{
					snapshot: childToolRuntime, agentID: agentID,
					model: model,
				}
				pinRegistryForAgentRuntime(subReg, runtimeProvider, childToolRuntime)
				subReg.SetRuntimeContextProvider(runtimeProvider)
				// Give the sub-agent its own depth-incremented AgentTool.
				subReg.Register(&AgentTool{
					Provider:          teamProvider,
					Registry:          subReg,
					System:            teamRuntime.System,
					Model:             model,
					Background:        teamLaunch.background,
					TeamManager:       mgr,
					SkillManager:      teamLaunch.skillManager,
					HookRunner:        teamRuntime.HookRunner,
					PermissionHandler: teamPermissionHandler,
					TeamMember:        true,
					NonInteractive:    true,
					Depth:             1,
					MaxDepth:          DefaultMaxAgentDepth,
				})
				subLoop := loop.New(teamProvider, subReg, loop.Config{
					DisableMaxTurns:        true,
					System:                 teamRuntime.System,
					Model:                  model,
					MaxTokens:              16384,
					SessionID:              agentID,
					CacheLineageID:         cacheLineageID,
					AgentID:                agentID,
					AgentType:              agentType,
					ProjectRoot:            teamRuntime.ToolRuntime.ProjectRoot,
					CWD:                    teamRuntime.CWD,
					SkillManager:           teamLaunch.skillManager,
					SkillProjectGeneration: teamSkillGeneration,
					HookRunner:             teamRuntime.HookRunner,
					PermissionHandler:      teamPermissionHandler,
				})
				var result strings.Builder
				err := subLoop.Run(agentCtx, task.Description, func(event loop.Event) {
					if event.Type == loop.EventText {
						result.WriteString(event.Text)
					}
				})
				output := result.String()
				if output == "" && err == nil {
					output = i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolLegacyDTeamAgentNoOutput, agentID, task.ID)
				}
				return output, err
			}
		} else {
			// Stub fallback when no provider is configured.
			executeFn = func(_ context.Context, task *coordinator.Task) (string, error) {
				return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolLegacyDTeamAgentCompleted, agentID, task.ID), nil
			}
		}

		agent := &coordinator.Agent{
			ID:           id,
			Name:         spec.Role,
			Capabilities: []string{spec.Role},
			Execute:      executeFn,
		}
		agentsToRegister = append(agentsToRegister, agent)
		agentIDs = append(agentIDs, id)
	}

	if err := swarm.CreateTeamConfigAs(storageName, buildPersistedTeamConfig(finalTeamName, in.Description, leadAgentID, leadSessionID, teamCWD, teamModel, agentSpecs)); err != nil {
		return swarmErrorResponse(err), nil
	}

	teamID := ""
	commitErr := skillAuthority.withGenerationLease(t.Manager.SkillManager, func() error {
		t.Manager.mu.Lock()
		defer t.Manager.mu.Unlock()
		if t.Manager.currentTeamOwnerLocked() != teamOwner {
			return i18n.WrapInternalError(
				i18n.KeyLoopQueryValidateSkillGenerationFailed,
				skills.ErrSkillProjectGenerationChanged,
			)
		}
		if current := t.Manager.teamInfoForOwnerLocked(teamOwner); current != nil {
			return i18n.NewError(i18n.KeyToolLegacyDTeamAlreadyLeading, current.Name)
		}
		teamCoordinator := coordinator.NewCoordinator()
		for _, agent := range agentsToRegister {
			teamCoordinator.RegisterAgent(agent)
		}
		t.Manager.nextTeamID++
		teamID = fmt.Sprintf("team-%d", t.Manager.nextTeamID)
		if t.Manager.activeByOwner == nil {
			t.Manager.activeByOwner = make(map[teamOwnerKey]string)
		}
		t.Manager.activeByOwner[teamOwner] = teamID
		t.Manager.activeTeamID = teamID
		t.Manager.coordinator = teamCoordinator
		t.Manager.teams[teamID] = &TeamInfo{
			ID: teamID, Name: finalTeamName, StorageName: storageName,
			Description: in.Description, LeadAgentID: leadAgentID,
			FilePath: teamFilePath, Agents: agentIDs,
			OwnerSessionID: teamOwner.SessionID, OwnerProjectRoot: teamOwner.ProjectRoot,
			coordinator: teamCoordinator,
		}
		return nil
	})
	if commitErr != nil {
		_ = swarm.DeleteTeamConfig(storageName)
		return ErrorResponse(commitErr), nil
	}
	t.Manager.notifyTaskListChanged()

	if lifecycle := t.Manager.lifecycleForRoot(teamOwner.ProjectRoot); lifecycle != nil {
		_ = lifecycle.Publish(ctx, RuntimeLifecycleEvent{
			Type:      LifecycleTeamCreate,
			EntityID:  teamID,
			ToolName:  "TeamCreate",
			Status:    "active",
			SessionID: leadSessionID,
			Payload: map[string]any{
				"name":               finalTeamName,
				"storage_name":       storageName,
				"description":        in.Description,
				"lead_agent_id":      leadAgentID,
				"file_path":          teamFilePath,
				"agents":             append([]string(nil), agentIDs...),
				"owner_project_root": teamOwner.ProjectRoot,
			},
		})
	}

	return ResponseJSON(map[string]any{
		"team_name":      finalTeamName,
		"team_file_path": teamFilePath,
		"lead_agent_id":  leadAgentID,
	})
}

// ─── TeamDeleteTool ───────────────────────────────────────────────────────────

// TeamDeleteTool removes a team (and its metadata) from the manager.
type TeamDeleteTool struct{ Manager *TeamManager }

func NewTeamDeleteTool(m *TeamManager) *TeamDeleteTool { return &TeamDeleteTool{Manager: m} }

func (t *TeamDeleteTool) Name() string        { return "TeamDelete" }
func (t *TeamDeleteTool) Description() string { return "Delete a team and its agents" }

func (t *TeamDeleteTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(map[string]any{})
}

func (t *TeamDeleteTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	lang := i18n.DetectOrLoadLanguage()
	if _, toolErr := parseStrictInputOrError[TeamDeleteInput](input); toolErr != nil {
		return *toolErr, nil
	}
	t.Manager.mutationMu.Lock()
	defer t.Manager.mutationMu.Unlock()
	skillAuthority, authorityErr := validateToolSkillAuthority(ctx, t.Manager.SkillManager)
	if authorityErr != nil {
		return ErrorResponse(authorityErr), nil
	}
	teamLaunch := t.Manager.captureLaunchRuntime()
	if err := skillAuthority.validateRuntime(teamLaunch.session.ToolRuntime); err != nil {
		return ErrorResponse(err), nil
	}
	owner := teamOwnerFromRuntime(teamLaunch.session)

	t.Manager.mu.Lock()
	teamID := t.Manager.teamIDForOwnerLocked(owner)
	info, ok := t.Manager.teams[teamID]
	t.Manager.mu.Unlock()

	if !ok {
		return ResponseJSON(map[string]any{
			"success": true,
			"message": i18n.Text(lang, i18n.KeyToolLegacyDTeamNothingToDelete),
		})
	}
	activeMembers, err := activeNonLeadTeamMembers(info)
	if err != nil {
		return swarmErrorResponse(err), nil
	}
	if len(activeMembers) > 0 {
		return ResponseJSON(map[string]any{
			"success":   false,
			"message":   i18n.Format(lang, i18n.KeyToolLegacyDTeamActiveMembers, len(activeMembers), strings.Join(activeMembers, ", ")),
			"team_name": info.Name,
		})
	}
	// Remove durable publication first while the manager still retains the
	// complete retry inventory. If the fenced in-memory commit later loses its
	// authority, restore the exact config before returning the error.
	var durableBackup *swarm.TeamConfig
	if info.StorageName != "" {
		if loaded, loadErr := swarm.LoadTeamConfig(info.StorageName); loadErr == nil {
			durableBackup = loaded
		}
		if err := swarm.DeleteTeamConfig(info.StorageName); err != nil {
			return swarmErrorResponse(err), nil
		}
	}
	activeChanged := false
	if err := skillAuthority.withGenerationLease(t.Manager.SkillManager, func() error {
		t.Manager.mu.Lock()
		defer t.Manager.mu.Unlock()
		if t.Manager.currentTeamOwnerLocked() != owner || t.Manager.teamIDForOwnerLocked(owner) != teamID {
			return i18n.WrapInternalError(
				i18n.KeyLoopQueryValidateSkillGenerationFailed,
				skills.ErrSkillProjectGenerationChanged,
			)
		}
		delete(t.Manager.teams, teamID)
		delete(t.Manager.activeByOwner, owner)
		if info.coordinator != nil {
			for _, agentID := range info.Agents {
				info.coordinator.RemoveAgent(agentID)
			}
		}
		if t.Manager.activeTeamID == teamID {
			t.Manager.activeTeamID = ""
			t.Manager.coordinator = coordinator.NewCoordinator()
			activeChanged = true
		}
		return nil
	}); err != nil {
		if durableBackup != nil {
			if restoreErr := swarm.CreateTeamConfigAs(info.StorageName, durableBackup); restoreErr != nil {
				return ErrorResponse(i18n.WrapInternalError(i18n.KeyAuxSwarmFailed, errors.Join(err, restoreErr))), nil
			}
		}
		return ErrorResponse(err), nil
	}
	if activeChanged {
		t.Manager.notifyTaskListChanged()
	}
	if lifecycle := t.Manager.lifecycleForRoot(owner.ProjectRoot); lifecycle != nil {
		_ = lifecycle.Publish(ctx, RuntimeLifecycleEvent{
			Type: LifecycleTeamDelete, EntityID: teamID, ToolName: "TeamDelete",
			Status: "deleted", SessionID: owner.SessionID,
			Payload: map[string]any{
				"name": info.Name, "storage_name": info.StorageName,
				"owner_project_root": owner.ProjectRoot,
			},
		})
	}
	return ResponseJSON(map[string]any{
		"success":   true,
		"message":   i18n.Format(lang, i18n.KeyToolLegacyDTeamDeleted, info.Name),
		"team_name": info.Name,
	})
}

// ─── TeamDispatchTool ─────────────────────────────────────────────────────────

// TeamDispatchTool adds tasks to the coordinator queue and dispatches them.
type TeamDispatchTool struct{ Manager *TeamManager }

func NewTeamDispatchTool(m *TeamManager) *TeamDispatchTool { return &TeamDispatchTool{Manager: m} }

func (t *TeamDispatchTool) Name() string { return "TeamDispatch" }
func (t *TeamDispatchTool) Description() string {
	return "Dispatch tasks to a team's agents and return aggregated results"
}

func (t *TeamDispatchTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"team_id": map[string]any{
				"type":        "string",
				"description": "ID of the team to dispatch tasks to",
			},
			"tasks": map[string]any{
				"type":        "array",
				"description": "List of tasks to dispatch",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"description": map[string]any{
							"type":        "string",
							"description": "Task description / prompt for the agent",
						},
						"priority": map[string]any{
							"type":        "integer",
							"description": "Task priority (higher = runs first), defaults to 0",
						},
					},
					"required": []string{"description"},
				},
			},
		},
		Required: []string{"team_id", "tasks"},
	}
}

func (t *TeamDispatchTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	lang := i18n.DetectOrLoadLanguage()
	in, toolErr := parseInputOrError[TeamDispatchInput](input)
	if toolErr != nil {
		return *toolErr, nil
	}
	if strings.TrimSpace(in.TeamID) == "" {
		return legacyDToolError(lang, i18n.KeyToolLegacyDRequiredFieldQuoted, "team_id"), nil
	}
	if len(in.Tasks) == 0 {
		return legacyDToolError(lang, i18n.KeyToolLegacyDTeamTasksRequired), nil
	}
	for _, taskSpec := range in.Tasks {
		if strings.TrimSpace(taskSpec.Description) == "" {
			return legacyDToolError(lang, i18n.KeyToolLegacyDTeamTaskDescriptionRequired), nil
		}
	}
	skillAuthority, authorityErr := validateToolSkillAuthority(ctx, t.Manager.SkillManager)
	if authorityErr != nil {
		return ErrorResponse(authorityErr), nil
	}
	teamLaunch := t.Manager.captureLaunchRuntime()
	if err := skillAuthority.validateRuntime(teamLaunch.session.ToolRuntime); err != nil {
		return ErrorResponse(err), nil
	}
	owner := teamOwnerFromRuntime(teamLaunch.session)
	missing := false
	var dispatchCoordinator *coordinator.Coordinator
	if err := skillAuthority.withGenerationLease(t.Manager.SkillManager, func() error {
		t.Manager.mu.Lock()
		defer t.Manager.mu.Unlock()
		if t.Manager.currentTeamOwnerLocked() != owner {
			return i18n.WrapInternalError(
				i18n.KeyLoopQueryValidateSkillGenerationFailed,
				skills.ErrSkillProjectGenerationChanged,
			)
		}
		info := t.Manager.teams[in.TeamID]
		if info == nil || !teamInfoOwnedBy(info, owner) {
			missing = true
			return nil
		}
		dispatchCoordinator = info.coordinator
		if dispatchCoordinator == nil {
			dispatchCoordinator = coordinator.NewCoordinator()
			info.coordinator = dispatchCoordinator
		}
		for _, taskSpec := range in.Tasks {
			dispatchCoordinator.AddTask(taskSpec.Description, taskSpec.Priority)
		}
		return nil
	}); err != nil {
		return ErrorResponse(err), nil
	}
	if missing {
		return types.ToolResult{
			Content: i18n.Format(lang, i18n.KeyToolLegacyDTeamNotFound, in.TeamID),
			IsError: true,
		}, nil
	}

	// Dispatch — runs until all reachable tasks are done.
	results := dispatchCoordinator.Dispatch(ctx)

	if len(results) == 0 {
		return types.ToolResult{Content: i18n.Text(lang, i18n.KeyToolLegacyDTeamDispatchEmpty)}, nil
	}

	var sb strings.Builder
	sb.WriteString(i18n.Format(lang, i18n.KeyToolLegacyDTeamDispatchComplete, len(results)))
	for i, r := range results {
		sb.WriteString(i18n.Format(lang, i18n.KeyToolLegacyDTeamDispatchTaskHeader, i+1, r.TaskID, r.AgentID))
		if r.Error != nil {
			sb.WriteString(i18n.Format(lang, i18n.KeyToolLegacyDTeamDispatchError, r.Error.Error()))
		} else {
			sb.WriteString(r.Result)
			sb.WriteString("\n")
		}
	}

	return types.ToolResult{Content: sb.String()}, nil
}
