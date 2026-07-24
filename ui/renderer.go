// Package ui provides terminal rendering abstractions for the CLI layer.
// Future TUI frameworks (e.g. Bubble Tea) only need a new Renderer implementation.
package ui

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

	// ToolCall renders a tool invocation header (e.g. ⚡ Bash `ls -la`).
	// input is the raw tool input map; the renderer is responsible for
	// generating a human-readable preview from it.
	ToolCall(name string, input map[string]any)
	// ToolResult renders complete tool output. Renderers may summarize the
	// presentation, but must retain a lossless path to the original evidence.
	ToolResult(content string, isError bool)
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

	// ContextBar displays effective model input-context utilisation after the
	// output reservation has been removed; maxTokens is that effective capacity.
	// Example: [Context: ████████░░░░ 67% (134K/200K)]
	ContextBar(usedTokens, maxTokens int)

	// SpinnerStart prints a "running" indicator for toolName and returns a stop
	// function that clears the indicator when called.
	SpinnerStart(toolName string) func()

	// PermissionRequest shows a rich permission prompt for toolName with the
	// supplied input and riskLevel (1=low 🟢, 2=medium 🟡, 3+=high 🔴).
	// It blocks until the user responds and returns "y", "n", or "a" (always).
	PermissionRequest(toolName string, input map[string]any, riskLevel int) string
}
