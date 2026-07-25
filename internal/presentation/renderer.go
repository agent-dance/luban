// Package presentation defines renderer-neutral events, projections, and
// capabilities shared by terminal, full-screen TUI, and application adapters.
package presentation

import "github.com/agent-dance/luban/types"

// Renderer abstracts all terminal output for the CLI layer.
type Renderer interface {
	// Text prints streaming token text (no newline appended).
	Text(s string)
	// Thinking prints thinking/reasoning text in a subdued style.
	Thinking(s string)
	// Error prints an error message.
	Error(s string)
	// Info prints informational text in a subdued style.
	Info(s string)
	// Success prints a success message.
	Success(s string)
	// Warning prints a warning message.
	Warning(s string)
	// Bold prints emphasized text.
	Bold(s string)

	// RenderToolCall renders a causally identified tool invocation.
	RenderToolCall(ToolEventContext, types.ToolUseBlock)
	// RenderToolResult renders the complete typed tool result.
	RenderToolResult(ToolEventContext, types.ToolResultBlock)
	// Usage renders one completed provider request. It is never cumulative.
	// StructuredUsageRenderer is preferred when session/context ledgers exist.
	Usage(u *types.Usage)

	// Banner renders the startup banner with provider and model info.
	Banner(provider, model string)
	// SessionInfo renders session ID and available tools.
	SessionInfo(id string, tools []string)
	// Prompt returns the prompt string for the input line.
	Prompt() string
	// Newline prints a blank line.
	Newline()
	// Goodbye prints the exit message.
	Goodbye()

	// CostSummary displays per-request and cumulative-session cost. The token
	// counts are from the last request, never cumulative session totals.
	// Example: 💰 Turn: $0.0034 | Session: $0.12 | Tokens: 1.2K in / 450 out
	CostSummary(turnCost, cumulativeCost float64, inputTokens, outputTokens int)

	// ContextBar displays current occupancy against the active model's complete
	// configured context window. Output reservation is an internal compaction
	// concern and must not replace this denominator.
	// Example: [Context: ████████░░░░ 67% (134K/200K)]
	ContextBar(usedTokens, maxTokens int)

	// SpinnerStart prints a "running" indicator for toolName and returns a stop
	// function that clears the indicator when called.
	SpinnerStart(toolName string) func()
}

// CurrencyCostRenderer is an optional extension for renderers that expose the
// legacy per-request cost summary instead of StructuredUsageRenderer.
type CurrencyCostRenderer interface {
	CostSummaryInCurrency(turnCost, cumulativeCost float64, currency string, inputTokens, outputTokens int)
}
