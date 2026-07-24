package tools

// agent_classify_handoff.go mirrors the TS classifyHandoffIfNeeded helper
// from src/tools/AgentTool/agentToolUtils.ts. When the parent runs in
// auto-mode and a sub-agent's transcript lands, the parent normally
// pauses and asks the user whether to continue. If the work clearly
// finished a self-contained task (Explore returned findings, build
// passed, etc.) the auto-classifier votes "continue" and the parent
// resumes without an interrupt. When the work is genuinely ambiguous
// (errors, partial results, open questions) the classifier defers to
// the human.
//
// The Go runtime does not yet have direct access to the classifier
// model, so this helper returns a deterministic verdict from
// transcript surface signals: presence of error markers, "FAIL"/"BLOCKED"
// keywords, or unresolved tool_use blocks. Any future hook can swap the
// implementation by calling SetAgentHandoffClassifier; the default keeps
// behaviour conservative (HandoffPause) when in doubt.

import (
	"strings"
	"sync"

	"github.com/agent-dance/luban/types"
)

// AgentHandoffVerdict captures the classifier's decision.
type AgentHandoffVerdict string

const (
	HandoffContinue AgentHandoffVerdict = "continue"
	HandoffPause    AgentHandoffVerdict = "pause"
)

// AgentHandoffClassifier is the pluggable contract; the default heuristic
// classifier ships with this file but production deployments can wire a
// model-backed classifier.
type AgentHandoffClassifier interface {
	ClassifyHandoff(transcript []types.Message, parentMode string) AgentHandoffVerdict
}

// AgentHandoffClassifierFunc adapts a plain function to the interface.
type AgentHandoffClassifierFunc func([]types.Message, string) AgentHandoffVerdict

// ClassifyHandoff implements AgentHandoffClassifier.
func (f AgentHandoffClassifierFunc) ClassifyHandoff(t []types.Message, mode string) AgentHandoffVerdict {
	return f(t, mode)
}

var (
	handoffClassifierMu sync.RWMutex
	handoffClassifier   AgentHandoffClassifier = AgentHandoffClassifierFunc(defaultHandoffClassifier)
)

// SetAgentHandoffClassifier swaps the classifier. Pass nil to restore
// the default heuristic.
func SetAgentHandoffClassifier(c AgentHandoffClassifier) {
	handoffClassifierMu.Lock()
	defer handoffClassifierMu.Unlock()
	if c == nil {
		handoffClassifier = AgentHandoffClassifierFunc(defaultHandoffClassifier)
		return
	}
	handoffClassifier = c
}

// ClassifyAgentHandoff is the public entry. parentMode is "auto",
// "default", "plan", etc.; only "auto" callers honour HandoffContinue.
// In every other mode the function is a no-op that returns HandoffPause.
func ClassifyAgentHandoff(transcript []types.Message, parentMode string) AgentHandoffVerdict {
	switch strings.ToLower(strings.TrimSpace(parentMode)) {
	case "auto", "acceptedits", "bypasspermissions":
		// Auto is represented by acceptEdits or bypassPermissions in runtime
		// permission snapshots, depending on the active frontend policy.
	default:
		return HandoffPause
	}
	handoffClassifierMu.RLock()
	classifier := handoffClassifier
	handoffClassifierMu.RUnlock()
	if classifier == nil {
		return HandoffPause
	}
	return classifier.ClassifyHandoff(transcript, parentMode)
}

// defaultHandoffClassifier is a deterministic surface-signal heuristic.
// It returns HandoffPause when:
//   - the transcript has unresolved tool_use blocks
//   - the final assistant text contains FAIL / BLOCKED / ERROR markers
//   - the final assistant text is empty
//
// Otherwise it returns HandoffContinue so auto-mode parents keep
// momentum after a clean async run.
func defaultHandoffClassifier(transcript []types.Message, _ string) AgentHandoffVerdict {
	if len(transcript) == 0 {
		return HandoffPause
	}
	// Reject transcripts with orphaned tool_use blocks — that means the
	// sub-agent was interrupted, so we must hand back to the human.
	resolved := collectResolvedToolUseIDs(transcript)
	for _, msg := range transcript {
		if msg.Role != types.RoleAssistant {
			continue
		}
		for _, block := range msg.Content {
			if tu, ok := block.(types.ToolUseBlock); ok {
				if _, ok := resolved[tu.ID]; !ok {
					return HandoffPause
				}
			}
		}
	}
	// Inspect the final assistant text for failure markers.
	finalText := lastAssistantText(transcript)
	if finalText == "" {
		return HandoffPause
	}
	upper := strings.ToUpper(finalText)
	for _, marker := range []string{"FAIL", "BLOCKED", "ERROR:", "PANIC", "UNRESOLVED"} {
		if strings.Contains(upper, marker) {
			return HandoffPause
		}
	}
	return HandoffContinue
}

func lastAssistantText(transcript []types.Message) string {
	for i := len(transcript) - 1; i >= 0; i-- {
		msg := transcript[i]
		if msg.Role != types.RoleAssistant {
			continue
		}
		var parts []string
		for _, block := range msg.Content {
			if tb, ok := block.(types.TextBlock); ok {
				if t := strings.TrimSpace(tb.Text); t != "" {
					parts = append(parts, t)
				}
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n")
		}
	}
	return ""
}
