package tools

// agent_runasync_lifecycle.go provides the end-to-end async-agent
// lifecycle helper that drives spawn → progress register → run →
// summarize → classifyHandoff → worktree cleanup → terminal
// notification. Mirrors TS runAsyncAgentLifecycle from
// src/tools/AgentTool/agentToolUtils.ts.
//
// The helper is intentionally a thin orchestrator: each step delegates to
// existing infrastructure (BackgroundTaskManager, AgentTool.Progress,
// CleanupShellTasksForAgent, ClassifyAgentHandoff). The notification sink
// is pluggable so terminal/toast adapters can swap in without changing
// call sites.

import (
	"strings"
	"sync"

	"github.com/agent-dance/luban/types"
)

// AsyncAgentNotification is the payload delivered when a fire-and-forget
// async agent finishes a run. UIs render it as a toast / system
// notification. Verdict is the classifier output for parents in
// auto-mode; AgentID/AgentType identify the producer.
type AsyncAgentNotification struct {
	AgentID      string
	AgentType    string
	Status       string // "completed" | "failed" | "killed"
	Summary      string // first ~200 chars of last assistant text
	Verdict      AgentHandoffVerdict
	WorktreePath string // populated when the agent ran in a worktree
}

// AsyncAgentNotificationSink receives terminal notifications. Sinks must
// be non-blocking; the lifecycle helper does not retry on slow consumers.
type AsyncAgentNotificationSink interface {
	OnAsyncAgentFinished(AsyncAgentNotification)
}

// AsyncAgentNotificationSinkFunc adapts a plain function to the sink
// interface.
type AsyncAgentNotificationSinkFunc func(AsyncAgentNotification)

// OnAsyncAgentFinished implements AsyncAgentNotificationSink.
func (f AsyncAgentNotificationSinkFunc) OnAsyncAgentFinished(n AsyncAgentNotification) {
	f(n)
}

var (
	asyncNotifyMu sync.RWMutex
	asyncNotify   AsyncAgentNotificationSink
)

// SetAsyncAgentNotificationSink installs (or clears, with nil) the
// global notification sink. Safe for concurrent callers.
func SetAsyncAgentNotificationSink(s AsyncAgentNotificationSink) {
	asyncNotifyMu.Lock()
	defer asyncNotifyMu.Unlock()
	asyncNotify = s
}

func emitAsyncAgentNotification(n AsyncAgentNotification) {
	asyncNotifyMu.RLock()
	sink := asyncNotify
	asyncNotifyMu.RUnlock()
	if sink == nil {
		return
	}
	defer func() { _ = recover() }()
	sink.OnAsyncAgentFinished(n)
}

// AsyncAgentLifecycleResult is what FinalizeAsyncAgentLifecycle returns
// to the caller — an aggregate of the post-run housekeeping, useful for
// tests and for callers that want to decide what to display next.
type AsyncAgentLifecycleResult struct {
	Verdict          AgentHandoffVerdict
	Summary          string
	KilledShellTasks []string
	NotifiedSink     bool
}

// FinalizeAsyncAgentLifecycle runs the post-completion phase of the
// async-agent lifecycle. The caller has already executed the agent loop
// and produced a transcript + status string. This function:
//
//  1. classifies the handoff (parent-mode aware)
//  2. extracts a short summary from the last assistant text
//  3. kills any shell background tasks the agent spawned (port leaks)
//  4. emits a terminal notification via the configured sink
func FinalizeAsyncAgentLifecycle(
	mgr *BackgroundTaskManager,
	agentID, agentType, parentMode, status, worktreePath string,
	transcript []types.Message,
) AsyncAgentLifecycleResult {
	verdict := ClassifyAgentHandoff(transcript, parentMode)
	summary := lastAssistantText(transcript)
	if len(summary) > 200 {
		summary = summary[:200] + "…"
	}
	var killed []string
	if mgr != nil {
		killed = mgr.CleanupShellTasksForAgent(agentID)
	}
	notify := AsyncAgentNotification{
		AgentID:      agentID,
		AgentType:    agentType,
		Status:       strings.TrimSpace(status),
		Summary:      summary,
		Verdict:      verdict,
		WorktreePath: worktreePath,
	}
	notified := false
	asyncNotifyMu.RLock()
	sink := asyncNotify
	asyncNotifyMu.RUnlock()
	if sink != nil {
		notified = true
		emitAsyncAgentNotification(notify)
	}
	return AsyncAgentLifecycleResult{
		Verdict:          verdict,
		Summary:          summary,
		KilledShellTasks: killed,
		NotifiedSink:     notified,
	}
}
