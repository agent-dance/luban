package skills

import (
	"errors"
	"net/url"
	"strings"
)

// ReplaceMCPCatalogInputsAtGeneration atomically replaces prompt- and
// resource-backed MCP inputs while the workspace generation is current.
func (m *Manager) ReplaceMCPCatalogInputsAtGeneration(expected ProjectSourceGeneration, inputs []MCPCatalogInput) error {
	if m == nil {
		return errors.New("nil skill manager")
	}
	if err := expected.Validate(); err != nil {
		return err
	}
	validated, err := validateMCPCatalogInputs(inputs)
	if err != nil {
		return err
	}
	m.txnMu.RLock()
	defer m.txnMu.RUnlock()
	m.mu.RLock()
	current := m.projectGeneration
	m.mu.RUnlock()
	if current != expected {
		return projectGenerationChangedError(expected, current)
	}
	store := m.ensureMCPCatalogStore()
	store.replace(validated)
	m.mu.Lock()
	m.populated = false
	m.mu.Unlock()
	return nil
}

// StageMCPCatalogInputs attaches a prevalidated unified MCP projection to an
// immutable workspace plan. CommitProjectSources publishes both the project
// directories and this projection under the same txnMu writer, so readers can
// observe only the old pair or the new pair.
func (m *Manager) StageMCPCatalogInputs(plan *ProjectSourcePlan, inputs []MCPCatalogInput) error {
	if m == nil || plan == nil || plan.manager != m {
		return errors.New("invalid skill project-source plan")
	}
	validated, err := validateMCPCatalogInputs(inputs)
	if err != nil {
		return err
	}
	plan.mcpCatalogInputs = validated
	plan.mcpProjectionSet = true
	return nil
}

func validateMCPCatalogInputs(inputs []MCPCatalogInput) ([]MCPCatalogInput, error) {
	validated := make([]MCPCatalogInput, 0, len(inputs))
	seen := make(map[SkillID]struct{}, len(inputs))
	for _, input := range inputs {
		if err := input.Validate(); err != nil {
			return nil, err
		}
		if _, err := mcpCatalogInputKind(input); err != nil {
			return nil, err
		}
		if _, duplicate := seen[input.ID]; duplicate {
			continue
		}
		seen[input.ID] = struct{}{}
		validated = append(validated, input.Clone())
	}
	return validated, nil
}

// MCPServerNameForCatalogInput returns the exact server identity encoded in a
// stable MCP locator. Runtime retargeting uses it to retain successful
// non-workspace projections without reverse-parsing presentation names.
func MCPServerNameForCatalogInput(input MCPCatalogInput) (string, error) {
	parsed, err := parseMCPCatalogInputLocator(input)
	if err != nil {
		return "", ErrInvalidSkillLocator
	}
	server := strings.TrimSpace(parsed.Query().Get("server"))
	if strings.EqualFold(parsed.Host, "resource") && server == "" {
		return "", ErrInvalidSkillLocator
	}
	return server, nil
}

func mcpCatalogInputKind(input MCPCatalogInput) (string, error) {
	parsed, err := parseMCPCatalogInputLocator(input)
	if err != nil {
		return "", err
	}
	kind := strings.ToLower(strings.TrimSpace(parsed.Host))
	if kind != "prompt" && kind != "resource" {
		return "", ErrInvalidSkillLocator
	}
	return kind, nil
}

func parseMCPCatalogInputLocator(input MCPCatalogInput) (*url.URL, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}
	parsed, err := url.Parse(string(input.Locator))
	if err != nil || !strings.EqualFold(parsed.Scheme, mcpCatalogLocatorScheme) {
		return nil, ErrInvalidSkillLocator
	}
	return parsed, nil
}
