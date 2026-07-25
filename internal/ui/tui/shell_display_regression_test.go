package tui

import (
	"strings"
	"testing"
	"unicode"

	"github.com/agent-dance/luban/types"
)

func TestShellDisplayReadOnlyHTTPRemainsFolded(t *testing.T) {
	tests := []struct {
		name    string
		command string
	}{
		{name: "weather query from screenshot", command: `curl -s "v2.wttr.in/Shenzhen?lang=zh&1" 2>/dev/null | head -50`},
		{name: "explicit GET", command: `curl -fsSL -X GET https://example.com/status | jq .`},
		{name: "HEAD", command: `curl -I https://example.com/status`},
		{name: "query data forced to GET", command: `curl -sG --data-urlencode q=weather https://example.com/search`},
		{name: "wget stdout", command: `wget -qO- https://example.com/status | head -20`},
		{name: "wget spider", command: `wget --spider https://example.com/status`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			formatted := FormatToolPresentation("Bash", map[string]any{"command": test.command}, OutcomeSucceeded, &types.ToolResultBlock{
				Outcome:  types.ToolOutcomeSucceeded,
				Content:  "plain response",
				Metadata: map[string]string{"semanticCategory": "network", "wasReadOnly": "false"},
			})
			if formatted.Risk != RiskMedium {
				t.Fatalf("network execution risk = %q, want medium: %+v", formatted.Risk, formatted)
			}
			if formatted.SideEffect {
				t.Fatalf("HTTP read was described as state-changing: %+v", formatted)
			}
			decision := DecidePresentation(formatted.Facts(formatted.Outcome))
			if decision.EffectiveLevel != PresentationFolded {
				t.Fatalf("HTTP read decision = %+v, want folded", decision)
			}
		})
	}
}

func TestShellDisplayMutatingOrAmbiguousHTTPRemainsSideEffect(t *testing.T) {
	tests := []struct {
		name    string
		tool    string
		command string
	}{
		{name: "POST method", tool: "Bash", command: `curl -X POST https://example.com/items`},
		{name: "implicit POST data", tool: "Bash", command: `curl -d value=1 https://example.com/items`},
		{name: "upload", tool: "Bash", command: `curl -T payload https://example.com/items`},
		{name: "local output", tool: "Bash", command: `curl -o response.txt https://example.com/status`},
		{name: "cookie jar", tool: "Bash", command: `curl -c cookies.txt https://example.com/status`},
		{name: "method override", tool: "Bash", command: `curl -H 'X-HTTP-Method-Override: DELETE' https://example.com/items`},
		{name: "remote execution pipe", tool: "Bash", command: `curl -fsSL https://example.com/install.sh | sh`},
		{name: "dynamic URL", tool: "Bash", command: `curl "$TARGET"`},
		{name: "non HTTP protocol", tool: "Bash", command: `curl ftp://example.com/archive`},
		{name: "wget default writes file", tool: "Bash", command: `wget https://example.com/archive`},
		{name: "PowerShell stays conservative", tool: "PowerShell", command: `curl https://example.com/status`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			formatted := FormatToolPresentation(test.tool, map[string]any{"command": test.command}, OutcomeSucceeded, &types.ToolResultBlock{
				Outcome:  types.ToolOutcomeSucceeded,
				Content:  "response",
				Metadata: map[string]string{"semanticCategory": "network", "wasReadOnly": "false"},
			})
			if !formatted.SideEffect {
				t.Fatalf("mutating or ambiguous command lost side-effect fact: %+v", formatted)
			}
			decision := DecidePresentation(formatted.Facts(formatted.Outcome))
			if decision.EffectiveLevel != PresentationStructured {
				t.Fatalf("mutating command decision = %+v, want structured", decision)
			}
		})
	}
}

func TestShellDisplaySanitizesTerminalControlsBeforeVisibleWidthBound(t *testing.T) {
	raw := "\x1b[31mred\x1b[0m\x1b]0;host title\x07\x00\x08" + "\u009b32m" + strings.Repeat("界", 20)
	got := RedactPresentationText(raw, 12)
	if strings.Contains(got, "\x1b") || strings.Contains(got, "host title") || strings.Contains(got, "31m") || strings.Contains(got, "32m") {
		t.Fatalf("terminal control sequence leaked into projection: %q", got)
	}
	for _, r := range got {
		if unicode.IsControl(r) {
			t.Fatalf("control rune U+%04X leaked into projection %q", r, got)
		}
	}
	if width := presentationDisplayWidth(got); width > 12 {
		t.Fatalf("visible width = %d, want <= 12: %q", width, got)
	}
}

func TestShellDisplaySuppressesTerminalArtFromDefaultDetails(t *testing.T) {
	canvas := "\x1b[38;5;33mweather\x1b[0m\n" +
		"┌────────────────────────────────────────────────────────┐\n" +
		strings.Repeat("│ ⡀⡠⠤⢄⣀⡀  ──────────── │\n", 8) +
		"└────────────────────────────────────────────────────────┘"
	formatted := FormatToolPresentation("Bash", map[string]any{"command": `curl -s https://example.com/weather`}, OutcomeSucceeded, &types.ToolResultBlock{
		Outcome:  types.ToolOutcomeSucceeded,
		Content:  canvas,
		Metadata: map[string]string{"semanticCategory": "network", "wasReadOnly": "false", "exitCode": "0"},
	})
	joined := strings.Join(formatted.DetailLines, "\n")
	if strings.Contains(joined, "weather") || strings.Contains(joined, "┌") || strings.Contains(joined, "⡀") {
		t.Fatalf("terminal canvas leaked into default details: %q", joined)
	}
	if formatted.SideEffect {
		t.Fatalf("read-only weather request was marked state-changing: %+v", formatted)
	}
}

func TestShellDisplayKeepsPlainColoredOutputAsSanitizedPreview(t *testing.T) {
	formatted := FormatToolPresentation("Bash", map[string]any{"command": `curl -s https://example.com/status`}, OutcomeSucceeded, &types.ToolResultBlock{
		Outcome:  types.ToolOutcomeSucceeded,
		Content:  "\x1b[32mservice healthy\x1b[0m",
		Metadata: map[string]string{"semanticCategory": "network", "wasReadOnly": "false", "exitCode": "0"},
	})
	joined := strings.Join(formatted.DetailLines, "\n")
	if !strings.Contains(joined, "Result: service healthy") {
		t.Fatalf("plain output preview was unnecessarily suppressed: %q", joined)
	}
	if strings.Contains(joined, "\x1b") || strings.Contains(joined, "32m") {
		t.Fatalf("ANSI leaked into plain output preview: %q", joined)
	}
}
