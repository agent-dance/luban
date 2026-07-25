package tui

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/observability"
	"github.com/agent-dance/luban/types"
)

var (
	ErrActivityScopeMismatch        = errors.New("activity does not belong to the active session epoch")
	ErrActivityStateOutcomeMismatch = errors.New("activity state/outcome mismatch")
)

type ActivityScope struct {
	SessionID string
	Epoch     uint64
}

type ActivityKind string

const (
	ActivityTool       ActivityKind = "tool"
	ActivityCommand    ActivityKind = "command"
	ActivityAgent      ActivityKind = "agent"
	ActivityBackground ActivityKind = "background"
	ActivityMCP        ActivityKind = "mcp"
	ActivityDecision   ActivityKind = "decision"
	ActivityHook       ActivityKind = "hook"
)

type ActivityPhase string

const (
	ActivityPhaseExecuting ActivityPhase = "executing"
	ActivityPhaseVerifying ActivityPhase = "verifying"
)

type ActivityState string

const (
	ActivitySpawning    ActivityState = "spawning"
	ActivityQueued      ActivityState = "queued"
	ActivityRunning     ActivityState = "running"
	ActivityWaiting     ActivityState = "waiting"
	ActivityBlocked     ActivityState = "blocked"
	ActivityNeedsInput  ActivityState = "needs_input"
	ActivityCompleted   ActivityState = "completed"
	ActivityFailed      ActivityState = "failed"
	ActivityCancelled   ActivityState = "cancelled"
	ActivityReadyReview ActivityState = "ready_for_review"
)

// ActivityLifecycle is the execution state of one activity run. Attention is
// tracked separately so a completed run can remain ready for review and a
// running or blocked run can request input without losing its lifecycle.
type ActivityLifecycle string

const (
	ActivityLifecycleSpawning  ActivityLifecycle = "spawning"
	ActivityLifecycleQueued    ActivityLifecycle = "queued"
	ActivityLifecycleRunning   ActivityLifecycle = "running"
	ActivityLifecycleWaiting   ActivityLifecycle = "waiting"
	ActivityLifecycleBlocked   ActivityLifecycle = "blocked"
	ActivityLifecycleCompleted ActivityLifecycle = "completed"
	ActivityLifecycleFailed    ActivityLifecycle = "failed"
	ActivityLifecycleCancelled ActivityLifecycle = "cancelled"
)

type ActivityAttentionKind string

const (
	// Empty attention means an event did not specify an attention update. The
	// explicit "none" value clears a previous attention state.
	ActivityAttentionNone           ActivityAttentionKind = "none"
	ActivityAttentionNeedsInput     ActivityAttentionKind = "needs_input"
	ActivityAttentionReadyForReview ActivityAttentionKind = "ready_for_review"
	ActivityAttentionWarning        ActivityAttentionKind = "warning"
	ActivityAttentionCritical       ActivityAttentionKind = "critical"
)

type ActivityAttentionSeverity string

const (
	ActivityAttentionSeverityInfo     ActivityAttentionSeverity = "info"
	ActivityAttentionSeverityWarning  ActivityAttentionSeverity = "warning"
	ActivityAttentionSeverityError    ActivityAttentionSeverity = "error"
	ActivityAttentionSeverityCritical ActivityAttentionSeverity = "critical"
)

type ActivityAttention struct {
	Kind       ActivityAttentionKind
	Severity   ActivityAttentionSeverity
	Unread     bool
	DecisionID string
	Message    string
}

type ActivityActionability string

const (
	ActivityActionDecision   ActivityActionability = "decision"
	ActivityActionProgress   ActivityActionability = "progress"
	ActivityActionTransition ActivityActionability = "transition"
)

type ActivityAction string

const (
	ActivityCancel      ActivityAction = "cancel"
	ActivityJump        ActivityAction = "jump"
	ActivityDetails     ActivityAction = "details"
	ActivityAcknowledge ActivityAction = "acknowledge"
)

type ActivityActor struct {
	ID   string
	Type string
}

type ActivityProgress struct {
	Current          int
	Total            int
	Message          string
	AgentID          string
	AgentType        string
	ParentToolUseID  string
	Phase            string
	LatestTool       string
	Output           string
	ElapsedMs        int64
	TokensUsed       int
	Provider         string
	Model            string
	Usage            *types.Usage
	LastRequestUsage *types.Usage
	DroppedCount     uint64
}

type ActivityControl struct {
	Cancelable bool
	JumpTarget string
	DetailRefs []DetailRef
}

type ActivityEvent struct {
	ID          string
	RunID       string
	Attempt     int
	BatchID     string
	ParentRunID string
	// SupersedesRunID links a retry to the immediately preceding run for the
	// same stable activity identity. History remains append-only while the
	// current-work projection can explain why an older failure is historical.
	SupersedesRunID string
	AgentPath       string
	SessionID       string
	Epoch           uint64
	TurnID          string
	WorkUnitID      string
	Actor           ActivityActor
	Kind            ActivityKind
	Name            string
	Phase           ActivityPhase
	// State is a reducer-derived presentation label. Producers and persisted
	// checkpoints use Lifecycle plus Attention as the typed source of truth.
	State     ActivityState `json:"-"`
	Lifecycle ActivityLifecycle
	Attention ActivityAttention
	Outcome   ObservationOutcome
	// Provisional marks a non-authoritative transport inference. A later typed
	// tool result for the same run may replace this terminal-looking state.
	Provisional bool
	Sequence    uint64
	// SourceSequence fences late producer events independently of Sequence,
	// which is owned by the presentation reducer and reflects receipt order.
	SourceSequence uint64
	Progress       ActivityProgress
	Control        ActivityControl
}

type Activity struct {
	ActivityEvent
	Actionability ActivityActionability
	Actions       []ActivityAction
	// PresentationWorkUnitID is the immutable historical label. WorkUnitID is
	// still remapped to the fork target for operational grouping and actions.
	PresentationWorkUnitID string
	OccurrenceCount        int
	FirstSequence          uint64
	LastSequence           uint64
	Acknowledged           bool
}

type ActivityCounts struct {
	Total       int
	Spawning    int
	Queued      int
	Running     int
	Waiting     int
	Blocked     int
	NeedsInput  int
	Failed      int
	Partial     int
	Denied      int
	TimedOut    int
	Cancelled   int
	Completed   int
	Orphan      int
	Unread      int
	ReadyReview int
}

type ActivitySnapshot struct {
	Activities []Activity
	Counts     ActivityCounts
}

type ActivityStore struct {
	mu           sync.RWMutex
	scope        ActivityScope
	byID         map[string]Activity
	historyOrder []string
}

func NewActivityStore(scope ActivityScope) *ActivityStore {
	return &ActivityStore{scope: scope, byID: make(map[string]Activity)}
}

// Restore installs persisted activity runs into the current session epoch.
// The sidecar epoch is intentionally ignored: a resume creates a new UI epoch
// while preserving stable activity/run identity and acknowledgement state.
func (s *ActivityStore) Restore(activities []Activity) (uint64, error) {
	if s == nil {
		return 0, fmt.Errorf("activity store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var maxSequence uint64
	for _, activity := range activities {
		event := cloneActivityEvent(activity.ActivityEvent)
		event.SessionID = s.scope.SessionID
		event.Epoch = s.scope.Epoch
		if event.Sequence == 0 || activity.OccurrenceCount <= 0 || activity.FirstSequence == 0 ||
			activity.LastSequence < activity.FirstSequence || event.Sequence != activity.LastSequence ||
			(event.RunID != "" && event.Attempt <= 0) {
			return maxSequence, i18n.NewError(i18n.KeyTUISessionViewInvalidCheckpoint)
		}
		normalized, err := normalizeActivityEvent(event)
		if err != nil {
			return maxSequence, err
		}
		activity.ActivityEvent = normalized
		if activity.Acknowledged {
			activity.Attention.Unread = false
		}
		key := activityStorageKey(activity.ID, activity.RunID)
		if existing, ok := s.byID[key]; ok {
			if existing.LastSequence > activity.LastSequence {
				continue
			}
		} else {
			s.historyOrder = append(s.historyOrder, key)
		}
		s.byID[key] = activity
		if activity.LastSequence > maxSequence {
			maxSequence = activity.LastSequence
		}
	}
	return maxSequence, nil
}

// ReconcileNonTerminal compares restored work with authoritative live runtime
// receipts. Persisted rows without a matching live run become historical
// orphans; session switches inside the same process can retain genuine live
// work by providing its stable ID and run ID.
func (s *ActivityStore) ReconcileNonTerminal(liveRuns map[string]string) int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	reconciled := 0
	for key, activity := range s.byID {
		if isTerminalActivityLifecycle(activity.Lifecycle) {
			continue
		}
		liveRun, live := liveRuns[activity.ID]
		if live && (liveRun == "" || activity.RunID == "" || liveRun == activity.RunID) {
			continue
		}
		activity.Lifecycle = ActivityLifecycleFailed
		activity.Outcome = OutcomeOrphan
		activity.Attention = ActivityAttention{Kind: ActivityAttentionNone}
		activity.State = activityPresentationState(activity.Lifecycle, activity.Attention)
		activity.Control.Cancelable = false
		activity.Actionability, activity.Actions = deriveActivityControls(activity.ActivityEvent)
		s.byID[key] = activity
		reconciled++
	}
	s.mu.Unlock()
	observability.RecordActivityOrphans(reconciled, observability.ActivitySourceRestoreReconcile)
	return reconciled
}

func (s *ActivityStore) Apply(event ActivityEvent) error {
	if event.SessionID != s.scope.SessionID || event.Epoch != s.scope.Epoch {
		observability.RecordActivityStaleDrop(observability.ActivitySourceScopeFence)
		return ErrActivityScopeMismatch
	}
	if event.ID == "" {
		return fmt.Errorf("activity has empty ID")
	}
	var err error
	event, err = normalizeActivityEvent(event)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	storageKey := activityStorageKey(event.ID, event.RunID)
	if existing, ok := s.byID[storageKey]; ok {
		if event.Sequence <= existing.Sequence {
			observability.RecordActivityStaleDrop(observability.ActivitySourceSequenceFence)
			return nil
		}
		if event.SourceSequence > 0 && existing.SourceSequence > 0 && event.SourceSequence <= existing.SourceSequence &&
			!activityLifecycleAdvances(existing.Lifecycle, event.Lifecycle) && !activityAttentionAdvances(existing.Attention, event.Attention) {
			observability.RecordActivityStaleDrop(observability.ActivitySourceSequenceFence)
			return nil
		}
		if isTerminalActivityLifecycle(existing.Lifecycle) {
			if existing.Provisional && !event.Provisional && event.Sequence > existing.LastSequence {
				replacement := mergeActivityEvent(existing.ActivityEvent, event)
				replacement.Lifecycle = event.Lifecycle
				replacement.State = event.State
				replacement.Outcome = event.Outcome
				replacement.Attention = event.Attention
				replacement.Control = event.Control
				replacement.Provisional = false
				existing.ActivityEvent = replacement
				existing.LastSequence = event.Sequence
				existing.Acknowledged = false
				existing.Actionability, existing.Actions = deriveActivityControls(replacement)
				s.byID[storageKey] = existing
				return nil
			}
			if event.Lifecycle != existing.Lifecycle || event.Sequence <= existing.LastSequence {
				observability.RecordActivityStaleDrop(observability.ActivitySourceTerminalFence)
				return nil
			}
			preserveAcknowledgedResult := existing.Acknowledged && existing.Lifecycle == ActivityLifecycleCompleted
			if existing.Lifecycle == ActivityLifecycleFailed || existing.Lifecycle == ActivityLifecycleCancelled {
				existing.OccurrenceCount++
			}
			existing.ActivityEvent = mergeTerminalActivityEvent(existing.ActivityEvent, event)
			existing.LastSequence = event.Sequence
			existing.Acknowledged = preserveAcknowledgedResult
			if preserveAcknowledgedResult {
				existing.Attention.Unread = false
			}
			existing.Actionability, existing.Actions = deriveActivityControls(existing.ActivityEvent)
			s.byID[storageKey] = existing
			return nil
		}
		existing.ActivityEvent = mergeActivityEvent(existing.ActivityEvent, event)
		existing.LastSequence = event.Sequence
		existing.Actionability, existing.Actions = deriveActivityControls(existing.ActivityEvent)
		s.byID[storageKey] = existing
		return nil
	}
	activity := Activity{ActivityEvent: cloneActivityEvent(event), OccurrenceCount: 1, FirstSequence: event.Sequence, LastSequence: event.Sequence}
	if activity.RunID != "" && activity.SupersedesRunID == "" {
		if previous, ok := s.latestActivityLocked(activity.ID); ok && previous.RunID != "" && previous.RunID != activity.RunID && activityRunIsLater(activity, previous) {
			activity.SupersedesRunID = previous.RunID
		}
	}
	if hasUnreadActivityAttention(event.Attention) {
		activity.Acknowledged = false
	}
	activity.Actionability, activity.Actions = deriveActivityControls(event)
	s.byID[storageKey] = activity
	s.historyOrder = append(s.historyOrder, storageKey)
	return nil
}

func mergeTerminalActivityEvent(existing, update ActivityEvent) ActivityEvent {
	merged := mergeActivityEvent(existing, update)
	merged.Control.Cancelable = false
	return merged
}

func mergeActivityEvent(existing, update ActivityEvent) ActivityEvent {
	merged := cloneActivityEvent(existing)
	merged.Sequence = update.Sequence
	if update.RunID != "" {
		merged.RunID = update.RunID
	}
	if merged.Attempt <= 0 && update.Attempt > 0 {
		merged.Attempt = update.Attempt
	}
	if update.BatchID != "" {
		merged.BatchID = update.BatchID
	}
	if update.ParentRunID != "" {
		merged.ParentRunID = update.ParentRunID
	}
	if update.SupersedesRunID != "" {
		merged.SupersedesRunID = update.SupersedesRunID
	}
	if update.AgentPath != "" {
		merged.AgentPath = update.AgentPath
	}
	if update.SessionID != "" {
		merged.SessionID = update.SessionID
	}
	if update.TurnID != "" {
		merged.TurnID = update.TurnID
	}
	if update.WorkUnitID != "" {
		merged.WorkUnitID = update.WorkUnitID
	}
	if update.Actor.ID != "" {
		merged.Actor.ID = update.Actor.ID
	}
	if update.Actor.Type != "" {
		merged.Actor.Type = update.Actor.Type
	}
	if update.Kind != "" {
		merged.Kind = update.Kind
	}
	if update.Name != "" {
		merged.Name = update.Name
	}
	if update.Phase != "" {
		merged.Phase = update.Phase
	}
	if update.Lifecycle != "" {
		merged.Lifecycle = update.Lifecycle
	}
	if update.Attention.Kind != "" {
		merged.Attention = update.Attention
	}
	if isTerminalActivityLifecycle(update.Lifecycle) {
		// A terminal transition resolves any prior needs-input/review attention
		// unless the terminal producer explicitly supplies new attention.
		merged.Attention = update.Attention
	}
	if update.Outcome != OutcomeUnknown {
		merged.Outcome = update.Outcome
	}
	if update.Provisional {
		merged.Provisional = true
	}
	if update.SourceSequence > 0 {
		merged.SourceSequence = update.SourceSequence
	}
	if update.Progress.Current != 0 {
		merged.Progress.Current = update.Progress.Current
	}
	if update.Progress.Total != 0 {
		merged.Progress.Total = update.Progress.Total
	}
	if update.Progress.Message != "" {
		merged.Progress.Message = update.Progress.Message
	}
	if update.Progress.AgentID != "" {
		merged.Progress.AgentID = update.Progress.AgentID
	}
	if update.Progress.AgentType != "" {
		merged.Progress.AgentType = update.Progress.AgentType
	}
	if update.Progress.ParentToolUseID != "" {
		merged.Progress.ParentToolUseID = update.Progress.ParentToolUseID
	}
	if update.Progress.Phase != "" {
		merged.Progress.Phase = update.Progress.Phase
	}
	if update.Progress.LatestTool != "" {
		merged.Progress.LatestTool = update.Progress.LatestTool
	}
	if update.Progress.Output != "" {
		merged.Progress.Output = update.Progress.Output
	}
	if update.Progress.ElapsedMs != 0 {
		merged.Progress.ElapsedMs = update.Progress.ElapsedMs
	}
	if update.Progress.TokensUsed != 0 {
		merged.Progress.TokensUsed = update.Progress.TokensUsed
	}
	if update.Progress.Provider != "" {
		merged.Progress.Provider = update.Progress.Provider
	}
	if update.Progress.Model != "" {
		merged.Progress.Model = update.Progress.Model
	}
	if update.Progress.Usage != nil {
		usage := *update.Progress.Usage
		merged.Progress.Usage = &usage
	}
	if update.Progress.LastRequestUsage != nil {
		usage := *update.Progress.LastRequestUsage
		merged.Progress.LastRequestUsage = &usage
	}
	if update.Progress.DroppedCount != 0 {
		merged.Progress.DroppedCount = update.Progress.DroppedCount
	}
	if update.Control.JumpTarget != "" {
		merged.Control.JumpTarget = update.Control.JumpTarget
	}
	for _, ref := range update.Control.DetailRefs {
		seen := false
		for _, current := range merged.Control.DetailRefs {
			if current.Source == ref.Source && current.Key == ref.Key && current.Digest == ref.Digest {
				seen = true
				break
			}
		}
		if !seen {
			merged.Control.DetailRefs = append(merged.Control.DetailRefs, ref)
		}
	}
	merged.Control.Cancelable = merged.Control.Cancelable || update.Control.Cancelable
	merged.State = activityPresentationState(merged.Lifecycle, merged.Attention)
	return merged
}

func (s *ActivityStore) AcknowledgeTerminal() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, activity := range s.byID {
		if isTerminalActivityLifecycle(activity.Lifecycle) {
			activity.Acknowledged = true
			if activity.Attention.Kind != "" && activity.Attention.Kind != ActivityAttentionNone {
				activity.Attention.Unread = false
			}
			s.byID[id] = activity
		}
	}
}

// AcknowledgeLatest marks the current projected run for an activity as reviewed.
func (s *ActivityStore) AcknowledgeLatest(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	activity, ok := s.latestActivityLocked(id)
	if !ok {
		return false
	}
	storageKey := activityStorageKey(activity.ID, activity.RunID)
	return s.acknowledgeLocked(storageKey, activity)
}

// AcknowledgeRun marks one exact retained run as reviewed.
func (s *ActivityStore) AcknowledgeRun(id, runID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	storageKey := activityStorageKey(id, runID)
	activity, ok := s.byID[storageKey]
	if !ok {
		return false
	}
	return s.acknowledgeLocked(storageKey, activity)
}

func (s *ActivityStore) acknowledgeLocked(storageKey string, activity Activity) bool {
	activity.Acknowledged = true
	activity.Attention.Unread = false
	activity.Actionability, activity.Actions = deriveActivityControls(activity.ActivityEvent)
	s.byID[storageKey] = activity
	return true
}

func (s *ActivityStore) Snapshot() ActivitySnapshot {
	s.mu.RLock()
	// The work view has one row per stable activity identity. Historical runs
	// remain addressable through GetRun but do not inflate current counts or
	// create duplicate focus IDs.
	latestByID := make(map[string]Activity, len(s.byID))
	for _, activity := range s.byID {
		activity.ActivityEvent = cloneActivityEvent(activity.ActivityEvent)
		activity.Actions = append([]ActivityAction(nil), activity.Actions...)
		current, ok := latestByID[activity.ID]
		if !ok || activityRunIsLater(activity, current) {
			latestByID[activity.ID] = activity
		}
	}
	s.mu.RUnlock()
	activities := make([]Activity, 0, len(latestByID))
	for _, activity := range latestByID {
		activities = append(activities, activity)
	}
	activities = projectPendingDecisionLifecycles(activities)
	sortActivitiesByWorkAndActor(activities)
	counts := ActivityCounts{Total: len(activities)}
	for _, activity := range activities {
		switch activity.Lifecycle {
		case ActivityLifecycleSpawning:
			counts.Spawning++
		case ActivityLifecycleQueued:
			counts.Queued++
		case ActivityLifecycleRunning:
			counts.Running++
		case ActivityLifecycleWaiting:
			counts.Waiting++
		case ActivityLifecycleBlocked:
			counts.Blocked++
		case ActivityLifecycleFailed:
			switch activity.Outcome {
			case OutcomePartial:
				counts.Partial++
			case OutcomeDenied:
				counts.Denied++
			case OutcomeOrphan:
				counts.Orphan++
			default:
				counts.Failed++
			}
		case ActivityLifecycleCancelled:
			if activity.Outcome == OutcomeTimedOut {
				counts.TimedOut++
			} else {
				counts.Cancelled++
			}
		case ActivityLifecycleCompleted:
			counts.Completed++
		}
		if activityHasUnreadAttention(activity) {
			counts.Unread++
			switch activity.Attention.Kind {
			case ActivityAttentionNeedsInput:
				counts.NeedsInput++
			case ActivityAttentionReadyForReview:
				counts.ReadyReview++
			}
		}
	}
	return ActivitySnapshot{Activities: activities, Counts: counts}
}

// projectPendingDecisionLifecycles keeps the durable execution lifecycle
// independent from a transient permission wait. A tool may still be alive in
// the runtime while its actor cannot execute it until the user decides; the
// user-facing snapshot must describe that interval as blocked, not running.
// Because this is a snapshot-only projection, resolving the decision reveals
// the latest underlying running or terminal lifecycle without manufacturing a
// second compatibility state or racing late runtime receipts.
func projectPendingDecisionLifecycles(activities []Activity) []Activity {
	pending := make([]Activity, 0)
	decisionByID := make(map[string]Activity, len(activities))
	for _, activity := range activities {
		if activity.Kind != ActivityDecision {
			continue
		}
		decisionID := strings.TrimPrefix(activity.ID, "decision:")
		if activity.Attention.DecisionID != "" {
			decisionID = activity.Attention.DecisionID
		}
		if decisionID != "" {
			decisionByID[decisionID] = activity
		}
		if activity.Lifecycle == ActivityLifecycleBlocked && activity.Outcome == OutcomeRunning &&
			activity.Attention.Kind == ActivityAttentionNeedsInput {
			pending = append(pending, activity)
		}
	}
	sort.Slice(pending, func(i, j int) bool {
		if pending[i].FirstSequence != pending[j].FirstSequence {
			return pending[i].FirstSequence < pending[j].FirstSequence
		}
		if pending[i].LastSequence != pending[j].LastSequence {
			return pending[i].LastSequence < pending[j].LastSequence
		}
		return pending[i].ID < pending[j].ID
	})

	for index := range activities {
		activity := &activities[index]
		if activity.Kind == ActivityDecision {
			if activity.Lifecycle == ActivityLifecycleBlocked && activity.Outcome == OutcomeRunning &&
				activity.Attention.Kind == ActivityAttentionNeedsInput {
				// OutcomeRunning is the durable non-terminal marker, not a claim
				// that execution is continuing behind the prompt. Suppress that
				// marker in the user-facing projection while input is required.
				activity.Outcome = OutcomeUnknown
			}
			continue
		}
		if isTerminalActivityLifecycle(activity.Lifecycle) {
			continue
		}
		decision, blocked := pendingDecisionForActivity(pending, *activity)
		if blocked {
			activity.Lifecycle = ActivityLifecycleBlocked
			activity.Outcome = OutcomeUnknown
			if activity.Kind == ActivityAgent || activity.Kind == ActivityBackground {
				decisionID := decision.Attention.DecisionID
				if decisionID == "" {
					decisionID = strings.TrimPrefix(decision.ID, "decision:")
				}
				message := decision.Attention.Message
				if message == "" {
					message = activity.Attention.Message
				}
				activity.Attention = ActivityAttention{
					Kind: ActivityAttentionNeedsInput, Severity: ActivityAttentionSeverityWarning,
					Unread: true, DecisionID: decisionID, Message: message,
				}
				activity.Acknowledged = false
			}
			activity.State = activityPresentationState(activity.Lifecycle, activity.Attention)
			activity.Actionability, activity.Actions = deriveActivityControls(activity.ActivityEvent)
			continue
		}

		// The renderer records the requesting agent as blocked so the state is
		// checkpointable. If the referenced decision is already terminal, hide
		// that stale intermediary projection until the renderer's restoration
		// event arrives in the same update batch.
		if activity.Lifecycle == ActivityLifecycleBlocked &&
			activity.Attention.Kind == ActivityAttentionNeedsInput && activity.Attention.DecisionID != "" {
			if decision, ok := decisionByID[activity.Attention.DecisionID]; ok && isTerminalActivityLifecycle(decision.Lifecycle) {
				activity.Lifecycle = ActivityLifecycleRunning
				activity.Attention = ActivityAttention{Kind: ActivityAttentionNone}
				activity.State = ActivityRunning
				activity.Actionability, activity.Actions = deriveActivityControls(activity.ActivityEvent)
			}
		}
	}
	return activities
}

func pendingDecisionForActivity(pending []Activity, activity Activity) (Activity, bool) {
	for _, decision := range pending {
		if activitiesShareDecisionScope(decision, activity) {
			return decision, true
		}
	}
	return Activity{}, false
}

// activitiesShareDecisionScope accepts partial runtime correlation metadata:
// every shared field must agree and at least one stable field must match. This
// covers direct tool adapters that omit actor identity without allowing a
// decision from a parallel actor or work unit to block unrelated work.
func activitiesShareDecisionScope(decision, activity Activity) bool {
	matched := false
	for _, pair := range [][2]string{
		{decision.Actor.ID, activity.Actor.ID},
		{decision.WorkUnitID, activity.WorkUnitID},
		{decision.TurnID, activity.TurnID},
	} {
		left, right := strings.TrimSpace(pair[0]), strings.TrimSpace(pair[1])
		if left == "" || right == "" {
			continue
		}
		if left != right {
			return false
		}
		matched = true
	}
	return matched
}

// AgentByCorrelation resolves the latest agent/background row for a decision
// or runtime event without conflating the independent decision activity with
// the agent's lifecycle row.
func (s *ActivityStore) AgentByCorrelation(actorID, workUnitID string) (Activity, bool) {
	if s == nil || strings.TrimSpace(actorID) == "" {
		return Activity{}, false
	}
	s.mu.RLock()
	var latest Activity
	found := false
	for _, activity := range s.byID {
		if activity.Kind != ActivityAgent && activity.Kind != ActivityBackground {
			continue
		}
		if activity.Actor.ID != actorID && strings.TrimPrefix(activity.ID, "background:") != actorID {
			continue
		}
		if workUnitID != "" && activity.WorkUnitID != "" && activity.WorkUnitID != workUnitID {
			continue
		}
		if !found || activityRunIsLater(activity, latest) {
			latest = activity
			found = true
		}
	}
	s.mu.RUnlock()
	if !found {
		return Activity{}, false
	}
	latest.ActivityEvent = cloneActivityEvent(latest.ActivityEvent)
	latest.Actions = append([]ActivityAction(nil), latest.Actions...)
	return latest, true
}

func activityLifecycleAdvances(existing, update ActivityLifecycle) bool {
	if update == "" || update == existing {
		return false
	}
	if isTerminalActivityLifecycle(update) && !isTerminalActivityLifecycle(existing) {
		return true
	}
	return activityLifecycleRank(update) > activityLifecycleRank(existing)
}

func activityLifecycleRank(lifecycle ActivityLifecycle) int {
	switch lifecycle {
	case ActivityLifecycleSpawning:
		return 1
	case ActivityLifecycleQueued:
		return 2
	case ActivityLifecycleRunning, ActivityLifecycleWaiting:
		return 3
	case ActivityLifecycleBlocked:
		return 4
	case ActivityLifecycleCompleted, ActivityLifecycleFailed, ActivityLifecycleCancelled:
		return 5
	default:
		return 0
	}
}

func activityAttentionAdvances(existing, update ActivityAttention) bool {
	if update.Kind == "" {
		return false
	}
	if update.Kind == ActivityAttentionNone {
		return existing.Kind != "" && existing.Kind != ActivityAttentionNone
	}
	return update.Unread && (update.Kind != existing.Kind || !existing.Unread || update.DecisionID != existing.DecisionID)
}

// RunHistory returns every retained run, including superseded attempts that
// Snapshot intentionally hides from the current work view.
func (s *ActivityStore) RunHistory() []Activity {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	out := make([]Activity, 0, len(s.byID))
	for _, key := range s.historyOrder {
		activity, ok := s.byID[key]
		if !ok {
			continue
		}
		activity.ActivityEvent = cloneActivityEvent(activity.ActivityEvent)
		activity.Actions = append([]ActivityAction(nil), activity.Actions...)
		out = append(out, activity)
	}
	s.mu.RUnlock()
	return out
}

// RunCount reports the retained run cardinality without allocating a snapshot.
func (s *ActivityStore) RunCount() int {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	count := len(s.byID)
	s.mu.RUnlock()
	return count
}

func (s *ActivityStore) Get(id string) (Activity, bool) {
	s.mu.RLock()
	activity, ok := s.latestActivityLocked(id)
	s.mu.RUnlock()
	if !ok {
		return Activity{}, false
	}
	activity.ActivityEvent = cloneActivityEvent(activity.ActivityEvent)
	activity.Actions = append([]ActivityAction(nil), activity.Actions...)
	return activity, true
}

// GetRun returns one exact retained activity run.
func (s *ActivityStore) GetRun(id, runID string) (Activity, bool) {
	s.mu.RLock()
	activity, ok := s.byID[activityStorageKey(id, runID)]
	s.mu.RUnlock()
	if !ok {
		return Activity{}, false
	}
	activity.ActivityEvent = cloneActivityEvent(activity.ActivityEvent)
	activity.Actions = append([]ActivityAction(nil), activity.Actions...)
	return activity, true
}

func (s *ActivityStore) latestActivityLocked(id string) (Activity, bool) {
	var latest Activity
	found := false
	for _, activity := range s.byID {
		if activity.ID != id {
			continue
		}
		if !found || activityRunIsLater(activity, latest) {
			latest = activity
			found = true
		}
	}
	return latest, found
}

func activityRunIsLater(candidate, current Activity) bool {
	if candidate.Attempt != current.Attempt {
		return candidate.Attempt > current.Attempt
	}
	if candidate.FirstSequence != current.FirstSequence {
		return candidate.FirstSequence > current.FirstSequence
	}
	if candidate.LastSequence != current.LastSequence {
		return candidate.LastSequence > current.LastSequence
	}
	return candidate.RunID > current.RunID
}

func activityStorageKey(id, runID string) string {
	if runID == "" {
		return id
	}
	return fmt.Sprintf("run:%d:%s:%d:%s", len(id), id, len(runID), runID)
}

func cloneActivityEvent(event ActivityEvent) ActivityEvent {
	event.Control.DetailRefs = append([]DetailRef(nil), event.Control.DetailRefs...)
	if event.Progress.Usage != nil {
		usage := *event.Progress.Usage
		event.Progress.Usage = &usage
	}
	if event.Progress.LastRequestUsage != nil {
		usage := *event.Progress.LastRequestUsage
		event.Progress.LastRequestUsage = &usage
	}
	return event
}

func normalizeActivityEvent(event ActivityEvent) (ActivityEvent, error) {
	if (event.RunID == "") != (event.Attempt == 0) || event.Attempt < 0 {
		return event, i18n.NewError(i18n.KeyTUIActivityRunAttemptInvalid, event.RunID, event.Attempt)
	}
	if event.Lifecycle == "" {
		return event, i18n.WrapInternalError(i18n.KeyTUIActivityStateLifecycleIncompatible, ErrActivityStateOutcomeMismatch, event.State, event.Lifecycle)
	}
	if event.Outcome != OutcomeUnknown && !activityLifecycleAcceptsOutcome(event.Lifecycle, event.Outcome) {
		return event, i18n.WrapInternalError(i18n.KeyTUIActivityStateOutcomeIncompatible, ErrActivityStateOutcomeMismatch, event.State, event.Outcome)
	}
	if event.Attention.Kind == "" || ((event.Lifecycle == ActivityLifecycleFailed || event.Lifecycle == ActivityLifecycleCancelled) && event.Attention.Kind == ActivityAttentionNone) {
		event.Attention = defaultActivityAttention(event.Lifecycle, event.Outcome)
	}
	event.State = activityPresentationState(event.Lifecycle, event.Attention)
	return event, nil
}

func activityLifecycleForOutcome(outcome ObservationOutcome) ActivityLifecycle {
	switch outcome {
	case OutcomeRunning:
		return ActivityLifecycleRunning
	case OutcomeSucceeded:
		return ActivityLifecycleCompleted
	case OutcomeCancelled, OutcomeTimedOut, OutcomeEscaped, OutcomeShutdown:
		return ActivityLifecycleCancelled
	case OutcomeFailed, OutcomePartial, OutcomeDenied, OutcomeOrphan, OutcomeConflict:
		return ActivityLifecycleFailed
	default:
		return ""
	}
}

func activityLifecycleAcceptsOutcome(lifecycle ActivityLifecycle, outcome ObservationOutcome) bool {
	switch outcome {
	case OutcomeRunning:
		return lifecycle == ActivityLifecycleSpawning || lifecycle == ActivityLifecycleQueued || lifecycle == ActivityLifecycleRunning || lifecycle == ActivityLifecycleWaiting || lifecycle == ActivityLifecycleBlocked
	case OutcomeSucceeded:
		return lifecycle == ActivityLifecycleCompleted
	case OutcomeCancelled, OutcomeTimedOut, OutcomeEscaped, OutcomeShutdown:
		return lifecycle == ActivityLifecycleCancelled
	case OutcomeFailed, OutcomePartial, OutcomeDenied, OutcomeOrphan, OutcomeConflict:
		return lifecycle == ActivityLifecycleFailed
	default:
		return true
	}
}

func defaultActivityAttention(lifecycle ActivityLifecycle, outcome ObservationOutcome) ActivityAttention {
	switch lifecycle {
	case ActivityLifecycleFailed:
		// Tool attempts are often recovered by a later model action. They remain
		// in history but do not become user-actionable merely because they ended
		// in failed/partial/denied state. Producers must explicitly request
		// attention for a failure that needs user action.
		return ActivityAttention{Kind: ActivityAttentionNone}
	case ActivityLifecycleCancelled:
		if outcome == OutcomeTimedOut || outcome == OutcomeShutdown {
			return ActivityAttention{Kind: ActivityAttentionWarning, Severity: ActivityAttentionSeverityError, Unread: true}
		}
		return ActivityAttention{Kind: ActivityAttentionNone}
	default:
		return ActivityAttention{}
	}
}

func activityPresentationState(lifecycle ActivityLifecycle, attention ActivityAttention) ActivityState {
	if attention.Kind == ActivityAttentionNeedsInput {
		return ActivityNeedsInput
	}
	if lifecycle == ActivityLifecycleCompleted && attention.Kind == ActivityAttentionReadyForReview {
		return ActivityReadyReview
	}
	switch lifecycle {
	case ActivityLifecycleSpawning:
		return ActivitySpawning
	case ActivityLifecycleQueued:
		return ActivityQueued
	case ActivityLifecycleRunning:
		return ActivityRunning
	case ActivityLifecycleWaiting:
		return ActivityWaiting
	case ActivityLifecycleBlocked:
		return ActivityBlocked
	case ActivityLifecycleCompleted:
		return ActivityCompleted
	case ActivityLifecycleFailed:
		return ActivityFailed
	case ActivityLifecycleCancelled:
		return ActivityCancelled
	default:
		return ""
	}
}

func isTerminalActivityLifecycle(lifecycle ActivityLifecycle) bool {
	return lifecycle == ActivityLifecycleCompleted || lifecycle == ActivityLifecycleFailed || lifecycle == ActivityLifecycleCancelled
}

func hasUnreadActivityAttention(attention ActivityAttention) bool {
	return attention.Kind != "" && attention.Kind != ActivityAttentionNone && attention.Unread
}

func activityHasUnreadAttention(activity Activity) bool {
	return hasUnreadActivityAttention(activity.Attention)
}

func sortActivitiesByWorkAndActor(activities []Activity) {
	workRanks := make(map[string]int)
	actorRanks := make(map[string]int)
	workCreated := make(map[string]uint64)
	actorCreated := make(map[string]uint64)
	for _, activity := range activities {
		rank := activitySortRank(activity)
		workKey := activityWorkGroupKey(activity)
		actorKey := workKey + "\x00" + activityActorGroupKey(activity)
		if current, ok := workRanks[workKey]; !ok || rank < current {
			workRanks[workKey] = rank
		}
		if current, ok := actorRanks[actorKey]; !ok || rank < current {
			actorRanks[actorKey] = rank
		}
		if current, ok := workCreated[workKey]; !ok || activity.FirstSequence < current {
			workCreated[workKey] = activity.FirstSequence
		}
		if current, ok := actorCreated[actorKey]; !ok || activity.FirstSequence < current {
			actorCreated[actorKey] = activity.FirstSequence
		}
	}
	sort.Slice(activities, func(i, j int) bool {
		leftWork, rightWork := activityWorkGroupKey(activities[i]), activityWorkGroupKey(activities[j])
		if workRanks[leftWork] != workRanks[rightWork] {
			return workRanks[leftWork] < workRanks[rightWork]
		}
		if workCreated[leftWork] != workCreated[rightWork] {
			return workCreated[leftWork] < workCreated[rightWork]
		}
		if leftWork != rightWork {
			return leftWork < rightWork
		}
		leftActor, rightActor := activityActorGroupKey(activities[i]), activityActorGroupKey(activities[j])
		leftActorKey, rightActorKey := leftWork+"\x00"+leftActor, rightWork+"\x00"+rightActor
		if actorRanks[leftActorKey] != actorRanks[rightActorKey] {
			return actorRanks[leftActorKey] < actorRanks[rightActorKey]
		}
		if actorCreated[leftActorKey] != actorCreated[rightActorKey] {
			return actorCreated[leftActorKey] < actorCreated[rightActorKey]
		}
		if leftActor != rightActor {
			return leftActor < rightActor
		}
		leftRank, rightRank := activitySortRank(activities[i]), activitySortRank(activities[j])
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if activities[i].FirstSequence != activities[j].FirstSequence {
			return activities[i].FirstSequence < activities[j].FirstSequence
		}
		if activities[i].Attempt != activities[j].Attempt {
			return activities[i].Attempt < activities[j].Attempt
		}
		if activities[i].ID != activities[j].ID {
			return activities[i].ID < activities[j].ID
		}
		return activities[i].RunID < activities[j].RunID
	})
}

func activityWorkGroupKey(activity Activity) string {
	if batchID := strings.TrimSpace(activity.BatchID); batchID != "" {
		return batchID
	}
	return strings.TrimSpace(activity.WorkUnitID)
}

func activityActorGroupKey(activity Activity) string {
	if path := strings.TrimSpace(activity.AgentPath); path != "" {
		return path
	}
	if id := strings.TrimSpace(activity.Actor.ID); id != "" {
		return id
	}
	return strings.TrimSpace(activity.Actor.Type)
}

func deriveActivityControls(event ActivityEvent) (ActivityActionability, []ActivityAction) {
	var actionability ActivityActionability
	switch {
	case event.Attention.Kind == ActivityAttentionNeedsInput:
		actionability = ActivityActionDecision
	case !isTerminalActivityLifecycle(event.Lifecycle):
		actionability = ActivityActionProgress
	default:
		actionability = ActivityActionTransition
	}
	var actions []ActivityAction
	if !isTerminalActivityLifecycle(event.Lifecycle) && event.Control.Cancelable {
		actions = append(actions, ActivityCancel)
	}
	if event.Control.JumpTarget != "" {
		actions = append(actions, ActivityJump)
	}
	if event.Kind != ActivityAgent && len(event.Control.DetailRefs) > 0 {
		actions = append(actions, ActivityDetails)
	}
	if hasUnreadActivityAttention(event.Attention) {
		actions = append(actions, ActivityAcknowledge)
	}
	return actionability, actions
}

func activitySortRank(activity Activity) int {
	if activityHasUnreadAttention(activity) {
		switch activity.Attention.Kind {
		case ActivityAttentionNeedsInput:
			return 0
		case ActivityAttentionReadyForReview:
			return 1
		case ActivityAttentionCritical:
			return 2
		case ActivityAttentionWarning:
			if activity.Lifecycle != ActivityLifecycleCancelled || activity.Outcome == OutcomeTimedOut || activity.Outcome == OutcomeShutdown {
				return 2
			}
		}
	}
	switch activity.Lifecycle {
	case ActivityLifecycleFailed:
		return 2
	case ActivityLifecycleSpawning, ActivityLifecycleQueued, ActivityLifecycleRunning, ActivityLifecycleWaiting, ActivityLifecycleBlocked:
		return 3
	case ActivityLifecycleCompleted:
		return 4
	case ActivityLifecycleCancelled:
		if activity.Outcome == OutcomeTimedOut || activity.Outcome == OutcomeShutdown {
			return 2
		}
		return 5
	default:
		return 5
	}
}
