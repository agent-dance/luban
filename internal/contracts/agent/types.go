package agent

import (
	"context"
	"time"

	"github.com/agent-dance/luban/types"
)

const (
	TaskTypeLocalBash  = "local_bash"
	TaskTypeLocalAgent = "local_agent"
)

type Input struct {
	Description     string `json:"description"`
	Prompt          string `json:"prompt"`
	SubagentType    string `json:"subagent_type,omitempty"`
	Model           string `json:"model,omitempty"`
	RunInBackground bool   `json:"run_in_background,omitempty"`
	Name            string `json:"name,omitempty"`
	TeamName        string `json:"team_name,omitempty"`
	Isolation       string `json:"isolation,omitempty"`
	CWD             string `json:"cwd,omitempty"`
	Color           string `json:"color,omitempty"`
}

type ProgressPhase string

const (
	ProgressStart      ProgressPhase = "start"
	ProgressMCPReady   ProgressPhase = "mcp_ready"
	ProgressRunning    ProgressPhase = "running"
	ProgressToolUse    ProgressPhase = "tool_use"
	ProgressAssistant  ProgressPhase = "assistant"
	ProgressCompleted  ProgressPhase = "completed"
	ProgressError      ProgressPhase = "error"
	ProgressAborted    ProgressPhase = "aborted"
	ProgressBackground ProgressPhase = "background"
)

type ProgressEvent struct {
	AgentID          string        `json:"agentId,omitempty"`
	AgentType        string        `json:"agentType,omitempty"`
	SessionID        string        `json:"sessionId,omitempty"`
	TurnID           string        `json:"turnId,omitempty"`
	WorkUnitID       string        `json:"workUnitId,omitempty"`
	ParentToolUseID  string        `json:"parentToolUseId,omitempty"`
	RunID            string        `json:"runId,omitempty"`
	Attempt          int           `json:"attempt,omitempty"`
	BatchID          string        `json:"batchId,omitempty"`
	SourceSequence   uint64        `json:"sourceSequence,omitempty"`
	DroppedCount     uint64        `json:"droppedCount,omitempty"`
	Phase            ProgressPhase `json:"phase"`
	MessageCount     int           `json:"messageCount"`
	LatestTool       string        `json:"latestTool,omitempty"`
	PartialText      string        `json:"partialText,omitempty"`
	ElapsedMs        int64         `json:"elapsedMs"`
	TokensUsed       int           `json:"tokensUsed"`
	Provider         string        `json:"provider,omitempty"`
	Model            string        `json:"model,omitempty"`
	Usage            *types.Usage  `json:"usage,omitempty"`
	LastRequestUsage *types.Usage  `json:"lastRequestUsage,omitempty"`
	Detail           string        `json:"detail,omitempty"`
	Timestamp        time.Time     `json:"timestamp"`
}

type RunOutcome string

const (
	RunOutcomeRunning     RunOutcome = "running"
	RunOutcomeSucceeded   RunOutcome = "succeeded"
	RunOutcomePartial     RunOutcome = "partial"
	RunOutcomeFailed      RunOutcome = "failed"
	RunOutcomeCancelled   RunOutcome = "cancelled"
	RunOutcomeTimedOut    RunOutcome = "timed_out"
	RunOutcomeInterrupted RunOutcome = "interrupted"
)

type RunRecord struct {
	RunID            string         `json:"run_id"`
	Attempt          int            `json:"attempt"`
	BatchID          string         `json:"batch_id,omitempty"`
	ParentRunID      string         `json:"parent_run_id,omitempty"`
	AgentPath        string         `json:"agent_path,omitempty"`
	Status           string         `json:"status"`
	Prompt           string         `json:"prompt,omitempty"`
	StartedAt        time.Time      `json:"started_at"`
	FinishedAt       *time.Time     `json:"finished_at,omitempty"`
	UpdatedAt        time.Time      `json:"updated_at"`
	TranscriptPath   string         `json:"transcript_path,omitempty"`
	DurationMs       *int64         `json:"duration_ms,omitempty"`
	TotalTokens      *int           `json:"total_tokens,omitempty"`
	Usage            *types.Usage   `json:"usage,omitempty"`
	Error            string         `json:"error,omitempty"`
	Result           string         `json:"result,omitempty"`
	Outcome          RunOutcome     `json:"outcome,omitempty"`
	TerminalReason   string         `json:"terminal_reason,omitempty"`
	ToolUseCount     int            `json:"tool_use_count,omitempty"`
	LatestToolUse    string         `json:"latest_tool_use,omitempty"`
	ArtifactRefs     []string       `json:"artifact_refs,omitempty"`
	VerificationRefs []string       `json:"verification_refs,omitempty"`
	LatestProgress   *ProgressEvent `json:"latest_progress,omitempty"`
}

type ApprovalRouting string

const (
	ApprovalAttached      ApprovalRouting = "attached"
	ApprovalFailClosed    ApprovalRouting = "fail_closed"
	ApprovalParentSession ApprovalRouting = "parent_session"
)

type SessionMetadata struct {
	AgentType              string                    `json:"agent_type,omitempty"`
	Provider               string                    `json:"provider,omitempty"`
	Model                  string                    `json:"model,omitempty"`
	CacheLineageID         string                    `json:"cache_lineage_id,omitempty"`
	CWD                    string                    `json:"cwd,omitempty"`
	Mode                   string                    `json:"mode,omitempty"`
	PermissionSnapshot     *types.ToolRuntimeContext `json:"permission_snapshot,omitempty"`
	ApprovalRouting        ApprovalRouting           `json:"approval_routing,omitempty"`
	PresentationSessionID  string                    `json:"presentation_session_id,omitempty"`
	Isolation              string                    `json:"isolation,omitempty"`
	WorktreeRepoRoot       string                    `json:"worktree_repo_root,omitempty"`
	WorktreePath           string                    `json:"worktree_path,omitempty"`
	WorktreeBranch         string                    `json:"worktree_branch,omitempty"`
	WorktreeHeadCommit     string                    `json:"worktree_head_commit,omitempty"`
	SkipInitialPrompt      bool                      `json:"skip_initial_prompt,omitempty"`
	TeamMember             bool                      `json:"team_member,omitempty"`
	SkillProjectGeneration uint64                    `json:"skill_project_generation,omitempty"`
}

type Hook struct {
	Type       string            `json:"type"`
	Kind       string            `json:"kind,omitempty"`
	Command    string            `json:"command,omitempty"`
	Timeout    int               `json:"timeout,omitempty"`
	URL        string            `json:"url,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	RetryCount int               `json:"retry_count,omitempty"`
	Matcher    string            `json:"matcher,omitempty"`
}

type HookInput struct {
	Type                 string         `json:"type"`
	HookEventName        string         `json:"hook_event_name,omitempty"`
	ToolName             string         `json:"tool_name,omitempty"`
	ToolUseID            string         `json:"tool_use_id,omitempty"`
	SessionID            string         `json:"session_id,omitempty"`
	ProjectRoot          string         `json:"project_root,omitempty"`
	TurnID               string         `json:"turn_id,omitempty"`
	WorkUnitID           string         `json:"work_unit_id,omitempty"`
	HookConfigID         string         `json:"hook_config_id,omitempty"`
	HookExecutionID      string         `json:"hook_execution_id,omitempty"`
	ToolInput            map[string]any `json:"tool_input,omitempty"`
	Result               string         `json:"result,omitempty"`
	Messages             []any          `json:"messages,omitempty"`
	UserInput            string         `json:"user_input,omitempty"`
	AgentID              string         `json:"agent_id,omitempty"`
	AgentType            string         `json:"agent_type,omitempty"`
	AgentTranscriptPath  string         `json:"agent_transcript_path,omitempty"`
	LastAssistantMessage string         `json:"last_assistant_message,omitempty"`
	StopHookActive       bool           `json:"stop_hook_active,omitempty"`
	TeammateName         string         `json:"teammate_name,omitempty"`
	TeamName             string         `json:"team_name,omitempty"`
	TaskID               string         `json:"task_id,omitempty"`
	TaskSubject          string         `json:"task_subject,omitempty"`
	TaskDescription      string         `json:"task_description,omitempty"`
	TaskOwner            string         `json:"task_owner,omitempty"`
	Owner                string         `json:"owner,omitempty"`
	Trigger              string         `json:"trigger,omitempty"`
	CustomInstructions   *string        `json:"custom_instructions,omitempty"`
	CompactSummary       string         `json:"compact_summary,omitempty"`
	Message              string         `json:"message,omitempty"`
	Title                string         `json:"title,omitempty"`
}

type HookOutput struct {
	SystemReminder        string
	Block                 bool
	ModifiedInput         map[string]any
	PreventContinuation   bool
	StopReason            string
	AdditionalContext     string
	AdditionalContexts    []string
	PermissionBehavior    string
	NewCustomInstructions string
	UserDisplayMessage    string
	ExitCode              int
	ExecutionError        string
	Stdout                string
	StdoutBytes           int64
	StdoutTruncated       bool
	Stderr                string
	StderrBytes           int64
	StderrTruncated       bool
}

type HookExecutionReceipt struct {
	HookType    string     `json:"hook_type"`
	ExecutionID string     `json:"execution_id"`
	ConfigID    string     `json:"config_id"`
	ConfigIndex int        `json:"config_index"`
	Hook        Hook       `json:"hook"`
	Input       HookInput  `json:"input"`
	Output      HookOutput `json:"output"`
	RecordedAt  time.Time  `json:"recorded_at"`
}

type RuntimeNotification struct {
	ID                  string                 `json:"id"`
	Kind                string                 `json:"kind"`
	TaskID              string                 `json:"task_id"`
	RunID               string                 `json:"run_id,omitempty"`
	Attempt             int                    `json:"attempt,omitempty"`
	SessionID           string                 `json:"session_id,omitempty"`
	ProjectRoot         string                 `json:"project_root,omitempty"`
	SessionProjectDir   string                 `json:"session_project_dir,omitempty"`
	Title               string                 `json:"title"`
	Message             string                 `json:"message"`
	Status              string                 `json:"status,omitempty"`
	ExitCode            *int                   `json:"exit_code,omitempty"`
	TranscriptPath      string                 `json:"transcriptPath,omitempty"`
	DurationMs          *int64                 `json:"durationMs,omitempty"`
	TotalTokens         *int                   `json:"totalTokens,omitempty"`
	Provider            string                 `json:"provider,omitempty"`
	Model               string                 `json:"model,omitempty"`
	Usage               *types.Usage           `json:"usage,omitempty"`
	CreatedAt           time.Time              `json:"created_at"`
	Attempts            int                    `json:"attempts,omitempty"`
	LastError           string                 `json:"last_error,omitempty"`
	SinkRequired        bool                   `json:"sink_required,omitempty"`
	ObserverRequired    bool                   `json:"observer_required,omitempty"`
	FollowUpRequired    bool                   `json:"follow_up_required,omitempty"`
	SinkDeliveredAt     *time.Time             `json:"sink_delivered_at,omitempty"`
	ObserverDeliveredAt *time.Time             `json:"observer_delivered_at,omitempty"`
	FollowUpDeliveredAt *time.Time             `json:"follow_up_delivered_at,omitempty"`
	DeliveredAt         *time.Time             `json:"delivered_at,omitempty"`
	HookExecutions      []HookExecutionReceipt `json:"hook_executions,omitempty"`
}

type TaskSnapshot struct {
	ID                     string
	Type                   string
	Status                 string
	Description            string
	Command                string
	Prompt                 string
	OutputPath             string
	ExitCode               *int
	Error                  string
	Result                 string
	OwnerSessionID         string
	OwnerSessionProjectDir string
	OwnerProjectRoot       string
	OwnerAgentID           string
	OwnerPID               int
	AgentAlias             string
	Detached               bool
	CurrentRunID           string
	Attempt                int
	BatchID                string
	ParentRunID            string
	AgentPath              string
	QueuedPrompts          int
	QueueReason            string
	Runs                   []RunRecord
	LatestProgress         *ProgressEvent
	TranscriptPath         string
	DurationMs             *int64
	TotalTokens            *int
	Usage                  *types.Usage
	Outcome                RunOutcome
	TerminalReason         string
	Timeout                time.Duration
	ArtifactRefs           []string
	VerificationRefs       []string
}

type ScopedTool interface {
	BindAgentScope(agentID, projectRoot string) types.Tool
}

// RuntimeScopedTool binds an immutable runtime provider without requiring the
// agent owner to know a domain tool's concrete type.
type RuntimeScopedTool interface {
	WithRuntime(types.ToolRuntimeContextProvider) types.Tool
}

// TeammateSpawnRequest is the agent-domain input to the collaboration owner.
// Team persistence, membership naming, and transaction serialization remain
// outside the agent runtime.
type TeammateSpawnRequest struct {
	SpawnID       string
	TeamName      string
	Input         Input
	ParentModel   string
	ParentSession string
}

// TeammateIdentity is assigned by the collaboration owner before the agent
// runtime creates the retained child session.
type TeammateIdentity struct {
	AgentID string
	Name    string
	Team    string
}

// TeammateLaunch is a prepared retained session. Start publishes the initial
// prompt only after collaboration state is durable; Rollback compensates a
// prepared session when the surrounding team transaction fails.
type TeammateLaunch struct {
	Result   types.ToolResult
	CWD      string
	Model    string
	Start    func() error
	Rollback func() error
}

type TeammateLauncher func(context.Context, TeammateIdentity) (TeammateLaunch, error)

// CollaborationSpawner is the neutral boundary between Agent execution and
// the team/send-message owner. Implementations own team storage and atomic
// reserve/activate/rollback behavior.
type CollaborationSpawner interface {
	CurrentTeamName() string
	TeamExists(teamName string) bool
	SpawnTeammate(context.Context, TeammateSpawnRequest, TeammateLauncher) (types.ToolResult, error)
}
