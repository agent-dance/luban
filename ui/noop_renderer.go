package ui

import "github.com/agent-dance/luban/types"

// NoOpRenderer silently discards all output. Useful for tests, SDK mode,
// and MCP mode where no terminal output is desired.
type NoOpRenderer struct{}

func (NoOpRenderer) Text(string)                     {}
func (NoOpRenderer) Thinking(string)                 {}
func (NoOpRenderer) Error(string)                    {}
func (NoOpRenderer) Info(string)                     {}
func (NoOpRenderer) Success(string)                  {}
func (NoOpRenderer) Warning(string)                  {}
func (NoOpRenderer) Bold(string)                     {}
func (NoOpRenderer) ToolCall(string, map[string]any) {}
func (NoOpRenderer) ToolResult(string, bool)         {}
func (NoOpRenderer) Usage(*types.Usage)              {}
func (NoOpRenderer) Banner(string, string)           {}
func (NoOpRenderer) SessionInfo(string, []string)    {}
func (NoOpRenderer) Prompt() string                  { return "> " }
func (NoOpRenderer) Newline()                        {}
func (NoOpRenderer) Goodbye()                        {}

func (NoOpRenderer) CostSummary(float64, float64, int, int)               {}
func (NoOpRenderer) ContextBar(int, int)                                  {}
func (NoOpRenderer) SpinnerStart(string) func()                           { return func() {} }
func (NoOpRenderer) PermissionRequest(string, map[string]any, int) string { return "n" }
