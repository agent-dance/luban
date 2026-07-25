package agent

// This file contains the agent-definition snapshot used by compaction and the
// presentation helpers consumed by the Agent tool contract.

import (
	"path"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/registry"
)

const agentSourceBuiltin = "builtin"

// AgentDefinition is the public, copy-friendly representation of an agent
// profile loaded from built-ins or .luban-code/agents/*.md frontmatter. It mirrors
// the relevant subset of agentProfile so external callers can introspect the
// registry without depending on internal types.
type AgentDefinition struct {
	Name            string
	WhenToUse       string
	SystemPrefix    string
	Model           string
	Isolation       string
	Skills          []string
	MCPServers      []string
	AllowedTools    []string
	DisallowedTools []string
	MaxTurns        int
	Background      bool
	Color           string
	Source          string // "builtin" | "project" | "user" | "managed"
}

// LoadAgentDefinitionsForRuntime returns the definitions visible to the
// active Agent runtime.
func (t *AgentTool) LoadAgentDefinitionsForRuntime(cwd string) ([]AgentDefinition, error) {
	defs := []AgentDefinition{}
	for _, p := range t.builtinAgentProfilesForRuntime() {
		def := agentProfileToDefinition(p)
		def.Source = "builtin"
		defs = append(defs, def)
	}
	custom, err := loadCustomAgentProfiles(cwd)
	if err != nil {
		return defs, err
	}
	byName := map[string]int{}
	for i, def := range defs {
		byName[strings.ToLower(strings.TrimSpace(def.Name))] = i
	}
	for _, p := range custom {
		def := agentProfileToDefinition(p)
		def.Source = "project"
		key := strings.ToLower(strings.TrimSpace(def.Name))
		if idx, ok := byName[key]; ok {
			defs[idx] = def
			continue
		}
		byName[key] = len(defs)
		defs = append(defs, def)
	}
	if t != nil && t.Registry != nil {
		if err := validateAgentDefinitionToolAllowLists(defs, t.Registry); err != nil {
			return nil, err
		}
	}
	return defs, nil
}

func validateAgentDefinitionToolAllowLists(defs []AgentDefinition, reg *registry.Registry) error {
	if reg == nil {
		return nil
	}
	available := reg.Names()
	for _, def := range defs {
		if strings.EqualFold(strings.TrimSpace(def.Source), agentSourceBuiltin) {
			continue
		}
		for _, spec := range def.AllowedTools {
			raw := strings.TrimSpace(spec)
			if raw == "" || raw == "*" {
				continue
			}
			name := strings.TrimSpace(raw)
			if parsed := normalizedToolNameFromPermissionSpec(raw); parsed != "" {
				name = parsed
			}
			matched := false
			for _, candidate := range available {
				if strings.EqualFold(candidate, name) {
					matched = true
				}
				if !matched && strings.ContainsAny(name, "*?[") {
					matched, _ = path.Match(strings.ToLower(name), strings.ToLower(candidate))
				}
				if matched {
					break
				}
			}
			if !matched {
				return i18n.NewError(i18n.KeyToolAgentDefinitionUnknownTool, def.Name, raw)
			}
		}
	}
	return nil
}

func agentProfileToDefinition(p agentProfile) AgentDefinition {
	allowed := make([]string, 0, len(p.AllowedToolSpecs))
	allowed = append(allowed, p.AllowedToolSpecs...)
	disallowed := make([]string, 0, len(p.DisallowedToolSpecs))
	disallowed = append(disallowed, p.DisallowedToolSpecs...)
	skills := make([]string, 0, len(p.Skills))
	skills = append(skills, p.Skills...)
	mcp := make([]string, 0, len(p.MCPServers))
	mcp = append(mcp, p.MCPServers...)
	return AgentDefinition{
		Name:            p.Name,
		WhenToUse:       p.WhenToUse,
		SystemPrefix:    p.SystemPrefix,
		Model:           p.Model,
		Isolation:       p.Isolation,
		Skills:          skills,
		MCPServers:      mcp,
		AllowedTools:    allowed,
		DisallowedTools: disallowed,
		MaxTurns:        p.MaxTurns,
		Background:      p.Background,
		Color:           p.Color,
	}
}
