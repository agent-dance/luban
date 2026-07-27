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
		Name        string                   `json:"name"`
		Description string                   `json:"description"`
		InputSchema types.JSONSchema         `json:"input_schema"`
		Strict      bool                     `json:"strict"`
		Type        types.ToolDefinitionType `json:"type,omitempty"`
		Format      *types.ToolInputFormat   `json:"format,omitempty"`
	}{
		Name:        definition.Name,
		Description: definition.Description,
		InputSchema: definition.InputSchema,
		Strict:      definition.Strict,
		Type:        definition.Type,
		Format:      definition.Format,
	}
	raw, _ := json.Marshal(wireShape)
	return agenticToolWorkflowOrder(definition.Name) + "\x00" +
		strings.ToLower(strings.TrimSpace(definition.Name)) + "\x00" + definition.Name + "\x00" + string(raw)
}

// agenticToolWorkflowOrder keeps the three-tool coding kernel in the same
// inspect → mutate → verify order used by the system prompt. The prefix remains
// deterministic for arbitrary caller slices, so cache stability does not rely
// on registration order.
func agenticToolWorkflowOrder(name string) string {
	switch strings.TrimSpace(name) {
	case "Inspect":
		return "0"
	case "ApplyPatch":
		return "1"
	case "Run":
		return "2"
	default:
		return "3"
	}
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
