package provider

import (
	"encoding/json"
	"sort"

	"github.com/agent-dance/luban/types"
)

func convertToolsToResponsesAPIWithStrictMode(tools []types.ToolDefinition, strictMode bool) []map[string]any {
	return convertToolsToResponsesAPIForSemantics(tools, strictMode, ResponsesSemanticsCompatible)
}

func convertToolsToResponsesAPIForSemantics(tools []types.ToolDefinition, strictMode bool, semantics ResponsesSemantics) []map[string]any {
	result := make([]map[string]any, 0, len(tools))
	for _, t := range canonicalToolDefinitions(tools) {
		if t.IsCustom() {
			tool := map[string]any{
				"type":        "custom",
				"name":        t.Name,
				"description": t.Description,
			}
			if t.Format != nil {
				tool["format"] = map[string]any{
					"type":       t.Format.Type,
					"syntax":     t.Format.Syntax,
					"definition": t.Format.Definition,
				}
			}
			result = append(result, tool)
			continue
		}
		schema := t.InputSchema
		if schema.Properties == nil {
			schema.Properties = map[string]any{}
		}
		strict := strictMode && t.Strict
		var parameters any = schema
		if semantics == ResponsesSemanticsOpenAIPublic {
			parameters = canonicalOpenAIResponsesToolSchema(schema, strict)
		}
		tool := map[string]any{
			"type":        "function",
			"name":        t.Name,
			"description": t.Description,
			"parameters":  parameters,
		}
		if strict {
			tool["strict"] = true
		}
		result = append(result, tool)
	}
	return result
}

func definitionsHaveCustomTools(definitions []types.ToolDefinition) bool {
	for _, definition := range definitions {
		if definition.IsCustom() {
			return true
		}
	}
	return false
}

func responseToolChoiceType(definitions []types.ToolDefinition, name string) string {
	for _, definition := range definitions {
		if definition.Name == name && definition.IsCustom() {
			return "custom"
		}
	}
	return "function"
}

// canonicalOpenAIResponsesToolSchema returns a detached, OpenAI-public wire
// schema. Local schemas intentionally carry semantic annotations used by tool
// introspection; the Responses API must never receive those private markers.
func canonicalOpenAIResponsesToolSchema(schema types.JSONSchema, strict bool) map[string]any {
	raw, err := json.Marshal(schema)
	if err != nil {
		return map[string]any{}
	}
	var detached map[string]any
	if err := json.Unmarshal(raw, &detached); err != nil {
		return map[string]any{}
	}
	return sanitizeOpenAIResponsesSchemaNode(detached, strict).(map[string]any)
}

func sanitizeOpenAIResponsesSchemaNode(value any, strict bool) any {
	switch node := value.(type) {
	case map[string]any:
		clean := make(map[string]any, len(node))
		for key, child := range node {
			switch key {
			case "_semantic", "integer", "default":
				continue
			}
			clean[key] = sanitizeOpenAIResponsesSchemaKeyword(key, child, strict)
		}
		if strict && openAIResponsesSchemaIncludesType(clean["type"], "object") {
			properties, _ := clean["properties"].(map[string]any)
			if properties == nil {
				properties = map[string]any{}
				clean["properties"] = properties
			}
			clean["required"] = canonicalRequiredProperties(clean["required"], properties)
			clean["additionalProperties"] = false
		}
		return clean
	case []any:
		clean := make([]any, len(node))
		for index, child := range node {
			clean[index] = sanitizeOpenAIResponsesSchemaNode(child, strict)
		}
		return clean
	default:
		return value
	}
}

func sanitizeOpenAIResponsesSchemaKeyword(keyword string, value any, strict bool) any {
	switch keyword {
	case "properties", "patternProperties", "$defs", "definitions", "dependentSchemas":
		named, ok := value.(map[string]any)
		if !ok {
			return value
		}
		clean := make(map[string]any, len(named))
		for name, schema := range named {
			// Property and definition names are data, not schema keywords.
			clean[name] = sanitizeOpenAIResponsesSchemaNode(schema, strict)
		}
		return clean
	case "items", "prefixItems", "contains", "additionalProperties", "unevaluatedProperties",
		"propertyNames", "not", "if", "then", "else", "oneOf", "anyOf", "allOf":
		return sanitizeOpenAIResponsesSchemaNode(value, strict)
	default:
		// canonicalOpenAIResponsesToolSchema already detached every value through
		// JSON, so annotations and literal enum values are safe to retain as-is.
		return value
	}
}

func openAIResponsesSchemaIncludesType(value any, target string) bool {
	switch typed := value.(type) {
	case string:
		return typed == target
	case []any:
		for _, candidate := range typed {
			if candidate == target {
				return true
			}
		}
	}
	return false
}

func canonicalRequiredProperties(value any, properties map[string]any) []string {
	required := make([]string, 0, len(properties))
	seen := make(map[string]struct{}, len(properties))
	if entries, ok := value.([]any); ok {
		for _, entry := range entries {
			name, ok := entry.(string)
			if !ok {
				continue
			}
			if _, exists := properties[name]; !exists {
				continue
			}
			if _, duplicate := seen[name]; duplicate {
				continue
			}
			seen[name] = struct{}{}
			required = append(required, name)
		}
	}
	missing := make([]string, 0, len(properties)-len(required))
	for name := range properties {
		if _, exists := seen[name]; !exists {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	return append(required, missing...)
}
