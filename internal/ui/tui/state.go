// Package tui provides the go-tui based terminal UI for the CLI.
// It replaces the TermRenderer with a full TUI application using
// github.com/grindlemire/go-tui for layout, reactive state, and rendering.
package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/contracts/interaction"
	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/internal/presentation"
	"github.com/agent-dance/luban/internal/runtime/goal"
	"github.com/agent-dance/luban/permissions"
	"github.com/agent-dance/luban/runtimeevent"
	"github.com/agent-dance/luban/types"
	"github.com/grindlemire/go-tui"
)

// streamDebounceInterval is the minimum time between state updates triggered
// by streaming tokens. Tokens arriving within this window are buffered and
// flushed as a single batch, reducing go-tui redraws from ~60-80/s (one per
// token) to ~20/s. This is the single most important performance knob for
// streaming: without it, every token triggers a full element tree rebuild.
const streamDebounceInterval = 50 * time.Millisecond

// --- Message Types ---

// MsgKind classifies message entries in the conversation.
type MsgKind int

const (
	MsgUser MsgKind = iota
	MsgAssistant
	MsgAssistantThinking
	MsgToolCall
	MsgToolResult
	MsgSystem
	MsgError
	MsgInfo
	MsgSuccess
	MsgWarning
	MsgSendUserMessage
)

// ImageAttachment represents an image pasted from the clipboard that is
// attached to a user message. ID is a sequential counter for display
// (e.g. "[Image #1]"). Base64 and MediaType are used to build API content
// blocks at submission time.
type ImageAttachment struct {
	ID          int    // 1-based display index
	Base64      string // base64-encoded image data
	MediaType   string // e.g. "image/png", "image/jpeg"
	Placeholder string // localized token wrapped with composer spacing at render time
}

func imageComposerPlaceholder(placeholder string) string {
	if placeholder == "" {
		return ""
	}
	return " " + placeholder + " "
}

// Message represents a single entry in the conversation log.
type Message struct {
	Kind               MsgKind
	Text               string
	WorkDuration       time.Duration  `json:"-"` // transient total query time on the final assistant reply
	ToolName           string         // for ToolCall / ToolResult
	Input              map[string]any // for ToolCall
	IsError            bool           // for ToolResult
	Collapsed          bool           // compact rendering state for transcript rows
	Timestamp          time.Time
	TurnCount          int
	Brief              *interaction.SendUserMessageOutput
	BriefMode          presentation.SendUserMessageRenderMode
	ObservationID      string
	ToolUseID          string
	SessionID          string
	TurnID             string
	WorkUnitID         string
	ActorID            string
	Outcome            ObservationOutcome
	Completeness       types.ToolResultCompleteness
	Disclosure         DisclosureState
	DetailRefs         []DetailRef
	PresentationHidden bool
	AggregateID        string
	AggregateSummary   string

	// Images attached to this message (typically MsgUser with pasted images).
	Images []ImageAttachment

	// Stream is the incremental Markdown renderer for this message.
	// Non-nil only for MsgAssistant messages that are being streamed.
	// Once streaming ends (Finalize is called), the renderer holds the
	// final cached output and is never mutated again.
	Stream *StreamRenderer `json:"-"`
}

// --- Interaction Modes ---

// InteractionMode controls how the TUI handles tool permissions.
// It maps to the user-facing mode indicator shown in the status bar
// and input prompt when Shift+Tab switches modes.
type InteractionMode int

const (
	// ModeAutoEdit allows all tool calls automatically (no prompts).
	ModeAutoEdit InteractionMode = iota
	// ModeAskEdit asks the user before each tool call.
	ModeAskEdit
	// ModePlanEdit enters plan mode — write tools are blocked until
	// the AI explicitly exits plan mode.
	ModePlanEdit
)

// String returns the human-readable name for the mode.
func (m InteractionMode) String() string {
	lang := i18n.DetectOrLoadLanguage()
	switch m {
	case ModeAutoEdit:
		return i18n.Text(lang, i18n.KeyModeAuto)
	case ModeAskEdit:
		return i18n.Text(lang, i18n.KeyModeAsk)
	case ModePlanEdit:
		return i18n.Text(lang, i18n.KeyModePlan)
	default:
		return i18n.Text(lang, i18n.KeyTUIOutcomeUnknown)
	}
}

// Code returns the stable persistence value. String is localized display copy
// and must never be serialized as a protocol enum.
func (m InteractionMode) Code() string {
	switch m {
	case ModeAutoEdit:
		return "auto"
	case ModePlanEdit:
		return "plan"
	default:
		return "ask"
	}
}

// Badge returns a short colored badge for the status bar.
func (m InteractionMode) Badge() string {
	switch m {
	case ModeAutoEdit:
		return "⚡"
	case ModeAskEdit:
		return "❓"
	case ModePlanEdit:
		return "📋"
	default:
		return "?"
	}
}

// PromptPrefix returns the input prompt prefix for this mode.
func (m InteractionMode) PromptPrefix() string {
	return m.PromptPrefixInLanguage(i18n.DetectOrLoadLanguage())
}

func (m InteractionMode) PromptPrefixInLanguage(lang i18n.Language) string {
	switch m {
	case ModeAutoEdit:
		return "> "
	case ModeAskEdit:
		return "> "
	case ModePlanEdit:
		return i18n.Text(lang, i18n.KeyRuntimePlanPrompt)
	default:
		return "> "
	}
}

// Next returns the next mode in the cycle: Auto → Ask → Plan → Auto.
func (m InteractionMode) Next() InteractionMode {
	switch m {
	case ModeAutoEdit:
		return ModeAskEdit
	case ModeAskEdit:
		return ModePlanEdit
	case ModePlanEdit:
		return ModeAutoEdit
	default:
		return ModeAutoEdit
	}
}

// --- App State ---

// AppState holds all reactive state for the TUI.
type AppState struct {
	Messages            *tui.State[[]Message]
	ObservationRevision *tui.State[uint64]
	Observations        *ObservationStore
	Details             DetailStore
	ActivityRevision    *tui.State[uint64]
	Activities          *ActivityStore
	ActivityFocus       *tui.State[string]
	ActivityViewOffset  *tui.State[int]
	LLMCall             *tui.State[*LLMCallStatus]
	LLMRequestMetrics   *tui.State[*LLMRequestMetricsStatus]
	CompactionProgress  *tui.State[*CompactionProgressStatus]
	activitySequence    uint64

	// Decision request state
	DecisionReq       *tui.State[*DecisionRequest]
	DecisionSelected  *tui.State[int]
	DecisionResp      chan permissions.PromptResponse
	DecisionHistory   *tui.State[[]DecisionRecord]
	DecisionReceipt   *tui.State[string]
	AskUserDraft      *tui.State[*AskUserPromptState]
	TranscriptShowAll *tui.State[bool]
	// ToolSegmentExpansion stores only explicit user overrides. Segment
	// membership and default expansion are derived from the transcript on every
	// render so restore and late result updates cannot leave stale groups.
	ToolSegmentExpansion *tui.State[map[string]bool]

	// Banner / session info (reactive, thread-safe)
	Provider                   *tui.State[string]
	Model                      *tui.State[string]
	SessionID                  *tui.State[string]
	SessionNS                  *tui.State[string]
	SessionEpoch               *tui.State[uint64]
	ContextGeneration          *tui.State[uint64]
	ContextGenerationPersisted *tui.State[bool]
	SessionUsageKnown          *tui.State[bool]
	SessionRoundUsageKnown     *tui.State[bool]
	InteractionRevision        *tui.State[uint64]
	ViewRevision               *tui.State[uint64]
	Tools                      *tui.State[[]string]
	Goal                       *tui.State[*GoalViewState]

	// Banner enrichment from ModelCatalog (set by TuiRenderer.Banner)
	ContextWindowK    *tui.State[string]  // e.g. "200K", "1M", "" if unknown
	ModelCostIn       *tui.State[float64] // cost per 1M input tokens (0 = unknown)
	ModelCostOut      *tui.State[float64] // cost per 1M output tokens (0 = unknown)
	ModelCostCurrency *tui.State[string]  // ISO 4217 currency for model costs
	ModelCanSeeImages *tui.State[bool]    // true when the active model accepts image inputs
	// Selected reasoning effort for the active model (e.g. "low", "medium", "high")
	ReasoningEffort *tui.State[string]

	// Provider connection status (shown in status bar)
	ProvStatus *tui.State[ProviderStatus]

	// Session usage tracking shown in the status bar. Total and compaction
	// baseline states are authoritative; latest/completed-round states remain
	// only for durable compatibility and diagnostics.
	CumulativeCost                    *tui.State[float64]
	SessionCostKnown                  *tui.State[bool]
	SessionInputTokens                *tui.State[int]
	SessionOutputTokens               *tui.State[int]
	SessionCacheReadTokens            *tui.State[int]
	SessionCacheCreateTokens          *tui.State[int]
	SessionWebSearchRequests          *tui.State[int]
	SessionTotalInputTokens           *tui.State[int]
	SessionTotalOutputTokens          *tui.State[int]
	SessionTotalCacheReadTokens       *tui.State[int]
	SessionTotalCacheCreateTokens     *tui.State[int]
	SessionHasCompacted               *tui.State[bool]
	SessionCompactionBaselineKnown    *tui.State[bool]
	SessionCompactionCount            *tui.State[int]
	SessionProgressiveProjectionCount *tui.State[int]
	SessionProgressiveProjectedTools  *tui.State[int]
	SessionProgressiveTokensSaved     *tui.State[int]
	SessionProgressiveSavingsUSD      *tui.State[float64]
	ProgressivePendingTools           *tui.State[int]
	ProgressivePendingTokens          *tui.State[int]
	SessionCompletedRoundInputTokens  *tui.State[int]
	SessionCompletedRoundOutputTokens *tui.State[int]
	SessionInputTokensAtCompact       *tui.State[int]
	SessionCacheReadAtCompact         *tui.State[int]

	// Context bar
	UsedTokens         *tui.State[int]
	MaxTokens          *tui.State[int]
	ContextMeasurement *tui.State[presentation.ContextMeasurement]

	// Pending images pasted via Ctrl+V / Alt+V, waiting to be sent with the
	// next user message. Reactive so the input area can show "[Image #N]" tags.
	PendingImages        *tui.State[[]ImageAttachment]
	PendingImageSelected *tui.State[int] // -1 means the input is active and no image is selected

	// Session picker modal state (used by /resume in TUI mode).
	SessionPicker *tui.State[*SessionPickerState]
	// Fork picker modal state (used by /fork in TUI mode).
	ForkPicker *tui.State[*ForkPickerState]

	// Model picker modal state (used by Meta+P in TUI mode).
	ModelPicker *tui.State[*ModelPickerState]
	// SkillsMenu is the isolated direct exact-/skills checklist overlay.
	SkillsMenu *tui.State[*SkillsMenuState]

	// Interaction mode (Shift+Tab cycles: Auto → Ask → Plan)
	Mode *tui.State[InteractionMode]

	// Language (Shift+L cycles through supported languages)
	Language *tui.State[i18n.Language]

	// ExpandedView mirrors the TS app-state surface used by task tools. A
	// revision increment gives the interactive view a stable refresh signal even
	// when it was already expanded.
	ExpandedView     *tui.State[string]
	TaskListRevision *tui.State[uint64]
	TaskViewItems    *tui.State[[]TaskViewItem]

	// imageCounter is the next ImageAttachment.ID to assign. Protected by mu.
	imageCounter int

	// Query cancel support: when non-nil, Ctrl+C cancels the active query
	// instead of exiting. Protected by mu.
	QueryCancelFn    func()
	QueuedInputTexts *tui.State[[]string]
	queryGeneration  uint64
	queryInFlight    bool

	// TermWidth caches the terminal width in cells (set by RootComponent.Render).
	// Used by StreamRenderer for width-aware table column shrinking.
	TermWidth int

	// stopCh is closed when the app is shutting down. Goroutines blocked on
	// channel receives (e.g. PermissionRequest) should select on this to
	// unblock and exit cleanly.
	stopCh chan struct{}

	// clearEpoch is incremented on every ClearMessages() call.
	// Writers check this to detect that a clear happened mid-stream
	// and avoid creating ghost messages after a /clear.
	clearEpoch uint64

	// --- Debounce state for streaming tokens ---
	// streamBuf accumulates tokens between debounce flushes.
	// Protected by mu.
	streamBuf []string
	// streamTimer fires after streamDebounceInterval to flush buffered tokens.
	// nil when no flush is pending.
	streamTimer *time.Timer

	mu                      sync.Mutex
	activeInteraction       SessionInteraction
	checkpointSequence      uint64
	checkpointWriterID      string
	searchMu                sync.Mutex
	search                  *TranscriptSearchController
	batch                   func(func())
	disclosureReturn        map[string]SessionInteraction
	compactionBoundaries    sync.Map
	progressiveProjections  sync.Map
	usageProjectionMu       sync.Mutex
	usageProjectionRevision uint64
}

func (s *AppState) bindBatch(fn func(func())) { s.batch = fn }

func (s *AppState) runBatch(fn func()) {
	if s.batch != nil {
		s.batch(fn)
		return
	}
	fn()
}

type SessionUsage struct {
	Known                      bool
	RoundUsageKnown            bool
	InputTokens                int
	OutputTokens               int
	CacheReadTokens            int
	CacheCreateTokens          int
	HasCompacted               bool
	CompactionBaselineKnown    bool
	CompactionCount            int
	ProgressiveProjectionCount int
	ProgressiveProjectedTools  int
	ProgressiveTokensSaved     int
	ProgressiveSavingsUSD      float64
	CompletedRoundInputTokens  int
	CompletedRoundOutputTokens int
	InputTokensAtCompact       int
	CacheReadAtCompact         int
	LastInputTokens            int
	LastOutputTokens           int
	LastCacheReadTokens        int
	LastCacheCreateTokens      int
	WebSearchRequests          int
	CumulativeCost             float64
	UsedTokens                 int
	MaxTokens                  int
}

type SessionInteraction struct {
	FocusedObservationID string
	ScrollAnchorID       string
	ScrollOffset         int
	InputDraft           string
	InputCursor          int
	InputCursorSet       bool
	SlashSelected        int
	SlashSelectedSet     bool
	SlashDismissedInput  string
}

type TaskViewItem struct {
	ID        string
	Subject   string
	Status    string
	Owner     string
	BlockedBy []string
}

// GoalViewState is the durable, presentation-only projection of a session
// goal, including the Agent-authored acceptance contract.
type GoalViewState struct {
	Status    string
	Objective string
	Revision  int
	Criteria  []GoalCriterionViewState
}

type GoalCriterionViewState struct {
	ID     string
	Text   string
	Status string
	Reason string
}

type LLMCallPhase string

const (
	LLMCallWorking  LLMCallPhase = "working"
	LLMCallRetrying LLMCallPhase = "retrying"
	LLMCallProblem  LLMCallPhase = "problem"
)

// LLMCallStage is the last model-work boundary directly observed by the
// runtime. Stages are intentionally coarse and never imply a percentage.
type LLMCallStage string

const (
	LLMStagePreparing         LLMCallStage = "preparing"
	LLMStageWaitingFirstToken LLMCallStage = "waiting_first_token"
	LLMStageThinking          LLMCallStage = "thinking"
	LLMStageToolInput         LLMCallStage = "tool_input"
	LLMStageToolExecution     LLMCallStage = "tool_execution"
	LLMStageWaitingAfterTools LLMCallStage = "waiting_after_tools"
	LLMStageResponse          LLMCallStage = "response"
)

// LLMCallStatus is transient presentation state for the active model execution.
// It is intentionally excluded from session checkpoints.
type LLMCallStatus struct {
	RequestID          string
	Phase              LLMCallPhase
	Stage              LLMCallStage
	StageDetail        string
	ToolInputBytes     int
	StageStartedAt     time.Time
	Attempt            int
	MaxRetries         int
	RetryCount         int
	RetryDelay         time.Duration
	RetryKind          string
	RequestDuration    time.Duration
	HasRequestDuration bool
	FirstTokenDuration time.Duration
	HasFirstToken      bool
	TotalDuration      time.Duration
	UpdatedAt          time.Time
	WorkStartedAt      time.Time
	Error              string
}

// LLMRequestMetricsStatus is the most recently established provider request's
// presentation-safe latency and throughput summary. It intentionally outlives
// the active LLMCallStatus so the last API measurements remain visible after
// query settlement, but is reset when the session surface is replaced.
type LLMRequestMetricsStatus struct {
	RequestID                    string
	ConnectionDuration           time.Duration
	FirstTokenDuration           time.Duration
	HasFirstToken                bool
	AverageOutputTokensPerSecond float64
	HasAverageOutputTokenRate    bool
}

// NewAppState creates a new AppState with initial values.
func NewAppState() *AppState {
	details := NewMemoryDetailStore()
	state := &AppState{
		Messages:                          tui.NewState([]Message(nil)),
		ObservationRevision:               tui.NewState(uint64(0)),
		Observations:                      NewObservationStore(details),
		Details:                           details,
		ActivityRevision:                  tui.NewState(uint64(0)),
		Activities:                        NewActivityStore(ActivityScope{}),
		ActivityFocus:                     tui.NewState(""),
		ActivityViewOffset:                tui.NewState(0),
		LLMCall:                           tui.NewState[*LLMCallStatus](nil),
		LLMRequestMetrics:                 tui.NewState[*LLMRequestMetricsStatus](nil),
		CompactionProgress:                tui.NewState[*CompactionProgressStatus](nil),
		DecisionReq:                       tui.NewState[*DecisionRequest](nil),
		DecisionSelected:                  tui.NewState(0),
		DecisionResp:                      make(chan permissions.PromptResponse, 1),
		DecisionHistory:                   tui.NewState([]DecisionRecord(nil)),
		DecisionReceipt:                   tui.NewState(""),
		AskUserDraft:                      tui.NewState[*AskUserPromptState](nil),
		TranscriptShowAll:                 tui.NewState(false),
		ToolSegmentExpansion:              tui.NewState(map[string]bool(nil)),
		Provider:                          tui.NewState(""),
		Model:                             tui.NewState(""),
		SessionID:                         tui.NewState(""),
		SessionNS:                         tui.NewState(""),
		SessionEpoch:                      tui.NewState(uint64(0)),
		ContextGeneration:                 tui.NewState(uint64(0)),
		ContextGenerationPersisted:        tui.NewState(false),
		SessionUsageKnown:                 tui.NewState(false),
		SessionRoundUsageKnown:            tui.NewState(true),
		InteractionRevision:               tui.NewState(uint64(0)),
		ViewRevision:                      tui.NewState(uint64(0)),
		Tools:                             tui.NewState([]string(nil)),
		Goal:                              tui.NewState[*GoalViewState](nil),
		ContextWindowK:                    tui.NewState(""),
		ModelCostIn:                       tui.NewState(0.0),
		ModelCostOut:                      tui.NewState(0.0),
		ModelCostCurrency:                 tui.NewState("USD"),
		ModelCanSeeImages:                 tui.NewState(false),
		ReasoningEffort:                   tui.NewState(""),
		ProvStatus:                        tui.NewState(StatusUnknown),
		CumulativeCost:                    tui.NewState(0.0),
		SessionCostKnown:                  tui.NewState(true),
		SessionInputTokens:                tui.NewState(0),
		SessionOutputTokens:               tui.NewState(0),
		SessionCacheReadTokens:            tui.NewState(0),
		SessionCacheCreateTokens:          tui.NewState(0),
		SessionWebSearchRequests:          tui.NewState(0),
		SessionTotalInputTokens:           tui.NewState(0),
		SessionTotalOutputTokens:          tui.NewState(0),
		SessionTotalCacheReadTokens:       tui.NewState(0),
		SessionTotalCacheCreateTokens:     tui.NewState(0),
		SessionHasCompacted:               tui.NewState(false),
		SessionCompactionBaselineKnown:    tui.NewState(false),
		SessionCompactionCount:            tui.NewState(0),
		SessionProgressiveProjectionCount: tui.NewState(0),
		SessionProgressiveProjectedTools:  tui.NewState(0),
		SessionProgressiveTokensSaved:     tui.NewState(0),
		SessionProgressiveSavingsUSD:      tui.NewState(0.0),
		ProgressivePendingTools:           tui.NewState(0),
		ProgressivePendingTokens:          tui.NewState(0),
		SessionCompletedRoundInputTokens:  tui.NewState(0),
		SessionCompletedRoundOutputTokens: tui.NewState(0),
		SessionInputTokensAtCompact:       tui.NewState(0),
		SessionCacheReadAtCompact:         tui.NewState(0),
		UsedTokens:                        tui.NewState(0),
		MaxTokens:                         tui.NewState(0),
		ContextMeasurement:                tui.NewState(presentation.ContextMeasurementUnknown),
		PendingImages:                     tui.NewState([]ImageAttachment(nil)),
		PendingImageSelected:              tui.NewState(-1),
		SessionPicker:                     tui.NewState[*SessionPickerState](nil),
		ForkPicker:                        tui.NewState[*ForkPickerState](nil),
		ModelPicker:                       tui.NewState[*ModelPickerState](nil),
		SkillsMenu:                        tui.NewState[*SkillsMenuState](nil),
		QueuedInputTexts:                  tui.NewState([]string(nil)),
		Mode:                              tui.NewState(ModeAutoEdit),
		Language:                          tui.NewState(i18n.DetectOrLoadLanguage()),
		ExpandedView:                      tui.NewState(""),
		TaskListRevision:                  tui.NewState(uint64(0)),
		TaskViewItems:                     tui.NewState([]TaskViewItem(nil)),
		stopCh:                            make(chan struct{}),
		disclosureReturn:                  make(map[string]SessionInteraction),
	}
	state.checkpointWriterID = newSessionViewWriterID()
	return state
}

// DurableSessionView is the single versioned owner of every settled,
// session-owned rendering input outside the transcript projection itself.
// Checkpoint persistence and atomic session publication embed this exact type;
// adding a durable visible surface therefore cannot update one path without
// producing a compile-time schema change in the other.
//
// Process-local execution controls, pickers, active permission prompts,
// provider connectivity, language/theme, catalog data, and viewport size are
// render context or transient runtime state and deliberately do not belong
// here.
type DurableSessionView struct {
	Provider             string                        `json:"provider,omitempty"`
	Model                string                        `json:"model,omitempty"`
	Usage                SessionUsage                  `json:"usage"`
	SessionCostKnown     bool                          `json:"session_cost_known"`
	Goal                 *GoalViewState                `json:"goal,omitempty"`
	Interaction          SessionInteraction            `json:"interaction"`
	DisclosureReturns    map[string]SessionInteraction `json:"disclosure_returns,omitempty"`
	PermissionMode       InteractionMode               `json:"permission_mode"`
	Decisions            []DecisionRecord              `json:"decisions,omitempty"`
	Activities           []Activity                    `json:"activities,omitempty"`
	ActivityFocus        string                        `json:"activity_focus,omitempty"`
	ActivityViewOffset   int                           `json:"activity_view_offset,omitempty"`
	ToolSegmentExpansion map[string]bool               `json:"tool_segment_expansion,omitempty"`
	ExpandedView         string                        `json:"expanded_view,omitempty"`
	TaskViewItems        []TaskViewItem                `json:"task_view_items,omitempty"`
	DecisionReceipt      string                        `json:"decision_receipt,omitempty"`
	PendingImages        []ImageAttachment             `json:"pending_images,omitempty"`
	PendingImageSelected int                           `json:"pending_image_selected"`
}

// SessionSnapshot is the prepared presentation half of an atomic session
// transition. Fallible loading and validation happen before this value is
// published to AppState.
type SessionSnapshot struct {
	Identity     SessionIdentity
	Projection   SessionProjection
	ViewSequence uint64
	// ContextGeneration is transient runtime authority, not user interaction
	// state. It is refreshed from the authoritative session manifest whenever a
	// snapshot is prepared.
	ContextGeneration          uint64
	ContextGenerationPersisted bool
	DurableSessionView
}

// ApplySessionSnapshot publishes a fully prepared transcript, detail store,
// observation index, and epoch as one AppState critical section.
func (s *AppState) ApplySessionSnapshot(snapshot SessionSnapshot) error {
	var err error
	s.runBatch(func() { err = s.applySessionSnapshot(snapshot) })
	return err
}

func (s *AppState) applySessionSnapshot(snapshot SessionSnapshot) error {
	if snapshot.Identity.SessionID == "" {
		return i18n.NewError(i18n.KeyTUISessionSnapshotEmptySessionID)
	}
	for _, image := range snapshot.PendingImages {
		placeholder := strings.TrimSpace(image.Placeholder)
		if image.ID <= 0 || strings.TrimSpace(image.Base64) == "" || strings.TrimSpace(image.MediaType) == "" || placeholder == "" ||
			!strings.Contains(snapshot.Interaction.InputDraft, imageComposerPlaceholder(placeholder)) {
			return i18n.NewError(i18n.KeyTUISessionViewInvalidCheckpoint)
		}
	}
	details := snapshot.Projection.Details
	if details == nil {
		details = NewMemoryDetailStore()
	}
	observations := NewObservationStore(details)
	observations.mu.Lock()
	for _, observation := range snapshot.Projection.Observations {
		if observation.ID == "" {
			observations.mu.Unlock()
			return i18n.NewError(i18n.KeyTUISessionSnapshotObservationEmptyID)
		}
		if _, exists := observations.byID[observation.ID]; exists {
			observations.mu.Unlock()
			return i18n.NewError(i18n.KeyTUISessionSnapshotDuplicateObservation, observation.ID)
		}
		normalized := cloneObservation(observation)
		isToolObservation := normalized.ToolName != "" || normalized.ToolUseID != "" || len(normalized.ResultRefs) > 0 || len(normalized.EnvelopeRefs) > 0
		if isToolObservation && (normalized.Presentation.Summary == "" || normalized.Decision.Surface == "") {
			observations.mu.Unlock()
			return i18n.NewError(i18n.KeyTUISessionViewInvalidCheckpoint)
		}
		observations.appendLocked(normalized)
		if observation.ToolUseID != "" && observation.Outcome != OutcomeOrphan && observation.Outcome != OutcomeConflict {
			observations.callCounts[toolObservationID(observation.SessionID, observation.ToolUseID)] = 1
		}
	}
	if snapshot.Projection.Aggregates == nil {
		observations.rebuildAggregatesLocked()
	} else if !observations.restoreAggregatesLocked(snapshot.Projection.Aggregates) {
		observations.mu.Unlock()
		return i18n.NewError(i18n.KeyTUISessionViewInvalidCheckpoint)
	}
	observations.mu.Unlock()
	messages := make([]Message, len(snapshot.Projection.Messages))
	for i := range snapshot.Projection.Messages {
		messages[i] = clonePresentationMessage(snapshot.Projection.Messages[i])
		if messages[i].ObservationID != "" {
			if observation, ok := observations.Get(messages[i].ObservationID); ok {
				if observation.ToolName != "" || observation.ToolUseID != "" {
					messages[i].Disclosure = observation.Disclosure
					messages[i].Outcome = observation.Outcome
					messages[i].SessionID = observation.SessionID
					messages[i].TurnID = observation.TurnID
					messages[i].ActorID = observation.ActorID
					messages[i].WorkUnitID = observation.WorkUnitID
				}
			}
		}
	}
	activities := NewActivityStore(ActivityScope{SessionID: snapshot.Identity.SessionID, Epoch: snapshot.Identity.Epoch})
	activitySequence, restoreErr := activities.Restore(snapshot.Activities)
	if restoreErr != nil {
		return i18n.WrapError(i18n.KeyTUISessionSnapshotRestoreActivities, restoreErr)
	}
	activityFocus := snapshot.ActivityFocus
	if activityFocus != "" {
		if _, exists := activities.Get(activityFocus); !exists {
			activityFocus = ""
		}
	}
	activityOffset := snapshot.ActivityViewOffset
	if activityOffset < 0 {
		activityOffset = 0
	}
	if total := len(activities.Snapshot().Activities); total == 0 {
		activityOffset = 0
	} else if activityOffset >= total {
		activityOffset = total - 1
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancelStreamTimerLocked()
	s.streamBuf = s.streamBuf[:0]
	s.Messages.Set(messages)
	s.Details = details
	s.Observations = observations
	s.Activities = activities
	s.activitySequence = activitySequence
	s.ActivityFocus.Set(activityFocus)
	s.ActivityViewOffset.Set(activityOffset)
	s.LLMCall.Set(nil)
	s.LLMRequestMetrics.Set(nil)
	s.CompactionProgress.Set(nil)
	s.SessionNS.Set(snapshot.Identity.Namespace)
	s.SessionID.Set(snapshot.Identity.SessionID)
	s.SessionEpoch.Set(snapshot.Identity.Epoch)
	s.ContextGeneration.Set(snapshot.ContextGeneration)
	s.ContextGenerationPersisted.Set(snapshot.ContextGenerationPersisted)
	s.Provider.Set(snapshot.Provider)
	s.Model.Set(snapshot.Model)
	if snapshot.Goal == nil {
		s.Goal.Set(nil)
	} else {
		s.Goal.Set(normalizeGoalView(snapshot.Goal))
	}
	s.DecisionReq.Set(nil)
	s.DecisionSelected.Set(0)
	s.AskUserDraft.Set(nil)
	s.SessionPicker.Set(nil)
	s.ForkPicker.Set(nil)
	s.ModelPicker.Set(nil)
	s.SkillsMenu.Set(nil)
	decisions := make([]DecisionRecord, len(snapshot.Decisions))
	copy(decisions, snapshot.Decisions)
	s.DecisionHistory.Set(decisions)
	s.DecisionReceipt.Set(snapshot.DecisionReceipt)
	// Evidence/show-all is an explicit, process-local disclosure choice and is
	// never restored from a resumed or forked durable checkpoint.
	s.TranscriptShowAll.Set(false)
	s.ToolSegmentExpansion.Set(cloneBoolMap(snapshot.ToolSegmentExpansion))
	s.ExpandedView.Set(snapshot.ExpandedView)
	taskItems := append([]TaskViewItem(nil), snapshot.TaskViewItems...)
	for index := range taskItems {
		taskItems[index].BlockedBy = append([]string(nil), taskItems[index].BlockedBy...)
	}
	s.TaskViewItems.Set(taskItems)
	pendingImages := append([]ImageAttachment(nil), snapshot.PendingImages...)
	interaction := snapshot.Interaction
	s.PendingImages.Set(pendingImages)
	pendingSelection := snapshot.PendingImageSelected
	if len(pendingImages) == 0 || pendingSelection < 0 || pendingSelection >= len(pendingImages) {
		pendingSelection = -1
	}
	s.PendingImageSelected.Set(pendingSelection)
	s.imageCounter = 0
	for _, image := range pendingImages {
		if image.ID > s.imageCounter {
			s.imageCounter = image.ID
		}
	}
	s.searchMu.Lock()
	s.search = nil
	s.searchMu.Unlock()
	s.disclosureReturn = make(map[string]SessionInteraction, len(snapshot.DisclosureReturns))
	for id, restore := range snapshot.DisclosureReturns {
		s.disclosureReturn[id] = restore
	}
	s.compactionBoundaries.Clear()
	s.progressiveProjections.Clear()
	s.usageProjectionMu.Lock()
	s.usageProjectionRevision = 0
	s.SessionUsageKnown.Set(snapshot.Usage.Known)
	s.SessionRoundUsageKnown.Set(snapshot.Usage.RoundUsageKnown)
	s.SessionInputTokens.Set(snapshot.Usage.LastInputTokens)
	s.SessionOutputTokens.Set(snapshot.Usage.LastOutputTokens)
	s.SessionCacheReadTokens.Set(snapshot.Usage.LastCacheReadTokens)
	s.SessionCacheCreateTokens.Set(snapshot.Usage.LastCacheCreateTokens)
	s.SessionWebSearchRequests.Set(snapshot.Usage.WebSearchRequests)
	s.SessionTotalInputTokens.Set(snapshot.Usage.InputTokens)
	s.SessionTotalOutputTokens.Set(snapshot.Usage.OutputTokens)
	s.SessionTotalCacheReadTokens.Set(snapshot.Usage.CacheReadTokens)
	s.SessionTotalCacheCreateTokens.Set(snapshot.Usage.CacheCreateTokens)
	s.SessionHasCompacted.Set(snapshot.Usage.HasCompacted)
	s.SessionCompactionBaselineKnown.Set(snapshot.Usage.CompactionBaselineKnown)
	s.SessionCompactionCount.Set(snapshot.Usage.CompactionCount)
	s.SessionProgressiveProjectionCount.Set(snapshot.Usage.ProgressiveProjectionCount)
	s.SessionProgressiveProjectedTools.Set(snapshot.Usage.ProgressiveProjectedTools)
	s.SessionProgressiveTokensSaved.Set(snapshot.Usage.ProgressiveTokensSaved)
	s.SessionProgressiveSavingsUSD.Set(snapshot.Usage.ProgressiveSavingsUSD)
	s.ProgressivePendingTools.Set(0)
	s.ProgressivePendingTokens.Set(0)
	s.SessionCompletedRoundInputTokens.Set(snapshot.Usage.CompletedRoundInputTokens)
	s.SessionCompletedRoundOutputTokens.Set(snapshot.Usage.CompletedRoundOutputTokens)
	s.SessionInputTokensAtCompact.Set(snapshot.Usage.InputTokensAtCompact)
	s.SessionCacheReadAtCompact.Set(snapshot.Usage.CacheReadAtCompact)
	s.CumulativeCost.Set(snapshot.Usage.CumulativeCost)
	s.SessionCostKnown.Set(snapshot.SessionCostKnown)
	s.usageProjectionMu.Unlock()
	s.UsedTokens.Set(snapshot.Usage.UsedTokens)
	s.MaxTokens.Set(snapshot.Usage.MaxTokens)
	s.ContextMeasurement.Set(presentation.ContextMeasurementUnknown)
	s.activeInteraction = interaction
	s.checkpointSequence = snapshot.ViewSequence
	s.InteractionRevision.Set(s.InteractionRevision.Get() + 1)
	s.Mode.Set(snapshot.PermissionMode)
	s.ObservationRevision.Set(s.ObservationRevision.Get() + 1)
	s.ActivityRevision.Set(s.ActivityRevision.Get() + 1)
	s.bumpViewRevision()
	s.clearEpoch++
	return nil
}

// SetGoalStatus publishes the active session's compact goal projection. Goal
// states that cannot steer work are intentionally absent from the status bar.
func (s *AppState) SetGoalStatus(status, objective string) {
	s.SetGoalView(&GoalViewState{Status: status, Objective: objective})
}

func (s *AppState) SetGoalView(current *GoalViewState) {
	s.Goal.Set(normalizeGoalView(current))
}

// GoalViewFromGoal converts persisted domain state without exposing storage
// or evaluation internals to the renderer.
func GoalViewFromGoal(current *goal.Goal) *GoalViewState {
	if current == nil {
		return nil
	}
	normalized := goal.Normalize(*current)
	view := &GoalViewState{Status: string(normalized.Status), Objective: normalized.Objective, Revision: normalized.Revision}
	results := make(map[string]goal.AcceptanceCriterionEvaluation)
	if normalized.LastAcceptanceEvaluation != nil && normalized.LastAcceptanceEvaluation.Revision == normalized.Revision {
		for _, result := range normalized.LastAcceptanceEvaluation.Criteria {
			results[strings.ToUpper(result.CriterionID)] = result
		}
	}
	for _, criterion := range normalized.AcceptanceCriteria {
		item := GoalCriterionViewState{ID: criterion.ID, Text: criterion.Text, Status: "pending"}
		if result, ok := results[strings.ToUpper(criterion.ID)]; ok {
			item.Status, item.Reason = "unmet", result.Reason
			if result.Met {
				item.Status = "met"
			}
		}
		view.Criteria = append(view.Criteria, item)
	}
	return normalizeGoalView(view)
}

func normalizeGoalView(current *GoalViewState) *GoalViewState {
	if current == nil {
		return nil
	}
	view := *current
	view.Status = strings.ToLower(strings.TrimSpace(view.Status))
	view.Objective = strings.TrimSpace(view.Objective)
	if view.Objective == "" || view.Status == "" || view.Status == "cleared" {
		return nil
	}
	view.Criteria = append([]GoalCriterionViewState(nil), view.Criteria...)
	for index := range view.Criteria {
		view.Criteria[index].ID = strings.TrimSpace(view.Criteria[index].ID)
		view.Criteria[index].Text = strings.TrimSpace(view.Criteria[index].Text)
		view.Criteria[index].Reason = strings.TrimSpace(view.Criteria[index].Reason)
		switch strings.ToLower(strings.TrimSpace(view.Criteria[index].Status)) {
		case "met":
			view.Criteria[index].Status = "met"
		case "unmet":
			view.Criteria[index].Status = "unmet"
		default:
			view.Criteria[index].Status = "pending"
		}
	}
	return &view
}

type PreparedTranscriptSearch struct {
	controller *TranscriptSearchController
	epoch      uint64
	match      TranscriptSearchMatch
	count      int
	ok         bool
}

// PrepareTranscriptSearch performs the potentially expensive evidence scan
// without mutating reactive UI state. PublishTranscriptSearch must run on the
// event loop after preparation succeeds.
func (s *AppState) PrepareTranscriptSearch(query string) (PreparedTranscriptSearch, error) {
	interaction := s.ActiveSessionInteraction()
	s.mu.Lock()
	observations, details := s.Observations, s.Details
	messages := append([]Message(nil), s.Messages.Get()...)
	epoch := s.SessionEpoch.Get()
	s.mu.Unlock()
	controller := NewTranscriptSearchController(observations, details, messages)
	if err := controller.Open(query, TranscriptViewState{
		FocusTarget:  interaction.FocusedObservationID,
		ScrollAnchor: TranscriptScrollAnchor{ObservationID: interaction.ScrollAnchorID, RowOffset: interaction.ScrollOffset},
	}); err != nil {
		return PreparedTranscriptSearch{}, err
	}
	match, ok := controller.Current()
	return PreparedTranscriptSearch{controller: controller, epoch: epoch, match: match, count: len(controller.Matches()), ok: ok}, nil
}

func (s *AppState) PublishTranscriptSearch(prepared PreparedTranscriptSearch) (TranscriptSearchMatch, int, bool, error) {
	if prepared.controller == nil {
		return TranscriptSearchMatch{}, 0, false, i18n.NewError(i18n.KeyTUITranscriptSearchNotPrepared)
	}
	if !s.AdmitEpoch(prepared.epoch) {
		return TranscriptSearchMatch{}, 0, false, i18n.NewError(i18n.KeyTUITranscriptSearchSessionChanged)
	}
	var publishErr error
	s.runBatch(func() {
		s.closeTranscriptSearch()
		s.searchMu.Lock()
		s.search = prepared.controller
		s.searchMu.Unlock()
		if prepared.ok {
			publishErr = s.focusSearchMatchBatched(prepared.match)
		}
	})
	if publishErr != nil {
		return TranscriptSearchMatch{}, 0, false, publishErr
	}
	return prepared.match, prepared.count, prepared.ok, nil

}

func (s *AppState) OpenTranscriptSearch(query string) (TranscriptSearchMatch, int, bool, error) {
	prepared, err := s.PrepareTranscriptSearch(query)
	if err != nil {
		return TranscriptSearchMatch{}, 0, false, err
	}
	return s.PublishTranscriptSearch(prepared)
}

func (s *AppState) MoveTranscriptSearch(delta int) (TranscriptSearchMatch, int, bool, error) {
	s.searchMu.Lock()
	controller := s.search
	s.searchMu.Unlock()
	if controller == nil {
		return TranscriptSearchMatch{}, 0, false, i18n.NewError(i18n.KeyTUITranscriptSearchNotOpen)
	}
	var match TranscriptSearchMatch
	var ok bool
	if delta < 0 {
		match, ok = controller.Previous()
	} else {
		match, ok = controller.Next()
	}
	if ok {
		if err := s.focusSearchMatch(match); err != nil {
			return TranscriptSearchMatch{}, 0, false, err
		}
	}
	return match, len(controller.Matches()), ok, nil
}

func (s *AppState) focusSearchMatch(match TranscriptSearchMatch) error {
	var err error
	s.runBatch(func() { err = s.focusSearchMatchBatched(match) })
	return err
}

func (s *AppState) focusSearchMatchBatched(match TranscriptSearchMatch) error {
	if _, ok := s.Observations.Get(match.ObservationID); ok {
		if err := s.setObservationDisclosure(match.ObservationID, DisclosureEvidence, false); err != nil {
			return err
		}
	}
	s.mu.Lock()
	s.activeInteraction.FocusedObservationID = match.ObservationID
	s.activeInteraction.ScrollAnchorID = match.ObservationID
	s.activeInteraction.ScrollOffset = 0
	s.mu.Unlock()
	s.InteractionRevision.Set(s.InteractionRevision.Get() + 1)
	return nil
}

func (s *AppState) CloseTranscriptSearch() TranscriptViewState {
	var view TranscriptViewState
	s.runBatch(func() { view = s.closeTranscriptSearch() })
	return view
}

func (s *AppState) closeTranscriptSearch() TranscriptViewState {
	s.searchMu.Lock()
	controller := s.search
	s.search = nil
	s.searchMu.Unlock()
	if controller == nil {
		return TranscriptViewState{}
	}
	view := controller.Close()
	s.mu.Lock()
	messages := append([]Message(nil), s.Messages.Get()...)
	for i := range messages {
		if messages[i].ObservationID == "" {
			continue
		}
		if observation, ok := s.Observations.Get(messages[i].ObservationID); ok {
			messages[i].Disclosure = observation.Disclosure
		}
	}
	s.Messages.Set(messages)
	s.activeInteraction.FocusedObservationID = view.FocusTarget
	s.activeInteraction.ScrollAnchorID = view.ScrollAnchor.ObservationID
	s.activeInteraction.ScrollOffset = view.ScrollAnchor.RowOffset
	s.mu.Unlock()
	s.InteractionRevision.Set(s.InteractionRevision.Get() + 1)
	s.ObservationRevision.Set(s.ObservationRevision.Get() + 1)
	return view
}

// AdmitEpoch reports whether an asynchronous event belongs to the currently
// visible presentation generation.
func (s *AppState) AdmitEpoch(epoch uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return epoch == s.SessionEpoch.Get()
}

// AdmitRuntimeGeneration fences asynchronous work by both presentation epoch
// and durable model-context generation. Persisted and unpersisted events are
// separate states; generation zero is never treated as a wildcard.
func (s *AppState) AdmitRuntimeGeneration(epoch, generation uint64, persisted bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if epoch != s.SessionEpoch.Get() {
		return false
	}
	current := s.ContextGeneration.Get()
	currentPersisted := s.ContextGenerationPersisted.Get() || current != 0
	if currentPersisted != persisted {
		return false
	}
	if !currentPersisted {
		return generation == 0
	}
	return generation != 0 && generation == current
}

// PublishContextGeneration advances the visible generation only for the exact
// owning presentation epoch. A stale refresher cannot move a new session.
func (s *AppState) PublishContextGeneration(sessionID string, epoch, generation uint64) bool {
	return s.PublishContextGenerationState(sessionID, epoch, generation, true)
}

// PublishContextGenerationState publishes an explicit durable/unpersisted
// authority state for the exact owning presentation epoch.
func (s *AppState) PublishContextGenerationState(sessionID string, epoch, generation uint64, persisted bool) bool {
	if (persisted && generation == 0) || (!persisted && generation != 0) {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.SessionID.Get() != sessionID || s.SessionEpoch.Get() != epoch {
		return false
	}
	s.ContextGeneration.Set(generation)
	s.ContextGenerationPersisted.Set(persisted)
	return true
}

// ClearView clears only the current visible projection. Model context and the
// retained observation/detail audit remain untouched.
func (s *AppState) ClearView() {
	s.runBatch(s.clearView)
}

func (s *AppState) clearView() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancelStreamTimerLocked()
	s.streamBuf = s.streamBuf[:0]
	s.Messages.Set(nil)
	s.DecisionReceipt.Set("")
	s.TranscriptShowAll.Set(false)
	s.ToolSegmentExpansion.Set(nil)
	s.activeInteraction.FocusedObservationID = ""
	s.activeInteraction.ScrollAnchorID = ""
	s.activeInteraction.ScrollOffset = 0
	s.InteractionRevision.Set(s.InteractionRevision.Get() + 1)
	s.clearEpoch++
}

func (s *AppState) ActiveSessionUsage() SessionUsage {
	s.usageProjectionMu.Lock()
	defer s.usageProjectionMu.Unlock()
	return SessionUsage{
		Known: s.SessionUsageKnown.Get(), RoundUsageKnown: s.SessionRoundUsageKnown.Get(),
		InputTokens: s.SessionTotalInputTokens.Get(), OutputTokens: s.SessionTotalOutputTokens.Get(),
		CacheReadTokens: s.SessionTotalCacheReadTokens.Get(), CacheCreateTokens: s.SessionTotalCacheCreateTokens.Get(),
		HasCompacted: s.SessionHasCompacted.Get(), CompactionCount: s.SessionCompactionCount.Get(),
		ProgressiveProjectionCount: s.SessionProgressiveProjectionCount.Get(), ProgressiveProjectedTools: s.SessionProgressiveProjectedTools.Get(),
		ProgressiveTokensSaved: s.SessionProgressiveTokensSaved.Get(), ProgressiveSavingsUSD: s.SessionProgressiveSavingsUSD.Get(),
		CompactionBaselineKnown:   s.SessionCompactionBaselineKnown.Get(),
		CompletedRoundInputTokens: s.SessionCompletedRoundInputTokens.Get(), CompletedRoundOutputTokens: s.SessionCompletedRoundOutputTokens.Get(),
		InputTokensAtCompact: s.SessionInputTokensAtCompact.Get(), CacheReadAtCompact: s.SessionCacheReadAtCompact.Get(),
		LastInputTokens: s.SessionInputTokens.Get(), LastOutputTokens: s.SessionOutputTokens.Get(),
		LastCacheReadTokens: s.SessionCacheReadTokens.Get(), LastCacheCreateTokens: s.SessionCacheCreateTokens.Get(),
		WebSearchRequests: s.SessionWebSearchRequests.Get(), CumulativeCost: s.CumulativeCost.Get(),
		UsedTokens: s.UsedTokens.Get(), MaxTokens: s.MaxTokens.Get(),
	}
}

// DurableSessionViewSnapshot is the sole AppState -> durable view projection.
// Persistence, resume, and fork must consume this value rather than reading
// individual visible fields independently. The surrounding checkpoint capture
// fences it with SessionLifecycleRevision to reject a torn multi-state read.
func (s *AppState) DurableSessionViewSnapshot() DurableSessionView {
	if s == nil {
		return DurableSessionView{}
	}
	view := DurableSessionView{
		Provider: s.Provider.Get(), Model: s.Model.Get(),
		Usage: s.ActiveSessionUsage(), SessionCostKnown: s.SessionCostKnown.Get(), Goal: cloneGoalViewState(s.Goal.Get()),
		Interaction: interactionWithPendingImagePlaceholders(s.ActiveSessionInteraction(), s.PendingImages.Get()), DisclosureReturns: s.ActiveDisclosureReturns(),
		PermissionMode: s.Mode.Get(), Decisions: append([]DecisionRecord(nil), s.DecisionHistory.Get()...),
		Activities: s.ActivityRunHistory(), ActivityFocus: s.ActivityFocus.Get(), ActivityViewOffset: s.ActivityViewOffset.Get(),
		ToolSegmentExpansion: cloneBoolMap(s.ToolSegmentExpansion.Get()),
		ExpandedView:         s.ExpandedView.Get(), TaskViewItems: append([]TaskViewItem(nil), s.TaskViewItems.Get()...),
		DecisionReceipt: s.DecisionReceipt.Get(), PendingImages: append([]ImageAttachment(nil), s.PendingImages.Get()...),
		PendingImageSelected: s.PendingImageSelected.Get(),
	}
	return cloneDurableSessionView(view)
}

func (s *AppState) ObservationSnapshot() []Observation {
	s.mu.Lock()
	observations := s.Observations
	s.mu.Unlock()
	if observations == nil {
		return nil
	}
	return observations.Snapshot()
}

func (s *AppState) ObservationAggregateSnapshot() []ObservationAggregate {
	s.mu.Lock()
	observations := s.Observations
	s.mu.Unlock()
	if observations == nil {
		return []ObservationAggregate{}
	}
	groups := observations.AggregateSnapshot()
	if groups == nil {
		return []ObservationAggregate{}
	}
	return groups
}

func (s *AppState) GetObservation(id string) (Observation, bool) {
	s.mu.Lock()
	observations := s.Observations
	s.mu.Unlock()
	if observations == nil {
		return Observation{}, false
	}
	return observations.Get(id)
}

func (s *AppState) PinnedObservationSnapshot() []Observation {
	s.mu.Lock()
	observations := s.Observations
	s.mu.Unlock()
	if observations == nil {
		return nil
	}
	return observations.PinnedSnapshot()
}

func (s *AppState) ReadDetail(ref DetailRef) ([]byte, error) {
	s.mu.Lock()
	details := s.Details
	s.mu.Unlock()
	return details.Get(ref)
}

func (s *AppState) GetActivity(id string) (Activity, bool) {
	s.mu.Lock()
	activities := s.Activities
	s.mu.Unlock()
	if activities == nil {
		return Activity{}, false
	}
	return activities.Get(id)
}

func (s *AppState) ActivitySnapshot() ActivitySnapshot {
	s.mu.Lock()
	activities := s.Activities
	s.mu.Unlock()
	if activities == nil {
		return ActivitySnapshot{}
	}
	return activities.Snapshot()
}

func (s *AppState) ActivityRunHistory() []Activity {
	s.mu.Lock()
	activities := s.Activities
	s.mu.Unlock()
	if activities == nil {
		return nil
	}
	return activities.RunHistory()
}

func (s *AppState) ActivityRunCount() int {
	s.mu.Lock()
	activities := s.Activities
	s.mu.Unlock()
	if activities == nil {
		return 0
	}
	return activities.RunCount()
}

func (s *AppState) AgentActivityByCorrelation(actorID, workUnitID string) (Activity, bool) {
	s.mu.Lock()
	activities := s.Activities
	s.mu.Unlock()
	if activities == nil {
		return Activity{}, false
	}
	return activities.AgentByCorrelation(actorID, workUnitID)
}

func (s *AppState) TranscriptResources() (*ObservationStore, DetailStore, []Message) {
	s.mu.Lock()
	observations, details := s.Observations, s.Details
	messages := append([]Message(nil), s.Messages.Get()...)
	s.mu.Unlock()
	return observations, details, messages
}

func (s *AppState) ObservationEvidence(id string) ([]byte, error) {
	lang := s.Language.Get()
	observation, ok := s.GetObservation(id)
	if !ok {
		return nil, fmt.Errorf("%s", i18n.Format(lang, i18n.KeyEvidenceObservationNotFound, id))
	}
	if strings.EqualFold(strings.TrimSpace(observation.ToolName), "Agent") {
		return []byte(strings.Join(observation.Presentation.DetailLines, "\n")), nil
	}
	var output strings.Builder
	fmt.Fprint(&output, i18n.Format(lang, i18n.KeyEvidenceObservationHeader, observation.ID, observation.SessionID, observation.TurnID, observation.WorkUnitID, observation.ActorID, observation.ToolName, observation.Outcome))
	if len(observation.ToolInput) > 0 {
		encoded, err := json.MarshalIndent(observation.ToolInput, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("%s: %w", i18n.Text(lang, i18n.KeyEvidenceEncodeInputError), err)
		}
		fmt.Fprint(&output, i18n.Format(lang, i18n.KeyEvidenceInput, encoded))
	}
	for index, ref := range observation.ResultRefs {
		evidence, err := s.ReadDetail(ref)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", i18n.Format(lang, i18n.KeyEvidenceReadResultError, index+1), err)
		}
		fmt.Fprint(&output, i18n.Format(lang, i18n.KeyEvidenceResultBoundary, index+1, evidence, index+1))
	}
	for index, ref := range observation.EnvelopeRefs {
		evidence, err := s.ReadDetail(ref)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", i18n.Format(lang, i18n.KeyEvidenceReadStructuredError, index+1), err)
		}
		fmt.Fprint(&output, i18n.Format(lang, i18n.KeyEvidenceStructured, index+1, evidence))
	}
	return []byte(output.String()), nil
}

func (s *AppState) ActiveSessionInteraction() SessionInteraction {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activeInteraction
}

func interactionWithPendingImagePlaceholders(interaction SessionInteraction, images []ImageAttachment) SessionInteraction {
	for _, image := range images {
		composerPlaceholder := imageComposerPlaceholder(image.Placeholder)
		if composerPlaceholder == "" || strings.Contains(interaction.InputDraft, composerPlaceholder) {
			continue
		}
		if strings.Contains(interaction.InputDraft, image.Placeholder) {
			interaction.InputDraft = strings.Replace(interaction.InputDraft, image.Placeholder, composerPlaceholder, 1)
		} else {
			interaction.InputDraft += composerPlaceholder
		}
		interaction.InputCursor = utf8.RuneCountInString(interaction.InputDraft)
		interaction.InputCursorSet = true
	}
	return interaction
}

func (s *AppState) ActiveDisclosureReturns() map[string]SessionInteraction {
	s.mu.Lock()
	defer s.mu.Unlock()
	returns := make(map[string]SessionInteraction, len(s.disclosureReturn))
	for id, restore := range s.disclosureReturn {
		returns[id] = restore
	}
	return returns
}

func (s *AppState) SetInteractionDraft(draft string) {
	s.SetInteractionEditor(draft, utf8.RuneCountInString(draft))
}

// SetInteractionEditor atomically publishes the durable composer text and
// cursor. Cursor positions are rune offsets so wide and combining characters
// do not turn a restored draft into a different editor view.
func (s *AppState) SetInteractionEditor(draft string, cursor int) {
	if cursor < 0 {
		cursor = 0
	}
	if maximum := utf8.RuneCountInString(draft); cursor > maximum {
		cursor = maximum
	}
	s.mu.Lock()
	if s.activeInteraction.InputDraft == draft && s.activeInteraction.InputCursor == cursor && s.activeInteraction.InputCursorSet {
		s.mu.Unlock()
		return
	}
	s.activeInteraction.InputDraft = draft
	s.activeInteraction.InputCursor = cursor
	s.activeInteraction.InputCursorSet = true
	s.mu.Unlock()
	s.bumpViewRevision()
}

func (s *AppState) SetInteractionCursor(cursor int) {
	if cursor < 0 {
		cursor = 0
	}
	s.mu.Lock()
	if maximum := utf8.RuneCountInString(s.activeInteraction.InputDraft); cursor > maximum {
		cursor = maximum
	}
	if s.activeInteraction.InputCursor == cursor && s.activeInteraction.InputCursorSet {
		s.mu.Unlock()
		return
	}
	s.activeInteraction.InputCursor = cursor
	s.activeInteraction.InputCursorSet = true
	s.mu.Unlock()
	s.bumpViewRevision()
}

func (s *AppState) SetInteractionSlash(selected int, selectedSet bool, dismissedInput string) {
	if selected < 0 {
		selected = 0
	}
	s.mu.Lock()
	if s.activeInteraction.SlashSelected == selected && s.activeInteraction.SlashSelectedSet == selectedSet && s.activeInteraction.SlashDismissedInput == dismissedInput {
		s.mu.Unlock()
		return
	}
	s.activeInteraction.SlashSelected = selected
	s.activeInteraction.SlashSelectedSet = selectedSet
	s.activeInteraction.SlashDismissedInput = dismissedInput
	s.mu.Unlock()
	s.bumpViewRevision()
}

func (s *AppState) SetInteractionScroll(offset int) {
	if offset < 0 {
		offset = 0
	}
	s.mu.Lock()
	if s.activeInteraction.ScrollOffset == offset {
		s.mu.Unlock()
		return
	}
	s.activeInteraction.ScrollOffset = offset
	s.mu.Unlock()
	s.bumpViewRevision()
}

func (s *AppState) SetInteractionAnchor(anchor string) {
	s.mu.Lock()
	if s.activeInteraction.ScrollAnchorID == anchor {
		s.mu.Unlock()
		return
	}
	s.activeInteraction.ScrollAnchorID = anchor
	s.mu.Unlock()
	s.bumpViewRevision()
}

// SessionLifecycleRevision is a comparable change token for durable view
// checkpointing. Any state that can alter a resumed or forked frame must flow
// through one of these revisions instead of inventing another persistence
// watcher.
type SessionLifecycleRevision struct {
	Activity           uint64
	Observation        uint64
	Interaction        uint64
	View               uint64
	Tasks              uint64
	Decisions          int
	Messages           int
	Mode               InteractionMode
	ShowAll            bool
	Expanded           string
	Receipt            string
	GoalStatus         string
	GoalObject         string
	GoalRevision       int
	GoalCriteria       string
	Provider           string
	Model              string
	Language           string
	ActivityFocus      string
	ActivityViewOffset int
	Usage              SessionUsage
	SessionCostKnown   bool
	PendingCount       int
	PendingSelected    int
}

func (s *AppState) SessionLifecycleRevision() SessionLifecycleRevision {
	if s == nil {
		return SessionLifecycleRevision{}
	}
	revision := SessionLifecycleRevision{
		Activity: s.ActivityRevision.Get(), Observation: s.ObservationRevision.Get(),
		Interaction: s.InteractionRevision.Get(), View: s.ViewRevision.Get(),
		Tasks: s.TaskListRevision.Get(), Decisions: len(s.DecisionHistory.Get()),
		Messages: len(s.Messages.Get()), Mode: s.Mode.Get(), ShowAll: s.TranscriptShowAll.Get(),
		Expanded: s.ExpandedView.Get(), Receipt: s.DecisionReceipt.Get(),
		Provider: s.Provider.Get(), Model: s.Model.Get(), Language: s.Language.Get().Code(),
		ActivityFocus: s.ActivityFocus.Get(), ActivityViewOffset: s.ActivityViewOffset.Get(),
		Usage: s.ActiveSessionUsage(), SessionCostKnown: s.SessionCostKnown.Get(),
		PendingCount: len(s.PendingImages.Get()), PendingSelected: s.PendingImageSelected.Get(),
	}
	if goal := s.Goal.Get(); goal != nil {
		revision.GoalStatus, revision.GoalObject = goal.Status, goal.Objective
		revision.GoalRevision = goal.Revision
		if encoded, err := json.Marshal(goal.Criteria); err == nil {
			revision.GoalCriteria = string(encoded)
		}
	}
	return revision
}

func (s *AppState) bumpViewRevision() {
	if s != nil && s.ViewRevision != nil {
		s.ViewRevision.Set(s.ViewRevision.Get() + 1)
	}
}

func (s *AppState) SetTranscriptShowAll(show bool) {
	if s == nil || s.TranscriptShowAll.Get() == show {
		return
	}
	s.TranscriptShowAll.Set(show)
	s.bumpViewRevision()
}

func (s *AppState) SetExpandedView(view string) {
	if s == nil || s.ExpandedView.Get() == view {
		return
	}
	s.ExpandedView.Set(view)
	s.bumpViewRevision()
}

func clonePresentationMessage(message Message) Message {
	message.Input = cloneStringAnyMap(message.Input)
	message.Completeness = message.Completeness.Clone()
	message.DetailRefs = append([]DetailRef(nil), message.DetailRefs...)
	message.Images = append([]ImageAttachment(nil), message.Images...)
	if message.Brief != nil {
		brief := *message.Brief
		brief.Attachments = append([]interaction.SendUserMessageAttachment(nil), message.Brief.Attachments...)
		message.Brief = &brief
	}
	return message
}

// ApplyToolCall normalizes a structured call and appends exactly one visible
// transcript anchor for the resulting observation. Malformed unmatched calls
// are retained as orphan evidence even when a diagnostic error is returned.
func (s *AppState) ApplyToolCall(ctx ToolEventContext, call types.ToolUseBlock) error {
	return s.applyToolCall(ctx, call, true)
}

func (s *AppState) ApplyHiddenToolCall(ctx ToolEventContext, call types.ToolUseBlock) error {
	return s.applyToolCall(ctx, call, false)
}

func (s *AppState) applyToolCall(ctx ToolEventContext, call types.ToolUseBlock, visible bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx.Language = s.Language.Get()
	ctx.LanguageSet = true
	before := len(s.Observations.Snapshot())
	err := s.Observations.ApplyToolCall(ctx, call)
	observation, ok := s.appliedObservation(before, ctx.SessionID, call.ID)
	if !ok {
		return err
	}

	activities := s.Activities
	if visible {
		s.appendMessageLocked(messageFromObservation(observation, MsgToolCall))
	}
	s.ObservationRevision.Set(s.ObservationRevision.Get() + 1)
	if activities != nil && call.ID != "" {
		_ = s.applyActivityLocked(ActivityEvent{
			ID: "tool:" + call.ID, SessionID: ctx.SessionID, Epoch: s.SessionEpoch.Get(), TurnID: ctx.TurnID,
			WorkUnitID: ctx.WorkUnitID, Actor: ActivityActor{ID: ctx.ActorID, Type: ctx.ActorType},
			Kind: activityKindForTool(call.Name), Name: call.Name, Phase: activityPhaseForTool(call.Name, call.Input, ctx.ActorType),
			Lifecycle: ActivityLifecycleRunning, Outcome: OutcomeRunning,
			Control: ActivityControl{JumpTarget: observation.ID},
		})
	}
	return err
}

// ApplyToolResult updates the call's existing transcript anchor by stable ID.
// Results without one unique call remain independent orphan/conflict rows.
func (s *AppState) ApplyToolResult(ctx ToolEventContext, result types.ToolResultBlock) error {
	return s.applyToolResult(ctx, result, true)
}

func (s *AppState) ApplyHiddenToolResult(ctx ToolEventContext, result types.ToolResultBlock) (Observation, error) {
	err := s.applyToolResult(ctx, result, false)
	observation, _ := s.Observations.Get(toolObservationID(ctx.SessionID, result.ToolUseID))
	return observation, err
}

func (s *AppState) applyToolResult(ctx ToolEventContext, result types.ToolResultBlock, visible bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx.Language = s.Language.Get()
	ctx.LanguageSet = true
	before := len(s.Observations.Snapshot())
	err := s.Observations.ApplyToolResult(ctx, result)
	observation, ok := s.appliedObservation(before, ctx.SessionID, result.ToolUseID)
	if !ok {
		return err
	}

	activities := s.Activities
	old := s.Messages.Get()
	updated := false
	if observation.Outcome != OutcomeOrphan && observation.Outcome != OutcomeConflict {
		next := make([]Message, len(old))
		copy(next, old)
		for i := range next {
			candidate, exists := s.Observations.Get(next[i].ObservationID)
			if !exists {
				continue
			}
			if candidate.ID != observation.ID && (observation.Aggregation.GroupID == "" || candidate.Aggregation.GroupID != observation.Aggregation.GroupID) {
				continue
			}
			replacement := messageFromObservation(candidate, next[i].Kind)
			replacement.Timestamp = next[i].Timestamp
			next[i] = replacement
			if candidate.ID == observation.ID {
				updated = true
			}
		}
		if updated {
			s.Messages.Set(next)
		}
	}
	if !updated && visible {
		s.appendMessageLocked(messageFromObservation(observation, MsgToolResult))
	}
	s.ObservationRevision.Set(s.ObservationRevision.Get() + 1)
	if activities != nil && result.ToolUseID != "" && observation.Outcome != OutcomeUnknown {
		activityOutcome := observation.Outcome
		if observationIsNormalPagination(observation.Outcome, observation.Presentation.Completeness) {
			activityOutcome = OutcomeSucceeded
		}
		_ = s.applyActivityLocked(ActivityEvent{
			ID: "tool:" + result.ToolUseID, SessionID: ctx.SessionID, Epoch: s.SessionEpoch.Get(), TurnID: ctx.TurnID,
			WorkUnitID: ctx.WorkUnitID, Actor: ActivityActor{ID: ctx.ActorID, Type: ctx.ActorType},
			Kind: activityKindForTool(observation.ToolName), Name: observation.ToolName, Phase: activityPhaseForTool(observation.ToolName, observation.ToolInput, ctx.ActorType),
			Lifecycle: activityLifecycleForOutcome(activityOutcome), Outcome: activityOutcome,
			Control: ActivityControl{JumpTarget: observation.ID, DetailRefs: append(append([]DetailRef(nil), observation.ResultRefs...), observation.EnvelopeRefs...)},
		})
	}
	return err
}

func activityKindForTool(name string) ActivityKind {
	lower := strings.ToLower(name)
	if strings.Contains(lower, "agent") || strings.Contains(lower, "task") {
		return ActivityAgent
	}
	if strings.Contains(lower, "mcp") || strings.Contains(lower, "server_tool") {
		return ActivityMCP
	}
	return ActivityTool
}

func activityPhaseForTool(name string, input map[string]any, actorType string) ActivityPhase {
	identity := strings.ToLower(strings.TrimSpace(name + " " + actorType))
	for _, marker := range []string{"verify", "verifier", "test", "tester", "lint", "check", "build"} {
		if strings.Contains(identity, marker) {
			return ActivityPhaseVerifying
		}
	}
	if command, ok := input["command"].(string); ok {
		command = strings.ToLower(command)
		for _, marker := range []string{"go test", "go vet", "staticcheck", "golangci-lint", "pytest", "npm test", "pnpm test", "yarn test", "cargo test", "make test", "build"} {
			if strings.Contains(command, marker) {
				return ActivityPhaseVerifying
			}
		}
	}
	return ActivityPhaseExecuting
}

// ActivityPhaseForTool exposes the deterministic production classifier to
// non-tool activity adapters such as background task projection.
func ActivityPhaseForTool(name string, input map[string]any, actorType string) ActivityPhase {
	return activityPhaseForTool(name, input, actorType)
}

func (s *AppState) ExpandActivityView() ActivitySnapshot {
	var snapshot ActivitySnapshot
	s.runBatch(func() {
		s.SetExpandedView("activities")
		s.ActivityRevision.Set(s.ActivityRevision.Get() + 1)
		if s.Activities == nil {
			return
		}
		snapshot = s.Activities.Snapshot()
		s.ActivityViewOffset.Set(0)
		if len(snapshot.Activities) > 0 {
			s.ActivityFocus.Set(snapshot.Activities[0].ID)
		}
	})
	return snapshot
}

func (s *AppState) AcknowledgeActivity(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Activities == nil || !s.Activities.AcknowledgeLatest(id) {
		return i18n.NewError(i18n.KeyTUIActivityNotFound, id)
	}
	s.ActivityRevision.Set(s.ActivityRevision.Get() + 1)
	return nil
}

func (s *AppState) appliedObservation(previousLen int, sessionID, toolUseID string) (Observation, bool) {
	snapshot := s.Observations.Snapshot()
	if len(snapshot) > previousLen {
		return snapshot[len(snapshot)-1], true
	}
	if toolUseID == "" {
		return Observation{}, false
	}
	return s.Observations.Get(toolObservationID(sessionID, toolUseID))
}

func (s *AppState) RevealObservation(id string, level DisclosureLevel) error {
	var err error
	s.runBatch(func() { err = s.setObservationDisclosure(id, level, level != DisclosureSummary) })
	return err
}

func (s *AppState) setObservationDisclosure(id string, level DisclosureLevel, pinned bool) error {
	observation, ok := s.Observations.Get(id)
	if !ok {
		return i18n.NewError(i18n.KeyTUIStateObservationNotFound, id)
	}
	if strings.EqualFold(strings.TrimSpace(observation.ToolName), "Agent") {
		level = DisclosureSummary
		pinned = false
	}
	disclosure := observation.Disclosure
	disclosure.Level = level
	disclosure.UserPinned = pinned
	if err := s.Observations.SetDisclosure(id, disclosure); err != nil {
		return err
	}
	updatedObservation, _ := s.Observations.Get(id)
	previousGroupID := observation.Aggregation.GroupID
	updatedGroupID := updatedObservation.Aggregation.GroupID
	s.mu.Lock()
	defer s.mu.Unlock()
	if pinned && level != DisclosureSummary {
		if _, exists := s.disclosureReturn[id]; !exists {
			s.disclosureReturn[id] = s.activeInteraction
		}
	}
	messages := append([]Message(nil), s.Messages.Get()...)
	for i := range messages {
		candidate, exists := s.Observations.Get(messages[i].ObservationID)
		if !exists {
			continue
		}
		if candidate.ID == id || (previousGroupID != "" && candidate.Aggregation.GroupID == previousGroupID) || (updatedGroupID != "" && candidate.Aggregation.GroupID == updatedGroupID) {
			replacement := messageFromObservation(candidate, messages[i].Kind)
			replacement.Timestamp = messages[i].Timestamp
			messages[i] = replacement
		}
	}
	s.Messages.Set(messages)
	if level == DisclosureSummary {
		if restore, ok := s.disclosureReturn[id]; ok {
			currentDraft := s.activeInteraction.InputDraft
			currentCursor := s.activeInteraction.InputCursor
			currentCursorSet := s.activeInteraction.InputCursorSet
			currentSlashSelected := s.activeInteraction.SlashSelected
			currentSlashSelectedSet := s.activeInteraction.SlashSelectedSet
			currentSlashDismissedInput := s.activeInteraction.SlashDismissedInput
			s.activeInteraction = restore
			s.activeInteraction.InputDraft = currentDraft
			s.activeInteraction.InputCursor = currentCursor
			s.activeInteraction.InputCursorSet = currentCursorSet
			s.activeInteraction.SlashSelected = currentSlashSelected
			s.activeInteraction.SlashSelectedSet = currentSlashSelectedSet
			s.activeInteraction.SlashDismissedInput = currentSlashDismissedInput
			delete(s.disclosureReturn, id)
		}
	} else {
		s.activeInteraction.FocusedObservationID = id
		s.activeInteraction.ScrollAnchorID = id
		s.activeInteraction.ScrollOffset = 0
	}
	s.ObservationRevision.Set(s.ObservationRevision.Get() + 1)
	s.InteractionRevision.Set(s.InteractionRevision.Get() + 1)
	return nil
}

func (s *AppState) CycleObservationDisclosure(id string) (DisclosureLevel, error) {
	observation, ok := s.Observations.Get(id)
	if !ok {
		return DisclosureSummary, i18n.NewError(i18n.KeyTUIStateObservationNotFound, id)
	}
	if strings.EqualFold(strings.TrimSpace(observation.ToolName), "Agent") {
		return DisclosureSummary, s.RevealObservation(id, DisclosureSummary)
	}
	next := DisclosureSummary
	switch observation.Disclosure.Level {
	case DisclosureSummary:
		next = DisclosureDetail
	case DisclosureDetail:
		next = DisclosureEvidence
	case DisclosureEvidence:
		next = DisclosureSummary
	}
	return next, s.RevealObservation(id, next)
}

func (s *AppState) ApplyActivity(event ActivityEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.applyActivityLocked(event)
}

func (s *AppState) FreezeObservationAggregates(sessionID, turnID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.SessionID.Get() != sessionID || strings.TrimSpace(turnID) == "" || s.Observations == nil {
		return 0
	}
	frozen := s.Observations.FreezeAggregates(sessionID, turnID)
	if frozen > 0 {
		s.ObservationRevision.Set(s.ObservationRevision.Get() + 1)
	}
	return frozen
}

func (s *AppState) applyActivityLocked(event ActivityEvent) error {
	activities := s.Activities
	if activities == nil {
		return i18n.NewError(i18n.KeyTUIActivityStoreUnavailable)
	}
	s.activitySequence++
	event.Sequence = s.activitySequence
	if err := activities.Apply(event); err != nil {
		return err
	}
	s.ActivityRevision.Set(s.ActivityRevision.Get() + 1)
	if strings.HasPrefix(event.ID, "tool:") || event.Kind == ActivityDecision || strings.HasPrefix(event.ID, "decision:") {
		s.syncToolPresentationLifecyclesLocked(activities.Snapshot())
	}
	return nil
}

// syncToolPresentationLifecyclesLocked applies the decision-aware activity
// snapshot to transcript tool rows. In particular, a pending permission must
// replace the running glyph with the existing blocked presentation, and a
// resolved permission must restore the underlying running or terminal state.
func (s *AppState) syncToolPresentationLifecyclesLocked(snapshot ActivitySnapshot) {
	if s.Observations == nil {
		return
	}
	changed := false
	for _, activity := range snapshot.Activities {
		if !strings.HasPrefix(activity.ID, "tool:") || activity.SessionID == "" {
			continue
		}
		observationID := toolObservationID(activity.SessionID, strings.TrimPrefix(activity.ID, "tool:"))
		if s.Observations.UpdatePresentationLifecycle(observationID, presentationLifecycleForActivity(activity.Lifecycle)) {
			changed = true
		}
	}
	if changed {
		s.ObservationRevision.Set(s.ObservationRevision.Get() + 1)
	}
}

func presentationLifecycleForActivity(lifecycle ActivityLifecycle) PresentationLifecycleState {
	switch lifecycle {
	case ActivityLifecycleSpawning:
		return PresentationLifecycleSpawning
	case ActivityLifecycleQueued:
		return PresentationLifecycleQueued
	case ActivityLifecycleWaiting:
		return PresentationLifecycleWaiting
	case ActivityLifecycleBlocked:
		return PresentationLifecycleBlocked
	case ActivityLifecycleCompleted:
		return PresentationLifecycleCompleted
	case ActivityLifecycleFailed:
		return PresentationLifecycleFailed
	case ActivityLifecycleCancelled:
		return PresentationLifecycleCancelled
	default:
		return PresentationLifecycleRunning
	}
}

func (s *AppState) RetainDetailForEpoch(sessionID string, epoch uint64, key string, data []byte) (DetailRef, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.SessionID.Get() != sessionID || s.SessionEpoch.Get() != epoch {
		return DetailRef{}, ErrActivityScopeMismatch
	}
	return s.Details.Put(key, data)
}

// AttachToolObservationDetailForEpoch links retained background evidence to
// the original tool observation without appending another transcript row.
func (s *AppState) AttachToolObservationDetailForEpoch(sessionID string, epoch uint64, toolUseID string, ref DetailRef) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.SessionID.Get() != sessionID || s.SessionEpoch.Get() != epoch {
		return "", ErrActivityScopeMismatch
	}
	observationID := toolObservationID(sessionID, strings.TrimSpace(toolUseID))
	if err := s.Observations.AttachResultRef(observationID, ref); err != nil {
		return "", err
	}
	s.ObservationRevision.Set(s.ObservationRevision.Get() + 1)
	return observationID, nil
}

// UpdateToolObservationAgentResultForEpoch publishes the typed final preview
// into the original Agent observation without appending a transcript row.
func (s *AppState) UpdateToolObservationAgentResultForEpoch(sessionID string, epoch uint64, toolUseID, result string, outcomes ...ObservationOutcome) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.SessionID.Get() != sessionID || s.SessionEpoch.Get() != epoch {
		return "", ErrActivityScopeMismatch
	}
	observationID := toolObservationID(sessionID, strings.TrimSpace(toolUseID))
	if err := s.Observations.UpdateAgentResultPreview(observationID, s.Language.Get(), result, outcomes...); err != nil {
		return "", err
	}
	s.ObservationRevision.Set(s.ObservationRevision.Get() + 1)
	return observationID, nil
}

func (s *AppState) RecordActivityEvidenceForEpoch(sessionID string, epoch uint64, activityID, name, workUnitID, actorID, key string, outcome ObservationOutcome, data []byte) (DetailRef, string, error) {
	return s.recordActivityEvidence(ToolEventContext{SessionID: sessionID, WorkUnitID: workUnitID, ActorID: actorID}, epoch, activityID, "", name, key, outcome, data)
}

func (s *AppState) RecordActivityEvidenceContextForEpoch(ctx ToolEventContext, epoch uint64, activityID, name, key string, outcome ObservationOutcome, data []byte) (DetailRef, string, error) {
	return s.recordActivityEvidence(ctx, epoch, activityID, "", name, key, outcome, data)
}

func (s *AppState) recordActivityEvidence(ctx ToolEventContext, epoch uint64, activityID, toolUseID, name, key string, outcome ObservationOutcome, data []byte) (DetailRef, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.SessionID.Get() != ctx.SessionID || s.SessionEpoch.Get() != epoch {
		return DetailRef{}, "", ErrActivityScopeMismatch
	}
	ref, err := s.Details.Put(key, data)
	if err != nil {
		return DetailRef{}, "", err
	}
	observationID := "activity:" + ctx.SessionID + ":" + activityID
	observation := Observation{
		ID: observationID, SessionID: ctx.SessionID, TurnID: ctx.TurnID, WorkUnitID: ctx.WorkUnitID, ActorID: ctx.ActorID,
		ToolUseID: toolUseID, ToolName: name, Outcome: outcome,
	}
	if err := s.Observations.UpsertEvidenceObservation(observation, ref); err != nil {
		return DetailRef{}, "", err
	}
	messages := append([]Message(nil), s.Messages.Get()...)
	message := messageFromObservation(observation, MsgToolResult)
	message.DetailRefs = []DetailRef{ref}
	updated := false
	for i := range messages {
		if messages[i].ObservationID == observationID {
			messages[i] = message
			updated = true
			break
		}
	}
	if !updated {
		messages = append(messages, message)
	}
	s.Messages.Set(messages)
	s.ObservationRevision.Set(s.ObservationRevision.Get() + 1)
	return ref, observationID, nil
}

func (s *AppState) ApplyHookSummary(ctx ToolEventContext, epoch uint64, summary presentation.HookSummary) error {
	payload, err := json.Marshal(struct {
		ExecutionID string         `json:"hook_execution_id"`
		ToolUseID   string         `json:"tool_use_id,omitempty"`
		HookName    string         `json:"hook_name"`
		Status      string         `json:"status"`
		Summary     string         `json:"summary,omitempty"`
		Metadata    map[string]any `json:"metadata,omitempty"`
		SessionID   string         `json:"session_id"`
		TurnID      string         `json:"turn_id"`
		ActorID     string         `json:"actor_id"`
		WorkUnitID  string         `json:"work_unit_id"`
	}{summary.ExecutionID, summary.ToolUseID, summary.Name, summary.Status, summary.Summary, summary.Metadata, ctx.SessionID, ctx.TurnID, ctx.ActorID, ctx.WorkUnitID})
	if err != nil {
		return err
	}
	outcome := OutcomeSucceeded
	lifecycle := ActivityLifecycleCompleted
	if summary.Status == "blocked" || summary.Status == "prevented" {
		outcome, lifecycle = OutcomeDenied, ActivityLifecycleFailed
	} else if summary.Status == "failed" {
		outcome, lifecycle = OutcomeFailed, ActivityLifecycleFailed
	}
	activityID := summary.ExecutionID
	if activityID == "" {
		activityID = "hook:" + ctx.TurnID + ":" + summary.Name
	}
	ref, observationID, err := s.recordActivityEvidence(ctx, epoch, activityID, summary.ToolUseID, "Hook "+summary.Name, "hook:"+activityID+":"+summary.Status, outcome, payload)
	if err != nil {
		return err
	}
	return s.ApplyActivity(ActivityEvent{
		ID: activityID, SessionID: ctx.SessionID, Epoch: epoch, TurnID: ctx.TurnID, WorkUnitID: ctx.WorkUnitID,
		Actor: ActivityActor{ID: ctx.ActorID, Type: ctx.ActorType}, Kind: ActivityHook, Name: summary.Name,
		Phase: ActivityPhaseVerifying, Lifecycle: lifecycle, Outcome: outcome,
		Progress: ActivityProgress{Message: firstNonEmptyString(summary.Summary, summary.Status)},
		Control:  ActivityControl{JumpTarget: observationID, DetailRefs: []DetailRef{ref}},
	})
}

func (s *AppState) ApplyRuntimeError(ctx ToolEventContext, toolUseID, text string, apiError *types.APIError, metadata map[string]any) error {
	event := presentation.NewRuntimeErrorEvent(presentation.ToolEventContext{
		SessionID: ctx.SessionID, SessionEpoch: s.SessionEpoch.Get(), TurnID: ctx.TurnID,
		ActorID: ctx.ActorID, ActorType: ctx.ActorType, WorkUnitID: ctx.WorkUnitID,
	}, toolUseID, text, apiError, metadata)
	return s.ApplyRuntimeEvent(event)
}

// ApplyRuntimeEvent retains a diagnostic audit projection in the private
// detail store and sends only the strict user projection to the transcript.
// Raw causes and metadata require a separate, explicitly authorized audit
// sink; an interactive TUI must not silently opt itself into raw audit.
func (s *AppState) ApplyRuntimeEvent(event types.RuntimeEvent) error {
	lang := s.Language.Get()
	projector := runtimeevent.NewAudienceProjector()
	audit, err := projector.Project(event, runtimeevent.ProjectionOptions{
		Audience: runtimeevent.AudienceAudit, Redaction: runtimeevent.RedactionDiagnostic,
		Language: lang, LanguageSet: true,
	})
	if err != nil {
		return err
	}
	payload, err := json.Marshal(audit)
	if err != nil {
		return i18n.WrapInternalError(i18n.KeyRuntimeEventInvalid, err)
	}
	public, err := projector.Project(event, runtimeevent.ProjectionOptions{
		Audience: runtimeevent.AudienceUser, Redaction: runtimeevent.RedactionStrict,
		Language: lang, LanguageSet: true,
	})
	if err != nil {
		return err
	}
	publicText := public.Message
	outcome := observationOutcomeFromToolOutcome(event.Outcome)
	if outcome == OutcomeUnknown {
		return i18n.NewError(i18n.KeyRuntimeEventInvalid)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ref, err := s.Details.Put("runtime-error:"+event.EventID, payload)
	if err != nil {
		return err
	}
	// Runtime transport failures are independent observations. Reusing the
	// tool observation would overwrite a prior typed outcome (for example a
	// denied result) and erase its diagnostic preview.
	observationID := "runtime-error:" + event.EventID
	observation := Observation{
		ID: observationID, SessionID: event.SessionID, TurnID: event.TurnID,
		WorkUnitID: event.WorkUnitID, ActorID: event.ActorID, ToolUseID: event.ToolUseID,
		ToolName: "RuntimeError", Outcome: outcome,
	}
	if err := s.Observations.UpsertEvidenceObservation(observation, ref); err != nil {
		return err
	}
	s.appendMessageLocked(Message{Kind: MsgError, Text: publicText, Timestamp: time.Now(), ObservationID: observationID,
		ToolUseID: event.ToolUseID, WorkUnitID: event.WorkUnitID, ActorID: event.ActorID, Outcome: outcome})
	if s.Activities != nil {
		var running []Activity
		if event.ToolUseID != "" {
			if activity, ok := s.Activities.Get("tool:" + event.ToolUseID); ok && activity.Lifecycle == ActivityLifecycleRunning {
				running = append(running, activity)
			}
		}
		for _, activity := range running {
			if err := s.applyActivityLocked(ActivityEvent{
				ID: activity.ID, SessionID: activity.SessionID, Epoch: activity.Epoch,
				Lifecycle: activityLifecycleForOutcome(outcome), Outcome: outcome, Provisional: true,
				Progress: ActivityProgress{Message: publicText},
				Control:  ActivityControl{JumpTarget: observationID, DetailRefs: []DetailRef{ref}},
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func observationIsNormalPagination(outcome ObservationOutcome, completeness types.ToolResultCompleteness) bool {
	return outcome == OutcomePartial && completeness.Source == types.ToolResultCompletenessComplete &&
		completeness.View == types.ToolResultCompletenessPagination && completeness.Pagination != nil &&
		completeness.Pagination.HasMore
}

func messageFromObservation(observation Observation, kind MsgKind) Message {
	normalPagination := observationIsNormalPagination(observation.Outcome, observation.Presentation.Completeness)
	return Message{
		Kind:               kind,
		Text:               observationSummary(observation),
		ToolName:           observation.ToolName,
		Input:              cloneStringAnyMap(observation.ToolInput),
		IsError:            observation.Outcome != OutcomeRunning && observation.Outcome != OutcomeSucceeded && !normalPagination,
		Timestamp:          time.Now(),
		ObservationID:      observation.ID,
		ToolUseID:          observation.ToolUseID,
		SessionID:          observation.SessionID,
		TurnID:             observation.TurnID,
		WorkUnitID:         observation.WorkUnitID,
		ActorID:            observation.ActorID,
		Outcome:            observation.Outcome,
		Completeness:       observation.Presentation.Completeness.Clone(),
		Disclosure:         observation.Disclosure,
		DetailRefs:         append([]DetailRef(nil), observation.ResultRefs...),
		PresentationHidden: observation.Aggregation.Hidden,
		AggregateID:        observation.Aggregation.GroupID,
		AggregateSummary:   observation.Aggregation.Summary,
	}
}

func observationSummary(observation Observation) string {
	return observationSummaryInLanguage(i18n.DetectOrLoadLanguage(), observation)
}

func observationSummaryInLanguage(lang i18n.Language, observation Observation) string {
	if observation.Aggregation.Representative && observation.Aggregation.Summary != "" {
		return observation.Aggregation.Summary
	}
	if summary := strings.TrimSpace(observation.Presentation.Summary); summary != "" {
		return summary
	}
	if observation.ToolName == "" {
		return observationOutcomeLabelInLanguage(lang, observation.Outcome)
	}
	if len(observation.ResultRefs) == 0 {
		return toolInputPreview(observation.ToolName, observation.ToolInput)
	}
	ref := observation.ResultRefs[len(observation.ResultRefs)-1]
	return i18n.Format(lang, i18n.KeyTUIObservationSummary, observation.ToolName, observationOutcomeLabelInLanguage(lang, observation.Outcome), ref.Size)
}

func (s *AppState) GetObservationAggregate(id string) (ObservationAggregate, bool) {
	s.mu.Lock()
	observations := s.Observations
	s.mu.Unlock()
	if observations == nil {
		return ObservationAggregate{}, false
	}
	return observations.Aggregate(id)
}

// RelocalizeToolPresentations updates every stored semantic projection and its
// transcript anchor while preserving raw evidence and user disclosure state.
func (s *AppState) RelocalizeToolPresentations(lang i18n.Language) error {
	if s == nil || s.Observations == nil {
		return nil
	}
	err := s.Observations.Relocalize(lang)
	s.mu.Lock()
	messages := append([]Message(nil), s.Messages.Get()...)
	for index := range messages {
		observation, ok := s.Observations.Get(messages[index].ObservationID)
		if !ok {
			continue
		}
		replacement := messageFromObservation(observation, messages[index].Kind)
		replacement.Timestamp = messages[index].Timestamp
		messages[index] = replacement
	}
	s.Messages.Set(messages)
	s.ObservationRevision.Set(s.ObservationRevision.Get() + 1)
	s.mu.Unlock()
	return err
}

func observationOutcomeLabelInLanguage(lang i18n.Language, outcome ObservationOutcome) string {
	code := "unknown"
	switch outcome {
	case OutcomeRunning:
		code = "running"
	case OutcomeSucceeded:
		code = "completed"
	case OutcomeFailed:
		code = "failed"
	case OutcomePartial:
		code = "partial"
	case OutcomeDenied:
		code = "denied"
	case OutcomeCancelled:
		code = "cancelled"
	case OutcomeTimedOut:
		code = "timed_out"
	case OutcomeConflict:
		code = "identity_conflict"
	case OutcomeOrphan:
		code = "orphan"
	}
	return i18n.TUIOutcomeLabel(lang, code)
}

// ExpandTasksView is the TaskCreate UI lifecycle sink. It is safe for adjacent
// concurrency-safe TaskCreate calls and always produces a refresh revision.
func (s *AppState) ExpandTasksView(items []TaskViewItem) {
	if s == nil {
		return
	}
	s.runBatch(func() {
		s.mu.Lock()
		changed := s.setTaskViewItemsLocked(items)
		s.SetExpandedView("tasks")
		if changed {
			s.TaskListRevision.Set(s.TaskListRevision.Get() + 1)
		}
		s.mu.Unlock()
	})
}

func (s *AppState) RefreshTasksView(items []TaskViewItem) {
	if s == nil {
		return
	}
	s.runBatch(func() {
		s.mu.Lock()
		if s.setTaskViewItemsLocked(items) {
			s.TaskListRevision.Set(s.TaskListRevision.Get() + 1)
		}
		s.mu.Unlock()
	})
}

func (s *AppState) setTaskViewItemsLocked(items []TaskViewItem) bool {
	current := s.TaskViewItems.Get()
	if taskViewItemsEqual(current, items) {
		return false
	}
	copyItems := make([]TaskViewItem, len(items))
	for i, item := range items {
		copyItems[i] = item
		copyItems[i].BlockedBy = append([]string(nil), item.BlockedBy...)
	}
	s.TaskViewItems.Set(copyItems)
	return true
}

func taskViewItemsEqual(left, right []TaskViewItem) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i].ID != right[i].ID || left[i].Subject != right[i].Subject || left[i].Status != right[i].Status || left[i].Owner != right[i].Owner || len(left[i].BlockedBy) != len(right[i].BlockedBy) {
			return false
		}
		for j := range left[i].BlockedBy {
			if left[i].BlockedBy[j] != right[i].BlockedBy[j] {
				return false
			}
		}
	}
	return true
}

// ResetSessionUsage clears the active session's usage counters shown in the
// status bar. Callers should use this when switching to a different session.
func (s *AppState) ResetSessionUsage() {
	s.compactionBoundaries.Clear()
	s.progressiveProjections.Clear()
	s.usageProjectionMu.Lock()
	defer s.usageProjectionMu.Unlock()
	s.usageProjectionRevision = 0
	s.SessionUsageKnown.Set(true)
	s.SessionRoundUsageKnown.Set(true)
	s.CumulativeCost.Set(0)
	s.SessionCostKnown.Set(true)
	s.SessionInputTokens.Set(0)
	s.SessionOutputTokens.Set(0)
	s.SessionCacheReadTokens.Set(0)
	s.SessionCacheCreateTokens.Set(0)
	s.SessionWebSearchRequests.Set(0)
	s.SessionTotalInputTokens.Set(0)
	s.SessionTotalOutputTokens.Set(0)
	s.SessionTotalCacheReadTokens.Set(0)
	s.SessionTotalCacheCreateTokens.Set(0)
	s.SessionHasCompacted.Set(false)
	s.SessionCompactionBaselineKnown.Set(false)
	s.SessionCompactionCount.Set(0)
	s.SessionProgressiveProjectionCount.Set(0)
	s.SessionProgressiveProjectedTools.Set(0)
	s.SessionProgressiveTokensSaved.Set(0)
	s.SessionProgressiveSavingsUSD.Set(0)
	s.ProgressivePendingTools.Set(0)
	s.ProgressivePendingTokens.Set(0)
	s.SessionCompletedRoundInputTokens.Set(0)
	s.SessionCompletedRoundOutputTokens.Set(0)
	s.SessionInputTokensAtCompact.Set(0)
	s.SessionCacheReadAtCompact.Set(0)
}

// ApplyProgressiveContextMetrics updates the transient pending snapshot or adds
// one successfully installed provider-view projection to the session benefit
// ledger. The receipt identity makes event redelivery idempotent; rejected and
// shadow candidates never affect realized savings.
func (s *AppState) ApplyProgressiveContextMetrics(sessionID string, epoch uint64, identity string, progress stream.ProgressEvent) bool {
	if s == nil || sessionID == "" || s.SessionID.Get() != sessionID || s.SessionEpoch.Get() != epoch ||
		progress.Stage != "progressive_context_projection" || strings.TrimSpace(identity) == "" {
		return false
	}
	if pendingOnly, _ := progress.Metadata["pending_only"].(bool); pendingOnly {
		pendingTools := max(compactionMetadataInt(progress.Metadata, "pending_tools"), 0)
		pendingTokens := max(compactionMetadataInt(progress.Metadata, "pending_tokens"), 0)
		if pendingTools == 0 || pendingTokens == 0 {
			pendingTools, pendingTokens = 0, 0
		}
		if _, loaded := s.progressiveProjections.LoadOrStore(identity, struct{}{}); loaded {
			return false
		}
		s.usageProjectionMu.Lock()
		defer s.usageProjectionMu.Unlock()
		s.ProgressivePendingTools.Set(pendingTools)
		s.ProgressivePendingTokens.Set(pendingTokens)
		return true
	}
	applied, _ := progress.Metadata["applied"].(bool)
	shadow, _ := progress.Metadata["shadow"].(bool)
	projectedTools := compactionMetadataInt(progress.Metadata, "projection_count")
	tokensSaved := compactionMetadataInt(progress.Metadata, "tokens_saved")
	if !applied || shadow || projectedTools <= 0 || tokensSaved <= 0 {
		return false
	}
	if _, loaded := s.progressiveProjections.LoadOrStore(identity, struct{}{}); loaded {
		return false
	}
	estimatedSavings := compactionMetadataFloat(progress.Metadata, "estimated_net_savings_usd")
	if estimatedSavings < 0 {
		estimatedSavings = 0
	}
	s.usageProjectionMu.Lock()
	defer s.usageProjectionMu.Unlock()
	s.SessionProgressiveProjectionCount.Set(s.SessionProgressiveProjectionCount.Get() + 1)
	s.SessionProgressiveProjectedTools.Set(s.SessionProgressiveProjectedTools.Get() + projectedTools)
	s.SessionProgressiveTokensSaved.Set(s.SessionProgressiveTokensSaved.Get() + tokensSaved)
	s.SessionProgressiveSavingsUSD.Set(s.SessionProgressiveSavingsUSD.Get() + estimatedSavings)
	return true
}

// MarkSessionCompacted closes the current conversation segment by committing
// its final request usage, then opens an empty segment while preserving the
// cumulative session ledger.
func (s *AppState) MarkSessionCompacted() {
	s.usageProjectionMu.Lock()
	defer s.usageProjectionMu.Unlock()
	s.SessionHasCompacted.Set(true)
	s.SessionCompactionBaselineKnown.Set(true)
	s.SessionCompactionCount.Set(s.SessionCompactionCount.Get() + 1)
	s.SessionCompletedRoundInputTokens.Set(s.SessionCompletedRoundInputTokens.Get() + s.SessionInputTokens.Get())
	s.SessionCompletedRoundOutputTokens.Set(s.SessionCompletedRoundOutputTokens.Get() + s.SessionOutputTokens.Get())
	s.SessionInputTokensAtCompact.Set(s.SessionTotalInputTokens.Get())
	s.SessionCacheReadAtCompact.Set(s.SessionTotalCacheReadTokens.Get())
	s.SessionInputTokens.Set(0)
	s.SessionOutputTokens.Set(0)
	s.SessionCacheReadTokens.Set(0)
	s.SessionCacheCreateTokens.Set(0)
	s.ProgressivePendingTools.Set(0)
	s.ProgressivePendingTokens.Set(0)
}

// MarkSessionCompactedBoundary applies a stable boundary identity once. The
// map is session-scoped and cleared whenever usage is reset for a new session.
func (s *AppState) MarkSessionCompactedBoundary(identity string) bool {
	identity = strings.TrimSpace(identity)
	if identity != "" {
		if _, loaded := s.compactionBoundaries.LoadOrStore(identity, struct{}{}); loaded {
			return false
		}
	}
	s.MarkSessionCompacted()
	return true
}

// ApplySessionInfo updates session metadata and resets status-bar usage when
// the active session changes. The returned bool reports whether the session ID
// changed from a previous non-empty value.
func (s *AppState) ApplySessionInfo(id string, tools []string) bool {
	prevID := s.SessionID.Get()
	s.SessionID.Set(id)
	s.Tools.Set(tools)
	if prevID != "" && prevID != id {
		s.ResetSessionUsage()
		return true
	}
	return false
}

// SignalStop signals all blocked goroutines (e.g. PermissionRequest) to unblock.
// Safe to call multiple times (close is idempotent via sync.Once internally—
// but since we only call it from one place, a simple recover guard suffices).
func (s *AppState) SignalStop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.stopCh:
		// already closed
	default:
		close(s.stopCh)
	}
}

// AppendMessage adds a message to the conversation (thread-safe).
func (s *AppState) AppendMessage(msg Message) {
	s.runBatch(func() { s.appendMessage(msg) })
}

func (s *AppState) appendMessage(msg Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.appendMessageLocked(msg)
}

// AppendSendUserMessage installs the Brief output as the primary assistant
// channel. In default/chat modes, assistant text streamed earlier in the same
// user turn is detail-only and is removed to prevent duplicate visible text.
func (s *AppState) AppendSendUserMessage(output interaction.SendUserMessageOutput, options presentation.SendUserMessageRenderOptions, observations ...Observation) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.flushStreamBufLocked()
	s.cancelStreamTimerLocked()

	old := s.Messages.Get()
	if len(old) > 0 {
		last := old[len(old)-1]
		if last.Kind == MsgAssistant && last.Stream != nil && !last.Stream.IsFinalized() {
			last.Stream.Finalize()
		}
	}
	if options.Mode != presentation.SendUserMessageRenderTranscript && options.DropAssistantText {
		old = append([]Message(nil), old...)
		if options.TurnCount != 0 {
			filtered := make([]Message, 0, len(old))
			for _, message := range old {
				if message.Kind == MsgAssistant && message.TurnCount == options.TurnCount {
					continue
				}
				filtered = append(filtered, message)
			}
			old = filtered
		} else if len(old) > 0 && old[len(old)-1].Kind == MsgAssistant {
			old = old[:len(old)-1]
		}
	} else {
		old = append([]Message(nil), old...)
	}

	copyOutput := output
	copyOutput.Attachments = append([]interaction.SendUserMessageAttachment(nil), output.Attachments...)
	messageTime := time.Now()
	if sentAt, err := time.Parse(time.RFC3339Nano, output.SentAt); err == nil {
		messageTime = sentAt
	}
	message := Message{
		Kind:      MsgSendUserMessage,
		Text:      output.Message,
		Timestamp: messageTime,
		TurnCount: options.TurnCount,
		Brief:     &copyOutput,
		BriefMode: options.Mode,
	}
	if len(observations) > 0 {
		observation := observations[0]
		message.ObservationID = observation.ID
		message.ToolUseID = observation.ToolUseID
		message.SessionID = observation.SessionID
		message.TurnID = observation.TurnID
		message.WorkUnitID = observation.WorkUnitID
		message.ActorID = observation.ActorID
		message.Outcome = observation.Outcome
		message.Disclosure = observation.Disclosure
		message.DetailRefs = append([]DetailRef(nil), observation.ResultRefs...)
	}
	next := make([]Message, len(old)+1)
	copy(next, old)
	next[len(old)] = message
	s.Messages.Set(next)
	s.bumpViewRevision()
}

// ClearMessages removes all messages from the conversation (thread-safe).
// Used by /clear to reset both engine and TUI state. Also cancels any pending
// debounce timer and increments clearEpoch
// to prevent inflight streaming writes from creating ghost messages.
func (s *AppState) ClearMessages() {
	s.runBatch(s.clearMessages)
}

func (s *AppState) clearMessages() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancelStreamTimerLocked()
	s.streamBuf = s.streamBuf[:0]
	s.Messages.Set(nil)
	s.ToolSegmentExpansion.Set(nil)
	s.LLMCall.Set(nil)
	s.LLMRequestMetrics.Set(nil)
	s.bumpViewRevision()
	s.clearEpoch++
}

func (s *AppState) SetLLMCall(status *LLMCallStatus) {
	if status == nil {
		s.LLMCall.Set(nil)
		return
	}
	copyStatus := *status
	s.LLMCall.Set(&copyStatus)
}

func (s *AppState) SetLLMRequestMetrics(status *LLMRequestMetricsStatus) {
	if status == nil {
		s.LLMRequestMetrics.Set(nil)
		return
	}
	copyStatus := *status
	s.LLMRequestMetrics.Set(&copyStatus)
}

// BeginLLMWork publishes the working state as soon as a submitted message is
// accepted for query dispatch, before provider setup or stream establishment.
func (s *AppState) BeginLLMWork() {
	now := time.Now()
	s.SetLLMCall(&LLMCallStatus{
		Phase: LLMCallWorking, Stage: LLMStagePreparing, StageStartedAt: now,
		UpdatedAt: now, WorkStartedAt: now,
	})
}

// SetLLMActivity advances the current query's visible stage only when an
// observable runtime boundary changes. Repeated streaming deltas in the same
// stage do not reset its elapsed timer or cause extra redraw state.
func (s *AppState) SetLLMActivity(stage LLMCallStage, detail string, toolInputBytes ...int) bool {
	return s.setLLMActivityAt(stage, detail, time.Now(), toolInputBytes...)
}

func (s *AppState) setLLMActivityAt(stage LLMCallStage, detail string, now time.Time, toolInputBytes ...int) bool {
	current := s.LLMCall.Get()
	if current == nil || stage == "" {
		return false
	}
	detail = strings.TrimSpace(detail)
	receivedBytes := 0
	if stage == LLMStageToolInput && len(toolInputBytes) > 0 && toolInputBytes[0] > 0 {
		receivedBytes = toolInputBytes[0]
	}
	sameStage := current.Stage == stage && current.StageDetail == detail
	if sameStage && current.ToolInputBytes == receivedBytes {
		return false
	}
	updated := *current
	updated.Stage = stage
	updated.StageDetail = detail
	updated.ToolInputBytes = receivedBytes
	if !sameStage {
		updated.StageStartedAt = now
	}
	updated.UpdatedAt = now
	s.SetLLMCall(&updated)
	return true
}

func (s *AppState) ClearLLMCall(requestID ...string) {
	s.clearLLMCallAt(time.Now(), requestID...)
}

func (s *AppState) clearLLMCallAt(now time.Time, requestID ...string) {
	current := s.LLMCall.Get()
	if current == nil {
		return
	}
	if len(requestID) > 0 && requestID[0] != "" && current.RequestID != requestID[0] {
		return
	}
	if !current.WorkStartedAt.IsZero() {
		duration := now.Sub(current.WorkStartedAt)
		if duration < 0 {
			duration = 0
		}
		s.mu.Lock()
		messages := s.Messages.Get()
		for index := len(messages) - 1; index >= 0; index-- {
			message := messages[index]
			if message.Kind != MsgAssistant || message.Timestamp.Before(current.WorkStartedAt) {
				continue
			}
			updated := make([]Message, len(messages))
			copy(updated, messages)
			updated[index].WorkDuration = duration
			s.Messages.Set(updated)
			s.bumpViewRevision()
			break
		}
		s.mu.Unlock()
	}
	s.LLMCall.Set(nil)
}

// ClearEpoch returns the current clear epoch (for snapshot comparison).
func (s *AppState) ClearEpoch() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.clearEpoch
}

// appendMessageLocked adds a message; caller must hold s.mu.
// Uses copy-on-write to produce a new slice, ensuring the previous slice
// returned by Get() is never mutated (important for go-tui diff detection
// and preventing data races with concurrent render reads).
func (s *AppState) appendMessageLocked(msg Message) {
	old := s.Messages.Get()
	nw := make([]Message, len(old)+1)
	copy(nw, old)
	nw[len(old)] = msg
	s.Messages.Set(nw)
	s.bumpViewRevision()
}

// AppendToLast appends text to the last message (thread-safe, for streaming tokens).
func (s *AppState) AppendToLast(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.appendToLastLocked(text)
}

// AppendOrMergeThinkingForTurn coalesces streamed reasoning deltas into one
// presentation row. A text/tool boundary changes the last message kind, and a
// loop turn change prevents unrelated reasoning blocks from being merged.
func (s *AppState) AppendOrMergeThinkingForTurn(text string, turnCount int) {
	if text == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	old := s.Messages.Get()
	if len(old) > 0 && old[len(old)-1].Kind == MsgAssistantThinking &&
		(turnCount == 0 || old[len(old)-1].TurnCount == turnCount) {
		nw := make([]Message, len(old))
		copy(nw, old)
		nw[len(nw)-1].Text += text
		s.Messages.Set(nw)
		s.bumpViewRevision()
		return
	}
	s.appendMessageLocked(Message{
		Kind: MsgAssistantThinking, Text: text,
		Timestamp: time.Now(), TurnCount: turnCount,
	})
}

// AccumulateSessionUsage records the latest completed model turn for the
// status bar while retaining cumulative counters for session persistence.
// Callers should pass EventTurnEnd usage, not partial stream deltas.
func (s *AppState) AccumulateSessionUsage(usage *types.Usage) {
	if usage == nil {
		return
	}
	s.usageProjectionMu.Lock()
	defer s.usageProjectionMu.Unlock()
	s.SessionUsageKnown.Set(true)
	if !s.SessionHasCompacted.Get() {
		s.SessionRoundUsageKnown.Set(true)
	}
	s.SessionInputTokens.Set(usage.TotalInputTokens())
	s.SessionOutputTokens.Set(max(usage.OutputTokens, 0))
	s.SessionCacheReadTokens.Set(usage.CacheReadInputTokens)
	s.SessionCacheCreateTokens.Set(usage.CacheCreationInputTokens)
	s.SessionWebSearchRequests.Set(s.SessionWebSearchRequests.Get() + usage.ServerToolUse.WebSearchRequests)
	s.SessionTotalInputTokens.Set(s.SessionTotalInputTokens.Get() + usage.TotalInputTokens())
	s.SessionTotalOutputTokens.Set(s.SessionTotalOutputTokens.Get() + max(usage.OutputTokens, 0))
	s.SessionTotalCacheReadTokens.Set(s.SessionTotalCacheReadTokens.Get() + usage.CacheReadInputTokens)
	s.SessionTotalCacheCreateTokens.Set(s.SessionTotalCacheCreateTokens.Get() + usage.CacheCreationInputTokens)
}

// ApplySessionUsageProjection publishes one atomic tracker projection and
// rejects an older revision that arrived late from another event producer.
func (s *AppState) ApplySessionUsageProjection(usage presentation.SessionUsageProjection) bool {
	s.usageProjectionMu.Lock()
	defer s.usageProjectionMu.Unlock()
	if usage.Revision > 0 {
		if usage.Revision < s.usageProjectionRevision {
			return false
		}
		s.usageProjectionRevision = usage.Revision
	}
	s.SessionUsageKnown.Set(usage.Known)
	s.SessionTotalInputTokens.Set(usage.TotalInputTokens)
	s.SessionTotalOutputTokens.Set(usage.OutputTokens)
	s.SessionTotalCacheReadTokens.Set(usage.TotalCacheRead)
	s.SessionTotalCacheCreateTokens.Set(usage.CacheCreateTokens)
	s.SessionWebSearchRequests.Set(usage.WebSearchRequests)
	s.SessionHasCompacted.Set(usage.HasCompacted)
	s.SessionCompactionBaselineKnown.Set(usage.BaselineKnown)
	s.SessionInputTokensAtCompact.Set(usage.InputAtCompact)
	s.SessionCacheReadAtCompact.Set(usage.CacheAtCompact)
	s.CumulativeCost.Set(usage.CostUSD)
	if usage.CostCurrency != "" {
		s.ModelCostCurrency.Set(usage.CostCurrency)
	}
	s.SessionCostKnown.Set(usage.CostKnown)
	return true
}

// SetSessionCost is the compatibility write path for renderers that have not
// adopted SessionUsageProjection.
func (s *AppState) SetSessionCost(cost float64) {
	s.usageProjectionMu.Lock()
	s.CumulativeCost.Set(cost)
	s.usageProjectionMu.Unlock()
}

// SetSessionCostKnown is the compatibility counterpart to SetSessionCost.
func (s *AppState) SetSessionCostKnown(known bool) {
	s.usageProjectionMu.Lock()
	s.SessionCostKnown.Set(known)
	s.usageProjectionMu.Unlock()
}

// AppendOrStreamText atomically checks the last message: if it's an assistant
// message with an active StreamRenderer, buffers the token for debounced flush;
// otherwise creates a new assistant message with a fresh StreamRenderer.
// This is the thread-safe entrypoint for streaming tokens.
//
// Debounce strategy: tokens are accumulated in streamBuf. A 50ms timer is
// started on the first token. When the timer fires, all buffered tokens are
// flushed in a single feedStreamLocked → Messages.Set() call, triggering
// exactly one go-tui redraw. This reduces redraws from ~60-80/s to ~20/s
// during fast streaming.
func (s *AppState) AppendOrStreamText(text string) {
	s.AppendOrStreamTextForTurn(text, 0)
}

// AppendOrStreamTextForTurn retains the loop turn on assistant messages so a
// later Brief result can hide only its own redundant text.
func (s *AppState) AppendOrStreamTextForTurn(text string, turnCount int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	msgs := s.Messages.Get()
	if len(msgs) > 0 && msgs[len(msgs)-1].Kind == MsgAssistant && msgs[len(msgs)-1].Stream != nil && !msgs[len(msgs)-1].Stream.IsFinalized() && (turnCount == 0 || msgs[len(msgs)-1].TurnCount == turnCount) {
		// Buffer the token for debounced flush
		s.streamBuf = append(s.streamBuf, text)

		if s.streamTimer == nil {
			// First token in this debounce window — start timer.
			// We also do an immediate flush of this first token so the user
			// sees text appear without a 50ms delay on the very first token.
			s.flushStreamBufLocked()
			s.streamTimer = time.AfterFunc(streamDebounceInterval, func() {
				s.mu.Lock()
				defer s.mu.Unlock()
				s.flushStreamBufLocked()
				s.streamTimer = nil
			})
		}
	} else {
		// No active streaming message — create a new one.
		// Cancel any stale debounce timer (shouldn't happen, but defensive).
		s.cancelStreamTimerLocked()

		sr := NewStreamRenderer()
		sr.Feed(text)
		s.appendMessageLocked(Message{
			Kind:      MsgAssistant,
			Text:      text,
			Stream:    sr,
			Timestamp: time.Now(),
			TurnCount: turnCount,
		})
	}
}

// flushStreamBufLocked flushes all buffered streaming tokens into the last
// message in a single batch, triggering one Messages.Set() call.
// Caller must hold s.mu.
func (s *AppState) flushStreamBufLocked() {
	if len(s.streamBuf) == 0 {
		return
	}
	// Join all buffered tokens
	var sb strings.Builder
	for _, t := range s.streamBuf {
		sb.WriteString(t)
	}
	s.streamBuf = s.streamBuf[:0]

	s.feedStreamLocked(sb.String())
}

// cancelStreamTimerLocked stops and clears the debounce timer. Caller must hold s.mu.
func (s *AppState) cancelStreamTimerLocked() {
	if s.streamTimer != nil {
		s.streamTimer.Stop()
		s.streamTimer = nil
	}
}

// feedStreamLocked appends a token to the last message's StreamRenderer and
// updates the raw Text field. Caller must hold s.mu.
//
// Copy-on-Write note: we create a new []Message slice so that the previous
// slice (which go-tui's render goroutine may be reading) is never mutated.
// However, the Message.Stream field is a *StreamRenderer POINTER — the new
// and old slices share the same StreamRenderer instance. This is intentional:
// the StreamRenderer's internal state (blocks, pending) is only mutated under
// s.mu, and the render goroutine only calls Stream.Lines() (read-only) after
// receiving the new slice via State.Get(). The copy-on-write guarantees slice-
// level isolation, NOT deep isolation of pointer fields.
func (s *AppState) feedStreamLocked(text string) {
	old := s.Messages.Get()
	if len(old) == 0 {
		return
	}
	nw := make([]Message, len(old))
	copy(nw, old)
	last := &nw[len(nw)-1]
	last.Text += text
	// Feed the new token to the StreamRenderer — this will detect block
	// boundaries and cache completed blocks via glamour.
	if last.Stream != nil {
		last.Stream.Feed(text)
	}
	s.Messages.Set(nw)
	s.bumpViewRevision()
}

// FinalizeStream finalizes the StreamRenderer on the last assistant message,
// rendering any remaining pending Markdown via glamour. Call this when the
// API stream ends. Thread-safe.
//
// This also flushes any debounce-buffered tokens and cancels the debounce
// timer, ensuring no tokens are lost at stream end.
//
// Copy-on-Write note: as with feedStreamLocked, the new and old slices share
// the same *StreamRenderer. Finalize() mutates sr.finalized and sr.blocks
// on the shared instance. This is safe because the old slice is immediately
// superseded by Messages.Set(nw), and the render goroutine will pick up the
// new slice on its next read.
func (s *AppState) FinalizeStream() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Flush any remaining debounced tokens before finalizing
	s.flushStreamBufLocked()
	s.cancelStreamTimerLocked()

	msgs := s.Messages.Get()
	if len(msgs) == 0 {
		return
	}
	last := msgs[len(msgs)-1]
	if last.Kind != MsgAssistant || last.Stream == nil {
		return
	}
	nw := make([]Message, len(msgs))
	copy(nw, msgs)
	nw[len(nw)-1].Stream.Finalize()
	s.Messages.Set(nw)
	s.bumpViewRevision()
}

// appendToLastLocked appends text to the last message; caller must hold s.mu.
// Uses copy-on-write to produce a new slice, preserving immutability of the
// previously returned value for go-tui diff and concurrent render safety.
//
// Note: For streaming assistant messages, prefer feedStreamLocked which also
// updates the StreamRenderer. This function is kept for non-assistant messages
// (e.g. AppendToLast which is used by tests).
func (s *AppState) appendToLastLocked(text string) {
	old := s.Messages.Get()
	if len(old) == 0 {
		return
	}
	nw := make([]Message, len(old))
	copy(nw, old)
	nw[len(nw)-1].Text += text
	s.Messages.Set(nw)
	s.bumpViewRevision()
}

// SetQueryCancel registers a cancel function for the active query.
// When set, Ctrl+C will call this instead of exiting the TUI.
func (s *AppState) SetQueryCancel(fn func()) uint64 {
	s.mu.Lock()
	s.queryGeneration++
	generation := s.queryGeneration
	s.QueryCancelFn = fn
	s.queryInFlight = fn != nil
	s.mu.Unlock()
	return generation
}

// TryReserveQuery atomically reserves the single foreground query slot before
// the composer is cleared or a worker goroutine is started.
func (s *AppState) TryReserveQuery(cancel func()) (uint64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.queryInFlight {
		return 0, false
	}
	s.queryGeneration++
	s.QueryCancelFn = cancel
	s.queryInFlight = true
	return s.queryGeneration, true
}

// SetReservedQueryCancel replaces the admission-time cancel function with the
// actual provider-query cancel function without changing the reservation.
func (s *AppState) SetReservedQueryCancel(generation uint64, cancel func()) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if generation == 0 || generation != s.queryGeneration || !s.queryInFlight {
		return false
	}
	s.QueryCancelFn = cancel
	return true
}

// HasActiveQueryOtherThan reports whether an operation conflicts with a
// reservation it does not own.
func (s *AppState) HasActiveQueryOtherThan(generation uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.queryInFlight && (generation == 0 || generation != s.queryGeneration)
}

// ClearQueryCancel removes the active query cancel function.
func (s *AppState) ClearQueryCancel(generations ...uint64) {
	s.mu.Lock()
	if len(generations) == 0 || generations[0] == s.queryGeneration {
		s.QueryCancelFn = nil
		s.queryInFlight = false
	}
	s.mu.Unlock()
}

// HasActiveQuery reports whether a session transition would race an in-flight
// query. Clear/resume callers must wait for or reject such a transition.
func (s *AppState) HasActiveQuery() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.queryInFlight
}

// TryCancelQuery attempts to cancel the active query.
// Returns true if a query was cancelled, false if no query is active.
// Uses swap-and-nil to ensure the cancel function is called at most once,
// even under concurrent Ctrl+C presses.
func (s *AppState) TryCancelQuery() bool {
	s.mu.Lock()
	fn := s.QueryCancelFn
	s.QueryCancelFn = nil // prevent duplicate cancellation; in-flight stays true until terminal commit
	s.mu.Unlock()
	if fn != nil {
		fn()
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Pending images (clipboard paste)
// ---------------------------------------------------------------------------

// AddPendingImage adds an image attachment to the pending list.
// Returns the assigned display ID (1-based). Thread-safe.
func (s *AppState) AddPendingImage(base64Data, mediaType string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.imageCounter++
	placeholder := i18n.Format(s.Language.Get(), i18n.KeyTUIImageTag, s.imageCounter)
	img := ImageAttachment{
		ID:          s.imageCounter,
		Base64:      base64Data,
		MediaType:   mediaType,
		Placeholder: placeholder,
	}
	old := s.PendingImages.Get()
	nw := make([]ImageAttachment, len(old)+1)
	copy(nw, old)
	nw[len(old)] = img
	s.PendingImages.Set(nw)
	s.bumpViewRevision()
	return img.ID
}

// RemovePendingImage removes the pending image with the supplied display ID.
func (s *AppState) RemovePendingImage(id int) (ImageAttachment, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	imgs := s.PendingImages.Get()
	for index, image := range imgs {
		if image.ID != id {
			continue
		}
		next := make([]ImageAttachment, 0, len(imgs)-1)
		next = append(next, imgs[:index]...)
		next = append(next, imgs[index+1:]...)
		s.PendingImages.Set(next)
		s.PendingImageSelected.Set(-1)
		s.bumpViewRevision()
		return image, true
	}
	return ImageAttachment{}, false
}

// MovePendingImageSelection moves the pending-image selection. A negative
// delta from the input selects the last image; moving past the last image
// returns focus semantics to the input by clearing the selection.
func (s *AppState) MovePendingImageSelection(delta int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	imgs := s.PendingImages.Get()
	if len(imgs) == 0 {
		if s.PendingImageSelected.Get() != -1 {
			s.PendingImageSelected.Set(-1)
			s.bumpViewRevision()
		}
		return false
	}
	idx := s.PendingImageSelected.Get()
	if idx < 0 || idx >= len(imgs) {
		if delta < 0 {
			s.PendingImageSelected.Set(len(imgs) - 1)
			s.bumpViewRevision()
			return true
		}
		return false
	}
	idx += delta
	switch {
	case idx < 0:
		idx = 0
	case idx >= len(imgs):
		s.PendingImageSelected.Set(-1)
		s.bumpViewRevision()
		return true
	}
	s.PendingImageSelected.Set(idx)
	s.bumpViewRevision()
	return true
}

// ClearPendingImageSelection returns keyboard focus semantics to the input.
func (s *AppState) ClearPendingImageSelection() {
	if s.PendingImageSelected.Get() == -1 {
		return
	}
	s.PendingImageSelected.Set(-1)
	s.bumpViewRevision()
}

// DeleteSelectedPendingImage removes the selected pending image, if any.
func (s *AppState) DeleteSelectedPendingImage() (ImageAttachment, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	imgs := s.PendingImages.Get()
	idx := s.PendingImageSelected.Get()
	if idx < 0 || idx >= len(imgs) {
		return ImageAttachment{}, false
	}
	removed := imgs[idx]
	next := make([]ImageAttachment, 0, len(imgs)-1)
	next = append(next, imgs[:idx]...)
	next = append(next, imgs[idx+1:]...)
	s.PendingImages.Set(next)
	if len(next) == 0 {
		s.PendingImageSelected.Set(-1)
	} else if idx >= len(next) {
		s.PendingImageSelected.Set(len(next) - 1)
	} else {
		s.PendingImageSelected.Set(idx)
	}
	s.bumpViewRevision()
	return removed, true
}

// ClearPendingImages removes all pending images. Thread-safe.
func (s *AppState) ClearPendingImages() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.PendingImages.Get()) == 0 && s.PendingImageSelected.Get() == -1 {
		return
	}
	s.PendingImages.Set(nil)
	s.PendingImageSelected.Set(-1)
	s.bumpViewRevision()
}

// TakePendingImages atomically removes and returns the pending images.
// Returns nil if no images are pending. Thread-safe.
func (s *AppState) TakePendingImages() []ImageAttachment {
	s.mu.Lock()
	defer s.mu.Unlock()
	imgs := s.PendingImages.Get()
	if len(imgs) == 0 {
		return nil
	}
	s.PendingImages.Set(nil)
	s.PendingImageSelected.Set(-1)
	s.bumpViewRevision()
	return imgs
}

// toolInputPreview returns a short human-readable preview of a tool's input map.
func toolInputPreview(name string, input map[string]any) string {
	switch name {
	case "Bash":
		if cmd, ok := input["command"].(string); ok {
			cmd = truncateRunes(cmd, 100, "...")
			return fmt.Sprintf("`%s`", cmd)
		}
	case "Read", "Write", "Edit":
		if fp, ok := input["file_path"].(string); ok {
			return fp
		}
	case "Glob":
		if p, ok := input["pattern"].(string); ok {
			return p
		}
	case "Grep":
		if p, ok := input["pattern"].(string); ok {
			return fmt.Sprintf("/%s/", p)
		}
	case "WebFetch":
		if target, ok := input["url"].(string); ok {
			return truncateRunes(strings.TrimSpace(target), 100, "...")
		}
	case "WebSearch":
		for _, key := range []string{"query", "search_query"} {
			if query, ok := input[key].(string); ok && strings.TrimSpace(query) != "" {
				return truncateRunes(strings.TrimSpace(query), 100, "...")
			}
		}
	case "Agent":
		agentLabel := ""
		if desc, ok := input["description"].(string); ok {
			agentLabel = strings.TrimSpace(desc)
		}
		if agentLabel == "" {
			if name, ok := input["name"].(string); ok {
				agentLabel = strings.TrimSpace(name)
			}
		}
		if typ, ok := input["subagent_type"].(string); ok && strings.TrimSpace(typ) != "" {
			if agentLabel != "" {
				agentLabel += " "
			}
			agentLabel += "[" + strings.TrimSpace(typ) + "]"
		}
		if agentLabel != "" {
			return truncateRunes(agentLabel, 80, "...")
		}
		if p, ok := input["prompt"].(string); ok {
			p = truncateRunes(p, 80, "...")
			return fmt.Sprintf("%q", p)
		}
	}
	return ""
}

// LastAssistantText returns the text of the most recent assistant message.
// Returns empty string if no assistant messages exist. Thread-safe.
func (s *AppState) LastAssistantText() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	msgs := s.Messages.Get()
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Kind == MsgAssistant {
			return msgs[i].Text
		}
	}
	return ""
}
