package provider

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/agent-dance/luban/types"
)

// canonicalToolDefinitions returns a shallow copy in a deterministic wire
// order. Provider serializers must not reorder the registry-owned input slice.
func canonicalToolDefinitions(definitions []types.ToolDefinition) []types.ToolDefinition {
	type keyedDefinition struct {
		definition types.ToolDefinition
		key        string
	}
	keyed := make([]keyedDefinition, len(definitions))
	for i, definition := range definitions {
		keyed[i] = keyedDefinition{definition: definition, key: toolDefinitionOrderKey(definition)}
	}
	sort.Slice(keyed, func(i, j int) bool {
		return keyed[i].key < keyed[j].key
	})
	ordered := make([]types.ToolDefinition, len(keyed))
	for i := range keyed {
		ordered[i] = keyed[i].definition
	}
	return ordered
}

func toolDefinitionOrderKey(definition types.ToolDefinition) string {
	// Tool names are expected to be unique. The remaining wire-visible fields
	// provide a deterministic tie-breaker for malformed duplicate-name input.
	wireShape := struct {
		Name        string           `json:"name"`
		Description string           `json:"description"`
		InputSchema types.JSONSchema `json:"input_schema"`
		Strict      bool             `json:"strict"`
	}{
		Name:        definition.Name,
		Description: definition.Description,
		InputSchema: definition.InputSchema,
		Strict:      definition.Strict,
	}
	raw, _ := json.Marshal(wireShape)
	return strings.ToLower(strings.TrimSpace(definition.Name)) + "\x00" + definition.Name + "\x00" + string(raw)
}

// canonicalServerToolDefinitions applies the same copy-before-sort contract to
// provider-hosted tools, whose type and options are all wire-visible.
func canonicalServerToolDefinitions(definitions []types.ServerToolDefinition) []types.ServerToolDefinition {
	type keyedDefinition struct {
		definition types.ServerToolDefinition
		key        string
	}
	keyed := make([]keyedDefinition, len(definitions))
	for i, definition := range definitions {
		keyed[i] = keyedDefinition{definition: definition, key: serverToolDefinitionOrderKey(definition)}
	}
	sort.Slice(keyed, func(i, j int) bool {
		return keyed[i].key < keyed[j].key
	})
	ordered := make([]types.ServerToolDefinition, len(keyed))
	for i := range keyed {
		ordered[i] = keyed[i].definition
	}
	return ordered
}

func serverToolDefinitionOrderKey(definition types.ServerToolDefinition) string {
	raw, _ := json.Marshal(definition)
	return strings.ToLower(strings.TrimSpace(definition.Type)) + "\x00" +
		strings.ToLower(strings.TrimSpace(definition.Name)) + "\x00" + string(raw)
}
