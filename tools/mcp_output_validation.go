package tools

import (
	"encoding/json"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

const (
	defaultMaxMCPOutputTokens = 25000
	mcpImageTokenEstimate     = 1600
)

func maybePersistLargeMCPResult(result types.ToolResult, resultType, formatDescription, serverName, toolName string) types.ToolResult {
	if !mcpContentNeedsPersistence(result) {
		return result
	}

	if isMCPEnvDefinedFalsy("ENABLE_MCP_LARGE_OUTPUT_FILES") {
		return truncateMCPToolResult(result)
	}

	if mcpToolResultContainsImages(result) {
		return truncateMCPToolResult(result)
	}

	content, isJSON := mcpPersistableContentString(result, resultType)
	if strings.TrimSpace(content) == "" {
		return result
	}
	persistID := newMCPPersistID(strings.TrimSpace(serverName)+"-"+strings.TrimSpace(toolName), "large-output")
	persisted := persistMCPTextOutput(content, persistID, isJSON)
	if persisted.Error != "" {
		msg := toolRuntimeFormat(i18n.KeyToolRuntimeMCPLargeOutputSaveFailed, formatMCPInteger(len(content)), persisted.Error) +
			toolRuntimeText(i18n.KeyToolMCPPaginationHint)
		result.Content = msg
		result.ContentBlocks = []types.ContentBlock{newTextBlock(msg)}
		return result
	}

	if strings.TrimSpace(formatDescription) == "" {
		formatDescription = getMCPFormatDescription(resultType, "")
	}
	msg := getMCPLargeOutputInstructions(persisted.Filepath, persisted.OriginalSize, formatDescription)
	result.Content = msg
	result.ContentBlocks = []types.ContentBlock{newTextBlock(msg)}
	if result.Metadata == nil {
		result.Metadata = map[string]string{}
	}
	result.Metadata["mcp.largeOutputPath"] = persisted.Filepath
	result.Metadata["mcp.largeOutputChars"] = strconv.Itoa(persisted.OriginalSize)
	result.Metadata["mcp.largeOutputFormat"] = formatDescription
	return result
}

func mcpContentNeedsPersistence(result types.ToolResult) bool {
	return estimateMCPToolResultTokens(result) > getMaxMCPOutputTokens()
}

func estimateMCPToolResultTokens(result types.ToolResult) int {
	if len(result.ContentBlocks) == 0 {
		return roughMCPTokenCount(result.Content)
	}
	total := 0
	for _, block := range result.ContentBlocks {
		switch typed := block.(type) {
		case types.TextBlock:
			total += roughMCPTokenCount(typed.Text)
		case types.ImageBlock:
			total += mcpImageTokenEstimate
		case types.DocumentBlock:
			total += mcpImageTokenEstimate
		default:
			total += roughMCPTokenCount(string(block.GetType()))
		}
	}
	return total
}

func truncateMCPToolResult(result types.ToolResult) types.ToolResult {
	maxChars := getMaxMCPOutputChars()
	msg := getMCPTruncationMessage()
	if len(result.ContentBlocks) == 0 {
		result.Content = truncateMCPString(result.Content, maxChars) + msg
		return result
	}

	var blocks []types.ContentBlock
	used := 0
	for _, block := range result.ContentBlocks {
		switch typed := block.(type) {
		case types.TextBlock:
			remaining := maxChars - used
			if remaining <= 0 {
				continue
			}
			text := typed.Text
			if len(text) > remaining {
				text = text[:remaining]
			}
			blocks = append(blocks, newTextBlock(text))
			used += len(text)
		case types.ImageBlock:
			imageChars := mcpImageTokenEstimate * 4
			if used+imageChars <= maxChars {
				blocks = append(blocks, typed)
				used += imageChars
			}
		default:
			blocks = append(blocks, block)
		}
	}
	blocks = append(blocks, newTextBlock(msg))
	result.ContentBlocks = blocks
	result.Content = mcpBlocksSummary(blocks)
	return result
}

func mcpPersistableContentString(result types.ToolResult, resultType string) (string, bool) {
	if len(result.ContentBlocks) == 0 {
		return result.Content, false
	}
	if strings.Contains(resultType, "contentArray") {
		data, err := json.MarshalIndent(result.ContentBlocks, "", "  ")
		if err != nil {
			return result.Content, false
		}
		return string(data), true
	}
	onlyText := true
	for _, block := range result.ContentBlocks {
		if _, ok := block.(types.TextBlock); !ok {
			onlyText = false
			break
		}
	}
	if onlyText {
		var parts []string
		for _, block := range result.ContentBlocks {
			parts = append(parts, block.(types.TextBlock).Text)
		}
		return strings.Join(parts, "\n"), false
	}
	data, err := json.MarshalIndent(result.ContentBlocks, "", "  ")
	if err != nil {
		return result.Content, false
	}
	return string(data), true
}

func mcpToolResultContainsImages(result types.ToolResult) bool {
	for _, block := range result.ContentBlocks {
		if _, ok := block.(types.ImageBlock); ok {
			return true
		}
	}
	return false
}

func inferMCPCompactSchemaFromRaw(raw json.RawMessage) string {
	if !rawJSONPresent(raw) {
		return ""
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return inferMCPCompactSchema(value, 2)
}

func inferMCPCompactSchema(value any, depth int) string {
	switch typed := value.(type) {
	case nil:
		return "null"
	case []any:
		if len(typed) == 0 {
			return "[]"
		}
		return "[" + inferMCPCompactSchema(typed[0], depth-1) + "]"
	case map[string]any:
		if depth <= 0 {
			return "{...}"
		}
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		parts := make([]string, 0, min(len(typed), 10))
		for i, key := range keys {
			if i >= 10 {
				break
			}
			parts = append(parts, key+": "+inferMCPCompactSchema(typed[key], depth-1))
		}
		suffix := ""
		if len(typed) > 10 {
			suffix = ", ..."
		}
		return "{" + strings.Join(parts, ", ") + suffix + "}"
	case string:
		return "string"
	case bool:
		return "boolean"
	case float64:
		return "number"
	default:
		return "unknown"
	}
}

func getMaxMCPOutputTokens() int {
	if raw := strings.TrimSpace(os.Getenv("MAX_MCP_OUTPUT_TOKENS")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			return parsed
		}
	}
	return defaultMaxMCPOutputTokens
}

func getMaxMCPOutputChars() int {
	return getMaxMCPOutputTokens() * 4
}

func getMCPTruncationMessage() string {
	return toolRuntimeFormat(i18n.KeyToolRuntimeMCPOutputTruncated, getMaxMCPOutputTokens()) +
		toolRuntimeText(i18n.KeyToolMCPTruncationHint)
}

func truncateMCPString(content string, maxChars int) string {
	if maxChars <= 0 {
		return ""
	}
	if len(content) <= maxChars {
		return content
	}
	return content[:maxChars]
}

func roughMCPTokenCount(content string) int {
	if content == "" {
		return 0
	}
	return (len(content) + 3) / 4
}

func isMCPEnvDefinedFalsy(name string) bool {
	value, ok := os.LookupEnv(name)
	if !ok {
		return false
	}
	trimmed := strings.ToLower(strings.TrimSpace(value))
	switch trimmed {
	case "", "0", "false", "no", "off":
		return true
	default:
		return false
	}
}
