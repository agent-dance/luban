package skills

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/agent-dance/luban/internal/mcp/catalog"
	mcpmanager "github.com/agent-dance/luban/internal/mcp/manager"
)

// discoverMCPPromptCatalogInputsFromConnections resolves prompts/get with
// stable placeholder values, turning the exact remotely returned template into
// the Skill body. SkillTool later substitutes those named placeholders with
// invocation arguments, so prompt-backed and resource-backed MCP skills share
// one execution and revision contract.
func discoverMCPPromptCatalogInputsFromConnections(ctx context.Context, states []mcpmanager.MCPServerConnection) ([]MCPCatalogInput, error) {
	if !mcpSkillsFeatureEnabled() {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var out []MCPCatalogInput
	var errs []error
	for _, state := range states {
		if state.Type != mcpmanager.MCPStateConnected || state.Client == nil || !mcpCapabilityExists(state.Capabilities, "prompts") {
			continue
		}
		prompts := append([]catalog.PromptDefinition(nil), state.Prompts...)
		if len(prompts) == 0 {
			result, err := state.Client.ListPrompts(ctx)
			if err != nil {
				errs = append(errs, fmt.Errorf("skills: list MCP prompts for %s: %w", state.Name, err))
				continue
			}
			prompts = result.Prompts
		}
		for _, definition := range prompts {
			name := strings.TrimSpace(definition.Name)
			if name == "" {
				continue
			}
			argNames := mcpPromptArgumentNames(definition.Arguments)
			placeholderArgs := make(map[string]string, len(argNames))
			for _, argName := range argNames {
				placeholderArgs[argName] = "$" + argName
			}
			result, err := state.Client.GetPrompt(ctx, name, placeholderArgs)
			if err != nil {
				errs = append(errs, fmt.Errorf("skills: get MCP prompt %s from %s: %w", name, state.Name, err))
				continue
			}
			body := mcpPromptTemplateBody(result.Messages)
			if strings.TrimSpace(body) == "" {
				continue
			}
			description := strings.TrimSpace(definition.Description)
			if description == "" {
				description = strings.TrimSpace(result.Description)
			}
			input, inputErr := NewMCPPromptCatalogInput(state.Name, name, description, argNames, body)
			if inputErr != nil {
				errs = append(errs, fmt.Errorf("skills: catalog MCP prompt %s from %s: %w", name, state.Name, inputErr))
				continue
			}
			out = append(out, input)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, errors.Join(errs...)
}

// DiscoverMCPCatalogInputsFromConnections resolves both MCP prompt templates
// and skill:// resources as one all-or-nothing projection.
func DiscoverMCPCatalogInputsFromConnections(ctx context.Context, states []mcpmanager.MCPServerConnection) ([]MCPCatalogInput, error) {
	resourceInputs, resourceErr := discoverMCPResourceCatalogInputsFromConnections(ctx, states)
	promptInputs, promptErr := discoverMCPPromptCatalogInputsFromConnections(ctx, states)
	if err := errors.Join(resourceErr, promptErr); err != nil {
		return nil, err
	}
	inputs := append(resourceInputs, promptInputs...)
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].ID < inputs[j].ID })
	return inputs, nil
}

func mcpPromptArgumentNames(arguments []catalog.PromptArgument) []string {
	out := make([]string, 0, len(arguments))
	for _, argument := range arguments {
		if name := strings.TrimSpace(argument.Name); name != "" {
			out = append(out, name)
		}
	}
	return out
}

func mcpPromptTemplateBody(messages []catalog.PromptMessage) string {
	parts := make([]string, 0, len(messages))
	for _, message := range messages {
		body := mcpPromptContentText(message.Content)
		if strings.TrimSpace(body) == "" {
			continue
		}
		if role := strings.TrimSpace(message.Role); role != "" {
			body = "<" + role + ">\n" + body
		}
		parts = append(parts, body)
	}
	return strings.Join(parts, "\n\n")
}

func mcpPromptContentText(content catalog.PromptContent) string {
	switch content.Type {
	case "text":
		return content.Text
	case "resource":
		if content.Resource == nil {
			return ""
		}
		if content.Resource.Text != "" {
			return content.Resource.Text
		}
	case "resource_link":
		return strings.TrimSpace(content.URI)
	}
	raw, err := json.Marshal(content)
	if err != nil {
		return ""
	}
	return string(raw)
}
