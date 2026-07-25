package agent

// This file defines the typed Agent result variants and their canonical wire
// representation.

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agent-dance/luban/i18n"
	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
)

// AgentResultKind names the variant of a discriminated AgentResult value.
type AgentResultKind string

const (
	AgentResultKindCompleted   AgentResultKind = "completed"
	AgentResultKindError       AgentResultKind = "error"
	AgentResultKindAborted     AgentResultKind = "aborted"
	AgentResultKindPartial     AgentResultKind = "partial"
	AgentResultKindTimedOut    AgentResultKind = "timed_out"
	AgentResultKindCancelled   AgentResultKind = "cancelled"
	AgentResultKindInterrupted AgentResultKind = "interrupted"
)

// AgentResult is the Go-side discriminated union returned by an Agent run.
// All variants share a small set of bookkeeping fields so harnesses that only
// need the basics can avoid switching on the variant.
type AgentResult interface {
	ResultKind() AgentResultKind
	GetTranscriptPath() string
	GetDurationMs() int64
	GetTotalTokens() int
}

// AgentResultBase carries the fields every variant exposes.
type AgentResultBase struct {
	Kind           AgentResultKind `json:"kind"`
	TranscriptPath string          `json:"transcriptPath,omitempty"`
	DurationMs     int64           `json:"durationMs"`
	TotalTokens    int             `json:"totalTokens"`
}

func (b AgentResultBase) GetTranscriptPath() string { return b.TranscriptPath }
func (b AgentResultBase) GetDurationMs() int64      { return b.DurationMs }
func (b AgentResultBase) GetTotalTokens() int       { return b.TotalTokens }

// AgentCompleted represents a fully-completed agent run.
type AgentCompleted struct {
	AgentResultBase
	AgentID        string                  `json:"agentId,omitempty"`
	AgentType      string                  `json:"agentType,omitempty"`
	Prompt         string                  `json:"prompt,omitempty"`
	Content        []agentToolContentBlock `json:"content,omitempty"`
	ToolUseCount   int                     `json:"totalToolUseCount,omitempty"`
	Usage          agentToolUsage          `json:"usage"`
	CWD            string                  `json:"cwd,omitempty"`
	Mode           string                  `json:"mode,omitempty"`
	Isolation      string                  `json:"isolation,omitempty"`
	Model          string                  `json:"model,omitempty"`
	WorktreePath   string                  `json:"worktreePath,omitempty"`
	WorktreeBranch string                  `json:"worktreeBranch,omitempty"`
	LatestToolUse  string                  `json:"latestToolUse,omitempty"`
	WireStatus     string                  `json:"status,omitempty"` // mirrors TS schema
}

func (c AgentCompleted) ResultKind() AgentResultKind { return AgentResultKindCompleted }

func (c AgentCompleted) MarshalJSON() ([]byte, error) {
	type wire AgentCompleted
	c.Kind = AgentResultKindCompleted
	return json.Marshal(wire(c))
}

// AgentError represents an agent run that exited with an error.
type AgentError struct {
	AgentResultBase
	AgentID    string `json:"agentId,omitempty"`
	AgentType  string `json:"agentType,omitempty"`
	Message    string `json:"message,omitempty"`
	ExitReason string `json:"exitReason,omitempty"`
	WireStatus string `json:"status,omitempty"`
}

func (e AgentError) ResultKind() AgentResultKind { return AgentResultKindError }

func (e AgentError) MarshalJSON() ([]byte, error) {
	type wire AgentError
	e.Kind = AgentResultKindError
	return json.Marshal(wire(e))
}

// AgentAborted represents an agent run that was cancelled.
type AgentAborted struct {
	AgentResultBase
	AgentID       string `json:"agentId,omitempty"`
	AgentType     string `json:"agentType,omitempty"`
	Reason        string `json:"reason,omitempty"`
	LatestToolUse string `json:"latestToolUse,omitempty"`
	WireStatus    string `json:"status,omitempty"`
}

func (a AgentAborted) ResultKind() AgentResultKind { return AgentResultKindAborted }

func (a AgentAborted) MarshalJSON() ([]byte, error) {
	type wire AgentAborted
	a.Kind = AgentResultKindAborted
	return json.Marshal(wire(a))
}

// AgentPartial represents a background launch where the run has not yet
// finished but a handle is returned to the caller.
type AgentPartial struct {
	AgentResultBase
	AgentID           string `json:"agentId,omitempty"`
	AgentType         string `json:"agentType,omitempty"`
	TaskID            string `json:"taskId,omitempty"`
	OutputFile        string `json:"outputFile,omitempty"`
	CanReadOutputFile bool   `json:"canReadOutputFile,omitempty"`
	Description       string `json:"description,omitempty"`
	Prompt            string `json:"prompt,omitempty"`
	IsAsync           bool   `json:"isAsync,omitempty"`
	Message           string `json:"message,omitempty"`
	WireStatus        string `json:"status,omitempty"`
	LatestToolUse     string `json:"latestToolUse,omitempty"`
}

func (p AgentPartial) ResultKind() AgentResultKind { return AgentResultKindPartial }

func (p AgentPartial) MarshalJSON() ([]byte, error) {
	type wire AgentPartial
	p.Kind = AgentResultKindPartial
	return json.Marshal(wire(p))
}

// AgentIncomplete represents a foreground run that terminated with a precise
// non-success outcome. It is deliberately separate from AgentPartial, whose
// historical meaning is an asynchronous launch handle. Keeping the variants
// distinct prevents callers from mistaking a finished partial/timed-out run
// for work that is still executing in the background.
type AgentIncomplete struct {
	AgentResultBase
	AgentID          string                   `json:"agentId,omitempty"`
	AgentType        string                   `json:"agentType,omitempty"`
	Prompt           string                   `json:"prompt,omitempty"`
	Content          []agentToolContentBlock  `json:"content,omitempty"`
	Outcome          agentcontract.RunOutcome `json:"outcome"`
	Reason           string                   `json:"reason,omitempty"`
	ToolUseCount     int                      `json:"totalToolUseCount,omitempty"`
	Usage            agentToolUsage           `json:"usage"`
	LatestToolUse    string                   `json:"latestToolUse,omitempty"`
	ArtifactRefs     []string                 `json:"artifactRefs,omitempty"`
	VerificationRefs []string                 `json:"verificationRefs,omitempty"`
	WireStatus       string                   `json:"status"`
}

func (i AgentIncomplete) ResultKind() AgentResultKind { return i.Kind }

func (i AgentIncomplete) MarshalJSON() ([]byte, error) {
	type wire AgentIncomplete
	i.Kind = agentResultKindForRunOutcome(i.Outcome)
	if i.WireStatus == "" {
		i.WireStatus = string(i.Outcome)
	}
	return json.Marshal(wire(i))
}

func agentResultKindForRunOutcome(outcome agentcontract.RunOutcome) AgentResultKind {
	switch outcome {
	case agentcontract.RunOutcomeTimedOut:
		return AgentResultKindTimedOut
	case agentcontract.RunOutcomeCancelled:
		return AgentResultKindCancelled
	case agentcontract.RunOutcomeInterrupted:
		return AgentResultKindInterrupted
	default:
		return AgentResultKindPartial
	}
}

// MarshalAgentResult emits the JSON representation of any AgentResult variant.
// The encoded form always includes the canonical `kind` discriminator.
func MarshalAgentResult(r AgentResult) ([]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("MarshalAgentResult: nil result")
	}
	return json.Marshal(r)
}

// AgentResultFromIncomplete preserves the exact foreground terminal outcome,
// including useful partial output and immutable evidence references.
func AgentResultFromIncomplete(summary agentRunSummary, transcriptPath string) AgentIncomplete {
	text := strings.TrimRight(summary.Output, "\n")
	outcome := summary.Outcome
	if outcome == "" || outcome == agentcontract.RunOutcomeSucceeded || outcome == agentcontract.RunOutcomeFailed {
		outcome = agentcontract.RunOutcomePartial
	}
	reason := strings.TrimSpace(summary.TerminalReason)
	if reason == "" {
		reason = string(outcome)
	}
	result := AgentIncomplete{
		AgentResultBase: AgentResultBase{
			Kind:           agentResultKindForRunOutcome(outcome),
			TranscriptPath: firstNonEmpty(summary.TranscriptPath, transcriptPath),
			DurationMs:     summary.TotalDuration,
			TotalTokens:    summary.TotalTokens,
		},
		AgentID:          summary.AgentID,
		AgentType:        summary.AgentType,
		Prompt:           summary.Prompt,
		Outcome:          outcome,
		Reason:           reason,
		ToolUseCount:     summary.ToolUseCount,
		Usage:            formatAgentUsage(summary.Usage),
		LatestToolUse:    summary.LatestToolUse,
		ArtifactRefs:     append([]string(nil), summary.ArtifactRefs...),
		VerificationRefs: append([]string(nil), summary.VerificationRefs...),
		WireStatus:       string(outcome),
	}
	if strings.TrimSpace(text) != "" {
		result.Content = []agentToolContentBlock{{Type: "text", Text: text}}
	}
	return result
}

// AgentResultFromCompleted builds an AgentCompleted variant from the existing
// agentRunSummary representation, threading transcriptPath through.
func AgentResultFromCompleted(summary agentRunSummary, transcriptPath, latestToolUse string) AgentCompleted {
	text := strings.TrimRight(summary.Output, "\n")
	if strings.TrimSpace(text) == "" {
		text = i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolBackgroundAgentEmptyOutput)
	}
	return AgentCompleted{
		AgentResultBase: AgentResultBase{
			Kind:           AgentResultKindCompleted,
			TranscriptPath: transcriptPath,
			DurationMs:     summary.TotalDuration,
			TotalTokens:    summary.TotalTokens,
		},
		AgentID:   summary.AgentID,
		AgentType: summary.AgentType,
		Prompt:    summary.Prompt,
		Content: []agentToolContentBlock{{
			Type: "text",
			Text: text,
		}},
		ToolUseCount:   summary.ToolUseCount,
		Usage:          formatAgentUsage(summary.Usage),
		CWD:            summary.CWD,
		Mode:           summary.Mode,
		Isolation:      summary.Isolation,
		Model:          summary.Model,
		WorktreePath:   summary.WorktreePath,
		WorktreeBranch: summary.WorktreeBranch,
		LatestToolUse:  latestToolUse,
		WireStatus:     "completed",
	}
}

// AgentResultFromError wraps a Go error as the AgentError variant.
func AgentResultFromError(agentID, agentType string, durationMs int64, err error) AgentError {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	return AgentError{
		AgentResultBase: AgentResultBase{
			Kind:        AgentResultKindError,
			DurationMs:  durationMs,
			TotalTokens: 0,
		},
		AgentID:    agentID,
		AgentType:  agentType,
		Message:    msg,
		ExitReason: "error",
		WireStatus: "error",
	}
}

// AgentResultFromAborted wraps a cancellation as the AgentAborted variant.
func AgentResultFromAborted(agentID, agentType, reason, latestToolUse string, durationMs int64, totalTokens int) AgentAborted {
	return AgentAborted{
		AgentResultBase: AgentResultBase{
			Kind:        AgentResultKindAborted,
			DurationMs:  durationMs,
			TotalTokens: totalTokens,
		},
		AgentID:       agentID,
		AgentType:     agentType,
		Reason:        reason,
		LatestToolUse: latestToolUse,
		WireStatus:    "aborted",
	}
}

// AgentResultFromAsyncLaunch builds an AgentPartial for the async_launched path.
func AgentResultFromAsyncLaunch(agentID, agentType, description, prompt, outputFile string, canReadOutputFile bool) AgentPartial {
	msg := fmt.Sprintf("Use SendMessage with to: %q to continue this agent. The agent is working in the background and will notify when it completes.", agentID)
	return AgentPartial{
		AgentResultBase: AgentResultBase{
			Kind: AgentResultKindPartial,
		},
		AgentID:           agentID,
		AgentType:         agentType,
		TaskID:            agentID,
		OutputFile:        outputFile,
		CanReadOutputFile: canReadOutputFile,
		Description:       description,
		Prompt:            prompt,
		IsAsync:           true,
		Message:           msg,
		WireStatus:        "async_launched",
	}
}
