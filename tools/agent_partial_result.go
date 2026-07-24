package tools

// agent_partial_result.go mirrors the TS extractPartialResult helper from
// src/tools/AgentTool/agentToolUtils.ts.
//
// When an agent is interrupted (SIGTERM, abort, timeout), the parent normally
// receives an "Agent error: cancelled" string and loses every byte the
// sub-agent already produced. extractPartialResult walks the in-progress
// transcript and salvages whatever the agent did emit before the interrupt:
// any completed text blocks and any tool_results that landed before the cut.
// This lets users cancel a stuck long-running agent and still keep the
// useful intermediate findings rather than starting from zero.

import (
	"strings"

	"github.com/agent-dance/luban/types"
)

// PartialAgentResult is the shape returned to a parent agent when an
// in-progress agent is killed/timed-out. Text contains a best-effort summary
// of completed assistant text blocks; ToolResults contains any tool_results
// that resolved before the cut. Empty when nothing useful was emitted.
type PartialAgentResult struct {
	Text        string
	ToolResults []PartialToolResult
}

// PartialToolResult is the per-tool sketch saved on partial extraction.
type PartialToolResult struct {
	ToolUseID string
	ToolName  string
	Content   string
	IsError   bool
}

// ExtractPartialAgentResult inspects the agent's transcript and returns the
// completed text + tool_results captured before the interrupt. The transcript
// is the message history the agent built up to the point of cancellation.
// The returned struct is safe to embed in the parent's transcript even when
// nothing was emitted (Text == "" and ToolResults == nil).
func ExtractPartialAgentResult(transcript []types.Message) PartialAgentResult {
	if len(transcript) == 0 {
		return PartialAgentResult{}
	}
	resolved := collectResolvedToolUseIDs(transcript)
	toolNames := collectToolUseNames(transcript)
	var (
		texts   []string
		results []PartialToolResult
	)
	seenResults := map[string]struct{}{}
	for _, msg := range transcript {
		for _, block := range msg.Content {
			switch b := block.(type) {
			case types.TextBlock:
				if msg.Role == types.RoleAssistant {
					if t := strings.TrimSpace(b.Text); t != "" {
						texts = append(texts, t)
					}
				}
			case types.ToolResultBlock:
				if _, ok := seenResults[b.ToolUseID]; ok {
					continue
				}
				seenResults[b.ToolUseID] = struct{}{}
				_ = resolved // reference for clarity; resolved is computed for parity with filter
				results = append(results, PartialToolResult{
					ToolUseID: b.ToolUseID,
					ToolName:  toolNames[b.ToolUseID],
					Content:   b.TextContent(),
					IsError:   b.IsError,
				})
			}
		}
	}
	out := PartialAgentResult{
		Text:        strings.Join(texts, "\n\n"),
		ToolResults: results,
	}
	return out
}

// collectToolUseNames returns a map from tool_use_id to the tool name as
// reported by the assistant. Used to enrich PartialToolResult so the parent
// can describe which tool produced each salvaged output.
func collectToolUseNames(transcript []types.Message) map[string]string {
	out := map[string]string{}
	for _, msg := range transcript {
		for _, block := range msg.Content {
			if tu, ok := block.(types.ToolUseBlock); ok {
				if tu.ID != "" {
					out[tu.ID] = tu.Name
				}
			}
		}
	}
	return out
}

// FormatPartialAgentResult renders the partial result into a parent-facing
// string suitable for embedding in the cancellation message. Mirrors TS
// formatPartialResult: includes a header, the salvaged text, and a per-tool
// summary block when ToolResults is non-empty.
func FormatPartialAgentResult(p PartialAgentResult) string {
	if p.Text == "" && len(p.ToolResults) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("[partial result before cancel]")
	if p.Text != "" {
		b.WriteString("\n\n")
		b.WriteString(p.Text)
	}
	if len(p.ToolResults) > 0 {
		b.WriteString("\n\nTool results captured:\n")
		for _, tr := range p.ToolResults {
			b.WriteString("- ")
			if tr.ToolName != "" {
				b.WriteString(tr.ToolName)
			} else {
				b.WriteString("tool")
			}
			if tr.IsError {
				b.WriteString(" (error)")
			}
			content := strings.TrimSpace(tr.Content)
			if content != "" {
				b.WriteString(": ")
				if len(content) > 200 {
					content = content[:200] + "…"
				}
				b.WriteString(content)
			}
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
