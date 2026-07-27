package agent

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/runtime/loop"
	"github.com/agent-dance/luban/internal/runtime/skillauthority"
	runtimestore "github.com/agent-dance/luban/internal/store/runtime"
	"github.com/agent-dance/luban/internal/store/secureio"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

type agentRunRequest struct {
	prompt   string
	response chan agentRunResponse
	done     chan struct{}
	ctx      context.Context
}

type agentRunResponse struct {
	summary agentRunSummary
	err     error
}

type backgroundAgentSession struct {
	parent            context.Context
	cancel            context.CancelFunc
	loop              *loop.QueryLoop
	metadata          agentcontract.SessionMetadata
	cleanup           func()
	task              *BackgroundTask
	manager           *BackgroundTaskManager
	queue             chan agentRunRequest
	done              chan struct{}
	permissionHandler *agentPermissionSnapshotHandler
	progress          *AgentProgressEmitter
	metadataMu        sync.RWMutex
}

func agentRunLineage(ctx context.Context, agentID string) (batchID, parentRunID, agentPath string) {
	agentID = strings.TrimSpace(agentID)
	agentPath = agentID
	if ctx == nil {
		return "", "", agentPath
	}
	execContext, ok := executioncontract.ToolExecutionContextFromContext(ctx)
	if !ok {
		return "", "", agentPath
	}
	batchID = firstNonEmpty(execContext.BatchID, execContext.TurnID)
	parentRunID = strings.TrimSpace(execContext.RunID)
	parentPath := strings.Trim(strings.TrimSpace(execContext.AgentPath), "/")
	if parentPath == "" && strings.EqualFold(strings.TrimSpace(execContext.ActorType), "agent") {
		parentPath = strings.Trim(strings.TrimSpace(execContext.ActorID), "/")
	}
	if parentPath != "" {
		agentPath = parentPath + "/" + agentID
	}
	return batchID, parentRunID, agentPath
}

func agentRunID(agentID string, attempt int) string {
	if attempt <= 0 {
		attempt = 1
	}
	id := runtimestore.SafeTaskPathComponent(strings.TrimSpace(agentID))
	if id == "" {
		id = "agent"
	}
	return fmt.Sprintf("agent-run-%s-%d-%s", id, attempt, nextAgentTranscriptRunSuffix())
}

func (s *backgroundAgentSession) progressEmitterForRun(runID string, attempt int, batchID string) *AgentProgressEmitter {
	if s.progress == nil || s.progress.Closed() {
		metadata := s.metadataSnapshot()
		next := newAgentProgressEmitter(s.task.ID, firstNonEmpty(metadata.AgentType, "general-purpose"))
		if s.progress != nil {
			next.ConfigureCorrelation(s.progress.correlation())
		}
		s.progress = next
	}
	emitter := s.progress
	emitter.ConfigureRun(runID, attempt, batchID)
	emitter.SetObserver(func(event agentcontract.ProgressEvent) {
		s.recordAgentProgress(event)
	})
	return emitter
}

func correlatedAgentRunStart(progress *AgentProgressEmitter, agentID, agentType, runID string, attempt int, batchID string, startedAt time.Time) *agentcontract.ProgressEvent {
	if progress == nil {
		return nil
	}
	sessionID, turnID, workUnitID, parentToolUseID := progress.correlation()
	if strings.TrimSpace(parentToolUseID) == "" {
		return nil
	}
	return &agentcontract.ProgressEvent{
		AgentID:         strings.TrimSpace(agentID),
		AgentType:       strings.TrimSpace(agentType),
		SessionID:       sessionID,
		TurnID:          turnID,
		WorkUnitID:      workUnitID,
		ParentToolUseID: parentToolUseID,
		RunID:           strings.TrimSpace(runID),
		Attempt:         attempt,
		BatchID:         strings.TrimSpace(batchID),
		Phase:           agentcontract.ProgressStart,
		Timestamp:       startedAt,
	}
}

func (s *backgroundAgentSession) recordAgentProgress(event agentcontract.ProgressEvent) {
	if s == nil || s.task == nil || event.RunID == "" {
		return
	}
	s.task.mu.Lock()
	if s.task.CurrentRunID != event.RunID {
		s.task.mu.Unlock()
		return
	}
	if isTerminalAgentProgressPhase(event.Phase) {
		// runAgentQueryLoop emits its terminal progress in a defer before the
		// caller has committed result/outcome/status. Publishing that event as the
		// latest snapshot creates a same-source-sequence race where presentation
		// can observe "running + terminal progress" and then reject the real
		// terminal snapshot as a duplicate. Hold it until finishAgentRunLocked has
		// all terminal facts and persist them atomically.
		s.task.pendingTerminalProgress = cloneAgentProgressEvent(&event)
		s.task.mu.Unlock()
		return
	}
	if current := s.task.LatestProgress; current != nil && current.RunID == event.RunID {
		if event.SourceSequence <= current.SourceSequence {
			s.task.mu.Unlock()
			return
		}
		if strings.TrimSpace(event.PartialText) == "" && strings.TrimSpace(current.PartialText) != "" {
			// Tool-use and turn-boundary events carry fresh structured progress
			// but no assistant text. Preserve the most recent narrative tail so a
			// coalesced retained snapshot cannot make live output disappear.
			event.PartialText = current.PartialText
		}
	}
	s.task.LatestProgress = cloneAgentProgressEvent(&event)
	for index := len(s.task.Runs) - 1; index >= 0; index-- {
		if s.task.Runs[index].RunID != event.RunID {
			continue
		}
		s.task.Runs[index].LatestProgress = cloneAgentProgressEvent(&event)
		s.task.Runs[index].UpdatedAt = event.Timestamp
		s.task.Runs[index].Status = progressPhaseRunStatus(event.Phase, s.task.Runs[index].Status)
		break
	}
	record := s.task.recordLocked()
	s.task.mu.Unlock()
	if s.manager != nil {
		s.manager.persistRecordForTask(s.task, record)
	}
}

func isTerminalAgentProgressPhase(phase agentcontract.ProgressPhase) bool {
	return phase == agentcontract.ProgressCompleted || phase == agentcontract.ProgressError || phase == agentcontract.ProgressAborted
}

func consumeAgentTerminalProgressLocked(task *BackgroundTask, runID string) {
	if task == nil || task.pendingTerminalProgress == nil || task.pendingTerminalProgress.RunID != runID {
		return
	}
	task.LatestProgress = cloneAgentProgressEvent(task.pendingTerminalProgress)
	task.pendingTerminalProgress = nil
}

func progressPhaseRunStatus(phase agentcontract.ProgressPhase, fallback string) string {
	switch phase {
	case agentcontract.ProgressCompleted:
		return "completed"
	case agentcontract.ProgressError:
		return "failed"
	case agentcontract.ProgressAborted:
		return "cancelled"
	default:
		return fallback
	}
}

func finishAgentRunLocked(task *BackgroundTask, runID, status string, summary agentRunSummary, runError string, finishedAt time.Time) {
	if task == nil || runID == "" {
		return
	}
	duration := summary.TotalDuration
	if duration <= 0 && !task.StartedAt.IsZero() {
		duration = finishedAt.Sub(task.StartedAt).Milliseconds()
	}
	tokens := summary.TotalTokens
	transcriptPath := firstNonEmpty(summary.TranscriptPath, task.TranscriptPath)
	task.TranscriptPath = transcriptPath
	task.DurationMs = &duration
	task.TotalTokens = &tokens
	task.Usage = cloneUsagePointer(summary.Usage)
	outcome := summary.Outcome
	task.Outcome = outcome
	task.TerminalReason = strings.TrimSpace(summary.TerminalReason)
	task.ArtifactRefs = append([]string(nil), summary.ArtifactRefs...)
	task.VerificationRefs = append([]string(nil), summary.VerificationRefs...)
	consumeAgentTerminalProgressLocked(task, runID)
	for index := len(task.Runs) - 1; index >= 0; index-- {
		run := &task.Runs[index]
		if run.RunID != runID {
			continue
		}
		run.Status = status
		run.FinishedAt = cloneTimePointer(&finishedAt)
		run.UpdatedAt = finishedAt
		run.TranscriptPath = transcriptPath
		run.DurationMs = cloneInt64Pointer(&duration)
		run.TotalTokens = cloneIntPointer(&tokens)
		run.Usage = cloneUsagePointer(summary.Usage)
		run.Error = strings.TrimSpace(runError)
		run.Result = summary.Output
		run.Outcome = outcome
		run.TerminalReason = strings.TrimSpace(summary.TerminalReason)
		run.ToolUseCount = summary.ToolUseCount
		run.LatestToolUse = summary.LatestToolUse
		run.ArtifactRefs = append([]string(nil), summary.ArtifactRefs...)
		run.VerificationRefs = append([]string(nil), summary.VerificationRefs...)
		run.LatestProgress = cloneAgentProgressEvent(task.LatestProgress)
		return
	}
}

func agentTaskStatus(outcome agentcontract.RunOutcome) string {
	switch outcome {
	case agentcontract.RunOutcomeSucceeded:
		return "completed"
	case agentcontract.RunOutcomeCancelled:
		return "cancelled"
	default:
		return "failed"
	}
}

func agentRunTerminalErrorText(summary agentRunSummary, runErr error) string {
	if runErr != nil {
		return runErr.Error()
	}
	if outcomeErr := agentRunOutcomeError(summary); outcomeErr != nil {
		return outcomeErr.Error()
	}
	return ""
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (s *backgroundAgentSession) metadataSnapshot() agentcontract.SessionMetadata {
	if s == nil {
		return agentcontract.SessionMetadata{}
	}
	s.metadataMu.RLock()
	defer s.metadataMu.RUnlock()
	return cloneAgentSessionMetadata(s.metadata)
}

func (s *backgroundAgentSession) storeMetadata(metadata agentcontract.SessionMetadata) {
	if s == nil {
		return
	}
	s.metadataMu.Lock()
	s.metadata = cloneAgentSessionMetadata(metadata)
	s.metadataMu.Unlock()
}

// registerAgentSession prepares filesystem-backed output before registration,
// then optionally publishes the in-memory retained session through one short
// caller-owned authority lease. Persistence and goroutine startup happen only
// after that atomic registration succeeds.
func (m *BackgroundTaskManager) registerAgentSession(agentID, alias, prompt, description string, input agentcontract.Input, ql *loop.QueryLoop, metadata agentcontract.SessionMetadata, cleanup func(), progress *AgentProgressEmitter, lease func(func() error) error, launchContext context.Context) (*backgroundAgentSession, *agentcontract.TaskSnapshot, error) {
	if err := runtimestore.ValidateTaskID(agentID); err != nil {
		return nil, nil, err
	}
	origin := m.taskOriginForContext(launchContext)
	if launchContext != nil {
		pinBackgroundTaskOriginHookContext(origin, launchContext)
	}
	if err := secureio.EnsurePrivateRuntimeDirectory(origin.outputDir); err != nil {
		return nil, nil, i18n.WrapError(i18n.KeyToolAgentDeepOutputDirCreateFailed, err)
	}

	outputPath := origin.taskOutputPath(agentID)
	f, err := secureio.OpenPrivateRuntimeAppendFile(outputPath)
	if err != nil {
		return nil, nil, i18n.WrapError(i18n.KeyToolBackgroundOutputOpenFailed, err)
	}
	_ = f.Close()

	batchID, parentRunID, agentPath := agentRunLineage(launchContext, agentID)
	task := &BackgroundTask{
		ID:                     agentID,
		Type:                   agentcontract.TaskTypeLocalAgent,
		Status:                 "completed",
		Description:            description,
		Command:                description,
		Prompt:                 prompt,
		OutputPath:             outputPath,
		done:                   closedTaskDoneChannel(),
		AgentAlias:             strings.TrimSpace(alias),
		Detached:               input.RunInBackground,
		AgentInput:             &input,
		AgentMetadata:          &metadata,
		origin:                 origin,
		OwnerSessionID:         backgroundTaskOwnerSessionID(launchContext),
		OwnerSessionProjectDir: backgroundTaskOwnerSessionProjectDir(launchContext),
		OwnerProjectRoot:       origin.projectRoot,
		OwnerPID:               os.Getpid(),
		BatchID:                batchID,
		ParentRunID:            parentRunID,
		AgentPath:              agentPath,
		Outcome:                agentcontract.RunOutcomeSucceeded,
	}
	sessionCtx, sessionCancel := m.managedContext(context.Background())
	session := &backgroundAgentSession{
		parent:   sessionCtx,
		cancel:   sessionCancel,
		loop:     ql,
		metadata: metadata,
		cleanup:  cleanup,
		task:     task,
		manager:  m,
		progress: progress,
		queue:    make(chan agentRunRequest, 32),
		done:     make(chan struct{}),
	}

	commit := func() error {
		m.mu.Lock()
		if m.shuttingDown {
			m.mu.Unlock()
			return i18n.NewError(i18n.KeyToolNotificationManagerShuttingDown)
		}
		m.tasks[agentID] = task
		m.sessions[agentID] = session
		if m.trustedAgentResumes == nil {
			m.trustedAgentResumes = make(map[string]trustedAgentResumeContext)
		}
		m.trustedAgentResumes[agentID] = trustedAgentResumeContext{Input: input, Metadata: cloneAgentSessionMetadata(metadata)}
		if trimmed := strings.TrimSpace(alias); trimmed != "" {
			m.aliases[trimmed] = agentID
		}
		m.mu.Unlock()
		return nil
	}
	if lease != nil {
		if err := lease(commit); err != nil {
			sessionCancel()
			return nil, nil, err
		}
	} else if err := commit(); err != nil {
		sessionCancel()
		return nil, nil, err
	}
	m.persistTask(task)

	go session.serve()

	snap := task.snapshot()
	return session, &snap, nil
}

func (m *BackgroundTaskManager) registerAgentSessionFromRecord(record runtimestore.RuntimeTaskRecord, ql *loop.QueryLoop, metadata agentcontract.SessionMetadata, cleanup func(), progress *AgentProgressEmitter) (*backgroundAgentSession, *agentcontract.TaskSnapshot, error) {
	agentID := strings.TrimSpace(record.ID)
	if agentID == "" {
		return nil, nil, i18n.NewError(i18n.KeyToolAgentDeepSessionRecordIDMissing)
	}
	if err := runtimestore.ValidateTaskID(agentID); err != nil {
		return nil, nil, err
	}
	origin := m.currentTaskOrigin()
	if err := secureio.EnsurePrivateRuntimeDirectory(origin.outputDir); err != nil {
		return nil, nil, i18n.WrapError(i18n.KeyToolAgentDeepOutputDirCreateFailed, err)
	}
	// OutputPath comes from durable JSON and is therefore untrusted on restore.
	// Recompute the only authorized path from the pinned origin and validated ID.
	outputPath := origin.taskOutputPath(agentID)
	f, err := secureio.OpenPrivateRuntimeAppendFile(outputPath)
	if err != nil {
		return nil, nil, i18n.WrapError(i18n.KeyToolBackgroundOutputOpenFailed, err)
	}
	_ = f.Close()

	status := strings.TrimSpace(record.Status)
	if status == "" || status == "running" {
		return nil, nil, retainedAgentOwnerMismatchError()
	}
	input := record.AgentInput
	if input == nil {
		return nil, nil, i18n.NewError(i18n.KeyToolAgentDeepSessionMissingInput, agentID)
	}
	task := &BackgroundTask{
		ID:                     agentID,
		Type:                   agentcontract.TaskTypeLocalAgent,
		Status:                 status,
		Description:            record.Description,
		Command:                record.Command,
		Prompt:                 record.Prompt,
		OutputPath:             outputPath,
		ExitCode:               record.ExitCode,
		Error:                  record.Error,
		Result:                 record.Result,
		StartedAt:              record.StartedAt,
		FinishedAt:             record.FinishedAt,
		done:                   closedTaskDoneChannel(),
		AgentAlias:             strings.TrimSpace(record.AgentAlias),
		Detached:               runtimeTaskRecordDetached(record),
		AgentInput:             input,
		AgentMetadata:          &metadata,
		AgentMessages:          append([]types.Message(nil), record.AgentMessages...),
		OwnerSessionID:         record.OwnerSessionID,
		OwnerSessionProjectDir: record.OwnerSessionProjectDir,
		OwnerProjectRoot:       record.OwnerProjectRoot,
		OwnerPID:               os.Getpid(),
		CurrentRunID:           record.CurrentRunID,
		Attempt:                record.Attempt,
		BatchID:                record.BatchID,
		ParentRunID:            record.ParentRunID,
		AgentPath:              record.AgentPath,
		QueuedPrompts:          record.QueuedPrompts,
		QueueReason:            record.QueueReason,
		Runs:                   cloneRuntimeTaskRunRecords(record.Runs),
		LatestProgress:         cloneAgentProgressEvent(record.LatestProgress),
		TranscriptPath:         record.TranscriptPath,
		DurationMs:             cloneInt64Pointer(record.DurationMs),
		TotalTokens:            cloneIntPointer(record.TotalTokens),
		Usage:                  cloneUsagePointer(record.Usage),
		Outcome:                record.Outcome,
		TerminalReason:         record.TerminalReason,
		ArtifactRefs:           append([]string(nil), record.ArtifactRefs...),
		VerificationRefs:       append([]string(nil), record.VerificationRefs...),
		origin:                 origin,
	}
	sessionCtx, sessionCancel := m.managedContext(context.Background())
	session := &backgroundAgentSession{
		parent:   sessionCtx,
		cancel:   sessionCancel,
		loop:     ql,
		metadata: metadata,
		cleanup:  cleanup,
		task:     task,
		manager:  m,
		progress: progress,
		queue:    make(chan agentRunRequest, 32),
		done:     make(chan struct{}),
	}

	m.mu.Lock()
	if m.shuttingDown {
		m.mu.Unlock()
		sessionCancel()
		return nil, nil, i18n.NewError(i18n.KeyToolNotificationManagerShuttingDown)
	}
	m.tasks[agentID] = task
	m.sessions[agentID] = session
	if m.trustedAgentResumes == nil {
		m.trustedAgentResumes = make(map[string]trustedAgentResumeContext)
	}
	m.trustedAgentResumes[agentID] = trustedAgentResumeContext{Input: *input, Metadata: cloneAgentSessionMetadata(metadata)}
	if task.AgentAlias != "" {
		m.aliases[task.AgentAlias] = agentID
	}
	m.mu.Unlock()
	m.persistTask(task)

	go session.serve()

	snap := task.snapshot()
	return session, &snap, nil
}

func (m *BackgroundTaskManager) ResolveAgentTarget(target string) (agentcontract.TaskSnapshot, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	agentID := strings.TrimSpace(target)
	if aliasID, ok := m.aliases[agentID]; ok {
		agentID = aliasID
	}
	task, ok := m.tasks[agentID]
	if !ok || task.Type != agentcontract.TaskTypeLocalAgent {
		return agentcontract.TaskSnapshot{}, false
	}
	return task.snapshot(), true
}

// AbortAgent cancels an in-process teammate agent's run context so its event
// loop unwinds at the next safe point. SM-09: the TS reference performs the
// equivalent of abortController.abort() through findTeammateTaskByAgentId; the
// approved-shutdown path in send_message_routing.go invokes this after writing
// the shutdown_approved envelope so the agent actually stops running.
//
// Returns true when an in-process session was found and the cancel was
// dispatched; false when the target is unknown, already-finished, or owned by
// another process (out-of-process teammates rely on the cooperative
// setTeamMemberActive(false) flag instead).
func (m *BackgroundTaskManager) AbortAgent(target string) bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	agentID := strings.TrimSpace(target)
	if aliasID, ok := m.aliases[agentID]; ok {
		agentID = aliasID
	}
	task, taskOK := m.tasks[agentID]
	session := m.sessions[agentID]
	m.mu.Unlock()

	if !taskOK || task.Type != agentcontract.TaskTypeLocalAgent {
		return false
	}

	task.mu.Lock()
	cancel := task.cancel
	if task.Status == "running" {
		task.Status = "killed"
		code := -1
		task.ExitCode = &code
		finishedAt := time.Now().UTC()
		task.FinishedAt = &finishedAt
	}
	task.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if session != nil {
		// Wake the session so its serve loop exits the queue waiter and the
		// running handleRequest unwinds via runCtx cancellation.
		session.cancel()
	}
	return true
}

func (m *BackgroundTaskManager) retireAgentSession(agentID string, session *backgroundAgentSession) {
	if m == nil || session == nil {
		return
	}
	m.mu.Lock()
	if m.sessions[agentID] == session {
		delete(m.sessions, agentID)
	}
	m.mu.Unlock()
	session.cancel()
}

// queueAgentPromptWithAuthority resumes a retained agent under the model run's
// pinned project-generation lease. The final durable owner check, approval
// detachment, queue publication, and their task-record writes are one fenced
// commit: a workspace retarget cannot slip between validation and enqueue.
//
// The lease is deliberately limited to local restore/queue publication. It is
// released before the retained QueryLoop makes any provider call.
func (m *BackgroundTaskManager) queueAgentPromptWithAuthority(ctx context.Context, target, prompt string, authority skillauthority.Authority, manager *skills.Manager) (snap agentcontract.TaskSnapshot, handled bool, err error) {
	if manager == nil || !authority.Enabled() {
		if task, session := m.inMemoryAgentPromptTarget(target); task != nil && session != nil {
			return agentcontract.TaskSnapshot{}, true, retainedAgentOwnerMismatchError()
		}
		if _, found := m.findStoredAgentRecord(target); found {
			return agentcontract.TaskSnapshot{}, true, retainedAgentOwnerMismatchError()
		}
		return agentcontract.TaskSnapshot{}, false, nil
	}
	task, session, handled, err := m.resolveAgentPromptTarget(ctx, target, &authority)
	if !handled || err != nil {
		return agentcontract.TaskSnapshot{}, handled, err
	}
	err = authority.WithGenerationLease(manager, func() error {
		// Alias maps and restoration can change while preparation is in flight.
		// Re-resolve under the generation lease and publish only to the exact
		// retained objects that passed the owner check above.
		finalTask, finalSession := m.inMemoryAgentPromptTarget(target)
		if finalTask == nil || finalSession == nil || finalTask != task || finalSession != session {
			return retainedAgentOwnerMismatchError()
		}
		if ownerErr := validateRetainedAgentRequestOwner(ctx, retainedAgentOwnerFromTask(finalTask), &authority); ownerErr != nil {
			return ownerErr
		}
		var commitErr error
		snap, commitErr = publishRetainedAgentPrompt(ctx, finalTask, finalSession, prompt)
		return commitErr
	})
	if err != nil {
		// A stale model run must not fall through and reinterpret the same target
		// as a team mailbox recipient after its retained-agent commit is fenced.
		return agentcontract.TaskSnapshot{}, true, err
	}
	return snap, handled, nil
}

func (m *BackgroundTaskManager) inMemoryAgentPromptTarget(target string) (*BackgroundTask, *backgroundAgentSession) {
	m.mu.Lock()
	defer m.mu.Unlock()
	agentID := strings.TrimSpace(target)
	if aliasID, ok := m.aliases[agentID]; ok {
		agentID = aliasID
	}
	task, ok := m.tasks[agentID]
	session := m.sessions[agentID]
	if !ok || task == nil || session == nil || task.Type != agentcontract.TaskTypeLocalAgent {
		return nil, nil
	}
	return task, session
}

func (m *BackgroundTaskManager) resolveAgentPromptTarget(ctx context.Context, target string, authority *skillauthority.Authority) (*BackgroundTask, *backgroundAgentSession, bool, error) {
	task, session := m.inMemoryAgentPromptTarget(target)

	if task != nil {
		if err := validateRetainedAgentRequestOwner(ctx, retainedAgentOwnerFromTask(task), authority); err != nil {
			return nil, nil, true, err
		}
		return task, session, true, nil
	}
	restoredTask, restoredSession, handled, err := m.restoreStoredAgentSession(ctx, strings.TrimSpace(target), authority)
	if !handled || err != nil {
		return nil, nil, handled, err
	}

	if err := validateRetainedAgentRequestOwner(ctx, retainedAgentOwnerFromTask(restoredTask), authority); err != nil {
		return nil, nil, true, err
	}
	return restoredTask, restoredSession, true, nil
}

func publishRetainedAgentPrompt(ctx context.Context, task *BackgroundTask, session *backgroundAgentSession, prompt string) (agentcontract.TaskSnapshot, error) {
	if task == nil || session == nil {
		return agentcontract.TaskSnapshot{}, retainedAgentOwnerMismatchError()
	}
	session.detachApprovalRouting()
	snap := task.snapshot()
	if err := session.enqueueWithContext(ctx, prompt, nil); err != nil {
		return agentcontract.TaskSnapshot{}, err
	}
	return snap, nil
}

type retainedAgentOwner struct {
	sessionID         string
	sessionProjectDir string
	projectRoot       string
	skillGeneration   skills.ProjectSourceGeneration
}

func retainedAgentOwnerFromTask(task *BackgroundTask) retainedAgentOwner {
	if task == nil {
		return retainedAgentOwner{}
	}
	task.mu.RLock()
	defer task.mu.RUnlock()
	owner := retainedAgentOwner{
		sessionID:         strings.TrimSpace(task.OwnerSessionID),
		sessionProjectDir: strings.TrimSpace(task.OwnerSessionProjectDir),
		projectRoot:       task.OwnerProjectRoot,
	}
	if task.AgentMetadata != nil {
		owner.skillGeneration = skills.ProjectSourceGeneration(task.AgentMetadata.SkillProjectGeneration)
	}
	return owner
}

func retainedAgentOwnerFromRecord(record runtimestore.RuntimeTaskRecord) retainedAgentOwner {
	owner := retainedAgentOwner{
		sessionID:         strings.TrimSpace(record.OwnerSessionID),
		sessionProjectDir: strings.TrimSpace(record.OwnerSessionProjectDir),
		projectRoot:       record.OwnerProjectRoot,
	}
	if record.AgentMetadata != nil {
		owner.skillGeneration = skills.ProjectSourceGeneration(record.AgentMetadata.SkillProjectGeneration)
	}
	return owner
}

// validateRetainedAgentRequestOwner treats a loop-owned execution context as
// an unforgeable request owner. A retained agent belongs to the exact parent
// session, durable session namespace, project root, and skill-source generation
// that registered it. A foreground A -> B -> A switch therefore cannot make an
// old in-memory ID or alias reachable merely because its path looks familiar.
//
// Missing or expired private provenance and incomplete owner metadata fail
// closed instead of falling back to exported identity strings.
func validateRetainedAgentRequestOwner(ctx context.Context, owner retainedAgentOwner, authority *skillauthority.Authority) error {
	if authority == nil || !authority.Enabled() {
		return retainedAgentOwnerMismatchError()
	}
	if err := authority.ValidateContext(ctx); err != nil {
		return err
	}
	return authority.ValidateOwner(owner.sessionID, owner.sessionProjectDir, owner.projectRoot, owner.skillGeneration)
}

func retainedAgentOwnerMismatchError() error {
	return i18n.WrapInternalError(
		i18n.KeyLoopQueryValidateSkillGenerationFailed,
		skills.ErrSkillProjectGenerationChanged,
	)
}

// detachApprovalRouting moves a retained agent onto the non-interactive
// background route before another prompt can start. The immutable permission
// snapshot is cloned unchanged; only the presentation route is removed. The
// task record is persisted before enqueue so a concurrent restart cannot
// restore the stale attached route.
func (s *backgroundAgentSession) detachApprovalRouting() {
	if s == nil {
		return
	}
	if s.permissionHandler != nil {
		s.permissionHandler.setApprovalRouting(agentcontract.ApprovalFailClosed, "")
	}
	if s.task == nil {
		return
	}

	metadata := s.metadataSnapshot()
	metadata.ApprovalRouting = agentcontract.ApprovalFailClosed
	metadata.PresentationSessionID = ""
	s.storeMetadata(metadata)

	s.task.mu.Lock()
	taskMetadata := cloneAgentSessionMetadata(metadata)
	if s.task.AgentMetadata != nil {
		taskMetadata = cloneAgentSessionMetadata(*s.task.AgentMetadata)
		taskMetadata.ApprovalRouting = agentcontract.ApprovalFailClosed
		taskMetadata.PresentationSessionID = ""
	}
	s.task.AgentMetadata = &taskMetadata
	record := s.task.recordLocked()
	s.task.mu.Unlock()

	if s.manager != nil {
		s.manager.updateTrustedAgentMetadata(s.task.ID, metadata)
		s.manager.persistRecordForTask(s.task, record)
	}
}

func (m *BackgroundTaskManager) restoreStoredAgentSession(ctx context.Context, target string, authority *skillauthority.Authority) (*BackgroundTask, *backgroundAgentSession, bool, error) {
	record, ok := m.findStoredAgentRecord(target)
	if !ok {
		return nil, nil, false, nil
	}
	if record.Type != agentcontract.TaskTypeLocalAgent {
		return nil, nil, false, nil
	}
	if record.Status == "running" {
		return nil, nil, true, i18n.NewError(i18n.KeyToolAgentDeepSessionRunningElsewhere, target)
	}
	if err := validateRetainedAgentRequestOwner(ctx, retainedAgentOwnerFromRecord(record), authority); err != nil {
		return nil, nil, true, err
	}

	m.mu.Lock()
	restorer := m.agentSessionRestorer
	m.mu.Unlock()
	if restorer == nil {
		return nil, nil, true, i18n.NewError(i18n.KeyToolAgentDeepSessionRestoreUnsupported, target)
	}

	if err := restorer(record.ID, record); err != nil {
		return nil, nil, true, err
	}
	m.mu.Lock()
	session := m.sessions[record.ID]
	m.mu.Unlock()
	if session == nil || session.task == nil {
		return nil, nil, true, i18n.NewError(i18n.KeyToolAgentDeepSessionRestoreEmpty, target)
	}
	return session.task, session, true, nil
}

func (m *BackgroundTaskManager) findStoredAgentRecord(target string) (runtimestore.RuntimeTaskRecord, bool) {
	trimmed := strings.TrimSpace(target)
	origin := m.currentTaskOrigin()
	if trimmed == "" || origin == nil || origin.store == nil {
		return runtimestore.RuntimeTaskRecord{}, false
	}
	if record, ok := origin.store.Get(trimmed); ok {
		return record, true
	}
	return origin.store.FindLocalAgentByAlias(trimmed)
}

func (s *backgroundAgentSession) runSync(ctx context.Context, prompt string) (agentRunSummary, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	response := make(chan agentRunResponse, 1)
	if err := s.enqueueWithContext(ctx, prompt, response); err != nil {
		return agentRunSummary{}, err
	}

	select {
	case <-ctx.Done():
		return agentRunSummary{}, ctx.Err()
	case <-s.parent.Done():
		return agentRunSummary{}, i18n.WrapError(i18n.KeyToolAgentDeepSessionUnavailable, ErrAgentRunInterrupted)
	case result := <-response:
		return result.summary, result.err
	}
}

func (s *backgroundAgentSession) enqueue(prompt string, response chan agentRunResponse) error {
	return s.enqueueWithContext(context.Background(), prompt, response)
}

func (s *backgroundAgentSession) enqueueWithContext(ctx context.Context, prompt string, response chan agentRunResponse) error {
	if strings.TrimSpace(prompt) == "" {
		return i18n.NewError(i18n.KeyToolAgentDeepPromptEmpty)
	}
	if ctx == nil {
		ctx = context.Background()
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.parent.Done():
		return i18n.WrapError(i18n.KeyToolAgentDeepSessionUnavailable, ErrAgentRunInterrupted)
	default:
	}

	var done chan struct{}
	s.task.mu.Lock()
	if s.task.Status != "running" {
		done = make(chan struct{})
		s.task.Status = "running"
		s.task.Error = ""
		s.task.Result = ""
		s.task.ExitCode = nil
		s.task.FinishedAt = nil
		s.task.StartedAt = time.Now().UTC()
		s.task.Prompt = prompt
		s.task.done = done
		s.task.Outcome = agentcontract.RunOutcomeRunning
		s.task.TerminalReason = ""
	}
	s.task.QueuedPrompts++
	updateAgentQueueReasonLocked(s.task)
	record := s.task.recordLocked()
	s.task.mu.Unlock()
	s.manager.persistRecordForTask(s.task, record)

	select {
	case <-ctx.Done():
		s.failUnqueuedRequest(done, ctx.Err())
		return ctx.Err()
	case <-s.parent.Done():
		runErr := i18n.WrapError(i18n.KeyToolAgentDeepSessionUnavailable, ErrAgentRunInterrupted)
		s.failUnqueuedRequest(done, runErr)
		return runErr
	case s.queue <- agentRunRequest{prompt: prompt, response: response, done: done, ctx: ctx}:
		return nil
	}
}

func (s *backgroundAgentSession) failUnqueuedRequest(done chan struct{}, runErr error) {
	s.task.mu.Lock()
	dequeueAgentPromptLocked(s.task)
	if done != nil && s.task.done == done && s.task.Status == "running" {
		outcome, reason := classifyAgentRunTermination(runErr, false)
		s.task.Status = agentTaskStatus(outcome)
		s.task.Outcome = outcome
		s.task.TerminalReason = reason
		s.task.Error = runErr.Error()
		code := -1
		s.task.ExitCode = &code
		finishedAt := time.Now().UTC()
		s.task.FinishedAt = &finishedAt
	}
	record := s.task.recordLocked()
	s.task.mu.Unlock()
	s.manager.persistRecordForTask(s.task, record)
	if done != nil {
		close(done)
	}
}

func (s *backgroundAgentSession) serve() {
	defer func() {
		runAgentCleanup(s.cleanup)
		close(s.done)
	}()
	for {
		select {
		case <-s.parent.Done():
			return
		case request := <-s.queue:
			s.handleRequest(request)
		}
	}
}

func (s *backgroundAgentSession) handleRequest(request agentRunRequest) {
	if request.ctx == nil {
		request.ctx = context.Background()
	}
	if err := request.ctx.Err(); err != nil {
		s.failQueuedRequest(request, err)
		return
	}

	runCtx, cancel := context.WithCancel(s.parent)
	stopRequestCancel := context.AfterFunc(request.ctx, cancel)
	defer stopRequestCancel()
	done := request.done
	if done == nil {
		done = make(chan struct{})
	}

	requestBatchID, requestParentRunID, requestAgentPath, hasRequestLineage := agentRunLineageForRequest(request.ctx, s.task.ID)
	s.task.mu.Lock()
	dequeueAgentPromptLocked(s.task)
	if s.task.Status == "killed" {
		code := -1
		s.task.ExitCode = &code
		finishedAt := time.Now().UTC()
		s.task.FinishedAt = &finishedAt
		s.task.cancel = nil
		record := s.task.recordLocked()
		s.task.mu.Unlock()
		s.manager.persistRecordForTask(s.task, record)
		if s.progress != nil {
			s.progress.Finish(agentcontract.ProgressAborted, toolRuntimeText(i18n.KeyToolAgentDeepTaskKilledBeforeStart))
		}
		if request.response == nil {
			s.manager.emitTaskCompletionNotification(s.manager.completionContextForTask(s.task), s.task, "killed", -1)
		}
		cancel()
		close(done)
		if request.response != nil {
			request.response <- agentRunResponse{err: i18n.NewError(i18n.KeyToolAgentDeepTaskKilledBeforeStart)}
		}
		return
	}
	s.task.Status = "running"
	s.task.Error = ""
	s.task.Result = ""
	s.task.ExitCode = nil
	s.task.FinishedAt = nil
	s.task.DurationMs = nil
	s.task.TotalTokens = nil
	s.task.Usage = nil
	s.task.Outcome = agentcontract.RunOutcomeRunning
	s.task.TerminalReason = ""
	s.task.ArtifactRefs = nil
	s.task.VerificationRefs = nil
	s.task.pendingTerminalProgress = nil
	startedAt := time.Now().UTC()
	s.task.StartedAt = startedAt
	s.task.Prompt = request.prompt
	s.task.cancel = cancel
	s.task.done = done
	if hasRequestLineage {
		s.task.BatchID = requestBatchID
		s.task.ParentRunID = requestParentRunID
		s.task.AgentPath = requestAgentPath
	}
	updateAgentQueueReasonLocked(s.task)
	s.task.Attempt++
	if s.task.Attempt <= 0 {
		s.task.Attempt = 1
	}
	runID := agentRunID(s.task.ID, s.task.Attempt)
	s.task.CurrentRunID = runID
	s.task.TranscriptPath = agentTranscriptPathForRun(runID)
	agentType := ""
	if s.task.AgentMetadata != nil {
		agentType = s.task.AgentMetadata.AgentType
	}
	if agentType == "" && s.task.AgentInput != nil {
		agentType = s.task.AgentInput.SubagentType
	}
	s.task.LatestProgress = correlatedAgentRunStart(s.progress, s.task.ID, agentType, runID, s.task.Attempt, s.task.BatchID, startedAt)
	s.task.Runs = append(s.task.Runs, agentcontract.RunRecord{
		RunID: runID, Attempt: s.task.Attempt, BatchID: s.task.BatchID,
		ParentRunID: s.task.ParentRunID, AgentPath: s.task.AgentPath,
		Status: "running", Outcome: agentcontract.RunOutcomeRunning, Prompt: request.prompt, StartedAt: startedAt,
		UpdatedAt: startedAt, TranscriptPath: s.task.TranscriptPath, LatestProgress: cloneAgentProgressEvent(s.task.LatestProgress),
	})
	attempt := s.task.Attempt
	batchID := s.task.BatchID
	parentRunID := s.task.ParentRunID
	agentPath := s.task.AgentPath
	record := s.task.recordLocked()
	s.task.mu.Unlock()
	emitter := s.progressEmitterForRun(runID, attempt, batchID)
	s.manager.persistRecordForTask(s.task, record)

	outFile, err := secureio.OpenPrivateRuntimeAppendFile(s.task.OutputPath)
	if err != nil {
		cancel()
		outputErr := i18n.WrapError(i18n.KeyToolBackgroundOutputOpenFailed, err)
		if emitter != nil {
			emitter.Finish(agentcontract.ProgressError, outputErr.Error())
		}
		s.task.mu.Lock()
		s.task.Status = "failed"
		s.task.Error = outputErr.Error()
		code := -1
		s.task.ExitCode = &code
		finishedAt := time.Now().UTC()
		s.task.FinishedAt = &finishedAt
		s.task.cancel = nil
		finishAgentRunLocked(s.task, runID, "failed", agentRunSummary{
			TranscriptPath: s.task.TranscriptPath, Outcome: agentcontract.RunOutcomeFailed, TerminalReason: "output_open_failed",
		}, s.task.Error, finishedAt)
		record := s.task.recordLocked()
		s.task.mu.Unlock()
		s.manager.persistRecordForTask(s.task, record)
		if request.response == nil {
			s.manager.emitTaskCompletionNotification(s.manager.completionContextForTask(s.task), s.task, "failed", -1)
		}
		close(done)
		if request.response != nil {
			request.response <- agentRunResponse{err: outputErr}
		}
		return
	}

	transcriptWriter, closeTranscript := openAgentTranscriptWriterForRunIdentity(runID, agentTranscriptIdentity{
		SessionID:    firstNonEmpty(s.task.OwnerSessionID, backgroundTaskOwnerSessionID(request.ctx)),
		ProjectRoot:  s.task.OwnerProjectRoot,
		ContextEpoch: agentTranscriptContextEpoch(request.ctx),
		ActorID:      s.task.ID, ActorType: "agent", RunID: runID,
	})
	if closeTranscript != nil {
		defer closeTranscript()
	}
	metadata := s.metadataSnapshot()
	runCtx = executioncontract.WithToolExecutionContext(runCtx, executioncontract.ToolExecutionContext{
		SessionID: s.task.OwnerSessionID, ActorID: s.task.ID, ActorType: metadata.AgentType,
		RunID: runID, BatchID: batchID, ParentRunID: parentRunID, AgentPath: agentPath,
	})
	runCtx = withAgentProgressEmitter(runCtx, emitter)
	runCtx = withAgentTranscriptWriter(runCtx, transcriptWriter)
	summary, runErr := runAgentQueryLoop(runCtx, s.loop, metadata, s.task.ID, request.prompt, outFile)
	var cleanedWorktree bool
	metadata, cleanedWorktree = finalizeAgentWorktreeMetadata(metadata)
	if cleanedWorktree {
		s.storeMetadata(metadata)
		if s.manager != nil {
			s.manager.updateTrustedAgentMetadata(s.task.ID, metadata)
		}
	}
	summary = applyAgentSessionMetadata(summary, metadata)
	if summary.Outcome == "" {
		summary.Outcome, summary.TerminalReason = classifyAgentRunTermination(runErr, strings.TrimSpace(summary.Output) != "")
	}
	terminalErr := runErr
	if terminalErr == nil {
		terminalErr = agentRunOutcomeError(summary)
	}
	terminalErrorText := agentRunTerminalErrorText(summary, terminalErr)
	if terminalErrorText != "" {
		// The retained task snapshot and output file are two views of the same
		// run. Persist the already-projected terminal error to the file as well;
		// semantic internal errors keep their private causes in the error chain
		// without rendering them here.
		_, _ = io.WriteString(outFile, terminalErrorText+"\n")
	}
	messages := s.loop.Messages()
	_ = outFile.Close()
	cancel()

	s.task.mu.Lock()
	s.task.AgentMessages = append([]types.Message(nil), messages...)
	if s.task.Status == "killed" {
		summary.Outcome = agentcontract.RunOutcomeCancelled
		summary.TerminalReason = "killed"
		code := -1
		s.task.ExitCode = &code
		s.task.Result = summary.Output
		s.task.Error = firstNonEmpty(errorString(terminalErr), "agent task was killed")
		finishedAt := time.Now().UTC()
		s.task.FinishedAt = &finishedAt
		s.task.cancel = nil
		if cleanedWorktree {
			metadataCopy := metadata
			s.task.AgentMetadata = &metadataCopy
		}
		finishAgentRunLocked(s.task, runID, "killed", summary, s.task.Error, finishedAt)
		record := s.task.recordLocked()
		s.task.mu.Unlock()
		s.manager.persistRecordForTask(s.task, record)
		if request.response == nil {
			s.manager.emitAgentCompletionNotification(s.manager.completionContextForTask(s.task), s.task, "killed", -1, summary)
		}
		close(done)
		if request.response != nil {
			request.response <- agentRunResponse{summary: summary, err: terminalErr}
		}
		if cleanedWorktree {
			s.manager.retireAgentSession(s.task.ID, s)
		}
		return
	}
	s.task.cancel = nil
	if cleanedWorktree {
		metadataCopy := metadata
		s.task.AgentMetadata = &metadataCopy
	}
	runStatus := agentTaskStatus(summary.Outcome)
	s.task.Status = runStatus
	s.task.Result = summary.Output
	s.task.Error = terminalErrorText
	exitCode := -1
	if summary.Outcome == agentcontract.RunOutcomeSucceeded {
		exitCode = 0
		s.task.Error = ""
	}
	s.task.ExitCode = &exitCode
	finishedAt := time.Now().UTC()
	s.task.FinishedAt = &finishedAt
	finishAgentRunLocked(s.task, runID, runStatus, summary, s.task.Error, finishedAt)
	record = s.task.recordLocked()
	s.task.mu.Unlock()
	s.manager.persistRecordForTask(s.task, record)
	if request.response == nil {
		s.manager.emitAgentCompletionNotification(s.manager.completionContextForTask(s.task), s.task, runStatus, exitCode, summary)
	}
	// A synchronous run is not complete until its post-run lifecycle has
	// committed. Signalling earlier lets the caller enqueue the next attempt
	// while this attempt still reads mutable session state.
	close(done)
	if request.response != nil {
		request.response <- agentRunResponse{summary: summary, err: terminalErr}
	}
	if cleanedWorktree {
		s.manager.retireAgentSession(s.task.ID, s)
	}
}

func agentRunLineageForRequest(ctx context.Context, agentID string) (batchID, parentRunID, agentPath string, ok bool) {
	if ctx == nil {
		return "", "", "", false
	}
	_, ok = executioncontract.ToolExecutionContextFromContext(ctx)
	if !ok {
		return "", "", "", false
	}
	batchID, parentRunID, agentPath = agentRunLineage(ctx, agentID)
	return batchID, parentRunID, agentPath, true
}

func (s *backgroundAgentSession) failQueuedRequest(request agentRunRequest, runErr error) {
	s.task.mu.Lock()
	dequeueAgentPromptLocked(s.task)
	if request.done != nil && s.task.done == request.done && s.task.Status == "running" {
		outcome, reason := classifyAgentRunTermination(runErr, false)
		s.task.Status = agentTaskStatus(outcome)
		s.task.Outcome = outcome
		s.task.TerminalReason = reason
		s.task.Error = runErr.Error()
		s.task.Result = ""
		s.task.cancel = nil
		code := -1
		s.task.ExitCode = &code
		finishedAt := time.Now().UTC()
		s.task.FinishedAt = &finishedAt
	}
	record := s.task.recordLocked()
	s.task.mu.Unlock()
	s.manager.persistRecordForTask(s.task, record)
	if request.done != nil {
		close(request.done)
	}
	if request.response != nil {
		request.response <- agentRunResponse{err: runErr}
	}
}

func updateAgentQueueReasonLocked(task *BackgroundTask) {
	if task == nil || task.QueuedPrompts <= 0 {
		if task != nil {
			task.QueuedPrompts = 0
			task.QueueReason = ""
		}
		return
	}
	if task.cancel != nil || task.QueuedPrompts > 1 {
		task.QueueReason = "dependency:active_run"
		return
	}
	task.QueueReason = "capacity:agent_session_worker"
}

func dequeueAgentPromptLocked(task *BackgroundTask) {
	if task == nil {
		return
	}
	if task.QueuedPrompts > 0 {
		task.QueuedPrompts--
	}
	updateAgentQueueReasonLocked(task)
}

func closedTaskDoneChannel() chan struct{} {
	done := make(chan struct{})
	close(done)
	return done
}
