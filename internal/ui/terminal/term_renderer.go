package ui

import (
	"fmt"
	"io"
	"strings"

	"github.com/agent-dance/luban/internal/contracts/interaction"
	"github.com/agent-dance/luban/internal/presentation"

	"github.com/agent-dance/luban/i18n"

	"github.com/charmbracelet/lipgloss"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/internal/runtime/compact"
	"github.com/agent-dance/luban/internal/ui/theme"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/runtimeevent"
	"github.com/agent-dance/luban/types"
)

// TermRenderer renders styled output to a terminal using lipgloss.
// It respects NO_COLOR and TERM=dumb automatically via lipgloss.
type TermRenderer struct {
	w          io.Writer
	lineState  *terminalLineStateWriter
	cacheDebug CacheBreakDebugDetector

	// Styles — initialised once in NewTermRenderer.
	greenStyle    lipgloss.Style
	yellowStyle   lipgloss.Style
	dimStyle      lipgloss.Style
	redStyle      lipgloss.Style
	boldCyanStyle lipgloss.Style
	boldRedStyle  lipgloss.Style
	brandStyle    lipgloss.Style
	bannerStyle   lipgloss.Style
	toolBoxStyle  lipgloss.Style
}

// terminalLineStateWriter lets line-oriented chrome such as usage receipts
// avoid being joined to a streamed model response that omitted its final
// newline. It observes bytes only; the underlying output remains unchanged.
type terminalLineStateWriter struct {
	w                io.Writer
	wrote            bool
	endedWithNewline bool
}

func (w *terminalLineStateWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	if n > 0 {
		w.wrote = true
		w.endedWithNewline = p[n-1] == '\n'
	}
	return n, err
}

func (w *terminalLineStateWriter) ensureLineBoundary() {
	if w.wrote && !w.endedWithNewline {
		_, _ = fmt.Fprintln(w)
	}
}

// NewTermRenderer creates a TermRenderer that writes to w.
func NewTermRenderer(w io.Writer) *TermRenderer {
	lineState := &terminalLineStateWriter{w: w}
	r := &TermRenderer{w: lineState, lineState: lineState}
	p := theme.Current()

	r.greenStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.Success))
	r.yellowStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.Warning))
	r.dimStyle = lipgloss.NewStyle().Faint(true)
	r.redStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.Danger))
	r.boldCyanStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(p.Accent))
	r.boldRedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(p.Danger))
	r.brandStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(p.Accent))
	r.bannerStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(p.Accent)).
		Padding(0, 1)
	r.toolBoxStyle = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(lipgloss.Color(p.Muted)).
		PaddingLeft(1)

	return r
}

// --- Text output ---

func (r *TermRenderer) Text(s string) {
	fmt.Fprint(r.w, s)
}

func (r *TermRenderer) Thinking(s string) {
	fmt.Fprint(r.w, r.dimStyle.Render(s))
}

func (r *TermRenderer) Error(s string) {
	fmt.Fprintln(r.w, r.boldRedStyle.Render(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyTerminalErrorPrefix)+s))
}

// RuntimeErrorEvent renders only the strict user projection. The raw loop
// message and provider diagnostics are intentionally not sent to the terminal;
// an explicitly authorized audit sink is required to expose them.
func (r *TermRenderer) RuntimeErrorEvent(ctx presentation.ToolEventContext, toolUseID, message string, apiError *types.APIError, metadata map[string]any) {
	r.Error(presentation.RuntimeErrorPublicMessage(ctx, toolUseID, message, apiError, metadata, i18n.DetectOrLoadLanguage(), false))
}

func (r *TermRenderer) Info(s string) {
	fmt.Fprintln(r.w, r.dimStyle.Render(s))
}

func (r *TermRenderer) Success(s string) {
	fmt.Fprintln(r.w, r.greenStyle.Render(s))
}

func (r *TermRenderer) Warning(s string) {
	fmt.Fprintln(r.w, r.yellowStyle.Render(s))
}

func (r *TermRenderer) Bold(s string) {
	fmt.Fprintln(r.w, r.boldCyanStyle.Render(s))
}

// --- Structured output ---

// toolInputPreview returns a short human-readable preview of a tool's input map.
func toolInputPreview(name string, input map[string]any) string {
	switch name {
	case "Bash":
		if cmd, ok := input["command"].(string); ok {
			if len(cmd) > 100 {
				cmd = cmd[:100] + "..."
			}
			return fmt.Sprintf(" `%s`", cmd)
		}
	case "Read", "Write", "Edit":
		if fp, ok := input["file_path"].(string); ok {
			return fmt.Sprintf(" %s", fp)
		}
	case "Glob":
		if p, ok := input["pattern"].(string); ok {
			return fmt.Sprintf(" %s", p)
		}
	case "Grep":
		if p, ok := input["pattern"].(string); ok {
			return fmt.Sprintf(" /%s/", p)
		}
	case "Agent":
		if p, ok := input["prompt"].(string); ok {
			if len(p) > 80 {
				p = p[:80] + "..."
			}
			return fmt.Sprintf(" %q", p)
		}
	}
	return ""
}

func (r *TermRenderer) RenderToolCall(_ presentation.ToolEventContext, call types.ToolUseBlock) {
	name, input := call.Name, call.Input
	fmt.Fprintln(r.w)
	preview := toolInputPreview(name, input)
	line := r.yellowStyle.Render("\u26a1 "+name) + r.dimStyle.Render(preview)
	fmt.Fprintln(r.w, line)
}

func (r *TermRenderer) RenderToolResult(ctx presentation.ToolEventContext, result types.ToolResultBlock) {
	event := types.NewToolResultRuntimeEvent(ctx.RuntimeIdentity(result.ToolUseID), result, i18n.KeyRuntimeToolResultPublicSummary, nil)
	if _, err := runtimeevent.NewAudienceProjector().Project(event, runtimeevent.ProjectionOptions{
		Audience: runtimeevent.AudienceUser, Redaction: runtimeevent.RedactionStrict,
	}); err != nil {
		r.RuntimeErrorEvent(ctx, result.ToolUseID, "", nil, nil)
		return
	}
	content, isError := result.TextContent(), presentation.ToolOutcomeIsError(result.Outcome)
	lines := strings.Split(content, "\n")

	prefix := "  \u21b3 " // ↳
	if isError {
		prefix = "  \u2717 " // ✗
	}

	var sb strings.Builder
	for i, line := range lines {
		if i == 0 {
			fmt.Fprintf(&sb, "%s%s\n", prefix, line)
		} else {
			fmt.Fprintf(&sb, "    %s\n", line)
		}
	}
	fmt.Fprint(r.w, r.dimStyle.Render(sb.String()))
}

// RenderSendUserMessageEvent prints Brief as assistant content rather than
// generic tool output, so no tool name, lightning marker, or result arrow is
// shown. The terminal does not persist identity, but accepts the contextual
// contract so dispatch never needs a string-only fallback.
func (r *TermRenderer) RenderSendUserMessageEvent(_ presentation.ToolEventContext, output interaction.SendUserMessageOutput, options presentation.SendUserMessageRenderOptions) {
	content := presentation.FormatSendUserMessage(output, options)
	if content == "" {
		return
	}
	fmt.Fprintln(r.w, content)
}

func (r *TermRenderer) Usage(u *types.Usage) {
	if u == nil {
		return
	}
	if u.CacheReadInputTokens > 0 || u.CacheCreationInputTokens > 0 {
		r.lineState.ensureLineBoundary()
		msg := i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyTerminalCacheUsage,
			u.CacheReadInputTokens/1000,
			u.CacheCreationInputTokens/1000,
			u.UncachedInputTokens()/1000)
		fmt.Fprintln(r.w, r.dimStyle.Render(msg))
	}
	if CacheBreakDebugEnabled() {
		if msg := r.cacheDebug.Check(u); msg != "" {
			fmt.Fprintln(r.w, r.dimStyle.Render(msg))
		}
	}
}

// --- Chrome ---

func (r *TermRenderer) Banner(providerName, model string) {
	lang := i18n.DetectOrLoadLanguage()
	logo := strings.Join(brand.TerminalLogoLines(60), "\n")
	status := strings.Join([]string{
		r.brandStyle.Render(brand.RuntimeName),
		r.dimStyle.Render(i18n.Text(lang, i18n.KeyBrandTagline)),
		"",
		i18n.Format(lang, i18n.KeyTerminalProvider, providerName),
		i18n.Format(lang, i18n.KeyTerminalModel, model),
	}, "\n")
	content := lipgloss.JoinHorizontal(lipgloss.Top, r.brandStyle.Render(logo), "  ", status)
	fmt.Fprintln(r.w, r.bannerStyle.Render(content))
}

func (r *TermRenderer) SessionInfo(id string, tools []string) {
	lang := i18n.DetectOrLoadLanguage()
	fmt.Fprintln(r.w, r.dimStyle.Render(i18n.Format(lang, i18n.KeyTerminalSession, id)))
	fmt.Fprintln(r.w, r.dimStyle.Render(i18n.Format(lang, i18n.KeyTerminalTools, strings.Join(tools, ", "))))
	fmt.Fprintln(r.w, r.dimStyle.Render(i18n.Text(lang, i18n.KeyTerminalTaskHint)))
}

func (r *TermRenderer) Prompt() string {
	return r.greenStyle.Render("> ")
}

func (r *TermRenderer) Newline() {
	fmt.Fprintln(r.w)
}

func (r *TermRenderer) Goodbye() {
	fmt.Fprintln(r.w, i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyTerminalGoodbye))
}

// --- Sprint 5 methods ---

// fmtK formats a token count as e.g. "1.2K" or "450".
func fmtK(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

// CostSummary prints a per-turn / cumulative cost line.
// Example: 💰 Turn: $0.0034 | Session: $0.12 | Tokens: 1.2K in / 450 out
func (r *TermRenderer) CostSummary(turnCost, cumulativeCost float64, inputTokens, outputTokens int) {
	r.CostSummaryInCurrency(turnCost, cumulativeCost, "USD", inputTokens, outputTokens)
}

// CostSummaryInCurrency prints amounts using the model catalog's billing
// currency while preserving the legacy cost-summary layout.
func (r *TermRenderer) CostSummaryInCurrency(turnCost, cumulativeCost float64, currency string, inputTokens, outputTokens int) {
	symbol := provider.CostCurrencySymbol(currency)
	msg := i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyTerminalCostSummary,
		symbol, turnCost, symbol, cumulativeCost, fmtK(inputTokens), fmtK(outputTokens))
	fmt.Fprintln(r.w, r.dimStyle.Render(msg))
}

// ContextBar prints a visual context-window usage bar.
// Example: [Context: ████████░░░░ 67% (134K/200K)]
func (r *TermRenderer) ContextBar(usedTokens, maxTokens int) {
	const barWidth = 12
	pct := 0.0
	if maxTokens > 0 {
		pct = float64(usedTokens) / float64(maxTokens)
		if pct > 1.0 {
			pct = 1.0
		}
	}

	filled := int(pct * barWidth)
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

	warning := maxTokens > 0 && usedTokens >= maxTokens-compact.WarningThresholdBufferTokens
	blocking := maxTokens > 0 && usedTokens >= maxTokens-compact.ManualCompactBufferTokens
	var barStyled string
	switch {
	case blocking:
		barStyled = r.redStyle.Render(bar)
	case warning:
		barStyled = r.yellowStyle.Render(bar)
	default:
		barStyled = r.greenStyle.Render(bar)
	}

	msg := i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyTerminalContextBar,
		barStyled, pct*100, fmtK(usedTokens), fmtK(maxTokens))
	contextLine := i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyTerminalContextBar, bar, pct*100, fmtK(usedTokens), fmtK(maxTokens))
	styledLine := strings.Replace(contextLine, bar, barStyled, 1)
	fmt.Fprintln(r.w, r.dimStyle.Render(styledLine))
	_ = msg // msg kept for readability; composite render used above
}

// SpinnerStart prints "⚡ Running {toolName}..." and returns a stop func that
// erases the line via a carriage-return + space overwrite.
func (r *TermRenderer) SpinnerStart(toolName string) func() {
	line := r.yellowStyle.Render(i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyTerminalRunning, toolName))
	fmt.Fprint(r.w, line)
	return func() {
		// Overwrite the line with spaces then CR back to start.
		fmt.Fprintf(r.w, "\r%s\r", strings.Repeat(" ", len(line)+4))
	}
}
