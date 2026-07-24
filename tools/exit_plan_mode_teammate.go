package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/swarm"
	"github.com/agent-dance/luban/types"
)

var ErrPlanApprovalNotPending = errors.New("plan approval request is not pending")

type pendingPlanApproval struct {
	AgentID     string
	RequestID   string
	PlanFile    string
	PlanContent string
	Allowed     []PlanAllowedPrompt
	State       *PlanState
	CreatedAt   time.Time
}

type TeammatePlanApprovalState struct {
	AgentID        string
	RequestID      string
	Active         bool
	Awaiting       bool
	Approved       bool
	PermissionMode string
	Feedback       string
}

type planApprovalCoordinator struct {
	mu      sync.Mutex
	pending map[string]pendingPlanApproval
	states  map[string]TeammatePlanApprovalState
}

var defaultPlanApprovalCoordinator = &planApprovalCoordinator{
	pending: make(map[string]pendingPlanApproval),
	states:  make(map[string]TeammatePlanApprovalState),
}

func (c *planApprovalCoordinator) register(pending pendingPlanApproval) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pending[pending.RequestID] = pending
	c.states[pending.AgentID] = TeammatePlanApprovalState{
		AgentID: pending.AgentID, RequestID: pending.RequestID, Active: true, Awaiting: true,
		PermissionMode: permissionModePlan,
	}
}

func (c *planApprovalCoordinator) unregister(requestID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if pending, ok := c.pending[requestID]; ok {
		delete(c.states, pending.AgentID)
	}
	delete(c.pending, requestID)
}

func (c *planApprovalCoordinator) resolve(requestID, from string, approved bool, feedback string) (TeammatePlanApprovalState, error) {
	if strings.TrimSpace(from) != teamLeadName {
		return TeammatePlanApprovalState{}, i18n.NewError(i18n.KeyToolIndirectPlanApprovalLeadOnly, teamLeadName)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	pending, ok := c.pending[strings.TrimSpace(requestID)]
	if !ok {
		return TeammatePlanApprovalState{}, ErrPlanApprovalNotPending
	}
	mode := permissionModePlan
	if pending.State != nil {
		pending.State.mu.RLock()
		runtime := pending.State.permissionRuntime
		pending.State.mu.RUnlock()
		if runtime != nil {
			if current := strings.TrimSpace(runtime.ToolRuntimeContext().PermissionMode); current != "" {
				mode = current
			}
		}
	}
	// In-process teammate tests and embedded runtimes can own a concrete
	// PlanState in addition to the retained agent session. Commit that state at
	// the same authenticated leader-response boundary. A failed commit leaves
	// the request pending so a mailbox delivery cannot manufacture an
	// "approved" snapshot while the runtime is still in plan mode.
	if approved && pending.State != nil && pending.State.IsActive() &&
		filepathClean(pending.State.PlanFile()) == filepathClean(pending.PlanFile) {
		if err := pending.State.commitExit(pending.Allowed, mode, false); err != nil {
			return TeammatePlanApprovalState{}, i18n.WrapError(i18n.KeyToolIndirectPlanApprovalCommit, err)
		}
	}
	delete(c.pending, pending.RequestID)
	state := TeammatePlanApprovalState{
		AgentID: pending.AgentID, RequestID: pending.RequestID, Active: !approved,
		Awaiting: false, Approved: approved, PermissionMode: mode,
		Feedback: strings.TrimSpace(feedback),
	}
	if !approved {
		state.PermissionMode = permissionModePlan
	}
	c.states[pending.AgentID] = state
	return state, nil
}

// ResolveTeammatePlanApprovalResponse is the authenticated in-process inbox
// consumer. Out-of-process teammates consume the same mailbox envelope.
// Leader approval cannot change a teammate's inherited permission snapshot.
func ResolveTeammatePlanApprovalResponse(requestID, from string, approved bool, feedback string) (TeammatePlanApprovalState, error) {
	return defaultPlanApprovalCoordinator.resolve(requestID, from, approved, feedback)
}

func TeammatePlanApprovalSnapshot(agentID string) (TeammatePlanApprovalState, bool) {
	defaultPlanApprovalCoordinator.mu.Lock()
	defer defaultPlanApprovalCoordinator.mu.Unlock()
	state, ok := defaultPlanApprovalCoordinator.states[strings.TrimSpace(agentID)]
	return state, ok
}

func (t *ExitPlanModeTool) withInProcessAgentID(agentID string) types.Tool {
	clone := *t
	clone.AgentID = strings.TrimSpace(agentID)
	clone.PlanModeRequired = true
	return &clone
}

func (t *ExitPlanModeTool) teammatePlanFilePath() string {
	root := ""
	if t != nil && t.State != nil {
		t.State.mu.RLock()
		root = t.State.projectRoot
		t.State.mu.RUnlock()
	}
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	name := sanitizeSwarmName(teammateAgentName(t.AgentID), "agent")
	return filepath.Join(root, ".claude", "plans", "plan-"+name+".md")
}

func teammateAgentName(agentID string) string {
	agentID = strings.TrimSpace(agentID)
	if at := strings.Index(agentID, "@"); at > 0 {
		return agentID[:at]
	}
	return agentID
}

func teammateTeamName(agentID string) string {
	agentID = strings.TrimSpace(agentID)
	if at := strings.Index(agentID, "@"); at >= 0 && at+1 < len(agentID) {
		return agentID[at+1:]
	}
	return ""
}

func teammatePlanFromExecutionContext(ctx context.Context) string {
	exec, ok := loop.ToolExecutionContextFromContext(ctx)
	if !ok {
		return ""
	}
	var sections []string
	for _, block := range exec.AssistantMessage.Content {
		switch value := block.(type) {
		case types.TextBlock:
			if text := strings.TrimSpace(value.Text); text != "" {
				sections = append(sections, text)
			}
		case *types.TextBlock:
			if value != nil {
				if text := strings.TrimSpace(value.Text); text != "" {
					sections = append(sections, text)
				}
			}
		}
	}
	return strings.Join(sections, "\n\n")
}

func (t *ExitPlanModeTool) executeTeammateExit(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	if !t.PlanModeRequired {
		return types.ToolResult{}, i18n.NewError(i18n.KeyToolIndirectPlanApprovalModeRequired)
	}
	normalized, err := t.NormalizeToolInput(ctx, input)
	if err != nil {
		return types.ToolResult{}, err
	}
	in, err := decodeExitPlanModeInput(normalized)
	if err != nil {
		return types.ToolResult{}, err
	}
	plan := ""
	if in.Plan != nil {
		plan = *in.Plan
	}
	if strings.TrimSpace(plan) == "" {
		return types.ToolResult{}, i18n.NewError(i18n.KeyToolIndirectPlanApprovalPlanRequired, in.PlanFilePath)
	}
	if err := os.MkdirAll(filepath.Dir(in.PlanFilePath), 0o755); err != nil {
		return types.ToolResult{}, i18n.WrapError(i18n.KeyToolIndirectPlanApprovalPrepareDir, err)
	}
	if _, err := persistEditedPlan(in.PlanFilePath, plan); err != nil {
		return types.ToolResult{}, i18n.WrapError(i18n.KeyToolIndirectPlanApprovalPersist, err)
	}

	teamName := teammateTeamName(t.AgentID)
	if t.TeamManager != nil {
		if cfg, cfgErr := loadActiveTeamConfig(t.TeamManager); cfgErr != nil {
			return types.ToolResult{}, cfgErr
		} else if cfg != nil {
			teamName = cfg.Name
		}
	}
	teamName = teamStorageName(teamName)
	if strings.TrimSpace(teamName) == "" || teamName == "team" {
		return types.ToolResult{}, i18n.NewError(i18n.KeyToolIndirectPlanApprovalTeamRequired)
	}
	mailbox, err := swarm.NewMailbox(teamName)
	if err != nil {
		return types.ToolResult{}, i18n.WrapInternalError(i18n.KeyAuxSwarmMailboxFailed, err)
	}
	agentName := teammateAgentName(t.AgentID)
	requestID := NewRequestID("plan_approval")
	timestamp := time.Now().UTC().Format(time.RFC3339)
	payload := map[string]any{
		"type": "plan_approval_request", "from": agentName, "timestamp": timestamp,
		"planFilePath": in.PlanFilePath, "planContent": plan, "requestId": requestID,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return types.ToolResult{}, i18n.WrapInternalError(i18n.KeyToolIndirectPlanApprovalEncodeRequest, err)
	}
	pending := pendingPlanApproval{
		AgentID: t.AgentID, RequestID: requestID, PlanFile: in.PlanFilePath,
		PlanContent: plan, Allowed: filterAllowedPrompts(in.AllowedPrompts),
		State: t.State, CreatedAt: time.Now().UTC(),
	}
	defaultPlanApprovalCoordinator.register(pending)
	if err := sendWithRetry(ctx, mailbox, teamLeadName, swarm.Message{From: agentName, Text: string(encoded), Timestamp: timestamp}); err != nil {
		defaultPlanApprovalCoordinator.unregister(requestID)
		return types.ToolResult{}, i18n.WrapInternalError(i18n.KeyAuxSwarmMailboxFailed, err)
	}
	DefaultInflightTable().Register(requestID, "plan_approval_request", agentName, []string{teamLeadName})
	result := exitPlanModeResult{
		Plan: stringPointer(plan), IsAgent: true, FilePath: in.PlanFilePath,
		AwaitingLeaderApproval: true, RequestID: requestID, Status: ExitPlanModeAwaiting,
	}
	return types.ToolResult{
		Content: exitPlanModeModelText(result), Data: result,
		Metadata: map[string]string{"exitPlanModeStatus": string(ExitPlanModeAwaiting)},
	}, nil
}
