package ui

import (
	"fmt"
	"io"

	"github.com/agent-dance/luban/internal/contracts/interaction"
	"github.com/agent-dance/luban/internal/presentation"

	"github.com/agent-dance/luban/types"
)

// QuietRenderer is a minimal presentation.Renderer that only outputs final assistant text.
// Everything else — thinking blocks, tool calls, cost summaries, spinners,
// banners — is silently discarded. It is used when --quiet / -q is set.
type QuietRenderer struct {
	w io.Writer
}

// NewQuietRenderer creates a QuietRenderer writing to w.
func NewQuietRenderer(w io.Writer) *QuietRenderer {
	return &QuietRenderer{w: w}
}

// Text writes streaming token text directly to the output.
func (r *QuietRenderer) Text(s string) { fmt.Fprint(r.w, s) }

// Everything below is intentionally a no-op.

func (r *QuietRenderer) Thinking(_ string)                                                     {}
func (r *QuietRenderer) Error(_ string)                                                        {}
func (r *QuietRenderer) Info(_ string)                                                         {}
func (r *QuietRenderer) Success(_ string)                                                      {}
func (r *QuietRenderer) Warning(_ string)                                                      {}
func (r *QuietRenderer) Bold(_ string)                                                         {}
func (r *QuietRenderer) RenderToolCall(presentation.ToolEventContext, types.ToolUseBlock)      {}
func (r *QuietRenderer) RenderToolResult(presentation.ToolEventContext, types.ToolResultBlock) {}
func (r *QuietRenderer) RenderSendUserMessageEvent(_ presentation.ToolEventContext, output interaction.SendUserMessageOutput, options presentation.SendUserMessageRenderOptions) {
	r.Text(presentation.FormatSendUserMessage(output, options))
}
func (r *QuietRenderer) Usage(_ *types.Usage)                           {}
func (r *QuietRenderer) Banner(_ string, _ string)                      {}
func (r *QuietRenderer) SessionInfo(_ string, _ []string)               {}
func (r *QuietRenderer) Prompt() string                                 { return "" }
func (r *QuietRenderer) Newline()                                       {}
func (r *QuietRenderer) Goodbye()                                       {}
func (r *QuietRenderer) CostSummary(_ float64, _ float64, _ int, _ int) {}
func (r *QuietRenderer) ContextBar(_ int, _ int)                        {}
func (r *QuietRenderer) SpinnerStart(_ string) func()                   { return func() {} }
