package ui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/agent-dance/luban/i18n"

	"github.com/charmbracelet/lipgloss"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/compact"
	"github.com/agent-dance/luban/terminaltheme"
	"github.com/agent-dance/luban/types"
)

// TermRenderer renders styled output to a terminal using lipgloss.
// It respects NO_COLOR and TERM=dumb automatically via lipgloss.
type TermRenderer struct {
	w          io.Writer
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

// NewTermRenderer creates a TermRenderer that writes to w.
func NewTermRenderer(w io.Writer) *TermRenderer {
	r := &TermRenderer{w: w}
	p := terminaltheme.Current()

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
func (r *TermRenderer) RuntimeErrorEvent(ctx ToolEventContext, toolUseID, message string, apiError *types.APIError, metadata map[string]any) {
	r.Error(RuntimeErrorPublicMessage(ctx, toolUseID, message, apiError, metadata, i18n.LangEN, false))
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

func (r *TermRenderer) ToolCall(name string, input map[string]any) {
	fmt.Fprintln(r.w)
	preview := toolInputPreview(name, input)
	line := r.yellowStyle.Render("\u26a1 "+name) + r.dimStyle.Render(preview)
	fmt.Fprintln(r.w, line)
}

func (r *TermRenderer) ToolResult(content string, isError bool) {
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

// RenderSendUserMessage prints Brief as assistant content rather than generic
// tool output, so no tool name, lightning marker, or result arrow is shown.
func (r *TermRenderer) RenderSendUserMessage(output types.SendUserMessageOutput, options SendUserMessageRenderOptions) {
	content := FormatSendUserMessage(output, options)
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
	msg := i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyTerminalCostSummary,
		turnCost, cumulativeCost, fmtK(inputTokens), fmtK(outputTokens))
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

// PermissionRequest shows a risk-badged permission prompt and reads a response
// from stdin.  It returns "y" (yes), "n" (no), or "a" (always allow).
func (r *TermRenderer) PermissionRequest(toolName string, input map[string]any, riskLevel int) string {
	// Risk badge.
	badge := "🟢"
	riskLabel := i18n.KeyTerminalRiskLow
	switch {
	case riskLevel == 2:
		badge = "🟡"
		riskLabel = i18n.KeyTerminalRiskMedium
	case riskLevel >= 3:
		badge = "🔴"
		riskLabel = i18n.KeyTerminalRiskHigh
	}

	fmt.Fprintln(r.w)
	lang := i18n.DetectOrLoadLanguage()
	fmt.Fprintln(r.w, r.boldCyanStyle.Render(i18n.Text(lang, i18n.KeyTerminalPermissionTitle)))
	fmt.Fprintf(r.w, "  %s\n", i18n.Format(lang, i18n.KeyTerminalPermissionTool, r.yellowStyle.Render(toolName)))
	fmt.Fprintf(r.w, "  %s\n", i18n.Format(lang, i18n.KeyTerminalPermissionRisk, badge, r.dimStyle.Render(i18n.Text(lang, riskLabel))))

	// Show up to 3 key/value pairs from the input map.
	shown := 0
	for k, v := range input {
		if shown >= 3 {
			break
		}
		val := fmt.Sprintf("%v", v)
		if len(val) > 80 {
			val = val[:80] + "..."
		}
		fmt.Fprintf(r.w, "  %-9s : %s\n", k, r.dimStyle.Render(val))
		shown++
	}

	fmt.Fprint(r.w, r.boldCyanStyle.Render(i18n.Text(lang, i18n.KeyTerminalPermissionAllow)))

	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		switch answer {
		case "y", "yes":
			return "y"
		case "a", "always":
			return "a"
		}
	}
	return "n"
}
