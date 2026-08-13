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
				"name":        responsesToolNameForSemantics(semantics, t.Name, true),
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
		strictDowngraded := false
		var parameters any = schema
		switch semantics {
		case ResponsesSemanticsOpenAIPublic:
			parameters = canonicalOpenAIResponsesToolSchema(schema, strict)
		case ResponsesSemanticsDeepSeek:
			// DeepSeek's strict contract cannot faithfully represent optional
			// properties: it requires every declared property to be required and
			// does not have OpenAI's documented nullable-optional projection. Keep
			// strict mode for schemas that are already fully required, but preserve
			// the local optional shape for tools such as Inspect.
			if strict && responsesSchemaHasOptionalProperties(schema) {
				strict = false
				strictDowngraded = true
			}
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
		} else if strictDowngraded {
			// Distinguish a provider capability downgrade from a tool that never
			// requested strict mode. DeepSeek must receive this decision explicitly.
			tool["strict"] = false
		}
		result = append(result, tool)
	}
	return result
}

func responsesCustomToolDefinitionsSupported(semantics ResponsesSemantics, model string, definitions []types.ToolDefinition) bool {
	if !supportsOpenAIResponsesCustomTools(semantics, model) {
		return false
	}
	if semantics != ResponsesSemanticsDeepSeek {
		return true
	}
	for _, definition := range definitions {
		if definition.IsCustom() && definition.Name != "ApplyPatch" && definition.Name != "apply_patch" {
			return false
		}
	}
	return true
}

func responsesToolNameForSemantics(semantics ResponsesSemantics, name string, custom bool) string {
	if semantics == ResponsesSemanticsDeepSeek && custom && (name == "ApplyPatch" || name == "apply_patch") {
		return "apply_patch"
	}
	return name
}

func responsesLocalToolNameForSemantics(semantics ResponsesSemantics, name string, custom bool) string {
	if semantics == ResponsesSemanticsDeepSeek && custom && name == "apply_patch" {
		return "ApplyPatch"
	}
	return name
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

// canonicalOpenAIResponsesToolSchema returns a detached Responses wire schema.
// Local schemas intentionally carry semantic annotations used by tool
// introspection; provider APIs must never receive those private markers. In
// strict mode, local optional properties become required nullable properties,
// which is OpenAI's supported representation for optional function input.
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
			requiredBeforeProjection := responsesSchemaRequiredPropertySet(clean["required"], properties)
			for name, property := range properties {
				if _, required := requiredBeforeProjection[name]; required {
					continue
				}
				properties[name] = openAIResponsesNullableSchema(property)
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

func openAIResponsesNullableSchema(value any) any {
	node, ok := value.(map[string]any)
	if !ok {
		return value
	}
	if openAIResponsesSchemaAcceptsNull(node) {
		return node
	}
	return map[string]any{
		"anyOf": []any{node, map[string]any{"type": "null"}},
	}
}

// openAIResponsesSchemaAcceptsNull recognizes schemas that already admit null
// without weakening sibling constraints such as enum or const. It is
// deliberately conservative for references and conditionals: an unnecessary
// outer nullable branch is safe, while incorrectly assuming nullability is not.
func openAIResponsesSchemaAcceptsNull(node map[string]any) bool {
	if _, hasRef := node["$ref"]; hasRef {
		return false
	}
	if _, conditional := node["if"]; conditional {
		return false
	}
	if typed, present := node["type"]; present && !openAIResponsesSchemaIncludesType(typed, "null") {
		return false
	}
	if enum, present := node["enum"]; present {
		values, ok := enum.([]any)
		if !ok || !responsesSchemaValuesIncludeNull(values) {
			return false
		}
	}
	if constant, present := node["const"]; present && constant != nil {
		return false
	}
	if alternative, present := node["not"]; present {
		if forbidden, ok := alternative.(map[string]any); !ok || openAIResponsesSchemaAcceptsNull(forbidden) {
			return false
		}
	}
	for _, keyword := range []string{"allOf", "anyOf", "oneOf"} {
		raw, present := node[keyword]
		if !present {
			continue
		}
		alternatives, ok := raw.([]any)
		if !ok || len(alternatives) == 0 {
			return false
		}
		accepting := 0
		for _, rawAlternative := range alternatives {
			alternative, ok := rawAlternative.(map[string]any)
			if ok && openAIResponsesSchemaAcceptsNull(alternative) {
				accepting++
			}
		}
		switch keyword {
		case "allOf":
			if accepting != len(alternatives) {
				return false
			}
		case "anyOf":
			if accepting == 0 {
				return false
			}
		case "oneOf":
			if accepting != 1 {
				return false
			}
		}
	}
	return true
}

func responsesSchemaValuesIncludeNull(values []any) bool {
	for _, value := range values {
		if value == nil {
			return true
		}
	}
	return false
}

func responsesSchemaHasOptionalProperties(schema types.JSONSchema) bool {
	raw, err := json.Marshal(schema)
	if err != nil {
		return true
	}
	var detached any
	if err := json.Unmarshal(raw, &detached); err != nil {
		return true
	}
	return responsesSchemaNodeHasOptionalProperties(detached)
}

func responsesSchemaNodeHasOptionalProperties(value any) bool {
	switch node := value.(type) {
	case map[string]any:
		if openAIResponsesSchemaIncludesType(node["type"], "object") {
			if properties, ok := node["properties"].(map[string]any); ok {
				required := responsesSchemaRequiredPropertySet(node["required"], properties)
				if len(required) != len(properties) {
					return true
				}
			}
		}
		for _, child := range node {
			if responsesSchemaNodeHasOptionalProperties(child) {
				return true
			}
		}
	case []any:
		for _, child := range node {
			if responsesSchemaNodeHasOptionalProperties(child) {
				return true
			}
		}
	}
	return false
}

func responsesSchemaRequiredPropertySet(value any, properties map[string]any) map[string]struct{} {
	required := make(map[string]struct{}, len(properties))
	var entries []any
	switch typed := value.(type) {
	case []any:
		entries = typed
	case []string:
		entries = make([]any, len(typed))
		for index, entry := range typed {
			entries[index] = entry
		}
	}
	for _, entry := range entries {
		name, ok := entry.(string)
		if !ok {
			continue
		}
		if _, exists := properties[name]; exists {
			required[name] = struct{}{}
		}
	}
	return required
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
