package mcp

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/tools/toolbase"
	"github.com/agent-dance/luban/types"
)

// mcp_render.go transforms MCP tools/call and resources/read envelopes into
// typed model-consumable ToolResults.

type mcpContentItem struct {
	Type        string           `json:"type,omitempty"`
	Text        string           `json:"text,omitempty"`
	URI         string           `json:"uri,omitempty"`
	Name        string           `json:"name,omitempty"`
	Description string           `json:"description,omitempty"`
	MimeType    string           `json:"mimeType,omitempty"`
	Data        string           `json:"data,omitempty"`
	Blob        string           `json:"blob,omitempty"`
	Resource    *mcpResourceItem `json:"resource,omitempty"`
	Annotations json.RawMessage  `json:"annotations,omitempty"`
	Meta        json.RawMessage  `json:"_meta,omitempty"`
}

type mcpResourceItem struct {
	URI         string          `json:"uri,omitempty"`
	Name        string          `json:"name,omitempty"`
	Description string          `json:"description,omitempty"`
	MimeType    string          `json:"mimeType,omitempty"`
	Text        string          `json:"text,omitempty"`
	Blob        string          `json:"blob,omitempty"`
	Annotations json.RawMessage `json:"annotations,omitempty"`
	Meta        json.RawMessage `json:"_meta,omitempty"`
}

type mcpToolsCallEnvelope struct {
	ToolResult        json.RawMessage   `json:"toolResult,omitempty"`
	Content           []json.RawMessage `json:"content,omitempty"`
	StructuredContent json.RawMessage   `json:"structuredContent,omitempty"`
	Meta              json.RawMessage   `json:"_meta,omitempty"`
	IsError           bool              `json:"isError,omitempty"`
}

type mcpTransformedContent struct {
	Blocks      []types.ContentBlock
	ContainsRaw bool
}

func newMCPTextBlock(text string) types.ContentBlock {
	return types.TextBlock{Type: types.ContentTypeText, Text: text}
}

// renderMCPCallToolResult preserves MCP metadata and transforms content[]
// items into typed blocks when the provider layer can consume them.
func renderMCPCallToolResult(raw json.RawMessage, serverName, toolName string) types.ToolResult {
	return renderMCPCallToolResultAt(raw, serverName, toolName, "")
}

func renderMCPCallToolResultAt(raw json.RawMessage, serverName, toolName, toolResultsDir string) types.ToolResult {
	var env mcpToolsCallEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return mcpTextToolResult(string(raw), false, nil)
	}

	metadata := mcpResultMetadata("toolResult", serverName, toolName)
	if rawJSONPresent(env.Meta) {
		metadata["mcp._meta"] = string(compactJSONRaw(env.Meta))
	}

	var blocks []types.ContentBlock
	resultType := "toolResult"
	formatDescription := getMCPFormatDescription(resultType, "")

	if rawJSONPresent(env.ToolResult) {
		text := mcpRawScalarToString(env.ToolResult)
		if text != "" {
			blocks = append(blocks, newMCPTextBlock(text))
		}
	}

	if rawJSONPresent(env.StructuredContent) {
		structured := compactJSONRaw(env.StructuredContent)
		schema := inferMCPCompactSchemaFromRaw(structured)
		metadata["mcp.structuredContent"] = string(structured)
		if schema != "" {
			metadata["mcp.structuredContentSchema"] = schema
		}
		blocks = append(blocks, newMCPTextBlock(string(structured)))
		resultType = "structuredContent"
		formatDescription = getMCPFormatDescription(resultType, schema)
	}

	if len(env.Content) > 0 {
		if resultType == "structuredContent" {
			resultType = "structuredContent+contentArray"
		} else {
			resultType = "contentArray"
			formatDescription = getMCPFormatDescription(resultType, inferMCPCompactSchemaFromRaw(mustMarshalRawArray(env.Content)))
		}
		for _, rawItem := range env.Content {
			transformed := transformMCPResultContent(rawItem, serverName, toolResultsDir)
			blocks = append(blocks, transformed.Blocks...)
		}
	}

	if len(blocks) == 0 {
		text := strings.TrimSpace(string(raw))
		result := mcpTextToolResult(text, env.IsError, metadata)
		return maybePersistLargeMCPResultAt(result, resultType, formatDescription, serverName, toolName, toolResultsDir)
	}

	metadata["mcp.resultType"] = resultType
	result := types.ToolResult{
		Content:       mcpBlocksSummary(blocks),
		ContentBlocks: blocks,
		IsError:       env.IsError,
		Metadata:      metadata,
	}
	return maybePersistLargeMCPResultAt(result, resultType, formatDescription, serverName, toolName, toolResultsDir)
}

func transformMCPResultContent(raw json.RawMessage, serverName, toolResultsDir string) mcpTransformedContent {
	var item mcpContentItem
	if err := json.Unmarshal(raw, &item); err != nil {
		return mcpTransformedContent{Blocks: []types.ContentBlock{newMCPTextBlock(string(raw))}, ContainsRaw: true}
	}

	switch item.Type {
	case "text":
		return mcpTransformedContent{Blocks: []types.ContentBlock{newMCPTextBlock(renderMCPText(item.Text, item.MimeType))}}
	case "image":
		return mcpTransformedContent{Blocks: transformMCPImageData(firstNonEmptyMCP(item.Data, item.Blob), item.MimeType, serverName, toolRuntimeFormat(i18n.KeyToolRuntimeMCPSourceImage, serverName), toolResultsDir)}
	case "audio":
		return mcpTransformedContent{Blocks: persistMCPBase64BlobToBlocks(firstNonEmptyMCP(item.Data, item.Blob), item.MimeType, serverName, toolRuntimeFormat(i18n.KeyToolRuntimeMCPSourceAudio, serverName), toolResultsDir)}
	case "resource":
		if item.Resource == nil {
			return mcpTransformedContent{}
		}
		return mcpTransformedContent{Blocks: transformMCPResourceItem(*item.Resource, serverName, toolResultsDir)}
	case "resource_link":
		return mcpTransformedContent{Blocks: []types.ContentBlock{newMCPTextBlock(renderMCPResourceLink(item.Name, item.URI, item.Description))}}
	default:
		if item.Text != "" {
			return mcpTransformedContent{Blocks: []types.ContentBlock{newMCPTextBlock(renderMCPText(item.Text, item.MimeType))}}
		}
		if blob := firstNonEmptyMCP(item.Data, item.Blob); blob != "" {
			if isMCPImageMime(item.MimeType) {
				return mcpTransformedContent{Blocks: transformMCPImageData(blob, item.MimeType, serverName, toolRuntimeFormat(i18n.KeyToolRuntimeMCPSourceImage, serverName), toolResultsDir)}
			}
			return mcpTransformedContent{Blocks: persistMCPBase64BlobToBlocks(blob, item.MimeType, serverName, toolRuntimeFormat(i18n.KeyToolRuntimeMCPSourceBlob, serverName), toolResultsDir)}
		}
		return mcpTransformedContent{Blocks: []types.ContentBlock{newMCPTextBlock(string(compactJSONRaw(raw)))}, ContainsRaw: true}
	}
}

func transformMCPResourceItem(resource mcpResourceItem, serverName, toolResultsDir string) []types.ContentBlock {
	var prefix string
	if strings.TrimSpace(resource.URI) != "" {
		prefix = toolRuntimeFormat(i18n.KeyToolRuntimeMCPSourceResourceAt, serverName, resource.URI)
	} else {
		prefix = toolRuntimeFormat(i18n.KeyToolRuntimeMCPSourceResource, serverName)
	}

	if resource.Text != "" {
		return []types.ContentBlock{newMCPTextBlock(prefix + renderMCPText(resource.Text, resource.MimeType))}
	}
	if resource.Blob != "" {
		if isMCPImageMime(resource.MimeType) {
			blocks := []types.ContentBlock{newMCPTextBlock(strings.TrimSpace(prefix))}
			blocks = append(blocks, transformMCPImageData(resource.Blob, resource.MimeType, serverName, prefix, toolResultsDir)...)
			return blocks
		}
		return persistMCPBase64BlobToBlocks(resource.Blob, resource.MimeType, serverName, prefix, toolResultsDir)
	}
	return []types.ContentBlock{newMCPTextBlock(strings.TrimSpace(prefix) + " " + mcpResourceSummary(resource))}
}

func transformMCPImageData(blob, mimeType, serverName, sourceDescription, toolResultsDir string) []types.ContentBlock {
	clean := stripBase64Whitespace(blob)
	raw, err := base64.StdEncoding.DecodeString(clean)
	if err != nil {
		return []types.ContentBlock{newMCPTextBlock(toolRuntimeFormat(i18n.KeyToolRuntimeMCPInvalidBase64Image, sourceDescription, err))}
	}
	mediaType := normalizeMCPImageMime(mimeType)
	if mediaType == "image/svg+xml" {
		return persistMCPBlobToBlocks(raw, mimeType, serverName, sourceDescription, toolResultsDir)
	}
	if len(raw) > toolbase.ModelImageMaxBytes {
		resized, resizedType, resizeErr := toolbase.ResizeImageBytes(raw, mediaType, toolbase.ModelImageMaxBytes)
		if resizeErr != nil {
			return persistMCPBlobToBlocks(raw, mimeType, serverName, sourceDescription, toolResultsDir)
		}
		raw = resized
		mediaType = resizedType
		clean = base64.StdEncoding.EncodeToString(raw)
	}
	return []types.ContentBlock{toolbase.NewBase64ImageBlock(clean, mediaType)}
}

func persistMCPBase64BlobToBlocks(blob, mimeType, serverName, sourceDescription, toolResultsDir string) []types.ContentBlock {
	raw, err := base64.StdEncoding.DecodeString(stripBase64Whitespace(blob))
	if err != nil {
		return []types.ContentBlock{newMCPTextBlock(toolRuntimeFormat(i18n.KeyToolRuntimeMCPInvalidBase64Binary, sourceDescription, err))}
	}
	return persistMCPBlobToBlocks(raw, mimeType, serverName, sourceDescription, toolResultsDir)
}

func persistMCPBlobToBlocks(raw []byte, mimeType, serverName, sourceDescription, toolResultsDir string) []types.ContentBlock {
	persistID := newMCPPersistID(serverName, "blob")
	result := persistMCPBinaryContentAt(raw, mimeType, persistID, toolResultsDir)
	if result.Error != "" {
		return []types.ContentBlock{newMCPTextBlock(toolRuntimeFormat(i18n.KeyToolRuntimeMCPBinarySaveFailed, sourceDescription, fallbackMCPMime(mimeType), len(raw), result.Error))}
	}
	return []types.ContentBlock{newMCPTextBlock(getMCPBinaryBlobSavedMessage(result.Filepath, mimeType, result.Size, sourceDescription))}
}

func renderMCPText(text, mimeType string) string {
	if pretty, ok := prettyMCPJSONTextForMime(text, mimeType); ok {
		return pretty
	}
	return text
}

func renderMCPResourceLink(name, uri, description string) string {
	displayName := strings.TrimSpace(name)
	if displayName == "" {
		displayName = strings.TrimSpace(uri)
	}
	text := toolRuntimeFormat(i18n.KeyToolRuntimeMCPResourceLink, displayName, strings.TrimSpace(uri))
	if strings.TrimSpace(description) != "" {
		text += " (" + strings.TrimSpace(description) + ")"
	}
	return text
}

func mcpBlocksSummary(blocks []types.ContentBlock) string {
	if len(blocks) == 1 {
		if text, ok := blocks[0].(types.TextBlock); ok {
			return text.Text
		}
	}
	return CreateMCPContentSummary(blocks)
}

func mcpTextToolResult(content string, isError bool, metadata map[string]string) types.ToolResult {
	result := types.ToolResult{Content: content, IsError: isError, Metadata: metadata}
	if strings.TrimSpace(content) != "" {
		result.ContentBlocks = []types.ContentBlock{newMCPTextBlock(content)}
	}
	return result
}

func mcpResultMetadata(resultType, serverName, toolName string) map[string]string {
	metadata := map[string]string{
		"mcp.resultType": resultType,
	}
	if strings.TrimSpace(serverName) != "" {
		metadata["mcp.serverName"] = serverName
	}
	if strings.TrimSpace(toolName) != "" {
		metadata["mcp.toolName"] = toolName
	}
	return metadata
}

func mcpRawScalarToString(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var v any
	if err := json.Unmarshal(raw, &v); err == nil {
		return fmt.Sprint(v)
	}
	return string(raw)
}

func mcpResourceSummary(resource mcpResourceItem) string {
	data, err := json.Marshal(resource)
	if err != nil {
		return ""
	}
	return string(data)
}

func prettyMCPJSONTextForMime(text, mimeType string) (string, bool) {
	if !strings.HasPrefix(normalizeMCPMime(mimeType), "application/json") {
		return "", false
	}
	return prettyMCPJSONText(text)
}

func prettyMCPJSONText(text string) (string, bool) {
	if strings.TrimSpace(text) == "" {
		return "", false
	}
	var holder json.RawMessage
	if err := json.Unmarshal([]byte(text), &holder); err != nil {
		return "", false
	}
	pretty, err := json.MarshalIndent(holder, "", "  ")
	if err != nil {
		return "", false
	}
	return string(pretty), true
}

func compactJSONRaw(raw json.RawMessage) json.RawMessage {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return raw
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, trimmed); err != nil {
		return json.RawMessage(append([]byte(nil), trimmed...))
	}
	return json.RawMessage(buf.Bytes())
}

func mustMarshalRawArray(items []json.RawMessage) json.RawMessage {
	data, err := json.Marshal(items)
	if err != nil {
		return nil
	}
	return compactJSONRaw(data)
}

func rawJSONPresent(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null"))
}

func normalizeMCPMime(mimeType string) string {
	return strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0]))
}

func fallbackMCPMime(mimeType string) string {
	if strings.TrimSpace(mimeType) == "" {
		return toolRuntimeText(i18n.KeyToolRuntimeMCPUnknownType)
	}
	return strings.TrimSpace(mimeType)
}

func normalizeMCPImageMime(mimeType string) string {
	mime := normalizeMCPMime(mimeType)
	if mime == "" {
		return "image/png"
	}
	return mime
}

func isMCPImageMime(mimeType string) bool {
	switch normalizeMCPMime(mimeType) {
	case "image/png", "image/jpeg", "image/jpg", "image/gif", "image/webp", "image/svg+xml":
		return true
	default:
		return false
	}
}

func stripBase64Whitespace(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\r', '\t', ' ':
			return -1
		default:
			return r
		}
	}, s)
}

func firstNonEmptyMCP(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
