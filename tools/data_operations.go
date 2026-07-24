package tools

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"hash"
	"net/url"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// Base64EncodeTool encodes text to base64
type Base64EncodeTool struct{}

func (t *Base64EncodeTool) Name() string {
	return "Base64Encode"
}

func (t *Base64EncodeTool) Description() string {
	return "Encode text to base64"
}

func (t *Base64EncodeTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"text": map[string]any{
				"type":        "string",
				"description": "Text to encode",
			},
		},
		Required: []string{"text"},
	}
}

func (t *Base64EncodeTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	text, err := MustGetStringField(input, "text")
	if err != nil {
		return ErrorResponse(err), nil
	}

	encoded := base64.StdEncoding.EncodeToString([]byte(text))

	return ResponseJSON(map[string]string{
		"encoded": encoded,
	})
}

// Base64DecodeTool decodes base64 text
type Base64DecodeTool struct{}

func (t *Base64DecodeTool) Name() string {
	return "Base64Decode"
}

func (t *Base64DecodeTool) Description() string {
	return "Decode base64 text"
}

func (t *Base64DecodeTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"text": map[string]any{
				"type":        "string",
				"description": "Base64 encoded text to decode",
			},
		},
		Required: []string{"text"},
	}
}

func (t *Base64DecodeTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	text, err := MustGetStringField(input, "text")
	if err != nil {
		return ErrorResponse(err), nil
	}

	decoded, err := base64.StdEncoding.DecodeString(text)
	if err != nil {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyABase64DecodeFailed, err)), nil
	}

	return ResponseJSON(map[string]string{
		"decoded": string(decoded),
	})
}

// HashTool computes hash values
type HashTool struct{}

func (t *HashTool) Name() string {
	return "Hash"
}

func (t *HashTool) Description() string {
	return "Compute hash values (MD5, SHA256)"
}

func (t *HashTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"text": map[string]any{
				"type":        "string",
				"description": "Text to hash",
			},
			"algorithm": map[string]any{
				"type":        "string",
				"description": "Hash algorithm (md5 or sha256, default: sha256)",
			},
		},
		Required: []string{"text"},
	}
}

func (t *HashTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	text, err := MustGetStringField(input, "text")
	if err != nil {
		return ErrorResponse(err), nil
	}

	algorithm := GetStringField(input, "algorithm", "sha256")

	var h hash.Hash
	switch algorithm {
	case "md5":
		h = md5.New()
	case "sha256":
		h = sha256.New()
	default:
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyAHashUnsupported, algorithm)), nil
	}

	h.Write([]byte(text))
	hashValue := hex.EncodeToString(h.Sum(nil))

	return ResponseJSON(map[string]string{
		"hash":      hashValue,
		"algorithm": algorithm,
	})
}

// JsonFormatTool formats JSON
type JsonFormatTool struct{}

func (t *JsonFormatTool) Name() string {
	return "JsonFormat"
}

func (t *JsonFormatTool) Description() string {
	return "Format and pretty-print JSON"
}

func (t *JsonFormatTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"json": map[string]any{
				"type":        "string",
				"description": "JSON string to format",
			},
			"indent": map[string]any{
				"type":        "number",
				"description": "Indentation spaces (default: 2)",
			},
		},
		Required: []string{"json"},
	}
}

func (t *JsonFormatTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	jsonStr, err := MustGetStringField(input, "json")
	if err != nil {
		return ErrorResponse(err), nil
	}

	indent := GetIntField(input, "indent", 2)

	// Parse JSON
	var obj interface{}
	err = json.Unmarshal([]byte(jsonStr), &obj)
	if err != nil {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyAJSONInvalid, err)), nil
	}

	// Format with indentation
	indentStr := ""
	for i := 0; i < indent; i++ {
		indentStr += " "
	}

	formatted, err := json.MarshalIndent(obj, "", indentStr)
	if err != nil {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyAJSONFormattingFailed, err)), nil
	}

	return ResponseJSON(map[string]string{
		"formatted": string(formatted),
	})
}

// JsonParseTool parses and extracts JSON fields
type JsonParseTool struct{}

func (t *JsonParseTool) Name() string {
	return "JsonParse"
}

func (t *JsonParseTool) Description() string {
	return "Parse JSON and extract fields"
}

func (t *JsonParseTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"json": map[string]any{
				"type":        "string",
				"description": "JSON string to parse",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Field path (dot-separated, e.g., 'user.name')",
			},
		},
		Required: []string{"json"},
	}
}

func (t *JsonParseTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	jsonStr, err := MustGetStringField(input, "json")
	if err != nil {
		return ErrorResponse(err), nil
	}

	path := GetStringField(input, "path", "")

	// Parse JSON
	var obj interface{}
	err = json.Unmarshal([]byte(jsonStr), &obj)
	if err != nil {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyAJSONInvalid, err)), nil
	}

	// If no path specified, return the whole object
	if path == "" {
		return ResponseJSON(map[string]interface{}{
			"value": obj,
		})
	}

	// Extract field by path
	parts := strings.Split(path, ".")
	current := obj
	for _, part := range parts {
		if mapObj, ok := current.(map[string]interface{}); ok {
			current = mapObj[part]
		} else {
			return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyAJSONPathNotFound, path)), nil
		}
	}

	return ResponseJSON(map[string]interface{}{
		"value": current,
	})
}

// HexEncodeTool encodes text to hexadecimal
type HexEncodeTool struct{}

func (t *HexEncodeTool) Name() string {
	return "HexEncode"
}

func (t *HexEncodeTool) Description() string {
	return "Encode text to hexadecimal"
}

func (t *HexEncodeTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"text": map[string]any{
				"type":        "string",
				"description": "Text to encode",
			},
		},
		Required: []string{"text"},
	}
}

func (t *HexEncodeTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	text, err := MustGetStringField(input, "text")
	if err != nil {
		return ErrorResponse(err), nil
	}

	encoded := hex.EncodeToString([]byte(text))

	return ResponseJSON(map[string]string{
		"encoded": encoded,
	})
}

// HexDecodeTool decodes hexadecimal text
type HexDecodeTool struct{}

func (t *HexDecodeTool) Name() string {
	return "HexDecode"
}

func (t *HexDecodeTool) Description() string {
	return "Decode hexadecimal text"
}

func (t *HexDecodeTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"text": map[string]any{
				"type":        "string",
				"description": "Hexadecimal encoded text to decode",
			},
		},
		Required: []string{"text"},
	}
}

func (t *HexDecodeTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	text, err := MustGetStringField(input, "text")
	if err != nil {
		return ErrorResponse(err), nil
	}

	decoded, err := hex.DecodeString(text)
	if err != nil {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyAHexDecodeFailed, err)), nil
	}

	return ResponseJSON(map[string]string{
		"decoded": string(decoded),
	})
}

// UrlEncodeTool URL-encodes text
type UrlEncodeTool struct{}

func (t *UrlEncodeTool) Name() string {
	return "UrlEncode"
}

func (t *UrlEncodeTool) Description() string {
	return "URL-encode text"
}

func (t *UrlEncodeTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"text": map[string]any{
				"type":        "string",
				"description": "Text to URL-encode",
			},
		},
		Required: []string{"text"},
	}
}

func (t *UrlEncodeTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	text, err := MustGetStringField(input, "text")
	if err != nil {
		return ErrorResponse(err), nil
	}

	// Use net/url for URL encoding
	encoded := url.QueryEscape(text)

	return ResponseJSON(map[string]string{
		"encoded": encoded,
	})
}
