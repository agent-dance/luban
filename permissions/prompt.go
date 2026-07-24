package permissions

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/agent-dance/luban/i18n"
)

// NewInteractivePrompt creates a promptFunc that asks the user via terminal.
// It reads from r (usually os.Stdin) and writes to w (usually os.Stderr so it
// doesn't pollute stdout in -p mode).
//
// Prompt format:
//
//	⚡ Allow {toolName}? {preview}  [y/N/a(lways)]:
//
// Responses:
//
//	y / Y            → DecisionAllowOnce  (allow this call only)
//	a / A            → DecisionAllow      (allow + cache for the session)
//	n / N / <empty>  → DecisionDeny
func NewInteractivePrompt(w io.Writer, r io.Reader) func(toolName string, input map[string]any) Decision {
	scanner := bufio.NewScanner(r)

	return func(toolName string, input map[string]any) Decision {
		preview := previewFor(toolName, input)
		fmt.Fprint(w, i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyPermissionPromptInline, toolName, preview))

		if !scanner.Scan() {
			// EOF or read error — deny for safety
			fmt.Fprintln(w)
			return DecisionDeny
		}

		response := strings.TrimSpace(scanner.Text())
		switch strings.ToLower(response) {
		case "y":
			return DecisionAllowOnce
		case "a":
			return DecisionAllow // will be cached by askOrCache
		default:
			return DecisionDeny
		}
	}
}

// previewFor returns a short human-readable preview of the tool invocation.
func previewFor(toolName string, input map[string]any) string {
	switch toolName {
	case "Bash", "PowerShell":
		if cmd, ok := input["command"].(string); ok {
			if len(cmd) > 80 {
				return cmd[:80] + "…"
			}
			return cmd
		}
	case "FileRead", "FileWrite", "FileEdit", "FileAppend", "FileDelete",
		"FileList", "FileGlob", "FileMove", "FileSearch", "FileLink",
		"Write", "Edit", "Read":
		if fp, ok := input["file_path"].(string); ok {
			return fp
		}
		if dir, ok := input["directory"].(string); ok {
			return dir
		}
		if pat, ok := input["pattern"].(string); ok {
			return pat
		}
	case "Agent":
		if p, ok := input["prompt"].(string); ok {
			if len(p) > 80 {
				return p[:80] + "…"
			}
			return p
		}
	case "SendMessage":
		return sendMessagePreview(input, 80)
	case "Grep":
		if pat, ok := input["pattern"].(string); ok {
			return pat
		}
	case "Glob":
		if pat, ok := input["pattern"].(string); ok {
			return pat
		}
	}
	// Fallback: try to show the most relevant input field
	for _, key := range []string{"file_path", "command", "path", "query", "url", "to"} {
		if v, ok := input[key].(string); ok && v != "" {
			if len(v) > 80 {
				return v[:80] + "…"
			}
			return v
		}
	}
	return ""
}
