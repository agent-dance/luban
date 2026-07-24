package tools

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/agent-dance/luban/types"
)

const (
	maxAgentLiveOutputRunes   = 12_000
	maxAgentToolInputRunes    = 240
	maxAgentToolResponseRunes = 8_000
)

var agentLiveSystemReminderRE = regexp.MustCompile(`(?s)<system-reminder>.*?</system-reminder>`)

type agentLiveOutputBuffer struct {
	text string
}

func (b *agentLiveOutputBuffer) appendAssistant(text string) {
	if b == nil || text == "" {
		return
	}
	b.appendRaw(sanitiseAgentLiveText(text), false)
}

func (b *agentLiveOutputBuffer) appendToolCall(toolUse types.ToolUseBlock) {
	if b == nil {
		return
	}
	name := strings.TrimSpace(toolUse.Name)
	if name == "" {
		return
	}
	line := "→ " + name
	if core := agentToolCoreInput(toolUse); core != "" {
		line += " " + core
	}
	b.appendRaw(line, true)
}

func (b *agentLiveOutputBuffer) appendToolResult(toolName string, result types.ToolResultBlock) {
	if b == nil {
		return
	}
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		toolName = "?"
	}
	entry := "← " + toolName
	if response := agentToolResponsePreview(result); response != "" {
		entry += "\n" + response
	}
	b.appendRaw(entry, true)
}

func (b *agentLiveOutputBuffer) appendRaw(text string, eventBoundary bool) {
	if b == nil || text == "" {
		return
	}
	if eventBoundary && b.text != "" && !strings.HasSuffix(b.text, "\n") {
		b.text += "\n"
	}
	b.text += text
	if eventBoundary && !strings.HasSuffix(b.text, "\n") {
		b.text += "\n"
	}
	runes := []rune(b.text)
	if len(runes) > maxAgentLiveOutputRunes {
		b.text = string(runes[len(runes)-maxAgentLiveOutputRunes:])
	}
}

func (b *agentLiveOutputBuffer) snapshot() string {
	if b == nil {
		return ""
	}
	return strings.TrimRight(b.text, "\r\n")
}

func agentToolCoreInput(toolUse types.ToolUseBlock) string {
	input := toolUse.Input
	parts := make([]string, 0, 4)
	add := func(key string) {
		if value := safeAgentToolInputScalar(input[key]); value != "" {
			parts = append(parts, key+"="+value)
		}
	}
	switch strings.ToLower(strings.TrimSpace(toolUse.Name)) {
	case "read", "fileread", "write", "filewrite", "edit", "fileedit":
		add("file_path")
		if strings.EqualFold(toolUse.Name, "Read") || strings.EqualFold(toolUse.Name, "FileRead") {
			add("offset")
			add("limit")
		}
	case "glob":
		add("pattern")
		add("path")
	case "grep":
		add("pattern")
		add("path")
		add("glob")
		add("output_mode")
	case "bash", "powershell":
		add("command")
		add("timeout_ms")
		add("timeout")
	case "webfetch":
		if raw := safeAgentToolInputScalar(input["url"]); raw != "" {
			parts = append(parts, "url="+sanitizeAgentProgressURL(raw))
		}
		add("prompt")
	case "websearch", "search":
		if _, ok := input["query"]; ok {
			add("query")
		} else {
			add("search_query")
		}
	case "skill":
		add("skill")
	case "readmcpresourcetool", "readmcpresource":
		add("server")
		add("uri")
	case "listmcpresourcestool", "listmcpresources":
		add("server")
	case "notebookedit":
		add("notebook_path")
		add("cell_id")
	case "lsp":
		add("operation")
		add("filePath")
	}
	return truncateAgentLiveRunes(strings.Join(parts, " · "), maxAgentToolInputRunes)
}

func safeAgentToolInputScalar(value any) string {
	var text string
	switch typed := value.(type) {
	case string:
		text = typed
	case json.Number:
		text = typed.String()
	case int:
		text = strconv.Itoa(typed)
	case int64:
		text = strconv.FormatInt(typed, 10)
	case float64:
		text = strconv.FormatFloat(typed, 'f', -1, 64)
	case bool:
		text = strconv.FormatBool(typed)
	default:
		return ""
	}
	text = collapseAgentLiveWhitespace(sanitiseAgentLiveText(text))
	if text == "" || scanForTeamMemorySecrets(text) != "" {
		return ""
	}
	return truncateAgentLiveRunes(text, maxAgentToolInputRunes)
}

func agentToolResponsePreview(result types.ToolResultBlock) string {
	text := sanitiseAgentLiveText(result.TextContent())
	text = agentLiveSystemReminderRE.ReplaceAllString(text, "")
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	safe := lines[:0]
	for _, line := range lines {
		if scanForTeamMemorySecrets(line) != "" {
			continue
		}
		safe = append(safe, strings.TrimRightFunc(line, unicode.IsSpace))
	}
	text = strings.TrimSpace(strings.Join(safe, "\n"))
	return truncateAgentLiveTailRunes(text, maxAgentToolResponseRunes)
}

func sanitiseAgentLiveText(text string) string {
	text = sanitiseUserMessageBody(text)
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			return r
		case unicode.IsControl(r):
			return -1
		case r == '\u202a' || r == '\u202b' || r == '\u202c' || r == '\u202d' || r == '\u202e':
			return -1
		case r >= '\u2066' && r <= '\u2069':
			return -1
		default:
			return r
		}
	}, text)
}

func collapseAgentLiveWhitespace(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

func sanitizeAgentProgressURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return raw
	}
	parsed.User = nil
	parsed.Fragment = ""
	query := parsed.Query()
	for key := range query {
		normalized := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
		if strings.Contains(normalized, "token") || strings.Contains(normalized, "secret") ||
			strings.Contains(normalized, "password") || strings.Contains(normalized, "signature") ||
			normalized == "key" || normalized == "api_key" || normalized == "authorization" {
			query.Del(key)
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func truncateAgentLiveRunes(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	if limit == 1 {
		return "…"
	}
	return string(runes[:limit-1]) + "…"
}

func truncateAgentLiveTailRunes(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	if limit == 1 {
		return "…"
	}
	return "…" + string(runes[len(runes)-limit+1:])
}
