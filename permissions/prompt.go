package permissions

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
	case "Write", "Edit", "Read":
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
