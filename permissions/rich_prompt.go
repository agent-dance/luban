package permissions

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/agent-dance/luban/i18n"
	"github.com/charmbracelet/lipgloss"
)

// Lipgloss styles — defined at package level so they're initialised once.
var (
	styleBold     = lipgloss.NewStyle().Bold(true)
	styleToolName = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))  // cyan
	styleLow      = lipgloss.NewStyle().Foreground(lipgloss.Color("46"))             // green
	styleMedium   = lipgloss.NewStyle().Foreground(lipgloss.Color("226"))            // yellow
	styleHigh     = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))            // red
	stylePrompt   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")) // white
	styleDim      = lipgloss.NewStyle().Faint(true)
	styleBorder   = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)
)

// RichPrompt is a styled, risk-aware interactive permission prompt.
type RichPrompt struct {
	w       io.Writer
	scanner *bufio.Scanner
	mu      sync.Mutex
}

// NewRichPrompt creates a RichPrompt that writes to w and reads answers from r.
func NewRichPrompt(w io.Writer, r io.Reader) *RichPrompt {
	return &RichPrompt{w: w, scanner: bufio.NewScanner(r)}
}

// DecisionRequest implements the structured permission prompt contract.
func (rp *RichPrompt) DecisionRequest(ctx context.Context, request PromptRequest) PromptResponse {
	if err := ctx.Err(); err != nil {
		return PromptResponse{DecisionID: request.DecisionID, Decision: DecisionDeny, Outcome: PromptOutcomeCancelled}
	}
	response := responseForDecision(rp.ask(request.ToolName, request.Input))
	response.DecisionID = request.DecisionID
	return response
}

// ask displays the rich prompt and returns the user's Decision.
func (rp *RichPrompt) ask(toolName string, input map[string]any) Decision {
	rp.mu.Lock()
	defer rp.mu.Unlock()

	risk := classifyRisk(toolName, input)
	preview := previewFor(toolName, input)
	lang := i18n.DetectOrLoadLanguage()

	// Build the badge string with appropriate colour.
	badge := rp.colourBadge(risk)

	// Build the key-value detail line.
	detail := rp.buildDetail(toolName, input)

	// Assemble inner content.
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s  %s\n",
		styleBold.Render(i18n.Text(lang, i18n.KeyPermissionPromptTool)), styleToolName.Render(toolName)))
	if preview != "" {
		sb.WriteString(fmt.Sprintf("%s  %s\n",
			styleBold.Render(i18n.Text(lang, i18n.KeyPermissionPromptCall)), styleDim.Render(preview)))
	}
	if detail != "" && detail != preview {
		sb.WriteString(fmt.Sprintf("%s  %s\n",
			styleBold.Render(i18n.Text(lang, i18n.KeyPermissionPromptInfo)), styleDim.Render(detail)))
	}
	sb.WriteString(fmt.Sprintf("%s  %s", styleBold.Render(i18n.Text(lang, i18n.KeyPermissionPromptRisk)), badge))

	boxed := styleBorder.Render(sb.String())
	fmt.Fprintf(rp.w, "\n%s\n", boxed)

	// Print the prompt line outside the box.
	fmt.Fprintf(rp.w, "%s ", stylePrompt.Render(i18n.Text(lang, i18n.KeyPermissionPromptAllow)))

	if !rp.scanner.Scan() {
		fmt.Fprintln(rp.w)
		return DecisionDeny
	}
	response := strings.TrimSpace(rp.scanner.Text())
	switch strings.ToLower(response) {
	case "y":
		return DecisionAllowOnce
	case "a":
		return DecisionAllow
	default:
		return DecisionDeny
	}
}

// colourBadge returns a lipgloss-styled badge string for the given risk level.
func (rp *RichPrompt) colourBadge(risk RiskLevel) string {
	lang := i18n.DetectOrLoadLanguage()
	key := i18n.KeyPermissionPromptRiskLow
	if risk == RiskMedium {
		key = i18n.KeyPermissionPromptRiskMedium
	} else if risk == RiskHigh {
		key = i18n.KeyPermissionPromptRiskHigh
	}
	text := i18n.Text(lang, key)
	switch risk {
	case RiskLow:
		return styleLow.Render(text)
	case RiskMedium:
		return styleMedium.Render(text)
	case RiskHigh:
		return styleHigh.Render(text)
	}
	return text
}

// buildDetail returns a concise summary of the most relevant input fields.
func (rp *RichPrompt) buildDetail(toolName string, input map[string]any) string {
	switch toolName {
	case "Bash", "PowerShell":
		if cmd, ok := input["command"].(string); ok {
			if len(cmd) > 120 {
				return cmd[:120] + "…"
			}
			return cmd
		}
	case "Write", "Edit", "Read":
		if fp, ok := input["file_path"].(string); ok {
			return fp
		}
	case "Grep":
		if pat, ok := input["pattern"].(string); ok {
			if fp, ok2 := input["path"].(string); ok2 {
				return fmt.Sprintf("pattern=%s path=%s", pat, fp)
			}
			return "pattern=" + pat
		}
	case "Glob":
		if pat, ok := input["pattern"].(string); ok {
			return "pattern=" + pat
		}
	case "SendMessage":
		target := sendMessageTarget(input)
		preview := sendMessagePreview(input, 60)
		if preview != "" {
			return preview
		}
		if target != "" {
			return "to=" + target
		}
	}
	// Generic fallback: enumerate up to 3 key=value pairs.
	parts := make([]string, 0, 3)
	for _, key := range []string{"file_path", "command", "path", "query", "url", "pattern", "to"} {
		if v, ok := input[key].(string); ok && v != "" {
			s := v
			if len(s) > 60 {
				s = s[:60] + "…"
			}
			parts = append(parts, fmt.Sprintf("%s=%s", key, s))
			if len(parts) == 3 {
				break
			}
		}
	}
	return strings.Join(parts, "  ")
}
